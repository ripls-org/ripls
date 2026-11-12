# Issue #1254: FCM 'NotRegistered' errors not triggering device cleanup — noisy ERROR alerts

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1254
**Filed:** 2026-04-24 | **Last activity:** 2026-04-24
**Labels:** bug, P2 | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When FCM returns `NotRegistered` for a dead iOS device token, the server logs at
ERROR and does **not** clean up the dead token, so the same token fires on every
subsequent notification delivery. Over the last 3 days on dev, one stale token
(`dkTKbhlp...`) has produced 23 ERROR log lines, each triggering the
"Server Error Logged - server-dev" Cloud Monitoring alert.

The self-healing device cleanup in `server/notifications/service.go:131` only
runs when the FCM provider wraps its error with `ErrInvalidToken`. The wrap
path in `server/notifications/fcm/provider.go:99` is guarded by
`isInvalidTokenError(err)`, a substring matcher whose list covers
`"registration-token-not-registered"` (the hyphenated machine-readable form)
but **not** `"NotRegistered"` (the human-readable `message` field that
firebase-admin-go surfaces through `err.Error()` for this error code).

The same bug exists on prod. It hasn't fired P2 alerts there yet, but any iOS
user who deletes the app will produce a perpetual ERROR stream tied to their
stale token until we redeploy with a fix.

## Context from Comments

No comments on the issue (it was filed fresh during this investigation). The
issue body already contains the evidence, root cause, and proposed fix — this
plan refines the implementation strategy and surfaces a testability wrinkle
that isn't obvious from the body alone.

## Related prior work

Issue #1004 ([`docs/issues/1004-fcm-log-levels.md`](1004-fcm-log-levels.md))
reshaped the FCM log cascade in April 2026 — adding `isAuthError` retry,
downgrading per-device failures to WARN, and keeping only aggregate failures
and `isInvalidTokenError` misses at ERROR. That work left the ERROR branch at
`provider.go:107` as the last remaining "something is genuinely wrong"
signal. The current bug means one of the cases that should have flowed into
the WARN + cleanup path is instead surfacing on this ERROR branch. Keeping
ERROR loud is correct (per user feedback on #1184 / #1185 — see memory
`feedback_keep_error_logs.md`); the fix is to route `NotRegistered` to the
cleanup path where it belongs, not to silence the ERROR log.

## Root-cause trace

1. FCM HTTP v1 returns a response body shaped like:
   ```json
   {
     "error": {
       "status": "NOT_FOUND",
       "message": "NotRegistered",
       "details": [{
         "@type": "type.googleapis.com/google.firebase.fcm.v1.FcmError",
         "errorCode": "UNREGISTERED"
       }]
     }
   }
   ```
2. `firebase-admin-go`'s `NewFirebaseErrorOnePlatform`
   (`internal/errors.go:135`) populates `FirebaseError.String` from
   `error.message` and `FirebaseError.Ext["messagingErrorCode"]` from
   `error.details[].errorCode`.
3. `FirebaseError.Error()` returns `.String` → `"NotRegistered"`. That's
   what slog writes as `"error": "NotRegistered"` in the Cloud Run log.
4. `messaging.IsUnregistered(err)` correctly returns `true` because it checks
   `Ext["messagingErrorCode"] == "UNREGISTERED"`, not the `message` text.
5. Our current `isInvalidTokenError` only walks `errMsg := err.Error()` and
   substring-matches. `"NotRegistered"` isn't in its list, so the check
   returns `false` and the error falls through to the generic ERROR branch
   without the `ErrInvalidToken` wrap.

## Potential Solutions

### Option A: Add `"NotRegistered"` to the substring list

Single-line addition to `isInvalidTokenError`. Restores cleanup for this
specific message form.

- **Approach:** Append `"NotRegistered"` to the `invalidTokenErrors` slice in
  `provider.go:162`.
- **Pros:** Smallest possible diff. Existing `errors.New(...)` test fixtures
  keep working with one more test case added.
- **Cons:** Fragile — the next time FCM changes a human-readable `message`
  string, we'll be back here. The substring list is already 6 entries long
  across two forms of the same underlying codes.
