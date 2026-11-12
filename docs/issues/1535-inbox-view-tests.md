# Issue #1535: [testing] `portfolio/inbox_view.go` is 756 lines and untested

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1535
**Filed:** 2026-04-28 | **Last activity:** 2026-04-29
**Labels:** auto-filed, review:testing | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`server/services/portfolio/inbox_view.go` (756 lines) drives the user-visible
portfolio inbox screen but has zero direct test coverage. It is the largest
untested file in `server/services/`. The companion `inbox_week.go` is an
18-line deprecated stub that returns `connect.CodeUnimplemented` and has no
test pinning that contract. Every other file in the portfolio package was
tested this cycle, so `inbox_view.go` is the lone gap.

The file owns the assembly of three independently visible regions on the
inbox screen — timed items today, anytime items, and "Coming Up" — plus
the people row, the weekly metrics overlay, the actions-today list, and the
shared-inventory list. Item-mixing (transfers / experiences / requests),
isoweek boundaries, and authorization are all unprotected by tests, so an
ordering or assembly regression would only be caught by a manual QA pass.

## Context from Comments

The triage comment from `/ripls-triage` on 2026-04-28 set Issue Type → Task
and Priority → P1, with the rationale "testing High: largest untested
service file (756 lines); portfolio is Tier-2 Betty pain surface." A
follow-up `@claude` mention on 2026-04-29 by the same collaborator handed
the issue off for resolution. No additional constraints or direction were
provided in the comments. The issue body recommends following the
`metric_detail_test.go` and `assembly_test.go` patterns already in the
directory.

## Potential Solutions

