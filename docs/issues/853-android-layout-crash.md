# Issue #853: Android crashlytics layout crash

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/853
**Filed:** 2026-03-17 | **Last activity:** 2026-03-17
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

A production crash on Android (version 0.6.0, build 4469) where Flutter's
rendering engine throws `RenderBox was not laid out: RenderPadding#90200` during
`performLayout()`. The crash originates in a `RenderFlex` (Row/Column) laying
out a child that contains a `_RenderCustomClip` (ClipRRect/ClipOval/ClipRect),
with two proxy box layers and then a `RenderPadding` that was never laid out.

The key stack frames:
```
RenderFlex._computeSizes → ChildLayoutHelper.layoutChild →
  _RenderCustomClip.performLayout →
    RenderProxyBoxMixin.performLayout →
      RenderProxyBoxMixin.performLayout →
        RenderBox.size → CRASH (hasSize assertion failed)
```

The crash happened once on Android, so it is likely a layout edge case — either
screen rotation, keyboard appearance, or an unusual widget state caused
unbounded or zero-height constraints to propagate through a `ClipRRect` +
`IntrinsicHeight` combination.

## Context from Comments

No comments on this issue. The report consists solely of the Crashlytics stack
trace. There's no reproduction path, no indication of what screen the user was
on, and no device model specified beyond "Android".

## Codebase Analysis

### Suspect pattern: `IntrinsicHeight` + `ClipRRect` + `Row(crossAxisAlignment: stretch)`

Three widgets use the same problematic layout pattern where `IntrinsicHeight`
wraps a `Row` with `CrossAxisAlignment.stretch` inside a `ClipRRect`:

**1. `ContentActionBar`** (`app/lib/presentation/widgets/content/content_action_bar.dart:108-127`)
```dart
ClipRRect(
  child: Material(
    child: IntrinsicHeight(
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _buildHelperSection(),    // Expanded
          _buildVerticalDivider(),  // Container(width: 1) — no height
          _buildChatPill(context),  // Ink (no intrinsic height)
          _buildVerticalDivider(),
          _buildActionButton(),     // KeyedSubtree → MorphingActionButton
        ],
      ),
    ),
  ),
)
```

**2. `CommentBar`** (`app/lib/presentation/widgets/content/comment_bar.dart:87-98, 101-144, 201-236`)
- `_buildEmpty()`: Same `IntrinsicHeight > Row > [Expanded, divider, actionButton]` pattern
- `_buildWithPreview()`: `Column > [Ink, divider, IntrinsicHeight > Row > ...]`

**3. `InboxActionCard`** (`app/lib/presentation/widgets/portfolio/inbox_action_card.dart:272-297`)
```dart
IntrinsicHeight(
  child: Row(
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: [
      Expanded(child: GestureDetector(...)),
      _buildReviewButton(context),  // Container with no height constraint
    ],
  ),
)
```

### Why `IntrinsicHeight` is dangerous here

`IntrinsicHeight` performs a two-pass layout:
1. First pass: queries all children's `getMaxIntrinsicHeight()` to determine the
   tallest child
2. Second pass: lays out all children with that computed height

If any child fails to properly report intrinsic dimensions, the computation can
fail. Key risk factors:
- `Container(width: 1)` vertical dividers have intrinsic height 0
- `KeyedSubtree` is transparent — intrinsic dimensions depend entirely on the
  wrapped `MorphingActionButton`
- `MorphingActionButton._buildFlatButton()` uses `Container(alignment: Alignment.center, ...)`,
  and `Align`-like widgets with unconstrained dimensions can interact poorly
  with intrinsic height queries
- `Ink` widget (used for splash effects) wraps `Material` which is a proxy box —
  its intrinsic height delegates to its child, but the chain of delegation can
  break under unusual constraints

### Where these widgets are used

- `CommentBar`: gear_content_view.dart, experience_content_view.dart, request_content_view.dart
- `ContentActionBar`: community_content_view.dart
- `InboxActionCard`: inbox screen (portfolio)

