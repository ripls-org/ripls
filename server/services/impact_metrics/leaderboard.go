package impact_metrics

import (
	"context"
	"fmt"
	"sort"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	impactlib "go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// getCommunityLeaderboard returns top N members ranked by impact for a
// specific dimension within a community.
func getCommunityLeaderboard(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	req *connect.Request[api.GetCommunityLeaderboardRequest],
) (*connect.Response[api.GetCommunityLeaderboardResponse], error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetCommunityLeaderboard",
		"community_id", req.Msg.CommunityId,
		"dimension", req.Msg.Dimension.String(),
	)

	callerInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	callerID := callerInfo.UserID

	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}

	if _, err := auth.RequireActiveCommunity(ctx, store, req.Msg.CommunityId); err != nil {
		return nil, err
	}

	limit := int(req.Msg.Limit)
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	logger.DebugContext(ctx, "computing community leaderboard")

	// Get all community members.
	memberships, err := storage.QueryByField[*models.CommunityUser](
		store, ctx, "community_id", req.Msg.CommunityId,
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community members", "error", err)
		return nil, connecterr.Internal(ctx, "getCommunityLeaderboard", err, "detail",

			// Pre-load community-wide data once to avoid N+1 queries in the member loop.
			"failed to query community members")
	}

	transfers, err := storage.QueryByField[*models.Transfer](store, ctx, "community_id", req.Msg.CommunityId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community transfers", "error", err)
		return nil, connecterr.Internal(ctx, "getCommunityLeaderboard", err, "detail", "failed to query community transfers")
	}

	communityRequests, err := storage.QueryByField[*models.CommunityRequest](store, ctx, "community_id", req.Msg.CommunityId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community requests", "error", err)
		return nil, connecterr.Internal(ctx, "getCommunityLeaderboard", err, "detail", "failed to query community requests")
	}

	requestIDs := storage.CollectField(communityRequests, func(cr *models.CommunityRequest) string { return cr.RequestId })
	var requestMap map[string]*models.Request
	if len(requestIDs) > 0 {
		requestMap, err = storage.GetByIDs[*models.Request](store, ctx, requestIDs, storage.QueryOptions{})
		if err != nil {
			logger.ErrorContext(ctx, "failed to batch fetch requests", "error", err)
			return nil, connecterr.Internal(ctx, "getCommunityLeaderboard", err, "detail", "failed to batch fetch requests")
		}
	}

	// Calculate per-user impact entirely in memory.
	type userMetric struct {
		userID string
		value  float64
	}

	var metrics []userMetric
	for _, membership := range memberships {
		savings := impactlib.CalculateUserImpactFromPreloadedData(
			membership.UserId, transfers, requestMap, req.Msg.Dimension,
		)
		if savings > 0 {
			metrics = append(metrics, userMetric{
				userID: membership.UserId,
				value:  savings,
			})
		}
	}

	// Sort descending by value.
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].value > metrics[j].value
	})

	// Batch fetch user profiles.
	userIDs := make([]string, 0, len(metrics))
	for _, m := range metrics {
		userIDs = append(userIDs, m.userID)
	}
	userMap := make(map[string]*models.User)
	if len(userIDs) > 0 {
		users, err := storage.GetByIDs[*models.User](store, ctx, userIDs, storage.QueryOptions{})
		if err == nil {
			userMap = users
		}
	}

	// Build response.
	topLimit := limit
	if len(metrics) < topLimit {
		topLimit = len(metrics)
	}

	members := make([]*api.LeaderboardMember, 0, topLimit)
	for i := range topLimit {
		m := metrics[i]
		user := userMap[m.userID]
		member := &api.LeaderboardMember{
			Rank:           int32(i + 1),
			UserId:         m.userID,
			FormattedValue: formatMetricValue(m.value, req.Msg.Dimension),
		}
		if user != nil {
			member.DisplayName = user.Name
			if avatar := services.PrimaryAvatarMediaID(user); avatar != "" {
				member.MediaId = &avatar
			}
		}
		members = append(members, member)
	}

	// Find calling user's rank if not in top N.
	resp := &api.GetCommunityLeaderboardResponse{Members: members}
	inTopN := false
	for _, m := range members {
		if m.UserId == callerID {
			inTopN = true
			break
		}
	}
	if !inTopN {
		for i, m := range metrics {
			if m.userID == callerID {
				user := userMap[callerID]
				callerMember := &api.LeaderboardMember{
					Rank:           int32(i + 1),
					UserId:         callerID,
					FormattedValue: formatMetricValue(m.value, req.Msg.Dimension),
				}
				if user != nil {
					callerMember.DisplayName = user.Name
					if avatar := services.PrimaryAvatarMediaID(user); avatar != "" {
						callerMember.MediaId = &avatar
					}
				}
				resp.CallingUser = callerMember
				break
			}
		}
	}

	logger.InfoContext(ctx, "computed community leaderboard",
		"total_members", len(metrics),
		"returned", len(members),
	)

	return connect.NewResponse(resp), nil
}
