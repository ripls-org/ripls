package experience

import (
	"context"
	"errors"
	"sync"
	stdtime "time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/category"
	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// SaveExperience inserts new experience or updates existing experience in the database.
// If req.Msg.Id is empty, creates new experience. Otherwise, updates existing experience.
// Only fields present in the request are updated; omitted fields are ignored.
// Repeated fields are replaced entirely, not appended.
func (s *Service) SaveExperience(
	ctx context.Context,
	req *connect.Request[api.SaveExperienceRequest],
) (*connect.Response[api.SaveExperienceResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"owner_id", authInfo.UserID,
	)

	if req.Msg.Time != nil {
		if err := validateExperienceTimeTimezone(req.Msg.Time); err != nil {
			return nil, err
		}
	}

	// Determine if this is an insert or update
	isInsert := req.Msg.Id == nil || *req.Msg.Id == ""

	if isInsert {
		// Default to TBD time if not specified
		time := req.Msg.Time
		if time == nil {
			time = &api.ExperienceTime{
				TimeType: &api.ExperienceTime_Tbd{
					Tbd: &api.TimeTBD{},
				},
			}
		}

		// Detect if this experience is a retroactive past entry.
		// If the time is explicitly in the past, skip the normal ACTIVE/JOINED states
		// and start directly in IN_PROCESS so the client can immediately record attendance.
		storedTime := convertTimeAPIToModels(time)
		now := clock.UnixSec(ctx)
		initialState := models.ExperienceState_EXPERIENCE_STATE_ACTIVE
		var startedAt *int64
		if isPastTime(storedTime, now) {
			initialState = models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS
			startedAt = &now
			logger.Info("past experience detected — setting initial state to IN_PROCESS")
		}

		// Create storage model from request
		expStored := &models.Experience{
			Name:             req.Msg.Name,
			Description:      req.Msg.Description,
			MediaIds:         req.Msg.MediaIds,
			LocationId:       req.Msg.LocationId,
			Time:             storedTime,
			MaxParticipants:  req.Msg.MaxParticipants,
			OwnerId:          authInfo.UserID,
			State:            initialState,
			StartedAtUnixSec: startedAt,
			ConversationId:   "",                // Set when shared to community
			SourceUrl:        req.Msg.SourceUrl, // Source URL from website creation
			CreatedAtUnixSec: now,
		}

		// Extract and store metadata from Gen response (value estimate, category).
		var valueUSD float32
		if req.Msg.Metadata != nil {
			valueEstimate, category := convertAPIExperienceMetadataToStorage(req.Msg.Metadata)
			expStored.ValueEstimate = valueEstimate
			expStored.Category = category

			if valueEstimate != nil {
				valueUSD = valueEstimate.EstimatedValueUsd
				logger.Info("storing experience value estimate",
					"value_usd", valueEstimate.EstimatedValueUsd)
			}
		}

		// Fallback: the experience-AI Gen path doesn't yet populate
		// Category (#2013). Derive a coarse label
		// from name+description so the `known_for` derivation can
		// surface "{Category} host" chips. Remove once Gen sets
		// Category itself.
		if expStored.Category == "" {
			expStored.Category = category.Categorize(
				expStored.Name, expStored.Description,
			)
		}

		// Store owner-provided social context if present.
		if req.Msg.SocialContext != nil {
			expStored.SocialContext = convertAPISocialContextToModels(req.Msg.SocialContext)
		}

		// Set initial ImpactEstimate before insert so it's persisted immediately.
		// Pass user-set duration and social context as hints so they're reflected from the start.
		if s.estimatorCfg != nil {
			hint := buildHintForSave(expStored, req.Msg.SocialContext)
			ie := impact_metrics.BuildExperienceImpactMetrics(valueUSD, 0, s.estimatorCfg, nil, hint)
			expStored.ImpactEstimate = impact_metrics.APIImpactToModels(ie)
		}

		// Insert into database
		id, err := s.storage.Insert(ctx, expStored)
		if err != nil {
			logger.Error("failed to insert experience", "error", err)
			return nil, connecterr.Internal(ctx, "SaveExperience", err)
		}
		expStored.Id = id

		// Run both LLM calls in parallel, blocking until both complete so the response
		// includes the refined ImpactEstimate and Suggestions rather than config defaults.
		if !clock.IsSimulated(ctx) && (expStored.Name != "" || expStored.Description != "") {
			var wg sync.WaitGroup
			var inference *ai.SocialAttributeInference
			var suggestions *ai.ExperienceSuggestionResult

			if s.estimatorCfg != nil && s.aiProvider != nil {
				wg.Add(1)
				name, desc := expStored.Name, expStored.Description
				logging.GoSafe(ctx, "save-experience-infer-social-attributes", func() {
					defer wg.Done()
					start := stdtime.Now()
					result, inferErr := ai.CallWithTimeout(ctx, 30*stdtime.Second, func(ctx context.Context) (*ai.SocialAttributeInference, error) {
						return s.aiProvider.InferSocialAttributes(ctx, name, desc, "experience", valueUSD)
					})
					if inferErr != nil {
						if errors.Is(inferErr, context.DeadlineExceeded) {
							logger.WarnContext(ctx, "LLM call exceeded timeout budget, keeping config defaults",
								"external_service", "ai_provider",
								"operation", "InferSocialAttributes",
								"duration_ms", stdtime.Since(start).Milliseconds(),
								"error", inferErr)
						} else {
							logger.WarnContext(ctx, "LLM inference at creation failed, keeping config defaults", "error", inferErr)
						}
						return
					}
					inference = result
				})
			}

			if s.aiProvider != nil {
				wg.Add(1)
				name, desc, cat := expStored.Name, expStored.Description, expStored.Category
				logging.GoSafe(ctx, "save-experience-generate-suggestions", func() {
					defer wg.Done()
					sugStart := stdtime.Now()
					result, sugErr := ai.CallWithTimeout(ctx, 30*stdtime.Second, func(ctx context.Context) (*ai.ExperienceSuggestionResult, error) {
						return s.aiProvider.GenerateExperienceSuggestions(ctx, name, desc, cat)
					})
					if sugErr != nil {
						if errors.Is(sugErr, context.DeadlineExceeded) {
							logger.WarnContext(ctx, "LLM call exceeded timeout budget, skipping suggestions",
								"external_service", "ai_provider",
								"operation", "GenerateExperienceSuggestions",
								"duration_ms", stdtime.Since(sugStart).Milliseconds(),
								"error", sugErr)
						} else {
							logger.WarnContext(ctx, "suggestion generation failed at creation, skipping", "error", sugErr)
						}
						return
					}
					suggestions = result
				})
			}

			wg.Wait()

			// Apply inference result to expStored.
			needsUpdate := false
			if inference != nil {
				userDuration := extractExperienceDurationMinutes(expStored)
				var hint *impact_metrics.QualityTimeHint
				if userDuration > 0 {
					hint = &impact_metrics.QualityTimeHint{
						UserDurationMinutes: userDuration,
						VulnerabilityLevel:  inference.VulnerabilityLevel,
						SocialContext:       req.Msg.SocialContext,
					}
				} else if inference.DurationMinutes > 0 || inference.VulnerabilityLevel != "" {
					hint = &impact_metrics.QualityTimeHint{
						DurationMinutes:    inference.DurationMinutes,
						VulnerabilityLevel: inference.VulnerabilityLevel,
						SocialContext:      req.Msg.SocialContext,
					}
				}
				if hint != nil {
					ie := impact_metrics.BuildExperienceImpactMetrics(valueUSD, 0, s.estimatorCfg, nil, hint)
					expStored.ImpactEstimate = impact_metrics.APIImpactToModels(ie)
					needsUpdate = true
					logger.DebugContext(ctx, "inferred social attributes at creation",
						"vulnerability", inference.VulnerabilityLevel,
						"duration_minutes", inference.DurationMinutes)
				}
			}

			// Apply suggestions result to expStored.
			if suggestions != nil && len(suggestions.Suggestions) > 0 {
				expStored.Suggestions = suggestions.Suggestions
				if suggestions.CategoryHint != "" {
					expStored.CategoryHint = &suggestions.CategoryHint
				}
				needsUpdate = true
				logger.DebugContext(ctx, "generated suggestion chips at creation",
					"count", len(suggestions.Suggestions),
					"category_hint", suggestions.CategoryHint)
			}

			if needsUpdate {
				if updateErr := s.storage.Update(ctx, expStored); updateErr != nil {
					logger.WarnContext(ctx, "failed to persist LLM results after creation", "error", updateErr)
				}
			}
		}

		// Note: we no longer auto-create an initial TimeProposal mirroring
		// experience.time. The event time itself is the source of truth, and
		// generating a proposal here without a poll_id makes it look like the
		// organizer started a poll on creation (the client groups proposals
		// by poll_id to render the per-poll history). Real poll proposals are
		// created on demand via ProposeTime, which mints a poll_id.

		// Note: Experience is NOT automatically shared with communities.
		// Use ShareExperience RPC to explicitly share experience with specific communities.

		// Fetch the created experience to return it (Id already set above, LLM fields populated)
		apiExp, err := s.buildAPIExperience(ctx, expStored, "")
		if err != nil {
			return nil, err
		}

		// Owner watches the experience for inbox tracking.
		ws := storage.NewWatchStorage(s.storage)
		if err := ws.UpsertWatch(ctx, authInfo.UserID, models.WatchedItemType_WATCHED_ITEM_TYPE_EXPERIENCE, id); err != nil {
			logger.Warn("failed to create watch for experience owner", "error", err)
		}

		// Provision the event's per-item community (#2492) via the shared community
		// library, then share the event into it — so every event is born with its
		// own audience community. The client opens the share/invite sheet against it.
		itemCommunityID, err := community.ProvisionPerItemCommunity(ctx, s.storage, s.bus, authInfo.UserID, community.Origin{ExperienceID: id})
		if err != nil {
			logger.Error("failed to provision per-item community for event", "error", err)
			return nil, connecterr.Internal(ctx, "SaveExperience", err)
		}
		if err := s.shareExperienceToCommunity(ctx, expStored, itemCommunityID, authInfo.UserID); err != nil {
			logger.Error("failed to share event into its per-item community", "error", err)
			return nil, err
		}

		logger.Info("experience created", "experience_id", id, "item_community_id", itemCommunityID)

		resp := &api.SaveExperienceResponse{Experience: apiExp}
		if itemCommunityID != "" {
			resp.ItemCommunityId = &itemCommunityID
		}
		res := connect.NewResponse(resp)
		res.Header().Set("Experience-Server-Version", "v1")
		return res, nil
	}

	// Update existing experience
	experienceID := *req.Msg.Id
	logger = logger.With("experience_id", experienceID)

	expStored, err := s.fetchExperienceForRead(ctx, experienceID, logger.Logger, "UpdateExperience")
	if err != nil {
		return nil, err
	}

	// Track which fields changed (for system messages and LLM re-inference).
	// A request that merely CARRIES the current time is not a change — the
	// "updated time →" system line on a no-op re-save reads as the host
	// second-guessing themselves seconds after creating the event (#2724).
	timeChanged := req.Msg.Time != nil &&
		!proto.Equal(convertTimeAPIToModels(req.Msg.Time), expStored.Time)
	locationChanged := req.Msg.LocationId != "" && req.Msg.LocationId != expStored.LocationId
	textChanged := (req.Msg.Name != "" && req.Msg.Name != expStored.Name) ||
		(req.Msg.Description != "" && req.Msg.Description != expStored.Description)

	// Verify ownership — with a carve-out for non-owners appending media.
	// Any member of a community the experience is shared with may add media
	// (e.g. attendee photos uploaded from the media carousel), as long as
	// they don't try to change any other field and the new media list is a
	// superset of the existing one. All other update paths remain
	// owner-only.
	if expStored.OwnerId != authInfo.UserID {
		appendOnly, appendErr := isAppendOnlyMediaUpdate(req.Msg, expStored, textChanged, timeChanged, locationChanged)
		if appendErr != nil {
			return nil, appendErr
		}
		if !appendOnly {
			return nil, connecterr.UserVisible(
				ctx,
				connect.CodePermissionDenied,
				"experience_owner_required_for_update",
				"only the owner can update this event",
				nil,
			)
		}
		if mayAddErr := s.callerMayAddExperienceMedia(ctx, experienceID, authInfo.UserID); mayAddErr != nil {
			return nil, mayAddErr
		}
	}
	// Update only fields that are present in the request
	// Empty strings are assumed unset.
	// For repeated fields, they are replaced entirely
	if req.Msg.Name != "" {
		expStored.Name = req.Msg.Name
	}
	if req.Msg.Description != "" {
		expStored.Description = req.Msg.Description
	}
	if req.Msg.LocationId != "" {
		expStored.LocationId = req.Msg.LocationId
	}
	timeEdited := req.Msg.Time != nil
	if timeEdited {
		expStored.Time = convertTimeAPIToModels(req.Msg.Time)
	}
	// Ensure time is always set (default to TBD if missing)
	if expStored.Time == nil {
		expStored.Time = &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Tbd{
				Tbd: &models.TimeTBD{},
			},
		}
	}
	// A direct time edit supersedes a prior poll confirmation. ConfirmTime is the
	// only path that keeps a confirmed proposal and experience.time in lockstep
	// (it sets expStored.Time = proposal.Time); editing the time here would
	// otherwise leave a stale IsConfirmed proposal, which the "When" panel shows
	// in place of the real time. Drop any confirmation that no longer matches.
	if timeEdited {
		if err := s.clearStaleTimeConfirmation(ctx, expStored); err != nil {
			logger.ErrorContext(ctx, "failed to clear stale time confirmation", "error", err)
			return nil, connecterr.Internal(ctx, "SaveExperience", err)
		}
	}
	if req.Msg.MaxParticipants != 0 {
		expStored.MaxParticipants = req.Msg.MaxParticipants
	}
	// Source URL can be set on update (though typically only set on create)
	if req.Msg.SourceUrl != nil {
		expStored.SourceUrl = req.Msg.SourceUrl
	}
	// Repeated fields are always replaced (even if empty slice)
	expStored.MediaIds = req.Msg.MediaIds

	// Persist owner-provided social context override when supplied on update.
	if req.Msg.SocialContext != nil {
		expStored.SocialContext = convertAPISocialContextToModels(req.Msg.SocialContext)
	}

	// Recalculate ImpactEstimate so QT reflects the updated duration, RSVPs, and
	// re-inferred social attributes when name or description changed.
	if s.estimatorCfg != nil {
		var valueUSD float32
		if expStored.ValueEstimate != nil {
			valueUSD = expStored.ValueEstimate.EstimatedValueUsd
		}
		// Count current YES RSVPs for group-size estimation.
		rsvpMsgs, rsvpErr := s.storage.QueryByField(ctx, "experience_id", experienceID, &models.ExperienceRSVP{})
		var rsvpCount int32
		if rsvpErr == nil {
			for _, m := range rsvpMsgs {
				if m.(*models.ExperienceRSVP).GetIntention() == models.RSVPIntention_RSVP_INTENTION_YES {
					rsvpCount++
				}
			}
		}

		// Re-infer social attributes when name or description changed.
		// LLM inference is best-effort; failure falls back to existing stored values.
		var llmVulnerability string
		if textChanged && s.aiProvider != nil && !clock.IsSimulated(ctx) {
			updStart := stdtime.Now()
			inference, inferErr := ai.CallWithTimeout(ctx, 30*stdtime.Second, func(ctx context.Context) (*ai.SocialAttributeInference, error) {
				return s.aiProvider.InferSocialAttributes(ctx, expStored.Name, expStored.Description, "experience", valueUSD)
			})
			if inferErr != nil {
				if errors.Is(inferErr, context.DeadlineExceeded) {
					logger.WarnContext(ctx, "LLM call exceeded timeout budget, keeping existing attributes",
						"external_service", "ai_provider",
						"operation", "InferSocialAttributes",
						"duration_ms", stdtime.Since(updStart).Milliseconds(),
						"error", inferErr)
				} else {
					logger.WarnContext(ctx, "LLM re-inference on update failed, keeping existing attributes", "error", inferErr)
				}
			} else if inference != nil {
				llmVulnerability = inference.VulnerabilityLevel
				logger.DebugContext(ctx, "re-inferred social attributes on update",
					"vulnerability", llmVulnerability)
			}
		}

		hint := buildHintForSave(expStored, req.Msg.SocialContext)
		// Resolve vulnerability: prefer fresh LLM result, then existing stored value.
		// (Only applies when social context doesn't already override vulnerability.)
		if req.Msg.SocialContext == nil || req.Msg.SocialContext.Vulnerability == api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_UNSPECIFIED {
			resolvedVulnerability := llmVulnerability
			if resolvedVulnerability == "" && expStored.ImpactEstimate != nil {
				stored := impact_metrics.ModelsImpactToAPI(expStored.ImpactEstimate)
				if stored.QualityTime != nil && stored.QualityTime.Attributes != nil {
					resolvedVulnerability = vulnerabilityLevelToString(stored.QualityTime.Attributes.Vulnerability)
				}
			}
			if resolvedVulnerability != "" {
				if hint == nil {
					hint = &impact_metrics.QualityTimeHint{}
				}
				hint.VulnerabilityLevel = resolvedVulnerability
			}
		}

		ie := impact_metrics.BuildExperienceImpactMetrics(valueUSD, rsvpCount, s.estimatorCfg, nil, hint)
		expStored.ImpactEstimate = impact_metrics.APIImpactToModels(ie)
	}

	// Update in database
	err = s.storage.Update(ctx, expStored)
	if err != nil {
		logger.Error("failed to update experience", "error", err)
		return nil, connecterr.Internal(ctx, "SaveExperience", err)
	}

	logger.Info("experience updated")

	// Emit DETAIL_CHANGED system message when time or location changes.
	if s.systemMessageWriter != nil && (timeChanged || locationChanged) && expStored.ConversationId != "" {
		displayName := s.getUserDisplayName(ctx, authInfo.UserID)
		var detailMsg chat.LocalizedMessage
		if timeChanged {
			detailMsg = chat.DetailChangedTimeMessage(displayName, experienceTimeParam(expStored.Time))
		} else if locationName, ok := s.locationDisplayName(ctx, expStored.LocationId); ok {
			detailMsg = chat.DetailChangedLocationMessage(displayName, locationName)
		} else {
			detailMsg = chat.DetailChangedLocationUnnamedMessage(displayName)
		}
		if err := s.systemMessageWriter.InsertLocalized(
			ctx,
			expStored.ConversationId,
			authInfo.UserID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_DETAIL_CHANGED,
			detailMsg,
		); err != nil {
			logger.WarnContext(ctx, "failed to write DETAIL_CHANGED system message",
				"conversation_id", expStored.ConversationId,
				"error", err)
		}
	}

	// Emit EXPERIENCE_UPDATED per community the experience is shared with
	// when time or location changed. Best-effort: failures are logged but
	// do not fail the save. Mirrors the MarkExperienceInProcess fan-out
	// pattern.
	if timeChanged || locationChanged {
		updatedLogger := logging.LoggerWithContext(ctx).With(
			"operation", "SaveExperience",
			"experience_id", experienceID,
			"time_changed", timeChanged,
			"location_changed", locationChanged,
		)
		communityExperiences, ceErr := s.storage.QueryByField(ctx, "experience_id", experienceID, &models.CommunityExperience{})
		if ceErr != nil {
			updatedLogger.WarnContext(ctx, "failed to query community experiences for EXPERIENCE_UPDATED notification", "error", ceErr)
		} else {
			now := clock.UnixSec(ctx)
			for _, msg := range communityExperiences {
				ce := msg.(*models.CommunityExperience)
				if _, pubErr := s.bus.Publish(ctx, &models.CommunityEvent{
					CommunityId: ce.CommunityId,
					EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED,
					ActorId:     authInfo.UserID,
					Topic: &models.CommunityEvent_ExperienceId{
						ExperienceId: experienceID,
					},
					TimeChanged:       &timeChanged,
					LocationChanged:   &locationChanged,
					OccurredAtUnixSec: now,
				}); pubErr != nil {
					updatedLogger.WarnContext(ctx, "failed to record EXPERIENCE_UPDATED event",
						"community_id", ce.CommunityId,
						"error", pubErr,
					)
				}
			}
		}
	}

	// Build API experience to return (no community context needed)
	apiExp, err := s.buildAPIExperience(ctx, expStored, "")
	if err != nil {
		return nil, err
	}

	res := connect.NewResponse(&api.SaveExperienceResponse{
		Experience: apiExp,
	})
	res.Header().Set("Experience-Server-Version", "v1")
	return res, nil
}

