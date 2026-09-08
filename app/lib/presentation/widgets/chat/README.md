# Chat Widgets

Components for the in-content real-time chat thread shown inside gear, request, experience, and community views.

## Purpose

These widgets render a streaming message list with system messages, emoji reactions, mention highlighting, and a morphing action button. They are used wherever an inline conversation appears.

## Key Files

### Core
- **`message_list.dart`** — scrollable message history with optimistic pending states, system message rendering, and per-message reaction display.
- **`conversation_overlay_chrome.dart`** — shared header treatment for the full-screen conversation panels (gear / request / experience): top inset + fade scrim so transcript text never collides with the controls, and a single dismiss affordance (the hosting screen hides its own back chevron while a panel is open).
- **`morphing_action_button.dart`** — animated FAB-style button that morphs between states (e.g., "help" → "offered").
- **`cached_media_image.dart`** — image widget backed by a stable media-ID cache key; use this instead of `Image.network` for all user media.
- **`system_message_text_resolver.dart`** — resolves a structured `SystemMessage` to locale-appropriate display text (#1904).
- **`action_dropdown_menu.dart`** — context menu for long-press actions on messages.

## Subdirectories

| Directory | Contents |
|-----------|----------|
| `message_list/` | Row-level pieces the list composes: chat bubble, media attachments, poll banner/label, system message row |
| `mention/` | `@`-mention autocomplete: text controller, parser, suggestion overlay, chip, and types |
| `reaction/` | Emoji reaction picker, badge display, and reaction detail sheet |

## When to add here vs. elsewhere

Chat display widgets belong here. If a widget is only used in a specific creation modal or screen, keep it in that screen's file. Generic reusable widgets (avatar, unread badge) live in `widgets/` root.
