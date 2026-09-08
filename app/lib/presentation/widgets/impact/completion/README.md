# Completion Impact Widgets

Impact UI for the completion ceremonies — the Mark Completed (events) and
Mark Fulfilled (requests) sheets and the per-metric detail modals they open.

## Key Files

- **`completion_impact_bar.dart`** — the glassy Saved · Quality Time · CO₂
  strip at the top of both completion sheets. Shows spinner placeholders
  while the draft estimate loads, suppresses zero metrics once loaded, and
  hides entirely when nothing is non-zero (#2724).
- **`value_detail_modal.dart`** / **`co2_detail_modal.dart`** /
  **`quality_time_detail_modal.dart`** — per-metric drill-down modals with
  the calculation breakdown and editable inputs.
- **`impact_modal_shared.dart`** — shared chrome (colors, headline, calc
  card, seg tabs) for the detail modals.
- **`impact_edit_chip.dart`** / **`impact_select_chip.dart`** — tappable
  input chips used on the detail modals' edit tab.

## When to add here vs. elsewhere

Widgets specific to the completion/fulfillment impact flow belong here.
General impact display (receipt screen cards, hero sections) lives in the
parent `widgets/impact/`; the dark person tiles/search that the completion
sheets also use live in `widgets/completion/`.
