package storage

import (
	"context"
	"fmt"
	"slices"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// Leaver-scoped cascade helpers used by LeaveCommunity (#1656). Each
// helper is parameterised by (community_id, leaver_user_id) and runs
// the §4 cleanup row-by-row in idempotent fashion. State changes are
// silent — no events are emitted; the leaver's single MEMBER_LEFT event
// is the user-visible signal for the entire cascade. Per architecture
// §1, none of these call into other services; they live in the storage
// layer so the community service does not depend on the request or
// transfer services.

// leaverItem is what the unshare cascade needs from a stored item: an ID to
// match join rows against.
type leaverItem interface {
	proto.Message
	GetId() string
}

// leaverJoin is what it needs from a community join row: an ID for logging and
// a soft-delete marker to read. Writing the marker goes through the spec, since
// generated protos expose fields rather than setters.
type leaverJoin interface {
	proto.Message
	GetId() string
	GetDeleted() *models.DeletedMetadata
}

// unshareLeaverSpec is the entity-specific half of unsharing a leaver's items
// from a community: prototypes for the two stored types, the field the leaver
// hangs off the item, and how to read the item ID off a join row and stamp the
// join row deleted. The two table names carry into log keys and error text.
type unshareLeaverSpec[T leaverItem, J leaverJoin] struct {
	item       T
	join       J
	ownerField string
	itemTable  string
	joinTable  string
	itemID     func(J) string
	setDeleted func(J, *models.DeletedMetadata)
}

// unshareLeaverItems soft-deletes every join row in the community whose backing
// item belongs to leaverUserID. The item row itself is left untouched — the
// leaver keeps their gear and their requests, which may be shared into other
// communities. Already soft-deleted join rows are skipped, which is what makes
// this idempotent.
//
// The gear and request cascades were 47 duplicated lines before this was
// extracted (#2816).
func unshareLeaverItems[T leaverItem, J leaverJoin](
	ctx context.Context, storage *ProtoSQLStorage,
	operation, communityID, leaverUserID string,
	deletedMetadata *models.DeletedMetadata,
	spec unshareLeaverSpec[T, J],
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", operation,
		"community_id", communityID,
		"user_id", leaverUserID,
	)

	itemRows, err := storage.QueryByField(ctx, spec.ownerField, leaverUserID, spec.item, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query "+spec.itemTable+" by owner", "error", err)
		return fmt.Errorf("failed to query %s by %s: %w", spec.itemTable, spec.ownerField, err)
	}
	if len(itemRows) == 0 {
		return nil
	}
	leaverItemIDs := make(map[string]struct{}, len(itemRows))
	for _, m := range itemRows {
		leaverItemIDs[m.(T).GetId()] = struct{}{}
	}

	joinRows, err := storage.QueryByField(ctx, "community_id", communityID, spec.join, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query "+spec.joinTable, "error", err)
		return fmt.Errorf("failed to query %s: %w", spec.joinTable, err)
	}

	joinIDKey := spec.joinTable + "_id"
	for _, m := range joinRows {
		join := m.(J)
		if _, owned := leaverItemIDs[spec.itemID(join)]; !owned {
			continue
		}
		if del := join.GetDeleted(); del != nil && del.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted "+spec.joinTable, joinIDKey, join.GetId())
			continue
		}
		spec.setDeleted(join, deletedMetadata)
		if err := storage.Update(ctx, join); err != nil {
			logger.ErrorContext(ctx, "failed to soft-delete "+spec.joinTable, joinIDKey, join.GetId(), "error", err)
			return fmt.Errorf("failed to soft-delete %s: %w", spec.joinTable, err)
		}
		logger.DebugContext(ctx, "soft-deleted leaver's "+spec.joinTable,
			joinIDKey, join.GetId(), spec.itemTable+"_id", spec.itemID(join))
	}
	return nil
}

