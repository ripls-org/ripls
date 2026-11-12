# Issue #1806: perf: ListMyRequests N+1 — 65 DB queries per call from per-item conversation enrichment

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1806
**Filed:** 2026-05-09 | **Last activity:** 2026-05-19
**Labels:** (none) | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`RequestService/ListMyRequests` runs ~65 database queries per call on a typical
inbox of 6 requests. The RPC traffic audit (pass 2) measured 7 calls × 65 queries
= 455 queries in a 5-minute session, making it the heaviest unary RPC observed.

The root cause is the per-item enrichment loop in
`server/services/request/queries.go:286-297`, which calls
`s.buildRequest(ctx, request, communityID)` once per result. `buildRequest`
(`server/services/request/helpers.go:22`) issues ~9 storage operations per
request:

1. `services.FetchAPIUser(requesterId)` — 1 query
2. `QueryByFields(RequestOffer{request_id, withdrawn:false})` — 1 query
3. `services.FetchAPIUsers(offererIDs)` — 1 batched user fetch
4. `QueryByFields(CommunityRequest{request_id, …})` — 1 query
5. `QueryByFields(CommunityEvent{community_id, event_type=REQUEST_CREATED})` — 1
   query (and the result is then linearly scanned for the matching `request_id`)
6. `GetByID(Location{request.LocationId})` — 1 query (when set)
7. `chat.GetConversationParticipantsWhoPosted(conversationID)` — 1 query
8. `services.FetchAPIUsers(filteredParticipantIDs)` — 1 batched user fetch
9. `chat.GetConversationMessageCount(conversationID)` — 1 query

`ListRequests` (community-scoped list of all requests, queries.go:120-220) uses
the same `buildRequest` loop and has the same N+1 shape — it has not been
measured but is structurally identical.

Goal: drop ListMyRequests db_queries per call from ~65 to ~10 by hoisting
per-item lookups into batches. Same fix family as #1280 (ListConversations),
#1281 (Leaderboard), #1283 (ListEvents) — all already shipped.

## Context from Comments

There are only two `@claude` mention comments on the issue; no substantive
discussion. The issue body itself is highly specific:

- Names the exact loop site (`queries.go:288`) and per-item function
  (`helpers.go:22`).
- Lists the nine inner storage operations with file:line citations.
- Cites the captured per-call breakdown (`6× GetConversationParticipantsWhoPosted
  + 6× GetConversationMessageCount + 1× RequireAuth + 1× AuthFunc = 65` for an
  inbox of 6 requests).
- Prescribes the fix shape: reuse `storage.GetByIDs` / `storage.QueryByFieldIn`,
  add `GetConversationParticipantsWhoPostedBatch` and
  `GetConversationMessageCountsBatch` helpers in `server/chat/conversations.go`,
  refactor `buildRequest` to accept pre-fetched maps "mirroring the
  `feedLookups` pattern used by `FeedService.GetFeed`".
- Targets ~10 queries per call.
- Specifies the test: `assertMaxQueries(t, ctx, 12, fn)` wrapping the existing
  `TestService_ListMyRequests`.

The closest in-repo prior art is `buildAPIExperiences` in
`server/services/experience/build_api.go:23-230`, which already implements
exactly this pattern for the equivalent Experience list — including the use of
`conversation.BatchEnrichWithCommentPreviewAndCounts` to batch comment-preview
enrichment across many conversations. That helper already exists and produces
both the preview maps and the non-system message-count map in one batch.

## Potential Solutions

### Option A: Add `buildAPIRequests` batched path, keep singular `buildRequest` (recommended)

