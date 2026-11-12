# Issues #1148 + #1149: Auto-file GitHub issues from Crashlytics and Cloud Monitoring (combined plan)

**Source:** [#1148](https://github.com/Type2Fun-Foundation/ripls/issues/1148), [#1149](https://github.com/Type2Fun-Foundation/ripls/issues/1149)
**Filed:** 2026-04-16 | **Last activity:** 2026-04-25
**Labels:** none on either | **Assignees:** briangbrown
**Context docs:** prior PRs #1322 (closed — "needs more human design") and #1323 (closed — "needs more human design"), `.github/workflows/claude_plan_issue.yaml`, `.github/workflows/claude_implement_plan.yaml`, `.github/workflows/claude_refine_plan.yaml`, `terraform/modules/monitoring/main.tf`, `app/lib/core/observability/firebase/firebase_crash_reporter.dart`, `server/github/`, `server/services/feedback/`, `docs/server/observability.md`, `docs/server/architecture.md`
**Audit pass:** 2026-05-07 — see [Audit Notes](#audit-notes) for what changed and why.

## Problem Statement

Two production-signal channels currently terminate in email and stop:

1. **Crashlytics** (#1148) — fatal/non-fatal crashes, velocity spikes, regression alerts. Today: visible in the Firebase console; humans copy stack traces into `docs/crashlytics/<date>/` and file GitHub issues by hand. Doesn't scale; spikes go unacknowledged.
2. **Cloud Monitoring** (#1149) — `Server Error Logged`, uptime, RPC errors, DB CPU/memory/disk/connections. Today: fires email-only via the channel configured in `terraform/modules/monitoring/main.tf`. Volume drives email-fatigue; production incidents land in inboxes that don't get triaged.

These are different sources but share four design concerns:
- **Context gathering** — pulling the right structured fields out of the alert payload so the issue is actionable.
- **Plumbing** — somewhere to receive the alert, render an issue body, call the GitHub API.
- **Dedup** — one GitHub issue per underlying root cause, not one per notification.
- **Routing into our existing Claude workflows** — when a new issue is filed, optionally trigger `claude_plan_issue.yaml` so Claude drafts a fix plan automatically.

The user's explicit constraints:
- **Don't duplicate** the existing Claude planning workflows. Funnel into them.
- **Cost-conscious** — both infra cost and Claude-API cost. Don't auto-plan every flake.
- **Low noise** — filing 50 issues for one root cause is worse than email.
- **Keep email** as a backup signal during rollout; don't replace it.

## Context from Comments

**Bryan rejected both prior PRs (#1322, #1323) on 2026-04-25** with one-line "needs more human design." Notable comment on #1323: *"I don't like expanding our RPC server to handle this bug-filing automation. Is there not a way to do it through github automation, or firebase automation, or something?"* Claude's response on #1323 *did* pivot to Firebase Cloud Functions (removed the Go server changes), but Bryan still rejected it — design wasn't holistic enough across both sources.

Triage on both: **P3** (`/ripls-triage` 2026-04-24). Useful but not urgent.

This combined plan is the design pass Bryan asked for.

## What we already have to leverage

**Existing Claude workflows under `.github/workflows/`:**
- `claude_plan_issue.yaml` — fires on issue labeled `claude-fix`, or on issue body/title containing `@claude`, or on issue comment containing `@claude` (excluding self-triggered comments by `claude[bot]`). Drafts a plan PR.
- `claude_refine_plan.yaml`, `claude_implement_plan.yaml` — downstream stages.
- `claude_address_pr_feedback.yaml`, `claude_resolve_conflict.yaml`, `claude_fix_ci_failure.yaml` — orthogonal automations.

**This is the integration point.** The auto-filing layer's job is to file the right issue with the right label. Claude's existing workflow takes it from there. We do not write a new Claude workflow.

**Existing GitHub-issue plumbing in the Go server (and why we still go external for alerts):**
- `server/github/` is a shared library (`AppClient`, `CreateIssue`, etc.) consumed by `server/services/feedback/` to file user-submitted feedback as GitHub issues.
- Per `docs/server/architecture.md` §1–2, "shared functionality goes in libraries; services don't depend on services." If we filed alerts from the Go server, that library is where the helper would live.
- We are deliberately **not** doing that. Bryan's rejection of #1322/#1323 was specifically *"I don't like expanding our RPC server to handle this bug-filing automation."* The principled distinction: feedback is **request-driven** (a user RPC submits it, so the server is already on the path), while alerts are **event-driven from outside the server** (Firebase / GCP Monitoring publish them; routing them through Cloud Run adds load and a failure mode for an automation that exists to catch failures). External infra is the right home for event-driven automation.

**Existing GitHub App authentication (reused for the Cloud Function):**
- An organization-level GitHub App was set up by Thomas (`tescobar`) in late 2025 — see commits `abc4d4e2b` ("Replace GitHub Personal Access Token w/ Github App on the server"), `ca02b2a3c` ("Add terraform support for Github app"), `ffb751148` ("Working on dev issues related to Github app"). It already grants `issues: write` on `Type2Fun-Foundation/ripls` and is consumed by `server/services/feedback/`.
- Coordinates (per `terraform/environments/{dev,prod}/main.tf`):
  - **App ID:** `2840389`
  - **Installation ID:** `109363272`
  - **Private key:** GCP Secret Manager secret `github-app-private-key-base64` (same name in both `ripls-dev` and `ripls-prod`).
- The Cloud Function reuses this App rather than provisioning a new bot user / PAT. Concretely:
  - One secret rotation surface (rotate the App's private key once → both Go server feedback and Cloud Function alert-filer pick up the new key).
  - One bot identity in GitHub UI for everything auto-filed.
  - Bot username is the App's display name, not `claude[bot]` — no collision with `claude_plan_issue.yaml`'s self-trigger guard.
- TypeScript-side, the function uses Octokit's `createAppAuth` strategy from `@octokit/auth-app` to mint short-lived installation tokens at invocation time.

**What we don't have:**
- No existing Firebase Functions / Cloud Functions deployment for ripls (`app/firebase.json` is just Flutter plugin config; no `cloud_functions/` directory). This work introduces the first one.

**Existing infra for #1149:**
- `terraform/modules/monitoring/main.tf` defines all 8 alert policies, currently attached to `google_monitoring_notification_channel` of type `email` (`support@ripls.org` per `docs/server/observability.md` §Notification Channels for both dev and prod).
- Pub/Sub notification channels are a first-class option in `google_monitoring_notification_channel` — this is where `terraform/` will grow.

**Existing canonical log fields (per `docs/server/observability.md` §Standard Field Names):**
- `request_id` — HTTP correlation ID (X-Request-ID header or auto-generated UUID).
- `target_request_id` — ID of a `models.Request` entity an RPC operates on (do not confuse with `request_id`).
- `user_id` — authenticated user ID.
- `user_email` — masked via `logging.MaskEmail` (e.g., `al***@example.com`); never plaintext.
- `operation` — function/RPC name (also extracted by the `rpc_errors_<env>` log-based metric for alert routing).
- `duration_ms`, `error`.
- Entity-specific: `gear_id`, `community_id`, `transfer_id`, `location_id`, `media_id`.
- External-service: `external_service`, `provider`, `model`.
- The `health_check_unhealthy` alert specifically surfaces `dependency` (e.g., `"ai"`) and `backend` (e.g., `"anthropic"`); these should appear in the issue title/body and feed into label routing (see Phase 2).

The Cloud Function's `upsertIssue` body templates should expect this exact field naming and never project new field names that might collide with the canonical set.

## Architectural shape

**One Firebase Cloud Functions deployment.** Not two services, not a
separate Pub/Sub processor and a separate Crashlytics processor — one
TypeScript codebase deployed once, with multiple exported functions
that each register a different trigger type. Concretely:

- Crashlytics signals arrive via `firebase-functions/v2/alerts/crashlytics`
  triggers (`onVelocityAlertPublished`, `onRegressionAlertPublished`,
  `onStabilityDigestPublished`). Firebase manages the underlying Eventarc
  subscriptions automatically.
- Cloud Monitoring signals arrive via `firebase-functions/v2/pubsub`'s
  `onMessagePublished` trigger, subscribed to a Terraform-managed
  Pub/Sub topic that Cloud Monitoring's notification channel publishes to.

Pub/Sub is **just a bus** — the consumer is a Cloud Function in the same
codebase as the Crashlytics handlers. Cloud Functions v2 has first-class
Pub/Sub triggers; no separate worker, no Cloud Run service, no custom HTTP
endpoint. All trigger types share a single `upsertIssue` helper, a single
Octokit-from-GitHub-App auth path, a single set of deployed Cloud Run
revisions under the hood, and a single log stream. Two trigger types, one
deployable artifact.

The diagram below shows the data flow:

```
Crashlytics ──(onVelocityAlertPublished, onRegressionAlertPublished)──┐
                                                                       │
                                                                       ▼
                                                              ┌─────────────────┐
                                                              │ Cloud Functions │
                                                              │  (TypeScript)   │      shared upsertIssue()
Cloud Monitoring ──(notification_channel: pubsub)──> Topic ───┤                 ├──> GitHub REST API
   (8 alert policies)                                gcp-     │  - crashlytics  │     (label-based dedup)
                                                     alerts-  │  - gcpAlerts    │
                                                     to-      │  - upsertIssue  │            │
                                                     github   └─────────────────┘            │
                                                                                              ▼
                                                                                       ┌──────────────┐
                                                                                       │  GH issue    │
                                                                                       │ (with label) │
                                                                                       └──────┬───────┘
                                                                                              │
                                                                       label `claude-fix` ────┤
                                                                       (only when             │
                                                                        gating signal met)    ▼
                                                                                  claude_plan_issue.yaml
                                                                                  (existing workflow)
```

**Why this shape:**
- One deployment, multiple trigger types → shared `upsertIssue` helper, shared GitHub-App auth, one set of secrets to manage, one place to read logs.
- Cloud Functions v2 with `min-instances=0` → costs ~$0/mo at our volume; cold-start latency is irrelevant for batched alert delivery.
- Pub/Sub between Cloud Monitoring and the function (rather than Webhook) → durable, retried, dead-letter-queueable. Crashlytics triggers are already event-driven and managed by Firebase; no extra durability layer needed.
- Email channels stay attached to alert policies in parallel → if the Cloud Function breaks, we still see prod incidents.
- Issue filing emits a label (e.g., `auto-filed`) and **conditionally** the `claude-fix` label. Claude's existing workflow handles planning. We do not run Claude during the alert-handler itself.

## Potential Solutions

### Option A: Single Cloud Functions deployment, two triggers, shared helper (recommended)

- **Approach:** One `cloud_functions/` directory at the repo root with TypeScript + `firebase-functions/v2`. Two exported functions:
  - `crashlyticsHandler` — uses `onVelocityAlertPublished` + `onRegressionAlertPublished` triggers from `firebase-functions/v2/alerts/crashlytics`.
  - `gcpAlertsHandler` — uses `onMessagePublished` from `firebase-functions/v2/pubsub`, subscribed to a Terraform-managed topic `gcp-alerts-to-github`.
  - Shared `upsertIssue(label, title, body, options)` helper using Octokit.
- **Pros:** One deploy artifact, one set of secrets, shared helper avoids drift, shared logging. Bryan's "firebase automation" framing is satisfied. Crashlytics integration uses the official trigger SDK (no Firebase Console webhook hack). Cloud Monitoring side gets durability via Pub/Sub.
- **Cons:** Introduces a new build/deploy unit (TypeScript + Firebase CLI). New onboarding cost. Dual deploy mechanism for Cloud Monitoring — Terraform manages the topic + notification channel, Firebase deploys the consumer function (slightly awkward boundary, but standard).
- **Effort:** Medium (~1–2 days end-to-end, including secret setup, threshold tuning, doc).

### Option B: Single Cloud Run service with two HTTP endpoints

- **Approach:** Skip Firebase Functions entirely. Deploy a tiny stateless Go or TypeScript Cloud Run service with `/crashlytics` and `/cloud-monitoring` endpoints. Configure Crashlytics as Webhook → endpoint, Cloud Monitoring as Pub/Sub → subscription → push to endpoint, or as a Webhook channel direct.
- **Pros:** Reuses our existing Cloud Run muscle. Crashlytics webhook delivery is supported (though the v2 alert triggers are richer). One container to deploy.
- **Cons:** Crashlytics doesn't have native webhooks for v2 alert types — you'd be pulling from the legacy Firebase webhooks integration which is being deprecated. Loses the typed Cloud Functions SDK. More plumbing for less benefit.
- **Effort:** Medium. Comparable LOC, worse ergonomics on the Crashlytics side.

### Option C: Two separate deployments (one per source)

- **Approach:** Separate `cloud_functions/crashlytics/` (Firebase) and a parallel Terraform-deployed `google_cloudfunctions2_function` for GCP alerts. Each ships its own GitHub client.
- **Pros:** Strict separation of concerns. Each source can evolve independently. No shared dep tree.
- **Cons:** Two GitHub tokens to rotate, two `upsertIssue` implementations that will drift, two log streams to monitor. Bryan's reason for rejecting #1322 + #1323 was *lack* of holistic design — split deployments push against that.
- **Effort:** Medium-large.

### Option D: Third-party (Sentry / PagerDuty / Opsgenie)

- **Approach:** Adopt a third-party error tracker that has native GitHub integration. Sentry replaces Crashlytics; PagerDuty / Opsgenie can route Cloud Monitoring incidents to GitHub.
- **Pros:** No custom code. Mature lifecycle UIs.
- **Cons:** New vendor cost (Sentry isn't free at our volume). Migrating away from Crashlytics is significant. Doesn't solve Cloud Monitoring without a second vendor. Doubles up on the existing observability spend.
- **Effort:** Large; multi-week migration with no business-feature payoff.

## Recommendation

**Option A.** It's the smallest holistic answer and directly addresses Bryan's two objections:

- *"Don't expand the Go server"* — handler lives in Firebase Functions, server stays unchanged.
- *"Through firebase automation, or something"* — uses the official Firebase Cloud Functions v2 Crashlytics triggers, plus standard GCP Pub/Sub for the monitoring side. No bespoke webhook routing.

The shared-helper approach also lets us land Phase 1 (Crashlytics only) and Phase 2 (Cloud Monitoring) as separate PRs while sharing 80% of the code, which is the right cadence for production automation.

## Testing strategy

The handlers are split into pure logic (testable directly) and thin trigger
wrappers (not unit-tested — they're three lines of glue). This is deliberate
so the bulk of the surface area is verifiable without spinning up Firebase
or making real GitHub API calls.

**Unit tests (vitest, all green at PR merge time):** ~36 cases stubbing
`Octokit` with hand-rolled mocks. Cover:

- `upsertIssue` dedup branches (no match → create, open match → comment,
  closed match + reopenIfClosed → reopen, closed match + reopenIfClosed
  false → comment-only, defensive PR filter).
- Threshold edges (`shouldClaudeFixVelocity` at, above, and below 1%).
- Body-formatting fixtures (`buildVelocityIssueBody`, `buildRegressionIssueBody`,
  `buildStabilityDigestBody`) asserting all canonical fields make it into
  the rendered Markdown including the dedup label echo.
- `isoMondayOfWeek` edge cases (Monday, Tuesday, Sunday, year boundary,
  invalid input).
- Platform inference (Android title → `platform:android`, iOS title →
  `platform:ios`, ambiguous → no platform label).
- Multi-day digest rollup (same week → comment on existing; new week →
  fresh issue with distinct dedup label).
- `decodeBase64Pem` round-trip and rejection of non-PEM payloads.

**Integration tests: none for Phase 1, intentionally.** End-to-end coverage
would require a real Firebase project with a real velocity alert firing,
which can't be reasonably scripted (it depends on real traffic crossing
the threshold). The cost-vs-value isn't there. Phase 3 may add a scheduled
Cloud Function polling Crashlytics REST APIs for resolved-issue auto-close;
that has its own integration-test surface and we'll address it then.

**Manual smoke:**

1. After deploy, verify both handlers register as Cloud Functions:
   `gcloud functions list --project=ripls-prod --gen2 --filter="name~ripls-alerts"`.
2. Sample logs: `gcloud functions logs read crashlyticsVelocityAlert --project=ripls-prod --gen2 --limit=10`.
   Empty is expected until a real alert fires.
3. End-to-end: wait for the next real velocity alert (typically days, not
   weeks) and verify a GitHub issue lands with the right labels, the right
   body, and that `claude_plan_issue.yaml` fires when `claude-fix` is set.

**Local emulator (optional, for debugging only):** the Firebase Functions
Emulator Suite (`firebase emulators:start --only functions`) can drive
handlers against fixture payloads matching the
`type.googleapis.com/google.events.firebase.firebasealerts.v1.*` shapes.
Useful when iterating on body templates without a deploy round-trip; not
load-bearing for correctness because the unit tests already cover the
pure logic.

## Cost and noise control (the load-bearing design choice)

This is what made the prior PRs feel under-designed. Pulling it out of the Implementation Plan as a first-class section:

### Issue-creation gating

Not every alert should become a GitHub issue. Filter at the source-of-truth boundary inside the Cloud Function:

| Source | Trigger | Default behavior |
|---|---|---|
| Crashlytics | `onVelocityAlertPublished` | File issue. These are by definition trending, signal-rich. |
| Crashlytics | `onRegressionAlertPublished` | File issue (or reopen existing). Regressions are high-signal. |
| Crashlytics | `onStabilityDigestPublished` | File a **rolling weekly issue** dedup'd by `crashlytics-digest-week:<isoMonday>`. First digest of the week creates with `claude-fix`; subsequent days append comments. Captures the trending-issues email signal in tracker form. |
| Crashlytics | `onNewFatalIssuePublished` | **Drop by default.** Wait for it to trend. Re-enable in Phase 2 if signal is sparse. |
| Crashlytics | `onNewNonfatalIssuePublished` | **Drop by default.** Volume is too high. Re-enable per-issue type only after Phase 2. |
| Cloud Monitoring | `state=open` | File issue. |
| Cloud Monitoring | `state=open` for the *same* `incident_id` we already filed | Comment with new occurrence count, don't dup. |
| Cloud Monitoring | `state=closed` | Close the GitHub issue with a "resolved" comment. |
| Cloud Monitoring | `state=open` for a policy with `severity:warn` (e.g., `all_errors`) firing > N times in M minutes | **Aggregate** — single issue per `policy_name` per day, not per `incident_id`. |

The aggregation rule on the last row is the cheap noise filter: a flapping `Server Error Logged` alert that fires 50× / hour shouldn't create 50 issues. We'd dedup on `gcp-policy:<policy>-<yyyymmdd>` instead of `gcp-incident:<id>` for the warn-severity policies and append occurrences as comments.

### Claude-trigger gating (the Claude-API cost lever)

Filing an issue is cheap. Triggering `claude_plan_issue.yaml` runs a real LLM and costs real money. The default for auto-filed issues should be **don't trigger Claude**. Add the `claude-fix` label only when one of these is true:

- Crashlytics velocity alert with `crashPercentage >= 1%` (large-blast-radius regressions get a draft fix automatically).
- Crashlytics regression alert (we already had a fix once; the diff context makes Claude more likely to land another).
- Crashlytics stability digest on the **first digest of the week** only (one Claude plan per weekly tracking issue, not one per day; subsequent days append comments without re-firing Claude).
- Cloud Monitoring incident on `database_*` policies (DB ops are slow to triage manually; Claude can usually narrow the cause).

Other auto-filed issues land with `auto-filed` only — humans triage them as a batch and can add `claude-fix` themselves if they want a draft. This keeps Claude-API spend bounded by manual escalation, which is the policy that scales.

### Bytes-on-the-wire and infra cost

- Cloud Functions v2 with `min-instances=0`: invoke-only billing. At our alert rate (low tens / day), monthly cost is < $1.
- Pub/Sub topic + subscription: < $0.50 / mo at this volume.
- GitHub API: free (we're well under the 5000 req/hr authenticated-user limit).
- Firebase secrets: free.
- Total monthly infra delta: rounding error.

## Open Questions

- **Q1: One GitHub identity or two (Crashlytics vs Cloud Monitoring)?** — Recommendation: **one identity, the existing GitHub App.** Don't provision a new bot user or new App. Reuse the App that `server/services/feedback/` already authenticates with (App ID `2840389`, Installation ID `109363272`, private key in GCP Secret Manager `github-app-private-key-base64`). One rotation surface, one attribution string in the GitHub UI, no naming collision with `claude[bot]`. The Cloud Function reads the same Secret Manager secret and uses Octokit's `createAppAuth` to mint installation tokens. *(Original Phase 1 plan called for a fine-grained PAT under a new bot user; revised after discovering the existing App.)*
- **Q2: Where does the Cloud Functions directory live?** — Recommendation: top-level `cloud_functions/` (sibling of `app/`, `server/`). Reasons: (a) it's not Flutter (so not in `app/`), (b) it's not Go (so not in `server/`), (c) it's not infrastructure-as-code (so not in `terraform/`), (d) Firebase tooling expects a project-local layout (`firebase.json` already lives in `app/` for Flutter; we add a separate `firebase.json` inside `cloud_functions/` for the Functions project so the two configs don't collide). Bryan's review feedback on PR #1762 prompted the rename from the original `functions/` — too generic for a top-level directory.
- **Q3: Crashlytics auto-close on resolution?** — Recommendation: **defer to Phase 3.** Crashlytics doesn't fire a "resolved" alert event. Auto-close requires a scheduled function polling the Crashlytics REST API. Worth doing eventually but not load-bearing for the primary value (visibility + triage). Phase 1/2 ships without it; humans close GitHub issues when fixes ship.
- **Q4: Are aggregated `severity:warn` issues opened/closed within a day, or kept as ratchet trackers?** — Recommendation: opened on first occurrence of the policy on a given day, **closed automatically at end-of-day (UTC)** by a scheduled Cloud Function that walks open `gcp-policy:*` labels older than 24h and either closes them (no fresh comments) or carries them over to today's issue. Prevents stale ratchet trackers but keeps a daily summary that captures cumulative load.
- **Q5: Issue body format — minimal or rich?** — Recommendation: rich, but bounded. For Crashlytics: title, app version, crash %, stack trace (top 20 frames), Firebase console link. For Cloud Monitoring: policy, condition, summary, masked structured fields (`request_id`, `user_id`, `error_message`), Cloud Logging link with the same `request_id`. Skip raw payload dumps. Keep under ~5 KB.
- **Q6: PII in issue bodies?** — Recommendation: same rule as `docs/server/observability.md` §Sensitive Data Protection — pre-masked at log time via `logging.MaskEmail` (`alice@example.com` → `al***@example.com`) and `logging.MaskToken` (first 8 chars only), with `RedactedEmail` / `RedactedToken` `slog.LogValuer` types covering the rest. The Cloud Function should additionally **never** include `user_email` or any phone field, and should treat any `*_email` field as masked-or-drop as defense-in-depth. Tests at the function level should assert the redaction list against representative payloads.

- **Q9: Issue-filing bot identity vs `claude_plan_issue.yaml` self-trigger guard?** — `claude_plan_issue.yaml` skips events where `github.actor == 'claude[bot]'` to prevent re-fires when Claude itself comments. We're reusing the existing GitHub App (set up by Thomas in 2025 — see "Existing GitHub App authentication" above), and its bot identity is not `claude[bot]`, so the guard lets the auto-filer through. No new bot needed. *(Original Phase 1 plan was to provision a new bot named `ripls-alert-bot`; obsoleted by the existing-App reuse.)*
- **Q7: Dev environment coverage?** — Recommendation: prod only for Phase 1–2. Dev incidents are dominated by deploy-churn and don't merit a permanent issue. Reconsider in Phase 4 if we miss real bugs.
- **Q8: Both auto-filing systems need to know about each other for cross-source dedup?** — Recommendation: **no.** A Crashlytics crash and a Cloud Monitoring server error that share a root cause will land as two issues. Manual labeling links them. Cross-source dedup is a real-but-not-load-bearing cleanup; defer indefinitely.

## Implementation Plan

### Phase 1: Crashlytics → GitHub (high-signal triggers only)

**Goal:** Velocity and regression alerts land as GitHub issues with the right labels. Validate the deployment shape, the GitHub-token model, and the dedup pattern before adding the second source.

**Tasks:**
- [ ] Create top-level `cloud_functions/` directory: `package.json`, `tsconfig.json`, `firebase.json` (Functions-only), `.firebaserc`, `src/index.ts`, `src/upsertIssue.ts`, `src/__tests__/upsertIssue.test.ts`.
- [ ] Implement `crashlyticsHandler` exporting two functions: `onVelocityAlertPublished` and `onRegressionAlertPublished`. Each parses the alert payload, calls `upsertIssue`, and exits.
- [ ] Implement `upsertIssue(label, title, body, { reopenIfClosed, addClaudeFixLabel })`:
  - GET `/repos/.../issues?state=all&labels=<label>&per_page=1`. The `crashlytics:<id>` label is unique → exactly 0 or 1 result.
  - If found and open → POST a comment with new stats.
  - If found and closed → reopen + comment (regression case).
  - If not found → POST new issue with all labels including `auto-filed`, `crash`, platform label, and conditionally `claude-fix`.
- [ ] Apply the Claude-trigger gating from the Cost section: `claude-fix` only when `crashPercentage >= 1%` for velocity, or always for regression.
- [ ] Reuse the existing GitHub App (App ID `2840389`, Installation ID `109363272`) — do not provision a new bot or new App. The Cloud Function authenticates via Octokit's `createAppAuth` reading the private key from GCP Secret Manager (`github-app-private-key-base64`, same secret consumed by the Go server's feedback service). Document setup in `docs/server/observability.md` (new section "Auto-filed Crashlytics issues" — same doc that already covers Cloud Monitoring alerts) with: how to rotate the App's private key, how to disable the function in an emergency, how to re-enable.
- [ ] Grant the Cloud Functions runtime service account `roles/secretmanager.secretAccessor` on the `github-app-private-key-base64` secret in `ripls-prod` (and `ripls-dev` if Phase 4 ships). The default Cloud Functions runtime SA is `<project-number>-compute@developer.gserviceaccount.com` unless overridden — Terraform-managed grant rather than ad-hoc gcloud.
- [ ] Set up GitHub Actions deploy authentication via **Workload Identity Federation**, not a service-account JSON key. The org policy `iam.disableServiceAccountKeyCreation` blocks key creation entirely; WIF is the supported alternative. Concretely: a `github-actions` Workload Identity Pool in `ripls-prod`, an OIDC provider scoped to `Type2Fun-Foundation`, and a `roles/iam.workloadIdentityUser` binding on `cloud-functions-deployer@ripls-prod.iam.gserviceaccount.com` for the `attribute.repository/Type2Fun-Foundation/ripls` principal. The deploy workflow uses `google-github-actions/auth@v2` with the provider/SA hardcoded (these are identity pointers, not secrets).
- [ ] Configure velocity alert threshold in Firebase Console (initial: affects ≥ 1% of sessions in 1 hour). Document the chosen threshold and the rationale in `docs/server/observability.md` so future tuning has a record.
- [ ] Add unit tests for `upsertIssue`: dedup hit, dedup miss, regression-reopen path, redaction list. Use `nock` or `msw` to mock GitHub API.
- [ ] Deploy from a GitHub Actions workflow on push to `main` for `cloud_functions/**` (new `.github/workflows/deploy_cloud_functions.yaml`). Cite the existing workflow style.
- [ ] Manual smoke: force a velocity alert by surfacing a known-fatal error in dev, verify a GitHub issue is created with the right body and `claude-fix` label, verify `claude_plan_issue.yaml` fires and drafts a PR.

**Validates:** End-to-end Crashlytics → issue → Claude plan flow works. Dedup on issue ID prevents duplicates as the spike evolves. Cost is rounding-error.

### Phase 2: Cloud Monitoring → GitHub (Pub/Sub channel + same Cloud Function)

**Goal:** All 8 prod alert policies land as GitHub issues with proper open/closed lifecycle. Aggregation rule on warn-severity policies prevents flap noise.

**Tasks:**
- [ ] In `terraform/modules/monitoring/`: add `google_pubsub_topic.gcp_alerts_to_github` and `google_monitoring_notification_channel.pubsub_github`. Wire the new channel into all 8 `google_monitoring_alert_policy` resources alongside the existing email channel.
- [ ] In `cloud_functions/`: add `gcpAlertsHandler` using `onMessagePublished` on the topic. Parse the [Cloud Monitoring incident JSON schema](https://cloud.google.com/monitoring/support/notification-options#pubsub) into a typed shape.
- [ ] Branch on `state` (`open` / `closed`). Branch on policy severity for label/dedup-key selection (`gcp-incident:<id>` vs `gcp-policy:<policy>-<yyyymmdd>`).
- [ ] Severity classifier: derive `severity:page` for `uptime_failure`, `health_check_unhealthy`, `database_*`; `severity:warn` for `rpc_errors`, `all_errors`. Document mapping. For `health_check_unhealthy` specifically, surface the alert's `dependency` and `backend` labels (per `docs/server/observability.md` §Health Check Unhealthy) into both the issue title and a `dep:<dependency>` label so flapping `ai`/`anthropic`-vs-`gemini` failures are dedup'd correctly and stay distinguishable from `database` failures.
- [ ] Apply the Claude-trigger gating: `claude-fix` only on `database_*` policies and on first-occurrence-of-the-day for warn policies.
- [ ] Apply prod-only for Phase 2 (only attach the channel in `terraform/environments/prod/main.tf`; not dev).
- [ ] Apply PII redaction in the body — never include any `*_email` or phone field, even if present in `resource.labels`.
- [ ] Schedule a daily Cloud Function (Cloud Scheduler → Pub/Sub → function) that closes any open `gcp-policy:*` issue older than 24h. Lives in the same `cloud_functions/` deployment.
- [ ] Add unit tests covering `state=open` create, `state=open` dup-comment, `state=closed` close, severity-based label routing, daily-close sweep.
- [ ] Documentation: extend Phase 1's section in `docs/server/observability.md` (extend the existing "Cloud Monitoring & Alerting" section already in that doc; do not create a new top-level alerting doc — `observability.md` is the single canonical home).
- [ ] Manual smoke: force an `all_errors` alert in prod via a controlled rollout, verify the issue lifecycle (open → comment → close).

**Validates:** Full monitoring coverage, with noise control. Pub/Sub durability proven (force a function deploy with the topic still active and verify backfill on cold start).

### Phase 3: Stale-issue close via GitHub Action + Phase 1/2 noise tuning

**Status:** Deferred until ~2 weeks of Phase 1 + 2 production signal exists. Tuning thresholds without observed data is a guess; auto-close may not even be needed if volume stays low. Revisit 2026-05-21.

**Reframe from the original plan:** the original plan called for a Cloud Function polling the Crashlytics REST API. That API doesn't actually exist as a public surface — Firebase Crashlytics exposes alert *triggers* (no resolution event), an admin API (no issue listing), and a BigQuery export (heavyweight, ongoing query cost). The clean alternative is to do the close logic in a **GitHub Action** that filters by the `auto-filed` label set we already stamp.

**Why GitHub Actions is the better fit:**
- Same trust boundary as `claude_plan_issue.yaml` (existing GitHub App identity, no new secrets, no new IAM, no Cloud Function deploy).
- Workflow YAML is in the repo — visible, reviewable, easy to iterate.
- Decouples close logic from any specific Crashlytics API surface — the Action can re-query whatever signal makes sense (Firebase REST, BigQuery, or pure heuristics on GH issue activity) without redeploying a Cloud Function.
- Trivial to filter — every auto-filed issue carries `auto-filed` plus a unique dedup label, so a simple `is:issue is:open label:auto-filed label:crashlytics:abc123` query returns the live set.

**Label inventory the Action can rely on** (all stamped by `upsertIssue` in `cloud_functions/src/upsertIssue.ts` and the per-source handlers; canonical list, do not drift):
- `auto-filed` — universal marker
- Dedup labels — `crashlytics:<id>`, `gcp-incident:<id>`, `gcp-policy:<slug>-<yyyymmdd>`, `crashlytics-digest-week:<isoMonday>`
- Severity labels — `severity:page`, `severity:warn`, `severity:unknown`
- Content-type labels — `crash`, `regression`, `stability-digest`, `ops`
- Platform/dep labels — `platform:android`, `platform:ios`, `dep:<dep>`, `backend:<backend>`
- Optional `claude-fix` (per gating)

**Tasks (when we revisit):**
- [ ] Add `.github/workflows/sweep_auto_filed_issues.yaml` — scheduled (e.g., daily at 09:00 UTC) workflow that lists open issues with `auto-filed` + `crashlytics:*` labels and closes any with no comment activity in the last **14 days**. Comment body: *"Auto-closed: no firings in 14 days. Reopen if it returns; the next Crashlytics velocity alert will create a fresh issue with the same dedup label anyway."*
  - Comment-on-each-firing is already the dedup pattern → comment silence is a strong proxy for "Crashlytics hasn't seen this fire again."
  - Use `gh` CLI inside the workflow (already available in GHA) — no Octokit, no auth secret beyond the default `GITHUB_TOKEN`.
- [ ] Same workflow extends to `gcp-incident:*` if needed. (`gcp-policy:*` is already daily-swept by `dailyGcpPolicySweep` in the Cloud Function — no GHA cleanup needed there.)
- [ ] Tune velocity threshold based on Phase 1 noise. Tune Cloud Monitoring aggregation window / classifier patterns based on Phase 2 false-positive rate.
- [ ] Add a noise-budget summary: workflow that comments on a single tracking issue weekly with the count of `auto-filed` issues opened in the last 7 days, broken down by severity. Self-throttle alarm if any week exceeds N.

**Out of scope (rejected from original plan):**
- ~~Cloud Function polling Crashlytics REST API~~ — the API doesn't exist as a public surface.
- ~~BigQuery export approach~~ — heavyweight; query cost; not justified at our volume.

**Validates:** Lifecycle is closed-loop on the Crashlytics side without a heavyweight Crashlytics-API integration. Volume stays bounded with visible noise-budget tracking.

### Phase 4: (optional) Dev environment coverage and `onNewNonfatalIssuePublished`

**Goal:** Extend coverage if Phase 1–3 signal proves valuable. Skip if not.

**Tasks:**
- [ ] Attach `pubsub_github` channel to dev alert policies; tag dev issues with `env:dev` label and `severity:debug` so they don't compete with prod triage.
- [ ] Add `onNewNonfatalIssuePublished` with a strict allow-list of issue types (e.g., specific exception classes we care about) — no broad enable.
- [ ] Cross-source dedup if a real pattern emerges (e.g., a Crashlytics non-fatal that maps 1:1 with a server `all_errors`).

**Validates:** Coverage is right-sized; we're not paying noise tax for a long-tail signal that doesn't drive action.

## Production setup commands (executed 2026-05-07 against `ripls-prod`)

Captures the exact commands run during Phase 1 setup so future contributors
have a runbook (and so we can replay against `ripls-dev` in Phase 4 by
substituting the project ID). Authenticated as `user3@example.org` (org
admin). Project number for `ripls-prod`: `507242812633`.

### 1. Enable required APIs

```zsh
gcloud services enable \
  cloudfunctions.googleapis.com \
  eventarc.googleapis.com \
  run.googleapis.com \
  cloudbuild.googleapis.com \
  artifactregistry.googleapis.com \
  firebaseextensions.googleapis.com \
  cloudbilling.googleapis.com \
  cloudscheduler.googleapis.com \
  --project=ripls-prod
```

`run`, `cloudbuild`, `artifactregistry` were already enabled for the
server; `gcloud services enable` is idempotent.

**`firebaseextensions.googleapis.com` and `cloudbilling.googleapis.com`
were added during the Phase 1 deploy** — Firebase CLI checks both
during `firebase deploy --only functions`. The deploy SA does not have
permission to enable them itself (no `serviceUsageAdmin` role), so a
human admin enables them upfront. Once enabled they stay enabled.

**`cloudscheduler.googleapis.com` was added during the Phase 2 deploy**
— `dailyGcpPolicySweep` uses `firebase-functions/v2/scheduler.onSchedule`
which provisions a Cloud Scheduler job under the hood. Same shape as
the others: deploy SA can't enable it, human admin pre-enables, stays
enabled.

### 2. Grant Cloud Functions runtime SA access to the GitHub App key

Added to `terraform/environments/prod/main.tf`:

```hcl
data "google_project" "current" {
  project_id = var.project_id
}

resource "google_secret_manager_secret_iam_member" "functions_github_app_private_key" {
  project   = var.project_id
  secret_id = "github-app-private-key-base64"
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${data.google_project.current.number}-compute@developer.gserviceaccount.com"
}
```

Applied surgically without touching the rest of the plan:

```zsh
cd terraform/environments/prod
terraform apply -target=google_secret_manager_secret_iam_member.functions_github_app_private_key
```

### 3. Create the GitHub Actions deploy service account + grant 9 project-level roles

```zsh
gcloud iam service-accounts create cloud-functions-deployer \
  --display-name="Cloud Functions deployer for GitHub Actions" \
  --project=ripls-prod

SA=cloud-functions-deployer@ripls-prod.iam.gserviceaccount.com
for role in \
  roles/cloudfunctions.admin \
  roles/iam.serviceAccountUser \
  roles/cloudbuild.builds.builder \
  roles/artifactregistry.writer \
  roles/eventarc.developer \
  roles/firebase.admin \
  roles/storage.admin \
  roles/resourcemanager.projectIamAdmin \
  roles/cloudscheduler.admin; do
  gcloud projects add-iam-policy-binding ripls-prod \
    --member="serviceAccount:${SA}" \
    --role="$role" \
    --condition=None
done
```

(Note: org policy `iam.disableServiceAccountKeyCreation` blocks
`gcloud iam service-accounts keys create`. Authentication uses Workload
Identity Federation in step 5 below — no JSON keys ever generated.)

**`roles/resourcemanager.projectIamAdmin` was added during the Phase 1
deploy.** Firebase CLI v2 deploys grant project-level IAM bindings to
service agents (Eventarc, Pub/Sub, Cloud Functions admin robot) as
part of trigger setup; without `setIamPolicy` on the project, the
deploy fails with *"We failed to modify the IAM policy for the
project"*. This is an expected requirement, not an over-permission —
the deploy must be able to wire the trigger plumbing.

**`roles/cloudscheduler.admin` was added during the Phase 2 deploy.**
The `dailyGcpPolicySweep` function uses `onSchedule`, which provisions
a Cloud Scheduler job per-function on each deploy. Without
`cloudscheduler.jobs.update` (covered by `cloudscheduler.admin`), the
deploy fails with `HTTP Error: 403, The principal lacks IAM permission
"cloudscheduler.jobs.update"`. Required as long as any `onSchedule`
trigger is part of the codebase.

### 4. Grant deploy SA read access to the GitHub App key secret

The deploy SA needs to **inspect** the secret at deploy time (verify it
exists, read metadata) when binding it to the function via
`defineSecret`. The runtime SA's grant in step 2 only covers payload
access, not metadata reads.

```zsh
SA=cloud-functions-deployer@ripls-prod.iam.gserviceaccount.com
gcloud secrets add-iam-policy-binding github-app-private-key-base64 \
  --member="serviceAccount:${SA}" \
  --role="roles/secretmanager.secretAccessor" \
  --project=ripls-prod
gcloud secrets add-iam-policy-binding github-app-private-key-base64 \
  --member="serviceAccount:${SA}" \
  --role="roles/secretmanager.viewer" \
  --project=ripls-prod
```

Both roles are needed — `secretAccessor` covers `versions.access` (read
the payload) but **not** `secrets.get` (read metadata), which the
deploy CLI calls during binding. `secretmanager.viewer` covers the
metadata reads. Deferring to a future cleanup whether to migrate this
into Terraform alongside the runtime SA grant.

### 5. Workload Identity Federation for GitHub Actions

```zsh
# Pool
gcloud iam workload-identity-pools create github-actions \
  --location=global \
  --display-name="GitHub Actions" \
  --project=ripls-prod

# OIDC provider scoped to the Type2Fun-Foundation org
gcloud iam workload-identity-pools providers create-oidc github \
  --location=global \
  --workload-identity-pool=github-actions \
  --display-name="GitHub" \
  --issuer-uri="https://token.actions.githubusercontent.com" \
  --attribute-mapping="google.subject=assertion.sub,attribute.repository=assertion.repository,attribute.repository_owner=assertion.repository_owner" \
  --attribute-condition="assertion.repository_owner=='Type2Fun-Foundation'" \
  --project=ripls-prod

# Bind the deploy SA so only Type2Fun-Foundation/ripls can impersonate it
gcloud iam service-accounts add-iam-policy-binding \
  cloud-functions-deployer@ripls-prod.iam.gserviceaccount.com \
  --role="roles/iam.workloadIdentityUser" \
  --member="principalSet://iam.googleapis.com/projects/507242812633/locations/global/workloadIdentityPools/github-actions/attribute.repository/Type2Fun-Foundation/ripls" \
  --project=ripls-prod
```

Resulting WIF identifier (hardcoded in
`.github/workflows/deploy_cloud_functions.yaml`, not a secret):

```
projects/507242812633/locations/global/workloadIdentityPools/github-actions/providers/github
```

### 6. Create the `prod` GitHub Environment

Required so the `environment: prod` line in the deploy workflow resolves:

```zsh
gh api -X PUT "/repos/Type2Fun-Foundation/ripls/environments/prod" --input - <<<'{}'
```

No protection rules added in Phase 1. To require manual approval before
deploys (recommended once the pipeline is proven, optional now):

```zsh
gh api -X PUT "/repos/Type2Fun-Foundation/ripls/environments/prod" \
  --input - <<<'{"reviewers":[{"type":"User","id":<gh-user-id>}]}'
```

### 7. Configure Crashlytics velocity alert threshold

UI-only step (no `gcloud` equivalent). Firebase Console → `ripls-prod` →
Crashlytics → ⚙ Settings → Velocity Alerts → enable, threshold:

- **Affected users:** 25
- **Affected sessions:** 1% in 1 hour

Repeated for both Android and iOS apps. Stability digest is on by default
and emails to `support@ripls.org`; nothing to configure beyond ensuring
the email channel exists.

### 8. First-deploy: wait for Eventarc service-agent propagation

The very first 2nd-gen Cloud Function deployed to a project needs the
Eventarc service agent's permissions to propagate. On Phase 1's first
deploy this surfaced as:

> Permission denied while using the Eventarc Service Agent. If you
> recently started to use Eventarc, it may take a few minutes before
> all necessary permissions are propagated to the Service Agent.
>
> Since this is your first time using 2nd gen functions, we need a
> little bit longer to finish setting everything up. Retry the
> deployment in a few minutes.

After enabling `eventarc.googleapis.com` in step 1, wait ~3–5 minutes
before kicking off the first deploy, or accept that the first deploy
will fail and re-run after the propagation window. Subsequent deploys
to the same project don't hit this.

### 9. Phase 2: bootstrap the Cloud Monitoring service identity

Before applying the Phase 2 Terraform changes (Pub/Sub topic + IAM grant),
force-create Cloud Monitoring's notification publisher service identity:

```zsh
gcloud beta services identity create --service=monitoring.googleapis.com --project=ripls-prod
```

The IAM grant on the Pub/Sub topic targets
`service-<project-number>@gcp-sa-monitoring-notification.iam.gserviceaccount.com`,
which is created **on demand** the first time a project asks Monitoring
to publish to a Pub/Sub channel. If you `terraform apply` before this
identity exists, you get:

> Service account `service-<project-number>@gcp-sa-monitoring-notification.iam.gserviceaccount.com`
> does not exist.

The `gcloud beta services identity create` call provisions the SA
synchronously. After it returns, the IAM binding can target it. Once
created, the identity persists — only required on first Phase 2 setup.

A future cleanup is to add a `google_project_service_identity` resource
(requires the `google-beta` provider) so this is declarative; not in
scope for Phase 2.

### 10. Phase 2: handle Pub/Sub topic ordering between Firebase and Terraform

The Pub/Sub topic `gcp-alerts-to-github` is referenced by **both**:

- The Cloud Function `gcpAlertsHandler` (via `onMessagePublished({ topic: ... })`),
  which auto-creates the topic on deploy if missing.
- The Terraform `google_pubsub_topic.github_alerts` resource, which
  expects to own and create the topic.

If the Cloud Function deploys first (which is what happened during
Phase 2's prod rollout), `terraform apply` errors with:

> Error 409: Resource already exists in the project (resource=gcp-alerts-to-github).

Fix: import the existing topic into Terraform state so subsequent
applies manage it as the owner.

```zsh
terraform import 'module.monitoring.google_pubsub_topic.github_alerts[0]' projects/ripls-prod/topics/gcp-alerts-to-github
```

(The `[0]` is because the resource has `count = var.enable_github_pubsub_channel ? 1 : 0`.)

To avoid the dance entirely on Phase 4, **apply the Phase 2 Terraform
changes BEFORE deploying the function** — the topic exists when
Firebase deploys, the function reuses it, no import needed.

### 11. (Out of band, by hand)

Items not yet captured in code:

- Bryan-as-reviewer assignment on PR #1762: `gh pr edit 1762 --add-reviewer maintainer`.

### Phase 4 (dev coverage) replay

When Phase 4 ships, replay steps 1–6 + 9 against `ripls-dev`:

```zsh
PROJECT=ripls-dev
PROJECT_NUMBER=$(gcloud projects describe "$PROJECT" --format="value(projectNumber)")
# substitute PROJECT and PROJECT_NUMBER into the steps above
```

Step 7's threshold may want to be lower for dev (more crash signal,
less impact). Step 8's propagation wait still applies — `ripls-dev`
hasn't deployed any 2nd-gen functions yet either.

Step 10's topic-ordering gotcha is avoidable on dev: apply the Phase 2
Terraform pieces (`module.monitoring.google_pubsub_topic.github_alerts`
+ `google_monitoring_notification_channel.pubsub_github` +
`google_pubsub_topic_iam_member.monitoring_publisher`) **before** the
first Cloud Function deploy. The topic will exist when Firebase
inspects it, and the function reuses it without creating.

## Related Issues

- **#1148** — Crashlytics half of this combined plan.
- **#1149** — Cloud Monitoring half.
- **#1322** — closed prior PR (Cloud Monitoring side; rejected by maintainer with "needs more human design").
- **#1323** — closed prior PR (Crashlytics side; rejected with same comment after a pivot from Go server to Firebase Functions).
- **`.github/workflows/claude_plan_issue.yaml`** — the existing workflow we're funneling into. Triggered by `claude-fix` label or `@claude` mention. **Do not duplicate.**

## Audit Notes

This plan was audited against the project's architectural references on 2026-05-07. Changes applied:

- **Doc paths corrected.** Plan originally referenced `docs/server/logging.md` (renamed to `docs/server/observability.md` per the existing doc's own §Migration Notes — a stale reference would mislead future readers) and `docs/client/observability.md` (does not exist; client-side observability is documented elsewhere and the Cloud Function infrastructure is server-side observability anyway). All references now point at `docs/server/observability.md`, which already has §Cloud Monitoring & Alerting as the natural home for both new sub-sections.
- **Existing `server/github` library + `server/services/feedback` surfaced.** A shared `github.AppClient` already exists and is used by the feedback service to file user-submitted feedback as GitHub issues. The plan now acknowledges this and articulates the principled distinction (request-driven vs. event-driven) for why alerts go external despite the existing pattern. This protects against a reviewer asking "why aren't we just adding a method to `server/github`?"
- **Standard log-field names anchored.** Plan was using ad-hoc field names (`request_id`, `user_id`, `error_message`). Now references the canonical list from `docs/server/observability.md` §Standard Field Names, including the `target_request_id` distinction and the `request_id` HTTP-correlation reservation. The Cloud Function body templates should expect this naming exactly.
- **`MaskEmail` / `MaskToken` / `RedactedEmail` / `RedactedToken` mechanics expanded** in Q6. Server-side masking is the first line of defense; the Cloud Function adds defense-in-depth. Test assertions called out explicitly.
- **`health_check_unhealthy` payload shape** documented: alert includes `dependency` and `backend` labels per `observability.md` §Alert Policies, surfaced into the issue title + a `dep:<dependency>` label so AI-vs-DB failures don't dedup against each other.
- **`claude_plan_issue.yaml` self-trigger guard** flagged (new Q9). Guard skips `github.actor == 'claude[bot]'`. Auto-filer must use a distinct identity. Phase 1 task list now includes a verification step.
- **Notification-channel target acknowledged**. Both dev and prod email currently route to `support@ripls.org` per `observability.md`; this stays unchanged when we add the Pub/Sub channel in parallel.
- **Per `docs/server/architecture.md` §1–2**, the "shared library, not service-to-service" principle informs why the *external* helper duplicates a thin slice of `server/github`'s behavior rather than the Cloud Function calling back into the Go server: the alert-handler is a separate runtime by Bryan's design directive, and runtimes don't share live code. The duplication is intentional and bounded (~50 LOC of GitHub REST calls). If the helper grows, the right move is a thin extraction into `cloud_functions/src/lib/` rather than a server callback.