### Option A: Pure-assembly refactor + table-driven tests
- **Approach:** Mirror the structure of `inbox_feed.go` / `inbox_feed_test.go`
  by extracting the body of `GetPortfolioInboxView` into a pure
  `assembleInboxView(d *fetchedData, extras *inboxViewExtras, userID string, tz *time.Location, now time.Time, weekly *api.WeeklyMetrics) *api.GetPortfolioInboxViewResponse`
  function that takes pre-fetched data and returns the response — no storage
  touches and no error return (mirrors `assembleFeedItems`'s pure shape).
  Move the storage calls inline in `GetPortfolioInboxView`
  (`fetchRecentlyArchivedRequests`, the `RequestOffer` `QueryByField`, and
  the storage-touching branch of `assembleSharedInventory`) into a sister
  `fetchInboxViewExtras` helper, and keep `s.computeWeeklyMetrics` as a
  service method called from the RPC handler (its result is passed into
  `assembleInboxView`). Then write `inbox_view_test.go` building hand-rolled
  `fetchedData` via `emptyFetchedData()` from `inbox_feed_test.go`,
  exercising authorization, item-type mixing, isoweek boundaries, sort
  order, and the empty-communities short-circuit. Add `inbox_week_test.go`
  pinning the `Unimplemented` contract.

**Audit note (Option A):** Corrected the proposed signature to drop the
`error` return (a pure assembler has no I/O — matches
`assembleFeedItems`) and to spell out the parameters that actually need
to flow in (extras, weekly metrics). Removed the claim that the four
storage calls land in `fetchAll`; the Open Questions and Phase 3 already
agree the right shape is a sister `fetchInboxViewExtras` helper plus
keeping `computeWeeklyMetrics` separate, so the headline option matches.
- **Pros:** Matches the codebase convention (`assembleFeedItems` is the
  blueprint). Tests run at unit-test speed with no DB. Coverage scales
  cheaply — adding a new item-type case is one helper call, not a
  testcontainer fixture. Forces a structural cleanup that makes future
  changes safer.
- **Cons:** Requires a real refactor of `inbox_view.go` plus changes to
  `fetch.go`, `metrics.go`, and `actions.go` to surface the data. Touches
  ~150 lines outside the test file.
- **Effort:** Medium.

### Option B: Helpers-only table tests, no refactor
- **Approach:** Cover only the already-pure helpers that live at the bottom
  of `inbox_view.go`: `deduplicateAllTransfers`,
  `deduplicateAndFilterTransfers`, `anytimeItemTypeOrder`,
  `buildComingUpGroups`, `collectUserIDs`,
  `collectCommunityItemOwnerIDs`, `buildCommunityPeople`. Add a
  `Unimplemented` test for `GetPortfolioInboxWeek`. Leave the
  `GetPortfolioInboxView` orchestration untested.
- **Pros:** Smallest delta. Lowest risk of regression in the refactor.
- **Cons:** Leaves the assembly logic — the part the issue specifically
  calls out ("inbox assembly, item-type mixing, week boundaries,
  authorization paths") — uncovered. The bug class the issue
  worries about ("assembly/ordering bugs surface as wrong items in front
  of end users") would not be caught.
- **Effort:** Small.

### Option C: Storage-backed integration test
- **Approach:** Spin up a `pgvector/pgvector:pg16` testcontainer with
  `storage.SetupTestStorage(t)`, seed transfers/experiences/requests/gear
  for a synthetic user, call `GetPortfolioInboxView` end-to-end, and
  assert on the response.
- **Pros:** Highest realism. Catches storage-layer interactions.
- **Cons:** No test in the portfolio package currently uses storage —
  this would be a new pattern. Slow (testcontainer per package, ~10–30 s
  startup), and table-driven exhaustiveness is hard. The package's existing
  pattern of building `fetchedData` directly is cheaper for the same
  coverage of assembly logic.
- **Effort:** Large.

## Recommendation

**Option A** — pure-assembly refactor + table-driven tests.

The portfolio package has a clear, recent precedent: `inbox_feed.go` was
written in exactly this shape, and `inbox_feed_test.go` (880 lines, 30+
table-driven cases) exercises every inclusion rule against hand-built
`fetchedData`. Mirroring that structure for `inbox_view.go` keeps the
package internally consistent and gives us the same cheap unit-test
exhaustiveness for the inbox screen.

Option B leaves the bug class the issue actually worries about —
assembly/ordering — uncovered. Option C breaks the package's pattern and
runs slowly for less specific coverage.

The refactor itself is the only meaningful risk. We mitigate it by
extracting `assembleInboxView` first as a thin wrapper that calls into the
existing inline body, verifying `npm run build` and existing tests still
pass, and only then moving the storage calls into `fetchAll`. Each phase
ships independently.

## Open Questions

- **Should `computeWeeklyMetrics` be moved into `fetchAll` or kept as a
  service method called separately?** It currently issues its own storage
  queries (transfers as owner / recipient) that overlap with `fetchAll`'s
  transfer query. Recommendation: leave `computeWeeklyMetrics` as a
  service method but make `assembleInboxView` accept its result as a
  parameter; the RPC handler stays the only place that joins fetched data
  with weekly metrics. This avoids a larger refactor of `metrics.go` and
  keeps the test surface focused.

- **Should `fetchRecentlyArchivedRequests` and the request-offers query be
  moved into `fetchAll` or kept inline?** Both run only on the
  inbox-view code path. Recommendation: move both into a sister
  `fetchInboxViewExtras(ctx, userID, fetchedData, todayStart, todayEnd)
  (*inboxViewExtras, error)` helper called from the RPC handler; pass
  the result into `assembleInboxView`. Keeps `fetchAll` unchanged so
  `inbox_feed.go` is unaffected.

- **Should `assembleSharedInventory` be split?** It has a storage call
  for completed-transfer history. Recommendation: extract the storage
  fetch into the new `fetchInboxViewExtras` helper, leave the
  pure-assembly half in `actions.go` callable from `assembleInboxView`.

- **What does `inbox_week_test.go` actually pin?** The file just returns
  `Unimplemented`. Recommendation: a single
  `TestGetPortfolioInboxWeek_Unimplemented` table case that asserts the
  RPC returns `connect.CodeUnimplemented`. If we ever delete the stub,
  the test deletes with it.

- **How exhaustive should the assembly table tests be?** Recommendation:
  one table-driven case per item-type × inclusion-rule branch
  (transfer materialToday vs unread-only; experience timed vs anytime
  vs coming-up; request anytime; coming-up label boundaries Tomorrow /
  Later This Week / Next Week; pickup and return windows). Mirror
  `inbox_feed_test.go`'s naming.

## Implementation Plan

### Phase 1: Cover already-pure helpers
**Goal:** Lock in the trivially testable functions immediately so the
phase-2 refactor has a safety net.
**Tasks:**
- [ ] Create `server/services/portfolio/inbox_view_test.go`.
- [ ] Add `TestDeduplicateAllTransfers` — table-driven covering: empty
  inputs, owner-only, recipient-only, owner+recipient overlap, deleted
  transfer skipped (regardless of state), terminal-state transfer kept
  (unlike `deduplicateAndFilterTransfers`).
- [ ] Add `TestDeduplicateAndFilterTransfers` — same shape but asserting
  `TRANSFER_STATE_COMPLETED` and `TRANSFER_STATE_CANCELLED` are dropped
  while non-terminal states are kept.
- [ ] Add `TestAnytimeItemTypeOrder` — table-driven over all
  `DailyItemType` values plus an unknown default; assert REQUEST < EXPERIENCE < everything else.
- [ ] Add `TestBuildComingUpGroups` — verify: empty input → nil; group
  ordering is `Tomorrow`, `Later This Week`, `Next Week`; items inside a
  group sorted ascending by `TimeUnixSec`; unknown labels dropped.
- [ ] Add `TestCollectUserIDs` — owner + recipient IDs from transfers,
  experience owner + attendees, request requester + confirmed helpers,
  empty IDs filtered, dedup across categories.
- [ ] Add `TestCollectCommunityItemOwnerIDs` — gear owners always
  included, archived community items skipped, unresolved IDs (in
  `commExpRaw` but missing from `expMap`) silently skipped.
- [ ] Add `TestBuildCommunityPeople` — self excluded, archived items
  excluded, deleted gear/experiences/requests excluded, ordering by most
  recent `sharedAt` descending, unresolved users (missing from
  `userMap`) dropped, ties resolved deterministically.
- [ ] Run `go test ./server/services/portfolio/...` — green.
**Validates:** All currently-pure helpers have direct coverage; nothing
moved yet.

### Phase 2: Pin the deprecated week endpoint
**Goal:** Lock the `Unimplemented` contract on `GetPortfolioInboxWeek`
so a refactor cannot silently change it.
**Tasks:**
- [ ] Create `server/services/portfolio/inbox_week_test.go`.
- [ ] Add `TestGetPortfolioInboxWeek_Unimplemented` calling the method
  on a zero-value `Service` (no storage needed) and asserting the
  returned error is a `*connect.Error` with `connect.CodeUnimplemented`.
- [ ] Run `go test ./server/services/portfolio/...` — green.
**Validates:** Deprecated stub behavior pinned; the file can be deleted
later without surprises.

### Phase 3: Extract `assembleInboxView` (pure)
**Goal:** Carve out the assembly body of `GetPortfolioInboxView` into a
pure function callable from tests, with no behavior change.
**Tasks:**
- [ ] In `inbox_view.go`, add a private helper
  `assembleInboxView(d *fetchedData, extras *inboxViewExtras, userID string, tz *time.Location, now time.Time, weekly *api.WeeklyMetrics) *api.GetPortfolioInboxViewResponse`
  containing the in-line body of the existing handler from "Assemble
  DailyItems with community names" through the final `connect.NewResponse`.
- [ ] Define `type inboxViewExtras struct { recentArchivedReqs map[string]proto.Message; offeredHelpOffers map[string]int64; sharedValue string; sharedItems []*api.SharedItem }`
  (concrete fields TBD during implementation; goal is "everything
  storage-touched outside `fetchAll`"). Note that `assembleSharedInventory`
  already returns `(string, []*api.SharedItem)` — the `string` is the
  formatted total value that lands in `GetPortfolioInboxViewResponse.SharedValue`,
  which is a proto `string` field, not a message type.
- [ ] Add a sister `fetchInboxViewExtras(ctx, s, d, todayStart, todayEnd)
  (*inboxViewExtras, error)` private helper that performs the three
  remaining storage calls currently inline in the handler
  (`fetchRecentlyArchivedRequests`, request-offers query, the
  storage-touching branch of `assembleSharedInventory`) and returns
  the populated extras struct.
- [ ] Rewrite `GetPortfolioInboxView` as: auth → tz/now → `fetchAll` →
  empty-communities short-circuit → `computeWeeklyMetrics` →
  `fetchInboxViewExtras` → `assembleInboxView`.
- [ ] No change to `assembleSharedInventory`'s exported pure half;
  callable from `assembleInboxView` with pre-fetched arguments.
- [ ] Run `npm run build`, `go test ./server/services/portfolio/...`,
  and `go test -tags=integration -timeout 20m
  ./server/services/portfolio/...` — all green. The Phase 1 tests must
  still pass unchanged.
**Validates:** Refactor preserves behavior. Assembly is now testable
without storage.

**Audit note (Phase 3):** Replaced the placeholder field types
`*api.PortfolioSharedItem` and `*api.SharedValue` (neither exists) with
the actual API types: `[]*api.SharedItem` and `string`. The proto
contract is `GetPortfolioInboxViewResponse.shared_value` (string) and
`shared_items` (repeated `SharedItem`), and `assembleSharedInventory`
already returns `(string, []*api.SharedItem)`. Stating the wrong type
names risked a downstream confusion between API and Models proto
packages — `api/portfolio.proto` is canonical here.

### Phase 4: Table-driven tests for `assembleInboxView`
**Goal:** Cover inbox assembly, item-type mixing, week boundaries, and
authorization paths as the issue specifies.
**Tasks:**
- [ ] Reuse `emptyFetchedData()`, `makeLoanTransfer`, `makeGiveawayTransfer`,
  `makeGear`, `makeExperience`, `makeCommunityExperience`,
  `makeRequest`, `makeCommunityRequest`, and `makeCommunityGear` from
  `inbox_feed_test.go`. Add minimal `inboxViewExtras` builder helper
  in the new test file.
- [ ] Add `TestAssembleInboxView_NoCommunities` — empty communityIDs in
  `fetchedData` → response has only `DateLabel` and an empty
  `WeeklyMetrics` with the correct goal. (Mirror the
  short-circuit branch of the handler, which we'll keep there;
  optionally lift to assembleInboxView.)
- [ ] Add `TestAssembleInboxView_TransferAnytime_MaterialToday` — a
  transfer with a `LatestRequestUnixSec` inside today's bounds and
  zero unread is included in `AnytimeItems`.
- [ ] Add `TestAssembleInboxView_TransferAnytime_UnreadOnly` — a
  transfer with unread > 0 outside today's bounds is included.
- [ ] Add `TestAssembleInboxView_TransferAnytime_Excluded` — transfer
  with no unread and no material-today activity is dropped.
- [ ] Add `TestAssembleInboxView_ExperienceTimedToday` — experience
  whose Specific time is inside today's bounds lands in
  `TimedItems` with `UnreadCount` accumulated.
- [ ] Add `TestAssembleInboxView_ExperienceComingUp_Tomorrow` — an
  experience scheduled tomorrow lands under `Tomorrow`.
- [ ] Add `TestAssembleInboxView_ExperienceComingUp_LaterThisWeek` —
  scheduled day-after-tomorrow → `Later This Week`.
- [ ] Add `TestAssembleInboxView_ExperienceComingUp_NextWeek` —
  scheduled past current isoweek end → `Next Week`.
- [ ] Add `TestAssembleInboxView_ExperienceTerminal_Excluded` —
  COMPLETED and CANCELLED experiences are skipped from coming-up.
- [ ] Add `TestAssembleInboxView_RequestAnytime` — a request whose
  fields populate correctly produces a `DAILY_ITEM_TYPE_REQUEST` item.
- [ ] Add `TestAssembleInboxView_TransferComingUp_Pickup` — transfer
  with `EstimatedPickupUnixSec` ≥ todayEnd appears in coming-up with
  the pickup label.
- [ ] Add `TestAssembleInboxView_TransferComingUp_Return` — non-giveaway
  transfer with `LoanDurationDays` and `ActualPickupUnixSec` set such
  that the return date is in the future appears under the return
  label.
- [ ] Add `TestAssembleInboxView_AnytimeOrder` — mixed REQUEST,
  EXPERIENCE, and TRANSFER items are sorted REQUEST→EXPERIENCE→TRANSFER,
  with stable secondary order by `UnreadCount` descending.
- [ ] Add `TestAssembleInboxView_TimedOrder` — multiple timed items
  ascending by `TimeUnixSec`.
- [ ] Add `TestAssembleInboxView_TotalUnreadAggregation` — verifies
  unread counts from transfers, experiences, requests, and coming-up
  experiences all sum into `TotalUnreadCount`.
- [ ] Add `TestAssembleInboxView_People_Aggregated` — `People` row is
  populated from `buildCommunityPeople` output deterministically.
- [ ] Add `TestGetPortfolioInboxView_Unauthenticated` — call the RPC
  with a context that lacks auth and assert
  `connect.CodeUnauthenticated` is returned. (Auth wraps the entire
  handler; this is the "authorization paths" line item from the
  issue.)
- [ ] Run `go test ./server/services/portfolio/... -run InboxView` — all
  green; then full `go test ./server/services/portfolio/...` and
  `go test -tags=integration -timeout 20m ./server/services/portfolio/...`.
- [ ] Run `npm run build` to verify lint passes.
**Validates:** All major branches of the inbox view assembly have
direct coverage; the file is no longer the largest untested file in
`server/services/`.

## Related Issues
- #1361 — request service helpers untested (same pattern: tests for an
  undertested service file in `server/services/`).
- #1288 — impact metrics service tests (same pattern).
- #1289 — auth repository tests (same pattern).
- #1360 — feed-helper tests (closely adjacent: `inbox_feed.go` is the
  pattern this plan mirrors).
