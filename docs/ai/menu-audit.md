# Menu Workflow Audit

Comparison of the client manage menu implementation (`docs/client/workflow.md`) against the authoritative workflow documents (`docs/workflows/*.md`).

**Audit Status:** Reviewed against `architecture.md`, `caching.md`, `i18n.md`, `server/architecture.md`. Fixes flagged with **[AUDIT]**.

**Files analyzed:**
- `app/lib/presentation/screens/experience/experience_content_view.dart`
- `app/lib/presentation/screens/request/request_content_view.dart`
- `app/lib/presentation/screens/gear/gear_content_view.dart`
- `app/lib/presentation/widgets/content/checklist_status_icons.dart`
- `app/lib/presentation/screens/experience/mark_completed_modal.dart`
- `app/lib/presentation/viewmodels/event_modal_view_model.dart`
- `server/services/transfer/interest.go`

---

## Problems Found

### P1: Cancelled state shows active menu (all four workflows)

**Severity:** High — owners can attempt actions on cancelled items

All four content views only check for the "completed/fulfilled" terminal state when building the owner menu. Cancelled items fall through to the active checklist branch, showing actions like "Mark Completed", "Select Recipient", "Close Event", etc. on items that cannot accept further state transitions.

| Workflow | Checks Completed? | Checks Cancelled? |
|---|---|---|
| Experience | `isCompleted` ✓ | ✗ — falls to active menu |
| Request | `isFulfilled` ✓ | ✗ — falls to active menu |
| Giveaway | `GIVEAWAY_PHASE_COMPLETED` ✓ | ✗ — falls to active menu |
| Loan | N/A (loans return to available) | N/A |

**Affected code:**
- `experience_content_view.dart:608-705` — `if (isCompleted) { ... } else { /* active menu */ }`
- `request_content_view.dart:469-533` — `if (isFulfilled) { ... } else { /* active menu */ }`
- `gear_content_view.dart:1070-1071` — only checks `GIVEAWAY_PHASE_COMPLETED`

**Note:** The `MorphingActionButton` receives `isTerminal: true` for cancelled states, which may prevent the dropdown from opening in practice. If so, this is a latent bug rather than a visible one — but still worth fixing for correctness and defensive coding.

**Decision:** Cancelled items show no menu at all — just a read-only "Cancelled" button with terminal styling.

### P2: Loan menu says "Select Borrower" but loans are auto-approved

**Severity:** Medium — misleading label, no functional bug

The loan workflow doc (`docs/workflows/loan.md`) confirms that `ExpressInterest` auto-approves loans directly to `RECIPIENT_SELECTED` (server code in `interest.go:119-123`). The owner never manually selects a borrower.

The client shows a "Select Borrower" menu item when `borrowerSections` is empty (no active transfers). This label implies manual selection that doesn't exist. The item opens the transfer modal, which is fine — but the label is confusing.

**Affected code:** `gear_content_view.dart:1314-1318`

**Decision:** Remove "Select Borrower" from the menu entirely. The loan owner menu should only show borrower name+avatar sections with sub-items under each (same as when borrowers exist). When no borrowers exist, the menu simply has Set Details, Confirm Location, and Close Lending — no selection prompt. The `docs/client/workflow.md` may be out of date on this point.

### P3: Loan "View Impact" shown unconditionally

**Severity:** Low — cosmetic issue

When no active loan transfers exist, the code always adds "View Impact" to the menu:
```dart
items.add(ActionDropdownItem(
  icon: Icons.insights,
  label: context.l10n.gearMenuViewImpact,
  ...
));
```

The comment says "Show View Impact if there have been past loans" but there's no conditional check. Brand-new gear that has never been loaned shows "View Impact" with no data.

The workflow doc (`docs/client/workflow.md` line 228-229) specifies this should only appear after at least one loan has completed.

**Affected code:** `gear_content_view.dart:1319-1331`

**[AUDIT]** The guard data must come from `GearState` (which wraps the proto response), not from a direct repository call. Per `architecture.md`: "Screens only interact with ViewModels" and "ViewModels are the ONLY layer that calls Repositories." `GetGearResponse` doesn't currently include `times_loaned` — this field exists only in `GetGearStatsResponse` (a separate RPC). Requires a proto + server change first.

