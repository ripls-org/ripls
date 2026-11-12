# Issue #2005: Ways to keep going click goes to old create

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2005
**Co-fixes:** https://github.com/Type2Fun-Foundation/ripls/issues/1993 (nudges open legacy create modal — same root cause)
**Filed:** 2026-05-17 | **Last activity:** 2026-05-18
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** `docs/design/unified-create.md`

## Audit log

- 2026-05-17 — `ripls-audit` pass. Findings applied:
  - Flagged `Environment.unifiedCreateEnabled` as a compile-time const
    that cannot be toggled in tests; introduced a Phase 1 step to wrap
    it in a `Provider<bool>` so widget tests can override via
    `ProviderScope.overrides`. This follows the Provider-injection
    pattern in `docs/client/architecture.md` § Providers and removes
    the need for `--dart-define` in tests.
  - Re-framed test framing from "either ViewModel or widget" to
    explicitly **widget-level**, citing the
    `docs/client/architecture.md` § Widget Async Exceptions list —
    dispatcher-style dialog invocation is the canonical "UI-level,
    awkward in a ViewModel" case.
  - Added an explicit "no a11y / l10n / caching changes needed"
    note to confirm compliance with `docs/client/i18n.md` and
    `docs/client/accessibility.md` (both target modals already
    localize and label their own semantics).
- 2026-05-18 — Scope expansion per reviewer (@briangbrown). Switched
  the recommendation from **Option A** (Workshop fallbacks only) to
  **Option C** (every secondary creation surface). Reasoning:
  - Reviewer also wants #1993 (nudges) co-fixed, which shares the
    exact same root cause — legacy modal calls that ignore the flag.
  - An exhaustive `grep` of `app/lib/` turned up **21 production
    call sites** across 5 files. All five exhibit the same
    flag-blindness bug. Fixing one surface and filing follow-ups for
    the rest leaves four user-visible regressions on PROD where the
    user has already opted into the unified flow.
  - The Option C "cons" listed in the original plan (mostly:
    result-type mismatch on a few callers) are tractable. They were
    the reason to defer; they are not the reason to never do it.

## Problem Statement

When `ENABLE_UNIFIED_CREATE=true` is set (the value in
`app/env.prod.json:7`, so every PROD build), the user has opted into a
**single unified Create surface** — `UnifiedCreateModal` — per the
rollout in `docs/design/unified-create.md` § Goals 1 and § Feature
flag. Today only the home `+` button at
`app/lib/presentation/screens/home/home_screen.dart:78` respects that
flag. Every other entry point still calls one of the legacy per-type
modals directly:

- `ExperienceCreationModal.show(context)` — legacy single-type Event
  create.
- `CreateGearModal.show(context)` — legacy single-type gear create.
- `RequestCreationModal.show(context)` — legacy single-type request
  create.

The user-visible result is exactly what #2005 and #1993 describe: the
user taps a card that says "share gear" / "create event" / "ask for
help" / "three ways to keep it going" and lands in the **old**
single-type modal, contradicting the unified-create promise.

**Exhaustive audit (21 production call sites, 5 files):**

