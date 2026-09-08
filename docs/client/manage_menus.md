---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Owner-facing Manage menus for experiences, requests, and gear (giveaway/loan) — the shared flat-sheet primitive PollManageMenuSheet and its per-type wrappers, the legacy gear status-icon checklist (gear_menu_items, slated to migrate), and inline post-completion impact via ContentImpactRow.
  globs: [app/lib/presentation/screens/experience/widgets/experience_manage_menu_sheet.dart, app/lib/presentation/screens/request/widgets/request_manage_menu_sheet.dart, app/lib/presentation/screens/gear/widgets/gear_menu_items.dart]
  triggers: [manage-menu, manage-sheet, checklist, giveaway, loan, request, experience, view-impact]
  lens: [client, workflow]
  domain: client
freshness:
  verified_commit: "f06c2a95b"
  verified_on: "2026-06-18"
---
<!-- docs/client/manage_menus.md — owner-facing Manage menus for experiences, requests, and gear (giveaway/loan): the shared flat-sheet primitive, its per-type wrappers, the legacy gear status-icon checklist, and inline post-completion impact. -->
# Client Manage Menus

## Overview

The owner of a piece of coordinated content — a community **experience**, a
help **request**, or a piece of **gear** (shared for loan or giveaway) — drives
its lifecycle through an owner-facing **Manage menu**. The menu is a bottom
sheet of action rows opened from the content view.

There are **two** Manage-menu implementations in the codebase today:

| Content type | Implementation | Source |
|---|---|---|
| **Experience** | Flat sheet (`PollManageMenuSheet`) | [experience_manage_menu_sheet.dart](../../app/lib/presentation/screens/experience/widgets/experience_manage_menu_sheet.dart) |
| **Request** | Flat sheet (`PollManageMenuSheet`) | [request_manage_menu_sheet.dart](../../app/lib/presentation/screens/request/widgets/request_manage_menu_sheet.dart) |
| **Gear** — item-level | Flat sheet (`PollManageMenuSheet`) | [gear_manage_menu_sheet.dart](../../app/lib/presentation/screens/gear/widgets/gear_manage_menu_sheet.dart) |
| **Gear** — per-borrower workflow | Status-icon checklist (action-button dropdown) | [gear_menu_items.dart](../../app/lib/presentation/screens/gear/widgets/gear_menu_items.dart) |

