package impact_metrics

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	impactlib "go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// loanStats holds per-gear loan aggregates for utilization computation.
type loanStats struct {
	loanCount   int
	totalDays   float64 // total loan-days (sum of actual durations)
	totalSaved  float64
	completedAt []time.Time // for trend computation
}

// getCommunityUtilization computes utilization analytics for a community.
func getCommunityUtilization(
	ctx context.Context,
	s *Service,
	req *connect.Request[api.GetCommunityUtilizationRequest],
) (*connect.Response[api.GetCommunityUtilizationResponse], error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetCommunityUtilization",
		"community_id", req.Msg.CommunityId,
	)

	if _, err := auth.RequireAuth(ctx); err != nil {
		return nil, err
	}

	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}

	store := s.calculator.Storage()
	if _, err := auth.RequireActiveCommunity(ctx, store, req.Msg.CommunityId); err != nil {
		return nil, err
	}

	// Load community gear via junction table.
	communityGearRaw, err := storage.QueryByField[*models.CommunityGear](store, ctx, "community_id", req.Msg.CommunityId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community gear", "error", err)
		return nil, connecterr.Internal(ctx, "getCommunityUtilization", err, "detail", "failed to query community gear")
	}

	gearIDs := storage.CollectField(communityGearRaw, func(cg *models.CommunityGear) string { return cg.GearId })
	gearMap, err := storage.GetByIDs[*models.Gear](store, ctx, gearIDs, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch load gear", "error", err)
		return nil, connecterr.Internal(ctx, "getCommunityUtilization", err, "detail",

			// Load all completed loans for this community.
			"failed to load gear")
	}

	transfersRaw, err := store.QueryByField(ctx, "community_id", req.Msg.CommunityId, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query transfers", "error", err)
		return nil, connecterr.Internal(ctx, "getCommunityUtilization", err, "detail",

			// Build per-gear loan stats.
			"failed to query transfers")
	}

	gearLoanStats := make(map[string]*loanStats)
	for _, raw := range transfersRaw {
		t := raw.(*models.Transfer)
		if t.Deleted != nil || t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		if t.TransferType != models.TransferType_TRANSFER_TYPE_LOAN {
			continue
		}

		stats, ok := gearLoanStats[t.GearId]
		if !ok {
			stats = &loanStats{}
			gearLoanStats[t.GearId] = stats
		}
		stats.loanCount++

		// Compute actual loan duration in days.
		if t.ActualPickupUnixSec != nil && *t.ActualPickupUnixSec > 0 && t.ActualReturnUnixSec != nil && *t.ActualReturnUnixSec > 0 {
			pickup := time.Unix(*t.ActualPickupUnixSec, 0)
			returnAt := time.Unix(*t.ActualReturnUnixSec, 0)
			duration := returnAt.Sub(pickup).Hours() / 24
			if duration > 0 {
				stats.totalDays += duration
			}
			stats.completedAt = append(stats.completedAt, returnAt)
		}

		// Sum money saved.
		if t.ImpactEstimate != nil && t.ImpactEstimate.MoneySaved != nil && t.ImpactEstimate.MoneySaved.ValueUsd != nil {
			stats.totalSaved += float64(t.ImpactEstimate.MoneySaved.ValueUsd.Mean)
		}
	}

	// Batch-load gear owners for display names.
	ownerIDs := make(map[string]bool)
	for _, gear := range gearMap {
		if gear.Deleted != nil {
			continue
		}
		ownerIDs[gear.OwnerId] = true
	}
	ownerIDSlice := make([]string, 0, len(ownerIDs))
	for id := range ownerIDs {
		ownerIDSlice = append(ownerIDSlice, id)
	}
	userMap, _ := storage.GetByIDs[*models.User](store, ctx, ownerIDSlice)

	// Build utilization items for all active gear.
	now := time.Now()
	var items []*utilizationEntry
	var totalValue float64

	for _, gear := range gearMap {
		if gear.Deleted != nil {
			continue
		}

		valueUSD := float64(0)
		if gear.ValueEstimate != nil {
			valueUSD = float64(gear.ValueEstimate.EstimatedValueUsd)
		}
		totalValue += valueUSD

		daysListed := int32(0)
		if gear.CreatedAtUnixSec > 0 {
			daysListed = int32(now.Sub(time.Unix(gear.CreatedAtUnixSec, 0)).Hours() / 24)
			if daysListed < 1 {
				daysListed = 1
			}
		}

		stats := gearLoanStats[gear.Id]
		loanCount := 0
		loanDays := 0.0
		savedUSD := 0.0
		if stats != nil {
			loanCount = stats.loanCount
			loanDays = stats.totalDays
			savedUSD = stats.totalSaved
		}

		// Utilization rate = loan-days / days-listed × 100.
		utilPct := 0.0
		if daysListed > 0 {
			utilPct = (loanDays / float64(daysListed)) * 100
			if utilPct > 100 {
				utilPct = 100
			}
		}

		ownerName := ""
		if user, ok := userMap[gear.OwnerId]; ok {
			ownerName = user.Name
		}

		category := ""
		if gear.Category != nil {
			category = gear.Category.Value
		}

		items = append(items, &utilizationEntry{
			gearID:     gear.Id,
			name:       gear.Name,
			ownerName:  ownerName,
			utilPct:    utilPct,
			loanCount:  int32(loanCount),
			valueUSD:   valueUSD,
			daysListed: daysListed,
			savedUSD:   savedUSD,
			category:   category,
		})
	}

	// Sort for most utilized (descending by utilPct).
	mostUtilized := sortedItems(items, func(a, b *utilizationEntry) bool {
		return a.utilPct > b.utilPct
	}, 10)

	// Untapped potential: high value, low utilization (< 5% util, sorted by value desc).
	untapped := make([]*utilizationEntry, 0)
	for _, item := range items {
		if item.utilPct < 5 && item.valueUSD > 0 && item.daysListed > 30 {
			untapped = append(untapped, item)
		}
	}
	sort.Slice(untapped, func(i, j int) bool {
		return untapped[i].valueUSD > untapped[j].valueUSD
	})
	if len(untapped) > 10 {
		untapped = untapped[:10]
	}

	// Redundancy groups: items sharing the same non-empty category.
	categoryGroups := make(map[string][]*utilizationEntry)
	for _, item := range items {
		cat := strings.TrimSpace(item.category)
		if cat == "" {
			continue
		}
		categoryGroups[cat] = append(categoryGroups[cat], item)
	}
	var redundancy []*api.RedundancyGroup
	for cat, group := range categoryGroups {
		if len(group) < 2 {
			continue
		}
		var groupValue float64
		var groupItems []*api.UtilizationItem
		for _, item := range group {
			groupValue += item.valueUSD
			groupItems = append(groupItems, item.toProto())
		}
		redundancy = append(redundancy, &api.RedundancyGroup{
			Label:               cat,
			Count:               int32(len(group)),
			FormattedTotalValue: impactlib.FormatMoney(groupValue),
			Items:               groupItems,
		})
	}
	// Sort redundancy by group size descending.
	sort.Slice(redundancy, func(i, j int) bool {
		return redundancy[i].Count > redundancy[j].Count
	})
	if len(redundancy) > 5 {
		redundancy = redundancy[:5]
	}

	// Compute utilization trend (monthly).
	trend := computeUtilizationTrend(gearLoanStats, items, now)

	// Overall utilization rate.
	overallUtil := 0.0
	if totalValue > 0 {
		var totalSaved float64
		for _, stats := range gearLoanStats {
			totalSaved += stats.totalSaved
		}
		overallUtil = totalSaved / totalValue * 100
		if overallUtil > 100 {
			overallUtil = 100
		}
	}

	logger.InfoContext(ctx, "computed utilization analytics",
		"item_count", len(items),
		"overall_util_pct", overallUtil,
		"most_utilized_count", len(mostUtilized),
		"untapped_count", len(untapped),
		"redundancy_groups", len(redundancy),
	)

	resp := &api.GetCommunityUtilizationResponse{
		UtilizationRate:       overallUtil,
		FormattedUtilization:  fmt.Sprintf("%.0f%%", overallUtil),
		UtilizationTrend:      trend,
		MostUtilized:          toProtoItems(mostUtilized),
		UntappedPotential:     toProtoItems(untapped),
		RedundancyGroups:      redundancy,
		FormattedLibraryValue: impactlib.FormatMoney(totalValue),
		ItemCount:             int32(len(items)),
	}

	return connect.NewResponse(resp), nil
}

