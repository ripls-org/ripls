# Issue #1514: [code_quality] `.golangci.yml` still missing widely-adopted linters

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1514
**Filed:** 2026-04-28 | **Last activity:** 2026-04-30
**Labels:** auto-filed, review:code_quality | **Assignees:** unassigned
**Context docs:** docs/reviews/reports/code_quality_review.md

## Problem Statement

`server/.golangci.yml` is missing nine linters that the last two code-quality
reviews flagged as standard, well-adopted, and known to surface real
existing issues in this codebase: `errorlint`, `misspell`, `bodyclose`,
`nilerr`, `nilnil`, `goconst`, `funlen`, `cyclop`, `unparam`. The reviewer
explicitly recommends a two-wave rollout: a "low-noise" Wave 1
(`errorlint`, `misspell`, `bodyclose`, `nilerr`, `nilnil`) and a "needs
`//nolint:` grandfathering" Wave 2 (`goconst`, `funlen`, `cyclop`,
`unparam`). Each wave should land with matching baseline fixes so the gate
goes green on merge and protects against regression going forward.

The current config (post-`dupl`/`forbidigo` adoption) enables 21 linters;
adding these nine will close the longest-standing tooling gap in the
report's "Tooling Recommendations" table (rows 2 and 3 of section
`docs/reviews/reports/code_quality_review.md` lines 437–439).

## Context from Comments

The triage comment (2026-04-28) sets **Issue Type: Task, Priority: P2**
with the rationale "code_quality High durable: tooling-only fix; missing
linters in `.golangci.yml`." A second comment from a maintainer is just
`@claude`, kicking this run.

Concrete hooks already known from the review and a quick grep:

- `errorlint` will fire on 5 production sites in `server/storage/`
  (`cascade_delete.go:14`, `testing.go:456,546,599,609`) plus ~14 test
  sites that match `strings.Contains(err.Error(), …)`. The fix is the
  one already prescribed in the review (`var ErrRecordNotFound =
  errors.New(…)` at `server/storage/errors.go`, wrap at the source,
  match with `errors.Is`).
- `nilnil` candidate sites: ~50 production `return nil, nil` patterns
  across `server/ai/`, `server/auth/`, `server/location/`,
  `server/product/`, `server/services/*`. Most encode "not found"
  semantics — the right long-term shape is a sentinel error, but for
  a first wave a targeted set of rewrites plus `//nolint:nilnil` with
  a justification on the rest is acceptable.
- `bodyclose`: review notes today's HTTP callers in `server/ai/` and
  `server/location/` already use `defer resp.Body.Close()` correctly,
  so this should land green or near-green and act purely as a
  regression guard.
- `misspell`: count unknown until first run; expected to be very low.
- `funlen` / `cyclop`: review names `server/main.go` (1026 lines),
  `server/ai/prompts.go` (1892 lines), and
  `server/services/gear/service.go` (1381 lines) as known violators.
  These are explicitly Wave-2 grandfathering territory.

No comments push back on the recommendation, so the open dispute is
mechanical (scope + grandfathering style), not strategic.

## Potential Solutions

### Option A: Wave 1 only this PR; file a follow-up issue for Wave 2

- **Approach:** Enable `errorlint`, `misspell`, `bodyclose`, `nilerr`,
  `nilnil` in `server/.golangci.yml`; land all matching baseline fixes
  in the same PR so `npm run lint:go` is green; file a tracker issue
  for Wave 2 (`goconst`, `funlen`, `cyclop`, `unparam`) with the
  thresholds and known violators already enumerated.
- **Pros:** Matches the review's explicit two-wave recommendation.
  PR stays small enough to read end-to-end. Each wave's fixes are
  isolated, so a regression in one batch doesn't roll back the other.
  Wave 1 closes 5 known production `errors.Is` bugs immediately.
- **Cons:** Two PRs and two follow-up issues instead of one. The
  Wave 2 work has to wait until someone picks up the follow-up.
