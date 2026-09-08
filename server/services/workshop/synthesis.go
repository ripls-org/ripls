package workshop

import (
	"context"
	"fmt"
	"math"
	"sort"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
)

// GetWorkshopSynthesis returns the "Together this season" panels for
// the host's circles. Reuses `server/impact_metrics.Calculator` (a
// shared library; no service-to-service injection per
// `docs/server/architecture.md`) for the underlying counts and
// replaced-cost estimate. Each panel carries a per-circle breakdown
// so the SeasonReportScreen can render rows like "Backcountry · 23h".
func (s *Service) GetWorkshopSynthesis(
	ctx context.Context,
	req *connect.Request[api.GetWorkshopSynthesisRequest],
) (*connect.Response[api.GetWorkshopSynthesisResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Drop soft-deleted or non-member communities before any data access.
	communityIDs, _, err := auth.FilterActiveMemberCommunities(ctx, s.storage, req.Msg.CommunityIds, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetWorkshopSynthesis",
		"user_id", authInfo.UserID,
		"community_count", len(communityIDs),
	)

	if s.calculator == nil {
		// No calculator wired — return an empty synthesis rather than a
		// hard failure. The client treats empty panels as "quiet month."
		logger.InfoContext(ctx, "no calculator wired; returning empty synthesis")
		return connect.NewResponse(&api.GetWorkshopSynthesisResponse{}), nil
	}

	// Sum activity counts and replaced-cost estimates across the
	// requested circles. Track per-community values so each panel can
	// emit a breakdown.
	totals := aggregateTotals{}
	perCommunity := map[string]*communityTotals{}
	for _, communityID := range communityIDs {
		ct := &communityTotals{}
		acts, err := s.calculator.CalculateActivityCounts(ctx, communityID)
		if err != nil {
			logger.WarnContext(ctx, "activity counts failed",
				"community_id", communityID,
				"error", err,
			)
			acts = nil
		}
		savings, err := s.calculator.CalculateImpactSavings(ctx, communityID)
		if err != nil {
			logger.WarnContext(ctx, "impact savings failed",
				"community_id", communityID,
				"error", err,
			)
			savings = nil
		}
		ct.add(acts, savings)
		totals.add(acts, savings)
		ct.displayName = s.lookupCommunityName(ctx, communityID)
		perCommunity[communityID] = ct
	}

	if totals.empty() {
		return connect.NewResponse(&api.GetWorkshopSynthesisResponse{}), nil
	}

	panels := totals.toPanels(perCommunity)
	logger.DebugContext(ctx, "synthesis assembled",
		"panel_count", len(panels),
	)
	return connect.NewResponse(&api.GetWorkshopSynthesisResponse{
		Panels: panels,
	}), nil
}

// lookupCommunityName fetches the community's display name. Returns ""
// when the community can't be loaded — the breakdown row is dropped
// in that case rather than rendering a blank label.
func (s *Service) lookupCommunityName(ctx context.Context, communityID string) string {
	if communityID == "" {
		return ""
	}
	c := &models.Community{}
	if err := s.storage.GetByID(ctx, communityID, c); err != nil {
		return ""
	}
	return c.Name
}

// communityTotals tracks per-community values so each synthesis panel
// can emit a breakdown row.
type communityTotals struct {
	displayName string
	pastEvents  int32
	costSavings float64
	carbonGrams float64
	timeMinutes float64
}

func (t *communityTotals) add(a *impact_metrics.ActivityCounts, s *impact_metrics.ImpactSavingsResult) {
	if a != nil {
		t.pastEvents += a.PastEvents
	}
	if s != nil {
		if s.CostSavings != nil {
			t.costSavings += float64(s.CostSavings.Mean)
		}
		if s.CarbonSavings != nil {
			t.carbonGrams += float64(s.CarbonSavings.Mean)
		}
		// timeMinutes feeds the Workshop home hero's "hours together"
		// metric, which the client looks up under the QUALITY_TIME
		// dimension. The brief therefore reads QualityTime (social
		// hours), not TimeBanked (logistical time saved), so the home
		// tile matches the detail-screen breakdown.
		if s.QualityTime != nil {
			t.timeMinutes += float64(s.QualityTime.Mean)
		}
	}
}

// aggregateTotals sums activity + savings across circles for the
// synthesis surface. Kept inline to this file because no other caller
// needs the same shape; if a third surface eventually wants it, lift
// it into the impact_metrics shared library.
type aggregateTotals struct {
	pastEvents     int32
	upcomingEvents int32
	completedLoans int32
	openGiveaways  int32
	actsCount      int
	// problemsSolved (X) = completed loans + fulfilled requests + claimed
	// needs; problemsPotential (Y) = all non-cancelled loan transfers + all
	// non-cancelled requests + all posted needs. Both come wholesale from
	// the calculator's CalculateProblemsCounts (collectSeasonTotals), the
	// single source of truth, so the headline fraction and the deep-dive
	// always agree.
	problemsSolved    int
	problemsPotential int
	costSavings       float64
	carbonGrams       float64
	timeMinutes       float64
}

func (t *aggregateTotals) add(a *impact_metrics.ActivityCounts, s *impact_metrics.ImpactSavingsResult) {
	if a != nil {
		t.pastEvents += a.PastEvents
		t.upcomingEvents += a.UpcomingEvents
		t.completedLoans += a.CompletedLoans
		t.openGiveaways += a.OpenGiveaways
	}
	if s != nil {
		if s.CostSavings != nil {
			t.costSavings += float64(s.CostSavings.Mean)
		}
		if s.CarbonSavings != nil {
			t.carbonGrams += float64(s.CarbonSavings.Mean)
		}
		// See [communityTotals.add] — QualityTime is the metric backing
		// the Workshop home "hours together" tile and the detail
		// screen's QUALITY_TIME breakdown.
		if s.QualityTime != nil {
			t.timeMinutes += float64(s.QualityTime.Mean)
		}
	}
}

// addActs adds the count from an [impact_metrics.ActsBreakdown] to the
// running total. Split from [add] because acts are computed from a
// separate calculator call (CountActs), independent of the
// ActivityCounts/ImpactSavingsResult pair.
func (t *aggregateTotals) addActs(a *impact_metrics.ActsBreakdown) {
	if a != nil {
		t.actsCount += int(a.Total)
	}
}

func (t *aggregateTotals) empty() bool {
	return t.pastEvents == 0 &&
		t.completedLoans == 0 &&
		t.openGiveaways == 0 &&
		t.costSavings == 0 &&
		t.timeMinutes == 0
}

// toPanels renders the aggregateTotals as the three serif panels the
// SeasonReportScreen expects: together-time, what-sharing-replaced,
// and CO₂ avoided. Each panel carries a per-circle breakdown so the
// client can render rows like "Backcountry · 23h".
func (t *aggregateTotals) toPanels(perCommunity map[string]*communityTotals) []*api.SynthesisPanel {
	out := make([]*api.SynthesisPanel, 0, 3)

	if t.timeMinutes > 0 {
		out = append(out, &api.SynthesisPanel{
			Kind:      "time",
			Label:     "Together-time",
			Lede:      togetherTimeLede(t),
			Body:      togetherTimeBody(t),
			Breakdown: timeBreakdown(perCommunity),
		})
	}
	if t.costSavings > 0 {
		out = append(out, &api.SynthesisPanel{
			Kind:      "replaced",
			Label:     "What sharing replaced",
			Lede:      replacedPanelLede(t),
			Body:      replacedPanelBody(t),
			Breakdown: replacedBreakdown(perCommunity),
		})
	}
	if t.carbonGrams > 0 {
		out = append(out, &api.SynthesisPanel{
			Kind:      "co2",
			Label:     "CO₂ avoided",
			Lede:      co2PanelLede(t),
			Body:      co2PanelBody(t),
			Breakdown: co2Breakdown(perCommunity),
		})
	}
	return out
}

func replacedPanelLede(t *aggregateTotals) string {
	return fmt.Sprintf("About $%d stayed off rental shelves this season.", int(math.Round(t.costSavings)))
}

func replacedPanelBody(t *aggregateTotals) string {
	if t.completedLoans > 0 {
		return fmt.Sprintf("%d things borrowed instead of bought. The shared pool quietly does the work.", t.completedLoans)
	}
	return "The shared pool quietly does the work."
}

func togetherTimeLede(t *aggregateTotals) string {
	hours := int(math.Round(t.timeMinutes / 60))
	if hours <= 0 {
		hours = 1
	}
	return fmt.Sprintf("%s hours together this season.", spelledOutHours(hours))
}

func togetherTimeBody(t *aggregateTotals) string {
	if t.pastEvents > 0 {
		return fmt.Sprintf("Across %d gatherings — that's the rhythm.", t.pastEvents)
	}
	return "Steady minutes that add up."
}

func co2PanelLede(t *aggregateTotals) string {
	lbs := t.carbonGrams / 453.592
	rounded := int(math.Round(lbs/10) * 10)
	if rounded <= 0 {
		rounded = int(math.Round(lbs))
	}
	return fmt.Sprintf("About %d lbs of CO₂ kept off the road.", rounded)
}

func co2PanelBody(t *aggregateTotals) string {
	if t.completedLoans > 0 {
		return "Trips that doubled up. Tools borrowed instead of bought."
	}
	return "Carbon that didn't leave the road."
}

// timeBreakdown emits per-circle hours rows. Sorted descending by hours.
func timeBreakdown(perCommunity map[string]*communityTotals) []*api.SynthesisPanelBreakdown {
	rows := []*api.SynthesisPanelBreakdown{}
	for cid, ct := range perCommunity {
		if ct == nil || ct.timeMinutes <= 0 {
			continue
		}
		hours := int(math.Round(ct.timeMinutes / 60))
		if hours <= 0 {
			continue
		}
		rows = append(rows, &api.SynthesisPanelBreakdown{
			CommunityId:    cid,
			DisplayName:    ct.displayName,
			FormattedValue: fmt.Sprintf("%dh", hours),
		})
	}
	sortByFormattedHours(rows)
	return rows
}

// replacedBreakdown emits per-circle replaced-cost rows. Sorted descending.
func replacedBreakdown(perCommunity map[string]*communityTotals) []*api.SynthesisPanelBreakdown {
	rows := []*api.SynthesisPanelBreakdown{}
	for cid, ct := range perCommunity {
		if ct == nil || ct.costSavings <= 0 {
			continue
		}
		rows = append(rows, &api.SynthesisPanelBreakdown{
			CommunityId:    cid,
			DisplayName:    ct.displayName,
			FormattedValue: fmt.Sprintf("~$%d", int(math.Round(ct.costSavings))),
		})
	}
	sortByCostSavings(rows, perCommunity)
	return rows
}

// co2Breakdown emits per-circle CO₂-avoided rows in pounds. Sorted descending.
func co2Breakdown(perCommunity map[string]*communityTotals) []*api.SynthesisPanelBreakdown {
	rows := []*api.SynthesisPanelBreakdown{}
	for cid, ct := range perCommunity {
		if ct == nil || ct.carbonGrams <= 0 {
			continue
		}
		lbs := ct.carbonGrams / 453.592
		rounded := int(math.Round(lbs/10) * 10)
		if rounded <= 0 {
			continue
		}
		rows = append(rows, &api.SynthesisPanelBreakdown{
			CommunityId:    cid,
			DisplayName:    ct.displayName,
			FormattedValue: fmt.Sprintf("~%d lbs", rounded),
		})
	}
	sortByCarbon(rows, perCommunity)
	return rows
}

func sortByFormattedHours(rows []*api.SynthesisPanelBreakdown) {
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].FormattedValue > rows[j].FormattedValue
	})
}

