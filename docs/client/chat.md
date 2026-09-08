---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Client chat subsystem — bidirectional streaming messaging, inline workflow/status cards, per-user read tracking, presence-based notification management.
  globs: [app/lib/**/chat/**, server/chat/**]
  triggers: [chat, messaging, conversation, stream, read-tracking, system-message]
  lens: [client, domain]
  domain: client
freshness:
  verified_commit: "f4c5cf097"
  verified_on: "2026-07-30"
---
# Chat System

## Overview

The Ripls chat system enables real-time messaging for community members coordinating around shared items (transfers), help requests, and experiences. Conversations are the primary coordination surface: workflow actions, status cards, and completion summaries appear inline alongside messages, so users rarely need to leave the chat to manage their items.

The system uses bidirectional streaming for instant message delivery, per-user read tracking, and intelligent notification management based on user presence.

## Architecture Principles

**Context-Based Conversations:** Every conversation is anchored to a specific context (gear loan/giveaway, request, experience, or community), creating focused discussions around actionable items.

**Content-Embedded Chat:** The chat thread renders inline inside each content view's Chat tab, alongside the item's other tabs. Workflow status (RSVP, recipient selection, completion) surfaces as system messages in the timeline, while the primary workflow CTA lives on the content view's action bar rather than inside the chat thread.

**Real-Time Streaming:** Clients establish persistent gRPC streams to receive new messages instantly without polling, with automatic fallback to push notifications when streams are inactive.

**Per-User Read Tracking:** Each message maintains individual read status for every participant, enabling accurate unread counts and archive management.

**Smart Notifications:** The server tracks active streams and user presence to avoid sending duplicate notifications to users already viewing conversations.

### Conversation Scoping per Item Type

The conversation ID now lives on each primary entity (migrated off the `Community*`
junction rows, which keep their `conversation_id` fields only as deprecated
back-compat). Each item type uses a different scoping strategy:

| Item type | Conversation scope | Conversation ID source |
|-----------|-------------------|----------------------|
| Loan / Giveaway | **One conversation per gear**, shared across loans and giveaways of that gear | `Gear.conversation_id` (`proto/ripls/models/gear.proto:79`); topic carries `gear_id` |
| Request | One conversation per request | `Request.conversation_id` (`proto/ripls/models/request.proto:88`); topic carries `request_id` |
| Experience | **One global conversation for all attendees** | `Experience.conversation_id` (`proto/ripls/models/experience.proto`); topic carries `experience_id` |
| Community | One conversation per community | `Community.conversation_id` (canonical on `Gear`-style migration); topic carries `community_id` |

`GetConversationForTransfer` looks up the transfer's `gear_id`, then reads
`Gear.conversation_id` — both loans and giveaways resolve to the same gear-scoped
conversation rather than a transfer-specific one. A conversation's topic is the
`ConversationTopic` oneof (`proto/ripls/models/conversation_topic.proto`:
`transfer_id` / `request_id` / `experience_id` / `gear_id` / `community_id`).

Experience conversations are deliberately global: attendees from different
communities are all attending the same event and benefit from a shared
coordination space. The Chat tab in `ExperienceScreen` and the portfolio inbox
both use `Experience.conversation_id` for consistency.

See [sharing.md](./sharing.md#conversations-and-community-sharing) for the full conversation model table.

## Chat Data Flow

### Complete Messaging Flow

```
User opens the Chat tab of a content view (gear / request / experience / community)
    |
InlineConversationView lazily initializes the conversation
    |
ConversationViewModel.initialize()
    |-- Fetch conversation context (transfer/request/experience details)
    |-- Load message history via ChatRepository
    |-- Mark messages as read
    +-- Start real-time message stream (with reconnection)
    |
User types and sends message
    |
ConversationViewModel.sendMessage()
    |
ChatRepository.sendMessage() [gRPC call]
    |
Server: ChatService.SendMessage()
    |-- Validate user is participant
    |-- Create ChatMessage with per-user read status
    |-- Save to PostgreSQL
    |-- Broadcast to active streams
    +-- Send push notifications to offline participants
    |
Real-Time Delivery:
    |-- Active streams receive via StreamMessages RPC
    |-- Offline users receive push notification
    +-- ConversationViewModel updates local state
    |
Message appears in UI instantly
```

---

## Client Implementation

Conversations are no longer a standalone inbox + full-screen reader. The chat
thread renders **inline** inside the Chat tab of each content view (gear,
request, experience, community), and cross-community activity lives on the Home
tab (see [inbox.md](inbox.md)), which links into those content tabs.

### Cross-community activity (Home tab)

Cross-community activity is surfaced by the **Home tab** ("Needs you" + the
community pulse), documented in [inbox.md](inbox.md) — there is no standalone
chat inbox screen (the legacy `DailyItem`-based `PortfolioInboxScreen` was
removed in #2020). Home rows link into each item's content view, opening its
Chat tab when there are unread messages.

### Inline Conversation View

**File:** [inline_conversation_view.dart](../../app/lib/presentation/widgets/content/inline_conversation_view.dart)

**Purpose:** Renders the full conversation experience inside the Chat tab of a
content view. There is no dedicated `ConversationScreen`; the request, experience,
and community screens embed this widget via their `*_chat_pane` wrapper, and
`gear_content_view.dart` embeds it directly.

**Key Features:**

- **Collapsed / expanded modes:** Collapsed mode shows the last three messages and
  a simple input; tapping (or focusing the field) expands to the full
  [MessageList](../../app/lib/presentation/widgets/chat/message_list.dart) plus a
  pill-style compose bar (attach + mention-aware text field + send).
- **Message History:** Scrollable list of user messages and system events,
  rendered newest-at-bottom (`reverse: true`).
- **Real-Time Updates:** New messages appear instantly via streaming; a
  disconnected banner appears when the stream drops and reconnection is retrying.
- **Mentions, reactions, edit/reply/delete:** The compose field is a
  `MentionableTextField`; messages support emoji reactions and an edit/reply/delete
  long-press menu.
- **Optimistic Updates:** Messages appear immediately in a "pending" state, then
  update to "delivered" when the server confirms.
- **Auto-Scroll:** Scrolls to the newest message on first load and when new
  messages arrive.
- **Media attachments:** Pending image/video attachments can be staged and sent
  with a message.

The primary workflow CTA and participant/chat affordances are **not** inside this
view — they live in the content view's `ContentActionBar` (see
[Workflow Integration](#workflow-integration)).

**State Management:**

The conversation uses [ConversationViewModel](../../app/lib/presentation/viewmodels/conversation_view_model.dart),
a composed notifier (`conversationProvider(conversationId)`) backed by helper
notifiers — `conversation_messages_notifier.dart` (stream + message list),
`conversation_attachments_notifier.dart` (staged media), and the
`conversation_{transfer,request,experience}_actions.dart` mixins (workflow
actions). Its [ConversationState](../../app/lib/presentation/viewmodels/conversation_state.dart)
maintains:

- The `ConversationItem` (`conversation`), its `unreadCount`, and the cached
  `ConversationContext`
- Cached `Transfer` / `Request` / `GetExperienceResponse` details
- Message list with delivery states (pending, delivered, failed)
- A single message stream with reconnection (`isStreamDisconnected`)
- RSVP status map (`rsvpStatusMap`) and transfer status map (`transferStatusMap`)
- Reply / edit compose state and pending attachments

**Message States:**

Messages progress through states managed by the ViewModel:

1. **Pending:** User sends message, added optimistically to local state with temporary ID
2. **Delivered:** Server confirms receipt, temporary message replaced with real ID
3. **Failed:** Network error or server rejection, message marked for retry

**Stream Lifecycle:**

`conversation_messages_notifier.startMessageStream()` establishes a single
persistent stream. The stream:

- Starts in `initialize()` after loading message history
- Receives new messages from all participants in real-time
- Reconnects automatically on transient errors; after exhausting retries it sets
  `isStreamDisconnected` (surfacing the in-view banner) and stops
- Handles duplicate detection using message ID tracking
- Refreshes workflow state when system messages arrive (e.g., RSVP, offer,
  recipient selection events)

**Context Loading:**

Conversations efficiently load context using the `GetConversationContext` RPC,
which returns lightweight summaries instead of full objects:

- Gear (loan/giveaway) conversations: gear details, owner info, transfer state, giveaway phase, selected recipient
- Request conversations: request details, requester info, offer count
- Experience conversations: event details, RSVP counts, organizer info

This avoids expensive repository calls for full gear/request/experience objects.

---

## Workflow Integration

Chat conversations embed workflow status directly in the message timeline, with
the primary workflow action surfaced on the content view's action bar (not inside
the chat thread). Two mechanisms cooperate: **system-message rendering** in the
timeline and the **content action bar** alongside it.

### System Message Rendering

The server sends standard system messages (e.g., "Thomas is going", "Alex offered
to help") for every workflow event. The client renders them in
[MessageList](../../app/lib/presentation/widgets/chat/message_list.dart):

- Most system messages render as a centered
  [SystemMessageRow](../../app/lib/presentation/widgets/chat/message_list/system_message_row.dart)
  — the resolved label (localized via `resolveSystemMessageText`) plus an optional
  reaction badge.
- `TIME_PROPOSED` messages render a tappable inline
  [PollBannerOrLabel](../../app/lib/presentation/widgets/chat/message_list/poll_banner_or_label.dart)
  when the conversation is an experience with a live poll; older proposals fall
  back to a plain label and open a read-only view of their own poll.

The collapsible-pill machinery (`SystemPill`, `SystemPillContentFactory`,
`ItemPillBody`) and the standalone workflow-card widgets (`RsvpInviteCard`,
`AttendanceCard`, `CompletionCard`, `GearInfoCard`, `GiveawaySelectionCard`,
`RequestCompletionCard`, `LoanActiveCard`, `LoanCompletionCard`, and the
glassmorphic `WidgetShell` that wrapped them) were removed once nothing but
their own widget tests referenced them. The production timeline renders
`SystemMessageRow` and `PollBannerOrLabel`; the anchored-card slots that fed the
workflow cards went with the `WorkflowWidgetFactory` dispatcher.

### Morphing action button (primary CTA)

The workflow CTA lives in the content view, not the chat thread. The community
content view supplies a
[MorphingActionButton](../../app/lib/presentation/widgets/chat/morphing_action_button.dart),
which renders a coral CTA, a sage confirmed pill, or a muted terminal pill, with
a chevron that opens an upward-anchored
[ActionDropdownMenu](../../app/lib/presentation/widgets/chat/action_dropdown_menu.dart)
overlay of role-specific secondary actions (tap outside to dismiss). The gear
"who's using" panel uses the same dropdown directly.

### Participant Badges on Message Bubbles

Received message bubbles show a small badge pill next to the sender's name indicating their workflow status:

- **Experiences:** RSVP status — "Going" (sage), "Maybe" (amber), "Can't Go" (grey)
- **Requests:** "Helping" (sage) for users who have offered to help
- **Giveaways:** "Interested" (blue) for users who expressed interest, "Selected" (sage) for the chosen recipient

Badges disappear for terminal states (completed, cancelled).

---

## Server Implementation

### Chat Service Architecture

**Files:** [server/services/chat/](../../server/services/chat/)

**Core Components:**

1. **Service Struct** ([service.go](../../server/services/chat/service.go)):
   - In-memory stream registry: maps conversation IDs to active client streams
   - User presence tracker: tracks foreground/background state for notification routing
   - Notification service integration for push notifications
   - Optional AI provider for conversation summarization

2. **Message Streaming** ([streaming.go](../../server/services/chat/streaming.go)):
   - `StreamMessages` RPC: establishes bidirectional stream per client
   - Sends historical messages immediately upon connection
   - Registers client for real-time broadcasts
   - Cleans up streams on disconnect

3. **Message Handling** ([messages.go](../../server/services/chat/messages.go)):
   - `SendMessage` RPC: validates, persists, and broadcasts new messages
   - `GetConversationHistory` RPC: retrieves paginated message history
   - `MarkMessagesRead` RPC: updates per-user read status

4. **Conversation Management** ([conversations.go](../../server/services/chat/conversations.go)):
   - `StartConversation` RPC: creates conversation for transfer/request/experience
   - `ListConversations` RPC: retrieves user's conversations with optional archival filter
   - `GetConversationContext` RPC: efficient context loading without full object fetches

5. **Authorization** ([authorization.go](../../server/services/chat/authorization.go)):
   - `requireParticipant`: verifies the user is a conversation participant
   - `collectSharedCommunityIDs`: resolves the communities a viewer shares with a conversation
   - Enforces privacy: users only see their own conversations

6. **Presence** ([presence.go](../../server/services/chat/presence.go)):
   - `UpdatePresence` RPC: tracks per-user foreground/background state
   - Exposes `HasActiveStream` and `IsUserInForeground` hooks used by the push-notification subscriber for smart routing

Push notification routing is **not** in this package. `SendMessage` publishes to the chat event bus (`server/chat_event_bus`); the subscriber in [`server/notifications/chat_subscriber`](../../server/notifications/chat_subscriber/) dispatches push to inactive participants, skipping users with an active stream or in the foreground via the hooks above. The subscriber is wired to the bus in `server/wiring.go`.

### System Messages

When workflow actions occur (RSVP, offer, recipient selection, completion, cancellation), the server inserts system messages into the conversation. The shared `SystemMessageWriter` ([server/chat/system_messages.go](../../server/chat/system_messages.go)) owns this — domain services (experience, request, transfer, planning) call into it rather than into the chat service directly, so no service-to-service coupling is required. It also publishes the inserted message to the chat event bus and coalesces rapid duplicate events.

System message types include:
- **Experience:** RSVP_YES, RSVP_MAYBE, RSVP_NO, EXPERIENCE_CREATED, TIME_PROPOSED, TIME_POLL_CANCELLED, DETAIL_CHANGED, COMPLETED, CANCELLED
- **Request:** REQUEST_CREATED, OFFERED, FULFILLED, CANCELLED
- **Transfer:** JOINED (interest expressed), LEFT (interest withdrawn), APPROVED (recipient selected), STARTED, COMPLETED, CANCELLED
- **Giveaway-specific:** GIVEAWAY_SHARED (gear listed for giveaway)

These system messages serve two purposes: they appear as visible status updates in the chat, and they act as **anchor points** for the client to render widget cards at the correct position in the timeline.

### Message Broadcasting

The server maintains an in-memory stream registry that maps conversation IDs to active client streams. When a user sends a message via `SendMessage`:

1. **Validate and Persist:** Check authorization, validate content, save to PostgreSQL
2. **Update Conversation:** Update last message timestamp on conversation record
3. **Publish to the Chat Event Bus:** Publish the message to `server/chat_event_bus`. Two subscribers consume it: the **stream subscriber** ([stream_subscriber.go](../../server/services/chat/stream_subscriber.go)) fans out to all active streams for this conversation via `broadcastMessage()`, and the **notification subscriber** ([server/notifications/chat_subscriber](../../server/notifications/chat_subscriber/)) dispatches push notifications to offline participants.

The stream fan-out uses a non-blocking channel send. If a stream buffer is full, the message will be retrieved via history instead, ensuring messages are never lost.

### Per-User Read Tracking

Each `ChatMessage` (defined in [chat.proto](../../proto/ripls/models/chat.proto)) contains a map of participant read status (`participant_id_to_is_read`). When a user opens a conversation, the client calls `MarkMessagesRead`. The server updates read status for all unread messages, enabling accurate per-user unread counts and archive/active filtering.

### Conversation Context Optimization

**File:** [context.go](../../server/services/chat/context.go)

`GetConversationContext` returns lightweight summaries instead of full objects. The `ConversationContext` message contains only essential fields needed for inbox display and workflow rendering. For giveaway conversations, this includes the server-computed `GiveawayPhase` and `selected_recipient` so all participants see the correct workflow state regardless of their own transfer status.

This reduces database queries, network payload size, and client-side processing compared to fetching full objects.

---

## Data Models

### ConversationItem

**Proto:** [chat_service.proto](../../proto/ripls/api/chat_service.proto)

Returned by `ListConversations` RPC, containing:
- Conversation ID and participants
- Last message timestamp and unread count
- Topic reference (transfer ID, request ID, or experience ID)
- Preview fields for UI (last message text and sender name)

There is no client-side wrapper model around `ConversationItem`. The
`InboxItem` model that used to play that role was deleted in #2814 once the
`GetConversationContext` migration finished: `ConversationState` now holds the
`ConversationItem` and its `unreadCount` directly, and everything else the
wrapper carried (a client-composed description string, a derived timestamp,
full `Transfer` / `Request` / `GetExperienceResponse` objects) had no reader
left. `InlineConversationView` fetches the `ConversationItem` and the
`ConversationContext` and passes both straight to
`ConversationNotifier.initialize`.

Active/done partitioning lives in the portfolio inbox feed and filter
providers — see [client/inbox.md](inbox.md), which is a separate,
server-assembled surface (`GetHomeView`) unrelated to this model.

### ChatMessage

**File:** [chat_message.dart](../../app/lib/presentation/models/chat_message.dart)

Wrapper around API `MessageHistoryItem` with delivery state tracking. Includes the message content, sender information, state (pending/delivered/failed), system action type, and whether it's a temporary optimistic update.

---

## Common Patterns

### Pattern 1: Starting a Conversation

**Use Case:** User expresses interest in a transfer or offers to help with a request.

**Client Flow:**

Repositories ([TransferRepository](../../app/lib/data/repositories/transfer_repository.dart), [RequestRepository](../../app/lib/data/repositories/request_repository.dart)) resolve the conversation for an item via the appropriate `GetConversationFor*` RPC, passing the item ID. The returned conversation ID is handed to the content view's Chat tab, where [InlineConversationView](../../app/lib/presentation/widgets/content/inline_conversation_view.dart) loads and streams it.

**Server Flow:**

The server ([conversations.go](../../server/services/chat/conversations.go)):

1. Validates the item exists (transfer/request/experience)
2. Checks if a conversation already exists between these participants
3. If exists, returns the existing conversation ID
4. If new, creates a conversation with participants:
   - Gear (loan/giveaway): owner + interested parties (the one gear conversation grows as users express interest)
   - Request: requester + offerers
   - Experience: organizer + attendees
5. Sends the appropriate creation/system message

### Pattern 2: Real-Time Message Updates

**Client Implementation:**

The conversation's messages notifier maintains a single stream subscription. When `startMessageStream()` is called, it subscribes to the chat repository's stream for this conversation. `_onStreamMessage` resets reconnect state and delegates to `_handleStreamedMessage`, which fans the response out to typed handlers (`_handleUserMessageUpdate`, `_handleSystemMessageUpdate`, `_handleReactionUpdate`, `_handleMessageDelete`), filtering duplicates and converting payloads to `ChatMessage` objects. When a system message arrives that indicates a workflow state change (e.g., OFFERED, APPROVED, COMPLETED), the notifier also refreshes the relevant workflow context so the action bar and timeline stay current.

**Server Implementation:**

The server maintains an in-memory registry of active streams. When `SendMessage` is called, it validates and persists the message, then publishes it to the chat event bus. The stream subscriber fans the event out to all active streams for this conversation using `broadcastMessage()`.

### Pattern 3: Optimistic Message Sending

ConversationViewModel's `sendMessage` method implements optimistic updates:

1. **Create Temporary Message:** Generate a temporary message with a UUID-based ID and `MessageState.pending`
2. **Add to UI Immediately:** Append the temporary message to the state's message list (instant feedback)
3. **Send to Server:** Call `chatRepository.sendMessage()` asynchronously
4. **Replace on Success:** When the server responds, replace the temporary message with the real one and update state to `MessageState.delivered`
5. **Mark on Failure:** If the network request fails, update the temporary message's state to `MessageState.failed`

### Pattern 4: Smart Notification Routing

The notification subscriber (`server/notifications/chat_subscriber`) implements intelligent notification routing as it consumes each chat bus event:

1. **Skip Sender:** Don't notify the message sender
2. **Skip Active Streams:** Check if the participant has an active stream for this conversation (via the `HasActiveStream` hook)
3. **Skip Foreground Users:** Check if the user is currently in the app foreground (via the `IsUserInForeground` hook)
4. **Send Push Notification:** Only send to participants who are truly offline

### Pattern 5: Workflow State Refresh on System Messages

When the message stream delivers a workflow-relevant system message (RSVP change, offer, recipient selection, completion, cancellation), the ViewModel automatically refreshes the conversation context. This ensures the timeline and the content action bar always reflect the current state, even when another participant triggers the change.

The refresh is non-blocking — it happens in the background while the system message itself appears immediately in the chat timeline.

---

## Performance Characteristics

### Stream Scalability

**Memory Usage per Stream:**
- Go channel: ~50 bytes
- Stream metadata: ~100 bytes
- **Total per stream:** ~150 bytes

**Typical Load:**
- 1,000 concurrent users
- Average 2 active conversations per user
- **Total streams:** 2,000 streams x 150 bytes = ~300 KB

**Message Broadcast Performance:**
- O(N) where N = number of active streams for conversation
- Typical conversation: 2-5 participants (giveaways can be larger)
- Broadcast latency: <1ms for small groups

### Database Query Optimization

**Conversation List Loading:**

`GetConversationContext` uses a single query with LEFT JOINs to fetch flattened context fields, returning conversations with embedded transfer/request context in one round-trip. This is ~10x faster than querying full objects per conversation.

### Unread Count Tracking

**Client-Side Cache:**

UnreadCountRepository ([unread_count_repository.dart](../../app/lib/data/repositories/unread_count_repository.dart)) reads per-conversation and per-community unread counts from the server (`getUnreadCount` / `getUnreadCounts` / `getUnreadCountsByCommunity`), backed by a cache over the `GetUnreadCounts` RPC. Counts are refreshed when:

1. **Inbox / chat loads:** counts are fetched (and the cache refreshed) on load
2. **Real-time updates:** new stream messages bump the relevant count
3. **Mark as read:** opening a conversation calls `MarkMessagesRead`, zeroing its count

---

## Testing

### Client Tests

**ConversationViewModel Tests** ([conversation_view_model_test.dart](../../app/test/presentation/viewmodels/conversation_view_model_test.dart)):
- Message history loading
- Real-time message streaming
- Optimistic message sending
- Message failure handling
- Context loading
- Workflow action execution (RSVP, offer, express interest, completion)
- Disposal safety for async workflow methods

**System-Message Widget Tests:**
- `SystemMessageRow` and `PollBannerOrLabel` — the live render path for system
  messages and inline time-poll banners.

**Repository Tests:**
- `ChatRepository`: Verify gRPC calls with correct parameters
- `UnreadCountRepository`: Test cache synchronization
- Mock `ChatService` with Mockito

### Server Tests

**Message Tests** ([messages_test.go](../../server/services/chat/messages_test.go)):
- Send message with authorization
- Message persistence and retrieval
- Per-user read status initialization

**Streaming Tests** ([streaming_test.go](../../server/services/chat/streaming_test.go)):
- Stream registration and cleanup
- Message broadcasting to multiple clients
- Historical messages on stream start

**Context Tests** ([context_test.go](../../server/services/chat/context_test.go)):
- Context loading for transfers, requests, and experiences
- GiveawayPhase computation for giveaway conversations
- Efficient query verification

---

## Data Migration: Per-Community to Global Experience Conversations

Before the inbox was updated to use `Experience.conversation_id`, the server populated `CommunityExperience.conversation_id` (one per community share) and chat messages were stored against those per-community IDs. Any chat history from that period lives on the wrong conversation ID and won't be visible in the current Chat tab or inbox.

### Why a Go migration (not pure SQL)

The `binary_proto` column stores the full serialized proto blob for each message, including `ConversationId`. A pure SQL `UPDATE` can change the scalar `conversation_id` column, but the blob must be decoded, mutated, and re-encoded in Go — there's no SQL way to re-serialize a protobuf.

### Migration steps

1. **Ensure every experience has a global conversation ID.** If `Experience.conversation_id` is empty, create a new `ChatConversation` row and write the ID back to the experience.

2. **For each experience:** load all `CommunityExperience` rows. For each per-community `conversation_id` that differs from the global ID, load all `ChatMessage` rows in that conversation.

3. **Skip creation anchors** (`SystemMessage_ExperienceCreated`) — these will be recreated or are already present on the global conversation.

4. **Skip messages already on the global conversation** (check `ConversationId == exp.ConversationId`) — makes the migration idempotent.

5. **For each message to move:** decode `binary_proto`, set `ConversationId = exp.ConversationId`, re-marshal, and `UPDATE` both the `conversation_id` scalar column and the `binary_proto` column in a single statement.

6. **Batch updates per experience** inside a transaction. Roll back the entire experience batch on error.

7. **Optionally union participant lists:** after all messages are moved, merge the `participant_ids` from the per-community `ChatConversation` rows into the global one.

8. **Leave `CommunityExperience.conversation_id` rows in place** — backward-compatible; the server simply ignores them.

### Verification

After running the migration, confirm that:
- The global `ChatConversation` has all expected participant IDs.
- Message counts in the per-community conversations match those now in the global conversation.
- The Chat tab on existing experiences shows all historical messages.

---

## Resources

### Internal Files

**Client — Chat UI:**
- [inline_conversation_view.dart](../../app/lib/presentation/widgets/content/inline_conversation_view.dart) — Inline chat thread inside a content view's Chat tab
- [conversation_view_model.dart](../../app/lib/presentation/viewmodels/conversation_view_model.dart) — Conversation state (composed notifier)
- [conversation_messages_notifier.dart](../../app/lib/presentation/viewmodels/conversation_messages_notifier.dart) — Message stream + list management
- [conversation_state.dart](../../app/lib/presentation/viewmodels/conversation_state.dart) — Conversation state object
- [chat_repository.dart](../../app/lib/data/repositories/chat_repository.dart) — Data access layer
- [chat_service.dart](../../app/lib/services/chat_service.dart) — gRPC client
- [chat_message.dart](../../app/lib/presentation/models/chat_message.dart) — Message model

**Client — Action Buttons:**
- [morphing_action_button.dart](../../app/lib/presentation/widgets/chat/morphing_action_button.dart) — Coral CTA → sage confirmed pill with dropdown
- [action_dropdown_menu.dart](../../app/lib/presentation/widgets/chat/action_dropdown_menu.dart) — Upward-anchored overlay dropdown

**Client — Messages:**
- [message_list.dart](../../app/lib/presentation/widgets/chat/message_list.dart) — Scrollable message list (renders bubbles, system rows, poll banners)
- [message_list/system_message_row.dart](../../app/lib/presentation/widgets/chat/message_list/system_message_row.dart) — Centered system-message label + reaction badge (live render path)
- [message_list/poll_banner_or_label.dart](../../app/lib/presentation/widgets/chat/message_list/poll_banner_or_label.dart) — Inline live time-poll banner for `TIME_PROPOSED`
- [system_message_text_resolver.dart](../../app/lib/presentation/widgets/chat/system_message_text_resolver.dart) — Resolves a structured `SystemMessage` to locale-appropriate display text

**Server:**
- [service.go](../../server/services/chat/service.go) — Service struct and dependencies
- [streaming.go](../../server/services/chat/streaming.go) — Real-time message streaming
- [messages.go](../../server/services/chat/messages.go) — Send/receive message handlers
- [conversations.go](../../server/services/chat/conversations.go) — Conversation management
- [context.go](../../server/services/chat/context.go) — Efficient context loading
- [presence.go](../../server/services/chat/presence.go) — Presence tracking + stream/foreground hooks
- [stream_subscriber.go](../../server/services/chat/stream_subscriber.go) — Chat event bus → live stream fan-out
- [authorization.go](../../server/services/chat/authorization.go) — Access control
- [server/notifications/chat_subscriber/](../../server/notifications/chat_subscriber/) — Push notification routing (separate package, subscribes to the chat event bus)

**Protocol Definitions:**
- [chat_service.proto](../../proto/ripls/api/chat_service.proto) — Chat RPC API
- [conversation.proto](../../proto/ripls/api/conversation.proto) — Conversation types
- [chat.proto](../../proto/ripls/models/chat.proto) — Storage models

**Related Docs:**
- [manage_menus.md](./manage_menus.md) — Workflow manage-menu sheets (experience / request / gear)
- [time.md](./time.md) — Time-poll sheets and inline poll banners
- [sharing.md](./sharing.md) — Community sharing and the conversation model
- [architecture.md](./architecture.md) — Overall MVVM client architecture
- [server/architecture.md](../server/architecture.md) — Server design principles

---

**Last Updated:** 2026-06-09
