# Issue #1424: Refactor Go files longer than 1000 lines + add CI gate

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1424
**Filed:** 2026-04-26 | **Last activity:** 2026-04-28
**Labels:** none | **Assignees:** unassigned | **Type:** Task | **Priority:** P3
**Context docs:** `docs/server/conventions.md` (anchor), `CLAUDE.md` file-size rule + Dart gate wording, `scripts/check_dart_file_size.sh` (#1324/#1293 precedent), `.github/workflows/test_flutter.yaml` (a11y/color shrink-ratchet model), `.github/workflows/test_go.yaml`

## Problem Statement

CLAUDE.md says source files "should be short (preferably < 1,000 lines)," and the
Dart side enforces this with a hard CI gate (`npm run lint:dart:size`, added in
#1293, after the #1324 sweep brought every `app/lib/**` file under the limit). The
Go side has **no gate**, and the backlog has grown accordingly.

**The issue's numbers are stale — the situation is worse than described.** The
issue body (2026-04-26) lists 23 over-limit files; the triage comment (2026-04-28)
claims "5 Go files >1000 lines remain (down from 6)." Neither matches reality. A
fresh scan today finds **36 non-generated files over 1,000 lines** under `server/`:

- **27 test files** (largest: `server/services/gear/service_test.go` at **3,909**,
  `server/services/feed/service_test.go` at **3,698**, `server/storage/protosql_test.go`
  at **2,704**).
- **9 production files**: `portfolio/home_view.go` (1,789), `ai/prompts.go` (1,774),
  `impact_metrics/metric_detail_calculator.go` (1,772), `services/gear/service.go`
  (1,573), `media/pexels_provider.go` (1,383), `services/feed/service.go` (1,130),
  `impact_metrics/calculator.go` (1,098), `services/portfolio/assembly.go` (1,064),
  `services/experience/gen_ai_streaming.go` (1,054).

The triage "5" almost certainly counted a partial snapshot; the true count has
*grown* since filing precisely because nothing stops new large files from landing.
That growth is the strongest argument in the issue: the durable win is the **gate**
(which permanently stops regressions), not just the one-time split.

The same review pressures that motivated the Dart sweep apply to Go: review load,
merge conflicts, slow incremental compilation, and hard-to-isolate tests. This is
intentionally **Go-only and `server/`-only** — the Dart sweep is done.

## Context from Comments

Only one comment: the 2026-04-28 `/ripls-triage` update setting Type=Task,
Priority=P3, with rationale "code_quality polish." No design discussion, no
disagreement. The single actionable signal is that its file count was already
wrong two days after filing — reinforcing that any plan must re-scan the codebase
rather than trust the issue body's list. Otherwise the issue is unusually
self-contained: it prescribes the approach (mirror #1324), the script location,
the exclusions, and the acceptance criteria. This plan's job is to sequence that
work safely and resolve the one real design fork (how to gate 36 files at once).

## Potential Solutions

The *what* is settled by the issue and the #1324 precedent (split by concern into
sibling files, add a CI gate). The only real decision is **how to introduce the
gate against a 36-file backlog** — gate-first with a shrinking baseline, or
sweep-everything-then-gate.

### Option A: Gate-first with a shrink-only allowlist (recommended)

- **Approach:** Land the gate script + a `scripts/go_size_allowlist.txt` seeded
  with today's 36 over-limit files **in Phase 1**, wired into `test_go.yaml` with
  the same "allowlist may only shrink" check the repo already uses for a11y
  (`scripts/a11y_allowlist.txt`) and hardcoded colors (`scripts/color_allowlist.txt`).
  New files >1,000 lines fail immediately; each subsequent split PR deletes its
  file from the allowlist. When the allowlist reaches empty it (and its shrink
  check) are deleted, leaving a pure hard gate identical to the Dart end state.
- **Pros:** The durable value — stopping *new* large files — ships on day one
  instead of waiting weeks for 36 splits. Reuses a battle-tested pattern already
  in the repo (a11y/color ratchets), so reviewers recognize it. Splits become
  independently mergeable, low-risk PRs. The allowlist is honest, self-documenting
  transitional scaffolding, not a permanent `// go-line-count-allow` on 36 files.
- **Cons:** Introduces a temporary allowlist file that must be drained and then
  removed (one extra cleanup step). Slightly more machinery than Option B up front.
- **Effort:** Medium overall (small Phase 1; the bulk is the mechanical splits).

### Option B: Sweep everything, then add a hard gate (strict #1324 mirror)

- **Approach:** Split all 36 files across a branch (or several PRs) with **no gate
  in place**, then land `check_go_file_size.sh` as a hard 1,000-line limit at the
  very end — exactly how Dart did it (`4e6248676` shows only 2 residual files
  needed the escape hatch after the sweep).
- **Pros:** Cleanest end state with zero transitional scaffolding; a perfect mirror
  of the precedent the issue names.
- **Cons:** No protection during the multi-PR split window — a new 1,500-line file
  could land and go unnoticed, and the backlog literally grew this way. Pressures
  the whole sweep into one big-bang effort to avoid a long unguarded gap, which
  fights against small reviewable PRs.
- **Effort:** Medium, but riskier sequencing.

### Option C: Bulk `// go-line-count-allow` escape hatches instead of an allowlist

- **Approach:** Gate-first (like A), but mark all 36 existing files with the
  per-file `// go-line-count-allow` comment (each pointing at #1424) rather than a
  central allowlist.
- **Pros:** No separate allowlist file; the escape-hatch mechanism the issue
  already prescribes does double duty.
- **Cons:** Abuses an escape hatch meant for the *rare, permanently* unsplittable
  file. 36 identical `// go-line-count-allow (#1424)` comments is noise, invites
  copy-paste abuse for genuinely-splittable new files, and offers no single place
  to see remaining debt or enforce "shrink-only." Diffs churn twice (add comment,
  then remove it).
- **Effort:** Medium; comparable to A but messier.

## Recommendation

**Option A — gate-first with a shrink-only allowlist.** It ships the permanent
protection immediately (the growth from 23→36 files proves that's the real
problem), reuses an existing, reviewer-familiar repo pattern, and keeps every
split as a small independent PR. Reserve the `// go-line-count-allow` comment
(per the issue) strictly for any file that *genuinely* cannot be split, with a
tracking-issue link — the Dart end state (`4c3939701`/`4e6248676`) had only two
such files, and Go should aim for zero. The allowlist is explicitly transitional:
Phase 4 deletes it once drained, leaving a hard 1,000-line gate identical to Dart.

Threshold stays at **1,000 lines** for parity with the Dart gate and CLAUDE.md;
use **800** as the *soft target* when deciding how finely to split (the issue's
"target <800"), but don't gate at 800 — that would immediately flag near-boundary
files like `server/cmd/eval-seed-from-urls/main.go` (995) and create churn without
a policy decision.

## Open Questions

- **Allowlist vs. escape-hatch for the baseline?** — Recommendation: **allowlist**
  (Option A). It mirrors `a11y_allowlist.txt`/`color_allowlist.txt`, gives a single
  shrink-only ledger of remaining debt, and is deleted when empty. Keep the
  `// go-line-count-allow` comment only for permanent, justified exceptions.
- **Gate threshold: 1,000 or 800?** — Recommendation: **gate at 1,000** (Dart
  parity, matches CLAUDE.md's stated rule), split toward **≤800** as a soft target
  so files have headroom before they trip the gate again.
- **What exactly does the gate exclude?** — Recommendation: exclude `*.pb.go`,
  `*_connect.go`, anything under a `gen/` directory (matching `buf.gen.yaml`
  output), plus files carrying `// go-line-count-allow`. Scope the scan to
  `server/` only (the issue is Go-/server-scoped; `app/` is covered by the Dart
  gate, and root/`scripts/` Go is negligible). Confirm no other generated Go
  suffix exists (e.g. mocks) before finalizing the exclude list.
- **Wire into `npm run lint:go` or keep separate?** — Recommendation: **separate**,
  mirroring Dart. Add `"lint:go:size": "bash scripts/check_go_file_size.sh"`, add
  it to the top-level `lint` chain, and add an explicit step in `test_go.yaml`
  (alongside the existing ephemeral-comments/codeinternal steps) plus the
  shrink-check step. Do *not* nest it inside `lint:go` (the Dart size check is
  likewise not nested inside `lint:dart`), so a formatting-only `lint:go` run stays
  fast and focused.
- **Do any files genuinely resist splitting?** — Recommendation: assume **none**.
  Every production file inspected has clean function/type boundaries that split by
  concern (see Phase 3). If one survives, it gets a `// go-line-count-allow` with a
  *new* tracking issue (not #1424), per CLAUDE.md's TODO/escape-hatch policy.
- **Batch test-file splits by sub-agent — how to avoid churn on shared helpers?**
  — Recommendation: split **within the existing package into sibling `_test.go`
  files** so unexported fixtures/helpers (`setupTest`, table builders) remain
  visible with zero new imports. Never extract a new package (that would trip the
  "Require tests on every Go package" gate and add import overhead).

## Implementation Plan

Each phase is an independently shippable PR (or small PR series). After each phase:
`go test ./...` **and** `go test -tags=integration ./...` must pass, and the
allowlist must be strictly smaller than the previous phase.

### Phase 1: Gate infrastructure + baseline (ship the ratchet first)
**Goal:** Permanently stop new >1,000-line Go files; document the current backlog.
**Tasks:**
- [ ] Write `scripts/check_go_file_size.sh` modeled on `scripts/check_dart_file_size.sh`:
      threshold arg (default 1000), scan `server/**/*.go`, skip `*.pb.go`,
      `*_connect.go`, `*/gen/*`, and any file containing `// go-line-count-allow`;
      also skip files listed in `scripts/go_size_allowlist.txt`; print
      `FAIL: <path> has N lines (limit 1000)` and exit non-zero on violation.
- [ ] Create `scripts/go_size_allowlist.txt` seeded with today's 36 over-limit
      files (one repo-relative path per line, `#`-comment header explaining it is
      shrink-only transitional scaffolding for #1424).
- [ ] Add `"lint:go:size": "bash scripts/check_go_file_size.sh"` to `package.json`
      and append it to the top-level `lint` chain.
- [ ] Wire into `.github/workflows/test_go.yaml`: a "Check Go file sizes (#1424)"
      step running `npm run lint:go:size`, plus a "Verify Go size allowlist did not
      grow" step copied from the a11y/color shrink checks (`git fetch` base ref,
      compare entry counts, fail if grown). **Note:** this is the first server-side
      use of the shrink-only-allowlist pattern — it borrows the exact mechanism the
      Dart side already uses (`scripts/a11y_allowlist.txt`, `scripts/color_allowlist.txt`
      in `test_flutter.yaml`), so reviewers recognize it.
- [ ] **Add `scripts/check_go_file_size.sh` AND `scripts/go_size_allowlist.txt` to
      BOTH trigger surfaces in `test_go.yaml`** — the top-level `on.push.paths` list
      *and* the `changes` job's `dorny/paths-filter` `relevant:` filter — mirroring
      how `scripts/check_ephemeral_comments.js` is listed in both today. Without the
      `changes`-job entry, a PR that touches *only* the allowlist or the script
      matches no `server/**` path, so the `test` job skips and reports success — the
      shrink check would silently never run on exactly the PRs that edit it.
**Validates:** The gate mechanism works end-to-end; a deliberately-oversized test
file fails CI; the 36-file backlog is captured in one auditable ledger. No product
code changed yet.

### Phase 2: Split the 27 test files (biggest count, lowest risk)
**Goal:** Clear the bulk of the backlog with mechanical, zero-production-risk splits.
**Tasks:**
- [ ] Fan out **parallel sub-agents**, one per test file (or per package), splitting
      by RPC method / fixture group into sibling `_test.go` files in the *same
      package* (e.g. `service_test.go` → `service_save_test.go`,
      `service_list_test.go`, …). Keep shared helpers where the package can see them.
- [ ] **Split `integration_tests/*` along sub-feature/scenario boundaries, not
      arbitrary halves.** `server/architecture.md §3` designates `integration_tests/`
      as "one `_test.go` file per feature," so `experience_test.go` (1,864) should
      break into coherent sub-flows (e.g. RSVP, time-proposals, sharing) — following
      the precedent the service package already sets with `experience/rsvp_test.go`
      and `experience/time_proposals_test.go` — with every new file staying
      `package integration_tests`.
- [ ] **Preserve any `//go:build integration` tag as the first line of every new
      file** (per `server/conventions.md` §Server Testing); keep `integration_tests/`
      and `e2e_test.go` splits behind their tags. Run the integration suite, not just
      the unit suite, after these.
- [ ] Batch by risk: unit-test files first, then the heavier tagged
      `integration_tests/*` and `e2e_test.go`.
- [ ] Delete each split file from `go_size_allowlist.txt` as it drops under 1,000.
- [ ] **Verify per batch:** `npm run build` (golangci-lint incl.
      gofumpt/godot/revive + `go build ./...`, the documented per-change gate) **plus**
      `go build -tags=integration ./...` for tagged moves, then `go test ./...` **and**
      `go test -tags=integration ./...`. Observe the **Go-specific split gotchas**
      (below) on every file.
**Validates:** ~75% of the backlog cleared; both test suites still green; allowlist
shrinks by ~27 entries; the gate + shrink-check exercised on real PRs.

### Phase 3: Split the 9 production files (by concern, with review + rationalize)
**Goal:** Bring production code under the limit and dedup what the splits expose.

**Follow the `server/architecture.md §3` naming convention** (its `services/community/`
→ `membership.go`/`invitations.go`/`gear_sharing.go` example is the model): `service.go`
retains the struct, constructor, `Set*` wiring, and shared types; feature files are
named after functionality (lowercase-underscore) and group related RPC handlers. **Where
the package name already namespaces the domain, drop the redundant prefix** — in
`services/gear/` prefer `save.go`/`get.go`/`list.go` over `gear_save.go` (revive's
no-stutter rule discourages `gear.gear_*`). Keep a descriptive prefix only in
multi-concern packages (e.g. `portfolio/` already holds `home_view.go`, `inbox_view.go`,
`assembly.go`, `fetch.go`, `metrics.go`, so `home_view_*` stays meaningful there).

**Tasks (each file → sibling files in the same package, GetX orchestrators stay thin):**
- [ ] `portfolio/home_view.go` → by section: `home_view_decisions.go` (assemble*Decisions),
      `home_view_upnext.go`, `home_view_calendar.go`, `home_view_gear.go`,
      `home_view_activity.go`, `home_view_format.go` (label/time helpers). Prefix kept
      because `portfolio/` is multi-concern.
- [ ] `ai/prompts.go` → by domain: `prompts_gear.go`, `prompts_experience.go`,
      `prompts_request.go`, `prompts_summary.go`, `prompts_story.go`,
      `prompts_suggestions.go` (the `ai` package holds providers too, so the
      `prompts_` prefix disambiguates).
- [ ] `impact_metrics/metric_detail_calculator.go` → by metric kind
      (money/time/CO2/quality-time/social + subsidiary), mirroring the
      `drill_down_builder.dart` split from #1324.
- [ ] `services/gear/service.go` → by RPC group in the namespaced package: `save.go`,
      `get.go`, `list.go`, `delete.go`, plus `convert.go` for the `convert*` API↔storage
      helpers. `service.go` keeps the `Service` struct, `New`, and the `Set*` setters.
- [ ] `media/pexels_provider.go` → separate the HTTP client + response types
      (`pexels_client.go`) from the provider (`pexels_provider.go`); optionally
      photos vs. videos.
- [ ] `services/feed/service.go` → `feed_items.go` (`create*Item` constructors) and
      `freshness.go` (unread/fresh/action-required helpers); keep
      `GetFeed`/`generateFeedItems` in `service.go`.
- [ ] `impact_metrics/calculator.go`, `services/portfolio/assembly.go`,
      `services/experience/gen_ai_streaming.go` → split by concern (smaller, ~1,050–1,100).
- [ ] **Rationalize:** after each split, look for cross-file dedup the split exposes
      (the Dart pass surfaced `ContentViewPaneSwitcher` etc.); fold obvious duplicates.
      Watch for repeated proto↔API conversion blocks (`gear/service.go` already has a
      cluster of `convert*` helpers) — the code_quality rubric flags these as a
      missing-helper smell.
- [ ] Delete each file from `go_size_allowlist.txt`; verify per `npm run build` +
      both test suites per file; observe the **Go-specific split gotchas** (below).
**Validates:** Every production file <1,000 lines; behavior unchanged (tests green);
allowlist drained to zero (or only genuinely-unsplittable files remain, each moved
to a `// go-line-count-allow` with a *new* tracking issue).

### Phase 4: Finalize the gate + docs
**Goal:** Convert the transitional ratchet into a permanent hard gate and record the rule.
**Tasks:**
- [ ] Once the allowlist is empty, **delete `scripts/go_size_allowlist.txt`** and its
      "allowlist did not grow" CI step, and drop the allowlist lookup from
      `check_go_file_size.sh` — leaving a pure hard 1,000-line gate identical to Dart.
- [ ] Update **CLAUDE.md**: add a Go-gate paragraph next to the Dart one under
      "File Organization" (threshold 1,000; excludes `*.pb.go`/`*_connect.go`/`gen/`;
      `npm run lint:go:size`; wired into `test_go.yaml`; `// go-line-count-allow`
      escape hatch requires a tracking-issue link).
- [ ] Update **`docs/server/architecture.md §3 "Short, Focused Files"`** — this is
      where the `<1000` rule already lives (as "preferably"). Change it to note the
      rule is now **CI-enforced** (hard 1,000-line gate via `npm run lint:go:size`,
      with the `// go-line-count-allow` escape hatch), and **bump its `freshness:`
      stamp**. (`docs/server/conventions.md` has no file-organization section today —
      don't invent one; a one-line cross-reference from its Build-Verification
      section to the gate is enough if desired.)
- [ ] Confirm acceptance criteria: every non-generated `server/**` `.go` file
      <1,000 lines (or justified escape hatch); `scripts/check_go_file_size.sh` runs
      as `npm run lint:go:size` and in `test_go.yaml`; CLAUDE.md updated;
      `go test ./...` and `go test -tags=integration ./...` pass.
**Validates:** Issue #1424 acceptance criteria fully met; gate is permanent and
matches the Dart precedent exactly.

## Go-Specific Split Gotchas

These apply to *every* file split in Phases 2–3 (Go differs from the Dart sweep in
ways that will fail CI if ignored):

- **Exactly one package doc comment survives.** A `// Package <name>` comment must
  exist once per package (revive `package-comments`, usually parked in `doc.go`).
  When splitting, do **not** copy it onto sibling files — duplicate package comments
  fail `go vet`/revive. New files just start with `package <name>`.
- **`//go:build integration` must be the literal first line** of any tagged file
  (blank line after it), per `server/conventions.md` §Server Testing. Every split of
  a tagged file inherits the tag.
- **Splits move code; they must not add new exported symbols or rename moved ones**
  — `revive.exported` (severity error) enforces doc-comment-starts-with-name and the
  no-package-stutter rule, so a careless rename during a move fails the build.
- **No ephemeral phase-label comments** in new files or the allowlist header
  (`scripts/check_ephemeral_comments.js` scans `server/**/*.go`); reference `#1424`,
  never "Phase 2"/"the split PR".
- **The size gate is deliberately *not* part of `npm run build`** (matching Dart,
  which keeps it out of `flutter analyze` and the build). It runs in `npm run lint`
  and `test_go.yaml`. Tradeoff to accept: a local `npm run build` won't flag an
  oversize file — `npm run lint` and CI will. Don't nest it inside `lint:go` (that
  chain stays formatting/vet-focused).
- **Keep splits inside existing packages.** Extracting a *new* package would trip the
  "Require tests on every Go package" gate in `test_go.yaml` (needs its own
  `*_test.go` or a `.github/coverage-allowlist.txt` entry) and add cross-package
  imports for no benefit.

## Audit Notes

Audited against the documentation context map on 2026-07-08.

**Docs consulted:** `docs/server/architecture.md` (⚓ — §3 "Short, Focused Files" is
the canonical anchor: it already states the `<1000` rule, gives the
`services/community/` split example, and defines the file-naming convention; §7
testing layers), `docs/server/conventions.md` (⚓ — Server Testing build tags,
`revive.exported` no-stutter, package comments, `check_ephemeral_comments.js`, Build
Verification), `docs/testing/architecture.md` (⚓ — test layers, testcontainers),
`docs/reviews/code_quality_review_prompt.md` (Areas §1 DRY, §8 File Sizing — tests
exempt from DRY; prefer tooling over manual fixes), `docs/reviews/tech_debt_review_prompt.md`
(Areas §3 duplication), and `.github/workflows/test_go.yaml` +
`.github/workflows/test_flutter.yaml` (a11y/color shrink-ratchet CI model).

**Changes applied:**
1. **CI wiring corrected (Phase 1):** the `test_go.yaml` `changes` job would skip an
   allowlist-only or script-only PR, making the shrink check a silent no-op on exactly
   those PRs. Required adding both paths to the `push.paths` list *and* the `changes`
   job filter (mirroring `check_ephemeral_comments.js`).
2. **Integration-test split nuance (Phase 2):** architecture.md §3 designates
   `integration_tests/` as "one `_test.go` file per feature" — splits must follow
   sub-feature boundaries, not arbitrary halves; every new file stays
   `package integration_tests` with its build tag.
3. **File-naming aligned to architecture.md §3 (Phase 3):** dropped redundant
   package-name prefixes where the package already namespaces (`services/gear/` →
   `save.go`/`get.go`, not `gear_save.go`) per revive's no-stutter rule; kept
   `service.go` for struct/constructor/`Set*`/shared types; kept meaningful prefixes
   only in multi-concern packages (`portfolio/`, `ai/`).
4. **Doc-update target corrected (Phase 4):** the `<1000` rule lives in
   architecture.md §3, not a (nonexistent) conventions.md file-org section — Phase 4
   now updates §3 to mark the rule CI-enforced and bumps its freshness stamp.
5. **Added a Go-Specific Split Gotchas section** capturing package-comment
   uniqueness, build-tag placement, no-new-exported-symbols, ephemeral-comment gate,
   the `npm run build` tradeoff, and the keep-splits-in-package requirement.

**No blocking conflicts found.** The plan is well-aligned with documented standards —
architecture.md §3 literally prescribes this refactor and its file-naming convention,
and the a11y/color ratchets are an established repo pattern the Phase-1 gate reuses.
DRY caution: per the code_quality rubric, **test bodies are exempt from DRY** (prefer
DAMP) — Phase 2's rationalization should consolidate shared *fixtures/helpers* only,
not collapse readable inline test setup.

## Related Issues
- #1324 — Dart file-size sweep (completed). This issue is its Go counterpart; the
  split-by-concern playbook and the rationalization step come from it.
- #1293 — Dart line-count CI gate (completed). `scripts/check_dart_file_size.sh`
  and its `test_flutter.yaml` wiring are the direct model for the Go gate.
- The a11y (#1946-era) and hardcoded-color ratchets in `test_flutter.yaml` are the
  model for the **shrink-only allowlist** transition mechanism (Option A).
