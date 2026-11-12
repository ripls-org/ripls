# Issue #1051: Completed gives still listed

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1051
**Filed:** 2026-04-06 | **Last activity:** 2026-05-21
**Labels:** user-feedback | **Assignees:** unassigned
**Priority:** P1 (triaged 2026-04-24)
**Context docs:** none

## Problem Statement

A user (Leslie, prod, 0.9.3) reports that an item she gifted via a giveaway
still appears in her "My Items" inventory with the **GIVING** badge even though
the transaction has been completed. She describes it as "misleading clutter in
the inventory."

The user's mental model: once a giveaway is complete, the item has left her
possession and should no longer appear in her active inventory (or at the very
least should not still be labeled "GIVING," which implies an in-flight
transfer).

The bug has two related but distinct symptoms a fix must address:

1. **Clutter** — the item appears at all in the inventory list after the
   giveaway completes.
2. **Misleading badge** — even if the item is allowed to appear, the active
   "GIVING" transfer badge is wrong: there is no active transfer; it completed.

## Context from Comments

- **2026-04-24 (`maintainer`, triage)**: assigned P1 with rationale "Product
  lens — high-impact issue for key personas tied to top-tier priorities
  (story activation, core loops, or growth use cases)."
- **2026-05-21 (`maintainer`)**: `@claude` ping to start work.

No technical clarifications from the author; the reproduction is the
screenshot only. No disagreement in the thread.

## Root Cause Analysis

Three independent code paths combine to produce the bug. Files and line
references are at HEAD on `main` as of this plan.

### 1. Server: `ListUserGear` returns given-away gear

`server/services/gear/service.go:755` queries gear strictly by
`owner_id = current_user`, with no filter on `Gear.state`. When a giveaway
completes, `server/services/transfer/state_machine.go:354 completeGiveaway`
deliberately keeps `owner_id` pointing at the original owner and sets
`Gear.state = GEAR_STATE_GIVEN_AWAY` instead. The design comment is explicit:
"Gear ownership stays with the original owner (not transferred to recipient)."
So given-away gear keeps coming back from `ListUserGear` forever.

### 2. API contract: `GearItem` omits `state`

The list-shape message `GearItem` in
`proto/ripls/api/gear_service.proto:200` (used by `ListUserGearResponse`) does
**not** include the `GearItemState state` field, even though the singular
`GetGearResponse` (line 118) does. The client therefore cannot tell
locally that a gear item is in `GIVEN_AWAY` state — it has to either ask
again per item or trust the server to filter.

### 3. Client: `transfersByGearId` keeps the completed transfer as "active"

`app/lib/presentation/viewmodels/profile_gear_view_model.dart:482-485` builds
`transfersByGearId` by iterating every transfer the user owns (from
`listMyTransfers`) and writing
`transfersByGearId[transfer.gearId] = transfer` unconditionally. There is no
state filter and no tiebreak when multiple transfers exist for one gear, so
a completed giveaway lands in the map as if it were active. The card builder
at `portfolio_items_screen.dart:260` reads
`activeTransfer = state.transfersByGearId[gear.id]` and forwards it to
`PortfolioItemCard`, which renders the **GIVING** badge from `transferType`
without checking `state`.

Note: the `ListMyTransfers` RPC itself (`server/services/transfer/queries.go:33-44`)
*does* accept an optional `state` filter; the client just never passes one.
The client filter is still the right place for this fix (defense in depth),
but it is not because the RPC is unfilterable.

The `ListCompletedGiveaways` RPC (`community_service.dart:456`) already
exists for surfacing past gifts in the community context, so completed
giveaways have a separate, intentional home — there is no reason to keep
them in the active "My Items" list.

## Potential Solutions

### Option A: Server-side filter on `ListUserGear`

Filter out `Gear.state == GEAR_STATE_GIVEN_AWAY` (and any future terminal
states) at the storage query in
`server/services/gear/service.go ListUserGear`. Push the filter down to SQL
via `QueryByFields` so we are not loading and discarding rows in Go.

