# Issue #1360: [testing] Feed service has 6 of 9 helper files untested

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1360
**Filed:** 2026-04-25 | **Last activity:** 2026-04-27
**Labels:** auto-filed, review:testing | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The testing review (`docs/reviews/reports/testing_review.md`) flagged six helper
files in `server/services/feed/` as lacking direct test coverage:

- `feed_dedup.go`
- `nudge_storage.go`
- `consume_nudge.go`
- `feed_nudges.go`
- `feed_status.go`
- `terminator_pool.go`

A closer audit shows the headline is overstated — most of these files _are_
exercised today, but indirectly:

| File | Coverage today |
| --- | --- |
| `feed_dedup.go` | `itemKeyForEvent` is in `service_test.go::TestItemKeyForEvent`; `deduplicateEventsByItemKey` is exercised via `TestGetFeed_*` integration; `feedItemEntityKey` is **not tested at all**. |
| `nudge_storage.go` | `insertNudge`, `getActiveNudgesForUser`, `markNudgeConsumed`, `softDeleteNudge` are exercised via `nudges_test.go::TestNudgeLifecycle_*`; `updateNudgeMediaID`, `getPreviousGlobalTerminators` are **not tested**; `getGlobalTerminatorsForToday`'s day-of-year filter is **not tested**. |
| `consume_nudge.go` | The RPC handler `Service.ConsumeNudge` has **no direct test**. The underlying `markNudgeConsumed` is tested. |
| `feed_nudges.go` | `interleaveNudges`, `buildTerminator`, `nudgeToFeedItem` are tested. `pickTerminator` and the freshness-expiry branch (>24h) are **not directly tested**. |
| `feed_status.go` | `GetFeedStatus` has multiple targeted tests in `service_test.go` covering terminal events, supersession, and view records. The conversation-activity (chat message) branch is **not directly tested**. |
| `terminator_pool.go` | Fully tested in `nudges_test.go`. |

The real gap is therefore narrower than the issue states. The issue's
recommendation — three new files (`feed_dedup_test.go`, `nudge_storage_test.go`,
`consume_nudge_test.go`) — lines up with the actual missing coverage if we also
plug the small gaps in `feed_nudges.go` and `feed_status.go`.

## Context from Comments

The only comment is `@claude` from the maintainer triggering automation. No
additional clarification or scope reduction has been provided.

## Potential Solutions

### Option A: Add three new test files exactly as the issue recommends; do not move existing tests

- **Approach:** Create `feed_dedup_test.go`, `nudge_storage_test.go`,
  `consume_nudge_test.go` covering only the gaps above. Leave existing tests
  in `service_test.go` and `nudges_test.go` where they are.
- **Pros:** Minimal churn, no merge-conflict risk against in-flight branches,
  matches the issue recommendation verbatim. Each new file is small and easy
  to review.
- **Cons:** Test cohesion stays imperfect — `TestItemKeyForEvent` lives in
  `service_test.go` while a new sibling test lives in `feed_dedup_test.go`.
  A future reader has to grep two files to see all dedup tests.
- **Effort:** Small.

### Option B: Add the three new files **and** relocate existing helper tests so each `_test.go` mirrors its `.go` sibling

- **Approach:** Same as Option A, plus move `TestItemKeyForEvent` from
  `service_test.go` to `feed_dedup_test.go`, and move `TestNudgeLifecycle_*`
  / `TestBuildTerminator_*` / `TestInterleaveNudges_*` / `TestGetTerminatorForSlot_*`
  / `TestFetchNudgeImagery_ConcurrencyBound` from `nudges_test.go` into the
  files that match their subjects (`feed_nudges_test.go`, `terminator_pool_test.go`,
  `nudge_storage_test.go`, `nudges_test.go`).
- **Pros:** Cleanest long-term layout — each test file pairs 1:1 with a source
  file, matching the convention `stories_test.go ↔ stories.go`.
- **Cons:** Moves a lot of code. Several hundred lines of `git mv`-style
  rearrangement that adds review burden and conflict risk. The existing
  `nudges_test.go` predates the helper-file split and is not actively wrong.
- **Effort:** Medium.

### Option C: Skip new test files; just expand `service_test.go` and `nudges_test.go` to cover the gaps

- **Approach:** Add the missing tests inline to the existing two files.
- **Pros:** Smallest diff. No new files.
- **Cons:** Pushes `service_test.go` further past its already-painful 3,596 lines.
  Directly contradicts the issue recommendation.
- **Effort:** Small.

## Recommendation

**Option A.** It matches the issue's stated recommendation, fixes the actual
coverage gaps, and avoids the churn of Option B. The cohesion argument for
Option B is real but weak — the existing layout is workable, and a follow-up
reorganization can be done later as a focused PR if it ever becomes painful.
Option C makes the largest test file in the package even bigger, which the
testing review explicitly cited as a problem elsewhere.

## Open Questions

