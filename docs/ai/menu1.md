# Experience Manage Menu — Checklist Redesign

> **Audit (2026-03-28):** Plan reviewed against `docs/client/architecture.md`,
> `docs/client/caching.md`, `docs/client/i18n.md`, and `docs/server/architecture.md`.
> Changes flagged with **[AUDIT]** inline.

## Problem Statement

The current "Manage" dropdown menu on the experience content view is a flat list of actions with no visual indication of workflow progress. An event owner opening the menu sees the same icons and labels whether they've completed zero steps or four. There is no sense of forward momentum or "what's left to do."

### Current menu (top to bottom):
1. RSVP (`event_available_outlined`)
2. Find Best Time / See Time Results (`tune_outlined` / `poll_outlined`)
3. Change Details (`edit_outlined`)
4. Mark Completed (`check_circle_outline`)
5. Close Event (`delete_outline`, destructive)

### Problems:
- **No progress feedback.** The menu looks the same at creation as it does right before completion. Owners can't glance at it and know "3 of 5 steps done."
- **Order doesn't match workflow.** "Change Details" (editing) appears between time polling and completion — it's not a workflow step, it's a utility. The order should reflect the natural event lifecycle.
- **No visual distinction between done and not-done.** The existing `isCompleted` flag on `ActionDropdownItem` only shows a trailing green checkmark. There's no support for intermediate states (poll active, maybe RSVP, etc.) or negative states (no location set).
- **Post-completion clutter.** After marking an event complete, the menu still shows all the same items. The owner's only action at that point is reviewing impact.

---

## Proposed Menu (top to bottom)

| # | Label | Checklist icon logic |
|---|-------|---------------------|
| 1 | **Set Details** | Green checkmark when title + description + at least 1 media are all present; default icon otherwise |
| 2 | **RSVP** | Default icon until the owner RSVPs. Then: green check (Yes), red X (No), yellow ? (Maybe) with matching background/border colors from RSVP buttons |
| 3 | **Confirm Location** | Green checkmark if a location is set. Red X if no location set |
| 4 | **Confirm Time** | Green checkmark when any time is set (creation or poll-confirmed). Red `Icons.how_to_vote` when a poll is active. Yellow ? when a poll is complete but no time confirmed |
| 5 | **Mark Completed** | Current icon until marked completed. Once completed: items 1-4 are removed, replaced by a single "View Impact" item that opens the metrics screen |
| 6 | **Close Event** | Unchanged (destructive, always present) |

### Post-completion menu:
| # | Label | Icon |
|---|-------|------|
| 1 | **View Impact** | Metrics icon (green) |

---

## Potential Solutions

### Option A: Extend `ActionDropdownItem` with status icon support

Add a `statusIcon` widget field to `ActionDropdownItem` that replaces the leading circular icon container when present. Each menu item computes its own status widget based on experience state.

**Pros:** Minimal structural change. The dropdown menu renderer just checks `statusIcon != null` and renders it instead of the default icon circle.
**Cons:** Pushes presentation logic into the content view's menu-building code. Each status widget is built inline.

### Option B: New `ChecklistDropdownItem` subclass

Create a `ChecklistDropdownItem` that extends `ActionDropdownItem` with a `ChecklistStatus` enum (`completed`, `active`, `warning`, `incomplete`, `default`) and computes the icon/colors automatically.

**Pros:** Clean separation — the menu renderer handles all visual states. Content view just sets the enum.
**Cons:** The RSVP item needs three sub-states (yes/no/maybe) with different colors, which doesn't fit neatly into a simple enum. Over-abstraction for one menu.

### Option C: Custom leading widget per item (recommended)

Add an optional `Widget? leadingWidget` field to `ActionDropdownItem`. When set, it replaces the default circular icon container. Build small helper methods in the content view that return the correct status circle for each menu item.

**Pros:** Flexible enough to handle RSVP's three-color states and the time poll's active/complete distinction without a rigid enum. No subclass needed. The rendering code stays simple — just `item.leadingWidget ?? _defaultIcon(item)`.
**Cons:** Slightly more widget code in the content view, but it's isolated in private helper methods.

---

## Recommendation

**Option C** — custom leading widget per item. It gives us the visual flexibility needed for RSVP's multi-color states and the time poll's active state without over-engineering the dropdown infrastructure. The status circle helpers are small, testable, and local to the experience content view.

