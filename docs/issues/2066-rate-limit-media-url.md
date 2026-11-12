# Issue #2066: Add per-user rate limit to `AddMediaFromURL` generic-URL path

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2066
**Filed:** 2026-05-20 | **Last activity:** 2026-05-20
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

> The user asked at planning time to broaden the scope: "is there any other
> rate limiting that we are currently doing or that we should be doing? I
> would like to do this in a standardized way for better security." This
> plan treats #2066 as the **first concrete application** of a
> standardized per-user rate-limit pattern, and uses the same plan to
> inventory what else needs the same treatment.

## Problem Statement

`AddMediaFromURL` (`server/services/media/service.go:145`) lets an
authenticated client request a server-side fetch of any HTTPS URL. PR
#2062 closed the SSRF gap by validating that the resolved IP is
publicly routable and constraining response content-type and body size.
What it did **not** add was a per-user budget on how often a single
authenticated caller can trigger an outbound fetch.

The risks are spelled out clearly in the issue:

- **Egress amplification** — an authenticated user can fan out
  arbitrary outbound fetches as fast as the network allows, turning the
  server into a download proxy on its public IP.
- **Per-user storage abuse** — every successful call writes a media
  row up to 60 MB plus a bucket object owned by the caller; a scripted
  client could exhaust GCS quota cheaply.
- **Cost surface** — bucket writes, storage rows, GCS egress on every
  hit. The generic-URL path has no cap.

The issue recommends Option A (per-user token bucket, sized for normal
Replace-Media interaction, ~10/min burst 3) and notes Options B
(per-instance) and C (interceptor pattern across the whole service) as
alternatives. **The codebase audit done while planning this issue
revealed Option C is already 80% built**, which changes the
recommendation.

## Context from Comments

No comments on the issue. The body itself is well-scoped, and the
audit findings below fill the remaining gaps.

## Codebase Audit Findings

### What rate limiting already exists

**`server/middleware/ratelimit/ratelimit.go`** ships a production
token-bucket Connect interceptor:

- Built on `golang.org/x/time/rate`.
- `Rule{Procedure, IPLimit, EmailLimit}` — per-procedure budgets keyed
  on client IP and on email (when the request message has a
  `GetEmail()` getter).
- `sync.Map`-backed limiter store with a 5-minute idle-eviction
  janitor — already solves the unbounded-memory worry.
- Logs rejections via `logging.LoggerWithContext` with
  `procedure`, `key_type`, `remote_addr`, masked `user_email`.
- Returns `connect.CodeResourceExhausted` (HTTP 429).
- Tested in `server/middleware/ratelimit/ratelimit_test.go` and in a
  smoke test under `server/services/login/ratelimit_smoke_test.go`.

### Where it's applied today

**Only on `LoginService`**, and only in production
(`server/main.go:1487-1489`). `cfg.devMode` bypasses it so simulations
and integration tests are not throttled. `DefaultLoginRules()` in
`ratelimit.go:200-254` covers email/OIDC/phone login + register,
password reset, refresh, and check-token endpoints with IP and
email budgets.

### What's missing for #2066

The interceptor **does not have a user-id key dimension**. The two
keys it supports today are:

- **IP** — read via `logging.RemoteAddrFromContext(ctx)`. Fine for
  unauthenticated endpoints; on authenticated traffic behind NAT, mobile
  carriers, or shared corporate egress it punishes legitimate users.
- **Email** — read by type-asserting the request message to
  `emailGetter { GetEmail() string }`. The `AddMediaFromURLRequest`
  message has no email field — this dimension would never fire for
  authenticated services.

So `AddMediaFromURL` can only be rate-limited per-IP today, which is
exactly the coarse failure mode Option B in the issue warned about.

### Why a user-id dimension is straightforward to add

`authMiddleware.Wrap()` is an **HTTP middleware** (wraps the Connect
handler from outside, see `server/main.go:708, 1437`). Connect
interceptors run **inside** the Connect handler. The order is:

```
HTTP → authMiddleware (sets authn ctx)
     → Connect handler
       → interceptors (metrics, then per-service)
         → RPC method
```

