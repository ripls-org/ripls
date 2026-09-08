package impact_metrics

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/api"
	impactlib "go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
)

// getCommunityActsDetail returns the per-category and per-month
// breakdown of community acts.
func getCommunityActsDetail(
	ctx context.Context,
	s *Service,
	req *connect.Request[api.GetCommunityActsDetailRequest],
) (*connect.Response[api.GetCommunityActsDetailResponse], error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetCommunityActsDetail",
		"community_id", req.Msg.CommunityId,
	)

	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			fmt.Errorf("community_id is required"),
		)
	}

	if _, err := auth.RequireActiveCommunity(ctx, s.calculator.Storage(), req.Msg.CommunityId); err != nil {
		return nil, err
	}

	breakdown, err := s.calculator.CountActs(ctx, req.Msg.CommunityId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "getCommunityActsDetail", err,
			"detail", "failed to count community acts")
	}

	monthly, err := s.calculator.CountActsByMonth(ctx, req.Msg.CommunityId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "getCommunityActsDetail", err,
			"detail", "failed to bucket community acts by month")
	}

	recentItems, err := s.metricDetailCalculator.ComputeRecentItemsForActs(ctx, req.Msg.CommunityId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "getCommunityActsDetail", err,
			"detail", "failed to compute recent acts items")
	}

	resp := &api.GetCommunityActsDetailResponse{
		TotalCount:  breakdown.Total,
		Categories:  actsCategoriesToAPI(breakdown),
		MonthlyBars: monthlyActsToAPI(monthly),
		RecentItems: recentItems,
	}
	logger.DebugContext(ctx, "acts detail computed",
		"total", breakdown.Total,
		"category_rows", len(resp.Categories),
		"monthly_rows", len(resp.MonthlyBars),
		"recent_items_count", len(resp.RecentItems),
	)
	return connect.NewResponse(resp), nil
}

func actsCategoriesToAPI(b *impactlib.ActsBreakdown) []*api.ActsCategoryBreakdown {
	out := make([]*api.ActsCategoryBreakdown, 0, len(impactlib.ActsCategoryOrder))
	for _, cat := range impactlib.ActsCategoryOrder {
		out = append(out, &api.ActsCategoryBreakdown{
			Key:   string(cat),
			Label: impactlib.ActsCategoryLabel[cat],
			Count: b.ByCategory[cat],
		})
	}
	return out
}

func monthlyActsToAPI(months []impactlib.MonthlyActs) []*api.TimeSeriesPoint {
	out := make([]*api.TimeSeriesPoint, 0, len(months))
	for _, m := range months {
		bucketStart := time.Date(m.Year, m.Month, 1, 0, 0, 0, 0, time.UTC)
		out = append(out, &api.TimeSeriesPoint{
			Value:              float64(m.Count),
			BucketStartUnixSec: proto.Int64(bucketStart.Unix()),
		})
	}
	return out
}
