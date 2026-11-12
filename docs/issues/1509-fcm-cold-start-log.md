# Issue #1509: FCMService: expired-token-at-cold-start logs SEVERE for an expected, recoverable state

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1509
**Filed:** 2026-04-28 | **Last activity:** 2026-04-30
**Labels:** none (triaged Bug / P3) | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

On a cold start with a stale-but-refreshable access token,
`FCMService._registerDeviceToken` makes its `RegisterDeviceToken` RPC with the
expired token already on disk. The server returns `unauthenticated`, the
`ConnectException` propagates up two `try` blocks, and the client logs three
SEVERE lines (`Failed to register device token: ... token is expired`,
`EXCEPTION CAUGHT BY FCMSERVICE`, and `Device token registration failed
(message handlers still active)`).

Functionally the app is fine — `_setupMessageHandlers()` runs unconditionally
first, and `FirebaseMessaging.onTokenRefresh` will re-call
`_registerDeviceToken()` once Firebase rotates the FCM token. The user-visible
behaviour is correct.

The defect is the log severity. SEVERE is what alerts route on; "expected,
recoverable transient state" is not what SEVERE should mean. The issue body
quotes the project convention: don't downgrade the log to silence alerts —
fix the upstream race so the log doesn't fire in the happy path.

## Context from Comments

Two comments:

1. **Triage (2026-04-28, maintainer)** — set Type=Bug, Priority=P3.
   Rationale: "log noise: FCMService logs SEVERE for an expected, recoverable
   cold-start state. Fix is one-liner." The "one-liner" framing is
   aspirational — see Recommendation; the cleanest fix is a small refactor,
   not a single line, but it is still a focused, mechanical change.
2. **`@claude` invocation (2026-04-30, maintainer)** — kicks off this run.

The issue body itself ranks the three solution options in order of
preference: defer-until-fresh > retry-once > demote-log. It explicitly
rejects "demote-log" as the primary fix because it masks the upstream race.

## Potential Solutions

### Option A: Use `RpcUtils.executeRpc` for the registration call

- **Approach:** `RpcUtils.executeRpc` is the codebase-wide pattern for
  authenticated RPCs (used by `ChatService`, `CommunityService`,
  `GearService`, `LoanService`, etc.). It catches `unauthenticated`, calls
  the globally-configured token-refresh handler
  (`AuthStateNotifier.refreshAccessToken` via `RpcUtils.configureTokenRefresh`
  in `auth_state.dart:99`), and retries the RPC once. On the happy path with
  a stale token: first attempt → `WARNING: RPC ... failed: unauthenticated`,
  then `INFO: Token refreshed, retrying RPC`, then success. No SEVERE.

  Concretely:
  - Add `errorHandler` and `onUnauthenticated` constructor params to
    `FCMService` (mirroring `ChatService`).
  - Wrap the `client.registerDeviceToken(...)` call inside
    `RpcUtils.executeRpc(...)`.
  - Use `RpcUtils.buildHeaders(_getAccessToken)` instead of building headers
    by hand — keeps `X-Request-ID` correlation consistent with every other
    service.
  - Tighten the surrounding `try`/`catch`: `executeRpc` already converts
    `ConnectException` into `ServiceException` and logs the warning. The
    outer catch in `_registerDeviceToken` then logs SEVERE only if the
    refresh-and-retry path also failed (the genuine failure case the
    acceptance criteria still wants to alert on).
  - Update `fcmServiceProvider` in `app_providers.dart` to pass the new
    deps (`rpcErrorHandlerProvider`, optional `onUnauthenticated`).

- **Pros:** Uses the existing, well-tested cross-cutting pattern. No
  FCM-specific retry logic to maintain. Picks up future improvements to
  `executeRpc` for free. Genuine failures (network, 5xx, refresh-fails)
  still log SEVERE, satisfying the acceptance criterion. Consistent with
  every other authenticated client call in the app.
- **Cons:** Public-interface change to the `FCMService` constructor.
  `_registerDeviceToken` will still emit a single WARNING on cold start
  before the refresh fires — that's correct (the failure happened, it was
  just recoverable) but it's worth confirming with the reviewer that
  WARNING at cold start is acceptable.
- **Effort:** Small. Three files: `fcm_service.dart`, `app_providers.dart`,
  `fcm_service_test.dart`.

