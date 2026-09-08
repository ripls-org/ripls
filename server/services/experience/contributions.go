package experience

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
)

// AddExperienceContribution adds a free-form contribution to an experience (any participant).
func (s *Service) AddExperienceContribution(
	ctx context.Context,
	req *connect.Request[api.AddExperienceContributionRequest],
) (*connect.Response[api.AddExperienceContributionResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "AddExperienceContribution",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	if req.Msg.Title == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("contribution title is required"))
	}

	exp, err := s.loadActiveExperience(ctx, req.Msg.ExperienceId)
	if err != nil {
		return nil, err
	}

	if err = s.requireParticipant(ctx, authInfo.UserID, exp); err != nil {
		return nil, err
	}

	gearID := req.Msg.GetGearId()
	if err := planning.ValidateGearLink(ctx, s.storage, authInfo.UserID, gearID); err != nil {
		logger.InfoContext(ctx, "rejected gear link on contribution", "gear_id", gearID, "error", err)
		return nil, err
	}

	contrib := &models.PlanningContribution{
		Scope:            &models.PlanningContribution_ExperienceId{ExperienceId: req.Msg.ExperienceId},
		ContributorId:    authInfo.UserID,
		Title:            req.Msg.Title,
		Description:      req.Msg.Description,
		CreatedAtUnixSec: clock.UnixSec(ctx),
		GearId:           nilIfEmpty(gearID),
	}

	contribID, err := s.storage.Insert(ctx, contrib)
	if err != nil {
		logger.ErrorContext(ctx, "failed to insert experience contribution", "error", err)
		return nil, connecterr.Internal(ctx, "AddExperienceContribution", err)
	}
	contrib.Id = contribID

	logger.InfoContext(ctx, "experience contribution added",
		"contribution_id", contribID, "title", contrib.Title)

	displayName := s.getUserDisplayName(ctx, authInfo.UserID)
	if err := planning.PostContributionAdded(ctx, s.systemMessageWriter, exp.ConversationId, authInfo.UserID, displayName, contrib); err != nil {
		return nil, connecterr.Internal(ctx, "AddExperienceContribution", err)
	}
	s.emitPlanningCommunityEvent(ctx,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_ADDED,
		authInfo.UserID, req.Msg.ExperienceId, contrib.Title)

	contribResp, err := s.buildContributionResponse(ctx, contrib)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.AddExperienceContributionResponse{Contribution: contribResp}), nil
}

// EditExperienceContribution updates a contribution's title and description (contributor only).
func (s *Service) EditExperienceContribution(
	ctx context.Context,
	req *connect.Request[api.EditExperienceContributionRequest],
) (*connect.Response[api.EditExperienceContributionResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "EditExperienceContribution",
		"user_id", authInfo.UserID,
		"contribution_id", req.Msg.ContributionId,
		"experience_id", req.Msg.ExperienceId,
	)

	if req.Msg.Title == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("contribution title is required"))
	}

	_, err = s.loadActiveExperience(ctx, req.Msg.ExperienceId)
	if err != nil {
		return nil, err
	}

	contrib := &models.PlanningContribution{}
	if err = s.storage.GetByID(ctx, req.Msg.ContributionId, contrib); err != nil {
		logger.ErrorContext(ctx, "failed to get contribution", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if expID := contrib.GetExperienceId(); expID != req.Msg.ExperienceId {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("contribution not found in this experience"))
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
	contrib.GearId = nilIfEmpty(gearID)
	contrib.UpdatedAtUnixSec = &now
	if err = s.storage.Update(ctx, contrib); err != nil {
		logger.ErrorContext(ctx, "failed to update contribution", "error", err)
		return nil, connecterr.Internal(ctx, "EditExperienceContribution", err)
	}

	logger.InfoContext(ctx, "experience contribution updated", "contribution_id", req.Msg.ContributionId)

	contribResp, err := s.buildContributionResponse(ctx, contrib)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.EditExperienceContributionResponse{Contribution: contribResp}), nil
}

