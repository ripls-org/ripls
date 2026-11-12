# Comment Bar Expansion — Design Document

> **Audit log (2026-03-17):** Plan reviewed against architecture.md, caching.md,
> i18n.md, and server/architecture.md. Changes flagged with `[AUDIT]`.

## Problem Statement

The current `ContentActionBar` has a small chat pill in the center segment that
shows a message count and unread dot, but provides no preview of the
conversation. Users must tap into the chat to see what's been said and by whom.
The reference designs show a richer comment bar that surfaces:

1. **No comments** — a placeholder input ("Ask a question or leave a note…")
   with a chat icon, plus the Review button.
2. **Unread comments** — an avatar stack of recent commenters, the latest
   comment preview (author + timestamp + truncated text), an "N unread comments"
   label in coral, plus the Review button.
3. **All read** — same layout but muted tones, "View all N comments" label
   instead of unread count.

This replaces the current 3-segment bar (helpers | chat pill | action button)
with a taller glass card that lives at the same position (bottom of the content
view, above the safe area).

## Current State

### ContentActionBar (shared widget)
- 3-segment row: helpers section | chat pill | action button
- Chat pill shows: icon + message count text, or filled icon + unread badge
- No comment preview, no avatar stack of commenters, no last-message text

### Proto data available on Gear/Experience/Request
- `message_count` (int32) — total messages
- `conversation_id` (string) — links to the conversation
- Unread counts come from a separate `unreadCountProvider` (calls
  `GetUnreadCounts` RPC, keyed by conversation_id)

### Proto data on ConversationItem (from ListConversations/GetConversation)
- `participants` (repeated User)
- `last_message_text` (string, truncated ~150 chars)
- `last_message_sender` (User)
- `last_message_at_unix_sec` (int64)
- `unread_count` (int32)

### Gap
The Gear/Experience/Request protos do **not** carry `last_message_text`,
`last_message_sender`, `last_message_at_unix_sec`, or `participants`
(commenters). To render the reference designs, we need this data either:
- (A) Added to the item protos themselves (denormalized), or
- (B) Fetched from the conversation system at display time.

## Potential Solutions

### Option A: Denormalize comment preview into item protos
Add fields to `Gear`, `Experience`, and `Request` protos:
```protobuf
optional string last_message_text = 14;
optional User last_message_sender = 15;
optional int64 last_message_at_unix_sec = 16;
int32 unread_count = 17;
repeated User recent_commenters = 18;
```
Server populates these during GetGear/ListGear/etc. by joining against the
conversation table.

**Pros:** Single RPC per item, no extra client calls, data always consistent.
**Cons:** Adds 5 fields to 3 protos; server must join conversations on every
read; more proto surface area to maintain.

### Option B: Fetch conversation context on the client side
Use the existing `GetConversation` or `GetConversationContext` RPC on the
client, keyed by `conversation_id`, to load preview data when rendering the
content view.

**Pros:** No proto changes; reuses existing conversation system.
**Cons:** Extra RPC per content view load; data may arrive after the action bar
renders (flash of empty state); cache coordination between gear and conversation
repos.

### Option C: Hybrid — add only to Gear proto, reuse ConversationItem for others
Experience and Request content views already route through the chat system more
heavily (workflow conversations). Gear is the main feed item that needs inline
comment previews. Add the denormalized fields to Gear only; experience and
request can use Option B or keep their current chat pill.

## Recommendation

**Option A (denormalize into item protos)** is the recommended approach.

Rationale:
- The action bar renders immediately on content view open — an extra async fetch
  would cause a visible loading flash.
- The server already joins conversations to populate `message_count`; adding
  `last_message_*` and `recent_commenters` is incremental.
- All three content types (gear, experience, request) benefit from the same
  preview pattern, so treating them consistently avoids divergent UX.
- The portfolio inbox already follows this pattern (it denormalizes
  `last_message_sender`, `last_message_text`, `last_message_time_ago`,
  `unread_count` onto `InboxItem`).

