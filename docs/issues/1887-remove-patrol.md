# Issue #1887: Remove all Patrol testing code

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1887
**Filed:** 2026-05-12 | **Last activity:** 2026-05-12
**Labels:** none | **Assignees:** maintainer
**Context docs:** none

## Problem Statement

The Patrol-based E2E test stack — the Flutter `patrol_test/` suite, the
purpose-built `patrol-test-server` Go binary, the shell scripts that wire them
together, and the build/config plumbing that supports them — has not paid off
in practice. Android tests are flaky and brittle, iOS support never worked at
all, and the surrounding infrastructure (a separate Go binary, an HTTP test
actions server, dev-only test endpoints, a custom Android instrumentation
runner, a 1,400-line server harness, three shell scripts, a dedicated env
file) is a tax on every other change in the repo. The decision in #1887 is to
delete the whole thing rather than keep maintaining it.

A working branch (`remove-patrol`) is already cut but currently has zero
commits — the deletion has not started yet.

## Context from Comments

The two comments on the issue are both `@claude` pings from the issue author
(themselves the assignee). There is no discussion, no maintainer pushback, and
no debate about scope — this is a straight cleanup directive. The author is
the project lead and has already created the working branch. That signals: do
the removal, don't relitigate it.

No comments call for a replacement framework. The implicit assumption is that
the remaining test layers — Go unit + integration tests, the
`server/simulation/` scenario harness, and Flutter widget tests in `app/test/`
— are what we're keeping.

## Potential Solutions

### Option A: One-PR scorched-earth removal
- **Approach:** Single PR that deletes every Patrol-related file and edits
  every config that references it. Regenerate `pubspec.lock` and
  `Podfile.lock` in the same PR. Land it as one atomic change.
- **Pros:** Matches the nature of the work (pure deletion, no behavioral
  risk). Easy to review — every hunk is "delete X". No half-removed state
  on `main`. Smallest total churn.
- **Cons:** A single large PR is harder to bisect later if a non-obvious
  consumer breaks. Memory cleanup is included in the same change set as code
  deletion, which slightly broadens review surface.
- **Effort:** Small. The deletion is largely mechanical.

### Option B: Phased PRs — code first, then config and docs
- **Approach:** PR 1 deletes `app/patrol_test/`, `server/cmd/patrol-test-server/`,
  and the three scripts. PR 2 strips the build/config residue
  (`pubspec.yaml`, `build.gradle.kts`, `package.json`, `.gitignore`,
  `server/storage/testing.go`, `server/cmd/check-goroutines/main.go`). PR 3
  cleans up docs and the auto-memory entries.
- **Pros:** Each PR is small and focused. Easier to revert one slice if
  something surprises us.
- **Cons:** PR 1 alone leaves the repo in an inconsistent state — pubspec
  still depends on a now-empty `patrol_test/`, Gradle still references
  `PatrolJUnitRunner`, `npm run test:patrol` still exists but breaks. Each
  intermediate state is broken until the next PR lands. Three PRs of churn
  for what is functionally one deletion.
- **Effort:** Small per PR, but ~3x the coordination cost.

### Option C: Quarantine then delete
- **Approach:** First move everything to a `_archive/` directory and disable
  the npm scripts; only later (separate PR) actually delete.
- **Pros:** Reversible if someone realizes we needed something.
- **Cons:** Adds work, leaves dead code in tree, doesn't match the issue's
  intent ("unburden the code base"). The git history already preserves
  everything — `git revert` or `git checkout <sha> -- path/` is the right
  recovery mechanism, not an archive directory.
- **Effort:** Small + tax of the second pass.

## Recommendation

**Option A.** This is pure dead-code deletion. The issue author has already
decided, no replacement is being built, every change is reversible via git,
and a single PR matches the scope. Splitting into phases (Option B) creates
broken intermediate states without buying anything; archiving (Option C)
defers the cleanup the issue is explicitly asking for.

## Open Questions

