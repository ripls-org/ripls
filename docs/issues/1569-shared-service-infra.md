# Issue #1569: Move shared service infra out of server/services/ into server/ libraries

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1569
**Filed:** 2026-04-29 | **Last activity:** 2026-04-29
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`server/services/service_isolation_test.go` enforces that no service may import another service — services must depend on shared libraries instead, per `docs/server/architecture.md` §"Service Independence". Three packages physically live under `server/services/` but act as shared infrastructure:

- `server/services/streaming/` — generic Connect server-streaming sender; imported by community, experience, gear, request. **Genuinely a library** (3 files, no RPC handler logic).
- `server/services/fanout/` — concurrency primitive for parallel post-AI work in Gen* RPCs; imported by experience, gear, request. **Genuinely a library** (2 files, pure concurrency primitive).
- `server/services/impact_metrics/` — full RPC service (~25 files, implements `ImpactMetricsService`). The *only* cross-service consumer is `server/services/experience/lifecycle_test.go`, which constructs `impact_metrics_svc.NewService(...)` to drive a `DraftImpactEstimate` → `CompleteExperience` end-to-end test. `server/main.go` also imports the service for wiring, but `TestServiceIsolation` only walks `server/services/` so wiring is not flagged. The impact_metrics service is *not* shared infrastructure — it's a normal RPC service that happens to be exercised by an experience test.

Because the three packages live under `server/services/`, the isolation test grandfathers each cross-service edge in a `knownAllowedServiceImports` map. PR #1567 (community → streaming) was the most recent dilution. The fix the test itself recommends is in its error message: "move shared code to a library package under `server/`."

Worth flagging up front: the existing convention in this repo is for a library and a sibling service to share a directory name (`server/community/` library + `server/services/community/` service; `server/chat/` library + `server/services/chat/` service; `server/impact_metrics/` library + `server/services/impact_metrics/` service). The Go module path makes them distinct. So "moving impact_metrics service to `server/impact_metrics/`" isn't a literal move — the library is already there.

## Context from Comments

The issue has a single `@claude` summon and no substantive discussion. The acceptance criteria already in the body are clear:

- `knownAllowedServiceImports` is empty (or the map+test simplified) once all three packages are out.
- `TestServiceIsolation` still passes.
- No `server/services/<X>` import exists from another service.

The body proposes three independent moves and suggests they can ship as three small PRs. Two wrinkles are **not** called out in the issue:

1. The impact_metrics service is not shared infrastructure — it's a normal RPC service. The cross-service violation is one test (`server/services/experience/lifecycle_test.go`) that drives a draft → commit flow. So "move the service to a library" misframes the problem; the actual fix is to remove the test's direct cross-service dependency, which can be done by moving the test rather than the service.
2. `server/impact_metrics/` already exists as a sibling library package and the service-side package at `server/services/impact_metrics/` shares the directory name. The codebase pattern is intentionally that library + sibling service share names (see `server/community/` + `server/services/community/`, `server/chat/` + `server/services/chat/`). So a literal `git mv server/services/impact_metrics server/impact_metrics` would clobber the existing library.

## Potential Solutions

### Option A: Three independent PRs, treating each package on its merits

- **Approach:**
  1. **PR-A1 (streaming):** `git mv server/services/streaming server/streaming`; rewrite imports across community, experience, gear, request; drop the streaming entry from `knownAllowedServiceImports`; update `server/cmd/check-goroutines/main.go` allowlist (`server/services/streaming/sender.go` → `server/streaming/sender.go`).
  2. **PR-A2 (fanout):** `git mv server/services/fanout server/fanout`; rewrite imports across experience, gear, request; drop the fanout entry.
  3. **PR-A3 (impact_metrics):** *Don't* move the service. Instead, move the cross-service test out of the experience service. Specifically: extract `TestCompleteExperience_DraftThenOverrideThenCommit` (the only test using `impact_metrics_svc.NewService`) to `server/integration_tests/experience_impact_test.go` (package `integration_tests`), where the isolation rule does not apply. Then drop the impact_metrics entry. The service stays at `server/services/impact_metrics/`, sibling to the existing `server/impact_metrics/` library — consistent with the community/chat/conversation library+service convention.
- **Pros:** Each PR is small and independently revertible. Bisect-friendly. Matches the issue author's suggested cadence. PR-A3 is honest about the architectural reality: impact_metrics is a service, not infrastructure, and the violation is a test artifact.
- **Cons:** Three review cycles. Minor merge churn between PRs (each regenerates the map). PR-A3 deviates from the issue body's "move it to `server/impact_metrics/`" — needs reviewer buy-in.
- **Effort:** Small per PR. PR-A3 is the smallest of the three (one file moved, one entry removed).

### Option B: Move all three packages literally (per the issue body), renaming impact_metrics to avoid collision

