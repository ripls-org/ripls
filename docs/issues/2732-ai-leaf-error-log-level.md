# Issue #2732: [ops:warn] Service-level error logged (excluding middleware and infrastructure) — 2026-07-16

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2732
**Filed:** 2026-07-16 | **Last activity:** 2026-07-16
**Labels:** auto-filed, claude-fix, ops, severity:warn, gcp-policy:service-level-error-logged-exc-20260716
**Assignees:** unassigned
**Context docs:** `docs/server/observability.md` (log levels; alert policies; ⚓ `docs/server/architecture.md`, `docs/server/conventions.md`; `server/ai/README.md`; prior fix PR #2712 (DeleteGear soft-deleted log level).

## Problem Statement

The Cloud Monitoring policy `Server Error Logged - server` fired on
2026-07-16T21:33:12Z with:

```
message=gemini API call failed
operation=GenerateNudgeContent
request_id=00000000-0000-4000-8000-000000000010
```

The `all_errors` policy trips on any ERROR-level application log. In this
case the ERROR log was almost certainly a false positive: prod runs
`NewAllProvidersWithFallback` (`server/ai/provider_factory.go`), which
composes `FallbackProvider(gemini, [anthropic, openai])`. When the primary
Gemini call inside `geminiCallStructured` failed, the leaf helper at
`server/ai/provider_helpers.go:165` emitted the ERROR log **before**
returning, so the alert fired even if the FallbackProvider recovered via
Anthropic and the RPC returned success.

The FallbackProvider itself already gets the levels right — see
`server/ai/provider_fallback.go:143,151,154`: primary failure logs
WARN, per-fallback failure logs WARN, and only "all providers failed"
logs ERROR. But the leaf-provider helpers log ERROR unconditionally, so
the WARN-vs-ERROR discipline that the fallback wrapper enforces is
undone one call site earlier.

The same shape exists in every leaf provider today:

- `server/ai/provider_helpers.go:85,125,165` — the shared
  `openaiCallStructured` / `anthropicCallStructured` / `geminiCallStructured`
  helpers used by most operations (including `GenerateNudgeContent`).
- `server/ai/provider_gemini.go:263,269,364` — inline calls that bypass
  the helpers (`ParseInformalTime`, empty-response case, `ClassifyUnifiedCreate`).
- `server/ai/provider_openai.go:346,434` — inline calls
  (`ParseInformalTime`, unified-create classifier).
- `server/ai/provider_anthropic.go:380,465` — inline calls
  (`ParseInformalTime`, unified-create classifier).
- `server/ai/provider_gemini_streaming.go:115`,
  `provider_openai_streaming.go:159`,
  `provider_anthropic_streaming.go:164` — streaming paths.
- `server/ai/provider_loadbalanced.go:380,413` — the LoadBalancedProvider's
  own operation-level ERROR log (only used when weighted load balancing is
  configured — not the prod default, but relevant for eval and future
  configs).

The convention that governs this decision is
`docs/server/observability.md#L72-L96` (and its companion in
`docs/server/conventions.md#L99-L103`):

> WARN: Unexpected but handled situations (retries, fallbacks, slow queries)
> ERROR: Failures requiring attention (database errors, external API failures)
>
> User errors (invalid input, not found) are INFO or WARN, not ERROR. ERROR
> is reserved for system failures.

There is a surface tension between the two clauses — "external API
failures" (ERROR) vs "retries, fallbacks" (WARN). The observability doc
resolves it in the background-job transient-error paragraph immediately
below (§L81-96): transient failures that are handled by a retry / streak
mechanism stay at WARN, and only escalate to ERROR when the recovery
mechanism has exhausted itself. The `FallbackProvider` chain IS that
recovery mechanism for AI leaf calls, and its "all providers failed"
line at `provider_fallback.go:154` is the analog of the transient-streak
escalation surface. So the correct reading is: leaf ERROR-level
"external API failure" logs are only warranted when they are the
terminal, unrecovered failure — which the wrapper already handles
authoritatively.

## Context from Comments

The issue has no human comments — it was auto-filed by the
`gcp-alerts-to-github` Cloud Function pipeline
(`cloud_functions/src/gcpAlertsHandler.ts`) as a warn-severity aggregator
(`gcp-policy:service-level-error-logged-exc-20260716`).

**Prior instance:** #2711 (same policy, 2026-07-13) fired for
`DeleteGear` and was fixed in PR #2712 by downgrading the specific
already-soft-deleted branch from ERROR to WARN, keeping hard misses at
ERROR. That fix's doctrine — "downgrade the log level for expected /
handled failures; keep true system failures at ERROR" — is the same
doctrine to apply here.

**Related observability doctrine.** `docs/server/observability.md`
(lines 81–96) describes the `errs.TransientStreak` /
`errs.TransientPageAfter` pattern for background jobs: transient
failures log WARN until they persist past a window, then escalate to
ERROR. That pattern is a close cousin of what's needed here — the
fallback chain IS the streak, and its "all providers failed" ERROR at
`provider_fallback.go:154` is the escalation surface.

## Potential Solutions

### Option A: Downgrade the leaf-provider "API call failed" logs to WARN, unconditionally

- **Approach:** Change the ERROR calls in `provider_helpers.go` (3 sites),
  `provider_gemini.go` (3 sites), `provider_openai.go` (2 sites),
  `provider_anthropic.go` (2 sites), and the three `*_streaming.go` files
  (1 site each) from `ErrorContext` / `Error` to `WarnContext` / `Warn`.
  Keep the returned `error` unchanged so behavior for callers doesn't
  drift; only the log level changes. The escalation to ERROR then lives
  in exactly two places that already exist:
  - `FallbackProvider.withFallback` at `provider_fallback.go:154`
    ("all providers failed").
  - `LoadBalancedProvider.GenerateNudgeContent` /
    `GenerateStoryContent` at `provider_loadbalanced.go:380,413`
    (single-selected-provider error → this stays ERROR since load
    balancing has no fallback of its own).
- **Pros:**
  - Matches the observability doctrine ("retries, fallbacks → WARN")
    exactly.
  - Minimal, mechanical change — no new abstractions or signatures.
  - Aligns with what `FallbackProvider.withFallback` already does one
    layer up.
  - Preserves ERROR-level signal for the actually-failed case:
    * fallback chain exhausted → `provider_fallback.go:154` still ERROR.
    * load-balanced single provider fails → `provider_loadbalanced.go`
      still ERROR.
    * caller returns 5xx → `rpc_error_rate` alert catches it via
      `rpc_requests_{env}` (see observability doc §Alert Policies).
  - Fixes not just #2732 (Gemini/GenerateNudgeContent) but the whole
    class of alert — Anthropic/OpenAI leaf failures behind fallback also
    trip the same alert today.
- **Cons:**
  - A single-provider prod config (no fallback) would lose the
    leaf ERROR signal. Prod isn't configured that way today
    (`NewAllProvidersWithFallback` guarantees fallback whenever any of
    Anthropic/OpenAI keys are set), and the `rpc_error_rate` /
    RPC-response 5xx path already provides ERROR-level signal for
    RPC-facing calls. Callers of best-effort operations (nudges, stories,
    imagery) intentionally swallow errors — the loss there is intentional.
  - Loses the "raw API failure" grep target at ERROR. Mitigation: the log
    line stays; only the level changes. `severity>=WARN` and the message
    substring still find it.
- **Effort:** Small. ~12 log-line edits across 7 files. Plus test coverage
  (see Phase 2).

### Option B: Move the ERROR log from the leaf helper to the caller (per-operation)

- **Approach:** Delete the leaf-level ERROR log. Add ERROR logging at each
  operation caller (nudges.go, gear service, community service, etc.),
  based on how important the operation is. Best-effort callers
  (nudges) log WARN; user-facing RPC callers log ERROR.
- **Pros:** Per-caller granularity — a best-effort nudge failure is
  treated differently from a user-facing gear-generation failure.
- **Cons:**
  - Much larger blast radius: every `s.aiProvider.<method>` call site
    (dozens) would need a log-level judgment. Some already log the wrapped
    error, some don't; some log at WARN, some at ERROR.
  - Redundant with what the FallbackProvider already does (its WARN /
    ERROR split IS the operation-vs-leaf distinction).
  - Higher risk of missing sites and getting inconsistent behavior.
- **Effort:** Medium-to-large. Requires reviewing every AI provider call
  site.

### Option C: Fallback-aware leaf logging via ctx flag

- **Approach:** Have `FallbackProvider` set a context value like
  `ai.WrappedByFallback` before delegating; leaf helpers demote to WARN
  when set, keep ERROR otherwise.
- **Pros:** Preserves ERROR signal for genuinely-unwrapped single-provider
  configs.
- **Cons:**
  - Adds a new context-based signaling mechanism the codebase doesn't
    currently use for logging severity. Fragile — any composition change
    (LoadBalanced wrapping Fallback wrapping ...) requires updating the
    flag.
  - The unwrapped-single-provider case doesn't exist in prod, and the
    intended-single-provider setups (tests, eval harness) already
    tolerate WARN because they inspect returned errors, not logs.
  - Layering intent belongs at the wrapper, not the leaf — spreading it
    across abstractions is the anti-pattern the FallbackProvider was
    added to fix.
- **Effort:** Medium. New context key, tests for both directions.

## Recommendation

**Option A.** It's the minimum change that resolves the alert class,
matches the stated observability doctrine, and mirrors what the
FallbackProvider wrapper is already doing one layer up. The single
notional downside (losing the ERROR signal for a hypothetical
single-provider config) doesn't apply to the current prod topology, and
the RPC-response and `rpc_error_rate` paths keep the true-failure signal
intact.

Concretely for #2732: `geminiCallStructured` becomes a WARN, the
Gemini→Anthropic fallback still recovers, `FallbackProvider` logs its
existing WARN, and `all_errors` doesn't fire. When both providers
actually fail, `provider_fallback.go:154` still emits ERROR and the alert
still fires — with the correct semantics.

Do **not** touch `LoadBalancedProvider.GenerateNudgeContent`'s
`ErrorContext` at lines 380/413: that log fires when the load-balanced
selection has no fallback of its own, and a wrapping FallbackProvider (if
present) will then catch the returned error, log its own WARN, and try a
fallback. Since the LB layer doesn't know whether it's wrapped, keeping
its own log at ERROR would double-count with FallbackProvider's ERROR at
the top of the chain when everything fails — so LB should also be
downgraded to WARN for consistency with the same reasoning. **This is an
open question; see below.**

## Open Questions

- **Should `LoadBalancedProvider`'s ERROR logs at
  `provider_loadbalanced.go:380,413` also be downgraded to WARN?** —
  Recommendation: **yes**, for consistency. Prod uses
  `NewWeightedProviderWithFallback` in some configs (see factory line
  281), which wraps LB in a FallbackProvider — meaning LB's ERROR would
  fire before FallbackProvider had a chance to retry, replicating the
  exact bug pattern. Downgrade for the same reason. The "top of the
  chain" ERROR always comes from `FallbackProvider.withFallback` when a
  fallback is present, and from the calling RPC handler / job when it
  isn't.

- **Should the streaming leaf sites
  (`provider_{gemini,openai,anthropic}_streaming.go`) be included?** —
  Recommendation: **yes**. They emit the same class of "API call failed"
  ERROR log from inside the same leaf-provider layer. The streaming path
  is used by `StreamGenUnifiedCreate` which sits atop `FallbackProvider`
  in prod. Same reasoning, same fix.

- **Should we also handle the leaf-level "empty response from Gemini"
  ERROR at `provider_gemini.go:269`?** — Recommendation: **yes**,
  downgrade to WARN. Empty-response is functionally identical to
  API-call-failed from the wrapper's perspective — the wrapper still
  gets a non-nil error and retries. Same reasoning.

- **Do we need tests to prevent regression?** — Recommendation: **yes**.
  Add a table-driven test in `provider_fallback_test.go` (or a new
  `provider_helpers_test.go`) that asserts the leaf-helper log level is
  WARN, not ERROR, when the upstream errors. Use a slog test handler
  (`slogtest`, or a `*bytes.Buffer` handler + `logging.NewLogger` at
  DEBUG) to capture emitted records. This is the guardrail that catches
  a future accidental re-elevation to ERROR.

- **Should we update `docs/server/observability.md` with a paragraph
  documenting the "leaf provider logs WARN; fallback wrapper logs
  ERROR" contract?** — Recommendation: **yes**, add a short note under
  the "Appropriate Log Levels" section (near the background-job
  transient-errors paragraph at lines 81–96) so future contributors have
  the same guidance the code now embodies. Non-blocking; can land in
  Phase 3.

## Implementation Plan

### Phase 1: Downgrade leaf-level ERROR logs to WARN

**Goal:** Stop the false-positive alert; align leaf-provider log severity
with the observability doctrine.

**Tasks:**
- [ ] `server/ai/provider_helpers.go`: change `ErrorContext` → `WarnContext`
      at lines 85 (openai), 125 (anthropic), 165 (gemini). Keep field set
      unchanged; keep returned error unchanged.
- [ ] `server/ai/provider_gemini.go`: change `Error` / `ErrorContext` →
      `Warn` / `WarnContext` at lines 263 (API call failed), 269 (empty
      response), 364 (API call failed in ClassifyUnifiedCreate).
- [ ] `server/ai/provider_openai.go`: change lines 346 and 434.
- [ ] `server/ai/provider_anthropic.go`: change lines 380 and 465.
- [ ] `server/ai/provider_gemini_streaming.go` (line 115),
      `provider_openai_streaming.go` (line 159),
      `provider_anthropic_streaming.go` (line 164): same downgrade.
- [ ] `server/ai/provider_loadbalanced.go`: change lines 380 (`AI story
      generation failed`) and 413 (`AI nudge generation failed`) from
      `ErrorContext` to `WarnContext`.
- [ ] Do **not** touch `provider_fallback.go:154` (`all providers
      failed`) — this stays ERROR; it is the escalation surface.

**Validates:** Prod false-positive alerts on `gemini API call failed` /
`openai API call failed` / `anthropic API call failed` stop firing when
the FallbackProvider recovers.

### Phase 2: Test coverage for the log-level contract

**Goal:** Regression guard so the level can't silently drift back to
ERROR.

**Tasks:**
- [ ] Add a test file `server/ai/provider_helpers_test.go` (or extend an
      existing test file) with subtests for each of `openaiCallStructured`,
      `anthropicCallStructured`, `geminiCallStructured`. Use a
      `*bytes.Buffer`-backed `logging.NewLogger` with format=json,
      level=debug. Force an upstream error (test-double SDK client that
      always errors) and assert the captured record has `level=WARN` and
      the expected message substring.
- [ ] Extend `server/ai/provider_fallback_test.go` with a test where the
      primary returns an error, a fallback succeeds, and the emitted log
      records contain **no** ERROR-level entries. This is the
      end-to-end contract that #2732 codifies.
- [ ] Run:
      `go test -tags=integration -timeout 20m ./server/ai/...`
      `npm run build` (server-side build gate).
      Also run `go build -tags=integration ./...` per
      `docs/server/conventions.md#L152-L158`, because the ai package is
      consumed by integration-tagged tests in
      `server/services/experience/` and `server/services/unified_create/`.

**Validates:** Any future accidental re-elevation to ERROR (or new
provider added without the WARN convention) fails CI, not prod.

### Phase 3: Docs

**Goal:** Encode the contract in `docs/server/observability.md` so
reviewers and future contributors apply the same doctrine to new AI
providers or new leaf integrations.

**Tasks:**
- [ ] In `docs/server/observability.md`, near the "Appropriate Log
      Levels" and "Background-job transient errors" section (lines
      72–96), add a short paragraph titled something like
      "Leaf provider + fallback wrapper" documenting:
      - Leaf provider methods log **WARN** on upstream API errors —
        never ERROR.
      - `FallbackProvider.withFallback` owns the escalation to **ERROR**
        when all providers in the chain have failed.
      - Callers that receive a returned error (`s.aiProvider.<method>`
        result) can log at their own preferred level based on operation
        semantics (best-effort → WARN; user-critical → ERROR).
