package impact_metrics

import (
	"context"
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
)

// computeQualityTimeDetail computes Quality Time specific detail fields.
//
// Derives weekly average and sufficiency progress from the total QT and the
// selected period. Category breakdown is computed from per-transaction QT
// estimates grouped by interaction type (loans, giveaways, requests, events).
// Subsidiary metrics (Belonging Minutes, Trust Credits, Network Diversity) are
// aggregated from per-transaction QualityTimeEstimate fields. Monthly bars
// and per-capita QT are computed from the provided arguments.
func (c *MetricDetailCalculator) computeQualityTimeDetail(
	ctx context.Context,
	totalSF float32,
	period api.ImpactMetricPeriod,
	transfers []*models.Transfer,
	requests []*models.Request,
	experiences []*models.Experience,
	monthlyBars []*api.TimeSeriesPoint,
	monthlyAverage float64,
	communityID string,
) *api.QualityTimeDetail {
	logger := logging.LoggerWithContext(ctx).With("operation", "computeQualityTimeDetail")

	// Compute weekly average from total and period width.
	weeks := periodWeeks(period)
	weeklyAvgSF := float32(0)
	if weeks > 0 {
		weeklyAvgSF = totalSF / float32(weeks)
	}

	// Sufficiency progress toward the weekly 360-minute target.
	const sufficiencyTarget = 360
	sufficiencyPct := weeklyAvgSF / sufficiencyTarget
	if sufficiencyPct > 1.0 {
		sufficiencyPct = 1.0
	}

	// Equivalence: weekly minutes → hours of quality social time.
	hours := weeklyAvgSF / 60
	equivalence := fmt.Sprintf("%.1f hours of quality social time per week", hours)

	// Category breakdown: aggregate QT by interaction type.
	categories := c.computeQTCategories(ctx, logger, totalSF, transfers, requests, experiences)

	// Per-capita SF from member count (prefer the preloaded dataset
	// when one is attached to ctx).
	var perCapitaSF float64
	communityMembers, err := resolveCommunityUsers(ctx, c.storage, communityID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community members for per-capita QT", "error", err)
	}
	memberCount := len(communityMembers)
	if memberCount > 0 {
		perCapitaSF = float64(totalSF) / float64(memberCount)
	}

	// Subsidiary metrics: Belonging Minutes, Trust Credits, Network Diversity.
	subsidiaryMetrics := c.computeSubsidiaryMetrics(ctx, logger, transfers, requests, experiences, memberCount)

	// Recent activity: most recent social interactions.
	recentActivity := c.computeSocialRecentActivity(ctx, logger, transfers, requests, experiences)

	members := c.computeQTMembers(ctx, logger, communityMembers)

	return &api.QualityTimeDetail{
		WeeklyAverageMinutes: &api.Estimate{Mean: weeklyAvgSF},
		SufficiencyPct:       sufficiencyPct,
		SufficiencyTarget:    sufficiencyTarget,
		Equivalence:          equivalence,
		Categories:           categories,
		SubsidiaryMetrics:    subsidiaryMetrics,
		MonthlyBars:          monthlyBars,
		MonthlyAverage:       monthlyAverage,
		PerCapitaMinutes:     perCapitaSF,
		RecentActivity:       recentActivity,
		Members:              members,
	}
}

