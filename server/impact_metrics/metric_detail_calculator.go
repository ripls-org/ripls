package impact_metrics

import (
	"context"
	"fmt"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// MetricDetailCalculator computes detailed metric breakdowns for a community.
type MetricDetailCalculator struct {
	storage *storage.ProtoSQLStorage
	cfg     *estimator.Config
}

// NewMetricDetailCalculator creates a new MetricDetailCalculator instance.
func NewMetricDetailCalculator(storage *storage.ProtoSQLStorage, cfg *estimator.Config) *MetricDetailCalculator {
	return &MetricDetailCalculator{
		storage: storage,
		cfg:     cfg,
	}
}

// MetricDetailResult holds the computed metric detail.
type MetricDetailResult struct {
	TotalValue               *api.Estimate
	FormattedTotal           string
	SourceBreakdown          []*api.SourceBreakdown
	CumulativeTrend          []*api.TimeSeriesPoint
	MonthlyBars              []*api.TimeSeriesPoint
	MonthlyAverage           float64
	Factors                  []*api.ContributingFactor
	TopItems                 []*api.TopItem
	TopContributors          []*api.TopContributor
	MoneyDetail              *api.MoneySavedDetail
	TimeDetail               *api.TimeSavedDetail
	EmissionsPreventedDetail *api.EmissionsPreventedDetail
	QualityTimeDetail        *api.QualityTimeDetail
	Comparison               *api.CommunityComparison
	// LibraryItems contains all active gear sorted by value descending, for the
	// LIBRARY pill drill-down. Populated only for the MONEY dimension. Includes
	// items with zero transactions, unlike TopItems which requires activity.
	LibraryItems []*api.TopItem
	// LibraryValueTrend is the cumulative library value over time, for the
	// LIBRARY pill drill-down chart. Populated only for the MONEY dimension.
	LibraryValueTrend []*api.TimeSeriesPoint
	// LibraryItemCountTrend is the cumulative item count over time, for the
	// dual-axis overlay in the LIBRARY pill drill-down chart. Populated only
	// for the MONEY dimension.
	LibraryItemCountTrend []*api.TimeSeriesPoint
	// RecentItems is the most recent contributing items for this dimension,
	// newest first. Capped at recentItemsLimit.
	RecentItems []*api.RecentActivity
}

// ComputeMetricDetail loads a one-shot CommunityDataset and delegates to
// ComputeMetricDetailFromDataset. Use the dataset variant directly when
// the caller already has a preloaded dataset (e.g. when stitching
// detail data into a larger response). See #2055.
func (c *MetricDetailCalculator) ComputeMetricDetail(
	ctx context.Context,
	communityID string,
	dimension api.ImpactMetricDimension,
	period api.ImpactMetricPeriod,
) (*MetricDetailResult, error) {
	ds, err := LoadCommunityDataset(ctx, c.storage, communityID)
	if err != nil {
		return nil, fmt.Errorf("failed to load community dataset: %w", err)
	}
	return c.ComputeMetricDetailFromDataset(ctx, ds, dimension, period)
}

// ComputeMetricDetailFromDataset computes detailed metric breakdowns
// for a community from a preloaded dataset. Replaces the per-row
// GetByID fan-out in the previous shape with map lookups against
// ds.Requests / ds.Experiences / ds.Gear.
func (c *MetricDetailCalculator) ComputeMetricDetailFromDataset(
	ctx context.Context,
	ds *CommunityDataset,
	dimension api.ImpactMetricDimension,
	period api.ImpactMetricPeriod,
) (*MetricDetailResult, error) {
	communityID := ds.CommunityID
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ComputeMetricDetail",
		"community_id", communityID,
		"dimension", dimension.String(),
		"period", period.String(),
	)

	logger.DebugContext(ctx, "computing metric detail from preloaded dataset")

	// Filter source rows from the preloaded dataset — pure compute,
	// no DB access. Replaces the per-row GetByID loops in
	// loadFulfilledRequests / loadCompletedExperiences (which on the
	// populated dev fixture issued ~400 queries).
	transfers := FilterCompletedTransfers(ds.Transfers)
	requests := FilterFulfilledRequests(ds)
	experiences := FilterCompletedExperiences(ds)

	// Attach the dataset's gear map to ctx so transferImpact and
	// other helpers can short-circuit per-transfer GetByID(Gear)
	// fallbacks into map hits. Also attach the full dataset so nested
	// helpers can pull preloaded CommunityGear / CommunityUser slices
	// instead of issuing their own QueryByField.
	ctx = WithDatasetGearMap(ctx, ds)
	ctx = WithCommunityDataset(ctx, ds)

	// Filter by period
	start, end := periodToTimeRange(period)
	transfers = c.filterTransfersByPeriod(transfers, start, end)
	requests = c.filterRequestsByPeriod(requests, start, end)
	experiences = c.filterExperiencesByPeriod(experiences, start, end)

	logger.DebugContext(ctx, "filtered transactions by period",
		"transfers", len(transfers),
		"requests", len(requests),
		"experiences", len(experiences),
	)

	// Compute source breakdown and total
	totalValue, breakdown := c.computeSourceBreakdown(ctx, transfers, requests, experiences, dimension)

	cumulativeTrend := c.computeTimeSeries(transfers, requests, experiences, dimension, period, start, end)

	// Compute monthly bars for CO2 and Social (always show 6 months)
	var monthlyBars []*api.TimeSeriesPoint
	var monthlyAverage float64
	if dimension == api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS ||
		dimension == api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME {
		monthlyBars = c.computeMonthlyBars(transfers, requests, experiences, dimension)
		monthlyAverage = computeMonthlyAverage(monthlyBars)
	}

	factors, err := c.computeFactors(ctx, transfers, requests, experiences, dimension, period, communityID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to compute factors", "error", err)
		// Don't fail the whole request, just use empty factors
		factors = []*api.ContributingFactor{}
	}

	// EntityCache is prepopulated from the dataset so any helper that
	// calls cache.GetGear(t.GearId) hits the in-memory map instead of
	// issuing a DB round-trip.
	cache := NewEntityCacheFromDataset(c.storage, ds)
	topItems := c.computeTopItems(ctx, transfers, requests, experiences, dimension, cache)
	topContributors := c.computeTopContributors(ctx, transfers, requests, experiences, dimension, 2, cache)

	// Compute library items and value/count trends for the LIBRARY pill drill-down (MONEY only).
	var libraryItems []*api.TopItem
	var libraryValueTrend []*api.TimeSeriesPoint
	var libraryItemCountTrend []*api.TimeSeriesPoint
	if dimension == api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY {
		libraryItems = c.computeLibraryItems(ctx, communityID, transfers)
		libraryValueTrend, libraryItemCountTrend = c.computeLibraryValueTrend(ctx, communityID, period)
	}

	var moneyDetail *api.MoneySavedDetail
	var timeDetail *api.TimeSavedDetail
	var co2Detail *api.EmissionsPreventedDetail
	var qualityTimeDetail *api.QualityTimeDetail

	switch dimension {
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY:
		var err error
		moneyDetail, err = c.computeMoneyDetail(ctx, communityID, totalValue.Mean)
		if err != nil {
			logger.ErrorContext(ctx, "failed to compute money detail", "error", err)
			// Don't fail the whole request, just skip the detail
		}

	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_TIME:
		var err error
		timeDetail, err = c.computeTimeDetail(ctx, communityID, totalValue.Mean)
		if err != nil {
			logger.ErrorContext(ctx, "failed to compute time detail", "error", err)
			// Don't fail the whole request, just skip the detail
		}

	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS:
		co2Detail = c.computeCO2Detail(totalValue.Mean, monthlyBars, monthlyAverage)

	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME:
		qualityTimeDetail = c.computeQualityTimeDetail(ctx, totalValue.Mean, period, transfers, requests, experiences, monthlyBars, monthlyAverage, communityID)
	}

	comparison := c.computeComparison(ctx, communityID, totalValue.Mean, transfers, requests, experiences, dimension)

	recentItems := c.computeRecentItems(ctx, logger, transfers, requests, experiences, dimension)

	result := &MetricDetailResult{
		TotalValue:               totalValue,
		FormattedTotal:           c.formatValue(totalValue.Mean, dimension),
		SourceBreakdown:          breakdown,
		CumulativeTrend:          cumulativeTrend,
		MonthlyBars:              monthlyBars,
		MonthlyAverage:           monthlyAverage,
		Factors:                  factors,
		TopItems:                 topItems,
		TopContributors:          topContributors,
		MoneyDetail:              moneyDetail,
		TimeDetail:               timeDetail,
		EmissionsPreventedDetail: co2Detail,
		QualityTimeDetail:        qualityTimeDetail,
		Comparison:               comparison,
		LibraryItems:             libraryItems,
		LibraryValueTrend:        libraryValueTrend,
		LibraryItemCountTrend:    libraryItemCountTrend,
		RecentItems:              recentItems,
	}

	logger.InfoContext(ctx, "computed metric detail",
		"total_mean", totalValue.Mean,
		"breakdown_count", len(breakdown),
	)

	return result, nil
}

