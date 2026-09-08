// Package media — per-resource read authorization for the GetMedia RPC.
//
// canUserAccessMedia answers "does this user have any legitimate path to
// this media id?" by checking, in cheapest-first order: owner, avatar,
// community-direct surface (Community / Story), community-via-join
// surface (Gear / Experience / Request), or chat participant.
//
// See docs/issues/1529-getmedia-auth.md for the design + audit.
package media

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/gen/ripls/models"
	mediapkg "go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/storage"
)

// Access-rule reason codes. Logged on every GetMedia call so production
// telemetry shows which surface granted (or failed to grant) access.
//
// Deny codes are per-surface: each per-surface helper returns the
// most-specific failure it observed (no link at all vs. link present
// but caller not a member). canUserAccessMedia surfaces the deepest
// observed deny so a single `would_deny` log entry names the closest
// near-miss.
const (
	accessReasonOwner      = "owner"
	accessReasonSystem     = "system"
	accessReasonAvatar     = "avatar"
	accessReasonCommunity  = "community"
	accessReasonStory      = "story"
	accessReasonGear       = "gear"
	accessReasonExperience = "experience"
	accessReasonRequest    = "request"
	accessReasonChat       = "chat"
	accessReasonTransfer   = "transfer"

	// Per-surface deny reasons. "no-link" means the array-contains probe
	// returned zero hits — this media id is not surfaced on any row of
	// that type. "no-membership" means hits exist but the caller's
	// membership / participation check failed against all of them.
	accessReasonDenyNotOwnerOrAvatar          = "deny:not-owner-or-avatar"
	accessReasonDenyNoCommunityLink           = "deny:no-community-link"
	accessReasonDenyNoCommunityMembership     = "deny:no-community-membership"
	accessReasonDenyNoStoryCommunity          = "deny:no-story-community"
	accessReasonDenyNoStoryMembership         = "deny:no-story-membership"
	accessReasonDenyNoGearLink                = "deny:no-gear-link"
	accessReasonDenyNoGearCommunityMembership = "deny:no-gear-community-membership"
	accessReasonDenyNoExpLink                 = "deny:no-experience-link"
	accessReasonDenyNoExpCommunityMembership  = "deny:no-experience-community-membership"
	accessReasonDenyNoReqLink                 = "deny:no-request-link"
	accessReasonDenyNoReqCommunityMembership  = "deny:no-request-community-membership"
	accessReasonDenyNoChatMessage             = "deny:no-chat-message"
	accessReasonDenyNoChatParticipation       = "deny:no-chat-participation"
	accessReasonDenyNoTransferLink            = "deny:no-transfer-link"
	accessReasonDenyNoTransferParticipation   = "deny:no-transfer-participation"

	accessReasonUnknown = ""
)

