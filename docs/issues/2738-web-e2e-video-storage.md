# Issue #2738: CI: web-e2e records video for every test → 97.5% of the 2 GB org Actions-storage quota

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2738
**Filed:** 2026-07-17 | **Last activity:** 2026-07-17
**Labels:** none | **Assignees:** unassigned
**Context docs:** `docs/testing/architecture.md` (⚓ testing anchor), `docs/client/testing.md` (e2e boundary), `docs/walkthroughs.md` (governs `e2e/lib/capture.ts`, the video-capture recipe), `docs/proto_conventions.md` (anchor — not engaged, no proto change)

## Problem Statement

The `Type2Fun-Foundation` org is at **90% of the included 2 GB GitHub Actions storage** (1.8 GB used, resets 2026-08-01). Once the included quota is exceeded we're billed for overage, or — if a $0 budget is set — further Actions usage is blocked until reset. This has bitten us before, so the fix must be a durable prevention, not just a purge.

The storage is almost entirely two artifacts uploaded by `test_web_e2e.yaml`:

| Artifact | Live count | Total MB | Per-run |
|---|---|---|---|
| `web-e2e-test-results` (videos/traces/screenshots) | 10 | **1082** | ~108 MB |
| `web-e2e-playwright-report` | 10 | **693** | ~69 MB |
| `go-test-output` | 11 | 24 | — |
| `web-e2e-server-log` | 10 | 13 | — |
| `gitleaks-results.sarif` | 329 | 2 | tiny (but 329 objects) |

Together the two web-e2e artifacts are **~1775 MB, ~97.5% of live Actions storage.**

The uploads are already gated on `if: failure()` (a prior fix — see the comment at `test_web_e2e.yaml:204-207`), so they only come from *failing* runs. The remaining problem is that **each failing run uploads ~108 MB of video** because Playwright records video for *every test in the run*, not just the failing one.

> **Immediate mitigation already applied (2026-07-17):** the 20 accumulated heavy artifacts (1775 MB) were deleted via the GitHub API, dropping live Actions storage to ~44 MB. That is a one-time purge; without the code fix below the videos re-accumulate on the next failing runs.

## Context from Comments

The issue has no GitHub comments yet (filed as part of this task). However, the repo owner gave direct in-session direction that IS the decision:

> "let's have that not produce videos except on failure — at some point I switched it to always save the videos."

So the resolution is settled: **record video only on failure in CI**, restoring the prior behavior. The "always save videos" mode was a deliberate local-dev affordance that leaked into CI. No design debate remains; this plan is about doing it correctly (there are two code sites, not one) without breaking the local watch flow or the walkthrough reel pipeline.

## Root Cause

`e2e/playwright.config.ts` records video unconditionally in two places, both of which reach the CI projects:

