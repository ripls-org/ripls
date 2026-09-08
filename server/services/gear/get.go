package gear

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/conversation"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// GetGear retrieves gear from the database by its ID.
// If community_id is provided, also returns community-specific fields (conversation_id, active_loan, availability).
func (s *Service) GetGear(
	ctx context.Context, req *connect.Request[api.GetGearRequest],
) (*connect.Response[api.GetGearResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetGear",
		"user_id", authInfo.UserID,
		"gear_id", req.Msg.Id,
		"community_id", req.Msg.CommunityId,
	)
	logger.Debug("requesting gear")

	// Include soft-deleted gear so transfer-bound surfaces (non-owners with
	// historical or pending Transfer rows pointing at this gear) can render a
	// tombstone instead of triggering ERROR alerts on the standard
	// not-found path. See #1701 — DeleteGear deliberately leaves Transfer
	// rows alive as audit trail; readers degrade gracefully.
	gearStored := &models.Gear{}
	err = s.storage.GetByID(ctx, req.Msg.Id, gearStored, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			logger.ErrorContext(ctx, "failed to get gear", "error", err)
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		logger.ErrorContext(ctx, "failed to get gear", "error", err)
		return nil, connecterr.Internal(ctx, "GetGear", err)
	}

	if gearStored.Deleted != nil {
		return s.buildDeletedGearResponse(ctx, logger, gearStored)
	}

	// Caller must be an active member of at least one community the gear is
	// (or was) shared with, or be the owner. The read gate counts archived
	// shares so completed giveaways stay visible to the communities they
	// happened in — including the recipient (#2695).
	sharedCommunityIDs, callerCommunityIDs, err := auth.RequireReadAccessToCommunityScopedEntity(
		ctx, s.storage, authInfo.UserID,
		auth.EntityGear, gearStored.Id, gearStored.OwnerId,
	)
	if err != nil {
		return nil, err
	}

	// Fetch owner details from user storage
	owner, err := services.FetchAPIUser(ctx, s.storage, gearStored.OwnerId)
	if err != nil {
		logger.Error("failed to get owner for gear", "owner_id", gearStored.OwnerId, "error", err)
		return nil, connecterr.Internal(ctx, "GetGear", fmt.Errorf("failed to load owner information"))
	}

	response := &api.GetGearResponse{
		Id:            gearStored.Id,
		Name:          gearStored.Name,
		Description:   gearStored.Description,
		Owner:         owner,
		MediaIds:      gearStored.MediaIds,
		State:         convertGearState(gearStored.State),
		LocationId:    gearStored.LocationId,
		ValueEstimate: convertStorageValueEstimateToAPI(gearStored.ValueEstimate),
		Metadata:      convertStorageGearMetadataToAPI(gearStored),
		SourceUrl:     gearStored.SourceUrl,
		// Seed with the gear's own created_at — populateGearSharedCommunities
		// overrides this with the earliest CommunityGear.created_at when the
		// gear is shared anywhere. The fallback covers gear that has never
		// been shared and pre-2026-03-29 gear that predates the field.
		CreatedAtUnixSec: gearStored.CreatedAtUnixSec,
	}

	// If community_id provided, validate it's active and populate
	// community-specific fields. Reject if missing or soft-deleted.
	if req.Msg.CommunityId != "" {
		if _, err := auth.RequireActiveCommunity(ctx, s.storage, req.Msg.CommunityId); err != nil {
			return nil, err
		}
		if err := s.populateCommunityFields(ctx, req.Msg.Id, req.Msg.CommunityId, authInfo.UserID, response); err != nil {
			logger.ErrorContext(ctx, "failed to populate community fields", "error", err)
			return nil, connecterr.Internal(ctx, "GetGear", err, "detail", "failed to get community context")
		}
	}

	// Populate shared communities using the full set for counts and the
	// caller-scoped subset for the SharedCommunities list.
	if err := s.populateGearSharedCommunities(ctx, req.Msg.Id, sharedCommunityIDs, callerCommunityIDs, response); err != nil {
		logger.WarnContext(ctx, "failed to populate shared communities", "error", err)
	}

	// Populate times_loaned for the "View Impact" guard on the loan menu.
	timesLoaned, err := s.countTimesLoaned(ctx, req.Msg.Id)
	if err != nil {
		logger.WarnContext(ctx, "failed to count times loaned", "error", err)
	} else {
		response.TimesLoaned = &timesLoaned
	}

	res := connect.NewResponse(response)
	return res, nil
}

