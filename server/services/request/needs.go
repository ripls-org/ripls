package request

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/planning"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

const (
	maxRequestNeedSlots     = 20
	defaultRequestNeedSlots = 1
)

// AddRequestNeed adds a new need to a request (any participant).
func (s *Service) AddRequestNeed(
	ctx context.Context,
	req *connect.Request[api.AddRequestNeedRequest],
) (*connect.Response[api.AddRequestNeedResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "AddRequestNeed",
		"user_id", authInfo.UserID,
		"target_request_id", req.Msg.RequestId,
	)

	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("need name is required"))
	}

	slots := req.Msg.Slots
	if slots <= 0 {
		slots = defaultRequestNeedSlots
	}
	if slots > maxRequestNeedSlots {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("slots cannot exceed %d", maxRequestNeedSlots))
	}

	requestStored, err := loadActiveRequest(ctx, s.storage, req.Msg.RequestId)
	if err != nil {
		return nil, err
	}

	need := &models.PlanningNeed{
		Scope:            &models.PlanningNeed_RequestId{RequestId: req.Msg.RequestId},
		ProposerId:       authInfo.UserID,
		Name:             req.Msg.Name,
		Note:             req.Msg.Note,
		Slots:            slots,
		SlotsRemaining:   slots,
		CreatedAtUnixSec: clock.UnixSec(ctx),
	}

	needID, err := s.storage.Insert(ctx, need)
	if err != nil {
		logger.ErrorContext(ctx, "failed to insert request need", "error", err)
		return nil, connecterr.Internal(ctx, "AddRequestNeed", err)
	}
	need.Id = needID

	logger.InfoContext(ctx, "request need added", "need_id", needID, "name", need.Name)

	displayName := s.getUserDisplayName(ctx, authInfo.UserID)
	if err := planning.PostNeedAdded(ctx, s.systemMessageWriter, requestStored.ConversationId, authInfo.UserID, displayName, need, nil); err != nil {
		return nil, connecterr.Internal(ctx, "AddRequestNeed", err)
	}

	needResp, err := buildRequestNeedResponse(ctx, s, need)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.AddRequestNeedResponse{Need: needResp}), nil
}

// RemoveRequestNeed removes a need from a request (requester/proposer only).
func (s *Service) RemoveRequestNeed(
	ctx context.Context,
	req *connect.Request[api.RemoveRequestNeedRequest],
) (*connect.Response[api.RemoveRequestNeedResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RemoveRequestNeed",
		"user_id", authInfo.UserID,
		"need_id", req.Msg.NeedId,
		"target_request_id", req.Msg.RequestId,
	)

	requestStored, err := loadActiveRequest(ctx, s.storage, req.Msg.RequestId)
	if err != nil {
		return nil, err
	}

	need := &models.PlanningNeed{}
	if err = s.storage.GetByID(ctx, req.Msg.NeedId, need); err != nil {
		logger.ErrorContext(ctx, "failed to get request need", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if need.GetRequestId() != req.Msg.RequestId {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("need not found in this request"))
	}

	if need.ProposerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the proposer can remove a need"))
	}

	now := clock.UnixSec(ctx)
	need.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: now}
	if err = s.storage.Update(ctx, need); err != nil {
		logger.ErrorContext(ctx, "failed to soft-delete request need", "error", err)
		return nil, connecterr.Internal(ctx, "RemoveRequestNeed", err)
	}

	// Cascade: every contribution that pointed at this need (claims
	// recorded via from_need_id) is voided alongside the need so the
	// breakdown disappears as a unit. Other helpers' contributions are
	// removed as a side effect of the proposer cancelling — the
	// proposer's authority to remove the need extends to its slots'
	// occupants.
	cascaded, cascadeErr := planning.SoftDeleteContributionsForNeed(ctx, s.storage, req.Msg.NeedId)
	if cascadeErr != nil {
		logger.ErrorContext(ctx, "failed to cascade-soft-delete contributions for need",
			"need_id", req.Msg.NeedId, "error", cascadeErr)
		return nil, cascadeErr
	}

	logger.InfoContext(ctx, "request need removed",
		"need_id", req.Msg.NeedId, "cascaded_contributions", cascaded)

	displayName := s.getUserDisplayName(ctx, authInfo.UserID)
	planning.PostNeedRemoved(ctx, s.systemMessageWriter, requestStored.ConversationId, authInfo.UserID, displayName, req.Msg.NeedId, need.Name)

	return connect.NewResponse(&api.RemoveRequestNeedResponse{}), nil
}