// filterTransfersByPeriod filters transfers by their completion timestamp.
func (c *MetricDetailCalculator) filterTransfersByPeriod(
	transfers []*models.Transfer,
	start, end time.Time,
) []*models.Transfer {
	// If no filtering (ALL period), return all
	if start.IsZero() {
		return transfers
	}

	var filtered []*models.Transfer
	for _, transfer := range transfers {
		if transfer.ActualReturnUnixSec == nil || *transfer.ActualReturnUnixSec == 0 {
			continue
		}

		completedAt := time.Unix(*transfer.ActualReturnUnixSec, 0)
		if (completedAt.Equal(start) || completedAt.After(start)) && completedAt.Before(end) {
			filtered = append(filtered, transfer)
		}
	}

	return filtered
}

// filterRequestsByPeriod filters requests by their completion timestamp.
// Note: Request model does not have a completion timestamp field, so for now
// all fulfilled requests are included regardless of period. Accurate period
// filtering would need a completion timestamp added to the Request model.
func (c *MetricDetailCalculator) filterRequestsByPeriod(
	requests []*models.Request,
	_, _ time.Time,
) []*models.Request {
	// Request model lacks a completion timestamp, so we can't filter by period.
	// Return all fulfilled requests for now.
	return requests
}

// filterExperiencesByPeriod filters experiences by their completion timestamp.
func (c *MetricDetailCalculator) filterExperiencesByPeriod(
	experiences []*models.Experience,
	start, end time.Time,
) []*models.Experience {
	// If no filtering (ALL period), return all
	if start.IsZero() {
		return experiences
	}

	var filtered []*models.Experience
	for _, experience := range experiences {
		// completed_at_unix_sec is an optional field (pointer)
		if experience.CompletedAtUnixSec == nil || *experience.CompletedAtUnixSec == 0 {
			continue
		}

		completedAt := time.Unix(*experience.CompletedAtUnixSec, 0)
		if (completedAt.Equal(start) || completedAt.After(start)) && completedAt.Before(end) {
			filtered = append(filtered, experience)
		}
	}

	return filtered
}

