# services/experience

The `experience` service implements the ExperienceService RPC interface: creating, editing, and managing community experiences (events, activities, and skill shares). It handles AI-generated content from text, images, and web URLs; RSVP and attendance tracking; needs and contributions; lifecycle transitions (upcoming → active → concluded); and impact estimation at conclusion.

## Key files

- `service.go` — service struct and dependency wiring.
- `save.go` — `SaveExperience`: create or update an experience with geocoding and stock imagery.
- `gen_ai.go`, `gen_ai_streaming.go`, `gen_ai_fanout.go` — AI generation from text/image/webpage, streaming variant, and async fan-out.
- `lifecycle.go` — `CompleteExperience`, `CancelExperience`, `DeleteExperience`.
- `rsvp.go` — RSVP and attendance management.
- `needs_and_contributions.go` — planning needs/contributions for experiences.
- `time_proposals.go` — proposed time slot negotiation.
- `stock_imagery.go`, `webpage_image.go` — stock imagery and web-scraped imagery helpers.
- `undo.go` — undo support for experience actions.
- `stats.go`, `people.go`, `preview.go`, `share.go` — ancillary read and sharing RPCs.

## Parallel LLM calls in SaveExperience

`SaveExperience` (create branch) runs two LLM calls in parallel and blocks until both complete before returning the response. The response therefore always contains LLM-refined fields rather than config defaults.

1. **InferSocialAttributes** — refines the `ImpactEstimate` with vulnerability and duration inferred from the experience text.
2. **GenerateExperienceSuggestions** — produces category-specific suggestion chips (`Suggestions` + `CategoryHint`).

Both calls share the request context and each has a 30-second `context.WithTimeout`. A `sync.WaitGroup` ensures the response is not sent until both complete (or time out). Results are applied to the in-memory experience struct and persisted in a single `storage.Update` before the response is built.

The lazy-fill path in `ListExperienceNeedsAndContributions` (`needs_and_contributions.go`) remains a safety net for cases where both calls time out.

## When to add code here vs. elsewhere

Experience RPC logic lives here. Cross-entity planning helpers (needs/contributions) live in `server/planning`. Requests (borrow/help) that are not events belong in `server/services/request`. Community events like RSVP notifications share patterns with `server/services/community`.
