# Issue #1334: [code_quality] Missing file comments and folder READMEs throughout the codebase

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1334
**Filed:** 2026-04-25 | **Last activity:** 2026-04-25
**Labels:** documentation, auto-filed, review:code_quality | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Two related documentation gaps make the codebase hard to navigate for new contributors (human or AI):

1. **Missing package/file-level comments.** Of 322 non-generated, non-test Go files under `server/`, 270 (~84%) start directly with `package <name>` — no `// Package …` or top-of-file doc comment. CLAUDE.md and Go convention both expect one. The same gap exists in Dart, where most files start straight at imports/class.
2. **Missing folder READMEs.** Of 38 multi-file Go directories under `server/`, only 2 have a `README.md` (`server/ai`, `server/logging`). Of 50 multi-file Dart directories under `app/lib/`, only 1 has one (`app/lib/presentation/widgets/creation`). CLAUDE.md identifies the major packages but doesn't document them per-directory.

The lint configuration already has `revive` running with `package-comments` enabled — but at `severity: warning`, so the violations don't fail builds and have accumulated. Without a forcing function, every new file copies the no-comment precedent.

The issue's recommendation is concrete: (1) promote `package-comments` to error; (2) add a CI check for READMEs in multi-file directories under `server/` and `app/lib/`; (3) land one PR that stubs comments and READMEs across the ~85 affected locations.

## Context from Comments

One comment, from the issue author/maintainer (maintainer, 2026-04-25): "@claude make it so!" — a direct go-ahead with no scope changes. There's no debate to resolve, but also no direction on how strict the enforcement should be (warning vs. error), how thorough the stub content needs to be, or whether to gate on per-file comments vs. just package comments + READMEs.

## Codebase Findings

- `server/.golangci.yml` already enables `revive` with `package-comments` and `exported` rules at `severity: warning`. Promoting to error is a one-line change. `godot` is also already on (toplevel scope), so once package comments exist they'll be checked for terminal punctuation.
- The `revive` `package-comments` rule fires once per package (on the file containing the package comment), not once per file. So "fix it" for Go really means: write **one** good `// Package foo …` comment per package, anywhere in the package — not 270 individual file headers. This dramatically reduces effort vs. the issue's framing.
- Existing good examples to model from: `server/ai/README.md` (architecture overview + directory layout + design principles); `server/middleware`, `server/product`, `server/github`, `server/itemstats`, `server/timezone`, `server/health` already have proper `// Package …` comments — so the pattern is established, just inconsistently applied.
- Dart has no equivalent of `revive package-comments`; we'd need a custom lint or CI script to enforce file headers there. A README-presence check is much cheaper to enforce than per-file Dart comments.
- CLAUDE.md (lines 27–63) already lists most server and app subpackages with a one-line description. That content is the natural seed for per-package READMEs — extract and expand, don't reinvent.

## Potential Solutions

### Option A: Full scope — file headers + READMEs everywhere, promote lint to error

- **Approach:** Promote `revive.package-comments` and `revive.exported` to `severity: error`. Add CI scripts that (1) require a `README.md` in every directory under `server/` and `app/lib/` with >1 source file and (2) require a `///` doc comment at the top of every non-generated Dart file. Land one large PR that stubs all ~85 READMEs and adds package/file comments everywhere.
- **Pros:** Maximally addresses the issue as written. Permanent forcing function — drift cannot re-accumulate.
- **Cons:** ~85 READMEs + ~270 Go package decisions + ~540 Dart file headers in one PR is enormous, hard to review meaningfully, and almost certainly produces low-quality stubs ("This package contains user-related code"). Per-file Dart enforcement is a new lint we'd have to build and maintain. Easy to make the gate so strict that legitimate small changes start needing throwaway header tweaks.
- **Effort:** Large.

### Option B: Recommended — package comments + folder READMEs, file-header rule deferred

