package story_subscriber

import (
	"context"
	"errors"
	"time"

	"go.ripls.org/ripls/server/ai"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/story"
)

// handleInvitationLinkUsed generates a welcome story when a user joins a
// community via an invitation link. Moved from
// services/community/invitations.go::generateStoryForNewMember.
//
// The actor (the new member) is denormalized in evt.Actor (api.User);
// the community is the same row pre-fetched on evt.Event.CommunityId,
// but we re-fetch it for the human-readable name field on the story
// request. Both lookups are bounded.
func (s *Subscriber) handleInvitationLinkUsed(ctx context.Context, evt *cebus.PublishedEvent) {
	event := evt.Event
	if evt.Actor == nil || event.CommunityId == "" {
		return
	}
	userID := event.ActorId
	communityID := event.CommunityId

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "story_subscriber.invitation",
		"user_id", userID,
		"community_id", communityID,
	)

	// Fetch community details for the story name field.
	community := &models.Community{}
	if err := s.storage.GetByID(ctx, communityID, community); err != nil {
		logger.WarnContext(ctx, "failed to get community for welcome story", "error", err)
		return
	}

	storyReq := story.CreateStoryRequest{
		CommunityID:        communityID,
		CommunityName:      community.Name,
		StoryType:          ai.StoryTypeNewMemberWelcome,
		RelatedEntityID:    userID,
		RelatedCommunityID: communityID,
		ParticipantIDs:     []string{userID},
		ParticipantNames:   []string{evt.Actor.Name},
	}

	start := time.Now()
	createdStory, err := ai.CallWithTimeout(ctx, 60*time.Second, func(ctx context.Context) (*models.Story, error) {
		return s.creator.CreateStory(ctx, storyReq)
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			logger.WarnContext(ctx, "LLM call exceeded timeout budget",
				"external_service", "ai_provider",
				"operation", "CreateStory",
				"duration_ms", time.Since(start).Milliseconds(),
				"error", err)
		} else {
			logger.WarnContext(ctx, "failed to create welcome story", "error", err)
		}
		return
	}
	logger.InfoContext(ctx, "welcome story created", "story_id", createdStory.Id)
}
