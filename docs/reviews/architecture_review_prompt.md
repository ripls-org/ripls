---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Architecture review checklist — service independence, file organization, proto separation, caching patterns, code duplication, dependency injection, testing structure. Follow its "Areas to Review" section.
  triggers: [architecture, service-independence, layering, duplication, dependency-injection]
  lens: [architecture]
  alwaysApply: true
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Architecture Review Prompt

Run this review with Claude Code CLI:
```bash
claude -p "$(cat docs/reviews/architecture_review_prompt.md)"
```

Or use the slash command: `/architecture-review`

---

## Instructions

Perform a comprehensive architecture review of this codebase. Write your findings to `docs/reviews/reports/architecture_review.md`.

## Step 0. Review Previous Report (If Exists)

Before starting the review, check if a previous report exists:

```bash
cat docs/reviews/reports/architecture_review.md 2>/dev/null || echo "No previous report"
```

**If a previous report exists:**
1. Note the date, grade, and all findings from the previous report
2. Track which issues were identified previously
3. In your new report, include a "Changes Since Last Review" section that highlights:
   - Issues that have been resolved
   - Issues that remain unaddressed
   - New issues discovered
   - Architectural improvements made
   - Grade change explanation (if grade changed)
   - Overall trend (improving/stable/declining)

**First, read the architecture documentation:**
- `docs/server/architecture.md` - Server design principles
- `docs/client/architecture.md` - Client MVVM architecture
- `docs/client/caching.md` - Repository pattern and caching
- `CLAUDE.md` - Proto separation rules

## Areas to Review

### 1. Service Independence (Critical)

Per `docs/server/architecture.md`, services should NOT depend on other services.

- **Check service constructors**: Look for services injecting other services
- **Dependency graph**: Map what each service in `server/services/` depends on
- **Shared logic**: Identify logic that should be factored into libraries

**Good pattern:**
```go
type GearService struct {
    storage    *storage.ProtoSQLStorage
    aiProvider ai.Provider // Interface, not service
}
```

**Bad pattern:**
```go
type GearService struct {
    equityService *equity.Service // Service dependency - BAD
}
```

### 2. File Organization

Per architecture guidelines, files should be < 1000 lines and focused.

- **Find large files**: Use `wc -l` or similar to find files > 1000 lines
- **Check server services**: Each service should be split into focused files
- **Check Flutter viewmodels**: Look for monolithic viewmodels
- **Identify mixed concerns**: Files handling unrelated functionality

**Good example:** `server/services/community/` - a ~150-line service.go split across many focused files
**Target:** Service files < 500 lines, max 1000 lines

### 3. Proto Separation (Critical)

Per `CLAUDE.md`, API and models packages must never import each other.

- **Check imports**: Verify `proto/ripls/api/` and `proto/ripls/models/` don't cross-import
- **Client usage**: Flutter should ONLY use `ripls/api` protos, never `ripls/models`
- **RPC message reuse**: Each RPC should have dedicated request/response types

### 4. Caching Patterns (Flutter)

Per `docs/client/caching.md`:

- **Repository usage**: ViewModels should use repositories, not services directly
- **Cache key conventions**: Check `'namespace:item_id'` pattern
- **Manual caches**: Look for `Map<String, T>` caches in viewmodels (bad)
- **Cache invalidation**: Verify `refresh*()` or `invalidate*()` after mutations

### 5. Code Duplication

- **Authorization patterns**: Look for repeated "fetch entity, check ownership" logic
- **Error handling**: Identify repeated error handling patterns
- **Conversion logic**: Find duplicated proto/model conversion code
- **Opportunities**: Patterns repeated 3+ times should be abstracted

### 6. Dependency Injection

- **Constructor injection**: Verify dependencies are injected via constructors
- **Interface usage**: Check for interface-based dependencies (testability)
- **Optional dependencies**: Verify graceful handling when optional deps are nil

### 7. Testing Structure

- **Test coverage**: Check for missing test files alongside implementations
- **Mock usage**: Verify mock implementations exist for interfaces
- **Test isolation**: Each test should be independent

## Output Format

Write `docs/reviews/reports/architecture_review.md` with:

```markdown
# Architecture Review - [DATE]

## Executive Summary
[Overall architecture grade: A/B/C/D/F with justification]

## Changes Since Last Review

> *If no previous report exists, write "This is the first architecture review."*

**Previous Review Date**: [DATE or N/A]
**Previous Grade**: [A/B/C/D/F or N/A]
**Current Grade**: [A/B/C/D/F]
**Grade Change**: [↑ Improved / → Stable / ↓ Declined]

**Resolved Issues:**
- ✅ [Issue from previous report that has been fixed]
  - Location: [file:line]
  - How resolved: [Brief description]

**Unaddressed Issues:**
- ⏳ [Issue still present from previous report]
  - Original Priority: [Critical/High/Medium/Low]
  - Days Open: [X days]

**New Issues:**
- 🆕 [Issue not present in previous report]

**Architectural Improvements:**
- [Improvements made since last review]

**Metrics Comparison:**
| Metric | Previous | Current | Change |
|--------|----------|---------|--------|
| Service Independence | X% | Y% | +/-Z% |
| Files > 1000 lines | X | Y | +/-Z |
| Caching Pattern Compliance | X% | Y% | +/-Z% |

**Trend**: [Improving / Stable / Declining] - [Brief explanation]

---

## Critical Issues
[Architecture violations requiring immediate attention]

## High Priority
[Significant deviations from patterns]

## Medium Priority
[Areas for improvement]

## Low Priority
[Minor refinements]

## Positive Findings
[What's working well - cite specific examples]

## Metrics
- Service Independence: X% compliant
- File Size Distribution: X files < 500, Y files 500-1000, Z files > 1000
- Proto Separation: Compliant/Non-compliant
- Caching Pattern: X% using repositories

## Recommendations
[Prioritized action items with estimated effort]
```

For each finding include:
- **Location**: File path and line number
- **Description**: What the issue is
- **Impact**: How it affects maintainability/scalability
- **Recommendation**: How to fix it with code examples where helpful