// canUserAccessMedia returns whether userID has a legitimate read path to
// media. On allow, reason names the matching rule. On deny, reason is the
// most-specific deny code observed across the probes (a membership-deny
// when any surface saw a hit-but-no-membership, else the universal
// accessReasonDenyNotOwnerOrAvatar). err is non-nil only on storage
// failures, never on a clean deny.
//
// Short-circuit order is cheapest-first: owner is one comparison (no SQL),
// avatar is a single indexed lookup, and each community-surface branch
// costs one array-contains probe + 1-N membership checks.
func canUserAccessMedia(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	userID string,
	media *models.Media,
) (bool, string, error) {
	if media == nil {
		return false, accessReasonUnknown, errors.New("canUserAccessMedia: media must not be nil")
	}

	// 1. Owner. Zero SQL.
	if media.UserId == userID {
		return true, accessReasonOwner, nil
	}

	// 1.5. System-owned. Stock images (Unsplash, Pexels, Pixabay,
	//      and the fake provider used in dev / tests) are written
	//      with UserId == media.SystemUserID. They are public-by-
	//      design — any authenticated caller is allowed. Zero SQL.
	if media.UserId == mediapkg.SystemUserID {
		return true, accessReasonSystem, nil
	}

	// 2. Avatar. Any user whose media_ids contains this id — broadly
	//    visible by design since avatars surface in cross-community
	//    contexts (search results, notifications, who-RSVPed).
	hits, err := s.QueryByArrayContains(ctx, "media_ids", media.Id, &models.User{})
	if err != nil {
		return false, accessReasonUnknown, fmt.Errorf("avatar probe: %w", err)
	}
	if len(hits) > 0 {
		return true, accessReasonAvatar, nil
	}

	// Track the deepest observed deny across the remaining probes.
	// "Deepest" = the most-specific failure reason we saw, so a deny
	// log names the closest near-miss rather than collapsing to "deny".
	denyReason := accessReasonDenyNotOwnerOrAvatar

	// 3. Community direct. Community avatar / banner; caller must be a
	//    member of the community holding the media.
	ok, reason, err := communityDirectAllows(ctx, s, userID, media.Id)
	if err != nil {
		return false, accessReasonUnknown, fmt.Errorf("community probe: %w", err)
	}
	if ok {
		return true, accessReasonCommunity, nil
	}
	denyReason = mostSpecificDeny(denyReason, reason)

	// 4. Story. Stories carry a direct community_id, no join row.
	ok, reason, err = storyAllows(ctx, s, userID, media.Id)
	if err != nil {
		return false, accessReasonUnknown, fmt.Errorf("story probe: %w", err)
	}
	if ok {
		return true, accessReasonStory, nil
	}
	denyReason = mostSpecificDeny(denyReason, reason)

	// 5. Gear. Media is on a gear; the gear is shared to one or more
	//    communities via CommunityGear join rows; caller is a member.
	ok, reason, err = gearAllows(ctx, s, userID, media.Id)
	if err != nil {
		return false, accessReasonUnknown, fmt.Errorf("gear probe: %w", err)
	}
	if ok {
		return true, accessReasonGear, nil
	}
	denyReason = mostSpecificDeny(denyReason, reason)

	// 5.5. Transfer. The caller is a party (owner or recipient) to a
	//      loan/giveaway of the gear carrying this media. This rescues
	//      loan-history views where the gear is no longer surfaced in a
	//      community the caller belongs to — most importantly when the
	//      gear has been soft-deleted, which the gear probe above cannot
	//      see. The transfer relationship is the durable grant, so this
	//      probe intentionally looks through soft-deleted gear.
	ok, reason, err = transferAllows(ctx, s, userID, media.Id)
	if err != nil {
		return false, accessReasonUnknown, fmt.Errorf("transfer probe: %w", err)
	}
	if ok {
		return true, accessReasonTransfer, nil
	}
	denyReason = mostSpecificDeny(denyReason, reason)

	// 6. Experience. Same shape as Gear via CommunityExperience.
	ok, reason, err = experienceAllows(ctx, s, userID, media.Id)
	if err != nil {
		return false, accessReasonUnknown, fmt.Errorf("experience probe: %w", err)
	}
	if ok {
		return true, accessReasonExperience, nil
	}
	denyReason = mostSpecificDeny(denyReason, reason)

	// 7. Request. Same shape as Gear via CommunityRequest.
	ok, reason, err = requestAllows(ctx, s, userID, media.Id)
	if err != nil {
		return false, accessReasonUnknown, fmt.Errorf("request probe: %w", err)
	}
	if ok {
		return true, accessReasonRequest, nil
	}
	denyReason = mostSpecificDeny(denyReason, reason)

	// 8. Chat. Media is attached to a ChatMessage; caller participates
	//    in the parent ChatConversation.
	ok, reason, err = chatAllows(ctx, s, userID, media.Id)
	if err != nil {
		return false, accessReasonUnknown, fmt.Errorf("chat probe: %w", err)
	}
	if ok {
		return true, accessReasonChat, nil
	}
	denyReason = mostSpecificDeny(denyReason, reason)

	return false, denyReason, nil
}

// mostSpecificDeny upgrades the running deny reason only when the
// incoming reason is a membership-deny — those are the real near-misses
// (a probe actually walked rows but the caller's membership /
// participation check failed). Surface-specific "no-link" codes carry
// no distinct information at the rollup; if no surface had any link,
// the universal accessReasonDenyNotOwnerOrAvatar code is the right
// signal. When multiple membership-denies fire, the last one wins;
// per-surface helpers can be inspected directly from unit tests for
// finer-grained assertions.
func mostSpecificDeny(current, incoming string) string {
	if isMembershipDeny(incoming) {
		return incoming
	}
	return current
}

func isMembershipDeny(reason string) bool {
	switch reason {
	case accessReasonDenyNoCommunityMembership,
		accessReasonDenyNoStoryMembership,
		accessReasonDenyNoGearCommunityMembership,
		accessReasonDenyNoExpCommunityMembership,
		accessReasonDenyNoReqCommunityMembership,
		accessReasonDenyNoChatParticipation,
		accessReasonDenyNoTransferParticipation:
		return true
	}
	return false
}

// requireMediaReadAccess wraps [canUserAccessMedia] for handlers that
// enforce the rule. Returns nil on allow, [connect.CodePermissionDenied]
// on deny, the original error on storage failure.
//
// See #1529. Callers that have not yet flipped to enforce mode use
// canUserAccessMedia directly for log-only telemetry and continue
// regardless of the result; switching to this wrapper enforces the rule.
func requireMediaReadAccess(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	userID string,
	media *models.Media,
) error {
	allowed, _, err := canUserAccessMedia(ctx, s, userID, media)
	if err != nil {
		return err
	}
	if !allowed {
		return connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("not authorized to access this media"))
	}
	return nil
}

