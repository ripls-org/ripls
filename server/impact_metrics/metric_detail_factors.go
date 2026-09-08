package impact_metrics

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// computeFactors computes contributing factor cards for each source type.
func (c *MetricDetailCalculator) computeFactors(
	ctx context.Context,
	transfers []*models.Transfer,
	requests []*models.Request,
	experiences []*models.Experience,
	dimension api.ImpactMetricDimension,
	period api.ImpactMetricPeriod,
	communityID string,
) ([]*api.ContributingFactor, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "computeFactors",
		"dimension", dimension.String(),
	)

	var factors []*api.ContributingFactor

	// Create entity cache for recent activity lookups
	cache := NewEntityCache(c.storage)

	// Separate transfers by type
	var loans, giveaways []*models.Transfer
	for _, transfer := range transfers {
		switch transfer.TransferType {
		case models.TransferType_TRANSFER_TYPE_LOAN:
			loans = append(loans, transfer)
		case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
			giveaways = append(giveaways, transfer)
		}
	}

	// For Money dimension, add Items factor first
	if dimension == api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY {
		itemsFactor, err := c.computeItemsFactor(ctx, communityID, dimension, period)
		if err != nil {
			logger.ErrorContext(ctx, "failed to compute items factor", "error", err)
			// Don't fail the whole request, just skip this factor
		} else if itemsFactor != nil {
			factors = append(factors, itemsFactor)
		}
	}

	// Loans factor
	if len(loans) > 0 {
		loansFactor := c.buildTransferFactor(ctx,
			api.ImpactSourceType_IMPACT_SOURCE_TYPE_LOANS,
			loans, dimension, period, cache)
		factors = append(factors, loansFactor)
	}

	// Giveaways factor. The time dimension reads this slot as help requests
	// rather than giveaways, which selects which side of the combined set the
	// recent-activity rows are drawn from.
	if len(giveaways) > 0 || len(requests) > 0 {
		source := api.ImpactSourceType_IMPACT_SOURCE_TYPE_GIVEAWAYS
		if dimension == api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_TIME {
			source = api.ImpactSourceType_IMPACT_SOURCE_TYPE_REQUESTS
		}
		// Combine giveaways and requests for this factor
		factor := c.buildHelpFactor(ctx, source, giveaways, requests, dimension, period, cache)
		factors = append(factors, factor)
	}

	// Events factor
	if len(experiences) > 0 {
		eventsFactor := c.buildExperiencesFactor(ctx,
			api.ImpactSourceType_IMPACT_SOURCE_TYPE_EVENTS,
			experiences, dimension, period, cache)
		factors = append(factors, eventsFactor)
	}

	logger.DebugContext(ctx, "computed factors", "count", len(factors))

	return factors, nil
}

// computeItemsFactor computes the Items factor for the Money dimension.
// This factor shows library value and item count, not transaction-level impact.
func (c *MetricDetailCalculator) computeItemsFactor(
	ctx context.Context,
	communityID string,
	dimension api.ImpactMetricDimension,
	period api.ImpactMetricPeriod,
) (*api.ContributingFactor, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "computeItemsFactor",
		"community_id", communityID,
	)

	// Load gear via CommunityGear junction — preloaded from dataset
	// when available.
	communityGear, err := resolveCommunityGear(ctx, c.storage, communityID)
	if err != nil {
		return nil, fmt.Errorf("failed to query community gear: %w", err)
	}

	var totalValue float32
	var gearCount int32
	var gearItems []*models.Gear // Track gear items for recent activity

	// Track CommunityGear records with timestamps for time-series
	var gearTimestamps []gearWithTimestamp

	// Collect gear values. Uses the preloaded gear map from context when
	// present (set by ComputeMetricDetailFromDataset) to avoid per-row
	// GetByID round-trips; falls back to DB lookup otherwise. See
	// #2055.
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
		gearCount++
		gearItems = append(gearItems, gear)
		if gear.ValueEstimate != nil {
			totalValue += gear.ValueEstimate.EstimatedValueUsd
		}

		// Track with timestamp for time-series
		gearTimestamps = append(gearTimestamps, gearWithTimestamp{
			gear:      gear,
			timestamp: cg.CreatedAtUnixSec,
		})
	}

	if gearCount == 0 {
		return nil, nil
	}

	// Compute recent items (Task 3)
	cache := NewEntityCache(c.storage)
	recentItems := c.computeRecentGearItems(ctx, gearItems, dimension, 3, cache)

	// Compute time-series using CommunityGear timestamps
	trend, volume, cumulative := c.computeGearTimeSeries(gearTimestamps, period)

	return &api.ContributingFactor{
		Count:       gearCount,
		RecentItems: recentItems,
		Trend:       trend,
		Volume:      volume,
		Cumulative:  cumulative,
	}, nil
}

