package portfolio

import (
	"context"
	"fmt"
	"math"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
)

// GetPortfolioMetrics returns three-perspective impact metrics for the authenticated user:
// what they saved themselves, what they gave to others, and the combined total across all
// their communities.
func (s *Service) GetPortfolioMetrics(
	ctx context.Context,
	req *connect.Request[api.GetPortfolioMetricsRequest],
) (*connect.Response[api.GetPortfolioMetricsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	userID := authInfo.UserID
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetPortfolioMetrics",
		"user_id", userID,
	)

	// Step 1: Get all communities the user belongs to, then optionally narrow to
	// the client-requested subset when community_ids is provided.
	membershipRaw, err := s.storage.QueryByField(ctx, "user_id", userID, &models.CommunityUser{})
	if err != nil {
		return nil, fmt.Errorf("failed to query community memberships: %w", err)
	}
	memberSet := make(map[string]bool, len(membershipRaw))
	for _, m := range membershipRaw {
		memberSet[m.(*models.CommunityUser).CommunityId] = true
	}

	// When the client specifies community_ids, restrict to the intersection of
	// the user's memberships and the requested IDs.
	requestedIDs := req.Msg.CommunityIds
	communitySet := make(map[string]bool, len(memberSet))
	if len(requestedIDs) > 0 {
		for _, id := range requestedIDs {
			if memberSet[id] {
				communitySet[id] = true
			}
		}
	} else {
		communitySet = memberSet
	}
	communityIDs := make([]string, 0, len(communitySet))
	for id := range communitySet {
		communityIDs = append(communityIDs, id)
	}

	// Drop soft-deleted communities. Per the multi-community pattern in
	// auth.FilterActiveMemberCommunities, deleted communities are silently
	// skipped — this is a portfolio rollup, so we just exclude them.
	activeCommunityIDs, _, err := auth.FilterActiveMemberCommunities(ctx, s.storage, communityIDs, userID)
	if err != nil {
		return nil, err
	}
	communityIDs = activeCommunityIDs

	// Step 2: Batch-fetch transfers as owner and as recipient.
	transfersAsOwnerRaw, err := s.storage.QueryByField(ctx, "owner_id", userID, &models.Transfer{})
	if err != nil {
		return nil, fmt.Errorf("failed to query transfers as owner: %w", err)
	}
	transfersAsRecipientRaw, err := s.storage.QueryByField(ctx, "recipient_id", userID, &models.Transfer{})
	if err != nil {
		return nil, fmt.Errorf("failed to query transfers as recipient: %w", err)
	}

	savedSelf := computeReceivedSection(transfersAsRecipientRaw, communitySet)
	savedOthers := computeGivenSection(transfersAsOwnerRaw, communitySet)
	commTotal := computePersonalTotalSection(transfersAsOwnerRaw, transfersAsRecipientRaw, communitySet)

	// Compute supplementary aggregate fields from portfolio data.
	// These fields are available after `npm run generate` regenerates the Go types
	// from the updated proto/ripls/api/portfolio.proto. The computePortfolioAggregates
	// helper in aggregates.go computes all four values.
	co2PotentialGrams, qtPerWeekMinutes, activeWeeks, timeToSolveMinutes := s.computePortfolioAggregates(ctx, userID, communityIDs)
	_, _, _, _ = co2PotentialGrams, qtPerWeekMinutes, activeWeeks, timeToSolveMinutes

	logger.InfoContext(ctx, "portfolio metrics computed",
		"community_count", len(communityIDs),
	)

	return connect.NewResponse(&api.GetPortfolioMetricsResponse{
		SavedSelf:        savedSelf,
		SavedOthers:      savedOthers,
		CommunitiesTotal: commTotal,
		// New fields (co2_potential_grams, quality_time_per_week_minutes,
		// active_weeks, time_to_solve_minutes, percentile_*) are populated
		// after `npm run generate` adds them to the generated struct.
	}), nil
}

// computeReceivedSection builds the "What I've saved" section from transfers where the
// user is the recipient (borrowing, receiving giveaways) within their communities.
func computeReceivedSection(
	transfersRaw []proto.Message,
	communitySet map[string]bool,
) *api.PortfolioMetricsSection {
	var totalCostUSD, totalMinutes, totalCO2Grams, qualityTimeMinutes float64
	var itemsBorrowed, itemsReceived int

	for _, m := range transfersRaw {
		t := m.(*models.Transfer)
		if t.Deleted != nil || t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		if !communitySet[t.CommunityId] {
			continue
		}
		ie := t.ImpactEstimate
		if ie == nil {
			continue
		}

		switch t.TransferType {
		case models.TransferType_TRANSFER_TYPE_LOAN:
			itemsBorrowed++
		case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
			itemsReceived++
		}

		cost, co2, mins, qt := extractImpactValues(ie)
		totalCostUSD += cost
		totalCO2Grams += co2
		totalMinutes += mins
		qualityTimeMinutes += qt
	}

	var activityParts []string
	if itemsBorrowed > 0 {
		activityParts = append(activityParts, fmt.Sprintf("%d item%s borrowed", itemsBorrowed, plural(itemsBorrowed)))
	}
	if itemsReceived > 0 {
		activityParts = append(activityParts, fmt.Sprintf("%d item%s received", itemsReceived, plural(itemsReceived)))
	}

	return &api.PortfolioMetricsSection{
		Title:              "What I've saved",
		Subtitle:           "Impact of borrowing and receiving",
		CostSaved:          impact_metrics.FormatMoney(totalCostUSD),
		TimeRecovered:      impact_metrics.FormatTime(totalMinutes),
		Co2Avoided:         impact_metrics.FormatCO2(totalCO2Grams),
		QualityTimeMinutes: int32(math.Round(qualityTimeMinutes)),
		ActivitySummary:    joinParts(activityParts),
	}
}

