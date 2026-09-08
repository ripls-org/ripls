package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

type communityCacheKey struct {
	communityID string
	userID      string
}

type communityCacheValue struct {
	community  *models.Community
	membership *models.CommunityUser
}

type communityCacheCtxKey struct{}

// WithCommunityCache returns a context that carries a request-scoped cache for
// (Community, CommunityUser) lookups. Subsequent calls to
// RequireMemberOfActiveCommunity / RequireActiveCommunity within the same
// request reuse the cached values, so handlers that check membership in
// multiple sub-functions don't pay for repeat queries. Cache lifetime is the
// request lifetime; no eviction.
//
// The pattern mirrors storage.WithQueryStats — callers that want memoization
// opt in by wrapping the incoming context once at the top of an RPC handler.
// Calls without WithCommunityCache still work; they just don't memoize.
func WithCommunityCache(ctx context.Context) context.Context {
	if ctx.Value(communityCacheCtxKey{}) != nil {
		return ctx
	}
	return context.WithValue(ctx, communityCacheCtxKey{}, &sync.Map{})
}

func cacheLookup(ctx context.Context, key communityCacheKey) (*communityCacheValue, bool) {
	m, ok := ctx.Value(communityCacheCtxKey{}).(*sync.Map)
	if !ok {
		return nil, false
	}
	v, ok := m.Load(key)
	if !ok {
		return nil, false
	}
	return v.(*communityCacheValue), true
}

func cacheStore(ctx context.Context, key communityCacheKey, value *communityCacheValue) {
	m, ok := ctx.Value(communityCacheCtxKey{}).(*sync.Map)
	if !ok {
		return
	}
	m.Store(key, value)
}

// RequireMemberOfActiveCommunity authorizes a caller against a community in a
// single round-trip: it fetches the Community and the caller's CommunityUser
// row via one LEFT JOIN, then enforces the active-community + membership
// invariants.
//
// Error semantics (in priority order):
//   - NotFound: community does not exist, OR community is soft-deleted and the
//     caller is not a member (no information leak — non-members cannot
//     distinguish "deleted" from "doesn't exist").
//   - FailedPrecondition: community is soft-deleted and the caller is a member.
//     Members get a distinct code so the client can route them to Settings →
//     Communities for the restore flow.
//   - PermissionDenied: community is active but the caller is not a member.
//
// On success returns the (Community, CommunityUser) pair. Callers should reuse
// the returned Community instead of calling GetByID again — that's the whole
// point of this helper.
//
// Caching: if the context was wrapped with WithCommunityCache, the result is
// memoized by (communityID, userID) for the request lifetime.
func RequireMemberOfActiveCommunity(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	communityID, userID string,
) (*models.Community, *models.CommunityUser, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RequireMemberOfActiveCommunity",
		"community_id", communityID,
		"user_id", userID,
	)

	key := communityCacheKey{communityID: communityID, userID: userID}
	if cached, ok := cacheLookup(ctx, key); ok {
		return cached.community, cached.membership, evaluateActiveMembership(cached.community, cached.membership)
	}

	community, membership, err := s.GetCommunityWithMembership(ctx, communityID, userID)
	if err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			logger.DebugContext(ctx, "community not found")
			return nil, nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("community not found"))
		}
		logger.ErrorContext(ctx, "failed to fetch community + membership", "error", err)
		return nil, nil, connecterr.Internal(ctx, "RequireMemberOfActiveCommunity", err,
			"community_id", communityID, "user_id", userID)
	}

	cacheStore(ctx, key, &communityCacheValue{community: community, membership: membership})
	return community, membership, evaluateActiveMembership(community, membership)
}

// RequireActiveCommunity is the no-membership variant: same semantics for
// active/deleted, but does not require the caller to be a member. Used by
// admin paths and any future public-read RPCs.
//
// Returns NotFound if the community does not exist or is soft-deleted. Does
// not differentiate the soft-deleted case (no caller identity to gate it on).
func RequireActiveCommunity(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	communityID string,
) (*models.Community, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RequireActiveCommunity",
		"community_id", communityID,
	)

	community := &models.Community{}
	if err := s.GetByID(ctx, communityID, community); err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			logger.DebugContext(ctx, "community not found")
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("community not found"))
		}
		logger.ErrorContext(ctx, "failed to fetch community", "error", err)
		return nil, connecterr.Internal(ctx, "RequireActiveCommunity", err, "community_id", communityID)
	}

	if isSoftDeleted(community) {
		logger.DebugContext(ctx, "community is soft-deleted")
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("community not found"))
	}
	return community, nil
}

func isSoftDeleted(c *models.Community) bool {
	return c.Deleted != nil && c.Deleted.DeletedAtUnixSec > 0
}

// FilterActiveMemberCommunities resolves a list of community IDs to the subset
// the caller can read in a multi-community surface (search, feed,
// list-all-my). Uses a single batched JOIN query — O(1) round-trips for N
// communities, replacing the per-id loop pattern.
//
// Behavior:
//   - Communities that do not exist OR are soft-deleted are silently dropped
//     from the returned slice. Deleted communities are invisible everywhere
//     except Settings → Communities, so a stale ID in the caller's input
//     should degrade gracefully.
//   - If the caller is NOT a member of an active community in the input list,
//     returns PermissionDenied immediately. Matches today's per-id behavior:
//     non-member access surfaces a real client error, it isn't masked.
//
// Returns the subset of communityIDs the caller can act on (preserving input
// order), and a map of communityID → Community for callers that need the
// resolved Community objects (e.g., feed). The map only contains entries for
// IDs that survived the filter.
//
// Storage failures wrap with connecterr.Internal.
func FilterActiveMemberCommunities(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	communityIDs []string,
	userID string,
) ([]string, map[string]*models.Community, error) {
	if len(communityIDs) == 0 {
		return nil, map[string]*models.Community{}, nil
	}

	pairs, err := s.GetCommunitiesWithMembership(ctx, communityIDs, userID)
	if err != nil {
		return nil, nil, connecterr.Internal(ctx, "FilterActiveMemberCommunities", err,
			"user_id", userID, "community_count", len(communityIDs))
	}

	active := make([]string, 0, len(communityIDs))
	resolved := make(map[string]*models.Community, len(communityIDs))
	for _, cid := range communityIDs {
		pair, ok := pairs[cid]
		if !ok {
			// Community does not exist; drop silently.
			continue
		}
		if isSoftDeleted(pair.Community) {
			// Soft-deleted; drop silently per design doc §6.5.
			continue
		}
		if pair.Membership == nil {
			// Active community, caller is not a member — surface as an error
			// to match the single-community RequireMemberOfActiveCommunity
			// semantics and today's per-id loop behavior.
			return nil, nil, connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("user is not a member of community %s", cid))
		}
		active = append(active, cid)
		resolved[cid] = pair.Community
	}
	return active, resolved, nil
}

func evaluateActiveMembership(community *models.Community, membership *models.CommunityUser) error {
	deleted := isSoftDeleted(community)
	isMember := membership != nil

	switch {
	case deleted && !isMember:
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("community not found"))
	case deleted && isMember:
		return connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("community is in deleted state and pending restore or purge"))
	case !deleted && !isMember:
		return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("user is not a member of this community"))
	default:
		return nil
	}
}
