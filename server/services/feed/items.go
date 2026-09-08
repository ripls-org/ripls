package feed

import (
	"context"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
)

// eventToFeedItem converts a CommunityEvent to a FeedItem.
func (s *Service) eventToFeedItem(ctx context.Context, event *models.CommunityEvent, viewerUserID string, fc *feedLookups) *api.FeedItem {
	switch event.EventType {
	// Creation events
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED:
		return s.createGearSharedItem(ctx, event, fc)
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED:
		return s.createRequestCreatedItem(ctx, event, viewerUserID, fc)
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_CREATED:
		return s.createCommunityCreatedItem(ctx, event, viewerUserID, fc)
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED:
		return s.createExperienceCreatedItem(ctx, event, viewerUserID, fc)

	// Transfer lifecycle events — reuse gear card with different item_type
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE:
		return s.createGearSharedItem(ctx, event, fc)

	// Request lifecycle events — reuse request card
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE:
		return s.createRequestCreatedItem(ctx, event, viewerUserID, fc)

	// Experience RSVP events — reuse experience card
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_NO:
		return s.createExperienceCreatedItem(ctx, event, viewerUserID, fc)

	// Terminal events (completed/cancelled) appear as Story cards — skip the event.
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED:
		return nil

	default:
		return nil // Skip unhandled event types
	}
}

// createGearSharedItem creates a feed item for gear sharing events.
func (s *Service) createGearSharedItem(ctx context.Context, event *models.CommunityEvent, fc *feedLookups) *api.FeedItem {
	if event.GearId == "" {
		return nil
	}

	// Look up gear from pre-fetched map (fetched without IncludeDeleted, so deleted gear won't be present)
	gear, ok := fc.gearMap[event.GearId]
	if !ok {
		logging.LoggerWithContext(ctx).Warn("gear not found in batch for feed", "gear_id", event.GearId)
		return nil
	}

	if gear.Deleted != nil {
		logging.LoggerWithContext(ctx).Warn("skipping soft-deleted gear in feed",
			"gear_id", event.GearId,
			"deleted_at", gear.Deleted.DeletedAtUnixSec)
		return nil
	}

	// Gear feed cards always show the gear owner as the actor, regardless of
	// which event (share, interest expressed, recipient selected, active loan)
	// bumped the item to the top of the feed.
	actorUser := fc.userMap[gear.OwnerId]
	if actorUser == nil {
		logging.LoggerWithContext(ctx).Error("gear owner not found in batch", "owner_id", gear.OwnerId)
		return nil
	}

	// Look up CommunityGear from pre-fetched map.
	communityGear, ok := fc.communityGearMap[event.GearId]
	if !ok {
		return nil // Gear has been unshared from this community.
	}
	if communityGear.Archived {
		return nil
	}

	availability := api.Availability(communityGear.Availability)

	return &api.FeedItem{
		Id:                event.Id,
		ItemType:          api.FeedItemType_FEED_ITEM_TYPE_GEAR_SHARED,
		OccurredAtUnixSec: event.OccurredAtUnixSec,
		Payload: &api.FeedItem_GearShared{
			GearShared: &api.GearSharedPayload{
				GearId:       gear.Id,
				GearName:     gear.Name,
				Actor:        actorUser,
				MediaIds:     gear.MediaIds,
				Availability: availability,
			},
		},
	}
}

// rsvpIntentionToAPI converts a stored RSVP intention to the feed's enum. An
// unrecognised value maps to unspecified rather than failing the feed.
func rsvpIntentionToAPI(intention models.RSVPIntention) api.FeedRSVPIntention {
	switch intention {
	case models.RSVPIntention_RSVP_INTENTION_YES:
		return api.FeedRSVPIntention_FEED_RSVP_INTENTION_YES
	case models.RSVPIntention_RSVP_INTENTION_MAYBE:
		return api.FeedRSVPIntention_FEED_RSVP_INTENTION_MAYBE
	case models.RSVPIntention_RSVP_INTENTION_NO:
		return api.FeedRSVPIntention_FEED_RSVP_INTENTION_NO
	default:
		return api.FeedRSVPIntention_FEED_RSVP_INTENTION_UNSPECIFIED
	}
}

