# Presentation Models

Client-side data models used by the presentation layer — not stored on the server and not generated from protos.

## Purpose

These models represent UI-layer concepts: view data shapes, list item adapters, and client-only state that does not map 1:1 to an API proto. Many are Freezed classes for immutability and `copyWith` support.

## Key Files

### List / feed items
- **`discover_item.dart`** — display model for discover search results.
- **`magazine_item.dart`** — display model for magazine-style feed cards.

### Chat
- **`chat_message.dart`** — wraps `MessageHistoryItem` with client-side `MessageState` for optimistic UI (pending / delivered / failed).
- **`avatar_status_badge.dart`** — badge data for avatars in the message list.

### Impact / metrics adapters
- **`item_metric_data_base.dart`** — base type for per-item metric data.
- **`drill_down_data.dart`** — aggregated drill-down data model for impact detail screens.

## When to add here vs. elsewhere

Add a model here when it is a presentation-only concept with no direct server equivalent. If the shape is generated from a proto, use `app/lib/data/gen/` directly. If the model carries business logic or is shared with services, put it in `app/lib/data/`.
