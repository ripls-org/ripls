# Gear Manage Menu — Checklist Redesign (Giveaway & Loan)

## Problem Statement

The giveaway and loan manage menus on `gear_content_view.dart` have the same flat-list problem described in [menu1.md](menu1.md): no progress feedback, no visual status icons, and no post-completion simplification.

### Current giveaway menu (top to bottom):
1. Select Recipient (`person_add_outlined`)
2. Mark as Picked Up (`check_circle_outline`) — disabled until recipient selected
3. Change Details (`edit_outlined`)
4. Close Item (`delete_outline`, destructive)

### Current loan menu — scheduled/active (top to bottom):
1. Arrange Pickup (`calendar_today_outlined`)
2. Mark Picked Up (`check_circle_outline`)
3. Mark Returned (`assignment_return_outlined`)
4. Change Details (`edit_outlined`)
5. Metrics (`bar_chart`) — scheduled only
6. Close Loan (`delete_outline`, destructive)

### Problems:
- **No checklist progress.** Same icons whether zero or all steps are done.
- **No recipient/borrower identity in the menu.** The owner has to remember who was selected or look elsewhere. For loans with multiple open requests, there's no way to see them from the menu.
- **Post-completion clutter.** After a giveaway is picked up or a loan is returned, the menu still shows all workflow items.
- **Inconsistent with experience/request menus.** Those now have checklist icons and post-completion View Impact. Gear menus should match.

---

## Proposed Menus

### Giveaway — pre-recipient-selection:

| # | Label | Checklist icon logic |
|---|-------|---------------------|
| 1 | **Set Details** | Green check when title + description + at least 1 media; default icon otherwise |
| 2 | **Confirm Location** | Green check if location set; red X if not (same as experience) |
| 3 | **Select Recipient** | Default icon (no recipient yet) |
| 4 | **Close Giveaway** | Destructive, unchanged |

### Giveaway — post-recipient-selection (pre-completion):

| # | Label | Checklist icon logic |
|---|-------|---------------------|
| 1 | **Set Details** | Same as above |
| 2 | **Confirm Location** | Same as above |
| 3 | **[Recipient name + avatar]** | Leading widget is the recipient's avatar (CircleAvatar); label is their display name |
| 4 | ↳ **Confirm Pickup Time** | Green check once `estimated_pickup_unix_sec` is set; default icon otherwise |
| 5 | ↳ **Confirm Received** | Green check once completed (giveaway state = COMPLETED); default icon otherwise |
| 6 | **Close Giveaway** | Destructive, unchanged |

Items 4-5 are visually nested under the recipient row (indented or grouped).

### Giveaway — post-completion:

| # | Label | Icon |
|---|-------|------|
| 1 | **[Recipient name + avatar]** | Recipient avatar |
| 2 | **View Impact** | Metrics icon (green), opens `ItemMetricsScreen` via `NavigationHelpers.pushWithSlide()` |

No Close Giveaway after completion (consistent with experience/request).

### Loan — pre-borrower-selection:

| # | Label | Checklist icon logic |
|---|-------|---------------------|
| 1 | **Set Details** | Same as above |
| 2 | **Confirm Location** | Same as above |
| 3 | **Select Borrower** | Default icon (no borrower yet) |
| 4 | **Close Lending** | Destructive, unchanged |

### Loan — with open requests (active borrowers):

| # | Label | Checklist icon logic |
|---|-------|---------------------|
| 1 | **Set Details** | Same as above |
| 2 | **Confirm Location** | Same as above |
| 3 | **[Borrower 1 name + avatar]** | Borrower's avatar; label is their display name |
| 4 | ↳ **Confirm Pickup** | Green check once `actual_pickup_unix_sec` is set; default icon otherwise |
| 5 | ↳ **Confirm Return** | Green check once `actual_return_unix_sec` is set; default icon otherwise |
| 6 | **[Borrower 2 name + avatar]** | (Repeat for each open request/active transfer) |
| 7 | ↳ **Confirm Pickup** | Same logic |
| 8 | ↳ **Confirm Return** | Same logic |
| 9 | **Close Lending** | Destructive, unchanged |

### Loan — most recent loan returned (no active transfers):

| # | Label | Icon |
|---|-------|------|
| 1 | **Set Details** | Same checklist logic |
| 2 | **Confirm Location** | Same checklist logic |
| 3 | **Select Borrower** | Default icon (ready for re-lending) |
| 4 | **View Impact** | Metrics icon (green), opens `ItemMetricsScreen` via `NavigationHelpers.pushWithSlide()` |
| 5 | **Close Lending** | Destructive |

Note: Unlike giveaways/experiences, loans return to the "ready for new borrower" state after return rather than collapsing to View Impact only, since gear can be re-lent.

---

## Potential Solutions

### Option A: Flat list with recipient rows as regular `ActionDropdownItem`s

Render recipient/borrower rows as standard `ActionDropdownItem`s with `leadingWidget` set to a `CircleAvatar`. Sub-items (Confirm Pickup, Confirm Return) are regular items with left padding to simulate nesting.

