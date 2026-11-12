# Issue #1536: [testing] `impact_metrics/user_detail.go` (526 lines) and `user_metrics.go` lack tests

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1536
**Filed:** 2026-04-28 | **Last activity:** 2026-04-29
**Labels:** auto-filed, review:testing | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`server/services/impact_metrics/user_detail.go` (526 lines) and
`server/services/impact_metrics/user_metrics.go` (154 lines) carry
non-trivial aggregation logic that feeds the per-user profile screen
and the leaderboard. After this cycle's testing sweep these were the
only two non-trivial untested files in the package.

`service_test.go` already contains a small `TestGetUserImpactMetrics`
covering missing-`user_id` validation, the empty-activity case, and
the `is_own_profile` flag — three test cases against an RPC whose
response carries fifteen-plus assembled fields plus six private
helpers in `user_detail.go` (`userSourceBreakdowns`,
`userMoneyTrend`, `userCo2Trend`, `userQualityTimeTrend`,
`userDirectionBreakdowns`, `userCommunityRanking`, plus the
contributing `userRequestImpact`, `userExperienceImpact`,
`buildSourceBreakdown`). Every aggregation branch — giveaway vs.
loan totals, request-helper impact, experience-owner impact,
direction (Given/Received), monthly cumulative trends, the
ten-community ranking cap — is currently unverified. A regression
that, say, double-counts giveaway money or drops the helper-side
request impact would silently ship.

The fix is purely additive test code: no production-code changes,
no new public APIs.

## Context from Comments

- **2026-04-28 (maintainer, /ripls-triage):** Auto-classified as
  Task / P1, citing the file size of `user_detail.go` (526 LOC) and
  the link to Tier-3 priority #14 (testing).
- **2026-04-29 (maintainer):** `@claude` ping — instruction to
  begin work.

No disagreements or open requirements beyond the issue body.

## Potential Solutions

### Option A: Two RPC-level test files modeled exactly on `user_impact_detail_test.go`
- **Approach:** Add `user_metrics_test.go` and `user_detail_test.go`
  that exercise everything through the `GetUserImpactMetrics` RPC
  (the only public surface that touches these files). Existing
  `TestGetUserImpactMetrics` cases in `service_test.go` move to the
  new `user_metrics_test.go`, leaving `service_test.go` focused on
  community-level tests. Helpers live in `common_test.go` so
  experience/giveaway/return-time inserts are reusable by any future
  test in the package.
- **Pros:**
  - Mirrors the issue's literal recommendation ("modeled on
    `user_impact_detail_test.go`").
  - Black-box: tests survive private-helper renames or refactors of
    `userMoneyTrend` / `userSourceBreakdowns` etc. without churn.
  - Same `setupTestService` / `contextWithAuth` / `insertTest*`
    plumbing that the rest of the package already uses.
  - Catches integration bugs (e.g., a helper silently returning
    `nil` because the RPC call path skipped it).
- **Cons:**
  - Each test does a bit more setup than a unit test would
    (container, full RPC) — but the testcontainer is shared
    package-wide so the marginal cost is small.
  - Failure messages point at the response field, not the helper —
    we mitigate by keeping one failure per case.
- **Effort:** Medium (two files, ~300 lines total, plus 4 small
  helpers in `common_test.go`).

### Option B: Unit-test private helpers directly in `user_detail_test.go`, RPC-test in `user_metrics_test.go`
- **Approach:** Call `userSourceBreakdowns`, `userMoneyTrend`,
  `userCo2Trend`, `userQualityTimeTrend`, `userDirectionBreakdowns`,
  `userCommunityRanking`, and `buildSourceBreakdown` directly from
  `user_detail_test.go` (same package, so unexported access works).
  Keep RPC-level validation/auth/assembly tests in
  `user_metrics_test.go`.
- **Pros:**
  - Targeted failure messages.
  - Easier to enumerate edge cases for a single helper
    (e.g. >6 months window, >10 communities cap) without setting
    up a full RPC response.
