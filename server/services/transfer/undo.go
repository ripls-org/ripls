package transfer

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// UndoSelectRecipient reverses a prior SelectRecipient action.
//
// Flow (see docs/ai/undo_plan.md §6 for the canonical version):
//  1. Load the CommunityEvent by id; validate exists, actor matches,
//     event type is TRANSFER_RECIPIENT_SELECTED, not already undone.
//  2. Load the transfer; drift-check that it is still in RECIPIENT_SELECTED.
//  3. Revert to INTEREST_EXPRESSED and clear recipient_id.
//  4. Soft-delete the approval system chat message referenced by
//     SystemChatMessageId on the event.
//  5. Emit a retraction CommunityEvent (TRANSFER_RECIPIENT_SELECTED_UNDONE).
//  6. Emit a replacement system chat message (CHAT_SYSTEM_ACTION_UNDONE)
//     referencing the retracted action.
func (s *Service) UndoSelectRecipient(
	ctx context.Context,
	req *connect.Request[api.UndoSelectRecipientRequest],
) (*connect.Response[api.UndoSelectRecipientResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "undo_select_recipient",
		"user_id", authInfo.UserID,
		"community_event_id", req.Msg.CommunityEventId,
	)

	if req.Msg.CommunityEventId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_event_id is required"))
	}

	// Step 1: Load and validate the CommunityEvent.
	event := &models.CommunityEvent{}
	if err := s.storage.GetByID(ctx, req.Msg.CommunityEventId, event); err != nil {
		logger.InfoContext(ctx, "undo target event not found", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("community event not found"))
	}

	if event.ActorId != authInfo.UserID {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_NOT_ACTOR,
			"Only the person who selected the recipient can undo this.",
		)
	}

	if event.EventType != models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_WRONG_EVENT_TYPE,
			"That action cannot be undone with UndoSelectRecipient.",
		)
	}

	// Check for an existing retraction.
	alreadyUndone, err := s.hasRetractionEvent(
		ctx,
		req.Msg.CommunityEventId,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED_UNDONE,
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to check for retraction", "error", err)
		return nil, connecterr.Internal(ctx, "UndoSelectRecipient", err, "detail", "undo pre-check failed")
	}
	if alreadyUndone {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_ALREADY_UNDONE,
			"This action has already been undone.",
		)
	}

	// Step 2: Load the transfer and drift-check.
	transferID := event.GetTransferId()
	if transferID == "" {
		logger.ErrorContext(ctx, "community event has no transfer_id topic", "event_id", event.Id)
		return nil, connecterr.Internal(ctx, "UndoSelectRecipient", fmt.Errorf("community event missing transfer topic"))
	}
	transferStored := &models.Transfer{}
	if err := s.storage.GetByID(ctx, transferID, transferStored); err != nil {
		logger.InfoContext(ctx, "undo target transfer not found", "transfer_id", transferID, "error", err)
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED,
			"The transfer has been removed.",
		)
	}
	if transferStored.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED,
			"The transfer has changed since then — it's no longer in a state where this action can be undone.",
		)
	}

	// Step 3: Revert transfer state. recipient_id is intentionally NOT
	// cleared: for giveaways the interested user's id was already on
	// recipient_id pre-selection (ExpressInterest writes it at transfer
	// creation time) and SelectRecipient's write was idempotent; for
	// loans SelectRecipient is skipped entirely (auto-approved). Either
	// way, the pre-action recipient_id equals the post-selection one,
	// and undo should leave it alone.
	transferStored.State = models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED
	if err := s.storage.Update(ctx, transferStored); err != nil {
		logger.ErrorContext(ctx, "failed to revert transfer state", "error", err)
		return nil, connecterr.Internal(ctx, "UndoSelectRecipient", err, "detail",

			// Step 4: Soft-delete the approval system chat message, if one was recorded.
			"failed to revert transfer")
	}

	conversationID, err := s.getGearConversationID(ctx, transferStored.GearId, transferStored.CommunityId)
	if err != nil {
		logger.WarnContext(ctx, "failed to resolve conversation for undo retraction", "error", err)
		// Non-fatal: state has already been reverted. The retraction
		// message just won't be posted.
		conversationID = ""
	}

	if systemChatMessageID := event.GetSystemChatMessageId(); systemChatMessageID != "" {
		msg := &models.ChatMessage{}
		if err := s.storage.GetByID(ctx, systemChatMessageID, msg); err != nil {
			logger.WarnContext(ctx, "approval chat message not found for soft-delete",
				"system_chat_message_id", systemChatMessageID,
				"error", err,
			)
		} else {
			now := clock.UnixSec(ctx)
			msg.Deleted = &models.DeletedMetadata{
				DeletedByUserId:  authInfo.UserID,
				DeletedAtUnixSec: now,
			}
			if err := s.storage.Update(ctx, msg); err != nil {
				logger.WarnContext(ctx, "failed to soft-delete approval chat message",
					"system_chat_message_id", systemChatMessageID,
					"error", err,
				)
			}
		}
	}

	// Step 5: Emit retraction CommunityEvent (no system_chat_message_id —
	// the replacement system message below is informational, not itself
	// reversible).
	retraction := &models.CommunityEvent{
		CommunityId:  event.CommunityId,
		EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED_UNDONE,
		ActorId:      authInfo.UserID,
		GearId:       event.GearId,
		Topic:        &models.CommunityEvent_TransferId{TransferId: transferID},
		TransferType: event.TransferType,
	}
	if _, err := s.bus.Publish(ctx, retraction); err != nil {
		logger.WarnContext(ctx, "failed to record retraction community event", "error", err)
		// Non-fatal: the state reversal has already succeeded.
	}

	// Step 6: Emit replacement system chat message.
	if conversationID != "" && s.systemMessageWriter != nil {
		actorName := s.getUserDisplayName(ctx, authInfo.UserID)
		undoneAction := models.ChatSystemAction_CHAT_SYSTEM_ACTION_APPROVED
		if _, err := s.systemMessageWriter.InsertLocalizedReturnID(
			ctx,
			conversationID,
			authInfo.UserID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_UNDONE,
			chat.UndoneMessage(actorName, undoneAction),
		); err != nil {
			logger.WarnContext(ctx, "failed to insert undone system message", "error", err)
		}
	}

	logger.InfoContext(ctx, "undid select recipient",
		"transfer_id", transferID,
		"outcome", "success",
	)

	return connect.NewResponse(&api.UndoSelectRecipientResponse{}), nil
}

