package experience

import (
	"context"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/planning"
	"go.ripls.org/ripls/server/services"
)

// ListExperienceNeedsAndContributions returns all active needs and contributions for an experience.
func (s *Service) ListExperienceNeedsAndContributions(
	ctx context.Context,
	req *connect.Request[api.ListExperienceNeedsAndContributionsRequest],
) (*connect.Response[api.ListExperienceNeedsAndContributionsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ListExperienceNeedsAndContributions",
		"experience_id", req.Msg.ExperienceId,
	)

	exp, err := s.fetchExperienceForRead(ctx, req.Msg.ExperienceId, logger.Logger, "ListExperienceNeedsAndContributions")
	if err != nil {
		return nil, err
	}

	// Caller must be an active member of at least one community the experience
	// is shared with, or be the owner.
	if _, _, err := auth.RequireAccessToCommunityScopedEntity(
		ctx, s.storage, authInfo.UserID,
		auth.EntityExperience, exp.Id, exp.OwnerId,
	); err != nil {
		return nil, err
	}

	scope := planning.Scope{ExperienceID: req.Msg.ExperienceId}
	result, err := planning.ListNeedsAndContributions(ctx, s.storage, scope)
	if err != nil {
		logger.ErrorContext(ctx, "failed to list needs and contributions", "error", err)
		return nil, err
	}

	needResps := make([]*api.ExperienceNeedResponse, 0, len(result.Needs))
	for _, e := range result.Needs {
		needResps = append(needResps, &api.ExperienceNeedResponse{
			Id:               e.Need.Id,
			ExperienceId:     e.Need.GetExperienceId(),
			Proposer:         e.Proposer,
			Name:             e.Need.Name,
			Note:             e.Need.Note,
			Slots:            e.Need.Slots,
			SlotsRemaining:   e.Need.SlotsRemaining,
			CreatedAtUnixSec: e.Need.CreatedAtUnixSec,
		})
	}

	contribResps := make([]*api.ExperienceContributionResponse, 0, len(result.Contributions))
	for _, e := range result.Contributions {
		contribResps = append(contribResps, &api.ExperienceContributionResponse{
			Id:               e.Contribution.Id,
			ExperienceId:     e.Contribution.GetExperienceId(),
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
	// Enrich rows whose gear was brought to the event as a real loan/giveaway
	// (#2708) with the child transfer's id/state/type — one batched query.
	if err := s.enrichContributionTransfers(ctx, contribResps, result.Contributions); err != nil {
		return nil, err
	}

	// Populate suggestions from the experience. For pre-feature or failed-generation
	// experiences where suggestions is empty, attempt lazy synchronous generation with a
	// 5s timeout. Best-effort: failures are logged as warnings and return empty chips
	// rather than blocking the response.
	suggestions := exp.Suggestions
	categoryHint := exp.CategoryHint
	if len(suggestions) == 0 &&
		!isTerminalExperienceState(exp.State) &&
		s.aiProvider != nil &&
		!clock.IsSimulated(ctx) &&
		(exp.Name != "" || exp.Description != "") {
		genCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		genResult, genErr := s.aiProvider.GenerateExperienceSuggestions(genCtx, exp.Name, exp.Description, exp.Category)
		if genErr != nil {
			logger.WarnContext(ctx, "lazy suggestion generation failed", "error", genErr)
		} else if genResult != nil && len(genResult.Suggestions) > 0 {
			suggestions = genResult.Suggestions
			if genResult.CategoryHint != "" {
				categoryHint = &genResult.CategoryHint
			}
			exp.Suggestions = suggestions
			exp.CategoryHint = categoryHint
			if updateErr := s.storage.Update(ctx, exp); updateErr != nil {
				logger.WarnContext(ctx, "failed to persist lazily generated suggestions", "error", updateErr)
			} else {
				logger.DebugContext(ctx, "persisted lazily generated suggestions",
					"count", len(suggestions),
					"category_hint", genResult.CategoryHint)
			}
		}
	}

	return connect.NewResponse(&api.ListExperienceNeedsAndContributionsResponse{
		Needs:         needResps,
		Contributions: contribResps,
		Suggestions:   suggestions,
		CategoryHint:  categoryHint,
	}), nil
}

// buildNeedResponse builds an enriched ExperienceNeedResponse from a stored planning need.
func (s *Service) buildNeedResponse(ctx context.Context, need *models.PlanningNeed) (*api.ExperienceNeedResponse, error) {
	proposer, err := services.FetchAPIUser(ctx, s.storage, need.ProposerId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "buildNeedResponse", err)
	}
	return &api.ExperienceNeedResponse{
		Id:               need.Id,
		ExperienceId:     need.GetExperienceId(),
		Proposer:         proposer,
		Name:             need.Name,
		Note:             need.Note,
		Slots:            need.Slots,
		SlotsRemaining:   need.SlotsRemaining,
		CreatedAtUnixSec: need.CreatedAtUnixSec,
	}, nil
}

// buildContributionResponse builds an enriched ExperienceContributionResponse from a stored planning contribution.
func (s *Service) buildContributionResponse(ctx context.Context, contrib *models.PlanningContribution) (*api.ExperienceContributionResponse, error) {
	contributor, err := services.FetchAPIUser(ctx, s.storage, contrib.ContributorId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "buildContributionResponse", err)
	}
	resp := &api.ExperienceContributionResponse{
		Id:               contrib.Id,
		ExperienceId:     contrib.GetExperienceId(),
		Contributor:      contributor,
		Title:            contrib.Title,
		Description:      contrib.Description,
		FromNeedId:       contrib.FromNeedId,
		OriginalNeedName: contrib.OriginalNeedName,
		OriginalNeedNote: contrib.OriginalNeedNote,
		CreatedAtUnixSec: contrib.CreatedAtUnixSec,
		UpdatedAtUnixSec: contrib.UpdatedAtUnixSec,
		GearId:           contrib.GearId,
	}
	// If this contribution's gear was brought to the event as a real transfer
	// (#2708), surface the child transfer's id/state/type.
	if tid := contrib.GetTransferId(); tid != "" {
		transfer := &models.Transfer{}
		if err := s.storage.GetByID(ctx, tid, transfer); err == nil {
			applyTransferToContribResp(resp, transfer)
		}
	}
	return resp, nil
}

// nilIfEmpty returns nil for an empty string and a pointer otherwise.
// Used to map optional proto request fields onto optional storage fields
// where the empty string and the absent value are equivalent.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
