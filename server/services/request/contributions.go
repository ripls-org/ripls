package request

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/planning"
)

// AddRequestContribution adds a free-form contribution to a request (anyone, including the requester).
func (s *Service) AddRequestContribution(
	ctx context.Context,
	req *connect.Request[api.AddRequestContributionRequest],
) (*connect.Response[api.AddRequestContributionResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "AddRequestContribution",
		"user_id", authInfo.UserID,
		"target_request_id", req.Msg.RequestId,
	)

	if req.Msg.Title == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("contribution title is required"))
	}

	requestStored, err := loadActiveRequest(ctx, s.storage, req.Msg.RequestId)
	if err != nil {
		return nil, err
	}

	gearID := req.Msg.GetGearId()
	if err := planning.ValidateGearLink(ctx, s.storage, authInfo.UserID, gearID); err != nil {
		logger.InfoContext(ctx, "rejected gear link on contribution", "gear_id", gearID, "error", err)
		return nil, err
	}

	contrib := &models.PlanningContribution{
		Scope:            &models.PlanningContribution_RequestId{RequestId: req.Msg.RequestId},
		ContributorId:    authInfo.UserID,
		Title:            req.Msg.Title,
		Description:      req.Msg.Description,
		CreatedAtUnixSec: clock.UnixSec(ctx),
		GearId:           reqNilIfEmpty(gearID),
	}

	contribID, err := s.storage.Insert(ctx, contrib)
	if err != nil {
		logger.ErrorContext(ctx, "failed to insert request contribution", "error", err)
		return nil, connecterr.Internal(ctx, "AddRequestContribution", err)
	}
	contrib.Id = contribID

	logger.InfoContext(ctx, "request contribution added",
		"contribution_id", contribID, "title", contrib.Title)

	displayName := s.getUserDisplayName(ctx, authInfo.UserID)
	if err := planning.PostContributionAdded(ctx, s.systemMessageWriter, requestStored.ConversationId, authInfo.UserID, displayName, contrib); err != nil {
		return nil, connecterr.Internal(ctx, "AddRequestContribution", err)
	}

	contribResp, err := buildRequestContributionResponse(ctx, s, contrib)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.AddRequestContributionResponse{Contribution: contribResp}), nil
}

// EditRequestContribution updates a contribution's title and description (contributor only).
func (s *Service) EditRequestContribution(
	ctx context.Context,
	req *connect.Request[api.EditRequestContributionRequest],
) (*connect.Response[api.EditRequestContributionResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "EditRequestContribution",
		"user_id", authInfo.UserID,
		"contribution_id", req.Msg.ContributionId,
		"target_request_id", req.Msg.RequestId,
	)

	if req.Msg.Title == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("contribution title is required"))
	}

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
			fmt.Errorf("only the contributor can edit a contribution"))
	}

	if contrib.Deleted != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("contribution has been removed"))
	}

	gearID := req.Msg.GetGearId()
	if err := planning.ValidateGearLink(ctx, s.storage, authInfo.UserID, gearID); err != nil {
		logger.InfoContext(ctx, "rejected gear link on edit", "gear_id", gearID, "error", err)
		return nil, err
	}

	now := clock.UnixSec(ctx)
	contrib.Title = req.Msg.Title
	contrib.Description = req.Msg.Description
	contrib.GearId = reqNilIfEmpty(gearID)
	contrib.UpdatedAtUnixSec = &now
	if err = s.storage.Update(ctx, contrib); err != nil {
		logger.ErrorContext(ctx, "failed to update contribution", "error", err)
		return nil, connecterr.Internal(ctx, "EditRequestContribution", err)
	}

	logger.InfoContext(ctx, "request contribution updated", "contribution_id", req.Msg.ContributionId)

	contribResp, err := buildRequestContributionResponse(ctx, s, contrib)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.EditRequestContributionResponse{Contribution: contribResp}), nil
}

// RemoveRequestContribution soft-deletes a contribution (contributor only).
func (s *Service) RemoveRequestContribution(
	ctx context.Context,
	req *connect.Request[api.RemoveRequestContributionRequest],
) (*connect.Response[api.RemoveRequestContributionResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RemoveRequestContribution",
		"user_id", authInfo.UserID,
		"contribution_id", req.Msg.ContributionId,
		"target_request_id", req.Msg.RequestId,
	)

	requestStored, err := loadActiveRequest(ctx, s.storage, req.Msg.RequestId)
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
			fmt.Errorf("only the contributor can remove a contribution"))
	}

	if contrib.Deleted != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("contribution already removed"))
	}

	now := clock.UnixSec(ctx)
	contrib.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: now}
	if err = s.storage.Update(ctx, contrib); err != nil {
		logger.ErrorContext(ctx, "failed to soft-delete contribution", "error", err)
		return nil, connecterr.Internal(ctx, "RemoveRequestContribution", err)
	}

	logger.InfoContext(ctx, "request contribution removed", "contribution_id", req.Msg.ContributionId)

	displayName := s.getUserDisplayName(ctx, authInfo.UserID)
	planning.PostContributionRemoved(ctx, s.systemMessageWriter, requestStored.ConversationId, authInfo.UserID, displayName, req.Msg.ContributionId, contrib.Title)

	return connect.NewResponse(&api.RemoveRequestContributionResponse{}), nil
}

