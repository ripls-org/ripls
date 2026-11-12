# Issue #2398: Hierarchical doc-context routing for ripls-issue/ripls-audit (frontmatter + globs + generated llms.txt + CI gate)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2398
**Filed:** 2026-06-07 | **Last activity:** 2026-06-07
**Labels:** documentation, enhancement | **Assignees:** unassigned
**Context docs:** none (design settled interactively — see Design Decisions below)

## Problem Statement

The `ripls-issue` and `ripls-audit` skills drive correctness from architecture
and product docs, but their doc handling is broken in two opposite ways, and
both are blind to most of the repo's durable guidance.

- **`ripls-audit` hardcodes 7 docs and reads all of them on every audit**
  (`docs/client/{architecture,caching,i18n,accessibility,testing}.md`,
  `docs/server/{architecture,observability}.md`).
  - *Stale:* that list predates the infrastructure docs (`infra_mimir`,
    `secrets`, `secret-rotation`, `toolchain_automation`,
    `dependency_modernization`), the server-subsystem docs (`llm`, `metrics`,
    `web`, `semantic_search`, `scheduled_notifications`, `l10n`, `profiling`,
    `private_llm_*`), `proto_conventions.md`, `testing/architecture.md`, and the
    ~20 client/server subsystem docs. It carries **no infrastructure or security
    lens at all** — the exact gap that motivated this issue.
  - *Inefficient:* a pure-Flutter UI audit still loads `server/observability`; a
    server SSRF audit still loads `client/i18n` + `accessibility`. Seven docs,
    unconditionally, regardless of what the plan touches.

- **`ripls-issue` auto-loads zero guidance docs.** It relies on the user
  hand-passing context-doc paths as args (`/ripls-issue 757 docs/foo.md`), which
  requires already knowing which of ~90 docs matter. In practice most plans are
  written with no product/architecture grounding. This is arguably the larger
  gap.

The fix is a single mechanism both skills share: route **only the salient
subset** of the **full** durable-guidance set into context per run, structured
hierarchically, kept honest by a CI gate.

## Context from Comments

Issue just filed; no comments yet. The design below was settled interactively
with the maintainer over four decision points (index shape, selection authority,
load depth, freshness enforcement) plus a prior-art review that revised two
choices (glob triggers, llms.txt output).

## Codebase Findings

- **No doc index exists.** There is no `docs/README.md`, `INDEX.md`, or map. The
  only index-style files are `docs/users/README.md` ("How to Use These Docs")
  and the `## Table of Contents` / `## Additional Resources` sections inside
  `docs/server/architecture.md`. Filenames are unreliable as a guide:
  `docs/planning.md` is a *feature* doc (Needs & Contributions), not a process
  map.
- **Doc corpus: 597 markdown files, but only ~90 are durable guidance.** The
  rest are outputs/ephemera: `docs/issues/*` (373 issue-plan outputs of this
  very skill), `docs/release/notes/*` (40), `docs/ai/*` (66 scratch plans),
  `docs/crashlytics/*`, `**/eval*`, `docs/reviews/reports/*`. These must be
  excluded from the tree.
- **The repo already has a mature gate-script family** wired into `npm run lint`:
  `scripts/check_{ephemeral_comments,versions,codeinternal,active_community_gate,
  i18n,a11y,...}.js`. They share a house style: Node, `'use strict'`, a file
  walk, self-tests that run before the scan, a clear violation report, and
  `process.exit(1)` on failure. `scripts/check_ephemeral_comments.js` is the
  closest template for our generator/gate.
- **`npm run lint` is the single lint entrypoint** (package.json), composing
  ~16 sub-linters; `test_flutter.yaml` is the CI workflow that runs lint. Adding
  `lint:context-map` to the chain is a one-line wiring change.
- **Sibling skills already use the vocabulary we're formalizing.**
  `ripls-triage` reads docs by "lens" (`docs/users/` = product lens,
  `docs/reviews/` = engineering lens) but brute-reads *all* of each dir.
  `ripls-reviews` discovers `docs/reviews/*_review_prompt.md` from the
  filesystem rather than hardcoding — the anti-staleness pattern to copy.
