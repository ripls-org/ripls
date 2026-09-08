---
name: ripls-audit
description: Audits a plan or change for compliance with the project's architecture, conventions, infrastructure, security, and engineering-review standards. Use when the user asks to audit a plan, "Is the plan compliant?", or to check a change against the docs. Takes a target .md plan file (or, with no argument, the current working diff).
---

# Audit Skill

Audit a target plan (any `.md` plan file) — or, if none is given, the current
working diff — against the project's documented standards. **Select the relevant
guidance from the documentation context map (`docs/llms.txt`) rather than a fixed
doc list**, so an infra change is checked against infra/security docs and a
Flutter change against client/accessibility docs — and nothing irrelevant is
loaded.

## Workflow

### Phase 1 — Determine scope

From the target's file references (or the diff's paths), identify:

- **Side:** client (`app/lib/**`), server (`server/**`), or both.
- **Subsystems:** the features/areas involved (chat, media, auth, impact, …).
- **Engaged engineering dimensions** — which review dimensions the change
  implicates: `security` (auth, secrets, SSRF, PII), `error_handling`,
  `observability`, `performance` (N+1, hot paths), `testing`, `code_quality`,
  `api_design` (proto/RPC changes), `accessibility` (UI), `dependency` (new
  deps), `tech_debt`. For server work, `error_handling` + `observability` +
  `security` + `testing` + `code_quality` + `api_design` are almost always in
  play; add the rest when the change touches them.

### Phase 2 — Select context from the map

Read `docs/llms.txt`, then load only what's relevant:

1. **Anchors** — every ⚓ doc whose `lens` is a subset of the active lenses, where
   active lenses = `{architecture, conventions}` + the side(s) + the engaged
   dimensions. This pulls `server/architecture.md` and/or `client/architecture.md`,
   `proto_conventions.md`, `server/observability.md` and/or `testing/architecture.md`,
   and the review prompt for each engaged dimension.
2. **Review-dimension rubrics** — for each engaged dimension, open its
   `docs/reviews/<dimension>_review_prompt.md` and apply its **"Areas to Review"**
   section as the checklist. Ignore the runner scaffolding (*Instructions*,
   *Step 0*, *Output Format*) — that's for the `/ripls-reviews` runner.
3. **Leaf docs** — the subsystem/feature docs whose `globs` match the touched
   paths, or whose `triggers`/`description` match the change. Only consider docs
   whose `skills` includes `audit` (or omits `skills`, meaning all).

Read the selected docs. **Do not read the whole corpus**, and **do not fall back
to a fixed doc list** — that hardcoded list (and its blind spots for infra and
security) is the gap this skill replaced.

### Phase 3 — Audit

Review the target against the loaded standards for:

- Compliance with the architecture and convention anchors.
- The **"Areas to Review"** checklist of each engaged review dimension.
- Server/client boundary alignment; correct caching, i18n, observability,
  testing, security, and infra practices where they apply.
- Any contradiction with an existing documented decision.

### Phase 4 — Apply fixes

Update the plan in-place with fixes, flagging what changed and why. **List the
docs you consulted** at the end of your audit notes, so the reviewer sees the
basis for the audit.

## Notes

- `docs/context_map.md` is the source of truth for the schema, lens vocabulary,
  and selection model. If `docs/llms.txt` is missing, run
  `npm run generate:context-map`.
- Selecting only relevant docs is the point — it keeps the audit fast and lets it
  cover the whole guidance set (including infra/security) instead of a frozen
  subset.
