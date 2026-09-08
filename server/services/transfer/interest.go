package transfer

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// ExpressInterest expresses interest in a piece of gear.
// Creates a new transfer if one doesn't exist, or adds the user to an existing group chat (for giveaways).
func (s *Service) ExpressInterest(
	ctx context.Context,
	req *connect.Request[api.ExpressInterestRequest],
) (*connect.Response[api.ExpressInterestResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	gearID := req.Msg.GearId
	logger.InfoContext(
		ctx, "expressing interest in gear",
		"gear_id", gearID,
	)

	// Validate gear
	gear, err := s.validateGearForInterest(ctx, gearID, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	// Find matching community and determine transfer type
	communityID, transferType, err := s.findMatchingCommunityAndType(ctx, gearID, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	// Check for an existing non-terminal transfer (idempotent dedupe).
	existingTransfers, err := s.storage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"recipient_id": authInfo.UserID,
		"community_id": communityID,
	}, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to query existing transfers for dedupe check",
			"gear_id", gearID,
			"community_id", communityID,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "ExpressInterest", err)
	}
	for _, row := range existingTransfers {
		t := row.(*models.Transfer)
		if t.State != models.TransferState_TRANSFER_STATE_COMPLETED &&
			t.State != models.TransferState_TRANSFER_STATE_CANCELLED {
			logger.InfoContext(
				ctx, "express interest is idempotent; returning existing transfer",
				"operation", "ExpressInterest",
				"transfer_id", t.Id,
				"state", t.State.String(),
				"dedupe", true,
			)
			existing, buildErr := s.buildTransfer(ctx, t)
			if buildErr != nil {
				logger.ErrorContext(
					ctx, "failed to build transfer item",
					"transfer_id", t.Id,
					"error", buildErr,
				)
				return nil, buildErr
			}
			return connect.NewResponse(&api.ExpressInterestResponse{
				Transfer: existing,
			}), nil
		}
	}

	// NOTE: Both loans and giveaways now create individual transfers per requester
	// They share the same gear conversation, but each has their own transfer record
	// This allows tracking individual interest/approval/completion states

	// Create new transfer (returns the stored transfer with its generated ID)
	newTransfer, err := s.createNewTransfer(ctx, gear, communityID, transferType, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	// Build the full transfer details for the response
	apiTransfer, err := s.buildTransfer(ctx, newTransfer)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to build transfer item",
			"transfer_id", newTransfer.Id,
			"error", err,
		)
		return nil, err
	}

	// Create watch entries: borrower/claimer watches the gear, owner marked unread.
	ws := storage.NewWatchStorage(s.storage)
	if err := ws.UpsertWatch(ctx, authInfo.UserID, models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, gearID); err != nil {
		logger.WarnContext(ctx, "failed to create watch for borrower", "error", err)
	}
	if err := ws.UpsertWatch(ctx, gear.OwnerId, models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, gearID); err != nil {
		logger.WarnContext(ctx, "failed to create watch for owner", "error", err)
	}
	if err := ws.MarkUnreadForWatchers(ctx, models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, gearID, authInfo.UserID); err != nil {
		logger.WarnContext(ctx, "failed to mark owner watch unread", "error", err)
	}

	return connect.NewResponse(&api.ExpressInterestResponse{
		Transfer: apiTransfer,
	}), nil
}

// validateGearForInterest validates the gear exists, is available, and user doesn't own it.
func (s *Service) validateGearForInterest(ctx context.Context, gearID, userID string) (*models.Gear, error) {
	logger := logging.LoggerWithContext(ctx)

	gear := &models.Gear{}
	err := s.storage.GetByID(ctx, gearID, gear)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to get gear",
			"gear_id", gearID,
			"error", err,
		)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
	}

	if gear.State != models.GearState_GEAR_STATE_AVAILABLE {
		logger.InfoContext(
			ctx, "gear unavailable for interest",
			"gear_id", gearID,
			"gear_state", gear.State.String(),
		)
		return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "gear_unavailable", "this item is currently unavailable", nil)
	}

	if gear.OwnerId == userID {
		logger.InfoContext(
			ctx, "user attempted to express interest in own gear",
			"gear_id", gearID,
			"user_id", userID,
		)
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot express interest in your own gear"))
	}

	return gear, nil
}

