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
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/rsvpstate"
	"go.ripls.org/ripls/server/storage"
)

// RSVPToExperience records a user's RSVP intention for an experience.
func (s *Service) RSVPToExperience(
	ctx context.Context,
	req *connect.Request[api.RSVPToExperienceRequest],
) (*connect.Response[api.RSVPToExperienceResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
		"community_id", req.Msg.CommunityId,
		"intention", req.Msg.Intention.String(),
	)

	expStored, err := s.fetchExperienceForRead(ctx, req.Msg.ExperienceId, logger.Logger, "RSVPToExperience")
	if err != nil {
		return nil, err
	}

	// Validate that the experience is shared with the supplied community before
	// touching any state. An empty community_id means "no community context" and
	// skips the check (matches RecordAttendance behaviour). Without this guard a
	// client with a stale communityId would silently record the RSVP under the
	// wrong community, skewing per-community attendance counts and firing chat
	// events in the wrong place.
	if req.Msg.CommunityId != "" {
		// Reject if the target community is missing, soft-deleted, or the
		// caller is not a member.
		if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
			return nil, err
		}
		ces, err := s.storage.QueryByFields(ctx, map[string]any{
			"experience_id": req.Msg.ExperienceId,
			"community_id":  req.Msg.CommunityId,
		}, &models.CommunityExperience{})
		if err != nil {
			return nil, connecterr.Internal(ctx, "RSVPToExperience", err,
				"experience_id", req.Msg.ExperienceId,
				"community_id", req.Msg.CommunityId,
			)
		}
		if len(ces) == 0 {
			logger.WarnContext(ctx, "RSVP community mismatch: experience not shared with community",
				"mismatch", "community")
			return nil, connect.NewError(connect.CodeNotFound,
				fmt.Errorf("experience not shared with this community"))
		}
	}

	// Check if experience is accepting RSVPs. IN_PROCESS is intentionally allowed so
	// that users who discover an ongoing event can still join it.
	if expStored.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE &&
		expStored.State != models.ExperienceState_EXPERIENCE_STATE_JOINED &&
		expStored.State != models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("experience is not accepting RSVPs (current state: %s)", expStored.State))
	}

	// Check if max participants reached (if specified and intention is YES)
	// Note: max_participants is per-community since each community has separate RSVPs
	if req.Msg.Intention == api.RSVPIntention_RSVP_INTENTION_YES && expStored.MaxParticipants > 0 {
		// Count current attending RSVPs in this community
		rsvpQuery := map[string]any{
			"experience_id": req.Msg.ExperienceId,
			"community_id":  req.Msg.CommunityId,
		}
		rsvpMessages, err := s.storage.QueryByFields(ctx, rsvpQuery, &models.ExperienceRSVP{})
		if err != nil {
			logger.Error("failed to query RSVPs", "error", err)
			return nil, connecterr.Internal(ctx, "RSVPToExperience", err)
		}

		attendingCount := 0
		for _, msg := range rsvpMessages {
			rsvp := msg.(*models.ExperienceRSVP)
			if rsvp.UserId != authInfo.UserID && rsvp.GetIntention() == models.RSVPIntention_RSVP_INTENTION_YES {
				attendingCount++
			}
		}

		if int32(attendingCount) >= expStored.MaxParticipants {
			return nil, connect.NewError(connect.CodeResourceExhausted,
				fmt.Errorf("experience has reached maximum participants in this community"))
		}
	}

	// Check for an existing RSVP across all communities. A user may only RSVP to
	// an experience once regardless of how many communities it is shared with.
	queryFields := map[string]any{
		"experience_id": req.Msg.ExperienceId,
		"user_id":       authInfo.UserID,
	}
	existingRsvps, err := s.storage.QueryByFields(ctx, queryFields, &models.ExperienceRSVP{})
	if err != nil {
		logger.Error("failed to query existing RSVP", "error", err)
		return nil, connecterr.Internal(ctx, "RSVPToExperience", err)
	}

	now := clock.UnixSec(ctx)
	intention := convertRSVPIntentionToModel(req.Msg.Intention)

	// intentionChanged tracks whether this call actually changed the stored
	// intention. It gates side effects (system message, community event) that
	// must not fire when the user re-submits the same choice.
	intentionChanged := true

	if len(existingRsvps) > 0 {
		// Update existing RSVP only when the intention differs.
		rsvp := existingRsvps[0].(*models.ExperienceRSVP)
		if rsvp.GetIntention() == intention {
			intentionChanged = false
			logger.Debug("RSVP intention unchanged, skipping side effects", "intention", intention)
		} else {
			rsvp.Intention = intention
			rsvp.LastUpdatedUnixSec = now

			err = s.storage.Update(ctx, rsvp)
			if err != nil {
				logger.Error("failed to update RSVP", "error", err)
				return nil, connecterr.Internal(ctx, "RSVPToExperience", err)
			}
		}
	} else {
		// Create new RSVP
		rsvp := &models.ExperienceRSVP{
			ExperienceId:       req.Msg.ExperienceId,
			UserId:             authInfo.UserID,
			CommunityId:        req.Msg.CommunityId,
			RsvpedAtUnixSec:    now,
			LastUpdatedUnixSec: now,
		}
		rsvp.Intention = intention

		_, err = s.storage.Insert(ctx, rsvp)
		if err != nil {
			logger.Error("failed to create RSVP", "error", err)
			return nil, connecterr.Internal(ctx, "RSVPToExperience", err)
		}
	}

	logger.Info("recorded RSVP", "intention_changed", intentionChanged)

	// Add user to the experience conversation when they RSVP Yes/Maybe.
	if req.Msg.Intention == api.RSVPIntention_RSVP_INTENTION_YES ||
		req.Msg.Intention == api.RSVPIntention_RSVP_INTENTION_MAYBE {
		if expStored.ConversationId != "" {
			if err = chat.AddParticipantToConversation(ctx, storage.NewChatConversationStorage(s.storage), expStored.ConversationId, authInfo.UserID); err != nil {
				logger.Warn("failed to add user to conversation",
					"conversation_id", expStored.ConversationId,
					"error", err)
			} else {
				logger.Debug("added user to conversation", "conversation_id", expStored.ConversationId)
			}
		}

		// Promote a yes/maybe RSVPer into the event's ad-hoc origin community so
		// they surface by name in the host's Who's-In roster instead of staying a
		// collapsed count inside a named community they came through (#2548).
		// Best-effort: a membership hiccup must not fail the RSVP. The roster
		// event recorded below streams the host's open roster the refresh.
		if err := s.ensureRSVPerInOriginCommunity(ctx, req.Msg.ExperienceId, authInfo.UserID, expStored.OwnerId); err != nil {
			logger.Warn("failed to promote RSVPer into origin community", "error", err)
		}
	}

	// Update experience state based on RSVPs across all communities
	if req.Msg.Intention == api.RSVPIntention_RSVP_INTENTION_YES ||
		req.Msg.Intention == api.RSVPIntention_RSVP_INTENTION_MAYBE {
		// Transition to JOINED if currently ACTIVE
		if expStored.State == models.ExperienceState_EXPERIENCE_STATE_ACTIVE {
			expStored.State = models.ExperienceState_EXPERIENCE_STATE_JOINED
			err = s.storage.Update(ctx, expStored)
			if err != nil {
				logger.Warn("failed to update experience state to JOINED", "error", err)
				// Don't fail the request, just log the error
			}
		}
	} else if req.Msg.Intention == api.RSVPIntention_RSVP_INTENTION_NO {
		// Check if we need to revert to ACTIVE (no more Yes/Maybe RSVPs across all communities)
		if expStored.State == models.ExperienceState_EXPERIENCE_STATE_JOINED {
			// Query all RSVPs across all communities to check for remaining Yes/Maybe intentions
			allRsvps, err := s.storage.QueryByField(ctx, "experience_id", req.Msg.ExperienceId, &models.ExperienceRSVP{})
			if err != nil {
				logger.Warn("failed to query RSVPs for state check", "error", err)
			} else {
				hasActiveRsvp := false
				for _, msg := range allRsvps {
					rsvp := msg.(*models.ExperienceRSVP)
					// Skip the current user's RSVP in the current community (already changed to NO)
					if rsvp.UserId == authInfo.UserID && rsvp.CommunityId == req.Msg.CommunityId {
						continue
					}
					if rsvpstate.IsGoing(rsvp.GetIntention()) {
						hasActiveRsvp = true
						break
					}
				}

				if !hasActiveRsvp {
					// No more Yes/Maybe RSVPs across any community, revert to ACTIVE
					expStored.State = models.ExperienceState_EXPERIENCE_STATE_ACTIVE
					err = s.storage.Update(ctx, expStored)
					if err != nil {
						logger.Warn("failed to revert experience state to ACTIVE", "error", err)
						// Don't fail the request, just log the error
					} else {
						logger.Info("reverted experience to ACTIVE state (no more Yes/Maybe RSVPs across all communities)")
					}
				}
			}
		}
	}

	// Record community event and system message only when the intention
	// actually changed. Re-submitting the same intention must not produce
	// duplicate notifications or chat messages.
	if intentionChanged {
		// Record community event for this RSVP (triggers notification to owner)
		// Only record if not the owner RSVPing to their own experience
		if authInfo.UserID != expStored.OwnerId {
			s.recordRSVPEvent(ctx, req.Msg.ExperienceId, authInfo.UserID, expStored.OwnerId, req.Msg.Intention)
		}

		// Send system message to the experience conversation.
		if s.systemMessageWriter != nil && expStored.ConversationId != "" {
			displayName := s.getUserDisplayName(ctx, authInfo.UserID)
			var systemAction models.ChatSystemAction
			var rsvpMsg chat.LocalizedMessage

			switch req.Msg.Intention {
			case api.RSVPIntention_RSVP_INTENTION_YES:
				systemAction = models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_YES
				rsvpMsg = chat.RSVPYesMessage(displayName)
			case api.RSVPIntention_RSVP_INTENTION_MAYBE:
				systemAction = models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_MAYBE
				rsvpMsg = chat.RSVPMaybeMessage(displayName)
			case api.RSVPIntention_RSVP_INTENTION_NO:
				systemAction = models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_NO
				rsvpMsg = chat.RSVPNoMessage(displayName)
			}

			if err := s.systemMessageWriter.InsertLocalized(ctx, expStored.ConversationId, authInfo.UserID, systemAction, rsvpMsg); err != nil {
				logger.Warn("failed to write RSVP system message",
					"conversation_id", expStored.ConversationId,
					"error", err)
			}
		}
	}

	// Rebuild social footprint with the updated YES RSVP count as the group size.
	// This keeps RM scores current as people commit to attending, without touching
	// money, carbon, or time dimensions.
	if s.estimatorCfg != nil && expStored.ImpactEstimate != nil {
		allYesRsvps, rsvpErr := s.storage.QueryByFields(ctx, map[string]any{
			"experience_id": req.Msg.ExperienceId,
			"intention":     int32(models.RSVPIntention_RSVP_INTENTION_YES),
		}, &models.ExperienceRSVP{})
		if rsvpErr != nil {
			logger.WarnContext(ctx, "failed to count YES RSVPs for QT group size update", "error", rsvpErr)
		} else {
			// Group size = number of YES RSVPs (host is already one of the RSVPs or is
			// not counted separately to avoid double-counting).
			groupSize := int32(len(allYesRsvps))
			reasoningMsg := fmt.Sprintf("Group size from %d confirmed RSVP(s)", len(allYesRsvps))
			newQT := impact_metrics.RebuildQualityTimeForGroupSize(
				estimator.TransactionExperienceConcluded,
				groupSize,
				estimator.RoleMutual,
				reasoningMsg,
				s.estimatorCfg,
			)
			if newQT != nil {
				expStored.ImpactEstimate = impact_metrics.UpdateQualityTime(expStored.ImpactEstimate, newQT)
				if updateErr := s.storage.Update(ctx, expStored); updateErr != nil {
					logger.WarnContext(ctx, "failed to persist QT group size update after RSVP", "error", updateErr)
				} else {
					logger.DebugContext(ctx, "updated QT group size after RSVP",
						"group_size", groupSize, "quality_time_minutes", newQT.GetQualityTimeMinutes().GetMean())
				}
			}
		}
	}

	// Refetch experience to get latest state (may have been updated to JOINED)
	err = s.storage.GetByID(ctx, req.Msg.ExperienceId, expStored)
	if err != nil {
		logger.Error("failed to refetch experience", "error", err)
		return nil, connecterr.Internal(ctx, "RSVPToExperience", err)
	}

	// Attendee watches the experience; owner marked unread.
	ws := storage.NewWatchStorage(s.storage)
	if err := ws.UpsertWatch(ctx, authInfo.UserID, models.WatchedItemType_WATCHED_ITEM_TYPE_EXPERIENCE, req.Msg.ExperienceId); err != nil {
		logger.Warn("failed to create watch for RSVP attendee", "error", err)
	}
	if err := ws.UpsertWatch(ctx, expStored.OwnerId, models.WatchedItemType_WATCHED_ITEM_TYPE_EXPERIENCE, req.Msg.ExperienceId); err != nil {
		logger.Warn("failed to create watch for experience owner", "error", err)
	}
	if authInfo.UserID != expStored.OwnerId {
		if err := ws.MarkUnreadForWatchers(ctx, models.WatchedItemType_WATCHED_ITEM_TYPE_EXPERIENCE, req.Msg.ExperienceId, authInfo.UserID); err != nil {
			logger.Warn("failed to mark owner watch unread on RSVP", "error", err)
		}
	}

	// Build API experience to return (no community context needed)
	apiExp, err := s.buildAPIExperience(ctx, expStored, "")
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&api.RSVPToExperienceResponse{
		Experience: apiExp,
	}), nil
}

