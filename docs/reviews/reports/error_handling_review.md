# Error Handling Review - 2026-04-28

## Executive Summary

Significant resilience improvements have landed since the last review (2026-04-24): HTTP panic-recovery middleware is now in place at the outermost handler chain, a `logging.GoSafe` helper wraps fire-and-forget goroutines in `server/services/`, init-time panics in `server/email/mailgun.go` and `server/services/web/service.go` have been replaced with returned errors, and conversation/inbox message streams now expose `isStreamDisconnected` and exponential-backoff reconnection in the Flutter ViewModels. Three of the four previous Critical findings are remediated; the fourth (goroutines without recover) is largely remediated within `server/services/` but a couple of holdouts remain (`server/services/chat/messages.go:148` and `server/services/streaming/sender.go:42` — the latter has bespoke recovery — plus all `server/ai/provider_*_streaming.go` goroutines).

The two longstanding High-severity patterns are unchanged or worse: bare `connect.NewError(connect.CodeInternal, err)` sites grew from 205 to 481 (counting all CodeInternal call-sites; ~206 with literal `err` — same magnitude), and Flutter ViewModels assigning raw `e.toString()` to user-visible `errorMessage` is at 67 sites across 28 files (was 64). No `RpcErrorHandler.extractMessage` helper or `connecterr.Internal` wrapper has been introduced. The `GetUserLocationFallback` swallowed-error pattern has expanded from 15 to 22 sites. Trend: **Improving** on resilience; **Stable-to-declining** on raw-error-message hygiene.

## Changes Since Last Review

**Previous Review Date**: 2026-04-24

**Remediated:**
- HTTP panic recovery middleware is wired as the outermost wrapper — `server/main.go:979` calls `middleware.PanicRecovery(logger)`, implementation in `server/middleware/recovery.go` with tests.
- Init-time panics removed: `server/email/mailgun.go:65-106` and `server/services/web/service.go:60-83` now return `(*T, error)`.
- `logging.GoSafe(ctx, name, fn)` helper added (`server/logging/logger.go:266-283`) and applied to `server/services/waitlist/service.go:100,118`, `server/services/fanout/fanout.go:159,175`, `server/services/feed/feed_nudges.go:48,71,151,185`, `server/services/experience/gen_ai_streaming.go:213,234,438,458,674,693`, `server/services/gear/gen_ai_streaming.go:279,308`, `server/services/request/gen_streaming.go:142,162,311,331`.
- Stream disconnection surfaced to UI with reconnection: `app/lib/presentation/viewmodels/conversation_messages_notifier.dart:154-225` and `app/lib/presentation/viewmodels/inbox_view_model.dart:313-360` now track `isStreamDisconnected`, log severe, and run exponential-backoff reconnection until a max-attempts cap.
- `server/services/streaming/sender.go:42-54` has inline `recover()` with explicit rationale (must signal `s.done` to unblock `Close()`).

**Unaddressed:**
- Bare `connect.NewError(connect.CodeInternal, err)` — High. Roughly the same magnitude (~206 sites passing raw `err`); no `connecterr.Internal` helper introduced. Representative: `server/services/experience/needs.go:75,83,134,210,219`, `server/services/chat/messages.go:50`.
- Raw `e.toString()` in user-visible `errorMessage` — High. 67 sites across 28 ViewModel files (was 64). No `RpcErrorHandler.extractMessage` helper. Representative: `event_modal_view_model.dart` (16 sites), `request_owner_view_model.dart` (multi), `experience_view_model.dart`.
- `GetUserLocationFallback` errors silently discarded — Medium. Now 22 sites (was 15). Streaming siblings continue to clone the pattern. Representative: `server/services/experience/gen_ai_streaming.go:289,292,509,512,760,763`.
- `app/lib/data/repositories/user_repository.dart:51-53,78-80` still has bare `catch (e) {}` for media-load failures with no logging.
- `server/storage/protosql_schema.go:345-384` schema initialization still runs CREATE/ALTER/INDEX outside any transaction.
- `server/notifications/service.go:107-148` still has no circuit breaker; failures iterate every device.
- `server/services/chat/messages.go:148` launches a bare `go func()` for `notifyParticipants` without `recover()`.
- `server/ai/provider_anthropic_streaming.go`, `server/ai/provider_openai_streaming.go`, `server/ai/provider_gemini_streaming.go` — many bare `go func()` goroutines without `recover()` (relying solely on `defer close(channel)`).

**New:**
- None — net new findings are absent; the AI-provider streaming goroutines were not flagged previously but were already present and behind the goroutine-recovery rule.