- **Per-package READMEs already exist and are CI-enforced** under `server/` and
  `app/lib/` (CLAUDE.md + issue #1334). They are the natural finest tier, but we
  deliberately stop above them (see Decision 3).

## Design Decisions

These were decided interactively; alternatives considered are noted for the
record.

### Decision 1 — Index shape: per-doc frontmatter + generated index

Metadata lives in a `context:` YAML frontmatter block on each durable doc (single
source of truth, co-located). A generator reads the frontmatter and emits the
Tier-0 index. *Alternatives rejected:* a single hand-maintained map file (drifts,
grows unboundedly); per-area index files (more files to sync, still
hand-maintained).

### Decision 2 — Selection: hybrid (deterministic anchors + matched leaves)

A guaranteed anchor floor per lens, plus glob/keyword/description-matched leaf
docs. *Alternatives rejected:* pure model judgment (can skip a load-bearing
doc); a fully deterministic routing table (brittle, can't reason about novel
issues).

### Decision 3 — Depth: stop at subsystem docs

Auto-loading stops at subsystem/feature-doc granularity. Per-package READMEs are
**not** auto-pulled; normal codebase research surfaces them. *Alternative
rejected:* auto-descend into touched-package READMEs (more reads per run for
marginal gain, given they're already enforced to exist and research finds them).

### Decision 4 — Freshness: CI staleness gate

`npm run lint:context-map` regenerates the index and diffs; drift, missing
frontmatter, or an invalid `lens` value fails the build. *Alternative rejected:*
convention-only (which is exactly what let the current 7-doc list rot).

### Decision 5 — Triggers: globs primary, keywords secondary

Adopt the Cursor/Copilot mechanism — `globs` (file-path patterns) are the primary
relevance signal, matched against the work's actual touched/changed paths;
keyword `triggers` are a secondary aid for conceptual docs that don't map to a
code path. *Alternatives rejected:* keyword-only (less deterministic), glob-only
(can't express product/strategy docs with no code path).

### Decision 6 — Stand on the convergent industry pattern

The schema mirrors the pattern Cursor `.mdc` rules (`description` + `globs` +
`alwaysApply`; "Agent Requested" = model selects from `description`), Copilot
`.github/instructions/*.instructions.md` (`applyTo:` globs), and AGENTS.md
converged on. The Tier-0 artifact conforms to **llms.txt** (the curated-markdown
index format already published by Anthropic/Stripe/Cloudflare and consumed by
IDE agents). Our additions on top: the `lens`/`skills` fields, the CI gate, and
skill-driven deterministic selection. *Alternative rejected:* adopting an
off-the-shelf rules system wholesale — they're tool-specific (`.cursor/`,
`.github/`), give no skill-driven selection, and have no staleness gate.

### Decision 7 — Project-wide adoption via CLAUDE.md

The map is not only for the two skills. Once the index exists and the gate is
enforcing (end of Phase 2), a **"Finding Documentation Context"** section in the
project `CLAUDE.md` directs *every* task — human or agent, feature work, bug fix,
or review — to consult `docs/llms.txt` and read the docs whose `globs`/`lens`
match the work *before* writing code (and not to read every doc). This
generalizes the skill protocol into a project-wide working norm and is the
highest-leverage payoff of the index: a bug fix touching `server/ai/**` should
read `server/llm.md` whether or not it came through `ripls-issue`. The drafted
section is in the Appendix. It lands only *after* Phase 2 so it never points at a
missing or incomplete index. *Alternative rejected:* leaving the protocol
encapsulated in the two skills (re-creates the original gap — most work doesn't
flow through a skill).

### Decision 8 — Self-describing docs: breadcrumb to the system hub

CLAUDE.md is auto-loaded for Claude Code, but it is *not* loaded for other
readers — non-Claude tools (Cursor/Copilot/CI bots), humans skimming a doc on
GitHub, or narrow sub-agents — and can be compressed away in very long sessions.
So a doc's `context:` block must be self-describing: a one-line breadcrumb
comment above it points to the canonical explainer, **`docs/context_map.md`**,
which fully documents the schema, the selection protocol, the obligation to
update the block when the doc changes, and how to regenerate. The breadcrumb is
a YAML comment (the generator ignores `#` lines), so it never reaches the index.
`docs/context_map.md` is the single hub; CLAUDE.md, the `docs/llms.txt` header,
and both skills all point to it (stable pointers, one source of truth).
*Alternative rejected:* relying on CLAUDE.md alone (invisible to every
non-Claude reader and to the GitHub web view of a doc).

### Decision 9 — Review prompts are dimension anchors; lens vocab expands

The engineering-review prompts under `docs/reviews/` hold distilled
**"Areas to Review"** guidance per dimension — exactly the kind of "what good
looks like" a plan or audit should follow. Rather than a single catch-all
`rubric` lens, each dimension becomes a first-class **technical lens**
(`accessibility`, `api_design`, `code_quality`, `dependency`, `error_handling`,
`performance`, `security`, `tech_debt`, plus `architecture`/`observability`/
`testing`), and each `X_review_prompt.md` is the **anchor** for lens `X`. When a
dimension is engaged by the work, its prompt loads and the agent follows the
prompt's **"Areas to Review"** section as the rubric (ignoring the
`/ripls-reviews` runner scaffolding — *Instructions*, *Step 0*, *Output Format*).

The guidance is **referred to, not factored out**: it already lives in exactly
one place, so extracting it would only create a second copy that drifts. The
review prompt stays the single source; plans/audits read its "Areas to Review"
section in place. *Alternatives rejected:* a generic `rubric` lens (loses
per-dimension routing); copying "Areas to Review" into standalone files
(needless duplication + drift risk — revisit only if the runner scaffolding
proves to confuse loaders).

This refined the selection model: an anchor loads when **all** its lenses are
active (`lens ⊆ activeLenses`), where `activeLenses` = the selector's base lenses
+ side + the dimensions the work engages. The subset rule fixes a latent bug — a
Flutter change no longer pulls `server/architecture.md`.

## The Schema

Each durable doc carries one `context:` frontmatter block, preceded by the
breadcrumb comment:

```yaml
---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: How the server calls LLM providers — streaming, fallback, eval hooks.
  globs: [server/ai/**, server/services/experience/**]   # PRIMARY trigger (path match)
  triggers: [llm, gemini, provider, classifier]          # SECONDARY (conceptual keywords)
  lens: [architecture, server]                           # controlled vocabulary
  skills: [issue, audit]                                 # default: [issue, audit, triage, reviews]
  alwaysApply: false                                     # true => always-load anchor for its lens
  domain: server                                         # optional; default derived from path
---
```

| Field | Required | Type | Meaning |
|-------|----------|------|---------|
| `description` | required for anchors; else optional (falls back to first paragraph) | string | One-line summary shown in the index; what the model selects on. |
| `globs` | optional | list of path globs | Primary trigger — a doc is relevant when the work touches a matching path. |
| `triggers` | optional | list of strings | Secondary trigger — conceptual keywords for docs without a clean code-path mapping. |
| `lens` | required | list (controlled vocab) | Drives anchor routing + grouping. See vocabulary below. |
| `skills` | optional (default all) | list of skill names | Which skills consider this doc. Gates the candidate set. |
| `alwaysApply` | optional (default false) | bool | `true` => always loaded for its lens (the anchor floor). |
| `domain` | optional (default from path) | string | Map grouping only. |

**Controlled `lens` vocabulary** (the gate enforces membership; extend by editing
`LENS_VOCAB` in the generator + `docs/context_map.md`). Structural:
`architecture`, `conventions`, `product`, `infra`, `client`, `server`, `domain`,
`workflow`. Technical dimensions (one per `docs/reviews/*_review_prompt.md`,
which is the anchor for its lens): `accessibility`, `api_design`, `code_quality`,
`dependency`, `error_handling`, `observability`, `performance`, `security`,
`tech_debt`, `testing` (+ `architecture`). See Decision 9.

**Selection logic (the hybrid protocol).** `docs/context_map.md` is the canonical
spec; in brief:

```
activeLenses(skill, work) = baseLenses(skill) ∪ side(work) ∪ engagedDimensions(work)
anchorsFor(skill, work)   = docs where alwaysApply == true
                              AND lens ⊆ activeLenses(skill, work)   # subset rule
                              AND skill ∈ doc.skills
leavesFor(skill, work)    = docs where skill ∈ doc.skills
                              AND ( anyGlob(doc.globs, touchedPaths)        # deterministic
                                    OR modelJudges(doc.triggers, doc.description, text) )  # judgment
load = anchorsFor ∪ leavesFor
```

The **subset rule** (`lens ⊆ activeLenses`) is why a Flutter change loads
`client/architecture.md` but not `server/architecture.md`. For each engaged
dimension, the loaded review prompt's **"Areas to Review"** section is the rubric.

| Skill | base lenses (always active) | engaged dimensions |
|-------|------------------------------|--------------------|
| `ripls-issue` | architecture, conventions, product (+ side) | dimensions the plan introduces (new endpoint → api_design, error_handling, security, testing) |
| `ripls-audit` | architecture, conventions (+ side) | dimensions the change touches (server: error_handling, observability, security, testing, code_quality, api_design; + performance/dependency/accessibility when relevant) |

## Generated `docs/llms.txt` (Tier-0 index)

llms.txt-format, grouped by `domain`, regenerated from frontmatter and **committed**
(it is a small, human-readable, served-able doc index — appropriately versioned,
unlike generated Go/Dart code; the gate diffs against the committed copy):

```markdown
# Ripls documentation context map

> Auto-generated by scripts/gen_context_map.js from per-doc `context:` frontmatter.
> Do not edit by hand — run `npm run generate:context-map`. CI gate: `npm run lint:context-map`.

## server
- ⚓ [server/observability.md](server/observability.md): Structured logging, request IDs, masking, levels. — lens: observability, server — skills: audit, triage
- [server/llm.md](server/llm.md): How the server calls LLM providers; streaming, fallback. — lens: architecture, server — globs: server/ai/** — skills: issue, audit

## infra
- [infra_mimir.md](infra_mimir.md): Self-hosted CI runner host; image rebuilds, repair. — lens: infra — globs: .github/workflows/**, runners/** — skills: audit
```

(⚓ marks `alwaysApply` anchors.)

## Open Questions

- **YAML parser for the generator.** ✅ *Resolved:* `js-yaml` added as a
  devDependency (the maintainer waived the minimal-deps preference for a standard
  YAML parser). It parses the breadcrumb `#` comments natively. CI installs it via
  the existing `npm install` step.
- **`description` required vs. first-paragraph fallback.** **Recommendation:**
  optional with fallback to the doc's first non-heading paragraph (cuts backfill
  cost), **but required for `alwaysApply` anchors** since they're load-bearing.