// periodToTimeRange converts a MetricPeriod enum to a time range.
// Returns (zero, zero) for ALL period (no filtering).
func periodToTimeRange(period api.ImpactMetricPeriod) (start, end time.Time) {
	now := time.Now()

	switch period {
	case api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_FOUR_WEEKS:
		start = now.AddDate(0, 0, -28) // 4 weeks ago
		end = now
	case api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_THREE_MONTHS:
		start = now.AddDate(0, -3, 0) // 3 months ago
		end = now
	case api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ONE_YEAR:
		start = now.AddDate(-1, 0, 0) // 1 year ago
		end = now
	default: // METRIC_PERIOD_ALL or UNSPECIFIED
		// Return zero times to indicate no filtering
		return time.Time{}, time.Time{}
	}

	return start, end
}

// computeSourceBreakdown groups transactions by type and computes breakdown percentages.
func (c *MetricDetailCalculator) computeSourceBreakdown(
	ctx context.Context,
	transfers []*models.Transfer,
	requests []*models.Request,
	experiences []*models.Experience,
	dimension api.ImpactMetricDimension,
) (*api.Estimate, []*api.SourceBreakdown) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "computeSourceBreakdown",
		"dimension", dimension.String(),
	)

	// Aggregate by source type
	loansEstimates := make([]*api.Estimate, 0)
	giveawaysEstimates := make([]*api.Estimate, 0)
	requestsEstimates := make([]*api.Estimate, 0)
	eventsEstimates := make([]*api.Estimate, 0)

	// Process transfers (loans and giveaways)
	for _, transfer := range transfers {
		ie := c.transferImpact(ctx, transfer, logger)
		estimate := c.extractEstimate(ie, dimension)

		if estimate == nil {
			continue
		}

		switch transfer.TransferType {
		case models.TransferType_TRANSFER_TYPE_LOAN:
			loansEstimates = append(loansEstimates, estimate)
		case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
			giveawaysEstimates = append(giveawaysEstimates, estimate)
		}
	}

	// Process requests
	for _, request := range requests {
		ie := c.requestImpact(request)
		estimate := c.extractEstimate(ie, dimension)

		if estimate != nil {
			requestsEstimates = append(requestsEstimates, estimate)
		}
	}

	// Process experiences
	for _, experience := range experiences {
		ie := c.experienceImpact(experience)
		estimate := c.extractEstimate(ie, dimension)

		if estimate != nil {
			eventsEstimates = append(eventsEstimates, estimate)
		}
	}

	// Sum each source type
	loansTotal := estimator.SumEstimates(loansEstimates)
	giveawaysTotal := estimator.SumEstimates(giveawaysEstimates)
	requestsTotal := estimator.SumEstimates(requestsEstimates)
	eventsTotal := estimator.SumEstimates(eventsEstimates)

	// Compute grand total
	allEstimates := append(append(loansEstimates, giveawaysEstimates...), requestsEstimates...)
	allEstimates = append(allEstimates, eventsEstimates...)
	grandTotal := estimator.SumEstimates(allEstimates)

	// Build breakdown with percentages
	breakdown := c.buildBreakdown(
		grandTotal.Mean,
		dimension,
		loansTotal.Mean,
		giveawaysTotal.Mean,
		requestsTotal.Mean,
		eventsTotal.Mean,
		int32(len(loansEstimates)),
		int32(len(giveawaysEstimates)),
		int32(len(requestsEstimates)),
		int32(len(eventsEstimates)),
	)

	logger.DebugContext(ctx, "computed source breakdown",
		"loans_count", len(loansEstimates),
		"giveaways_count", len(giveawaysEstimates),
		"requests_count", len(requestsEstimates),
		"events_count", len(eventsEstimates),
		"total_mean", grandTotal.Mean,
	)

	return grandTotal, breakdown
}

