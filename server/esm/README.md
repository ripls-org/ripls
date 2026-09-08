# server/esm — ESM shared library

Storage queries, response aggregation, and prompt-authoring helpers for
Experience Sampling Methodology. Consumed by the ESM service
(`server/services/esm/`), the experience service's recap-story
materialization, and the workshop momentum detector — the multi-consumer
shape is what justifies the shared library and avoids service-to-service
injection.

## Files

| File | Purpose |
|------|---------|
| `prompts.go` | `GetPrompt`, `QueryPromptsByExperience`, `InsertPrompt` storage helpers for `StoredESMPrompt` |
| `responses.go` | `GetResponseForUser`, `QueryUnresolvedForUser`, `QueryResponsesByPrompt`, `InsertResponseRows`, `UpdateResponse` storage helpers for `StoredESMResponse` |
| `aggregation.go` | `Aggregate(rows, completedAtUnixSec) []*Signal` — pure function with no storage access; unit-tested in isolation. `AggregateAttendeesOnly` filters to `respondent_was_attendee == true`. |
| `story_embed.go` | Read-side enrichment: produces the `EmbeddedEsmPrompt` payload mounted on `StoryPayload` for the recap story embed. |
| `authoring.go` | `AttendeeUserIDs(experienceID)` and `BuildPromptForExperience(experienceID, draft, now)` helpers used by the experience service's recap-story materialization (and, in v2, the automated trigger) |

## When to add code here

- New storage queries against `StoredESMPrompt` / `StoredESMResponse`.
- New aggregation modes (e.g., per-circle aggregations, time-window
  aggregations) — keep the existing `Aggregate` function pure; layer new
  shapes alongside it.
- Helpers that more than one ESM consumer needs.

## When NOT to add code here

- RPC handlers (those go in `server/services/esm/`).
- Cross-service auth helpers (those go in `server/auth/` if shared, or
  `server/services/esm/authorization.go` if ESM-specific).
- Workshop cascade priority detection — that lives in
  `server/services/workshop/momentum/` and *consumes* the read helpers
  here. Keep the shared library cleanly scoped to ESM concerns.

## Consumers

| Caller | What it uses | Why |
|--------|--------------|-----|
| `server/services/esm/` | `GetPrompt`, `GetResponseForUser`, `QueryResponsesByPrompt`, `UpdateResponse`, `InsertResponseRows` | RPC handler for response submission |
| `server/services/feed/` | `EmbeddedPromptForStory`, `SocialProofForStory` | Recap-story embed enrichment merged into feed assembly |
| `server/services/workshop/momentum/` | `QueryPromptsByExperience`, `QueryResponsesByPrompt`, `AggregateAttendeesOnly` | Workshop cascade priority (2): "recently completed event with strong ESM repeat signal ≥60%" |
| `server/services/experience/` (recap-story materialization) | `AttendeeUserIDs`, `BuildPromptForExperience`, `InsertPrompt`, `InsertResponseRows` | Inline ESM prompt materialization at experience completion |

## Forward-compatibility

The authoring helpers (`AttendeeUserIDs`, `BuildPromptForExperience`) are
the same code path the v2 automated trigger will call. The aggregation
and storage queries are stable across v1 and v2.
