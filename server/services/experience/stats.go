package experience

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/rsvpstate"
)

// GetExperienceStats retrieves statistics for an experience (sessions, attendees, value created, upcoming RSVPs).
func (s *Service) GetExperienceStats(
	ctx context.Context,
	req *connect.Request[api.GetExperienceStatsRequest],
) (*connect.Response[api.GetExperienceStatsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"experience_id", req.Msg.ExperienceId,
	)

	logger.DebugContext(ctx, "fetching experience stats")

	// Fetch experience to verify it exists. Soft-deleted experiences are
	// reported as NotFound at WARN level — clients commonly hold stale
	// references from preserved activity-log Stories.
	experience, err := s.fetchExperienceForRead(ctx, req.Msg.ExperienceId, logger.Logger, "GetExperienceStats")
	if err != nil {
		return nil, err
	}

	// Calculate sessions held
	// A completed experience counts as one session; multiple sessions per experience
	// may be tracked in the future.
	var sessionsHeld int32
	if experience.State == models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
		sessionsHeld = 1
	}

	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}

	// Reject if community is missing, soft-deleted, or caller is not a member.
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Build query for RSVPs scoped to the specific community.
	queryFields := map[string]any{
		"experience_id": req.Msg.ExperienceId,
		"community_id":  req.Msg.CommunityId,
	}

	// Fetch RSVPs for this experience in this community.
	rsvpsProto, err := s.storage.QueryByFields(ctx, queryFields, &models.ExperienceRSVP{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query RSVPs",
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "GetExperienceStats", fmt.Errorf("failed to query RSVPs"))
	}

	// Calculate statistics from RSVPs
	var totalAttendees int32
	var upcomingRSVPs int32

	// Track unique users to avoid double-counting
	uniqueAttendees := make(map[string]struct{})
	uniqueUpcomingRSVPs := make(map[string]struct{})
	uniqueYesRSVPs := make(map[string]struct{})

	for _, msg := range rsvpsProto {
		rsvp := msg.(*models.ExperienceRSVP)

		// Count attendees (users who actually attended)
		if rsvp.GetAttended() == models.AttendedStatus_ATTENDED_STATUS_YES {
			uniqueAttendees[rsvp.UserId] = struct{}{}
		}

		// Count upcoming RSVPs (users who RSVP'd but haven't attended yet)
		if rsvp.GetAttended() == models.AttendedStatus_ATTENDED_STATUS_UNKNOWN && rsvpstate.IsGoing(rsvp.GetIntention()) {
			uniqueUpcomingRSVPs[rsvp.UserId] = struct{}{}
		}

		// Count all current YES RSVPs for group size estimation.
		if rsvp.GetIntention() == models.RSVPIntention_RSVP_INTENTION_YES {
			uniqueYesRSVPs[rsvp.UserId] = struct{}{}
		}
	}

	totalAttendees = int32(len(uniqueAttendees))
	upcomingRSVPs = int32(len(uniqueUpcomingRSVPs))

	// Extract value from experience if available
	var valueUSD float32
	if experience.ValueEstimate != nil {
		valueUSD = experience.ValueEstimate.EstimatedValueUsd
	}

	// Build a QualityTimeHint from user-set duration and any stored social context override.
	hint := buildHintForSave(experience, convertModelsSocialContextToAPI(experience.SocialContext))

	// Impact for completed experiences must come from the value persisted at
	// completion — that is the only place user overrides and per-input
	// provenance live. Recomputing here would silently drop overrides.
	// Fall back to a fresh build only if no impact was persisted (older
	// records from before completion-time impact existed).
	var valueCreatedUSD float32
	var ie *api.ImpactEstimate
	if sessionsHeld > 0 {
		if experience.ImpactEstimate != nil {
			ie = impact_metrics.ModelsImpactToAPI(experience.ImpactEstimate)
		} else if s.estimatorCfg != nil {
			ie = impact_metrics.BuildExperienceImpactMetrics(valueUSD, totalAttendees, s.estimatorCfg, nil, hint)
		}
	}

	// Calculate per-session impact (always populated). Used by SharingImpactCard to show
	// what each individual session saves. Group size = host (1) + YES RSVP count so the
	// social context card reflects the actual group the host is planning for.
	var potentialImpact *api.ImpactEstimate
	if s.estimatorCfg != nil {
		// Group size = number of YES RSVPs; the host is already counted among them
		// or is not added separately to avoid double-counting.
		groupSize := int32(len(uniqueYesRSVPs))
		potentialImpact = impact_metrics.BuildExperienceImpactMetrics(valueUSD, groupSize, s.estimatorCfg, nil, hint)
	}

	logger.DebugContext(ctx, "calculated experience stats",
		"sessions_held", sessionsHeld,
		"total_attendees", totalAttendees,
		"value_created_usd", valueCreatedUSD,
		"upcoming_rsvps", upcomingRSVPs,
	)

	return connect.NewResponse(&api.GetExperienceStatsResponse{
		SessionsHeld:    sessionsHeld,
		TotalAttendees:  totalAttendees,
		ValueCreatedUsd: valueCreatedUSD,
		UpcomingRsvps:   upcomingRSVPs,
		Impact:          ie,
		PotentialImpact: potentialImpact,
	}), nil
}
