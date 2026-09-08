# Completion Widgets

Dark-themed person-search and selection components used in transfer completion modals.

## Purpose

When completing a loan or giveaway, the owner must confirm or search for the counterparty. These widgets provide a consistent dark-themed search-and-select UI for that step, supporting both registered users and unregistered contacts.

## Key Files

- **`completion_widgets.dart`** — barrel export for all widgets in this directory.
- **`dark_person_search.dart`** — self-contained search panel that combines a community quick-add grid and an inline search field. Manages its own debounce and suggestions state so parent modals stay simple.
- **`dark_search_result_tile.dart`** — a single search result row (avatar + name + optional action button) on a dark background.
- **`dark_person_tile.dart`** — a confirmed-selection tile showing a person with a remove button.
- **`dark_placeholder_avatar.dart`** — dashed-circle placeholder shown when no person has been selected yet.
- **`dark_check_circle.dart`** — animated checkmark indicator for the confirmed state.

## When to add here vs. elsewhere

Use these widgets in completion and confirmation modals that require person selection against a dark background. General-purpose person search or selection with a light theme belongs in `widgets/shared/` or inline in the calling screen.
