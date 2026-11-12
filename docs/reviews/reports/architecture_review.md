# Architecture Review - 2026-04-28

## Executive Summary

**Overall Architecture Grade: A-**

This is a substantial improvement cycle. The two longest-standing findings — the `server/services/community` cross-service dependency (143 days open) and the oversized Dart files — are both **resolved**, and the resolutions came with the prevention controls earlier reviews recommended (`TestServiceIsolation` and `lint:dart:size`).

- **Service-to-service dependencies eliminated.** Zero non-test files under `server/services/<X>/` import `server/services/<Y>/community/` (down from 11 across 5 services). `RecordCommunityEventAndNotify`, `GetNumCommunityMembers`, and `MaxCommunityMembers` all live in `server/community/` now. `TestServiceIsolation` (`server/services/service_isolation_test.go`) statically enforces this invariant at CI time, with three explicitly-grandfathered exceptions (`fanout`, `streaming`, `impact_metrics`) documented in code.
- **Oversized Dart files eliminated.** Zero non-generated Dart files under `app/lib/` exceed 1,000 lines (down from 20). `conversation_view_model.dart` is gone; `gear_content_view.dart` and `experience_content_view.dart` are no longer in the top-30 by size. `npm run lint:dart:size` (`scripts/check_dart_file_size.sh`) is wired into the lint pipeline.
- **`convertTimeModelsToAPI` deduplicated.** Centralized to `server/services/experience_converters.go::ConvertTimeModelsToAPI` with five callers across `experience`, `search`, `feed`, `community/shadow_users`, and `experience/time_proposals`. The previous three duplicate copies are gone.
- **`server/services/impact_metrics/` now has tests.** 10 `_test.go` files (up from zero), including `service_test.go`, `actions_test.go`, `leaderboard_test.go`, `percentile_test.go`, `time_to_solve_test.go`, `utilization_test.go`, `user_impact_detail_test.go`, `drafting_test.go`, `prompt_golden_test.go`, plus shared `common_test.go`.
- **Dart viewmodel test coverage up.** 74 of 97 viewmodels have tests (~76%, up from ~66%). `lint:dart:viewmodel-tests` now enforces this.

The few residual issues are old, low-impact, and well-known: `chat/system_messages.go` still imports upward into `server/services`; `PostCreationService` still imports two viewmodels; six viewmodels still read service providers directly; `connect.NewError(connect.CodeInternal, …)` is up to 492 sites across 100 files. None of these clear the issue-filing bar — they're tracked here and in the report sections below.

Grade rises to **A-** (from B). It would be A if `connect.NewError` boilerplate had centralized this cycle, but that's the only structural item with no fix in flight.

---

## Changes Since Last Review

**Previous Review Date**: 2026-04-24
**Previous Grade**: B
**Current Grade**: A-
**Grade Change**: ↑ Improved

**Resolved Issues:**
- ✅ **Service-to-service dependency on `server/services/community`** (Critical, 143 days open)
  - Location: now `server/community/{events,membership,constants}.go`
  - How resolved: shared utilities moved to library package; 11 importers migrated; new `TestServiceIsolation` (`server/services/service_isolation_test.go`) statically enforces no service imports another service.
- ✅ **`conversation_view_model.dart` (3092 lines)** (High, 143 days open)
  - Location: file no longer exists in its previous oversized form
  - How resolved: file is no longer in the top non-generated Dart files; zero non-generated Dart files exceed 1,000 lines.
- ✅ **Oversized Dart screens/widgets (20 files > 1000 lines)** (High)
  - How resolved: all 20 files now under 1,000 lines; `npm run lint:dart:size` is wired into the lint pipeline (`scripts/check_dart_file_size.sh`).
- ✅ **`convertTimeModelsToAPI` duplicated 3×** (High, 143 days open)
  - Location: centralized in `server/services/experience_converters.go:11` (`ConvertTimeModelsToAPI`)
  - How resolved: deduped into shared helper in `server/services/`; five callers now share it.
- ✅ **`server/services/impact_metrics/` had zero tests** (Medium, 143 days open)
  - Location: `server/services/impact_metrics/` now has 10 `_test.go` files
  - How resolved: test files added across the service surface (RPC handlers, leaderboard, percentile, drafting, time-to-solve, utilization, user impact detail).
- ✅ **CI gate for file-size regression** (Low, prior recommendation)
  - How resolved: `npm run lint:dart:size` exists; `package.json` chains it into `npm run lint`.

