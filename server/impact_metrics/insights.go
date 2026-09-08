package impact_metrics

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// insightCandidate holds one candidate insight with a priority score.
type insightCandidate struct {
	text     string
	priority float64
}

// GenerateInsights loads a one-shot dataset and delegates to
// GenerateInsightsFromDataset. Use the dataset variant directly when
// the caller already has a preloaded dataset.
func GenerateInsights(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	communityID string,
	totalValueUSD float64,
	gearCount int32,
) []string {
	ds, err := LoadCommunityDataset(ctx, store, communityID)
	if err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "insights: dataset load failed",
			"operation", "GenerateInsights",
			"community_id", communityID,
			"error", err,
		)
		return nil
	}
	return GenerateInsightsFromDataset(ctx, ds, totalValueUSD, gearCount)
}

// GenerateInsightsFromDataset produces 1–2 behavioral insight strings
// from a preloaded CommunityDataset. Insights surface non-obvious
// patterns (trends, top-item concentration, day patterns, idle items)
// rather than restating visible numbers. Pure compute — no DB access.
func GenerateInsightsFromDataset(
	ctx context.Context,
	ds *CommunityDataset,
	totalValueUSD float64,
	gearCount int32,
) []string {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GenerateInsights",
		"community_id", ds.CommunityID,
	)

	var candidates []insightCandidate

	// 1. Period-over-period trend.
	trendInsight := computeTrendInsight(ds.Transfers)
	if trendInsight != "" {
		candidates = append(candidates, insightCandidate{text: trendInsight, priority: 3.0})
	}

	// 2. Top item concentration.
	concInsight := computeConcentrationInsight(ds)
	if concInsight != "" {
		candidates = append(candidates, insightCandidate{text: concInsight, priority: 2.5})
	}

	// 3. Day pattern.
	dayInsight := computeDayPatternInsight(ds.Transfers)
	if dayInsight != "" {
		candidates = append(candidates, insightCandidate{text: dayInsight, priority: 2.0})
	}

	// 4. Idle items.
	idleInsight := computeIdleItemsInsight(ds.Transfers, totalValueUSD, gearCount)
	if idleInsight != "" {
		candidates = append(candidates, insightCandidate{text: idleInsight, priority: 1.5})
	}

	// 5. Fallback: anchoring comparison.
	if len(candidates) == 0 {
		if totalValueUSD > 0 {
			kg := totalValueUSD * 0.05
			trees := int(kg / 20)
			if trees > 0 {
				candidates = append(candidates, insightCandidate{
					text:     fmt.Sprintf("Your community has saved the equivalent of %d trees this year.", trees),
					priority: 0.5,
				})
			}
		}
	}

	// Sort by priority descending, return top 2.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].priority > candidates[j].priority
	})

	var result []string
	for i, c := range candidates {
		if i >= 2 {
			break
		}
		result = append(result, c.text)
	}

	logger.DebugContext(ctx, "generated insights", "count", len(result))
	return result
}

// computeTrendInsight compares current 30-day transaction count to prior 30 days.
func computeTrendInsight(transfers []*models.Transfer) string {
	now := time.Now()
	thirtyDaysAgo := now.AddDate(0, 0, -30)
	sixtyDaysAgo := now.AddDate(0, 0, -60)

	var currentCount, priorCount int
	for _, t := range transfers {
		if t.Deleted != nil || t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		if t.ActualReturnUnixSec == nil {
			continue
		}
		completedAt := time.Unix(*t.ActualReturnUnixSec, 0)
		if completedAt.After(thirtyDaysAgo) {
			currentCount++
		} else if completedAt.After(sixtyDaysAgo) {
			priorCount++
		}
	}

	if priorCount == 0 || currentCount == 0 {
		return ""
	}

	changePct := float64(currentCount-priorCount) / float64(priorCount) * 100
	if math.Abs(changePct) < 15 {
		return ""
	}

	if changePct > 0 {
		return fmt.Sprintf("Sharing is up %.0f%% this month compared to last.", changePct)
	}
	return fmt.Sprintf("Sharing is down %.0f%% this month — spring could bring a rebound.", math.Abs(changePct))
}

// computeConcentrationInsight checks if a few items dominate total savings.
func computeConcentrationInsight(ds *CommunityDataset) string {
	// Aggregate savings by gear ID using preloaded transfers.
	gearSavings := make(map[string]float64)
	gearNames := make(map[string]string)
	var totalSavings float64

	for _, t := range ds.Transfers {
		if t.Deleted != nil || t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		if t.ImpactEstimate == nil || t.ImpactEstimate.MoneySaved == nil {
			continue
		}
		saved := float64(t.ImpactEstimate.MoneySaved.ValueUsd.Mean)
		gearSavings[t.GearId] += saved
		totalSavings += saved

		if _, ok := gearNames[t.GearId]; !ok {
			if gear, found := ds.Gear[t.GearId]; found && gear != nil {
				gearNames[t.GearId] = gear.Name
			}
		}
	}

	if totalSavings <= 0 || len(gearSavings) < 3 {
		return ""
	}

	// Find the top item.
	var topGearID string
	var topSaved float64
	for gearID, saved := range gearSavings {
		if saved > topSaved {
			topSaved = saved
			topGearID = gearID
		}
	}

	pct := topSaved / totalSavings * 100
	if pct < 30 {
		return ""
	}

	name := gearNames[topGearID]
	if name == "" {
		name = "Your top item"
	}
	return fmt.Sprintf("%s accounts for %.0f%% of total savings.", name, pct)
}

// computeDayPatternInsight finds the busiest day of the week.
func computeDayPatternInsight(transfers []*models.Transfer) string {
	dayCounts := make(map[time.Weekday]int)
	var total int
	for _, t := range transfers {
		if t.Deleted != nil || t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		if t.ActualReturnUnixSec == nil {
			continue
		}
		completedAt := time.Unix(*t.ActualReturnUnixSec, 0)
		dayCounts[completedAt.Weekday()]++
		total++
	}

	if total < 10 {
		return ""
	}

	var busiestDay time.Weekday
	var busiestCount int
	for day, count := range dayCounts {
		if count > busiestCount {
			busiestCount = count
			busiestDay = day
		}
	}

	pct := float64(busiestCount) / float64(total) * 100
	if pct < 20 {
		return ""
	}

	dayName := busiestDay.String() + "s"
	return fmt.Sprintf("%s are your community's sharing day.", dayName)
}

// computeIdleItemsInsight counts items that have never been shared.
func computeIdleItemsInsight(
	transfers []*models.Transfer,
	totalValueUSD float64,
	gearCount int32,
) string {
	if gearCount < 5 {
		return ""
	}

	sharedGearIDs := make(map[string]bool)
	for _, t := range transfers {
		if t.Deleted != nil {
			continue
		}
		sharedGearIDs[t.GearId] = true
	}

	idleCount := int(gearCount) - len(sharedGearIDs)
	if idleCount < 3 {
		return ""
	}

	// Estimate potential value from idle items.
	if totalValueUSD > 0 && gearCount > 0 {
		avgValue := totalValueUSD / float64(gearCount)
		potentialSavings := avgValue * 0.075 * float64(idleCount)
		if potentialSavings >= 10 {
			return fmt.Sprintf("%d items haven't been shared yet — they could unlock $%.0f in potential savings.", idleCount, potentialSavings)
		}
	}

	return fmt.Sprintf("%d items haven't been shared yet. Listing them could start new sharing.", idleCount)
}