All three are placed inside `Column(mainAxisSize: MainAxisSize.min)` in their
respective content views, which provides tight vertical constraints. The crash
likely occurs when these constraints become unusual (e.g., during keyboard
appearance, screen rotation, or system UI changes on Android).

## Potential Solutions

### Option A: Replace `IntrinsicHeight` with explicit sizing

- **Approach:** Remove `IntrinsicHeight` entirely. Give the `Row` a fixed height
  (e.g., via `SizedBox(height: ...)` or `ConstrainedBox`). The action bars
  already have predictable content heights (text + padding = ~36-40px). The
  vertical dividers get their height from the parent `SizedBox` instead of from
  `IntrinsicHeight`.
- **Pros:** Eliminates the two-pass layout entirely. Simpler render tree.
  Deterministic sizing regardless of child content. Prevents the crash class
  entirely.
- **Cons:** Less flexible if content height changes in the future (e.g., multi-line
  text). Requires matching height constants across widgets.
- **Effort:** Small — straightforward replacement in 3 files.

### Option B: Keep `IntrinsicHeight` but fix child constraints

- **Approach:** Ensure every child of the `Row` inside `IntrinsicHeight` properly
  reports intrinsic dimensions. Specifically:
  - Give vertical dividers a `ConstrainedBox` with `minHeight: 1` so they have
    non-zero intrinsic height
  - Wrap `_buildActionButton()` in a `SizedBox(width: 90)` with explicit
    constraints
  - Ensure `MorphingActionButton._buildFlatButton()` doesn't use `alignment`
    property (which creates `Align`)
- **Pros:** Preserves dynamic height behavior. Minimal visual change.
- **Cons:** Still uses `IntrinsicHeight` which is inherently fragile and
  performs a two-pass layout. May not catch all edge cases.
- **Effort:** Small-medium — requires understanding intrinsic dimension
  contracts.

### Option C: Restructure action bars to use `LayoutBuilder` + explicit constraints

- **Approach:** Replace `IntrinsicHeight` with a `LayoutBuilder` that measures
  available space and passes explicit constraints to children. The dividers
  become `SizedBox(width: 1, height: constraints.maxHeight)`.
- **Pros:** Explicit constraint propagation with no fragile intrinsic queries.
  Works correctly under all constraint scenarios.
- **Cons:** More code. `LayoutBuilder` adds a frame delay for sizing. Overkill
  for this use case since the content height is predictable.
- **Effort:** Medium.

## Recommendation

**Option A: Replace `IntrinsicHeight` with explicit sizing.** The action bar
content has a predictable, fixed height in all three widgets:
- `ContentActionBar`: text + padding = 36px
- `CommentBar` empty: text + padding = 36px
- `CommentBar` with preview: top row ~48px + divider 1px + bottom row ~40px
- `InboxActionCard` bottom bar: text + padding = 40px

Using explicit `SizedBox(height: X)` wrapping eliminates the crash class entirely
and simplifies the render tree. The `CrossAxisAlignment.stretch` on the Row
will still stretch all children (including dividers) to fill the height without
needing `IntrinsicHeight`.

For the `InboxActionCard`, the height is already set by the parent
`SizedBox(height: totalHeight)` and `Positioned` constraints, so the
`IntrinsicHeight` wrapper on the bottom bar row is unnecessary.

## Open Questions

- **Which screen was the user on when the crash occurred?** The Crashlytics
  trace has no app-specific frames, so we can't pinpoint the exact widget.
  **Recommendation:** Fix all three widgets since they all share the same
  fragile pattern. The fix is small and low-risk for each.

- **Is the crash Android-specific?** The report is from Android, but the
  pattern exists on both platforms. **Recommendation:** Treat it as
  cross-platform — `IntrinsicHeight` layout issues can occur on any platform
  under the right conditions.

