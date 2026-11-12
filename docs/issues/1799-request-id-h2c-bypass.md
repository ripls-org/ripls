# Issue #1799: bug: X-Request-ID threading broken between Flutter client and server (blocks #1581 audit)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1799
**Filed:** 2026-05-09 | **Last activity:** 2026-05-09
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Audit log (2026-05-09)

Audited against `docs/server/observability.md`,
`docs/server/architecture.md`, `docs/client/observability.md`,
`docs/client/architecture.md`, and `docs/client/testing.md`. Changes
made:

- **Phase 2 — AuthService header threading**: clarified that
  `RpcUtils.buildHeaders` must NOT receive an `onUnauthenticated`
  callback for login/register flows. The helper invokes that callback
  whenever the access-token getter returns null, which is the
  *expected* state during login. Without this guard the change would
  cause every login attempt to fire the unauthenticated handler. The
  call site is now spelled out explicitly.
- **Phase 1 — `clock.SimulationTimestamp` placement**: turned the
  vague "sanity-check" task into a concrete instruction. The clock
  middleware reads `X-Simulation-Timestamp` per request and must be
  inside the per-stream chain for the same reason every other
  middleware must — otherwise simulator-driven HTTP/2 traffic loses
  injected timestamps.
- **Phase 3 — added a hermetic Dart-side test** alongside the Go
  integration test. We can verify each side independently without
  spinning up the full stack: a Dart unit test injects a fake
  `httpClient` callback into a `connect.Transport` and asserts that
  user-supplied `X-Request-ID` reaches the request frame. Pairs
  cleanly with the Go-side test that asserts the server log captures
  the same UUID.
- **Open Questions — added two**: (a) whether to bump
  `&http2.Server{}` away from zero defaults, and (b) whether to
  unify `RequestIDFromContext` / `WithRequestID` naming with the doc
  (which calls it `ContextWithRequestID`). Both are deferrable; flagged
  to avoid surprise during review.
- **Standard-field naming sanity check**: confirmed the plan uses
  `request_id` (HTTP correlation) consistently with
  `docs/server/observability.md:106` and never collides with
  `target_request_id` (entity ID). No change needed; recorded for
  reviewers.

No changes from caching, i18n, or accessibility audits — this issue
touches none of those layers.

## Cleanup landed alongside this plan (2026-05-09)

The request-id naming cleanup happened in the same branch, before the
server-side h2c fix:

- `app/lib/core/utils/request_id.dart` — added `RequestIdGenerator.current`
  and `RequestIdGenerator.runWithRequestId(...)` for Zone-scoped
  propagation.
- `app/lib/core/utils/rpc_utils.dart` — `executeRpc` now generates a
  fresh request_id per call, runs the rpc closure inside
  `runWithRequestId`, and `buildHeaders` reuses the zone-active id when
  one is set. Net effect: a single id per RPC, shared by the outbound
  `X-Request-ID` header and any client-side log entries emitted while
  the RPC is in flight.
- `app/lib/core/observability/logging/logger.dart` — deleted dead
  `LogContext.requestId` / `LogContextHolder.setRequestId` (never
  wired in production, race-prone as a global static). `ObservableLogger`
  now reads the zone-current id at log time.
- `app/test/core/utils/{request_id,rpc_utils}_test.dart`,
  `app/test/core/observability/logging/logger_test.dart` — tests
  rewritten to drive request_id through the zone; new assertions lock
  in the executeRpc ↔ buildHeaders contract.
- `docs/server/observability.md` — fixed `ContextWithRequestID` →
  `WithRequestID` doc drift, and added a "Request ID glossary" section
  distinguishing the HTTP correlation request_id from the
  `models.Request` entity ID (logged as `target_request_id`).

These changes mean the Phase 3 client-side regression tests already
exist; only the server-side hermetic test remains to be written.

## Problem Statement

`X-Request-ID` correlation between the Flutter client and the Go server
is broken in two distinct, interacting ways:

1. The 619 unique UUIDs the Flutter client emits per RPC over a 7-minute
   walkthrough do **not** appear in any of 95k server log lines for the
   same window. The middleware never sees the client's header.
