# Issue #1531: [security] No rate limiting on authentication endpoints

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1531
**Filed:** 2026-04-28 | **Last activity:** 2026-04-29
**Labels:** auto-filed, review:security | **Assignees:** unassigned
**Priority:** P2 (per triage 2026-04-28)
**Context docs:** none

## Problem Statement

The `LoginService` is registered without any rate limiting:

```go
mux.Handle(loginPath, hostMiddleware(loginHandler)) // No auth middleware for login service
```

Every authentication-adjacent RPC — `EmailLogin`, `OIDCLogin`, `PhoneLogin`, `EmailRegister`, `OIDCRegister`, `PhoneRegister`, `RefreshToken`, `RequestPasswordReset`, `ResetPassword`, `CheckResetPasswordToken`, `CheckInvitation` — is reachable at whatever rate the front door allows. Bcrypt cost 12 raises *per-attempt* CPU cost but does not bound *attempt rate*. A credential-stuffing campaign with a leaked email/password list, an OIDC token-replay flood, or a password-reset-spam campaign is unconstrained today.

Two secondary observations from the codebase walkthrough that affect the plan:

1. **`middleware.RemoteAddr` exists but is not wired into the handler chain.** `server/middleware/remote_addr.go` defines a middleware that resolves the client IP (X-Forwarded-For, fallback to `r.RemoteAddr`) and stuffs it into `context` via `logging.WithRemoteAddr`. It has unit tests and is referenced by every login handler via `logging.RemoteAddrFromContext(ctx)` in failure log lines — but `main.go`'s handler chain (`PanicRecovery → RequestID → Logging → QueryStats → CORS → inner`) never calls it. So today every "remote_addr" field in auth-failure logs is empty. Rate limiting absolutely needs this middleware live.

2. **No existing Connect interceptors.** The codebase uses HTTP middleware exclusively (`connectrpc.com/authn` is technically a Connect-shaped wrapper but is invoked as `authMiddleware.Wrap`). There is no precedent for Connect `UnaryInterceptorFunc`. This matters because the plan must choose between HTTP middleware (path-only awareness) and a Connect interceptor (typed request access for `email` extraction).

## Context from Comments

The single non-bot comment is `@claude` summoning this run; no human shaped the plan further. Substantive context lives entirely in the issue body, which:

- Identifies four files (`authentication.go`, `password_reset.go`, `registration.go`, `refresh.go`) but the actual surface is broader: `phone_auth.go` (`PhoneLogin`, `PhoneRegister`) and the lookup endpoints (`CheckInvitation`, `CheckResetPasswordToken`) are equally credential-discovery-adjacent and should be in scope.
- Specifies dual-key strategy: `(IP, endpoint)` and `(email, endpoint)`.
- Specifies library: `golang.org/x/time/rate` (already in `go.mod` as an indirect dep at v0.15.0; can be promoted to a direct dep).
- Specifies indicative budgets: 5/min/IP for `Login`, 3/hour/email for `RequestPasswordReset`. These are starting points, not gospel.

The triage comment classifies this Medium severity, P2 — durable security hardening rather than incident-response.

## Potential Solutions

### Option A: Connect interceptor + wire `RemoteAddr` middleware (recommended)