- **Approach:** Mirror the experience service. Introduce a new
  `buildAPIRequests(ctx, requests []*models.Request, communityID string) ([]*api.Request, error)`
  helper in `server/services/request/helpers.go` that:
  1. Collects request IDs, requester IDs, location IDs, conversation IDs.
  2. Batch-fetches `RequestOffer` (`QueryByFieldIn("request_id", reqIDs)` filtered
     to `withdrawn=false`), `CommunityRequest`
     (`QueryByFieldIn("request_id", reqIDs)`), `CommunityEvent`
     (`QueryByFieldIn("request_id", reqIDs)` filtered server-side or in-process
     to `REQUEST_CREATED`), and `Location` (`GetByIDs`).
  3. Batch-fetches all users (requesters + offerers + participants) via
     `services.FetchAPIUsersBatch` after collecting IDs across all groups.
  4. Replaces the per-conversation chat calls by reusing
     `conversation.BatchEnrichWithCommentPreviewAndCounts(ctx, store,
     convIDs, authUserID)` for both the message count and the comment-preview
     fields. This eliminates the need for new chat helpers — the existing batch
     reads messages once and returns counts.
  5. Computes the participants-who-posted-and-aren't-the-requester list from the
     same pre-fetched message slice (grouped by conversation in
     `BatchEnrichWithCommentPreviewAndCounts`). This requires extending the
     existing `BatchEnrichWithCommentPreviewAndCounts` to also return a
     `map[conversationID][]string` of poster IDs in chronological order — a
     small additive change to a function that already iterates the messages.
  6. Wire `ListMyRequests`, `ListRequests`, and the `populateRequestSharedCommunities`
     follow-up step in `GetRequest` so that all three list paths benefit. Singular
     callers in `sharing.go:130`, `lifecycle.go:480`, `offers.go:207`, `queries.go:44`
     (GetRequest) keep calling `buildRequest`, which now delegates to
     `buildAPIRequests` with a one-item slice (or stays as today — the diff is
     smaller if we leave it alone).

- **Pros:**
  - Uses the canonical pattern already in `buildAPIExperiences`; matches the
    existing prior fixes for #1280/#1281/#1283.
  - Single round-trip per *shape* of data → ~10 queries regardless of inbox
    size.
  - Reuses `BatchEnrichWithCommentPreviewAndCounts`, which already batches all
    message reads and user fetches. No new chat helpers needed.
  - Singular code path is untouched, so `GetRequest`/`SubmitOffer`/etc. behaviour
    is byte-identical to today.
- **Cons:**
  - Adds one new helper file (~150 lines) plus the small extension to
    `BatchEnrichWithCommentPreviewAndCounts`.
- **Effort:** Medium (one file new, two list handlers updated, one helper
  extended, one new test).

### Option B: Add new chat batch helpers as the issue suggests

- **Approach:** Implement `GetConversationParticipantsWhoPostedBatch` and
  `GetConversationMessageCountsBatch` in `server/chat/conversations.go` exactly
  as the issue describes, then refactor `buildRequest` to take pre-fetched
  maps for participants and counts.
- **Pros:** Most literal interpretation of the issue body.
- **Cons:**
  - Duplicates what `conversation.BatchEnrichWithCommentPreviewAndCounts`
    already does (it reads all messages and could produce the same
    participants-who-posted list almost for free).
  - The request handler would end up issuing **two** batched message reads —
    one from the new chat helpers (for posters + counts) and one from
    `EnrichWithCommentPreview` for comment-preview fields — when one read is
    sufficient. The preview enrichment is already in `buildRequest` and would
    not go away.
  - Inconsistent with the `buildAPIExperiences` pattern that is the established
    "feedLookups"-style approach for similar list handlers.
- **Effort:** Medium, but architecturally less clean.

### Option C: Lazy in-loop cache

- **Approach:** Cache user fetches and message reads in maps inside the
  `ListMyRequests` loop, without batching.
- **Pros:** Smallest diff.
- **Cons:** Still issues N queries per unique conversation; same anti-pattern
  called out in #1283's plan. Violates "Never query in a loop" from CLAUDE.md
  SQL Efficiency rules.
- **Effort:** Small but wrong direction.

## Recommendation

**Option A.** The `buildAPIExperiences` pattern is already in the repo and is
the canonical fix for this exact class of N+1. Reusing
`BatchEnrichWithCommentPreviewAndCounts` means we make *one* batched message
read per call instead of the two we'd end up with under Option B. Extending it
to also return chronological poster IDs is a small, additive change to a
function that already walks the message list.

