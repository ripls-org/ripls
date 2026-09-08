package auth

import (
	"context"
	"fmt"
	"sync"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// EntityKind identifies the type of a community-scoped entity for the shared
// access helper.
type EntityKind int

const (
	// EntityGear represents a gear item (CommunityGear join table).
	EntityGear EntityKind = iota
	// EntityExperience represents an experience (CommunityExperience join table).
	EntityExperience EntityKind = iota
	// EntityRequest represents a request (CommunityRequest join table).
	EntityRequest EntityKind = iota
)

// String returns a loggable label for the entity kind.
func (e EntityKind) String() string {
	switch e {
	case EntityGear:
		return "gear"
	case EntityExperience:
		return "experience"
	case EntityRequest:
		return "request"
	default:
		return "unknown"
	}
}

// entityCommunityCacheKey is a sync.Map key used to cache the community IDs
// an entity is shared with. Stored in the WithCommunityCache context map
// alongside the existing (communityID, userID) pairs. includeArchived is part
// of the key because the strict and read gates resolve different community
// sets for gear (#2695).
type entityCommunityCacheKey struct {
	kind            EntityKind
	entityID        string
	includeArchived bool
}

// RequireAccessToCommunityScopedEntity enforces that the caller is authorized
// to read an entity shared with one or more communities.
//
// Owner-bypass: when callerID == ownerID (and ownerID is non-empty), the
// function loads the entity's shared communities and returns the full set as
// both sharedCommunityIDs and callerCommunityIDs without a membership check.
// This allows owners to read their own gear/experiences/requests even when the
// entity is not yet shared with any community.
//
// For non-owners: loads the entity's CommunityGear / CommunityExperience /
// CommunityRequest rows to derive sharedCommunityIDs, then calls
// GetCommunitiesWithMembership to determine which of those communities the
// caller is an active member of. Returns PermissionDenied if the entity is
// shared nowhere or the caller is in none of the shared communities.
//
// Returns (sharedCommunityIDs, callerCommunityIDs, error). Handlers that
// render a viewer-aware response (filtering SharedCommunities to the caller's
// lens) should use callerCommunityIDs; handlers that only need the gate can
// discard the return values.
//
// Composes with WithCommunityCache: the entity-to-community mapping is cached
// in the same context-scoped sync.Map, so a second call with the same
// entityID/kind in the same request costs one fewer DB query.
func RequireAccessToCommunityScopedEntity(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	callerID string,
	kind EntityKind,
	entityID string,
	ownerID string,
) (sharedCommunityIDs, callerCommunityIDs []string, err error) {
	return requireAccessToCommunityScopedEntity(ctx, s, callerID, kind, entityID, ownerID, false)
}

// RequireReadAccessToCommunityScopedEntity is the READ-shaped variant of
// RequireAccessToCommunityScopedEntity: for gear it also counts archived
// CommunityGear rows (items unshared or given away) as a sharing
// relationship. Completed giveaways archive every share, but the item must
// stay readable — the conversation stays open, Past Giveaways lists it, and
// the recipient is exposed to viewers (docs/workflows/giveaway.md, #2695).
// Mutation paths must keep using the strict gate so archived shares never
// authorize new bookings/claims.
func RequireReadAccessToCommunityScopedEntity(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	callerID string,
	kind EntityKind,
	entityID string,
	ownerID string,
) (sharedCommunityIDs, callerCommunityIDs []string, err error) {
	return requireAccessToCommunityScopedEntity(ctx, s, callerID, kind, entityID, ownerID, true)
}

func requireAccessToCommunityScopedEntity(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	callerID string,
	kind EntityKind,
	entityID string,
	ownerID string,
	includeArchived bool,
) (sharedCommunityIDs, callerCommunityIDs []string, err error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RequireAccessToCommunityScopedEntity",
		"caller_id", callerID,
		"entity_id", entityID,
		"kind", kind.String(),
	)

	isOwner := ownerID != "" && callerID == ownerID

	// Load the community set the entity is shared with, using the cache when
	// the context was wrapped with WithCommunityCache.
	sharedCommunityIDs, err = loadEntityCommunityIDsCached(ctx, s, kind, entityID, includeArchived)
	if err != nil {
		return nil, nil, connecterr.Internal(ctx, "RequireAccessToCommunityScopedEntity", err,
			"entity_id", entityID, "kind", kind.String())
	}

	if isOwner {
		// Owner sees all communities the entity is shared with.
		return sharedCommunityIDs, sharedCommunityIDs, nil
	}

	if len(sharedCommunityIDs) == 0 {
		logger.WarnContext(ctx, "access denied: entity not shared with any community",
			"reason", "none")
		return nil, nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("not authorized to access this resource"))
	}

	// Determine which shared communities the caller is an active member of in a
	// single batched JOIN query.
	pairs, err := s.GetCommunitiesWithMembership(ctx, sharedCommunityIDs, callerID)
	if err != nil {
		return nil, nil, connecterr.Internal(ctx, "RequireAccessToCommunityScopedEntity", err,
			"entity_id", entityID, "kind", kind.String())
	}

	callerCommunityIDs = make([]string, 0, len(pairs))
	for _, cid := range sharedCommunityIDs {
		pair, ok := pairs[cid]
		if !ok {
			continue
		}
		if isSoftDeleted(pair.Community) {
			continue
		}
		if pair.Membership == nil {
			continue
		}
		if pair.Membership.Deleted != nil && pair.Membership.Deleted.DeletedAtUnixSec > 0 {
			continue
		}
		callerCommunityIDs = append(callerCommunityIDs, cid)
	}

	if len(callerCommunityIDs) == 0 {
		logger.WarnContext(ctx, "access denied: caller is not a member of any shared community",
			"reason", "nonmember")
		return nil, nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("not authorized to access this resource"))
	}

	return sharedCommunityIDs, callerCommunityIDs, nil
}

