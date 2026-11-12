# Issue #1342: [observability] No audit logging for failed authentication attempts

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1342
**Filed:** 2026-04-25 | **Last activity:** 2026-04-26
**Labels:** auto-filed, review:observability | **Assignees:** unassigned
**Context docs:** docs/ai/reviews/reports/observability_review.md

## Problem Statement

All authentication failure paths across the login service return `CodeUnauthenticated`
with zero structured logging. A bad password, a non-existent account, an invalid OIDC
token, and an expired refresh token are all silent. This makes it impossible to:
- Detect brute-force or credential-stuffing attacks
- Investigate account compromises after the fact
- Build compliance-grade audit trails for authentication events
- Lay groundwork for future rate-limiting (which requires a count of recent failures)

Success paths log correctly (`logger.InfoContext` with `user_id` and masked email), and
refresh token *reuse* (theft signal) is already warned. But the common failure cases — bad
password, unknown email, invalid OIDC token, unknown phone number, expired refresh token —
are all invisible.

## Context from Comments

A single comment from maintainer tagging `@claude` with no additional text — this is
a direct assignment to implement the fix as described in the issue body, with no
constraints or questions added.

The source review (`docs/ai/reviews/reports/observability_review.md`) is explicit: the
refresh token reuse warning at `refresh.go:45` is the **only** security-specific log in
the entire auth service, and recommends logging failed attempts with masked email, reason,
and remote address.

## Potential Solutions

### Option A: Add WarnContext at failure branches only (no middleware)

Add `logger.WarnContext` calls at each failure return site in `authentication.go`,
`phone_auth.go`, and `refresh.go`, extracting the remote address from
`req.Header().Get("X-Forwarded-For")` directly inside each handler.

- **Approach:** Purely additive changes to 3 existing files. Each failure branch gets a
  `WarnContext` call. Remote addr comes from `req.Header()` since Connect-Go exposes HTTP
  headers via `req.Header()`. No new middleware or context key.
- **Pros:** Minimal blast radius; no middleware plumbing; all context extraction is local
  to each handler.
- **Cons:** Cannot log the TCP `RemoteAddr` (the address from the actual connection —
  `r.RemoteAddr` from `net/http.Request`), only the `X-Forwarded-For` header. IP is
  absent for direct connections without a load balancer. IP extraction logic is repeated
  across all handlers.
- **Effort:** Small.

### Option B: Add RemoteAddr middleware + context key + WarnContext at failure branches (recommended)

Add a new `server/middleware/remote_addr.go` HTTP middleware that extracts the real client
IP (`X-Forwarded-For` header with fallback to `r.RemoteAddr`), stores it in context via a
new `logging.WithRemoteAddr` / `logging.RemoteAddrFromContext` pair, then adds
`WarnContext` at each failure branch.

- **Approach:** New middleware wired into the server chain alongside `RequestID`. Handlers
  call `logging.RemoteAddrFromContext(ctx)` — no headers API, no scattered extraction
  logic. Follows the existing pattern for `request_id` and `user_id`.
- **Pros:** Consistent with existing middleware chain pattern. Provides TCP `RemoteAddr`
  as a fallback. Remote addr is available to all future handlers, not just auth. Sets up
  the prerequisite the review mentions: "Add a RemoteAddr middleware alongside RequestID
  to pull X-Forwarded-For into context. Prerequisite for future rate-limit logic."
- **Cons:** Two more files to write/modify (`middleware/remote_addr.go`, wire into
  `main.go`), two new exported symbols in the logging package.
- **Effort:** Small-medium.

### Option C: Use connect.Peer from context

Connect-Go sets peer information (including the remote address) on the context via
`connect.PeerFromContext(ctx)`, which returns a `connect.Peer` with an `Addr` field.

- **Approach:** Handlers call `connect.PeerFromContext(ctx).Addr` to get the remote
  address without any new middleware or context key.
- **Pros:** No new middleware; uses the Connect-Go API directly.
- **Cons:** `connect.Peer.Addr` reflects the direct TCP connection address and does NOT
  follow `X-Forwarded-For`, making it unreliable behind a load balancer. This is the same
  limitation as Option A but dressed as an advantage. Also couples the log format to a
  Connect-specific type rather than the general logging context pattern.
- **Effort:** Small.

## Recommendation

**Option B.** The middleware pattern is already established for `request_id` and `user_id`;
adding `remote_addr` is a natural extension. The `X-Forwarded-For` fallback to `RemoteAddr`
is the correct approach for Cloud Run (behind a load balancer). The review explicitly names
this middleware as a prerequisite for future rate-limit logic, so implementing it now is
the right call even if it's slightly more work than Option A.

