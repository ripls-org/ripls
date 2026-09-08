# AI Eval Harness

The `eval` package runs hand-crafted golden datasets against a live AI provider to catch prompt regressions and give prompt rewrites a quantitative pass/fail signal. It covers every input mode (text, image, webpage) for each of the three user-facing generation types: gear, experience, and request.

## When to use this package

Run the eval suite when changing prompts in `server/ai/prompts.go` or when adopting a new model. The default `go test` run and CI skip these tests because they require live API credentials; see `server/ai/README.md` for how to invoke them. Unit tests for the helper functions (`Jaccard`, `Check`, `Tally`, etc.) run in the default suite without any build tag.

## Key files

- `eval.go` — shared evaluation primitives: `Check`, `Tally`, `Report`, `Jaccard` similarity, tolerance helpers.
- `gear.go`, `gear_image.go`, `gear_webpage.go` — evaluators for the three gear generation modes.
- `experience.go`, `experience_image.go`, `experience_webpage.go` — evaluators for the three experience generation modes.
- `request.go` — evaluator for request generation (text only).
- `prompt_test.go` — live-API benchmark tests; gated behind the `benchmark` build tag.
- `testdata/` — JSON golden files; one file per mode and type (e.g. `gear_goldens.json`, `experience_image_goldens.json`).

## Adding a golden

Drop a new entry into the appropriate `testdata/*_goldens.json`. Only fields listed in the `expected` block are checked; omitted fields are ignored. Per-field tolerances for inherently noisy inferences (weight, value) are expressed inline in the golden data.

### Seeding image-mode gear goldens from manufacturer URLs

`server/cmd/eval-seed-from-urls` is an interactive ingest tool that turns manufacturer product-page URLs into proposed golden cases for `gear_image_goldens.json`. Curated URLs across diverse brands give us the brand variety that catalog datasets (Amazon Berkeley Objects was the first attempt — fatally Amazon-house-brand-dominated) cannot.

Curate a list of ~30–50 URLs in `scripts/eval_seed_urls.txt`, one per line, with optional category-hint prefix:

```text
power-tools  https://www.dewalt.com/product/dcd771c2/...
camping      https://www.rei.com/product/187624/...
kitchen      https://www.kitchenaid.com/major/stand-mixers/...
```

Run the ingest interactively:

```bash
go run ./server/cmd/eval-seed-from-urls \
  -urls scripts/eval_seed_urls.txt \
  -out-cases /tmp/proposed-cases.json \
  -out-images server/test_data/manual
```

Per URL, the tool fetches the page, extracts product metadata from Schema.org Product JSON-LD (with OpenGraph fallback), and prints the extracted title / brand / model / weight / price / image to the terminal. The operator then accepts as-is (`a`), edits any field (`e`), skips (`s`), or quits-and-saves (`q`). Accepted cases are appended to `-out-cases` immediately and the hero image is downloaded into `-out-images/`, so Ctrl-C is recoverable — re-run with the same arguments and the tool resumes by skipping already-accepted URLs.

Review `/tmp/proposed-cases.json`, then append accepted entries to `testdata/gear_image_goldens.json` and commit the corresponding files under `server/test_data/manual/`.

The extractor handles Schema.org Product JSON-LD (`name`, `brand`, `model`/`mpn`, `weight` with QuantitativeValue, `offers` with USD price, `image`, `description`) and falls back to OpenGraph and `<meta>` tags. Internal-SKU-shaped `model` values (alphanumeric soup with no spaces, like `DCD771C2`) get filtered out at finalize-time so we don't assert on strings vision models won't reproduce.

## Style notes when calibrating goldens

A golden's `title_contains` and `search_keywords_general` lists are an implicit bet on a *style* the model will produce — literal nouns ("garden", "climb") vs. evocative branding ("Boulder Backyard Hub", "Boulder Beta Buddies"). Different models pick different styles for the same prompt; the eval shouldn't penalize one stylistic choice over another when both demonstrate topic understanding.

When adding or revising a community/experience golden, prefer breadth: include synonyms and topic-adjacent tokens that any reasonable model output could plausibly contain. Use `title_contains_mode: "any"` (the default) and avoid `search_keywords_must_contain` unless one specific token is genuinely required for downstream behavior. The 2026-05-09 cross-task baseline (#1777) caught a Haiku-vs-Gemini style mismatch that was fixed by relaxing #1800 — the underlying assertions were valid against Haiku's literal output but failed Gemini's creative output, even though both demonstrated understanding.

## Attribution

Hand-curated images under `server/test_data/` (`atomic_maverick_ski.webp`, `mixer.jpg`, `rei_half_dome.jpg`, `rocky_talkie.jpeg`, `tent.jpg`, etc.) were photographed for this project. URL-sourced images under `server/test_data/manual/` come from the manufacturer's product page; the `source_url` recorded on each golden case is the attribution pointer. Use only with respect for the manufacturer's terms; in practice we use the same hero image vision models would see when a user uploads from a product detail page, and the eval suite is internal and CI-skipped, so this is a fair-use, internal-research footprint.
