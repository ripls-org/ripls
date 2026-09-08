package gear

import (
	"context"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// DeleteGear soft-deletes gear from the database.
// Any active or pending transfers for this gear are cancelled.
func (s *Service) DeleteGear(
	ctx context.Context,
	req *connect.Request[api.DeleteGearRequest],
) (*connect.Response[api.DeleteGearResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "DeleteGear",
		"user_id", authInfo.UserID,
		"gear_id", req.Msg.GearId,
	)
	logger.Info("deleting gear")

	// Fetch the gear via the soft-delete-aware helper so a delete against an
	// already soft-deleted row (stale detail screen, multi-device race,
	// batch-selection includes item already deleted elsewhere) logs WARN
	// instead of ERROR (#2711). Hard misses still log ERROR.
	gearStored, err := s.fetchGearForModify(ctx, req.Msg.GearId, logger.Logger, "DeleteGear")
	if err != nil {
		return nil, err
	}

	// Verify ownership
	if gearStored.OwnerId != authInfo.UserID {
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "gear_owner_required_for_delete", "only the owner can delete this gear", nil)
	}

	// Cancel any active or pending transfers for this gear
	transfersProto, err := s.storage.QueryByField(ctx, "gear_id", req.Msg.GearId, &models.Transfer{})
	if err != nil {
		logger.Error("failed to query transfers for gear", "error", err)
		return nil, connecterr.Internal(ctx, "DeleteGear", err)
	}

	for _, msg := range transfersProto {
		transfer := msg.(*models.Transfer)
		// Cancel transfers that are not already in a terminal state
		if transfer.State != models.TransferState_TRANSFER_STATE_COMPLETED &&
			transfer.State != models.TransferState_TRANSFER_STATE_CANCELLED {
			transfer.State = models.TransferState_TRANSFER_STATE_CANCELLED
			if err := s.storage.Update(ctx, transfer); err != nil {
				logger.Error("failed to cancel transfer", "transfer_id", transfer.Id, "error", err)
				return nil, connecterr.Internal(ctx, "DeleteGear", err)
			}
			logger.Info("cancelled transfer due to gear deletion", "transfer_id", transfer.Id)
		}
	}

	// Set the deleted metadata (soft delete)
	deletedMetadata := &models.DeletedMetadata{
		DeletedByUserId:  authInfo.UserID,
		DeletedAtUnixSec: time.Now().Unix(),
	}
	gearStored.Deleted = deletedMetadata

	err = s.storage.Update(ctx, gearStored)
	if err != nil {
		logger.Error("failed to soft-delete gear", "error", err)
		return nil, connecterr.Internal(ctx, "DeleteGear", err)
	}

	// Cascade deletion to all conversations linked to this gear
	if err := storage.CascadeDeleteConversationsByTopic(ctx, s.storage, "topic_gear_id", req.Msg.GearId, deletedMetadata); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteGear", err)
	}

	// Cascade deletion to community_gear join rows so archived and live shares
	// stop surfacing the deleted gear.
	if err := storage.CascadeDeleteCommunityGearByGearID(ctx, s.storage, req.Msg.GearId, deletedMetadata); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteGear", err)
	}

	// Cascade deletion to share links that point at this gear.
	if err := storage.CascadeDeleteShareLinksByGearID(ctx, s.storage, req.Msg.GearId, deletedMetadata); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteGear", err)
	}

	// Cascade deletion to associated media
	if err := storage.CascadeDeleteMedia(ctx, s.storage, gearStored.MediaIds, deletedMetadata); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteGear", err, "detail", "failed to cascade delete media")
	}

	logger.Info("gear deleted successfully")
	return connect.NewResponse(&api.DeleteGearResponse{}), nil
}

// convertGearState converts from storage model GearState to API GearItemState.
func convertGearState(state models.GearState) api.GearItemState {
	switch state {
	case models.GearState_GEAR_STATE_AVAILABLE:
		return api.GearItemState_GEAR_ITEM_STATE_AVAILABLE
	case models.GearState_GEAR_STATE_UNAVAILABLE:
		return api.GearItemState_GEAR_ITEM_STATE_UNAVAILABLE
	case models.GearState_GEAR_STATE_GIVEN_AWAY:
		return api.GearItemState_GEAR_ITEM_STATE_GIVEN_AWAY
	default:
		return api.GearItemState_GEAR_ITEM_STATE_UNSPECIFIED
	}
}

