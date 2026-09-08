package impact_metrics

import (
	"context"
	"fmt"
	"sort"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	impactlib "go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// getCommunityPercentileDetail computes the community's percentile rank for a
// given metric dimension and returns the top communities list.
func getCommunityPercentileDetail(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	cfg *estimator.Config,
	req *connect.Request[api.GetCommunityPercentileDetailRequest],
) (*connect.Response[api.GetCommunityPercentileDetailResponse], error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetCommunityPercentileDetail",
		"community_id", req.Msg.CommunityId,
		"dimension", req.Msg.Dimension.String(),
	)

	if _, err := auth.RequireAuth(ctx); err != nil {
		return nil, err
	}

	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}

	if _, err := auth.RequireActiveCommunity(ctx, store, req.Msg.CommunityId); err != nil {
		return nil, err
	}

	logger.DebugContext(ctx, "computing community percentile detail")

	// Batched: load all communities' datasets once (constant query
	// count regardless of community count). Replaces the previous
	// per-community CalculateImpactSavings loop. See #2055.
	datasets, err := impactlib.LoadAllCommunitiesDatasets(ctx, store)
	if err != nil {
		logger.ErrorContext(ctx, "failed to load all-communities datasets", "error", err)
		return nil, connecterr.Internal(ctx, "getCommunityPercentileDetail", err, "detail", "failed to load communities")
	}

	// Resolve display names via a second indexed lookup. Cheaper than
	// reading every Community proto through ListAll since we only need
	// the name + presence — but here we already need the Community row
	// for the name, so do one ListAll and join.
	communityMsgs, err := store.ListAll(ctx, &models.Community{})
	if err != nil {
		return nil, connecterr.Internal(ctx, "getCommunityPercentileDetail", err, "detail", "failed to query communities")
	}
	communityNames := make(map[string]string, len(communityMsgs))
	for _, m := range communityMsgs {
		c := m.(*models.Community)
		if c.Deleted == nil {
			communityNames[c.Id] = c.Name
		}
	}

	type communityMetric struct {
		id    string
		name  string
		value float64
	}

	calc := impactlib.NewCalculator(store, cfg)
	metrics := make([]communityMetric, 0, len(datasets))
	for id, ds := range datasets {
		val := aggregateValueFromDataset(ctx, calc, ds, req.Msg.Dimension)
		metrics = append(metrics, communityMetric{
			id:    id,
			name:  communityNames[id],
			value: val,
		})
	}

	if len(metrics) == 0 {
		return connect.NewResponse(&api.GetCommunityPercentileDetailResponse{}), nil
	}

	// Sort ascending by value for percentile computation.
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].value < metrics[j].value
	})

	// Find this community's rank and compute percentile.
	var thisRank int32
	var thisValue float64
	for i, m := range metrics {
		if m.id == req.Msg.CommunityId {
			thisRank = int32(len(metrics) - i) // 1-indexed, descending
			thisValue = m.value
			break
		}
	}

	percentile := int32(0)
	if len(metrics) > 1 {
		// Number of communities below this one / total * 100.
		below := 0
		for _, m := range metrics {
			if m.value < thisValue {
				below++
			}
		}
		percentile = int32(float64(below) / float64(len(metrics)) * 100)
	}

	// Build top 10 list (sorted descending).
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].value > metrics[j].value
	})

	limit := 10
	if len(metrics) < limit {
		limit = len(metrics)
	}
	topCommunities := make([]*api.RankedCommunity, 0, limit)
	for i := range limit {
		topCommunities = append(topCommunities, &api.RankedCommunity{
			Rank:            int32(i + 1),
			CommunityName:   metrics[i].name,
			FormattedValue:  formatMetricValue(metrics[i].value, req.Msg.Dimension),
			IsThisCommunity: metrics[i].id == req.Msg.CommunityId,
		})
	}

	logger.InfoContext(ctx, "computed community percentile",
		"percentile", percentile,
		"rank", thisRank,
		"total_communities", len(metrics),
	)

	return connect.NewResponse(&api.GetCommunityPercentileDetailResponse{
		CurrentPercentile:  percentile,
		TopCommunities:     topCommunities,
		ThisCommunityRank:  thisRank,
		ThisCommunityValue: formatMetricValue(thisValue, req.Msg.Dimension),
	}), nil
}

