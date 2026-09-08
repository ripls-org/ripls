# chat_event_bus

`server/chat_event_bus` is the ChatMessage-specific domain adapter on top of
`server/pubsub`. It is the second domain adapter in the project; see
`server/community_event_bus/README.md` for the first.

## Key difference from community_event_bus

`community_event_bus.Publish` inserts the audit row **and** dispatches.
`chat_event_bus.Publish` does **not** insert any row — the `ChatMessage` row
is written by the calling RPC before `Publish` is invoked. The bus's only
job is to:

1. Pre-fetch denormalized context (sender `User`, `Transfer` for
   transfer-topic deep links — `Conversation` is provided by the caller).
2. Fan out asynchronously to subscribers via the underlying
   `pubsub.MemTopic[*PublishedEvent]`.

## Event shape

`PublishedEvent` carries:

| Field              | Source                         | Notes                                   |
|--------------------|--------------------------------|-----------------------------------------|
| `Kind`             | caller                         | USER_MESSAGE, SYSTEM_MESSAGE, etc.      |
| `Message`          | caller                         | nil for KindReactionUpdate              |
| `Conversation`     | caller                         | always non-nil                          |
| `Sender`           | prefetched                     | nil on prefetch failure                 |
| `Transfer`         | prefetched (if transfer topic) | nil when no transfer topic              |
| `MentionedUserIDs` | WithMentionedUserIDs option    | for notification title customization    |
| `OnBehalfOf`       | WithOnBehalfOf option          | suppresses push in chat_subscriber      |
| `ReactionUpdate`   | WithReactionUpdate option      | populated for KindReactionUpdate        |

## Subscribers

Two subscribers are registered on the bus in `main.go`:

1. **`notifications/chat_subscriber`** — push notifications. Handles only
   `KindUserMessage`, skips `OnBehalfOf=true`, gates on community soft-delete
   and per-user `NOTIFICATION_CATEGORY_CHATS` preference, suppresses for
   users with active streams in the foreground.
2. **`services/chat.streamSubscriber`** — real-time stream fan-out. Handles
   all four kinds, converts `*PublishedEvent` → `*api.StreamMessagesResponse`,
   and fans out to all registered in-memory stream channels.

## Adding a new subscriber

1. Implement `pubsub.Subscriber[*chat_event_bus.PublishedEvent]`.
2. Construct it in `main.go` after `chatService` exists (stream registry
   is needed for the stream checker hook).
3. Call `chatEventBus.Subscribe(yourSubscriber)`.

## Topic name

`TopicName = "chat_messages"` appears as `topic="chat_messages"` on every
pubsub log line — use this field to filter in dashboards.

## See also

- `server/pubsub/README.md` — the generic bus primitive
- `server/community_event_bus/README.md` — the other domain adapter
