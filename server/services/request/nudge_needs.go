package request

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
)

// NudgeUncoveredRequestNeedClaimers dispatches a reminder push to each
// existing RequestOffer member who has not yet created any
// PlanningContribution on the request. Requester only. Returns the
// count of users notified.
//
// Mirrors the experience-scope NudgeUncoveredNeedClaimers' shape. The
// candidate set is intentionally narrower than the broadcast-community
// membership: only members who already offered to help are pinged, to
// avoid spamming the wider community.
func (s *Service) NudgeUncoveredRequestNeedClaimers(
	ctx context.Context,
	req *connect.Request[api.NudgeUncoveredRequestNeedClaimersRequest],
) (*connect.Response[api.NudgeUncoveredRequestNeedClaimersResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "NudgeUncoveredRequestNeedClaimers",
		"user_id", authInfo.UserID,
		"target_request_id", req.Msg.RequestId,
	)

	if req.Msg.RequestId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("request_id is required"))
	}

	requestStored, err := loadActiveRequest(ctx, s.storage, req.Msg.RequestId)
	if err != nil {
		return nil, err
	}

	if requestStored.RequesterId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the requester can nudge unclaimed contributors"))
	}

	offerMessages, err := s.storage.QueryByField(ctx, "request_id", req.Msg.RequestId, &models.RequestOffer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query offers for needs nudge", "error", err)
		return nil, connecterr.Internal(ctx, "NudgeUncoveredRequestNeedClaimers", err)
	}

	candidates := make(map[string]struct{}, len(offerMessages))
	for _, msg := range offerMessages {
		offer := msg.(*models.RequestOffer)
		if offer.Withdrawn {
			continue
		}
		if offer.UserId == "" || offer.UserId == requestStored.RequesterId {
			continue
		}
		candidates[offer.UserId] = struct{}{}
	}

	contribMessages, err := s.storage.QueryByField(ctx, "request_id", req.Msg.RequestId, &models.PlanningContribution{})
	if err != nil {
		logger.WarnContext(ctx, "failed to load contributions for needs nudge; nudging full offerer set",
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
		return connect.NewResponse(&api.NudgeUncoveredRequestNeedClaimersResponse{NudgedCount: 0}), nil
	}

	// Drop the nudge entirely if every community the request is shared with
	// has been soft-deleted. Mirrors the active-community gate applied to
	// bus-driven pushes by the community_subscriber.
	links, err := s.storage.QueryByField(ctx, "request_id", req.Msg.RequestId, &models.CommunityRequest{})
	if err != nil {
		logger.WarnContext(ctx, "failed to load community-request links for needs nudge",
			"error", err)
	} else {
		anyActive := false
		for _, msg := range links {
			cr := msg.(*models.CommunityRequest)
			if cr.Deleted != nil {
				continue
			}
			if community.IsActive(ctx, s.storage, cr.CommunityId) {
				anyActive = true
				break
			}
		}
		if !anyActive {
			logger.InfoContext(ctx, "no active community for request-needs nudge; suppressing pushes")
			return connect.NewResponse(&api.NudgeUncoveredRequestNeedClaimersResponse{NudgedCount: 0}), nil
		}
	}

	// The request title rides the payload, not just the rendered body: the
	// off-app senders ignore Title/Body and re-render the sentence from these
	// fields, so a title left off here is a blank slot in someone's text (#2896).
	notification := notification_content.SystemNotification(ctx, &models.CommunityEventPayload{
		EventType:    "REQUEST_NEEDS_NUDGE",
		RequestTitle: requestStored.Title,
		RequestId:    proto.String(req.Msg.RequestId),
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

	logger.InfoContext(ctx, "request-needs nudge dispatched",
		"nudged_count", nudged, "candidate_count", len(candidates))

	return connect.NewResponse(&api.NudgeUncoveredRequestNeedClaimersResponse{
		NudgedCount: int32(nudged),
	}), nil
}
