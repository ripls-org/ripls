# AI Integration

This directory provides AI/LLM capabilities for the Common Goods application, enabling features like automated gear detection from images, content generation, and conversation summarization.

## Architecture Overview

### Design Principles

1. **Provider Abstraction**: Support multiple AI providers (Gemini, OpenAI, Anthropic) through a unified interface
2. **Load Balancing**: Distribute requests across providers using weighted random selection
3. **Fault Tolerance**: Automatic fallback to backup providers on server errors
4. **Type Safety**: Use structured outputs for reliable JSON responses from LLMs

### Directory Structure

```
ai/
├── README.md                       # This file
├── provider.go                     # Core Provider interface and types
├── provider_gemini.go              # Google Gemini implementation
├── provider_openai.go              # OpenAI implementation
├── provider_anthropic.go           # Anthropic Claude implementation
├── provider_loadbalanced.go        # Load balancing decorator
├── provider_fallback.go            # Fallback decorator
├── provider_factory.go             # Factory functions for provider creation
├── prompts.go                      # Prompt builders for each Gen* method
├── sanitize.go                     # Output sanitization helpers
├── provider_test.go                # Unit tests
├── provider_integration_test.go    # Integration tests with real APIs (tag: integration)
├── provider_loadbalanced_test.go   # Load balancing tests
├── provider_fallback_test.go       # Fallback behavior tests
├── provider_factory_test.go        # Factory configuration tests
├── embedding/                      # Embedding subpackage (vector search)
└── eval/                           # Prompt eval harness (tag: benchmark)
    ├── eval.go                     # Shared helpers: Check, Tally, Report, Jaccard, etc.
    ├── experience.go               # Text-mode: ExperienceGoldens + EvaluateExperience
    ├── experience_image.go         # Image-mode: GenerateExperienceFromImage evaluator
    ├── experience_webpage.go       # Webpage-mode: GenerateExperienceFromWebpage evaluator
    ├── gear.go                     # Text-mode: GearGoldens + EvaluateGear
    ├── gear_image.go               # Image-mode: DetectGearInImage evaluator
    ├── gear_webpage.go             # Webpage-mode: GenerateGearFromWebpage evaluator
    ├── request.go                  # Text-mode: RequestGoldens + EvaluateRequest
    ├── eval_test.go                # Unit tests for shared helpers (default suite)
    ├── experience_test.go          # Unit tests for Experience evaluator (default suite)
    ├── experience_image_test.go    # Unit tests for Experience image evaluator (default suite)
    ├── experience_webpage_test.go  # Unit tests for Experience webpage evaluator (default suite)
    ├── gear_test.go                # Unit tests for Gear evaluator (default suite)
    ├── gear_image_test.go          # Unit tests for Gear image evaluator (default suite)
    ├── gear_webpage_test.go        # Unit tests for Gear webpage evaluator (default suite)
    ├── request_test.go             # Unit tests for Request evaluator (default suite)
    ├── prompt_test.go              # Live-API prompt eval runs (tag: benchmark)
    └── testdata/
        ├── experience_goldens.json         # 26 hand-crafted Experience text cases
        ├── experience_image_goldens.json   # 2 Experience image cases
        ├── experience_webpage_goldens.json # 2 Experience webpage cases
        ├── gear_goldens.json               # 13 Gear text cases
        ├── gear_image_goldens.json         # 4 Gear image cases
        ├── gear_webpage_goldens.json       # 3 Gear webpage cases
        └── request_goldens.json            # 12 Request cases
```

## Material category mapping

`material_category.go` is the single source of truth for the
`MaterialCategory` enum ↔ JSON-token mapping. The two exported helpers are:

- `MaterialCategoryFromJSON(s string) api.MaterialCategory` — converts the
  AI-returned lowercase token (e.g. `"solid_metal"`) to the proto enum value.
  Returns `MATERIAL_CATEGORY_UNSPECIFIED` for empty or unrecognized input.
- `MaterialCategoryJSON(c api.MaterialCategory) string` — inverse: enum →
  lowercase JSON token. Returns `""` for `UNSPECIFIED`.

Both maps are built once at `init()` by reflecting over
`api.MaterialCategory_name`, so adding a new proto enum value requires only
the `.proto` edit — no other Go changes.

**Do not add new string-to-enum or enum-to-string mappings elsewhere.**
All AI-boundary conversion goes through these two functions. The eval harness
validates golden `material_category_any_of` tokens against the helper at
test time so a typo in a golden file causes a test panic rather than a silent
miss.

## Provider Interface

The `Provider` interface defines the contract for all AI providers:

