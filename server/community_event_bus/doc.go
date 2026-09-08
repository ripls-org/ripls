// Package community_event_bus is the CommunityEvent-specific publisher built on
// top of server/pubsub. It owns the storage row insert and pre-fetches
// denormalized entity context (gear, transfer, request, experience, actor)
// referenced by the event so subscribers don't fan out N+1 reads. See
// README.md for the contract.
package community_event_bus // snake_case package name matches the docs/issues/510 plan.
