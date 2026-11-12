# Issue #1108: [Bug Report] iPhone SE doesn't fit lend / giveaway buttons

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1108
**Filed:** 2026-04-13 | **Last activity:** 2026-04-13
**Labels:** bug, user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

On small-screen devices (iPhone SE) and iPhones with large/accessibility text
sizes, the "Lend" and "Giveaway" action buttons on the gear creation preview
screen are not visible. The user (Susan) cannot complete a giveaway because
the buttons are pushed off the bottom of the viewport. (The bug report was
filed from a Pixel 9 Pro, but the issue is on Susan's iPhone SE.)

The root cause is that all three content creation preview modals
(`GearPreviewModal`, `ExperiencePreviewModal`, `RequestPreviewModal`) use a
non-scrollable `Column` layout with `KeyboardActions(disableScroll: true)`.
On small screens, the combined height of header + media picker + editable
fields + metadata/location selectors + action buttons exceeds the available
viewport height, causing the bottom action buttons to overflow and disappear.

The layout pattern in all three screens is identical:

```dart
Column(
  children: [
    _buildHeader(),
    const Spacer(),          // Absorbs space on large screens, but can't go below 0
    MediaPickerButton(...),
    _buildDetails(),         // Title + Description (4-8 lines) + selectors
    _buildActionButtons(),   // <-- Gets pushed off-screen
  ],
)
```

## Context from Comments

- **Bryan (maintainer):** "I think an easy short-term fix for this would be to
  make the creation screens scrollable." — This directly points to the fix: wrap
  the content in a scrollable container.

## Potential Solutions

### Option A: Wrap content Column in SingleChildScrollView

- **Approach:** Replace the non-scrollable `Column` with `CustomScrollView` +
  `SliverFillRemaining(hasScrollBody: false)`. This is Flutter's built-in
  pattern for "fill the viewport but scroll if content overflows." `Spacer()`
  works normally inside the sliver. `KeyboardActions` stays `disableScroll: true`
  since the `CustomScrollView` handles scrolling.
- **Pros:** Minimal structural change. Preserves the existing visual layout on
  large screens. Buttons are always reachable by scrolling. No performance cost.
- **Cons:** None significant.
- **Effort:** Small.

### Option B: Pin action buttons in a fixed bottom bar

- **Approach:** Move action buttons out of the scrollable column and into a
  `Scaffold.bottomNavigationBar` or a `Positioned` bottom overlay, similar to
  how `TransferModalShell` separates content from `bottomBar`.
- **Pros:** Buttons are always visible regardless of scroll position.
- **Cons:** Requires restructuring the layout from a single Stack+Column to a
  split content/bottom-bar pattern. The background image + gradient overlay
  would need adjustment to avoid the bottom bar area looking disconnected.
  More invasive change.
- **Effort:** Medium.

### Option C: Reduce content height on small screens

- **Approach:** Use responsive sizing (smaller fonts, reduced padding, fewer
  description lines) on small screens.
- **Pros:** No layout restructuring needed.
- **Cons:** Doesn't actually solve the problem — just pushes the threshold to
  even smaller screens. Fragile. Also degrades the UX on small screens.
- **Effort:** Medium (many conditional size values).

## Recommendation

**Option A** — wrapping the content in a `SingleChildScrollView`. This is the
simplest fix, directly matches the maintainer's suggestion, and handles all
screen sizes gracefully. The `Spacer()` still pushes content to the bottom on
large screens, and on small screens users simply scroll to reach the buttons.

## Open Questions

- **Should the keyboard-open state scroll to the focused field?**
  Recommendation: Yes. Changing `disableScroll: false` on `KeyboardActions`
  enables this automatically — when the keyboard opens, the focused text field
  scrolls into view.

- **Should we add a bottom fade/gradient hint when content is scrollable?**
  Recommendation: No, not for this fix. The screens already have a dark
  gradient overlay, and the content amount is small enough that users will
  naturally discover scrolling. Can revisit if user feedback indicates
  discoverability issues.

## Implementation Plan

### Phase 1: Make all three preview modals scrollable

**Goal:** Action buttons are reachable on all screen sizes.

**Tasks:**
- [x] `GearPreviewModal` (`app/lib/presentation/screens/gear/gear_preview_modal.dart`):
  - Replaced `SafeArea > Padding > Column` with `SafeArea > Padding >
    CustomScrollView > SliverFillRemaining(hasScrollBody: false) > Column`
  - `KeyboardActions` stays `disableScroll: true`
- [x] `ExperiencePreviewModal` (`app/lib/presentation/screens/experience/experience_preview_modal.dart`):
  - Same pattern
- [x] `RequestPreviewModal` (`app/lib/presentation/screens/request/request_preview_modal.dart`):
  - Same pattern
- [x] Tested on iPhone SE 3rd gen simulator — buttons visible, scrolling works
- [ ] Test on large screen to verify `Spacer()` still pushes content to bottom
  (no visual regression)
- [ ] Test keyboard interaction: opening keyboard should scroll focused field
  into view

**Validates:** Buttons are visible and tappable on iPhone SE and similarly
small screens. No visual regression on standard and large screens.

## Related Issues

- #853 — Android layout crash (prior overflow issue, different screen)
- #804 — Consent modal overflow (similar small-screen layout overflow pattern)

## Audit

**Reviewed against:** `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/server/architecture.md`

**Result: Compliant.** This is a pure UI layout fix in screen widgets. No
architectural boundaries are crossed — no changes to state management, data
access, caching, or server/client contracts. No new strings added (existing
hardcoded strings like 'Lend' and 'Giveaway' are pre-existing and outside
scope of this fix).
