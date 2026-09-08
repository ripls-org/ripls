# ESMService

Implements the `ESMService` RPC interface for Experience Sampling
Methodology prompts. Recipients submit structured responses or dismissals
against a prompt; downstream signal aggregation happens server-internally.

## Files

| File | Purpose |
|------|---------|
| `service.go` | Service struct + `New()` constructor + handler interface assertion |
| `responses.go` | `RespondToESMPrompt` handler + option-key validation |
| `authorization.go` | `requireESMRespondentOrStoryViewer` (recipient eligibility, lazy non-attendee row insert) |

The package depends only on `server/storage` and the `server/esm` shared
library — no other RPC service is injected. Cross-service helpers (host
membership, community ownership) come from `server/auth/` if and when
they need to expand beyond experience-host checks.

## When to add code here

- New RPC handlers on the ESM service.
- Service-specific authorization helpers (the existing one is
  intentionally scoped to ESM).

Storage queries, response aggregation, and authoring helpers live in
[server/esm/](../../esm/) so other consumers (the experience service, the
workshop momentum detector) can reach them without a service-to-service
dependency. If you find yourself reaching for storage inside this package
directly, consider whether the helper belongs in the shared library
instead.

## Authoring path

In v1, ESM prompts are materialized inline with the
`STORY_TYPE_EXPERIENCE_CONCLUDED` recap story by the experience service
(see [server/services/experience/stories_esm.go](../experience/stories_esm.go)).
The shared helpers `esm.AttendeeUserIDs` and `esm.BuildPromptForExperience`
live in [server/esm/](../../esm/). v2's automated trigger
([docs/issues/1580-esm-cards.md](../../../docs/issues/1580-esm-cards.md))
will reuse the same helpers and add a template bank keyed to event
flavor.

**The `RespondToESMPrompt` RPC in this package does not change between
v1 and v2.** The producer side (authoring) evolves; the consumer side
(response submission) stays stable.

## Privacy boundary (re-stated)

- `RespondToESMPrompt` accepts only the caller's own `UNRESOLVED` row —
  no impersonation. Validated by `requireESMRespondentOrStoryViewer`.
- Aggregate signal is computed server-internally by the workshop
  momentum detector (`esm.AggregateAttendeesOnly`) and is never returned
  to a client over the wire. The resulting `WORKSHOP_QUEST_HERO` nudge
  is host-scoped via the existing `user_id` + `community_id` filter on
  `StoredNudge`.
