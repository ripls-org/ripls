# Issue #1621: Read-path audit: filter deleted communities in every list/search/feed surface

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1621
**Filed:** 2026-04-30 | **Last activity:** 2026-04-30
**Labels:** none | **Assignees:** briangbrown
**Context docs:** [`docs/community_delete_and_leave.md`](../community_delete_and_leave.md)

> **Audit applied** (`/ripls-audit`, 2026-04-30) — refined four spots
> against `docs/server/architecture.md` and `docs/server/observability.md`:
> 1. Storage-layer change is no longer a generic
>    `QueryOptions{ActiveCommunitiesOnly: true}` — protosql is generic
>    and has no per-table community schema. Aggregates use dedicated
>    `ProtoSQLStorage` methods that JOIN explicitly, mirroring
>    `protosql_search.go:154` and `protosql_spatial.go:141`.
> 2. The new auth helper must use
>    `connecterr.Internal(ctx, op, err, kv...)` for storage failures,
>    not bare `connect.NewError(CodeInternal, ...)` (gated by
>    `scripts/check_codeinternal.js`).
> 3. Logging in the helper follows the established `LoggerWithContext +
>    operation field` pattern (see `server/auth/community.go:73`).
> 4. Integration test lands in `server/integration_tests/` using
>    `storage.SetupTestStorage(t)` per the architecture doc.

## Problem Statement

