package transfer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// SelectRecipient allows the owner to select a recipient from interested users.
func (s *Service) SelectRecipient(
	ctx context.Context,
	req *connect.Request[api.SelectRecipientRequest],
) (*connect.Response[api.SelectRecipientResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	// Fetch the transfer
	transferStored := &models.Transfer{}
	err = s.storage.GetByID(ctx, req.Msg.TransferId, transferStored)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to get transfer",
			"transfer_id", req.Msg.TransferId,
			"error", err,
		)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("transfer not found"))
	}

	// Verify actor is the owner
	if transferStored.OwnerId != authInfo.UserID {
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "transfer_owner_required_for_select_recipient", "only the gear owner can select the recipient", nil)
	}

	// Verify transfer is in INTEREST_EXPRESSED state
	if transferStored.State != models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
		return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "transfer_invalid_state_for_select_recipient", "can only select a recipient when the transfer is awaiting interest", nil)
	}

	// Update recipient and state
	transferStored.RecipientId = req.Msg.RecipientId
	transferStored.State = models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED

	err = s.storage.Update(ctx, transferStored)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to update transfer",
			"transfer_id", req.Msg.TransferId,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "SelectRecipient", err)
	}

	// Log appropriate message based on transfer type
	if transferStored.TransferType == models.TransferType_TRANSFER_TYPE_LOAN {
		logger.InfoContext(
			ctx, "approved loan request",
			"transfer_id", req.Msg.TransferId,
			"recipient_id", req.Msg.RecipientId,
		)
	} else {
		logger.InfoContext(
			ctx, "selected recipient for giveaway",
			"transfer_id", req.Msg.TransferId,
			"recipient_id", req.Msg.RecipientId,
		)
	}

	// Get the gear conversation (both loans and giveaways use gear conversations).
	// Looked up before emitting the system message so we can thread the
	// system_chat_message_id onto the CommunityEvent below; that id is what
	// UndoSelectRecipient uses to soft-delete the message on undo.
	conversationID, err := s.getGearConversationID(ctx, transferStored.GearId, transferStored.CommunityId)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to get gear conversation",
			"gear_id", transferStored.GearId,
			"community_id", transferStored.CommunityId,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "SelectRecipient", err, "detail",

			// Insert system message for approval in the gear conversation. Capture
			// the message id so we can persist it on the CommunityEvent.
			"failed to get gear conversation")
	}

	var systemChatMessageID string
	if s.systemMessageWriter != nil {
		recipientName := s.getUserDisplayName(ctx, req.Msg.RecipientId)
		msgID, err := s.systemMessageWriter.InsertLocalizedReturnID(
			ctx,
			conversationID,
			authInfo.UserID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_APPROVED,
			chat.ApprovedMessage(recipientName),
		)
		if err != nil {
			logger.ErrorContext(
				ctx, "failed to insert approval system message",
				"conversation_id", conversationID,
				"error", err,
			)
			return nil, connecterr.Internal(ctx, "SelectRecipient", err, "detail", "failed to insert system message")
		}
		systemChatMessageID = msgID
	}

	// Record community event.
	// Note: ObjectUserId is NOT set. The notification system looks up the recipient
	// from the transfer instead.
	eventID, err := s.recordCommunityEventForTransfer(
		ctx,
		req.Msg.TransferId,
		transferStored.GearId,
		models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		transferStored.TransferType,
		authInfo.UserID,
		"",
		transferStored.CommunityId,
		systemChatMessageID,
		nil, // SelectRecipient has no cascade; no UndoData required.
	)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to record community event for transfer",
			"transfer_id", req.Msg.TransferId,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "SelectRecipient", err, "detail", "failed to record community event")
	}

	return connect.NewResponse(&api.SelectRecipientResponse{
		ConversationId:   conversationID,
		CommunityEventId: eventID,
	}), nil
}