// UndoCompleteLoan reverses a prior CompleteTransfer on a loan.
//
// Flow:
//  1. Load the CommunityEvent by id; validate it exists, actor matches,
//     event type is TRANSFER_COMPLETED on a LOAN, carries a
//     LoanCompletion UndoData variant, and is not already undone.
//  2. Load the transfer; drift-check that it is still COMPLETED.
//  3. Revert to ACTIVE, restore ActualReturnUnixSec and ImpactEstimate
//     from UndoData.
//  4. Restore the gear state captured in UndoData (typically
//     UNAVAILABLE, since the loan was active before completion).
//  5. Soft-delete the completion system chat message referenced by
//     SystemChatMessageId.
//  6. Emit a TRANSFER_COMPLETED_UNDONE retraction event.
//  7. Emit a CHAT_SYSTEM_ACTION_UNDONE replacement system message.
func (s *Service) UndoCompleteLoan(
	ctx context.Context,
	req *connect.Request[api.UndoCompleteLoanRequest],
) (*connect.Response[api.UndoCompleteLoanResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "undo_complete_loan",
		"user_id", authInfo.UserID,
		"community_event_id", req.Msg.CommunityEventId,
	)

	if req.Msg.CommunityEventId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_event_id is required"))
	}

	// Step 1: Load and validate the CommunityEvent.
	event := &models.CommunityEvent{}
	if err := s.storage.GetByID(ctx, req.Msg.CommunityEventId, event); err != nil {
		logger.InfoContext(ctx, "undo target event not found", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("community event not found"))
	}

	if event.ActorId != authInfo.UserID {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_NOT_ACTOR,
			"Only the person who marked this complete can undo it.",
		)
	}

	if event.EventType != models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED ||
		event.TransferType != models.TransferType_TRANSFER_TYPE_LOAN {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_WRONG_EVENT_TYPE,
			"That action cannot be undone with UndoCompleteLoan.",
		)
	}

	loanUndo := event.GetUndoData().GetLoanCompletion()
	if loanUndo == nil {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_WRONG_EVENT_TYPE,
			"This completion is missing the data needed to reverse it.",
		)
	}

	alreadyUndone, err := s.hasRetractionEvent(
		ctx,
		req.Msg.CommunityEventId,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED_UNDONE,
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to check for retraction", "error", err)
		return nil, connecterr.Internal(ctx, "UndoCompleteLoan", err, "detail", "undo pre-check failed")
	}
	if alreadyUndone {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_ALREADY_UNDONE,
			"This action has already been undone.",
		)
	}

	// Step 2: Load the transfer and drift-check.
	transferID := event.GetTransferId()
	if transferID == "" {
		logger.ErrorContext(ctx, "community event has no transfer_id topic")
		return nil, connecterr.Internal(ctx, "UndoCompleteLoan", fmt.Errorf("community event missing transfer topic"))
	}
	transferStored := &models.Transfer{}
	if err := s.storage.GetByID(ctx, transferID, transferStored); err != nil {
		logger.InfoContext(ctx, "undo target transfer not found", "transfer_id", transferID, "error", err)
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED,
			"The loan has been removed.",
		)
	}
	if transferStored.State != models.TransferState_TRANSFER_STATE_COMPLETED {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED,
			"The loan is no longer in a state where this action can be undone.",
		)
	}

	// Step 3: Revert transfer state and timestamps/IE from the snapshot.
	transferStored.State = models.TransferState_TRANSFER_STATE_ACTIVE
	transferStored.ActualReturnUnixSec = loanUndo.PriorActualReturnUnixSec
	transferStored.ImpactEstimate = loanUndo.PriorImpactEstimate
	if err := s.storage.Update(ctx, transferStored); err != nil {
		logger.ErrorContext(ctx, "failed to revert transfer state", "error", err)
		return nil, connecterr.Internal(ctx, "UndoCompleteLoan", err, "detail",

			// Step 4: Restore the gear to its prior state (usually UNAVAILABLE
			// since the loan was active before completion).
			"failed to revert transfer")
	}

	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, transferStored.GearId, gear); err != nil {
		logger.WarnContext(ctx, "gear for undo target not found; continuing", "error", err)
	} else if gear.State != loanUndo.PriorGearState {
		gear.State = loanUndo.PriorGearState
		if err := s.storage.Update(ctx, gear); err != nil {
			logger.WarnContext(ctx, "failed to restore prior gear state", "error", err)
		}
	}

	// Step 5: Soft-delete the completion system chat message.
	conversationID, convErr := s.getGearConversationID(ctx, transferStored.GearId, transferStored.CommunityId)
	if convErr != nil {
		logger.WarnContext(ctx, "failed to resolve conversation for undo retraction", "error", convErr)
		conversationID = ""
	}
	if systemChatMessageID := event.GetSystemChatMessageId(); systemChatMessageID != "" {
		msg := &models.ChatMessage{}
		if err := s.storage.GetByID(ctx, systemChatMessageID, msg); err != nil {
			logger.WarnContext(ctx, "completion chat message not found for soft-delete",
				"system_chat_message_id", systemChatMessageID, "error", err)
		} else {
			msg.Deleted = &models.DeletedMetadata{
				DeletedByUserId:  authInfo.UserID,
				DeletedAtUnixSec: clock.UnixSec(ctx),
			}
			if err := s.storage.Update(ctx, msg); err != nil {
				logger.WarnContext(ctx, "failed to soft-delete completion chat message",
					"system_chat_message_id", systemChatMessageID, "error", err)
			}
		}
	}

	// Step 6: Emit retraction CommunityEvent.
	retraction := &models.CommunityEvent{
		CommunityId:  event.CommunityId,
		EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED_UNDONE,
		ActorId:      authInfo.UserID,
		GearId:       event.GearId,
		Topic:        &models.CommunityEvent_TransferId{TransferId: transferID},
		TransferType: event.TransferType,
	}
	if _, err := s.bus.Publish(ctx, retraction); err != nil {
		logger.WarnContext(ctx, "failed to record retraction community event", "error", err)
	}

	// Step 7: Emit replacement system chat message.
	if conversationID != "" && s.systemMessageWriter != nil {
		actorName := s.getUserDisplayName(ctx, authInfo.UserID)
		undoneAction := models.ChatSystemAction_CHAT_SYSTEM_ACTION_COMPLETED
		if _, err := s.systemMessageWriter.InsertLocalizedReturnID(
			ctx,
			conversationID,
			authInfo.UserID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_UNDONE,
			chat.UndoneMessage(actorName, undoneAction),
		); err != nil {
			logger.WarnContext(ctx, "failed to insert undone system message", "error", err)
		}
	}

	logger.InfoContext(ctx, "undid complete loan",
		"transfer_id", transferID,
		"outcome", "success",
	)

	return connect.NewResponse(&api.UndoCompleteLoanResponse{}), nil
}