- **Effort:** small (~hours). Wave 1 has limited fix surface — 5
  production `errorlint` sites with a clear playbook, ~50 `nilnil`
  sites of which most resolve to `//nolint` with rationale, plus
  whatever `misspell` finds.

### Option B: Both waves in one PR, separate commits

- **Approach:** Same config changes as A plus the Wave 2 linters and
  thresholds, with `funlen=100` and `cyclop=15`. Grandfather every
  current violator with `//nolint:funlen,cyclop` directives carrying
  a tracking issue link, mirroring the `dart-line-count-allow` hatch
  pattern. Commit by wave for review.
- **Pros:** Closes the entire gap in one go. No follow-up needed.
- **Cons:** PR is large and noisy — `funlen`/`cyclop` will fire on
  many functions in `prompts.go`, `main.go`, gear `Service`, etc.,
  each needing a `//nolint` with rationale. `goconst` is likely to
  surface 20+ literals. Reviewer fatigue risk; harder to revert one
  wave without the other. Conflicts with the review's own staging
  recommendation.
- **Effort:** medium (~1–2 days), most of it spent on grandfathering
  and rationale comments, not real fixes.

### Option C: Wave 1 plus low-noise subset of Wave 2 (`unparam`, `goconst`)

- **Approach:** Wave 1 as in A, plus `unparam` and `goconst`; defer
  only `funlen` and `cyclop` to a follow-up because those are the
  ones with truly large grandfathering surface.
- **Pros:** Slightly more leverage per PR than A. `unparam` typically
  has small finding counts; `goconst` would mechanize the
  material-category strings the review keeps calling out.
- **Cons:** Mixes a "needs grandfathering" linter (`goconst` will
  almost certainly hit material-category strings) into the
  low-noise PR, reintroducing the noise the two-wave split is
  designed to avoid. Loses the clean "Wave 1 == green on first run"
  property.
- **Effort:** small-to-medium.

## Recommendation

**Option A.** It matches the explicit recommendation in the issue body
and the source review, keeps the diff small enough that a reviewer can
sanity-check every fix, and delivers concrete value (5 production
`errors.Is` corrections) on the first wave. The follow-up issue for
Wave 2 is cheap to file and gives the team a place to argue thresholds
(`funlen=100`? `cyclop=15`?) without holding up Wave 1.

Two specific tightenings for Option A:

1. **Add `prealloc`** to Wave 1 alongside the five named linters. The
   review's tooling table lists it ("Small perf wins") and tags it as
   a low-noise addition. If the first run produces more than ~10
   findings, drop it back out and roll into Wave 2; otherwise it's
   essentially free.
2. **Introduce `var ErrRecordNotFound` once,** in `server/storage/errors.go`,
   so the production `errorlint` fixes all share one sentinel. Don't
   thread it through every `_test.go` site — exempt tests via the
   existing `path: _test\.go` exclusion if `errorlint` test noise is
   high.

## Open Questions

- **Should `nilnil` be configured with `checked-error-types: true` or
  similar tuning?** — Recommendation: keep defaults. The default
  config is "flag any `return nil, nil` pair where the second return
  is an error type." Tuning is premature until we see the noise floor.
