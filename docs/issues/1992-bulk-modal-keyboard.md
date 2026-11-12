# Issue #1992: Cannot see request suggestions

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1992
**Filed:** 2026-05-16T23:01:54Z | **Last activity:** 2026-05-17T02:58:48Z
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When a user opens the **Request something** sheet
([batch_add_need_sheet.dart](app/lib/presentation/widgets/experience/needs/batch_add_need_sheet.dart))
or the **Offer something** sheet
([batch_add_contribution_sheet.dart](app/lib/presentation/widgets/experience/needs/batch_add_contribution_sheet.dart))
on iOS, the suggestion chips (and the bottom "Add to list" / "Save"
action row) are obscured by the on-screen keyboard. The reporter
confirmed in a follow-up comment that the bug repros in both the
"needs bulk modal" and the "contribution bulk modal."

Reproduced on iPhone, iOS 26.4.2, app 0.21.1 build 6979 (PROD).

The sheets share the same chrome and have the same structure: a
header, a pending-items list, a text input field with `autofocus:
true`, an optional suggestions grid, an optional RSVP disclosure,
and a footer action row. Because the input field auto-focuses on
open, iOS raises the keyboard immediately; the sheet content sits
at its natural height anchored to the screen bottom, so anything
below the input field (the entire suggestions section the user is
trying to read) gets covered by the keyboard with no way to scroll
to it.

## Context from Comments

Only one comment, from the issue author (maintainer):

> This happens in both the needs bulk modal and contribution bulk modal

That's important context — the bug is symmetric across both batch
sheets, so the fix needs to land in shared chrome rather than one
sheet's body. No design discussion, no proposed workaround.

## Root Cause

