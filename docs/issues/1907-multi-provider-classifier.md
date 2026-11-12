# Issue #1907: unified-create P2.16: Multi-provider classifier via the existing provider abstraction

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1907
**Filed:** 2026-05-13 | **Last activity:** 2026-05-13
**Labels:** (none) | **Assignees:** unassigned
**Context docs:** none (issue is self-contained; this plan also pulls from `docs/design/unified-create.md` § Architecture, § LLM design, § Phase 2)

## Audit findings (applied 2026-05-13, revised after Option-A decision)

Cross-checked against `docs/server/architecture.md` and `docs/server/observability.md`. The user selected **Option A** (extend the existing `Provider` interface) after the initial audit; the findings below have been re-cast for that direction.

1. **Log field rename `classifier_provider` → `provider`** (still applies, even stronger under Option A). Every other method on `Provider` already logs `provider=anthropic|gemini|openai` via the per-implementation `Classify`/`Generate*` methods. Adding `ClassifyUnifiedCreate` inherits the same field — the existing dashboards segment automatically by `provider` once the `operation=UnifiedCreateClassify` rows arrive. For the fallback case, `FallbackProvider` logs the **served-by** provider name in the `provider` field directly (see `withFallback` in `provider_fallback.go:127`), so no `served_by` field is needed under Option A.
2. **Health check is free under Option A.** `FallbackProvider.CheckHealth` (`server/ai/provider_fallback.go:426`) already aggregates per-provider statuses; the existing `healthService.RegisterFunc("ai", aiProvider.CheckHealth)` in `main.go` already exposes them under `/health`. No new `health.Checker` to register.
3. **Fallback policy stays as-is for now.** `provider_fallback.go`'s `withFallback` falls back on *any* error today. The classifier benefits from `ai.IsTransientError` (`server/ai/errors.go:5`) gating, but applying it asymmetrically (classifier-only) inside `withFallback` creates a behavior split that's worse than the cost it saves. **Plan deferred to the targeted `Provider` refactor.** For this issue: accept that a `ClassifyUnifiedCreate` parse error wastes one extra call to the fallback before failing — the same as every other method on `Provider` does today. File a separate issue if the wasted-call cost shows up in latency telemetry.
4. **Observability doc edit is an explicit file/line target.** Phase 4 names the file (`docs/server/observability.md`) and the section (the unified-create stream table at lines ~772–784) to update with the new classifier fields.
5. **Service-independence preserved via service-local interface alias.** `server/services/unified_create/classifier.go` keeps `type Classifier interface { Classify(...) (*ai.UnifiedCreateClassification, error) }` as the service-local narrow interface. A tiny adapter (`type providerClassifier struct{ p ai.Provider }`) wraps `Provider` into `Classifier` for production wiring. The stub and integration-test `mockClassifier` remain trivial — they implement the one-method `Classifier`, not the full `Provider`. This satisfies `docs/server/architecture.md` § Pattern 2 (Interface-Based Dependencies) and keeps test surface small.
6. **Mock strategy.** `docs/server/architecture.md` § Testing Strategy → Mock Strategy says: "Define mocks in the library package." `MockProvider` (`server/ai/provider_mock.go`) gets a new `ClassifyUnifiedCreate` method that returns a fixed classification; service tests reuse it through the narrow `Classifier` interface.
7. **Stream-error surface confirmed.** Classifier failures (after fallback exhaustion) are converted to `GEN_STREAM_ERROR_CODE_*` events by the unified-create handler — never to bare provider error text on the wire. No CLAUDE.md `connecterr.Internal` change needed because the classifier is called inside a streaming dispatch, not directly from a Connect handler. Phase 3 calls this out so an implementer doesn't accidentally surface raw errors.
8. **Goroutine safety not impacted.** Fallback is sequential (try primary → fallbacks in order), no goroutines. The `logging.GoSafe` requirement from `docs/server/architecture.md` § Goroutine Safety doesn't apply.
9. **Interface bloat acknowledged, deferred.** Adding a method to `Provider` means touching `AnthropicProvider`, `GeminiProvider`, `OpenAIProvider`, `FallbackProvider`, `LoadBalancedProvider`, and `MockProvider`. The user has explicitly accepted this and scoped the cleanup to a targeted `Provider` refactor follow-up. See § Deferred follow-up: targeted `Provider` refactor.

