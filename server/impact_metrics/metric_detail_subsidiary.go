package impact_metrics

import (
	"context"
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// optionalString returns s as an optional-field pointer, or nil when empty —
// absence (not empty string) is what tells clients to render a localized
// fallback.
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return proto.String(s)
}

// computeSubsidiaryMetrics aggregates Belonging Minutes, Trust Credits, and
// Network Diversity from per-transaction QualityTimeEstimate fields.
func (c *MetricDetailCalculator) computeSubsidiaryMetrics(
	ctx context.Context,
	logger *logging.Logger,
	transfers []*models.Transfer,
	requests []*models.Request,
	experiences []*models.Experience,
	memberCount int,
) []*api.SubsidiaryMetric {
	var totalBelongingMinutes float32
	var totalTrustCredits float32
	var inPersonCount int
	var totalInteractions int

	// Monthly bucketing for sparklines (last 6 months).
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month()-5, 1, 0, 0, 0, 0, time.UTC)
	buckets := c.monthlyBuckets(monthStart)
	belongingMonthly := make([]float32, 6)
	trustMonthly := make([]float32, 6)

	// Collect distinct contacts per member for network diversity.
	contactsByUser := make(map[string]map[string]bool)

	addContacts := func(userA, userB string) {
		if userA == "" || userB == "" {
			return
		}
		if contactsByUser[userA] == nil {
			contactsByUser[userA] = make(map[string]bool)
		}
		contactsByUser[userA][userB] = true
		if contactsByUser[userB] == nil {
			contactsByUser[userB] = make(map[string]bool)
		}
		contactsByUser[userB][userA] = true
	}

	accumulateQT := func(ie *api.ImpactEstimate, completedAt time.Time) {
		if ie == nil || ie.QualityTime == nil {
			return
		}
		sf := ie.QualityTime
		totalInteractions++
		inPersonCount++ // All sharing interactions are in-person.

		if sf.BelongingMinutes != nil {
			totalBelongingMinutes += sf.BelongingMinutes.Mean
		}
		if sf.TrustCredits != nil {
			totalTrustCredits += sf.TrustCredits.Mean
		}

		// Monthly bucketing for sparklines.
		if !completedAt.IsZero() {
			idx := c.findMonthlyBucketIndex(completedAt, buckets)
			if idx >= 0 && idx < 6 {
				if sf.BelongingMinutes != nil {
					belongingMonthly[idx] += sf.BelongingMinutes.Mean
				}
				if sf.TrustCredits != nil {
					trustMonthly[idx] += sf.TrustCredits.Mean
				}
			}
		}
	}

	// Process transfers.
	for _, t := range transfers {
		ie := c.transferImpact(ctx, t, logger)
		var completedAt time.Time
		if t.ActualReturnUnixSec != nil && *t.ActualReturnUnixSec > 0 {
			completedAt = time.Unix(*t.ActualReturnUnixSec, 0)
		}
		accumulateQT(ie, completedAt)
		addContacts(t.OwnerId, t.RecipientId)
	}

	// Process requests.
	for _, r := range requests {
		ie := c.requestImpact(r)
		accumulateQT(ie, time.Now()) // Requests lack completion timestamps.
		for _, helperID := range r.ConfirmedHelperIds {
			addContacts(r.RequesterId, helperID)
		}
	}

	// Process experiences.
	for _, e := range experiences {
		ie := c.experienceImpact(e)
		var completedAt time.Time
		if e.CompletedAtUnixSec != nil && *e.CompletedAtUnixSec > 0 {
			completedAt = time.Unix(*e.CompletedAtUnixSec, 0)
		}
		accumulateQT(ie, completedAt)
	}

	// Compute network diversity: average distinct contacts per member.
	var avgDiversity float64
	if memberCount > 0 && len(contactsByUser) > 0 {
		totalContacts := 0
		for _, contacts := range contactsByUser {
			totalContacts += len(contacts)
		}
		avgDiversity = float64(totalContacts) / float64(memberCount)
	}

	// Belonging minutes: convert to hours for display.
	totalBelongingHours := totalBelongingMinutes / 60

	// Build monthly sparkline points.
	monthBuckets := c.monthlyBuckets(monthStart)
	belongingSparkline := make([]*api.TimeSeriesPoint, 6)
	trustSparkline := make([]*api.TimeSeriesPoint, 6)
	for i := 0; i < 6; i++ {
		bucketStart := proto.Int64(monthBuckets[i].Unix())
		belongingSparkline[i] = &api.TimeSeriesPoint{
			Value:              float64(belongingMonthly[i] / 60),
			BucketStartUnixSec: bucketStart,
		}
		trustSparkline[i] = &api.TimeSeriesPoint{
			Value:              float64(trustMonthly[i]),
			BucketStartUnixSec: bucketStart,
		}
	}

	// Count high-trust interactions (trust credits > 1.5 indicates high vulnerability).
	highTrustCount := 0
	for _, t := range transfers {
		ie := c.transferImpact(ctx, t, logger)
		if ie != nil && ie.QualityTime != nil && ie.QualityTime.TrustCredits != nil {
			if ie.QualityTime.TrustCredits.Mean > 1.5 {
				highTrustCount++
			}
		}
	}

	// Each metric carries its key and its raw value; the client maps the key
	// to a label and formats the value in the viewer's locale (#2835).
	return []*api.SubsidiaryMetric{
		{
			Key:       "belonging_minutes",
			Value:     float64(totalBelongingHours),
			Sparkline: belongingSparkline,
		},
		{
			Key:       "trust_credits",
			Value:     float64(totalTrustCredits),
			Sparkline: trustSparkline,
		},
		{
			Key:   "network_diversity",
			Value: avgDiversity,
		},
	}
}

