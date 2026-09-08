# Reaction Widgets

Emoji reaction components for the chat message list.

## Purpose

These widgets implement the full reaction flow: the long-press quick-picker bar, per-message reaction badge display, and the detail sheet listing who reacted with each emoji.

## Key Files

- **`reaction_picker.dart`** — pill-shaped quick-reaction bar shown above a message on long-press. Contains six default emoji and an expand button.
- **`emoji_picker_sheet.dart`** — full system emoji picker presented when the user taps the expand button in `ReactionPicker`.
- **`reaction_badge.dart`** — compact badge (emoji + count) overlaid at the bottom of a message bubble.
- **`reaction_detail_sheet.dart`** — bottom sheet listing all reactions on a message grouped by emoji, with the users who chose each.

## When to add here vs. elsewhere

Reaction UI belongs here. Reaction state management (toggling, optimistic updates) lives in `viewmodels/conversation_view_model.dart`. Generic emoji display not tied to reactions should go in a more general location.