// createNewTransfer creates a new transfer and conversation for the first user expressing interest.
// For loans: adds user to the existing CommunityGear conversation and auto-transitions to RECIPIENT_SELECTED.
// For giveaways: creates a new transfer-specific conversation and remains in INTEREST_EXPRESSED.
// Returns the stored transfer (with generated ID) so callers don't need to re-fetch.
func (s *Service) createNewTransfer(ctx context.Context, gear *models.Gear, communityID string, transferType models.TransferType, userID string) (*models.Transfer, error) {
	logger := logging.LoggerWithContext(ctx)

	now := clock.UnixSec(ctx)

	// Determine initial state based on transfer type
	// Loans auto-approve (single recipient), giveaways require owner selection (multiple potential recipients)
	initialState := models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED
	if transferType == models.TransferType_TRANSFER_TYPE_LOAN {
		initialState = models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED
	}

	transfer := &models.Transfer{
		GearId:               gear.Id,
		OwnerId:              gear.OwnerId,
		RecipientId:          userID,
		TransferType:         transferType,
		State:                initialState,
		CommunityId:          communityID,
		LatestRequestUnixSec: now,
	}

	transferID, err := s.storage.Insert(ctx, transfer)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to insert transfer",
			"gear_id", gear.Id,
			"owner_id", gear.OwnerId,
			"recipient_id", userID,
			"transfer_type", transferType.String(),
			"initial_state", initialState.String(),
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "createNewTransfer", err)
	}
	transfer.Id = transferID

	if transferType == models.TransferType_TRANSFER_TYPE_LOAN {
		logger.InfoContext(
			ctx, "auto-approved loan transfer",
			"transfer_id", transferID,
			"gear_id", gear.Id,
			"state", models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED.String(),
		)
	}

	var conversationID string

	// For both loans and giveaways, use the CommunityGear conversation (perpetual, community-wide)
	switch transferType {
	case models.TransferType_TRANSFER_TYPE_LOAN:
		conversationID, err = s.getOrCreateGearConversation(ctx, gear.Id, communityID, userID)
		if err != nil {
			logger.ErrorContext(
				ctx, "failed to get or create gear conversation",
				"gear_id", gear.Id,
				"community_id", communityID,
				"error", err,
			)
			return nil, connecterr.Internal(ctx, "createNewTransfer", err, "detail", "transfer created but failed to get conversation")
		}

		logger.InfoContext(
			ctx, "created loan transfer and added user to gear conversation",
			"transfer_id", transferID,
			"conversation_id", conversationID,
		)
	case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
		// For giveaways, also use the gear conversation (same pattern as loans)
		conversationID, err = s.getOrCreateGiveawayGearConversation(ctx, gear.Id, communityID, userID)
		if err != nil {
			logger.ErrorContext(
				ctx, "failed to get or create gear conversation for giveaway",
				"gear_id", gear.Id,
				"community_id", communityID,
				"error", err,
			)
			return nil, connecterr.Internal(ctx, "createNewTransfer", err, "detail", "transfer created but failed to get conversation")
		}

		logger.InfoContext(
			ctx, "created giveaway transfer and added user to gear conversation",
			"transfer_id", transferID,
			"conversation_id", conversationID,
		)
	}

	// Record community event AFTER conversation exists (so notification can find conversation_id)
	// Use the actual state of the transfer (RECIPIENT_SELECTED for loans, INTEREST_EXPRESSED for giveaways)
	// Note: ObjectUserId is NOT set for RECIPIENT_SELECTED. The notification system looks up
	// the recipient from the transfer instead.
	actualState := transfer.State
	if _, err := s.recordCommunityEventForTransfer(ctx, transferID, gear.Id, actualState, transferType, userID, "", communityID, "", nil); err != nil {
		logger.ErrorContext(
			ctx, "failed to record community event for transfer",
			"transfer_id", transferID,
			"state", actualState.String(),
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "createNewTransfer", err, "detail", "failed to record community event")
	}

	return transfer, nil
}

