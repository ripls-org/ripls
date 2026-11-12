# Issue #2124: Replace date/time pickers in location poll deadline with combined calendar+time modal

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2124
**Filed:** 2026-05-22 | **Last activity:** 2026-05-22
**Labels:** none | **Assignees:** unassigned
**Context docs:** none (design reference: [docs/cowork/App Design/edit-time-modal-standalone.html](../cowork/App%20Design/edit-time-modal-standalone.html))

> **Audit pass (2026-05-22):** revised after reading
> [docs/client/architecture.md](../client/architecture.md),
> [docs/client/modals.md](../client/modals.md),
> [docs/client/accessibility.md](../client/accessibility.md),
> [docs/client/i18n.md](../client/i18n.md),
> [docs/client/testing.md](../client/testing.md), and
> [docs/client/caching.md](../client/caching.md). Key changes:
> (1) the modal must enter via `showAccessibleModal` + `GlassSheet`, not
> `showDialog` — the design is a bottom sheet, not a centered dialog,
> and the `prefer_accessible_modal` lint rejects raw show-helpers;
> (2) results go through the **Navigator Result Pattern**
> (`Navigator.pop(result)`), not callbacks like `onRemove`;
> (3) ARB strings under a non-`a11y*` prefix for user-visible text, and
> reuse existing `a11y*` keys for SemanticsLabels;
> (4) state lives in a Notifier with Freezed state so the wheel
> behavior is testable per the ViewModel-first convention, not a bare
> `StatefulWidget`;
> (5) accessibility primitives (`IconAction`, `Tappable`, `LiveRegion`)
> are mandatory inside the new modal.

## Problem Statement

The app's date/time selection UX is split across two separate sheets in every
flow that needs both a date and a time:

