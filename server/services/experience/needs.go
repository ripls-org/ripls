// Package experience implements the ExperienceService RPC handlers.
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
	"go.ripls.org/ripls/server/rsvpstate"
)

const (
	maxNeedSlots     = 20
	defaultNeedSlots = 1
)

// AddExperienceNeed adds a new need to an experience (any participant).
func (s *Service) AddExperienceNeed(
	ctx context.Context,
	req *connect.Request[api.AddExperienceNeedRequest],
) (*connect.Response[api.AddExperienceNeedResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "AddExperienceNeed",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("need name is required"))
	}

	slots := req.Msg.Slots
	if slots <= 0 {
		slots = defaultNeedSlots
	}
	if slots > maxNeedSlots {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("slots cannot exceed %d", maxNeedSlots))
	}

	exp, err := s.loadActiveExperience(ctx, req.Msg.ExperienceId)
	if err != nil {
		return nil, err
	}

	if err = s.requireParticipant(ctx, authInfo.UserID, exp); err != nil {
		return nil, err
	}

	need := &models.PlanningNeed{
		Scope:            &models.PlanningNeed_ExperienceId{ExperienceId: req.Msg.ExperienceId},
		ProposerId:       authInfo.UserID,
		Name:             req.Msg.Name,
		Note:             req.Msg.Note,
		Slots:            slots,
		SlotsRemaining:   slots,
		CreatedAtUnixSec: clock.UnixSec(ctx),
	}

	needID, err := s.storage.Insert(ctx, need)
	if err != nil {
		logger.ErrorContext(ctx, "failed to insert experience need", "error", err)
		return nil, connecterr.Internal(ctx, "AddExperienceNeed", err)
	}
	need.Id = needID

	logger.InfoContext(ctx, "experience need added", "need_id", needID, "name", need.Name)

	displayName := s.getUserDisplayName(ctx, authInfo.UserID)
	if err := planning.PostNeedAdded(ctx, s.systemMessageWriter, exp.ConversationId, authInfo.UserID, displayName, need, nil); err != nil {
		return nil, connecterr.Internal(ctx, "AddExperienceNeed", err)
	}
	s.emitPlanningCommunityEvent(ctx,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED,
		authInfo.UserID, req.Msg.ExperienceId, need.Name)

	needResp, err := s.buildNeedResponse(ctx, need)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.AddExperienceNeedResponse{Need: needResp}), nil
}

// RemoveExperienceNeed removes a need from an experience (proposer only).
func (s *Service) RemoveExperienceNeed(
	ctx context.Context,
	req *connect.Request[api.RemoveExperienceNeedRequest],
) (*connect.Response[api.RemoveExperienceNeedResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RemoveExperienceNeed",
		"user_id", authInfo.UserID,
		"need_id", req.Msg.NeedId,
		"experience_id", req.Msg.ExperienceId,
	)

	exp, err := s.loadActiveExperience(ctx, req.Msg.ExperienceId)
	if err != nil {
		return nil, err
	}

	need := &models.PlanningNeed{}
	if err = s.storage.GetByID(ctx, req.Msg.NeedId, need); err != nil {
		logger.ErrorContext(ctx, "failed to get experience need", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if expID := need.GetExperienceId(); expID != req.Msg.ExperienceId {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("need not found in this experience"))
	}

	if need.ProposerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the proposer can remove a need"))
	}

	now := clock.UnixSec(ctx)
	need.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: now}
	if err = s.storage.Update(ctx, need); err != nil {
		logger.ErrorContext(ctx, "failed to soft-delete experience need", "error", err)
		return nil, connecterr.Internal(ctx, "RemoveExperienceNeed", err)
	}

	// Cascade: every contribution that pointed at this need (claims
	// recorded via from_need_id) is voided alongside the need. Mirrors
	// the Request-scope behavior so the two surfaces stay in lockstep
	// per docs/client/needs.md.
	cascaded, cascadeErr := planning.SoftDeleteContributionsForNeed(ctx, s.storage, req.Msg.NeedId)
	if cascadeErr != nil {
		logger.ErrorContext(ctx, "failed to cascade-soft-delete contributions for need",
			"need_id", req.Msg.NeedId, "error", cascadeErr)
		return nil, cascadeErr
	}

	logger.InfoContext(ctx, "experience need removed",
		"need_id", req.Msg.NeedId, "cascaded_contributions", cascaded)

	displayName := s.getUserDisplayName(ctx, authInfo.UserID)
	planning.PostNeedRemoved(ctx, s.systemMessageWriter, exp.ConversationId, authInfo.UserID, displayName, req.Msg.NeedId, need.Name)

	return connect.NewResponse(&api.RemoveExperienceNeedResponse{}), nil
}