> **[AUDIT] Architecture compliance note.** This approach is compliant with the
> MVVM layering rules. The status icon helpers are pure presentation logic that
> read from `ExperienceState` (already loaded by the ViewModel). They do not
> access repositories or services directly — all required data (RSVP intention,
> time proposals, time poll active, location name) is already present in
> `ExperienceState` and `GetExperienceResponse`. No new data fetching is needed.
> No server changes required — the existing `GetExperience` RPC returns all
> necessary fields.

---

## Open Questions (Resolved)

1. **"Set Details" always green?** **Resolved: No.** Check for non-empty title + description + at least 1 media item. Show default icon if any of those are missing; green checkmark only when all three are present.
2. **RSVP — whose RSVP?** **Resolved: Owner's personal RSVP.** The status icon reflects the owner's own RSVP intention, not the aggregate count.
3. **"Confirm Time" red icon — which icon?** **Resolved: `Icons.how_to_vote`.** The ballot icon for active polls.
4. **Time confirmed vs time set at creation.** **Resolved: Any set time = green.** Whether the time was set at creation or confirmed via poll, a set time shows green.
5. **Post-completion "View Impact" destination.** **Resolved: Open `ExperienceMetricsScreen` via slide-from-right navigation.** This screen already has the impact metrics. Use `NavigationHelpers.pushWithSlide()`.
6. **"Close Event" after completion.** **Resolved: Remove it.** The post-completion menu should only contain "View Impact" — no "Close Event" item.

---

## Phased Implementation Plan

### Phase 1: Extend `ActionDropdownItem` with leading widget support

**Files modified:**
- `app/lib/presentation/widgets/chat/action_dropdown_menu.dart`

**Changes:**
- Add `Widget? leadingWidget` field to `ActionDropdownItem`
- In `_ActionRow` build method: when `leadingWidget != null`, render it in place of the default circular icon container (same 36x36 sizing)
- Existing menus (gear, request) are unaffected — they don't set `leadingWidget`

**Tests:**
- Unit tests for `ActionDropdownItem` rendering with and without `leadingWidget`

---

### Phase 2: Build status icon helpers

**Files modified:**
- `app/lib/presentation/screens/experience/experience_content_view.dart`
- `app/lib/core/theme/app_colors.dart`

> **[AUDIT] Added `app_colors.dart`.** Status circle colors must use `AppColors`
> semantic helpers, not hardcoded hex. Reuse existing RSVP badge colors where
> possible. Add new theme-aware helpers only if no existing color fits.

**Changes:**
- Add private helper methods to `_ExperienceContentViewState`:
  - `_buildStatusCircle({required Color backgroundColor, required Color iconColor, required IconData icon})` — returns a 36x36 circle with colored icon, matching the existing icon container dimensions
  - `_buildDetailsStatus(ExperienceState state)` — returns green checkmark when title is non-empty AND description is non-empty AND at least 1 media item exists; default icon otherwise
  - `_buildRsvpStatus(RSVPIntention? intention)` — returns:
    - Default icon (`event_available_outlined`) when no RSVP
    - Green check using `AppColors.rsvpYesBadgeBackground` / `rsvpYesBadgeText` for YES
    - Red X using `AppColors.rsvpNoBadgeBackground` / `rsvpNoBadgeText` for NO
    - Yellow ? using `AppColors.rsvpMaybeBadgeBackground` / `rsvpMaybeBadgeText` for MAYBE
  - `_buildTimeStatus(Experience exp)` — returns:
    - Green check when time is set and no active poll
    - Red icon (e.g. `Icons.how_to_vote`) when `timePollActive == true`
    - Yellow ? when proposals exist but poll is not active and no time confirmed
  - `_buildLocationStatus(ExperienceState state)` — returns:
    - Green check when `locationName != null`
    - Red X when no location

**Tests:**
- Widget tests verifying correct icon/color for each state combination

---

### Phase 3: Rewire owner menu items with checklist order and status icons

**Files modified:**
- `app/lib/presentation/screens/experience/experience_content_view.dart`
- `app/lib/l10n/app_en.arb`
- `app/lib/l10n/app_es.arb`