// buildDeletedGearResponse returns a tombstone GetGearResponse for a
// soft-deleted gear. The response is intentionally narrow: id, name,
// description, media_ids, owner (resolved leniently for also-deleted
// owners) and deleted=true. Community-side data is cascade-deleted with
// the gear, so populateCommunityFields and populateGearSharedCommunities
// are skipped. The standard access check is also skipped — non-owner
// participants with dangling Transfer rows are the entire point of this
// branch, and the response leaks only last-known display data the caller
// previously had access to.
func (s *Service) buildDeletedGearResponse(
	ctx context.Context,
	logger *logging.Logger,
	gearStored *models.Gear,
) (*connect.Response[api.GetGearResponse], error) {
	logger.WarnContext(
		ctx, "returning tombstone for soft-deleted gear",
		"reason", "soft_deleted",
		"deleted_at_unix_sec", gearStored.Deleted.DeletedAtUnixSec,
		"deleted_by_user_id", gearStored.Deleted.DeletedByUserId,
	)

	// Resolve the owner leniently. A deleted gear may belong to a
	// soft-deleted user; fall back to a former-member placeholder so the
	// response shape is always populated.
	owner, err := services.FetchAPIUser(ctx, s.storage, gearStored.OwnerId)
	if err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			owner = services.FormerMemberPlaceholder(gearStored.OwnerId)
		} else {
			logger.ErrorContext(
				ctx, "failed to load owner for deleted gear",
				"owner_id", gearStored.OwnerId,
				"error", err,
			)
			return nil, connecterr.Internal(ctx, "GetGear", fmt.Errorf("failed to load owner information"))
		}
	}

	response := &api.GetGearResponse{
		Id:               gearStored.Id,
		Name:             gearStored.Name,
		Description:      gearStored.Description,
		Owner:            owner,
		MediaIds:         gearStored.MediaIds,
		State:            convertGearState(gearStored.State),
		LocationId:       gearStored.LocationId,
		CreatedAtUnixSec: gearStored.CreatedAtUnixSec,
		Deleted:          proto.Bool(true),
	}
	return connect.NewResponse(response), nil
}

