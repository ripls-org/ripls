package portfolio

import (
	"context"
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// isoWeekKey formats an ISO year+week into a comparable string key.
func isoWeekKey(year, week int) string {
	return fmt.Sprintf("%d-%02d", year, week)
}

// computePortfolioAggregates computes the supplementary aggregate fields for
// GetPortfolioMetricsResponse: CO₂ potential, quality-time-per-week,
// active-week count, and time-to-solve median.
//
// All queries are batched to avoid N+1 patterns. Errors are logged but not
// returned; missing data produces zero values rather than failing the RPC.
func (s *Service) computePortfolioAggregates(
	ctx context.Context,
	userID string,
	communityIDs []string,
) (co2PotentialGrams, qtPerWeekMinutes float32, activeWeeks int32, timeToSolveMinutes float32) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "computePortfolioAggregates",
		"user_id", userID,
	)

	// ── CO₂ potential ─────────────────────────────────────────────────────────
	// Sum embodied carbon of all active (non-deleted) gear shared in any of the
	// user's communities. A single QueryByFieldIn fetches all CommunityGear rows,
	// then a single GetByIDs resolves the gear protos.
	co2PotentialGrams = computeCo2PotentialForUser(ctx, s, logger, communityIDs)

	// ── Quality time per week ──────────────────────────────────────────────────
	// Re-use the already-computed total QT minutes from the SavedSelf section
	// rather than re-querying. We compute active weeks from the completed
	// transfer history to avoid another full scan.
	qtPerWeekMinutes, activeWeeks = computeQTPerWeek(ctx, s, logger, userID, communityIDs)

	// ── Time-to-solve ──────────────────────────────────────────────────────────
	timeToSolveMinutes = computeTimeToSolveForUser(ctx, s, logger, communityIDs)

	logger.InfoContext(ctx, "portfolio aggregates computed",
		"co2_potential_grams", co2PotentialGrams,
		"qt_per_week_minutes", qtPerWeekMinutes,
		"active_weeks", activeWeeks,
		"time_to_solve_minutes", timeToSolveMinutes,
	)
	return co2PotentialGrams, qtPerWeekMinutes, activeWeeks, timeToSolveMinutes
}

// computeCo2PotentialForUser sums the embodied carbon of all active gear
// shared in any of the user's communities. Uses batch queries.
func computeCo2PotentialForUser(
	ctx context.Context,
	s *Service,
	logger *logging.Logger,
	communityIDs []string,
) float32 {
	if len(communityIDs) == 0 {
		return 0
	}

	commGearRaw, err := s.storage.QueryByFieldIn(ctx, "community_id", communityIDs, &models.CommunityGear{})
	if err != nil {
		logger.WarnContext(ctx, "failed to query community gear for CO2 potential", "error", err)
		return 0
	}

	gearIDs := make([]string, 0, len(commGearRaw))
	seen := make(map[string]bool, len(commGearRaw))
	for _, m := range commGearRaw {
		cg := m.(*models.CommunityGear)
		if cg.Archived || seen[cg.GearId] {
			continue
		}
		seen[cg.GearId] = true
		gearIDs = append(gearIDs, cg.GearId)
	}

	if len(gearIDs) == 0 {
		return 0
	}

	gearMap, err := s.storage.GetByIDs(ctx, gearIDs, &models.Gear{})
	if err != nil {
		logger.WarnContext(ctx, "failed to batch-fetch gear for CO2 potential", "error", err)
		return 0
	}

	var totalGrams float32
	for _, m := range gearMap {
		g := m.(*models.Gear)
		if g.Deleted != nil && g.Deleted.DeletedAtUnixSec > 0 {
			continue
		}
		if g.EmbodiedCarbon != nil && g.EmbodiedCarbon.Co2EGrams != nil {
			totalGrams += g.EmbodiedCarbon.Co2EGrams.Mean
		}
	}
	return totalGrams
}