**Pros:** No changes to `ActionDropdownMenu` rendering. Uses existing `leadingWidget` field.
**Cons:** Visual nesting is faked with padding — no structural grouping. Hard to distinguish the recipient header from its sub-actions.

### Option B: Grouped sections in ActionDropdownMenu (recommended)

Add an optional `List<ActionDropdownItem>? children` field to `ActionDropdownItem`. When set, the item renders as a non-tappable header row (avatar + name) followed by its children with indented left padding. The menu renderer handles the grouping.

**Pros:** Clean structural hierarchy. Recipient header is visually distinct from sub-actions. The menu renderer can add subtle dividers or indentation automatically. Reusable for future grouped menus.
**Cons:** Requires a small change to `ActionDropdownMenu` rendering to handle children.

### Option C: Separate section widgets composed outside ActionDropdownMenu

Build the recipient sections as custom widgets and compose them with the dropdown items in a `Column`. The dropdown menu accepts a mix of `ActionDropdownItem`s and custom widgets.

**Pros:** Maximum flexibility.
**Cons:** Breaks the uniform `List<ActionDropdownItem>` contract. Makes the menu-building code in the content view more complex. Testing is harder.

---

## Recommendation

**Option B** — grouped sections via `children` on `ActionDropdownItem`. It keeps the menu contract uniform (`List<ActionDropdownItem>`), makes recipient/borrower grouping a first-class concept, and the rendering change is small. The `children` field is optional, so existing menus (experience, request) are unaffected.

---

## Open Questions (Resolved)

1. **Loan "Select Borrower" action.** **Resolved: Opens the existing transfer modal.** This already works today.
2. **Multiple concurrent loans.** **Resolved: Yes.** A gear can have multiple concurrent loans. The menu shows a borrower section for each open request/active transfer.
3. **Loan "all completed" state.** **Resolved: When the most recent loan is returned.** After return, the loan-specific items collapse and View Impact appears. The gear can be re-lent (new requests will re-expand the menu).
4. **Recipient avatar source.** **Resolved: `User.media_id` is available.** The API `User` message includes `media_id` (field 3) — the primary profile image. No extra fetch needed.
5. **Giveaway pickup vs completion.** **Resolved: Pickup = completion, but with two distinct steps.** "Confirm Pickup Time" is a scheduling step (separate from completion). "Confirm Received" is the final completion step. This replaces the original "Confirm Pickup" + "Mark Completed" proposal.
6. **Close Lending label.** **Resolved: Yes, rename to "Close Lending".** ARB key: `gearMenuCloseLending`.

---

## Phased Implementation Plan

### Phase 1: Add `children` support to `ActionDropdownItem` and menu renderer

**Files modified:**
- `app/lib/presentation/widgets/chat/action_dropdown_menu.dart`

**Changes:**
- Add `List<ActionDropdownItem>? children` field to `ActionDropdownItem`
- In the menu renderer: when `item.children != null`, render the item as a non-tappable header row (no tap highlight, slightly different text styling) followed by each child with 16px additional left padding
- Children inherit the same rendering logic (leading widget, status icons, destructive styling) as top-level items
- A subtle divider above grouped sections to separate them visually from regular items

**Tests:**
- Widget tests for `ActionDropdownMenu` with items that have children
- Verify header rows are not tappable
- Verify child items render with indentation

---

### Phase 2: Add shared checklist helpers for pickup/return/completion status

**Files modified:**
- `app/lib/presentation/widgets/content/checklist_status_icons.dart`

**Changes:**
- Add `buildPickupTimeStatusIcon({required BuildContext context, required bool hasPickupTime})` — green check when `estimated_pickup_unix_sec` is set, default icon otherwise
- Add `buildPickupStatusIcon({required BuildContext context, required bool isPickedUp})` — green check when `actual_pickup_unix_sec` is set, default icon otherwise
- Add `buildReturnStatusIcon({required BuildContext context, required bool isReturned})` — green check when `actual_return_unix_sec` is set, default icon otherwise
- Add `buildReceivedStatusIcon({required BuildContext context, required bool isCompleted})` — green check when giveaway state is COMPLETED, default icon otherwise
- Add `buildRecipientLeadingWidget({required BuildContext context, required String? avatarMediaId, required String displayName})` — returns a 36x36 `CircleAvatar` with the user's photo (using `CachedMediaImage` with thumbnail cache key from `User.media_id`) or initials fallback

**Tests:**
- Unit tests for each new helper verifying correct icon/color per state

---

### Phase 3: Giveaway menu checklist redesign

**Files modified:**
- `app/lib/presentation/screens/gear/gear_content_view.dart`
- `app/lib/l10n/app_en.arb`
- `app/lib/l10n/app_es.arb`