- The **location-poll deadline** flow in
  [app/lib/presentation/screens/experience/location_poll_propose_modal.dart:235-270](../../app/lib/presentation/screens/experience/location_poll_propose_modal.dart#L235-L270)
  chains stock `showDatePicker` → `showTimePicker`. Two sheets, two
  dismiss/confirm steps, and the stock Material chrome doesn't match the
  dark glass-tint design used by everything else in the propose flow.
- The **experience time-selection** flows (single-time experience create
  / edit, time-poll propose, transfer pickup chips) all use
  `showGlassDatePicker` + `showGlassTimePicker` as two independent steps.
  These are glass-themed, but the user still has to tap "Date" then later
  tap "Time" through separate sheets.

The design at
[docs/cowork/App Design/edit-time-modal-standalone.html](../cowork/App%20Design/edit-time-modal-standalone.html)
puts a month calendar grid and the hour/minute/AM-PM wheels in **one**
sheet, with a single Save action and a Remove variant when editing an
existing value. The original issue scoped this to the location-poll
deadline only; **the author's follow-up comment broadens the scope: "This
is not just for setting deadlines. This should be used when selecting a
specific date/time for experiences."** That means the new component must
be designed as a shared widget that replaces every date-and-time pair in
the app, not a one-off for the deadline button.

## Context from Comments

- **@tescobar (issue author), 2026-05-22:** "This is not just for setting
  deadlines. This should be used when selecting a specific date/time for
  experiences." This is a scope expansion — the original issue body says
  the only caller to migrate is `_pickDeadline`, but the comment tells us
  to plan for migrating the experience date/time flows as well.

No other comments. No maintainer pushback or disagreement. The expanded
scope is now the working interpretation.

## Codebase Findings

### Existing date/time selection callers

| File | Line | Picker | Selects | Type |
|---|---|---|---|---|
| `presentation/widgets/experience/propose_time_form.dart` | 239 | `showGlassDatePicker` | Experience event date | date-only |
| `presentation/widgets/experience/propose_time_form.dart` | 264 | `showGlassTimePicker` | Experience event start time | time-only |
| `presentation/widgets/experience/duration_picker_modal.dart` | 36 | Custom wheel | Experience duration | duration |
| `presentation/screens/experience/location_poll_propose_modal.dart` | 240 | `showDatePicker` (raw Material) | Poll deadline date | date-only |
| `presentation/screens/experience/location_poll_propose_modal.dart` | 247 | `showTimePicker` (raw Material) | Poll deadline time | time-only |
| `presentation/screens/experience/experience_preview_modal.dart` | 309 | `_showTimePicker` → glass | Edit experience time | time-only |
| `presentation/screens/experience/time_poll_propose_modal.dart` | 91 | `showGlassDatePicker` | Time-poll option date | date-only |
| `presentation/screens/experience/time_poll_propose_modal.dart` | 98 | `showGlassTimePicker` | Time-poll option time | time-only |
| `presentation/widgets/transfer/quick_date_chips.dart` | 113 | `showGlassDatePicker` | Transfer pickup date | date-only |
| `presentation/widgets/transfer/quick_time_chips.dart` | 101 | `showGlassTimePicker` | Transfer pickup time | time-only |

The location-poll deadline is the only caller still on raw Material
pickers; everything else is already on the glass-themed wrappers.

### Existing glass-sheet infrastructure to reuse

- [app/lib/presentation/widgets/modal/glass/glass_sheet.dart](../../app/lib/presentation/widgets/modal/glass/glass_sheet.dart)
  — translucent surface + backdrop blur container the new modal must sit
  in. Per [docs/client/modals.md](../client/modals.md), the canonical
  bottom-sheet entry pattern is `showAccessibleModal` (from
  [app/lib/presentation/widgets/accessibility/show_accessible_modal.dart](../../app/lib/presentation/widgets/accessibility/show_accessible_modal.dart))
  + `GlassSheet`. Raw `showModalBottomSheet` is rejected by the
  `prefer_accessible_modal` lint rule.
- [app/lib/presentation/widgets/modal/glass/glass_footer_buttons.dart](../../app/lib/presentation/widgets/modal/glass/glass_footer_buttons.dart)
  — Cancel/Save and Remove/Save button rows already styled. Must use this
  primitive — modals.md explicitly forbids hand-rolled
  `OutlinedButton`/`ElevatedButton` pairs in the footer.
- [app/lib/presentation/widgets/modal/glass/glass_modal_header.dart](../../app/lib/presentation/widgets/modal/glass/glass_modal_header.dart)
  — Eyebrow + title + close-X header. Already wraps the title in
  `Semantics(header: true)` per the accessibility doc.
- [app/lib/presentation/widgets/experience/duration_picker_modal.dart](../../app/lib/presentation/widgets/experience/duration_picker_modal.dart)
  — Existing wheel-picker implementation with the same iOS-style wheel rows
  the design calls for. Pattern to mirror, not directly reuse. Note: it
  uses `showDialog` (centered dialog); we use `showAccessibleModal`
  because the new modal is bottom-sheet-shaped per the design.
- [app/lib/presentation/screens/experience/unified_time_modal.dart](../../app/lib/presentation/screens/experience/unified_time_modal.dart)
  and [app/lib/presentation/viewmodels/time_picker_view_model.dart](../../app/lib/presentation/viewmodels/time_picker_view_model.dart)
  — Existing ViewModel-backed time picker that drives the experience time
  flow. The new modal lives parallel to it; Phase 3 integrates the new
  modal as the inner "Set / change a specific date+time" picker that
  `UnifiedTimeModal` can route to.
- [app/lib/core/theme/app_colors.dart](../../app/lib/core/theme/app_colors.dart)
  — `modalSurface`, `modalBackdrop`, `modalSurfaceBlurSigma`,
  `modalPrimaryButtonBackground` (sage), `modalChipBackground`,
  `modalTextPrimary/Secondary/Muted`, `error`. All tokens the design needs
  already exist; no new color constants. Per modals.md, glass modals
  flow colors from `AppColors.modal*` only — no `Color()` literals.
- [app/lib/core/theme/app_theme.dart](../../app/lib/core/theme/app_theme.dart)
  — `headingFont = 'Libre Baskerville'` for the title, and the
  `ModalTheme` constants (`sheetHorizontalInset`, `sheetTopRadius`,
  `sheetMaxHeightFraction`) that `GlassSheet` already enforces.

### ARB strings

`commonCancel`, `commonSave`, `commonRemove`, `commonDone`, and
`contentRowsEditTime` already exist in
[app/lib/l10n/app_en.arb](../../app/lib/l10n/app_en.arb). New strings
needed: a title ("Pick a date & time" / "Edit time"), a subtitle, and a
preview-line formatter.

Per [docs/client/i18n.md](../client/i18n.md):

- User-visible strings (title, subtitle, eyebrow) use a neutral
  domain-prefixed key — recommend `dateTimeModal*`.
- Accessibility-only strings (close-X semantics, wheel
  increment/decrement announcements) must use the `a11y*` prefix.
  Reuse the existing `a11yClose` rather than minting a new one.
- Every new key must land in **both** `app_en.arb` and `app_es.arb`
  in the same PR — `npm run lint:dart:l10n-parity`
  (`scripts/check_l10n_parity.js`) fails CI otherwise. English
  placeholder text in `app_es.arb` is acceptable; parity is the gate.
- All accessibility-visible strings must come from `context.l10n.*` —
  the `require_l10n_for_semantic_labels` lint rule (in
  `scripts/check_a11y.js`) rejects raw string literals on `tooltip:`,
  `semanticLabel:`, or inside `Semantics(...)`.

## Potential Solutions

### Option A: Single shared `DateTimePickerModal`, migrate every caller in one PR

- **Approach:** Build one new widget that owns the combined calendar+wheels
  UI, exposes a `show(context, initial, first, last, mode: create|edit)`
  static, returns `DateTime?` (plus a sentinel for the Remove case). In the
  same PR, swap every date+time pair in the table above to the new
  widget. Delete `showGlassTimePicker` once nothing references it (the
  date-only `showGlassDatePicker` callers stay — there are still
  date-only contexts).
- **Pros:** One PR = one mental model for reviewers, no parallel APIs
  living side-by-side, no risk of half-migrated callers drifting.
- **Cons:** Touches ~6 caller files and their tests in one PR. Large diff,
  longer review, harder to back out if one flow regresses.
- **Effort:** Large.

### Option B: Build the widget, migrate only the deadline caller, follow up the rest

- **Approach:** Same shared widget, but the PR migrates only
  `_pickDeadline`. Other callers stay on `showGlassDatePicker` +
  `showGlassTimePicker` and are migrated in follow-up issues (one per
  surface). This is what the **original** issue body asked for; the
  author's comment expanded the scope but doesn't require it land in one
  PR.
- **Pros:** Small, reviewable first PR. New widget is exercised in a real
  flow before we commit every caller to it. We can adjust the API after
  feedback from the deadline flow before locking in the rest.
- **Cons:** Two date/time UX patterns coexist in the app for however long
  the follow-up migrations take. Reviewer reading the experience-create
  flow will be confused why there are two paradigms.
- **Effort:** Medium first PR, then 3-5 small follow-up PRs.

### Option C: Phased migration in one tracking issue, each surface its own PR

- **Approach:** Build the widget + migrate deadline in PR 1, migrate
  propose_time_form in PR 2, time_poll_propose_modal in PR 3, transfer
  chips in PR 4. All driven from this single issue; each PR is small and
  focused.
- **Pros:** Same incremental safety as Option B, but with an explicit plan
  and timeline rather than open-ended follow-ups. Easier to track
  completion. Each PR can be reviewed by the person closest to that
  surface.
- **Cons:** Slightly more coordination overhead than Option B. Still has
  the dual-paradigm window, but it's bounded.
- **Effort:** Medium total, spread across 4 small PRs.

## Recommendation

**Option C.** The shared widget should be built behind a clean static API
(`DateTimePickerModal.show(...)`) so callers can adopt it in any order,
and the location-poll deadline is the right first caller because (a) it's
the only one still on raw Material pickers and therefore the highest-bang
migration, and (b) it's a lower-traffic surface than the experience
create flow, so any rough edges in the new widget surface in a
lower-stakes context first.

The key tradeoff vs. Option A is dual-paradigm time. Option A spends one
big review cycle to eliminate that risk; Option C accepts a few weeks of
two paradigms in exchange for safer rollout. Given that all the
non-deadline callers are already on the glass-themed pickers (just split
across two sheets rather than one), the dual-paradigm cost is "two taps
vs one tap," not "broken styling vs polished styling." That's an
acceptable transient.

## Open Questions

- **Should the new widget return the deadline-style sentinel for "Remove"
  via a separate callback, or via a result type union?** —
  Recommendation: **Navigator Result Pattern** with a small result type,
  not a callback. [docs/client/modals.md](../client/modals.md) is
  explicit: "Return data via `Navigator.pop(result)` instead of
  callbacks." Define a `DateTimePickerResult` sealed class (Freezed
  union) with two variants — `DateTimePickerResult.saved(DateTime
  value)` and `DateTimePickerResult.removed()`. Cancel resolves to
  `null` (Navigator pop without a result). Callers without a Remove
  path simply construct the modal with `allowRemove: false`, which
  hides the destructive button — they will never receive the
  `removed` variant. This is preferred over a callback because
  callbacks force impedance mismatch in `await`-based call sites and
  contradict the standardized modal pattern.