// RecordAttendance records who actually attended an experience.
// Owner-only; allowed in any valid state.
func (s *Service) RecordAttendance(
	ctx context.Context,
	req *connect.Request[api.RecordAttendanceRequest],
) (*connect.Response[api.RecordAttendanceResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RecordAttendance",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
		"community_id", req.Msg.CommunityId,
		"attendee_count", len(req.Msg.Attendance),
	)

	expStored, err := s.fetchExperienceForRead(ctx, req.Msg.ExperienceId, logger.Logger, "RecordAttendance")
	if err != nil {
		return nil, err
	}

	// Verify ownership - only owner can record attendance
	if expStored.OwnerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the owner can record attendance"))
	}

	// Reject if the scoping community is soft-deleted (empty community_id is
	// allowed — same convention as RSVPToExperience).
	if req.Msg.CommunityId != "" {
		if _, err := auth.RequireActiveCommunity(ctx, s.storage, req.Msg.CommunityId); err != nil {
			return nil, err
		}
	}

	// Attendance can be recorded in any valid state. Only reject UNSPECIFIED (zero value),
	// which indicates a corrupted or uninitialized record.
	if expStored.State == models.ExperienceState_EXPERIENCE_STATE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("experience has invalid state"))
	}

	// Fetch every RSVP for this experience once, then resolve each attendance
	// record in memory. Attendance is event-scoped, not community-scoped: the
	// completion roster (buildRSVPs) is deduped across every shared community, so
	// an attendee's RSVP may live in a community other than the one this request
	// carries. Resolving each attendee against a single community_id (the old
	// behaviour) 404'd the whole batch whenever an attendee had RSVP'd via a
	// different community (#2690). Fetching once also avoids an N+1 over the
	// attendance list.
	now := clock.UnixSec(ctx)
	allRSVPs, err := storage.QueryByField[*models.ExperienceRSVP](s.storage, ctx, "experience_id", req.Msg.ExperienceId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "RecordAttendance", err)
	}

	// Index registered RSVPs by user_id (most-recently-updated wins, matching the
	// buildRSVPs dedup) and provisional RSVPs by (provisional_user_id,
	// community_id) — provisional attendees are genuinely per-community.
	byUser := make(map[string]*models.ExperienceRSVP, len(allRSVPs))
	byProvisional := make(map[string]*models.ExperienceRSVP, len(allRSVPs))
	for _, rsvp := range allRSVPs {
		if pid := rsvp.GetProvisionalUserId(); pid != "" {
			key := pid + "\x00" + rsvp.CommunityId
			if existing, ok := byProvisional[key]; !ok || rsvp.LastUpdatedUnixSec > existing.LastUpdatedUnixSec {
				byProvisional[key] = rsvp
			}
			continue
		}
		if rsvp.UserId == "" {
			continue
		}
		if existing, ok := byUser[rsvp.UserId]; !ok || rsvp.LastUpdatedUnixSec > existing.LastUpdatedUnixSec {
			byUser[rsvp.UserId] = rsvp
		}
	}

	successCount := 0
	for _, record := range req.Msg.Attendance {
		isProvisional := record.ProvisionalUserId != nil && *record.ProvisionalUserId != ""

		var rsvp *models.ExperienceRSVP
		if isProvisional {
			rsvp = byProvisional[*record.ProvisionalUserId+"\x00"+req.Msg.CommunityId]
		} else {
			rsvp = byUser[record.UserId]
			if rsvp != nil && rsvp.CommunityId != req.Msg.CommunityId {
				// Cross-community attendee: their RSVP lives in a different community
				// than this request carries. Normal — the roster is event-scoped — so
				// record attendance on the row we found instead of failing (#2690).
				logger.DebugContext(ctx, "recording attendance for cross-community RSVP",
					"user_id", record.UserId,
					"rsvp_community_id", rsvp.CommunityId)
			}
		}

		if rsvp == nil {
			// No RSVP yet for this attendee — create one in the request's community.
			// Registered: a participant added at completion time (past-event flow)
			// who never RSVP'd. Provisional: a non-registered attendee named at
			// completion time.
			rsvp = &models.ExperienceRSVP{
				ExperienceId:       req.Msg.ExperienceId,
				CommunityId:        req.Msg.CommunityId,
				RsvpedAtUnixSec:    now,
				LastUpdatedUnixSec: now,
			}
			rsvp.Intention = models.RSVPIntention_RSVP_INTENTION_YES
			rsvp.Attended = convertAttendedStatusToModel(record.Attended)
			if isProvisional {
				rsvp.ProvisionalUserId = record.ProvisionalUserId
			} else {
				rsvp.UserId = record.UserId
			}
			if _, err := s.storage.Insert(ctx, rsvp); err != nil {
				logger.WarnContext(ctx, "failed to insert RSVP for attendance",
					"user_id", record.UserId,
					"provisional_user_id", record.GetProvisionalUserId(),
					"error", err)
				continue // Skip this attendee, continue with others
			}
			successCount++
			continue
		}

		// Update attendance status on the resolved RSVP.
		rsvp.Attended = convertAttendedStatusToModel(record.Attended)
		rsvp.LastUpdatedUnixSec = now
		if err := s.storage.Update(ctx, rsvp); err != nil {
			logger.WarnContext(ctx, "failed to update attendance",
				"user_id", record.UserId,
				"provisional_user_id", record.GetProvisionalUserId(),
				"error", err)
			continue // Skip this attendee, continue with others
		}
		successCount++
	}

	logger.InfoContext(ctx, "attendance recorded successfully", "success_count", successCount)

	return connect.NewResponse(&api.RecordAttendanceResponse{}), nil
}

