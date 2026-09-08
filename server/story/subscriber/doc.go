// Package story_subscriber implements the story-creation subscriber for
// community_event_bus. It listens for terminal-state community events
// (TRANSFER_COMPLETED, REQUEST_FULFILLED, EXPERIENCE_COMPLETED,
// INVITATION_LINK_USED) and calls story.Creator.CreateStory with the
// appropriate denormalized payload. See README.md for the contract.
package story_subscriber // snake_case package name matches the docs/issues/510 plan.