`ListRequests` (community-scoped) should be fixed in the same change since it
calls the same `buildRequest` loop and has the same N+1 shape — leaving it
in-loop creates an inconsistent codebase and risks the same audit re-flagging it
in the next pass.

## Open Questions

- **Should the chronological-posters list live in `BatchEnrichWithCommentPreviewAndCounts`
  or in a sibling function?** — **Recommendation:** add a third return value
  (`postersByConv map[string][]string`) to
  `BatchEnrichWithCommentPreviewAndCounts` (and have the existing two-return
  `BatchEnrichWithCommentPreview` wrapper drop it). Same data, same iteration —
  doing it once is the whole point. Update the one current caller
  (`server/services/experience/build_api.go:119`) to ignore the new return
  value. Alternatively, add a new exported helper `BatchPostersByConversation`
  in the conversation package that takes the already-fetched messages and
  returns the map — this avoids touching existing signatures but requires
  callers to fetch messages separately. Prefer the first option for code reuse.

- **Filter `CommunityEvent` by event_type in SQL or in memory?** — The current
  per-item query passes `{community_id, event_type=REQUEST_CREATED}` to
  `QueryByFields`. For the batched version we have `request_id` (which is far
  more selective than `event_type` since each request has at most one
  `REQUEST_CREATED` event). **Recommendation:** use
  `QueryByFieldIn("request_id", requestIDs, &models.CommunityEvent{})` and
  filter `event_type == REQUEST_CREATED` in Go.

> [AUDIT FIX] Confirmed there is **no existing index on `community_event(request_id)`**.
> The only `community_event` indexes in `server/storage/protosql_schema.go:407,426`
> are `(community_id, occurred_at_unix_sec DESC)` and `(occurred_at_unix_sec DESC)`.
> Without a new index the `request_id IN (…)` lookup will be a sequential scan that
> degrades as the table grows. Phase 5 is therefore **not optional** — it must add
> `CREATE INDEX IF NOT EXISTS idx_community_event_request_id ON "community_event"
> (request_id) WHERE request_id <> ''` (or equivalent) in the same PR. Marked
> below.

- **Should the singular `buildRequest` callers (`GetRequest`, `SubmitOffer`,
  `Sharing`, `Lifecycle`) also be migrated?** — **Recommendation:** no. They
  each handle exactly one request and the singular path is fast enough.
  Migrating them is a larger diff with no measured performance gain. Leave
  `buildRequest` in place as the singular path; add `buildAPIRequests` as the
  batched path. (`GetRequest` already follows up `buildRequest` with its own
  batched `populateRequestSharedCommunities`; that pattern is fine.)

- **Should we also rework `populateRequestSharedCommunities`?** — It is already
  batched (`GetByIDs`, `QueryByFieldIn`). Leave it as-is. `ListMyRequests` does
  not call it today, and the issue does not ask for shared-community population
  in the list response. Do **not** add it in this PR.

- **Test query budget — what is the realistic ceiling?** — Issue suggests
  `assertMaxQueries(t, ctx, 12, fn)`. After hoisting:
  - 1× initial list query (Request or CommunityRequest+Request)
  - 1× RequestOffer batch
  - 1× CommunityRequest batch (for shared-community IDs / conversation
    fallback / state-aware filtering)
  - 1× CommunityEvent batch
  - 1× Location batch
  - 1× ChatMessage batch (inside `BatchEnrichWithCommentPreviewAndCounts`)
  - 1× User batch (`FetchAPIUsersBatch`)
  - 2× framing (`RequireAuth`, etc.)

  ≈ 9 queries. **Recommendation:** assert `≤12` to leave headroom for
  incidental queries (auth, query-stats overhead). Match the `ListEvents` plan
  which used `≤6` for a smaller surface; 12 here is generous but matches the
  issue body and gives ourselves room for one extra helper without immediately
  having to update the test.

## Implementation Plan

