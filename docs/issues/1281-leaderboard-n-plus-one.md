# Issue #1281: [performance] N+1 in GetCommunityLeaderboard — community-wide queries per member

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1281
**Filed:** 2026-04-25 | **Last activity:** 2026-04-25
**Labels:** auto-filed, review:performance | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`GetCommunityLeaderboard` iterates over every community member and calls
`Calculator.CalculateUserImpactInCommunity` once per member. That function
issues two community-wide SQL queries per call:

1. `SELECT * FROM Transfer WHERE community_id = ?`
2. `SELECT * FROM CommunityRequest WHERE community_id = ?` (plus a batch
   `GetByIDs` for the requests themselves)

For a 50-member community this produces ~100 full-table scans against the same
tables with the same predicate — a classic N+1. Latency scales linearly with
member count and will become user-visible as communities grow.

The fix is to hoist those three queries out of the loop, load the data once at
the leaderboard level, and change `CalculateUserImpactInCommunity` so it can
compute entirely in memory when given pre-loaded slices.

## Context from Comments

- **Triage (2026-04-24, maintainer):** Confirmed P2 Bug. Engineering lens
  confirms N+1 in a user-facing RPC path. 100 full-table scans for a 50-member
  community.
- No additional technical discussion or alternative approaches raised.

## Potential Solutions

### Option A: Add a data-layer method that accepts pre-loaded slices

Add `CalculateUserImpactFromPreloadedData(ctx, userID, transfers, requestMap, dimension)` to `Calculator`. The `transfers` slice and `requestMap` are loaded once at the call site; the method operates entirely in memory. The existing `CalculateUserImpactInCommunity` keeps its current signature (backward-compatible). In `leaderboard.go`, fetch data once then call the in-memory variant in the loop.

_Note: `communityRequests []*models.CommunityRequest` is not passed to the new method — the caller resolves the community request IDs and batch-fetches the `requestMap` before calling the in-memory method._

- **Pros:** Preserves backward compatibility; keeps the per-user computation logic in one place; easy to unit-test without a DB; follows the "push filtering to SQL" / "never query in a loop" rules from CLAUDE.md.
- **Cons:** Slightly more surface area — two methods instead of one for the same computation.
- **Effort:** Small.

### Option B: Add a batch method `CalculateAllUsersImpactInCommunity`

Add a single method that accepts a `[]string` of user IDs plus the community ID and dimension, fetches data once, and returns a `map[string]float64`. Remove the per-user call from `leaderboard.go` entirely.

- **Pros:** Single-call API from the leaderboard side; slightly less code in the service layer.
- **Cons:** The community-wide query is hidden inside Calculator, making it harder to compose with other queries done at the leaderboard level (e.g. the existing member and user-profile fetches). Harder to test the per-user computation in isolation.
- **Effort:** Small.

### Option C: Push community transfer+request filtering to SQL with user-specific WHERE

Rewrite the storage queries to filter by `community_id AND (owner_id = ? OR recipient_id = ?)` for transfers and similarly for requests, and run those per-user. This keeps the per-user call pattern but cuts the data returned per query significantly.

- **Pros:** Minimal code change.
- **Cons:** Still N+1 queries — just smaller result sets. Does not eliminate the fundamental problem. Index coverage for compound predicates may not exist today.
- **Effort:** Small, but incomplete fix.

## Recommendation

**Option A** — pre-load data once in `leaderboard.go`, add an in-memory
`CalculateUserImpactFromPreloadedData` method to `Calculator`.

This is the most targeted fix. It eliminates all three repeated community-wide
queries (transfers, community requests, request fetch) in a single change, keeps
per-user logic testable without a database, and aligns with CLAUDE.md's "Never
query in a loop" and "Batch writes" guidance. Option B is fine but hides the
data-loading inside a wider method that's harder to compose; Option C is not a
real fix.

## Open Questions

- **Should `CalculateUserImpactInCommunity` be kept, deprecated, or removed?**
  Recommendation: Keep it. Nothing else calls it today (it was only called from
  the leaderboard loop), but removing it would be a breaking change to the
  Calculator public API without a compelling reason. Deprecate with a comment
  if desired, or leave it unchanged.

- **Should the pre-loaded request map be built with the full community's
  request IDs or only the fulfilled subset?**
  Recommendation: Fetch all request objects once (all IDs from
  `CommunityRequest`) to match the current behavior, then filter to
  `FULFILLED` in memory. Filtering before the batch fetch would save some I/O
  but requires joining CommunityRequest with Request state at the SQL layer,
  adding complexity. Keep it simple.

## Implementation Plan

### Phase 1: Add in-memory computation method to Calculator

**Goal:** Introduce `CalculateUserImpactFromPreloadedData` so the per-user
leaderboard calculation can operate on slices loaded once.

**Tasks:**
- [ ] Add `CalculateUserImpactFromPreloadedData(ctx context.Context, userID string, transfers []*models.Transfer, requestMap map[string]*models.Request, dimension api.ImpactMetricDimension) float64` to `server/impact_metrics/calculator.go`.
  - Extract the filtering/summation logic from the existing `CalculateUserImpactInCommunity` body.
  - Returns `float64` directly (no error — pure in-memory computation, no I/O).
- [ ] Add unit tests in `calculator_test.go` covering: user with matching transfers, user with matching requests, user with both, user with no matches, deleted/non-completed records excluded.

### Phase 2: Hoist queries out of the leaderboard loop

**Goal:** Reduce 3N DB queries in `getCommunityLeaderboard` to 3 queries total.

**Tasks:**
- [ ] In `server/services/impact_metrics/leaderboard.go`, before the member loop:
  - Fetch all transfers: `storage.QueryByField[*models.Transfer](store, ctx, "community_id", communityID)`
  - Fetch all community requests: `storage.QueryByField[*models.CommunityRequest](store, ctx, "community_id", communityID)`
  - Collect request IDs and batch-fetch requests: `storage.GetByIDs[*models.Request](...)`
- [ ] Replace the `calc.CalculateUserImpactInCommunity(...)` call in the loop with `impactlib.CalculateUserImpactFromPreloadedData(ctx, membership.UserId, transfers, requestMap, req.Msg.Dimension)`.
- [ ] Remove the per-iteration `impactlib.NewCalculator(store, cfg)` call (create the calculator once outside the loop).
- [ ] Add a test in `server/services/impact_metrics/` covering multi-member leaderboard computation with preloaded data, asserting correct ranking.

### Phase 3: Verify and ship

**Goal:** Confirm no regressions.

**Tasks:**
- [ ] Run `npm run build` (lint + compile).
- [ ] Run `go test -tags=integration -timeout 20m ./server/impact_metrics/... ./server/services/impact_metrics/...`.
- [ ] Commit, push, open PR.

## Related Issues

- #1096 — DB connection exhaustion under load (related performance concern; leaderboard N+1 would contribute to connection pressure at scale).
