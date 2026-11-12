# Issue #728: End-to-end tests need to clean themselves up

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/728
**Filed:** 2026-02-12 | **Last activity:** 2026-03-24
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

E2E tests in `server/e2e_test.go` run against development infrastructure (when
`E2E_SERVER_URL` is set) but never clean up the data they create. Each test run
leaves behind users, communities, gear, media, transfers, conversations, and
other entities. Over time this pollutes the dev environment, causes flaky CI
from stale data interference, and slows development velocity.

The test file already has placeholder cleanup functions that log "not
implemented" messages (lines 236-238 for users, lines 267-268 for media).
`cleanupGear()` works via the existing `DeleteGear` RPC, but the broader
entity graph (users, communities, and their cascading dependents) has no
cleanup path from the e2e tests.

## Context from Comments

- **Triage (tescobar, 2026-03-05):** Rated P2. Test reliability affects
  development velocity — flaky CI from leftover test data slows feature
  delivery.
- **maintainer (2026-03-24):** The fix should add minimal RPC functionality
  to delete what is created, gated behind the existing `devMode` flag to
  protect production infrastructure.

## Potential Solutions

### Option A: Use existing CleanupSimulation RPC with simulation_id tagging

The infrastructure for this already exists:

- `RegisterRequest` has an `optional string simulation_id` field (field 6)
- `CreateCommunityRequest` has an `optional string simulation_id` field (field 4)
- `AdminService.CleanupSimulation` RPC accepts a `simulation_id` and cascades
  through 15+ entity types (equity, events, gear, transfers, requests,
  experiences, chat, media, etc.)
- `CleanupSimulation` is already gated behind `devMode`

The e2e tests would: (1) generate a unique simulation ID per test run, (2) pass
it when creating users and communities, (3) call `CleanupSimulation` in
`t.Cleanup()` or `defer`.

- **Approach:** Tag all e2e-created entities with a simulation ID, then call
  CleanupSimulation at the end of each test (or test suite).
- **Pros:** Zero new RPCs needed. Battle-tested cascade deletion logic already
  exists. Consistent with how the system was designed. Handles the full
  dependency graph (users → gear → transfers → conversations → equity → etc.)
  in a single call. Protects production via devMode gate.
- **Cons:** Requires the e2e tests to pass simulation_id on every create call
  (minor wiring). CleanupSimulation does hard deletes, not soft deletes — but
  this is appropriate for test data.
- **Effort:** Small.

### Option B: Add individual delete RPCs per entity type

Add `DeleteUser`, `DeleteMedia`, `DeleteCommunity`, etc. RPCs gated behind
devMode.

- **Approach:** Implement deletion RPCs for each entity the tests create, wire
  them into cleanup functions.
- **Pros:** Fine-grained control — each test cleans up exactly what it created.
- **Cons:** Substantial new code: multiple new RPCs, proto definitions, service
  implementations, cascade logic for each entity type. Duplicates work that
  `CleanupSimulation` already does. More surface area to maintain. Higher risk
  of incomplete cleanup if a new entity type is added but the delete RPC isn't.
- **Effort:** Large.

### Option C: Database truncation endpoint (like patrol test server)

The patrol test server already has a `/test-reset` endpoint that runs
`TRUNCATE TABLE ... CASCADE` across all tables.

- **Approach:** Add a similar endpoint to the main server (devMode only), call
  it between tests.
- **Pros:** Guarantees a clean slate. Simple implementation.
- **Cons:** Nuclear option — destroys ALL data, not just test data. Unusable
  when running e2e tests against a shared dev environment with real user data.
  Can't run tests in parallel. Not suitable for the `E2E_SERVER_URL` scenario
  where the dev server has persistent state we want to preserve.
- **Effort:** Small, but wrong scope.

## Recommendation

**Option A** — use the existing `CleanupSimulation` RPC with `simulation_id`
tagging. This is the clear winner:

