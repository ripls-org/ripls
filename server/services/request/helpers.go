package request

import (
	"context"
	"fmt"
	"sort"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/conversation"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// buildRequest builds an API Request from a stored Request model.
// For multi-community requests, communityID can be provided to return the community-specific conversation ID.
// If communityID is empty, uses the first community's data.
func (s *Service) buildRequest(ctx context.Context, request *models.Request, communityID string) (*api.Request, error) {
	logger := logging.LoggerWithContext(ctx).With("target_request_id", request.Id)

	// Get requester details
	requester, err := services.FetchAPIUser(ctx, s.storage, request.RequesterId)
	if err != nil {
		logger.Error("failed to get requester", "requester_id", request.RequesterId, "error", err)
		return nil, connecterr.Internal(ctx, "buildRequest", err)
	}

	// Build the offerers list from all non-withdrawn offers.
	offerResults, queryErr := s.storage.QueryByFields(ctx, map[string]any{
		"request_id": request.Id,
		"withdrawn":  false,
	}, &models.RequestOffer{})
	if queryErr != nil {
		logger.Error("failed to query offers", "error", queryErr)
		return nil, connecterr.Internal(ctx, "buildRequest", queryErr)
	}
	offererIDsMap := make(map[string]bool)
	for _, result := range offerResults {
		offer := result.(*models.RequestOffer)
		offererIDsMap[offer.UserId] = true
	}
	offererIDs := make([]string, 0, len(offererIDsMap))
	for userID := range offererIDsMap {
		offererIDs = append(offererIDs, userID)
	}
	offerers, fetchErr := services.FetchAPIUsers(ctx, s.storage, offererIDs)
	if fetchErr != nil {
		logger.Error("failed to get offerers", "error", fetchErr)
		return nil, connecterr.Internal(ctx, "buildRequest", fetchErr)
	}

	// Get all CommunityRequest records to find communities and conversation
	// For fulfilled/cancelled requests, include archived records since they've been archived in all communities
	var communityRequests []any
	if request.State == models.RequestState_REQUEST_STATE_FULFILLED || request.State == models.RequestState_REQUEST_STATE_CANCELLED {
		results, queryErr := s.storage.QueryByField(ctx, "request_id", request.Id, &models.CommunityRequest{})
		if queryErr != nil || len(results) == 0 {
			logger.Warn("request not linked to any community")
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found in any community"))
		}
		communityRequests = make([]any, len(results))
		for i, r := range results {
			communityRequests[i] = r
		}
	} else {
		results, queryErr := s.storage.QueryByFields(ctx, map[string]any{
			"request_id": request.Id,
			"archived":   false,
		}, &models.CommunityRequest{})
		if queryErr != nil || len(results) == 0 {
			logger.Warn("request not linked to any community")
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found in any community"))
		}
		communityRequests = make([]any, len(results))
		for i, r := range results {
			communityRequests[i] = r
		}
	}

	if len(communityRequests) == 0 {
		logger.Warn("request not linked to any community")
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found in any community"))
	}

	// Extract shared community IDs and determine the target community.
	sharedCommunityIDs := make([]string, 0, len(communityRequests))
	var targetCommunityID string

	for _, result := range communityRequests {
		cr := result.(*models.CommunityRequest)
		sharedCommunityIDs = append(sharedCommunityIDs, cr.CommunityId)
		if communityID != "" && cr.CommunityId == communityID {
			targetCommunityID = cr.CommunityId
		}
	}
	if targetCommunityID == "" {
		targetCommunityID = communityRequests[0].(*models.CommunityRequest).CommunityId
	}

	conversationID := request.ConversationId

	// Get creation timestamp from community event log
	createdAtUnixSec := int64(0)
	events, err := s.storage.QueryByFields(ctx, map[string]any{
		"community_id": targetCommunityID,
		"event_type":   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
	}, &models.CommunityEvent{})
	if err == nil {
		// Find the event for this specific request
		for _, msg := range events {
			event := msg.(*models.CommunityEvent)
			if event.GetRequestId() == request.Id {
				createdAtUnixSec = event.OccurredAtUnixSec
				break
			}
		}
	}

	// Get location details if location_id is set
	locationName := ""
	var latitudeDeg, longitudeDeg float64
	if request.LocationId != "" {
		location := &models.Location{}
		if err := s.storage.GetByID(ctx, request.LocationId, location); err == nil {
			if location.Address != nil {
				locationName = location.Address.Locality
			}
			if location.Geolocation != nil {
				latitudeDeg = location.Geolocation.LatitudeDeg
				longitudeDeg = location.Geolocation.LongitudeDeg
			}
		}
	}

	// Fetch conversation participants (excluding requester) and message count
	var conversationParticipants []*api.User
	var messageCount int32
	if conversationID != "" {
		logger.DebugContext(ctx, "fetching conversation participants and message count")

		// Get users who have posted messages (not just participants)
		participantIDs, err := chat.GetConversationParticipantsWhoPosted(ctx, s.storage, conversationID, true /* postedOnly */)
		if err != nil {
			logger.Warn("failed to fetch conversation participants", "error", err)
		} else {
			// Filter out the requester from participants
			var filteredParticipantIDs []string
			for _, participantID := range participantIDs {
				if participantID != request.RequesterId {
					filteredParticipantIDs = append(filteredParticipantIDs, participantID)
				}
			}

			// Fetch user details for each participant (excluding requester)
			if len(filteredParticipantIDs) > 0 {
				conversationParticipants, err = services.FetchAPIUsers(ctx, s.storage, filteredParticipantIDs)
				if err != nil {
					logger.Warn("failed to fetch participant user details", "error", err)
					conversationParticipants = nil // Reset to empty on error
				}
			}
		}

		// Get message count for the conversation
		messageCount, err = chat.GetConversationMessageCount(ctx, s.storage, conversationID)
		if err != nil {
			logger.Warn("failed to fetch conversation message count", "error", err)
			messageCount = 0 // Default to 0 on error
		}
	}

	result := &api.Request{
		Id:                       request.Id,
		Requester:                requester,
		Title:                    request.Title,
		Description:              request.Description,
		State:                    modelRequestStateToAPI(request.State),
		Offerers:                 offerers,
		ConversationId:           &conversationID,
		MediaIds:                 request.MediaIds,
		CreatedAtUnixSec:         createdAtUnixSec,
		LocationId:               request.LocationId,
		LocationName:             locationName,
		LatitudeDeg:              latitudeDeg,
		LongitudeDeg:             longitudeDeg,
		ConversationParticipants: conversationParticipants,
		MessageCount:             messageCount,
		ResolutionSummary:        request.ResolutionSummary,
		ConfirmedHelperIds:       request.ConfirmedHelperIds,
		CommunityId:              targetCommunityID,
		SharedCommunityIds:       sharedCommunityIDs,
		FulfilledAtUnixSec:       request.FulfilledAtUnixSec,
		NeededByUnixSec:          request.NeededByUnixSec,
	}

	// Gear-backed offers (#2702): the supply side of the request.
	gearOffers, err := s.buildRequestGearOffers(ctx, request.Id)
	if err != nil {
		logger.Error("failed to build gear offers", "error", err)
		return nil, connecterr.Internal(ctx, "buildRequest", err)
	}
	result.GearOffers = gearOffers

	// Enrich with comment preview if conversation exists and user is authenticated.
	if conversationID != "" {
		if authInfo, ok := auth.GetAuthInfo(ctx); ok {
			preview, err := conversation.EnrichWithCommentPreview(ctx, s.storage, conversationID, authInfo.UserID)
			if err != nil {
				logger.Warn("failed to enrich request comment preview", "conversation_id", conversationID, "error", err)
			} else {
				result.UnreadCount = preview.UnreadCount
				result.LastMessageText = preview.LastMessageText
				result.LastMessageSender = preview.LastMessageSender
				result.LastMessageTimeAgo = preview.LastMessageTimeAgo
				result.RecentCommenters = preview.RecentCommenters
			}
		}
	}

	return result, nil
}

// buildRequestGearOffers assembles the gear-backed offers (#2702) for a
// request read: the transfers carrying this request as their origin, enriched
// with gear, helper, and the escalated contribution. All lookups are batched —
// one query for transfers, one for gear, one for users, one for contributions.
func (s *Service) buildRequestGearOffers(ctx context.Context, requestID string) ([]*api.RequestGearOffer, error) {
	logger := logging.LoggerWithContext(ctx).With("target_request_id", requestID)

	transfers, err := storage.QueryByField[*models.Transfer](s.storage, ctx, "origin_request_id", requestID)
	if err != nil {
		return nil, fmt.Errorf("query origin transfers: %w", err)
	}
	if len(transfers) == 0 {
		return nil, nil
	}
	// Newest first.
	sort.Slice(transfers, func(i, j int) bool {
		return transfers[i].LatestRequestUnixSec > transfers[j].LatestRequestUnixSec
	})

	gearIDs := make([]string, 0, len(transfers))
	helperIDs := make([]string, 0, len(transfers))
	for _, t := range transfers {
		gearIDs = append(gearIDs, t.GearId)
		helperIDs = append(helperIDs, t.OwnerId)
	}
	// Include soft-deleted gear so offer history stays viewable after the
	// underlying gear is deleted (mirrors the transfer service's builder).
	gearMap, err := storage.GetByIDs[*models.Gear](s.storage, ctx, gearIDs, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		return nil, fmt.Errorf("batch fetch offered gear: %w", err)
	}
	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, helperIDs)
	if err != nil {
		return nil, fmt.Errorf("batch fetch helpers: %w", err)
	}
	contributions, err := storage.QueryByField[*models.PlanningContribution](s.storage, ctx, "request_id", requestID)
	if err != nil {
		return nil, fmt.Errorf("query contributions for gear offers: %w", err)
	}
	contributionByTransfer := make(map[string]*models.PlanningContribution, len(contributions))
	for _, c := range contributions {
		if c.GetTransferId() != "" {
			contributionByTransfer[c.GetTransferId()] = c
		}
	}

	offers := make([]*api.RequestGearOffer, 0, len(transfers))
	for _, t := range transfers {
		gear, ok := gearMap[t.GearId]
		if !ok {
			logger.Warn("gear missing for gear offer; skipping", "gear_id", t.GearId, "transfer_id", t.Id)
			continue
		}
		gearMediaID := ""
		if len(gear.MediaIds) > 0 {
			gearMediaID = gear.MediaIds[0]
		}
		offer := &api.RequestGearOffer{
			TransferId:   t.Id,
			TransferType: modelTransferTypeToAPI(t.TransferType),
			State:        modelTransferStateToAPI(t.State),
			GearId:       t.GearId,
			GearName:     gear.Name,
			GearMediaId:  gearMediaID,
			Helper:       services.ResolveUserOrFormer(userMap, t.OwnerId),
		}
		if c, ok := contributionByTransfer[t.Id]; ok {
			offer.ContributionId = &c.Id
			offer.NeedId = c.FromNeedId
			offer.AcceptedAtUnixSec = c.AcceptedAtUnixSec
		}
		offers = append(offers, offer)
	}
	return offers, nil
}

