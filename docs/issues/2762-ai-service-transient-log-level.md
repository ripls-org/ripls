# Issue #2762: [ops:warn] Service-level error logged (excluding middleware and infrastructure) — 2026-07-25

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2762
**Filed:** 2026-07-25 | **Last activity:** 2026-07-25
**Labels:** auto-filed, claude-fix, ops, severity:warn, gcp-policy:service-level-error-logged-exc-20260725
**Assignees:** unassigned
**Context docs:** `docs/server/observability.md` (log levels §5 L72-96; leaf/fallback doctrine L98-126; alert policies), ⚓ `docs/server/architecture.md`, ⚓ `docs/server/conventions.md` (log-level rules L99-103), `server/ai/README.md`, `server/errs/README.md` (transient predicate), prior plan `docs/issues/2732-ai-leaf-error-log-level.md` and its fix PR #2733, `docs/reviews/observability_review_prompt.md`, `docs/reviews/error_handling_review_prompt.md` (§4 Logging & Observability, §5 Retry & Recovery).

## Problem Statement

The Cloud Monitoring policy `Server Error Logged - server` fired three
times on 2026-07-25 between 18:27 and 18:31 UTC on revision
`server-00129-rmt`. The three matched log entries all originate in the
unified-create streaming path and share a single root cause — Vertex AI
Gemini returning `Error 429, Status: RESOURCE_EXHAUSTED` under quota
pressure:

```
message=AI streaming generation failed
operation=StreamGenUnifiedCreate
error_message=gemini streaming call failed: Error 429, Message: Resource exhausted…, Status: RESOURCE_EXHAUSTED

message=gemini API call failed  (x2)
operation=UnifiedCreateClassify
```

Sister issue #2761 fired at the same moment with the same body.

**Two distinct defects are visible in this alert cluster:**

1. **Streaming has no fallback.** `FallbackProvider.*Streaming` in
   `server/ai/provider_streaming_unary.go` delegates directly to
   `p.primary.*Streaming(...)` — there is no retry against `p.fallbacks`
   even when the primary fails before emitting any `FieldEvent`. So a
   Vertex 429 on the primary Gemini streaming call terminates the entire
   `StreamGenUnifiedCreate` request, whereas the unary path (used by
   the classifier) would have transparently recovered via Anthropic /
   OpenAI. Users see a `GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED` event
   even though other providers were available and healthy.

2. **Service-level handlers log transient failures at ERROR.** After
   PR #2733 (issue #2732) demoted leaf-provider "API call failed" logs
   from ERROR to WARN, the alert now trips one layer up — at the
   service-handler ERROR logs that fire when the streaming call fails
   without any fallback recovery:

   ```
   server/services/{experience,gear,request,community}/gen_*_streaming.go
   server/services/unified_create/stream_gen.go
   server/services/experience/gen_ai.go
   ```

   These are essentially the same class of transient failure the leaf
   fix already addressed — the underlying cause is a 429, not a system
   failure — but they escalate to ERROR because they *are* the terminal
   log for streaming (no wrapper above will demote them).

   The two `message=gemini API call failed / operation=UnifiedCreateClassify`
   entries in this alert cluster are almost certainly emitted by
   pre-PR-#2733 code still resident on `server-00129-rmt` — the current
   `provider_gemini.go:364` and `provider_helpers.go:165` are both
   `WarnContext`. They will disappear on the next production deploy
   regardless of this plan. The new work here is (1) and the service-
   layer half of (2).

The observability doctrine at `docs/server/observability.md` §L98-126
already codifies "leaf: WARN, fallback wrapper: ERROR" and the sibling
`docs/server/observability.md` §L81-96 documents transient errors in
background jobs using `errs.TransientStreak`. The AI call sites need
the analogous discipline: transient failures (429 / RESOURCE_EXHAUSTED
/ upstream 5xx / connection resets) log WARN, terminal / non-transient
failures log ERROR, using the existing `ai.IsTransientError` /
`errs.IsTransient` predicate as the classifier. This exactly matches
what `server/services/experience/gen_ai.go:475-478` already does for
the return-code decision — the log-level decision needs the same
predicate.