// UpdateExperienceNeed edits an existing need in place (proposer only). Any
// fields set on the request replace the current values on the need; fields
// left unset are preserved. Reducing slots below the current claim count
// (slots - slots_remaining) fails with FailedPrecondition — callers must
// release claims first.
func (s *Service) UpdateExperienceNeed(
	ctx context.Context,
	req *connect.Request[api.UpdateExperienceNeedRequest],
) (*connect.Response[api.UpdateExperienceNeedResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "UpdateExperienceNeed",
		"user_id", authInfo.UserID,
		"need_id", req.Msg.NeedId,
		"experience_id", req.Msg.ExperienceId,
	)

	exp, err := s.loadActiveExperience(ctx, req.Msg.ExperienceId)
	if err != nil {
		return nil, err
	}

	need := &models.PlanningNeed{}
	if err = s.storage.GetByID(ctx, req.Msg.NeedId, need); err != nil {
		logger.ErrorContext(ctx, "failed to get experience need", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if expID := need.GetExperienceId(); expID != req.Msg.ExperienceId {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("need not found in this experience"))
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
		if newSlots > maxNeedSlots {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("slots cannot exceed %d", maxNeedSlots))
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
			logger.ErrorContext(ctx, "failed to update experience need", "error", err)
			return nil, connecterr.Internal(ctx, "UpdateExperienceNeed", err)
		}
		logger.InfoContext(ctx, "experience need updated", "need_id", need.Id)

		displayName := s.getUserDisplayName(ctx, authInfo.UserID)
		planning.PostNeedUpdated(ctx, s.systemMessageWriter, exp.ConversationId, authInfo.UserID, displayName, need.Id, need.Name)
		s.emitPlanningCommunityEvent(ctx,
			models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_UPDATED,
			authInfo.UserID, req.Msg.ExperienceId, need.Name)
	}

	needResp, err := s.buildNeedResponse(ctx, need)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.UpdateExperienceNeedResponse{Need: needResp}), nil
}

// ClaimExperienceNeed claims a slot on a need, creating a linked contribution.
func (s *Service) ClaimExperienceNeed(
	ctx context.Context,
	req *connect.Request[api.ClaimExperienceNeedRequest],
) (*connect.Response[api.ClaimExperienceNeedResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ClaimExperienceNeed",
		"user_id", authInfo.UserID,
		"need_id", req.Msg.NeedId,
		"experience_id", req.Msg.ExperienceId,
	)

	exp, err := s.loadActiveExperience(ctx, req.Msg.ExperienceId)
	if err != nil {
		return nil, err
	}

	if err = s.requireParticipant(ctx, authInfo.UserID, exp); err != nil {
		return nil, err
	}

	need := &models.PlanningNeed{}
	if err = s.storage.GetByID(ctx, req.Msg.NeedId, need); err != nil {
		logger.ErrorContext(ctx, "failed to get experience need", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if expID := need.GetExperienceId(); expID != req.Msg.ExperienceId {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("need not found in this experience"))
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
		Scope:            &models.PlanningContribution_ExperienceId{ExperienceId: req.Msg.ExperienceId},
		ContributorId:    authInfo.UserID,
		Title:            need.Name,
		Description:      description,
		FromNeedId:       &need.Id,
		OriginalNeedName: &need.Name,
		OriginalNeedNote: need.Note,
		CreatedAtUnixSec: clock.UnixSec(ctx),
		GearId:           nilIfEmpty(gearID),
	}

	contribID, err := s.storage.Insert(ctx, contrib)
	if err != nil {
		logger.ErrorContext(ctx, "failed to insert contribution from need claim", "error", err)
		if decremented {
			planning.RestoreSlotOnFailure(ctx, s.storage, need.Id)
		}
		return nil, connecterr.Internal(ctx, "ClaimExperienceNeed", err)
	}
	contrib.Id = contribID

	logger.InfoContext(ctx, "experience need claimed",
		"need_id", req.Msg.NeedId, "contribution_id", contribID)

	displayName := s.getUserDisplayName(ctx, authInfo.UserID)
	if err := planning.PostNeedClaimed(ctx, s.systemMessageWriter, exp.ConversationId, authInfo.UserID, displayName, need, contrib); err != nil {
		return nil, connecterr.Internal(ctx, "ClaimExperienceNeed", err)
	}
	s.emitPlanningCommunityEvent(ctx,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_CLAIMED,
		authInfo.UserID, req.Msg.ExperienceId, need.Name)

	contribResp, err := s.buildContributionResponse(ctx, contrib)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.ClaimExperienceNeedResponse{Contribution: contribResp}), nil
}

