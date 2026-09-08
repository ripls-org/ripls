# Story

The `story` package is a library for generating and storing community stories — the short human-readable entries that appear in the activity feed when a loan is completed, an experience concludes, a member joins, and so on. It is a library, not a service; the feed RPC that reads stories lives in `server/services/feed`.

## Key files

- `generator.go` — `Generator` and `CreateStory`: orchestrates template-based story creation with optional AI enrichment. Implements the `StoryCreator` interface.
- `templates.go` — text and title templates for each story type (loan completed, giveaway received, experience concluded, etc.).
- `types.go` — `CreateStoryRequest`, `StoryCreator` interface, and story-type constants.

## When to add code here vs. elsewhere

Add a new story type by adding a template entry in `templates.go` and a new `StoryType` constant in `types.go`. The service that triggers story creation (e.g. transfer service on loan completion) injects a `StoryCreator` and calls `CreateStory`. Feed delivery and pagination belong in `server/services/feed`. Storage for stories belongs in `server/storage` (`StoryStorage`).
