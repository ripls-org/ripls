# View Models

Riverpod notifiers and state classes for the presentation layer, organized by feature.

## Purpose

Each viewmodel manages the observable state for one screen or modal. State classes are Freezed for immutability; notifiers call repositories and services, then expose the result as typed state. Generated `.freezed.dart` files live alongside their source files and are not checked in.

## Key Files

### Community
- **`community_view_model.dart`** — media carousel, edit state for the community detail screen.
- **`community_content_view_model.dart`** / **`community_content_state.dart`** — feed and tab state for the community content view.
- **`community_edit_view_model.dart`** — in-place editing of community fields.
- **`community_impact_view_model.dart`** — community impact and metrics screens.
- **`manage_members_view_model.dart`** — member management screen state.
- **`governance_view_model.dart`** — governance screen state.

### Gear
- **`gear_view_model.dart`** — gear detail screen state and watch/unwatch.
- **`gear_sharing_view_model.dart`** — sharing settings for a gear item.
- **`gear_metric_view_model.dart`** — per-item impact metric display.

### Requests
- **`request_view_model.dart`** — request detail screen state.
- **`request_creation_view_model.dart`** — request creation flow.
- **`request_sharing_view_model.dart`** — community sharing for requests.
- **`request_needs_view_model.dart`** — needs sub-flow.
- **`request_metric_view_model.dart`** — per-request impact metrics.
- **`gen_request_view_model.dart`** — AI request generation flow.
- **`fulfill_modal_view_model.dart`** — mark-fulfilled modal state.

### Experiences
- **`experience_view_model.dart`** — experience detail screen state.
- **`experience_creation_view_model.dart`** — experience creation flow.
- **`experience_sharing_view_model.dart`** — community sharing for experiences.
- **`experience_needs_view_model.dart`** — needs management within an experience.
- **`experience_metric_view_model.dart`** — per-experience impact metrics.
- **`gen_experience_view_model.dart`** — AI experience generation flow.

### Transfers (loans and giveaways)
- **`loan_borrower_view_model.dart`** / **`loan_borrower_state.dart`** — borrower side of a loan (request, return).
- **`giveaway_giver_view_model.dart`** / **`giveaway_giver_state.dart`** — giver side of a giveaway.
- **`giveaway_receiver_view_model.dart`** / **`giveaway_receiver_state.dart`** — receiver side of a giveaway.

### Portfolio
- **`portfolio_view_model.dart`** — personal item library screen.

### Profile and user
- **`user_profile_view_model.dart`** / **`user_profile_state.dart`** — another user's profile.
- **`profile_metrics_view_model.dart`** — current user's personal metrics.
- **`profile_locations_view_model.dart`** — user location management.
- **`register_view_model.dart`** — registration flow state.

### Home, feed, discover
- **`home_view_model.dart`** — selected community, tab, and global home state.
- **`feed_view_model.dart`** — community activity feed.
- **`discover_view_model.dart`** — discover/search screen.
- **`search_view_model.dart`** — search query and results.

### Modals and utilities
- **`event_modal_view_model.dart`** / **`event_modal_state.dart`** — community event action modal.
- **`time_modal_view_model.dart`** — time-slot scheduling modal.
- **`location_picker_view_model.dart`** — location search and selection modal.
- **`gen_community_view_model.dart`** — community creation state: name/description editing, background-image streaming, media replacement, and submission.
- **`feedback_sheet_view_model.dart`** — user feedback submission.
- **`locale_view_model.dart`** — locale selection.
- **`splash_view_model.dart`** — app startup / deep-link routing.
- **`story_view_model.dart`** — story display and undo.
- **`unread_count_view_model.dart`** — badge count across tabs.
- **`conversation_view_model.dart`** — in-content chat thread.
- **`needs_scope.dart`** — shared scope for needs sub-flows in experiences and requests.

## Chat message ordering invariant

`conversation_messages_notifier.dart` is the sole owner of chronological message order. Every code path that produces or merges a message list (`loadMessages`, `_handleStreamedMessage`, `_mergeServerHistory`) must pass the result through `_sortByChronology` before writing to state. Widgets must not assume any particular order in the list they receive — they render what the notifier provides. This invariant prevents out-of-order display caused by stream delivery jitter, reconnect races, or an unsorted server backlog (see #1826).

## When to add here vs. elsewhere

Add a viewmodel here for any screen or modal with meaningful async state or user interactions. Simple stateless widgets that only display data from a provider do not need a dedicated viewmodel — read the provider directly in the widget.