// CascadeDeleteCommunityGearByOwnerInCommunity soft-deletes every
// CommunityGear join row in the community whose backing Gear is owned
// by leaverUserID. The Gear row itself is left untouched — the leaver
// keeps ownership of their gear after departing the community. Already
// soft-deleted join rows are skipped.
func CascadeDeleteCommunityGearByOwnerInCommunity(
	ctx context.Context, storage *ProtoSQLStorage,
	communityID, leaverUserID string, deletedMetadata *models.DeletedMetadata,
) error {
	return unshareLeaverItems(ctx, storage,
		"CascadeDeleteCommunityGearByOwnerInCommunity", communityID, leaverUserID, deletedMetadata,
		unshareLeaverSpec[*models.Gear, *models.CommunityGear]{
			item:       &models.Gear{},
			join:       &models.CommunityGear{},
			ownerField: "owner_id",
			itemTable:  "gear",
			joinTable:  "community_gear",
			itemID:     (*models.CommunityGear).GetGearId,
			setDeleted: func(cg *models.CommunityGear, d *models.DeletedMetadata) { cg.Deleted = d },
		})
}

// CancelLeaverCommunityRequestsInCommunity soft-deletes every
// CommunityRequest join row in the community whose backing Request was
// authored by leaverUserID. The Request row itself is left untouched —
// the leaver keeps the request and may have shared it into other
// communities. Already soft-deleted join rows are skipped.
func CancelLeaverCommunityRequestsInCommunity(
	ctx context.Context, storage *ProtoSQLStorage,
	communityID, leaverUserID string, deletedMetadata *models.DeletedMetadata,
) error {
	return unshareLeaverItems(ctx, storage,
		"CancelLeaverCommunityRequestsInCommunity", communityID, leaverUserID, deletedMetadata,
		unshareLeaverSpec[*models.Request, *models.CommunityRequest]{
			item:       &models.Request{},
			join:       &models.CommunityRequest{},
			ownerField: "requester_id",
			itemTable:  "request",
			joinTable:  "community_request",
			itemID:     (*models.CommunityRequest).GetRequestId,
			setDeleted: func(cr *models.CommunityRequest, d *models.DeletedMetadata) { cr.Deleted = d },
		})
}

// CancelLeaverActiveTransfersInCommunity silently transitions every
// active Transfer in the community where the leaver is sender or
// recipient to TRANSFER_STATE_CANCELLED. Active means state in
// (INTEREST_EXPRESSED, RECIPIENT_SELECTED, ACTIVE). Completed,
// cancelled, and unspecified transfers are left untouched. No
// TRANSFER_CANCELLED event is emitted; the leaver's MEMBER_LEFT event
// is the user-visible signal.
func CancelLeaverActiveTransfersInCommunity(
	ctx context.Context, storage *ProtoSQLStorage,
	communityID, leaverUserID string,
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CancelLeaverActiveTransfersInCommunity",
		"community_id", communityID,
		"user_id", leaverUserID,
	)

	rows, err := storage.QueryByField(ctx, "community_id", communityID, &models.Transfer{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query transfers by community", "error", err)
		return fmt.Errorf("failed to query transfers: %w", err)
	}

	for _, m := range rows {
		t := m.(*models.Transfer)
		if t.OwnerId != leaverUserID && t.RecipientId != leaverUserID {
			continue
		}
		if !isActiveTransferState(t.State) {
			continue
		}
		t.State = models.TransferState_TRANSFER_STATE_CANCELLED
		if err := storage.Update(ctx, t); err != nil {
			logger.ErrorContext(ctx, "failed to cancel transfer", "transfer_id", t.Id, "error", err)
			return fmt.Errorf("failed to cancel transfer: %w", err)
		}
		logger.DebugContext(ctx, "cancelled leaver's active transfer", "transfer_id", t.Id)
	}
	return nil
}

// isActiveTransferState returns true for transfer states that should
// be cancelled when a participant leaves the community.
func isActiveTransferState(state models.TransferState) bool {
	switch state {
	case models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
		models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		models.TransferState_TRANSFER_STATE_ACTIVE:
		return true
	default:
		return false
	}
}