- [ ] Bump `freshness.verified_commit` / `verified_on` in the doc's
      frontmatter per `docs/context_map.md`.
- [ ] `docs/server/llm.md`'s "Logging" section (lines 155–166) states
      that "each provider implementation logs at method entry, on error,
      and on completion with timing information" — this stays accurate
      without a change; it doesn't specify a log level. No frontmatter
      bump needed unless we choose to add a cross-reference to the new
      observability paragraph.

**Validates:** New provider integrations pick up the pattern by reading
the doc; the doc-freshness ratchet stays green.

---

**Audit notes (Phase 4 of ripls-issue skill).** This plan was audited
against the ripls documentation context map. Consulted docs:

- ⚓ `docs/server/architecture.md` (server anchor)
- ⚓ `docs/server/conventions.md` (server anchor; log-level rules L99–L103)
- ⚓ `docs/server/observability.md` (observability anchor; log-level +
  alert-policy sections L72–L96, L843–L865)
- ⚓ `docs/reviews/observability_review_prompt.md` (Areas to Review §1
  Logging: log levels, structured fields, error context)
- ⚓ `docs/reviews/error_handling_review_prompt.md` (Areas to Review §4
  Logging & Observability; §5 Retry & Recovery — graceful degradation)
- ⚓ `docs/reviews/testing_review_prompt.md` (Areas to Review §2 Server
  Testing — Error Path Testing, Table-Driven Tests)