// getOrCreateGearConversation gets the CommunityGear conversation or creates it if missing (lazy creation).
// Adds the borrower as a participant to the conversation.
func (s *Service) getOrCreateGearConversation(ctx context.Context, gearID, communityID, borrowerID string) (string, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "getOrCreateGearConversation",
		"gear_id", gearID,
		"community_id", communityID,
		"borrower_id", borrowerID,
	)

	// Query CommunityGear to find the conversation_id
	queryFields := map[string]any{
		"gear_id":      gearID,
		"community_id": communityID,
	}
	communityGears, err := s.storage.QueryByFields(ctx, queryFields, &models.CommunityGear{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community gear", "error", err)
		return "", err
	}

	if len(communityGears) == 0 {
		return "", fmt.Errorf("no community gear found for gear_id=%s, community_id=%s", gearID, communityID)
	}

	// Fetch the gear — conversation_id is now stored on the Gear model.
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gear); err != nil {
		logger.ErrorContext(ctx, "failed to get gear", "error", err)
		return "", fmt.Errorf("failed to get gear: %w", err)
	}

	conversationID := gear.ConversationId

	// Check if conversation exists, create if missing (lazy creation for legacy data)
	if conversationID == "" {
		logger.InfoContext(ctx, "conversation_id missing on gear, creating lazily")

		topic := &models.ConversationTopic{
			TopicId: &models.ConversationTopic_GearId{GearId: gearID},
		}

		chatConvStorage := storage.NewChatConversationStorage(s.storage)
		var participantIDs []string
		conversationID, participantIDs, err = chat.CreateOrGetConversation(ctx, s.storage, chatConvStorage, communityID, topic)
		if err != nil {
			return "", fmt.Errorf("failed to create conversation: %w", err)
		}

		// Persist conversation_id on the gear.
		gear.ConversationId = conversationID
		if err := s.storage.Update(ctx, gear); err != nil {
			logger.ErrorContext(ctx, "failed to update gear with conversation_id", "error", err)
			return "", fmt.Errorf("failed to update gear: %w", err)
		}

		logger.InfoContext(ctx, "created gear conversation lazily", "conversation_id", conversationID)

		alreadyParticipant := false
		for _, pid := range participantIDs {
			if pid == gear.OwnerId {
				alreadyParticipant = true
				break
			}
		}

		if !alreadyParticipant {
			if err := chat.AddParticipantToConversation(ctx, chatConvStorage, conversationID, gear.OwnerId); err != nil {
				logger.ErrorContext(ctx, "failed to add owner to gear conversation", "owner_id", gear.OwnerId, "error", err)
				return "", fmt.Errorf("failed to add owner to conversation: %w", err)
			}
		}
	} else {
		logger.DebugContext(ctx, "found existing gear conversation", "conversation_id", conversationID)
	}

	// Add user to the conversation as a convenience (but they could also join independently)
	if err := chat.AddParticipantToConversation(ctx, storage.NewChatConversationStorage(s.storage), conversationID, borrowerID); err != nil {
		logger.ErrorContext(ctx, "failed to add borrower to gear conversation", "error", err)
		return "", fmt.Errorf("failed to add borrower to conversation: %w", err)
	}

	// Post system message for the transfer action (expressing interest to borrow)
	if s.systemMessageWriter != nil {
		userName := s.getUserDisplayName(ctx, borrowerID)
		if err := s.systemMessageWriter.InsertLocalized(
			ctx,
			conversationID,
			borrowerID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED,
			chat.RequestedToBorrowMessage(userName),
		); err != nil {
			logger.ErrorContext(
				ctx, "failed to insert interest system message",
				"conversation_id", conversationID,
				"error", err,
			)
			return "", fmt.Errorf("failed to insert system message: %w", err)
		}
	}

	return conversationID, nil
}

