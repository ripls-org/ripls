# Issue #1362: [testing] 21 ViewModels untested, including transfer flows (loan_borrower, loan_owner)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1362
**Filed:** 2026-04-25 | **Last activity:** 2026-04-27
**Labels:** auto-filed, review:testing | **Assignees:** unassigned
**Context docs:** none (auto-filed by `/ripls-reviews`)

## Problem Statement

The Flutter app has 67 `*_view_model.dart` files under
`app/lib/presentation/viewmodels/`. Only 47 have accompanying
`*_view_model_test.dart` files in `app/test/presentation/viewmodels/`.
Twenty ViewModels are unit-test-free, including the **transfer flow** ViewModels
that drive the core lending product:

- `loan_borrower_view_model.dart` (408 LOC) — borrower-side state machine across
  Arrange Pickup → Confirmed → In Use → Confirm Return → Returned phases,
  plus a periodic countdown timer with `ref.onDispose` cleanup.
- `loan_owner_view_model.dart` (279 LOC) — owner-side state machine across
  Request → Scheduled → On Loan → Returning → Returned, with non-trivial
  pending-request filtering (only INTEREST_EXPRESSED until a recipient is
  selected, then they're hidden).

Other notable gaps:

- `community_edit_view_model.dart` (398 LOC) — destructive: edits live community
  fields (name, description, background media) including video upload.
- `request_creation_view_model.dart` (319 LOC) — core user path: creates a
  Request via streaming AI generation.
- `splash_view_model.dart`, `gen_community_view_model.dart` — initialization /
  AI streaming paths that decide what users see on first launch.

There is currently nothing stopping a new ViewModel from landing without a
test. The issue's recommendation is to (a) backfill the highest-risk gaps
starting with the transfer flows, and (b) add a CI guard that blocks new
unpaired ViewModels going forward.

The full untested set today (20 files; the issue says "21" — counts have
drifted since the review report was written):

| # | ViewModel | LOC | Risk |
|---|-----------|-----|------|
| 1 | `loan_borrower_view_model.dart` | 408 | **High — transfer flow + timer** |
| 2 | `loan_owner_view_model.dart` | 279 | **High — transfer flow** |
| 3 | `community_edit_view_model.dart` | 398 | **High — destructive** |
| 4 | `request_creation_view_model.dart` | 319 | **High — core path + streaming** |
| 5 | `gen_community_view_model.dart` | ? | Medium — AI streaming |
| 6 | `splash_view_model.dart` | ? | Medium — boot sequencing |
| 7 | `experience_sharing_view_model.dart` | ? | Medium — share flow |
| 8 | `request_sharing_view_model.dart` | ? | Medium — share flow |
| 9 | `request_needs_view_model.dart` | ? | Medium |
| 10 | `community_actions_view_model.dart` | ? | Medium |
| 11 | `community_metric_detail_view_model.dart` | ? | Low — metric display |
| 12 | `community_utilization_view_model.dart` | ? | Low — metric display |
| 13 | `experience_metric_view_model.dart` | ? | Low — metric display |
| 14 | `request_metric_view_model.dart` | ? | Low — metric display |
| 15 | `portfolio_metric_detail_view_model.dart` | ? | Low — metric display |
| 16 | `portfolio_metrics_view_model.dart` | ? | Low — metric display |
| 17 | `portfolio_view_model.dart` | 59 | Low — nav visibility only |
| 18 | `locale_view_model.dart` | ? | Low — locale toggle |
| 19 | `time_modal_view_model.dart` | ? | Low — modal state |
| 20 | `time_to_solve_view_model.dart` | ? | Low |

Phase 1 below proves the count and locks the exact list before any tests are
written.

## Context from Comments

The two issue comments are both bare `@claude` mentions from
`maintainer` (the maintainer who triaged the issue) — the second was
likely sent because an earlier `@claude` run did not produce a plan PR. No
direction-changing commentary was added beyond the original body, which sets
the goals: backfill transfer flows first, add a CI gate for future VMs.

## Potential Solutions

### Option A: One-shot — backfill all 20 tests in a single PR, then add the CI gate

- **Approach:** Implement all 20 missing test files plus the CI gate in one
  branch. The gate would have no allowlist because there are no surviving gaps
  by the time it lands.
- **Pros:** Permanent fix in one merge; no lingering allowlist file to maintain.
- **Cons:** A ~3,000-line, 20-file diff is brutal to review. Tests for
  `loan_borrower` (timer + 5-state phase machine) deserve careful review and
  will get less of it buried in a pile of trivial nav-visibility tests. High
  conflict risk if the branch sits unmerged for more than a day.
- **Effort:** Large (estimated 1–2 weeks of test-writing for a single owner).

### Option B: Phased backfill, gate added at the end

- **Approach:** Four phases. Each phase is a shippable PR.
  - **Phase 1:** Transfer flows (`loan_borrower`, `loan_owner`).
  - **Phase 2:** Destructive / core paths (`community_edit`,
    `request_creation`, `splash`, `gen_community`).
  - **Phase 3:** Sharing & action VMs (`experience_sharing`,
    `request_sharing`, `request_needs`, `community_actions`).
  - **Phase 4:** Metric / display / trivial VMs (10 remaining files), then
    add the CI gate (no allowlist needed — by now the gap is closed).
- **Pros:** Each PR is reviewable. Highest-risk tests land first. Aligns
  exactly with the issue's recommendation order.
- **Cons:** Until Phase 4 lands, nothing prevents a new untested VM from
  slipping in. If Phase 4 stalls, the gate never ships.
- **Effort:** Medium per phase; large overall (same total work as A, just
  spread out).

### Option C: Gate first with an allowlist, then drain the allowlist

- **Approach:** **Phase 1** — add a `npm run lint:dart:viewmodel-tests`
  script (modeled on `lint:dart:size`) that fails if any
  `app/lib/presentation/viewmodels/*_view_model.dart` file lacks a paired
  test, with an explicit allowlist file
  `scripts/viewmodel_test_allowlist.txt` listing the 20 currently-missing
  VMs. Wire it into the Flutter CI workflow alongside `lint:dart:size`.
  **Phases 2–N** drain the allowlist by writing tests and removing each
  filename from it; CI fails if a removed file's test goes missing again.
- **Pros:** Prevents *new* untested VMs from landing on day 1, regardless of
  how long backfill takes. Allowlist makes the remaining debt visible and
  countable. Mirrors how this repo already handles known-bad-but-grandfathered
  state — `// dart-line-count-allow` in `check_dart_file_size.sh` is the same
  shape.
- **Cons:** Adds a small allowlist file to maintain (auto-shrinks to zero).
  Slightly more machinery upfront than Option B.
- **Effort:** Small for the gate (Phase 1, ~half a day); medium per backfill
  phase after.

## Recommendation

**Option A** (per maintainer direction in PR #1463: "please just do option A,
add all tests and the gate in one PR"). All 20 tests plus the CI gate land in
a single implementation PR. The gate ships with **no allowlist** — by the time
it merges, every `*_view_model.dart` has a paired test, so the gate is a pure
preventative from day 1.

Trade-offs accepted by this choice:

- The diff will be large (~3,000 lines across ~21 new files). To keep it
  reviewable, the implementation PR groups commits by risk tier so reviewers
  can read the high-stakes tests first: (1) transfer flows
  (`loan_borrower`, `loan_owner`), (2) destructive / core paths
  (`community_edit`, `request_creation`, `splash`, `gen_community`),
  (3) sharing / action VMs, (4) metric / display / trivial VMs, (5) the gate
  itself.
- Higher conflict risk while the branch is open. The implementation should
  rebase aggressively and not let the branch sit unmerged longer than ~2 days.
- One owner sees the whole job through; no follow-up issues to file.

## Open Questions

- **Does "ViewModel" mean only `*_view_model.dart`, or also `*_notifier.dart`
  files like `conversation_messages_notifier.dart` and
  `impact_draft_notifier.dart`?** — Recommendation: gate on
  `*_view_model.dart` only. Notifiers are colocated helpers used by other
  ViewModels and are tested transitively. If we want to expand later, it's a
  one-line script change. Keep the scope tight for the first version.
- **Should the gate also enforce that the test file actually contains tests
  (e.g. `void main()` with at least one `test(...)` call)?** —
  Recommendation: no, not in v1. File presence is enough to start;
  empty test files would be flagged in code review and `flutter test` will
  silently pass them. Re-evaluate if we see empty stubs being merged.
- **Should `portfolio_view_model.dart` (59 LOC, only nav visibility) really be
  tested?** — Recommendation: yes, but with a single-file low-effort test
  (`showNav` / `hideNav` / `toggleNav` round trip). The test is ~30 lines.
  Don't make exemptions just because something is small.
- **CI gate name?** — Recommendation:
  `npm run lint:dart:viewmodel-tests` (matches the
  `lint:dart:<thing>` family of scripts and slots into the existing
  `npm run lint` aggregator).

## Implementation Plan

This plan delivers all 20 missing tests **and** the CI gate in a single PR.
Work is split into five sequential commits within that PR so reviewers can
read the high-stakes tests first; do not split into multiple PRs. The
gate ships with no allowlist.

Conventions every test file in this PR must follow:

- Mirror the structure of an existing canonical sibling test (the closest
  cousin already in `app/test/presentation/viewmodels/`). Use Riverpod's
  `ProviderContainer` with provider overrides; mock repositories with
  `@GenerateMocks` from Mockito.
- Notifiers using `SafeNotifierMixin` log a warning on post-disposal
  `safeUpdateState`; tests rely on that contract per
  `docs/client/architecture.md` §"Async Safety in Notifiers" — do not
  catch exceptions for disposal.
- ViewModel tests need **no** `MaterialApp` localization setup
  (`docs/client/i18n.md`: "Viewmodel tests using `ProviderContainer` need no
  localization setup"). Error strings the VMs surface (e.g.
  `errorMessage: 'Transfer not found'`) are matched as raw strings.
- Each new test file targets the 90 %+ ViewModel coverage bar from
  `docs/client/testing.md` §"Test Coverage Goals". Verify with
  `cd app && flutter test --coverage` and spot-check
  `coverage/lcov.info` for the new files before opening the PR.
- Generate mocks with the exact command CI uses
  (`.github/workflows/test_flutter.yaml`):
  `cd app && flutter pub run build_runner build --delete-conflicting-outputs`.
  This produces `*_test.mocks.dart` siblings.

### Tier 1: Transfer-flow ViewModels (commit 1)

**Goal:** Pin the two highest-risk state machines.

- [ ] **`loan_borrower_view_model_test.dart`** at
  `app/test/presentation/viewmodels/`. Mirror the structure of
  `giveaway_giver_view_model_test.dart` (mocks `TransferRepository` via
  `@GenerateMocks`, overrides `transferRepositoryProvider`). Test groups:
    - `initialize`:
        - phase = `arrangePickup` when transfer is `INTEREST_EXPRESSED`.
        - phase = `confirmed` when `RECIPIENT_SELECTED` *and* both
          `estimatedPickupUnixSec` and `loanDurationDays` are set.
        - phase = `arrangePickup` when `RECIPIENT_SELECTED` but pickup
          details unset (regression guard for the
          `_determinePhase` branch on line 396).
        - phase = `inUse` when `ACTIVE`; verify countdown timer started by
          asserting `state.countdownDays/Hours/Minutes` populated for a
          known `actualPickupUnixSec` + `loanDurationDays`.
        - phase = `returned` when `COMPLETED`.
        - error path: repository throws → `errorMessage` set,
          `isLoading` false, no countdown.
        - missing transfer (repo returns null) → `errorMessage = "Transfer
          not found"`.
    - `setPickupDate` / `setPickupTime` / `setDuration`:
        - simple state mutations propagate.
    - `submitPickupDetails`:
        - early-returns when `transfer == null` (no repo call).
        - early-returns when date or time unset (no repo call).
        - happy path: calls `updateTransfer` with UTC-combined epoch seconds
          and the selected duration; reloads transfer; updates phase.
        - error path: sets `errorMessage`.
        - reload returning null: sets `errorMessage = "Failed to reload
          transfer after update"`.
    - `markPickedUp`:
        - happy path: returns `communityEventId` from `startLoan`,
          transitions to `inUse`, starts countdown.
        - early return when transfer null.
        - error path: returns null, sets `errorMessage`.
    - `confirmReturn`:
        - happy path: returns `communityEventId`, cancels countdown timer,
          phase becomes `returned`.
        - early return when transfer null.
        - error path: returns null, sets `errorMessage`.
    - `withdrawInterest`:
        - happy path: clears transfer from state, cancels timer.
        - early return when transfer null.
        - error path: sets `errorMessage`.
    - `disposal safety` (named exactly like the canonical example in
      `docs/client/architecture.md`):
        - start `initialize` with a transfer that puts the VM into the
          `inUse` phase (so the periodic countdown timer is running),
          then call `container.dispose()` mid-flight. The pending future
          should `complete` without throwing.
        - regression guard for the `ref.onDispose` block at lines 41–44:
          after disposal, additional `setPickupDate`/etc. mutations must
          not crash and must not mutate observable state.
- [ ] **`loan_owner_view_model_test.dart`**:
    - `initialize`:
        - filters transfers to the requested `gearId` only.
        - separates active from `INTEREST_EXPRESSED` pending requests.
        - when `transferId` is provided, that transfer wins as `activeTransfer`
          even if other gear transfers exist.
        - active transfer skips `CANCELLED` and `COMPLETED`.
        - once an active transfer exists, `INTEREST_EXPRESSED` transfers are
          *not* added to `pendingRequests` (regression guard for lines 73–88).
        - empty case: no transfers → `request` phase, empty pending.
        - error path.
    - `selectRecipient`: clears `pendingRequests`, transitions to
      `scheduled`, error path.
    - `startLoan`: transitions to `onLoan`, error path.
    - `completeLoan`: transitions to `returned`, error path.
    - `cancelTransfer`: clears `activeTransfer`, error path.
    - `_determinePhase`: implicitly covered by the above; add one
      explicit test for the default branch (unknown state →
      `LoanOwnerPhase.request`).

### Tier 2: Destructive / core-path ViewModels (commit 2)

**Goal:** Cover destructive edits and the core user paths.

- [ ] **`community_edit_view_model_test.dart`** — destructive: edits live
  community fields including video upload. Cover field mutations
  (name/description), media upload (mock `MediaRepository`), submit happy +
  error paths, and disposal safety.
- [ ] **`request_creation_view_model_test.dart`** — streaming AI generation.
  Mirror `gen_request_view_model_test.dart` for `GenStreamController` fakes.
  Cover stream emit → state, stream error → `errorMessage`, mid-stream
  disposal completes without throwing.
- [ ] **`splash_view_model_test.dart`** — boot sequencing. Cover the routing
  decision matrix: unauthenticated → login, authenticated + onboarded →
  home, authenticated + not onboarded → onboarding, error → error state.
- [ ] **`gen_community_view_model_test.dart`** — AI streaming for community
  generation. Same `GenStreamController` fake pattern as
  `request_creation`. Cover stream lifecycle and disposal.

### Tier 3: Sharing / action ViewModels (commit 3)

**Goal:** Cover the share and action flows.

- [ ] **`experience_sharing_view_model_test.dart`** — share flow state.
- [ ] **`request_sharing_view_model_test.dart`** — share flow state.
- [ ] **`request_needs_view_model_test.dart`** — needs editing.
- [ ] **`community_actions_view_model_test.dart`** — community action menu.

For each: cover initialize, every public mutator, error paths, and disposal
safety.

### Tier 4: Metric / display / trivial ViewModels (commit 4)

**Goal:** Cover the remaining 10 lower-risk VMs. These tests are short
(often <50 lines) but required for the gate to pass with no allowlist.

- [ ] `community_metric_detail_view_model_test.dart`
- [ ] `community_utilization_view_model_test.dart`
- [ ] `experience_metric_view_model_test.dart`
- [ ] `request_metric_view_model_test.dart`
- [ ] `portfolio_metric_detail_view_model_test.dart`
- [ ] `portfolio_metrics_view_model_test.dart`
- [ ] `portfolio_view_model_test.dart` — `showNav` / `hideNav` /
  `toggleNav` round trip (~30 lines).
- [ ] `locale_view_model_test.dart` — locale toggle round trip and
  persistence to whatever store backs it.
- [ ] `time_modal_view_model_test.dart` — modal state open/close.
- [ ] `time_to_solve_view_model_test.dart` — value-update round trip.

For each: cover initialize, every public mutator, and error paths. Disposal
safety only where the VM holds async work or timers (most of these don't).

### Tier 5: CI gate (commit 5, last)

**Goal:** Lock the achievement in. With every VM now paired, the gate
ships **without an allowlist** — any new untested VM fails CI immediately.

- [ ] Create `scripts/check_viewmodel_tests.sh` modeled on
  `scripts/check_dart_file_size.sh`. It must:
    - find every `app/lib/presentation/viewmodels/*_view_model.dart` (excluding
      `*.freezed.dart`).
    - for each, check that
      `app/test/presentation/viewmodels/<basename>_test.dart` exists.
    - print every failing file as `FAIL: <relpath> has no test at <expected
      path>`, then exit 1 if any failed.
    - reference issue #1362 in a top-of-file comment so future readers can
      find the rationale.
- [ ] Add to `package.json`:
    - `"lint:dart:viewmodel-tests": "bash scripts/check_viewmodel_tests.sh"`
    - extend the aggregate `lint` script to include
      `&& npm run lint:dart:viewmodel-tests`.
- [ ] Add a step to `.github/workflows/test_flutter.yaml` named
  `Check ViewModel test coverage (issue #1362)` immediately after the existing
  `Check Dart file sizes` step, running `npm run lint:dart:viewmodel-tests`.
- [ ] Run `npm run lint:dart:viewmodel-tests` locally; expect it to pass —
  every VM now has a paired test.
- [ ] Verify the failure path by temporarily creating
  `app/lib/presentation/viewmodels/zzz_test_view_model.dart` (a 1-line stub),
  running the lint, confirming the failure message, then deleting the stub.
- [ ] Do **not** create `scripts/viewmodel_test_allowlist.txt`. The gate
  works without one. If a future VM is genuinely untestable, the right move
  is to file a separate issue and add the allowlist mechanism then — not to
  pre-build it.

### Final validation (before opening PR)

- [ ] `cd app && flutter test --reporter expanded` — full suite green,
  including all 20 new files.
- [ ] `cd app && flutter test --coverage` — spot-check `coverage/lcov.info`
  for each new file; transfer-flow and core-path VMs hit the 90 %+ bar.
- [ ] `npm run lint:dart:viewmodel-tests` — passes with no allowlist.
- [ ] `npm run lint` — full lint aggregate passes.

**Validates:** Every `*_view_model.dart` is paired with a test, the gate
prevents regressions starting on merge day, and the issue closes with no
follow-up backfill work outstanding.

## Related Issues

- **#1324, #1293** — referenced by the existing `Check Dart file sizes` CI
  step. These issues established the precedent for codebase-wide hygiene gates
  enforced via small bash scripts checked into `scripts/`. This plan follows
  the same pattern intentionally.
- **`docs/reviews/reports/testing_review.md`** — the source review that filed
  this issue. The "21 ViewModels untested" finding is one of several testing
  gaps; other findings from that review may be filed as their own issues.

## Audit Notes

**Refinement 2026-04-27** — flipped the recommendation from Option C
(gate-first with allowlist + phased backfill) to **Option A** (all 20
tests + the gate in a single PR, no allowlist) per maintainer direction
on PR #1463. The Implementation Plan now lays out five sequential commits
inside one PR, ordered by risk so reviewers can read the high-stakes
tests first. Removed the allowlist-location open question and the
"Phases 3–5 as follow-up issues" section since they no longer apply.

`/ripls-audit` (2026-04-27) — earlier audit pass against
`docs/client/architecture.md` and `docs/client/testing.md`:

- Renamed the `dispose` test group to `disposal safety` and rewrote it to
  match the canonical pattern in `architecture.md` §"Disposal Testing"
  (start async work, call `container.dispose()`, expect future to
  `complete`). The previous wording was vague.
- Called out that `loan_borrower_view_model.dart` uses `SafeNotifierMixin`,
  so post-disposal mutation calls log warnings rather than throwing; tests
  must rely on this contract, not catch exceptions.
- Pinned the `build_runner` invocation to the exact command used by
  `.github/workflows/test_flutter.yaml` (`flutter pub run build_runner
  build --delete-conflicting-outputs`) to keep local and CI mock
  generation aligned.
- Added the 90 %+ ViewModel coverage target from
  `docs/client/testing.md` §"Test Coverage Goals" as the validation
  criterion for the transfer-flow tests (now applied to all tiers).
- Noted the `i18n.md` rule that `ProviderContainer`-based ViewModel tests
  need no localization setup — relevant because `loan_borrower` /
  `loan_owner` surface error messages as raw strings (see
  `errorMessage: 'Transfer not found'`), and the test-writer should
  resist the urge to localize them or wrap tests in `MaterialApp`.
