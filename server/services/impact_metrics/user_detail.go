package impact_metrics

import (
	"context"
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	impactlib "go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// userSourceBreakdowns computes source breakdowns for money, CO₂, and quality
// time dimensions from a user's completed transactions.
func userSourceBreakdowns(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID string,
	logger *logging.Logger,
) (money, co2, qt []*api.SourceBreakdown) {
	// Load completed transfers owned by this user.
	loansRaw, err := storage.QueryByField[*models.Transfer](store, ctx, "owner_id", userID)
	if err != nil {
		logger.DebugContext(ctx, "failed to query user transfers", "error", err)
		return money, co2, qt
	}

	var loanMoney, giveawayMoney float64
	var loanCO2, giveawayCO2 float64
	var loanQT, giveawayQT float64

	for _, t := range loansRaw {
		if t.Deleted != nil || t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		ie := t.ImpactEstimate
		if ie == nil {
			continue
		}
		moneySaved := float64(0)
		if ie.MoneySaved != nil && ie.MoneySaved.ValueUsd != nil {
			moneySaved = float64(ie.MoneySaved.ValueUsd.Mean)
		}
		co2Saved := float64(0)
		if ie.EmissionsPrevented != nil {
			if ie.EmissionsPrevented.ManufactureAvoidedCarbon != nil && ie.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams != nil {
				co2Saved += float64(ie.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams.Mean)
			}
			if ie.EmissionsPrevented.WasteReducedCarbon != nil && ie.EmissionsPrevented.WasteReducedCarbon.Co2EGrams != nil {
				co2Saved += float64(ie.EmissionsPrevented.WasteReducedCarbon.Co2EGrams.Mean)
			}
		}
		qtSaved := float64(0)
		if ie.QualityTime != nil && ie.QualityTime.QualityTimeMinutes != nil {
			qtSaved = float64(ie.QualityTime.QualityTimeMinutes.Mean)
		}

		switch t.TransferType {
		case models.TransferType_TRANSFER_TYPE_LOAN:
			loanMoney += moneySaved
			loanCO2 += co2Saved
			loanQT += qtSaved
		case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
			giveawayMoney += moneySaved
			giveawayCO2 += co2Saved
			giveawayQT += qtSaved
		}
	}

	// Load fulfilled requests where user is a helper.
	requestMoney, requestCO2, requestQT := userRequestImpact(ctx, store, userID, logger)

	// Load completed experiences owned by user.
	eventMoney, eventCO2, eventQT := userExperienceImpact(ctx, store, userID, logger)

	money = buildSourceBreakdown(loanMoney, giveawayMoney, requestMoney, eventMoney)
	co2 = buildSourceBreakdown(loanCO2, giveawayCO2, requestCO2, eventCO2)
	qt = buildSourceBreakdown(loanQT, giveawayQT, requestQT, eventQT)
	return money, co2, qt
}

// userRequestImpact computes total impact from requests where user is a helper.
func userRequestImpact(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID string,
	logger *logging.Logger,
) (money, co2, qt float64) {
	requestsRaw, err := store.QueryByField(ctx, "confirmed_helper_ids", userID, &models.Request{})
	if err != nil {
		logger.DebugContext(ctx, "failed to query user requests", "error", err)
		return money, co2, qt
	}
	for _, raw := range requestsRaw {
		r := raw.(*models.Request)
		if r.Deleted != nil || r.State != models.RequestState_REQUEST_STATE_FULFILLED {
			continue
		}
		ie := r.ImpactEstimate
		if ie == nil {
			continue
		}
		if ie.MoneySaved != nil && ie.MoneySaved.ValueUsd != nil {
			money += float64(ie.MoneySaved.ValueUsd.Mean)
		}
		if ie.EmissionsPrevented != nil {
			if ie.EmissionsPrevented.ManufactureAvoidedCarbon != nil && ie.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams != nil {
				co2 += float64(ie.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams.Mean)
			}
		}
		if ie.QualityTime != nil && ie.QualityTime.QualityTimeMinutes != nil {
			qt += float64(ie.QualityTime.QualityTimeMinutes.Mean)
		}
	}
	return money, co2, qt
}

// userExperienceImpact computes total impact from experiences owned by user.
func userExperienceImpact(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID string,
	logger *logging.Logger,
) (money, co2, qt float64) {
	experiencesRaw, err := storage.QueryByField[*models.Experience](store, ctx, "owner_id", userID)
	if err != nil {
		logger.DebugContext(ctx, "failed to query user experiences", "error", err)
		return money, co2, qt
	}
	for _, e := range experiencesRaw {
		if e.Deleted != nil || e.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			continue
		}
		ie := e.ImpactEstimate
		if ie == nil {
			continue
		}
		if ie.MoneySaved != nil && ie.MoneySaved.ValueUsd != nil {
			money += float64(ie.MoneySaved.ValueUsd.Mean)
		}
		if ie.EmissionsPrevented != nil {
			if ie.EmissionsPrevented.ManufactureAvoidedCarbon != nil && ie.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams != nil {
				co2 += float64(ie.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams.Mean)
			}
		}
		if ie.QualityTime != nil && ie.QualityTime.QualityTimeMinutes != nil {
			qt += float64(ie.QualityTime.QualityTimeMinutes.Mean)
		}
	}
	return money, co2, qt
}