**Improvements:**
- `server/middleware/recovery.go` includes test coverage (`recovery_test.go`).
- `logging.GoSafe` includes test coverage (`logger_test.go:522-590`).
- Stream reconnection in inbox/conversation also includes max-attempts handling and explicit "stream disconnected" UI signaling.

**Trend**: Improving. Three of four prior Critical findings closed; one mostly closed. The two persistent Highs (`CodeInternal` raw-err leak, ViewModel raw-toString) are durable patterns and carry the most user-facing/security risk now.

---

## Critical Issues

### 1. Bare `go func()` in chat message send path
**Location**: `server/services/chat/messages.go:148`.
**Description**: `SendMessage` launches `go func() { s.notifyParticipants(...); notifDone <- struct{}{} }()` without `recover()`. A panic in `notifyParticipants` (e.g., nil-pointer through a notification provider) crashes the server. The HTTP-level `PanicRecovery` middleware does not see panics from goroutines that outlive the request.
**Impact**: Server-wide crash on any unexpected panic in the notification fan-out path. This is a hot RPC.
**Recommendation**: Migrate to `logging.GoSafe(ctx, "chat-notify-participants", func() { ... })`. If `notifDone` must be signaled even on panic (test-coordination channel), use the `streaming/sender.go:42-54` inline-recovery shape and explicitly send to `notifDone` in the recovery branch.

### 2. AI provider streaming goroutines lack `recover()`
**Location**: `server/ai/provider_anthropic_streaming.go:101,228,299,358,426,500,570,644,718`; `server/ai/provider_openai_streaming.go:103,226,299,383,485,568,642,731,809`; `server/ai/provider_gemini_streaming.go:58,152,199,266` (and additional sites in the same files).
**Description**: Streaming-call goroutines defer channel closes but do not `recover()`. A panic from the third-party SDK (e.g., a malformed response struct, deserialization edge case) crashes the server.
**Impact**: Production crashes triggered by upstream model output. AI streaming is increasingly load-bearing for Gen* RPCs.
**Recommendation**: Wrap each in `logging.GoSafe` or, when panic recovery must coordinate with channel closes (mirroring `streaming/sender.go`), add inline `defer recover()` that pushes a synthetic error to `final` and logs the stack. Add a custom lint rule (e.g., `forbidigo` regex) that flags `^\s*go func\b` outside of allowlisted patterns.

---

## High Priority

### 3. Raw exception messages exposed to users in ViewModels (67 sites, 28 files)
**Location**: 28 files in `app/lib/presentation/viewmodels/`. Representative: `event_modal_view_model.dart:215,319,355,387,470,499,564,590,710,782,861,919`, `request_owner_view_model.dart` (multi), `experience_view_model.dart` (multi), `gear_view_model.dart`, `community_edit_view_model.dart`, `loan_borrower_view_model.dart`, `giveaway_receiver_view_model.dart`, `search_view_model.dart`, `inbox_view_model.dart`.
**Description**: `errorMessage: 'Failed to X: ${e.toString()}'` and `errorMessage: e.toString()`. When `e` is a `ConnectException`, Postgres error string, or `FormatException`, users see SQL fragments and stack-trace-shaped output.
**Impact**: Poor UX; potential PII / internal-state leakage in error strings.
**Recommendation**: Add `RpcErrorHandler.extractMessage(e, fallback)` returning the user-safe `e.message` for `ServiceException` / known typed exceptions and a localized generic string otherwise. Migrate all 67 sites and add a CI grep that fails on `errorMessage:.*\$\{e\.toString\(\)\}` outside of allowlisted dev-only paths.

### 4. Bare `connect.NewError(CodeInternal, err)` leaks raw internals (~206 sites)
**Location**: ~206 sites across `server/services/`. Representative: `server/services/experience/needs.go:75,83,134,210,219`, `server/services/chat/messages.go:50`, `server/services/gear/service.go`, `server/services/community/invitations.go`, `server/services/device/service.go`, `server/services/request/gen.go`, all `server/services/*/gen_streaming.go`.
**Description**: Raw errors — including Postgres constraint violations, JSON decoder errors, third-party API responses — are passed verbatim to clients. The Flutter `RpcErrorHandler` reports `Code.internal` errors to crash analytics, polluting signal with low-value SQL strings.
**Impact**: Client message leaks internal detail (security/UX) and floods crash reporting with noisy server-internal text.
**Recommendation**: Add a `connecterr.Internal(ctx, logger, "operation", err)` helper that logs the wrapped error server-side and returns a generic-message `CodeInternal` to the client. Migrate incrementally; add a ruleguard/revive rule forbidding bare `connect.NewError(connect.CodeInternal, err)` outside the helper.