- ⚓ `docs/reviews/code_quality_review_prompt.md` (Areas to Review — DRY,
  dead code, log discipline)
- ⚓ `docs/reviews/tech_debt_review_prompt.md` (this alert class is a
  recurring debt signal — see #2711 fix history)
- `docs/server/llm.md` (leaf: `server/ai/**`, `server/services/experience/**`)
- `docs/reviews/reports/` history: prior fix PR #2712 (same policy,
  same doctrine)

Findings applied inline:
- Added the WARN-vs-ERROR convention-tension resolution (observability
  doc §L81-96 background-job pattern) to Problem Statement.
- Added `go build -tags=integration ./...` requirement in Phase 2 per
  `docs/server/conventions.md#L152-L158`.
- Confirmed `docs/server/llm.md`'s Logging section stays accurate
  under this change (no level specified there).

## Related Issues

- **#2711 / PR #2712** — Same policy (`gcp-policy:service-level-error-logged-exc-*`).
  Prior fix followed the same doctrine: downgrade expected/handled ERROR
  to WARN, keep true system failures at ERROR. This plan generalizes the
  doctrine to the AI leaf-provider layer.
- **#2622 / #2623 / #2624** — Observability follow-ups. #2623 retired the
  legacy count-based `rpc_errors` alert; the `all_errors` policy that
  fired here is intentionally kept as the "any ERROR log" tripwire — so
  keeping ERROR discipline (only true failures) at the source of every
  log line is now the primary lever.
- **#1237** — `ai.CallWithTimeout` helper (context of how AI calls compose
  timeout + fallback + operation-level error handling in `nudges.go`).
- **#1613** — SLI/SLO log-based metrics (5xx error rate now covers the
  actual RPC-response failure surface, independent of leaf log levels).