```go
type Provider interface {
    DetectGearInImage(ctx context.Context, req *DetectionImage) (*GearDetections, error)
    GenerateCommunityContent(ctx context.Context, prompt string, region string) (*CommunityGeneration, error)
    GenerateRequestContent(ctx context.Context, prompt string, region string) (*RequestGeneration, error)
    GenerateConversationSummary(ctx context.Context, messages []ConversationMessage, requestTitle string, requestDescription string) (*ConversationSummary, error)
    ClassifyUnifiedCreate(ctx context.Context, in UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error)
    // ...
}
```

`ClassifyUnifiedCreate` runs the unified-create classifier prompt
(`prompts_unified.go`). The unified-create service consumes it via a
narrow service-local adapter
(`server/services/unified_create.NewProviderClassifier`) so the
classifier inherits the same fallback + load-balancing as every other
method on `Provider`.

## Supported Providers

### Google Gemini

- Primary cloud provider with cost-effective pricing
- Native structured output support via Genkit
- Models: `googleai/gemini-2.5-flash`, `vertexai/gemini-2.5-flash`
- Best for: Production use, vision analysis, cost optimization

### OpenAI

- Industry-standard provider
- Structured output via JSON schema mode
- Default model: `gpt-5-mini`
- Best for: High-quality text generation, broad model selection

### Anthropic Claude

- Alternative cloud provider
- Structured output via forced tool use pattern
- Default model: `claude-sonnet-4-5`
- Best for: Complex reasoning, nuanced content generation

## Configuration

### Command-Line Flags

```bash
# Single provider (Gemini only - backward compatible)
./server --gemini-api-key=$GEMINI_API_KEY

# Single provider with Vertex AI
./server --vertex-ai-project=my-project --vertex-ai-location=us-central1

# Multiple providers with default load balancing
./server \
  --openai-api-key=$OPENAI_API_KEY \
  --anthropic-api-key=$ANTHROPIC_API_KEY \
  --gemini-api-key=$GEMINI_API_KEY

# With model overrides
./server \
  --openai-api-key=$OPENAI_API_KEY \
  --openai-model=gpt-4-turbo \
  --anthropic-api-key=$ANTHROPIC_API_KEY \
  --anthropic-model=claude-3-opus-20240229
```

### Default Configuration

When multiple providers are configured, the system uses:

- **Weighted providers**: OpenAI (50%) + Anthropic (50%)
- **Fallback**: Gemini
- **Default models**: gpt-5-mini, claude-sonnet-4-5, gemini-2.5-flash

### Factory Functions

For programmatic configuration, use the factory functions in `provider_factory.go`:

```go
// Recommended: Haiku primary with Gemini fallback
provider, err := ai.NewAllProvidersWithFallback(ctx, ai.AllProvidersConfig{
    AnthropicAPIKey: anthropicKey,
    VertexAIProject: gcpProject,
})

// Single provider
provider, err := ai.NewSingleProvider(ctx, ai.ProviderConfig{
    Type:   ai.ProviderTypeOpenAI,
    APIKey: apiKey,
    Model:  "gpt-4o",
})

// Custom weighted configuration
provider, err := ai.NewWeightedProviderWithFallback(ctx,
    []ai.WeightedProviderConfig{
        {Config: openaiConfig, Weight: 0.7},
        {Config: anthropicConfig, Weight: 0.3},
    },
    []ai.ProviderConfig{geminiConfig},
)
```

## Load Balancing

The `LoadBalancedProvider` distributes requests across providers using weighted random selection:

- Weights are normalized to sum to 1.0
- Each request independently selects a provider
- Per-method weights can be configured for different use cases

## Fallback Behavior

The `FallbackProvider` provides automatic retry on server errors:

**Triggers fallback (5xx errors):**
- 500 Internal Server Error
- 502 Bad Gateway
- 503 Service Unavailable
- 504 Gateway Timeout
- "overloaded" error messages

**Does NOT trigger fallback (4xx errors):**
- 400 Bad Request
- 401 Unauthorized
- 403 Forbidden
- 404 Not Found
- 429 Too Many Requests

## Testing

### Unit Tests

```bash
# Run all unit tests (no API keys required)
go test ./server/ai/... -v
```

### Integration Tests

Integration tests require API keys and call real AI APIs. They run in CI (the default CI Go suite passes `-tags=integration`):

```bash
# Set API keys
export GEMINI_API_KEY=your-key
export OPENAI_API_KEY=your-key
export ANTHROPIC_API_KEY=your-key

# Run integration tests
go test -tags=integration ./server/ai/... -v -run TestProviderIntegration
```

### Prompt Eval Harness (`eval/` subpackage)

The `eval/` subpackage runs hand-crafted goldens against a live AI provider
and gates on an aggregate per-field pass rate. It exists to catch prompt and
schema regressions — and to give future prompt rewrites a quantitative
pass/fail rather than anecdotal spot-checks. See [docs/issues/1157-genexperience-latency.md](../../docs/issues/1157-genexperience-latency.md) for the motivating context.