**Decision:** Go with option (a) — add `optional int32 times_loaned` to `GetGearResponse` proto, populate server-side reusing the query from `GetGearStats`, then guard with `gear.hasTimesLoaned() && gear.timesLoaned > 0`.

### P4: Giveaway has "Confirm Pickup Time" step not in workflow doc

**Severity:** Low — feature present in client but undocumented

The giveaway workflow doc (`docs/workflows/giveaway.md`) describes a simple two-step flow after recipient selection: `RECIPIENT_SELECTED → COMPLETED`. No mention of pickup time scheduling.

The client adds a "Confirm Pickup Time" child item under the recipient header that checks `estimated_pickup_unix_sec`. The client workflow doc (`docs/client/workflow.md`) documents this feature.

This is not a bug — pickup time coordination makes UX sense for physical handoffs. But the workflow doc is incomplete.

**Affected code:** `gear_content_view.dart:1109-1120`

### P5: Non-owner menus incomplete and inconsistent

**Severity:** Medium — non-owner recipients/borrowers lack proper coordination menus

The client workflow doc states: "Non-owners do not see the checklist manage menu." However, `_buildBorrowerLoanMenuItems` (line 1205) already creates a 4-item dropdown for non-owner borrowers — proving non-owner menus are a needed pattern.

**Current state of non-owner menus:**

| Workflow | Non-owner role | Has menu? | What they see |
|---|---|---|---|
| Experience | Attendee | No | RSVP button → opens RsvpModal |
| Request | Helper | No | Offer button (disabled after offering) |
| Giveaway | Selected recipient | No | Button opens transfer modal |
| Loan | Borrower | Yes (4 items) | Arrange Pickup, Mark Picked Up, Mark Returned, Cancel Request |

**Decision:** Non-owner menus should be a formal pattern for loan recipients and selected giveaway recipients, since both need to coordinate physical handoffs. Experiences and requests don't need menus — they correctly use modals (RSVP modal and offer-to-help flow respectively). The loan and giveaway recipient menus should mirror the owner's checklist style (status icons, grouped items).

**Affected code:** `gear_content_view.dart:1205-1248`, `gear_content_view.dart:492-516`

### P6: Borrower loan menu items are not i18n'd

**Severity:** Low — hardcoded English strings

The borrower loan menu items at lines 1211-1244 use hardcoded strings ('Arrange Pickup', 'Mark Picked Up', 'Mark Returned', 'Cancel Request') instead of `context.l10n.*` keys. All other menu items across the four workflows use localized strings.

**Affected code:** `gear_content_view.dart:1211-1244`

---

## Confirmed Consistency (No Issues)

These areas were checked and found consistent between the workflow docs and client implementation:

- **Experience menu structure** — Set Details, RSVP, Confirm Location, Confirm Time, Mark Completed, Close Event matches workflow doc
- **Experience "Mark Completed" handles IN_PROCESS** — `event_modal_view_model.dart:582` calls `markInProcess()` before `completeExperience()`, correctly implementing the two-step state transition (ACTIVE/JOINED → IN_PROCESS → COMPLETED) behind a single menu action
- **Request menu structure** — Set Details, Confirm Location (optional, no red X), Mark Fulfilled, Close Request matches workflow doc
- **Giveaway post-completion menu** — Recipient header + View Impact only, matches workflow doc
- **Loan borrower sections** — Grouped header with Confirm Pickup + Confirm Return children matches workflow doc
- **Location `requiredLocation` flag** — Requests pass `requiredLocation: false`, all others use default `true`, matching the spec
- **Status icon states** — All icon builders in `checklist_status_icons.dart` match the documented states
- **Post-completion menu collapse** — All workflows correctly collapse to View Impact on completion

---

## Recommendations

| # | Problem | Recommendation |
|---|---------|----------------|
| P1 | Cancelled = active menu | Show read-only "Cancelled" button with terminal styling, no dropdown menu |
| P2 | "Select Borrower" for auto-approved loans | Remove from menu entirely; show only borrower sections when they exist |
| P3 | View Impact always shown for loans | Add `times_loaned` to `GetGearResponse` proto; guard with `> 0` |
| P4 | Giveaway pickup time undocumented | Update `docs/workflows/giveaway.md` to document the pickup time coordination step |
| P5 | Non-owner menus incomplete | Add checklist-style menus for loan borrowers and selected giveaway recipients |
| P6 | Hardcoded borrower menu strings | Add i18n keys for all non-owner menu labels |

