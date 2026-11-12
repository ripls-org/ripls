# Issue #2588: Off-app email/SMS sent to E2E simulation users exhausts shared Mailgun daily quota

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2588
**Filed:** 2026-06-29 | **Last activity:** 2026-06-29
**Labels:** bug, P1, ops, dep:email, backend:mailgun | **Assignees:** unassigned
**Context docs:** `docs/sms_notifications.md` (governs `off_app_email.go`/`off_app_sms.go`), `docs/server/architecture.md` ⚓, `docs/server/conventions.md` ⚓, `docs/server/observability.md` ⚓, `docs/server/metrics.md`, `docs/testing/architecture.md` ⚓, `docs/simulation.md`, `docs/users/priorities.md` ⚓; review prompts skimmed: observability, error_handling, testing.

> **Status (2026-06-29):** Phases 1–2 implemented in PR #2590. Phase 3
> (deployed-dev verification) runs after merge + deploy; Phase 4 (separate dev
> Mailgun domain) to be filed as a follow-up issue.

## Problem Statement

The off-app email channel (deliver a notification as email when a recipient has
no active app device) went live recently (`off_app_email_enabled = true` in both
`terraform/environments/dev/main.tf:265` and `prod/main.tf:406`). Immediately
after, `server-dev` began hitting Mailgun's **per-domain daily** send limit on
`ripls.app` (HTTP 429, "daily request limit exceeded").

Log investigation (project `ripls-dev`, service `server-dev`, last 24h, filtered
on `jsonPayload.email_type:*`) showed **all 210 send attempts were
`email_type=off_app_notification`** and went to **exactly 4 synthetic recipients,
every one on `e2etest.example.com`** — the E2E fixture personas. 168 were
accepted, then 42 were rejected with the 429. No real user appears in the window.

Root cause: `server/e2e_test.go` registers real users with
`{prefix}-{id}@e2etest.example.com` via the live `EmailRegister` RPC
(`createUniqueTestUser`, `e2e_test.go:245-248`). When the suite runs with
`E2E_SERVER_URL=https://server-dev.ripls.org` it drives the **deployed dev
server**, which holds a real Mailgun key. The test users are **deviceless**, so
every workflow notification falls through to `dispatchOffApp` and sends a real
email. The same is true of the `server/simulation` load-test harness, which also
registers users via `EmailRegister` with a `SimulationId`
(`server/simulation/runner.go:236,240`). Local E2E runs are unaffected — the
hermetic server starts with no `-mailgun-api-key` (`e2e_test.go:140`) and uses
the mock sender, **and** with no `-off-app-email-enabled`, so the off-app channel
is wired off entirely.

Amplifier: neither dev nor prod sets `mailgun_domain`, so both default to
`ripls.app` (`terraform/modules/containers/variables.tf:116`) — **one shared
daily quota**. E2E noise against dev can therefore 429 real prod email
(password resets, invites, notifications).

The clean fix: notifications to **simulation/test users must never touch a real
external delivery provider**. Simulation users are already distinguishable —
`User.simulation_id` is persisted (field 17, `proto/ripls/models/user.proto:88`;
set in `server/services/login/registration.go:107` and `phone_auth.go:107`).

**Added requirement (from review):** the E2E suite should *assert* the corrected
behavior — that a notification to a deviceless simulation recipient **reached its
expected off-app channel and was suppressed because the recipient is a simulation
user**, with **no real/mock provider send** leaking. This makes the fix a
regression guard, not just a one-time correction.

## Context from Comments

**No GitHub comments** — this is a freshly filed issue (filed by the same
investigation that produced this plan), so the body already carries the full
root-cause analysis, log evidence, recommended fix, acceptance criteria, and the
two out-of-scope follow-ups. One refinement came **in-conversation** after
filing: the requester asked that the E2E tests assert the suppression behavior
(reached-the-expected-channel-then-suppressed), captured above and in Phase 2.

## Potential Solutions

### Option A: Per-channel `SimulationId` suppression in the off-app send path (recommended)