// getOrCreateGiveawayGearConversation gets the CommunityGear conversation for giveaways or creates it if missing.
// Similar to getOrCreateGearConversation but uses JoinedText instead of RequestedToBorrowText.
func (s *Service) getOrCreateGiveawayGearConversation(ctx context.Context, gearID, communityID, claimerID string) (string, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "getOrCreateGiveawayGearConversation",
		"gear_id", gearID,
		"community_id", communityID,
		"claimer_id", claimerID,
	)

	// Query CommunityGear to find the conversation_id
	queryFields := map[string]any{
		"gear_id":      gearID,
		"community_id": communityID,
	}
	communityGears, err := s.storage.QueryByFields(ctx, queryFields, &models.CommunityGear{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community gear", "error", err)
		return "", err
	}

	if len(communityGears) == 0 {
		return "", fmt.Errorf("no community gear found for gear_id=%s, community_id=%s", gearID, communityID)
	}

	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gear); err != nil {
		logger.ErrorContext(ctx, "failed to get gear for conversation lookup", "error", err)
		return "", fmt.Errorf("failed to get gear: %w", err)
	}

	var conversationID string
	if gear.ConversationId != "" {
		conversationID = gear.ConversationId
		logger.DebugContext(ctx, "using existing gear conversation", "conversation_id", conversationID)
	} else {
		// Lazy creation for records with no conversation yet.
		logger.InfoContext(ctx, "conversation_id missing on gear, creating lazily")

		topic := &models.ConversationTopic{
			TopicId: &models.ConversationTopic_GearId{GearId: gearID},
		}

		chatConvStorage2 := storage.NewChatConversationStorage(s.storage)
		var participantIDs []string
		conversationID, participantIDs, err = chat.CreateOrGetConversation(ctx, s.storage, chatConvStorage2, communityID, topic)
		if err != nil {
			return "", fmt.Errorf("failed to create conversation: %w", err)
		}

		// Persist to Gear (canonical location).
		gear.ConversationId = conversationID
		if err := s.storage.Update(ctx, gear); err != nil {
			logger.ErrorContext(ctx, "failed to update gear with conversation_id", "error", err)
			return "", fmt.Errorf("failed to update gear: %w", err)
		}

		logger.InfoContext(ctx, "created gear conversation lazily", "conversation_id", conversationID)

		alreadyParticipant := false
		for _, pid := range participantIDs {
			if pid == gear.OwnerId {
				alreadyParticipant = true
				break
			}
		}

		if !alreadyParticipant {
			if err := chat.AddParticipantToConversation(ctx, chatConvStorage2, conversationID, gear.OwnerId); err != nil {
				logger.ErrorContext(ctx, "failed to add owner to gear conversation", "owner_id", gear.OwnerId, "error", err)
				return "", fmt.Errorf("failed to add owner to conversation: %w", err)
			}
		}
	}

	// Add user to the conversation as a convenience (but they could also join independently)
	if err := chat.AddParticipantToConversation(ctx, storage.NewChatConversationStorage(s.storage), conversationID, claimerID); err != nil {
		logger.ErrorContext(ctx, "failed to add claimer to gear conversation", "error", err)
		return "", fmt.Errorf("failed to add claimer to conversation: %w", err)
	}

	// Post system message for the transfer action (raising a hand for a giveaway)
	if s.systemMessageWriter != nil {
		userName := s.getUserDisplayName(ctx, claimerID)
		if err := s.systemMessageWriter.InsertLocalized(
			ctx,
			conversationID,
			claimerID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED,
			chat.RaisedHandMessage(userName),
		); err != nil {
			logger.ErrorContext(
				ctx, "failed to insert interest system message",
				"conversation_id", conversationID,
				"error", err,
			)
			return "", fmt.Errorf("failed to insert system message: %w", err)
		}
	}

	return conversationID, nil
}

// WithdrawInterest withdraws the user's interest in a transfer.
// For loans: archives the user's individual transfer request.
// For giveaways: removes user from group conversation.
func (s *Service) WithdrawInterest(
	ctx context.Context,
	req *connect.Request[api.WithdrawInterestRequest],
) (*connect.Response[api.WithdrawInterestResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.InfoContext(
		ctx, "withdrawing interest from transfer",
		"transfer_id", req.Msg.TransferId,
	)

	// Fetch the transfer
	transfer := &models.Transfer{}
	err = s.storage.GetByID(ctx, req.Msg.TransferId, transfer)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to get transfer",
			"transfer_id", req.Msg.TransferId,
			"error", err,
		)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("transfer not found"))
	}

	// Validate transfer is in INTEREST_EXPRESSED or RECIPIENT_SELECTED state
	// (loans auto-transition to RECIPIENT_SELECTED, so we allow withdrawal from either state)
	if transfer.State != models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED &&
		transfer.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("can only withdraw interest before transfer becomes active"))
	}

	// Cannot be the owner
	if transfer.OwnerId == authInfo.UserID {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("owner cannot withdraw interest"))
	}

	// Route to appropriate handler based on transfer type
	if transfer.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
		return s.withdrawFromGiveaway(ctx, transfer, authInfo.UserID)
	}

	return s.withdrawFromLoan(ctx, transfer, authInfo.UserID)
}