- **Does the calendar need to support tapping the month label to switch
  to year selection?** — Recommendation: not for v1. The design doesn't
  show it and the `firstDate..lastDate = now..now+365d` window we use
  today means month arrows are sufficient. Add year navigation only if
  follow-up callers need wider ranges (e.g. profile birthday).

- **Time-zone display in the preview line.** — Recommendation: render in
  the device's local zone with a short abbreviation (`PT`, `ET`)
  derived from `DateTime.now().timeZoneName`. Same as the design's "Sat,
  May 16 · 12:00 PM PT (voters see their own zone too)" line. Don't
  introduce a tz-picker affordance; deadline storage stays unix seconds
  as today.

- **Minute granularity.** — Recommendation: 15-minute increments as the
  design shows ("00 15 30 45"). All current callers default to 0-minute
  start times and there's no flow that needs minute precision. Adding a
  `minuteStep` parameter is a follow-up if any caller needs it.

- **What's the screen-reader experience for the wheel widget?** —
  Recommendation: per [docs/client/accessibility.md](../client/accessibility.md),
  wrap each wheel in a `Semantics` widget that exposes
  `increasedValue` / `decreasedValue` so VoiceOver / TalkBack swipe-up
  / swipe-down increment correctly, and use `SemanticAnnouncer.announce`
  (one-shot) when the user spins to a new value. The day-grid cells
  must be `Tappable` (not raw `GestureDetector` — the
  `avoid_raw_gesture_detector` lint rule rejects raw gesture detectors
  in `widgets/`). The close-X is an `IconAction` with a
  `tooltip:` + `semanticsLabel:` pulled from `context.l10n.a11yClose`.
  The preview line should sit inside a `LiveRegion` so changes to the
  selected date or time announce automatically without a manual
  call. Run a manual VoiceOver / TalkBack check before merging the
  first PR.

