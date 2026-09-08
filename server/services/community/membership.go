package community

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// ListCommunityUsers lists users in a community.
func (s *Service) ListCommunityUsers(
	ctx context.Context,
	req *connect.Request[api.ListCommunityUsersRequest],
) (*connect.Response[api.ListCommunityUsersResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_id", req.Msg.CommunityId,
	)

	logger.DebugContext(ctx, "listing users for community")

	// Single round-trip: fetch community + verify membership + reject deleted.
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Get all memberships for this community
	memberships, err := s.storage.QueryByField(ctx, "community_id", req.Msg.CommunityId, &models.CommunityUser{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community memberships", "error", err)
		return nil, connecterr.Internal(ctx, "ListCommunityUsers", err)
	}

	// Build a map of user ID → join timestamp and collect IDs for batch fetch.
	userIDs := make([]string, 0, len(memberships))
	joinedAt := make(map[string]int64, len(memberships))
	for _, msg := range memberships {
		membership := msg.(*models.CommunityUser)
		userIDs = append(userIDs, membership.UserId)
		joinedAt[membership.UserId] = membership.CreatedAtUnixSec
	}

	items, err := services.FetchAPIUsers(ctx, s.storage, userIDs)
	if err != nil {
		logger.ErrorContext(ctx, "failed to fetch users", "error", err)
		return nil, connecterr.Internal(ctx, "ListCommunityUsers", err)
	}

	members := make([]*api.CommunityMember, 0, len(items))
	for _, u := range items {
		members = append(members, &api.CommunityMember{
			User:            u,
			JoinedAtUnixSec: joinedAt[u.Id],
		})
	}

	return connect.NewResponse(&api.ListCommunityUsersResponse{
		Members: members,
	}), nil
}

// SearchCommunityUsers searches community members by display name using ILIKE fuzzy matching.
// Returns up to req.Msg.Limit results (default 10, max 50).
// Used to resolve AI-extracted participant names to real user accounts.
func (s *Service) SearchCommunityUsers(
	ctx context.Context,
	req *connect.Request[api.SearchCommunityUsersRequest],
) (*connect.Response[api.SearchCommunityUsersResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"community_id", req.Msg.CommunityId,
		"query", req.Msg.Query,
	)

	// Verify caller is a member of an active (non-deleted) community.
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	limit := int(req.Msg.Limit)
	storedUsers, err := s.storage.SearchCommunityUsersByName(ctx, req.Msg.CommunityId, req.Msg.Query, limit)
	if err != nil {
		logger.ErrorContext(ctx, "failed to search community users", "error", err)
		return nil, connecterr.Internal(ctx, "SearchCommunityUsers", err)
	}

	apiUsers := make([]*api.User, 0, len(storedUsers))
	for _, u := range storedUsers {
		apiUsers = append(apiUsers, services.ToAPIUser(u))
	}

	logger.DebugContext(ctx, "searched community users", "results", len(apiUsers))

	return connect.NewResponse(&api.SearchCommunityUsersResponse{
		Users: apiUsers,
	}), nil
}

// ListCommunities lists communities the user is a member of, optionally filtered by region.
func (s *Service) ListCommunities(
	ctx context.Context,
	req *connect.Request[api.ListCommunitiesRequest],
) (*connect.Response[api.ListCommunitiesResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	// Add region filter to logs if present
	if req.Msg.RegionFilter != nil {
		logger = logger.With(
			"region_id", req.Msg.RegionFilter.RegionId,
		)
	}

	logger.DebugContext(ctx, "listing communities")

	// Get communities where user is a member
	memberships, err := s.storage.QueryByField(ctx, "user_id", authInfo.UserID, &models.CommunityUser{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query user memberships", "error", err)
		return nil, connecterr.Internal(ctx, "ListCommunities", err)
	}

	// Collect community IDs for a single batch fetch.
	communityIDs := make([]string, len(memberships))
	for i, msg := range memberships {
		communityIDs[i] = msg.(*models.CommunityUser).CommunityId
	}

	communityMap, err := storage.GetByIDs[*models.Community](s.storage, ctx, communityIDs)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get communities", "error", err)
		return nil, connecterr.Internal(ctx, "ListCommunities", err)
	}

	// Batch-load the members of every community the user belongs to (one query,
	// soft-deleted excluded). Used for the member count and — on ad-hoc
	// (nameless) communities — to (a) suppress host-only ones and (b) render
	// the rest like a group chat from the first few other members' first names.
	memberRows, err := s.storage.QueryByFieldIn(ctx, "community_id", communityIDs, &models.CommunityUser{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch-load community members", "error", err)
		return nil, connecterr.Internal(ctx, "ListCommunities", err)
	}
	memberIDsByCommunity := make(map[string][]string, len(communityMap))
	for _, m := range memberRows {
		cu := m.(*models.CommunityUser)
		memberIDsByCommunity[cu.CommunityId] = append(memberIDsByCommunity[cu.CommunityId], cu.UserId)
	}

	// Only nameless communities need a member-name preview (named ones show
	// their name). Collect the other members' ids (excluding the caller) so
	// display names load in a single batch.
	previewUserIDSet := make(map[string]struct{})
	for cid, community := range communityMap {
		if community.Name != "" {
			continue
		}
		for _, uid := range memberIDsByCommunity[cid] {
			if uid != authInfo.UserID {
				previewUserIDSet[uid] = struct{}{}
			}
		}
	}
	previewUserIDs := make([]string, 0, len(previewUserIDSet))
	for uid := range previewUserIDSet {
		previewUserIDs = append(previewUserIDs, uid)
	}
	previewUsers, err := storage.GetByIDs[*models.User](s.storage, ctx, previewUserIDs)
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch-load member preview users", "error", err)
		return nil, connecterr.Internal(ctx, "ListCommunities", err)
	}

	// Resolve the spawning item's name for nameless ad-hoc communities so the
	// client can label them "Group from {item}" (#2492). Best-effort: a
	// resolution failure leaves the name empty rather than failing the list.
	originItemNames := s.resolveOriginItemNames(ctx, logger, communityMap)

	// Per-community shared-gear count and area anchor (centroid of the
	// geocoded gear) for the Library location sheet (#2634). Three batched
	// queries regardless of community count.
	gearStats, err := s.loadCommunityGearStats(ctx, communityIDs)
	if err != nil {
		logger.ErrorContext(ctx, "failed to load community gear stats", "error", err)
		return nil, connecterr.Internal(ctx, "ListCommunities", err)
	}

	// Build response, applying optional region filter.
	items := make([]*api.CommunityItem, 0, len(communityMap))
	for _, msg := range memberships {
		membership := msg.(*models.CommunityUser)
		community, ok := communityMap[membership.CommunityId]
		if !ok {
			logger.WarnContext(ctx, "community not found in batch result", "community_id", membership.CommunityId)
			continue
		}

		if req.Msg.RegionFilter != nil {
			matchesRegion, err := s.communityMatchesRegion(ctx, community.Id, req.Msg.RegionFilter)
			if err != nil {
				logger.ErrorContext(ctx, "failed to check region filter", "community_id", community.Id, "error", err)
				continue
			}
			if !matchesRegion {
				continue
			}
		}

		memberCount := len(memberIDsByCommunity[community.Id])

		// Suppress host-only ad-hoc communities: a nameless community whose
		// only member is the owner is a per-item community with no audience —
		// pure clutter. It reappears, rendered as a group chat, once anyone
		// else joins (#2492).
		if community.Name == "" && memberCount <= 1 {
			continue
		}

		var previewFirstNames []string
		if community.Name == "" {
			previewFirstNames = firstFewMemberFirstNames(
				memberIDsByCommunity[community.Id], authInfo.UserID, previewUsers, memberPreviewLimit,
			)
		}

		logger.DebugContext(ctx, "list communities item",
			"operation", "ListCommunities",
			"community_id", community.Id,
			"media_ids", community.MediaIds,
		)

		item := &api.CommunityItem{
			Id:                      community.Id,
			Name:                    community.Name,
			Description:             community.Description,
			MediaIds:                community.MediaIds,
			OwnerUserId:             community.OwnerUserId,
			MemberCount:             int32(memberCount),
			MemberPreviewFirstNames: previewFirstNames,
			OriginItemName:          originItemNames[community.Id],
		}
		if st := gearStats[community.Id]; st != nil {
			item.GearCount = st.gearCount
			if st.hasArea {
				item.AreaLatitudeDeg = &st.latitudeDeg
				item.AreaLongitudeDeg = &st.longitudeDeg
			}
		}
		items = append(items, item)
	}

	if err := s.sortCommunitiesByActivity(ctx, items, logger); err != nil {
		// Don't fail the whole request on a sort failure — the caller
		// still benefits from the unsorted list. Log and continue.
		logger.WarnContext(ctx, "failed to sort communities by activity", "error", err)
	}

	// Nameless (ad-hoc / per-item) communities sort *after* every named
	// community, regardless of activity. They read as second-class "needs a
	// name" entries in the Workshop carousel, where the owner can promote one
	// to a real community by giving it a name + photo + description (#2492).
	// SliceStable keeps the activity order within each group intact.
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Name != "" && items[j].Name == ""
	})

	return connect.NewResponse(&api.ListCommunitiesResponse{
		Communities: items,
	}), nil
}

