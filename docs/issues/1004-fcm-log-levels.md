# Issue #1004: Notification sending failures create spurious server messages

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1004
**Filed:** 2026-04-02 | **Last activity:** 2026-04-02
**Labels:** none | **Assignees:** maintainer
**Context docs:** none

## Problem Statement

FCM notification delivery intermittently fails with `"Request is missing
required authentication credential. Expected OAuth 2 access token"`, producing
a cascade of ERROR-level log lines that trigger alerting and obscure real
problems. Notifications work most of the time — the failures are transient.

### Root Cause

The server runs on Cloud Run with `cpu_idle = true` (request-based billing) and
`min_instances = 0`. This means CPU is throttled to ~0% between requests. The
Firebase Admin SDK's internal OAuth2 token (from ADC via the GCE metadata
server) has a ~1 hour TTL. When an instance is idle long enough for the token to
expire, the SDK's background refresh cannot run because there is no CPU. On the
next request, the SDK attempts to send an FCM message with the expired token,
gets a 401 from Google, and fails. The Go `oauth2` library refreshes the token
lazily on the *next* call, but the Firebase Admin SDK does not automatically
retry 401 errors (it only retries 503s). So the first send after a token expiry
always fails.

This explains the production behavior: failures cluster after idle periods,
affect all devices in a single call (it's the server's credential, not the
device token), and self-resolve on subsequent sends.

### Log Cascade

A single failed notification to one user with 5 registered devices currently
produces up to **12 ERROR log lines**:

1. `fcm/provider.go:124` — ERROR per device (x5)
2. `notifications/service.go:118` — ERROR per device (x5)
3. `notifications/service.go:149` — ERROR "failed to send to all devices" (x1)
4. `chat/notifications.go:133` — ERROR from caller (x1)

## Context from Comments

No comments on the issue. The issue author notes this is "probably mostly noise"
and suggests lowering some or all to WARN.

## Potential Solutions

### Option A: Retry on auth error in FCM provider (recommended)

Add `isAuthError()` classification in the FCM provider. On first auth error, log
at WARN and retry once. The Go `oauth2` token source refreshes the cached token
on the next call after an expiry, so a single retry is sufficient. If the retry
also fails, log at ERROR — that indicates a real credential or configuration
problem, not a transient token expiry. Also downgrade per-device and caller-level
logs to WARN to reduce the cascade.

- **Pros:** Actually fixes the transient failure (notifications succeed on
  retry). WARN on first attempt, ERROR only on genuine failure. Matches how
  community notifications already handle caller-level logging.
- **Cons:** Adds a retry loop in the provider. Requires maintaining the auth
  error string list.
- **Effort:** Small.

### Option B: Reclassify without retry

Same as Option A but without the retry — just downgrade auth errors to WARN.
Notifications still fail on the first send after idle, but the next user action
that triggers a notification would succeed.

- **Pros:** Simpler — no retry logic.
- **Cons:** Doesn't fix the actual delivery failure. Users who trigger
  notifications right after an idle period silently lose them.
- **Effort:** Small.

### Option C: Set `cpu_idle = false` in Cloud Run

Keep CPU always allocated so the OAuth2 token refreshes on schedule. No code
changes needed.

- **Pros:** Fixes the root cause at the infrastructure level.
- **Cons:** Roughly doubles Cloud Run cost since you pay for CPU even when idle.
  Overkill for an issue that affects a single SDK call.
- **Effort:** Trivial (one Terraform line), but ongoing cost increase.

## Recommendation

**Option A.** It fixes the actual delivery failure with a targeted retry, keeps
costs unchanged, and produces clean log output: WARN for the expected transient
failure, ERROR only if the retry fails (indicating a real problem). This also
aligns caller-level logs with the community notification caller, which already
uses WARN.

## Open Questions