// UndoCompleteGiveaway reverses a prior CompleteTransfer on a giveaway.
//
// Flow:
//  1. Load the CommunityEvent; validate actor, event_type is
//     TRANSFER_COMPLETED on a GIVEAWAY, carries a GiveawayCascade
//     UndoData variant, and is not already undone.
//  2. Load the transfer; drift-check still COMPLETED.
//  3. Revert transfer to RECIPIENT_SELECTED, clear ActualPickupUnixSec,
//     restore ImpactEstimate.
//  4. Restore gear state from the cascade snapshot (typically
//     AVAILABLE, since that's what the giveaway was shared as).
//  5. Revive each cancelled sibling transfer to its prior state. If a
//     sibling has been further mutated since (e.g., manually
//     cancelled), return CASCADE_UNRECOVERABLE — we won't silently
//     overwrite a legitimate later action.
//  6. Un-archive each community_gear row the cascade archived, with
//     the same drift check.
//  7. Soft-delete the completion system chat message; emit retraction
//     event + replacement CHAT_SYSTEM_ACTION_UNDONE message.
func (s *Service) UndoCompleteGiveaway(
	ctx context.Context,
	req *connect.Request[api.UndoCompleteGiveawayRequest],
) (*connect.Response[api.UndoCompleteGiveawayResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "undo_complete_giveaway",
		"user_id", authInfo.UserID,
		"community_event_id", req.Msg.CommunityEventId,
	)

	if req.Msg.CommunityEventId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_event_id is required"))
	}

	event := &models.CommunityEvent{}
	if err := s.storage.GetByID(ctx, req.Msg.CommunityEventId, event); err != nil {
		logger.InfoContext(ctx, "undo target event not found", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("community event not found"))
	}

	if event.ActorId != authInfo.UserID {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_NOT_ACTOR,
			"Only the person who completed this giveaway can undo it.",
		)
	}

	if event.EventType != models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED ||
		event.TransferType != models.TransferType_TRANSFER_TYPE_GIVEAWAY {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_WRONG_EVENT_TYPE,
			"That action cannot be undone with UndoCompleteGiveaway.",
		)
	}

	cascade := event.GetUndoData().GetGiveawayCascade()
	if cascade == nil {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_WRONG_EVENT_TYPE,
			"This completion is missing the data needed to reverse it.",
		)
	}

	alreadyUndone, err := s.hasRetractionEvent(
		ctx,
		req.Msg.CommunityEventId,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED_UNDONE,
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to check for retraction", "error", err)
		return nil, connecterr.Internal(ctx, "UndoCompleteGiveaway", err, "detail", "undo pre-check failed")
	}
	if alreadyUndone {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_ALREADY_UNDONE,
			"This action has already been undone.",
		)
	}

	transferID := event.GetTransferId()
	if transferID == "" {
		return nil, connecterr.Internal(ctx, "UndoCompleteGiveaway", fmt.Errorf("community event missing transfer topic"))
	}
	transferStored := &models.Transfer{}
	if err := s.storage.GetByID(ctx, transferID, transferStored); err != nil {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED,
			"The giveaway has been removed.",
		)
	}
	if transferStored.State != models.TransferState_TRANSFER_STATE_COMPLETED {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED,
			"The giveaway is no longer in a state where this action can be undone.",
		)
	}

	// Step 3: revert primary transfer.
	transferStored.State = models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED
	transferStored.ActualPickupUnixSec = nil
	transferStored.ImpactEstimate = cascade.PriorImpactEstimate
	if err := s.storage.Update(ctx, transferStored); err != nil {
		return nil, connecterr.Internal(ctx, "UndoCompleteGiveaway", err, "detail",

			// Step 4: restore gear state.
			"failed to revert transfer")
	}

	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, transferStored.GearId, gear); err != nil {
		logger.WarnContext(ctx, "gear not found for undo", "error", err)
	} else if gear.State == models.GearState_GEAR_STATE_GIVEN_AWAY {
		gear.State = cascade.PriorGearState
		if err := s.storage.Update(ctx, gear); err != nil {
			logger.WarnContext(ctx, "failed to restore prior gear state", "error", err)
		}
	}

	// Step 5: revive each cancelled sibling. If a sibling no longer
	// exists or has been mutated away from CANCELLED, treat the cascade
	// as unrecoverable — we won't clobber an actor who explicitly
	// cancelled a revived transfer.
	for _, sibling := range cascade.CancelledSiblings {
		st := &models.Transfer{}
		if err := s.storage.GetByID(ctx, sibling.TransferId, st); err != nil {
			return nil, undoFailure(
				api.UndoFailureReason_UNDO_FAILURE_REASON_CASCADE_UNRECOVERABLE,
				"A related transfer is no longer available, so this can't be undone cleanly.",
			)
		}
		if st.State != models.TransferState_TRANSFER_STATE_CANCELLED {
			return nil, undoFailure(
				api.UndoFailureReason_UNDO_FAILURE_REASON_CASCADE_UNRECOVERABLE,
				"A related transfer has changed, so this can't be undone cleanly.",
			)
		}
		st.State = sibling.PriorState
		if err := s.storage.Update(ctx, st); err != nil {
			return nil, connecterr.Internal(ctx, "UndoCompleteGiveaway", err, "detail", "failed to revive sibling transfer")
		}
	}

	// Step 6: un-archive each community_gear row the cascade archived.
	for _, cgID := range cascade.ArchivedCommunityGearIds {
		cg := &models.CommunityGear{}
		if err := s.storage.GetByID(ctx, cgID, cg); err != nil {
			logger.WarnContext(ctx, "community_gear not found for undo; skipping", "id", cgID, "error", err)
			continue
		}
		if !cg.Archived {
			// Someone else already un-archived it. Leave as-is.
			continue
		}
		cg.Archived = false
		if err := s.storage.Update(ctx, cg); err != nil {
			logger.WarnContext(ctx, "failed to un-archive community_gear", "id", cgID, "error", err)
		}
	}

	// Step 7: chat retraction + retraction event + replacement message.
	conversationID, convErr := s.getGearConversationID(ctx, transferStored.GearId, transferStored.CommunityId)
	if convErr != nil {
		conversationID = ""
	}
	if systemChatMessageID := event.GetSystemChatMessageId(); systemChatMessageID != "" {
		msg := &models.ChatMessage{}
		if err := s.storage.GetByID(ctx, systemChatMessageID, msg); err == nil {
			msg.Deleted = &models.DeletedMetadata{
				DeletedByUserId:  authInfo.UserID,
				DeletedAtUnixSec: clock.UnixSec(ctx),
			}
			if err := s.storage.Update(ctx, msg); err != nil {
				logger.WarnContext(ctx, "failed to soft-delete completion chat message", "error", err)
			}
		}
	}

	retraction := &models.CommunityEvent{
		CommunityId:  event.CommunityId,
		EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED_UNDONE,
		ActorId:      authInfo.UserID,
		GearId:       event.GearId,
		Topic:        &models.CommunityEvent_TransferId{TransferId: transferID},
		TransferType: event.TransferType,
	}
	if _, err := s.bus.Publish(ctx, retraction); err != nil {
		logger.WarnContext(ctx, "failed to record retraction community event", "error", err)
	}

	if conversationID != "" && s.systemMessageWriter != nil {
		actorName := s.getUserDisplayName(ctx, authInfo.UserID)
		if _, err := s.systemMessageWriter.InsertLocalizedReturnID(
			ctx,
			conversationID,
			authInfo.UserID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_UNDONE,
			chat.UndoneMessage(actorName, models.ChatSystemAction_CHAT_SYSTEM_ACTION_COMPLETED),
		); err != nil {
			logger.WarnContext(ctx, "failed to insert undone system message", "error", err)
		}
	}

	logger.InfoContext(ctx, "undid complete giveaway",
		"transfer_id", transferID,
		"revived_siblings", len(cascade.CancelledSiblings),
		"unarchived_community_gears", len(cascade.ArchivedCommunityGearIds),
		"outcome", "success",
	)

	return connect.NewResponse(&api.UndoCompleteGiveawayResponse{}), nil
}

