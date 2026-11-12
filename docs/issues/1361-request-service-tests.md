# Issue #1361: [testing] Request service helpers / events / sharing / stats untested -- recurring 142+ days

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1361
**Filed:** 2026-04-25 | **Last activity:** 2026-04-26
**Labels:** auto-filed, review:testing | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Five source files in `server/services/request/` have no dedicated test coverage:
`helpers.go`, `events.go`, `sharing.go`, `stats.go`, and `people.go`. The
lifecycle and offer tests exercise these paths incidentally, but there are no
targeted tests that pin the contracts of state conversion, event emission
invariants, ACL rules in sharing, stats calculation, or people assembly. This
means regressions in any of these paths would require an integration-level
lifecycle test to catch them — a slow feedback loop for logic that could be
unit-tested cheaply.

## Context from Comments

The only comment is `@claude` from a collaborator, indicating the issue was
assigned to Claude for resolution. No additional direction or constraints were
provided. The issue body links to `docs/reviews/reports/testing_review.md` as
the source review.

## Potential Solutions

### Option A: Targeted unit tests for pure functions + integration tests for RPC handlers
- **Approach:** Write table-driven tests for the pure conversion functions
  (`modelRequestStateToApi`, `apiRequestStateToModel`, `vulnerabilityLevelToString`,
  `buildHintFromStoredRequest`) in a pure-function style (no DB), and integration
  tests (using `setupTestServiceWithNotifications`) for the RPC handlers
  (`ShareRequest`, `UnshareRequest`, `GetRequestStats`, `GetRequestPeople`) and
  the event helper (`recordCommunityEventForRequest`).
- **Pros:** Tests match the complexity of what they're testing. Pure functions get
  cheap table-driven tests. RPC handlers get realistic coverage using the existing
  test infrastructure.
- **Cons:** Slightly more files to write; needs to distinguish which helpers can be
  called directly vs. which require the full service stack.
- **Effort:** Medium.

### Option B: Integration-only tests that call full RPC endpoints
- **Approach:** Only write integration tests that exercise these functions indirectly
  by calling RPC endpoints, similar to `lifecycle_test.go` and `offers_test.go`.
- **Pros:** Simpler approach — reuse existing patterns without needing to think
  about which internal helpers to call directly.
- **Cons:** Pure conversion functions remain untested directly; table-driven
  exhaustiveness is harder to achieve; slow feedback loop for logic bugs.
- **Effort:** Small.

### Option C: Exhaustive unit tests via public-interface mocking
- **Approach:** Create mock implementations of the storage interface to test handlers
  with injected errors, edge cases, and failure modes without a real DB.
- **Pros:** Maximum isolation and speed; can test error injection precisely.
- **Cons:** The storage interface is not designed for mocking; `ProtoSQLStorage` is
  a concrete type used directly. Setting up mocks would require interface extraction
  first — out of scope.
- **Effort:** Large (requires refactor).

## Recommendation

**Option A** — targeted unit tests for pure functions + integration tests for RPC
handlers.

The existing test infrastructure (`setupTestServiceWithNotifications`) makes
integration tests straightforward and well-precedented. The pure conversion
functions (`modelRequestStateToApi`, `apiRequestStateToModel`,
`vulnerabilityLevelToString`, `buildHintFromStoredRequest`) are self-contained
and benefit from table-driven exhaustiveness. Event helpers and RPC handlers need
the full service stack because they write to the DB and trigger notifications.

## Open Questions

- **Should `buildRequest` be tested directly?** It's an internal method but
  exercises significant logic. Recommendation: test it indirectly via
  `GetRequest`/`ListRequests` (already covered in `queries_test.go`) and via the
  `ShareRequest` response — avoid duplicating what `queries_test.go` already
  covers.

