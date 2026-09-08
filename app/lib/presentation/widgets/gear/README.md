# Gear Widgets

Reusable widgets specific to the gear detail view and its transfer sub-flows.

## Purpose

These components are used inside `GearContentView` and gear-related modals. They handle metadata display and editing, borrower/interest flows, and gear-specific information panels.

## Key Files

- **`gear_metadata_sheet.dart`** — bottom sheet for viewing and editing gear metadata: category, brand, model, material, weight, and estimated value. Returns a `GearMetadataEditResult` with the edited fields flagged for provenance tracking.
- **`gear_borrower_sheet.dart`** — bottom sheet for initiating a borrow request: date selection and message.
- **`gear_interest_sheet.dart`** — bottom sheet for expressing interest in a gear item without requesting a specific date.

## When to add here vs. elsewhere

Gear-specific widgets belong here. Transfer lifecycle widgets (pickup scheduling, phase bars) are in `widgets/transfer/`. General content building blocks (action bar, metadata chips) are in `widgets/content/`.
