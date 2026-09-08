package planning

import (
	"context"

	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// EnrichedNeeds holds a batch of PlanningNeeds with their proposers resolved.
type EnrichedNeeds struct {
	Need     *models.PlanningNeed
	Proposer *api.User
}

// EnrichedContributions holds a PlanningContribution with its contributor resolved.
type EnrichedContribution struct {
	Contribution *models.PlanningContribution
	Contributor  *api.User
}

// ListResult holds the enriched needs and contributions for a scope.
type ListResult struct {
	Needs         []*EnrichedNeeds
	Contributions []*EnrichedContribution
}

// ListNeedsAndContributions fetches all active (non-deleted) needs and contributions
// for the given scope, enriches them with user profiles, and returns the result.
// Fully-claimed needs (slots_remaining == 0) are excluded from the needs list.
func ListNeedsAndContributions(
	ctx context.Context,
	st *storage.ProtoSQLStorage,
	scope Scope,
) (*ListResult, error) {
	scopeField := scope.scopeFieldName()

	rawNeeds, err := storage.QueryByFields[*models.PlanningNeed](
		st, ctx, map[string]any{scopeField: scope.ScopeID()},
	)
	if err != nil {
		return nil, connecterr.Internal(ctx, "ListNeedsAndContributions", err)
	}

	rawContribs, err := storage.QueryByFields[*models.PlanningContribution](
		st, ctx, map[string]any{scopeField: scope.ScopeID()},
	)
	if err != nil {
		return nil, connecterr.Internal(ctx, "ListNeedsAndContributions", err)
	}

	// Collect all user IDs for a single batch fetch.
	userIDSet := make(map[string]struct{})
	for _, n := range rawNeeds {
		userIDSet[n.ProposerId] = struct{}{}
	}
	for _, c := range rawContribs {
		userIDSet[c.ContributorId] = struct{}{}
	}
	userIDs := make([]string, 0, len(userIDSet))
	for id := range userIDSet {
		userIDs = append(userIDs, id)
	}

	userMap, err := services.FetchAPIUsersBatch(ctx, st, userIDs)
	if err != nil {
		return nil, connecterr.Internal(ctx, "ListNeedsAndContributions", err)
	}

	// Build enriched needs. v2 returns every active need including
	// fully-claimed ones — the client renders them in a single
	// unified list with voter stacks showing who covered each slot.
	// The v1 "exclude when SlotsRemaining == 0" filter was a relic
	// of the two-column "Still needed / Who's bringing what" split
	// and is gone with v2. See docs/client/needs.md.
	enrichedNeeds := make([]*EnrichedNeeds, 0, len(rawNeeds))
	for _, n := range rawNeeds {
		proposer := userMap[n.ProposerId]
		if proposer == nil {
			continue
		}
		enrichedNeeds = append(enrichedNeeds, &EnrichedNeeds{Need: n, Proposer: proposer})
	}

	// Build enriched contributions.
	enrichedContribs := make([]*EnrichedContribution, 0, len(rawContribs))
	for _, c := range rawContribs {
		contributor := userMap[c.ContributorId]
		if contributor == nil {
			continue
		}
		enrichedContribs = append(enrichedContribs, &EnrichedContribution{Contribution: c, Contributor: contributor})
	}

	return &ListResult{Needs: enrichedNeeds, Contributions: enrichedContribs}, nil
}
