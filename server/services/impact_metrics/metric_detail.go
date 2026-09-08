package impact_metrics

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
)

// getCommunityMetricDetail returns detailed breakdown data for a specific metric dimension.
func getCommunityMetricDetail(
	ctx context.Context,
	s *Service,
	req *connect.Request[api.GetCommunityMetricDetailRequest],
) (*connect.Response[api.GetCommunityMetricDetailResponse], error) {
	startTime := time.Now()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetCommunityMetricDetail",
		"community_id", req.Msg.CommunityId,
		"dimension", req.Msg.Dimension.String(),
		"period", req.Msg.Period.String(),
	)

	// Validate request
	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}
	if req.Msg.Dimension == api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("dimension is required"))
	}
	if req.Msg.Period == api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("period is required"))
	}

	if _, err := auth.RequireActiveCommunity(ctx, s.calculator.Storage(), req.Msg.CommunityId); err != nil {
		return nil, err
	}

	logger.DebugContext(ctx, "computing metric detail")

	// Compute metric detail using the calculator
	result, err := s.metricDetailCalculator.ComputeMetricDetail(
		ctx,
		req.Msg.CommunityId,
		req.Msg.Dimension,
		req.Msg.Period,
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to compute metric detail", "error", err)
		return nil, connecterr.Internal(ctx, "getCommunityMetricDetail", err, "detail", "failed to compute metric detail")
	}

	response := &api.GetCommunityMetricDetailResponse{
		Dimension:                req.Msg.Dimension,
		TotalValue:               result.TotalValue,
		FormattedTotal:           result.FormattedTotal,
		SourceBreakdown:          result.SourceBreakdown,
		CumulativeTrend:          result.CumulativeTrend,
		Factors:                  result.Factors,
		TopItems:                 result.TopItems,
		TopContributors:          result.TopContributors,
		MoneyDetail:              result.MoneyDetail,
		TimeDetail:               result.TimeDetail,
		LibraryItems:             result.LibraryItems,
		LibraryValueTrend:        result.LibraryValueTrend,
		LibraryItemCountTrend:    result.LibraryItemCountTrend,
		EmissionsPreventedDetail: result.EmissionsPreventedDetail,
		QualityTimeDetail:        result.QualityTimeDetail,
		Comparison:               result.Comparison,
		RecentItems:              result.RecentItems,
	}

	durationMs := time.Since(startTime).Milliseconds()
	logger.InfoContext(ctx, "metric detail computed successfully",
		"dimension", req.Msg.Dimension.String(),
		"period", req.Msg.Period.String(),
		"total_value", result.TotalValue.Mean,
		"breakdown_count", len(result.SourceBreakdown),
		"recent_items_count", len(result.RecentItems),
		"duration_ms", durationMs,
	)

	return connect.NewResponse(response), nil
}
