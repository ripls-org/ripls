package community

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"connectrpc.com/connect"

	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// IsAdHoc reports whether a community is ad-hoc, i.e. nameless. Name-presence
// is the ad-hoc-vs-real discriminator (#2492): an unnamed community is the
// audience for a single item and becomes a real persistent community the moment
// it gains a name. There is deliberately no `kind` enum.
func IsAdHoc(c *models.Community) bool {
	return c != nil && c.Name == ""
}

// AdHocOrigin identifies the single item an ad-hoc community is spun up to be
// the audience for (#2492). Exactly one field must be non-empty; it maps to the
// Community.origin_item oneof. Exported so item-creation services (CREATE-1) can
// provision an ad-hoc community without reaching into the generated oneof
// wrapper types, which are unexported.
type AdHocOrigin struct {
	ExperienceID string
	GearID       string
	RequestID    string
	TransferID   string
}

// toLib converts to the shared library's Origin type. The service keeps its own
// AdHocOrigin (used by ShareItem / InviteConsent) and maps to communitylib.Origin
// when delegating community creation to the shared library.
func (o AdHocOrigin) toLib() communitylib.Origin {
	return communitylib.Origin{
		ExperienceID: o.ExperienceID,
		GearID:       o.GearID,
		RequestID:    o.RequestID,
		TransferID:   o.TransferID,
	}
}

// newCommunityParams carries everything createCommunity needs. Shared by the
// CreateCommunity RPC (named, no origin) and ProvisionAdHocCommunity (unnamed,
// origin set).
type newCommunityParams struct {
	name         string
	description  string
	ownerUserID  string
	mediaIDs     []string
	simulationID *string
	// origin, when non-nil, records the item this community is the audience
	// for and leaves the community unnamed (ad-hoc).
	origin *AdHocOrigin
	// extraMembers seeds additional real-user members beyond the owner.
	// Deduped against the owner. Provisional invitees are attached separately
	// via the provisional-user path, not here.
	extraMembers []string
	// memberSignature is the ad-hoc dedup key (see adHocMemberSignature). Empty
	// for named communities and audience-less ad-hoc communities.
	memberSignature string
}

// AdHocAudience is the intended audience of an ad-hoc community at creation
// time, used both to seed membership and to compute the dedup signature.
type AdHocAudience struct {
	// MemberUserIDs are real-user invitees seeded as members (beyond the host).
	MemberUserIDs []string
	// ContactHandles are normalized provisional-invitee handles (phone E.164 or
	// email). They contribute to the dedup signature so an all-phone-invite
	// audience dedups correctly, but they are NOT seeded as members here — the
	// caller attaches provisional users separately (idempotent on reuse).
	ContactHandles []string
}

// adHocMemberKeys returns the canonical, de-duplicated, sorted identity keys for
// an ad-hoc audience: the host plus every invitee, each tagged by kind ("u:" for
// a real-user id, "c:" for a contact handle) so a user id can never collide with
// a handle. Order-independent — the same set of people always yields the same
// keys.
func adHocMemberKeys(hostUserID string, audience AdHocAudience) []string {
	set := map[string]struct{}{"u:" + hostUserID: {}}
	for _, id := range audience.MemberUserIDs {
		if id != "" {
			set["u:"+id] = struct{}{}
		}
	}
	for _, h := range audience.ContactHandles {
		if h != "" {
			set["c:"+h] = struct{}{}
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// adHocMemberSignature hashes the canonical audience keys into the dedup key
// stored on Community.member_signature. Returns "" for an audience-less
// (host-only) ad-hoc community, which is never deduped.
func adHocMemberSignature(keys []string) string {
	if len(keys) <= 1 { // host only — nothing to dedup against
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(keys, "|")))
	return hex.EncodeToString(sum[:])
}

// findAdHocBySignature returns the id of an existing, live ad-hoc (nameless)
// community whose creation-time audience matches signature, or found=false. A
// non-empty signature is host-specific (the host id is one of the hashed keys),
// so a match is always one of this host's own ad-hoc communities.
func (s *Service) findAdHocBySignature(ctx context.Context, signature string) (string, bool, error) {
	if signature == "" {
		return "", false, nil
	}
	rows, err := storage.QueryByField[*models.Community](s.storage, ctx, "member_signature", signature)
	if err != nil {
		return "", false, connecterr.Internal(ctx, "findAdHocBySignature", err)
	}
	for _, c := range rows {
		if IsAdHoc(c) {
			return c.Id, true, nil
		}
	}
	return "", false, nil
}

// createCommunity delegates to the shared community-creation library
// (communitylib.CreateCommunity), which is the single owner of the create
// sequence (insert row, seed membership, compute regions, publish the creation
// event, open the conversation) so item services can create per-item communities
// without depending on this service. Wraps library errors as Connect errors.
func (s *Service) createCommunity(ctx context.Context, p newCommunityParams) (string, error) {
	var origin *communitylib.Origin
	if p.origin != nil {
		o := p.origin.toLib()
		if err := o.Validate(); err != nil {
			return "", connect.NewError(connect.CodeInvalidArgument, err)
		}
		origin = &o
	}
	id, err := communitylib.CreateCommunity(ctx, s.storage, s.bus, communitylib.NewCommunityParams{
		Name:            p.name,
		Description:     p.description,
		OwnerUserID:     p.ownerUserID,
		MediaIDs:        p.mediaIDs,
		SimulationID:    p.simulationID,
		Origin:          origin,
		ExtraMembers:    p.extraMembers,
		MemberSignature: p.memberSignature,
	})
	if err != nil {
		return "", connecterr.Internal(ctx, "createCommunity", err)
	}
	return id, nil
}

// ProvisionAdHocCommunity returns the nameless per-item community for an item's
// audience (#2492). hostUserID becomes creator+owner and first member; origin
// records which item the community was spun up for; audience carries the real
// and provisional invitees.
//
// Dedup (the reviewer's "three events with the same three people shouldn't be
// three communities"): if this host already has a live ad-hoc community whose
// creation-time audience is exactly the same set of people, that community is
// REUSED — its id is returned and nothing new is created, so the host's
// community list never shows duplicates. The reused community keeps its original
// `origin_item` (the first item it was spun up for); the caller associates the
// new item with the returned community. Only the audience is matched, not later
// emergent membership — post-creation join/leave drift is not retroactively
// merged (that "community merging" is deliberately out of scope).
//
// On a fresh create the community is intentionally left unnamed: name-presence
// is the ad-hoc-vs-real signal, and naming it later via UpdateCommunity promotes
// it to a real persistent community (the origin back-reference survives as
// provenance).
func (s *Service) ProvisionAdHocCommunity(ctx context.Context, hostUserID string, origin AdHocOrigin, audience AdHocAudience) (string, error) {
	if hostUserID == "" {
		return "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("ad-hoc community requires a host user id"))
	}

	keys := adHocMemberKeys(hostUserID, audience)
	signature := adHocMemberSignature(keys)
	if existingID, found, err := s.findAdHocBySignature(ctx, signature); err != nil {
		return "", err
	} else if found {
		logging.LoggerWithContext(ctx).InfoContext(ctx,
			"reusing existing ad-hoc community for identical audience",
			"community_id", existingID, "host_user_id", hostUserID)
		return existingID, nil
	}

	return s.createCommunity(ctx, newCommunityParams{
		ownerUserID:     hostUserID,
		origin:          &origin,
		extraMembers:    audience.MemberUserIDs,
		memberSignature: signature,
	})
}