// loadEntityCommunityIDsCached fetches the community IDs an entity is shared
// with, reading from the WithCommunityCache sync.Map when available and
// populating it on miss.
func loadEntityCommunityIDsCached(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	kind EntityKind,
	entityID string,
	includeArchived bool,
) ([]string, error) {
	key := entityCommunityCacheKey{kind: kind, entityID: entityID, includeArchived: includeArchived}

	// Try cache first.
	if m, ok := ctx.Value(communityCacheCtxKey{}).(*sync.Map); ok {
		if v, hit := m.Load(key); hit {
			return v.([]string), nil
		}
	}

	ids, err := loadEntityCommunityIDs(ctx, s, kind, entityID, includeArchived)
	if err != nil {
		return nil, err
	}

	// Populate cache on success.
	if m, ok := ctx.Value(communityCacheCtxKey{}).(*sync.Map); ok {
		m.Store(key, ids)
	}

	return ids, nil
}

// loadEntityCommunityIDs fetches the community IDs an entity is shared with
// by querying the appropriate CommunityX join table. For gear, archived rows
// (items unshared or given away) no longer represent an active sharing
// relationship, so the strict gate excludes them (mutations must not be
// authorized by a past share); the read gate includes them so completed
// giveaways stay visible per docs/workflows/giveaway.md (#2695).
// includeArchived only affects gear — the other join tables have no
// archived flag.
func loadEntityCommunityIDs(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	kind EntityKind,
	entityID string,
	includeArchived bool,
) ([]string, error) {
	switch kind {
	case EntityGear:
		rows, err := s.QueryByField(ctx, "gear_id", entityID, &models.CommunityGear{})
		if err != nil {
			return nil, fmt.Errorf("failed to query community gear: %w", err)
		}
		ids := make([]string, 0, len(rows))
		for _, m := range rows {
			cg := m.(*models.CommunityGear)
			if cg.Archived && !includeArchived {
				continue
			}
			ids = append(ids, cg.CommunityId)
		}
		return ids, nil

	case EntityExperience:
		rows, err := s.QueryByField(ctx, "experience_id", entityID, &models.CommunityExperience{})
		if err != nil {
			return nil, fmt.Errorf("failed to query community experience: %w", err)
		}
		ids := make([]string, 0, len(rows))
		for _, m := range rows {
			ids = append(ids, m.(*models.CommunityExperience).CommunityId)
		}
		return ids, nil

	case EntityRequest:
		rows, err := s.QueryByField(ctx, "request_id", entityID, &models.CommunityRequest{})
		if err != nil {
			return nil, fmt.Errorf("failed to query community request: %w", err)
		}
		ids := make([]string, 0, len(rows))
		for _, m := range rows {
			ids = append(ids, m.(*models.CommunityRequest).CommunityId)
		}
		return ids, nil

	default:
		return nil, fmt.Errorf("unknown entity kind: %d", kind)
	}
}

// RequireSharedCommunityWithUser verifies that the caller shares at least one
// active community with the target user.
//
// Self-bypass: when callerID == targetID, returns nil without any DB lookup.
//
// Returns PermissionDenied when the caller and target share no active
// communities, and CodeInternal on storage failures.
func RequireSharedCommunityWithUser(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	callerID string,
	targetID string,
) error {
	if callerID == targetID {
		return nil
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RequireSharedCommunityWithUser",
		"caller_id", callerID,
		"target_user_id", targetID,
	)

	// Fetch active CommunityUser rows for both users. QueryByField excludes
	// soft-deleted rows by default, so ex-members are naturally excluded.
	callerRows, err := s.QueryByField(ctx, "user_id", callerID, &models.CommunityUser{})
	if err != nil {
		return connecterr.Internal(ctx, "RequireSharedCommunityWithUser", err,
			"caller_id", callerID, "target_user_id", targetID)
	}
	targetRows, err := s.QueryByField(ctx, "user_id", targetID, &models.CommunityUser{})
	if err != nil {
		return connecterr.Internal(ctx, "RequireSharedCommunityWithUser", err,
			"caller_id", callerID, "target_user_id", targetID)
	}

	callerCommunities := make(map[string]struct{}, len(callerRows))
	for _, m := range callerRows {
		callerCommunities[m.(*models.CommunityUser).CommunityId] = struct{}{}
	}

	for _, m := range targetRows {
		if _, ok := callerCommunities[m.(*models.CommunityUser).CommunityId]; ok {
			return nil
		}
	}

	logger.WarnContext(ctx, "access denied: caller and target share no active community",
		"reason", "nonmember")
	return connect.NewError(connect.CodePermissionDenied,
		fmt.Errorf("not authorized to access this resource"))
}