2. A single server-generated `request_id` (`748b9982-…`) is reused
   across 3,424 log lines spanning unrelated RPCs (login
   `GetUserByEmail`, `ListCommunities`, multiple `StreamMessages`
   invocations on different conversations, `ResolveConnectionContext`,
   etc.). A `request_id` is supposed to scope to a single HTTP request,
   so even server-only correlation is broken.

The other 126 server-side IDs in the post-seed window are essentially
all attached to single `/media/...` GETs (the Flutter image loader
doesn't decorate those with `X-Request-ID`, so the server's
auto-generation kicks in once per request — those work correctly).
HTTP/1.1 traffic from the seeding sim binary (11k+ matched lines) also
threaded `X-Request-ID` end-to-end with no problem in the same server
binary, ruling out any "request_id middleware is broken everywhere"
theory.

The combination blocks the analysis in #1581: client RPC bursts and
cache misses cannot be joined to server `db_queries` /
`response_bytes` by `request_id`. It also breaks Crashlytics → server
log joins via `RpcErrorHandler` and any production Cloud Logging
queries that key off the correlation field.

## Context from Comments

The issue has no comments. The body itself is the canonical write-up,
including evidence, suspects ranked, and a suggested order of attack.
No comments means no maintainer pushback or revision since filing —
treat the body as the spec.

## Codebase findings (verifying the issue's hypotheses)

I traced the middleware chain and the Connect-Dart transport before
proposing solutions. Three concrete findings:

### Finding 1 (root cause): `h2c.NewHandler` bypasses the outer middleware

`server/main.go:1185-1202`:

```go
var inner http.Handler = h2c.NewHandler(mux, &http2.Server{})
// ... clock.SimulationTimestamp wrap in dev ...
handler := middleware.PanicRecovery(logger)(
    middleware.RequestID(
        middleware.RemoteAddr(
            middleware.Logging(logger)(
                middleware.Metrics()(
                    middleware.QueryStats(logger)(
                        corsMiddleware(inner),
                    ),
                ),
            ),
        ),
    ),
)
server := &http.Server{Addr: addr, Handler: handler, ...}
```

The order is: **outer middleware → CORS → h2c → mux**. That looks
correct on paper, but `golang.org/x/net/http2/h2c` does something
sneaky for HTTP/2 cleartext: when it detects either an
`Upgrade: h2c` request or the prior-knowledge HTTP/2 preface, it
**hijacks the underlying TCP connection** and hands it off to the
embedded `http2.Server` (`golang.org/x/net@v0.44.0/http2/h2c/h2c.go:97-119`):

```go
s.s.ServeConn(conn, &http2.ServeConnOpts{
    Context:          r.Context(),  // ← the upgrade request's ctx
    Handler:          s.Handler,    // ← the inner h: just `mux`
})
```

After hijack, the http/2 server creates a fresh `*http.Request` for
**each new HTTP/2 stream** on that connection and calls
`s.Handler.ServeHTTP` (== `mux.ServeHTTP`) directly. The outer
middleware chain is permanently bypassed for every subsequent unary
call and stream on that connection. This single behavior explains
**all three** server-side symptoms in the issue:

- **Client `X-Request-ID` is not visible on the server.** The
  `RequestID` middleware never executes for HTTP/2 streams, so
  `r.Header.Get("X-Request-ID")` never runs.
- **One `748b9982` for the whole session.** The hijack uses
  `Context: r.Context()` from the **single** HTTP/1.1 upgrade
  request — the only request that *did* run through `RequestID`
  middleware. Every HTTP/2 stream inherits that context and therefore
  that one `request_id` value, so every `LoggerWithContext(ctx)` call
  on those streams emits the same ID. That is exactly the
  3,424-line shared scope the issue describes.
- **The `http request …path=/ripls.api…` close-line is missing.** The
  `Logging` middleware never runs for HTTP/2 streams either, so no
  close-line is emitted for Connect RPCs over HTTP/2. `/media/...` GETs
  arrive over HTTP/1.1 (h2c does **not** hijack those), so they
  continue to log normally. This matches the 119 close-lines for
  `/media/` paths and zero for `/ripls.api`.

### Finding 2 (secondary): CORS allow-list omits `X-Request-ID`

`server/main.go:1181`:

```go
handlers.AllowedHeaders([]string{
  "Content-Type", "Authorization",
  "Connect-Protocol-Version", "Connect-Timeout-Ms",
}),
```

