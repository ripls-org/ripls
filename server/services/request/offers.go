package request

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
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// OfferToFulfill expresses interest in fulfilling a request.
// Creates a RequestOffer record and adds the offerer to the community-specific conversation.
func (s *Service) OfferToFulfill(
	ctx context.Context,
	req *connect.Request[api.OfferToFulfillRequest],
) (*connect.Response[api.OfferToFulfillResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"target_request_id", req.Msg.RequestId,
		"community_id", req.Msg.CommunityId,
	)

	logger.Info("offering to fulfill request")

	// Validate required fields with clear error messages
	if req.Msg.CommunityId == "" {
		logger.Warn("missing required community_id")
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required to offer help on a request"))
	}

	if req.Msg.RequestId == "" {
		logger.Warn("missing required request_id")
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("request_id is required"))
	}

	// Fetch the request
	requestStored := &models.Request{}
	err = s.storage.GetByID(ctx, req.Msg.RequestId, requestStored)
	if err != nil {
		logger.Error("failed to get request", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
	}

	// Validate request is in appropriate state
	if requestStored.State != models.RequestState_REQUEST_STATE_ACTIVE && requestStored.State != models.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
		return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "request_not_accepting_offers", "this request is not accepting offers", nil)
	}

	// Verify user is not the requester
	if requestStored.RequesterId == authInfo.UserID {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot offer to fulfill your own request"))
	}

	// Verify community is active and caller is a member (single round-trip).
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Verify request is shared with this community
	communityRequests, err := s.storage.QueryByFields(ctx, map[string]any{
		"request_id":   req.Msg.RequestId,
		"community_id": req.Msg.CommunityId,
	}, &models.CommunityRequest{})
	if err != nil {
		logger.Error("failed to query community request", "error", err)
		return nil, connecterr.Internal(ctx, "OfferToFulfill", err)
	}
	if len(communityRequests) == 0 {
		logger.WarnContext(ctx, "offer community mismatch: request not shared with community",
			"mismatch", "community")
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found in this community"))
	}
	// Check if user has already offered in this community
	existingOffers, err := s.storage.QueryByFields(ctx, map[string]any{
		"request_id":   req.Msg.RequestId,
		"user_id":      authInfo.UserID,
		"community_id": req.Msg.CommunityId,
		"withdrawn":    false,
	}, &models.RequestOffer{})
	if err != nil {
		logger.Error("failed to query existing offers", "error", err)
		return nil, connecterr.Internal(ctx, "OfferToFulfill", err)
	}

	alreadyOffered := len(existingOffers) > 0

	// Create RequestOffer record if not already offered
	if !alreadyOffered {
		offer := &models.RequestOffer{
			RequestId:        req.Msg.RequestId,
			UserId:           authInfo.UserID,
			CommunityId:      req.Msg.CommunityId,
			Message:          "", // Client can add message later
			CreatedAtUnixSec: clock.UnixSec(ctx),
			Withdrawn:        false,
		}
		_, err = s.storage.Insert(ctx, offer)
		if err != nil {
			logger.Error("failed to create request offer", "error", err)
			return nil, connecterr.Internal(ctx, "OfferToFulfill", err)
		}

		// Transition request to OFFERS_RECEIVED if this is the first offer (across all communities)
		if requestStored.State == models.RequestState_REQUEST_STATE_ACTIVE {
			requestStored.State = models.RequestState_REQUEST_STATE_OFFERS_RECEIVED
			logger.Info("transitioning request to OFFERS_RECEIVED state")
			if err := s.storage.Update(ctx, requestStored); err != nil {
				logger.Error("failed to update request state", "error", err)
				return nil, connecterr.Internal(ctx, "OfferToFulfill", err)
			}
		}
	}

	conversationID := requestStored.ConversationId
	if err := chat.AddParticipantToConversation(ctx, storage.NewChatConversationStorage(s.storage), conversationID, authInfo.UserID); err != nil {
		logger.ErrorContext(ctx, "failed to add offerer to conversation", "error", err)
		return nil, connecterr.Internal(ctx, "OfferToFulfill", err, "detail",

			// Record community event for each offer made
			"failed to add offerer to conversation")
	}

	if err := s.recordOfferMadeEvent(ctx, req.Msg.RequestId, authInfo.UserID, req.Msg.CommunityId); err != nil {
		logger.Error("failed to record offer made event", "error", err)
		return nil, err
	}

	// Send system message for offer (only if not already offered)
	if !alreadyOffered && s.systemMessageWriter != nil {
		displayName := s.getUserDisplayName(ctx, authInfo.UserID)
		if err := s.systemMessageWriter.InsertLocalized(ctx, conversationID, authInfo.UserID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_OFFERED, chat.OfferedToHelpMessage(displayName)); err != nil {
			logger.ErrorContext(ctx, "failed to write OFFERED system message", "error", err)
			return nil, connecterr.Internal(ctx, "OfferToFulfill", err, "detail", "failed to write OFFERED system message")
		}
	}

	logger.Info("user added to request conversation", "conversation_id", conversationID)

	// Rebuild social footprint with the updated offer count as the group size.
	// Group size = 1 (requestor) + number of active offers across all communities.
	// Only the SF field is updated; money, carbon, and time dimensions are unchanged.
	if !alreadyOffered && s.estimatorCfg != nil && requestStored.ImpactEstimate != nil {
		allOffers, offerErr := s.storage.QueryByFields(ctx, map[string]any{
			"request_id": req.Msg.RequestId,
			"withdrawn":  false,
		}, &models.RequestOffer{})
		if offerErr != nil {
			logger.WarnContext(ctx, "failed to count offers for QT group size update", "error", offerErr)
		} else {
			offerCount := len(allOffers)
			groupSize := int32(1 + offerCount) // 1 requestor + offerers
			reasoningMsg := fmt.Sprintf("Group size from requestor + %d offer(s)", offerCount)
			newQT := impact_metrics.RebuildQualityTimeForGroupSize(
				estimator.TransactionRequestFulfilled,
				groupSize,
				estimator.RoleMutual,
				reasoningMsg,
				s.estimatorCfg,
			)
			if newQT != nil {
				requestStored.ImpactEstimate = impact_metrics.UpdateQualityTime(requestStored.ImpactEstimate, newQT)
				if updateErr := s.storage.Update(ctx, requestStored); updateErr != nil {
					logger.WarnContext(ctx, "failed to persist QT group size update after offer", "error", updateErr)
				} else {
					logger.DebugContext(ctx, "updated QT group size after offer",
						"group_size", groupSize, "quality_time_minutes", newQT.GetQualityTimeMinutes().GetMean())
				}
			}
		}
	}

	// Helper watches the request; requester marked unread.
	ws := storage.NewWatchStorage(s.storage)
	if err := ws.UpsertWatch(ctx, authInfo.UserID, models.WatchedItemType_WATCHED_ITEM_TYPE_REQUEST, requestStored.Id); err != nil {
		logger.Warn("failed to create watch for offer helper", "error", err)
	}
	if err := ws.UpsertWatch(ctx, requestStored.RequesterId, models.WatchedItemType_WATCHED_ITEM_TYPE_REQUEST, requestStored.Id); err != nil {
		logger.Warn("failed to create watch for request owner", "error", err)
	}
	if err := ws.MarkUnreadForWatchers(ctx, models.WatchedItemType_WATCHED_ITEM_TYPE_REQUEST, requestStored.Id, authInfo.UserID); err != nil {
		logger.Warn("failed to mark requester watch unread on offer", "error", err)
	}

	// Build the full request details for the response, using the community-specific conversation
	apiRequest, err := s.buildRequest(ctx, requestStored, req.Msg.CommunityId)
	if err != nil {
		logger.Error("failed to build request item", "error", err)
		return nil, err
	}

	return connect.NewResponse(&api.OfferToFulfillResponse{
		Request: apiRequest,
	}), nil
}

