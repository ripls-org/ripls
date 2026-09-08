---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Error-handling review checklist — Go error wrapping/Connect codes/silent failures, Flutter error states, API error responses, logging, retry/recovery, validation. Follow its "Areas to Review" section.
  triggers: [error, error-handling, panic, recover, retry, exception, validation, wrapping]
  lens: [error_handling]
  alwaysApply: true
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Error Handling Review Prompt

Run this review with Claude Code CLI:
```bash
claude -p "$(cat docs/reviews/error_handling_review_prompt.md)"
```

Or use the slash command: `/error-handling-review`

---

## Instructions

Perform a comprehensive error handling review of this codebase. Write your findings to `docs/reviews/reports/error_handling_review.md`.

## Step 0. Review Previous Report (If Exists)

Before starting the review, check if a previous report exists:

```bash
cat docs/reviews/reports/error_handling_review.md 2>/dev/null || echo "No previous report"
```

**If a previous report exists:**
1. Note the date and all findings from the previous report
2. Track which issues were identified previously
3. In your new report, include a "Changes Since Last Review" section that highlights:
   - Issues that have been remediated
   - Issues that remain unaddressed
   - New issues discovered
   - Error handling improvements made
   - Overall trend (improving/stable/declining)

## Areas to Review

### 1. Go Error Handling (server/)

- **Error Wrapping**: Check for proper error wrapping with context (`fmt.Errorf("...: %w", err)`)
- **Error Types**: Verify appropriate use of Connect error codes (NotFound, InvalidArgument, etc.)
- **Silent Failures**: Look for ignored errors (`_ = someFunc()` or missing error checks)
- **Error Messages**: Check that error messages are descriptive but don't leak sensitive info
- **Panic Recovery**: Verify panics are recovered appropriately in HTTP handlers

### 2. Flutter Error Handling (app/lib/)

- **Try-Catch Usage**: Check for appropriate exception handling
- **Error States**: Verify ViewModels properly expose error states to UI
- **User-Facing Messages**: Check error messages are user-friendly (not stack traces)
- **Error Recovery**: Look for opportunities to recover gracefully from errors
- **Async Error Handling**: Verify Future and Stream errors are handled

### 3. API Error Responses

- **Consistent Format**: Check all endpoints return errors in consistent format
- **HTTP Status Codes**: Verify appropriate status codes are used
- **Error Details**: Check if errors include enough detail for debugging
- **Client Handling**: Verify Flutter properly handles all error types from API

### 4. Logging & Observability

- **Error Logging**: Check errors are logged with sufficient context
- **Log Levels**: Verify appropriate log levels (error vs warn vs info)
- **Correlation IDs**: Check for request tracing through error paths
- **Sensitive Data**: Ensure errors don't log passwords, tokens, PII

### 5. Retry & Recovery

- **Transient Errors**: Check if transient errors (network, timeout) trigger retries
- **Retry Logic**: Verify retry logic has backoff and limits
- **Circuit Breakers**: Look for circuit breaker patterns on external services
- **Graceful Degradation**: Check for fallback behavior when services fail

### 6. Validation Errors

- **Input Validation**: Check validation errors are specific and actionable
- **Field-Level Errors**: Verify form errors indicate which fields are invalid
- **Validation Timing**: Check validation happens early (fail fast)

## Output Format

Write `docs/reviews/reports/error_handling_review.md` with:

```markdown
# Error Handling Review - [DATE]

## Executive Summary
[1-2 paragraph overview of error handling posture]

## Changes Since Last Review

> *If no previous report exists, write "This is the first error handling review."*

**Previous Review Date**: [DATE or N/A]

**Remediated Issues:**
- ✅ [Issue from previous report that has been fixed]
  - Location: [file:line]
  - Fixed in: [commit/PR reference if known]

**Unaddressed Issues:**
- ⚠️ [Issue still present from previous report]
  - Original Priority: [Critical/High/Medium/Low]
  - Current Priority: [May escalate if aging]
  - Days Open: [X days]

**New Issues:**
- 🆕 [Issue not present in previous report]

**Improvements:**
- [Error handling improvements since last review]

**Trend**: [Improving / Stable / Declining] - [Brief explanation]

---

## Critical Issues
[Unhandled errors that could crash or corrupt data]

## High Priority
[Poor error handling affecting user experience]

## Medium Priority
[Inconsistent or unclear error handling]

## Low Priority
[Minor improvements and best practices]

## Positive Findings
[Well-handled error scenarios worth highlighting]

## Recommendations
[Prioritized action items]
```

For each finding include:
- **Location**: File path and line number
- **Description**: What the issue is
- **Impact**: What could go wrong
- **Recommendation**: How to fix it with code examples