// isPastTime reports whether the given ExperienceTime refers to a moment already in the past.
// Returns true only for SpecificTime with a unix_timestamp_sec strictly before now.
// Range and TBD times are never considered past.
func isPastTime(t *models.ExperienceTime, nowUnixSec int64) bool {
	if t == nil {
		return false
	}
	specific, ok := t.TimeType.(*models.ExperienceTime_Specific)
	if !ok {
		return false
	}
	return specific.Specific.GetUnixTimestampSec() < nowUnixSec
}

// buildHintForSave builds a QualityTimeHint from a stored experience and an
// optional social context override from the request. User-set duration and
// social context are both included when present.
func buildHintForSave(exp *models.Experience, sc *api.SocialContext) *impact_metrics.QualityTimeHint {
	userDuration := extractExperienceDurationMinutes(exp)
	if userDuration == 0 && sc == nil {
		return nil
	}
	hint := &impact_metrics.QualityTimeHint{}
	if userDuration > 0 {
		hint.UserDurationMinutes = userDuration
	}
	hint.SocialContext = sc
	return hint
}

// convertAPISocialContextToModels converts an api.SocialContext to its
// storage representation, encoding enums as int32 to avoid cross-package imports.
func convertAPISocialContextToModels(sc *api.SocialContext) *models.SocialContext {
	if sc == nil {
		return nil
	}
	return &models.SocialContext{
		TieStrength:   int32(sc.TieStrength),
		Reciprocity:   int32(sc.Reciprocity),
		Novelty:       int32(sc.Novelty),
		Vulnerability: int32(sc.Vulnerability),
		Modality:      int32(sc.Modality),
	}
}