- **Cons:**
  - Couples tests to private function signatures — any refactor
    that fuses `userMoneyTrend` and `userCo2Trend`, for instance,
    rewrites the test file.
  - Diverges from the pattern the issue cites
    (`user_impact_detail_test.go` is RPC-level).
  - `buildSourceBreakdown` is pure and worth one quick unit test,
    but the rest read storage and need the same DB scaffolding
    Option A uses.
- **Effort:** Medium-large (more cases, more wiring).

### Option C: Single combined `user_metrics_test.go`
- **Approach:** Put everything in one file.
- **Pros:** Less file-shuffling.
- **Cons:** Issue explicitly asks for two files mirroring
  `user_impact_detail_test.go`'s placement next to its production
  file. A single file would also balloon past the package's
  ~200-line-per-test-file norm.
- **Effort:** Small.

## Recommendation

**Option A.** It matches the issue's literal recommendation, the
package's existing test style (RPC-level via `setupTestService`),
and the file-per-production-file convention. The one concession
to Option B: keep a tiny pure-function test for
`buildSourceBreakdown` inside `user_detail_test.go` (no DB needed),
since it is the only fully pure helper and the percentage math is
worth a direct round-trip check.

## Open Questions

- **Move existing `TestGetUserImpactMetrics` from `service_test.go`
  to the new `user_metrics_test.go`, or leave it in place and add
  alongside?** — Recommendation: **move it.**
  `service_test.go` becomes the home for `Service`-construction
  and community-level tests; `user_metrics_test.go` owns
  user-level RPC tests, mirroring the
  `user_impact_detail.go` ↔ `user_impact_detail_test.go` pairing.
  The move is a `git mv`-style cut/paste, not a behavior change.

- **Do we need to assert exact percentage values in
  `SourceBreakdown` (e.g., 60% loan / 40% giveaway), or is
  presence-and-ordering enough?** — Recommendation: **assert exact
  percentages and exact ordering** for the multi-source happy path
  (one money case is sufficient). The percentage math in
  `buildSourceBreakdown` is the kind of code that silently drifts
  when a contributor adds a fifth source; one explicit assertion
  freezes the contract.

- **How do we exercise the >6-month window and >10-community cap
  without making test fixtures huge?** — Recommendation: build the
  fixtures inline in two dedicated subtests. For the 7-month case,
  insert seven completed transfers with `ActualReturnUnixSec`
  spread across distinct months (using a shared `time.Now()`
  baseline + month offsets). For the 11-community case, loop
  insertions. Both are <30 lines of setup each.

- **`testImpactEstimate` only fills `MoneySaved`,
  `EmissionsPrevented`, and `TimeSaved` — but `user_detail.go`
  reads `QualityTime`. Extend the helper or add a sibling?** —
  Recommendation: **add a sibling** `testImpactEstimateWithQT`
  that takes a fourth `qualityTimeMinutes` parameter and populates
  `QualityTimeEstimate.QualityTimeMinutes`. Keep the existing helper
  unchanged so unrelated tests stay terse.

- **Do we add coverage for the request-helper and
  experience-owner impact paths (`userRequestImpact`,
  `userExperienceImpact`)?** — Recommendation: **yes, one subtest
  each.** They are the source-breakdown branches most likely to
  silently return zero on a refactor (the experience path also
  uses the partial `manufacture_avoided_carbon` aggregation that
  loans use — worth a regression check).

## Audit notes

- Reviewed against `docs/server/architecture.md` (Testing Strategy,
  File Organization, Service Architecture). The plan adds only test
  files alongside their implementation files (matching the
  documented `[filename]_test.go` convention) and introduces no
  production-code changes, no new services, and no new shared
  libraries. The pure-function `buildSourceBreakdown` test honors
  the doc's "Unit tests are always preferable" guidance for the one
  helper that does not need DB scaffolding; the rest stay at
  RPC level to match the package's existing testcontainer pattern.
- No client-side architecture, caching, or i18n implications —
  plan is server-only.

## Related Issues

- #14 — Tier-3 testing priority that surfaced this gap.
- The companion file `user_impact_detail.go` already has
  `user_impact_detail_test.go`; this plan extends the same coverage
  pattern to the remaining two untested files.

## Implementation Plan