1. The infrastructure already exists and is tested.
2. It handles the complete dependency graph correctly.
3. It's scoped to test data only (won't destroy real dev data).
4. It's already gated behind devMode.
5. It requires minimal new code — just wiring in the e2e tests.

The comment from maintainer says "add minimal RPC functionality to delete
what is created" — CleanupSimulation is exactly that functionality, and it
already exists.

## Open Questions

- **Should cleanup happen per-test or per-suite?** — Recommendation: Per-test.
  Each test gets its own simulation ID and cleans up after itself via
  `t.Cleanup()`. This keeps tests isolated and makes failures easier to debug
  (you know exactly which test left data behind if cleanup fails). The overhead
  of one CleanupSimulation call per test is negligible.
- **Should we clean up on test failure too?** — Recommendation: Yes, use
  `t.Cleanup()` which runs regardless of pass/fail. If we skip cleanup on
  failure for debugging, add a `E2E_SKIP_CLEANUP=true` env var escape hatch.
- **What about the E2E_SERVER_URL (remote dev server) case?** —
  Recommendation: CleanupSimulation works over the network, so it works for
  remote servers too — just create an AdminService client and call it. The
  remote server must be running in devMode (which it does — the dev deployment
  uses `-dev-mode`).

## Implementation Plan

### Phase 1: Wire simulation_id into e2e test helpers

**Goal:** All e2e-created entities are tagged with a simulation ID and cleaned
up after each test.

**Tasks:**
- [ ] Add a `generateSimulationID(testName string) string` helper that creates
  a unique simulation ID per test (e.g., `e2e-{testName}-{timestamp}`)
- [ ] Update `createUniqueTestUser()` to accept and pass `simulation_id` in
  the `RegisterRequest`
- [ ] Update community creation calls in tests to pass `simulation_id` in
  `CreateCommunityRequest`
- [ ] Replace the no-op cleanup functions with a real cleanup function that
  calls `AdminService.CleanupSimulation` via a Connect client
- [ ] Wire cleanup into each test via `t.Cleanup()` so it runs on both pass
  and failure
- [ ] Add `E2E_SKIP_CLEANUP` env var support for debugging (skip cleanup when
  set)
- [ ] Remove the `cleanupGear()` helper — it's no longer needed since
  CleanupSimulation handles gear deletion as part of the cascade
- [ ] Run the e2e tests locally and verify cleanup happens (check logs for
  CleanupSimulation response counts)

**Validates:** Test data is fully cleaned up after each test run, both locally
and against remote dev servers.

### Phase 2: Verify on CI and against dev environment

**Goal:** Confirm cleanup works in the real deployment scenario.

**Tasks:**
- [ ] Run e2e tests against the dev server (`E2E_SERVER_URL`) and verify data
  is cleaned up
- [ ] Verify `ListSimulations` shows no leftover e2e simulation IDs after a
  clean test run
- [ ] Run existing e2e cleanup for any accumulated test data on the dev server
  (use `ListSimulations` to find stale e2e simulation IDs, then
  `CleanupSimulation` each)

**Validates:** The solution works end-to-end in the real CI/CD pipeline and
dev environment.

## Audit Notes

Reviewed 2026-03-24 against `docs/client/architecture.md`,
`docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`.

- **Scope:** Server-side only (e2e test infrastructure). No client, caching, or
  i18n concerns.
- **Architecture compliance:** Option A reuses existing `CleanupSimulation` RPC
  and `simulation_id` proto fields — no new RPCs, no service-to-service
  dependencies, consistent with the server architecture doc's dependency
  injection and shared library patterns.
- **Auth note:** `AdminService` is registered without auth middleware
  (`main.go:767`), so the cleanup client does not need an auth token. The
  `devMode` check inside the RPC is the only gate. This simplifies the
  implementation — no need to pass auth headers to the AdminService client.
- **No contradictions found** with existing architecture decisions.

## Related Issues

None referenced in the issue or comments.
