# Needs & Contributions widgets

Shared client-side widgets for the collaborative-planning layer used by
both Experiences and help Requests. The same surfaces power both scopes;
scope-specific behavior is funnelled through `NeedsScope` (see
[`needs_scope.dart`](../../viewmodels/needs_scope.dart)).

The lifecycle parallels the experience-poll lifecycle (Propose / Vote /
Manage / Confirm / Finalized) — see [`docs/client/needs.md`](../../../../docs/client/needs.md)
for the surfaces table and the parity contract.

## Files

| File | Purpose |
|------|---------|
| `needs_actions.dart` | Shared handler routing Plan-tab and chat-pill taps to the right sheet; `openClaimSheet` opens the name-keyed `NeedsClaimConfirmSheet` ("Claim a need" / "Edit your contribution") for any item |
| `need_claim_chip.dart` | `NeedClaimChip`: shared `GlassChip` pill (RSVP composer + pitching-in roster) whose tap opens the claim/edit sheet via `NeedsActions.openClaimSheet` |
| `needs_sheet_chrome.dart` | `GlassSheet` + `ModalHeader` + `GlassFooterButtons` composition used by every needs sheet |
| `needs_claim_row.dart` | Tap-to-claim row with idle / claimed visual states; built on `Toggle` |
| `needs_suggestion_grid.dart` | Category-tinted suggestion grid (gear / food / help / personal) |
| `needs_picker_modal.dart` | Picker surface: search, suggestion grid, slot stepper |
| `needs_volunteer_sheet.dart` | Volunteer surface: tap-to-claim list, autosaves |
| `needs_archived_sheet.dart` | Read-only post-event roster (NA): same shape as the Volunteer sheet, dimmed uncovered rows, single Save CTA |
| `needs_volunteer_atoms.dart` | Foundation atoms (claim counter, save indicator, edit/done button, edit-mode banner, add-option ghost) |
| `needs_manage_menu.dart` | Organizer manage menu (Add a thing, Mark ready, Cancel needs) |

## Parity contract

The two scopes (Experience and Request) share the same widgets and the
same lifecycle. New differences should be treated as bugs unless they
are listed in [`docs/client/needs.md` §Intentional differences](../../../../docs/client/needs.md).
Anything you find here that the scopes do differently and that is not
listed there is drift — fix the divergence or add an entry to the doc.

## When to add code here vs. an adjacent directory

- Goes here: any widget whose contract is "shape of the needs flow"
  (sheets, claim rows, suggestion grids, picker chrome).
- Goes in `widgets/shared/needs/`: low-level building blocks the
  experience-scoped batch sheets share with these surfaces (`ChecklistRow`
  and its row family, `NeedsSectionHeader`). Once those are absorbed into
  the sheets here, move them.
- Goes in `widgets/modal/glass/`: cross-feature glass primitives
  (`GlassSheet`, `GlassFooterButtons`, etc.). Do not add needs-specific
  widgets there.
- Goes in `widgets/poll/`: poll-specific shared widgets. Do not mix
  needs widgets with poll widgets — the lifecycles parallel each other
  but the surfaces are not interchangeable.
