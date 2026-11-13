# Workflow audit for the Forgejo port (#3293)

**Date:** 2026-11-13 | **Plan:** [`3293-forgejo-port.md`](3293-forgejo-port.md)

Every workflow, job and step that runs on GitHub today, checked against what
changes around it:

- **the forge** (Forgejo 17: see the spike results in the plan);
- **the runners** (a fresh container from the toolchain image per job, no
  hosted runners, shared host Docker daemon);
- **hosting** (servers and Postgres left GCP in September; Cloud Run, Cloud SQL
  and the image registry were deleted on 2026-11-11);
- **one public repo.**

Method: all 41 workflows, the composite actions and the scripts they call were
read step by step, along with the agent repo. The last 100 runs of each
workflow on GitHub were pulled for context. Findings below that say *verify*
rest on reading, not on a run.

## Summary

- **Nothing was found that should simply be deleted today.** No workflow is
  dead. The GCP steps that look left over from Cloud Run are still needed:
  they read Secret Manager.
- **Two workflows prove less than they claim, on GitHub, today:**
  - Go Tests never runs the Twilio integration test on the fleet. The step
    fetches its credentials with `gcloud`, which the runner image doesn't
    have, and hides the error. The test self-skips on every run.
  - `test_dev_e2e` ignores the server URL its caller passes.
- **The shadow CI on Forgejo is less green than it looks.** Web E2E reports
  success there with its real job skipped, and Go Tests runs degraded, because
  the dev GCP key isn't on Forgejo yet.
- **The agent flow is the biggest port and the least used.** Plan, refine,
  implement, address and fix-ci have not run once through the dispatch bridge
  on this repo. Only fix-renovate, release-cut and the nightly doc refresh are
  live. Decided: port all of it (decision 1).
- **One agent loop is burning runs now:** fix-renovate retries the same
  blocked Renovate PR about three times a day (decision 2).

## Verdicts

| Workflow | Verdict | Why |
| --- | --- | --- |
| `check_context_map`, `check_design_tokens`, `check_versions` | keep as is | Pass on Forgejo unchanged. |
| `gitleaks-pr`, `gitleaks-push` | keep | Pass on Forgejo. Comments about staying on hosted runners no longer hold. |
| `lint_workflows` | port needed | actionlint checks GitHub's schema and will reject correct Forgejo spellings. Every path breaks when workflows move to `.forgejo/`. |
| `test_go` | keep with changes | Twilio fetch (A1), two latent pipefail lines (C2), artifact upload (B5). |
| `test_flutter` | keep with changes | Passes on Forgejo. One leftover step to drop at the flip. |
| `test_flutter_web` | keep as is | Passes on Forgejo. |
| `test_web_e2e` | keep with changes | Skipped on Forgejo until the GCP key is there. Re-downloads Playwright, the auth emulator and gcloud on every job (D2). |
| `test_docker_build` | keep with changes | Passes, but its build cache is dead in a fresh container (D1). |
| `test_terraform` | keep as is | Passes on Forgejo. |
| `test_cloud_functions` | keep with changes | Redundant Node setup. Not yet run on Forgejo. |
| `test_ai_integration` | keep with changes | A no-op cleanup step to remove. Needs the GCP key. |
| `test_dev_e2e` | keep with changes | Input bug (A2). Instance-only. |
| `test_ios_build` | port needed | No Mac runner on Forgejo yet. |
| `verify_toolchain` | drop at cut-over | Proves nothing on Forgejo: `ubuntu-latest` is now the toolchain image (B6, decision 3). |
| `check_oss_export` | drop at cut-over | Planned. Its tree gate and denylist load must move to a workflow that ships. |
| `build_and_deploy_container` | keep with changes | All 18 steps still do something. Dead cache, a prod-ref risk (B7), leftovers. |
| `deploy_to_dev_on_push`, `deploy_manual` | keep | Work as they are once the instance variable is set. |
| `deploy_cloud_functions` | port needed | OIDC into GCP, and a failure alert written with `gh` (B3). |
| `deploy_staging_website`, `cleanup_staging_website` | port one step | The sticky PR comment action calls GitHub's API. |
| `publish_website` | keep as is | Nothing forge-specific. |
| `release_app` | port needed | Release creation, the Mac job, one pipefail line. |
| `release_finalize` | port needed | `gh workflow run`, and a double-trigger hazard (B4). |
| `release_cut` | port needed | Goes through the agent bridge. |
| `release_build_android` | keep with changes | Works. Its failures are real: stuck Renovate PRs. |
| `build_runner_image` | keep with changes | Builds fine. On Forgejo a new image is live for the next job, with no rollout step in between. |
| `build_renovate_image` | keep as is | The workflow is forge-neutral. The image's contents are not. |
| `validate_runner_candidate` | port needed | Gated on a `[bot]` login that won't exist; comments through `gh`. |
| `renovate_auto_approve` | port needed | Approve action, `[bot]` logins, reviewer and auto-merge through `gh`. |
| `auto_assign_author` | keep with changes | `user.type` and one `gh` call. |
| `claude_fix_renovate_ci` | port needed (redesign) | The one live dispatcher. Loses `workflow_run`. |
| `claude_resolve_conflict` | port needed | `gh` and GitHub's three-state `mergeable`. The hourly cron's reason is GitHub-specific. |
| `claude_plan_issue`, `claude_refine_plan`, `claude_implement_plan`, `claude_address_pr_feedback`, `claude_fix_ci_failure` | port needed | Unused through the bridge, ported anyway (decision 1). |