- **Approach:** PR-B1 and PR-B2 same as A1 and A2. PR-B3: move `server/services/impact_metrics/` to a new top-level location that doesn't collide with the existing library. Candidate target: `server/impact_metrics/service/` (sub-package), with the Go package renamed to `metricsservice` so callers can import both library and service unambiguously. Update `server/main.go:57` and `server/services/experience/lifecycle_test.go:16` accordingly.
- **Pros:** Closer to the literal recommendation in the issue body. Treats impact_metrics symmetrically with streaming/fanout. No need to refactor or relocate the experience test.
- **Cons:** Misclassifies impact_metrics as shared infrastructure when it's actually a service. Adds a `service/` sub-package under a library — an unusual layout that doesn't appear elsewhere in the codebase. Requires renaming the Go package (mechanical but touches every file in the service). Sets a precedent that any cross-service test dependency should be answered by relocating one of the services, which is the opposite of the architecture's intent.
- **Effort:** Larger than Option A — A3's "move one test" is ~40 lines of edits, while B3's "move the whole service" is ~30 files renamed plus all internal alias adjustments.

### Option C: One combined PR

- **Approach:** Whichever shape (A or B) is chosen, ship as one PR. Empty out `knownAllowedServiceImports` entirely (or delete it and inline the simpler test).
- **Pros:** One review; one CI run; the map ends in its final empty form rather than going through intermediate states.
- **Cons:** Larger diff. Harder to bisect if anything regresses. Conflates three independent decisions into one review.
- **Effort:** Same total work as A or B, compressed.

## Recommendation

**Option A** — three independent PRs, with PR-A3 moving the cross-service test rather than the service. Reasons:

- The issue author framed this as "three small PRs"; matching that cadence is the lowest-friction path for review.
- Streaming and fanout are genuinely shared infrastructure (generic Connect plumbing, generic concurrency primitive); moving them is mechanical and architecturally correct.
- The impact_metrics service is **not** shared infrastructure — it's an RPC service like chat or community. The cross-service violation is a single test, not a design problem with the service. Moving the test out of `server/services/experience/` (to `server/integration_tests/experience_impact_test.go`) is the surgical fix and matches the existing pattern: cross-service end-to-end tests already live in `server/integration_tests/` (e.g. `experience_test.go`, `loan_test.go`).
- This avoids inventing a new layout (`server/impact_metrics/service/`) that doesn't exist anywhere else in the codebase, and avoids renaming a Go package across ~25 files just to dodge a name collision the codebase otherwise tolerates.
- If anything regresses, only one of the three is affected. Bisect is cheap.

If review prefers the literal interpretation of the issue body, fall back to Option B. If review prefers fewer round-trips, compress into Option C around either the A or B shape.

## Open Questions

- **Is moving the test (Option A3) preferable to moving the service (Option B3)?** — Recommendation: yes. The impact_metrics service is a service by every other measure (it implements an `ImpactMetricsService` RPC interface, has a dedicated `service.go` constructor, has its own README that describes it as the RPC service). Calling it "shared infrastructure" because one cross-service test imports it inverts the architecture's logic. The cleanest, smallest, most architecturally honest fix is to move that one test to `server/integration_tests/`, which exists for exactly this purpose. If review disagrees, Option B3 is feasible — it's just bigger and introduces an unusual layout.
- **If we go with Option A3, what file should the relocated test live in?** — Recommendation: `server/integration_tests/experience_impact_test.go` (new file), package `integration_tests`. The existing `experience_test.go` in that directory already tests experience flows; the impact-specific draft+commit test fits as a sibling rather than mixing into that file. The new file imports both `server/services/experience` and `server/services/impact_metrics`, which is exactly the situation `integration_tests` is designed for.
- **Should `knownAllowedServiceImports` plus `TestServiceIsolation`'s allowlist branch be deleted, or just emptied?** — Recommendation: delete the map and the branch that consults it. Once all three exceptions are gone, the test simplifies to "any cross-service import fails", which is the durable rule. Empty maps invite re-population.
- **Should the streaming move also rename `package streaming` to something more specific (e.g. `connectstream`)?** — Recommendation: no. `package streaming` reads cleanly when imported as `streaming.NewSender(...)`, and no other top-level `server/streaming/` exists. Same for `fanout`.
- **Does anything else under `server/services/` belong in libraries?** — Recommendation: out of scope for this issue. `roundtrip_helpers.go`, `experience_converters.go`, `test_util.go`, `user_util.go` live at the `server/services/` top level (not inside a sub-service) and are already shared library-shaped code. They are not flagged by `TestServiceIsolation` and changing them would expand the issue.

## Implementation Plan

### Phase 1: Move `streaming`

