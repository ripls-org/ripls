# Issue #1276: [error_handling] Add panic recovery to fire-and-forget goroutines in services

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1276
**Filed:** 2026-04-25 | **Last activity:** 2026-04-25
**Labels:** auto-filed, review:error_handling | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Multiple goroutines launched across `server/services/` lack `recover()`. Any
panic — from a nil pointer dereference, an unexpected third-party return value,
or a race — crashes the entire server process with no useful stack trace in the
application logs. The affected goroutines fall into two categories:

1. **Fire-and-forget goroutines** that detach from the request and run
   independently (notifications, email sends, feed nudge operations, the
   streaming-sender drainer).
2. **Branch goroutines** coordinated via `sync.WaitGroup` inside streaming
   AI generation handlers.

Both categories will kill the process on panic. The fire-and-forget ones are
especially dangerous because they reference `context.Background()` /
`context.WithoutCancel()` contexts, meaning they outlive requests and can be
triggered well after the original caller has returned.

## Context from Comments

- **Triage (maintainer, 2026-04-24):** Raised to P1 reliability. "An
  unrecovered panic in any of them silently kills the process without a useful
  stack trace."
- The review that filed this issue (`docs/reviews/reports/error_handling_review.md`)
  specifically calls out that the streaming-gen work in the latest cycle expanded
  the surface area.
- No disagreements — the direction (a shared helper + migration) is consensus.

## Potential Solutions

### Option A: `logging.GoSafe(ctx, name, fn)` for all goroutines

Add one helper to the logging package that launches a goroutine with panic
recovery. Migrate every `go func()` in `server/services/` to use it.
WaitGroup goroutines use it by placing `defer wg.Done()` inside `fn`.
`streaming/sender.go` uses an inline recover because it must also signal
`s.done` on panic.

- **Approach:** One new function in `logging/`, ~10 call sites migrated.
- **Pros:** Consistent pattern across all goroutines; all panics logged to the
  same structured log field; easy to grep for missing coverage; no new packages.
- **Cons:** The `sender.go` goroutine is structurally different and needs inline
  handling; callers must remember to put `defer wg.Done()` inside `fn` (not
  outside the call).
- **Effort:** Small — the helper is ~15 lines; migration is mechanical.

### Option B: Separate `GoSafe` (fire-and-forget) + `SafeRun` (synchronized)

`logging.GoSafe(ctx, name, fn)` launches a goroutine.
`logging.SafeRun(ctx, name, fn)` wraps a function body (no goroutine), used
inside existing `go func() { defer wg.Done(); logging.SafeRun(...) }()`.

- **Approach:** Two helpers; WaitGroup goroutine structure is explicit.
- **Pros:** Clear semantic distinction; the WaitGroup pattern stays readable.
- **Cons:** Two helpers to document and test; callers choose the wrong one;
  slightly more verbose at call sites.
- **Effort:** Small-medium.

### Option C: Inline recover everywhere (no helper)

Add `defer func() { if r := recover(); r != nil { ... } }()` directly in each
goroutine, as already done manually in `server/services/request/lifecycle.go`.

- **Approach:** No new API; each site is self-contained.
- **Pros:** No new public API; each site controls its logging fields.
- **Cons:** Verbose and inconsistent; already proven by `lifecycle.go` to be
  copy-pasted with varying quality; harder to audit for completeness; no lint
  path.
- **Effort:** Small per file, medium total (inconsistency risk).

## Recommendation

**Option A.** A single `logging.GoSafe(ctx, name, fn)` helper is enough. The
WaitGroup pattern works cleanly with it by placing `defer wg.Done()` inside
`fn`. The only exception is `streaming/sender.go`, which needs inline recovery
because it must send to `s.done` on panic to prevent `Close()` from blocking
forever. That one inline site is documented with a comment explaining why
GoSafe is not used.

## Open Questions

- **Should `GoSafe` log panics as `Error` or `Warn`?**
  Recommendation: `Error`. A panic in a goroutine is always unexpected; it
  indicates a programming bug or an upstream library invariant violation.
  `Warn` would undersell severity.

- **Should `GoSafe` include a stack trace?**
  Recommendation: Yes, via `debug.Stack()`. The existing manual pattern in
  `lifecycle.go` already does this. Without a stack trace the log entry is
  nearly useless for diagnosis.

