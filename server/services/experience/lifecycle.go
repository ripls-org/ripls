package experience

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
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// MarkExperienceInProcess transitions an experience from JOINED to IN_PROCESS state.
// This indicates the experience is currently happening.
func (s *Service) MarkExperienceInProcess(
	ctx context.Context,
	req *connect.Request[api.MarkExperienceInProcessRequest],
) (*connect.Response[api.MarkExperienceInProcessResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	expStored, err := s.fetchExperienceForRead(ctx, req.Msg.ExperienceId, logger.Logger, "MarkExperienceInProcess")
	if err != nil {
		return nil, err
	}

	// Verify ownership
	if expStored.OwnerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the owner can mark experience as in-process"))
	}

	// Validate state transition
	if expStored.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE &&
		expStored.State != models.ExperienceState_EXPERIENCE_STATE_JOINED {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("experience must be in ACTIVE or JOINED state (current: %s)", expStored.State))
	}

	// Update state and started timestamp
	now := clock.UnixSec(ctx)
	expStored.State = models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS
	expStored.StartedAtUnixSec = &now
	err = s.storage.Update(ctx, expStored)
	if err != nil {
		logger.Error("failed to update experience state", "error", err)
		return nil, connecterr.Internal(ctx, "MarkExperienceInProcess", err)
	}

	logger.Info("marked experience as in-process")

	// Send system message if conversation exists
	if expStored.ConversationId != "" && s.systemMessageWriter != nil {
		displayName := s.getUserDisplayName(ctx, authInfo.UserID)
		if err := s.systemMessageWriter.InsertLocalized(ctx, expStored.ConversationId, authInfo.UserID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_STARTED, chat.ExperienceStartedMessage(displayName)); err != nil {
			logger.Warn("failed to write STARTED system message", "error", err)
			// Don't fail the whole operation for system message
		}
	}

	// Emit EXPERIENCE_STARTED per community the experience is shared with.
	// Recipients (Yes/Maybe RSVPs minus actor) and stream-suppression are
	// resolved inside RecordCommunityEventAndNotify via the funnel.
	inProcessCommunityExperiences, err := s.storage.QueryByField(ctx, "experience_id", req.Msg.ExperienceId, &models.CommunityExperience{})
	if err != nil {
		logger.Warn("failed to query community experiences for STARTED notification", "error", err)
		// Best-effort: don't fail the state transition for event recording.
	} else {
		for _, msg := range inProcessCommunityExperiences {
			ce := msg.(*models.CommunityExperience)
			if _, err := s.bus.Publish(ctx, &models.CommunityEvent{
				CommunityId: ce.CommunityId,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED,
				ActorId:     authInfo.UserID,
				Topic: &models.CommunityEvent_ExperienceId{
					ExperienceId: req.Msg.ExperienceId,
				},
				OccurredAtUnixSec: now,
			}); err != nil {
				logger.WarnContext(ctx, "failed to record EXPERIENCE_STARTED event",
					"community_id", ce.CommunityId,
					"error", err,
				)
				// Best-effort: don't fail the state transition for event recording.
			}
		}
	}

	// Build API experience to return (no community context needed)
	apiExp, err := s.buildAPIExperience(ctx, expStored, "")
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&api.MarkExperienceInProcessResponse{
		Experience: apiExp,
	}), nil
}

