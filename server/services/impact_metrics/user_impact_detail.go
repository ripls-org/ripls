package impact_metrics

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	impactlib "go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// getUserCommunityImpactDetail returns a user's impact breakdown within a
// specific community: formatted total + transaction list.
func getUserCommunityImpactDetail(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	cfg *estimator.Config,
	req *connect.Request[api.GetUserCommunityImpactDetailRequest],
) (*connect.Response[api.GetUserCommunityImpactDetailResponse], error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetUserCommunityImpactDetail",
		"community_id", req.Msg.CommunityId,
		"user_id", req.Msg.UserId,
		"dimension", req.Msg.Dimension.String(),
	)

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.CommunityId == "" || req.Msg.UserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id and user_id are required"))
	}

	// Verify the community is active and the caller is an active member.
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, store, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	logger.DebugContext(ctx, "computing user community impact detail")

	// Fetch the user profile.
	var user models.User
	if err := store.GetByID(ctx, req.Msg.UserId, &user); err != nil {
		logger.ErrorContext(ctx, "failed to fetch user", "error", err)
		return nil, connecterr.Internal(ctx, "getUserCommunityImpactDetail", err, "detail",

			// Calculate total.
			"failed to fetch user")
	}

	calc := impactlib.NewCalculator(store, cfg)
	totalValue, err := calc.CalculateUserImpactInCommunity(
		ctx, req.Msg.UserId, req.Msg.CommunityId, req.Msg.Dimension,
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to calculate user impact", "error", err)
		return nil, connecterr.Internal(ctx, "getUserCommunityImpactDetail", err, "detail",

			// Build transaction list from transfers and requests.
			"failed to calculate user impact")
	}

	var transactions []*api.UserImpactTransaction

	// Transfers.
	transfers, err := storage.QueryByField[*models.Transfer](store, ctx, "community_id", req.Msg.CommunityId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "getUserCommunityImpactDetail", err, "detail",

			// Batch fetch gear names.
			"failed to query transfers")
	}

	gearIDs := storage.CollectField(transfers, func(t *models.Transfer) string { return t.GearId })
	gearMap, _ := storage.GetByIDs[*models.Gear](store, ctx, gearIDs, storage.QueryOptions{IncludeDeleted: true})

	for _, t := range transfers {
		if t.Deleted != nil || t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		if t.OwnerId != req.Msg.UserId && t.RecipientId != req.Msg.UserId {
			continue
		}
		val := impactlib.ExtractDimensionValue(t.ImpactEstimate, req.Msg.Dimension)
		if val <= 0 {
			continue
		}
		itemName := ""
		if g, ok := gearMap[t.GearId]; ok {
			itemName = g.Name
		}
		role := "borrower"
		if t.OwnerId == req.Msg.UserId {
			role = "owner"
		}
		txnType := "loan"
		if t.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
			txnType = "giveaway"
		}
		completedAt := int64(0)
		if t.ActualReturnUnixSec != nil {
			completedAt = *t.ActualReturnUnixSec
		} else if t.ActualPickupUnixSec != nil {
			completedAt = *t.ActualPickupUnixSec
		}
		transactions = append(transactions, &api.UserImpactTransaction{
			TransactionId:      t.Id,
			ItemName:           itemName,
			TransactionType:    txnType,
			Role:               role,
			CompletedAtUnixSec: completedAt,
			FormattedValue:     formatMetricValue(val, req.Msg.Dimension),
		})
	}

	// Requests.
	communityRequests, err := storage.QueryByField[*models.CommunityRequest](store, ctx, "community_id", req.Msg.CommunityId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "getUserCommunityImpactDetail", err, "detail", "failed to query community requests")
	}
	requestIDs := storage.CollectField(communityRequests, func(cr *models.CommunityRequest) string { return cr.RequestId })
	if len(requestIDs) > 0 {
		requestMap, err := storage.GetByIDs[*models.Request](store, ctx, requestIDs, storage.QueryOptions{})
		if err != nil {
			return nil, connecterr.Internal(ctx, "getUserCommunityImpactDetail", err, "detail", "failed to batch fetch requests")
		}
		for _, r := range requestMap {
			if r.State != models.RequestState_REQUEST_STATE_FULFILLED {
				continue
			}
			if r.RequesterId != req.Msg.UserId && !impactlib.ContainsString(r.ConfirmedHelperIds, req.Msg.UserId) {
				continue
			}
			val := impactlib.RequestDimensionValue(r, req.Msg.Dimension)
			if val <= 0 {
				continue
			}
			role := "helper"
			if r.RequesterId == req.Msg.UserId {
				role = "requester"
			}
			completedAt := int64(0)
			if r.FulfilledAtUnixSec != nil {
				completedAt = *r.FulfilledAtUnixSec
			}
			transactions = append(transactions, &api.UserImpactTransaction{
				TransactionId:      r.Id,
				ItemName:           r.Title,
				TransactionType:    "request",
				Role:               role,
				CompletedAtUnixSec: completedAt,
				FormattedValue:     formatMetricValue(val, req.Msg.Dimension),
			})
		}
	}

	// Sort transactions reverse chronological.
	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].CompletedAtUnixSec > transactions[j].CompletedAtUnixSec
	})

	// Build cumulative trend from transactions sorted chronologically.
	chronological := make([]*api.UserImpactTransaction, len(transactions))
	copy(chronological, transactions)
	sort.Slice(chronological, func(i, j int) bool {
		return chronological[i].CompletedAtUnixSec < chronological[j].CompletedAtUnixSec
	})
	var trend []*api.TimeSeriesPoint
	var cumulative float64
	for _, txn := range chronological {
		if txn.CompletedAtUnixSec <= 0 {
			continue
		}
		val := parseTxnValue(txn.FormattedValue)
		cumulative += val
		trend = append(trend, &api.TimeSeriesPoint{
			Value:              cumulative,
			BucketStartUnixSec: proto.Int64(txn.CompletedAtUnixSec),
		})
	}

	resp := &api.GetUserCommunityImpactDetailResponse{
		DisplayName:     user.Name,
		FormattedTotal:  formatMetricValue(totalValue, req.Msg.Dimension),
		CumulativeTrend: trend,
		Transactions:    transactions,
	}
	if avatar := services.PrimaryAvatarMediaID(&user); avatar != "" {
		resp.MediaId = &avatar
	}

	logger.InfoContext(ctx, "computed user community impact detail",
		"total_value", totalValue,
		"transaction_count", len(transactions),
	)

	return connect.NewResponse(resp), nil
}

// parseTxnValue extracts the numeric value from a formatted metric string
// like "$123", "$1.2K", "45 kg", "1.5 hrs", "30 min", etc.
func parseTxnValue(formatted string) float64 {
	s := strings.TrimSpace(formatted)
	s = strings.TrimPrefix(s, "$")

	// Handle K suffix (e.g., "1.2K").
	if strings.HasSuffix(s, "K") {
		s = strings.TrimSuffix(s, "K")
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0
		}
		return v * 1000
	}

	// Strip unit suffixes.
	for _, suffix := range []string{" kg", " g", " hrs", " min", " t"} {
		s = strings.TrimSuffix(s, suffix)
	}

	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}
