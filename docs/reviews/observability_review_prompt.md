---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Observability review checklist — server logging, request tracing, metrics, health checks, alerting, debug capability, client observability, audit logging. Follow its "Areas to Review" section.
  triggers: [observability, logging, tracing, metrics, health, alerting, audit-log]
  lens: [observability]
  alwaysApply: true
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Observability Review Prompt

Run this review with Claude Code CLI:
```bash
claude -p "$(cat docs/reviews/observability_review_prompt.md)"
```

Or use the slash command: `/observability-review`

---

## Instructions

Perform a comprehensive observability review of this codebase. Write your findings to `docs/reviews/reports/observability_review.md`.

## Step 0. Review Previous Report (If Exists)

Before starting the review, check if a previous report exists:

```bash
cat docs/reviews/reports/observability_review.md 2>/dev/null || echo "No previous report"
```

**If a previous report exists:**
1. Note the date and all findings from the previous report
2. Track which issues were identified previously
3. In your new report, include a "Changes Since Last Review" section that highlights:
   - Issues that have been remediated
   - Issues that remain unaddressed
   - New issues discovered
   - Observability improvements made
   - Overall trend (improving/stable/declining)

## Areas to Review

### 1. Logging (server/)

- **Log Levels**: Check appropriate use of log levels (debug, info, warn, error)
- **Structured Logging**: Verify logs use structured format (JSON)
- **Context Fields**: Check logs include request ID, user ID, etc.
- **Error Logging**: Verify errors are logged with stack traces/context
- **Sensitive Data**: Ensure logs don't contain passwords, tokens, PII

### 2. Request Tracing

- **Request IDs**: Check for request ID generation and propagation
- **Distributed Tracing**: Look for trace context propagation
- **Span Creation**: Check for spans around key operations
- **Trace Context**: Verify context is passed through async operations

### 3. Metrics

- **Request Metrics**: Check for request count, latency, error rate metrics
- **Business Metrics**: Look for business-relevant metrics (users, transactions)
- **Resource Metrics**: Check for CPU, memory, connection pool metrics
- **Custom Metrics**: Identify opportunities for application-specific metrics

### 4. Health Checks

- **Liveness Probe**: Check for liveness endpoint
- **Readiness Probe**: Verify readiness checks include dependencies
- **Dependency Checks**: Check database, external service health
- **Startup Probes**: Look for slow-starting component handling

### 5. Alerting Foundations

- **Error Rates**: Check if error rates are measurable/alertable
- **Latency**: Verify latency is tracked for SLO alerting
- **Saturation**: Look for queue depth, connection pool metrics
- **Business Events**: Check for alertable business events

### 6. Debug Capabilities

- **Debug Endpoints**: Check for debug/profiling endpoints (secured)
- **Log Level Control**: Verify runtime log level adjustment
- **Request Sampling**: Look for detailed request logging capability
- **State Inspection**: Check for admin endpoints to inspect state

### 7. Client Observability (app/)

- **Error Reporting**: Check for crash reporting integration
- **Analytics Events**: Verify key user actions are tracked
- **Performance Monitoring**: Look for app performance metrics
- **Network Logging**: Check for API call logging/timing

### 8. Audit Logging

- **Security Events**: Check login, logout, permission changes are logged
- **Data Changes**: Verify significant data mutations are logged
- **Admin Actions**: Check admin operations are audited
- **Retention**: Look for log retention considerations

## Output Format

Write `docs/reviews/reports/observability_review.md` with:

```markdown
# Observability Review - [DATE]

## Executive Summary
[1-2 paragraph overview of observability posture]

## Changes Since Last Review

> *If no previous report exists, write "This is the first observability review."*

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
- [Observability improvements since last review]

**Trend**: [Improving / Stable / Declining] - [Brief explanation]

---

## Observability Maturity

| Pillar | Status | Coverage | Notes |
|--------|--------|----------|-------|
| Logging | 🟢/🟡/🔴 | X% | ... |
| Metrics | 🟢/🟡/🔴 | X% | ... |
| Tracing | 🟢/🟡/🔴 | X% | ... |
| Alerting | 🟢/🟡/🔴 | X% | ... |

## Critical Issues
[Blind spots that prevent debugging production issues]

## High Priority
[Significant observability gaps]

## Medium Priority
[Missing observability for important flows]

## Low Priority
[Nice-to-have improvements]

## Positive Findings
[Well-instrumented areas worth highlighting]

## Recommendations
[Prioritized action items]
```

For each finding include:
- **Location**: File path and line number
- **Description**: What's missing or incorrect
- **Impact**: How it affects debugging/monitoring
- **Recommendation**: How to add observability with examples