// buildTransferFactor builds a factor for loans.
func (c *MetricDetailCalculator) buildTransferFactor(
	ctx context.Context,
	source api.ImpactSourceType,
	transfers []*models.Transfer,
	dimension api.ImpactMetricDimension,
	period api.ImpactMetricPeriod,
	cache *EntityCache,
) *api.ContributingFactor {
	count := int32(len(transfers))

	// Compute recent activity
	recentItems := c.computeRecentActivity(ctx, transfers, []*models.Request{}, []*models.Experience{}, source, dimension, cache)

	// Compute time-series data (Trend, Volume, Cumulative)
	trend, volume, cumulative := c.computeTransferSeries(transfers, dimension, period)

	return &api.ContributingFactor{
		Count:       count,
		RecentItems: recentItems,
		Trend:       trend,
		Volume:      volume,
		Cumulative:  cumulative,
	}
}

// buildHelpFactor builds a factor combining giveaways and requests.
func (c *MetricDetailCalculator) buildHelpFactor(
	ctx context.Context,
	source api.ImpactSourceType,
	giveaways []*models.Transfer,
	requests []*models.Request,
	dimension api.ImpactMetricDimension,
	period api.ImpactMetricPeriod,
	cache *EntityCache,
) *api.ContributingFactor {
	count := int32(len(giveaways) + len(requests))

	// Compute recent activity
	recentItems := c.computeRecentActivity(ctx, giveaways, requests, []*models.Experience{}, source, dimension, cache)

	// Compute time-series data (Trend, Volume, Cumulative)
	trend, volume, cumulative := c.computeHelpSeries(giveaways, requests, dimension, period)

	return &api.ContributingFactor{
		Count:       count,
		RecentItems: recentItems,
		Trend:       trend,
		Volume:      volume,
		Cumulative:  cumulative,
	}
}

// buildExperiencesFactor builds a factor for events/experiences.
func (c *MetricDetailCalculator) buildExperiencesFactor(
	ctx context.Context,
	source api.ImpactSourceType,
	experiences []*models.Experience,
	dimension api.ImpactMetricDimension,
	period api.ImpactMetricPeriod,
	cache *EntityCache,
) *api.ContributingFactor {
	count := int32(len(experiences))

	// Compute recent activity
	recentItems := c.computeRecentActivity(ctx, []*models.Transfer{}, []*models.Request{}, experiences, source, dimension, cache)

	// Compute time-series data (Trend, Volume, Cumulative)
	trend, volume, cumulative := c.computeExperienceSeries(experiences, dimension, period)

	return &api.ContributingFactor{
		Count:       count,
		RecentItems: recentItems,
		Trend:       trend,
		Volume:      volume,
		Cumulative:  cumulative,
	}
}

// computeTransferSeries computes trend, volume, and cumulative data for transfer-based factors.
func (c *MetricDetailCalculator) computeTransferSeries(
	transfers []*models.Transfer,
	dimension api.ImpactMetricDimension,
	period api.ImpactMetricPeriod,
) (trend, volume, cumulative []*api.TimeSeriesPoint) {
	start, end := periodToTimeRange(period)
	buckets := c.createTimeBuckets(period, start, end)

	// Group transactions into buckets and sum values
	bucketValues := make([]float32, len(buckets))
	bucketCounts := make([]int32, len(buckets))

	for _, transfer := range transfers {
		if transfer.ActualReturnUnixSec == nil || *transfer.ActualReturnUnixSec == 0 {
			continue
		}
		completedAt := time.Unix(*transfer.ActualReturnUnixSec, 0)
		bucketIdx := c.findBucketIndex(completedAt, buckets)
		if bucketIdx >= 0 {
			ie := c.transferImpactForSeries(transfer)
			value := c.extractValue(ie, dimension)
			bucketValues[bucketIdx] += value
			bucketCounts[bucketIdx]++
		}
	}

	// Build trend (cumulative values)
	trend = make([]*api.TimeSeriesPoint, len(buckets))
	cumulativeValue := float64(0)
	for i := range buckets {
		cumulativeValue += float64(bucketValues[i])
		trend[i] = &api.TimeSeriesPoint{
			Value:              cumulativeValue,
			BucketStartUnixSec: proto.Int64(buckets[i].Unix()),
		}
	}

	// Build volume (non-cumulative counts per bucket)
	volume = make([]*api.TimeSeriesPoint, len(buckets))
	for i := range buckets {
		volume[i] = &api.TimeSeriesPoint{
			Value:              float64(bucketCounts[i]),
			BucketStartUnixSec: proto.Int64(buckets[i].Unix()),
		}
	}

	// Build cumulative (cumulative values, same as trend for now)
	cumulative = make([]*api.TimeSeriesPoint, len(buckets))
	cumulativeVal := float64(0)
	for i := range buckets {
		cumulativeVal += float64(bucketValues[i])
		cumulative[i] = &api.TimeSeriesPoint{
			Value:              cumulativeVal,
			BucketStartUnixSec: proto.Int64(buckets[i].Unix()),
		}
	}

	return trend, volume, cumulative
}