---

## Medium Priority

### 5. `GetUserLocationFallback` errors silently discarded (22 sites)
**Location**: `server/services/experience/gen_ai.go:291,295`, `server/services/experience/gen_ai_streaming.go:289,292,509,512,760,763`, `server/services/gear/gen_ai.go:152,156,263,442`, `server/services/gear/gen_ai_streaming.go:182,348,351,437`, `server/services/request/gen.go:124,128,256`, `server/services/request/gen_streaming.go:201,204,374`.
**Description**: 22 occurrences of `_, geocodedLocation, _ = locationlib.GetUserLocationFallback(...)`. Storage failures on this hot path are invisible.
**Recommendation**: Either log at `Warn` when the error is non-nil or introduce a `GetUserLocationFallbackLogged` variant that never returns an error (logs internally).

### 6. Schema initialization without transaction
**Location**: `server/storage/protosql_schema.go:345-384`.
**Description**: CREATE TABLE / ALTER / CREATE INDEX run as separate `db.Exec` calls. A mid-sequence failure leaves the schema half-applied.
**Recommendation**: Wrap init in BEGIN/COMMIT or introduce a `schema_version` table with checksummed migrations.

### 7. Notification provider lacks circuit breaker
**Location**: `server/notifications/service.go:107-148`.
**Description**: During a broad FCM/APNs outage, every device for every notification is attempted.
**Recommendation**: Track consecutive non-token-specific failures per call; after N failures, log and abort remaining sends.

### 8. `user_repository.dart` silent media catches
**Location**: `app/lib/data/repositories/user_repository.dart:51-53,78-80`.
**Description**: `catch (e) {}` discards media-load failures with no logging. If presigned-URL signing breaks, no signal reaches logs.
**Recommendation**: Add `_log.warning('Failed to load user media $mediaId: $e')` inside each catch. Keep graceful-degradation behavior.

---

## Low Priority

### 9. No general retry for transient errors
The Flutter clients have no shared transient-error retry; only FCM has provider-side single-shot auth retry.

### 10. Hardcoded English error messages
Most user-facing error strings are not localized.

### 11. `streaming/sender.go` recovery rationale duplicates structure
Inline recovery is correct but the comment-then-pattern shape could be promoted to a helper if more channel-coordinated goroutines emerge.

---

## Positive Findings

1. `server/middleware/recovery.go` with test coverage shipped.
2. `logging.GoSafe` helper with test coverage shipped and adopted across 14+ service-tree call sites.
3. Mailgun and web init now propagate errors (`server/email/mailgun.go:65-106`, `server/services/web/service.go:60-83`).
4. Streaming sender has bespoke inline panic recovery with documented rationale (`server/services/streaming/sender.go:42-54`).
5. Conversation/inbox message streams expose `isStreamDisconnected` with exponential-backoff reconnection and max-attempts cap.
6. Centralized Flutter error handling (`rpc_error_handler.dart`) and `ServiceException` continue to be used at the service-call seam.
7. PII masking applied consistently (`logging.MaskEmail`, `logging.MaskToken`).
8. AdminService remains gated behind `devMode` (carry-over from prior review).
9. Request lifecycle goroutines (`server/services/request/lifecycle.go:153-194`) include the canonical recover+log pattern.

---

## Recommendations

**Immediate (Critical):**
1. Migrate `server/services/chat/messages.go:148` `notifyParticipants` goroutine to `logging.GoSafe` or coordinated-recover variant.
2. Add panic recovery to all `server/ai/provider_*_streaming.go` goroutines (or migrate to a coordinated helper that closes their channels on panic).
3. Add a `forbidigo`/ruleguard rule that flags raw `go func\b` outside an allowlist.

**Short-term (High):**
4. Add `RpcErrorHandler.extractMessage` and migrate the 67 ViewModel `e.toString()` sites; gate with CI grep.
5. Add `connecterr.Internal(ctx, logger, op, err)` and migrate ~206 raw-err `CodeInternal` sites; gate with a custom rule.

**Medium:**
6. Replace silent `GetUserLocationFallback` discards with a logging variant.
7. Wrap schema init in a transaction or move to checksummed migrations.
8. Add notification-provider circuit breaker.
9. Add logging to `user_repository.dart` silent media catches.

---

## Issues to File

