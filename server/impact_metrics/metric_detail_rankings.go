package impact_metrics

import (
	"context"
	"sort"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// EntityCache provides request-scoped caching to avoid N+1 GetByID queries.
type EntityCache struct {
	storage *storage.ProtoSQLStorage
	cache   map[string]interface{}
}

// NewEntityCache creates a new entity cache.
func NewEntityCache(storage *storage.ProtoSQLStorage) *EntityCache {
	return &EntityCache{
		storage: storage,
		cache:   make(map[string]interface{}),
	}
}

// NewEntityCacheFromDataset creates a new entity cache prepopulated
// with the gear rows from a preloaded CommunityDataset. Subsequent
// GetGear calls for any gear id present in the dataset become map
// hits with no DB round-trip — the rest of the metric-detail helpers
// (computeTopItems, computeTopContributors, computeFactors, …) keep
// using the same GetGear API but stop fanning out. See #2055.
func NewEntityCacheFromDataset(s *storage.ProtoSQLStorage, ds *CommunityDataset) *EntityCache {
	c := NewEntityCache(s)
	for id, gear := range ds.Gear {
		c.cache["gear:"+id] = gear
	}
	return c
}

// GetGear loads a gear item, using cache if available.
func (c *EntityCache) GetGear(ctx context.Context, gearID string, logger *logging.Logger) *models.Gear {
	key := "gear:" + gearID
	if cached, ok := c.cache[key]; ok {
		return cached.(*models.Gear)
	}

	gear := &models.Gear{}
	if err := c.storage.GetByID(ctx, gearID, gear); err != nil {
		logger.DebugContext(ctx, "could not load gear", "gear_id", gearID, "error", err)
		return nil
	}

	c.cache[key] = gear
	return gear
}

// GetUser loads a user, using cache if available.
func (c *EntityCache) GetUser(ctx context.Context, userID string, logger *logging.Logger) *models.User {
	key := "user:" + userID
	if cached, ok := c.cache[key]; ok {
		return cached.(*models.User)
	}

	user := &models.User{}
	if err := c.storage.GetByID(ctx, userID, user); err != nil {
		logger.DebugContext(ctx, "could not load user", "user_id", userID, "error", err)
		return nil
	}

	c.cache[key] = user
	return user
}

// computeRecentActivity returns recent transactions of a specific type.
// recentActivityLimit caps how many recent transactions a metric detail shows.
const recentActivityLimit = 3

// recentTransferActivity returns the most recently completed transfers of one
// type, newest first, as activity rows. Loans and giveaways differ only in
// which transfer type they select and which party is the counterparty — the
// recipient in both cases (borrower / new owner).
func (c *MetricDetailCalculator) recentTransferActivity(
	ctx context.Context,
	transfers []*models.Transfer,
	transferType models.TransferType,
	dimension api.ImpactMetricDimension,
	cache *EntityCache,
	logger *logging.Logger,
) []*api.RecentActivity {
	var matching []*models.Transfer
	for _, transfer := range transfers {
		if transfer.TransferType == transferType {
			matching = append(matching, transfer)
		}
	}
	sort.Slice(matching, func(i, j int) bool {
		return matching[i].GetActualReturnUnixSec() > matching[j].GetActualReturnUnixSec()
	})

	var items []*api.RecentActivity
	for _, transfer := range matching {
		if len(items) >= recentActivityLimit {
			break
		}

		gear := cache.GetGear(ctx, transfer.GearId, logger)
		if gear == nil {
			continue
		}

		var personName *string
		mediaID := ""
		if recipient := cache.GetUser(ctx, transfer.RecipientId, logger); recipient != nil {
			personName = optionalString(recipient.Name)
			mediaID = firstMediaID(recipient.MediaIds)
		}

		value := float32(0)
		if estimate := c.extractEstimate(c.transferImpactForSeries(transfer), dimension); estimate != nil {
			value = estimate.Mean
		}

		items = append(items, &api.RecentActivity{
			Kind:               api.RecentActivityKind_RECENT_ACTIVITY_KIND_GEAR,
			ContentId:          transfer.GearId,
			ItemName:           optionalString(gear.Name),
			PersonDisplayName:  personName,
			RawValue:           proto.Float64(float64(value)),
			MediaId:            mediaID,
			CompletedAtUnixSec: transfer.GetActualReturnUnixSec(),
		})
	}
	return items
}

func (c *MetricDetailCalculator) computeRecentActivity(
	ctx context.Context,
	transfers []*models.Transfer,
	requests []*models.Request,
	experiences []*models.Experience,
	sourceType api.ImpactSourceType,
	dimension api.ImpactMetricDimension,
	cache *EntityCache,
) []*api.RecentActivity {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "computeRecentActivity",
		"source_type", sourceType.String(),
	)

	var items []*api.RecentActivity

	switch sourceType {
	case api.ImpactSourceType_IMPACT_SOURCE_TYPE_LOANS:
		items = c.recentTransferActivity(ctx, transfers,
			models.TransferType_TRANSFER_TYPE_LOAN, dimension, cache, logger)

	case api.ImpactSourceType_IMPACT_SOURCE_TYPE_GIVEAWAYS:
		items = c.recentTransferActivity(ctx, transfers,
			models.TransferType_TRANSFER_TYPE_GIVEAWAY, dimension, cache, logger)

	case api.ImpactSourceType_IMPACT_SOURCE_TYPE_REQUESTS:
		// Sort requests by ID as proxy for recency (no completion timestamp)
		sort.Slice(requests, func(i, j int) bool {
			return requests[i].Id > requests[j].Id
		})

		count := 0
		for _, request := range requests {
			if count >= recentActivityLimit {
				break
			}

			// Skip requests without a confirmed helper
			if len(request.ConfirmedHelperIds) == 0 {
				continue
			}

			// Use first confirmed helper
			personName := ""
			mediaID := ""
			helper := cache.GetUser(ctx, request.ConfirmedHelperIds[0], logger)
			if helper != nil {
				personName = helper.Name
				mediaID = firstMediaID(helper.MediaIds)
			}

			// Skip if we couldn't load the helper user
			if personName == "" {
				continue
			}

			ie := c.requestImpact(request)
			estimate := c.extractEstimate(ie, dimension)
			value := float32(0)
			if estimate != nil {
				value = estimate.Mean
			}

			items = append(items, &api.RecentActivity{
				Kind:              api.RecentActivityKind_RECENT_ACTIVITY_KIND_REQUEST,
				ContentId:         request.Id,
				ItemName:          optionalString(request.Title),
				PersonDisplayName: proto.String(personName),
				RawValue:          proto.Float64(float64(value)),
				MediaId:           mediaID,
			})
			count++
		}

	case api.ImpactSourceType_IMPACT_SOURCE_TYPE_EVENTS:
		// Sort experiences by completion timestamp
		sort.Slice(experiences, func(i, j int) bool {
			iTime := int64(0)
			jTime := int64(0)
			if experiences[i].CompletedAtUnixSec != nil {
				iTime = *experiences[i].CompletedAtUnixSec
			}
			if experiences[j].CompletedAtUnixSec != nil {
				jTime = *experiences[j].CompletedAtUnixSec
			}
			return iTime > jTime
		})

		count := 0
		for _, experience := range experiences {
			if count >= recentActivityLimit {
				break
			}

			ie := c.experienceImpact(experience)
			estimate := c.extractEstimate(ie, dimension)
			value := float32(0)
			if estimate != nil {
				value = estimate.Mean
			}

			var personName *string
			mediaID := ""
			if experience.OwnerId != "" {
				organizer := cache.GetUser(ctx, experience.OwnerId, logger)
				if organizer != nil {
					if organizer.Name != "" {
						personName = proto.String(organizer.Name)
					}
					mediaID = firstMediaID(organizer.MediaIds)
				}
			}

			completedAt := int64(0)
			if experience.CompletedAtUnixSec != nil {
				completedAt = *experience.CompletedAtUnixSec
			}

			items = append(items, &api.RecentActivity{
				Kind:               api.RecentActivityKind_RECENT_ACTIVITY_KIND_EXPERIENCE,
				ContentId:          experience.Id,
				ItemName:           optionalString(experience.Name),
				PersonDisplayName:  personName,
				RawValue:           proto.Float64(float64(value)),
				MediaId:            mediaID,
				CompletedAtUnixSec: completedAt,
			})
			count++
		}
	}

	logger.DebugContext(ctx, "computed recent activity", "count", len(items))
	return items
}