// monthlyBuckets returns 6 monthly bucket start times beginning at monthStart.
func (c *MetricDetailCalculator) monthlyBuckets(monthStart time.Time) []time.Time {
	buckets := make([]time.Time, 6)
	for i := 0; i < 6; i++ {
		buckets[i] = monthStart.AddDate(0, i, 0)
	}
	return buckets
}

// computeSocialRecentActivity builds the most recent social interactions for display.
func (c *MetricDetailCalculator) computeSocialRecentActivity(
	ctx context.Context,
	logger *logging.Logger,
	transfers []*models.Transfer,
	requests []*models.Request,
	experiences []*models.Experience,
) []*api.RecentActivity {
	cache := NewEntityCache(c.storage)

	type recentItem struct {
		completedAt time.Time
		activity    *api.RecentActivity
	}

	var items []recentItem

	// Collect transfers with QT values.
	for _, t := range transfers {
		if t.ActualReturnUnixSec == nil || *t.ActualReturnUnixSec == 0 {
			continue
		}
		ie := c.transferImpact(ctx, t, logger)
		if ie == nil || ie.QualityTime == nil || ie.QualityTime.QualityTimeMinutes == nil {
			continue
		}
		sfValue := ie.QualityTime.QualityTimeMinutes.Mean

		gear := cache.GetGear(ctx, t.GearId, logger)
		gearName := ""
		if gear != nil {
			gearName = gear.Name
		}

		owner := cache.GetUser(ctx, t.OwnerId, logger)
		var ownerName *string
		ownerMediaID := ""
		if owner != nil {
			ownerName = optionalString(owner.Name)
			ownerMediaID = firstMediaID(owner.MediaIds)
		}

		items = append(items, recentItem{
			completedAt: time.Unix(*t.ActualReturnUnixSec, 0),
			activity: &api.RecentActivity{
				Kind:               api.RecentActivityKind_RECENT_ACTIVITY_KIND_GEAR,
				ContentId:          t.GearId,
				ItemName:           optionalString(gearName),
				PersonDisplayName:  ownerName,
				RawValue:           proto.Float64(float64(sfValue)),
				MediaId:            ownerMediaID,
				CompletedAtUnixSec: *t.ActualReturnUnixSec,
			},
		})
	}

	// Collect experiences with QT values.
	for _, e := range experiences {
		if e.CompletedAtUnixSec == nil || *e.CompletedAtUnixSec == 0 {
			continue
		}
		ie := c.experienceImpact(e)
		if ie == nil || ie.QualityTime == nil || ie.QualityTime.QualityTimeMinutes == nil {
			continue
		}
		sfValue := ie.QualityTime.QualityTimeMinutes.Mean

		owner := cache.GetUser(ctx, e.OwnerId, logger)
		var ownerName *string
		ownerMediaID := ""
		if owner != nil {
			ownerName = optionalString(owner.Name)
			ownerMediaID = firstMediaID(owner.MediaIds)
		}

		items = append(items, recentItem{
			completedAt: time.Unix(*e.CompletedAtUnixSec, 0),
			activity: &api.RecentActivity{
				Kind:               api.RecentActivityKind_RECENT_ACTIVITY_KIND_EXPERIENCE,
				ContentId:          e.Id,
				ItemName:           optionalString(e.Name),
				PersonDisplayName:  ownerName,
				RawValue:           proto.Float64(float64(sfValue)),
				MediaId:            ownerMediaID,
				CompletedAtUnixSec: *e.CompletedAtUnixSec,
			},
		})
	}

	// Collect requests with QT values.
	for _, r := range requests {
		ie := c.requestImpact(r)
		if ie == nil || ie.QualityTime == nil || ie.QualityTime.QualityTimeMinutes == nil {
			continue
		}
		sfValue := ie.QualityTime.QualityTimeMinutes.Mean

		requester := cache.GetUser(ctx, r.RequesterId, logger)
		var requesterName *string
		requesterMediaID := ""
		if requester != nil {
			requesterName = optionalString(requester.Name)
			requesterMediaID = firstMediaID(requester.MediaIds)
		}

		items = append(items, recentItem{
			completedAt: time.Now(),
			activity: &api.RecentActivity{
				Kind:              api.RecentActivityKind_RECENT_ACTIVITY_KIND_REQUEST,
				ContentId:         r.Id,
				ItemName:          optionalString(r.Title),
				PersonDisplayName: requesterName,
				RawValue:          proto.Float64(float64(sfValue)),
				MediaId:           requesterMediaID,
			},
		})
	}

	// Sort by completion time descending and take top 4.
	sort.Slice(items, func(i, j int) bool {
		return items[i].completedAt.After(items[j].completedAt)
	})

	const maxItems = 4
	if len(items) > maxItems {
		items = items[:maxItems]
	}

	result := make([]*api.RecentActivity, len(items))
	for i, item := range items {
		result[i] = item.activity
	}
	return result
}