// communityDirectAllows checks the Community rule: media surfaces on a
// community's own media_ids and the caller is a member. Returns the
// specific deny reason (no-link vs no-membership) on a clean false.
func communityDirectAllows(ctx context.Context, s *storage.ProtoSQLStorage, userID, mediaID string) (bool, string, error) {
	hits, err := s.QueryByArrayContains(ctx, "media_ids", mediaID, &models.Community{})
	if err != nil {
		return false, accessReasonUnknown, err
	}
	if len(hits) == 0 {
		return false, accessReasonDenyNoCommunityLink, nil
	}
	for _, hit := range hits {
		community := hit.(*models.Community)
		if community.Id == "" {
			continue
		}
		ok, err := auth.IsMemberOfCommunity(ctx, s, community.Id, userID)
		if err != nil {
			return false, accessReasonUnknown, err
		}
		if ok {
			return true, accessReasonCommunity, nil
		}
	}
	return false, accessReasonDenyNoCommunityMembership, nil
}

// storyAllows checks the Story rule: media is on a story's media_ids and
// the caller is a member of the story's community. Returns the specific
// deny reason (no-link vs no-membership) on a clean false.
func storyAllows(ctx context.Context, s *storage.ProtoSQLStorage, userID, mediaID string) (bool, string, error) {
	hits, err := s.QueryByArrayContains(ctx, "media_ids", mediaID, &models.Story{})
	if err != nil {
		return false, accessReasonUnknown, err
	}
	if len(hits) == 0 {
		return false, accessReasonDenyNoStoryCommunity, nil
	}
	for _, hit := range hits {
		story := hit.(*models.Story)
		if story.CommunityId == "" {
			continue
		}
		ok, err := auth.IsMemberOfCommunity(ctx, s, story.CommunityId, userID)
		if err != nil {
			return false, accessReasonUnknown, err
		}
		if ok {
			return true, accessReasonStory, nil
		}
	}
	return false, accessReasonDenyNoStoryMembership, nil
}

// gearAllows checks the Gear rule: media is on a gear's media_ids; the
// gear is shared to one or more communities via CommunityGear; the caller
// is a member of any one of them. Returns the specific deny reason on a
// clean false.
func gearAllows(ctx context.Context, s *storage.ProtoSQLStorage, userID, mediaID string) (bool, string, error) {
	gears, err := s.QueryByArrayContains(ctx, "media_ids", mediaID, &models.Gear{})
	if err != nil {
		return false, accessReasonUnknown, err
	}
	if len(gears) == 0 {
		return false, accessReasonDenyNoGearLink, nil
	}
	for _, hit := range gears {
		gear := hit.(*models.Gear)
		joins, err := s.QueryByField(ctx, "gear_id", gear.Id, &models.CommunityGear{})
		if err != nil {
			return false, accessReasonUnknown, err
		}
		if ok, err := anyCommunityMembership(ctx, s, userID, joins, communityIDFromCommunityGear); err != nil {
			return false, accessReasonUnknown, err
		} else if ok {
			return true, accessReasonGear, nil
		}
	}
	return false, accessReasonDenyNoGearCommunityMembership, nil
}

// transferAllows checks the Transfer rule: the media is on a gear, and the
// caller is a party (owner or recipient) to a loan/giveaway of that gear.
//
// Unlike the other surfaces, the gear lookup opts into IncludeDeleted: a
// transfer outlives the gear (gear deletion is soft — the row persists), and
// the loan inbox / transfer-context surfaces deliberately render thumbnails
// for soft-deleted gear (server/services/transfer/service.go loads gear with
// IncludeDeleted). The durable authorization is the transfer relationship, not
// the gear's current community sharing, so a historical participant retains
// read access to the gear's media. Returns the specific deny reason on a clean
// false: no-transfer-link when no transfer references a gear carrying the media
// (a non-membership deny, so it does not upgrade the rollup past the universal
// no-owner-or-avatar code), and no-transfer-participation when transfers exist
// but the caller is neither owner nor recipient (a membership-deny near-miss).
func transferAllows(ctx context.Context, s *storage.ProtoSQLStorage, userID, mediaID string) (bool, string, error) {
	gears, err := s.QueryByArrayContains(ctx, "media_ids", mediaID, &models.Gear{}, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		return false, accessReasonUnknown, err
	}
	if len(gears) == 0 {
		return false, accessReasonDenyNoTransferLink, nil
	}
	sawTransfer := false
	for _, hit := range gears {
		gear := hit.(*models.Gear)
		transfers, err := s.QueryByField(ctx, "gear_id", gear.Id, &models.Transfer{}, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			return false, accessReasonUnknown, err
		}
		if len(transfers) > 0 {
			sawTransfer = true
		}
		for _, t := range transfers {
			transfer := t.(*models.Transfer)
			if transfer.OwnerId == userID || transfer.RecipientId == userID {
				return true, accessReasonTransfer, nil
			}
		}
	}
	// The gear carries the media but no transfer references it: there is no
	// transfer relationship at all, so this is a no-link deny (the universal
	// rollup code stands), not a participation near-miss.
	if !sawTransfer {
		return false, accessReasonDenyNoTransferLink, nil
	}
	return false, accessReasonDenyNoTransferParticipation, nil
}