Covers every input mode for each of the three user-facing generation types:

| Type       | Mode     | Provider method                  | Goldens file                                    | Cases | Test name                     |
|------------|----------|----------------------------------|-------------------------------------------------|-------|-------------------------------|
| Experience | Text     | `GenerateExperienceFromText`     | `eval/testdata/experience_goldens.json`         | 26    | `TestExperiencePrompt`        |
| Experience | Image    | `GenerateExperienceFromImage`    | `eval/testdata/experience_image_goldens.json`   | 2     | `TestExperienceImagePrompt`   |
| Experience | Webpage  | `GenerateExperienceFromWebpage`  | `eval/testdata/experience_webpage_goldens.json` | 2     | `TestExperienceWebpagePrompt` |
| Gear       | Text     | `GenerateGearFromText`           | `eval/testdata/gear_goldens.json`               | 13    | `TestGearPrompt`              |
| Gear       | Image    | `DetectGearInImage`              | `eval/testdata/gear_image_goldens.json`         | 4     | `TestGearImagePrompt`         |
| Gear       | Webpage  | `GenerateGearFromWebpage`        | `eval/testdata/gear_webpage_goldens.json`       | 3     | `TestGearWebpagePrompt`       |
| Request    | Text     | `GenerateRequestContent`         | `eval/testdata/request_goldens.json`            | 12    | `TestRequestPrompt`           |

Gated behind the `benchmark` build tag so **CI and default `go test` skip them** — they only run on explicit benchmark invocations.

Each suite runs against every credentialed `(provider, model)` pair in a single invocation. Pairs are gated independently against `modelThresholds` (in `eval/threshold_test.go`); pairs without an entry fall back to the default `0.85` floor (`defaultPromptEvalPassRate`). The map is empty by default — populate it after the expanded golden set in #1777 lands and we have variance data per pair. Per-assertion leniency for inherently noisier modes (e.g., single-photo weight inference is looser than text-prompt date parsing) lives in the golden data itself via `weight_tolerance_pct` / `value_estimate_tolerance_pct`.

**How to run:**

