package impact_metrics

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// CommunityDataset bundles the source-of-truth rows the impact-metrics
// calculators need for one community. Loading it once and threading the
// pointer through every Calculate* / Generate* call eliminates the
// per-function fan-out that today queries the same tables 2–4 times in
// a single GetCommunityImpactMetrics request — see #2055.
//
// All slices and maps are preloaded with QueryOptions{IncludeDeleted: true};
// consumers retain their existing soft-delete filters so behaviour is
// preserved.
type CommunityDataset struct {
	CommunityID string

	// CommunityGear is every community_gear row for the community.
	CommunityGear []*models.CommunityGear

	// Gear maps gear_id → *Gear for every gear referenced by CommunityGear
	// or by Transfer rows that need fallback impact computation.
	Gear map[string]*models.Gear

	// Transfers is every transfer row for the community (any state).
	Transfers []*models.Transfer

	// CommunityRequests is every community_request row for the community.
	CommunityRequests []*models.CommunityRequest

	// Requests maps request_id → *Request for every request referenced by
	// CommunityRequests.
	Requests map[string]*models.Request

	// CommunityExperiences is every community_experience row for the community.
	CommunityExperiences []*models.CommunityExperience

	// Experiences maps experience_id → *Experience for every experience
	// referenced by CommunityExperiences.
	Experiences map[string]*models.Experience

	// CommunityUsers is every community_user (membership) row.
	CommunityUsers []*models.CommunityUser
}

// LoadCommunityDataset issues exactly one QueryByField per source table
// plus the batched GetByIDs needed to hydrate gear/request/experience
// rows referenced by the junction tables. Eight queries total.
//
// Consumers (CalculateActivityCounts, CalculateImpactSavings, etc.)
// previously each issued their own QueryByField for the same tables —
// LoadCommunityDataset replaces all of that with one preload at the top
// of the handler.
func LoadCommunityDataset(ctx context.Context, store *storage.ProtoSQLStorage, communityID string) (*CommunityDataset, error) {
	ds := &CommunityDataset{CommunityID: communityID}

	cg, err := storage.QueryByField[*models.CommunityGear](store, ctx, "community_id", communityID)
	if err != nil {
		return nil, fmt.Errorf("query community_gear: %w", err)
	}
	ds.CommunityGear = cg

	transfers, err := storage.QueryByField[*models.Transfer](store, ctx, "community_id", communityID)
	if err != nil {
		return nil, fmt.Errorf("query transfers: %w", err)
	}
	ds.Transfers = transfers

	cr, err := storage.QueryByField[*models.CommunityRequest](store, ctx, "community_id", communityID)
	if err != nil {
		return nil, fmt.Errorf("query community_request: %w", err)
	}
	ds.CommunityRequests = cr

	ce, err := storage.QueryByField[*models.CommunityExperience](store, ctx, "community_id", communityID)
	if err != nil {
		return nil, fmt.Errorf("query community_experience: %w", err)
	}
	ds.CommunityExperiences = ce

	cu, err := storage.QueryByField[*models.CommunityUser](store, ctx, "community_id", communityID)
	if err != nil {
		return nil, fmt.Errorf("query community_user: %w", err)
	}
	ds.CommunityUsers = cu

	// Collect every gear id we might need: gear in the library plus the
	// gear referenced by transfers that lack an inline ImpactEstimate
	// (the fallback path computes from the gear row).
	gearIDSet := make(map[string]struct{}, len(cg)+len(transfers))
	for _, g := range cg {
		gearIDSet[g.GearId] = struct{}{}
	}
	for _, t := range transfers {
		if t.ImpactEstimate == nil && t.GearId != "" {
			gearIDSet[t.GearId] = struct{}{}
		}
	}
	gearIDs := make([]string, 0, len(gearIDSet))
	for id := range gearIDSet {
		gearIDs = append(gearIDs, id)
	}
	gearMap, err := storage.GetByIDs[*models.Gear](store, ctx, gearIDs, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		return nil, fmt.Errorf("batch fetch gear: %w", err)
	}
	ds.Gear = gearMap

	requestIDs := storage.CollectField(cr, func(cr *models.CommunityRequest) string { return cr.RequestId })
	requestMap, err := storage.GetByIDs[*models.Request](store, ctx, requestIDs, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		return nil, fmt.Errorf("batch fetch requests: %w", err)
	}
	ds.Requests = requestMap

	experienceIDs := storage.CollectField(ce, func(ce *models.CommunityExperience) string { return ce.ExperienceId })
	experienceMap, err := storage.GetByIDs[*models.Experience](store, ctx, experienceIDs, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		return nil, fmt.Errorf("batch fetch experiences: %w", err)
	}
	ds.Experiences = experienceMap

	return ds, nil
}