// UndoStartLoan reverses a prior StartLoan call.
//
// Flow: load event, validate actor + event_type (TRANSFER_ACTIVE on a
// LOAN), drift-check transfer is still ACTIVE, revert state to
// RECIPIENT_SELECTED, restore ActualPickupUnixSec + gear state from
// UndoData.LoanStart, soft-delete the started system chat message,
// emit retraction + replacement.
func (s *Service) UndoStartLoan(
	ctx context.Context,
	req *connect.Request[api.UndoStartLoanRequest],
) (*connect.Response[api.UndoStartLoanResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "undo_start_loan",
		"user_id", authInfo.UserID,
		"community_event_id", req.Msg.CommunityEventId,
	)

	if req.Msg.CommunityEventId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_event_id is required"))
	}

	event := &models.CommunityEvent{}
	if err := s.storage.GetByID(ctx, req.Msg.CommunityEventId, event); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("community event not found"))
	}
	if event.ActorId != authInfo.UserID {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_NOT_ACTOR, "Only the person who started the loan can undo it.")
	}
	if event.EventType != models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE ||
		event.TransferType != models.TransferType_TRANSFER_TYPE_LOAN {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_WRONG_EVENT_TYPE, "That action cannot be undone with UndoStartLoan.")
	}

	loanStart := event.GetUndoData().GetLoanStart()
	if loanStart == nil {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_WRONG_EVENT_TYPE, "This start-loan event is missing the data needed to reverse it.")
	}

	alreadyUndone, err := s.hasRetractionEvent(ctx, req.Msg.CommunityEventId, models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE_UNDONE)
	if err != nil {
		return nil, connecterr.Internal(ctx, "UndoStartLoan", err, "detail", "undo pre-check failed")
	}
	if alreadyUndone {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_ALREADY_UNDONE, "This action has already been undone.")
	}

	transferID := event.GetTransferId()
	transferStored := &models.Transfer{}
	if err := s.storage.GetByID(ctx, transferID, transferStored); err != nil {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED, "The loan has been removed.")
	}
	if transferStored.State != models.TransferState_TRANSFER_STATE_ACTIVE {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED, "The loan is no longer in a state where this action can be undone.")
	}

	transferStored.State = models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED
	transferStored.ActualPickupUnixSec = loanStart.PriorActualPickupUnixSec
	// Clear the ImpactEstimate that StartLoan set; we have no persisted
	// prior IE snapshot for start-loan (the plan variant intentionally
	// omits it — IE is refreshed on completion anyway).
	transferStored.ImpactEstimate = nil
	if err := s.storage.Update(ctx, transferStored); err != nil {
		return nil, connecterr.Internal(ctx, "UndoStartLoan", err, "detail",

			// Restore gear state.
			"failed to revert transfer")
	}

	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, transferStored.GearId, gear); err == nil {
		if gear.State != loanStart.PriorGearState {
			gear.State = loanStart.PriorGearState
			if err := s.storage.Update(ctx, gear); err != nil {
				logger.WarnContext(ctx, "failed to restore prior gear state", "error", err)
			}
		}
	}

	s.softDeleteAndRetractTransferEvent(ctx, event, transferStored, authInfo.UserID,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE_UNDONE,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_STARTED, logger)

	logger.InfoContext(ctx, "undid start loan", "transfer_id", transferID, "outcome", "success")
	return connect.NewResponse(&api.UndoStartLoanResponse{}), nil
}