// recentItemsLimit caps the recent-items list returned on each detail
// response. Eight is the largest set the detail screens render before
// requiring scroll; clients can re-query for more if needed.
const recentItemsLimit = 8

// firstMediaID returns the first non-empty entry from a media-ids slice,
// or the empty string if none is set. Used to populate the thumbnail
// field on RecentActivity entries.
func firstMediaID(ids []string) string {
	for _, id := range ids {
		if id != "" {
			return id
		}
	}
	return ""
}

// ComputeRecentItemsForActs returns the most recent contributing items
// across the union of completed transfers, events, and pitch-ins for a
// community, newest first. Each entry's Value is omitted (acts have no
// single dimension-mean to report).
func (c *MetricDetailCalculator) ComputeRecentItemsForActs(
	ctx context.Context,
	communityID string,
) ([]*api.RecentActivity, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ComputeRecentItemsForActs",
		"community_id", communityID,
	)
	ds, err := LoadCommunityDataset(ctx, c.storage, communityID)
	if err != nil {
		return nil, fmt.Errorf("load community dataset: %w", err)
	}
	transfers := FilterCompletedTransfers(ds.Transfers)
	requests := FilterFulfilledRequests(ds)
	experiences := FilterCompletedExperiences(ds)
	ctx = WithDatasetGearMap(ctx, ds)

	cache := NewEntityCacheFromDataset(c.storage, ds)
	type recentItem struct {
		completedAt time.Time
		activity    *api.RecentActivity
	}
	var items []recentItem

	for _, t := range transfers {
		if t.ActualReturnUnixSec == nil || *t.ActualReturnUnixSec == 0 {
			continue
		}
		gear := cache.GetGear(ctx, t.GearId, logger)
		gearName, gearMediaID := "", ""
		if gear != nil {
			gearName = gear.Name
			gearMediaID = firstMediaID(gear.MediaIds)
		}
		var ownerName *string
		if owner := cache.GetUser(ctx, t.OwnerId, logger); owner != nil {
			ownerName = optionalString(owner.Name)
		}
		completedAt := *t.ActualReturnUnixSec
		items = append(items, recentItem{
			completedAt: time.Unix(completedAt, 0),
			activity: &api.RecentActivity{
				ItemName:           optionalString(gearName),
				PersonDisplayName:  ownerName,
				MediaId:            gearMediaID,
				ContentId:          t.GearId,
				Kind:               api.RecentActivityKind_RECENT_ACTIVITY_KIND_GEAR,
				CompletedAtUnixSec: completedAt,
			},
		})
	}
	for _, e := range experiences {
		if e.CompletedAtUnixSec == nil || *e.CompletedAtUnixSec == 0 {
			continue
		}
		var ownerName *string
		if owner := cache.GetUser(ctx, e.OwnerId, logger); owner != nil {
			ownerName = optionalString(owner.Name)
		}
		completedAt := *e.CompletedAtUnixSec
		items = append(items, recentItem{
			completedAt: time.Unix(completedAt, 0),
			activity: &api.RecentActivity{
				ItemName:           optionalString(e.Name),
				PersonDisplayName:  ownerName,
				MediaId:            firstMediaID(e.MediaIds),
				ContentId:          e.Id,
				Kind:               api.RecentActivityKind_RECENT_ACTIVITY_KIND_EXPERIENCE,
				CompletedAtUnixSec: completedAt,
			},
		})
	}
	for _, r := range requests {
		if r.FulfilledAtUnixSec == nil || *r.FulfilledAtUnixSec == 0 {
			continue
		}
		var requesterName *string
		if requester := cache.GetUser(ctx, r.RequesterId, logger); requester != nil {
			requesterName = optionalString(requester.Name)
		}
		completedAt := *r.FulfilledAtUnixSec
		items = append(items, recentItem{
			completedAt: time.Unix(completedAt, 0),
			activity: &api.RecentActivity{
				ItemName:           optionalString(r.Title),
				PersonDisplayName:  requesterName,
				MediaId:            firstMediaID(r.MediaIds),
				ContentId:          r.Id,
				Kind:               api.RecentActivityKind_RECENT_ACTIVITY_KIND_REQUEST,
				CompletedAtUnixSec: completedAt,
			},
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].completedAt.After(items[j].completedAt)
	})
	if len(items) > recentItemsLimit {
		items = items[:recentItemsLimit]
	}
	result := make([]*api.RecentActivity, len(items))
	for i, it := range items {
		result[i] = it.activity
	}
	logger.DebugContext(ctx, "computed recent items for acts",
		"result_count", len(result),
	)
	return result, nil
}