## Open Questions

- **Which failure branches need WarnContext?**
  Recommendation: All `CodeUnauthenticated` returns in `authentication.go`,
  `phone_auth.go`, and `refresh.go`. Specifically:
  - `EmailLogin`: user not found; wrong password
  - `OIDCLogin`: token validation failure; user not found
  - `PhoneLogin`: user not found (the phone token validation failure already goes to
    `CodeUnauthenticated` and should also be logged)
  - `RefreshToken`: token not found; token expired (the revoked-token reuse case already
    logs a `WarnContext`)

- **What fields to include in the warn log?**
  Recommendation: `"user_email"` (masked, when available), `"reason"` (a short string
  like `"unknown_email"`, `"wrong_password"`, `"token_validation_failed"`, etc.),
  `"remote_addr"`. For phone auth, phone number should NOT be logged at all (PII);
  log only `"reason"` and `"remote_addr"`.

- **X-Forwarded-For: first IP or last?**
  Recommendation: Use the first (leftmost) value from `X-Forwarded-For`, which is the
  original client IP set by the first proxy in the chain. This is the standard approach.

## Implementation Plan

### Phase 1: Remote addr context infrastructure
**Goal:** Get client IP into context so all downstream handlers can log it.
**Tasks:**
- [ ] Add `remoteAddrKey` context key, `WithRemoteAddr`, and `RemoteAddrFromContext` to
  `server/logging/logger.go`
- [ ] Create `server/middleware/remote_addr.go` with a `RemoteAddr` HTTP middleware that
  reads `X-Forwarded-For` (first IP) and falls back to `r.RemoteAddr`, storing the result
  via `logging.WithRemoteAddr`
- [ ] Wire `middleware.RemoteAddr` into the handler chain in `server/main.go` alongside
  `middleware.RequestID`
- [ ] Add unit tests in `server/middleware/remote_addr_test.go`
**Validates:** `logging.RemoteAddrFromContext(ctx)` returns the client IP in any handler.

### Phase 2: Audit warn logs at all failure branches
**Goal:** Every authentication failure emits a structured warn log.
**Tasks:**
- [ ] `authentication.go` `EmailLogin`: add `WarnContext` at "user not found" and "wrong
  password" branches with `user_email` (masked), `reason`, `remote_addr`
- [ ] `authentication.go` `OIDCLogin`: add `WarnContext` at "token validation failed" and
  "user not found" branches
- [ ] `phone_auth.go` `PhoneLogin`: add `WarnContext` at "phone token verification
  failed" and "user not found" branches (no phone number — `reason` + `remote_addr` only)
- [ ] `refresh.go` `RefreshToken`: add `WarnContext` at "token not found" and "token
  expired" branches
**Validates:** `go test ./server/services/login/...` passes; test output shows no
regressions; new tests verify warn log fields.

### Phase 3: Tests verifying audit log output
**Goal:** Prevent future regressions where failure branches lose their logs.
**Tasks:**
- [ ] Add test cases in `authentication_test.go`, `phone_auth_test.go`, and
  `refresh_test.go` that capture log output (using `logging.NewLogger` with a
  `bytes.Buffer` output and injecting via `logging.WithLogger`) and assert the warn
  message and key fields are present after a failed auth attempt
**Validates:** CI enforces audit logging; a future change that removes a warn log will
break a test.

## Related Issues

- This is sourced from the observability review at `docs/ai/reviews/reports/observability_review.md`
- The review's finding #4 ("Limited Audit Logging") is the direct source.
- Future rate-limiting work will depend on the `remote_addr` context key added here.

---

## Audit Notes

**Audited against:** `docs/server/architecture.md`, `docs/client/architecture.md`,
`docs/client/caching.md`, `docs/client/i18n.md` and the live server source tree.

### Server Architecture Compliance

**Middleware pattern (Option B) — COMPLIANT.**
The existing middleware chain in `server/main.go` wires middleware as
`PanicRecovery → RequestID → Logging → QueryStats → CORS → inner`. The plan's
`RemoteAddr` middleware belongs between `RequestID` and `Logging`, consistent with
the existing chain ordering. The `RequestID` middleware is the canonical model for
how a context-key pair (`WithRequestID` / `RequestIDFromContext`) should be added to
`server/logging/logger.go`; the plan correctly mirrors that pattern for `WithRemoteAddr`
/ `RemoteAddrFromContext`. No architectural deviation.

**Shared-library placement — COMPLIANT.**
Adding context helpers to `server/logging/logger.go` is the right home; the package
already owns all other context keys (`requestIDKey`, `userIDKey`, `operationKey`).
Adding a new `remoteAddrKey` alongside them is consistent and does not violate the
single-responsibility principle.