// ListRequestNeedsAndContributions returns all active needs and contributions for a request.
func (s *Service) ListRequestNeedsAndContributions(
	ctx context.Context,
	req *connect.Request[api.ListRequestNeedsAndContributionsRequest],
) (*connect.Response[api.ListRequestNeedsAndContributionsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ListRequestNeedsAndContributions",
		"target_request_id", req.Msg.RequestId,
	)

	requestStored := &models.Request{}
	if err := s.storage.GetByID(ctx, req.Msg.RequestId, requestStored); err != nil {
		logger.ErrorContext(ctx, "failed to get request", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	// Caller must be an active member of at least one community the request is
	// shared with, or be the requester.
	if _, _, err := auth.RequireAccessToCommunityScopedEntity(
		ctx, s.storage, authInfo.UserID,
		auth.EntityRequest, requestStored.Id, requestStored.RequesterId,
	); err != nil {
		return nil, err
	}

	scope := planning.Scope{RequestID: req.Msg.RequestId}
	result, err := planning.ListNeedsAndContributions(ctx, s.storage, scope)
	if err != nil {
		logger.ErrorContext(ctx, "failed to list needs and contributions", "error", err)
		return nil, err
	}

	needResps := make([]*api.RequestNeedResponse, 0, len(result.Needs))
	for _, e := range result.Needs {
		needResps = append(needResps, &api.RequestNeedResponse{
			Id:               e.Need.Id,
			RequestId:        e.Need.GetRequestId(),
			Proposer:         e.Proposer,
			Name:             e.Need.Name,
			Note:             e.Need.Note,
			Slots:            e.Need.Slots,
			SlotsRemaining:   e.Need.SlotsRemaining,
			CreatedAtUnixSec: e.Need.CreatedAtUnixSec,
		})
	}

	contribResps := make([]*api.RequestContributionResponse, 0, len(result.Contributions))
	for _, e := range result.Contributions {
		contribResps = append(contribResps, &api.RequestContributionResponse{
			Id:               e.Contribution.Id,
			RequestId:        e.Contribution.GetRequestId(),
			Contributor:      e.Contributor,
			Title:            e.Contribution.Title,
			Description:      e.Contribution.Description,
			FromNeedId:       e.Contribution.FromNeedId,
			OriginalNeedName: e.Contribution.OriginalNeedName,
			OriginalNeedNote: e.Contribution.OriginalNeedNote,
			CreatedAtUnixSec: e.Contribution.CreatedAtUnixSec,
			UpdatedAtUnixSec: e.Contribution.UpdatedAtUnixSec,
			GearId:           e.Contribution.GearId,
		})
	}

	// Lazy-fill suggestions if missing and request is not in a terminal state.
	additionalAsks := requestStored.AdditionalAsks
	breakdownPieces := requestStored.BreakdownPieces
	offerIdeas := requestStored.OfferIdeas
	if len(additionalAsks) == 0 && len(breakdownPieces) == 0 && len(offerIdeas) == 0 &&
		!isTerminalRequestState(requestStored.State) &&
		s.aiProvider != nil &&
		(requestStored.Title != "" || requestStored.Description != "") {
		s.lazyFillRequestSuggestions(ctx, requestStored, logger)
		additionalAsks = requestStored.AdditionalAsks
		breakdownPieces = requestStored.BreakdownPieces
		offerIdeas = requestStored.OfferIdeas
	}

	return connect.NewResponse(&api.ListRequestNeedsAndContributionsResponse{
		Needs:           needResps,
		Contributions:   contribResps,
		AdditionalAsks:  additionalAsks,
		BreakdownPieces: breakdownPieces,
		OfferIdeas:      offerIdeas,
	}), nil
}

// lazyFillRequestSuggestions generates and persists suggestion chips for a request.
// Best-effort: logs warnings and mutates requestStored in place.
func (s *Service) lazyFillRequestSuggestions(ctx context.Context, requestStored *models.Request, logger *logging.Logger) {
	if s.aiProvider == nil {
		return
	}
	start := time.Now()
	genResult, err := ai.CallWithTimeout(ctx, 30*time.Second, func(ctx context.Context) (*ai.RequestSuggestionResult, error) {
		return s.aiProvider.GenerateRequestSuggestions(ctx, requestStored.Title, requestStored.Description, "")
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			logger.WarnContext(ctx, "LLM call exceeded timeout budget, skipping suggestions",
				"external_service", "ai_provider",
				"operation", "GenerateRequestSuggestions",
				"duration_ms", time.Since(start).Milliseconds(),
				"error", err)
		} else {
			logger.WarnContext(ctx, "lazy request suggestion generation failed", "error", err)
		}
		return
	}
	if genResult == nil {
		return
	}
	requestStored.AdditionalAsks = genResult.AdditionalAsks
	requestStored.BreakdownPieces = genResult.BreakdownPieces
	requestStored.OfferIdeas = genResult.OfferIdeas
	if err := s.storage.Update(ctx, requestStored); err != nil {
		logger.WarnContext(ctx, "failed to persist lazily generated request suggestions", "error", err)
	} else {
		logger.DebugContext(ctx, "persisted lazily generated request suggestions",
			"additional_asks", len(genResult.AdditionalAsks),
			"breakdown_pieces", len(genResult.BreakdownPieces),
			"offer_ideas", len(genResult.OfferIdeas))
	}
}
