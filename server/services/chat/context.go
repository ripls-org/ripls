package chat

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	chatlib "go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
)

// GetConversationContext retrieves context information for a conversation.
// This includes the topic title, subtitle, image, and message count for display in UI.
func (s *Service) GetConversationContext(
	ctx context.Context,
	req *connect.Request[api.GetConversationContextRequest],
) (*connect.Response[api.GetConversationContextResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"conversation_id", req.Msg.ConversationId,
	)

	logger.DebugContext(ctx, "getting conversation context")

	// Verify user has access to this conversation (participant or community member)
	conversation, err := s.requireConversationAccess(ctx, req.Msg.ConversationId, authInfo.UserID)
	if err != nil {
		logger.WarnContext(ctx, "conversation access denied", "error", err)
		return nil, err
	}

	// Get message count
	messageCount, err := chatlib.GetConversationMessageCount(ctx, s.storage, req.Msg.ConversationId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get message count", "error", err)
		return nil, connecterr.Internal(ctx, "GetConversationContext", err, "detail",

			// Build response based on topic type
			"failed to get message count")
	}

	response := &api.ConversationContext{
		ConversationId: req.Msg.ConversationId,
		MessageCount:   messageCount,
	}

	// Determine topic type and fetch details
	convTopic := conversation.GetTopic()
	logger.DebugContext(ctx, "conversation topic type",
		"conversation_id", req.Msg.ConversationId,
		"topic_type", fmt.Sprintf("%T", convTopic.GetTopicId()),
	)

	switch topic := convTopic.GetTopicId().(type) {
	case *models.ConversationTopic_TransferId:
		if err := s.populateTransferContext(ctx, topic.TransferId, response); err != nil {
			logger.ErrorContext(ctx, "failed to populate transfer context", "error", err)
			return nil, err
		}

	case *models.ConversationTopic_RequestId:
		if err := s.populateRequestContext(ctx, topic.RequestId, response); err != nil {
			logger.ErrorContext(ctx, "failed to populate request context", "error", err)
			return nil, err
		}

	case *models.ConversationTopic_ExperienceId:
		if err := s.populateExperienceContext(ctx, topic.ExperienceId, response); err != nil {
			logger.ErrorContext(ctx, "failed to populate experience context", "error", err)
			return nil, err
		}

	case *models.ConversationTopic_GearId:
		if err := s.populateGearContext(ctx, topic.GearId, conversation, authInfo.UserID, response); err != nil {
			logger.ErrorContext(ctx, "failed to populate gear context", "error", err)
			return nil, err
		}

	case *models.ConversationTopic_CommunityId:
		if err := s.populateCommunityContext(ctx, topic.CommunityId, response); err != nil {
			logger.ErrorContext(ctx, "failed to populate community context", "error", err)
			return nil, err
		}

	default:
		logger.ErrorContext(ctx, "unknown conversation topic type")
		return nil, connecterr.Internal(ctx, "GetConversationContext", fmt.Errorf("unknown conversation topic type"))
	}

	logger.InfoContext(ctx, "retrieved conversation context",
		"topic_title", response.TopicTitle,
		"message_count", response.MessageCount,
	)

	return connect.NewResponse(&api.GetConversationContextResponse{
		Context: response,
	}), nil
}

// populateTransferContext populates context for a transfer conversation.
func (s *Service) populateTransferContext(ctx context.Context, transferID string, response *api.ConversationContext) error {
	transfer := &models.Transfer{}
	if err := s.storage.GetByID(ctx, transferID, transfer); err != nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("transfer not found"))
	}

	// Get gear details
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, transfer.GearId, gear); err != nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
	}

	// Set transfer type enum for client to handle i18n
	response.TransferType = api.TransferType(transfer.TransferType)
	response.TopicTitle = gear.Name
	response.TopicSubtitle = "" // Don't show owner on top line
	response.Topic = &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: transferID}}

	// Get first media if available
	if len(gear.MediaIds) > 0 {
		response.TopicImageMediaId = gear.MediaIds[0]
	}

	return nil
}

// populateRequestContext populates context for a request conversation.
func (s *Service) populateRequestContext(ctx context.Context, requestID string, response *api.ConversationContext) error {
	request := &models.Request{}
	if err := s.storage.GetByID(ctx, requestID, request); err != nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
	}

	response.TopicTitle = request.Title
	response.TopicSubtitle = "" // Don't show requester on top line
	response.Topic = &api.ConversationTopic{TopicId: &api.ConversationTopic_RequestId{RequestId: requestID}}

	// Get first media if available
	if len(request.MediaIds) > 0 {
		response.TopicImageMediaId = request.MediaIds[0]
	}

	return nil
}

