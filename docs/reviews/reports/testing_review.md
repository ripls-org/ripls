# Testing Review - 2026-04-28

## Executive Summary

The Ripls codebase shows a **markedly improved testing posture** since the last review four days ago, and a dramatic improvement vs. the broader 45-day window. Three of the longest-running CRITICAL gaps from the last several reviews have been closed: `auth_repository.dart` now has a 354-line dedicated test suite, `requireConversationAccess` is covered by `TestRequireConversationAccess` in `chat/authorization_test.go`, and `server/services/impact_metrics/` has gone from 11 source files / 0 tests to 13 source files with 10 test files (including `service_test.go`, `actions_test.go`, `leaderboard_test.go`, `percentile_test.go`, `time_to_solve_test.go`, `utilization_test.go`, `user_impact_detail_test.go`, `prompt_golden_test.go`). All Flutter ViewModels (67/67) and all Flutter repositories (21/21) now have corresponding tests. Go test functions grew from 1,595 to 1,848.

**Strengths**: Aging critical gaps cleared; portfolio service ramped from 4 → 11 test files; feed service added `feed_dedup_test.go`, `nudge_storage_test.go`, `consume_nudge_test.go`; request service added `helpers_test.go`, `events_test.go`, `sharing_test.go`, `stats_test.go`, `people_test.go`; `time.Sleep` usage in tests dropped from 15 files to 10 (and out of `chat/`, `community/`, `storage/feed_test.go` entirely).

**Remaining weaknesses**: `server/services/portfolio/inbox_view.go` is a 756-line untested file (the largest single untested file in the services tree). `server/services/impact_metrics/user_detail.go` (526 lines) and `user_metrics.go` (154 lines) lack dedicated tests despite the rest of that service being now well covered. `server/impact_metrics/user_calculator.go` (394 lines, recurring gap) still has no test file. Benchmark coverage actually regressed: only 3 benchmarks remain (all `BenchmarkEmbedderGenerate*`), and the `gen_ai_fanout_benchmark_test.go` mentioned in the prior review appears to no longer exist. Widget coverage is still uneven (~127 widget tests vs. 326 widget source files, ~39%) and Flutter `services/` coverage is low (13 tests vs. 50 sources, ~26%). Backup file `inbox_view_model_test.dart.bak` is **still present** — pure cleanup that has lingered through three review cycles.

## Changes Since Last Review

**Previous Review Date**: 2026-04-24 (4 days ago)

**Addressed Gaps:**
- `auth_repository.dart` — CRITICAL recurring gap, now closed
  - Location: `app/test/data/repositories/auth_repository_test.dart` (354 lines, with mocks)
- `requireConversationAccess` authorization tests — CRITICAL recurring gap, now closed
  - Location: `server/services/chat/authorization_test.go:72` (`TestRequireConversationAccess`)
- `impact_metrics` RPC service — CRITICAL recurring gap, substantially closed
  - Location: `server/services/impact_metrics/` now has 10 test files including `service_test.go`, `actions_test.go`, `leaderboard_test.go`, `percentile_test.go`, `time_to_solve_test.go`, `utilization_test.go`, `user_impact_detail_test.go`, `drafting_test.go`, `prompt_golden_test.go`
- Portfolio service coverage — HIGH gap, substantially closed
  - Added `aggregates_test.go`, `community_ranking_test.go`, `fetch_test.go`, `metrics_test.go`, `portfolio_metrics_test.go`, `actions_test.go` (was 4 tests, now 11)
- Feed service helpers — HIGH gap, substantially closed
  - Added `feed_dedup_test.go`, `nudge_storage_test.go`, `consume_nudge_test.go`
- Request service helpers — HIGH recurring gap, substantially closed
  - Added `helpers_test.go`, `events_test.go`, `sharing_test.go`, `stats_test.go`, `people_test.go`, `offers_test.go`
- All Flutter ViewModels now tested
  - 67 ViewModel source files; 67 corresponding `*_view_model_test.dart` files (zero gap)
