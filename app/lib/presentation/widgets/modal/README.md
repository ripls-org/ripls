# Modal Widgets

Scaffolding utilities and building blocks for bottom-sheet modals.

## Purpose

These components provide a consistent visual structure and interaction behavior for all bottom-sheet modals in the app: standard height handling, gradient headers, action buttons, and chip data types.

## Key Files

- **`modal_helpers.dart`** — `ModalHelpers.showStandardModal()` — the canonical way to present a bottom sheet. Handles keyboard-aware height (75% default, 90% with keyboard), smooth animation, and dismiss behavior.
- **`modal_builders.dart`** — `ModalBuilders` static methods for common modal sub-sections: gradient header, info display rows, and section dividers.
- **`modal_header.dart`** — the standard drag-handle + title header shown at the top of modals.
- **`modal_action_buttons.dart`** — primary and secondary action button layout for the bottom of a modal.
- **`chip_data.dart`** — `ChipData` value type used by modals that display selectable chip rows (e.g., location picker).

## When to add here vs. elsewhere

Add here when building a reusable modal component that is shared across feature areas. Feature-specific modal content (e.g., gear creation steps) stays in the feature's own screen or widget directory. See `widgets/creation/` for creation-specific modal components.