// modelTransferTypeToAPI converts a storage TransferType to its API form.
func modelTransferTypeToAPI(t models.TransferType) api.TransferType {
	switch t {
	case models.TransferType_TRANSFER_TYPE_LOAN:
		return api.TransferType_TRANSFER_TYPE_LOAN
	case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
		return api.TransferType_TRANSFER_TYPE_GIVEAWAY
	default:
		return api.TransferType_TRANSFER_TYPE_UNSPECIFIED
	}
}

// modelTransferStateToAPI converts a storage TransferState to its API form.
func modelTransferStateToAPI(s models.TransferState) api.TransferState {
	switch s {
	case models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED:
		return api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED
	case models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
		return api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED
	case models.TransferState_TRANSFER_STATE_ACTIVE:
		return api.TransferState_TRANSFER_STATE_ACTIVE
	case models.TransferState_TRANSFER_STATE_COMPLETED:
		return api.TransferState_TRANSFER_STATE_COMPLETED
	case models.TransferState_TRANSFER_STATE_CANCELLED:
		return api.TransferState_TRANSFER_STATE_CANCELLED
	default:
		return api.TransferState_TRANSFER_STATE_UNSPECIFIED
	}
}

// vulnerabilityLevelToString converts a SocialVulnerabilityLevel enum to the
// string form expected by QualityTimeHint ("high", "medium", or "low").
func vulnerabilityLevelToString(v api.SocialVulnerabilityLevel) string {
	switch v {
	case api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_HIGH:
		return "high"
	case api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_MEDIUM:
		return "medium"
	case api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_LOW:
		return "low"
	default:
		return ""
	}
}