// CompleteExperience transitions an experience to COMPLETED state.
// This indicates the experience has finished successfully.
// Optionally accepts a summary that will be stored and posted to the conversation.
func (s *Service) CompleteExperience(
	ctx context.Context,
	req *connect.Request[api.CompleteExperienceRequest],
) (*connect.Response[api.CompleteExperienceResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CompleteExperience",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	expStored, err := s.fetchExperienceForRead(ctx, req.Msg.ExperienceId, logger.Logger, "CompleteExperience")
	if err != nil {
		return nil, err
	}

	// Verify ownership
	if expStored.OwnerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the owner can complete experience"))
	}

	// Validate state transition — allow completing from ACTIVE, JOINED, or IN_PROCESS.
	// Direct completion from ACTIVE/JOINED skips the separate MarkInProcess step,
	// which avoids race conditions and unnecessary "in progress" notifications.
	switch expStored.State {
	case models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
		models.ExperienceState_EXPERIENCE_STATE_JOINED,
		models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS:
		// Valid source states for completion.
	default:
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("experience cannot be completed from state %s", expStored.State))
	}

	// Capture pre-completion snapshot for UndoCompleteExperience. Done
	// before any mutation so UndoData restores the true prior state.
	priorExperienceState := expStored.State
	priorImpactEstimate := expStored.ImpactEstimate

	// Set state fields in memory (persisted in a single Update after IE computation).
	now := clock.UnixSec(ctx)
	expStored.State = models.ExperienceState_EXPERIENCE_STATE_COMPLETED
	expStored.CompletedAtUnixSec = &now
	if expStored.StartedAtUnixSec == nil {
		expStored.StartedAtUnixSec = &now
	}

	// Store optional summary if provided
	if req.Msg.Summary != nil && *req.Msg.Summary != "" {
		expStored.CompletionSummary = req.Msg.Summary
		logger.InfoContext(ctx, "storing completion summary", "summary_length", len(*req.Msg.Summary))
	}

	// Auto-mark attendance fallback: If no attendance has been recorded yet,
	// automatically mark all YES RSVPs as attended. This ensures stories are
	// generated even if the owner didn't explicitly record attendance.
	if err := s.autoMarkAttendanceIfNeeded(ctx, req.Msg.ExperienceId); err != nil {
		// Log warning but don't fail - story generation will proceed regardless
		logger.ErrorContext(ctx, "failed to auto-mark attendance",
			"experience_id", req.Msg.ExperienceId,
			"error", err,
		)
	}

	// Fetch CommunityExperience records once — reused for impact, story gen, and archival.
	communityExperiences, err := s.storage.QueryByField(ctx, "experience_id", req.Msg.ExperienceId, &models.CommunityExperience{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community experiences", "error", err)
		return nil, connecterr.Internal(ctx, "CompleteExperience", err, "detail", "failed to query community experiences")
	}

	// Get actual attendee count (RSVPs with attended = YES).
	attendeeRsvps, err := s.storage.QueryByFields(ctx, map[string]any{
		"experience_id": req.Msg.ExperienceId,
		"attended":      int32(models.AttendedStatus_ATTENDED_STATUS_YES),
	}, &models.ExperienceRSVP{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query attendees for impact estimation",
			"experience_id", req.Msg.ExperienceId,
			"error", err,
		)
		// Continue with 0 attendees if query fails
	}
	attendeeCount := int32(len(attendeeRsvps))

	// Group size: the completion modal's confirmed set wins (same derivation
	// as PreviewExperienceImpact — host and provisional users included) so
	// the committed Quality Time matches the previewed one (#2724). Legacy
	// callers that send no confirmed set fall back to attended-YES RSVPs.
	groupSize := attendeeCount
	if n := int32(len(req.Msg.ConfirmedAttendeeIds)); n > 0 {
		groupSize = n
	}
	if req.Msg.ConfirmedAttendeeCount > 0 {
		groupSize = req.Msg.ConfirmedAttendeeCount
	}
	if groupSize < 1 {
		groupSize = 1
	}

	// Build impact estimate for completed experience.
	var ie *api.ImpactEstimate
	if s.estimatorCfg != nil {
		var valueUSD float32
		if expStored.ValueEstimate != nil {
			valueUSD = expStored.ValueEstimate.EstimatedValueUsd
		}
		// Resolve connection context between owner and the first confirmed
		// non-owner attendee (same rule as the preview), falling back to the
		// first attended-YES non-owner for legacy callers.
		var communityID string
		if len(communityExperiences) > 0 {
			communityID = communityExperiences[0].(*models.CommunityExperience).CommunityId
		}
		var firstAttendeeID string
		for _, uid := range req.Msg.ConfirmedAttendeeIds {
			if uid != expStored.OwnerId {
				firstAttendeeID = uid
				break
			}
		}
		if firstAttendeeID == "" {
			for _, rsvp := range attendeeRsvps {
				uid := rsvp.(*models.ExperienceRSVP).UserId
				if uid != expStored.OwnerId {
					firstAttendeeID = uid
					break
				}
			}
		}
		connCtx := s.resolveConnectionContext(ctx, expStored.OwnerId, firstAttendeeID, communityID, logger)
		// Stored attributes only — the sole source PreviewExperienceImpact
		// can see. Inferring social attributes here would hand the commit an
		// input the preview never had, and the headline number would change
		// under the host at the instant they wrap up. Every input must be one
		// both paths share (#2724); detection-time inference still reaches
		// this through the stored estimate.
		hint := buildHintFromStoredEstimate(expStored)
		ie = impact_metrics.BuildExperienceImpactMetrics(valueUSD, groupSize, s.estimatorCfg, connCtx, hint)

		// Gear that changed hands through this event carries the material
		// impact: roll the child transfers' item-based money/emissions/time
		// into the display estimate (#2724), mirroring the request-side
		// adoption (#2702). Quality Time stays the experience's own.
		if children, childErr := s.deliverableChildTransfers(ctx, req.Msg.ExperienceId); childErr != nil {
			logger.WarnContext(ctx, "failed to query child transfers for impact; keeping value-based estimate", "error", childErr)
		} else if adoptedID := s.adoptChildTransferImpact(ctx, ie, children, connCtx, logger); adoptedID != "" {
			expStored.ImpactAdoptedFromTransferId = &adoptedID
		}

		// Apply user-supplied overrides from the completion modal when present.
		if req.Msg.QualityTimeOverrides != nil || req.Msg.MoneySavingsOverrides != nil || req.Msg.EmissionsOverrides != nil {
			overrides := &impact_metrics.ImpactOverrides{
				QualityTime:  req.Msg.QualityTimeOverrides,
				MoneySavings: req.Msg.MoneySavingsOverrides,
				Emissions:    req.Msg.EmissionsOverrides,
			}
			if applied, applyErr := impact_metrics.ApplyImpactOverrides(ie, overrides, s.estimatorCfg); applyErr == nil {
				ie = applied
			} else {
				logger.WarnContext(ctx, "failed to apply completion overrides, using base estimate", "error", applyErr)
			}
		}
		expStored.ImpactEstimate = impact_metrics.APIImpactToModels(ie)
	} else {
		ie = &api.ImpactEstimate{}
	}

	// Single Update persists state change, summary, and ImpactEstimate together.
	err = s.storage.Update(ctx, expStored)
	if err != nil {
		logger.ErrorContext(ctx, "failed to update experience", "error", err)
		return nil, connecterr.Internal(ctx, "CompleteExperience", err)
	}

	logger.InfoContext(ctx, "completed experience successfully")

	// Fetch display name once for system messages
	var displayName string
	if expStored.ConversationId != "" && s.systemMessageWriter != nil {
		displayName = s.getUserDisplayName(ctx, authInfo.UserID)
	}

	// Send the COMPLETED system message first so its id can be threaded
	// onto the EXPERIENCE_COMPLETED CommunityEvent(s) below — undo uses
	// it to soft-delete the message by direct read.
	var completedSystemMessageID string
	if displayName != "" {
		msgID, err := s.systemMessageWriter.InsertLocalizedReturnID(ctx, expStored.ConversationId, authInfo.UserID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_COMPLETED, chat.ExperienceCompletedByActorMessage(displayName))
		if err != nil {
			logger.Warn("failed to write COMPLETED system message", "error", err)
			// Don't fail the whole operation for system message
		} else {
			completedSystemMessageID = msgID
		}
	}

	// Record an EXPERIENCE_COMPLETED CommunityEvent per community the
	// experience was shared with. The first event's id is returned to
	// the client so snackbar/story-screen undo can target this action;
	// secondary events share the same UndoData snapshot.
	undoData := &models.UndoData{
		Variant: &models.UndoData_ExperienceCompletion{
			ExperienceCompletion: &models.ExperienceCompletionUndo{
				PriorState:          priorExperienceState,
				PriorAttendeeCount:  attendeeCount,
				PriorImpactEstimate: priorImpactEstimate,
			},
		},
	}
	var primaryCompletionEventID string
	for _, msg := range communityExperiences {
		ce := msg.(*models.CommunityExperience)
		event := &models.CommunityEvent{
			CommunityId: ce.CommunityId,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED,
			ActorId:     authInfo.UserID,
			Topic: &models.CommunityEvent_ExperienceId{
				ExperienceId: req.Msg.ExperienceId,
			},
			OccurredAtUnixSec: now,
			UndoData:          undoData,
		}
		if completedSystemMessageID != "" {
			event.SystemChatMessageId = &completedSystemMessageID
		}
		if _, err := s.bus.Publish(ctx, event); err != nil {
			logger.WarnContext(ctx, "failed to record EXPERIENCE_COMPLETED event",
				"community_id", ce.CommunityId,
				"error", err,
			)
			// Best-effort: don't fail completion for event recording.
			continue
		}
		if primaryCompletionEventID == "" {
			primaryCompletionEventID = event.Id
		}
	}

	// Post summary to conversation if provided
	if req.Msg.Summary != nil && *req.Msg.Summary != "" && displayName != "" {
		if err := s.systemMessageWriter.InsertLocalized(ctx, expStored.ConversationId, authInfo.UserID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_COMPLETED, chat.ExperienceCompletionSummaryMessage(displayName, *req.Msg.Summary)); err != nil {
			logger.Warn("failed to post summary to conversation", "error", err)
			// Don't fail the whole operation for summary posting
		}
	}

	// Story generation is owned by the story_subscriber on the
	// community-event bus (#510 PR 4) — EXPERIENCE_COMPLETED events
	// emitted above flow through the bus and trigger one story per
	// community via the subscriber.

	// Archive all CommunityExperience records (already fetched above).
	for _, msg := range communityExperiences {
		communityExperience := msg.(*models.CommunityExperience)
		communityExperience.Archived = true
		if err := s.storage.Update(ctx, communityExperience); err != nil {
			logger.ErrorContext(ctx, "failed to archive community experience",
				"community_experience_id", communityExperience.Id,
				"error", err)
			return nil, connecterr.Internal(ctx, "CompleteExperience", err, "detail", "failed to archive community experience")
		}
		logger.InfoContext(ctx, "community experience archived",
			"community_experience_id", communityExperience.Id,
			"community_id", communityExperience.CommunityId)
	}

	// Build API experience to return (no community context needed)
	// Dismiss all watches — experience reached terminal state.
	ws := storage.NewWatchStorage(s.storage)
	if err := ws.DismissAllForItem(ctx, models.WatchedItemType_WATCHED_ITEM_TYPE_EXPERIENCE, expStored.Id); err != nil {
		logger.WarnContext(ctx, "failed to dismiss watches on experience completion", "error", err)
	}

	apiExp, err := s.buildAPIExperience(ctx, expStored, "")
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&api.CompleteExperienceResponse{
		Experience:       apiExp,
		Impact:           ie,
		CommunityEventId: primaryCompletionEventID,
	}), nil
}

