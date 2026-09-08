# Shared Widgets

Low-level building blocks reused by more than one feature area but too
small to own a feature directory.

## Purpose

Only `needs/` remains here. It holds the primitive rows and labels the
needs flow composes from; the flow itself — sheets, claim chips, picker
chrome — lives in [`widgets/needs/`](../needs/README.md), and the
experience-scoped batch sheets that consume these primitives live in
`widgets/experience/needs/`.

## Key Files

### `needs/`
- **`action_button.dart`** — `NeedsSectionHeader`, the bold section-title
  label used above needs and contribution lists.
- **`checklist_row.dart`** — the checklist row family (`SuggestionRow`,
  `NeedRow`, `ClaimedRow`) plus `PollChip`, the inline coral pill marking
  a row whose tap opens a poll-creation flow rather than adding a thing.

## When to add here vs. elsewhere

Add here only when a primitive is consumed by two or more needs surfaces
that live in different directories. A widget used by a single feature
belongs in that feature's widget directory (`widgets/needs/`,
`widgets/experience/`, `widgets/request/`). Cross-feature glass
primitives belong in `widgets/modal/glass/`.