// UpdateRequestNeed edits an existing need in place (proposer only). Any
// fields set on the request replace the current values on the need;
// fields left unset are preserved. Reducing slots below the current
// claim count (slots - slots_remaining) fails with FailedPrecondition.
func (s *Service) UpdateRequestNeed(
	ctx context.Context,
	req *connect.Request[api.UpdateRequestNeedRequest],
) (*connect.Response[api.UpdateRequestNeedResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "UpdateRequestNeed",
		"user_id", authInfo.UserID,
		"need_id", req.Msg.NeedId,
		"target_request_id", req.Msg.RequestId,
	)

	requestStored, err := loadActiveRequest(ctx, s.storage, req.Msg.RequestId)
	if err != nil {
		return nil, err
	}

	need := &models.PlanningNeed{}
	if err = s.storage.GetByID(ctx, req.Msg.NeedId, need); err != nil {
		logger.ErrorContext(ctx, "failed to get request need", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if need.GetRequestId() != req.Msg.RequestId {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("need not found in this request"))
	}

	if need.ProposerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the proposer can update a need"))
	}

	changed := false
	if req.Msg.Name != nil {
		name := *req.Msg.Name
		if name == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("need name cannot be empty"))
		}
		if need.Name != name {
			need.Name = name
			changed = true
		}
	}
	if req.Msg.Note != nil {
		newNote := *req.Msg.Note
		existing := ""
		if need.Note != nil {
			existing = *need.Note
		}
		if newNote != existing {
			if newNote == "" {
				need.Note = nil
			} else {
				need.Note = proto.String(newNote)
			}
			changed = true
		}
	}
	if req.Msg.Slots != nil {
		newSlots := *req.Msg.Slots
		if newSlots <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("slots must be positive"))
		}
		if newSlots > maxRequestNeedSlots {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("slots cannot exceed %d", maxRequestNeedSlots))
		}
		claimedCount := need.Slots - need.SlotsRemaining
		if newSlots < claimedCount {
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("cannot reduce slots below current claim count (%d)", claimedCount))
		}
		if need.Slots != newSlots {
			delta := newSlots - need.Slots
			need.Slots = newSlots
			need.SlotsRemaining += delta
			changed = true
		}
	}

	if changed {
		if err = s.storage.Update(ctx, need); err != nil {
			logger.ErrorContext(ctx, "failed to update request need", "error", err)
			return nil, connecterr.Internal(ctx, "UpdateRequestNeed", err)
		}
		logger.InfoContext(ctx, "request need updated", "need_id", need.Id)

		displayName := s.getUserDisplayName(ctx, authInfo.UserID)
		planning.PostNeedUpdated(ctx, s.systemMessageWriter, requestStored.ConversationId, authInfo.UserID, displayName, need.Id, need.Name)
	}

	needResp, err := buildRequestNeedResponse(ctx, s, need)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.UpdateRequestNeedResponse{Need: needResp}), nil
}

