// Package chat_subscriber is the push-notification consumer of chat_event_bus.
// One instance is registered on the chat event bus; it owns recipient
// resolution, preference gating, soft-delete gating, copy assembly, and FCM
// dispatch for chat message push notifications.
package chat_subscriber