**Unaddressed Issues:**
- ⏳ **`chat/system_messages.go` imports `server/services`** (originally Low)
  - Days Open: 147 days
  - `server/chat/system_messages.go:11` still imports `server/services` for `services.FetchAPIUser`. Architecturally noisy but currently harmless (no compile-time cycle because nothing in `server/services/` imports `server/chat/` directly).
- ⏳ **`PostCreationService` imports viewmodels** (originally Medium)
  - Days Open: 147 days
  - `app/lib/services/post_creation_service.dart:2-3` still imports `feed_view_model.dart` and `home_view_model.dart`.
- ⏳ **Direct `*ServiceProvider` reads in 6 viewmodels** (originally Medium)
  - Days Open: 147 days
  - Same six locations: `gen_gear_view_model.dart:615`, `gear_metric_view_model.dart:249`, `gen_request_view_model.dart:340`, `governance_view_model.dart:115`, `profile_locations_view_model.dart:42`, `leaderboard_view_model.dart:143/204`.
- ⏳ **Error-handling boilerplate** (originally Medium)
  - `connect.NewError(connect.CodeInternal, …)` now appears 492 times across 100 files (up from 478/99). No `server/connecterr/` package exists yet.
- ⏳ **`server/services/fanout` and `streaming` are libraries in service-space** (originally Low)
  - Days Open: 147 days
  - Still under `server/services/`. These are now formal grandfathered exceptions in `TestServiceIsolation`, which makes the situation explicit but does not move them.

**New Issues:**
- 🆕 None of architectural significance. Three Go files remain over 1,000 lines (`prompts.go` 1892, `gear/service.go` 1381, `feed/service.go` 1146); `prompts.go` and `gear/service.go` are the same calls flagged previously; `feed/service.go` actually shrank (1187 → 1146).

**Architectural Improvements:**
- New `TestServiceIsolation` static enforcement (`server/services/service_isolation_test.go`).
- `server/community/` library now real (events, membership, constants, regions, doc, plus tests).
- `experience_converters.go` and `roundtrip_helpers.go` form a clear cross-service-helper layer at `server/services/`.
- Multiple lint scripts wired into `npm run lint`: `lint:dart:size`, `lint:dart:a11y`, `lint:dart:viewmodel-tests`, `lint:dart:i18n`, `lint:nav`.
- Service test coverage now 21/21 services with ≥1 test file.

**Metrics Comparison:**

| Metric | Previous | Current | Change |
|--------|----------|---------|--------|
| Services importing another service (cross-imports) | 11 files / 5 services | 0 files (3 grandfathered libs only) | Resolved |
| Files > 1000 lines (Go, non-test) | 6 | 5 | -1 |
| Files > 1000 lines (Dart, non-gen, non-l10n) | 20 | 0 | -20 (resolved) |
| Dart viewmodels with tests | ~66% (52/79) | ~76% (74/97) | +10pp |
| Proto separation | 100% | 100% | Stable |
| `connect.NewError(CodeInternal)` call sites | 478 | 492 | +14 (slow growth) |
| Services with ≥1 test file | 21/22 | 21/21 | All covered |
| `impact_metrics` service test files | 0 | 10 | +10 |
| `convertTimeModelsToAPI` duplicates | 3 | 1 (single shared helper) | Resolved |

**Trend**: ↑ Strongly improving. Three of the five long-standing findings closed this cycle, each with a CI/test gate to prevent regression.

---

## Critical Issues

_None._

---

## High Priority

_None this cycle. The previous review's two High findings (`conversation_view_model.dart` and oversized screens) are resolved._

---

## Medium Priority

### 1. `connect.NewError(connect.CodeInternal, …)` Boilerplate at 492 Sites

**Location:** 492 occurrences across 100 Go files (up from 478/99). No central wrapper exists.

**Impact:** Each error boundary hand-rolls the same wrap with slightly different message phrasing. Inconsistent messages across services and a tendency to default to `CodeInternal` where `NotFound`, `InvalidArgument`, or `PermissionDenied` would be more accurate. The count grew this cycle, so the trend is still away from a fix.

**Recommendation:** Create `server/connecterr/connecterr.go` with constructors:
```go
func Internal(err error, op string) *connect.Error
func PermissionDenied(msg string) *connect.Error
func NotFound(entityType, id string) *connect.Error
func InvalidArgument(err error) *connect.Error
```
Migrate top 3 services (gear, transfer, request — the largest call sites). Add a `golangci-lint` rule that flags raw `connect.NewError(connect.CodeInternal, …)` outside `connecterr/`.

### 2. `PostCreationService` Imports ViewModels (Layer Violation, Unresolved)