// computeTopItems ranks items by cumulative impact.
//
// Includes transfers (loans/giveaways), requests, and events. No limit is
// applied — all items are returned sorted by cumulative value descending.
// Ranking is done on the float the estimator produced; the sort used to
// re-parse the server's own formatted display strings, which went away with
// those strings in #2835.
func (c *MetricDetailCalculator) computeTopItems(
	ctx context.Context,
	transfers []*models.Transfer,
	requests []*models.Request,
	experiences []*models.Experience,
	dimension api.ImpactMetricDimension,
	cache *EntityCache,
) []*api.TopItem {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "computeTopItems",
		"dimension", dimension.String(),
	)

	// rankedItem pairs an item with the float it ranks on, so the sort never
	// has to read the value back out of a formatted string.
	type rankedItem struct {
		item  *api.TopItem
		value float32
	}
	var ranked []rankedItem

	// --- Transfers: group by gear_id per source type ---
	type gearStats struct {
		gearID          string
		sourceType      api.ImpactSourceType
		cumulativeValue float32
		loanCount       int32
	}

	gearMap := make(map[string]*gearStats)
	for _, transfer := range transfers {
		ie := c.transferImpactForSeries(transfer)
		estimate := c.extractEstimate(ie, dimension)
		if estimate == nil {
			continue
		}

		sourceType := api.ImpactSourceType_IMPACT_SOURCE_TYPE_LOANS
		if transfer.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
			sourceType = api.ImpactSourceType_IMPACT_SOURCE_TYPE_GIVEAWAYS
		}

		// Key by gear+source so the same gear can appear under both Loans and Giveaways.
		key := transfer.GearId + ":" + sourceType.String()
		if _, exists := gearMap[key]; !exists {
			gearMap[key] = &gearStats{gearID: transfer.GearId, sourceType: sourceType}
		}

		gearMap[key].cumulativeValue += estimate.Mean
		gearMap[key].loanCount++
	}

	for _, stats := range gearMap {
		gear := cache.GetGear(ctx, stats.gearID, logger)
		if gear == nil {
			continue
		}

		owner := cache.GetUser(ctx, gear.OwnerId, logger)
		sharerName := ""
		if owner != nil {
			sharerName = owner.Name
		}

		ranked = append(ranked, rankedItem{
			item: &api.TopItem{
				GearId:     stats.gearID,
				Name:       gear.Name,
				SharerName: sharerName,
				LoanCount:  stats.loanCount,
			},
			value: stats.cumulativeValue,
		})
	}

	// --- Requests ---
	for _, request := range requests {
		if len(request.ConfirmedHelperIds) == 0 {
			continue
		}

		ie := c.requestImpact(request)
		estimate := c.extractEstimate(ie, dimension)
		if estimate == nil {
			continue
		}

		helper := cache.GetUser(ctx, request.ConfirmedHelperIds[0], logger)
		helperName := ""
		if helper != nil {
			helperName = helper.Name
		}

		ranked = append(ranked, rankedItem{
			item: &api.TopItem{
				Name:       request.Title,
				SharerName: helperName,
			},
			value: estimate.Mean,
		})
	}

	// --- Events (experiences) ---
	for _, experience := range experiences {
		ie := c.experienceImpact(experience)
		estimate := c.extractEstimate(ie, dimension)
		if estimate == nil {
			continue
		}

		organizerName := ""
		if experience.OwnerId != "" {
			organizer := cache.GetUser(ctx, experience.OwnerId, logger)
			if organizer != nil {
				organizerName = organizer.Name
			}
		}

		ranked = append(ranked, rankedItem{
			item: &api.TopItem{
				Name:       experience.Name,
				SharerName: organizerName,
			},
			value: estimate.Mean,
		})
	}

	// Sort all items by cumulative value descending.
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].value > ranked[j].value
	})

	topItems := make([]*api.TopItem, len(ranked))
	for i, r := range ranked {
		topItems[i] = r.item
	}

	logger.DebugContext(ctx, "computed top items", "count", len(topItems))
	return topItems
}

