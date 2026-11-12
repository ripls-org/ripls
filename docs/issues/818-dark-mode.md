# Issue #818: App is not supporting dark mode well

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/818
**Filed:** 2026-03-09 | **Last activity:** 2026-03-09
**Labels:** none | **Assignees:** tescobar
**Context docs:** none

## Problem Statement

After changing the default background color for modals and other areas, the dark
mode no longer looks dark. Many widgets and screens use hardcoded light colors
(`Colors.white`, `Colors.grey[100-300]`, `Color(0xFFFAF9F7)`) instead of the
existing theme-aware `AppColors` helpers, making them appear as bright white
cards and surfaces when dark mode is active.

Additionally, the app does not inherit the OS-level light/dark mode setting. The
`ThemeModeNotifier` defaults to `ThemeMode.light` and is not persisted, so:
1. The app always starts in light mode regardless of OS preference.
2. Toggling dark mode in settings resets on restart.

## Context from Comments

One comment from the issue author (tescobar) adds a requirement:
- The app should **automatically inherit the OS-level light/dark mode setting**.
- The toggle should still allow the user to override it (comment appears cut off
  at "It should still be over" — likely "overridable").

## Potential Solutions

### Option A: Incremental fix — replace hardcoded colors + persist theme + respect system

- **Approach:** Audit and replace all hardcoded light colors with existing
  `AppColors.xxx(context)` helpers. Add `ThemeMode.system` as the default. Persist
  theme preference to `SharedPreferences`.
- **Pros:** Uses the existing color system (no new abstractions). Fixes the root
  cause. Each file change is small and testable.
- **Cons:** Many files to touch (~60+ widgets/screens). Risk of missing spots.
- **Effort:** Medium — the changes are mechanical but numerous.

### Option B: Add BottomSheetThemeData + bulk-fix remaining widgets

- **Approach:** In addition to Option A's color fixes, define `BottomSheetThemeData`
  in both light and dark `ThemeData` so bottom sheets automatically get the right
  background without each call site setting `backgroundColor: Colors.transparent`.
  Also add `CardTheme`, `DialogTheme`, etc. defaults for dark mode.
- **Pros:** Reduces future regressions — new bottom sheets automatically get dark
  backgrounds. Centralizes styling.
- **Cons:** Slightly more upfront work. Could change existing sheet visuals if
  not careful.
- **Effort:** Medium.

### Option C: Design token system overhaul

- **Approach:** Replace the static `AppColors` class with a full design token
  system using `ThemeExtension<T>`, allowing typed access like
  `Theme.of(context).extension<RiplsColors>()!.cardBackground`.
- **Pros:** More idiomatic Material 3. Enables per-component token overrides.
- **Cons:** Large refactor with no immediate visual benefit beyond what Option A
  achieves. Over-engineering for the current problem.
- **Effort:** Large.

## Recommendation

**Option A** with selected elements from Option B. The existing `AppColors`
context-aware helpers already provide the right abstraction — the problem is
simply that many widgets don't use them. Adding `BottomSheetThemeData` to
`ThemeData` is a small addition that prevents future regressions for sheets.
Option C is not justified since `AppColors` is already working well where it's
used.

## Open Questions

- **What should the toggle UI look like?** Currently a simple on/off toggle.
  With system-default support, it should become a 3-way choice (System / Light /
  Dark). — Recommendation: Use a segmented control or dropdown in settings with
  three options. Default to "System".
- **Should the cut-off comment "It should still be over..." be interpreted as
  "overridable"?** — Recommendation: Yes, treat as "the user can still override
  the system setting." This is standard behavior.

## Implementation Plan

### Phase 1: Theme persistence and system default
**Goal:** App respects OS dark/light mode and persists user override.
**Tasks:**
- [ ] Change `ThemeModeNotifier.build()` to load saved preference from
      `SharedPreferences`; default to `ThemeMode.system` when no preference saved.
      **Note:** `SharedPreferences` is accessed directly from the Notifier because
      theme mode is local device state, not server data — this does not go through
      the repository/service pattern.
- [ ] Save to `SharedPreferences` in `setThemeMode()`
- [ ] Remove or deprecate `toggleTheme()` — with a 3-way selector
      (System / Light / Dark), a binary toggle is ambiguous. Replace all call sites
      with `setThemeMode()`.
- [ ] Replace the binary toggle in `profile_settings_screen.dart` with a 3-way
      selector (System / Light / Dark) that calls `setThemeMode()`
- [ ] Test: verify app follows OS setting on fresh install, persists override
      across restarts
**Validates:** Theme selection works end-to-end with OS integration.

### Phase 2: Fix hardcoded card/container backgrounds
**Goal:** All card and container backgrounds use theme-aware colors.
**Tasks:**
- [ ] Replace `Colors.white` card/container backgrounds (~40 sites) with
      `AppColors.cardBackground(context)` or `AppColors.surface(context)`