// CancelTransfer cancels a transfer (any state → CANCELLED).
func (s *Service) CancelTransfer(
	ctx context.Context,
	req *connect.Request[api.CancelTransferRequest],
) (*connect.Response[api.CancelTransferResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	// Fetch the transfer to get gear_id and community_id
	transferStored := &models.Transfer{}
	err = s.storage.GetByID(ctx, req.Msg.TransferId, transferStored)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to get transfer",
			"transfer_id", req.Msg.TransferId,
			"error", err,
		)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("transfer not found"))
	}

	// Insert system message before state transition (use gear conversation
	// for both loans and giveaways). Capture the id so UndoCancelTransfer
	// can soft-delete the message by direct read.
	var cancelSystemMessageID string
	if s.systemMessageWriter != nil {
		conversationID, err := s.getGearConversationID(ctx, transferStored.GearId, transferStored.CommunityId)
		if err != nil {
			logger.ErrorContext(
				ctx, "failed to get gear conversation",
				"gear_id", transferStored.GearId,
				"community_id", transferStored.CommunityId,
				"error", err,
			)
			return nil, connecterr.Internal(ctx, "CancelTransfer", err, "detail", "failed to get gear conversation")
		}
		msgID, err := s.systemMessageWriter.InsertLocalizedReturnID(
			ctx,
			conversationID,
			authInfo.UserID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_CANCELLED,
			chat.CancelledTransferMessage(),
		)
		if err != nil {
			logger.ErrorContext(
				ctx, "failed to insert cancel system message",
				"conversation_id", conversationID,
				"error", err,
			)
			return nil, connecterr.Internal(ctx, "CancelTransfer", err, "detail", "failed to insert system message")
		}
		cancelSystemMessageID = msgID
	}

	_, _, cancelEventID, err := s.transitionTransferState(ctx, transferStored, authInfo.UserID, models.TransferState_TRANSFER_STATE_CANCELLED, cancelSystemMessageID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&api.CancelTransferResponse{
		CommunityEventId: cancelEventID,
	}), nil
}

// StartLoan starts a loan (RECIPIENT_SELECTED → ACTIVE).
// Only applies to loans, not giveaways.
func (s *Service) StartLoan(
	ctx context.Context,
	req *connect.Request[api.StartLoanRequest],
) (*connect.Response[api.StartLoanResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	// Fetch the transfer to get gear_id and community_id
	transferStored := &models.Transfer{}
	err = s.storage.GetByID(ctx, req.Msg.TransferId, transferStored)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to get transfer",
			"transfer_id", req.Msg.TransferId,
			"error", err,
		)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("transfer not found"))
	}

	// Check for existing active transfer on this gear
	activeTransfers, err := storage.QueryByFields[*models.Transfer](s.storage, ctx, map[string]any{
		"gear_id": transferStored.GearId,
		"state":   models.TransferState_TRANSFER_STATE_ACTIVE,
	})
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to check for active transfers",
			"gear_id", transferStored.GearId,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "StartLoan", err, "detail", "failed to check for active transfers")
	}
	if len(activeTransfers) > 0 {
		logger.WarnContext(
			ctx, "cannot start loan while another loan is active",
			"gear_id", transferStored.GearId,
			"active_transfer_id", activeTransfers[0].Id,
			"requested_transfer_id", req.Msg.TransferId,
		)
		return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "transfer_loan_in_progress", "a loan is already in progress for this gear", nil)
	}

	// Insert system message for loan start before the state transition
	// so the emitted CommunityEvent can carry its id (UndoStartLoan
	// uses system_chat_message_id to soft-delete the message by direct
	// read).
	var startedSystemMessageID string
	if s.systemMessageWriter != nil {
		conversationID, err := s.getGearConversationID(ctx, transferStored.GearId, transferStored.CommunityId)
		if err != nil {
			logger.ErrorContext(
				ctx, "failed to get gear conversation",
				"gear_id", transferStored.GearId,
				"community_id", transferStored.CommunityId,
				"error", err,
			)
			return nil, connecterr.Internal(ctx, "StartLoan", err, "detail", "failed to get gear conversation")
		}
		msgID, err := s.systemMessageWriter.InsertLocalizedReturnID(
			ctx,
			conversationID,
			authInfo.UserID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_STARTED,
			chat.StartedLoanMessage(),
		)
		if err != nil {
			logger.ErrorContext(
				ctx, "failed to insert start loan system message",
				"conversation_id", conversationID,
				"error", err,
			)
			return nil, connecterr.Internal(ctx, "StartLoan", err, "detail", "failed to insert system message")
		}
		startedSystemMessageID = msgID
	}

	updatedTransfer, gear, startEventID, err := s.transitionTransferState(ctx, transferStored, authInfo.UserID, models.TransferState_TRANSFER_STATE_ACTIVE, startedSystemMessageID)
	if err != nil {
		return nil, err
	}

	// Persist initial ImpactEstimate on the already-updated transfer.
	if s.estimatorCfg != nil {
		if gear == nil {
			gear = &models.Gear{}
		}
		connCtx := s.resolveConnectionContext(ctx, updatedTransfer.OwnerId, updatedTransfer.RecipientId, updatedTransfer.CommunityId, logger)
		ie := impact_metrics.BuildTransferImpactMetrics(gear, updatedTransfer.TransferType, s.estimatorCfg, connCtx, nil)
		updatedTransfer.ImpactEstimate = impact_metrics.APIImpactToModels(ie)
		if err := s.storage.Update(ctx, updatedTransfer); err != nil {
			logger.ErrorContext(ctx, "failed to persist ImpactEstimate on transfer",
				"transfer_id", req.Msg.TransferId, "error", err)
			return nil, connecterr.Internal(ctx, "StartLoan", err, "detail", "failed to persist impact estimate")
		}
	}

	return connect.NewResponse(&api.StartLoanResponse{
		CommunityEventId: startEventID,
	}), nil
}