// utilizationEntry holds per-item utilization data during computation.
type utilizationEntry struct {
	gearID     string
	name       string
	ownerName  string
	utilPct    float64
	loanCount  int32
	valueUSD   float64
	daysListed int32
	savedUSD   float64
	category   string
}

// toProto converts a utilizationEntry to the proto UtilizationItem.
func (e *utilizationEntry) toProto() *api.UtilizationItem {
	return &api.UtilizationItem{
		GearId:           e.gearID,
		Name:             e.name,
		OwnerName:        e.ownerName,
		UtilizationPct:   e.utilPct,
		LoanCount:        e.loanCount,
		FormattedValue:   impactlib.FormatMoney(e.valueUSD),
		DaysListed:       e.daysListed,
		FormattedSavings: impactlib.FormatMoney(e.savedUSD),
		Category:         e.category,
	}
}

// sortedItems returns the top N items sorted by the given comparator.
func sortedItems(items []*utilizationEntry, less func(a, b *utilizationEntry) bool, limit int) []*utilizationEntry {
	sorted := make([]*utilizationEntry, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool { return less(sorted[i], sorted[j]) })
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	return sorted
}

// toProtoItems converts a slice of utilizationEntry to proto UtilizationItems.
func toProtoItems(entries []*utilizationEntry) []*api.UtilizationItem {
	result := make([]*api.UtilizationItem, len(entries))
	for i, e := range entries {
		result[i] = e.toProto()
	}
	return result
}

// computeUtilizationTrend computes monthly utilization rates for the last 6 months.
func computeUtilizationTrend(
	gearStats map[string]*loanStats,
	items []*utilizationEntry,
	now time.Time,
) []*api.TimeSeriesPoint {
	if len(items) == 0 {
		return nil
	}

	// Count completed loans per month.
	monthlyLoans := make(map[string]int)
	for _, stats := range gearStats {
		for _, completed := range stats.completedAt {
			key := fmt.Sprintf("%d-%02d", completed.Year(), completed.Month())
			monthlyLoans[key]++
		}
	}

	// Build 6-month window.
	var points []*api.TimeSeriesPoint
	totalItems := len(items)
	if totalItems == 0 {
		totalItems = 1
	}

	for i := 5; i >= 0; i-- {
		month := now.AddDate(0, -i, 0)
		key := fmt.Sprintf("%d-%02d", month.Year(), month.Month())
		loans := monthlyLoans[key]
		// Utilization proxy: loans completed that month / total items × 100.
		utilPct := float64(loans) / float64(totalItems) * 100
		if utilPct > 100 {
			utilPct = 100
		}
		bucketStart := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
		points = append(points, &api.TimeSeriesPoint{
			Value:              utilPct,
			BucketStartUnixSec: proto.Int64(bucketStart.Unix()),
		})
	}

	return points
}