### Phase 1: Test scaffolding in `common_test.go`
**Goal:** Reusable fixtures for the new tests (and any future
user-impact tests) — added to `common_test.go`, not duplicated.
**Tasks:**
- [ ] Add `insertTestExperience(t, db, ownerID, ie *models.ImpactEstimate) string`
      that inserts a `models.Experience` with
      `State = EXPERIENCE_STATE_COMPLETED` and the supplied estimate.
- [ ] Add `insertGiveawayTransfer(t, db, communityID, ownerID, recipientID, gearID, ie) string`
      — same as `insertTestCompletedTransfer` but with
      `TransferType_TRANSFER_TYPE_GIVEAWAY`.
- [ ] Add `insertCompletedTransferAt(t, db, communityID, ownerID, recipientID, gearID, ie, completedAtUnixSec int64) string`
      — variant that lets the test set `ActualReturnUnixSec`
      explicitly, for trend-window tests.
- [ ] Add `testImpactEstimateWithQT(costUSD, carbonGrams, timeMinutes, qtMinutes float32) *models.ImpactEstimate`
      that wraps `testImpactEstimate` and additionally populates
      `QualityTimeEstimate.QualityTimeMinutes`.
**Validates:** Helpers compile and existing tests still green
(`go test ./server/services/impact_metrics/...`).

### Phase 2: `user_metrics_test.go`
**Goal:** Cover the RPC entry point: validation, auth-derived
fields, and the metrics-block plumbing in `getUserImpactMetrics`.
**Tasks:**
- [ ] Create `server/services/impact_metrics/user_metrics_test.go`.
- [ ] Move the three subtests of `TestGetUserImpactMetrics` from
      `service_test.go` into the new file (delete the test from
      `service_test.go`, leaving the community tests there).
- [ ] Add subtest: **`unauthenticated request returns response with
      is_own_profile=false`** — `getUserImpactMetrics` calls
      `auth.RequireAuth(ctx)` at `user_metrics.go:100` and
      discards the error, using the resulting `authUser != nil`
      check only to set `IsOwnProfile`. Pin that contract: call
      with `context.Background()` (no auth), assert no error and
      `resp.Msg.IsOwnProfile == false`. If a future change makes
      auth required, this test fails loudly and forces the
      decision into review.
- [ ] Add subtest: **`response carries user profile fields`** —
      insert a user with `Name`, `Description`,
      `MediaIds=[mediaID]`; assert
      `UserName`, `UserDescription`, `UserMediaId` populate.
- [ ] Add subtest: **`response includes location name when user has
      primary residence`** — insert a `Location` with `Name="SF"`,
      a user with `PrimaryResidenceLocationId` pointing at it;
      assert `UserLocationName == "SF"`.
- [ ] Add subtest: **`activity counts reflect inserted transfers`** —
      insert two completed loans owned by the user and one borrowed;
      assert `Metrics.TotalLoans == 2`, `Metrics.TotalBorrows == 1`.
**Validates:** `go test -run TestGetUserImpactMetrics ./server/services/impact_metrics/...` green.

### Phase 3: `user_detail_test.go` — source breakdowns + community ranking
**Goal:** Cover the dimension-aggregation helpers via the RPC,
plus a pure-function test for `buildSourceBreakdown`.
**Tasks:**
- [ ] Create `server/services/impact_metrics/user_detail_test.go`.
- [ ] Pure unit subtest **`TestBuildSourceBreakdown`**:
      - empty totals → returns `nil`
      - one source → returns single entry, 100% percentage
      - mixed (40, 60, 0, 0) → ordering "Loans","Giveaways", percentages 40/60.
- [ ] **`TestGetUserImpactMetrics_SourceBreakdownsViaRPC`** (subtests):
      - **`loan + giveaway split`**: insert one completed loan
        (money=100) and one completed giveaway (money=50); assert
        `SourceBreakdownMoney` has two entries with labels
        "Loans" (100, 66.7%) and "Giveaways" (50, 33.3%).
      - **`requests contribute via helper path`**: insert a
        fulfilled request with the user as `ConfirmedHelperIds[0]`,
        money=80; assert "Requests" entry appears with value 80.
      - **`experiences contribute via owner path`**: insert a
        completed experience with the user as owner, money=40;
        assert "Events" entry appears with value 40.
      - **`co2 breakdown sums manufacture + waste carbon`**:
        custom `ImpactEstimate` with both
        `ManufactureAvoidedCarbon` and `WasteReducedCarbon`;
        assert `SourceBreakdownCo2[0].Value` equals the sum.
