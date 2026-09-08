# Gear Screens

Screens and modals for viewing, creating, and managing gear items and their transfer flows.

## Purpose

Gear items are physical objects shared within a community. This directory covers the full gear surface: detail screen, AI-assisted creation, and the complete loan and giveaway lifecycle modals.

## Key Files

### Detail
- **`gear_screen.dart`** — entry point; slides in from the right and hosts `GearContentView`.
- **`gear_content_view.dart`** — scrollable body: media, metadata, availability, needs, action bar, and chat.

### Creation
Gear creation lives in `screens/create/` — every entry point routes through `openBlankCreateGear` → `UnifiedCreateModal`. No gear-specific creation modal exists here.

### Loan flow
- **`loan_owner_modal.dart`** — owner approves, tracks, or completes a loan.
- **`loan_borrower_modal.dart`** — borrower requests a loan and marks it returned.
- **`loan_manage_modal.dart`** — manage an active loan (messages, extend, complete).
- **`loan_list_modal.dart`** — list of all active and past loans for a gear item.
- **`arrange_pickup_modal.dart`** — coordinate pickup time and location.
- **`past_transfer_modal.dart`** — read-only view of a completed transfer.
- **`transfer_modal_router.dart`** — routes to the correct transfer modal based on transfer type and the viewer's role.

### Giveaway flow
- **`giveaway_giver_modal.dart`** — giver selects a recipient and confirms.
- **`giveaway_receiver_modal.dart`** — receiver accepts or declines a giveaway offer.

## When to add here vs. elsewhere

Gear-specific screens and modals belong here. Reusable gear widgets (metadata sheet, interest sheet, borrower sheet) live in `widgets/gear/`. Transfer-step widgets shared across loans and giveaways live in `widgets/transfer/`.