### [Critical] Add panic recovery to chat notification and AI streaming goroutines
- **Fingerprint**: `review:error_handling:remaining-bare-goroutines-recover`
- **Type**: Task
- **Why it matters**: After this cycle's GoSafe rollout, two hot paths still launch bare `go func()` without recover: `chat/messages.go:148` (every SendMessage call) and ~25 goroutines across `server/ai/provider_*_streaming.go` (every streaming Gen* call). A panic in any of these — third-party SDK regression, unexpected nil from a notification provider — crashes the server. The new `middleware.PanicRecovery` cannot help because these goroutines outlive their request.
- **Locations**: `server/services/chat/messages.go:148`, `server/ai/provider_anthropic_streaming.go:101,228,299,358,426,500,570,644,718`, `server/ai/provider_openai_streaming.go:103,226,299,383,485,568,642,731,809`, `server/ai/provider_gemini_streaming.go:58,152,199,266`
- **Recommendation**: Migrate the chat-messages call to `logging.GoSafe`. For AI streaming goroutines (which must signal `final`/`fields` channels on panic), follow the `server/services/streaming/sender.go:42-54` inline-recover shape. Add a `forbidigo` rule that flags raw `go func\b` outside `server/logging/logger.go` and an explicit allowlist. This is the last gap in the goroutine-recovery story.
- **Suggested labels**: `server,reliability`

### [High] Wrap `connect.NewError(CodeInternal, err)` to stop leaking raw internals
- **Fingerprint**: `review:error_handling:bare-err-codeinternal-connect`
- **Type**: Bug
- **Why it matters**: ~206 call sites pass raw errors (Postgres constraint strings, decoder errors, third-party API bodies) directly to clients via `Code.internal`. Flutter's `RpcErrorHandler` reports `Code.internal` to crash analytics, so these also pollute crash reporting with low-signal SQL/JSON text and may leak schema details. Pattern is unchanged since last review; no helper exists yet.
- **Locations**: ~206 sites across `server/services/`; representative: `server/services/experience/needs.go:75,83,134,210,219`, `server/services/chat/messages.go:50`, `server/services/gear/service.go`, `server/services/community/invitations.go`, `server/services/device/service.go`, `server/services/request/gen.go`, all `server/services/*/gen_streaming.go`
- **Recommendation**: Add `connecterr.Internal(ctx, logger, "operation", err)` that logs server-side and returns a generic-message `CodeInternal` to the client. Migrate sites incrementally, starting with streaming-gen and chat. Add a ruleguard/revive rule forbidding bare `connect.NewError(connect.CodeInternal, err)` outside the helper.
- **Suggested labels**: `server,security`

### [High] Sanitize raw exception messages shown to users in ViewModels
- **Fingerprint**: `review:error_handling:raw-exception-tostring-in-viewmodels`
- **Type**: Bug
- **Why it matters**: 67 sites across 28 ViewModel files assign `e.toString()` to user-visible `errorMessage`. When `e` is a `ConnectException`, Postgres error, or `FormatException`, users see leaky/technical text. Pattern grew slightly since last review (was 64); no helper introduced.
- **Locations**: 28 files in `app/lib/presentation/viewmodels/`; representative: `app/lib/presentation/viewmodels/event_modal_view_model.dart:215,319,355,387,470,499,564,590,710,782,861,919`, `app/lib/presentation/viewmodels/request_owner_view_model.dart` (multi), `app/lib/presentation/viewmodels/experience_view_model.dart` (multi), `app/lib/presentation/viewmodels/giveaway_receiver_view_model.dart`, `app/lib/presentation/viewmodels/loan_borrower_view_model.dart`
- **Recommendation**: Add `RpcErrorHandler.extractMessage(e, fallback)` returning `e.message` for `ServiceException` and a localized generic string otherwise. Migrate all 67 sites and enforce with a CI grep against `errorMessage:.*\$\{e\.toString\(\)\}`.
- **Suggested labels**: `client,ux`

### [Medium] Stop silently discarding `GetUserLocationFallback` errors
- **Fingerprint**: `review:error_handling:getuserlocationfallback-silent`
- **Type**: Bug
- **Why it matters**: 22 sites use `_, geocodedLocation, _ = locationlib.GetUserLocationFallback(...)` — up from 15 because streaming siblings keep cloning the pattern. Systemic storage issues on this hot path are invisible.
- **Locations**: `server/services/experience/gen_ai.go:291,295`, `server/services/experience/gen_ai_streaming.go:289,292,509,512,760,763`, `server/services/gear/gen_ai.go:152,156,263,442`, `server/services/gear/gen_ai_streaming.go:182,348,351,437`, `server/services/request/gen.go:124,128,256`, `server/services/request/gen_streaming.go:201,204,374`
- **Recommendation**: Either log at `Warn` when non-nil or introduce a `GetUserLocationFallbackLogged` variant that never returns an error and logs internally.
- **Suggested labels**: `server,observability`