- **Should globs be validated against the filesystem (catch rotted paths)?**
  **Recommendation:** soft warning in Phase 1 (a glob matching zero paths is
  likely stale), promote to hard error in a follow-up once the corpus is clean.
- **Where does the per-skill `anchorLenses` config live?** **Recommendation:**
  inline in each `SKILL.md` (the skills are the consumers; keep it transparent
  and co-located) rather than a shared config file.
- **Final `lens` vocabulary.** ✅ *Resolved:* 18 values — 8 structural + the 11
  technical review dimensions (some overlap). The `rubric` placeholder was
  dropped in favor of per-dimension lenses (Decision 9). The gate's allowlist
  makes adding one a deliberate one-line change.
- **`docs/llms.txt` location/name.** **Recommendation:** `docs/llms.txt` (the
  standard filename), generated + committed + diffed.

## Implementation Plan

### Phase 1: Schema + generator + gate (no mass doc edits)

**Goal:** the mechanism exists and is enforced on a seed set; adding/removing a
doc with bad frontmatter fails CI.

**Tasks (✅ done):**
- [x] Schema + `lens` vocabulary finalized in this doc and in the new
      `docs/context_map.md` authoring/hub guide (every doc's breadcrumb points
      there).
- [x] `scripts/gen_context_map.js` (house style of `check_ephemeral_comments.js`):
      walks `docs/**/*.md`, skips the exclude globs, parses the `context:` block
      via `js-yaml`, validates fields + `lens` membership + anchor-needs-
      description + unknown-key guard, emits `docs/llms.txt`. `--check` mode
      diffs against the committed file; pre-scan self-tests cover valid/invalid/
      missing/malformed blocks. `ENFORCE_FULL_COVERAGE=false` until Phase 2 ends.