## Context from Comments

The issue has no human comments — it was auto-filed by the
`gcp-alerts-to-github` Cloud Function pipeline
(`cloud_functions/src/gcpAlertsHandler.ts`) as a warn-severity
aggregator (`gcp-policy:service-level-error-logged-exc-20260725`).

**Sister issue.** #2761 fired at the same moment with the same
`AI streaming generation failed / RESOURCE_EXHAUSTED` body — same
incident, same fix.

**Prior instance history:**
- **#2732 / PR #2733** (2026-07-16 → 2026-07-17) — Same alert policy;
  demoted every leaf-provider `*_helpers.go` / `provider_{gemini,openai,
  anthropic}{,_streaming,_loadbalanced}.go` `Error` → `Warn`. Established
  the "leaf WARN, wrapper ERROR" doctrine now in
  `docs/server/observability.md` §L98-126. That fix was necessary but not
  sufficient — it stopped leaf-level alert firings but left the service-
  layer ERROR logs (this issue) and the streaming-no-fallback gap
  untouched.
- **#2711 / PR #2712** (2026-07-13) — Same alert policy on `DeleteGear`;
  established the doctrine "downgrade the log level for expected /
  handled failures; keep true system failures at ERROR."
- **#1236, #973** (older) — Recurring Vertex 429 pain (integration
  tests). Vertex quota exhaustion is a *known* transient class in this
  codebase.

**Existing infrastructure this plan reuses:**
- `errs.IsTransient(err)` (`server/errs/transient.go`) — string-based
  substring predicate; matches `429`, `rate limit`, `resource exhausted`,
  `quota`, `503/502/504`, `unavailable`, `deadline exceeded`, connection
  resets. Already used by `errs.TransientStreak` in background jobs.
- `ai.IsTransientError(err)` (`server/ai/errors.go`) — thin forward to
  `errs.IsTransient`, exists specifically so AI call sites don't need
  the `errs` import.
- `server/services/experience/gen_ai.go:475-478` — reference pattern
  for the return-code decision: `if ai.IsTransientError(err) { return
  connect.CodeUnavailable } else { return connect.CodeInternal }`.

## Potential Solutions

### Option A: Service-layer transient-aware log-level (log-only fix)

- **Approach:** At each of the ten service-handler ERROR log sites
  listed below, gate `logger.ErrorContext` on
  `ai.IsTransientError(err)`: transient → `WarnContext`, non-transient →
  `ErrorContext`. No behavioral change — the error is still returned to
  the caller and the RPC still emits `GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED`
  or a `connect.CodeInternal` / `CodeUnavailable`. Only the log level
  changes.

  Sites:
  - `server/services/experience/gen_ai_streaming.go:286`
  - `server/services/experience/gen_ai_streaming_media.go:216, 461`
  - `server/services/experience/gen_ai.go:475`
  - `server/services/request/gen_streaming.go:199, 397`
  - `server/services/community/gen_streaming.go:168`
  - `server/services/gear/gen_ai_streaming.go:349, 459`
  - `server/services/unified_create/stream_gen.go:192` (`classifier
    failed`), `246` (`per-type generation failed`)

- **Pros:**
  - Minimum code churn (10 sites, no new interface). Mirrors PR #2733
    exactly, one layer up.
  - Uses the existing `ai.IsTransientError` predicate — no new
    classifier logic, no duplicated substring lists.
  - Matches the observability doctrine: transient/handled → WARN,
    terminal/system → ERROR. Adds the service-layer half of the
    contract that the doc's §L98-126 leaf/fallback paragraph already
    established.
  - Preserves the ERROR signal for real system failures — a Gemini
    responding with malformed JSON, a Genkit config bug, an image-mode
    decoder crash all still fire the alert (non-transient errors).
  - The `rpc_error_rate` metric (see observability §Alert Policies)
    continues to alert on user-visible failures via the 5xx-rate signal
    independent of these log levels.
  - **Does not address the streaming-no-fallback gap.** Users still see
    an error whenever the primary streaming provider 429s. But it stops
    the false-positive page.