// autoMarkAttendanceIfNeeded automatically marks YES RSVPs as attended if no
// attendance has been explicitly recorded yet. This is a fallback mechanism to
// ensure story generation works even when owners don't manually record attendance.
func (s *Service) autoMarkAttendanceIfNeeded(ctx context.Context, experienceID string) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "autoMarkAttendanceIfNeeded",
		"experience_id", experienceID,
	)

	// Query all RSVPs for this experience across all communities
	allRsvps, err := s.storage.QueryByField(ctx, "experience_id", experienceID, &models.ExperienceRSVP{})
	if err != nil {
		return fmt.Errorf("failed to query RSVPs: %w", err)
	}

	if len(allRsvps) == 0 {
		logger.InfoContext(ctx, "no RSVPs found, skipping auto-mark")
		return nil
	}

	// Check if any attendance has already been recorded
	hasAttendanceRecorded := false
	for _, r := range allRsvps {
		rsvp := r.(*models.ExperienceRSVP)
		if rsvp.GetAttended() == models.AttendedStatus_ATTENDED_STATUS_YES || rsvp.GetAttended() == models.AttendedStatus_ATTENDED_STATUS_NO {
			hasAttendanceRecorded = true
			break
		}
	}

	// If attendance has been explicitly recorded, don't auto-mark
	if hasAttendanceRecorded {
		logger.InfoContext(ctx, "attendance already recorded, skipping auto-mark")
		return nil
	}

	// Auto-mark all YES RSVPs as attended
	autoMarkedCount := 0
	for _, r := range allRsvps {
		rsvp := r.(*models.ExperienceRSVP)

		// Only auto-mark YES intentions
		if rsvp.GetIntention() != models.RSVPIntention_RSVP_INTENTION_YES {
			continue
		}

		// Mark as attended
		rsvp.Attended = models.AttendedStatus_ATTENDED_STATUS_YES
		if err := s.storage.Update(ctx, rsvp); err != nil {
			logger.ErrorContext(ctx, "failed to auto-mark RSVP as attended",
				"rsvp_id", rsvp.Id,
				"user_id", rsvp.UserId,
				"error", err,
			)
			// Continue with other RSVPs even if one fails
			continue
		}

		autoMarkedCount++
	}

	if autoMarkedCount > 0 {
		logger.InfoContext(ctx, "auto-marked YES RSVPs as attended (fallback)",
			"auto_marked_count", autoMarkedCount,
			"total_rsvps", len(allRsvps),
		)
	} else {
		logger.InfoContext(ctx, "no YES RSVPs to auto-mark")
	}

	return nil
}

