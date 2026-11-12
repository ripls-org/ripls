---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: How to drive Claude through GitHub to plan, implement, and iterate on fixes — the claude_*.yaml workflow triggers, stage labels, plan-PR lifecycle, refine/implement/feedback routing, CI auto-fix, conflict resolution, and the doc-freshness ratchet.
  globs: [.github/workflows/claude_plan_issue.yaml, .github/workflows/claude_refine_plan.yaml, .github/workflows/claude_implement_plan.yaml, .github/workflows/claude_address_pr_feedback.yaml, .github/workflows/claude_fix_ci_failure.yaml, .github/workflows/claude_fix_renovate_ci.yaml, .github/workflows/claude_resolve_conflict.yaml, .github/actions/agent-dispatch/action.yaml]
  triggers: [claude, coding-agent, plan-pr, claude-fix, workflow, github-actions, automation]
  lens: [conventions]
  skills: [issue]
  domain: infra
freshness:
  verified_commit: "31ab391d7"
  verified_on: "2026-06-09"
---
# Coding Agent Cheat Sheet

How to drive Claude through GitHub to plan, implement, and iterate on
fixes. The triggers live in `.github/workflows/claude_*.yaml`.

### Where the agent runs

The `claude_*` workflows here are thin **dispatchers** (#2970). They check the
trigger (trusted actor, phrase, labels) and start a run of `agent.yaml` in the
private agent repo, `vars.AGENT_REPO`, passing only ids. The agent validates
the request again, checks this repo out, and works on it as `ripls-agent[bot]`.
The Claude token and the agent's GitHub App key live only there; this repo
holds just the key that starts agent runs. `release_cut` dispatches the same
way (mode `release-cut`).

What that means day to day:

- **Agent runs and logs are in the agent repo's Actions tab**, not here. The
  dispatcher run here only shows "Dispatched <mode> for #N". Execution logs are
  `claude-*-log-*` artifacts on the agent run.
- **Refusals are also there.** If a trigger seemed to do nothing, look for a
  failed `validate` job on the agent run titled `agent <mode> <repo>#N`; its
  error says why (e.g. "PR moved since the request").
- The agent's commits, PRs and comments come from **`ripls-agent[bot]`**.

## Triggers at a glance

| Where you type it | What you type | Workflow that fires | Model |
|---|---|---|---|
| Issue body / title | `@claude ...` | `claude_plan_issue` | Opus |
| Issue comment | `@claude ...` | `claude_plan_issue` | Opus |
| Issue label | `claude-fix` | `claude_plan_issue` | Opus |
| **Plan PR** (label `claude-plan-pr`) — comment / review | `@claude <what to change>` | `claude_refine_plan` | Opus |
| **Plan PR** (label `claude-plan-pr`) — comment / review | `@claude implement` | `claude_implement_plan` | Sonnet |
| **Implemented PR** (label `claude-implemented`) — submit review | (no `@claude` needed) | `claude_address_pr_feedback` | default |
| **Code PR** comment / review | `@claude ...` (anything not `implement`) | `claude_address_pr_feedback` | default |
| any `claude/*` PR | (CI fails) | `claude_fix_ci_failure` | default |
| Renovate `renovate/*` PR | (CI fails) | `claude_fix_renovate_ci` | default |
| `claude/*` PRs | (push to main / new PR / hourly) | `claude_resolve_conflict` | default |
| merged changes touching doc-governed code | (nightly, ~2am US Central) | `refresh_docs` in the agent repo | default |

Self-triggered events are filtered out — Claude can't loop on its own
comments.

### Stage labels

The workflows route on three labels. They're created automatically the
first time the plan workflow runs:

| Label | Meaning | Set by | Cleared by |
|---|---|---|---|
| `claude-plan-pr` | Plan is being iterated on; refine fires for `@claude` comments | `claude_plan_issue` (when the draft plan PR opens) | `claude_implement_plan` (when implementation starts; restored if Sonnet bails) |
| `claude-implementing` | Sonnet is currently executing the plan; both refine and feedback skip | `claude_implement_plan` (pre-Sonnet) | `claude_implement_plan` (post-Sonnet, success or blocked) |
| `claude-implemented` | Implementation done; PR is in normal code-review territory, feedback owns `@claude` | `claude_implement_plan` (on success) | (manual; usually stays for the life of the PR) |

### Finding your PRs (assignee + review-request)

Every Claude PR — fast-path or plan-path — is **assigned to the human
who triggered the run** (the `claude-fix` labeler, the `@claude`
commenter, the issue opener who included `@claude` in the body, or the
manual dispatcher). The assignee is set on `gh pr create` and persists
through refine, implement, and feedback runs.

When `claude_implement_plan` succeeds and swaps the label to
`claude-implemented`, it also **requests review from the assignee** so
the PR shows up in their standard review queue.

This gives you two saved searches that unambiguously surface "PRs
waiting on me":

| State you care about | Saved search |
|---|---|
| Plan PRs awaiting my decision (iterate / `@claude implement`) | `is:pr is:open assignee:@me label:claude-plan-pr` |
| Implemented PRs awaiting my first-pass code review | `is:pr is:open review-requested:@me label:claude-implemented` |
| Anything I triggered that's still open | `is:pr is:open assignee:@me label:claude-plan-pr,claude-implementing,claude-implemented` |

**Two-stage review.** The original requester does a first-pass review,
then **manually re-requests review from a second reviewer** and removes
themselves: on the PR, click *Reviewers* → remove yourself, add the
second reviewer, then submit a comment-style review with "ready for
second review" so the next reviewer is notified. The assignee stays the
same (you still own the PR through merge). No workflow automates this
hand-off — it's a one-click human action that keeps the
`review-requested:@me` query meaningful for whichever pass you're in.

## Lifecycle of an issue

```
file an issue
        │
        ▼
@claude in body  │  add `claude-fix` label  │  manual workflow_dispatch
        │
        ▼
claude_plan_issue  (Opus triages)
        │
        ├──── fast path (typo / one-liner)
        │       └─► Opus implements, opens regular PR, comments on issue
        │
        └──── plan path (anything non-trivial)
                └─► Opus writes docs/issues/<n>-<slug>.md
                    opens DRAFT PR with markers + claude-plan-pr label
                    + assignee = the human who triggered this run
                    title:  "[plan] Fix #N: <summary>"
                    body:   ~500-word plan summary
                    posts a sticky reviewer-instructions comment
                    comments on the issue
                    STOPS
                            │
                            ▼   (label: claude-plan-pr)
                    [you iterate on the plan]
                       │
                       ├── @claude <what to change>      ──► claude_refine_plan (Opus)
                       │                                       └─► edits plan file, REWRITES PR description
                       │                                            (title summary too if scope changed),
                       │                                            pushes commit, comments
                       │                                            ↻ loop until satisfied
                       │
                       ├── @claude <question>            ──► claude_refine_plan (Opus)
                       │                                       └─► answers in a comment, no edit
                       │
                       └── (you edit the plan file by hand) ──► no agent runs
                                                                (PR description not auto-updated)
                            │
                            ▼
                    you comment `@claude implement` on the draft PR
                            │
                            ▼   (label: claude-plan-pr → claude-implementing)
                    claude_implement_plan  (Sonnet)
                            └─► reads plan file as authoritative
                                implements, runs tests, pushes commits
                                gh pr ready (drops draft)
                                REWRITES PR title (drops [plan] prefix)
                                  and body (implementation summary)
                                requests review from the assignee
                                  (original requester does first pass)
                                comments "ready for review"
                            │
                            ▼   (label: claude-implementing → claude-implemented)
                    [you review the code]
                       │
                       ├── submit review (commented / changes_requested)
                       │     ──► claude_address_pr_feedback (auto-address)
                       │           reads review body + every inline comment,
                       │           addresses each as a logical commit,
                       │           posts per-item summary
                       │
                       └── @claude <single request> on the PR
                            ──► claude_address_pr_feedback (single-request)
                                  pushes one focused follow-up commit
```

## Filing an issue for Claude

Three ways to start, ranked by ergonomics:

1. **Mention in the body.** When opening the issue, include `@claude`
   anywhere in the title or body. The plan workflow fires on
   `issues.opened`.
2. **Label after the fact.** Add the `claude-fix` label to an existing
   issue. Fires on `issues.labeled`.
3. **Comment.** Post a comment containing `@claude` on an open issue.
   Fires on `issue_comment.created`.
4. **Manual dispatch.** From the Actions tab, run **Claude Plan Issue**
   with the issue number as input. Useful for retries.

The issue body is the contract — Opus reads every comment too. Be
specific about acceptance criteria. If you have a preferred approach
or a "don't do X" constraint, say so.

### Re-trigger guard

If a draft plan PR is already open for the issue, the plan workflow
no-ops and posts a comment pointing at the existing PR. To force a
fresh plan, close the existing plan PR (or merge / abandon it) before
re-triggering.

## Reviewing a plan PR

The plan PR is a draft. It contains a single file:
`docs/issues/<issue-number>-<slug>.md`. The PR **description** carries
a ~500-word standalone summary of the plan (Problem / Approach /
Implementation outline / optional Risks) so a reviewer can sign off
from the PR landing page if the plan is clearly correct without ever
opening the file. Reviewer instructions ("how to iterate," "how to
implement") live in a separate sticky comment on the PR, not in the
description.

The first two lines of the description are HTML-comment markers —
leave them in place; the refine and implement workflows parse them:

```
<!-- claude-plan-pr: 123 -->
<!-- claude-plan-path: docs/issues/123-foo.md -->
```

The description **evolves** with the plan and after implementation:

- **At plan open**: ~500-word plan summary, `[plan]` prefix in the
  title.
- **After each refine**: Opus rewrites the description so the
  summary stays accurate. Title's summary phrase may change if scope
  shifted; the `[plan]` prefix stays.
- **After implementation**: Sonnet drops the `[plan]` prefix and
  replaces the plan summary with an implementation summary (What this
  PR does / Changes / Tests / link to the plan file kept on the branch
  for reference).

There are three ways to shape the plan before triggering
implementation. Pick whichever fits the change:

1. **Comment `@claude <what to change>`** on the PR — fires
   `claude_refine_plan` (Opus), which edits the plan file, rewrites
   the PR description, commits, pushes, and replies with a summary of
   what changed. Best for directives like "swap to option B," "rewrite
   step 3 to use a JOIN," "add an Open Question about caching." See
   [Iterating on the plan with Opus](#iterating-on-the-plan-with-opus)
   below.
2. **Edit the plan file by hand** — web editor (pencil icon on the
   plan file in the PR) or `gh pr checkout <num>` + local edit. No
   agent runs and the PR description is **not** updated; mention what
   you changed in a comment so the next refine has context.
3. **Leave inline review comments** — they're advisory only. Sonnet
   reads the file, not the comments. Use this to ask questions or
   note context for human reviewers.

The plan file's `## Recommendation` and `## Implementation Plan`
sections are what Sonnet executes against. If you want a different
approach than the one Opus recommended, rewrite those sections — via
agent or by hand. Whatever's in the file at implement time wins.

## Iterating on the plan with Opus

While the PR carries the `claude-plan-pr` label, any comment / review
mentioning `@claude` (without the literal phrase `implement`) routes
to `claude_refine_plan`. Examples:

- `@claude rewrite step 3 to use a single JOIN instead of looping over
  user IDs.` — directive, edits the plan, commits.
- `@claude split option B into two phases — schema migration first,
  then the API change.` — directive, edits the plan, commits.
- `@claude why did you pick option A over option C?` — question;
  Opus replies with a comment, doesn't edit the plan.
- `@claude what's the migration risk if we change the column type?` —
  question; Opus replies in a comment.

Mixed messages (question + directive) are answered in the same PR
comment that announces the edit. When in doubt about whether the
intent is a question or a directive, Opus errs toward replying with
a comment — a wrong edit is worse than a missing one. Re-ask
explicitly if you wanted the change.

A per-PR concurrency group serializes refine runs, so multiple
back-to-back `@claude` comments process in order. Refine **does not**
fire while `claude-implementing` is active (Sonnet has the lock).

## Triggering implementation

Comment `@claude implement` on the draft plan PR (or submit a review
with that text). The implement workflow:

- Verifies the PR has both plan markers and a `claude/*` head ref.
- Swaps the `claude-plan-pr` label for `claude-implementing` (so
  refine can't fire mid-implement) before invoking Sonnet.
- Checks out the head, runs `npm run generate:proto`, build_runner.
- Reads the **current** state of the plan file from the branch (so
  refine edits and your hand-edits both land).
- Runs Sonnet against the plan as the authoritative contract.
- On success: pushes commits, runs `gh pr ready` (drops draft state),
  **rewrites the PR title and description** (drops the `[plan]`
  prefix, replaces the plan summary with an implementation summary —
  What this PR does / Changes / Tests), swaps `claude-implementing`
  for `claude-implemented`, **requests review from the PR's assignee**
  (the original requester) so it lands in their `review-requested:@me`
  queue, comments on the PR.
- On blocked: drops `claude-implementing` and restores
  `claude-plan-pr` so refine can pick the iteration back up, leaves
  the PR as a draft with the plan-stage title and description intact,
  posts a "blocked" comment explaining what step it couldn't carry
  out.

If Sonnet bails, edit the plan (via refine or by hand) and trigger
`@claude implement` again, or take it over manually.

A per-PR concurrency group serializes implement runs, so spamming
`@claude implement` won't fan out parallel jobs against the same
branch.

## Iterating on a code PR

Once the PR is past the plan stage (label `claude-implemented`, or
it's a fast-path PR with no plan label), there are two ways to get
the feedback agent to push fixes:

### Just submit a review (auto-address, `claude-implemented` only)

On PRs Claude implemented, **submitting a normal GitHub review with
`commented` or `changes_requested`** auto-fires
`claude_address_pr_feedback`. No `@claude` mention needed. The agent
reads the review body and every inline comment that was part of the
review, builds a checklist, addresses each actionable item as a
logical commit, runs the affected test suites, and posts a per-item
summary comment that maps each comment to a SHA. Approved or
dismissed reviews skip — nothing to do.

This is the path you want for ordinary code review on Claude PRs.

### Mention `@claude` (any PR)

For ad-hoc requests, anywhere — including human-authored PRs that
have no Claude label — comment `@claude <what to change>`:

```
@claude please use a test fixture for the user instead of constructing one inline
```

The agent addresses just that single request, narrowly scoped — no
"while I'm here" cleanup. Works in:
- top-level PR comments (`issue_comment` on a PR)
- inline diff comments (`pull_request_review_comment`)
- review bodies (`pull_request_review`)

The feedback workflow is **excluded** from PRs carrying
`claude-plan-pr` or `claude-implementing` (those route to refine /
implement instead), and from any comment whose body contains
`@claude implement` (the implement workflow's exclusive trigger).

A workflow-level concurrency group keyed on PR number serializes
runs, so back-to-back reviews and comments process in order against
the same branch.

## CI auto-fix on `claude/*` PRs

If any of these test workflows fails on a `claude/*` PR head:

- Go Tests, Flutter Tests, Terraform Tests, Test iOS Build,
  Test Docker Build, AI Provider Integration Tests,
  Run End-to-End Tests on Dev

…then `claude_fix_ci_failure` fires automatically. It pulls the failed
job logs (`gh run view --log-failed`), reads the PR diff, diagnoses,
and pushes a fix commit + PR comment. If it can't (real product bug,
infra issue, ambiguous failure), it posts a "blocked" comment instead.

If the branch has advanced past the SHA the failed run targeted, the
workflow no-ops — assumes someone is already addressing it.

To dispatch manually: Actions → **Claude Fix CI Failure** →
**Run workflow**, supply the PR number and (optionally) a failed run
ID.

## Auto merge-conflict resolution

`claude_resolve_conflict` keeps `claude/*` PRs caught up to `main`. It
fires on:

- push to `main`
- `pull_request` events (opened / synchronize / reopened /
  ready_for_review) for `claude/*` head branches
- hourly cron (safety net for GitHub's lazy mergeability recompute)
- manual dispatch

For each conflicting `claude/*` PR:

1. **Mechanical merge first.** Tries `git merge --no-ff origin/main`.
   If clean, pushes and comments "Caught up to `main` via clean merge."
2. **Escalates on conflict.** If the merge produces conflicts, captures
   the conflicting paths, dispatches Claude to resolve them, run
   affected tests, commit, push, and comment.
3. **Rebase fallback.** If a merge truly tangles, Claude may rebase
   and force-push-with-lease — only as a fallback, and it'll say so in
   the PR comment.

To dispatch manually: Actions → **Claude Resolve PR Conflicts** →
**Run workflow**, optionally supplying a PR number to limit the scan.

## Doc-freshness ratchet

The agent repo's `refresh_docs` workflow keeps this repo's durable docs honest
as code changes (the #2420 ratchet; `vars.AGENT_DOCS_TARGET` there names this
repo). It runs **nightly** (08:00 UTC ≈ 2am US Central) over everything
merged to main since its last successful run — no `@claude` needed:

1. Anchors the diff window at the `doc-freshness-verified` tag — the main SHA
   docs were last verified against (first run ever falls back to a 26-hour
   lookback) — then computes the **affected docs**: durable docs whose
   `context: globs` match a file the window changed, via `node
   scripts/doc_freshness.js affected --base <tag>`. If none, the run is a
   no-op.
2. Runs the `ripls-doc-freshness` skill in **apply mode** on just those docs:
   verifies each doc's prose against the changed code, fixes genuine drift, and
   advances each edited doc's `freshness:` stamp.
3. **Opens a single docs-only PR** (branch `claude/doc-freshness-YYYYMMDD`,
   `git add docs/` only) summarizing per-doc drift and fixes. A clean night
   makes no commit and no PR. It **enables auto-merge** on the PR, so it lands
   once required checks pass. Every successful run advances the
   `doc-freshness-verified` tag, so a failed night is simply covered by the
   next night's window.

Loop-safe structurally (it reads main and writes to its own `claude/*` branch;
a schedule can't re-trigger itself) and security-scoped: a **docs-only**
`--allowedTools` surface, so a prompt-injection in changed code can't make it
edit code or run arbitrary commands. Dependency-bump merges that only move
manifest pins match broad config globs but are confirm-fresh fast paths, not
prose drift. To force a run: the agent repo's Actions → **Refresh Docs** →
**Run workflow**, optionally overriding the diff base. The freshness stamp + skill
are documented in
`docs/context_map.md` → *Prose freshness* and `.claude/skills/ripls-doc-freshness/`.

## Manual dispatch summary

All workflows expose `workflow_dispatch` for retries and forced runs:

| Workflow | Required inputs | Optional inputs |
|---|---|---|
| Claude Plan Issue | `issue_number` | — |
| Claude Refine Plan | `pr_number`, `instruction` | — |
| Claude Implement Plan | `pr_number` (must be a plan PR) | — |
| Claude Address PR Feedback | `pr_number`, `instruction` | — |
| Claude Fix CI Failure | `pr_number` | `failed_run_id` |
| Claude Fix Renovate CI Failure | `pr_number` | `failed_run_id` |
| Claude Resolve PR Conflicts | `pr_number` | — |
| Refresh Docs (agent repo) | — | `base` (diff-base ref override) |
| Cut Release | `bump` (`fix` / `minor` / `major`) | — |

## Troubleshooting

- **Workflow ran but Claude said nothing.** Check the agent run in the
  agent repo. A failed `validate` job means the request was refused (its
  error says why). Otherwise each mode ends with a deliverable assertion:
  pushed commit *or* comment within 30 minutes. If it didn't produce one,
  the run fails loudly and uploads the Claude execution log as a
  `claude-*-log-*` artifact on the agent run, with the full prompt and tool
  transcript.
- **"PR moved since the request" on implement.** Someone pushed to the plan
  PR between `@claude implement` and the agent starting. The agent only
  implements the plan as it stood when you asked; re-comment
  `@claude implement`.
- **Implement workflow no-ops on a regular PR.** Expected — only PRs
  whose body carries `<!-- claude-plan-pr: N -->` and
  `<!-- claude-plan-path: ... -->` markers are recognized as plan PRs.
  Use `@claude` (without `implement`) for feedback on regular PRs.
- **Plan workflow refuses with "plan PR already open."** Close the
  existing plan PR first. The guard is matched against the
  `claude-plan-pr: N` marker in any open PR's body.
- **Sonnet posted a "blocked" comment.** Read what it tried; the
  workflow restored the `claude-plan-pr` label so refine is back in
  charge. Tell Opus what to fix (`@claude rework step 4 to use X`)
  or edit the plan by hand, then re-trigger `@claude implement`.
- **Refine edited the plan when I just wanted an answer.** Opus
  defaults to "reply with a comment" on ambiguous messages, but it
  occasionally guesses wrong. Revert the commit and re-ask
  explicitly (`question only — please don't change the plan, just
  explain ...`).
- **Stuck label state on a plan PR.** The implement workflow has a
  reconciler step that runs `if: always()` — if you see both
  `claude-plan-pr` and `claude-implementing` set after a run, that
  reconciler should have cleaned it up. Manually clear with
  `gh pr edit <num> --remove-label claude-implementing` if not.
- **CI fix or conflict resolve pushed something weird.** Both scope strictly
  to `claude/*` head branches (fix-renovate to Renovate's `renovate/*`
  branches); only address, which you trigger yourself, works on
  human-authored PRs. Revert the offending commit and post `@claude
  <correction>` on the PR.

## Hard rules the agents follow

- No `Co-Authored-By` / `Generated with Claude` branding in commits or
  PR bodies.
- Do not merge PRs (always a human's call).
- Do not modify the PR title or markers.
- Stay scoped to the request — no "while I'm here" cleanup.
- CLAUDE.md is authoritative for code conventions (proto rules,
  structured logging, fail-fast errors, SQL efficiency, Flutter
  structure, etc.).

## Workflow files

| File | Purpose |
|---|---|
| `.github/workflows/claude_plan_issue.yaml` | Dispatch plan: Opus triages an issue → fast path or draft plan PR |
| `.github/workflows/claude_refine_plan.yaml` | Dispatch refine: Opus edits the plan file in response to `@claude` on a `claude-plan-pr` PR |
| `.github/workflows/claude_implement_plan.yaml` | Dispatch implement: Sonnet executes the plan file; stage labels |
| `.github/workflows/claude_address_pr_feedback.yaml` | Dispatch address: `@claude` feedback on code PRs |
| `.github/workflows/claude_fix_ci_failure.yaml` | Dispatch fix-ci: failed CI on `claude/*` PRs |
| `.github/workflows/claude_fix_renovate_ci.yaml` | Hold gate + dispatch fix-renovate: failed CI on Renovate PRs |
| `.github/workflows/claude_resolve_conflict.yaml` | Scan for conflicting `claude/*` PRs + dispatch resolve |
| `.github/actions/agent-dispatch/action.yaml` | Shared mint-token-and-dispatch step |

The agents themselves (prompts, validation, the mode jobs and the nightly
`refresh_docs`) live in the private agent repo; see its README.
