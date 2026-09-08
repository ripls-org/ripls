---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: The Home tab inbox (#2435, v4.1; restructured by the #2634 converged-nav dock) — an editorial-light Home view (Needs you + a community-pulse feed) assembled by one GetHomeView RPC, plus its one linked "see all" screen (Needs you). The Calendar surface it used to link to is now the standalone Plans dock destination.
  globs: [app/lib/presentation/screens/portfolio/home_tab_screen.dart, app/lib/presentation/screens/portfolio/home_needs_you_see_all_screen.dart, app/lib/presentation/screens/portfolio/home_calendar_screen.dart, app/lib/presentation/screens/portfolio/home_decision_routing.dart, app/lib/presentation/viewmodels/home_tab_view_model.dart, app/lib/presentation/widgets/home/**, app/lib/data/repositories/portfolio_repository.dart, server/services/portfolio/home_view.go]
  triggers: [inbox, home-tab, needs-you, yours, calendar, up-next, decision, mark-done, your-stuff]
  lens: [client, domain]
  domain: client
freshness:
  verified_commit: "82c3518d0"
  verified_on: "2026-08-17"
---
# Home Inbox

## Overview

The inbox is the **Home tab** (#2435, the "v4.1" redesign; restructured again by
the **#2634 converged-nav dock**) — the first destination in the bottom
[`NavDock`](../../app/lib/presentation/widgets/navigation/nav_dock.dart). With the
dock on (the default — see [`navDockEnabledProvider`](../../app/lib/core/config/feature_flags.dart)),
the dock holds four place destinations — **Home · Plans · Library · People** —
and the discovery Feed is no longer directly reachable from the bar; it now only
resurfaces inline as the Home tab's community-pulse section (below). It is a
single scrolling, editorial-light view of everything currently on the viewer's
plate, assembled server-side by **one RPC, `GetHomeView`**, and rendered by
[`HomeTabScreen`](../../app/lib/presentation/screens/portfolio/home_tab_screen.dart).

The Home tab replaced the legacy four-pill portfolio inbox, which — along with
its `ENABLE_HOME_REDESIGN` flag and the `PortfolioInboxScreen` fallback — has
since been removed (#2020). The Home tab is now the unconditional product.

The root view leads with **Needs you** — the decision queue, capped to
`_kRootCap` (= 3) rows with a "See all ›" affordance that opens the one
remaining linked full screen:

| From section | Opens | Screen |
|--------------|-------|--------|
| Needs you | **Needs you** | [`home_needs_you_see_all_screen.dart`](../../app/lib/presentation/screens/portfolio/home_needs_you_see_all_screen.dart) |

The **Calendar** and **Yours** screens this table used to also link to are gone
from the Home tab as of #2634: Calendar is now its own top-level **Plans** dock
destination (see [Calendar / Plans tab](#calendar--plans-tab) below), and the
dedicated Yours see-all screen (`home_gear_see_all_screen.dart`, `YoursRow`) was
deleted outright — see [Yours data (no longer rendered)](#yours-data-no-longer-rendered).

The Needs-you screen reads from the same cached `GetHomeViewResponse`
([`homeTabProvider`](../../app/lib/presentation/viewmodels/home_tab_view_model.dart))
as the root — there is no per-screen fetch. Times are never invented: undated
items never get a fabricated time, and the server excludes them from
time-anchored sections.

---

## Home Root (`HomeTabScreen`)

The body is a single `ListView`, with pull-to-refresh wired to both
`homeTabProvider.notifier.refresh()` and a `feedProvider` reload (the pulse
section is fed by the feed), plus infinite-scroll pagination of the feed near
the bottom. From top to bottom:

1. **Greeting header** — a shared [`DestinationHeader`](../../app/lib/presentation/widgets/destination_header.dart)
   (the same header every dock destination uses) whose title is a two-line
   time-of-day greeting ("Good morning,\n{first name}"); it carries the avatar
   chip and the universal-search chip. There is no separate date banner.
2. **Needs you** — `_buildNeedsYou`. The top `_kRootCap` (= 3) *interpersonal*
   decisions (`MARK_DONE` excluded) rendered as
   [`HomeEditorialRow`](../../app/lib/presentation/widgets/home/home_editorial_row.dart)s
   with dark-sage bullets, each with an inline action pill. The section only
   renders when there's at least one decision or a nonzero total; its "See all"
   affordance carries the full count (e.g. "See all 29 ›") and opens the
   Needs-you screen.
3. **Community pulse** — [`CommunityPulseSection`](../../app/lib/presentation/widgets/home/community_pulse_section.dart),
   the feed retold as a single-column stream of full-width snapshot posts (see
   [Community pulse](#community-pulse-communitypulsesection) below). This is
   the page's long tail — the host screen drives `feedProvider`'s pagination as
   the user nears the bottom, exactly as the old Feed tab did.

There is no more Up-next hero and no Yours preview on the root — both were
removed in the #2634 rewrite. See [Calendar data](#calendar-data) for what
happened to `up_next`, and [Yours data](#yours-data-no-longer-rendered) for
the Yours fields.

**Zero state.** When Needs-you is empty (`needsYou.isEmpty`), the greeting stays
and the Needs-you section collapses into a single focused
[`HomeZeroStateCard`](../../app/lib/presentation/widgets/home/home_empty_states.dart),
rendered above the pulse (which may still show below it once communities have
activity).

The card always shows **exactly one hero**, and which one depends on whether the
server sent an [inbox nudge](#inbox-nudge):

- **No nudge** — the request leads: a prominent hero ("Need a hand with
  something?" → **Ask the crew**, opening request creation) carrying a
  **rotating real-feeling example** that models what's okay to ask for, with a
  quieter offer prompt below it ("Have something to share?" → **Offer
  something**, opening the unified create flow).
- **Nudge present** — the nudge card leads at full card width in its own frame,
  and the request drops to a quiet prompt above the offer prompt. The two
  prompts share one button; the request keeps the filled sage pill and the offer
  keeps the ghost pill, so requesting still reads as the primary of the pair.

The nudge is **never nested inside the request hero** (#2796). A nudge's
`cta_action` is one of four (plan an event, list gear, ask for help, browse
listings), so request-specific framing is incoherent for three of them, and
`NudgeContentView` is a full-bleed composition with its own headline, imagery,
and filled CTA — nesting it put two heroes and two primary buttons in one box.

### Decisions and the routing mixin

Every "Needs you" item is a server-assembled **`HomeDecision`** with a typed
`kind`:

| `HomeDecisionKind` | Meaning |
|--------------------|---------|
| `LENDING_REQUEST` | Someone wants to borrow the viewer's gear |
| `GIVEAWAY_REQUEST` | Someone wants the viewer's giveaway item |
| `ASK_CLAIM` | Someone offered to help on the viewer's request (say thanks) |
| `REPLY` | An unread conversation that needs a reply |
| `MARK_DONE` | A past event or past-due request to close out |
| `TRANSFER_UPDATE` | Gear the viewer is borrowing whose status to advance (pickup) |

[`HomeDecisionRouting`](../../app/lib/presentation/screens/portfolio/home_decision_routing.dart)
is the mixin shared by the root and the Needs-you screen for opening an item
(`openDecision` / `openHomeItem`) and running a decision's full action
(`runDecisionAction` — accept in place, open the thread, confirm a borrowing
transfer in place, or open a completion modal). The single Needs-you number (root badge, OS app-icon badge, and section
header) comes from [`homeNeedsYouCountProvider`](../../app/lib/presentation/viewmodels/home_tab_view_model.dart)
= `effectiveDecisionCount` (the pre-cap server count minus optimistic removals).

---

## Needs you (see-all)

A plain `Scaffold` (back + serif "Needs you · N" title, no search) listing every
open decision **grouped by kind**, each group a label + count:

| Group | Kinds |
|-------|-------|
| **Ready to pick up** | `TRANSFER_UPDATE` |
| **Lend** | `LENDING_REQUEST` / `GIVEAWAY_REQUEST` |
| **Say thanks** | `ASK_CLAIM` |
| **Reply** | `REPLY` |
| **Did it happen?** | `MARK_DONE` (group header carries a **"Mark all done"** action when >1) |

Each row ([`NeedsYouGroupRow`](../../app/lib/presentation/widgets/home/needs_you_group_row.dart))
is a 48px thumbnail/avatar + serif title + muted subtitle (bold first name) + a
trailing **action chip** carrying the decision's `acceptLabel` (e.g. "Mark
picked up"). The chip and the row body are **separate tap targets**: tapping the
chip runs the typed action; tapping the body opens the item.

### Action chips + undo (shared with the root)

This screen uses the **same row contract as the inbox-root preview** — the same
`acceptLabel` chip and the same `runDecisionAction` from `HomeDecisionRouting`,
so the short and comprehensive views behave identically. Tapping a chip opens the
associated modal or screen rather than completing in place:

| Kind | Chip action (`runDecisionAction`) | Undo |
|------|-----------------------------------|------|
| `MARK_DONE` (event) | opens `MarkCompletedModal` | the modal's "Undo" snackbar → `ExperienceRepository.undoCompleteExperience` |
| `MARK_DONE` (request) | opens `MarkFulfilledModal` | the modal's "Undo" snackbar → `RequestRepository.undoMarkRequestFulfilled` |
| `LENDING_REQUEST` / `GIVEAWAY_REQUEST` | accepts in place (`selectRecipient`) | "Undo" snackbar → `undoAcceptDecision` → `TransferRepository.undoSelectRecipient` |
| `ASK_CLAIM` | acknowledges + opens the thread | n/a (opens the thread to say thanks) |
| `REPLY` | opens the conversation | n/a |
| `TRANSFER_UPDATE` | confirms in place via a lightweight confirm dialog, then runs the typed `transferAction` (`START_LOAN` → `startLoan`; `COMPLETE_LOAN` / `COMPLETE_GIVEAWAY` → `completeTransfer`) | "Undo" snackbar → `undoTransferUpdate` → the matching `TransferRepository.undoStartLoan` / `undoCompleteLoan` / `undoCompleteGiveaway` |

**Every state-changing action is undoable, and the undo reflects on the home
view.** The mark-done modals' undo and the lend/give accept-undo all route
through **repository** methods (not the raw service) that mirror the forward
mutation's cache invalidations and fire `onDailyInvalidated` →
`portfolioCacheInvalidationProvider`, so the home view refreshes and the
re-opened decision re-surfaces. (Previously these called the service directly,
which left the decision dismissed on the home view after an undo.) The "Did it
happen?" group keeps a **"Mark all done"** affordance that runs `bulkWrapUp`
(optimistic removal + per-item completion) behind a confirm dialog.

---

## Community pulse (`CommunityPulseSection`)

[`CommunityPulseSection`](../../app/lib/presentation/widgets/home/community_pulse_section.dart)
is the #2634 v3 replacement for the old standalone Feed tab: the feed rendered
as a single-column stream of full-width snapshot posts, one shared anatomy per
post —

1. **Snapshot** — the full-width photo (skeleton shimmer until it paints), an
   optional day pill, and title/body/status overlaid on the standard bottom
   scrim in white.
2. **Footer strip** — who/where: the asker's avatar fronts a request, the
   owner's avatar fronts gear, a glyph fronts stories, with an action CTA in
   the corner and an unread dot on the face.

The corner slot has **three** treatments, not two. A solid pill carries the
give/ask verbs and a ghost pill carries join — but once the viewer has already
acted, the slot states that instead: **Going / Maybe / Not going / Hosting** on
an event, **Offered / Your request** on a request (#2800). Those come from
`viewer_rsvp` / `viewer_has_offered` / `can_edit` on the feed payload (see
[the_feed.md](the_feed.md) → *Viewer-scoped participation*); when the field is
absent — meaning the server could not determine it — the card falls back to the
invite rather than asserting a status.

Two tap targets per post: the snapshot/caption (and CTA) opens the item's
full-screen view; the footer's who-strip opens that person's profile (stories
have nobody fronting them, so their who-strip falls back to the item). A
**status** chip is not a third target — it renders outside any `Tappable`,
because a chip announcing as a button named "Going" would promise an action it
does not perform; the card body still opens the item, so changing an answer
stays one tap away.

It renders every loaded `feedProvider` item — the Home scroll's long tail — with
a loading tail while the next page fetches; only feed items that map to an
openable full-screen view become posts (system items like nudges and milestone
celebrations stay feed-only).

The pulse shows only what `GetFeed` returns, which is bounded by a **14-day
horizon**: items older than that are dropped unless they are live opportunities
(an event still ahead, a request still wanted by its due date, a giveaway still
on offer). Note that the pulse never marks items viewed — the seen-based expiry
belongs to the swipe `FeedScreen`, which no dock destination points at — so the
horizon is the only thing bounding this list (#2799). See
[the_feed.md](the_feed.md) → *The Horizon*.

---

## Yours data (no longer rendered)

The dedicated **Yours** see-all screen — grouped type sections (Requests,
Events, Loans, Items, Completed), inline search, kebab bulk-edit menu, and the
`YoursRow` widget — was deleted outright in the #2634 rewrite
(`home_gear_see_all_screen.dart` and `widgets/home/yours_row.dart` are gone;
grep confirms zero remaining references). Nothing on the Home tab links to a
"Yours" screen any more.

The underlying data is still assembled and returned on `GetHomeViewResponse`
(`your_asks`, `gear`, `your_events`, `recent_activity`), and
[`HomeTabState`](../../app/lib/presentation/viewmodels/home_tab_view_model.dart)
still exposes `visibleGear` / `visibleAsks` / `visibleEvents` plus the
`bulkComplete` mutation — but as of this writing nothing in the client calls
them. Whether a future Library-tab surface (`app/lib/presentation/widgets/discover/`)
takes over this data, or it gets pruned as dead weight, is outside this doc's
scope (`discover/**` is not in its governed globs).

---

## Calendar / Plans tab

The **month × backdrop** calendar (#2514), implemented by
[`HomeCalendarScreen`](../../app/lib/presentation/screens/portfolio/home_calendar_screen.dart).
As of #2634 this is no longer opened *from* the Home tab — there is no more
Up-next hero or Calendar button. Instead the same screen is instantiated with
`embedded: true` as the **Plans** destination, the second slot in the
[`NavDock`](../../app/lib/presentation/widgets/navigation/nav_dock.dart)
(`HomeScreen`'s `IndexedStack`, stack index 4). It is the only place the
widget is constructed in the app; the old "pushed, non-embedded" entry point
this doc used to document no longer has a caller.

The selected day's marquee photo fills the screen behind a floating
Monday-first month grid; in compact windows the bottom half shows the
selected day's detail, while at desktop widths `CalendarPanes` (#2912)
switches to grid-beside-detail instead (see [responsive.md](responsive.md)).
In embedded (Plans-tab) mode, the header is the shared
[`DestinationHeader`](../../app/lib/presentation/widgets/destination_header.dart):
the title is the dynamic month ("July 2026 ▾"), tapping it opens the
**Plans date & filter sheet** ([`PlansDateSheet`](../../app/lib/presentation/widgets/home/calendar/plans_date_sheet.dart))
— typed M/D/Y entry, quick jumps, and a community-group filter — rather than
the plain OS date picker; the subtitle states the active scope (a filtered
group's name, or "across your groups"). The non-embedded header (‹ / ›
chevrons + tap-to-date-picker title) still exists in the widget for
completeness but is currently unreachable. A `SearchScopePill` can further
scope the tab to a universal-search query (embedded only), clearable inline.
Each day cell is a rounded event thumbnail + day number + multi-event badge +
**weather glyph**; the selected day takes an amber ring. The bottom half shows
the selected day's detail:

- **Event day** → the event's full detail (single) or an earliest-first list
  (multi, first row emphasized), over the same backdrop.
- **Open day** → a weather-fitted **suggestion** (proposed activity + the usual
  crew + a "Plan it with them" CTA), else the contextual nudge, else a static
  "Nothing planned" prompt.

Weather sits in every cell and a pill in the detail; see
[weather.md](../weather.md). The screen and its parts live in
[`home_calendar_screen.dart`](../../app/lib/presentation/screens/portfolio/home_calendar_screen.dart)
and [`widgets/home/calendar/`](../../app/lib/presentation/widgets/home/calendar/).

### Calendar data

The Calendar reads a **dedicated `calendar` field** on `GetHomeViewResponse` —
*not* `up_next`. `up_next` must stay upcoming-only for the (now unused)
`isCompletelyEmpty` getter's contract; the calendar instead anchors each item
on the date that matters for it, and includes recent **past** milestones so
the strip can be paged backward (bounded by `homeCalendarPastWindow` = 180 days,
capped at 200, ascending by time).

The calendar covers **everything the viewer has access to, regardless of owner** —
every event, request, and gear shared into one of their communities, not just
items they own — plus the items they host / RSVP'd to and their own loans:

| Calendar entry | Anchor date | `HomeUpNextKind` |
|----------------|-------------|------------------|
| Event (any in the viewer's communities, hosting, going, maybe, or directly invited pre-response) | the event's scheduled date | `EVENT` |
| Request with a due date | the needed-by / due date | `ASK` |
| Request with no due date | the date it was **first shared** | `ASK` |
| Gear (owned or shared into the viewer's communities) | the date it was **first shared / the viewer got access** | `GEAR_SHARED` |
| Upcoming loan / giveaway | pickup, handoff, and due-back dates | `GEAR_OBLIGATION` |

Owned items use their own created date; items owned by others use the date they
were shared into the viewer's community (when the viewer got access).

The calendar additionally carries `repeated DayForecast forecast` and
`repeated OpenDaySuggestion open_day_suggestions` (#2514) for the weather
substrate and open-day suggestions — see [calendar.md](calendar.md) and
[weather.md](../weather.md).

---

## Data Model

`GetHomeView` (request carries only the viewer's timezone) returns one
`GetHomeViewResponse` with every section:

| Field | Drives |
|-------|--------|
| `decisions` | Needs-you queue (capped at 50; root renders top 3) |
| `decision_count` | Pre-cap Needs-you total → badges + section header |
| `up_next` | Upcoming-only, capped at 20; assembled server-side but currently unconsumed by the client (see [Calendar data](#calendar-data)) |
| `your_asks` | Assembled, currently unrendered — see [Yours data](#yours-data-no-longer-rendered) |
| `gear` / `gear_counts` / `gear_has_more` | Assembled (capped at 100), currently unrendered — see [Yours data](#yours-data-no-longer-rendered) |
| `your_events` | Assembled, currently unrendered — see [Yours data](#yours-data-no-longer-rendered) |
| `recent_activity` | Assembled (capped at 50), currently unrendered — see [Yours data](#yours-data-no-longer-rendered) |
| `calendar` | Plans tab (see [Calendar data](#calendar-data)) |
| `nudge` | An optional contextual `NudgePayload` rendered on an empty calendar day / zero-state, reusing the feed nudge system (see [Inbox nudge](#inbox-nudge)) |
| `forecast` | Per-day calendar weather (#2514); best-effort. See [weather.md](../weather.md) |
| `open_day_suggestions` | Weather×history suggestions for open calendar days (#2514) |

Each `HomeDecision` carries `kind`, a typed `reason` (+ `dueAtUnixSec` /
`messagePreview`) the client localizes into the context line, `subjectTitle`,
the counterparty / helper faces, routing (`contentId`, `itemType`),
`transferId`, `communityId`, and a typed `transferAction` the client turns
into the chip label. The server-rendered `title` / `why` / `acceptLabel`
strings were retired in #2835. `HomeUpNextEntry` (reused for both `up_next`
and `calendar`) carries `kind`, `title` (still server-set for events, empty
for gear obligations), a typed `status` (+ `goingCount`) the client
localizes into the role/state line, `timeUnixSec` / `allDay` the client
formats into the clock, thumbnail, routing, and (for requests) `kindTag` /
`helpers`. Its `subtitle` / `timeDisplay` strings were retired the same way.

---

## View Model

[`HomeTabNotifier`](../../app/lib/presentation/viewmodels/home_tab_view_model.dart)
(provider `homeTabProvider`) owns the whole Home view as a Freezed
`HomeTabState`:

```
HomeTabState
  ├── view                     — GetHomeViewResponse (all sections)
  ├── isLoading / error
  ├── removedDecisionIds       — decisions optimistically removed (accepted / dismissed)
  ├── removedYoursIds          — "Yours" rows (gear / request / event) optimistically removed by a bulk action
  ├── activityLastOpenedUnixSec — drives the "N new" activity pill
  └── consumedNudgeIds         — inbox nudge IDs consumed this session (CTA tapped)
```

Derived getters: `visibleDecisions` (drops `removedDecisionIds`),
`visibleGear` / `visibleAsks` / `visibleEvents` (drop `removedYoursIds`; unused
by any current UI — see [Yours data](#yours-data-no-longer-rendered)),
`inboxNudge` (the nudge unless consumed this session),
`effectiveDecisionCount`, `isCompletelyEmpty` (also unused post-#2634 — the
root's zero state now checks Needs-you alone), `newActivityCount`.

Key behaviours:

- **`load()` / `refresh()`** fetch via `PortfolioRepository.getHomeView(tz)`.
  `_prunedRemovals` keeps optimistically-removed IDs hidden only while the server
  still returns them, so a mid-bulk refetch doesn't re-surface rows one at a time.
- **`acceptDecision` / `undoAcceptDecision`** power the lend/give chip: accept
  removes the card optimistically and returns the `communityEventId`; undo
  reverses `selectRecipient` (via the repo, so the home view refreshes) and
  restores the card.
- **`bulkWrapUp`** completes a group of `MARK_DONE` decisions, optimistically
  removing each and rolling back failures ("Mark all done").
- **Background refresh** — a `portfolioCacheInvalidationProvider` listener
  debounces (300 ms) a `refresh()` on any mutation.

---

## Server-Side Assembly

[`home_view.go`](../../server/services/portfolio/home_view.go) implements
`GetHomeView`: it runs the shared `fetchAll()` batch-fetch, then assembles each
section. The per-section assemblers, each capped and sorted:

| Assembler | Builds |
|-----------|--------|
| `assembleHomeDecisions` | The flat Needs-you queue (assembled Lend/Give → pickup → Say thanks → Reply → Mark done, with that precedence driving the `represented` dedup set, then re-sorted oldest-first by created time), one row per underlying item |
| `assembleHomeUpNext` | Upcoming events + dated gear obligations + dated requests (ascending, `< today` excluded) |
| `assembleHomeCalendar` | The calendar timeline across **all items the viewer can see** (community-shared, regardless of owner) — events on their date, requests (due or first-shared), gear first-shared/access markers, and upcoming obligations; includes recent past within `homeCalendarPastWindow` |
| `assembleHomeAsks` | The viewer's open requests with claim progress |
| `assembleHomeGear` | Owner-side loans + listed/at-home library rows with per-category counts |
| `assembleOwnedEvents` | The viewer's active hosted events |
| `assembleHomeActivity` | Recent completed loans / events / fulfilled requests (newest first) |
| `assembleOpenDaySuggestions` | Weather-fitted open-day proposals from the viewer's past activities ([`home_suggestions.go`](../../server/services/portfolio/home_suggestions.go); see [Calendar data](#calendar-data)) |

---

## Inbox nudge

`GetHomeViewResponse.nudge` is an optional `NudgePayload` that **reuses the feed
nudge system** ([nudges.md](../nudges.md)) rather than a bespoke engine. Server
side, [`home_view.go`](../../server/services/portfolio/home_view.go) calls the
feed service through an injected `NudgeProvider` interface
(`feed.Service.InboxNudge`), passing all the viewer's communities
([`orderedCommunityIDs`](../../server/services/portfolio/home_view.go)).
`InboxNudge` reuses the feed pool (`queryActiveNudges` + `pruneExpiredNudges` +
`nudgeToPayload`) but, unlike the feed, **does not require imagery** — a text-only
nudge renders fine on the card's solid background, which removes the stock-imagery
dependency and the feed's two-load latency. It **searches across every community**
and returns the first active **host prompt** it finds. #2936 removed the generic
AI-written pool along with the host-prompt → has-imagery → text-only ranking that
used to sort a mixed pool (see [nudges.md](../nudges.md) → *Host prompts on the
inbox*); with only host prompts left, there is nothing left to rank against.
`InboxNudge` never generates a nudge on this path — when no in-scope community has
an active host prompt, the field is left empty and the client shows its static
prompt. A nudge failure never fails the home view.

Client side, [`HomeTabState.inboxNudge`](../../app/lib/presentation/viewmodels/home_tab_view_model.dart)
exposes the nudge unless it's been consumed this session. The Calendar renders it
on an **empty day** via the shared [`NudgeContentView`](../../app/lib/presentation/widgets/nudge/nudge_content_view.dart)
(falling back to the static "Nothing planned" prompt when absent); tapping the CTA
routes through `consumeInboxNudge`, which optimistically hides it and fires the
existing `ConsumeNudge` RPC. The same field also feeds the zero-state card
([Home Root](#home-root-hometabscreen) → *Zero state*), where the nudge is the
hero rather than an inset.

Both hosts bound the card's height, because `NudgeContentView`'s variants are
`Stack(fit: StackFit.expand)` compositions that cannot size to their content:
320 in the zero state, 280 (floor 200) on the calendar's open day. Both pass
`NudgePresentation.embedded`, which fits the copy to whatever height it is
given rather than clipping it (#2801) — see [nudges.md](../nudges.md) →
*Presentation*.

## Caching

`GetHomeViewResponse` is cached by [`PortfolioRepository`](../../app/lib/data/repositories/portfolio_repository.dart)
under `portfolio:home_view`. `refreshHomeView()` invalidates it; the notifier
invalidates before every pull-to-refresh and (debounced) background refresh.

---

## Key Files

| File | Role |
|------|------|
| [home_tab_screen.dart](../../app/lib/presentation/screens/portfolio/home_tab_screen.dart) | The Home root — greeting header, Needs-you section, community pulse, zero-state card |
| [home_decision_routing.dart](../../app/lib/presentation/screens/portfolio/home_decision_routing.dart) | Shared mixin: open item, run decision action |
| [home_needs_you_see_all_screen.dart](../../app/lib/presentation/screens/portfolio/home_needs_you_see_all_screen.dart) | Needs-you screen — grouped rows, action chips → `runDecisionAction`, Mark all done |
| [home_calendar_screen.dart](../../app/lib/presentation/screens/portfolio/home_calendar_screen.dart) | Calendar / Plans-tab screen — month grid over the selected day's photo backdrop, date & filter sheet (embedded), weather glyphs, open-day suggestions |
| [home_tab_view_model.dart](../../app/lib/presentation/viewmodels/home_tab_view_model.dart) | `HomeTabNotifier` state, completion/undo, refresh suppression, counts |
| [community_pulse_section.dart](../../app/lib/presentation/widgets/home/community_pulse_section.dart) | The feed retold as full-width snapshot posts (#2634 v3) |
| [widgets/home/](../../app/lib/presentation/widgets/home/) | Row/card widgets in current use: `HomeEditorialRow`, `NeedsYouGroupRow`, `HomeZeroStateCard`, `CommunityPulseSection` |
| [portfolio_repository.dart](../../app/lib/data/repositories/portfolio_repository.dart) | `getHomeView` / `refreshHomeView` cache wrapper |
| [home_view.go](../../server/services/portfolio/home_view.go) | `GetHomeView` handler and all section assemblers |

---

**Last Updated:** 2026-07-19
