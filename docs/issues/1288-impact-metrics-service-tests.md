# Issue 1288: Impact Metrics RPC Service Has Zero Test Files

## Problem Statement

The `server/services/impact_metrics/` package contains 11 source files (9 RPC
handlers plus helpers) and zero `_test.go` files. This gap creates two concrete
risks:

1. **Authorization leaks** — six of the nine RPCs gate on `auth.RequireAuth`.
   Without tests, a refactor could silently strip the check and expose
   community-private metrics to unauthenticated callers.
2. **Aggregation bugs** — percentile, leaderboard, time-to-solve, and
   utilization calculations involve non-trivial math. Regressions in those
   functions are undetectable without unit tests.

## Context from Comments

The triage comment (2026-04-24) confirms P2 priority with the same two-risk
framing. No additional context beyond the issue body.

## Potential Solutions

### Option A — Single large `service_test.go`

Write one file with all table-driven tests covering all nine RPCs. Simple to
navigate, but the file grows large (> 800 lines) quickly.

### Option B — One file per RPC handler (e.g., `actions_test.go`, `leaderboard_test.go`, …)

Mirrors the per-file structure of the source package. Files stay focused, each
under ~200 lines, and failures are easy to pin to a specific handler. Slightly
more boilerplate for shared helpers.

### Option C — Shared helpers in `common_test.go`, per-handler test files

Combines B's organization with a dedicated helpers file. This is the pattern
used by the `chat` and `community` sibling services and is consistent with
CLAUDE.md's guideline that "files should be short (preferably < 1000 lines)
and focused on related functionality."

## Recommendation

**Option C.** Mirrors what `server/services/chat/` already does:
`common_test.go` for setup/helpers, then one test file per handler group. Keeps
each file focused, stays under 1000 lines, and is immediately familiar to any
contributor who has read the sibling services.

## Open Questions

1. **Should `GetCommunityMetricDetail` be tested end-to-end or just for
   validation?**  
   Recommended answer: validation only (missing community_id, missing
   dimension, missing period), plus a smoke test that the RPC returns without
   error for a real empty community. The calculator itself is already tested in
   `server/impact_metrics/metric_detail_calculator_test.go`.

2. **Should tests create real users/communities or use raw UUID strings as IDs?**  
   Recommended answer: insert real `models.User` and `models.Community` rows
   via `storage.Insert` so foreign-key constraints are satisfied (same pattern
   as `calculator_test.go`).

3. **Which RPCs are highest priority for auth coverage?**  
   Recommended answer: `GetCommunityActions`, `GetCommunityUtilization`,
   `GetTimeToSolveDetail`, `GetCommunityLeaderboard`,
   `GetUserCommunityImpactDetail`, `GetCommunityPercentileDetail`. All six
   call `auth.RequireAuth` unconditionally.

## Implementation Plan

### Files to create

| File | Contents |
|------|----------|
| `server/services/impact_metrics/common_test.go` | `setupTestService`, `contextWithAuth`, `insertTestUser`, `insertTestCommunity`, `insertTestMembership`, `insertTestCompletedTransfer`, `insertTestFulfilledRequest` |
| `server/services/impact_metrics/service_test.go` | `TestGetCommunityImpactMetrics`, `TestGetUserImpactMetrics`, `TestGetCommunityMetricDetail` |
| `server/services/impact_metrics/actions_test.go` | `TestGetCommunityActions` — auth, validation, pagination, multiple action types |
| `server/services/impact_metrics/leaderboard_test.go` | `TestGetCommunityLeaderboard` — auth, validation, ranking, calling-user rank injection |
| `server/services/impact_metrics/time_to_solve_test.go` | `TestGetTimeToSolveDetail` — auth, validation, median computation |
| `server/services/impact_metrics/utilization_test.go` | `TestGetCommunityUtilization` — auth, validation, basic utilization with gear + loans |
| `server/services/impact_metrics/percentile_test.go` | `TestGetCommunityPercentileDetail` — auth, validation, percentile math |

### Test categories per RPC

For each RPC, write at minimum:
- **Validation**: missing required field → `CodeInvalidArgument`
- **Auth guard** (where applicable): unauthenticated context → error
- **Empty community/user**: no data → zero-value response without error
- **Happy path**: insert minimal test data, verify key response fields

### Steps

1. Generate code (`npm run generate`) — no proto changes needed.
2. Create `common_test.go` with shared helpers.
3. Create test files one at a time, running `go test ./server/services/impact_metrics/...` after each.
4. Run `npm run build` to confirm lint passes.
5. Run full suite `go test -tags=integration -timeout 20m ./...` for CI parity.

## Audit Notes

*Reviewed against `docs/server/architecture.md`, `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`.*

- **File layout**: Proposed `common_test.go` + per-feature `*_test.go` files matches the Standard Service Package Layout in `docs/server/architecture.md` exactly.
- **Real storage**: Using `storage.SetupTestStorage(t)` with testcontainers is the established pattern for this repository (see `chat/`, `community/`, `impact_metrics/calculator_test.go`); this is correct despite the architecture doc's "prefer mocks" aspiration.
- **Auth coverage**: All six auth-gated RPCs (`GetCommunityActions`, `GetCommunityUtilization`, `GetTimeToSolveDetail`, `GetCommunityLeaderboard`, `GetUserCommunityImpactDetail`, `GetCommunityPercentileDetail`) are covered.
- **Client docs not applicable**: This plan touches only Go server test files; Flutter client patterns (caching, i18n) are irrelevant.
- **No blocking findings.**

## Related Issues

- `server/impact_metrics/calculator_test.go` — existing unit tests for the
  calculator layer (good reference for test helpers).
- Issue #840 — patrol loan tests (same testing-gap category, already fixed).
- Testing review report: `docs/reviews/reports/testing_review.md`.
