# Issue #2709: CI: self-hosted Go Tests runner flakes on testcontainer image pulls (`No such image`)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2709
**Filed:** 2026-07-12 | **Last activity:** 2026-07-12
**Labels:** bug | **Assignees:** unassigned
**Context docs:** `docs/infra_mimir.md` (⚓ anchor), `docs/testing/architecture.md` (⚓), `docs/dependency_automation.md`, `docs/server/architecture.md` (⚓), `docs/server/conventions.md` (⚓); memory `reference_mimir_ci_runners.md`. Source files read: `runners/Dockerfile`, `runners/docker-compose.yml`, `runners/sync.sh`, `runners/recreate_runners.sh`, `.github/workflows/build_runner_image.yaml`, `.github/workflows/test_go.yaml`, `server/storage/testing.go`, `server/storage/testing_test.go`.

> **Status:** Phase 1 **IMPLEMENTED** (workflow + docs only; no Go change). See
> [Implementation notes](#implementation-notes) at the bottom for the one design
> change made during implementation (tag-based warm instead of digest-pinning).

## Problem Statement

The **Go Tests** job (`self-hosted-linux`, runs on mimir) intermittently reds out
on testcontainer-backed tests. The diagnostic tell in the log is:

```
testing_test.go:137: could not create test container (image pull may have failed): Error response from daemon: No such image: alpine:3
```

The failing *tests* differ run-to-run and live in packages the PR under test
never touched — e.g. `TestEndToEnd_CompleteRequestLifecycle` (`server`) in one
run, `TestQueryGearByCommunitySearch_ThresholdFiltersIrrelevantResults`
(`server/storage`) in the next — while both are green locally, including under
CI's exact flags and stress counts. That signature (different unrelated tests,
all green locally, both carrying the image-pull line) is an **environment/runner
problem, not a code regression** — a code bug would fail the *same* test
deterministically.

**Two nuances the issue text blurs, established by reading the code, that the fix
must respect:**

1. **The `alpine:3` line is a co-occurring canary, not the failing test.** It
   comes from `TestCleanupOldTestContainers_RemovesOrphanWithVolume`
   (`server/storage/testing_test.go:129`), which calls the raw Docker API
   `ContainerCreate` with `Image: "alpine:3"`. `ContainerCreate` **never
   auto-pulls** — it errors `No such image` if the tag isn't already in the host
   daemon's image store. That test already degrades to `t.Skipf` on this error
   (`testing_test.go:137`), so it does **not** red the build. It's a legible
   signal that *the host daemon's image cache was cold / the daemon couldn't
   pull at that moment* — the same condition that then breaks the tests that
   genuinely depend on `pgvector/pgvector:pg16`.

2. **The RED failures are the `pgvector/pgvector:pg16` tests degrading under
   contention.** `setupSharedPostgreSQL` (`server/storage/testing.go:247`) starts
   one shared Postgres testcontainer per job via `postgres.Run(..., "pgvector/
   pgvector:pg16", ...)`. When the host daemon can't pull that image (cold cache,
   Docker Hub hiccup/rate-limit, or IO saturation), setup fails and the package's
   tests fail. Run 2's `"embedding storage failed" ... "record not found"` is the
   downstream symptom: under daemon/IO contention the async embedding backfill
   can't persist before the semantic-search assertion runs, so
   `TestQueryGearByCommunitySearch_...` asserts on missing embeddings.

**Root cause.** CI runs the whole tree in one shot under contention:
`go test -v -json -tags=integration -timeout 20m -coverprofile=coverage.out
-covermode=atomic ./...` (`test_go.yaml:133`). Packages run in parallel with
coverage instrumentation against a **shared self-hosted Docker daemon**. The
proximate, most-legible failure mode is **test image availability on that
daemon**; a deeper contributor is **IO/daemon saturation** under the parallel
`./...` + coverage load (consistent with the IO-deadlock class documented for
mimir in `reference_mimir_ci_runners.md`).

