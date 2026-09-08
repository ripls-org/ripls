package feed

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

// ListStories returns stories for a community, ordered by creation time (newest first).
func (s *Service) ListStories(
	ctx context.Context,
	req *connect.Request[api.ListStoriesRequest],
) (*connect.Response[api.ListStoriesResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ListStories",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_id", req.Msg.CommunityId,
		"limit", req.Msg.Limit,
	)

	logger.InfoContext(ctx, "listing community stories")

	// Verify community is active and caller is a member (single round-trip).
	if _, _, memberErr := auth.RequireMemberOfActiveCommunity(ctx, s.sqlStorage, req.Msg.CommunityId, authInfo.UserID); memberErr != nil {
		return nil, memberErr
	}

	// Set default limit and enforce max
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 10 // default limit
	}
	if limit > 50 {
		limit = 50 // max limit
	}

	// Fetch stories from storage
	modelStories, err := s.storyStorage.ListByCommunity(ctx, req.Msg.CommunityId, limit)
	if err != nil {
		logger.Error("failed to list stories", "error", err)
		return nil, connecterr.Internal(ctx, "ListStories", err)
	}

	// Batch-convert stories with a single user fetch across all stories
	apiStories, err := s.convertStoriesToAPI(ctx, modelStories, authInfo.UserID)
	if err != nil {
		logger.Error("failed to convert stories", "error", err)
		return nil, connecterr.Internal(ctx, "ListStories", err)
	}

	s.enrichStoriesWithEmbeddedESM(ctx, apiStories, authInfo.UserID)

	logger.InfoContext(ctx, "successfully listed stories",
		"story_count", len(apiStories),
	)

	return connect.NewResponse(&api.ListStoriesResponse{
		Stories: apiStories,
	}), nil
}

// resolveItemKind resolves an item ID to its entity kind and owner ID by
// probing CommunityGear, CommunityExperience, and CommunityRequest in order.
// Returns NotFound if no community-scoped owner row exists for the item.
// Reads directly from storage without crossing service boundaries.
func resolveItemKind(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	itemID string,
) (auth.EntityKind, string, error) {
	// Try CommunityGear first.
	cgRows, err := s.QueryByField(ctx, "gear_id", itemID, &models.CommunityGear{})
	if err != nil {
		return 0, "", fmt.Errorf("query community gear: %w", err)
	}
	if len(cgRows) > 0 {
		// Fetch the gear to get its owner ID.
		gear := &models.Gear{}
		if err := s.GetByID(ctx, itemID, gear); err == nil {
			return auth.EntityGear, gear.OwnerId, nil
		}
	}

	// Try CommunityExperience.
	ceRows, err := s.QueryByField(ctx, "experience_id", itemID, &models.CommunityExperience{})
	if err != nil {
		return 0, "", fmt.Errorf("query community experience: %w", err)
	}
	if len(ceRows) > 0 {
		exp := &models.Experience{}
		if err := s.GetByID(ctx, itemID, exp); err == nil {
			return auth.EntityExperience, exp.OwnerId, nil
		}
	}

	// Try CommunityRequest.
	crRows, err := s.QueryByField(ctx, "request_id", itemID, &models.CommunityRequest{})
	if err != nil {
		return 0, "", fmt.Errorf("query community request: %w", err)
	}
	if len(crRows) > 0 {
		req := &models.Request{}
		if err := s.GetByID(ctx, itemID, req); err == nil {
			return auth.EntityRequest, req.RequesterId, nil
		}
	}

	return 0, "", connect.NewError(connect.CodeNotFound, fmt.Errorf("item not found"))
}

// ListItemStories returns stories for a specific item (gear, request, or experience), ordered by creation time (newest first).
func (s *Service) ListItemStories(
	ctx context.Context,
	req *connect.Request[api.ListItemStoriesRequest],
) (*connect.Response[api.ListItemStoriesResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ListItemStories",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"item_id", req.Msg.ItemId,
		"limit", req.Msg.Limit,
	)

	logger.InfoContext(ctx, "listing item stories")

	// Resolve the item to its entity kind and owner, then verify the caller is
	// authorized to read stories for this item.
	kind, ownerID, err := resolveItemKind(ctx, s.sqlStorage, req.Msg.ItemId)
	if err != nil {
		return nil, err
	}
	// Read gate: stories about a completed giveaway must stay readable even
	// though completion archived every share of the gear (#2695).
	if _, _, err := auth.RequireReadAccessToCommunityScopedEntity(
		ctx, s.sqlStorage, authInfo.UserID,
		kind, req.Msg.ItemId, ownerID,
	); err != nil {
		return nil, err
	}

	// Set default limit and enforce max
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 10 // default limit
	}
	if limit > 50 {
		limit = 50 // max limit
	}

	// Fetch stories from storage
	modelStories, err := s.storyStorage.ListByItem(ctx, req.Msg.ItemId, limit)
	if err != nil {
		logger.Error("failed to list item stories", "error", err)
		return nil, connecterr.Internal(ctx, "ListItemStories", err)
	}

	// Batch-convert stories with a single user fetch across all stories
	apiStories, err := s.convertStoriesToAPI(ctx, modelStories, authInfo.UserID)
	if err != nil {
		logger.Error("failed to convert stories", "error", err)
		return nil, connecterr.Internal(ctx, "ListItemStories", err)
	}

	s.enrichStoriesWithEmbeddedESM(ctx, apiStories, authInfo.UserID)

	logger.InfoContext(ctx, "successfully listed item stories",
		"story_count", len(apiStories),
	)

	return connect.NewResponse(&api.ListItemStoriesResponse{
		Stories: apiStories,
	}), nil
}

