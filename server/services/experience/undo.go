package experience

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

// UndoCompleteExperience reverses a prior CompleteExperience call.
//
// Flow:
//  1. Load the CommunityEvent by id; validate actor, event type, that
//     an ExperienceCompletion UndoData variant is present, and that
//     no retraction event already exists.
//  2. Load the experience; drift-check state is still COMPLETED.
//  3. Revert state, CompletedAtUnixSec, and ImpactEstimate from the
//     UndoData snapshot.
//  4. Soft-delete the completion system chat message referenced by
//     SystemChatMessageId.
//  5. Emit an EXPERIENCE_COMPLETED_UNDONE retraction event per
//     community the experience was shared with.
//  6. Un-archive CommunityExperience rows archived during completion,
//     so the experience resurfaces in community feeds.
//  7. Emit a CHAT_SYSTEM_ACTION_UNDONE replacement system message.
func (s *Service) UndoCompleteExperience(
	ctx context.Context,
	req *connect.Request[api.UndoCompleteExperienceRequest],
) (*connect.Response[api.UndoCompleteExperienceResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "undo_complete_experience",
		"user_id", authInfo.UserID,
		"community_event_id", req.Msg.CommunityEventId,
	)

	if req.Msg.CommunityEventId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_event_id is required"))
	}

	// Step 1: Load and validate event.
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

	if event.EventType != models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_WRONG_EVENT_TYPE,
			"That action cannot be undone with UndoCompleteExperience.",
		)
	}

	expUndo := event.GetUndoData().GetExperienceCompletion()
	if expUndo == nil {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_WRONG_EVENT_TYPE,
			"This completion is missing the data needed to reverse it.",
		)
	}

	experienceID := event.GetExperienceId()
	if experienceID == "" {
		logger.ErrorContext(ctx, "community event has no experience_id topic")
		return nil, connecterr.Internal(ctx, "UndoCompleteExperience", fmt.Errorf("community event missing experience topic"))
	}

	alreadyUndone, err := s.hasExperienceRetraction(ctx, experienceID, event.OccurredAtUnixSec)
	if err != nil {
		logger.ErrorContext(ctx, "failed to check for retraction", "error", err)
		return nil, connecterr.Internal(ctx, "UndoCompleteExperience", err, "detail", "undo pre-check failed")
	}
	if alreadyUndone {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_ALREADY_UNDONE,
			"This action has already been undone.",
		)
	}

	// Step 2: Load experience and drift-check.
	expStored := &models.Experience{}
	if err := s.storage.GetByID(ctx, experienceID, expStored); err != nil {
		logger.InfoContext(ctx, "undo target experience not found", "error", err)
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED,
			"The experience has been removed.",
		)
	}
	if expStored.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
		return nil, undoFailure(
			api.UndoFailureReason_UNDO_FAILURE_REASON_ENTITY_CHANGED,
			"The experience is no longer in a state where this action can be undone.",
		)
	}

	// Step 3: Revert state + completion-time fields. The adopted-from marker
	// travels with the completion-time estimate (#2724), so it clears too.
	expStored.State = expUndo.PriorState
	expStored.CompletedAtUnixSec = nil
	expStored.ImpactEstimate = expUndo.PriorImpactEstimate
	expStored.ImpactAdoptedFromTransferId = nil
	if err := s.storage.Update(ctx, expStored); err != nil {
		logger.ErrorContext(ctx, "failed to revert experience state", "error", err)
		return nil, connecterr.Internal(ctx, "UndoCompleteExperience", err, "detail",

			// Step 4: Soft-delete the completion system chat message.
			"failed to revert experience")
	}

	if systemChatMessageID := event.GetSystemChatMessageId(); systemChatMessageID != "" {
		msg := &models.ChatMessage{}
		if err := s.storage.GetByID(ctx, systemChatMessageID, msg); err != nil {
			logger.WarnContext(ctx, "completion chat message not found for soft-delete", "error", err)
		} else {
			msg.Deleted = &models.DeletedMetadata{
				DeletedByUserId:  authInfo.UserID,
				DeletedAtUnixSec: clock.UnixSec(ctx),
			}
			if err := s.storage.Update(ctx, msg); err != nil {
				logger.WarnContext(ctx, "failed to soft-delete completion chat message", "error", err)
			}
		}
	}

	// Step 5 + 6: For each community the experience was shared with,
	// emit a retraction event and un-archive the CommunityExperience row.
	communityExperiences, err := s.storage.QueryByField(ctx, "experience_id", experienceID, &models.CommunityExperience{})
	if err != nil {
		logger.WarnContext(ctx, "failed to query community experiences for un-archive", "error", err)
	} else {
		for _, msg := range communityExperiences {
			ce := msg.(*models.CommunityExperience)
			retraction := &models.CommunityEvent{
				CommunityId: ce.CommunityId,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED_UNDONE,
				ActorId:     authInfo.UserID,
				Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
			}
			if _, err := s.bus.Publish(ctx, retraction); err != nil {
				logger.WarnContext(ctx, "failed to record retraction event", "community_id", ce.CommunityId, "error", err)
			}

			if ce.Archived {
				ce.Archived = false
				if err := s.storage.Update(ctx, ce); err != nil {
					logger.WarnContext(ctx, "failed to un-archive community experience",
						"community_experience_id", ce.Id, "error", err)
				}
			}
		}
	}

	// Step 7: Emit replacement system chat message.
	if expStored.ConversationId != "" && s.systemMessageWriter != nil {
		actorName := s.getUserDisplayName(ctx, authInfo.UserID)
		undoneAction := models.ChatSystemAction_CHAT_SYSTEM_ACTION_COMPLETED
		if _, err := s.systemMessageWriter.InsertLocalizedReturnID(
			ctx,
			expStored.ConversationId,
			authInfo.UserID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_UNDONE,
			chat.UndoneMessage(actorName, undoneAction),
		); err != nil {
			logger.WarnContext(ctx, "failed to insert undone system message", "error", err)
		}
	}

	logger.InfoContext(ctx, "undid complete experience",
		"experience_id", experienceID,
		"outcome", "success",
	)

	return connect.NewResponse(&api.UndoCompleteExperienceResponse{}), nil
}

// hasExperienceRetraction returns true if an EXPERIENCE_COMPLETED_UNDONE
// event exists for experienceID at or after originalOccurredAtUnixSec.
// CompleteExperience may emit one event per community it was shared with;
// we treat any matching retraction as proof the action has been undone.
func (s *Service) hasExperienceRetraction(ctx context.Context, experienceID string, originalOccurredAtUnixSec int64) (bool, error) {
	if experienceID == "" {
		return false, errors.New("experienceID required")
	}
	candidates, err := s.storage.QueryByField(ctx, "experience_id", experienceID, &models.CommunityEvent{})
	if err != nil {
		return false, fmt.Errorf("query candidates: %w", err)
	}
	for _, msg := range candidates {
		ev := msg.(*models.CommunityEvent)
		if ev.EventType != models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED_UNDONE {
			continue
		}
		if ev.OccurredAtUnixSec < originalOccurredAtUnixSec {
			continue
		}
		return true, nil
	}
	return false, nil
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