// CompleteTransfer completes a transfer (ACTIVE → COMPLETED for loans, RECIPIENT_SELECTED → COMPLETED for giveaways).
func (s *Service) CompleteTransfer(
	ctx context.Context,
	req *connect.Request[api.CompleteTransferRequest],
) (*connect.Response[api.CompleteTransferResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	// Fetch the transfer to get gear_id and community_id
	transferStored := &models.Transfer{}
	err = s.storage.GetByID(ctx, req.Msg.TransferId, transferStored)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to get transfer",
			"transfer_id", req.Msg.TransferId,
			"error", err,
		)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("transfer not found"))
	}

	// Insert system message before state transition (use gear conversation
	// for both loans and giveaways). Captured id is threaded onto the
	// emitted CommunityEvent so UndoCompleteLoan can soft-delete the
	// message via direct read.
	var completionSystemMessageID string
	if s.systemMessageWriter != nil {
		conversationID, err := s.getGearConversationID(ctx, transferStored.GearId, transferStored.CommunityId)
		if err != nil {
			logger.ErrorContext(
				ctx, "failed to get gear conversation",
				"gear_id", transferStored.GearId,
				"community_id", transferStored.CommunityId,
				"error", err,
			)
			return nil, connecterr.Internal(ctx, "CompleteTransfer", err, "detail", "failed to get gear conversation")
		}
		// Loans read "Handed off ✓"; giveaways name the recipient. "Transfer"
		// is internal jargon banned in user copy (#2724).
		completionMsg := chat.HandedOffMessage()
		if transferStored.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
			completionMsg = chat.GivenToMessage(s.getUserDisplayName(ctx, transferStored.RecipientId))
		}
		msgID, err := s.systemMessageWriter.InsertLocalizedReturnID(
			ctx,
			conversationID,
			authInfo.UserID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_COMPLETED,
			completionMsg,
		)
		if err != nil {
			logger.ErrorContext(
				ctx, "failed to insert complete system message",
				"conversation_id", conversationID,
				"error", err,
			)
			return nil, connecterr.Internal(ctx, "CompleteTransfer", err, "detail", "failed to insert system message")
		}
		completionSystemMessageID = msgID
	}

	updatedTransfer, gear, completionEventID, err := s.transitionTransferState(ctx, transferStored, authInfo.UserID, models.TransferState_TRANSFER_STATE_COMPLETED, completionSystemMessageID)
	if err != nil {
		return nil, err
	}

	// Ensure gear is loaded for impact estimation (may be nil for giveaway transitions).
	if gear == nil {
		gear = &models.Gear{}
		if err := s.storage.GetByID(ctx, updatedTransfer.GearId, gear); err != nil {
			logger.WarnContext(
				ctx, "failed to fetch gear for impact estimation",
				"gear_id", updatedTransfer.GearId,
				"error", err,
			)
			gear = &models.Gear{} // Use empty gear — builder handles nil fields gracefully
		}
	}

	// Build impact estimate for this single transfer.
	var ie *api.ImpactEstimate
	if s.estimatorCfg != nil {
		connCtx := s.resolveConnectionContext(ctx, updatedTransfer.OwnerId, updatedTransfer.RecipientId, updatedTransfer.CommunityId, logger)
		// Infer social attributes via LLM for improved QT estimation.
		// Skip during simulation to avoid expensive AI calls.
		var hint *impact_metrics.QualityTimeHint
		if s.aiProvider != nil && !clock.IsSimulated(ctx) {
			txTypeStr := "gear_loan"
			if updatedTransfer.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
				txTypeStr = "giveaway"
			}
			var gearValueUSD float32
			if gear.ValueEstimate != nil {
				gearValueUSD = gear.ValueEstimate.EstimatedValueUsd
			}
			start := time.Now()
			inference, inferErr := ai.CallWithTimeout(ctx, 30*time.Second, func(ctx context.Context) (*ai.SocialAttributeInference, error) {
				return s.aiProvider.InferSocialAttributes(ctx, gear.Name, gear.Description, txTypeStr, gearValueUSD)
			})
			if inferErr != nil {
				if errors.Is(inferErr, context.DeadlineExceeded) {
					logger.WarnContext(ctx, "LLM call exceeded timeout budget, using config defaults",
						"external_service", "ai_provider",
						"operation", "InferSocialAttributes",
						"duration_ms", time.Since(start).Milliseconds(),
						"error", inferErr)
				} else {
					logger.WarnContext(ctx, "LLM social attribute inference failed, using config defaults", "error", inferErr)
				}
			} else if inference != nil {
				hint = &impact_metrics.QualityTimeHint{
					DurationMinutes:    inference.DurationMinutes,
					VulnerabilityLevel: inference.VulnerabilityLevel,
				}
			}
		}
		ie = impact_metrics.BuildTransferImpactMetrics(gear, updatedTransfer.TransferType, s.estimatorCfg, connCtx, hint)
	} else {
		ie = &api.ImpactEstimate{}
	}

	// Persist refined ImpactEstimate on the already-updated transfer.
	if s.estimatorCfg != nil {
		updatedTransfer.ImpactEstimate = impact_metrics.APIImpactToModels(ie)
		if err := s.storage.Update(ctx, updatedTransfer); err != nil {
			logger.WarnContext(ctx, "failed to persist ImpactEstimate on completed transfer",
				"transfer_id", req.Msg.TransferId, "error", err)
		}
	}

	// Story generation is owned by the story_subscriber on the
	// community-event bus (#510 PR 4) — TRANSFER_COMPLETED events
	// emitted above flow through the bus and trigger the subscriber.

	return connect.NewResponse(&api.CompleteTransferResponse{
		Impact:           ie,
		CommunityEventId: completionEventID,
	}), nil
}

