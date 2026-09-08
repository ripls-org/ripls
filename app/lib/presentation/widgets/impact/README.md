# Impact Widgets

Display components for impact metrics across user, item, and community scopes.

## Purpose

These widgets compose the impact hero sections, the formula and
provenance breakdowns behind a number, and the loan-history entries that
feed a total. Sub-folder `community/` holds the community-scoped chart
card.

## Key Files

### Hero and summary
- **`impact_hero_section.dart`** — dark-gradient hero with name, metadata line, description, and stat chips. Supports item, user, and community variants.
- **`stat_chip.dart`** — small bordered chip displaying a label and value, used in hero sections.

### Formula and provenance
- **`formula_visualization.dart`** — visual breakdown of the calculation formula.
- **`formula_component_card.dart`** — a single component in the formula visualization.
- **`audit_trail_step.dart`** — one attribute's provenance row (source badge + value) in the audit trail screen.
- **`reference_list.dart`** — list of methodology sources shown in detail views.

### History
- **`loan_history_card.dart`** — a completed loan summarized as an impact history entry.

### Sharing
- **`sharing_impact_card.dart`** — card showing the total impact of a single sharing action.

## Subdirectories

`community/` — the community-scoped chart card. See its own README.

## When to add here vs. elsewhere

Impact display widgets belong here. Data computation lives in
`viewmodels/`. The screens that explain how a metric was derived are in
`screens/impact_metrics/`.
