# Issue #1333: [code_quality] AI provider methods duplicate ~60-line scaffolding across three files

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1333
**Filed:** 2026-04-25 | **Last activity:** 2026-04-25
**Labels:** auto-filed, review:code_quality | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Twenty Provider interface methods are implemented three times — in
`server/ai/provider_openai.go`, `provider_anthropic.go`, and `provider_gemini.go` — with
nearly identical scaffolding in every copy. Each implementation repeats:

1. Logger setup with standard fields
2. `startTime := time.Now()`
3. A local `type outputSchema struct { ... }` definition
4. Schema registration / generation (provider-specific)
5. The raw SDK call
6. Response extraction and JSON unmarshal
7. Error wrapping with duration logging
8. Success logging

For OpenAI this boilerplate is ~18 lines per method; for Anthropic ~20 lines. The
Gemini provider already benefits from `genkit.GenerateData[T]`'s generic typed
return, but still repeats ~8 lines of error-handling scaffolding.

Existing subtle drift across providers:
- Some methods use `logger.Error` (missing ctx), others `logger.ErrorContext` — inconsistent
- `GenerateNudgeContent` in Anthropic omits `warnIfTruncated`
- `GenerateExperienceSuggestions` in Anthropic uses `MaxTokens: 1024` vs the default 4096

## Context from Comments

A single comment from `maintainer` with just `@claude` — this is a direct
"go fix it" instruction with no additional design constraints. The issue body
already contains an explicit recommendation: introduce a generic
`callStructuredOutput[T any](ctx, adapter, promptText, logger) (T, error)`.

## Potential Solutions

### Option A: Per-provider generic helper functions (recommended)

Introduce package-level generic helpers, one per SDK:
- `openaiCallStructured[T any](ctx, logger, startTime, client, model, tempParam, msgs, schemaName) (T, error)`
- `anthropicCallStructured[T any](ctx, logger, startTime, client, model, temp, maxTokens, msgs, toolName) (T, *anthropic.Message, error)`
- `geminiCallStructured[T any](ctx, logger, startTime, g, model, temp, opts ...ai.Option) (T, error)`

Each helper handles schema generation, the raw API call, response extraction, JSON
unmarshal, and error-path logging. The caller still owns success logging and result
mapping to the public return type.

- **Pros:** Follows the existing Go generic pattern (`generateSchema[T]`,
  `genkit.GenerateData[T]`). Each helper is SDK-specific and idiomatic for that
  SDK. Can be placed in `server/ai/provider_helpers.go` and tested in isolation.
  Eliminates the 18-20 line boilerplate cleanly; each method collapses to ~12-18 lines.
- **Cons:** Three helper functions instead of one unified abstraction. The caller
  still writes a success log line and result-mapping block.
- **Effort:** Medium (refactor ~17 methods across 3 files; add unit tests for helpers).

### Option B: Single `callStructuredOutput[T any]` with a `rawCallFn` closure

Define `type rawCallFn[T any] func(ctx context.Context) (T, error)` and a unified
wrapper that handles timing and logging:

```go
func callStructuredOutput[T any](ctx, logger, op string, fn rawCallFn[T]) (T, error)
```

Each method passes a closure that does the SDK-specific work.

- **Pros:** True single abstraction matching the issue's recommendation.
- **Cons:** The closure captures local `outputSchema` types, which works in Go but
  requires more care. The Anthropic path also needs to return `*anthropic.Message`
  for `logAPICallCompleted` token counts, so this abstraction leaks.  The unified
  wrapper would not be testable at unit level without mocking the closure.
- **Effort:** Medium-large (extra indirection; Anthropic token-logging becomes awkward).

### Option C: Minimal local refactor — extract tool-use loop only

Extract just the Anthropic tool-use extraction loop (the most consistently duplicated
block) into a helper, and leave the API-call scaffolding in place.

- **Pros:** Zero risk to the API call path; smallest possible change.
- **Cons:** Doesn't address the issue's core concern. Still 3000 lines of near-duplicate
  code. The issue itself says this is High severity.
- **Effort:** Small but doesn't solve the problem.

## Recommendation

**Option A.** It aligns with the existing generic patterns in the codebase, keeps each
helper testable in isolation, and yields the largest code reduction without obscuring
the per-provider differences. The Anthropic helper returns `*anthropic.Message` so
callers can still use `logAPICallCompleted` for token-count logging.

The per-provider approach is better than a single generic wrapper (Option B) because:
1. Each SDK has genuinely different concerns (Anthropic returns token counts; Gemini is
   typed natively; OpenAI has strict JSON schema constraints).
2. A single `rawCallFn[T]` abstraction would force callers to close over things that
   differ between methods, obscuring rather than eliminating the duplication.

Also add `dupl` to `.golangci.yml` as the issue recommends to guard against future
re-duplication.

## Open Questions

- **Should the helpers log success?** — Recommendation: No. Success logging stays with
  the caller because different methods use different log messages and field sets
  (e.g., `nudge_count`, `title_length`). The helpers only log errors.