Every read path that scopes by community must skip communities where
`Community.deleted IS NOT NULL` once #1619 ships, **except** the Settings →
Communities surface which explicitly opts in via
`storage.QueryOptions{IncludeDeleted: true}`. A community in the 30-day
soft-delete window must be invisible everywhere else: feed, search,
leaderboard, impact metrics, gear/request/experience listings, chat
conversations, push notifications (covered by #1622), streaming.

This issue ships **before** #1619 produces any deleted state, so the new
filter is a no-op at runtime. Tests force-flip `Community.deleted` to
prove the guard fires.

**The hard constraint that makes this issue interesting** is performance.
Community-scoped reads are the hottest path in the entire server. The fix
must not add a DB round trip per read. Naive solutions ("call
`GetByID(community)` at the top of every handler to check `Deleted`") add
one query to RPCs that already do six; on a cold cache they ladder into
real latency. The plan below piggy-backs on queries the handlers
**already** make so the steady-state cost is **zero extra queries** for
the dominant read paths.

## Context from Comments

No comments yet (issue filed today). Additional steer from the user when
this plan was requested:

> Because this is going to be in the load path for many many things we
> need to come up with a solution that is super performant. We don't want
> an extra round trip with the db for example on every read.

That direction is the spine of the recommendation below.

## What the read paths actually look like today

Single-community handlers (the dominant pattern) already do **two**
queries before they touch the data they're returning:

1. `s.storage.GetByID(ctx, communityID, &models.Community{})` to confirm
   the community exists.
2. `auth.RequireUserIsCommunityMember(ctx, ...)` which calls
   `IsMemberOfCommunity` → `QueryByFields` on `community_user`.

Examples:
- `server/services/community/gear_sharing.go:39-50` (`ListCommunityGear`)
- `server/services/feed/service.go:90-102` (`GetFeed`)
- `server/services/search/service.go:74-84` (`Search`)

This is good news — the `Community` row is **already in memory** by the
time data fetching starts. Adding `if community.Deleted != nil { return
NotFound }` after step 1 is a free check.

The membership check (step 2) is a separate `community_user` query that
does **not** join with `community`. We can rewrite it to JOIN, which
collapses the two existing queries into one without adding any.

Two surfaces don't follow this pattern:

- **Multi-community surfaces** (`Search`, `GetFeed`, `ListStories`):
  loop over `community_ids` doing `GetByID` per community. Same fix —
  inline check after each `GetByID`.
- **Storage-layer aggregates** (leaderboard rollups, impact metrics, feed
  story queries): query rows directly by `community_id` without first
  fetching the `Community`. Need a SQL-level filter (JOIN or subquery).

## Potential Solutions

### Option A: Inline check at every handler

- **Approach:** Find every place a community-scoped read happens. After
  the existing `GetByID(community)`, add `if community.Deleted != nil
  { return NotFound }`. For storage-layer aggregates, add a JOIN to
  `community` with `WHERE c.deleted_deleted_at_unix_sec IS NULL` (or
  `= 0`).
- **Pros:** Simplest possible. Zero new abstractions. Zero extra round
  trips because the `GetByID` is already happening. Easy to audit — one
  pattern, one grep.
- **Cons:** Repetition (the same 2-line check in ~30 handlers). New
  handlers can forget to add the check. No central enforcement.
- **Effort:** Medium — the work is in finding every site, not in the
  fix itself.

### Option B: Single-JOIN auth helper + per-context memoization

- **Approach:** Introduce one helper that replaces today's
  `GetByID(community) + RequireUserIsCommunityMember` two-query pattern
  with a **single JOIN** query that returns the `Community` row, the
  `CommunityUser` row, and an error if either is missing or the
  community is soft-deleted:

  ```go
  func RequireMemberOfActiveCommunity(
      ctx context.Context, s *storage.ProtoSQLStorage,
      communityID, userID string,
  ) (*models.Community, *models.CommunityUser, error)
  ```

  The query is something like:

  ```sql
  SELECT c.*, cu.*
  FROM community c
  LEFT JOIN community_user cu ON cu.community_id = c.id AND cu.user_id = $2
  WHERE c.id = $1
    AND (c.deleted_deleted_at_unix_sec IS NULL OR c.deleted_deleted_at_unix_sec = 0);
  ```

  Plus a complementary `RequireActiveCommunity(ctx, s, communityID)
  (*models.Community, error)` for the rare RPCs that need the community
  but no membership check (admin paths, public reads).

  Pair with **request-scoped memoization** on `context.Context` so two
  calls to `RequireMemberOfActiveCommunity` for the same `(communityID,
  userID)` in one RPC return the cached value — useful for handlers that
  do membership checks across multiple sub-functions.

  For storage-layer aggregates (leaderboard, impact metrics, feed-story
  queries), follow the existing pattern in `server/storage/protosql_search.go:154`
  and `server/storage/protosql_spatial.go:141`: each consumer adds a
  dedicated method on `ProtoSQLStorage` that writes the JOIN explicitly.
  A generic `QueryOptions{ActiveCommunitiesOnly: true}` was considered
  but rejected — the generic protosql layer has no per-table
  community-scoping schema, so we'd be sneaking SQL knowledge into the
  generic builder. Per-method JOINs keep the scope local and greppable.

- **Pros:** Single chokepoint — a new handler that calls the helper
  cannot forget the deleted check. **Reduces query count by 1** in
  every handler that calls it (was 2 queries, now 1). Memoization makes
  multi-step handlers free. The helper returns the `Community` object
  so callers don't need a separate `GetByID`.
- **Cons:** Mass refactor — every handler that does
  `GetByID(community) + RequireUserIsCommunityMember` needs its
  signature touched. Storage option adds a small layer to the protosql
  query builder.
- **Effort:** Medium-large. The helper itself is small; the migration
  of ~30 handlers and the storage filter is the bulk.

### Option C: Process-wide "deleted community IDs" set

- **Approach:** Maintain an in-memory `map[string]struct{}` of currently
  soft-deleted community IDs in each server process. Refresh on a 30s
  ticker (`SELECT id FROM community WHERE deleted_deleted_at_unix_sec
  > 0`) and invalidate on local writes (`DeleteCommunity`,
  `RestoreCommunity`). Read path is an `O(1)` map lookup.
- **Pros:** Reads cost zero queries, full stop — including for
  handlers that don't already fetch the Community. Cardinality is tiny
  (only soft-deleted communities, which auto-purge after 30 days).
- **Cons:** Stale window (up to 30s on other instances). Multi-instance
  consistency requires either Pub/Sub fan-out or accepting the
  staleness. New invalidation contract to get right. Distributed-system
  bug surface. The user-visible promise of "the moment a community is
  deleted, it disappears" softens to "within 30s on remote instances."
- **Effort:** Medium for v1, but the distributed-correctness work is
  recurring tax.

## Recommendation

**Option B**, with the per-context memoization. Rationale:

1. The dominant read paths **already** do `GetByID(community) +
   membership check`. Option B collapses those two queries into one.
   Steady-state we are **negative one** query per RPC, not "no extra
   round trip." That's the most performant outcome available and it
   drops out of fixing the bug, not as a separate optimization pass.
2. The single-helper pattern gives us central enforcement. New handlers
   that call `RequireMemberOfActiveCommunity` cannot forget the check.
3. Storage-layer aggregates use a `WithActiveCommunityFilter` query
   option — pushes the predicate to SQL, no extra round trip.
4. Option C's staleness window is unacceptable: a user who restores
   their community and then takes an action expects the action to
   succeed immediately, not 30s later. Option C also fails clean for
   long-lived processes (memory map drifts; bugs surface as "this
   community is deleted but the member can still post").

**Option C is appealing as a v2 layer** if telemetry shows residual
hot spots — for example, a hot endpoint that doesn't currently fetch
the Community at all. We can add the in-process cache later as a
read-through in front of the helper without changing call sites.

## Open Questions

- **Exact SQL shape for the JOIN.** Postgres `LEFT JOIN` returning
  nullable membership columns vs. two separate parameterized queries
  in a single transaction.
  **Recommendation:** Single SQL statement with `LEFT JOIN`. The
  protosql layer already supports parameterized SQL; one statement is
  one round trip and lets the planner do its thing. Membership-row
  null check happens in Go.
- **Should `RequireActiveCommunity` 404 or `FailedPrecondition` for
  soft-deleted communities?** Today's `GetCommunity` returns 404 for
  not-found.
  **Recommendation:** 404 (`NotFound`) for non-members and
  `FailedPrecondition` for members of a soft-deleted community. Reason:
  members need a different code so the client can route them to
  Settings → Communities to restore. Non-members must be unable to
  distinguish "doesn't exist" from "deleted" (no information leak).
- **Storage-level filter: opt-in or opt-out?**
  **Recommendation:** Per-method JOIN, not a generic option. See the
  rationale under Option B above. Aggregates that need to skip deleted
  communities get a dedicated `ProtoSQLStorage` method (e.g.,
  `QueryByCommunityIDActive`) that writes the JOIN explicitly. Mirrors
  the established pattern in `server/storage/protosql_search.go:154`
  and `server/storage/protosql_spatial.go:141`.
- **Per-context memoization key.**
  **Recommendation:** `(communityID, userID)` as the key, stored on the
  context as a `sync.Map`. TTL is the request lifetime (no ticker).
  Caches both the `Community` and `CommunityUser` rows.
- **What about handlers that fetch the Community but do not check
  membership** (e.g., a hypothetical "preview public community" RPC)?
  **Recommendation:** Use `RequireActiveCommunity` (community-only, no
  membership). Same JOIN reused, with the `LEFT JOIN community_user`
  branch dropped.
- **Streaming session termination.** The audit lives here; the actual
  `community_deleted` signal lives in #1619 Phase 2 step 10.
  **Recommendation:** This issue's scope is "the gRPC handler that
  opens the stream rejects on a deleted community." The mid-stream
  termination signal stays in #1619.
- **How do we prove no extra round trips?**
  **Recommendation:** Use the existing `storage.WithQueryStats(ctx)` /
  `storage.AssertMaxQueries(t, ctx, max, fn)` infrastructure (see
  `server/storage/instrumented_db_test.go:72`). Add per-handler
  upper-bound assertions for the top 5 hot-path RPCs (`ListCommunityGear`,
  `GetFeed`, `Search`, `ListCommunityRequests`,
  `ListCommunityExperiences`). If anyone adds a query, the test fails.

## Implementation Plan

### Phase 1: Storage primitives and helper

**Goal:** Land the new helper and storage option as standalone, callable
APIs with full unit-test coverage. No callers migrated yet — system is
unchanged in production behavior.

**Tasks:**
- [ ] Add `auth.RequireMemberOfActiveCommunity(ctx, s, communityID,
      userID) (*models.Community, *models.CommunityUser, error)` in
      `server/auth/community.go`. Implements the JOIN as a single
      parameterized SQL query (mirroring the raw-SQL helper pattern in
      `server/storage/protosql_search.go:154`). Returns
      `connect.CodeNotFound` if community is missing, `NotFound` if
      community is soft-deleted *and* caller is not a member,
      `FailedPrecondition` if soft-deleted *and* caller is a member,
      `PermissionDenied` if active but caller is not a member. Storage
      failures wrap with `connecterr.Internal(ctx,
      "RequireMemberOfActiveCommunity", err, "community_id", id,
      "user_id", uid)` — never bare `connect.NewError(CodeInternal, ...)`,
      which is gated by `scripts/check_codeinternal.js`. Logging
      follows the pattern in `server/auth/community.go:73` —
      `logging.LoggerWithContext(ctx).With("operation",
      "RequireMemberOfActiveCommunity", "community_id", id, "user_id",
      uid)`. PII fields use `logging.MaskEmail`/`MaskToken` as needed.
- [ ] Add `auth.RequireActiveCommunity(ctx, s, communityID)
      (*models.Community, error)` for the no-membership variant. Same
      logging and error-wrapping conventions.
- [ ] Add per-context memoization helper:
      `auth.WithCommunityCache(ctx) context.Context` and an internal
      lookup so repeated calls for the same `(communityID, userID)`
      within one RPC return cached values. Pattern mirrors
      `storage.WithQueryStats(ctx)` in
      `server/storage/instrumented_db.go` — context value is a
      `*sync.Map` keyed by `(communityID, userID)`. Cache lifetime is
      the request lifetime; no ticker, no eviction.
- [ ] **No** generic storage-layer option. Per-table JOINs land in
      Phase 4 as dedicated methods on `ProtoSQLStorage` (see Phase 4
      tasks).
- [ ] Unit tests in `server/auth/community_test.go`: helper return
      values for all four error paths (use `storage.SetupTestStorage(t)`
      for a real PG container per architecture doc § Testing). Test
      memoization correctness — second call for same key issues no
      additional query (verified via
      `storage.AssertMaxQueries(t, ctx, 1, fn)`).

**Validates:** The primitives are correct in isolation. We can flip a
test community to deleted and the helper returns the right error code
in each scenario.

### Phase 2: Migrate single-community handlers

**Goal:** Every RPC that takes a single `community_id` and currently
does `GetByID(community) + RequireUserIsCommunityMember` switches to
the new helper.

**Tasks:**
- [ ] `grep -rn "RequireUserIsCommunityMember" server/services/` —
      enumerate call sites. ~30 expected.
- [ ] For each: replace the two-query pattern with a single
      `community, membership, err := auth.RequireMemberOfActiveCommunity(...)`.
      Reuse the returned `community` object instead of refetching.
- [ ] Same pass for handlers that do `GetByID(community)` without a
      membership check (admin/public paths) — switch to
      `RequireActiveCommunity`.
- [ ] Per-handler `AssertMaxQueries` test for the top 5 hot endpoints
      to lock in the query count drop.

**Validates:** The migration is mechanical and reduces query counts by
1 per migrated RPC. No deleted state exists yet, so production
behavior is unchanged.

### Phase 3: Multi-community surfaces

**Goal:** `Search`, `GetFeed`, `ListStories`, and any other handler
that loops over `community_ids` skips deleted communities cleanly.

**Tasks:**
- [ ] `server/services/search/service.go:74-84`: replace the per-id
      `GetByID + RequireUserIsCommunityMember` loop with per-id
      `RequireMemberOfActiveCommunity`. Filter the deleted ones out
      of `communityIDs` before the actual search.
- [ ] `server/services/feed/service.go:90-102`: same.
- [ ] `server/services/feed/stories.go`: audit and apply same
      pattern.
- [ ] Per-handler test: pass two community IDs, force-flip one to
      deleted, assert results exclude the deleted community without
      erroring on it.

**Validates:** Multi-community RPCs degrade gracefully when one of
several communities is deleted.

### Phase 4: Storage-layer aggregates

**Goal:** Rollups and aggregates that query rows directly by
`community_id` (without first fetching the `Community`) filter at SQL
level via dedicated `ProtoSQLStorage` methods.

**Tasks:**
- [ ] Audit `server/services/leaderboard/`,
      `server/services/impact_metrics/`, and the feed-story queries in
      `server/services/feed/` for direct community-scoped
      `QueryByField`/`GetByIDs` calls.
- [ ] For each cluster of similar queries, add a dedicated method on
      `ProtoSQLStorage` (e.g.,
      `QueryCommunityScopedActive(ctx, table, communityIDs,
      message)`) that writes the JOIN/EXISTS predicate inline.
      Mirrors the established raw-SQL pattern in
      `server/storage/protosql_search.go:154` and
      `server/storage/protosql_spatial.go:141`. Methods carry their
      own logger and use `connecterr.Internal` for failures.
- [ ] Settings → Communities deleted-list (Phase 3 of #1619) is the
      *only* surface that uses the unfiltered query path — flag it
      with a `// includes deleted intentionally for restore UI` comment
      so future readers don't "fix" it.
- [ ] Per-handler test in `*_test.go` alongside each modified file:
      insert leaderboard rows for an active and a deleted community,
      assert deleted community is excluded from rollups.

**Validates:** SQL-level filtering works for aggregates without
requiring the caller to hold a `Community` object.

### Phase 5: Streaming + chat entry-point checks

**Goal:** New `WatchCommunity*` streams and chat conversation opens
reject deleted communities at handshake. (Mid-stream termination is
out of scope; that ships with #1619 Phase 2 step 10.)

**Tasks:**
- [ ] `server/services/chat/conversations.go`: any handler that
      resolves a community-scoped conversation calls
      `RequireMemberOfActiveCommunity` first.
- [ ] `server/services/community/streaming.go`: stream-open handler
      calls the helper.

**Validates:** A user cannot open a new chat or stream against a
deleted community.

### Phase 6: Integration test + audit

**Goal:** End-to-end proof that no community-scoped read leaks deleted
state, and a regression gate.

**Tasks:**
- [ ] Single integration test in `server/integration_tests/` (same
      directory as the recently-merged `experience_impact_test.go`)
      using `storage.SetupTestStorage(t)` per the architecture doc §
      Testing. Test: (a) create community + member + gear + request +
      experience + chat; (b) force-flip community to deleted via a
      direct storage write; (c) call every surface in the issue's
      table and assert the documented behavior (404, empty, 403,
      FailedPrecondition).
- [ ] Add a CI grep gate (Node.js script in `scripts/`, style match
      `scripts/check_codeinternal.js`) that flags any
      `RequireUserIsCommunityMember` or direct
      `storage.GetByID(..., &models.Community{})` call site in
      `server/services/` outside the helper itself. Wire into
      `npm run lint:go` (currently invokes
      `node scripts/check_codeinternal.js`).
- [ ] `storage.WithQueryStats(ctx)` + `storage.AssertMaxQueries(t,
      ctx, max, fn)` per-handler assertions for the top 5 hot RPCs
      (`ListCommunityGear`, `GetFeed`, `Search`,
      `ListCommunityRequests`, `ListCommunityExperiences`) to lock in
      the post-migration query counts. These tests live alongside
      each handler's existing `*_test.go`.

**Validates:** No surface leaks deleted state; the system is
regression-protected; the perf goal (zero extra round trips, in fact
**fewer** queries) is enforced by tests.

## Related Issues

- #1619 — parent issue (community soft-delete with 30-day restore window).
  This issue is Phase 1 scaffolding; #1619 Phase 2 produces the deleted
  state that this issue's filter acts on.
- #1622 — sibling Phase 1 issue (notification dispatcher guard). Reuses
  the helper from Phase 1 of this plan.
- #1623 — sibling Phase 1 issue (background-job write guard). Reuses
  the helper from Phase 1 of this plan.
- #665 — recently-closed cross-community RSVP leakage. Same class of
  bug (failure to filter by community boundary at the read path).
  Lessons applicable: every entry point needs the check; tests must
  cover the "without filter" code paths explicitly.
