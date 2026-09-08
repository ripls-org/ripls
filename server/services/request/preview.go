package request

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// PreviewRequestImpact computes a live impact estimate for the fulfillment
// modal as helpers are toggled. Read-only — does not persist. Mirrors
// PreviewExperienceImpact.
func (s *Service) PreviewRequestImpact(
	ctx context.Context,
	req *connect.Request[api.PreviewRequestImpactRequest],
) (*connect.Response[api.PreviewRequestImpactResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PreviewRequestImpact",
		"user_id", authInfo.UserID,
		"target_request_id", req.Msg.RequestId,
	)

	if req.Msg.RequestId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("request_id is required"))
	}

	stored := &models.Request{}
	if err := s.storage.GetByID(ctx, req.Msg.RequestId, stored); err != nil {
		logger.ErrorContext(ctx, "failed to get request for preview", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if s.estimatorCfg == nil {
		return connect.NewResponse(&api.PreviewRequestImpactResponse{
			Impact: &api.ImpactEstimate{},
		}), nil
	}

	var communityID string
	communityRequests, err := s.storage.QueryByField(ctx, "request_id", req.Msg.RequestId, &models.CommunityRequest{})
	if err == nil && len(communityRequests) > 0 {
		communityID = communityRequests[0].(*models.CommunityRequest).CommunityId
	}

	// The preview IS the fulfillment computation (#2724): the identical
	// deliverable gear-offer roll-up, group size, connection context, and
	// stored social hints MarkRequestFulfilled uses. No input differs, so
	// nothing can move under the requester when they commit.
	ie, _ := s.requestFulfillmentImpact(ctx, stored, req.Msg.ConfirmedHelperIds, req.Msg.ConfirmedHelperCount, communityID, nil, logger)

	logger.DebugContext(ctx, "preview impact computed",
		"confirmed_helper_count", req.Msg.ConfirmedHelperCount,
		"confirmed_helper_ids", len(req.Msg.ConfirmedHelperIds),
	)

	return connect.NewResponse(&api.PreviewRequestImpactResponse{
		Impact: ie,
	}), nil
}
