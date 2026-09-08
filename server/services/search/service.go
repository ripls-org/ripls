package search

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// Service implements the SearchService RPC interface.
type Service struct {
	storage *storage.ProtoSQLStorage
}

// New creates a new search service.
func New(sqlStorage *storage.ProtoSQLStorage) *Service {
	return &Service{
		storage: sqlStorage,
	}
}

// resolveSearchCommunityIDs returns the list of community IDs to search.
func resolveSearchCommunityIDs(req *api.SearchRequest) []string {
	return req.CommunityIds
}

// Search performs a unified search across gear, requests, experiences, and users.
func (s *Service) Search(
	ctx context.Context,
	req *connect.Request[api.SearchRequest],
) (*connect.Response[api.SearchResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	communityIDs := resolveSearchCommunityIDs(req.Msg)
	if len(communityIDs) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("at least one community_id or community_ids entry is required"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_ids", strings.Join(communityIDs, ","),
	)

	logger.InfoContext(ctx, "search initiated",
		"search_query", req.Msg.Query,
		"strategy", req.Msg.Strategy.String(),
		"item_types", req.Msg.ItemTypes,
		"latitude_deg", req.Msg.LatitudeDeg,
		"longitude_deg", req.Msg.LongitudeDeg,
	)

	// Single batched JOIN: collapses the per-id round-trip loop down to one
	// query for all communities. Drops communities that are missing,
	// soft-deleted, or that the caller does not belong to — multi-community
	// surfaces degrade gracefully.
	activeCommunityIDs, _, err := auth.FilterActiveMemberCommunities(ctx, s.storage, communityIDs, authInfo.UserID)
	if err != nil {
		return nil, err
	}
	if len(activeCommunityIDs) == 0 {
		return connect.NewResponse(&api.SearchResponse{Results: nil}), nil
	}

	// Build set of requested item types.
	itemTypes := s.buildItemTypeSet(req.Msg.ItemTypes)

	// Search across all communities and deduplicate results.
	var apiResults []*api.SearchResultItem
	seenIDs := make(map[string]bool)
	for _, cid := range activeCommunityIDs {
		var results []*api.SearchResultItem
		if req.Msg.Strategy == api.SearchStrategy_SEARCH_STRATEGY_EXACT {
			results, err = s.exactSearch(ctx, cid, req.Msg, itemTypes)
		} else {
			results, err = s.semanticSearch(ctx, cid, req.Msg, itemTypes)
		}
		if err != nil {
			logger.ErrorContext(ctx, "search query failed", "error", err, "community_id", cid)
			return nil, connecterr.Internal(ctx, "Search", err)
		}
		for _, r := range results {
			itemID := searchResultEntityKey(r)
			if itemID != "" && seenIDs[itemID] {
				continue
			}
			if itemID != "" {
				seenIDs[itemID] = true
			}
			apiResults = append(apiResults, r)
		}
	}

	// Apply max_results limit (default 200).
	maxResults := req.Msg.MaxResults
	if maxResults <= 0 {
		maxResults = 200
	}
	if len(apiResults) > int(maxResults) {
		apiResults = apiResults[:maxResults]
	}

	logger.InfoContext(ctx, "search completed",
		"search_query", req.Msg.Query,
		"result_count", len(apiResults),
	)

	return connect.NewResponse(&api.SearchResponse{
		Results: apiResults,
	}), nil
}

// searchResultEntityKey returns a stable entity-scoped key for cross-community
// deduplication. The key includes a type prefix so different entity types with
// the same UUID cannot collide. This mirrors feedItemEntityKey in the feed
// package.
func searchResultEntityKey(r *api.SearchResultItem) string {
	switch {
	case r.GetGear() != nil:
		return "gear:" + r.GetGear().GetId()
	case r.GetRequest() != nil:
		return "request:" + r.GetRequest().GetId()
	case r.GetExperience() != nil:
		return "experience:" + r.GetExperience().GetId()
	case r.GetUser() != nil:
		return "user:" + r.GetUser().GetId()
	default:
		return ""
	}
}

// buildItemTypeSet creates a set of item types to search for.
// If no types are specified, returns all types.
func (s *Service) buildItemTypeSet(requestedTypes []api.SearchItemType) map[api.SearchItemType]bool {
	if len(requestedTypes) == 0 {
		return map[api.SearchItemType]bool{
			api.SearchItemType_SEARCH_ITEM_TYPE_GEAR:       true,
			api.SearchItemType_SEARCH_ITEM_TYPE_REQUEST:    true,
			api.SearchItemType_SEARCH_ITEM_TYPE_EXPERIENCE: true,
			api.SearchItemType_SEARCH_ITEM_TYPE_USER:       true,
		}
	}
	types := make(map[api.SearchItemType]bool)
	for _, t := range requestedTypes {
		if t != api.SearchItemType_SEARCH_ITEM_TYPE_UNSPECIFIED {
			types[t] = true
		}
	}
	return types
}

// semanticSearch performs the existing semantic/text search.
func (s *Service) semanticSearch(
	ctx context.Context,
	communityID string,
	req *api.SearchRequest,
	itemTypes map[api.SearchItemType]bool,
) ([]*api.SearchResultItem, error) {
	results, err := s.storage.QueryCommunitySearch(
		ctx,
		communityID,
		req.Query,
		req.LatitudeDeg,
		req.LongitudeDeg,
		req.IncludeCompleted,
	)
	if err != nil {
		return nil, err
	}

	// Collect all user IDs needed across all results
	var userIDs []string
	for _, result := range results {
		switch result.ItemType {
		case "gear":
			if result.Gear != nil {
				userIDs = append(userIDs, result.Gear.OwnerId)
			}
		case "request":
			if result.Request != nil {
				userIDs = append(userIDs, result.Request.RequesterId)
			}
		case "experience":
			if result.Experience != nil {
				userIDs = append(userIDs, result.Experience.OwnerId)
			}
		}
	}

	// Batch fetch all users
	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, userIDs)
	if err != nil {
		return nil, fmt.Errorf("batch fetch users: %w", err)
	}

	// Convert and filter by item types
	apiResults := make([]*api.SearchResultItem, 0, len(results))
	for _, result := range results {
		item, err := s.convertToSearchResultItemBatch(result, userMap)
		if err != nil {
			return nil, fmt.Errorf("failed to convert search result %s: %w", result.ID, err)
		}
		// Filter by requested item types
		if itemTypes[item.ItemType] {
			apiResults = append(apiResults, item)
		}
	}

	// If users are requested, add them (semantic search doesn't include users)
	if itemTypes[api.SearchItemType_SEARCH_ITEM_TYPE_USER] {
		userResults, err := s.searchUsers(ctx, communityID, req.Query)
		if err != nil {
			return nil, fmt.Errorf("failed to search users: %w", err)
		}
		apiResults = append(apiResults, userResults...)
	}

	return apiResults, nil
}

