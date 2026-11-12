# Issue #865: Experience data entry colors bad on dark mode

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/865
**Filed:** 2026-03-20 | **Last activity:** 2026-03-28
**Labels:** bug, user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Text entry fields across all creation/editing modals have broken contrast in dark
mode. Text fields show light text on a light (white) background, making input
invisible. Date/time selection fields show dark text on a dark background. The
root cause is hardcoded light-mode colors in shared form widgets (`TextInputArea`,
`ProposeTimeForm`) and scattered hardcoded greys in modal screens, rather than
using the existing theme-aware `AppColors` helpers.

The reporter initially flagged experience screens, but a follow-up comment from
maintainer confirms this affects "several editing/creation screens" — gear,
request, community, and experience creation modals all use the same broken
`TextInputArea` widget.

## Context from Comments

One comment from maintainer (member):
- Confirms the issue spans multiple editing/creation screens, not just experiences
- Describes two failure modes: (1) light text on light background (invisible), (2) dark text on dark background (barely visible)
- Requests "consistent and matched/different sets of colors for text and buttons in light and dark mode that improve the contrast"
- Provides additional screenshots showing the issue across different screens

## Potential Solutions

### Option A: Replace hardcoded colors with existing AppColors helpers

- **Approach:** The `AppColors` class already provides theme-aware getters for
  form fields: `editableFieldBackground()`, `editableFieldBorder()`,
  `editableFieldText()`, `editableFieldHint()`. Replace all hardcoded
  `Colors.white`, `Color(0xFF...)`, and `Colors.grey[X]` in form widgets and
  modal screens with these existing helpers.
- **Pros:** No new abstractions needed. AppColors already handles light/dark
  switching. Each change is small and mechanical.
- **Cons:** Need to touch many files. Risk of missing spots.
- **Effort:** Medium.

### Option B: Fix InputDecorationTheme in app_theme.dart + widget fixes

- **Approach:** Fix the dark theme's `InputDecorationTheme` to use AppColors
  values (currently uses raw `Colors.grey[X]`), then remove hardcoded overrides
  from widgets so they inherit from the theme. Widgets that override the theme
  (like `TextInputArea`) still need individual fixes.
- **Pros:** Reduces future regressions — new text fields automatically get
  correct dark mode colors. Centralizes form field styling.
- **Cons:** Doesn't fix widgets that override InputDecoration locally (which is
  most of the affected widgets).
- **Effort:** Medium.

### Option C: Combined — fix theme defaults + replace widget overrides

- **Approach:** Fix `InputDecorationTheme` in app_theme.dart for both light and
  dark themes (Option B), then replace hardcoded colors in individual widgets
  with AppColors helpers (Option A). Also fix date/time picker theming.
- **Pros:** Best of both worlds — fixes current bugs and prevents future ones.
- **Cons:** Slightly more work upfront.
- **Effort:** Medium.

## Recommendation

**Option C** — fix the theme defaults AND replace widget-level overrides. The
`InputDecorationTheme` in `app_theme.dart` already uses raw `Colors.grey[X]`
values that don't match the warm dark palette (e.g., `hintStyle` uses
`Colors.grey[600]` which is too dark on dark backgrounds). Fixing the theme
prevents regressions for any widget that doesn't override InputDecoration. Then
fixing the widgets that DO override (TextInputArea, ProposeTimeForm, etc.)
handles the reported bugs.

## Open Questions

- **Should date/time pickers get custom theme data?** Currently `showDatePicker()`
  and `showTimePicker()` use Flutter defaults, which may not match the app's warm
  color palette. — Recommendation: Yes, add `datePickerTheme` and
  `timePickerTheme` to the dark ThemeData using AppColors values. This is a
  small addition that ensures pickers match the rest of the app.
