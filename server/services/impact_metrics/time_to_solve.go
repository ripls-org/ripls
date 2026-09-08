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
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// getTimeToSolveDetail returns the median time-to-solve and recent fulfilled
// requests for a community.
func getTimeToSolveDetail(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	req *connect.Request[api.GetTimeToSolveDetailRequest],
) (*connect.Response[api.GetTimeToSolveDetailResponse], error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetTimeToSolveDetail",
		"community_id", req.Msg.CommunityId,
	)

	if _, err := auth.RequireAuth(ctx); err != nil {
		return nil, err
	}

	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}

	if _, err := auth.RequireActiveCommunity(ctx, store, req.Msg.CommunityId); err != nil {
		return nil, err
	}

	logger.DebugContext(ctx, "computing time-to-solve detail")

	// Query fulfilled requests in this community.
	communityRequests, err := storage.QueryByField[*models.CommunityRequest](
		store, ctx, "community_id", req.Msg.CommunityId,
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community requests", "error", err)
		return nil, connecterr.Internal(ctx, "getTimeToSolveDetail", err, "detail", "failed to query community requests")
	}

	requestIDs := storage.CollectField(communityRequests, func(cr *models.CommunityRequest) string { return cr.RequestId })
	if len(requestIDs) == 0 {
		return connect.NewResponse(&api.GetTimeToSolveDetailResponse{}), nil
	}

	requestMap, err := storage.GetByIDs[*models.Request](store, ctx, requestIDs, storage.QueryOptions{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch fetch requests", "error", err)
		return nil, connecterr.Internal(ctx, "getTimeToSolveDetail", err, "detail",

			// Build a lookup from request ID to shared_at for solve time computation.
			"failed to batch fetch requests")
	}

	sharedAt := make(map[string]int64, len(communityRequests))
	for _, cr := range communityRequests {
		sharedAt[cr.RequestId] = cr.SharedAtUnixSec
	}

	// Collect solve times for fulfilled requests.
	type solveEntry struct {
		request      *models.Request
		solveMinutes float64
		fulfilledAt  int64
	}
	var entries []solveEntry

	for _, r := range requestMap {
		if r.State != models.RequestState_REQUEST_STATE_FULFILLED {
			continue
		}
		if r.FulfilledAtUnixSec == nil || *r.FulfilledAtUnixSec <= 0 {
			continue
		}
		shared, ok := sharedAt[r.Id]
		if !ok || shared <= 0 {
			continue
		}
		diffSec := *r.FulfilledAtUnixSec - shared
		if diffSec <= 0 {
			continue
		}
		entries = append(entries, solveEntry{
			request:      r,
			solveMinutes: float64(diffSec) / 60,
			fulfilledAt:  *r.FulfilledAtUnixSec,
		})
	}

	if len(entries) == 0 {
		return connect.NewResponse(&api.GetTimeToSolveDetailResponse{}), nil
	}

	// Sort by fulfilled_at descending for recent list.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].fulfilledAt > entries[j].fulfilledAt
	})

	// Compute median.
	solveTimes := make([]float64, len(entries))
	for i, e := range entries {
		solveTimes[i] = e.solveMinutes
	}
	sort.Float64s(solveTimes)
	medianMinutes := solveTimes[len(solveTimes)/2]
	if len(solveTimes)%2 == 0 {
		medianMinutes = (solveTimes[len(solveTimes)/2-1] + solveTimes[len(solveTimes)/2]) / 2
	}

	// Fetch helper names in batch.
	helperIDs := make([]string, 0, len(entries))
	for _, e := range entries {
		if len(e.request.ConfirmedHelperIds) > 0 {
			helperIDs = append(helperIDs, e.request.ConfirmedHelperIds[0])
		}
	}
	helperMap := make(map[string]string)
	if len(helperIDs) > 0 {
		users, err := storage.GetByIDs[*models.User](store, ctx, helperIDs, storage.QueryOptions{})
		if err == nil {
			for _, u := range users {
				helperMap[u.Id] = u.Name
			}
		}
	}

	// Build recent requests list (limit 20).
	limit := 20
	if len(entries) < limit {
		limit = len(entries)
	}
	recent := make([]*api.SolvedRequest, 0, limit)
	for _, e := range entries[:limit] {
		helperName := ""
		if len(e.request.ConfirmedHelperIds) > 0 {
			helperName = helperMap[e.request.ConfirmedHelperIds[0]]
		}
		recent = append(recent, &api.SolvedRequest{
			RequestId:          e.request.Id,
			Title:              e.request.Title,
			HelperName:         helperName,
			SolveMinutes:       float32(e.solveMinutes),
			FormattedSolveTime: formatDuration(e.solveMinutes),
			FulfilledAtUnixSec: e.fulfilledAt,
		})
	}

	logger.InfoContext(ctx, "computed time-to-solve detail",
		"median_minutes", medianMinutes,
		"fulfilled_count", len(entries),
	)

	return connect.NewResponse(&api.GetTimeToSolveDetailResponse{
		MedianMinutes:   float32(medianMinutes),
		FormattedMedian: formatDuration(medianMinutes),
		RecentRequests:  recent,
	}), nil
}

// formatDuration formats minutes into a human-readable string.
func formatDuration(minutes float64) string {
	if minutes < 60 {
		return fmt.Sprintf("%.0f min", minutes)
	}
	hours := minutes / 60
	if hours < 24 {
		return fmt.Sprintf("%.1f hrs", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%.1f days", days)
}
