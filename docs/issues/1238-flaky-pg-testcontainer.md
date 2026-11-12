# Issue #1238: Shared pg testcontainer dies mid-run; masks failures with 'connection refused' cascade

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1238
**Status:** RESOLVED in PR #1243 (2026-04-23)
**Filed:** 2026-04-23 | **Last activity:** 2026-04-23
**Labels:** bug, P1 | **Assignees:** maintainer
**Context docs:** none

## Resolution Summary (2026-04-23)

Root cause was **not** what the issue originally described (container
dying mid-run from OOM/daemon/reaper). The actual culprit:
`cleanupOldTestContainers` in `server/storage/testing.go` was
force-removing pg testcontainers belonging to *concurrently running CI
jobs*, because its liveness probe (container-name PPID → `processExists`)
doesn't work across PID namespaces on the self-hosted runner. Each CI
job runs in its own container sharing the host Docker daemon; from job
B's namespace, job A's `go test` process is invisible → probe reports
"dead" → cleanup SIGKILLs job A's container. Every new concurrent run
kills the others' pg containers within ~2s of start; Postgres never
finishes initdb; testcontainers retries; each retry gets SIGKILLed
again. The "matched 0 times, expected 2" readiness-wait error was the
downstream symptom.

**Fix (shipped):** Gate `cleanupOldTestContainers` to local-dev only
(`os.Getenv("CI") == ""`). CI relies on runner job isolation for
cleanup. Ryuk stays disabled in CI to avoid the original race it was
meant to prevent.

**Verification matrix on `flaky_test_container`:**
- Cleanup enabled, even with PPID-alive guard: **0/4 concurrent runs
  passed** — SIGKILL cascade reproduces deterministically.
- Cleanup fully disabled: **1/4 passed**, 3/4 hit unrelated 20m LLM-hang
  timeouts tracked in #1237.
- Cleanup CI-gated (final fix): expected to match the disabled case.

**Also landed in the fix PR:** rich `SETUP DIAGNOSTIC` and
`CONTAINER DIAGNOSTIC` dumps (pg container logs + Docker events since
run start) so the *next* container-level flake is self-diagnosing
without re-running. The evidence that pinned the root cause above was
captured by these dumps.

The rest of this document is the original pre-resolution plan, kept for
audit trail. Phase 1 (richer diagnostics) shipped as part of the fix
PR; Phases 2 and 3 are moot now that the root cause is known and
addressed.

---

## Problem Statement

The Go integration test suite shares a single PostgreSQL testcontainer
(`ripls-test-postgres-<PPID>`) across all packages via
`testcontainers.WithReuseByName`. During long CI runs the container becomes
unreachable mid-suite. Every subsequent test then fails with
`dial tcp 172.17.0.1:<port>: connect: connection refused` — misleading errors
that bury whatever real failure (if any) came first. Result: flaky, noisy CI
that obscures both the trigger and the root cause.

Two distinct failure modes are visible in the evidence:

1. **Container gone** — `docker inspect` reports "No such container"
   (e.g. runs `24796041104`, `24796691610`). Something removed the container
   entirely.
2. **Container running but Postgres dead** — `docker inspect` reports
   `status=running running=true` but TCP connections are refused
   (e.g. run `24679052613`). The process/listener inside the container died
   without the container exiting.

Both symptoms currently produce the same generic `connection refused` cascade
from every downstream test. The existing diagnostics (`logContainerDiagnostics`
in `server/storage/testing.go:124-159`) fire once per test run but stop at
`inspect failed` — they don't capture Postgres stderr, Docker events, or OOM
kills that happened moments before.

Note: ryuk is already disabled (`server/storage/testing.go:45`), and previous
runs' containers are cleaned up once at startup via
`cleanupOldTestContainers`. So the container outliving the test process isn't
the issue — it's dying *during* the run.

## Context from Comments

- **maintainer** (issue author, maintainer): noted they'd added the
  existing diagnostic logging in PR #1150 and wondered whether those logs
  contained anything more. PR #1150 did add the `logContainerDiagnostics`
  function present today; it captures container state but can't see inside a
  container that's already gone.

