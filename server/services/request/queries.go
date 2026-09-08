package request

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// GetRequest retrieves details of a specific request.
func (s *Service) GetRequest(
	ctx context.Context,
	req *connect.Request[api.GetRequestRequest],
) (*connect.Response[api.GetRequestResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"target_request_id", req.Msg.RequestId,
	)

	logger.Debug("getting request")

	// Fetch the request
	requestStored := &models.Request{}
	err = s.storage.GetByID(ctx, req.Msg.RequestId, requestStored)
	if err != nil {
		logger.Error("failed to get request", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
	}

	// Caller must be an active member of at least one community the request is
	// shared with, or be the requester.
	sharedCommunityIDs, callerCommunityIDs, err := auth.RequireAccessToCommunityScopedEntity(
		ctx, s.storage, authInfo.UserID,
		auth.EntityRequest, requestStored.Id, requestStored.RequesterId,
	)
	if err != nil {
		return nil, err
	}

	// Build request item with the requested community context (empty = first community).
	requestItem, err := s.buildRequest(ctx, requestStored, req.Msg.CommunityId)
	if err != nil {
		return nil, err
	}

	// Populate shared communities using the full set for counts and the
	// caller-scoped subset for the SharedCommunities list.
	if err := s.populateRequestSharedCommunities(ctx, requestStored.Id, sharedCommunityIDs, callerCommunityIDs, requestItem); err != nil {
		logger.WarnContext(ctx, "failed to populate request shared communities", "error", err)
	}

	return connect.NewResponse(&api.GetRequestResponse{
		Request: requestItem,
	}), nil
}

// populateRequestSharedCommunities attaches the communities the request is
// shared with to the api.Request message.
//
// sharedCommunityIDs is the full set of communities the request is in.
// callerCommunityIDs is the caller-scoped subset (communities the caller is
// an active member of). TotalSharedCommunityCount and TotalDistinctMemberCount
// are derived from the full set; SharedCommunities is built from
// callerCommunityIDs to avoid leaking community details to non-members.
func (s *Service) populateRequestSharedCommunities(
	ctx context.Context,
	requestID string,
	sharedCommunityIDs []string,
	callerCommunityIDs []string,
	item *api.Request,
) error {
	if len(sharedCommunityIDs) == 0 {
		return nil
	}

	crRaw, err := s.storage.QueryByField(ctx, "request_id", requestID, &models.CommunityRequest{})
	if err != nil {
		return fmt.Errorf("failed to query community requests: %w", err)
	}

	// Build a set of the full shared community IDs for fast lookup.
	sharedSet := make(map[string]struct{}, len(sharedCommunityIDs))
	for _, cid := range sharedCommunityIDs {
		sharedSet[cid] = struct{}{}
	}

	// Filter CommunityRequest rows to the full shared set.
	allFilteredIDs := make([]string, 0, len(sharedCommunityIDs))
	crByCommunityID := make(map[string]*models.CommunityRequest, len(sharedCommunityIDs))
	for _, m := range crRaw {
		cr := m.(*models.CommunityRequest)
		if _, ok := sharedSet[cr.CommunityId]; !ok {
			continue
		}
		allFilteredIDs = append(allFilteredIDs, cr.CommunityId)
		crByCommunityID[cr.CommunityId] = cr
	}
	if len(allFilteredIDs) == 0 {
		return nil
	}

	// Report the full count of communities the request is shared with.
	item.TotalSharedCommunityCount = int32(len(allFilteredIDs))

	// Fetch memberships for ALL communities to compute the true distinct user count.
	membershipsRaw, err := s.storage.QueryByFieldIn(ctx, "community_id", allFilteredIDs, &models.CommunityUser{})
	if err != nil {
		return fmt.Errorf("failed to batch-fetch memberships: %w", err)
	}
	memberCounts := make(map[string]int32, len(allFilteredIDs))
	distinctUsers := make(map[string]struct{}, len(membershipsRaw))
	// Member ids per community, used to surface the request's own ad-hoc origin
	// community's directly-invited individuals (#2492).
	membersByCommunity := make(map[string][]string, len(allFilteredIDs))
	for _, m := range membershipsRaw {
		cu := m.(*models.CommunityUser)
		memberCounts[cu.CommunityId]++
		membersByCommunity[cu.CommunityId] = append(
			membersByCommunity[cu.CommunityId], cu.UserId,
		)
		distinctUsers[cu.UserId] = struct{}{}
	}
	item.TotalDistinctMemberCount = int32(len(distinctUsers))

	// Build SharedCommunities from the caller-scoped subset only.
	callerSet := make(map[string]struct{}, len(callerCommunityIDs))
	for _, cid := range callerCommunityIDs {
		callerSet[cid] = struct{}{}
	}

	viewerIDs := make([]string, 0, len(callerCommunityIDs))
	for _, cid := range allFilteredIDs {
		if _, ok := callerSet[cid]; ok {
			viewerIDs = append(viewerIDs, cid)
		}
	}
	if len(viewerIDs) == 0 {
		return nil
	}

	communityMap, err := s.storage.GetByIDs(ctx, viewerIDs, &models.Community{})
	if err != nil {
		return fmt.Errorf("failed to batch-fetch communities: %w", err)
	}

	var originCommunityID string
	for _, cid := range viewerIDs {
		cm, ok := communityMap[cid]
		if !ok {
			continue
		}
		community := cm.(*models.Community)
		mediaID := ""
		if len(community.MediaIds) > 0 {
			mediaID = community.MediaIds[0]
		}
		if community.GetOriginRequestId() == requestID {
			originCommunityID = cid
		}
		cr := crByCommunityID[cid]
		item.SharedCommunities = append(item.SharedCommunities, &api.SharedCommunity{
			CommunityId:     cid,
			CommunityName:   community.Name,
			MediaId:         mediaID,
			MemberCount:     memberCounts[cid],
			SharedAtUnixSec: cr.SharedAtUnixSec,
		})
	}

	// Surface the request's own ad-hoc origin community's directly-invited
	// individuals (excl. the requester) so the "Shared with" surface can list
	// them by name; named communities stay collapsed to a member count. A
	// request has no RSVP, so there is nothing to exclude beyond the requester
	// (#2492).
	if originCommunityID != "" {
		ownerID := item.GetRequester().GetId()
		invited, err := services.FetchInvitedIndividuals(
			ctx, s.storage, membersByCommunity[originCommunityID], ownerID, nil,
		)
		if err != nil {
			return fmt.Errorf("failed to fetch invited individuals: %w", err)
		}
		item.InvitedIndividuals = invited
	}
	return nil
}

// ListRequests lists requests in a community with optional filtering.
func (s *Service) ListRequests(
	ctx context.Context,
	req *connect.Request[api.ListRequestsRequest],
) (*connect.Response[api.ListRequestsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_id", req.Msg.CommunityId,
	)

	logger.Debug("listing requests in community")

	// Verify community is active and caller is a member (single round-trip).
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Query CommunityRequest junction table to get request IDs for this community
	// Note: We query all CommunityRequest records (including archived ones) because
	// archived requests should still appear in ListRequests when filtering by state=FULFILLED/CANCELLED.
	// The archived flag only affects the feed, not request listings.
	communityRequests, err := storage.QueryByField[*models.CommunityRequest](s.storage, ctx, "community_id", req.Msg.CommunityId)
	if err != nil {
		logger.Error("failed to query community requests", "error", err)
		return nil, connecterr.Internal(ctx, "ListRequests", err)
	}

	// Build a map of requestID -> CommunityRequest for later filtering
	communityRequestMap := storage.ToMap(communityRequests, func(cr *models.CommunityRequest) string { return cr.RequestId })
	requestIDs := storage.CollectField(communityRequests, func(cr *models.CommunityRequest) string { return cr.RequestId })

	if len(requestIDs) == 0 {
		// No requests in this community
		return connect.NewResponse(&api.ListRequestsResponse{
			Requests: []*api.Request{},
		}), nil
	}

	// Batch fetch all requests by IDs
	requestMap, err := storage.GetByIDs[*models.Request](s.storage, ctx, requestIDs)
	if err != nil {
		logger.Error("failed to batch fetch requests", "error", err)
		return nil, connecterr.Internal(ctx, "ListRequests", err)
	}

	filteredRequests := make([]*models.Request, 0, len(requestIDs))
	for _, requestID := range requestIDs {
		request, ok := requestMap[requestID]
		if !ok {
			continue
		}

		// Check if this request is archived in this community
		communityRequest := communityRequestMap[request.Id]
		if communityRequest.Archived {
			// Only include archived requests if they're in FULFILLED or CANCELLED state
			// (archived because completed, not because unshared)
			if request.State != models.RequestState_REQUEST_STATE_FULFILLED &&
				request.State != models.RequestState_REQUEST_STATE_CANCELLED {
				continue
			}
		}

		filteredRequests = append(filteredRequests, request)
	}

	// Apply state filter before the batched build to keep the batch input trimmed.
	stateFiltered := make([]*models.Request, 0, len(filteredRequests))
	for _, request := range filteredRequests {
		if req.Msg.State != api.RequestState_REQUEST_STATE_UNSPECIFIED {
			if request.State != apiRequestStateToModel(req.Msg.State) {
				continue
			}
		} else {
			// Filter out FULFILLED and CANCELLED if no specific state requested.
			if request.State == models.RequestState_REQUEST_STATE_FULFILLED || request.State == models.RequestState_REQUEST_STATE_CANCELLED {
				continue
			}
		}
		stateFiltered = append(stateFiltered, request)
	}

	// Build response with enriched request data using a single batched round-trip.
	requests, err := s.buildAPIRequests(ctx, stateFiltered, req.Msg.CommunityId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "ListRequests", err)
	}

	return connect.NewResponse(&api.ListRequestsResponse{
		Requests: requests,
	}), nil
}