// resolveOriginItemNames delegates to the shared resolver in
// server/community (communitylib.ResolveOriginItemNames), which also
// serves non-service consumers like the ops activity digest. Kept as a
// thin method so call sites in this service read locally.
func (s *Service) resolveOriginItemNames(
	ctx context.Context,
	_ *logging.Logger,
	communityMap map[string]*models.Community,
) map[string]string {
	return communitylib.ResolveOriginItemNames(ctx, s.storage, communityMap)
}

// communityGearStats aggregates, per community, the shared-gear count and —
// when hasArea is true — the centroid of the geocoded gear used as the
// community's area anchor on the API (#2634).
type communityGearStats struct {
	gearCount int32
	// hasArea reports whether at least one counted gear had resolvable
	// coordinates; latitudeDeg / longitudeDeg are meaningful only when true.
	hasArea      bool
	latitudeDeg  float64
	longitudeDeg float64
}

// loadCommunityGearStats returns, per community id, the number of gear
// currently shared into the community and the centroid (arithmetic mean of
// lat/lng) over that community's geocoded gear, powering CommunityItem's
// gear_count and area anchor (#2634). Archived community-gear rows are
// excluded from both, and gear whose record no longer resolves (soft-deleted)
// drops out with them. Gear without a resolvable location still counts but
// does not contribute to the centroid. Runs on every ListCommunities call, so
// the cost is capped at three batched queries regardless of community count:
// the community-gear rows, their gear, their locations.
func (s *Service) loadCommunityGearStats(
	ctx context.Context,
	communityIDs []string,
) (map[string]*communityGearStats, error) {
	stats := make(map[string]*communityGearStats)
	if len(communityIDs) == 0 {
		return stats, nil
	}

	cgRows, err := storage.QueryByFieldIn[*models.CommunityGear](s.storage, ctx, "community_id", communityIDs)
	if err != nil {
		return nil, fmt.Errorf("query community gear: %w", err)
	}
	active := make([]*models.CommunityGear, 0, len(cgRows))
	gearIDSet := make(map[string]struct{}, len(cgRows))
	gearIDs := make([]string, 0, len(cgRows))
	for _, cg := range cgRows {
		if cg.Archived {
			continue
		}
		active = append(active, cg)
		if _, seen := gearIDSet[cg.GearId]; !seen {
			gearIDSet[cg.GearId] = struct{}{}
			gearIDs = append(gearIDs, cg.GearId)
		}
	}

	// Soft-deleted gear is excluded by the storage layer's default deleted
	// filter, so a share whose gear was deleted simply fails to resolve here
	// and is skipped below.
	gearMap, err := storage.GetByIDs[*models.Gear](s.storage, ctx, gearIDs)
	if err != nil {
		return nil, fmt.Errorf("batch fetch community gear: %w", err)
	}

	locationIDSet := make(map[string]struct{})
	locationIDs := make([]string, 0, len(gearMap))
	for _, g := range gearMap {
		if g.LocationId == "" {
			continue
		}
		if _, seen := locationIDSet[g.LocationId]; !seen {
			locationIDSet[g.LocationId] = struct{}{}
			locationIDs = append(locationIDs, g.LocationId)
		}
	}
	locationMap, err := storage.GetByIDs[*models.Location](s.storage, ctx, locationIDs)
	if err != nil {
		return nil, fmt.Errorf("batch fetch gear locations: %w", err)
	}

	// Aggregate in memory: count every resolvable share, and sum coordinates
	// for the geocoded ones so the centroid falls out at the end.
	type coordSum struct {
		latSum, lngSum float64
		geocoded       int
	}
	sums := make(map[string]*coordSum)
	for _, cg := range active {
		gear, ok := gearMap[cg.GearId]
		if !ok {
			continue // gear soft-deleted or missing
		}
		st := stats[cg.CommunityId]
		if st == nil {
			st = &communityGearStats{}
			stats[cg.CommunityId] = st
		}
		st.gearCount++

		if gear.LocationId == "" {
			continue
		}
		loc, ok := locationMap[gear.LocationId]
		if !ok {
			continue
		}
		geo := loc.GetGeolocation()
		// (0, 0) is treated as absent: it is the proto zero value, not a
		// plausible gear location.
		if geo == nil || (geo.LatitudeDeg == 0 && geo.LongitudeDeg == 0) {
			continue
		}
		sum := sums[cg.CommunityId]
		if sum == nil {
			sum = &coordSum{}
			sums[cg.CommunityId] = sum
		}
		sum.latSum += geo.LatitudeDeg
		sum.lngSum += geo.LongitudeDeg
		sum.geocoded++
	}
	for cid, sum := range sums {
		st := stats[cid]
		st.hasArea = true
		st.latitudeDeg = sum.latSum / float64(sum.geocoded)
		st.longitudeDeg = sum.lngSum / float64(sum.geocoded)
	}
	return stats, nil
}