| # | File | Lines | Surface |
|---|------|-------|---------|
| 1 | `screens/home/home_screen.dart` | 98, 105, 130 | `PlusButtonModal` legacy branch (flag-off path) |
| 2 | `widgets/empty_content_state.dart` | 49, 52, 64 | Empty content state CTAs |
| 3 | `screens/impact_metrics/widgets/community_metrics_new_stage.dart` | 167, 183, 187, 399, 407 | Community metrics empty / listed stage CTAs |
| 4 | `widgets/nudge/nudge_content_view.dart` | 101, 103, 124, 128, 148, 158, 163 | Nudge CTAs and workshop-draft fallbacks (#1993) |
| 5 | `widgets/workshop/workshop_lever_dispatcher.dart` | 28, 37, 64 | Workshop "Three ways to keep it going" fallbacks (#2005 specifically) |

Three of the 21 sites consume a typed result the unified modal does
not produce — those need explicit handling, not just a swap:

- `home_screen.dart:105` (`ExperienceCreationModal` → checks
  `ExperienceCreationResult.isPastEvent` to chain
  `MarkCompletedModal`).
- `home_screen.dart:130` and `empty_content_state.dart:64`
  (`RequestCreationModal` → checks `requestId != null` to call
  `postCreationServiceProvider.handlePostCreation()`).

`UnifiedCreateModal.show` returns `Future<bool?>` (`true` = something
was shared). The `handlePostCreation` follow-up has a clean
translation: trigger it when the returned bool is `true`. The
`MarkCompletedModal` past-event follow-up does **not** have a clean
translation because the unified modal cannot today flag a past-event
Experience. The plan handles that explicitly (see Open Questions).

## Context from Comments

- **#2005** (filed by @briangbrown, COLLABORATOR): one
  `@claude plan` trigger; no maintainer direction in the thread
  beyond "should go to the new unified create."
- **#1993** (filed by user10@example.org via in-app feedback, marked by
  @briangbrown as "similar to #2005"): same root cause — nudge stories
  open the old modal. Co-fixed here.
- **PR #2007 review comment** (@briangbrown, 2026-05-18): "Change the
  plan to incorporate #1993 the nudges creation flow to also use the
  unified creation flow. Do an exhaustive search of the codebase and
  make sure all old creation flows go to the unified creation flow
  when the feature flag is turned on." Drives the scope expansion in
  this refinement.

Reporter on #2005 is on PROD, v0.21.1, Android 16, Pixel 8, build
6979, which is the released app version where
`ENABLE_UNIFIED_CREATE=true` is live (`app/env.prod.json:7`).

## Potential Solutions

### Option A: Redirect only Workshop fallback paths (original)

Fix `workshop_lever_dispatcher.dart` (3 sites). Minimal change, ~1
hour, but leaves four other surfaces flag-blind on PROD and ignores
#1993. **Not chosen** — the reviewer explicitly asked for an
exhaustive migration.

### Option B: Always route Workshop postcards through UnifiedCreateModal (original)

Adds a new `seedFromWorkshopDraft` API to `UnifiedCreateViewModel`
and a force-type preview transition. Medium effort, design call
required, doesn't address #1993. **Not chosen** — the seeding API is
a separate, larger piece of work and is not needed to fix either
user-reported issue.

### Option C: Migrate all secondary creation surfaces under the flag (chosen)

For every legacy `ExperienceCreationModal.show` /
`CreateGearModal.show` / `RequestCreationModal.show` call in
`app/lib/`, gate the call on the new `unifiedCreateEnabledProvider`:
when the flag is on, route through `UnifiedCreateModal.show(context,
ref)`; when off, fall back to the existing legacy modal so the
rollback path the rollout doc requires keeps working. The three
result-consuming call sites translate their post-modal follow-up to
match the unified modal's `Future<bool?>` shape (or, where there's no
translation, are tracked in Open Questions). Estimated effort:
**1.5–2 days** including the shared helper, all 21 call-site swaps,
widget tests for each affected file, and manual QA across every
changed surface.

## Recommendation

**Option C.** The reviewer's directive in
[PR #2007 comment](https://github.com/Type2Fun-Foundation/ripls/pull/2007#issuecomment-4478691594)
is unambiguous: every legacy creation flow should route through
`UnifiedCreateModal` when the flag is on. The cost of doing this in
one PR is moderate (1.5–2 days), the cost of *not* doing it is that
PROD users with the flag on continue to hit the legacy modal from 4
of 5 surfaces — including the surface that prompted #1993. Issue
#1993 is closed by the same change set, so two user-feedback reports
get resolved together.

To keep the blast radius contained we use **one shared helper**
(`openBlankCreate(context, ref)`) and **one shared provider**
(`unifiedCreateEnabledProvider`) so every site has identical
branching semantics. Adding a new surface in the future means one
function call, not a fresh flag check. The success paths that
legitimately *want* a per-type prefilled preview
(`workshop_lever_dispatcher.dart` happy path → `ExperiencePreviewModal`
with seeded draft; nudge `schedule_repeat` / `revive_experience` →
`_openWorkshopDraft` → seeded preview) are **preserved** — only the
"blank create" fallback branches inside those flows route to the
unified modal.

The two known result-type mismatches are addressed inline:

- `handlePostCreation` follow-up on success — translated by checking
  the unified modal's `Future<bool?>` return for `true` (same pattern
  already in use at `home_screen.dart:83`).