1. **Global `use.video: 'on'`** (~line 79) → applies to the **`webkit`** CI project (it doesn't set its own `video`, so it inherits the global).
2. **`phoneVideoCaptureChromium.video: { mode: 'on', size: PHONE_VIDEO_SIZE }`** (~line 39) → applies to the **`mobile-chromium`** CI project (the primary gate) and is *also* spread into the `walkthrough` project.

The web-e2e CI workflow runs `--project=mobile-chromium --project=webkit` (`test_web_e2e.yaml:202`). When any spec in the run fails, the `if: failure()` step uploads the **entire** `e2e/test-results/` directory — which, under `video: 'on'`, holds a clip for every test that ran, not just the failing one → ~108 MB. Roughly 10 failing runs sit inside the 7-day retention window → ~1.8 GB.

`trace: 'retain-on-failure'` and `screenshot: 'only-on-failure'` are already correct. **Video is the sole outlier**, and it must be gated in *both* sites — changing only the global `use.video` would leave `mobile-chromium` (the main CI project) still recording everything, because the project-level `phoneVideoCaptureChromium.video` overrides the global.

Constraints that must not regress:
- **Local dev:** `video: 'on'` locally is intentional — `global-teardown.ts` flattens every clip into `e2e/videos/` for easy watching (`playwright.config.ts:8-16`, `global-teardown.ts:21-29`). Keep local behavior at `'on'`.
- **Walkthrough/reel deliverables:** the `walkthrough` project exists to *produce* watchable videos on pass (#2684, epic #2492). `docs/walkthroughs.md:325` confirms it runs under `--project=walkthrough` and **never gates CI**. Its video must stay `'on'` unconditionally, independent of any CI gate. (Note: `playwright.config.ts` has exactly three projects — `mobile-chromium`, `webkit`, `walkthrough`. There is **no** `marketing-video` project; the name in the `test_web_e2e.yaml:196` comment is stale — it was renamed to `walkthrough` under #2684.)

## Potential Solutions

### Option A: CI-gate the video mode in `playwright.config.ts` (recommended)
- **Approach:** Introduce a single derived constant, e.g. `const videoMode = process.env.CI ? 'retain-on-failure' : 'on';`. Apply it to (1) global `use.video` and (2) `phoneVideoCaptureChromium.video.mode`. Give the `walkthrough` project an **explicit** `video: { mode: 'on', size: PHONE_VIDEO_SIZE }` override so the deliverable never depends on the CI-gated helper.
- **Pros:** Fixes the root cause at the source. On a failing CI run only the failing test keeps a clip (artifact drops from ~108 MB to single-digit MB). Local watch flow and reel pipeline both preserved. One small, well-understood file.
- **Cons:** Slightly more nuance in the config (two gated sites + one explicit override). A future reader must understand why `walkthrough` re-pins video — mitigated with a comment.
- **Effort:** Small.

### Option B: Prune the uploaded directory in the workflow before upload
- **Approach:** Keep `video: 'on'`, but in `test_web_e2e.yaml` add a failure-time step that deletes passing tests' videos from `e2e/test-results/` before `upload-artifact` (or upload a curated subdirectory).
- **Pros:** No config change; the local `e2e/videos/` collection is unchanged in CI too.
- **Cons:** Brittle — depends on Playwright's per-test output-dir layout and on distinguishing pass vs. fail folders in shell. Still *records* all video (wasted runner CPU/IO on the self-hosted runner). Solves the symptom, not the cause. The owner explicitly asked to "not produce videos except on failure" — i.e. stop recording, which this doesn't do.
- **Effort:** Medium, and higher maintenance risk.

### Option C: Just lower retention / manual purge, no recording change
- **Approach:** Drop `retention-days` and periodically delete artifacts.
- **Pros:** Trivial.
- **Cons:** Doesn't fix the cause — a burst of failing runs in a short window still blows the quota because each is ~108 MB. Contradicts the owner's decision. Rejected as a standalone fix.

## Recommendation

**Option A.** It directly implements the owner's decision, eliminates ~97% of the storage at the source, and preserves both the local watch affordance and the reel pipeline. Pair it with two cheap, non-behavioral safety margins from the secondary list (lower retention; keep the deliverable override explicit). Option B/C are inferior — B doesn't stop the recording the owner asked to stop, and C doesn't fix the cause.

The one-time purge of already-accumulated artifacts (see the mitigation note above) has already been done to get back under quota; it is **not** part of the code change and doesn't gate the PR.

## Open Questions

- **CI video mode: `retain-on-failure` vs. `off`?** — Recommendation: `retain-on-failure`. A failing test's own video is often the fastest triage signal, and the cost is tiny (only failing tests, and only on failure runs). `off` would save marginally more but removes useful debugging footage; `retain-on-failure` matches the existing `trace`/`screenshot` policy for symmetry.
- **Lower `retention-days` (7 → 3) for the two heavy web-e2e artifacts?** — Recommendation: yes, do it in this PR. It's a one-line-each belt-and-suspenders bound so even shrunken failure artifacts age out faster; zero behavioral risk. Leave `go-test-output`/`server-log` at 7 (they're small).
- **Touch `gitleaks-results.sarif` (329 live objects, ~2 MB)?** — Recommendation: no, out of scope. It's storage-negligible; the object count is cosmetic. If desired, file a separate tiny follow-up to add a short retention to the gitleaks upload — don't couple it to this fix.
- **Should CI ever keep the HTML report on green?** — Recommendation: no change. It's already `if: failure()` only; that's correct.

## Implementation Plan

### Phase 1: Stop recording pass-video in CI (the fix)
**Goal:** A failing web-e2e CI run uploads only the failing test's video; green runs upload nothing; local + reel behavior unchanged.
**Tasks:**
- [ ] In `e2e/playwright.config.ts`, add `const videoMode = process.env.CI ? 'retain-on-failure' : 'on';` (with a short comment: CI keeps only failure clips to bound the org Actions-storage quota, #2738; local keeps all clips for `global-teardown.ts` → `e2e/videos/`).
- [ ] Set global `use.video: videoMode` (drives the `webkit` project).
- [ ] Set `phoneVideoCaptureChromium.video: { mode: videoMode, size: PHONE_VIDEO_SIZE }` (drives `mobile-chromium`). Keep the `as const` typing valid for Playwright's union.
- [ ] Give the `walkthrough` project an explicit `video: { mode: 'on' as const, size: PHONE_VIDEO_SIZE }` override (after the `...phoneVideoCaptureChromium` spread) so the reel deliverable always records regardless of `CI`, with a comment referencing #2684/#2492 and `docs/walkthroughs.md`.
- [ ] Update the header comment block (`playwright.config.ts:8-16`) so the "video recorded for EVERY test" prose reflects the new CI/local split.
**Validates:** Root cause removed at the source; the ~108 MB-per-failure artifact shrinks to single-digit MB.

### Phase 2: Bound retention as a safety margin
**Goal:** Even shrunken failure artifacts age out fast; a burst of failures can't silently re-approach the quota.
**Tasks:**
- [ ] In `test_web_e2e.yaml`, lower `retention-days: 7` → `3` on the `web-e2e-test-results` and `web-e2e-playwright-report` upload steps. Leave `web-e2e-server-log` and `go-test-output`/`go-test-output-*` as-is (small).
- [ ] (In-scope freebie — same file) Fix the stale `test_web_e2e.yaml:196` comment that references "the `marketing-video` project"; that project was renamed to `walkthrough` under #2684. Update it to name the `walkthrough` project so the comment matches the config.
**Validates:** Defense in depth independent of the recording change.

### Phase 3: Verify and document
**Goal:** Prove the split works both ways without breaking local watch or the reel pipeline.
**Tasks:**
- [ ] Local: run a mobile-chromium spec with no `CI` env; confirm a video lands in `e2e/videos/` (unchanged `'on'` behavior) and `global-teardown.ts` logs `collected N video(s)`.
- [ ] CI-mode passing: run `CI=true npx playwright test --project=mobile-chromium <a passing spec>`; confirm `e2e/test-results/` has **no** `.webm` for the passing test.
- [ ] CI-mode failing: force one spec to fail under `CI=true`; confirm only that test's `.webm` is retained.
- [ ] Reel: run `npx playwright test --project=walkthrough <one @walkthrough spec>` (or `e2e/scripts/run_walkthrough.sh <slug>`) and confirm video still records on pass.
- [ ] If any prose in `docs/walkthroughs.md` / `docs/client/testing.md` now describes video capture inaccurately, update it (the nightly doc-freshness ratchet governs `e2e/lib/capture.ts`; keep claims true).
- [ ] Run `npm run lint` (includes `lint:context-map`) to confirm no doc/lint gate regressed.
**Validates:** End-to-end correctness of the CI/local/reel three-way split.

### Out of band (operational — already done)
One-time purge that got live storage from 1820 MB → 44 MB (2026-07-17):
```bash
gh api "repos/Type2Fun-Foundation/ripls/actions/artifacts" --paginate \
  | jq -r '.artifacts[] | select(.expired==false)
           | select(.name=="web-e2e-test-results" or .name=="web-e2e-playwright-report")
           | .id' \
  | xargs -I{} gh api -X DELETE "repos/Type2Fun-Foundation/ripls/actions/artifacts/{}"
```
(GitHub's billing/storage UI lags the actual API state by a cycle, so the reported percentage takes a while to drop.)

## Acceptance Criteria
- A failing web-e2e CI run uploads only the failing test's video (artifact single-digit MB, not ~108 MB).
- A green web-e2e CI run uploads no video/report artifacts (already true; unchanged).
- Local `npx playwright test` still records every clip; `global-teardown.ts` still populates `e2e/videos/`.
- The `walkthrough` project still records video on pass; reel pipeline unaffected.
- No other workflow uploads large artifacts without both `if: failure()` and a bounded `retention-days`.

## Related Issues
- (prior art) The `if: failure()` gating already in `test_web_e2e.yaml:204-235` was an earlier partial fix for this same quota pressure; this issue completes it by stopping pass-video recording.
- Potential tiny follow-up (not filed): short retention on the gitleaks `results.sarif` upload to trim its 329-object count.

## Audit Notes (`/ripls-audit`, 2026-07-17)

**Scope:** Infra/CI change touching `e2e/playwright.config.ts` and `.github/workflows/test_web_e2e.yaml`. Side: client-adjacent (e2e harness), no `app/lib/**` or `server/**` runtime code. **Engaged dimensions:** `testing`, `tech_debt`/`code_quality`, infra/CI. **Not engaged:** security (no secrets/auth/PII/SSRF), api_design (no proto/RPC), accessibility (no UI), performance (no hot path), dependency (no new deps).

**Findings applied:**
1. **Non-existent project reference (fixed).** The draft named a `marketing-video` project alongside `walkthrough`. Verified against `playwright.config.ts` — only three projects exist (`mobile-chromium`, `webkit`, `walkthrough`). Corrected the plan to reference `walkthrough` only.
2. **Stale workflow comment surfaced (task added).** `test_web_e2e.yaml:196` still names "the `marketing-video` project" (renamed to `walkthrough` under #2684; the marketing/sizzle vocabulary was retired). Added an in-scope Phase-2 task to fix it, since that file is already being edited — keeps the comment true to the config per the doc-freshness convention.

**Verified accurate (no change needed):** all cited line refs (config `39`/`79`, workflow `196`/`202`/`204-235`); `trace: 'retain-on-failure'` + `screenshot: 'only-on-failure'` already correct; the two-site override relationship (project-level `phoneVideoCaptureChromium.video` shadows global `use.video`); `global-teardown.ts` collecting to `e2e/videos/`; `docs/walkthroughs.md:325` "never gate CI". The `videoMode` typing (`process.env.CI ? 'retain-on-failure' : 'on'`) is a valid Playwright `VideoMode` union and matches the file's existing `process.env.CI` gating idiom (`forbidOnly`, `retries`, `workers`).

**Testing convention check:** This is config, not a library function, so CLAUDE.md's "unit test each new library function" rule doesn't apply; Phase 3's four-way manual verification (local / CI-pass / CI-fail / reel) is the right validation for a Playwright config change per `docs/testing/architecture.md`.

**Docs consulted:** `docs/llms.txt` (map), `docs/testing/architecture.md` (⚓), `docs/client/testing.md`, `docs/walkthroughs.md` (governs `e2e/lib/capture.ts`), plus direct verification against `e2e/playwright.config.ts`, `e2e/global-teardown.ts`, and `.github/workflows/test_web_e2e.yaml`.