> **[AUDIT] i18n compliance.** The plan introduces new user-visible strings
> ("Set Details", "Confirm Time", "Confirm Location", "View Impact") but did
> not mention adding them to ARB files. Per `docs/client/i18n.md`: "Every
> user-visible string should use `context.l10n.*`." Added ARB files to modified
> list. New keys should follow the `experience*` prefix convention (e.g.,
> `experienceMenuSetDetails`, `experienceMenuConfirmTime`,
> `experienceMenuConfirmLocation`, `experienceMenuViewImpact`). Both `app_en.arb`
> and `app_es.arb` must be updated, then run `npm run generate`.

**Changes:**
- Reorder menu items from current (RSVP, Time, Details, Completed, Close) to new order (Details, RSVP, Location, Time, Completed, Close)
- Rename "Change Details" → "Set Details"
- Rename "Find Best Time"/"See Time Results" → "Confirm Time"
- Add new "Confirm Location" item (opens location picker, same as tapping the location card in Details tab)
- Wire `leadingWidget` on each item using the Phase 2 helpers
- Keep `onTap` handlers pointing to existing modals/actions
- Add new ARB keys for all renamed/new menu labels; use `context.l10n.*` in the menu-building code

**Tests:**
- Widget tests verifying menu item order, labels, and leading widget presence
- Widget tests need `localizationsDelegates` and `supportedLocales` on their `MaterialApp` wrapper (see `app/test/helpers/l10n_helpers.dart`)

---

### Phase 4: Post-completion menu transformation

**Files modified:**
- `app/lib/presentation/screens/experience/experience_content_view.dart`

**Changes:**
- When `state.experienceDetails.experience.state == EXPERIENCE_STATE_COMPLETED`:
  - Replace all items with a single "View Impact" item (no "Close Event" — resolved in Open Questions)
  - "View Impact" icon: `Icons.insights` or `Icons.bar_chart` with green styling (use `AppColors.rsvpYesBadgeText` or a new `AppColors` helper — no raw hex)
  - `onTap`: open `ExperienceMetricsScreen` via `NavigationHelpers.pushWithSlide()` (slide from right)
- The `isCompleted` check should happen early in the menu-building logic so the entire list is different, not just individual items hidden
- Use `context.l10n.experienceMenuViewImpact` for the label (ARB key added in Phase 3)

**Tests:**
- Widget tests for post-completion menu: verify only "View Impact" appears (no Close Event)
- Verify "View Impact" opens `ExperienceMetricsScreen` via `pushWithSlide`

---

### Phase 5: Polish and edge cases

**Files modified:**
- `app/lib/presentation/screens/experience/experience_content_view.dart`
- `app/lib/presentation/widgets/chat/action_dropdown_menu.dart` (if animation needed)