## A. Wrong on GitHub today

1. **The Twilio integration test never runs.** `test_go.yaml` fetches its
   test credentials with `scripts/fetch_secret.sh`, which calls `gcloud`. The
   runner image has no `gcloud` (checked in the image), the step has no
   `setup-gcloud`, and `2>/dev/null || true` hides the failure. The latest
   green run on `main` shows the test skipping with "set
   TWILIO_TEST_ACCOUNT_SID…". The comment above the step says the auth action
   "already configured gcloud"; it only writes a credentials file.
   - Fix: bake `google-cloud-cli` into the runner image and stop discarding
     stderr. Baking it also removes a per-job SDK download from two other
     workflows on Forgejo.
2. **`test_dev_e2e` ignores its caller's `server_url`.** Line 57 reads
   `github.event.inputs.server_url`, which a `workflow_call` doesn't fill. The
   default happens to equal what the caller passes, so nothing shows.
   - Fix: `inputs.server_url`.
3. **Two workflows put a Ruby 3.4 gem path on `PATH`** (`test_ios_build:128`,
   `release_app:161`) while `versions.env` pins Ruby 4.0.3. The path is
   probably dead. *Verify on the Mac.*
4. **`renovate_auto_approve` still matches `renovate[bot]`.** Every recent
   Renovate PR comes from the self-hosted bot; the hosted app is gone. The
   doc already says to remove it.
5. **Leftovers from Cloud Run** in the deploy path: `packages: write` and
   `id-token: write` permissions that nothing uses, a `deploy` input every
   caller sets to true, hardcoded project ids next to repo variables that
   hold the same values, and `environment: prod` on a GitHub environment with
   no rules and no secrets.

## B. Breaks or misleads on Forgejo

1. **Anything through `gh` or a GitHub App.** The agent bridge (three token
   mints, three `gh workflow run`), `renovate_auto_approve`,
   `auto_assign_author`, `validate_runner_candidate`,
   `deploy_cloud_functions`'s alert, `release_finalize`, and in the agent
   repo 19 token mints and about 100 `gh` references. None use GraphQL
   directly. A few have no single REST call behind them: failed-job logs,
   the combined check status, draft PRs (a title prefix on Forgejo) and
   search qualifiers.
2. **Event fields the dispatchers read:**
   - `author_association` is gone. The trust check can't stay an `if:`
     expression; it becomes a step with a token allowed to read permissions.
   - The self-trigger guard tests `ripls-agent[bot]`; the Forgejo login is
     `ripls-agent`. Once trust is fixed, the agent's own "comment `@claude`
     to change this" text would re-trigger it in a loop.
   - `action == 'labeled'` is `label_updated`. As written, the plan
     dispatcher's *other* branch would then match any label change on an
     issue that mentions `@claude`.
   - `review.state` is `review.type`. *Verify:* whether the review payload
     has `body` and `id`; the agent's validation needs the id.
   - `user.type == 'User'` probably doesn't exist. *Verify.*