// populateExperienceContext populates context for an experience conversation.
func (s *Service) populateExperienceContext(ctx context.Context, experienceID string, response *api.ConversationContext) error {
	experience := &models.Experience{}
	if err := s.storage.GetByID(ctx, experienceID, experience); err != nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("experience not found"))
	}

	response.TopicTitle = experience.Name
	response.TopicSubtitle = "" // Don't show host on top line
	response.Topic = &api.ConversationTopic{TopicId: &api.ConversationTopic_ExperienceId{ExperienceId: experienceID}}

	// Get first media if available
	if len(experience.MediaIds) > 0 {
		response.TopicImageMediaId = experience.MediaIds[0]
	}

	return nil
}

// populateGearContext populates context for a gear conversation.
func (s *Service) populateGearContext(ctx context.Context, gearID string, conversation *models.ChatConversation, userID string, response *api.ConversationContext) error {
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gear); err != nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
	}

	// Query CommunityGear to determine availability (loan vs giveaway)
	queryFields := map[string]any{
		"gear_id":      gearID,
		"community_id": conversation.CommunityId,
	}
	communityGears, err := s.storage.QueryByFields(ctx, queryFields, &models.CommunityGear{})
	if err == nil && len(communityGears) > 0 {
		communityGear := communityGears[0].(*models.CommunityGear)
		// Set availability enum for client to handle i18n
		response.Availability = api.Availability(communityGear.Availability)
	}

	response.TopicTitle = gear.Name
	response.TopicDescription = gear.Description
	response.TopicSubtitle = "" // Don't show owner on top line
	response.Topic = &api.ConversationTopic{TopicId: &api.ConversationTopic_GearId{GearId: gearID}}

	// Get first media if available
	if len(gear.MediaIds) > 0 {
		response.TopicImageMediaId = gear.MediaIds[0]
	}

	// Add transfer context for loan workflows
	transferContext, err := s.getGearTransferContext(ctx, gearID, userID, gear.OwnerId)
	if err != nil {
		logger := logging.LoggerWithContext(ctx)
		logger.ErrorContext(ctx, "failed to get transfer context", "error", err)
		return fmt.Errorf("failed to get transfer context: %w", err)
	}
	if transferContext != nil {
		logger := logging.LoggerWithContext(ctx)
		logger.InfoContext(ctx, "populated gear transfer context",
			"has_user_transfer", transferContext.UserTransfer != nil,
			"pending_requests_count", len(transferContext.PendingRequests),
			"available_actions_count", len(transferContext.AvailableActions),
		)
		response.GearTransferContext = transferContext
	}

	return nil
}