// computeGivenSection builds the "What I've given" section from transfers where the
// user is the owner (lending, giving away items) within their communities.
func computeGivenSection(
	transfersRaw []proto.Message,
	communitySet map[string]bool,
) *api.PortfolioMetricsSection {
	var totalCostUSD, totalMinutes, totalCO2Grams, qualityTimeMinutes float64
	var itemsLent, itemsGiven int

	for _, m := range transfersRaw {
		t := m.(*models.Transfer)
		if t.Deleted != nil || t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		if !communitySet[t.CommunityId] {
			continue
		}
		ie := t.ImpactEstimate
		if ie == nil {
			continue
		}

		switch t.TransferType {
		case models.TransferType_TRANSFER_TYPE_LOAN:
			itemsLent++
		case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
			itemsGiven++
		}

		cost, co2, mins, qt := extractImpactValues(ie)
		totalCostUSD += cost
		totalCO2Grams += co2
		totalMinutes += mins
		qualityTimeMinutes += qt
	}

	var activityParts []string
	if itemsLent > 0 {
		activityParts = append(activityParts, fmt.Sprintf("%d item%s lent", itemsLent, plural(itemsLent)))
	}
	if itemsGiven > 0 {
		activityParts = append(activityParts, fmt.Sprintf("%d item%s given away", itemsGiven, plural(itemsGiven)))
	}

	return &api.PortfolioMetricsSection{
		Title:              "What I've given",
		Subtitle:           "Impact of my sharing on others",
		CostSaved:          impact_metrics.FormatMoney(totalCostUSD),
		TimeRecovered:      impact_metrics.FormatTime(totalMinutes),
		Co2Avoided:         impact_metrics.FormatCO2(totalCO2Grams),
		QualityTimeMinutes: int32(math.Round(qualityTimeMinutes)),
		ActivitySummary:    joinParts(activityParts),
	}
}

// computePersonalTotalSection builds the combined personal impact section by summing
// the user's given (owner) and received (recipient) transfers, deduplicating by transfer ID.
func computePersonalTotalSection(
	ownerRaw []proto.Message,
	recipientRaw []proto.Message,
	communitySet map[string]bool,
) *api.PortfolioMetricsSection {
	var totalCostUSD, totalMinutes, totalCO2Grams, qualityTimeMinutes float64
	var totalTransfers int
	seen := make(map[string]bool)

	for _, batches := range [][]proto.Message{ownerRaw, recipientRaw} {
		for _, m := range batches {
			t := m.(*models.Transfer)
			if seen[t.Id] {
				continue
			}
			seen[t.Id] = true
			if t.Deleted != nil || t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
				continue
			}
			if !communitySet[t.CommunityId] {
				continue
			}
			ie := t.ImpactEstimate
			if ie == nil {
				continue
			}
			cost, co2, mins, qt := extractImpactValues(ie)
			totalCostUSD += cost
			totalCO2Grams += co2
			totalMinutes += mins
			qualityTimeMinutes += qt
			totalTransfers++
		}
	}

	activitySummary := ""
	if totalTransfers > 0 {
		activitySummary = fmt.Sprintf("%d total exchange%s", totalTransfers, plural(totalTransfers))
	}

	return &api.PortfolioMetricsSection{
		Title:              "My total impact",
		Subtitle:           "Combined given and received across all communities",
		CostSaved:          impact_metrics.FormatMoney(totalCostUSD),
		TimeRecovered:      impact_metrics.FormatTime(totalMinutes),
		Co2Avoided:         impact_metrics.FormatCO2(totalCO2Grams),
		QualityTimeMinutes: int32(math.Round(qualityTimeMinutes)),
		ActivitySummary:    activitySummary,
	}
}

// extractImpactValues extracts cost (USD), CO2 (grams), time (minutes), and quality time
// (minutes) from a models.ImpactEstimate. Returns zeros for absent fields.
func extractImpactValues(ie *models.ImpactEstimate) (costUSD, co2Grams, minutes, qualityTimeMinutes float64) {
	if ie == nil {
		return costUSD, co2Grams, minutes, qualityTimeMinutes
	}
	if ie.MoneySaved != nil && ie.MoneySaved.ValueUsd != nil {
		costUSD = float64(ie.MoneySaved.ValueUsd.Mean)
	}
	if ie.EmissionsPrevented != nil {
		if ie.EmissionsPrevented.ManufactureAvoidedCarbon != nil &&
			ie.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams != nil {
			co2Grams += float64(ie.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams.Mean)
		}
		if ie.EmissionsPrevented.WasteReducedCarbon != nil &&
			ie.EmissionsPrevented.WasteReducedCarbon.Co2EGrams != nil {
			co2Grams += float64(ie.EmissionsPrevented.WasteReducedCarbon.Co2EGrams.Mean)
		}
	}
	if ie.TimeSaved != nil && ie.TimeSaved.Minutes != nil {
		minutes = float64(ie.TimeSaved.Minutes.Mean)
	}
	if ie.QualityTime != nil && ie.QualityTime.QualityTimeMinutes != nil {
		qualityTimeMinutes = float64(ie.QualityTime.QualityTimeMinutes.Mean)
	}
	return costUSD, co2Grams, minutes, qualityTimeMinutes
}

// lookupName returns the value from m for key, or fallback if not found.
func lookupName(m map[string]string, key, fallback string) string {
	if v, ok := m[key]; ok && v != "" {
		return v
	}
	return fallback
}

// plural returns "s" when count != 1, for simple English pluralization.
func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

// joinParts joins activity summary parts with ", ".
func joinParts(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		result := parts[0]
		for _, p := range parts[1:] {
			result += ", " + p
		}
		return result
	}
}