// experienceAllows mirrors [gearAllows] for experiences via CommunityExperience.
func experienceAllows(ctx context.Context, s *storage.ProtoSQLStorage, userID, mediaID string) (bool, string, error) {
	experiences, err := s.QueryByArrayContains(ctx, "media_ids", mediaID, &models.Experience{})
	if err != nil {
		return false, accessReasonUnknown, err
	}
	if len(experiences) == 0 {
		return false, accessReasonDenyNoExpLink, nil
	}
	for _, hit := range experiences {
		exp := hit.(*models.Experience)
		joins, err := s.QueryByField(ctx, "experience_id", exp.Id, &models.CommunityExperience{})
		if err != nil {
			return false, accessReasonUnknown, err
		}
		if ok, err := anyCommunityMembership(ctx, s, userID, joins, communityIDFromCommunityExperience); err != nil {
			return false, accessReasonUnknown, err
		} else if ok {
			return true, accessReasonExperience, nil
		}
	}
	return false, accessReasonDenyNoExpCommunityMembership, nil
}

// requestAllows mirrors [gearAllows] for requests via CommunityRequest.
func requestAllows(ctx context.Context, s *storage.ProtoSQLStorage, userID, mediaID string) (bool, string, error) {
	requests, err := s.QueryByArrayContains(ctx, "media_ids", mediaID, &models.Request{})
	if err != nil {
		return false, accessReasonUnknown, err
	}
	if len(requests) == 0 {
		return false, accessReasonDenyNoReqLink, nil
	}
	for _, hit := range requests {
		req := hit.(*models.Request)
		joins, err := s.QueryByField(ctx, "request_id", req.Id, &models.CommunityRequest{})
		if err != nil {
			return false, accessReasonUnknown, err
		}
		if ok, err := anyCommunityMembership(ctx, s, userID, joins, communityIDFromCommunityRequest); err != nil {
			return false, accessReasonUnknown, err
		} else if ok {
			return true, accessReasonRequest, nil
		}
	}
	return false, accessReasonDenyNoReqCommunityMembership, nil
}

// chatAllows checks the Chat rule: media is attached to a ChatMessage
// and the caller is in the parent ChatConversation's participant_ids.
// Returns the specific deny reason on a clean false.
func chatAllows(ctx context.Context, s *storage.ProtoSQLStorage, userID, mediaID string) (bool, string, error) {
	msgs, err := s.QueryByArrayContains(ctx, "media_ids", mediaID, &models.ChatMessage{})
	if err != nil {
		return false, accessReasonUnknown, err
	}
	if len(msgs) == 0 {
		return false, accessReasonDenyNoChatMessage, nil
	}
	for _, hit := range msgs {
		msg := hit.(*models.ChatMessage)
		if msg.ConversationId == "" {
			continue
		}
		conv := &models.ChatConversation{}
		if err := s.GetByID(ctx, msg.ConversationId, conv); err != nil {
			// A missing conversation row shouldn't fail the whole check —
			// keep walking other matching messages.
			continue
		}
		for _, pid := range conv.ParticipantIds {
			if pid == userID {
				return true, accessReasonChat, nil
			}
		}
	}
	return false, accessReasonDenyNoChatParticipation, nil
}

// anyCommunityMembership returns true when userID is a member of any
// community referenced by the join rows in hits, via extractCommunityID.
func anyCommunityMembership(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	userID string,
	hits []proto.Message,
	extractCommunityID func(proto.Message) string,
) (bool, error) {
	for _, hit := range hits {
		communityID := extractCommunityID(hit)
		if communityID == "" {
			continue
		}
		ok, err := auth.IsMemberOfCommunity(ctx, s, communityID, userID)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func communityIDFromCommunityGear(m proto.Message) string {
	return m.(*models.CommunityGear).CommunityId
}

func communityIDFromCommunityExperience(m proto.Message) string {
	return m.(*models.CommunityExperience).CommunityId
}

func communityIDFromCommunityRequest(m proto.Message) string {
	return m.(*models.CommunityRequest).CommunityId
}
