# Content Widgets

Shared building blocks for gear, request, experience, and community detail views.

## Purpose

These are the reusable components that make all content-type screens look and behave consistently: edit bars, action bars, metadata chips, avatar rows, gradient overlays, error states, and more. The majority of the content-screen layout is assembled from widgets in this directory.

## Key Files

### Layout and scaffolding
- **`content_view_builders.dart`** — top-level builder functions for assembling the standard content view layout (header, media, body, action bar).
- **`content_view_helpers.dart`** — navigation and context helpers for content screens.
- **`content_tab_bar.dart`** / **`content_view_tab_bar.dart`** — tab bars for multi-section content views.
- **`content_gradient_overlay.dart`** — dark gradient overlaid on background media.

### Redesigned content surface (sheet-over-hero)
Role-agnostic building blocks for the redesigned content views. See `docs/issues/2278-experience-content-redesign.md`. These take plain view-data + callbacks (no Experience/Request types) so the experience view and, later, the request view share them. They are always-on-dark surfaces (the background is media), so they compose from the `dark*` tokens plus a caller-supplied accent — like nudge cards.
- **`content_headline.dart`** — serif title (`AppTheme.headingFont`) + optional description.
- **`content_facts_row.dart`** — one or two label/value fact cards (when/where), optionally tappable.
- **`content_discussion_card.dart`** — the compact conversation entry point: a single serif-italic quote (the description, attributed to the owner) that cross-fades on a timer to the most recent reply, with the thread meta ("— Alfred · 3 replies · last 2h") and an unread dot on the attribution line; tapping opens the conversation. Cycling is skipped under reduce-motion or when only the description exists. Supersedes the headline-description + messages-card split on the experience view.
- **`content_edges_card.dart`** — the contribution graph card (`ContentEdgesCard`: tappable, header label + trailing count) + a participant row (`ContentEdgeRow`: avatar, name, contribution, status pill).
- **`content_edge_data.dart`** — `EdgeViewData` + `EdgeStatus` (Flutter-free); view-models assemble it, `ContentEdgeRow` renders it.
- **`content_lifecycle_phase.dart`** — `ContentLifecyclePhase` enum (sharing → confirmed → leaving → on-the-road → wrapped → cancelled); view-models derive it, widgets map it to copy.

### Editing
- **`content_edit_bar.dart`** — floating edit controls shown when the owner is in edit mode.
- **`content_editable_field.dart`** — tappable text field with overlay styling for in-place editing.
- **`content_editing_mixin.dart`** — mixin providing save/cancel/dirty-check logic to content screens.
- **`close_item_modal.dart`** — confirmation modal for closing/deleting an item.

### Display rows
- **`content_owner_row.dart`** — owner avatar, name, and location row at the top of content bodies.
- **`content_people_row.dart`** — row showing helpers, attendees, or borrowers with avatar stack.
- **`content_avatar.dart`** — styled avatar for content views.

### Actions
- **`content_action_button.dart`** — single full-width primary action button.
- **`content_overflow_menu.dart`** — three-dot menu with owner/admin actions.

### Info and metrics
- **`content_metric_sheet.dart`** — bottom sheet showing impact metrics for the current item.
- **`content_impact_tiles.dart`** — row of compact impact metric tiles (money, time, CO₂).

### Error and access states
- **`content_error_banner.dart`** — inline error message banner.
- **`content_error_view.dart`** — full content area error state with retry.
- **`content_removed_view.dart`** — placeholder shown when content has been deleted.
- **`access_sheet.dart`** — bottom sheet for community access gating.

### Utilities
- **`linkified_text.dart`** — text widget that detects and renders URLs as tappable links.
- **`checklist_status_icons.dart`** — status icons for checklist steps.
- **`content_shared_widgets.dart`** — miscellaneous small shared widgets (dividers, spacing helpers).
- **`inline_conversation_view.dart`** — embeds the chat thread directly inside a content view.

## When to add here vs. elsewhere

Use this directory for widgets used across two or more content types (gear, request, experience, community). If a widget is specific to one content type, place it in that type's widget directory (e.g., `widgets/experience/`). Screen-level assembly logic belongs in `screens/<feature>/`.