- **Approach:**
  1. Promote `revive.package-comments` from warning to error in `server/.golangci.yml`. (`exported` stays warning to keep this PR focused.)
  2. Add a CI check (small bash or Go script in `scripts/`, wired into `npm run lint`) that fails if any directory under `server/` or `app/lib/` with >1 source file lacks a `README.md`. Generated dirs and `*_test.go`/`*.g.dart`/`*.freezed.dart`/`*.mocks.dart` are excluded from the file count.
  3. Stub one `// Package foo …` comment per Go package missing one (~36 packages, not 270 files) and one short README per multi-file directory (~85 directories total). Use CLAUDE.md's existing one-liners as seed text and expand to 3–8 lines per README covering: purpose, key types/files, when to add things here vs. elsewhere.
  4. Skip per-Dart-file headers for now — file an explicit follow-up issue if we want them.
- **Pros:** Captures the bulk of the value (a reader landing in any directory has an anchor) without a new Dart lint. Package comments are a known, supported lint with fast feedback. Effort is bounded and reviewable when split by directory.
- **Cons:** Doesn't enforce file-by-file Dart comments — the original recommendation #3's per-file gap remains. Stub READMEs may still be shallow if rushed; needs reviewer discipline.
- **Effort:** Medium.

### Option C: Minimum — lint promotion + READMEs only, no big stub PR

- **Approach:** Promote `revive.package-comments` to error and add the README presence check, then let the build break. Fix violations as a side effect of normal work — every PR that touches a directory without a README is forced to add one before merging.
- **Pros:** Smallest immediate PR. Forces high-quality READMEs because they're written by whoever is actively in that directory. Avoids the "85 stubs in one PR" review problem.
- **Cons:** Many directories rarely change; their READMEs may never get written. Builds break for unrelated PRs that happen to touch a stale directory, frustrating contributors. Doesn't actually close the documentation gap on a known timeline.
- **Effort:** Small (immediate); medium (spread over months).

## Recommendation

**Option B**, with this scope for the first PR (per maintainer direction 2026-04-25): land **all README.md files + all 37 missing Go package comments + the `revive.package-comments` lint promotion in this PR**. Only per-file source comments (Dart file headers, Go file headers beyond the package comment) are deferred to a follow-up issue.

Rationale: package comments and READMEs are the two anchors a reader actually uses to orient. Each is short and mechanical to produce. Bundling them lands the documentation gap closure in one PR while leaving the much larger per-file enforcement question (custom Dart lint, etc.) for a focused follow-up.

The README-presence CI gate is also deferred — without it, drift can re-accumulate, but adding it requires a script + allowlist mechanism that's worth a separate, focused PR.

## Open Questions

- **How strict should README content requirements be — minimum length, required sections?** Recommendation: require *presence* in CI, not content shape. Set an informal expectation in the PR template / CLAUDE.md that READMEs cover (a) what this package does in one sentence, (b) the main types/files, (c) when to add code here vs. elsewhere. Keep it 5–30 lines. Stricter content checks invite gaming.
- **Should the README CI check live in `npm run lint` or be its own npm script?** Recommendation: own script (`npm run lint:readmes`) that the existing `lint` target depends on. Easier to run locally when iterating, easier to skip in narrow CI jobs.
- **Should we also promote `revive.exported` (exported-symbol comments) to error in this issue, or is that out of scope?** Recommendation: leave it at warning for now. It's a much larger surface area (every exported func/type in the codebase) and would balloon the PR scope. File a separate follow-up if we want it.
- **Where do generated-code directories sit in the README check?** Recommendation: exclude `server/gen/`, `app/lib/data/gen/`, and any path matching `*/gen/*`. Generated dirs are not for human readers.
- **What about `server/` and `app/lib/` themselves (top-level)?** Recommendation: include. These are the two most important READMEs to have, and they currently don't exist. Frame each as an index pointing into the subpackage READMEs.
- **Should each Go package comment go in a dedicated `doc.go` file or in an existing source file?** Recommendation: dedicated `doc.go` for any package with >2 source files; otherwise inline at the top of the most representative file. `doc.go` keeps the package comment from being lost during refactors.

## Implementation Plan

This PR (single, atomic):

### Step 1: Add `doc.go` for all 37 missing Go packages
- One `doc.go` per package containing `// Package <name> …` (1–3 sentences).
- Required so the lint promotion in Step 3 doesn't break the build.
- Seed content from CLAUDE.md where present; for service packages, describe the RPC surface and what state it owns.

