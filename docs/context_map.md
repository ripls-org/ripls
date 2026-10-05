# The documentation context map

This repo has ~90 durable guidance docs under `docs/`. No single task needs all
of them. The **context map** lets a task — a skill, an agent, or a human — load
only the docs salient to the work at hand, instead of reading everything or
guessing.

It works in two pieces:

1. **Per-doc `context:` frontmatter.** Every durable doc declares, in a small
   YAML block, what it covers and which work it's relevant to.
2. **A generated index, `docs/llms.txt`.** `scripts/gen_context_map.js` reads
   every block and emits a curated, [llms.txt](https://llmstxt.org/)-format index
   grouped by area. Skills and the CLAUDE.md "Finding Documentation Context"
   protocol read `docs/llms.txt` to select what to load.

A CI gate (`npm run lint:context-map`) keeps the index and the blocks honest.

> **If you change a doc, update its `context:` block. If you add a durable doc,
> add one.** Then run `npm run generate:context-map` and commit the regenerated
> `docs/llms.txt`. The gate fails the build otherwise.

## The `context:` block

Each durable doc starts with a breadcrumb comment (pointing back here) and a
`context:` block:

```yaml
---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: How the server calls LLM providers — streaming, fallback, eval hooks.
  globs: [server/ai/**, server/services/experience/**]
  triggers: [llm, gemini, provider, classifier]
  lens: [architecture, server]
  skills: [issue, audit]
  alwaysApply: false
  domain: server
---
```

| Field | Required? | Type | Meaning |
|-------|-----------|------|---------|
| `description` | required for anchors; else optional | string | One-line summary shown in the index — what a selector reads to decide relevance. Omitted ⇒ falls back to the doc's first body paragraph. |
| `globs` | optional | list of path globs | **Primary trigger.** The doc is relevant when the work touches a matching path (e.g. editing `server/ai/**`). |
| `triggers` | optional | list of strings | **Secondary trigger.** Conceptual keywords for docs that don't map cleanly to a code path (`ssrf`, `n+1`, `rsvp`). |
| `lens` | **required** | list (controlled vocab) | The perspective(s) the doc serves. Drives anchor routing and index grouping. |
| `skills` | optional (default: all) | list of skill names | Which skills consider this doc. Omitted ⇒ all skills. |
| `alwaysApply` | optional (default: false) | bool | `true` ⇒ an **anchor**: always loaded for its lens (see below). |
| `domain` | optional (default: from path) | string | Index grouping only. Top-level `docs/*.md` should set this. |

### `lens` vocabulary

The only controlled field. The gate rejects any value not in this list (so it
can't sprawl); to add one, edit `LENS_VOCAB` in `scripts/gen_context_map.js` and
this list together.

**Structural lenses** — perspective, side, and topic:

`architecture` · `conventions` · `product` · `infra` · `client` · `server` ·
`domain` · `workflow`

- **architecture / conventions** — system structure and the rules code must
  follow (the usual always-on anchors).
- **product** — personas, priorities, journeys; "is this the right thing to
  build."
- **infra** — CI/CD, hosting, secrets, toolchain.
- **client / server** — which side a doc is about.
- **domain** — a feature/business-logic area (chat, impact, invitations…).
- **workflow** — the loan / giveaway / request / experience state flows.

**Technical-dimension lenses** — one per engineering-review dimension. Each
`docs/reviews/<dimension>_review_prompt.md` is the **anchor** for its lens:

`accessibility` · `api_design` · `architecture` · `code_quality` ·
`dependency` · `error_handling` · `observability` · `performance` ·
`security` · `tech_debt` · `testing`

When a dimension is engaged by the work, load its review prompt and follow that
prompt's **"Areas to Review"** section as the rubric — **ignore the prompt's
runner scaffolding** (*Instructions*, *Step 0*, *Output Format*), which is for
the `/ripls-reviews` runner, not for planning or auditing. (`architecture`,
`observability`, `security`, and `testing` are both structural and dimension
lenses.)

### `skills`

`issue` · `audit` · `triage` · `reviews` — the doc-driven skills. Use this to
keep a doc out of an irrelevant skill (e.g. `personas.md` is `[issue, triage]`,
not `audit`; the review prompts are `[audit, triage, reviews]`, not `issue`).

## How selection works

A run has a set of **active lenses**:

- the selector's **base lenses** — always on (see the table) — plus the **side**
  (`client` / `server`) inferred from the work's paths; and
- the **engaged dimensions** — the technical-dimension lenses the work touches,
  inferred from the dimension review-prompt `triggers` / `globs` and the
  selector's read of the change.

A doc loads when its `skills` includes the selector **and** either:

- **Anchor** (`alwaysApply: true`): **all** of the doc's `lens` values are active
  (`lens ⊆ active lenses`). This pulls the architecture/conventions/product
  anchors for the side in play, plus the review prompt for each engaged
  dimension. The subset rule is why a Flutter change loads
  `client/architecture.md` but not `server/architecture.md`.
- **Leaf** (not an anchor): the work's paths match the doc's `globs`
  (deterministic), or the task text matches its `triggers` / `description`
  (judgment).

Base lenses by selector:

| Selector | Base lenses (always active) | Dimensions engaged |
|----------|-----------------------------|--------------------|
| `ripls-issue` | architecture, conventions, product (+ side) | the dimensions the plan will introduce — e.g. a new endpoint engages api_design, error_handling, security, testing |
| `ripls-audit` | architecture, conventions (+ side) | the dimensions the change touches — for server work typically error_handling, observability, security, testing, code_quality, api_design; plus performance / dependency / accessibility when relevant |

For every engaged dimension, the loaded review prompt's **"Areas to Review"**
section is the rubric to apply (the rest of the prompt is runner scaffolding).

`CLAUDE.md` § *Finding Documentation Context* describes the same protocol for any
task that isn't running through a skill.

## Regenerating and the gate

```bash
npm run generate:context-map   # rewrite docs/llms.txt from the blocks
npm run lint:context-map       # CI gate: blocks valid AND docs/llms.txt current
```

`lint:context-map` (wired into `npm run lint`) fails on: a malformed block, an
unknown `lens`, an `alwaysApply` anchor with no `description`, or a stale
`docs/llms.txt`. Docs with no block yet are reported as "untagged" but don't fail
the build until the backfill is complete (`ENFORCE_FULL_COVERAGE` in the
generator).

## What's indexed (and what isn't)

Indexed: durable guidance — conventions/architecture, client/server subsystems,
workflows, domain/feature docs, infrastructure & ops, product/users, and the
engineering rubrics.

Not indexed (outputs / ephemera): `docs/issues/`, `docs/ai/`,
`docs/release/notes/`, `docs/crashlytics/`, `docs/reviews/reports/`,
`docs/evals/`, and this spec itself. See `EXCLUDED_PREFIXES` in the generator for
the authoritative list.

## Prose freshness (the `freshness:` block)

The `context:` block keeps **routing** honest, but it does not guard the doc
**body**. A separate mechanism — the `ripls-doc-freshness` skill and its
`scripts/doc_freshness.js` helper (#2420) — verifies that the prose still
*semantically* matches the code it describes, and records the result in a
**second, top-level frontmatter block**:

```yaml
---
# ... breadcrumb comment ...
context:
  # ... routing metadata (above) ...
freshness:
  verified_commit: "1c742d1dd"   # the commit the prose was last verified against
  verified_on: "2026-06-08"      # ISO date of that verification
---
```

Key properties:

- **Separate from `context:` on purpose.** `gen_context_map.js` reads only
  `doc.context`, so a `freshness:` block is invisible to the routing gate —
  stamping a doc never changes `docs/llms.txt` or trips `lint:context-map`. (A
  `node --test` cross-check in `scripts/doc_freshness.test.js` locks this in.)
- **Earned by verification, not by authorship.** Anyone may advance the stamp —
  the skill on every verification run (whether it edits the doc or confirms it
  fresh), the nightly ratchet, or you, by hand, when you have checked the prose
  against the code. The rule is verify *then* stamp; what the stamp must never
  record is a claim nobody checked. So: update a doc and re-read it against the
  code it describes → stamp it. Fix one paragraph without reading the rest →
  leave the stamp where it is, and let the next verification run move it. A
  stale stamp on a doc you improved costs nothing; the ratchet will come back
  to it. A stamp on unread prose is the one thing that breaks the mechanism,
  because every later reader takes it as a guarantee.
- **Drives the ratchet.** The stamp lets any check ask the precise question *"has
  this doc's globbed code changed since it was last verified?"* — powering the
  nightly auto-refresh without a sidecar manifest.

See the `ripls-doc-freshness` skill (`.claude/skills/ripls-doc-freshness/`) for
the verification rubric and the apply/stamp flow.

## Design rationale

The full rationale — why frontmatter-on-docs over separate rule files, why globs
over keywords, why a CI gate, and how this maps to the Cursor/Copilot/llms.txt
prior art — is in the plan at `docs/issues/2398-context-doc-tree.md`.