Credentials and model lists are passed to the test binary via flags. Each provider supports either a value flag (`-anthropic-api-key=...`) or a file flag (`-anthropic-api-key-file=/path/to/file`); use the file form on shared hosts so the key never appears in `ps`/cmdline (#1885). Pairs whose credential is empty are silently skipped.

`-provider` is a filter (`all` default; or `anthropic` | `openai` | `gemini` to restrict); `-mode` is the input-mode filter (`all` | `text` | `image` | `webpage`). Each provider takes a comma-separated `-{provider}-models` list — a single invocation can rank multiple models from the same provider against the same goldens.

**Preferred setup:** materialize each key to a tmpfile once, then point the file flag at it:

```bash
# Materialize the keys you need (run once per shell session).
# gcp_project.sh resolves "dev" to your project id via .env.local.
DEV="$(scripts/gcp_project.sh dev)"
umask 077
mkdir -p /tmp/ripls-eval
gcloud secrets versions access latest --secret=anthropic-api-key --project="$DEV" > /tmp/ripls-eval/anthropic
gcloud secrets versions access latest --secret=openai-api-key    --project="$DEV" > /tmp/ripls-eval/openai
chmod 0400 /tmp/ripls-eval/*
```

```bash
# Default: all seven suites against every credentialed provider's default model
go test -tags=benchmark -v -timeout 30m ./server/ai/eval/ \
  -anthropic-api-key-file=/tmp/ripls-eval/anthropic

# Compare two Anthropic models on the image-mode gear suite
go test -tags=benchmark -v -timeout 10m -run TestGearImagePrompt ./server/ai/eval/ \
  -anthropic-api-key-file=/tmp/ripls-eval/anthropic \
  -anthropic-models=claude-haiku-4-5,claude-sonnet-4-5

# Text-only across multiple providers (fastest iteration loop)
go test -tags=benchmark -v -timeout 15m ./server/ai/eval/ \
  -mode=text \
  -anthropic-api-key-file=/tmp/ripls-eval/anthropic \
  -openai-api-key-file=/tmp/ripls-eval/openai

# Image-only across providers (6 cases: 4 gear_image + 2 experience_image)
go test -tags=benchmark -v -timeout 10m ./server/ai/eval/ \
  -mode=image \
  -anthropic-api-key-file=/tmp/ripls-eval/anthropic \
  -gemini-project="$DEV"

# Restrict to one provider via -provider (still respects -*-models for multi-model)
go test -tags=benchmark -v -timeout 45m ./server/ai/eval/ \
  -provider=openai \
  -openai-api-key-file=/tmp/ripls-eval/openai \
  -openai-models=gpt-5-mini,gpt-5.4-nano

# Gemini via Vertex AI (requires gcloud ADC; no API key)
go test -tags=benchmark -v -timeout 15m ./server/ai/eval/ \
  -provider=gemini \
  -gemini-project="$DEV" \
  -gemini-location=us-central1 \
  -gemini-models=vertexai/gemini-2.5-flash,vertexai/gemini-2.5-flash-lite

# One specific test against the default models
go test -tags=benchmark -v -timeout 5m -run TestExperiencePrompt ./server/ai/eval/ \
  -anthropic-api-key-file=/tmp/ripls-eval/anthropic

# A single case under a specific (provider, model) pair
go test -tags=benchmark -v -timeout 5m \
  -run 'TestExperiencePrompt/anthropic/claude-haiku-4-5/explicit_hike_tomorrow' \
  ./server/ai/eval/ \
  -anthropic-api-key-file=/tmp/ripls-eval/anthropic
```

The value flags (`-anthropic-api-key=...`, `-openai-api-key=...`) still work and are convenient for one-off CI runs where the host is dedicated and `ps` exposure is irrelevant. On a developer laptop or any shared host, prefer the `-file` form.

**Flag reference:**

| Flag                    | Default                          | Used by    |
|-------------------------|----------------------------------|------------|
| `-provider`             | `all`                            | dispatcher |
| `-mode`                 | `all`                            | dispatcher |
| `-anthropic-api-key`      | *(empty; skip)*                | anthropic  |
| `-anthropic-api-key-file` | *(empty; skip)*                | anthropic  |
| `-anthropic-models`       | `claude-haiku-4-5`             | anthropic  |
| `-openai-api-key`         | *(empty; skip)*                | openai     |
| `-openai-api-key-file`    | *(empty; skip)*                | openai     |
| `-openai-models`          | `gpt-5-mini`                   | openai     |
| `-gemini-project`       | *(empty; skip)*                  | gemini     |
| `-gemini-location`      | `global`                         | gemini     |
| `-gemini-models`        | `vertexai/gemini-2.5-flash`      | gemini     |

Each `-{provider}-models` flag accepts a comma-separated list. Gemini `preview` models are auto-routed to the `global` location regardless of `-gemini-location`, mirroring the production factory's override.

Sub-test names are namespaced as `Test{Suite}/{provider}/{model}/{case_id}`, so a failing case attributes directly to the pair it ran under.

**Iterating on a prompt:**

1. Edit `server/ai/prompts.go` (e.g. `buildExperienceFromTextPrompt`).
2. Re-run the relevant `Test*Prompt` suite with the command above.
3. Each `(provider, model)` pair logs its own per-field breakdown and aggregate pass rate, gated against its `modelThresholds` entry (or the default `0.85`). Sub-test failures attribute to `Test{Suite}/{provider}/{model}/{case_id}` so you can see which pair regressed.
4. Template sizes are budget-capped by `TestPromptSizes` (`prompt_size_test.go`, default suite — no API key). If your change trips a budget, either trim the prompt or, if the growth is justified, raise the budget **in the same commit** with a passing eval run to point at. Growth is a per-call TTFT and cost tax — see the header comment in `prompts.go`.
5. To keep a run for posterity, pass `-eval-report-out=docs/ai/eval/baselines/<date>_<shortsha>_<label>` and `-eval-report-label=<label>` — one directory per run, one JSON per suite. The #1265 rewrite runs under `docs/ai/eval/baselines/2026-07-08_*` are the convention to copy.

**Adding a golden:**

Drop a new entry into the right `testdata/*_goldens.json`. Every field in the `expected` block is opt-in — omitted fields aren't checked. See the existing cases for shape.

### Unit Tests for the Eval Helpers

The eval-helper unit tests (Jaccard, containsCheck, setEqual, value/weight tolerance, per-type evaluator branches) run in the default test suite — no API key, no build tag:

```bash
go test ./server/ai/eval/
```

### Test Images

Test images are located in `server/test_data/`:

- `rocky_talkie.jpeg` - Handheld radio for testing gear detection

## Error Handling

- Provider initialization errors are logged but don't prevent server startup
- If no providers are configured, AI features are disabled
- Provider API errors return appropriate gRPC error codes
- Fallback providers are tried automatically on server errors

## Integration with Services

AI providers are injected into service implementations during initialization:

```go
// In server/main.go
aiProvider, err := initializeAIProvider(ctx, cfg)
if aiProvider != nil {
    gearService.SetAIProvider(aiProvider)
    communityService.SetAIProvider(aiProvider)
    chatService.SetAIProvider(aiProvider)
}
```

Services use the provider to implement AI-powered features like gear detection, content generation, and conversation summarization.
