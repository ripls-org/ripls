---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: How /ripls-reviews and triage turn review findings into GitHub issues — required metadata, labels, issue type, dedup, and format.
  triggers: [issue-filing, labels, metadata, triage, github-issue]
  lens: [conventions]
  skills: [reviews, triage]
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Review-to-Issue Filing Spec

This spec is followed by every review prompt in `docs/reviews/` and executed by
the `ripls-reviews` skill. Its purpose: turn the highest-impact findings from
each review into GitHub issues so they flow into the backlog and get prioritized
by `/ripls-triage`.

## What gets filed

**File issues only for findings worth a human's prioritization attention.**
Concretely:

- **Always file:** Critical and High severity findings (security vulnerabilities,
  data-loss risks, regressions in core flows, performance hotspots, broken
  invariants, missing test coverage around modules that keep regressing,
  systemic patterns affecting many files).
- **File selectively:** Medium-severity findings only if they represent a
  durable problem (a pattern, not a one-off) or if fixing them would unblock
  other work.
- **Do not file:** Low/nice-to-have items, individual style nits, anything
  that's a single-line cleanup. Those stay in the report.

For categorical reviews (e.g., code quality), file one issue per *pattern*,
not one per occurrence. "30% of files in `server/services/` lack file
comments" → one issue, not 50.

**Maximum issues per review per run:** 10. If a review surfaces more, pick
the 10 most impactful and note in the report's Executive Summary that further
findings are documented but not filed.

## Output section: "Issues to File"

Every review report MUST end with a section titled exactly `## Issues to File`.
If there's nothing severe enough to file, write:

```markdown
## Issues to File

_None this run._
```

Otherwise, list each finding using this exact format (the `ripls-reviews` skill
parses it):

```markdown
## Issues to File

### [SEVERITY] Short title — keep under 70 characters

- **Fingerprint**: `review:<review_name>:<short-stable-key>`
- **Type**: Bug | Task
- **Why it matters**: 1–2 sentences on user/codebase impact.
- **Locations**: `path/to/file.go:123`, `path/to/other.dart:45`
- **Recommendation**: 1–2 sentences on the suggested fix or approach.
- **Suggested labels**: `comma,separated,labels` (optional)

### [SEVERITY] Next finding...
```

Field rules:

- **SEVERITY**: `Critical` or `High` (Medium allowed only when criteria above
  are met). Bracketed in the heading exactly as shown.
- **Fingerprint**: a stable, URL-safe identifier the skill uses to detect
  duplicates across runs. Format: `review:<review_name>:<descriptor>`, where
  `<review_name>` matches the report filename without `_review.md` (e.g.,
  `tech_debt`, `code_quality`, `security`), and `<descriptor>` is a short
  kebab-case slug describing the finding (e.g., `n+1-complete-transfer`,
  `missing-file-comments-services`). Fingerprints must be stable: the same
  finding in a future run must produce the same fingerprint, even if line
  numbers change.
- **Type**: `Bug` for things that are broken or wrong; `Task` for engineering
  work without user-facing change (refactors, missing tests, observability
  gaps, doc gaps, lint config changes).
- **Why it matters**: concrete impact, not generic ("blocks compaction of
  hot path: P95 latency on CompleteTransfer is 3× P50" beats "performance
  problem").
- **Locations**: file paths with line numbers where relevant. List up to ~5;
  if the pattern is widespread, give 2–3 representative locations and a count.
- **Recommendation**: actionable. If a tooling change can prevent recurrence
  (linter rule, CI check), recommend that.
- **Suggested labels**: omit unless the review surfaces one obviously useful.
  The triage skill will assign Priority and Issue Type later; don't try to
  pre-empt it. Don't include labels like `P0`–`P4` — the Project field owns
  that.

## How the skill files issues

The `ripls-reviews` skill, after all reviews complete, parses each report's
`## Issues to File` section and:

1. **Dedupes** against existing open issues by searching for the fingerprint
   string in issue bodies. If found, the skill skips re-filing; it may post a
   short comment on the existing issue if the locations or impact have
   meaningfully changed.
2. **Creates** new issues with:
   - **Title**: `[<review_name>] <finding title>` (e.g.,
     `[tech_debt] N+1 in CompleteTransfer hot path`)
   - **Body**: review name, severity, why-it-matters, locations,
     recommendation, link to the report file, and an HTML comment containing
     the fingerprint for future dedup:
     ```
     <!-- review-fingerprint: review:tech_debt:n+1-complete-transfer -->
     ```
   - **Labels**: `auto-filed`, `review:<review_name>`, plus any suggested
     labels from the finding.
   - **Issue Type**: not set at filing time. The finding's `Type` field is
     recorded as a "Type hint" in the issue body; `/ripls-triage` assigns the
     GitHub Issue Type later.
3. **Does not** assign Priority — that's `/ripls-triage`'s job. Issues land
   on the board untriaged and get picked up next triage cycle.

## Notes for review authors

- Don't pad. Filing 10 mediocre issues is worse than filing 2 sharp ones.
- Stable fingerprints matter more than perfect titles. A wobbly fingerprint
  causes dupes across runs.
- If a finding belongs to a *different* review's domain, mention it in your
  report but don't file it — let the right review own it.
