package known_for

import (
	"context"
	"fmt"
	"strings"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// addAskerCounts accumulates request-participation contributions
// into [counts]. The "participation" set covers both requesters (per
// the requester_id) and, on per-user mode, confirmed helpers (per
// the request's confirmed_helper_ids list). Per-community mode
// counts each request once regardless of helper count.
//
// Weighting per category:
//   - +1 for every distinct participated request
//   - +1 bonus when the request's state is FULFILLED
//
// Query budget:
//   - per-user: 4 reads (2 for requested + 2 for helped)
//   - per-community: 2 reads (community_request by community-in +
//     request batch)
func addAskerCounts(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	sharedSet map[string]struct{},
	counts categoryCounts,
) error {
	requests, err := loadScopedRequests(ctx, s, opts, sharedSet)
	if err != nil {
		return fmt.Errorf("load requests: %w", err)
	}
	seen := make(map[string]struct{}, len(requests))
	for _, r := range requests {
		seen[r.Id] = struct{}{}
		// Cancelled requests never reflected an actual need at scale,
		// so they don't earn the scope a chip.
		if r.State == models.RequestState_REQUEST_STATE_CANCELLED {
			continue
		}
		cat := strings.TrimSpace(r.Category)
		counts.add(cat)
		if r.State == models.RequestState_REQUEST_STATE_FULFILLED {
			counts.add(cat)
		}
	}

	// Per-user: also count requests the target helped fulfill (and
	// didn't file). Same +1/+1 weighting as filing.
	if opts.Mode == ModePerUser {
		helped, err := loadHelpedScopedRequestsByUser(ctx, s, opts.OwnerID, opts, sharedSet)
		if err != nil {
			return fmt.Errorf("load helped requests: %w", err)
		}
		for id, r := range helped {
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			if r.State == models.RequestState_REQUEST_STATE_CANCELLED {
				continue
			}
			cat := strings.TrimSpace(r.Category)
			counts.add(cat)
			if r.State == models.RequestState_REQUEST_STATE_FULFILLED {
				counts.add(cat)
			}
		}
	}
	return nil
}

// loadHelpedScopedRequestsByUser returns requests in scope where
// [target] appears in confirmed_helper_ids. Since confirmed_helper_ids
// is a repeated field with no dedicated index, the storage layer
// loads every in-scope request and filters in memory.
//
// Query budget: 2 reads (community_request by community-in + request
// batch).
func loadHelpedScopedRequestsByUser(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	target string,
	opts Options,
	sharedSet map[string]struct{},
) (map[string]*models.Request, error) {
	if target == "" {
		return map[string]*models.Request{}, nil
	}
	pivots, err := storage.QueryByFieldIn[*models.CommunityRequest](
		s, ctx, "community_id", opts.CommunityIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("query community_request by community: %w", err)
	}
	idSet := make(map[string]struct{}, len(pivots))
	for _, cr := range pivots {
		if cr.Deleted != nil {
			continue
		}
		if _, ok := sharedSet[cr.CommunityId]; !ok {
			continue
		}
		idSet[cr.RequestId] = struct{}{}
	}
	if len(idSet) == 0 {
		return map[string]*models.Request{}, nil
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	byID, err := storage.GetByIDs[*models.Request](s, ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get requests by ids: %w", err)
	}
	out := make(map[string]*models.Request)
	for id, r := range byID {
		if r == nil || r.Deleted != nil {
			continue
		}
		for _, h := range r.ConfirmedHelperIds {
			if h == target {
				out[id] = r
				break
			}
		}
	}
	return out, nil
}

// loadScopedRequests returns the non-deleted requests shared into
// the requested community scope, regardless of state. The
// FULFILLED-state bonus is applied at the counting stage.
//
//   - Per-user: query Request by requester_id, pivot through
//     community_request to keep only ones shared into the scope.
//   - Per-community: pivot through community_request by community,
//     batch-fetch requests.
func loadScopedRequests(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	sharedSet map[string]struct{},
) (map[string]*models.Request, error) {
	return loadScoped(ctx, s, opts, sharedSet,
		scopedSpec[*models.Request, *models.CommunityRequest]{
			entity:     &models.Request{},
			pivot:      &models.CommunityRequest{},
			ownerField: "requester_id",
			pivotField: "request_id",
			pivotTable: "community_request",
			noun:       "request",
			plural:     "requests",
			entityID:   (*models.CommunityRequest).GetRequestId,
		})
}