// BatchAddRequestNeeds adds multiple needs to a request in a single call (any participant).
func (s *Service) BatchAddRequestNeeds(
	ctx context.Context,
	req *connect.Request[api.BatchAddRequestNeedsRequest],
) (*connect.Response[api.BatchAddRequestNeedsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "BatchAddRequestNeeds",
		"user_id", authInfo.UserID,
		"target_request_id", req.Msg.RequestId,
	)

	if len(req.Msg.Items) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("at least one item is required"))
	}

	requestStored, err := loadActiveRequest(ctx, s.storage, req.Msg.RequestId)
	if err != nil {
		return nil, err
	}

	now := clock.UnixSec(ctx)
	needModels := make([]*models.PlanningNeed, 0, len(req.Msg.Items))
	for i, item := range req.Msg.Items {
		if item.Name == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("item %d: need name is required", i))
		}
		slots := item.Slots
		if slots <= 0 {
			slots = defaultRequestNeedSlots
		}
		if slots > maxRequestNeedSlots {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("item %d: slots cannot exceed %d", i, maxRequestNeedSlots))
		}
		needModels = append(needModels, &models.PlanningNeed{
			Scope:            &models.PlanningNeed_RequestId{RequestId: req.Msg.RequestId},
			ProposerId:       authInfo.UserID,
			Name:             item.Name,
			Note:             item.Note,
			Slots:            slots,
			SlotsRemaining:   slots,
			CreatedAtUnixSec: now,
		})
	}

	protoMsgs := make([]proto.Message, len(needModels))
	for i, n := range needModels {
		protoMsgs[i] = n
	}

	ids, err := s.storage.InsertBatch(ctx, protoMsgs)
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch insert request needs", "error", err)
		return nil, connecterr.Internal(ctx, "BatchAddRequestNeeds", err)
	}
	for i, id := range ids {
		needModels[i].Id = id
	}

	logger.InfoContext(ctx, "request needs batch added", "count", len(needModels))

	displayName := s.getUserDisplayName(ctx, authInfo.UserID)
	// Sequential timestamps keep the lines in creation order — the whole
	// batch lands in the same clock second, and same-second ties otherwise
	// sort on a random UUID id (#2724).
	batchStartSec := clock.UnixSec(ctx)
	for i, need := range needModels {
		sentAt := batchStartSec + int64(i)
		if err := planning.PostNeedAdded(ctx, s.systemMessageWriter, requestStored.ConversationId, authInfo.UserID, displayName, need, &sentAt); err != nil {
			logger.WarnContext(ctx, "failed to post need-added message", "need_id", need.Id, "error", err)
		}
	}

	needResps := make([]*api.RequestNeedResponse, 0, len(needModels))
	for _, need := range needModels {
		needResp, err := buildRequestNeedResponse(ctx, s, need)
		if err != nil {
			return nil, err
		}
		needResps = append(needResps, needResp)
	}
	return connect.NewResponse(&api.BatchAddRequestNeedsResponse{Needs: needResps}), nil
}

// ClaimRequestNeed claims a slot on a need, creating a contribution. Auto-creates a RequestOffer if needed.
func (s *Service) ClaimRequestNeed(
	ctx context.Context,
	req *connect.Request[api.ClaimRequestNeedRequest],
) (*connect.Response[api.ClaimRequestNeedResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ClaimRequestNeed",
		"user_id", authInfo.UserID,
		"need_id", req.Msg.NeedId,
		"target_request_id", req.Msg.RequestId,
	)

	requestStored, err := loadActiveRequest(ctx, s.storage, req.Msg.RequestId)
	if err != nil {
		return nil, err
	}

	// Reject claims scoped to a soft-deleted community. Empty community_id is
	// allowed for cross-community / requester self-claims.
	if req.Msg.CommunityId != "" {
		if _, err := auth.RequireActiveCommunity(ctx, s.storage, req.Msg.CommunityId); err != nil {
			return nil, err
		}
	}

	need := &models.PlanningNeed{}
	if err = s.storage.GetByID(ctx, req.Msg.NeedId, need); err != nil {
		logger.ErrorContext(ctx, "failed to get request need", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if need.GetRequestId() != req.Msg.RequestId {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("need not found in this request"))
	}

	if need.Deleted != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("need has been removed"))
	}

	gearID := req.Msg.GetGearId()
	if err := planning.ValidateGearLink(ctx, s.storage, authInfo.UserID, gearID); err != nil {
		logger.InfoContext(ctx, "rejected gear link on claim", "gear_id", gearID, "error", err)
		return nil, err
	}

	decremented, err := planning.ClaimSlot(ctx, s.storage, need)
	if err != nil {
		logger.ErrorContext(ctx, "failed to claim need slot", "error", err)
		return nil, err
	}

	var description *string
	if req.Msg.Note != nil && *req.Msg.Note != "" {
		description = req.Msg.Note
	}

	contrib := &models.PlanningContribution{
		Scope:            &models.PlanningContribution_RequestId{RequestId: req.Msg.RequestId},
		ContributorId:    authInfo.UserID,
		Title:            need.Name,
		Description:      description,
		FromNeedId:       &need.Id,
		OriginalNeedName: &need.Name,
		OriginalNeedNote: need.Note,
		CreatedAtUnixSec: clock.UnixSec(ctx),
		GearId:           reqNilIfEmpty(gearID),
	}

	contribID, err := s.storage.Insert(ctx, contrib)
	if err != nil {
		logger.ErrorContext(ctx, "failed to insert contribution from need claim", "error", err)
		// Only restore when we actually decremented — on the over-claim
		// path ClaimSlot was a no-op, so restoring would silently bump
		// slots_remaining past the proposer's original count.
		if decremented {
			planning.RestoreSlotOnFailure(ctx, s.storage, need.Id)
		}
		return nil, connecterr.Internal(ctx, "ClaimRequestNeed", err)
	}
	contrib.Id = contribID

	logger.InfoContext(ctx, "request need claimed", "need_id", req.Msg.NeedId, "contribution_id", contribID)

	// Auto-create a RequestOffer for the claimer if they haven't offered yet.
	// This ensures the helper appears in GetRequestPeople.
	if authInfo.UserID != requestStored.RequesterId && req.Msg.CommunityId != "" {
		if err := s.ensureRequestOffer(ctx, req.Msg.RequestId, authInfo.UserID, req.Msg.CommunityId, requestStored); err != nil {
			logger.WarnContext(ctx, "failed to auto-create request offer on claim", "error", err)
		}
	}

	displayName := s.getUserDisplayName(ctx, authInfo.UserID)
	if err := planning.PostNeedClaimed(ctx, s.systemMessageWriter, requestStored.ConversationId, authInfo.UserID, displayName, need, contrib); err != nil {
		return nil, connecterr.Internal(ctx, "ClaimRequestNeed", err)
	}

	contribResp, err := buildRequestContributionResponse(ctx, s, contrib)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.ClaimRequestNeedResponse{Contribution: contribResp}), nil
}