// exactSearch performs case-insensitive partial string matching for @-mentions.
func (s *Service) exactSearch(
	ctx context.Context,
	communityID string,
	req *api.SearchRequest,
	itemTypes map[api.SearchItemType]bool,
) ([]*api.SearchResultItem, error) {
	var apiResults []*api.SearchResultItem

	// Search users if requested
	if itemTypes[api.SearchItemType_SEARCH_ITEM_TYPE_USER] {
		userResults, err := s.searchUsers(ctx, communityID, req.Query)
		if err != nil {
			return nil, fmt.Errorf("failed to search users: %w", err)
		}
		apiResults = append(apiResults, userResults...)
	}

	// Search gear if requested
	if itemTypes[api.SearchItemType_SEARCH_ITEM_TYPE_GEAR] {
		gearResults, err := s.searchGearExact(ctx, communityID, req.Query)
		if err != nil {
			return nil, fmt.Errorf("failed to search gear: %w", err)
		}
		apiResults = append(apiResults, gearResults...)
	}

	// Search requests if requested
	if itemTypes[api.SearchItemType_SEARCH_ITEM_TYPE_REQUEST] {
		requestResults, err := s.searchRequestsExact(ctx, communityID, req.Query)
		if err != nil {
			return nil, fmt.Errorf("failed to search requests: %w", err)
		}
		apiResults = append(apiResults, requestResults...)
	}

	// Search experiences if requested
	if itemTypes[api.SearchItemType_SEARCH_ITEM_TYPE_EXPERIENCE] {
		experienceResults, err := s.searchExperiencesExact(ctx, communityID, req.Query)
		if err != nil {
			return nil, fmt.Errorf("failed to search experiences: %w", err)
		}
		apiResults = append(apiResults, experienceResults...)
	}

	return apiResults, nil
}