// withdrawFromLoan cancels a loan transfer when the borrower withdraws interest.
func (s *Service) withdrawFromLoan(ctx context.Context, transfer *models.Transfer, userID string) (*connect.Response[api.WithdrawInterestResponse], error) {
	logger := logging.LoggerWithContext(ctx)

	// Verify user is the recipient of this specific transfer
	if transfer.RecipientId != userID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("user is not the requester of this transfer"))
	}

	// Get the gear conversation ID from CommunityGear (loans use perpetual gear conversations)
	conversationID, err := s.getGearConversationID(ctx, transfer.GearId, transfer.CommunityId)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to get gear conversation",
			"transfer_id", transfer.Id,
			"gear_id", transfer.GearId,
			"community_id", transfer.CommunityId,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "withdrawFromLoan", err, "detail",

			// Cancel the transfer (withdrawal is treated as cancellation)
			"failed to get gear conversation")
	}

	transfer.State = models.TransferState_TRANSFER_STATE_CANCELLED
	err = s.storage.Update(ctx, transfer)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to update transfer",
			"transfer_id", transfer.Id,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "withdrawFromLoan", err)
	}

	logger.InfoContext(
		ctx, "user withdrew interest from loan transfer",
		"transfer_id", transfer.Id,
		"user_id", userID,
	)

	// Insert system message for withdrawal (user remains in conversation for loans)
	// NOTE: Unlike giveaways, we do NOT remove the user from the gear conversation.
	// The gear conversation is perpetual and community-wide for loans.
	if s.systemMessageWriter != nil {
		userName := s.getUserDisplayName(ctx, userID)
		if err := s.systemMessageWriter.InsertLocalized(
			ctx,
			conversationID,
			userID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_LEFT,
			chat.WithdrewInterestMessage(userName),
		); err != nil {
			logger.ErrorContext(
				ctx, "failed to insert withdrawal system message",
				"conversation_id", conversationID,
				"error", err,
			)
			return nil, connecterr.Internal(ctx, "withdrawFromLoan", err, "detail", "failed to insert system message")
		}
	}

	// Record community event for withdrawal
	if err := s.recordWithdrawalEvent(ctx, transfer, userID); err != nil {
		logger.ErrorContext(
			ctx, "failed to record withdrawal event for transfer",
			"transfer_id", transfer.Id,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "withdrawFromLoan", err, "detail", "failed to record withdrawal event")
	}

	return connect.NewResponse(&api.WithdrawInterestResponse{}), nil
}

