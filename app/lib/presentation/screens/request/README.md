# Request Screens

Screens and modals for viewing, creating, and fulfilling community requests.

## Purpose

Requests are asks posted to a community (e.g., "Does anyone have a ladder?"). This directory covers the full request surface: the detail screen, creation flow, and the modals for owners and helpers to manage the request lifecycle.

## Key Files

### Detail
- **`request_screen.dart`** — entry point; slides in from the right and hosts `RequestContentView`.
- **`request_content_view.dart`** — scrollable body: media, description, needs section, action bar, and chat.
- **`request_preview_modal.dart`** — read-only preview card shown to potential helpers before they offer to help.

### Creation
- **`request_creation_modal.dart`** — AI-assisted request creation bottom sheet (text or image input).

### Lifecycle
- **`request_management_modal.dart`** — owner controls: close the request, view offers, select a helper.
- **`mark_fulfilled_modal.dart`** — owner marks a request fulfilled and records impact data.

## When to add here vs. elsewhere

Request-specific screens and modals belong here. Reusable request widgets (timeline, needs sheets, management card) live in `widgets/request/`. Shared needs-management widgets used by both requests and experiences live in `widgets/shared/`.