// UndoCancelTransfer reverses a prior CancelTransfer call, returning
// the transfer to its prior_state (and, when cancel-from-ACTIVE on a
// loan, restoring gear state).
func (s *Service) UndoCancelTransfer(
	ctx context.Context,
	req *connect.Request[api.UndoCancelTransferRequest],
) (*connect.Response[api.UndoCancelTransferResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "undo_cancel_transfer",
		"user_id", authInfo.UserID,
		"community_event_id", req.Msg.CommunityEventId,
	)

	if req.Msg.CommunityEventId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_event_id is required"))
	}

	event := &models.CommunityEvent{}
	if err := s.storage.GetByID(ctx, req.Msg.CommunityEventId, event); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("community event not found"))
	}
	if event.ActorId != authInfo.UserID {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_NOT_ACTOR, "Only the person who cancelled this can undo it.")
	}
	if event.EventType != models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_WRONG_EVENT_TYPE, "That action cannot be undone with UndoCancelTransfer.")
	}

	cancelUndo := event.GetUndoData().GetTransferCancel()
	if cancelUndo == nil {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_WRONG_EVENT_TYPE, "This cancel event is missing the data needed to reverse it.")
	}

	alreadyUndone, err := s.hasRetractionEvent(ctx, req.Msg.CommunityEventId, models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED_UNDONE)
	if err != nil {
		return nil, connecterr.Internal(ctx, "UndoCancelTransfer", err, "detail", "undo pre-check failed")
	}
	if alreadyUndone {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_ALREADY_UNDONE, "This action has already been undone.")
	}

	transferID := event.GetTransferId()
	transferStored := &models.Transfer{}
	if err := s.storage.GetByID(ctx, transferID, transferStored); err != nil {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED, "The transfer has been removed.")
	}
	if transferStored.State != models.TransferState_TRANSFER_STATE_CANCELLED {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED, "The transfer is no longer cancelled, so this can't be undone.")
	}

	transferStored.State = cancelUndo.PriorState
	if err := s.storage.Update(ctx, transferStored); err != nil {
		return nil, connecterr.Internal(ctx, "UndoCancelTransfer", err, "detail",

			// Restore gear state if the cancel came from ACTIVE on a loan.
			"failed to revert transfer")
	}

	if cancelUndo.PriorGearState != nil {
		gear := &models.Gear{}
		if err := s.storage.GetByID(ctx, transferStored.GearId, gear); err == nil {
			if gear.State != *cancelUndo.PriorGearState {
				gear.State = *cancelUndo.PriorGearState
				if err := s.storage.Update(ctx, gear); err != nil {
					logger.WarnContext(ctx, "failed to restore prior gear state", "error", err)
				}
			}
		}
	}

	s.softDeleteAndRetractTransferEvent(ctx, event, transferStored, authInfo.UserID,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED_UNDONE,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_CANCELLED, logger)

	logger.InfoContext(ctx, "undid cancel transfer", "transfer_id", transferID, "outcome", "success")
	return connect.NewResponse(&api.UndoCancelTransferResponse{}), nil
}

