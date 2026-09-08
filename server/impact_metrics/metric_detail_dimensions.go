package impact_metrics

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// computeMoneyDetail computes Money dimension-specific detail fields.
// When ctx carries a preloaded gear map (set by
// ComputeMetricDetailFromDataset), the library-value pass becomes a
// pure in-memory aggregation; otherwise it falls back to the original
// QueryByField + per-row GetByID path. See #2055.
func (c *MetricDetailCalculator) computeMoneyDetail(
	ctx context.Context,
	communityID string,
	totalMoneySaved float32,
) (*api.MoneySavedDetail, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "computeMoneyDetail",
		"community_id", communityID,
	)

	// Load community gear once for the library-value pass. Uses the
	// preloaded dataset from ctx when available; falls back to
	// QueryByField otherwise.
	communityGear, err := resolveCommunityGear(ctx, c.storage, communityID)
	if err != nil {
		return nil, fmt.Errorf("failed to query community gear: %w", err)
	}

	var libraryValue float32
	for _, cg := range communityGear {
		var gear *models.Gear
		if g, ok := GearFromContext(ctx, cg.GearId); ok {
			gear = g
		} else {
			gear = &models.Gear{}
			if getErr := c.storage.GetByID(ctx, cg.GearId, gear); getErr != nil {
				logger.DebugContext(ctx, "skipping gear that could not be loaded",
					"gear_id", cg.GearId, "error", getErr)
				continue
			}
		}
		if gear.Deleted != nil {
			continue
		}
		if gear.ValueEstimate != nil {
			libraryValue += gear.ValueEstimate.EstimatedValueUsd
		}
	}

	// Compute utilization rate (avoid division by zero)
	utilizationRate := "0%"
	if libraryValue > 0 {
		rate := (totalMoneySaved / libraryValue) * 100
		utilizationRate = FormatPercentage(float64(rate))
	}

	// Generate summary text
	summaryText := fmt.Sprintf(
		"Your circle has saved %s by sharing instead of buying — that's %s of your total library put to work.",
		FormatMoney(float64(totalMoneySaved)),
		utilizationRate,
	)

	logger.DebugContext(ctx, "computed money detail",
		"library_value", libraryValue,
		"utilization_rate", utilizationRate,
	)

	return &api.MoneySavedDetail{
		LibraryValue:    FormatMoney(float64(libraryValue)),
		UtilizationRate: utilizationRate,
		SummaryText:     summaryText,
		CurrentUsd:      float64(totalMoneySaved),
	}, nil
}

// computeTimeDetail computes Time dimension-specific detail fields.
func (c *MetricDetailCalculator) computeTimeDetail(
	ctx context.Context,
	communityID string,
	totalMinutesSaved float32,
) (*api.TimeSavedDetail, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "computeTimeDetail",
		"community_id", communityID,
	)

	// Get member count for per-person calculation (prefer the
	// preloaded dataset when one is attached to ctx).
	communityMembers, err := resolveCommunityUsers(ctx, c.storage, communityID)
	if err != nil {
		return nil, fmt.Errorf("failed to query community members: %w", err)
	}
	memberCount := len(communityMembers)

	// Convert minutes to hours
	totalHours := int32(totalMinutesSaved / 60)

	// Per-person hours (avoid division by zero)
	perPersonHours := int32(0)
	if memberCount > 0 {
		perPersonHours = int32(float32(totalHours) / float32(memberCount))
	}

	// Get config values for time comparison
	borrowMinutes := int32(1) // Default: 1 minute to borrow
	shopMinutes := int32(45)  // Default: 45 minutes to shop
	if c.cfg.TimeDefaults != nil && c.cfg.TimeDefaults.GearShoppingTimeMinutes > 0 {
		shopMinutes = c.cfg.TimeDefaults.GearShoppingTimeMinutes
	}

	// Speed comparison
	speedComparison := fmt.Sprintf("%d× faster through your circle", shopMinutes/borrowMinutes)

	logger.DebugContext(ctx, "computed time detail",
		"total_hours", totalHours,
		"member_count", memberCount,
		"per_person_hours", perPersonHours,
	)

	return &api.TimeSavedDetail{
		TotalHours:      totalHours,
		PerPersonHours:  perPersonHours,
		BorrowMinutes:   borrowMinutes,
		ShopMinutes:     shopMinutes,
		SpeedComparison: speedComparison,
	}, nil
}

// computeCO2Detail computes CO2 dimension-specific detail fields.
func (c *MetricDetailCalculator) computeCO2Detail(
	totalGramsCO2 float32,
	monthlyBars []*api.TimeSeriesPoint,
	monthlyAverage float64,
) *api.EmissionsPreventedDetail {
	// Convert grams to kg
	currentKg := float64(totalGramsCO2 / 1000)

	// Goal: default to 75 kg if not set
	// (Config doesn't have AnnualCo2GoalKg field, so using default)
	goalKg := float64(75)

	// Generate equivalence text
	// Use simple tree equivalence: ~20kg CO2 per tree per year
	trees := int(currentKg / 20)
	equivalence := "0 trees for a year"
	if trees > 0 {
		if trees == 1 {
			equivalence = "1 tree for a year"
		} else {
			equivalence = fmt.Sprintf("%d trees for a year", trees)
		}
	}

	return &api.EmissionsPreventedDetail{
		CurrentKg:      currentKg,
		GoalKg:         goalKg,
		Equivalence:    equivalence,
		MonthlyBars:    monthlyBars,
		MonthlyAverage: monthlyAverage,
	}
}
