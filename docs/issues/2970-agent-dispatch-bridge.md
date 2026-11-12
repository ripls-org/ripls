# Issue #2970 + #2959: Agent dispatch bridge

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2970 (OSS-16),
https://github.com/Type2Fun-Foundation/ripls/issues/2959 (OSS-7, remaining half)
**Filed:** 2026-08-22 | **Last activity:** 2026-10-19
**Labels:** open-source
**Context docs:** `docs/issues/2953-open-source-release.md` (Phase 2 anatomy
notes), `ripls-notes/coding_agent_cheat_sheet.md`, `docs/reviews/security_review_prompt.md`

Supersedes the two closed plan PRs (#2968, #2972). #2972 planned an ephemeral
runner pool, which the 2026-08-22 decision replaced with "no runners on the
public repo; dispatch into a private one".

## Decisions (2026-10-19)

1. **Built target-agnostic now, enabled publicly at OSS-17.** The agent takes
   `target_repo` as an input. Until OSS-17 it serves this repo's issues and PRs
   exactly as the eight `claude_*` workflows do today. The public dispatchers
   ship in the export but stay inert, with no dispatch credential in
   `ripls-org/ripls`, until OSS-17 reverses the sync direction. Agent work
   merged on the public repo before then would be overwritten by the next sync,
   and plan docs in `docs/issues/` would be deleted outright.
2. **The agent lives in a dedicated private repo, `Type2Fun-Foundation/ripls-agent`,**
   not in this one. The dispatch credential needs `actions: write` on whatever
   repo it targets, and GitHub cannot scope that to a single workflow. Here it
   could start `deploy_manual`, `release_cut`, `release_app`, `publish_website`
   or `deploy_cloud_functions`. In a repo whose only workflow is the agent, a
   leaked dispatch credential can only *request* an agent run, and the agent
   re-validates every request.
3. **GitHub Apps, not PATs**: one for acting on targets and one for dispatching
   (§Credentials).

## Architecture

```
 caller repo (this repo now; ripls-org/ripls after OSS-17)
 ─────────────────────────────────────────────────────────
 claude_<mode>.yaml   thin dispatcher, ubuntu-latest, no checkout
   triggers: unchanged (issue_comment, issues, pull_request_review[_comment],
             workflow_run, schedule, workflow_dispatch)
   gate:     actor != bot; author_association ∈ OWNER/MEMBER/COLLABORATOR;
             trigger phrase/labels as today
   action:   mint ripls-dispatch token → gh workflow run agent.yaml
             -R Type2Fun-Foundation/ripls-agent -f mode=… -f target_repo=…
                                   │
                                   ▼
 Type2Fun-Foundation/ripls-agent (private; only workflow in the repo)
 ─────────────────────────────────────────────────────────
 agent.yaml  workflow_dispatch, org runner fleet
   1. validate: target_repo ∈ vars.AGENT_TARGETS; actor has write+ on target;
                PR exists, OPEN, same-repo head, head_sha matches (PR modes)
   2. mint ripls-agent token scoped to target_repo
   3. checkout target @ ref; shared setup (composite actions in this repo)
   4. render prompts/<mode>.md; run claude-code-action with github_token=<target token>
   5. mode epilogue (labels, reconcile, guard, assert) against target
```

The **discover/guard logic moves private**. The dispatcher only gates on the
event and passes identifiers; everything else (`head_ref`, `head_sha`,
`plan_path`, `issue_number`) is re-derived from the target with the agent's own
token. Inputs are untrusted even though the dispatcher is ours, because a leaked
dispatch key lets anyone call `agent.yaml` directly.

### Dispatch input contract

| Input | Used by | Notes |
|---|---|---|
| `mode` | all | `plan`, `refine`, `implement`, `address`, `fix-ci`, `fix-renovate`, `resolve`, `refresh-docs` |
| `target_repo` | all | allowlisted |
| `number` | all but refresh-docs | issue (plan) or PR number |
| `actor` | all | the human who triggered it; re-checked, becomes `REQUESTER` for plan |
| `caller_run_id` | all | the dispatcher's own run id in the target; binds the request to a real trigger (below) |
| `head_sha` | PR modes | must equal the PR's current head; stale requests are dropped |
| `trigger_kind`, `comment_id` / `review_id` | address, refine | the agent **fetches** the comment or review body by id from the target. No free text crosses the bridge |
| `instruction` | address, refine (manual) | free text, only from `workflow_dispatch` on the caller, which requires write access |
| `failed_run_id`, `failed_workflow_name` | fix-ci, fix-renovate | logs fetched cross-repo with `actions: read` |
| `base` | refresh-docs | optional window override |

Passing ids rather than bodies keeps attacker-shaped text out of the dispatch
payload and the run's input log. It also means a hand-crafted dispatch cannot
smuggle in text that never appeared on the target.

**Binding requests to real triggers.** An actor check alone does not stop a
leaked dispatch key: the forger simply names a maintainer as `actor`. So every
dispatch carries `caller_run_id`, and the agent looks that run up in the target
with its own token. It must:
- be a run of the mode's dispatcher workflow;
- have been triggered by `actor`;
- be under 15 minutes old;
- not have failed or been skipped;
- have the run name `agent-dispatch <mode> <number>`. The dispatcher sets this
  with `run-name:` from the event, so neither the key holder nor the commenter
  chooses it.

A forger can therefore only replay a request a trusted actor genuinely made
within the last 15 minutes.

## Credentials

| App | Owner, installed on | Permissions | Key stored in |
|---|---|---|---|
| **ripls-agent** | Type2Fun-Foundation, *installable on any account*; installed on `Type2Fun-Foundation/ripls` and `ripls-org/ripls` | Contents RW, Pull requests RW, Issues RW, **Workflows RW** (agents edit `.github/workflows/`), Actions R, Checks R, Metadata R | `ripls-agent` repo secrets only |
| **ripls-dispatch** | Type2Fun-Foundation, private; installed **only** on `ripls-agent` | Actions RW, Metadata R | each caller repo (`AGENT_DISPATCH_APP_ID` var + `AGENT_DISPATCH_PRIVATE_KEY` secret) |

- **What ripls-agent replaces:** `CLAUDE_WORKFLOW_PAT`, and the Claude GitHub
  App token that claude-code-action mints from OIDC for the *running* repo,
  which would be the wrong repo here.
- **Tokens:** minted per job with `actions/create-github-app-token`, scoped with
  `repositories:` to the single target.
- **App tokens trigger workflows** (unlike `GITHUB_TOKEN`). That keeps
  `claude_fix_ci_failure` and CI firing on agent pushes, as the PAT does today.
- **Visible identity changes** from `claude[bot]` to `ripls-agent[bot]`. Every
  loop guard (`github.actor != 'claude[bot]'`) must accept both until the
  cut-over is complete.
- **Agent-repo secrets:** `CLAUDE_CODE_OAUTH_TOKEN`, `OPENAI_API_KEY`,
  `GCP_SERVICE_ACCOUNT_KEY_DEV`, `BUF_TOKEN` and the ripls-agent key move here
  (or are copied, during migration). The public repo never holds any of them.

## Known blockers (from reading `claude-code-action@v1` source)

1. **The action rewrites `origin` to the running repo.**
   `configureGitAuth` runs
   `git remote set-url origin https://x-access-token:<token>@github.com/<context.repo>.git`,
   and `context.repo` comes from `GITHUB_REPOSITORY`, which is `ripls-agent`.
   Claude's `git push origin` and every `gh` call that infers the repo from
   `origin` would hit the wrong repo.
   **Chosen: (b), plus an explicit re-point for the epilogue** (implemented in
   `ripls-agent` `agent.yaml`). Read from the v1.0.253 source: with no
   `allowed_non_write_users`, the action writes exactly
   `https://x-access-token:<github_token>@github.com/<GITHUB_REPOSITORY>.git`,
   and we supply `<github_token>`, so the `insteadOf` key is known in advance and
   does not depend on GitHub honouring an env override. Confirmed by the spike.
   The candidates were:
   - (a) Step-level `env: GITHUB_REPOSITORY: <target>` on the action step.
     GitHub documents that `GITHUB_*` cannot be overwritten; check whether this
     holds for step `env:`.
   - (b) Set `url."https://x-access-token:<tok>@github.com/<target>.git".insteadOf`
     to the exact URL the action writes. We pass `github_token`, so we know
     `<tok>`.
   - (c) Re-point `origin` after the action, and instruct the agent to use
     `git push "$TARGET_REMOTE"`. This is the least robust option.

   Set `GH_REPO=<target>` on the action step regardless.
2. **Actor check.** Agent mode runs `checkHumanActor` on `github.actor`. Behind
   `gh workflow run` that is `ripls-dispatch[bot]`, so it goes in
   `allowed_bots`. The existing `allowed_bots` (`ripls-alerts-bot` for plan,
   `ripls-renovate` for fix-renovate) express a policy about *who triggered*.
   That policy moves to the dispatcher gate and the agent's actor validation.
3. **`@v1` is a floating tag.** We now depend on its internals (the origin
   rewrite, OVERRIDE_GITHUB_TOKEN), so pin it by commit SHA and let Renovate
   bump it.
4. **The local composite action** `./.github/actions/cleanup-test-postgres`
   resolves against the checked-out *target*. That keeps working, because the
   action ships in the export, but it is a coupling to note: the agent's own
   composite actions are referenced from `ripls-agent`'s checkout, at a
   separate path.

## Pre-existing defects to fix during the port

- **resolve:** "Resolve PR refs" runs `gh pr view` before checkout and without
  `GH_REPO`. It only works because self-hosted runners reuse a workspace that
  still holds `.git`.
- **plan:** the prompt interpolates the issue title raw. Pass it via the staged
  `/tmp/issue.json`, as the body already is.
- **Post-agent `git ls-remote origin` / `git push origin`** (Assert in 7 modes,
  implement's Reconcile, fix-renovate's Guard) run after the action revokes its
  token. Supplying our own `github_token` skips that revocation, but use an
  explicit authenticated remote for the target anyway.
- **`ripls-notes/coding_agent_cheat_sheet.md` is stale:**
  - it omits `claude_fix_renovate_ci`;
  - it says refresh-docs never auto-merges, which is no longer true;
  - it lists resolve's `pr_number` as optional, but it is required.

## Implementation plan

### Phase 0: Spike (one mode, end to end) ✅ 2026-10-19

- `ripls-agent` created. The mimir runners were re-registered at org level
  (`RUNNER_SCOPE: org`, #3174), which needs an `admin:org` PAT on the host.
- Both Apps were created from manifests:
  - `ripls-agent`, App ID 5388136, installed on `ripls`;
  - `ripls-dispatch`, App ID 5388143, installed on `ripls-agent` only.
- **plan** is ported (#3175, #3177). Its dispatcher, `claude_bridge_plan.yaml`,
  runs alongside `claude_plan_issue.yaml` behind `@claude-bridge`.
  `claude_plan_issue.yaml` ignores that phrase.

**Results:**

- **End to end:** `@claude-bridge` on #3176 produced PR #3178. Its branch,
  commit, PR, issue comment and assignee all landed on `ripls`, authored as
  `ripls-agent[bot]`. Nothing was pushed to `ripls-agent`. CI ran on the
  pushed branch.
- **Origin fix (blocker 1):** the `insteadOf` mapping works.
- **Actor check (blocker 2):** `allowed_bots: ripls-dispatch` works.
- **Refused, as intended:**
  - a non-allowlisted `target_repo`;
  - an unported mode;
  - a `caller_run_id` that is not a run in the target;
  - the real dispatcher run, re-cited with a different `number`
    ("Caller run was not for plan #3000") or a different `actor`;
  - a dispatcher run that failed, which therefore cannot be cited.
- **Found:** `gh workflow run` without `--ref` needs Contents read, to look up
  the default branch. The dispatcher passes `--ref main`, so the dispatch App
  keeps only Actions write.
- **Not yet exercised:**
  - a non-collaborator's comment. It needs a second account; the dispatcher
    gate is the same `author_association` check the current workflows use.
  - a stale `head_sha`, which moves to refine, the first PR mode.

### Phases 1–2: Shared pieces and the remaining modes ✅ 2026-10-20

All eight modes run in `ripls-agent` (#3182, #3185). Every `claude_*` workflow
here is now a dispatcher, and `claude_refresh_docs` moved there entirely as a
scheduled `refresh_docs.yaml`.

**How it is built:**

- **Validation** lives in one script, `scripts/validate_request.sh`, whose
  header tabulates the per-mode rules. Each request must cite a real dispatcher
  run whose run-name encodes it:
  - `agent-dispatch <mode> <number>`;
  - for `workflow_run` triggers, `<failed run id>`;
  - for resolve scans, `scan`.
- **Where trust comes from** when a person didn't directly cause the event:
  - fix-ci and fix-renovate: a failed run on the PR's own branch, at its head;
  - scan-triggered resolve: GitHub reporting the PR as `CONFLICTING`.
- **Comment and review triggers** must be the actor's own text on that PR.
- **Implement** refuses a PR whose head moved after the request.
- **Modes are `workflow_call`-only** `mode_*.yaml` files, so the dispatch App
  cannot start one without passing validation.
- **Concurrency is per PR, on the mode job.** A `recheck` job runs once the
  lock is held: implement, fix and resolve skip if the PR moved while queued;
  refine and address follow the new head.
- **Shared composite actions:** `setup-target`, `prepare-agent`,
  `agent-epilogue`, `assert-deliverable`, `recheck-pr`. Prompts render through
  `scripts/render_prompt.sh` with explicit placeholder lists.
- **Dispatchers here** share `.github/actions/agent-dispatch`. Those that load
  it run on GitHub-hosted runners: loading it needs a sparse checkout, which
  poisons reused self-hosted workspaces (see `claude_fix_renovate_ci.yaml`).

**Verified end to end (2026-10-20):**

| Mode | Test |
|---|---|
| plan | #3186 → #3187 |
| refine | questions on #3183 |
| implement | #3187: labels moved to `claude-implemented`, ready for review, reviewer requested |
| address | inline review comment on #3187 → fix commit |
| fix-ci | manual dispatch |
| resolve | manual dispatch; clean-merge path |
| refresh-docs | manual run; correctly found nothing stale |

Not yet exercised: fix-renovate (needs a real Renovate CI failure) and
resolve's conflict path.

**Found during rollout:**

- **Re-clone churn.** Agent jobs share one workspace per runner. Checking the
  agent repo out at its root made actions/checkout re-clone ripls every run,
  about an hour each. Fixed by always using `.agent/`, plus seeding each
  runner's workspace from a full bare clone (see the agent README).
- **One-hour token.** App installation tokens last an hour; implement's
  timeout is 90 minutes. That is the same limit the old workflow had with the
  token claude-code-action minted.
- **Docs PR approval.** The ripls-agent App cannot approve its own docs PR, as
  the old `github-actions[bot]` approval did. `refresh_docs` relies on `main`
  needing no approving review.

### Phase 3: Cut over and retire ✅ 2026-10-23

Verified by the v0.33.0 release: Cut Release opened #3206 as the agent's bot
with CI running, and Release Finalize deployed, tagged and started Release App
on the tag, which built both platforms and created the GitHub release. The
first cut attempt hung in `git fetch` until its 30-minute timeout; every agent
mode now sets `GIT_HTTP_LOW_SPEED_LIMIT`/`_TIME` so a stalled fetch aborts and
`actions/checkout` retries it.

- `release_cut` becomes a dispatcher for a new agent mode, `release-cut`.
  - The bump travels in `instruction`, and `number` is empty.
  - The agent only writes the notes. Its git and gh access is read-only.
  - The workflow refuses any change outside the release-notes paths, then
    commits, pushes and opens the PR with the App token, so CI runs.
- `release_finalize` pushes the tag with `GITHUB_TOKEN`, which triggers
  nothing. It then dispatches Release App on the tag.
  - Release App now runs its tag-on-main check on any tag ref.
  - A dispatch creates the GitHub release only if no requested build failed.
  - So the dispatch behaves exactly like the old tag push.
- Remove `CLAUDE_WORKFLOW_PAT` and `CLAUDE_CODE_OAUTH_TOKEN` from this repo.
  `OPENAI_API_KEY` and `GCP_SERVICE_ACCOUNT_KEY_DEV` **stay**:
  - deploys and the Go, AI-integration and web E2E tests use them;
  - the agent repo holds copies, and `secret-rotation.md` now says to rotate
    both.
- Drop `claude[bot]` from the loop guards. Drop `claude_bridge_plan.yaml`
  from plan's dispatcher list.
- Update the cheat sheets and `secret-rotation.md`.
- Seed new runners' agent workspaces automatically
  (`runners/seed_agent_workspaces.sh`, #3189).

### Phase 4: Public enablement (with OSS-17)

- Add the `ripls-dispatch` key to `ripls-org/ripls`.
- Install `ripls-agent` there.
- Add `ripls-org/ripls` to `AGENT_TARGETS`.
- Create the stage labels.
- Run the fork-negative test: a non-member `@claude` on a fork PR dispatches
  nothing.
- Run the member-positive test: the plan PR lands on `ripls-org/ripls`.

**Status 2026-10-23:** done except the fork-negative test, which needs an
account outside the org.

- Key, App install (all repositories; `ripls` is the org's only repo),
  `AGENT_TARGETS`, and the `AGENT_REPO`/`AGENT_DISPATCH_APP_ID` variables are
  set. The labels are created on first use.
- The public repo only gets the dispatchers by a sync (`npm run oss:sync`,
  #3210). The 10-19 sync predated the bridge.
- Member-positive passed: `@claude` on ripls-org/ripls#1 produced plan PR #2
  from `ripls-agent[bot]` with CI running; both closed.
- **Org membership must be public.** On a public repo, the event payload
  reports a private org member's `author_association` as `CONTRIBUTOR`, so the
  dispatchers' trusted-actor check skips them. It fails closed, but every
  maintainer who should be able to start the agent there must make their
  `ripls-org` membership public.

## Risks

| Risk | Mitigation |
|---|---|
| Origin rewrite cannot be neutralised cleanly | Spike decides before any porting; (b) does not depend on GitHub honouring an env override |
| Prompts drift between old and new paths during migration | Port one mode at a time and delete the old job in the same PR |
| `ripls-agent` starves for runners | Phase 0 sizes org-level capacity; agent concurrency groups keyed `agent-<mode>-<target>-<number>` |
| A leaked dispatch key | Can only start agent runs, and each must cite a real dispatcher run under 15 minutes old that matches its mode, number and actor; rotate the App key |
| A leaked ripls-agent key | Most powerful credential in the system; stored in one repo with no `pull_request` triggers |
