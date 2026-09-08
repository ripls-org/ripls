package request

import (
	"context"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/conversation"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// buildAPIRequests builds API Requests from a slice of stored Request models.
// It performs all dependency fetches as batches before assembling responses,
// eliminating N+1 queries from list endpoints. Output order matches input order.
// The communityID parameter selects which community's conversation to surface
// for multi-community requests; if empty, the first community is used.
func (s *Service) buildAPIRequests(ctx context.Context, requests []*models.Request, communityID string) ([]*api.Request, error) {
	if len(requests) == 0 {
		return nil, nil
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "buildAPIRequests",
		"count", len(requests),
	)

	// Step 1: collect IDs needed for batch queries.
	requestIDs := make([]string, 0, len(requests))
	requesterIDs := make([]string, 0, len(requests))
	locationIDs := make([]string, 0, len(requests))
	convIDs := make([]string, 0, len(requests))
	for _, r := range requests {
		requestIDs = append(requestIDs, r.Id)
		requesterIDs = append(requesterIDs, r.RequesterId)
		if r.LocationId != "" {
			locationIDs = append(locationIDs, r.LocationId)
		}
		if r.ConversationId != "" {
			convIDs = append(convIDs, r.ConversationId)
		}
	}

	// Step 2: batch-fetch RequestOffers; group by request_id, keep non-withdrawn only.
	allOffers, err := storage.QueryByFieldIn[*models.RequestOffer](s.storage, ctx, "request_id", requestIDs)
	if err != nil {
		logger.Warn("failed to batch fetch request offers", "error", err)
		allOffers = nil
	}
	offersByRequest := make(map[string][]*models.RequestOffer, len(requests))
	for _, o := range allOffers {
		if !o.Withdrawn {
			offersByRequest[o.RequestId] = append(offersByRequest[o.RequestId], o)
		}
	}

	// Step 3: batch-fetch CommunityRequest rows; group by request_id.
	allCRs, err := storage.QueryByFieldIn[*models.CommunityRequest](s.storage, ctx, "request_id", requestIDs)
	if err != nil {
		logger.Warn("failed to batch fetch community requests", "error", err)
		allCRs = nil
	}
	crsByRequest := make(map[string][]*models.CommunityRequest, len(requests))
	for _, cr := range allCRs {
		crsByRequest[cr.RequestId] = append(crsByRequest[cr.RequestId], cr)
	}

	// Step 4: batch-fetch CommunityEvents; filter to REQUEST_CREATED in-process.
	// Build map[request_id]OccurredAtUnixSec.
	allEvents, err := storage.QueryByFieldIn[*models.CommunityEvent](s.storage, ctx, "request_id", requestIDs)
	if err != nil {
		logger.Warn("failed to batch fetch community events for created timestamps", "error", err)
		allEvents = nil
	}
	createdAtByRequest := make(map[string]int64, len(requests))
	for _, ev := range allEvents {
		if ev.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED {
			if _, seen := createdAtByRequest[ev.GetRequestId()]; !seen {
				createdAtByRequest[ev.GetRequestId()] = ev.OccurredAtUnixSec
			}
		}
	}

	// Step 5: batch-fetch locations.
	locMap, err := storage.GetByIDs[*models.Location](s.storage, ctx, locationIDs)
	if err != nil {
		logger.Warn("failed to batch fetch locations", "error", err)
		locMap = make(map[string]*models.Location)
	}

	// Step 6: batch-fetch comment previews, message counts, and poster IDs.
	authUserID := ""
	if authInfo, ok := auth.GetAuthInfo(ctx); ok {
		authUserID = authInfo.UserID
	}
	previewMap, countMap, postersByConv, err := conversation.BatchEnrichWithCommentPreviewAndCounts(ctx, s.storage, convIDs, authUserID)
	if err != nil {
		logger.Warn("failed to batch enrich comment previews", "error", err)
		previewMap = make(map[string]*conversation.CommentPreview)
		countMap = make(map[string]int32)
		postersByConv = make(map[string][]string)
	}

	// Step 7: build the union user-ID set: requesters ∪ offerers ∪ conversation posters.
	// User fetches for comment-preview senders are handled inside
	// BatchEnrichWithCommentPreviewAndCounts already; we only need offerers and
	// posters here so they can be resolved to *api.User for the response fields.
	allUserIDs := make([]string, 0, len(requesterIDs)+len(allOffers))
	allUserIDs = append(allUserIDs, requesterIDs...)
	for _, offers := range offersByRequest {
		for _, o := range offers {
			allUserIDs = append(allUserIDs, o.UserId)
		}
	}
	for _, posterIDs := range postersByConv {
		allUserIDs = append(allUserIDs, posterIDs...)
	}
	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, dedupStrings(allUserIDs))
	if err != nil {
		return nil, connecterr.Internal(ctx, "buildAPIRequests", err, "detail", "failed to batch fetch users")
	}

	// Step 8: assemble one *api.Request per input request.
	results := make([]*api.Request, 0, len(requests))
	for _, r := range requests {
		reqLogger := logger.With("target_request_id", r.Id)

		requester := userMap[r.RequesterId]
		if requester == nil {
			reqLogger.Warn("requester not found, skipping request", "requester_id", r.RequesterId)
			continue
		}

		// Apply state-aware CommunityRequest filtering (mirrors buildRequest).
		var activeCRs []*models.CommunityRequest
		allReqCRs := crsByRequest[r.Id]
		if r.State == models.RequestState_REQUEST_STATE_FULFILLED || r.State == models.RequestState_REQUEST_STATE_CANCELLED {
			activeCRs = allReqCRs
		} else {
			for _, cr := range allReqCRs {
				if !cr.Archived {
					activeCRs = append(activeCRs, cr)
				}
			}
		}
		if len(activeCRs) == 0 {
			reqLogger.Warn("request not linked to any active community, skipping")
			continue
		}

		// Determine target community and collect shared community IDs.
		sharedCommunityIDs := make([]string, 0, len(activeCRs))
		var targetCommunityID string
		for _, cr := range activeCRs {
			sharedCommunityIDs = append(sharedCommunityIDs, cr.CommunityId)
			if communityID != "" && cr.CommunityId == communityID {
				targetCommunityID = cr.CommunityId
			}
		}
		if targetCommunityID == "" {
			targetCommunityID = activeCRs[0].CommunityId
		}

		conversationID := r.ConversationId

		// Resolve location fields.
		locationName := ""
		var latitudeDeg, longitudeDeg float64
		if r.LocationId != "" {
			if loc, ok := locMap[r.LocationId]; ok {
				if loc.Address != nil {
					locationName = loc.Address.Locality
				}
				if loc.Geolocation != nil {
					latitudeDeg = loc.Geolocation.LatitudeDeg
					longitudeDeg = loc.Geolocation.LongitudeDeg
				}
			}
		}

		// Build offerer list.
		offers := offersByRequest[r.Id]
		offerers := make([]*api.User, 0, len(offers))
		for _, o := range offers {
			if u := userMap[o.UserId]; u != nil {
				offerers = append(offerers, u)
			}
		}

		// Build conversation participants (posters excluding the requester).
		var conversationParticipants []*api.User
		if conversationID != "" {
			for _, posterID := range postersByConv[conversationID] {
				if posterID == r.RequesterId {
					continue
				}
				if u := userMap[posterID]; u != nil {
					conversationParticipants = append(conversationParticipants, u)
				}
			}
		}

		// Message count and comment preview from the batched maps.
		messageCount := countMap[conversationID]

		result := &api.Request{
			Id:                       r.Id,
			Requester:                requester,
			Title:                    r.Title,
			Description:              r.Description,
			State:                    modelRequestStateToAPI(r.State),
			Offerers:                 offerers,
			ConversationId:           &conversationID,
			MediaIds:                 r.MediaIds,
			CreatedAtUnixSec:         createdAtByRequest[r.Id],
			LocationId:               r.LocationId,
			LocationName:             locationName,
			LatitudeDeg:              latitudeDeg,
			LongitudeDeg:             longitudeDeg,
			ConversationParticipants: conversationParticipants,
			MessageCount:             messageCount,
			ResolutionSummary:        r.ResolutionSummary,
			ConfirmedHelperIds:       r.ConfirmedHelperIds,
			CommunityId:              targetCommunityID,
			SharedCommunityIds:       sharedCommunityIDs,
			FulfilledAtUnixSec:       r.FulfilledAtUnixSec,
			NeededByUnixSec:          r.NeededByUnixSec,
		}

		// Attach comment preview fields.
		if conversationID != "" {
			if preview := previewMap[conversationID]; preview != nil {
				result.UnreadCount = preview.UnreadCount
				result.LastMessageText = preview.LastMessageText
				result.LastMessageSender = preview.LastMessageSender
				result.LastMessageTimeAgo = preview.LastMessageTimeAgo
				result.RecentCommenters = preview.RecentCommenters
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// dedupStrings returns a deduplicated slice preserving order, dropping empty strings.
func dedupStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}