- **Effort:** Small (one line + one test case).

### Option B: Replace substring matching with typed FCM predicates

Use `messaging.IsUnregistered`, `messaging.IsInvalidArgument`,
`messaging.IsSenderIDMismatch` from firebase-admin-go, which read the
machine-readable `errorCode` detail instead of the free-form `message`.

- **Approach:** Rewrite `isInvalidTokenError` to call the three typed
  predicates and return true if any match.
- **Pros:** Canonical, future-proof, survives FCM wording changes.
- **Cons:** The typed predicates only match `*internal.FirebaseError`
  instances, and `firebase.google.com/go/v4/internal` is unimportable from
  outside the firebase module. The existing unit tests construct fake errors
  with `errors.New("...")` and would all stop matching — we'd need integration
  tests or test helpers to cover the logic.
- **Effort:** Small in production code, medium in test rework.

### Option C: Hybrid — typed predicates first, substring fallback

Try `messaging.IsUnregistered` / `IsInvalidArgument` / `IsSenderIDMismatch`;
if none match, fall back to the existing substring list (with
`"NotRegistered"` added).

- **Approach:** Two-pass check in `isInvalidTokenError`. Typed predicates
  handle real FCM errors; the substring list handles any error surface that
  carries the text but not a typed `FirebaseError` (wrapped errors,
  test fixtures, future legacy paths).
- **Pros:** Robust to FCM wording drift (predicates win in production);
  keeps the existing unit tests working without importing
  unexportable packages; adds the immediate `NotRegistered` fix via the
  string list. Belt-and-suspenders is cheap for three extra predicate calls.
- **Cons:** Slightly more code than either pure option. Two mechanisms to
  reason about.
- **Effort:** Small.

## Recommendation

