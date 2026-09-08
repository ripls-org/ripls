---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Tech-debt review checklist — TODO/FIXME/HACK, deprecated code, duplication, architecture violations, missing abstractions, doc/test/dependency debt. Follow its "Areas to Review" section.
  triggers: [tech-debt, todo, fixme, hack, deprecated, duplication, debt]
  lens: [tech_debt]
  alwaysApply: true
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Tech Debt Review Prompt

Run this review with Claude Code CLI:
```bash
claude -p "$(cat docs/reviews/tech_debt_review_prompt.md)"
```

Or use the slash command: `/tech-debt-review`

---

## Instructions

Perform a comprehensive tech debt review of this codebase. Write your findings to `docs/reviews/reports/tech_debt_review.md`.

## Step 0. Review Previous Report (If Exists)

Before starting the review, check if a previous report exists:

```bash
cat docs/reviews/reports/tech_debt_review.md 2>/dev/null || echo "No previous report"
```

**If a previous report exists:**
1. Note the date and all findings from the previous report
2. Track which items were identified previously
3. In your new report, include a "Changes Since Last Review" section that highlights:
   - Tech debt that has been paid down
   - Tech debt that remains unaddressed
   - New tech debt discovered
   - Overall trend (improving/stable/declining)

## Areas to Review

### 1. TODO/FIXME/HACK Comments

Search for and catalog all tech debt markers:

```bash
grep -rn "TODO\|FIXME\|HACK\|XXX\|TEMPORARY\|WORKAROUND" server/ app/lib/ --include="*.go" --include="*.dart"
```

- **Categorize**: Group TODOs by type (feature, bug, refactor, cleanup)
- **Age**: Note file modification dates to estimate age
- **Priority**: Assess which are blocking vs nice-to-have
- **Staleness**: Identify TODOs that reference old issues or departed developers

### 2. Deprecated Code

- **Deprecated APIs**: Code using deprecated language/framework features
- **Deprecated Dependencies**: Packages marked as deprecated
- **Dead Code**: Unused functions, classes, or files
- **Legacy Patterns**: Old patterns that don't match current architecture

### 3. Code Duplication

- **Copy-Paste Code**: Identify duplicated logic across files
- **Similar Functions**: Functions that do nearly the same thing
- **Repeated Patterns**: Patterns that should be abstracted
- **Config Duplication**: Duplicated configuration or constants

### 4. Architecture Violations

Reference `docs/server/architecture.md` and `docs/client/architecture.md`:

- **Layer Violations**: Code that bypasses architectural layers
- **Circular Dependencies**: Packages that depend on each other
- **Wrong Location**: Code in the wrong package/directory
- **Outdated Patterns**: Code not following current best practices

### 5. Missing Abstractions

- **Hard-Coded Values**: Magic numbers or strings that should be constants
- **Repeated Logic**: Code that should be extracted to shared utilities
- **Missing Interfaces**: Concrete dependencies that should be interfaces
- **Configuration**: Values that should be configurable but aren't

### 6. Documentation Debt

- **Outdated Docs**: Documentation that no longer matches code
- **Missing Docs**: Public APIs without documentation
- **Stale Comments**: Comments that don't match the code anymore
- **README Gaps**: Setup/usage instructions that are incomplete

### 7. Test Debt

- **Skipped Tests**: Tests marked as skip/ignore
- **Commented Tests**: Test code that's been commented out
- **Flaky Tests**: Tests that intermittently fail
- **Missing Tests**: Known gaps in test coverage

### 8. Dependency Debt

- **Outdated Versions**: Dependencies multiple major versions behind
- **Unused Dependencies**: Packages in go.mod/pubspec.yaml that aren't used
- **Heavy Dependencies**: Large dependencies used for small features
- **Forked Dependencies**: Dependencies that have been forked/patched

## Output Format

Write `docs/reviews/reports/tech_debt_review.md` with:

```markdown
# Tech Debt Review - [DATE]

## Executive Summary
[1-2 paragraph overview of tech debt status]

## Changes Since Last Review

> *If no previous report exists, write "This is the first tech debt review."*

**Previous Review Date**: [DATE or N/A]

**Paid Down:**
- ✅ [Tech debt from previous report that has been addressed]
  - Location: [file:line]
  - Resolved in: [commit/PR reference if known]

**Still Outstanding:**
- ⚠️ [Tech debt still present from previous report]
  - Original Priority: [Critical/High/Medium/Low]
  - Days Open: [X days]

**New Tech Debt:**
- 🆕 [Tech debt not present in previous report]

**Trend**: [Improving / Stable / Declining] - [Brief explanation]

---

## Tech Debt Inventory

### TODOs/FIXMEs by Category

| Category | Count | High Priority | Files |
|----------|-------|---------------|-------|
| Bug Fixes | X | Y | file1, file2 |
| Refactoring | X | Y | ... |
| Features | X | Y | ... |
| Cleanup | X | Y | ... |

### Full TODO List
[List each TODO with file:line, content, and estimated age]

## Critical Tech Debt
[Debt that's causing active problems]

## High Priority
[Debt that should be addressed soon]

## Medium Priority
[Debt to address when touching related code]

## Low Priority
[Nice to address eventually]

## Quick Wins
[Low-effort items that can be fixed quickly]

## Recommendations
[Prioritized action items with effort estimates]
```

For each finding include:
- **Location**: File path and line number
- **Description**: What the tech debt is
- **Impact**: How it affects development velocity or quality
- **Effort**: Estimated effort to address (hours/days)
- **Recommendation**: Suggested approach to resolve