// UnclaimRequestNeed releases a previously claimed need slot (contributor only).
func (s *Service) UnclaimRequestNeed(
	ctx context.Context,
	req *connect.Request[api.UnclaimRequestNeedRequest],
) (*connect.Response[api.UnclaimRequestNeedResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "UnclaimRequestNeed",
		"user_id", authInfo.UserID,
		"contribution_id", req.Msg.ContributionId,
		"target_request_id", req.Msg.RequestId,
	)

	_, err = loadActiveRequest(ctx, s.storage, req.Msg.RequestId)
	if err != nil {
		return nil, err
	}

	contrib := &models.PlanningContribution{}
	if err = s.storage.GetByID(ctx, req.Msg.ContributionId, contrib); err != nil {
		logger.ErrorContext(ctx, "failed to get contribution", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if contrib.GetRequestId() != req.Msg.RequestId {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("contribution not found in this request"))
	}

	if contrib.ContributorId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the contributor can unclaim a need"))
	}

	if contrib.FromNeedId == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("this contribution was not created by claiming a need"))
	}

	if contrib.Deleted != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("contribution already removed"))
	}

	now := clock.UnixSec(ctx)
	contrib.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: now}
	if err = s.storage.Update(ctx, contrib); err != nil {
		logger.ErrorContext(ctx, "failed to soft-delete contribution", "error", err)
		return nil, connecterr.Internal(ctx, "UnclaimRequestNeed", err)
	}

	planning.ReleaseSlot(ctx, s.storage, *contrib.FromNeedId)

	logger.InfoContext(ctx, "request need unclaimed",
		"contribution_id", req.Msg.ContributionId, "need_id", *contrib.FromNeedId)

	return connect.NewResponse(&api.UnclaimRequestNeedResponse{}), nil
}

// loadActiveRequest fetches a request and verifies it is not in a terminal state.
func loadActiveRequest(ctx context.Context, st *storage.ProtoSQLStorage, requestID string) (*models.Request, error) {
	reqStored := &models.Request{}
	if err := st.GetByID(ctx, requestID, reqStored); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if isTerminalRequestState(reqStored.State) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("request is in a terminal state (%s); no changes allowed", reqStored.State))
	}
	return reqStored, nil
}

// isTerminalRequestState reports whether the given state prevents further collaboration.
func isTerminalRequestState(state models.RequestState) bool {
	return state == models.RequestState_REQUEST_STATE_FULFILLED ||
		state == models.RequestState_REQUEST_STATE_CANCELLED
}

// ensureRequestOffer creates a RequestOffer for the user if one does not already exist.
// Also adds the user to the request conversation. Best-effort: callers log on failure.
func (s *Service) ensureRequestOffer(
	ctx context.Context,
	requestID, userID, communityID string,
	requestStored *models.Request,
) error {
	existing, err := s.storage.QueryByFields(ctx, map[string]any{
		"request_id":   requestID,
		"user_id":      userID,
		"community_id": communityID,
		"withdrawn":    false,
	}, &models.RequestOffer{})
	if err != nil {
		return fmt.Errorf("failed to query existing offers: %w", err)
	}
	if len(existing) > 0 {
		return nil // already offered
	}

	offer := &models.RequestOffer{
		RequestId:        requestID,
		UserId:           userID,
		CommunityId:      communityID,
		CreatedAtUnixSec: clock.UnixSec(ctx),
		Withdrawn:        false,
	}
	if _, err := s.storage.Insert(ctx, offer); err != nil {
		return fmt.Errorf("failed to create request offer: %w", err)
	}

	if requestStored.State == models.RequestState_REQUEST_STATE_ACTIVE {
		requestStored.State = models.RequestState_REQUEST_STATE_OFFERS_RECEIVED
		if err := s.storage.Update(ctx, requestStored); err != nil {
			return fmt.Errorf("failed to update request state: %w", err)
		}
	}
	return nil
}