- [x] npm scripts `generate:context-map` + `lint:context-map`, the latter wired
      into the `lint` aggregate **and** a dedicated `.github/workflows/
      check_context_map.yaml` (modeled on `check_versions.yaml`, no path filter →
      catches doc drift anywhere; runs `npm install` for js-yaml). Intended as a
      required check (branch-protection registration is a manual follow-up).
- [x] Seeded frontmatter on the 7 structural anchors + 11 review-dimension
      anchors + the filing spec + 5 representative leaves (24 docs). Committed
      `docs/llms.txt`. Verified `--check` passes when current and fails on drift.

**Structural anchor seed (`alwaysApply: true`):**
`client/architecture.md` (architecture, client), `server/architecture.md`
(architecture, server), `proto_conventions.md` (conventions),
`testing/architecture.md` (testing), `server/observability.md` (observability,
server), `users/priorities.md` (product), `users/personas.md` (product).

**Dimension anchors (`alwaysApply: true`, Decision 9):** all 11
`docs/reviews/*_review_prompt.md` — `lens: [<dimension>]`, loaders follow each
prompt's "Areas to Review" section.

(`CLAUDE.md` is already auto-loaded into every session, so it is intentionally
*not* in the tree.)

**Validates:** the gate works and the generated index is correct before we touch
the remaining ~87 docs.