- **Where should the picker's state live — `StatefulWidget`,
  `Notifier`, or `AsyncNotifier`?** — Recommendation: a regular
  `Notifier` with Freezed state (`DateTimePickerState`) backing the
  modal. The modal has no async work and no repository access, so
  `AsyncNotifier` is overkill — but a `Notifier` lets us write
  ViewModel-style tests (`ProviderContainer` + Mockito-free, since
  there are no services to mock) per
  [docs/client/testing.md](../client/testing.md)'s "test business
  logic, not Flutter widgets" rule. `setState` in a `StatefulWidget`
  is technically allowed for "local UI state" per architecture.md,
  but a 5-field state object (selected day, hour, minute, period,
  visible-month) crosses the line into business logic the wheel-snap
  / clamping rules need to enforce. Use `autoDispose` so the state
  doesn't leak past the modal.

## Implementation Plan

### Phase 1: Build the shared widget
**Goal:** A working `DateTimePickerModal` widget plus tests, not yet
wired into any caller.
**Tasks:**
- [ ] Create `app/lib/presentation/widgets/modal/glass/date_time_picker_modal.dart`
      with a static
      `DateTimePickerModal.show(context, {required initialDateTime, required firstDate, required lastDate, bool allowRemove = false, String? title}) → Future<DateTimePickerResult?>`.
      The static helper internally calls `showAccessibleModal` with
      `isScrollControlled: true`, `backgroundColor: Colors.transparent`,
      `barrierColor: AppColors.modalBackdrop`, and a builder that returns
      `DateTimePickerModal(...)`. **Do not** use `showModalBottomSheet`
      or `showDialog` directly — the former is blocked by the
      `prefer_accessible_modal` lint, and the latter produces a centered
      dialog that doesn't match the design's bottom-sheet shape.