// softDeleteAndRetractTransferEvent performs the three final steps
// common to every transfer-domain undo handler: soft-delete the
// original system chat message, emit the retraction CommunityEvent,
// and insert a CHAT_SYSTEM_ACTION_UNDONE replacement message. All
// failures here are non-fatal (logged) because the primary state
// revert has already succeeded.
func (s *Service) softDeleteAndRetractTransferEvent(
	ctx context.Context,
	event *models.CommunityEvent,
	transferStored *models.Transfer,
	actorID string,
	retractionType models.CommunityEventType,
	undoneAction models.ChatSystemAction,
	logger *logging.Logger,
) {
	conversationID, convErr := s.getGearConversationID(ctx, transferStored.GearId, transferStored.CommunityId)
	if convErr != nil {
		logger.WarnContext(ctx, "failed to resolve conversation for undo retraction", "error", convErr)
		conversationID = ""
	}

	if systemChatMessageID := event.GetSystemChatMessageId(); systemChatMessageID != "" {
		msg := &models.ChatMessage{}
		if err := s.storage.GetByID(ctx, systemChatMessageID, msg); err == nil {
			msg.Deleted = &models.DeletedMetadata{
				DeletedByUserId:  actorID,
				DeletedAtUnixSec: clock.UnixSec(ctx),
			}
			if err := s.storage.Update(ctx, msg); err != nil {
				logger.WarnContext(ctx, "failed to soft-delete original system chat message", "error", err)
			}
		}
	}

	retraction := &models.CommunityEvent{
		CommunityId:  event.CommunityId,
		EventType:    retractionType,
		ActorId:      actorID,
		GearId:       event.GearId,
		Topic:        &models.CommunityEvent_TransferId{TransferId: transferStored.Id},
		TransferType: event.TransferType,
	}
	if _, err := s.bus.Publish(ctx, retraction); err != nil {
		logger.WarnContext(ctx, "failed to record retraction community event", "error", err)
	}

	if conversationID != "" && s.systemMessageWriter != nil {
		actorName := s.getUserDisplayName(ctx, actorID)
		if _, err := s.systemMessageWriter.InsertLocalizedReturnID(
			ctx,
			conversationID,
			actorID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_UNDONE,
			chat.UndoneMessage(actorName, undoneAction),
		); err != nil {
			logger.WarnContext(ctx, "failed to insert undone system message", "error", err)
		}
	}
}

