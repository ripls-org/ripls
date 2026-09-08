---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Dependency review checklist — Go (go.mod) and Flutter (pubspec) dependencies, security/CVEs, health metrics, transitive analysis, breaking changes, update priority. Follow its "Areas to Review" section.
  triggers: [dependency, dependencies, package, cve, upgrade, go-mod, pubspec, renovate]
  lens: [dependency]
  globs: [go.mod, go.sum, app/pubspec.yaml, package.json]
  alwaysApply: true
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Dependency Review Prompt

Run this review with Claude Code CLI:

```bash
claude -p "$(cat docs/reviews/dependency_review_prompt.md)"
```

Or use the slash command: `/dependency-review`

---

## Instructions

Perform a comprehensive dependency review of this codebase. Write your findings to `docs/reviews/reports/dependency_review.md`.

## Step 0. Review Previous Report (If Exists)

Before starting the review, check if a previous report exists:

```bash
cat docs/reviews/reports/dependency_review.md 2>/dev/null || echo "No previous report"
```

**If a previous report exists:**

1. Note the date and key findings from the previous report
2. Track which issues were identified previously
3. In your new report, include a "Changes Since Last Review" section that highlights:
   - Issues that have been resolved
   - Issues that remain unaddressed
   - New issues discovered
   - Dependencies that were updated
   - Overall trend (improving/stable/declining)

## Steps

### 1. Gather Dependency Information

Run these commands to collect current state:

```bash
# Go dependencies with available updates
go list -m -u all

# Go dependency graph (direct vs transitive)
go mod graph | head -50

# Go module vulnerabilities (if govulncheck installed)
govulncheck ./...

# Flutter dependencies
cd app && flutter pub outdated

# Flutter dependency tree
cd app && flutter pub deps
```

### 2. Review Go Dependencies (go.mod)

**Check for:**

- **Major version updates**: Breaking changes that need migration
- **Security patches**: Minor/patch updates with security fixes
- **Deprecated packages**: Packages that are no longer maintained
- **Duplicate functionality**: Multiple packages doing the same thing
- **Indirect dependency bloat**: Large transitive dependency trees

**Key dependencies to monitor:**

- `connectrpc.com/connect` - RPC framework
- `google.golang.org/protobuf` - Proto runtime
- `github.com/jackc/pgx` - PostgreSQL driver
- `firebase.google.com/go` - Firebase SDK
- `cloud.google.com/go` - GCP SDK

**For each dependency with available updates:**

- Review the changelog/release notes for breaking changes
- Check migration guides if jumping major versions
- Note any deprecated APIs we currently use

### 3. Review Flutter Dependencies (app/pubspec.yaml)

**Check for:**

- **Major version updates**: Breaking API changes
- **Deprecated packages**: Packages marked as discontinued
- **Platform compatibility**: iOS/Android version requirements
- **Transitive dependency conflicts**: Version resolution issues
- **SDK constraints**: Flutter/Dart SDK version requirements

**Key dependencies to monitor:**

- `flutter_riverpod` - State management
- `go_router` - Navigation
- `connectrpc` - RPC client
- `firebase_*` - Firebase packages
- `google_sign_in` - Authentication

**For each dependency with available updates:**

- Check pub.dev for breaking change notices
- Review the package changelog
- Verify platform support (iOS min version, Android SDK level)

### Known Blockers (Do Not Flag as Issues)

The following dependency updates are blocked by upstream constraints. Monitor for resolution but do not flag as actionable issues:

| Package | Blocked Version | Blocker | Action |
|---------|-----------------|---------|--------|
| protobuf (Flutter) | 6.0.0 | `connectrpc ^1.0.0` requires `protobuf <5.0.0` | Monitor connectrpc releases for protobuf 6.x support |

When reviewing, check if these blockers have been resolved:
- Search pub.dev for connectrpc changelog/releases
- If a new version supports protobuf 6.x, remove from this list and flag as actionable

### 4. Security Analysis

**Vulnerability scanning:**

- Search GitHub Security Advisories for each major dependency
- Check NVD (National Vulnerability Database) for known CVEs
- Review `govulncheck` output for Go dependencies
- Search for "[package name] vulnerability" for recent reports

**For each major dependency, check:**

- Known CVEs affecting current or recent versions
- Security-related commits in recent releases
- Open security issues in the package's issue tracker
- Whether security issues are addressed promptly (response time)

**Red flags:**

- No commits in 12+ months
- Unaddressed security issues older than 90 days
- Archived/deprecated status
- Single maintainer with no recent activity
- History of slow security response

### 5. Dependency Health Metrics

**For critical dependencies, evaluate:**

