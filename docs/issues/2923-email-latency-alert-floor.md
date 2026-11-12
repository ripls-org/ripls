# Issue #2923: [ops:warn] P95 delivery latency above 10000ms for sign-in codes — 2026-08-15

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2923
**Filed:** 2026-08-15 | **Last activity:** 2026-08-15
**Labels:** auto-filed, claude-fix, ops, severity:warn,
`gcp-policy:p95-delivery-latency-above-100-20260815` | **Assignees:** unassigned
**Context docs:** `docs/llms.txt` (index); anchors `server/architecture.md`,
`server/conventions.md`, `server/observability.md`, `proto_conventions.md`;
review prompt `docs/reviews/observability_review_prompt.md`; prior plan
`docs/issues/2862-email-delivery-webhook.md` (this alert's design rationale,
including the pre-designed remedy for exactly this scenario); prior plan for
first firing `docs/issues/2904-email-latency-alert-floor.md` on branch
`claude/fix-issue-2904-31667785891` (drafted as PR #2905, **closed unmerged
2026-08-13 by maintainer with no comment**).

**UI investigation:** skipped — no UI surface. This is a Terraform monitoring
policy; nothing here is observable in the app or web bundle.

---

## Problem Statement

`Email Sign-in Code Delivery Slow - server` fired again on 2026-08-15 at
17:24:28 UTC with observed P95 = 12160ms against the 10000ms threshold, on
`email_delivery_latency_prod` filtered to `metric.labels.email_type="email_code"`.
This is the **second** firing in three days of the same policy — the first was
issue #2904 (2026-08-13, P95 = 24960ms). No code has landed between the two
firings; `terraform/modules/monitoring/alerts.tf:387–394` still carries the
"if it proves noisy, the fix is a second condition on the delivered *count*
with combiner = AND" docstring, and no `email_code_delivered_min_count` variable
has been added.

The prior recurrence was analyzed in detail in the plan on branch
`claude/fix-issue-2904-31667785891`
(`docs/issues/2904-email-latency-alert-floor.md`), drafted as PR #2905. **That
PR was closed unmerged by maintainer on 2026-08-13 with no comment**, so the
recommendation (add a volume floor, keep the 10s threshold, mirror the SMS
policy shape) stands unrejected but also unaccepted. The second firing is fresh
evidence for the "if it proves noisy" branch of the alert's design, and this
plan re-opens the same decision with the additional data point.