Experience and request use the flat-sheet primitive for all owner actions. **Gear
(#2509) now uses a hybrid:** its *item-level* owner actions (Set details / Set
location / View impact / Log past / Close) live in the flat-sheet
`GearManageMenuSheet`, opened from the read shell's top-bar `···` overflow. Its
*per-borrower* workflow actions (confirm pickup time / pickup / return, select
recipient) — which a flat enum sheet can't express, because one gear item can
carry several concurrent loans — stay on the **status-icon checklist** dropdown
of the sticky action button, still built by `gear_menu_items.dart` (see
[Legacy gear status-icon checklist](#legacy-gear-status-icon-checklist) below).

---

## Flat-Sheet Primitive: `PollManageMenuSheet<T>`

The shared primitive lives at
[poll_manage_menu_sheet.dart](../../app/lib/presentation/widgets/poll/poll_manage_menu_sheet.dart).
It was first introduced for the location- and time-poll Manage menus (hence the
`Poll` prefix) and is now the single chrome for every flat Manage sheet.

`PollManageMenuSheet<T>` is a `GlassSheet` rendering an ordered list of
`PollManageMenuItem<T>` rows. Each row is:

- `icon` — leading `IconData` (size 18).
- `label` — already-localized title (the caller resolves `context.l10n`; the
  primitive stays poll-kind-agnostic so ARB strings never live inside it).
- `description` — optional already-localized one-line subtitle; when null the
  row collapses to label-only.
- `action` — the `T` enum value popped via `Navigator.pop(item.action)` when the
  row is tapped.
- `destructive` — renders the row in `AppColors.error` (red) for close/cancel
  actions.

The sheet itself does not know what the actions mean — the calling content view
reads the popped enum and dispatches. There are **no status icons** in the
flat-sheet variant; rows are a plain icon + label (+ optional description).

### Per-type wrappers

Both wrappers are thin `StatelessWidget`s that build a `PollManageMenuSheet`
parameterized over a per-type action enum.

**`ExperienceManageMenuSheet`** —
[experience_manage_menu_sheet.dart](../../app/lib/presentation/screens/experience/widgets/experience_manage_menu_sheet.dart).
Action enum `ExperienceManageAction { editDetails, markCompleted, closeEvent }`.
Rows:

| # | Row | Action |
|---|-----|--------|
| 1 | Update Details | `editDetails` |
| 2 | Mark Completed | `markCompleted` |
| 3 | Close Event (destructive) | `closeEvent` |

The sheet is opened from the **edit pencil** (`Icons.edit_outlined`) next to the
title in [experience_event_pane.dart](../../app/lib/presentation/screens/experience/widgets/experience_event_pane.dart),
shown only when `state.isOwner && !isTerminal` — a completed or cancelled
experience has **no Manage menu**. The popped action is dispatched in
`experience_event_pane.dart` (`_showManageSheet`); the close/cancel branch also
appears in [experience_close_handlers_mixin.dart](../../app/lib/presentation/screens/experience/widgets/experience_close_handlers_mixin.dart).
There is **no View Impact row** in the experience Manage sheet — post-completion
impact renders inline (see [Post-completion impact](#post-completion-impact)).

**`RequestManageMenuSheet`** —
[request_manage_menu_sheet.dart](../../app/lib/presentation/screens/request/widgets/request_manage_menu_sheet.dart).
Action enum
`RequestManageAction { editDetails, markFulfilled, closeRequest, viewImpact }`.
The sheet takes a `showViewImpact` flag (set when the request is in the
`REQUEST_STATE_FULFILLED` terminal state) that appends a fourth row:

| # | Row | Action | When |
|---|-----|--------|------|
| 1 | Edit Details | `editDetails` | always |
| 2 | Mark Fulfilled | `markFulfilled` | always |
| 3 | Close Request (destructive) | `closeRequest` | always |
| 4 | View Impact | `viewImpact` | `showViewImpact == true` (fulfilled) |

Dispatch lives in `request_content_view.dart` (`_showManageSheet`). The
`viewImpact` branch pushes
[ItemMetricsScreen](../../app/lib/presentation/screens/item/item_metrics_screen.dart)
(`ItemType.request`) via `NavigationHelpers.pushWithSlide`. This is the one
flat-sheet menu that still has an explicit View-Impact row; the request pane also
shows the inline impact row described below.

**`GearManageMenuSheet`** —
[gear_manage_menu_sheet.dart](../../app/lib/presentation/screens/gear/widgets/gear_manage_menu_sheet.dart).
Action enum
`GearManageAction { editDetails, setLocation, viewImpact, logPast, closeItem }`.
`GearManageMenuSheet.forState(state)` derives two flags from gear state: whether
the item is a giveaway (changes the "Log past" / "Close" copy and the
`closeItem` content type) and whether to include `viewImpact` (a completed
giveaway, or loan gear with `timesLoaned > 0`). Rows:

| # | Row | Action | When |
|---|-----|--------|------|
| 1 | Set details | `editDetails` | always |
| 2 | Set location | `setLocation` | always |
| 3 | View Impact | `viewImpact` | `timesLoaned > 0` |
| 4 | Log past loan / giveaway | `logPast` | always |
| 5 | Close Lending / Giveaway (destructive) | `closeItem` | always |

Dispatch lives in `gear_content_view.dart` (`_showManageSheet`): `editDetails`
toggles edit mode, `setLocation` opens the location picker, `viewImpact` opens
the equity screen (`showGearEquityModal`), `logPast` opens the past-transfer
modal, and `closeItem` opens `CloseItemModal` (unshare / cancel-transfer /
delete). Per-borrower confirm actions are **not** here — they live on the sticky
action button's checklist dropdown (below).

---

## Legacy gear status-icon checklist

Gear (giveaway and loan) still uses the **status-icon checklist** Manage menu,
built by `GearMenuItems` in
[gear_menu_items.dart](../../app/lib/presentation/screens/gear/widgets/gear_menu_items.dart).
Unlike the flat sheet, every checklist row carries a **leading status icon** — a
36×36 circle whose color/glyph reflect the step's completion state. These icons
come from the shared builders in
[checklist_status_icons.dart](../../app/lib/presentation/widgets/content/checklist_status_icons.dart)
(`buildDetailsStatusIcon`, `buildLocationStatusIcon`, `buildPickupTimeStatusIcon`,
`buildPickupStatusIcon`, `buildReturnStatusIcon`, `buildReceivedStatusIcon`,
`buildStatusCircle`). Rows are `ActionDropdownItem`s
([action_dropdown_menu.dart](../../app/lib/presentation/widgets/chat/action_dropdown_menu.dart)),
which support a `leadingWidget` (the status circle or a borrower/recipient
avatar) and a `children` list for grouping per-person actions under an identity
header.

> **Status (#2509):** the gear *item-level* actions have migrated to the
> flat-sheet `PollManageMenuSheet` pattern (`gear_manage_menu_sheet.dart`, opened
> from the read shell's top-bar overflow). The status-icon checklist below is now
> scoped to the **per-borrower workflow actions** on the sticky action button's
> dropdown — confirm pickup time / pickup / return and select-recipient, grouped
> per borrower. These resist a flat enum sheet because a single gear item can
> carry several concurrent loans, so the checklist (and
> `checklist_status_icons.dart`) stays the live surface for them.

`GearMenuItems` exposes two owner builders (plus non-owner recipient/borrower
builders) consumed by `gear_content_view.dart`:

### `buildGiveawayOwnerItems`

Branches on `GearTransferContext.overallPhase` / the owner's
`Transfer.state`:

- **Cancelled** (`GIVEAWAY_PHASE_CANCELLED`): empty list — no menu.
- **Completed** (`GIVEAWAY_PHASE_COMPLETED`): recipient header row + a single
  **View Impact** row (opens `onShowEquityModal`).
- **Recipient selected** (`TRANSFER_STATE_RECIPIENT_SELECTED`): Set Details,
  Confirm Location, a recipient header grouping `Confirm Pickup Time` +
  `Confirm Received` as `children`, then Close Giveaway.
- **Default (open / accepting interest):** Set Details, Confirm Location,
  Select Recipient (or "Waiting for Interest" when there are no pending
  requests), **Log Past Giveaway** (`pastTransferMenuGiveaway`, `Icons.history`,
  opens `onOpenPastTransferModal`), then Close Giveaway.

### `buildLoanOwnerItems`

A single gear item can carry **multiple concurrent loans**, so the loan menu is
built from sections rather than a fixed list:

1. Set Details
2. Confirm Location
3. Zero or more **borrower sections** — one per active/selected transfer
   (`buildBorrowerSection`: header avatar + `Confirm Pickup Time`, `Confirm
   Pickup`, `Confirm Return` children) and one per pending request
   (`buildUpcomingBorrowerSection`: header + Arrange Pickup only).
4. **View Impact** — appended **only when `Gear.timesLoaned > 0`** (opens
   `onShowEquityModal`); brand-new gear that has never been loaned shows no
   View Impact row.
5. **Log Past Loan** (`pastTransferMenuLoan`, `Icons.history`, opens
   `onOpenPastTransferModal`).
6. Close Lending (destructive).

The **"Log Past …" history row is inserted before Close** in both the giveaway
default menu and the loan menu — it opens the past-transfer logging modal so an
owner can record a giveaway/loan that happened offline.

### Gear state machines

```
Giveaway phase (GearTransferContext.overallPhase, server-computed):
OPEN --> RECIPIENT_SELECTED --> COMPLETED
  |             |
  +-------------+------------> CANCELLED

Transfer state (per loan/giveaway transfer):
INTEREST_EXPRESSED --> RECIPIENT_SELECTED --> ACTIVE --> COMPLETED
       |                       |                |
       +-----------------------+----------------+----> CANCELLED
```

Giveaway phase is **server-computed** (`GearTransferContext.overall_phase`)
because a non-selected participant's transfer stays `INTEREST_EXPRESSED` even
after a recipient is chosen — only the server can inspect every transfer to
decide the overall phase.

---

## Post-completion impact

When content reaches a terminal state, **impact is surfaced inline on the
content's first tab**, not on a dedicated metrics screen. The inline row is built
by `ContentImpactRow.build` in
[content_impact_row.dart](../../app/lib/presentation/widgets/content/content_impact_row.dart):
an icon-left row whose value column is three subtle, individually-tappable pills
(Money saved · Quality time · CO₂ avoided), each opening `ContentMetricSheet`
with the full breakdown. It returns `null` when no impact metric is present, so
callers spread it with the `?` collection element.

- **Experience:** `experience_event_pane.dart` builds the impact row when
  `isCompleted` (`state.experienceStats?.impact`). There is **no
  `ExperienceMetricsScreen`** — that screen no longer exists.
- **Request:** `request_request_pane.dart` builds the same inline row on a
  fulfilled request, in addition to the `viewImpact` Manage-menu row that opens
  `ItemMetricsScreen`.
- **Gear:** the giveaway-completed and loan menus expose a **View Impact** row
  that opens the equity modal (`onShowEquityModal`), gated on
  `Gear.timesLoaned > 0` for loans.

---

## Action Execution Pattern

All Manage-menu actions follow the same flow:

1. Owner opens the Manage sheet (edit pencil for experience/request; the
   content view's action button for gear).
2. The sheet pops an action enum (`PollManageMenuSheet`) or invokes an
   `ActionDropdownItem.onTap` callback (gear checklist).
3. The content view dispatches to the ViewModel (`experienceProvider`,
   `requestProvider`, `gearProvider`).
4. The ViewModel calls the repository, which executes the RPC and invalidates
   caches; the ViewModel then refreshes state from the server.
5. The content view re-renders — flat-sheet rows update which actions are
   offered; gear checklist status icons update on the next open.

Widgets never touch repositories directly — actions flow through the ViewModel
per the [client architecture](./architecture.md).

---

## Key Source Files

### Flat-sheet Manage menus
- [poll_manage_menu_sheet.dart](../../app/lib/presentation/widgets/poll/poll_manage_menu_sheet.dart) — the shared `PollManageMenuSheet<T>` / `PollManageMenuItem<T>` primitive
- [experience_manage_menu_sheet.dart](../../app/lib/presentation/screens/experience/widgets/experience_manage_menu_sheet.dart) — `ExperienceManageMenuSheet` + `ExperienceManageAction`
- [request_manage_menu_sheet.dart](../../app/lib/presentation/screens/request/widgets/request_manage_menu_sheet.dart) — `RequestManageMenuSheet` + `RequestManageAction`
- [gear_manage_menu_sheet.dart](../../app/lib/presentation/screens/gear/widgets/gear_manage_menu_sheet.dart) — `GearManageMenuSheet` + `GearManageAction` (item-level gear actions)

### Legacy gear checklist menu
- [gear_menu_items.dart](../../app/lib/presentation/screens/gear/widgets/gear_menu_items.dart) — `GearMenuItems` builders for giveaway and loan owners (and non-owner borrower/recipient menus)
- [checklist_status_icons.dart](../../app/lib/presentation/widgets/content/checklist_status_icons.dart) — shared status-icon builders
- [action_dropdown_menu.dart](../../app/lib/presentation/widgets/chat/action_dropdown_menu.dart) — `ActionDropdownItem` with `leadingWidget` + `children` support

### Inline impact
- [content_impact_row.dart](../../app/lib/presentation/widgets/content/content_impact_row.dart) — `ContentImpactRow` inline three-pill row
- [item_metrics_screen.dart](../../app/lib/presentation/screens/item/item_metrics_screen.dart) — `ItemMetricsScreen` (request View Impact + gear equity)

### Content views (dispatch sites)
- [experience_event_pane.dart](../../app/lib/presentation/screens/experience/widgets/experience_event_pane.dart) — opens the experience sheet, dispatches actions, builds the inline impact row
- [request_content_view.dart](../../app/lib/presentation/screens/request/request_content_view.dart) — opens the request sheet, dispatches actions
- [gear_content_view.dart](../../app/lib/presentation/screens/gear/gear_content_view.dart) — wires `GearMenuItems` into the gear action button

### Related Docs
- [content.md](./content.md) — content view architecture (tabs, edit mode, media, community context)
- [chat.md](./chat.md) — chat system architecture
- [architecture.md](./architecture.md) — overall MVVM client architecture