- **Pros:** Fixes the clutter complaint at the root and is the smallest
  user-visible change. Single call site for the affected query
  (`profile_gear_view_model.dart:469` is the only consumer in the Flutter
  app). Network payload shrinks. Aligns with the design intent
  (`completeGiveaway` already marks the gear terminally; the inventory list
  shouldn't show terminal-state items). Keeps `owner_id` semantics
  unchanged so impact-history queries that read by owner still work.
- **Cons:** Does not address the underlying badge bug — a stale completed
  transfer would still attach a misleading "GIVING" badge to other gear if
  any future code path showed it. Server-only fixes are invisible to the
  client cache until the user pulls to refresh or the cache TTL expires.
- **Effort:** Small.

### Option B: Client-side filter on `transfersByGearId`

In `profile_gear_view_model.dart` `loadGear`, drop transfers whose `state`
is `TRANSFER_STATE_COMPLETED` or `TRANSFER_STATE_CANCELLED` before building
the `transfersByGearId` map. Optionally also tiebreak: pick the
chronologically latest non-terminal transfer per gear.

- **Pros:** Fixes the misleading badge for every consumer of
  `transfersByGearId`, defensively. Pure client change, ships with the next
  app release.
- **Cons:** Leaves the given-away gear in the inventory list (no badge, but
  still clutter). Doesn't address Leslie's primary complaint ("clutter in
  the inventory"). Requires a client release, not a server hotfix.
- **Effort:** Small.

### Option C: Combined — server filter + client filter (recommended)

Do both A and B, plus add the missing `state` field to the API `GearItem`
message so the client has the data to make richer decisions in the future
(e.g. a separate "Given away" history tab):

1. Add `GearItemState state = 11;` to `GearItem` in
   `proto/ripls/api/gear_service.proto`, populate it in
   `ListUserGear`, run `npm run generate`.
2. Filter `state == GEAR_STATE_GIVEN_AWAY` out of `ListUserGear` at the
   storage query layer.
3. In `profile_gear_view_model.dart`, when building `transfersByGearId`,
   skip transfers in terminal states (`TRANSFER_STATE_COMPLETED`,
   `TRANSFER_STATE_CANCELLED`) and prefer the latest non-terminal transfer
   per gear on ties.

- **Pros:** Fixes the user-visible bug end to end. Defense in depth: even if
  one layer regresses, the other still hides the misleading state. Sets up
  a clean follow-up to add a "Given away" history view by reading
  `GEAR_STATE_GIVEN_AWAY` items via a new RPC (or an existing one filtered
  by state) — out of scope for this fix.
- **Cons:** Touches three layers (proto, server, client). Proto change
  requires regen. Slightly larger than the minimal patch.
- **Effort:** Small-medium.

### Option D: Default `ListMyTransfers` to non-terminal

Make `ListMyTransfers` exclude terminal states by default, with an opt-in
flag to include them.

- **Pros:** Fixes the source of the stale data, not just one consumer.
- **Cons:** Behavior change for an RPC with multiple call sites; risk of
  breaking screens that legitimately want history (e.g. transfer detail,
  metrics). Not the right place to fix this user-facing bug — would need
  audit of every consumer. Recommend as a separate, larger refactor
  tracked in its own issue.
- **Effort:** Medium, with audit risk.

## Recommendation

**Option C.** It directly addresses both symptoms Leslie reported (clutter
and misleading badge) and leaves no plausible path by which a future change
could resurrect the bug from either side. The proto addition is a wire-
compatible field-number extension (per the optional/scalar guidance in
`CLAUDE.md`) and unlocks a future "Given away" history tab without a second
schema change.

If the reviewer prefers a minimal hotfix because the proto change feels
heavyweight, fall back to **Option A only**: it shrinks the patch to one
file and fully resolves the user-visible bug. Option B alone is not
sufficient because the user-reported "clutter" complaint would remain.

## Open Questions

- **Should given-away gear surface anywhere in the owner's UI (e.g. a
  "History" or "Given Away" section)?** —
  Recommendation: not in this fix. Existing `ListCompletedGiveaways` is
  community-scoped and is the right hook for a future "My giveaway history"
  view; that work belongs in a follow-up issue. The current fix's only
  obligation is to stop the given-away item from appearing as active
  inventory.

- **Should the `state` field on `GearItem` be `optional`?** —
  Recommendation: yes, per the proto-field convention in `CLAUDE.md`:
  "Use `optional` for conditionally-present fields." Old clients/old data
  benefit from explicit presence tracking. Default to
  `GEAR_ITEM_STATE_UNSPECIFIED` server-side when state is not set on
  storage.

- **Should `ListMyTransfers` be changed at the same time?** —
  Recommendation: no, file a separate issue. The Flutter client filter
  (Option B inside C) addresses the bug for the affected screen; a wider
  API contract change needs its own audit of every consumer (transfer
  detail, conversation context, impact metrics, etc.).

- **Cache invalidation after `CompleteTransfer`?** —
  Confirmed gap: `TransferRepository.completeTransfer`
  (`app/lib/data/repositories/transfer_repository.dart:302-340`) calls
  `_gearRepository.invalidate(transfer.gearId)` (item key) and
  `invalidateStats(...)`, but does **not** call `refreshUserGear()` or
  invalidate the `gear:user:list` cache key. That means after a
  giveaway completes, the cached `ListUserGear` response still contains
  the given-away gear until the global TTL expires (5–60 minutes
  depending on env). The Phase 1 server filter alone will not fix
  Leslie's report on the same session — the client must also invalidate
  the user gear list. Add `await _gearRepository.refreshUserGear()`
  to the giveaway branch (or unconditionally) inside
  `TransferRepository.completeTransfer`. Cover this with a
  TransferRepository test that asserts `refreshUserGear()` is invoked
  after a giveaway completion.

## Implementation Plan

### Phase 1: Server-side filter on `ListUserGear`

**Goal:** Stop the server from returning given-away gear in the inventory
list. Single-line behavior change with the largest user-visible payoff.

**Tasks:**
- [ ] In `server/services/gear/service.go` `ListUserGear` (around line 755),
  filter out `GEAR_STATE_GIVEN_AWAY`. **Storage constraint:** `QueryByFields`
  (`server/storage/protosql.go:282-345`) builds an equality `AND` WHERE
  clause only — it does **not** support `!=`, `NOT IN`, or arbitrary
  predicates. The available alternatives are:
    1. **(Recommended)** Keep the existing
       `s.storage.QueryByField(ctx, "owner_id", authInfo.UserID, &models.Gear{})`
       call and filter `gearStored.State == models.GearState_GEAR_STATE_GIVEN_AWAY`
       out in the existing `for _, msg := range messages` loop before
       appending to `items`. Per-user gear counts are bounded (small),
       and this avoids adding a multi-value query helper.
    2. Add a positive equality filter for the one currently-active state
       (`GEAR_STATE_AVAILABLE`), but this drops `GEAR_STATE_UNAVAILABLE`
       and `GEAR_STATE_UNSPECIFIED` gear, which the user still owns and
       expects to see — **not acceptable**.
    3. Extend `storage/` with a new `QueryByFieldNotIn` helper. Out of
       scope for this fix; file a separate issue if a server-side
       pattern emerges.
  Add a debug log line after filtering with the dropped count
  (`logger.DebugContext(ctx, "filtered terminal-state gear", "dropped_count", n)`)
  so production logs show the filter is working without leaking gear IDs.
- [ ] Update `server/services/gear/service_test.go TestService_ListUserGear`
  to add a fixture where one of the user's gear items is in
  `GEAR_STATE_GIVEN_AWAY` and assert it is excluded.
- [ ] Run `npm run build` (golangci-lint + `go build ./...`) and
  `go test ./server/services/gear/...`. `TestService_ListUserGear` is a
  unit/storage test (not integration-tagged), so the `-tags=integration`
  flag is not required for this phase — but run
  `go test -tags=integration ./...` as well per CLAUDE.md if any exported
  signature changes (none expected here).

**Validates:** The reported reproduction — completed-giveaway gear no
longer appears in "My Items" — works on a server-only deploy with no app
release required.

### Phase 2: Client-side filter on `transfersByGearId`

**Goal:** Defense in depth — even if a stale completed transfer makes it
to the client, no misleading "GIVING" badge ever renders.

**Tasks:**
- [ ] In `app/lib/presentation/viewmodels/profile_gear_view_model.dart`
  `loadGear` (around line 482), skip transfers whose `state` is
  `TRANSFER_STATE_COMPLETED` or `TRANSFER_STATE_CANCELLED` before writing
  to `transfersByGearId`. When multiple non-terminal transfers exist for
  one gear, keep the most recent one (currently last-write-wins on an
  unsorted iteration; this is a latent correctness bug worth fixing in the
  same change).
- [ ] In `app/lib/data/repositories/transfer_repository.dart`
  `completeTransfer` (around line 302-340), add
  `await _gearRepository.refreshUserGear();` so the cached
  `gear:user:list` does not retain the given-away item until the global
  TTL expires. The existing `_gearRepository.invalidate(transfer.gearId)`
  call only clears the per-item cache, not the list. Without this, a user
  who completes a giveaway and immediately navigates to "My Items" still
  sees the stale list from the cache.
- [ ] Extend the existing test file
  `app/test/presentation/viewmodels/profile_gear_view_model_test.dart`
  (already present — uses `ProviderContainer` + Mockito-generated
  `MockGearRepository` / `MockTransferRepository`; add cases to the
  existing `group('ProfileGearViewModel', ...)`) covering:
  (a) completed giveaway transfer is not in `transfersByGearId`;
  (b) cancelled transfer is not in `transfersByGearId`;
  (c) when both an active loan and a stale completed giveaway exist for
  the same gear, the active loan wins. After adding mocks for any new
  repository methods (e.g. `refreshUserGear`), regenerate mocks with
  `cd app && dart run build_runner build --delete-conflicting-outputs`.
- [ ] Add a TransferRepository test in
  `app/test/data/repositories/transfer_repository_test.dart` (or extend
  the existing file) asserting that `completeTransfer` for a
  `TRANSFER_TYPE_GIVEAWAY` invokes `_gearRepository.refreshUserGear()`.
- [ ] Run `cd app && flutter test --reporter expanded`. The
  `--reporter expanded` flag is required per CLAUDE.md — the default
  `compact` reporter uses carriage returns that produce huge output when
  piped.

**Validates:** No misleading badges regardless of what the server returns,
including for offline-cached transfers from before the Phase 1 deploy, and
no stale `ListUserGear` cache entry retains the given-away item between
the completion action and the global cache TTL.

### Phase 3: Add `state` to `GearItem` proto

**Goal:** Give the client first-class visibility into gear state so a
future "Given away" history view (and any other status-aware UI) can be
built without a second proto round trip.

**Tasks:**
- [ ] Add `optional GearItemState state = 11;` to message `GearItem` in
  `proto/ripls/api/gear_service.proto`. The `GearItemState` enum is
  already defined at line 18 of the same file. Field 11 is the next
  free tag (field 2 was historically skipped; current last tag is
  `created_at_unix_sec = 10`).
- [ ] Comment on the new field must describe **what** the field means
  to a client ("the gear item's lifecycle state"), not why the proto
  uses `optional` or how the server populates it. Proto comments are
  the API contract; do not reference CLAUDE.md conventions, transport
  details, or storage mechanics there. (Per CLAUDE.md "Proto Comments".)
- [ ] Run `npm run generate` to regenerate Go and Dart bindings (do not
  hand-edit `server/gen/` or `app/lib/data/gen/`).
- [ ] Populate `State: convertGearState(gearStored.State)` (Go field
  name is `State`, not `state`) on each `&api.GearItem{...}` in
  `server/services/gear/service.go ListUserGear`. The helper
  `convertGearState` at line 916 already maps storage → API state and
  handles `GEAR_STATE_GIVEN_AWAY` correctly.
- [ ] Update the test added in Phase 1 to assert the API `State` field is
  populated correctly on the returned items.
- [ ] Run `npm run build`. Then run `go test ./server/services/gear/...`
  and `go test ./...`. Only run `go test -tags=integration ./...` (and
  `go build -tags=integration ./...`) if any exported signature
  consumed by an integration test changes — adding an optional proto
  field is wire-compatible and should not affect signatures, so this
  is precautionary.

**Validates:** The wire format now carries the data needed for the
follow-up "given away" history view, without requiring this issue to
build that UI.

## Related Issues

- None directly. A follow-up issue should be filed for surfacing
  completed/given-away gear in the owner's UI (history section), which
  this fix intentionally does not build.

## Audit changes

Adjusted during architecture/convention audit (see `ripls-audit`):

- **Storage API correctness (Phase 1):** Verified `QueryByFields`
  (`server/storage/protosql.go:282-345`) only supports equality `AND`
  conjunctions — no `!=`, `NOT IN`, or arbitrary predicate support.
  Rewrote Phase 1 tasks to recommend the Go-side filter on top of the
  existing `QueryByField("owner_id", ...)` call and explicitly rejected
  the "positive-state equality" alternative (which would drop
  `GEAR_STATE_UNAVAILABLE` / `UNSPECIFIED` gear the user still owns).
- **Cache invalidation gap (Phase 2):** Inspected
  `TransferRepository.completeTransfer` and confirmed it invalidates the
  per-item gear cache and stats but **does not** invalidate
  `gear:user:list`. Added a task to call
  `_gearRepository.refreshUserGear()` from the giveaway completion path
  so the Phase 1 server fix is actually visible to the user in the
  current session (otherwise the cached list keeps the stale item for
  up to the global TTL). Cross-referenced caching doc
  (`docs/client/caching.md` — "Repositories handle mutations and
  automatically invalidate caches").
- **Test conventions (Phase 2):** Confirmed
  `app/test/presentation/viewmodels/profile_gear_view_model_test.dart`
  already exists with the standard `ProviderContainer` +
  Mockito-generated mocks pattern. Updated Phase 2 to *extend* the
  existing file rather than create it, added the mock-regeneration step
  (`dart run build_runner build --delete-conflicting-outputs`), and
  added a TransferRepository test for the new cache-invalidation call.
  Added the required `--reporter expanded` flag per CLAUDE.md.
- **Proto comments (Phase 3):** Added a task reminding the implementer
  that proto comments are the API contract — must describe what `state`
  *means* to clients, not why the field is `optional` or how the server
  populates it (per CLAUDE.md "Proto Comments"). Confirmed field tag 11
  is free on `GearItem` (current last is 10).
- **Inaccurate root-cause claim:** Removed the claim that
  `ListMyTransfers` does not filter by state. The RPC at
  `server/services/transfer/queries.go:33-44` *does* accept an optional
  `state` filter; the client just never passes one. Defense-in-depth
  client filter is still the right fix, but for the right reason.
- **Observability:** Added an optional `dropped_count` debug log on the
  Phase 1 filter (no gear IDs) so prod logs surface the filter taking
  effect, consistent with `docs/server/observability.md`.
- **Build verification:** Pulled exact commands forward per CLAUDE.md
  ("Go Server Build Verification") — `npm run build` for golangci-lint
  + `go build ./...`, with `-tags=integration` only when a signature
  consumed by an integration test changes.
- Left **Option C as the recommendation** intact; the underlying
  three-layer fix is consistent with the architecture (server filter at
  the storage edge per `docs/server/architecture.md`, repository-owned
  cache invalidation per `docs/client/caching.md`, ViewModel-only
  business logic per `docs/client/architecture.md`).
