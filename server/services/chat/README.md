# services/chat

The `chat` service implements the ChatService RPC interface: sending and listing messages, reactions, @mentions, streaming new events, conversation authorization, and AI-generated conversation summaries.

## Key files

- `service.go` — service struct, constructor, and authorization helpers.
- `messages.go` — `SendMessage`, `ListMessages`.
- `conversations.go` — `ListConversations`, conversation authorization.
- `streaming.go` — `StreamConversationEvents`: server-sent real-time message delivery; `StreamSubscriber` fan-out via `chat_event_bus`.
- `stream_subscriber.go` — `chatStreamSubscriber` wires the chat bus to open SSE streams.
- `reactions.go` — message reactions (add/remove).
- `summary.go` — AI-generated conversation summaries.
- `mention_handler.go` — parses and stores @mentions from message content.
- `unread_counts.go` — `GetUnreadCounts` RPC.
- `message_converters.go` — conversion between storage and API message types.

## Message ordering invariant

Any code path that returns a list of `ChatMessage`s for user display must use `storage.ListByConversation` (defined in `server/storage/chat_message.go`), which includes `ORDER BY sent_at_unix_sec ASC, id ASC`. The bare `storage.QueryByField(..., "conversation_id", ...)` returns PostgreSQL heap order — unrelated to insertion time — and must not be used for user-visible message lists. See #1826 for the bug that heap order caused.

The invariant assumes `sent_at_unix_sec` is ≤ wall-clock time at insert. Producers must enforce that; the client and storage layer do not clamp. A future-stamped row will sort to the bottom of the conversation, render with a "now" relative-time label (the timeago library collapses any future timestamp to the same bucket), and confuse the user into thinking newer content is older. See #1920 — the simulator used to produce such rows by forward-constructing lifecycle steps past `scenario.EndTime`; it now anchors every flow at a terminal time inside the window and derives earlier steps backward from there.

## Access control

Every RPC that reads or mutates a `ChatConversation` or its `ChatMessage` children must call `requireConversationAccess` (or `requireParticipant` where strict participant membership is required) before doing any work. The only exception is `StartConversation`, which uses `auth.RequireMemberOfActiveCommunity` because no conversation exists yet.

`requireConversationAccess` grants access when the caller is (a) already listed in `conversation.ParticipantIds`, or (b) a member of any active community the conversation's topic is shared with. It returns `CodeNotFound` when the conversation row does not exist and `CodePermissionDenied` when the row exists but the caller has no access right. See #2132 for the precedent: `SendMessage` was the only writer missing this gate, which allowed any authenticated user with a known `conversation_id` to inject messages.

## When to add code here vs. elsewhere

RPC handlers for chat belong here. Shared conversation primitives (topic mapping, system messages, unread count queries) belong in `server/chat`. AI provider calls for summarization are orchestrated from `summary.go` but the prompt and provider interface live in `server/ai`.

Push notifications for chat messages are not in this package. They are handled by `server/notifications/chat_subscriber`, which subscribes to `server/chat_event_bus` in `server/main.go`. The stream-checker and foreground-checker hooks (`HasActiveStream`, `IsUserInForeground`) are wired from this service into that subscriber after construction.