// computeQTPerWeek computes the average quality-time minutes per active ISO week.
// Active weeks are distinct ISO weeks that had at least one completed transfer,
// experience, or fulfilled request contributing quality time. Minimum 1 if any
// QT exists.
func computeQTPerWeek(
	ctx context.Context,
	s *Service,
	logger *logging.Logger,
	userID string,
	communityIDs []string,
) (qtPerWeekMinutes float32, activeWeeks int32) {
	if len(communityIDs) == 0 {
		return 0, 0
	}

	// Fetch completed transfers as owner or recipient in user's communities.
	communitySet := make(map[string]bool, len(communityIDs))
	for _, id := range communityIDs {
		communitySet[id] = true
	}

	ownerRaw, err := s.storage.QueryByField(ctx, "owner_id", userID, &models.Transfer{})
	if err != nil {
		logger.WarnContext(ctx, "failed to query owner transfers for QT/week", "error", err)
		return 0, 0
	}
	recipientRaw, err := s.storage.QueryByField(ctx, "recipient_id", userID, &models.Transfer{})
	if err != nil {
		logger.WarnContext(ctx, "failed to query recipient transfers for QT/week", "error", err)
		return 0, 0
	}

	seenID := make(map[string]bool)
	weekSet := make(map[string]bool) // ISO "YYYY-WW" keys.
	var totalQTMinutes float64

	for _, msgs := range [][]proto.Message{ownerRaw, recipientRaw} {
		for _, m := range msgs {
			t := m.(*models.Transfer)
			if seenID[t.Id] {
				continue
			}
			seenID[t.Id] = true
			if !communitySet[t.CommunityId] {
				continue
			}
			if t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
				continue
			}
			ie := t.ImpactEstimate
			if ie == nil || ie.QualityTime == nil || ie.QualityTime.QualityTimeMinutes == nil {
				continue
			}
			qt := float64(ie.QualityTime.QualityTimeMinutes.Mean)
			if qt <= 0 {
				continue
			}
			totalQTMinutes += qt
			year, week := time.Unix(t.LatestRequestUnixSec, 0).UTC().ISOWeek()
			weekSet[isoWeekKey(year, week)] = true
		}
	}

	if totalQTMinutes == 0 {
		return 0, 0
	}

	weeks := len(weekSet)
	if weeks == 0 {
		weeks = 1
	}
	activeWeeks = int32(weeks)
	qtPerWeekMinutes = float32(totalQTMinutes / float64(weeks))
	return qtPerWeekMinutes, activeWeeks
}

// computeTimeToSolveForUser computes the median minutes from request creation
// to fulfillment across all fulfilled requests in the user's communities.
// Queries are limited to the last 100 fulfilled requests for performance.
func computeTimeToSolveForUser(
	ctx context.Context,
	s *Service,
	logger *logging.Logger,
	communityIDs []string,
) float32 {
	if len(communityIDs) == 0 {
		return 0
	}

	commReqRaw, err := s.storage.QueryByFieldIn(ctx, "community_id", communityIDs, &models.CommunityRequest{})
	if err != nil {
		logger.WarnContext(ctx, "failed to query community requests for time-to-solve", "error", err)
		return 0
	}

	// Collect archived (completed/fulfilled) request IDs with their shared timestamp.
	type sharedEntry struct {
		requestID   string
		sharedAtSec int64
	}
	entries := make([]sharedEntry, 0, len(commReqRaw))
	for _, m := range commReqRaw {
		cr := m.(*models.CommunityRequest)
		if cr.Archived && cr.RequestId != "" {
			entries = append(entries, sharedEntry{requestID: cr.RequestId, sharedAtSec: cr.SharedAtUnixSec})
		}
	}

	if len(entries) == 0 {
		return 0
	}

	// Sort descending by sharedAt and cap at 100 for performance.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].sharedAtSec > entries[j].sharedAtSec
	})
	if len(entries) > 100 {
		entries = entries[:100]
	}

	reqIDs := make([]string, len(entries))
	sharedAt := make(map[string]int64, len(entries))
	for i, e := range entries {
		reqIDs[i] = e.requestID
		sharedAt[e.requestID] = e.sharedAtSec
	}

	reqMap, err := s.storage.GetByIDs(ctx, dedup(reqIDs), &models.Request{})
	if err != nil {
		logger.WarnContext(ctx, "failed to batch-fetch requests for time-to-solve", "error", err)
		return 0
	}

	var solveTimes []float64
	for _, m := range reqMap {
		r := m.(*models.Request)
		if r.State != models.RequestState_REQUEST_STATE_FULFILLED {
			continue
		}
		if r.FulfilledAtUnixSec == nil || *r.FulfilledAtUnixSec <= 0 {
			continue
		}
		shared, ok := sharedAt[r.Id]
		if !ok || shared <= 0 {
			continue
		}
		diff := *r.FulfilledAtUnixSec - shared
		if diff <= 0 {
			continue
		}
		solveTimes = append(solveTimes, float64(diff)/60)
	}

	if len(solveTimes) == 0 {
		return 0
	}

	sort.Float64s(solveTimes)
	n := len(solveTimes)
	median := solveTimes[n/2]
	if n%2 == 0 {
		median = (solveTimes[n/2-1] + solveTimes[n/2]) / 2
	}
	return float32(median)
}
