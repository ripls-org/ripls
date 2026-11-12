# Issue #1093: GitHub Actions storage and CI performance

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1093
**Filed:** 2026-04-11 | **Last activity:** 2026-04-11
**Labels:** none | **Assignees:** maintainer
**Context docs:** none

## Problem Statement

The GitHub Actions storage budget hit 90% by the 11th day of the billing cycle.
The org is on the GitHub Team plan (2 GB included storage). Beyond the immediate
storage problem, there are opportunities to improve CI performance by better
leveraging the self-hosted runner infrastructure.

### Storage breakdown

| Category | Size | Count | Notes |
|---|---|---|---|
| Actions artifacts | 1.86 GB | ~315 non-expired | Android AABs + dockerbuild stubs |
| Release assets | 1.17 GB | 14 releases with AABs | Persist indefinitely |
| Git repo | 236 MB | — | Normal |
| **Total billable** | **~3.3 GB** | — | **165% of 2 GB included** |

The 4 GB of Actions caches (Go modules, Docker buildkit layers) use a separate
10 GB per-repo budget and do NOT count toward billing.

Root cause: every release stores an ~90 MB Android AAB **twice** (workflow
artifact with 90-day retention + permanent release asset), and neither copy is
needed since Fastlane uploads directly to Play Store and TestFlight.

### Current runner landscape

| Workflow | Runner | Duration | Notes |
|---|---|---|---|
| test_go | self-hosted-linux | ~6 min | Downloads ONNX Runtime every run |
| test_flutter | **ubuntu-latest** | ~10 min | Only workflow still on GitHub-hosted |
| test_ios_build | self-hosted-mac | ~8 min | CocoaPods cache via actions/cache |
| release_build_android | self-hosted-linux | ~11 min | Downloads Go, Flutter, Java, Android SDK, NDK every run |
| build_and_deploy_container | **ubuntu-latest** | ~5 min | Docker build, needs GAR auth |
| test_docker_build | **ubuntu-latest** | ~3 min | PR-only Docker build test |
| test_dev_e2e | **ubuntu-latest** | ~3 min | Lightweight, just Go + proto gen |

### Caching anti-pattern on self-hosted runners

`actions/cache` and `actions/setup-*` with `cache: true` are designed for
ephemeral GitHub-hosted runners. On persistent self-hosted runners they're
counterproductive:

- **Upload**: After every build, GBs of cache data are uploaded to GitHub's
  servers over the network
- **Download**: Before every build, those same GBs are downloaded back
- **Pointless**: The data already exists on the runner's local filesystem between
  runs

Current cache entries being round-tripped through GitHub on self-hosted runners:
- `setup-go-macOS-arm64` — **2.09 GB** (Go modules + build cache, Mac runner)
- `setup-go-Linux-x64-ubuntu24` — **479 MB** (Go modules + build cache, Linux runners)

For Docker-based Linux runners, the issue is that `/tmp/runner/work` doesn't
persist between container restarts. But the fix is persistent volumes, not
network round-trips through GitHub.

## Implementation Plan

### Phase 1: Stop uploading build artifacts

**Goal:** Prevent new releases from consuming storage.

**Tasks:**
- [ ] Remove the `Upload AAB artifact` step from `release_app.yaml` (lines 307-311)
- [ ] Remove the `Upload IPA artifact` step from `release_app.yaml` (lines 179-184)
- [ ] Remove the `Download all artifacts` step from the `create-release` job (lines 335-339)
- [ ] Remove the `List available artifacts` step (lines 341-343)
- [ ] Remove the `files:` section from `softprops/action-gh-release` (lines 352-356)
  and remove `fail_on_unmatched_files` since there are no files to match
- [ ] Keep the `create-release` job itself — it still creates the GitHub Release
  with auto-generated release notes, just without binary attachments

**Validates:** New releases create a GitHub Release (with notes) but store zero
artifacts.

### Phase 2: Delete existing artifacts and release assets

**Goal:** Reclaim the ~3 GB already consumed.

**Tasks:**
- [ ] Delete all `android-aab-*` workflow artifacts (22 files, 1.84 GB)
- [ ] Delete all `ios-ipa-*` workflow artifacts (if any exist)
- [ ] Delete all `*.dockerbuild` workflow artifacts (291 files, 19 MB)
- [ ] Remove AAB assets from all past GitHub releases (14 releases, 1.17 GB)
- [ ] Verify storage usage drops well within the 2 GB limit

