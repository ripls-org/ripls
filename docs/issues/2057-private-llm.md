# Issue #2057: Evaluate self-hosted LLM inference for ripls workloads (NVIDIA 5060 Ti on mimir)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2057
**Filed:** 2026-05-19 | **Last activity:** 2026-05-19
**Labels:** *(none)* | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Every `Gen*` and classifier call in `server/ai/` goes to a paid cloud
provider today (Gemini via Vertex, Anthropic, OpenAI). Embeddings already
run locally on the server box via ONNX (`server/ai/embedding/`); generative
LLM traffic does not. The hosted hardware is now sufficient — an NVIDIA
5060 Ti / 16 GB on `mimir` (Unraid, Ollama already installed) — to make a
private self-hosted generative endpoint a real option for one or more of:
cost (the `Gen*` paths and the unified-create classifier dominate spend),
privacy (image / webpage / free text never leaves our infra), vendor
diversity (a fourth row in the load-balancer/fallback chain), and latency
floor (interesting for the classifier and other small calls).

Naming note up front: we call this the **private LLM** path throughout
code and docs — not "local" (it's not on the Go server's machine) and
not "OpenAI-compatible" at the public abstraction layer (the wire
format is an implementation detail, not the user-facing concept). The
public surface — `ProviderTypePrivate`, `NewPrivateLLMProvider`,
`--private-llm-*` flags (the `-llm-` infix is deliberate — bare
`--private-*` is ambiguous in a flag-help context, unlike the
existing `--openai-*` / `--anthropic-*` / `--gemini-*` patterns
where each prefix is a unique vendor name),
`Name() == "private"` — should never carry the word "OpenAI." That
word stays inside whatever struct the wire plumbing reuses.

Observability follow-on of that choice: `Name() == "private"` flows
into the `provider` log field on every Gen* call
(`docs/server/observability.md` §Standard Field Names; classifier-stage
example in §Unified-create stream). That feeds the `rpc_errors`
log-based metric (which groups by `operation` and labels per provider)
and, once `CheckHealth` is registered in `main.go`, the
`health_check_unhealthy` alert's per-backend state-change notification.
That's exactly the attribution we want for the eval: a failure during
the eval shows up tagged `provider=private` in the same dashboards a
production failure would, with no special-casing.

The ask is **not** to ship private-first inference yet. It's to stand
up the endpoint, plumb it through `server/ai/` opt-in so the existing
eval harness can target it, run all 7 suites plus the classifier
against the private endpoint and the current cloud providers, write a
report, and *then* decide whether any production routing is justified
— as a follow-up issue, not this one. We do all the work on the
existing `private_llm` branch and **do not open a PR until the eval
data in phase 4 justifies merging the plumbing**. If the data is
negative, the only artifact that lands on main is the eval report
itself (which is still valuable as a record of what we tried).

Our workload is unusually well-suited to this exploration: ~25 fixed
`buildXxxPrompt` functions in `server/ai/prompts*.go`, all `fmt.Sprintf`
over long static preambles + short variable tails, all stateless one-shot
calls with fixed JSON-schema outputs. That shape means prefix caching
is a real win on llama.cpp-class servers, KV cache per request is
small (the webpage mode is the long pole at ~5K tokens), and the eval
surface is fully enumerable — there's no hidden long tail to fear.

There are no comments on the issue yet, so this plan is grounded
entirely in the description and the current codebase.

## Context from Comments

No comments on the issue. The issue body is unusually detailed for a
P1 infra exploration — it already names the hardware, the workload
characteristics, the model candidates, the approach, the risks, the
deliverables, and the explicit out-of-scope list. Most of the planning
work below is grounding those choices against the actual code and
naming the few real decisions the issue leaves open.

Flag: with no comments yet, there's no second voice on the model
candidates. I've taken positions in **Open Questions** below; happy to
revise before any code lands.

## Code grounding (what already exists)

Stuff this plan can lean on rather than re-invent:

- **`server/ai/provider_factory.go`** — `ProviderConfig{Type, APIKey,
  Model, Temperature, VertexAIProject, VertexAILocation}` and a
  `createProvider(ctx, cfg)` switch with three cases (`ProviderTypeGemini`,
  `ProviderTypeOpenAI`, `ProviderTypeAnthropic`). Adding a fourth case is
  mechanical.