// gearMapCtxKey carries a request-scoped gear map through context so
// transferImpact (and other helpers that need gear-row data) can pick
// up the preloaded map without changing every callsite signature.
//
// The slow path remains a real DB lookup when no map is attached —
// callers that don't preload a dataset (one-off invocations, tests)
// keep working unchanged.
type gearMapCtxKey struct{}

// datasetCtxKey carries the request-scoped *CommunityDataset so deep
// helpers can pull preloaded junction tables (CommunityGear,
// CommunityUser, …) from context without changing every callsite
// signature. Helpers fall back to QueryByField when no dataset is
// attached (one-off callers, tests).
type datasetCtxKey struct{}

// WithDatasetGearMap attaches ds.Gear to ctx so downstream helpers
// (transferImpact, EntityCache cache misses) can short-circuit to a
// map hit instead of issuing per-row GetByID.
func WithDatasetGearMap(ctx context.Context, ds *CommunityDataset) context.Context {
	if ds == nil || ds.Gear == nil {
		return ctx
	}
	return context.WithValue(ctx, gearMapCtxKey{}, ds.Gear)
}

// GearFromContext returns the preloaded gear and true when the gear id
// is present in the request-scoped gear map. Returns (nil, false)
// when no map is attached or the id is unknown.
func GearFromContext(ctx context.Context, gearID string) (*models.Gear, bool) {
	m, _ := ctx.Value(gearMapCtxKey{}).(map[string]*models.Gear)
	if m == nil {
		return nil, false
	}
	g, ok := m[gearID]
	return g, ok
}

// WithCommunityDataset attaches the full dataset to ctx so helpers
// nested inside ComputeMetricDetailFromDataset can pull preloaded
// CommunityGear / CommunityUser slices instead of issuing their own
// QueryByField. See #2055.
func WithCommunityDataset(ctx context.Context, ds *CommunityDataset) context.Context {
	if ds == nil {
		return ctx
	}
	return context.WithValue(ctx, datasetCtxKey{}, ds)
}

// DatasetFromContext returns the request-scoped *CommunityDataset and
// true when one is attached. Helpers that issue per-community
// QueryByField against junction tables should consult this first and
// only fall back to the DB when no dataset is attached or it covers a
// different community (cross-community paths still hit the DB).
func DatasetFromContext(ctx context.Context) (*CommunityDataset, bool) {
	ds, ok := ctx.Value(datasetCtxKey{}).(*CommunityDataset)
	return ds, ok && ds != nil
}

// resolveCommunityGear returns the active CommunityGear rows for the
// given community. When the request-scoped dataset is attached to ctx
// and covers the requested community, the rows come from
// ds.CommunityGear (in-memory); otherwise the function falls back to
// a single QueryByField. Used by helpers that previously issued their
// own per-community QueryByField against CommunityGear. See #2055.
func resolveCommunityGear(ctx context.Context, store *storage.ProtoSQLStorage, communityID string) ([]*models.CommunityGear, error) {
	if ds, ok := DatasetFromContext(ctx); ok && ds.CommunityID == communityID {
		return ds.CommunityGear, nil
	}
	return storage.QueryByField[*models.CommunityGear](store, ctx, "community_id", communityID)
}

