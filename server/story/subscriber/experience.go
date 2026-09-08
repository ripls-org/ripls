package story_subscriber

import (
	"context"
	"errors"
	"time"

	"go.ripls.org/ripls/server/ai"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
	"go.ripls.org/ripls/server/story"
)

// handleExperienceCompleted generates a story per community the experience
// is shared with when an experience reaches the COMPLETED state. Moved
// verbatim (with minor adaptation) from
// services/experience/lifecycle.go::generateStoryForCommunity.
//
// The publisher emits one EXPERIENCE_COMPLETED event per CommunityExperience
// link, so this handler runs once per community automatically — no per-
// community fan-out loop needed inside the subscriber.
func (s *Subscriber) handleExperienceCompleted(ctx context.Context, evt *cebus.PublishedEvent) {
	experience := evt.Experience
	if experience == nil {
		return
	}
	communityID := evt.Event.CommunityId

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "story_subscriber.experience",
		"experience_id", experience.Id,
		"community_id", communityID,
	)

	// Get all attendees (RSVPs with attended = YES) for this experience in
	// this community.
	rsvps, err := storage.QueryByFields[*models.ExperienceRSVP](s.storage, ctx, map[string]any{
		"experience_id": experience.Id,
		"community_id":  communityID,
		"attended":      int32(models.AttendedStatus_ATTENDED_STATUS_YES),
	})
	if err != nil {
		logger.WarnContext(ctx, "failed to query RSVPs", "error", err)
		return
	}
	if len(rsvps) == 0 {
		logger.InfoContext(ctx, "no attendees for experience, skipping story")
		return
	}

	// Separate regular and provisional attendees; collect IDs for batch fetching.
	userIDs := make([]string, 0, len(rsvps)+1)
	provisionalIDs := make([]string, 0)
	userIDs = append(userIDs, experience.OwnerId)
	for _, rsvp := range rsvps {
		if rsvp.ProvisionalUserId != nil && *rsvp.ProvisionalUserId != "" {
			provisionalIDs = append(provisionalIDs, *rsvp.ProvisionalUserId)
		} else if rsvp.UserId != "" && rsvp.UserId != experience.OwnerId {
			userIDs = append(userIDs, rsvp.UserId)
		}
	}

	userMap, err := storage.GetByIDs[*models.User](s.storage, ctx, userIDs)
	if err != nil {
		logger.WarnContext(ctx, "failed to batch-fetch users for experience story", "error", err)
		return
	}
	provisionalMap, err := storage.GetByIDs[*models.ProvisionalUser](s.storage, ctx, provisionalIDs)
	if err != nil {
		logger.WarnContext(ctx, "failed to batch-fetch provisional users for experience story", "error", err)
		return
	}

	// Build participant lists: real users first (owner then attendees), then
	// provisional users.
	participantIDs := make([]string, 0, len(userIDs))
	participantNames := make([]string, 0, len(userIDs)+len(provisionalIDs))
	for _, uid := range userIDs {
		user, ok := userMap[uid]
		if !ok {
			if uid == experience.OwnerId {
				logger.WarnContext(ctx, "owner not found for experience story", "owner_id", uid)
				return
			}
			logger.ErrorContext(ctx, "user not found for RSVP", "user_id", uid)
			continue
		}
		participantIDs = append(participantIDs, user.Id)
		participantNames = append(participantNames, user.Name)
	}
	for _, sid := range provisionalIDs {
		su, ok := provisionalMap[sid]
		if !ok {
			logger.ErrorContext(ctx, "provisional user not found for RSVP", "provisional_user_id", sid)
			continue
		}
		// Provisional users are not real accounts so they have no participant
		// ID, but their name should appear in the story content.
		participantNames = append(participantNames, su.Name)
	}

	logger.InfoContext(ctx, "story generation triggered",
		"story_type", ai.StoryTypeExperienceConcluded,
		"participant_count", len(participantIDs),
	)

	ie := impact_metrics.ModelsImpactToAPI(experience.ImpactEstimate)
	storyReq := story.CreateStoryRequest{
		CommunityID:         communityID,
		StoryType:           ai.StoryTypeExperienceConcluded,
		RelatedEntityID:     experience.Id,
		RelatedEntityName:   experience.Name,
		RelatedExperienceID: experience.Id,
		ParticipantIDs:      participantIDs,
		ParticipantNames:    participantNames,
		MediaIDs:            experience.MediaIds,
		CostSavedUSD:        impact_metrics.CostUSD(ie),
		TimeSavedMinutes:    impact_metrics.QualityTimeMinutes(ie),
		Co2SavedGrams:       impact_metrics.Co2Grams(ie),
		CommunityEventID:    evt.Event.Id,
		Kicker:              formatStoryKicker(ctx, s.storage, experience),
	}

	storyStart := time.Now()
	createdStory, err := ai.CallWithTimeout(ctx, 60*time.Second, func(ctx context.Context) (*models.Story, error) {
		return s.creator.CreateStory(ctx, storyReq)
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			logger.WarnContext(ctx, "LLM call exceeded timeout budget",
				"external_service", "ai_provider",
				"operation", "CreateStory",
				"duration_ms", time.Since(storyStart).Milliseconds(),
				"error", err)
		} else {
			logger.WarnContext(ctx, "failed to create story for experience", "error", err)
		}
		return
	}
	logger.InfoContext(ctx, "story created for completed experience", "story_id", createdStory.Id)

	// Materialize the ESM voting prompt embedded in the story. Failure is
	// logged but does not roll back the story — the story degrades
	// gracefully without an embedded prompt rather than disappearing.
	if err := materializeESMPromptForStory(ctx, s.storage, experience, communityID, createdStory.Id, time.Now()); err != nil {
		logger.ErrorContext(ctx, "failed to materialize ESM prompt for story", "error", err)
	}
}