- **`server/ai/provider_openai.go:84-101`** — `NewOpenAIProvider(apiKey,
  model, temperature)` builds the OpenAI Go SDK client with
  `option.WithAPIKey(apiKey), option.WithMaxRetries(5)`. The SDK's
  `option.WithBaseURL(...)` is already used in
  `server/ai/provider_helpers_test.go:44-87` against `httptest` fixtures,
  so the SDK path against a non-OpenAI host is known-good.
- **`server/ai/provider_helpers.go:54+`** — `openaiCallStructured[T]`
  builds the strict `response_format: json_schema` request that all
  text/webpage Gen* methods (and the classifier) flow through. This is
  the exact surface the eval needs to exercise against Ollama's
  OpenAI-compatible endpoint.
- **`server/main.go:543-565`** — `initializeAIProvider(ctx, cfg, logger)`
  composes `NewAllProvidersWithFallback` from `cfg.openAIAPIKey`,
  `cfg.openAIModel`, etc. The `serverConfig` struct at lines 173-223 is
  the central flag-bag; adding `privateLLMBaseURL` / `privateLLMModel` /
  etc. follows the established pattern.
- **`server/ai/eval/provider_setup_test.go`** — under the `benchmark`
  build tag. `providerSpecsFromFlags()` builds a list of `{Name, Models,
  HasCreds}` triples; `newProviderInstance(t, name, model)` switches on
  name to build the actual provider. Adding a fourth arm and a
  `-private-base-url`/`-private-models` flag pair is the natural
  extension. Threshold lookups happen against `"<provider>/<model>"` —
  so a private entry like `private/qwen2.5-vl-7b` slots in without
  changing `threshold_test.go`'s structure.
- **`infra/private_llm/`** — already exists as an empty directory on
  this branch. Confirms the drop slot; the README + Modelfile + run
  scripts are net-new.

The handful of provider methods that are *not* implemented on
`OpenAIProvider` today — `GenerateHeroCardCopy`,
`GenerateImpactRowNarratives` — return sentinel errors
(`ErrHeroCardCopyNotImplemented`, `ErrImpactRowNarrativesNotImplemented`).
If the private path reuses that struct underneath, it inherits the same
"not implemented, fall back to template copy" semantics for free.

## Potential Solutions

### Option A: Reuse the OpenAI wire plumbing under a `Private` public surface (recommended)

- **Approach:** At the public layer, add `ProviderTypePrivate
  ProviderType = "private"`, `NewPrivateLLMProvider(baseURL, model,
  temperature) (Provider, error)`, and a `case ProviderTypePrivate:`
  arm in `createProvider`. Wire `--private-llm-enabled`,
  `--private-llm-base-url`, `--private-llm-model`, `--private-llm-temperature`
  into `serverConfig` and surface them through `AllProvidersConfig` as
  `PrivateLLMBaseURL`, `PrivateLLMModel`, `PrivateLLMTemperature`. `Name()`
  returns `"private"`. The private provider is **not** in the default
  fallback chain in this issue — it's only constructed when
  `--private-llm-enabled` is on, and `NewAllProvidersWithFallback`'s
  default composition stays untouched.

  Under the hood, since Ollama exposes an OpenAI-compatible HTTP
  surface, the implementation reuses `*OpenAIProvider`'s methods via a
  thin wrapper struct (or via a small refactor where `OpenAIProvider`'s
  client field accepts a base URL and the name field is configurable —
  either factoring is fine, code review picks). The constructor passes
  a sentinel `option.WithAPIKey("private")` to the SDK so Ollama is
  happy and the code path is identical to the cloud OpenAI path — no
  empty-key branching anywhere.
- **Pros:** Public surface is honest about what it is — a private LLM
  provider, not "OpenAI." Internal reuse means we inherit structured-
  output strict mode, retry behavior, the streaming JSON parser in
  `streaming_json.go`, health-check shape, and the existing 600+ lines
  of test coverage. Routing it through the eval harness is a single
  factory call. Lowest blast radius if the eval comes back negative —
  rip out the fourth arm + the flags and close the branch.
- **Cons:** Coupling to Ollama's OpenAI-compat layer is more lossy
  than llama-server's native API for things like grammar-constrained
  decoding. If structured-output fidelity fails the eval (the main
  risk the issue names), we swap to llama-server next — but that's
  still small Option B work, not thrown-away work, because the public
  `Private` plumbing stays.