**Changes:**
- Handle edge cases:
  - Experience with no conversation yet (time poll status defaults)
  - Experience cancelled (not completed) — keep current terminal behavior
  - Non-owner menu (no changes — non-owners don't see the manage menu)
- Verify status icons update reactively when state changes (e.g., owner RSVPs from within the modal, menu should reflect new state on next open)
- Ensure the leading widget sizes consistently across all items (36x36 constraint)
- Accessibility: ensure status icons have semantic labels for screen readers

**Tests:**
- Integration-style widget tests: open menu → perform action → reopen menu → verify status changed
- Edge case tests: no location, no time, no RSVP, cancelled event

> **[AUDIT] Test runner note.** All Flutter tests must use `--reporter expanded`
> per CLAUDE.md. Run with `flutter test --reporter expanded` or `npm run test:app`.

---

## Color Reference

> **[AUDIT] Theming violation fixed.** The original plan used raw hex values
> (`#E8F3E9`, `#3D7048`, etc.) which violates the architecture rule: "Don't use
> hardcoded hex `Color(0xFF...)` values for UI elements." The RSVP badge colors
> already exist in `AppColors` (`rsvpYesBadgeBackground`, `rsvpYesBadgeText`,
> etc., lines 284-306 of `app_colors.dart`). Reuse those. For checklist-specific
> states (time poll, location missing) that don't have existing AppColors helpers,
> add new semantic helpers to `AppColors` (e.g., `checklistSuccess`,
> `checklistWarning`, `checklistError`) rather than hardcoding hex. These should
> be theme-aware for dark mode support.

| State | Background | Icon/Border Color | Icon |
|-------|-----------|-------------------|------|
| Completed (green check) | `AppColors.rsvpYesBadgeBackground` | `AppColors.rsvpYesBadgeText` | `Icons.check` |
| RSVP Yes | `AppColors.rsvpYesBadgeBackground` | `AppColors.rsvpYesBadgeText` | `Icons.check` |
| RSVP No | `AppColors.rsvpNoBadgeBackground` | `AppColors.rsvpNoBadgeText` | `Icons.close` |
| RSVP Maybe | `AppColors.rsvpMaybeBadgeBackground` | `AppColors.rsvpMaybeBadgeText` | `Icons.question_mark` |
| Time poll active | `AppColors.rsvpNoBadgeBackground` | `AppColors.rsvpNoBadgeText` | `Icons.how_to_vote` |
| Time poll complete (unconfirmed) | `AppColors.rsvpMaybeBadgeBackground` | `AppColors.rsvpMaybeBadgeText` | `Icons.question_mark` |
| Location missing | `AppColors.rsvpNoBadgeBackground` | `AppColors.rsvpNoBadgeText` | `Icons.close` |
| Default (no action taken) | `AppColors.surface(context)` | `AppColors.textSecondary(context)` | Original icon |

---

### Phase 6: Request menu checklist redesign

Apply the same checklist pattern to the request content view's owner manage menu.

#### Current request menu (top to bottom):
1. Mark Fulfilled (`check_circle_outline`)
2. Change Details (`edit_outlined`)
3. Close Request (`delete_outline`, destructive)

#### Proposed request menu (top to bottom):

| # | Label | Checklist icon logic |
|---|-------|---------------------|
| 1 | **Set Details** | Green checkmark when title + description + at least 1 media are all present; default icon otherwise (same logic as experience) |
| 2 | **Confirm Location** | Green checkmark if `locationName != null`; neutral default icon if no location (location is optional for requests — no red X) |
| 3 | **Mark Fulfilled** | Current icon until fulfilled. Once fulfilled: items 1-2 are removed, replaced by "View Impact" opening `RequestMetricsScreen` via `NavigationHelpers.pushWithSlide()` |
| 4 | **Close Request** | Unchanged (destructive, always present pre-fulfillment; removed post-fulfillment, same as experience post-completion) |

#### Post-fulfillment menu:
| # | Label | Icon |
|---|-------|------|
| 1 | **View Impact** | Metrics icon (green) |

**Files modified:**
- `app/lib/presentation/screens/request/request_content_view.dart`
- `app/lib/l10n/app_en.arb`
- `app/lib/l10n/app_es.arb`

**Changes:**
- Reorder menu items from current (Fulfilled, Details, Close) to new order (Details, Location, Fulfilled, Close)
- Rename "Change Details" → "Set Details"
- Add new "Confirm Location" item (opens location picker)
- Extract `_buildStatusCircle`, `_buildDetailsStatus`, and `_buildLocationStatus` helpers from `_ExperienceContentViewState` into a shared file (e.g., `app/lib/presentation/widgets/content/checklist_status_icons.dart`) so both views can use them. Update experience content view to import from the shared file.
- For request location status: use `_buildLocationStatus` but pass a flag or variant that returns the neutral default icon (not red X) when location is missing
- Wire `leadingWidget` on each item
- Post-fulfillment: replace all items with "View Impact" opening `RequestMetricsScreen` via `NavigationHelpers.pushWithSlide()` (no "Close Request")
- Add ARB keys: `requestMenuSetDetails`, `requestMenuConfirmLocation`, `requestMenuViewImpact` (both locales)
- Use `context.l10n.*` for all labels

**Tests:**
- Widget tests verifying menu item order, labels, and leading widget presence
- Widget tests for post-fulfillment menu: verify only "View Impact" appears
- Edge case tests: no location, no media, cancelled request

#### Open Questions (Resolved)

1. **"Set Details" check — same fields as experience?** **Resolved: Yes.** Same criteria: title + description + at least 1 media item.
2. **"View Impact" destination for requests.** **Resolved: Open `RequestMetricsScreen` (or `ItemMetricsScreen`) via `NavigationHelpers.pushWithSlide()`.**
3. **Confirm Location — required or optional for requests?** **Resolved: Neutral default icon when no location.** No red X — location is optional for requests, so missing location is not an incomplete state.
4. **Shared status icon helpers.** **Resolved: Extract during this phase.** Phases 1-5 are already implemented. This phase extracts the helpers from `_ExperienceContentViewState` into a shared file as part of the work.

---

**Last Updated:** 2026-03-28