- **MaxTokens variation in Anthropic — constant vs parameter?** — Recommendation:
  Parameter (`maxTokens int64`). This keeps the helper general. Define named constants
  `anthropicDefaultMaxTokens = 4096` and `anthropicSmallMaxTokens = 1024`.
- **Should `ParseInformalTime` be refactored?** — Recommendation: No. It doesn't use
  structured output (no schema, raw text response) and has a different call shape in
  all three providers.
- **Should streaming delegates be touched?** — Recommendation: No. Methods that
  delegate to their `*Streaming` counterparts are already ~5 lines.
- **Should image methods be covered?** — Recommendation: Yes. The image methods still
  use the same helper — callers just construct the right messages list (with image
  parts) before calling the helper.
- **Should `dupl` be added to `.golangci.yml`?** — Recommendation: Yes, with a
  reasonable threshold (~80 tokens) as a regression guard.

## Implementation Plan

### Phase 1: Create helper infrastructure

**Goal:** Add `provider_helpers.go` with the three generic helper functions and unit
tests for each.

**Tasks:**
- [ ] Create `server/ai/provider_helpers.go` with:
  - `openaiCallStructured[T any](ctx, logger, startTime, client, model, tempParam, msgs, schemaName) (T, error)`
  - `anthropicCallStructured[T any](ctx, logger, startTime, client, model, temp, maxTokens, msgs, toolName) (T, *anthropic.Message, error)`
  - `geminiCallStructured[T any](ctx, logger, startTime, g, model, temp, opts ...ai.Option) (T, error)`
  - Named constants `anthropicDefaultMaxTokens = int64(4096)`, `anthropicSmallMaxTokens = int64(1024)`
- [ ] Add `server/ai/provider_helpers_test.go` with table-driven unit tests for each helper.
  Test both success and error paths:
  - OpenAI: mock HTTP server returning `{"choices":[{"message":{"content":"…"}}]}`; error on no choices;
    error on bad JSON; error on transport failure
  - Anthropic: mock HTTP server returning tool_use block; error on transport failure; truncation warning
  - Gemini: difficult to unit-test without live Genkit, so test error path only via a
    deliberately-misconfigured genkit instance; document why full success tests are integration-only
- [ ] Verify `npm run build` passes

**Architecture note:** `docs/server/architecture.md` §3 requires files < 1000 lines
(target < 500). The three provider files currently exceed this at 920–1390 lines. Phases
2–4 are expected to reduce each file by ~300–400 lines, bringing them to or below the
1000-line limit.

### Phase 2: Refactor OpenAI provider

**Goal:** Apply `openaiCallStructured` to all eligible methods in `provider_openai.go`.

**Tasks:**
- [ ] Refactor these methods: `GenerateGearFromWebpage`, `DetectGearInImage`,
  `GenerateCommunityContent`, `GenerateConversationSummary`,
  `GenerateExperienceCompletionSummary`, `GenerateExperienceFromImage`,
  `GenerateExperienceFromWebpage`, `GenerateRequestFromImage`,
  `GenerateStoryContent`, `GenerateNudgeContent`, `GenerateExperienceSuggestions`,
  `GenerateRequestSuggestions`, `InferSocialAttributes`
- [ ] Skip: `GenerateGearFromText`, `GenerateRequestContent`,
  `GenerateExperienceFromText` (streaming delegates), `ParseInformalTime` (no schema)
- [ ] Verify `npm run build` passes with no regressions

### Phase 3: Refactor Anthropic provider

**Goal:** Apply `anthropicCallStructured` to all eligible methods in `provider_anthropic.go`.
Also fix the `warnIfTruncated` drift on `GenerateNudgeContent`.

**Tasks:**
- [ ] Refactor the same method set as Phase 2 using `anthropicCallStructured`
- [ ] Fix `GenerateNudgeContent` to pass through `warnIfTruncated` (already in helper)
- [ ] Standardize all error calls to use `ErrorContext` (already in helper)
- [ ] Verify `npm run build` passes

### Phase 4: Refactor Gemini provider

**Goal:** Apply `geminiCallStructured` to eligible methods in `provider_gemini.go`.

**Tasks:**
- [ ] Refactor the same method set using `geminiCallStructured`
- [ ] Standardize error log calls to `ErrorContext`
- [ ] Verify `npm run build` passes

### Phase 5: Add dupl linter

**Goal:** Guard against future re-duplication.

**Tasks:**
- [ ] Add `dupl` to the `enable` list in `server/.golangci.yml`
- [ ] Set `dupl` threshold to `150` tokens in `linters-settings` to match the
  `~80 tokens` intent from the issue while avoiding false positives in legitimate
  repetitive patterns (e.g., the result-mapping blocks after each helper call)
- [ ] Verify `npm run build` still passes (fix any dupl false positives that appear)

## Related Issues

- #392 — "rationalize-ai-mock": related cleanup of the mock provider, may benefit
  from the same helper pattern applied here