So `auth.GetAuthInfo(ctx)` (`server/auth/tokens.go:223`) is already
populated by the time the rate-limit interceptor runs. The interceptor
can read `authInfo.UserID` without any wiring changes.

### Other RPCs that need rate limiting

Once we have a `UserIDLimit` dimension, the same rule set should cover
every authenticated RPC that fans out to an external paid API or
amplifies egress. From the audit:

| RPC | Why | Risk |
|---|---|---|
| `MediaService.AddMediaFromURL` | This issue. Generic-URL fetch + 60 MB write. | egress + storage + GCS cost |
| `MediaService.AddMediaFromURL` (trusted path) | Same handler entry, Pexels/Unsplash via `CopyStockImageForUser`. | external API quota |
| `ExperienceService.StreamGenExperience{,FromText,FromImage,FromWebpage}` (`gen_ai_streaming.go:39,117,337,577`) | Streaming Vertex AI / Gemini calls. | Token spend, model latency, queue saturation |
| `GearService.StreamGenGear{,FromText,FromImage,FromWebpage}` (`gen_ai_streaming.go:40,129,232,411`) | Same. | Same. |
| `UnifiedCreateService.StreamGenUnifiedCreate` (`stream_gen.go:26`) | Fan-out of the gear/experience streams. | Same — typically the entry point clients hit. |
| `CommunityService.StreamGenCommunity` (community stream gen) | Same. | Same. |

These are the obvious tier-1 targets. A second sweep should pick up
any RPC that issues unbounded outbound HTTP (e.g. Mapbox geocoding,
webpage fetching) but the audit didn't surface any not already wrapped
by `AddMediaFromURL` or the gen-AI handlers.

### What we are NOT proposing to rate-limit (and why)

- **Read-heavy authenticated RPCs** — list/get of gear, communities,
  experiences, search. These are fast, mostly DB-bound, and have no
  external egress amplification. Adding rate limits here costs UX (legit
  scroll/refresh churn) for negligible safety gain.
- **Chat sends** — already constrained by UX flows; revisit if
  abuse appears.
- **AdminService** — dev-mode only; not exposed in production.

## Potential Solutions

### Option A — Per-handler token bucket (issue's recommendation, narrow)

Add an in-handler check at the top of `AddMediaFromURL` using a
package-local `*rate.Limiter` map keyed on user ID.

- **Pros:** Smallest possible change to land #2066 today. No
  cross-cutting design.
- **Cons:** Duplicates infrastructure that already exists in
  `server/middleware/ratelimit/`. Sets a bad precedent — the next RPC
  that needs the same protection will copy this code instead of
  reaching for the interceptor. Doesn't address the user's
  standardization request.
- **Effort:** small.

### Option B — Extend `ratelimit.Rule` with `UserIDLimit`, wire across services (recommended)

Add a third dimension to the existing `Rule` struct:

```go
// UserIDLimit enforces a per-(procedure, user-id) token-bucket limit.
// User ID is extracted from auth.GetAuthInfo(ctx); procedures whose
// caller has no authenticated user (e.g. LoginService) skip this dim.
UserIDLimit *Budget
```

In the interceptor's match step, after the existing IP and email
checks, look up `auth.GetAuthInfo(ctx)`; if present and non-empty,
key on `proc + "\x00" + authInfo.UserID`. Reuse the same
`sync.Map` + janitor machinery. Add `DefaultMediaRules()`,
`DefaultGenAIRules()` constructors that return the per-procedure
budgets — same shape as `DefaultLoginRules()`. Wire each authenticated
service's `connect.HandlerOption` chain to include the rate-limit
interceptor with the appropriate rule set.

- **Pros:** One pattern across the codebase. Reuses tested machinery.
  Per-user-id keying is the right granularity for authenticated
  traffic. Future RPCs get added by appending a `Rule` literal to the
  appropriate `Default*Rules()` constructor.
- **Cons:** Touches more files than Option A. Requires deciding on
  budgets for multiple RPCs at once (mitigation: phase the rollout —
  ship the extension first, apply to `AddMediaFromURL` next, apply to
  gen-AI in a follow-up).