// DropLeaverExperienceRSVPsInCommunity soft-deletes every
// ExperienceRSVP authored by the leaver in this community. Already
// soft-deleted RSVPs are skipped. The ExperienceRSVP row carries
// community_id directly, so a single QueryByFields finds the target
// set.
func DropLeaverExperienceRSVPsInCommunity(
	ctx context.Context, storage *ProtoSQLStorage,
	communityID, leaverUserID string, deletedMetadata *models.DeletedMetadata,
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "DropLeaverExperienceRSVPsInCommunity",
		"community_id", communityID,
		"user_id", leaverUserID,
	)

	rows, err := storage.QueryByFields(ctx, map[string]any{
		"community_id": communityID,
		"user_id":      leaverUserID,
	}, &models.ExperienceRSVP{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query experience_rsvp", "error", err)
		return fmt.Errorf("failed to query experience_rsvp: %w", err)
	}

	for _, m := range rows {
		rsvp := m.(*models.ExperienceRSVP)
		if rsvp.Deleted != nil && rsvp.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted experience_rsvp", "experience_rsvp_id", rsvp.Id)
			continue
		}
		rsvp.Deleted = deletedMetadata
		if err := storage.Update(ctx, rsvp); err != nil {
			logger.ErrorContext(ctx, "failed to soft-delete experience_rsvp", "experience_rsvp_id", rsvp.Id, "error", err)
			return fmt.Errorf("failed to soft-delete experience_rsvp: %w", err)
		}
		logger.DebugContext(ctx, "soft-deleted leaver's experience_rsvp", "experience_rsvp_id", rsvp.Id)
	}
	return nil
}

// DeleteLeaverCommunityNotificationPreferences soft-deletes the
// leaver's CommunityNotificationPreferences row in the community.
// The row sticks around in soft-deleted state so the rejoin path
// (#1657) can un-soft-delete it and preserve the user's prior
// preferences across a leave/rejoin cycle.
func DeleteLeaverCommunityNotificationPreferences(
	ctx context.Context, storage *ProtoSQLStorage,
	communityID, leaverUserID string, deletedMetadata *models.DeletedMetadata,
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "DeleteLeaverCommunityNotificationPreferences",
		"community_id", communityID,
		"user_id", leaverUserID,
	)

	rows, err := storage.QueryByFields(ctx, map[string]any{
		"community_id": communityID,
		"user_id":      leaverUserID,
	}, &models.CommunityNotificationPreferences{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community_notification_preferences", "error", err)
		return fmt.Errorf("failed to query community_notification_preferences: %w", err)
	}

	for _, m := range rows {
		prefs := m.(*models.CommunityNotificationPreferences)
		if prefs.Deleted != nil && prefs.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted community_notification_preferences", "id", prefs.Id)
			continue
		}
		prefs.Deleted = deletedMetadata
		if err := storage.Update(ctx, prefs); err != nil {
			logger.ErrorContext(ctx, "failed to soft-delete community_notification_preferences", "id", prefs.Id, "error", err)
			return fmt.Errorf("failed to soft-delete community_notification_preferences: %w", err)
		}
		logger.DebugContext(ctx, "soft-deleted leaver's community_notification_preferences", "id", prefs.Id)
	}
	return nil
}

