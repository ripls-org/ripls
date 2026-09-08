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
	"go.ripls.org/ripls/server/storage"
)

const (
	defaultPageSize = 25
	maxPageSize     = 100
)

// getCommunityActions returns a paginated list of individual sharing actions
// with per-transaction impact values.
func getCommunityActions(
	ctx context.Context,
	s *Service,
	req *connect.Request[api.GetCommunityActionsRequest],
) (*connect.Response[api.GetCommunityActionsResponse], error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetCommunityActions",
		"community_id", req.Msg.CommunityId,
	)

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}

	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.calculator.Storage(), req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	pageSize := int(req.Msg.PageSize)
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	logger.DebugContext(ctx, "loading community actions", "page_size", pageSize)

	// Load all completed transactions in parallel-safe batches.
	transfers, err := loadCompletedTransfers(ctx, s.calculator.Storage(), req.Msg.CommunityId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to load transfers", "error", err)
		return nil, connecterr.Internal(ctx, "getCommunityActions", err, "detail", "failed to load transfers")
	}

	requests, err := loadFulfilledRequests(ctx, s.calculator.Storage(), req.Msg.CommunityId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to load requests", "error", err)
		return nil, connecterr.Internal(ctx, "getCommunityActions", err, "detail", "failed to load requests")
	}

	experiences, err := loadCompletedExperiences(ctx, s.calculator.Storage(), req.Msg.CommunityId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to load experiences", "error", err)
		return nil, connecterr.Internal(ctx, "getCommunityActions", err, "detail",

			// Build action items from all transaction types.
			"failed to load experiences")
	}

	cache := impactlib.NewEntityCache(s.calculator.Storage())
	var items []*api.ActionItem

	for _, t := range transfers {
		item := transferToActionItem(ctx, t, cache, s.calculator.Storage(), logger)
		if item != nil {
			items = append(items, item)
		}
	}

	for _, r := range requests {
		item := requestToActionItem(ctx, r, cache, logger)
		if item != nil {
			items = append(items, item)
		}
	}

	for _, e := range experiences {
		item := experienceToActionItem(ctx, e, cache, logger)
		if item != nil {
			items = append(items, item)
		}
	}

	// Sort by completed_at descending (most recent first).
	sort.Slice(items, func(i, j int) bool {
		return items[i].CompletedAtUnixSec > items[j].CompletedAtUnixSec
	})

	// Paginate using offset-based page tokens.
	startIdx := 0
	if req.Msg.PageToken != nil && *req.Msg.PageToken != "" {
		_, _ = fmt.Sscanf(*req.Msg.PageToken, "%d", &startIdx)
	}
	if startIdx > len(items) {
		startIdx = len(items)
	}

	endIdx := startIdx + pageSize
	if endIdx > len(items) {
		endIdx = len(items)
	}

	page := items[startIdx:endIdx]

	var nextToken *string
	if endIdx < len(items) {
		token := fmt.Sprintf("%d", endIdx)
		nextToken = &token
	}

	logger.InfoContext(ctx, "returning community actions",
		"total_items", len(items),
		"page_start", startIdx,
		"page_size", len(page),
	)

	return connect.NewResponse(&api.GetCommunityActionsResponse{
		Items:         page,
		NextPageToken: nextToken,
	}), nil
}

// transferToActionItem converts a completed Transfer to an ActionItem.
func transferToActionItem(
	ctx context.Context,
	t *models.Transfer,
	cache *impactlib.EntityCache,
	_ *storage.ProtoSQLStorage,
	logger *logging.Logger,
) *api.ActionItem {
	txType := "loan"
	if t.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
		txType = "giveaway"
	}

	// Resolve item name from gear.
	itemName := "Unknown item"
	gear := cache.GetGear(ctx, t.GearId, logger)
	if gear != nil {
		itemName = gear.Name
	}

	// Resolve counterparty name (the recipient/borrower).
	counterpartyName := ""
	var counterpartyUserID *string
	if t.RecipientId != "" {
		user := cache.GetUser(ctx, t.RecipientId, logger)
		if user != nil {
			counterpartyName = user.Name
		}
		counterpartyUserID = &t.RecipientId
	}

	var completedAt int64
	if t.ActualReturnUnixSec != nil {
		completedAt = *t.ActualReturnUnixSec
	}

	var impact *api.ImpactEstimate
	if t.ImpactEstimate != nil {
		impact = impactlib.ModelsImpactToAPI(t.ImpactEstimate)
	}

	return &api.ActionItem{
		TransactionId:      t.Id,
		TransactionType:    txType,
		ItemName:           itemName,
		CounterpartyName:   counterpartyName,
		CounterpartyUserId: counterpartyUserID,
		CompletedAtUnixSec: completedAt,
		Impact:             impact,
	}
}