// GetTransferRequestCount retrieves the count of transfer requests for a gear item.
func (s *Service) GetTransferRequestCount(
	ctx context.Context,
	req *connect.Request[api.GetTransferRequestCountRequest],
) (*connect.Response[api.GetTransferRequestCountResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"gear_id", req.Msg.GearId,
	)
	logger.Debug("requesting transfer count for gear")

	// Fetch the gear to verify it exists and get owner
	gearStored := &models.Gear{}
	err = s.storage.GetByID(ctx, req.Msg.GearId, gearStored)
	if err != nil {
		logger.Error("failed to get gear", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	// Caller must be an active member of at least one community the gear is
	// (or was) shared with, or be the owner — read gate, see #2695.
	if _, _, err := auth.RequireReadAccessToCommunityScopedEntity(
		ctx, s.storage, authInfo.UserID,
		auth.EntityGear, gearStored.Id, gearStored.OwnerId,
	); err != nil {
		return nil, err
	}

	response := &api.GetTransferRequestCountResponse{
		ReceivedCount:        0,
		HasActiveRequest:     false,
		ActiveConversationId: "",
	}

	// Check if user is the owner
	isOwner := gearStored.OwnerId == authInfo.UserID

	if isOwner {
		// For owners: count all non-archived requests across all communities
		queryFields := map[string]any{
			"gear_id":  req.Msg.GearId,
			"owner_id": authInfo.UserID,
		}

		// Query all transfers for this gear owned by current user
		transfersProto, err := s.storage.QueryByFields(ctx, queryFields, &models.Transfer{})
		if err != nil {
			logger.Error("failed to query transfers", "error", err)
			return nil, connecterr.Internal(ctx, "GetTransferRequestCount", err)
		}

		// Count non-terminal transfers
		count := int32(0)
		for _, msg := range transfersProto {
			transfer := msg.(*models.Transfer)
			if !isTerminalTransferState(transfer.State) {
				count++
			}
		}

		response.ReceivedCount = count
		logger.Debug("owner transfer request count", "received_count", count)
	} else {
		// For borrowers: check if they have an active request
		queryFields := map[string]any{
			"gear_id":      req.Msg.GearId,
			"recipient_id": authInfo.UserID,
		}

		transfersProto, err := s.storage.QueryByFields(ctx, queryFields, &models.Transfer{})
		if err != nil {
			logger.Error("failed to query transfers", "error", err)
			return nil, connecterr.Internal(ctx, "GetTransferRequestCount", err)
		}

		// Check for non-terminal transfer
		for _, msg := range transfersProto {
			transfer := msg.(*models.Transfer)
			if !isTerminalTransferState(transfer.State) {
				response.HasActiveRequest = true

				// Get conversation ID for this transfer
				conversations, err := s.storage.QueryByField(ctx, "topic_transfer_id", transfer.Id, &models.ChatConversation{})
				if err == nil && len(conversations) > 0 {
					conversation := conversations[0].(*models.ChatConversation)
					response.ActiveConversationId = conversation.Id

					// For giveaways, populate participant count
					if transfer.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
						response.ParticipantCount = int32(len(conversation.ParticipantIds))
					}
				}

				logger.Debug(
					"borrower has active transfer request",
					"conversation_id", response.ActiveConversationId,
					"participant_count", response.ParticipantCount,
				)
				break
			}
		}

		// Even if borrower doesn't have an active request, check for giveaway participant count
		// This allows new users to see how many people have already claimed a giveaway
		if !response.HasActiveRequest {
			// Query for ANY active giveaway transfer for this gear
			allTransfersProto, err := s.storage.QueryByField(ctx, "gear_id", req.Msg.GearId, &models.Transfer{})
			if err == nil {
				for _, msg := range allTransfersProto {
					transfer := msg.(*models.Transfer)
					// Look for non-terminal giveaway transfers
					if !isTerminalTransferState(transfer.State) &&
						transfer.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
						// Get participant count from the giveaway conversation
						conversations, err := s.storage.QueryByField(ctx, "topic_transfer_id", transfer.Id, &models.ChatConversation{})
						if err == nil && len(conversations) > 0 {
							conversation := conversations[0].(*models.ChatConversation)
							response.ParticipantCount = int32(len(conversation.ParticipantIds))
							logger.Debug("found giveaway with participants", "participant_count", response.ParticipantCount)
						}
						break
					}
				}
			}
		}
	}

	res := connect.NewResponse(response)
	return res, nil
}
