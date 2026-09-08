# Conversation

The `conversation` package provides shared helpers for reasoning about conversation state relative to its associated item (transfer, request, or experience). It is a library, not a service; callers are typically the feed service and any code that needs to archive or display conversations.

## Key files

- `state.go` — `IsItemDone` and `IsItemDoneFromMaps`: determine whether the item a conversation is attached to has reached a terminal state (completed, fulfilled, or cancelled). The `FromMaps` variant avoids per-conversation database queries when iterating over many conversations.
- `comment_preview.go` — utilities for generating short conversation previews (the last-message snippet shown in list views).

## When to add code here vs. elsewhere

Add cross-cutting conversation helpers here when they are needed by more than one service and have no dependency on RPC types. The full RPC surface (send message, list messages, streaming) lives in `server/services/chat`. Conversation creation and topic mapping live in `server/chat`.