// resolveCommunityUsers returns the active CommunityUser rows for the
// given community. Same dataset-first pattern as resolveCommunityGear.
func resolveCommunityUsers(ctx context.Context, store *storage.ProtoSQLStorage, communityID string) ([]*models.CommunityUser, error) {
	if ds, ok := DatasetFromContext(ctx); ok && ds.CommunityID == communityID {
		return ds.CommunityUsers, nil
	}
	return storage.QueryByField[*models.CommunityUser](store, ctx, "community_id", communityID)
}

// FilterCompletedTransfers returns the subset of ds.Transfers in state
// COMPLETED and not soft-deleted. Pure compute; replaces the previous
// MetricDetailCalculator.loadCompletedTransfers query.
func FilterCompletedTransfers(transfers []*models.Transfer) []*models.Transfer {
	out := make([]*models.Transfer, 0, len(transfers))
	for _, t := range transfers {
		if t.Deleted != nil {
			continue
		}
		if t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		out = append(out, t)
	}
	return out
}

// FilterFulfilledRequests walks ds.CommunityRequests, looks each row's
// Request up in ds.Requests, and returns those in state FULFILLED and
// not soft-deleted. Pure compute; replaces the previous per-row
// GetByID loop in MetricDetailCalculator.loadFulfilledRequests.
func FilterFulfilledRequests(ds *CommunityDataset) []*models.Request {
	out := make([]*models.Request, 0, len(ds.CommunityRequests))
	for _, cr := range ds.CommunityRequests {
		r, ok := ds.Requests[cr.RequestId]
		if !ok || r.Deleted != nil {
			continue
		}
		if r.State != models.RequestState_REQUEST_STATE_FULFILLED {
			continue
		}
		out = append(out, r)
	}
	return out
}

// FilterCompletedExperiences walks ds.CommunityExperiences, looks each
// row's Experience up in ds.Experiences, and returns those in state
// COMPLETED and not soft-deleted. Pure compute; replaces the previous
// per-row GetByID loop in MetricDetailCalculator.loadCompletedExperiences.
func FilterCompletedExperiences(ds *CommunityDataset) []*models.Experience {
	out := make([]*models.Experience, 0, len(ds.CommunityExperiences))
	for _, ce := range ds.CommunityExperiences {
		e, ok := ds.Experiences[ce.ExperienceId]
		if !ok || e.Deleted != nil {
			continue
		}
		if e.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			continue
		}
		out = append(out, e)
	}
	return out
}