- **Do we keep `integration_test:` as a Flutter dev_dependency?** It's only
  consumed by Patrol today; no `app/integration_test/` source directory
  exists. — Recommendation: **remove** it alongside `patrol` / `patrol_finders`.
  Re-add when something actually needs it. Carrying an unused Flutter SDK dep
  hides intent.

- **Do we keep the Android `testInstrumentationRunner` /
  `ANDROIDX_TEST_ORCHESTRATOR` blocks in `build.gradle.kts`?** They were added
  for Patrol but are technically reusable for any AndroidX instrumented test.
  — Recommendation: **remove** them. There are no other AndroidX
  instrumented tests in the repo. If we add Espresso/UI Automator later we
  can re-add the configuration then. YAGNI.

- **Do we delete `docs/ai/patrol_plan.md` and `docs/testing/patrol_tests.md`?**
  Both describe infrastructure that will no longer exist. — Recommendation:
  **delete** both. Plan docs that describe gone code rot in place and mislead
  future readers. Issue plans under `docs/issues/728-e2e-test-cleanup.md` and
  `docs/issues/840-patrol-loan-tests.md` should stay — they are historical
  records of issues that were closed at the time and describe motivation,
  not current architecture.

- **Do we clean up the two `feedback_patrol_*.md` auto-memory entries?** —
  Recommendation: **delete** them and remove their lines from `MEMORY.md`.
  Once the infrastructure is gone, "Patrol's tap calls pumpAndTrySettle"
  is no longer load-bearing guidance and will only confuse future memory
  recall.

- **Do `docs/secret-rotation.md` / `docs/design/unified-create.md` /
  `docs/issues/1260-remove-unary-gen.md` references to `env.patrol.json`
  need updating?** — Recommendation: **leave the historical references in
  issue plans alone** (they describe a past state). **Update
  `docs/secret-rotation.md`** because it is an active runbook that lists
  `env.patrol` as a current rotation target — a runbook with stale steps is
  worse than no runbook. Same for `docs/design/unified-create.md` if it's
  describing current design rather than history.

- **Are there any external consumers (CI workflows, runbooks, dashboards)
  that depend on the `test:patrol:*` npm scripts?** — Recommendation:
  **verified none in `.github/workflows/` already** (no `test_patrol.yaml`
  exists). Spot-check during implementation by grepping for `test:patrol`
  one last time before merge.

- **Should we run `flutter clean` + `pod install` as part of the PR, or
  leave it to local developer setup?** — Recommendation: **regenerate
  `pubspec.lock` and `Podfile.lock` in the PR**. CI rebuilds these anyway,
  but checking them in keeps `git status` quiet for the next person who
  pulls.

## Implementation Plan

### Phase 1: Delete code, tests, scripts, and Patrol-only docs
**Goal:** Remove all source files whose only purpose is Patrol.
**Tasks:**
- [ ] `git rm -r app/patrol_test/` (all test files, helpers, page objects)
- [ ] `git rm -r server/cmd/patrol-test-server/` (main.go, actions.go, media.go, README.md, test_media/)
- [ ] `git rm scripts/run_patrol_tests.sh scripts/record_patrol_tests.sh scripts/run_patrol_videos.sh`
- [ ] `git rm app/env.patrol.json`
- [ ] `git rm docs/testing/patrol_tests.md`
- [ ] `git rm docs/ai/patrol_plan.md`

**Validates:** The deletion compiles cleanly *only* once Phase 2 lands —
intermediate state after Phase 1 will fail. Keep Phase 1 and Phase 2 in the
same PR; the phases are commit boundaries, not ship boundaries.

### Phase 2: Strip Patrol from build, config, and shared code
**Goal:** Remove every reference Patrol left in files that survive.

**Build / dependency config:**
- [ ] `app/pubspec.yaml` — delete `patrol: ^4.3.0`, `patrol_finders: ^3.1.0`,
      `integration_test:` dev_dependency (lines ~71–77) AND the entire
      `patrol:` configuration block at the bottom (lines ~111–117).
