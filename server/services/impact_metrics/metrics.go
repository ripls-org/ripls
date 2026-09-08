package impact_metrics

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/api"
	impactlib "go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// getCommunityImpactMetrics calculates and returns impact metrics for a community.
func getCommunityImpactMetrics(
	ctx context.Context,
	calculator *impactlib.Calculator,
	req *connect.Request[api.GetCommunityImpactMetricsRequest],
) (*connect.Response[api.GetCommunityImpactMetricsResponse], error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetCommunityImpactMetrics",
		"community_id", req.Msg.CommunityId,
	)

	// Validate request
	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}

	// Attach query-count stats so we can observe the post-#2055 query
	// budget per request. Emitted as a Debug log at the bottom of the
	// handler.
	ctx = storage.WithQueryStats(ctx)
	handlerStart := time.Now()

	if _, err := auth.RequireActiveCommunity(ctx, calculator.Storage(), req.Msg.CommunityId); err != nil {
		return nil, err
	}

	logger.DebugContext(ctx, "calculating community impact metrics")

	// Preload the community's source rows once and thread the dataset
	// pointer through every Calculate*/Generate* call. Replaces the
	// per-function fan-out that previously issued the same QueryByField
	// 2–4 times within a single handler call. See #2055.
	dataset, err := impactlib.LoadCommunityDataset(ctx, calculator.Storage(), req.Msg.CommunityId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to load community dataset", "error", err)
		return nil, connecterr.Internal(ctx, "getCommunityImpactMetrics", err, "detail", "failed to load community dataset")
	}

	activityCounts := calculator.CalculateActivityCountsFromDataset(ctx, dataset)
	savings := calculator.CalculateImpactSavingsFromDataset(ctx, dataset)
	totalValue := calculator.CalculateTotalValueFromDataset(ctx, dataset)

	qtMean := float32(0)
	if savings.QualityTime != nil {
		qtMean = savings.QualityTime.Mean
	}

	logger.InfoContext(ctx, "calculated impact metrics",
		"cost_savings_mean_usd", savings.CostSavings.Mean,
		"cost_savings_count", savings.CostCount,
		"carbon_savings_mean_grams", savings.CarbonSavings.Mean,
		"carbon_savings_count", savings.CarbonCount,
		"time_banked_mean_minutes", savings.TimeBanked.Mean,
		"quality_time_minutes_mean", qtMean,
		"total_value_usd", totalValue.TotalValueUsd,
		"total_value_gear_count", totalValue.GearCount,
	)

	co2Potential := calculator.CalculateCo2PotentialFromDataset(ctx, dataset)

	var qtPerPersonMinutes float32
	if activityCounts.MemberCount > 0 && savings.QualityTime != nil {
		qtPerPersonMinutes = savings.QualityTime.Mean / float32(activityCounts.MemberCount)
	}

	timeToSolveMinutes := calculator.CalculateTimeToSolveMedianFromDataset(ctx, dataset)

	pctMoney, pctCo2, pctQt, err := computePercentiles(
		ctx, calculator.Storage(), calculator.EstimatorConfig(), req.Msg.CommunityId,
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to compute percentiles", "error", err)
		// Non-fatal: continue with zero percentiles.
		pctMoney, pctCo2, pctQt = 0, 0, 0
	}

	// Generate insights from the same preloaded dataset.
	insights := impactlib.GenerateInsightsFromDataset(
		ctx,
		dataset,
		float64(totalValue.TotalValueUsd),
		activityCounts.GearCount,
	)

	// Convert to API response format
	response := &api.GetCommunityImpactMetricsResponse{
		Co2PotentialGrams:           &co2Potential,
		QualityTimePerPersonMinutes: &qtPerPersonMinutes,
		TimeToSolveMinutes:          &timeToSolveMinutes,
		PercentileMoney:             &pctMoney,
		PercentileCo2:               &pctCo2,
		PercentileQualityTime:       &pctQt,
		Insights:                    insights,
		Metrics: &api.CommunityImpactMetrics{
			GearCount:               activityCounts.GearCount,
			ActiveLoans:             activityCounts.ActiveLoans,
			CompletedLoans:          activityCounts.CompletedLoans,
			TotalLoans:              activityCounts.ActiveLoans + activityCounts.CompletedLoans,
			OpenGiveaways:           activityCounts.OpenGiveaways,
			CompletedGiveaways:      activityCounts.CompletedGiveaways,
			OpenRequests:            activityCounts.OpenRequests,
			FulfilledRequests:       activityCounts.FulfilledRequests,
			UpcomingEvents:          activityCounts.UpcomingEvents,
			PastEvents:              activityCounts.PastEvents,
			TotalEvents:             activityCounts.UpcomingEvents + activityCounts.PastEvents,
			MemberCount:             activityCounts.MemberCount,
			CostSavingsUsd:          savings.CostSavings,
			CostSavingsCount:        savings.CostCount,
			CarbonSavingsGrams:      savings.CarbonSavings,
			CarbonSavingsCount:      savings.CarbonCount,
			TimeBankedMinutes:       savings.TimeBanked,
			TimeFromLoansMinutes:    savings.TimeFromLoans,
			TimeFromSkillsMinutes:   savings.TimeFromSkills,
			TimeFromRequestsMinutes: savings.TimeFromRequests,
			TotalValueUsd:           totalValue.TotalValueUsd,
			QualityTimeMinutes:      savings.QualityTime,
		},
	}

	if stats := storage.GetQueryStats(ctx); stats != nil {
		logger.DebugContext(ctx, "metrics query stats",
			"query_count", stats.Count.Load(),
			"query_duration_ms", stats.TotalDuration().Milliseconds(),
			"duration_ms", time.Since(handlerStart).Milliseconds(),
		)
	}

	return connect.NewResponse(response), nil
}
