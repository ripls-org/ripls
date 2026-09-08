package available_now

import (
	"context"
	"fmt"
	"sort"
	"time"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// buildItem constructs an [api.Item] for an Available Now rail tile.
// Subtitle and media_id are populated only when non-empty so the
// optional-field semantics on the wire reflect actual presence.
func buildItem(
	id string,
	kind api.ItemKind,
	title, subtitle, miniLabel, mediaID string,
) *api.Item {
	item := &api.Item{
		ContextId: id,
		Kind:      kind,
		Title:     title,
	}
	if subtitle != "" {
		s := subtitle
		item.Subtitle = &s
	}
	if miniLabel != "" {
		l := miniLabel
		item.MiniLabel = &l
	}
	if mediaID != "" {
		m := mediaID
		item.MediaId = &m
	}
	return item
}

// Limit caps the number of items returned. The rail renders ~3 tiles
// in view at a time; 12 leaves room to scroll without overfetching.
const Limit = 12

// Mode selects between per-user and per-community gathering.
type Mode int

const (
	// ModePerUser collects items owned by [Options.TargetUserID] that
	// are also listed inside [Options.CommunityIDs] — the user profile
	// surface's view of "what is this person offering inside the
	// communities we share.".
	ModePerUser Mode = iota

	// ModePerCommunity collects items listed inside
	// [Options.CommunityIDs] regardless of owner — the workshop
	// surface's view of "what's currently available in this community
	// scope.".
	ModePerCommunity
)

// Options carries the parameters that vary by call site.
type Options struct {
	// Mode picks per-user vs per-community gathering. Required.
	Mode Mode

	// TargetUserID is the owner the per-user mode filters on. Required
	// when Mode == ModePerUser; ignored otherwise.
	TargetUserID string

	// CommunityIDs is the scope of communities considered. An empty
	// slice always returns an empty result regardless of Mode.
	CommunityIDs []string
}

// stampedItem pairs an item with its sort key. The mode-specific
// gather functions return uniformly-shaped slices so the orchestrator
// can merge across kinds with a single sort.
type stampedItem struct {
	item    *api.Item
	recency int64
}

// Gather returns the Available Now rail items for the requested
// scope. Each kind (gear / request / experience) is loaded in
// parallel-friendly serial fashion (one storage call per kind via
// QueryByFieldIn, followed by a batched entity fetch), then merged
// and ordered.
//
// Returns at most [Limit] items, ordered most-recent first (and
// soonest-first for upcoming experiences within their bucket).
// Returns an empty slice (never nil) when the scope is empty or no
// qualifying entity exists.
//
// Query budget: 6 reads when all three kinds are present in both
// modes. Tests should wrap calls in [storage.AssertMaxQueries] with
// max=6.
func Gather(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
) ([]*api.Item, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GatherAvailableNow",
		"community_id_count", len(opts.CommunityIDs),
		"mode", modeName(opts.Mode),
	)
	if len(opts.CommunityIDs) == 0 {
		return []*api.Item{}, nil
	}
	if opts.Mode == ModePerUser && opts.TargetUserID == "" {
		return nil, fmt.Errorf("gather available_now: per-user mode requires TargetUserID")
	}

	sharedSet := make(map[string]struct{}, len(opts.CommunityIDs))
	for _, id := range opts.CommunityIDs {
		sharedSet[id] = struct{}{}
	}

	gear, err := gatherGear(ctx, s, opts, sharedSet)
	if err != nil {
		return nil, fmt.Errorf("gear: %w", err)
	}
	requests, err := gatherRequests(ctx, s, opts, sharedSet)
	if err != nil {
		return nil, fmt.Errorf("requests: %w", err)
	}
	experiences, err := gatherExperiences(ctx, s, opts, sharedSet)
	if err != nil {
		return nil, fmt.Errorf("experiences: %w", err)
	}

	all := make([]stampedItem, 0, len(gear)+len(requests)+len(experiences))
	all = append(all, gear...)
	all = append(all, requests...)
	all = append(all, experiences...)
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].recency > all[j].recency
	})
	if len(all) > Limit {
		all = all[:Limit]
	}

	items := make([]*api.Item, len(all))
	for i, st := range all {
		items[i] = st.item
	}
	logger.DebugContext(ctx, "available_now: assembled",
		"items_returned", len(items),
	)
	return items, nil
}