- **Approach:** Two-part change.
  1. Add `middleware.RemoteAddr` to the handler chain in `main.go` between `RequestID` and `Logging` so the rate limiter (and the existing audit-log sites) actually see the client IP.
  2. Add a new sub-package `server/middleware/ratelimit/` (single-purpose library, in line with the architecture doc's "focused libraries" principle and matching how `notifications/fcm`, `notifications/noop` are organized). It exposes:
     - `ratelimit.Rule` — `Procedure string`, `IPLimit *Budget`, `EmailLimit *Budget`.
     - `ratelimit.Budget` — `Rate rate.Limit`, `Burst int`.
     - `ratelimit.Interceptor(rules ...Rule) connect.UnaryInterceptorFunc` — returns the actual interceptor.
     - Non-stuttering names per CLAUDE.md (e.g. `ratelimit.Rule`, not `ratelimit.RateLimitRule`). The placement also matches the issue body's "middleware/ratelimit.go" intent — only the file becomes a directory.
  3. The interceptor:
     - Looks up `req.Spec().Procedure` against the rule set; passes through if no rule matches.
     - Resolves IP via `logging.RemoteAddrFromContext(ctx)`. If the rule has an `IPLimit` and the IP is non-empty, calls `Allow()` on the per-(procedure, ip) limiter.
     - For rules with `EmailLimit`, type-asserts `req.Any()` to a small interface `interface{ GetEmail() string }` (proto-generated getters on messages with an `email` field satisfy this). If the email is non-empty, calls `Allow()` on the per-(procedure, email) limiter.
     - On rejection, logs at `Warn` (procedure, key_type, masked email or remote_addr) and returns `connect.NewError(connect.CodeResourceExhausted, …)`. Connect maps that to HTTP 429.
  4. Limiters are stored in two `sync.Map`s — `ipLimiters` and `emailLimiters` — keyed by `procedure + "\x00" + ip|email`. A janitor goroutine, started lazily by `sync.Once` on first interceptor construction, runs every 5 minutes and removes entries whose underlying limiter is full (idle long enough to refill — prevents unbounded memory growth from one entry per IP forever). The janitor uses a `clock` interface (defaults to real `time.Now`/`time.AfterFunc`, swappable in tests) so the janitor's eviction policy is unit-testable without sleeps.
  5. Apply only to `LoginService` by wrapping its handler: `apiconnect.NewLoginServiceHandler(loginService, connect.WithInterceptors(ratelimit.Interceptor(rules...)))`.
  6. Bypass entirely in dev mode (gated by `cfg.devMode`) so simulation runs and integration tests aren't artificially throttled.

- **Pros:** Connect interceptors get the typed request struct via `req.Any()` and the canonical procedure string via `req.Spec().Procedure`, so both `(ip, procedure)` and `(email, procedure)` keys are clean. Per-procedure limits are declarative. Localized to `LoginService` registration; no impact on other services. Fixes the latent `RemoteAddr` wiring bug as a free side-effect, restoring real client IPs to existing audit-log fields. Aligns with `golang.org/x/time/rate` token-bucket semantics requested in the issue.

- **Cons:** Introduces the first Connect interceptor in the codebase — adds a new pattern future contributors must learn. In-memory limiters mean limits reset on deploy and don't share state across server replicas (the deployment is currently single-replica per `docs/server/architecture.md` patterns, but this is worth flagging).

- **Effort:** Medium. ~1 new middleware file + tests, ~10 lines of wiring in `main.go`, ~5 lines per `LoginService` handler registration. Total ~600 LOC including thorough tests.

### Option B: HTTP middleware on the LoginService path only

- **Approach:** Same `RemoteAddr` wiring fix. Then add `middleware.RateLimit(rules ...Rule) func(http.Handler) http.Handler` that inspects `r.URL.Path` to derive the procedure (e.g. `/ripls.api.LoginService/EmailLogin`), reads the client IP from context, applies the IP-keyed bucket, and returns a Connect-compatible 429 JSON error body if exhausted. Wrap the LoginService registration: `mux.Handle(loginPath, hostMiddleware(rateLimitMiddleware(loginHandler)))`.
- **Pros:** Matches the existing middleware-chain pattern exactly — no new framework concept. Slightly less code to test.
- **Cons:** **Cannot do `(email, procedure)` keys cleanly.** The HTTP body has not been deserialized at this layer; the only way to extract the email is to read+buffer+rewind `r.Body` and JSON-decode it (or proto-decode for the binary Connect protocol — a fragile content-negotiation reimplementation of what Connect already does internally). This sacrifices the issue's explicit dual-key requirement, which exists for a reason: IP-only limits are bypassable behind shared CGNAT or by attackers rotating IPs, and email-only limits don't help against credential-stuffing where one IP probes thousands of emails. Either you ship without the email key (incomplete fix) or you reimplement protocol decoding (worse than Option A).
- **Effort:** Small for IP-only; medium-large if we try to do email keys at HTTP layer.

### Option C: Distributed rate limiter (Redis / Postgres)

- **Approach:** Persist the token-bucket state in Postgres or Redis so it survives restarts and is shared across replicas.
- **Pros:** Limits are real across deploys and any future horizontal scaling. Survives restart-based attacks (where an attacker could deliberately trigger redeploys).
- **Cons:** No Redis dependency exists in the project; adding one is a deployment-architecture change well beyond this issue's scope. Storing rate-limit counters in Postgres adds a write per request and serializes a hot table — defeats the purpose of `golang.org/x/time/rate`'s in-memory token bucket. Single-replica deployment makes the durability argument moot today. Issue priority is P2; this would balloon to a multi-week project.
- **Effort:** Large.

## Recommendation

**Option A.** It is the only option that satisfies both halves of the issue's dual-key requirement (`(IP, endpoint)` *and* `(email, endpoint)`) without reimplementing Connect's protocol decoding by hand. The "first Connect interceptor in the codebase" cost is real but small — the connect-go interceptor API is short and well-documented, and rate limiting is exactly the kind of cross-cutting RPC concern interceptors exist for. Option B is tempting because it matches existing middleware conventions, but it forces a choice between an incomplete fix (IP only) and reimplementing the wire protocol (Option B's "cons"). Option C is over-architected for the deployment.

We deliberately *also* fix the `RemoteAddr` wiring as part of this PR because it's load-bearing for the rate limiter and a small enough side-quest that doing it separately would either block this work behind an unrelated PR or ship a rate limiter that silently keys all traffic to the empty string. Restoring real client IPs to existing audit-log fields is a free improvement.

## Open Questions

- **Should `RefreshToken` be rate-limited? It's authenticated implicitly by token possession.**
  Recommendation: Yes — at a *looser* IP-only budget (e.g. 60/min/IP). A flood of invalid refresh tokens is a credential-discovery vector against the storage's `token_hash` lookup; the existing token-theft-detection logic in `refresh.go` revokes all tokens on revoked-token reuse but does not bound the *unknown-token* probe rate.

- **What budgets should ship?**
  Recommendation: Mirror the issue's suggestions and add reasonable defaults for the rest. All values are constants in `ratelimit.go` and easy to tune later.
  - `EmailLogin` / `OIDCLogin` / `PhoneLogin`: 5/min/IP, 5/hour/email (Login only — OIDC/Phone don't carry an email field at request time).
  - `EmailRegister` / `OIDCRegister` / `PhoneRegister`: 3/hour/IP (registration is rare; tight budget acceptable).
  - `RequestPasswordReset`: 5/min/IP, 3/hour/email — matches issue body.
  - `ResetPassword`: 5/min/IP (token-bearer; tight is fine).
  - `CheckResetPasswordToken` / `CheckInvitation`: 30/min/IP (used during normal flows; loose).
  - `RefreshToken`: 60/min/IP.
  Reset and Login share an IP budget bucket (separate per RPC, not shared across RPCs) so a brute-forcer can't spread attempts across endpoints to evade limits.

- **What to do with `localhost`/loopback in dev?**
  Recommendation: Skip rate limiting entirely when `cfg.devMode == true`. Production servers use `--dev-mode=false` so this is a clean toggle. Dev-mode integration tests already make rapid requests; throttling them would create flaky CI.

- **How to handle X-Forwarded-For trust?**
  Recommendation: Out of scope — the existing `middleware.RemoteAddr` reads the leftmost XFF entry. Document in the PR that production deployment must terminate TLS at a trusted proxy that overwrites or strips client-supplied XFF headers (Cloud Run does this by default). File a follow-up issue if the trust boundary needs hardening — that's a deployment-config issue, not a server-code issue.

- **Should we emit metrics / structured logs on throttle?**
  Recommendation: Yes — log a `Warn` line per throttled request with `remote_addr`, `procedure`, and `key_type` (`ip` vs `email`), masking email via `logging.MaskEmail`. No new metrics infrastructure needed today — the structured logs feed the existing pipeline. A rate-limited request should never log at `Error` (it's expected behavior, not a system failure).

## Implementation Plan

### Phase 1: Wire `RemoteAddr` middleware into the handler chain

**Goal:** Restore real client IPs to existing audit-log fields and unblock the rate limiter's IP key.
**Tasks:**
- [ ] In `server/main.go`, insert `middleware.RemoteAddr` between `middleware.RequestID` and `middleware.Logging` in the chain (the README places this responsibility "alongside RequestID").
- [ ] Update `server/middleware/README.md` to list `remote_addr.go` under "Key files".
- [ ] Run `npm run build` to confirm clean.
- [ ] Verify that an existing audit-log test exercising an `EmailLogin` failure now sees a non-empty `remote_addr` field. If no such test exists end-to-end, add a minimal one in `audit_logging_test.go` that wires the middleware via `httptest.NewServer` and asserts the field is populated.

**Validates:** The latent wiring bug is fixed in isolation, with a test that pins the chain order so future refactors don't silently un-wire it.

### Phase 2: Build the rate-limit interceptor

**Goal:** A reusable `connect.UnaryInterceptorFunc` keyed on (procedure, IP) and (procedure, email) using `golang.org/x/time/rate`.
**Tasks:**
- [ ] Promote `golang.org/x/time` from indirect to direct in `go.mod` (`go get golang.org/x/time/rate`).
- [ ] Create the sub-package `server/middleware/ratelimit/`:
  - `doc.go` — `// Package ratelimit provides a Connect interceptor ...` (1–3 sentences) so `revive.package-comments` passes.
  - `ratelimit.go`:
    - `Rule`, `Budget` types as described in Option A.
    - `Interceptor(rules ...Rule) connect.UnaryInterceptorFunc` returns the interceptor.
    - Limiters stored in two `sync.Map`s keyed by `procedure + "\x00" + ip|email`.
    - Janitor goroutine: lazily started by `sync.Once`, runs every 5 minutes, evicts limiters whose tokens have fully refilled (idle).
    - Internal `clock` interface (`Now()`, `AfterFunc(d, fn)`) — defaults to real time, swappable in tests.
    - Per-RPC log line at `Warn` on rejection: `"rate_limited"`, `procedure`, `key_type` (`"ip"` or `"email"`), `remote_addr` (or `logging.MaskEmail(email)`).
    - All exported names follow `revive.exported`: doc comments start with the symbol name; no `ratelimit.RateLimit*` stutter.
  - `ratelimit_test.go`:
    - Per-IP budget enforces `Burst` then rejects until refill (use the fake clock — no real sleeps).
    - Per-email budget independent of the IP key.
    - Combined IP+email rule: rejection on either key triggers 429 with `connect.CodeResourceExhausted`.
    - Pass-through when procedure has no rule.
    - Empty IP and empty email fall through (don't false-throttle when upstream wiring is broken — fail-open is the safer default for transport plumbing bugs since the audit log still records the underlying failure cause).
    - Janitor evicts idle limiters when the fake clock advances past the eviction threshold.
    - Concurrent requests don't double-count or race (run with `-race`).
    - Verify the `Warn` log line shape using a `bytes.Buffer`-backed JSON logger, mirroring `audit_logging_test.go`.
- [ ] `npm run build` and `go test -race -timeout 5m ./server/middleware/ratelimit/...`.

**Validates:** The interceptor behaves correctly per-RPC under load, in isolation from the LoginService.

### Phase 3: Wire the interceptor into LoginService

**Goal:** Apply the rate limiter to authentication RPCs in production mode.
**Tasks:**
- [ ] Expose budget defaults from `ratelimit` itself: add `ratelimit.DefaultLoginRules()` returning the budget table from "Open Questions" above. Keeping the defaults inside the package (rather than in `main.go`) makes them auditable in one place and reusable by future callers (e.g. an admin RPC throttle).
- [ ] Update LoginService registration in `server/main.go`:
  ```go
  var loginOpts []connect.HandlerOption
  if !cfg.devMode {
      loginOpts = append(loginOpts,
          connect.WithInterceptors(ratelimit.Interceptor(ratelimit.DefaultLoginRules()...)))
  }
  loginPath, loginHandler := apiconnect.NewLoginServiceHandler(loginService, loginOpts...)
  mux.Handle(loginPath, hostMiddleware(loginHandler))
  ```
- [ ] Add a smoke test in `server/services/login/` that exercises rate limiting end-to-end via `httptest.NewServer` with the full handler chain (`storage.SetupTestStorage(t)` for the database). Hammer `EmailLogin` until 429, confirm the response code and JSON body shape match what real clients will see.
- [ ] Run `npm run build` and `go test -tags=integration -timeout 20m ./...`.
- [ ] Manual test: `npm run server` with `--dev-mode=false`; hit `EmailLogin` from `curl` 6 times in 60 seconds, confirm the 6th returns 429 with a `resource_exhausted` Connect error.

**Validates:** Production traffic is throttled; dev-mode traffic is not; existing tests still pass.

### Phase 4: Documentation

**Goal:** Make the new pattern discoverable and the budgets reviewable.
**Tasks:**
- [ ] Add `server/middleware/ratelimit/README.md` covering: purpose, the `Rule` / `Budget` API, why it lives at the Connect-interceptor layer (typed-request access for email keys) rather than HTTP middleware, and the budget table. Style model: `server/middleware/README.md` for the small-package format.
- [ ] Update `server/middleware/README.md` to add a one-line pointer to the new sub-package.
- [ ] Update `server/services/login/README.md` to mention that production traffic is rate-limited, with a pointer to `ratelimit.DefaultLoginRules`.
- [ ] Add a one-line entry to the security review report (`docs/reviews/reports/security_review.md`) marking this finding as resolved with a PR link.

**Validates:** Future contributors can find and tune the budgets without spelunking.

## Related Issues

- #1530 — Companion security review finding (referenced by triage rationale: "narrower than #1530"). Likely a higher-priority auth issue; check whether anything in #1530 changes the surface this plan touches before implementation begins.
- (None of the existing `docs/issues/15*-*.md` plans overlap.)

## Architecture audit notes

Audited against `docs/server/architecture.md` and `CLAUDE.md`:

- **Service Independence** ✅ Rate limiting lives at the transport layer (interceptor), not as a service-to-service dependency.
- **Shared Functionality in Libraries** ✅ Code is placed in `server/middleware/ratelimit/`, mirroring focused-library patterns like `notifications/fcm`. Renamed the package from a single `ratelimit.go` file to a sub-package after audit because it lets us cleanly export non-stuttering names (`ratelimit.Rule`, `ratelimit.Budget`, `ratelimit.Interceptor`) per `revive.exported` rules and keeps the budget table colocated with the implementation.
- **Short, focused files** ✅ Implementation, tests, and `doc.go` are split into separate files; each well under the 1000-line cap.
- **Test coverage** ✅ Plan calls for unit tests (with a fake clock to avoid sleep-based flakiness), end-to-end smoke test against the real handler chain via `httptest.NewServer` + `storage.SetupTestStorage(t)`, and `-race` execution.
- **Logging** ✅ Uses `logging.LoggerWithContext`, `logging.MaskEmail`, `Warn` level for expected-but-noteworthy events — matches `docs/server/observability.md` conventions.
- **CLAUDE.md proto-comment ban** N/A — no proto changes.
- **CLAUDE.md "every package needs a `doc.go`"** ✅ Phase 2 explicitly creates `doc.go`.
- **CLAUDE.md "every multi-file directory needs a README"** ✅ Phase 4 adds `server/middleware/ratelimit/README.md`.
- **CLAUDE.md "fail fast / no silently-ignored errors"** ✅ Rate-limit rejection returns `connect.CodeResourceExhausted` (a real error to the caller); empty-IP/empty-email pass-through is *documented* in tests as a deliberate fail-open for transport-wiring bugs, not a silent error swallow.
