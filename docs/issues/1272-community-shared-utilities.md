# Issue #1272: [architecture] Extract community shared utilities to server/community/ library

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1272
**Filed:** 2026-04-25 | **Last activity:** 2026-04-25
**Labels:** auto-filed, review:architecture | **Assignees:** unassigned
**Context docs:** docs/server/architecture.md

## Problem Statement

Three symbols defined in the `server/services/community` RPC service package —
`RecordCommunityEventAndNotify`, `GetNumCommunityMembers`, and
`MaxCommunityMembers` — are imported by 11 files across five unrelated services
(`experience`, `transfer`, `request`, `login`, `web`). This violates the
service-independence principle in `docs/server/architecture.md`: services must
depend on shared libraries, not on other services.

The issue has been open for over 143 days and has grown from 7 import sites to
12 (including `main.go`). Each new feature that needs community capacity logic
or event recording quietly deepens the coupling.

`server/community/` already exists as a proper library package (it holds
`regions.go`). The three shared symbols belong there; the RPC service package
should depend on the library, not the other way around.

## Context from Comments

- P3 triage (2026-04-24): low user-facing urgency, but the pattern compounds —
  7 → 11 import sites in 45 days.
- Owner explicitly asked for the fix (`@claude do it`).

## Potential Solutions

### Option A: Move symbols to `server/community/` library (recommended)

- **Approach:** Create three new files in `server/community/` —
  `events.go`, `membership.go`, `constants.go` — containing
  `RecordCommunityEventAndNotify` + its private helpers + global broadcaster
  vars, `GetNumCommunityMembers`, and `MaxCommunityMembers` respectively.
  Remove them from `server/services/community/`. Update all 11 external
  importers to use the new path (import alias unchanged: `community.`). Update
  the service-internal callers to use a `communitylib` alias. Add a Go test
  under `server/services/` that enforces no cross-service imports as a CI
  gate.
- **Pros:** Eliminates the violation entirely. Import path for external services
  is a trivially mechanical change. The library's package name (`community`)
  stays the same so call sites need only the import line changed. Enforced by
  CI going forward.
- **Cons:** The service package itself must import the library (acceptable:
  library → service direction is correct). Tests in `server/services/community/`
  that call the moved functions directly need import-alias updates.
- **Effort:** Medium (20–25 files, but changes are mechanical).

### Option B: Keep symbols in service package; gate access with a thin interface

- **Approach:** Define an interface (e.g., `EventRecorder`) in a shared package
  that the community service implements, then inject it into the other services.
- **Pros:** Avoids moving the notification logic out of the service package.
- **Cons:** Adds runtime dependency injection overhead and machinery for what is
  really a pure data-processing function with no service lifecycle. The issue
  explicitly recommends a library package, not an interface layer. Does not fix
  the structural coupling; it just hides it.
- **Effort:** Medium–large (new interface, new injection points in five services,
  new constructors).

### Option C: Inline copies of each helper per service

- **Approach:** Copy `RecordCommunityEventAndNotify`, `GetNumCommunityMembers`,
  and `MaxCommunityMembers` into each consuming service's package.
- **Pros:** No new shared dependency.
- **Cons:** Code duplication. Violates DRY. Fixes the import violation but
  creates a maintenance burden. Not recommended.
- **Effort:** Medium (copies plus tests per service).

## Recommendation

**Option A.** It is the approach explicitly named in the issue, it follows the
architecture doc's "standalone library" pattern, and the per-file change is
mechanical (find-replace import path). The only non-trivial work is:

1. Moving `RecordCommunityEventAndNotify` along with its private helpers and
   the two package-level broadcaster vars (`eventBroadcaster`, `streamChecker`)
   and their setter functions.
2. Updating the community service-internal callers to use an import alias so the
   `community` short name does not collide with the `package community`
   declaration.
3. Adding the cross-service import guard as a CI test.

## Open Questions

- **Q: Should the package-level `eventBroadcaster`/`streamChecker` globals move
  to the library as well?**
  Recommendation: Yes. They are semantically coupled to
  `RecordCommunityEventAndNotify`. Moving them is what allows the community
  service to register its broadcast callbacks without the library having a
  circular dependency back to the service.