// userStats tracks a user's contribution statistics.
type userStats struct {
	userID          string
	totalValue      float32
	itemsListed     int
	loansShared     int
	giveawaysShared int
	requestsHelped  int
	eventsOrganized int
}

// computeTopContributors ranks users by their contribution to impact.
func (c *MetricDetailCalculator) computeTopContributors(
	ctx context.Context,
	transfers []*models.Transfer,
	requests []*models.Request,
	experiences []*models.Experience,
	dimension api.ImpactMetricDimension,
	limit int,
	cache *EntityCache,
) []*api.TopContributor {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "computeTopContributors",
		"dimension", dimension.String(),
	)

	// Group by user and sum contribution
	userMap := make(map[string]*userStats)

	// Track contributions from transfers (as gear owner)
	for _, transfer := range transfers {
		gear := cache.GetGear(ctx, transfer.GearId, logger)
		if gear == nil {
			continue
		}

		ownerID := gear.OwnerId
		if _, exists := userMap[ownerID]; !exists {
			userMap[ownerID] = &userStats{userID: ownerID}
		}

		ie := c.transferImpactForSeries(transfer)
		estimate := c.extractEstimate(ie, dimension)
		if estimate != nil {
			userMap[ownerID].totalValue += estimate.Mean
		}

		switch transfer.TransferType {
		case models.TransferType_TRANSFER_TYPE_LOAN:
			userMap[ownerID].loansShared++
		case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
			userMap[ownerID].giveawaysShared++
		}
	}

	// Track contributions from requests (as helper)
	for _, request := range requests {
		// Use first confirmed helper if available
		if len(request.ConfirmedHelperIds) == 0 {
			continue
		}

		helperID := request.ConfirmedHelperIds[0]
		if _, exists := userMap[helperID]; !exists {
			userMap[helperID] = &userStats{userID: helperID}
		}

		ie := c.requestImpact(request)
		estimate := c.extractEstimate(ie, dimension)
		if estimate != nil {
			userMap[helperID].totalValue += estimate.Mean
		}

		userMap[helperID].requestsHelped++
	}

	// Track contributions from experiences (as owner/organizer)
	for _, experience := range experiences {
		organizerID := experience.OwnerId
		if organizerID == "" {
			continue
		}

		if _, exists := userMap[organizerID]; !exists {
			userMap[organizerID] = &userStats{userID: organizerID}
		}

		ie := c.experienceImpact(experience)
		estimate := c.extractEstimate(ie, dimension)
		if estimate != nil {
			userMap[organizerID].totalValue += estimate.Mean
		}

		userMap[organizerID].eventsOrganized++
	}

	// Count items listed per user
	for gearID := range c.countListedItemsByOwner(ctx, transfers, cache, logger) {
		gear := cache.GetGear(ctx, gearID, logger)
		if gear == nil {
			continue
		}

		ownerID := gear.OwnerId
		if stats, exists := userMap[ownerID]; exists {
			stats.itemsListed++
		}
	}

	// Convert to slice and sort by total value
	var allUsers []*userStats
	for _, stats := range userMap {
		allUsers = append(allUsers, stats)
	}

	sort.Slice(allUsers, func(i, j int) bool {
		return allUsers[i].totalValue > allUsers[j].totalValue
	})

	// Build top contributors with loaded user info
	var topContributors []*api.TopContributor
	count := 0
	for _, stats := range allUsers {
		if count >= limit {
			break
		}

		user := cache.GetUser(ctx, stats.userID, logger)
		if user == nil {
			continue
		}

		// Name goes on the wire as-is — empty when unknown, rather than the
		// server inventing an English "Unknown User" (#2835).
		topContributors = append(topContributors, &api.TopContributor{
			UserId:  stats.userID,
			Name:    user.Name,
			Value:   c.formatValue(stats.totalValue, dimension),
			MediaId: services.PrimaryAvatarMediaID(user),
		})
		count++
	}

	logger.DebugContext(ctx, "computed top contributors", "count", len(topContributors))
	return topContributors
}

