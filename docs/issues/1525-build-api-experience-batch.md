# Issue #1525: [performance] buildAPIExperience runs per-experience queries inside list endpoints

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1525
**Filed:** 2026-04-28 | **Last activity:** 2026-04-29
**Labels:** auto-filed, review:performance | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`Service.buildAPIExperience` (`server/services/experience/service.go:156`) issues
~7–10 database queries per experience to assemble a single `*api.Experience`:

1. owner user (`services.FetchAPIUser` → `GetByID`)
2. conversation message count (`chat.GetConversationMessageCount` → `QueryByField`)
3. all `CommunityExperience` rows for the experience (`QueryByField`)
4. location row (`GetByID`, only if `LocationId` is set)
5. RSVPs for the experience (`QueryByFields`)
6. time proposals (`buildTimeProposals` → `QueryByField`), and **for each
   proposal**: 1 `FetchAPIUser` for the proposer + 1 `QueryByField` for votes,
   and **for each vote**: 1 `FetchAPIUser` for the voter
7. comment preview (`conversation.EnrichWithCommentPreview` → 1 `QueryByField`
   for messages + 1 batched `FetchAPIUsersBatch` for senders)

`ListExperiences` (`server/services/experience/get.go:183`) and
`ListMyExperiences` (`server/services/experience/get.go:225`) call
`buildAPIExperience` inside a loop over the experiences they just batch-fetched.
A user with 20 experiences therefore triggers ~140 queries per list call. This
is a textbook N+1 violation of CLAUDE.md's SQL Efficiency rule #1 ("Never query
in a loop") and is on a hot Tier-2 path (the Betty/Emma feed loops noted by
triage on 2026-04-28).

The recommended fix in the issue is a `buildAPIExperiences` (plural) batch path
that mirrors `portfolio.fetchAll` — pre-collect all owner / location / RSVP /
proposal / message dependencies in a handful of `QueryByFieldIn` + `GetByIDs`
calls, then assemble each `*api.Experience` from the in-memory maps. The issue
also explicitly asks for an `AssertMaxQueries(t, ctx, 8, ...)` regression test.

## Context from Comments

Two comments:

- 2026-04-28 collaborator triage (`maintainer`): set Issue Type → Task and
  Priority → P1, with rationale "performance High: per-experience queries in
  list endpoints; touches Tier-2 Betty/Emma loops." Linked to
  `docs/issues/triage-2026-04-28.md`.
- 2026-04-29 collaborator: bare `@claude` ping, asking the agent to take this on.

No design debate, no workarounds proposed. The recommendation is uncontested.

## Potential Solutions

### Option A: Introduce `buildAPIExperiences` (plural) and call it from both list endpoints (recommended)

- **Approach:** Add a new method `(*Service).buildAPIExperiences(ctx,
  []*models.Experience) ([]*api.Experience, error)` in
  `server/services/experience/service.go` (or a new file
  `service_batch.go` if it gets long). It performs every dependency fetch as a
  batch *before* assembling responses:

  1. **Experience IDs** — collected from input.
  2. **Owner / proposer / voter / message-sender users** — one
     `FetchAPIUsersBatch` over the union, keyed by user ID.
  3. **CommunityExperience rows** — one
     `QueryByFieldIn("experience_id", expIDs, &models.CommunityExperience{})`,
     grouped into `map[string][]*models.CommunityExperience` by experience ID.
  4. **Locations** — one `GetByIDs(ctx, locationIDs, &models.Location{})` over
     the de-duplicated set of `exp.LocationId` values.
  5. **RSVPs** — one
     `QueryByFieldIn("experience_id", expIDs, &models.ExperienceRSVP{})`,
     grouped per experience. Apply the same dedup-by-user / count rules as the
     current single-experience path.
  6. **Time proposals** — one
     `QueryByFieldIn("experience_id", expIDs, &models.ExperienceTimeProposal{})`,
     then one `QueryByFieldIn("proposal_id", proposalIDs, &models.TimeVote{})`
     for all votes. Build proposals from the in-memory maps + the user map
     (re-using the existing `services.ConvertTimeModelsToAPI` helper).
  7. **Conversation message counts and previews** — collect all non-empty
     `exp.ConversationId` values, then **call the already-existing
     `conversation.BatchEnrichWithCommentPreview`** which does both the message
     fetch and the user batch fetch in one shot, plus compute message counts
     from the same fetched messages (avoids the redundant
     `chat.GetConversationMessageCount` query). To do this cleanly, extend
     `BatchEnrichWithCommentPreview` (or add a sibling function) to also return
     non-system message counts per conversation, OR build a small local helper
     that re-uses the messages-by-conversation map.

  Then, in a tight loop, build each `*api.Experience` from those maps. The
  list endpoints become:

  ```go
  apiExps, err := s.buildAPIExperiences(ctx, expsInOrder)
  if err != nil { … }
  resp := &api.ListExperiencesResponse{Experiences: apiExps}
  ```

  Keep `buildAPIExperience` (singular) as a thin wrapper that calls
  `buildAPIExperiences` with a single-element slice, so the seven other
  callers (`save.go`, `rsvp.go`, `lifecycle.go`, `get.go:GetExperience`) keep
  working with no behavior change. Single-experience callers eat one extra
  unconditional batch round-trip vs. today, but they were already issuing 7+
  queries — the absolute count is not worse and is often better (no
  per-proposal/vote N+1 even for one experience).