// createRequestCreatedItem creates a feed item for request creation events.
func (s *Service) createRequestCreatedItem(ctx context.Context, event *models.CommunityEvent, viewerUserID string, fc *feedLookups) *api.FeedItem {
	requestID := event.GetRequestId()
	if requestID == "" {
		return nil
	}

	// Request may be absent if soft-deleted (fetched without IncludeDeleted).
	request, ok := fc.requestMap[requestID]
	if !ok {
		logging.LoggerWithContext(ctx).Warn("request not found in batch for feed", "target_request_id", requestID)
		return nil
	}

	// Look up CommunityRequest from pre-fetched map.
	communityRequest, ok := fc.communityRequestMap[requestID]
	if !ok {
		return nil // Request has been unshared from this community.
	}
	if communityRequest.Archived {
		return nil
	}

	requesterUser := fc.userMap[request.RequesterId]
	if requesterUser == nil {
		logging.LoggerWithContext(ctx).Error("requester not found in batch", "requester_id", request.RequesterId)
		return nil
	}

	// Look up location from pre-fetched map.
	locationName := ""
	if request.LocationId != "" {
		if location, ok := fc.locationMap[request.LocationId]; ok {
			if location.Address != nil {
				locationName = location.Address.Locality
			}
		}
	}

	offerCount := fc.offerCountMap[requestID]

	// Nil when the offer batch fetch failed — leave the field absent so the
	// client shows a neutral CTA rather than asserting the viewer has not
	// offered.
	var viewerHasOffered *bool
	if fc.viewerOfferedSet != nil {
		offered := fc.viewerOfferedSet[requestID]
		viewerHasOffered = &offered
	}

	return &api.FeedItem{
		Id:                event.Id,
		ItemType:          api.FeedItemType_FEED_ITEM_TYPE_REQUEST_CREATED,
		OccurredAtUnixSec: event.OccurredAtUnixSec,
		Payload: &api.FeedItem_RequestCreated{
			RequestCreated: &api.RequestCreatedPayload{
				RequestId:        request.Id,
				Title:            request.Title,
				Description:      request.Description,
				MediaIds:         request.MediaIds,
				LocationId:       request.LocationId,
				LocationName:     locationName,
				OfferCount:       offerCount,
				ViewerHasOffered: viewerHasOffered,
				Requester:        requesterUser,
				CanEdit:          request.RequesterId == viewerUserID,
			},
		},
	}
}

// createCommunityCreatedItem creates a feed item for community creation events.
func (s *Service) createCommunityCreatedItem(ctx context.Context, event *models.CommunityEvent, viewerUserID string, fc *feedLookups) *api.FeedItem {
	// Get community details (not batch-fetched since there's typically only one per feed)
	community := &models.Community{}
	if err := s.sqlStorage.GetByID(ctx, event.CommunityId, community); err != nil {
		logging.LoggerWithContext(ctx).Error("failed to get community", "community_id", event.CommunityId, "error", err)
		return nil
	}

	// Look up actor from pre-fetched user map
	actorUser := fc.userMap[event.ActorId]
	if actorUser == nil {
		logging.LoggerWithContext(ctx).Error("actor not found in batch", "actor_id", event.ActorId)
		return nil
	}

	// Determine if viewer can edit (only creator can edit)
	canEdit := community.CreatorId == viewerUserID

	return &api.FeedItem{
		Id:                event.Id,
		ItemType:          api.FeedItemType_FEED_ITEM_TYPE_COMMUNITY_CREATED,
		OccurredAtUnixSec: event.OccurredAtUnixSec,
		Payload: &api.FeedItem_CommunityCreated{
			CommunityCreated: &api.CommunityCreatedPayload{
				CommunityId:          community.Id,
				CommunityName:        community.Name,
				CommunityDescription: community.Description,
				Actor:                actorUser,
				MediaIds:             community.MediaIds,
				CanEdit:              canEdit,
			},
		},
	}
}