// hasRetractionEvent returns true if a retraction event of the given type
// already references the original community_event_id via the same topic
// and within the same community. We look up by topic rather than by
// event-id because CommunityEvent doesn't have a native "reverses" link;
// a retraction event shares the original's topic + community, has the
// matching _UNDONE event type, and was emitted after the original.
func (s *Service) hasRetractionEvent(ctx context.Context, originalEventID string, retractionType models.CommunityEventType) (bool, error) {
	original := &models.CommunityEvent{}
	if err := s.storage.GetByID(ctx, originalEventID, original); err != nil {
		return false, fmt.Errorf("load original event: %w", err)
	}
	transferID := original.GetTransferId()
	if transferID == "" {
		return false, errors.New("original event has no transfer topic")
	}
	candidates, err := s.storage.QueryByField(ctx, "transfer_id", transferID, &models.CommunityEvent{})
	if err != nil {
		return false, fmt.Errorf("query candidates: %w", err)
	}
	for _, msg := range candidates {
		ev := msg.(*models.CommunityEvent)
		if ev.EventType != retractionType {
			continue
		}
		if ev.OccurredAtUnixSec < original.OccurredAtUnixSec {
			continue
		}
		return true, nil
	}
	return false, nil
}

// undoFailure builds a typed Connect error with an UndoErrorDetail
// attached so clients can render a precise human-readable message and
// metrics can bucket by reason.
func undoFailure(reason api.UndoFailureReason, userMessage string) error {
	cerr := connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("undo failed: %s", reason))
	detail, derr := connect.NewErrorDetail(&api.UndoErrorDetail{
		Reason:      reason,
		UserMessage: userMessage,
	})
	if derr != nil {
		return cerr
	}
	cerr.AddDetail(detail)
	return cerr
}
