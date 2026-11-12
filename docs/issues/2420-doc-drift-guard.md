# Issue #2420: Doc-drift guard: PR-time check that touched docs still match the code they reference

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2420
**Filed:** 2026-06-08 | **Last activity:** 2026-06-08
**Labels:** documentation | **Assignees:** unassigned
**Context docs:** `docs/context_map.md`, `docs/llms.txt`, `scripts/gen_context_map.js`, `.github/workflows/check_context_map.yaml`, `docs/coding_agent_cheat_sheet.md` + the `.github/workflows/claude_*.yaml` fleet (the CI-auto-commit precedent the ratchet reuses), `docs/reviews/{code_quality,testing,tech_debt}_review_prompt.md` (engaged dimensions). This change is repo tooling + a doc-driven skill, not `server/**` / `app/lib/**` product code, so the server/client architecture anchors don't apply; the `conventions` (CLAUDE.md skill/TODO/file-size rules) and the context-map spec are the governing docs.

> **Design direction (from issue owner, during planning).** The first cut of this
> plan proposed a deterministic static check ("do the referenced paths still
> resolve on disk?"). That was rejected as low-value: *"just checking that paths
> are alive is not that valuable. Rather, we want the prose docs that describe
> structure, strategy, API, design, etc. to actually match the code they
> describe — much better as an LLM skill."* The agreed shape is: **design a
> per-doc verification skill → test it on a couple docs → run a full one-time
> sweep with independent agents (expensive, up-front) → build a ratchet that
> re-runs the skill for the docs whose referenced code changes (and vice versa).**
> Path-existence is **not** the check — it is folded in as one cheap finding
> class, so the same sweep also closes **#2403** (the stale-path cleanup).
>
> **The ratchet auto-applies the fix.** The PR author should have to do
> *nothing*: when a PR changes code a doc describes, CI runs the skill in
> **apply mode**, commits the refreshed doc onto the PR branch, and comments —
> riding along for review with the code, never auto-merged. This mirrors the
> existing `.github/workflows/claude_*.yaml` fleet (Claude headless in CI,
> commits to the PR head, self-trigger-filtered, no Claude branding, human
> merges). A local pre-push hook was considered and rejected as the primary
> path (see Open Questions).

## Problem Statement

The documentation context map (#2398 / PR #2406) made docs first-class inputs to
coding agents: `ripls-issue`, `ripls-audit`, and the `CLAUDE.md` "Finding
Documentation Context" protocol all read `docs/llms.txt` and the docs it routes to
before planning or auditing. Its CI gate (`npm run lint:context-map`) keeps
**routing** honest — it fails on a malformed `context:` block, an unknown `lens`,
or a stale `docs/llms.txt`, so the frontmatter `globs` reliably point at the real
code.

But the gate validates only the frontmatter. **Nothing checks whether the prose
still tells the truth about the code it describes.** A doc body can keep
describing a five-state machine that now has four states, an RPC shape that has
changed, a data model that has been restructured, or a strategy that has been
superseded — and every path it cites can still resolve while the *description* is
wrong. The shallow failure mode (a renamed file) is the easy 10%; the expensive
failure mode is semantic drift in docs that exist precisely to convey structure,
API surface, control flow, and design intent.

Because agents now read these docs to plan and audit, a confidently-wrong
description misleads the agent, not just a human — and unlike a human, the agent
has no independent sense that the doc is stale. The cost of *semantic* drift went
up the moment docs became machine inputs.

This issue asks for the **ongoing guard**: a recurring mechanism that verifies
the docs relevant to a change still *correctly represent the state of the code
they reference* — semantically, not just syntactically.

Two scoping facts from the issue and owner:
- This is **item 2 of 2**. Item 1 (#2403) is the one-time cleanup of
  *already-present* drift. The owner has folded #2403 **into this work**: the
  per-doc skill includes a referenced-path existence check, so the one-time sweep
  fixes both semantic drift and the #2403 stale paths, and #2403 closes when the
  sweep lands.
- A static path-resolver was explicitly considered and rejected as the *check*.
  It survives only as a trivial sub-case the skill subsumes, and as the basis for
  the ratchet's change-detection.

## Context from Comments

**The GitHub issue has no comments** — it is a self-contained spec written as a
follow-up from PR #2406 review, and the substantive design direction came from
the issue owner *during this planning session* (quoted in the banner above and
threaded through the recommendation). The issue itself flags two things as
**"design TBD"**, both reframed by the owner's direction:

1. *Extend the `context:` block vs. body scanning* → moot for the **check**
   (it's an LLM reading the body + code), but the `context:` `globs` become the
   **dependency graph the ratchet runs on**.
2. *How to scope the check* ("cheap, targeted… scoped to the docs a PR touches")
   → satisfied by the ratchet: re-verify only the docs whose governed code the PR
   changed.

## Why this is the right tool, and what it builds on

- **Semantic match is a judgment, not a `stat`.** "Does this paragraph still
  describe how `StartLoan` transitions state?" cannot be answered by a regex. It
  needs a reader that understands both the prose and the code — an LLM skill.
- **The dependency graph already exists.** Every durable doc's `context: globs`
  declares the code it governs (#2398). `gen_context_map.js` already parses all of
  them. Inverting that map (changed file → docs whose globs match) gives the
  ratchet a precise, repo-native trigger for free — *cheap and targeted* (the
  issue's ask) while the check stays *semantic* (the owner's ask).
- **Skill plumbing exists.** The repo already runs doc-driven skills
  (`ripls-issue`, `ripls-audit`, `ripls-reviews`) that read `docs/llms.txt`, and
  multi-agent fan-out is an established pattern (`ripls-reviews` runs every review
  prompt in parallel). The full sweep is the same fan-out shape: one agent per
  doc.
- **CI auto-commit-to-PR is already a solved, governed pattern.** The
  `.github/workflows/claude_*.yaml` fleet (`coding_agent_cheat_sheet.md`) runs
  Claude headless in CI and pushes commits to PR head branches —
  `claude_fix_ci_failure` and `claude_address_pr_feedback` do exactly "diagnose,
  edit files, commit, comment." They already solve the hard parts the auto-apply
  ratchet needs: self-trigger filtering (no infinite loops), per-PR concurrency
  serialization, a no-op guard when the branch advanced past the targeted SHA, a
  deliverable assertion (commit *or* comment within 30 min) with an uploaded
  execution-log artifact, and the hard rules (no Claude branding, never merge,
  stay scoped). The doc-refresh ratchet is a new sibling, not new infrastructure.
- **`#2403` becomes a byproduct.** The skill's cheap finding class —
  *referenced path/file/symbol no longer exists* — catches exactly the nine
  drifts #2403 catalogs (verified: `provenance.md` cites `server/impact/...`; the
  real package is `server/impact_metrics/...`). Fixing them is part of the sweep.

## Potential Solutions

The approach (LLM skill, not a static gate) is settled. The genuine open design
fork is the **ratchet** — how the ongoing guard decides which docs to re-verify
and when. Three viable architectures:

### Option A: PR-time reverse-index ratchet, **auto-applying** (recommended)
- **Approach:** A new `claude_refresh_docs.yaml` workflow (sibling to the
  `claude_*.yaml` fleet) fires on PR `opened`/`synchronize`. It computes the PR's
  changed files (`git diff --name-only` vs. merge base), loads every doc's
  `context: globs` (reusing `gen_context_map.js`'s parser), and for each changed
  file finds the docs whose globs match — *plus* docs the PR edits directly. For
  that targeted doc set it runs the per-doc freshness skill in **apply mode**: the
  skill edits the docs to match the code, CI commits the refreshed docs onto the
  PR head branch (no Claude branding, following the fleet's commit conventions),
  and posts a summary comment. The author does nothing; the doc fix is reviewed
  and merged *with* the code. Never auto-merges — the human merge is the backstop
  for an LLM-applied edit.
- **Pros:** Zero author effort — the headline requirement. Incremental and
  cheap-per-PR (only re-verifies docs whose governed code moved). Bidirectional
  (code→doc and doc→doc). Reuses the proven `claude_*.yaml` auto-commit machinery
  (loop-avoidance, concurrency, no-op guards, deliverable assertion all solved).
  Keeps docs current *continuously* instead of decaying to the next manual sweep.
- **Cons:** LLM-applied edits land in the PR diff (mitigated: human review before
  merge; high-precision skill from Phase 2; the deterministic path-fix class is
  always-safe). Per-PR token cost on doc-adjacent changes. Auto-pushing to PR
  heads needs write-capable CI auth (the repo's GitHub App already does this for
  the fleet) and is awkward for fork PRs (a non-issue for this single-org repo,
  same as the existing workflows).
- **Effort:** Medium.

### Option B: Scheduled freshness sweep keyed off recent code churn
- **Approach:** No per-PR step. A scheduled job (cron / routine) periodically
  diffs the last N days of merged code, maps changed paths → docs via globs, and
  runs the skill on the affected docs, filing/Updating a "doc may be stale" issue
  or PR per finding.
- **Pros:** Zero PR latency; batches token cost; easy to make a low-priority
  background process. Good for catching drift introduced by merges that didn't
  touch the doc.
- **Cons:** Not *PR-time* (the issue title literally asks for PR-time); drift
  lands on `main` before it's caught; weaker coupling between the change and the
  re-verification. Loses the "the person who moved the code is the person who
  should fix the doc" property.
- **Effort:** Medium.

### Option C: Freshness-stamp bookkeeping — **adopted into the core, combined with A**
- **Approach:** Each doc carries a `verified` marker (a `freshness:` frontmatter
  block) recording the commit its prose was last confirmed against. Because the
  skill *already* knows exactly which doc it is verifying, advancing the stamp is
  nearly free — so rather than treat this as a separate, heavier alternative to A,
  we **fold it into A**: the skill stamps every doc it verifies (the sweep stamps
  the whole corpus; the ratchet advances the stamp on every doc it touches). The
  stamp then lets any check ask the precise question *"has this doc's globbed code
  changed since the doc was last verified?"* — catching drift that a pure per-PR
  diff misses (e.g. code merged through a PR that never re-triggered the doc).
- **Pros:** Most precise "is this doc behind the code" signal; makes freshness an
  explicit, auditable property; powers a precise periodic backstop (diff each
  doc's globs against *its own* stamp, not a time window). The skill drives the
  stamp forward as a byproduct of work it's already doing.
- **Cons:** A small amount of bookkeeping — a new frontmatter block, and a
  one-line stamp bump even when a doc is verified *fresh* (acceptable: the ratchet
  is committing to the branch anyway, and the recorded "verified at commit X" is
  the whole point). Guard against rubber-stamping by only advancing the stamp on
  an actual skill run, never by hand.
- **Effort:** Small once A exists (the owner's call — "the skill will know when it
  is touching a doc, let's do them now").

## Recommendation

Follow the owner's sequence, with **Option A** as the ongoing ratchet and the
per-doc skill at the center of all of it:

1. **Design the per-doc freshness skill** as the reusable unit of work. Its
   contract: given **one doc**, read its body *and* the code it governs (from its
   `context: globs` plus paths/symbols cited inline), and judge whether the prose
   still accurately describes that code across a fixed rubric — **structure**
   (packages/layers/files described), **API surface** (RPCs, request/response
   shapes, function signatures), **data model** (proto/message fields, enums,
   state machines), **control flow** (lifecycles, ordering, who-calls-what),
   **named symbols** (types/functions cited), **referenced paths exist** (the
   cheap #2403 class), and **design/strategy claims** (does the stated intent
   still match reality). It emits structured findings — *claim → current reality →
   severity → suggested edit* — or "fresh," and can propose the doc edit so
   staying current is cheap. Critically, it must **not flag healthy abstraction**
   (docs intentionally summarize) or **forward-looking design drafts** (e.g.
   `docs/design/unified-create.md` proposes not-yet-built names) as drift — that
   precision discipline is the make-or-break of the whole feature. On every run it
   **advances the doc's freshness stamp** (the commit it was verified against) —
   nearly free, since the skill already knows which doc and commit it is looking at.

2. **Validate on a few docs** before spending on a sweep. Hand-grade the skill on
   a diverse, deliberately-chosen set: a known-drifted doc (`provenance.md`), a
   structural anchor (`server/architecture.md`), an API/proto doc, a
   strategy/product doc (`users/priorities.md`), and a believed-fresh doc — to
   measure precision (false "drift") and recall (missed drift) and tune the prompt.

3. **One-time full sweep with independent agents** — the expensive, up-front pass.
   Fan out one agent per durable doc (~90, the same set `gen_context_map.js`
   enumerates), each running the per-doc skill, producing findings + proposed
   edits batched by domain for review and landing. This brings the whole corpus
   current **and fixes #2403's stale paths in the same pass** (then close #2403).

4. **Build the ratchet (Option A + freshness stamps), auto-applying** for ongoing
   incremental freshness: at PR time, re-run the skill only on docs whose globbed
   code the PR changed (or the doc itself changed), **commit the refreshed docs
   straight onto the PR branch** (author does nothing; reviewed with the code),
   and **advance each touched doc's freshness stamp**. Every edit the skill
   produces is auto-applied — there is no confidence gate; the human review at
   merge is the backstop, and where the skill genuinely can't determine a fix
   (e.g. a referenced thing was deleted with no clear replacement) it leaves a
   comment instead of guessing. The stamps make freshness an explicit, auditable
   property and enable a precise periodic backstop. Modeled on the `claude_*.yaml`
   fleet; never auto-merges.

This is deliberately heavyweight — but it delivers what a path-checker cannot:
documentation that stays *semantically* correct, verified against the code by a
reader that understands both, and kept fresh by a targeted, change-driven loop.

## Open Questions

- **Delivery — auto-apply vs. advisory (decided: auto-apply, no confidence gate).**
  The ratchet **commits the refreshed docs onto the PR branch** so the author does
  nothing; the doc fix is reviewed and merged with the code, never auto-merged.
  Rationale and precedent: the `claude_*.yaml` fleet already auto-commits agent
  edits to PR heads. **Decided (owner): no confidence gating** — every edit the
  skill produces is auto-applied; the human review at merge is the backstop. The
  only non-edit path is when the skill *cannot determine* a fix, where it comments
  instead of guessing. Sub-question — **does it ever hard-fail a merge?**
  Recommendation: **no** for the LLM-applied semantic edits (a hard block on a
  non-deterministic verdict trains people to disable it); the deterministic
  **path-missing** class (the #2403 lane) may post a blocking status only when it
  can't auto-fix.
- **CI auto-commit vs. local hook (the owner's "trigger the skill locally?"
  alternative).** Recommendation: **CI auto-commit is the primary**; a local hook
  is a poor fit for "author does nothing." A git `pre-push` hook runs *before* the
  push lands, so it cannot inject the doc commit into the in-flight push — it would
  have to abort and tell the author to commit + re-push (that *is* doing
  something), and it would require every dev to have the `claude` CLI + auth
  locally and would stall every push on an LLM call. Keep a local pre-push hook
  only as an **opt-in convenience** for authors who want a fast "your docs will be
  refreshed" heads-up before CI does it — never as the mechanism the freshness
  guarantee depends on.
- **Where does the skill live and how is it invoked?** — Recommendation: a new
  `ripls-doc-freshness` skill under `.claude/skills/`, with the per-doc verifier
  as its core and a sweep/ratchet driver around it; the `claude_refresh_docs.yaml`
  workflow invokes it headless in apply mode. Register it in the context map
  `skills` vocabulary if docs should be routed to it. Reuse `gen_context_map.js`'s
  doc enumeration + glob parsing rather than re-implementing.
- **Freshness stamps — included now (decided). Where does the stamp live?** —
  The owner pulled stamps into the core (the skill already knows which doc/commit
  it's on, so stamping is nearly free). Open sub-question is *where* the stamp
  lives. Recommendation: a **dedicated top-level `freshness:` frontmatter block**,
  *separate* from the `context:` routing block —
  `freshness: { verified_commit: <sha>, verified_on: <date> }`. Keeping it outside
  `context:` means `gen_context_map.js` ignores it (it only reads `doc.context`),
  so the routing gate needs **no** change and the mutable stamp never entangles
  the routing metadata; the stamp stays with the doc (visible in the diff, no
  sidecar merge-conflict hotspot). Document the new block in `docs/context_map.md`.
  Only the skill writes it — never by hand — to avoid rubber-stamping.
- **Precision guardrails — how do we keep false-drift low?** — Recommendation:
  bake explicit "do NOT flag" guidance into the skill prompt (intentional
  abstraction, forward-looking drafts, illustrative/partial examples), carry a
  doc-level opt-out for genuine design drafts (a frontmatter facet the
  context-map validator learns), and treat Phase 2's hand-grading as the gate on
  whether the skill is good enough to sweep with.
- **Sweep mechanics — cost and batching?** — Recommendation: run the sweep as a
  parallel agent fan-out (the `ripls-reviews` pattern), batch findings by domain,
  and land doc fixes in reviewable per-domain PRs rather than one mega-PR. Log
  any docs skipped/capped so coverage is honest.
- **Does the semantic sweep fully supersede #2403, or run alongside it?** —
  Recommendation: **supersede.** The skill's path-existence finding class covers
  100% of #2403's catalog; do those fixes inside the sweep and close #2403 with a
  pointer here. Don't run a separate path-only pass.

## Implementation Plan

### Phase 1: Design and build the per-doc freshness skill (with freshness stamps)
**Goal:** A single, reusable skill that verifies **one** doc against the code it
governs, emits structured findings (incl. the cheap path-existence class), and
records when/against-what-commit it verified.
**Tasks:**
- [x] Define the verification rubric (structure / API / data model / control flow
      / named symbols / referenced-paths-exist / strategy) and the finding schema
      (`claim → current reality → severity → suggested edit`), plus the explicit
      "do NOT flag" precision rules (abstraction, forward-looking drafts, examples).
      → `.claude/skills/ripls-doc-freshness/SKILL.md`.
- [x] Author `.claude/skills/ripls-doc-freshness/` with the per-doc verifier as
      its core: input = one doc; it reads the body, resolves the doc's
      `context: globs` + inline-cited paths/symbols, reads that code, and produces
      findings or "fresh." → `SKILL.md` (single / `--sweep` / `--base` modes).
- [x] Reuse `gen_context_map.js`'s doc enumeration, `splitFrontmatter`, and glob
      parsing — done by making `gen_context_map.js` importable (`require.main`
      guard + `module.exports`) and `require`-ing it from
      `scripts/doc_freshness.js`, so both share one definition of "durable doc"
      and frontmatter parsing (no duplication). A minimal in-house glob matcher
      avoids adding a dependency.
- [x] Include referenced-path existence as one deterministic finding class (the
      #2403 lane) → `doc_freshness.js check-paths` (verified it flags the live
      `provenance.md` drift).
- [x] **Freshness stamp:** top-level `freshness:` block (`verified_commit`,
      `verified_on`), documented in `docs/context_map.md`; written by
      `doc_freshness.js stamp` and the skill on every run. Confirmed
      `gen_context_map.js --check` ignores it (stamping a real doc kept the gate
      green) with a `node --test` cross-check in `scripts/doc_freshness.test.js`.
**Validates:** That a single doc can be verified end-to-end, yields actionable,
correctly-scoped findings, and carries a machine-readable record of what commit it
was verified against. ✅ **Done** — 11 unit tests pass; CLI proven against the
real corpus (`check-paths`, `affected`, `stamp`); routing gate stays green.

### Phase 2: Calibrate on a representative doc set
**Goal:** Prove the skill's precision/recall before spending on a full sweep.
**Tasks:**
- [x] Ran the skill live on `provenance.md` (known drift) as the pilot — caught
      the path drift **and** two high-severity semantic finds a path-checker
      can't (the `ProvenanceSource` enum was 6→4 with modality-in-name; the
      `MethodologyDocForProvenance` server mechanism was removed). Fixed + stamped.
- [~] Broader hand-grade was effectively absorbed into the Phase-3 sweep's
      spot-checks (anchor docs, the highest-edit docs, and all 15 needs-human
      docs reviewed) rather than run as a separate pre-sweep pass — the pilot gave
      enough confidence to proceed (owner: "sweep all docs").
- [x] Precision held: edits surgical and file:line-grounded; zero invented drift
      observed in spot-checks; needs-human discipline (comment, don't guess) worked.
**Validates:** The skill is trustworthy enough to fan out — confirmed by the pilot
plus the sweep's spot-checks.

### Phase 3: One-time full sweep with independent agents (closes #2403)
**Goal:** Bring the entire durable-doc corpus current — semantically and on paths.
**Tasks:**
- [x] Fan out one agent per domain batch (27 batches, ≤5 docs each) over the
      `gen_context_map.js` doc set, each running the Phase-1 skill — ran as a
      resumable `Workflow` at commit `9fdddc4d0`. **112/112 docs, 27/27 batches,
      none skipped.** Report: `docs/issues/2420-sweep-report.md`.
- [x] Batch by domain; committed per-domain (reviewable diffs, not one mega-PR).
      Coverage logged in the report (63 edited, 34 fresh, 15 needs-human; 279
      edits, 134 high-severity findings).
- [x] #2403 stale paths: the sweep fixed them everywhere; the last holdout,
      `daily.md` (whole-subsystem `daily`→portfolio-inbox rename), was resolved in
      the needs-human pass — salvaged into the new `docs/server/portfolio.md` and
      deleted. **#2403 is now fully closeable.**
- [x] Needs-human resolution (15 docs): decisions in
      `docs/issues/2420-needs-human-decisions.md`; executed as a 15-agent workflow
      — 13 rewrites + 2 replacements (`daily.md`→`server/portfolio.md`,
      `client/workflow.md`→`client/manage_menus.md`). All re-verified against code
      and stamped; gate green (112 docs).
- [x] **Stamped the whole corpus:** all 112 docs carry a `freshness:` block at
      `9fdddc4d0` — the baseline the ratchet/backstop diff against.
- [x] Ran `npm run generate:context-map` + `lint:context-map` after edits (no
      `context:` block changed → `docs/llms.txt` unchanged; gate green) and
      `npm run test:scripts` (green).
**Validates:** The whole corpus is current at a known commit, every doc carries a
freshness baseline, and the semantic skill's value is demonstrated at scale. ✅
**Done** — precision held on spot-checks (surgical, file:line-grounded edits; the
15 needs-human docs correctly applied only unambiguous partial fixes).

### Phase 4: The ongoing ratchet (Option A — auto-applying reverse-index)

**Goal:** Keep docs incrementally fresh with **zero author effort** — when a PR
changes code a doc describes, CI refreshes the doc onto the PR branch.

**Tasks:**
- [x] Build the reverse index: changed files (`git diff --name-only` vs. merge
      base; `fetch-depth: 0` per `gitleaks-pr.yaml`) → docs whose `context: globs`
      match, plus docs the PR edits directly. Reuse `gen_context_map.js`'s glob
      parser.
- [x] Add `.github/workflows/claude_refresh_docs.yaml`, sibling to the
      `claude_*.yaml` fleet, firing on PR `opened`/`synchronize`. It runs the
      freshness skill in **apply mode** on the targeted doc set, **commits the
      refreshed docs (and their advanced `freshness:` stamps) onto the PR head
      branch** (fleet commit conventions: no Claude branding, never merge), and
      posts a summary comment.
- [x] Reuse the fleet's solved mechanics: self-trigger filtering + a "doc-only
      commit" guard so the auto-commit can't re-trigger the workflow into a loop;
      per-PR concurrency serialization; the no-op-if-branch-advanced guard; the
      deliverable assertion + execution-log artifact.
- [x] **No confidence gating** (owner decision): auto-apply every edit the skill
      produces; the human review at merge is the backstop. The only non-edit path
      is a finding the skill can't resolve into a concrete fix (e.g. a referenced
      thing deleted with no clear replacement) — that becomes a comment, not a
      guess.
- [x] **Lock down the auto-commit security surface** (this workflow runs an LLM
      over PR-changed code — *attacker-controllable on a contributor PR* — and
      pushes its output with a write token). Mirror the fleet's controls and
      tighten: trigger on `pull_request` (**never** `pull_request_target`);
      least-privilege `permissions` (`contents: write`, `pull-requests: write`,
      `id-token: write`); restrict the agent's `--allowedTools` to a **docs-only**
      surface (`Edit`/`Write`/`Read`/`Glob`/`Grep` + `Bash(git:*)`/`Bash(gh:*)` +
      `npm run lint:context-map`) so a prompt-injection in changed code can't make
      it touch `server/**`/`app/**` or run arbitrary commands; commit via
      `CLAUDE_WORKFLOW_PAT` (as the fleet does, so the pushed commit re-triggers
      required checks that `GITHUB_TOKEN` pushes would suppress); never auto-merge
      (human review is the backstop). Note the broader-than-fleet exposure: this
      fires **automatically** (no `@claude` gate) on every doc-adjacent PR, so the
      docs-only scope + no-auto-merge are load-bearing, not nice-to-haves.
- [x] Update `CLAUDE.md` "Finding Documentation Context" and `docs/context_map.md`
      with a "Prose freshness" section, and add `claude_refresh_docs.yaml` to
      `coding_agent_cheat_sheet.md` (+ its `context.globs`) so the fleet doc stays
      complete — itself a nice dogfood of the ratchet.
**Validates:** The end-to-end guard with zero author effort — a PR that
restructures code a doc describes comes back with the doc already refreshed on the
branch; an unrelated PR is untouched; the auto-commit doesn't loop. ✅ **Built**
— `claude_refresh_docs.yaml` added (YAML validated), reverse index via
`doc_freshness.js affected --base`, docs-only `--allowedTools`,
`CLAUDE_WORKFLOW_PAT`, never-merge. **Inert until merged to `main`** (PR-event
workflows run from the base branch) and non-required by default (advisory); watch
its first live runs before relying on it.

### Phase 5 (fast-follow, optional): stamp-driven periodic backstop

**Goal:** Catch drift the per-PR ratchet misses — code that reached `main` through
a PR that never re-triggered the relevant doc (squash merges, doc added at a stale
commit). The freshness stamps from Phases 1/3/4 make this precise; this phase just
adds the sweep that consumes them.

**Tasks:**

- [ ] A scheduled job (cron / routine) computes, per doc, whether any file under
      its `context: globs` changed between the doc's `freshness.verified_commit`
      and `main` HEAD; the stale-suspect set is the backstop worklist.
- [ ] Run the freshness skill on that set (same apply-mode path as the ratchet),
      opening a doc-refresh PR with the results, and advancing the stamps.
- [ ] Only build this if Phase 4 shows real gaps in practice — the stamps exist
      regardless, so this stays a cheap add-on. Measure first.
**Validates:** Whether the per-PR ratchet needs a time-based backstop, and that the
stamp diff precisely identifies docs whose code moved without re-verification.

## Engaged review dimensions (designed-for)

This change is tooling + a skill, not product code; the engaged rubrics:

- **testing** — any deterministic helper extracted for the ratchet (glob
  reverse-index, doc enumeration, path-existence) gets a `node --test` suite
  (`npm run test:scripts`), mirroring `doctor_versions.test.js`. The skill itself
  is calibrated by Phase 2's hand-grading (its "test").
- **code_quality** — share doc-enumeration / `splitFrontmatter` / glob parsing
  with `gen_context_map.js` rather than copy-paste (DRY); named constants, typed
  exit codes, actionable output; keep any script well under the 1,000-line ceiling.
- **tech_debt** — this feature is a *net reducer* of documentation debt (rubric §6
  "Outdated Docs"); the design-draft opt-out must carry a tracking-issue reference
  (consistent with the TODO-needs-an-issue rule) so it can't become a silent mute.
- **error_handling** — the ratchet tolerates docs with no governed code, missing
  merge base, and skill non-determinism without crashing CI; advisory findings
  never hard-fail except the one deterministic path-missing class.
- **observability** — findings name the exact doc, the exact claim, current
  reality, and a suggested fix, matching the existing gate's actionable-failure
  style; sweep logs honest coverage (docs skipped/capped).
- **security** — the auto-apply ratchet is the real surface: an LLM editing files
  and pushing with a write token, triggered automatically by attacker-controllable
  PR content. Controls (detailed in Phase 4): `pull_request` not
  `pull_request_target`, least-privilege permissions, a **docs-only**
  `--allowedTools` scope to contain prompt-injection, `CLAUDE_WORKFLOW_PAT` for
  re-triggering checks, and never-auto-merge so a human gate stands between an
  LLM-applied edit and `main`. No secrets, PII, or auth code is touched by the
  feature itself.
- **dependency / cost** — no new runtime deps (reuses `js-yaml` + the existing
  skill runtime); the new cost is LLM tokens per doc-adjacent PR and per sweep —
  bounded by the reverse-index (only docs whose code changed) and surfaced in the
  sweep's coverage log.

## Related Issues

- **#2403** — the stale-path cleanup. **Absorbed into this work:** the per-doc
  skill includes a path-existence finding class, so the Phase-3 sweep fixes those
  paths and #2403 closes. Do not run a separate path-only pass.
- **#2398 / PR #2406** — the documentation context map foundation. Its
  `context: globs` are the doc↔code dependency graph the Phase-4 ratchet runs on;
  its routing gate is the structural half, this is the semantic half of the same
  "doc honesty" lineage.

---

## Audit notes (`/ripls-audit`)

**Scope.** Infra/tooling + a doc-driven skill — no `server/**` or `app/lib/**`
product code. Engaged dimensions: code_quality, testing, tech_debt,
error_handling, observability, and (the highest-value find) **security** for the
auto-apply CI workflow, plus dependency/cost.

**Findings applied to the plan:**
1. **Security (auto-commit surface) — added.** The first auto-apply draft didn't
   spell out that apply-mode runs an LLM over attacker-controllable PR code and
   pushes with a write token. Added a Phase-4 task and a `security` dimension
   pinning the controls to the real `claude_*.yaml` auth model verified during the
   audit (`contents: write` + `pull-requests: write`, `CLAUDE_WORKFLOW_PAT` for
   re-triggering checks, restricted `--allowedTools`): `pull_request` not
   `pull_request_target`, docs-only tool scope against prompt-injection, never
   auto-merge. Flagged the broader-than-fleet exposure (fires automatically, no
   `@claude` gate).
2. **Dependency/cost dimension — added.** No new runtime deps; token cost bounded
   by the reverse-index and logged.
3. **Convention checks — pass.** `.yaml` workflow extension; npm task-runner
   sub-targets; `node --test` test pattern; escape-hatch carries a tracking-issue
   ref (TODO-hygiene rule); file-size ceiling noted; no Claude branding in
   auto-commits (fleet hard rule).
4. **Markdown lint** — cleared the flagged MD022/MD032 blank-line diagnostics in
   the edited regions.

**Docs consulted:** `docs/context_map.md`, `docs/llms.txt`,
`scripts/gen_context_map.js`, `.github/workflows/check_context_map.yaml`,
`.github/workflows/claude_address_pr_feedback.yaml` +
`.github/workflows/claude_implement_plan.yaml` (auth/permission/allowedTools
model), `docs/coding_agent_cheat_sheet.md`, and the `docs/reviews/`
**Areas to Review** rubrics for `security`, `code_quality`, `testing`, and
`tech_debt`. Anchors `server/architecture.md` / `client/architecture.md` were
deliberately *not* loaded — no product code is in scope.