Restated for scope: this is **not** a "diagnose slow email" issue. Prod email
sign-in-code volume is still single-digit-per-week (per #2862's plan and the
alert's own docstring), so P95 in a 10-minute window is effectively one message.
Both firings sit inside the 15-minute `mailgunMaxClockSkew` guard in
`server/services/web/email_status.go:201`, so the raw numbers are real Mailgun
delivery times — not clock artefacts. The SLI is behaving exactly as its
docstring says it will at low volume, and the fix is the one named in advance:
gate the percentile on a delivered-count floor so a lone outlier cannot page.

## Context from Comments

The issue itself is auto-filed by the GCP → GitHub pipeline
(`cloud_functions/src/gcpAlertsHandler.ts`) and has zero discussion comments —
all context lives in adjacent artefacts:

- **Prior plan `docs/issues/2904-email-latency-alert-floor.md`** — the full
  analysis of the first firing. Recommended Option A (volume floor + combiner
  AND). Reviewed against `observability_review_prompt.md`. Went to PR #2905
  and was closed unmerged with no comment. Nothing about that plan has become
  wrong; the second firing simply confirms its premise.
- **`docs/issues/2862-email-delivery-webhook.md`** — the plan that shipped the
  alert. Its *Open Questions → "What p95 threshold?"* explicitly recommends
  *not* picking a fixed threshold until prod has a week of data, and names
  30s-sustained-15-minutes as a defensible placeholder if one is required. It
  also names the min-count floor design pattern ("in the spirit of
  `rpc_error_rate_min_errors_per_second`").
- **The inline comment on the alert resource** (`alerts.tf:387–394`). The
  primary contract: the fix if noisy is a second condition on the delivered
  *count* with `combiner = "AND"`.
- **The SMS analog**, `sms_delivery_failure_rate` in `alerts.tf:283–360`. It
  ships the exact two-condition combiner-AND shape this issue will mirror:
  condition 1 is the ratio/percentile threshold, condition 2 is the min-count
  floor over the same alignment window; both display names contain "delivery
  fail" so `cloud_functions/src/gcpAlertsHandler.ts:108` still routes them to
  WARN.
- **Related open issue #2907** (2026-08-13) — a proposal to alert on phone
  sign-in going dark. Its "Needs a volume floor" section names #2904 as the
  precedent for the same pattern applied here, meaning any change here will be
  the reference implementation the phone-sign-in alert borrows from. Landing
  the floor here first, cleanly, is the smaller of the two.

**The prior plan's closure without comment is the unresolved input to this
plan.** Options B and C exist precisely so the reviewer has a clean lane to
say "not that shape" without re-litigating.

## Review-dimension notes

Checked against the `observability` review prompt (the only engaged dimension —
this is a monitoring policy change with no user input, no code path, no data
touched). All notes carry forward from the #2904 plan unchanged:

- **`condition_threshold`, not PromQL.** Log-based metrics do not enter the
  Prometheus series catalog until data arrives, and a PromQL policy
  referencing an empty log-based metric is rejected server-side at create time
  (the trap #2873 was written to close). Both conditions must stay on
  `condition_threshold`.
- **Alert classifier is pinned.** The existing condition display name
  `"P95 delivery latency above 10000ms for sign-in codes"` is pinned in
  `cloud_functions/src/__tests__/gcpAlertsHandler.test.ts:142–147`. Any new
  condition's display name must also match one of the WARN patterns in
  `cloud_functions/src/gcpAlertsHandler.ts:91–109`, or an incident naming the
  new condition will demote to `unknown` (#2622). Route: keep the word
  `latency` in the floor condition's name so it hits `/latency/i` at line 100.
- **Clock-skew guard.** `email_status.go:201` already discards latencies
  ≥ 15 minutes as clock disagreement, so 12160ms and 24960ms are real
  measurements. No defensive metric-side clamp is needed.
- **Alignment window parity.** With `combiner = "AND"` both conditions must
  share `alignment_period = "600s"` so a slow window and a low-volume window
  compare like-for-like. This matches the SMS policy's shape.

## Potential Solutions

The shape is fixed by the alert's own docstring; the options are which
*values* to pick and whether to also raise the threshold. Options mirror the
prior plan for #2904 verbatim so the reviewer can compare directly against
that closed PR.

### Option A: Add a min-count floor at 5 delivered/10 min, keep threshold at 10s (recommended)

- **Approach:** Add a second `conditions {}` block to
  `email_code_delivery_latency` in `alerts.tf`, filtering the same
  `email_delivery_latency_prod` metric on `email_type="email_code"` with
  `per_series_aligner = "ALIGN_COUNT"` and
  `cross_series_reducer = "REDUCE_SUM"` over
  `alignment_period = "600s"` and
  `threshold_value = var.email_code_delivered_min_count`, default `5`
  (`COMPARISON_GT`, so ≥6 delivered codes in a 10-minute window). `ALIGN_COUNT`
  — not `ALIGN_DELTA` — because the underlying metric is `DISTRIBUTION`, so
  `ALIGN_DELTA` + `REDUCE_SUM` (the SMS pattern, valid for INT64 counters)
  would leave the aggregation non-scalar and the `condition_threshold`
  invalid. `ALIGN_COUNT` on a distribution returns the count of underlying
  samples per series (i.e., matching log entries), which is exactly the
  delivered-event count we want; `REDUCE_SUM` sums across Cloud Run
  instances. Switch the policy's `combiner` from `OR` to `AND`. Add the new
  variable to `variables.tf`. Add a pin for the new condition string to
  `cloud_functions/src/__tests__/gcpAlertsHandler.test.ts`. Update
  `docs/server/observability.md`'s alert row to describe the floor.
- **Pros:** Exactly the remedy named in the alert's docstring, in #2862's
  plan, and in the SMS analog. Preserves the 10s threshold so the moment prod
  volume rises, a real slowdown pages. Two firings within three days without
  a real deliverability regression is textbook evidence for the "if it proves
  noisy" branch. Follows the SMS policy shape line for line, which reduces
  cognitive load and keeps future edits parallel. Also unblocks #2907 (phone
  sign-in alert) by shipping the reference implementation it borrows from.
- **Cons:** Under current single-digit-per-week volume, this alert will
  effectively never fire — including if there *is* a real deliverability
  problem, because it will not have volume to prove it. That is the accepted
  tradeoff (see *Recommendation*), but worth naming.
- **Effort:** Small. Terraform + one classifier test line + one docs row +
  one apply (two-pass unlikely — the descriptor already exists).

### Option B: Raise the threshold to 30s, no floor

- **Approach:** Change `email_code_p95_latency_threshold_ms` default from
  `10000` to `30000` in `variables.tf`. Rename the condition display name to
  match ("P95 delivery latency above 30000ms for sign-in codes"). Update the
  classifier test pin. Update `observability.md`.
- **Pros:** Smallest diff. Matches the "30s sustained 15 min is defensible"
  fallback from #2862's Open Questions. No two-pass apply. If the closure of
  #2905 reflected a preference to move the threshold rather than gate the
  percentile, this is the shape.
- **Cons:** Does not remove the underlying problem — a single 30s+ Mailgun
  delivery in an otherwise-empty window still fires the alert. It just moves
  the noise floor. The two observed samples were 24.96s and 12.16s; the next
  outlier is not guaranteed to stay under 30s just because these two did (the
  first was 25s, well within striking distance of 30). And it discards the
  design guidance the alert's docstring gives, in favor of a number for which
  neither issue has evidence beyond "the last outlier stayed under it."
- **Effort:** Small.

### Option C: Both a floor and a raised threshold

- **Approach:** Combine A and B — add the min-count floor *and* raise the
  threshold to 30s.
- **Pros:** Defensive on both axes. If the volume floor lets meaningful
  traffic through and the true product-acceptable latency is closer to 30s
  than 10s, this is the only shape that survives both.
- **Cons:** Loses the useful property of A that a real slowdown at
  currently-typical latencies (Mailgun median 700–900ms; observed outliers
  6.7s, 12.16s, and 24.96s) still pages once volume arrives. If both a floor
  and a higher threshold are required, that is two changes to justify
  separately. The plan doc for #2862 explicitly favored a floor over a
  higher threshold ("do not pick one until prod has a week of data"), and
  three data points is not yet a week.
- **Effort:** Small, but two decisions instead of one.

## Recommendation

**Option A**, unchanged from #2904's plan.

Two firings in three days without any deliverability regression is direct
evidence for the "if it proves noisy" branch the alert's own docstring names.
The SMS analog already runs the exact shape. Adopting it — rather than
debating a threshold value with only three low-volume samples on hand — keeps
the SLI's *meaning* honest: "P95 delivery time when we have enough volume
for P95 to mean anything." Raising the threshold without a floor (B) just
moves the noise floor and is still noise; combining both (C) is overkill
given no evidence for 30s over 10s.

`min_count = 5` (with `GT`, so ≥6 events per 10-minute window) is
deliberately set well above current single-digit-per-week volume. At that
level the alert is *silent by design* until real volume arrives, which is
what the whole issue thread is about — the SLI is aspirational at current
scale, and the honest engineering is to say so in code rather than fire on
noise and rename the noise "signal." When #2571's passwordless-email rollout
reaches meaningful volume the floor becomes cheap headroom rather than a
suppression. It also matches the review prompt's guidance that low-volume
percentile alerts need a volume guard or they degrade to "one sample went
weird."

**One thing to keep from B, applied as a follow-up not this PR:** #2862's
plan still calls for revisiting the threshold "after a week of prod data."
Once volume is enough for the floor to allow the alert to fire, that
revisit becomes actionable. Open Question 3 lands that as a follow-up
tracked in this issue's PR body rather than folded into the initial change.

**Because #2905 was closed with no comment, please confirm.** If the closure
reflected a preference for Option B, or for doing nothing until volume
rises, or for waiting on a broader alert-quality decision, the reviewer can
say so on this PR and the plan swaps or stops. Absent that signal, the
alert is going to keep firing weekly-ish on real but isolated Mailgun jitter
and paging noise for something that isn't actionable.

## Open Questions

- **Why was #2905 closed?** — Recommendation: this is the load-bearing
  unknown. Ask on the PR before implementing. If the answer is "we should
  raise the threshold instead," swap to Option B. If it is "we should not
  change the alert until volume rises," close this PR too and add a WONTFIX
  note to the alert docstring so the next auto-fired issue does not re-open
  the loop. Anything else, implement Option A as written.
- **What min-count value?** — Recommendation: `5` (so ≥6 delivered codes in
  a 10-minute alignment window are required before the P95 side can page).
  This is above current volume (single-digit-per-week ≈ ≤2 per 10 min at
  peak), so during the low-volume phase the alert is effectively silent;
  once passwordless sign-in becomes primary the floor is comfortably below
  even a modest hour. Parameterize as a new variable
  `email_code_delivered_min_count` so a future retune is one number, and
  document the value in `observability.md` with the reasoning.
- **Should the P95 threshold also change?** — Recommendation: no, not in
  this change. Keeping `10000` preserves the SLI as "10s is slow," which is
  a defensible statement of the product intent (a mailed sign-in code
  arriving in 10s is already awful UX). Move the threshold revisit to a
  follow-up per #2862's own open question, once the floor has let real
  volume through and there is a week of representative data.
- **What is the new condition's display name?** — Recommendation:
  `"Sign-in code delivery latency: minimum sample floor of ${var.email_code_delivered_min_count} per 10 min"`.
  Includes "latency" so the classifier's `/latency/i` WARN pattern
  (`gcpAlertsHandler.ts:100`) still matches; describes the floor's purpose
  without the misleading "P95" prefix (this condition is a count, not a
  percentile); interpolates the volume so a future retune stays honest.
  Pin the exact rendered string in
  `cloud_functions/src/__tests__/gcpAlertsHandler.test.ts` alongside the
  existing pin.
- **Two-pass apply?** — Recommendation: expect it, but it should not be
  needed here. The SMS policy hit two-pass because it added a *new* metric
  descriptor alongside the policy; here the metric
  (`email_delivery_latency_prod`) already exists and already has data
  (that's the whole reason this issue exists), so the policy's second
  condition references an already-populated descriptor. If Terraform does
  hit a propagation issue, re-run — same as #2871.
- **Update the alert docstring?** — Recommendation: yes, replace the "if it
  proves noisy" paragraph with a short "as of #2923, a floor of N delivered
  per 10 min is in place, because the alert fired twice at low volume as
  predicted." Keeps `git blame` legible without a reader needing to trace
  through the doc.
- **Should this also apply to `email_delivery_latency_dev`?** —
  Recommendation: yes, because the module is applied identically to both
  environments and the same variables control both. Dev volume is even lower
  than prod, so the floor is even more warranted there. The change ships to
  both automatically; nothing extra to do.

## Implementation Plan

### Phase 1: Add the min-count floor to the terraform module

**Goal:** The alert requires *both* P95 > 10s *and* ≥6 delivered codes in
the same 10-minute window before firing.

**Tasks:**

- [ ] `terraform/modules/monitoring/variables.tf`: add
      `variable "email_code_delivered_min_count"`, default `5`, matching the
      style of `sms_delivery_failure_min_count` (comment referencing #2923).
- [ ] `terraform/modules/monitoring/alerts.tf`, resource
      `email_code_delivery_latency`: change `combiner = "OR"` to
      `combiner = "AND"`; add a second `conditions {}` block with
      `display_name = "Sign-in code delivery latency: minimum sample floor
      of ${var.email_code_delivered_min_count} per 10 min"`, a
      `condition_threshold` filtering the same
      `email_delivery_latency_prod`/`_dev` metric on
      `metric.labels.email_type="email_code"`, `comparison = "COMPARISON_GT"`,
      `threshold_value = var.email_code_delivered_min_count`,
      `duration = "300s"` (matches the P95 condition),
      `aggregations { alignment_period = "600s"; per_series_aligner =
      "ALIGN_COUNT"; cross_series_reducer = "REDUCE_SUM" }`. `ALIGN_COUNT`
      rather than `ALIGN_DELTA` because the metric is `DISTRIBUTION` and the
      threshold needs a scalar count of delivered events, not a delta
      distribution (see Option A rationale above; the SMS floor's
      `ALIGN_DELTA`/`REDUCE_SUM` shape is valid there because
      `sms_delivery_outcomes` is `INT64`).
- [ ] Replace the multi-paragraph "if it proves noisy" comment on the
      resource with a shorter block referencing #2923 and describing the
      current shape (floor value, why AND, why the display name is what it
      is).
- [ ] `cloud_functions/src/__tests__/gcpAlertsHandler.test.ts`: extend the
      existing `email delivery latency actual condition string → warn` test
      (line 142) to also pin the new floor condition's rendered string.
      Confirm both classify as `warn`.

**Validates:** `terraform plan` shows the alert modified in-place with a
second condition, combiner AND, and the new variable added. Classifier
tests still green.

### Phase 2: Apply and confirm on prod

**Goal:** The live policy has the floor; the currently-firing incident
auto-closes on the 7-day `auto_close`, or resolves at the next evaluation
that no longer satisfies both conditions.

**Tasks:**

- [ ] `terraform plan -target=module.monitoring` from
      `terraform/environments/prod`; expect one policy modified, one
      variable-driven condition added, no metric or dashboard churn (the
      metric already exists and has data).
- [ ] `terraform apply`. If Terraform reports a descriptor-propagation error
      (`Error 400: The following … metric(s) are invalid`), re-run — same as
      #2871's `sms_delivery_outcomes` two-pass, though it should not happen
      here because the metric predates the policy edit.
- [ ] Also apply to `dev` for parity (`deploy_to_dev_on_push.yaml` does not
      pick this up automatically for Terraform).
- [ ] Confirm in the Cloud Monitoring UI that the policy shows two
      conditions with combiner AND, and that the currently-open incident
      (`0.obgy62ignklh`) either auto-closes or resolves within one
      alignment window.

**Validates:** The policy on the ground matches the source, and the
open incident does not reopen at the next evaluation.

### Phase 3: Document and close

**Goal:** The alert table in `docs/server/observability.md` describes the
new shape; the issue closes; #2907's phone-sign-in alert has a reference
implementation to copy.

**Tasks:**

- [ ] `docs/server/observability.md:935`: replace the "No volume floor yet"
      row with a description of the two-condition combiner-AND shape,
      naming the min-count variable and its value, and referencing #2923.
- [ ] `docs/issues/2862-email-delivery-webhook.md`, Open Questions
      "What p95 threshold?": add a `**Resolved 2026-08-15:**` note pointing
      to #2923 for the volume-floor half of the answer, leaving the
      threshold-revisit half as a follow-up.
- [ ] Confirm `npm run lint:context-map` passes.
- [ ] Post a follow-up task on this PR / open a small follow-up issue for
      the "revisit the 10s threshold once prod has a week of real data"
      remainder from #2862 — do not fold it into this change.
- [ ] Cross-link the merged PR in #2907's "Needs a volume floor" bullet so
      the phone-sign-in alert has a working precedent to copy.
- [ ] Close #2923 (and re-close #2904, which was auto-closed by daily sweep
      but shares the fix) with a link to the merged PR.

**Validates:** The doc-freshness gate stays green; a future reader picking
up the alert can see from the source alone why the shape is what it is.

## Related Issues

- **#2904** — the first firing of this identical policy (2026-08-13). Its
  plan (`docs/issues/2904-email-latency-alert-floor.md`, branch
  `claude/fix-issue-2904-31667785891`, PR #2905) recommended this same
  Option A. **PR #2905 was closed unmerged with no comment.** The absence of
  that comment is Open Question 1 above.
- **#2862** — the plan that shipped the alert. Its Open Questions section
  explicitly recommended a volume floor over a higher threshold, and its
  Implementation Plan Phase 4 named this precisely as the fix if the initial
  threshold proved noisy.
- **#2569 / #2871 / #2873** — the SMS status-callback and delivery-failure
  work whose two-condition combiner-AND shape this change mirrors. #2871 in
  particular is the working precedent for the same pattern on a log-based
  metric.
- **#2571 / #2864** — passwordless email OTP. When the passwordless flow
  becomes primary, prod email-code volume rises, the floor stops
  suppressing the alert, and the threshold-revisit follow-up becomes
  actionable.
- **#2622** — the classifier "unknown" hazard this change avoids by
  ensuring the new condition display name still matches a WARN pattern in
  `gcpAlertsHandler.ts`.
- **#2907** — phone sign-in going dark alert. Its "Needs a volume floor"
  section names #2904's approach as its reference; this PR provides the
  landed precedent it borrows from.
