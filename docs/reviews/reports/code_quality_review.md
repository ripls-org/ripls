# Code Quality Review - 2026-04-28

## Executive Summary

Since the previous review (2026-04-24), the codebase has made substantial,
measurable progress on the "headline" findings. Folder READMEs jumped from
~3 directories total to **39 server READMEs and 52 Dart READMEs**, the AI
provider files dropped from ~3,608 lines combined to **1,614 lines** (the
generic-adapter refactor recommended last time happened), and the Dart
file-size gate is now wired into CI (`npm run lint:dart:size`, called from
`.github/workflows/test_flutter.yaml`). All non-l10n Dart files are now
under 1,000 lines — a meaningful win that closes the largest sizing gap.
`revive`'s `package-comments` rule has been promoted to error severity, and
36 `doc.go` files have appeared to satisfy it. `dupl` has been added to
`.golangci.yml` (threshold 150).

What did **not** change: Dart still uses bare `flutter_lints` instead of
`very_good_analysis`; the `errorlint`/`misspell`/`goconst`/`bodyclose`/
`funlen`/`cyclop`/`nilerr`/`nilnil`/`unparam` linters are still missing
from `.golangci.yml`; **all 46** `//nolint:staticcheck` ConversationId/
CommunityId suppressions remain; vendor names still appear in 14 sites of
API protos and on internal Go service interfaces (`SetMapboxClient`,
`MapboxClient` etc.); material categories are still stringly-typed in the
AI providers and gear service despite a proto enum existing; phase-number
references in production source comments have actually grown (now 106
across non-test files) since several recent multi-phase efforts
(accessibility, impact, provenance, streaming) shipped without scrubbing
their phase markers; and TODO hygiene is roughly unchanged — 30 production
TODOs (down from 41), zero referencing a `TODO(#NNN)` issue.

