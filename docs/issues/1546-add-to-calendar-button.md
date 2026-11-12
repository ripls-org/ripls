# Issue #1546: Add to my calendar

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1546
**Filed:** 2026-04-28 | **Last activity:** 2026-04-28
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

A user (Leslie) reports there is no obvious way to add a Ripls event to a Google
or Apple calendar from an event post. The capability is not actually missing —
the underlying machinery shipped in PR #944 (issue #831) and still works on both
platforms — but the entry point is buried.

Today the calendar-export icon sits inside the **time detail modal** rendered by
`TimeReadView` (`app/lib/presentation/widgets/experience/time_read_view.dart:145-190`):
the user has to (1) open an event, (2) tap the time/date chip on the Event tab
to open `UnifiedTimeModal`, and (3) tap the small circular calendar icon inside
the hero time card. From an event post itself, no calendar affordance is
visible.

Per maintainer (and confirmed by the user), the icon should live on the
**main event screen** itself, to the right of the time/place info line, and the
calendar event we create should include a link back into the Ripls event so the
calendar entry stays useful (lets the attendee jump back to chat, RSVP, etc.).

## Context from Comments

Single comment from maintainer (2026-04-28):
- A calendar icon used to be visible directly on the event surface but was
  removed during the refactor of items into the three-tab/four-tab view.
- We should locate the existing `CalendarHelper` code (works on Apple + Google)
  and re-surface it on the main event screen.
- The created calendar event should include a link back into the Ripls event.

A follow-up clarification from the issue author in this conversation:
- Place the icon on the main item screen for events, **to the right of the
  time/place information, right-justified**.

## Codebase findings

- `app/lib/core/utils/calendar_helper.dart` is intact and supports SpecificTime
  and TimeRange events. It builds title/description/location/duration but does
  **not** add a link back to Ripls.
- The current callers are inside the time modal:
  - `time_read_view.dart:148` (non-owner path — fetches fresh data via
    `experienceRepositoryProvider` and reports success/failure via snackbar).
  - `time_set_view.dart:148` (owner path).
- The Event tab is rendered by
  `app/lib/presentation/screens/experience/widgets/experience_event_pane.dart`.
  The time/location chip line is built by `_buildInfoLine()` (line 203–272) as a
  single `Text.rich` with tappable `WidgetSpan`s. It currently has no slot for a
  trailing widget — re-introducing the icon means wrapping that `Text.rich` in a
  `Row` with the calendar button on the right.
- Building a public URL back to a Ripls experience already exists via
  `getOrCreateInviteLink(communityId:, experienceId:)`
  (`item_share_sheet.dart:95-99`), which returns an `inviteUrl` of the form
  `https://ripls.app/go/<shortcode>?experience_id=<id>`. The router already
  handles `/go/:code` (`app/lib/core/router/app_router.dart:212`).
- The `experience_event_pane` already has a `state.communityId` available, so we
  can call `getOrCreateInviteLink` without an additional context fetch.
- Localization keys `timeExportOpeningCalendar` and `timeExportFailed` already
  exist (used by the in-modal icon) and can be reused.

## Potential Solutions

### Option A: Re-surface the icon inline next to time/location, with deep link

- **Approach:** Add a small calendar `Tappable` icon button to the right of the
  time/location info line in `experience_event_pane._buildInfoLine`. Wrap the
  current `Text.rich` in `Expanded` inside a `Row` and place the new icon
  trailing, right-justified.
  Because `ExperienceEventPane` is a `StatelessWidget` with no `ref`, the tap
  is plumbed up as a new `onAddToCalendar` callback (matching the existing
  `onShowTimeModal` / `onShowLocationPickerModal` pattern), wired in
  `experience_content_view.dart` where `ref` is available.
  The handler in `experience_content_view.dart` calls a new
  `ExperienceNotifier.exportToCalendar()` method that owns the URL fetch (via
  `communityRepository.getOrCreateInviteLink(communityId:, experienceId:)` —
  ViewModels are the only layer allowed to call repositories per
  `docs/client/architecture.md`). The notifier returns the URL (or null on
  failure), and the widget then calls `CalendarHelper.addToCalendar(...)` —
  this single platform-UI call is the documented Widget Async Exception. Show
  the existing `timeExportOpeningCalendar` / `timeExportFailed` snackbars.
  Hide the icon when the time is TBD (export is not meaningful —
  `CalendarHelper` already returns `false` in that case but a disabled /
  hidden icon is clearer).