func modeName(m Mode) string {
	if m == ModePerUser {
		return "per_user"
	}
	return "per_community"
}

// gatherGear loads the qualifying gear list for the requested mode
// and marks the giveaway-vs-borrow mini-label based on the gear's
// listing inside the scoped communities.
func gatherGear(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	sharedSet map[string]struct{},
) ([]stampedItem, error) {
	gearByID, gearIDs, err := loadGear(ctx, s, opts)
	if err != nil {
		return nil, err
	}
	if len(gearIDs) == 0 {
		return nil, nil
	}

	communityGears, err := storage.QueryByFieldIn[*models.CommunityGear](
		s, ctx, "gear_id", gearIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("query community_gear: %w", err)
	}

	type gearScope struct {
		giveaway bool
		inScope  bool
	}
	scopes := make(map[string]*gearScope, len(gearByID))
	for _, cg := range communityGears {
		if cg.Deleted != nil {
			continue
		}
		if _, ok := sharedSet[cg.CommunityId]; !ok {
			continue
		}
		sc := scopes[cg.GearId]
		if sc == nil {
			sc = &gearScope{}
			scopes[cg.GearId] = sc
		}
		sc.inScope = true
		if cg.Availability == models.Availability_AVAILABILITY_FOR_GIVEAWAY {
			sc.giveaway = true
		}
	}

	out := make([]stampedItem, 0, len(scopes))
	for gearID, sc := range scopes {
		if !sc.inScope {
			continue
		}
		g, ok := gearByID[gearID]
		if !ok {
			continue
		}
		kind := api.ItemKind_ITEM_KIND_GEAR
		miniLabel := "BORROW"
		if sc.giveaway {
			kind = api.ItemKind_ITEM_KIND_GIVEAWAY
			miniLabel = "GIVEAWAY"
		}
		mediaID := ""
		if len(g.MediaIds) > 0 {
			mediaID = g.MediaIds[0]
		}
		item := buildItem(g.Id, kind, g.Name, "", miniLabel, mediaID)
		out = append(out, stampedItem{item: item, recency: g.CreatedAtUnixSec})
	}
	return out, nil
}

// loadGear returns the gear-by-id map and gear-id slice the gear
// gather flow needs, with the mode-appropriate entity-level query.
func loadGear(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
) (map[string]*models.Gear, []string, error) {
	if opts.Mode == ModePerUser {
		ownedGear, err := storage.QueryByField[*models.Gear](s, ctx, "owner_id", opts.TargetUserID)
		if err != nil {
			return nil, nil, fmt.Errorf("query gear by owner: %w", err)
		}
		available := make(map[string]*models.Gear, len(ownedGear))
		ids := make([]string, 0, len(ownedGear))
		for _, g := range ownedGear {
			if !gearQualifies(g) {
				continue
			}
			available[g.Id] = g
			ids = append(ids, g.Id)
		}
		return available, ids, nil
	}

	// ModePerCommunity: start from community_gear pivots, batch-load gear.
	pivots, err := storage.QueryByFieldIn[*models.CommunityGear](
		s, ctx, "community_id", opts.CommunityIDs,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("query community_gear by community: %w", err)
	}
	gearIDSet := make(map[string]struct{}, len(pivots))
	for _, cg := range pivots {
		if cg.Deleted != nil {
			continue
		}
		gearIDSet[cg.GearId] = struct{}{}
	}
	if len(gearIDSet) == 0 {
		return nil, nil, nil
	}
	gearIDs := make([]string, 0, len(gearIDSet))
	for id := range gearIDSet {
		gearIDs = append(gearIDs, id)
	}
	gearByID, err := storage.GetByIDs[*models.Gear](s, ctx, gearIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("get gear by ids: %w", err)
	}
	available := make(map[string]*models.Gear, len(gearByID))
	ids := make([]string, 0, len(gearByID))
	for id, g := range gearByID {
		if !gearQualifies(g) {
			continue
		}
		available[id] = g
		ids = append(ids, id)
	}
	return available, ids, nil
}