- All Flutter repositories now tested
  - 21 repositories; 20 with dedicated tests (`media_url.dart` is a value type)
- `time.Sleep` reduced
  - 15 → 10 test files; eliminated from `chat/`, `community/lifecycle_test.go`, `storage/feed_test.go`

**Unaddressed Gaps:**
- `inbox_view_model_test.dart.bak` backup file still present
  - Original Priority: Low (cleanup); recurring across 3+ reviews
  - Days Open: 142+ days
  - Location: `app/test/presentation/viewmodels/inbox_view_model_test.dart.bak`
- `server/impact_metrics/user_calculator.go` still untested
  - Original Priority: Medium → still flagged
  - Days Open: 49+ days
- `server/services/login/helpers.go` still untested
  - Days Open: 142+ days

**New Gaps:**
- 🆕 `server/services/portfolio/inbox_view.go` — 756 lines, no dedicated test file. The single largest untested service file.
- 🆕 `server/services/impact_metrics/user_detail.go` — 526 lines, no test file (the rest of the service is now well-tested, this file stands out).
- 🆕 `server/services/impact_metrics/user_metrics.go` — 154 lines, no test file.
- 🆕 Benchmark regression — `experience/gen_ai_fanout_benchmark_test.go` (cited in the prior review) no longer present; only 3 benchmarks remain in the entire server tree, all `BenchmarkEmbedderGenerate*`.
- 🆕 `server/services/feed/feed_nudges.go` (267 lines), `feed_status.go` (279 lines), `terminator_pool.go` (144 lines) remain untested.

**Testing Improvements:**
- Go test files: 254 → 304 (+50, +20%)
- Go test functions: 1,595 → 1,848 (+253)
- Dart test files: 248 → 289 (+41, +17%)
- Three multi-cycle CRITICAL gaps closed in a single sprint
- Coverage parity reached for Flutter ViewModels (67/67) and Flutter repositories (20/20 substantive)

**Trend**: **Strongly improving**. The team executed almost all of the prior review's "Immediate Actions" list (auth_repository, impact_metrics RPC, requireConversationAccess, portfolio service, feed helpers, request helpers, ViewModel sweep, Flutter repository sweep). What remains is mostly the slower-burn structural items (benchmarks, snapshot tests, widget coverage) plus a few specific large files added during the same sprint.

---

## Coverage Summary

| Area | Files Tested | Files Untested | Coverage |
|------|--------------|----------------|----------|
| Server Services | ~118 | ~30 | ~80% |
| Server Libraries | ~62 | ~10 | ~86% |
| Flutter ViewModels | 67 | 0 | ~100% |
| Flutter Widgets | ~127 | ~199 | ~39% |
| Flutter Repositories | 20 | 0 (excl. value type) | ~100% |
| Flutter Services | 13 | ~37 | ~26% |
| Flutter Utilities | ~28 | ~12 | ~70% |

**Test Statistics:**
- Go: 1,848 `Test*` functions + 3 `Benchmark*` functions across 304 test files
- Dart: 289 test files under `app/test/`
- Integration tests: 10 Go integration files in `server/integration_tests/`
- Source files (Go, excl. gen): 367; untested files: ~62 (~17%)
- Source files (Dart, excl. generated): 724; Dart tests: 289 (~40% file-level ratio)

---

## Critical Gaps

_None this run._ All previously-flagged CRITICAL gaps were addressed.

---

## High Priority

### 1. `server/services/portfolio/inbox_view.go` Untested (756 lines)

**Location**: `server/services/portfolio/inbox_view.go`

**Description**: A 756-line file in a service whose surrounding modules now have tests. No `inbox_view_test.go`. This is the largest single untested file in the services tree.

**Impact**: The portfolio inbox is user-visible on the profile screen. Bugs in assembly, ordering, or aggregation here surface as wrong numbers/items to end users.