// WithdrawOffer withdraws the user's offer to fulfill a request.
// This marks the RequestOffer as withdrawn and removes the user from the conversation.
// If this was the last active offerer, the request returns to ACTIVE state.
func (s *Service) WithdrawOffer(
	ctx context.Context,
	req *connect.Request[api.WithdrawOfferRequest],
) (*connect.Response[api.WithdrawOfferResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"target_request_id", req.Msg.RequestId,
		"community_id", req.Msg.CommunityId,
	)

	logger.Info("withdrawing offer from request")

	// Fetch the request
	requestStored := &models.Request{}
	err = s.storage.GetByID(ctx, req.Msg.RequestId, requestStored)
	if err != nil {
		logger.Error("failed to get request", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
	}

	// Validate request is in appropriate state for withdrawal
	if requestStored.State != models.RequestState_REQUEST_STATE_ACTIVE && requestStored.State != models.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
		return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "request_invalid_state_for_withdraw_offer", "can only withdraw offers from active requests", nil)
	}

	// Verify user is not the requester
	if requestStored.RequesterId == authInfo.UserID {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("requester cannot withdraw offer"))
	}

	// Find the user's offer for this request in this community
	offers, err := s.storage.QueryByFields(ctx, map[string]any{
		"request_id":   req.Msg.RequestId,
		"user_id":      authInfo.UserID,
		"community_id": req.Msg.CommunityId,
		"withdrawn":    false,
	}, &models.RequestOffer{})
	if err != nil {
		logger.Error("failed to query offers", "error", err)
		return nil, connecterr.Internal(ctx, "WithdrawOffer", err)
	}
	if len(offers) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user has not offered to fulfill this request in this community"))
	}

	// Mark the offer as withdrawn
	offer := offers[0].(*models.RequestOffer)
	offer.Withdrawn = true
	if err := s.storage.Update(ctx, offer); err != nil {
		logger.Error("failed to update offer", "error", err)
		return nil, connecterr.Internal(ctx, "WithdrawOffer", err)
	}

	// Check if there are any remaining active offers (across all communities)
	allOffers, err := s.storage.QueryByFields(ctx, map[string]any{
		"request_id": req.Msg.RequestId,
		"withdrawn":  false,
	}, &models.RequestOffer{})
	if err != nil {
		logger.Warn("failed to count remaining offers", "error", err)
	} else if len(allOffers) == 0 {
		// Last offer was withdrawn, return request to ACTIVE state
		requestStored.State = models.RequestState_REQUEST_STATE_ACTIVE
		if err := s.storage.Update(ctx, requestStored); err != nil {
			logger.Error("failed to update request state", "error", err)
		} else {
			logger.Info("last offerer withdrew, returned request to ACTIVE state")
		}
	}

	// Get the conversation for system message and participant removal.
	conversationID := requestStored.ConversationId
	if conversationID != "" {
		// Send system message before removing from conversation.
		if s.systemMessageWriter != nil {
			displayName := s.getUserDisplayName(ctx, authInfo.UserID)
			if err := s.systemMessageWriter.InsertLocalized(ctx, conversationID, authInfo.UserID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_LEFT, chat.WithdrewOfferMessage(displayName)); err != nil {
				logger.Warn("failed to write LEFT system message", "error", err)
			}
		}
		if err := chat.RemoveParticipantFromConversation(ctx, storage.NewChatConversationStorage(s.storage), conversationID, authInfo.UserID); err != nil {
			logger.Warn("failed to remove user from conversation", "error", err)
		}
	}

	// Record community event for withdrawal
	if err := s.recordOfferWithdrawnEvent(ctx, req.Msg.RequestId, authInfo.UserID, req.Msg.CommunityId); err != nil {
		logger.Warn("failed to record offer withdrawn event", "error", err)
	}

	logger.Info("user withdrew offer from request")

	return connect.NewResponse(&api.WithdrawOfferResponse{}), nil
}