**Validates:** Billable storage drops to ~236 MB (repo only).

### Phase 3: Build a custom runner image with tools pre-installed

**Goal:** Eliminate per-run tool installation overhead.

Right now every run of `test_go`, `release_build_android`, and other workflows
on the self-hosted Linux runners spends minutes downloading and installing Go,
Node.js, Flutter, Android SDK/NDK, JDK, ONNX Runtime, protoc plugins, and npm
dependencies. Since the runners are persistent Docker containers, these tools
should be baked into the image.

**Tasks:**
- [ ] Create a `Dockerfile.runner` (in a new `runner/` directory) based on
  `myoung34/github-runner:ubuntu-noble` that pre-installs:
  - Go 1.25.8
  - Node.js 22 + npm
  - Flutter 3.38.3 (linux)
  - JDK 17 (Temurin)
  - Android SDK (command-line tools, build-tools, platform-tools)
  - Android NDK (version matching Flutter's requirement)
  - ONNX Runtime 1.20.1
  - protoc-gen-go, protoc-gen-connect-go
  - buf CLI
  - Git LFS
  - Ruby + Bundler (for Fastlane on the Android release job)
  - golangci-lint
- [ ] Add bind-mount volumes in Unraid's appdata directory for caches that
  accumulate at runtime. Unraid convention is
  `/mnt/user/appdata/<container>/` on the cache-backed user share, which
  gives SSD speed and is picked up by the Appdata Backup plugin
  automatically. Map each container path to its own appdata subdirectory:
  - Go module cache: `/home/runner/go/pkg/mod`
  - Go build cache: `/home/runner/.cache/go-build`
  - Flutter pub cache: `/home/runner/.pub-cache`
  - Gradle caches: `/home/runner/.gradle`
  - npm cache: `/home/runner/.npm`
- [ ] Use a shared `_work` volume per runner so checkout and build artifacts
  survive container restarts (avoids re-cloning the repo after Unraid
  reboots or Docker engine restarts)
- [ ] Update `docker-compose.yaml` to use the custom image and bind-mount
  volumes
- [ ] Update workflows to skip `actions/setup-go`, `actions/setup-node`,
  `subosito/flutter-action`, `actions/setup-java`, `android-actions/setup-android`,
  and `Install ONNX Runtime` steps when running on `self-hosted-linux`
  (the tools are already in PATH)

**Example docker-compose.yaml:**

Uses Unraid's `/mnt/user/appdata/github-runner/` as the base path. This
lands on the cache pool (SSD) for fast I/O, and the Appdata Backup plugin
will automatically include it in scheduled backups.

```yaml
x-runner-common: &runner-common
  build:
    context: ./runner
    dockerfile: Dockerfile.runner
  restart: unless-stopped
  devices:
    - "/dev/kvm:/dev/kvm"
  environment: &runner-env
    REPO_URL: https://github.com/Type2Fun-Foundation/ripls
    ACCESS_TOKEN: ${ACCESS_TOKEN}
    LABELS: unraid,self-hosted-linux,linux

services:
  github-runner-1:
    <<: *runner-common
    environment:
      <<: *runner-env
      RUNNER_NAME: unraid-runner-1
      RUNNER_WORKDIR: /tmp/runner/work
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /mnt/user/appdata/github-runner/runner-1/work:/tmp/runner/work
      - /mnt/user/appdata/github-runner/runner-1/go-mod:/home/runner/go/pkg/mod
      - /mnt/user/appdata/github-runner/runner-1/go-cache:/home/runner/.cache/go-build
      - /mnt/user/appdata/github-runner/runner-1/pub-cache:/home/runner/.pub-cache
      - /mnt/user/appdata/github-runner/runner-1/gradle:/home/runner/.gradle
      - /mnt/user/appdata/github-runner/runner-1/npm:/home/runner/.npm

  github-runner-2:
    <<: *runner-common
    environment:
      <<: *runner-env
      RUNNER_NAME: unraid-runner-2
      RUNNER_WORKDIR: /tmp/runner/work
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /mnt/user/appdata/github-runner/runner-2/work:/tmp/runner/work
      - /mnt/user/appdata/github-runner/runner-2/go-mod:/home/runner/go/pkg/mod
      - /mnt/user/appdata/github-runner/runner-2/go-cache:/home/runner/.cache/go-build
      - /mnt/user/appdata/github-runner/runner-2/pub-cache:/home/runner/.pub-cache
      - /mnt/user/appdata/github-runner/runner-2/gradle:/home/runner/.gradle
      - /mnt/user/appdata/github-runner/runner-2/npm:/home/runner/.npm

  github-runner-3:
    <<: *runner-common
    environment:
      <<: *runner-env
      RUNNER_NAME: unraid-runner-3
      RUNNER_WORKDIR: /tmp/runner/work
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /mnt/user/appdata/github-runner/runner-3/work:/tmp/runner/work
      - /mnt/user/appdata/github-runner/runner-3/go-mod:/home/runner/go/pkg/mod
      - /mnt/user/appdata/github-runner/runner-3/go-cache:/home/runner/.cache/go-build
      - /mnt/user/appdata/github-runner/runner-3/pub-cache:/home/runner/.pub-cache
      - /mnt/user/appdata/github-runner/runner-3/gradle:/home/runner/.gradle
      - /mnt/user/appdata/github-runner/runner-3/npm:/home/runner/.npm

  github-runner-4:
    <<: *runner-common
    environment:
      <<: *runner-env
      RUNNER_NAME: unraid-runner-4
      RUNNER_WORKDIR: /tmp/runner/work
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /mnt/user/appdata/github-runner/runner-4/work:/tmp/runner/work
      - /mnt/user/appdata/github-runner/runner-4/go-mod:/home/runner/go/pkg/mod
      - /mnt/user/appdata/github-runner/runner-4/go-cache:/home/runner/.cache/go-build
      - /mnt/user/appdata/github-runner/runner-4/pub-cache:/home/runner/.pub-cache
      - /mnt/user/appdata/github-runner/runner-4/gradle:/home/runner/.gradle
      - /mnt/user/appdata/github-runner/runner-4/npm:/home/runner/.npm
```

**Why per-runner directories instead of shared volumes:** GitHub Actions runners
can execute concurrent jobs. Sharing a single Go build cache or work directory
between runners would cause file contention and cache corruption. Each runner
gets its own isolated tree under `/mnt/user/appdata/github-runner/runner-N/`.

**Unraid notes:**
- Ensure the `appdata` share is configured with "Prefer: Cache" (the default)
  so these bind mounts land on the SSD pool, not spinning disks
- The Appdata Backup plugin will pick up
  `/mnt/user/appdata/github-runner/` automatically
- Total cache size per runner is ~3-4 GB (Go module+build ~2.5 GB, Gradle
  ~500 MB, pub+npm ~200 MB), so ~16 GB across all four runners

**Validates:** Workflows skip tool installation steps. Go tests drop from ~6 min
to ~3-4 min. Android builds drop from ~11 min to ~5-6 min.

### Phase 4: Move Flutter tests to self-hosted Linux

**Goal:** Stop paying for GitHub-hosted runner minutes for Flutter tests and
benefit from pre-installed tools.

`test_flutter.yaml` is the only remaining test workflow on `ubuntu-latest`. It
currently takes ~10 min, most of which is installing Go, Node, Flutter, and npm
dependencies.

**Tasks:**
- [ ] Change `runs-on: ubuntu-latest` to `runs-on: self-hosted-linux` in
  `test_flutter.yaml`
- [ ] Remove the `Clean Flutter pub cache` step (the persistent volume handles
  cache freshness; nuking it every run defeats the purpose)
- [ ] Remove `cache: true` from `subosito/flutter-action` step (unnecessary
  with pre-installed Flutter and persistent pub cache volume)
- [ ] If Phase 3 custom image is done: remove `actions/setup-go`,
  `actions/setup-node`, and `subosito/flutter-action` steps entirely since
  tools are pre-installed
- [ ] Verify tests pass on self-hosted runner

**Validates:** Flutter tests run in ~3-4 min instead of ~10 min. Zero
GitHub-hosted minutes consumed for test workflows.

### Phase 5: Remove actions/cache from self-hosted iOS workflows

**Goal:** Stop round-tripping cache data through GitHub for the self-hosted Mac.

The self-hosted Mac runner currently uses `actions/cache` for CocoaPods and
`actions/setup-go` with `cache: true` (via the generate-proto-code action).
This uploads a 2 GB Go cache to GitHub after every run and downloads it before
the next — pointless since the Mac runner is persistent.

**Tasks:**
- [ ] Remove the `Cache CocoaPods` step and `Clean stale Pods on cache miss`
  from `test_ios_build.yaml` and `release_app.yaml` (iOS build job). CocoaPods
  are installed by `pod install` and persist on the Mac filesystem between runs
  naturally. A `Podfile.lock` change triggers a fresh `pod install` regardless.
- [ ] In the `generate-proto-code` composite action, change `cache: true` to
  `cache: false` on `actions/setup-go` — or better, make the cache option
  conditional on runner type. Since the action is shared across GitHub-hosted
  and self-hosted workflows, the simplest approach is to disable Go caching
  in the action (it's a few hundred MB savings on GitHub-hosted vs eliminating
  a 2 GB round-trip on self-hosted).
- [ ] Optionally: remove the `setup-go-macOS-arm64` and
  `setup-go-Linux-x64-ubuntu24` cache entries from GitHub to reclaim 2.5 GB
  of (non-billing) cache space

**Validates:** iOS builds and Go tests on self-hosted runners no longer upload
or download multi-GB caches. Build times may slightly improve due to skipping
the upload step.

### Phase 6: Optimize Docker cache (optional, non-billing)

**Goal:** Reduce Docker buildkit cache footprint.

The `build_and_deploy_container` and `test_docker_build` workflows use
`cache-to: type=gha,mode=max` which stores ALL intermediate layers. These
workflows still run on `ubuntu-latest` (GitHub-hosted), so `actions/cache` makes
sense here — but `mode=max` is unnecessarily aggressive.

**Tasks:**
- [ ] In `build_and_deploy_container.yaml`, change `cache-to: type=gha,mode=max`
  to `cache-to: type=gha,mode=min`
- [ ] In `test_docker_build.yaml`, make the same change
- [ ] Monitor build times to ensure no regression

**Validates:** Docker buildkit cache shrinks from ~1.5 GB without significantly
impacting build times.

### Phase 7: Move Docker builds and e2e tests to self-hosted (optional)

**Goal:** Eliminate remaining GitHub-hosted runner usage.

The remaining GitHub-hosted workflows are:
- `build_and_deploy_container` — Docker build + push to GAR
- `test_docker_build` — PR Docker build test
- `test_dev_e2e` — Lightweight Go e2e tests
- `publish_website` — Hugo static site build
- `claude.yaml` — Claude Code agent (keep on GitHub-hosted)
- `test_terraform` — Terraform validation (keep on GitHub-hosted)

**Tasks:**
- [ ] Move `build_and_deploy_container` to `self-hosted-linux` — Docker is
  already available via the mounted socket. Eliminates the need for buildkit
  GHA cache entirely (use local Docker layer cache instead). Requires GAR
  auth credentials on the runner.
- [ ] Move `test_docker_build` to `self-hosted-linux` for the same reason
- [ ] Move `test_dev_e2e` to `self-hosted-linux` — just needs Go (already
  installed in Phase 3)
- [ ] Keep `publish_website`, `claude.yaml`, and `test_terraform` on
  `ubuntu-latest` (lightweight, infrequent, not worth optimizing)

**Validates:** GitHub-hosted runner minutes drop to near-zero. Docker builds
use local layer cache instead of GHA cache, eliminating the 1.5 GB buildkit
cache entirely.

## Summary of expected improvements

| Metric | Before | After |
|---|---|---|
| Billable storage | 3.3 GB (165% of limit) | ~236 MB (12% of limit) |
| Flutter test time | ~10 min | ~3-4 min |
| Go test time | ~6 min | ~3-4 min |
| Android release build | ~11 min | ~5-6 min |
| GitHub-hosted minutes | All test+build workflows | Only website, terraform, claude |
| Cache round-trips | 2.5 GB up+down per self-hosted run | Zero |

## Related Issues

None identified.
