package impact_metrics

import (
	"context"
	"fmt"
	"math"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// UserActivityCounts represents activity counts for a user.
type UserActivityCounts struct {
	ItemsShared        int32
	CommunityCount     int32
	ActiveLoans        int32
	CompletedLoans     int32
	TotalLoans         int32
	ActiveBorrows      int32
	CompletedBorrows   int32
	TotalBorrows       int32
	CompletedGiveaways int32
	FulfilledRequests  int32
	EventsCreated      int32
}

// UserTotalValueResult represents the total value of a user's items.
type UserTotalValueResult struct {
	TotalValueUsd float32
	GearCount     int32
}

// CalculateUserActivityCounts calculates activity counts for a user across all their contributions.
func (c *Calculator) CalculateUserActivityCounts(ctx context.Context, userID string) (*UserActivityCounts, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CalculateUserActivityCounts",
		"user_id", userID,
	)

	logger.DebugContext(ctx, "calculating user activity counts")

	counts := &UserActivityCounts{}

	// Count gear items owned by the user
	gearItems, err := storage.QueryByField[*models.Gear](c.storage, ctx, "owner_id", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user gear: %w", err)
	}

	// Track unique communities and count non-deleted gear
	communityIDs := make(map[string]bool)
	for _, gear := range gearItems {
		// Skip soft-deleted gear
		if gear.Deleted != nil {
			continue
		}
		counts.ItemsShared++

		// Track communities via CommunityGear junction
		communityGear, cgErr := storage.QueryByField[*models.CommunityGear](c.storage, ctx, "gear_id", gear.Id)
		if cgErr != nil {
			logger.DebugContext(ctx, "failed to query community gear", "gear_id", gear.Id, "error", cgErr)
			continue
		}
		for _, cg := range communityGear {
			communityIDs[cg.CommunityId] = true
		}
	}
	// #nosec G115 - len(map) will not overflow int32 in practice
	counts.CommunityCount = int32(len(communityIDs))

	// Count loans (items lent to others) from Transfer table
	loans, err := storage.QueryByField[*models.Transfer](c.storage, ctx, "owner_id", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user loans: %w", err)
	}

	for _, transfer := range loans {
		// Skip soft-deleted transfers
		if transfer.Deleted != nil {
			continue
		}

		switch transfer.TransferType {
		case models.TransferType_TRANSFER_TYPE_LOAN:
			switch transfer.State {
			case models.TransferState_TRANSFER_STATE_ACTIVE:
				counts.ActiveLoans++
			case models.TransferState_TRANSFER_STATE_COMPLETED:
				counts.CompletedLoans++
			}
		case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
			if transfer.State == models.TransferState_TRANSFER_STATE_COMPLETED {
				counts.CompletedGiveaways++
			}
		}
	}
	counts.TotalLoans = counts.ActiveLoans + counts.CompletedLoans

	// Count borrows (items borrowed from others) - query by recipient_id
	borrows, err := storage.QueryByField[*models.Transfer](c.storage, ctx, "recipient_id", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user borrows: %w", err)
	}

	for _, transfer := range borrows {
		// Skip soft-deleted transfers
		if transfer.Deleted != nil {
			continue
		}

		// Only count loans (not giveaways) for borrows
		if transfer.TransferType == models.TransferType_TRANSFER_TYPE_LOAN {
			switch transfer.State {
			case models.TransferState_TRANSFER_STATE_ACTIVE:
				counts.ActiveBorrows++
			case models.TransferState_TRANSFER_STATE_COMPLETED:
				counts.CompletedBorrows++
			}
		}
	}
	counts.TotalBorrows = counts.ActiveBorrows + counts.CompletedBorrows

	// Count fulfilled requests (help offered by user)
	requests, err := storage.QueryByField[*models.Request](c.storage, ctx, "requester_id", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user requests: %w", err)
	}

	for _, request := range requests {
		// Skip soft-deleted requests
		if request.Deleted != nil {
			continue
		}

		if request.State == models.RequestState_REQUEST_STATE_FULFILLED {
			counts.FulfilledRequests++
		}
	}

	// Count events created by user
	experiences, err := storage.QueryByField[*models.Experience](c.storage, ctx, "owner_id", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user experiences: %w", err)
	}

	for _, experience := range experiences {
		// Skip soft-deleted experiences
		if experience.Deleted == nil {
			counts.EventsCreated++
		}
	}

	logger.InfoContext(ctx, "calculated user activity counts",
		"items_shared", counts.ItemsShared,
		"community_count", counts.CommunityCount,
		"total_loans", counts.TotalLoans,
		"total_borrows", counts.TotalBorrows,
	)

	return counts, nil
}

