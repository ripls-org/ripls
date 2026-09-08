// Package chat_event_bus is the ChatMessage-specific publisher built on
// top of server/pubsub. Unlike community_event_bus, it does NOT insert any
// storage row — the ChatMessage audit row is written by the calling RPC before
// Publish is invoked. The bus owns pre-fetch of denormalized context (sender
// User, transfer for transfer-topic deep links) and fan-out to subscribers.
// See README.md for the contract.
package chat_event_bus // snake_case package name matches the docs/issues/1842 plan.
