# Issue #1364: [testing] time.Sleep-based synchronization in 15 test files creates flakiness risk

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1364
**Filed:** 2026-04-25 | **Last activity:** 2026-04-26
**Labels:** auto-filed, review:testing | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Fifteen test files use `time.Sleep` calls ranging from 10 ms to 1.1 s for two
distinct purposes: (1) ensuring that consecutive `clock.UnixSec(ctx)` calls
return distinct values (timestamp-ordering sleeps) and (2) waiting for
notification goroutines to finish before asserting results (async-sync sleeps).
Both patterns create test flakiness under CI load: the timestamp sleeps waste
1–3 s per test, while the async sleeps race against goroutine scheduling and
silently produce false positives when the goroutine hasn't run yet.

Research found that many of the flagged sleeps fall into four buckets:

- **Timestamp ordering** (1 s – 1.1 s): sleeping to advance wall-clock time so
  that two calls to `clock.UnixSec(ctx)` in the same second produce different
  values. Fixable by injecting a simulated time via `clock.WithSimulationTime`.
- **Sync broadcast** (50 ms): waiting after `broadcastMessage` /
  `BroadcastCommunityEvent`, which are already synchronous send-to-buffered-
  channel loops. These sleeps are simply unnecessary.
- **Async notification goroutine** (50–100 ms): waiting for `go
  s.notifyParticipants(...)` in chat, or the notification goroutine in
  `RecordCommunityEventAndNotify`. The community service already uses a
  `notificationDone chan struct{}` for this. Chat does not.
- **Legitimate timing tests** (fanout, webfetch, experience): proving
  parallelism or simulating slow HTTP servers. These must not be changed.

## Context from Comments

A single comment (`@claude`) from the collaborator maintainer, filed
2026-04-26, requesting the fix be implemented. No additional technical
direction was provided; the issue body is the authoritative specification.

## Potential Solutions

### Option A: Fix each category with the appropriate pattern (recommended)

- **Approach:**
  1. Remove sleeps after synchronous broadcast calls (no code path is async).
  2. Replace timestamp-ordering sleeps with `clock.WithSimulationTime(ctx, ...)`.
  3. Add `notifyDone chan struct{}` to the chat `Service` (mirroring the
     existing community pattern) and signal it from the notification goroutine.
     Tests that need to wait receive the signal instead of sleeping.
  4. Update `storage/feed.go:IncrementViewCount` to call `clock.UnixSec(ctx)`
     instead of `time.Now().Unix()` so feed tests can also use simulated time.
  5. Leave fanout, webfetch, experience, nudges, timeout, and simulation tests
     untouched — their sleeps are integral to what the tests prove.
- **Pros:** Each fix is minimal and idiomatic; mirrors the pattern already in
  use in the community service; zero production behaviour change; tests become
  fully deterministic.
- **Cons:** Touches ~10 test files and 3 production files; requires care not to
  alter the legitimate timing tests.
- **Effort:** Medium.

### Option B: Use `testing/synctest` (Go 1.24 experimental package)

- **Approach:** Wrap test goroutines in `synctest.Run` to make `time.Sleep`
  deterministic — the test scheduler only advances time when all goroutines
  are blocked.
- **Pros:** Minimal test-code changes; handles both timestamp and async
  patterns automatically.
- **Cons:** `testing/synctest` is experimental in Go 1.24; not stable API;
  cannot be used in standard CI without build tags; requires the entire test
  goroutine to be rewritten around the synthetic clock. Likely overkill for
  a repo already on Go 1.25.
- **Effort:** Medium-large.

### Option C: Replace goroutines with synchronous calls in tests via functional option

- **Approach:** Add a `WithSyncNotify` option to the chat `Service` that
  replaces `go s.notifyParticipants(...)` with a direct call. Tests pass this
  option; production does not.
- **Pros:** Eliminates the need for a done channel entirely.
- **Cons:** Alters production code path semantics (the production path
  intentionally runs notifications asynchronously to not block the RPC
  response); divergence between test and prod call path could hide bugs where
  async behaviour matters.