// requestToActionItem converts a fulfilled Request to an ActionItem.
func requestToActionItem(
	ctx context.Context,
	r *models.Request,
	cache *impactlib.EntityCache,
	logger *logging.Logger,
) *api.ActionItem {
	counterpartyName := ""
	var counterpartyUserID *string
	if len(r.ConfirmedHelperIds) > 0 {
		helperID := r.ConfirmedHelperIds[0]
		user := cache.GetUser(ctx, helperID, logger)
		if user != nil {
			counterpartyName = user.Name
		}
		counterpartyUserID = &helperID
	}

	var completedAt int64
	if r.FulfilledAtUnixSec != nil {
		completedAt = *r.FulfilledAtUnixSec
	}

	var impact *api.ImpactEstimate
	if r.ImpactEstimate != nil {
		impact = impactlib.ModelsImpactToAPI(r.ImpactEstimate)
	}

	return &api.ActionItem{
		TransactionId:      r.Id,
		TransactionType:    "request",
		ItemName:           r.Title,
		CounterpartyName:   counterpartyName,
		CounterpartyUserId: counterpartyUserID,
		CompletedAtUnixSec: completedAt,
		Impact:             impact,
	}
}

// experienceToActionItem converts a completed Experience to an ActionItem.
func experienceToActionItem(
	ctx context.Context,
	e *models.Experience,
	cache *impactlib.EntityCache,
	logger *logging.Logger,
) *api.ActionItem {
	counterpartyName := ""
	var counterpartyUserID *string
	if e.OwnerId != "" {
		user := cache.GetUser(ctx, e.OwnerId, logger)
		if user != nil {
			counterpartyName = user.Name
		}
		counterpartyUserID = &e.OwnerId
	}

	var completedAt int64
	if e.CompletedAtUnixSec != nil {
		completedAt = *e.CompletedAtUnixSec
	}

	var impact *api.ImpactEstimate
	if e.ImpactEstimate != nil {
		impact = impactlib.ModelsImpactToAPI(e.ImpactEstimate)
	}

	return &api.ActionItem{
		TransactionId:      e.Id,
		TransactionType:    "event",
		ItemName:           e.Name,
		CounterpartyName:   counterpartyName,
		CounterpartyUserId: counterpartyUserID,
		CompletedAtUnixSec: completedAt,
		Impact:             impact,
	}
}

// loadCompletedTransfers loads all completed, non-deleted transfers for a community.
func loadCompletedTransfers(ctx context.Context, store *storage.ProtoSQLStorage, communityID string) ([]*models.Transfer, error) {
	transfersRaw, err := store.QueryByField(ctx, "community_id", communityID, &models.Transfer{})
	if err != nil {
		return nil, fmt.Errorf("failed to query transfers: %w", err)
	}

	var completed []*models.Transfer
	for _, msgRaw := range transfersRaw {
		transfer := msgRaw.(*models.Transfer)
		if transfer.Deleted != nil {
			continue
		}
		if transfer.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		completed = append(completed, transfer)
	}
	return completed, nil
}

// loadFulfilledRequests loads all fulfilled requests for a community via CommunityRequest junction.
func loadFulfilledRequests(ctx context.Context, store *storage.ProtoSQLStorage, communityID string) ([]*models.Request, error) {
	communityRequestsRaw, err := store.QueryByField(ctx, "community_id", communityID, &models.CommunityRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to query community requests: %w", err)
	}

	var fulfilled []*models.Request
	for _, msgRaw := range communityRequestsRaw {
		cr := msgRaw.(*models.CommunityRequest)
		request := &models.Request{}
		if getErr := store.GetByID(ctx, cr.RequestId, request); getErr != nil {
			continue
		}
		if request.Deleted != nil || request.State != models.RequestState_REQUEST_STATE_FULFILLED {
			continue
		}
		fulfilled = append(fulfilled, request)
	}
	return fulfilled, nil
}

// loadCompletedExperiences loads all completed experiences for a community via CommunityExperience junction.
func loadCompletedExperiences(ctx context.Context, store *storage.ProtoSQLStorage, communityID string) ([]*models.Experience, error) {
	communityExperiencesRaw, err := store.QueryByField(ctx, "community_id", communityID, &models.CommunityExperience{})
	if err != nil {
		return nil, fmt.Errorf("failed to query community experiences: %w", err)
	}

	var completed []*models.Experience
	for _, msgRaw := range communityExperiencesRaw {
		ce := msgRaw.(*models.CommunityExperience)
		experience := &models.Experience{}
		if getErr := store.GetByID(ctx, ce.ExperienceId, experience); getErr != nil {
			continue
		}
		if experience.Deleted != nil || experience.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			continue
		}
		completed = append(completed, experience)
	}
	return completed, nil
}