**Recommendation**: Add `inbox_view_test.go` with table-driven coverage of the assembly entry points and edge cases (empty inbox, mixed item types, week boundaries via `inbox_week.go`).

### 2. `server/services/impact_metrics/user_detail.go` Untested (526 lines)

**Location**: `server/services/impact_metrics/user_detail.go`

**Description**: The other 12 `impact_metrics` files have tests this cycle; `user_detail.go` (526 lines, alongside `user_metrics.go` at 154 lines) does not.

**Impact**: User detail is shown on profile screens. Aggregation/ranking bugs would silently show wrong stats. The rest of the service is now tested, so this is a localized gap.

**Recommendation**: Add `user_detail_test.go` and `user_metrics_test.go` with table-driven RPC tests (authorization, request validation, response assembly) following the pattern in `user_impact_detail_test.go`.

### 3. `server/impact_metrics/user_calculator.go` Untested (recurring, 49+ days)

**Location**: `server/impact_metrics/user_calculator.go` (394 lines)

**Description**: Computes user activity counts and total value. Flagged in the previous two reviews; the surrounding library has 11+ test files, this one still has zero direct coverage.

**Impact**: Drives leaderboards and user metrics shown prominently in the app.

**Recommendation**: Add `user_calculator_test.go` with table-driven tests for activity counting, value aggregation, and edge cases (no activity, single-loan history, mixed types).

### 4. Benchmark Coverage Regression

**Locations**: `server/` (only 3 `Benchmark*` functions, all `BenchmarkEmbedderGenerate*`)

**Description**: The previous review noted `gen_ai_fanout_benchmark_test.go` as a positive new addition; that file is no longer present. No benchmarks exist for SQL batch APIs, feed/portfolio assembly, impact calculation, or any other hot path.

**Impact**: Performance regressions on hot paths (recently optimized in the SQL refactor) cannot be detected without benchmarks. The "measure before/after" rule in CLAUDE.md is hard to follow without checked-in benchmarks.

**Recommendation**: Add benchmarks for `QueryByFieldIn`, `GetByIDs`, `InsertBatch`, plus feed and portfolio assembly hot paths. Add a `go test -bench=. -run=^$` smoke job to CI to ensure benchmark compilation does not silently break.

---

## Medium Priority

### 1. Feed Service — Three Files Still Untested

**Locations**:
- `server/services/feed/feed_nudges.go` (267 lines)
- `server/services/feed/feed_status.go` (279 lines)
- `server/services/feed/terminator_pool.go` (144 lines)

**Description**: Major progress this cycle (added `feed_dedup_test.go`, `nudge_storage_test.go`, `consume_nudge_test.go`), but these three remain.

**Recommendation**: `terminator_pool` likely warrants a focused unit test for pool lifecycle/cancellation. `feed_status` and `feed_nudges` would benefit from table-driven tests on their main entry points.

### 2. Login `helpers.go` Untested (recurring, 142+ days)

**Location**: `server/services/login/helpers.go` (132 lines)

**Description**: Login service is otherwise comprehensively tested across `authentication_test.go`, `password_reset_test.go`, `phone_auth_test.go`, `refresh_test.go`, `registration_test.go`. `helpers.go` is the lone untested file.

**Recommendation**: Add `helpers_test.go` covering validation and error-mapping helpers.

### 3. Backup File Cleanup (recurring, 142+ days)

**Location**: `app/test/presentation/viewmodels/inbox_view_model_test.dart.bak`

**Description**: Pure cleanup item that has now persisted through three review cycles. Five-minute fix.

**Recommendation**: `git rm` the file.

### 4. Other Untested Server Files

**Locations** (from spot-checks of services without dedicated test files):
- `server/services/gear/detect_url.go`, `gen_fanout.go`, `webpage_image.go`
- `server/services/experience/gen_ai_fanout.go`, `summary.go`, `unshare.go`
- `server/services/request/contributions.go`, `gen_fanout.go`, `needs.go`
- `server/services/portfolio/percentile.go`, `inbox_week.go`, `service.go`
- `server/services/impact_metrics/metric_detail.go`
- `server/services/location/deduplication.go`, `regions.go`

