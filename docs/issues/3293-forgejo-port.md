# Issue #3293: [OSS-18] Port CI, the agent and the feedback tracker to Forgejo

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/3293
**Filed:** 2026-11-12 | **Last activity:** 2026-11-12
**Labels:** open-source | **Assignees:** unassigned
**Context docs:** `docs/issues/3228-single-public-repo.md` (the cut-over this
port unblocks), `docs/secrets.md`, `docs/infra/dependency_automation.md`,
`runners/README.md`, `docs/release/automation.md`, the Forgejo v17 Actions
reference, and the mimir repo's `stacks/forgejo`. Side anchors (`client/*`,
`server/*`) not loaded: apart from the feedback client, this issue changes CI
and tooling, not product code.

## Problem Statement

#3228 ends with one public repo that development, deploy and release all run
from. On 2026-11-12 the destination was decided: the self-hosted Forgejo
instance at `code.overt.org`, with `ripls-org/ripls` on GitHub kept as a push
mirror. The private tracker for user feedback and crash alerts goes to Forgejo
too.

Nothing in this repo runs on Forgejo today. Actions are disabled on the
instance, and four kinds of thing assume GitHub:

| What | Size | Why it doesn't carry over |
| --- | --- | --- |
| Workflows in `.github/workflows/` | 41 files | Most are plain test and build jobs. About a dozen use something Forgejo lacks or spells differently (below). |
| The agent (`ripls-agent`) | ~100 `gh` calls | `gh` only talks to GitHub. Its dispatchers also depend on `workflow_run`, `author_association` and a GitHub App. |
| `gh` in this repo | ~30 calls in workflows and scripts, more in four Claude skills | Same. |
| Feedback filing | `server/github/client.go`, `cloud_functions/` | GitHub API clients with a GitHub App for auth. |

What Forgejo v17 does and doesn't have, from its Actions reference. Items
marked *verify* were not stated there; the spike answered them (see *Spike
results*):

