# Issue #936: Feedback form in dark mode has zero contrast for text entry fields

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/936
**Filed:** 2026-03-28 | **Last activity:** 2026-03-28
**Labels:** bug, user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The feedback bottom sheet (`FeedbackSheet`) uses `AppColors.surface(context)` as its container background. In dark mode, this resolves to `darkSurface` (`0xFF3A2F2A`). The global `inputDecorationTheme` for dark mode also uses `darkSurface` as the `fillColor` for text inputs. The result: both text fields (title and description) are visually invisible against the modal background — zero contrast, indistinguishable until focused (when the border turns coral).

This is the same family of bug as #865 (dark mode form colors) and #818 (dark mode), but specific to the feedback sheet.

### Root Cause

In `app/lib/core/theme/app_theme.dart:350-352`:
```dart
inputDecorationTheme: InputDecorationTheme(
  filled: true,
  fillColor: AppColors.darkSurface,  // 0xFF3A2F2A
```

In `app/lib/presentation/widgets/feedback/feedback_sheet.dart:53`:
```dart
color: AppColors.surface(context),  // also 0xFF3A2F2A in dark mode
```

Same color for both container and fill → zero contrast.

Other modals avoid this by using `darkBackground` (`0xFF1A1512`) or `darkCardBackground` (`0xFF2D2420`) as their container color, which contrasts with the `darkSurface` fill. For example, `community_creation_modal.dart` uses `experienceModalBackground(context)` which maps to `darkBackground`.

## Context from Comments

One comment from the issue author with a screenshot confirming the bug — text fields are completely invisible against the modal background in dark mode on Android. No other discussion.

## Potential Solutions

### Option A: Change the feedback sheet background to `cardBackground`

- **Approach:** Change the sheet's container `color` from `AppColors.surface(context)` to `AppColors.cardBackground(context)` (dark: `0xFF2D2420`). This matches how other modals work — they use a darker background that contrasts with the `darkSurface` input fill.
- **Pros:** One-line fix. Follows the pattern used by other modals. No impact on light mode (lightCardBackground `0xFFFFFFFF` contrasts fine with lightSurface `0xFFF8F3EE`). Also fixes the type selector pills which use `cardBackground` — they'll now need a different treatment or will naturally contrast.
- **Cons:** Subtle visual shift in the sheet background for dark mode. Type selector pills currently use `cardBackground` too, so they'd blend into the new sheet background.
- **Effort:** Small.

### Option B: Override `fillColor` on the feedback sheet's text fields

- **Approach:** Explicitly set `fillColor: AppColors.cardBackground(context)` on both `InputDecoration` instances in the feedback sheet, overriding the global theme.
- **Pros:** Targeted fix. No change to the modal background.
- **Cons:** Creates a local override that diverges from the theme — future text fields added to this sheet would need to remember to do the same. Doesn't address the systemic issue (surface-on-surface).
- **Effort:** Small.

### Option C: Change the global dark theme `fillColor` to `darkCardBackground`

- **Approach:** In `app_theme.dart`, change the dark `inputDecorationTheme.fillColor` from `darkSurface` to `darkCardBackground` (`0xFF2D2420`).
- **Pros:** Fixes every surface-on-surface text field in the app at once.
- **Cons:** Broader blast radius — could change the look of text fields on screens that already have correct contrast. Needs thorough visual audit across the app.
- **Effort:** Medium (small code change, larger testing surface).

## Recommendation

**Option A** — change the feedback sheet background to `AppColors.cardBackground(context)`. This is the minimal, targeted fix that follows the existing pattern used by other modals. The feedback sheet is the outlier here — most modals use a background that's darker than `surface`, giving text fields natural contrast.

The type selector pills (line 189) also need adjustment since they currently use `cardBackground` for the unselected state — they should switch to `surface` so they contrast against the new sheet background.

## Open Questions

- **Should the enabled border color also change?** The dark `enabledBorder` uses `darkBorder` (`0xFF3A2F2A`), which is the same as `darkSurface` — also invisible on a surface-colored background. With Option A, the sheet background changes so the border will contrast naturally. — **Recommendation:** No extra change needed with Option A; the `darkBorder` contrasts against `darkCardBackground`.
- **Should we also fix the global `editableFieldBackground` helper?** It currently returns `darkSurface`, same as the theme fillColor. — **Recommendation:** Out of scope for this bug; it's correct for fields on `darkBackground` screens.

## Implementation Plan

### Phase 1: Fix the feedback sheet contrast

**Goal:** Text fields are visually distinguishable from the modal background in dark mode.

**Tasks:**
- [ ] In `feedback_sheet.dart` line 53, change `AppColors.surface(context)` to `AppColors.cardBackground(context)`
- [ ] In `_buildTypePill` line 189, change the unselected pill background from `AppColors.cardBackground(context)` to `AppColors.surface(context)` so pills contrast against the new sheet background
- [ ] Visually verify in both light and dark mode on Android emulator

**Validates:** Text fields, type pills, and all other sheet content have proper contrast in both themes.

## Audit

**Status: PASS** — no compliance issues found.

- **Theming:** The recommended fix (`cardBackground` for modal, `surface` for inputs/chips) aligns with the color quick-reference table in `docs/client/architecture.md` (lines 311-318): "Card / modal background → `AppColors.cardBackground(context)`", "Surface (input, chip) → `AppColors.surface(context)`". The current code violates this guidance by using `surface` for the modal background; the fix corrects it.
- **Caching:** No caching involvement — purely a visual fix.
- **Server/client boundary:** No server changes needed — client-only theming fix.
- **Architecture patterns:** No new widgets, state, or data flow changes. The fix stays within the existing `AppColors` theme-aware helper pattern.

## Related Issues

- #865 — Dark mode form colors (same color family of bugs)
- #818 — Dark mode (general dark mode tracking)