// buildRequestNeedResponse builds an enriched RequestNeedResponse from a stored planning need.
func buildRequestNeedResponse(ctx context.Context, s *Service, need *models.PlanningNeed) (*api.RequestNeedResponse, error) {
	proposer, err := services.FetchAPIUser(ctx, s.storage, need.ProposerId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "buildRequestNeedResponse", err, "detail", "failed to load proposer")
	}
	return &api.RequestNeedResponse{
		Id:               need.Id,
		RequestId:        need.GetRequestId(),
		Proposer:         proposer,
		Name:             need.Name,
		Note:             need.Note,
		Slots:            need.Slots,
		SlotsRemaining:   need.SlotsRemaining,
		CreatedAtUnixSec: need.CreatedAtUnixSec,
	}, nil
}

// buildRequestContributionResponse builds an enriched RequestContributionResponse from a stored planning contribution.
func buildRequestContributionResponse(ctx context.Context, s *Service, contrib *models.PlanningContribution) (*api.RequestContributionResponse, error) {
	contributor, err := services.FetchAPIUser(ctx, s.storage, contrib.ContributorId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "buildRequestContributionResponse", err, "detail", "failed to load contributor")
	}
	return &api.RequestContributionResponse{
		Id:               contrib.Id,
		RequestId:        contrib.GetRequestId(),
		Contributor:      contributor,
		Title:            contrib.Title,
		Description:      contrib.Description,
		FromNeedId:       contrib.FromNeedId,
		OriginalNeedName: contrib.OriginalNeedName,
		OriginalNeedNote: contrib.OriginalNeedNote,
		CreatedAtUnixSec: contrib.CreatedAtUnixSec,
		UpdatedAtUnixSec: contrib.UpdatedAtUnixSec,
		GearId:           contrib.GearId,
	}, nil
}

// reqNilIfEmpty returns nil for an empty string and a pointer otherwise.
// Maps optional proto request fields onto optional storage fields where
// the empty string and the absent value are equivalent.
func reqNilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// BatchClaimRequestNeeds claims slots on multiple needs in one call.
// Per-claim success/failure is reported in the response so a partial
// success (e.g. one slot filled before another claimer arrived) is
// recoverable on the client. Used by the compose flow to apply the
// requester's "I've got this" pre-claims after publishing.
//
// Each claim mirrors the semantics of ClaimRequestNeed: load the need,
// validate gear link, claim a slot, insert a contribution, post a
// system message, and (when claimer != requester) auto-create a
// RequestOffer. A failure on one claim does not abort the batch; it
// records an error_message in the corresponding result and moves on.
func (s *Service) BatchClaimRequestNeeds(
	ctx context.Context,
	req *connect.Request[api.BatchClaimRequestNeedsRequest],
) (*connect.Response[api.BatchClaimRequestNeedsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "BatchClaimRequestNeeds",
		"user_id", authInfo.UserID,
		"target_request_id", req.Msg.RequestId,
		"claim_count", len(req.Msg.Claims),
	)

	if len(req.Msg.Claims) == 0 {
		return connect.NewResponse(&api.BatchClaimRequestNeedsResponse{}), nil
	}

	requestStored, err := loadActiveRequest(ctx, s.storage, req.Msg.RequestId)
	if err != nil {
		return nil, err
	}

	// Reject claims scoped to a soft-deleted community. Empty community_id is
	// allowed for cross-community / requester self-claims (the common
	// pre-claim case).
	if req.Msg.CommunityId != "" {
		if _, err := auth.RequireActiveCommunity(ctx, s.storage, req.Msg.CommunityId); err != nil {
			return nil, err
		}
	}

	displayName := s.getUserDisplayName(ctx, authInfo.UserID)
	results := make([]*api.BatchClaimRequestNeedResult, 0, len(req.Msg.Claims))
	successCount := 0

	for _, claim := range req.Msg.Claims {
		result := &api.BatchClaimRequestNeedResult{NeedId: claim.NeedId}

		contribResp, claimErr := s.batchClaimOneNeed(
			ctx, logger, authInfo.UserID, requestStored,
			req.Msg.CommunityId, displayName, claim,
		)
		if claimErr != nil {
			msg := claimErr.Error()
			result.ErrorMessage = &msg
			logger.InfoContext(ctx, "batch claim entry failed",
				"need_id", claim.NeedId, "error", msg)
		} else {
			result.Contribution = contribResp
			successCount++
		}
		results = append(results, result)
	}

	logger.InfoContext(ctx, "batch claim completed",
		"success_count", successCount,
		"failure_count", len(req.Msg.Claims)-successCount)

	return connect.NewResponse(&api.BatchClaimRequestNeedsResponse{
		Results: results,
	}), nil
}

