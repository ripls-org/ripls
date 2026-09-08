package experience

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications/notification_content"
	"go.ripls.org/ripls/server/rsvpstate"
)

// NudgeUncoveredNeedClaimers dispatches a reminder push to each Yes/Maybe
// RSVP who has not yet created any PlanningContribution on the
// experience. Owner only. Returns the count of users notified.
//
// Mirrors NudgeTimePollVoters' shape: same active-community gate, same
// notification payload pattern, same per-user fan-out via the
// notification service.
func (s *Service) NudgeUncoveredNeedClaimers(
	ctx context.Context,
	req *connect.Request[api.NudgeUncoveredNeedClaimersRequest],
) (*connect.Response[api.NudgeUncoveredNeedClaimersResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "NudgeUncoveredNeedClaimers",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	if req.Msg.ExperienceId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("experience_id is required"))
	}

	expStored := &models.Experience{}
	if err := s.storage.GetByID(ctx, req.Msg.ExperienceId, expStored); err != nil {
		logger.ErrorContext(ctx, "failed to get experience", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if expStored.OwnerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the experience owner can nudge unclaimed contributors"))
	}

	rsvpMessages, err := s.storage.QueryByField(ctx, "experience_id", req.Msg.ExperienceId, &models.ExperienceRSVP{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query rsvps for needs nudge", "error", err)
		return nil, connecterr.Internal(ctx, "NudgeUncoveredNeedClaimers", err)
	}

	candidates := make(map[string]struct{}, len(rsvpMessages))
	for _, msg := range rsvpMessages {
		rsvp := msg.(*models.ExperienceRSVP)
		if rsvp.Deleted != nil {
			continue
		}
		if rsvp.UserId == "" || rsvp.UserId == expStored.OwnerId {
			continue
		}
		if rsvpstate.IsGoing(rsvp.GetIntention()) {
			candidates[rsvp.UserId] = struct{}{}
		}
	}

	// Subtract anyone who has already created a contribution.
	contribMessages, err := s.storage.QueryByField(ctx, "experience_id", req.Msg.ExperienceId, &models.PlanningContribution{})
	if err != nil {
		logger.WarnContext(ctx, "failed to load contributions for needs nudge; nudging full RSVP set",
			"error", err)
	} else {
		for _, msg := range contribMessages {
			contrib := msg.(*models.PlanningContribution)
			if contrib.Deleted != nil {
				continue
			}
			delete(candidates, contrib.ContributorId)
		}
	}

	if len(candidates) == 0 {
		logger.InfoContext(ctx, "no unclaimed contributors to nudge")
		return connect.NewResponse(&api.NudgeUncoveredNeedClaimersResponse{NudgedCount: 0}), nil
	}

	// Drop the nudge entirely if every community the experience is shared
	// with has been soft-deleted. Mirrors the active-community gate
	// applied to bus-driven pushes by the community_subscriber.
	links, err := s.storage.QueryByField(ctx, "experience_id", req.Msg.ExperienceId, &models.CommunityExperience{})
	if err != nil {
		logger.WarnContext(ctx, "failed to load community-experience links for needs nudge",
			"error", err)
	} else {
		anyActive := false
		for _, msg := range links {
			ce := msg.(*models.CommunityExperience)
			if ce.Archived || ce.Deleted != nil {
				continue
			}
			if community.IsActive(ctx, s.storage, ce.CommunityId) {
				anyActive = true
				break
			}
		}
		if !anyActive {
			logger.InfoContext(ctx, "no active community for needs nudge; suppressing pushes")
			return connect.NewResponse(&api.NudgeUncoveredNeedClaimersResponse{NudgedCount: 0}), nil
		}
	}

	// The experience name rides the payload, not just the rendered body: the
	// off-app senders ignore Title/Body and re-render the sentence from these
	// fields, so a name left off here is a blank slot in someone's text (#2896).
	notification := notification_content.SystemNotification(ctx, &models.CommunityEventPayload{
		EventType:      "EXPERIENCE_NEEDS_NUDGE",
		ExperienceName: expStored.Name,
		ExperienceId:   proto.String(req.Msg.ExperienceId),
	})

	nudged := 0
	if s.notificationService != nil {
		for userID := range candidates {
			if err := s.notificationService.NotifyUser(ctx, userID, notification); err != nil {
				logger.WarnContext(ctx, "failed to nudge unclaimed contributor",
					"recipient_user_id", userID, "error", err)
				continue
			}
			nudged++
		}
	}

	logger.InfoContext(ctx, "needs nudge dispatched",
		"nudged_count", nudged, "candidate_count", len(candidates))

	return connect.NewResponse(&api.NudgeUncoveredNeedClaimersResponse{
		NudgedCount: int32(nudged),
	}), nil
}
