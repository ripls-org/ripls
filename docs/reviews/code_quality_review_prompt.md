---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Code-quality review checklist — DRY, strong typing, comments/docs, dead code, TODO hygiene, naming, function/file sizing, typed errors, magic values, boolean params. Follow its "Areas to Review" section.
  triggers: [code-quality, dry, naming, dead-code, typing, todo, magic-number, file-size]
  lens: [code_quality]
  alwaysApply: true
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Code Quality Review Prompt

Run this review with Claude Code CLI:
```bash
claude -p "$(cat docs/reviews/code_quality_review_prompt.md)"
```

Or use the slash command: `/code-quality-review`

---

## Instructions

Perform a comprehensive code quality review of this codebase, focused on durable
best practices: DRY, strong typing, comments and documentation, style guide
enforcement, dead code, and TODO hygiene. Write your findings to
`docs/reviews/reports/code_quality_review.md`.

This review intentionally overlaps minimally with the dedicated reviews for
tech debt, security, testing, performance, error handling, observability,
accessibility, architecture, API design, and dependencies. When an issue
clearly belongs to one of those reviews, note it briefly and defer. When an
issue obviously straddles boundaries (e.g., DRY violations are also tech debt),
include it here and cross-reference.

**Bias toward patterns over individual nits.** A single missing file comment
isn't worth a finding; "30% of files in `server/services/` lack file comments"
is. Whenever a finding could be enforced or auto-fixed by a tool, recommend
the tool change rather than (or in addition to) the manual fix.

**Output is categorical, not severity-bucketed.** Group findings by principle
(DRY, Typing, Documentation, Style/Automation, Dead Code, TODO Hygiene,
Naming, Sizing, Error Typing, Magic Values, Boolean Parameters), not by
Critical/High/Medium/Low.

## Step 0. Review Previous Report (If Exists)

```bash
cat docs/reviews/reports/code_quality_review.md 2>/dev/null || echo "No previous report"
```

If a previous report exists, note paid-down items, items still outstanding,
and any new issues, in a "Changes Since Last Review" section.

## Areas to Review

### 1. DRY (Don't Repeat Yourself)

**Thresholds:**
- Code blocks of **>20 lines** repeated across files should almost always be
  shared via a library/helper.
- A pattern reused **3 or more times** should be factored, even if individual
  occurrences are short.
- **Tests are exempt** — prefer DAMP (Descriptive And Meaningful Phrases) over
  DRY in test bodies. Test fixtures and helpers should be shared, but readable
  inline setup beats a clever shared helper that hides what's being tested.

**What to look for:**
- Long copy-pasted blocks across server packages (`server/services/*`),
  across Flutter widgets/screens, or across viewmodels.
- Cross-language duplication where Go and Dart compute the same thing —
  flag it as a candidate for proto-defined logic, server-side computation,
  or generated code.
- Repeated proto-to-API conversion blocks (these are common and often a
  sign that a helper is missing).
- Repeated SQL filter/where construction.
- Repeated UI patterns in Flutter that should be extracted to widgets.

**Acceptable duplication:** generated code, near-identical proto messages
that are intentionally distinct per RPC (CLAUDE.md forbids reuse), test
fixtures.

**Tooling to recommend:**
- For Go: consider `dupl` (`go install github.com/mibk/dupl@latest`) or
  `golangci-lint`'s `dupl` linter.
- For Dart: there is no first-class duplication detector; recommend grep-based
  spot checks and mention `dart_code_metrics` if it surfaces useful signals.

### 2. Strong Typing

Strong typing moves errors from runtime to compile time. If JSON strings or
field labels are pulling the weight that types should carry, that is a bad
smell.

**Go antipatterns to flag:**
- `interface{}` / `any` outside of generic constraints, JSON marshaling
  boundaries, or genuinely heterogeneous containers. Prefer concrete types
  or sum types via interfaces with sealed methods.
- `map[string]any` or `map[string]interface{}` carrying structured data —
  prefer a struct (or a proto type if it crosses a wire).
- Stringly-typed identifiers: `string` parameters named `userID`, `gearID`,
  etc., where a named type (`type UserID string`) would prevent argument
  swapping at compile time.
- String-based enums where a typed enum (`type Status int` with iota, or a
  proto enum) would do.
- `error` checks via `strings.Contains(err.Error(), "...")` — should be
  `errors.Is` / `errors.As` against typed sentinels.
- Type switches on `any` to recover structure that should have been kept.
- Runtime reflection (`reflect`) used for things that could be statically
  typed (acceptable in framework/storage layers, suspicious elsewhere).
- Untyped struct tags or magic strings driving behavior (e.g., `db:"col"`
  where the struct could be the source of truth instead).
- `panic`/`recover` used as control flow rather than for genuinely
  unrecoverable states.

