# Issue #1727: Save Button Hidden When Editing

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1727
**Filed:** 2026-05-05 | **Last activity:** 2026-05-05
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When the user enters edit mode on a Gear, Experience, or Request detail
screen — even just to tweak the description — the **Save** pill rendered by
`ContentEditBar` gets covered by the parent screen's **Info (i)** button.
The Save pill is positioned at the right edge of the top bar; the Info
button sits at the same right edge in a separate `Positioned` widget that
is layered *after* the content view in the `Stack`, so the Info icon paints
on top of Save and the user has to fish around the Info button to hit Save.

Reproduction (per the screenshot in the report):

1. Open a piece of Gear (or an Experience/Request) you own on iOS.
2. Tap into edit mode and modify the description.
3. The blue Save pill appears at the top-right but the round Info button
   overlaps it, and tapping the visible Save pill area sometimes opens the
   Info modal instead of saving.

## Context from Comments

The only comment is `@claude` from a maintainer requesting Claude to take
the issue. There is no additional discussion clarifying the desired UX
(whether Watch and Back should also disappear during edit, etc.).

## Research

The same overlap pattern exists on three near-identical screens:

- `app/lib/presentation/screens/gear/gear_screen.dart` — `_buildAppBar()` at
  lines 138–165 always renders Back + Watch + Info regardless of edit
  state.
- `app/lib/presentation/screens/experience/experience_screen.dart` —
  `_buildAppBar()` at lines 139–166, same structure.
- `app/lib/presentation/screens/request/request_screen.dart` —
  `_buildAppBar()` at lines 138–165, same structure.

Each parent screen wraps its `*ContentView` in a `Stack` and then layers
the app bar on top. The corresponding `*ContentView` already contains
`ContentEditBar` (`app/lib/presentation/widgets/content/content_edit_bar.dart`),
which renders Cancel ↔ "Editing" ↔ Save across the full top of the screen.

There is already prior art for hiding the right-side icons in edit mode:
when `GearContentView` is rendered standalone (e.g. inside the feed) it
gates its own info button on `state.isEditing`:

```dart
// app/lib/presentation/screens/gear/gear_content_view.dart:388-389
// 5. Info button — upper right, hidden while editing
if (!state.isEditing) _buildInfoButton(isNavVisible, state),
```

`experience_content_view.dart:497` and `request_content_view.dart:446`
follow the same pattern internally. The bug exists only because the parent
screens (`gear_screen`, `experience_screen`, `request_screen`) render an
**additional** app bar on top of the content view that does not honor the
edit state.

The relevant `isEditing` flags are already on each view-model state:
- `GearState.isEditing` (`presentation/viewmodels/gear_view_model.dart:44`)
- `ExperienceState.isEditing` (`presentation/viewmodels/experience_view_model.dart:37`)
- `RequestState.isEditing` (`presentation/viewmodels/request_state.dart:31`)

So no new state plumbing is required.

## Potential Solutions

### Option A: Hide only the right-side icons (Watch + Info) when editing
- **Approach:** In each `_buildAppBar()`, watch the relevant
  `*Provider(id)` for `state.isEditing` and skip the Watch + Info icons
  when true. Back button stays visible. Cancel/Save are provided by the
  already-on-screen `ContentEditBar`.
- **Pros:** Smallest change. Mirrors the existing pattern in
  `gear_content_view.dart:389`. No change to UX expectations beyond
  what users already see when the same content view is rendered in
  the feed.
