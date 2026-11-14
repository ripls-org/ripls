# Self-hosted Renovate (`renovate/`)

Renovate runs **self-hosted on `mimir`** (the Ubuntu CI host — see
the host runbook, kept privately) as a scheduled batch job,
replacing the hosted Mend app. It is outbound-only: it clones the repo, computes
dependency updates, pushes branches, and opens/merges PRs via the forge's API. No
inbound networking, no web UI (the in-repo **Dependency Dashboard issue** is the
inspection surface), no reverse proxy.

The repo-level rules live in [`renovate.json`](../renovate.json) at the repo root
(managers, package rules, custom managers, the gomod `postUpgradeTasks`). This
directory holds the **self-hosting** pieces that can't live in `renovate.json`.

## Why self-hosted

Generated Go code (`server/gen/**`) is not checked in. Renovate's built-in
go.sum refresh runs `go mod tidy` / `go get`, which cannot resolve the in-module
imports of the generated packages and fails — leaving `go.mod`-only PRs with a
stale `go.sum` that breaks every Go build. Self-hosting lets a `postUpgradeTask`
regenerate the Go proto packages first (see
[`scripts/renovate_gomod_regen.sh`](../scripts/renovate_gomod_regen.sh)), which
the hosted Mend app cannot do. Self-hosting also gives full control over the
toolchain-mirror grouping in `renovate.json`.

## Key files

| File | Purpose |
| --- | --- |
| `Dockerfile` | `ripls-renovate:latest` — `renovate/renovate` + Go (from `versions.env`) + `buf` + `protoc-gen-go`/`protoc-gen-connect-go`, just enough to run the Go go.sum regen. No Flutter/Dart/Java. |
| `config.js` | Self-hosted **admin** config: repo list, `binarySource`, the `allowedCommands` allowlist that authorizes `postUpgradeTasks`, git author. Secrets (the GitHub App credentials) come from the host-side env, never committed. |
| — | The entrypoint that fires Renovate is **not in this repo**. It is `/opt/nas/scripts/run-renovate.sh`, owned by the mimir repo and run by the `run-renovate` systemd timer: `docker run --rm` the image off-peak, memory-capped, `flock`-guarded against overlap. This repo shipped its own `run_renovate.sh` until the Ubuntu migration, when the two copies diverged and the local one was deleted. |

## Build → run pipeline (mirrors the runner image)

1. **Build (CI):** [`.github/workflows/build_renovate_image.yaml`](../.github/workflows/build_renovate_image.yaml)
   runs on `self-hosted-linux` on merges to `main` touching `versions.env` /
   `renovate/Dockerfile` / the workflow. Because the runner mounts the host
   Docker socket, `docker build` tags `ripls-renovate:latest` **directly in
   mimir's daemon** — no registry, no push.
2. **Run (systemd timer):** `run-renovate.service` on mimir does
   `docker run --rm ripls-renovate:latest`. There is **no recreate step** (unlike
   the runner fleet): each fire is a fresh container, so it always uses the
   newest image. `update-docker-stacks` does not touch it — it is not a compose
   stack.

The Renovate version is pinned by the `FROM` tag in `Dockerfile` and bumped by
the dockerfile manager (manual, like other base images). `GO_VERSION` is a
build-arg from `versions.env`; `GOTOOLCHAIN=auto` then fetches whatever `go.mod`
pins at run time.

## Auth

Renovate works as the **`ripls-renovate` account** on the Forgejo instance the
repository lives on (#3293). The account is a collaborator on the repository
with write access and nothing else. Its access token is `RENOVATE_TOKEN` in a
host-side file under `/etc/nas/` on mimir (never committed, and deliberately
outside the backed-up app directory), read by `run-renovate.sh`. The account is
the author of Renovate's pull requests; workflows recognise it by the
repository variable `RENOVATE_BOT_LOGIN`.

The org-owned **GitHub App** of the same name, which authenticated Renovate
while the repository was on GitHub, is still used for one thing:
`mint_app_token.js` mints a short-lived token from it that Renovate uses to
look up releases and changelogs of dependencies hosted on github.com
(`GITHUB_COM_TOKEN`). It cannot write anywhere Renovate works, and Renovate
runs without it, more slowly. See
[`docs/infra/dependency_automation.md`](../docs/infra/dependency_automation.md).

## Inspecting / operating

- **What's pending/blocked/errored:** the Dependency Dashboard issue in the repo.
- **Run history / logs:** `ssh root@mimir 'journalctl -u run-renovate -n 200'`,
  plus the tee'd log under the app directory.
- **Reproduce extraction / config problems locally:**
  `npx renovate@latest --platform=github --token="$(gh auth token)" --dry-run=full --onboarding=false Type2Fun-Foundation/ripls`.
- **Validate config after editing `renovate.json`:**
  `npx --yes --package renovate@latest -- renovate-config-validator renovate.json`.
