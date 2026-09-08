---
name: ripls-issue
description: Create a detailed implementation plan for a GitHub issue. Use this skill when the user types /ripls-issue, asks to "fix issue #X", "plan issue #X", "break down issue #X", "write up issue #X", or wants an analysis and implementation plan for a specific GitHub issue. Also triggers when the user says "look at issue #X and figure out how to solve it", "what's the plan for #X", or asks for a phased approach to an issue. Requires an issue number as input.
---

# Issue Plan Skill

Read a GitHub issue thoroughly — description, comments, and all context — then produce a structured planning document with problem analysis, potential solutions, recommendations, open questions, and a phased implementation plan.

## Input

The user provides a GitHub issue number and optionally one or more context documents. Examples:
- `/issue-plan 757` — just the issue
- `/issue-plan 757 docs/registration_and_login.md` — issue plus one context doc
- `/issue-plan 757 docs/registration_and_login.md docs/auth_architecture.md` — issue plus multiple context docs

Extract the issue number and any file paths from whatever format the user provides. Anything that looks like a path (contains `/` or ends in `.md`) is a context doc.

## Workflow

### Phase 1 — Read the issue

1. Fetch the full issue including body and comments:
   ```bash
   gh issue view <number> --json number,title,body,comments,labels,assignees,milestone,state,createdAt,updatedAt,url
   ```

2. Read every comment — not just the description. Critical context, clarifications, and revised requirements often live in the comments. Pay attention to who said what; maintainer comments carry different weight than drive-by suggestions.

3. Note the issue title. You'll use a slugified version of it in the filename.

### Phase 1.5 — Read context documents

4. If the user provided any context documents, read each one before proceeding. These docs provide domain knowledge, architectural context, or background that the issue assumes you already know. Treat them as required reading — they should inform your analysis just as much as the issue itself.

   If a context doc path doesn't exist, tell the user and continue with what you have.

### Phase 1.6 — Load documentation context

Before and during codebase research, consult the **documentation context map** so
your plan is grounded in the project's architecture, conventions, and product
priorities — not just the issue text.

1. Read `docs/llms.txt` (the generated index of all durable guidance docs).
2. **Load the anchors** — every ⚓ doc whose `lens` is a subset of
   `{architecture, conventions, product}` plus the side(s) the issue touches
   (client for `app/lib/**`, server for `server/**`). This pulls the relevant
   architecture doc(s) and `proto_conventions.md`, plus any product-lens
   anchors the index declares. Take the index as authoritative about what
   exists: product-strategy docs are deployment-specific and a given checkout
   may carry none.
3. As Phase 2 research surfaces the files and subsystems involved, **load the
   matching leaf docs** — those whose `globs` match the touched paths or whose
   `triggers`/`description` match the issue (and whose `skills` includes `issue`,
   or omits `skills`).
4. **Note the engaged engineering dimensions** the solution will introduce (e.g.
   a new endpoint → api_design, error_handling, security, testing) and skim each
   one's `docs/reviews/<dimension>_review_prompt.md` **"Areas to Review"** section
   so the plan designs for them up front.

Load only what's relevant — not the whole corpus. Manual context docs (Phase 1.5)
are additive to whatever the map selects. Record the docs you loaded in the
plan's **Context docs** header. The full schema and selection model are in
`docs/context_map.md`; if `docs/llms.txt` is missing, run
`npm run generate:context-map`.

### Phase 2 — Research the codebase

5. Before proposing solutions, understand what exists. Based on what the issue describes and what the context docs explain, look at the relevant parts of the codebase:
   - Search for related files, functions, or modules mentioned in the issue.
   - Check for existing patterns the solution should follow.
   - Look for related issues or PRs referenced in the comments.

   This step is essential — don't skip it. A plan that ignores the existing codebase will be wrong.

6. **If the issue describes UI-observable misbehavior** — a user saw the wrong
   thing on a screen, a gesture misfired, state didn't update, duplicate
   entries appeared (user-feedback issues usually qualify) — run the
   `ripls-ui-investigate` skill on the issue before writing Potential
   Solutions. It reproduces the report against a hermetic web environment,
   bisects the failure to a layer (client render / client state / server /
   data / UX gap), and writes a `## UI Investigation` section into the same
   `docs/issues/` doc this plan uses. Ground the Problem Statement and
   Potential Solutions in its verdict instead of guessing the layer from the
   issue text. Skip it only when the behavior can't exist on the web bundle
   (platform-native: push notifications, camera, app lifecycle) — note in the
   plan why it was skipped.

