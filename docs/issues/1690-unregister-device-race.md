# Issue #1690: Race in UnregisterDevice: Delete fails with 'no record found' when concurrent NotifyUser cleans up same invalid token

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1690
**Filed:** 2026-05-04 | **Last activity:** 2026-05-04
**Labels:** bug | **Assignees:** unassigned
**Context docs:** none

> **Audit (2026-05-04):** Reviewed against `docs/server/architecture.md` and
> `docs/server/observability.md`. Plan is compliant — fix lives in the shared
> `server/storage/` library (no service-to-service coupling), uses unit-test-first
> strategy at the most local level, and removes a false ERROR per the
> observability doctrine ("ERROR is reserved for system failures... user errors
> like not found are INFO or WARN"). Phase 2 task list updated to use the
> existing `setupTestService(t)` harness in `server/notifications/service_test.go`
> rather than introducing new test scaffolding. Phase 3 references the
> "Server Error Logged" alert policy (rate-limited 1/5min) named in
> observability.md §Part 4.

## Problem Statement

When two `NotifyUser` calls fire for the same user within milliseconds (e.g.
back-to-back community event + chat message), each independently fetches the
user's devices, each gets `ErrInvalidToken` from FCM for the same dead token,
and each calls `UnregisterDevice(token)` to clean it up. Both calls pass the
`getDeviceByToken` lookup successfully — but only one wins the `DELETE`. The
loser receives `RowsAffected == 0` from `storage.Delete`, which surfaces as
`"failed to delete device: no record found with id: ..."` and triggers the
`Server Error Logged - server` Cloud Run alert.

The cleanup goal (dead device gone) is already achieved by the winning call;
the ERROR is purely false-alarm noise that pages oncall.

Confirmed by prod logs on revision `server-00087-2rg`, same Cloud Run instance,
within ~12ms — `successfully removed invalid device` (INFO) at 14:56:30.192397
and `failed to delete device: no record found` (ERROR) at 14:56:30.193057, for
the same `device_id`.

The existing comment in `UnregisterDevice` ("Not found is OK (idempotent)")
shows the intent was already to make this operation idempotent — but it only
covers the lookup half of the race, not the delete half.

## Context from Comments

- The author (maintainer) added a follow-up comment with prod log evidence
  confirming the race: two `NotifyUser` calls on the same instance, with
  different `total_devices` snapshots (5 and 7), both saw the same invalid
  token, one logged `successfully removed invalid device` (INFO) and the other
  logged `failed to delete device: no record found` (ERROR) ~660µs later.
- Issue body proposes two fix options. **Option A** (preferred): make
  `storage.Delete` return the `storage.ErrRecordNotFound` sentinel (already
  used by `Get`), and have `UnregisterDevice` treat that as success.
  **Option B**: string-match the error message in `UnregisterDevice`. Author
  recommends A — same shape as #1254, fixes a pre-existing sentinel-error
  inconsistency.
- Cross-references #1254 (FCM `NotRegistered` errors not triggering device
  cleanup → noisy ERROR alerts) as the same category of false-ERROR fix.

## Codebase findings

- `server/storage/errors.go:6` — `ErrRecordNotFound` sentinel exists already.
- `server/storage/protosql.go:118` — `Get` already wraps the sentinel
  correctly: `fmt.Errorf("record not found with id %s: %w", id, ErrRecordNotFound)`.
- `server/storage/protosql.go:397` (`Update`) and `:455` (`Delete`), and
  `server/storage/embedding.go:200` (`UpdateEmbedding`) — all return a bare
  `fmt.Errorf("no record found with id: %s", id)` **without** wrapping the
  sentinel. This is the inconsistency to fix.
- `server/notifications/service.go:202` — the buggy lookup-then-delete site.
- Existing sentinel users: `server/auth/community_active.go:104,135`,
  `server/services/gear/people.go:51`, `server/storage/cascade_delete.go:14`,
  `server/storage/community.go:46`. None of them inspect error strings, so
  changing the error message format is safe.
- `server/storage/errors_test.go` already verifies the `errors.Is` wrap
  behavior — pattern to mirror in new tests.

## Potential Solutions

### Option A: Sentinel-error fix (recommended)

- **Approach:** Wrap `ErrRecordNotFound` in the three `RowsAffected == 0`
  sites (`Update`, `Delete`, `UpdateEmbedding`). In `UnregisterDevice`, check
  `errors.Is(err, storage.ErrRecordNotFound)` and return `nil` (idempotent
  success). Add a unit test for the race using the storage layer directly.
- **Pros:** Type-safe, consistent with the existing `Get` pattern, fixes the
  same inconsistency in `Update` and `UpdateEmbedding`. Mirrors the #1254 fix
  shape. No string matching across packages.
- **Cons:** Touches three storage call sites, not just the buggy one. Slightly
  larger blast radius — but every change is purely additive (wrapping a
  sentinel into an existing error message) and existing callers that don't
  use `errors.Is` keep working unchanged.
- **Effort:** Small (~30 lines + tests).

### Option B: String-match in `UnregisterDevice`

- **Approach:** In `UnregisterDevice`, check
  `strings.Contains(err.Error(), "no record found")` and return `nil`.
- **Pros:** Minimal diff. Only touches the one buggy function.
- **Cons:** String matching across a package boundary is brittle — any
  rephrase of the storage error breaks the cleanup path silently and the
  ERROR returns. Doesn't fix the underlying sentinel inconsistency. Doesn't
  protect future similar sites.
- **Effort:** Small (~5 lines).

### Option C: Use `DeleteByField` and tolerate zero rows

- **Approach:** Replace the lookup-then-delete with a single
  `DeleteByField(ctx, "user_device", "device_token", token)` call (already
  exists in storage). Treat `rowsDeleted == 0` as idempotent success.