- [ ] **`TestGetUserImpactMetrics_CommunityRanking`**:
      - **`empty memberships → empty ranking`**: assert
        `len(RankedCommunities) == 0`.
      - **`ranks by money saved descending`**: user is a member of
        three communities (A, B, C). Insert one completed transfer
        per community with money 30 / 90 / 60; assert ranking
        order B, C, A with `Rank` 1/2/3 and matching
        `CommunityName`.
      - **`includes zero-impact communities`**: user is a member
        of two communities; only one has activity. Assert both
        appear; the inactive one gets `FormattedValue` for $0.
      - **`caps at 10 communities`**: insert 11 memberships with
        descending impact; assert exactly 10 returned and the
        smallest one is dropped.
**Validates:** `go test -run TestBuildSourceBreakdown -run TestGetUserImpactMetrics_SourceBreakdownsViaRPC -run TestGetUserImpactMetrics_CommunityRanking ./server/services/impact_metrics/...` green.

### Phase 4: `user_detail_test.go` — trends + direction breakdown
**Goal:** Cover the time-series helpers and the
`UserDirectionBreakdown` aggregator.
**Tasks:**
- [ ] **`TestGetUserImpactMetrics_MoneyTrend`**:
      - **`empty trend when no completed transfers`** → nil.
      - **`single month → single point with cumulative value`**.
      - **`>6 months keeps last 6 cumulative`**: seven completed
        transfers, one per month over seven months; assert
        `len(MoneyTrend) == 6` and `MoneyTrend[5].Value` equals
        the cumulative of months 2-7. Use
        `insertCompletedTransferAt` with month offsets.
- [ ] **`TestGetUserImpactMetrics_Co2Trend`**:
      - assert sums `ManufactureAvoidedCarbon` and
        `WasteReducedCarbon` per month, cumulative across the
        last six months.
- [ ] **`TestGetUserImpactMetrics_QualityTimeTrend`**:
      - uses `testImpactEstimateWithQT`; asserts QT minutes
        accumulate correctly.
- [ ] **`TestGetUserImpactMetrics_DirectionBreakdown`**:
      - **`given + received both populate with em-dash for zero`**:
        user gives 1 loan (money=$50) and receives nothing →
        Given row has count "1 transaction", `FormattedValue=$50`;
        Received row has count "0 transactions", `FormattedValue="—"`
        (em-dash, U+2014).
      - **`given and received accumulate independently`**:
        user gives 2 loans (money $30 + $20), receives 1 loan
        (money $40); assert Given count="2 transactions"
        formatted="$50", Received count="1 transaction"
        formatted="$40".
      - **`pluralization at count == 1`**: count of 1 →
        "1 transaction" (singular).
**Validates:** `go test -run TestGetUserImpactMetrics_ ./server/services/impact_metrics/...` green; total package test runtime stays under 90 s on the dev box.

### Phase 5: Verification
**Goal:** Build green, no lint regressions, no integration-tag
breakage.
**Tasks:**
- [ ] `npm run build` — `golangci-lint` + `go build ./...` green.
- [ ] `go test ./server/services/impact_metrics/...` green.
- [ ] `go test -tags=integration ./server/services/impact_metrics/...`
      green (the package contains a `prompt_golden_test.go` behind
      the integration tag — confirm we did not break its
      compilation).
- [ ] Confirm no `.go` file in `server/services/impact_metrics/`
      crosses the soft 1000-line ceiling after additions.
**Validates:** Issue acceptance criteria met — both `user_detail.go`
and `user_metrics.go` have dedicated test files modeled on
`user_impact_detail_test.go`, covering authorization, request
validation, response assembly, and the no-activity / single-
transaction edge cases the issue calls out.