// memberPreviewLimit caps how many member first names a nameless (ad-hoc)
// community shows in its group-chat-style label; the rest fold into the
// client's "& N others" overflow (derived from member_count).
const memberPreviewLimit = 3

// firstFewMemberFirstNames returns up to limit first names of members other
// than excludeUserID, sorted for a stable order, used to render a nameless
// community like a group chat. Members whose user record or name is missing
// are skipped.
func firstFewMemberFirstNames(memberIDs []string, excludeUserID string, users map[string]*models.User, limit int) []string {
	names := make([]string, 0, len(memberIDs))
	for _, uid := range memberIDs {
		if uid == excludeUserID {
			continue
		}
		u, ok := users[uid]
		if !ok {
			continue
		}
		if fn := firstName(u.Name); fn != "" {
			names = append(names, fn)
		}
	}
	sort.Strings(names)
	if len(names) > limit {
		names = names[:limit]
	}
	return names
}

// firstName returns the first whitespace-delimited token of a full name, or
// "" when the name is blank.
func firstName(full string) string {
	fields := strings.Fields(full)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// sortCommunitiesByActivity reorders [items] in-place so the community
// with the most recent CommunityEvent appears first and the one with
// the oldest event appears last. Communities with no events sink to
// the bottom in their pre-existing membership order — preserving a
// stable fallback for fresh memberships that have not yet generated
// activity. Powers the Workshop carousel's recency-first ordering
// (#1898).
func (s *Service) sortCommunitiesByActivity(
	ctx context.Context,
	items []*api.CommunityItem,
	logger *logging.Logger,
) error {
	if len(items) <= 1 {
		return nil
	}
	ids := make([]string, len(items))
	for i, c := range items {
		ids[i] = c.Id
	}
	latest, err := s.storage.MaxInt64FieldByGroup(
		ctx, "community_event", "community_id", "occurred_at_unix_sec", ids,
	)
	if err != nil {
		return err
	}
	logger.DebugContext(ctx, "sorted communities by latest CommunityEvent",
		"community_count", len(items),
		"with_activity", len(latest),
	)
	// Surface each community's latest-activity time so the directory can
	// order and day-group by recency (#2568).
	for _, c := range items {
		if ts, ok := latest[c.Id]; ok {
			v := ts
			c.LastActivityUnixSec = &v
		}
	}
	// Capture pre-sort index so communities with equal (or absent)
	// timestamps retain their inbound order — sort.Slice is not stable.
	indexOf := make(map[string]int, len(items))
	for i, c := range items {
		indexOf[c.Id] = i
	}
	sort.SliceStable(items, func(i, j int) bool {
		ti, hi := latest[items[i].Id]
		tj, hj := latest[items[j].Id]
		if hi != hj {
			// Communities with at least one event come before those
			// without.
			return hi
		}
		if ti != tj {
			return ti > tj
		}
		return indexOf[items[i].Id] < indexOf[items[j].Id]
	})
	return nil
}

// communityMatchesRegion checks if a community has a region matching the filter.
func (s *Service) communityMatchesRegion(ctx context.Context, communityID string, filter *api.RegionFilter) (bool, error) {
	// Check if community has the specified region_id
	regions, err := s.storage.QueryByFields(ctx, map[string]any{
		"community_id": communityID,
		"region_id":    filter.RegionId,
	}, &models.CommunityRegion{})
	if err != nil {
		return false, err
	}

	return len(regions) > 0, nil
}

// ListDeletedCommunitiesForRestore returns the soft-deleted
// communities the caller is eligible to restore — i.e., the
// communities where the caller's user_id appears in
// Community.deleted_snapshot.member_user_ids. Powers the
// Settings → Communities deleted-list surface (#1717). See
// docs/community_delete_and_leave.md §2.5, §7.1.
//
// Distinct from ListCommunities, which is membership-scoped and
// strips soft-deleted communities. Per the proto convention in
// CLAUDE.md, this RPC has its own dedicated request/response
// types (no reuse of ListCommunitiesRequest / CommunityItem).
func (s *Service) ListDeletedCommunitiesForRestore(
	ctx context.Context,
	req *connect.Request[api.ListDeletedCommunitiesForRestoreRequest],
) (*connect.Response[api.ListDeletedCommunitiesForRestoreResponse], error) {
	_ = req

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ListDeletedCommunitiesForRestore",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)
	logger.DebugContext(ctx, "listing deleted communities for restore")

	const limit = 100
	communities, err := s.storage.FindCommunitiesEligibleForRestore(ctx, authInfo.UserID, limit)
	if err != nil {
		logger.ErrorContext(ctx, "failed to find communities eligible for restore", "error", err)
		return nil, connecterr.Internal(ctx, "ListDeletedCommunitiesForRestore", err)
	}

	items := make([]*api.DeletedCommunityItem, 0, len(communities))
	for _, c := range communities {
		items = append(items, &api.DeletedCommunityItem{
			Id:               c.Id,
			Name:             c.Name,
			Description:      c.Description,
			MediaIds:         c.MediaIds,
			DeletedAtUnixSec: c.GetDeleted().GetDeletedAtUnixSec(),
			DeletedByUserId:  c.GetDeleted().GetDeletedByUserId(),
		})
	}

	logger.InfoContext(ctx, "listed deleted communities for restore", "result_count", len(items))
	return connect.NewResponse(&api.ListDeletedCommunitiesForRestoreResponse{
		Communities: items,
	}), nil
}

// ListRejoinableCommunities returns the active communities the
// caller can rejoin without a fresh invite — communities where
// the caller has a soft-deleted CommunityUser row less than 30
// days old AND the community itself is still active. Powers the
// Settings → Communities recently-left surface (#1721). See
// docs/community_delete_and_leave.md §2.6, §2.7.
//
// Distinct from ListCommunities, which is active-membership-scoped.
// Per the proto convention in CLAUDE.md, this RPC has its own
// dedicated request/response types.
func (s *Service) ListRejoinableCommunities(
	ctx context.Context,
	req *connect.Request[api.ListRejoinableCommunitiesRequest],
) (*connect.Response[api.ListRejoinableCommunitiesResponse], error) {
	_ = req

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ListRejoinableCommunities",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)
	logger.DebugContext(ctx, "listing rejoinable communities")

	const limit = 100
	now := clock.UnixSec(ctx)
	pairs, err := s.storage.FindRejoinableCommunitiesForUser(ctx, authInfo.UserID, now, communitylib.RejoinWindowSeconds, limit)
	if err != nil {
		logger.ErrorContext(ctx, "failed to find rejoinable communities", "error", err)
		return nil, connecterr.Internal(ctx, "ListRejoinableCommunities", err)
	}

	items := make([]*api.RejoinableCommunityItem, 0, len(pairs))
	for _, pair := range pairs {
		items = append(items, &api.RejoinableCommunityItem{
			Id:            pair.Community.Id,
			Name:          pair.Community.Name,
			Description:   pair.Community.Description,
			MediaIds:      pair.Community.MediaIds,
			LeftAtUnixSec: pair.Membership.GetDeleted().GetDeletedAtUnixSec(),
		})
	}

	logger.InfoContext(ctx, "listed rejoinable communities", "result_count", len(items))
	return connect.NewResponse(&api.ListRejoinableCommunitiesResponse{
		Communities: items,
	}), nil
}