// UnclaimExperienceNeed removes a claimed contribution and returns the slot to the need.
func (s *Service) UnclaimExperienceNeed(
	ctx context.Context,
	req *connect.Request[api.UnclaimExperienceNeedRequest],
) (*connect.Response[api.UnclaimExperienceNeedResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "UnclaimExperienceNeed",
		"user_id", authInfo.UserID,
		"contribution_id", req.Msg.ContributionId,
		"experience_id", req.Msg.ExperienceId,
	)

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
		return nil, connecterr.Internal(ctx, "UnclaimExperienceNeed", err)
	}

	planning.ReleaseSlot(ctx, s.storage, *contrib.FromNeedId)

	logger.InfoContext(ctx, "experience need unclaimed",
		"contribution_id", req.Msg.ContributionId, "need_id", *contrib.FromNeedId)

	return connect.NewResponse(&api.UnclaimExperienceNeedResponse{}), nil
}

// BatchAddExperienceNeeds adds multiple needs to an experience in a single call (any participant).
func (s *Service) BatchAddExperienceNeeds(
	ctx context.Context,
	req *connect.Request[api.BatchAddExperienceNeedsRequest],
) (*connect.Response[api.BatchAddExperienceNeedsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "BatchAddExperienceNeeds",
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
	needModels := make([]*models.PlanningNeed, 0, len(req.Msg.Items))
	for i, item := range req.Msg.Items {
		if item.Name == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("item %d: need name is required", i))
		}
		slots := item.Slots
		if slots <= 0 {
			slots = defaultNeedSlots
		}
		if slots > maxNeedSlots {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("item %d: slots cannot exceed %d", i, maxNeedSlots))
		}
		needModels = append(needModels, &models.PlanningNeed{
			Scope:            &models.PlanningNeed_ExperienceId{ExperienceId: req.Msg.ExperienceId},
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
		logger.ErrorContext(ctx, "failed to batch insert experience needs", "error", err)
		return nil, connecterr.Internal(ctx, "BatchAddExperienceNeeds", err)
	}
	for i, id := range ids {
		needModels[i].Id = id
	}

	logger.InfoContext(ctx, "experience needs batch added", "count", len(needModels))

	displayName := s.getUserDisplayName(ctx, authInfo.UserID)
	// Sequential timestamps keep the lines in creation order — the whole
	// batch lands in the same clock second, and same-second ties otherwise
	// sort on a random UUID id (#2724).
	batchStartSec := clock.UnixSec(ctx)
	for i, need := range needModels {
		sentAt := batchStartSec + int64(i)
		if err := planning.PostNeedAdded(ctx, s.systemMessageWriter, exp.ConversationId, authInfo.UserID, displayName, need, &sentAt); err != nil {
			logger.WarnContext(ctx, "failed to post need-added message", "need_id", need.Id, "error", err)
		}
	}
	if len(needModels) > 0 {
		s.emitPlanningCommunityEvent(ctx,
			models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED,
			authInfo.UserID, req.Msg.ExperienceId, "")
	}

	needResps := make([]*api.ExperienceNeedResponse, 0, len(needModels))
	for _, need := range needModels {
		needResp, err := s.buildNeedResponse(ctx, need)
		if err != nil {
			return nil, err
		}
		needResps = append(needResps, needResp)
	}
	return connect.NewResponse(&api.BatchAddExperienceNeedsResponse{Needs: needResps}), nil
}

// loadActiveExperience fetches an experience and verifies it is not in a terminal state.
func (s *Service) loadActiveExperience(ctx context.Context, experienceID string) (*models.Experience, error) {
	exp := &models.Experience{}
	if err := s.storage.GetByID(ctx, experienceID, exp); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if isTerminalExperienceState(exp.State) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("experience is in a terminal state (%s); no changes allowed", exp.State))
	}
	return exp, nil
}

// isTerminalExperienceState reports whether the given state prevents further collaboration.
func isTerminalExperienceState(state models.ExperienceState) bool {
	return state == models.ExperienceState_EXPERIENCE_STATE_COMPLETED ||
		state == models.ExperienceState_EXPERIENCE_STATE_CANCELLED
}

// requireParticipant checks that the user is the owner or has RSVPed Yes/Maybe.
func (s *Service) requireParticipant(ctx context.Context, userID string, exp *models.Experience) error {
	if exp.OwnerId == userID {
		return nil
	}
	rsvps, err := s.storage.QueryByFields(ctx, map[string]any{
		"experience_id": exp.Id,
		"user_id":       userID,
	}, &models.ExperienceRSVP{})
	if err != nil {
		return connecterr.Internal(ctx, "requireParticipant", err, "detail", "failed to check participation")
	}
	for _, m := range rsvps {
		rsvp := m.(*models.ExperienceRSVP)
		if rsvpstate.IsGoing(rsvp.GetIntention()) {
			return nil
		}
	}
	return connect.NewError(connect.CodePermissionDenied,
		fmt.Errorf("you must be an experience participant to perform this action"))
}