Only one comment so far. The issue is fresh (filed today) and the author has
already ranked proposed fixes — (1) post-mortem diagnostics, (2) stop sharing,
(3) split CI jobs, (4) tune reaper/shm. Their recommendation: do (1)
immediately, then pick (2) or (3) based on evidence.

Branch `flaky_test_container` already exists, suggesting work is imminent.

## Potential Solutions

### Option A: Diagnostics-first (maintainer's recommendation)

- **Approach:** Extend `logContainerDiagnostics` so that when the container is
  gone or unreachable, it captures whatever forensic data is still available:
  `docker logs <name>` (pg stderr, crash output, `oom-killed` messages),
  `docker events --since <run start>` (kill/die/destroy events, exit codes),
  `dmesg | grep -i oom` on the runner host if accessible, and the
  `docker inspect` payload of the last known state. Dump all of this to the
  test log on first failure so the next CI flake produces actionable data.
  Leave the shared-container setup alone for now.
- **Pros:** Small, low-risk change. Doesn't trade the existing performance
  amortization for speculative isolation. Gives us the evidence to pick the
  right structural fix instead of guessing.
- **Cons:** Doesn't actually fix the flake — only makes the next one
  diagnosable. If CI is badly broken right now, we'd still be red while
  waiting for another failure to analyze.
- **Effort:** Small.

### Option B: Stop sharing the container across packages

- **Approach:** Drop `WithReuseByName` in `server/storage/testing.go:102`. Each
  Go test package spins up its own Postgres container. A crash in one
  package's container no longer cascades to others; failures stay localized
  to the offending suite.
- **Pros:** Structurally eliminates the cascade. Each package's failure is
  self-contained and identifiable. Simplifies `testing.go` (no more
  shared-state `sync.Once`, no PPID trickery, no orphan cleanup).
- **Cons:** Startup cost multiplies. `go test ./...` currently amortizes one
  container across ~30+ packages; per-package containers means ~30+ pulls of
  the pgvector image (cached after first), ~30+ container starts. On the
  self-hosted runner this likely adds several minutes and pressures Docker
  daemon / disk / memory harder, which may *increase* the odds of the
  daemon-level issues we're trying to escape. Also risks port exhaustion
  under parallel `go test`.
  **Architectural note:** `docs/server/architecture.md` §Testing Strategy
  explicitly codifies the current model — *"Each test creates a unique
  database within a shared container"*. Option B rescinds that decision,
  so if we pick it, the architecture doc must be updated in the same PR
  to avoid leaving a contradicting reference.
- **Effort:** Medium. The code change is small, but validating CI impact
  (wall-clock time, runner resource headroom) requires careful measurement.

### Option C: Split CI into a heavy-integration job and a unit-test job

- **Approach:** Keep the shared container for lightweight packages but move
  heavy suites (`server/integration_tests`, `server/ai`) into their own CI
  job (or their own container via a build-tag split). The fragile high-load
  work is isolated; unit tests stay fast and green.
- **Pros:** Targets the actual risk — most of the DB churn comes from a
  handful of suites. Keeps the amortization win for the long tail. Can run
  the two jobs in parallel on CI for wall-clock neutrality.
- **Cons:** More CI plumbing. Self-hosted runner capacity must support two
  concurrent jobs. Doesn't directly answer *why* the container dies — if the
  root cause is a Docker daemon issue on the unraid runner, both jobs can
  still fail in tandem.
- **Effort:** Medium.

### Option D: Harden the container (shm/memory/restart)

- **Approach:** If diagnostics confirm OOM, bump `testcontainers.WithShmSize`
  and/or pass `--memory` / `--restart=on-failure` to the Postgres container.
  Potentially pin a different Postgres image if pgvector/pg16 is implicated.
- **Pros:** Cheap, minimally invasive, directly addresses the OOM hypothesis.
- **Cons:** Only valid if (A) identifies OOM as the cause. A no-op against
  Docker daemon issues or ryuk/reaper races.
- **Effort:** Small.

## Recommendation

**Do Option A first, then reassess.** The issue author ranked it #1 for good
reason: we have two failure modes with overlapping symptoms, and we currently
can't tell OOM from reaper kill from daemon hiccup from in-container
segfault. Picking B/C/D now is a guess, and Option B in particular could
easily make things worse on a resource-constrained self-hosted runner.

