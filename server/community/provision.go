package community

import (
	"context"
	"fmt"

	chatlib "go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/clock"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// This file is the shared community-creation primitive (#2492). It lives in the
// `community` library — not a service — so any service can create a community
// (e.g. the per-item ad-hoc community each item is born with) without depending
// on another service, per docs/server/architecture.md § Service Independence.
//
// The community RPC service's CreateCommunity / ProvisionAdHocCommunity delegate
// here, and the item services (experience, request) call ProvisionPerItemCommunity
// directly at item creation.

// Origin identifies the single item an ad-hoc per-item community is the audience
// for. Exactly one field must be non-empty; it maps to the
// Community.origin_item oneof.
type Origin struct {
	ExperienceID string
	GearID       string
	RequestID    string
	TransferID   string
}

// Validate reports whether exactly one origin id is set. ErrInvalidOrigin is
// returned otherwise so callers can map it to an InvalidArgument response.
func (o Origin) Validate() error {
	set := 0
	for _, id := range []string{o.ExperienceID, o.GearID, o.RequestID, o.TransferID} {
		if id != "" {
			set++
		}
	}
	if set != 1 {
		return fmt.Errorf("%w: got %d item ids", ErrInvalidOrigin, set)
	}
	return nil
}

// ErrInvalidOrigin is returned by Origin.Validate when the origin doesn't set
// exactly one item id. Callers map it to a Connect InvalidArgument error.
var ErrInvalidOrigin = fmt.Errorf("ad-hoc community origin must set exactly one item id")

// apply sets the matching Community.origin_item oneof variant, after validating
// that exactly one origin id is present.
func (o Origin) apply(c *models.Community) error {
	if err := o.Validate(); err != nil {
		return err
	}
	switch {
	case o.ExperienceID != "":
		c.OriginItem = &models.Community_OriginExperienceId{OriginExperienceId: o.ExperienceID}
	case o.GearID != "":
		c.OriginItem = &models.Community_OriginGearId{OriginGearId: o.GearID}
	case o.RequestID != "":
		c.OriginItem = &models.Community_OriginRequestId{OriginRequestId: o.RequestID}
	case o.TransferID != "":
		c.OriginItem = &models.Community_OriginTransferId{OriginTransferId: o.TransferID}
	}
	return nil
}

// column returns the protosql column and id for this origin's set variant. The
// Community origin_item oneof columnizes to the bare field names (see
// protosql_schema.go), so the column is e.g. "origin_experience_id".
func (o Origin) column() (column, id string) {
	switch {
	case o.ExperienceID != "":
		return "origin_experience_id", o.ExperienceID
	case o.GearID != "":
		return "origin_gear_id", o.GearID
	case o.RequestID != "":
		return "origin_request_id", o.RequestID
	case o.TransferID != "":
		return "origin_transfer_id", o.TransferID
	}
	return "", ""
}

// NewCommunityParams carries everything CreateCommunity needs. A named community
// leaves Origin nil; an ad-hoc per-item community sets Origin and leaves Name
// empty (name-presence is the ad-hoc-vs-real discriminator).
type NewCommunityParams struct {
	Name         string
	Description  string
	OwnerUserID  string
	MediaIDs     []string
	SimulationID *string
	// Origin, when non-nil, records the item this community is the audience for
	// and leaves the community unnamed (ad-hoc).
	Origin *Origin
	// ExtraMembers seeds additional real-user members beyond the owner. Deduped
	// against the owner.
	ExtraMembers []string
	// MemberSignature is the ad-hoc audience dedup key. Empty for named
	// communities and audience-less (host-only) ad-hoc communities.
	MemberSignature string
}

// CreateCommunity is the shared community-creation core: insert the row, seed
// membership (owner + any extras), compute regions, publish the creation event,
// and open the community conversation. Returns the new community id. Errors are
// plain (callers wrap with connecterr as needed).
func CreateCommunity(ctx context.Context, store *storage.ProtoSQLStorage, bus cebus.Publisher, p NewCommunityParams) (string, error) {
	now := clock.UnixSec(ctx)

	community := &models.Community{
		Name:             p.Name,
		Description:      p.Description,
		CreatorId:        p.OwnerUserID,
		OwnerUserId:      p.OwnerUserID,
		MediaIds:         p.MediaIDs,
		CreatedAtUnixSec: now,
		UpdatedAtUnixSec: now,
		SimulationId:     p.SimulationID,
		MemberSignature:  p.MemberSignature,
	}
	if p.Origin != nil {
		if err := p.Origin.apply(community); err != nil {
			return "", err
		}
	}

	id, err := store.Insert(ctx, community)
	if err != nil {
		return "", fmt.Errorf("insert community: %w", err)
	}

	// Owner is always the first member. Dedupe extras against the owner so the
	// community_user uniqueness constraint isn't violated.
	memberIDs := []string{p.OwnerUserID}
	seen := map[string]bool{p.OwnerUserID: true}
	for _, m := range p.ExtraMembers {
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		memberIDs = append(memberIDs, m)
	}
	for _, uid := range memberIDs {
		if _, err := store.Insert(ctx, &models.CommunityUser{
			CommunityId:      id,
			UserId:           uid,
			InviterId:        p.OwnerUserID,
			CreatedAtUnixSec: now,
		}); err != nil {
			return "", fmt.Errorf("seed community member: %w", err)
		}
	}

	// Compute initial regions from the seeded members' primary locations.
	regions, err := ComputeRegions(ctx, store, id)
	if err != nil {
		return "", fmt.Errorf("compute community regions: %w", err)
	}
	if err := SaveRegions(ctx, store, id, regions); err != nil {
		return "", fmt.Errorf("save community regions: %w", err)
	}

	if _, err := bus.Publish(ctx, &models.CommunityEvent{
		CommunityId: id,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_CREATED,
		ActorId:     p.OwnerUserID,
	}); err != nil {
		return "", fmt.Errorf("publish community creation event: %w", err)
	}

	// Open the community-wide conversation. A backfill job recovers communities
	// that miss this call, so a failure here is logged but non-fatal.
	chatConvStore := storage.NewChatConversationStorage(store)
	if _, err := chatlib.CreateCommunityConversation(ctx, store, chatConvStore, id); err != nil {
		logging.LoggerWithContext(ctx).Warn("failed to create community conversation during community creation",
			"error", err, "community_id", id)
	}

	return id, nil
}

// FindAdHocByOrigin returns the id of the per-item ad-hoc community provisioned
// for this item — matched on its origin_item back-reference — or found=false.
// Each item provisions exactly one nameless per-item community, so at most one
// ad-hoc match exists.
func FindAdHocByOrigin(ctx context.Context, store *storage.ProtoSQLStorage, origin Origin) (string, bool, error) {
	column, id := origin.column()
	if id == "" {
		return "", false, nil
	}
	rows, err := storage.QueryByField[*models.Community](store, ctx, column, id)
	if err != nil {
		return "", false, fmt.Errorf("query community by origin: %w", err)
	}
	for _, c := range rows {
		if c.Name == "" { // nameless ⇒ ad-hoc
			return c.Id, true, nil
		}
	}
	return "", false, nil
}

// ProvisionPerItemCommunity returns the per-item ad-hoc community for an item,
// creating a host-only nameless one (origin set) if it doesn't exist yet (#2492).
// Idempotent by origin: the item-creation flow calls it once at insert. Every
// item gets its own community, keyed by origin_item — there is no audience-
// signature dedup here.
func ProvisionPerItemCommunity(ctx context.Context, store *storage.ProtoSQLStorage, bus cebus.Publisher, hostUserID string, origin Origin) (string, error) {
	if hostUserID == "" {
		return "", fmt.Errorf("per-item community requires a host user id")
	}
	if id, found, err := FindAdHocByOrigin(ctx, store, origin); err != nil {
		return "", err
	} else if found {
		return id, nil
	}
	return CreateCommunity(ctx, store, bus, NewCommunityParams{
		OwnerUserID: hostUserID,
		Origin:      &origin,
	})
}