### Option B: Defer registration until auth state is fresh

- **Approach:** Don't call `_registerDeviceToken()` from `initialize()` at
  all. Instead, listen to `authStateProvider` and register the device token
  the first time the access token transitions to a non-stale value (either
  the initially-loaded token if `_parseJwtExpiry` shows it's not near
  expiry, or the refreshed token after the first proactive refresh).

- **Pros:** Eliminates the race entirely. No WARNING at cold start either —
  the first registration attempt always sees a fresh token. Most aligned
  with the issue body's stated preference order.
- **Cons:** Considerably more invasive. Requires:
  - A new state machine inside `FCMService` (or a dedicated `Notifier`) to
    track "have we registered with the server yet?" across the auth
    lifecycle.
  - Wiring `FCMService` to a Riverpod `ref` (it currently only takes a
    closure for `getAccessToken`) or moving the registration trigger out
    to `main.dart` and calling `fcmService.registerNow()` from a listener.
  - Handling the cross-cut between Firebase's `onTokenRefresh` (FCM token
    rotation) and the auth state listener (access-token freshness) so we
    don't double-register or skip a registration.
  - More test surface (lifecycle ordering, late-arriving auth, simultaneous
    FCM-token + auth refresh).
- **Effort:** Medium. Touches `fcm_service.dart`, `app_providers.dart`,
  `main.dart`, and meaningfully expands `fcm_service_test.dart`.

### Option C: Demote `unauthenticated` at cold start to debug/info

- **Approach:** In the `on connect.ConnectException` block at
  `fcm_service.dart:186-188`, check `e.code == Code.unauthenticated` and
  log at `info`/`debug`; let other codes continue to log SEVERE. Outer
  catch at `fcm_service.dart:74-78` similarly inspects the exception type.
- **Pros:** One-block change. No constructor or DI changes. No new tests
  needed beyond updating existing assertions.
- **Cons:** Doesn't fix the race — registration still fails on cold start
  and only retries when Firebase happens to rotate the FCM token, which
  may be hours or days later. Until then the device is unregistered for
  push. The issue body explicitly calls this out as "stopgap only" and
  the project convention is to fix the upstream bug rather than mute the
  log.
- **Effort:** Trivial.

## Recommendation

**Option A.** It removes the race (refresh-and-retry succeeds before any
SEVERE fires), it follows the codebase's established cross-cutting pattern
for every other authenticated RPC, and it preserves the desired alert
behaviour for genuine failures. The Option B win — zero log noise instead of
one WARNING — isn't worth the lifecycle plumbing required, and the WARNING
on cold start is itself useful diagnostic context if a future regression
breaks the refresh path.

If the reviewer wants the strict "no log of any level on the cold-start
happy path" outcome from Option B, this plan should be rewritten before
implementation — Option A and Option B share little code.

## Open Questions

- **Is one WARNING line on the cold-start happy path acceptable?** —
  Recommendation: yes. WARNING is the standard `executeRpc` level for
  recoverable RPC failures across the app (see `rpc_utils.dart:109`); this
  matches every other service. The acceptance criteria in the issue only
  forbids SEVERE; WARNING is consistent with project convention. If the
  reviewer disagrees, escalate to Option B.
- **Should `FCMService.unregisterDevice` (line 376) also move to
  `executeRpc`?** — Recommendation: yes, in the same PR. It's the
  symmetric call, currently uses the same hand-rolled headers + `try`/
  `catch` pattern, and inherits the same staleness risk on app exit. The
  existing logs are already at WARNING (not SEVERE) so it's not part of
  the bug, but the consistency is worth taking while we're touching the
  file.
- **Should `_onTokenRefresh` (line 196) also benefit from the retry?** —
  Recommendation: implicit yes. It calls `_registerDeviceToken()`, so once
  that uses `executeRpc`, the FCM-token-rotation path inherits the same
  refresh-and-retry behaviour for free.
- **Does `FCMService` need to take an `onUnauthenticated` callback?** —
  Recommendation: pass `null`. The other services use it to navigate the
  user to the login screen on a true logout; FCM registration silently
  failing after a real logout is fine — the user isn't authenticated, so
  the token isn't needed. Reviewer should confirm.