// RevokeLeaverCommunityInvitationLinks soft-deletes every
// CommunityInvitationLink AND ShareLink in the community that was
// created by the leaver. The is_revoked flag is independent and is
// left as-is — soft-delete is the cascade marker, is_revoked is the
// user-facing kill switch.
//
// TODO(#2056): When the legacy community_invitation_link table is
// dropped, remove the first scan and keep only the share_link scan.
func RevokeLeaverCommunityInvitationLinks(
	ctx context.Context, storage *ProtoSQLStorage,
	communityID, leaverUserID string, deletedMetadata *models.DeletedMetadata,
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RevokeLeaverCommunityInvitationLinks",
		"community_id", communityID,
		"user_id", leaverUserID,
	)

	legacyRows, err := storage.QueryByFields(ctx, map[string]any{
		"community_id": communityID,
		"inviter_id":   leaverUserID,
	}, &models.CommunityInvitationLink{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community_invitation_link", "error", err)
		return fmt.Errorf("failed to query community_invitation_link: %w", err)
	}

	for _, m := range legacyRows {
		link := m.(*models.CommunityInvitationLink)
		if link.Deleted != nil && link.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted community_invitation_link", "id", link.Id)
			continue
		}
		link.Deleted = deletedMetadata
		if err := storage.Update(ctx, link); err != nil {
			logger.ErrorContext(ctx, "failed to soft-delete community_invitation_link", "id", link.Id, "error", err)
			return fmt.Errorf("failed to soft-delete community_invitation_link: %w", err)
		}
		logger.DebugContext(ctx, "soft-deleted leaver's community_invitation_link", "id", link.Id)
	}

	shareRows, err := storage.QueryByFields(ctx, map[string]any{
		"community_id": communityID,
		"inviter_id":   leaverUserID,
	}, &models.ShareLink{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query share_link", "error", err)
		return fmt.Errorf("failed to query share_link: %w", err)
	}

	for _, m := range shareRows {
		link := m.(*models.ShareLink)
		if link.Deleted != nil && link.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted share_link", "id", link.Id)
			continue
		}
		link.Deleted = deletedMetadata
		if err := storage.Update(ctx, link); err != nil {
			logger.ErrorContext(ctx, "failed to soft-delete share_link", "id", link.Id, "error", err)
			return fmt.Errorf("failed to soft-delete share_link: %w", err)
		}
		logger.DebugContext(ctx, "soft-deleted leaver's share_link", "id", link.Id)
	}
	return nil
}

