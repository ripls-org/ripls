# Mention Widgets

`@`-mention autocomplete system for the chat text entry field.

## Purpose

These components together implement a complete in-chat mention flow: detecting `@` in the text, fetching matching members, displaying a suggestion overlay, inserting the mention as a styled token, and rendering existing mentions as tappable chips in the message list.

## Key Files

- **`mentionable_text_field.dart`** — drop-in `TextField` replacement that shows the suggestion overlay when the user types `@`. The entry point for adding mention support to any text input.
- **`mention_text_controller.dart`** — `TextEditingController` subclass that tracks mention spans and exposes the serialized message for submission.
- **`mention_parser.dart`** — parses raw message strings into a sequence of plain-text and mention spans.
- **`mention_types.dart`** — `MentionSuggestion` and related data types shared across mention components.
- **`mention_suggestion_overlay.dart`** — floating overlay list of matching member suggestions.
- **`mention_suggestion_item.dart`** — a single row in the suggestion overlay (avatar + name).
- **`mention_suggestion_converter.dart`** — converts API user/member protos into `MentionSuggestion` objects.
- **`mention_chip.dart`** — inline `@Name` chip rendered inside the message bubble.
- **`mention_text.dart`** — rich text widget that renders a parsed message with tappable mention chips.

## When to add here vs. elsewhere

Mention-specific components belong here. Generic text-styling or link-detection lives in `widgets/content/linkified_text.dart`.
