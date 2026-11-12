# Issue #1802: design: migrate remaining bottom-sheet modals to glass surface

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1802
**Filed:** 2026-05-09 | **Last activity:** 2026-05-09
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** [docs/issues/1797-glass-modal-revamp.md](1797-glass-modal-revamp.md), [app/lib/presentation/widgets/modal/glass/README.md](../../app/lib/presentation/widgets/modal/glass/README.md)

## Audit notes (2026-05-09)

Audited against [docs/client/architecture.md](../client/architecture.md), [docs/client/i18n.md](../client/i18n.md), [docs/client/accessibility.md](../client/accessibility.md), [docs/client/testing.md](../client/testing.md), [docs/client/caching.md](../client/caching.md), [docs/server/architecture.md](../server/architecture.md), [docs/server/observability.md](../server/observability.md). Findings folded into the plan body:

- The white-on-glass surface is an **already-blessed deviation** from the
  "all colors must be theme-aware" rule in `architecture.md` §"Colors and
  Theming" — the parent #1797 plan documents the deviation in
  `docs/client/design.md`. This issue introduces no *new* hardcoded
  colors: every new color comes from `AppColors.modal*` and every new
  geometry constant from `ModalTheme`. (Affirmed in Phase 0 and the
  acceptance criteria.)
- New primitive `GlassInsetCard` (Phase 2) follows the
  accessibility-primitives contract from `accessibility.md`: no raw
  `GestureDetector` / `InkWell`, `Tappable` if interactive, no
  `BackdropFilter` (flat alpha fill), and a golden test alongside the
  widget per `testing.md` §Visual Testing. (Affirmed in Phase 2.)
- New ARB keys for `semanticsLabel` / `tooltip` strings use the
  **`a11y` prefix** per `accessibility.md` §"Writing good accessibility
  labels" and `i18n.md` §"Key Naming Convention". User-visible kicker
  and field-label strings use the relevant domain prefix
  (`location*`, `gear*`, `experience*`, etc.). (Affirmed across Phases
  1, 3, 4, 5.)
- This is a **widget-layer-only refactor**: no `Notifier`, no
  `@freezed` state, no repository, no provider, no service, no proto,
  no RPC changes. Per `i18n.md` viewmodels never resolve strings —
  this rule is upheld trivially because no viewmodels are touched.
  Per `architecture.md` ViewModels are the only layer that calls
  Repositories — also upheld trivially. Per `caching.md` repositories
  own cache invalidation — irrelevant to this work. Server
  observability and architecture: not applicable. (Affirmed in
  Recommendation.)
- Per-PR CI gates from `accessibility.md` §"Lint rules" and
  `i18n.md` §"Best Practices": every PR in every phase runs `npm run
  lint:dart:a11y`, `npm run lint:dart:i18n`, `npm run lint:dart:size`,
  `flutter test --reporter expanded`, `npm run build`. The
  `scripts/a11y_allowlist.txt` file must remain at zero entries.
  (Folded into acceptance criteria.)
- `GlassModalHeader` already wraps its value in
  `Semantics(header: true)` per the primitives README — but RSVP's
  migration is a special case because it currently has a *custom*
  drag handle and may also have a custom title widget. The Phase 1
  RSVP task explicitly preserves the `Semantics(header: true)`
  contract; if RSVP renders a non-`GlassModalHeader` title, wrap it
  manually. (Affirmed in Phase 1.)

## Problem Statement

#1797 shipped the foundation — `AppColors.modal*` tokens, `ModalTheme`,
the eight glass primitives (`GlassSurface`, `GlassSheet`,
`GlassModalHeader`, `GlassFieldLabel`, `GlassChip`, `GlassInlineAction`,
`GlassFooterButtons`, `GlassSearchInput`), the `showGlassDatePicker` /
`showGlassTimePicker` wrappers, and an initial wave of ~11 modal
migrations (mostly the time-modal stack and a handful of small action
sheets). What remains is the bulk of the codebase's bottom-sheet
inventory — every modal still rendering on the legacy cream / sage /
coral surfaces. The unit of work in this issue is exclusively
**re-skinning** existing modals; no behavior, navigation, validation,
state machine, or keyboard handling moves.

The issue body enumerates 37 files across waves A–E plus a Wave F
cleanup. Codebase research surfaces two complications the body does not
flag, both load-bearing on the implementation plan:

1. **Enumeration is incomplete.** At least 13 additional bottom-sheet
   files exist under `app/lib/presentation/` that are not glass yet and
   are not in the issue's wave lists (see Codebase findings §1).