- **Cons:** Three files duplicate the same conditional. Back button
  during edit mode is debatable (tapping Back with unsaved changes
  silently discards them — but that's pre-existing behavior, not new).
- **Effort:** Small.

### Option B: Hide the entire parent app bar when editing
- **Approach:** In each parent screen, wrap the `_buildAppBar()` call in
  the Stack with `if (!state.isEditing)`.
- **Pros:** Cleanest visual result — only `ContentEditBar` is shown at
  the top during edit. Eliminates the Back-discards-edits ambiguity.
- **Cons:** The user loses the back affordance entirely until they
  Cancel or Save. That's a behavior change — a user who taps into edit
  by accident now has only the Cancel pill (which is what they should
  use anyway, but it's a different mental model).
- **Effort:** Small.

### Option C: Extract a shared `ContentScreenAppBar` widget that gates on isEditing
- **Approach:** Create
  `presentation/widgets/content/content_screen_app_bar.dart` that takes
  a back handler, an optional watch button, an info-tap callback, and
  an `isEditing` flag, then collapses to back-only (or nothing) when
  editing. Rewrite all three parent screens to use it.
- **Pros:** Removes the three-way duplication. Future content screens
  (e.g. a new entity type) inherit the correct behavior.
- **Cons:** Larger surface area for a one-line bug. The three call
  sites have small differences (different Watch state sources,
  different info modal payloads) that the shared widget needs to
  thread through. Worth doing as cleanup but not on a hotfix branch.
- **Effort:** Medium.

## Recommendation

**Option A.** The bug is a positioning collision; the minimal fix is to
honor `isEditing` in each `_buildAppBar()` exactly the way each
`ContentView` already does internally for the feed-embedded case. Keeping
the Back button visible matches the rest of the app's edit flows
(`community_content_view.dart` does not hide its back affordance during
edit either) and avoids stranding a user who entered edit mode and wants
to bail out via the OS-style Back chevron.

Option C is worth filing as a follow-up cleanup but is out of scope for
this user-feedback bug.

## Open Questions

- **Should Back also be hidden during edit (per Option B)?** Recommendation:
  No — keep Back visible. Tapping Back during edit currently pops the
  screen and discards the in-progress edit; that's pre-existing behavior
  and changing it is a separate UX call. The reporter's complaint is
  specifically about Save being unreachable, not about Back.
- **Should we factor a shared widget (Option C)?** Recommendation: No,
  not on this PR. File a follow-up cleanup issue once #1727 is fixed.
- **Are there iPad / landscape layouts where the overlap is different
  enough to need a separate fix?** Recommendation: Assume no — the
  parent app bar and `ContentEditBar` both anchor right with identical
  16-pt padding, so they collide on every form factor. If a tester
  surfaces a layout-specific case, treat as a follow-up.
- **Regression test?** Recommendation: Add a single widget test against
  one screen (e.g. `gear_screen`) that pumps the screen with
  `GearState.isEditing = true` and asserts the Info button is absent.
  A focused test against one of the three screens gives us a regression
  guard without the full mocking burden of testing all three;
  the other two follow the same one-line conditional and are covered
  by code review and visual QA.

## Implementation Plan

### Phase 1: Hide right-side icons when editing
**Goal:** Resolve the overlap so the Save pill is fully tappable.

**Tasks:**
- [ ] In `app/lib/presentation/screens/gear/gear_screen.dart`,
      change `_buildAppBar()` to also watch `gearProvider(widget.gearId)`
      for `state.isEditing` and gate the
      `if (gear != null) ...[ WatchIconButton(...), const SizedBox(width: 8), ContentIconButton(...info_outline...) ]`
      block on `!state.isEditing` in addition to the existing `gear != null`
      check. Leave the Back button outside the gate.
- [ ] Apply the analogous change in
      `app/lib/presentation/screens/experience/experience_screen.dart`
      (gate on `experienceProvider(widget.experienceId).isEditing`).
- [ ] Apply the analogous change in
      `app/lib/presentation/screens/request/request_screen.dart`
      (gate on `requestProvider(widget.requestId).isEditing`).

**Validates:** Visual check — entering edit mode on each of the three
screen types removes the Watch + Info icons, leaving the Save pill from
`ContentEditBar` unobstructed at the right edge of the top bar.

### Phase 2: Regression test
**Goal:** Lock in the fix so a future refactor cannot reintroduce the
overlap.

The repository's testing guidance (`docs/client/testing.md`) is "test
business logic, not Flutter widgets" — but `isEditing`-gated rendering
is the actual bug here, so a lightweight UI assertion is warranted.
Two paths considered; recommend the second:

- **Heavy path:** Pump `GearScreen` with a `gearProvider` override and
  use `find.byIcon(Icons.info_outline)`. Rejected: `GearScreen` requires
  `auth`, `selectedCommunity`, `home`, several repositories, and
  `SwipeToCloseMixin` machinery — mocking them all is disproportionate
  for a one-line conditional.
- **Recommended path:** Extract the right-side icon row in each
  `_buildAppBar()` into a tiny in-file private widget (e.g.
  `_AppBarTrailingIcons` taking `isEditing`, `isWatched`, `onToggleWatch`,
  `onShowInfo`) that returns `SizedBox.shrink()` when `isEditing` is true.
  Test that widget directly with `pumpWidget(MaterialApp(home: ...))`
  — no providers needed.

**Tasks:**
- [ ] In each of the three screens, extract the watch + info row into a
      private `_AppBarTrailingIcons` widget that takes `isEditing` plus
      the small handlers it needs.
- [ ] Add `app/test/presentation/screens/gear/gear_screen_app_bar_test.dart`
      (and analogous files for experience and request) that pumps the
      private widget twice — once with `isEditing: false` (asserts the
      Info icon is present) and once with `isEditing: true` (asserts
      `find.byIcon(Icons.info_outline)` returns nothing). Keep the test
      file under 60 lines.

**Validates:** `flutter test app/test/presentation/screens/` passes on
the fix branch and fails on `main`. The CI a11y check
(`npm run lint:dart:a11y`) is unaffected — we are *hiding* icons whose
existing `tooltip:` / `semanticsLabel:` already came from
`context.l10n.a11yShowDetails`, not adding new ones.

### Phase 3: Manual QA & cleanup
**Goal:** Confirm on-device.

**Tasks:**
- [ ] Run the iOS simulator. Open a piece of owned gear, tap edit, edit
      the description, confirm Save is reachable. (`npm run test:app`
      already gates analyzer warnings via `flutter analyze` in CI.)
- [ ] Repeat for an Experience and a Request (each owned by the
      logged-in test user; non-owners do not see the edit affordance).
- [ ] `npm run lint:dart`, `npm run lint:dart:a11y`, and
      `npm run lint:dart:i18n` clean. No new ARB keys are introduced
      so `i18n` should be a no-op.

**Validates:** End-to-end manual repro of the screenshot scenario passes
on iOS (the original report) and on Android (sanity check that the fix
isn't iOS-only).

## Architectural Compliance Notes

- **MVVM boundary preserved.** The fix reads `isEditing` from existing
  view-model state via `ref.watch()` in `build()`. No new state, no new
  notifier methods, no widget-side business logic. Matches the rule
  "Screens only interact with ViewModels" in `docs/client/architecture.md`.
- **No caching impact.** Pure UI conditional; no repository or service
  calls change.
- **Accessibility ratchet preserved.** No new icons, no new tooltips,
  no new strings. The `scripts/a11y_allowlist.txt` empty-baseline
  invariant holds.
- **i18n unchanged.** The extracted private widget keeps the existing
  `context.l10n.a11yShowDetails` semantics label untouched; no ARB
  edits required.
- **No server-side change.** `isEditing` is client-only state on the
  view-model — the bug is purely client.

## Related Issues
- None directly. The internal `_buildInfoButton` pattern in
  `gear_content_view.dart`, `experience_content_view.dart`, and
  `request_content_view.dart` predates this bug and is the template
  the fix copies.