**Location:** `app/lib/services/post_creation_service.dart:2-3`
```dart
import 'package:ripls/presentation/viewmodels/feed_view_model.dart';
import 'package:ripls/presentation/viewmodels/home_view_model.dart';
```

**Impact:** Services are below the presentation layer; importing viewmodels inverts the dependency. One file today; it sets a precedent. Open 147 days.

**Recommendation:** Move `PostCreationService` to `app/lib/presentation/services/` (a presentation-layer service) or convert it into a mixin applied to viewmodels that need the post-creation flow. Optionally add a custom_lint rule forbidding `services/*.dart` from importing `presentation/viewmodels/*.dart`.

### 3. Direct Service Access in 6 ViewModels (Reduced but Persistent)

| ViewModel | Service | Location |
|-----------|---------|----------|
| `leaderboard_view_model.dart` | `communityServiceProvider` | Lines 143, 204 |
| `profile_locations_view_model.dart` | `userServiceProvider` | Line 42 |
| `gen_gear_view_model.dart` | `locationServiceProvider` | Line 615 |
| `governance_view_model.dart` | `locationServiceProvider` | Line 115 |
| `gen_request_view_model.dart` | `locationServiceProvider` | Line 340 |
| `gear_metric_view_model.dart` | `transferServiceProvider` | Line 249 |

**Impact:** Bypasses cache layer; can produce stale or duplicate fetches. Same six locations as last review. `locationRepository`, `leaderboardRepository`, and `userRepository` already exist so the migrations are mechanical.

**Recommendation:** Route each call through its repository. Effort ≈ a few hours.

---

## Low Priority

### 4. Three Oversized Go Files

| File | Lines | Status |
|------|-------|--------|
| `server/ai/prompts.go` | 1892 | grew slightly (1851 → 1892) |
| `server/impact_metrics/metric_detail_calculator.go` | 1511 | unchanged |
| `server/services/gear/service.go` | 1381 | improved (1388 → 1381) |
| `server/services/feed/service.go` | 1146 | improved (1187 → 1146) |
| `server/main.go` | 1026 | newly above (was 988) |