- **Effort:** medium for the package extension + #2066 wiring; the
  gen-AI follow-up is additive.

### Option C — Lift rate-limiting into a global default interceptor

Wrap every service in the interceptor with an empty rule set by
default, so adding a rule never requires touching `main.go`. Optional
follow-up after Option B lands.

- **Pros:** Adding rules becomes pure additive code in
  `ratelimit/rules.go`; no per-service wiring changes.
- **Cons:** Bigger architectural change, defer until at least three
  services use the interceptor.
- **Effort:** small once Option B is in place.

## Recommendation

**Option C, sequenced after the Phase 1 package extension.** During
planning the user asked whether there's any real cost to doing C from
the start. The honest answer is *no* — the costs (slightly larger
first PR, devMode-bypass moves up one level, interceptor-ordering
needs a moment of thought to keep metrics outside rate-limit) are
trivial, and the standardization benefit is real: **every rate limit
in the server lives in one file**, easy to audit, easy to extend.
Adding a new rule becomes a one-line edit to `AllRules()` with no
`main.go` change.

The phasing below lands the package extension first (Phase 1), then
hoists into `commonOpts` as the shared interceptor (Phase 2), then
applies to `AddMediaFromURL` to close #2066 (Phase 3). Gen-AI rules
follow in a separate issue (Phase 4).

## Open Questions

- **Should the existing `EmailLimit` dimension stay, or be removed in
  favor of `UserIDLimit`?** — Recommendation: **keep `EmailLimit` for
  unauthenticated procedures only** (login, register, password reset
  on the LoginService — these don't have a `UserID` yet at the
  interceptor layer). For authenticated services use `UserIDLimit`.
  Document the split in the package doc.

- **What budget for `AddMediaFromURL`?** — Recommendation:
  `rate.Every(6 * time.Second), Burst: 3` (10/min, burst 3). Matches
  the issue's suggested shape and is comfortably above any normal
  Replace-Media interaction (a user usually picks one candidate per
  modal open).

- **Apply to the trusted-provider path too?** — Recommendation:
  **yes, but at the same rule.** The handler entry is shared
  (`AddMediaFromURL`), so the interceptor matches the procedure name
  regardless of which branch the handler takes internally. Trusted
  path is safer but still hits an external API per call; one rule for
  the whole RPC is simpler than two.

- **Soak mode (warn-only) before enforcing?** — Recommendation: **no
  for `AddMediaFromURL`** — the budget is so far above legitimate use
  (10/min vs. ~1/modal) that the false-positive risk is negligible. If
  we add a `MetricsMode` flag later for the gen-AI RPCs (where budgets
  will be tighter and the false-positive risk higher), that's the time
  to add it package-wide.

- **Should we expose Retry-After?** — Recommendation: **yes,
  package-wide in Phase 1.** Compute
  `time.Until(lim.Reserve().Delay())`, ceil to whole seconds, and set
  it as the standard HTTP `Retry-After` response header via Connect's
  error metadata API:

  ```go
  e := connect.NewError(connect.CodeResourceExhausted, errRateLimited)
  e.Meta().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
  return nil, e
  ```

  `connect.Error.Meta()` is `http.Header`; the value travels back as
  a normal HTTP header so well-behaved clients (and intermediaries
  like CDNs) can back off correctly without parsing custom metadata.

- **Dev-mode behavior?** — Recommendation: **match the LoginService
  precedent** — skip rate-limit interceptors entirely in `cfg.devMode`
  so integration tests and simulation runs aren't throttled.

## Implementation Plan

### Phase 1 — Add `UserIDLimit` dimension and `Retry-After` to `ratelimit` package

**Goal:** The pattern for per-user-id rate limiting exists and is
tested, but is not yet wired to any RPC. Standalone, mergeable, no
behavior change for existing LoginService rules.

**Tasks:**
- [ ] Extend `Rule` in `server/middleware/ratelimit/ratelimit.go` with
  `UserIDLimit *Budget`.