// ListMyRequests lists all requests created by the calling user.
func (s *Service) ListMyRequests(
	ctx context.Context,
	req *connect.Request[api.ListMyRequestsRequest],
) (*connect.Response[api.ListMyRequestsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.Debug("listing user's requests")

	var matchedRequests []*models.Request

	// If filtering by community, query via CommunityRequest join table
	if req.Msg.CommunityId != "" {
		// First get request IDs from CommunityRequest join table
		communityRequests, err := storage.QueryByFields[*models.CommunityRequest](s.storage, ctx, map[string]any{
			"community_id": req.Msg.CommunityId,
			"archived":     false,
		})
		if err != nil {
			logger.Error("failed to query community requests", "error", err)
			return nil, connecterr.Internal(ctx, "ListMyRequests", err)
		}

		// Collect request IDs and batch fetch
		crRequestIDs := storage.CollectField(communityRequests, func(cr *models.CommunityRequest) string { return cr.RequestId })

		requestMap, err := storage.GetByIDs[*models.Request](s.storage, ctx, crRequestIDs)
		if err != nil {
			logger.Error("failed to batch fetch requests", "error", err)
			return nil, connecterr.Internal(ctx, "ListMyRequests", err)
		}

		// Filter by requester
		matchedRequests = make([]*models.Request, 0)
		for _, requestID := range crRequestIDs {
			request, ok := requestMap[requestID]
			if !ok {
				continue
			}
			// Only include requests created by this user
			if request.RequesterId == authInfo.UserID {
				matchedRequests = append(matchedRequests, request)
			}
		}
	} else {
		// Query all requests by this user
		queryResult, err := storage.QueryByFields[*models.Request](s.storage, ctx, map[string]any{
			"requester_id": authInfo.UserID,
		})
		if err != nil {
			logger.Error("failed to query requests", "error", err)
			return nil, connecterr.Internal(ctx, "ListMyRequests", err)
		}
		matchedRequests = queryResult
	}

	// Build response with enriched request data using a single batched round-trip.
	requests, err := s.buildAPIRequests(ctx, matchedRequests, req.Msg.CommunityId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "ListMyRequests", err)
	}

	return connect.NewResponse(&api.ListMyRequestsResponse{
		Requests: requests,
	}), nil
}