**Dart antipatterns to flag:**
- `dynamic` outside of JSON parsing or proto interop edges.
- `Map<String, dynamic>` carrying structured data — prefer a class or a
  generated proto message.
- `as` casts without prior `is` checks (or that could be replaced by
  pattern matching).
- Nullable types where non-null is the invariant, or non-nullable types
  with `!` bang operators that should be properly null-checked.
- String-keyed enum-like lookups instead of `enum` types.
- `late` variables that could be `final` with constructor initialization.

**Proto antipatterns to flag:**
- String fields that carry an enumerated set of values — should be
  `enum` types.
- `bytes` fields carrying structured data that should be a typed message.
- Missing `optional` markers on conditionally-present fields (CLAUDE.md
  rule — see "Proto Field Conventions").
- Reused request/response messages across RPCs (CLAUDE.md violation).
- API protos importing from `models/` or vice versa (CLAUDE.md violation).

**The rule:** string-keyed lookups for things with a known fixed set of
values are not strictly forbidden, but each occurrence should have a
clear justification. If you can't articulate why a string is being used
in place of a type, recommend converting it.

### 3. Comments & Documentation

**File-level comments:**
- Every non-trivial source file should have a top-of-file comment
  explaining what the file does. Required across all languages.
- **Exempt:** test files (`*_test.go`, `*_test.dart`), generated files
  (anything in `server/gen/`, `app/lib/data/gen/`, `*.pb.go`,
  `*.pb.dart`, `*.g.dart`, `*.mocks.dart`), build artifacts.
- **Single-symbol files:** if the file contains exactly one important
  exported symbol (a single type, a single entrypoint function), a doc
  comment on that symbol is sufficient — no separate file comment needed.
- File comments should describe **purpose, key invariants, and gotchas**,
  not implementation steps.

**Folder READMEs:**
- Every meaningful module boundary should have a `README.md` summarizing
  the package's purpose, the main entry points, and how it fits into the
  broader system.
- Required at: every Go package directory under `server/` that contains
  more than one file (e.g., `server/services/community/`,
  `server/storage/`, `server/impact_metrics/`); every Flutter feature directory
  under `app/lib/data/`, `app/lib/services/`, `app/lib/presentation/`
  that groups multiple files.
- Not required at: deep leaf directories that hold a single file, vendor
  or generated directories, or pure entrypoint directories like
  `server/cmd/`.

**API vs implementation comments (cross-language):**
- API/declaration comments describe the **contract** seen by callers —
  what the function does, what it returns, what it requires, what it
  guarantees. They must not leak implementation: vendor names, transport
  details, internal pipeline mechanics, sentinel values, server-side
  fan-out, caching behavior, etc.
- Implementation comments describe **how** — they belong inside function
  bodies, on private types, or on the implementation side of a
  declare/implement split.
- For Go and Dart, which lack a separate declare/implement file split,
  put implementation notes inside the function body or in a private
  helper. If a non-obvious implementation detail must live on the
  declaration (e.g., on an interface method that has only one
  implementation), call it out as `Implementation note:` so the contract
  vs. note distinction is explicit.
- For protos: enforce the rules in CLAUDE.md "Proto Comments" section
  — flag any vendor names, transport plumbing, server mechanics, or
  internal sentinel values appearing in `.proto` files.

**Stale and dead comments:**
- Comments that describe code that no longer exists or no longer matches.
- Comments referencing ephemeral context: phase numbers, PR numbers, issue
  numbers, branch names ("new in this refactor", "Phase 3a", "pre-rewrite").
  CLAUDE.md forbids these in source comments.

### 4. Style Guides & Automation

**The principle:** follow the most widely adopted style guide for each
language and enforce compliance with automated tools, not human review.

**Audit existing config and critique gaps:**

For Go:
- Read `.golangci.yaml` (or `.golangci.yml`) and the Makefile/`package.json`
  scripts that invoke it. List which linters are enabled.
- Compare against the recommended set: `gofumpt`, `goimports`, `govet`,
  `staticcheck`, `revive`, `gocritic`, `errcheck`, `ineffassign`,
  `unused`, `unparam`, `prealloc`, `gosec`, `bodyclose`, `nilerr`,
  `nilnil`, `errorlint`, `wrapcheck`, `godot`, `misspell`, `dupl`,
  `gocyclo` or `cyclop`, `funlen`, `lll`.
- Flag any commonly recommended linter that is not enabled, with a
  one-line rationale for why enabling it would help this codebase.

For Dart:
- Read `analysis_options.yaml`. List which lint sets it extends
  (`flutter_lints`, `lints/recommended.yaml`, `very_good_analysis`).
- Recommend `very_good_analysis` if the project uses only the default
  Flutter lints — it is the most comprehensive widely-adopted Dart lint
  set.
