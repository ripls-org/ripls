---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Testing review checklist — coverage analysis, Go and Flutter test layers, test quality and organization, missing test types. Follow its "Areas to Review" section.
  triggers: [testing, test, coverage, mock, flaky, integration, unit]
  lens: [testing]
  alwaysApply: true
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Testing Review Prompt

Run this review with Claude Code CLI:
```bash
claude -p "$(cat docs/reviews/testing_review_prompt.md)"
```

Or use the slash command: `/testing-review`

---

## Instructions

Perform a comprehensive testing review of this codebase. Write your findings to `docs/reviews/reports/testing_review.md`.

## Step 0. Review Previous Report (If Exists)

Before starting the review, check if a previous report exists:

```bash
cat docs/reviews/reports/testing_review.md 2>/dev/null || echo "No previous report"
```

**If a previous report exists:**
1. Note the date and all findings from the previous report
2. Track which gaps were identified previously
3. In your new report, include a "Changes Since Last Review" section that highlights:
   - Gaps that have been addressed
   - Gaps that remain unaddressed
   - New gaps discovered
   - Testing improvements made
   - Overall testing trend (improving/stable/declining)

## Areas to Review

### 1. Test Coverage Analysis

- **Untested Files**: Find source files without corresponding test files
- **Untested Functions**: Identify public functions without test coverage
- **Critical Path Coverage**: Verify auth, payment, and data mutation paths are tested
- **Edge Cases**: Check if tests cover boundary conditions and error cases

### 2. Server Testing (Go)

- **Unit Test Quality**: Check for meaningful assertions vs trivial tests
- **Table-Driven Tests**: Verify appropriate use of table-driven testing
- **Mock Usage**: Check mocks are used appropriately (not over-mocked)
- **Integration Tests**: Verify database and external service integration tests exist
- **Test Isolation**: Ensure tests don't depend on each other or shared state
- **Error Path Testing**: Verify error conditions are tested

### 3. Flutter Testing (app/test/)

- **Widget Tests**: Check for missing widget tests on key screens
- **ViewModel Tests**: Verify state management logic is tested
- **Repository Tests**: Check data layer testing
- **Mock Generation**: Verify Mockito mocks are up to date
- **Golden Tests**: Identify opportunities for visual regression tests

### 4. Test Quality

- **Flaky Tests**: Look for tests with timing dependencies or race conditions
- **Test Readability**: Check test names clearly describe what's being tested
- **Setup/Teardown**: Verify proper test setup and cleanup
- **Assertion Quality**: Check for meaningful assertions (not just "no error")
- **Test Data**: Look for hardcoded test data that should be generated

### 5. Test Organization

- **File Structure**: Verify test files mirror source file structure
- **Test Helpers**: Check for duplicated test setup that could be shared
- **Test Categories**: Verify unit/integration/e2e tests are properly separated
- **CI Integration**: Check tests are running in CI pipeline

### 6. Missing Test Types

- **Security Tests**: Auth bypass, injection, authorization tests
- **Performance Tests**: Load testing, benchmark tests
- **Contract Tests**: API contract verification
- **Snapshot Tests**: UI snapshot testing for Flutter

## Output Format

Write `docs/reviews/reports/testing_review.md` with:

```markdown
# Testing Review - [DATE]

## Executive Summary
[1-2 paragraph overview of testing posture]

## Changes Since Last Review

> *If no previous report exists, write "This is the first testing review."*

**Previous Review Date**: [DATE or N/A]

**Addressed Gaps:**
- ✅ [Gap from previous report that has been addressed]
  - Location: [file:line]
  - Added in: [commit/PR reference if known]

**Unaddressed Gaps:**
- ⚠️ [Gap still present from previous report]
  - Original Priority: [Critical/High/Medium/Low]
  - Current Priority: [May escalate if aging]
  - Days Open: [X days]

**New Gaps:**
- 🆕 [Gap not present in previous report]

**Testing Improvements:**
- [New tests or testing infrastructure added since last review]

**Trend**: [Improving / Stable / Declining] - [Brief explanation]

---

## Coverage Summary

| Area | Files Tested | Files Untested | Coverage |
|------|--------------|----------------|----------|
| Server Services | X | Y | Z% |
| Server Libraries | X | Y | Z% |
| Flutter ViewModels | X | Y | Z% |
| Flutter Widgets | X | Y | Z% |
| Flutter Repositories | X | Y | Z% |

## Critical Gaps
[Untested critical functionality - auth, data mutations, etc.]

## High Priority
[Important missing tests]

## Medium Priority
[Tests that should be added]

## Low Priority
[Nice-to-have tests]

## Test Quality Issues
[Problems with existing tests]

## Positive Findings
[Well-tested areas worth highlighting]

## Recommendations
[Prioritized action items]
```

For each finding include:
- **Location**: File path that needs tests
- **Description**: What's missing
- **Impact**: Risk of not having tests
- **Recommendation**: Specific tests to add
