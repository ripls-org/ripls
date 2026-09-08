---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: API design review checklist — proto/RPC design, request/response shape, consistency, backwards compatibility, client usability, API security. Follow its "Areas to Review" section.
  triggers: [api, rpc, proto, endpoint, request, response, backwards-compatibility, versioning]
  lens: [api_design]
  globs: [proto/ripls/api/**]
  alwaysApply: true
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# API Design Review Prompt

Run this review with Claude Code CLI:
```bash
claude -p "$(cat docs/reviews/api_design_review_prompt.md)"
```

Or use the slash command: `/api-design-review`

---

## Instructions

Perform a comprehensive API design review of this codebase. Write your findings to `docs/reviews/reports/api_design_review.md`.

## Step 0. Review Previous Report (If Exists)

Before starting the review, check if a previous report exists:

```bash
cat docs/reviews/reports/api_design_review.md 2>/dev/null || echo "No previous report"
```

**If a previous report exists:**
1. Note the date and all findings from the previous report
2. Track which issues were identified previously
3. In your new report, include a "Changes Since Last Review" section that highlights:
   - Issues that have been remediated
   - Issues that remain unaddressed
   - New issues discovered
   - API improvements made
   - Overall trend (improving/stable/declining)

## Areas to Review

### 1. Proto/RPC Design (proto/ripls/api/)

See [`docs/proto_conventions.md`](../../proto_conventions.md) for the
authoritative conventions (separation, API message shape, naming parity
where fields round-trip, where conversion lives, what belongs on the API,
and round-trip test expectations). Review findings in this section should
cite that doc when flagging deviations.

- **Naming Consistency**: Check RPC and message naming conventions
- **Message Design**: Verify messages follow proto best practices
- **Field Numbering**: Check for gaps or reuse in field numbers
- **Deprecation**: Verify deprecated fields are marked correctly
- **Comments**: Check API documentation in proto comments
- **Read-path coverage**: When a new API field is added to a save-style
  request, verify every read-path response that should carry it was
  updated. This is the #1142 class of bug — caught by round-trip tests
  in `server/services/*/roundtrip_test.go` when they exist, otherwise by
  reviewer attention.

### 2. Request/Response Design

Per CLAUDE.md, each RPC should have dedicated request/response types:

- **Message Reuse**: Ensure no request/response types are shared across RPCs
- **Field Flattening**: Check appropriate use of inline vs nested messages
- **Optional Fields**: Verify optional fields are used appropriately
- **Repeated Fields**: Check for unbounded repeated fields
- **Pagination**: Verify list endpoints support pagination

### 3. API Consistency

- **Naming Patterns**: Check verb consistency (Get, List, Create, Update, Delete)
- **Error Codes**: Verify consistent use of Connect error codes
- **Field Names**: Check snake_case consistency
- **ID Formats**: Verify consistent ID field naming and types
- **Timestamp Fields**: Check consistent timestamp handling

### 4. Backwards Compatibility

- **Breaking Changes**: Identify potential breaking changes in recent commits
- **Field Removal**: Check for removed fields (should be reserved)
- **Type Changes**: Look for field type changes
- **Semantic Changes**: Check for changes in field meaning
- **Versioning**: Verify versioning strategy if applicable

### 5. Client Usability

- **Response Completeness**: Check if responses include all needed data
- **Request Simplicity**: Verify requests aren't overly complex
- **Error Messages**: Check error messages are actionable
- **Batch Operations**: Look for opportunities to reduce round trips

### 6. Documentation

- **Proto Comments**: Check all RPCs and messages have comments
- **Field Descriptions**: Verify field comments explain purpose and constraints
- **Examples**: Look for example values in documentation
- **Error Documentation**: Check error conditions are documented

### 7. API Security Considerations

- **Sensitive Fields**: Check sensitive fields are appropriately marked
- **Authorization**: Verify authorization requirements are documented
- **Input Validation**: Check validation constraints are documented
- **Rate Limits**: Look for rate limit documentation

## Output Format

Write `docs/reviews/reports/api_design_review.md` with:

```markdown
# API Design Review - [DATE]

## Executive Summary
[1-2 paragraph overview of API design quality]

## Changes Since Last Review

> *If no previous report exists, write "This is the first API design review."*

**Previous Review Date**: [DATE or N/A]

**Remediated Issues:**
- ✅ [Issue from previous report that has been fixed]
  - Location: [file:line]
  - Fixed in: [commit/PR reference if known]

**Unaddressed Issues:**
- ⚠️ [Issue still present from previous report]
  - Original Priority: [Critical/High/Medium/Low]
  - Days Open: [X days]

**New Issues:**
- 🆕 [Issue not present in previous report]

**Improvements:**
- [API design improvements since last review]

**Trend**: [Improving / Stable / Declining] - [Brief explanation]

---

## API Inventory

| Service | RPCs | Documented | Issues |
|---------|------|------------|--------|
| GearService | X | Y% | Z |
| UserService | X | Y% | Z |
| ... | ... | ... | ... |

## Critical Issues
[Breaking changes or severe design flaws]

## High Priority
[Significant design issues]

## Medium Priority
[Consistency and usability issues]

## Low Priority
[Minor improvements]

## Positive Findings
[Well-designed APIs worth highlighting]

## Recommendations
[Prioritized action items]
```

For each finding include:
- **Location**: Proto file path and line number
- **Description**: What the issue is
- **Impact**: How it affects API consumers
- **Recommendation**: How to fix it with examples