// CascadeRemoveLeaverFromChatConversationsInCommunity removes leaverUserID from
// the ParticipantIds of every ChatConversation whose topic is related to the
// community the leaver is exiting, provided the leaver has no remaining active
// membership in any other community the topic is also shared with.
//
// Four topic kinds are processed:
//   - Gear: conversations whose topic_gear_id belongs to a gear shared with communityID.
//   - Experience: conversations whose topic_experience_id belongs to an experience
//     shared with communityID.
//   - Request: conversations whose topic_request_id belongs to a request shared
//     with communityID.
//   - Community-wide: conversations whose topic_community_id == communityID.
//
// Transfer-topic conversations are intentionally skipped — they must remain
// accessible to both parties until the transfer is resolved, mirroring
// GetConversationForTransfer's participant-based check.
//
// The helper is idempotent: re-running after the leaver is already stripped is a
// no-op. It issues O(1) SQL round-trips regardless of how many conversations or
// communities are involved.
//
// TODO(#2134): This helper fixes the invariant going forward but does not
// retroactively clean up stale ParticipantIds for users who left before this
// code was deployed. Once the read-path fix in requireConversationAccess (#2133)
// is in production, the stale entries are no longer a security issue — they only
// cause ghost rows in the inbox preview. A one-shot backfill job should iterate
// soft-deleted CommunityUser rows and call this helper per (communityID,
// leaverUserID) pair to restore the invariant for historical leavers.
func CascadeRemoveLeaverFromChatConversationsInCommunity(
	ctx context.Context, store *ProtoSQLStorage,
	communityID, leaverUserID string,
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeRemoveLeaverFromChatConversationsInCommunity",
		"community_id", communityID,
		"user_id", leaverUserID,
	)

	// ── Step 1: collect candidate topic IDs in the leaving community ──────────

	cgRows, err := store.QueryByField(ctx, "community_id", communityID, &models.CommunityGear{})
	if err != nil {
		return fmt.Errorf("query community_gear: %w", err)
	}
	ceRows, err := store.QueryByField(ctx, "community_id", communityID, &models.CommunityExperience{})
	if err != nil {
		return fmt.Errorf("query community_experience: %w", err)
	}
	crRows, err := store.QueryByField(ctx, "community_id", communityID, &models.CommunityRequest{})
	if err != nil {
		return fmt.Errorf("query community_request: %w", err)
	}

	gearIDs := make([]string, 0, len(cgRows))
	for _, m := range cgRows {
		gearIDs = append(gearIDs, m.(*models.CommunityGear).GearId)
	}
	expIDs := make([]string, 0, len(ceRows))
	for _, m := range ceRows {
		expIDs = append(expIDs, m.(*models.CommunityExperience).ExperienceId)
	}
	reqIDs := make([]string, 0, len(crRows))
	for _, m := range crRows {
		reqIDs = append(reqIDs, m.(*models.CommunityRequest).RequestId)
	}

	// ── Step 2: fetch candidate conversations by topic kind ───────────────────

	// Community-wide conversations are tied to exactly this community.
	commConvRows, err := store.QueryByField(ctx, "topic_community_id", communityID, &models.ChatConversation{})
	if err != nil {
		return fmt.Errorf("query community-wide conversations: %w", err)
	}

	// Gear/experience/request conversations: QueryByFieldIn returns nil on empty input.
	gearConvRows, err := store.QueryByFieldIn(ctx, "topic_gear_id", gearIDs, &models.ChatConversation{})
	if err != nil {
		return fmt.Errorf("query gear conversations: %w", err)
	}
	expConvRows, err := store.QueryByFieldIn(ctx, "topic_experience_id", expIDs, &models.ChatConversation{})
	if err != nil {
		return fmt.Errorf("query experience conversations: %w", err)
	}
	reqConvRows, err := store.QueryByFieldIn(ctx, "topic_request_id", reqIDs, &models.ChatConversation{})
	if err != nil {
		return fmt.Errorf("query request conversations: %w", err)
	}

	// ── Step 3: fetch full fan-out to find OTHER communities for each topic ───
	//
	// For gear/experience/request, a single item can be shared with many
	// communities. We need to know whether the leaver still has active
	// membership in any of those OTHER communities — if so, they keep access.

	allCGRows, err := store.QueryByFieldIn(ctx, "gear_id", gearIDs, &models.CommunityGear{})
	if err != nil {
		return fmt.Errorf("query full gear fan-out: %w", err)
	}
	allCERows, err := store.QueryByFieldIn(ctx, "experience_id", expIDs, &models.CommunityExperience{})
	if err != nil {
		return fmt.Errorf("query full experience fan-out: %w", err)
	}
	allCRRows, err := store.QueryByFieldIn(ctx, "request_id", reqIDs, &models.CommunityRequest{})
	if err != nil {
		return fmt.Errorf("query full request fan-out: %w", err)
	}

	// Build per-item community fan-out maps (excluding the leaving community).
	gearFanout := make(map[string][]string) // gearID → []communityID
	for _, m := range allCGRows {
		cg := m.(*models.CommunityGear)
		if cg.CommunityId == communityID {
			continue
		}
		gearFanout[cg.GearId] = append(gearFanout[cg.GearId], cg.CommunityId)
	}
	expFanout := make(map[string][]string) // expID → []communityID
	for _, m := range allCERows {
		ce := m.(*models.CommunityExperience)
		if ce.CommunityId == communityID {
			continue
		}
		expFanout[ce.ExperienceId] = append(expFanout[ce.ExperienceId], ce.CommunityId)
	}
	reqFanout := make(map[string][]string) // reqID → []communityID
	for _, m := range allCRRows {
		cr := m.(*models.CommunityRequest)
		if cr.CommunityId == communityID {
			continue
		}
		reqFanout[cr.RequestId] = append(reqFanout[cr.RequestId], cr.CommunityId)
	}

	// ── Step 4: single batched membership probe across all other communities ──

	otherCommunitySet := make(map[string]struct{})
	for _, communities := range gearFanout {
		for _, cid := range communities {
			otherCommunitySet[cid] = struct{}{}
		}
	}
	for _, communities := range expFanout {
		for _, cid := range communities {
			otherCommunitySet[cid] = struct{}{}
		}
	}
	for _, communities := range reqFanout {
		for _, cid := range communities {
			otherCommunitySet[cid] = struct{}{}
		}
	}
	otherCommunityIDs := make([]string, 0, len(otherCommunitySet))
	for cid := range otherCommunitySet {
		otherCommunityIDs = append(otherCommunityIDs, cid)
	}

	pairs, err := store.GetCommunitiesWithMembership(ctx, otherCommunityIDs, leaverUserID)
	if err != nil {
		return fmt.Errorf("batch membership probe: %w", err)
	}

	// hasActiveMembershipIn reports whether the leaver has an active (non-soft-deleted)
	// CommunityUser row in any of the provided communityIDs.
	hasActiveMembershipIn := func(communityIDs []string) bool {
		for _, cid := range communityIDs {
			pair, ok := pairs[cid]
			if !ok {
				continue
			}
			if pair.Community.Deleted != nil && pair.Community.Deleted.DeletedAtUnixSec > 0 {
				continue
			}
			if pair.Membership == nil {
				continue
			}
			if pair.Membership.Deleted != nil && pair.Membership.Deleted.DeletedAtUnixSec > 0 {
				continue
			}
			return true
		}
		return false
	}

	// ── Step 5: strip leaver from each candidate conversation if warranted ────

	type candidate struct {
		conv     *models.ChatConversation
		otherIDs []string // other communities sharing this conversation's topic
	}
	var candidates []candidate

	for _, m := range commConvRows {
		// Community-wide conversations: no other shared communities.
		candidates = append(candidates, candidate{conv: m.(*models.ChatConversation)})
	}
	for _, m := range gearConvRows {
		conv := m.(*models.ChatConversation)
		candidates = append(candidates, candidate{conv: conv, otherIDs: gearFanout[conv.GetTopic().GetGearId()]})
	}
	for _, m := range expConvRows {
		conv := m.(*models.ChatConversation)
		candidates = append(candidates, candidate{conv: conv, otherIDs: expFanout[conv.GetTopic().GetExperienceId()]})
	}
	for _, m := range reqConvRows {
		conv := m.(*models.ChatConversation)
		candidates = append(candidates, candidate{conv: conv, otherIDs: reqFanout[conv.GetTopic().GetRequestId()]})
	}

	for _, c := range candidates {
		conv := c.conv

		// Skip if leaver is not in the participant list.
		leaverIdx := slices.Index(conv.ParticipantIds, leaverUserID)
		if leaverIdx < 0 {
			continue
		}

		// Skip if the leaver still has active membership in another shared community.
		if hasActiveMembershipIn(c.otherIDs) {
			continue
		}

		// Splice leaver out of the participant list.
		before := len(conv.ParticipantIds)
		conv.ParticipantIds = slices.Delete(conv.ParticipantIds, leaverIdx, leaverIdx+1)
		if err := store.Update(ctx, conv); err != nil {
			return fmt.Errorf("update conversation %s: %w", conv.Id, err)
		}

		topicKindStr := "community"
		if topic := conv.GetTopic(); topic != nil {
			switch {
			case topic.GetGearId() != "":
				topicKindStr = "gear"
			case topic.GetExperienceId() != "":
				topicKindStr = "experience"
			case topic.GetRequestId() != "":
				topicKindStr = "request"
			}
		}
		logger.DebugContext(
			ctx, "removed leaver from conversation participants",
			"conversation_id", conv.Id,
			"topic_kind", topicKindStr,
			"participant_count_before", before,
			"participant_count_after", len(conv.ParticipantIds),
		)
	}

	return nil
}
