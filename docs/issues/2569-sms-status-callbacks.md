# Issue #2569: SMS: handle Twilio outbound delivery status callbacks

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2569
**Filed:** 2026-06-27 | **Last activity:** 2026-06-27
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** `docs/sms_notifications.md`, `docs/server/observability.md`, `docs/server/metrics.md`, `docs/server/architecture.md` ⚓, `docs/server/conventions.md` ⚓, `docs/proto_conventions.md` ⚓ (no proto change — confirmed N/A), plus review dimensions `docs/reviews/observability_review_prompt.md`, `docs/reviews/security_review_prompt.md`

## Problem Statement

Platform SMS (epic #2492, landed in #2566) currently learns only the
**synchronous** result of a send: the `accepted`/`queued` that Twilio returns
from the `POST .../Messages.json` call (`TwilioSMSSender.SendSMS`,
`server/notifications/sms/twilio.go`). It never learns whether the message was
actually **`delivered`**, or was **`undelivered`/`failed`** with a carrier
`ErrorCode` (e.g. 30007 carrier filtering, 21610 recipient opted out, 30003/30005
unreachable/unknown destination). Twilio reports that lifecycle asynchronously to
a **separate status-callback URL** that we do not yet expose.

The gap is purely **observability**: a message that Twilio accepts but a carrier
silently filters looks identical to a delivered one in our logs today. For an A2P
10DLC program that just cleared registration, undelivered/failed rates are the
earliest signal of campaign/reputation trouble, so we want to see them.

**Scope decision (from discussion, supersedes the issue's item 2).** The issue as
written asks for a Prometheus `ripls_sms_delivery_total{status}` counter *and*
ERROR logs. We observe deliverability the same way we observe every other server
signal — **through Cloud Logging log-based metrics and alerts**
(`docs/server/observability.md`: "Log-Based Metrics — Implemented 100%",
"Alerting — Implemented 100%"; the existing `rpc_errors_{env}` /
`health_check_failures_{env}` metrics are built from log lines, not from
`/metrics`). So the deliverable is a handler that emits **structured slog** for
each delivery status; the counting and alerting are defined **outside code** as a
log-based metric in Cloud Monitoring, exactly like the `rpc_errors` metric. No
Prometheus counter is added. See Option A vs. Option B for the tradeoff, and Open
Questions for how the log-based metric gets created.

## Context from Comments

**The issue has no comments.** The design is, however, well-specified in the issue
body and — unusually — the target subsystem doc **already describes this exact
follow-up** as *Future work*:

- `docs/sms_notifications.md` → "Future work → Outbound delivery status callbacks
  (#2569)": *"A separate `POST /sms/status` endpoint (distinct from the inbound
  `/sms/webhook`; payload is `MessageSid`/`MessageStatus`/`ErrorCode`,
  signature-verified the same way) would record delivery outcomes."*
- The same doc's Status block lists this as the one remaining gap now that
  outbound send and inbound STOP/START/HELP are both verified against real Twilio.

So there is a clear, doc-blessed shape to build against. The only material
decision *not* pre-settled is the log-vs-metric observability sink — resolved
above in favor of structured logging + a Cloud log-based metric.

## Existing code this builds on (Phase 2 research)

The inbound webhook is a near-perfect template; the status handler is a sibling of
it with a different payload and no state change.

- **Handler pattern** — `server/services/web/sms_webhook.go`
  (`HandleSMSWebhook`): `ParseForm` → build `publicURL = "https://" + s.hostname +
  path` → `sms.ValidateSignature(s.smsWebhookAuthToken, publicURL, r.PostForm,
  header)` → `403` on bad signature *before any side effect* → do work → ack.
  The status handler mirrors steps 1–3 and 5; step 4 is "log", not "persist".
- **Signature primitive** — `sms.ValidateSignature(authToken, publicURL, params,
  header)` in `server/notifications/sms/twilio.go`. Provider-agnostic HMAC-SHA1;
  empty token ⇒ returns `true` (verification disabled — dev/local only). Reused
  verbatim; the status callback is signed the same way over its own URL + params.
  **Note for security review:** the SHA-1 here is **Twilio's mandated
  X-Twilio-Signature algorithm** (HMAC-SHA1 over URL+params), not a free
  cryptographic choice — it is HMAC (keyed) and compared in constant time
  (`hmac.Equal`), and the inbound webhook already relies on it. Do not "upgrade" it.
  The `ErrorCode` we log (`twilio_code`) is a Twilio numeric error code, **not
  PII** — the same field the send path already logs with that annotation.
- **Auth token wiring** — `Service.smsWebhookAuthToken` +
  `SetSMSWebhookAuthToken(token)` (`server/services/web/service.go`), set in
  `main.go:1311-1312` from `cfg.twilioAuthToken`. It is **the same Twilio account
  auth token** that signs both the inbound webhook and the status callback, so the
  status handler reuses the same field — but the field/setter name says "webhook",
  which now undersells it (see Open Questions on a rename).
- **Host binding** — the web service `hostname` is `cfg.inviteLinkHostname`
  (`ripls.app` / `dev.ripls.app`). The inbound webhook signature-verifies against
  `https://<hostname>/sms/webhook`; the status endpoint verifies against
  `https://<hostname>/sms/status`. Twilio's configured callback URL must match that
  host exactly (same host the inbound webhook already uses on `dev.ripls.app`).
- **Route registration** — `main.go:1670`:
  `mux.Handle("POST /sms/webhook", hostMiddleware(http.HandlerFunc(webService.HandleSMSWebhook)))`.
  The status route is registered one line over, identically.
- **Logging fields to reuse for join-ability** — `SendSMS` already logs
  `message_sid`, `message_status`, `twilio_code`, `twilio_message`,
  `recipient_phone` (via `logging.MaskPhone`), `duration_ms`, and
  `external_service="twilio"`. Emitting the **same field names** from the status
  handler lets a log query correlate a send to its delivery outcome by
  `message_sid`. Log levels per `docs/server/observability.md`: system failures →
  ERROR (trips the "Server Error Logged" alert), normal lifecycle → INFO/DEBUG.
- **PII rule** — phone masked in every log (`logging.MaskPhone`); body never
  logged. Status callbacks carry **no** `Body`/`From`, so the PII surface is just
  the `To` number — masked. (`docs/sms_notifications.md` → Observability.)
- **Test template** — `server/services/web/sms_webhook_test.go`: `twilioSignature`
  helper (recomputes Twilio's HMAC so a *validly-signed* request is proven
  accepted, not just a forged one rejected), `newWebServiceForTest`, valid/invalid
  signature cases. The status tests clone this structure.

## Potential Solutions

### Option A: `POST /sms/status` handler that emits structured logs only (recommended)

- **Approach:** New file `server/services/web/sms_status.go` with
  `HandleSMSStatusCallback`. Signature-verify against `https://<hostname>/sms/status`
  using the existing `smsWebhookAuthToken`; on bad signature return `403` before
  anything else. Parse `MessageSid`, `MessageStatus`, `ErrorCode`, `To`. Emit one
  structured slog line per callback, reusing the `SendSMS` field names
  (`message_sid`, `message_status`, `twilio_code`, `recipient_phone`
  masked): **ERROR** for `undelivered`/`failed` (so the existing "Server Error
  Logged" alert policy fires and Cloud Logging can build a per-`message_status`
  log-based metric), **INFO** for `delivered`, **DEBUG** for intermediate
  `queued`/`sending`/`sent`. Always return `204 No Content` once authenticated
  (Twilio ignores the body of a status-callback response and retries on non-2xx).
  Wire the route at `main.go:1670`. Configure Twilio to call it (Messaging Service
  "Delivery Status Callback URL" — see Option's sub-choice below). Document in
  `docs/sms_notifications.md`.
- **Pros:** Smallest possible change; a near-exact sibling of the proven inbound
  webhook. Zero new metric cardinality, zero `metrics.go` churn. Matches how the
  project *actually* observes everything (log-based metrics/alerts). ERROR lines
  slot straight into the existing alert policy. No Twilio type or PII escapes.
- **Cons:** Delivery *rate* dashboards live in Cloud Monitoring (a log-based metric
  defined in Terraform/console), not in the `/metrics` scrape — one more place to
  look, and the metric must be created out-of-band (Open Questions). Log-based
  counters have coarser resolution and retention than a Prometheus counter.
- **Effort:** Small.

### Option B: Same handler, but with a `ripls_sms_delivery_total{status}` Prometheus counter (the issue as literally written)

- **Approach:** Everything in Option A **plus** register a
  `SMSDeliveryTotal = CounterVec{status}` in `server/metrics/metrics.go`
  (+ `newDefaultRegistry`), `.WithLabelValues(status).Inc()` in the handler, and
  document it in `docs/server/metrics.md`.
- **Pros:** First-class time-series with proper histograms/retention; a
  self-contained rate dashboard without a Cloud Monitoring log-metric. Literally
  satisfies the issue text.
- **Cons:** Adds a metric the team does not currently scrape/alert on (the
  `/metrics` Prometheus scrape config is still a *planned* GMP follow-up per
  `docs/server/metrics.md` → Scrape Configuration), so the counter would be
  write-only until that lands — duplicating an observability path we already have
  via logs. Against the user's explicit steer.
- **Effort:** Small (marginally larger than A).

### Option C: Feed hard failures back into the opt-out / suppression store

- **Approach:** On a status callback with a hard-failure `ErrorCode` (notably
  `21610` opted-out; possibly repeated `30007`/`30003`), mirror into
  `server/smsoptout` (or a new per-number failure counter) so the send path
  suppresses future texts to a dead number.
- **Pros:** Turns observability into self-healing deliverability; stops burning A2P
  spend + reputation on numbers that will never receive.
- **Cons:** Real behavior change with correctness stakes (a transient `30007` is
  not a permanent condition — must not permanently suppress on a blip); needs a
  threshold/decay policy and its own tests. The issue explicitly lists this as
  *"Optional"*. Out of scope for the observability fix.
- **Effort:** Medium.

## Recommendation

**Option A**, with the Twilio side wired at the **Messaging Service level** rather
than per-message.

Rationale:

1. **Log-based observability is the house style**, confirmed by
   `docs/server/observability.md` (log-based metrics + alerting both 100%
   implemented; `rpc_errors`/`health_check_failures` are log-derived) and the
   user's steer. An ERROR line on `undelivered`/`failed` immediately participates
   in the existing alert policy; a Cloud Monitoring log-based metric grouped by
   `message_status` gives the rate dashboard without new scrape infrastructure.
2. **Messaging Service-level callback mirrors the inbound webhook.** The inbound
   `/sms/webhook` is configured on the Messaging Service in the Twilio console, and
   sends already route through the Messaging Service SID (no bare number). Setting
   the Messaging Service's single "Delivery Status Callback URL" to
   `https://<inviteLinkHostname>/sms/status` covers **all** messages with **zero**
   change to `TwilioSMSSender` — keeping the deliberately-thin, hostname-agnostic
   `sms` package free of the public URL. Per-message `StatusCallback` (plumbing a
   `statusCallbackURL` through `SMSChannel` → `NewTwilioSMSSender` → the send form)
   is the fallback only if we ever need per-message-type routing; we don't.
3. **Defer Option C.** File it as the tracked follow-up the issue already
   anticipates; don't fold a behavior change into an observability fix.

If the team later stands up the GMP/Prometheus scrape (the planned follow-up in
`metrics.md`), promoting to Option B is a two-line addition — the handler already
has `status` in hand.

## Open Questions

- **Where does the log-based delivery metric + alert get defined?** —
  Recommendation: create a Cloud Monitoring **log-based metric** (counter, label =
  `message_status` extracted from the structured field) plus an alert on
  `undelivered`/`failed` rate, in the same Terraform/console location as the
  existing `rpc_errors_{env}` metric — **out of this PR's code**, matching
  `metrics.md` → "Relationship to Logging" (scrape/alert config lives in Terraform,
  not the server change). Track it as an ops task in the same issue. The code PR's
  job is only to emit a clean, greppable ERROR/INFO line with a stable
  `message_status` field.
- **Rename `smsWebhookAuthToken` → `smsAuthToken`?** — Recommendation: **yes**,
  rename the field + `SetSMSWebhookAuthToken` → `SetSMSAuthToken` (and the
  `main.go` call site) in this PR. It is the Twilio *account* auth token that now
  verifies **two** endpoints, not just the inbound webhook; the "webhook" name will
  mislead the next reader. Small, mechanical, and keeps the seam honest. If we'd
  rather minimize diff, keep the name and add a one-line comment that it also
  signs `/sms/status` — but the rename is cleaner.
- **What `MessageStatus` values do we log, and at what level?** —
  Recommendation: log every callback but level by terminal outcome — `failed`,
  `undelivered` → **ERROR** (with `twilio_code`); `delivered` → **INFO**;
  `queued`/`sending`/`sent`/`accepted` → **DEBUG** (intermediate, high-volume,
  low-signal). `read` (RCS/WhatsApp) → DEBUG for now. Keep the raw Twilio string in
  the `message_status` field so the log-based metric can group on it without a
  server-side enum to maintain.
- **Do we act on `ErrorCode 21610` (opted-out) seen in a status callback?** —
  Recommendation: **not in this PR** — the inbound webhook already mirrors opt-out
  state, and 21610 in a callback is redundant with that. Log it (ERROR, with code)
  and let Option C decide any store write later.
- **Rate-limiting / abuse of the public endpoint?** — Recommendation: rely on
  **signature verification** as the gate (a forged unsigned POST is `403` before
  any work, same as the webhook) and skip a rate limiter for now. The handler does
  no DB write and no send, so the blast radius of spam is a log line; add a per-IP
  limit only if production shows abuse (mirrors the `/go/{code}/decline` posture).
- **Does `hostMiddleware` / host matching interfere?** — Recommendation: register
  identically to `/sms/webhook` (wrapped in `hostMiddleware`); it only injects the
  request host into context for local media storage and is a no-op on GCS, so no
  special handling is needed.

## Implementation Plan

> **Status (2026-07-01):** **Phase 1 shipped** — `POST /sms/status`
> (`server/services/web/sms_status.go`, `HandleSMSStatusCallback`) with
> signature verification, per-outcome structured logging, 400/403/204 paths, and
> `sms_status_test.go` (captured-`slog` assertions); route wired in `main.go`;
> `smsWebhookAuthToken`→`smsAuthToken` rename applied; `sms_status.go` renders no
> user strings (no `check_no_inline_user_strings` registration). **Phase 2 docs
> shipped** — `sms_notifications.md` + web README updated, context map + freshness
> refreshed. **Remaining = ops (Phase 2 wiring + Phase 3):** Messaging Service
> Delivery Status Callback URL **configured** — confirm end-to-end against real
> Twilio; define the Cloud log-based metric + alert in Terraform.

### Phase 1: Status-callback handler + tests (the whole code change)

**Goal:** `POST /sms/status` signature-verifies and emits structured delivery
logs; wired into the mux; covered by tests mirroring the inbound webhook.

**Tasks:**
- [ ] Add `server/services/web/sms_status.go` with `HandleSMSStatusCallback`.
  Derive the logger from context — `logging.LoggerWithContext(ctx).With("operation",
  "SMSStatusCallback")` — so `request_id`/`user_id` propagate (same as the
  webhook). Flow: `ParseForm` (→ `400` on malformed form, mirroring the webhook) →
  `publicURL = "https://" + s.hostname + "/sms/status"` → `sms.ValidateSignature(...)`
  → `403` before any side effect (WARN on reject; WARN when the token is empty,
  same as the webhook) → parse `MessageSid`/`MessageStatus`/`ErrorCode`/`To` →
  structured slog (ERROR for `failed`/`undelivered` with `twilio_code`; INFO for
  `delivered`; DEBUG for intermediate), reusing `message_sid` / `message_status` /
  `twilio_code` / `recipient_phone`(masked via `logging.MaskPhone`) field names →
  `204 No Content`.
- [ ] Define `const smsStatusPath = "/sms/status"` next to `smsWebhookPath`.
- [ ] Register the route at `main.go:1670`:
  `mux.Handle("POST /sms/status", hostMiddleware(http.HandlerFunc(webService.HandleSMSStatusCallback)))`.
- [ ] (Recommended) Rename `smsWebhookAuthToken` → `smsAuthToken` and
  `SetSMSWebhookAuthToken` → `SetSMSAuthToken`, updating the `sms_webhook.go`
  reference and the `main.go:1312` call site.
- [ ] Add `server/services/web/sms_status_test.go` mirroring
  `sms_webhook_test.go`: reuse the `twilioSignature` helper + `newWebServiceForTest`.
  **Assertion design note:** unlike the webhook, this handler has **no DB state
  change** to assert on — its only observable output is the log line. So the
  behavioral assertions (right *level*/fields per status, masked phone, no body)
  require driving the handler with a **captured `slog` handler** (an
  `slog.NewJSONHandler` over a `bytes.Buffer`, injected via the logging package's
  context/default-logger seam) and parsing the emitted record — otherwise the test
  only proves the 204/403 signature contract, not the observability contract that
  is the entire point of this change. Confirm the `logging` package exposes a way
  to inject a test logger into context; if not, that seam is a small prerequisite.
  Cases: (a) valid signature → 204 **and** `delivered`→INFO / `failed`+`ErrorCode`→ERROR
  with `message_sid`+`twilio_code`+masked `recipient_phone` present and no raw
  number/body, (b) forged signature → 403 and **no** log record for the status,
  (c) intermediate status accepted quietly (DEBUG, still 204), (d) empty-token path
  accepts (dev mode).
- [ ] Register `sms_status.go` in `scripts/check_no_inline_user_strings.js` **only
  if** it emits user strings — expected N/A; note the decision in the PR.
- [ ] Update `server/services/web/README.md` to list the new `sms_status.go`
  handler (the project convention requires the per-directory README to cover key
  files). No `server/notifications/sms/README.md` change — the Messaging
  Service-level callback means the `sms` package/sender is untouched.

**Validates:** The endpoint accepts a genuinely-signed Twilio status callback,
rejects a forged one, and produces the exact ERROR/INFO/DEBUG lines a log-based
metric will key on — end-to-end in `go test` (server tests run locally per project
convention).

### Phase 2: Twilio wiring + docs

**Goal:** Twilio actually calls the endpoint in dev; the subsystem doc reflects
reality.

**Tasks:**
- [ ] Set the Messaging Service **Delivery Status Callback URL** to
  `https://dev.ripls.app/sms/status` (dev) in the Twilio console; verify a real
  send produces a `delivered` (and, against a magic-fail number, a `failed`)
  callback that the handler logs — the same end-to-end verification done for the
  inbound webhook (`docs/sms_notifications.md` cites `dev.ripls.app/sms/webhook`).
- [ ] Update `docs/sms_notifications.md`: move the "Future work → status callbacks"
  paragraph into the architecture/observability sections as **implemented**, add
  `/sms/status` to the Inbound/architecture diagram, and note the ERROR-on-failure
  logging + the Cloud log-based metric. Bump the doc's `freshness` stamp.
- [ ] Update the Status block's "Not yet handled" line (it currently names #2569).

**Validates:** A real Twilio message's delivery outcome shows up in dev logs with
the expected fields/levels; docs match the shipped behavior.

### Phase 3 (deferred, separate issue): log-based metric + alert, and hard-failure feedback

**Goal:** Close the observability loop in ops and (optionally) self-heal dead
numbers.

**Tasks:**
- [ ] Define a Cloud Monitoring **log-based metric** on the delivery ERROR/INFO
  lines, grouped by `message_status`, + an alert on failed/undelivered rate —
  alongside `rpc_errors_{env}` in Terraform (ops task, not app code).
- [ ] (Optional, Option C) File/scope hard-failure feedback into `smsoptout` /
  a suppression counter with a threshold+decay policy so a transient `30007`
  doesn't permanently suppress.

**Validates:** Deliverability rate is dashboarded and paged on; repeated hard
failures stop wasting A2P spend.

## Related Issues
- #2492 — Epic (phone-first / SMS-native pivot); this closes the last observability
  gap it enumerates.
- #2566 — Landed platform SMS (outbound send + inbound STOP/START/HELP webhook);
  the direct precedent and template for this handler.
- #2573 — Opt-out footer cadence (adjacent SMS follow-up).
- #2574 — RCS prep (RCS/WhatsApp `read` status is the same callback shape; this
  handler's `message_status` logging generalizes to it).

## Audit notes (ripls-audit)

Audited against the documentation context map. **Scope:** server-only; subsystems
SMS notifications + web service + observability. **Engaged dimensions:** security,
observability, error_handling, testing, code_quality; api_design confirmed N/A (no
proto/RPC change — status callbacks are Twilio→server form POSTs, not RPCs).

Fixes applied during audit:
- **Security:** annotated the HMAC-**SHA1** as Twilio's mandated signature
  algorithm (keyed HMAC, constant-time compare, already trusted by the inbound
  webhook) so the security rubric's "weak algorithm" check doesn't false-positive;
  confirmed `twilio_code`/`ErrorCode` is non-PII.
- **Observability / error handling:** made the context-derived logger
  (`logging.LoggerWithContext`, for `request_id` propagation) and the
  malformed-form `400` explicit in the handler tasks, matching the webhook.
- **Code quality (conventions):** added a task to update
  `server/services/web/README.md` for the new handler file (per-directory README
  rule); noted the `sms` package README is untouched.
- **Testing:** surfaced that the handler has no DB state to assert on, so proving
  the log level/fields/masking (the actual deliverable) needs a captured `slog`
  handler, not just status-code assertions — and flagged the test-logger seam as a
  possible small prerequisite.

Docs consulted: `docs/llms.txt`; anchors `docs/server/architecture.md`,
`docs/server/conventions.md`, `docs/proto_conventions.md`,
`docs/server/observability.md`; leaf docs `docs/sms_notifications.md`,
`docs/server/metrics.md`; review rubrics `docs/reviews/security_review_prompt.md`,
`docs/reviews/observability_review_prompt.md` (Areas to Review). Also read the
implementing code: `server/services/web/sms_webhook.go` + `_test.go`,
`server/services/web/service.go`, `server/notifications/sms/twilio.go`,
`server/notifications/off_app_sms.go`, `server/metrics/metrics.go`, and the
`main.go` wiring at lines 896–907, 1304–1313, 1628–1670.
