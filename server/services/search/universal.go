package search

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

const (
	// defaultUniversalGroupSize is the per-group result cap applied when
	// max_results_per_group is unset or non-positive.
	defaultUniversalGroupSize = 10

	// maxUniversalGroupSize is the ceiling for max_results_per_group; larger
	// requested values are clamped to it.
	maxUniversalGroupSize = 25
)

// UniversalSearch searches everything in the caller's communities and returns
// the matches as fixed groups (library / plans / people). The community scope
// is always derived from the caller's own memberships — the request carries no
// scope, so a client can never widen the search beyond them.
func (s *Service) UniversalSearch(
	ctx context.Context,
	req *connect.Request[api.UniversalSearchRequest],
) (*connect.Response[api.UniversalSearchResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	query := strings.TrimSpace(req.Msg.Query)
	if query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("query must be non-empty"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.InfoContext(ctx, "universal search initiated",
		"search_query", query,
	)

	activeCommunityIDs, err := s.callerActiveCommunityIDs(ctx, authInfo.UserID)
	if err != nil {
		return nil, err
	}
	if len(activeCommunityIDs) == 0 {
		return connect.NewResponse(&api.UniversalSearchResponse{}), nil
	}

	results, err := s.searchCommunitiesDeduplicated(ctx, query, activeCommunityIDs)
	if err != nil {
		logger.ErrorContext(ctx, "universal search query failed", "error", err)
		return nil, connecterr.Internal(ctx, "UniversalSearch", err)
	}

	groupLimit := universalGroupLimit(req.Msg)
	library, plans, people := groupUniversalResults(results, groupLimit)

	logger.InfoContext(ctx, "universal search completed",
		"search_query", query,
		"library_count", len(library),
		"plans_count", len(plans),
		"people_count", len(people),
	)

	return connect.NewResponse(&api.UniversalSearchResponse{
		LibraryResults: library,
		PlansResults:   plans,
		PeopleResults:  people,
	}), nil
}

// callerActiveCommunityIDs returns the IDs of the active communities the
// caller belongs to. Memberships pointing at missing or soft-deleted
// communities are dropped so stale rows degrade gracefully.
func (s *Service) callerActiveCommunityIDs(ctx context.Context, userID string) ([]string, error) {
	memberships, err := storage.QueryByField[*models.CommunityUser](s.storage, ctx, "user_id", userID)
	if err != nil {
		return nil, connecterr.Internal(ctx, "UniversalSearch", err,
			"detail", "failed to list memberships")
	}
	communityIDs := storage.CollectField(memberships, func(m *models.CommunityUser) string { return m.CommunityId })
	activeCommunityIDs, _, err := auth.FilterActiveMemberCommunities(ctx, s.storage, communityIDs, userID)
	if err != nil {
		return nil, err
	}
	return activeCommunityIDs, nil
}

// searchCommunitiesDeduplicated runs the semantic search across all item types
// in every given community and deduplicates the merged results by entity key,
// mirroring the cross-community loop in Search.
func (s *Service) searchCommunitiesDeduplicated(
	ctx context.Context,
	query string,
	communityIDs []string,
) ([]*api.SearchResultItem, error) {
	itemTypes := s.buildItemTypeSet(nil)

	var apiResults []*api.SearchResultItem
	seenIDs := make(map[string]bool)
	for _, cid := range communityIDs {
		perCommunityReq := &api.SearchRequest{
			Query: query,
		}
		results, err := s.semanticSearch(ctx, cid, perCommunityReq, itemTypes)
		if err != nil {
			return nil, fmt.Errorf("search community %s: %w", cid, err)
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
	return apiResults, nil
}

// universalGroupLimit resolves the per-group result cap: the server default
// when the field is unset or non-positive, clamped to the server ceiling.
func universalGroupLimit(req *api.UniversalSearchRequest) int {
	limit := int(req.GetMaxResultsPerGroup())
	if limit <= 0 {
		return defaultUniversalGroupSize
	}
	if limit > maxUniversalGroupSize {
		return maxUniversalGroupSize
	}
	return limit
}

// groupUniversalResults splits deduplicated results into the three response
// groups: library (gear + requests), plans (experiences), and people (users).
// Each group is sorted by composite score descending and capped to limit.
func groupUniversalResults(
	results []*api.SearchResultItem,
	limit int,
) (library, plans, people []*api.SearchResultItem) {
	for _, r := range results {
		switch r.ItemType {
		case api.SearchItemType_SEARCH_ITEM_TYPE_GEAR, api.SearchItemType_SEARCH_ITEM_TYPE_REQUEST:
			library = append(library, r)
		case api.SearchItemType_SEARCH_ITEM_TYPE_EXPERIENCE:
			plans = append(plans, r)
		case api.SearchItemType_SEARCH_ITEM_TYPE_USER:
			people = append(people, r)
		}
	}
	return sortAndCapByScore(library, limit),
		sortAndCapByScore(plans, limit),
		sortAndCapByScore(people, limit)
}

// sortAndCapByScore sorts items by composite score descending — stable, so
// per-community relevance order is preserved among ties — and truncates the
// slice to limit.
func sortAndCapByScore(items []*api.SearchResultItem, limit int) []*api.SearchResultItem {
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CompositeScore > items[j].CompositeScore
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}