### Step 2: Author README.md for every multi-file directory
- Every directory under `server/` and `app/lib/` with >1 source file (excluding `*/gen/*`, `_test.go`, `*.g.dart`, `*.freezed.dart`, `*.mocks.dart`).
- Plus top-level indexes: `server/README.md`, `app/README.md`, `app/lib/README.md`.
- Each README: 5–30 lines covering (a) what this package/directory does, (b) the main files/types and their roles, (c) when to add code here vs. an adjacent directory.
- Style model: `server/ai/README.md` for larger packages; `app/lib/presentation/widgets/creation/README.md` for smaller ones.

### Step 3: Promote `revive.package-comments` to error
- One-line change in `server/.golangci.yml` (`severity: warning` → `severity: error`).
- Forcing function so future packages can't ship without a Package comment.

### Step 4: Update CLAUDE.md style guidelines
- Add a short note: every Go package needs `// Package …`; every multi-file directory under `server/` and `app/lib/` needs `README.md`.

### Step 5: Verify
- `npm run build` (lint + go build) passes.

## Deferred to follow-up issues

- **README-presence CI gate** — script that fails CI when a new multi-file directory lacks a README. Worth its own focused PR with an allowlist mechanism for any future drift. → #1426
- **Per-file Dart headers** — would need a custom lint or `dart_code_metrics` rule. File a separate issue scoping the rule and deciding whether to gate by new-files-only. → #1427
- **Per-file Go headers beyond the package comment** — e.g., `revive.exported` promotion. Much larger surface (every exported func/type) and merits its own PR. → #1428

## Related Issues

- None linked from this issue, but the recommendation echoes the broader documentation thread surfaced by `/ripls-reviews` (see `docs/reviews/reports/code_quality_review.md`).

## Audit Findings (post-implementation, 2026-04-25)

Cross-checked the executed work against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, and `docs/server/architecture.md`. Three items worth flagging — none are blockers; treat as follow-up cleanup.

### A. Vendor-name rule was over-applied to READMEs → #1430

The agent prompts forbade vendor names ("Mapbox", "Mailgun", "Firebase Cloud Messaging", "Vertex AI", "Pexels", etc.) in READMEs, citing the proto-comments convention in `CLAUDE.md`. That convention exists because **proto comments are the API contract clients see** — vendor names there leak implementation choices into the wire format. READMEs are internal contributor docs and the canonical architecture doc (`docs/server/architecture.md`) uses vendor names freely throughout (e.g., "PostgreSQL, Firebase, GCS, Mapbox, Mailgun, Vertex AI"). Some READMEs are now more abstract than they need to be ("push notification provider", "geocoding provider").

- **Impact:** Mild — READMEs still orient a reader, just with one extra mental hop.
- **Follow-up:** When touching a README, name the actual vendor where it helps. Update `CLAUDE.md` Style Guidelines to clarify: vendor names *out* of proto comments, *in* READMEs is fine.

### B. `app/lib/l10n/README.md` points at the wrong API → #1429

The generated README tells contributors to access strings via `AppLocalizations.of(context)!.yourKey`. The canonical pattern (`docs/client/i18n.md`) is `context.l10n.yourKey` via the extension in `app/lib/core/extensions/l10n_extensions.dart`. The README also omits three load-bearing rules: viewmodels never resolve strings, the `npm run lint:dart:i18n` CI check, and the `nullable-getter: false` config that catches missing keys at compile time.

- **Impact:** A new contributor following this README would write non-canonical code.
- **Follow-up:** Update `app/lib/l10n/README.md` to use `context.l10n.*` and to reference the CI lint check and the no-strings-in-viewmodels rule.

### C. Architectural alignment is good

Cross-checked the highest-stakes READMEs against the canonical docs:
- `app/lib/data/repositories/README.md` — correctly enforces repository-pattern boundary (viewmodels never call services directly), cache-key conventions match `docs/client/caching.md`.
- `app/lib/data/cache/README.md` — accurate `CacheService` / namespace / TTL behavior.
- `server/storage/README.md` — accurately describes the Proto-SQL pattern from `docs/server/architecture.md`.
- `server/services/README.md` — captures the service-independence principle (services depend on shared libraries, not other services).
- `server/services/transfer/doc.go` and other service `doc.go` comments — describe the RPC contract, not transport plumbing or storage internals. Aligned with server architecture conventions.

No architectural contradictions with the canonical docs.
