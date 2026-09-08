---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: HISTORICAL — documents GetPortfolioInboxView and its inbox assembly (shared inventory, weekly impact metrics, people row, actions today, needs-attention/coming-up), which have been fully removed (#2834, 2026-08-01). Retained pending a decision on replacement docs for the current GetHomeView / GetDirectoryPeople assemblies.
  globs: [server/services/portfolio/**]
  triggers: [portfolio-inbox, daily-view, weekly-metrics, quality-time, shared-inventory, needs-attention, actions-today, coming-up]
  lens: [server, domain]
  domain: server
freshness:
  verified_commit: "f32ce281c"
  verified_on: "2026-08-02"
---
# Portfolio Inbox — Server Assembly (REMOVED)

> **This RPC and the assembly it describes no longer exist.**
> `GetPortfolioInboxView` and ~2,400 lines of supporting code (`inbox_view.go`,
> `metrics.go`, `actions.go`, `assembly.go` — shared inventory, weekly impact
> metrics, people row, actions today, needs-attention/coming-up) were deleted
> 2026-07-30 once #2820 found no viewmodel caller for any of the four legacy
> portfolio-inbox RPCs (`GetPortfolioInboxView`, `GetPortfolioInboxWeek`,
> `GetPortfolioInboxFeed`, `GetMyStuffItems`). The Directory's read of
> `GetPortfolioInboxView.people` was the one live caller; it moved to a
> dedicated `GetDirectoryPeople` RPC
> ([directory_people.go](../../server/services/portfolio/directory_people.go)),
> which is where `buildCommunityPeople` and `sharedCommunityNamesByUser` now
> live. A small legacy shim kept `GetPortfolioInboxView` itself alive for that
> one caller; the shim and the RPC were deleted for good on 2026-08-01
> (closes #2834).
>
> Everything below this notice describes the deleted assembly and does not
> match any code currently in the repo. It is kept as a historical record
> pending a decision on whether to replace it with docs for the current
> `GetHomeView` (`home_view*.go`) / `GetDirectoryPeople` (`directory_people.go`)
> assemblies, or retire this file outright.

---

This doc covers the **server-side assembly** of the (now-removed)
`GetPortfolioInboxView` response: how the portfolio service joined transfers,
experiences, requests, and community items into a single personal inbox view
aggregated across **all of the user's communities**, plus the weekly impact
metrics that rode along with it.

The RPC is implemented by `GetPortfolioInboxView` in
[server/services/portfolio/inbox_view.go](../../server/services/portfolio/inbox_view.go).
The request carries only the user's IANA `timezone`
([proto/ripls/api/portfolio.proto](../../proto/ripls/api/portfolio.proto), the
`GetPortfolioInboxViewRequest` message); all entity selection comes from the
authenticated user's communities. The response message
(`GetPortfolioInboxViewResponse`) and its `DailyItem` / `DailyGroup` /
`DailyPerson` / `DailyAction` / `WeeklyMetrics` / `SharedItem` types are also
defined in `portfolio.proto`.

For the **client** rendering of this data — the four-pill portfolio inbox UI —
see [docs/client/inbox.md](../client/inbox.md). (That doc has not been
independently re-verified as part of this pass; it may itself need updating
now that the server-side RPC is gone.)

---

## Cross-community scope

Every section below is assembled across **all of the user's communities**, not a
single community. `GetPortfolioInboxView` calls `s.fetchAll(ctx, userID)` to load
the user's communities and their items, then short-circuits to an empty response
(only a date label and the goal-only `WeeklyMetrics`) when `d.communityIDs` is
empty. Weekly metrics are computed with
`computeWeeklyMetrics(ctx, userID, d.communityIDs, tz, now)` — the full community
set, not one community.

The bulk of assembly happens in `assembleInboxView`, a **pure function**: all
storage I/O runs before it (in `fetchAll` and `fetchInboxViewExtras`), and it
joins the pre-fetched maps into the response.

---

## Shared inventory

The user's total shared value and inventory list are assembled by
`assembleSharedInventory` in
[server/services/portfolio/actions.go](../../server/services/portfolio/actions.go).
It includes, across all of the user's communities:

- All active loans where the user is the lender (transfer state
  `RECIPIENT_SELECTED` or `ACTIVE`, non-giveaway).
- All gear the user owns that is listed as available in any of their communities
  (`CommunityGear` not archived, availability `FOR_LOAN` or `FOR_GIVEAWAY`;
  resolved gear must be `GEAR_STATE_AVAILABLE`, not deleted, owned by the user).

Items are sorted by estimated value descending (`Gear.ValueEstimate.EstimatedValueUsd`).
Each entry carries the item name, its formatted USD value, and a status line
("Lent to [first name]" or "Available"). The total `SharedValue` is the sum of
all estimated values, formatted as USD. When there are no entries the function
returns an empty value and `nil` slice.

---

## Weekly metrics

Computed by `computeWeeklyMetrics` in
[server/services/portfolio/metrics.go](../../server/services/portfolio/metrics.go).
Source data is all **completed** transfers (queried by both `owner_id` and
`recipient_id`, then filtered to the user's communities), completed experiences
(host or YES/MAYBE RSVP), and fulfilled requests (requester or confirmed helper)
whose completion timestamp falls within the current ISO week (Monday–Sunday) in
the user's timezone. Week bounds come from `isoWeekBounds(now, tz)` in
`metrics.go`, which returns the local Monday 00:00 and the following Monday.

Four dimensions are produced, each with a per-item breakdown of `MetricLineItem`
rows (item name, formatted amount, day-of-week note, `content_id`, and
`item_type` so the row is tappable on the client):

### Quality minutes

- **What it measures:** quality-time minutes earned from completed sharing
  activity this week.
- **Who earns it:** every participant earns quality time from each completed
  transfer, completed experience, or fulfilled request they were involved in.
- **Source field:** `ImpactEstimate.QualityTime.QualityTimeMinutes.Mean` on each
  completed transfer, completed experience, and fulfilled request
  (`metrics.go`, e.g. line 190 for transfers, with the same field read for
  experiences and requests).
- **Goal:** `qualityTimeGoal = 360` minutes per week (constant in
  [assembly.go](../../server/services/portfolio/assembly.go)).
- **Breakdown:** one line item per completed item that contributed quality time,
  each showing the amount earned (e.g. "+30"). `weeklyEncouragement` produces a
  headline/subtitle based on progress toward the goal.

### Cost saved

- **What it measures:** money saved by using community alternatives instead of
  buying or renting.
- **Who earns it:** every participant — both owner and recipient for transfers;
  every attendee (including host) for experiences; the requester and all
  confirmed helpers for fulfilled requests. Each person gets individual credit;
  community-level deduplication happens separately in the impact service.
- **Source field:** `ImpactEstimate.MoneySaved.ValueUsd.Mean` per item.

### Time recovered

- **What it measures:** time saved by using community alternatives instead of
  acquiring, transporting, or doing things alone.
- **Who earns it:** every participant (same rule as Cost saved).
- **Source field:** `ImpactEstimate.TimeSaved.Minutes.Mean` per item.

### CO₂ avoided

- **What it measures:** carbon emissions avoided by sharing instead of
  manufacturing a new item.
- **Who earns it:** every participant (same rule as Cost saved).
- **Source fields:**
  `ImpactEstimate.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams.Mean` +
  `EmissionsPrevented.WasteReducedCarbon.Co2EGrams.Mean`, summed per item.

`assembleInboxView` additionally augments the `WeeklyMetrics` with pure
view-side fields after `computeWeeklyMetrics` returns: `WeekBackgroundMediaId`,
`ItemsSharedThisWeek` / `ItemsSharedBreakdown`, and `ActiveDays`, all computed
from the same week bounds over the user's community items.

---

## People row

Assembled as `GetPortfolioInboxViewResponse.people` by `buildCommunityPeople` in
[inbox_view.go](../../server/services/portfolio/inbox_view.go). It surfaces the
people the viewer is connected to — owners of all non-archived, non-deleted
community items (gear, experiences, requests) the user can see across their
communities, plus (for the #2568 directory recency signal) anyone who recently
sent a chat message in a visible conversation or RSVPed to a visible
experience — **excluding the authenticated user**.

**Ordering:** sorted by most recently shared descending. The "most recent
activity" timestamp per person is the maximum of
`CommunityGear.created_at_unix_sec`, `CommunityExperience.shared_at_unix_sec`,
`CommunityRequest.shared_at_unix_sec`, latest chat message sent time in a
visible conversation, and latest RSVP time to a visible experience
(`updateMax` in `buildCommunityPeople`). Each `DailyPerson` carries the user
ID, display name, avatar media ID, that last-activity time
(`LastActivityUnixSec`), and up to three shared-community display names
(`SharedCommunityNames`, from `sharedCommunityNamesByUser`) ordered by the
community's most recent activity — only communities the viewer belongs to are
ever named.

---

## Actions today

Concrete actions the user took today — things they did, not things that happened
to them — assembled by `assembleActionsToday` in
[actions.go](../../server/services/portfolio/actions.go). The list spans all the
user's communities and is omitted entirely when empty. Four categories
(verified in `actions.go`):

**Category A — Lent/Gave today:** transfers where the user is the owner and the
state moved to `RECIPIENT_SELECTED` or `ACTIVE` today (by `LatestRequestUnixSec`).

**Category A2 — Shared with community today:** new items the user listed or
posted to a community today — gear (`CommunityGear.created_at_unix_sec`),
experiences (`CommunityExperience.shared_at_unix_sec`), and requests
(`CommunityRequest.shared_at_unix_sec`). These `communityShareEntry` rows are
collected in `assembleInboxView` and passed in.

**Category B — Completed today:** loans/giveaways that reached `COMPLETED` today
(owner sees "Lent/Gave away", recipient sees "Returned/Received"); fulfilled help
requests archived today (requester sees "Help request fulfilled", confirmed
helper sees "Helped with"); experiences scheduled/started/completed today where
the user is host ("Hosted … today") or attendee ("Attended …").

**Category C — Engaged today:** expressed interest / requested to borrow (user is
the recipient of an `INTEREST_EXPRESSED` transfer timestamped today); offered to
help on an active request today (`RequestOffer`, not withdrawn, deduped against
confirmed helpers); confirmed helpers on active requests; RSVPs to upcoming
experiences.

Each `DailyAction` carries a title, action verb, item name, emoji, optional
`time_ago`, the impact values (cost saved, quality time, CO₂), and a
`content_id` + `item_type` so the client can navigate to the item.

---

## Inbox items: needs attention and coming up

`assembleInboxView` walks the pre-fetched active items and bins them into
`timed_items`, `anytime_items`, and `coming_up` groups. Per-item rows are built
by `assembleTransfer`, `assembleExperience`, and `assembleRequest` in
[assembly.go](../../server/services/portfolio/assembly.go).

**Transfers** → anytime. Included only if they have an unread count or a material
event today (`LatestRequestUnixSec` falls within today).

**Experiences** (user is host, has a YES/MAYBE RSVP, explicitly watched, or was
directly invited — a member of the experience's per-item/origin community — and
has not declined) → timed if scheduled within today's bounds, anytime if no/past
time (only when there are unreads), and coming-up if scheduled beyond today. This
inclusion set is `d.activeExpIDs`, computed in
[fetch.go](../../server/services/portfolio/fetch.go).

**Requests** (user is requester or confirmed helper) → anytime.

**Coming Up** additionally pulls in **all** non-terminal, non-deleted community
experiences scheduled beyond today (regardless of attendance), transfers with a
future estimated pickup (`EstimatedPickupUnixSec >= todayEnd`), and active loans
with a computable expected return date (`ActualPickupUnixSec + LoanDurationDays ×
86400 >= todayEnd`). Coming-up entries are grouped into **Tomorrow**, **Later
This Week**, and **Next Week** by `buildComingUpGroups`, each group sorted
ascending by time.

**Sorting:** timed items ascending by time; anytime items grouped by type via
`anytimeItemTypeOrder` (Requests, then Experiences, then Transfers) and within
each group by unread count descending.

---

## Unread count consistency

Every item with unread messages must appear somewhere in the response, and
`TotalUnreadCount` is the sum of all per-item unread counts (the nav-badge total
on the client). Terminal-state items (completed/cancelled transfers,
fulfilled/cancelled requests) are excluded from both the view and the unread
count using the shared `IsItemDone` in
[server/conversation/state.go](../../server/conversation/state.go) (and its
map-based variant `IsItemDoneFromMaps`).

---

## Key server files

| File | Purpose |
|------|---------|
| [server/services/portfolio/inbox_view.go](../../server/services/portfolio/inbox_view.go) | `GetPortfolioInboxView` handler, `assembleInboxView`, `buildCommunityPeople`, `buildComingUpGroups`, `fetchInboxViewExtras` |
| [server/services/portfolio/metrics.go](../../server/services/portfolio/metrics.go) | `computeWeeklyMetrics`, weekly breakdowns, `isoWeekBounds` |
| [server/services/portfolio/actions.go](../../server/services/portfolio/actions.go) | `assembleActionsToday` and `assembleSharedInventory` |
| [server/services/portfolio/assembly.go](../../server/services/portfolio/assembly.go) | Per-item assembly (`assembleTransfer` / `assembleExperience` / `assembleRequest`), `qualityTimeGoal` |
| [server/conversation/state.go](../../server/conversation/state.go) | Shared `IsItemDone` / `IsItemDoneFromMaps` for unread-count consistency |