- **Cons:**
  - Users still see failures on primary-provider 429s. The alert quiets,
    but the underlying issue persists. Streaming reliability requires
    Option B or C.
  - New defensive check at every site. Small maintenance cost.

- **Effort:** Small. ~10 log-line edits + one predicate call each +
  tests.

### Option B: Streaming-side fallback in FallbackProvider (behavioral fix)

- **Approach:** Rework `FallbackProvider.*Streaming` in
  `server/ai/provider_streaming_unary.go` to attempt the fallback
  providers when the primary streaming call fails **at setup time**
  (before any `FieldEvent` has been emitted to the consumer). The safe-
  retry contract: buffer/discard the primary's `fields` and `final`
  channels; if `final` reports an error and no field event has crossed
  the wire yet, try the next fallback. If any field event has already
  been forwarded, surface the error on the final channel unchanged (the
  existing behavior, since the consumer's downstream state cannot be
  rolled back).

  Because streaming producer goroutines return `<-chan FieldEvent` and
  `<-chan Final` immediately, the cleanest shape is a small orchestrator
  that starts the primary, watches for the first FieldEvent OR final
  error, decides whether to retry, and only then wires the surviving
  producer's channels to the consumer. `LoadBalancedProvider.*Streaming`
  should reuse the same orchestrator wrapped over the LB-selected
  provider so weighted fleets get the same recovery.

- **Pros:**
  - Fixes the root cause. Users get a working stream when *any* provider
    is healthy — the same reliability the unary path gets today.
  - Eliminates the alert firings by preventing the terminal ERROR in
    the first place (no error to log if the fallback succeeds).
  - No new logging discipline needed — the existing
    `FallbackProvider.withFallback` doctrine ("WARN on primary/fallback
    failure, ERROR on all-failed") naturally extends.

- **Cons:**
  - Larger surface: 10 streaming method wrappers to update on
    `FallbackProvider` and 10 on `LoadBalancedProvider`, plus a new
    orchestrator helper and its tests.
  - Non-trivial concurrency: buffering channels, timing the "first
    FieldEvent" check, propagating context cancellation across the
    retry, and preserving each provider's own goroutine-panic recovery
    all have to be right. Every wrong turn is a latency regression or a
    goroutine leak.
  - Doesn't help if all providers are 429ing at once (all-Vertex fleets
    can share a quota pool — worth checking factory config). In that
    case the service handler still needs Option A's log-level fix so
    the residual "everything failed" firing doesn't page.

- **Effort:** Medium-to-large. New orchestrator + 20 delegating methods
  updated + concurrency tests. Realistically two phases: orchestrator +
  a couple of methods to prove the shape (Phase 1), then rollout across
  the rest (Phase 2).

### Option C: A + B (recommended)

- **Approach:** Apply Option A immediately as Phase 1 (small, mechanical,
  ships fast and quiets the alert), then Option B as Phase 2 (fixes the
  underlying reliability defect). Once B is in place, A's guard
  effectively becomes dormant for the common case — but it stays as the
  correct handling for "all providers failed" (which is what
  `FallbackProvider.withFallback` will emit).

- **Pros:** Combines A's fast alert quiet with B's real fix. Ships in
  reviewable increments.
- **Cons:** More total work than either alone. But each phase is
  independently valuable and independently reviewable.
- **Effort:** Small (Phase 1) + Medium (Phase 2).

### Option D: Filter the alert policy to exclude 429s (alert-side fix)

- **Approach:** Add `NOT jsonPayload.error =~ "RESOURCE_EXHAUSTED|Error 429"`
  to the `all_errors` alert filter in
  `terraform/modules/monitoring/main.tf`.