The container-*lifecycle* side is already hardened (per-job container names from
`GITHUB_RUN_ID/ATTEMPT/JOB`; ryuk disabled and cleanup skipped in CI; `1df1884eb`
honored the shutdown ctx in the embedding backfill goroutine; `45a7652ae` wrapped
the `record not found` sentinel). The remaining gap this issue targets is **image
availability on the host daemon**.

## Context from Comments

**The issue has no comments** — it was filed straight from the PR #2705 review
loop with a complete write-up (evidence table, local-repro results, root-cause
analysis, and a ranked fix list). Nothing here is contested; the "discussion"
lives in the issue body itself. No maintainer decision is pending, so this plan
does not need more input before proceeding — it needs a design choice among the
proposed fixes, which is what the Recommendation section makes.

## Critical correction to the issue's proposed fixes

The issue's **highest-ranked fix — "bake `docker pull …` into `runners/
Dockerfile`"** — **cannot work as written**, and the plan must not implement it
literally:

- **There is no Docker daemon during the image build.** `runners/Dockerfile` is
  built by buildkit (`docker build` in `build_runner_image.yaml`). A `RUN docker
  pull …` inside it has no daemon to talk to (that would require docker-in-docker).
- **Even if it could pull, it would cache into the wrong place.** Every runner
  mounts the **host** socket (`runners/docker-compose.yml:61`,
  `/var/run/docker.sock:/var/run/docker.sock`). Testcontainers and the raw
  `ContainerCreate` therefore create containers on **mimir's host daemon**, whose
  image store is `docker.img` (the loopback btrfs `data-root`). Layers baked into
  `ripls-runner:latest`'s filesystem are **invisible** to that host daemon.

So the pre-pull must land in the **host daemon's image store**, reached either
(a) per-job over the mounted socket from inside a runner (a `docker pull` in
`test_go.yaml` executes against the host daemon — exactly like the runner-image
build does), or (b) directly on the host via `recreate_runners.sh` (an Unraid
User Script running as host cron). Both are correct; the Dockerfile is not.

**Why a host-side pre-pull is durable:** none of mimir's maintenance jobs prune
*tagged* images. `recreate_runners.sh` ends in `docker image prune -f`
(dangling-only), `update_docker_stacks` does `docker image prune` (dangling-only),
and `cleanup_ripls_orphans.sh` sweeps stale containers, buildx builders, and
buildkit *build cache* — never tagged images (`docs/infra_mimir.md:160-165`). A
pinned, tagged `pgvector/pgvector:pg16` therefore survives on the host daemon
until its tag is deliberately re-pointed.

## Potential Solutions

### Option A: Per-job image warm-up step in `test_go.yaml` (RECOMMENDED)

- **Approach:** Add a step *before* `go test` that guarantees the test image is
  present in the **host daemon** via the mounted socket, **gated on a local
  presence check** so the registry is contacted only on a cold cache, pinned by
  digest, and re-tagged so the local tag exists:
  ```bash
  docker image inspect pgvector/pgvector:pg16 >/dev/null 2>&1 || {
    docker pull pgvector/pgvector@sha256:<PINNED>
    docker tag  pgvector/pgvector@sha256:<PINNED> pgvector/pgvector:pg16
  }
  ```
  with a bounded retry (e.g. 3× with backoff) around the `pull`. Two details are
  load-bearing:
  - **The local-presence guard is mandatory, not cosmetic.** `docker pull` — even
    of an already-resident digest — makes a Docker Hub *manifest* request every
    time. Run on every Go Tests job across the fleet from mimir's single egress
    IP, an ungated pull is constant anonymous-rate-limit pressure (Docker Hub
    anonymous ≈ 100 pulls/6h/IP, manifest checks included) and could re-introduce
    the same failure class as `toomanyrequests`. Gating on `docker image inspect
    <tag>` makes the steady-state warm-up a **local, zero-registry** no-op; the
    pull fires only when the image is genuinely absent.
  - **Digest-pull-then-retag** (not a bare `docker pull …@sha256:…`): a digest
    pull stores the image by digest but does **not** create the `:pg16` tag
    locally, so testcontainers — which requests the *tag* — would still round-trip
    the registry to resolve it. Re-tagging makes the local tag resolve with zero
    registry calls (testcontainers' default pull policy only pulls when the tag is
    absent locally; the raw `ContainerCreate` in the alpine test likewise needs
    the tag present). Concurrent Go Tests jobs share the one host daemon, so
    concurrent `pull`/`tag` of the same image are naturally deduped/idempotent.
- **Pros:** In-repo and reviewable; the pin lives in version control. **Self-healing
  per job** — a cold host cache is fixed by the first job and warm for the rest,
  independent of any host-cron state. Directly satisfies both acceptance criteria.
  No new infrastructure, no host credential.
- **Cons:** Adds one `docker pull` attempt per Go Tests job. In steady state
  (image already local) this is a fast manifest/"up to date" check; on a genuinely
  cold cache it pays a one-time ~300 MB pull, then warm. If Docker Hub is *down*
  and the image is *absent*, the step fails explicitly — a clear, retryable
  failure that is strictly better than a mid-suite flake.
- **Effort:** Small.

### Option B: Warm the host cache in `recreate_runners.sh` (host cron)

- **Approach:** Add the same digest-pull-then-retag to `recreate_runners.sh` so
  the daily recreate (and any short-interval idle pass) re-warms the host daemon
  for **all** jobs, not just Go Tests. To keep one source of truth, factor the
  pin+pull into `scripts/warm_test_images.sh` and extend `sync.sh` to ship it
  alongside the other User Scripts.
- **Pros:** Makes Option A's step a steady-state no-op; benefits any future job
  that needs these images; centralizes the pin.
- **Cons:** More moving parts on the host (another synced script; `recreate_
  runners.sh` currently has no repo dependency). Doesn't *replace* Option A —
  a prune or a fresh host still leaves a cold window until the next cron pass, so
  the per-job guarantee is still wanted. Best as an **enhancement to A**, not an
  alternative.
- **Effort:** Small–medium.

### Option C: Docker registry pull-through mirror on the host (issue's fix #2)

- **Approach:** Run a `registry:2` pull-through cache container on mimir and point
  the host daemon at it via `/etc/docker/daemon.json` `registry-mirrors`. Optionally
  authenticate it with a Docker Hub token to dodge anonymous rate limits.
- **Pros:** Serves *all* Docker Hub pulls locally; survives registry hiccups and
  rate limits fleet-wide, not just for these two images.
- **Cons:** Standing infrastructure to run, monitor, and secure on a host that
  already has reliability concerns (`daemon.json` change needs a daemon restart —
  disruptive to in-flight jobs). Once Option A/B keep the images resident, pulls
  become rare, so the marginal value is low. **Overkill for two pinned images.**
  Keep as a host-level follow-up only if pulls of *other* images start flaking.
- **Effort:** Medium (host infra + docs).

### Option D: Retry + reduce contention in test code (issue's fixes #3 + #4)

- **Approach:** (#3) bounded retry/backoff around testcontainer setup and the raw
  `ContainerCreate`; (#4) cap `go test -p` or split the Go job to lower parallel
  IO on the shared daemon.
- **Pros:** #4 attacks the *contention* root that pre-pull doesn't fully remove
  (the embedding-backfill race under IO saturation). Defensive.
- **Cons:** #3 is largely mooted by pre-pull — with the tag resident locally,
  testcontainers never calls the registry, so there's nothing to retry in the
  common path (and the alpine test already `t.Skipf`s). #4 trades wall-clock for
  stability and is a bigger change; premature before we see whether warming alone
  clears the flakes. **Follow-up, gated on residual flake rate.**
- **Effort:** #3 small; #4 medium.

## Recommendation

**Do Option A now** (per-job warm-up in `test_go.yaml`), with the **local-presence
guard** so it stays a zero-registry no-op in steady state. This is the smallest
change that (a) makes the pre-pull land on the *correct* daemon, (b) self-heals
every job regardless of host-cron state, and (c) directly satisfies both
acceptance criteria. It corrects the issue's Dockerfile-baking approach, which
would target the wrong daemon.

For the **`alpine:3` line**, the default is to **also warm `alpine:3`** in the
same step (it is ~7 MB, so warming it is nearly free) and **leave the cleanup
test as-is**. This satisfies acceptance criterion 1 (the image is resident, so
the raw `ContainerCreate` succeeds and the `No such image` line never appears)
*without* disturbing the test's semantics. Folding the test onto `pgvector`
instead — to get a single image to warm — is a viable alternative **but is not
free**: it introduces a real wrinkle (see Open Questions and the testing note
below) and is not required to satisfy acceptance.

**Add Option B** as a cheap durability enhancement **if** the per-job pull latency
proves noticeable in practice (it usually won't, since A keeps the host warm
job-to-job). **Hold Options C and D as follow-ups**, pursued only if flakes
persist after warming — at which point the residual cause is contention, and the
right levers are C (mirror) and/or D-#4 (cap `-p` / split job / move docker
`data-root` off the loopback btrfs, the structural IO fix from
`reference_mimir_ci_runners.md`).

**Tradeoff being made:** we prioritize the legible, high-leverage, in-repo fix
(image availability) that satisfies acceptance, and *defer* the deeper IO-contention
work rather than front-loading it — matching the issue author's own ranking
((1)+(3) stop the bleeding; (2)+(4) are host follow-ups). The risk we accept: if
the embedding-backfill assertion flakes are driven by IO saturation *independent*
of image pulls, Phase 1 will reduce but not fully eliminate them, and we escalate
to Phase 3. Phase 1's week-long acceptance window is exactly the measurement that
tells us which.

## Open Questions

- **Pull by digest-then-retag, or by tag?** — Recommendation: **digest-then-retag.**
  It gives reproducibility (content pinned by `@sha256:…`) *and* a resident local
  tag (so testcontainers and the raw `ContainerCreate` hit cache with zero
  registry calls). A bare tag pull is simpler but not reproducible; a bare digest
  pull leaves the tag unresolved locally and defeats the purpose. Record the
  digest next to the tag with a comment noting how to refresh it.
- **Consolidate the `alpine:3` cleanup test onto `pgvector/pgvector:pg16`, or
  just warm `alpine:3` too?** — Recommendation: **warm `alpine:3` too and leave
  the test alone** (the default above). Consolidation *looks* attractive (one
  image to warm, honors minimal-dependencies) but has a real wrinkle:
  `TestCleanupOldTestContainers_RemovesOrphanWithVolume` declares one anonymous
  volume via `Volumes: {"/data": {}}` and then asserts on the **first**
  `Type=="volume"` mount it finds (`testing_test.go:147`). `alpine:3` declares no
  `VOLUME`, so the container has exactly that one anonymous volume — clean.
  `pgvector/pgvector:pg16` *inherits* `VOLUME /var/lib/postgresql/data`, so the
  container would have **two** anonymous volumes and the assertion could latch
  onto the inherited pg-data volume instead of the declared `/data` one — the
  cleanup code still removes both (so the test wouldn't fail), but the test's
  *intent* becomes muddy (weaker "Assertion Quality"/"Test Readability" per the
  testing rubric). If consolidation is still preferred (e.g. to drop the second
  Docker Hub dependency), it **must** be paired with selecting the mount by
  `Destination == "/data"` rather than "first volume mount", and the comment
  updated to explain both the reuse and the precise selection. Warming the ~7 MB
  alpine image sidesteps all of this.
- **Where does the pin live if we later add Option B?** — Recommendation: a single
  `scripts/warm_test_images.sh` sourced by both `test_go.yaml` and (once
  `sync.sh` ships it) `recreate_runners.sh`. For Phase 1 (Option A only), inlining
  the two-line pull+tag directly in the workflow step is fine; extract to the
  shared script only when Phase 2 lands, to avoid premature indirection.
- **How is the pinned digest kept fresh so it doesn't rot?** — Recommendation:
  **register it with the existing Renovate docker manager** (or a `custom.regex`
  manager, the pattern already used for the `versions.env` mirrors — see
  `dependency_automation.md`) so the `pgvector/pgvector` (and `alpine`, if warmed)
  digest is bumped and PR'd like every other base-image pin, instead of becoming
  a hand-maintained pin that silently drifts from what `postgres.Run` resolves
  `:pg16` to at runtime. If a custom manager is disproportionate for one or two
  pins, the fallback is an explicit comment next to the pin stating it is
  manually refreshed and how (`docker buildx imagetools inspect pgvector/
  pgvector:pg16`), plus a note in `dependency_automation.md`'s held-back section.
  Either way the pin must be *tracked*, not orphaned.
- **Authenticate the cold-cache pull against Docker Hub to dodge anonymous
  rate-limits?** — Recommendation: **not initially.** With the local-presence
  guard the pull fires only on a genuinely cold cache, so anonymous limits are
  unlikely to bite. Revisit only if cold-pull logs show `toomanyrequests`; the
  cheap insurance is a `docker login` with a `DOCKERHUB_TOKEN` GitHub secret in
  the warm-up step — passed via `--password-stdin` (never on the command line, so
  the token can't leak into process args or job logs; secrets rubric §3).

## Implementation Plan

### Phase 1: Per-job image warm-up (stop the bleeding)
**Goal:** Guarantee both test images are resident on the host daemon before
`go test` runs — with the registry contacted only on a cold cache — satisfying
both acceptance criteria without disturbing test semantics.
**Tasks:**
- [ ] Pick and record pinned digests for `pgvector/pgvector:pg16` (and `alpine:3`
      if warming it) via `docker buildx imagetools inspect`, with a comment on how
      to refresh them.
- [ ] Register the pin(s) with Renovate (docker or `custom.regex` manager) so the
      digest is bumped like other base images — or, if that's disproportionate,
      add an explicit "manually refreshed" comment at the pin and a held-back note
      in `dependency_automation.md`. The pin must be tracked, not orphaned.
- [ ] Add a **"Warm testcontainer images"** step to `test_go.yaml` immediately
      before the `go test` step, **guarded on a local presence check**:
      `docker image inspect <tag> >/dev/null 2>&1 || { docker pull …@sha256:<PIN>;
      docker tag …@sha256:<PIN> <tag>; }`, with a bounded retry (3× w/ backoff)
      around the pull — for both `pgvector/pgvector:pg16` and `alpine:3`. Comment
      that it runs against the **host** daemon via the mounted socket, why the
      guard is mandatory (rate-limit), and why retag-after-digest-pull is needed.
- [ ] Leave `TestCleanupOldTestContainers_RemovesOrphanWithVolume`
      (`server/storage/testing_test.go`) on `alpine:3` (default). *If* the team
      opts to consolidate onto `pgvector/pgvector:pg16` instead, also change the
      mount selection to match `Destination == "/data"` (not "first volume") to
      account for pgvector's inherited `VOLUME`, and update the comment.
- [ ] Run the affected packages locally under CI flags to confirm no regression:
      `go test -tags=integration -covermode=atomic -count=3 ./server/... ` (at
      least `server` and `server/storage`), plus the two named tests with
      `-count` stress. Verify integration tests actually execute (creds via
      Secret Manager) — a self-skip is not a pass.
- [ ] Update `docs/infra_mimir.md` (runner-image / maintenance section) and/or
      `docs/testing/architecture.md` (testcontainers) with a one-line note that
      the host daemon's test-image cache is warmed per Go Tests job, and why the
      warm-up must target the host daemon (not the runner image).
**Validates:** No `No such image:` lines and no manual re-runs across a week of
Go Tests runs (both acceptance criteria) — measured by scanning Go Tests run logs
(`gh run list --workflow=test_go.yaml` → `gh run view --log`, grep `No such
image` / re-run counts). If assertion flakes persist *without* the image-pull
line, the residual cause is contention → Phase 3.

### Phase 2: Keep the host cache warm out-of-band (optional durability)
**Goal:** Make the Phase 1 step a steady-state no-op and warm the daemon for any
non-test job, only if per-job pull latency proves noticeable.
**Tasks:**
- [ ] Extract the pin + pull+tag into `scripts/warm_test_images.sh`; have
      `test_go.yaml` call it instead of the inline commands.
- [ ] Add a call to that script in `recreate_runners.sh` (both the full and
      `--idle-only` passes, or gated to once per pass to limit registry chatter).
- [ ] Extend `runners/sync.sh` to ship `scripts/warm_test_images.sh` to the host
      alongside the other User Scripts, so the host copy stays in sync.
**Validates:** Go Tests logs consistently show the warm-up step reporting "up to
date" (no cold pulls) in steady state.

### Phase 3: Contention follow-ups (only if flakes survive warming)
**Goal:** Attack the IO/daemon-saturation root if warming doesn't fully clear the
assertion flakes.
**Tasks:**
- [ ] Instrument/observe: after Phase 1, confirm whether residual failures still
      carry the image-pull line (→ availability, revisit) or not (→ contention).
- [ ] If contention: evaluate capping `go test -p` and/or splitting the Go job so
      fewer packages hit the shared daemon at once (issue fix #4).
- [ ] Evaluate a `registry:2` pull-through mirror on mimir (issue fix #2) if pulls
      of *other* images also start flaking.
- [ ] Escalate to the structural IO fix if warranted: move docker `data-root` off
      the loopback `docker.img` onto a native NVMe btrfs subvolume
      (`reference_mimir_ci_runners.md` — the leading structural suspect for
      mimir's D-state IO deadlocks). Invasive; file as its own issue.
**Validates:** Flake rate under sustained parallel `./...` + coverage load returns
to zero.

## Related Issues
- **#2705** — the PR whose review loop surfaced this flake; the evidence (two runs,
  two unrelated failing tests) comes from it.
- **#2657** (`project_2657_notification_flake`) — a prior CI flake root-caused to a
  real race, not the environment; useful contrast for confirming this one *is*
  environmental (different unrelated tests, all green locally).
- **#2287** — the runner-image "build on self-hosted → host daemon, no registry"
  pipeline this plan's warm-up step piggybacks on (same mounted-socket mechanism).
- **#2395** — runner-rebuild automation (`recreate_runners.sh` idle/daily passes);
  the seam Phase 2 would extend.
- **#1424** — Go file-size gate; unrelated but touches the same `test_go.yaml`.
- (Potential follow-up) a dedicated issue for moving docker `data-root` off the
  loopback btrfs backend, if Phase 3 escalates to the structural IO fix.

---

## Audit notes (`/ripls-audit`)

Audited against the infra/testing/dependency/security guidance selected from
`docs/llms.txt`. Changes applied to this plan:

1. **Perf + dependency (HIGH):** added a **mandatory local-presence guard**
   (`docker image inspect <tag> || pull`) to the warm-up. An ungated `docker
   pull` makes a Docker Hub manifest request on *every* Go Tests job; fleet-wide
   from mimir's single IP that is constant anonymous-rate-limit pressure and could
   re-introduce the failure as `toomanyrequests`. The guard makes the steady-state
   step a zero-registry no-op.
2. **Testing:** rebalanced the `alpine:3` handling. The original "consolidate onto
   pgvector" recommendation collides with the test's mount-selection logic
   (`testing_test.go:147` picks the first `Type=="volume"`) because pgvector
   inherits an extra `VOLUME`. New default: **warm `alpine:3` too and leave the
   test unchanged**; consolidation is offered only paired with a precise
   `Destination=="/data"` mount selection.
3. **Dependency:** added a task/open-question to **track the pinned digest via
   Renovate** (or an explicit manual-refresh note), per the `versions.env` +
   Renovate single-source ethos in `dependency_automation.md`, so the pin can't
   rot.
4. **Security:** specified `--password-stdin` for the deferred optional Docker Hub
   login (secrets rubric §3 — keep the token out of argv/logs).
5. **Observability:** named the concrete acceptance-measurement mechanism
   (`gh run` log scan for `No such image` / re-run counts).

Verified the plan's durability claim by reading the maintenance scripts: no
`image prune -a` / `rmi` / `system prune` anywhere; only `recreate_runners.sh:106`
`docker image prune -f` (dangling-only), so a pinned *tagged* image survives.

**Docs consulted:** `docs/infra_mimir.md` (⚓ anchor — host-daemon/socket
mechanism, build→recreate pipeline, maintenance-script schedule/prune behavior),
`docs/testing/architecture.md` (⚓ — testcontainers), `docs/dependency_automation.md`
(versions.env + Renovate single source, docker managers), `docs/reviews/
testing_review_prompt.md` (Areas to Review — flaky tests, assertion quality, test
isolation, CI integration), `docs/reviews/security_review_prompt.md` (§3 secrets
management), `docs/reviews/dependency_review_prompt.md` (pin tracking/update
priority); memory `reference_mimir_ci_runners.md`. Source verified:
`runners/{Dockerfile,docker-compose.yml,sync.sh,recreate_runners.sh,
cleanup_ripls_orphans.sh}`, `.github/workflows/{build_runner_image,test_go}.yaml`,
`server/storage/{testing.go,testing_test.go}`.

---

## Implementation notes

**Phase 1 shipped as workflow + docs only** — no Go change (the cleanup test
stays on `alpine:3`, which is now warmed).

- **`.github/workflows/test_go.yaml`** — added a "Warm testcontainer images (host
  daemon)" step before `go test`: a `warm()` bash helper that does `docker image
  inspect <ref> || docker pull <ref>` (bounded 3× retry) for both
  `pgvector/pgvector:pg16` and `alpine:3`, failing loudly if a pull truly can't
  complete. Runs against the host daemon via the mounted socket.
- **`docs/infra_mimir.md`** — one bullet under the mounted-socket section noting
  the per-run host-daemon warm-up and why baking into the runner image wouldn't
  work.

**Design change vs. the audited plan — dropped digest-pinning; warm by tag.**
The plan (post-audit) called for `docker pull …@sha256:<pin>` + retag, tracked by
a Renovate custom manager. Implementing it, that proved to be burden for marginal
gain: **testcontainers already resolves `pgvector/pgvector:pg16` by tag with no
pin** (`postgres.Run`, `server/storage/testing.go:276`), so the suite's
reproducibility posture is already tag-based — a warm-up pin doesn't close a gap
the tests leave open, but it *does* add a hardcoded `@sha256` that rots plus a
`renovate.json` custom-manager to keep it fresh. The **rate-limit concern that
motivated the audit's guard is fully handled by the presence check alone**
(`docker image inspect` short-circuits before any registry call). Warming by tag
is therefore simpler, can't rot, needs no Renovate change, and satisfies both
acceptance criteria. This dissolves audit finding C (nothing to track) and the
"pin freshness" open question. If a supply-chain pin is ever wanted, it should be
applied at the `postgres.Run` call site (so testcontainers itself is pinned), not
only in the warm-up — filed as a possible follow-up, not done here.

**Verification (local):** `actionlint` clean on `test_go.yaml`; `bash -n` +
`shellcheck` clean on the warm-up snippet. The step itself can only be exercised
on the self-hosted runner (needs the host Docker socket), so it validates on the
PR's Go Tests run; the images-work path was already proven green locally in the
issue's repro. No Go code changed, so no package re-run was needed.
