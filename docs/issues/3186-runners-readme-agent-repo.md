# Issue #3186: Bridge test: note the agent repo in the runners README

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/3186
**Filed:** 2026-10-20 | **Last activity:** 2026-10-20
**Labels:** none | **Assignees:** unassigned
**Context docs:** `ripls-notes/infra_mimir.md` (Fleet shape — already states the
org-level registration and Default runner group), `runners/README.md`,
`runners/docker-compose.yml`

## Problem Statement

`runners/README.md` only mentions org-level registration in passing, as a
reason the host PAT needs `admin:org` (the `.env.example` bullet, line 33). It
does not say *why* the fleet is registered at the org level — so the private
agent repo `Type2Fun-Foundation/ripls-agent` (#2970) can use the same runners —
or that the org's **Default** runner group excludes public repos, which keeps
public repos off the self-hosted fleet. A reader of the runners README has to
find `ripls-notes/infra_mimir.md` → *Fleet shape* to learn this.

The issue is also a test of the dispatch bridge cut-over (#2970). It asks for
the plan path and a short plan, so that implement can be tested on it next.

## Context from Comments

- Both comments are from maintainer: "@claude please plan this", and a
  retry after a runner workspace fix. Neither adds requirements. The body sets
  the scope: one or two sentences, a one-paragraph docs change.

## Potential Solutions

### Option A: Add a short paragraph to the README intro
- **Approach:** After the intro paragraph (lines 3–6) and before the
  compose-project-name paragraph, add 1–2 sentences: the runners register at
  the Type2Fun-Foundation org level (`RUNNER_SCOPE: org` in
  `docker-compose.yml`), so both `ripls` and the private agent repo
  `ripls-agent` (#2970) can use them; they sit in the org's Default runner
  group, which excludes public repos. Link `ripls-notes/infra_mimir.md`'s
  *Fleet shape* section for more detail.
- **Pros:** Easy to find. Matches what the issue asks for. The existing
  `.env.example` bullet can keep its PAT-scope note.
- **Cons:** States the org-registration fact a second time inside the README.
- **Effort:** small

### Option B: Expand the existing `.env.example` bullet
- **Approach:** Add the agent-repo and runner-group sentences to the bullet at
  line 32–34.
- **Pros:** No duplication: everything about org registration sits in one place.
- **Cons:** Hidden under a bullet about the `.env` file. A reader looking for
  "which repos use these runners" won't look there.
- **Effort:** small

## Recommendation

**Option A.** The point is that a reader can see which repos the fleet serves,
so it belongs in the intro. The `.env.example` bullet stays as it is, because
it explains the PAT scope, which is a separate concern.

## Open Questions

- **Name the agent repo, or just "the private agent repo"?** — Recommendation:
  name it (`ripls-agent`) and cite #2970, as `infra_mimir.md` does. The repo is
  private, but its name is already public in this repo's docs.
- **Mention the `mimir-runner-N` registration names?** — Recommendation: no.
  The issue asks for one or two sentences, and `infra_mimir.md` already has
  that detail.

## Implementation Plan

### Phase 1: README paragraph
**Goal:** `runners/README.md` says which repos use the fleet and how public
repos are kept off it.
**Tasks:**
- [ ] In `runners/README.md`, insert a new paragraph between the intro (ends
      "`./sync.sh`.", line 6) and the "The compose project name must equal…"
      paragraph. Suggested text:

      > The runners register at the Type2Fun-Foundation org level
      > (`RUNNER_SCOPE: org`), so the private agent repo `ripls-agent` (#2970)
      > shares the fleet with `ripls`. They sit in the org's Default runner
      > group, which excludes public repos, so public repos can't schedule jobs
      > on them. See `ripls-notes/infra_mimir.md` → *Fleet shape*.

- [ ] Wrap at ~80 columns to match the surrounding prose. Make no other edits.
**Validates:** `npm run lint` passes. `runners/README.md` is not in
`docs/llms.txt` and has no `context:`/`freshness:` frontmatter, so the
context-map gate is unaffected. Its markdown lint still runs.

## Audit Notes

- **Scope:** docs-only, infra. The security dimension is engaged because the
  paragraph describes who can schedule jobs on self-hosted runners that mount
  the host Docker socket.
- **Security:** it's fine to state that public repos are excluded. This repo
  already says so in `infra_mimir.md`, and the paragraph names no host
  credentials or secrets. One caveat: the Default-group "exclude public repos"
  behaviour is an org setting in GitHub. Nothing in this repo enforces it, so
  the README sentence describes that setting and can't be checked against code.
  The suggested text says "sit in the org's Default runner group" and does not
  claim the repo enforces it.
- **Conventions:** using the vendor and repo names (GitHub, `ripls-agent`) is
  correct for contributor docs, per CLAUDE.md → *READMEs and Internal
  Documentation*. No TODOs are introduced.
- **No changes required** to the plan beyond this note.
- **Docs consulted:** `docs/llms.txt`, `ripls-notes/infra_mimir.md`,
  `docs/reviews/security_review_prompt.md` (Areas to Review: secrets
  management), CLAUDE.md.

## Related Issues
- #2970 — agent dispatch bridge; introduced the private `ripls-agent` repo
  that shares the runner fleet.