- **Pros:** Matches the user's exact request (right-justified, on the main item
  screen). Reuses all existing code paths — no new platform plumbing. Smallest
  delta. The deep-link addition makes the calendar entry a stable jump-off
  point. Doesn't disturb the in-modal export — that still works for users who
  open the time modal.
- **Cons:** Crowds the info line slightly on narrow screens; need to make sure
  the time text ellipsizes properly when the calendar icon claims its 32–40 px.
- **Effort:** Small.

### Option B: Add a dedicated row under the info line

- **Approach:** Same logic as A, but instead of squeezing the icon into the
  info-line row, add a separate "Add to calendar" pill or icon-button on its own
  row beneath the time/location/source-url block.
- **Pros:** No layout pressure on the info line. Slightly more discoverable
  ("Add to calendar" label).
- **Cons:** Doesn't match the user's stated placement ("to the right of the
  time/place information"). Adds vertical real estate to a content-dense
  surface. Larger visual change.
- **Effort:** Small.

### Option C: Move the action into the owner manage menu and the non-owner RSVP flow

- **Approach:** Add an "Add to calendar" `ActionDropdownItem` to the owner
  manage menu (`experience_event_pane.dart:446-507`) and a tertiary action next
  to the RSVP button for non-owners.
- **Pros:** Discoverable for owners; uses existing patterns.
- **Cons:** Owners discover it via a menu; non-owners need a different surface.
  Inconsistent with the user's request. Buries the action again — same class of
  problem the issue is reporting.
- **Effort:** Small–medium.

## Recommendation

**Option A.** It directly satisfies the user's clarified placement, reuses
`CalendarHelper` and existing l10n strings, and is the smallest change with the
clearest behavior. The deep-link-back addition is a small extension to
`CalendarHelper` that benefits the existing in-modal call sites for free.

The right-justified-icon placement also implicitly *deprecates* the icon inside
the time modal — once the inline button is shipped, the in-modal icon becomes
redundant. We should leave it for one release for muscle-memory continuity, then
remove it in a follow-up.

## Open Questions

- **Should the calendar event description embed the Ripls URL, or should we use
  the calendar Event's URL field?** — Recommendation: append it to the
  description as a labeled line ("View in Ripls: <url>"). The `add_2_calendar`
  package's `Event` does not expose a dedicated URL field on Android, and
  Google/Apple Calendar both render http(s) URLs in description as tappable
  links. Keeps platform behavior identical.
- **Should we always require a community context to mint the share URL?** —
  Recommendation: yes — use `state.communityId` (the community in which the
  user is currently viewing the event). It is already loaded by the time the
  Event pane renders. If `communityId` is empty for any reason (defensive),
  fall back to skipping the URL — calendar export still works, just without the
  link-back. Don't block the export on URL minting.
- **Should the URL minting block the calendar tap?** — Recommendation: do it in
  parallel-but-sequential — call `getOrCreateInviteLink` first (it's a single
  RPC, typically <100 ms and cached client-side), then call
  `CalendarHelper.addToCalendar()`. If the link fetch fails, log and fall
  through with no URL appended. Show a single snackbar, only on the calendar
  outcome, to avoid double-feedback.
- **Should the icon be hidden when the experience time is TBD?** —
  Recommendation: yes — hide it entirely when neither `time.hasSpecific()` nor
  `time.hasRange()` is true. Otherwise the user taps a button and gets a silent
  no-op or failure snackbar. The condition is already encoded in
  `CalendarHelper.addToCalendar`.
- **Should we add an analytics event for the inline tap?** — Recommendation:
  yes, lightly — emit an `ExperienceCalendarExportTappedEvent` (or reuse an
  existing event taxonomy) so we can measure adoption now that the action is
  surfaced on the main screen. Small follow-up if no taxonomy fits.
- **Should we eventually remove the in-modal icon?** — Recommendation: keep it
  for this release to avoid surprising existing users; file a small follow-up to
  remove it once the inline action ships. Out of scope for this issue.

## Implementation Plan

### Phase 1: Add deep-link support to CalendarHelper

**Goal:** `CalendarHelper` can accept (or look up) a Ripls URL and embed it in
the calendar event description. Existing callers continue to work without any
URL.

**Tasks:**
- [ ] Extend `CalendarHelper.addToCalendar()` to take an optional
      `String? riplsUrl` argument (default `null`).
- [ ] In `_buildFromSpecificTime` / `_buildFromTimeRange`, when `riplsUrl` is
      provided, append `\n\nView in Ripls: <url>` to the description (or set
      description to just the URL line if `experience.description` is empty).
- [ ] Update the two existing in-modal call sites (`time_read_view.dart:154`
      and `time_set_view.dart:148`) to source the URL via the experience
      notifier's new `exportToCalendar()` method (per Phase 2). These widgets
      currently access `experienceRepositoryProvider` directly under the
      Widget Async Exception comment — the URL fetch is a regular RPC that
      doesn't qualify for the exception, so it must go through the notifier.
      Tolerate URL failures by passing `null` to `addToCalendar`.
- [ ] Unit test: `app/test/core/utils/calendar_helper_test.dart` — verify
      description is correctly composed in the four shapes (description+url,
      description-only, url-only, neither). Run with
      `flutter test --reporter expanded`.

**Validates:** The link-back appears in calendar entries created from the
existing entry points; nothing else changes visually.

### Phase 2: Add the inline calendar button on the Event tab

**Goal:** A right-justified calendar icon appears on the Event tab, in the same
row as the time/location info, and tapping it adds the event to the user's
calendar with the Ripls link embedded.

**Tasks:**
- [ ] Add `exportToCalendar()` to `ExperienceNotifier` (path:
      `app/lib/presentation/viewmodels/experience_view_model.dart`). The
      method takes no args (uses notifier `state.communityId` and the loaded
      experience), calls `communityRepository.getOrCreateInviteLink(...)`, and
      returns a small result record / freezed value containing the
      `Experience` and the optional `riplsUrl` (null on URL fetch failure).
      The notifier does **not** call `CalendarHelper` itself — the platform
      calendar UI call stays in the widget under the Widget Async Exception.
      Add `// ref.mounted` checks per `docs/client/architecture.md` async
      safety patterns.
- [ ] Add a viewmodel unit test for `exportToCalendar()` covering: success
      (URL returned), URL fetch failure (null returned, no exception),
      community-id-empty defensive case (null returned). Use
      `ProviderContainer` + a fake `CommunityRepository` per
      `docs/client/architecture.md` testing guidance.
- [ ] In `experience_event_pane.dart`, refactor `_buildInfoLine` so the
      `Text.rich` is wrapped in `Expanded` inside a `Row`, with a trailing
      calendar icon button when (a) the experience time is specific or has a
      range and (b) the experience is not in a state where export makes no
      sense (TBD, deleted). Cancelled events still allow export — the user may
      want to remove the placeholder calendar entry, or just record that it was
      cancelled. Owner edit-mode hides the icon (consistent with other inline
      actions). Add a new `onAddToCalendar` `VoidCallback` to the pane's
      constructor — keep parity with the existing callback-based pattern
      (`onShowTimeModal`, `onShowLocationPickerModal`).
- [ ] Wire `onAddToCalendar` in `experience_content_view.dart` to a new
      `_addToCalendar()` handler. The handler:
      1. Calls
         `ref.read(experienceProvider(experienceId).notifier).exportToCalendar()`.
      2. Awaits the returned record, then calls
         `CalendarHelper.addToCalendar(experience, riplsUrl: url)` — with the
         `// Widget async exception: platform calendar UI` comment per
         `docs/client/architecture.md`.
      3. Adds a `if (!context.mounted) return;` guard before the snackbar.
      4. Shows `timeExportOpeningCalendar` (success) or `timeExportFailed`
         (failure) via `ScaffoldMessenger`.
- [ ] Use `AppColors.transferSage` (or the local accent already in use on the
      Event tab) for the calendar icon — no hardcoded `Colors.*` per
      `docs/client/architecture.md` theming rules.
- [ ] Add a new accessibility label (`a11yExpAddToCalendar`) — distinct from
      the existing `a11yExpExportToCalendar` so the screen reader output
      references the visible affordance ("Add to calendar"). Add the key to
      both `app_en.arb` and `app_es.arb` per `docs/client/i18n.md` ("Each new
      string needs both locales"). Run `npm run generate` so
      `AppLocalizations` regenerates. (Or reuse `a11yExpExportToCalendar` if
      its wording is acceptable — check first.)
- [ ] Verify the icon renders correctly with long location names / long
      informal time descriptions — on overflow, the time text should
      ellipsize, not the icon.
- [ ] Visual test on Android emulator and iOS simulator: tap the inline icon,
      confirm the right calendar app opens with title, description (including
      Ripls URL), location, and time pre-filled. Use the
      `ripls-app-logs` skill to watch for errors during the test.
- [ ] Add an `experience_event_pane_test.dart` widget test asserting the icon
      is present when `time.hasSpecific()` and absent when the time is TBD.
      Wrap in the `MaterialApp` localization helpers
      (`app/test/helpers/l10n_helpers.dart`) per `docs/client/i18n.md`. Run
      with `flutter test --reporter expanded`.

**Validates:** The user's reported gap is closed — calendar export is one tap
from the event post.

### Phase 3: Polish & follow-ups

**Goal:** The new path is observable and we have a clean handoff for retiring
the duplicate in-modal icon.

**Tasks:**
- [ ] Emit an analytics event on tap (decide on event name during
      implementation; if no taxonomy fits, skip and file a follow-up).
- [ ] File a tracking issue to remove the in-modal calendar icon in
      `time_read_view.dart` / `time_set_view.dart` after one release of soak
      time on the inline button.
- [ ] Update `docs/issues/831-calendar-export.md` with a "superseded by #1546"
      note pointing readers at the new entry point (light cross-reference).

**Validates:** The change is durable — measurable, documented, and the
duplicate UI has a clear removal plan.

## Related Issues

- #831 — original calendar export feature; this issue re-surfaces its UI after
  the experience-screen refactor hid it.
- #944 — PR that fixed Android calendar intent resolution (still required;
  nothing in this plan touches the manifest).

## Audit notes (2026-04-28)

`/ripls-audit` reviewed this plan against `docs/client/architecture.md`,
`docs/client/caching.md`, and `docs/client/i18n.md`. Changes applied in-place:

- **Layering fix.** Original Phase 2 had the widget call
  `getOrCreateInviteLink` directly. That violates the rule "ViewModels are the
  ONLY layer that calls Repositories" — only the platform calendar UI call
  qualifies for the documented Widget Async Exception. Phase 2 now adds an
  `ExperienceNotifier.exportToCalendar()` method that owns the URL fetch and
  returns a result record; the widget then calls `CalendarHelper.addToCalendar`
  (the legitimate platform-UI exception). Option A description updated to match.
- **Widget-class fix.** `ExperienceEventPane` is a `StatelessWidget` with no
  `ref`; tap callbacks are wired in `experience_content_view.dart` (which has
  `ref`). Phase 2 now adds an `onAddToCalendar` callback to the pane and wires
  the handler in `experience_content_view.dart`, matching the existing
  `onShowTimeModal` / `onShowLocationPickerModal` callback pattern.
- **In-modal call sites also rerouted.** Phase 1 no longer suggests the widget
  in `time_read_view.dart` / `time_set_view.dart` mint the URL itself — those
  call sites now also funnel through `exportToCalendar()` for consistency with
  the layering rule.
- **i18n reinforcement.** Phase 2 explicitly cites the "each new string needs
  both locales" rule from `docs/client/i18n.md` and notes that `npm run
  generate` must be re-run after touching ARB files.
- **Theming rule called out.** Phase 2 adds an explicit step to use `AppColors`
  for the new icon (no hardcoded `Colors.*` per `docs/client/architecture.md`).
- **Test convention.** Both unit tests and widget tests now specify the
  `--reporter expanded` flag (CLAUDE.md / project convention) and the widget
  test references `app/test/helpers/l10n_helpers.dart` per
  `docs/client/i18n.md`.
- **Async-safety.** Phase 2 calls out `if (!context.mounted) return;` between
  the await and the snackbar, and `ref.mounted` checks inside the notifier
  method, per the architecture's "Async Safety in Notifiers" guidance.

No caching-doc compliance issues found — `getOrCreateInviteLink` already routes
through `communityRepositoryProvider` with transparent caching, and the new
notifier method is a pass-through to that repository.
