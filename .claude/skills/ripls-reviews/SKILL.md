---
name: ripls-reviews
description: Run all code review prompts in docs/reviews/ in parallel, write reports, and auto-file the highest-impact findings as GitHub issues for triage. Use when the user types /ripls-reviews or asks to "run reviews", "run all reviews".
---

Run all review prompts from `docs/reviews/` in parallel, each producing a
report in `docs/reviews/reports/`. Then parse each report's "Issues to File"
section and create GitHub issues for the highest-impact findings so they flow
into the backlog for `/ripls-triage` to prioritize.

## Phase 1 — Spawn review agents

1. List all `*_review_prompt.md` files in `docs/reviews/`. Skip
   `issue_filing_spec.md` — it's a shared spec, not a review.

2. For each review prompt file, spawn a separate Agent
   (subagent_type: "general-purpose") **in parallel** using
   `run_in_background: true`. Each agent should:
   - Read the full contents of its assigned review prompt file.
   - **Also read `docs/reviews/issue_filing_spec.md`** and follow it: every
     report must end with an `## Issues to File` section in the format the
     spec defines.
   - Follow the instructions in its prompt to perform the review.
   - Write its report to the corresponding file in `docs/reviews/reports/`
     (e.g., `tech_debt_review_prompt.md` → `reports/tech_debt_review.md`).

   The agent prompt should include:

   > "You are performing a code review of the ripls project. Read and follow
   > all instructions in `docs/reviews/{filename}`. Before reviewing, also
   > consult `docs/llms.txt` (the documentation context map) and read the
   > subsystem/feature docs whose `globs`/`triggers` match this review's
   > dimension — they document the intended design, so your findings are
   > grounded in what the code is *supposed* to do, not just what it does.
   > Also read `docs/reviews/issue_filing_spec.md` and append an
   > `## Issues to File` section to your report exactly as the spec defines —
   > this is how the most severe findings get promoted to GitHub issues for
   > triage.
   >
   > **Calibrate the bar honestly.** Zero findings is a valid, healthy
   > outcome — if nothing in this codebase clears the severity bar defined
   > by the prompt and the issue-filing spec, write `_None this run._`
   > under the `## Issues to File` section and move on. Do **not** invent,
   > pad, or downgrade the bar to fill the section just because the prompt
   > asked you to look. A short, accurate report with no filed issues is
   > more useful than a long one full of marginal findings — those create
   > triage noise and erode trust in the review pipeline.
   >
   > Write your report to `docs/reviews/reports/{report_name}.md`. Be
   > thorough and follow the output format specified in both documents."

3. Wait for all agents to complete.

## Phase 2 — Verify reports

4. For each expected report, confirm the file exists and contains an
   `## Issues to File` section. If a report is missing the section, note
   it in the summary and skip filing for that review (don't fabricate
   findings).

## Phase 3 — File issues

This phase is fully automated by `file_issues.py` (next to this file). The
script parses every report's `## Issues to File` section, dedupes against
existing open issues by fingerprint, and creates new issues via `gh`.

5. **Check `gh` auth.** Confirm `gh auth status` succeeds. If not, surface
   the error and stop before Phase 3.

6. **Ensure required labels exist.** Run once if labels are missing:

   ```bash
   gh label create auto-filed --color "EDEDED" --description "Auto-filed by /ripls-reviews; awaits triage" 2>/dev/null
   for r in accessibility api_design architecture code_quality dependency \
            error_handling observability performance security tech_debt testing; do
     gh label create "review:${r}" --color "C5DEF5" --description "Filed by /ripls-reviews from ${r} review" 2>/dev/null
   done
   ```

   These labels are how filed issues are identified — `auto-filed` tags every
   bug created by this skill; `review:<name>` records which review surfaced it.

7. **Dry-run first.** Always preview before applying:

   ```bash
   python3 .claude/skills/ripls-reviews/file_issues.py
   ```

   Default severity filter is `Critical` only. Pass `--severities=Critical,High`
   or `--severities=Critical,High,Medium` to widen the scope.

8. **Apply.** When the dry-run looks right:

   ```bash
   python3 .claude/skills/ripls-reviews/file_issues.py --apply
   ```

   The script:
   - Pulls existing `auto-filed` issues and extracts their
     `<!-- review-fingerprint: ... -->` markers; matches on the fingerprint
     skip filing.
   - Creates each new issue with title `[<review_name>] <finding title>`.
   - Adds `auto-filed` and `review:<name>` labels (any suggested labels not
     already present in the repo are dropped and reported in the summary
     so you can decide whether to create them).
   - Does NOT set Priority — `/ripls-triage` owns that.

## Phase 4 — Summarize

11. Print a summary:
    - Reviews completed (with report paths).
    - Reviews that failed or were missing the `## Issues to File` section.
    - **Issues filed:** count and list of new issue numbers/titles, grouped
      by review.
    - **Issues deduped:** count and list of fingerprints that matched
      existing open issues.
    - **Issues failed:** count with errors.
    - **Next step:** suggest running `/ripls-triage` to prioritize the
      newly-filed issues.

## Edge cases

- **First-time run, no `auto-filed` label exists yet:** `gh issue list`
  returns empty. The dedup map is empty. All findings get filed. Subsequent
  runs will dedupe against these.
- **Fingerprint drift:** if a review author rewrites a fingerprint between
  runs, the skill will create a duplicate issue. The summary should make
  filed issues easy to scan so the user can spot dupes and close them.
- **`gh` not authenticated or repo not linked:** skip Phase 3 entirely; tell
  the user to authenticate and re-run with `--file-issues` (or just re-run).
  The reports themselves are still valuable on their own.
- **Issue Types not enabled on the repo:** filing still succeeds; type just
  isn't set. `/ripls-triage` handles type assignment.
- **A review writes "_None this run._" under Issues to File:** that's a
  valid healthy outcome. Note it in the summary so the user knows the
  review ran cleanly.
- **Closed issue with matching fingerprint:** the `--state open` filter
  means closed issues won't dedupe. If a finding was previously fixed and
  resurfaces, a new issue is correct (regression).
