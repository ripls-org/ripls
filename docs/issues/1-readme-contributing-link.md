# Issue #1: Link CONTRIBUTING.md from the README

**Source:** https://github.com/ripls-org/ripls/issues/1
**Filed:** 2026-10-23 | **Last activity:** 2026-10-24
**Labels:** none | **Assignees:** unassigned
**Context docs:** none — the change touches only the root `README.md`, which no
durable doc in `docs/llms.txt` governs (no doc's `globs` match `README.md`, and
no engineering-review dimension is engaged by a docs-only edit).

## Problem Statement

The root `README.md` (300 lines) is the landing page for anyone opening the
repository, but it never mentions `CONTRIBUTING.md`. A repo-wide grep finds no
Markdown file that links to it. New contributors therefore miss the
contributor-facing rules that live only in `CONTRIBUTING.md`:

- the DCO `Signed-off-by` requirement, which CI enforces (`## Sign your commits (DCO)`)
- the pre-PR checklist (`npm run generate`, `npm run lint`, tests need Docker)
  (`## Before you open a pull request`)
- conventions (`## Conventions worth knowing up front`)
- how to report security issues (`## Reporting a security issue`)

The issue asks for a short "Contributing" section near the end of the README
that links to `CONTRIBUTING.md`. The request is unambiguous.

## Context from Comments

- The single comment (from `bryanklingner`, MEMBER) repeats "@claude please
  plan this." It adds no new requirements.
- The issue body says it tests the agent dispatch bridge on the public repo
  (#2970 Phase 4) and will be closed afterwards. The change itself is still a
  real improvement and is worth merging on its own merits.

## Potential Solutions

### Option A: Append a `## Contributing` section at the end of the README
- **Approach:** Add a new H2 `## Contributing` after the existing `## Testing`
  section, which is currently the last section (it ends with
  `For server-specific testing details, see [server/README.md](server/README.md).`).
  Write two to four sentences that link to `CONTRIBUTING.md` and name its key
  topics (DCO sign-off, pre-PR checks).
- **Pros:** This matches the issue's request ("near the end"). It is the
  standard GitHub convention and the smallest possible diff. It fits the
  README's flow: setup, then server, then testing, then contributing.
- **Cons:** Readers who skim only the top of the README won't see it. GitHub's
  "Contributing" sidebar link partly makes up for this.
- **Effort:** small

### Option B: Option A plus a one-line pointer near the top
- **Approach:** Do everything in Option A, and also add a sentence under the
  tagline on line 3 (e.g. "Want to contribute? See CONTRIBUTING.md.").
- **Pros:** More visible.
- **Cons:** It goes beyond what the issue asked for and duplicates the link.
- **Effort:** small

### Option C: A "Community" section linking CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, LICENSE
- **Approach:** Add a section at the end that links all four root community
  files (`CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, `LICENSE`).
  The README doesn't currently link any of them.
- **Pros:** Fills a wider discoverability gap in one edit.
- **Cons:** Scope creep beyond the issue. Each extra link is a reviewer
  decision (for example, whether LICENSE needs a README mention when GitHub
  already shows it).
- **Effort:** small

## Recommendation

**Option A.** It does exactly what the issue asks, with the smallest diff.
Linking `CODE_OF_CONDUCT.md` from that same section is a cheap optional
extension (see Open Questions). It is not required for acceptance.

## Open Questions

- **Should the section also link `CODE_OF_CONDUCT.md` / `SECURITY.md`?** —
  Recommendation: no. Keep the scope to `CONTRIBUTING.md` as the issue
  requests. `CONTRIBUTING.md` already has a `## Reporting a security issue`
  section, so security reports are reachable through it. A reviewer can widen
  the scope by editing this plan.
- **Heading name?** — Recommendation: `## Contributing`, matching the issue
  text and the H1 of `CONTRIBUTING.md`.
- **Relative or absolute link?** — Recommendation: a relative link,
  `[CONTRIBUTING.md](CONTRIBUTING.md)`. This matches the README's existing
  relative links (e.g. `[server/README.md](server/README.md)`) and works in
  forks.

## Implementation Plan

### Phase 1: Add the Contributing section
**Goal:** The README links new contributors to `CONTRIBUTING.md`.
**Tasks:**
- [ ] In `README.md`, after the final line of the `## Testing` section, add one
  blank line and then:

  ```markdown
  ## Contributing

  Contributions are welcome. Before opening a pull request, read
  [`CONTRIBUTING.md`](CONTRIBUTING.md) — it covers signing your commits (DCO),
  the checks to run before a PR, and how to report a security issue.
  ```

  The wording may change slightly, but the section must be an H2 named
  `Contributing`, be the last section in the file, and contain a working
  relative link to `CONTRIBUTING.md`.
- [ ] Make sure the file ends with exactly one trailing newline.
- [ ] Verify with `grep -n "CONTRIBUTING.md" README.md`: it must return the new
  link. Check that `git diff --stat` touches only `README.md` (this plan file
  is already committed).
- [ ] No lint gate in `npm run lint` covers the root README, and no code
  generation or tests are needed. (`cspell.yaml` exists, but cspell is not a
  project dependency or a CI step, so don't `npx`-install it for this change.)

**Validates:** On the PR's rendered README on GitHub, the new section appears
at the bottom and the link opens `CONTRIBUTING.md`.

## Audit Notes

`ripls-audit` scope: a docs-only edit to the root `README.md`. It touches no
client or server side, and the only engineering dimension engaged is
`code_quality` (docs). No ⚓ architecture or side anchor applies, and no leaf
doc's `globs` match `README.md`.

- **Fixed:** removed an optional `npx cspell README.md` step. cspell is not in
  `package.json` or any workflow, so running it would download a tool from the
  network for no gating benefit.
- **Checked:** the plan follows the CLAUDE.md README rules (contributor-facing
  prose, relative links). It adds no TODOs and is a single focused change. The
  `docs/issues/` plan files are excluded from the context map, so
  `lint:context-map` is unaffected.

Docs consulted: `CLAUDE.md`, `docs/llms.txt`, `.claude/skills/ripls-issue/SKILL.md`,
`CONTRIBUTING.md`, `README.md`, `package.json` (lint scripts).

## Related Issues
- #2970 — agent dispatch bridge rollout. This issue is a Phase 4 smoke test of
  that bridge (it lives in the upstream tracker, not this repo).