- **Pros:** Direct fit for the issue's recommendation. Mirrors the working
  `portfolio.fetchAll` precedent. Reduces both list endpoints from O(7N) to
  O(1) extra queries (~7 batches regardless of experience count). Fixes the
  separately-N+1 `buildTimeProposals` → `buildTimeProposal` chain at the same
  time. Leaves the singular method available for non-list callers, so the
  blast radius is contained. Uses existing storage primitives only.
- **Cons:** Touches `buildTimeProposals`/`buildTimeProposal` to factor out an
  in-memory `buildTimeProposalFromMaps` variant; either
  `BatchEnrichWithCommentPreview` needs to expose message counts, or the new
  batch path duplicates a small amount of message-count logic. Slightly more
  RAM per request (all messages in memory at once), but the list endpoints
  already returned full experiences with full data so the increase is bounded
  by `len(experiences) × messages_per_conversation`.
- **Effort:** Medium. ~1 new file (or large addition to `service.go`), edits
  to two list endpoints, edits to `time_proposals.go` (factor out a maps-based
  builder), one small extension to `BatchEnrichWithCommentPreview` or a tiny
  local message-count helper, and three test additions
  (`AssertMaxQueries` for both list endpoints + a unit test for the new
  batch helper).

### Option B: Inline the batch fetches into `ListExperiences` / `ListMyExperiences` only (no shared helper)

- **Approach:** Skip the helper. Hand-write the batch fetches inside each list
  RPC and assemble responses in place, leaving `buildAPIExperience` untouched
  for non-list callers.
- **Pros:** Smallest single-PR diff. Doesn't require any singular-callers to
  exercise the new code path.
- **Cons:** Two near-identical batch blocks (one per list endpoint) that will
  drift over time. New experience fields added in the future have to be wired
  up in three places (singular + two inlined batches) instead of two
  (singular wraps batch). Loses the symmetry with `portfolio.fetchAll`.
- **Effort:** Small in the short term, but creates real long-term maintenance
  cost. Already a known anti-pattern in this codebase — `portfolio.fetchAll`
  exists specifically because we previously had this same duplication problem.

### Option C: Denormalize the data onto the `Experience` model

- **Approach:** Cache derived fields (`message_count`, `rsvp_yes_count`,
  `last_message_text`, etc.) directly on the `Experience` model and update
  them on every relevant write (RSVP change, message send, time vote, etc.).
- **Pros:** O(1) reads.
- **Cons:** Cache invalidation across many write paths (RSVP service, chat
  service, time-proposal service, lifecycle transitions). High risk of stale
  data. Significant proto-schema migration. Far larger scope than the
  performance issue warrants and re-introduces a class of correctness bugs we
  do not have today.
- **Effort:** Large — out of scope.

## Recommendation

**Option A.** It directly mirrors the issue's recommendation and the working
`portfolio.fetchAll` precedent, fits CLAUDE.md's SQL Efficiency guidelines,
and contains the change to a single subsystem. Option B's inlined-duplication
is exactly the trap that motivated `fetchAll` in the first place; Option C is
a denormalization project, not a perf fix.

The one design call inside Option A is how to compute message counts without
re-querying chat messages a second time. Recommended approach: extend
`conversation.BatchEnrichWithCommentPreview` to also return a parallel
`map[string]int32` of non-system message counts (using the same messages it
already fetches and the same `chat.IsSystemMessage` predicate that
`GetConversationMessageCount` uses). This deletes a per-experience query and
avoids touching the existing `chat.GetConversationMessageCount` callers.

## Open Questions

- **Should `buildAPIExperience` (singular) keep existing as a wrapper, or be
  deleted in favor of `buildAPIExperiences`?**
  Recommendation: Keep it as a one-line wrapper that calls the plural form
  with a single-element slice. The seven non-list callers
  (`save.go:209`, `save.go:388`, `rsvp.go:293`, `lifecycle.go:86`,
  `lifecycle.go:404`, `lifecycle.go:580`, and `get.go:44`) all want the
  community-context behavior of the singular form, and rewriting them all is
  out of scope for the perf issue. The wrapper preserves their semantics with
  zero risk.