- **Should `errorlint` enable `errorf`, `asserts`, and `comparison`
  all at default-on?** — Recommendation: yes (the linter's defaults).
  All three subchecks are valuable; if any one of them produces
  unmanageable noise we can disable it in `linters-settings.errorlint`.
- **Test files: opt out, or apply the same rules?** — Recommendation:
  add `_test\.go` exclusions for `errorlint` and `nilnil` only. Tests
  legitimately use `strings.Contains(err.Error(), …)` for assertion
  shape and may legitimately return `nil, nil` from helper builders.
  Keep `misspell`, `bodyclose`, `nilerr`, `prealloc` applied
  everywhere — tests deserve the same hygiene there.
- **Where to file the Wave 2 issue?** — Recommendation: a new GitHub
  issue titled `[code_quality] Wave 2: enable funlen, cyclop, goconst,
  unparam` referencing this issue. Include the known violators list
  (`server/main.go`, `server/ai/prompts.go`,
  `server/services/gear/service.go`) and proposed thresholds
  (`funlen=100`, `cyclop=15`).
- **Does `bodyclose` work cleanly with our HTTP plumbing?** —
  Recommendation: yes, expected. The review explicitly says current
  callers use `defer resp.Body.Close()` correctly. The PR will
  surface any false positives during the first lint run.

## Implementation Plan

### Phase 1: Enable Wave 1 linters

**Goal:** `server/.golangci.yml` enables the five Wave-1 linters plus
`prealloc`, with appropriate test-file exclusions, and `npm run lint:go`
fails before any baseline fixes land.

**Tasks:**
- [ ] Edit `server/.golangci.yml` to add `errorlint`, `misspell`,
      `bodyclose`, `nilerr`, `nilnil`, `prealloc` to `linters.enable`
      (alphabetized into the existing list).
- [ ] Add to `issues.exclude-rules`:
      - `path: _test\.go` excludes `errorlint` and `nilnil`.
      - Keep the existing `path: gen/` blanket exclude.
- [ ] No `linters-settings` overrides in this phase (use defaults).
- [ ] Run `golangci-lint run ./...` from repo root to capture the
      violation list. Save it as the baseline for Phase 2.

**Validates:** The config edit is syntactically valid and the linters
load. The baseline violation list bounds the work in Phase 2.

### Phase 2: Baseline fixes — `errorlint`

**Goal:** Replace the `strings.Contains(err.Error(), "record not found
…")` pattern with a sentinel error matched by `errors.Is`, and clear
the other production `errorlint` violations.

**Audit note:** `server/storage/testing.go` is **not** a `_test.go`
file (it is exported test-scaffolding used from tests in other
packages). The proposed `_test\.go` exclusion from Phase 1 does **not**
cover it, so its `errorlint` findings have to be fixed for real, not
suppressed.

**Audit note:** the actual emit site is `server/storage/protosql.go:117`
(`fmt.Errorf("record not found with id: %s", id)`). That is the line
that has to wrap `ErrRecordNotFound`. The same function at
`server/storage/protosql.go:116` does `if err == sql.ErrNoRows` — also
an `errorlint` violation; fix it to `errors.Is(err, sql.ErrNoRows)` in
the same edit.

**Tasks:**
- [ ] Create `server/storage/errors.go` declaring
      `var ErrRecordNotFound = errors.New("storage: record not found")`.
      Add the godoc `// ErrRecordNotFound is returned when a Get/Update
      target is missing.` per `revive.exported` (CLAUDE.md style
      guidelines). The package already has `server/storage/doc.go`, so
      no new package comment is needed. Pattern matches the existing
      `var ErrEmbeddingNotConfigured` in `server/storage/embedding.go`.
- [ ] In `server/storage/protosql.go`:
      - Line 116: `err == sql.ErrNoRows` → `errors.Is(err, sql.ErrNoRows)`.
      - Line 117: `fmt.Errorf("record not found with id: %s", id)` →
        `fmt.Errorf("record not found with id %s: %w", id, ErrRecordNotFound)`.
- [ ] Update `server/storage/cascade_delete.go:14` to
      `return errors.Is(err, ErrRecordNotFound)`.
- [ ] Update `server/storage/testing.go`:
      - Lines 456, 546 (`"EOF"`): match with `errors.Is(err, io.EOF)`.
      - Lines 599, 609 (`"connection refused"`): match with
        `errors.Is(err, syscall.ECONNREFUSED)`. If the wrapped
        `*net.OpError` swallows that on the runtime, use `errors.As`
        on `*net.OpError` instead and pull the inner error. Last
        resort: `//nolint:errorlint // upstream net error string is
        the only stable signal here` with a one-sentence rationale,
        but try the structural match first per CLAUDE.md fail-fast.
- [ ] Sweep any other production `errorlint` findings the first
      `golangci-lint run` surfaces (`fmt.Errorf("…: %s", err)` →
      `"…: %w"`; `err == ErrFoo` → `errors.Is`).
- [ ] Add `server/storage/errors_test.go` covering `errors.Is`
      matching on a wrapped `ErrRecordNotFound` from
      `ProtoSQLStorage.Get` against a missing id, plus a direct
      `errors.Is(fmt.Errorf("…: %w", ErrRecordNotFound),
      ErrRecordNotFound)` round-trip.

**Validates:** `golangci-lint run ./...` passes for `errorlint` across
the whole tree (production + `server/storage/testing.go`); the five
known sites and `protosql.go:116` are now `errors.Is`-driven.

### Phase 3: Baseline fixes — `nilnil`, `misspell`, `bodyclose`, `nilerr`, `prealloc`

**Goal:** Resolve every remaining Wave-1 finding so `npm run lint:go`
is green.

**Tasks:**
- [ ] `nilnil`: For each production site, classify as either
      (a) genuine "not found" → return a sentinel like
      `ErrNotFound` defined in the package (CLAUDE.md fail-fast
      convention prefers an explicit sentinel over an ambiguous
      `nil, nil`), or (b) intentional "nothing to do" → add
      `//nolint:nilnil // <one-sentence reason>` per the existing
      `//nolint` convention. Aim for classification first,
      suppression as fallback. Known clusters to triage first:
      `server/auth/users.go:82,121`, `server/product/lookup.go`
      (multiple), `server/services/location/deduplication.go:39,81`,
      `server/impact_metrics/metric_detail_*.go`,
      `server/services/community/notification_prefs.go:114`.
- [ ] `misspell`: accept the autofixes (`golangci-lint run --fix`)
      and eyeball the diff.
- [ ] `bodyclose`: expected zero or near-zero findings; fix any.
- [ ] `nilerr`: expected zero or near-zero findings; fix any.
- [ ] `prealloc`: if more than ~10 findings, drop the linter from
      this PR and add it to the Wave 2 follow-up. Otherwise apply
      the suggested capacity hints.
- [ ] If `errorlint` test-file noise is acceptable (under ~20
      findings), drop the `_test\.go` exclusion for it. Otherwise
      keep the exclusion.

**Validates:** `npm run lint:go` exits 0. `npm run build` (which
chains `lint:go` and `go build ./...`) exits 0.

### Phase 4: Wave 2 follow-up issue + verification

**Goal:** File the Wave 2 tracker and prove the gate holds in CI.

**Tasks:**
- [ ] Run `npm run build`.
- [ ] Run `go test -tags=integration -timeout 20m ./...` to confirm no
      behavior regressions from the `errorlint` refactor.
- [ ] File a new GitHub issue
      `[code_quality] Wave 2: enable funlen, cyclop, goconst, unparam`
      referencing this issue, with the known violators list and the
      proposed thresholds (`funlen=100`, `cyclop=15`).
- [ ] Update `docs/reviews/reports/code_quality_review.md` only if a
      review pass is in scope; otherwise leave for the next
      `/ripls-reviews` run to pick up.

**Validates:** CI's `Lint Go code` step (currently `gofmt -d .` in
`.github/workflows/test_go.yaml`) is unaffected, but contributors
running `npm run lint:go` locally now see the new gate. `npm run
build` is the durable enforcement point per CLAUDE.md.

**Audit note (CI gating):** today only `gofmt -d .` runs in the Go test
workflow — `golangci-lint` is not actually enforced in CI. The
implementation must not assume otherwise. If wiring a CI job for
`golangci-lint run ./...` is in scope, do it as a separate step in
this PR or as a follow-up; either way, call it out in the PR
description so the reviewer makes the explicit call. Without it, the
new linters protect contributors who run `npm run build` but not
contributors who don't.

## Related Issues

- Source review: `docs/reviews/reports/code_quality_review.md` lines
  279–302 (linter config gap), 437–439 (tooling table rows 2 and 3).
- Previously-resolved tooling moves: `dupl` (threshold 150),
  `forbidigo` (banning `time.Sleep` outside tests). This issue
  continues that thread.
- Wave 2 follow-up (to be filed in Phase 4): `funlen`, `cyclop`,
  `goconst`, `unparam` with thresholds and grandfathering plan.