- [ ] Add a `userIDFromContext` helper (in the `ratelimit` package, not
  a new import cycle: depend on `server/auth` for `GetAuthInfo`). The
  helper extracts `authInfo.UserID` and returns "" if absent.
- [ ] In `Interceptor()`, after the email-dimension block, add a
  user-id dimension block keyed `proc + "\x00" + userID`. Log
  rejection via `logging.LoggerWithContext(ctx).WarnContext(...)` with
  fields `procedure`, `key_type: "user_id"`, and `user_id` (raw; the
  observability contract in `docs/server/observability.md` already
  treats `user_id` as a standard, non-masked field — emails and tokens
  are masked, IDs are not). `request_id` is added automatically by
  `LoggerWithContext` so rejections are joinable to the originating
  HTTP request.
- [ ] Compute `Retry-After` for every rejection (ceil
  `time.Until(lim.Reserve().Delay())` to whole seconds) and set it via
  `connect.Error.Meta().Set("Retry-After", ...)` so it travels back as
  a standard HTTP header. Apply to all three dimensions.
- [ ] Add a Prometheus rejection counter following the conventions in
  `docs/server/metrics.md`:
  `ripls_ratelimit_rejections_total{procedure, key_type}` —
  `procedure` is the full Connect FQN (matches the log field for
  joinability), `key_type` ∈ {`ip`, `email`, `user_id`}. Register the
  collector in the package init alongside the existing metrics in
  `server/middleware/` so it shows up at `/metrics` automatically. Do
  **not** add a `mode` label until Phase 3 introduces metrics-only mode.
- [ ] Tests:
  - `TestInterceptor_UserIDLimit_AllowsUnderBudget`
  - `TestInterceptor_UserIDLimit_RejectsAboveBudget`
  - `TestInterceptor_UserIDLimit_SkipsWhenUnauthenticated`
  - `TestInterceptor_RetryAfter_HeaderPopulated`
  - `TestInterceptor_Metrics_RejectionCounterIncremented`
- [ ] Update the package `doc.go` (or `ratelimit.go` package comment)
  to document: IP is for unauthenticated traffic, email is for
  unauthenticated-but-email-bearing requests, user-id is for
  authenticated traffic. Also note the deliberate field-name choice:
  the interceptor logs `procedure` (full Connect FQN, e.g.
  `/ripls.api.MediaService/AddMediaFromURL`), not the standard
  `operation` field (handler method name). The Prometheus counter uses
  the same `procedure` label so logs and metrics join cleanly.

**Validates:** `npm run build` passes (golangci-lint, including
`revive.exported`, `gofumpt`, `godot`); all new tests pass; existing
LoginService rate-limit tests still pass unchanged.

### Phase 2 — Hoist into a single shared interceptor via `commonOpts`

**Goal:** Move from per-service `connect.WithInterceptors(ratelimit.Interceptor(Default<Foo>Rules()...))`
to one shared interceptor applied to every service via
`commonOpts`. After this lands, adding a new rule is a one-line
edit to `AllRules()` with no `main.go` changes — the standardized
shape the user asked for.

**Tasks:**
- [ ] Add `AllRules()` to `ratelimit.go` that concatenates every
  `Default*Rules()` constructor. Today that's only `DefaultLoginRules()`;
  later phases append `DefaultMediaRules()`, `DefaultGenAIRules()`, etc.
- [ ] In `server/main.go`, build the shared interceptor inside the
  `commonOpts` slice, gated on `!cfg.devMode`. Order matters:
  metrics interceptor must wrap *around* the rate-limit interceptor
  so rejections still count toward `ripls_connect_rpcs_total{connect_code="resource_exhausted"}`:

  ```go
  commonOpts := []connect.HandlerOption{
      connect.WithInterceptors(middleware.ConnectMetricsInterceptor()),
  }
  if !cfg.devMode {
      commonOpts = append(commonOpts,
          connect.WithInterceptors(ratelimit.Interceptor(ratelimit.AllRules()...)))
  }
  ```