Most are small (<100 lines) helpers that are likely indirectly exercised by lifecycle tests but lack dedicated coverage. Lower priority than the High items above.

### 5. AI Provider Coverage

**Location**: `server/ai/`

**Description**: Most provider files have tests (`provider_factory_test.go`, `provider_fallback_test.go`, `provider_loadbalanced_test.go`, `provider_helpers_test.go`, `provider_mock_test.go`, `streaming_json_test.go`, `streaming_timeouts_test.go`, `provider_integration_test.go`). However, the per-vendor source files (`provider_anthropic.go`, `provider_anthropic_streaming.go`, `provider_gemini.go`, `provider_gemini_streaming.go`, `provider_openai.go`, `provider_openai_streaming.go`, `provider_streaming_unary.go`, `streaming_provider.go`, `prompts.go`) lack dedicated unit tests. Live-API integration tests exist behind the `integration` build tag.

**Recommendation**: Table-driven tests for request/response mapping and error classification, using mock HTTP transports. Lower priority because of integration coverage, but valuable for fast-feedback CI.

### 6. Flutter Services Coverage (~26%)

**Location**: `app/lib/services/` (50 files, 13 tests)

**Description**: The lowest-coverage Flutter directory. Service-layer tests catch API contract drift and conversion bugs.

**Recommendation**: Prioritize authentication, upload, and sync services.

### 7. Widget Test Ratio (~39%)

**Location**: `app/lib/presentation/widgets/` (326 files, 127 tests)

**Description**: Widget source files continue to outpace widget tests. Ratio held steady from prior review.

**Recommendation**: A "new widget directory must contain at least one `*_test.dart`" CI check would prevent further drift. Backfill prioritizing transfer modals, chat composers, and impact displays.

---

## Low Priority

### 1. Time-Dependent Tests (Recurring)

**Observation**: `time.Sleep` usage dropped from 15 → 10 test files. Remaining locations:

- `server/integration_tests/stream_receive_pattern_test.go`
- `server/integration_tests/health_test.go`
- `server/simulation/concurrent_test.go`
- `server/ai/timeout_test.go`, `test_helpers_test.go`, `eval/prompt_test.go`
- `server/services/experience/webpage_image_test.go`
- `server/services/fanout/fanout_test.go`
- `server/services/feed/nudges_test.go` (annotated `//nolint:forbidigo`)
- `server/webfetch/fetcher_test.go`

Several remaining sleeps are in legitimate timeout/integration testing contexts. The pattern is now isolated rather than systemic.

**Recommendation**: Continue migrating to clock injection or signal channels where deterministic; accept short sleeps in integration timeout tests where they are part of the test contract.

### 2. Snapshot / Contract Tests

No dedicated API response snapshot tests; round-trip tests exist for some services but do not compare against stored golden responses. Volume of widget tests still does not warrant golden tests yet.

### 3. Benchmark CI Compilation Smoke Test

The 3 remaining benchmarks should be guarded by a CI "benchmark compiles" check to prevent silent rot.

---

## Test Quality Issues

### 1. Authorization Test Depth Improving

`requireConversationAccess` now has explicit table-driven tests covering participant, community member, non-member, and edge cases. This is the right pattern; recommend extending it with a shared helper across services (community, gear, request, experience) so that every RPC has at least one assertion of `connect.CodeUnauthenticated` / `CodePermissionDenied`.

### 2. Error Assertion Specificity

Many tests still use bare `if err != nil` rather than asserting on Connect error codes. Transfer/chat authorization tests demonstrate the right idiom (`var connectErr *connect.Error; errors.As(err, &connectErr); …`).

### 3. Test Helper Centralization