// buildSourceBreakdown creates source breakdown entries from per-type totals.
func buildSourceBreakdown(loans, giveaways, requests, events float64) []*api.SourceBreakdown {
	total := loans + giveaways + requests + events
	if total <= 0 {
		return nil
	}

	var result []*api.SourceBreakdown
	addSource := func(sourceType api.ImpactSourceType, value float64) {
		if value <= 0 {
			return
		}
		result = append(result, &api.SourceBreakdown{
			SourceType: sourceType,
			Value:      value,
			Percentage: value / total * 100,
		})
	}

	addSource(api.ImpactSourceType_IMPACT_SOURCE_TYPE_LOANS, loans)
	addSource(api.ImpactSourceType_IMPACT_SOURCE_TYPE_GIVEAWAYS, giveaways)
	addSource(api.ImpactSourceType_IMPACT_SOURCE_TYPE_REQUESTS, requests)
	addSource(api.ImpactSourceType_IMPACT_SOURCE_TYPE_EVENTS, events)

	return result
}

// userCo2Trend computes cumulative CO₂ savings (in grams) over time for a user.
func userCo2Trend(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID string,
	_ *logging.Logger,
) []*api.TimeSeriesPoint {
	loansRaw, err := storage.QueryByField[*models.Transfer](store, ctx, "owner_id", userID)
	if err != nil {
		return nil
	}

	monthlyMap := make(map[string]float64)
	for _, t := range loansRaw {
		if t.Deleted != nil || t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		if t.ActualReturnUnixSec == nil || t.ImpactEstimate == nil {
			continue
		}
		ts := time.Unix(*t.ActualReturnUnixSec, 0)
		key := fmt.Sprintf("%d-%02d", ts.Year(), ts.Month())
		if ep := t.ImpactEstimate.EmissionsPrevented; ep != nil {
			if ep.ManufactureAvoidedCarbon != nil && ep.ManufactureAvoidedCarbon.Co2EGrams != nil {
				monthlyMap[key] += float64(ep.ManufactureAvoidedCarbon.Co2EGrams.Mean)
			}
			if ep.WasteReducedCarbon != nil && ep.WasteReducedCarbon.Co2EGrams != nil {
				monthlyMap[key] += float64(ep.WasteReducedCarbon.Co2EGrams.Mean)
			}
		}
	}

	if len(monthlyMap) == 0 {
		return nil
	}

	var months []string
	for m := range monthlyMap {
		months = append(months, m)
	}
	sort.Strings(months)

	if len(months) > 6 {
		months = months[len(months)-6:]
	}

	var points []*api.TimeSeriesPoint
	var cumulative float64
	for _, m := range months {
		cumulative += monthlyMap[m]
		ts, _ := time.Parse("2006-01", m)
		points = append(points, &api.TimeSeriesPoint{
			Value:              cumulative,
			BucketStartUnixSec: proto.Int64(ts.Unix()),
		})
	}
	return points
}

// userQualityTimeTrend computes cumulative quality-time minutes over time for a user.
func userQualityTimeTrend(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID string,
	_ *logging.Logger,
) []*api.TimeSeriesPoint {
	loansRaw, err := storage.QueryByField[*models.Transfer](store, ctx, "owner_id", userID)
	if err != nil {
		return nil
	}

	monthlyMap := make(map[string]float64)
	for _, t := range loansRaw {
		if t.Deleted != nil || t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		if t.ActualReturnUnixSec == nil || t.ImpactEstimate == nil {
			continue
		}
		ts := time.Unix(*t.ActualReturnUnixSec, 0)
		key := fmt.Sprintf("%d-%02d", ts.Year(), ts.Month())
		if qt := t.ImpactEstimate.QualityTime; qt != nil && qt.QualityTimeMinutes != nil {
			monthlyMap[key] += float64(qt.QualityTimeMinutes.Mean)
		}
	}

	if len(monthlyMap) == 0 {
		return nil
	}

	var months []string
	for m := range monthlyMap {
		months = append(months, m)
	}
	sort.Strings(months)

	if len(months) > 6 {
		months = months[len(months)-6:]
	}

	var points []*api.TimeSeriesPoint
	var cumulative float64
	for _, m := range months {
		cumulative += monthlyMap[m]
		ts, _ := time.Parse("2006-01", m)
		points = append(points, &api.TimeSeriesPoint{
			Value:              cumulative,
			BucketStartUnixSec: proto.Int64(ts.Unix()),
		})
	}
	return points
}