> **[AUDIT] Unread count staleness risk.** The current architecture uses
> `unreadCountProvider` with an **optimistic delta pattern**
> (see caching.md § Optimistic Delta Pattern) for instant badge updates — when a
> user reads messages, the unread dot disappears immediately without waiting for
> a server round-trip. If we bake `unread_count` into the item proto, the cached
> item data becomes stale the moment the user reads a message or receives a new
> one. Two options to preserve instant updates:
>
> **(A) Keep `unreadCountProvider` as the live overlay.** The `CommentBar`
> reads `unread_count` from the provider (exactly as `ContentActionBar` does
> today), NOT from the proto field. The proto's `unread_count` is only used as
> the initial seed / fallback. This preserves optimistic deltas with zero new
> work.
>
> **(B) Remove `unreadCountProvider` and accept stale counts until item
> refetch.** Simpler, but badges lag until the cached item expires or is
> invalidated.
>
> **Recommendation:** Option (A) — keep the existing optimistic delta overlay.
> Phase 4 should NOT remove `unreadCountProvider` usage.

## Open Questions

1. **Which content views get the new bar?** The reference only shows gear. Do
   experience and request content views also get the expanded comment bar, or
   keep the existing 3-segment layout? **Recommendation:** Apply to all three
   (gear, experience, request). Community content view is a special case and can
   keep its current layout.

2. **Does the "Review" button behavior change?** In the reference designs,
   "Review" appears in all three states. Currently, the action button morphs
   between Review/Request/Requested/On Loan depending on context. Does the new
   comment bar coexist with the existing action button states, or does "Review"
   always open the conversation? **Recommendation:** Keep the existing
   `MorphingActionButton` behavior — it already handles Review/Request/etc.
   The new comment bar simply replaces the helpers+chat segments.

3. **Does the helper/attendee section disappear?** The reference designs show
   no helper count — just commenters + comment preview + action button. Is the
   helper count (e.g., "3 helping") removed entirely, or moved elsewhere?
   **Recommendation:** The helper section is replaced by the comment preview.
   Helper count can be shown elsewhere (e.g., in the metadata area above).

4. **Tap target for the comment preview area** — should tapping the comment
   preview open the conversation screen (same as the current chat pill tap)?
   **Recommendation:** Yes, the entire comment preview area is tappable and
   opens the conversation.

5. **Avatar stack composition** — the reference shows 3 avatars of recent
   commenters. Should this be the most recent unique message senders, or the
   conversation participants? **Recommendation:** Most recent unique
   commenters (non-system-message senders), limited to 3, ordered by recency.

6. **"Review" button on non-owner items** — currently non-owners see "Request"
   instead of "Review". The reference always shows "Review". Should non-owners
   still see "Request" or change to "Review"? **Recommendation:** Keep existing
   button labels — the reference designs show the owner perspective only.

7. **[AUDIT] Cache invalidation after chat mutations.** When a user sends a
   message from the content view, the denormalized `last_message_*` and
   `recent_commenters` fields baked into the cached item proto become stale.
   How is the item cache refreshed? **Recommendation:** `ChatRepository` should
   call the appropriate cache invalidation provider (e.g.,
   `gearCacheInvalidationProvider`) after `sendMessage()`, similar to how
   `portfolioCacheInvalidationProvider` is notified today. This triggers an
   in-place refresh of the content view's item data.

## Implementation Plan

### Phase 1: Proto changes (server)
**Goal:** Add comment preview fields to Gear, Experience, and Request protos.

**Files:**
- `proto/ripls/api/gear.proto` — add `unread_count`, `last_message_text`,
  `last_message_sender`, `last_message_at_unix_sec`, `recent_commenters`
- `proto/ripls/api/experience.proto` — same fields
- `proto/ripls/api/request.proto` — same fields
- Run `npm run generate`

**Fields to add (same on all three):**
```protobuf
// Number of unread messages in the conversation for the current user.
int32 unread_count = <next>;

// Truncated text of the most recent user message (~100 chars).
optional string last_message_text = <next>;

// The user who sent the most recent message.
optional User last_message_sender = <next>;

// Unix timestamp of the most recent message.
optional int64 last_message_at_unix_sec = <next>;

// Up to 3 most recent unique commenters, ordered by recency.
repeated User recent_commenters = <next>;
```

### Phase 2: Server population (server)
**Goal:** Populate the new fields in all RPCs that return Gear/Experience/Request.

**Files:**
- `server/services/gear/` — GetGear, ListGear, SearchGear etc.
- `server/services/loan/` — GetTransfer etc. (returns embedded Gear)
- `server/services/search/` — search results
- `server/services/experience/` — GetExperience, ListExperiences
- `server/services/community/` — community gear/experience listings

**Approach:** Create a shared helper (e.g., `conversation.EnrichWithCommentPreview`)
that takes a conversation_id + current user_id and returns the preview fields.
Call this helper wherever items are assembled. Use batch loading
(`GetConversationsByIDs`) for list endpoints to avoid N+1 queries.