func sortByCostSavings(rows []*api.SynthesisPanelBreakdown, perCommunity map[string]*communityTotals) {
	sort.SliceStable(rows, func(i, j int) bool {
		ai := perCommunity[rows[i].CommunityId]
		aj := perCommunity[rows[j].CommunityId]
		if ai == nil || aj == nil {
			return false
		}
		return ai.costSavings > aj.costSavings
	})
}

func sortByCarbon(rows []*api.SynthesisPanelBreakdown, perCommunity map[string]*communityTotals) {
	sort.SliceStable(rows, func(i, j int) bool {
		ai := perCommunity[rows[i].CommunityId]
		aj := perCommunity[rows[j].CommunityId]
		if ai == nil || aj == nil {
			return false
		}
		return ai.carbonGrams > aj.carbonGrams
	})
}

// spelledOutHours formats small hour counts as words ("Sixty-seven")
// matching the JSX reference style. Falls back to digits for larger
// numbers — the tone is meant to feel like a host's narrative, not a
// dashboard reading.
func spelledOutHours(hours int) string {
	switch hours {
	case 1:
		return "One"
	case 2, 3, 4, 5, 6, 7, 8, 9, 10:
		return []string{"Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten"}[hours-2]
	}
	return fmt.Sprintf("%d", hours)
}