**Option A.** Decided on 2026-04-24: since the typed predicates can't be
unit-tested from our package (they only match `*internal.FirebaseError`
instances that we can't construct), adding the typed-predicate pass buys us
code that isn't exercised by any test — which is worse than skipping it. Add
`"NotRegistered"` to the substring list, add the mirror test case, ship it.
If FCM changes wording in the future we'll see the same symptom and can
revisit.

## Open Questions

- **Should `isAuthError` get the same treatment?** — Recommendation: not in
  this issue. `isAuthError` matches on OAuth2 token-expiry messages that are
  already well-covered by the substring list (confirmed by #1004 logs). No
  evidence of a missed auth-error surface form today. Keep the scope tight.
- **Should we add a third predicate, `messaging.IsThirdPartyAuthError`, to
  the invalid-token matcher?** — Recommendation: no. APNs auth failures
  (`THIRD_PARTY_AUTH_ERROR`) are already handled in the retry branch at
  `provider.go:73` and produce their own ERROR with actionable guidance;
  routing them to device cleanup would be wrong — the token is fine, our
  APNs certificate isn't.
- **Do we also want a smoke test that boots the notification service and
  verifies an `UNREGISTERED` response triggers `UnregisterDevice`?** —
  Recommendation: skip. Unit tests covering the predicate plus a log-based
  verification after deploy (Phase 3) are sufficient. Integration coverage
  here would require a fake FCM HTTP server, which is out of proportion to
  the bug.

## Implementation Plan

### Phase 1: Fix the matcher and add tests
**Goal:** `NotRegistered` errors wrap with `ErrInvalidToken` and trigger
device cleanup.
**Tasks:**
- [ ] In `server/notifications/fcm/provider.go`, rewrite `isInvalidTokenError`
  to try `messaging.IsUnregistered(err) || messaging.IsInvalidArgument(err) ||
  messaging.IsSenderIDMismatch(err)` first, then fall back to the existing
  substring list with `"NotRegistered"` appended.
- [ ] In `server/notifications/fcm/provider_test.go`, add a
  `"NotRegistered (HTTP v1 human-readable message)"` case to
  `TestIsInvalidTokenError` and a `TestSend_NotRegistered_ReturnsInvalidToken`
  mirroring `TestSend_InvalidTokenError_ReturnsInvalidToken` at line 539.
- [ ] Run `go test ./server/notifications/...` to confirm no regressions.

**Validates:** The matcher now recognizes `NotRegistered`; `Send` returns an
`ErrInvalidToken`-wrapped error for that surface form.

### Phase 2: Verify build and deploy to dev
**Goal:** Ship the fix to dev and confirm the alert stream subsides.
**Tasks:**
- [ ] `npm run build` — lint + compile.
- [ ] Deploy to dev via the usual pipeline.
- [ ] Tail Cloud Logging for ~2 hours after deploy and confirm:
  - At most one `"invalid or expired token detected"` WARN line per stale
    token, followed by `"automatically removing invalid device token"` INFO
    and `"successfully removed invalid device"` INFO.
  - No further ERROR `"failed to send FCM message"` lines with
    `error: "NotRegistered"`.
- [ ] Confirm the `dkTKbhlp...` device row is gone from the `devices` table
  on dev.

**Validates:** The fix works against real FCM errors, not just the mocked
ones in tests.

### Phase 3: Confirm prod behavior (rollout only, no extra work)
**Goal:** Same-shaped confirmation on prod after the next release rolls.
**Tasks:**
- [ ] After the next prod deploy, spot-check Cloud Logging on `server` for
  the same `NotRegistered` → cleanup pattern. No prod-specific code
  changes needed.

**Validates:** Prod drains its own backlog of stale iOS tokens naturally as
their next notification delivery triggers cleanup.

## Related Issues

- #1004 — The sibling fix that reshaped the FCM log cascade and created
  `isAuthError` / `isInvalidTokenError`. This issue patches a gap in the
  latter.
- #1184 / #1185 — User feedback on keeping ERROR logs loud. This fix honors
  that: we're routing a misclassified ERROR into its correct WARN+cleanup
  path, not silencing it.

---

## Audit Notes

**Audited against:** `docs/server/architecture.md`. Client docs
(`docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`) are not applicable — this is a server-side,
library-internal fix with no API surface, no client behavior, and no cached
entity involvement.

**Findings:**

1. **Shared library boundary (OK):** The fix stays inside
   `server/notifications/fcm`, which is a shared library per the server
   architecture doc's "Shared Functionality in Libraries" principle. No RPC
   handler or service-layer behavior changes, so no service coupling is
   introduced.
2. **Log-level discipline (OK):** The change moves `NotRegistered` from the
   generic ERROR branch (`provider.go:107`) to the existing WARN branch
   (`provider.go:99` via `isInvalidTokenError`) that already signals
   "invalid or expired token detected" and wraps `ErrInvalidToken`. That
   matches the server-architecture log-level guidance — WARN for
   "unexpected but handled," ERROR reserved for "system failures requiring
   attention." The remaining ERROR branch stays loud for genuine failures,
   consistent with the #1184/#1185 feedback.
3. **Device cleanup safety (OK):** Wrapping with `ErrInvalidToken` triggers
   the existing `UnregisterDevice` path in `server/notifications/service.go`.
   The failure modes added to the list (`NotRegistered`) describe
   *client-side* invalid tokens; they cannot be confused with server-side
   auth issues (which go through `isAuthError` + retry on a different
   branch). No risk of spurious device removal.
4. **Test coverage (OK):** Plan adds both a unit case in
   `TestIsInvalidTokenError` and an end-to-end mock case in
   `TestSend_NotRegistered_ReturnsInvalidToken`, following the table-driven
   + behavioral pattern the architecture doc requires for new code in shared
   libraries.
5. **No storage, proto, or monitoring-config changes:** No schema or
   Terraform impact. The existing "Server Error Logged" alert policy in
   `terraform/modules/monitoring/main.tf:402` will simply stop firing for
   this surface form once the fix deploys — the desired outcome, not a
   policy change.
6. **Scope discipline (OK):** Decision log in Recommendation records *why*
   we skipped the typed-predicate option (Option B) and the hybrid
   (Option C) — the typed predicates require unimportable
   `*internal.FirebaseError` types that no unit test can construct. Adding
   untested production code just to look idiomatic would contradict the
   "comprehensive test coverage" principle.
