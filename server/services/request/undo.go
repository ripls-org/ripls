package request

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
	"go.ripls.org/ripls/server/storage"
)

// UndoMarkRequestFulfilled reverses a prior MarkRequestFulfilled call.
//
// Flow: load event + validate actor/event_type/UndoData, drift-check
// request is still FULFILLED, revert state + ImpactEstimate + clear
// FulfilledAtUnixSec, un-archive CommunityRequest rows so the request
// resurfaces in community feeds, soft-delete the fulfillment system
// chat message, emit retraction + replacement.
func (s *Service) UndoMarkRequestFulfilled(
	ctx context.Context,
	req *connect.Request[api.UndoMarkRequestFulfilledRequest],
) (*connect.Response[api.UndoMarkRequestFulfilledResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "undo_mark_request_fulfilled",
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
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_NOT_ACTOR, "Only the person who marked this fulfilled can undo it.")
	}
	if event.EventType != models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_WRONG_EVENT_TYPE, "That action cannot be undone with UndoMarkRequestFulfilled.")
	}
	fulfillUndo := event.GetUndoData().GetRequestFulfillment()
	if fulfillUndo == nil {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_WRONG_EVENT_TYPE, "This fulfillment is missing the data needed to reverse it.")
	}

	requestID := event.GetRequestId()
	if requestID == "" {
		return nil, connecterr.Internal(ctx, "UndoMarkRequestFulfilled", fmt.Errorf("community event missing request topic"))
	}

	alreadyUndone, err := s.hasRequestRetraction(ctx, requestID, models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED_UNDONE, event.OccurredAtUnixSec)
	if err != nil {
		return nil, connecterr.Internal(ctx, "UndoMarkRequestFulfilled", err, "detail", "undo pre-check failed")
	}
	if alreadyUndone {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_ALREADY_UNDONE, "This action has already been undone.")
	}

	requestStored := &models.Request{}
	if err := s.storage.GetByID(ctx, requestID, requestStored); err != nil {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED, "The request has been removed.")
	}
	if requestStored.State != models.RequestState_REQUEST_STATE_FULFILLED {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED, "The request is no longer in a state where this action can be undone.")
	}

	requestStored.State = fulfillUndo.PriorState
	requestStored.FulfilledAtUnixSec = nil
	requestStored.ImpactEstimate = fulfillUndo.PriorImpactEstimate
	// The adopted-impact marker (#2702) describes the fulfillment being
	// undone; the restored pre-fulfillment estimate is the request's own.
	requestStored.ImpactAdoptedFromTransferId = nil
	if err := s.storage.Update(ctx, requestStored); err != nil {
		return nil, connecterr.Internal(ctx, "UndoMarkRequestFulfilled", err, "detail",

			// Un-archive CommunityRequest rows so the request resurfaces.
			"failed to revert request")
	}

	communityRequests, err := storage.QueryByField[*models.CommunityRequest](s.storage, ctx, "request_id", requestID)
	if err != nil {
		logger.WarnContext(ctx, "failed to query community_requests for un-archive", "error", err)
	} else {
		for _, cr := range communityRequests {
			if !cr.Archived {
				continue
			}
			cr.Archived = false
			if err := s.storage.Update(ctx, cr); err != nil {
				logger.WarnContext(ctx, "failed to un-archive community_request", "id", cr.Id, "error", err)
			}
		}
	}

	s.softDeleteAndRetractRequestEvent(ctx, event, requestStored, authInfo.UserID,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED_UNDONE,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_FULFILLED, logger)

	logger.InfoContext(ctx, "undid mark request fulfilled", "target_request_id", requestID, "outcome", "success")
	return connect.NewResponse(&api.UndoMarkRequestFulfilledResponse{}), nil
}