// convertModelsSocialContextToAPI converts a stored SocialContext back
// to the API proto, restoring enum types from their int32 storage representation.
func convertModelsSocialContextToAPI(sc *models.SocialContext) *api.SocialContext {
	if sc == nil {
		return nil
	}
	return &api.SocialContext{
		TieStrength:   api.SocialTieStrength(sc.TieStrength),
		Reciprocity:   api.SocialReciprocity(sc.Reciprocity),
		Novelty:       api.SocialNovelty(sc.Novelty),
		Vulnerability: api.SocialVulnerabilityLevel(sc.Vulnerability),
		Modality:      api.SocialModality(sc.Modality),
	}
}

// clearStaleTimeConfirmation drops a time-proposal confirmation that no longer
// matches the experience's time. ConfirmTime keeps a confirmed proposal and
// experience.time in lockstep; a later direct time edit (SaveExperience) moves
// experience.time without touching the proposal, leaving a stale IsConfirmed
// flag that the client's "When" panel renders in place of the real time. Only
// confirmations whose time differs from the current experience.time are
// cleared, so re-saving an unchanged time is a no-op. Mutates exp in place
// (clears TimeProposalsLocked); the caller persists exp afterward.
func (s *Service) clearStaleTimeConfirmation(ctx context.Context, exp *models.Experience) error {
	proposalsRaw, err := s.storage.QueryByField(ctx, "experience_id", exp.Id, &models.ExperienceTimeProposal{})
	if err != nil {
		return err
	}
	cleared := false
	for _, m := range proposalsRaw {
		p := m.(*models.ExperienceTimeProposal)
		if !p.IsConfirmed || proto.Equal(p.Time, exp.Time) {
			continue
		}
		p.IsConfirmed = false
		if err := s.storage.Update(ctx, p); err != nil {
			return err
		}
		cleared = true
	}
	if cleared {
		exp.TimeProposalsLocked = nil
	}
	return nil
}
