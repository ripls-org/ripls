package user

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// ListUserStories returns stories for a user where they are a participant,
// ordered by creation time (newest first).
func (s *Service) ListUserStories(
	ctx context.Context,
	req *connect.Request[api.ListUserStoriesRequest],
) (*connect.Response[api.ListUserStoriesResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ListUserStories",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"target_user_id", req.Msg.UserId,
		"limit", req.Msg.Limit,
	)

	logger.InfoContext(ctx, "listing user stories")

	// Set default limit and enforce max
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 10 // default limit
	}
	if limit > 50 {
		limit = 50 // max limit
	}

	// Fetch stories from storage
	modelStories, err := s.storyStorage.ListByUser(ctx, req.Msg.UserId, limit)
	if err != nil {
		logger.Error("failed to list user stories", "error", err)
		return nil, connecterr.Internal(ctx, "ListUserStories", err)
	}

	// Batch-convert stories with a single user fetch across all stories
	apiStories, err := convertStoriesToAPI(ctx, s.storage, modelStories)
	if err != nil {
		logger.Error("failed to convert stories", "error", err)
		return nil, connecterr.Internal(ctx, "ListUserStories", err)
	}

	logger.InfoContext(ctx, "successfully listed user stories",
		"story_count", len(apiStories),
	)

	return connect.NewResponse(&api.ListUserStoriesResponse{
		Stories: apiStories,
	}), nil
}

// convertStoriesToAPI batch-converts model stories to API stories.
// Collects all participant IDs across all stories, fetches them in a single batch,
// then distributes to each story.
func convertStoriesToAPI(ctx context.Context, sqlStorage *storage.ProtoSQLStorage, stories []*models.Story) ([]*api.StoryPayload, error) {
	logger := logging.LoggerWithContext(ctx).With("operation", "convertStoriesToAPI")

	// Collect all unique participant IDs across all stories
	uniqueIDs := make(map[string]struct{})
	for _, story := range stories {
		for _, pid := range story.ParticipantIds {
			uniqueIDs[pid] = struct{}{}
		}
	}

	allIDs := make([]string, 0, len(uniqueIDs))
	for id := range uniqueIDs {
		allIDs = append(allIDs, id)
	}

	// Batch-fetch all users in a single query
	userMap, err := services.FetchAPIUsersBatch(ctx, sqlStorage, allIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to batch-fetch story participants: %w", err)
	}

	// Convert each story using the pre-fetched user map
	apiStories := make([]*api.StoryPayload, 0, len(stories))
	for _, story := range stories {
		participants := make([]*api.User, 0, len(story.ParticipantIds))
		for _, pid := range story.ParticipantIds {
			user, ok := userMap[pid]
			if !ok {
				logger.Warn("participant not found, skipping",
					"story_id", story.Id,
					"participant_id", pid,
				)
				continue
			}
			participants = append(participants, user)
		}

		storyType := convertStoryTypeToAPI(story.StoryType)

		apiStories = append(apiStories, &api.StoryPayload{
			StoryType:      storyType,
			Title:          story.Title,
			Description:    story.Description,
			MediaIds:       story.MediaIds,
			Participants:   participants,
			GearId:         story.GearId,
			LoanId:         story.LoanId,
			ExperienceId:   story.ExperienceId,
			RequestId:      story.RequestId,
			Kicker:         story.Kicker,
			TemplateKey:    story.TemplateKey,
			TemplateParams: story.TemplateParams,
		})
	}

	return apiStories, nil
}

// convertStoryTypeToAPI converts a story type string to the API enum.
func convertStoryTypeToAPI(storyType string) api.StoryType {
	switch storyType {
	case ai.StoryTypeLoanCompleted:
		return api.StoryType_STORY_TYPE_LOAN_COMPLETED
	case ai.StoryTypeGiveawayCompleted:
		return api.StoryType_STORY_TYPE_GIVEAWAY_COMPLETED
	case ai.StoryTypeExperienceConcluded:
		return api.StoryType_STORY_TYPE_EXPERIENCE_CONCLUDED
	case ai.StoryTypeRequestFulfilled:
		return api.StoryType_STORY_TYPE_REQUEST_FULFILLED
	case ai.StoryTypeNewMemberWelcome:
		return api.StoryType_STORY_TYPE_NEW_MEMBER_WELCOME
	case ai.StoryTypeExperienceRSVP:
		return api.StoryType_STORY_TYPE_EXPERIENCE_RSVP
	default:
		return api.StoryType_STORY_TYPE_UNSPECIFIED
	}
}
