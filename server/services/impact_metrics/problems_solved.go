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
)

// getCommunityProblemsSolvedDetail returns the community's "problems handled"
// X-of-Y breakdown — handled (completed loans + fulfilled requests + claimed
// needs) over potential (all non-cancelled loans / requests / posted needs) —
// with the handled entries listed most-recent-first.
func getCommunityProblemsSolvedDetail(
	ctx context.Context,
	s *Service,
	req *connect.Request[api.GetCommunityProblemsSolvedDetailRequest],
) (*connect.Response[api.GetCommunityProblemsSolvedDetailResponse], error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetCommunityProblemsSolvedDetail",
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

	store := s.calculator.Storage()
	cache := impactlib.NewEntityCache(store)

	// Authoritative X/Y counts — shared with the Workshop headline so the
	// fraction can never drift from the deep-dive.
	counts, err := s.calculator.CalculateProblemsCounts(ctx, req.Msg.CommunityId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to compute problems counts", "error", err)
		return nil, connecterr.Internal(ctx, "getCommunityProblemsSolvedDetail", err, "detail", "failed to compute problems counts")
	}

	// Handled entries for the list.
	transfers, err := loadCompletedTransfers(ctx, store, req.Msg.CommunityId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to load transfers", "error", err)
		return nil, connecterr.Internal(ctx, "getCommunityProblemsSolvedDetail", err, "detail", "failed to load transfers")
	}
	requests, err := loadFulfilledRequests(ctx, store, req.Msg.CommunityId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to load requests", "error", err)
		return nil, connecterr.Internal(ctx, "getCommunityProblemsSolvedDetail", err, "detail", "failed to load requests")
	}
	needData, err := s.calculator.CommunityNeedData(ctx, req.Msg.CommunityId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to load needs", "error", err)
		return nil, connecterr.Internal(ctx, "getCommunityProblemsSolvedDetail", err, "detail", "failed to load needs")
	}

	var items []*api.ProblemSolvedItem
	for _, t := range transfers {
		// Only loans count as a handled problem here (giveaways excluded).
		if t.TransferType != models.TransferType_TRANSFER_TYPE_LOAN {
			continue
		}
		items = append(items, transferToProblemItem(ctx, t, cache, logger))
	}
	for _, r := range requests {
		items = append(items, requestToProblemItem(ctx, r, cache, logger))
	}
	for _, n := range needData.Needs {
		claim, ok := needData.Claims[n.Id]
		if !ok {
			continue // unclaimed need: counted in potential, not a handled entry
		}
		items = append(items, needToProblemItem(ctx, n, claim, cache, logger))
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].CompletedAtUnixSec > items[j].CompletedAtUnixSec
	})

	logger.InfoContext(ctx, "returning problems-handled detail",
		"handled", counts.Handled(), "potential", counts.Potential(),
	)

	return connect.NewResponse(&api.GetCommunityProblemsSolvedDetailResponse{
		HandledCount:      int32(counts.Handled()),
		HandledLoans:      int32(counts.HandledLoans),
		HandledRequests:   int32(counts.HandledRequests),
		HandledNeeds:      int32(counts.HandledNeeds),
		Items:             items,
		PotentialCount:    int32(counts.Potential()),
		PotentialLoans:    int32(counts.PotentialLoans),
		PotentialRequests: int32(counts.PotentialRequests),
		PotentialNeeds:    int32(counts.PotentialNeeds),
	}), nil
}

func transferToProblemItem(
	ctx context.Context,
	t *models.Transfer,
	cache *impactlib.EntityCache,
	logger *logging.Logger,
) *api.ProblemSolvedItem {
	title := "Unknown item"
	media := ""
	if gear := cache.GetGear(ctx, t.GearId, logger); gear != nil {
		title = gear.Name
		if len(gear.MediaIds) > 0 {
			media = gear.MediaIds[0]
		}
	}
	person := ""
	if t.RecipientId != "" {
		if u := cache.GetUser(ctx, t.RecipientId, logger); u != nil {
			person = u.Name
		}
	}
	var completedAt int64
	if t.ActualReturnUnixSec != nil {
		completedAt = *t.ActualReturnUnixSec
	}
	return &api.ProblemSolvedItem{
		Kind:               api.ProblemSolvedKind_PROBLEM_SOLVED_KIND_LOAN,
		Title:              title,
		PersonName:         person,
		CompletedAtUnixSec: completedAt,
		ContentId:          t.GearId,
		MediaId:            media,
		ContentType:        "gear",
	}
}

func requestToProblemItem(
	ctx context.Context,
	r *models.Request,
	cache *impactlib.EntityCache,
	logger *logging.Logger,
) *api.ProblemSolvedItem {
	person := ""
	if len(r.ConfirmedHelperIds) > 0 {
		if u := cache.GetUser(ctx, r.ConfirmedHelperIds[0], logger); u != nil {
			person = u.Name
		}
	}
	media := ""
	if len(r.MediaIds) > 0 {
		media = r.MediaIds[0]
	}
	var completedAt int64
	if r.FulfilledAtUnixSec != nil {
		completedAt = *r.FulfilledAtUnixSec
	}
	return &api.ProblemSolvedItem{
		Kind:               api.ProblemSolvedKind_PROBLEM_SOLVED_KIND_REQUEST,
		Title:              r.Title,
		PersonName:         person,
		CompletedAtUnixSec: completedAt,
		ContentId:          r.Id,
		MediaId:            media,
		ContentType:        "request",
	}
}

// needToProblemItem builds the entry for a claimed planning need. The title is
// the need name, the person is whoever claimed it (the contribution's
// contributor), and the timestamp is when the claim was made. Navigation
// targets the need's parent experience or request.
func needToProblemItem(
	ctx context.Context,
	n *models.PlanningNeed,
	claim *models.PlanningContribution,
	cache *impactlib.EntityCache,
	logger *logging.Logger,
) *api.ProblemSolvedItem {
	person := ""
	if claim.ContributorId != "" {
		if u := cache.GetUser(ctx, claim.ContributorId, logger); u != nil {
			person = u.Name
		}
	}
	contentID := n.GetExperienceId()
	contentType := "experience"
	if contentID == "" {
		contentID = n.GetRequestId()
		contentType = "request"
	}
	return &api.ProblemSolvedItem{
		Kind:               api.ProblemSolvedKind_PROBLEM_SOLVED_KIND_NEED,
		Title:              n.Name,
		PersonName:         person,
		CompletedAtUnixSec: claim.CreatedAtUnixSec,
		ContentId:          contentID,
		ContentType:        contentType,
	}
}