**Changes:**
- Rewrite `_buildGiveawayOwnerMenuItems` with three branches:
  1. **Post-completion:** Single recipient header + "View Impact" item (opens `ItemMetricsScreen` via `NavigationHelpers.pushWithSlide()`)
  2. **Post-recipient-selection:** Set Details, Confirm Location, recipient header with children (Confirm Pickup Time, Confirm Received), Close Giveaway
  3. **Pre-recipient-selection:** Set Details, Confirm Location, Select Recipient, Close Giveaway
- Recipient header: `leadingWidget` is avatar from `transferContext.userTransfer.recipient`, label is `recipient.name`, `children` contains the sub-actions
- Wire `leadingWidget` on Set Details and Confirm Location using existing `checklist_status_icons.dart` helpers
- Rename "Change Details" → "Set Details", "Close Item" → "Close Giveaway"
- Add ARB keys: `gearMenuSetDetails`, `gearMenuConfirmLocation`, `gearMenuSelectRecipient`, `gearMenuConfirmPickupTime`, `gearMenuConfirmReceived`, `gearMenuCloseGiveaway`, `gearMenuViewImpact` (both locales)

**Tests:**
- Widget tests for each menu branch (pre-selection, post-selection, post-completion)
- Verify recipient name and avatar appear in post-selection menu
- Verify children items render under recipient header

---

### Phase 4: Loan menu checklist redesign

**Files modified:**
- `app/lib/presentation/screens/gear/gear_content_view.dart`
- `app/lib/l10n/app_en.arb`
- `app/lib/l10n/app_es.arb`

**Changes:**
- Rewrite `_buildScheduledLoanOwnerMenuItems` and `_buildActiveLoanOwnerMenuItems` into a unified `_buildLoanOwnerMenuItems` with branches:
  1. **Has open requests/active transfers:** Set Details, Confirm Location, then for each open transfer: borrower header (`transferContext.pendingRequests` or active transfer recipient) with children (Confirm Pickup, Confirm Return), then Close Lending
  2. **No active transfers (returned or no requests yet):** Set Details, Confirm Location, Select Borrower, View Impact (if any past loans exist), Close Lending
- Each borrower header: `leadingWidget` is borrower avatar, label is borrower name, `children` has Confirm Pickup + Confirm Return with status icons based on `actual_pickup_unix_sec` and `actual_return_unix_sec`
- Rename "Change Details" → "Set Details", "Close Loan" → "Close Lending"
- Add ARB keys: `gearMenuSelectBorrower`, `gearMenuConfirmPickup`, `gearMenuConfirmReturn`, `gearMenuCloseLending` (both locales)

**Tests:**
- Widget tests for each menu branch (no requests, single borrower, multiple borrowers, all completed)
- Verify borrower names and avatars appear
- Verify pickup/return status icons update correctly

---

### Phase 5: Polish and edge cases

**Files modified:**
- `app/lib/presentation/screens/gear/gear_content_view.dart`
- `app/lib/presentation/widgets/chat/action_dropdown_menu.dart` (if needed)
- `app/lib/presentation/widgets/content/checklist_status_icons.dart` (if needed)

**Changes:**
- Handle edge cases:
  - Giveaway cancelled before recipient selected — keep current terminal behavior
  - Loan with no conversation yet — default status icons
  - Borrower menu items (`_buildBorrowerLoanMenuItems`) — no checklist changes needed (non-owner), but verify compatibility with new `children` field
  - Transfer with recipient but recipient user data is null (defensive guard)
- Verify status icons update reactively when state changes (e.g., owner confirms pickup from modal, menu should reflect new state on next open)
- Ensure avatar leading widgets size consistently at 36x36 to match status circle dimensions
- Accessibility: semantic labels on all status icons and avatar widgets

**Tests:**
- Edge case tests: cancelled giveaway, cancelled loan, null recipient
- Integration-style tests: open menu → confirm pickup → reopen menu → verify green check

---

## Data Availability

All required data is already present in `GearTransferContext` from the existing `GetGear` RPC response:

| Data needed | Source | Field |
|------------|--------|-------|
| Recipient name/avatar | `transferContext.userTransfer.recipient` | `User.name`, `User.media_id` (field 3) |
| Borrower name/avatar | `transferContext.pendingRequests[n].borrower` | `User.name`, `User.media_id` (field 3) |
| Pickup time scheduled | `transferContext.userTransfer.estimated_pickup_unix_sec` | Non-null = pickup time set |
| Pickup confirmed | `transferContext.userTransfer.actual_pickup_unix_sec` | Non-null = picked up |
| Return confirmed | `transferContext.userTransfer.actual_return_unix_sec` | Non-null = returned |
| Giveaway phase | `transferContext.giveawayPhase` | `RECIPIENT_SELECTED`, `COMPLETED`, etc. |
| Transfer state | `transferContext.userTransfer.state` | `RECIPIENT_SELECTED`, `ACTIVE`, `COMPLETED` |
| Gear details (title, desc, media) | `state.gearDetails` | `name`, `description`, `allMediaItems` |
| Location | `state.locationName` | Non-null = location set |

No new server RPCs or proto changes are needed.

---

**Last Updated:** 2026-03-28