// CalculateUserImpactSavings calculates aggregated savings metrics for a user.
func (c *Calculator) CalculateUserImpactSavings(ctx context.Context, userID string) (*ImpactSavingsResult, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CalculateUserImpactSavings",
		"user_id", userID,
	)

	logger.DebugContext(ctx, "calculating user impact savings")

	// Query all transfers where user is the owner (items lent to others)
	transfers, err := storage.QueryByField[*models.Transfer](c.storage, ctx, "owner_id", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user transfers: %w", err)
	}

	// Aggregate savings across all transfers
	var costSavings, carbonSavings, timeFromLoans []float64
	costCount := 0
	carbonCount := 0

	for _, transfer := range transfers {
		// Skip soft-deleted transfers
		if transfer.Deleted != nil {
			continue
		}

		// Only count completed loans for impact savings
		if transfer.TransferType != models.TransferType_TRANSFER_TYPE_LOAN ||
			transfer.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}

		// Get impact estimate for this transfer
		impactEstimate := c.transferImpact(transfer)
		if impactEstimate == nil {
			continue
		}

		// Aggregate cost savings
		if impactEstimate.MoneySaved != nil && impactEstimate.MoneySaved.ValueUsd != nil && impactEstimate.MoneySaved.ValueUsd.Mean > 0 {
			costSavings = append(costSavings, float64(impactEstimate.MoneySaved.ValueUsd.Mean))
			costCount++
		}

		// Aggregate carbon savings (sum of manufacture avoided + waste reduced)
		if impactEstimate.EmissionsPrevented != nil {
			var carbonSum float64
			if impactEstimate.EmissionsPrevented.ManufactureAvoidedCarbon != nil && impactEstimate.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams != nil {
				carbonSum += float64(impactEstimate.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams.Mean)
			}
			if impactEstimate.EmissionsPrevented.WasteReducedCarbon != nil && impactEstimate.EmissionsPrevented.WasteReducedCarbon.Co2EGrams != nil {
				carbonSum += float64(impactEstimate.EmissionsPrevented.WasteReducedCarbon.Co2EGrams.Mean)
			}
			if carbonSum > 0 {
				carbonSavings = append(carbonSavings, carbonSum)
				carbonCount++
			}
		}

		// Aggregate time from loans
		if impactEstimate.TimeSaved != nil && impactEstimate.TimeSaved.Minutes != nil && impactEstimate.TimeSaved.Minutes.Mean > 0 {
			timeFromLoans = append(timeFromLoans, float64(impactEstimate.TimeSaved.Minutes.Mean))
		}
	}

	// Query requests fulfilled by user for time from requests
	impactRequests, err := storage.QueryByField[*models.Request](c.storage, ctx, "requester_id", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user requests: %w", err)
	}

	var timeFromRequests []float64
	for _, request := range impactRequests {
		// Skip soft-deleted or non-fulfilled requests
		if request.Deleted != nil || request.State != models.RequestState_REQUEST_STATE_FULFILLED {
			continue
		}

		impactEstimate := c.requestImpact(request)
		if impactEstimate != nil && impactEstimate.TimeSaved != nil && impactEstimate.TimeSaved.Minutes != nil && impactEstimate.TimeSaved.Minutes.Mean > 0 {
			timeFromRequests = append(timeFromRequests, float64(impactEstimate.TimeSaved.Minutes.Mean))
		}
	}

	// Query experiences created by user for time from skills
	impactExperiences, err := storage.QueryByField[*models.Experience](c.storage, ctx, "owner_id", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user experiences: %w", err)
	}

	var timeFromSkills []float64
	for _, experience := range impactExperiences {
		// Skip soft-deleted experiences
		if experience.Deleted != nil {
			continue
		}

		impactEstimate := c.experienceImpact(experience)
		if impactEstimate != nil && impactEstimate.TimeSaved != nil && impactEstimate.TimeSaved.Minutes != nil && impactEstimate.TimeSaved.Minutes.Mean > 0 {
			timeFromSkills = append(timeFromSkills, float64(impactEstimate.TimeSaved.Minutes.Mean))
		}
	}

	// Combine time sources using quadrature
	timeBanked := aggregateWithQuadrature(append(append(timeFromLoans, timeFromRequests...), timeFromSkills...))
	timeLoans := aggregateWithQuadrature(timeFromLoans)
	timeRequests := aggregateWithQuadrature(timeFromRequests)
	timeSkills := aggregateWithQuadrature(timeFromSkills)

	result := &ImpactSavingsResult{
		CostSavings:      aggregateWithQuadrature(costSavings),
		CostCount:        int32(costCount),
		CarbonSavings:    aggregateWithQuadrature(carbonSavings),
		CarbonCount:      int32(carbonCount),
		TimeBanked:       timeBanked,
		TimeFromLoans:    timeLoans,
		TimeFromRequests: timeRequests,
		TimeFromSkills:   timeSkills,
	}

	logger.InfoContext(ctx, "calculated user impact savings",
		"cost_savings_mean", result.CostSavings.Mean,
		"cost_count", result.CostCount,
		"carbon_savings_mean", result.CarbonSavings.Mean,
		"carbon_count", result.CarbonCount,
		"time_banked_mean", result.TimeBanked.Mean,
	)

	return result, nil
}

