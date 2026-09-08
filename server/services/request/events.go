package request

import (
	"context"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// recordCommunityEventForRequest records a community event for a request state change.
// systemChatMessageID is optional and, when non-empty, is persisted on the CommunityEvent so
// Undo* RPCs can soft-delete the corresponding system chat message via direct read.
// undoData is optional and carries the prior-state snapshot required to reverse this action.
// Returns the inserted event's id (or the empty string when the state has no associated event type).
func (s *Service) recordCommunityEventForRequest(
	ctx context.Context,
	requestID string,
	requestState models.RequestState,
	actorID, communityID, systemChatMessageID string,
	undoData *models.UndoData,
) (string, error) {
	// Determine event type based on request state
	var eventType models.CommunityEventType
	switch requestState {
	case models.RequestState_REQUEST_STATE_ACTIVE:
		eventType = models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED
	case models.RequestState_REQUEST_STATE_OFFERS_RECEIVED:
		// This event is recorded when the first offer transitions the state
		// Individual offers are recorded via recordOfferMadeEvent
		return "", nil // Don't record a separate state transition event
	case models.RequestState_REQUEST_STATE_FULFILLED:
		eventType = models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED
	case models.RequestState_REQUEST_STATE_CANCELLED:
		eventType = models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED
	default:
		return "", nil // Unknown state, nothing to log
	}

	event := &models.CommunityEvent{
		CommunityId: communityID,
		EventType:   eventType,
		ActorId:     actorID,
		Topic:       &models.CommunityEvent_RequestId{RequestId: requestID},
		UndoData:    undoData,
	}
	if systemChatMessageID != "" {
		event.SystemChatMessageId = &systemChatMessageID
	}
	if _, err := s.bus.Publish(ctx, event); err != nil {
		logger := logging.LoggerWithContext(ctx).With(
			"community_id", communityID,
			"target_request_id", requestID,
		)
		logger.Error("failed to record event for community", "error", err)
		return "", err
	}

	return event.Id, nil
}

// recordOfferMadeEvent records a community event when someone offers to fulfill a request.
func (s *Service) recordOfferMadeEvent(ctx context.Context, requestID, offererID, communityID string) error {
	// Record event
	if _, err := s.bus.Publish(ctx, &models.CommunityEvent{
		CommunityId: communityID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE,
		ActorId:     offererID,
		Topic:       &models.CommunityEvent_RequestId{RequestId: requestID},
	}); err != nil {
		logger := logging.LoggerWithContext(ctx).With(
			"community_id", communityID,
			"target_request_id", requestID,
		)
		logger.Error("failed to record offer made event for community", "error", err)
		return err
	}

	return nil
}

// recordOfferWithdrawnEvent records a community event when someone withdraws their offer from a request.
func (s *Service) recordOfferWithdrawnEvent(ctx context.Context, requestID, offererID, communityID string) error {
	if _, err := s.bus.Publish(ctx, &models.CommunityEvent{
		CommunityId: communityID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_WITHDRAWN,
		ActorId:     offererID,
		Topic:       &models.CommunityEvent_RequestId{RequestId: requestID},
	}); err != nil {
		logger := logging.LoggerWithContext(ctx).With(
			"community_id", communityID,
			"target_request_id", requestID,
		)
		logger.Error("failed to record offer withdrawn event for community", "error", err)
		return err
	}

	return nil
}
