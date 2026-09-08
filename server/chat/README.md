# Chat

The `chat` package is a shared library for conversation management — creating conversations, writing system messages, tracking unread counts, and handling @mentions. It is a library, not an RPC service; the RPC surface lives in `server/services/chat`.

## Key files

- `conversations.go` — `CreateOrGetConversation`: idempotent conversation creation keyed by topic (transfer, request, experience, or gear).
- `system_messages.go` — `SystemMessageWriter`: inserts server-authored or on-behalf-of user messages into conversations.
- `mentions.go` — mention parsing and storage helpers.
- `unread.go` — unread-count computation per user per conversation.
- `topic.go` — `TopicQueryField`: maps a `ConversationTopic` oneof to the protosql column name used for queries.

## When to add code here vs. elsewhere

Add code here when multiple services need to share conversation logic (e.g. both the transfer and experience services create conversations). RPC handler logic, streaming, and message-read RPCs belong in `server/services/chat`. Business rules tied to a single entity type (e.g. transfer-specific message text) belong in the owning service and should call this library.