---

## Resolved Questions

1. **What should cancelled items show?** No menu at all. Read-only "Cancelled" button with terminal styling.

2. **Should "Select Borrower" open the transfer modal?** No — this item should not exist on the menu. The owner menu shows borrower name+avatar with sub-items under each when borrowers exist. When no borrowers exist, the menu just has Set Details, Confirm Location, and Close Lending.

3. **Should non-owner menus be a formal pattern?** Yes. Loan borrowers and selected giveaway recipients get checklist-style menus mirroring the owner's format. Experiences don't need menus (RSVP modal handles it). Requests don't need menus (offer modal + ability to withdraw handles it).

4. **How to detect past loans?** Option (a): add `optional int32 times_loaned` to `GetGearResponse` proto, populate server-side.

---

## Phased Implementation Plan

### Phase 1: Fix cancelled state handling (P1)

**Files:** `experience_content_view.dart`, `request_content_view.dart`, `gear_content_view.dart`

For each content view, add a cancelled branch that returns no menu items and sets the button to terminal/"Cancelled" state. The `MorphingActionButton` already supports `ActionButtonStatus.terminal` — use that with a "Cancelled" label.

**Experience:**
```dart
if (isCompleted) {
  ownerMenuItems = [/* View Impact */];
} else if (expState == ExperienceState.EXPERIENCE_STATE_CANCELLED) {
  ownerMenuItems = const [];
  // Button shows "Cancelled" with terminal styling, no dropdown
} else {
  ownerMenuItems = [/* active checklist */];
}
```

**Request:** Same pattern with `isCancelled`.

**Giveaway:** Add `GIVEAWAY_PHASE_CANCELLED` check before the active branches.

**[AUDIT] Compliant.** This is a pure presentation-layer change. No ViewModel, Repository, or caching changes needed. Menu building is a screen concern per `architecture.md` rule 3.

**Estimated scope:** ~30 lines changed across 3 files. No new dependencies.

### Phase 2: Loan owner menu — remove "Select Borrower", guard View Impact (P2, P3)

**Files:**
- `proto/ripls/api/gear_service.proto` — add `optional int32 times_loaned` to `GetGearResponse`
- `server/services/gear/queries.go` (or equivalent) — populate `times_loaned` in `GetGear` handler
- `gear_content_view.dart` — remove "Select Borrower" item; guard View Impact
- `app/lib/l10n/app_en.arb`, `app/lib/l10n/app_es.arb` — remove or repurpose `gearMenuSelectBorrower`

**Changes to `_buildLoanOwnerMenuItems`:**
1. Remove the `Select Borrower` `ActionDropdownItem` entirely (the block at line 1314-1318).
2. When `borrowerSections` is empty, menu is just: Set Details, Confirm Location, Close Lending. No selection prompt.
3. Guard "View Impact" with `gear.hasTimesLoaned() && gear.timesLoaned > 0`.

**Proto/server prerequisite for P3:** Add `optional int32 times_loaned = N;` to `GetGearResponse` in `gear_service.proto`. Populate in server `GetGear` handler by reusing the query from `GetGearStats`. Run `npm run generate` after proto change.

**[AUDIT] Architecture constraint:** The guard uses data from `GearState` (which wraps `GetGearResponse`). Content views never call repositories directly per `architecture.md` rule 3.

**Estimated scope:** ~5 lines proto + ~10 lines server + ~15 lines content view + 2 ARB edits.

### Phase 3: Non-owner recipient/borrower menus (P5)

**Files:** `gear_content_view.dart`

Build proper checklist-style menus for non-owner loan borrowers and selected giveaway recipients, mirroring the owner's format with status icons.

**Loan borrower menu** (refactor existing `_buildBorrowerLoanMenuItems`):

| # | Item | Status Icon Logic |
|---|------|-------------------|
| 1 | **Arrange Pickup** | Green check when `estimated_pickup_unix_sec` is set; default icon otherwise |
| 2 | **Confirm Picked Up** | Green check when `actual_pickup_unix_sec` is set; default icon otherwise |
| 3 | **Confirm Returned** | Green check when `actual_return_unix_sec` is set; default icon otherwise |
| 4 | **Cancel Request** | Destructive |

