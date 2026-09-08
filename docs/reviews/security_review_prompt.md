---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Security review checklist — auth/authz, input validation/injection, secrets management, API and client security, cryptography, dependency risk. Follow its "Areas to Review" section.
  triggers: [security, auth, authz, ssrf, injection, secret, pii, crypto, idor]
  lens: [security]
  alwaysApply: true
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Security Review Prompt

Run this review with Claude Code CLI:
```bash
claude -p "$(cat docs/reviews/security_review_prompt.md)"
```

Or use the slash command: `/security-review-custom`

---

## Instructions

Perform a comprehensive security audit of this codebase. Write your findings to `docs/reviews/reports/security_review.md`.

## Step 0. Review Previous Report (If Exists)

Before starting the review, check if a previous report exists:

```bash
cat docs/reviews/reports/security_review.md 2>/dev/null || echo "No previous report"
```

**If a previous report exists:**
1. Note the date and all findings from the previous report
2. Track which vulnerabilities were identified previously
3. In your new report, include a "Changes Since Last Review" section that highlights:
   - Vulnerabilities that have been remediated
   - Vulnerabilities that remain unaddressed (escalate priority if aging)
   - New vulnerabilities discovered
   - Security improvements made
   - Overall security trend (improving/stable/declining)

## Areas to Review

### 1. Authentication & Authorization (server/auth/)

- **JWT Implementation**: Review token generation, validation, expiry handling
- **OIDC Integration**: Check OAuth flow, state parameter usage, token exchange
- **Session Management**: Look for session fixation, improper invalidation
- **Permission Checks**: Verify `auth.RequireAuth()` usage on all protected endpoints
- **Role-Based Access**: Check authorization logic in service handlers

### 2. Input Validation & Injection

- **SQL Injection**: Review `server/storage/` for parameterized queries
  - Since #2795 this sweep is CI-gated by `npm run lint:go:sql`
    (`server/cmd/check-sql`): values must be bound with `Placeholder(n)`,
    identifiers must go through `quoteIdent`, and nothing else may enter a
    query string. Do not re-read every `Query*`/`Exec*` call — the gate does.
  - **Do** review every `// sql-fragment-allow:` marker: each one is a human
    claim that a raw fragment cannot carry caller input, and the checker takes
    it on trust. `grep -rn 'sql-fragment-allow:' server/` lists them.
  - **Do** check that gosec's G201/G202 are still enabled and still finding
    nothing for the documented reason (interface-typed executor), not because
    someone re-added an exclude.
- **Command Injection**: Search for `exec.Command`, `os.StartProcess`
- **Path Traversal**: Check file operations for `../` or absolute path handling
- **Proto Validation**: Verify input validation on RPC handlers

### 3. Secrets Management

- **Hardcoded Secrets**: Search for API keys, passwords, tokens in code
  - Check for common patterns: `apikey`, `secret`, `password`, `token`
  - Review config files and environment variable handling
- **Logging**: Ensure secrets aren't logged in error messages
- **Storage**: Verify secrets use proper secret management (not plaintext)

### 4. API Security

- **Authentication Gaps**: Find endpoints missing `auth.RequireAuth()`
- **Authorization Bypass**: Check if users can access others' resources
- **Rate Limiting**: Look for rate limiting implementation
- **Error Disclosure**: Ensure errors don't leak sensitive info

### 5. Client Security (app/)

- **Token Storage**: How are auth tokens stored? (secure storage vs plaintext)
- **Sensitive Data**: Check for PII in local storage or logs
- **Certificate Pinning**: Is TLS properly configured?
- **Deep Link Handling**: Check for open redirect vulnerabilities

### 6. Cryptography

- **Algorithm Choices**: Check for weak algorithms (MD5, SHA1 for security)
- **Random Number Generation**: Verify `crypto/rand` usage, not `math/rand`
- **Key Management**: How are encryption keys stored/rotated?

### 7. Dependencies

- **Known Vulnerabilities**: Check go.mod and pubspec.yaml for CVEs
- **Outdated Packages**: Identify security-critical outdated dependencies

## Output Format

Write `docs/reviews/reports/security_review.md` with:

```markdown
# Security Review - [DATE]

## Executive Summary
[1-2 paragraph overview of security posture]

## Changes Since Last Review

> *If no previous report exists, write "This is the first security review."*

**Previous Review Date**: [DATE or N/A]

**Remediated Vulnerabilities:**
- ✅ [Vulnerability from previous report that has been fixed]
  - Location: [file:line]
  - Fixed in: [commit/PR reference if known]

**Unaddressed Vulnerabilities:**
- ⚠️ [Vulnerability still present from previous report]
  - Original Priority: [Critical/High/Medium/Low]
  - Current Priority: [May escalate if aging]
  - Days Open: [X days]

**New Vulnerabilities:**
- 🆕 [Vulnerability not present in previous report]

**Security Improvements:**
- [New security measures implemented since last review]

**Trend**: [Improving / Stable / Declining] - [Brief explanation]

---

## Critical Issues
[Issues requiring immediate attention]

## High Priority
[Significant vulnerabilities]

## Medium Priority
[Issues to address soon]

## Low Priority
[Minor concerns and hardening opportunities]

## Positive Findings
[Security measures that are working well]

## Recommendations
[Prioritized action items]
```

For each finding include:
- **Location**: File path and line number
- **Description**: What the issue is
- **Impact**: What could happen if exploited
- **Recommendation**: How to fix it
