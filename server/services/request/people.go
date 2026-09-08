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
)

// GetRequestPeople retrieves people associated with a request (requester, helpers, offerers).
func (s *Service) GetRequestPeople(
	ctx context.Context,
	req *connect.Request[api.GetRequestPeopleRequest],
) (*connect.Response[api.GetRequestPeopleResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"target_request_id", req.Msg.RequestId,
	)

	logger.DebugContext(ctx, "fetching request people")

	// Fetch request to verify it exists and get requester
	request := &models.Request{}
	err = s.storage.GetByID(ctx, req.Msg.RequestId, request)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get request",
			"error", err,
		)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
	}

	// Fetch requester details
	requester, err := services.FetchAPIUser(ctx, s.storage, request.RequesterId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to fetch requester",
			"requester_id", request.RequesterId,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "GetRequestPeople", fmt.Errorf("failed to fetch requester"))
	}

	// For now, we don't track "current helper" or "past helpers" in the request model
	// These would require additional fields in the Request model to track fulfillments
	var currentHelper *api.User
	var pastHelpers []*api.User

	// Build query for offers related to this request
	queryFields := map[string]any{
		"request_id": req.Msg.RequestId,
		"withdrawn":  false, // Only include non-withdrawn offers
	}

	// If community context is provided, validate it's active and filter by it.
	if req.Msg.CommunityId != "" {
		if _, err := auth.RequireActiveCommunity(ctx, s.storage, req.Msg.CommunityId); err != nil {
			return nil, err
		}
		queryFields["community_id"] = req.Msg.CommunityId
	}

	// Fetch all offers for this request
	offersProto, err := s.storage.QueryByFields(ctx, queryFields, &models.RequestOffer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query offers",
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "GetRequestPeople", fmt.Errorf("failed to query offers"))
	}

	// Track unique offerers
	uniqueOfferers := make(map[string]*api.User)
	for _, msg := range offersProto {
		offer := msg.(*models.RequestOffer)

		// Skip if we've already fetched this user
		if _, exists := uniqueOfferers[offer.UserId]; exists {
			continue
		}

		// Fetch offerer user
		offererUser, err := services.FetchAPIUser(ctx, s.storage, offer.UserId)
		if err != nil {
			logger.ErrorContext(ctx, "failed to fetch offerer user",
				"user_id", offer.UserId,
				"error", err,
			)
			continue // Skip this offer if we can't fetch the user
		}
		uniqueOfferers[offer.UserId] = offererUser
	}

	// Convert map to slice
	var offerers []*api.User
	for _, user := range uniqueOfferers {
		offerers = append(offerers, user)
	}

	// Calculate total count (requester + current helper + past helpers + offerers)
	totalCount := int32(1) // Always count the requester
	totalCount += int32(len(offerers))

	logger.DebugContext(ctx, "fetched request people",
		"total_count", totalCount,
		"current_helper", currentHelper != nil,
		"past_helpers_count", len(pastHelpers),
		"offerers_count", len(offerers),
	)

	return connect.NewResponse(&api.GetRequestPeopleResponse{
		Requester:     requester,
		CurrentHelper: currentHelper,
		PastHelpers:   pastHelpers,
		Offerers:      offerers,
		TotalCount:    totalCount,
	}), nil
}