// recordCommunityEventForTransfer records a community event for a transfer state change.
// objectUserID is optional and represents the other party in a transfer (for events like COMPLETED or CANCELLED).
// systemChatMessageID is optional and, when non-empty, is persisted on the CommunityEvent so Undo* RPCs can
// soft-delete the corresponding system chat message via direct read rather than a time-range query.
// undoData is optional and carries the prior-state snapshot required to reverse this action via an Undo* RPC.
// Returns the inserted event's id (or the empty string if the state does not map to a known event type).
func (s *Service) recordCommunityEventForTransfer(
	ctx context.Context,
	transferID, gearID string,
	transferState models.TransferState,
	transferType models.TransferType,
	actorID, objectUserID, communityID, systemChatMessageID string,
	undoData *models.UndoData,
) (string, error) {
	logger := logging.LoggerWithContext(ctx)

	// Determine event type based on transfer state
	var eventType models.CommunityEventType
	switch transferState {
	case models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED:
		eventType = models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED
	case models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
		eventType = models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED
	case models.TransferState_TRANSFER_STATE_ACTIVE:
		eventType = models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE
	case models.TransferState_TRANSFER_STATE_COMPLETED:
		eventType = models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED
	case models.TransferState_TRANSFER_STATE_CANCELLED:
		eventType = models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED
	default:
		return "", nil // Unknown state, nothing to log
	}

	event := &models.CommunityEvent{
		CommunityId:  communityID,
		EventType:    eventType,
		ActorId:      actorID,
		ObjectUserId: objectUserID,
		GearId:       gearID,
		Topic:        &models.CommunityEvent_TransferId{TransferId: transferID},
		TransferType: transferType,
		UndoData:     undoData,
	}
	if systemChatMessageID != "" {
		event.SystemChatMessageId = &systemChatMessageID
	}
	if _, err := s.bus.Publish(ctx, event); err != nil {
		logger.ErrorContext(
			ctx, "failed to record event for community",
			"community_id", communityID,
			"transfer_id", transferID,
			"error", err,
		)
		return "", err
	}

	return event.Id, nil
}