`X-Request-ID` is not in the list. `ExposedHeaders` does include
`middleware.RequestIDHeader` (line 1182), confirming the round-trip
intent. This bug is real but only affects browsers (which run CORS
preflight) — native iOS/Android clients ignore CORS, so this is **not**
the cause of the iOS-Simulator symptom in the issue. It still needs
fixing for any future web client and is a one-line change, so we'll
roll it in.

### Finding 3 (client gap): `AuthService` doesn't send `X-Request-ID`

`app/lib/services/auth_service.dart` calls `_client.emailRegister(req)`,
`_client.emailLogin(req)`, etc. **without** the `headers:` named
argument. None of the login/register RPCs are decorated with
`RpcUtils.buildHeaders()`, so even after fixing the middleware bypass
above, those calls will still arrive without an `X-Request-ID`. Most
other services (`chat_service.dart`, `community_service.dart`, etc.)
do thread headers via a private `_buildHeaders()` helper.

### Connect-Dart 1.0.0 transport behaves correctly

I traced `package:connectrpc/connect.dart` from the generated client
(`login_service.connect.client.dart:20-27`) through
`Client.unary` → `ProtocolTransport.unary`
(`connectrpc-1.0.0/lib/src/protocol/transport.dart:50-65`) →
`Connect.requestHeaders` (`lib/src/connect/protocol.dart:42-58`) →
`requestHeader()` (`lib/src/connect/header.dart:33-67`) →
`Headers.from(userProvidedHeaders)`. The user's `X-Request-ID` is
preserved (lowercased to `x-request-id` per HTTP/2 rules) and emitted
on the request frame via the http2 transport
(`lib/src/http2/http2.dart:47-48`).

The Connect-Dart side is **not** the bug. We don't need a `tcpdump`.

## Potential Solutions

### Option A: Move the middleware chain inside `h2c.NewHandler`

- **Approach:** Wrap `mux` with the middleware chain first, then pass
  the wrapped mux as the inner handler to `h2c.NewHandler`. The
  hijack path already calls `s.Handler.ServeHTTP` per HTTP/2 stream,
  so each stream automatically runs through the full middleware. The
  HTTP/1.1 fallback (line 125 of `h2c.go`: `s.Handler.ServeHTTP(w, r)`)
  uses the same wrapped handler, so HTTP/1 traffic still gets
  middleware too.
- **Pros:** Surgical, ~5-line diff in `server/main.go`. Fixes both
  symptoms (header pass-through *and* per-stream `request_id`) in one
  shot. No new dependencies, no protocol changes.
- **Cons:** Requires care with `RemoteAddr` middleware — the http/2
  server populates `r.RemoteAddr` from the underlying conn for each
  stream, so this should still work, but I want a regression test
  asserting per-stream remote-addr threading. `PanicRecovery` was
  outermost in the current setup; inside h2c, a panic in
  `h2c.NewHandler` itself would no longer be caught — but h2c itself
  does its own recovery and this is a non-issue in practice.
- **Effort:** Small.

### Option B: Add a Connect-Go interceptor that re-derives `request_id`

- **Approach:** Register a `connect.UnaryInterceptorFunc` /
  `StreamingInterceptorFunc` that pulls `X-Request-ID` from the
  incoming Connect request headers, generates one if missing, and
  injects it into context via `logging.WithRequestID`. Wire it
  alongside the existing `ConnectMetricsInterceptor` in
  `server/main.go:1041`.
- **Pros:** Bypasses the h2c question entirely — Connect interceptors
  always run per RPC regardless of transport. Easier to reason about
  for streams (interceptor sees both unary and streaming).
