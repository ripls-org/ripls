# Transfer Widgets

Modal shell, step components, and utility widgets for loan and giveaway flows.

## Purpose

Loans and giveaways share a multi-step modal structure with a consistent header, scrollable content area, and phase-aware bottom bar. These widgets provide that shared scaffold and the step-level building blocks used by both flow types.

## Key Files

### Modal structure
- **`transfer_modal_shell.dart`** — consistent modal layout: optional header, scrollable content, optional bottom bar. Used by every transfer step modal.
- **`transfer_modal_header.dart`** — header section showing gear thumbnail, title, and community context.
- **`transfer_modal_title.dart`** — styled title row for the modal header.

### Phase and progress
- **`transfer_phase_bottom_bar.dart`** — bottom bar that adapts its action buttons to the current transfer phase (request / approve / return / complete).
- **`transfer_progress_bar.dart`** — horizontal progress bar indicating the current step within a transfer lifecycle.

### Step cards
- **`waiting_on_user_card.dart`** — card shown when the flow is paused waiting for the counterparty to act.
- **`what_happens_next_card.dart`** — explanatory card describing the next step in the transfer.
- **`what_they_see_card.dart`** — preview of what the counterparty's view looks like at the current step.

### Utilities
- **`transfer_utils.dart`** — `formatRelativeTimestamp()` and other small formatting helpers used across transfer widgets.

## When to add here vs. elsewhere

Transfer-step widgets that are shared between loan and giveaway modals belong here. Content that is specific to one transfer type (loan owner controls, giveaway recipient selection) lives in the respective screen modals under `screens/gear/`. Pickup scheduling widgets used by both transfers and experiences are in `widgets/shared/`.
