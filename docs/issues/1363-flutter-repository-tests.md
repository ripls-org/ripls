# Issue #1363: [testing] 5 Flutter repositories besides auth_repository are untested

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1363
**Filed:** 2026-04-25 | **Last activity:** 2026-04-26
**Labels:** auto-filed, review:testing | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Five Flutter data-layer repositories lack test coverage: `impact_repository`,
`leaderboard_repository`, `portfolio_repository`, `search_repository`, and
`shadow_user_repository`. Repositories own caching, invalidation, and API
mapping; untested repositories can silently return stale or corrupted data.

On inspection, `impact_repository_test.dart` already exists but covers only the
two draft methods (draftImpact/redraftImpact). It is missing tests for all ten
cached methods and the invalidation/refresh methods. The other four repositories
have no test file at all.

## Context from Comments

A single comment (`@claude`) from a collaborator assigns this to the Claude
agent. No additional design direction is provided; the issue body is the
authoritative spec.

## Potential Solutions

### Option A: Expand impact test + add four new test files
- **Approach:** Add cached-method tests to the existing `impact_repository_test.dart`
  and create `leaderboard_repository_test.dart`, `portfolio_repository_test.dart`,
  `search_repository_test.dart`, and `shadow_user_repository_test.dart`. Each file
  follows the Mockito pattern (StashCacheManager + `@GenerateMocks`), covering
  cache hit/miss, invalidation, callbacks, and error paths.
- **Pros:** Minimal, targeted. Matches the pattern already established in the
  codebase. After `build_runner` regenerates mocks the tests are immediately
  runnable.
- **Cons:** Requires running `flutter pub run build_runner build` to generate mock
  classes. The generated `.mocks.dart` files must be committed alongside the test
  files.
- **Effort:** Medium (5 test files, ~25–40 test cases total).

### Option B: Add integration-level tests using a fake service instead of mocks
- **Approach:** Implement a `FakeXxxService` for each service and write tests
  against it instead of Mockito mocks.
- **Pros:** Slightly more realistic, less boilerplate per test.
- **Cons:** Diverges from the existing Mockito pattern used in every other
  repository test in the codebase. Higher initial cost (fake implementation for
  each service). Not what the issue recommends.
- **Effort:** Large.

### Option C: Table-driven test helper
- **Approach:** Build a generic `RepositoryTestHarness` that generates common
  cache-hit/miss/error tests parameterically.
- **Pros:** Less test code overall.
- **Cons:** Over-engineering for five files. The existing pattern is already
  clear and readable.
- **Effort:** Large (helper + five wire-ups).

## Recommendation

**Option A.** It is the minimal, targeted fix, directly follows the existing
pattern, and is exactly what the issue recommends (`"Follow the Mockito pattern
in user_repository_test.dart / gear_repository_test.dart"`).

## Open Questions

- **Should mock files be committed or gitignored?**
  Recommendation: commit them. All existing mock files are committed
  (`*_test.mocks.dart` present in repo). Keep consistent.

- **How deeply should impact caching tests be extended?**
  Recommendation: cover the top-priority cached methods (getMetrics,
  getUserMetrics, getMetricDetail, getCommunityUtilization, getCommunityLeaderboard)
  plus invalidation/refresh. Lower-priority getters (getTimeToSolveDetail,
  getCommunityPercentileDetail, getUserCommunityImpactDetail, getCommunityActions)
  can get lightweight smoke tests.

- **Does LeaderboardRepository use CacheManager.clear(pattern:) directly?**
  Yes. The test can verify invalidation by calling `invalidateLeaderboard()` and
  confirming the service is called again on the next `searchCommunities()` call.

## Audit Findings — architectural review

_Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`._

**✅ Plan follows architecture correctly:**
- Uses `StashCacheManager` + Mockito service mocks — the prescribed repository test
  pattern (see `caching.md` §Testing with Caching).
- Tests are scoped to the repository layer; no ViewModel, widget, or provider
  machinery needed.
- Mock files will be committed alongside test files, matching all existing tests.

**🔴 Bug found — must fix in Phase 1:**

`ImpactMetricsRepository.invalidateMetrics` invalidates key
`community:{id}:metrics`, but `getMetricsResponse()` stores under
`community:{id}:metrics_response`. These two strings are different; the
invalidation key is a dead key that has no corresponding cached entry.

Consequence: `refreshMetrics()` — which calls `invalidateMetrics()` then
`getMetrics()` — never actually clears the cache. The second call returns the
cached response unchanged. The same bug affects `invalidateAll()` (which calls
`invalidateMetrics()`).

Fix: change `invalidateMetrics` to invalidate `community:$communityId:metrics_response`.
This is a one-line fix to `impact_repository.dart` that must land before the
`refreshMetrics` test can be written correctly.

## Implementation Plan

### Phase 1: Fix ImpactMetricsRepository bug + expand test file
**Goal:** Repair the dead invalidation key and cover all cached/invalidation methods.
**Tasks:**
- [ ] Fix `impact_repository.dart`: change `invalidateMetrics` to invalidate
  `community:$communityId:metrics_response` (was `metrics` — dead key that never matched)
- [ ] Add group 'ImpactMetricsRepository — cached methods' to existing test file
- [ ] Test getMetrics cache hit (two calls → service called once)
- [ ] Test getUserMetrics cache hit
- [ ] Test getMetricDetail cache hit, keyed on dimension+period
- [ ] Test getCommunityUtilization cache hit
- [ ] Test getCommunityLeaderboard cache hit, keyed on communityId+dimension
- [ ] Test refreshMetrics: invalidate + re-fetch (service called twice)
- [ ] Test invalidateUserMetrics: second call re-fetches from service
- [ ] Test error path: service throws → exception propagates
- [ ] Run `flutter pub run build_runner build` (mock already exists; ensure clean)

**Validates:** All cached read-paths in ImpactMetricsRepository are exercised; the dead-key bug is caught by the refreshMetrics test.

### Phase 2: LeaderboardRepository tests
**Goal:** Cover searchCommunities caching, invalidation, and callback.
**Tasks:**
- [ ] Create `leaderboard_repository_test.dart` with `@GenerateMocks([LeaderboardService])`
- [ ] Test searchCommunities returns mapped CommunityLeaderboardItem list
- [ ] Test searchCommunities cache hit (two calls → service called once)
- [ ] Test searchCommunities uses different cache keys for different bounds/sortBy
- [ ] Test invalidateLeaderboard clears cache so next call re-fetches
- [ ] Test onCacheInvalidated callback is called during invalidation
- [ ] Run build_runner and commit generated mock file

**Validates:** Cache key construction, invalidation, and callback wiring.

### Phase 3: PortfolioRepository tests
**Goal:** Cover inbox/metrics caching, weekly goals mutation, and watch callbacks.
**Tasks:**
- [ ] Create `portfolio_repository_test.dart` with `@GenerateMocks([PortfolioService])`
- [ ] Test getInboxView cache hit
- [ ] Test getPortfolioMetrics cache hit; verify community ID list is sorted in key
- [ ] Test getInboxWeek cache hit, keyed by weekOffset
- [ ] Test getInboxFeed cache hit
- [ ] Test setWeeklyGoals invalidates inbox_week:0 cache
- [ ] Test watchItem: calls service, invalidates inbox_feed, fires onWatchMutated
- [ ] Test dismissInboxItem: calls service, invalidates inbox_feed, fires onWatchMutated
- [ ] Test markInboxItemRead invalidates inbox_feed
- [ ] Test getPortfolioMetricDetail cache hit; community IDs sorted in key
- [ ] Run build_runner and commit generated mock file

**Validates:** Mutation → invalidation → callback chain.

### Phase 4: SearchRepository tests
**Goal:** Cover search caching, community-scoped invalidation, and callback.
**Tasks:**
- [ ] Create `search_repository_test.dart` with `@GenerateMocks([SearchService])`
- [ ] Test search returns result list
- [ ] Test search cache hit (same params → service called once)
- [ ] Test search cache miss for different query params
- [ ] Test invalidateSearches clears cache so next call re-fetches
- [ ] Test invalidateSearchesForCommunity clears only matching keys
- [ ] Test onCacheInvalidated callback fires during invalidation
- [ ] Test invalidateAll delegates to invalidateSearches
- [ ] Run build_runner and commit generated mock file

**Validates:** Cache key isolation between different queries.

### Phase 5: ShadowUserRepository tests
**Goal:** Cover listShadowUsers caching, searchShadowUsers bypass, createShadowUser invalidation.
**Tasks:**
- [ ] Create `shadow_user_repository_test.dart` with `@GenerateMocks([CommunityService])`
- [ ] Test listShadowUsers cache hit
- [ ] Test searchShadowUsers bypasses cache (two calls → service called twice)
- [ ] Test createShadowUser calls service and invalidates list cache
- [ ] Test getActivities bypasses cache (two calls → service called twice)
- [ ] Run build_runner and commit generated mock file

**Validates:** Only the list is cached; ephemeral queries and activities are not.

## Related Issues

- Auto-filed from `docs/reviews/reports/testing_review.md` — no sibling issues directly referenced.
