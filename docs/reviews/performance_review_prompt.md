---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Performance review checklist — DB/storage, API and Go server performance, Flutter UI and state-management performance, network, proto/serialization. Follow its "Areas to Review" section.
  triggers: [performance, latency, n+1, query, slow, throughput, memory, render]
  lens: [performance]
  alwaysApply: true
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Performance Review Prompt

Run this review with Claude Code CLI:
```bash
claude -p "$(cat docs/reviews/performance_review_prompt.md)"
```

Or use the slash command: `/performance-review`

---

## Instructions

Perform a comprehensive performance review of this codebase. Write your findings to `docs/reviews/reports/performance_review.md`.

## Step 0. Review Previous Report (If Exists)

Before starting the review, check if a previous report exists:

```bash
cat docs/reviews/reports/performance_review.md 2>/dev/null || echo "No previous report"
```

**If a previous report exists:**
1. Note the date and all findings from the previous report
2. Track which issues were identified previously
3. In your new report, include a "Changes Since Last Review" section that highlights:
   - Issues that have been remediated
   - Issues that remain unaddressed (escalate priority if aging)
   - New issues discovered
   - Performance improvements made
   - Overall performance trend (improving/stable/declining)

## Areas to Review

### 1. Database & Storage (server/storage/)

- **N+1 Queries**: Look for loops that execute queries inside them
- **Missing Indexes**: Check query patterns vs available indexes
- **Unbounded Queries**: Queries without LIMIT that could return large datasets
- **Connection Pooling**: Verify proper connection management
- **Query Complexity**: Identify expensive JOINs or subqueries

### 2. API Performance (server/services/)

- **Pagination**: Verify list endpoints use pagination
- **Batch Operations**: Look for opportunities to batch multiple operations
- **Caching Opportunities**: Identify frequently-accessed data that could be cached
- **Response Size**: Check for endpoints returning excessive data
- **Async Operations**: Identify blocking operations that could be async

### 3. Go Server Performance

- **Goroutine Leaks**: Check for goroutines that may not terminate
- **Channel Usage**: Verify channels are properly closed
- **Memory Allocation**: Look for unnecessary allocations in hot paths
- **Context Cancellation**: Verify operations respect context cancellation
- **Defer in Loops**: Check for defer statements inside loops

### 4. Flutter UI Performance (app/lib/)

- **Widget Rebuilds**: Look for unnecessary rebuilds (missing const, poor state management)
- **Build Method Complexity**: Large build methods that could be split
- **Image Loading**: Check for proper image caching and sizing
- **List Performance**: Verify ListView.builder usage for long lists
- **Animation Performance**: Check for expensive operations during animations

### 5. State Management (Flutter)

- **Provider Granularity**: Check if providers are too broad (causing unnecessary rebuilds)
- **Computed Values**: Look for expensive computations that could be memoized
- **Stream Subscriptions**: Verify streams are properly disposed
- **Memory Leaks**: Check for retained references preventing garbage collection

### 6. Network Performance

- **Request Batching**: Identify multiple sequential requests that could be batched
- **Payload Size**: Check for oversized request/response payloads
- **Compression**: Verify gzip/compression is enabled
- **Connection Reuse**: Check HTTP client configuration

### 7. Proto/Serialization

- **Large Messages**: Identify proto messages with excessive fields
- **Repeated Fields**: Check for unbounded repeated fields
- **Serialization Overhead**: Look for unnecessary serialization/deserialization

## Output Format

Write `docs/reviews/reports/performance_review.md` with:

```markdown
# Performance Review - [DATE]

## Executive Summary
[1-2 paragraph overview of performance posture]

## Changes Since Last Review

> *If no previous report exists, write "This is the first performance review."*

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

**Performance Improvements:**
- [New optimizations implemented since last review]

**Trend**: [Improving / Stable / Declining] - [Brief explanation]

---

## Critical Issues
[Issues causing significant performance degradation]

## High Priority
[Issues affecting user experience]

## Medium Priority
[Optimization opportunities]

## Low Priority
[Minor improvements and best practices]

## Positive Findings
[Well-optimized code worth highlighting]

## Recommendations
[Prioritized action items]
```

For each finding include:
- **Location**: File path and line number
- **Description**: What the issue is
- **Impact**: Estimated performance impact
- **Recommendation**: How to fix it with code examples where helpful