- [ ] Compose the modal body with `GlassSheet` + `GlassModalHeader` +
      `GlassFooterButtons`, and existing `AppColors.modal*` /
      `ModalTheme` tokens. No new theme constants, no `Color()`
      literals, no hardcoded radii. The footer uses
      `GlassFooterButtons` with `showSecondary` controlled by
      `allowRemove` — secondary is destructive Remove (per modals.md
      the secondary slot maps to the left button) and primary is
      Save.
- [ ] Add a `DateTimePickerNotifier` (regular `Notifier.autoDispose`)
      with `DateTimePickerState` (Freezed) tracking
      `selectedDate`, `selectedHour`, `selectedMinute`, `isPm`,
      `visibleMonth`. Place at
      `app/lib/presentation/viewmodels/date_time_picker_view_model.dart`
      following the convention of the existing
      [time_picker_view_model.dart](../../app/lib/presentation/viewmodels/time_picker_view_model.dart).
      Initialize via `initialize(initial:, firstDate:, lastDate:)`
      from a post-frame callback in the modal widget.
- [ ] Split UI into atom files if the main file exceeds ~600 lines
      (the existing
      `location_poll_propose_modal_atoms.dart` precedent is
      `<modal>_atoms.dart` next to the screen):
      `date_time_picker_modal_atoms.dart` holding the calendar grid
      and the wheel rows. Keep the main file under 1000 lines (the
      `npm run lint:dart:size` gate at 1000 lines is CI-enforced).
- [ ] Calendar grid atom: month nav via `IconAction` (icon-only
      button — required `tooltip:` + `semanticsLabel:` from
      `context.l10n`); day cells via `Tappable` with a
      `semanticsLabel` like
      `context.l10n.a11yDayCellLabel('Sat May 16')`; selected day
      uses `AppColors.modalPrimaryButtonBackground` (sage) fill;
      respects `firstDate`/`lastDate` (out-of-range cells are
      disabled and rendered with `modalTextMuted`).
- [ ] Wheel atom: h / m / AM-PM rows with 15-min minute step.
      Mirror the wheel rendering already in
      [duration_picker_modal.dart](../../app/lib/presentation/widgets/experience/duration_picker_modal.dart).
      Wrap each wheel in `Semantics(increasedValue:, decreasedValue:,
      onIncrease:, onDecrease:)` so VoiceOver users can swipe to
      change values. Call `SemanticAnnouncer.announce` on commit.
- [ ] Render the preview line (`EEE, MMM d · h:mm a zzz` via `intl`)
      inside a `LiveRegion` so the screen reader re-announces on
      change.
