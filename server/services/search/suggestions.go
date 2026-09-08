package search

import (
	"context"
	"fmt"
	"sort"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/known_for"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// suggestionsCategoryMax caps the top_known_for_categories list
// returned by GetSearchSuggestions. The community list is intentionally
// uncapped — the carousel surfaces the caller's full active
// membership set so they can scope a search to any of their
// communities, not just the most recently joined five.
const suggestionsCategoryMax = 5

// userMembership pairs a community ID with the timestamp the caller
// joined it. Used internally to order top-communities most-recently-
// joined first and to feed known_for.Derive the user's full active
// scope without a second membership round-trip.
type userMembership struct {
	communityID   string
	joinedUnixSec int64
}

// GetSearchSuggestions returns personalized suggestions used to seed
// the idle search panel: the caller's top "known for" categories and
// a short list of their communities, most recently joined first.
//
// The caller does not specify a community scope — the server uses the
// authenticated user's full active membership set for both derived
// signals.
func (s *Service) GetSearchSuggestions(
	ctx context.Context,
	_ *connect.Request[api.GetSearchSuggestionsRequest],
) (*connect.Response[api.GetSearchSuggestionsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetSearchSuggestions",
	)

	memberships, err := s.activeMemberships(ctx, authInfo.UserID)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetSearchSuggestions.Memberships", err)
	}
	if len(memberships) == 0 {
		logger.InfoContext(ctx, "suggestions: caller has no active memberships, returning empty")
		return connect.NewResponse(&api.GetSearchSuggestionsResponse{}), nil
	}

	communityIDs := make([]string, 0, len(memberships))
	for _, m := range memberships {
		communityIDs = append(communityIDs, m.communityID)
	}

	categories, err := known_for.Derive(ctx, s.storage, known_for.Options{
		Mode:         known_for.ModePerUser,
		OwnerID:      authInfo.UserID,
		CommunityIDs: communityIDs,
	})
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetSearchSuggestions.DeriveKnownFor", err)
	}
	if len(categories) > suggestionsCategoryMax {
		categories = categories[:suggestionsCategoryMax]
	}

	topCommunities, err := s.namesForMemberships(ctx, memberships)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetSearchSuggestions.TopCommunities", err)
	}

	logger.InfoContext(ctx, "suggestions: completed",
		"category_count", len(categories),
		"community_count", len(topCommunities),
	)

	return connect.NewResponse(&api.GetSearchSuggestionsResponse{
		TopKnownForCategories: categories,
		TopCommunities:        topCommunities,
	}), nil
}

// activeMemberships returns the user's CommunityUser rows that are
// not soft-deleted, sorted most-recently-joined first.
func (s *Service) activeMemberships(
	ctx context.Context,
	userID string,
) ([]userMembership, error) {
	rows, err := storage.QueryByField[*models.CommunityUser](
		s.storage, ctx, "user_id", userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query memberships: %w", err)
	}

	active := make([]userMembership, 0, len(rows))
	for _, m := range rows {
		if m.Deleted != nil {
			continue
		}
		active = append(active, userMembership{
			communityID:   m.CommunityId,
			joinedUnixSec: m.CreatedAtUnixSec,
		})
	}
	sort.Slice(active, func(i, j int) bool {
		return active[i].joinedUnixSec > active[j].joinedUnixSec
	})
	return active, nil
}

// namesForMemberships resolves community names for every entry of
// [memberships] (already most-recently-joined first). Soft-deleted
// communities are dropped. Uncapped on purpose — the carousel UI
// surfaces the caller's full community set so they can scope a search
// to any of them.
//
// Query budget: 1 batched GetByIDs across all community ids.
func (s *Service) namesForMemberships(
	ctx context.Context,
	memberships []userMembership,
) ([]*api.SharedCommunityRef, error) {
	if len(memberships) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(memberships))
	for _, m := range memberships {
		ids = append(ids, m.communityID)
	}
	communities, err := storage.GetByIDs[*models.Community](s.storage, ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load community names: %w", err)
	}

	refs := make([]*api.SharedCommunityRef, 0, len(memberships))
	for _, m := range memberships {
		c, ok := communities[m.communityID]
		if !ok || c == nil {
			continue
		}
		if c.Deleted != nil {
			continue
		}
		ref := &api.SharedCommunityRef{
			Id:   c.Id,
			Name: c.Name,
		}
		if len(c.MediaIds) > 0 && c.MediaIds[0] != "" {
			primary := c.MediaIds[0]
			ref.MediaId = &primary
		}
		refs = append(refs, ref)
	}
	return refs, nil
}
