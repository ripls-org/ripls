# Issue #1283: [performance] N+1 in ListCommunityEvents — per-event user and gear queries

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1283
**Filed:** 2026-04-25 | **Last activity:** 2026-04-25
**Labels:** auto-filed, review:performance | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`ListCommunityEvents` iterates over every event in the filtered result set and issues
up to three individual database queries per event:

1. `services.FetchAPIUser` for the actor (`event.ActorId`)
2. `services.FetchAPIUser` for the object user (`event.ObjectUserId`)
3. `s.storage.GetByID` for the gear name (`event.GearId`)

For a community with 50 events this produces 100–150 round-trips to the database
per request. The handler already has `FetchAPIUsersBatch` and the generic
`storage.GetByIDs[T]` available; the fix is to use them.

## Context from Comments

- **Triage (2026-04-24):** Classified as P2 Bug. The reviewer confirmed this is a
  user-facing N+1 in `ListCommunityEvents` and has been present for 45+ days.
- The issue body names the exact function and lines (`events.go:108-134`) and
  specifies the fix: collect all IDs before the loop, batch-fetch, then build the
  response from maps.

## Potential Solutions

### Option A: Pre-fetch all IDs before the loop (recommended)

- **Approach:** Walk `filteredMessages` once to collect unique actor IDs, object
  user IDs, and gear IDs. Call `FetchAPIUsersBatch` for all user IDs in one query,
  `storage.GetByIDs[*models.Gear]` for all gear IDs in one query. Build the
  response items from the two resulting maps.
- **Pros:** Eliminates the N+1 entirely; at most 2 extra queries regardless of
  event count; uses the already-present batch helpers; zero API or proto changes.
- **Cons:** None material — the logic is straightforward.
- **Effort:** Small (one file).

### Option B: Lazy-cache inside the loop

- **Approach:** Keep the loop structure but add a `map[string]*api.User` / gear map
  as a local cache; skip the DB call if the ID was already fetched.
- **Pros:** Slightly simpler diff.
- **Cons:** Still issues one query per *unique* ID; degrades to N queries when every
  event has a different actor. Adds mutable state to the loop that makes the code
  harder to read.
- **Effort:** Small, but wrong approach.

> [AUDIT FIX] Removed the claim that "CLAUDE.md explicitly prohibits manual cache
> maps" as the justification. That rule is a Flutter/Dart client-side constraint
> (ViewModels must not maintain manual cache maps). On the Go server side the rule
> against in-loop caches comes from the SQL Efficiency guideline: "Never query in a
> loop — use batch APIs instead." The rationale is still correct; the cited source
> was wrong.

### Option C: SQL JOIN in storage layer

- **Approach:** Add a specialised SQL query that joins `community_events`, `users`,
  and `gear` in one trip.
- **Pros:** Single round-trip to the database.
- **Cons:** Requires new storage code, new SQL, and bypasses the standard proto-SQL
  abstraction. Over-engineered for the problem; the batch helpers already give us
  O(1) queries.
- **Effort:** Large; not worth it.

## Recommendation

**Option A.** It is the minimal, idiomatic fix: collect IDs, batch-fetch, build from
maps. The existing `FetchAPIUsersBatch` and `storage.GetByIDs[*models.Gear]` helpers
were built precisely for this pattern and are already used throughout the codebase.

## Open Questions

- **Should error handling for missing users/gear be strict or lenient?**
  Recommendation: keep the same behaviour as today — a missing actor returns a hard
  error; a missing gear name is silently omitted (the `if err == nil` guard already
  reflects that). The batch helpers silently omit missing IDs from the result map,
  so the gear case stays lenient naturally. For the user case, `FetchAPIUsersBatch`
  also silently omits missing users — the implementation must therefore explicitly
  check that the actor ID was found in the returned map after the batch call and
  return an error if it was not. A nil map value for a non-empty actor ID must be
  treated as a hard error, matching the current per-event `FetchAPIUser` behaviour.

> [AUDIT FIX] Clarified that `FetchAPIUsersBatch` silently omits missing entries
> like `GetByIDs`, so "fail fast for actors" cannot be achieved by a bare map
> lookup — the implementation must add an explicit presence check after the batch
> call. Without this note an implementer could miss the check and silently drop
> actor data instead of erroring.

## Implementation Plan

### Phase 1: Batch-fetch and refactor loop

**Goal:** Eliminate N+1 queries in `ListCommunityEvents`.

**Tasks:**
- [ ] Walk `filteredMessages` to collect unique actor IDs, object user IDs, gear IDs.
- [ ] Call `services.FetchAPIUsersBatch` with the union of all user IDs.
- [ ] Call `storage.GetByIDs[*models.Gear]` with all gear IDs.
- [ ] Replace the per-event `FetchAPIUser` / `GetByID` calls with map lookups.
      For actors: after looking up the map, if the actor ID is non-empty but absent
      from the result map, return a hard `connect.CodeInternal` error (matching
      current behaviour). For gear: if absent from the gear map, leave `GearName`
      empty (lenient, matching current `if err == nil` guard).
- [ ] Add a `TestListCommunityEvents_NoN1` test using `storage.WithQueryStats` /
  `storage.AssertMaxQueries` to prevent regressions. The correct pattern is:

  ```go
  statsCtx := storage.WithQueryStats(ctxAlice)  // wrap the authed context
  storage.AssertMaxQueries(t, statsCtx, 6, func() {
      listResp, err := service.ListCommunityEvents(statsCtx, connect.NewRequest(...))
      // assertions inside func
  })
  ```

  The `statsCtx` must be passed **into the handler** (not just to `AssertMaxQueries`)
  so that queries executed by the handler are counted. `SetupTestStorage` already
  uses `InstrumentedDB` (wired in `storage.initializeDatabase`), so no additional
  setup is needed.

  **Query budget (after fix, for a test with multiple events):**
  1. `GetByID` — community existence check
  2. `QueryByFields` — membership check (`IsMemberOfCommunity`)
  3. `QueryByField` — all events for the community
  4. `FetchAPIUsersBatch` — single batch user fetch
  5. `GetByIDs[*models.Gear]` — single batch gear fetch

  Total: 5 queries regardless of event count. Use max = **6** to allow one unit of
  slack without hiding future N+1 regressions.

> [AUDIT FIX 1] Added the explicit actor presence-check requirement (see Open
> Questions above).
>
> [AUDIT FIX 2] Expanded test task with the correct usage pattern: `statsCtx` must
> be passed into the RPC handler, not only to `AssertMaxQueries`. Added computed
> query budget (5 queries) and a concrete `max` value of 6.

**Validates:** `go test ./server/services/community/...` passes and the new
query-count assertion holds for a multi-event feed.

## Related Issues

- None referenced.
