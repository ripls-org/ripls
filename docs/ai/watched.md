# Watched Items — Explicit User Watch List

## Problems

### 1. Inbox inclusion is implicit and opaque

Today, the inbox shows items based on hard-coded heuristic rules in
[`inbox_feed.go`](../../server/services/portfolio/inbox_feed.go): ownership +
engagement (requests, RSVPs, offers), or conversation participation with at
least one human message. Users cannot control what appears. An item they care
about may not surface (e.g., they want to monitor a request before anyone has
offered help), and items they no longer care about cannot be dismissed without
completing or cancelling them.

### 2. No concept of "read" at the item level

Unread state is tracked at the **message** level inside conversations
(`ChatMessage.participant_id_to_is_read`). The inbox badge counts items whose
conversations have unread messages. But there is no notion of whether the user
has **seen the item card itself** — only whether they have read every chat
message. This means:

- An item with no new messages but a meaningful state change (e.g., a new RSVP,
  a returned item) has no way to appear "unread."
- The bold title treatment described in the inbox row spec is tied exclusively to
  `unread_count > 0`, which is a chat-message metric, not an item-attention
  metric.

### 3. Archives are item-lifecycle state, not user attention state

The existing `archived` flag on `CommunityGear`, `CommunityRequest`, and
`CommunityExperience` marks items in a **terminal lifecycle state** (completed,
cancelled, fulfilled). It is set by the server when the item is done — the user
never controls it. There is no mechanism for a user to say "I'm done watching
this item" while the item itself remains active. Archives cannot serve this
purpose because:

- They are per-community-relationship, not per-user.
- They are write-once (set on terminal state, never unset).
- They apply to the item's lifecycle, not the user's attention.

### 4. Badge count conflates attention with chat activity

The bottom nav badge (`inboxUnreadCountProvider`) counts inbox items with
`effectiveUnreadCount > 0`. This means:

- Items with important state changes but no new messages show no badge.
- Items the user has already triaged but with lingering unread messages inflate
  the badge.
- The app icon badge (`AppBadgeService`) uses the same metric — it tells the
  user "you have unread messages" rather than "you have items that need
  attention."

---

## Proposed Solution: Explicit Watch List

Introduce a server-managed **watch list** — a per-user set of `(user_id,
item_type, item_id)` tuples with associated read/unread state. The watch list
becomes the single source of truth for "what appears in this user's inbox."

### Core Model

```
WatchedItem {
  user_id       string    // the watching user
  item_type     enum      // TRANSFER, EXPERIENCE, REQUEST, GEAR
  item_id       string    // the entity ID (transfer ID, experience ID, gear ID, etc.)
  is_read       bool      // has the user seen the latest state?
  created_at    timestamp // when the watch was added
  updated_at    timestamp // last state change that marked it unread
}
```

Watch entries are **one per item globally** — not per community. An experience
shared with communities A and B produces a single `(user_id, EXPERIENCE, exp_id)`
watch. Community filtering happens at display time via the item's `community_id`.

For gear, a single `(user_id, GEAR, gear_id)` watch covers both the shared
listing and any transfer/giveaway derived from it. The inbox dedup rule (transfer
row takes priority over shared listing) is preserved at display time, not in the
watch table.

### Automatic Watch Rules

The system creates watch entries automatically when a user takes a qualifying
action. These mirror the current inbox inclusion rules but are now explicit
records:

| Action | Watch created for |
|--------|-------------------|
| User creates a transfer/giveaway | Owner watches the gear (`GEAR`, gear ID) |
| Someone submits a borrow request | Owner watches the gear (if not already); marked unread |
| User submits a borrow request | Borrower watches the gear (`GEAR`, gear ID) |
| User creates an experience | Owner watches the experience (`EXPERIENCE`, exp ID) |
| User RSVPs to an experience | Attendee watches the experience |
| Non-owner RSVPs to an owned experience | Owner watches (if not already); marked unread |
| User creates a request | Requester watches the request (`REQUEST`, request ID) |
| User offers help on a request | Helper watches the request |
| Someone offers help on user's request | Owner watches (if not already); marked unread |
| User sends first message on a shared gear listing | Owner watches the gear (if not already) |

