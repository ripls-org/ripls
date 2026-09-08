package workshop

import (
	"context"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/known_for"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// GetCategoryDetail returns the representative items and the people
// behind them for one capability tag, scoped to a community set and
// optional owner. Backs the "known for" chip-detail screen on both
// the workshop community surface (ModePerCommunity) and the user-
// profile surface (ModePerUser).
//
// The shape mirrors the chip derivation in `known_for.Derive`: same
// scope rules, same archived-pivot semantics, same query budget. The
// caller's community-ID list is filtered through
// FilterActiveMemberCommunities so a viewer can never reach into a
// community they don't belong to via this RPC. Items are returned as
// RecentActivity rows so the client can render them with the same
// list widget the impact-metric detail screens use; the metric
// `value` column is intentionally left empty on this surface.
func (s *Service) GetCategoryDetail(
	ctx context.Context,
	req *connect.Request[api.GetCategoryDetailRequest],
) (*connect.Response[api.GetCategoryDetailResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	communityIDs, _, err := auth.FilterActiveMemberCommunities(
		ctx, s.storage, req.Msg.CommunityIds, authInfo.UserID,
	)
	if err != nil {
		return nil, err
	}

	mode := known_for.ModePerCommunity
	ownerID := ""
	if req.Msg.Mode == api.KnownForMode_KNOWN_FOR_MODE_PER_USER {
		mode = known_for.ModePerUser
		if req.Msg.OwnerId != nil {
			ownerID = *req.Msg.OwnerId
		}
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetCategoryDetail",
		"user_id", authInfo.UserID,
		"mode", req.Msg.Mode.String(),
		"category", req.Msg.Category,
		"community_id_count", len(communityIDs),
	)
	if ownerID != "" {
		logger = logger.With("owner_id", ownerID)
	}

	if len(communityIDs) == 0 {
		return connect.NewResponse(&api.GetCategoryDetailResponse{}), nil
	}

	suppressedKeys, err := loadSuppressedKeysForScope(ctx, s, mode, ownerID, communityIDs)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetCategoryDetail.loadSuppressions", err)
	}

	result, err := known_for.Detail(ctx, s.storage, known_for.DetailOptions{
		Mode:           mode,
		OwnerID:        ownerID,
		CommunityIDs:   communityIDs,
		Category:       req.Msg.Category,
		SuppressedKeys: suppressedKeys,
	})
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetCategoryDetail.Detail", err)
	}

	resp := &api.GetCategoryDetailResponse{
		MemberIds: result.MemberIDs,
	}
	if result.Suppressed {
		suppressed := true
		resp.Suppressed = &suppressed
		logger.DebugContext(ctx, "category detail: scope is suppressed")
		return connect.NewResponse(resp), nil
	}

	// Resolve owner display names with a single batched fetch — never
	// per-row, per the N+1 ban in CLAUDE.md.
	ownerNames, err := loadOwnerNames(ctx, s.storage, result.Items)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetCategoryDetail.loadOwnerNames", err)
	}

	resp.Items = make([]*api.RecentActivity, 0, len(result.Items))
	for _, it := range result.Items {
		if row := recentActivityFromDetail(it, ownerNames); row != nil {
			resp.Items = append(resp.Items, row)
		}
	}

	logger.DebugContext(ctx, "category detail: assembled",
		"item_count", len(resp.Items),
		"member_count", len(resp.MemberIds),
	)
	return connect.NewResponse(resp), nil
}

// loadOwnerNames batches a single User lookup for every distinct
// owner referenced by [items]. Returns a userID → display name map.
// Missing or soft-deleted users surface as an empty string and the
// row simply omits the owner — the chip-detail client falls back to
// "Xh ago" without the owner prefix.
func loadOwnerNames(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	items []known_for.DetailItem,
) (map[string]string, error) {
	idSet := make(map[string]struct{}, len(items))
	for _, it := range items {
		id := detailItemOwnerID(it)
		if id != "" {
			idSet[id] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return map[string]string{}, nil
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	byID, err := storage.GetByIDs[*models.User](s, ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(byID))
	for id, u := range byID {
		if u == nil {
			continue
		}
		out[id] = u.Name
	}
	return out, nil
}

// detailItemOwnerID returns the user id of the row's "owner" —
// gear owner, experience host, or request requester. Used to drive
// the personName column in the rendered row.
func detailItemOwnerID(it known_for.DetailItem) string {
	switch it.Kind {
	case known_for.DetailKindGear:
		if it.Gear != nil {
			return it.Gear.OwnerId
		}
	case known_for.DetailKindExperience:
		if it.Experience != nil {
			return it.Experience.OwnerId
		}
	case known_for.DetailKindRequest:
		if it.Request != nil {
			return it.Request.RequesterId
		}
	}
	return ""
}

// recentActivityFromDetail converts an in-package DetailItem to the
// wire RecentActivity row used by the chip-detail screen. The
// metric value column is intentionally left empty so the
// RecentItemsList widget skips rendering it.
func recentActivityFromDetail(
	it known_for.DetailItem,
	ownerNames map[string]string,
) *api.RecentActivity {
	switch it.Kind {
	case known_for.DetailKindGear:
		return gearToRecentActivity(it.Gear, ownerNames, it.RecencySecs)
	case known_for.DetailKindExperience:
		return experienceToRecentActivity(it.Experience, ownerNames, it.RecencySecs)
	case known_for.DetailKindRequest:
		return requestToRecentActivity(it.Request, ownerNames, it.RecencySecs)
	default:
		return nil
	}
}

// optionalName returns a pointer to s, or nil when s is empty, so an unknown
// name is absent on the wire rather than a server-invented English placeholder
// — the client renders its own fallback (#2835).
func optionalName(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func gearToRecentActivity(
	g *models.Gear,
	ownerNames map[string]string,
	recencySecs int64,
) *api.RecentActivity {
	if g == nil {
		return nil
	}
	row := &api.RecentActivity{
		ItemName:           optionalName(g.Name),
		PersonDisplayName:  optionalName(ownerNames[g.OwnerId]),
		ContentId:          g.Id,
		Kind:               api.RecentActivityKind_RECENT_ACTIVITY_KIND_GEAR,
		CompletedAtUnixSec: recencySecs,
	}
	if len(g.MediaIds) > 0 {
		row.MediaId = g.MediaIds[0]
	}
	return row
}

func experienceToRecentActivity(
	e *models.Experience,
	ownerNames map[string]string,
	recencySecs int64,
) *api.RecentActivity {
	if e == nil {
		return nil
	}
	row := &api.RecentActivity{
		ItemName:           optionalName(e.Name),
		PersonDisplayName:  optionalName(ownerNames[e.OwnerId]),
		ContentId:          e.Id,
		Kind:               api.RecentActivityKind_RECENT_ACTIVITY_KIND_EXPERIENCE,
		CompletedAtUnixSec: recencySecs,
	}
	if len(e.MediaIds) > 0 {
		row.MediaId = e.MediaIds[0]
	}
	return row
}

func requestToRecentActivity(
	r *models.Request,
	ownerNames map[string]string,
	recencySecs int64,
) *api.RecentActivity {
	if r == nil {
		return nil
	}
	row := &api.RecentActivity{
		ItemName:           optionalName(r.Title),
		PersonDisplayName:  optionalName(ownerNames[r.RequesterId]),
		ContentId:          r.Id,
		Kind:               api.RecentActivityKind_RECENT_ACTIVITY_KIND_REQUEST,
		CompletedAtUnixSec: recencySecs,
	}
	if len(r.MediaIds) > 0 {
		row.MediaId = r.MediaIds[0]
	}
	return row
}