func gearQualifies(g *models.Gear) bool {
	if g == nil || g.Deleted != nil {
		return false
	}
	return g.State == models.GearState_GEAR_STATE_AVAILABLE
}

// gatherRequests loads the qualifying request list and the
// shared-at timestamps it sorts on.
func gatherRequests(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	sharedSet map[string]struct{},
) ([]stampedItem, error) {
	open, requestIDs, err := loadRequests(ctx, s, opts)
	if err != nil {
		return nil, err
	}
	if len(requestIDs) == 0 {
		return nil, nil
	}

	communityRequests, err := storage.QueryByFieldIn[*models.CommunityRequest](
		s, ctx, "request_id", requestIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("query community_request: %w", err)
	}

	scoped := make(map[string]int64, len(open))
	for _, cr := range communityRequests {
		if cr.Deleted != nil || cr.Archived {
			continue
		}
		if _, ok := sharedSet[cr.CommunityId]; !ok {
			continue
		}
		if existing, ok := scoped[cr.RequestId]; !ok || cr.SharedAtUnixSec > existing {
			scoped[cr.RequestId] = cr.SharedAtUnixSec
		}
	}

	out := make([]stampedItem, 0, len(scoped))
	for reqID, sharedAt := range scoped {
		r, ok := open[reqID]
		if !ok {
			continue
		}
		mediaID := ""
		if len(r.MediaIds) > 0 {
			mediaID = r.MediaIds[0]
		}
		item := buildItem(
			r.Id,
			api.ItemKind_ITEM_KIND_REQUEST,
			r.Title,
			"",
			"REQUEST",
			mediaID,
		)
		out = append(out, stampedItem{item: item, recency: sharedAt})
	}
	return out, nil
}

func loadRequests(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
) (map[string]*models.Request, []string, error) {
	if opts.Mode == ModePerUser {
		owned, err := storage.QueryByField[*models.Request](s, ctx, "requester_id", opts.TargetUserID)
		if err != nil {
			return nil, nil, fmt.Errorf("query requests by requester: %w", err)
		}
		open := make(map[string]*models.Request, len(owned))
		ids := make([]string, 0, len(owned))
		for _, r := range owned {
			if !requestQualifies(r) {
				continue
			}
			open[r.Id] = r
			ids = append(ids, r.Id)
		}
		return open, ids, nil
	}

	// ModePerCommunity
	pivots, err := storage.QueryByFieldIn[*models.CommunityRequest](
		s, ctx, "community_id", opts.CommunityIDs,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("query community_request by community: %w", err)
	}
	reqIDSet := make(map[string]struct{}, len(pivots))
	for _, cr := range pivots {
		if cr.Deleted != nil || cr.Archived {
			continue
		}
		reqIDSet[cr.RequestId] = struct{}{}
	}
	if len(reqIDSet) == 0 {
		return nil, nil, nil
	}
	ids := make([]string, 0, len(reqIDSet))
	for id := range reqIDSet {
		ids = append(ids, id)
	}
	reqByID, err := storage.GetByIDs[*models.Request](s, ctx, ids)
	if err != nil {
		return nil, nil, fmt.Errorf("get requests by ids: %w", err)
	}
	open := make(map[string]*models.Request, len(reqByID))
	openIDs := make([]string, 0, len(reqByID))
	for id, r := range reqByID {
		if !requestQualifies(r) {
			continue
		}
		open[id] = r
		openIDs = append(openIDs, id)
	}
	return open, openIDs, nil
}

func requestQualifies(r *models.Request) bool {
	if r == nil || r.Deleted != nil {
		return false
	}
	return r.State == models.RequestState_REQUEST_STATE_ACTIVE ||
		r.State == models.RequestState_REQUEST_STATE_OFFERS_RECEIVED
}