// CalculateUserTotalValue calculates the total value of all items owned by a user.
func (c *Calculator) CalculateUserTotalValue(ctx context.Context, userID string) (*UserTotalValueResult, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CalculateUserTotalValue",
		"user_id", userID,
	)

	logger.DebugContext(ctx, "calculating user total value")

	// Query all gear owned by user
	gearForValue, err := storage.QueryByField[*models.Gear](c.storage, ctx, "owner_id", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user gear: %w", err)
	}

	var totalValue float32
	var gearCount int32

	for _, gear := range gearForValue {
		// Skip soft-deleted gear
		if gear.Deleted != nil {
			continue
		}

		// Add value estimate if available
		if gear.ValueEstimate != nil && gear.ValueEstimate.EstimatedValueUsd > 0 {
			totalValue += gear.ValueEstimate.EstimatedValueUsd
			gearCount++
		}
	}

	result := &UserTotalValueResult{
		TotalValueUsd: totalValue,
		GearCount:     gearCount,
	}

	logger.InfoContext(ctx, "calculated user total value",
		"total_value_usd", result.TotalValueUsd,
		"gear_count", result.GearCount,
	)

	return result, nil
}

// aggregateWithQuadrature aggregates estimates using quadrature summation for uncertainty.
// This is a helper function that combines multiple estimates with their uncertainties.
func aggregateWithQuadrature(values []float64) *api.Estimate {
	if len(values) == 0 {
		return &api.Estimate{Mean: 0, Stddev: 0}
	}

	// Sum of means
	var meanSum float64
	for _, v := range values {
		meanSum += v
	}

	// For stddev, use quadrature (sqrt of sum of squares)
	// Assuming each value has proportional uncertainty (30% is a reasonable default)
	const proportionalUncertainty = 0.3
	var varianceSum float64
	for _, v := range values {
		stddev := v * proportionalUncertainty
		varianceSum += stddev * stddev
	}

	return &api.Estimate{
		Mean:   float32(meanSum),
		Stddev: float32(math.Sqrt(varianceSum)),
	}
}

// GetUser fetches a user by ID from storage.
func (c *Calculator) GetUser(ctx context.Context, userID string) (*models.User, error) {
	user := &models.User{}
	err := c.storage.GetByID(ctx, userID, user)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return user, nil
}

// GetLocation fetches a location by ID from storage.
func (c *Calculator) GetLocation(ctx context.Context, locationID string) (*models.Location, error) {
	location := &models.Location{}
	err := c.storage.GetByID(ctx, locationID, location)
	if err != nil {
		return nil, fmt.Errorf("failed to get location: %w", err)
	}
	return location, nil
}