### Unread Triggers

A watched item is marked `is_read = false` when any of these occur:

| Event | Who is marked unread |
|-------|---------------------|
| New human message in conversation | All watchers except the sender |
| New borrow request on a transfer | The owner |
| New RSVP on an experience | The owner |
| New offer on a request | The requester |
| Item state change (confirmed, returned, cancelled, etc.) | All watchers except the actor |

### Unwatching

In V1, watches are managed entirely by the system — items enter the watch list
via automatic rules and leave only when the item reaches a terminal lifecycle
state (completed, cancelled). The `dismissed_at` column exists in the data model
to support manual unwatch without a schema migration.

Phase 4 adds user-facing unwatch controls: a watch toggle icon on content
screens and swipe-to-unwatch on inbox rows. Dismissed items re-appear if new
activity occurs (`UpsertWatch` clears `dismissed_at`). See
[Phase 4](#phase-4-manual-watch--unwatch) for full details.

The existing `archived` flag on community models remains for its current
lifecycle purpose (marking completed items). When an item is lifecycle-archived,
the server also dismisses all watch entries for that item.

---

## Decisions

1. **Unwatching UI: deferred to a future phase.**
   Users cannot manually unwatch items in V1. The system manages the watch list
   automatically. Manual unwatch (swipe-to-dismiss, unwatch button) will be added
   in a future phase alongside manual watch. See
   [inbox.md Next Steps](../../docs/client/inbox.md#next-steps).

2. **Re-watch on new activity: not applicable in V1.**
   Since users cannot unwatch, the re-watch question does not arise. When manual
   unwatch is added in a future phase, the re-watch policy will be decided then.

3. **Gear listings vs. transfers: one watch per item, one inbox row per item.**
   Watch the underlying item (gear ID), not the relationship type. A single
   `(user_id, GEAR, gear_id)` watch entry covers both the shared listing and any
   transfer that derives from it. The inbox always shows at most one row per
   item — if an active transfer exists for the gear, the transfer row takes
   priority (same dedup rule as today). No duplicate watch entries.

4. **Migration: lazy backfill (option B).**
   On the first `GetPortfolioInboxFeed` call post-deploy, if `GetActiveWatches`
   returns empty for a user, fall back to the current heuristic inclusion logic,
   assemble the inbox as today, and create watch entries for every included item.
   Subsequent calls use the watch list directly. This is invisible to users and
   requires no downtime or batch migration.

5. **Per-community or global watch: one watch globally.**
   One watch per `(user_id, item_id)` regardless of how many communities the item
   is shared with. Community filtering in `inboxFlatItemsProvider` continues to
   work by checking the item's `community_id` field, not the watch record.

6. **Manual watch & unwatch: designed in Phase 4.**
   The data model supports manual watch (no `auto_created` flag — all watches are
   equal). Phase 4 adds a watch toggle icon on content screens and swipe-to-unwatch
   on inbox rows. Dismissed items re-appear on new activity (`UpsertWatch` clears
   `dismissed_at`). See [Phase 4](#phase-4-manual-watch--unwatch) below.

---

## Recommended Implementation

### Phase 1: Data Model & Server

**New storage model** — add to `proto/ripls/models/`:

```protobuf
// watched.proto
message WatchedItem {
  string id = 1;
  string user_id = 2;
  WatchedItemType item_type = 3;
  string item_id = 4;
  bool is_read = 5;
  int64 created_at_unix_sec = 6;
  int64 updated_at_unix_sec = 7;
  optional int64 dismissed_at_unix_sec = 8;  // soft-delete for future manual unwatch
}

// Watches track the underlying item, not the relationship type.
// Transfers and giveaways are watched via GEAR (the item they derive from).
enum WatchedItemType {
  WATCHED_ITEM_TYPE_UNSPECIFIED = 0;
  WATCHED_ITEM_TYPE_GEAR = 1;        // covers shared listings, transfers, giveaways
  WATCHED_ITEM_TYPE_EXPERIENCE = 2;
  WATCHED_ITEM_TYPE_REQUEST = 3;
}
```

**New database table:**

```sql
CREATE TABLE watched_items (
  id              TEXT PRIMARY KEY,
  user_id         TEXT NOT NULL,
  item_type       INT  NOT NULL,
  item_id         TEXT NOT NULL,
  is_read         BOOLEAN NOT NULL DEFAULT false,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  dismissed_at    TIMESTAMPTZ,
  UNIQUE (user_id, item_type, item_id)
);

CREATE INDEX idx_watched_items_user_active
  ON watched_items (user_id, is_read)
  WHERE dismissed_at IS NULL;

CREATE INDEX idx_watched_items_item
  ON watched_items (item_type, item_id)
  WHERE dismissed_at IS NULL;
```

**New storage layer** — `server/storage/watched.go`:

- `UpsertWatch(ctx, userID, itemType, itemID)` — create or un-dismiss
- `MarkRead(ctx, userID, itemType, itemID)` — set `is_read = true`
- `MarkUnreadForWatchers(ctx, itemType, itemID, exceptUserID)` — bulk unread
- `DismissWatch(ctx, userID, itemType, itemID)` — soft-delete
- `GetActiveWatches(ctx, userID)` — all non-dismissed watches for a user
- `GetUnreadCount(ctx, userID)` — count of `is_read = false` where not dismissed
- `GetWatchersForItem(ctx, itemType, itemID)` — all active watchers (for notifications)

<!-- AUDIT FIX: Per server architecture.md, services must not depend on other
     services. Watch storage operations are on `storage.ProtoSQLStorage` (a shared
     library), so all services can call them directly — this is the correct pattern.
     Each service that needs watch writes must receive `*storage.ProtoSQLStorage`
     via constructor injection (they already do). No new service dependency is
     introduced. -->

**Integration points** — add `UpsertWatch` calls to:

| Service | Method | Watch action |
|---------|--------|-------------|
| `transfer` | `CreateTransfer` | Owner watches |
| `transfer` | `RequestToBorrow` | Borrower watches; owner marked unread |
| `transfer` | state machine transitions | All watchers marked unread (except actor) |
| `gear` | `ShareGear` (on first message) | Owner watches |
| `experience` | `CreateExperience` | Owner watches |
| `experience` | `RSVP` | Attendee watches; owner marked unread |
| `request` | `CreateRequest` | Requester watches |
| `request` | `OfferHelp` | Helper watches; requester marked unread |
| `chat` | `SendMessage` | All watchers marked unread (except sender) |

**Modify `inbox_feed.go`:** Replace the current heuristic inclusion logic with a
query join. `GetPortfolioInboxFeed` fetches the user's active watch list, then
batch-loads the associated items. The existing assembly logic
(`assembleTransfer`, `assembleExperience`, etc.) remains unchanged — it just
operates on a watch-list-driven set of items instead of a heuristic-filtered set.

Lazy backfill: if `GetActiveWatches` returns empty for a user, fall back to the
current heuristic logic and create watch entries for each included item. This
is a one-time operation per user on their first post-deploy inbox load.

### Phase 2: API

**New API messages** — add to `proto/ripls/api/portfolio.proto`:

```protobuf
// AUDIT FIX: Use DailyItemType enum (already defined in portfolio.proto)
// instead of string for item_type. Strings invite typos and bypass proto
// enum validation. Each RPC gets its own request/response types per CLAUDE.md.
message MarkInboxItemReadRequest {
  DailyItemType item_type = 1;
  string item_id = 2;
}

message MarkInboxItemReadResponse {}

// DismissInboxItem deferred to future manual-unwatch phase.
// The request/response types and RPC will be added then.
```

**Extend `DailyItem`:**

```protobuf
message DailyItem {
  // ... existing fields ...
  bool is_watched_unread = 33;  // true if the watch entry is_read == false
}
```

**Extend `GetPortfolioInboxFeedResponse`:**

```protobuf
message GetPortfolioInboxFeedResponse {
  repeated DailyItem items = 1;
  int32 unread_watched_count = 3;  // total unread watched items
}
```

### Phase 3: Client — Repository & ViewModel

<!-- AUDIT FIX: The original plan had the ViewModel calling RPCs directly and
     skipped the Repository layer entirely. Per architecture.md rules:
     - "Repositories are the ONLY layer that calls Services"
     - "ViewModels are the ONLY layer that calls Repositories"
     - "Never implement manual cache maps in ViewModels"
     All watch mutations must flow through PortfolioRepository. -->

**`app/lib/data/repositories/portfolio_repository.dart` changes:**

Add methods for the two new RPCs with cache invalidation:

```dart
Future<void> markInboxItemRead({
  required DailyItemType itemType,
  required String itemId,
}) async {
  await _service.markInboxItemRead(itemType: itemType, itemId: itemId);
  await _cache.invalidate('inbox_feed');
}

// dismissInboxItem() deferred to future manual-unwatch phase.
```

**`app/lib/presentation/viewmodels/portfolio_inbox_view_model.dart` changes:**

Add optimistic mark-read and dismiss methods that call the repository (not the
service). Use `ref.mounted` checks after async gaps per architecture.md async
safety rules. Track optimistic read-state deltas in Freezed state (not a manual
`Map` field — use the existing `unreadDeltas` pattern as a model):

```dart
// In PortfolioInboxState (Freezed):
@Default(<String>{}) Set<String> optimisticallyReadItemIds,
```

The viewmodel computes effective `isWatchedUnread` by checking the server-baked
`item.isWatchedUnread` against `optimisticallyReadItemIds`. On the next background
refresh, the server response includes the updated state and the optimistic sets
are cleared.

**`app/lib/presentation/widgets/portfolio/inbox_row.dart` changes:**

- If effective `isWatchedUnread == true`: bold title (current style), full-opacity row.
- If effective `isWatchedUnread == false`: regular-weight title, slightly muted row.
  <!-- AUDIT FIX: Use `AppColors.textPrimary(context)` for bold and
       `AppColors.textSecondary(context)` for muted — never hardcode colors.
       Per architecture.md theming rules. -->
- The existing `effectiveUnreadCount` (chat messages) continues to drive the
  unread message badge on line 3.
- On tap, call `ref.read(portfolioInboxProvider.notifier).markItemRead(item)` which
  optimistically updates state, then calls the repository method.

**Badge changes:**

Replace `inboxUnreadCountProvider`'s computation:

```dart
// Before: count of items with unread chat messages
// After: count of items where isWatchedUnread == true (accounting for optimistic reads)
final inboxUnreadCountProvider = Provider<int>((ref) {
  final items = ref.watch(inboxFlatItemsProvider);
  final inboxState = ref.watch(portfolioInboxProvider);
  return items.where((item) =>
    inboxState.effectiveIsWatchedUnread(item)).length;
});
```

Both the bottom nav badge and the app icon badge now reflect "items needing
attention" rather than "items with unread chat messages."

### Phase 4: Manual Watch & Unwatch

This phase adds two user-facing controls that complement the automatic watch
system:

1. **Watch icon on content screens** — lets users watch or unwatch from the
   item detail view.
2. **Swipe-to-unwatch on inbox rows** — lets users remove items from the inbox
   without navigating into the item.

#### 4a. Watch Icon on Content Screens

Add a watch toggle icon to the floating app bar on all three content screens:
[GearScreen](../../app/lib/presentation/screens/gear/gear_screen.dart),
[ExperienceScreen](../../app/lib/presentation/screens/experience/experience_screen.dart),
and [RequestScreen](../../app/lib/presentation/screens/request/request_screen.dart).

All three screens share an identical `_buildAppBar()` pattern:

```
BackButton | Spacer | [info icon]
```

The watch icon is inserted between the Spacer and the info icon:

```
BackButton | Spacer | [watch icon] | SizedBox(8) | [info icon]
```

**Icon states:**

| State | Icon | Meaning |
|-------|------|---------|
| Not watched | `Icons.visibility_outlined` | User is not watching this item |
| Watched (auto or manual) | `Icons.visibility` (solid) | User is watching — the item appears in the inbox |

The icon uses the same 34×34 circular frosted-glass container style as the
existing info icon (`Colors.black.withValues(alpha: 0.3)` background,
`Colors.white.withValues(alpha: 0.12)` border, icon size 16, icon color
`Colors.white.withValues(alpha: 0.7)`).

**Behavior:**

- On load, the screen fetches the watch state for the current user + item via a
  new `IsWatched` RPC (or a field on the existing detail response).
- Tapping the icon toggles the watch. If currently watched, calls
  `DismissInboxItem`; if not watched, calls `WatchItem`. Both are optimistic —
  the icon flips immediately.
- When a user takes an automatic-watch action (e.g., RSVP, borrow request), the
  icon should update to solid without requiring a full screen refresh. The
  viewmodel can set the local watch state to `true` after any action that
  triggers `UpsertWatch` on the server.
- The icon is visible to all users, not just owners. Anyone viewing an item can
  choose to watch it before taking any action.

**New API messages** — add to `proto/ripls/api/portfolio.proto`:

```protobuf
message WatchItemRequest {
  DailyItemType item_type = 1;
  string item_id = 2;
}
message WatchItemResponse {}

message DismissInboxItemRequest {
  DailyItemType item_type = 1;
  string item_id = 2;
}
message DismissInboxItemResponse {}

message IsWatchedRequest {
  DailyItemType item_type = 1;
  string item_id = 2;
}
message IsWatchedResponse {
  bool is_watched = 1;
}
```

**New RPCs** — add to `PortfolioService`:

```protobuf
rpc WatchItem(WatchItemRequest) returns (WatchItemResponse) {}
rpc DismissInboxItem(DismissInboxItemRequest) returns (DismissInboxItemResponse) {}
rpc IsWatched(IsWatchedRequest) returns (IsWatchedResponse) {}
```

**Client-side watch state provider:**

A per-item `isWatchedProvider(itemType, itemId)` family provider fetches the
initial state via `IsWatched` and exposes a toggle method. The content screen
viewmodels do not need to manage watch state directly — they just read and write
via this provider. After a toggle, the provider invalidates
`portfolio:inbox_feed` in the repository cache.

#### 4b. Swipe-to-Unwatch on Inbox Rows

Add a `Dismissible` wrapper around each
[InboxRow](../../app/lib/presentation/widgets/portfolio/inbox_row.dart) in the
inbox list. The existing
[profile_locations_screen.dart](../../app/lib/presentation/screens/profile/profile_locations_screen.dart)
uses `Dismissible` with `endToStart` direction as a reference pattern.

**Swipe direction:** Right-to-left (`DismissDirection.endToStart`).

**Background:** Muted background with an eye-off icon and "Unwatch" label:

```
                         [Icons.visibility_off]
                              Unwatch
```

Use `AppColors.textTertiary(context)` for the icon and label color, and
`AppColors.surface(context)` for the background — this is a neutral removal
action, not a destructive delete. No confirmation dialog; the action is
undoable via a SnackBar.

**On dismiss:**

1. Optimistically remove the item from `inboxFlatItemsProvider` by adding its
   content ID to an `optimisticallyDismissedItemIds` set in the viewmodel state.
2. Show a `SnackBar` with an "Undo" action. The SnackBar text uses an i18n key
   (e.g., `portfolioInboxItemUnwatched` / `commonUndo`).
3. Fire `DismissInboxItem` RPC in the background via the repository.
4. If "Undo" is tapped before the SnackBar timer expires, call `WatchItem` to
   re-watch and remove the ID from the optimistic set.
5. On next server refresh, the optimistic set is cleared and the server state
   is authoritative.

**Re-watch policy:** If new activity occurs on a dismissed item (e.g., a new
message or state change), the server's existing `UpsertWatch` logic
re-activates the watch entry (it clears `dismissed_at`). This means dismissing
an item is a "mute until next activity" action, not a permanent removal.

#### 4c. i18n Keys

| Key | English | Usage |
|-----|---------|-------|
| `portfolioInboxItemUnwatched` | `"Removed from inbox"` | SnackBar after swipe-to-unwatch |
| `commonUndo` | `"Undo"` | SnackBar undo action (may already exist) |
| `watchItemTooltip` | `"Watch"` | Accessibility label for watch icon (outline state) |
| `unwatchItemTooltip` | `"Unwatch"` | Accessibility label for watch icon (solid state) |

#### 4d. Files to Change

| File | Change |
|------|--------|
| `proto/ripls/api/portfolio.proto` | Add `WatchItem`, `DismissInboxItem`, `IsWatched` RPCs and messages |
| `server/services/portfolio/watch.go` | Implement `WatchItem`, `DismissInboxItem`, `IsWatched` handlers |
| `app/lib/services/portfolio_service.dart` | Add `watchItem`, `dismissInboxItem`, `isWatched` methods |
| `app/lib/data/repositories/portfolio_repository.dart` | Add repository methods with cache invalidation |
| `app/lib/presentation/viewmodels/portfolio_inbox_view_model.dart` | Add `optimisticallyDismissedItemIds`, `dismissItem`, undo logic |
| `app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart` | Wrap `InboxRow` in `Dismissible`, add SnackBar logic |
| `app/lib/presentation/screens/gear/gear_screen.dart` | Add watch icon to `_buildAppBar()` |
| `app/lib/presentation/screens/experience/experience_screen.dart` | Add watch icon to `_buildAppBar()` |
| `app/lib/presentation/screens/request/request_screen.dart` | Add watch icon to `_buildAppBar()` |
| `app/lib/presentation/viewmodels/gear_view_model.dart` | Set local watch state to true after auto-watch actions |
| `app/lib/presentation/viewmodels/experience_view_model.dart` | Set local watch state to true after auto-watch actions |
| `app/lib/presentation/viewmodels/request_view_model.dart` | Set local watch state to true after auto-watch actions |
| `app/lib/core/l10n/app_en.arb` (and other locales) | Add `portfolioInboxItemUnwatched`, `watchItemTooltip`, `unwatchItemTooltip` |

---

## Files to Change

| File | Change |
|------|--------|
| `proto/ripls/models/watched.proto` | **New.** `WatchedItem` message and `WatchedItemType` enum |
| `proto/ripls/api/portfolio.proto` | Add `is_watched_unread` to `DailyItem`; add `unread_watched_count` to response; add `MarkInboxItemRead` RPC with `DailyItemType` enum |
| `server/storage/watched.go` | **New.** CRUD operations for `watched_items` table |
| `server/storage/watched_test.go` | **New.** Unit tests for watch storage |
| `server/services/portfolio/inbox_feed.go` | Replace heuristic filter with watch-list-driven query; lazy backfill |
| `server/services/portfolio/inbox_feed_test.go` | Update tests for watch-based inclusion |
| `server/services/transfer/state_machine.go` | Add `UpsertWatch` / `MarkUnreadForWatchers` calls at state transitions |
| `server/services/transfer/service.go` | Add watch on `RequestToBorrow` |
| `server/services/experience/service.go` | Add watch on `CreateExperience`, `RSVP` |
| `server/services/request/service.go` | Add watch on `CreateRequest`, `OfferHelp` |
| `server/services/chat/service.go` | Add `MarkUnreadForWatchers` on `SendMessage` |
| `app/lib/data/repositories/portfolio_repository.dart` | **AUDIT ADD.** Add `markInboxItemRead()` with cache invalidation |
| `app/lib/services/portfolio_service.dart` | **AUDIT ADD.** Add service method wrapping `MarkInboxItemRead` RPC |
| `app/lib/presentation/widgets/portfolio/inbox_row.dart` | Conditional bold/regular title based on `isWatchedUnread` using theme-aware colors |
| `app/lib/presentation/viewmodels/portfolio_inbox_view_model.dart` | Update `inboxUnreadCountProvider` to use `isWatchedUnread`; add optimistic mark-read via repository |
| `app/lib/presentation/screens/home/home_screen.dart` | Badge now driven by `unread_watched_count` (no structural change, just different count source) |
| `app/lib/services/app_badge_service.dart` | Same — count source changes, no structural change |
| Database migration | `CREATE TABLE watched_items` with indexes |

---

## Relationship to Archives

The existing `archived` flag on `CommunityGear`, `CommunityRequest`, and
`CommunityExperience` is **not replaced or removed**. It continues to mark
terminal lifecycle state. The watch list handles user-level attention separately:

| Concept | Scope | Who controls it | When set |
|---------|-------|----------------|----------|
| `archived` | Per community-item relationship | Server (on terminal state) | Item completes/cancels |
| Watch dismissed | Per user-item | Server only (V1); user manual in future phase | Item reaches terminal state (V1) |
| Watch `is_read` | Per user-item | Server (on events) / client (on view) | New activity marks unread; viewing marks read |

When an item reaches terminal state and `archived` is set, the server also
dismisses all watch entries for that item. This means archived items disappear
from all users' inboxes — the same behavior as today, but now explicit. Manual
user-initiated dismiss will be added in a future phase (see
[inbox.md Next Steps](../../docs/client/inbox.md#next-steps)).

---

## Audit Findings

Audited against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, and `docs/server/architecture.md` on 2026-04-15.

### Issues Found & Fixed

| # | Severity | Section | Issue | Fix Applied |
|---|----------|---------|-------|-------------|
| 1 | **High** | Phase 3 (Client) | **Missing Repository layer.** Original plan had the ViewModel calling `MarkInboxItemRead` / `DismissInboxItem` RPCs directly, violating architecture.md rule: "Repositories are the ONLY layer that calls Services" and "ViewModels are the ONLY layer that calls Repositories." | Added `portfolio_repository.dart` and `portfolio_service.dart` to Files to Change. Rewrote Phase 3 to route all mutations through `PortfolioRepository` with cache invalidation. |
| 2 | **High** | Phase 3 (Client) | **Missing cache invalidation.** No mention of how watch-state mutations invalidate the `portfolio:inbox_feed` cache. Without this, the inbox would show stale read/unread state until TTL expiry or pull-to-refresh. | Repository methods now call `_cache.invalidate('inbox_feed')` after each mutation. Optimistic state in ViewModel is reconciled on next background refresh (matching the existing `unreadDeltas` pattern from caching.md Pattern 7). |
| 3 | **Medium** | Phase 2 (API) | **`item_type` as string instead of enum.** `MarkInboxItemReadRequest` and `DismissInboxItemRequest` used `string item_type` which invites typos and bypasses proto validation. `DailyItemType` enum already exists in `portfolio.proto`. | Changed both request messages to use `DailyItemType item_type`. |
| 4 | **Medium** | Phase 4 (Dismiss) | **Missing i18n.** The SnackBar "undo" text and dismiss confirmation were hardcoded English. | Moot — Phase 4 (dismiss UI) deferred to future phase. i18n keys will be added when dismiss is implemented. |
| 5 | **Medium** | Phase 3 (Client) | **Hardcoded color risk.** The read/unread styling ("slightly muted row") could lead to hardcoded `Colors.grey` usage. Per architecture.md theming rules, all colors must use `AppColors.*` helpers. | Added audit comment specifying `AppColors.textPrimary(context)` for bold titles and `AppColors.textSecondary(context)` for muted titles. |
| 6 | **Low** | Phase 1 (Server) | **Service independence not explicitly stated.** Multiple services (transfer, experience, request, chat) gain watch-write calls. The plan didn't clarify that these call `storage.ProtoSQLStorage` methods directly (shared library), not a watch "service." Per server architecture.md: "Services should not call or depend on other services." | Added audit comment confirming watch operations are storage-layer calls via constructor-injected `*storage.ProtoSQLStorage` — no new service dependency. |

### Compliant (No Changes Needed)

- **Proto package separation**: `WatchedItem` in `models/`, API messages in `api/` — correct per CLAUDE.md.
- **No cross-package proto imports**: `WatchedItemType` is in `models/`, `DailyItemType` is in `api/` — they are independent enums with different scopes (storage vs. API). The API mapping between them happens in Go service code, not in proto imports.
- **SQL efficiency**: `MarkUnreadForWatchers` is a single bulk UPDATE, not a loop. `GetActiveWatches` + batch-load is a 2-query pattern, not N+1. Partial indexes on `dismissed_at IS NULL` are correct.
- **Server file organization**: `watched.go` in `server/storage/` is the right location for a shared storage abstraction.
- **Constructor injection**: All services already receive `*storage.ProtoSQLStorage` — no constructor changes needed.

---

**Last Updated:** 2026-04-15
