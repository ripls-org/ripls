# Unread Indicator on Content Action Bar Chat Pill

## Problem Statement

The `ContentActionBar` chat pill currently uses an **outline** chat icon (`Icons.chat_bubble_outline`) and displays only the **total message count** as a text label when messages exist. It provides no visual distinction between "messages exist" and "messages exist that you haven't read yet." Users have no way to know from the content view whether a conversation has unread messages waiting for them.

### Current Behavior

- Chat icon: `Icons.chat_bubble_outline` (always outline)
- When `messageCount > 0`: coral-tinted background + count label (e.g., "12")
- When `messageCount == 0`: neutral background, no label
- No awareness of unread state at all

### Desired Behavior

- When **unread messages exist**: solid chat icon (`Icons.chat_bubble`) + standard unread circle indicator (no count text)
- When **no unread messages** but messages exist: outline chat icon (`Icons.chat_bubble_outline`) + total message count text (current behavior)
- When **no messages at all**: outline chat icon, no label (current behavior)

### Affected Content Views

1. `gear_content_view.dart` — passes `gear.messageCount` and `gear.conversationId`
2. `request_content_view.dart` — passes `request.messageCount` and `request.conversationId`
3. `experience_content_view.dart` — passes `experience.experience.messageCount` and `experience.experience.conversationId`
4. `community_content_view.dart` — not affected (hardcodes `messageCount: 0`, no chat)

## Existing Infrastructure

### Unread Count System (already built)

The app has a complete unread tracking system that is **not currently used** by `ContentActionBar`:

- **`UnreadCountRepository`** (`app/lib/data/repositories/unread_count_repository.dart`): Caches unread counts per conversation via `GetUnreadCounts` RPC. Supports optimistic increments/resets. Key method: `getUnreadCounts()` returns `Map<String, int>` of `conversationId -> unreadCount`.
- **`UnreadCountNotifier` / `unreadCountProvider`** (`app/lib/presentation/viewmodels/unread_count_view_model.dart`): Riverpod notifier exposing `UnreadCountState` with `conversationIdToUnreadCount` map. Already watched elsewhere in the app.
- **`UnreadBadge`** widget (`app/lib/presentation/widgets/unread_badge.dart`): Amber pill badge used on community avatars and inbox items. Displays count (or "99+"). Hidden when count is 0.

### Content Action Bar

`ContentActionBar` (`app/lib/presentation/widgets/content/content_action_bar.dart`) is a pure `StatelessWidget` — all data comes from constructor parameters. It currently accepts `int messageCount` but has no concept of `unreadCount`.

### Data Availability

Each content item proto (Gear, Request, Experience) has:
- `conversation_id` — links to the conversation
- `message_count` — total messages in the conversation

The `UnreadCountNotifier` state contains `conversationIdToUnreadCount` — a map that can be looked up by `conversation_id` to get the per-conversation unread count. This data is already loaded on app startup.

## Potential Solutions

### Option A: Add `unreadCount` parameter to ContentActionBar

Add a new `int unreadCount` parameter to `ContentActionBar`. Each content view looks up the unread count from `unreadCountProvider` using the item's `conversationId` and passes it down. The `_buildChatPill()` method switches between solid/outline icon and shows either the unread dot or total count based on this value.

**Pros:** Clean, follows existing pure-data-in pattern. ContentActionBar remains a pure StatelessWidget.
**Cons:** Each content view needs to watch `unreadCountProvider` and look up the count.

### Option B: Make ContentActionBar a ConsumerWidget

Convert `ContentActionBar` to a `ConsumerWidget` that watches `unreadCountProvider` directly, accepting a `conversationId` parameter instead of `messageCount`/`unreadCount`.

**Pros:** Centralizes the unread lookup logic.
**Cons:** Violates the widget's documented design ("all data comes from constructor parameters"). Introduces provider dependency into a pure widget. Makes testing harder.

### Option C: Create a wrapper widget

Create a new `ContentActionBarWithUnread` that wraps `ContentActionBar`, watches `unreadCountProvider`, and passes the resolved unread count down.

**Pros:** Doesn't modify existing widget.
**Cons:** Adds unnecessary indirection. Another file to maintain.

## Recommendation

**Option A** is the clear winner. It preserves `ContentActionBar`'s pure StatelessWidget design, follows the established pattern of passing all data as constructor parameters, and requires minimal changes. The lookup logic in each content view is a single line reading from an already-available provider.

## Open Questions

1. **Unread dot style**: Should the unread indicator be a small filled circle (like iOS notification dots), or reuse the existing `UnreadBadge` amber pill? A small dot is simpler and fits the compact chat pill better. **Recommendation: small filled circle (white or coral) positioned top-right of the chat icon, similar to standard notification dots.**

2. **Community content view**: Currently hardcodes `messageCount: 0`. Should it also get unread support? **Recommendation: no, leave as-is since community content views don't have conversations.**

## Implementation Plan — Phase 1: Unread Indicator on Chat Pill

### Step 1: Update ContentActionBar widget

**File:** `app/lib/presentation/widgets/content/content_action_bar.dart`

- Add `int unreadCount` parameter (default 0) to constructor
- Update `_buildChatPill()`:
  - When `unreadCount > 0`: use `Icons.chat_bubble` (solid), show a small filled circle indicator (8px, coral color) offset top-right of the icon via a `Stack`, suppress the count text
  - When `unreadCount == 0 && messageCount > 0`: use `Icons.chat_bubble_outline` (current), show `messageCount` text (current behavior)
  - When both are 0: use `Icons.chat_bubble_outline`, no label (current behavior)
- Keep coral-tinted background when `messageCount > 0` (regardless of unread state — messages exist)

### Step 2: Update content views to pass unreadCount

**Files:**
- `app/lib/presentation/screens/gear/gear_content_view.dart`
- `app/lib/presentation/screens/request/request_content_view.dart`
- `app/lib/presentation/screens/experience/experience_content_view.dart`

For each content view:
- In the method that builds the action bar, watch `unreadCountProvider` to get `conversationIdToUnreadCount`
- Look up `unreadCount` using the item's `conversationId`
- Pass `unreadCount: unreadCount` to `ContentActionBar`

Example pattern:
```dart
final unreadCounts = ref.watch(unreadCountProvider).conversationIdToUnreadCount;
final unreadCount = unreadCounts[gear.conversationId] ?? 0;

return ContentActionBar(
  // ... existing params ...
  messageCount: gear.messageCount,
  unreadCount: unreadCount,
  // ...
);
```

### Step 3: Update ContentActionBar tests

**File:** `app/test/presentation/widgets/content/content_action_bar_test.dart`

Add test cases:
- Chat pill shows solid icon (`Icons.chat_bubble`) when `unreadCount > 0`
- Chat pill shows outline icon (`Icons.chat_bubble_outline`) when `unreadCount == 0`
- Chat pill shows unread dot indicator when `unreadCount > 0`
- Chat pill shows message count text when `unreadCount == 0 && messageCount > 0`
- Chat pill hides both dot and count when both are 0
- Unread dot is not shown when `unreadCount == 0` even if `messageCount > 0`

### Step 4: Verify and test

- Run `flutter test --reporter expanded` on the action bar test file
- Run `flutter analyze` to check for lint issues
- Manual verification that existing content views compile and pass the new parameter
