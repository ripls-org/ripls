---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Client Needs & Contributions flow — collaborative who-brings-what / who-helps planning shared by Experiences and Requests; surfaces, lifecycle, gear-linking on claims, and the Experience/Request parity contract.
  globs: [app/lib/presentation/widgets/needs/**]
  triggers: [needs, contributions, claim, volunteer, slot, parity, gear-pill]
  lens: [client, domain]
  domain: client
freshness:
  verified_commit: "11d9d200b"
  verified_on: "2026-07-11"
---
# Needs & Contributions (Client)

The Needs & Contributions flow is the collaborative-planning layer
shared by Experiences and help Requests — "who's bringing what / who's
helping with what." It is conceptually adjacent to the experience-poll
flows but **not the same shape**: it is *N people each pick K of M
things to do/bring*, not *N people pick 1 of M options*. Multiple
participants claim the same Need concurrently up to its slot capacity,
freestanding Contributions exist alongside Need-derived ones, and there
is no single "winner" to lock in.

The two scopes (Experience and Request) share the same widgets, the
same lifecycle, and the same chrome. **New differences between the two
scopes should be treated as bugs** — parity is the design contract.
See [Intentional differences](#intentional-differences) for the narrow
set of divergences that are deliberate, and [Deferred work](#deferred-work)
for items intentionally left out of this round.

The data-layer story (slots, atomic claim, snapshot-on-claim, chat
integration) lives in [`docs/planning.md`](../planning.md). This file
covers the *client* lifecycle, surfaces, and parity contract.

Widgets live under [`app/lib/presentation/widgets/needs/`](../../app/lib/presentation/widgets/needs/).
Visual reference: [`docs/cowork/App Design/needs-redesign-v2.html`](../cowork/App%20Design/needs-redesign-v2.html).

### Gear linking on claims and contributions

A contribution may optionally link to **one** `Gear` item from the
contributor's library. The link surfaces on the row as an inline
[`GearPill`](../../app/lib/presentation/widgets/gear/gear_pill.dart);
tapping the pill opens the gear detail screen. When the linked gear
cannot be resolved (deleted or out of scope) the pill renders as a
dim "Removed item" tombstone with an explicit `Semantics` annotation.

The link is captured in the NK3 confirm view via a "Link an item"
disclosure that opens
[`NeedsClaimGearPicker`](../../app/lib/presentation/widgets/needs/needs_claim_gear_picker.dart):

- Name-match search reuses the @-mention search infrastructure
  (`searchRepository.search` with `SEARCH_STRATEGY_EXACT` and
  `SEARCH_ITEM_TYPE_GEAR`), intersected with the caller's owned gear
  to enforce owner-only links.
- Empty libraries render a dominant "Snap your gear" affordance that
  launches the unified create flow
  ([`UnifiedCreateModal`](../../app/lib/presentation/screens/create/unified_create_modal.dart) —
  the same modal wired to the home-screen `+` FAB, with its camera /
  text / hyperlink switcher intact); full libraries surface the same
  affordance as a secondary row below the list. The capture is
  save-only — it does **not** auto-open the Share sheet, since the user
  is picking an item, not sharing one. After a successful capture the
  picker diffs the refreshed library against a pre-open snapshot to find
  the new gear and auto-confirms with it linked.
- After successful capture, `gearRepository.refreshUserGear()`
  invalidates the `gear:user:list` cache and a "Saved to your
  library." toast confirms the side effect.

Parity contract: the same widget, picker, and capture flow serve
both Experience and Request scopes. The **escalation** of a gear-linked
claim into a real transfer now works in **both** scopes — a Request offer
to the requester (#2702) or an Experience contribution lent to the host
for the event's duration (#2708) — with narrow event-specific lifecycle
differences. See [Intentional differences](#intentional-differences) item 4.

---

## Lifecycle (v2)

A Needs list lives until the parent entity terminates. There is **no
"Mark ready" / lock-in step**:

```
  (no needs)
      │  organizer (or eligible participant) opens Propose sheet (NP1)
      │  and drafts items
      ▼
  ACTIVE  ── participants tap Volunteer-sheet rows to claim
      │  ── anyone can add a thing via the AddOptionGhost
      │  ── organizer opens Manage menu: Edit list, Nudge unclaimed, Cancel
      ▼
  TERMINAL (parent COMPLETED/CANCELLED or FULFILLED/CANCELLED)
      │
      ▼
  ARCHIVED (NA)  → read-only roster, single Save CTA, uncovered rows dim
```

The terminal state is inherited from the parent entity — there is no
separate `needs_locked` flag. v2 deliberately removed the v1 Mark Ready
affordance: the list is alive until the event itself happens, and the
same view simply becomes read-only afterward.

---

## Surfaces

Four glass-modal surfaces compose the flow. Every surface conforms to
the standard glass-modal shape — see [`docs/client/modals.md`](modals.md).

The planner-facing Propose sheet was removed once nothing opened it: adding
things now starts from the Picker.

| Surface | File | Role |
|---|---|---|
| Picker | [`needs_picker_modal.dart`](../../app/lib/presentation/widgets/needs/needs_picker_modal.dart) | Search + suggestion grid + slot stepper. **v2:** suggestion tiles dedup against the parent list (taken state). Plus an NK4 edit variant entered via `NeedsPickerModal.showEdit(...)` — same chrome with name/qty/note pre-filled and Remove + Save buttons. Mock IDs NK1–NK4 |
| Volunteer | [`needs_volunteer_sheet.dart`](../../app/lib/presentation/widgets/needs/needs_volunteer_sheet.dart) | Tap-to-claim list. Autosaves on each tap; no Save button. Uncovered rows sort first. **v2:** single unified list (no "Still needed" / "Who's bringing what" split), explicit Edit/Done toggle, [Edit/Done \| SaveIndicator] bottom row for members, [Manage \| SaveIndicator] for organizers, AddOptionGhost ghost row at the bottom of the list. Mock IDs NV1 / NV2 / NV2-edit / NO1 |
| Manage | [`needs_manage_menu_sheet.dart`](../../app/lib/presentation/widgets/needs/needs_manage_menu_sheet.dart) | Organizer manage menu: **v2:** Edit list + Nudge unclaimed + Cancel. Wraps the generic [`PollManageMenuSheet<T>`](../../app/lib/presentation/widgets/poll/poll_manage_menu_sheet.dart). Mock ID NO2 |
| Archived | [`needs_archived_sheet.dart`](../../app/lib/presentation/widgets/needs/needs_archived_sheet.dart) | Read-only post-event roster. Same body shape as the Volunteer sheet; uncovered rows dim to 0.55; single Save CTA. Mock ID NA |

Shared primitives the surfaces compose:

| Primitive | Purpose |
|---|---|
| [`needs_sheet_chrome.dart`](../../app/lib/presentation/widgets/needs/needs_sheet_chrome.dart) | `GlassSheet` + keyboard-aware padding |
| [`needs_claim_row.dart`](../../app/lib/presentation/widgets/needs/needs_claim_row.dart) | Tap-to-claim row built on `Toggle`. **v2:** carries note + "added by {name}" + count line "{X} of {Y} claimed ✓" for multi-slot Needs + inline multi-qty stepper ("BRINGING – \| N \| + of N needed") + edit-mode pencil affordance + voter stack |
| [`needs_suggestion_grid.dart`](../../app/lib/presentation/widgets/needs/needs_suggestion_grid.dart) | Category-tinted suggestion grid for the Picker empty state |
| [`needs_volunteer_atoms.dart`](../../app/lib/presentation/widgets/needs/needs_volunteer_atoms.dart) | Foundation atoms: `NeedsClaimCounter`, `NeedsSaveIndicator`, `NeedsAddOptionGhost`, `NeedsEditDoneButton`, `NeedsEditModeBanner`, `NeedsManagePillButton` |

### Routing

The Plan-tab card tap dispatches state-aware from
[`needs_actions.dart`](../../app/lib/presentation/widgets/needs/needs_actions.dart)
via `NeedsActions.openPlanTabDispatcher`:

| Parent state | Sub-modal |
|---|---|
| Active | Volunteer sheet (NV1 / NV2 / NO1) |
| Terminal | Archived sheet (NA) |

The Volunteer sheet's trailing manage trigger is rendered only for the
organizer (Experience owner / Request requester). The Picker is opened
from the Propose sheet's "Add a thing" row, from the Volunteer-sheet
AddOptionGhost, and (NK4 edit variant) from an edit-mode row tap.

---

## Scope dispatch

The two scopes share every widget. Scope-specific behaviour is funnelled
through the [`NeedsScope`](../../app/lib/presentation/viewmodels/needs_scope.dart)
sealed class — passing one to [`NeedsActions`](../../app/lib/presentation/widgets/needs/needs_actions.dart)
is all that's needed to route the correct provider, permission checks,
and chrome variant.

| Concern | Experience scope | Request scope |
|---|---|---|
| Provider | `experienceNeedsProvider(experienceId)` | `requestNeedsProvider(requestId)` |
| Batch-add type | `BatchNeedItem` | `RequestBatchNeedItem` |
| Update RPC | `UpdateExperienceNeed` | `UpdateRequestNeed` |
| Nudge RPC | `NudgeUncoveredNeedClaimers` (targets YES/MAYBE RSVPs) | `NudgeUncoveredRequestNeedClaimers` (targets existing RequestOffer members) |
| Auto-RSVP on first claim | Yes — YES intention is submitted before the claim if the user isn't already RSVPed | No — every community member can claim without pre-membership ceremony |
| Manage menu trigger eligibility | Experience owner | Request requester |
| Propose entry eligibility | Owner or YES/MAYBE RSVP | Requester only (any member may offer freestanding) |
| Edit-mode `canEdit` (member) | Organizer always; member if `n.proposer.id == currentUserId` or member has a contribution on the Need | Same rule, organizer = requester |

---

## Edit mode

v2 introduces an explicit edit-mode flip on the live reply screens to
avoid the v1 ambiguity where row-tap could mean "claim" or "edit".

- **Trigger**: member — tap the Edit chip in the bottom-left.
  Organizer — open the Manage menu, tap "Edit list".
- **Visual**: sage `NeedsEditModeBanner` above the list; editable rows
  pick up a sage outline + pencil glyph and become tappable to fire
  `NeedsActions._openEditRow` → opens NK4 picker pre-populated; locked
  rows dim to 0.45 opacity. The bottom-left toggle flips to sage
  "Done".
- **Permissions**: organizer can edit any row; member can edit only
  rows they proposed (Needs) or claimed (contributions). The Volunteer
  sheet's `NeedsVolunteerEntry.canEdit` is computed at the call site.
- **AddOptionGhost** stays visible in edit mode — adding a new thing
  shouldn't require exiting/re-entering edit mode.

NP2 (Propose) is implicitly always in edit mode — pre-launch there's
no competing claim gesture, so drafted rows render with the sage
pencil affordance from the start and skip the Edit/Done toggle
entirely.

---

## Intentional differences

Parity is the design contract; everything in this section is a
deliberate divergence that follows from the underlying concept being
modelled. Anything *not* in this list that the two scopes do
differently should be treated as drift.

1. **Auto-RSVP on first claim (Experience scope only).** When an
   Experience participant taps to claim a Need without an existing
   RSVP, the client submits a YES RSVP first and only then the claim.
   Request scope has no membership ceremony.
2. **Nudge candidate set differs by scope.** Experience nudge targets
   every Yes/Maybe RSVP who has zero contributions. Request nudge
   targets every existing `RequestOffer` member who has zero
   contributions — narrower, to avoid pinging the wider community.
   Same UI affordance, different server-side fan-out.
3. **Manage trigger eligibility.** The Volunteer sheet's manage icon
   is rendered for the Experience owner and for the Request requester
   only. Both flows share the same `NeedsManageMenuSheet`; the
   eligibility check lives in the calling site, never inside the
   shared sheet.
4. **Gear-backed offer escalation — both scopes, with event-specific
   lifecycle (#2702 requests, #2708 experiences).** On both scopes the
   claim sheet surfaces a Lend/Give choice (`NeedsLendGiveChoice`) when
   gear is linked, and confirming escalates the claim into a real
   loan/giveaway with an undoable "offer sent" toast. The escalation
   surfaces from the `openClaimSheet` path only (the pitching-in roster /
   RSVP composer for events; the helpers panel / contribution composer for
   requests) — never the Plan-tab Volunteer sheet, which stays plain
   tap-to-claim in both scopes.
   - **Request** (`TransferRepository.offerTransfer`): recipient = the
     requester; the requester can optionally accept ("this one works"); the
     handoff auto-fulfills a single-need request.
   - **Experience** (`TransferRepository.offerExperienceTransfer`):
     recipient = the **host** (derived server-side, so a self-loan by the
     host is suppressed — the Lend/Give choice is hidden when
     `currentUserId == ownerId`); the transfer completes when the **event
     completes** and cancels when it is cancelled (no manual handoff, no
     return reminder); the live-offer guard reads the contribution's own
     `transferState`/`transferType` (added on `ExperienceContributionResponse`,
     #2708) rather than a separate `gearOffers` array. There is **no host
     "accept"** in v1, so event rows render a plain gear link today; a
     Lending/Giving row tag is a deferred polish follow-up.

---

## Deferred work

Items intentionally left out of this round. Each has (or needs) a
tracking issue before any inline TODO is added.

1. **DeadlineStrip** + `needs_claims_deadline_unix_sec` on Experience
   and Request, plus a `SetNeedsClaimsDeadline` RPC. v2 mocks show
   the strip on NP2 and the reply views; we ship without it for now.
2. **Multi-slot claim batching.** Today each `+`/`–` tap on the
   inline multi-qty stepper is its own `ClaimExperienceNeed` /
   `UnclaimExperienceNeed` RPC. If contention becomes a problem add
   an optional `quantity` field to the claim request messages.
3. **Friend-graph hint in NK3.** The mock shows a violet one-liner
   ("Marcus has a 60m rope in his kit — flag him?") when a community
   member already has the item listed in their kit. Picker reserves
   the slot; content comes later.
4. **Archived "Save" persistence.** NA's Save CTA currently fires a
   toast and dismisses the sheet. A future feature lets the user keep
   a personal copy of the list outside the parent entity.
5. **Saved kits (Picker rail).** Removed in v2 simplification — a
   power feature that didn't earn its slot in MVP. May return as a
   follow-up.
6. **NK2 autocomplete LLM extraction.** v2 picker autocomplete is a
   local-only filter for now; LLM-backed extraction (with the sage
   "Add '{query}' as a new thing" tail row) is a follow-up.
7. **AddOptionGhost path for non-requesters on Request scope.** The
   v2 dashed row implies any member can add to the shared list. On
   Request scope server-side that means `AddRequestNeed` would need
   to open up (currently requester-only). Needs a product call.

---

## Open questions

- **Cancel-needs bulk RPC.** `NeedsActions._cancelAllNeeds` loops
  `removeNeed` one-by-one because there's no bulk-cancel RPC. Fine
  for the typical list size (< 20); track separately if list sizes
  grow.
- **Edit-mode locked rows on the organizer side.** Today the
  organizer's `canEdit` is `true` on every row. If we add member-level
  contributions the organizer can't legitimately edit, the rule will
  need to change.

---

## Server foundations (v2 additions)

| RPC | Where | Notes |
|---|---|---|
| `UpdateExperienceNeed` / `UpdateRequestNeed` | [`server/services/experience/needs.go`](../../server/services/experience/needs.go) + [`server/services/request/needs.go`](../../server/services/request/needs.go) | Proposer only. Reducing `slots` below the current claim count fails with `FailedPrecondition`. Emits `CHAT_SYSTEM_ACTION_PLANNING_NEED_UPDATED` |
| `NudgeUncoveredNeedClaimers` / `NudgeUncoveredRequestNeedClaimers` | [`server/services/experience/nudge_needs.go`](../../server/services/experience/nudge_needs.go) + [`server/services/request/nudge_needs.go`](../../server/services/request/nudge_needs.go) | Organizer / requester only. Mirrors `NudgeTimePollVoters` shape — same active-community gate, same per-user fan-out via the notification service |
| `CHAT_SYSTEM_ACTION_PLANNING_NEED_UPDATED` | [`proto/ripls/models/chat.proto`](../../proto/ripls/models/chat.proto) | Coalesces into the existing `exp_need` family alongside add / remove |
| `COMMUNITY_EVENT_TYPE_PLANNING_NEED_UPDATED` | [`proto/ripls/models/community.proto`](../../proto/ripls/models/community.proto) | Classified `ClassificationClientOnly` in `server/undo/registry.go` (inverse is another `UpdateNeed`) |

---

## Adding a new surface

When a new Needs surface ships:

1. Compose `GlassSheet` directly (or via `needs_sheet_chrome.dart`)
   so keyboard inset and padding stay consistent across sheets.
2. Use `Toggle` for any selectable row state (not `Tappable` —
   `Toggle` provides the screen-reader `selected` flag).
3. Open via `showAccessibleModal`, never `showModalBottomSheet`
   directly.
4. Every new user-visible string lands in both `app_en.arb` and
   `app_es.arb` in the same change; `npm run lint:dart:l10n-parity`
   enforces this.
5. Add an entry to [Intentional differences](#intentional-differences)
   for anything the surface genuinely needs to do differently across
   scopes. If nothing fits, the surface shouldn't diverge.

---

## Appendix: per-scope copy

Today every user-facing string on the Needs/Contributions sheets is
shared verbatim across both scopes. That made the v2 build easy, but it
flattens out the conceptual difference between the two surfaces: an
**Experience** is "we're doing this thing together — who can chip in?",
while a **Request** is "I need help with this — let's break it down,
and here's what I'm already handling myself." This appendix proposes a
scope-specific copy table. **Nothing in the table is implemented yet** —
the strings live as single shared ARB keys; once we converge on the
copy below, each row turns into either (a) a per-scope ARB key pair or
(b) a runtime branch in the widget that picks the right key from
`NeedsScope`.

Conventions used below:
- *Same* in the Request column means the existing string works for both
  scopes; no change needed.
- `{name}`, `{count}`, `{claimed}`, etc. are ICU placeholders that
  already exist on the current key — preserved verbatim.
- A few rows split into multiple cells (e.g. Propose has *Empty / One /
  Many* headline variants). Each row covers one ARB key.

### Propose sheet (NP1 — bulk-draft + one-at-a-time)

Opened by the organizer/requester to seed a new list of things.

| ARB key | Experience copy | Request copy |
|---|---|---|
| `needsProposeTitle` | What does the group need? | What can the group help with? |
| `needsProposeKicker` | Plan the event | Break down the request |
| `needsProposeHeadlineEmpty` | What do we need? | What would help? |
| `needsProposeHeadlineOneV2` | One thing to ask about. | One piece to share. |
| `needsProposeHeadlineMany` | That's the list. | That's the breakdown. |
| `needsProposeSubtitleEmpty` | Add things people can bring or jobs they can take on. | Break the request into pieces. Add what you're already handling, too. |
| `needsProposeSubtitleReady` | Tap any row to change it, or "Ask the group" when you're ready. | Tap any row to change it, or "Ask for help" when you're ready. |
| `needsProposeAddItem` | Add a thing | Add a piece |
| `needsProposeAddAnother` | Add another thing | Add another piece |
| `needsProposeAddBigHint` | Pick from the grid, or type something to bring or a task to take on. | Pick from the grid, or type a piece — to lend, do, or cover. |
| `needsProposeEmptyCta` | Add at least one thing | Add at least one piece |
| `needsProposeBulkOrDivider` | or add a few things | or break it down quickly |
| `needsProposeBulkPlaceholder` | Folding chairs, drinks for 8, set up tables, take photos, drive carpool… | Pickup truck, moving blankets, two hours of loading, watch the kids… |
| `needsProposeBulkAddCta` | Add items | Add pieces |
| `needsProposeBulkHint` | We'll split on commas, semicolons, or new lines. | *Same* |
| `needsProposeAskTheGroup` | Ask the group | Ask for help |

### Picker (NK1–NK4 — single-item add / edit)

Opened from the Propose sheet's "Add a thing" row, the Volunteer
sheet's AddOptionGhost, and edit-mode rows.

| ARB key | Experience copy | Request copy |
|---|---|---|
| `needsPickerTitle` | Add a thing | Add a piece |
| `needsPickerHeadline` | What are we adding? | What's the piece? |
| `needsPickerSubtitle` | Pick a suggestion or type a new thing. | Pick a suggestion or type a new piece. |
| `needsPickerSearchHint` | Search or type a new thing… | Search or type a piece… |
| `needsPickerSuggestionsHeader` | Suggestions | Suggestions for this ask |
| `needsPickerSuggestionsForCategory` | Suggested for {category} | *Same* |
| `needsPickerTakenSubtitle` | on the list | already on the breakdown |
| `needsPickerSuggestionQuantityHint` | needs ~{count} | ~{count} helpful |
| `needsPickerCustomAddRow` | Add "{query}" as a new thing | Add "{query}" as a piece |
| `needsPickerBackToPick` | Pick another | Pick another |
| `needsPickerAddToList` | Add to list | Add to breakdown |
| `needsPickerEditEyebrow` | Edit a thing | Edit a piece |
| `needsPickerEditRemove` | Remove | *Same* |
| `needsPickerEditSave` | Save | *Same* |
| `needsPickerEditAlreadyClaimed` | Already claimed | Already taken |
| `experienceNeedsNoteLabel` | Note (optional) | *Same* |
| `experienceNeedsSlotsLabel` | How many can chip in? | How many helpers? |

### Volunteer sheet (NV1 / NV2 / NO1 — live tap-to-claim)

The active screen: members claim slots, organizers manage the list.

| ARB key | Experience copy | Request copy |
|---|---|---|
| `needsVolunteerTitle` | Pick what you can bring or do | Pick a piece you can take on |
| `needsVolunteerEyebrow` | What can you chip in? | How can you help? |
| `needsVolunteerHeadlineOrganizer` | Who's chipping in? | Who's picking up which piece? |
| `needsVolunteerHeadlineHelper` | Pick anything you can bring or do. | Pick a piece you can cover. |
| `needsVolunteerSubtitleOrganizer` | You've laid out what's needed. Claim a row, or open Manage to edit the list. | Your ask plus what you're handling. Claim a row, or open Manage to edit. |
| `needsVolunteerSubtitleHelper` | Tap a row to chip in. Tap again to release. | Tap a row to take a piece. Tap again to release it. |
| `needsVolunteerHeaderYourein` | You're in. | You're on it. |
| `needsVolunteerEyebrowYourein` | You're in | You're on it |
| `needsVolunteerSubtitleYourein` | Tap to undo, or claim more. We save as you go. | Tap to undo, or pick up more pieces. We save as you go. |
| `needsVolunteerCoverage` | {claimed} of {total} claimed | {claimed} of {total} picked up |
| `needsVolunteerCoverageIncludingYou` | · INCLUDING YOU | *Same* |
| `needsVolunteerCoverageEyebrow` | Coverage | Coverage |
| `needsVolunteerCoverageThingsCovered` | {covered} of {total} things covered | {covered} of {total} pieces covered |
| `needsVolunteerCoveragePeopleChippedIn` | {count, plural, =0{No-one's chipped in yet} =1{1 person chipped in} other{{count} people chipped in}} | {count, plural, =0{No-one's helping yet} =1{1 person is helping} other{{count} people are helping}} |
| `needsVolunteerRowClaimed` | {claimed} of {total} claimed | {claimed} of {total} picked up |
| `needsVolunteerManageButton` | Manage | *Same* |
| `needsVolunteerEmpty` | Nothing requested yet. | No pieces yet. |
| `needsVolunteerStillNeededHeader` | Still needs a bringer | Still needs a helper |
| `needsVolunteerCoveredHeader` | Covered | Covered |
| `needsVolunteerSavedIdle` | Tap any item to claim — saves as you go | Tap any piece to take it on — saves as you go |
| `needsVolunteerSavedDone` | Saved · {claimed} of {total} covered | Saved · {claimed} of {total} picked up |
| `needsAddOptionGhost` | Add something we're missing | Add a piece we're missing |

### Manage menu (NO2 — organizer actions)

Same three rows for both scopes, but the copy reframes "the list" vs.
"the breakdown."

| ARB key | Experience copy | Request copy |
|---|---|---|
| `needsManageEditList` | Edit list | Edit the breakdown |
| `needsManageEditListDescription` | Change item names, quantities, or notes. | Change pieces, quantities, or notes. |
| `needsManageNudgeUnclaimed` | Nudge unclaimed | Nudge for help |
| `needsManageNudgeUnclaimedDescription` | Ping anyone who hasn't claimed yet. | Ping anyone offering help who hasn't picked up a piece yet. |
| `needsManageCancel` | Cancel needs | Cancel the breakdown |
| `needsManageCancelDescription` | Remove every open request from this list. | Remove every open piece from the breakdown. |

### Edit-mode atoms

| ARB key | Experience copy | Request copy |
|---|---|---|
| `needsEditButton` | Edit | *Same* |
| `needsDoneButton` | Done | *Same* |
| `needsEditBannerOrganizer` | Editing — tap any item to change it. | Editing — tap any piece to change it. |
| `needsEditBannerMember` | You can edit things you added or claimed. Others are read-only. | You can edit pieces you added or took on. Others are read-only. |

### Claim row (per-row chrome)

| ARB key | Experience copy | Request copy |
|---|---|---|
| `needsRowAddedBy` | added by {name} | *Same* |
| `needsRowBringingLabel` | AMOUNT | *Same* |
| `needsRowCountLine` | {claimed} of {needed} claimed | {claimed} of {needed} picked up |

### Archived sheet (NA — post-terminal read-only roster)

| ARB key | Experience copy | Request copy |
|---|---|---|
| `needsArchivedEyebrow` | Event ended | Request wrapped up |
| `needsArchivedTitle` | Here's how it went. | Here's who chipped in. |
| `needsArchivedSave` | Save | *Same* |
| `needsArchivedSavedToast` | Saved to your records. | *Same* |

### Scope-neutral / unchanged

These strings work as-is for both scopes and don't need a per-scope
variant:

- `needsPickerSearchHint` (when the picker terminology stays the same;
  see Request column above if we tighten "piece")
- `needsPickerCustomAddRow` (the surrounding sheet already establishes
  the noun)
- Everything under "Edit-mode atoms" except the banner messages
- `needsArchivedSave`, `needsArchivedSavedToast`

### What's deliberately *not* in this table

- **AddOption-ghost on Request scope**: still gated behind the deferred
  "any member can add to a Request breakdown" decision (Deferred work
  item 7). When that ships, the Request copy for `needsAddOptionGhost`
  may want to switch from "Add a piece we're missing" to "Suggest a
  piece" to make the broader audience explicit.
- **Empty Needs row on the parent screen** (`needsEventRowTbd`,
  `needsEventRowHelpCta`, `needsEventRowOnlyNeeded`,
  `needsEventRowProvidedAndNeeded`, `needsEventRowNothingNeeded`,
  `needsEventRowHelpedCta`): these live on the content view rows, not
  the modals. If the modal copy lands first they're a natural follow-up
  using the same Experience/Request framing.
- **Manage menu eligibility copy**: the menu is only shown to the
  organizer (Experience owner / Request requester), so we don't need a
  separate "non-organizer" copy column. If we add a member-visible
  variant later, that becomes a third column.

### Implementation notes (for the follow-up)

Once the table is approved, the cleanest split is:

1. Rename every shared key in the table above from `needs*` to
   `needsExp*` and add a parallel `needsReq*` key. The l10n-parity lint
   guarantees both locales stay in sync.
2. Plumb a `NeedsScope`-aware helper into the sheets that resolves to
   the right key at `build()` time (e.g., `_l10nForScope(context, scope,
   exp: l10n.needsExpProposeTitle, req: l10n.needsReqProposeTitle)`).
   Sheets already receive scope via `NeedsActions` — no new plumbing.
3. Rows marked *Same* keep a single shared key; only divergent rows
   pay the duplication cost.

---

## See also

- [`docs/issues/2148-needs-modal-redesign.md`](../issues/2148-needs-modal-redesign.md) — the original v1 implementation plan that produced the surface set.
- [`docs/cowork/App Design/needs-redesign-v2.html`](../cowork/App%20Design/needs-redesign-v2.html) — the v2 visual reference.
- [`docs/planning.md`](../planning.md) — the data-layer story (slots, atomic claim, snapshot-on-claim, chat integration).
- [`docs/client/polls.md`](polls.md) — the parallel parity contract for experience-poll surfaces.
- [`docs/client/modals.md`](modals.md) — the standard glass-modal shape every surface above conforms to.
