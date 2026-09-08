---
name: ripls-doc-freshness
description: Verify that a durable guidance doc still semantically matches the code it describes — structure, API surface, data model, control flow, named symbols, referenced paths, and stated strategy — then apply minimal fixes and advance the doc's freshness stamp. Use when the user types /ripls-doc-freshness, asks to "check if the docs are stale / out of date", "verify docs match the code", "refresh the docs for <area>", "is this doc still accurate", or to run the doc-drift guard (#2420). Takes one or more doc paths, `--sweep` (all durable docs), or `--base <ref>` (docs whose governed code changed).
---

# Doc Freshness Skill

Verify that a durable guidance doc under `docs/` still tells the truth about the
code it describes, fix the drift it has accumulated, and record what commit it was
verified against. The **unit of work is one doc**; sweeps and the PR-time ratchet
are just this same per-doc verification fanned out over a set.

This is the **semantic** guard (#2420). The shallow "do the referenced paths
exist" check is one cheap finding class folded in — not the point. The point is
that a doc which says "five-state machine," "this RPC returns X," or "the package
lives at Y" still matches reality.

> **Why this matters.** These docs are machine inputs now: `ripls-issue`,
> `ripls-audit`, and the `CLAUDE.md` "Finding Documentation Context" protocol read
> them to plan and audit. A confidently-wrong doc misleads the agent, not just a
> human — and the agent has no independent sense that it's stale.

## Deterministic helper

`scripts/doc_freshness.js` (a sibling of `scripts/gen_context_map.js`, sharing its
durable-doc enumeration and frontmatter parsing) owns the mechanical parts. Use it
— do not re-derive these by hand:

```bash
node scripts/doc_freshness.js list                       # durable docs + stamp state
node scripts/doc_freshness.js governs <doc>              # the code a doc governs + what resolves
node scripts/doc_freshness.js check-paths <doc> [--all]  # inline code paths that DON'T resolve (#2403 lane)
node scripts/doc_freshness.js affected --base <ref>      # docs whose governed code changed since <ref>
node scripts/doc_freshness.js affected --changed <f>...  # same, from an explicit file list
node scripts/doc_freshness.js stamp <doc> --commit <sha> [--date YYYY-MM-DD]
```

`governs` resolves the doc's `context: globs` plus the code paths cited inline in
its body — that union is the **code surface** to read when judging the doc.

## Input / modes

- **Single (default):** one or more doc paths → verify each.
  `/ripls-doc-freshness docs/provenance.md`
- **Sweep (`--sweep`):** every durable doc (Phase 3 of #2420). Expensive — fan out
  one agent per doc; see "Sweep" below.
- **Ratchet (`--base <ref>`):** the docs whose governed code changed since `<ref>`
  (`node scripts/doc_freshness.js affected --base <ref>`). This is what the
  nightly `claude_refresh_docs.yaml` workflow runs; `<ref>` is the
  `doc-freshness-verified` tag (the main SHA of the last verified run).

## Per-doc workflow

For each target doc:

### 1. Gather the code surface
- Read the doc in full.
- Run `node scripts/doc_freshness.js governs <doc>` to get its `context: globs`
  and inline-cited paths. Run `check-paths <doc>` for the deterministic
  unresolved-path findings (the #2403 lane).
- Read that code — the files under the globs and the cited paths. This is the
  ground truth you check the prose against. If the doc describes a subsystem, read
  enough of it to judge the specific claims (entry points, the types/enums/state
  machines it names, the RPCs it documents), not every line.

### 2. Verify against the rubric
Go through the doc's claims and check each against the code you just read, across
these dimensions:

- **Structure** — packages, layers, files, and directories the doc names. Do they
  exist where it says, organized how it says?
- **API surface** — RPCs, request/response shapes, public function/method
  signatures. Do the names, parameters, and return shapes match?
- **Data model** — proto messages, fields, enums, and their values; state-machine
  states and transitions. Is the set/shape current?
- **Control flow** — lifecycles, ordering guarantees, who-calls-what, the
  sequence a doc narrates. Does the code still do it in that order?
- **Named symbols** — every type/function/constant the doc cites by name. Does it
  still exist under that name?
- **Referenced paths exist** — the deterministic `check-paths` findings. A cited
  path that doesn't resolve (and isn't generated/output) is drift.
- **Strategy / design intent** — the "why" and the design decisions. Has a stated
  decision been reversed or superseded in the code?

### 3. Produce findings
Each finding is: **claim → current reality → severity → suggested edit.**

- *claim*: what the doc asserts (quote or tightly paraphrase, with the line/section).
- *current reality*: what the code actually does now, with the file:line proof.
- *severity*: `high` (actively misleading — wrong path/symbol/signature/state),
  `medium` (stale but not dangerous — outdated counts, renamed-but-findable),
  `low` (cosmetic — a moved example).
- *suggested edit*: the concrete minimal change to the doc that makes it true.

If the doc is accurate, the finding set is empty — say so plainly. **Zero findings
is a healthy, common outcome; do not invent drift.**

### 4. Apply the fixes
- Edit the doc in place to make every real finding true. **Minimal edits** —
  correct the path/name/claim; do not rewrite healthy prose around it.
- **No confidence gating** — apply every edit you're confident is correct. The
  human review at merge is the backstop.
- Where a finding has **no determinable fix** (e.g. a referenced thing was deleted
  with no clear replacement, or the right rewrite depends on intent you can't
  infer), **do not guess** — leave it out of the edits and report it as a comment
  for a human to resolve.
- After editing, run `npm run lint:context-map` if you changed any `context:`
  block (most freshness edits are body-only and don't need it).

### 5. Advance the freshness stamp
Always stamp a doc you verified — whether you edited it or confirmed it fresh.
This records the commit the prose was confirmed against and drives the precise
periodic backstop.

```bash
node scripts/doc_freshness.js stamp <doc> --commit "$(git rev-parse --short HEAD)"
```

The stamp lives in a top-level `freshness:` frontmatter block, **separate from
`context:`** — `gen_context_map.js` ignores it, so stamping never makes
`docs/llms.txt` stale or trips `lint:context-map`. Only this skill writes it;
never hand-edit a stamp (that would rubber-stamp unverified prose).

### 6. Report
Per doc, summarize: findings by severity, what you edited, what you left as a
comment (no determinable fix), and the new stamp. End with a one-line verdict
(`fresh` / `N edits applied` / `M comments need a human`).

## Do NOT flag (precision rules)

False "drift" erodes trust and is the make-or-break of this skill. Do **not** flag:

- **Healthy abstraction.** Docs intentionally summarize. "The estimator computes
  savings from item value" is not drift just because the code has more steps.
- **Forward-looking design drafts.** A design doc (e.g. `docs/design/*`) proposes
  names/paths that intentionally don't exist yet. If the doc frames a reference as
  future/proposed, it is not stale. When unsure whether a doc is a draft, prefer a
  low-severity comment over an edit.
- **Generated / output paths.** Anything under `server/gen/`, `app/lib/data/gen/`,
  build/output dirs — absent on a clean checkout by design (the helper already
  excludes these).
- **Illustrative or partial examples.** `server/services/<name>/service.go` as a
  pattern, not a literal file.
- **Vendor-name conventions.** READMEs/docs deliberately use real vendor names
  (Mapbox, Mailgun, FCM, Vertex/Gemini) — that's a project rule, not drift.

When the call is genuinely ambiguous, comment rather than edit.

## Sweep (`--sweep`, Phase 3 of #2420)

The one-time corpus refresh. Fan out **one agent per durable doc**
(`node scripts/doc_freshness.js list`), each running the per-doc workflow above,
following the `ripls-reviews` parallel pattern. Then:

- Collect findings; **batch by domain** and land doc corrections in reviewable
  per-domain PRs, not one mega-PR.
- This pass also fixes every stale path #2403 catalogs — verify those are covered,
  then **close #2403** with a pointer to this work.
- Stamp every doc the sweep verifies (establishes the baseline the ratchet and
  backstop diff against).
- **Log honest coverage** — any doc skipped or capped — so "swept everything"
  isn't a silent overstatement.

## Edge cases

- **Doc has no `context: globs`** (e.g. a pure product/strategy doc): there's no
  code surface to diff structurally — verify only the claims that *do* reference
  code (via `governs`'s inline paths) and the strategy/intent dimension; still
  stamp it.
- **A path resolves but the symbol moved within it:** that's exactly the semantic
  drift the deterministic lane misses — flag it from your reading of the code.
- **The skill can't read the governed code** (deleted wholesale): high-severity
  finding; the subsystem the doc describes may be gone — escalate as a comment,
  don't fabricate a rewrite.
- **`git` unavailable / detached** in a stamp context: stamp with whatever
  `git rev-parse --short HEAD` returns; if truly unavailable, note it and skip the
  stamp rather than writing a bogus one.