- **Should we add regression tests?** **Recommendation:** Yes, add widget tests
  for `ContentActionBar`, `CommentBar`, and `InboxActionCard` that verify
  layout succeeds under tight constraints (e.g., `SizedBox(width: 200)`) and
  zero-size scenarios. These are quick to write and prevent recurrence.

## Implementation Plan

### Phase 1: Remove `IntrinsicHeight` from action bars
**Goal:** Eliminate the root cause of the crash in all three widgets.
**Tasks:**
- [ ] `ContentActionBar`: Add a `static const double height = 36.0` constant. Remove `IntrinsicHeight`, wrap inner `Row` in `SizedBox(height: height)`. The `ClipRRect` parent handles clipping, and `CrossAxisAlignment.stretch` stretches dividers and children to fill the 36px height. _(Verified: padding vertical 10×2 = 20 + icon/text ~16 = 36.)_
- [ ] `CommentBar._buildEmpty()`: Remove `IntrinsicHeight`, use `SizedBox(height: ...)` matching the existing `emptyHeight` constant (36.0, minus the 3px outer border). `CommentBar` already exposes `emptyHeight` and `previewHeight` — these are already consumed by gear/experience/request content views for FAB positioning.
- [ ] `CommentBar._buildWithPreview()`: Remove `IntrinsicHeight` from the bottom row, use explicit height for the bottom row section matching the existing `previewHeight` budget.
- [ ] `InboxActionCard._buildActionRow()`: Remove `IntrinsicHeight`. The parent `Positioned(bottom: 0, left: 0, right: 0)` inside a `SizedBox(height: totalHeight)` already constrains the height. The existing `_actionRowHeight = 40.0` constant already governs the layout budget.
- [ ] Verify the vertical dividers (`Container(width: 1)`) correctly stretch to fill height via `CrossAxisAlignment.stretch` under bounded-height parents.

**Validates:** App builds and renders correctly on both platforms. The three action bar variants look identical to before.

### Phase 2: Add widget tests
**Goal:** Prevent regression and verify layout under edge-case constraints.
**Tasks:**
- [ ] Add widget test for `ContentActionBar` — renders without errors in a tight `SizedBox(width: 300, height: 100)` container
- [ ] Add widget test for `CommentBar` — both empty and preview states render without layout errors
- [ ] Add widget test for `InboxActionCard` — renders without layout errors
- [ ] Test with very narrow widths (e.g., 200px) to stress the layout
- [ ] Run with `flutter test --reporter expanded` per project convention

**Validates:** Layout is deterministic under various constraint scenarios.

---

## Audit Notes

**Audited against:** `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`

**Findings:**
- **No architectural violations.** This is a pure widget-layer layout fix. No state management, data access, repository, or server changes are involved.
- **Existing height constants should be reused.** `CommentBar.emptyHeight` (36.0), `CommentBar.previewHeight` (105.0), and `InboxActionCard._actionRowHeight` (40.0) already exist and are consumed by parent content views for FAB positioning. The plan now references these explicitly. `ContentActionBar` needs a new public height constant (36.0) for consistency.
- **No caching impact.** No repository or cache layer changes.
- **No i18n impact.** No user-visible strings added or changed.
- **No server/client boundary concern.** Client-only fix.
- **Testing approach is consistent.** Widget tests for layout correctness are appropriate per the architecture's testing guidelines. Phase 2 now includes the `--reporter expanded` convention.
- **Why `IntrinsicHeight` removal is safe:** These widgets live inside `Column(mainAxisSize: MainAxisSize.min)` in content views, which passes **unbounded** height constraints to children. Without `IntrinsicHeight`, the `Row` with `CrossAxisAlignment.stretch` would try to stretch children to infinity — that's why explicit `SizedBox(height:)` wrapping is mandatory (not just `IntrinsicHeight` removal alone). The plan correctly pairs removal with explicit sizing.

## Related Issues

- None referenced in the issue or comments.