// convertStoriesToAPI batch-converts model stories to API stories.
// Collects all participant IDs across all stories, fetches them in a single batch,
// then distributes to each story. viewerID is the requesting user; when a story
// has a community_event_id whose actor matches, the API story is populated with
// an UndoableAction so the client can render an undo affordance.
func (s *Service) convertStoriesToAPI(ctx context.Context, stories []*models.Story, viewerID string) ([]*api.StoryPayload, error) {
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
	userMap, err := services.FetchAPIUsersBatch(ctx, s.sqlStorage, allIDs)
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

		// Convert impact metrics if present. Story.TimeSavedMinutes is a single
		// scalar, but for experience/request stories it actually carries the
		// QualityTime composite (set by lifecycle when committing). Route it
		// to the matching ImpactEstimate field so clients can format it
		// consistently with the completion modal and detail modals.
		var impactEst *api.ImpactEstimate
		if story.CostSavedUsd > 0 || story.TimeSavedMinutes > 0 || story.Co2SavedGrams > 0 {
			impactEst = &api.ImpactEstimate{
				MoneySaved: &api.MoneySavings{ValueUsd: &api.Estimate{Mean: story.CostSavedUsd}},
				EmissionsPrevented: &api.PreventedEmissions{
					ManufactureAvoidedCarbon: &api.CarbonEstimate{Co2EGrams: &api.Estimate{Mean: story.Co2SavedGrams}},
				},
			}
			if isQualityTimeStoryType(story.StoryType) {
				impactEst.QualityTime = &api.QualityTimeEstimate{
					QualityTimeMinutes: &api.Estimate{Mean: story.TimeSavedMinutes},
				}
			} else {
				impactEst.TimeSaved = &api.TimeSavings{
					Minutes: &api.Estimate{Mean: story.TimeSavedMinutes},
				}
			}
		}

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
			Impact:         impactEst,
			UndoableAction: s.computeUndoableAction(ctx, story, viewerID, logger),
			Kicker:         story.Kicker,
			TemplateKey:    story.TemplateKey,
			TemplateParams: story.TemplateParams,
		})
	}

	return apiStories, nil
}

// computeUndoableAction returns an UndoableAction when the viewer can
// still reverse the action this story commemorates, or nil otherwise.
// Load-bearing checks: the story has a CommunityEvent reference, the
// viewer is the event's actor, and no retraction event for this
// action already exists.
func (s *Service) computeUndoableAction(ctx context.Context, story *models.Story, viewerID string, logger *logging.Logger) *api.UndoableAction {
	if story.CommunityEventId == nil || *story.CommunityEventId == "" {
		return nil
	}
	event := &models.CommunityEvent{}
	if err := s.sqlStorage.GetByID(ctx, *story.CommunityEventId, event); err != nil {
		// Event deleted or not found — silently omit the affordance.
		logger.Debug("story community_event_id not found, no undo affordance",
			"story_id", story.Id, "error", err)
		return nil
	}
	if event.ActorId != viewerID {
		return nil
	}

	retractionType, label := undoableMetadataForEvent(event)
	if retractionType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_UNSPECIFIED {
		return nil
	}

	// Check if a retraction already exists for this action. Scoped by
	// the event's topic (transfer_id / request_id / experience_id).
	topicField, topicValue := topicFilterForEvent(event)
	if topicField == "" {
		return nil
	}
	candidates, err := s.sqlStorage.QueryByField(ctx, topicField, topicValue, &models.CommunityEvent{})
	if err != nil {
		logger.Warn("failed to query for retraction event", "story_id", story.Id, "error", err)
		return nil
	}
	for _, msg := range candidates {
		ev := msg.(*models.CommunityEvent)
		if ev.EventType == retractionType && ev.OccurredAtUnixSec >= event.OccurredAtUnixSec {
			return nil
		}
	}

	return &api.UndoableAction{
		CommunityEventId: event.Id,
		Label:            label,
	}
}

// undoableMetadataForEvent returns the retraction event type and a
// human-readable label for the Undo affordance. Returns UNSPECIFIED
// for events that are not server-authoritative undoable.
func undoableMetadataForEvent(event *models.CommunityEvent) (models.CommunityEventType, string) {
	switch event.EventType {
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED:
		switch event.TransferType {
		case models.TransferType_TRANSFER_TYPE_LOAN:
			return models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED_UNDONE, "Undo marking as returned"
		case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
			return models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED_UNDONE, "Undo completing giveaway"
		}
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED:
		return models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED_UNDONE, "Undo marking fulfilled"
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED:
		return models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED_UNDONE, "Undo completing experience"
	}
	return models.CommunityEventType_COMMUNITY_EVENT_TYPE_UNSPECIFIED, ""
}

// topicFilterForEvent returns the (field, value) pair to use when
// searching for retraction events associated with event.
func topicFilterForEvent(event *models.CommunityEvent) (string, string) {
	switch t := event.Topic.(type) {
	case *models.CommunityEvent_TransferId:
		return "transfer_id", t.TransferId
	case *models.CommunityEvent_RequestId:
		return "request_id", t.RequestId
	case *models.CommunityEvent_ExperienceId:
		return "experience_id", t.ExperienceId
	}
	return "", ""
}

// isQualityTimeStoryType reports whether a story's TimeSavedMinutes scalar
// carries a QualityTime composite (true) or a true TimeSaved value (false).
// Experience and request lifecycles write QT minutes into TimeSavedMinutes
// at commit time; gear lifecycles write actual shopping-time-saved.
func isQualityTimeStoryType(storyType string) bool {
	switch storyType {
	case ai.StoryTypeExperienceConcluded, ai.StoryTypeRequestFulfilled:
		return true
	default:
		return false
	}
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