// BatchAddExperienceContributions adds multiple free-form contributions to an experience in a single call (any participant).
func (s *Service) BatchAddExperienceContributions(
	ctx context.Context,
	req *connect.Request[api.BatchAddExperienceContributionsRequest],
) (*connect.Response[api.BatchAddExperienceContributionsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "BatchAddExperienceContributions",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	if len(req.Msg.Items) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("at least one item is required"))
	}

	exp, err := s.loadActiveExperience(ctx, req.Msg.ExperienceId)
	if err != nil {
		return nil, err
	}

	if err = s.requireParticipant(ctx, authInfo.UserID, exp); err != nil {
		return nil, err
	}

	now := clock.UnixSec(ctx)
	contribModels := make([]*models.PlanningContribution, 0, len(req.Msg.Items))
	for i, item := range req.Msg.Items {
		if item.Title == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("item %d: contribution title is required", i))
		}
		contribModels = append(contribModels, &models.PlanningContribution{
			Scope:            &models.PlanningContribution_ExperienceId{ExperienceId: req.Msg.ExperienceId},
			ContributorId:    authInfo.UserID,
			Title:            item.Title,
			Description:      item.Description,
			CreatedAtUnixSec: now,
		})
	}

	protoMsgs := make([]proto.Message, len(contribModels))
	for i, c := range contribModels {
		protoMsgs[i] = c
	}

	ids, err := s.storage.InsertBatch(ctx, protoMsgs)
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch insert experience contributions", "error", err)
		return nil, connecterr.Internal(ctx, "BatchAddExperienceContributions", err)
	}
	for i, id := range ids {
		contribModels[i].Id = id
	}

	logger.InfoContext(ctx, "experience contributions batch added", "count", len(contribModels))

	displayName := s.getUserDisplayName(ctx, authInfo.UserID)
	for _, contrib := range contribModels {
		if err := planning.PostContributionAdded(ctx, s.systemMessageWriter, exp.ConversationId, authInfo.UserID, displayName, contrib); err != nil {
			logger.WarnContext(ctx, "failed to post contribution-added message", "contribution_id", contrib.Id, "error", err)
		}
	}
	if len(contribModels) > 0 {
		s.emitPlanningCommunityEvent(ctx,
			models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_ADDED,
			authInfo.UserID, req.Msg.ExperienceId, "")
	}

	contribResps := make([]*api.ExperienceContributionResponse, 0, len(contribModels))
	for _, contrib := range contribModels {
		contribResp, err := s.buildContributionResponse(ctx, contrib)
		if err != nil {
			return nil, err
		}
		contribResps = append(contribResps, contribResp)
	}
	return connect.NewResponse(&api.BatchAddExperienceContributionsResponse{Contributions: contribResps}), nil
}

// RemoveExperienceContribution soft-deletes a contribution (contributor only).
func (s *Service) RemoveExperienceContribution(
	ctx context.Context,
	req *connect.Request[api.RemoveExperienceContributionRequest],
) (*connect.Response[api.RemoveExperienceContributionResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RemoveExperienceContribution",
		"user_id", authInfo.UserID,
		"contribution_id", req.Msg.ContributionId,
		"experience_id", req.Msg.ExperienceId,
	)

	exp, err := s.loadActiveExperience(ctx, req.Msg.ExperienceId)
	if err != nil {
		return nil, err
	}

	contrib := &models.PlanningContribution{}
	if err = s.storage.GetByID(ctx, req.Msg.ContributionId, contrib); err != nil {
		logger.ErrorContext(ctx, "failed to get contribution", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if expID := contrib.GetExperienceId(); expID != req.Msg.ExperienceId {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("contribution not found in this experience"))
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
		return nil, connecterr.Internal(ctx, "RemoveExperienceContribution", err)
	}

	logger.InfoContext(ctx, "experience contribution removed", "contribution_id", req.Msg.ContributionId)

	displayName := s.getUserDisplayName(ctx, authInfo.UserID)
	planning.PostContributionRemoved(ctx, s.systemMessageWriter, exp.ConversationId, authInfo.UserID, displayName, req.Msg.ContributionId, contrib.Title)

	return connect.NewResponse(&api.RemoveExperienceContributionResponse{}), nil
}