### Phase 2: Backfill frontmatter across the durable corpus (✅ done)

**Goal:** every durable doc carries a valid `context:` block; the gate flips to
fully enforcing.

**Tasks (✅ done):**
- [x] Enumerated the 87 remaining durable docs (everything under `docs/` minus
      the exclude globs, minus the 24 seeded in Phase 1).
- [x] Agent-assisted pass: 8 area-scoped agents (client UI ×2, server backend,
      workflows/testing/design, product/users, top-level domain ×2, infra/ops),
      each following `/tmp/backfill_spec.md` + `docs/context_map.md`, reading each
      doc and verifying `globs` against the real filesystem before writing.
- [x] Hand-reviewed the generated `docs/llms.txt` (all 111 entries) for
      `globs`/`skills`/`lens` quality; fixed one invalid block (`community_health`
      had a bare `32` YAML-coerced to int — the gate caught it).
- [x] Regenerated + committed `docs/llms.txt` (111 tagged, 0 untagged); flipped
      `ENFORCE_FULL_COVERAGE = true`; gate green under full enforcement.
- [x] Added the **"Finding Documentation Context"** section to the project
      `CLAUDE.md` (after `## Architecture`).

**Validates:** the full set is routable; the index reflects the whole corpus;
the whole project (not just two skills) is pointed at it.

**Surfaced (out of scope — follow-up):** the backfill agents found stale code
paths in several doc *bodies* (`provenance` cites `server/impact`, `daily` cites
`server/services/daily`, `esm`/`semantic_search`/`push_notifications` cite moved
or removed files, `sharing` cites a deleted `sidebar.dart`, `stock_imagery` a
renamed proto). The `globs` use the real paths; the prose drift wants a separate
doc-refresh pass.

### Phase 3: Rewire `ripls-audit` (✅ done)

**Goal:** the audit loads conventions/architecture/infra/security/observability/
testing context selectively, drawn from the full set.

**Tasks (✅ done):**
- [x] Replaced the hardcoded 7-doc list in `ripls-audit/SKILL.md` with the
      map-driven protocol: determine scope from the target plan/diff → read
      `docs/llms.txt` → load anchors via the subset rule (`lens ⊆` base
      `{architecture, conventions}` + side + engaged dimensions) → apply each
      engaged dimension's review-prompt **"Areas to Review"** → glob/trigger
      leaves whose `skills` include `audit` → audit.
- [x] Infra/security now in purview via the dimension anchors plus infra/security
      leaf docs (the original gap).
- [x] Trace-validated against the live map: a server-auth change now surfaces
      `registration_and_login`, `secret-rotation`, and `apple_sign_in_issues`
      (security/infra) that the old list never loaded; a Flutter change excludes
      `server/architecture` via the subset rule. (A live skill smoke-test on a
      real plan is available on request.)

**Validates:** audits stop loading irrelevant docs and start covering infra/
security.

### Phase 4: Rewire `ripls-issue` (✅ done)

**Goal:** issue plans auto-load product + architecture + touched-subsystem
context with no manual doc args.