- **Approach:** Add a simulation check inside **each** off-app channel function,
  positioned *after* that channel is determined applicable (so the channel is the
  one that would really have been used) and *before* the provider call:
  - `sendOffAppSMS` (`server/notifications/off_app_sms.go`): after the
    `Enabled && Sender != nil` and `GetPhoneNumber() != ""` gates, before the
    quiet-hours / opt-out / send logic — record `{channel="sms",
    outcome="suppressed_simulation"}` and return `true` (handled — do **not**
    fall back to email).
  - `sendOffAppEmail` (`server/notifications/off_app_email.go`): after the
    `Enabled && Sender != nil` and `user.Email != ""` gates, before building the
    message and sending — record `{channel="email",
    outcome="suppressed_simulation"}` and return.
  - Share one helper, e.g.
    `func (s *service) suppressedForSimulation(ctx, user, channel string) bool`,
    that returns true (and logs + counts) when `user.GetSimulationId() != ""`.
- **Why per-channel, not a single pre-selection guard:** the channel label then
  reflects the channel the notification **actually routed to** (SMS for a phone
  recipient with SMS enabled; email otherwise — including the realistic case
  where SMS is flag-off, which naturally records `email`). That is exactly what
  the "reached the expected channel" assertion needs, and it makes the metric and
  log self-describing. A single guard in `dispatchOffApp` before channel
  selection cannot name the channel.
- **Pros:** Keyed on **intent** (this row is test/simulation data), not a brittle
  string pattern. Covers **both** channels and **both** producers (E2E +
  simulation harness), every environment, with one small helper. Matches the
  existing best-effort, log-and-count style of each channel function. Provider is
  never called, so even a mock sender records zero sends.
- **Cons:** Two call sites instead of one (mitigated by the shared helper). Adds
  one metric outcome value (docs/dashboards update).
- **Effort:** Small.

### Option B: Single pre-selection guard in `dispatchOffApp`, count under `email`

- **Approach:** One check right after the user load in `dispatchOffApp`, before
  channel selection; count `{channel="email", suppressed_simulation}` by
  convention (matching the existing `suppressed_disabled`/`suppressed_no_handle`
  pre-selection counts at `off_app_email.go:74,82`).
- **Pros:** One call site; marginally simpler.
- **Cons:** **Cannot satisfy the new requirement** — the suppression happens
  before a channel is chosen, so it cannot assert "reached the *expected*
  channel." The `email` label would be a fiction for a phone recipient. Rejected
  in favor of A for exactly this reason.
- **Effort:** Small.

### Option C: Suppress by recipient email domain (`@e2etest.example.com`)

- **Approach:** Skip in `sendOffAppEmail` when `user.Email` ends in the E2E test
  domain.
- **Cons:** Email-only (leaves the metered A2P SMS path exposed); string-pattern
  kludge that breaks if a fixture domain changes and does nothing for the
  simulation harness's phone personas (`feedback_fix_root_cause`). Rejected.
- **Effort:** Small.

### Option D: Infra hardening (separate dev Mailgun domain) — complementary follow-up

- Give `ripls-dev` its own Mailgun sandbox/subdomain so dev can never draw on the
  prod `ripls.app` quota. Defense in depth, but does **not** fix the "we're
  emailing fake people" defect. Tracked as a separate follow-up (Phase 4).

## Recommendation

**Option A.** It fixes the root cause (never deliver to simulation identities),
is channel-accurate (so the metric/log name the real channel and the E2E
assertion can verify "reached the expected channel"), protects the A2P SMS path,
and covers both the E2E and simulation-harness producers. Option B is rejected
specifically because it can't support the reached-the-expected-channel assertion;
C is a weaker subset; D is additive and filed separately.

Illustrative shape (final naming at implementation):