// getGearTransferContext retrieves transfer context for a gear conversation.
// This includes the user's transfer (if any), available actions, and pending requests (for owners).
func (s *Service) getGearTransferContext(ctx context.Context, gearID, userID, ownerID string) (*api.GearTransferContext, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "getGearTransferContext",
		"gear_id", gearID,
		"user_id", userID,
	)

	transferContext := &api.GearTransferContext{}
	isOwner := userID == ownerID

	logger.DebugContext(ctx, "determining ownership",
		"user_id", userID,
		"owner_id", ownerID,
		"is_owner", isOwner,
	)

	// Find the most relevant non-terminal transfer for this gear.
	//
	// For non-owners: queries by recipient_id to find the user's own transfer.
	// For owners: queries all transfers for the gear and picks the most
	// advanced one (ACTIVE > RECIPIENT_SELECTED > INTEREST_EXPRESSED). This
	// lets the owner see the current loan phase and act on it from the chat.
	{
		queryFields := map[string]any{"gear_id": gearID}
		if !isOwner {
			queryFields["recipient_id"] = userID
		}

		transfers, err := s.storage.QueryByFields(ctx, queryFields, &models.Transfer{})
		if err != nil {
			logger.ErrorContext(ctx, "failed to query transfers", "error", err)
			return nil, fmt.Errorf("failed to query transfers: %w", err)
		}

		if len(transfers) > 0 {
			logger.DebugContext(ctx, "found transfers for gear",
				"transfer_count", len(transfers),
				"is_owner", isOwner,
			)

			// Pick the most advanced non-terminal transfer (ACTIVE > SELECTED > EXPRESSED).
			var best *models.Transfer
			for _, t := range transfers {
				transfer := t.(*models.Transfer)
				if transfer.State == models.TransferState_TRANSFER_STATE_COMPLETED ||
					transfer.State == models.TransferState_TRANSFER_STATE_CANCELLED {
					continue
				}
				if best == nil || transfer.State > best.State {
					best = transfer
				}
			}

			if best != nil {
				transferContext.UserTransfer = convertTransferToAPI(best)
				logger.DebugContext(ctx, "selected non-terminal transfer",
					"transfer_id", best.Id,
					"state", best.State.String(),
				)
			}
		}
	}

	// Get pending requests (visible to all users per Decision 1)
	// Include both INTEREST_EXPRESSED and RECIPIENT_SELECTED states so all users
	// can see selected transfers (important for giveaway "SELECTED" badges)
	pendingTransfers, err := s.storage.QueryByFields(ctx, map[string]any{
		"gear_id": gearID,
		"state":   int32(models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED),
	}, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query pending transfers", "error", err)
		return nil, fmt.Errorf("failed to query pending transfers: %w", err)
	}

	for _, t := range pendingTransfers {
		transfer := t.(*models.Transfer)

		// Fetch borrower user info
		borrower, err := services.FetchAPIUser(ctx, s.storage, transfer.RecipientId)
		if err != nil {
			logger.ErrorContext(ctx, "failed to fetch borrower", "recipient_id", transfer.RecipientId, "error", err)
			return nil, fmt.Errorf("failed to fetch borrower %s: %w", transfer.RecipientId, err)
		}

		transferContext.PendingRequests = append(transferContext.PendingRequests, &api.TransferRequest{
			TransferId:         transfer.Id,
			Borrower:           borrower,
			RequestedAtUnixSec: transfer.LatestRequestUnixSec,
		})
	}

	// Also include RECIPIENT_SELECTED transfers so all users can see them
	// (important for showing "SELECTED" badge to non-owners)
	selectedTransfers, err := s.storage.QueryByFields(ctx, map[string]any{
		"gear_id": gearID,
		"state":   int32(models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED),
	}, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query selected transfers", "error", err)
		return nil, fmt.Errorf("failed to query selected transfers: %w", err)
	}

	for _, t := range selectedTransfers {
		transfer := t.(*models.Transfer)

		// Fetch borrower user info
		borrower, err := services.FetchAPIUser(ctx, s.storage, transfer.RecipientId)
		if err != nil {
			logger.ErrorContext(ctx, "failed to fetch borrower", "recipient_id", transfer.RecipientId, "error", err)
			return nil, fmt.Errorf("failed to fetch borrower %s: %w", transfer.RecipientId, err)
		}

		req := &api.TransferRequest{
			TransferId:         transfer.Id,
			Borrower:           borrower,
			RequestedAtUnixSec: transfer.LatestRequestUnixSec,
		}
		transferContext.PendingRequests = append(transferContext.PendingRequests, req)
		// Expose the selected recipient separately so clients can display a badge
		// without iterating pending_requests by state.
		transferContext.SelectedRecipient = req
	}

	// Determine available actions based on state and role
	transferContext.AvailableActions = s.determineTransferActions(isOwner, transferContext.UserTransfer, len(transferContext.PendingRequests) > 0)

	// Log available actions for debugging
	actionNames := make([]string, len(transferContext.AvailableActions))
	for i, action := range transferContext.AvailableActions {
		actionNames[i] = action.String()
	}

	// Set ownership flag for client UI
	transferContext.IsOwner = isOwner

	// Query for completed and cancelled transfers to determine terminal phases.
	completedTransfers, err := s.storage.QueryByFields(ctx, map[string]any{
		"gear_id": gearID,
		"state":   int32(models.TransferState_TRANSFER_STATE_COMPLETED),
	}, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query completed transfers", "error", err)
		return nil, fmt.Errorf("failed to query completed transfers: %w", err)
	}

	cancelledTransfers, err := s.storage.QueryByFields(ctx, map[string]any{
		"gear_id": gearID,
		"state":   int32(models.TransferState_TRANSFER_STATE_CANCELLED),
	}, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query cancelled transfers", "error", err)
		return nil, fmt.Errorf("failed to query cancelled transfers: %w", err)
	}

	// Compute server-side giveaway phase so non-selected participants can
	// correctly determine the global state of the giveaway.
	transferContext.OverallPhase = computeGiveawayPhase(
		len(pendingTransfers),
		len(selectedTransfers),
		len(completedTransfers),
		len(cancelledTransfers),
	)

	logger.DebugContext(ctx, "built gear transfer context",
		"is_owner", isOwner,
		"has_user_transfer", transferContext.UserTransfer != nil,
		"pending_requests_count", len(transferContext.PendingRequests),
		"available_actions_count", len(transferContext.AvailableActions),
		"available_actions", actionNames,
	)

	return transferContext, nil
}