### Phase 3 — Write the plan

7. Generate the filename from the issue number and a short slug derived from the title. The slug should be 2-4 words, lowercase, hyphenated, capturing the core topic. Examples:
   - Issue #757 "User login fails on mobile Safari" → `docs/issues/757-mobile-login.md`
   - Issue #42 "Add dark mode support" → `docs/issues/42-dark-mode.md`
   - Issue #183 "PostgreSQL connection pool exhaustion under load" → `docs/issues/183-db-pool-exhaustion.md`

   Create the `docs/issues/` directory if it doesn't exist.

8. Write the document using the template below.

### Phase 4 — Run audit

9. After creating the plan document, run the `/ripls-audit` skill and pass the filename. For example: `/ripls-audit docs/issues/757-mobile-login.md`. This is not optional — every issue plan gets audited.

## Plan document template

```markdown
# Issue #[number]: [full title]

**Source:** [issue URL]
**Filed:** [created date] | **Last activity:** [updated date]
**Labels:** [labels] | **Assignees:** [assignees or "unassigned"]
**Context docs:** [docs you loaded — both any passed as input (Phase 1.5) and those auto-selected from `docs/llms.txt` (Phase 1.6) — or "none"]

## Problem Statement

A clear, concise description of what's wrong or what's needed. Written in your
own words after synthesizing the issue description and comments — not just
copy-pasting the issue body. Call out any contradictions or ambiguities between
the original description and later comments.

## Context from Comments

Summarize the key points raised in the issue comments. Highlight:
- Clarifications from the issue author
- Maintainer decisions or direction
- Workarounds people have found
- Disagreements or unresolved debates

If there are no comments, note that and flag it — an issue with no discussion
may need more input before planning.

## Potential Solutions

Present 2-3 viable approaches. For each one:

### Option A: [name]
- **Approach:** What this involves.
- **Pros:** Why this is attractive.
- **Cons:** What the downsides or risks are.
- **Effort:** Rough estimate (small / medium / large).

### Option B: [name]
(same structure)

### Option C: [name]
(same structure, if applicable)

## Recommendation

State which option you recommend and why. Be specific about the tradeoffs
you're making. If the answer depends on information you don't have, say so
and list what you'd need to know.

## Open Questions

Things that need to be answered before or during implementation. For each
question, include your best recommendation so the reviewer has a starting
point — not just a list of unknowns.
- **[question 1]** — Recommendation: [your suggested answer and reasoning]
- **[question 2]** — Recommendation: [your suggested answer and reasoning]

## Implementation Plan

Break the recommended approach into phases. Each phase should be a shippable
increment — don't front-load all the work into phase 1.

### Phase 1: [name]
**Goal:** [what this achieves]
**Tasks:**
- [ ] Task 1
- [ ] Task 2
- [ ] Task 3
**Validates:** [what you'll learn or prove by completing this phase]

### Phase 2: [name]
(same structure)

### Phase 3: [name]
(same structure, if needed)

## Related Issues
- #[number] — [brief description of relationship]
```

## Edge cases

- **Issue doesn't exist or is inaccessible:** Stop and tell the user. Don't create a plan document for a missing issue.
- **Issue is already closed:** Proceed but note prominently at the top that the issue is closed. The user may want a plan for reopening or for a related follow-up.
- **Issue has no body / description is empty:** Flag this in Open Questions. You can still plan based on the title and comments, but note the gap.
- **Issue is enormous (epic-level):** If the issue covers too much ground for a single plan, say so and suggest breaking it into sub-issues first. Offer to help with that breakdown.
- **`/ripls-audit` skill not available:** If the audit skill isn't installed or doesn't trigger, tell the user that the plan was created but the audit step couldn't run, and suggest they run it manually with `/ripls-audit docs/issues/<filename>`.
- **File already exists for this issue:** If `docs/issues/<number>-*.md` already exists, don't overwrite it. Instead, use a versioned name: `757-mobile-login-v2.md`. Tell the user the previous version still exists so they can diff.

## Tips for best results

- The more detailed the GitHub issue, the better the plan. If an issue is vague, the Open Questions section will be long — that's a signal to go back and clarify the issue first.
- Plans are living documents. After review, update the plan with decisions made and then use it as a reference during implementation.
- The `/ripls-audit` step at the end catches architectural issues, security concerns, and alignment problems early — before any code is written.