## Implementation Plan

### Phase 1: Wrap registration in `executeRpc`

**Goal:** Cold start with a stale-but-refreshable token completes
registration via the centralized refresh+retry path and emits no SEVERE.

**Tasks:**
- [ ] `app/lib/services/fcm_service.dart` — extend the `FCMService`
  constructor with `RpcErrorHandler? errorHandler` and
  `Future<void> Function()? onUnauthenticated` (mirror
  `ChatService` constructor at `chat_service.dart:21-29`). Default
  `errorHandler` to `RpcErrorHandler()` for tests that don't pass one.
- [ ] `app/lib/services/fcm_service.dart` — replace the manual header
  construction inside `_registerDeviceToken` (lines 178-184) with
  `RpcUtils.buildHeaders(_getAccessToken, onUnauthenticated:
  _onUnauthenticated)`. Wrap the `client.registerDeviceToken(...)` call
  in `RpcUtils.executeRpc(() async { ... }, _errorHandler,
  onUnauthenticated: _onUnauthenticated, operationName:
  'RegisterDeviceToken')`.
- [ ] `app/lib/services/fcm_service.dart` — drop the `on
  ConnectException` arm at lines 186-188 (now unreachable —
  `executeRpc` swallows it). Keep the outer `catch` at lines 74-78,
  but its log is now correct: it only fires on real failure
  (`ServiceException` after refresh+retry both failed, network error,
  unknown exception). Tighten the message to make this clear, e.g.
  `'Device token registration failed after retry (message handlers
  still active): $e'`.
- [ ] `app/lib/services/fcm_service.dart` — apply the same pattern to
  `unregisterDevice` (line 376) for consistency. Existing logs are
  already WARNING, so behaviour is unchanged; this is a tidy-up.
- [ ] `app/lib/services/providers/app_providers.dart` — update
  `fcmServiceProvider` to pass `errorHandler:
  ref.watch(rpcErrorHandlerProvider)` and leave `onUnauthenticated`
  unset (`null`) — see Open Questions.

**Validates:** Manual cold start with a deliberately-stale token
(`flutter pub run` after manually corrupting the stored `exp` claim, or
by waiting past expiry) emits no SEVERE log from `FCMService`. The new
WARNING from `executeRpc` is followed by INFO `Token refreshed` and the
RPC succeeds.

### Phase 2: Tests

**Goal:** Lock the new behaviour in so a future refactor can't reintroduce
the SEVERE-on-recoverable-state regression.

**Test-seam note (audit fix):** The existing `_StubTransport extends Fake
implements connect.Transport` in `fcm_service_test.dart` is a no-op fake
that can't simulate RPC responses — Connect transports operate at the
wire level. To unit-test the new `executeRpc` path we need a controllable
seam at the `DeviceServiceClient` layer. Take the smallest cut:

1. Add an optional `DeviceServiceClient Function(connect.Transport)?
   clientFactory` constructor parameter to `FCMService`, defaulting to
   `DeviceServiceClient.new`. Production code is unchanged; tests inject
   a factory that returns a fake client.
2. Add a `_FakeDeviceServiceClient extends Fake implements
   DeviceServiceClient` test double that lets tests script the response
   sequence for `registerDeviceToken` (first call throws
   `ConnectException(unauthenticated)`, second call returns a
   `RegisterDeviceTokenResponse`).