- **Where does `communityID`-scoped behavior live in the batch path?**
  Recommendation: The list endpoints both call `buildAPIExperience(..., "")`
  (no community context), so the batch entry point used by lists takes no
  community ID at all. The singular wrapper continues to thread `communityID`
  through to per-experience scoping for RSVP filtering and `created_at`
  selection, calling the batch path with one experience and post-filtering
  the RSVP map. This keeps the batch helper simple and avoids encoding
  community-scoped logic in two places.

- **Should the `AssertMaxQueries` cap be 8, as the issue suggests?**
  Recommendation: Aim for ≤ 8 for `ListExperiences` and ≤ 7 for
  `ListMyExperiences` (the latter skips the `CommunityExperience`-by-community
  filter step). Concretely: 1 community-experience query + 1 experience batch
  + 1 user batch + 1 location batch + 1 community-experience-by-id batch + 1
  RSVP batch + 1 proposal batch + 1 vote batch + 1 message batch (which is
  what `BatchEnrichWithCommentPreview` already does) = ~9 in the worst case.
  Setting the cap at 12 leaves a small headroom while still slamming the door
  on N+1; revisit downward after measuring. Cap will be tightened in Phase 4
  if measurement shows headroom.

- **Should the new batch helper live in `service.go` or its own file?**
  Recommendation: New file `server/services/experience/build_api.go` (or
  similar). `service.go` is already 545 lines and adding ~200 lines of batch
  plumbing pushes it past the 1000-line guideline.

- **Should we also fix the `buildTimeProposals` per-proposal N+1 in singular
  callers?**
  Recommendation: Yes, automatically — once the singular form delegates to
  the plural one, every caller benefits. No extra effort.

## Implementation Plan

### Phase 1: Extend `BatchEnrichWithCommentPreview` to also return message counts
**Goal:** A single batch fetch can serve both the comment-preview enrichment
and the message-count field, so the new builder doesn't have to re-query.
**Tasks:**
- [ ] In `server/conversation/comment_preview.go`, add a new function
      `BatchEnrichWithCommentPreviewAndCounts(ctx, store, conversationIDs,
      currentUserID) (map[string]*CommentPreview, map[string]int32, error)`.
      It can either be a new function or an extra return value on the
      existing one — prefer a sibling function so we don't churn callers.
      Implementation re-uses the same `messagesByConv` map; counts are
      `chat.IsSystemMessage`-filtered.
- [ ] Unit-test the new function for: empty input, conversations with no
      messages, conversations with mixed system/user messages, and
      conversations not in the request set (must be absent from both maps).
**Validates:** Counts match what `chat.GetConversationMessageCount` returns
for the same conversation, and the function is purely additive.

### Phase 2: Factor out a maps-based time-proposal builder
**Goal:** Make time-proposal building independent of the database so the
batch path can drive it from in-memory maps.
**Tasks:**
- [ ] In `server/services/experience/time_proposals.go`, add an unexported
      helper `buildTimeProposalsFromMaps(experienceID string, proposalsByExp
      map[string][]*models.ExperienceTimeProposal, votesByProposal
      map[string][]*models.TimeVote, userMap map[string]*api.User)
      []*api.TimeProposal`. It does the same conversion the existing
      `buildTimeProposal` does, but with no DB calls.
- [ ] Make `buildTimeProposals` (singular call) optionally delegate to the
      maps-based path by fetching its own slice of proposals + votes and
      then calling the helper. (Optional cleanup; the existing path can stay
      if delegation noise is unwelcome — but the helper itself is required.)
- [ ] Unit-test `buildTimeProposalsFromMaps` for: no proposals, proposals
      with no votes, votes whose user is missing from the user map (skip,
      log warn), confirmed proposals, and stable ordering.
**Validates:** The new helper produces output identical to the existing
per-experience builder for the same data.