- **Cons:** Doesn't fix the missing close-line in
  `middleware/logging.go` (that's HTTP-level). Doesn't fix the
  missing `Logging`, `Metrics`, `QueryStats`, `RemoteAddr`,
  `PanicRecovery` for HTTP/2 streams either — just the request_id
  symptom. We'd be papering over the bigger problem and would still
  have invisible HTTP-level metrics for all Connect traffic.
- **Effort:** Small in isolation; large if we then have to duplicate
  every middleware as an interceptor.

### Option C: Stop using h2c; require TLS + ALPN in production

- **Approach:** Drop `h2c.NewHandler` entirely. Use HTTPS with proper
  ALPN-negotiated HTTP/2 (Go's `http.Server` handles HTTP/2 natively
  when ListenAndServeTLS is used). For local dev, run behind a TLS
  proxy or use HTTP/1.1.
- **Pros:** Removes the entire class of bug; HTTP/2 over TLS goes
  through `Server.Handler` like normal.
- **Cons:** Massive blast radius. Cloud Run already terminates TLS in
  front, but local dev, integration tests, the simulator binary, and
  CI all assume cleartext + h2c. Would require changes throughout
  development tooling. Wrong tool for this bug.
- **Effort:** Large.

## Recommendation

**Option A.** It's the smallest change that addresses every observable
symptom in the issue (no client header reaching the server, the shared
`748b9982`, the missing `/ripls.api` close-line). Option B only fixes
one of three symptoms and leaves a worse tail of duplicated middleware.
Option C is the right answer in a vacuum but isn't proportionate to
the bug.

Roll the CORS allow-list fix (`Finding 2`) and the `AuthService`
client-side header threading (`Finding 3`) in alongside, since they're
each one-file, one-pattern changes that improve correlation hygiene
once the main fix lands.

## Open Questions

- **How should we order the middleware once it moves inside h2c?** —
  Recommendation: keep the existing semantic order
  (PanicRecovery outermost, then RequestID, RemoteAddr, Logging,
  Metrics, QueryStats, CORS, mux). The only behavioral change is that
  the chain now executes per HTTP/2 stream instead of per
  TCP-connection-upgrade. CORS innermost is fine: native HTTP/2
  clients don't preflight, and browsers preflight via OPTIONS over
  HTTP/1.1 — that path is unaffected.

- **Will moving `Logging` inside h2c flood the logs for long-lived
  streams?** — Recommendation: no, because `Logging` writes one line
  *at stream close*, not per message. For `StreamMessages` /
  `StreamCommunityEvents` that's still one line per logical stream.
  Per-event correlation inside a stream is a separate concern (next
  question).

- **Streams will still share one `request_id` across thousands of
  events. Do we add a per-event sub-ID now?** — Recommendation: defer.
  The audit in #1581 cares primarily about unary RPCs; once unary
  correlation works, we can decide whether streams need a nested ID
  (e.g., `request_id` for the stream + `event_seq` for the message).
  File a follow-up issue but don't gate this fix on it.

- **Do we keep the CORS allow-list fix even if we never run a web
  client?** — Recommendation: yes. The list and the corresponding
  `ExposedHeaders` already document round-trip intent (`X-Request-ID`
  is exposed but not allowed-in, which is incoherent), and the cost is
  literally one string. Future web/admin tooling will hit this.

- **Should `AuthService` send `X-Request-ID` for unauthenticated
  calls?** — Recommendation: yes. `RpcUtils.buildHeaders` already
  handles the no-token case (it just omits `Authorization`). Threading
  it through login/register makes Crashlytics → server-log joins work
  for the registration funnel too.

- **Do we add an integration test that round-trips `X-Request-ID`
  across HTTP/2?** — Recommendation: yes. A test in
  `server/integration_tests/` that opens a Connect-Go HTTP/2 client,
  stamps a known UUID, and asserts the server logs (or a test-only
  hook) saw the same value. This is the one test that would have
  caught the original bug and would catch any regression where the
  middleware order silently drifts back.

## Implementation Plan

### Phase 1: Move middleware inside h2c (the actual fix) — DONE

**Goal:** Get client `X-Request-ID` onto the server's `request_id`
log field for every HTTP/2 RPC, and ensure each unary call /
each stream gets its own `request_id` in context (no more shared
`748b9982`).

**Tasks:**
- [x] Edit `server/main.go:1185-1202`. Build the middleware chain
  around `mux` first, then pass the wrapped handler as the inner
  argument to `h2c.NewHandler`. PanicRecovery stays as the outermost
  wrapper so it catches panics from the h2c hijack path itself.
- [x] `clock.SimulationTimestamp` (dev mode) is now also inside the
  per-stream chain, so simulator-driven HTTP/2 traffic preserves
  injected timestamps.
- [x] `npm run build` passes.
- [x] `go test ./server/middleware/... ./server/logging/...` and
  `go test -run TestLogging ./server/integration_tests/...` all
  green; no middleware-order regressions.

**Validates:**
- Manual reproduction (the issue's repro steps): launch local server,
  run iOS Simulator app, exercise a few screens, then `grep
  request_id` across both logs. Pick a client UUID and confirm it
  shows up server-side. Pick a server `request_id` and confirm it
  appears on a *bounded* number of log lines (one unary RPC's worth,
  or one stream's lifetime — not 3,424 lines across unrelated calls).
- The user has offered to run the app manually. Use that for the
  walkthrough at the end of Phase 1 before declaring the fix done.

### Phase 2: CORS allow-list and AuthService client-header hygiene — DONE

**Goal:** Round out client/server correlation for the cases the main
fix doesn't touch (browsers, login flow).

**Tasks:**
- [x] `server/main.go`: added `middleware.RequestIDHeader` to
  `handlers.AllowedHeaders`. Matches the constant already used in
  `ExposedHeaders` two lines below.
- [x] `app/lib/services/auth_service.dart`: introduced a private
  `_buildHeaders()` calling `RpcUtils.buildHeaders(() => null)`
  (without an `onUnauthenticated` callback, since the missing token
  is the expected state during login). Threaded
  `headers: _buildHeaders()` through all 11 `_client.*` call sites
  (emailRegister, emailLogin, oIDCRegister, oIDCLogin, phoneRegister,
  phoneLogin, refreshToken, checkInvitation, requestPasswordReset,
  checkResetPasswordToken, resetPassword).

**Validates:**
- Browser CORS preflight no longer strips `X-Request-ID` (manual
  test against a future web build, or a synthetic curl with the
  appropriate Origin header).
- Login/register RPCs in the server log carry the same UUID the
  client emitted (verify via the same grep workflow as Phase 1).

### Phase 3: Regression tests on both sides

**Goal:** Lock in the fix so a future middleware re-shuffle can't
silently revert it. Each side gets a hermetic test — no need to spin
up the full Flutter-app-talking-to-Go-server stack to validate the
correlation contract.

**Client-side (already landed as part of the request-id cleanup
work).** `app/test/core/utils/rpc_utils_test.dart` now contains:

- `executeRpc publishes a request_id to the active zone for the rpc closure`
- `buildHeaders inside executeRpc reuses the executeRpc id`
- `each executeRpc call gets a distinct request_id`

Together these prove that (a) every RPC stamps an `X-Request-ID`
header, (b) the same id is visible to client-side logs emitted during
the call, and (c) the id is per-call (no leakage across calls). No
network needed.

**Server-side — DONE.** `server/integration_tests/request_id_h2c_test.go`:

- boots the test server with the real middleware chain
- constructs a Connect-Go client over HTTP/2 (h2c, matching dev/prod
  transport parity — this is where the original bug lived)
- sets a known `X-Request-ID` UUID on a unary call
- calls a small RPC (e.g., `HealthService.Check`)
- asserts that the server's logger observed exactly that UUID for
  the call's log entries (introduce a test-only `slog.Handler` that
  captures records, or expose the request_id via a debug response
  header)
- runs *two* sequential calls on the same Connect client (which
  reuses the HTTP/2 connection) and asserts the server emits *two
  distinct* `request_id` values — directly guarding the shared-context
  regression that #1799 reported

Wire under the `integration` build tag if it needs extra plumbing;
otherwise put it in the standard suite. Note the CLAUDE.md guidance
about `go build -tags=integration ./...` if any exported signature is
touched.

**End-to-end walkthrough (one-time, manual).** Phase 1's validation
step — iOS Simulator → local server, grep a client UUID in both
logs — is the only step that requires both pieces running together.
After it passes once, the per-side regression tests above are what we
rely on going forward.

**Validates:** Both halves of the original bug — header pass-through
*and* per-stream isolation — are caught by CI from now on, on both
sides of the wire.

### Phase 4: Per-event correlation inside chat streams — DONE

**Goal:** Slice a long-lived stream's `request_id` scope down to
individual messages, and reconnect a chat send's full trajectory to
the SendMessage RPC that produced it.

After the h2c fix in Phase 1, each stream has its own `request_id`.
That solves the "one ID across thousands of unrelated streams" bug,
but a busy chat stream still emits many log lines under one ID. And
the broadcast path (`SendMessage` → fan out to N subscriber streams)
was severing the correlation chain entirely: `broadcastMessage`
called `logging.Default()`, so the fan-out lines had no `request_id`
at all.

The fix uses entity IDs as the in-stream sub-ID — instead of a
synthetic `stream_event_seq`, every per-event log line carries the
relevant `message_id` (or `community_event_id`, `gear_id`, etc. for
other streams). Querying `request_id=<stream> message_id=<X>`
filters to one message's processing path. No new ID concept, no
schema churn.

**Tasks:**
- [x] Threaded `ctx` through the `MessageBroadcaster` interface
  (`server/chat/system_messages.go`) and through
  `chat.Service.BroadcastMessage` / `broadcastMessage` in
  `server/services/chat/`. All four internal call sites — SendMessage
  (`messages.go`), AddReaction / RemoveReaction (`reactions.go`),
  mention handler (`mention_handler.go`) — now pass the request
  handler's `ctx` into the broadcast. Three system-message broadcast
  paths in `system_messages.go` updated similarly.
- [x] `broadcastMessage` switched from `logging.Default()` to
  `logging.LoggerWithContext(ctx)`, so the originating RPC's
  `request_id` flows into the broadcast log lines. Added
  `message_id` and per-recipient `recipient_user_id` to broadcast
  logs.
- [x] Tagged per-event log lines inside `StreamMessages` with
  `message_id` (delivered backlog message, delivered live message,
  send-error). Lets you slice within a busy stream's `request_id`
  scope to one message's delivery.
- [x] Updated `docs/server/observability.md` with a "Streams and
  per-event correlation" subsection that documents the convention.
- [x] Added `server/integration_tests/chat_broadcast_request_id_test.go`,
  which subscribes a borrower to `StreamMessages`, then has the
  lender SendMessage with a known `X-Request-ID`, and asserts the
  broadcast log line carries that exact ID alongside the broadcast
  `message_id`. This is the hermetic regression assertion for the
  ctx-threading change.
- [x] Tests for `server/chat/` (mock broadcaster signature) and
  `server/services/chat/` (broadcastMessage call sites) updated to
  match the new signature; full suite green.

**Validates:** A `SendMessage` call's `request_id` now joins to: the
DB write, the broadcast fan-out, and (for any subscriber whose
stream emits per-event lines tagged with the same `message_id`) the
delivery itself.

**Also landed:** the same ctx-threading pattern was applied to the
community event stream in `server/services/community/streaming.go`:

- `BroadcastCommunityEvent` now takes `ctx` and uses
  `logging.LoggerWithContext(ctx)`, so the originating RPC's
  `request_id` reaches the broadcast log lines (e.g. ShareGear →
  GEAR_SHARED broadcast → log carries the ShareGear `request_id`).
  Broadcast lines also tag `event_id` + `event_type` for per-event
  slicing.
- `registerEventStream` / `unregisterEventStream` (community) and
  `registerStream` / `unregisterStream` (chat) all take `ctx` so
  stream-lifecycle log lines match the stream's `request_id`.
- Catch-up and live send-error logs in `StreamCommunityEvents` gained
  `event_id`.
- New regression test
  `server/integration_tests/community_broadcast_request_id_test.go`
  asserts a ShareGear under a known `X-Request-ID` produces a
  `broadcasting community event` log line carrying that ID and
  `event_type=GEAR_SHARED`.

**Out of scope (future):** Generation streams (`StreamGenCommunity`,
`StreamGenGear`, etc.) are 1:1 user sessions with no fan-out, so the
broadcaster pattern doesn't apply — the stream's own `request_id`
already scopes everything. Async log lines outside streaming
(waitlist email goroutines, web template render errors) still use
`logging.Default()` and could be tightened in a separate cleanup.

## Related Issues

- #1581 — RPC traffic audit. This bug blocks the analysis there;
  fixing #1799 unblocks the join between client `RPC_AUDIT` lines and
  server `db_queries` / `response_bytes` fields. The findings doc
  referenced in the issue body
  (`docs/ai/rpc_audit/findings-2026-05-09.md`) is upcoming work in
  #1581 and not present in the tree yet.