- Check `analyzer.errors:` overrides — flag any rule downgraded from
  `error` to `warning` or `ignore` without justification.

For protos:
- buf STANDARD is already enabled. Consider whether `buf.yaml` should
  add `COMMENTS` rules (e.g., `COMMENT_FIELD`, `COMMENT_MESSAGE`,
  `COMMENT_SERVICE`) to require doc comments on protos.
- Check `buf.gen.yaml` for any `disable_lint` overrides.

**Disabled lints in source:**
- Search for `//nolint`, `// nolint`, `// ignore:`, `// ignore_for_file:`,
  `// dart-lint:`, and similar suppressions across the codebase.
- A handful of justified suppressions is fine. A pattern of suppressions
  for the same rule is a signal — either the rule should be globally
  disabled (with a note) or the underlying code should change.
- Every suppression should have a comment explaining why; flag any
  suppression without a justification.

```bash
grep -rn "//nolint\|// nolint\|// ignore:\|// ignore_for_file:" \
  server/ app/lib/ --include="*.go" --include="*.dart" | head -200
```

**Formatting:**
- Confirm `gofmt`/`gofumpt` runs as part of `npm run build` or pre-commit.
- Confirm `dart format` runs as part of CI.
- Recommend a pre-commit hook if formatting is only enforced in CI.

### 5. Dead Code

All forms of dead code should be flagged for removal — version control
preserves anything we might want back.

- **Commented-out code blocks** of any size. Search:
  ```bash
  grep -rn "^[[:space:]]*//.*[a-zA-Z(){};]" server/ app/lib/ \
    --include="*.go" --include="*.dart" | grep -v "^[^:]*:[0-9]*://[[:space:]]*[A-Z]"
  ```
  (Heuristic: comments that look like code rather than prose.)
- **Unused exported symbols** — for Go, run `staticcheck -unused.whole-program=true`
  or rely on `unused` linter. For Dart, run `dart analyze` and check for
  `unused_element` warnings.
- **Unreferenced files** — files not imported anywhere in the build.
- **Unreachable branches** — `if false`, code after unconditional
  `return`/`panic`/`throw`, dead `default` cases.
- **Disabled tests** — `t.Skip(...)`, `@Skip(...)`, `skip:` annotations
  without an issue link. (These also belong in the testing review;
  cross-reference rather than re-enumerate.)

### 6. TODO Hygiene

If something is temporary, mark it with a TODO comment linked to a GitHub
issue. Don't let TODOs fester.

**Format:** `TODO(#NNN): short description` where `#NNN` is a GitHub issue
number. The username form (`TODO(username)`) is acceptable for very
short-lived TODOs that will be resolved in the same PR, but anything
intended to outlive the PR must reference an issue.

**What to flag:**
- TODOs without an issue link.
- TODOs referencing issues that are already closed (suggests the TODO
  should be removed or the issue reopened).
- TODOs older than ~90 days (estimated via file modification date or
  `git blame`).
- `FIXME`, `HACK`, `XXX`, `WORKAROUND` markers — these should either be
  promoted to a linked TODO or fixed.

```bash
grep -rn "TODO\|FIXME\|HACK\|XXX\|WORKAROUND" server/ app/lib/ \
  --include="*.go" --include="*.dart"
```

For each TODO, check whether the referenced GitHub issue (if any) is
still open:
```bash
gh issue view NNN --json state,title 2>/dev/null
```

### 7. Naming Consistency

Same concept should be named the same way everywhere.

- The same identifier expressed differently across boundaries:
  `userId` / `user_id` / `uid` / `userID` referring to the same value
  in different files.
- Inconsistent casing for similar concepts (proto `snake_case`, Go
  `CamelCase`, Dart `camelCase` are all correct *within* their language;
  the issue is when the *concept* drifts: `gear_id` in proto becoming
  `itemId` in Dart).
- Boolean naming inconsistency (`isActive` vs. `active` vs. `enabled`
  for the same concept).
- Acronym capitalization drift (`URL` vs. `Url`, `ID` vs. `Id`) — pick
  the language convention and stick to it.

### 8. Function & File Sizing

- **Files:** CLAUDE.md says <1000 lines preferred. Flag any file >1000
  lines and recommend a split. Flag any file >500 lines that lacks clear
  internal sectioning.
- **Functions:** flag functions >100 lines or with cyclomatic complexity
  >15 (use `gocyclo` or `cyclop` for Go, `dart_code_metrics` for Dart
  if available).
- **Types:** flag structs/classes with >20 fields — usually a sign of
  missing sub-types.

### 9. Typed Errors