| Metric            | Healthy    | Concerning  | Critical          |
| ----------------- | ---------- | ----------- | ----------------- |
| Last commit       | < 3 months | 3-12 months | > 12 months       |
| Open issues       | < 100      | 100-500     | > 500 unaddressed |
| Release frequency | Monthly    | Quarterly   | > 1 year          |
| Contributors      | > 5 active | 2-5 active  | 1 or archived     |
| Test coverage     | > 70%      | 40-70%      | < 40% or unknown  |
| Documentation     | Complete   | Partial     | Missing           |

**Check these sources:**

- GitHub repository activity (commits, issues, PRs)
- Package registry stats (pub.dev likes/points, pkg.go.dev imports)
- Community activity (Stack Overflow questions, Discord/Slack)

### 6. Transitive Dependency Analysis

**Identify:**

- **Deep dependency chains**: Dependencies with many levels of transitive deps
- **Diamond dependencies**: Same package required at different versions
- **Heavy dependencies**: Packages that pull in many transitive dependencies
- **Outdated transitive deps**: Security issues in indirect dependencies

**Go commands:**

```bash
# Show why a specific module is needed
go mod why -m <module>

# Show all modules and their dependencies
go mod graph

# Identify largest dependency trees
go list -m all | wc -l
```

**Flutter commands:**

```bash
# Full dependency tree
flutter pub deps

# Check for dependency conflicts
flutter pub outdated --dependency-overrides
```

### 7. Breaking Change Analysis

**For each major version update available:**

1. Read the full changelog between current and latest version
2. Identify all breaking changes and deprecations
3. Search our codebase for affected APIs: `grep -r "affectedFunction" server/`
4. Estimate migration effort (trivial/moderate/significant)
5. Note any required code changes

**Document in report:**

- Which APIs we use that are changing
- Required code modifications
- Whether migration can be done incrementally
- Any feature flags or compatibility shims available

### 8. Alternative Package Assessment

**For any dependency that is:**

- Unmaintained (no commits in 12+ months)
- Has unresolved security issues
- Is deprecated or archived
- Has problematic licensing

**Research alternatives:**

- Search for actively maintained alternatives
- Compare feature sets, performance, and community size
- Check if the alternative is a drop-in replacement
- Note migration effort required

### 9. Update Priority Assessment

**Critical (update immediately):**

- Security vulnerabilities with published CVE
- Security vulnerabilities actively being exploited
- Breaking bugs affecting production
- License compliance issues

**High (update this week):**

- Security patches without CVE but with known exploit
- Dependencies 2+ major versions behind
- Deprecated packages with maintained alternatives
- Packages with concerning health metrics

**Medium (update this month):**

- Minor version updates with useful features we need
- Performance improvements
- Dependencies 1 major version behind with clear migration path
- Packages approaching unmaintained status

**Low (update when convenient):**

- Patch version updates with only bug fixes
- Dev/test dependency updates
- Minor updates with no features we need

### 10. Update Testing Strategy

**For each update, document:**

- Unit tests that should be run
- Integration tests that should be run
- Manual testing scenarios
- Rollback plan if issues discovered

**Recommended testing approach:**

```bash
# Go: Run full test suite after updates
go test ./...

# Flutter: Run full test suite after updates
cd app && flutter test

# Integration tests
npm run test:integration
```

## Output Format

Write `docs/reviews/reports/dependency_review.md` with:

````markdown
# Dependency Review - [DATE]

## Executive Summary

**Overall Health**: [Healthy / Needs Attention / Critical]

**Key Findings:**

- X critical updates required
- X high priority updates recommended
- X packages need replacement due to maintenance status
- X license issues identified

**Immediate Actions Required:**

1. [Most urgent action]
2. [Second most urgent]

---

## Changes Since Last Review

> _If no previous report exists, write "This is the first dependency review."_

**Previous Review Date**: [DATE or N/A]

**Resolved Issues:**

- ✅ [Issue from previous report that has been fixed]
- ✅ [Package X updated from v1.0 to v2.0]

**Remaining Issues:**

- ⏳ [Issue still present from previous report]

**New Issues:**

- 🆕 [Issue not present in previous report]

**Dependencies Updated Since Last Review:**
| Package | Previous | Current | Notes |
|---------|----------|---------|-------|
| ... | v1.0.0 | v1.2.0 | Security patch applied |

**Trend**: [Improving / Stable / Declining] - [Brief explanation]

---

## Go Dependencies

### Critical Updates

| Package | Current | Latest | CVE/Issue | Impact | Migration Effort |
| ------- | ------- | ------ | --------- | ------ | ---------------- |
| ...     | ...     | ...    | CVE-XXXX  | High   | Trivial          |

### High Priority Updates

| Package | Current | Latest | Reason                  | Migration Effort |
| ------- | ------- | ------ | ----------------------- | ---------------- |
| ...     | ...     | ...    | 2 major versions behind | Moderate         |

### Medium Priority Updates