- **How should we test ConsumeNudge "idempotency under concurrent consumers"?
  The current implementation simply overwrites `consumed_at_unix_sec` /
  `consumed_action` on every call — there is no explicit guard against double
  consumption.** — Recommendation: write the test as documenting the *observed*
  behavior, not aspirational behavior: ten concurrent `ConsumeNudge` calls
  for the same nudge ID must (a) all return without error, (b) leave the
  nudge marked consumed exactly once in the sense that `consumed_at_unix_sec`
  is set and `getActiveNudgesForUser` no longer returns it. If the reviewer
  wants stronger guarantees (e.g. reject the second call), that is a
  separate behavior-change issue, not a test gap.
- **Should `feed_dedup_test.go` re-test `itemKeyForEvent` (already covered in
  `service_test.go::TestItemKeyForEvent`)?** — Recommendation: no.
  Don't duplicate. Cover only `deduplicateEventsByItemKey` and
  `feedItemEntityKey` in the new file. Add a leading comment pointing to the
  existing `TestItemKeyForEvent` so a reader of the new file knows where
  the sibling helper is tested.
- **Do we test the >24h freshness-expiry branch in `feed_nudges.go`?** —
  Recommendation: yes, add `TestInterleaveNudges_ExpiresOldNudges` to
  `nudges_test.go`. Insert one nudge with `CreatedAtUnixSec` 25h in the past
  and assert it is excluded from the returned feed. The async soft-delete is
  out of scope (testing it requires waiting for the goroutine and reaches into
  `logging.GoSafe` plumbing); just assert the synchronous filtering effect.
- **Do we add a `pickTerminator` unit test?** — Recommendation: yes, small.
  Cover the three branches: empty input → fallback record; some candidates have
  media → only those are picked from; no candidates have media → fall back to
  full pool. Goes in `nudges_test.go` since `pickTerminator` lives in
  `feed_nudges.go` (no separate `feed_nudges_test.go` file under Option A).

## Audit Notes

Audit checks (against `docs/server/architecture.md`):

- ✅ Unit tests preferred over integration; this plan adds direct unit tests.
- ✅ Table-driven tests where shapes vary (`TestFeedItemEntityKey_AllPayloadVariants`).
- ✅ File-per-functionality layout (`feed_dedup_test.go ↔ feed_dedup.go`,
  `nudge_storage_test.go ↔ nudge_storage.go`,
  `consume_nudge_test.go ↔ consume_nudge.go`) matches the standard service
  package layout.
- ✅ Reuses existing helpers (`setupTestStorage`, `createAuthenticatedContext`,
  `setupTestUser`, `setupTestCommunity`) — no new mocks or fixtures invented.
- ✅ Auth-test pattern uses `auth.RequireAuth(ctx)` as the package does today,
  not the older `auth.UserFromContext` referenced in the architecture doc.
- ✅ No client/caching/i18n concerns — server-test-only change.

## Implementation Plan

### Phase 1: `feed_dedup_test.go`

**Goal:** Direct unit tests for the two dedup helpers in `feed_dedup.go` that
have no current direct coverage.

**Tasks:**
- [ ] Create `server/services/feed/feed_dedup_test.go`.
- [ ] `TestDeduplicateEventsByItemKey_LatestWins` — insert two `CommunityEvent`
  records sharing an item key but different `OccurredAtUnixSec`; assert
  `eventGroup.latestEvent` is the newer one and `totalCount == 2`.
- [ ] `TestDeduplicateEventsByItemKey_DropsUnkeyedEvents` — pass an event with
  an unhandled `EventType` (e.g. `MEMBER_LEFT`); assert it is excluded from
  the result.
- [ ] `TestDeduplicateEventsByItemKey_GroupsAcrossEventTypes` — `GEAR_SHARED`
  and `TRANSFER_INTEREST_EXPRESSED` for the same `gear_id` must collapse
  into a single group.
- [ ] `TestFeedItemEntityKey_AllPayloadVariants` — table-driven test over
  `GearShared`, `RequestCreated`, `ExperienceCreated`, `CommunityCreated`,
  and a fallback (e.g. `Story`/`Nudge`) payload; assert each returns the
  expected `entity:id` form or `item.Id`.
- [ ] No DB setup needed — these are pure functions on protobuf values.

**Validates:** Dedup logic is correctness-locked at the unit level so
regressions surface in <1s instead of through full-RPC integration tests.

### Phase 2: `nudge_storage_test.go`

**Goal:** Direct unit tests for the nudge_storage helpers that lack coverage.

**Tasks:**
- [ ] Create `server/services/feed/nudge_storage_test.go`.
- [ ] `TestUpdateNudgeMediaID_SetsField` — insert a nudge with no media, call
  `updateNudgeMediaID`, fetch via `GetByID`, assert `MediaId` is set.
- [ ] `TestUpdateNudgeMediaID_NotFound` — call against an unknown ID; assert
  the returned error is non-nil and wraps a useful message.
- [ ] `TestGetGlobalTerminatorsForToday_FiltersByDay` — insert two
  `globalTerminatorUserID` nudges, one with `CreatedAtUnixSec` for today and
  one for "yesterday" (24h prior); assert `getGlobalTerminatorsForToday`
  returns only today's. Use `unixSecToDayOfYear(time.Now().Unix())` to derive
  today's day-of-year deterministically.