// UndoCancelRequest reverses a prior CancelRequest call.
func (s *Service) UndoCancelRequest(
	ctx context.Context,
	req *connect.Request[api.UndoCancelRequestRequest],
) (*connect.Response[api.UndoCancelRequestResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "undo_cancel_request",
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
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_NOT_ACTOR, "Only the person who cancelled this request can undo it.")
	}
	if event.EventType != models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_WRONG_EVENT_TYPE, "That action cannot be undone with UndoCancelRequest.")
	}
	cancelUndo := event.GetUndoData().GetRequestCancel()
	if cancelUndo == nil {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_WRONG_EVENT_TYPE, "This cancel event is missing the data needed to reverse it.")
	}

	requestID := event.GetRequestId()
	if requestID == "" {
		return nil, connecterr.Internal(ctx, "UndoCancelRequest", fmt.Errorf("community event missing request topic"))
	}

	alreadyUndone, err := s.hasRequestRetraction(ctx, requestID, models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED_UNDONE, event.OccurredAtUnixSec)
	if err != nil {
		return nil, connecterr.Internal(ctx, "UndoCancelRequest", err, "detail", "undo pre-check failed")
	}
	if alreadyUndone {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_ALREADY_UNDONE, "This action has already been undone.")
	}

	requestStored := &models.Request{}
	if err := s.storage.GetByID(ctx, requestID, requestStored); err != nil {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED, "The request has been removed.")
	}
	if requestStored.State != models.RequestState_REQUEST_STATE_CANCELLED {
		return nil, undoFailure(api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED, "The request is no longer cancelled, so this can't be undone.")
	}

	requestStored.State = cancelUndo.PriorState
	if err := s.storage.Update(ctx, requestStored); err != nil {
		return nil, connecterr.Internal(ctx, "UndoCancelRequest", err, "detail",

			// Un-archive CommunityRequest rows that cancel archived, so the
			// request resurfaces in community feeds.
			"failed to revert request")
	}

	communityRequests, err := storage.QueryByField[*models.CommunityRequest](s.storage, ctx, "request_id", requestID)
	if err != nil {
		logger.WarnContext(ctx, "failed to query community_requests for un-archive", "error", err)
	} else {
		for _, cr := range communityRequests {
			if !cr.Archived {
				continue
			}
			cr.Archived = false
			if err := s.storage.Update(ctx, cr); err != nil {
				logger.WarnContext(ctx, "failed to un-archive community_request", "id", cr.Id, "error", err)
			}
		}
	}

	s.softDeleteAndRetractRequestEvent(ctx, event, requestStored, authInfo.UserID,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED_UNDONE,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_CANCELLED, logger)

	logger.InfoContext(ctx, "undid cancel request", "target_request_id", requestID, "outcome", "success")
	return connect.NewResponse(&api.UndoCancelRequestResponse{}), nil
}

// hasRequestRetraction returns true when a retraction event of the
// given type for requestID exists at or after originalOccurredAtUnixSec.
func (s *Service) hasRequestRetraction(ctx context.Context, requestID string, retractionType models.CommunityEventType, originalOccurredAtUnixSec int64) (bool, error) {
	if requestID == "" {
		return false, errors.New("requestID required")
	}
	candidates, err := s.storage.QueryByField(ctx, "request_id", requestID, &models.CommunityEvent{})
	if err != nil {
		return false, fmt.Errorf("query candidates: %w", err)
	}
	for _, msg := range candidates {
		ev := msg.(*models.CommunityEvent)
		if ev.EventType != retractionType {
			continue
		}
		if ev.OccurredAtUnixSec < originalOccurredAtUnixSec {
			continue
		}
		return true, nil
	}
	return false, nil
}

// softDeleteAndRetractRequestEvent performs the three final steps of
// a request-domain undo: soft-delete the original system chat message,
// emit the retraction CommunityEvent, insert a replacement UNDONE
// message. All failures are logged non-fatally (primary state revert
// has already succeeded).
func (s *Service) softDeleteAndRetractRequestEvent(
	ctx context.Context,
	event *models.CommunityEvent,
	requestStored *models.Request,
	actorID string,
	retractionType models.CommunityEventType,
	undoneAction models.ChatSystemAction,
	logger *logging.Logger,
) {
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
		CommunityId: event.CommunityId,
		EventType:   retractionType,
		ActorId:     actorID,
		Topic:       &models.CommunityEvent_RequestId{RequestId: requestStored.Id},
		// Carry the retracted action's undo data so downstream subscribers
		// can unwind their side of it — the transfer service cancels the
		// handoff-driving transfer recorded on a fulfillment undo (#2702).
		UndoData: event.UndoData,
	}
	if _, err := s.bus.Publish(ctx, retraction); err != nil {
		logger.WarnContext(ctx, "failed to record retraction community event", "error", err)
	}

	if requestStored.ConversationId != "" && s.systemMessageWriter != nil {
		actorName := s.getUserDisplayName(ctx, actorID)
		if _, err := s.systemMessageWriter.InsertLocalizedReturnID(
			ctx,
			requestStored.ConversationId,
			actorID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_UNDONE,
			chat.UndoneMessage(actorName, undoneAction),
		); err != nil {
			logger.WarnContext(ctx, "failed to insert undone system message", "error", err)
		}
	}
}

// undoFailure builds a typed Connect error with an UndoErrorDetail
// attached so clients can render a precise human-readable message.
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
