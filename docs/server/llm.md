---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: How the server uses LLMs — multi-provider ai.Provider interface, structured output, load balancing, graceful fallback; powers gear/community/request/experience creation assist and structured inference. No LLM writes user-visible prose (#2936).
  globs: [server/ai/**, server/services/experience/**]
  triggers: [llm, gemini, openai, anthropic, provider, classifier, generation, structured-output, vertex]
  lens: [architecture, server]
  skills: [issue, audit]
  domain: server
freshness:
  verified_commit: "82c3518d0"
  verified_on: "2026-08-17"
---
# LLM Architecture

## Overview

The Ripls server uses Large Language Models (LLMs) for two things: **creation
assist** (drafting a title, description, and structured metadata from a photo,
a link, or a sentence, which the user then reviews and edits before anything
is saved) and **structured inference** (classification, extraction, time
parsing). The implementation provides a flexible multi-provider architecture
with structured output generation, automatic load balancing, and graceful
fallback. All LLM operations use the `ai.Provider` interface, allowing
transparent switching between OpenAI, Anthropic, and Google Gemini providers.

**No LLM writes prose the product speaks in its own voice.** #2936 removed
that category: story cards, nudge cards, Workshop hero copy, profile impact
narratives, and conversation/recap summaries all rendered model-written
sentences to users who never asked for them, and they read as generated. Those
surfaces now render from template catalogs. The distinction that matters is
whether a user solicited the text and can edit it before it ships — creation
assist passes that test, and unbidden narration never did.

## Design Principles

**Provider Abstraction**: All LLM functionality is accessed through the `ai.Provider` interface. Services depend on this abstraction, not concrete provider implementations. This enables provider composition, testing with mocks, and runtime provider configuration without code changes.

**Structured Output**: All LLM operations return structured data via JSON schemas, not free-form text. This ensures type safety, consistent parsing, and reliable integration with application logic. Providers use native structured output features (OpenAI Structured Outputs, Anthropic JSON mode, Gemini schema enforcement).

**Provider Flexibility**: The architecture supports multiple LLM providers through a common abstraction interface. This enables switching providers based on cost, capabilities, or availability without code changes. While the system supports weighted load balancing and automatic fallback for specialized use cases, production deployments typically use a single reliable provider with standard retry logic for transient failures.

## Provider Interface

The `ai.Provider` interface (defined in [server/ai/provider.go](../../server/ai/provider.go)) defines all LLM operations:

- **Gear detection**: `DetectGearInImage` - Analyzes images to identify shareable items
- **Content generation from text**: `GenerateCommunityContent`, `GenerateRequestContent`, `GenerateExperienceFromText`
- **Content generation from images**: `GenerateRequestFromImage`, `GenerateExperienceFromImage`
- **Structured inference**: `ParseInformalTime`, `InferSocialAttributes`, `ClassifyUnifiedCreate`

### Provider Implementations

Three concrete providers implement this interface:

- **GeminiProvider** ([server/ai/provider_gemini.go](../../server/ai/provider_gemini.go)): Uses Google's Gemini models via Vertex AI and the Firebase Genkit SDK. Default model when constructed directly: `vertexai/gemini-2.5-flash-lite`. The production factory selects `vertexai/gemini-3.1-flash-lite`.
- **OpenAIProvider** ([server/ai/provider_openai.go](../../server/ai/provider_openai.go)): Uses OpenAI models with native Structured Outputs. Default model: `gpt-5-mini`. SDK configured with 5 retries and exponential backoff for 429/rate-limit errors.
- **AnthropicProvider** ([server/ai/provider_anthropic.go](../../server/ai/provider_anthropic.go)): Uses Anthropic Claude models with JSON mode. Default model: `claude-haiku-4-5`. SDK configured with 5 retries and exponential backoff for 429/rate-limit errors.

### Composite Providers

Two wrapper implementations enable advanced provider composition for specialized use cases:

- **LoadBalancedProvider** ([server/ai/provider_loadbalanced.go](../../server/ai/provider_loadbalanced.go)): Distributes requests across multiple providers using weighted random selection. Useful for A/B testing different models or distributing load during high-volume events. Health check returns a single composite status: healthy when at least one weighted provider is healthy.
- **FallbackProvider** ([server/ai/provider_fallback.go](../../server/ai/provider_fallback.go)): Wraps a primary provider with fallback providers. On any error, automatically retries with fallback providers in order. This is the default production configuration: Gemini 3.1 Flash-Lite primary with Haiku fallback. Health check returns a single composite status using an at-least-one-healthy rule — a failing secondary or tertiary does not page when the primary is up.

## Prompt Engineering

Prompts are centralized under `server/ai/prompts*.go` and shared across provider implementations: [server/ai/prompts.go](../../server/ai/prompts.go) holds the Gen* content-generation templates, with a sibling for the unified-create classifier (`prompts_unified.go`). The story, nudge, hero-card, and impact-row prompts were deleted with the surfaces they fed (#2936). The summary prompts (`prompts_summaries.go`) and the `GenerateConversationSummary`/`GenerateExperienceCompletionSummary` provider methods they back are unchanged in code but orphaned — no RPC calls them since #2936 stubbed `chat.GenConversationSummary` and `experience.GenerateCompletionSummary` to `Unimplemented`; TODO(#2938) deletes both once that release has soaked. This approach assumes providers are reasonably interchangeable with the same prompts, though output quality and style may vary between models. Provider-specific prompt tuning has not been implemented but may be beneficial if quality differences emerge in production.

### Tone and Style

A shared `toneAndStyle` constant defines the voice for all content generation: casual and conversational like a text message, avoiding marketing language and excessive formality.

### Prompt Structure

The Gen* prompts follow a compact, token-budgeted structure (#1265):

1. **Task framing**: one sentence on who the model is helping and what to extract
2. **Context**: user input, region, current time (with weekday appended for date grounding)
3. **FIELDS**: one numbered entry per output field with its rules inline — at most two worked examples per prompt, chosen to never overlap eval goldens
4. No trailing JSON-shape recitation: output shape is enforced by the canonical wire schemas in [server/ai/schemas.go](../../server/ai/schemas.go), not by prose

Every added line costs TTFT and money on each call, so template sizes are capped by the `TestPromptSizes` budget ratchet in [server/ai/prompt_size_test.go](../../server/ai/prompt_size_test.go); raising a budget requires re-running the benchmark-tagged eval suite for that prompt (see `server/ai/README.md`).

## Provider Configuration

### Production Configuration

The recommended production setup uses `NewAllProvidersWithFallback()` from [server/ai/provider_factory.go](../../server/ai/provider_factory.go):

**Configuration**: Vertex AI project (primary), Anthropic API key (fallback), optional OpenAI API key (tertiary)

**Result**:
1. **Primary**: Gemini 3.1 Flash-Lite via Vertex AI — best accuracy-to-cost ratio on the small-model tier
2. **Fallback**: Anthropic Claude Haiku 4.5 — on any error from primary
3. **Tertiary fallback**: OpenAI GPT-5-mini (if configured)

The default was selected from the 2026-05-09 gear-image eval (#1777): Gemini 3.1 Flash-Lite scored 93.6% to Haiku's 84.9% on the small-model tier with comparable median latency and lower per-call cost, so the production config uses Flash-Lite as primary with Haiku fallback for resilience.

If only one provider is configured, it's used directly without fallback.

### Model Selection

Default models are optimized for cost, latency, and quality:

- **OpenAI**: `gpt-5-mini` - Fast, cost-effective structured outputs
- **Anthropic**: `claude-haiku-4-5` - Best-in-class speed for this task complexity
- **Gemini**: `vertexai/gemini-3.1-flash-lite` - Vertex AI primary, strong image analysis

Models can be overridden via configuration flags (each defaults to empty, so
the factory's per-provider default applies):

- `--gemini-model`: Override Gemini model (factory default: `vertexai/gemini-3.1-flash-lite`)
- `--openai-model`: Override OpenAI model (factory default: `gpt-5-mini`)
- `--anthropic-model`: Override Anthropic model (factory default: `claude-haiku-4-5`)

Example:
```bash
# Use Gemini 3.0 Flash preview for testing
go run ./server --gemini-model=vertexai/gemini-3-flash-preview
```

See [server/ai/provider_factory.go](../../server/ai/provider_factory.go) for configuration details.

## Service Integration

Services receive an optional `ai.Provider` via dependency injection. If no provider is configured, AI-powered RPCs return `CodeFailedPrecondition` errors.

### Provider Injection

The startup sequence initializes the provider in [server/main.go](../../server/main.go) and injects it into services via `SetAIProvider()` or constructor parameters in [server/wiring.go](../../server/wiring.go). 

### Integration Pattern

Each service follows the same pattern:

1. Authenticate user with `auth.RequireAuth(ctx)`
2. Check `if s.aiProvider == nil` and return `CodeFailedPrecondition` if not configured
3. Fetch media (if image-based) and verify ownership
4. Generate presigned URL for images (15 minute expiration)
5. Call `s.aiProvider.Method()` with structured input
6. Convert AI response to API format and return

See [server/services/gear/gen_ai.go:51](../../server/services/gear/gen_ai.go#L51) for `GenGear` implementation and [server/services/community/gen.go:26](../../server/services/community/gen.go#L26) for `GenCommunity` implementation.

## Generate-Then-Preview Pattern

All content generation follows a two-phase pattern:

1. **Generation Phase**: AI creates content from minimal input (text prompt or image). Results are returned to client but NOT saved to database. The `Gen*` RPCs are pure generators with no database writes.

2. **Preview Phase**: Client displays AI-generated content for user review and editing. User can modify title, description, location, time, or reject entirely.

3. **Persistence Phase**: User confirms → client calls `Create*` or `Save*` RPC to persist final content. This ensures human-in-the-loop validation before database writes.

This pattern applies to:
- Gear: `GenGear` → user previews → `SaveGear` + `ShareGear`
- Request: `GenRequest` / `GenRequestFromMedia` → user previews → `SubmitRequest`
- Experience: `GenExperience` / `GenExperienceFromMedia` → user previews → `CreateExperience`

Community creation is the exception: it does **not** generate any user-visible
text. The name and description are user-entered, and `GenCommunity` /
`StreamGenCommunity` only extract image keywords to fetch a background image
(`GenerateCommunityContent` returns just `SearchKeywords`) → `CreateCommunity`.

## Image Handling

## Logging

All LLM operations use structured logging (see [docs/server/observability.md](observability.md)) with consistent fields:

**Standard fields**:
- `provider`: Provider name (gemini, openai, anthropic)
- `operation`: Method name (DetectGearInImage, GenerateCommunityContent, etc.)
- `model`: Specific model used
- `duration_ms`: Request latency
- `error`: Error message on failure

Each provider implementation logs at method entry, on error, and on completion with timing information.

## Testing

### Mock Provider

Tests use `MockProvider` from [server/ai/provider_mock.go](../../server/ai/provider_mock.go) for deterministic testing without external API calls. The mock returns configurable responses or errors.

Example: [server/services/gear/gen_ai_test.go](../../server/services/gear/gen_ai_test.go) demonstrates mocking gear detection responses.

`NewE2EDeterministicProvider()` ([server/ai/provider_e2e.go](../../server/ai/provider_e2e.go)) wraps `MockProvider` with a keyword-based unified-create classifier and non-empty, immediately-saveable generated content, so end-to-end tests can drive the real create UI (prompt → classify → stream → preview → Save) without a live LLM. It is wired into the server via the `--mock-ai-provider` flag (e2e/dev only — never production).

### Integration Tests

Provider integration tests in [server/ai/provider_integration_test.go](../../server/ai/provider_integration_test.go) verify real LLM behavior when API keys are configured. These are skipped in CI but useful for prompt engineering and quality validation.

## Structured Output Details

Each provider uses native structured output features to ensure type-safe JSON responses:

### Gemini (Genkit SDK)

[server/ai/provider_gemini.go](../../server/ai/provider_gemini.go) uses Genkit's `GenerateData[T]` with Go struct schemas. The Firebase Genkit SDK handles schema enforcement and JSON parsing.

### OpenAI

[server/ai/provider_openai.go](../../server/ai/provider_openai.go) uses native Structured Outputs with JSON schemas passed via `ResponseFormat.JSONSchema` with `Strict: true`. This guarantees schema-conformant responses.

### Anthropic

[server/ai/provider_anthropic.go](../../server/ai/provider_anthropic.go) uses the forced tool-use pattern: each call defines a single tool whose input schema is generated from the canonical wire struct (`generateToolSchema[T]`), and the model is required to call it — so the response is constrained to the schema rather than relying on prompt text.

## Schema Definition Guidelines

### OpenAI Strict Mode Requirements

OpenAI's Structured Outputs with `Strict: true` requires all properties in a schema to be in the `required` array. To avoid schema validation errors:

**Rules**:
- Use value types, not pointers (e.g., `valueEstimateOutput` not `*valueEstimateOutput`)
- Don't use `omitempty` JSON tags
- All nested object properties must also be required

**Common Error**: `Invalid schema for response_format: Missing 'value_estimate' in the required array`

**Cause**: Using `*valueEstimateOutput` with `json:"value_estimate,omitempty"` instead of value type `valueEstimateOutput`

**Examples**: See the per-call `type outputSchema struct` definitions in [server/ai/provider_openai.go](../../server/ai/provider_openai.go) and the shared `valueEstimateOutput` schema in [server/ai/schemas.go](../../server/ai/schemas.go)

**Note**: Gemini and Anthropic are more lenient, but use the same patterns for consistency across providers.

## Startup Sequence

1. Parse configuration flags (API keys, project IDs, model overrides) in [server/config](../../server/config/config.go)
2. Initialize provider via `initializeAIProvider()`:
   - With `--mock-ai-provider` (e2e/dev only), returns `ai.NewE2EDeterministicProvider()` and skips live providers
   - Otherwise calls `ai.NewAllProvidersWithFallback()`
   - Returns configured provider or nil if no keys set
3. Inject provider into services via `SetAIProvider()` or constructor
4. Services check `if s.aiProvider == nil` before calling LLM methods
5. Graceful degradation: missing provider returns `CodeFailedPrecondition`

## Key Files

- [server/ai/provider.go](../../server/ai/provider.go): Provider interface and type definitions
- [server/ai/prompts.go](../../server/ai/prompts.go): Gen* prompt templates (the unified-create classifier prompt lives in `prompts_unified.go`)
- [server/ai/provider_gemini.go](../../server/ai/provider_gemini.go): Gemini/Vertex AI provider implementation
- [server/ai/provider_openai.go](../../server/ai/provider_openai.go): OpenAI provider implementation
- [server/ai/provider_anthropic.go](../../server/ai/provider_anthropic.go): Anthropic Claude provider implementation
- [server/ai/provider_loadbalanced.go](../../server/ai/provider_loadbalanced.go): Weighted load balancing wrapper
- [server/ai/provider_fallback.go](../../server/ai/provider_fallback.go): Automatic fallback wrapper
- [server/ai/provider_factory.go](../../server/ai/provider_factory.go): Provider construction and composition
- [server/ai/provider_mock.go](../../server/ai/provider_mock.go): Mock provider for testing
- [server/services/gear/service.go](../../server/services/gear/service.go): Gear detection RPC implementation
- [server/services/community/gen.go](../../server/services/community/gen.go): Community generation RPC
- [server/services/request/gen.go](../../server/services/request/gen.go): Request generation RPC
- [server/services/experience/gen_ai.go](../../server/services/experience/gen_ai.go): Experience generation RPC
- [server/services/unified_create/classifier.go](../../server/services/unified_create/classifier.go): Unified-create type classification
- [server/main.go](../../server/main.go): Provider initialization ([server/wiring.go](../../server/wiring.go): service wiring)

## Related Documentation

- [Client Creation Flows](../client/create.md): Client-side UX for AI-powered creation
- [Semantic Search](semantic_search.md): Local embeddings (separate from LLM generation)
- [Media Architecture](../client/media.md): Presigned URLs and media storage patterns