// computeQTMembers returns a lightweight member summary list for the Quality
// Time social graph overlay. It batch-loads user profiles to populate display
// names and profile media IDs, and resolves inviter names for each member.
func (c *MetricDetailCalculator) computeQTMembers(
	ctx context.Context,
	logger *logging.Logger,
	communityMembers []*models.CommunityUser,
) []*api.CommunityMemberSummary {
	if len(communityMembers) == 0 {
		return nil
	}

	// Collect all user IDs (members + their inviters) for a single batch load.
	userIDs := make([]string, 0, len(communityMembers)*2)
	members := make([]*models.CommunityUser, 0, len(communityMembers))
	for _, cu := range communityMembers {
		members = append(members, cu)
		userIDs = append(userIDs, cu.UserId)
		if cu.InviterId != "" {
			userIDs = append(userIDs, cu.InviterId)
		}
	}

	// Deduplicate user IDs.
	seen := make(map[string]struct{}, len(userIDs))
	deduped := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			deduped = append(deduped, id)
		}
	}

	// Batch load all user profiles.
	userMap, err := c.storage.GetByIDs(ctx, deduped, &models.User{})
	if err != nil {
		logger.WarnContext(ctx, "failed to batch-load users for QT members", "error", err)
		userMap = map[string]proto.Message{}
	}

	// Build member summaries sorted by join date (oldest first).
	sort.Slice(members, func(i, j int) bool {
		return members[i].CreatedAtUnixSec < members[j].CreatedAtUnixSec
	})

	summaries := make([]*api.CommunityMemberSummary, 0, len(members))
	for i, cu := range members {
		summary := &api.CommunityMemberSummary{
			UserId:          cu.UserId,
			JoinedAtUnixSec: cu.CreatedAtUnixSec,
		}

		if raw, ok := userMap[cu.UserId]; ok {
			if u, ok := raw.(*models.User); ok {
				summary.DisplayName = u.Name
				summary.ProfileMediaId = services.PrimaryAvatarMediaID(u)
			}
		}

		// First member is the founder; subsequent members show inviter name.
		if i > 0 && cu.InviterId != "" {
			if raw, ok := userMap[cu.InviterId]; ok {
				if u, ok := raw.(*models.User); ok {
					summary.InvitedByDisplayName = u.Name
				}
			}
		}

		summaries = append(summaries, summary)
	}

	return summaries
}

// periodWeeks returns the number of weeks in a metric period.
func periodWeeks(period api.ImpactMetricPeriod) int {
	switch period {
	case api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_FOUR_WEEKS:
		return 4
	case api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_THREE_MONTHS:
		return 13
	case api.ImpactMetricPeriod_IMPACT_METRIC_PERIOD_ONE_YEAR:
		return 52
	default: // ALL — use one year as a conservative denominator
		return 52
	}
}

// computeQTCategories builds the QT category breakdown from per-transaction estimates.
func (c *MetricDetailCalculator) computeQTCategories(
	ctx context.Context,
	logger *logging.Logger,
	totalSF float32,
	transfers []*models.Transfer,
	requests []*models.Request,
	experiences []*models.Experience,
) []*api.SocialCategory {
	if totalSF <= 0 {
		return nil
	}

	var loansSF, giveawaysSF, requestsSF, experiencesSF float32

	for _, t := range transfers {
		ie := c.transferImpact(ctx, t, logger)
		est := c.extractEstimate(ie, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME)
		if est == nil {
			continue
		}
		if t.TransferType == models.TransferType_TRANSFER_TYPE_LOAN {
			loansSF += est.Mean
		} else {
			giveawaysSF += est.Mean
		}
	}

	for _, r := range requests {
		ie := c.requestImpact(r)
		est := c.extractEstimate(ie, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME)
		if est != nil {
			requestsSF += est.Mean
		}
	}

	for _, e := range experiences {
		ie := c.experienceImpact(e)
		est := c.extractEstimate(ie, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME)
		if est != nil {
			experiencesSF += est.Mean
		}
	}

	categories := make([]*api.SocialCategory, 0, 4)
	addCategory := func(label, icon string, value float32) {
		if value <= 0 {
			return
		}
		categories = append(categories, &api.SocialCategory{
			Label:      label,
			Value:      float64(value),
			Percentage: float64(value / totalSF * 100),
			Icon:       icon,
		})
	}

	addCategory("Events", "🎉", experiencesSF)
	addCategory("Requests", "🤝", requestsSF)
	addCategory("Loans", "📦", loansSF)
	addCategory("Giveaways", "🎁", giveawaysSF)

	return categories
}