// apiRequestStateToModel converts an API RequestState to a model RequestState.
func apiRequestStateToModel(apiState api.RequestState) models.RequestState {
	switch apiState {
	case api.RequestState_REQUEST_STATE_ACTIVE:
		return models.RequestState_REQUEST_STATE_ACTIVE
	case api.RequestState_REQUEST_STATE_OFFERS_RECEIVED:
		return models.RequestState_REQUEST_STATE_OFFERS_RECEIVED
	case api.RequestState_REQUEST_STATE_FULFILLED:
		return models.RequestState_REQUEST_STATE_FULFILLED
	case api.RequestState_REQUEST_STATE_CANCELLED:
		return models.RequestState_REQUEST_STATE_CANCELLED
	default:
		return models.RequestState_REQUEST_STATE_UNSPECIFIED
	}
}

// modelRequestStateToAPI converts a model RequestState to an API RequestState.
func modelRequestStateToAPI(modelState models.RequestState) api.RequestState {
	switch modelState {
	case models.RequestState_REQUEST_STATE_ACTIVE:
		return api.RequestState_REQUEST_STATE_ACTIVE
	case models.RequestState_REQUEST_STATE_OFFERS_RECEIVED:
		return api.RequestState_REQUEST_STATE_OFFERS_RECEIVED
	case models.RequestState_REQUEST_STATE_FULFILLED:
		return api.RequestState_REQUEST_STATE_FULFILLED
	case models.RequestState_REQUEST_STATE_CANCELLED:
		return api.RequestState_REQUEST_STATE_CANCELLED
	default:
		return api.RequestState_REQUEST_STATE_UNSPECIFIED
	}
}