- **Effort:** small. A day for the plumbing, a half day for the eval
  harness wiring, then the actual time goes into model selection and
  the eval runs themselves.

### Option B: Generic `OpenAIProvider` with `BaseURL` field, no new type

- **Approach:** Don't add `ProviderTypePrivate`. Instead, add `BaseURL`
  to the existing OpenAI config so any base URL (real OpenAI, Ollama,
  LM Studio, llama-server) is just data. Same `OpenAIProvider` struct,
  same `Name()` returning `"openai"`.
- **Pros:** Even less code than Option A.
- **Cons:** Loses attribution. `LoadBalancedProvider` logs,
  `FallbackProvider` error paths, eval threshold map (keyed by
  `"openai/<model>"`), `CheckHealth` outputs, and structured log lines
  all conflate Ollama traffic with real-OpenAI traffic. That kills the
  signal we're trying to gather: "did the private box take this call,
  and did it succeed?" Also fragile if anyone ever wires up both a
  private endpoint and a real OpenAI endpoint at the same time.
  Directly contradicts the naming feedback ("not say 'openai'
  unnecessarily where it's actually the private provider").
- **Effort:** smaller than A by an hour or two; cost we pay back the
  first time we try to read a metric.

### Option C: Net-new `PrivateLLMProvider` struct, fork from `OpenAIProvider`

- **Approach:** Copy `provider_openai.go` to `provider_private_llm.go`,
  edit the constructor and `Name()`, and diverge from there as the
  private endpoint's quirks accumulate.
- **Pros:** Most flexibility long-term. If Ollama's structured-output
  layer is flaky and we end up writing GBNF grammars or post-processing
  JSON, the divergence has a clean home.
- **Cons:** ~600 lines of cloned code on day one to support a decision
  we haven't made yet. Drift is inevitable; the OpenAI provider evolves
  and the private provider lags. Premature abstraction in the exact
  way CLAUDE.md cautions against.
- **Effort:** medium upfront, large ongoing maintenance burden.

## Recommendation

**Option A.** Public layer is honestly named "private"; internal layer
reuses the OpenAI wire plumbing because Ollama speaks that wire format
and there's no value in pretending otherwise. The opt-in flag gating
keeps the production blast radius literally zero until we choose to
wire `private` into `NewAllProvidersWithFallback`'s defaults in a
separate, evidence-backed PR — which won't happen in this issue.

Factoring detail to settle in code review: whether the internal reuse
is (a) a thin wrapper struct that embeds `*OpenAIProvider` and
overrides `Name()`, or (b) a small refactor where `OpenAIProvider`
grows a configurable name field and an optional base URL, with both
`NewOpenAIProvider` and `NewPrivateLLMProvider` constructing it. Both
land in the same place behaviorally; (b) is slightly less code and
keeps everything in one file, (a) keeps `OpenAIProvider` unchanged.
Defer the final pick until the patch — what matters in this plan is
that the *public* names — `Provider`, `ProviderType`, flag names,
`Name()` return value — are all `Private`, never `OpenAI`.

If the structured-output eval comes back red against Ollama, switching
to llama-server keeps the same shape: same provider struct, same flag
names, same eval threshold rows — only the runtime on `mimir` changes.

## Open Questions

- **One vision-capable model for all 25 prompts, or two models (text-only
  for text/webpage suites, vision-capable for image suites)?** —
  Recommendation: **one vision-capable model serves everything**. That
  matches commercial-provider behavior — we don't branch on modality
  when calling Gemini/Claude/GPT today, the same provider handles every
  `buildXxxPrompt`. Vision-capable models like Qwen2.5-VL-7B are also
  fully competent text models, so a single model loaded on `mimir`
  serves all 7 suites + the classifier without swaps. This also
  preserves the prefix-cache win the issue calls out (swapping models
  drops the cache).

  Conditional escalation: if the vision model's text suites pass-rate
  lags a similarly-sized text-only model by more than the eval's
  noise floor (a margin we'll size from the existing per-pair
  variance in `eval/threshold_test.go`'s `defaultPromptEvalPassRate`
  context), run a *second* eval with a text-only model (e.g.
  Qwen2.5-14B-Instruct Q4) over text+webpage suites only and document
  the gap. Do not run both models live during a single eval pass —
  one model loaded throughout — and report which-model-served-which
  in the report so the comparison is honest. If the gap is real and
  matters, per-method model routing inside `PrivateLLMProvider`
  becomes a Phase-2+ lever (flagged in the issue, not built now).

- **Should the private provider hit Ollama's `/v1` OpenAI-compat path
  directly, or front it with llama-server from day one?** — Recommendation:
  Ollama `/v1`. The issue defers vLLM/llama-server unless the eval
  surfaces a structured-output problem Ollama can't solve, and that's
  the right call — one variable changed at a time. Setting
  `option.WithBaseURL("http://mimir.local:11434/v1")` (or whichever
  hostname resolves on the LAN) and shipping is faster, and the eval
  is what decides whether we revisit. Document the llama-server
  fallback path in `infra/private_llm/README.md` so the swap is one
  Modelfile + flag away if needed.

- **Reachability.** The dev machine is on the LAN with `mimir`, so a
  direct LAN address (`mimir.local`, the static IP, or whatever DNS
  the LAN already serves) is sufficient for the eval. **No** Tailscale
  / SSH-tunnel decision in this issue. If we later route any
  production traffic to the private endpoint, *that* PR picks the
  reachability story for the production Cloud Run path — it's a
  different problem (Cloud Run can't see the LAN, so a Tailscale
  subnet router or a public-but-mTLS-fronted endpoint becomes
  relevant) and shouldn't be coupled to this exploration.

- **Streaming.** Every text-mode provider method in `OpenAIProvider`
  funnels through a `*Streaming` counterpart and drains it for the
  unary response shape. Ollama's `/v1/chat/completions` supports
  `stream: true`, and the SDK's stream reader is the same — but does
  Ollama emit the same SSE event shape (`tool_calls.function.arguments`
  chunks) that `streaming_json.go` is parsing? — Recommendation:
  assume yes (Ollama emits OpenAI-compatible SSE), and surface this
  as the first thing the eval validates with a single golden case
  before running all suites. If it doesn't work, fall back to
  non-streaming requests for the eval and flag streaming as a
  Phase-2-plus item before any production routing. Concretely, the
  fallback is a `privateStreaming bool` field on the provider config,
  off-by-default for `private`, that turns the unary methods into
  single `chat/completions` POSTs instead of stream-drain.

- **What pass-rate bar does `private` need to clear to be considered
  viable?** — Recommendation: hold to the existing `0.85` floor from
  `eval/threshold_test.go`'s `defaultPromptEvalPassRate` for the
  decision memo, but **don't** add a `private` row to `modelThresholds`
  in this issue. The threshold map is for pairs we ship; this is an
  exploration. If a follow-up issue routes any real traffic to the
  private endpoint, the threshold entry lands in that PR alongside
  the routing change.

## Implementation Plan

All work happens on the existing `private_llm` branch (which is the
current branch). **No PR is opened until phase 4's eval data justifies
it.** If the data is positive, phases 1-3's code is reviewed and
merged together with the report; if it's negative, the only artifact
that lands on main is the eval report (which is still valuable on its
own — it documents what we tried, what failed, and what we'd want to
try next if the constraints change).

### Phase 1: Stand up Ollama on `mimir`, document reproducibly

**Goal:** A single source of truth — `infra/private_llm/` — that
contains everything needed to bring up the endpoint on a clean Unraid
box. No code changes to the server in this phase.

**Tasks:**
- [ ] `infra/private_llm/README.md` covering: hardware, Ollama install
      (or "already installed", with version pin), how to reach the
      box (LAN hostname), how to verify the `/v1/chat/completions`
      endpoint with `curl`, and how to swap to llama-server if the
      eval forces it.
- [ ] `infra/private_llm/Modelfile.vision`: pinned vision-capable
      model + tag (initial candidate: `qwen2.5vl:7b`), `PARAMETER
      num_ctx 6144`, `PARAMETER num_predict 1024` (the long pole on
      output tokens; verify against
      `buildExperienceFromWebpagePrompt`), `SYSTEM`-line minimal.
- [ ] *Conditional* `infra/private_llm/Modelfile.text` (only if the
      open question above escalates to running a second eval with a
      text-only model): pinned text model + tag (e.g.
      `qwen2.5:14b-instruct-q4_K_M`).
- [ ] `infra/private_llm/scripts/up.sh`: idempotent `ollama pull` +
      `ollama create -f` + smoke-test `curl` against
      `/v1/chat/completions` with a tiny JSON-schema response, exit
      non-zero on failure.
- [ ] Set `OLLAMA_KEEP_ALIVE=1h` (or longer) via Unraid systemd unit
      / env file so the model doesn't swap out mid-eval. Document
      where the env file lives in the README.
- [ ] Confirm the box is reachable from the dev machine via LAN
      address, document the exact hostname/port, and verify `curl
      http://<host>:11434/v1/models` returns the expected model(s).

**Validates:** Re-running `infra/private_llm/scripts/up.sh` on a
clean Unraid box produces a working endpoint without needing the
issue author's help.

### Phase 2: Server plumbing — `Private` provider, opt-in, branch-only

**Goal:** On the `private_llm` branch, the server can be told via
flags to construct a private provider. The default code path is
unchanged. The private provider is **not** in the default fallback
chain. **Not opened as a PR yet.**

**Tasks:**
- [ ] New file `server/ai/provider_private.go` (same `package ai`;
      no new package, so no `doc.go` needed). Public symbols
      (`NewPrivateLLMProvider`, `ProviderTypePrivate`) need godoc
      starting with the symbol name — `revive.exported` is a
      build-failing gate per CLAUDE.md.
- [ ] Add `NewPrivateLLMProvider(baseURL, model, temperature)
      (Provider, error)` as the public constructor. Internally it
      builds the same OpenAI-compatible HTTP client used by
      `OpenAIProvider`, threaded through `option.WithBaseURL(baseURL)`
      and `option.WithAPIKey("private")` (sentinel; Ollama accepts
      anything). Either embed `*OpenAIProvider` in a small wrapper
      struct or factor `OpenAIProvider` to accept a configurable
      `name` and optional `baseURL` — pick whichever is shorter in
      review.
- [ ] `Name()` returns `"private"`. No mention of "openai" leaks out
      of the public surface.
- [ ] **`CheckHealth(ctx)` implementation.** Required by the
      `ai.Provider` interface (the type won't compile without it);
      mirror `OpenAIProvider.CheckHealth` by GETing `/v1/models` on
      the configured base URL — Ollama exposes this and it's free.
      Return a `*health.Status` with `Backend: "ollama"` (or whatever
      the upstream actually is — record it from the response if
      available, else "ollama-compatible") and the resolved model
      name in `Metadata`. This is what unlocks the existing health-
      check pipeline; see `docs/server/observability.md` §Health
      Monitoring for the contract.
- [ ] Add `BaseURL string` to `ai.ProviderConfig` in
      `provider_factory.go`. (Used only by the private path in this
      issue; other provider types ignore it.)
- [ ] Add `ProviderTypePrivate ProviderType = "private"` and the
      matching `case` arm in `createProvider`, calling
      `NewPrivateLLMProvider(...)`.
- [ ] Add `PrivateLLMBaseURL`, `PrivateLLMModel`, `PrivateLLMTemperature`
      (with the same NaN-means-no-temperature semantics as the
      benchmark harness) to `AllProvidersConfig`. **Do not** include
      `private` in the default fallback chain.
- [ ] Add `--private-llm-enabled`, `--private-llm-base-url`, `--private-llm-model`,
      `--private-llm-temperature` flags to `server/main.go` and thread
      through `initializeAIProvider`. (Flag names match the existing
      `--openai-*` / `--anthropic-*` / `--gemini-*` pattern — no
      `-llm-` infix.) The flag's only role in *this* issue is to
      construct the private provider so a routing follow-up has the
      plumbing in place; this issue does not add an accessor or wire
      `private` into `LoadBalancedProvider`.
- [ ] Eval-harness consumption is direct: the harness imports and
      calls `ai.NewPrivateLLMProvider(...)` exactly like the existing
      `newProviderInstance` arms call `ai.NewAnthropicProvider` /
      `ai.NewOpenAIProvider` / `ai.NewGeminiProvider`. No
      server-side accessor is needed.
- [ ] In `server/main.go`, conditionally register the private
      provider's health check with `healthService.RegisterFunc("ai",
      ...)` when `--private-llm-enabled` is on. Since this issue ships
      only behind a flag that's off in production, the
      `health_check_unhealthy` alert won't fire in prod from this
      change. (When the future routing PR enables it in prod, the
      alert's per-backend granularity will flag `backend="private"`
      specifically, which is desired.)
- [ ] Unit tests on the constructor (mirror
      `provider_helpers_test.go:44+` with an `httptest` Ollama
      stand-in that returns a fixed structured response), the
      `CheckHealth` implementation against an `httptest`
      `/v1/models` stub, the `createProvider` arm, and the factory
      flag plumbing.
- [ ] `npm run build` clean (golangci-lint with revive + gofumpt +
      the project's `check_codeinternal` script).

**Validates:** A server started on the branch with
`--private-llm-enabled --private-llm-base-url=http://192.0.2.1:11434/v1
--private-llm-model=ripls-vision` constructs a working provider that
answers `Name() == "private"`, passes its own `CheckHealth` against
the LAN endpoint, and passes the existing mock-server tests, with
zero impact on prod startup paths when the flag is off. Nothing
pushed to main yet.

### Phase 3: Wire `private` into the eval harness

**Goal:** `go test -tags=benchmark ./server/ai/eval/
-private-base-url=... -private-models=...` ranks the private endpoint
on every existing suite, attributing per-case results as
`Test{Suite}/private/<model>/<case_id>`. Still on the branch, still
no PR.

**Tasks:**
- [ ] Add `privateBaseURL`, `privateModels` flags to
      `server/ai/eval/provider_setup_test.go` following the same
      `newStringSliceFlag` pattern. (No `privateAPIKey` flag — the
      sentinel is internal.)
- [ ] Extend `providerSpecsFromFlags()` and `selectProviderSpecs` so
      `private` is a valid `-provider` filter value.
- [ ] Add the `case "private":` arm to `newProviderInstance(t, name,
      model)` calling `ai.NewPrivateLLMProvider(...)`.
- [ ] Sanity-run *one* case (e.g. a single text-mode gear golden)
      against the private endpoint to confirm the SSE/stream parser
      handles Ollama's chunk shape. If it doesn't, this is where the
      `privateStreaming=false` fallback lands.
- [ ] Document the invocation in `server/ai/README.md` under the
      "How to run" eval section, alongside the existing provider
      recipes. (This README doc edit lands on the branch but only
      ships to main as part of the eventual phase 5 PR.)

**Validates:** A full `go test -tags=benchmark ./server/ai/eval/
-provider=private -private-base-url=... -private-models=...,...`
invocation produces a per-(provider, model) pass rate exactly like
the cloud providers, attributable in CI-style output. Run from the
dev machine on the LAN.

### Phase 4: Run the eval, write the report

**Goal:** `docs/ai/private_llm_eval.md` exists, captures the data,
and the report can be diffed against itself when the next model
lands. This is the decision input for phase 5.

**Tasks:**
- [ ] Run all 7 suites (`gear` × text/image/webpage, `experience` ×
      text/image/webpage, `request` × text) plus
      `TestUnifiedCreateClassifier*` against the private endpoint
      with the candidate vision-capable model (`qwen2.5vl:7b`
      initially) **and** against the current production primary
      (`vertexai/gemini-3.1-flash-lite`) for side-by-side
      comparison.
- [ ] Run a warm-up pass before any timing measurement; record cold
      vs. warm latency once per (model, suite) so the prefix-cache
      effect the issue calls out is visible in the data.
- [ ] If the vision model's text/webpage suites lag the cloud
      baseline materially, run the conditional second eval with a
      text-only model over text+webpage suites only and add a side-
      by-side section to the report. Otherwise note that one vision-
      capable model handled everything (as a positive result on its
      own).
- [ ] Report at `docs/ai/private_llm_eval.md` with: per-(provider,
      model, suite) pass rate table; median + p95 latency per suite
      (cold and warm); per-field failure breakdown for any suite
      below the `0.85` floor; structured-output fidelity notes (how
      often the private endpoint returns invalid JSON, off-schema
      fields, or hallucinated enum tokens); a short qualitative
      section on vision quality.
- [ ] **Strict-mode fidelity check.** Per `docs/server/llm.md`
      §Schema Definition Guidelines, real OpenAI with `Strict: true`
      enforces "all properties in the `required` array, no
      `omitempty`, value types not pointers." Ollama's `/v1` is
      historically more lenient — a response that *would* have been
      rejected by real OpenAI but passes through Ollama is a hidden
      bug that surfaces only at routing time. For every suite,
      record: did the endpoint return a schema-conformant response,
      or did it succeed only because the deserializer was tolerant?
      Cross-check by running the same prompts against the real
      OpenAI endpoint (we have credentials in `gcloud secrets`) and
      diff schema violations. Treat lenient-Ollama as a fail for
      production-routing purposes even if our test passes.
- [ ] The exact invocation that produced the numbers goes in the
      report's opening so the run is reproducible from the report
      alone.

**Validates:** A reader who hasn't followed the work can open the
report, see "private can/can't replace Gemini for the classifier-only
path" (or "for nothing yet"), and understand why.

### Phase 5: Decision + PR-or-close

**Goal:** Close this issue with a recommendation, and either open a
PR for phases 1-3's code + the report, or close the branch and only
land the report.

**Tasks:**
- [ ] One-page decision section appended to
      `docs/ai/private_llm_eval.md`: which methods, if any, are
      candidates for production routing; the proposed routing shape
      (weighted in `LoadBalancedProvider`? fallback after Gemini?
      classifier-only via a method-level override?); the proposed
      pass-rate threshold for the private row in `modelThresholds`;
      the rollback plan.
- [ ] **If the data supports the plumbing:** open a PR from
      `private_llm` to main with phases 1-3's code + the eval
      report. Defer the actual production routing change to a
      follow-up issue (filed at PR-open time, linked from #2057).
- [ ] **If the data does not support the plumbing:** open a smaller
      PR with just `docs/ai/private_llm_eval.md` + `infra/private_llm/`
      (the Modelfile/README are still useful as a reproducible
      record). Drop phases 2-3's server code; the branch closes
      after the docs PR merges. Summarize blockers in the issue
      close comment.
- [ ] Either way, close #2057 with the eval doc URL as the artifact.

**Validates:** This issue closes with an externally-visible
artifact — the eval doc — that future model-swap or hardware-upgrade
work can rerun against without reconstructing the invocation.

## Out of scope (mirroring the issue, restated for the audit)

- Embeddings (already local; `server/ai/embedding/`).
- Production routing changes or default-fallback-chain edits.
- vLLM / llama-server swap (revisit only if Ollama fails structured-output).
- Fine-tuning / LoRA.
- Multi-GPU, model architecture changes, replacing the embedding
  model.
- Production reachability story for `mimir` from Cloud Run
  (Tailscale subnet router, public-mTLS fronting, etc.) — picked in
  the follow-up routing issue, not here.
- Per-method model routing inside `PrivateLLMProvider` — flagged as
  a Phase-2+ lever only if phase 4 surfaces a text/vision quality
  gap that one model can't bridge.

## Related Issues

- *(none directly cited in #2057; if a follow-up routing issue is filed
  out of phase 5, link it here.)*

## Spike findings (2026-05-19)

A pre-Phase-2 spike (build-tagged `privateprobe` test under
`server/ai/private_llm_probe_test.go`) ran the production
`OpenAIProvider` machinery against `http://192.0.2.1:11434/v1`
on `mimir` to validate the plan's factoring decision before any
production-facing code lands. Three subtests covered the three
major paths the eval will exercise: a non-streaming smoke schema,
the full streaming `GenerateGearFromText` path with the real
`buildGearGenerationPrompt`-derived schema, and the streaming
image path via `DetectGearInImage` against a real test photo.
Results inform several decisions below.

### Warm-state latencies (Qwen 2.5-VL-7B Q4 on the 5060 Ti, GPU-resident)

| Path | Warm latency | First delta | Quality |
|---|---|---|---|
| Non-streaming simple schema | 2.0 s | — | ✓ Schema honored |
| Streaming text Gen* (real `buildGearGenerationPrompt`) | **2.3 s** | 583 ms | ✓ Title/category/brand/material/value all correct; `description` empty |
| Streaming image Gen* (`DetectGearInImage` on REI Half Dome 2 photo) | **4.0 s** | 1.35 s | ✓ Brand, model, category, material, value all correct; rich description |

These warm numbers are **competitive with cloud baselines** —
Gemini 3.1 Flash-Lite is ~3.2 s median on the gear-image suite per
the 2026-05-09 baseline; Anthropic Haiku 4.5 is ~3.9 s. The cost
delta ($0/token vs cloud pricing) is the load-bearing variable
the production-routing decision will hinge on, *not* a 10× latency
penalty.

### Validated by data

- **Phase 2's factoring decision (reuse `OpenAIProvider` under a
  `Private` public surface) works end-to-end.** Strict-mode JSON
  schema generated from `GearGeneration` via reflection round-trips
  through Ollama's `/v1` without a compat shim.