**Giveaway selected-recipient menu** (new `_buildGiveawayRecipientMenuItems`):

| # | Item | Status Icon Logic |
|---|------|-------------------|
| 1 | **Arrange Pickup** | Green check when `estimated_pickup_unix_sec` is set; default icon otherwise |
| 2 | **Confirm Received** | Green check when giveaway is completed; default icon otherwise |
| 3 | **Withdraw Interest** | Destructive |

Wire both menus into the `MorphingActionButton` in `_buildTypeActions`, replacing the current empty menu for giveaway non-owners and enhancing the existing borrower menu.

**Key changes to `_buildTypeActions`:**
- Giveaway non-owner with `isSelectedRecipient == true`: build `_buildGiveawayRecipientMenuItems` and pass to `MorphingActionButton`
- Loan non-owner with `hasRequested == true`: refactored `_buildBorrowerLoanMenuItems` with status icons

**[AUDIT] Compliant.** Menu items use existing status icon builders from `checklist_status_icons.dart`. Actions route through ViewModel notifiers (e.g., `ref.read(gearProvider(...).notifier).arrangePickup()`). No direct repository access from the screen.

**Estimated scope:** ~60 lines for new giveaway recipient menu + ~30 lines refactoring existing borrower menu.

### Phase 4: i18n all non-owner menu strings (P6)

**Files:** `gear_content_view.dart`, `app/lib/l10n/app_en.arb`, `app/lib/l10n/app_es.arb`

Add i18n keys for all non-owner menu labels. Per `i18n.md`, each key needs:
- A value in `app_en.arb` with `@key` metadata
- A corresponding value in `app_es.arb`
- Keys follow `scopeContextDescription` naming

**New keys (loan borrower):**
- `gearBorrowerArrangePickup` — "Arrange Pickup"
- `gearBorrowerConfirmPickedUp` — "Confirm Picked Up"
- `gearBorrowerConfirmReturned` — "Confirm Returned"
- `gearBorrowerCancelRequest` — "Cancel Request"

**New keys (giveaway recipient):**
- `gearRecipientArrangePickup` — "Arrange Pickup"
- `gearRecipientConfirmReceived` — "Confirm Received"
- `gearRecipientWithdrawInterest` — "Withdraw Interest"

Run `npm run generate` after adding ARB keys, and verify with `npm run lint:dart:i18n`.

**[AUDIT] Compliant** with `i18n.md`. Viewmodels do not resolve these strings — the content view resolves them in the widget build method via `context.l10n.*`.

**Estimated scope:** ~15 lines changed in content view + 14 ARB entries (7 keys x 2 locales).

### Phase 5: Documentation updates (P4, P5, all changes)

**Files:** `docs/client/workflow.md`, `docs/workflows/giveaway.md`

Update both docs to reflect all changes made in Phases 1-4:

**`docs/client/workflow.md` updates:**
1. **Cancelled state behavior:** Add section describing that cancelled items show a read-only "Cancelled" button with no dropdown menu (all four workflows).
2. **Loan owner menu:** Remove "Select Borrower" from the pre-borrower-selection table. When no active transfers exist, the menu is: Set Details, Confirm Location, View Impact (if `times_loaned > 0`), Close Lending.
3. **Non-owner menus:** Rewrite the "Non-Owner Interactions" section to document:
   - Experience attendees: RSVP button → RsvpModal (no menu, unchanged)
   - Request helpers: Offer button → offer flow (no menu, unchanged)
   - Loan borrowers: Checklist-style dropdown with Arrange Pickup, Confirm Picked Up, Confirm Returned, Cancel Request (with status icons)
   - Giveaway selected recipients: Checklist-style dropdown with Arrange Pickup, Confirm Received, Withdraw Interest (with status icons)
4. **View Impact guard:** Note that loan View Impact only appears when `times_loaned > 0`.

**`docs/workflows/giveaway.md` updates:**
1. Add "Confirm Pickup Time" step to the post-recipient-selection section, documenting the `estimated_pickup_unix_sec` field.

**Estimated scope:** Documentation only, no code changes.

---

**Last Updated:** 2026-03-28