// populateCommunityFields populates community-specific fields in GetGearResponse.
// This includes conversation_id, active_loan, availability, and comment preview.
func (s *Service) populateCommunityFields(
	ctx context.Context,
	gearID string,
	communityID string,
	currentUserID string,
	response *api.GetGearResponse,
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "populateCommunityFields",
		"gear_id", gearID,
		"community_id", communityID,
	)

	// Fetch the gear to get its canonical conversation_id.
	// conversation_id lives on Gear (not on CommunityGear), so populate it
	// first — even if the gear is not currently shared with this community
	// (e.g. the user still has an active transfer after the listing was removed).
	gearStored := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gearStored); err != nil {
		return fmt.Errorf("failed to fetch gear for community fields: %w", err)
	}
	response.ConversationId = gearStored.ConversationId

	// Get CommunityGear record for community-specific fields.
	communityGears, err := s.storage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"community_id": communityID,
	}, &models.CommunityGear{})
	if err != nil {
		return fmt.Errorf("failed to query community gear: %w", err)
	}

	var communityGear *models.CommunityGear
	if len(communityGears) > 0 {
		communityGear = communityGears[0].(*models.CommunityGear)

		// Populate community-specific fields.
		response.Availability = convertAvailability(communityGear.Availability)
		response.SharedAtUnixSec = communityGear.CreatedAtUnixSec

		if communityGear.Availability == models.Availability_AVAILABILITY_FOR_LOAN ||
			communityGear.Availability == models.Availability_AVAILABILITY_FOR_GIVEAWAY {
			activeLoan, err := s.getActiveLoanForGear(ctx, gearID, communityID)
			if err != nil {
				logger.ErrorContext(ctx, "failed to get active loan", "error", err)
				return fmt.Errorf("failed to get active loan: %w", err)
			}
			response.ActiveLoan = activeLoan
		}
	} else {
		logger.DebugContext(ctx, "gear not shared with this community")
	}

	// Populate message count and comment preview if conversation exists.
	// Always done regardless of CommunityGear presence — the conversation belongs to the gear.
	if response.ConversationId != "" {
		messageCount, err := s.getMessageCount(ctx, response.ConversationId)
		if err != nil {
			logger.ErrorContext(ctx, "failed to get message count", "conversation_id", response.ConversationId, "error", err)
			return fmt.Errorf("failed to get message count: %w", err)
		}
		response.MessageCount = messageCount

		preview, err := conversation.EnrichWithCommentPreview(ctx, s.storage, response.ConversationId, currentUserID)
		if err != nil {
			logger.Warn("failed to enrich comment preview", "conversation_id", response.ConversationId, "error", err)
		} else {
			applyCommentPreviewToGearResponse(response, preview)
		}
	}

	logger.DebugContext(
		ctx, "populated community fields",
		"conversation_id", response.ConversationId,
		"availability", response.Availability,
		"has_active_loan", response.ActiveLoan != nil,
		"message_count", response.MessageCount,
	)

	return nil
}

// getMessageCount retrieves the number of messages in a conversation.
func (s *Service) getMessageCount(ctx context.Context, conversationID string) (int32, error) {
	return chat.GetConversationMessageCount(ctx, s.storage, conversationID)
}