- [ ] Replace `Colors.grey[50/100/200/300]` backgrounds (~15 sites) with
      `AppColors.surface(context)` or `AppColors.border(context)` as appropriate
- [ ] Replace `Color(0xFFFAF9F7)` in `location_picker_modal.dart` with
      `AppColors.surface(context)`
- [ ] Add `BottomSheetThemeData` to both light and dark `ThemeData` in
      `app_theme.dart` for default sheet backgrounds
- [ ] Spot-check each change in both light and dark mode
- [ ] Add or update widget tests for any widgets that already have tests to
      verify they build without errors in both light and dark `ThemeData`
**Validates:** Modals, cards, and containers render with correct dark backgrounds.

### Phase 3: Fix borders, overlays, and semi-transparent colors
**Goal:** Remaining hardcoded colors are theme-aware.
**Tasks:**
- [ ] Replace `Colors.grey[300]` border colors with `AppColors.border(context)`
- [ ] Replace semi-transparent `Colors.white.withValues(alpha: ...)` overlays
      with theme-aware equivalents (e.g., `AppColors.surface(context).withValues(alpha: ...)`)
- [ ] Replace `Colors.white` avatar borders with `AppColors.cardBackground(context)`
- [ ] Visual QA pass through all major screens in dark mode
- [ ] Add a note in `AppColors` doc comment listing available helpers to guide
      future development
- [ ] Add or update widget tests for any widgets that already have tests to
      verify they build without errors in both light and dark `ThemeData`
**Validates:** Full dark mode visual consistency across the app.

## Audit Notes

**Reviewed against:** `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/server/architecture.md`

- **Compliant:** All changes are in the presentation layer (`core/theme/`,
  `presentation/screens/`, `presentation/widgets/`) and the theme-mode Notifier
  in `services/providers.dart`. No repository, service, or server changes needed.
- **SharedPreferences exception documented:** Phase 1 accesses
  `SharedPreferences` directly from `ThemeModeNotifier` for local device state.
  This is intentional — theme mode is not server data and does not warrant a
  repository. The plan now documents this explicitly to avoid setting a precedent.
- **`toggleTheme()` removal added:** The original plan kept the binary toggle
  method, which is incompatible with the 3-way System/Light/Dark selector. Fixed.
- **Test tasks added to Phases 2 and 3:** The architecture docs require tests for
  new functionality. Phases 2 and 3 now include test verification tasks for widgets
  that already have test coverage.

## Files to Change (by phase)

### Phase 1
- `app/lib/services/providers.dart` (ThemeModeNotifier)
- `app/lib/presentation/screens/profile/profile_settings_screen.dart`

### Phase 2 (major files — not exhaustive)
- `app/lib/core/theme/app_theme.dart` (add BottomSheetThemeData)
- `app/lib/presentation/widgets/impact/` (money_saved_detail_view, co2_detail_view, time_recovered_detail_view, sharing_impact_card, impact_metric_card, loan_history_card, loan_history_timeline, stat_chip, impact_hero_section)
- `app/lib/presentation/widgets/impact/community/` (impact_chart_card, insight_card, scenario_card, milestone_card, metric_description_card, trend_card, stat_pill_row, welcome_card, community_compare_v2)
- `app/lib/presentation/widgets/transfer/` (conversation_preview_card, summary_edit_card, helper_offer_card, elapsed_time_card, impact_metrics_card)
- `app/lib/presentation/widgets/experience/` (location_row, when_row, propose_time_form, poll_options_builder, chat_button, attendee_avatar_stack, rsvp_toggle_buttons)
- `app/lib/presentation/widgets/location/location_picker_modal.dart`
- `app/lib/presentation/widgets/leaderboard/` (leaderboard_header, community_detail_modal)
- `app/lib/presentation/widgets/daily/` (weekly_metrics_carousel, weekly_metrics_grid)
- `app/lib/presentation/widgets/plus_button_modal.dart`
- `app/lib/presentation/widgets/modal/modal_builders.dart`
- `app/lib/presentation/screens/` (leaderboard_screen, gear_preview_modal, request_preview_modal, experience_preview_modal, event_organizer_modal, event_joiner_modal, conversation_screen, discover_screen, item_metrics_screen, community_metrics_screen, portfolio_metrics_screen, portfolio_metric_detail_screen, invite_sheet, story_content_view)

### Phase 3
- Avatar border files (community_avatar_list, avatar_stack, attendee_avatar_stack)
- Semi-transparent overlay files (media_carousel, content_view_builders, conversation_top_bar, camera_instruction_overlay, discover_event_carousel)
- `app/lib/presentation/widgets/observability/consent_dialog.dart`
- `app/lib/presentation/widgets/profile_gear_card.dart`
- `app/lib/presentation/widgets/profile_filter_bar.dart`

## Related Issues
- None identified.