- [ ] `app/android/app/build.gradle.kts` — delete the
      `testInstrumentationRunner = "pl.leancode.patrol.PatrolJUnitRunner"`
      line, the `testInstrumentationRunnerArguments["clearPackageData"]`
      line, and the surrounding `testOptions { execution = "ANDROIDX_TEST_ORCHESTRATOR" }`
      block (lines ~68–75).
- [ ] `package.json` — delete the four `test:patrol*` scripts
      (`test:patrol`, `test:patrol:server`, `test:patrol:record`,
      `test:patrol:ci`) around lines 65–68.
- [ ] `.gitignore` — remove `server/patrol-test-server` line (~89).

**Server-side support code:**
- [ ] `server/storage/testing.go` — delete `SetupPatrolTestDatabase()` and
      its doc comment (lines ~722–770). Verify no other callers remain.
- [ ] `server/cmd/check-goroutines/main.go` — remove the
      `"server/cmd/patrol-test-server/main.go": true,` allowlist entry and
      the surrounding "patrol test server and integration test helpers"
      comment line.
- [ ] `server/simulation/README.md` — remove the `patrol-test-server`
      paragraph (line ~31).
- [ ] `scripts/fetch_secret.sh` — update the comment that references
      `test:patrol:record` (line ~5) to drop the patrol example.

**Doc / skill / runbook touch-ups:**
- [ ] `docs/testing/architecture.md` — remove the line referencing
      `../ai/patrol_plan.md` (line ~30).
- [ ] `docs/testing/update_system_tests.md` — remove the
      `app/patrol_tests/` bullet (line ~8) and the "don't implement
      patrol tests" instruction (line ~202).
- [ ] `.claude/skills/ripls-update-system-tests/SKILL.md` — remove the
      "Do NOT implement patrol tests" instruction (line ~59).
- [ ] `docs/secret-rotation.md` — remove `env.patrol` from the active
      Mapbox rotation steps (lines ~312, ~328, ~533). Active runbook only;
      leave issue-plan references alone.
- [ ] `docs/client/testing.md` — **audit finding:** lines ~502–507
      ("Next Steps → E2E Testing") explicitly recommend "Set up Patrol or
      integration_test framework", and the "Weaknesses" list (line ~471)
      cites "No E2E Tests" as a gap. Rewrite this section to reflect the
      post-#1887 stance: client-side E2E is not currently planned;
      coverage comes from Go integration tests (`server/main_integration_test.go`),
      Go E2E tests (`server/e2e_test.go`), the `server/simulation/`
      scenario harness, and Flutter ViewModel/Repository tests. Leave the
      example `integration_test/app_test.dart` snippet (line ~435) but
      add a note that no such directory exists today.

**Memory cleanup:**
- [ ] Delete `~/.claude/projects/-Users-bryan-Documents-ripls/memory/feedback_patrol_testing.md`
- [ ] Delete `~/.claude/projects/-Users-bryan-Documents-ripls/memory/feedback_patrol_pump_hang.md`
- [ ] Remove the "Patrol E2E Testing" section from
      `~/.claude/projects/-Users-bryan-Documents-ripls/memory/MEMORY.md`.

**Validates:** `npm run build` passes (Go compiles, linters clean) and
`flutter analyze` produces no errors for `app/lib/`. No remaining grep hits
for `patrol` in source-controlled files outside `docs/issues/` history.

### Phase 3: Regenerate lockfiles and run the full validation gauntlet
**Goal:** Land the PR in a state where `git status` is clean after a fresh
checkout + standard setup.

**Tasks:**
- [ ] `cd app && flutter clean && flutter pub get` — regenerates
      `pubspec.lock` without the `patrol`, `patrol_finders`, `patrol_log`,
      and `integration_test` entries.
