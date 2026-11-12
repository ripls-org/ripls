# Issue #2862: Mailgun delivery-event webhook: true email delivery SLI (email analog of #2569)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2862
**Filed:** 2026-08-06 | **Last activity:** 2026-08-07
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** `docs/llms.txt` (index); anchors `server/architecture.md`,
`server/conventions.md`, `server/observability.md`, `proto_conventions.md`;
leaf docs `sms_notifications.md` (the #2569 analog this mirrors),
`server/l10n.md` (globs `server/email/**`); review prompts
`reviews/observability_review_prompt.md`, `reviews/security_review_prompt.md`,
`reviews/error_handling_review_prompt.md`. Prior plans read:
`docs/issues/2569-sms-status-callbacks.md`, `docs/issues/2571-email-otp.md`.

**UI investigation:** skipped — the issue is a server-side webhook and a
monitoring-configuration concern with no UI surface. Nothing here can be observed in the
web bundle.

---

## ⚠️ Read this first: the code already merged, and one hazard is live

This is **not** a greenfield plan. Between the issue being filed and this plan
being written, PR #2863 merged the handler, the metrics, and the secret wiring
to `main`, and someone registered the Mailgun webhooks. The state on the ground
diverges from both the issue body and the runbook comment.

**Verified live, 2026-08-08:**

| Check | Result |
|---|---|
| `POST https://dev.ripls.app/email/status` | **403** ✅ deployed, rejecting unsigned |
| `POST https://ripls.app/email/status` | **403** ✅ — prod deployed 21:10:13Z, revision `server-00136-ghj`. Was **200** (SPA catch-all) earlier the same day; see "the window that was open" below |
| `POST https://ripls.app/sms/status` | 403 (the shape to match) |
| Mailgun webhooks registered | **all 8 event types**, each pointing at **both** `ripls.app` and `dev.ripls.app` |
| Log metrics in `ripls-dev` | `email_delivery_latency_dev`, `email_delivery_failures_dev`, `email_code_age_dev`, `email_code_failures_dev`, `email_signins_by_credential_dev` |
| Log metrics in `ripls-prod` | **all five** ✅ — applied 2026-08-08; was none, including #2571's three |
| `mailgun-webhook-signing-key` dev vs prod | same value (SHA-256 prefix `f37ad726e6cfcaec` both) ✅ |
| Alert policies referencing any email metric | **none** |
| Serving dev revision | `server-dev-01165-mhb`, created 20:26:15Z — auto-deployed from `main` by `deploy_to_dev_on_push.yaml` |

**The window that was open, and is now closed.** The runbook comment said "do
not register before the endpoint is deployed," because `ripls.app/email/status`
answered 200 from the Flutter SPA catch-all, and Mailgun reads 200 as success —
no retry, no error, silent discard. Registration happened ahead of the prod
deploy, so for roughly a day production delivery events were discarded while the
webhook looked healthy from Mailgun's side. The 21:10Z deploy closed it: the
endpoint now returns 403 to an unsigned payload, matching `/sms/status`.

Nothing is recoverable from that window and nothing needs to be — the handler
records no state, so the cost was observability only, over a period when prod
email volume is small. Worth keeping as the reason the ordering in the runbook
was not arbitrary.

**What is still missing in prod is the measurement, not the handler.** Prod has
no email log-based metric at all — not #2862's two, and not #2571's three
either. The endpoint is now receiving events and logging them; nothing is
counting them.

## Problem Statement

Once #2571/#2864 removes password sign-in, a mailed one-time code is the only
way into an email account, so a code that arrives slowly or never is a
**lockout**. The issue's premise is that we could not measure time-to-inbox:
`server/email/mailgun.go` logs only the Mailgun API *accept* latency, and
`email_code_age_{env}` is a proxy that bundles delivery with human read-and-type
time and only exists for codes someone actually used.

The measurement gap itself is now closed — the handler works, and real signed
webhooks are landing. What remains is that **nothing alerts on the measurement,
and the registration is over-broad**. Restating the remaining problem precisely:

1. ~~**Prod is dark.**~~ **Resolved 2026-08-08.** The endpoint was undeployed
   and the metrics unapplied while the webhooks already pointed at prod, so
   every prod delivery event was discarded by the SPA catch-all. The 21:10Z
   release and the monitoring apply closed both halves.
2. **Nothing alerts.** The issue's scope explicitly named "a log-based metric +
   alert on delivery failure rate and p95 delivery latency for the `email_code`
   type specifically." The metrics landed; the alert never did. A metric no one
   is paged on is a dashboard, not an SLI.
3. **The `bounce_severity` fix is live in dev but unconfirmed.** Dev
   auto-deploys from `main` (`deploy_to_dev_on_push.yaml`), and revision
   `server-dev-01165-mhb` came up at 20:26:15Z carrying the rename (8a7234740).
   Every webhook event in the logs predates it: the most recent failed event
   (19:33Z, revision `01163-v8w`) still shows the corruption the rename fixed —
   `"severity": "ERRORpermanent"`, the log level and the bounce severity
   concatenated by Cloud Logging. The `severity_kind` label extractor in
   `log_metrics.tf` reads `jsonPayload.bounce_severity`, a field no event has
   yet been observed to carry. Nothing here needs building; it needs one failed
   event to confirm.
4. **Registration is over-broad.** All eight event types are registered, not the
   three the runbook specified. `accepted`, `opened`, `clicked`, and
   `unsubscribed` produce a billed Cloud Run request and a DEBUG log each, with
   no signal — and they arrive at *both* environments, so the volume doubles.
5. **No documentation.** `sms_notifications.md` documents the SMS status
   callback; there is no email counterpart, and `server/observability.md`'s
   metric and alert tables do not mention the new metrics. Per CLAUDE.md's
   freshness protocol this is a gap the nightly ratchet will otherwise open a PR
   against.

**The SMS analog was a weaker precedent than the issue implies — until it was
fixed.** As filed, #2569 had built the handler but **never its metric or
alert**; `terraform/modules/monitoring/` contained no reference to SMS at all,
and `sms_notifications.md` contradicted itself in three places, asserting in its
observability section that the metric and failure-rate alert "are defined in
ops/Terraform" while its own remaining-work list still asked for them.

**PRs #2871 and #2873 closed that gap** (2026-08-09): `sms_delivery_outcomes_{env}`
plus the `SMS Delivery Failures High` policy are now live in dev and prod, and
the doc is true in all three places. So SMS is a real template again rather than
an aspirational one — and more usefully, it is the template that has already
been through a failed apply, which is where the `condition_threshold`-not-PromQL
constraint on Phase 4 comes from.

**Contradiction worth naming:** the issue body scopes "A webhook endpoint under
`server/services/web/` verifying Mailgun's HMAC signature" as work to be done.
That is done. Anyone picking this issue up from the body alone will re-plan
already-merged code. The scope that remains is operational, not constructive.

## Context from Comments

One comment, from the maintainer (maintainer, 2026-08-07), is a webhook
registration runbook. Its substance and how reality diverged:

- **"Do not register before the endpoint is deployed."** The reasoning is
  correct and specific: 200 from the SPA catch-all is worse than 404, because
  Mailgun retries a 404 and eventually disables the webhook *visibly*, whereas
  200 silently succeeds forever. **This instruction was not followed** — the
  webhooks are registered and prod still returns 200.
- **Readiness gate: the endpoint is ready when it answers 403**, matching the
  already-deployed `/sms/status`. Dev passes this gate today; prod does not.
- **Register `delivered` / `failed` / `complained` only**, because the handler
  logs everything else at DEBUG and `opened`/`clicked` add volume with no
  signal. Reality: eight types are registered. Note also that Mailgun's webhook
  IDs are `permanent_fail` and `temporary_fail` — there is no `failed` ID — so
  the runbook's `-F "id=failed"` loop as written would not have worked. The
  `event-data.event` *value* is `failed`, which is what the handler switches on,
  so the handler is right and the runbook's registration snippet is the part
  that is wrong.
- **Three things the unit tests cannot cover**, because they sign payloads the
  same way the handler verifies them: the HMAC scheme, the
  `signature`/`event-data` envelope, and the `severity`/`reason` field names on
  `failed` events.

**All three are now resolved by live data, which closes the comment's open
questions.** Real dev logs show:

```
email delivered      email_code   delivery_latency_ms=713
email delivered      email_code   delivery_latency_ms=851
email delivered      email_code   delivery_latency_ms=6702
email delivery failed email_code  severity="ERRORpermanent" reason="generic"
                                  smtp_code=498
                                  smtp_message="No MX for sim.ripls.app, …"
```

- HMAC-SHA256 over (timestamp + token) with the webhook signing key: **correct**
  — real signed payloads verify.
- The `signature` / `event-data` envelope: **correct**.
- The custom-variable round-trip: **correct** — `delivery_latency_ms` is
  populated, which requires `ripls_sent_at_ms` to have survived the round trip,
  and `email_type=email_code` requires `ripls_email_type` to have.
- `severity` and `reason`: **correct field names** — the failed event produced
  `permanent` and `generic`. (They appear here mangled only because this
  revision predates the rename; the underlying values are right.)

One more thing the comment could not have anticipated: the dev log also carries
recurring `invalid signature; rejecting` WARNINGs. **These are benign.** The two
most recent (2026-08-08T20:08:43Z and 20:08:59Z) are the readiness probes run
while researching this plan — an unsigned `{}` body decodes cleanly and then
fails signature verification, which is the designed behavior. Earlier ones carry
`signing_key_configured: false` and come from revision `01162`, before the
secret was wired. There is no signature defect. It does mean any unauthenticated
scanner hitting `/email/status` produces a WARNING, which is worth knowing
before writing an alert over that message.

## Review-dimension notes

Checked against the `observability`, `security`, and `error_handling` review
prompts, since this is a public unauthenticated endpoint that writes to the
observability stream.

**What is already right, and should not be "improved" during this work:**

- **PII masking** — the only PII in the payload is the recipient address, and
  every log line masks it (`logging.MaskEmail` → `ma***@sim.ripls.app`).
  Matches the `MaskPhone` treatment in `sms_status.go`.
- **Reject-when-unconfigured** — `verifyMailgunSignature` returns false on an
  empty key, so a missing secret makes delivery metrics go dark rather than
  accepting forged events. This is the safer default and the opposite of what
  `sms_status.go` does (it logs a warning and *proceeds* when no auth token is
  configured). The email side is stricter; keep it.
- **Replay bounded** — the Mailgun signature never expires on its own, and the
  ±15-minute `mailgunMaxClockSkew` window is what stops a captured payload from
  being replayable forever. The same constant re-bounds the latency computation
  so clock disagreement cannot emit an absurd figure into the metric.
- **Body bounded** — `http.MaxBytesReader(w, r.Body, 1<<20)`.
- **Structured fields and `operation`** — every line carries
  `operation="EmailStatusWebhook"` plus `request_id` from the middleware, which
  is what makes the log-based metric filters possible.

**Worth deciding rather than discovering later:**

- **The endpoint is an internet-reachable WARNING-log generator.** Any
  unauthenticated POST of valid JSON produces
  `"email status webhook: invalid signature; rejecting"` at WARN. That is
  correct behavior — a real signature failure must be loud, since silent
  discard is the failure mode this endpoint exists to prevent — but it means a
  scanner can drive log volume at will. Two consequences for Phase 4: do not
  build an alert that fires on *count* of that message without a rate threshold,
  and expect a baseline of these entries in prod once the path is public and
  answering 403. (The ones in dev today are this plan's own readiness probes.)
- **`email delivery failed` at ERROR feeds `all_errors`.** Covered under
  Recommendation below; noting it here because the observability prompt's
  "Alerting Foundations" area is exactly where this interaction belongs.

## Potential Solutions

The remaining work is one coherent operational sequence, so the options are
about **sequencing and alert design**, not about architecture.

### Option A: Deploy-and-apply first, prune registration, then alert (recommended)

- **Approach:** Order the work by how much risk each step retires. (1) Confirm
  in dev — already running the `bounce_severity` fix — that `severity_kind`
  populates. (2) Cut a prod release so `ripls.app/email/status` answers 403, and
  apply the monitoring module to prod so the metrics exist. (3) Prune the Mailgun
  registration to the four signal-carrying event types. (4) Only then add alert
  policies, with thresholds informed by the latency distribution prod actually
  produces. (5) Document.
- **Pros:** Stops the live discard first. Alert thresholds get written against
  real prod data instead of three dev samples. Each step is independently
  verifiable with a one-line `curl` or `gcloud logging read`. Pruning after the
  prod deploy means no window where an event type is registered at a URL that
  discards it.
- **Cons:** Alerting — the part the issue names as scope — lands last, so the
  issue stays open through a release cycle. Prod delivery data between now and
  the release is lost (unrecoverable, but only observability).
- **Effort:** Small-to-medium. Most of it is deploy, `terraform apply`, and one
  new alert resource.

### Option B: Alert-first on dev, then promote to prod

- **Approach:** Write the alert policies now against the dev metrics that
  already exist, tune them in dev, then deploy prod and apply everything at once.
- **Pros:** Delivers the named scope soonest; the alert gets soak time before it
  can page anyone in prod.
- **Cons:** Dev has **three delivered events in fourteen days** (713ms, 851ms,
  6702ms). A p95 threshold tuned on that sample is tuned on noise — the single
  6702ms sample would set p95 nearly an order of magnitude above the other two.
  It also leaves prod discarding events for longer, since the deploy moves to
  the end. Tuning in an environment with no traffic is not tuning.
- **Effort:** Small, but the output is low-confidence.

### Option C: Unregister the prod URL now, re-register after deploy

- **Approach:** Immediately `DELETE` the `ripls.app/email/status` URL from all
  eight webhooks so prod events stop being silently accepted, deploy, then
  re-register the three that matter.
- **Pros:** Closes the "looks healthy, discards everything" state within
  minutes rather than within a release. Strictly restores the runbook's intended
  ordering.
- **Cons:** Mailgun webhook URLs are a per-event-type list; removing one URL
  from eight events and restoring three afterward is more API calls and more
  chances to leave the account in a half-configured state than simply deploying.
  And the practical difference is nil — the events are discarded either way; the
  only thing unregistering changes is whether Mailgun *knows* they were
  discarded. It optimizes for provider-side bookkeeping we do not read.
- **Effort:** Small, but it is motion without benefit unless the prod release is
  genuinely far off.

## Recommendation

**Option A**, with one amendment borrowed from C: if the prod release is more
than roughly a week out, do C's unregistration of the `ripls.app` URL as a
holding action. Otherwise skip it.

The reasoning is that the discard is real but cheap — it costs observability
only, and only for prod email volume that is currently small. What is *not*
cheap is writing a p95 lockout alert against three dev samples and then trusting
it. The whole point of this issue is that the sign-in code path deserves a
delivery number rather than judgement; an alert threshold picked from noise
substitutes a different kind of judgement while looking like a number.

Sequencing the dev confirmation first is deliberate and nearly free — it is now
one bounced send, since the build is already deployed. It is the only step that
proves `severity_kind` populates end-to-end, and that label is what separates a
hard bounce (someone is locked out) from a temporary deferral (Mailgun will
retry). If that extractor is wrong, the failure alert cannot distinguish the
two, and it is far better to learn that in dev. Note this has never been
observed working: every event in the logs predates the revision that emits
`bounce_severity`.

One thing to decide explicitly rather than inherit: **`email delivery failed`
logs at ERROR, and `enable_all_errors_alerting` defaults to `true`**, so once
prod deploys, every hard bounce fires the "Server Error Logged" alert to
ops@ripls.org and, in prod, to the GitHub auto-filer. `sms_status.go` sets the
same precedent (`logger.ErrorContext(ctx, "sms delivery failed")`), and the
project's standing guidance is to keep ERROR logs loud and fix the upstream
cause rather than silence the alert. Keep ERROR. But go in knowing that a single
typo'd address will open a GitHub issue, and that this is the intended behavior
rather than a surprise to be debugged later.

## Open Questions

- **Should the p95 latency alert be `email_code`-only, or all types with
  per-type thresholds?** — Recommendation: `email_code`-only for now. The issue
  names it specifically, and it is the only type where latency is an
  availability signal; a digest arriving two minutes late is invisible. Filtering
  to one label value also sidesteps the low-volume problem for every other type.
  Revisit if off-app notification delivery ever becomes latency-sensitive.
- **What p95 threshold?** — Recommendation: do not pick one until prod has a
  week of data; ship the failure-rate alert first, which needs no distribution
  tuning. If a placeholder is required to close the issue, 30s sustained 15 min
  is defensible — it is far above the 713–851ms typical and above the 6.7s
  outlier, so it fires on a genuine deliverability stall rather than on jitter.
  Whatever is chosen, add a minimum-volume guard in the spirit of
  `rpc_error_rate_min_errors_per_second`; without one, two slow messages in an
  empty hour will page.
  **Resolved 2026-08-15 (#2923, volume-floor half):** a minimum-sample floor of
  5 delivered codes per 10-minute window (COMPARISON_GT, so ≥6 required) was
  added as a second combiner-AND condition on the `email_code_delivery_latency`
  alert. The 10s threshold is unchanged; the threshold-revisit follow-up
  (once prod has a week of real data through the floor) remains open.
- **Failure-rate alert as a fraction or a count?** — Recommendation: **count**,
  not fraction, and only for `severity_kind="permanent"` on
  `email_type="email_code"`. There is no clean denominator — `email_delivery_
  failures` and `email_delivery_latency` are separate metrics and the latter only
  counts successes — so a fraction would require a ratio across two log-based
  metrics for very little gain. At current volume any permanent bounce on a
  sign-in code is worth a look; start at >0 in 30 min and raise it if it proves
  noisy.
- **Prune the four no-signal webhook registrations?** — Recommendation: yes,
  after the prod deploy. `accepted`/`opened`/`clicked`/`unsubscribed` are each a
  billed Cloud Run request and a DEBUG log at both URLs. Keep `delivered`,
  `complained`, `permanent_fail`, and `temporary_fail` — note that is **four**
  registration IDs, not the runbook's three, because Mailgun splits `failed`
  into two IDs even though both post `event: "failed"`.
- **Should `sim.ripls.app` join the reserved-domain skip list?** —
  Recommendation: no, not under this issue. Both dev failures came from that
  domain, and `server/email/undeliverable.go` covers only the RFC 2606/6761
  reserved names, so sends to it are genuinely attempted and genuinely bounce.
  But it is two events from what looks like manual testing, not a simulation
  seed — `sim.ripls.app` appears nowhere in `server/`, `e2e/`, or `scripts/`. If
  a simulation run ever adopts that domain it would poison the exact metric this
  issue builds, so it is worth a follow-up issue rather than silent scope creep.
- **Build the missing SMS delivery metric + alert in the same pass?** —
  **Resolved: yes, done separately in PR #2871.** The original recommendation
  here was to defer it, on the grounds that bundling would widen the Phase 2
  prod apply. Splitting it into its own PR keeps that property while still
  shipping the work, so the reasoning held and the conclusion changed. Two
  things came out of it that this issue depends on:
  - The classifier gap. Writing the SMS alert surfaced that **any** new policy
    whose condition name misses `gcpAlertsHandler`'s patterns degrades to
    `unknown`. #2871 adds the `/delivery\s*fail/i` pattern and pins the email
    condition string too, so Phase 4 inherits a working route instead of
    rediscovering the bug.
  - The metric shape. SMS gets one counter (successes and failures together)
    because Twilio offers no send timestamp to read back, which gives its
    failure fraction a joinable denominator. Email cannot copy that — its
    successes live in a latency distribution — which is the concrete reason
    the email failure alert should be count-based, as recommended above.
  - **The PromQL trap**, which is the expensive one. #2871's alert used PromQL,
    passed `validate`/`plan`/`fmt`, merged, and then **could not be created**:
    Cloud Monitoring rejects PromQL against a log-based metric that has no
    series yet, and log-based metrics do not backfill. #2873 rewrote it as a
    `condition_threshold`. Phase 4 must start from that shape — see the note on
    the phase itself. Worth naming the process lesson too: nothing in the
    local toolchain catches this, because the rejection is server-side at
    create time. The apply *is* the test.
  Both PRs are merged and applied to dev and prod; `sms_delivery_outcomes_{env}`
  and the `SMS Delivery Failures High` policy are live in both.
- **Does the low-volume reality undermine the SLI premise?** — Recommendation:
  name it, do not fix it. Three delivered events in fourteen days in dev, and
  prod send volume is small too. The failure alert is meaningful at any volume;
  the p95 alert genuinely is not until traffic arrives. Say so in the doc rather
  than shipping a percentile everyone quietly distrusts.

## Implementation Plan

### Phase 1: Confirm the label chain end-to-end in dev
**Goal:** Prove `bounce_severity` → `severity_kind` works before it matters in
prod. **No deploy needed** — dev auto-deployed the fix at 20:26:15Z; this phase
is pure verification against a build that is already running.

> **A second half of this chain was broken, and is now fixed.** The `severity_kind`
> label extractor in `email_delivery_failures_dev` was still reading
> `jsonPayload.severity` rather than `jsonPayload.bounce_severity` — dev's metric
> had been applied *before* the rename landed, so the label was capturing the
> corrupted concatenated value it was renamed to avoid. Found and corrected by
> the 2026-08-09 dev apply. So the chain had two stale links, the build and the
> extractor, and only the build was visible from the logs. Prod never had the
> stale version, having been applied after the rename.
>
> This does not make the phase unnecessary — it makes it more worth doing. The
> label has still never been *observed* populating end to end.

**Tasks:**
- [ ] Send a dev email to an address that hard-bounces; confirm the log shows
      `bounce_severity: "permanent"` and a clean `severity: "ERROR"` — not the
      concatenated `"ERRORpermanent"` visible today.
- [ ] Confirm `email_delivery_failures_dev` records a non-empty `severity_kind`
      label for that event.
- [ ] Confirm a `delivered` event still populates `delivery_latency_ms`.
**Validates:** The metric's most important label actually populates, and the
Cloud Logging key collision is genuinely fixed in a running build rather than
only in source.

### Phase 2: Ship prod and close the silent-discard window
**Goal:** `ripls.app/email/status` answers 403, and prod has the metrics.
**Tasks:**
- [x] Cut a release including #2862 and #2571's merged work. — **done
      2026-08-08 21:10:13Z**, revision `server-00136-ghj`.
- [x] Gate on the runbook's check: `curl -s -o /dev/null -w '%{http_code}' -X
      POST https://ripls.app/email/status -H 'Content-Type: application/json'
      -d '{}'` must return **403**, matching `/sms/status`. — **passes.** The
      one `invalid signature; rejecting` entry in prod at 21:13:32Z is that
      probe, not a real event.
- [ ] `terraform apply` the monitoring module for `ripls-prod`. Prod currently
      has **no** email metrics — this applies #2571's `email_code_age`,
      `email_code_failures`, `email_signins_by_credential` alongside #2862's
      `email_delivery_latency` and `email_delivery_failures`. Run
      `terraform plan` from `terraform/environments/prod` first and read the
      diff — prod monitoring has drifted well behind dev, so the plan may
      contain more than these five resources. (No doc prescribes `-target` for
      prod; the shared-state caution applies to feature-branch **dev** applies,
      not to applying `main` to prod.)

      **Planned 2026-08-08 — `terraform plan -target=module.monitoring` from
      `terraform/environments/prod` reports `5 to add, 5 to change, 0 to
      destroy`:**
      - *Add* — exactly the five expected metrics: `email_code_age`,
        `email_code_failures`, `email_signins_by_credential` (#2571) plus
        `email_delivery_latency`, `email_delivery_failures` (#2862).
      - *Change* — four dashboards (`db_health`, `go_runtime`, `rpc_errors`,
        `rpc_latency`), all cosmetic `targetAxis` / `xPos` / `yPos` normalization
        from the provider version rather than any intended edit.
      - *Change* — `module.containers.google_cloud_run_v2_service.server`,
        clearing `client = "gcloud"` and `client_version = "568.0.0"` to null.
        These are annotations the 21:10 `gcloud run deploy` stamped on the
        service; terraform wants them gone because they are not in config. All
        31 other attributes — image, env, secrets, scaling — are unchanged, so
        this does not roll a revision. It is the standing artifact of
        deploying by `gcloud` while managing by terraform, and it will reappear
        after every deploy.
      - *Destroy* — nothing.

      To avoid touching the service and the dashboards at all, target the five
      metrics directly instead of the whole module.
- [x] Verify `gcloud logging metrics list --project=ripls-prod | grep email`
      lists all five. — **applied 2026-08-08**; all five present
      (`email_code_age_prod`, `email_code_failures_prod`,
      `email_signins_by_credential_prod`, `email_delivery_latency_prod`,
      `email_delivery_failures_prod`). Applied as the whole module, so the
      dashboard drift and the Cloud Run annotation clear went with it.
- [ ] Confirm a real prod send produces `jsonPayload.operation=
      "EmailStatusWebhook"` with `"email delivered"` and a `delivery_latency_ms`.
- [ ] Confirm the environment filter works in the direction that matters: a dev
      send must **not** appear in prod's logs as anything but the DEBUG
      "ignoring email delivery event from another environment" line.
**Validates:** The end-to-end chain in the environment the SLI is for, and the
`ripls_env` filter under real dual-environment traffic — which cannot be
exercised anywhere else, since both URLs receive both environments' events.

### Phase 3: Prune the registration
**Goal:** Only signal-carrying events reach the endpoints.
**Tasks:**
- [ ] Remove `accepted`, `opened`, `clicked`, `unsubscribed` registrations for
      both URLs.
- [ ] Confirm `delivered`, `complained`, `permanent_fail`, `temporary_fail`
      remain, each pointing at both `ripls.app` and `dev.ripls.app`.
- [ ] Re-verify with `GET /v3/domains/ripls.app/webhooks`.
**Validates:** Request volume drops to the events that produce metrics, and the
`failed`-vs-`permanent_fail`/`temporary_fail` naming is settled in one place.

### Phase 4: Alert
**Goal:** The SLI pages someone.

> **Use `condition_threshold`, not PromQL.** This is the one thing to get right
> before writing a line, and it cost a merged-then-broken policy to learn.
> Cloud Monitoring validates a PromQL condition against the **Prometheus series
> catalog**, which a log-based metric only enters once it has received a data
> point — and log-based metrics do not backfill. `rpc_error_rate` gets away with
> PromQL because RPC traffic is constant. `email_delivery_failures_prod` has no
> series until a real bounce happens, so a PromQL policy is rejected outright
> with `Error 400: The following PromQL metric(s) are invalid`. A
> `condition_threshold` validates against the metric *descriptor*, which exists
> as soon as terraform creates the metric. #2873 converted the SMS policy for
> exactly this reason; `dependency_health_failures` is the older precedent.
>
> Two consequences follow. A native ratio threshold **cannot** express a
> minimum-volume floor in the same condition — the SMS policy makes the floor a
> second condition and sets `combiner = "AND"`. And applying to a fresh
> environment takes **two passes**: the descriptor has not propagated when the
> policy referencing it is created, `depends_on` does not help, and a re-run
> succeeds.

**Tasks:**
- [ ] Add a failure alert in `terraform/modules/monitoring/alerts.tf` on
      `email_delivery_failures_{env}` filtered to `email_type="email_code"` and
      `severity_kind="permanent"`, as a `condition_threshold` (see above).
- [ ] Add a p95 latency alert on `email_delivery_latency_{env}` filtered to
      `email_type="email_code"`, with a minimum-volume guard, after a week of
      prod data.
- [ ] Route both through the existing notification channels; prod inherits the
      `gcp-alerts-to-github` Pub/Sub channel automatically.
- [ ] **Name the condition so the classifier matches it.**
      `cloud_functions/src/gcpAlertsHandler.ts` routes on the condition display
      name, and an unmatched name degrades to `unknown` — a per-incident issue
      with no claude-fix, the state #2622 found two policies in. PR #2871 (the
      SMS work) added a `/delivery\s*fail/i` WARN pattern that covers the email
      policy too, and pinned **"Email delivery failures exceed 5% of sign-in
      codes"** in the classifier tests. Either use that exact string or update
      the test alongside; do not invent a name that misses the pattern. If the
      policy ends up with a floor condition as well, **both** condition names
      must match — either can be the one named on an incident.
- [ ] Add both rows to the alert table in `docs/server/observability.md`.
**Validates:** The issue's actual named scope — a delivery *number* with a
threshold, replacing the proxy-metric-plus-judgement the issue objects to.

### Phase 5: Document
**Goal:** The webhook is discoverable by the next person, and the context map
stays green.
**Tasks:**
- [ ] Add an `docs/email_notifications.md` (or an email section in an existing
      doc) covering the delivery webhook, the custom-variable stamping that
      makes latency measurable without a correlation table, the shared-Mailgun-
      account environment filter, and the registration runbook — mirroring what
      `sms_notifications.md` does for #2569.
- [ ] Add the two new metrics to `server/observability.md`'s metric table.
- [ ] Record the resolved unknowns from the issue comment: the HMAC scheme, the
      envelope, and the `severity`/`reason` field names are confirmed against
      real payloads. Update the comment block in `email_status.go` that still
      describes them as unverified.
- [x] Fix the self-contradiction in `sms_notifications.md`. — **done in PR
      #2871**, which also *built* the missing SMS metric and alert rather than
      only correcting the claim, so the doc is now true in the better
      direction. See "SMS parity" below.
- [ ] Add `context:` frontmatter and run `npm run generate:context-map`;
      confirm `npm run lint:context-map` passes.
**Validates:** The doc-freshness gate, and that `email_status.go`'s own comments
stop describing verified behavior as unverified.

## Related Issues

- **#2569** — the SMS status-callback work this is explicitly modeled on.
  `sms_status.go` is the shape reference for the handler and
  `sms_notifications.md` for the doc. Its metric and alert had never been built
  and its doc claimed otherwise; **PR #2871 builds both and corrects the doc**,
  so it is now a real template rather than an aspirational one.
- **#2571 / #2864** — passwordless email OTP. Phase 4 removes password sign-in
  after a release soak; this issue is what turns the pre-removal check from a
  proxy metric into a delivery number. Note that #2571's own metrics are also
  unapplied in prod, so Phase 2 unblocks both.
- **#2809** — GCP cost review. Relevant to Phase 3: four no-signal webhook event
  types at two URLs each are billed Cloud Run requests for nothing.