// buildBreakdown creates the SourceBreakdown slice with computed percentages.
// Only includes sources with non-zero values.
func (c *MetricDetailCalculator) buildBreakdown(
	total float32,
	dimension api.ImpactMetricDimension,
	loansValue, giveawaysValue, requestsValue, eventsValue float32,
	loansCount, giveawaysCount, requestsCount, eventsCount int32,
) []*api.SourceBreakdown {
	var breakdown []*api.SourceBreakdown

	addSource := func(sourceType api.ImpactSourceType, value float32, count int32) {
		if value > 0 {
			percentage := float64(0)
			if total > 0 {
				percentage = float64((value / total) * 100)
			}
			breakdown = append(breakdown, &api.SourceBreakdown{
				SourceType: sourceType,
				Value:      float64(value),
				Percentage: percentage,
				Count:      &count,
			})
		}
	}

	// Add sources in typed display order. For Time dimension: giveaways are
	// not a time source, so fold their value into Requests.
	if dimension == api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_TIME {
		addSource(api.ImpactSourceType_IMPACT_SOURCE_TYPE_LOANS, loansValue, loansCount)
		addSource(api.ImpactSourceType_IMPACT_SOURCE_TYPE_REQUESTS, requestsValue+giveawaysValue, requestsCount+giveawaysCount)
		addSource(api.ImpactSourceType_IMPACT_SOURCE_TYPE_EVENTS, eventsValue, eventsCount)
	} else {
		addSource(api.ImpactSourceType_IMPACT_SOURCE_TYPE_LOANS, loansValue, loansCount)
		addSource(api.ImpactSourceType_IMPACT_SOURCE_TYPE_GIVEAWAYS, giveawaysValue, giveawaysCount)
		addSource(api.ImpactSourceType_IMPACT_SOURCE_TYPE_REQUESTS, requestsValue, requestsCount)
		addSource(api.ImpactSourceType_IMPACT_SOURCE_TYPE_EVENTS, eventsValue, eventsCount)
	}

	return breakdown
}

