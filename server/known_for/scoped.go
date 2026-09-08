package known_for

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// Scope loading, shared by experiences and requests.
//
// loadScopedExperiences and loadScopedRequests were 86 duplicated lines
// differing only in the two stored types, the owner field ("owner_id" vs
// "requester_id") and the pivot's foreign key. Both traverse the same shape:
// per-user, query the owner's items then pivot to keep the ones shared into
// scope; per-community, pivot by community then batch-fetch. Keeping one copy
// of that traversal is what stops the two from silently diverging (#2816).

// scopedEntity is what scope loading needs from a stored item.
type scopedEntity interface {
	proto.Message
	GetId() string
	GetDeleted() *models.DeletedMetadata
}

// scopedPivot is what scope loading needs from a community pivot row. The
// entity-side foreign key is not on this interface because its accessor is
// named after the entity — scopedSpec.entityID supplies it.
type scopedPivot interface {
	proto.Message
	GetCommunityId() string
	GetArchived() bool
	GetDeleted() *models.DeletedMetadata
}

// scopedSpec is the entity-specific half of scope loading: prototypes for the
// two stored types, the fields to query them by, and how to read the entity ID
// off a pivot row. noun and plural appear only in wrapped error text.
type scopedSpec[T scopedEntity, P scopedPivot] struct {
	entity     T
	pivot      P
	ownerField string
	pivotField string
	pivotTable string
	noun       string
	plural     string
	entityID   func(P) string
}

// loadScoped returns the non-deleted items shared into the requested community
// scope, regardless of state — state bonuses are applied at the counting stage.
func loadScoped[T scopedEntity, P scopedPivot](
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	sharedSet map[string]struct{},
	spec scopedSpec[T, P],
) (map[string]T, error) {
	if opts.Mode == ModePerUser {
		return loadScopedForOwner(ctx, s, opts, sharedSet, spec)
	}
	return loadScopedForCommunities(ctx, s, opts, spec)
}

// loadScopedForOwner queries the owner's items directly, then pivots to keep
// only the ones shared into a community in scope.
func loadScopedForOwner[T scopedEntity, P scopedPivot](
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	sharedSet map[string]struct{},
	spec scopedSpec[T, P],
) (map[string]T, error) {
	ownedRaw, err := s.QueryByField(ctx, spec.ownerField, opts.OwnerID, spec.entity)
	if err != nil {
		return nil, fmt.Errorf("query %s by %s: %w", spec.plural, spec.ownerField, err)
	}

	byID := make(map[string]T, len(ownedRaw))
	ids := make([]string, 0, len(ownedRaw))
	for _, m := range ownedRaw {
		e, ok := m.(T)
		if !ok || !liveEntity(e) {
			continue
		}
		byID[e.GetId()] = e
		ids = append(ids, e.GetId())
	}
	if len(ids) == 0 {
		return nil, nil
	}

	pivotsRaw, err := s.QueryByFieldIn(ctx, spec.pivotField, ids, spec.pivot)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", spec.pivotTable, err)
	}
	scoped := make(map[string]T)
	for _, m := range pivotsRaw {
		p, ok := m.(P)
		if !ok || p.GetDeleted() != nil || p.GetArchived() {
			continue
		}
		if _, inScope := sharedSet[p.GetCommunityId()]; !inScope {
			continue
		}
		id := spec.entityID(p)
		if e, found := byID[id]; found {
			scoped[id] = e
		}
	}
	return scoped, nil
}

// loadScopedForCommunities pivots by community, then batch-fetches the items.
func loadScopedForCommunities[T scopedEntity, P scopedPivot](
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	spec scopedSpec[T, P],
) (map[string]T, error) {
	pivotsRaw, err := s.QueryByFieldIn(ctx, "community_id", opts.CommunityIDs, spec.pivot)
	if err != nil {
		return nil, fmt.Errorf("query %s by community: %w", spec.pivotTable, err)
	}

	idSet := make(map[string]struct{}, len(pivotsRaw))
	for _, m := range pivotsRaw {
		p, ok := m.(P)
		if !ok || p.GetDeleted() != nil {
			continue
		}
		// Note: the pivot's Archived flag flips when an experience completes or
		// is cancelled, and when a request is fulfilled or cancelled (see
		// `services/experience/lifecycle.go` and `services/request/lifecycle.go`).
		// We deliberately accept archived pivots here — a completed experience
		// or fulfilled request is the strongest signal a scope is "known for"
		// the category — and rely on the entity-state filter at the counting
		// stage to skip cancelled items instead.
		idSet[spec.entityID(p)] = struct{}{}
	}
	if len(idSet) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}

	byIDRaw, err := s.GetByIDs(ctx, ids, spec.entity)
	if err != nil {
		return nil, fmt.Errorf("get %s by ids: %w", spec.plural, err)
	}
	scoped := make(map[string]T, len(byIDRaw))
	for id, m := range byIDRaw {
		e, ok := m.(T)
		if !ok || !liveEntity(e) {
			continue
		}
		scoped[id] = e
	}
	return scoped, nil
}

// liveEntity reports whether a fetched item is usable and not soft-deleted.
// The empty-ID test stands in for the nil check the type parameter cannot
// express: a nil message pointer answers every generated getter with the zero
// value, so it fails this the same way a nil would.
func liveEntity(e scopedEntity) bool {
	return e.GetId() != "" && e.GetDeleted() == nil
}