// countListedItemsByOwner returns a set of unique gear IDs from transfers.
func (c *MetricDetailCalculator) countListedItemsByOwner(
	_ context.Context,
	transfers []*models.Transfer,
	_ *EntityCache,
	_ *logging.Logger,
) map[string]bool {
	gearIDs := make(map[string]bool)
	for _, transfer := range transfers {
		gearIDs[transfer.GearId] = true
	}
	return gearIDs
}

// computeLibraryItems returns all active gear in a community sorted by value
// descending, for the LIBRARY pill drill-down view. Unlike computeTopItems,
// this includes gear with zero completed transactions so the list is never
// empty just because no loans have occurred yet. Uses batch queries to avoid
// N+1 patterns.
func (c *MetricDetailCalculator) computeLibraryItems(
	ctx context.Context,
	communityID string,
	transfers []*models.Transfer,
) []*api.TopItem {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "computeLibraryItems",
		"community_id", communityID,
	)

	communityGear, err := resolveCommunityGear(ctx, c.storage, communityID)
	if err != nil {
		logger.WarnContext(ctx, "failed to query community gear for library items", "error", err)
		return nil
	}

	gearIDs := make([]string, 0, len(communityGear))
	seen := make(map[string]bool, len(communityGear))
	for _, cg := range communityGear {
		if cg.Archived || seen[cg.GearId] {
			continue
		}
		seen[cg.GearId] = true
		gearIDs = append(gearIDs, cg.GearId)
	}
	if len(gearIDs) == 0 {
		return nil
	}

	// Prefer the preloaded gear map from the dataset when available;
	// otherwise batch-fetch from storage.
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
			logger.WarnContext(ctx, "failed to batch-fetch gear for library items", "error", err)
			return nil
		}
	}

	// Collect unique owner IDs for batch user lookup.
	ownerIDs := make([]string, 0, len(gearMap))
	ownerSeen := make(map[string]bool, len(gearMap))
	for _, g := range gearMap {
		if g.OwnerId != "" && !ownerSeen[g.OwnerId] {
			ownerSeen[g.OwnerId] = true
			ownerIDs = append(ownerIDs, g.OwnerId)
		}
	}
	userMap, err := storage.GetByIDs[*models.User](c.storage, ctx, ownerIDs)
	if err != nil {
		logger.WarnContext(ctx, "failed to batch-fetch users for library items, continuing without names", "error", err)
		userMap = map[string]*models.User{}
	}

	// Count completed loans per gear from already-loaded transfers.
	loanCounts := make(map[string]int32, len(transfers))
	for _, t := range transfers {
		if t.GearId != "" {
			loanCounts[t.GearId]++
		}
	}

	type entry struct {
		item  *api.TopItem
		value float64
	}
	entries := make([]entry, 0, len(gearMap))

	for gearID, gear := range gearMap {
		if gear.Deleted != nil && gear.Deleted.DeletedAtUnixSec > 0 {
			continue
		}

		sharerName := ""
		if u, ok := userMap[gear.OwnerId]; ok {
			sharerName = u.Name
		}

		var valueForSort float64
		if gear.ValueEstimate != nil {
			valueForSort = float64(gear.ValueEstimate.EstimatedValueUsd)
		}

		entries = append(entries, entry{
			item: &api.TopItem{
				GearId:     gearID,
				Name:       gear.Name,
				SharerName: sharerName,
				LoanCount:  loanCounts[gearID],
			},
			value: valueForSort,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].value > entries[j].value
	})

	items := make([]*api.TopItem, len(entries))
	for i, e := range entries {
		items[i] = e.item
	}
	return items
}
