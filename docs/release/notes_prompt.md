---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Canonical prompt and steps for generating docs/release/notes/X.Y.Z.md — separating features from fixes, citing GitHub issues, and crediting community reporters from feedback-issue Submitted By sections.
  globs: [.github/workflows/release_cut.yaml, docs/release/notes/**]
  triggers: [release-notes, notes-prompt, changelog, reporter-credit, release]
  lens: [infra]
  skills: [issue, audit, triage]
  domain: release
freshness:
  verified_commit: "2ba311708"
  verified_on: "2026-06-09"
---
# Release notes generation prompt

Canonical prompt for generating `docs/release/notes/X.Y.Z.md`. Referenced by
`.github/workflows/release_cut.yaml`, `.claude/skills/ripls-release/SKILL.md`
step 6, and `docs/release/cheat_sheet.md` so all three stay in sync.

---

## Prompt

Look at the changes since the last release tag, including all closed GitHub
issues and reviewing all merged PRs and their descriptions. Try to separate
out the significant new features from bug fixes. Whenever possible cite
specific GitHub issues with links in the release notes file. Try to be
concise and informative, targeting users of the app as well as developers,
emphasizing user-facing improvements.

## Steps

1. Find the most recent release tag.
2. Look at all changes since that tag: merged PRs, closed issues, and commit history.
3. Separate significant new features from bug fixes.
4. **Credit community reporters**: For each closed issue referenced in the release, fetch the issue body using `gh issue view`. If the body contains a `## Submitted By` section (these are app-submitted feedback issues), extract the email address. Use the local part of the email (everything before `@`) as the username. Include the credit inline with the corresponding bullet point, e.g.:
   ```markdown
   - **Chat Notifications**: Fixed deep-link for chat notifications
     ([#1036](…), reported by **ledreher**)
   ```
5. Create `docs/release/notes/$version.md` with concise, informative release notes targeting both app users and developers. Cite specific GitHub issues with links where possible, with inline reporter credits from step 4.
6. Rerun `npm run generate:release-notes` to convert the release notes to HTML in website directory for publication.
7. Commit the release notes together with the version bump.
