---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: The calendar timeline (GetHomeView.calendar) — which items appear, the per-kind anchor dates, and how it renders. One server-assembled source feeds the inbox Calendar (all communities), the Workshop community calendar (filtered to one community), and the shared community/person profile calendar door.
  globs: [app/lib/presentation/screens/portfolio/home_calendar_screen.dart, app/lib/presentation/widgets/home/calendar/**, app/lib/presentation/screens/workshop/workshop_library_calendar_panel.dart, app/lib/presentation/screens/workshop/workshop_library_common.dart, app/lib/presentation/screens/communities/shared_calendar_screen.dart, server/services/portfolio/home_view.go, server/services/portfolio/home_view_upnext.go]
  triggers: [calendar, up-next, agenda, week-view, scheduled-date, due-date, first-shared, community-calendar, when, plans]
  lens: [client, domain]
  domain: client
freshness:
  verified_commit: "82c3518d0"
  verified_on: "2026-08-17"
---
# Calendar

## Overview

There is **one calendar timeline**, assembled server-side and reused by three
surfaces:

| Surface | Scope | Screen / widget |
|---------|-------|-----------------|
| **Inbox Calendar** (the Plans destination on the bottom nav dock, #2634) | every community the viewer can see | [`home_calendar_screen.dart`](../../app/lib/presentation/screens/portfolio/home_calendar_screen.dart) → [`widgets/home/calendar/`](../../app/lib/presentation/widgets/home/calendar/) (month × backdrop) |
| **Community calendar** (the Workshop "When" destination) | a single community | [`workshop_library_calendar_panel.dart`](../../app/lib/presentation/screens/workshop/workshop_library_calendar_panel.dart) |
| **Shared calendar** (the community/person profile "Calendar" door, #2568) | one community, or the communities a viewer shares with a person | [`shared_calendar_screen.dart`](../../app/lib/presentation/screens/communities/shared_calendar_screen.dart) |

All three render the **same entries** — a `repeated HomeUpNextEntry calendar` field on
`GetHomeViewResponse`, built by **`assembleHomeCalendar`** in
[`home_view_upnext.go`](../../server/services/portfolio/home_view_upnext.go),
next to the `assembleHomeUpNext` it is deliberately distinct from. The only
difference is the filter: the inbox shows the whole timeline; the community and
shared calendars keep the entries whose `community_id` is in scope. The
client never invents dates — it groups by the server's `time_unix_sec`. The
clock label itself is composed client-side, locale-aware, from `time_unix_sec`
/ `all_day` (`homeUpNextTimeLabel` in
[`home_up_next_copy.dart`](../../app/lib/presentation/widgets/home/home_up_next_copy.dart));
the server stopped rendering row copy in #2835.

This is intentionally **not** the `up_next` field. `up_next` is upcoming-only
(it also feeds the root Up-next hero and the empty-state check); the calendar
anchors each item on the date that matters for it and includes recent past so
the week strip can page backward. See [inbox.md](inbox.md) for the rest of the
Home tab.

---

## What appears, and the date each item is anchored on

`assembleHomeCalendar` covers **everything the viewer has access to, regardless
of owner** — every event, request, and gear shared into one of their
communities, plus the items they host / RSVP'd to and their own loans. Times are
never fabricated: an item with no usable date is excluded.

| Calendar entry | `HomeUpNextKind` | Anchor date (`time_unix_sec`) | Server field read |
|----------------|------------------|-------------------------------|-------------------|
| **Event** in a viewer's community, hosting, going, maybe, or directly invited (pre-response) | `EVENT` | the event's scheduled date | `experienceScheduledTime(e)` (`scheduled_at_unix_sec`) |
| **Request** with a due date | `ASK` | the needed-by / due date (past-due excluded) | `needed_by_unix_sec` |
| **Request** with no due date | `ASK` | the date it was **first shared** (else created); ancient excluded | `CommunityRequest.shared_at_unix_sec`, else `created_at_unix_sec` |
| **Gear** (owned or shared in) | `GEAR_SHARED` | the date it was **first shared / the viewer got access** | owned: `created_at_unix_sec`; shared: `CommunityGear.created_at_unix_sec` |
| **Loan pickup** | `GEAR_OBLIGATION` | the estimated pickup date | `estimated_pickup_unix_sec` |
| **Loan due-back** | `GEAR_OBLIGATION` | the expected return date | `expected_return_unix_sec` |

**A single item can produce two entries.** An active loan contributes both a
**pickup** entry (state `RECIPIENT_SELECTED`) and a **due-back** entry (state
`ACTIVE`). A giveaway contributes only a pickup (it completes on handoff).

**Owner vs. shared dates.** Owned items use their own created date; items owned
by others use the date they were shared into the viewer's community (when the
viewer got access).

**Window, cap, sort.** Includes recent past bounded by `homeCalendarPastWindow`
(180 days), capped at `homeMaxCalendar` (200), ascending by `time_unix_sec`.

### Every entry carries its community

Each `HomeUpNextEntry` sets `community_id` (field 7) — events use the resolved
sharing community, obligations use `transfer.community_id`, requests/gear use the
first community the item is shared into. This is what lets the community
calendar filter the shared timeline without a separate RPC or re-implemented
date logic.

---

## The entry shape

`HomeUpNextEntry` ([portfolio.proto](../../proto/ripls/api/portfolio.proto)) is
reused for both `up_next` and `calendar`:

| Field | Use |
|-------|-----|
| `kind` | `EVENT` / `GEAR_OBLIGATION` / `ASK` / `GEAR_SHARED` (`GEAR_SHARED` is calendar-only) |
| `time_unix_sec` | the anchor date — **always set**; used for day grouping |
| `all_day` | date-only entry (requests, obligations, all-day events) — client shows an all-day label instead of a clock |
| `title` | row text — the item's own name for events/requests/gear (verbatim user content); empty for gear obligations, whose title the client composes from `status` + `gear_name` + `counterparty_first_name` |
| `status` / `going_count` | typed role/state (`HomeUpNextStatus`) the client localizes into the role/state line (e.g. "Going", "Borrowing", "8 going") |
| `community_id` / `community_name` | provenance + the community filter |
| `thumbnail_media_id` | row/card media |
| `content_id` / `item_type` | navigation target |
| `kind_tag` / `helpers` / `needed_count` | request-specific extras |

`item_type` (`DailyItemType`) drives routing. There is **no** `DAILY_ITEM_TYPE_GEAR`:
gear-shared and obligation entries carry the **gear id** in `content_id` with
`item_type = DAILY_ITEM_TYPE_TRANSFER`, which routes to the gear screen. The
routing all calendars use:

| `item_type` | Opens |
|-------------|-------|
| `TRANSFER` / `GIVEAWAY` | gear screen (`content_id` = gear id) |
| `EXPERIENCE` | experience screen |
| `REQUEST` | request screen |
| anything else | not navigable |

---

## Rendering

All three surfaces share the same grouping: bucket entries by the **local day** of
`time_unix_sec`, page a week at a time, show a per-day count dot, and render an
agenda for the selected day (with a "nothing planned" nudge for empty days). The
time string on each row is composed client-side from `time_unix_sec` / `all_day`
(`homeUpNextTimeLabel`), not read from the server.

- **Inbox** — the **month × backdrop** calendar (#2514,
  [`home_calendar_screen.dart`](../../app/lib/presentation/screens/portfolio/home_calendar_screen.dart)
  + [`widgets/home/calendar/`](../../app/lib/presentation/widgets/home/calendar/)):
  the selected day's marquee photo fills the screen behind a floating
  Monday-first month grid. Each day cell is a rounded thumbnail of the day's
  earliest photo'd event, the day number, a multi-event count badge, and a
  **weather glyph** (see [weather.md](../weather.md)); today's number is amber
  with a dot, the selected day an amber ring. In compact windows the bottom
  half is the selected day's detail; at desktop widths `CalendarPanes` (#2912)
  switches to grid-beside-detail instead (see [responsive.md](responsive.md)).
  The detail itself (`CalendarDayDetail`) is a single event's full detail, a
  multi-event list (earliest-first, first emphasized), or — on an open day — a
  weather-fitted **open-day suggestion** (`CalendarSuggestionPanel`) or a
  static "plan an event" prompt. The generic contextual nudge that used to
  fill an open day with no suggestion was removed in #2936 — an open day is
  already the whole message. The backdrop crossfades on day change
  unless reduce-motion is on. (The legacy week-strip `UpNextWeekView` /
  `CalendarDayHero` widgets are retired from this screen.) As the Plans
  destination (`embedded: true`, #2634) the same screen adds a group-filter
  and date-jump sheet (`PlansDateSheet`) and an optional universal-search
  scope pill; pushed standalone it keeps the original soonest-day seek.
- **Community** — `WorkshopLibraryCalendarPanel` (#2514): the **same month ×
  backdrop design** as the inbox calendar, composed from the same shared widgets
  (`CalendarBackdrop` + `CalendarScrim` + `CalendarPanes` (grid/detail arrangement,
  #2912) + `CalendarMonthGrid` + `CalendarDayDetail`),
  filtered to one community. It sits in the Workshop's morph panel with its own
  month-nav header (the close X is the morph's), and reuses the same suggestion →
  create flow (refreshing the shared home view and re-reading the community
  calendar on save). Weather and open-day suggestions are the viewer's (not
  community-scoped).
- **Shared** — `SharedCalendarScreen` (#2568): the same month × backdrop
  design and shared widgets, reused from a community or person profile's
  "Calendar" door, filtered to the given community ids. Unlike the Workshop
  panel it is a plain pushed screen, not a morph panel; it otherwise reuses
  the same suggestion → create flow. Weather and open-day suggestions are the
  viewer's (not scoped).

### Weather and open-day suggestions (#2514)

`GetHomeViewResponse` carries two calendar companions to the `calendar` list:

- **`repeated DayForecast forecast`** — per-day weather (a coarse condition, a
  pre-formatted temperature, an `is_typical` flag for climate-normal days). A
  **sibling list**, not a field on `HomeUpNextEntry` — open days carry no entry,
  and `HomeUpNextEntry` is reused by `up_next`. Best-effort; a weather failure
  never fails the view. Full subsystem: [weather.md](../weather.md).
- **`repeated OpenDaySuggestion open_day_suggestions`** — for open days only,
  built from the viewer's past activities × the usual crew × that day's weather
  ([home_suggestions.go](../../server/services/portfolio/home_suggestions.go)).
  An activity only qualifies once the viewer has done it **at least twice**,
  where "twice" counts semantically-similar events as one activity (the viewer's
  involved experiences are clustered by a **precomputed name-only embedding** —
  read via `storage.GroupIDsByStoredEmbedding` — at a picky similarity threshold,
  so "Flatirons hike" and "Sunday hike" accrue together while unrelated
  activities don't) — this suppresses the awkward one-off suggestion from #2674.
  Beyond-horizon ("typical") days are flagged `is_low_confidence`. Empty when no
  activity clears the ≥2 bar (or there's no weather); the client falls back to
  the static "plan an event" prompt (the generic contextual-nudge fallback was
  removed in #2936).

The client buckets all three streams by local day in
[`CalendarMonthData`](../../app/lib/presentation/widgets/home/calendar/calendar_month_data.dart).

### Community calendar data source

[`workshopCommunityCalendarProvider`](../../app/lib/presentation/screens/workshop/workshop_library_common.dart)
(autoDispose family, keyed by the comma-joined community ids) resolves the
viewer timezone, reads the **cached** `GetHomeView` (the same fetch the inbox
calendar uses — no second round-trip, no duplicated logic), and returns a
`WorkshopCalendarData` record: the `calendar` entries whose `community_id` is in
scope, plus the response's `forecast` and `open_day_suggestions` (the viewer's,
unfiltered). Because it shares the cache, both the community calendar and the
shared calendar (`SharedCalendarScreen`, keyed the same way) stay in lock-step
with the inbox calendar.

> Historical note: the community calendar previously sourced the Workshop
> brief's `available_now_items` (which carry no per-item dates) and anchored
> everything to "today" — that's the bug this provider replaced.

---

## Server-side assembly

`assembleHomeCalendar` ([home_view_upnext.go](../../server/services/portfolio/home_view_upnext.go))
runs after the shared `fetchAll()` batch-fetch and emits the entries described
above. It is one of the per-section assemblers behind `GetHomeView`; see
[inbox.md → Server-Side Assembly](inbox.md#server-side-assembly).

`GetHomeViewResponse` is cached by [`PortfolioRepository`](../../app/lib/data/repositories/portfolio_repository.dart)
under `portfolio:home_view`; all three calendars read through that cache.

---

## Key files

| File | Role |
|------|------|
| [home_view_upnext.go](../../server/services/portfolio/home_view_upnext.go) | `assembleHomeCalendar` — the canonical item-selection + anchor-date logic |
| [home_view.go](../../server/services/portfolio/home_view.go) | `assembleHomeView` calls it; holds `homeMaxCalendar` (200) and `homeCalendarPastWindow` (180 days) |
| [portfolio.proto](../../proto/ripls/api/portfolio.proto) | `HomeUpNextEntry`, `GetHomeViewResponse.calendar` |
| [home_calendar_screen.dart](../../app/lib/presentation/screens/portfolio/home_calendar_screen.dart) | inbox Calendar screen (month × backdrop, month picker, entry routing) |
| [widgets/home/calendar/](../../app/lib/presentation/widgets/home/calendar/) | inbox month × backdrop pieces — `CalendarMonthData` (per-day buckets), `CalendarMonthGrid` / `CalendarDayCell` (the Monday-first grid), `CalendarBackdrop`, `CalendarPanes` / `month_grid_metrics.dart` (stacked-vs-side-by-side grid/detail arrangement at desktop widths, #2912 — see [responsive.md](responsive.md)), `CalendarDayDetail`, `CalendarSuggestionPanel`, weather glyphs |
| [home_up_next_copy.dart](../../app/lib/presentation/widgets/home/home_up_next_copy.dart) | `homeUpNextTitle` / `homeUpNextStatusLabel` / `homeUpNextTimeLabel` — client-side, locale-aware row copy resolved from `status` / `going_count` / `all_day` / `time_unix_sec` (#2827) |
| [workshop_library_calendar_panel.dart](../../app/lib/presentation/screens/workshop/workshop_library_calendar_panel.dart) | community calendar panel |
| [workshop_library_common.dart](../../app/lib/presentation/screens/workshop/workshop_library_common.dart) | `workshopCommunityCalendarProvider` (community filter over the shared timeline; also backs the shared calendar) |
| [shared_calendar_screen.dart](../../app/lib/presentation/screens/communities/shared_calendar_screen.dart) | shared calendar screen — community/person profile "Calendar" door (#2568), pushed screen (not a morph panel) |

---

**Last Updated:** 2026-08-02