// aggregateValueFromDataset returns the aggregate metric value for a
// preloaded community dataset in the given dimension. Pure compute —
// no DB access. Replaces the previous aggregateValueForDimension shape
// that queried per call. See #2055.
func aggregateValueFromDataset(
	ctx context.Context,
	calc *impactlib.Calculator,
	ds *impactlib.CommunityDataset,
	dimension api.ImpactMetricDimension,
) float64 {
	savings := calc.CalculateImpactSavingsFromDataset(ctx, ds)
	switch dimension {
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY:
		return float64(savings.CostSavings.Mean)
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS:
		return float64(savings.CarbonSavings.Mean)
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME:
		if savings.QualityTime != nil {
			return float64(savings.QualityTime.Mean)
		}
	}
	return 0
}

// formatMetricValue formats a raw metric value for display based on dimension.
func formatMetricValue(value float64, dimension api.ImpactMetricDimension) string {
	switch dimension {
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY:
		dollars := int(value)
		if dollars < 1000 {
			return fmt.Sprintf("$%d", dollars)
		}
		return fmt.Sprintf("$%.1fK", float64(dollars)/1000)
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS:
		kg := value / 1000
		if kg < 1 {
			return fmt.Sprintf("%.0f g", value)
		}
		if kg < 1000 {
			return fmt.Sprintf("%.0f kg", kg)
		}
		return fmt.Sprintf("%.1ft", kg/1000)
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME:
		hours := value / 60
		if hours < 1 {
			return fmt.Sprintf("%.0f min", value)
		}
		return fmt.Sprintf("%.1f hrs", hours)
	default:
		return fmt.Sprintf("%.0f", value)
	}
}

// computePercentiles calculates percentile values for all three
// dimensions for a specific community. Returns money, co2,
// quality_time percentiles (0–100).
//
// Batched: loads every community's source rows once via
// LoadAllCommunitiesDatasets (constant query count regardless of N
// communities), then reduces each in memory with the pure
// CalculateImpactSavingsFromDataset. Replaces the previous N×6
// per-community CalculateImpactSavings loop. See #2055.
func computePercentiles(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	cfg *estimator.Config,
	communityID string,
) (money, co2, qt int32, err error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "computePercentiles",
		"community_id", communityID,
	)

	datasets, err := impactlib.LoadAllCommunitiesDatasets(ctx, store)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to load all-communities datasets: %w", err)
	}

	type vals struct {
		money float64
		co2   float64
		qt    float64
	}
	calc := impactlib.NewCalculator(store, cfg)
	communityVals := make(map[string]vals, len(datasets))
	for id, ds := range datasets {
		savings := calc.CalculateImpactSavingsFromDataset(ctx, ds)
		v := vals{
			money: float64(savings.CostSavings.Mean),
			co2:   float64(savings.CarbonSavings.Mean),
		}
		if savings.QualityTime != nil {
			v.qt = float64(savings.QualityTime.Mean)
		}
		communityVals[id] = v
	}

	thisVals, ok := communityVals[communityID]
	if !ok {
		return 0, 0, 0, nil
	}

	total := len(communityVals)
	if total <= 1 {
		return 0, 0, 0, nil
	}

	countBelow := func(thisVal float64, extract func(vals) float64) int32 {
		below := 0
		for _, v := range communityVals {
			if extract(v) < thisVal {
				below++
			}
		}
		return int32(float64(below) / float64(total) * 100)
	}

	money = countBelow(thisVals.money, func(v vals) float64 { return v.money })
	co2 = countBelow(thisVals.co2, func(v vals) float64 { return v.co2 })
	qt = countBelow(thisVals.qt, func(v vals) float64 { return v.qt })

	logger.InfoContext(ctx, "computed percentiles",
		"money", money, "co2", co2, "quality_time", qt,
		"community_count", total,
	)

	return money, co2, qt, nil
}