```go
// shared helper — pass the channel's already-scoped logger so the line keeps
// that channel's recipient field (off_app_sms.go scopes recipient_phone;
// off_app_email.go scopes user_id=recipient). Do NOT re-derive a logger from
// ctx here: LoggerWithContext injects the *authenticated caller's* user_id,
// which in dispatchOffApp is not necessarily the recipient, and adding a second
// user_id would shadow the channel logger's recipient binding (observability.md
// → request-id/user-id field rules). INFO matches the sibling suppressions.
func suppressedForSimulation(ctx context.Context, logger *slog.Logger, user *models.User, channel string) bool {
    if user.GetSimulationId() == "" {
        return false
    }
    // Simulation/test identities (E2E suite, load-test harness) must never reach
    // a real delivery provider — they would burn Mailgun's shared ripls.app
    // daily quota and the metered A2P SMS path on fake recipients (#2588).
    logger.InfoContext(ctx, "off-app: simulation recipient; suppressing send",
        "simulation_id", user.GetSimulationId(),
        "channel", channel, "outcome", "suppressed_simulation")
    metrics.OffAppNotificationsTotal.WithLabelValues(channel, "suppressed_simulation").Inc()
    return true
}

// sendOffAppSMS: after the enabled+phone gates and the `logger` build (currently
// off_app_sms.go:76), before the quiet-hours/opt-out logic:
if suppressedForSimulation(ctx, logger, user, "sms") {
    return true // handled — do not fall back to email
}

// sendOffAppEmail: right after the `logger` build (currently off_app_email.go:97),
// before the no-email / opted-out checks, so any simulation recipient routed to
// the email channel is recorded as suppressed (email is the last-resort channel):
if suppressedForSimulation(ctx, logger, user, "email") {
    return
}
```

(`simulation_id` is a synthetic, non-PII tag — safe to log and the key the E2E
assertion filters on; no email/phone is logged by the new code, consistent with
the masking rules in `docs/server/observability.md`.)

## Open Questions

- **Which `channel` label does the suppression carry?** — Resolved by Option A:
  the **actual selected channel** (`sms` for a phone recipient with SMS on,
  `email` otherwise). No "count under email by convention" needed; the per-channel
  placement makes the label truthful. Counted **once** (the chosen channel
  short-circuits the other).
- **How should the E2E suite observe the suppression?** — Recommendation:
  **structured-log capture filtered by `simulation_id`** (primary). The local
  e2e server already logs JSON (`-log-format json`); capture its stdout into a
  thread-safe buffer and poll for a line with `outcome="suppressed_simulation"`
  and this test's `simulation_id`. This attributes each suppression to *this*
  test's simulation (no cross-subtest counter race), and the line names the
  `channel` and `user_id` — fully satisfying "reached the expected channel."
  **Alternative:** scrape `GET ${serverURL}/metrics` (open in dev-mode,
  `main.go:486`) and assert a delta on
  `ripls_off_app_notifications_total{channel="email",outcome="suppressed_simulation"}`;
  simpler but the counter is process-global and unlabeled by simulation, so under
  async dispatch a previous subtest's in-flight goroutine could perturb an exact
  delta — assert "increased by ≥ N" if using this. Use logs for precision.
- **Dispatch is asynchronous — how does the assertion avoid flaking?** — Some
  notifications are sent in background goroutines (`request/lifecycle.go:128,159`),
  so the suppression log/metric appears *after* the triggering RPC returns. Poll
  the captured log/metrics with a bounded timeout (reuse the
  `waitForServerReady` polling style) rather than reading once.
- **Should the guard also cover the push path (users *with* devices)?** — No.
  Simulation/E2E users never register real FCM/APNs tokens, so push to them
  no-ops or fails harmlessly without reaching a person; they hit the off-app path
  precisely because they're deviceless. Keep the guard in the off-app channels.
- **Does the local e2e server need new wiring to exercise this?** — Yes, minimal:
  start it with `-off-app-email-enabled` so the off-app **email** channel is wired
  (its `Sender` is `MockEmailService` since no Mailgun key — `main.go:867`, so no
  real email is possible). Verify no startup validation blocks enabling off-app
  email without a postal address / unsub secret; if it does, pass dummy
  `-mailgun-postal-address` / unsub-secret flags. The SMS channel can't be
  mock-wired through `main.go` today (it builds a real Twilio sender only from the
  three secrets), so **E2E asserts the email channel; the SMS-channel suppression
  is covered by the unit test** (`off_app_dispatch_test.go`, which can wire
  `MockSMSSender` directly).
