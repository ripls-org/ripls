# Issue #1428: Promote revive.exported to error severity for exported Go symbols

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1428
**Filed:** 2026-04-26 | **Last activity:** 2026-04-27
**Labels:** documentation | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`server/.golangci.yml` currently runs `revive.exported` at `severity: warning`,
so any new violation is logged but does not fail the build. The sister rule
`revive.package-comments` was promoted to `severity: error` in PR #1377; the
`exported` rule was deferred to keep that PR's scope bounded. While the rule
sits at warning, drift accumulates with every new export.

`revive.exported` covers two distinct checks: (a) doc-comment presence on
exported `func`/`type`/`const`/`var`, and (b) name-stuttering (e.g.
`admin.AdminService`). The issue body anticipated a "much larger surface" of
remediation, primarily for missing doc comments.

**Empirical scope check.** Running the project's `golangci-lint` config today
surfaces only **7** revive `exported` violations across the entire `server/`
tree, **all of them name-stuttering, zero of them missing-comment**. The
codebase is already documentation-clean (PR #1377 shipped most of that). The
remaining work is the seven renames listed below plus the one-line config
flip — substantially smaller than the issue body assumed.

The 7 violations:

| # | File:line | Symbol | Linter recommendation |
|---|-----------|--------|------------------------|
| 1 | `services/admin/admin_service.go:8` | `type AdminService` | rename to `Service` |
| 2 | `services/experience/prompts.go:4` | `func ExperienceFromTextPrompt` | rename to `FromTextPrompt` |
| 3 | `services/experience/prompts.go:72` | `func ExperienceFromTextPromptPast` | rename to `FromTextPromptPast` |
| 4 | `services/experience/prompts.go:97` | `func ExperienceFromImagePrompt` | rename to `FromImagePrompt` |
| 5 | `product/lookup.go:21` | `type ProductInfo` | rename to `Info` |
| 6 | `story/types.go:12` | `type StoryCreator` (interface) | rename to `Creator` |
| 7 | `story/templates.go:9` | `type StoryTemplate` (struct) | rename to `Template` |

## Context from Comments

The single comment on the issue is `@claude` summoning this run; no human
discussion has shaped the plan further. Salient context lives entirely in the
issue body:

- The author called out the "audit existing violations" step as the bulk of
  the work, expecting a multi-file remediation PR.
- The author offered an alternate path: a `nolint:revive` suppression sweep
  with incremental cleanup. That alternate is now unnecessary at the original
  scale because the actual violation count is 7, not "every undocumented
  exported symbol."
- The author explicitly scoped this issue to **server Go**, separate from
  per-file Dart headers (#1427).
- "Deferred From PR #1377": `revive.package-comments` was promoted in that
  PR; this is the follow-on for the related rule.

## Potential Solutions

### Option A: Promote rule + rename all 7 stuttering symbols (recommended)
- **Approach:** Flip `severity: warning` → `severity: error` for `revive.exported` in `server/.golangci.yml`. Apply the linter's suggested renames to all 7 violations, updating every call site (struct types include their constructors and method receivers; interfaces include all `package.Type` references; functions include their tests). Use `gopls`-style rename semantics: rename the declaration and all references in one sweep per symbol.
- **Pros:** Sustainable — no suppressions accrue. Removes name-stutter that the rule explicitly flags as a readability problem. Aligns the codebase with Go style (`io.Reader`, `http.Client`, `bytes.Buffer`, never `bytes.BytesBuffer`).
- **Cons:** Touches a known-bounded but non-trivial set of files (admin: ~6, experience: ~3, product: ~2, story: ~6 files affected by cross-package usage; experience prompt funcs are in-package only). Test files included.
- **Effort:** Small-medium. ~25–35 file edits, all mechanical renames.

### Option B: Promote rule + suppress all 7 with `//nolint:revive`
- **Approach:** Same config flip. Add `//nolint:revive // <reason>` directives on the 7 declarations to suppress only the `exported` rule for each.
- **Pros:** Smallest diff. No call-site churn.
- **Cons:** Defeats the spirit of the promotion — the codebase ships the rule at error severity but with a hard-coded exemption list. Suppressions tend to multiply once introduced. The point of moving to error is to *force* the resolution, not annotate around it. The author's own scope note offered this as a fallback for a much larger remediation, not as a primary path for 7 items.
- **Effort:** Trivial. ~8 file edits.

### Option C: Promote rule + mix of renames and unexports
- **Approach:** Same config flip. For symbols used only within their own package, *unexport* them (lowercase first letter) instead of renaming — this also clears the rule because the rule applies only to exported names. For cross-package symbols, rename per Option A.
  - In-package only (unexport): `experience.{ExperienceFromTextPrompt,ExperienceFromTextPromptPast,ExperienceFromImagePrompt}` (only used inside `services/experience/`), `story.StoryTemplate` (only used inside `story/`).
  - Cross-package (rename): `admin.AdminService`, `product.ProductInfo`, `story.StoryCreator`.
- **Pros:** Reduces the package's exported surface to what's actually consumed across boundaries — a cleaner outcome than renaming things that should never have been exported. Matches the Go convention "export the minimum."
- **Cons:** Mixed strategy is harder to explain in the commit message and review. Slightly larger reasoning load per symbol. Test files in the same package can still call unexported funcs/types, so test impact is minor.
- **Effort:** Small-medium. Same order of magnitude as Option A; slightly less external diff because the in-package renames don't ripple.

## Recommendation

**Option C.** It produces the right end-state: every exported symbol earns its
export by being used across a package boundary. The four in-package-only
symbols (`ExperienceFromTextPrompt`, `ExperienceFromTextPromptPast`,
`ExperienceFromImagePrompt`, `StoryTemplate`) become unexported; the three
genuine cross-package symbols are renamed to drop the stutter. This both
satisfies the linter and removes accidental exports — strictly better than
Option A on long-term hygiene, with effectively the same effort.

If reviewers prefer the simpler narrative of "always rename, never unexport,"
fall back to **Option A**.

Do **not** take **Option B**. Suppressions on day one of an error-severity
promotion are an anti-pattern.

## Open Questions

- **Q1: Should we also update CLAUDE.md?**
  Recommendation: **Yes, briefly.** Add a one-paragraph entry under "Style Guidelines" / "Go Code Comments" reminding contributors that exported Go symbols must (a) have a doc comment that starts with the symbol name and (b) avoid stuttering with the package name (`bytes.Buffer`, not `bytes.BytesBuffer`). Reference `revive.exported` enforcement so authors know the build will fail. Keep it under 5 lines — the rule is largely self-enforcing now that it's at error severity.

- **Q2: Will renaming `experience.ExperienceFromTextPrompt` etc. to unexported break integration tests?**
  Recommendation: **No, but verify.** The `gen_ai_suggested_time_test.go` file lives in `package experience` (not `experience_test`), so it can call unexported names directly. Confirm by reading the test file's package declaration before renaming; if it turns out to be `experience_test`, fall back to renaming exported (`FromTextPrompt`, etc.) per Option A for these three. Same check applies to `product/lookup_test.go` and `story/templates_test.go` (if any).

- **Q3: Does renaming `admin.AdminService` interact with proto-generated code?**
  Recommendation: **No.** The proto service is also called `AdminService`, which means Buf generates `apiconnect.NewAdminServiceClient` and `apiconnect.NewAdminServiceHandler`. Those symbols are independent of the Go struct's name in `server/services/admin/`; they are derived from the proto `service AdminService` declaration and live in `server/gen/`. Renaming the Go struct from `AdminService` → `Service` does not touch the proto, the generated handler, the wire format, or any client. Verified by grepping: no Go file outside `server/gen/` reads or writes the proto-derived `apiconnect.AdminService*` names except via the constructor pattern, which is unaffected by the struct rename.

- **Q4: Do we need to rename source files alongside the symbols?**
  Recommendation: **No.** Filenames don't affect the linter. Leave `admin_service.go`, `lookup.go`, `types.go`, `templates.go`, `prompts.go` as-is to keep the diff focused. A future cleanup PR can rename `admin_service.go` → `service.go` if anyone cares. Not in scope here.

- **Q5: Should we also rename `NewAdminService` → `NewService` (for consistency with `admin.Service`)?**
  Recommendation: **Yes.** Idiomatic Go pairs `Service` with `NewService`. The function name doesn't itself stutter (`admin.NewAdminService` reads fine), but keeping `NewAdminService` returning a `*Service` is jarring. Update all call sites in `main.go`, `cleanup.go`, and `cleanup_test.go`. Same logic for `story.StoryCreator` interface — there is no `NewStoryCreator` to rename, but check `story/generator.go` (`NewGenerator`?) for any constructor whose name encodes `StoryCreator`.

## Implementation Plan

### Phase 1: Config flip + CLAUDE.md note
**Goal:** Change the lint severity and document the expectation, separately from the symbol renames so the diff is reviewable as two distinct concerns.

**Tasks:**
- [ ] Edit `server/.golangci.yml`: change the `exported` rule's `severity: warning` to `severity: error` (single-line change at line 38).
- [ ] Edit `CLAUDE.md`: add a short bullet under "Go Code Comments" stating that exported symbols (a) need a doc comment starting with the name and (b) must avoid package-name stutter, enforced by `revive.exported` at error severity.
- [ ] Confirm `npm run build` *fails* on this branch with the 7 expected violations (proves the promotion is active).

**Validates:** The lint rule is wired to fail. CLAUDE.md communicates the convention.

### Phase 2: In-package renames (unexport)
**Goal:** Reduce accidental exports for symbols used only within their own package.

**Tasks:**
- [ ] `server/services/experience/prompts.go`: rename `ExperienceFromTextPrompt` → `fromTextPrompt`. Update its doc comment to start with `fromTextPrompt`. Update all call sites (only `gen_ai_suggested_time_test.go`, confirmed in-package).
- [ ] Same file: rename `ExperienceFromTextPromptPast` → `fromTextPromptPast`. Update doc comment + test references in `gen_ai_suggested_time_test.go`.
- [ ] Same file: rename `ExperienceFromImagePrompt` → `fromImagePrompt`. Update doc comment.
- [ ] `server/story/templates.go`: rename `StoryTemplate` (struct) → `storyTemplate`. Update all references in `templates.go` (the `getTemplateForStoryType` return type and 6 struct literals). No external references — confirmed by `grep -rn StoryTemplate`.
- [ ] Pre-flight check: re-confirm test file package declarations (`package experience` vs `package experience_test`) for `services/experience/gen_ai_suggested_time_test.go`. If any test is `_test`-suffixed, that symbol must instead be renamed-and-kept-exported (e.g. `FromTextPrompt`) — call this out in the PR description and adjust.

**Validates:** Build still passes; the four warnings disappear from `golangci-lint run`.

### Phase 3: Cross-package renames
**Goal:** Drop the stutter on three genuinely cross-package symbols.

**Tasks:**
- [ ] `server/services/admin/admin_service.go`: rename `type AdminService` → `Service`. Rename `NewAdminService` → `NewService`. Update doc comments. Receivers stay as `*AdminService` → `*Service`.
- [ ] Update admin call sites: `services/admin/cleanup.go` (8 refs), `services/admin/cleanup_test.go` (9 refs), `main.go` (1 constructor call: `admin.NewAdminService` → `admin.NewService`; locals like `adminService :=` keep their variable name — only the type/func names change). `e2e_test.go` and `cmd/simulate/main.go` reference `apiconnect.NewAdminServiceClient` (proto-generated) — leave untouched, see Q3.
- [ ] `server/product/lookup.go`: rename `type ProductInfo` → `Info`. Update doc comment. Update all references in `lookup.go` (5 in-file refs) and `lookup_test.go` (8 refs).
- [ ] Update `services/gear/gen_ai.go:287`: `&product.ProductInfo{` → `&product.Info{`.
- [ ] `server/story/types.go`: rename `type StoryCreator` → `Creator`. Update doc comment.
- [ ] Update all `story.StoryCreator` cross-package call sites (~14 occurrences across `main.go`, `jobs/experience_stories.go`, `services/{community,experience,request,transfer}/service.go`).
- [ ] If `story/generator.go` declares any constructor whose name encodes `StoryCreator` (e.g., `NewStoryGenerator` returning `StoryCreator`), leave the constructor name alone but update the return type to `Creator`. The constructor name itself doesn't stutter unless it does.

**Validates:** `npm run build` passes (lint clean + compiles). `go test -tags=integration -timeout 20m ./...` passes. No `revive.exported` violations remain.

## Acceptance Criteria

- `server/.golangci.yml` shows `severity: error` for the `exported` rule.
- `golangci-lint run ./...` from `server/` reports zero `exported:` violations.
- `npm run build` and `npm run lint` both succeed.
- `go test -tags=integration -timeout 20m ./...` passes.
- CLAUDE.md has a short note under "Go Code Comments" referencing the rule.
- No `//nolint:revive` directives are added.

## Related Issues

- #1377 — promoted `revive.package-comments` to error and added package comments + READMEs. This issue is the explicitly-deferred follow-on.
- #1427 — Dart per-file headers; the issue body explicitly notes this should not be bundled together (different surface, different tools).

## Audit Notes

Audited against `docs/server/architecture.md`, `docs/client/architecture.md`,
`docs/client/caching.md`, `docs/client/i18n.md`. Plan is server-only (Go lint
config + Go symbol renames); the client docs do not apply. The server
architecture guide explicitly endorses the post-rename naming in its
canonical example (`type Service struct` inside `package community`,
not `type CommunityService`) — see `docs/server/architecture.md:247-263`.
The recommendation (Option C: unexport in-package, rename cross-package) is
consistent with the guide's "Expose minimal public API surface" principle
(`docs/server/architecture.md:122`). No fixes required.