| GitHub feature | Used by | Forgejo v17 |
| --- | --- | --- |
| `workflow_run` trigger | `claude_fix_ci_failure`, `claude_fix_renovate_ci` | Absent. |
| `pull_request_review` trigger | `claude_implement_plan`, `claude_address_pr_feedback`, `claude_refine_plan` | Works, though undocumented. The action is `reviewed`, and the kind of review is in `review.type`. |
| `author_association` in the event | four agent dispatchers | Absent. The collaborator-permission API answers the same question. |
| `environment:` secret scoping | five deploy and release workflows | Absent. Secrets are per repo, org or user. |
| `permissions:` | most workflows | Ignored. The automatic token is read-only for fork PRs and read-write otherwise. |
| `id-token: write` (OIDC into GCP) | `deploy_cloud_functions`, `release_finalize` | Spelled `enable-openid-connect`, and it works. GCP needs a provider for the new issuer. |
| GitHub App tokens | the dispatch bridge, `claude_refine_plan`, `claude_resolve_conflict` | No Apps. A bot account with scoped access tokens instead. |
| Reusable workflows | `build_and_deploy_container`, `test_dev_e2e` | Supported, same repo or any public repo. |
| `concurrency`, `schedule`, `workflow_dispatch`, `issue_comment`, `pull_request_target`, `services`, `run-name`, `secrets: inherit` | throughout | Supported. Concurrency groups are best-effort. |
| Marketplace actions that call the GitHub API | `softprops/action-gh-release`, `marocchino/sticky-pull-request-comment`, `hmarr/auto-approve-action`, `dorny/paths-filter`, `actions/upload-artifact` | `paths-filter` and `upload-artifact` (v3, and Forgejo's v4 fork) work unchanged. The other three call the GitHub API and get a few lines of `scripts/forge` instead. |
| Push protection | required by `docs/secrets.md` for a public repo | Absent. A server-side hook can do the same job. |

Two things get simpler. Registration on the instance is closed, so no outside
account can open a PR, and the self-hosted runners and deploy secrets never sit
behind a fork. The runners also share a host with the forge, so the slow part
of an agent job today, cloning over the uplink from GitHub, becomes local.

## Context from Comments

No comments; the issue was filed with this plan. The decision it records was
made on 2026-11-12 against #3228's recommendation text, which kept GitHub as
the fallback "if the Forgejo spike fails". That fallback still stands: phase 0
here is the spike, and its exit is a go or no-go.

## Potential Solutions

### Option A: Flip, then fix

- **Approach:** import the public repo into Forgejo, make it canonical, and
  port workflows as each one fails.
- **Pros:** shortest path to "one repo"; no period of running two forges.
- **Cons:** deploy, release and the agent are all broken on day one, and fixing
  them happens on the repo that production ships from.
- **Effort:** large, and front-loaded into an outage.

### Option B: Shadow mirror, then flip

- **Approach:**
  - Create `ripls-org/ripls` on Forgejo as a pull mirror of the GitHub public
    repo, so every `npm run oss:sync` reaches it.
  - Port in this repo, where the changes ship through the existing export:
    workflows are made to run on both forges, and the Forgejo side runs them in
    shadow.
  - Flip only when shadow CI is green, and the agent, a deploy and a release
    have each been proven from Forgejo against the dev environment.
- **Pros:**
  - Nothing production depends on changes until the flip.
  - Each incompatibility is found by a real run, not by reading docs.
  - The flip itself shrinks to moving secrets and repointing remotes.
- **Cons:**
  - Workflows carry both spellings for a while (for example
    `permissions: id-token: write` next to `enable-openid-connect`).
  - Shadow runs add load to a host that also runs the GitHub fleet.
- **Effort:** large, spread over four phases that each ship.

### Option C: CI on Forgejo, agent stays on GitHub

- **Approach:** port tests, deploy and release. Leave the agent working against
  the GitHub mirror.
- **Pros:** skips the biggest single piece, the agent's `gh` calls.
- **Cons:** the mirror is one-way, so agent PRs would open on a repo that isn't
  canonical and need carrying back by hand. That is the two-repo coupling #3228
  exists to remove.
- **Effort:** medium, with a permanent tax afterwards.

## Recommendation

**Option B.** The port is mostly discovering small differences, and a shadow
mirror turns each one into a failed run on a repo nobody depends on. Option A
finds the same differences during an outage. Option C keeps two forges forever.

One trade-off is accepted up front: with no `environment:` scoping, every
workflow on a maintainer's branch can read every repo secret. On GitHub that
would be a real loss. Here the people who can push a branch are the people who
already hold the deploy credentials, and closed registration keeps everyone
else out. Branch protection on `main` and the gitleaks gates stay.

## Open Questions

- **Does the spike pass?**
  - It is a go if all of these work on the instance: a test workflow on a
    self-hosted runner, `pull_request_review` or a workable substitute, a
    workflow dispatched into a private repo by a bot token, and an OIDC token
    GCP accepts (or a decision to use a key instead).
  - Recommendation: if any one fails with no substitute, stop and take #3228's
    GitHub fallback. Don't build around a missing trigger with polling.
- **How do the runners run jobs?**
  - Today each of the five runner containers is the toolchain image with
    GitHub's runner as its entrypoint.
  - Recommendation: keep that shape. Add `forgejo-runner` to the image and run
    it in host mode inside the same containers. The cache mounts, the CPU
    budget, the recreate timers and `sync.sh` all keep working, and
    `runs-on: self-hosted-linux` keeps its meaning.
  - The alternative is docker mode, one runner daemon starting a fresh
    toolchain container per job. It is cleaner but redoes the cache layout and
    the CPU budget.
- **Does the separate agent repo survive?**
  - It exists because a public GitHub repo can't safely hold the agent's
    secrets next to fork PRs. Closed registration removes that reason.
  - Recommendation: keep it for this port and decide later. Folding it in is a
    refactor with no deadline; moving it as it is keeps the port mechanical.
    The bridge becomes a bot token that can only dispatch workflows there.
  - **Decided 2026-11-13: it stays separate.** Folding it in would remove the
    bridge, but the separation is what makes it safe to open registration
    later, and its complexity is already paid for. It is imported as the
    private `ripls-org/ripls-agent`.
  - What follows from that: the agent's secrets (the Claude token, the agent
    account's token) are **repo** secrets on `ripls-agent`, never org secrets.
    An org secret is readable by every repo in the org, the public one
    included, which is the exposure the separate repo exists to prevent.
- **What replaces `gh`?**
  - Recommendation: one script, `scripts/forge`, wrapping the Forgejo REST API
    for the subcommands actually used (`issue`, `pr`, `run`, `api`, `label`,
    `release`). Workflows, agent prompts and skills call it instead of `gh`.
  - It takes the same arguments as the `gh` forms in use, so prompts change by
    one word and the agent's habits carry over.
  - Off-the-shelf CLIs (`tea`, `fj`) cover issues and PRs but not Actions runs
    and logs, which the CI fixer needs.
- **Where do workflows live after the flip?**
  - Forgejo reads `.forgejo/workflows/` and falls back to `.github/workflows/`.
  - Recommendation: stay in `.github/workflows/` through the shadow period,
    then `git mv` to `.forgejo/workflows/` at the flip. The GitHub mirror then
    has no workflows and runs nothing, with no settings to maintain there.
    Confirmed in the spike: with both directories present, only
    `.forgejo/workflows/` ran.
- **OIDC or a key for GCP?**
  - Two workflows authenticate to GCP with Workload Identity Federation.
  - Recommendation: add a second provider to the pool for the Forgejo issuer.
    It keeps "no long-lived cloud keys in CI" true. If GCP rejects the token
    shape, fall back to a service-account key in a repo secret and record it in
    the secrets inventory.
- **How do issues and their numbers move?**
  - `TODO(#5)` and `TODO(#6)` name issues in `ripls-org/ripls`.
  - Recommendation: use Forgejo's GitHub migration for that repo, which keeps
    issue numbers. Check both numbers after the import. This repo's own issues
    are not imported: they are archived with it (#3229).
- **How does an outside contributor take part?**
  - They can't register. The GitHub mirror takes issues; patches arrive by
    email or through an account a maintainer creates.
  - Recommendation: accept that for now and say so in `CONTRIBUTING.md` at the
    flip. It is the known cost of Option C in #3228.
- **Push-time secret scanning?**
  - Recommendation: a `pre-receive` hook on the instance that runs gitleaks
    with this repo's `.gitleaks.toml`, and a paragraph in `docs/secrets.md`
    replacing the push-protection sentence. A hook rejects the push, so unlike
    the Actions scan it has no exposure gap.

## Implementation Plan

Work in this repo ships to the public repo through `npm run oss:sync` until the
flip, as now. Changes to the instance itself land in the mimir repo.

### Phase 0: Spike
**Goal:** a go or no-go, from real runs.
**Tasks:**
- [x] Enable Actions on the instance (mimir repo, `stacks/forgejo`).
- [x] Register one throwaway runner and create a scratch repo.
- [x] Run and record the result of each:
  - a job on `runs-on: self-hosted-linux` with a Postgres service container;
  - `pull_request_review` as a trigger;
  - the event payload for `issue_comment` and `issues`, and the
    collaborator-permission call that stands in for `author_association`;
  - a bot account's token dispatching a workflow in a private repo, and
    whether that token can be limited to doing only that;
  - `enable-openid-connect`, and the token's issuer and claims;
  - `dorny/paths-filter`, `actions/upload-artifact` and the three API-calling
    actions, each by full URL;
  - whether `.forgejo/workflows/` wins over `.github/workflows/`;
  - what the automatic token may do on a same-repo PR.
- [x] Write the results into this doc and mark the go or no-go.

**Validates:** every *verify* in the tables above has an answer.

#### Spike results (2026-11-13): go

Run on Forgejo 17.0.1 with runner v13.2.1, against a private scratch repo
with a maintainer account and a bot account that has write access.

| Probe | Result |
| --- | --- |
| Job on `runs-on: self-hosted-linux` | Passed. The runner started each job in a fresh container from the existing `ripls-runner:latest` image, with no change to the image. Go, Flutter, Node and the Docker socket were all there. |
| Postgres service container | Reachable by service name within a second. |
| Triggers | `push`, `pull_request`, `pull_request_target`, `issues`, `issue_comment`, `pull_request_review`, `pull_request_review_comment` and `workflow_dispatch` all fired. `workflow_run` never did. |
| Event payloads | No `author_association` anywhere. A label change arrives as `issues` with action `label_updated`, not `labeled`. A comment on a PR arrives as `issue_comment` with `is_pull: true`. A review's kind is `review.type`: `pull_request_review_approved`, `pull_request_review_rejected` or `pull_request_review_comment`. |
| Trust check | `GET /repos/{owner}/{repo}/collaborators/{user}/permission` returns `owner`, `write` or `none`. Only a repo admin may ask about someone else: the automatic token gets a 403, and the bot may only ask about itself. |
| Automatic token | On a same-repo run it can read the repo, create an issue, list runs and dispatch a workflow. Unlike on GitHub, a dispatch made with it does start a run. |
| Dispatch by the bot | The bot's token dispatched a workflow in the private repo. Tokens are scoped by API area (`write:repository`, `write:issue`), not by action, so what limits the bot is which repos it is a collaborator on. |
| `if: failure()` final job | Ran after a failed job and had the run id, which is what the CI fixer needs. |
| OIDC | A job with `enable-openid-connect: true` minted a token. Issuer `https://code.overt.org/api/actions`, with a public discovery document and key set. Subject `repo:<owner>-<id>/<repo>-<id>:ref:<ref>`; claims include `repository`, `ref`, `workflow` and `event_name`. GCP accepting it is proven in phase 3. |
| Actions | `actions/checkout@v4` resolved by short name. `dorny/paths-filter@v3` worked by full GitHub URL. `actions/upload-artifact@v3` and `forgejo/upload-artifact@v4` both uploaded. `GITHUB_STEP_SUMMARY` is set. |
| Workflow directory | With both present, only `.forgejo/workflows/` ran. |
| API | Runs, jobs, logs, reruns and artifacts are all in the REST API, which is what `scripts/forge` needs for the CI fixer. |

What this changes in the plan:

- **Trust check.** It can't run on the automatic token. The dispatchers need a
  secret holding a token that may read permissions, or the repo moves under
  an org and the check becomes org membership, which any member may read.
  Phase 2 picks one.
- **CI fixer.** The last-job pattern works, and that job can dispatch within
  the repo on the automatic token alone. Only the hop into the agent repo
  needs the bot's token.
- **Runner mode.** Docker mode worked with the image as it is, so adding
  `forgejo-runner` to the image may not be needed. Phase 1 chooses between
  host mode in the existing containers and five small runner daemons that
  each start jobs from the image with that runner's cache mounts.
- **Dispatcher conditions** that test `github.event.action == 'labeled'` or
  `review.state` need the Forgejo spellings.
- Not covered: the Mac runner, and GCP accepting the token.

### Phase 1: Runners and shadow CI
**Goal:** every push to the public repo runs the test suite on Forgejo.
**Tasks:**
- [ ] Add `forgejo-runner` to `runners/Dockerfile`; register two of the five
  containers with Forgejo and leave three on GitHub until the flip.
- [x] Register the Mac that builds iOS: `runners/forgejo/mac/`, a
  host-mode runner started as a LaunchAgent, label `self-hosted-mac`. A probe
  job checked out, found Xcode, CocoaPods, Ruby and the JDK, and installed
  the pinned Flutter. Not yet run: a real iOS build, and signing. Both wait
  for the release rehearsal in phase 3.
- [ ] Create `ripls-org/ripls` on Forgejo as a pull mirror of the GitHub repo,
  with LFS.
- [ ] Make the test and lint workflows pass there. Replace or script around
  each action the spike found wanting.
- [ ] Set `CI_RUNNER_LINUX` on the Forgejo repo; leave `INSTANCE_PIPELINES`
  unset so nothing deploys from the shadow.
- [ ] Update `runners/README.md` and `docs/infra/dependency_automation.md` for
  the two-forge period.

**Validates:** a sync push turns green on both forges.

### Phase 2: `scripts/forge` and the agent
**Goal:** the agent plans, implements, addresses review and fixes CI from
Forgejo events.
**Tasks:**
- [x] Write `scripts/forge` with tests against recorded API responses
  (#3306, #3307). It covers every `gh` form in use across both repos, plus
  two commands of its own (`forge file`, `forge permission`).
- [x] Switch this repo's dispatchers from `gh` to it (#3310 to #3313). On
  GitHub it passes through to `gh`, so both forges keep working. Still on
  `gh`: `release_cut`, `release_finalize`, `deploy_cloud_functions`.
- [x] Create the bot accounts and their tokens; replace the three App-token
  steps. Three accounts, by what each may do (see *Probe round two*):
  - `ripls-agent`: writes to the repo (branches, PRs, comments). A
    collaborator, not an org member.
  - `ripls-dispatch`: starts workflows in the agent repo and answers the
    trust check. Created 2026-11-13 and proven to be able to do only that.
  - a Renovate account, when Renovate moves.
- [x] Port the dispatchers (#3311 to #3313):
  - the trust check becomes a step: is the actor a member of the org? It
    uses the dispatch account's token, since the automatic token can't ask;
  - the CI fixers lose `workflow_run`. Each watched workflow gains a last job,
    `if: failure()`, that dispatches the fixer with the run id.
- [x] Import `ripls-agent` and port its workflows and
  `validate_request.sh`. See *The agent on Forgejo* below for how. Not yet
  ported: the nightly docs refresh (`refresh_docs`), and release-cut is ported
  but has not run.
- [ ] Self-host Renovate against the Forgejo repo, and port
  `renovate_auto_approve` and `validate_runner_candidate`.

**Validates:** on the shadow repo, the agent takes one issue from plan to
merged PR, and fixes one deliberately broken test run.

The baseline to match is the same exercise on GitHub, run on 2026-11-13
against a real task (the connectrpc 2 upgrade): plan, refine, implement and
address-feedback each worked once through the dispatch bridge. Fix-ci did not
run, because nothing failed.

#### Probe round two (2026-11-13)

Run on a second scratch repo under the org, on the real runner fleet, to
answer what the workflow audit left as *verify*.

| Question | Answer |
| --- | --- |
| Is `github.run_id` the id the API takes? | Yes. It is the global run id; `run_number` is the per-repo index shown in URLs. |
| `github.triggering_actor` | Empty. Use `github.actor`. |
| Label events | Action `label_updated`. The payload has both a singular `label` object and `issue.labels`. A `types: [labeled]` filter did not need changing to receive it. |
| Is a comment on a PR distinguishable? | Yes, two ways: `is_pull: true`, and `issue.pull_request` is an object as on GitHub. The existing expression works unchanged. |
| Review payload | Only `review.type` and `review.content`. No review id and no `body`. A dispatcher that needs the id must look the review up. `types: [submitted]` still matches. |
| `user.type` | Absent. Bots are ordinary users; tell them apart by login. |
| What does a merged `pull_request: closed` run check out? | The merge commit, which is also `github.sha`. `release_finalize`'s prod build is therefore built from the right commit (audit B7 does not apply). |
| `if: always()` after a skipped `needs` | Runs, with `needs.<job>.result == 'skipped'`. The path-gated pattern works. |
| `if: always()` steps when a run is cancelled | **Do not run.** A container the job started on the host daemon was still up after the cancel. The orphan sweep is the only cleanup for superseded runs (audit D5). |
| `actions/upload-artifact@v7` | **Fails**: "not currently supported on GHES". Only v3 and Forgejo's v4 fork work. |
| What can the automatic token ask about a user? | Nothing useful: collaborator permission is 403, org membership is a redirect, team lists are 403. |

**The dispatch account is confined as intended.** `ripls-dispatch` is in an
org team whose only unit is Actions (write), on one repo. With its token:

| Attempt | Result |
| --- | --- |
| Dispatch a workflow in that repo | 204 |
| Read a file, write a file, create a branch, open an issue | 403 each |
| List secrets, set a variable | 403 each |
| Read another repo in the org | 404 |
| Is a given user an org member? | 204 for a member, 404 for a non-member and for a collaborator-only bot |

So the credential the public repo holds can start an agent run and nothing
else, which restores what the GitHub App gave (audit B9). The same token
answers the trust check, so the dispatchers need one secret, not two. Team
membership can't be read with it (403), so trust is "member of the org", and
the bot accounts that write to the repo stay collaborators, not members.

#### The agent on Forgejo (2026-11-13)

The agent repo runs on both forges from one set of files. GitHub's path is
left as it was; Forgejo's sits beside it, chosen by `github.server_url`.

| Part | GitHub | Forgejo |
| --- | --- | --- |
| Who the agent is | The `ripls-agent` App; a token minted per step | The `ripls-agent` account; its token is the `AGENT_TOKEN` secret |
| Trust check in validation | Collaborator permission | Org membership, asked with `AGENT_TRUST_TOKEN` (a `ripls-dispatch` token that can read the org and nothing else) |
| A fixer started by CI | The caller run's event is `workflow_run` | The caller run was dispatched by the system account |
| `gh` in scripts and prompts | The real `gh` | `forge`, installed under the name `gh` from the target's default branch |
| Claude | `anthropics/claude-code-action` | The Claude Code CLI (`run-claude-cli` action). The action rejects the event Forgejo reports inside a reusable workflow |
| Execution log | An artifact | Printed to the job log (`upload-artifact@v7` does not run) |

All three secrets (`AGENT_TOKEN`, `AGENT_TRUST_TOKEN`,
`CLAUDE_CODE_OAUTH_TOKEN`) are repo secrets on the agent repo, never org
secrets, so no workflow in the public repo can read them.

**Verified on the shadow repo, 2026-11-13.** One issue went through every
mode, each started the way a person or CI would start it:

| Mode | Started by | Result |
| --- | --- | --- |
| plan | an `@claude` comment on the issue | Draft plan PR, labeled, assigned to the requester |
| refine | an `@claude` comment on the plan PR | Plan edited and pushed; PR description updated |
| implement | an approving review saying `@claude implement` | Code and tests pushed; PR marked ready; stage labels swapped; review requested |
| address | a change-requesting review with an inline comment | Both points addressed and pushed |
| fix-ci | CI failing on a planted bad test, with nobody asking | Cause found, fix pushed |
| resolve | a manual request on a PR that conflicted with its base | Merge commit pushed keeping both sides |

Validation was also sent five forged requests directly (wrong issue for the
caller run, wrong actor, a stale caller run, extra fields, a repo the agent
cannot read) and refused each.

Two Forgejo behaviors cost a debugging round each and are worth knowing:

| Behavior | What it broke | What works on both forges |
| --- | --- | --- |
| On a job that calls a reusable workflow, `inputs.*` in `if:` does not resolve | `if: inputs.mode == 'plan' && …` was false for every mode, so validation passed and then nothing ran | `github.event.inputs.mode` |
| `toJSON(needs.<job>.outputs)` renders as `{"JobID": …, "Outputs": null}` | The modes received no validation results | The validate script writes one JSON output itself; callers pass `needs.validate.outputs.ctx` |

### Phase 3: Deploy, release and the tracker
**Goal:** everything production needs has run once from Forgejo.
**Tasks:**
- [ ] Add the GCP identity provider and prove `deploy_cloud_functions`.
  The provider is in place in prod (#3326): pool `forgejo-actions`, provider
  `forgejo`, accepting only runs of this repository (by its id) on `main`
  that a pull request did not trigger. The workflow picks the provider by
  forge, and its failure alert goes through `scripts/forge`. Proven from a
  scratch repository on Forgejo: `google-github-actions/auth` runs there, and
  Google verified the run's token and refused it for one reason, the
  attribute condition (wrong repository). Left for the cut-over change: the
  top-level key `enable-openid-connect: true`, which GitHub rejects as an
  unknown key (tested), and with it the first deploy from Forgejo. The
  functions deploy to prod only, so there is no dev run to prove it against
  first.
- [ ] Port `release_app`, `release_finalize` and the deploy workflows: releases
  and tags through `scripts/forge`, artifacts through the action the spike
  chose. Done in the workflow files, with each piece tried on a scratch
  repository:
  - `release_finalize` starts Release App through `scripts/forge workflow
    run` on both forges. On Forgejo, as on GitHub, a tag pushed with the
    workflow's token starts nothing (the same tag pushed by a user does), so
    the explicit dispatch is needed there too. A dispatch on a tag ref runs
    on that tag, and `inputs.*` resolves in an ordinary job's `if:`.
  - `release_app` creates the release with `softprops/action-gh-release` on
    GitHub and `scripts/forge release create` on Forgejo, with the release's
    notes file as the body. Its tag-on-main check no longer pipes into
    `grep -q`.
  - Both iOS workflows pointed at a Ruby 3.4 gem directory that does not
    exist on the Mac; they now name the 4.0 one.
  Not done: a run of either workflow on Forgejo. That is the rehearsal below.
- [ ] Run a dev deploy and a full release rehearsal that stops before the store
  upload. Ready for it (2026-11-14): the 17 secrets the deploy, release and
  eval workflows read were copied to the Forgejo repository by a one-off
  GitHub workflow that sent each value from GitHub's secret store to
  Forgejo's API, so no value was printed or handled by a person. The deploy
  variables are set too. `INSTANCE_PIPELINES` is still unset there, which is
  what keeps the shadow from deploying; setting it is the switch.
  Not copied, because no workflow reads them: `GEMINI_API_KEY`, `MAPBOX_KEY`,
  `UNSPLASH_KEYS`, `RIPLS_FEEDBACK_PAT`, `GITLEAKS_LICENSE`, the GitHub App
  key, and the org's two Apple push secrets.
- [x] Create the private tracker repo on the instance (`ripls-org/tracker`,
  issues only). Still to do: move #2760 into it.
- [x] Give `server/github` a Forgejo implementation behind its existing
  interface, chosen by config (#3318), and port `cloud_functions/` from
  Octokit to the Forgejo API (#3321). Unit tests cover both. See *The tracker
  move* below.
- [x] Install the `pre-receive` gitleaks hook and update `docs/secrets.md`.
  The hook is `infra/forgejo/pre-receive-gitleaks`, installed on the shadow
  repository by `infra/forgejo/install_hook.sh`. On this Forgejo version a
  repository has no hooks directory of its own until one is made: the global
  `pre-receive` runs whatever is in `<repo>.git/hooks/pre-receive.d/`. Tried
  on the instance: a clean push went through, and a push carrying a made-up
  token was refused with the finding printed and nothing left on the server.
  `gitleaks-push` stays until the cut-over (audit decision 5).

**Validates:** a feedback report and a test crash alert each open an issue in
the private Forgejo tracker; the dev deploy and the rehearsal pass.

#### The tracker move (2026-11-13)

Two accounts file issues in the tracker, each with a token scoped to issues
and write access to that one repository: `ripls-feedback` (the server) and
`ripls-alerts` (the Cloud Functions). Either token gets 403 on the code repo.

| Path | State | Verified by |
| --- | --- | --- |
| Alerts, prod | Live | A synthetic incident published to the prod alert topic opened an issue; the matching resolved message commented on it and closed it |
| Feedback, dev | Live | A test report to the dev server opened an issue with labels `bug` and `user-feedback` |
| Feedback, prod | **Not yet** | See below |

**Prod feedback switches at the next prod release.** `infra/server/common.yaml`
on `main` already points both environments at the tracker (#3323), and prod's
secret holds a token. But the image prod runs predates #3318, and a deploy
does not ship settings; `infra/sync.sh` does. So:

1. Run `infra/sync.sh` immediately before the release deploy, so the new
   settings and the new image arrive together.
2. Check the prod log for "feedback service initialized with a Forgejo
   tracker".

Do not run `infra/sync.sh` and then restart prod on the old image: the old
server ignores the tracker URL and the GitHub App ids are gone from the shared
file, so feedback would be switched off.

Alerts are private for now. In the private tracker `claude-fix` starts
nothing. Making the safe alerts public is tracked on the Forgejo code repo.

One more Forgejo behavior, found by a live run: listing issues with a label
filter that names a label the repository does not have returns **every**
issue, not none. Anything that finds "the issue with this label" has to check
the labels on what comes back.

### Phase 4: Hand back to #3228
**Goal:** #3228's phase 4 runs as written, with Forgejo as the destination.
**Tasks:**
- [x] Turn the Forgejo repo into the canonical repo, and `ripls-org/ripls` on
  GitHub into its push mirror. Done 2026-11-14; see *The cut-over* below.
- [ ] `git mv .github/workflows .forgejo/workflows`; drop the GitHub spellings
  and the `gh` pass-through. Not done. Actions is switched off on the GitHub
  mirror instead, which is one setting; the move touches every workflow's own
  `paths:` and the reusable-workflow `uses:` lines, and can follow.
- [x] Runners. The Forgejo fleet stays at three Linux daemons, not five: see
  below. The GitHub fleet is stopped (its containers and the runner on the
  Mac), and its recreate timers are removed from the host.
- [x] Continue with #3228 phase 4: final export, secrets, branch protection,
  docs, the proving release. #3229 (archiving the private repository) is the
  one step left.

**Validates:** #3228's criteria.

#### The cut-over (2026-11-14)

What was done, in order:

1. A final export of the private repository's `main` to both forges, with the
   denylist and gitleaks gates passing.
2. Actions switched off on the three GitHub repositories (the private one, the
   private agent repo and the public mirror), so nothing deploys twice.
3. The cut-over change, as the first pull request on Forgejo (#10 there):
   `enable-openid-connect: true` for the Cloud Functions deploy; `gitleaks-push`,
   `verify_toolchain` and the hourly conflict scan dropped; the denylist
   scanner shipped with a `Denylist Check` workflow in place of the export
   check; `TODO(#5)` renumbered to the issue's number here, #7.
4. `INSTANCE_PIPELINES` set on the Forgejo repository.
5. A push mirror from Forgejo to GitHub for `main` and tags, authenticated by
   a deploy key on the GitHub repository.
6. Branch protection on `main`: no force-push, and five always-run checks
   required to merge a pull request.

What ran from Forgejo afterwards:

| What | Result |
| --- | --- |
| A full pull-request run (30 checks) | Passed |
| Push mirror | GitHub `main` followed every merge |
| Cloud Functions deploy to prod, with the OIDC token | Passed on the first run |
| iOS build on the Mac, Android release build | Passed |
| Deploy to Dev, including the dev end-to-end tests | Passed, after two fixes below |
| Staging website preview and its cleanup | Passed, after one fix below |

Four things only a real run found:

| Finding | Fix |
| --- | --- |
| **Five runner daemons exhaust the host's Docker address pool.** Every job takes a network of its own; with five jobs at once, creating one failed with "all predefined address pools have been fully subnetted", and two image builds collided in the shared image store. | Back to three daemons. Issue #12 has what would lift the cap. |
| **The host refused the deploy key.** It accepted that key only from Docker's first address range, and a job's network can come from the second. sshd logged "correct key but not from a permitted host". | The key's allowed source range on the host now covers both of Docker's ranges and still excludes the LAN. |
| **Repository variables are printed in logs; secrets are masked.** The first deploy log showed the deploy host's address. | The deploy and website workflows read each endpoint from a secret of the same name first, falling back to the variable. |
| **Two more GitHub-only steps:** the staging preview's sticky comment (a marketplace action) and the runner-candidate notice (`gh`). | `.github/scripts/sticky_comment.sh`, which uses the two REST calls both forges share; and `scripts/forge`. |

Still to do, none of it blocking daily work:

- Renovate against the Forgejo repository, and `renovate_auto_approve`. Until
  then dependency updates are paused; the host's `run-renovate` timer still
  points at the private GitHub repository.
- The nightly docs refresh in the agent repo.
- The move to `.forgejo/workflows/`.
- Making the Forgejo repository public.
- Enlarging the Docker address pool (#12).

## Related Issues
- #3228 (OSS-17): the cut-over. Its phase 4 is gated on this port.
- #3229 (OSS-15): archives this repo once the cut-over is proven.
- #2953: the epic.
- #2970 (OSS-16): the dispatch bridge that phase 2 ports.
- #2760: the open issue filed from user feedback; it moves to the private
  tracker in phase 3.
