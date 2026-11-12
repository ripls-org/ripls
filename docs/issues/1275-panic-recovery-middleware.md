# Issue #1275: [error_handling] Add HTTP panic recovery middleware to server handler chain

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1275
**Filed:** 2026-04-25 | **Last activity:** 2026-04-25
**Labels:** `auto-filed`, `review:error_handling` | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The server has no panic recovery in its HTTP middleware chain. Any uncaught panic in an RPC handler, fanout goroutine, or middleware will crash the entire server process, taking all in-flight requests down with it. The issue has been open for 507+ days and is now rated P1 because new streaming-generation code has added more surfaces that can panic (nil pointer dereferences in streaming handlers, etc.).

There is a goroutine-level recovery pattern in `server/services/request/lifecycle.go:153-164`, but it protects only async goroutines in that one service — the main HTTP handler chain has no equivalent protection.

## Context from Comments

- **Triage (2026-04-24):** Elevated to P1. Rationale: nil pointer in streaming code = whole-server crash. Streaming-gen work this cycle added new panicking surfaces.
- No implementation direction disagreements; the recommendation is clear: add recovery as the outermost HTTP middleware using the existing recover+log pattern.

## Potential Solutions

### Option A: PanicRecovery HTTP middleware (recommended)
- **Approach:** Create `server/middleware/recovery.go` with a `PanicRecovery(logger)` function that wraps any `http.Handler`, defers a recover, logs the panic value and stack trace, then writes an HTTP 500 response with a Connect-compatible JSON error body (`{"code":"internal","message":"internal server error"}`). Wire it as the outermost layer in `server/main.go`.
- **Pros:** Isolates each request. Server stays up. Returns a well-formed Connect error the client can parse. Follows existing middleware pattern exactly. Minimal, focused change.
- **Cons:** None material. A panic in a goroutine spawned *inside* a handler still needs its own recovery (like the existing lifecycle.go pattern), but this is out of scope.
- **Effort:** Small.

### Option B: Use a third-party recovery middleware (e.g. gorilla/handlers)
- **Approach:** The gorilla/handlers package (already imported in main.go for CORS) has a `RecoveryHandler`. Use it instead of rolling our own.
- **Pros:** Less code to write.
- **Cons:** The gorilla recovery handler returns plain 500 HTML, not Connect JSON. Doesn't integrate with our structured logger. The issue explicitly requests using our own logger and returning `CodeInternal`. Adding a dependency for 15 lines is over-engineering.
- **Effort:** Small.

### Option C: Per-handler interceptors (Connect interceptors)
- **Approach:** Add a Connect interceptor at the handler registration level that recovers panics in RPC handlers.
- **Pros:** Operates at the Connect protocol layer; can return native Connect errors.
- **Cons:** Does not protect middleware panics (auth, logging, CORS). Requires wiring interceptors to every registered service. Much more invasive. Panics in non-RPC paths (e.g. `/health`) are unprotected.
- **Effort:** Large.

## Recommendation

**Option A.** Roll a small `PanicRecovery` middleware following the exact pattern in `server/middleware/logging.go`. Place it as the outermost wrapper so it catches panics from any layer below (auth middleware, Connect handlers, CORS, etc.). Write a Connect-compatible JSON error response so the client receives a parseable error rather than a connection drop.

## Open Questions

- **Should the response body be a proper Connect JSON error or just HTTP 500?** Recommendation: Write `Content-Type: application/json` with `{"code":"internal","message":"internal server error"}`. Connect clients will parse this. Non-Connect clients (health checkers) will get a valid JSON 500. The extra 50 bytes of body are worth the improved client experience.
- **Should we log the request ID in the panic log line?** Recommendation: Yes — the request ID is in context by the time most panics happen (RequestID middleware is inside PanicRecovery). Include it if present, same as the Logging middleware does.

## Implementation Plan

### Phase 1: Create recovery middleware
**Goal:** A tested `PanicRecovery` middleware that converts panics to logged 500 responses.
**Tasks:**
- [ ] Create `server/middleware/recovery.go` with `PanicRecovery(logger *logging.Logger) func(http.Handler) http.Handler`
- [ ] Use `defer recover()`, log panic value + `debug.Stack()` at Error level with `request_id` if present
- [ ] Write `Content-Type: application/json` + HTTP 500 + `{"code":"internal","message":"internal server error"}`
- [ ] Create `server/middleware/recovery_test.go` with tests for: panic is caught, server stays up, 500 returned, no-panic path is transparent, request ID is logged
**Validates:** Middleware correctly wraps a panicking handler and returns 500 without crashing.

### Phase 2: Wire into main.go
**Goal:** PanicRecovery is live in the server handler chain.
**Tasks:**
- [ ] Wrap existing handler chain in `middleware.PanicRecovery(logger)` as outermost layer in `server/main.go`
- [ ] Update the chain comment to reflect new order: `panic recovery → request ID → logging → ...`
**Validates:** `npm run build` passes; the server compiles and lints cleanly.

## Related Issues
- None directly referenced. The review that filed this is `docs/reviews/reports/error_handling_review.md`.

---

## Audit Notes (ripls-audit)

**Reviewed against:** `docs/server/architecture.md`, `docs/client/architecture.md`

**Status: PASS — no blocking issues.**

- Plan is server-only; client architecture docs are not applicable.
- Middleware pattern follows the established convention in `server/middleware/` (function returning `func(http.Handler) http.Handler`, same as `Logging` and `QueryStats`).
- Structured logging pattern complies with `docs/server/architecture.md` observability section: use `logging.LoggerWithContext(ctx)`, include `request_id` when present, log at `ERROR` level for server failures.
- Fail-fast rule from CLAUDE.md is intentionally not applicable here — the middleware's *purpose* is to catch panics rather than propagate them, which is the documented exception for a recovery boundary.
- `debug.Stack()` import path is `runtime/debug` — no external dependency needed.
- No changes required. Proceed to implementation.