### Phase 1: Extend `BatchEnrichWithCommentPreviewAndCounts` with poster IDs

**Goal:** Get a `postersByConv map[string][]string` map for free from the same
message scan that already powers comment previews.

**Tasks:**
- [ ] In `server/conversation/comment_preview.go`, extend
      `BatchEnrichWithCommentPreviewAndCounts` to also build and return
      `map[string][]string` of chronological user-message senders per
      conversation (de-duplicated, first-poster-first). Use the same predicate
      as `chat.GetConversationParticipantsWhoPosted` (`userMsg != nil`).
- [ ] Update the one current caller in
      `server/services/experience/build_api.go:119` to either ignore the new
      return value or use it where appropriate.
- [ ] Add a unit test in `server/conversation/comment_preview_test.go`:
      `TestBatchEnrichWithCommentPreviewAndCounts_PosterOrder` covering
      chronological ordering, dedup, multiple conversations, and the
      system-message-excluded predicate.

**Validates:** The shared helper produces the data the new request batcher
needs without requiring a second message scan.

> [AUDIT FIX] Convention reminders for the implementation agent:
> - **Log field naming:** Use `target_request_id` (not `request_id`) for the
>   `models.Request` ID when calling `logger.With(...)`. The existing
>   `helpers.go:23` and `lifecycle.go` callers already follow this rule, which
>   CLAUDE.md mandates because `request_id` is reserved for the HTTP correlation
>   header. The new `buildAPIRequests` helper must do the same in any log lines
>   that name a specific request.
> - **Error wrapping:** All `connect.CodeInternal` errors must be returned via
>   `connecterr.Internal(ctx, "buildAPIRequests", err, kv...)`. A bare
>   `connect.NewError(connect.CodeInternal, err)` is gated by
>   `scripts/check_codeinternal.js` and will fail the build.
> - **Logger sourcing:** Inside the new helper, use `logging.LoggerWithContext(ctx).With("operation", "buildAPIRequests", "count", len(requests))`,
>   matching `buildAPIExperiences` in `server/services/experience/build_api.go:28-31`.

### Phase 2: Add `buildAPIRequests` batched helper

**Goal:** New private method on `*Service` that takes
`[]*models.Request` + `communityID` and returns `[]*api.Request` with **O(1)**
DB queries.

**Tasks:**
- [ ] Add `buildAPIRequests` in `server/services/request/helpers.go` (new file
      `helpers_batch.go` is fine if `helpers.go` gets too long — file
      organisation rule allows splitting). Modelled on
      `server/services/experience/build_api.go:23-230`.