// convertToSearchResultItemBatch converts a UnifiedSearchResult to a SearchResultItem
// using a pre-fetched user map for O(1) lookups instead of per-result DB queries.
func (s *Service) convertToSearchResultItemBatch(result storage.UnifiedSearchResult, userMap map[string]*api.User) (*api.SearchResultItem, error) {
	var latDeg, lonDeg float64
	if result.Location != nil && result.Location.Geolocation != nil {
		latDeg = result.Location.Geolocation.LatitudeDeg
		lonDeg = result.Location.Geolocation.LongitudeDeg
	}

	item := &api.SearchResultItem{
		CompositeScore:     result.CompositeScore,
		SemanticSimilarity: result.SemanticSimilarity,
		DistanceMeters:     result.DistanceMeters,
	}

	switch result.ItemType {
	case "gear":
		owner := userMap[result.Gear.OwnerId]
		item.ItemType = api.SearchItemType_SEARCH_ITEM_TYPE_GEAR
		gearMsg := &api.Gear{
			Id:           result.Gear.Id,
			Name:         result.Gear.Name,
			Description:  result.Gear.Description,
			Owner:        owner,
			MediaIds:     result.Gear.MediaIds,
			LocationId:   result.Gear.LocationId,
			LatitudeDeg:  latDeg,
			LongitudeDeg: lonDeg,
			Availability: api.Availability(result.Availability),
		}
		if result.Gear.SourceUrl != "" {
			gearMsg.SourceUrl = &result.Gear.SourceUrl
		}
		if category := result.Gear.GetCategory().GetValue(); category != "" {
			gearMsg.Category = &category
		}
		item.Item = &api.SearchResultItem_Gear{Gear: gearMsg}

	case "request":
		requester := userMap[result.Request.RequesterId]
		item.ItemType = api.SearchItemType_SEARCH_ITEM_TYPE_REQUEST
		item.Item = &api.SearchResultItem_Request{
			Request: &api.Request{
				Id:           result.Request.Id,
				Title:        result.Request.Title,
				Description:  result.Request.Description,
				State:        api.RequestState(result.Request.State),
				Requester:    requester,
				MediaIds:     result.Request.MediaIds,
				LocationId:   result.Request.LocationId,
				LatitudeDeg:  latDeg,
				LongitudeDeg: lonDeg,
			},
		}

	case "experience":
		host := userMap[result.Experience.OwnerId]
		item.ItemType = api.SearchItemType_SEARCH_ITEM_TYPE_EXPERIENCE
		item.Item = &api.SearchResultItem_Experience{
			Experience: &api.Experience{
				Id:              result.Experience.Id,
				Name:            result.Experience.Name,
				Description:     result.Experience.Description,
				Owner:           host,
				MediaIds:        result.Experience.MediaIds,
				LocationId:      result.Experience.LocationId,
				ConversationId:  &result.Experience.ConversationId,
				LatitudeDeg:     latDeg,
				LongitudeDeg:    lonDeg,
				State:           convertExperienceState(result.Experience.State),
				Time:            services.ConvertTimeModelsToAPI(result.Experience.Time),
				MaxParticipants: result.Experience.MaxParticipants,
			},
		}

	default:
		return nil, fmt.Errorf("unknown item type: %s", result.ItemType)
	}

	return item, nil
}

// searchUsers searches for community members by name using case-insensitive partial matching.
func (s *Service) searchUsers(ctx context.Context, communityID, query string) ([]*api.SearchResultItem, error) {
	// Get all community memberships
	memberships, err := storage.QueryByField[*models.CommunityUser](s.storage, ctx, "community_id", communityID)
	if err != nil {
		return nil, fmt.Errorf("failed to query community memberships: %w", err)
	}

	// Collect all user IDs and batch fetch
	userIDs := storage.CollectField(memberships, func(m *models.CommunityUser) string { return m.UserId })

	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, userIDs)
	if err != nil {
		return nil, fmt.Errorf("batch fetch users: %w", err)
	}

	// Filter by query
	queryLower := strings.ToLower(query)
	var results []*api.SearchResultItem
	for _, userID := range userIDs {
		user := userMap[userID]
		if user == nil {
			continue
		}

		// Case-insensitive partial match on name
		if query == "" || strings.Contains(strings.ToLower(user.Name), queryLower) {
			results = append(results, &api.SearchResultItem{
				ItemType:       api.SearchItemType_SEARCH_ITEM_TYPE_USER,
				CompositeScore: 1.0, // Exact match gets max score
				Item: &api.SearchResultItem_User{
					User: user,
				},
			})
		}
	}

	return results, nil
}