**Goal:** Eliminate the four-service streaming exception.
**Tasks:**
- [ ] `git mv server/services/streaming server/streaming` (single command preserves history for both `doc.go` and `sender.go` + `sender_test.go`).
- [ ] Rewrite imports in callers: `server/services/community/gen_streaming.go:22`, `server/services/experience/gen_ai_streaming.go:27`, `server/services/gear/gen_ai_streaming.go:28`, `server/services/request/gen_streaming.go:24` — change `"type2fun.org/ripls/server/services/streaming"` to `"type2fun.org/ripls/server/streaming"`.
- [ ] Update the goroutine-recovery allowlist at `server/cmd/check-goroutines/main.go:32` from `"server/services/streaming/sender.go"` to `"server/streaming/sender.go"`.
- [ ] Remove the `"server/services/streaming"` entry from `knownAllowedServiceImports` in `server/services/service_isolation_test.go`.
- [ ] Run `npm run build` and `go test -tags=integration -timeout 20m ./server/streaming/... ./server/services/...` to confirm.

**Validates:** `TestServiceIsolation` still passes with one fewer exception; the four caller services compile and their tests pass.

### Phase 2: Move `fanout`

**Goal:** Eliminate the three-service fanout exception.
**Tasks:**
- [ ] `git mv server/services/fanout server/fanout`.
- [ ] Rewrite imports in callers: `server/services/experience/gen_ai.go:17`, `server/services/experience/gen_ai_streaming.go:26`, `server/services/experience/gen_ai_fanout.go:12`, `server/services/gear/gen_fanout.go:11`, `server/services/request/gen_fanout.go:12`, `server/services/request/gen.go:19`, `server/services/request/gen_streaming.go:23`.
- [ ] Remove the `"server/services/fanout"` entry from `knownAllowedServiceImports`.
- [ ] Run `npm run build` and `go test -tags=integration -timeout 20m ./server/fanout/... ./server/services/...`.

**Validates:** `TestServiceIsolation` passes with one more exception removed; gen-flow tests for experience/gear/request still pass.

### Phase 3: Move `TestCompleteExperience_DraftThenOverrideThenCommit` to `server/integration_tests/`

**Goal:** Eliminate the final exception by relocating the cross-service test, leaving the impact_metrics RPC service in place.
**Tasks:**
- [ ] Create `server/integration_tests/experience_impact_test.go` (`package integration_tests`). Move `TestCompleteExperience_DraftThenOverrideThenCommit` from `server/services/experience/lifecycle_test.go` into this file.
- [ ] Adjust imports in the moved test: it will now need to import both `server/services/experience` and `server/services/impact_metrics`, plus whatever helpers it currently uses from `setupTestService(t)`. If `setupTestService` is unexported in the experience package, either (a) export an equivalent helper from the experience service for integration use or (b) replicate the bootstrap inline using `storage.SetupTestStorage(t)` and direct service constructors. Pick (a) if the helper is non-trivial; pick (b) if it's small.
- [ ] Remove the original test (and its now-unused `impact_metrics_svc` import) from `server/services/experience/lifecycle_test.go`. Verify no other tests in that file reference the removed test's helpers.
- [ ] Delete `knownAllowedServiceImports` and the branch in `TestServiceIsolation` that consults it. Replace lines 89–98 of `server/services/service_isolation_test.go` with the simpler form that always fails on a cross-service import.
- [ ] Run `npm run build`, `go build -tags=integration ./...`, and `go test -tags=integration -timeout 20m ./...`. The relocated test should pass identically; `TestServiceIsolation` should pass with no allowlist.

**Validates:** `knownAllowedServiceImports` and its allowlist branch are gone; `TestServiceIsolation` is the durable rule; the draft → commit → override coverage that the moved test provides is preserved in its new home.

## Related Issues

- #1567 — community → streaming PR that prompted this cleanup; provides a recent example of the dilution being avoided.

## Audit Notes

Reviewed against `docs/server/architecture.md` (Service Independence §1, Shared Functionality in Libraries §2, Service Architecture, Testing Strategy §1–§2) and `docs/server/observability.md`. Findings:

- The recommended approach restores `docs/server/architecture.md` §1 ("services should not call or depend on other services") rather than deviating from it. The allowlist was always documented as a transitional accommodation.
- Streaming and fanout fit `docs/server/architecture.md` §2 "Pattern 1: Standalone Libraries for Shared Logic" cleanly — minimal public API, generic plumbing, used by 3+ services.
- `server/integration_tests/` is the documented home for cross-service end-to-end tests (`docs/server/architecture.md` §"Testing Strategy" → "Integration Tests"); relocating the experience↔impact_metrics test there fits the existing layout.
- No client, i18n, a11y, or caching surface area is touched, so those audit dimensions don't apply.
- No new logging, masking, or observability paths are introduced — only file moves and import rewrites.
