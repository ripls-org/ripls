package experience

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
)

// PreviewExperienceImpact computes a live impact estimate for the completion modal.
// It accepts the experience ID plus the current confirmed attendee set, applies the
// real QT formula with user-set duration and connection context, and returns the
// estimate without persisting anything. Used by the client completion modal for
// live preview as attendees are toggled.
func (s *Service) PreviewExperienceImpact(
	ctx context.Context,
	req *connect.Request[api.PreviewExperienceImpactRequest],
) (*connect.Response[api.PreviewExperienceImpactResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PreviewExperienceImpact",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	if req.Msg.ExperienceId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("experience_id is required"))
	}

	// Fetch the experience to get value estimate, time, and community context.
	expStored, err := s.fetchExperienceForRead(ctx, req.Msg.ExperienceId, logger.Logger, "PreviewExperienceImpact")
	if err != nil {
		return nil, err
	}

	// Determine group size: confirmed_attendee_count takes priority over
	// len(confirmed_attendee_ids) so provisional users are included in the count.
	groupSize := int32(len(req.Msg.ConfirmedAttendeeIds))
	if req.Msg.ConfirmedAttendeeCount > 0 {
		groupSize = req.Msg.ConfirmedAttendeeCount
	}
	if groupSize < 1 {
		groupSize = 1
	}

	if s.estimatorCfg == nil {
		return connect.NewResponse(&api.PreviewExperienceImpactResponse{
			Impact: &api.ImpactEstimate{},
		}), nil
	}

	var valueUSD float32
	if expStored.ValueEstimate != nil {
		valueUSD = expStored.ValueEstimate.EstimatedValueUsd
	}

	// Resolve connection context between owner and first confirmed non-owner attendee.
	var communityID string
	communityExperiences, err := s.storage.QueryByField(ctx, "experience_id", req.Msg.ExperienceId, &models.CommunityExperience{})
	if err == nil && len(communityExperiences) > 0 {
		communityID = communityExperiences[0].(*models.CommunityExperience).CommunityId
	}

	var firstAttendeeID string
	for _, uid := range req.Msg.ConfirmedAttendeeIds {
		if uid != expStored.OwnerId {
			firstAttendeeID = uid
			break
		}
	}

	// Fetch the first attendee's full profile only when needed for connection context.
	var connCtx *api.ConnectionContext
	if firstAttendeeID != "" {
		connCtx = s.resolveConnectionContext(ctx, expStored.OwnerId, firstAttendeeID, communityID, logger)
	}

	// Build the QT hint using the stored attributes from the experience.
	// Use user-set duration and LLM-inferred vulnerability (from stored estimate).
	hint := buildHintFromStoredEstimate(expStored)

	ie := impact_metrics.BuildExperienceImpactMetrics(valueUSD, groupSize, s.estimatorCfg, connCtx, hint)

	// Roll the deliverable child transfers' item-based money/emissions/time
	// into the preview (#2724) — the same adoption CompleteExperience
	// persists, so the modal's numbers survive the commit.
	if children, childErr := s.deliverableChildTransfers(ctx, req.Msg.ExperienceId); childErr != nil {
		logger.WarnContext(ctx, "failed to query child transfers for preview; keeping value-based estimate", "error", childErr)
	} else {
		s.adoptChildTransferImpact(ctx, ie, children, connCtx, logger)
	}

	logger.DebugContext(ctx, "preview impact computed",
		"group_size", groupSize,
		"value_usd", valueUSD,
	)

	return connect.NewResponse(&api.PreviewExperienceImpactResponse{
		Impact: ie,
	}), nil
}

// buildHintFromStoredEstimate extracts a QualityTimeHint from the experience's
// stored fields. Uses the user-set duration from the time field (highest
// priority) and the vulnerability level from the stored impact estimate (if
// set by a previous LLM inference). Returns nil when no stored data is available.
func buildHintFromStoredEstimate(exp *models.Experience) *impact_metrics.QualityTimeHint {
	userDuration := extractExperienceDurationMinutes(exp)

	// Extract vulnerability level from stored impact estimate if available.
	var vulnerabilityLevel string
	if exp.ImpactEstimate != nil {
		api := impact_metrics.ModelsImpactToAPI(exp.ImpactEstimate)
		if api.QualityTime != nil && api.QualityTime.Attributes != nil {
			vulnerabilityLevel = vulnerabilityLevelToString(api.QualityTime.Attributes.Vulnerability)
		}
	}

	if userDuration == 0 && vulnerabilityLevel == "" {
		return nil
	}

	return &impact_metrics.QualityTimeHint{
		UserDurationMinutes: userDuration,
		VulnerabilityLevel:  vulnerabilityLevel,
	}
}

// vulnerabilityLevelToString converts a SocialVulnerabilityLevel enum to the
// string form expected by QualityTimeHint ("high", "medium", or "low").
func vulnerabilityLevelToString(v api.SocialVulnerabilityLevel) string {
	switch v {
	case api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_HIGH:
		return "high"
	case api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_MEDIUM:
		return "medium"
	case api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_LOW:
		return "low"
	default:
		return ""
	}
}
