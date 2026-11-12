# AI eval baselines

Machine-readable snapshots of `server/ai/eval/` runs against live AI
providers, committed periodically so that comparing a candidate
(e.g. a new model, a new prompt, a self-hosted endpoint) doesn't
require re-running the expensive cloud sweep every time.

## Layout

```
docs/ai/eval/baselines/
├── README.md                                 # this file
└── YYYY-MM-DD_<git-sha7>_<label>/            # one directory per run
    ├── experience.json                       # one file per suite
    ├── experience_image.json
    ├── experience_webpage.json
    ├── gear.json
    ├── gear_image.json
    ├── gear_webpage.json
    ├── request.json
    ├── community.json
    └── unified_classifier.json
```

The directory name encodes the date, the git SHA at the time of the
run, and a short label describing what's in it (`cloud`, `private`,
`smoke`, etc.). The eval harness writes the suite files; the surrounding
directory is created by `-eval-report-out=<dir>` (the harness mkdirs
ancestors as needed).

## File format

Per-suite JSON, schema v1. Top-level fields:

| Field | Meaning |
|---|---|
| `schema_version` | Constant `1` today. Bump when the shape changes incompatibly. |
| `suite` | Go test name (e.g. `"TestRequestPrompt"`). |
| `run.timestamp_utc` | ISO 8601 UTC of when this suite finished. |
| `run.git_commit` | First 10 chars of HEAD at run time. |
| `run.git_branch` | Branch name at run time. |
| `run.label` | The `-eval-report-label` value (`"cloud"`, `"private"`, etc.). |
| `goldens_file` | Source file name (no path) inside `server/ai/eval/testdata/`. |
| `goldens_version` | The `version` field inside the goldens JSON. Bumped by hand whenever the goldens change. |
| `pairs[]` | One element per `(provider, model)` pair that ran. |

Per-pair fields:

| Field | Meaning |
|---|---|
| `provider` | `"anthropic"` / `"openai"` / `"gemini"` / `"private"`. |
| `model` | Model identifier (Gemini's slashes are preserved here, not in `provider`). |
| `pass_rate` | Overall pass rate across all checks for this pair. |
| `threshold` | The `modelThresholds` value applied to this pair (or `defaultPromptEvalPassRate`). |
| `passed` | `pass_rate >= threshold`. |
| `median_latency_ms` | Median wall-clock per provider call, in ms. |
| `p95_latency_ms` | 95th percentile latency, in ms. |
| `field_pass_rates` | Per-field-assertion pass rate. Keys vary per suite. |
| `invocation_failures` | Case IDs where the provider call itself errored (rate-limit-exhausted, network, schema-rejection). Absent when zero. |

Per-case detail (per-case latencies, individual field check outcomes)
is intentionally NOT captured at schema v1 — the per-field pass rates
already capture the granularity we use for prompt-tuning decisions,
and skipping case-level data keeps these files small and diff-friendly
in PRs. Schema v2 can add a `cases[]` array when we have a use case.

## Generating a baseline

Pass `-eval-report-out=<dir>` to the harness. Convention is to use the
literal date + git SHA + a label in the path:

```bash
git_sha=$(git rev-parse --short=7 HEAD)
date_iso=$(date -u +%Y-%m-%d)
out="docs/ai/eval/baselines/${date_iso}_${git_sha}_cloud"

# Cloud baseline: the three small production models, all suites.
go test -tags=benchmark -v -timeout 4h ./server/ai/eval/ \
  -anthropic-api-key-file=/tmp/ripls-eval/anthropic \
  -anthropic-models=claude-haiku-4-5 \
  -openai-api-key-file=/tmp/ripls-eval/openai \
  -openai-models=gpt-5-mini \
  -gemini-project=ripls-dev \
  -gemini-models=vertexai/gemini-3.1-flash-lite \
  -eval-report-out="${out}" \
  -eval-report-label=cloud

# Private baseline (per #2057). Run separately so the model swaps
# don't cost ~50s of cold-load between cloud and private.
out="docs/ai/eval/baselines/${date_iso}_${git_sha}_private"
go test -tags=benchmark -v -timeout 90m ./server/ai/eval/ \
  -provider=private \
  -private-base-url=http://192.0.2.1:11434/v1 \
  -private-models=ripls-vision \
  -eval-report-out="${out}" \
  -eval-report-label=private
```

## When to re-run

- **A new model lands** in production — re-run the cloud baseline so
  diffs are against the actual prod-serving pair.
- **The goldens change materially** (bumped `version` field, new cases,
  retired cases) — re-run so the saved baseline is comparable.
- **A new candidate** (private endpoint, prompt revision, vLLM swap)
  needs a comparison — run the candidate only, compare to the most
  recent saved cloud baseline.

Don't re-run cloud after small cosmetic tweaks. Each run costs
$5–15 and 3–4 hours of wall-clock; the data we keep should be from
deliberate snapshots, not casual reruns.

## Comparison conventions

When writing a comparison report (e.g. `docs/server/private_llm_eval.md`):

- Reference baseline files by relative path from the repo root so
  reviewers can navigate.
- Quote the `run.timestamp_utc` and `run.git_commit` so anyone can
  reproduce.
- If goldens have changed between the two runs (different
  `goldens_version`), flag the comparison as approximate and explain.
- Comparing pass rates across runs with different `schema_version`
  values is unsafe — first migrate the older file forward to the new
  schema (e.g. via a one-off script), or qualify the comparison.

## Historical context

The first periodic baseline (#1777) lives at
[`docs/issues/1777-baseline-2026-05-09.md`](../../../issues/1777-baseline-2026-05-09.md)
and predates this machine-readable convention — it captures the
gear-image suite across 8 cloud (provider, model) pairs in narrative
form. The 2026-05-09 numbers there for the three small models
(Haiku 4.5, GPT-5.4 nano, Gemini 3.1 Flash-Lite) remain the most
recent gear-image baseline for those pairs even after the JSON
convention lands.