// searchGearExact searches for gear in a community by name using case-insensitive partial matching.
func (s *Service) searchGearExact(ctx context.Context, communityID, query string) ([]*api.SearchResultItem, error) {
	// Get all gear shared with this community
	communityGears, err := storage.QueryByField[*models.CommunityGear](s.storage, ctx, "community_id", communityID)
	if err != nil {
		return nil, fmt.Errorf("failed to query community gear: %w", err)
	}

	// Collect gear IDs and build availability map
	gearIDs := storage.CollectField(communityGears, func(cg *models.CommunityGear) string { return cg.GearId })
	availabilityMap := make(map[string]models.Availability, len(communityGears))
	for _, cg := range communityGears {
		availabilityMap[cg.GearId] = cg.Availability
	}

	// Batch fetch all gear
	gearProtoMap, err := storage.GetByIDs[*models.Gear](s.storage, ctx, gearIDs)
	if err != nil {
		return nil, fmt.Errorf("batch fetch gear: %w", err)
	}

	// Filter by query and collect owner IDs
	queryLower := strings.ToLower(query)
	type matchedGear struct {
		gear         *models.Gear
		availability models.Availability
	}
	var matched []matchedGear
	var ownerIDs []string

	for _, gearID := range gearIDs {
		gear, ok := gearProtoMap[gearID]
		if !ok {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(gear.Name), queryLower) {
			avail := availabilityMap[gearID]
			matched = append(matched, matchedGear{gear: gear, availability: avail})
			ownerIDs = append(ownerIDs, gear.OwnerId)
		}
	}

	// Batch fetch all owners
	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, ownerIDs)
	if err != nil {
		return nil, fmt.Errorf("batch fetch gear owners: %w", err)
	}

	// Build results
	var results []*api.SearchResultItem
	for _, m := range matched {
		owner := userMap[m.gear.OwnerId]
		if owner == nil {
			continue
		}
		gearMsg := &api.Gear{
			Id:           m.gear.Id,
			Name:         m.gear.Name,
			Description:  m.gear.Description,
			Owner:        owner,
			MediaIds:     m.gear.MediaIds,
			LocationId:   m.gear.LocationId,
			Availability: api.Availability(m.availability),
		}
		if m.gear.SourceUrl != "" {
			gearMsg.SourceUrl = &m.gear.SourceUrl
		}
		if category := m.gear.GetCategory().GetValue(); category != "" {
			gearMsg.Category = &category
		}
		results = append(results, &api.SearchResultItem{
			ItemType:       api.SearchItemType_SEARCH_ITEM_TYPE_GEAR,
			CompositeScore: 1.0,
			Item:           &api.SearchResultItem_Gear{Gear: gearMsg},
		})
	}

	return results, nil
}

// searchRequestsExact searches for requests in a community by title using case-insensitive partial matching.
func (s *Service) searchRequestsExact(ctx context.Context, communityID, query string) ([]*api.SearchResultItem, error) {
	// Get all requests shared with this community via CommunityRequest junction table
	communityRequests, err := storage.QueryByFields[*models.CommunityRequest](s.storage, ctx, map[string]any{
		"community_id": communityID,
		"archived":     false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query community requests: %w", err)
	}

	// Collect request IDs
	requestIDs := storage.CollectField(communityRequests, func(cr *models.CommunityRequest) string { return cr.RequestId })

	// Batch fetch all requests
	requestProtoMap, err := storage.GetByIDs[*models.Request](s.storage, ctx, requestIDs)
	if err != nil {
		return nil, fmt.Errorf("batch fetch requests: %w", err)
	}

	// Filter by query and collect requester IDs
	queryLower := strings.ToLower(query)
	var matched []*models.Request
	var requesterIDs []string
	for _, requestID := range requestIDs {
		request, ok := requestProtoMap[requestID]
		if !ok {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(request.Title), queryLower) {
			matched = append(matched, request)
			requesterIDs = append(requesterIDs, request.RequesterId)
		}
	}

	// Batch fetch all requesters
	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, requesterIDs)
	if err != nil {
		return nil, fmt.Errorf("batch fetch requesters: %w", err)
	}

	// Build results
	var results []*api.SearchResultItem
	for _, request := range matched {
		requester := userMap[request.RequesterId]
		if requester == nil {
			continue
		}
		results = append(results, &api.SearchResultItem{
			ItemType:       api.SearchItemType_SEARCH_ITEM_TYPE_REQUEST,
			CompositeScore: 1.0,
			Item: &api.SearchResultItem_Request{
				Request: &api.Request{
					Id:          request.Id,
					Title:       request.Title,
					Description: request.Description,
					State:       api.RequestState(request.State),
					Requester:   requester,
					MediaIds:    request.MediaIds,
					LocationId:  request.LocationId,
				},
			},
		})
	}

	return results, nil
}