- **Do we still want the infra follow-up (separate dev Mailgun domain)?** — Yes,
  as a separate issue (Phase 4). Defense in depth; don't block this fix on it.

## Implementation Plan

### Phase 1: Per-channel simulation suppression (the fix)
**Goal:** No off-app email or SMS is ever sent for a recipient with a non-empty
`SimulationId`; the suppression is observable and channel-accurate.
**Tasks:**
- [ ] Add `suppressedForSimulation(ctx, logger, user, channel)` helper (INFO log
  through the **channel's scoped logger** + increments
  `{channel, suppressed_simulation}`) and call it in `sendOffAppSMS` (after the
  enabled+phone gate and `logger` build, `off_app_sms.go:76`, return `true`) and
  `sendOffAppEmail` (right after the `logger` build, `off_app_email.go:97`,
  return). Do not re-derive a logger from ctx in the helper (avoids shadowing the
  recipient `user_id`/`recipient_phone` binding — see observability rules).
- [ ] Extend the `OffAppNotificationsTotal` doc comment in
  `server/metrics/metrics.go:79-92` to list the new `suppressed_simulation`
  outcome.
- [ ] Update `docs/server/metrics.md:98-100` and the Observability section of
  `docs/sms_notifications.md:186-194` to include `suppressed_simulation`; advance
  the `freshness` stamp on `docs/sms_notifications.md`. (`docs/server/observability.md`
  needs no change — it defers `/metrics` enumeration to `metrics.md`.)
- [ ] Unit tests in `server/notifications/off_app_dispatch_test.go` (extend
  `insertDevicelessUser`, or add `insertSimulationUser`, to set `SimulationId`):
  (a) simulation user with **phone** + both channels enabled → **0** SMS, **0**
  email, `{sms, suppressed_simulation}` counted;
  (b) simulation user with **email only** → **0** email, `{email,
  suppressed_simulation}` counted;
  (c) regression: non-simulation deviceless users → existing behavior unchanged.
**Validates:** Provider is never called for a simulation recipient, on either
channel, with the correct channel label — proven with the existing
`fakeEmailSender` + `sms.MockSMSSender` rig (no live providers).

### Phase 2: E2E assertion — reached-the-expected-channel-then-suppressed
**Goal:** The end-to-end suite proves a real workflow's notification to a
deviceless simulation recipient routes to its off-app channel and is suppressed,
with nothing leaking to a (mock) provider — a durable regression guard.
**Tasks:**
- [ ] In `startLocalServer` (`server/e2e_test.go`), add `-off-app-email-enabled`
  to the args so the off-app email channel is wired with the mock sender (add
  dummy postal-address / unsub-secret flags only if startup validation requires
  them).
- [ ] Capture the server subprocess's stdout/stderr: set `cmd.Stdout`/`cmd.Stderr`
  to an `io.MultiWriter(os.Stderr, lockedBuf)` (thread-safe buffer) so logs both
  surface in test output and are inspectable. Add a small
  `waitForLogLine(buf, predicate, timeout)` helper that polls the buffer.
- [ ] In an existing notifying workflow (e.g. the request or loan lifecycle test,
  whose recipients are deviceless `SimulationId`-tagged users), after the step
  that triggers a notification, poll the captured log for a JSON line with
  `outcome="suppressed_simulation"`, `channel="email"`, and this test's
  `simulation_id`; assert it appears (count ≥ the notifications that workflow
  emits to deviceless recipients).
- [ ] Assert the **negative**: no `off-app email sent to deviceless recipient` /
  `email sent successfully` line for that `simulation_id` — i.e. nothing leaked to
  the provider.
- [ ] (Optional, if cheap) cross-check via `GET /metrics` that
  `ripls_off_app_notifications_total{outcome="suppressed_simulation"}` is
  non-zero after the run.
**Validates:** The corrected routing-then-suppression behavior holds through the
full server, not just the unit under test; future regressions (e.g. a guard moved
or removed) fail CI.