- **Should `sender.go`'s panic recovery also drain `s.ch`?**
  Recommendation: No. Draining requires waiting for `close(s.ch)` which only
  happens from `Close()` — creating a deadlock risk if Close is never called.
  Sending a panic error to `s.done` is sufficient; the streaming handler's
  defer calls `Close()`, which will then unblock.

- **Should this PR add a custom lint rule?**
  Recommendation: Defer. The issue mentions it, but adding a lint rule requires
  a custom analyser plugin and CI plumbing. The migration itself plus the
  structured helper provides the foundation. File a separate issue.

## Implementation Plan

### Phase 1: Add `logging.GoSafe` helper

**Goal:** Introduce the shared helper and unit-test it.

**Tasks:**
- [ ] Add `GoSafe(ctx context.Context, name string, fn func())` to
  `server/logging/logger.go`. The function launches a goroutine; the goroutine
  defers a recover that logs via `LoggerWithContext(ctx)` at Error level with
  fields `"goroutine"`, `"panic"`, and `"stack"`. Requires adding
  `"runtime/debug"` to the imports in `logger.go` for `debug.Stack()`.
- [ ] Add a unit test in `server/logging/logger_test.go` that verifies a
  panicking fn is recovered without crashing the test and that the log entry
  contains the goroutine name and panic value.

**Validates:** The helper works correctly in isolation.

### Phase 2: Migrate fire-and-forget goroutines

**Goal:** Replace all unprotected fire-and-forget `go func()` calls.

**Tasks:**
- [ ] `server/services/community/notifications.go:69` — wrap the notification
  goroutine with `logging.GoSafe(ctx, "notify-community-event", ...)`. Pass the
  original request `ctx` (not `context.Background()`) as GoSafe's first argument
  so that any panic recovery log includes the request's `user_id` and `request_id`.
  The goroutine body still uses `context.Background()` internally for the
  long-lived work.
- [ ] `server/services/waitlist/service.go:100` — same: pass the request `ctx`
  to GoSafe; the body uses `context.Background()` internally.
- [ ] `server/services/waitlist/service.go:119` — same pattern.
- [ ] `server/services/feed/feed_nudges.go:47` — expire-nudge goroutine
  (capture `n.Id` before the call).
- [ ] `server/services/feed/feed_nudges.go:70` — generate-nudges goroutine.
- [ ] `server/services/feed/feed_nudges.go:149` — delete-stale-terminator
  goroutine (capture `old.Id`).
- [ ] `server/services/feed/feed_nudges.go:181` — fetch-nudge-imagery goroutine.
- [ ] `server/services/streaming/sender.go:38` — add **inline** recover (not
  GoSafe) that logs via `logging.Default()` and sends a panic error to `s.done`.
  Comment explaining why GoSafe is not used here.

**Validates:** All fire-and-forget goroutines are protected; server no longer
crashes on a panic in these paths.

### Phase 3: Migrate WaitGroup branch goroutines

**Goal:** Protect the synchronized branch goroutines in streaming AI handlers.

**Tasks:**
- [ ] `server/services/fanout/fanout.go:159,175` — wrap both goroutines with
  `logging.GoSafe`. Move `defer wg.Done()` to be the first statement inside `fn`
  (i.e., inside the closure passed to GoSafe, not outside it). Go's defer rules
  guarantee `wg.Done()` runs on panic before GoSafe's outer recover fires, so
  the WaitGroup always unblocks correctly.
- [ ] `server/services/experience/gen_ai_streaming.go:195,216` — same pattern;
  place `defer branchWG.Done()`, `defer close(mapboxDone)`, and
  `defer close(mediaDone)` as the first statements inside `fn` so they always
  fire on panic, keeping the WaitGroup and downstream channel selects unblocked.
- [ ] `server/services/gear/gen_ai_streaming.go:157,185` — same pattern.
- [ ] `server/services/request/gen_streaming.go:129,148` — same pattern.

**Validates:** A panic in an AI branch goroutine logs the error and does not
crash the server. The WaitGroup still unblocks correctly because `defer wg.Done()`
runs before GoSafe's recover catches the panic.

## Related Issues

- #1273 — service init error handling (same review cycle, already fixed)
- #1277 — related error handling issue (already fixed)
