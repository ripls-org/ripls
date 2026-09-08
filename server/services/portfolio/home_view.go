package portfolio

import (
	"context"
	"sort"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// Section caps for GetHomeView (#2435). Every repeated field in the response
// is bounded; gear additionally reports gear_has_more when truncated.
const (
	homeMaxDecisions = 50
	homeMaxUpNext    = 20
	homeMaxAsks      = 20
	homeMaxEvents    = 20
	homeMaxGear      = 100
	homeMaxActivity  = 50
	homeMaxCalendar  = 200

	// homeActivityWindowDays bounds how far back recent activity reaches.
	homeActivityWindowDays = 30

	// homeCalendarPastWindow bounds how far back the calendar's milestone
	// markers (gear/request first-shared dates, past events) reach, so the
	// timeline stays a near-term planning aid rather than full history.
	homeCalendarPastWindow = 180 * 24 * time.Hour

	// homeDueSoonWindow is the "due ≤ 72h" threshold for the amber gear pill.
	homeDueSoonWindow = 72 * time.Hour

	// homeDecisionSnooze is how long a "Not now" keeps a decision out of the
	// Needs-you queue before it returns (#2435). Snooze, not dismiss.
	homeDecisionSnooze = 24 * time.Hour
)

// homeExtras holds data needed by the Home view beyond what fetchAll loads:
// planning needs (request claim progress), request offers (request-claim decisions),
// and the user's dismissed decision IDs ("Not now").
type homeExtras struct {
	// needsByRequestID maps request_id → non-deleted planning needs.
	needsByRequestID map[string][]*models.PlanningNeed

	// offersByRequestID maps request_id → non-withdrawn offers.
	offersByRequestID map[string][]*models.RequestOffer

	// dismissedDecisionIDs is the set of HomeDecision IDs the user dismissed
	// via "Not now" (stored as dismissed DECISION watch entries).
	dismissedDecisionIDs map[string]bool
}

// GetHomeView assembles the sectioned Home tab (v4.1 redesign, #2435):
// Needs-you decisions, the Up-next agenda, the viewer's requests, gear rows,
// and recent activity, across all of the user's communities.
func (s *Service) GetHomeView(
	ctx context.Context,
	req *connect.Request[api.GetHomeViewRequest],
) (*connect.Response[api.GetHomeViewResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	userID := authInfo.UserID

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetHomeView",
		"user_id", userID,
	)

	tz := time.UTC
	if req.Msg.Timezone != "" {
		if loc, tzErr := time.LoadLocation(req.Msg.Timezone); tzErr == nil {
			tz = loc
		}
	}
	now := time.Now()

	fetchStart := time.Now()
	d, err := s.fetchAll(ctx, userID)
	if err != nil {
		return nil, err
	}

	extras, err := s.fetchHomeExtras(ctx, d, userID, now)
	if err != nil {
		return nil, err
	}
	fetchDurationMs := time.Since(fetchStart).Milliseconds()

	resp := assembleHomeView(d, extras, userID, tz, now)

	// Surface an optional contextual nudge (reuses the feed nudge system) across
	// the viewer's communities. Best-effort: a nudge failure never fails the home
	// view. The primary (lexicographically-smallest) community leads so the async
	// generation target is stable.
	if s.nudgeProvider != nil && len(d.communityIDs) > 0 {
		communityIDs := orderedCommunityIDs(d.communityIDs)
		if nudge, nErr := s.nudgeProvider.InboxNudge(ctx, userID, communityIDs); nErr != nil {
			logger.WarnContext(ctx, "failed to fetch inbox nudge", "error", nErr)
		} else {
			resp.Nudge = nudge
		}
	}

	// Per-day weather for the calendar (best-effort; never fails the view). The
	// forecast also gates the open-day suggestions' confidence — beyond-horizon
	// "typical" days suggest more tentatively.
	weatherStart := time.Now()
	resp.Forecast = s.fetchHomeForecast(ctx, d, userID, tz, now)
	weatherDurationMs := time.Since(weatherStart).Milliseconds()
	resp.OpenDaySuggestions = assembleOpenDaySuggestions(d, userID, resp.Calendar, resp.Forecast, tz, now)

	// The per-phase durations answer "where did a slow request spend its
	// time" from this one line — the forensic gap that let #2646 be
	// misdiagnosed as Cloud Run cold starts.
	logger.InfoContext(ctx, "home view assembled",
		"decision_count", resp.DecisionCount,
		"up_next_count", len(resp.UpNext),
		"ask_count", len(resp.YourAsks),
		"gear_count", len(resp.Gear),
		"activity_count", len(resp.RecentActivity),
		"fetch_duration_ms", fetchDurationMs,
		"weather_duration_ms", weatherDurationMs,
	)
	return connect.NewResponse(resp), nil
}

// orderedCommunityIDs returns the user's communities sorted lexicographically,
// dropping empties. The inbox nudge searches them in this order, so the async
// generation target (the first entry) is stable across requests.
func orderedCommunityIDs(communityIDs []string) []string {
	ordered := make([]string, 0, len(communityIDs))
	for _, id := range communityIDs {
		if id != "" {
			ordered = append(ordered, id)
		}
	}
	sort.Strings(ordered)
	return ordered
}

// fetchHomeExtras loads the Home-view-specific data not covered by fetchAll.
// All storage I/O for the Home view happens here or in fetchAll, so that
// assembleHomeView stays a pure function over pre-fetched maps.
func (s *Service) fetchHomeExtras(ctx context.Context, d *fetchedData, userID string, now time.Time) (*homeExtras, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "fetchHomeExtras",
		"user_id", userID,
	)

	extras := &homeExtras{
		needsByRequestID:     make(map[string][]*models.PlanningNeed),
		offersByRequestID:    make(map[string][]*models.RequestOffer),
		dismissedDecisionIDs: make(map[string]bool),
	}

	// Owned, non-terminal request IDs — the only ones whose needs/offers the
	// Home view renders.
	var ownedActiveReqIDs []string
	for _, r := range d.ownedRequests {
		if isRequestTerminal(r.State) {
			continue
		}
		ownedActiveReqIDs = append(ownedActiveReqIDs, r.Id)
	}

	if len(ownedActiveReqIDs) > 0 {
		needsRaw, err := s.storage.QueryByFieldIn(ctx, "request_id", ownedActiveReqIDs, &models.PlanningNeed{})
		if err != nil {
			return nil, connecterr.Internal(ctx, "fetchHomeExtras", err, "detail", "failed to query planning needs")
		}
		for _, m := range needsRaw {
			n := m.(*models.PlanningNeed)
			if n.Deleted != nil && n.Deleted.DeletedAtUnixSec > 0 {
				continue
			}
			reqID := n.GetRequestId()
			if reqID == "" {
				continue
			}
			extras.needsByRequestID[reqID] = append(extras.needsByRequestID[reqID], n)
		}

		offersRaw, err := s.storage.QueryByFieldIn(ctx, "request_id", ownedActiveReqIDs, &models.RequestOffer{})
		if err != nil {
			return nil, connecterr.Internal(ctx, "fetchHomeExtras", err, "detail", "failed to query request offers")
		}
		for _, m := range offersRaw {
			o := m.(*models.RequestOffer)
			if o.Withdrawn {
				continue
			}
			extras.offersByRequestID[o.RequestId] = append(extras.offersByRequestID[o.RequestId], o)
		}
	}

	// Decisions leave the queue two ways, both recorded on a DECISION watch:
	//   - Resolved (is_read): the owner said thanks on a request claim — gone
	//     for good.
	//   - Snoozed (dismissed_at within the window): "Not now" — the badge
	//     decrements now, but the decision returns after the snooze window so
	//     it is never silently lost.
	// A failure here degrades gracefully — the queue re-shows the cards
	// rather than failing the whole view.
	snoozeFloor := now.Add(-homeDecisionSnooze).Unix()
	allWatches, err := s.watchStorage.GetAllWatches(ctx, userID)
	if err != nil {
		logger.WarnContext(ctx, "failed to fetch decision watches", "error", err)
		return extras, nil
	}
	for _, w := range allWatches {
		if w.ItemType != models.WatchedItemType_WATCHED_ITEM_TYPE_DECISION {
			continue
		}
		switch {
		case w.IsRead:
			// Resolved (thanked) — excluded permanently.
			extras.dismissedDecisionIDs[w.ItemId] = true
		case w.DismissedAtUnixSec != nil && *w.DismissedAtUnixSec >= snoozeFloor:
			// Snoozed within the window; older snoozes lapse and reappear.
			extras.dismissedDecisionIDs[w.ItemId] = true
		}
	}
	return extras, nil
}

// assembleHomeView joins the pre-fetched data into the Home view response.
// Pure function: no storage I/O.
func assembleHomeView(
	d *fetchedData,
	extras *homeExtras,
	userID string,
	tz *time.Location,
	now time.Time,
) *api.GetHomeViewResponse {
	decisions, decisionCount := assembleHomeDecisions(d, extras, userID, tz, now)
	gear, gearCounts, gearHasMore := assembleHomeGear(d, userID, now)
	return &api.GetHomeViewResponse{
		Decisions:      decisions,
		DecisionCount:  decisionCount,
		UpNext:         assembleHomeUpNext(d, extras, userID, tz, now),
		YourAsks:       assembleHomeAsks(d, extras),
		Gear:           gear,
		GearHasMore:    gearHasMore,
		GearCounts:     gearCounts,
		RecentActivity: assembleHomeActivity(d, userID, now),
		YourEvents:     assembleOwnedEvents(d, userID),
		Calendar:       assembleHomeCalendar(d, extras, userID, tz, now),
	}
}