Across the test corpus there are several near-duplicates of `setupTestService` / `createAuthenticatedContext` style helpers per service. They have diverged slightly; a shared `testutil/` package may be worth considering, but only if signal indicates the divergence is causing real friction.

---

## Positive Findings

### 1. CRITICAL Backlog Cleared

All three CRITICAL recurring gaps from the prior review (`auth_repository`, `requireConversationAccess`, `impact_metrics` RPC service) closed in a single 4-day window. This is a notable execution sprint.

### 2. Coverage Parity for Flutter ViewModels and Repositories

67/67 ViewModels and 20/20 substantive repositories now have dedicated test files — a structural win that makes future drift easy to spot.

### 3. Portfolio Service Maturity

Portfolio went from 4 test files to 11 in 4 days. `aggregates_test.go`, `community_ranking_test.go`, `fetch_test.go`, `metrics_test.go`, `portfolio_metrics_test.go`, `actions_test.go`, `goals_test.go`, `inbox_feed_test.go`, `metric_detail_test.go`, `assembly_test.go` constitute solid coverage for a service that just landed.

### 4. Time.Sleep Hygiene

Removed from `chat/`, `community/lifecycle_test.go`, `storage/feed_test.go`. Remaining usages are mostly legitimate (timeout/integration). Net file count: 15 → 10.

### 5. Strong Library Test Suites Hold Up

`server/impact_metrics/` library (excluding `user_calculator.go`) remains one of the best-tested areas: builder, calculator, format, provenance, social_context, metric_detail components, plus 7 estimator-component tests in `estimator/`. The library/service split now has tests on both sides of the boundary.

### 6. Login Service Comprehensive RPC Coverage

5 dedicated RPC test files (authentication, password_reset, phone_auth, refresh, registration) plus `audit_logging_test.go` and `common_test.go`.

### 7. Robust Test Infrastructure

`testcontainers` with `pgvector/pgvector:pg16`; `t.Cleanup` used consistently; `WaitForNotification()` async pattern; Mockito `@GenerateMocks` in Flutter; CI runs Go and Flutter on every push.

### 8. Regression Test Culture

Named regression tests like `search_view_model_refresh_bug_test.dart`, `disposal_safety_test.dart`, `unread_count_multi_community_integration_test.dart` continue to demonstrate a healthy practice.

---

## Recommendations

### Immediate Actions (Next Sprint)

1. **Add `portfolio/inbox_view_test.go`** (3-5h, Go) — 756-line untested file
2. **Add `impact_metrics/user_detail_test.go` and `user_metrics_test.go`** (2-3h, Go) — only remaining gaps in that service
3. **Add `server/impact_metrics/user_calculator_test.go`** (2-3h, Go) — recurring 49+ day gap
4. **Restore (and expand) the AI fan-out benchmark** (1-2h, Go) — re-add `gen_ai_fanout_benchmark_test.go` or equivalent
5. **Delete `inbox_view_model_test.dart.bak`** (5 min) — recurring 142+ day cleanup
6. **Add login `helpers_test.go`** (1-2h, Go)

### Short Term (2-4 Weeks)

7. **Add benchmarks for SQL batch APIs and feed/portfolio assembly** (4-6h, Go)
8. **Cover remaining feed helpers** (`feed_nudges`, `feed_status`, `terminator_pool`) (2-3h, Go)
9. **Cross-service authorization sweep** (4-6h, Go) — assert Connect error codes on community, gear, request, experience
10. **Flutter services backfill** (8-10h) — currently ~26% covered (13/50)

### Medium Term (Next Quarter)

11. **Widget test backfill** (8-10h) — prioritize transfer modals, chat composers, impact displays
12. **Snapshot/contract tests** (6-8h) — at least for user-facing RPC responses with client caching behavior

### Tooling / Prevention

