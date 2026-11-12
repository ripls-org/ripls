# #1580 ESM cards — Phase 4 privacy review

**Reviewed:** 2026-04-29
**Scope:** Manual ESM v1 (Phases 1–3 shipped). Verifies the privacy
constraints from [docs/users/priorities.md](../users/priorities.md) and
the issue body
([docs/issues/1580-esm-cards.md](1580-esm-cards.md)) hold across the
proto contract, the storage model, and the live RPC surface.

## Privacy contract (re-stated)

1. Responses stay inside the system; never visible to other circle
   members or to the public.
2. Aggregate signal can be surfaced to the host (e.g. "8 of 11 want this
   again") but **no per-user attribution** crosses the wire.
3. Free-text follow-ups are visible only to the experience host, never
   aggregated as a public surface, never visible to other respondents.
4. Prompts are about the event, never about the people. No questions
   that name another participant or evaluate relationships per se.
5. No leaderboards, no public displays, no per-person scores.

## Findings

### Pass — write-path data isolation

`RespondToESMPromptRequest` carries `response_option_key` and `free_text`
*from the recipient to the server only* — the request shape is one-way.
The server's `RespondToESMPromptResponse` is empty by design (an
acknowledgement); no response data is echoed back, including not to the
recipient who submitted it.

Verified: [proto/ripls/api/esm_service.proto](../../proto/ripls/api/esm_service.proto) lines 14–43.

### Pass — read-path aggregates only, no per-user fields

`ESMSignal` carries:

- `prompt_id` (identifier, not personal)
- `respondent_count` (count, no identity)
- `response_distribution: map<option_key, count>` (counts per option, no
  identity)
- `free_text_count` (count of *responses with free text*; not the text
  itself; not who wrote them)
- `completed_at_unix_sec` (experience timestamp, not personal)

No `user_id`, `email`, `name`, or any other identifying field appears in
`ESMSignal`. Free-text *content* is never returned by `GetESMSignal`.

Verified: [proto/ripls/api/esm_service.proto](../../proto/ripls/api/esm_service.proto) lines 47–69.

### Pass — host-only authorization on aggregate read

`GetESMSignal` is gated by `requireESMSignalReader` in
[server/services/esm/authorization.go](../../server/services/esm/authorization.go),
which loads the experience and rejects any caller whose `UserID` is not
the experience's `OwnerId`. Test coverage:
`TestGetESMSignal_RejectsNonHost` in `signal_test.go` confirms a non-host
gets `CodePermissionDenied`. No fallback paths.

### Pass — write authorization gated to targeted recipients

`RespondToESMPrompt` is gated by `requireESMRespondent`, which requires:

- a `StoredESMResponse` row keyed `(user_id == caller, prompt_id)` with
  `consumed_action == UNRESOLVED`
- the prompt is not soft-deleted
- the prompt has not expired

A non-targeted user receives `CodePermissionDenied` immediately and never
reaches the prompt's content (already known via the feed payload — that's
fine; the prompt question itself is intentionally public-to-recipients).

Test coverage: `TestRespondToESMPrompt_RejectsNonRecipient`,
`TestRespondToESMPrompt_RejectsExpired`,
`TestRespondToESMPrompt_RejectsRepeatSubmission` in `responses_test.go`.

### Pass — feed-card payload exposes only the prompt, not response history

The `ESMPromptPayload` (sent in `FeedItem.payload` for items of type
`FEED_ITEM_TYPE_ESM_PROMPT`) carries the prompt question, options, free-text
toggle, and the closes-at timestamp — and nothing else. It does not include
the recipient's prior response, other recipients' responses, or the
aggregate signal. Recipients who already responded simply do not receive
the feed item (the consumption-row filter in `FeedItemsForUser` excludes
non-`UNRESOLVED` rows).

Verified: [proto/ripls/api/feed_service.proto](../../proto/ripls/api/feed_service.proto) `ESMPromptPayload`.

### Pass — free-text content is host-only by construction

Free-text content lives on `StoredESMResponse.free_text` and is **never**
exposed via `GetESMSignal` (only the count is). No other RPC reads
free-text content today. Surfacing free-text content to the host would
require a new RPC explicitly authorized as host-only — that's a future
follow-up; v1 retains free-text strictly for analyst review.

### Pass — server-internal Workshop signal consumption respects the gate

When the Workshop nudge generation pipeline (in `server/momentum/`) calls
`GetESMSignal` server-internally to drive cascade priority (2), it
operates in a server context (no viewer impersonation). The host-only
auth gate is satisfied trivially. The resulting `WORKSHOP_QUEST_HERO`
nudge is then host-scoped via the existing `user_id` + `community_id`
filter on `StoredNudge`, so no cross-user leakage on the consumer side
either.

### Pass — operator runbook does not introduce a leak

[docs/runbooks/esm-manual-authoring.md](../runbooks/esm-manual-authoring.md)
authors prompts via direct storage layer access in dev/staging, not via
any user-facing API surface. The runbook does not expose response data
itself; verification SQL is read-only and runs in a privileged DB shell.
The `AttendeeUserIDs` helper used to prefill recipients reads
`ExperienceRSVP.attended` rows, which the operator already has rights to
inspect.

### Caveat — prompts are visible to recipients in cleartext

By design. The whole point is to ask each recipient a question. There is
no "encrypted prompt" or "anonymous question" mode in v1, and there
shouldn't be: the privacy concern the priorities doc identifies is
*responses* leaking, not *questions* leaking. Authoring guidance in the
runbook explicitly bans questions that name other participants or
evaluate relationships, which is the structural protection on the
question side.

## Tone constraints (re-stated, enforced through documentation)

The runbook embeds the canonical four prompt shapes — *worth doing
again?*, *did the time work?*, *anything missing?*, *would you bring a
friend next time?* — and warns against questions that could feel
surveillance-shaped. There is no automated linter on prompt question
text in v1; v2's automated trigger will draw from a curated bank, which
removes operator authoring discretion entirely.

## Action items: none

No code changes required. The privacy contract holds across the proto
shape, the storage model, the auth gates, and the feed payload. Free-text
content has no exposure path in v1; if that changes, a follow-up review
should re-validate.

## Re-verify when

- A new RPC is added to `ESMService` or `WorkshopService`.
- Free-text content gains an exposure path (host-facing read, analytics
  export, etc.).
- Cross-experience aggregation lands (changes the unit of aggregation).
- v2's automated trigger ships (re-verify the prompt template bank
  doesn't smuggle in person-evaluating language).