- [ ] Step-by-step inside the helper:
  1. Collect `requestIDs`, `requesterIDs`, `locationIDs`, `conversationIDs`.
  2. `storage.QueryByFieldIn[*models.RequestOffer](s.storage, ctx, "request_id", requestIDs)`
     filtered in-process to `withdrawn=false`. Group by `request_id`.
  3. `storage.QueryByFieldIn[*models.CommunityRequest](s.storage, ctx, "request_id", requestIDs)`.
     Group by `request_id`. Apply state-aware filtering matching the singular
     path (FULFILLED/CANCELLED include archived; others exclude archived) per
     request.
  4. `storage.QueryByFieldIn[*models.CommunityEvent](s.storage, ctx, "request_id", requestIDs)`,
     filter to `REQUEST_CREATED` in-process, build
     `map[string]int64` of `request_id → OccurredAtUnixSec`.
  5. `storage.GetByIDs[*models.Location](s.storage, ctx, locationIDs)`.
  6. `previewMap, countMap, postersByConv := conversation.BatchEnrichWithCommentPreviewAndCounts(...)`
     (Phase 1 signature).
  7. Build the union user-ID list = requesters ∪ all offerer IDs ∪
     (all posters minus the corresponding request's requester) and call
     `services.FetchAPIUsersBatch` once.
  8. Loop input requests, assemble `*api.Request` from the maps. Skip a
     request and log a warning if its requester is missing from the user map
     or if it has no community-request rows (mirrors current behaviour).
- [ ] Keep the singular `buildRequest` as-is; do not migrate `GetRequest`,
      `SubmitOffer`, `Sharing`, or `Lifecycle` callers.
- [ ] Preserve API response shape exactly. Compare proto field-by-field with
      the singular path output for the same input in a unit test
      (`TestBuildAPIRequests_MatchesSingular`).

**Validates:** Batched output is byte-equivalent to a sequential
`buildRequest` loop over the same inputs.

### Phase 3: Wire `ListMyRequests` and `ListRequests` to the batched path

**Goal:** Drop both list handlers' db_query count from O(N) to O(1).

**Tasks:**
- [ ] In `server/services/request/queries.go:286-297` replace the per-item
      `buildRequest` loop with one call to
      `s.buildAPIRequests(ctx, matchedRequests, req.Msg.CommunityId)`.
- [ ] In `server/services/request/queries.go:193-215` (the `ListRequests`
      handler) replace the per-item `buildRequest` loop with the batched call.
      Keep the existing per-state filter that happens **before**
      `buildAPIRequests` is called so the batch input is already trimmed.

**Validates:** Both list endpoints return the same response payload as before
but with O(1) queries.

### Phase 4: Tests

**Goal:** Guard the perf invariant in CI and verify correctness.

**Tasks:**
- [ ] Extend `TestService_ListMyRequests`
      (`server/services/request/queries_test.go:90`) to seed 30+ requests with
      conversations and messages, then wrap the call in
      `storage.WithQueryStats(ctx)` + `storage.AssertMaxQueries(t, statsCtx,
      12, fn)`. The `statsCtx` must be passed **into the handler**, not just
      to `AssertMaxQueries`, or queries won't be counted (see #1283 plan
      tip).
- [ ] Add `TestService_ListRequests_NoN1` mirroring the above for the
      community-scoped list handler.
- [ ] Add a correctness test
      `TestService_ListMyRequests_BatchedShapeMatchesSingular` that creates a
      small fixture (3 requests with conversations + offers + locations),
      calls `ListMyRequests`, then for each result calls `GetRequest`
      separately and asserts equality of all `api.Request` fields except
      `SharedCommunities` (which `ListMyRequests` does not populate today and
      `GetRequest` does via `populateRequestSharedCommunities`).
- [ ] Run `npm run build && go test -tags=integration -timeout 20m ./server/services/request/... ./server/conversation/... ./server/services/experience/...`.

**Validates:** N+1 regression is now CI-blocking; response payload is
preserved; experience handler still passes after the
`BatchEnrichWithCommentPreviewAndCounts` signature change.

### Phase 5: Add index on `community_event(request_id)`

**Goal:** Make the new `request_id IN (…)` lookup index-served instead of a
sequential scan. **Required** — see Open Questions audit finding; no existing
index covers this access pattern.

**Tasks:**
- [ ] Add `CREATE INDEX IF NOT EXISTS idx_community_event_request_id ON
      "community_event" (request_id) WHERE request_id <> ''` to
      `server/storage/protosql_schema.go` alongside the existing
      `community_event` index definitions (~line 407 / 426). Use a partial
      index because `request_id` is empty on every non-request event.
- [ ] No data migration is needed — the schema bootstrap runs the
      `CREATE INDEX IF NOT EXISTS` on startup. New deploys pick it up
      automatically.

**Validates:** Production query stays bounded as the `community_event` table
grows. `EXPLAIN ANALYZE` on the new `QueryByFieldIn` call should show an
index scan rather than a seq scan.

## Related Issues

- #1280 — ListConversations N+1 (same fix shape, shipped in PR #1301)
- #1281 — Leaderboard N+1 (same fix shape)
- #1283 — ListEvents N+1 (same fix shape, plan at
  `docs/issues/1283-list-events-n1.md`)
- #1581 — RPC traffic audit that surfaced this issue
- `docs/ai/rpc_audit/findings-2026-05-09-pass2.md` — F-NEW
  RequestService/ListMyRequests