// determineTransferActions determines which workflow actions are available to the user.
func (s *Service) determineTransferActions(isOwner bool, userTransfer *api.Transfer, hasPendingRequests bool) []api.TransferAction {
	var actions []api.TransferAction

	if isOwner {
		// Owner actions based on transfer state
		if hasPendingRequests {
			// Has pending requests - can approve/decline
			actions = append(actions, api.TransferAction_TRANSFER_ACTION_SELECT_RECIPIENT)
		}

		if userTransfer != nil {
			switch userTransfer.State {
			case api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_START_LOAN)
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_CANCEL)
			case api.TransferState_TRANSFER_STATE_ACTIVE:
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_COMPLETE)
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_CANCEL)
			}
		}
	} else {
		// Borrower actions based on transfer state
		if userTransfer == nil {
			// No transfer - can express interest
			actions = append(actions, api.TransferAction_TRANSFER_ACTION_EXPRESS_INTEREST)
		} else {
			switch userTransfer.State {
			case api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED:
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_WITHDRAW_INTEREST)
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_CANCEL)
			case api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_CANCEL)
			case api.TransferState_TRANSFER_STATE_ACTIVE:
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_COMPLETE)
				actions = append(actions, api.TransferAction_TRANSFER_ACTION_CANCEL)
			}
		}
	}

	return actions
}

// computeGiveawayPhase derives the overall giveaway phase from transfer state counts.
// Priority order: COMPLETED > RECIPIENT_SELECTED > CANCELLED > OPEN.
func computeGiveawayPhase(pendingCount, selectedCount, completedCount, cancelledCount int) api.GiveawayPhase {
	if completedCount > 0 {
		return api.GiveawayPhase_GIVEAWAY_PHASE_COMPLETED
	}
	if selectedCount > 0 {
		return api.GiveawayPhase_GIVEAWAY_PHASE_RECIPIENT_SELECTED
	}
	if cancelledCount > 0 && pendingCount == 0 {
		return api.GiveawayPhase_GIVEAWAY_PHASE_CANCELLED
	}
	return api.GiveawayPhase_GIVEAWAY_PHASE_OPEN
}

// populateCommunityContext populates context for a community-wide conversation.
func (s *Service) populateCommunityContext(ctx context.Context, communityID string, response *api.ConversationContext) error {
	community := &models.Community{}
	if err := s.storage.GetByID(ctx, communityID, community); err != nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("community not found"))
	}

	response.TopicTitle = community.Name
	response.Topic = &api.ConversationTopic{TopicId: &api.ConversationTopic_CommunityId{CommunityId: communityID}}

	if len(community.MediaIds) > 0 {
		response.TopicImageMediaId = community.MediaIds[0]
	}

	return nil
}

// convertTransferToAPI converts a models.Transfer to api.Transfer.
// Note: This creates a minimal transfer object suitable for conversation context.
// For full transfer details, use the transfer service's conversion functions.
func convertTransferToAPI(transfer *models.Transfer) *api.Transfer {
	apiTransfer := &api.Transfer{
		Id:           transfer.Id,
		GearId:       transfer.GearId,
		State:        api.TransferState(transfer.State),
		TransferType: api.TransferType(transfer.TransferType),
		// Note: Owner and Recipient are not populated here to avoid extra DB queries.
		// The UI can fetch these separately if needed.
	}
	if transfer.EstimatedPickupUnixSec != nil {
		apiTransfer.EstimatedPickupUnixSec = transfer.EstimatedPickupUnixSec
	}
	if transfer.ActualPickupUnixSec != nil {
		apiTransfer.ActualPickupUnixSec = transfer.ActualPickupUnixSec
	}
	if transfer.LoanDurationDays != nil {
		apiTransfer.LoanDurationDays = transfer.LoanDurationDays
	}
	if transfer.ActualReturnUnixSec != nil {
		apiTransfer.ActualReturnUnixSec = transfer.ActualReturnUnixSec
	}
	return apiTransfer
}
