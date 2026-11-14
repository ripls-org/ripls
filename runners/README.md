# GitHub Actions self-hosted runners

Canonical source of the Docker config for the five `self-hosted-linux`
runners that run CI for this repo. Lives on `mimir` (Ubuntu 26.04) under
`/opt/runners/`. This directory is the truth — `mimir` gets a copy via
`./sync.sh`.

The runners register at the org level (`RUNNER_SCOPE: org`), so the private
agent repo `ripls-agent` (#2970) shares the fleet with `ripls`. They sit in the
org's Default runner group, which excludes public repos, so public repos can't
schedule jobs on them.

The compose project name must equal that directory's name, so it is `runners`.
`docker compose -p` matches on the directory name, and a mismatched `-p` makes
`down` a silent no-op against a fleet that keeps running.

## Files

- `Dockerfile` — runner image (Go, Node, Flutter, JDK, Android SDK, ONNX,
  Ruby) on top of `myoung34/github-runner:ubuntu-noble`. Tool versions are NOT
  pinned here — they are bare `ARG`s supplied as build-args from the repo-root
  `versions.env` (the single source of truth; see #2287). Both CI builds go
  through `scripts/build_runner_image.sh <tag>`, which derives the build-args
  from those `ARG`s, so a new baked tool needs no workflow edit (#2883).
- `docker-compose.yml` — five runner services sharing one locally-built
  image (`ripls-runner:latest`); the `build:` block's args are interpolated
  from `versions.env`. See [Image rebuilds](#image-rebuilds). Per-runner state
  (`work/`, `gradle/`) lives under `/var/cache/nas/github-runner/runner-N/`;
  caches that are safe to share
  (`go-mod`, `go-build`, `pub-cache`, `npm`, `gradle-files`) live under
  `/var/cache/nas/github-runner/shared/`. The per-runner `~/.gradle` mount
  must come before the deeper shared `gradle/caches/modules-2/files-2.1` mount
  or Docker shadows it.
- `clean-entrypoint.sh` — wraps the upstream entrypoint to also delete
  `.credentials*` on start (otherwise in-place runner self-update bricks the
  container on restart).
- `.env.example` / `.gitignore` — `.env` holds `ACCESS_TOKEN` and is host-only.
  The runners register at the org level (`RUNNER_SCOPE: org`), so the PAT
  needs `admin:org`.
- `sync.sh` — `rsync` push to `/opt/runners` on mimir: the compose project
  files, the host-side scripts, and `versions.env`, then runs
  `seed_agent_workspaces.sh`. `--rebuild` is the local-rebuild fallback
  (sources `versions.env`, then `docker compose build` +
  `up -d --force-recreate`).
- `seed_agent_workspaces.sh` — gives each runner in `docker-compose.yml` a full
  ripls clone in its `ripls-agent/ripls-agent` workspace, where agent jobs
  (#2970) keep ripls at full history. Without one, a runner's first agent job
  clones all ~800 MB from GitHub, about an hour over mimir's uplink. It clones
  from the bare seed at `/var/cache/nas/github-runner/seed/ripls.git` and
  refreshes the seed's `main` from the runners' existing agent workspaces, so
  it needs no GitHub credential. It never touches an existing workspace, and
  work dirs are bind mounts, so this only does anything for a new runner or a
  wiped work dir. `recreate_runners.sh` runs it before each pass and `sync.sh`
  after each push.
- `cleanup_ripls_orphans.sh` — hourly host-side sweep of leaked CI cruft: stale
  `ripls-test-postgres-*` and abandoned `buildx_buildkit_*` containers, the
  dangling `buildx_buildkit_*_state` volumes those builders leave behind, and
  buildkit build cache above a 30 GB cap (`--max-used-space`). The volume +
  cache sweeps bound `/var/lib/docker` growth on the `fast` mirror. This file
  is the source of truth; `sync.sh` deploys it and the
  `ripls-cleanup-orphans` timer runs it.
- `recreate_runners.sh` — scheduled host-side recreate of the fleet onto the
  current `ripls-runner:latest` (see [Image rebuilds](#image-rebuilds)). Source
  of truth; `sync.sh` deploys it. Two modes: no-arg rolls the whole fleet
  (daily backstop, `ripls-runner-recreate.timer` at 04:40); `--idle-only` rolls
  just the stale + idle runners one at a time (short-interval prompt pass,
  #2395, `ripls-runner-recreate-idle.timer`).

  Both timers live in the **mimir repo** under `deploy/systemd/` and pass the
  flag directly. Before the Ubuntu migration this needed a separate
  `recreate_runners_idle.sh` wrapper, because the Unraid User Scripts plugin
  invoked every script as `<name>/script` with no arguments and so could not
  give the idle pass its own schedule. systemd can, so the wrapper is gone.

> **Check both passes are actually registered.** The idle entry was missing for
> a long time and nothing surfaced it: the daily backstop kept the fleet correct
> eventually, so the only symptom was latency. A baked-tool bump merged at 20:52
> on 2026-08-28 did not reach the runners until the 04:40 pass the next morning.
> `grep 'idle pass' /var/log/ripls-runner-recreate.log | tail` tells you whether
> it has run recently; if the newest entry is old, the schedule is not live.

## Sync

```bash
./sync.sh             # push files only
./sync.sh --rebuild   # push + docker compose build + up -d --force-recreate
```

`.env` is never overwritten on the host.

## Image rebuilds

The image is built **on mimir into mimir's local Docker store — no registry, no
host credential**. Build and recreate are split:

1. A version bump lands in `versions.env` (typically a Renovate PR labeled
   `needs runner rebuild`).
2. **Validate (CI, pre-merge):** `.github/workflows/validate_runner_candidate.yaml`
   builds a per-PR `ripls-runner:pr-<N>` from the PR's `versions.env` and runs
   `scripts/runner_candidate_checks.sh` inside it (Flutter test, Android
   release-build, Go compile) — proving the **new** toolchain before merge, since
   the PR's own CI runs on the **old** image. Reported as the required check
   `Validate Runner Candidate`; on green a patch/minor Renovate bump auto-merges
   (majors after human review). See `docs/infra/dependency_automation.md` §4–5.
3. **Build (CI):** `.github/workflows/build_runner_image.yaml` runs on a
   `self-hosted-linux` runner on merge to `main`. Because runner containers mount
   the host Docker socket, its `docker build` (via `scripts/build_runner_image.sh`,
   build-args from `versions.env`) tags `ripls-runner:latest` directly in
   **mimir's local daemon**. It uses GitHub Actions' built-in auth — no deploy
   key / PAT / App on the host — and only builds (no recreate), so it never
   kills its own job.
4. **Recreate (systemd timer):** `recreate_runners.sh`, run by the
   `ripls-runner-recreate{,-idle}` timers, needs no git/build/credentials and
   runs on the host (not inside a runner) to avoid the self-recreate hazard.
   The short-interval `--idle-only` pass rolls each stale + idle runner
   (skipping any with a live `Runner.Worker`, so in-flight jobs and the build
   job's own runner are untouched) within minutes of merge; the daily no-arg
   pass (`docker compose -p runners up -d --no-build`) rolls the whole fleet as
   a backstop for runners that were busy when the prompt pass ran.

**Rollback:** revert the change on `main`; the next CI build rebuilds the prior
image, and the following recreate rolls the fleet back.

**Caveats:**
- The daily no-arg full pass can recreate a runner mid-job (image changed),
  killing that job; low-risk at the scheduled hour, and a killed job reruns. The
  `--idle-only` prompt pass avoids this — it skips any runner with a live
  `Runner.Worker` — so the short-interval pass is safe to run often.
- The host's generic `update-docker-stacks` timer is pull-based, so it logs a
  benign failed-pull for this stack (`ripls-runner:latest` has no registry);
  `recreate_runners.sh` is what manages the runner stack.

**One-time host setup:** `./sync.sh` pushes the compose project and the
host-side scripts into `/opt/runners`; the schedules are systemd timers owned by the
mimir repo (`deploy/systemd/ripls-*.timer`), enabled once from there. No repo
clone or credentials needed on the host. To go live the first time:
`./sync.sh` (delivers the compose referencing `ripls-runner:latest`), ensure
`build_runner_image` has run once (`workflow_dispatch`) so the image exists,
then let the recreate — or `./sync.sh --rebuild` — roll the fleet onto it.
Tracked in #2287/#2395.

## CPU budget

mimir is one host with 20 logical CPUs (8 P-cores with SMT plus 4 E-cores)
that also runs the prod and dev servers, the websites and other services. Each runner
gets two settings in `docker-compose.yml`:

- `cpu_shares: 128` — cgroup weight 21 against the default 100. Under
  contention the whole fleet weighs about as much as one ordinary container,
  so the other services get the CPU they ask for. It costs CI nothing when
  the host is otherwise idle.
- `cpus: 10` — a ceiling. Go (`GOMAXPROCS`, hence `go test -p`), the JVM
  (Gradle workers) and Node read the limit and size themselves to it. **Dart
  does not**: `Platform.numberOfProcessors` reports 20 under both a quota and
  a cpuset, so `test_flutter.yaml` passes `--concurrency` from Node's
  `os.availableParallelism()`.

Containers a job starts through the mounted Docker socket are created on the
host daemon, **outside** the runner's cgroup, so neither setting reaches them.
The two heavy cases carry the budget explicitly: the BuildKit builder
(`driver-opts: cpu-quota` in `test_docker_build.yaml` and
`build_and_deploy_container.yaml`) and the candidate-image container
(`docker run --cpus` in `validate_runner_candidate.yaml`). Testcontainers
Postgres is left alone; it is light.

**Why not a strict partition (20 CPUs / 5 runners = 4 each).** Measured on
mimir, 2026-11-12, simulating one PR's heavy jobs landing together (Flutter
tests, Go integration tests, Android release build), seconds:

| per-job `cpus` | Flutter | Android | Go  | all done | job-seconds |
| -------------- | ------- | ------- | --- | -------- | ----------- |
| uncapped       | 215     | 319     | 336 | 336      | 870         |
| 4              | 176     | 290     | 474 | 475      | 940         |
| 6              | 171     | 277     | 426 | 427      | 874         |
| 8              | 184     | 278     | 370 | 371      | 832         |
| 10             | 191     | 363     | 330 | 364      | 884         |
| 12             | 210     | 299     | 349 | 350      | 858         |

Single runs, so treat ±10% as noise (the 10 row also ran `go test -p 20`;
repeating 8 with `-p 16` and `-p 20` changed nothing measurable). What survives the noise: total work is
flat from 6 up to uncapped, a cap moves time from Flutter and Android onto Go
(the last job to finish), and 4 is plainly worse. The Flutter suite alone
takes 96s uncapped, 93s at 8, 99s at 6 and 120s at 4. A high load average
during a burst is jobs queueing for cores, not lost work; a tight cap lowers
the load number by making the PR slower. About half of busy time has only one
or two heavy jobs running, and a strict partition would idle most of the host
then.

Go tests are the long pole and CPU-bound, so the real lever for PR turnaround
is the Go suite itself (its slowest packages run 200s+), not the budget.

## Forgejo runners

`forgejo/` is a second fleet, for the Forgejo instance the project is moving
to (#3293). Until the cut-over it runs CI in shadow next to the GitHub fleet
above; after it, this is the only fleet.

It works differently. Each service in `forgejo/docker-compose.yaml` is a small
daemon that polls Forgejo and starts a **fresh container from
`ripls-runner:latest` for every job**, through the host's Docker socket. So
the toolchain image is shared with the GitHub fleet unchanged, a rebuilt image
is used by the next job with no recreate pass, and nothing from one job's
workspace reaches the next.

- `forgejo/docker-compose.yaml` — the daemons. The compose project is
  `forgejo-runners`, deployed to `/opt/forgejo-runners`.
- `forgejo/config.template.yaml` — one daemon's config, including everything a
  job container gets: the CPU budget below, `/dev/kvm`, the cache mounts and
  the testcontainers override. A workflow can't mount anything else.
- `forgejo/setup.sh` — host side: renders a config per runner, registers the
  ones that aren't yet, starts the fleet. It also holds the label map.
  `ubuntu-latest` is one of the labels because Forgejo has no hosted runners.
- `forgejo/sync.sh` — run from a checkout: pushes the three files above and
  runs `setup.sh`. To register a new runner it fetches a one-time token from
  the Forgejo API (`FORGEJO_URL`, `FORGEJO_TOKEN` with `write:organization`)
  and pipes it to the host; the token is never stored.

Runners register at the organization, so every repo in it shares them. Their
state is under `/var/cache/nas/forgejo-runner/runner-N/`: `data/` holds the
registration and rendered config, `gradle/` that runner's Gradle home. The
content-addressed caches under `github-runner/shared/` are shared by both
fleets.

To add a runner, add a service to the compose file and run `./sync.sh`.

### The Mac that builds iOS

`forgejo/mac/` registers the build Mac with the same organization, under the
label `self-hosted-mac`. It is a different shape from the Linux fleet:

- **Host mode, no container.** Jobs run straight on the machine as the
  logged-in user, because Xcode and code signing need the real macOS session.
- **A LaunchAgent in the GUI session** (`org.ripls.forgejo-runner`), not a
  daemon, for the same reason: only that session reaches the login keychain.
- **One job at a time.** `xcodebuild` and `fastlane match` are not safe to run
  twice at once in one account. Until the cut-over the GitHub runner lives in
  the same account, so do not send both forges an iOS build at the same time.

The files:

- `forgejo/mac/config.template.yaml` — the daemon's config.
- `forgejo/mac/org.ripls.forgejo-runner.plist` — the LaunchAgent.
- `forgejo/mac/setup.sh` — Mac side: renders both, registers if needed, loads
  the agent. It holds the toolchain `PATH`, used by the daemon and by every job
  step. The daemon needs `node` on it to run JavaScript actions; the GitHub
  runner ships its own Node, this one does not.
- `forgejo/mac/sync.sh` — run from a checkout with `MAC_HOST=user@host`: pushes
  the three files and runs `setup.sh`, fetching a registration token the same
  way as `forgejo/sync.sh`.

The runner binary comes from Homebrew (`brew install forgejo-runner`); keep it
at the version `forgejo/docker-compose.yaml` pins. State is in
`~/forgejo-runner/` on the Mac: the registration, the rendered config, and
`work/`, which holds one directory per running job plus a persistent
`tool_cache/` (the Flutter SDKs). Logs are in `~/Library/Logs/forgejo-runner/`.

```bash
ssh "$MAC_HOST" 'launchctl print gui/$(id -u)/org.ripls.forgejo-runner | grep state'
ssh "$MAC_HOST" 'tail -20 ~/Library/Logs/forgejo-runner/stderr.log'
```

## Go version

`GO_VERSION` (from `versions.env`) is a **bootstrap**. With `GOTOOLCHAIN=auto`
(default in Go 1.21+) the bootstrap reads `go.mod` and downloads the requested
toolchain into the persistent module cache (`/root/go/pkg/mod`). Bumping
`go.mod`'s `go` directive doesn't require an image rebuild — the runners
auto-pull. Bump `GO_VERSION` in `versions.env` only to skip the one-time
per-version download (`check_versions.js` keeps it in step with `go.mod`).