// gatherExperiences loads upcoming experiences in scope. Recency is
// inverted so soonest-upcoming surfaces first within the bucket.
func gatherExperiences(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	sharedSet map[string]struct{},
) ([]stampedItem, error) {
	now := time.Now().Unix()
	upcoming, experienceIDs, err := loadExperiences(ctx, s, opts, now)
	if err != nil {
		return nil, err
	}
	if len(experienceIDs) == 0 {
		return nil, nil
	}

	communityExperiences, err := storage.QueryByFieldIn[*models.CommunityExperience](
		s, ctx, "experience_id", experienceIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("query community_experience: %w", err)
	}
	scoped := make(map[string]struct{}, len(upcoming))
	for _, ce := range communityExperiences {
		if ce.Deleted != nil || ce.Archived {
			continue
		}
		if _, ok := sharedSet[ce.CommunityId]; !ok {
			continue
		}
		scoped[ce.ExperienceId] = struct{}{}
	}

	out := make([]stampedItem, 0, len(scoped))
	for expID := range scoped {
		e, ok := upcoming[expID]
		if !ok {
			continue
		}
		start := ExperienceStartUnixSec(e)
		mediaID := ""
		if len(e.MediaIds) > 0 {
			mediaID = e.MediaIds[0]
		}
		item := buildItem(
			e.Id,
			api.ItemKind_ITEM_KIND_EXPERIENCE,
			e.Name,
			"",
			"UPCOMING",
			mediaID,
		)
		// Surface soonest-upcoming first within experiences; interleave
		// reasonably against gear/request dates by inverting the
		// distance-from-now.
		out = append(out, stampedItem{
			item:    item,
			recency: now*2 - start,
		})
	}
	return out, nil
}

func loadExperiences(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	now int64,
) (map[string]*models.Experience, []string, error) {
	if opts.Mode == ModePerUser {
		owned, err := storage.QueryByField[*models.Experience](s, ctx, "owner_id", opts.TargetUserID)
		if err != nil {
			return nil, nil, fmt.Errorf("query experiences by owner: %w", err)
		}
		upcoming := make(map[string]*models.Experience, len(owned))
		ids := make([]string, 0, len(owned))
		for _, e := range owned {
			if !experienceQualifies(e, now) {
				continue
			}
			upcoming[e.Id] = e
			ids = append(ids, e.Id)
		}
		return upcoming, ids, nil
	}

	// ModePerCommunity
	pivots, err := storage.QueryByFieldIn[*models.CommunityExperience](
		s, ctx, "community_id", opts.CommunityIDs,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("query community_experience by community: %w", err)
	}
	expIDSet := make(map[string]struct{}, len(pivots))
	for _, ce := range pivots {
		if ce.Deleted != nil || ce.Archived {
			continue
		}
		expIDSet[ce.ExperienceId] = struct{}{}
	}
	if len(expIDSet) == 0 {
		return nil, nil, nil
	}
	ids := make([]string, 0, len(expIDSet))
	for id := range expIDSet {
		ids = append(ids, id)
	}
	expByID, err := storage.GetByIDs[*models.Experience](s, ctx, ids)
	if err != nil {
		return nil, nil, fmt.Errorf("get experiences by ids: %w", err)
	}
	upcoming := make(map[string]*models.Experience, len(expByID))
	openIDs := make([]string, 0, len(expByID))
	for id, e := range expByID {
		if !experienceQualifies(e, now) {
			continue
		}
		upcoming[id] = e
		openIDs = append(openIDs, id)
	}
	return upcoming, openIDs, nil
}

func experienceQualifies(e *models.Experience, now int64) bool {
	if e == nil || e.Deleted != nil {
		return false
	}
	if e.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE &&
		e.State != models.ExperienceState_EXPERIENCE_STATE_JOINED &&
		e.State != models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS {
		return false
	}
	start := ExperienceStartUnixSec(e)
	return start != 0 && start > now
}

// ExperienceStartUnixSec extracts the start time from an Experience's
// oneof [time_type]. Returns 0 for TimeTBD or when no time is set,
// which causes the experience to be excluded from time-ordered
// surfaces (the rail, the profile presence sheet).
func ExperienceStartUnixSec(e *models.Experience) int64 {
	if e == nil || e.Time == nil {
		return 0
	}
	switch t := e.Time.TimeType.(type) {
	case *models.ExperienceTime_Specific:
		if t.Specific != nil {
			return t.Specific.UnixTimestampSec
		}
	case *models.ExperienceTime_Range:
		if t.Range != nil {
			return t.Range.StartUnixSec
		}
	}
	return 0
}