- **Pros:** Eliminates the lookup-then-delete pattern entirely — one SQL
  round-trip instead of two, and the race window collapses to a single
  atomic statement.
- **Cons:** `DeleteByField` returns `(int64, error)` rather than the per-row
  not-found semantics, so callers everywhere else in the codebase that use
  the lookup-then-delete pattern still have the race. Doesn't generalize.
  Also worth checking whether `DeleteByField` triggers any per-row hooks
  (cascades, embedding cleanup) that single `Delete` does — might not be
  drop-in equivalent.
- **Effort:** Small (~10 lines), but harder to verify safety.

## Recommendation

**Option A**. It's type-safe, consistent with the established `Get` pattern,
fixes a pre-existing inconsistency in two other storage methods, and matches
the shape of the #1254 fix that closed the same class of false-ERROR alert.
Option B is brittle; Option C is narrower but doesn't fix the root cause and
introduces equivalence questions about hooks. Option A's incremental cost
over B is small — three one-line wraps and one extra unit test — and it
buys idempotent semantics for every future caller of `Update`, `Delete`, and
`UpdateEmbedding`.

## Open Questions

- **Should `Update` and `UpdateEmbedding` also wrap the sentinel, or just
  `Delete`?** — Recommendation: wrap all three. They share the same bug
  shape (zero-rows-affected → bare `fmt.Errorf`), and a future caller of
  `Update` will hit the same race in the same way. Wrapping them now costs
  nothing and prevents the next instance of this issue. The `Get` site at
  protosql.go:118 already sets the precedent.
- **Should the `UnregisterDevice` not-found branch log at INFO/DEBUG to
  retain observability of the race, or stay silent?** — Recommendation:
  silent. The winning call already logs `successfully removed invalid
  device` at INFO; adding a "race detected" log on the loser would just
  create alert-rule churn for callers who don't care. If we want race
  visibility for diagnostic purposes later, a metric beats a log.
- **Are there any other `errors.Is(err, ...)` checks downstream that depend
  on the current bare-`fmt.Errorf` shape?** — Recommendation: no, codebase
  scan shows zero string-matchers on the "no record found" message; the
  only existing inspectors of storage errors use `errors.Is(err,
  ErrRecordNotFound)` and will keep working (and now also work for
  `Update`/`Delete`/`UpdateEmbedding`). No migration needed.
- **Add a regression test that simulates two concurrent `NotifyUser`
  calls?** — Recommendation: yes, but at the storage layer (two goroutines
  doing `getDeviceByToken` then `Delete` on the same row). A full
  notification-service-level test would be heavier and introduces FCM
  mocking; the storage-layer race test is the minimal repro and exercises
  the actual fix point.

## Implementation Plan

### Phase 1: Wrap `ErrRecordNotFound` in storage mutations

**Goal:** `Update`, `Delete`, and `UpdateEmbedding` return a sentinel-wrapped
error on zero rows affected, matching the existing `Get` pattern.

**Tasks:**
- [ ] `server/storage/protosql.go:397` — change `Delete`'s zero-rows-affected
      branch to `fmt.Errorf("record not found with id %s: %w", id, ErrRecordNotFound)`
- [ ] `server/storage/protosql.go:455` — same change for `Update`
- [ ] `server/storage/embedding.go:200` — same change for `UpdateEmbedding`
- [ ] Add unit tests in `server/storage/protosql_test.go` verifying
      `errors.Is(err, ErrRecordNotFound)` for each of the three methods
      when the target row doesn't exist
- [ ] Run `npm run build` and `go test ./server/storage/...` to confirm

**Validates:** Sentinel propagates correctly; no existing tests break.

### Phase 2: Make `UnregisterDevice` idempotent on Delete

**Goal:** The loser of a concurrent-cleanup race returns nil instead of
logging ERROR.

**Tasks:**
- [ ] `server/notifications/service.go:209` — wrap the `s.storage.Delete`
      error check: if `errors.Is(err, storage.ErrRecordNotFound)`, return
      `nil`; otherwise return the wrapped error as today
- [ ] Add a unit test in `server/notifications/service_test.go` using the
      existing `setupTestService(t)` harness. Insert one device, then call
      `UnregisterDevice` twice in parallel goroutines for the same token;
      assert both return nil and the device row is gone. (Existing
      `TestUnregisterDevice_NotFound` only covers the lookup-not-found
      branch; this adds delete-not-found / race coverage.)
- [ ] Run `npm run build` and `go test ./server/notifications/...`

**Validates:** Race-loser path returns nil; the false-ERROR no longer fires.

### Phase 3: Verify in prod

**Goal:** Confirm the alert stops firing for this signature.

**Tasks:**
- [ ] Merge and deploy
- [ ] Monitor the `Server Error Logged - server` alert policy
      (`docs/server/observability.md` §Part 4 — rate-limited to 1
      notification per 5 min) for the "failed to delete device: no record
      found" signature over the next ~7 days. Previously fired
      2026-05-03 14:56 UTC; query Cloud Logging back ~30 days to establish
      baseline frequency before/after.
- [ ] If the signature recurs after deploy, re-investigate (likely a
      different code path emitting the same string, or a non-notification
      caller of `storage.Delete` racing on a different table)

**Validates:** End-to-end fix confirmed against the original alert.

## Related Issues

- #1254 — FCM `NotRegistered` errors not triggering device cleanup; same
  category of false-ERROR alert in the same notification cleanup path.
  Closed via similar idempotency fix.
- #1165 — FCM 'Request contains an invalid argument' not classified as
  invalid-token; broken devices never get unregistered. Earlier fix in
  the same area (token classification, not race-on-delete).
