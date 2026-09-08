package impact_metrics

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/api"
	impactlib "go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
)

// getUserImpactMetrics calculates and returns impact metrics for a user.
func getUserImpactMetrics(
	ctx context.Context,
	calculator *impactlib.Calculator,
	req *connect.Request[api.GetUserImpactMetricsRequest],
) (*connect.Response[api.GetUserImpactMetricsResponse], error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetUserImpactMetrics",
		"user_id", req.Msg.UserId,
	)

	// Validate request
	if req.Msg.UserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id is required"))
	}

	logger.DebugContext(ctx, "calculating user impact metrics")

	// Calculate activity counts for the user
	activityCounts, err := calculator.CalculateUserActivityCounts(ctx, req.Msg.UserId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to calculate user activity counts", "error", err)
		return nil, connecterr.Internal(ctx, "getUserImpactMetrics", err, "detail",

			// Calculate impact savings for the user
			"failed to calculate activity counts")
	}

	savings, err := calculator.CalculateUserImpactSavings(ctx, req.Msg.UserId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to calculate user impact savings", "error", err)
		return nil, connecterr.Internal(ctx, "getUserImpactMetrics", err, "detail",

			// Calculate total value of user's items
			"failed to calculate impact savings")
	}

	totalValue, err := calculator.CalculateUserTotalValue(ctx, req.Msg.UserId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to calculate user total value", "error", err)
		return nil, connecterr.Internal(ctx, "getUserImpactMetrics", err, "detail",

			// Get user profile for display information
			"failed to calculate total value")
	}

	user, err := calculator.GetUser(ctx, req.Msg.UserId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get user profile", "error", err)
		return nil, connecterr.Internal(ctx, "getUserImpactMetrics", err, "detail",

			// Get primary location name
			"failed to get user profile")
	}

	var locationName string
	if user.PrimaryResidenceLocationId != "" {
		location, err := calculator.GetLocation(ctx, user.PrimaryResidenceLocationId)
		if err != nil {
			logger.WarnContext(ctx, "failed to get user location", "location_id", user.PrimaryResidenceLocationId, "error", err)
			// Non-critical: continue without location
		} else if location != nil {
			locationName = location.GetName()
		}
	}

	// Get primary media ID
	var mediaID string
	if len(user.MediaIds) > 0 {
		mediaID = user.MediaIds[0]
	}

	qtMean := float32(0)
	if savings.QualityTime != nil {
		qtMean = savings.QualityTime.Mean
	}

	logger.InfoContext(ctx, "calculated user impact metrics",
		"items_shared", activityCounts.ItemsShared,
		"community_count", activityCounts.CommunityCount,
		"total_loans", activityCounts.TotalLoans,
		"total_borrows", activityCounts.TotalBorrows,
		"cost_savings_mean_usd", savings.CostSavings.Mean,
		"cost_savings_count", savings.CostCount,
		"carbon_savings_mean_grams", savings.CarbonSavings.Mean,
		"carbon_savings_count", savings.CarbonCount,
		"time_banked_mean_minutes", savings.TimeBanked.Mean,
		"quality_time_minutes_mean", qtMean,
		"total_value_usd", totalValue.TotalValueUsd,
		"user_name", user.Name,
		"user_location", locationName,
	)

	// Require authentication and verify the caller shares a community with the
	// target user (or IS the target user).
	authUser, authErr := auth.RequireAuth(ctx)
	if authErr != nil {
		return nil, authErr
	}
	if err := auth.RequireSharedCommunityWithUser(ctx, calculator.Storage(), authUser.UserID, req.Msg.UserId); err != nil {
		return nil, err
	}
	isOwnProfile := authUser.UserID == req.Msg.UserId

	// Compute source breakdowns, chart data, direction breakdown, and community ranking.
	moneyBD, co2BD, qtBD := userSourceBreakdowns(ctx, calculator.Storage(), req.Msg.UserId, logger)
	moneyTrend := userMoneyTrend(ctx, calculator.Storage(), req.Msg.UserId, logger)
	co2Trend := userCo2Trend(ctx, calculator.Storage(), req.Msg.UserId, logger)
	qtTrend := userQualityTimeTrend(ctx, calculator.Storage(), req.Msg.UserId, logger)
	rankedCommunities := userCommunityRanking(ctx, calculator.Storage(), req.Msg.UserId, logger)

	// Convert to API response format
	response := &api.GetUserImpactMetricsResponse{
		UserName:                   user.Name,
		UserDescription:            user.Description,
		UserLocationName:           locationName,
		UserMediaId:                mediaID,
		IsOwnProfile:               isOwnProfile,
		SourceBreakdownMoney:       moneyBD,
		SourceBreakdownCo2:         co2BD,
		SourceBreakdownQualityTime: qtBD,
		MoneyTrend:                 moneyTrend,
		Co2Trend:                   co2Trend,
		QualityTimeTrend:           qtTrend,
		RankedCommunities:          rankedCommunities,
		Metrics: &api.UserImpactMetrics{
			ItemsShared:             activityCounts.ItemsShared,
			CommunityCount:          activityCounts.CommunityCount,
			ActiveLoans:             activityCounts.ActiveLoans,
			CompletedLoans:          activityCounts.CompletedLoans,
			TotalLoans:              activityCounts.TotalLoans,
			ActiveBorrows:           activityCounts.ActiveBorrows,
			CompletedBorrows:        activityCounts.CompletedBorrows,
			TotalBorrows:            activityCounts.TotalBorrows,
			CompletedGiveaways:      activityCounts.CompletedGiveaways,
			FulfilledRequests:       activityCounts.FulfilledRequests,
			EventsCreated:           activityCounts.EventsCreated,
			TotalValueUsd:           totalValue.TotalValueUsd,
			CostSavingsUsd:          savings.CostSavings,
			CostSavingsCount:        savings.CostCount,
			CarbonSavingsGrams:      savings.CarbonSavings,
			CarbonSavingsCount:      savings.CarbonCount,
			TimeBankedMinutes:       savings.TimeBanked,
			TimeFromLoansMinutes:    savings.TimeFromLoans,
			TimeFromSkillsMinutes:   savings.TimeFromSkills,
			TimeFromRequestsMinutes: savings.TimeFromRequests,
			QualityTimeMinutes:      savings.QualityTime,
		},
	}

	return connect.NewResponse(response), nil
}