- **Q: Does `notifications_test.go` in the service package need to move?**
  Recommendation: No. Those tests exercise the interaction between the service's
  RPC handlers and the notification flow; they use service-specific helpers
  (`setupTestService`, `createAuthenticatedContext`). They should stay in the
  service package but call `communitylib.RecordCommunityEventAndNotify` with an
  import alias.

- **Q: Where should the service isolation CI check live?**
  Recommendation: `server/services/service_isolation_test.go` as a standard Go
  test (no build tags). Uses `go/parser` to walk imports and fails if any file
  under `server/services/<A>/` imports `server/services/<B>/` (A ≠ B).

## Implementation Plan

### Phase 1: Create the library files in `server/community/`

**Goal:** The three shared symbols exist in the library package with tests.

**Tasks:**
- [ ] Create `server/community/events.go` — move `RecordCommunityEventAndNotify`,
  `SetEventBroadcaster`, `SetStreamChecker`, the two global vars, and all
  private helpers from `server/services/community/notifications.go`.
- [ ] Create `server/community/membership.go` — move `GetNumCommunityMembers`
  from `server/services/community/membership.go`.
- [ ] Create `server/community/constants.go` — move `MaxCommunityMembers` from
  `server/services/community/constants.go`.
- [ ] Create `server/community/membership_test.go` — unit tests for
  `GetNumCommunityMembers` using `storage.SetupTestStorage`.
- [ ] Create `server/community/events_test.go` — minimal unit test for
  `RecordCommunityEventAndNotify` (happy path + no-broadcaster guard), required
  because `docs/server/architecture.md` mandates unit tests for every shared
  library function. The service-package streaming/notification tests cover
  integration paths; this test covers the library's contract in isolation.

**Validates:** `go build ./server/community/...` passes.

### Phase 2: Update `server/services/community/` internals

**Goal:** The service package no longer defines the shared symbols; it imports
them from the library.

**Tasks:**
- [ ] Delete `server/services/community/constants.go` (empty after move).
- [ ] Remove `GetNumCommunityMembers` from `server/services/community/membership.go`.
- [ ] Remove all moved code from `server/services/community/notifications.go`
  (file becomes empty; can be deleted).
- [ ] Add `communitylib "type2fun.org/ripls/server/community"` import to all
  files in `server/services/community/` that call the moved symbols:
  `service.go`, `invitations.go`, `lifecycle.go`, `membership.go`,
  `gear_sharing.go`, and the test files
  (`streaming_test.go`, `membership_test.go`, `invitations_test.go`,
  `lifecycle_test.go`, `notifications_test.go`).
- [ ] Update each call site to use `communitylib.` prefix.
- [ ] Fix `streaming_test.go` line that reads `eventBroadcaster` directly —
  replace with `communitylib.SetEventBroadcaster(nil)` + defer to reset.

**Validates:** `go build ./server/services/community/...` passes.

### Phase 3: Update 11 external service importers

**Goal:** All cross-service imports of `server/services/community` are gone.

**Tasks:**
- [ ] In each of the 11 files, change the import path from
  `"type2fun.org/ripls/server/services/community"` to
  `"type2fun.org/ripls/server/community"`.
  The package qualifier used at call sites (`community.`) remains the same.
  Files: `experience/{events,share,lifecycle,undo}.go`,
  `login/registration.go`, `request/{events,undo}.go`,
  `transfer/{lifecycle,undo,interest}.go`, `web/service.go`.

**Validates:** `go build ./server/services/...` passes.

### Phase 4: Add cross-service import guard

**Goal:** Future regressions are caught automatically.

**Tasks:**
- [ ] Create `server/services/service_isolation_test.go` — Go test using
  `go/parser` to walk all `.go` files under `server/services/`, parse imports,
  and `t.Errorf` for any file in `server/services/<A>/` that imports
  `server/services/<B>/` (where `<A>` ≠ `<B>`).

**Validates:** `go test ./server/services/ -run TestServiceIsolation` passes
and would fail if someone re-introduced the violation.

### Phase 5: Final build and test pass

**Tasks:**
- [ ] `npm run build` — lint + `go build ./...`
- [ ] `go test -tags=integration -timeout 20m ./...`

## Related Issues

- None referenced directly. Flagged by `/ripls-reviews` architecture review.