// extractEstimate extracts the appropriate estimate from an ImpactEstimate based on dimension.
func (c *MetricDetailCalculator) extractEstimate(ie *api.ImpactEstimate, dimension api.ImpactMetricDimension) *api.Estimate {
	switch dimension {
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY:
		return CostEstimate(ie)
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_TIME:
		return TimeEstimate(ie)
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS:
		return CarbonEstimate(ie)
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME:
		return QualityTimeEstimate(ie)
	default:
		return nil
	}
}

// formatValue formats a value based on dimension.
func (c *MetricDetailCalculator) formatValue(value float32, dimension api.ImpactMetricDimension) string {
	switch dimension {
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY:
		return FormatMoney(float64(value))
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_TIME:
		return FormatTime(float64(value))
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS:
		// CO2 is stored in grams in ImpactEstimate, FormatCO2 expects grams
		return FormatCO2(float64(value))
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME:
		// QT minutes formatted as a decimal with one place (e.g. "12.3 min")
		return fmt.Sprintf("%.1f min", value)
	default:
		return ""
	}
}

// transferImpact returns the ImpactEstimate for a transfer, converting
// from storage model if present, or computing a fallback via the
// builder. When the request context carries a preloaded gear map (via
// WithDatasetGearMap), the fallback is a map lookup; otherwise the
// slow path issues a single GetByID. See #2055.
func (c *MetricDetailCalculator) transferImpact(ctx context.Context, transfer *models.Transfer, logger *logging.Logger) *api.ImpactEstimate {
	if transfer.ImpactEstimate != nil {
		return ModelsImpactToAPI(transfer.ImpactEstimate)
	}

	// Fast path: preloaded gear map from context (set by
	// ComputeMetricDetailFromDataset).
	if gear, ok := GearFromContext(ctx, transfer.GearId); ok {
		return BuildTransferImpactMetrics(gear, transfer.TransferType, c.cfg, nil, nil)
	}

	// Slow path: load gear via DB and compute via builder. Used by
	// one-off callers that don't preload a dataset.
	gear := &models.Gear{}
	if err := c.storage.GetByID(ctx, transfer.GearId, gear); err != nil {
		logger.DebugContext(ctx, "could not load gear for transfer fallback",
			"transfer_id", transfer.Id, "gear_id", transfer.GearId)
		return BuildTransferImpactMetrics(&models.Gear{}, transfer.TransferType, c.cfg, nil, nil)
	}
	return BuildTransferImpactMetrics(gear, transfer.TransferType, c.cfg, nil, nil)
}

// requestImpact returns the ImpactEstimate for a request, converting from storage model
// if present, or computing a fallback via the builder. Transfer-adopted
// dimensions are masked (#2702) — the transfer counts them, not the request.
func (c *MetricDetailCalculator) requestImpact(request *models.Request) *api.ImpactEstimate {
	if request.ImpactEstimate != nil {
		return MaskTransferAdoptedRequestDimensions(request, ModelsImpactToAPI(request.ImpactEstimate))
	}
	// Extract value from request if available, otherwise use 0
	var valueUSD float32
	if request.ValueEstimate != nil {
		valueUSD = request.ValueEstimate.EstimatedValueUsd
	}
	return MaskTransferAdoptedRequestDimensions(request, BuildRequestImpactMetrics(valueUSD, c.cfg, nil, nil))
}

// experienceImpact returns the ImpactEstimate for an experience, converting from storage model
// if present, or computing a fallback via the builder.
func (c *MetricDetailCalculator) experienceImpact(experience *models.Experience) *api.ImpactEstimate {
	if experience.ImpactEstimate != nil {
		return ModelsImpactToAPI(experience.ImpactEstimate)
	}
	// Extract value from experience if available, otherwise use 0
	var valueUSD float32
	if experience.ValueEstimate != nil {
		valueUSD = experience.ValueEstimate.EstimatedValueUsd
	}
	return BuildExperienceImpactMetrics(valueUSD, 1, c.cfg, nil, nil)
}
