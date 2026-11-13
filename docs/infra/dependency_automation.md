---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: As-built reference for dependency & toolchain automation — self-hosted Renovate on mimir, versions.env as the single tool-version source, the auto-merge/auto-approve machinery, the runner and Renovate images, and the deliberately held-back dependencies and what unblocks each.
  globs: [renovate.json, renovate/**, runners/**, versions.env, scripts/check_versions.js, scripts/doctor_versions.js, .github/workflows/**, go.mod, app/pubspec.yaml, package.json, Dockerfile]
  triggers: [renovate, dependabot, auto-merge, versions-env, runner-image, required-checks, toolchain, mimir, dependency, blocked-deps, upgrade]
  lens: [infra, dependency]
  skills: [issue, audit]
  domain: infra
freshness:
  verified_commit: "71aae247c"
  verified_on: "2026-11-11"
---
# Dependency & Toolchain Automation

The durable reference for how dependency and toolchain updates flow through this
repo: the automation engine, the single source of truth for tool versions, the
auto-merge machinery, the self-hosted runner and Renovate images, and — crucially
— the set of dependencies that are deliberately **held back** and what would
unblock each.

Read the **Gotchas** section before changing anything here; most of it is
non-obvious and was learned the hard way.

---

## TL;DR

- **Renovate** is the single dependency-update engine (Dependabot removed). It
  opens grouped PRs for libraries, GitHub Actions, and — via custom managers —
  the prebuilt-image toolchain in `versions.env`. It is **self-hosted on
  `mimir`** (a scheduled `docker run`, the `ripls-renovate` GitHub App, no Mend
  SaaS) so a `postUpgradeTask` can regenerate the Go protos before `go mod tidy`
  — the hosted app couldn't, which left every gomod PR with a stale `go.sum`.
  See [`renovate/README.md`](../../renovate/README.md).
- **Toolchain version mirrors move atomically.** The Go version (in
  `versions.env`, the `go.mod` directive, the prod Dockerfile `ARG`, and the
  setup-go pin) and the ONNX version (in `versions.env` + the Dockerfile `ARG`)
  are grouped so each bump is one PR that updates every `check_versions.js`
  mirror together — previously each surface opened its own PR and all failed.
- **`versions.env`** (repo root) is the single source of truth for the
  prebuilt-image toolchain. Consumers read it via build-args / sourcing — there
  are no hand-maintained mirrors. `scripts/check_versions.js` (CI) guards the few
  genuine source mirrors; `npm run doctor:versions` flags local drift.
- **Auto-merge is hands-off for safe updates:** patch/minor of
  gomod/npm/github-actions/gradle/terraform auto-merge on green with no human.
  PRs that need a person (majors, Flutter `pub`, base images) get a reviewer
  auto-assigned and native auto-merge enabled, so they merge **the moment the
  human approves** (§4). Patch/minor **baked-tool** (`needs runner rebuild`)
  bumps now auto-merge too, gated by a candidate-image check that proves the new
  toolchain pre-merge (§5); their majors still route to human review.
- **Failing Renovate PRs are auto-fixed by Claude.** When a bump needs a code
  migration, `claude_fix_renovate_ci.yaml` checks out the branch, migrates,
  pushes, and comments — with three layers guarding documented holds from being
  loosened (§8).
- **Required status checks** gate every merge (incl. Renovate's). Path-scoped
  checks use a job-level gating pattern so they're safe to require.
- **The self-hosted runner image is built on `mimir`, with no container
  registry.** Almost all CI runs on self-hosted runners.

---

## Components

### 1. Renovate — `renovate.json`

- Managers: `gomod`, `npm` (root + `cloud_functions/` + `e2e/`), `pub`,
  `github-actions`, `gradle`, `gradle-wrapper`, `terraform`, a **custom regex
  manager** over `versions.env`, plus auto-detected `dockerfile` /
  `docker-compose` / `bundler` / `cocoapods`.
- **Auto-merge** (`addLabels: ["automerge"]` + `platformAutomerge: true`):
  patch/minor of `gomod`/`npm`/`github-actions`/`gradle`/`gradle-wrapper`/
  `terraform`. **Manual:** all majors, `pub` (Flutter app deps), `dockerfile`/
  `docker-compose` (base images), `bundler`, and anything matching `versions.env`
  (labeled `needs runner rebuild`).
- **Blocked/pinned deps** (reasons in [Held-back dependencies](#held-back-dependencies)
  below): Dart `protobuf` (connect-dart cap), `analyzer` `<14` (freezed 4's
  ceiling), `microsoft/onnxruntime` `<1.30` (release-tarball ceiling, checked by
  `npm run check:onnx-tarballs`), `java` tracked
  major-only (`temurin-<major>-jdk` apt naming),
  `@types/node` `<23` (tracks Node 22 runtime),
  `mapbox_maps_flutter` (pinned), `dev.flutter.*` gradle plugins (SDK-provided,
  not on Maven).
- **Grouped deps** (one PR instead of several near-duplicates): `@types/node`
  across every package file (`types-node` — otherwise a caret-range project
  whose lockfile-only remediation splits onto a separate `…-lockfile` branch
  yields two identically-titled PRs for one bump) and the `@connectrpc/*`
  connect-es monorepo (`connectrpc`, released in lockstep) — alongside the
  existing `image decoders`, `go toolchain`, `onnx runtime`, and
  `android gradle toolchain` groups.
- **All non-major Go modules** are grouped into one `go modules` PR. Per-package
  gomod PRs each rewrite the shared `go.mod`/`go.sum`, so once one merges it
  re-conflicts every other open one and they drain only ~one per daily run (a
  backlog of N takes ~N days); grouping lands them in a single `go.sum` change.
  The rule sits **before** `image decoders` / `onnx runtime` / `go toolchain` so
  those more-specific groups still win (later rules override `groupName`) and the
  `go` directive stays in `go toolchain`. Majors are excluded — a breaking module
  bump stays an individual, human-reviewed PR.
- **`versions.env` custom manager:** each tracked line has a `# renovate:`
  annotation naming its datasource (Flutter / golang-version / node / java /
  github-releases for ONNX). NDK has no clean datasource → tracked manually.
- **Mirror custom managers + grouping (so `check_versions.js` passes):** the Go
  version also lives in the `go.mod` directive, the prod Dockerfile `ARG
  GO_VERSION` default, and the setup-go pin in `generate-proto-code`; ONNX also
  lives in the Dockerfile `ARG ONNXRUNTIME_VERSION` default. Custom managers
  surface the Go mirrors as `depName: go` and the Dockerfile ONNX ARG as
  `microsoft/onnxruntime`; a `go toolchain` packageRule (matched by **depName**,
  not datasource — that's what catches all four surfaces across managers) groups
  every Go surface into one branch, the duplicate dockerfile `golang` dep is
  disabled, and the ONNX mirror joins the `onnx runtime` group. Net: a Go or
  ONNX bump is one PR that moves every guarded mirror in lockstep.
- **Go go.sum `postUpgradeTask` (self-hosted only):** a gomod-scoped
  `postUpgradeTask` runs `scripts/renovate_gomod_regen.sh` (`buf generate
  --template buf.gen.go.yaml && go mod tidy`) so the branch carries a correct
  `go.sum`; only `go.mod`/`go.sum` are committed (the regenerated `server/gen`
  is gitignored). The command allowlist lives in `renovate/config.js`.
- **Path-versioned Go majors auto-migrate imports
  (`postUpdateOptions: ["gomodUpdateImportPaths"]`).** A major bump of a module
  whose import path carries the major (`go-github/v75` → `/v88`,
  `mailgun-go/v4` → `/v5`) changes the *path*, which lives in `.go` source — but
  Renovate's gomod manager edits only `go.mod`. Without this option such a bump
  landed as a cosmetic `go.mod`-only change our code never imported, then the next
  gomod PR's `go mod tidy` reconciled `go.mod` back to the still-`/v75`/`/v4`
  imports and silently **reverted** it — and Renovate re-proposed it forever
  (#2364/#2365 merged → reverted by #2466's tidy → re-proposed as #2477/#2478).
  `gomodUpdateImportPaths` runs the `mod` tool to rewrite the import paths too, so
  the bump is a real migration `tidy` won't undo; if the new major has other
  breaking API changes the PR then fails **honestly** (→ `claude_fix_renovate_ci`)
  instead of thrashing. Needs the `mod` tool (marwan-at-work/mod); `binarySource:
  install` does **not** supply it, so it is baked into the self-hosted Renovate
  image (see §6) — without it Renovate logs `spawn mod ENOENT` and silently skips
  the import rewrite.
- `schedule: ["before 6am on monday"]` limits **PR creation**;
  `automergeSchedule` defaults to any time, so auto-merge of green PRs isn't
  time-boxed. The self-hosted timer (`run-renovate`) can fire daily; on
  non-Mondays it rebases/auto-merges existing green PRs without creating new ones.

### 2. `versions.env` + `check_versions.js` + `doctor:versions`

- `versions.env` keys: `GO_VERSION`, `NODE_MAJOR`, `FLUTTER_VERSION`,
  `JAVA_VERSION`, `NDK_VERSION`, `ONNXRUNTIME_VERSION`, the Go lint toolchain
  (`GOLANGCI_LINT_VERSION`, `GOFUMPT_VERSION`), plus the iOS toolchain
  (`XCODE_VERSION`, `COCOAPODS_VERSION`, `RUBY_VERSION`, `IOS_DEPLOYMENT_TARGET`).
- **Consumed directly, not mirrored:** workflows source `FLUTTER_VERSION` into
  `$GITHUB_ENV` for `subosito`; the prod `Dockerfile` takes `--build-arg
  GO_VERSION`/`ONNXRUNTIME_VERSION` (with guarded ARG defaults for plain
  `docker build`); the runner `Dockerfile` declares **bare ARGs** (no defaults)
  fed by build-args.
- **The Go lint pins are baked AND honoured pre-merge (#3008).**
  `GOLANGCI_LINT_VERSION`/`GOFUMPT_VERSION` are baked like any other tool, but
  `.github/actions/setup-toolchain`'s `go-lint` input *also* installs them when
  the on-PATH version differs from the pin — so a bump takes effect in the PR
  that makes it, not only after the fleet rolls. This is the one baked tool with
  that property, and it exists because golangci-lint v1 and v2 **refuse each
  other's config file**: the schema migration had no state that was green both
  before and after the fleet rolled, so merging it the ordinary way would have
  turned every PR on `main` red until the next `recreate_runners` pass. Note
  that golangci-lint's module path carries its major
  (`github.com/golangci/golangci-lint/v2/cmd/...`), so a future major needs a
  `runners/Dockerfile` edit as well as a version bump.
- **One builder script, no hand-written arg lists:** both CI builds of the runner
  image go through `scripts/build_runner_image.sh <tag>`, which derives its
  `--build-arg` list from the Dockerfile's bare `ARG`s and fails loudly if
  `versions.env` has no value for one. Adding a baked tool therefore needs no
  workflow edit. #2883 is why: the `#1611` lint pins were wired into the merge
  build but not the candidate build, so every candidate image died on
  `golangci-lint@v` (empty version) and no baked-tool bump could pass its
  required check.
- `scripts/check_versions.js` (run on every PR by `check_versions.yaml`) enforces
  the two real source mirrors — `go.mod`'s `go` directive and the prod Dockerfile
  ARG defaults — plus a stray-literal guard (migrated files must not re-pin a
  literal), the runner-image **suppliers** (both workflows must delegate to
  `scripts/build_runner_image.sh`; `runners/docker-compose.yml` must pass every
  baked key as a build arg), and the iOS deployment target (Podfile + every
  Xcode build config).
- `npm run doctor:versions` (`scripts/doctor_versions.js`, pure core + node:test
  units) compares a developer's local Go/Node/Flutter/JDK — and Xcode/CocoaPods/
  Ruby on a Mac — against `versions.env`. Run it before "works on my machine."

### 3. Required checks + the job-gating pattern

Two repository **rulesets** protect `main` (NOT classic branch protection):

- **#9449689 "main requires review"** — 1 approval + deletion/non-fast-forward.
  The **Renovate app is a bypass actor** here (so Renovate skips *review* — see
  auto-merge below).
- **#17328641 "main required status checks"** — the required contexts, **no
  Renovate bypass** (so checks gate Renovate too). They're split on purpose: a
  ruleset bypass covers the *whole* ruleset, so checks must live separately.

**Required contexts (11):** `Go Tests`, `Flutter Tests`, `release-build`,
`build-ios`, `Check Versions`, `Gitleaks (PR)`, `Web E2E`,
`Cloud Functions Tests`, `Terraform Tests`, `test-build` (prod Docker image —
made required so auto-merge can't land a go.mod/server change that breaks the
image build), `Validate Runner Candidate` (builds a candidate runner image from
a `needs runner rebuild` PR's `versions.env` and runs Flutter/Android/Go checks
inside it — made required so a baked-tool bump can't auto-merge validated only by
the old image; #2395, see §4 and §5).

**The job-gating pattern** (every path-scoped required check uses it): the
workflow triggers on *all* PRs (no `on:`-level `paths:` filter); a `changes` job
(`dorny/paths-filter`, on `self-hosted-linux`) computes whether relevant paths
changed; the heavy job is `needs: [changes]` with
`if: ${{ always() && (github.event_name != 'pull_request' || needs.changes.outputs.relevant == 'true') }}`.
A PR that doesn't touch the paths **skips** the heavy job, which reports
**success** and satisfies the required check. Without this, a path-filtered
required check that never triggers shows "Expected — Waiting for status to be
reported" and blocks the PR forever.

### 4. Auto-merge (the tricky part)

`renovate.json` sets `platformAutomerge: true`, so Renovate enables **GitHub
native auto-merge** on eligible PRs. Two non-obvious facts shaped the design:

1. **Native auto-merge only waits for *required* checks.** So the test that
   validates an auto-merge-eligible change must itself be a *required* check, or
   the change merges unvalidated (this is why Web E2E / Cloud Functions /
   Terraform / `test-build` are required).
2. **Native auto-merge does NOT honor ruleset review-bypass actors** (verified:
   even an org-admin `always` bypass left a green auto-merge PR `REVIEW_REQUIRED`).
   So the Renovate review-bypass alone can't merge its PRs.

**Resolution — `renovate_auto_approve.yaml` (workflow name "Renovate PR
Automation").** A `pull_request_target` workflow, gated to the Renovate bot,
with two **mutually-exclusive** jobs split on the `automerge` label. A per-PR
`concurrency` group with `cancel-in-progress` serializes it: Renovate fires
`opened` plus one `labeled` event **per label** within a couple of seconds, so
without the guard each spawned a concurrent `approve` job that raced
`hmarr/auto-approve-action` — none saw the others' pending review and a 3-label
PR collected three separate approvals. Cancelling superseded runs leaves the
latest event's run, which approves once (idempotent if an earlier run already did). Baked-tool
`needs runner rebuild` PRs are routed like any other update (#2395): the required
`Validate Runner Candidate` check (§5) proves the new toolchain pre-merge, so a
patch/minor baked-tool bump auto-merges and a major routes to human review — both
still gated by that required check. (Previously both jobs excluded
`needs runner rebuild` entirely, because there was no pre-merge proof the new
image worked.)

- **`approve` (label present — patch/minor, incl. patch/minor baked-tool
  bumps):** approves via `hmarr/auto-approve-action`. A `github-actions[bot]`
  approval counts toward the 1-approval rule (confirmed), so review is satisfied →
  Renovate's native auto-merge merges on green. No human in the loop. For a
  baked-tool bump, "green" includes `Validate Runner Candidate`, so the rebuilt
  image is proven before merge.
- **`request-human-review` (label absent — majors incl. major baked-tool bumps,
  `pub`, base images, fastlane gems):** these need a person's eyes, but used to
  sit in `REVIEW_REQUIRED` until
  someone reviewed **and** manually clicked merge. This job requests review from
  `DEFAULT_RENOVATE_REVIEWER` (a repo variable naming the maintainer's login) and enables
  GitHub native auto-merge. Because native auto-merge **ignores the review-bypass**
  (above), the PR still waits for a **real human approval + green required
  checks** — then merges itself, no button-hunting. It does **not** auto-approve,
  so the human gate is intact. The reviewer re-request is guarded against
  dismissing an existing approval, and `--merge` is used (squash is disabled on
  this repo).

The author gate (both jobs) accepts **both** `renovate[bot]` (legacy Mend) and
`ripls-renovate[bot]` (self-hosted App) during the migration; drop `renovate[bot]`
once Mend is decommissioned.

This requires the org setting **"Allow GitHub Actions to create and approve pull
requests"** (enabled at the org level — it's off by default and a repo can't
override an org that forbids it).

### 5. Self-hosted runner image — built on `mimir`, no registry

- `runners/Dockerfile` (on `myoung34/github-runner:ubuntu-noble`) pins **no**
  tool versions — they're bare `ARG`s supplied from `versions.env`.
- **Validate (pre-merge):** `.github/workflows/validate_runner_candidate.yaml`
  runs on `self-hosted-linux` for every PR (job-gated). For a Renovate-authored
  `needs runner rebuild` PR it builds a per-PR `ripls-runner:pr-<N>` from the
  PR's `versions.env` and `docker run`s `scripts/runner_candidate_checks.sh`
  inside it (`--volumes-from "$(hostname)"` so the candidate sees the workspace +
  shared caches + host socket at their in-container paths) — exercising the **new**
  toolchain (Flutter codegen+analyze+test, Android release-build, Go compile)
  exactly as a production runner would. This closes the gap that the PR's own CI
  runs on the **old** image. Reported as the required check `Validate Runner
  Candidate`, so native auto-merge waits for it (§4) and a human can't merge an
  unvalidated bump. Gated to the Renovate bot because it executes PR-controlled
  content on a self-hosted runner with the host socket; a fork/human PR skips
  (→ success) and relies on human review.
- **Build:** `.github/workflows/build_runner_image.yaml` runs on
  `self-hosted-linux` on merges to `main` touching `versions.env`/`runners/*`.
  The runner mounts the host Docker socket, so its `docker build` (via the shared
  `scripts/build_runner_image.sh`, same as the candidate build) tags
  `ripls-runner:latest` **directly in mimir's local daemon** — no registry, no
  pull token. It only builds (no recreate), so it never kills its own job.
- **Recreate:** `runners/recreate_runners.sh` (run by the
  `ripls-runner-recreate{,-idle}` systemd timers, source-of-truth in repo,
  deployed by `sync.sh` to `/opt/runners`) rolls the fleet onto the new image.
  Two modes: no-arg `docker compose -p runners up -d --no-build` rolls the
  **whole** fleet (daily
  backstop); `--idle-only` rolls just the runners that are stale **and** idle
  (no live `Runner.Worker`), one at a time with `--no-deps`, so a short-interval
  schedule picks up a new toolchain within minutes of merge without killing
  in-flight jobs (#2395). It runs as host cron (not inside a runner) → no
  self-kill, and `--idle-only` self-skips the build job's own busy runner.
  The idle pass is `ripls-runner-recreate-idle.timer`, which passes
  `--idle-only` to the same script. **Both timers have to be enabled**; if only
  the daily one is, the fleet is still eventually correct, so the sole symptom
  is latency — an ONNX bump merged at 20:52 on 2026-08-28 did not reach the
  runners until the 04:40 pass next morning, and the pass that *did* run is the
  one that can kill an in-flight job. `grep 'idle pass'
  /var/log/ripls-runner-recreate.log | tail` shows whether it is live.

  Under Unraid this needed a second script (`recreate_runners_idle.sh`) purely
  because the User Scripts plugin invoked every script as `<name>/script` with
  no arguments. systemd passes the flag, so that wrapper is gone.
- **CPU budget:** each runner has `cpus: 10` (a ceiling Go, the JVM and Node
  size themselves to; Dart ignores it, so `test_flutter.yaml` passes
  `--concurrency` explicitly) and `cpu_shares: 128` (low cgroup weight, so the
  host's other services win under contention). Containers a job starts on the
  host daemon sit outside the runner's cgroup; the BuildKit builder and the
  candidate-image container carry the budget explicitly. Rationale and
  measurements: `runners/README.md` → *CPU budget*.
- `runners/docker-compose.yml` uses the shared local tag `ripls-runner:latest`;
  `runners/sync.sh --rebuild` is the manual build path; `sync.sh` (no args) ships
  the compose files, both host scripts, and `versions.env` to `/opt/runners`.
- **Android SDK is NOT pinned in the image.** The Android Gradle Plugin
  auto-downloads `build-tools`, the `compileSdk` platform, and `cmake` at build
  time (driven by `flutter.compileSdkVersion` + plugin CMake needs); licenses are
  pre-accepted so it's non-interactive. Only the SDK bootstrap (`cmdline-tools`),
  `platform-tools`, and the NDK are baked.

### 6. Self-hosted Renovate runtime — built/run on `mimir`, no registry

The same mimir-native, no-registry pattern as the runner image. Full detail in
[`renovate/README.md`](../../renovate/README.md); the essentials:

- **Image:** `renovate/Dockerfile` = `renovate/renovate` (pinned via the `FROM`
  tag, bumped by the dockerfile manager) + Go (build-arg from `versions.env`) +
  `buf` + `protoc-gen-go`/`protoc-gen-connect-go` + `mod` (marwan-at-work/mod, for
  `gomodUpdateImportPaths` — see §1; `binarySource: install` does **not** supply it,
  so it must be baked or Renovate logs `spawn mod ENOENT`). Only the Go toolchain is
  baked — the Go buf template uses local plugins, so no Flutter/Dart/Java is needed.
- **Build:** `build_renovate_image.yaml` runs on `self-hosted-linux` on merges
  to `main` touching `versions.env`/`renovate/Dockerfile`, tagging
  `ripls-renovate:latest` directly in mimir's daemon.
- **Run:** `/opt/nas/scripts/run-renovate.sh`, owned by the **mimir repo** and
  fired off-peak by the `run-renovate` systemd timer. It mints a
  `ripls-renovate` GitHub App installation token (`renovate/mint_app_token.js`,
  node built-ins only), then `docker run --rm` the image — memory/CPU-capped and
  `flock`-guarded for mimir's reliability budget. No long-running container and
  no compose stack, so it always uses the newest image and
  `update-docker-stacks` ignores it. This repo shipped its own copy of that
  entrypoint until the Ubuntu migration, when the two diverged and the local one
  was deleted.
- **Auth/secrets:** the App private key + ids live host-only under `/etc/nas/`,
  deliberately outside the backed-up app directory (never committed).
  `renovate/config.js` is
  the admin config (repo list, `binarySource: install`, the `allowedCommands`
  allowlist for the postUpgradeTask). `renovate/sync.sh` deploys the non-secret
  glue.

### 7. iOS & Android toolchain tracking

- **iOS** (recorded from the `tinker` self-hosted-Mac runner): `XCODE_VERSION`,
  `COCOAPODS_VERSION`, `RUBY_VERSION`, `IOS_DEPLOYMENT_TARGET` in `versions.env`;
  doctor-checked on Macs; `IOS_DEPLOYMENT_TARGET` also enforced against the
  Podfile + Xcode project by `check_versions.js`. `test_ios_build` fails the PR
  if `Podfile.lock` drifts after `pod install` (catches Firebase-pod surprises).
- **A `FLUTTER_VERSION` bump is an iOS change.** The `Flutter` pod's
  `SPEC CHECKSUMS` entry tracks the SDK, so bumping it requires a committed
  `pod install` — which is why `versions.env` is in `test_ios_build`'s path
  filter. It is the required check that blocks such a bump until the lock is
  regenerated; `validate_runner_candidate` cannot cover this (it builds the
  **Linux** runner image and never runs CocoaPods). Learned from #2883, whose
  bump auto-merged and left every `main` iOS build red until #2920. To fix such
  a PR you do not need a Mac: the failing check prints the lock diff, and the
  `+  Flutter: <sha>` line in it *is* `pod install`'s output from tinker.
- **Android** is mostly self-tracking: Gradle (wrapper), AGP, Kotlin, Firebase
  plugins, and lib deps are Renovate-managed and committed in-repo (so inherently
  synced); `compileSdk`/`targetSdk`/`minSdk`/`ndkVersion` are `flutter.*`
  (driven by the pinned Flutter); NDK + JDK are in `versions.env`.

### 8. Claude auto-fix for failing Renovate PRs (`claude_fix_renovate_ci.yaml`)

A major dependency bump often fails CI because the codebase needs a code
migration (a renamed Go API, a Terraform resource rename, a Gradle/Kotlin/TS
major). `claude_fix_renovate_ci.yaml` — a sibling of `claude_fix_ci_failure.yaml`
scoped to `renovate/*` branches — checks out such a PR, applies the migration,
pushes, and comments, instead of leaving it red for a human (#2396).

- **Trigger:** `workflow_run: completed` for any of the required-check workflows
  (`Go Tests`, `Flutter Tests`, `Terraform Tests`, `Test iOS Build`,
  `Test Docker Build`, `AI Provider Integration Tests`,
  `Run End-to-End Tests on Dev`) whose `head_branch` is `renovate/*`, plus manual
  `workflow_dispatch`. Serialized per branch via a `concurrency` group so the
  burst of events from several failed checks on one PR can't fan out parallel
  runs.
- **This workflow adds NO `automerge` label and runs NO `gh pr merge`** — all
  merge wiring lives in §4, and Claude's push fires `renovate_auto_approve`'s
  `synchronize` trigger, which re-applies it. An already-eligible patch/minor
  merges with no human once Claude's fix turns it green; a **major** has native
  auto-merge enabled and the reviewer assigned (the `request-human-review` job),
  so once Claude makes it green it merges **as soon as the human approves** — the
  human review is the only added gate, and the required-check set (incl.
  `test-build`) still applies. No special path to `main`.
- **Three safety layers stop Claude "fixing" CI by tearing out a documented hold:**
  1. **Pre-gate (discover):** skips `needs runner rebuild` PRs (baked-tool bumps —
     [#2395](https://github.com/Type2Fun-Foundation/ripls/issues/2395)'s scope)
     and skips any PR about a dependency that `renovate.json` caps
     (`allowedVersions`) or disables (`enabled:false`). The held-dep "needles" are
     derived live from `renovate.json` (so they never drift) and matched against
     the PR title/body/branch. Conservative — an over-skip just defers to a human.
  2. **Prompt:** Claude is handed `renovate.json` + this doc's *Held-back
     dependencies* section and a hard rule never to loosen a cap, re-enable a
     disabled dep, edit `renovate.json`, or downgrade the bump. If CI can only go
     green by crossing a documented hold, it comments and stops.
  3. **Post-fix guard (fix job):** if Claude's commits touched `renovate.json`, the
     guard **reverts that file on the branch** (keeping the rest of the migration)
     and fails the run. Failing the side-workflow alone wouldn't stop a merge — the
     commit is already pushed — so the active revert is the teeth; it runs
     immediately after the push, minutes before CI could go green.
- **Cost / loop control:** a per-head-SHA marker comment
  (`<!-- claude-renovate-fix:SHA -->`) dedups repeat triggers on the same branch
  state, and a rate cap (≥3 Claude markers within 6h → back off) bounds total
  attempts and breaks any revert-loop (the guard's revert advances the SHA, so the
  per-SHA dedup can't catch a loop on its own).
- **Auth:** the migration runs in the private agent repo (#2970), which
  checks out and pushes as `ripls-agent[bot]` with a token scoped to this repo
  (it can push to a `ripls-renovate[bot]`-authored branch); this repo only holds
  the dispatch credential. The renovate.json guard runs there too. A Claude commit on the branch is **not** clobbered
  by Renovate: Renovate's `isBranchModified` check sees the non-Renovate commit
  and skips the branch ("Branch has been edited … skipping") regardless of
  `rebaseWhen`.

**`rebaseWhen: "behind-base-branch"`.** Renovate rebases *every* open branch that
falls behind `main`, so a PR's CI always reflects the current base — otherwise a
stale branch's red/green is uninterpretable (you can't tell a real failure from
stale-base noise). The journey here: `conflicted` (added to protect Claude's
fix-commits, but it froze the whole backlog stale) → default `auto` (only
freshens the automerge candidates Renovate is driving toward merge, **not** the
human-review majors — confirmed empirically) → `behind-base-branch` (freshens
all). `isBranchModified` still protects Claude-modified branches under it (they're
skipped, so they won't auto-freshen — rebase those manually / via the PR's rebase
checkbox if needed). Trade-off: every `main` commit re-bases + re-runs CI on
behind branches; acceptable for trustworthy PR signal.

---

## Held-back dependencies

These are deliberately pinned below their latest version. Renovate enforces the
caps; this is the canonical record of **why** and **what unblocks each**. When a
blocker clears, remove the cap in `renovate.json` and let the update flow.

### Hard blockers — won't resolve or won't run

#### Dart `protobuf` 4 → 6 — blocked by the `connect-dart` client (server is fine)

A **client-only** blocker. `connectrpc` (latest published Dart release)
constrains `protobuf: ">=3.1.0 <5.0.0"`, and our generated Dart code targets the
protobuf v4 runtime. Three pieces must move together to adopt protobuf 6 in the
app:

1. **Runtime:** a `connect-dart` release that allows protobuf 6. None exists.
   Open upstream PR [connectrpc/connect-dart#38 "Protobuf v6"](https://github.com/connectrpc/connect-dart/pull/38)
   (conflict-free, has maintainer attention but unmerged/unreleased), plus
   [#36](https://github.com/connectrpc/connect-dart/pull/36) for well-known-type
   imports. The PR moves the constraint to `protobuf: ">=6.0.0 <7.0.0"`.
2. **Message codegen:** `protoc-gen-dart` ≥ 25 emits protobuf-6 code
   (`protoc_plugin` 23/24 → protobuf 5, 25 → protobuf 6). Our `buf.gen.dart.yaml`
   pins `buf.build/protocolbuffers/dart:v21.1.2` (protobuf 4) and would bump to
   the v6 remote plugin.
3. **Service codegen:** the `connect-dart` plugin
   (`buf.build/connectrpc/dart:v1.0.0`) must emit code for the new runtime. The
   v6 plugin is not published to buf.build, so until #38 ships we'd have to build
   and run the fork's plugin locally.

**Not worth forcing now:** the only path today is pinning a git dependency to an
unmerged fork branch *and* running a locally-built connect codegen plugin for a
core RPC transport — fragile and high-maintenance. The clean unblock is upstream
merging #38/#36, cutting a `connect-dart` release, and publishing the v6 buf
remote plugin. **Track #38.**

**The Go server needs no change.** "protobuf 5/6" is the Dart `protobuf` *pub
package* version, not a wire/spec version and not shared with Go. The server is
current (`connectrpc.com/connect` + `google.golang.org/protobuf`, which has a
single v1.x line — there is no "v5/v6"). The interop contract is the Connect
protocol + proto3 binary wire format + protobuf-JSON mapping — all frozen,
language-independent specs. The Dart `protobuf` 5.x/6.0 changelog confirms **no
wire or JSON format change** (only Dart API changes), so a protobuf-6 Dart client
emits byte-identical messages to today's protobuf-4 client and the connect-go
server accepts them unchanged.

#### Dart `analyzer` — capped `<14` by freezed (resolved for mockito, Aug 2026)

`analyzer` is a single shared node in the dependency graph: every build_runner
generator must agree on one version. `freezed` 4.0.0 — our `@freezed` codegen
backbone — requires `analyzer ^13.0.0`, and `mockito` 5.8.x wants
`analyzer >=13.3.0 <15.0.0`, so **13.x is the one window that satisfies both**.
That is the whole of the cap today; `renovate.json` holds `analyzer` at `<14.0.0`
and nothing else in the stack is pinned. **Lift to `<15` when a freezed release
widens past analyzer 13.**

This used to be a hard block — `mockito` was frozen at 5.6.4 and `analyzer` at 10
because no freezed release allowed analyzer 13, and forcing it broke `dart_style`
3.1.7 and with it the whole codegen pipeline. Three upstream moves cleared it:
`freezed` 4.0.0 allows `analyzer ^13.0.0`, `dart_style` 3.1.13 compiles against
analyzer 13, and the **Flutter 3.47.1 bump** supplied the Dart 3.13 floor freezed
4 requires *and* relaxed `flutter_test`'s `meta` pin from an exact `1.18.0` to
`^1.18.3` (analyzer ≥13.1 needs `meta ^1.18.3`, so on Flutter 3.44 it could not
resolve at all, whatever freezed allowed).

One in-repo migration was needed, in `app/tool/dead_code.dart`: analyzer 13
removed the `NamedCompilationUnitMember` supertype that exposed `name`
generically, and moved member lists behind a body node
(`ClassDeclaration.members` → `.body.members`, with enums carrying an `EnumBody`
and the rest a `ClassBody`). Names are now read per declaration kind — the
type-shaped declarations via `ClassNamePart.typeName`, functions and typedefs via
a bare token. Verified behaviour-preserving by diffing the tool's full report
before and after: byte-identical.

#### Go `onnxruntime_go` — native ORT version coupling (capped `<1.30`)

`github.com/yalue/onnxruntime_go` is coupled in lockstep to the native ONNX
Runtime library: each binding release vendors a fixed `ORT_API_VERSION` that the
loaded `.so`/`.dylib` must support (ORT 1.N supports API ≤ N). **The binding's own
version number is not the API version** — read
`onnxruntime_c_api.h` in the module to find it. Currently binding **v1.35.0**
(`ORT_API_VERSION 29`) against native ORT **1.29.0** in `Dockerfile`,
`runners/Dockerfile` and `versions.env`.

> **The two halves cannot move in one PR, even though they are one logical
> bump.** `Go Tests` runs on the *current* runner fleet, and the fleet only picks
> up a new `versions.env` toolchain after merge — `build_runner_image` rebuilds
> `ripls-runner:latest` on push to main, then mimir's scheduled
> `recreate_runners.sh --idle-only` rolls idle runners onto it. So a PR that
> raises the binding's required API above what the *running* image provides fails
> `Go Tests` no matter how correct the pairing is. `Validate Runner Candidate`
> does not save you here: it only runs on Renovate-authored PRs and skips (and
> reports success) on a human one.
>
> Sequence it instead, which works because ORT is backward compatible — a 1.29
> runtime still serves API 22: **(1)** raise the native runtime alone and merge,
> **(2)** let the fleet roll, **(3)** raise the binding. Step 1 is green on the
> old fleet *and* the new one, which is what makes it safe to merge first.

The remaining ceiling is the *tarball* ceiling, not the API ceiling
([#2416](https://github.com/Type2Fun-Foundation/ripls/issues/2416)): both images
download `onnxruntime-linux-<arch>-<v>.tgz` from the GitHub release, and some ONNX
Runtime patch releases are package-manager-only and publish no tarball. Pinning
one 404s the image build and the PR sticks red on `Validate Runner Candidate`
with a wget error (caught live by PR #2370). That is why the cap is expressed
against tarball availability.

> **Raise the cap from evidence, not prose.** `npm run check:onnx-tarballs`
> asserts the pinned `ONNXRUNTIME_VERSION` publishes both the x64 and aarch64
> tarballs; pass a version to vet a candidate before pinning it
> (`npm run check:onnx-tarballs -- 1.30.0`). It is not in `npm run lint` — it
> needs the GitHub API, and a lint gate that fails when you are offline or
> rate-limited is worse than the problem it prevents.
>
> This cap sat at `<1.22.1` for months after the constraint that justified it had
> passed: 1.26 through 1.29 all publish both tarballs. A hand-maintained cap whose
> reason is only prose does not notice when the reason expires.

> **Watch-out for `go get -u`:** it will silently pull `onnxruntime_go` past what
> the pinned native ORT supports. The embedder then fails to initialize
> (`Error setting ORT API base`) and every `server/integration_tests` test — each
> spawns the server binary — hangs to the test timeout. Renovate groups the
> binding with the native ORT version; **bump both together**, and verify locally
> by pointing `ONNXRUNTIME_LIB_PATH` at the target release's library. That env var
> is checked *before* the system paths precisely so a specific runtime can be
> tested against an installed one.

#### npm `firebase-admin` 14 — firebase-functions peer cap (`cloud_functions`)

Held at **<14** in `cloud_functions`. `firebase-functions` peer-requires
`firebase-admin: "^11.10.0 || ^12.0.0 || ^13.0.0"`, and the **latest**
`firebase-functions` (7.2.5) still caps at v13 — so `firebase-admin@14` fails
`npm install` with `ERESOLVE` (peer conflict) and the PR can never go green.
There is no `firebase-functions` release to bump to that widens the range yet, so
this is a pure upstream wait (capped in `renovate.json`). **Unblock:** a
`firebase-functions` release whose `firebase-admin` peer range includes `^14`;
then bump both together and lift the cap.

### Deferred majors — updatable, but each needs a code migration + manual QA

These resolve fine; they're left at the current major because the new major has
breaking API changes that touch app code and want manual QA. Each is a
self-contained follow-up.

| Package | Current → Latest | Notes |
| --- | --- | --- |
| `camera` | 0.11 → 0.12 | Platform interface bump (`camera_android_camerax` 0.6→0.7, `camera_avfoundation` 0.9→0.10). 4 usages. |

**Runtime-tracking pins (intentional, not deferrals):**

- `@types/node` tracks the runtime — pinned to the Node 22 line in both
  cloud_functions and e2e, not the latest 25.x. Bump it only when the Node
  runtime moves.

**cloud_functions eslint:** `eslint` is on v10 in `cloud_functions/package.json`,
but its `lint` script still calls the removed `--ext .ts` flag and there is no
flat `eslint.config.js`, so `npm run lint` there is broken. It is **not wired
into CI** (the `Cloud Functions Tests` workflow runs no lint step), so nothing is
blocked — but the flat-config migration is still owed (see follow-ups).

---

## Gotchas / non-obvious decisions — READ THIS

- **Renovate reads its config from the DEFAULT branch.** A dry-run with
  `RENOVATE_BASE_BRANCH_PATTERNS=<branch>` processes that branch's *files* but
  still loads `renovate.json` from `main`. To actually exercise a feature-branch
  config, point `RENOVATE_CONFIG_FILE` at the branch's `renovate.json` (and
  supply admin-only keys like `RENOVATE_ALLOWED_COMMANDS` via env). Config
  changes only take real effect once merged to `main`.
- **`baseBranches` is not a CLI flag.** It was renamed to `baseBranchPatterns`,
  and *neither* is a `renovate` CLI option in v43 — set config options via
  `RENOVATE_*` env vars, not `--flag`.
- **Generated Go code isn't committed, so Renovate's built-in go.sum refresh
  fails.** `go get`/`go mod tidy` can't resolve the in-module `server/gen/**`
  imports ("cannot find module providing package …") and leaves a stale go.sum.
  The fix is the gomod `postUpgradeTask` (`renovate_gomod_regen.sh`), which is
  why Renovate is self-hosted at all — the hosted Mend app can't run it. The
  built-in attempt still fails *first* (then the task overwrites go.sum), so
  Renovate would stamp a red `renovate/artifacts` status even though the branch
  is correct; `statusCheckNames.artifactError: null` suppresses that cosmetic
  status. It's non-required either way, and a genuinely broken go.sum still fails
  the required Go Tests.
- **Group the toolchain mirrors by `depName`, not datasource.** `check_versions`
  requires the Go version identical across `versions.env`, `go.mod`, the
  Dockerfile `ARG`, and the setup-go pin. Those surface across three managers
  with *different* datasources but the same `depName: go`, so the `go toolchain`
  group must match on `depName` (matching datasource+packageName drops the
  setup-go pin into a stray `renovate/go-1.x`). The duplicate dockerfile `golang`
  dep (depName `golang`, same Dockerfile `ARG`) is disabled so two managers don't
  fight over one line.
- **Native auto-merge ignores ruleset review-bypass** (any mode). Auto-merge for
  Renovate works via the **auto-approve workflow**, not the bypass. If auto-merge
  ever stops, check: org "Allow Actions to approve PRs" still on, the `automerge`
  label present, and the auto-approve run green.
- **Auto-merge only waits for *required* checks.** Adding an auto-merge-eligible
  manager means making its validating test a required check (with job-gating).
- **Path-scoped required checks must be job-gated** (skip→success). A required
  check whose workflow is filtered out at the `on:` level never reports →
  "Expected — Waiting" → blocks the PR.
- **Removing `on:`-level path filters means every PR triggers every gated
  workflow.** When the `changes` detectors ran on GitHub-hosted `ubuntu-latest`,
  a burst (several Renovate rebases at once) saturated GitHub's hosted
  concurrency cap → runs queued at GitHub's level → "Expected — Waiting" with
  *idle* self-hosted runners. Fix: the `changes` jobs (and nearly everything) run
  on `self-hosted-linux`.
- **`gitleaks/gitleaks-action` breaks on these self-hosted runners** — the scan
  passes but the action crashes on SARIF artifact upload ("rootDirectory is not a
  parent directory of the file") because the `myoung34` runner exposes the
  workspace at both `/github/workspace` and `/tmp/runner/work`. `gitleaks-pr` /
  `gitleaks-push` are kept on GitHub-hosted as a result (they add per-ref
  concurrency so redundant scans don't pile up on rapid pushes/Renovate rebases).
- **`onnxruntime_go` is coupled to the native ONNX Runtime.** A bare `go get -u`
  pulls the binding past the API the shipped `.so` supports and the server's
  embedder fails to init (and every `server/integration_tests` hangs). Renovate
  caps it `<1.25` and groups it with the native ORT version; bump both together.
- **A baked-tool version must be one `runners/Dockerfile` can actually build.**
  The `Validate Runner Candidate` gate (#2395) validates the candidate **image
  build**, so a `versions.env` value the Dockerfile can't realize fails the gate
  and sticks the PR red — it caught two real cases live
  ([#2416](https://github.com/Type2Fun-Foundation/ripls/issues/2416)), both now
  fixed at the *proposal* end so the version is never offered:
  (1) `JAVA_VERSION` is consumed as `temurin-${JAVA_VERSION}-jdk` (Temurin apt
  packages are **major-only**: `temurin-17-jdk`), but the `java-version`
  datasource returns full strings (`17.0.20+8` → nonexistent
  `temurin-17.0.20+8-jdk`), so every java *patch* bump failed — the annotation now
  carries `extractVersion=^(?<version>\d+)`, collapsing all 17.x.y to `17` (no
  patch PR) while majors (21, 25) still surface for human review and map to real
  packages; (2) ONNX `1.22.1/1.22.2` have no release tarball, so the cap is
  `<1.22.1` (above). Pre-#2395 a java patch would have silently broken the runner
  rebuild *after* merge. **When adding a new `versions.env` annotation, check the
  datasource's version *shape* against how the Dockerfile consumes it** — that
  mismatch, not the version itself, is the recurring failure.
- **`JAVA_VERSION` has a floor set by AGP, not just by AGP's documented
  minimum.** AGP 9.3+ bundles a lint (`intellij-core` `JavaDocParser`) that calls
  `java.util.List.removeLast()` — a **Java 21** API (JEP 431) — from **Java 8
  target bytecode**, so the call is unguarded and only resolves at runtime. On
  JDK 17 it threw `NoSuchMethodError` the moment `CommentDetector` parsed any
  Javadoc comment, killing `lintVitalAnalyzeRelease` and with it every Android
  release build ([#2787](https://github.com/Type2Fun-Foundation/ripls/issues/2787),
  caught live by PR #2782) — while AGP 9.3.0's own release notes list JDK 17 as
  the minimum *and* default. `JAVA_VERSION=25` clears it; **do not drop
  `JAVA_VERSION` below 21 while AGP is ≥9.3.**
- **To decide whether a dependency needs a newer JDK than we run, inspect its
  bytecode — don't reproduce the build.** `javap` settled #2787 in seconds where a
  release build takes ~16 min. `major version: 52` (Java 8 target) alongside a
  call to a newer-JDK API is the broken shape:

  ```bash
  V=32.3.1   # lint version = AGP version + 23
  curl -sO "https://dl.google.com/dl/android/maven2/com/android/tools/external/com-intellij/intellij-core/$V/intellij-core-$V.jar"
  unzip -o -q "intellij-core-$V.jar" 'com/intellij/java/syntax/parser/JavaDocParser*'
  javap -v -p com/intellij/java/syntax/parser/JavaDocParser.class | grep -E 'major version|removeLast'
  ```

- **The candidate gate never runs lint.** `scripts/runner_candidate_checks.sh` is
  `go build` + `go build -tags=integration` + the Flutter/Android checks — it does
  not run `npm run lint:go`. So a baked *lint* toolchain bump is validated by
  that gate not at all, on any PR. That is why the lint pins get the
  `setup-toolchain` treatment above rather than relying on the candidate image.
- **The candidate gate runs `go build`, not `go test`.** Go is self-validating:
  `GOTOOLCHAIN=auto` means the **`go.mod`** toolchain (not the baked `GO_VERSION`
  bootstrap) compiles + tests the code, so the existing required `Go Tests` check
  already exercises a go bump on the old image. Confirmed live: go 1.26.4 passed
  `Go Tests` on its PR (#2381) before auto-merge; the candidate only compile-smokes
  it (and avoids needing postgres/secrets in the PR-controlled container).
- **`npx renovate` may resolve a stale cached major** that rejects
  `managerFilePatterns` (it wants the old `fileMatch`). Validate with
  `renovate@latest` explicitly.
- **A domain message model's `sentAtUnixSec` is a plain `int`,** not the proto's
  `Int64` (wrap with `Int64(...)` in tests).
- **"Hidden Unicode" Renovate warnings** were a lone `0xA0` byte (Latin-1 NBSP,
  not the UTF-8 two-byte `C2 A0`) in HTML release-notes — `grep` for the right
  encoding when chasing these.
- **The Renovate auto-fixer must not add `automerge` or call `gh pr merge`**
  (§8). Adding the `automerge` label would route a *major* through the `approve`
  job and auto-land it with **no human review** — the human gate for majors comes
  precisely from their *not* having that label (they go through
  `request-human-review` instead, which enables auto-merge but does not approve).
  So the fixer leaves all merge wiring to §4 and relies only on its push
  re-firing `renovate_auto_approve` via `synchronize`. Its three hold-safety layers are
  belt-and-suspenders: the discover pre-gate + the prompt rule can both be
  fooled, but the post-fix **revert** of any `renovate.json` change is the
  mechanical backstop — failing the side-workflow alone does **not** stop a
  merge (the commit is already pushed), so the revert is what actually keeps a
  loosened cap off `main`.
- **`gh` and webhook payloads spell App-bot logins differently.** `gh pr view
  --json author` reports a GitHub App bot as **`app/<slug>`** (e.g.
  `app/ripls-renovate`), while the `pull_request` webhook payload uses
  **`<slug>[bot]`** (e.g. `ripls-renovate[bot]`). The §8 fixer first shipped
  gating on the `[bot]` form via `gh`, so it matched nothing and was a silent
  no-op until #2400. When gating on a bot author in a workflow that resolves it
  via `gh`, normalize by stripping an `app/` prefix and a `[bot]` suffix and
  matching the bare slug — `renovate_auto_approve.yaml`, which reads the author
  from the *event* payload, correctly uses the `[bot]` form instead.
- **Enabling auto-merge on an already-`CLEAN` PR merges it immediately.** A PR
  that is green on all required checks AND has its required approval shows
  `mergeStateStatus: CLEAN`; `gh pr merge --auto` on it doesn't "wait", it
  merges on the spot. When manually poking at auto-merge, confirm the PR is
  genuinely red/unapproved first — don't assume from a stale status.
- **Proto Dart/TS codegen needs `BUF_TOKEN` in every CI job.**
  `generate:proto:dart` and `generate:proto:ts` run `buf generate` against
  *remote* plugins on buf.build (the BSR); unauthenticated, they hit BSR rate
  limits (`generate:proto:go` uses local plugins — no token). Two ways this
  drifts: a job that forgets `env: BUF_TOKEN`, or a **reusable-workflow caller
  that doesn't pass it** — reusable workflows do **not** inherit secrets, so it
  must be declared in `workflow_call.secrets` and passed by every caller (this is
  how the deploy path — `deploy_to_dev_on_push` → `test_dev_e2e` /
  `build_and_deploy_container` — silently went unauthenticated). `generate:proto:dart`
  and `:ts` go through `scripts/buf_generate.sh`, which **fails fast in CI** with a
  clear message if `BUF_TOKEN` is empty, so drift breaks loudly at the point of use
  instead of as a confusing rate-limit. Local dev is unaffected (the guard is
  CI-only; developers authenticate via `buf registry login`).
- **A required check that *skips* still reports SUCCESS.**
  `Validate Runner Candidate` gates its real work on the PR being
  Renovate-authored — a genuine security control, since it builds and runs an
  image from the PR's own Dockerfile on mimir's daemon. But the gate is a skip,
  and a skipped required check is green. So a human-authored toolchain PR gets a
  tick from the one check that exists to validate toolchain PRs, having built
  nothing. PR #3010 hit exactly this. The gate stays; the silence does not — an
  `unvalidated-notice` job now annotates the run and comments once on the PR.
  **Generalise the shape:** whenever a required check is gated by `if:`, work out
  what its green tick means when the condition is false.
- **A leftover branch from the OLD bot identity is invisible-dead, not idle.**
  Renovate decides whether it may touch a branch by comparing the last commit's
  author to its own `gitAuthor`. The hosted Mend app committed as
  `renovate[bot]`; self-hosted Renovate commits as `ripls-renovate[bot]`. Every
  branch left over from before the self-hosted migration therefore reads as
  *human-edited*, and Renovate silently refuses to update it — it shows up under
  **PR Edited (Blocked)** on the Dependency Dashboard, which looks like a benign
  informational row rather than a stuck job. `renovate/lock-file-maintenance`
  sat that way from June 2026 until it was found in August: **no lockfile in the
  repo had been refreshed for ~3 months** (`app/pubspec.lock` had drifted ~100
  packages behind the ranges its own `pubspec.yaml` already allowed, and
  `Gemfile.lock` / `package-lock.json` / both `.terraform.lock.hcl` files with
  it). The fix is to delete the stale branch so Renovate recreates it under the
  current identity. **When auditing, read the dashboard's blocked sections as a
  work queue, not as status.**
- **`lockFileMaintenance` is recurring hygiene, so it sets
  `recreateWhen: "always"`.** With the default (`auto`), closing one week's lock
  file PR suppresses it permanently — the closed PR becomes a standing block.
  That is the right default for a specific dependency bump you have decided
  against, and the wrong one for a job whose whole purpose is to run again next
  week. Closing PR #2356 in June 2026 is half of why lockfile maintenance
  stayed dead (the stale-identity branch above is the other half); `always`
  makes a close mean "not this week" instead of "never again."

---

## Operational runbook

- **Bump a prebuilt-tool version (Go/Node/Flutter/JDK/NDK/ONNX):** edit
  `versions.env` (Renovate also opens these PRs, labeled `needs runner rebuild`).
  Pre-merge, `validate_runner_candidate` builds a candidate image and runs the
  toolchain checks inside it (§5); patch/minor Renovate bumps then auto-merge on
  green, majors after human review. On merge, `build_runner_image` rebuilds
  `ripls-runner:latest` on mimir and `recreate_runners.sh` rolls the fleet onto
  it (the short-interval `--idle-only` pass within minutes, the daily full pass as
  backstop). `npm run lint:versions` must stay green (update `go.mod` / prod
  Dockerfile ARG defaults if it complains).
- **Bump the Flutter pin:** the version is sourced two ways — baked into
  `runners/Dockerfile` (used by `self-hosted-linux` jobs) and via
  `subosito/flutter-action` + a `FLUTTER_VERSION` env in `test_ios_build`,
  `release_app`, and `release_build_android`. Both read `versions.env`. The
  `self-hosted-linux` jobs need a runner-image rebuild to pick up the new Flutter;
  the `subosito`-installed Mac jobs take effect on the next run. The
  `release_app` / `release_build_android` bumps mean the next production release
  builds on the new Flutter — validate device builds at that release cut.
- **Interrogate Renovate / its dashboard from the CLI:**
  `npx renovate@latest --platform=github --token="$(gh auth token)"
  --dry-run=full --onboarding=false Type2Fun-Foundation/ripls` reproduces the
  "Repository problems" and extraction. (Local Node 25 trips its Node-24 engine
  check → exit 1, but it still runs.) The in-repo **Dependency Dashboard issue**
  is the primary inspection surface (self-hosting keeps it; only Mend's hosted
  log console is lost — replaced by the tee'd cron log on mimir).
- **Run / debug self-hosted Renovate on mimir:** the entrypoint is
  `/opt/nas/scripts/run-renovate.sh` (mimir repo), fired by the `run-renovate`
  timer. For a one-off, `systemctl start run-renovate` or run the script by hand
  over ssh; `DRY_RUN=1` makes it `--dry-run=full`. Logs:
  `journalctl -u run-renovate -n 200`. Rebuild the image
  via the `build_renovate_image` workflow (or a local `docker build` into mimir's
  daemon). After editing `renovate.json`, validate with
  `npx --yes --package renovate@latest -- renovate-config-validator renovate.json`.
- **Renovate auth (`ripls-renovate` GitHub App):** app id + installation id and
  the private key live host-only under `/etc/nas/`, deliberately outside the
  backed-up app directory. `mint_app_token.js` exchanges them for a
  short-lived installation token each run. The App is a bypass actor on ruleset
  **#9449689** and its bot (`ripls-renovate[bot]`) is gated in
  `renovate_auto_approve.yaml`.
- **Add/remove a required check:** edit ruleset **#17328641** via
  `gh api -X PUT repos/Type2Fun-Foundation/ripls/rulesets/17328641`. Job-gate the
  workflow first (skip→success) and merge that to `main` *before* adding the
  context, or in-flight PRs strand on "Expected — Waiting" until rebased.
- **Migrate / check the runner fleet:** busy detection per runner is
  `docker exec github-runner-N pgrep -f Runner.Worker` (the org-runners API needs
  `admin:org`). Roll idle runners with `docker compose -p runners
  up -d --no-build --no-deps github-runner-N` — or just run
  `recreate_runners.sh --idle-only`, which does exactly this for every stale +
  idle runner.
- **Cut a release / deploy:** release & website workflows run on
  `self-hosted-linux`; each is a one-line `runs-on` revert if a job needs to move
  back to GitHub-hosted.
- **A Renovate PR is red on a code migration:** `claude_fix_renovate_ci.yaml`
  (§8) attempts it automatically. To trigger a one-off by hand:
  `gh workflow run claude_fix_renovate_ci.yaml -f pr_number=<N>`. To force a
  re-attempt after the per-SHA dedup or the 3-in-6h rate cap has it backing off,
  delete the `<!-- claude-renovate-fix:… -->` marker comments on the PR. If
  Claude posts a "documented hold" skip, decide in this doc's *Held-back
  dependencies* section whether the cap can be lifted — don't just remove it on
  the branch (the post-fix guard will revert that).

---

## GitHub-hosted exceptions (deliberate)

Almost all jobs run on `self-hosted-linux`. Three stay on GitHub-hosted on
purpose:

- **`renovate_auto_approve`** — it's `pull_request_target`; running that trigger
  on a persistent self-hosted runner is a security anti-pattern. Trivial API-only
  job, so ephemeral hosted is the safer choice.
- **`gitleaks-pr` / `gitleaks-push`** — `gitleaks-action` is incompatible with
  the self-hosted workspace layout (see Gotchas). To reach *zero* hosted jobs,
  switch these to invoking the `gitleaks` **binary** directly (also drops the
  action's org-license requirement) — not yet done.

## Instance-only workflows (`INSTANCE_PIPELINES`)

Some workflows only make sense in the repo that owns this deployment's runner
fleet and credentials: the runner and Renovate image builds and the runner
candidate validation. They run on fixed self-hosted labels, so in any other
repo (the public mirror, a fork) they would queue forever waiting for a runner
that isn't there.

Their jobs are gated on the repository variable `INSTANCE_PIPELINES == 'true'`.
It is set on the repo that owns the fleet and nowhere else. Elsewhere the jobs
skip. Moving the deployment to another repo means registering the runners there
and setting the variable there.

---

## Remaining work / follow-ups

- **Decommission the hosted Mend app** once the self-hosted cron is confirmed
  creating + auto-merging PRs, to avoid both engines managing the same branches.
  Then drop `renovate[bot]` from the `renovate_auto_approve.yaml` author gate and
  the Mend integration from ruleset #9449689's bypass actors.
- ~~**Install `recreate_runners.sh` on mimir as the scheduled User Script.**~~
  Done differently: the Ubuntu migration replaced the User Scripts plugin with
  systemd timers owned by the mimir repo, and both passes are enabled. Kept for
  the history — an earlier git-pull-based `rebuild_runners.sh` predated the
  CI-build + host-recreate design. `sync.sh` pushes the current script; formerly
  `schedule.json` needed a **daily** entry at
  the no-arg full pass (backstop) **and** a **short-interval** entry at
  `recreate_runners.sh --idle-only` (prompt pass, #2395), then remove the old
  `rebuild_runners.sh`.
- **Pin/track the remaining `@latest` tools baked in the runner image** —
  `protoc-gen-go` and `protoc-gen-connect-go` (both `go install @latest`),
  `firebase-tools@15`, and the `myoung34/github-runner` base image — none are
  Renovate-tracked today (they float at image-build time).
  `golangci-lint` and `gofumpt` are **done** (#1611): both are pinned in
  `versions.env` with `# renovate:` annotations, consumed as build args by
  `runners/Dockerfile`, guarded by `check_versions.js`'s `runnerBakedKeys`, and
  checked against the developer's local toolchain by `doctor_versions.js`. They
  were pinned first because `npm run lint:go` became a required check — an
  unpinned linter lets a runner rebuild turn a green branch red with no repo
  change, and a `gofumpt` bump can reformat files. The `protoc-gen-*` pair is
  lower risk: their output is regenerated on every build, so drift shows up as
  a diff rather than a mystery lint failure.
- **Pin/track the `@latest` tools in the Renovate image** — `protoc-gen-go`,
  `protoc-gen-connect-go`, and the pinned `buf` (`BUF_VERSION`); the
  `renovate/renovate` base tag is dockerfile-manager-tracked.
- **Verify `NDK_VERSION` matches `flutter.ndkVersion`** for the pinned Flutter.
  The Android build uses `flutter.ndkVersion`; if the baked NDK differs, the build
  downloads a different one and the doctor checks the wrong target. A Flutter bump
  can move it with nothing guarding the link.
- **Source the setup-go pin from `versions.env`.** `generate-proto-code` still
  pins `go-version` as a literal (Renovate keeps it fresh via the `go toolchain`
  group, but it's not `check_versions`-guarded); sourcing it like the Flutter
  workflows would remove the last unguarded Go literal.
- **Migrate cloud_functions eslint to flat config** (then wire `npm run lint` back
  into CI).
- **gitleaks → binary** to reach zero GitHub-hosted jobs (optional).
- **`needs runner rebuild` auto-merge is LIVE**
  ([#2395](https://github.com/Type2Fun-Foundation/ripls/issues/2395)) — shipped
  and verified end-to-end (June 2026): `validate_runner_candidate.yaml` +
  `scripts/runner_candidate_checks.sh` (§5) validate the new toolchain pre-merge,
  `Validate Runner Candidate` is a required check on ruleset #17328641, §4 routes
  these PRs through auto-merge, and `recreate_runners.sh --idle-only` rolls idle
  runners. Proven by flutter 3.44.1 + go 1.26.4 auto-merging + rebuilding the
  runner image; java/onnx bumps correctly blocked. **Remaining:** (1) ~~set the
  `--idle-only` schedule~~ — done, it is `ripls-runner-recreate-idle.timer`
  since the Ubuntu migration; (2) the §8 auto-fixer still
  skips `needs runner rebuild` PRs — repairing a red baked-tool bump is #2396's
  scope.

---

## Key references

- `versions.env`, `scripts/check_versions.js`, `scripts/doctor_versions.js`,
  `renovate.json`
- `.github/workflows/`: `check_versions`, `renovate_auto_approve` (name "Renovate
  PR Automation" — the §4 `approve` + `request-human-review` jobs),
  `claude_fix_renovate_ci` (§8 auto-fixer), `build_runner_image`,
  `build_renovate_image`, and the job-gated `test_*` / `release_build_android` /
  `test_ios_build`
- `renovate/`: `Dockerfile`, `config.js`, `mint_app_token.js`, `sync.sh`,
  `README.md`; plus `scripts/renovate_gomod_regen.sh`. The entrypoint that runs
  Renovate lives in the mimir repo, not here.
- `runners/`: `Dockerfile`, `docker-compose.yml`, `sync.sh`, `recreate_runners.sh`,
  `README.md`
- Rulesets: `gh api repos/Type2Fun-Foundation/ripls/rulesets` (#9449689 review,
  #17328641 required status checks)
- `docs/release/ios_build_server_setup.md` — the `tinker` Mac runner runbook
- `runners/README.md` — runner image build/recreate + mimir specifics
