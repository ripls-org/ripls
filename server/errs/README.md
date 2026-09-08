# errs

Shared infrastructure-error classification.

## Purpose

Two responsibilities live here:

1. **Transient-error predicate.** `IsTransient(err)` reports whether
   `err` looks like a transient infrastructure failure
   (HTTP 429 / rate limit / quota, gRPC UNAVAILABLE /
   RESOURCE_EXHAUSTED / DEADLINE_EXCEEDED, transport blips like
   connection-reset / broken-pipe / i/o-timeout, upstream 5xx, DNS
   lookup failures). It's deliberately string-based so the same
   predicate works across PostgreSQL drivers, gRPC, and HTTP
   clients for AI providers — all three lower their wire failures
   into Go errors with the same human-readable substrings.
2. **Transient-streak escalation for background jobs.**
   `TransientStreak` + `LogJobError` together implement the policy
   "a single transient blip on a 1-minute ticker is not worth
   paging; a 5-minute outage is." See
   `docs/server/observability.md` for the operator-facing contract.

## Key files

- `transient.go` — `IsTransient(err) bool`.
- `streak.go` — `TransientStreak` + `LogJobError`, plus the
  `TransientPageAfter = 5 * time.Minute` constant that is the
  SLO knob.

## When to add code here

- A new transient substring shows up in production logs and ought
  to suppress a page (add to `transient.go` with a test case).
- A new error-classification policy is shared by both
  `server/services/*` RPC handlers and background jobs (add a new
  helper alongside `LogJobError`).

## When NOT to add code here

- Service-specific error wrapping → stays in the service package.
- Connect-Go internal-error wrapping → use `server/connecterr`.
- Storage-layer error sentinels like `ErrRecordNotFound` → stays
  in `server/storage`.

## Consumers

- `server/main.go` — every long-running background-job goroutine
  uses `LogJobError`.
- `server/ai/errors.go` — `ai.IsTransientError` forwards to
  `errs.IsTransient` (kept as a thin shim so AI call sites don't
  need to rename).
- `server/ai/aitest/quota.go` — `aitest.IsTransientError`
  similarly forwards.
