package impact_metrics

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// computeTimeSeries computes cumulative time-series data for the main chart.
// Buckets transactions by time period and computes running totals.
func (c *MetricDetailCalculator) computeTimeSeries(
	transfers []*models.Transfer,
	requests []*models.Request,
	experiences []*models.Experience,
	dimension api.ImpactMetricDimension,
	period api.ImpactMetricPeriod,
	start, end time.Time,
) []*api.TimeSeriesPoint {
	// Determine bucket granularity
	buckets := c.createTimeBuckets(period, start, end)

	// Group transactions into buckets and sum values
	bucketValues := make([]float32, len(buckets))

	// Process transfers
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
		}
	}

	// Process requests (no timestamp filtering, add to all buckets proportionally or last bucket)
	// Since requests don't have completion timestamps, add them to the most recent bucket
	if len(buckets) > 0 {
		for _, request := range requests {
			ie := c.requestImpact(request)
			value := c.extractValue(ie, dimension)
			// Add to last bucket (most recent)
			bucketValues[len(bucketValues)-1] += value
		}
	}

	// Process experiences
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
		}
	}

	// Compute cumulative values
	cumulativeValues := make([]float64, len(bucketValues))
	cumulative := float64(0)
	for i, value := range bucketValues {
		cumulative += float64(value)
		cumulativeValues[i] = cumulative
	}

	// Build time-series points
	points := make([]*api.TimeSeriesPoint, len(buckets))
	for i := range buckets {
		points[i] = &api.TimeSeriesPoint{
			Value:              cumulativeValues[i],
			BucketStartUnixSec: proto.Int64(buckets[i].Unix()),
		}
	}

	return points
}

// computeMonthlyBars computes non-cumulative monthly bar data for CO2 breakdown.
// Always returns 6 months of data regardless of period filter.
func (c *MetricDetailCalculator) computeMonthlyBars(
	transfers []*models.Transfer,
	requests []*models.Request,
	experiences []*models.Experience,
	dimension api.ImpactMetricDimension,
) []*api.TimeSeriesPoint {
	// Always show last 6 months
	now := time.Now()
	startMonth := now.AddDate(0, -5, 0) // 6 months ago
	startMonth = time.Date(startMonth.Year(), startMonth.Month(), 1, 0, 0, 0, 0, time.UTC)

	// Create 6 monthly buckets
	buckets := make([]time.Time, 6)
	for i := 0; i < 6; i++ {
		buckets[i] = startMonth.AddDate(0, i, 0)
	}

	// Group transactions into monthly buckets
	bucketValues := make([]float32, 6)

	// Process transfers
	for _, transfer := range transfers {
		if transfer.ActualReturnUnixSec == nil || *transfer.ActualReturnUnixSec == 0 {
			continue
		}
		completedAt := time.Unix(*transfer.ActualReturnUnixSec, 0)
		bucketIdx := c.findMonthlyBucketIndex(completedAt, buckets)
		if bucketIdx >= 0 {
			ie := c.transferImpactForSeries(transfer)
			value := c.extractValue(ie, dimension)
			bucketValues[bucketIdx] += value
		}
	}

	// Process requests - distribute across recent months
	// Since requests don't have timestamps, add proportionally to recent months
	if len(requests) > 0 && len(buckets) > 0 {
		for _, request := range requests {
			ie := c.requestImpact(request)
			value := c.extractValue(ie, dimension)
			// Add to most recent month
			bucketValues[len(bucketValues)-1] += value
		}
	}

	// Process experiences
	for _, experience := range experiences {
		if experience.CompletedAtUnixSec == nil || *experience.CompletedAtUnixSec == 0 {
			continue
		}
		completedAt := time.Unix(*experience.CompletedAtUnixSec, 0)
		bucketIdx := c.findMonthlyBucketIndex(completedAt, buckets)
		if bucketIdx >= 0 {
			ie := c.experienceImpact(experience)
			value := c.extractValue(ie, dimension)
			bucketValues[bucketIdx] += value
		}
	}

	// Build monthly bar points (non-cumulative)
	points := make([]*api.TimeSeriesPoint, len(buckets))
	for i := range buckets {
		points[i] = &api.TimeSeriesPoint{
			Value:              float64(bucketValues[i]),
			BucketStartUnixSec: proto.Int64(buckets[i].Unix()),
		}
	}

	return points
}