// computeRecentItems returns the most recent contributing items for the
// given dimension, newest first, capped at recentItemsLimit. The value
// string on each entry is formatted to match the dimension (e.g.
// "$340" for MONEY, "2.5 hrs" for TIME).
func (c *MetricDetailCalculator) computeRecentItems(
	ctx context.Context,
	logger *logging.Logger,
	transfers []*models.Transfer,
	requests []*models.Request,
	experiences []*models.Experience,
	dimension api.ImpactMetricDimension,
) []*api.RecentActivity {
	cache := NewEntityCache(c.storage)

	type recentItem struct {
		completedAt time.Time
		activity    *api.RecentActivity
	}
	var items []recentItem

	for _, t := range transfers {
		if t.ActualReturnUnixSec == nil || *t.ActualReturnUnixSec == 0 {
			continue
		}
		ie := c.transferImpact(ctx, t, logger)
		value := extractDimensionValue(ie, dimension)
		if value == nil {
			continue
		}
		gear := cache.GetGear(ctx, t.GearId, logger)
		gearName, gearMediaID := "", ""
		if gear != nil {
			gearName = gear.Name
			gearMediaID = firstMediaID(gear.MediaIds)
		}
		ownerName := ""
		if owner := cache.GetUser(ctx, t.OwnerId, logger); owner != nil {
			ownerName = owner.Name
		}
		completedAt := *t.ActualReturnUnixSec
		items = append(items, recentItem{
			completedAt: time.Unix(completedAt, 0),
			activity: &api.RecentActivity{
				ItemName:           optionalString(gearName),
				PersonDisplayName:  optionalString(ownerName),
				RawValue:           proto.Float64(float64(*value)),
				MediaId:            gearMediaID,
				ContentId:          t.GearId,
				Kind:               api.RecentActivityKind_RECENT_ACTIVITY_KIND_GEAR,
				CompletedAtUnixSec: completedAt,
			},
		})
	}

	for _, e := range experiences {
		if e.CompletedAtUnixSec == nil || *e.CompletedAtUnixSec == 0 {
			continue
		}
		ie := c.experienceImpact(e)
		value := extractDimensionValue(ie, dimension)
		if value == nil {
			continue
		}
		ownerName := ""
		if owner := cache.GetUser(ctx, e.OwnerId, logger); owner != nil {
			ownerName = owner.Name
		}
		completedAt := *e.CompletedAtUnixSec
		items = append(items, recentItem{
			completedAt: time.Unix(completedAt, 0),
			activity: &api.RecentActivity{
				ItemName:           optionalString(e.Name),
				PersonDisplayName:  optionalString(ownerName),
				RawValue:           proto.Float64(float64(*value)),
				MediaId:            firstMediaID(e.MediaIds),
				ContentId:          e.Id,
				Kind:               api.RecentActivityKind_RECENT_ACTIVITY_KIND_EXPERIENCE,
				CompletedAtUnixSec: completedAt,
			},
		})
	}

	for _, r := range requests {
		ie := c.requestImpact(r)
		value := extractDimensionValue(ie, dimension)
		if value == nil {
			continue
		}
		if r.FulfilledAtUnixSec == nil || *r.FulfilledAtUnixSec == 0 {
			continue
		}
		requesterName := ""
		if requester := cache.GetUser(ctx, r.RequesterId, logger); requester != nil {
			requesterName = requester.Name
		}
		completedAt := *r.FulfilledAtUnixSec
		items = append(items, recentItem{
			completedAt: time.Unix(completedAt, 0),
			activity: &api.RecentActivity{
				ItemName:           optionalString(r.Title),
				PersonDisplayName:  optionalString(requesterName),
				RawValue:           proto.Float64(float64(*value)),
				MediaId:            firstMediaID(r.MediaIds),
				ContentId:          r.Id,
				Kind:               api.RecentActivityKind_RECENT_ACTIVITY_KIND_REQUEST,
				CompletedAtUnixSec: completedAt,
			},
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].completedAt.After(items[j].completedAt)
	})
	if len(items) > recentItemsLimit {
		items = items[:recentItemsLimit]
	}
	result := make([]*api.RecentActivity, len(items))
	for i, it := range items {
		result[i] = it.activity
	}
	return result
}

// extractDimensionValue returns the mean of the dimension-specific estimate
// on an ImpactEstimate, or nil if that dimension is not populated. Emissions
// values are returned in kilograms (the underlying estimate is in grams).
func extractDimensionValue(ie *api.ImpactEstimate, dimension api.ImpactMetricDimension) *float32 {
	if ie == nil {
		return nil
	}
	switch dimension {
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY:
		est := ie.GetMoneySaved().GetValueUsd()
		if est == nil {
			return nil
		}
		v := est.Mean
		return &v
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_TIME:
		est := ie.GetTimeSaved().GetMinutes()
		if est == nil {
			return nil
		}
		v := est.Mean
		return &v
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS:
		est := ie.GetEmissionsPrevented().GetManufactureAvoidedCarbon().GetCo2EGrams()
		if est == nil {
			return nil
		}
		v := est.Mean / 1000.0 // grams → kg
		return &v
	case api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME:
		est := ie.GetQualityTime().GetQualityTimeMinutes()
		if est == nil {
			return nil
		}
		v := est.Mean
		return &v
	default:
		return nil
	}
}