- **Pros:** Zero application code. Immediate silencing.
- **Cons:**
  - Moves the correctness discipline out of the codebase and into
    Terraform, where it's less visible and much harder to reason about
    ("why doesn't the alert fire on 429s?" requires reading the alert
    filter).
  - Silences legitimate 429 signal too — a sustained 429 storm because
    someone bumped traffic 100x is exactly the kind of thing on-call
    should hear about. The correct escalation surface for sustained
    transient failure is `errs.TransientStreak`, not "the alert never
    fires."
  - Doesn't fix the underlying streaming reliability gap.
- **Effort:** Trivial. Not recommended.

## Recommendation

**Option C.** Ship Phase 1 (Option A) immediately — it's ~10 lines of
mechanical change, mirrors PR #2733 one layer up, and matches the
already-documented observability doctrine. Then plan Phase 2 (Option B)
as a follow-up PR — the streaming-fallback orchestrator is real design
work that should get its own review cycle. Phase 3 codifies the
service-layer doctrine in `docs/server/observability.md` so the next
person adding an AI service handler picks it up by default.

**Do not go with Option D alone.** Alert-filter exclusions hide the
signal without fixing the code, and they don't help the streaming
reliability problem at all.

**Concretely for #2762 after Phase 1:** the two `AI streaming
generation failed` entries in this alert cluster (whose error contains
"Error 429" / "RESOURCE_EXHAUSTED") log WARN instead of ERROR,
`all_errors` doesn't fire on them, and the RPC still returns
`GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED` to the client. If a
Gemini-side bug returned malformed JSON instead of a 429, the log stays
at ERROR and the alert still fires — with the correct semantics.

## Open Questions

- **Should the transient-error predicate cover all
  `s.aiProvider.*` call sites, or only the ones surfaced by the current
  alerts?** — Recommendation: **all ten sites listed under Option A**,
  including `AI extraction from webpage failed` at
  `experience/gen_ai.go:475` and the two `stream_gen.go` sites.
  Uniformity beats surgical fixes: the substring predicate is cheap,
  the doctrine is the same, and Vertex 429s (or Anthropic 529s) can hit
  any of these paths tomorrow. Applying it consistently avoids "why is
  webpage-mode noisier than text-mode?" later.

- **Should the log's `error` field or a new `transient` boolean field
  be added?** — Recommendation: **add `transient=true|false` as a
  structured field** alongside `error` on the demoted logs so queries
  like `jsonPayload.transient=true` in Cloud Logging can measure
  transient-failure rate without regex-scraping the error message. Low
  cost; big observability payoff. Follow the convention already used
  by `errs.LogJobError`.

- **Should Option B's orchestrator ship in the same PR as Option A?** —
  Recommendation: **no**, ship Option A first as its own PR (small,
  reviewable, immediate value). Option B is meaningful new abstraction
  and belongs in its own PR — with adversarial concurrency review.

- **For Option B, when the primary fails but has already emitted a
  FieldEvent to the consumer, should we still try to recover from a
  subsequent Final error?** — Recommendation: **no**. Once a field
  event crossed the wire, downstream state (client render, Mapbox call
  fired, Pexels fetch triggered) is not rollback-safe. Surface the Final
  error unchanged. Document explicitly in the orchestrator's godoc.

- **Should `LoadBalancedProvider.*Streaming` get the same orchestrator
  (Phase 2)?** — Recommendation: **yes, but only where the LB layer
  itself has ≥2 providers configured**. When a LB wraps a single
  provider, adding a retry orchestrator is dead code; when it wraps
  multiple, we get free per-request recovery.

- **Should we document the "transient/handled → WARN, terminal → ERROR"
  service-layer rule in `docs/server/observability.md`?** —
  Recommendation: **yes**, in Phase 3. Extend the existing "Leaf
  provider + fallback wrapper" paragraph (§L98-126) to cover the
  service-caller half: "Callers that receive an error from
  `s.aiProvider.<method>` — log WARN if `ai.IsTransientError(err)`,
  ERROR otherwise. Return codes remain `CodeUnavailable` for transient
  and `CodeInternal` for terminal."

- **Do we need any changes to
  `terraform/modules/monitoring/main.tf`?** — Recommendation: **no**.
  The `all_errors` policy at line 447 filters on `severity>="ERROR"`;
  once the transient logs are WARN, they no longer match. No Terraform
  change needed to close this issue. (If a follow-up wants to tighten
  the filter for defense in depth — e.g. an *additional* exclusion for
  `NOT jsonPayload.transient=true` — that's a separate discussion.)

## Implementation Plan

### Phase 1: Downgrade transient service-level AI failure logs to WARN

**Goal:** Stop the false-positive alert firings on transient Vertex
429s / RESOURCE_EXHAUSTED at the service-handler layer. Align the
service-layer log severity with the existing observability doctrine
(and with the leaf fix from PR #2733).

**Tasks:**
- [ ] Add a small helper `LogAIError(ctx, logger, msg, err, kv...)` in
      `server/ai/logutil.go` (new file). Signature and shape mirror
      `errs.LogJobError` (`server/errs/streak.go:82`) — the same one-line
      wrapper the background-job goroutines already use:
      ```go
      func LogAIError(ctx context.Context, logger errs.LeveledLogger, msg string, err error, kv ...any) {
          if err == nil { return }
          level := slog.LevelError
          transient := IsTransientError(err)
          if transient { level = slog.LevelWarn }
          args := make([]any, 0, 4+len(kv))
          args = append(args, "error", err, "transient", transient)
          args = append(args, kv...)
          logger.Log(ctx, level, msg, args...)
      }
      ```
      Reuse `errs.LeveledLogger` (`server/errs/streak.go:78`) so both
      `*slog.Logger` and the project's `*logging.Logger` satisfy the
      parameter. Package-godoc a one-paragraph rationale referencing
      `docs/server/observability.md` §L98-126 and the parallel
      `errs.LogJobError` doctrine.
- [ ] Replace the ten `logger.ErrorContext(...)` / `logger.Error(...)`
      sites at the service layer with `ai.LogAIError(ctx, logger, msg, err,
      "duration_ms", ...)`. Preserve every existing structured field.
      Files:
      - `server/services/experience/gen_ai_streaming.go:286`
      - `server/services/experience/gen_ai_streaming_media.go:216`, `461`
      - `server/services/experience/gen_ai.go:475`
      - `server/services/request/gen_streaming.go:199`, `397`
      - `server/services/community/gen_streaming.go:168`
      - `server/services/gear/gen_ai_streaming.go:349`, `459`
      - `server/services/unified_create/stream_gen.go:192` (`classifier
        failed`), `246` (`per-type generation failed`)
- [ ] Add `server/ai/logutil_test.go` with a **table-driven** test
      (per `docs/reviews/testing_review_prompt.md` §2 Server Testing)
      covering: transient input (`errors.New("...429...")`,
      `errors.New("Status: RESOURCE_EXHAUSTED")`) → WARN +
      `transient=true`; non-transient input (`errors.New("bad JSON")`,
      wrapped `context.DeadlineExceeded` should be transient per
      `errs.IsTransient`) → ERROR + `transient=false`; nil error →
      no log emitted. Use the same `*bytes.Buffer` slog handler
      pattern as `server/ai/provider_helpers_test.go`
      (`assertWarnNotError` / `assertErrorLog`).
- [ ] Do **not** touch `FallbackProvider.withFallback`'s
      "all providers failed" ERROR (`provider_fallback.go:154`) — it
      remains the escalation surface for the unary path.
- [ ] Do **not** touch `media resolution failed`
      (`stream_gen.go:150`), `AI provider not configured` (`:218`), or
      `send type event` (`:214`) — those are non-AI or misconfig errors
      and correctly stay at ERROR.
- [ ] Run, in order:
      - `npm run build` (golangci-lint + `go build ./...`) — the
        conventions-required build gate (`docs/server/conventions.md`
        §L144-158).
      - `go build -tags=integration ./...` — required because
        `ai.LogAIError` is a new exported API that will be called from
        service handlers whose integration-tagged tests
        (`server/services/experience/*_integration_test.go`,
        `server/services/unified_create/*_integration_test.go`) compile
        against those packages (`docs/server/conventions.md` §L152-158).
      - `go test -count=1 ./server/ai/... ./server/services/...`
        — unit tests including the new `logutil_test.go`.

**Validates:** Prod false-positive alerts on
`message=AI streaming generation failed / RESOURCE_EXHAUSTED`,
`AI extraction from webpage failed / 429`, `classifier failed / 429`,
and `per-type generation failed / 429` stop firing.

### Phase 2: Streaming-side fallback in FallbackProvider (behavioral fix)

**Goal:** Restore per-request reliability parity between streaming and
unary. When the primary streaming provider fails at setup time (before
any FieldEvent is emitted), transparently retry against the fallback
providers so the user gets a working stream.

**Tasks:**
- [ ] Add `server/ai/streaming_fallback.go` with a generic
      `streamWithFallback[F any]` orchestrator: takes a `primaryCall
      func(ctx) (<-chan FieldEvent, <-chan F, error)`, a `[]Provider`
      fallback list, and a `fallbackCall func(Provider, ctx) (<-chan
      FieldEvent, <-chan F, error)`. Returns the surviving producer's
      channels to the consumer, retrying at setup or on Final-error
      *only if* no FieldEvent has been forwarded yet. Preserves
      goroutine-panic recovery from the underlying providers.
- [ ] Rewrite the ten `FallbackProvider.*Streaming` delegators in
      `server/ai/provider_streaming_unary.go:25-67` to route through
      `streamWithFallback`. Same public signatures.
- [ ] Rewrite the ten `LoadBalancedProvider.*Streaming` delegators
      (`:74-163`) to route through `streamWithFallback` when
      `getProvidersForMethod(...)` returns >1 provider; otherwise
      unchanged.
- [ ] Add tests to `server/ai/provider_fallback_test.go` (or a new
      `provider_streaming_fallback_test.go`):
      - primary errors at setup, fallback succeeds → consumer sees a
        working stream, only WARN logs
      - primary emits one FieldEvent then errors → consumer sees the
        event and the error, NO retry attempted (per the
        already-emitted-field contract)
      - all providers fail at setup → consumer sees terminal error,
        exactly one `all providers failed` ERROR log at the fallback
        wrapper (existing withFallback contract extended to streaming)
      - context cancellation during retry → all goroutines exit, no
        leaks (goleak or equivalent)
- [ ] Run the concurrency test with `-race -count=100` to shake out
      timing bugs (per `docs/reviews/testing_review_prompt.md` §4 Test
      Quality — Flaky Tests / race conditions). Also run the existing
      `server/ai/provider_streaming_panic_test.go` to confirm the panic
      recovery path is preserved end-to-end through the orchestrator.
- [ ] Update `docs/server/llm.md` §Composite Providers to document that
      `FallbackProvider` streaming methods now retry against fallbacks
      at setup time (before any FieldEvent is emitted). Bump its
      `freshness.verified_commit` / `verified_on`.

**Validates:** Streaming path recovers from primary-provider transient
failures without user impact. The remaining alert firings — if any —
are legitimate "all providers failed" cases that on-call *should* hear
about.

### Phase 3: Codify the service-layer doctrine in observability.md

**Goal:** Make the transient/terminal split explicit so future service
handlers pick it up by default.

**Tasks:**
- [ ] In `docs/server/observability.md`, extend the "Leaf provider +
      fallback wrapper (AI providers)" paragraph (currently §L98-126)
      with a fourth bullet:
      - **Service callers of `s.aiProvider.<method>`** classify the
        returned `error` with `ai.IsTransientError` before logging.
        Transient (429 / RESOURCE_EXHAUSTED / 5xx / connection reset)
        → WARN with `transient=true`. Terminal (malformed response,
        internal contract violation, misconfig) → ERROR. Use
        `ai.LogAIError` so the classification is consistent across
        handlers. This mirrors the leaf/wrapper rule one layer up and
        keeps the `all_errors` alert focused on true system failures.
- [ ] Bump `freshness.verified_commit` / `verified_on` in the doc's
      frontmatter per `docs/context_map.md`.
- [ ] No update needed to `docs/server/llm.md` — it doesn't specify
      service-layer log levels today; the freshness stamp there is not
      affected by this change.

**Validates:** The doc-freshness ratchet stays green; new AI service
handlers pick up the pattern via the durable doc rather than by
imitating a specific existing site.

---

**Audit notes.** Consulted docs during planning and audit (via
`docs/llms.txt`), and the fixes applied during the audit pass:

- ⚓ `docs/server/architecture.md` (server anchor)
- ⚓ `docs/server/conventions.md` (log-level rules §L99-103; build
  gates §L144-158)
- ⚓ `docs/server/observability.md` (log levels §L72-96; leaf/fallback
  doctrine §L98-126; alert policies §L843-895)
- ⚓ `docs/testing/architecture.md` (testing anchor)
- ⚓ `docs/reviews/observability_review_prompt.md` (Areas to Review §1
  Logging: log levels, structured fields, error context)
- ⚓ `docs/reviews/error_handling_review_prompt.md` (§4 Logging &
  Observability; §5 Retry & Recovery — graceful degradation, circuit
  breakers)
- ⚓ `docs/reviews/testing_review_prompt.md` (§2 Server Testing — Error
  Path Testing, Table-Driven Tests; §4 Test Quality — race conditions)
- `docs/server/llm.md` (leaf: `server/ai/**`,
  `server/services/experience/**`) — audit confirmed the "graceful
  fallback" design principle is what Phase 2 is restoring for streaming
- `server/errs/README.md` + `server/errs/streak.go` — direct pattern
  model for the new `ai.LogAIError` helper (mirror
  `errs.LogJobError`'s shape and `LeveledLogger` interface)
- `docs/issues/2732-ai-leaf-error-log-level.md` and PR #2733 (prior
  doctrine, same alert class, one layer deeper)

Fixes applied during audit:
- Phase 1's helper signature now explicitly follows `errs.LogJobError`
  (`server/errs/streak.go:82`) and its `LeveledLogger` interface,
  keeping the codebase's transient-classification helpers uniform.
- Phase 1's test task now specifies "table-driven" and calls out the
  reference helper pattern from `provider_helpers_test.go`, aligning
  with `docs/reviews/testing_review_prompt.md` §2.
- Phase 1's build-verification checklist now cites the specific
  conventions-doc lines that make `go build -tags=integration ./...`
  mandatory for this change (new exported API consumed by
  integration-tagged tests).
- Phase 2 now adds a `docs/server/llm.md` freshness bump so the leaf
  doc stays in sync with the new "streaming has fallback" contract.

## Related Issues

- **#2761** — Sister alert fired at the same moment on the same
  incident. Same root cause, same fix; will close when this ships.
- **#2732 / PR #2733** — Prior fix that demoted leaf-provider `Error`
  logs to `Warn`. This plan is the direct successor: it applies the
  same doctrine to the service-handler layer that PR #2733 didn't
  touch, and adds the streaming-fallback behavior that PR #2733
  couldn't (out of scope).
- **#2711 / PR #2712** — Original "downgrade expected/handled failures"
  doctrine. Grandparent of this fix.
- **#2622 / #2623 / #2624** — Observability follow-ups. #2623 retired
  the legacy count-based `rpc_errors` alert; the `all_errors` policy
  that fired here is intentionally the "any ERROR log" tripwire, so
  keeping ERROR discipline at every log site remains the primary
  lever.
- **#1236, #973** — Recurring Vertex 429 pain in the test suite,
  historical evidence that RESOURCE_EXHAUSTED is a known transient
  class in this codebase.
- **#1907** — `ClassifyUnifiedCreate` origin. Confirms the classifier
  uses the fallback chain in the unary path (relevant because the
  `operation=UnifiedCreateClassify` alerts in this cluster are pre-
  PR-#2733 leaf logs, not classifier-specific).