3. **`deploy_cloud_functions` fails twice over.** Its OIDC login needs
   `enable-openid-connect` and a second identity provider in GCP for the
   Forgejo issuer. When it fails, the step that files an issue about the
   failure is written with `gh` and fails too, which is the silent outage
   that step exists to prevent. Port the alert before the first Forgejo
   deploy. The planned fallback, a service-account key, is forbidden by an
   org policy the workflow's own header cites.
4. **`release_finalize` would start two release builds.** It pushes the tag
   and then dispatches `release_app`, because on GitHub a tag pushed with the
   workflow token triggers nothing. On Forgejo it does. Two runs in one
   concurrency group with `cancel-in-progress` means one cancels the other,
   possibly mid store upload. Run the dispatch on GitHub only, and delete it
   at the flip.
5. **`actions/upload-artifact@v7`** (six places, plus the agent repo). Only
   v3 and Forgejo's v4 fork were proven. All six run on failure only, so the
   cost is lost debug output on a job that is already red. *Verify v7.*
6. **`verify_toolchain` and the "hosted runner" comments.** Its job is to
   prove the toolchain installs on a machine with nothing on it. On Forgejo
   `ubuntu-latest` is the toolchain image, so both passes skip and the check
   passes without installing anything. Give it a bare `container:`, or drop
   it if installing from nothing is no longer a supported case. The same
   stale reasoning ("kept on a hosted runner so a fleet outage doesn't stop
   it") sits in the gitleaks workflows and seven dispatchers.
7. **The prod build may come from the wrong commit.** `release_finalize`
   runs on `pull_request: closed` and builds the default checkout. On GitHub
   that is `main`. On Forgejo a pull-request event checks out the PR head.
   *Verify.* Passing the merge commit explicitly is right on both.
8. **`validate_runner_candidate` skips and reports green** for every PR once
   Renovate's login changes, without validating anything. Put the bot login
   in a variable.
9. **The dispatch credential is weaker than its comment says.** On GitHub the
   bridge's App can only start workflows in the agent repo. A Forgejo token
   is scoped by API area: one that can dispatch there can also push a branch
   there, with a changed workflow, and dispatch on it. Use a separate
   dispatch-only account, on a team with Actions access and no code access.
   *Verify that the dispatch endpoint accepts that.*
10. **actionlint will reject correct Forgejo workflows**
    (`enable-openid-connect`, `label_updated`, URL-form `uses:`). Add targeted
    ignores for the two-forge period.
11. **Moving to `.forgejo/workflows/`** breaks every workflow that lists its
    own path in `paths:`, the reusable-workflow `uses:` lines, and
    `lint_workflows` (which would pass with nothing to lint).

## C. Default shell: `pipefail`

Forgejo runs `run:` steps with `pipefail`; GitHub's default shell does not.
Scripts called from a step are separate shells and unaffected.

1. Fixed: the four allowlist ratchets in `test_flutter` (#3300).
2. Latent, `test_go.yaml:290-292`: both coverage-allowlist pipelines fail the
   day the allowlist becomes empty, which is the goal of the ratchet.
3. Unlikely but release-blocking, `release_app.yaml:57`:
   `git branch -r --contains … | grep -q origin/main` can fail with SIGPIPE.
   Use `git merge-base --is-ancestor`.

Every other inline pipeline in the 41 workflows was checked and is safe.

## D. Fresh containers

1. **Docker layer caches are dead.** `test_docker_build` and
   `build_and_deploy_container` cache to `/tmp/docker-cache` inside the job
   container, so every build is cold and still pays for the export. Deploys
   are already about 20 minutes. Point the cache at a mounted shared
   directory.
2. **Downloads on every job:** the gcloud SDK (two workflows), Playwright's
   browsers and system packages, the Firebase auth emulator, Node via
   `setup-node` in three workflows although the image has it, Ruby gems in
   the Android release. Bake the first three into the image.
3. **Still needed:** removing test containers from the host daemon
   (`cleanup-test-postgres` and its siblings). A fresh job container doesn't
   reap what it started on the host. `setup-toolchain` is also still needed:
   it is what lets a version-bump PR test the new toolchain before the image
   is rebuilt.
4. **Not needed after the flip, harmless until then:** the "fix git
   ownership" step in about 20 workflows, deploy-key removal, the
   "restore flutter_tester" step, and the agent repo's workspace hiding and
   credential stripping.
5. **Leaks to check.** *Verify:* whether `if: always()` cleanup steps run
   when Forgejo cancels a job. If not, every superseded run leaves a
   container. The hourly orphan sweep only matches the Go test Postgres; it
   misses the web e2e Postgres and the compose stack from
   `test_docker_build`. The sweep script is also deployed only by the GitHub
   fleet's `sync.sh`.
6. **A bad runner image now breaks everything at once.** On GitHub a rebuilt
   image reaches the fleet through a separate recreate pass. On Forgejo the
   next job uses it, including the job that would rebuild it. Tagging the
   previous image before each build would make rollback one command.

## E. Run history

| Workflow | Last 100 runs | Reading |
| --- | --- | --- |
| Five agent dispatchers | 100 skipped each | Normal shape: they fire on every comment, review or CI run and match nothing. Variables are set. |
| `claude_fix_renovate_ci` | 94 skipped, 3 ok, 3 cancelled | Live. All six real runs are one PR (decision 2). |
| `deploy_cloud_functions` | 17 of 60 failed | Closed: nine during bring-up in May, eight from a Node 24 break in June. 31 straight passes since. |
| `release_app` | 31 failed | Historical: 26 are bring-up dispatches from last December. The last 24 runs are green. |
| `release_build_android` | 20 failed | Real: 19 are Renovate PRs that can't build, re-run on each daily rebase. |

A skipped job costs nothing on Forgejo either: on the first shadow push, with
no runner registered, the two gated workflows resolved to "skipped" while the
rest waited.

## Decisions (owner, 2026-11-13)

The audit ended with eight questions. The answers:

1. **Agent scope: port everything.** All nine dispatchers and every agent
   mode move before the cut-over, used recently or not.
2. **Fix-renovate loop: cap per PR, and close the connectrpc PR.** The agent
   stops after three attempts on a PR until a human comments.
3. **`verify_toolchain`: drop it at the cut-over.** Installing from nothing
   stops being a tested case. It keeps running on GitHub until then.
4. **`claude_resolve_conflict`: drop the hourly cron on Forgejo.** Push and
   pull-request triggers stay.
5. **Secret scanning: the pre-receive hook replaces `gitleaks-push`** at the
   cut-over. `gitleaks-pr` stays as the review gate.
6. **GCP login for Cloud Functions: add an identity provider for the Forgejo
   issuer.** No service-account key. The Terraform is written here and
   applied only with the owner's go-ahead.
7. **Deploy and release workflows wait for the phase 3 rehearsal.** They stay
   off in shadow, so nothing deploys twice.
8. **GitHub-side bugs: fix `test_dev_e2e`'s `server_url` now; leave the
   Twilio test skipping** until the owner decides whether it should call
   Twilio's test API on every run. Baking gcloud into the image is still
   wanted for the per-job download (D2), separately.

## Still to verify by running

- A pull request on the shadow repo: the detect jobs, `paths-filter` on a PR,
  whether a skipped job satisfies a required check, and the checkout ref on
  `pull_request: closed`.
- `upload-artifact@v7`; whether `if: always()` steps run on cancel; the
  expression in `gitleaks-push`'s `cancel-in-progress`.
- Review and label payload fields (B2).
- `validate_runner_candidate`'s `--volumes-from "$(hostname)"`.
- With the dev GCP key on Forgejo: Web E2E, and Go Tests undegraded.

## Corrections to the plan

- `environment:` scoping is used by one job, not five workflows, and protects
  nothing: the one GitHub environment has no rules and no secrets. The
  accepted trade-off in the plan costs nothing.
- `release_finalize` does not use OIDC. Only `deploy_cloud_functions` does.
