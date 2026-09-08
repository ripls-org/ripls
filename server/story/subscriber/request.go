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

// handleRequestFulfilled generates a story when a request is fulfilled.
// Moved verbatim from
// services/request/lifecycle.go::generateStoryForFulfilledRequest. The
// publisher emits one REQUEST_FULFILLED event per CommunityRequest link,
// so this handler runs once per community automatically.
func (s *Subscriber) handleRequestFulfilled(ctx context.Context, evt *cebus.PublishedEvent) {
	request := evt.Request
	if request == nil {
		return
	}
	communityID := evt.Event.CommunityId

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "story_subscriber.request",
		"target_request_id", request.Id,
		"community_id", communityID,
	)

	// Determine helper IDs: prefer the confirmed set chosen by the
	// requester; fall back to all non-withdrawn offers (capped at 2)
	// when none were confirmed.
	var helperIDs []string
	if len(request.ConfirmedHelperIds) > 0 {
		if len(request.ConfirmedHelperIds) > 2 {
			helperIDs = request.ConfirmedHelperIds[:2]
		} else {
			helperIDs = request.ConfirmedHelperIds
		}
	} else {
		offers, err := storage.QueryByFields[*models.RequestOffer](s.storage, ctx, map[string]any{
			"request_id": request.Id,
			"withdrawn":  false,
		})
		if err != nil {
			logger.WarnContext(ctx, "failed to query offers", "error", err)
			// Continue without offers — story can still be generated.
		}
		for _, offer := range offers {
			if len(helperIDs) >= 2 {
				break
			}
			helperIDs = append(helperIDs, offer.UserId)
		}
	}

	// Collect all user IDs to batch-fetch (requester + helpers).
	userIDs := append([]string{request.RequesterId}, helperIDs...)

	userMap, err := storage.GetByIDs[*models.User](s.storage, ctx, userIDs)
	if err != nil {
		logger.WarnContext(ctx, "failed to batch-fetch users for request story", "error", err)
		return
	}

	participantIDs := make([]string, 0, len(userIDs))
	participantNames := make([]string, 0, len(userIDs))
	for _, uid := range userIDs {
		user, ok := userMap[uid]
		if !ok {
			if uid == request.RequesterId {
				logger.WarnContext(ctx, "requester not found for request story", "requester_id", uid)
				return
			}
			logger.WarnContext(ctx, "helper user not found for request story", "user_id", uid)
			continue
		}
		participantIDs = append(participantIDs, user.Id)
		participantNames = append(participantNames, user.Name)
	}

	logger.InfoContext(ctx, "story generation triggered",
		"story_type", ai.StoryTypeRequestFulfilled,
		"trigger", "request_fulfillment",
	)

	ie := impact_metrics.ModelsImpactToAPI(request.ImpactEstimate)
	storyReq := story.CreateStoryRequest{
		CommunityID:       communityID,
		StoryType:         ai.StoryTypeRequestFulfilled,
		ParticipantIDs:    participantIDs,
		ParticipantNames:  participantNames,
		MediaIDs:          request.MediaIds,
		RelatedEntityID:   request.Id,
		RelatedEntityName: request.Title,
		RelatedRequestID:  request.Id,
		CostSavedUSD:      impact_metrics.CostUSD(ie),
		TimeSavedMinutes:  impact_metrics.QualityTimeMinutes(ie),
		Co2SavedGrams:     impact_metrics.Co2Grams(ie),
		CommunityEventID:  evt.Event.Id,
	}

	storyStart := time.Now()
	if _, err := ai.CallWithTimeout(ctx, 60*time.Second, func(ctx context.Context) (*models.Story, error) {
		return s.creator.CreateStory(ctx, storyReq)
	}); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			logger.WarnContext(ctx, "LLM call exceeded timeout budget",
				"external_service", "ai_provider",
				"operation", "CreateStory",
				"duration_ms", time.Since(storyStart).Milliseconds(),
				"error", err)
		} else {
			logger.WarnContext(ctx, "failed to create story for request", "error", err)
		}
		return
	}
	logger.InfoContext(ctx, "story created for fulfilled request")
}