// computeHelpSeries computes trend, volume, and cumulative data for giveaways + requests.
func (c *MetricDetailCalculator) computeHelpSeries(
	giveaways []*models.Transfer,
	requests []*models.Request,
	dimension api.ImpactMetricDimension,
	period api.ImpactMetricPeriod,
) (trend, volume, cumulative []*api.TimeSeriesPoint) {
	start, end := periodToTimeRange(period)
	buckets := c.createTimeBuckets(period, start, end)

	bucketValues := make([]float32, len(buckets))
	bucketCounts := make([]int32, len(buckets))

	// Process giveaways
	for _, transfer := range giveaways {
		if transfer.ActualReturnUnixSec == nil || *transfer.ActualReturnUnixSec == 0 {
			continue
		}
		completedAt := time.Unix(*transfer.ActualReturnUnixSec, 0)
		bucketIdx := c.findBucketIndex(completedAt, buckets)
		if bucketIdx >= 0 {
			ie := c.transferImpactForSeries(transfer)
			value := c.extractValue(ie, dimension)
			bucketValues[bucketIdx] += value
			bucketCounts[bucketIdx]++
		}
	}

	// Process requests - add to most recent bucket since they lack timestamps
	if len(buckets) > 0 {
		for _, request := range requests {
			ie := c.requestImpact(request)
			value := c.extractValue(ie, dimension)
			bucketValues[len(bucketValues)-1] += value
			bucketCounts[len(bucketCounts)-1]++
		}
	}

	// Build trend, volume, cumulative (same pattern as computeTransferSeries)
	trend = make([]*api.TimeSeriesPoint, len(buckets))
	volume = make([]*api.TimeSeriesPoint, len(buckets))
	cumulative = make([]*api.TimeSeriesPoint, len(buckets))

	cumulativeValue := float64(0)
	for i := range buckets {
		cumulativeValue += float64(bucketValues[i])
		bucketStart := proto.Int64(buckets[i].Unix())
		trend[i] = &api.TimeSeriesPoint{Value: cumulativeValue, BucketStartUnixSec: bucketStart}
		volume[i] = &api.TimeSeriesPoint{Value: float64(bucketCounts[i]), BucketStartUnixSec: bucketStart}
		cumulative[i] = &api.TimeSeriesPoint{Value: cumulativeValue, BucketStartUnixSec: bucketStart}
	}

	return trend, volume, cumulative
}

// computeExperienceSeries computes trend, volume, and cumulative data for experiences.
func (c *MetricDetailCalculator) computeExperienceSeries(
	experiences []*models.Experience,
	dimension api.ImpactMetricDimension,
	period api.ImpactMetricPeriod,
) (trend, volume, cumulative []*api.TimeSeriesPoint) {
	start, end := periodToTimeRange(period)
	buckets := c.createTimeBuckets(period, start, end)

	bucketValues := make([]float32, len(buckets))
	bucketCounts := make([]int32, len(buckets))

	for _, experience := range experiences {
		if experience.CompletedAtUnixSec == nil || *experience.CompletedAtUnixSec == 0 {
			continue
		}
		completedAt := time.Unix(*experience.CompletedAtUnixSec, 0)
		bucketIdx := c.findBucketIndex(completedAt, buckets)
		if bucketIdx >= 0 {
			ie := c.experienceImpact(experience)
			value := c.extractValue(ie, dimension)
			bucketValues[bucketIdx] += value
			bucketCounts[bucketIdx]++
		}
	}

	// Build trend, volume, cumulative
	trend = make([]*api.TimeSeriesPoint, len(buckets))
	volume = make([]*api.TimeSeriesPoint, len(buckets))
	cumulative = make([]*api.TimeSeriesPoint, len(buckets))

	cumulativeValue := float64(0)
	for i := range buckets {
		cumulativeValue += float64(bucketValues[i])
		bucketStart := proto.Int64(buckets[i].Unix())
		trend[i] = &api.TimeSeriesPoint{Value: cumulativeValue, BucketStartUnixSec: bucketStart}
		volume[i] = &api.TimeSeriesPoint{Value: float64(bucketCounts[i]), BucketStartUnixSec: bucketStart}
		cumulative[i] = &api.TimeSeriesPoint{Value: cumulativeValue, BucketStartUnixSec: bucketStart}
	}

	return trend, volume, cumulative
}