> **[AUDIT] Server architecture compliance.** Per server/architecture.md § Service
> Independence, services must not call other services. The shared helper belongs
> in the existing `server/conversation/` shared library package (which already
> contains `state.go`), NOT in `server/services/chat/`. Services inject the
> library, not the chat service. This is consistent with Pattern 1 (Standalone
> Libraries for Shared Logic).

### Phase 3: New comment bar widget (client)
**Goal:** Create a new `CommentBar` widget that renders all three states.

**New file:** `app/lib/presentation/widgets/content/comment_bar.dart`

**Three states:**
1. **Empty** (`messageCount == 0`): Row with chat icon circle + placeholder text
   + action button. Single-row layout matching the reference's glass card style.
2. **Unread** (`unreadCount > 0`): Two-row glass card. Top row: avatar stack +
   author name + timestamp + truncated message. Bottom row (below divider):
   "N unread comments" in coral + action button.
3. **All read** (`messageCount > 0, unreadCount == 0`): Same two-row layout but
   muted colors. Bottom row: "View all N comments" in muted white + action button.

**Props:**
```dart
class CommentBar extends StatelessWidget {
  final int messageCount;
  final int unreadCount;
  final String? lastMessageText;
  final String? lastMessageSenderName;
  final String? lastMessageTimeAgo;
  final List<User> recentCommenters;
  final Widget actionButton;
  final VoidCallback onCommentTap;
}
```

**Styling:** Glass card (`Colors.white.withValues(alpha: 0.12)` background with
`BackdropFilter`, rounded corners 14px, thin white border). Matches the
reference designs exactly.

> **[AUDIT] Theming exception — glass overlay colors.** Per architecture.md
> § Colors and Theming, all colors should use `AppColors`. However, glass-card
> overlays on full-bleed images are a documented exception — the existing
> `ContentActionBar` already uses `Colors.white.withValues(alpha: ...)` because
> these overlays render on top of arbitrary photo backgrounds, not on themed
> surfaces. The `CommentBar` should follow the same pattern as
> `ContentActionBar`: use `Colors.white.withValues(alpha: ...)` for glass card
> backgrounds, borders, and text on overlay. Use `AppColors.primary(context)` for
> the coral accent color (e.g., the "unread comments" label).

> **[AUDIT] i18n compliance.** Per i18n.md, every user-visible string must use
> `context.l10n.*`. The `CommentBar` introduces three new strings that need ARB
> keys in both `app_en.arb` and `app_es.arb`:
>
> | ARB key | English | Notes |
> |---------|---------|-------|
> | `commentBarPlaceholder` | `"Ask a question or leave a note…"` | Empty state |
> | `commentBarUnread` | `"{count, plural, =1{1 unread comment} other{{count} unread comments}}"` | ICU plural |
> | `commentBarViewAll` | `"View all {count} comments"` | All-read state |
>
> The widget must accept `BuildContext` and call `context.l10n.*` — it should
> NOT receive pre-resolved strings as props. This keeps the widget testable
> with localization delegates (per i18n.md § Test localization).

### Phase 4: Replace ContentActionBar usage (client)
**Goal:** Swap `ContentActionBar` for `CommentBar` in all content views.

**Files:**
- `app/lib/presentation/screens/gear/gear_content_view.dart`
- `app/lib/presentation/screens/experience/experience_content_view.dart`
- `app/lib/presentation/screens/request/request_content_view.dart`

**Changes per file:**
- Replace `ContentActionBar(...)` with `CommentBar(...)`.
- Map the new proto fields to `CommentBar` props.
- **[AUDIT] Keep `unreadCountProvider`** — pass the live unread count from
  `unreadCountProvider` (not the proto field) to `CommentBar.unreadCount`.
  This preserves instant optimistic delta updates (see Recommendation note).
- Keep the `MorphingActionButton` as the `actionButton` prop — it still handles
  Review/Request/Requested/On Loan states.
- The helper/attendee section is removed from the action bar. If helper count
  is still needed, move it into the metadata/body area of the content view.
- **[AUDIT] Data flow must go through ViewModels.** Per architecture.md § Key
  Architectural Rules, screens never access repositories directly. The content
  view screens already get item data via their ViewModels (e.g.,
  `GearViewModel.state.gear`). The new proto fields (`lastMessageText`,
  `recentCommenters`, etc.) arrive through the same ViewModel state — no new
  repository access from screens is needed.