- Add a CI check that flags new `*_view_model.dart` files without `*_view_model_test.dart` (the parity is reached — guard it)
- Add a CI check that flags new `server/services/*/` directories without at least one `_test.go`
- Add a `go test -bench=. -benchtime=1x -run=^$` benchmark-compilation smoke job
- `grep`-based pre-merge check: forbid new `time.Sleep` in `_test.go` outside an allowlist of integration-timeout tests

---

## Conclusion

This is the strongest single-cycle improvement in the testing record. Three recurring CRITICAL gaps closed simultaneously, full Flutter ViewModel and Repository parity reached, portfolio and feed services moved from sparse to substantive coverage. Remaining gaps are now smaller and more specific (a few large files, the benchmark regression, the perennial `.bak` cleanup), and there are no current CRITICAL items in the queue.

**Overall Testing Grade: A** (improving) — Strong execution against the prior review's Immediate Actions; structural gaps remain but no longer in safety-critical paths.

---

## Issues to File

### [High] `portfolio/inbox_view.go` is 756 lines and untested

- **Fingerprint**: `review:testing:portfolio-inbox-view-untested`
- **Type**: Task
- **Why it matters**: Largest untested file in `server/services/`. Drives the user-visible portfolio inbox on the profile screen; assembly/ordering bugs surface as wrong items in front of end users. The rest of the portfolio service was tested this cycle, so this file stands out as the only major untested surface.
- **Locations**: `server/services/portfolio/inbox_view.go`
- **Recommendation**: Add `inbox_view_test.go` with table-driven coverage of inbox assembly, item-type mixing, week boundaries (`inbox_week.go`), and authorization paths. Follow the `metric_detail_test.go` / `assembly_test.go` patterns already in the directory.

### [High] `impact_metrics/user_detail.go` (526 lines) and `user_metrics.go` lack tests

- **Fingerprint**: `review:testing:impact-metrics-user-detail-untested`
- **Type**: Task
- **Why it matters**: After this cycle's sweep, these are the only two non-trivial untested files in `server/services/impact_metrics/`. They produce user-facing profile stats and leaderboard inputs; aggregation bugs would silently mislead users. Worth filing now while the service's testing momentum is high.
- **Locations**: `server/services/impact_metrics/user_detail.go`, `server/services/impact_metrics/user_metrics.go`
- **Recommendation**: Add `user_detail_test.go` and `user_metrics_test.go` modeled on `user_impact_detail_test.go` (same directory): authorization, request validation, response assembly, edge cases (no activity, single transaction).

### [High] Benchmark regression — only 3 benchmarks remain, no SQL/feed/portfolio coverage

- **Fingerprint**: `review:testing:benchmark-coverage-regression`
- **Type**: Task
- **Why it matters**: The prior review's `experience/gen_ai_fanout_benchmark_test.go` is no longer present, leaving only 3 `BenchmarkEmbedderGenerate*` functions across the entire server tree. Performance-sensitive hot paths (recently optimized in the SQL refactor) have no checked-in benchmark to detect regressions. CLAUDE.md's "measure before and after" rule is hard to follow without baselines.
- **Locations**: `server/` (no `Benchmark*` functions outside `server/ai/embedder_test.go`)
- **Recommendation**: Restore the AI fan-out benchmark and add benchmarks for `QueryByFieldIn`, `GetByIDs`, `InsertBatch`, plus feed and portfolio assembly hot paths. Add a CI smoke job (`go test -bench=. -benchtime=1x -run=^$ ./...`) so benchmark compilation can't silently break.

### [Medium] `server/impact_metrics/user_calculator.go` untested (recurring, 49+ days)

- **Fingerprint**: `review:testing:user-calculator-untested`
- **Type**: Task
- **Why it matters**: 394-line file that computes user activity counts and total value, feeding leaderboards and profile metrics. Flagged in two prior reviews; surrounding library files all have tests, this one stands alone.
- **Locations**: `server/impact_metrics/user_calculator.go`
- **Recommendation**: Add `user_calculator_test.go` with table-driven tests for activity counting and value aggregation across user-history shapes (no activity, single loan, mixed transaction types).