### Phase 3: Add `buildAPIExperiences` and rewire the two list endpoints
**Goal:** Lists fire ≤ ~10 batched queries regardless of experience count.
**Tasks:**
- [ ] Create `server/services/experience/build_api.go` (new file) with
      `(*Service).buildAPIExperiences(ctx, []*models.Experience)
      ([]*api.Experience, error)`. The function preserves input order on
      output (one `*api.Experience` per input, in the same slot). The
      authenticated user ID needed for unread-count computation is read once
      at the top via `auth.GetAuthInfo(ctx)` (callers without auth get an
      empty preview, mirroring the singular path's behavior). Logging
      follows server conventions: a single
      `logger := logging.LoggerWithContext(ctx).With("operation",
      "buildAPIExperiences", "count", len(experiences))` at the top, and
      every partial-failure path uses `logger.WarnContext(ctx, ...)` with
      the same masked-field rules as the singular form (no plaintext
      emails; entity IDs only). Order of operations:
      1. Collect experience IDs, location IDs, owner IDs, conversation IDs.
      2. Query `CommunityExperience` rows by `experience_id` (`QueryByFieldIn`)
         and group by `experience_id`.
      3. `GetByIDs(locationIDs, &models.Location{})`.
      4. `QueryByFieldIn(experience_id, expIDs, &models.ExperienceRSVP{})`,
         group by experience.
      5. `QueryByFieldIn(experience_id, expIDs,
         &models.ExperienceTimeProposal{})`, group by experience; collect
         proposal IDs.
      6. `QueryByFieldIn(proposal_id, proposalIDs, &models.TimeVote{})`,
         group by proposal.
      7. Collect user IDs (owners + proposers + voters + comment-preview
         senders), then `services.FetchAPIUsersBatch`.
      8. `BatchEnrichWithCommentPreviewAndCounts(convIDs, authUserID)`.
      9. For each experience, build the `*api.Experience` from the maps
         (mirroring the singular path's field assembly exactly).
- [ ] Convert `buildAPIExperience` to a one-line wrapper that calls
      `buildAPIExperiences` with `[]*models.Experience{exp}` and applies
      community-scoped post-filtering for RSVP counts and `created_at`
      selection (the singular form is the only caller passing a non-empty
      `communityID`).
- [ ] In `ListExperiences` (`get.go:183`) and `ListMyExperiences`
      (`get.go:225`): replace the per-experience loop call with one
      `s.buildAPIExperiences(ctx, expsInOrder)` call. Preserve the existing
      `state` filter by filtering `expsInOrder` before passing into the
      builder. Preserve the order-from-source-IDs behavior in
      `ListExperiences` (driven by `experienceIDs` from the
      `CommunityExperience` query).
- [ ] Make sure the singular wrapper still passes the same data shape on
      every error / partial-failure case the current code handles (warn +
      continue for missing community-experience rows, missing locations,
      etc. — these become "absent from map" lookups, which return zero
      values just like today).
**Validates:** All existing experience tests still pass with no changes;
list endpoints return the same data they returned before.

### Phase 4: Add `AssertMaxQueries` regression tests
**Goal:** Lock in the bound so future contributors can't reintroduce N+1.
**Tasks:**
- [ ] In `server/services/experience/get_test.go`, add
      `TestListExperiences_QueryCountBounded` that creates a community with 5
      shared experiences (each with 2 RSVPs, 1 time proposal with 2 votes,
      and 3 chat messages), then asserts
      `storage.AssertMaxQueries(t, ctxWithStats, 12, func(){
      service.ListExperiences(...) })`. Cap chosen with a small headroom
      over the expected ~9 batches; tighten after measurement.
- [ ] Add the same shape of test for `ListMyExperiences`.
- [ ] Add `storage.WithQueryStats(ctx)` on the auth context used in those
      tests; pattern is identical to
      `server/services/transfer/queries_test.go:626`.
**Validates:** Per the issue's explicit ask — N+1 is gone and stays gone.

### Phase 5: Build & integration verification
**Goal:** No build / lint regressions.
**Tasks:**
- [ ] Run `npm run build` (lint + non-tag build).
- [ ] Run `go build -tags=integration ./...` — the experience package has
      `*_integration_test.go` files (per CLAUDE.md the lint pass alone
      doesn't compile those, and exported-signature drift will only show
      up here).
- [ ] Run `go test -tags=integration -timeout 20m ./server/services/experience/...`
      since lifecycle / save / rsvp paths all call the singular wrapper.
- [ ] Run `go test ./server/services/portfolio/...` because the
      `portfolio.fetchAll` path independently consumes experiences and
      RSVPs — sanity-check that nothing it depends on shifted.
**Validates:** No build/lint regressions, no behavior regression in adjacent
suites.

## Related Issues

- #1280 — same N+1 class on `ListConversations` / `GetUnreadCounts`. Plan and
  fix already merged. The batch-fetch-then-assemble pattern in this plan
  follows the same shape and re-uses `BatchEnrichWithCommentPreview` from
  that work.
- #1196 — "batch needs and contributions" — a similar perf refactor for a
  different surface; precedent for the helper-and-test approach used here.
- #1096 — DB connection exhaustion under load. This issue is one of the
  callers contributing to that exhaustion class.