- `MarkCompletedModal` past-event follow-up — surfaced as a follow-up
  design issue rather than blocked, because (a) it is unreachable
  today when the flag is on (line 105's `PlusButtonModal` legacy
  branch only runs when the flag is off, so the user-visible
  contract is unchanged), and (b) hooking past-event detection into
  the unified flow is a real product question that belongs in its
  own design loop.

## Open Questions

- **What flag-off behaviour do we keep?** — Recommendation: **No
  change.** When `unifiedCreateEnabled` is false (read through the
  new provider — see below), every dispatch path behaves exactly as
  it does today. That preserves the rollback path the unified-create
  rollout doc requires.
- **How do we make the flag overridable in tests?** —
  Recommendation: **Introduce `Provider<bool>
  unifiedCreateEnabledProvider`** in `app/lib/core/config/`
  whose default reads `Environment.unifiedCreateEnabled`.
  `Environment.unifiedCreateEnabled` itself is a
  `const String.fromEnvironment` — a **compile-time const**, so
  widget / ViewModel tests cannot toggle it via `setUp()`. Wrapping
  it in a Riverpod `Provider` lets tests override the value through
  `ProviderScope.overrides`, matching the Provider-injection pattern
  documented in `docs/client/architecture.md` § Providers. The
  Provider also leaves room for a future runtime kill-switch (e.g.,
  from server config) without touching call sites again. Every flag
  check in the migrated files reads
  `ref.read(unifiedCreateEnabledProvider)` instead of
  `Environment.unifiedCreateEnabled`. The static getter stays for
  non-Riverpod call sites (e.g. `main.dart` bootstrapping).
- **Where does the shared helper live?** — Recommendation: a top-level
  `openBlankCreate(BuildContext context, WidgetRef ref)` in a new file
  `app/lib/presentation/screens/create/blank_create_dispatcher.dart`.
  Top-level (not a static method on a class) so it can be imported
  with one line and called inline. Co-located with
  `unified_create_modal.dart` because it is conceptually a thin
  router into that modal. The helper does the
  `ref.read(unifiedCreateEnabledProvider)` check and dispatches to
  either `UnifiedCreateModal.show(context, ref)` or — when the legacy
  modal type matters — to a per-type helper. We need three thin
  variants because the legacy modals are not interchangeable:
    - `openBlankCreate` (default — for Experience-flavoured "create"
      affordances; flag-off path opens `ExperienceCreationModal`).
    - `openBlankCreateGear` (flag-off path opens `CreateGearModal`).
    - `openBlankCreateRequest` (flag-off path opens
      `RequestCreationModal`; returns `Future<String?>` to preserve
      the requestId callers consume — flag-on path returns the
      success bool translated to a non-null sentinel so callers can
      branch identically).
  All three accept `(context, ref)` and read the same provider.
- **How do we translate the typed-result callers?** — Recommendation:
    - `home_screen.dart:130` and `empty_content_state.dart:64`
      (`RequestCreationModal` → `handlePostCreation`): call
      `openBlankCreateRequest` and run `handlePostCreation` when
      either (a) flag-on and the returned bool is `true`, or (b)
      flag-off and the returned requestId is non-null. The helper
      hides the branching from the caller.
    - `home_screen.dart:105` (`ExperienceCreationModal` →
      `MarkCompletedModal` for past events): **leave the existing
      legacy path untouched** for the flag-off branch (it is already
      flag-off because lines 78–89 gate the unified flow above it),
      and do **not** attempt to detect past-event Experiences from
      the unified modal in this PR. Document in the follow-up.
- **Should the Workshop / Nudge success paths also funnel through
  UnifiedCreateModal?** — Recommendation: **No, not in this PR.**
  The success paths
  (`workshop_lever_dispatcher.dart` → `ExperiencePreviewModal` with
  seeded draft; nudge `schedule_repeat` /
  `revive_experience` / `propose_share` → `_openWorkshopDraft`)
  serve a server-generated draft to the user one tap from a
  recommendation. That is the intentional payoff and not what users
  reported. Funnelling them through the unified modal requires
  Option B's seeding extension on `UnifiedCreateViewModel`. Capture
  as a separate design issue (see Phase 5).