// computeMonthlyAverage computes the average value across monthly bars.
func computeMonthlyAverage(monthlyBars []*api.TimeSeriesPoint) float64 {
	if len(monthlyBars) == 0 {
		return 0
	}

	total := float64(0)
	for _, point := range monthlyBars {
		total += point.Value
	}

	return total / float64(len(monthlyBars))
}

// createTimeBuckets creates the time buckets for a period. Callers put each
// bucket's start on the wire as bucket_start_unix_sec and the client renders
// the axis label from it in the viewer's locale (#2835) — the server used to
// return English "Jan" / "W1" labels alongside these.
func (c *MetricDetailCalculator) createTimeBuckets(period api.ImpactMetricPeriod, _, _ time.Time) []time.Time {
	now := time.Now()

	switch period {
	case api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_FOUR_WEEKS:
		// 4 weekly buckets
		buckets := make([]time.Time, 4)
		weekStart := now.AddDate(0, 0, -28) // 4 weeks ago
		for i := 0; i < 4; i++ {
			buckets[i] = weekStart.AddDate(0, 0, i*7)
		}
		return buckets

	case api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_THREE_MONTHS:
		// 12 weekly buckets
		buckets := make([]time.Time, 12)
		weekStart := now.AddDate(0, -3, 0) // 3 months ago
		for i := 0; i < 12; i++ {
			buckets[i] = weekStart.AddDate(0, 0, i*7)
		}
		return buckets

	case api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ONE_YEAR:
		// 12 monthly buckets
		buckets := make([]time.Time, 12)
		monthStart := now.AddDate(-1, 0, 0) // 1 year ago
		monthStart = time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, time.UTC)
		for i := 0; i < 12; i++ {
			buckets[i] = monthStart.AddDate(0, i, 0)
		}
		return buckets

	default: // METRIC_PERIOD_ALL
		// 6 monthly buckets
		buckets := make([]time.Time, 6)
		monthStart := now.AddDate(0, -5, 0) // 6 months ago
		monthStart = time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, time.UTC)
		for i := 0; i < 6; i++ {
			buckets[i] = monthStart.AddDate(0, i, 0)
		}
		return buckets
	}
}

// findBucketIndex finds the bucket index for a given timestamp.
// Returns -1 if the timestamp is before the first bucket.
func (c *MetricDetailCalculator) findBucketIndex(ts time.Time, buckets []time.Time) int {
	if len(buckets) == 0 {
		return -1
	}

	// Find the last bucket that starts before or at the timestamp
	for i := len(buckets) - 1; i >= 0; i-- {
		if ts.Equal(buckets[i]) || ts.After(buckets[i]) {
			return i
		}
	}

	return -1
}

// findMonthlyBucketIndex finds the monthly bucket index for a given timestamp.
func (c *MetricDetailCalculator) findMonthlyBucketIndex(ts time.Time, buckets []time.Time) int {
	if len(buckets) == 0 {
		return -1
	}

	// Normalize timestamp to start of month for comparison
	tsMonth := time.Date(ts.Year(), ts.Month(), 1, 0, 0, 0, 0, time.UTC)

	// Find exact month match
	for i, bucket := range buckets {
		bucketMonth := time.Date(bucket.Year(), bucket.Month(), 1, 0, 0, 0, 0, time.UTC)
		if tsMonth.Equal(bucketMonth) {
			return i
		}
	}

	return -1
}