// UpdateTransfer updates a transfer's properties.
// Allows updating transfer type (in INTEREST_EXPRESSED state only) and
// estimated pickup time/loan duration (in RECIPIENT_SELECTED state).
func (s *Service) UpdateTransfer(
	ctx context.Context,
	req *connect.Request[api.UpdateTransferRequest],
) (*connect.Response[api.UpdateTransferResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"transfer_id", req.Msg.TransferId,
	)

	// Fetch the transfer
	transferStored := &models.Transfer{}
	err = s.storage.GetByID(ctx, req.Msg.TransferId, transferStored)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to get transfer",
			"transfer_id", req.Msg.TransferId,
			"error", err,
		)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("transfer not found"))
	}

	// Verify actor is the owner or selected recipient
	isOwner := transferStored.OwnerId == authInfo.UserID
	isRecipient := transferStored.RecipientId == authInfo.UserID
	if !isOwner && !isRecipient {
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "transfer_owner_or_recipient_required_for_modify", "only the gear owner or recipient can modify this transfer", nil)
	}

	// Determine what can be updated based on current state
	isInterestExpressed := transferStored.State == models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED
	isRecipientSelected := transferStored.State == models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED

	// Transfer type can only be updated in INTEREST_EXPRESSED state by owner
	if req.Msg.TransferType != api.TransferType_TRANSFER_TYPE_UNSPECIFIED {
		if !isInterestExpressed {
			return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "transfer_invalid_state_for_update_type", "can only update the transfer type before interest is acted on", nil)
		}
		if !isOwner {
			return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "transfer_owner_required_for_update_type", "only the gear owner can update the transfer type", nil)
		}
		transferStored.TransferType = apiTransferTypeToModel(req.Msg.TransferType)
	}

	// Estimated pickup time and loan duration can be updated in RECIPIENT_SELECTED state
	if req.Msg.EstimatedPickupUnixSec != nil || req.Msg.LoanDurationDays != nil {
		if !isRecipientSelected {
			return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "transfer_invalid_state_for_pickup_details", "can only set pickup details after a recipient has been selected", nil)
		}
		if req.Msg.EstimatedPickupUnixSec != nil {
			transferStored.EstimatedPickupUnixSec = req.Msg.EstimatedPickupUnixSec
		}
		if req.Msg.LoanDurationDays != nil {
			transferStored.LoanDurationDays = req.Msg.LoanDurationDays
		}
	}

	err = s.storage.Update(ctx, transferStored)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to update transfer",
			"transfer_id", req.Msg.TransferId,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "UpdateTransfer", err)
	}

	logger.InfoContext(ctx, "transfer updated successfully")

	// Notify the other party when pickup time is proposed.
	if req.Msg.EstimatedPickupUnixSec != nil {
		event := &models.CommunityEvent{
			CommunityId:  transferStored.CommunityId,
			ActorId:      authInfo.UserID,
			EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_PICKUP_PROPOSED,
			GearId:       transferStored.GearId,
			Topic:        &models.CommunityEvent_TransferId{TransferId: transferStored.Id},
			TransferType: transferStored.TransferType,
		}
		if _, err := s.bus.Publish(ctx, event); err != nil {
			logger.WarnContext(ctx, "failed to record pickup proposed event", "error", err)
		}
	}

	return connect.NewResponse(&api.UpdateTransferResponse{}), nil
}

// getGearConversationID looks up the conversation ID for a gear item.
func (s *Service) getGearConversationID(ctx context.Context, gearID, communityID string) (string, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"gear_id", gearID,
		"community_id", communityID,
	)

	// Query CommunityGear to find the conversation_id
	communityGears, err := storage.QueryByFields[*models.CommunityGear](s.storage, ctx, map[string]any{
		"gear_id":      gearID,
		"community_id": communityID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to query community gear: %w", err)
	}

	if len(communityGears) == 0 {
		return "", fmt.Errorf("community gear not found")
	}

	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gear); err != nil {
		logger.WarnContext(ctx, "failed to get gear for conversation lookup", "error", err)
	}

	conversationID := gear.ConversationId
	if conversationID == "" {
		logger.ErrorContext(ctx, "no conversation_id found for gear")
		return "", fmt.Errorf("conversation not found for gear")
	}

	logger.DebugContext(ctx, "found gear conversation", "conversation_id", conversationID)
	return conversationID, nil
}

// generateStoryForCompletedTransfer moved to
// server/story/subscriber/transfer.go in #510 PR 4. TRANSFER_COMPLETED
// events published by transitionTransferState now flow through the
// community-event bus and trigger the story_subscriber.