// searchExperiencesExact searches for experiences in a community by name using case-insensitive partial matching.
func (s *Service) searchExperiencesExact(ctx context.Context, communityID, query string) ([]*api.SearchResultItem, error) {
	// Get all experiences shared with this community
	communityExperiences, err := storage.QueryByField[*models.CommunityExperience](s.storage, ctx, "community_id", communityID)
	if err != nil {
		return nil, fmt.Errorf("failed to query community experiences: %w", err)
	}

	// Collect experience IDs
	experienceIDs := storage.CollectField(communityExperiences, func(ce *models.CommunityExperience) string { return ce.ExperienceId })

	// Batch fetch all experiences
	expProtoMap, err := storage.GetByIDs[*models.Experience](s.storage, ctx, experienceIDs)
	if err != nil {
		return nil, fmt.Errorf("batch fetch experiences: %w", err)
	}

	// Filter by query and collect owner IDs
	queryLower := strings.ToLower(query)
	var matched []*models.Experience
	var ownerIDs []string
	for _, expID := range experienceIDs {
		experience, ok := expProtoMap[expID]
		if !ok {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(experience.Name), queryLower) {
			matched = append(matched, experience)
			ownerIDs = append(ownerIDs, experience.OwnerId)
		}
	}

	// Batch fetch all hosts
	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, ownerIDs)
	if err != nil {
		return nil, fmt.Errorf("batch fetch experience hosts: %w", err)
	}

	// Build results
	var results []*api.SearchResultItem
	for _, experience := range matched {
		host := userMap[experience.OwnerId]
		if host == nil {
			continue
		}
		results = append(results, &api.SearchResultItem{
			ItemType:       api.SearchItemType_SEARCH_ITEM_TYPE_EXPERIENCE,
			CompositeScore: 1.0,
			Item: &api.SearchResultItem_Experience{
				Experience: &api.Experience{
					Id:              experience.Id,
					Name:            experience.Name,
					Description:     experience.Description,
					Owner:           host,
					MediaIds:        experience.MediaIds,
					LocationId:      experience.LocationId,
					ConversationId:  &experience.ConversationId,
					State:           convertExperienceState(experience.State),
					Time:            services.ConvertTimeModelsToAPI(experience.Time),
					MaxParticipants: experience.MaxParticipants,
				},
			},
		})
	}

	return results, nil
}

func convertExperienceState(state models.ExperienceState) api.ExperienceState {
	switch state {
	case models.ExperienceState_EXPERIENCE_STATE_ACTIVE:
		return api.ExperienceState_EXPERIENCE_STATE_ACTIVE
	case models.ExperienceState_EXPERIENCE_STATE_JOINED:
		return api.ExperienceState_EXPERIENCE_STATE_JOINED
	case models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS:
		return api.ExperienceState_EXPERIENCE_STATE_IN_PROCESS
	case models.ExperienceState_EXPERIENCE_STATE_COMPLETED:
		return api.ExperienceState_EXPERIENCE_STATE_COMPLETED
	case models.ExperienceState_EXPERIENCE_STATE_CANCELLED:
		return api.ExperienceState_EXPERIENCE_STATE_CANCELLED
	default:
		return api.ExperienceState_EXPERIENCE_STATE_UNSPECIFIED
	}
}