2. **Wave E's infrastructure layer is a dependency of Wave B and Wave C,
   not a sibling of them.** `TransferModalShell` is consumed by
   `event_organizer_modal`, `event_joiner_modal`, `arrange_pickup_modal`,
   and the un-enumerated `loan_manage_modal`. Re-skinning the shell
   visually migrates those modals "for free", which means the wave
   ordering as written front-loads work that Wave E will redo (see
   Codebase findings §2).

## Context from Comments

The issue has no comments. The body is unusually thorough — it
inherits the design vocabulary, scope rules, and out-of-scope decisions
from the parent #1797 plan, and it splits the long tail into six
explicit waves with per-modal callouts (e.g. "date picker already glass;
shell still cream" on `mark_fulfilled_modal` and `time_poll_propose_modal`).
The role of this plan is to pressure-test that wave structure against
what's actually in the tree, not to refill missing requirements.

Two things the issue body claims that we should treat as authoritative
unless contradicted later:

- The "Already migrated" section is correct as a list of files that have
  landed — the git log (`e67f53408`, `7f7b5d18c`, `d0fd2ff13`,
  `a9ac24884`) shows the time-modal stack and creation time modals on
  the current branch. Don't re-migrate them.
- The Wave F cleanup list is the canonical set of dead-code deletions.
  Anything beyond that list (e.g. retiring `transferTextPrimary` or
  similar) needs an explicit decision.

## Codebase findings (from Phase 2 research)

These shape the recommendation; they are not in the issue body.

1. **The enumeration misses ~13 bottom-sheet files.** All of these still
   exist in `app/lib/presentation/`, are not glass (`grep` for
   `GlassSheet|GlassModalHeader|GlassChip|GlassSurface` returns nothing),
   and are not in the issue's waves:

   | File | Likely disposition |
   |---|---|
   | `screens/communities/invite_sheet.dart` | MIGRATE |
   | `screens/experience/widgets/carousel_media_picker_sheet.dart` | MIGRATE |
   | `screens/experience/widgets/preview/participant_picker_sheet.dart` | MIGRATE (verify it's bottom-sheet, not preview-screen-internal) |
   | `screens/workshop/workshop_roster_sheet.dart` | MIGRATE |
   | `widgets/chat/reaction/emoji_picker_sheet.dart` | MIGRATE |
   | `widgets/chat/reaction/reaction_detail_sheet.dart` | MIGRATE |
   | `widgets/discover/discover_bottom_sheet.dart` | MIGRATE (distinct from `discover_filter_sheet`) |
   | `widgets/experience/experience_metric_sheet.dart` | TRIAGE — possibly impact-detail aesthetic, possibly migrate |
   | `widgets/leaderboard/community_detail_modal.dart` | TRIAGE |
   | `widgets/sharing/bulk_community_selection_modal.dart` | MIGRATE |
   | `widgets/sharing/community_selection_modal.dart` | MIGRATE |
   | `widgets/sharing/item_share_sheet.dart` | MIGRATE |
   | `widgets/transfer/pickup_scheduling_sheet.dart` | TRIAGE — likely Wave E (transfer family) |
   | `screens/gear/loan_manage_modal.dart` | TRIAGE — uses `TransferModalShell`, so re-skins via Wave E infra |

   The triage CSV that #1797's Phase 2 called for
   (`docs/ai/modal_glass_migration.md`) was never created. This is the
   right moment to build it.

2. **Wave E's infrastructure is a dependency, not a peer, of parts of
   Waves B and C.** `grep -l "TransferModalShell\b"` returns:
   - `screens/gear/loan_borrower_modal.dart` (Wave E)
   - `screens/gear/loan_owner_modal.dart` (Wave E)
   - `screens/gear/giveaway_receiver_modal.dart` (Wave E)
   - `screens/gear/loan_manage_modal.dart` (un-enumerated)
   - `screens/gear/arrange_pickup_modal.dart` (Wave C)
   - `screens/experience/event_organizer_modal.dart` (Wave B)
   - `screens/experience/event_joiner_modal.dart` (Wave B)

   `giveaway_giver_modal.dart` is the lone Wave E implementation that
   doesn't go through `TransferModalShell` directly — worth confirming
   why during the migration.

   The implication: re-skinning `TransferModalShell` and the shared
   transfer cards (Wave E's first half) **visually migrates Wave B's
   event-organizer and event-joiner modals plus Wave C's
   arrange-pickup modal as a side-effect**. If we follow the issue's
   wave order literally (B → C → D → E), Wave B and the relevant Wave C
   entry will be migrated twice: once with bespoke code, once when the
   shell flips. The plan below reorders to land the transfer
   infrastructure first.

3. **Lint guardrails are healthy.** `grep -rn showModalBottomSheet
   app/lib --include='*.dart'` returns only doc comments and the glass-
   internal definitions; no production code bypasses
   `showAccessibleModal`. Wave F's "audit any remaining raw
   showModalBottomSheet call sites" task has zero hits today and should
   stay that way — the migration must not introduce new ones.

4. **Sub-widgets named in Wave E are correct but the `TransferTimeline`
   line item should call out a known constraint.** `BackdropFilter` does
   not compose recursively the way the issue body's "cards-within-cards
   at white/8" suggestion implies — the inner `BackdropFilter` reads the
   already-blurred parent texture, which on iOS+Metal can produce a
   visible ring artifact. A `GlassInsetCard` variant that uses a flat
   alpha fill (no inner blur) is the correct shape. The parent #1797
   plan already proposed this; the issue body's wording can be
   misread as "another `BackdropFilter`".

5. **`experience_participants_header.dart` and `qt_attributes_card.dart`
   trigger sheets via `showAccessibleModal` calls inside the widgets
   themselves**, confirmed by `grep -n showAccessibleModal`. The issue's
   parenthetical "its sheet entry point" is correct — the migration
   target is the inline sheet builder, not the header/card widget.
   `content_sharing_trigger.dart`'s `showModalBottomSheet` references
   are doc-comment examples only; before adding it to the migrate list,
   verify the widget actually opens a bottom sheet at runtime.

6. **The Wave F dead-code candidates are real.** Spot-check:
   `ModalBuilders.buildGradientHeader` is `@Deprecated` per #1797 Phase
   1 and will lose every call site once Waves A–E land;
   `ModalActionButtons` is referenced by `location_picker_modal.dart`
   (Wave A); `AppColors.experienceModalBackground` is gated on the
   creation-modal redesign landing separately (timing risk — see Open
   Questions). The cleanup phase as written is accurate.

## Potential Solutions

### Option A: Follow the issue's wave order literally (A → B → C → D → E → F)

- **Approach:** Migrate by user-visibility priority as the issue body
  proposes. Each wave is one or more PRs.
- **Pros:** Matches the issue body. Highest-visibility surfaces fix
  first. Easy to communicate progress to non-engineers.
- **Cons:** Wave B's `event_organizer_modal` + `event_joiner_modal` and
  Wave C's `arrange_pickup_modal` get migrated twice — once with bespoke
  glass code, again when Wave E re-skins `TransferModalShell`. Wastes
  effort and risks divergence between the two passes.
- **Effort:** Medium-large overall, with measurable rework cost.

### Option B: Reorder to land Wave E's infrastructure layer before Waves B/C consumers (recommended)

- **Approach:** Triage first (build the missing CSV, decide MIGRATE vs.
  out-of-scope vs. delete for the un-enumerated 13 files). Then ship
  Wave A (highest visibility, no shared infrastructure dependency). Then
  ship Wave E1 (transfer infrastructure: `TransferModalShell` and shared
  cards) **before** the rest of Waves B/C. Wave E1's PR auto-migrates
  the transfer-shell-using modals in Waves B and C as a side effect;
  remaining Wave B/C entries that don't share infra ship next. Then
  Wave D. Then Wave E2 (the four canonical transfer implementations
  with any per-flow tuning). Then Wave F cleanup.
- **Pros:** No double migration. Wave E1 is the natural moment to make
  the `GlassInsetCard` decision and apply it to the un-enumerated
  `loan_manage_modal` in the same diff. The triage CSV gives a single
  source of truth for progress.
- **Cons:** Wave E1 is the most architecturally sensitive PR
  (cards-on-glass, shared sub-widgets) and shipping it earlier
  front-loads the hardest call. Slightly less "user-visible to internal"
  ordering — Wave E1 ships before the reader notices Waves B and C
  finishing.
- **Effort:** Same total cost; redistributed to remove the double-pass
  on transfer-shell-backed modals.

### Option C: Single sweep PR

- **Approach:** One PR that migrates every remaining modal at once.
- **Pros:** No half-state, one screenshot pass.
- **Cons:** Unreviewable. Mixes ~50 files including the transfer family.
  Counter to the issue body's stated wave-by-wave preference. High
  regression risk.
- **Effort:** Large and dangerous.

## Recommendation

**Option B.** The triage step is non-negotiable — the issue's
enumeration misses files, the parent plan's CSV was never written,
and at least one modal file (`weekly_goal_editor.dart`) is dead and
should be deleted, not migrated. Reordering to ship
`TransferModalShell` before its Wave B/C consumers is the only way
to avoid migrating those modals twice. The visual win of Wave A
(location picker, RSVP, plus-button) ships first because it has no
shared-infrastructure dependency, so user-visible progress isn't
blocked on the harder transfer work.

The amendments to the issue body:

1. Insert a Wave 0 "Triage and inventory" step that builds the CSV at
   `docs/ai/modal_glass_migration.md`, confirms in/out-of-scope for
   each un-enumerated file, and **deletes any dead modal files in
   the same PR**.
2. Reorder Wave E so the infrastructure half (TransferModalShell +
   shared cards) ships **before** Waves B and C, not after.

Everything else in the issue body — the per-modal hints, the Wave F
cleanup list, the acceptance criteria — stands.

### Per-PR gates that apply to every phase

Per [`docs/client/accessibility.md`](../client/accessibility.md) §"Lint
rules" and [`docs/client/i18n.md`](../client/i18n.md) §"Best Practices",
every PR in every phase below must pass:

- `npm run lint:dart:a11y` (rules: `avoid_raw_gesture_detector`,
  `require_semantic_label_on_icon_button`,
  `require_l10n_for_semantic_labels`,
  `require_semantic_label_on_cached_media_image`,
  `prefer_accessible_modal`, `prefer_accessible_duration`,
  `avoid_color_only_status`)
- `npm run lint:dart:i18n` (no hardcoded user-visible strings)
- `npm run lint:dart:size` (1,000-line cap on non-generated Dart)
- `flutter test --reporter expanded`
- `npm run build`
- `scripts/a11y_allowlist.txt` remains at zero entries

Architectural rules upheld trivially because this is a widget-layer-
only refactor: no `Notifier` / `@freezed` state changes, no repository
or service changes, no proto or RPC changes, no provider rewires,
no caching changes. Viewmodels never resolve strings — and no
viewmodels are touched in this issue. ViewModels-only-call-Repositories
and Repositories-only-call-Services rules: not in scope.

## Resolved Decisions

All open questions are decided up front. Recording here so reviewers
can see the chosen direction without re-litigating.

- **The 13 un-enumerated modal files all get a triage disposition in
  Phase 0.** Each row in the CSV resolves to MIGRATE / OUT-OF-SCOPE /
  DELETE before any visual work begins. No other wave ships until
  triage is done.
- **Cards-within-cards on glass use `GlassInsetCard`** — flat
  `white/8` alpha fill, `white/12` border, no inner `BackdropFilter`.
  This avoids the recursive-blur ring on iOS+Metal flagged in
  Codebase findings §4. Defined in Phase 2.
- **`experience_metric_sheet.dart` disposition is decided during
  Phase 0 triage by inspection.** If it uses `ImpactModalColors`, mark
  OUT-OF-SCOPE: impact-detail. Otherwise assign to Wave D.
- **`pickup_scheduling_sheet.dart` is a Wave E1 entry.** Whether or
  not its surface is currently part of `TransferModalShell`, it
  belongs to the transfer family conceptually and should re-skin in
  the same PR as the shell so the visual delta lands together.
- **`arrange_pickup_modal.dart` is removed from the Wave C line
  items** — confirmed it goes through `TransferModalShell` and is
  auto-migrated by Wave E1. Phase 4 (Wave C remainder) only verifies
  it migrated correctly; no work needed.
- **`AppColors.experienceModalBackground` / `lightExperienceModalBackground`
  are NOT deleted in this issue.** They stay until the parallel
  creation-modal redesign lands. Phase 7 (Wave F) grep-confirms call
  sites and, if creation modals still hold them, leaves the delete to
  the creation effort with an explicit note in the cleanup PR.
- **`transferSage` / `transferCoral` keep their names and survive the
  migration.** They are semantic accents (success/urgency), not
  surface tokens.
- **Delete `transferBackgroundCream`, `transferCardBackground`,
  `transferBorder`, `transferBorderLight`, `transferHighlight`, AND
  `transferTextPrimary` / `transferTextSecondary` /
  `transferTextMuted` in Phase 7.** All seven tokens — the surface
  family and the text family — go once Wave E2 confirms zero
  remaining call sites. The glass surface uses `AppColors.modal*`
  text tokens, so the transfer text tokens are redundant after
  migration.

## Implementation Plan

### Phase 0: Triage, inventory, and dead-code deletion (single PR, ~half a day)

**Goal:** Replace the issue's enumeration with a verified inventory,
delete unused modal files, and fix the wave ordering before any visual
work starts.

**Tasks:**
- [ ] Build the triage CSV at `docs/ai/modal_glass_migration.md`. One
      row per modal-shaped file under `app/lib/presentation/`
      (everything matching `*_modal.dart`, `*_sheet.dart`, or
      `*_dialog.dart`, plus every file containing
      `showAccessibleModal` / `showStandardModal`). Columns: `file`,
      `enumerated_in_issue`, `uses_glass_today` (grep `GlassSheet|
      GlassModalHeader|GlassChip|GlassSurface`),
      `goes_through_TransferModalShell`, `caller_count` (grep for
      class name and filename across `app/lib` and `app/test`,
      excluding the file itself, generated files, and ARB-derived
      `app_localizations*.dart`), `disposition` (MIGRATE / OUT-OF-
      SCOPE: dialog / OUT-OF-SCOPE: creation / OUT-OF-SCOPE: preview
      / OUT-OF-SCOPE: impact-detail / DELETE: no-callers / TRIAGE-
      PENDING), `wave_assignment` (A / B / C / D / E1 / E2 / F or
      "n/a"), `notes`.
- [ ] Resolve every TRIAGE-PENDING row by reading the file. Specifically
      for the 13 un-enumerated files: confirm each is a bottom sheet
      (not a `Dialog`, not a creation/preview screen, not impact-detail)
      and assign a wave.
- [ ] **Hunt for dead modal files explicitly.** Any row with
      `caller_count == 0` is a delete candidate. Suspected dead today
      based on Phase 0 research:
      - **`weekly_goal_editor.dart`** — confirmed dead. Zero callers
        in production code; only its own file and
        `app/test/presentation/widgets/portfolio/weekly_goal_editor_test.dart`
        reference it. The `WeeklyGoal*` matches in
        `metric_detail_screen.dart` are the proto type, not the
        widget. Delete the file and its test in this Phase 0 PR.
      - `content_overflow_menu.dart` — checked; **NOT dead.**
        `community_content_view.dart:587` actively calls
        `showContentOverflowMenu(...)`. Remains MIGRATE/migrated.
      - Re-run the same grep for every `_modal.dart` /
        `_sheet.dart` file before assuming any is alive: false
        positives in the import-only-from-l10n column are common
        because some files share names with ARB keys.
- [ ] For each DELETE row, verify by running:
      `grep -rn '<ClassName>\\|<filename>' app/lib app/test --include='*.dart'
       | grep -v '<file_being_deleted>' | grep -v 'app_localizations'`.
      If the result is empty (or only matches generated/ARB files),
      delete the file and any companion test in this PR.
- [ ] Run `npm run build` and `flutter test --reporter expanded` after
      every delete to confirm nothing imports the removed file.
- [ ] Update the issue's wave assignments in the CSV to reflect:
      Wave E1 (infra) ships before Wave B's `event_organizer_modal` /
      `event_joiner_modal` and before Wave C's `arrange_pickup_modal`
      and `loan_manage_modal`. The Wave C `arrange_pickup_modal`
      checkbox in the issue body is already-migrated-once-Wave-E1-lands.
- [ ] Add a `GlassInsetCard` design decision note to
      `app/lib/presentation/widgets/modal/glass/README.md` (flat alpha
      fill, no recursive blur, `white/8` over `white/15` parent). Don't
      implement it yet — Wave E1 introduces the widget when needed.
- [ ] Affirm in the triage CSV that this issue introduces no new
      hardcoded colors or magic geometry numbers: every new color
      flows from `AppColors.modal*` and every new constant from
      `ModalTheme`. The white-on-glass deviation from "all colors
      must be theme-aware" was blessed in #1797 and documented in
      `docs/client/design.md`; nothing in this issue widens that
      deviation.

**Validates:** Every bottom-sheet modal in the codebase has a known
disposition. Dead modal files (confirmed: `weekly_goal_editor.dart`,
plus anything else surfaced by the `caller_count == 0` sweep) are
deleted. Wave order matches the dependency graph. No double
migrations are scheduled.

### Phase 1: Wave A — Picker / action modals (1 PR, optionally 3 small PRs)

**Goal:** Highest-visibility legacy surfaces flip to glass. No shared
infrastructure dependency on transfer.

**Tasks:**
- [ ] [`location_picker_modal.dart`](../../app/lib/presentation/widgets/location/location_picker_modal.dart) —
      replace `ModalBuilders.buildGradientHeader` →
      `GlassModalHeader`; cream search input → `GlassSearchInput`;
      chip row → `GlassChip` row backed by extended `ChipData`;
      `ModalActionButtons` → `GlassFooterButtons`. Drop the
      `ModalActionButtons` import — it goes in Wave F if no other
      caller remains.
- [ ] [`rsvp_modal.dart`](../../app/lib/presentation/screens/experience/rsvp_modal.dart) —
      cream container → `GlassSheet`; custom drag handle → built-in
      drag handle from `GlassSheet`; re-skin sub-widgets (`LocationRow`,
      `WhenRow`, `RSVPToggleButtons`, response badges) using
      `AppColors.modal*` text tokens. The largest single migration in
      this wave. **Special-case a11y:** if RSVP renders a non-
      `GlassModalHeader` title (it has historically had a custom
      title widget), wrap that title in `Semantics(header: true)`
      manually so the migration doesn't lose the modal-heading
      announcement. Use `Toggle` (not `GestureDetector`) on
      `RSVPToggleButtons` — they're selectable state. Run
      `npm run lint:dart:a11y` to confirm zero new violations.
- [ ] [`plus_button_modal.dart`](../../app/lib/presentation/widgets/plus_button_modal.dart) —
      `AppColors.background` cream container → `GlassSheet`. Keep the
      "Ask / Do / Share" segmented tab structure and the per-category
      gradient circles as informational accents (the issue body
      explicitly preserves them); only the sheet shell, segmented
      pill, and bottom buttons re-skin. Document the gradient-circle
      retention in a one-line code comment so a later sweep doesn't
      "fix" it.
- [ ] Add per-modal kicker / field-label ARB keys to `app_en.arb` AND
      `app_es.arb`. **Naming:** keys for `semanticsLabel` / `tooltip`
      use the `a11y` prefix (e.g. `a11yLocationPickerSearchInput`,
      `a11yPlusButtonClose`); keys for visible text use the relevant
      domain prefix (`location*`, `experience*`). Run
      `npm run generate` in the same PR.
- [ ] Each migrated modal: before/after screenshot in PR description.
- [ ] Per-PR CI gates: `npm run lint:dart:a11y`,
      `npm run lint:dart:i18n`, `npm run lint:dart:size`,
      `flutter test --reporter expanded`, `npm run build`. The
      `scripts/a11y_allowlist.txt` file must stay at zero entries.

**Validates:** Glass primitives compose against autocomplete dropdowns
(location), tab/pill segmented controls (plus-button), and toggle-rich
content (RSVP). The three highest-traffic surfaces look right.

### Phase 2: Wave E1 — Transfer infrastructure (1 PR)

**Goal:** Re-skin the shared transfer infrastructure once. This phase
lands **before** the rest of Waves B and C because it auto-migrates
several of their entries.

**Tasks:**
- [ ] Implement `GlassInsetCard` under `widgets/modal/glass/` (flat
      `white/8` alpha fill from a new `AppColors.modalInsetCardBg`
      token, `white/12` border from `AppColors.modalInsetCardBorder`,
      **no inner `BackdropFilter`**). The widget is
      presentational — no gesture wrapper unless a consumer actually
      passes an `onTap`, in which case it composes a `Tappable`
      internally rather than exposing a raw `GestureDetector`. Add a
      golden test against the dark-scrim backdrop in
      `app/test/presentation/widgets/modal/glass/`. Add a widget test
      that confirms `useBlur: false` from `GlassSurface`'s parent does
      not cause `GlassInsetCard` to attempt a blur. Add to the barrel
      `glass.dart` and to the README's table per the modal-glass
      README's "When to add a new widget here" rule.
- [ ] Re-skin `TransferModalShell`, `TransferModalHeader`,
      `TransferModalTitle`, `TransferPhaseBottomBar`,
      `TransferProgressBar`, `TransferErrorDisplay`. Outer surface
      becomes `GlassSheet`; replace bespoke header with
      `GlassModalHeader`; bottom bar uses `GlassFooterButtons` (or its
      multi-action variant if needed — only fork if the design demands
      it).
- [ ] Re-skin shared cards as `GlassInsetCard` consumers:
      `PickupDetailsCard`, `ImpactMetricsCard`, `ImpactMetricRow`,
      `CountdownDisplay`, `WaitingOnUserCard`, `WhatHappensNextCard`,
      `WhatTheySeeCard`, `OtherPartySees`, `RecipientSelectionCard`,
      `QuickDateChips`, `QuickTimeChips`, `DurationChips`,
      `StepChecklistItem`, `TransferTimeline`. Keep `transferSage` /
      `transferCoral` as accent borders / dot fills (semantic), not as
      surface fills.
- [ ] Verify visual quality on the four Wave E2 implementations
      (`loan_borrower_modal`, `loan_owner_modal`, `giveaway_receiver_
      modal`, `giveaway_giver_modal`) and on the three transitive
      consumers (`event_organizer_modal`, `event_joiner_modal`,
      `arrange_pickup_modal`) plus the un-enumerated
      `loan_manage_modal`. Screenshots per phase per implementation.
- [ ] Don't yet delete `transferBackgroundCream`,
      `transferCardBackground`, `transferBorder*`, `transferHighlight`
      — they're used by anything not yet migrated. The deletion is
      Wave F.
- [ ] Per-PR CI gates: same as Phase 1 (`lint:dart:a11y`,
      `lint:dart:i18n`, `lint:dart:size`, `flutter test`,
      `npm run build`); a11y allowlist stays at zero. **Plus:**
      golden tests for `GlassInsetCard` and any updated transfer
      shared widgets. The transfer family is the highest visual-
      regression risk in the migration; goldens are the primary
      signal.

**Validates:** Cards-on-glass compose without recursive-blur artifacts.
Phase transitions in the loan and giveaway flows don't flicker.
Eight modals (4 canonical + 3 enumerated transitive consumers + 1
un-enumerated) all render glass after this single PR.

### Phase 3: Wave B remainder — event/experience action sheets (1–2 PRs)

**Goal:** Migrate the Wave B entries that don't ride on
`TransferModalShell`. The two Wave B modals that do
(`event_organizer_modal`, `event_joiner_modal`) are already glass after
Phase 2 — verify and tick them off, no work needed.

**Tasks:**
- [ ] `mark_completed_modal.dart`
- [ ] `mark_fulfilled_modal.dart` (date picker already glass; flip the
      shell)
- [ ] `time_poll_propose_modal.dart` (pickers already glass; flip the
      shell)
- [ ] `time_poll_vote_modal.dart`
- [ ] `time_poll_confirm_modal.dart`
- [ ] `attendee_list_sheet.dart`
- [ ] The `widgets/experience/needs/` sheets:
      `add_contribution_sheet`, `batch_add_contribution_sheet`,
      `batch_add_need_sheet`, `edit_contribution_sheet`,
      `view_contribution_sheet`, `view_need_sheet`
- [ ] `experience_participants_header.dart` (its inline
      `showAccessibleModal` builder)
- [ ] Any Wave B-equivalent un-enumerated entries that triage (Phase
      0) put in Wave B — likely `participant_picker_sheet.dart`.
- [ ] Per-modal kicker / field-label ARB keys, both locales (`a11y`
      prefix for `semanticsLabel` / `tooltip` strings; domain prefix
      for visible text). Screenshots in PR description.
- [ ] Per-PR CI gates: `lint:dart:a11y`, `lint:dart:i18n`,
      `lint:dart:size`, `flutter test`, `npm run build`; allowlist
      stays at zero. Existing widget tests for migrated modals must
      stay green; if a test inspects legacy color tokens directly,
      update it to assert presence of the glass primitive instead.

**Validates:** Event-flow action sheets all read as the same material.

### Phase 4: Wave C remainder — gear/request/loan action sheets (1–2 PRs)

**Goal:** Non-transfer gear and request sheets. `arrange_pickup_modal`
and `loan_manage_modal` are already glass after Phase 2 — verify and
tick.

**Tasks:**
- [ ] `loan_list_modal.dart`
- [ ] `past_transfer_modal.dart` (date picker already glass; flip the
      shell)
- [ ] `gear_borrower_sheet.dart`
- [ ] `gear_interest_sheet.dart`
- [ ] `gear_metadata_sheet.dart`
- [ ] `helper_list_sheet.dart`
- [ ] `request_needs_sheets.dart`
- [ ] `request_management_modal.dart`
- [ ] `gear_access_sheet_helper.dart`
- [ ] `category_material_editor.dart`
- [ ] `pickup_scheduling_sheet.dart` if Phase 0 triage put it here
      rather than in Wave E1.
- [ ] Per-modal ARB keys, both locales (`a11y` prefix where
      relevant). Screenshots.
- [ ] Per-PR CI gates as in Phases 1–3.

**Validates:** Gear and request action surfaces are all glass.

### Phase 5: Wave D — community / portfolio / discover / settings (1 PR)

**Goal:** Migrate the remaining low-traffic surfaces.

**Tasks:**
- [ ] `community_picker_sheet.dart` — also re-skin `CommunityToggleTile`
      in glass, since it's a shared tile and a parallel cream
      version would create the exact "two materials" anti-pattern this
      issue retires.
- [ ] `discover_filter_sheet.dart`
- [ ] `portfolio_filter_sheet.dart`
- [ ] `feedback_sheet.dart`
- [ ] `shadow_user_profile_sheet.dart`
- [ ] `access_sheet.dart`
- [ ] `content_metric_sheet.dart` — confirm during triage that it's
      not impact-detail (`ImpactModalColors`); if it is, mark
      out-of-scope.
- [ ] `content_sharing_trigger.dart` — only if it actually opens a
      bottom sheet at runtime; the doc-comment `showModalBottomSheet`
      references aren't dispositive.
- [ ] `media_pick_from_carousel.dart`
- [ ] `qt_attributes_card.dart` (its inline `showAccessibleModal`
      builder)
- [ ] Un-enumerated entries that triage (Phase 0) puts here:
      `invite_sheet.dart`, `bulk_community_selection_modal.dart`,
      `community_selection_modal.dart`, `item_share_sheet.dart`,
      `discover_bottom_sheet.dart`, `emoji_picker_sheet.dart`,
      `reaction_detail_sheet.dart`, `workshop_roster_sheet.dart`,
      `carousel_media_picker_sheet.dart`, possibly
      `community_detail_modal.dart` (leaderboard) and
      `experience_metric_sheet.dart`.
- [ ] Per-PR CI gates as in earlier phases. ARB keys for any new
      kicker / field-label / `semanticsLabel` strings, both locales.

**Validates:** Every in-scope bottom-sheet modal in the codebase
renders on the glass surface.

### Phase 6: Wave E2 — Per-flow polish on the four canonical transfer modals (1 PR)

**Goal:** Catch anything Wave E1 left bespoke per implementation.

**Tasks:**
- [ ] `loan_borrower_modal.dart`
- [ ] `loan_owner_modal.dart`
- [ ] `giveaway_receiver_modal.dart`
- [ ] `giveaway_giver_modal.dart` — confirm why it doesn't go through
      `TransferModalShell` (only one of the four that doesn't); either
      route it through the shell or document the deliberate divergence.
- [ ] If the "outer-glass + inset-card" decision needs per-flow tuning
      (e.g. tighter spacing on giveaway-receiver), apply it here.
- [ ] Screenshots per phase per modal.
- [ ] Per-PR CI gates. Re-run the goldens added in Phase 2 — they
      should still pass without re-baselining (otherwise Phase 6
      changed the inset-card surface, which means Phase 2's golden
      was wrong or this PR is doing more than polish).

**Validates:** The four canonical transfer flows look polished, not
just functional, on glass.

### Phase 7: Wave F — Cleanup (1 PR)

**Goal:** Retire deprecated APIs, delete dead tokens, update docs.
Same task list as the issue body, with two amendments noted.

**Tasks:**
- [ ] Delete `ModalBuilders.buildGradientHeader` and any
      companions (`buildOwnerEditSection`,
      `buildHeaderActionButtons`, `buildReadOnlySummary`,
      `buildSearchInput`, `buildSuggestionsDropdown`,
      `buildChipsRow`, `buildChip`, `buildContentSection`,
      `buildInfoDisplayRow`, `buildModalScaffold`) with no remaining
      callers — likely the entire `ModalBuilders` class.
- [ ] Delete the corresponding tests in `modal_builders_test.dart`,
      including the three `// ignore: deprecated_member_use_from_
      same_package` sites added in #1797.
- [ ] Delete `ModalActionButtons` (Wave A removed its sole caller).
- [ ] **Conditional:** Delete `AppColors.experienceModalBackground` /
      `lightExperienceModalBackground` only if grep confirms zero
      remaining call sites. If creation modals (parallel redesign)
      still hold them, defer the delete to that effort and note in PR.
- [ ] Delete `transferBackgroundCream`, `transferCardBackground`,
      `transferBorder`, `transferBorderLight`, `transferHighlight`,
      and the `transferText*` family (`transferTextPrimary`,
      `transferTextSecondary`, `transferTextMuted`) once Wave E2
      confirms zero remaining call sites. Keep `transferSage` /
      `transferCoral` (semantic accents).
- [ ] Audit `showModalBottomSheet` call sites that bypass
      `showAccessibleModal` — should remain at zero (Codebase findings
      §3). If non-zero, route through `showAccessibleModal` and wrap
      in `GlassSheet`.
- [ ] Remove the "Modal Heights" inconsistency note from
      [docs/client/design.md](../client/design.md).
- [ ] Update [docs/client/modals.md](../client/modals.md) "Last
      Updated" and remove references to legacy modal background tokens.
      Add a note that creation/preview and impact-detail modals are
      intentionally on different surfaces.
- [ ] Verify `scripts/a11y_allowlist.txt` is still empty.
- [ ] Archive `docs/ai/modal_glass_migration.md` (move to
      `docs/ai/archive/`) once every MIGRATE row is checked off.

**Validates:** Acceptance criteria from the issue are met. Codebase
has exactly one bottom-sheet modal material; no dead `ModalBuilders`
or orphan transfer tokens remain.

## Related Issues

- **Parent:** [#1797](https://github.com/Type2Fun-Foundation/ripls/issues/1797) —
  the design and foundation. This issue's Phase 0 builds the triage
  CSV that #1797's Phase 2 specified but didn't ship.
- **Parallel effort (no issue link known):** Creation and preview
  modals (`create_gear_modal`, `experience_creation_modal`,
  `community_creation_modal`, `request_creation_modal`, and the four
  `*_preview_modal` files). These are intentionally out of scope for
  both #1797 and #1802. The deletion of
  `AppColors.experienceModalBackground` is gated on that effort
  landing.
- **Acknowledged-out-of-scope:** Impact-detail modals (`co2_detail_modal`,
  `value_detail_modal`, `quality_time_detail_modal`,
  `widgets/impact/completion/`) — `ImpactModalColors` warm-paper
  aesthetic stays, no follow-up issue needed.