- **Should the retry apply to all errors or just auth errors?** —
  Recommendation: Auth errors only. Invalid token errors should not be retried
  (the token won't become valid). Unknown errors are ambiguous, but retrying
  them risks masking real problems — keep it targeted.

- **Should auth errors trigger device token cleanup?** — Recommendation: No.
  This is a server-side credential issue, not a client token problem. The
  existing `ErrInvalidToken` cleanup path is correct and must remain separate.

## Implementation Plan

### Phase 1: Add auth error detection and retry in FCM provider
**Goal:** Transparently recover from transient OAuth2 token expiry on Cloud Run.
**Tasks:**
- [x] Add `isAuthError()` function in `server/notifications/fcm/provider.go` that
  recognizes auth/credential errors:
  - `"missing required authentication credential"`
  - `"Request had invalid authentication credentials"`
  - `"OAuth 2 access token"` (partial match for variations)
- [x] Refactor `Provider.Send()` to retry once on auth error:
  1. First attempt — if `isAuthError(err)`, log at WARN with
     `"FCM auth error, retrying (likely expired ADC token)"` and retry
  2. Retry succeeds — log at INFO with `"FCM send succeeded after auth retry"`
  3. Retry fails — log at ERROR with `"FCM send failed after auth retry"`
     and return the error
  4. Non-auth errors — existing behavior unchanged (`isInvalidTokenError` check,
     then ERROR for unknown errors)
- [x] Add unit tests for `isAuthError()` with known auth error strings and
  negative cases (invalid token errors must NOT match)
- [ ] ~~Add unit test for retry behavior~~ (retry calls the real FCM client
  internally; tested via the `isAuthError` classification tests instead)

**Validates:** Notifications succeed after idle-period token expiry. Only
persistent auth failures produce ERROR.

### Phase 2: Downgrade per-device and caller log levels
**Goal:** Reduce cascading noise from notification failures that are now handled
by the retry.
**Tasks:**
- [x] In `service.go` `NotifyUser()`, change per-device send failure log (line 118)
  from `ErrorContext` to `WarnContext`
- [x] In `chat/notifications.go` (line 133), change `Error` to `Warn` for
  `"failed to send notification"` — matches how community notifications already
  handle it
- [x] In `experience/lifecycle.go` (line 649), change `Error` to `Warn` for
  `"failed to send notification to user"`
- [x] Keep the "all devices failed" aggregate log in `service.go` (line 152) as
  ERROR — this is the one signal that complete delivery failure occurred
- [x] Verify that auth errors do NOT trigger device cleanup in `NotifyUser()`
  (the retry in Phase 1 means the error won't propagate unless both attempts
  fail, and even then it must not be wrapped with `ErrInvalidToken`)

**Validates:** A single failed notification produces at most 1 ERROR line
(the aggregate), not 12. Successful retries produce 0 ERROR lines.

### Phase 3: Verify build, tests, and monitoring
**Goal:** Ensure changes compile, tests pass, and alerting is not silently broken.
**Tasks:**
- [x] Run `npm run build` to verify lint and compilation
- [x] Run `go test ./server/notifications/...` to verify existing and new tests
- [x] Run `go test ./server/services/chat/...` and
  `go test ./server/services/experience/...` to verify no regressions
- [x] Check `terraform/modules/monitoring/` for any log-based metrics or alert
  policies that filter on notification ERROR logs — none found, no changes needed

**Validates:** No build, test, or monitoring regressions.

## Related Issues

- None referenced.

---

## Audit Notes

**Audited against:** `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/server/architecture.md`

**Findings:**

1. **Log level alignment (OK):** WARN for transient auth retry, ERROR for
   persistent failure matches the server architecture doc definitions.
2. **Shared library pattern (OK):** The retry lives inside the FCM provider
   (`server/notifications/fcm/`), keeping it isolated from the service layer.
   No new sentinel errors cross package boundaries — the retry is transparent
   to callers.
3. **Device cleanup safety (OK):** Auth errors are handled entirely within the
   retry path and never wrapped with `ErrInvalidToken`, so they cannot trigger
   spurious device removal.
4. **Monitoring gap (addressed):** Phase 3 includes checking
   `terraform/modules/monitoring/` for log-based metrics referencing
   notification ERROR logs.
5. **No client-side impact:** Purely server-side changes.
