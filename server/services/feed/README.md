# services/feed

The `feed` service implements the FeedService RPC interface: surfacing the community activity feed (stories from recent transactions and events) and the nudge system (AI-generated prompts that encourage sharing when the feed is sparse).

## Key files

- `service.go` — service struct and constructor.
- `stories.go` — `GetFeedStories`: assembles the paginated activity feed from the story storage.
- `nudges.go`, `feed_nudges.go` — nudge generation, scoring, and delivery.
- `feed_status.go` — per-user feed staleness and freshness metadata.
- `feed_dedup.go` — deduplication of story entries across pagination.
- `nudge_storage.go` — low-level nudge persistence helpers.
- `consume_nudge.go` — marks a nudge as consumed when the user acts on it.
- `terminator_pool.go` — goroutine pool for bounded async nudge generation.

## When to add code here vs. elsewhere

Feed and nudge logic lives here. Story creation (triggered by transfer completions, experience conclusions, etc.) lives in `server/story`. The `StoryStorage` interface used for reads lives in `server/storage`.