- **Streaming SSE shape lines up with `streaming_json.go`'s
  parser.** No `privateStreaming=false` fallback needed; can drop
  that contingency.
- **One vision-capable model for everything is the right call.**
  Beyond the prefix-cache argument the plan made on paper,
  *measured*: swapping between a text-only and a vision-capable
  Qwen 2.5-7B model costs ~50 s of cold-load time per swap.
  Alternating during an eval would dominate runtime. Also: the
  vision-capable model is *not slower* on text tasks (2.3 s vs
  4.0 s for text-only Qwen 2.5-7B-instruct), and was actually
  *better* at extracting `location_query` than the text-only
  model on the same prompt.

### Surfaced for Phase 4 eval

- **`description` field is empty on text-mode `GearGeneration`** in
  the spike output. May be a *prompt* issue (the existing
  `buildGearGenerationPrompt` enumerates other fields but never
  explicitly asks for `description` as a free-form field), in which
  case it likely affects cloud providers too. May be a
  *model-quality* issue. Phase 4's per-field pass-rate table will
  disambiguate; if it affects cloud providers too, file a follow-up
  prompt-tuning issue (out of scope for #2057).

### Operational lesson surfaced (and now documented in infra)

- **Ollama silently falls back to CPU when VRAM is insufficient at
  load time** and stays there until the model unloads — a ~25×
  latency penalty (~60 s vs ~2.3 s on the gear-text path) with no
  error or warning in the response. The only direct evidence is
  `ollama ps` reporting `100% CPU` in the `PROCESSOR` column.
- On `mimir`, the `koboldcpp` container holds ~11 GB of VRAM by
  default for creative-writing workloads. With it running, only
  ~5 GB is free, which isn't enough for Qwen 2.5-VL-7B at
  `num_ctx=6144`. The fix is `docker stop koboldcpp` (cheap and
  reversible on this shared host).
- Documented in `infra/private_llm/README.md` §VRAM gotcha and
  surfaced by a pre-flight check in `infra/private_llm/scripts/up.sh`
  that warns when free VRAM is below the recommended threshold.

## Audit log

`/ripls-audit` 2026-05-19 — applied:

- Flag-naming alignment: initially aligned to `--private-*` to match
  the existing `--openai-<attr>` / `--anthropic-<attr>` /
  `--gemini-<attr>` convention. **Reverted** on user feedback during
  Phase 2 implementation — `--private-*` is ambiguous in flag-help
  context (private *what?*) while the other prefixes are unique
  vendor names that don't have that problem. Final shape uses
  `--private-llm-*` flags and `PrivateLLM*` config fields. The
  wire-level identifier (`Name() = "private"`, `ProviderTypePrivate
  = "private"`) keeps the bare form because the disambiguation cost
  there is paid by co-occurring log fields like
  `model=ripls-vision` and `base_url=...` on every line.
- `CheckHealth` task added to Phase 2 — required by the `ai.Provider`
  interface (won't compile without it) and surfaced in
  `docs/server/observability.md` §Health Monitoring; previously
  silent in the plan.
- Health-checker registration in `main.go` added to Phase 2, with
  the prod-safety note that it ships behind a flag that's off in
  prod, so `health_check_unhealthy` won't fire prematurely.
- Dropped the "narrow accessor" phrasing — the eval harness builds
  providers directly via `ai.New*Provider(...)` per
  `server/ai/eval/provider_setup_test.go`; no server-side accessor
  needed.
- Added a positive observability note up front (`provider="private"`
  in log fields → `rpc_errors` metric → per-backend
  `health_check_unhealthy` attribution). Calling it out so the
  recommendation doesn't accidentally read like a new observability
  surface needs to be built.
- New file location (`server/ai/provider_private.go`) and
  `revive.exported` godoc requirement made explicit per CLAUDE.md
  §Go Code Comments.
- Phase 4 gained a "Strict-mode fidelity check" task per
  `docs/server/llm.md` §Schema Definition Guidelines — Ollama may
  succeed on responses that real OpenAI Strict mode would reject.
  That's a hidden bug for routing, and the eval has to catch it now
  rather than letting it surface in prod.

Client docs (`docs/client/architecture.md`, `caching.md`, `i18n.md`,
`accessibility.md`, `testing.md`) reviewed but produced no findings —
this is a server-only plan, no Flutter / repository / ViewModel /
ARB-string surface touched.