The two highest-leverage remaining moves are: (1) a single CI lint pass
that rejects `Phase\s*[0-9]` and bare `TODO:` outside test files, paired
with a one-PR scrub, and (2) the Dart lint upgrade to `very_good_analysis`
plus the missing Go linters, which would all light up real existing
issues. The deprecated-fields migration (#3 below) remains the largest
single piece of accreting tech debt.

## Changes Since Last Review

**Previous Review Date**: 2026-04-24

**Resolved:**
- Folder READMEs added en masse: server packages went from 2 → 39, Dart
  from 1 → 52. Issue #1334 ("file comments and READMEs") tracked this and
  has merged.
- AI provider files refactored: `provider_openai.go` (1357 → 543),
  `provider_anthropic.go` (1331 → 550), `provider_gemini.go` (920 → 521).
  Total ~2,000 lines of duplication eliminated. Confirms the
  `callStructuredOutput[T]` pattern shipped.
- Dart file-size gate enforced in CI (`scripts/check_dart_file_size.sh`,
  called from `test_flutter.yaml`). All non-generated, non-l10n Dart files
  are now under 1,000 lines — the previously-cited 19 offenders
  (`conversation_view_model.dart` 3092, `gear_content_view.dart` 2465,
  etc.) have been split.
- `revive.package-comments` promoted from `warning` to `error` and 36
  `doc.go` files added under `server/`. Build now fails on a missing
  package comment.
- `dupl` linter enabled in `.golangci.yml` (threshold 150).
- Most server services packages now have a `README.md`.

**Still Outstanding:**
- 46 `//nolint:staticcheck` ConversationId/CommunityId suppressions —
  unchanged.
- Vendor names in 14 API proto sites (Mapbox, Unsplash, Pexels, Firebase,
  FCM); same `MapboxClient` interface exposed on `gear.Service` /
  `experience.Service` / `request.Service` — unchanged.
- Material category strings in `provider_openai.go` (and equivalents) —
  unchanged; proto enum still ignored on the AI side.
- `convertStoryTypeToAPI`, `convertStorageValueEstimateToAPI`,
  `convertAPISocialContextToModels` still duplicated across 2-3 service
  packages.
- `determineTransferActions(bool, ..., bool)` still defined twice in
  `chat/context.go:394` and `transfer/context.go:282`.
- 5 `strings.Contains(err.Error(), ...)` sites in `server/storage/`.
- TODO format unchanged — 0 of 30 production TODOs use `TODO(#NNN)`. 7
  `TODO(phase1)` entries in `server/services/leaderboard/leaderboard.go`
  unchanged.
- `flutter_lints` baseline still in place; no `very_good_analysis`.
- No `errorlint`, `misspell`, `goconst`, `bodyclose`, `funlen`, `cyclop`,
  `nilerr`, `nilnil`, `unparam` — exactly the set called out last time.

**New:**
- Phase references in non-test source code grew from 25+ to **106** as
  multi-phase efforts (accessibility, impact, provenance, webpage
  streaming) shipped without scrubbing phase markers. Examples include
  `server/services/experience/save.go:117,165,307`, `server/services/
  feed/service.go:491,569`, `server/jobs/impact_backfill.go:63,66`,
  `server/services/portfolio/portfolio_metrics.go:131`.
- Some Go files unsplit since last review remain >1000 lines:
  `server/ai/prompts.go` 1892 (was 1851), `server/main.go` 1026 (was
  ~580), `server/services/gear/service.go` 1381 (was 1388).
- 14 directories (1 in app, 13 in server) added since the README sweep
  still lack READMEs — see Comments & Documentation.
- `forbidigo` linter added (banning `time.Sleep` outside tests, scoped to
  flake-resistant tests). Good addition; not a finding.

**Trend**: Improving, materially. The biggest items from the previous
report (file size, AI duplication, READMEs, package comments) are largely
resolved or in motion. The remaining items are the longer-tail backlog:
deprecated-field migration, lint config gaps, vendor abstraction, and the
ephemeral-context comments rule. None of them is regressing fast, but
the comments rule is drifting in the wrong direction.

---

## Findings by Category

### DRY

#### Pattern 1: AI provider duplication — significantly reduced, not zero

`provider_openai.go` (543), `provider_anthropic.go` (550), and
`provider_gemini.go` (521) are roughly the same length (good — they
implement the same interface). Spot inspection shows the generic adapter
helper now carries the structured-output and vision-call sequences. Some
prompt-fragment shaping and response-parsing helpers may still duplicate
across files; without `dupl` running at threshold 100 in CI, hard to
detect. `dupl` is now enabled at threshold 150 — recommend tightening to
100 or 75 to catch any residual duplication.

- Locations: `server/ai/provider_openai.go`, `server/ai/provider_anthropic.go`,
  `server/ai/provider_gemini.go`.
- Why it matters: This refactor was the single largest payoff cited in
  the previous review and largely shipped. The remaining tail can be
  caught mechanically.
- Recommendation: lower the `dupl` threshold once existing AI files are
  clean; consider a per-file noise budget if 100 is too noisy.
- Effort: 2 hours to tune.

#### Pattern 2: Proto-to-API converter duplication — partially fixed

Some duplicates from the previous review have been removed (notably the
`convertTimeModelsToAPI` triple); others persist:

- `convertStoryTypeToAPI` at `server/services/user/user_stories.go:130`
  and `server/services/feed/stories.go:310` (still 2 copies).
- `convertStorageValueEstimateToAPI` at `server/services/gear/service.go:1039`
  and `server/services/community/gear_sharing.go:581` (2 copies).
- `convertAPISocialContextToModels` at `server/services/experience/save.go:432`
  and `server/services/request/service.go:167` (2 copies);
  `convertModelsSocialContextToAPI` only in experience.
- Why it matters: each new oneof case requires updating N copies; one
  drift instance was already documented in the previous review.
- Recommendation: introduce `server/services/convert/` as a shared
  package for cross-service converters, or expand `server/services/user_util.go`
  in-tree pattern. Lower `dupl` threshold to surface future regressions
  mechanically.
- Effort: 1 day.

#### Pattern 3: 46 `//nolint:staticcheck` ConversationId/CommunityId fallback suppressions

Unchanged from last review. Distribution by package (samples):
`server/services/feed/service.go` 4, `server/services/search/service.go`
10, `server/services/portfolio/fetch.go` 3, `server/services/portfolio/
inbox_feed.go` 2, plus dozens more. Confirmed via:

```bash
grep -rn "//nolint:staticcheck" server/ --include="*.go" | grep -v _test.go | wc -l
# 46
```

This is the single largest, most concentrated tech-debt accretion in the
codebase; documented in the previous review and **filed as an issue**.
Carrying forward in `## Issues to File`.

#### Pattern 4: `determineTransferActions` defined twice in `chat` and `transfer` packages

- `server/services/chat/context.go:394` —
  `func (s *Service) determineTransferActions(isOwner bool, userTransfer *api.Transfer, hasPendingRequests bool) []api.TransferAction`
- `server/services/transfer/context.go:282` — identical function with an
  unused `context.Context` first arg.
- Why it matters: same business logic in two places; one already takes
  ctx and the other doesn't, suggesting partial migration.
- Recommendation: move to a shared package (`server/services/transfer/
  actions.go` exposing a public `DetermineActions(...)`), have `chat`
  call it.
- Effort: 1 hour.

### Strong Typing

#### Go

- **Stringly-typed material categories — unchanged.**
  `proto/ripls/{api,models}/common.proto` define `MaterialCategory` as a
  proto enum; `server/ai/provider_openai.go:25-55` still validates against
  a string map (`{"solid_metal": true, ...}`), and `server/services/gear/
  service.go` still switches on lowercase strings.
- **`strings.Contains(err.Error(), ...)` for sentinel matching — unchanged.**
  Live sites: `server/storage/cascade_delete.go:14`, plus 4 in
  `server/storage/testing.go`. Each is a regression vector for upstream
  driver-message changes.
- **Sentinel errors still rare.** Spot check shows ~3 exported sentinel
  errors. With 46 `//nolint:staticcheck` suppressions referencing
  deprecated fields, callers can't match by error type — they pattern-
  match on field presence. Defining the missing sentinels would simplify
  several of the fallback paths.

#### Dart

- Unchanged from previous review. `flutter_lints` baseline still in place,
  so the deeper antipatterns (`as` without `is`, dynamic data carriers)
  are not enforced.

#### Proto

- **Vendor names in 14 API proto sites — unchanged.**
  - `proto/ripls/api/location_service.proto:114,118,122,361` ("Mapbox").
  - `proto/ripls/api/request_service.proto:53,113`,
    `proto/ripls/api/feed_service.proto:273` ("Unsplash").
  - `proto/ripls/api/media_service.proto:60` ("Pexels").
  - `proto/ripls/api/login_service.proto:92,93,95,112,113,115` ("Firebase").
  - `proto/ripls/api/device_service.proto:19` ("FCM").
- **No `COMMENTS` rules added to `buf.yaml` — unchanged.** Adding them
  would force documentation and make a vendor-name lint feasible.

### Comments & Documentation

#### File-level comments

- **Go: 89/367 non-test, non-generated files (24%) start with a leading
  `//`.** Up from 18% (59/320). Improvement is modest because the
  package-comments rule is satisfied by one `doc.go` per package; most
  individual source files still have no purpose-explaining header. Where
  CLAUDE.md says "non-trivial source file should have a top-of-file
  comment," the bar isn't being held.
- **Dart: 21/724 non-generated files (3%) start with a leading comment.**
  Down in absolute terms from 59 — the file-splitting initiative created
  many new short files, none of which were given a header comment.

#### Folder READMEs

- **Server: 39 READMEs across 82 directories.** Major improvement.
  Remaining gaps (multi-file dirs without README): `server/integration_tests`,
  `server/community`, `server/notifications/noop`, `server/notifications/fcm`,
  `server/services/waitlist`, `server/services/health`, `server/services/
  leaderboard`, `server/services/feedback`, `server/services/streaming`,
  `server/services/search`, `server/services/device`, `server/services/media`.
  ~12 directories remain.
- **Dart: 52 READMEs across 93 directories.** Similar large improvement.
  Remaining gaps include `app/lib/services/experience`,
  `app/lib/services/providers` (15 files), `app/lib/core/utils/drill_down`,
  `app/lib/presentation/screens/{experience,gear,request,impact_metrics}/widgets`,
  several widget subdirectories. ~14 directories remain.

#### Ephemeral context in production source comments — regressing

Phase numbers in non-test, non-generated source: **106 occurrences**, up
from 25 in the previous review. Recent multi-phase efforts shipped
without phase-marker scrubs. Acute clusters:

- `server/services/experience/save.go:117,165,307` — references to
  Phase 2, Phase 7.
- `server/services/feed/service.go:491,569` — "Phase 3:" labels on
  in-method comments.
- `server/services/portfolio/portfolio_metrics.go:131` — "impact5 Phase 2".
- `server/jobs/impact_backfill.go:63,66` — Phase 1 / Phase 2 labels.
- `server/services/leaderboard/leaderboard.go:41,63,74,90,124,129,163` —
  7 `TODO(phase1)` (also a TODO-format violation; see TODO Hygiene).
- `server/simulation/runner.go:70,106,122` and `activity.go:217,244` —
  Phase 1/2/3 narration of execution flow (these are *legitimate*
  algorithmic phases, not project phases — but the lint can't tell, so
  rename to "Step" / "Pass" to keep them out of the regex).
- 20 `#NNNN` issue-number references in non-test source comments.

#### Vendor names in Go internal APIs — unchanged

- `server/services/experience/service.go:42-43,56,120-121` exposes
  `MapboxClient` interface and `SetMapboxClient(client MapboxClient)`.
- Same in `server/services/gear/service.go` (line 71-ish per last review)
  and `server/services/request/service.go`.
- `server/main.go:587,621,638,648,760,779` instantiates and wires
  vendor-named providers directly. The interface should be capability-
  named (`GeocodingClient`, `StockImageProvider`).

### Style Guides & Automation

#### Go (`server/.golangci.yml`)

Currently enabled (no change since last review except `dupl` + `forbidigo`):
`errcheck`, `gosimple`, `govet` (enable-all), `ineffassign`, `staticcheck`,
`typecheck`, `unused`, `gofmt`, `goimports`, `revive` (with
`exported`+`package-comments` at error severity), `gocritic`, `gosec`,
`unconvert`, `whitespace`, `godot`, `goheader`, `gofumpt`, `stylecheck`,
`dupl` (threshold 150), `forbidigo`.

Missing/recommended additions, unchanged from last review:

| Linter | Rationale |
|--------|-----------|
| `errorlint` | Would flag the 5 `strings.Contains(err.Error(), ...)` sites and missing `%w` verbs. |
| `bodyclose` | HTTP client callers in `server/ai/` and `server/location/` use defer correctly today; cheap regression guard. |
| `misspell` | Free; catches typos in comments and logs. |
| `unparam` | Several services helpers have always-same-value params. |
| `goconst` | Would surface the material-category strings and other repeated literals. |
| `funlen` | Enforce CLAUDE.md's <100-line guideline. `main.go` has at least one massive function. |
| `cyclop` / `gocyclo` | Enforce the cyclomatic-complexity guideline. |
| `nilerr` / `nilnil` | Catch ambiguous `return nil, nil`. |
| `prealloc` | Small perf wins. |

Recommend tightening `dupl` threshold from 150 → 100 once the remaining
converter and AI-provider duplication is paid down (otherwise the gate
would fire on those existing patterns first).

#### Dart (`app/analysis_options.yaml`)

Unchanged: `flutter_lints` baseline; only `implementation_imports: false`
override. The previous report's recommendation to switch to
`very_good_analysis` is unimplemented.

`dart format --set-exit-if-changed` is still not in `npm run lint:dart`;
spot check confirms `lint:dart` runs only `flutter analyze`. CI is
relying on commit hygiene.

#### Proto (`buf.yaml`)

Unchanged: `STANDARD` minus `PACKAGE_VERSION_SUFFIX`. No `COMMENTS` rules,
no custom vendor-string check.

#### `//nolint` suppression inventory

- `//nolint:staticcheck`: 46 occurrences (unchanged), all on the
  ConversationId/CommunityId fallback. Pattern.
- Other `//nolint:` patterns: not re-counted here; previous review found
  17 `//nolint:gosec` mostly without justification — recommend a sweep
  to add `// reason:` on each.

### Dead Code

- Spot-check passes on commented-out code: still respected.
- `unused` lint enabled; passing.
- 65+ `t.Skip` sites in Go — all integration-mode gating, legitimate.
- No new dead-code findings since previous review.

### TODO Hygiene

**Summary: 30 production TODOs (down from 41); 0 use `TODO(#NNN)` format
(unchanged); 7 use `TODO(phase1)` (unchanged); rest are bare `TODO:`.**

| TODO | File:Line | Linked Issue | Issue State | Age |
|------|-----------|--------------|-------------|-----|
| `TODO: Use environment variable in production` (dev JWT secret) | `server/main.go:524` | none | — | unknown |
| `TODO: Implement community membership check if needed` | `server/services/experience/get.go:41` | none | — | unknown |
| `TODO: Add category detection to AI provider` | `server/services/experience/gen_ai.go:345`, `server/services/request/gen.go:160,275` | none | — | unknown |
| `TODO: This requires a database query that can handle array field matching` | `server/services/chat/conversations.go:390` | none | — | unknown |
| `TODO: Implement state name -> code lookup table` | `server/services/location/regions.go:326` | none | — | unknown |
| `TODO: Implement ownership/privacy controls as needed` | `server/services/gear/service.go:385` | none | — | unknown |
| 7x `TODO(phase1): ...` | `server/services/leaderboard/leaderboard.go:41,63,74,90,124,129,163` | none | — | unknown |
| `TODO: Also soft-delete related CommunityUser, CommunityGear, and CommunityInvitation records` | `server/services/community/lifecycle.go:188` | none | — | unknown |
| `TODO: Replace with LLM-assisted estimation per request type` | `server/impact_metrics/estimator/savings.go:90,91,180` | none | — | unknown |
| `TODO: Add completion timestamp to Request model for accurate period filtering` | `server/impact_metrics/metric_detail_calculator.go:457` | none | — | unknown |
| `TODO: Call genCommunityFromMedia RPC when server supports it` | `app/lib/data/repositories/community_repository.dart:55` | none | — | unknown |
| `TODO: Consider replacing with flutter_measure_formatter` / `Consider geodart` | `app/lib/core/utils/distance_formatter.dart:4,21` | none | — | unknown |
| `TODO: Find way to merge service and manager` | `app/lib/data/cache/cache_service.dart:20` | none | — | unknown |
| `TODO: Implement actual sharing to community feed/conversation` | `app/lib/presentation/viewmodels/event_modal_view_model.dart:764` | none | — | unknown |
| `TODO: Implement distance calculation` | `app/lib/presentation/viewmodels/leaderboard_view_model.dart:92` | none | — | unknown |
| `TODO: Remove this factory after GetConversationContext migration is complete` | `app/lib/presentation/models/inbox_item.dart:117` | none | — | unknown |
| `TODO: Implement API endpoint to delete region overrides` | `app/lib/presentation/screens/governance/governance_screen.dart:576` | none | — | unknown |
| `TODO: Wire up actual fulfillment history from server` / `session history` | `app/lib/presentation/screens/item/item_metrics_screen.dart:314,320` | none | — | unknown |

- **Total production TODOs**: 30 (22 Go, 8 Dart). Down from 41.
- **TODOs with `TODO(#NNN)` issue-number format**: 0 (unchanged).
- **Dev-shortcut TODOs that are actually security issues**:
  `server/main.go:524` (hardcoded JWT secret in dev mode) — should be
  build-flagged or env-required, not TODO'd.
- **`FIXME`/`HACK`/`XXX`/`WORKAROUND` markers**: 0 hits.

### Naming Consistency

No new findings. Previous-report observations carry forward unchanged.
`MapboxClient` / `PexelsProvider` / `UnsplashProvider` etc. as exposed
type names is the durable issue — see Vendor Names in Comments &
Documentation.

### Function & File Sizing

#### Files >1000 lines (Go)

| File | Lines (prev → now) | Recommendation |
|------|--------------------|----------------|
| `server/ai/prompts.go` | 1851 → 1892 | Slightly larger. Split per-domain (gear/experience/request/story/nudge). |
| `server/impact_metrics/metric_detail_calculator.go` | 1511 → 1511 | Unchanged. Per-metric file split. |
| `server/services/gear/service.go` | 1388 → 1381 | Effectively unchanged. Extract converters to `gear/converters.go`. |
| `server/services/feed/service.go` | 1187 → 1146 | Marginally smaller. Split fan-out and converters. |
| `server/main.go` | (unmeasured before, ~580) → 1026 | New entry into the >1000-line bucket. Likely from added service-wiring. Extract logical sections (flag parsing, storage init, service wiring, jobs, listen) into helpers. |

#### Files >1000 lines (Dart)

**All non-l10n Dart files are now under 1,000 lines.** The 19 Dart files
called out previously have all been split — verified via:

```bash
find app/lib -name "*.dart" -not -name "*.g.dart" -not -name "*.freezed.dart" \
  -not -name "*.mocks.dart" -not -path "*/gen/*" \
  -exec wc -l {} \; | awk '$1 > 1000 && $2 !~ /app_localizations/'
# (no output)
```

The l10n files (`app_localizations*.dart`) are 4-8K lines but generated;
correctly exempted.

#### Functions

Not re-measured. The only functions previously called out >100 lines
were the AI provider methods (now refactored), `main()` (still
oversized — see above), and a handful in `simulation` that are
algorithmically structured around clear phases. Without `funlen`/
`cyclop` enabled, no new evidence to surface.

### Typed Errors

Unchanged. 5 `strings.Contains(err.Error(), ...)` sites in
`server/storage/cascade_delete.go:14` and `server/storage/testing.go:456,
546,599,609`. Define `var ErrRecordNotFound = errors.New(...)` at
`server/storage/errors.go`, wrap at the source, match with `errors.Is`.

### Magic Numbers and Strings

Unchanged. Material category strings (3+ sites) are the highest-value
target. `goconst` linter would mechanize.

### Boolean Parameters

`determineTransferActions` (3 bool-equivalent params, with one being
`hasPendingRequests bool`) still defined twice — see DRY pattern 4. No
other functions with 2+ bool params surfaced.

---

## Tooling Recommendations

Consolidated; previous-review recommendations carried forward where still
unimplemented, with status notes. Effort estimates assume baseline
familiarity with the relevant tools.

| # | Change | Status | Effort | Impact |
|---|--------|--------|--------|--------|
| 1 | Enable `dupl` in `.golangci.yml` | **DONE** (threshold 150) | — | Tighten threshold to 100 once remaining duplication is paid down. |
| 2 | Enable `errorlint`, `misspell`, `goconst`, `bodyclose`, `unparam`, `prealloc`, `nilerr`, `nilnil` | Open | 30 min + ~1-2 days fix | Cheap wins; surfaces 5+ known sites and probably more. |
| 3 | Enable `funlen` (limit 100) and `cyclop` (limit 15) | Open | 1 hr setup | Forces `main()` and `prompts.go` cleanup; grandfather existing. |
| 4 | Promote `revive`'s `package-comments` to error; add CI check requiring README per multi-file package | **DONE for package-comments**; README check still manual | 1 hr remaining | Closes the ~25 remaining README gaps. |
| 5 | Switch Dart to `very_good_analysis` | Open | 1 hr setup + ~2-3 days fix | Catches `dynamic`, `as` casts, missing return types. |
| 6 | Add `dart format --set-exit-if-changed` to `npm run lint:dart` | Open | 10 min | Brings Dart in line with Go's formatting enforcement. |
| 7 | Add `buf` `COMMENTS` rules + custom script flagging vendor strings in `.proto` comments | Open | 1 hr | Closes the 14 vendor-leak sites; prevents regression. |
| 8 | Add CI lint rejecting `TODO` without `(#NNN)` in non-test source | Open | 1 hr | Forces every TODO to link an issue per CLAUDE.md. |
| 9 | Add CI lint rejecting `Phase\s*[0-9]` and standalone `#NNNN` in non-test source comments | Open, **higher priority now** | 1-2 hr (regex needs to handle "Step 1" allowlist for legit algorithm narration) | Enforces CLAUDE.md's ephemeral-context rule. 106 sites today, regressing. |
| 10 | Add a pre-commit hook (lefthook/husky) running `gofumpt`, `dart format`, `buf lint` on staged files | Open | 1-2 hr | Shifts left from CI. |
| 11 | Add custom Dart file-size gate at 1000 lines | **DONE** (`scripts/check_dart_file_size.sh`) | — | Prevents regression. |

## Recommendations

Grouped by effort.

### Quick wins (hours)

1. **Add CI lint for `Phase\s*[0-9]` and bare `TODO` in non-test source.**
   Highest-leverage single change available — phase references regressed
   from 25 → 106 since the previous review. One regex-based check can
   freeze the trend, then a one-PR scrub clears the existing 106. Allow-
   list "Step 1/2/3" or rename legitimate `simulation/runner.go` phases.
2. **Add the missing Go linters** (`errorlint`, `misspell`, `goconst`,
   `bodyclose`, `nilerr`, `nilnil`, `unparam`). Each is a one-line config
   change; first runs typically have <20 findings.
3. **Switch `analysis_options.yaml` to `very_good_analysis`** (one-line
   include change + clearing the first run).
4. **Add `dart format --set-exit-if-changed` to `npm run lint:dart`.**
5. **Fix the 5 `strings.Contains(err.Error(), ...)` sites** by defining
   `ErrRecordNotFound` in `server/storage/errors.go`.
6. **Dedupe `determineTransferActions`** between `chat` and `transfer`.
7. **Fix the dev-secret `TODO`** at `server/main.go:524` — make it env-
   required or build-flagged, not a TODO.
8. **Tighten `dupl` threshold from 150 to 100** once converter cleanup
   lands.

### Medium efforts (days)

9. **Extract duplicated converter helpers** into `server/services/convert/`.
10. **Replace stringly-typed material-category code** with the proto
    enum; introduce a single string-↔-enum adapter at the AI JSON
    boundary.
11. **Split `server/main.go`** into focused helpers (1026 lines is too
    much for an entry point).
12. **Add per-package READMEs** for the ~12 server and ~14 Dart
    multi-file directories still missing one.
13. **Add per-file leading comments** for the ~80% of source files that
    lack them. Could be paired with a custom lint that requires a
    leading comment block (Go `revive` doesn't do this per-file by
    default).

### Large efforts (weeks)

14. **Complete the ConversationId/CommunityId migration:** backfill the
    canonical fields, drop the deprecated proto fields, delete all 46
    `//nolint:staticcheck` suppressions in one coordinated proto
    change. This is the single largest accreting tech-debt item
    remaining.
15. **Capability-rename internal vendor interfaces:** `MapboxClient` →
    `GeocodingClient`, `UnsplashProvider`/`PexelsProvider` →
    `StockImageProvider`. Touches services and `main.go` wiring; pair
    with proto-comment vendor scrub (#7 above).

---

## Issues to File

### [High] Phase-number references in production source comments regressing

- **Fingerprint**: `review:code_quality:ephemeral-context-in-comments`
- **Type**: Task
- **Why it matters**: Phase references in non-test, non-generated source
  grew from ~25 to 106 since the last review (2026-04-24). CLAUDE.md
  explicitly forbids ephemeral context in source comments — phase
  numbers, PR numbers, branch names rot the moment surrounding work
  ships. Without a CI gate the trend is in the wrong direction; recent
  multi-phase efforts (accessibility, impact, provenance, webpage
  streaming) all leaked phase markers into durable code. Some are also
  TODO-format violations (7 `TODO(phase1)` entries in
  `server/services/leaderboard/leaderboard.go`).
- **Locations**: representative clusters: `server/services/experience/save.go:117,165,307`;
  `server/services/feed/service.go:491,569`;
  `server/services/portfolio/portfolio_metrics.go:131`;
  `server/jobs/impact_backfill.go:63,66`;
  `server/services/leaderboard/leaderboard.go:41,63,74,90,124,129,163`;
  `server/chat/system_messages.go:93`. Total 106 across the tree.
- **Recommendation**: add a CI lint that fails on `Phase\s*[0-9]+` and
  standalone `#[0-9]{3,4}` in `*.go`/`*.dart` comments outside test
  files. Allowlist legit algorithmic phases ("Step 1/Step 2") via
  rename. Land one cleanup PR clearing the 106 existing sites.
- **Suggested labels**: `tech-debt,tooling`

### [High] Complete ConversationId/CommunityId deprecation migration

- **Fingerprint**: `review:code_quality:conversation-community-id-deprecation-fallback`
- **Type**: Task
- **Why it matters**: 46 `//nolint:staticcheck` suppressions across
  20+ files document live reads from deprecated proto fields —
  unchanged in count since the previous review. The pattern keeps
  accreting (every new feature touching conversation context adds
  another suppression) and gates real data-model fallback logic in
  hot paths.
- **Locations**: `server/services/feed/service.go` (4 sites); `server/services/search/service.go` (10);
  `server/services/portfolio/fetch.go` (3); `server/services/portfolio/inbox_feed.go` (2); plus 27 more —
  full enumeration via `grep -rn "//nolint:staticcheck" server/ --include="*.go" | grep -v _test.go`.
- **Recommendation**: backfill canonical `Gear.conversation_id`/`Request.conversation_id`/
  `Experience.conversation_id` for any pre-migration records, drop the
  deprecated fields on `CommunityGear`/`CommunityRequest`/`CommunityExperience`
  in a single coordinated proto change, delete all 46 suppressions
  together. Open one tracking issue.
- **Suggested labels**: `refactor,tech-debt`

### [High] `.golangci.yml` still missing widely-adopted linters

- **Fingerprint**: `review:code_quality:golangci-missing-recommended-linters`
- **Type**: Task
- **Why it matters**: Despite the previous review's call-out, none of
  `errorlint`, `misspell`, `goconst`, `bodyclose`, `unparam`, `funlen`,
  `cyclop`, `nilerr`, `nilnil` are enabled. Each maps to known existing
  issues in the codebase: `errorlint` would catch the 5 active
  `strings.Contains(err.Error(), ...)` sites; `goconst` would surface
  the material-category strings; `funlen`/`cyclop` would have caught
  the bloat of `main.go` (now 1026 lines).
- **Locations**: `server/.golangci.yml`.
- **Recommendation**: land in two waves. Wave 1 (low-noise):
  `errorlint`, `misspell`, `bodyclose`, `nilerr`, `nilnil`. Wave 2
  (needs `//nolint:` grandfathering): `goconst`, `funlen`, `cyclop`,
  `unparam`. Each wave lands with matching baseline fixes.
- **Suggested labels**: `tooling,tech-debt`

### [High] Vendor names persist in API protos and Go service interfaces

- **Fingerprint**: `review:code_quality:vendor-names-in-api-surface`
- **Type**: Task
- **Why it matters**: 14 sites in `proto/ripls/api/` still mention
  Mapbox, Unsplash, Pexels, Firebase, FCM (CLAUDE.md "Proto Comments"
  rule). Internal Go service interfaces still expose vendor-named
  types: `SetMapboxClient(MapboxClient)` on `gear.Service`,
  `experience.Service`, `request.Service`. Switching geocoders or
  stock-image providers requires changing every service's API, not
  just the location/media package.
- **Locations**: `proto/ripls/api/location_service.proto:114,118,122,361`;
  `proto/ripls/api/request_service.proto:53,113`;
  `proto/ripls/api/feed_service.proto:273`;
  `proto/ripls/api/media_service.proto:60`;
  `proto/ripls/api/login_service.proto:92,93,95,112,113,115`;
  `proto/ripls/api/device_service.proto:19`. Go: `server/services/experience/service.go:42-43,120`;
  `server/services/gear/service.go`; `server/services/request/service.go`.
- **Recommendation**: (1) rename proto comments to capability terms
  ("geocoding provider", "stock image provider", "phone OTP ID
  token"); (2) introduce `GeocodingClient`, `StockImageProvider`
  capability interfaces and have services depend on those; (3) add a
  custom buf lint or simple grep step that fails on a known
  vendor-name list in `.proto` comments.
- **Suggested labels**: `refactor,tech-debt`

### [Medium] Material-category enum exists but Go uses stringly-typed values

- **Fingerprint**: `review:code_quality:stringly-typed-material-category`
- **Type**: Task
- **Why it matters**: `proto/ripls/{api,models}/common.proto` define a
  `MaterialCategory` enum. AI providers and `gear.Service` still treat
  it as a string with hardcoded validator maps and lowercased switch
  arms, despite the proto enum being available. Adding a new value
  requires editing N source files instead of one. Errors surface only
  at runtime.
- **Locations**: `server/ai/provider_openai.go:25-55` (validator map);
  `server/services/gear/service.go` (lowercased switch ~lines 1358-1378
  per previous review).
- **Recommendation**: convert at the AI JSON boundary only; use
  `models.MaterialCategory` everywhere else in Go. Add a unit test
  asserting every proto enum value has a JSON-string mapping.
- **Suggested labels**: `refactor,type-safety`

### [Medium] TODOs unlinked to GitHub issues

- **Fingerprint**: `review:code_quality:todos-without-issue-links`
- **Type**: Task
- **Why it matters**: 30 production TODOs, 0 use the CLAUDE.md-mandated
  `TODO(#NNN)` format. 7 `TODO(phase1)` entries in
  `server/services/leaderboard/leaderboard.go` are also ephemeral
  context (overlapping with the phase-number issue). One TODO at
  `server/main.go:524` documents a hardcoded JWT secret as a
  production-readiness gap — that's a real security issue, not a
  TODO. Untracked TODOs rot.
- **Locations**: representative gaps: `server/main.go:524` (dev JWT
  secret); `server/services/leaderboard/leaderboard.go:41-163` (7
  `TODO(phase1)`); `server/services/community/lifecycle.go:188`
  (incomplete cascade soft-delete); `app/lib/presentation/models/inbox_item.dart:117`
  (factory pending migration cleanup). Full table in the report.
- **Recommendation**: (1) add a CI lint rejecting `TODO` without
  `(#NNN)` in non-test source; (2) one triage pass — file an issue
  per durable TODO, delete the rest, replace `TODO(phase1)` with
  `TODO(#NNN)` referencing the leaderboard tracking issue.
- **Suggested labels**: `tech-debt,tooling`