| Package | Current | Latest | Reason                   | Notes |
| ------- | ------- | ------ | ------------------------ | ----- |
| ...     | ...     | ...    | Performance improvements | ...   |

### Low Priority Updates

| Package | Current | Latest | Notes          |
| ------- | ------- | ------ | -------------- |
| ...     | ...     | ...    | Bug fixes only |

### Up to Date

- `package1` - v1.2.3
- `package2` - v2.0.0

### Dependency Health Concerns

| Package | Issue                   | Recommendation         |
| ------- | ----------------------- | ---------------------- |
| ...     | No commits in 18 months | Consider alternative X |

---

## Flutter Dependencies

### Critical Updates

| Package | Current | Latest | CVE/Issue | Impact | Migration Effort |
| ------- | ------- | ------ | --------- | ------ | ---------------- |
| ...     | ...     | ...    | ...       | ...    | ...              |

### High Priority Updates

| Package | Current | Latest | Reason | Migration Effort |
| ------- | ------- | ------ | ------ | ---------------- |
| ...     | ...     | ...    | ...    | ...              |

### Medium Priority Updates

| Package | Current | Latest | Reason | Notes |
| ------- | ------- | ------ | ------ | ----- |
| ...     | ...     | ...    | ...    | ...   |

### Low Priority Updates

| Package | Current | Latest | Notes |
| ------- | ------- | ------ | ----- |
| ...     | ...     | ...    | ...   |

### Up to Date

- `package1` - v1.2.3
- `package2` - v2.0.0

### Dependency Health Concerns

| Package | Issue | Recommendation |
| ------- | ----- | -------------- |
| ...     | ...   | ...            |

---

## Security Analysis

### Known Vulnerabilities

| Package | CVE           | Severity | Affected Versions | Fixed In | Our Version |
| ------- | ------------- | -------- | ----------------- | -------- | ----------- |
| ...     | CVE-XXXX-YYYY | Critical | < 1.2.3           | 1.2.3    | 1.2.0       |

### Packages with Security Concerns

| Package | Concern                | Risk Level | Recommendation  |
| ------- | ---------------------- | ---------- | --------------- |
| ...     | Slow security response | Medium     | Monitor closely |

### Supply Chain Risks

| Package | Risk              | Mitigation           |
| ------- | ----------------- | -------------------- |
| ...     | Single maintainer | Pin version, monitor |

---

## Deprecated/Unmaintained Packages

| Package | Status   | Last Activity | Alternative | Migration Effort |
| ------- | -------- | ------------- | ----------- | ---------------- |
| ...     | Archived | Jan 2023      | new-package | Moderate         |

---

## Transitive Dependency Issues

### Version Conflicts

| Package | Required By | Version A | Version B | Resolution |
| ------- | ----------- | --------- | --------- | ---------- |
| ...     | pkg1, pkg2  | 1.0.0     | 2.0.0     | Use 2.0.0  |

### Heavy Dependencies

| Package | Transitive Deps | Size Impact | Notes                        |
| ------- | --------------- | ----------- | ---------------------------- |
| ...     | 45 packages     | +2MB        | Consider lighter alternative |

---

## Breaking Change Migration Guide

### [Package Name] v1.x → v2.x

**Breaking Changes:**

1. `oldFunction()` renamed to `newFunction()`
2. `Config` struct field `X` removed

**Our Usage:**

- `server/services/example.go:45` - uses `oldFunction()`
- `server/config/config.go:12` - uses `Config.X`

**Migration Steps:**

1. Update imports
2. Rename function calls
3. Update config handling

**Estimated Effort:** Moderate (2-4 hours)

---

## Recommendations

### Immediate (This Week)

1. **[Action]** - [Reason] - [Specific command or steps]
2. ...

### Short Term (This Month)

1. **[Action]** - [Reason]
2. ...

### Long Term (This Quarter)

1. **[Action]** - [Reason]
2. ...

---

## Update Commands

### Critical Updates

```bash
# Package 1 - Security fix
go get package1@v1.2.3
go mod tidy

# Package 2 - Security fix
cd app && flutter pub upgrade package2
```
````

### High Priority Updates

```bash
# Go updates
go get package3@v2.0.0
go get package4@v1.5.0
go mod tidy

# Flutter updates
cd app && flutter pub upgrade package5 package6
```

### Bulk Update (After Testing)

```bash
# Update all Go dependencies to latest compatible
go get -u ./...
go mod tidy

# Update all Flutter dependencies
cd app && flutter pub upgrade
```

---

## Testing Checklist

After applying updates:

- [ ] `go test ./...` passes
- [ ] `flutter test` passes
- [ ] `npm run lint` passes
- [ ] Manual smoke test of critical paths
- [ ] No new deprecation warnings introduced

```

Include version numbers and specific commands for applying updates.
```