## Problem Statement

The unified-create classifier is a single-provider island in an otherwise multi-provider AI surface. Every other AI capability on the server — `DetectGearInImage`, `GenerateExperienceFromText`, `ParseInformalTime`, etc. — runs through a `FallbackProvider` that chains Gemini (primary) → Anthropic (fallback) → OpenAI (tertiary), configured once in `provider_factory.go`. The classifier doesn't:

- `server/ai/classifier_unified.go:48` defines `AnthropicUnifiedCreateClassifier` and only that.
- `server/main.go:1155-1158` wires it directly: `ai.NewUnifiedCreateClassifierFromAPIKey(cfg.anthropicAPIKey, ...)` with a stub fallback when no Anthropic key is set.
- The classifier hits the Anthropic SDK directly (`anthropic.NewClient`, `anthropicCallStructured`), not the `Provider` interface that the per-type generators consume.

Three operational gaps follow from this:

1. **No resilience.** If Anthropic returns 5xx or rate-limits, the entire unified-create flow fails — even though the per-type generators it would delegate to could keep serving traffic from Gemini.
2. **No provider A/B.** The design doc (`docs/design/unified-create.md` § Option A, line 404) explicitly calls out swapping the classifier as a near-term lever. There's no swap surface today.
3. **Eval is silently single-provider.** The P2.14 accuracy gate (#1880) will measure Anthropic-only behavior. If we later swap to Gemini we have no baseline.

The fix is small in shape: the `UnifiedCreateClassifier` interface (`classifier_unified.go:41`) already exists; we just need a second implementation (Gemini) and a fallback wrapper, then route construction through `provider_factory.go` instead of `main.go`.

There's a subtle modeling question hiding here: do we **add** `ClassifyUnifiedCreate` to the existing big `Provider` interface (and get fallback "for free" via the existing `FallbackProvider`), or do we **keep** the standalone `UnifiedCreateClassifier` interface and build a small parallel fallback wrapper for it? See Options below — they trade off interface-bloat against duplication of the fallback pattern.

## Context from Comments

No comments on the issue yet — it was just filed. This plan stands on the issue description plus the related design doc and the current code shape. Flag in Open Questions: the modeling choice (extend `Provider` vs. keep the standalone interface) should be confirmed before implementation starts.

## Potential Solutions

### Option A — Fold `ClassifyUnifiedCreate` into the existing `Provider` interface

- **Approach:** Add `ClassifyUnifiedCreate(ctx, in) (*UnifiedCreateClassification, error)` to the `Provider` interface. Implement it on `AnthropicProvider`, `GeminiProvider`, and `OpenAIProvider`. Add a `withFallback` method on `FallbackProvider`. Delete the standalone `UnifiedCreateClassifier` interface — the unified-create service depends on `Provider` directly (it already depends on `aiProvider` for per-type generation; the classifier becomes another method on the same handle).
- **Pros:**
  - One source of truth for provider routing. No parallel infrastructure.
  - Fallback, load-balancing (`provider_loadbalanced.go`), and `MethodFallbacks` overrides come for free.
  - Health check (`CheckHealth`) automatically covers the classifier.
  - Logging-wise, the `provider` field already discriminates on every call — `classifier_provider` becomes redundant; existing `provider` covers it.
- **Cons:**
  - The `Provider` interface is already large (~20 methods). Each new method is a maintenance tax across three implementations, the fallback wrapper, the load-balancer, and the mock.
  - Stub-classifier story changes: today `unified_create.NewStubClassifier()` is the no-API-key path. With Option A the stub would have to implement the full `Provider` interface, which it doesn't want to. Workaround: keep `unified_create.Classifier` as a service-local interface (`type Classifier interface { Classify(...) }`), satisfied by both the stub and `Provider`. That's a small adapter, not a parallel infrastructure.
- **Effort:** Medium. Touches all three providers + fallback + load-balancer + mock + tests.

### Option B — Keep the standalone `UnifiedCreateClassifier` interface, add a parallel mini-fallback

- **Approach:** Add `GeminiUnifiedCreateClassifier` (mirrors `AnthropicUnifiedCreateClassifier`, calls Gemini via the existing Vertex AI plumbing in `provider_gemini.go` — likely a small helper-extraction so we don't duplicate Vertex client construction). Add `FallbackUnifiedCreateClassifier{ primary, fallbacks []UnifiedCreateClassifier }` that mirrors `withFallback` from `provider_fallback.go` for this one method. Wire construction in `provider_factory.go` next to the main `Provider` construction; expose a new `BuildUnifiedCreateClassifier(...)` helper or extend the existing factory return to include it.
- **Pros:**
  - `Provider` interface stays the same size.
  - The standalone interface is more honest about scope — a classifier doesn't need to be able to `GenerateConversationSummary`.
  - Mock / stub classifier in tests stays trivial — no need to implement a giant interface.
  - Eval runner (`server/ai/eval/unified_classifier_test.go`) keeps its current shape; just iterates over a list of `UnifiedCreateClassifier` instances rather than provider-method tuples.
- **Cons:**
  - Duplicates the fallback pattern (a smaller version of `provider_fallback.go`). Two places to fix if the fallback semantics ever change.
  - Two parallel construction sites in `provider_factory.go` (the big `Provider` plus the classifier).
  - Health checks for classifier providers are separate from the main provider health roll-up unless we explicitly merge them.
- **Effort:** Small-to-medium. New files are small; no touch on the existing `Provider` interface or its implementations.

### Option C — Hybrid: classifier consumes `Provider` but exposes its own interface

- **Approach:** Extend the `Provider` interface with one low-level primitive (`StructuredCompletion(ctx, prompt, schemaName) (json.RawMessage, error)` or similar) that both the classifier and any future short structured-output use cases can ride. Then `UnifiedCreateClassifier` is implemented once, on top of `Provider`, and gets fallback via the existing `FallbackProvider`.
- **Pros:**
  - Most elegant in the long run: one provider abstraction with one low-level primitive, all higher-level features composed on top.
  - Future classifiers / short structured calls (e.g. `ParseInformalTime` already kind of fits this pattern) collapse onto the same primitive.
- **Cons:**
  - Scope creep. Designing a clean `StructuredCompletion` primitive that handles both Anthropic's tool-use-style JSON and Gemini's response-schema-style JSON is its own non-trivial design exercise. Not a Phase 2 task.
  - Anthropic and Gemini have meaningfully different "structured output" mechanisms — pretending they're the same primitive may force lowest-common-denominator behavior.
- **Effort:** Large. Real design work first.

## Recommendation

**Option A** — extend the `Provider` interface with `ClassifyUnifiedCreate`. Updated 2026-05-13 after user direction: "Option A of extending provider. We can do a targeted refactor of provider later."

Reasoning:

- One source of truth for provider routing. The classifier picks up `FallbackProvider`, `LoadBalancedProvider`, `MethodFallbacks` overrides, and `CheckHealth` aggregation **for free**. No parallel mini-fallback to maintain.
- `provider` log field already discriminates on every call — matches `docs/server/observability.md` § Standard Field Names without needing a new field name or a new pattern.
- A future "targeted refactor of `Provider`" (call it the long-term Option C destination — a smaller `StructuredCompletion` primitive that the classifier and other short structured-output methods compose on top of) becomes easier from this position: the classifier already lives on the same interface, so collapsing the per-method surface later is a refactor inside `Provider`, not a migration across two interfaces.

**Tradeoffs accepted:**

- `Provider` interface grows by one method, plus a new method on `FallbackProvider`, `LoadBalancedProvider`, and `MockProvider`. The interface was already wide; this issue acknowledges the cost and defers the cleanup to the follow-up refactor.
- The service-local `unified_create.Classifier` type alias in `server/services/unified_create/classifier.go` stays — it narrows the surface to `Classify(...)` only, so the stub classifier and integration-test `mockClassifier` don't have to implement the full `Provider`. A small adapter wraps `Provider` into `Classifier` for the production wiring.

## Deferred follow-up: targeted `Provider` refactor

Once this issue lands and we have a second cross-provider structured-output use case (the classifier is the first), file a follow-up to extract a `StructuredCompletion(ctx, prompt, schemaName, target any) error` primitive on `Provider` and rebuild `ClassifyUnifiedCreate` plus any similar short-output methods on top of it. Not scoped here — the existing wide-interface pattern is the right local minimum until there's a second consumer.

## Open Questions

- **Should `ClassifyUnifiedCreate` parse errors skip fallback?** A model returning invalid JSON is a prompt bug, not a provider outage — falling back wastes a call. **But:** every other method on `Provider` currently falls back on any error via `withFallback`, and tightening only `ClassifyUnifiedCreate` creates a behavior split. — **Recommendation:** accept the wasted-call cost for now (matches existing `Provider` behavior). If it shows up in latency telemetry from P2.12 (#1878), file a follow-up to add an `ai.IsTransientError`-aware variant of `withFallback` that all methods opt into.
- **Where does Gemini structured-output land — Vertex SDK's `ResponseSchema` or prompt-engineered JSON?** Anthropic uses `anthropicCallStructured` (tool-use-style). Gemini has a first-class `responseSchema` on `GenerationConfig`. — **Recommendation:** use Gemini's native `responseSchema`. Extract a `geminiCallStructured` helper symmetric to `anthropicCallStructured` so future structured-output methods on `GeminiProvider` can reuse it.
- **Stub classifier in the no-API-key path: should the stub be removed once Gemini is wired (since Vertex auth is typically available even when Anthropic isn't)?** — **Recommendation:** keep the stub as the floor for environments with no provider configured (CI without secrets, integration tests without `ANTHROPIC_API_KEY` and without `VERTEXAI_PROJECT`). The narrow service-local `Classifier` interface (`server/services/unified_create/classifier.go`) keeps the stub trivial — just one method to implement. Removing it is a separate cleanup.
- **OpenAI as tertiary classifier?** OpenAI is already in the `Provider` fallback chain for other methods. Adding `ClassifyUnifiedCreate` to `OpenAIProvider` means it's automatically in the chain for the classifier too. — **Recommendation:** yes, implement on `OpenAIProvider` in Phase 1. It's the same pattern, costs little, and there's no reason to leave the third provider out when the fallback chain already includes it. Phase 3's eval runner reports per-provider accuracy for all three.
- **Eval threshold per-provider vs. unified?** P2.14 (#1880) is the threshold gate. — **Recommendation:** report per-provider in the eval runner. Defer the threshold decision to #1880; that issue can pick a single floor or per-provider floors once it sees real numbers on the 17-entry corpus expanded to ≥20/bucket.
- **Health check rollup.** — **Resolved.** Option A puts the classifier on `Provider`, so `FallbackProvider.CheckHealth` and the existing `healthService.RegisterFunc("ai", aiProvider.CheckHealth)` registration in `main.go` cover the classifier with no extra wiring. No new `health.Checker` to register.
- **Schema-shape decision for `geminiCallStructured`.** Build the JSON schema from the Go type via reflection, or hard-code it as a literal struct in the call site? — **Recommendation:** literal struct at the call site for the classifier (one schema, stable shape, easy to read). If a second short structured-output method on `GeminiProvider` appears, generalize then.

## Implementation Plan

Option A: extend the `Provider` interface with `ClassifyUnifiedCreate`. Each phase ships a green build.

### Progress

| Phase | Status | Branch / commit |
|---|---|---|
| Phase 0 — Plan + design-doc edit | Done | `d5a4ce6c4` |
| Phase 1 — Extend `Provider` + implement on all providers | Done | `318519cd9` |
| Phase 2 — Service-local adapter + main.go wiring | Done | `7c89a485a` |
| Phase 3 — Eval runner per-provider accuracy | Done | `0fafd170e` + `2902d64f1` (refinement) |
| Phase 4 — Observability + design-doc handoff | Done | `8a6821de0` |

### Phase 1 notes

- All four leaf providers got fully working implementations (not stubs as the original plan hedged): `AnthropicProvider`, `GeminiProvider`, `OpenAIProvider`, and `MockProvider`. Each reuses the existing structured-output helper for that provider (`anthropicCallStructured`, `geminiCallStructured`, `openaiCallStructured`).
- `FallbackProvider` and `LoadBalancedProvider` got the standard pattern: a new entry in their per-method routing struct + `getFallbacksForMethod` / `getProvidersForMethod` case, plus the wrapper method itself.
- The old standalone `AnthropicUnifiedCreateClassifier` struct in `server/ai/classifier_unified.go` is **still in place** — Phase 2 deletes it as part of the main.go wiring swap.
- Tests added: `TestFallbackProvider_ClassifyUnifiedCreate` (primary-succeeds / primary-fails-fallback-succeeds / all-fail) and `TestMockProvider_ClassifyUnifiedCreate` (default + override + call tracking).

### Phase 2 notes

- Added `providerClassifier` adapter + `NewProviderClassifier` in `server/services/unified_create/classifier.go` — translates the narrow `Classify(...)` interface to `Provider.ClassifyUnifiedCreate(...)`. Keeps the service decoupled from the full `Provider` surface; stubs and `mockClassifier` stay trivial.
- `server/main.go` now constructs the classifier via `unified_create.NewProviderClassifier(aiProvider)` when a provider is configured; falls back to the stub otherwise.
- Deleted `AnthropicUnifiedCreateClassifier` struct, `NewAnthropicUnifiedCreateClassifier`, `NewUnifiedCreateClassifierFromAPIKey`, and the package-private `anthropicOpts` helper from `server/ai/classifier_unified.go`. The file now holds only `UnifiedCreateClassifierInput`, the narrow `UnifiedCreateClassifier` interface (kept for the eval runner), and `DefaultAnthropicModel`.
- `server/ai/eval/unified_classifier_test.go` was updated minimally to construct `ai.NewAnthropicProvider(...)` directly and call `ClassifyUnifiedCreate`. Phase 3 reworks this to run per-provider.
- READMEs updated: `server/services/unified_create/README.md` describes the adapter + `Provider`-chain shape; `server/ai/README.md` adds `ClassifyUnifiedCreate` to the documented interface.

### Phase 3 notes

The first cut of this phase put the test behind `//go:build integration` with bespoke flag plumbing. Reviewer feedback: match the pattern used by every other Gen\* eval test — live APIs must only be called under the **`benchmark`** build tag (not `integration`), via the shared `providersForEval` / `requireMode` / `retryOnRateLimit` / `Tally` / `reportAndGate` helpers in `server/ai/eval/`. Rewrote accordingly.

Final shape:

- **`server/ai/eval/unified_classifier.go`** (non-test): `LoadUnifiedClassifierGoldens` loader + `EvaluateUnifiedClassifier` evaluator. One `Check` per case on the `classified_type` field — passes when `result.Type` equals `ExpectedType` or any `AltTypes` entry. Mirrors `community.go` / `experience.go` / `gear.go` shape.
- **`server/ai/eval/unified_classifier_test.go`** (`//go:build benchmark`): `TestUnifiedClassifierPrompt` runs the goldens against every credentialed `(provider, model)` pair returned by `providersForEval(t)`, with per-pair `t.Run(pm.Key(), ...)`, `retryOnRateLimit` on the API call, `tally.Record(chk)` for each case, and `reportAndGate(t, tally, pm.Key())` to gate aggregate pass rate against `modelThresholds` (defaults to 0.85 from `threshold_test.go`).
- All ad-hoc flag plumbing (`-unified-classifier-*-key`) deleted — the shared `provider_setup_test.go` flags (`-anthropic-api-key`, `-gemini-project`, `-openai-models`, etc.) cover every provider × model invocation.
- Verified gating:
  - `go test ./server/ai/eval/` — `TestUnifiedClassifierPrompt` is **not** present (correct: `benchmark` tag absent).
  - `go test -tags=integration ./server/ai/eval/` — also not present (correct: this test belongs in `benchmark`, not `integration`).
  - `go test -tags=benchmark -list=. ./server/ai/eval/` — `TestUnifiedClassifierPrompt` listed alongside the other `Test*Prompt` suites.
- Pre-existing `TestAnthropicProvider_CheckHealth_ValidAPIKey` integration test fails locally on stale env keys; unrelated to this change (passes with `ANTHROPIC_API_KEY=` unset).
- Threshold tuning per (provider, model) deferred to #1880 (P2.14) — same pattern as the other `Test*Prompt` suites consume `modelThresholds`.

### Phase 4 notes

- `docs/server/observability.md` § Unified-create stream — table updated. Classifier-stage log lines now carry the standard `provider` and `model` fields emitted by every method on `ai.Provider`; under `FallbackProvider.withFallback` the `provider` field reflects the implementation that actually served the response, so a fallback-served call is visible directly in the existing log shape (no new field needed). A short note above the table points readers at #1907 for the migration context.
- `docs/design/unified-create.md` § Architecture (Server) — added a "Classifier lives on `ai.Provider`" paragraph documenting the `ClassifyUnifiedCreate` shape and the `unified_create.NewProviderClassifier` adapter.
- `docs/design/unified-create.md` § Phase 2 — row 16 marked **Done**, Option A choice recorded, follow-up refactor (#1908) linked.
- Comment left on #1880 (P2.14) summarizing the threshold-tuning surface this work delivers: per-(provider, model) overrides in `modelThresholds`, plus the need to grow the goldens corpus to ≥20/bucket before locking in numbers.
- Filed **#1908** — *Targeted refactor: extract a `StructuredCompletion` primitive on `ai.Provider`*. Scoped to deferred work from this plan's recommendation; not part of #1907.

### Phase 1: Extend the `Provider` interface and stub all implementations

**Goal:** `ClassifyUnifiedCreate` exists on `Provider`. Every implementation compiles. Anthropic implementation moves over from `classifier_unified.go`; the rest temporarily return a "not implemented" error gated for fallback. Nothing is wired yet — `main.go` still uses the direct Anthropic classifier.

**Tasks:**
- [ ] Add method to `server/ai/provider.go`'s `Provider` interface:
  ```go
  ClassifyUnifiedCreate(ctx context.Context, in UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error)
  ```
- [ ] Move the existing `AnthropicUnifiedCreateClassifier.Classify` body into a new method `(*AnthropicProvider).ClassifyUnifiedCreate` in `server/ai/provider_anthropic.go`. Use the same `anthropicCallStructured` helper; reuse the same prompt builder and structured-logging fields. The provider already carries `client`, `model`, `temperature` — no new fields needed.
- [ ] Add `(*GeminiProvider).ClassifyUnifiedCreate` in `server/ai/provider_gemini.go`. Implement via Vertex AI SDK's `GenerationConfig.ResponseSchema` + `ResponseMimeType: "application/json"`. If `provider_gemini.go` doesn't already have a structured-output helper symmetric to `anthropicCallStructured`, extract one as `geminiCallStructured[T any]` in `provider_gemini.go` (or `provider_gemini_structured.go` if `provider_gemini.go` is at the file-size threshold).
- [ ] Add `(*OpenAIProvider).ClassifyUnifiedCreate` in `server/ai/provider_openai.go` using OpenAI's structured-output JSON schema feature. Same parse + unknown-type rejection logic as the Anthropic implementation.
- [ ] Add `(*FallbackProvider).ClassifyUnifiedCreate` in `server/ai/provider_fallback.go` via the existing `withFallback` helper:
  ```go
  func (p *FallbackProvider) ClassifyUnifiedCreate(ctx context.Context, in UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error) {
      return withFallback(ctx, "ClassifyUnifiedCreate",
          func() (*UnifiedCreateClassification, error) { return p.primary.ClassifyUnifiedCreate(ctx, in) },
          p.getFallbacksForMethod("ClassifyUnifiedCreate"),
          func(fb Provider) (*UnifiedCreateClassification, error) { return fb.ClassifyUnifiedCreate(ctx, in) },
      )
  }
  ```
  Add `ClassifyUnifiedCreate []Provider` to `MethodFallbacks` and a matching `case "ClassifyUnifiedCreate"` to `getFallbacksForMethod`.
- [ ] Add `(*LoadBalancedProvider).ClassifyUnifiedCreate` in `server/ai/provider_loadbalanced.go` following the same pattern as its sibling methods.
- [ ] Add `(*MockProvider).ClassifyUnifiedCreate` in `server/ai/provider_mock.go` — returns a fixed `gear` classification with confidence 0.9, or a configured override field for test specificity.
- [ ] Unit tests:
  - `server/ai/provider_anthropic_test.go` (or new file if existing is at limit): verify `ClassifyUnifiedCreate` dispatches the text vs. webpage vs. image prompt, parses JSON, rejects unknown types.
  - `server/ai/provider_gemini_endpoint_test.go` (or new): same coverage for the Gemini path with a mocked Vertex client.
  - `server/ai/provider_fallback_test.go`: primary error → fallback called → result returned; all fail → error includes per-provider context.
  - `server/ai/provider_loadbalanced_test.go`: weighted routing applies to `ClassifyUnifiedCreate` the same as other methods.

**Validates:** `npm run build` clean (gofumpt/godot/revive + `go build ./...`). All existing tests pass. New `ClassifyUnifiedCreate` method works in isolation per provider.

### Phase 2: Service-local adapter + main.go wiring

**Goal:** `server/services/unified_create` consumes the classifier through its narrow `Classifier` interface, satisfied by a thin adapter over `Provider`. `main.go` constructs the classifier from `aiProvider`, not from a direct Anthropic call. The standalone `AnthropicUnifiedCreateClassifier` struct + `NewUnifiedCreateClassifierFromAPIKey` constructor are removed.

**Tasks:**
- [ ] In `server/services/unified_create/classifier.go`: keep the `Classifier` interface alias and add an adapter:
  ```go
  // providerClassifier adapts ai.Provider to the narrow Classifier
  // interface so the unified-create service can stay decoupled from
  // the full Provider surface.
  type providerClassifier struct{ p ai.Provider }

  func NewProviderClassifier(p ai.Provider) Classifier { return &providerClassifier{p: p} }

  func (c *providerClassifier) Classify(ctx context.Context, in ClassifierInput) (*ai.UnifiedCreateClassification, error) {
      return c.p.ClassifyUnifiedCreate(ctx, in)
  }
  ```
- [ ] Update `server/main.go:1155-1158` to construct the classifier through the provider:
  ```go
  var unifiedClassifier unified_create.Classifier = unified_create.NewStubClassifier()
  if aiProvider != nil {
      unifiedClassifier = unified_create.NewProviderClassifier(aiProvider)
  }
  ```
- [ ] Delete `AnthropicUnifiedCreateClassifier` struct, `NewAnthropicUnifiedCreateClassifier`, `NewUnifiedCreateClassifierFromAPIKey`, and `anthropicOpts` from `server/ai/classifier_unified.go`. Keep the `UnifiedCreateClassifierInput`, `UnifiedCreateClassification`, and `UnifiedCreateClassifier` types/interface — the prompt builder + classification types remain shared (the interface itself becomes unused-by-production but stays for the eval runner's convenience; if it has no remaining consumers, delete it too).
- [ ] Update `server/services/unified_create/integration_test.go` — the `mockClassifier` type already implements `Classifier`; verify the integration tests still pass against the adapter. No production-path test changes.
- [ ] Update `server/services/unified_create/README.md` and `server/ai/README.md` to describe the classifier-on-`Provider` shape, the narrow service-local `Classifier` interface, and the adapter.
- [ ] Confirm the unified-create handler still wraps classifier failures as `GEN_STREAM_ERROR_CODE_*` typed events (no raw provider error strings on the wire). Existing `stream_gen.go` already does this; just re-verify after the adapter swap.

**Validates:** `npm run build` clean; `go test ./server/ai/ ./server/services/unified_create/` green; manual smoke test of `StreamGenUnifiedCreate` with the dev `aiProvider` (Gemini primary, Anthropic fallback) returns a classification. `/health` continues to report the AI provider chain with no extra registration needed (the existing `aiProvider.CheckHealth` covers it).

### Phase 3: Eval runner extension

**Goal:** The integration eval runner reports per-provider classifier accuracy on the goldens corpus, so #1880 (P2.14) has data to base a threshold on.

**Tasks:**
- [ ] In `server/ai/eval/unified_classifier_test.go`: replace the single-classifier loop with one that instantiates each configured provider directly (bypassing `FallbackProvider` so we measure each implementation in isolation). Construction is gated on the same env-var pattern as `provider_factory.go` — `ANTHROPIC_API_KEY` → Anthropic; `VERTEXAI_PROJECT` → Gemini; `OPENAI_API_KEY` → OpenAI. Skip any provider without credentials; fail with a clear `t.Skip()` when none are available.
- [ ] Report per-provider accuracy via `t.Logf` in a stable format the dashboard / #1880 can later parse, e.g. `provider=anthropic accuracy=0.82 corpus_size=17`.
- [ ] Do not assert thresholds (still #1880's scope).

**Validates:** `go test -tags=integration ./server/ai/eval/ -run UnifiedClassifier -v` outputs per-provider accuracy lines for every configured provider.

### Phase 4: Observability + design-doc handoff

**Goal:** The unified-create observability section reflects that `provider` now varies, and the design doc records the Option-A decision + deferred refactor.

**Tasks:**
- [ ] Update the `Unified-create stream` field table in `docs/server/observability.md` (lines 772–784): the `classifier_duration_ms` row exists; add a `provider` row noting it discriminates `anthropic` / `gemini` / `openai` on classifier log lines (and is set by the underlying `FallbackProvider.withFallback` to the served-by provider when fallback engages). Add a one-line note above the table that classifier observability now follows the standard `Provider` log-field shape, so it segments cleanly on the existing `rpc_errors` policy and any future per-provider dashboard.
- [ ] Update `docs/design/unified-create.md` § Architecture (Server) to note the classifier rides the standard `Provider` interface + fallback chain. Update the § Phase 2 row added in this issue to link this plan and note Option A was chosen.
- [ ] Leave a one-paragraph comment on #1880 (P2.14) summarizing the per-provider accuracy numbers from Phase 3 and recommending whether the threshold gate should be a single global floor or per-provider floors.
- [ ] File the follow-up refactor issue: "Targeted refactor: extract `StructuredCompletion` primitive on `Provider`" — capture the long-term collapsing of `ClassifyUnifiedCreate` and any future short structured-output methods onto a shared primitive. Reference this plan as motivation.

**Validates:** The dashboard, threshold-gate, and refactor-followup issues have everything they need to proceed without re-touching the classifier code.

## Related Issues

- **#1878** (P2.12) — Classifier latency dashboard. The standard `provider` log field (already emitted by every `Provider` implementation today and inherited by `ClassifyUnifiedCreate` under Option A) is what #1878's dashboard pivots on for per-provider latency.
- **#1880** (P2.14) — Eval threshold gate for classifier accuracy. This issue produces per-provider accuracy numbers on the existing goldens corpus; #1880 consumes those numbers to pick a floor.
- **#1881** (P2.15) — Eval threshold gate for classifier round-trip latency. Per-provider latency from #1878's dashboard feeds this; this issue is independent of #1881 but should not regress p95.
- **#1879** (P2.13) — Speculative pre-fetch. Independent of this issue. The speculative pre-fetch reads the classifier confidence regardless of which provider produced it; provider identity is opaque to that flow.
- **#1906** — Coverage tests for `server/genai/*`. Adjacent cleanup; not coupled.