// populateGearSharedCommunities fetches communities the gear is shared with
// and attaches them to the response.
//
// sharedCommunityIDs is the full set of communities the gear is in (all
// communities returned by the access gate). callerCommunityIDs is the
// caller-scoped subset (communities the caller is an active member of).
//
// TotalSharedCommunityCount and TotalDistinctMemberCount are computed from
// the full sharedCommunityIDs set so the client can display the item's true
// reach. SharedCommunities is built from callerCommunityIDs only, avoiding
// disclosure of community names/avatars for communities the caller is not in.
// Owners receive callerCommunityIDs == sharedCommunityIDs and therefore see
// the full SharedCommunities list.
func (s *Service) populateGearSharedCommunities(
	ctx context.Context,
	gearID string,
	sharedCommunityIDs []string,
	callerCommunityIDs []string,
	response *api.GetGearResponse,
) error {
	if len(sharedCommunityIDs) == 0 {
		return nil
	}

	// 1. Batch-query all CommunityGear entries for this gear. Archived rows
	// are kept: a completed giveaway archives every share, but the item's
	// audience must stay visible to those communities (#2695) — the gate
	// already scoped sharedCommunityIDs to what this caller may see.
	cgRaw, err := s.storage.QueryByField(ctx, "gear_id", gearID, &models.CommunityGear{})
	if err != nil {
		return fmt.Errorf("failed to query community gear: %w", err)
	}

	// Build a set of the full shared community IDs for fast lookup.
	sharedSet := make(map[string]struct{}, len(sharedCommunityIDs))
	for _, cid := range sharedCommunityIDs {
		sharedSet[cid] = struct{}{}
	}

	// Filter to entries in the full shared set.
	allFilteredIDs := make([]string, 0, len(sharedCommunityIDs))
	cgByCommunityID := make(map[string]*models.CommunityGear, len(sharedCommunityIDs))
	var earliestSharedAt int64
	for _, m := range cgRaw {
		cg := m.(*models.CommunityGear)
		if _, ok := sharedSet[cg.CommunityId]; !ok {
			continue
		}
		allFilteredIDs = append(allFilteredIDs, cg.CommunityId)
		cgByCommunityID[cg.CommunityId] = cg
		if cg.CreatedAtUnixSec > 0 && (earliestSharedAt == 0 || cg.CreatedAtUnixSec < earliestSharedAt) {
			earliestSharedAt = cg.CreatedAtUnixSec
		}
	}
	if len(allFilteredIDs) == 0 {
		return nil
	}

	// Report the full count of communities the gear is shared with.
	response.TotalSharedCommunityCount = int32(len(allFilteredIDs))

	// CommunityGear.created_at_unix_sec has been reliably set on every share
	// since long before Gear.created_at_unix_sec existed (2026-03-29), so
	// the earliest CommunityGear timestamp is the canonical "first shared"
	// date. Prefer it over the gear-level field, which is 0 for any gear
	// created before that date.
	if earliestSharedAt > 0 {
		response.CreatedAtUnixSec = earliestSharedAt
	}

	// 2. Batch-fetch memberships for ALL communities to compute the true
	// distinct user count across the full sharing scope.
	membershipsRaw, err := s.storage.QueryByFieldIn(ctx, "community_id", allFilteredIDs, &models.CommunityUser{})
	if err != nil {
		return fmt.Errorf("failed to batch-fetch memberships: %w", err)
	}
	memberCounts := make(map[string]int32, len(allFilteredIDs))
	distinctUsers := make(map[string]struct{}, len(membershipsRaw))
	// Member ids per community, used to surface the gear's own ad-hoc origin
	// community's directly-invited individuals (#2492).
	membersByCommunity := make(map[string][]string, len(allFilteredIDs))
	for _, m := range membershipsRaw {
		cu := m.(*models.CommunityUser)
		memberCounts[cu.CommunityId]++
		membersByCommunity[cu.CommunityId] = append(
			membersByCommunity[cu.CommunityId], cu.UserId,
		)
		distinctUsers[cu.UserId] = struct{}{}
	}
	response.TotalDistinctMemberCount = int32(len(distinctUsers))

	// 3. Build SharedCommunities from the caller-scoped subset only, so
	// community names and avatars are not disclosed for communities the
	// caller is not a member of.
	callerSet := make(map[string]struct{}, len(callerCommunityIDs))
	for _, cid := range callerCommunityIDs {
		callerSet[cid] = struct{}{}
	}

	viewerIDs := make([]string, 0, len(callerCommunityIDs))
	for _, cid := range allFilteredIDs {
		if _, ok := callerSet[cid]; ok {
			viewerIDs = append(viewerIDs, cid)
		}
	}
	if len(viewerIDs) == 0 {
		return nil
	}

	communityMap, err := s.storage.GetByIDs(ctx, viewerIDs, &models.Community{})
	if err != nil {
		return fmt.Errorf("failed to batch-fetch communities: %w", err)
	}

	var originCommunityID string
	for _, cid := range viewerIDs {
		cm, ok := communityMap[cid]
		if !ok {
			continue
		}
		community := cm.(*models.Community)
		mediaID := ""
		if len(community.MediaIds) > 0 {
			mediaID = community.MediaIds[0]
		}
		if community.GetOriginGearId() == gearID {
			originCommunityID = cid
		}
		cg := cgByCommunityID[cid]
		response.SharedCommunities = append(response.SharedCommunities, &api.SharedCommunity{
			CommunityId:     cid,
			CommunityName:   community.Name,
			MediaId:         mediaID,
			MemberCount:     memberCounts[cid],
			SharedAtUnixSec: cg.CreatedAtUnixSec,
		})
	}

	// Surface the gear's own ad-hoc origin community's directly-invited
	// individuals (excl. the owner) so the "Shared with" surface can list them
	// by name; named communities stay collapsed to a member count. Gear has no
	// RSVP, so there is nothing to exclude beyond the owner (#2492).
	if originCommunityID != "" {
		ownerID := response.GetOwner().GetId()
		invited, err := services.FetchInvitedIndividuals(
			ctx, s.storage, membersByCommunity[originCommunityID], ownerID, nil,
		)
		if err != nil {
			return fmt.Errorf("failed to fetch invited individuals: %w", err)
		}
		response.InvitedIndividuals = invited
	}
	return nil
}