3. Configure `RpcUtils.configureTokenRefresh` in the test `setUp` with a
   stub handler that returns `true` (so `executeRpc`'s retry path runs).
4. Capture log output via `Logger.root.onRecord` in `setUp`/`tearDown`
   (the standard `package:logging` pattern — `ObservableLogger` writes
   to the same `Logger` tree, so SEVERE/WARNING records are observable
   by `Level`). Pattern reference: `rpc_utils_test.dart` and
   `rpc_error_handler_test.dart`.

**Tasks:**
- [ ] Extend `FCMService` constructor with the optional `clientFactory`
  parameter described above.
- [ ] Add `_FakeDeviceServiceClient` to `fcm_service_test.dart` with a
  scriptable `registerDeviceToken` queue (or a counter + branch on call
  index).
- [ ] Add `setUp` log capture: `final logs = <LogRecord>[]; final sub =
  Logger.root.onRecord.listen(logs.add); addTearDown(sub.cancel);`.
- [ ] Add test: `_registerDeviceToken` succeeds after refresh+retry when
  the first call throws `ConnectException(code: unauthenticated)` and the
  second succeeds. Assert `logs.where((r) => r.level == Level.SEVERE)` is
  empty for the FCMService logger; assert at least one `Level.WARNING`
  record from `RpcUtils` is present.
- [ ] Add test: `_registerDeviceToken` logs SEVERE once and rethrows when
  the call returns a non-`unauthenticated` `ConnectException` (e.g.
  `unavailable`).
- [ ] Add test: `_registerDeviceToken` logs SEVERE once and rethrows when
  refresh+retry both return `unauthenticated` (token refresh failed —
  configure the `configureTokenRefresh` handler to return `false`).
- [ ] Leave the placeholder `'documents the reordering fix for code
  review'` test (lines 216-223) untouched — it documents an unrelated
  invariant (`_setupMessageHandlers()` runs first) and removing it would
  silently drop that documentation.
- [ ] Run `cd app && flutter test --reporter github
  test/services/fcm_service_test.dart` until green.

**Validates:** Regression tests guard the no-SEVERE-on-recoverable-state
behaviour and the still-SEVERE-on-real-failure behaviour.

### Phase 3: Build & smoke

**Goal:** Confirm no Dart-side regressions.

**Tasks:**
- [ ] `cd app && flutter analyze` — clean.
- [ ] `cd app && flutter test --reporter github` — full suite green.
- [ ] No server-side changes; no `npm run build` needed unless the
  `app/pubspec.lock` change in the working tree warrants it.

**Validates:** Lint, type-check, and tests pass.

## Related Issues

- **#1004** (`docs/issues/1004-fcm-log-levels.md`) — server-side analogue:
  spurious FCM ERROR logs from the server's Firebase Admin SDK after Cloud
  Run idle. Same shape of problem (transient credential failure logged
  too loudly), opposite end of the wire. The Phase-1 retry-on-auth-error
  pattern there is the server-side cousin of this plan's reuse of
  `RpcUtils.executeRpc`.

---

## Audit Notes

**Audited against:** `docs/client/architecture.md`,
`docs/client/observability.md`, `docs/client/testing.md`.

**Findings:**

1. **Architecture compliance (OK):** `FCMService` lives in
   `app/lib/services/` and is consumed via Riverpod
   (`fcmServiceProvider`). It's a service-layer object, not a
   ViewModel — no `Notifier` / `ref.mounted` concerns apply. The plan
   keeps the existing layering: `FCMService` continues to call the
   generated Connect client directly; viewmodels and repositories
   are not involved.
2. **Observability semantics (OK):** `_log.error()` maps to
   `Level.SEVERE` (`logger.dart:156-162`), which records a breadcrumb
   and is reportable via the crash-reporter integration. The plan's
   recommendation (let `executeRpc` log `WARNING` for the recoverable
   first attempt; only call `_log.error()` after refresh+retry both
   fail) preserves the alerting contract — genuine failures still
   reach Crashlytics.
3. **Testing-layer compliance (addressed inline):** the original Phase-2
   plan glossed over the test seam. The Connect `Transport` interface
   is too low to fake usefully; the `Fake` extends pattern at the
   transport level only works because the existing tests don't call
   `_registerDeviceToken`. Plan now adds an explicit
   `clientFactory` constructor parameter to inject a fake
   `DeviceServiceClient`, plus a `Logger.root.onRecord` capture
   pattern modelled on `rpc_utils_test.dart` and
   `rpc_error_handler_test.dart`. This keeps the change small and
   matches the existing testability conventions.
4. **No i18n / a11y impact:** `FCMService` produces no user-facing
   strings; the only behavioural change is log severity. ARB files
   and accessibility labels are untouched.
5. **No caching impact:** Repository layer and Stash caches are not
   involved.
6. **Cross-cutting consistency (OK):** Reusing `RpcUtils.executeRpc`
   aligns FCMService with every other authenticated service
   (`ChatService`, `CommunityService`, etc.) and inherits the
   `X-Request-ID` correlation header via `RpcUtils.buildHeaders` — a
   side benefit the original plan didn't call out, now reflected in
   Phase 1.