- Go: errors should be wrapped with `fmt.Errorf("operation failed: %w", err)`
  and matched with `errors.Is` / `errors.As`. Flag bare `errors.New`
  for sentinel errors that aren't exported as named values, and flag
  any `err.Error() == "..."` or `strings.Contains(err.Error(), ...)`.
- Define typed error variants (`var ErrNotFound = errors.New(...)`) at
  package scope and reference them from callers.
- Dart: use typed exception classes, not raw `Exception` or `String`
  thrown values. Flag `throw 'string'` and `catch (e)` without typed
  narrowing.
- Confirm error messages include enough context to debug from a log
  line alone (operation name, key identifiers — masked when sensitive).

### 10. Magic Numbers and Strings

- Inline numeric literals other than `0`, `1`, `-1`, `2` in domain logic
  should be named constants.
- Inline string literals used as keys, identifiers, or sentinel values
  should be named constants.
- Acceptable inline literals: test data, error messages, log messages,
  format strings, obvious unit conversions (`* 1000` for ms→s with a
  named result).
- Recommend `goconst` linter for Go to surface repeated literals that
  should be constants.

### 11. Boolean Parameters

- Functions taking multiple `bool` parameters (`foo(true, false, true)`)
  are unreadable at call sites. Flag any function with 2+ bool params
  and recommend an options struct or enum.
- Single-bool parameters are tolerable but prefer a named enum when the
  bool represents a mode (`AuthMode.required` vs. `true`).
- Search:
  ```bash
  grep -rEn "func [A-Za-z]+\([^)]*bool[^)]*bool" server/ --include="*.go"
  ```

## Output Format

Write `docs/reviews/reports/code_quality_review.md` with:

```markdown
# Code Quality Review - [DATE]

## Executive Summary
[1-2 paragraph overview of the codebase's quality posture, calling out
the 2-3 most impactful patterns to address.]

## Changes Since Last Review

> *If no previous report exists, write "This is the first code quality review."*

**Previous Review Date**: [DATE or N/A]

**Resolved:**
- ✅ [Issue from previous report]

**Still Outstanding:**
- ⚠️ [Pattern still present]

**New:**
- 🆕 [Newly identified]

**Trend**: [Improving / Stable / Declining]

---

## Findings by Category

### DRY
[Patterns of duplication. Each finding: what's duplicated, where (file:line
for representative occurrences and a count of total occurrences), why it
matters, recommended factoring. Include any tooling recommendations.]

### Strong Typing
[Antipatterns observed, grouped by language (Go / Dart / Proto). Each
finding: pattern, representative occurrences with file:line, count,
recommended typed alternative.]

### Comments & Documentation
- File-level comments: [coverage stats, e.g., "412 of 580 non-test,
  non-generated files have file comments (71%); gaps concentrated in
  X, Y, Z"]
- Folder READMEs: [list of folders missing a README that should have one]
- API vs implementation leaks: [examples with file:line]
- Stale/ephemeral comments: [examples with file:line]

### Style Guides & Automation
- Go lint config audit: [current state, recommended additions, with
  rationale per addition]
- Dart lint config audit: [same]
- Proto lint config audit: [same]
- Disabled-lint patterns: [rules suppressed >N times, with locations]
- Formatting enforcement: [where it runs, gaps]

### Dead Code
[Commented-out blocks, unused exports, unreachable branches, disabled
tests. file:line for each cluster.]

### TODO Hygiene
| TODO | File:Line | Linked Issue | Issue State | Age |
|------|-----------|--------------|-------------|-----|
| ...  | ...       | #123         | open/closed | Xd  |

[Summary: total TODOs, % with issue links, count older than 90d, count
referencing closed issues.]

### Naming Consistency
[Concepts named differently across boundaries, with examples.]

### Function & File Sizing
| File | Lines | Recommendation |
| Function | Lines | Cyclomatic | Recommendation |

### Typed Errors
[Bare error patterns, string-matched errors, untyped catches.]

### Magic Numbers and Strings
[Repeated literals that should be constants.]

### Boolean Parameters
[Functions with 2+ bool params; recommend options struct/enum.]

## Tooling Recommendations

[Consolidated list of tooling changes: linters to enable, configs to
update, pre-commit hooks to add. Each item with effort estimate and
expected impact.]

## Recommendations

[Prioritized action items. Group by effort: quick wins (hours),
medium efforts (days), large efforts (weeks). For patterns, prefer
"enable linter X and fix the 30 resulting findings" over "fix these
30 findings manually" — automation lasts.]
```

For each finding include:
- **Pattern**: What the issue is (the recurring shape, not an individual instance)
- **Locations**: file:line for representative occurrences; total count
- **Why it matters**: Concrete impact (compile-time safety lost, readability,
  maintenance cost, bug surface)
- **Recommendation**: Tooling change preferred; manual fix as fallback
- **Effort**: Estimated effort (hours/days)