// applyCommentPreviewToGearResponse maps CommentPreview fields onto GetGearResponse.
func applyCommentPreviewToGearResponse(resp *api.GetGearResponse, p *conversation.CommentPreview) {
	resp.UnreadCount = p.UnreadCount
	resp.LastMessageText = p.LastMessageText
	resp.LastMessageSender = p.LastMessageSender
	resp.LastMessageTimeAgo = p.LastMessageTimeAgo
	resp.RecentCommenters = p.RecentCommenters
}

// getActiveLoanForGear queries for an active transfer or completed giveaway for a gear item in a community.
// Returns nil if no active transfer exists.
// For giveaways, returns completed transfers to show who received the gear.
// For loans, only returns active transfers (not completed ones).
func (s *Service) getActiveLoanForGear(ctx context.Context, gearID, communityID string) (*api.ActiveLoan, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "getActiveLoanForGear",
		"gear_id", gearID,
		"community_id", communityID,
	)

	// Query for active transfers first
	transfers, err := s.storage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"community_id": communityID,
		"state":        models.TransferState_TRANSFER_STATE_ACTIVE,
	}, &models.Transfer{})
	if err != nil {
		return nil, fmt.Errorf("failed to query active transfers: %w", err)
	}

	// If no active transfer, check for completed giveaways (not loans)
	// For giveaways, we want to show who received the item even after completion
	// For loans, we don't show anything after the item is returned
	if len(transfers) == 0 {
		completedTransfers, err := s.storage.QueryByFields(ctx, map[string]any{
			"gear_id":       gearID,
			"community_id":  communityID,
			"state":         models.TransferState_TRANSFER_STATE_COMPLETED,
			"transfer_type": models.TransferType_TRANSFER_TYPE_GIVEAWAY,
		}, &models.Transfer{})
		if err != nil {
			return nil, fmt.Errorf("failed to query completed giveaway transfers: %w", err)
		}
		transfers = completedTransfers
	}

	if len(transfers) == 0 {
		return nil, nil // No active transfer or completed giveaway
	}

	// Should be at most one active loan per gear
	transfer := transfers[0].(*models.Transfer)

	logger.DebugContext(
		ctx, "found transfer",
		"transfer_id", transfer.Id,
		"recipient_id", transfer.RecipientId,
		"state", transfer.State,
		"type", transfer.TransferType,
	)

	// Fetch recipient user details
	recipient, err := services.FetchAPIUser(ctx, s.storage, transfer.RecipientId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get recipient for transfer",
			"recipient_id", transfer.RecipientId,
			"transfer_id", transfer.Id,
			"error", err)
		return nil, err
	}

	// Determine status based on transfer type and state
	var status string
	if transfer.State == models.TransferState_TRANSFER_STATE_COMPLETED {
		if transfer.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
			status = "Given away"
		} else {
			status = "Returned"
		}
	} else {
		// ACTIVE state
		if transfer.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
			status = "Being given away"
		} else {
			status = "On loan"
		}
	}

	activeLoan := &api.ActiveLoan{
		Borrower:   recipient,
		TransferId: transfer.Id,
		Status:     status,
	}

	logger.DebugContext(ctx, "found transfer for gear",
		"transfer_id", transfer.Id,
		"recipient_id", transfer.RecipientId,
		"status", status)

	return activeLoan, nil
}

// convertAvailability converts from storage model Availability to API Availability.
func convertAvailability(availability models.Availability) api.Availability {
	switch availability {
	case models.Availability_AVAILABILITY_FOR_LOAN:
		return api.Availability_AVAILABILITY_FOR_LOAN
	case models.Availability_AVAILABILITY_FOR_GIVEAWAY:
		return api.Availability_AVAILABILITY_FOR_GIVEAWAY
	default:
		return api.Availability_AVAILABILITY_UNSPECIFIED
	}
}