- [ ] Add ARB strings to both `app_en.arb` and `app_es.arb` in the
      same PR:
      - `dateTimeModalCreateTitle`, `dateTimeModalEditTitle`,
        `dateTimeModalSubtitle`, `dateTimeModalEyebrow`
        (user-visible).
      - `a11yDateTimeModalMonthNext`, `a11yDateTimeModalMonthPrev`,
        `a11yDateTimeModalDayCell` (parametrized),
        `a11yDateTimeModalHourWheel`,
        `a11yDateTimeModalMinuteWheel`,
        `a11yDateTimeModalPeriodWheel` (accessibility-only).
      Reuse existing `commonCancel`, `commonSave`, `commonRemove`,
      `a11yClose`.
- [ ] ViewModel tests at
      `app/test/presentation/viewmodels/date_time_picker_view_model_test.dart`
      following the `ProviderContainer` pattern from
      [docs/client/testing.md](../client/testing.md): cover initial
      state from `initialize`, day selection clamps to
      `firstDate`/`lastDate`, hour/minute/period changes round-trip
      to a `DateTime`, month nav advances `visibleMonth` correctly.
      No service mocks required — the ViewModel has no repository
      dependencies.
- [ ] Add a smoke widget test for the modal at
      `app/test/presentation/widgets/modal/glass/date_time_picker_modal_test.dart`
      that confirms `show()` opens the sheet, Save returns
      `DateTimePickerResult.saved(...)`, Cancel returns `null`, and
      Remove returns `DateTimePickerResult.removed()` only when
      `allowRemove: true`. Use `l10n_helpers.dart` from
      `app/test/helpers/` for localization setup.
- [ ] Run `npm run test:app`, `npm run lint:dart:a11y`,
      `npm run lint:dart:i18n`, `npm run lint:dart:l10n-parity`,
      and `npm run lint:dart:size`.

**Validates:** The widget renders correctly and round-trips a
`DateTimePickerResult` through the API surface. ViewModel tests cover
the wheel-snap and clamping logic without a widget tree. No
production code depends on it yet, so we can iterate on the API
without breakage.