**Service independence — COMPLIANT.**
No new inter-service dependency is introduced. The middleware and logging package are
shared infrastructure, not services.

**File-size guidelines — COMPLIANT.**
`server/logging/logger.go` is currently 458 lines and will grow by fewer than 20 lines.
`server/middleware/request_id.go` is 47 lines; `remote_addr.go` will be similarly
small. Both remain well under the 1000-line maximum.

**Structured logging patterns — COMPLIANT.**
The plan calls for `logger.WarnContext` with `user_email` (masked via
`logging.MaskEmail` / `logging.RedactedEmail`), `reason`, and `remote_addr`. These
field names conform to CLAUDE.md conventions (`user_email` masked, no raw PII). The
plan explicitly prohibits logging phone numbers for `PhoneLogin`, which is correct.

**No client code changes — CONFIRMED.**
The plan is correctly scoped to `server/` only. No Flutter, Dart, proto API, or
client-side changes are involved.

### Issues Found and Corrected

**1. `PhoneLogin` "user not found" returns `CodeNotFound`, not `CodeUnauthenticated`.**
The problem statement in the plan and Phase 2 task list say to log at "`CodeUnauthenticated`
returns" in `phone_auth.go`. However, the actual code at
`server/services/login/phone_auth.go:179` returns `connect.CodeNotFound` for the
"no account found with this phone number" case, not `CodeUnauthenticated` (which is
reserved for the Firebase token validation failure at line 168). The audit log should
cover both the token validation failure (line 168, `CodeUnauthenticated`) and the
user-not-found failure (line 179, `CodeNotFound`). The Phase 2 task for `phone_auth.go`
is accurate in listing both branches but the problem statement's framing of "all
`CodeUnauthenticated` returns" is imprecise and would cause an implementer to miss the
`CodeNotFound` branch. **Action: implementers must log at both failure sites in
`PhoneLogin`, regardless of error code.**

**2. `LoggerWithContext` does not propagate `remote_addr` automatically.**
`logging.LoggerWithContext(ctx)` enriches the logger with `request_id`, `user_id`,
and `operation` from context — but not `remote_addr`. The plan correctly accounts for
this by having handlers call `logging.RemoteAddrFromContext(ctx)` explicitly as a field
on the `WarnContext` call. This is the right approach: `remote_addr` is a security
audit field that only needs to appear on failure logs, not on every log line, so it
should NOT be added to `LoggerWithContext`. No change needed to the plan, but
implementers should not attempt to add `remote_addr` to `LoggerWithContext` as an
ambient field.

**3. `Logging` middleware already logs `r.RemoteAddr` at the HTTP layer.**
`server/middleware/logging.go:44` already records `"remote_addr", r.RemoteAddr` on
every HTTP request log line. This is the TCP `RemoteAddr`, not the real client IP
behind a load balancer. The plan's `RemoteAddr` middleware correctly adds a
separate, load-balancer-aware `remote_addr` to context (using `X-Forwarded-For` with
fallback), which is a different and complementary piece of data. There is no
redundancy concern: the HTTP access log field and the per-failure audit log field
serve different purposes.

**4. Test injection pattern — needs clarification.**
Phase 3 proposes injecting a logger via `logging.WithLogger` to capture output in
a `bytes.Buffer`. The `logging.WithLogger` / `logging.FromContext` pair exists and
is the correct mechanism (confirmed in `server/logging/logger.go:179-191`). The
`setupTestService` helper in `common_test.go` does not currently inject a logger into
the context, so Phase 3 tests will need to create a context with `logging.WithLogger`
before calling the handler. The existing `request_id_test.go` pattern (using
`httptest.NewRequest` + checking captured values) is the model to follow for the
middleware test in `remote_addr_test.go`. No structural issue, but the implementation
note about `logging.NewLogger` with a `bytes.Buffer` output needs to be paired with
`logging.WithLogger(ctx, logger)` so that `LoggerWithContext(ctx)` inside the handler
picks up the test logger — simply creating a logger with a buffer is not sufficient
if it is not stored in the context.

### Confirmed Compliant

- Option B recommendation (middleware + context key) correctly follows the
  established `RequestID` pattern.
- `X-Forwarded-For` first-IP extraction is the correct approach for Cloud Run.
- Masking phone numbers entirely (log only `reason` + `remote_addr`) is correct PII
  handling per CLAUDE.md and `logging/redact.go` conventions.
- Three-phase plan structure (infrastructure → handler changes → regression tests)
  is the right order and matches server architecture testing guidelines.
- No proto changes, no new service dependencies, no client changes required.