Concretely:

- Land Option A in a small PR on the existing `flaky_test_container` branch.
- Let CI run for a few days and catch another flake.
- Read the new diagnostics, then pick:
  - **OOM in pg logs** → Option D (shm/memory tuning) first, then revisit
    isolation if it persists.
  - **Docker daemon errors / `container died` from `docker events` with no
    OOM** → Option C (isolate heavy suites) to reduce daemon load, and
    consider moving CI off the unraid runner for integration tests.
  - **In-container pg crash (segfault, FATAL, assertion)** → investigate the
    specific workload; Option B (no sharing) also becomes more attractive
    because it keeps one package's pg crash from poisoning the next.
  - **Nothing conclusive** → Option B for structural isolation, accepting
    the startup cost.

This sequencing respects the maintainer's stated preference, costs little,
and avoids thrashing the CI pipeline on speculation.

## Open Questions

- **Does the runner have persistent access to `docker` / `dmesg` from inside
  the test process?** — Recommendation: assume yes for `docker logs` and
  `docker events` (they run via the Docker socket the test already uses), and
  best-effort for `dmesg` (skip silently if we lack permissions). Don't rely
  on host-level data as the only evidence.
- **Should the post-mortem dump happen on first failure only, or every
  failure?** — Recommendation: keep the existing once-per-run
  `sync.Once` gating so logs don't explode to hundreds of duplicate dumps
  when all downstream tests fail, but *do* include `docker logs --tail 200`
  and the last few minutes of `docker events` so the first dump is rich
  enough to be conclusive.
- **Do we want to fail-fast the whole run when the container dies, instead
  of letting every package produce a misleading failure?** —
  Recommendation: yes, eventually. Once diagnostics are in, make
  `setupSharedPostgreSQL` / `createTestDatabase` return a distinctive
  `t.Fatal("shared Postgres container is dead — see DIAGNOSTIC log above")`
  and short-circuit subsequent tests in the same process. But defer this
  until Option A has produced at least one good forensic capture, so we
  don't mask the cascade we're actively trying to read.
- **Should we switch off the self-hosted `unraid-runner-3` for integration
  tests entirely?** — Recommendation: keep for now; it's the cheapest
  option and the evidence doesn't yet exonerate our code. Revisit if Option
  A shows recurring Docker daemon faults on that host specifically.
- **Is `ryuk` disablement contributing by leaving stale containers around
  long enough to collide across runs?** — Recommendation: unlikely, because
  `cleanupOldTestContainers` removes every `ripls-test-postgres-*` that
  doesn't match the current PPID. But worth confirming with `docker ps -a`
  on the runner after a flake.

## Implementation Plan

### Phase 1: Richer post-mortem diagnostics

**Goal:** Make the next CI flake diagnosable with a single log read.
**Tasks:**
- [ ] Extend `logContainerDiagnostics` in `server/storage/testing.go` so that
      when `ContainerInspect` fails (container gone) it still attempts
      `ContainerList --all --filter name=ripls-test-postgres-` to find
      remnants, and dumps whatever it finds (exit code, finished-at,
      error string).
- [ ] Add a `docker logs --tail 200` equivalent via the testcontainers /
      Docker SDK client (`ContainerLogs`) for the shared container name,
      best-effort — skip silently on error. Log both stdout and stderr
      (Postgres writes fatal errors to stderr).
- [ ] Add a `docker events` equivalent since the recorded run start:
      stash a `runStartTime` when the shared container is first created,
      query `Events(ctx, types.EventsOptions{Since: runStartTime})` on
      failure, filter by container name, log last N events.
- [ ] Also fire the diagnostic block when `inspect` succeeds but the state
      is not `running` (`Paused`, `Exited`, `Restarting`, `Dead`) — today
      we only log state; we should also dump logs in those cases.