### Phase 2: Migrate the location-poll deadline caller
**Goal:** Ship the new widget in one user-facing flow.
**Tasks:**
- [ ] Replace `_pickDeadline` in
      [location_poll_propose_modal.dart:235](../../app/lib/presentation/screens/experience/location_poll_propose_modal.dart#L235)
      with a single `DateTimePickerModal.show(...)` call. The result
      is a `DateTimePickerResult?`; switch on the sealed variants:
      `saved` → call
      `locationModalProvider(_experienceId).notifier.setLocationPollDeadline(...)`;
      `removed` → call the existing `_clearDeadline` path; `null` →
      no-op (user cancelled). Pass `allowRemove: data?.locationPollDeadlineSec != null`
      so the Remove button only appears when there is an existing
      deadline to clear.
- [ ] Update the ViewModel test in
      `app/test/presentation/viewmodels/location_modal_view_model_test.dart`
      (or wherever the deadline behavior is exercised today) to
      cover both the save and remove paths via mocked result.
      The modal itself is tested in Phase 1 — this phase tests the
      caller's response to each result variant.
- [ ] Manual smoke test on Android and iOS: pick a deadline, edit
      it, remove it, run a poll past the deadline.
- [ ] Verify VoiceOver (iOS) and TalkBack (Android) announce wheel
      values and that the preview-line `LiveRegion` re-announces on
      change.

**Validates:** The widget works in a real production surface. End of this
phase = the original issue scope is satisfied.

### Phase 3: Migrate the experience create / time-poll / preview-edit flows
**Goal:** Single unified date+time UX across all experience flows.
**Tasks:**
- [ ] In
      [propose_time_form.dart](../../app/lib/presentation/widgets/experience/propose_time_form.dart),
      collapse the separate "Date" and "Time" chip pickers into one chip
      that opens the combined modal. Keep the existing relative-date
      chips ("Tomorrow", weekday names) — those are a separate quick-pick
      affordance, not a picker.
- [ ] In
      [time_poll_propose_modal.dart](../../app/lib/presentation/screens/experience/time_poll_propose_modal.dart),
      replace the two-step picker with one.
- [ ] In
      [experience_preview_modal.dart](../../app/lib/presentation/screens/experience/experience_preview_modal.dart)'s
      `_showTimePicker` flow (line 309), route through the combined
      modal so users can adjust date and time together when editing.
- [ ] In
      [unified_time_modal.dart](../../app/lib/presentation/screens/experience/unified_time_modal.dart)
      and its ViewModel
      [time_picker_view_model.dart](../../app/lib/presentation/viewmodels/time_picker_view_model.dart),
      route the "Custom time…" / "Pick another date…" inline actions
      through the combined modal instead of the chained glass
      pickers. This is the highest-traffic experience time surface.
- [ ] Update each surface's **ViewModel** tests
      (`app/test/presentation/viewmodels/...`) per testing.md — UI
      logic stays in the existing widget tests but the bulk of the
      coverage lives at the ViewModel layer.

**Validates:** All experience-creation paths use the combined modal,
fulfilling the author's expanded scope from the issue comment.

### Phase 4: Migrate transfer chip pickers (follow-up)
**Goal:** Apply the same combined modal to the only remaining date+time
pair outside the experience domain.
**Tasks:**
- [ ] Decide whether the transfer pickup flow benefits from a combined
      modal or whether date-only / time-only quick-pickers are still the
      right pattern there (it's a different UX — quick chips, not a
      full proposal form).
- [ ] If yes, migrate
      [quick_date_chips.dart](../../app/lib/presentation/widgets/transfer/quick_date_chips.dart)
      and
      [quick_time_chips.dart](../../app/lib/presentation/widgets/transfer/quick_time_chips.dart).
- [ ] If no, document the deliberate exception in this plan and close it
      out.

**Validates:** Either every date+time pair in the app is on the combined
modal, or we have an explicit, documented exception for transfer chips.

## Related Issues

- None linked from the issue. Worth grepping for issues mentioning
  "showDatePicker" / "showTimePicker" / "deadline picker" before
  starting Phase 1 to make sure no other request would change the API.

## Audit changelog

- **2026-05-22** — Initial draft.
- **2026-05-22 (audit pass)** — Aligned with `docs/client/*` conventions:
  - Modal entry switched from a `showDialog`-style API to
    `showAccessibleModal` + `GlassSheet` (modals.md canonical pattern;
    `prefer_accessible_modal` lint).
  - Result delivery switched from `onRemove` callback to
    Navigator-result pattern with a Freezed `DateTimePickerResult`
    sealed class (modals.md "Return data via `Navigator.pop(result)`").
  - State container clarified as a `Notifier.autoDispose` with Freezed
    state, placed under `presentation/viewmodels/`, to keep tests at
    the ViewModel layer per testing.md.
  - Accessibility primitives (`IconAction`, `Tappable`,
    `LiveRegion`, `Semantics` increase/decrease, `SemanticAnnouncer`)
    promoted from "consider" to explicit Phase-1 tasks per
    accessibility.md.
  - i18n parity gate (`lint:dart:l10n-parity`) and a11y key prefixing
    rules (`a11y*`) called out explicitly per i18n.md and
    accessibility.md; reused `a11yClose` rather than minting a new
    key.
  - Phase 3 expanded to include `UnifiedTimeModal` + its
    `TimePickerViewModel`, which is the actual primary experience
    time surface — the initial draft missed it.
  - File-layout guidance added (`*_atoms.dart` neighbor file) to keep
    under the 1000-line CI cap.
  - No server-side or caching concerns apply (the modal does not
    touch repositories, services, or the cache layer); caching.md and
    server observability.md reviewed and found non-applicable.