// convertRSVPIntentionToModel converts the API enum to its storage twin.
// This and its Attended sibling are the only places the API enums cross into
// storage; per docs/proto_conventions.md the two schemas meet in a service
// package and nowhere else.
func convertRSVPIntentionToModel(intention api.RSVPIntention) models.RSVPIntention {
	switch intention {
	case api.RSVPIntention_RSVP_INTENTION_YES:
		return models.RSVPIntention_RSVP_INTENTION_YES
	case api.RSVPIntention_RSVP_INTENTION_MAYBE:
		return models.RSVPIntention_RSVP_INTENTION_MAYBE
	case api.RSVPIntention_RSVP_INTENTION_NO:
		return models.RSVPIntention_RSVP_INTENTION_NO
	default:
		return models.RSVPIntention_RSVP_INTENTION_UNSPECIFIED
	}
}

// convertAttendedStatusToModel converts the API enum to its storage twin.
func convertAttendedStatusToModel(status api.AttendedStatus) models.AttendedStatus {
	switch status {
	case api.AttendedStatus_ATTENDED_STATUS_YES:
		return models.AttendedStatus_ATTENDED_STATUS_YES
	case api.AttendedStatus_ATTENDED_STATUS_NO:
		return models.AttendedStatus_ATTENDED_STATUS_NO
	case api.AttendedStatus_ATTENDED_STATUS_UNKNOWN:
		return models.AttendedStatus_ATTENDED_STATUS_UNKNOWN
	default:
		return models.AttendedStatus_ATTENDED_STATUS_UNSPECIFIED
	}
}