Both sheets are opened through
[`showAccessibleModal`](app/lib/presentation/widgets/accessibility/show_accessible_modal.dart)
with `isScrollControlled: true` and `backgroundColor:
Colors.transparent`, then render their content inside
[`GlassSheet`](app/lib/presentation/widgets/modal/glass/glass_sheet.dart)
via `buildSheetContainer` in
[_sheet_chrome.dart:28-41](app/lib/presentation/widgets/experience/needs/_sheet_chrome.dart#L28-L41).

`isScrollControlled: true` is necessary for keyboard avoidance — it
lets the modal grow past the default half-screen ceiling — but it
does **not** by itself lift the sheet content above the keyboard.
Flutter only re-runs the bottom-sheet's `builder` against an updated
`MediaQuery.viewInsets`; widgets in the subtree have to *consume*
that bottom inset (typically as `EdgeInsets.only(bottom:
viewInsets.bottom)`) for the content to actually move.

Neither batch sheet does that. Their `build()` methods produce a
`Column(mainAxisSize: MainAxisSize.min, ...)` of header → pending
list → input → suggestions grid → action row, with no read of
`MediaQuery.viewInsets.bottom` anywhere
([batch_add_need_sheet.dart:182-289](app/lib/presentation/widgets/experience/needs/batch_add_need_sheet.dart#L182-L289),
[batch_add_contribution_sheet.dart:195-291](app/lib/presentation/widgets/experience/needs/batch_add_contribution_sheet.dart#L195-L291)).
The shared input field has `autofocus: true` at
[_sheet_chrome.dart:404](app/lib/presentation/widgets/experience/needs/_sheet_chrome.dart#L404),
which immediately summons the keyboard the moment the sheet opens.

There's a precedent for the correct pattern in
[feedback_sheet.dart:54-61](app/lib/presentation/widgets/feedback/feedback_sheet.dart#L54-L61),
which threads keyboard insets into its `GlassSheet` padding and
wraps the body in a `Flexible(SingleChildScrollView(...))` so the
content lifts above the keyboard and can be scrolled if it still
overflows:

```dart
return GlassSheet(
  padding: EdgeInsets.only(
    bottom: MediaQuery.of(context).viewInsets.bottom,
  ),
  child: ...
);
```

[`docs/client/modals.md:29`](docs/client/modals.md#L29) advertises
"Automatic height expansion (sheet auto-resizes when keyboard appears
via `isScrollControlled: true`)" — that wording is misleading
(`isScrollControlled` is a *necessary* but not sufficient condition)
and the bulk sheets fell into the trap it implies.

The same omission exists in the other needs sheets that auto-focus
a text field
([`add_contribution_sheet.dart`](app/lib/presentation/widgets/experience/needs/add_contribution_sheet.dart),
[`edit_contribution_sheet.dart`](app/lib/presentation/widgets/experience/needs/edit_contribution_sheet.dart)),
but the reporter only flagged the two batch sheets — those are the
ones with the suggestions grid below the input, where the missing
inset is most visible. The fix should cover them all because the
chrome is shared and not fixing the siblings would just re-file the
bug.

## Potential Solutions

### Option A: Apply `viewInsets.bottom` inside `buildSheetContainer`

**Approach:** Change the shared
[`buildSheetContainer`](app/lib/presentation/widgets/experience/needs/_sheet_chrome.dart#L28-L41)
helper to read `MediaQuery.of(context).viewInsets.bottom` and pass
it as the bottom padding of the wrapping `GlassSheet` (replacing or
augmenting the current `kSheetBottomPadding` const). Every needs
sheet that already wraps in `buildSheetContainer` picks up the fix
automatically — both batch sheets, plus `add_contribution_sheet`,
`edit_contribution_sheet`, `view_contribution_sheet`, and
`view_need_sheet`.

- **Pros:** Smallest possible diff (one helper, ~5 lines). Cures the
  reported bug **and** preempts the same symptom in the four
  sibling sheets that share the chrome. Matches the established
  pattern in `feedback_sheet.dart`. No layout reshuffling — the
  sheet just slides up the height of the keyboard.
- **Cons:** If the combined natural height of the sheet content
  *plus* the keyboard exceeds 92% of screen height (the
  `ModalTheme.sheetMaxHeightFraction` cap inside `GlassSheet`),
  Flutter's `Flexible` will clip rather than scroll because the
  Column body isn't scrollable. On a small phone (iPhone SE, ~568
  pt tall) with several pending items plus suggestions plus a
  visible RSVP disclosure, this is plausible.
- **Effort:** Small (≤ 15 lines + a unit/widget test).

### Option B: Option A plus a `SingleChildScrollView` around the body

**Approach:** Do Option A and also wrap each batch sheet's `Column`
body in a `SingleChildScrollView` (or refactor the shared chrome to
expose a scrollable variant). This matches `feedback_sheet`'s full
pattern: `Flexible(SingleChildScrollView(Column(...)))` inside a
keyboard-padded sheet.

- **Pros:** Belt-and-braces. Handles the case where the sheet
  doesn't fit even after the keyboard inset is applied (small phone
  + many pending items + RSVP disclosure visible). Future-proofs
  against adding more sections.
- **Cons:** Larger refactor. The footer `GlassFooterButtons` is
  meant to be visually pinned at the bottom of the sheet with a
  hairline divider above; putting it inside a scroll view means it
  scrolls with the content. Keeping it pinned requires
  restructuring (e.g. `Column[Flexible(scroll(body without
  footer)), footer]`), which touches both batch sheets and possibly
  the other needs sheets too.
- **Effort:** Medium (~50-80 LOC across files; needs careful manual
  testing on small + large viewports).

### Option C: Reorder the UI so suggestions sit *above* the input

**Approach:** Move the suggestions grid (and the "Agree on Time"
row in the need sheet) above the input field so that, even with the
keyboard up, the suggestions are always visible. The input and
action row would sit at the bottom of the sheet, where the keyboard
would naturally rest against them.

- **Pros:** Doesn't require any keyboard-inset plumbing. Solves the
  bug semantically.
- **Cons:** Significant UX/design change to a flow the user already
  knows. The current order matches the natural reading flow ("type
  what you need, or pick a suggestion"). Suggestions appear after
  the input on purpose so an empty list doesn't dominate the sheet.
  Reordering would also require l10n review — the hints
  (`experienceBatchNeedInputHint` etc.) lean on the input being the
  primary affordance.
- **Effort:** Small-to-medium code change, but the design review and
  potential regressions push the real cost higher.

## Recommendation

**Option A, with a follow-up monitor for the overflow edge case.**

The reported bug is a missing `viewInsets.bottom` read — putting
that one line into the shared chrome is the smallest, most targeted
fix and it transparently covers all six needs sheets. Doing it in
`buildSheetContainer` instead of in each sheet's `build()` reuses
the existing chrome helper exactly the way it was designed (single
place for shared modal layout) and matches `feedback_sheet`'s
proven pattern.

Option B is the *right* long-term shape but is a much bigger change
and pulls in a separate concern (overflow scrollability). It's
worth holding back until we have evidence that the keyboard-inset
fix alone doesn't suffice — most users with a few pending items on
a normal-sized phone will be fine. The smoke test in Phase 2 below
will tell us.

Option C trades a bug for a UX redesign and isn't justified by this
report.

## Open Questions

- **Should the inset go inside `buildSheetContainer` or be added
  per-sheet?** Recommendation: inside `buildSheetContainer`. The
  helper is the single chokepoint for all needs sheets' top-level
  `GlassSheet` wrapper; pushing the inset there makes every current
  and future caller correct by construction. Per-sheet would leave
  the bug latent in any sheet that forgets the line.

- **Should we also wrap the body in `SingleChildScrollView` now?**
  Recommendation: no, not in this PR. Defer to a follow-up issue if
  manual smoke-testing on iPhone SE shows residual clipping. The
  bulk sheets are vertically modest (header + ~3 list items +
  input + ~6 suggestion chips + footer ≈ 600 pt without keyboard)
  and the keyboard is ~336 pt on iOS, which fits inside 92% of an
  iPhone SE (~520 pt usable). The risk is the multi-pending +
  RSVP-disclosure combo on the contribution sheet — capture that
  in the smoke-test matrix.

- **Should the fix also touch the other sheets that auto-focus
  (`add_contribution_sheet`, `edit_contribution_sheet`)?**
  Recommendation: yes, by virtue of Option A landing in the shared
  helper. They all wrap through `buildSheetContainer`, so a single
  edit fixes the whole family without per-sheet diffs. View sheets
  (`view_contribution_sheet`, `view_need_sheet`) don't auto-focus
  inputs but harmlessly benefit from the same defensive padding.

- **Should `docs/client/modals.md`'s "Automatic height expansion"
  claim be corrected?** Recommendation: yes — in this PR's docs
  scope. Add one sentence clarifying that `isScrollControlled:
  true` *enables* keyboard handling but content widgets still have
  to consume `MediaQuery.viewInsets.bottom`, with a pointer to
  `feedback_sheet.dart` as the reference implementation. The
  current wording is what made the bulk sheets miss the pattern.

- **Should a regression test pin the fix?** Recommendation: yes —
  but as a **pure unit test on an extracted helper**, not as a
  widget pump. Per `docs/client/testing.md` ("test business logic,
  not Flutter widgets") and the precedent set by #1933's
  `_mergeForBottomSheet`, extract `EdgeInsets
  resolveBatchSheetPadding({required double base, required double
  keyboardInset})` and assert that with `base: 16` and
  `keyboardInset: 0` it returns `bottom: 16`, and with
  `keyboardInset: 336` it returns `bottom: 352` (sum, not max — see
  next question). The helper is a pure function, so the test needs
  no `ProviderContainer`, no widget pump, no `MaterialApp`. The
  `buildSheetContainer` call site stays thin: read
  `MediaQuery.of(context).viewInsets.bottom`, pipe it through the
  helper, hand the result to `GlassSheet`. This matches the
  ViewModel-first stance while still pinning the fix.

- **Sum or `max` when combining `kSheetBottomPadding` and
  `viewInsets.bottom`?** Recommendation: **sum**. The 16-pt
  `kSheetBottomPadding` is intended as visual breathing room above
  whatever sits below the content (home indicator with no keyboard,
  keyboard with one). `max` would let the keyboard butt up against
  the last sheet element with no buffer, regressing the spacing the
  constant is there to provide. `feedback_sheet.dart` happens to
  replace the default padding outright because it owns its own
  inner padding via the `SingleChildScrollView`; we don't, so we
  add rather than replace.

## Implementation Plan

### Phase 1: Thread `viewInsets.bottom` through `buildSheetContainer`

**Goal:** Make every needs sheet keyboard-aware via a single
shared change.

**Tasks:**
- [ ] Extract a pure helper in
  [_sheet_chrome.dart](app/lib/presentation/widgets/experience/needs/_sheet_chrome.dart),
  for example:
  `EdgeInsets resolveBatchSheetPadding({required double horizontal,
  required double top, required double base, required double
  keyboardInset}) => EdgeInsets.fromLTRB(horizontal, top,
  horizontal, base + keyboardInset);`. Keep the helper at the top
  of the file alongside the `kSheet*Padding` constants so the
  intent ("base padding is breathing room; keyboard inset stacks
  on top") is co-located with the constants it composes.
- [ ] Update
  [_sheet_chrome.dart:28-41](app/lib/presentation/widgets/experience/needs/_sheet_chrome.dart#L28-L41)
  to read `MediaQuery.of(context).viewInsets.bottom` and pipe both
  values through `resolveBatchSheetPadding(...)` before handing
  the result to `GlassSheet`. The function body stays a few lines
  — the pure helper carries the logic and the test.
- [ ] Add a one-line comment on the changed call site linking #1992
  and noting that `isScrollControlled: true` on
  `showAccessibleModal` is required but not sufficient — consumers
  must apply `viewInsets.bottom` themselves. Issue numbers are
  explicitly allowed in source per `CLAUDE.md`'s "Go Code Comments"
  rules; no phase markers (those would fail
  `scripts/check_ephemeral_comments.js`).
- [ ] Add a pure unit test under
  `app/test/presentation/widgets/experience/needs/sheet_chrome_test.dart`
  (create if needed) covering `resolveBatchSheetPadding`:
  (a) `keyboardInset: 0` returns the base bottom padding unchanged;
  (b) `keyboardInset: 336` returns base + 336 on bottom and leaves
  horizontal/top unchanged;
  (c) `keyboardInset: 0` with `base: 16` proves the home-indicator
  breathing room is preserved when no keyboard is up. The helper
  is a pure function — no `ProviderContainer`, no widget pump, no
  `MaterialApp` needed (matches `docs/client/testing.md`'s
  "ViewModel-first" stance and the `_mergeForBottomSheet` pattern
  from #1933).

**Validates:** Both reported sheets and their siblings consume the
keyboard inset; a regression test prevents future drift.

### Phase 2: Manual iOS smoke test

**Goal:** Confirm the screenshot scenario is gone on a real iOS
viewport and that no overflow surfaces on the smallest supported
device.

**Tasks:**
- [ ] On iOS simulator (iPhone 15 / iOS 26 to match the reporter's
  device class): open an experience, tap "Request" — confirm the
  sheet lifts above the keyboard, the suggestion chips are
  readable, and "Add to list" / "Save" are tappable without
  dismissing the keyboard.
- [ ] Repeat for "Offer" → `BatchAddContributionSheet`. Confirm
  the same and that the RSVP disclosure (visible when
  `isRsvped == false`) is still reachable.
- [ ] Repeat on iPhone SE (small viewport): add 3-4 pending items,
  open the contribution sheet with RSVP disclosure visible, focus
  the input. If the content clips above the suggestions, file a
  follow-up issue (Option B scope) but do not block this PR — the
  reported regression is gone and the residual is a separate UX
  ticket.
- [ ] Quick check on Android emulator to confirm no regression on
  a platform that wasn't reported.

**Validates:** The user-reported symptom no longer reproduces, and
we have a documented signal on whether the deferred scroll fix is
needed.

### Phase 3: Doc correction and close the loop

**Goal:** Make the next developer who builds a glass bottom sheet
get keyboard handling right by default.

**Tasks:**
- [ ] In [docs/client/modals.md:29](docs/client/modals.md#L29),
  clarify that `isScrollControlled: true` is necessary but not
  sufficient for keyboard handling and that content widgets must
  apply `MediaQuery.of(context).viewInsets.bottom` themselves. Add
  `feedback_sheet.dart` + the updated `_sheet_chrome.dart` as
  reference implementations.
- [ ] Open PR referencing #1992. PR description shows the
  before/after iOS screenshots and links to this plan.
- [ ] After merge, post a short comment on the issue with the PR
  link and close.

**Validates:** The misleading doc claim that led to the bug is
corrected, the fix is shipped, and the issue is traceable from a
future bug report.

## Related Issues

- [#1933 — IOS keyboard hides input area](docs/issues/1933-keyboard-hides-input.md)
  — Same family of bug (iOS keyboard occluding modal content) but
  in `unified_create_modal`. Root cause there was a frozen-at-open
  `MediaQuery` restatement; root cause here is a missing
  `viewInsets.bottom` read entirely. Mention in the PR as
  thematically related; the fixes are independent. The two fixes
  also share a structural pattern — extract a pure helper, unit-test
  it as a pure function — which keeps the project's testing
  conventions in sync.

## Audit notes (`/ripls-audit`)

Audited 2026-05-16 against `docs/client/architecture.md`,
`docs/client/testing.md`, `docs/client/accessibility.md`,
`docs/client/i18n.md`, `docs/client/caching.md`,
`docs/server/architecture.md`, and `docs/server/observability.md`.

| Concern | Finding | Resolution in plan |
|---------|---------|--------------------|
| MVVM / Riverpod boundaries | Fix is a pure layout change in a shared widget helper. No ViewModel, repository, or service involvement. | Compliant — no change. |
| Testing philosophy ("test business logic, not Flutter widgets") | Original Phase 1 proposed a widget test that pumps `BatchAddNeedSheet` and reads `GlassSheet`'s padding via `tester.widget<GlassSheet>()`. That conflicts with the project's ViewModel-first stance. | **Updated Phase 1** to extract a pure `resolveBatchSheetPadding` helper and unit-test it as a pure function — same approach #1933 took with `_mergeForBottomSheet`. No `ProviderContainer` or widget pump required. |
| Padding semantics (`sum` vs `max`) | Plan left this ambiguous in Open Questions. The 16-pt `kSheetBottomPadding` is breathing-room above the home indicator, which is just as needed above the keyboard. | **Pinned to `base + keyboardInset`** (sum, not max). Documented why `feedback_sheet.dart`'s replace-the-default pattern doesn't apply here. |
| Accessibility primitives | Fix doesn't touch any `Tappable`, `IconAction`, `Semantics`, or focus path. No new strings, so no new `a11y*` l10n keys. | Compliant — no change. |
| Accessibility — autofocus screen-reader behaviour | `_sheet_chrome.dart:404`'s `autofocus: true` will move the screen-reader cursor to the text field before the sheet's title is announced. Not new in this PR and not the cause of #1992, but worth a follow-up to debounce autofocus until after sheet-open announcement. Out of scope for the user-reported bug. | **Flagged** in Audit notes only — no plan change. File a follow-up issue if VoiceOver smoke-test in Phase 2 confirms the regression. |
| i18n | No new user-visible strings. The widget chrome (`buildSheetContainer`) is invisible to users. | Compliant — no change. |
| Modal standards (`docs/client/modals.md`) | The doc claims auto height-expansion via `isScrollControlled: true` — wording that is the root cause of this bug. | **Phase 3** already corrects the doc with a one-line clarification and adds the updated `_sheet_chrome.dart` as a second reference implementation alongside `feedback_sheet.dart`. |
| Caching | No data access; no cache key or repository involvement. | N/A. |
| Server boundary | Pure client-side rendering fix; no RPC, no proto, no `connecterr.Internal` plumbing. | N/A. |
| Server observability / structured logging | N/A — client fix. | N/A. |
| Ephemeral comments policy (`CLAUDE.md` "Go Code Comments" rules, mirror-applied to Dart via `scripts/check_ephemeral_comments.js`) | Phase 1 adds a `// fixes #1992` style anchor on the call site. Issue numbers are explicitly allowed; no phase markers proposed. | Compliant. |