### Phase 3: Verify against deployed dev and close out
**Goal:** Confirm the real-world symptom is gone.
**Tasks:**
- [ ] After merge + deploy to `server-dev`, run the notifying E2E suite against
  deployed dev (`E2E_SERVER_URL=https://server-dev.ripls.org`).
- [ ] Re-query `server-dev` logs for `email_type=off_app_notification` over the
  run window: expect **zero** Mailgun sends to `e2etest.example.com` and a
  non-zero `suppressed_simulation` count.
- [ ] Close #2588.
**Validates:** End-to-end the channel still works for real deviceless users but is
inert for simulation traffic; the shared-quota bleed stops.

### Phase 4: Infra hardening follow-up (separate issue, not blocking)
**Goal:** Remove the dev→prod shared-quota blast radius independent of the
simulation guard.
**Tasks:**
- [ ] File a follow-up issue: give `ripls-dev` its own Mailgun sandbox/subdomain
  (set `mailgun_domain` in `terraform/environments/dev/main.tf`).
- [ ] Note the operational check there: whether any real prod emails were 429'd
  during the 2026-06-29 window, and whether the notifying E2E suite should target
  deployed dev at all (local hermetic mode now asserts these paths with mocks).
**Validates:** Defense in depth; out of scope for the #2588 code fix.

## Related Issues
- #2492 — Phone-first / SMS-native pivot epic; introduced the off-app email + A2P
  SMS channels this guard protects.
- #2569 — SMS outbound delivery status callbacks (adjacent off-app SMS
  observability follow-up; unaffected by this change).

## Audit Notes

Audited against the documentation context map (`docs/llms.txt`) on 2026-06-29.
Scope: server-side change in `server/notifications/**` + a metric outcome + docs
+ E2E wiring. Engaged dimensions: observability, testing, error_handling,
code_quality, security/PII.

Findings applied:
- **Observability (logger scoping).** Original illustrative helper re-derived a
  logger from ctx and logged `user_id`. Per `docs/server/observability.md`
  (request-id/user-id field rules + `LoggerWithContext` injecting the
  *authenticated* user_id), and matching the existing channel functions
  (`off_app_sms.go:76` scopes `recipient_phone`; `off_app_email.go:97` scopes
  `user_id`=recipient), the helper now takes the **channel's scoped logger** and
  logs only `simulation_id`/`channel`/`outcome`. Avoids a shadowed/duplicated
  `user_id` and keeps each channel's recipient field intact.
- **Observability (level + metric reuse).** Suppression logs at **INFO**
  (matches sibling `suppressed_*` lines; an expected, handled outcome — not an
  ERROR/alertable event). Reuses the existing `OffAppNotificationsTotal` counter
  in the `server/metrics` registry rather than adding a metric, per the
  observability doc's "reuse the registry" guidance. Confirmed only
  `metrics.md` + `sms_notifications.md` need the new outcome documented;
  `observability.md` defers to `metrics.md`.
- **Insertion points pinned** to `off_app_sms.go:76` and `off_app_email.go:97`
  so the channel label is truthful (the channel actually selected), satisfying
  the reached-the-expected-channel requirement.
- **Testing (`docs/testing/architecture.md`).** Layering is correct: unit tests
  in `server/notifications/` (both channels, via `fakeEmailSender` /
  `sms.MockSMSSender`) + a system-level assertion in `server/e2e_test.go`. Noted
  the E2E channel constraint (SMS can't be mock-wired through `main.go`, so E2E
  covers email; unit covers both) so the suite isn't over-promised.
- **Security/PII.** No new PII surface — `simulation_id` is synthetic; no
  email/phone is logged by the new code (masking rules upheld).
- **Error handling.** Best-effort contract preserved — the guard returns cleanly
  (no error propagation); `sendOffAppSMS` returns `true` to correctly suppress
  the email fallback for a phone-bearing simulation recipient.

Docs consulted: `docs/llms.txt`, `docs/sms_notifications.md`,
`docs/server/observability.md`, `docs/server/metrics.md`,
`docs/testing/architecture.md`, `docs/server/architecture.md`,
`docs/server/conventions.md`, `docs/simulation.md`; review rubrics skimmed:
observability, testing, error_handling, code_quality, security.