### Phase 5: Time-ago formatting and i18n
**Goal:** Format `last_message_at_unix_sec` into a relative time string; add all
new ARB keys.

**Options for time-ago:**
- (A) Server computes `last_message_time_ago` string (like portfolio inbox does).
- (B) Client computes from unix timestamp.

**Recommendation:** Server-side (option A) — add `optional string last_message_time_ago`
to each proto. This is consistent with the portfolio inbox pattern and avoids
client-side time drift issues. Replace `last_message_at_unix_sec` with this
string field to keep the proto simpler.

> **[AUDIT] i18n: The server-generated time-ago string (e.g., "2h ago") is an
> acceptable non-localized token** because it is a compact format identical across
> locales in our supported set (en, es both use "2h"). If locale-sensitive time
> formatting is needed later, the client can compute from the unix timestamp
> using `intl` package DateFormat.

**i18n strings (this phase):**
- Add `commentBarPlaceholder`, `commentBarUnread` (ICU plural),
  `commentBarViewAll` to both `app_en.arb` and `app_es.arb`.
- Run `npm run generate` to regenerate `AppLocalizations`.

### Phase 6: Tests (client + server)
**Goal:** Full test coverage for the new widget and server enrichment.

**Client tests:**
- `app/test/presentation/widgets/content/comment_bar_test.dart`
  - Empty state: shows placeholder text, chat icon, action button
  - Unread state: shows avatar stack, author, timestamp, message preview,
    unread count in coral, action button
  - All-read state: shows avatar stack, muted preview, "View all N comments",
    action button
  - Tap on comment area calls `onCommentTap`
  - Long message text is truncated with ellipsis

> **[AUDIT] Widget tests must include localization delegates** (per i18n.md §
> Test localization). Use the `MaterialApp` wrapper from
> `app/test/helpers/l10n_helpers.dart` that provides `localizationsDelegates`
> and `supportedLocales`.

**Server tests:**
- Test the `EnrichWithCommentPreview` helper with: no conversation, conversation
  with messages, conversation with unread messages
- Test that list endpoints populate preview fields without N+1 queries
  (use `storage.AssertMaxQueries`)

### Phase 7: Cache invalidation wiring
**Goal:** Ensure comment previews stay fresh after chat mutations.

> **[AUDIT] Added phase.** Without this, sending a message from a content view
> leaves the denormalized `last_message_*` fields stale in the cached item.

- `ChatRepository.sendMessage()` should notify a cache invalidation provider
  (e.g., `gearCacheInvalidationProvider` or a new generic
  `contentCacheInvalidationProvider`) so that the content view ViewModel
  triggers an in-place refresh — same pattern as
  `portfolioCacheInvalidationProvider` (see caching.md § Pattern 7).
- Content view ViewModels (GearViewModel, ExperienceViewModel, RequestViewModel)
  listen to the invalidation provider and call `_refreshInPlace()` with
  debounce/cooldown to refetch item data.
- This ensures `last_message_text`, `recent_commenters`, and `message_count`
  update within seconds of sending or receiving a message.

### Phase 8: Cleanup
**Goal:** Remove dead code.

- **[AUDIT] Keep `ContentActionBar`** — `community_content_view.dart` still uses
  it. Do NOT delete the widget or its tests. Only remove imports from the three
  migrated content views.
- **[AUDIT] Keep `unreadCountProvider`** — it is still the source of truth for
  live unread counts across the app (portfolio, inbox, content views). Do NOT
  remove its usage from content views.
- Remove any dead helper/attendee-related code from the three migrated content
  view `_buildActionBar()` methods.

## Phase Summary

| Phase | Scope | Files | Depends On |
|-------|-------|-------|------------|
| 1 | Proto changes | 3 proto files | — |
| 2 | Server enrichment (`server/conversation/` shared lib) | gear/experience/request services | Phase 1 |
| 3 | CommentBar widget + ARB keys | 1 widget + 2 ARB files | Phase 1 (for types) |
| 4 | Content view integration (keep `unreadCountProvider`) | 3 content view files | Phases 2 + 3 |
| 5 | Time-ago formatting + i18n | proto + server helper + ARB | Phase 1 |
| 6 | Tests (with l10n delegates) | test files (client + server) | Phases 3 + 4 |
| 7 | Cache invalidation wiring | ChatRepository + ViewModels | Phase 4 |
| 8 | Cleanup (keep ContentActionBar + unreadCountProvider) | various | Phase 7 |

Phases 2, 3, and 5 can be developed in parallel after Phase 1.