**Recommendation:**
- `prompts.go`: split by subject (`prompts_gear.go`, `prompts_experience.go`, `prompts_request.go`, `prompts_system.go`).
- `gear/service.go`: split along CRUD vs. gen vs. sharing boundaries (mirror `community/`'s split).
- `main.go`: extract groups of service wiring into per-domain `wire_*.go` files.

### 5. `server/chat/system_messages.go` Imports Upward (Unresolved)

**Location:** `server/chat/system_messages.go:11` imports `type2fun.org/ripls/server/services` for `services.FetchAPIUser`.

**Impact:** `server/chat/` is intended as a library below the service layer; importing upward creates a latent cycle that could break compilation if another service tries to import chat. Today the graph happens not to cycle. Open 147 days.

**Recommendation:** Move `FetchAPIUser` from `server/services/user_util.go` into a lower-level `server/users/` (or `server/userutil/`) helper, then have `server/chat/system_messages.go` depend on that.

### 6. `server/services/fanout` and `streaming` Still in Service-Space

**Location:** `server/services/fanout/`, `server/services/streaming/`. Now formally grandfathered as exceptions in `TestServiceIsolation`.

**Impact:** Naming-only — they aren't services. The grandfathering keeps the situation explicit. Low.

**Recommendation:** Move to `server/fanout/` and `server/streaming/` alongside `server/chat/`. Drop the corresponding `knownAllowedServiceImports` entries.

---

## Positive Findings

### 1. Service Isolation Now Statically Enforced
- `TestServiceIsolation` (`server/services/service_isolation_test.go`) parses every Go file under `server/services/` and fails CI if a file imports a sibling service. Three packages are explicitly grandfathered with a clear migration intent. This is the single best architectural improvement this cycle.

### 2. `server/community/` Library Extraction Completed
- Files: `events.go`, `events_test.go`, `membership.go`, `membership_test.go`, `notifications_test.go`, `constants.go`, `regions.go`, `doc.go`. The previous review's "extraction was planned but not executed" finding is fully resolved.

### 3. Dart File-Size Regression Reversed
- Twenty oversized files → zero. Combined with `lint:dart:size` enforcement, this finding is closed at the source and prevented from recurring.

### 4. `convertTimeModelsToAPI` Centralized
- Single canonical implementation at `server/services/experience_converters.go::ConvertTimeModelsToAPI`. Five callers now share it (`experience/service.go:271`, `experience/time_proposals.go:426`, `search/service.go:367,622`, `community/shadow_users.go:326`, `feed/service.go:1014`).

### 5. Comprehensive Lint Pipeline
- `npm run lint` chains: `lint:proto`, `lint:go`, `lint:terraform`, `lint:dart`, `lint:dart:size`, `lint:dart:a11y`, `lint:dart:viewmodel-tests`, `lint:nav`. Each is a separate executable with focused scope.

### 6. Proto Separation — Still 100%
- Zero cross-imports between `proto/ripls/api/` and `proto/ripls/models/`.
- Zero `ripls/models/` imports in Flutter outside generated code.

### 7. Dependency Injection Still Clean
- All 21 services use constructor injection.
- No circular dependencies inside `server/services/*`.

### 8. Service Test Coverage Excellent
- 21/21 services have ≥1 `_test.go` file. Heavy hitters: experience (29), chat (15), request (14), community (12), transfer (11), portfolio (10), impact_metrics (10).

### 9. Viewmodel Test Coverage Up
- 74/97 (~76%) viewmodels have tests, up from 52/79 (~66%). `lint:dart:viewmodel-tests` enforces it.

---

## Metrics

| Metric | Value | Target | Status |
|--------|-------|--------|--------|
| Service Independence | 100% (zero cross-service imports outside grandfathered libs) | 100% | EXCELLENT |
| Proto Separation | 100% | 100% | COMPLIANT |
| RPC Message Uniqueness | 100% | 100% | COMPLIANT |
| Repository Usage (Flutter VMs) | ~94% (91/97 avoid direct-service fetching) | 100% | GOOD |
| Files > 1000 lines (Go, non-test) | 5 | 0 | NEEDS WORK |
| Files > 1000 lines (Dart, non-gen, non-l10n) | 0 | 0 | COMPLIANT |
| Go Service Test Coverage | 100% (21/21) | 100% | EXCELLENT |
| Dart VM Test Coverage | ~76% (74/97) | 80% | GOOD |
| Manual Cache Maps in VMs | 0 | 0 | COMPLIANT |
| Layer Violations (Flutter) | 1 (`PostCreationService`) | 0 | NEEDS FIX |
| Error-Pattern Call Sites | 492 | < 100 | NEEDS WORK |
| DI Compliance | 9.5/10 | 9+ | EXCELLENT |
| Static Service-Isolation Test | Present | Present | COMPLIANT |
| File-Size Lint Gate (Dart) | Present | Present | COMPLIANT |

---

## Recommendations

### Tier 1 — Medium (this or next cycle)

1. **Create `server/connecterr/` package** and migrate the top 3 services (gear, transfer, request).
   - Resolves a 478→492 trend that's growing slowly. ≈ 1 day.

2. **Move `PostCreationService` to `app/lib/presentation/services/`** (or convert to a mixin).
   - Resolves 147-day-open Flutter layer violation. ≈ 1-2 hours.

3. **Migrate the 6 remaining direct-service viewmodel reads to repositories.**
   - All target repositories already exist. ≈ Half-day.

### Tier 2 — Low (continuous)

4. Split `server/ai/prompts.go` by domain (1892 → 4 × ~500-line files).

5. Split `server/services/gear/service.go` mirroring `community/`'s decomposition.

6. Move `FetchAPIUser` to `server/userutil/` so `server/chat/system_messages.go` no longer imports `server/services`.

7. Promote `server/services/fanout` and `streaming` out of service-space; drop grandfathering in `TestServiceIsolation`.

8. Add a `golangci-lint` custom rule that flags new `connect.NewError(connect.CodeInternal, …)` outside `server/connecterr/` once that package exists.

---

## Document History

| Date | Author | Changes |
|------|--------|---------|
| 2025-12-03 | Claude Code | Initial architecture review |
| 2026-03-10 | Claude Code | protosql split resolved, community cross-deps identified, Dart test coverage improved |
| 2026-04-24 | Claude Code | Three review cycles in, community extraction and conversation VM split still unaddressed; Dart oversized files regressing; grade lowered to B |
| 2026-04-28 | Claude Code | Community extraction completed, Dart oversized files eliminated, service isolation statically enforced, impact_metrics service tested; grade raised to A- |

---

## Issues to File

_None this run._

The two findings that would have been Critical/High in past cycles (community extraction; oversized Dart files) are both resolved with CI gates preventing regression. Remaining findings are Medium/Low and either already represented by existing (likely) backlog issues or are not severe enough to warrant a new GitHub issue per `docs/reviews/issue_filing_spec.md`. They are documented in this report for the next reviewer to track.