- **How does this relate to #818?** Issue #818 is a broader dark mode overhaul
  covering backgrounds, borders, overlays, and theme persistence. This issue
  (#865) is a focused subset: form fields and data entry. — Recommendation:
  Implement #865 as a standalone fix targeting form inputs specifically. The
  fixes here are a subset of #818's Phase 2 but are independently shippable and
  address the most user-visible pain point.

## Implementation Plan

### Phase 1: Fix shared form widgets
**Goal:** All text input fields render correctly in dark mode.
**Tasks:**
- [ ] `text_input_area.dart`: Replace `fillColor: Colors.white` (line 61) with `AppColors.editableFieldBackground(context)`
- [ ] `propose_time_form.dart` `_FieldRow`: Replace `fillColor: Colors.white` (line 602) with `AppColors.editableFieldBackground(context)`
- [ ] `propose_time_form.dart` `_FieldRow`: Replace hardcoded border `Color(0xFFD4D4D4)` (line 604) with `AppColors.editableFieldBorder(context)`
- [ ] `propose_time_form.dart` `_FieldRow`: Replace hardcoded text colors (lines 614-615) with `AppColors.editableFieldText(context)` / `AppColors.editableFieldHint(context)`
- [ ] `propose_time_form.dart` section headers: Replace `Color(0xFF9E9E9E)` (lines 94, 136, 186) with `AppColors.textTertiary(context)`
- [ ] `propose_time_form.dart` `_ChipWithSubtitle` and `_SimpleChip`: Replace hardcoded `Color(0xFF2C2C2C)` text (lines 522, 572) with `AppColors.textPrimary(context)`
- [ ] Verify all four creation modals (experience, request, gear, community) render correctly in both light and dark mode
**Validates:** Text entry is readable in dark mode across all creation flows.

### Phase 2: Fix InputDecorationTheme and picker themes
**Goal:** Theme defaults produce correct dark mode form fields, preventing regressions.
**Tasks:**
- [ ] `app_theme.dart` dark theme `inputDecorationTheme`: Replace `Colors.grey[850]` fillColor with `AppColors.darkSurface`
- [ ] `app_theme.dart` dark theme `inputDecorationTheme`: Replace `Colors.grey[400]` labelStyle with `AppColors.darkTextSecondary`
- [ ] `app_theme.dart` dark theme `inputDecorationTheme`: Replace `Colors.grey[600]` hintStyle with a lighter value (e.g., `AppColors.darkTextTertiary` or `Color(0xFF9E9590)`)
- [ ] `app_theme.dart` dark theme `inputDecorationTheme`: Replace `Colors.grey[700]` border colors with `AppColors.darkBorder`
- [ ] `app_theme.dart` dark theme: Add `datePickerTheme: DatePickerThemeData(...)` using AppColors dark palette
- [ ] `app_theme.dart` dark theme: Add `timePickerTheme: TimePickerThemeData(...)` using AppColors dark palette
- [ ] Verify date and time pickers match app styling in dark mode
**Validates:** New text fields and pickers automatically inherit correct dark mode styling.

### Phase 3: Fix hardcoded colors in modal screens
**Goal:** Remaining hardcoded greys and accent colors in editing modals are theme-aware.
**Tasks:**
- [ ] `event_joiner_modal.dart`: Replace `Colors.grey[400/600/700/900]` (lines 153, 161, 169, 179, 275, 424) with `AppColors.textPrimary/Secondary/Tertiary(context)` as appropriate
- [ ] `event_organizer_modal.dart`: Replace `Colors.grey[400/600/700/900]` (lines 178, 186, 194, 204, 296, 427, 458, 503, 525, 626) with AppColors equivalents
- [ ] `experience_preview_modal.dart`: Replace `Colors.blue` (line 749) with `AppColors.primary(context)`
- [ ] `community_creation_modal.dart`: Replace hardcoded error banner colors (lines 212, 215) with `AppColors.error` and `AppColors.textPrimary(context)`
- [ ] `loan_owner_modal.dart`: Replace hardcoded cream/brown colors (lines 416-448) with theme-aware equivalents
- [ ] `loan_manage_modal.dart`: Replace hardcoded colors (lines 260-289) with theme-aware equivalents
- [ ] Visual QA pass through all affected screens in both light and dark mode
**Validates:** Full form entry and modal consistency in dark mode.

## Audit Notes

**Reviewed against:** `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/server/architecture.md`

- **Compliant:** All changes are in the presentation layer (`core/theme/`,
  `presentation/widgets/`, `presentation/screens/`). No repository, service,
  ViewModel, or server changes needed.
- **Architecture alignment:** The plan directly implements the "Colors and
  Theming" section of `docs/client/architecture.md` (lines 289-321), which
  mandates `AppColors.xxx(context)` for all UI colors and prohibits hardcoded
  `Colors.white`, `Colors.grey[X]`, and `Color(0xFF...)` values.
- **No i18n impact:** No new user-facing strings are introduced — this is purely
  a color/styling fix.
- **No caching impact:** No data layer or caching changes.
- **Relationship to #818:** This plan is a self-contained subset of #818's
  Phase 2. The fixes here are independently shippable and do not conflict with
  the broader dark mode plan. Completing this issue reduces #818's remaining
  scope.

## Related Issues
- #818 — Broader dark mode overhaul (backgrounds, borders, overlays, theme persistence). This issue is a focused subset of #818's Phase 2 targeting form fields specifically.