- **Test framing — ViewModel-level or widget-level?** —
  Recommendation: **Widget-level**, per `docs/client/architecture.md`
  § Widget Async Exceptions. The dispatcher / view paths under change
  all call `showDialog` / `showAccessibleModal`, which requires a
  real `BuildContext` — one of the documented Widget Async Exceptions.

## Implementation Plan

### Phase 1: Add `unifiedCreateEnabledProvider`

**Goal:** Make the unified-create flag readable through Riverpod so
every call site (and tests) can consume it via `ref.read` /
`ProviderScope.overrides`. The `Environment.unifiedCreateEnabled`
getter is a `const String.fromEnvironment(...)` — a **compile-time
const**, not runtime-overridable — which is what makes the existing
flag impossible to toggle from a widget test today.

**Tasks:**
- [ ] Add `app/lib/core/config/feature_flags.dart` exporting a single
  top-level `Provider<bool> unifiedCreateEnabledProvider` whose body
  returns `Environment.unifiedCreateEnabled`. Keep
  `Environment.unifiedCreateEnabled` itself — the provider is a thin
  wrapper, not a replacement, so non-Riverpod code paths (`main.dart`
  bootstrap) can still read the static getter.
- [ ] Ensure `app/lib/core/config/` has an up-to-date `README.md`
  covering the new file (per the CLAUDE.md "every multi-file
  directory needs a README" rule).
- [ ] Migrate the existing `home_screen.dart:78`
  `Environment.unifiedCreateEnabled` read to
  `ref.watch(unifiedCreateEnabledProvider)` for consistency. Behaviour
  unchanged; read source moves.

**Validates:** Provider compiles; home screen still opens
`UnifiedCreateModal` on PROD-flag-on builds. No behavioural change in
this phase.

### Phase 2: Add the shared `openBlankCreate*` dispatcher

**Goal:** One place that every secondary surface routes through, so
the branching semantics are identical everywhere.

**Tasks:**
- [ ] Create
  `app/lib/presentation/screens/create/blank_create_dispatcher.dart`
  exporting three top-level functions:
  ```dart
  Future<void> openBlankCreate(BuildContext context, WidgetRef ref);
  Future<void> openBlankCreateGear(BuildContext context, WidgetRef ref);
  Future<String?> openBlankCreateRequest(BuildContext context, WidgetRef ref);
  ```
  Each reads `ref.read(unifiedCreateEnabledProvider)` and dispatches
  to either `UnifiedCreateModal.show(context, ref)` (flag on) or the
  type-specific legacy modal (flag off). For
  `openBlankCreateRequest`, the flag-on path translates the unified
  modal's `Future<bool?>` to a `Future<String?>` using a sentinel
  non-empty string (e.g. `'unified-create:shared'`) when `true`,
  `null` otherwise — callers only branch on null vs. non-null today
  (see `home_screen.dart:133`, `empty_content_state.dart:66`), so the
  sentinel preserves their semantics without leaking the unified
  modal's internal shape.
- [ ] Document the sentinel in a function-level dartdoc on
  `openBlankCreateRequest`. Single-line comment is acceptable —
  CLAUDE.md prohibits multi-paragraph comment blocks.
- [ ] Unit-test the dispatcher in
  `app/test/presentation/screens/create/blank_create_dispatcher_test.dart`:
  three cases per helper (flag on opens unified, flag off opens
  legacy, sentinel translation works for `openBlankCreateRequest`).

**Validates:** Helpers compile, tests pass, no production call sites
yet wired up.

### Phase 3: Migrate every legacy call site

**Goal:** All 21 production call sites enumerated in the Problem
Statement use the shared helper. Result-consuming callers are
translated.

**Per-file task list:**

- [ ] **`screens/home/home_screen.dart`** (3 sites, lines 98 / 105 /
  130). These sit inside the `PlusButtonModal` branch, which only
  runs when the flag is **off** (lines 78–89 already gate the unified
  flow). For internal consistency, swap the bodies to call the
  shared helpers anyway — when flag is off the helpers behave
  identically to a direct call, and the indirection means future
  maintainers don't accidentally re-introduce a flag bypass.
- [ ] **`widgets/empty_content_state.dart`** (3 sites, lines 49 / 52
  / 64). Same `PlusButtonModal` shape, currently **not** gated.
  Swap to helpers. The `onRequestSomething` callback (line 64)
  becomes `final requestId = await openBlankCreateRequest(context,
  ref);` and keeps the existing
  `if (requestId != null && context.mounted) handlePostCreation()`
  branch — the sentinel makes the existing condition correct under
  both flag states.
- [ ] **`screens/impact_metrics/widgets/community_metrics_new_stage.dart`**
  (5 sites, lines 167 / 183 / 187 / 399 / 407). The two classes in
  this file (`CommunityMetricsNewStage`, `CommunityMetricsListedStage`)
  are `StatelessWidget`s that already receive `WidgetRef ref` as a
  constructor field — pass `this.ref` into each helper call. Update
  the three helper methods (`_openAddItemModal`,
  `_openAddEventModal`, `_openAddRequestModal`) to take `WidgetRef`
  alongside `BuildContext`, and the two inline `onTap` callbacks at
  lines 399 / 407 to call the helpers with the captured `ref` field.
- [ ] **`widgets/nudge/nudge_content_view.dart`** (7 sites — #1993).
  Replace each direct legacy call:
  - line 101 (`'list_item'` →
    `openBlankCreateGear(context, ref)`).
  - line 103 (`'ask_for_help'` →
    `openBlankCreateRequest(context, ref)`; nudge code today does
    not consume the result, so the sentinel is harmless).
  - line 124 (`'seed_subhost'` fallback →
    `openBlankCreate(context, ref)`).
  - line 128 (default / `'plan_experience'` →
    `openBlankCreate(context, ref)`).
  - lines 148, 158, 163 (the three `_openWorkshopDraft` fallback
    branches — empty contextId, RPC failure, empty draft.name) →
    `openBlankCreate(context, ref)`. The happy path that seeds and
    opens `ExperiencePreviewModal` is **unchanged**.
- [ ] **`widgets/workshop/workshop_lever_dispatcher.dart`** (3 sites,
  lines 28 / 37 / 64). Replace each direct
  `ExperienceCreationModal.show(context)` call with
  `openBlankCreate(context, ref)`. The dispatcher already takes
  `WidgetRef ref`, so no signature change. The happy path
  (`generateDraft` returns a usable draft → seed
  `genExperienceProvider` → open `ExperiencePreviewModal`) is
  **unchanged** — only the three blank-create fallbacks shift.

**Note on a11y / l10n / caching:** No new ARB strings. Both the
unified and legacy modals already localize their own copy via
`context.l10n` and manage their own semantics labels and `Tappable`
wrappers. No cache key changes. No `MediaRepository` /
`CachedMediaImage` paths are touched. Navigation continues to flow
through `showAccessibleModal` (used internally by every modal
involved).

**Note on `context.mounted` guards:** Every site that awaits a modal
already has the correct `context.mounted` guard after the await. The
shared helpers do no extra async work beyond the awaited
`Modal.show`, so guard placement does not shift.

**Validates:** Manual QA on PROD-flag-on dev build of each surface:
- Workshop tab → tap a Keep-It-Going postcard whose draft RPC fails
  / returns empty → expect `UnifiedCreateModal`.
- Nudge story (#1993) → tap each CTA variant (`list_item`,
  `ask_for_help`, `seed_subhost`, `plan_experience`) → expect
  `UnifiedCreateModal` for each.
- Empty content state → tap `+` → tap each of the three CTAs →
  expect `UnifiedCreateModal`.
- Community metrics new stage / listed stage → tap each CTA →
  expect `UnifiedCreateModal`.
- Toggle the flag off via local override → every surface above
  opens its respective legacy modal as before.

### Phase 4: Widget tests for each migrated surface

**Goal:** Lock the new behaviour with widget tests so a future
refactor cannot regress any of the 21 sites silently. The
architecture doc classifies dispatcher- and modal-launch widgets as
documented "Widget Async Exceptions"
(`docs/client/architecture.md` § Widget Async Exceptions).

**Tasks:**
- [ ] `app/test/presentation/widgets/workshop/workshop_lever_dispatcher_test.dart` —
  pump a minimal `MaterialApp` + `ProviderScope` with overrides for
  `workshopRepositoryProvider` (fake) and
  `unifiedCreateEnabledProvider` (`true` / `false`). Cover all three
  branches × two flag states (6 expectations).
- [ ] `app/test/presentation/widgets/nudge/nudge_content_view_modal_test.dart` —
  pump the nudge view with each of the four CTA action strings
  (`list_item`, `ask_for_help`, `seed_subhost`, `plan_experience`)
  × two flag states (8 expectations). Plus a separate test for the
  three `_openWorkshopDraft` fallback branches × two flag states
  (6 more expectations). Total 14.
- [ ] `app/test/presentation/widgets/empty_content_state_test.dart` —
  extend the existing file. Cover the three `PlusButtonModal`
  callbacks × two flag states (6 expectations). Verify the
  sentinel translation for `onRequestSomething` triggers
  `handlePostCreation` under both flag states.
- [ ] `app/test/presentation/screens/impact_metrics/widgets/community_metrics_new_stage_test.dart` —
  cover all five CTAs × two flag states (10 expectations).
- [ ] `app/test/presentation/screens/home/home_screen_create_test.dart` —
  cover the three `PlusButtonModal` callbacks × the **flag-off**
  state only (the flag-on state is already covered by the existing
  `home_screen.dart:78` unified path, which has its own test or
  should grow one as part of this PR if missing). 3 expectations.
- [ ] Every new test uses `app/test/helpers/l10n_helpers.dart` for
  `MaterialApp` localizations setup, hand-rolled fakes (no Mockito
  for single-method providers — keep tests self-contained), and
  `find.byType(UnifiedCreateModal)` / `find.byType(...LegacyModal)`
  for assertions.
- [ ] Run `cd app && npm run test:app` and the targeted suites with
  `--reporter expanded` to confirm no regressions before pushing.

**Validates:** CI catches any future change that re-routes any
secondary surface back to a legacy modal when the flag is on.

### Phase 5: Follow-up issue for the success-path migration

**Goal:** Capture the deferred design call so it doesn't get
forgotten.

**Tasks:**
- [ ] Open a GitHub issue titled "Funnel Workshop and Nudge
  prefilled-draft success paths through `UnifiedCreateModal`",
  linking #2005 and #1993. Note that this requires a new
  `seedFromWorkshopDraft` API on `UnifiedCreateViewModel` and a
  force-type preview transition (Option B from the original plan).
  Call out the open product question: is the prefilled per-type
  Event preview the intentional "Ways to keep it going" payoff
  (current behaviour) or should it funnel through the unified modal?
- [ ] Open a GitHub issue titled "Hook past-event Experience
  detection into `UnifiedCreateModal` (chain `MarkCompletedModal`)",
  linking #2005. This is the
  `home_screen.dart:105 → MarkCompletedModal` path that this PR
  intentionally leaves on the legacy branch.

**Validates:** Both follow-up issues exist and are linked from the
#2005 PR description so reviewers see the explicit scope decision.

## Related Issues

- **#1993** — Nudge story opens old create modal. **Co-fixed by
  this PR**: every legacy modal call in
  `widgets/nudge/nudge_content_view.dart` routes through the new
  helper.
- #1980 — Unified-create dismiss behavior. Adjacent surface; informs
  the dismiss expectations for the modal we are now routing more
  surfaces into.
- #1933 — MediaQuery / keyboard inset fix on `UnifiedCreateModal`
  bottom sheet. Confirms the modal is the canonical create entry
  point.
- #1996 — User profile self-view items. Adjacent rollout that
  surfaces the parallel "Three ways to keep it going" section on
  user profile, whose dispatcher (`UserProfileLeverDispatcher`)
  routes to entity detail screens, not creation modals — out of
  scope for this fix.
- Design doc: `docs/design/unified-create.md` § Goals 1 ("One entry
  point") and § Feature flag.