// batchClaimOneNeed performs the work of a single claim within a batch.
// Returns the API contribution response on success, or a sentinel error
// whose message is safe to surface in the per-claim error_message.
// Mirrors the body of ClaimRequestNeed but never returns connecterr.Internal
// for per-claim failures — those become per-claim error_message strings
// rather than aborting the whole batch.
func (s *Service) batchClaimOneNeed(
	ctx context.Context,
	logger *logging.Logger,
	userID string,
	requestStored *models.Request,
	communityID, displayName string,
	claim *api.BatchClaimRequestNeedItem,
) (*api.RequestContributionResponse, error) {
	need := &models.PlanningNeed{}
	if err := s.storage.GetByID(ctx, claim.NeedId, need); err != nil {
		return nil, fmt.Errorf("need not found")
	}
	if need.GetRequestId() != requestStored.Id {
		return nil, fmt.Errorf("need not found in this request")
	}
	if need.Deleted != nil {
		return nil, fmt.Errorf("need has been removed")
	}

	gearID := claim.GetGearId()
	if err := planning.ValidateGearLink(ctx, s.storage, userID, gearID); err != nil {
		return nil, err
	}

	decremented, err := planning.ClaimSlot(ctx, s.storage, need)
	if err != nil {
		return nil, err
	}

	var description *string
	if claim.Note != nil && *claim.Note != "" {
		description = claim.Note
	}

	contrib := &models.PlanningContribution{
		Scope:            &models.PlanningContribution_RequestId{RequestId: requestStored.Id},
		ContributorId:    userID,
		Title:            need.Name,
		Description:      description,
		FromNeedId:       &need.Id,
		OriginalNeedName: &need.Name,
		OriginalNeedNote: need.Note,
		CreatedAtUnixSec: clock.UnixSec(ctx),
		GearId:           reqNilIfEmpty(gearID),
	}

	contribID, err := s.storage.Insert(ctx, contrib)
	if err != nil {
		logger.ErrorContext(ctx, "failed to insert contribution in batch claim",
			"need_id", claim.NeedId, "error", err)
		if decremented {
			planning.RestoreSlotOnFailure(ctx, s.storage, need.Id)
		}
		return nil, fmt.Errorf("failed to record contribution")
	}
	contrib.Id = contribID

	// Auto-create a RequestOffer for the claimer if not the requester.
	if userID != requestStored.RequesterId && communityID != "" {
		if err := s.ensureRequestOffer(ctx, requestStored.Id, userID, communityID, requestStored); err != nil {
			logger.WarnContext(ctx, "failed to auto-create request offer in batch claim",
				"need_id", claim.NeedId, "error", err)
		}
	}

	if err := planning.PostNeedClaimed(ctx, s.systemMessageWriter, requestStored.ConversationId, userID, displayName, need, contrib); err != nil {
		logger.WarnContext(ctx, "failed to post system message for batch claim",
			"need_id", claim.NeedId, "error", err)
		// Don't fail the claim just because the system message failed —
		// the contribution is already persisted.
	}

	contribResp, err := buildRequestContributionResponse(ctx, s, contrib)
	if err != nil {
		// The contribution exists; the response builder failed. Surface a
		// generic error and let the client refetch.
		logger.ErrorContext(ctx, "failed to build batch claim contribution response",
			"need_id", claim.NeedId, "contribution_id", contribID, "error", err)
		return nil, fmt.Errorf("failed to build contribution response")
	}
	return contribResp, nil
}