// LoadAllCommunitiesDatasets returns one CommunityDataset per non-
// deleted community on the platform. Issues exactly one ListAll per
// source table (no community filter), then partitions in memory by
// community_id. Total query count is constant in the number of
// communities — the previous per-community loop in computePercentiles
// was 6×N; this is the batched replacement.
func LoadAllCommunitiesDatasets(ctx context.Context, store *storage.ProtoSQLStorage) (map[string]*CommunityDataset, error) {
	communityMsgs, err := store.ListAll(ctx, &models.Community{})
	if err != nil {
		return nil, fmt.Errorf("list communities: %w", err)
	}
	datasets := make(map[string]*CommunityDataset, len(communityMsgs))
	for _, m := range communityMsgs {
		c := m.(*models.Community)
		if c.Deleted != nil {
			continue
		}
		datasets[c.Id] = &CommunityDataset{CommunityID: c.Id}
	}

	// One ListAll per source table; partition in memory by community_id.
	transferMsgs, err := store.ListAll(ctx, &models.Transfer{})
	if err != nil {
		return nil, fmt.Errorf("list transfers: %w", err)
	}
	transfers := make([]*models.Transfer, 0, len(transferMsgs))
	for _, m := range transferMsgs {
		t := m.(*models.Transfer)
		if t.Deleted != nil {
			continue
		}
		transfers = append(transfers, t)
		if ds, ok := datasets[t.CommunityId]; ok {
			ds.Transfers = append(ds.Transfers, t)
		}
	}

	cgMsgs, err := store.ListAll(ctx, &models.CommunityGear{})
	if err != nil {
		return nil, fmt.Errorf("list community_gear: %w", err)
	}
	communityGear := make([]*models.CommunityGear, 0, len(cgMsgs))
	for _, m := range cgMsgs {
		cg := m.(*models.CommunityGear)
		if cg.Deleted != nil {
			continue
		}
		communityGear = append(communityGear, cg)
		if ds, ok := datasets[cg.CommunityId]; ok {
			ds.CommunityGear = append(ds.CommunityGear, cg)
		}
	}

	crMsgs, err := store.ListAll(ctx, &models.CommunityRequest{})
	if err != nil {
		return nil, fmt.Errorf("list community_request: %w", err)
	}
	communityRequests := make([]*models.CommunityRequest, 0, len(crMsgs))
	for _, m := range crMsgs {
		cr := m.(*models.CommunityRequest)
		if cr.Deleted != nil {
			continue
		}
		communityRequests = append(communityRequests, cr)
		if ds, ok := datasets[cr.CommunityId]; ok {
			ds.CommunityRequests = append(ds.CommunityRequests, cr)
		}
	}

	ceMsgs, err := store.ListAll(ctx, &models.CommunityExperience{})
	if err != nil {
		return nil, fmt.Errorf("list community_experience: %w", err)
	}
	communityExperiences := make([]*models.CommunityExperience, 0, len(ceMsgs))
	for _, m := range ceMsgs {
		ce := m.(*models.CommunityExperience)
		if ce.Deleted != nil {
			continue
		}
		communityExperiences = append(communityExperiences, ce)
		if ds, ok := datasets[ce.CommunityId]; ok {
			ds.CommunityExperiences = append(ds.CommunityExperiences, ce)
		}
	}

	cuMsgs, err := store.ListAll(ctx, &models.CommunityUser{})
	if err != nil {
		return nil, fmt.Errorf("list community_user: %w", err)
	}
	for _, m := range cuMsgs {
		cu := m.(*models.CommunityUser)
		if cu.Deleted != nil {
			continue
		}
		if ds, ok := datasets[cu.CommunityId]; ok {
			ds.CommunityUsers = append(ds.CommunityUsers, cu)
		}
	}

	// Hydrate gear/request/experience maps once each, globally. Each
	// dataset's map references the same underlying objects — fine
	// because consumers only read.
	gearIDSet := make(map[string]struct{})
	for _, cg := range communityGear {
		gearIDSet[cg.GearId] = struct{}{}
	}
	for _, t := range transfers {
		if t.ImpactEstimate == nil && t.GearId != "" {
			gearIDSet[t.GearId] = struct{}{}
		}
	}
	gearIDs := make([]string, 0, len(gearIDSet))
	for id := range gearIDSet {
		gearIDs = append(gearIDs, id)
	}
	gearMap, err := storage.GetByIDs[*models.Gear](store, ctx, gearIDs, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		return nil, fmt.Errorf("batch fetch gear: %w", err)
	}

	requestIDs := storage.CollectField(communityRequests, func(cr *models.CommunityRequest) string { return cr.RequestId })
	requestMap, err := storage.GetByIDs[*models.Request](store, ctx, requestIDs, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		return nil, fmt.Errorf("batch fetch requests: %w", err)
	}

	experienceIDs := storage.CollectField(communityExperiences, func(ce *models.CommunityExperience) string { return ce.ExperienceId })
	experienceMap, err := storage.GetByIDs[*models.Experience](store, ctx, experienceIDs, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		return nil, fmt.Errorf("batch fetch experiences: %w", err)
	}

	// Attach maps per dataset (shared read-only).
	for _, ds := range datasets {
		ds.Gear = gearMap
		ds.Requests = requestMap
		ds.Experiences = experienceMap
	}

	return datasets, nil
}