- [ ] Unit test the diagnostic paths: spawn a throwaway container, kill it
      via the Docker client, assert the diagnostic function captures logs
      and events without panicking. (Aligns with the "unit tests are always
      preferable" principle in `docs/server/architecture.md` §Testing.)
- [ ] Keep diagnostic output on `t.Logf` (test-scoped), not `slog`. The
      existing `logContainerDiagnostics` already does this; don't swap it
      for the structured logger since these logs belong to a specific test's
      output stream, not the server's production logs.
- [ ] Land on branch `flaky_test_container`.

**Validates:** The next recurrence of this flake produces a single CI log
with enough detail to classify the failure as OOM / daemon / in-process
crash / something else, without needing to SSH to the runner.

### Phase 2: Classify and target-fix

**Goal:** Apply the right structural fix based on Phase 1 evidence.
**Tasks:**
- [ ] Wait for at least one captured flake post-Phase-1 (or manually
      reproduce locally by `kill`-ing the container during a long test run
      to verify the diagnostic path).
- [ ] Decide among Options B / C / D from the analysis above based on what
      the diagnostics show.
- [ ] Add a short postmortem note to `docs/ai/` (or under this plan) with
      the finding and rationale for the chosen fix.
- [ ] Implement the chosen fix in a follow-up PR.

**Validates:** The structural fix addresses the actual failure mode
instead of a guess. One CI week green with no `connection refused`
cascade.

### Phase 3: Fail-fast on dead container

**Goal:** Stop a dead container from producing N misleading test failures.
**Tasks:**
- [ ] In `SetupTestDatabase` / `SetupPatrolTestDatabase`, detect the "shared
      container dead" condition (connection refused or container-gone
      inspect result) and mark a process-wide flag using `sync/atomic`
      (e.g. `atomic.Bool`) — not a plain bool behind a mutex — so the
      fast path in subsequent test setups is lock-free.
- [ ] When that flag is set, subsequent `SetupTestDatabase` calls should
      fast-fail with `t.Fatalf` referencing the earlier diagnostic block
      instead of attempting a doomed connection.
- [ ] Keep the diagnostic `sync.Once` gating so logs stay readable.
- [ ] Add a unit test that simulates container death (point
      `sharedBaseConnString` at a dead port) and asserts subsequent
      `SetupTestDatabase` calls fail fast with the expected message.

**Validates:** A single CI log shows one real failure + one short
"container dead" notice per remaining test, not 30+ misleading
`connection refused` stack traces.

## Related Issues

- #1150 — PR that introduced the original `logContainerDiagnostics`
  function. Phase 1 extends the work started there.
- Companion LLM-hang issue referenced in the evidence (`24764389346` etc.) —
  may compound the pg container flake by starving the runner, but is a
  separate fix. Track separately; don't couple scope.

## Audit Notes (ripls-audit, 2026-04-23)

Reviewed against `docs/server/architecture.md` §Testing Strategy and
CLAUDE.md conventions. Edits applied:

- **Option B architectural conflict flagged.** The architecture doc
  explicitly codifies "unique database within a shared container" as the
  test model, so picking Option B requires updating that doc in the same
  PR. Noted in Option B's Cons.
- **Phase 1 logging convention clarified.** Added an explicit task to
  keep diagnostic output on `t.Logf` (test-scoped) rather than migrating
  to `slog`. The existing code already does this correctly; the note
  prevents drift during the Phase 1 rewrite, since CLAUDE.md's "always
  use structured logging" rule could be misread as applying to test
  diagnostics.
- **Phase 1 unit-test task linked to architecture principle.** The
  diagnostic code is a shared library concern ("every shared library
  function should have unit tests"), and unit tests are the preferred
  test level per the architecture doc — made the alignment explicit.
- **Phase 3 concurrency primitive specified.** Fast-fail flag should use
  `sync/atomic` (e.g. `atomic.Bool`) for a lock-free hot path, not a
  mutex-guarded bool. Clarified in the Phase 3 task list.

No changes needed to: Option A (matches architecture principles cleanly),
Option C (CI orchestration only, no architectural impact), Option D
(cosmetic container tuning, no architectural impact), or the
recommendation sequencing. Boundary check — plan touches only
`server/storage/testing.go` and CI configuration; no client-side,
proto, or service-layer concerns, so client architecture/caching/i18n
docs were not load-bearing for this audit.