// createExperienceCreatedItem creates a feed item for experience creation events.
func (s *Service) createExperienceCreatedItem(ctx context.Context, event *models.CommunityEvent, viewerUserID string, fc *feedLookups) *api.FeedItem {
	experienceID := event.GetExperienceId()
	if experienceID == "" {
		return nil
	}

	// Experience may be absent if soft-deleted (fetched without IncludeDeleted).
	experience, ok := fc.experienceMap[experienceID]
	if !ok {
		logging.LoggerWithContext(ctx).Warn("experience not found in batch for feed", "experience_id", experienceID)
		return nil
	}

	// Look up CommunityExperience from pre-fetched map.
	communityExperience, ok := fc.communityExperienceMap[experienceID]
	if !ok {
		return nil // Experience unshared from this community.
	}
	if communityExperience.Archived {
		return nil
	}

	creatorUser := fc.userMap[experience.OwnerId]
	if creatorUser == nil {
		logging.LoggerWithContext(ctx).Error("creator not found in batch", "owner_id", experience.OwnerId)
		return nil
	}

	// Look up location from pre-fetched map.
	locationName := ""
	if experience.LocationId != "" {
		if location, ok := fc.locationMap[experience.LocationId]; ok {
			if location.Address != nil {
				locationName = location.Address.Locality
			}
		}
	}

	yesCount := fc.rsvpYesCountMap[experienceID]
	maybeCount := fc.rsvpMaybeCountMap[experienceID]
	canEdit := experience.OwnerId == viewerUserID
	apiState := api.ExperienceState(experience.State)

	// Absent both when the viewer has not responded and when the RSVP batch
	// fetch failed — the client must not read either as a standing invitation
	// to someone already going.
	var viewerRSVP *api.FeedRSVPIntention
	if fc.viewerRSVPMap != nil {
		if intention, ok := fc.viewerRSVPMap[experienceID]; ok {
			viewerRSVP = &intention
		}
	}

	return &api.FeedItem{
		Id:                event.Id,
		ItemType:          api.FeedItemType_FEED_ITEM_TYPE_EXPERIENCE_CREATED,
		OccurredAtUnixSec: event.OccurredAtUnixSec,
		Payload: &api.FeedItem_ExperienceCreated{
			ExperienceCreated: &api.ExperienceCreatedPayload{
				ExperienceId:    experience.Id,
				Name:            experience.Name,
				Description:     experience.Description,
				MediaIds:        experience.MediaIds,
				LocationId:      experience.LocationId,
				LocationName:    locationName,
				Time:            services.ConvertTimeModelsToAPI(experience.Time),
				YesCount:        yesCount,
				MaybeCount:      maybeCount,
				ViewerRsvp:      viewerRSVP,
				MaxParticipants: experience.MaxParticipants,
				Creator:         creatorUser,
				CanEdit:         canEdit,
				State:           apiState,
			},
		},
	}
}

// createStoryItem creates a feed item for a story.
func (s *Service) createStoryItem(ctx context.Context, story *models.Story, fc *feedLookups) *api.FeedItem {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "createStoryItem",
		"story_id", story.Id,
	)

	// Look up participant users from pre-fetched map
	participants := make([]*api.User, 0, len(story.ParticipantIds))
	for _, participantID := range story.ParticipantIds {
		user := fc.userMap[participantID]
		if user == nil {
			logger.Warn("participant not found in batch", "participant_id", participantID)
			continue
		}
		participants = append(participants, user)
	}

	// Convert story type string to enum
	storyType := api.StoryType_STORY_TYPE_UNSPECIFIED
	switch story.StoryType {
	case ai.StoryTypeLoanCompleted:
		storyType = api.StoryType_STORY_TYPE_LOAN_COMPLETED
	case ai.StoryTypeGiveawayCompleted:
		storyType = api.StoryType_STORY_TYPE_GIVEAWAY_COMPLETED
	case ai.StoryTypeExperienceConcluded:
		storyType = api.StoryType_STORY_TYPE_EXPERIENCE_CONCLUDED
	case ai.StoryTypeRequestFulfilled:
		storyType = api.StoryType_STORY_TYPE_REQUEST_FULFILLED
	case ai.StoryTypeNewMemberWelcome:
		storyType = api.StoryType_STORY_TYPE_NEW_MEMBER_WELCOME
	}

	logger.DebugContext(ctx, "story included in feed",
		"story_id", story.Id,
		"story_type", story.StoryType,
	)

	// Convert impact metrics if present.
	var impact *api.ImpactEstimate
	if story.CostSavedUsd > 0 || story.TimeSavedMinutes > 0 || story.Co2SavedGrams > 0 {
		impact = &api.ImpactEstimate{
			MoneySaved: &api.MoneySavings{ValueUsd: &api.Estimate{Mean: story.CostSavedUsd}},
			EmissionsPrevented: &api.PreventedEmissions{
				ManufactureAvoidedCarbon: &api.CarbonEstimate{Co2EGrams: &api.Estimate{Mean: story.Co2SavedGrams}},
			},
			TimeSaved: &api.TimeSavings{Minutes: &api.Estimate{Mean: story.TimeSavedMinutes}},
		}
	}

	return &api.FeedItem{
		Id:                story.Id,
		ItemType:          api.FeedItemType_FEED_ITEM_TYPE_STORY,
		OccurredAtUnixSec: story.CreatedAtUnixSec,
		Payload: &api.FeedItem_Story{
			Story: &api.StoryPayload{
				StoryType:    storyType,
				Title:        story.Title,
				Description:  story.Description,
				MediaIds:     story.MediaIds,
				Participants: participants,
				GearId:       story.GearId,
				LoanId:       story.LoanId,
				ExperienceId: story.ExperienceId,
				RequestId:    story.RequestId,
				Impact:       impact,
				Kicker:       story.Kicker,
			},
		},
	}
}