- [ ] `cd app/ios && pod install --repo-update` — regenerates
      `Podfile.lock` without the Patrol pod and removes
      `app/ios/Pods/Target Support Files/patrol/` and
      `app/ios/.symlinks/plugins/patrol/`.
- [ ] `npm run generate` — confirm protobuf codegen still works (no Patrol
      ties expected, but run anyway to be safe).
- [ ] `npm run build` — Go server compiles and lints clean.
- [ ] `go build -tags=integration ./...` — integration-tagged code still
      builds (because `SetupPatrolTestDatabase` was deleted, verify no
      integration test still calls it).
- [ ] `go test ./...` — full Go test suite passes.
- [ ] `npm run test:app` — Flutter analyze + widget tests pass.
- [ ] `git grep -i patrol -- ':(exclude)docs/issues/*' ':(exclude)docs/ai/invitation_improvements.md' ':(exclude)docs/design/unified-create.md' ':(exclude)docs/evals/1157-genexperience-latency.md' ':(exclude)docs/ai/simulation_plan.md'`
      — verify no remaining live references. Hits in historical issue/plan
      docs are expected and fine.
- [ ] Final sweep: confirm `find . -name 'patrol*' -not -path '*/build/*' -not -path '*/.git/*' -not -path '*/node_modules/*'` returns only files that are intentionally historical references in docs.

**Validates:** Clean checkout + `npm install && npm run generate && npm run build && npm run test:app` succeeds with no `patrol` mentions in the dependency graph.

## Audit Notes (2026-05-14)

The `/ripls-audit` pass against `docs/client/architecture.md`,
`docs/client/caching.md`, `docs/client/i18n.md`,
`docs/client/accessibility.md`, `docs/client/testing.md`,
`docs/server/architecture.md`, and `docs/server/observability.md`
produced one change to this plan and zero architectural conflicts:

- **Added `docs/client/testing.md` to Phase 2 doc edits.** That file
  currently recommends Patrol as a future E2E framework and lists
  "No E2E Tests" as a weakness. Removing Patrol without rewriting those
  sections would leave the client testing guide pointing at deleted
  infrastructure. The "Why:" lines that justify the ViewModel-first
  testing philosophy stay — that guidance is independent of E2E.
- **Server-side E2E is untouched.** `docs/server/architecture.md`
  describes `server/e2e_test.go` and `server/main_integration_test.go`,
  both pure-Go and unrelated to Patrol. Confirmed by grep: nothing in
  those files depends on `patrol-test-server` or `SetupPatrolTestDatabase`.
  The `npm run test:server:e2e` and `test:server:e2e:dev` scripts in
  `package.json` survive intact.
- **`integration_test:` dev_dependency removal is safe.**
  `grep -r "package:integration_test" app/lib app/test app/patrol_test`
  returns hits *only* under `app/patrol_test/`. No other consumer.
- **No caching, i18n, or accessibility primitives are affected.** This is
  pure test-infrastructure deletion; no ViewModels, repositories,
  caches, ARB strings, or a11y labels touch Patrol code.
- **Observability conventions are unchanged.** `LoggerWithContext`,
  request-ID propagation, and masking helpers are all server-runtime
  concerns; the `patrol-test-server` binary used them but did not
  define or override them. Deleting the binary removes a consumer, not
  a producer.

## Related Issues
- #728 — original E2E test cleanup that preceded the Patrol effort
  (`docs/issues/728-e2e-test-cleanup.md`). Historical context.
- #840 — built out the Patrol loan tests now being removed
  (`docs/issues/840-patrol-loan-tests.md`). Historical context — the
  artifact this PR deletes.
- #1513 — explicitly excluded `app/patrol_test/` from the
  phase-references-in-comments lint. After #1887, that exclusion can be
  dropped (one-line follow-up in `scripts/check_ephemeral_comments.js` if
  the directory is gone). Note as cleanup in the PR description.
- #1768 / #1770 — the env-file gitignoring work. `env.patrol.json` was
  going to be on that list; deletion supersedes the gitignore step.