- [ ] Remove the dedicated LoginService rate-limit block at
  `server/main.go:1486-1490` — LoginService now inherits the same
  shared interceptor as every other service. The `DefaultLoginRules()`
  entries are picked up by procedure-name match.
- [ ] Verify the LoginService smoke test
  (`server/services/login/ratelimit_smoke_test.go`) still passes
  unchanged (it constructs the interceptor directly; the wiring
  change is in `main.go` only).
- [ ] Add an integration-style test in
  `server/main_integration_test.go` (or equivalent) confirming that
  in non-devMode the LoginService EmailLogin endpoint enforces its
  budget, and in devMode it does not. This locks in the bypass
  behavior so future `commonOpts` edits don't accidentally regress it.

**Validates:** `npm run build` passes; LoginService rate-limit behavior
unchanged (existing smoke test green); rejected requests still appear
in `ripls_connect_rpcs_total` with `connect_code="resource_exhausted"`
(verified by counting before/after in the new integration test).

### Phase 3 — Apply per-user rate limit to `AddMediaFromURL` (closes #2066)

**Goal:** Ship the protection the issue asks for. With Phase 2 in
place, this is now a one-liner addition to `AllRules()`.

**Tasks:**
- [ ] Add `DefaultMediaRules()` to `ratelimit.go` returning one
  `Rule` for `apiconnect.MediaServiceAddMediaFromURLProcedure` with
  `UserIDLimit: &Budget{Rate: rate.Every(6 * time.Second), Burst: 3}`
  (10/min, burst 3).
- [ ] Append `DefaultMediaRules()...` to `AllRules()`. No `main.go`
  edit needed.
- [ ] Remove the `TODO(#2066)` block in
  `server/services/media/service.go:141-144`.
- [ ] Add a smoke test
  `server/services/media/ratelimit_smoke_test.go` mirroring
  `server/services/login/ratelimit_smoke_test.go` —
  use a fake stock-imagery provider (already exists in the media
  service tests) so the test doesn't hit real outbound. The test
  must inject an authenticated context via `authn.SetInfo(ctx, &auth.Info{UserID: ...})`
  before making the call (since `UserIDLimit` extracts user-id from
  auth context, not the request body). Verify the 4th request in a
  burst of 4 returns `connect.CodeResourceExhausted` and that the
  resulting `*connect.Error.Meta().Get("Retry-After")` is a positive
  integer string.
- [ ] PR description includes a one-liner on the budget rationale
  (typical user opens a Replace Media modal a handful of times per
  session; 10/min is ~30× headroom).

**Validates:** `npm run build` passes; smoke test passes; in dev
mode `npm run start:server` does not rate-limit any RPC (manual
check). Closes #2066.

### Phase 4 — Apply to AI streaming generators (follow-up issue, not #2066)

**Goal:** Extend the same pattern to the AI-backed gen RPCs where the
risk surface is biggest (token spend, model quota, latency).
Conservative budgets, with telemetry first.