// userCommunityRanking returns the user's communities ranked by money saved
// (all-dimension aggregate). A maximum of 10 communities are returned.
func userCommunityRanking(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID string,
	logger *logging.Logger,
) []*api.UserRankedCommunity {
	// Get all communities the user belongs to.
	memberships, err := storage.QueryByField[*models.CommunityUser](store, ctx, "user_id", userID)
	if err != nil {
		logger.WarnContext(ctx, "failed to load community memberships for ranking", "error", err)
		return nil
	}
	communitySet := make(map[string]bool, len(memberships))
	for _, m := range memberships {
		communitySet[m.CommunityId] = true
	}
	if len(communitySet) == 0 {
		return nil
	}

	// Sum money saved per community from owner-side completed transfers.
	ownerTransfers, err := storage.QueryByField[*models.Transfer](store, ctx, "owner_id", userID)
	if err != nil {
		logger.WarnContext(ctx, "failed to load owner transfers for community ranking", "error", err)
	}
	recipientTransfers, err := storage.QueryByField[*models.Transfer](store, ctx, "recipient_id", userID)
	if err != nil {
		logger.WarnContext(ctx, "failed to load recipient transfers for community ranking", "error", err)
	}

	communityTotals := make(map[string]float64, len(communitySet))
	seen := make(map[string]bool)
	for _, batch := range [][]*models.Transfer{ownerTransfers, recipientTransfers} {
		for _, t := range batch {
			if seen[t.Id] || t.Deleted != nil || t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
				continue
			}
			if !communitySet[t.CommunityId] {
				continue
			}
			seen[t.Id] = true
			if t.ImpactEstimate != nil && t.ImpactEstimate.MoneySaved != nil && t.ImpactEstimate.MoneySaved.ValueUsd != nil {
				communityTotals[t.CommunityId] += float64(t.ImpactEstimate.MoneySaved.ValueUsd.Mean)
			}
		}
	}

	// Build ranked list, including all communities (zero-value ones too).
	type entry struct {
		id    string
		value float64
	}
	ranked := make([]entry, 0, len(communitySet))
	for id := range communitySet {
		ranked = append(ranked, entry{id: id, value: communityTotals[id]})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].value > ranked[j].value })
	if len(ranked) > 10 {
		ranked = ranked[:10]
	}

	// Batch-fetch community names.
	topIDs := make([]string, len(ranked))
	for i, r := range ranked {
		topIDs[i] = r.id
	}
	communityMap, err := store.GetByIDs(ctx, topIDs, &models.Community{})
	if err != nil {
		logger.WarnContext(ctx, "failed to batch-fetch community names for user ranking", "error", err)
	}

	result := make([]*api.UserRankedCommunity, 0, len(ranked))
	for i, r := range ranked {
		entry := &api.UserRankedCommunity{
			Rank:           int32(i + 1),
			CommunityId:    r.id,
			FormattedValue: impactlib.FormatMoney(r.value),
		}
		if m, ok := communityMap[r.id]; ok {
			entry.CommunityName = m.(*models.Community).Name
		}
		result = append(result, entry)
	}
	return result
}

// userMoneyTrend computes cumulative money savings over time for a user.
func userMoneyTrend(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID string,
	_ *logging.Logger,
) []*api.TimeSeriesPoint {
	loansRaw, err := storage.QueryByField[*models.Transfer](store, ctx, "owner_id", userID)
	if err != nil {
		return nil
	}

	monthlyMap := make(map[string]float64)
	for _, t := range loansRaw {
		if t.Deleted != nil || t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		if t.ActualReturnUnixSec == nil || t.ImpactEstimate == nil {
			continue
		}
		ts := time.Unix(*t.ActualReturnUnixSec, 0)
		key := fmt.Sprintf("%d-%02d", ts.Year(), ts.Month())
		if t.ImpactEstimate.MoneySaved != nil && t.ImpactEstimate.MoneySaved.ValueUsd != nil {
			monthlyMap[key] += float64(t.ImpactEstimate.MoneySaved.ValueUsd.Mean)
		}
	}

	if len(monthlyMap) == 0 {
		return nil
	}

	// Sort months and compute cumulative.
	var months []string
	for m := range monthlyMap {
		months = append(months, m)
	}
	sort.Strings(months)

	// Keep last 6 months.
	if len(months) > 6 {
		months = months[len(months)-6:]
	}

	var points []*api.TimeSeriesPoint
	var cumulative float64
	for _, m := range months {
		cumulative += monthlyMap[m]
		t, _ := time.Parse("2006-01", m)
		points = append(points, &api.TimeSeriesPoint{
			Value:              cumulative,
			BucketStartUnixSec: proto.Int64(t.Unix()),
		})
	}
	return points
}