- [ ] `TestGetGlobalTerminatorsForToday_ExcludesSoftDeleted` — insert a
  global terminator for today, soft-delete it, assert it's excluded.
- [ ] `TestGetPreviousGlobalTerminators_ReturnsStale` — insert one for today
  and one for yesterday; assert only yesterday's is returned by
  `getPreviousGlobalTerminators(ctx, store, today)`.
- [ ] `TestGetPreviousGlobalTerminators_ExcludesSoftDeleted` — same as above
  but soft-delete the stale record; assert empty result.
- [ ] Reuse `setupTestStorage(t)` from `service_test.go` — no new DB helpers
  needed.

**Validates:** Storage helpers are unit-locked, including the
day-of-year boundary logic that drives daily terminator rotation.

### Phase 3: `consume_nudge_test.go`

**Goal:** Direct RPC-handler tests for `ConsumeNudge`.

**Tasks:**
- [ ] Create `server/services/feed/consume_nudge_test.go`.
- [ ] `TestConsumeNudge_RequiresAuth` — call without an auth context; assert
  the returned `connect.Code` is `Unauthenticated`. Mirror the auth-check
  pattern used by other RPC tests in the package (e.g.
  `TestGetFeedStatus_*` use `createAuthenticatedContext`).
- [ ] `TestConsumeNudge_MarksConsumed` — insert a nudge via
  `setupTestStorage` + `insertNudge`, call `ConsumeNudge`, then call
  `getActiveNudgesForUser` and assert the nudge is excluded. Also fetch the
  raw record via `GetByID` and assert `ConsumedAtUnixSec != nil` and
  `ConsumedAction == req.Action`.
- [ ] `TestConsumeNudge_RepeatedCalls_NoError` — call the RPC twice for the
  same nudge ID; assert both calls return success and the nudge is still
  excluded from active nudges. Document in a code comment that the
  semantics are "last writer wins; both calls are accepted".
- [ ] `TestConsumeNudge_ConcurrentCallers_NoError` — spawn 10 goroutines all
  calling `ConsumeNudge` for the same nudge ID; `sync.WaitGroup.Wait`; assert
  no errors. This is the issue's "idempotency under concurrent consumers"
  ask, scoped to the actual behavior.
- [ ] `TestConsumeNudge_UnknownNudge` — call with a random UUID; assert the
  returned `connect.Code` is `Internal` (current implementation surfaces
  storage `not found` errors as Internal — this test pins behavior; if
  reviewers want a different code, that's a code change, not a test change).

**Validates:** The most user-visible surface of the nudge feature has
direct RPC-level tests guarding the auth contract, the success path, and the
concurrent-consumer path called out by the issue.

### Phase 4: Plug remaining `feed_nudges.go` and `feed_status.go` gaps

**Goal:** Close the small remaining gaps without creating new files.

**Tasks:**
- [ ] Add `TestPickTerminator_*` to `nudges_test.go` covering: empty
  candidates → fallback synthetic record; mixed (some with media) → only
  candidates with media are returned; no candidates have media → full pool
  used. Run each branch many times if needed to handle the `rand.Intn`
  selection (10 iterations is sufficient for the "only-with-media" assertion
  since `pickTerminator` always selects from the filtered pool).
- [ ] Add `TestInterleaveNudges_ExpiresOldNudges` to `nudges_test.go`. Insert
  a nudge with `CreatedAtUnixSec = time.Now().Unix() - 25*3600`. Call
  `interleaveNudges` with empty content and assert no nudge appears in the
  result (terminator is allowed). Do not assert on the async soft-delete
  side effect.
- [ ] Add `TestGetFeedStatus_ChatActivityTriggersNew` to `service_test.go`.
  Setup: a `GEAR_SHARED` event the user has already viewed, plus a
  `ChatConversation` (or `Gear.ConversationId`) with a `ChatMessage`
  authored by another user with `CreatedAtUnixSec` after the user's view.
  Assert `hasNew == true`. Mirror the patterns from existing
  `TestGetFeedStatus_*` tests; reuse `setupTestStorage` /
  `createAuthenticatedContext`.

**Validates:** No untested top-level branch remains in the six helper files
flagged by the issue.

### Phase 5: Verification

**Goal:** Confirm the new tests pass and don't regress others, and that the
package's public API surface still compiles.

**Tasks:**
- [ ] Run `go test ./server/services/feed/...` and confirm all tests pass.
- [ ] Run `npm run build` to satisfy the project's lint + build gate.
- [ ] Spot-check that the new test files each end in a trailing newline
  and pass `gofumpt`. Add a `// Package feed ...` comment is **not** needed
  since the package already has `doc.go`.

**Validates:** The test additions ship clean with no follow-up cleanup.

## Related Issues

- The same testing-review pass that filed this issue likely filed analogous
  "helpers untested" tickets for other services. If a similar pattern is
  applied there, this plan is the template.