// computeLibraryValueTrend builds cumulative time-series charts showing how
// the community's library value and item count grew as items were added over
// time. Gear added before the period window contributes to the first bucket so
// the lines always start at the pre-period baseline rather than zero. Uses
// batch queries to avoid N+1 patterns.
//
// Returns (valueTrend, itemCountTrend) where valueTrend is the cumulative
// dollar value per bucket and itemCountTrend is the cumulative item count.
func (c *MetricDetailCalculator) computeLibraryValueTrend(
	ctx context.Context,
	communityID string,
	period api.ImpactMetricPeriod,
) ([]*api.TimeSeriesPoint, []*api.TimeSeriesPoint) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "computeLibraryValueTrend",
		"community_id", communityID,
	)

	communityGear, err := resolveCommunityGear(ctx, c.storage, communityID)
	if err != nil {
		logger.WarnContext(ctx, "failed to query community gear for library value trend", "error", err)
		return nil, nil
	}

	type gearRef struct {
		gearID       string
		createdAtSec int64
	}
	seen := make(map[string]bool, len(communityGear))
	gearRefs := make([]gearRef, 0, len(communityGear))
	for _, cg := range communityGear {
		if cg.Archived || seen[cg.GearId] {
			continue
		}
		seen[cg.GearId] = true
		gearRefs = append(gearRefs, gearRef{gearID: cg.GearId, createdAtSec: cg.CreatedAtUnixSec})
	}
	if len(gearRefs) == 0 {
		return nil, nil
	}

	gearIDs := make([]string, len(gearRefs))
	for i, ref := range gearRefs {
		gearIDs[i] = ref.gearID
	}
	// Prefer preloaded dataset gear; fall back to batch fetch.
	var gearMap map[string]*models.Gear
	if ds, ok := DatasetFromContext(ctx); ok && ds.CommunityID == communityID {
		gearMap = make(map[string]*models.Gear, len(gearIDs))
		for _, id := range gearIDs {
			if g, hit := ds.Gear[id]; hit {
				gearMap[id] = g
			}
		}
	} else {
		gearMap, err = storage.GetByIDs[*models.Gear](c.storage, ctx, gearIDs)
		if err != nil {
			logger.WarnContext(ctx, "failed to batch-fetch gear for library value trend", "error", err)
			return nil, nil
		}
	}

	buckets := c.createTimeBuckets(period, time.Time{}, time.Time{})
	bucketValues := make([]float64, len(buckets))
	bucketCounts := make([]int, len(buckets))

	for _, ref := range gearRefs {
		gear, ok := gearMap[ref.gearID]
		if !ok || (gear.Deleted != nil && gear.Deleted.DeletedAtUnixSec > 0) {
			continue
		}
		addedAt := time.Unix(ref.createdAtSec, 0)
		bucketIdx := c.findBucketIndex(addedAt, buckets)
		if bucketIdx < 0 {
			// Added before the period window — fold into the first bucket.
			bucketIdx = 0
		}
		bucketCounts[bucketIdx]++
		if gear.ValueEstimate != nil {
			bucketValues[bucketIdx] += float64(gear.ValueEstimate.EstimatedValueUsd)
		}
	}

	valueTrend := make([]*api.TimeSeriesPoint, len(buckets))
	countTrend := make([]*api.TimeSeriesPoint, len(buckets))
	cumulativeValue := 0.0
	cumulativeCount := 0
	for i := range buckets {
		cumulativeValue += bucketValues[i]
		cumulativeCount += bucketCounts[i]
		bucketStart := proto.Int64(buckets[i].Unix())
		valueTrend[i] = &api.TimeSeriesPoint{
			Value:              cumulativeValue,
			BucketStartUnixSec: bucketStart,
		}
		countTrend[i] = &api.TimeSeriesPoint{
			Value:              float64(cumulativeCount),
			BucketStartUnixSec: bucketStart,
		}
	}
	return valueTrend, countTrend
}

// transferImpactForSeries is a helper that gets impact for time-series computation.
// Unlike transferImpact, we don't need the logger context here.
func (c *MetricDetailCalculator) transferImpactForSeries(transfer *models.Transfer) *api.ImpactEstimate {
	if transfer.ImpactEstimate != nil {
		return ModelsImpactToAPI(transfer.ImpactEstimate)
	}
	// Fallback: compute via builder (without gear lookup for performance)
	return BuildTransferImpactMetrics(&models.Gear{}, transfer.TransferType, c.cfg, nil, nil)
}

// extractValue extracts the mean value from an ImpactEstimate based on dimension.
func (c *MetricDetailCalculator) extractValue(ie *api.ImpactEstimate, dimension api.ImpactMetricDimension) float32 {
	estimate := c.extractEstimate(ie, dimension)
	if estimate == nil {
		return 0
	}
	return estimate.Mean
}