// CancelExperience transitions an experience to CANCELLED state.
// This indicates the experience will not happen.
func (s *Service) CancelExperience(
	ctx context.Context,
	req *connect.Request[api.CancelExperienceRequest],
) (*connect.Response[api.CancelExperienceResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	expStored, err := s.fetchExperienceForRead(ctx, req.Msg.ExperienceId, logger.Logger, "CancelExperience")
	if err != nil {
		return nil, err
	}

	// Verify ownership
	if expStored.OwnerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the owner can cancel experience"))
	}

	// Validate state transition - can cancel from any state except COMPLETED or CANCELLED
	if expStored.State == models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("cannot cancel a completed experience"))
	}
	if expStored.State == models.ExperienceState_EXPERIENCE_STATE_CANCELLED {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("experience is already cancelled"))
	}

	// Update state
	expStored.State = models.ExperienceState_EXPERIENCE_STATE_CANCELLED
	err = s.storage.Update(ctx, expStored)
	if err != nil {
		logger.Error("failed to update experience state", "error", err)
		return nil, connecterr.Internal(ctx, "CancelExperience", err)
	}

	logger.Info("cancelled experience")

	// Send system message if conversation exists
	if expStored.ConversationId != "" && s.systemMessageWriter != nil {
		displayName := s.getUserDisplayName(ctx, authInfo.UserID)
		if err := s.systemMessageWriter.InsertLocalized(ctx, expStored.ConversationId, authInfo.UserID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_CANCELLED, chat.ExperienceCancelledMessage(displayName)); err != nil {
			logger.Warn("failed to write CANCELLED system message", "error", err)
			// Don't fail the whole operation for system message
		}
	}

	// Fetch community links once — reused for CANCELLED event emission and archival.
	communityExperiences, err := s.storage.QueryByField(ctx, "experience_id", req.Msg.ExperienceId, &models.CommunityExperience{})
	if err != nil {
		logger.Error("failed to query community experiences", "error", err)
		return nil, connecterr.Internal(ctx, "CancelExperience", err, "detail", "failed to query community experiences")
	}

	// Emit EXPERIENCE_CANCELLED per community so Yes/Maybe RSVPs (minus actor) are notified
	// through the standard funnel with stream-suppression and preference gating.
	cancelNow := clock.UnixSec(ctx)
	for _, msg := range communityExperiences {
		ce := msg.(*models.CommunityExperience)
		if _, err := s.bus.Publish(ctx, &models.CommunityEvent{
			CommunityId: ce.CommunityId,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED,
			ActorId:     authInfo.UserID,
			Topic: &models.CommunityEvent_ExperienceId{
				ExperienceId: req.Msg.ExperienceId,
			},
			OccurredAtUnixSec: cancelNow,
		}); err != nil {
			logger.WarnContext(ctx, "failed to record EXPERIENCE_CANCELLED event",
				"community_id", ce.CommunityId,
				"error", err,
			)
			// Best-effort: don't fail cancellation for event recording.
		}
	}

	for _, msg := range communityExperiences {
		communityExperience := msg.(*models.CommunityExperience)
		communityExperience.Archived = true
		if err := s.storage.Update(ctx, communityExperience); err != nil {
			logger.Error("failed to archive community experience",
				"community_experience_id", communityExperience.Id,
				"error", err)
			return nil, connecterr.Internal(ctx, "CancelExperience", err, "detail", "failed to archive community experience")
		}
		logger.Info("community experience archived",
			"community_experience_id", communityExperience.Id,
			"community_id", communityExperience.CommunityId)
	}

	// Dismiss all watches — experience reached terminal state.
	wsc := storage.NewWatchStorage(s.storage)
	if err := wsc.DismissAllForItem(ctx, models.WatchedItemType_WATCHED_ITEM_TYPE_EXPERIENCE, expStored.Id); err != nil {
		logger.WarnContext(ctx, "failed to dismiss watches on experience cancellation", "error", err)
	}

	// Build API experience to return (no community context needed)
	apiExp, err := s.buildAPIExperience(ctx, expStored, "")
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&api.CancelExperienceResponse{
		Experience: apiExp,
	}), nil
}

// extractExperienceDurationMinutes returns the user-set duration from an Experience's time
// field. Checks SpecificTime.duration_minutes and TimeRange.duration_minutes. Returns 0
// if the experience has no time set or the duration field is not populated.
func extractExperienceDurationMinutes(exp *models.Experience) float32 {
	if exp == nil || exp.Time == nil {
		return 0
	}
	switch t := exp.Time.TimeType.(type) {
	case *models.ExperienceTime_Specific:
		return float32(t.Specific.GetDurationMinutes())
	case *models.ExperienceTime_Range:
		return float32(t.Range.GetDurationMinutes())
	default:
		return 0
	}
}

// generateStoryForCommunity moved to
// server/story/subscriber/experience.go in #510 PR 4. The
// EXPERIENCE_COMPLETED events emitted by CompleteExperience flow through
// the community-event bus and trigger one story per community.
// materializeESMPromptForStory and formatStoryKicker also moved to the
// same package.