**Tasks (✅ done):**
- [x] Added **Phase 1.6 — Load documentation context** to `ripls-issue/SKILL.md`:
      read `docs/llms.txt` → load anchors for `{architecture, conventions,
      product}` + side → as Phase 2 research surfaces paths, load glob/trigger
      leaves whose `skills` include `issue` → skim each engaged dimension's
      review-prompt **"Areas to Review"** so the plan designs for them. Manual
      context-doc args (Phase 1.5) stay additive.
- [x] The plan template's **Context docs** header now records both input and
      auto-selected docs (transparency).
- [x] Selection trace-validated against the map (a live smoke-test on a real
      issue is available on request).

**Validates:** plans are grounded by default; the selection is visible and
reviewable.

### Phase 5: Extend to `ripls-triage` / `ripls-reviews` (✅ done)

**Goal:** the other doc-driven skills also use the map.

**Tasks (✅ done):**
- [x] `ripls-triage`: kept the intentional breadth read of all product +
      review-dimension docs (those are the prioritization frameworks), and added
      a **per-issue subsystem context** step — consult `docs/llms.txt` to pull
      the subsystem/feature docs whose `globs`/`triggers` match a specific issue,
      grounding severity/effort estimates in the documented design.
- [x] `ripls-reviews`: each per-dimension review agent now consults `docs/llms.txt`
      and reads the subsystem docs matching its dimension before reviewing, so
      findings are grounded in the intended design (not just the code).

**Validates:** one shared mechanism now serves all four doc-driven skills.

## Deferred to follow-up issues

- **Glob-vs-filesystem validation** (hard error when a doc's `globs` match zero
  paths) — once the corpus is clean. File when Phase 2 lands.
- **Publish `docs/llms.txt` at the web root** (`ripls.app/llms.txt`) if we want
  external agents to consume it — separate from this internal-tooling change.

## Related Issues

- #1334 — the per-package README / file-comment enforcement; established the
  CI-gate-for-docs precedent and the finest documentation tier this design
  deliberately sits above.

## Appendix: Drafted CLAUDE.md section

To be added to the project `CLAUDE.md` as the last task of Phase 2 (after
`docs/llms.txt` exists and the gate is green). Placement: a new top-level
section after `## Architecture`, since it is orientation that applies to all
work.

> ## Finding Documentation Context
>
> Durable guidance docs under `docs/` are indexed in **`docs/llms.txt`** — a
> generated, hierarchical map of every architecture, subsystem, infrastructure,
> workflow, convention, and product doc, each tagged with the code paths
> (`globs`) it governs. **Before implementing, auditing, or planning any
> non-trivial change, consult `docs/llms.txt` and read the docs relevant to the
> area you're touching** — don't work from the code alone, and don't read every
> doc.
>
> How to select what to read:
>
> 1. **Always read the anchors** for the side you're working on — the docs
>    marked ⚓ in the map: `client/architecture.md` + `proto_conventions.md` for
>    app work; `server/architecture.md` + `proto_conventions.md` for server work.
> 2. **Match by code path.** Each indexed doc lists `globs`. Read the docs whose
>    globs match the files your change touches (e.g. editing `app/lib/chat/**` →
>    read `client/chat.md` and `realtime_updates.md`).
> 3. **Match by topic.** For cross-cutting work without a clean code-path mapping
>    (product, security, infra), match the doc `description`/`triggers` in the
>    map to your task.
> 4. **Apply the engaged review dimensions.** For each engineering dimension your
>    change touches (security, error handling, performance, accessibility, …),
>    read its `docs/reviews/<dimension>_review_prompt.md` and follow that prompt's
>    **"Areas to Review"** section as the checklist — skip the runner scaffolding.
>
> When you add or substantially change a durable doc, add/update its `context:`
> frontmatter block (schema in `docs/context_map.md`). The CI gate
> `npm run lint:context-map` (part of `npm run lint`) fails if a durable doc is
> missing a valid block or if `docs/llms.txt` is stale — regenerate with
> `npm run generate:context-map`. Excluded from the map: `docs/issues/`,
> `docs/ai/`, `docs/release/notes/`, `docs/crashlytics/`, and eval/report
> outputs.
>
> The `ripls-issue` and `ripls-audit` skills consume this same map
> programmatically; this section is the same protocol for everyone else.
