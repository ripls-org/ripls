# Issue #831: Experiences should have an easy way to add to external calendar (Apple or Google)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/831
**Filed:** 2026-03-11 | **Last activity:** 2026-03-29
**Labels:** P1 | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The experience time detail modal has an "Export" button that opens Apple Calendar on iOS but does nothing on Android. The underlying code exists: `CalendarHelper.addToCalendar()` builds a calendar `Event` and delegates to the `add_2_calendar` package, which on Android fires an `Intent.ACTION_INSERT` with `CalendarContract.Events.CONTENT_URI`.

The bug is that on Android 11+ (API 30), `intent.resolveActivity(packageManager)` returns `null` unless the app declares the target intent in a `<queries>` block in `AndroidManifest.xml`. The plugin's own manifest is empty — it relies on the host app. Our manifest has no calendar-related `<queries>` entry, so `resolveActivity()` silently returns `null` and the plugin returns `false` without opening any calendar app. The Dart code catches the `false` return but shows no feedback to the user.

## Context from Comments

One comment from maintainer (2026-03-29):
- Confirms the Export button is visible in the time detail modal but does nothing on Android.
- Confirms it works on iPhone (creates a new Apple Calendar event).
- Includes a screenshot showing the Export button in the bottom bar next to the Edit button.

## Potential Solutions

### Option A: Add `<queries>` intent declaration to AndroidManifest

- **Approach:** Add a `<queries>` block declaring `Intent.ACTION_INSERT` with `content://com.android.calendar/events` data URI. This lets `resolveActivity()` find the calendar app on Android 11+.
- **Pros:** Minimal change, directly fixes the root cause. No new dependencies.
- **Cons:** None significant. This is the documented Android fix for package visibility.
- **Effort:** Small.

### Option B: Replace `add_2_calendar` with a different package

- **Approach:** Switch to a package like `device_calendar` that writes directly to the calendar provider instead of using an intent.
- **Pros:** More control over the event creation. Could support features like editing/deleting events later.
- **Cons:** Larger change. `device_calendar` requires runtime calendar permissions. Overkill for the current use case (launch calendar app with pre-filled event).
- **Effort:** Medium.

### Option C: Add `<queries>` + improve error feedback

- **Approach:** Same as Option A, plus add user-visible feedback when export fails (e.g., no calendar app installed) and when it succeeds.
- **Pros:** Fixes the root cause and improves UX for edge cases.
- **Cons:** Slightly more work than Option A alone.
- **Effort:** Small.

## Recommendation

**Option C** — the manifest fix is required and trivial, but the current code silently swallows failures in both export paths (owner bottom bar and non-owner inline button). Adding a success confirmation and a failure message makes the feature feel intentional. The non-owner path in `TimeReadView` doesn't even check the return value from `CalendarHelper.addToCalendar()`.

## Open Questions

- **Should the Export button show a success confirmation (e.g., snackbar)?** — Recommendation: Yes, a brief snackbar like "Opening calendar..." gives confidence the tap registered. The `add_2_calendar` intent approach opens the calendar app for final confirmation, so the user will see the calendar UI, but a snackbar bridges the gap if there's a brief delay.
- **Should the non-owner export path (`TimeReadView`) also fetch fresh experience data like the owner path does?** — Recommendation: Yes, for consistency. The owner path in `UnifiedTimeModal._exportToCalendar()` fetches from the repository; the non-owner path uses the local `experience` reference which could be stale.

## Implementation Plan

### Phase 1: Fix Android calendar intent resolution
**Goal:** Make the Export button open the calendar app on Android.
**Tasks:**
- [ ] Add `<queries>` entry to `app/android/app/src/main/AndroidManifest.xml` for `android.intent.action.INSERT` with `content://com.android.calendar/events` data URI
- [ ] Test on Android emulator (API 30+) that tapping Export opens Google Calendar (or calendar chooser)
**Validates:** The core bug is fixed — Export works on both platforms.

### Phase 2: Improve export feedback and consistency
**Goal:** Users get clear feedback on export success/failure. Both export paths behave consistently.
**Tasks:**
- [ ] Add l10n ARB keys for export feedback strings to both `app_en.arb` and `app_es.arb` (e.g., `timeExportSuccess`, `timeExportFailed`). All user-visible text must use `context.l10n.*` — no hardcoded strings.
- [ ] In `TimeReadView._buildCalendarExportRow()`, check the return value from `CalendarHelper.addToCalendar()` and show a localized snackbar on failure
- [ ] In `UnifiedTimeModal._exportToCalendar()`, check the return value from `CalendarHelper.addToCalendar()` and show a localized failure snackbar (currently only catches exceptions, not `false` returns)
- [ ] Add a brief localized confirmation snackbar on success in both paths
- [ ] Optionally: have `TimeReadView` fetch fresh experience data before exporting, matching the owner path pattern. Note: the existing owner path accesses `experienceRepositoryProvider` directly from the widget — this is a documented "Widget async exception" (platform UI integration, like image/file pickers per `docs/client/architecture.md`). Add the standard `// Widget async exception: platform calendar UI` comment if replicating this pattern.

**Validates:** Users see clear feedback regardless of success or failure. No silent failures.

## Related Issues

None identified.