- **Should `recordCommunityEventForRequest` be tested directly?** It's an
  internal method on `*Service`. Recommendation: yes — call it directly in
  `events_test.go` since it's accessible from within the package (same `package
  request` test files).

- **How many table cases for state conversion?** Recommendation: one case per
  defined enum value plus an explicit "unknown" case to nail the default branch.

## Implementation Plan

### Phase 1: Pure function tests (helpers_test.go)
**Goal:** Pin the state conversion and vulnerability string functions.
**Tasks:**
- [ ] Write `TestModelRequestStateToApi` (table-driven, all enum values + unknown)
- [ ] Write `TestApiRequestStateToModel` (table-driven, all enum values + unknown)
- [ ] Write `TestVulnerabilityLevelToString` (table-driven, all enum values + unknown)
**Validates:** Conversion regressions caught at unit-test speed.

### Phase 2: Event helper tests (events_test.go)
**Goal:** Verify event emission invariants for all state transitions.
**Tasks:**
- [ ] Write `TestRecordCommunityEventForRequest_Active` (emits REQUEST_CREATED)
- [ ] Write `TestRecordCommunityEventForRequest_Fulfilled` (emits REQUEST_FULFILLED)
- [ ] Write `TestRecordCommunityEventForRequest_Cancelled` (emits REQUEST_CANCELLED)
- [ ] Write `TestRecordCommunityEventForRequest_OffersReceived` (emits no event, returns "")
- [ ] Write `TestRecordCommunityEventForRequest_Unknown` (no event, no error)
- [ ] Write `TestRecordOfferMadeEvent` (emits REQUEST_OFFER_MADE)
- [ ] Write `TestRecordOfferWithdrawnEvent` (emits REQUEST_OFFER_WITHDRAWN)
**Validates:** Every event type is exercised; the OFFERS_RECEIVED no-op is
documented.

### Phase 3: Sharing RPC tests (sharing_test.go)
**Goal:** Verify ACL rules and share/unshare logic.
**Tasks:**
- [ ] Write `TestShareRequest_Success` (new community added, event recorded)
- [ ] Write `TestShareRequest_AlreadyShared` (idempotent — no duplicate record)
- [ ] Write `TestShareRequest_NonOwner` (returns PermissionDenied)
- [ ] Write `TestShareRequest_NonMember` (non-member community silently skipped)
- [ ] Write `TestUnshareRequest_Success` (community archived, returns empty response)
- [ ] Write `TestUnshareRequest_LastCommunity` (returns FailedPrecondition)
- [ ] Write `TestUnshareRequest_NonOwner` (returns PermissionDenied)
- [ ] Write `TestUnshareRequest_NotShared` (returns NotFound)
**Validates:** All ACL and precondition branches are covered.

### Phase 4: Stats RPC tests (stats_test.go)
**Goal:** Verify stats calculation for offers, fulfillment, days open.
**Tasks:**
- [ ] Write `TestGetRequestStats_NoOffers` (zero counts, zero days)
- [ ] Write `TestGetRequestStats_WithOffers` (offers_received count reflects active offers)
- [ ] Write `TestGetRequestStats_Fulfilled` (times_fulfilled = 1, impact populated)
- [ ] Write `TestGetRequestStats_WithCommunityFilter` (community_id filter applied)
- [ ] Write `TestBuildHintFromStoredRequest` (table-driven: nil, social context only, impact estimate only, both)
**Validates:** Stats calculation and hint building are pinned.

### Phase 5: People RPC tests (people_test.go)
**Goal:** Verify people assembly for requester, offerers, and total count.
**Tasks:**
- [ ] Write `TestGetRequestPeople_NoOfferers` (requester returned, total_count=1)
- [ ] Write `TestGetRequestPeople_WithOfferers` (offerers assembled, total_count correct)
- [ ] Write `TestGetRequestPeople_WithCommunityFilter` (community_id filter applied)
- [ ] Write `TestGetRequestPeople_RequiresAuth` (unauthenticated returns Unauthenticated)
- [ ] Write `TestGetRequestPeople_NotFound` (missing request_id returns NotFound)
**Validates:** People assembly, deduplication, and count logic are covered.

## Related Issues
- #1288 — impact metrics service tests (same pattern: adding tests for an undertested service)
- #1289 — auth repository tests (same pattern: adding test coverage to existing code)
