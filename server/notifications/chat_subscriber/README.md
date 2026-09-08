# chat_subscriber

`server/notifications/chat_subscriber` is the push-notification consumer of
`server/chat_event_bus`. It mirrors the layout of
`server/notifications/community_subscriber/`.

## What it does

The subscriber receives `*chat_event_bus.PublishedEvent` from the chat bus and,
for `KindUserMessage` events, sends FCM push notifications to eligible
participants.

### Handle flow

1. Short-circuit: only `KindUserMessage` events generate push. System messages,
   reaction updates, and coalesced updates are silently skipped.
2. Short-circuit: `OnBehalfOf=true` events are skipped — their notification is
   emitted by the caller's own flow.
3. Soft-delete gate: if the parent community is soft-deleted, skip. Conversations
   with no `community_id` (private DM-style) bypass this gate.
4. Recipient resolution (`recipients.go`): for community-wide conversations
   (topic = `CommunityId`, empty `ParticipantIds`) fan out to all members; for
   topic-scoped conversations use `ParticipantIds`. Sender is excluded.
5. Preference gate: check `NOTIFICATION_CATEGORY_CHATS` per recipient. Fail
   open on lookup error.
6. Suppression: skip users with an active stream + foreground app (hooks wired
   in `main.go` via `SetStreamChecker` / `SetForegroundChecker`).
7. Copy assembly (`copy.go`): build `models.Notification` with mention title
   customization and 100-char preview truncation.
8. FCM dispatch via `notifications.Service.NotifyUser`.

## Late-bound hooks

`StreamChecker` and `ForegroundChecker` are set after construction from
`main.go`, because the chat service's stream registry doesn't exist until after
`chat.New(...)` is called.

## File layout

| File               | Purpose                                              |
|--------------------|------------------------------------------------------|
| `subscriber.go`    | `Subscriber` type, `Handle` implementation, hooks    |
| `recipients.go`    | `resolveRecipients` (community-wide vs participant)  |
| `copy.go`          | `buildNotification`, mention title, text truncation  |
| `*_test.go`        | unit and bus-integration tests                       |