- **Effort:** Small, but carries a correctness risk.

## Recommendation

**Option A** — fix each category with the appropriate pattern.

The community service already established the correct conventions:
`clock.WithSimulationTime` for timestamp control and `notificationDone chan
struct{}` for async sync. Extending those patterns to the chat service and
feed storage is strictly additive, maintains test-prod parity for the async
path, and requires no experimental dependencies.

## Open Questions

- **Should the CI lint check be added now?** The issue recommends a grep-based
  check to prevent new `time.Sleep` in `_test.go` files. However, there are
  legitimate uses (fanout, webfetch, experience timeout tests) that would need
  to be excluded. Recommendation: add the check after this PR lands, scoped
  to an allow-list of known-OK files, as a follow-up. Doing it in this PR
  risks blocking CI if the exclusion list is incomplete.

- **Does `IncrementViewCount` also need `clock.UnixSec` for non-test callers?**
  Yes — using `clock.UnixSec(ctx)` is strictly better than `time.Now().Unix()`
  because it respects simulation contexts without any behavioural change for
  production callers (simulation time is never injected in production).

## Implementation Plan

### Phase 1: Remove unnecessary broadcast sleeps

**Goal:** Eliminate the simplest, zero-risk fixes first.
**Tasks:**
- [ ] Remove `time.Sleep(50 ms)` in `chat/streaming_test.go:100,157,211`
- [ ] Remove `time.Sleep(50–100 ms)` in `community/streaming_test.go:152,184,212,254,307,383,526`
- [ ] Remove `time.Sleep(50 ms)` in `community/notifications_test.go:69,142`
- [ ] Remove `time.Sleep(100 ms)` in `community/notifications_test.go:1096`
**Validates:** Build still passes; no test regressions.

### Phase 2: Fix timestamp-ordering sleeps with clock injection

**Goal:** Eliminate the 1–1.1 s sleeps that make tests slow.
**Tasks:**
- [ ] Replace `time.Sleep(1s)` in `chat/messages_test.go:145` with
  `clock.WithSimulationTime(ctx, time.Now().Add(2*time.Second))`
- [ ] Replace `time.Sleep(1100ms)` loop in `chat/messages_test.go:224` with
  advancing simulation time per iteration
- [ ] Replace `time.Sleep(1s)` in `community/lifecycle_test.go:252` similarly
- [ ] Update `storage/feed.go:IncrementViewCount` to use `clock.UnixSec(ctx)`
  instead of `time.Now().Unix()`
- [ ] Update `storage/feed.go:IncrementViewCountBatch` to use `clock.UnixSec(ctx)`
  instead of `time.Now().Unix()` — this function has the same pattern and must
  be updated together so both functions respect simulation time consistently
- [ ] Replace `time.Sleep(1100ms)` in `storage/feed_test.go:114` with
  `clock.WithSimulationTime`
**Validates:** Affected tests still pass and no longer spend wall time sleeping.

### Phase 3: Fix async notification sleeps with done-channel pattern

**Goal:** Eliminate the 50–100 ms races on notification goroutines.
**Tasks:**
- [ ] Add `notificationDone chan struct{}` field to `chat/service.go:Service`
  (mirrors the field name already used in the community service)
- [ ] Add `NewWithNotificationSignal` constructor to `chat/service.go`
  (mirrors the constructor name already used in the community service —
  `NewWithNotificationSignal` not `NewWithNotifySignal`)
- [ ] Update `chat/messages.go`: signal `notificationDone` after `notifyParticipants`
  goroutine completes (when field is non-nil)
- [ ] Update `chat/common_test.go:setupTestChatService` to use
  `NewWithNotificationSignal` and return the channel
- [ ] Replace `time.Sleep(100ms)` in `chat/messages_test.go:551,631,674` with
  `<-notificationDone`
- [ ] Replace `time.Sleep(50ms)` in `chat/streaming_test.go:434,494` with
  `<-notificationDone`
**Validates:** Notification tests are deterministic; no timing-dependent
  assertions remain.

## Related Issues

- None directly referenced in the issue or comments.