**Tasks (file a new issue, do not include in #2066 PR):**
- [ ] Add `DefaultGenAIRules()` covering
  `StreamGenExperience{,FromText,FromImage,FromWebpage}`,
  `StreamGenGear{,FromText,FromImage,FromWebpage}`,
  `StreamGenCommunity`, `StreamGenUnifiedCreate`. Suggested budget:
  `rate.Every(15 * time.Second), Burst: 4` per RPC, per user (4/min,
  burst 4). Tighter than media because each call is expensive.
- [ ] Append `DefaultGenAIRules()...` to `AllRules()`.
- [ ] Add a metrics-only mode to the package (`Budget.MetricsOnly bool`)
  so gen-AI budgets can soak in production before flipping to enforce.
  Same shape as the GetMedia auth log-only phase from #1529. When this
  lands, add a `mode` label to
  `ripls_ratelimit_rejections_total` (values: `enforce`, `metrics_only`)
  so the soak data is visible in PromQL before enforcement flips.
- [ ] Decide on coupling between `StreamGenUnifiedCreate` (the
  entry-point fan-out) and the downstream Stream* RPCs it triggers —
  if `UnifiedCreate` is rate-limited tightly, the downstream RPCs
  effectively are too. Avoid double-counting.
- [ ] Streaming-RPC interaction note: the gen-AI handlers use Connect
  streams (`ServerStream`), not Unary. The current
  `Interceptor()` returns a `UnaryInterceptorFunc` only. Build the
  parallel `StreamingHandlerInterceptorFunc` (mirrors the unary
  shape, looks up the same `Rule` registry by procedure name) and
  wire it via the same `connect.WithInterceptors` call in
  `commonOpts`. Without this, gen-AI streams effectively bypass
  rate limiting.

**Validates:** No behavior change in production (metrics-only first);
budgets visible in logs; flip to enforce once burst patterns are
characterized.

## Related Issues

- **#2038** — feature that introduced `AddMediaFromURL`.
- **#2062** — implementation PR; landed the SSRF guards but left rate
  limiting as a follow-up.
- **#1529** — per-resource auth check for `GetMedia`; the
  metrics-only / log-only soak pattern from there is the model for
  Phase 3's `MetricsOnly` budget knob.
- **#2076 / #2077 / #2078** — performance follow-ups to #2055.
  Independent, but the structured-logging conventions used there
  (`storage.WithQueryStats`, `logging.LoggerWithContext`) are the
  same ones the rate-limit telemetry should follow.

---

## Audit notes (added by `/ripls-audit` on 2026-05-21)

Reviewed against `docs/server/architecture.md`,
`docs/server/observability.md`, and `docs/server/metrics.md`. (Client
docs not applicable — this is a server-only change with no UI
impact.)

Changes applied to the plan:

- **Phase 1 logging shape** — spelled out the `LoggerWithContext` +
  `WarnContext` shape and confirmed `user_id` is logged raw per the
  observability contract (entity IDs are not masked; only emails and
  tokens are). Noted that `request_id` is added automatically so
  rejections join cleanly to the originating HTTP request.
- **Phase 1 `Retry-After` mechanics** — replaced "stuff into Connect
  error metadata as a string" with the concrete
  `connect.Error.Meta().Set("Retry-After", ...)` call. `Meta()` is
  `http.Header`, so the value rides back as a standard HTTP header —
  CDNs and well-behaved HTTP clients pick it up without parsing custom
  metadata. Smoke test asserts the header via
  `*connect.Error.Meta().Get("Retry-After")`.
- **Phase 1 metrics** — added a Prometheus counter
  `ripls_ratelimit_rejections_total{procedure, key_type}` following
  the `ripls_` prefix and bounded-cardinality conventions in
  `docs/server/metrics.md`. Phase 3 (metrics-only mode) adds the
  `mode` label.
- **Phase 1 field-name choice** — documented the deliberate use of
  `procedure` over the standard `operation` field: the interceptor
  works in terms of full Connect FQNs
  (e.g. `/ripls.api.MediaService/AddMediaFromURL`), not handler
  method names. Logs and metrics use the same `procedure` label so
  they join cleanly.
- **Phase 2 smoke-test auth context** — clarified the test must
  inject auth context via `authn.SetInfo(ctx, &auth.Info{UserID: ...})`
  because `UserIDLimit` reads from the auth context, not the request
  body — without this the test would silently pass-through the new
  dimension.
- **Phase 3 streaming vs. unary interceptor** — added a task to
  confirm whether
  `connect.StreamingHandlerInterceptorFunc` is needed alongside the
  existing `UnaryInterceptorFunc`. The gen-AI RPCs are streams, not
  unary, so Phase 3 may need a parallel streaming interceptor —
  this needs to be confirmed before sizing the work.
- **No other changes needed** — the package already follows the
  architecture-doc patterns (standalone library in `server/middleware/`,
  interface-based test stubs, `sync.Map` + janitor for bounded
  memory). Auth extraction uses `auth.GetAuthInfo(ctx)` (matches
  current code), not the stale `auth.UserFromContext(ctx)` shown in
  the architecture doc's RPC handler example. Server-only change, so
  no client/i18n/a11y impact to consider.