// gearWithTimestamp pairs a Gear with its CommunityGear timestamp.
type gearWithTimestamp struct {
	gear      *models.Gear
	timestamp int64
}

// computeGearTimeSeries computes trend, volume, and cumulative data using CommunityGear timestamps.
func (c *MetricDetailCalculator) computeGearTimeSeries(
	gearTimestamps []gearWithTimestamp,
	period api.ImpactMetricPeriod,
) (trend, volume, cumulative []*api.TimeSeriesPoint) {
	start, end := periodToTimeRange(period)
	buckets := c.createTimeBuckets(period, start, end)

	bucketCounts := make([]int32, len(buckets))
	bucketValues := make([]float32, len(buckets))

	// Bucket gear by when they were added to the community
	for _, gt := range gearTimestamps {
		if gt.timestamp == 0 {
			continue
		}
		sharedAt := time.Unix(gt.timestamp, 0)
		bucketIdx := c.findBucketIndex(sharedAt, buckets)
		if bucketIdx >= 0 {
			bucketCounts[bucketIdx]++
			if gt.gear.ValueEstimate != nil {
				bucketValues[bucketIdx] += gt.gear.ValueEstimate.EstimatedValueUsd
			}
		}
	}

	// Build trend (cumulative count), volume (new items per bucket), cumulative (cumulative value)
	trend = make([]*api.TimeSeriesPoint, len(buckets))
	volume = make([]*api.TimeSeriesPoint, len(buckets))
	cumulative = make([]*api.TimeSeriesPoint, len(buckets))

	cumulativeCount := int32(0)
	cumulativeValue := float64(0)
	for i := range buckets {
		cumulativeCount += bucketCounts[i]
		cumulativeValue += float64(bucketValues[i])
		bucketStart := proto.Int64(buckets[i].Unix())
		trend[i] = &api.TimeSeriesPoint{Value: float64(cumulativeCount), BucketStartUnixSec: bucketStart}
		volume[i] = &api.TimeSeriesPoint{Value: float64(bucketCounts[i]), BucketStartUnixSec: bucketStart}
		cumulative[i] = &api.TimeSeriesPoint{Value: cumulativeValue, BucketStartUnixSec: bucketStart}
	}

	return trend, volume, cumulative
}

// computeRecentGearItems computes recent activity for the Items factor.
// Since Gear model lacks a created_at timestamp, we return the first N items from the list.
func (c *MetricDetailCalculator) computeRecentGearItems(
	ctx context.Context,
	gearItems []*models.Gear,
	_ api.ImpactMetricDimension,
	limit int,
	cache *EntityCache,
) []*api.RecentActivity {
	logger := logging.LoggerWithContext(ctx).With("operation", "computeRecentGearItems")

	// Take first N items (no timestamp-based sorting available)
	var recentItems []*api.RecentActivity
	for i := 0; i < len(gearItems) && i < limit; i++ {
		gear := gearItems[i]

		// Load owner. Names and values go on the wire as-is or not at all —
		// the client supplies its own localized fallback (#2835).
		var personName *string
		mediaID := ""
		if gear.OwnerId != "" {
			owner := cache.GetUser(ctx, gear.OwnerId, logger)
			if owner != nil {
				if owner.Name != "" {
					personName = proto.String(owner.Name)
				}
				mediaID = firstMediaID(owner.MediaIds)
			}
		}

		var rawValue *float64
		if gear.ValueEstimate != nil {
			rawValue = proto.Float64(float64(gear.ValueEstimate.EstimatedValueUsd))
		}

		var itemName *string
		if gear.Name != "" {
			itemName = proto.String(gear.Name)
		}

		recentItems = append(recentItems, &api.RecentActivity{
			Kind:              api.RecentActivityKind_RECENT_ACTIVITY_KIND_GEAR,
			ContentId:         gear.Id,
			ItemName:          itemName,
			PersonDisplayName: personName,
			MediaId:           mediaID,
			RawValue:          rawValue,
		})
	}

	return recentItems
}