// withdrawFromGiveaway removes a user from a giveaway's gear conversation.
// If the withdrawing user is the current recipient_id, reassigns to another participant.
// The transfer remains active even if all participants leave (owner can still see it).
func (s *Service) withdrawFromGiveaway(ctx context.Context, transfer *models.Transfer, userID string) (*connect.Response[api.WithdrawInterestResponse], error) {
	logger := logging.LoggerWithContext(ctx)

	// Get the gear conversation ID from CommunityGear (giveaways use gear conversations, not transfer conversations)
	conversationID, err := s.getGearConversationID(ctx, transfer.GearId, transfer.CommunityId)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to get gear conversation",
			"transfer_id", transfer.Id,
			"gear_id", transfer.GearId,
			"community_id", transfer.CommunityId,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "withdrawFromGiveaway", fmt.Errorf("no conversation found for giveaway"))
	}

	// Get the conversation
	conversation := &models.ChatConversation{}
	err = s.storage.GetByID(ctx, conversationID, conversation)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to get conversation",
			"conversation_id", conversationID,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "withdrawFromGiveaway", err)
	}

	// Verify user is a participant (not the owner)
	isParticipant := false
	for _, pid := range conversation.ParticipantIds {
		if pid == userID && pid != transfer.OwnerId {
			isParticipant = true
			break
		}
	}

	if !isParticipant {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("user is not a participant in this giveaway"))
	}

	// Remove user from conversation using shared library
	err = chat.RemoveParticipantFromConversation(ctx, storage.NewChatConversationStorage(s.storage), conversation.Id, userID)
	if err != nil {
		logger.ErrorContext(
			ctx, "failed to remove participant from conversation",
			"conversation_id", conversation.Id,
			"user_id", userID,
			"error", err,
		)
		return nil, err
	}

	// Insert system message for withdrawal
	if s.systemMessageWriter != nil {
		userName := s.getUserDisplayName(ctx, userID)
		if err := s.systemMessageWriter.InsertLocalized(
			ctx,
			conversation.Id,
			userID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_LEFT,
			chat.WithdrewInterestMessage(userName),
		); err != nil {
			logger.ErrorContext(
				ctx, "failed to insert withdrawal system message",
				"conversation_id", conversation.Id,
				"error", err,
			)
			return nil, connecterr.Internal(ctx, "withdrawFromGiveaway", err, "detail", "failed to insert system message")
		}
	}

	// Handle recipient reassignment if the withdrawing user was the recipient_id
	if transfer.RecipientId == userID {
		if err := s.reassignGiveawayRecipient(ctx, transfer, conversation.Id, userID); err != nil {
			logger.ErrorContext(
				ctx, "failed to reassign recipient",
				"transfer_id", transfer.Id,
				"error", err,
			)
			return nil, err
		}
	}

	logger.InfoContext(
		ctx, "user withdrew interest from giveaway transfer",
		"transfer_id", transfer.Id,
		"user_id", userID,
	)

	// Record community event for withdrawal
	if err := s.recordWithdrawalEvent(ctx, transfer, userID); err != nil {
		logger.ErrorContext(
			ctx, "failed to record withdrawal event for transfer",
			"transfer_id", transfer.Id,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "withdrawFromGiveaway", err, "detail", "failed to record withdrawal event")
	}

	return connect.NewResponse(&api.WithdrawInterestResponse{}), nil
}

// reassignGiveawayRecipient updates the transfer's recipient_id when the current recipient withdraws.
// Picks the next available participant (excluding owner), or clears recipient if none remain.
func (s *Service) reassignGiveawayRecipient(ctx context.Context, transfer *models.Transfer, conversationID, withdrawingUserID string) error {
	logger := logging.LoggerWithContext(ctx)

	// Re-fetch conversation to get updated participant list
	conversation := &models.ChatConversation{}
	err := s.storage.GetByID(ctx, conversationID, conversation)
	if err != nil {
		return connecterr.Internal(ctx, "reassignGiveawayRecipient", err)
	}

	// Find a new recipient (any participant except the owner)
	var newRecipientID string
	for _, pid := range conversation.ParticipantIds {
		if pid != transfer.OwnerId && pid != withdrawingUserID {
			newRecipientID = pid
			break
		}
	}

	// Update transfer with new recipient (empty string if no participants remain)
	transfer.RecipientId = newRecipientID
	err = s.storage.Update(ctx, transfer)
	if err != nil {
		return connecterr.Internal(ctx, "reassignGiveawayRecipient", err)
	}

	if newRecipientID == "" {
		logger.InfoContext(
			ctx, "giveaway transfer has no remaining participants, cleared recipient",
			"transfer_id", transfer.Id,
		)
	} else {
		logger.InfoContext(
			ctx, "giveaway transfer recipient reassigned",
			"transfer_id", transfer.Id,
			"previous_recipient_id", withdrawingUserID,
			"new_recipient_id", newRecipientID,
		)
	}

	return nil
}

// recordWithdrawalEvent records a community event when a user withdraws interest from a transfer.
func (s *Service) recordWithdrawalEvent(ctx context.Context, transfer *models.Transfer, userID string) error {
	_, err := s.bus.Publish(ctx, &models.CommunityEvent{
		CommunityId:  transfer.CommunityId,
		EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_WITHDRAWN,
		ActorId:      userID,
		GearId:       transfer.GearId,
		Topic:        &models.CommunityEvent_TransferId{TransferId: transfer.Id},
		TransferType: transfer.TransferType,
	})
	return err
}
