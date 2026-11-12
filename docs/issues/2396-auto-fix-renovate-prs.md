# Issue #2396: Auto-fix failing Renovate dependency PRs with Claude (extend claude_fix_ci_failure to renovate/* branches)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2396
**Filed:** 2026-06-07 | **Last activity:** 2026-06-07
**Labels:** enhancement, review:dependency | **Assignees:** unassigned
**Context docs:** none provided (read during research: `docs/dependency_automation.md` — at research time this was two separate files, `docs/dependency_modernization.md` + `docs/toolchain_automation.md`, since merged by PR #2397; all references below point to the merged doc — plus `renovate.json`, `.github/workflows/claude_fix_ci_failure.yaml`, `.github/workflows/renovate_auto_approve.yaml`)

## Problem Statement

Major dependency bumps opened by Renovate routinely fail CI because the
codebase needs a code migration (a renamed Go API, a Terraform resource
rename, a Gradle/Kotlin/TypeScript major). Today those PRs sit red until a
human does the migration by hand. We already have an established
plan→implement→fix automation loop driven by Claude
(`claude_plan_issue` → `claude_implement_plan` → `claude_fix_ci_failure` →
`claude_address_pr_feedback`), and `claude_fix_ci_failure` does almost exactly
the work we want — diagnose a failed required check and push a fix — but it is
**gated to `claude/*` branches only**
(`startsWith(github.event.workflow_run.head_branch, 'claude/')`).

The ask: when a `renovate/*` PR's required checks fail because the codebase
needs migrating, have Claude check out the branch, diagnose, apply the
migration, push, and comment — instead of leaving the PR red for a human. The
migrated PR must still pass all required checks (including the now-required
`test-build` Docker image build) before the normal auto-approve/auto-merge
path applies; there is no special path to `main`.

The genuinely hard parts (the reasons this isn't a one-line `if` change) are
all about **not making things worse**:

1. **Branch ownership conflict.** Renovate owns and rebases `renovate/*`
   branches. A Claude push could be clobbered by a later Renovate rebase.
2. **Distinguishing "needs migration" from "intentionally held."** Several red
   PRs are deliberate caps (`invopop/jsonschema <0.14`, `onnxruntime_go <1.25`,
   the freezed/analyzer cap), not migrations. Claude must recognize a
   documented hold and **skip/comment** rather than tear out the cap.
3. **Auto-approve interaction.** A Claude-modified Renovate PR must not slip
   through the auto-approve gate unvalidated, nor get permanently stuck.

## Context from Comments

One comment, from the issue author (maintainer): this is one of a pair of
follow-ups to the self-hosted Renovate cutover (#2359). The sibling, **#2395**,
automates the `needs runner rebuild` baked-tool PRs (candidate-image
validation + auto-merge + auto-recreate). This issue (#2396) is the
**code-migration** half; #2395 is the **toolchain-rebuild** half. They share
infrastructure (the Renovate cutover, the auto-approve gate, the required-check
set) but are independent deliverables.

The discussion is thin — only the cross-link comment exists. The issue body
itself is detailed and already enumerates the design considerations, so the
sparse comment thread is not a blocker; the body carries the requirements.

## Research findings (codebase as-built)

What exists today that this builds on:

- **`claude_fix_ci_failure.yaml`** — `workflow_run`-triggered on the failed
  required workflows (`Go Tests`, `Flutter Tests`, `Terraform Tests`,
  `Test iOS Build`, `Test Docker Build`, `AI Provider Integration Tests`,
  `Run End-to-End Tests on Dev`). It has a `discover` job that resolves the PR
  from the head branch and **gates to `claude/*`** in two places: the
  job-level `if` (line 37-41) and a defensive `case "$HEAD_REF"` re-check
  (line 91-98). The `fix` job checks out the PR head with
  `secrets.CLAUDE_WORKFLOW_PAT`, has a **stale-branch guard** (skip if the
  branch advanced past the failed SHA), runs codegen, captures failed logs +
  PR JSON/diff, runs `claude-code-action@v1` with a detailed migration prompt,
  and asserts a deliverable (a new commit **or** a PR comment). This is the
  template to extend.

- **`renovate.json`** — the source of truth for holds. Caps live as
  `allowedVersions` (`onnxruntime_go <1.25.0`, `microsoft/onnxruntime <1.23.0`,
  `invopop/jsonschema <0.14.0`, `@types/node <23.0.0`) and as
  `"enabled": false` blocks (Dart `protobuf`, `mockito`/`analyzer`,
  `mapbox_maps_flutter`, `dev.flutter.*`). Auto-merge eligibility is the
  `automerge` label (added by the auto-merge `packageRule`); majors carry **no**
  label.

- **`renovate_auto_approve.yaml`** — `pull_request_target`, gated to the
  Renovate bot logins (`renovate[bot]`, `ripls-renovate[bot]`) **AND** the
  `automerge` label **AND NOT** `needs runner rebuild`. It approves so native
  auto-merge fires on green. **Majors get no `automerge` label**, so a major
  migration PR is *not* auto-approved — a human reviews and merges it. This is
  the desired end-state and the plan preserves it: Claude fixing a *major*
  Renovate PR turns it green but it still waits for a human; only the
  already-eligible patch/minor PRs (which carry the label and already have
  native auto-merge enabled by Renovate) auto-merge once Claude clears the
  broken prerequisite. The fix workflow does **not** add the `automerge` label
  or enable auto-merge itself — it relies entirely on the existing wiring, so
  the per-update-type policy is unchanged.

- **`docs/dependency_automation.md`** — prose rationale for every held dep,
  with the exact failure each hold prevents (e.g. "`anthropic-sdk-go v1.48`
  fails because it transitively forces jsonschema 0.14"). This is the document
  Claude must read before deciding a failure is "fixable."

- **Branch ownership** — `renovate.json` does **not** set `rebaseWhen`, so it
  uses Renovate's default. The default (`auto`) resolves to **`conflicted`**
  for a branch that has received non-Renovate commits — i.e. Renovate stops
  auto-rebasing a branch once someone else pushes to it, unless it goes
  conflicted or the PR body's "rebase checkbox" is ticked. This is the behavior
  open-question #1 option (a) depends on; it needs an explicit
  `"rebaseWhen": "conflicted"` (or `"never"`) to be *guaranteed* rather than
  relying on the default.

- **Auth** — the `fix` job already checks out with
  `secrets.CLAUDE_WORKFLOW_PAT` and pushes with it. A PAT (not the default
  `GITHUB_TOKEN`) can push to a branch authored by the Renovate App, so the
  push-auth question is largely already solved by the existing template; it
  needs verification, not new machinery.

## Potential Solutions

### Option A: Push directly to the `renovate/*` branch (extend the gate in place)

- **Approach:** Add `renovate/*` to the `discover` gate in
  `claude_fix_ci_failure.yaml` (or a near-clone sibling workflow). Before
  invoking Claude, run a **hold-check pre-gate** that parses `renovate.json`
  (`allowedVersions` + `enabled:false`) and `docs/dependency_automation.md`
  for the dep(s) this PR bumps; if the dep is held, comment-and-skip instead of
  fixing. Otherwise Claude checks out the branch, migrates, pushes a commit to
  `renovate/<dep>`, and comments. Set `"rebaseWhen": "conflicted"` in
  `renovate.json` so Renovate stops rebasing the branch once Claude pushes.
- **Pros:** Smallest diff; reuses the entire existing `fix` job verbatim
  (checkout, codegen, log capture, deliverable assertion, postgres cleanup).
  The PR stays the same PR — Renovate's dashboard entry and the existing
  auto-approve/auto-merge wiring keep applying with **no new merge machinery**.
  An already-eligible patch/minor (it carries the `automerge` label, Renovate
  enabled native auto-merge on it, and `renovate_auto_approve` approved it — it's
  only red because of the broken prerequisite) simply merges once Claude's fix
  turns it green. Majors (no label, no native auto-merge) stay waiting for a
  human, exactly as today. The per-update-type policy is preserved for free.
- **Cons:** Relies on `rebaseWhen: conflicted` actually holding. If Renovate
  ever rebases anyway (config drift, a manual rebase-checkbox tick, a
  conflict), Claude's commit is clobbered and the cycle repeats — wasteful but
  not unsafe (the migration just re-runs). The `renovate[bot]` PR now has a
  `claude[bot]` commit on it; the auto-approve gate must be confirmed to still
  behave (it gates on **PR author** = Renovate bot, which is unchanged by a
  co-author commit — so it still works, but this needs an explicit test).
- **Effort:** Small–medium.

### Option B: Claude opens a separate `claude/fix-<dep>` PR that supersedes the Renovate one

- **Approach:** On a fixable failed `renovate/*` PR, Claude branches
  `claude/fix-<dep>` *from* the Renovate head, applies the migration, opens a
  **new** PR, and closes the Renovate PR with a cross-link. The new PR flows
  through the normal `claude/*` automation (including `claude_fix_ci_failure`
  itself, which already handles `claude/*`).
- **Pros:** Zero branch-ownership conflict — Renovate never touches
  `claude/fix-*`. The new PR is a first-class `claude/*` PR, so the *entire*
  existing fix/feedback loop applies with no new gating. Clean provenance: the
  dependency bump + migration are one human-reviewable PR authored by the
  automation, not a bot PR with mixed authorship.
- **Cons:** Larger machinery — Claude must reconstruct the dep bump
  (`go.mod`/`pubspec`/lockfile changes) on the new branch, which means either
  cherry-picking Renovate's commit or re-deriving it; the `postUpgradeTask`
  go.sum regen that Renovate runs (`renovate_gomod_regen.sh`) must be
  replicated. Closing/reopening churn; Renovate may re-open a *new*
  `renovate/<dep>` PR for the same bump since its branch is now "unmerged,"
  creating duplicates unless suppressed. Loses Renovate's native auto-merge
  wiring (the new PR needs the `automerge` label applied deliberately, or stays
  manual). More moving parts, more failure modes.
- **Effort:** Large.

### Option C: Hold-detection only — comment, never auto-migrate (assistive, not autonomous)

- **Approach:** A lightweight workflow that, on a failed `renovate/*` PR,
  classifies the failure (held cap vs. needs-migration vs. flake) and posts a
  **triage comment** — "this needs a Go API migration, here's the breaking
  change" or "this is a documented hold, close it" — but does **not** push a
  fix. A human (or a follow-up `@claude implement`) does the migration.
- **Pros:** Lowest risk; no branch-ownership problem at all; cheap; surfaces
  the held-vs-migration distinction (the genuinely useful triage) without ever
  modifying a bot branch. A good **Phase 1** regardless of the eventual target.
- **Cons:** Doesn't meet the issue's actual goal (auto-*fix*). Still leaves the
  human to migrate. Half a solution on its own.
- **Effort:** Small.

## Recommendation

**Go straight to Option A as a single deliverable: extend `claude_fix_ci_failure`
to act on any failed `renovate/*` PR, and auto-merge the ones Claude resolves.**
No standalone comment-only phase. Option B (superseding PR) is rejected for the
reasons in its Cons — it re-implements Renovate's go.sum regen and auto-merge
wiring and invites duplicate-PR churn to dodge a problem `rebaseWhen: conflicted`
solves directly.

Two design points that this decision pins down:

- **The hold pre-gate is NOT cut — it becomes an inline hard gate inside Option
  A.** Dropping the standalone classifier phase means we skip shipping a
  comment-only workflow first; it does **not** mean Claude is turned loose
  without hold detection. The single worst failure mode is Claude "fixing" a
  deliberate cap by deleting it (ripping out `allowedVersions: <0.14` to make
  `anthropic-sdk-go` resolve, re-breaking dotprompt). So before the
  write-enabled Claude step runs, the workflow checks the PR's dep(s) against
  the holds in `renovate.json` + `docs/dependency_automation.md`; a match is
  a hard stop → comment-and-skip, never migrate. The migration prompt also
  forbids removing a documented cap, as a second layer.

- **Auto-merge is NOT extended — the existing per-update-type policy is
  preserved and requires no new merge machinery.** The goal is narrow: an
  *already-automerge-eligible* patch/minor PR that is red only because of a
  broken prerequisite should merge once Claude clears it. That PR already
  carries the `automerge` label, Renovate already enabled GitHub native
  auto-merge on it (`platformAutomerge`), and `renovate_auto_approve` already
  approved it — so the moment Claude's fix turns it green, the existing wiring
  merges it on the required-check set with no further action. **Majors are left
  manual:** they carry no `automerge` label and no native auto-merge, so
  Claude's fix turns them green but a human still reviews and merges. The fix
  workflow therefore must **not** add the `automerge` label and must **not** call
  `gh pr merge --auto` — doing either would (incorrectly) auto-land majors. The
  only mechanic to confirm is that Claude's push fires `renovate_auto_approve`'s
  `synchronize` trigger so any push-dismissed approval is re-applied (its gates
  — Renovate-bot author + `automerge` label — are unchanged by a Claude commit).

## Open Questions

- **Does `rebaseWhen` reliably stop Renovate rebasing a branch with a Claude
  commit?** — Recommendation: set `"rebaseWhen": "conflicted"` explicitly in
  `renovate.json` (don't rely on the default) and verify on one live PR that a
  Claude push survives the next Renovate run. Enabling native auto-merge on a
  successful fix also shrinks the clobber window — a green migrated PR merges
  promptly rather than sitting for Renovate's next cron. If `conflicted` still
  proves flaky on a genuine conflict, narrow to Option B *only* for the deps
  that conflict, not wholesale.

- **How does the workflow identify which dep(s) a `renovate/*` PR bumps, to
  cross-check against the holds list?** — Recommendation: parse the PR title
  (Renovate titles are structured: "Update module github.com/... to v88") and
  the diff of `go.mod`/`pubspec.yaml`/`*.tf`/`renovate.json`; match package
  names against `allowedVersions` keys and `enabled:false` `matchPackageNames`
  in `renovate.json`. Any match → hold → comment-and-skip. Feed
  `docs/dependency_automation.md` to Claude as required reading so prose-only
  holds (e.g. the eslint-flat-config blocker) are caught too.

- **Does an already-eligible patch/minor actually auto-merge after Claude's
  push, and do majors correctly stay manual?** — Recommendation: confirm on one
  live patch/minor that Claude's commit fires `renovate_auto_approve`'s
  `synchronize` trigger, the github-actions[bot] approval is (re-)applied even
  with a claude[bot] commit on the branch — covering the case where the
  ruleset's stale-review dismissal drops the prior approval on the new push —
  and native auto-merge (already enabled by Renovate) merges on green. Then
  confirm a Claude-fixed *major* goes green but does **not** merge (no label, no
  native auto-merge). The workflow adds no label and calls no merge command, so
  this should fall out of the existing wiring — but it's the behavior that lands
  code to `main`, so exercise both cases before trusting it broadly.

- **Trigger scope** — *Decided:* trigger on **any** failed required check for a
  `renovate/*` PR (not majors-only). The inline hold-gate + the existing
  flake/infra detection in the prompt do the filtering; a failing patch/minor
  that's a genuine migration gets fixed too.

- **Rate-limiting / cost control.** — Recommendation: a `concurrency` group
  keyed on PR number (as `claude_implement_plan` does) so a Monday burst of
  Renovate PRs can't fan out parallel runs on the same PR; gate on the
  `dependencies` label (all Renovate PRs carry it); and skip if a prior Claude
  fix comment already exists on the PR so it doesn't loop on the same PR more
  than once or twice.

## Implementation Plan

### Phase 1: Auto-migrate failed renovate/* PRs with an inline hold-gate (Option A)

**Goal:** On any failed required check for a `renovate/*` PR, the workflow
checks the dep(s) against the holds list; if not held, Claude checks out the
branch, applies the migration, pushes, and comments. If held (or a flake/infra
failure), it comments and skips — never modifying the cap.
**Tasks:**
- [ ] Set `"rebaseWhen": "conflicted"` in `renovate.json`; verify on one live
      PR that a non-Renovate push survives the next Renovate run.
- [ ] Extend the `discover` gate in `claude_fix_ci_failure.yaml` (or a sibling)
      to allow `renovate/*` in addition to `claude/*`; leave the `claude/*`
      path untouched. Gate on the Renovate bot author + the `dependencies`
      label.
- [ ] Build the **inline hold pre-gate** as a hard stop *before* the
      write-enabled Claude step: parse `renovate.json` (`allowedVersions` keys,
      `enabled:false` `matchPackageNames`) + the PR's dep list (title + manifest
      diff); on a match, comment-and-skip.
- [ ] Reuse the existing `fix` job (PAT checkout, codegen, log capture,
      deliverable assertion, postgres cleanup) with a Renovate-tailored prompt
      that hands Claude `renovate.json` + `docs/dependency_automation.md`,
      names the held-deps rule, and **forbids removing a documented cap** as a
      second layer behind the pre-gate.
- [ ] Add a `concurrency` group keyed on PR number and a "skip if a prior Claude
      fix comment exists" guard to prevent loops/fan-out.
- [ ] Confirm the PAT can push to a `ripls-renovate[bot]`-authored branch.
**Validates:** End-to-end on one genuine migration PR (e.g. `mailgun-go v4→v5`
or `go-github`) AND one held PR (e.g. a `jsonschema 0.14`-forcing bump): the
migration PR gets a green Claude commit that survives Renovate's next run; the
held PR gets a skip comment with its cap intact.

### Phase 2: Verify the existing auto-merge wiring carries Claude's fix (no new code)

**Goal:** Confirm an already-eligible patch/minor merges once Claude clears its
broken prerequisite, and that majors correctly stay manual — using the existing
auto-approve/auto-merge wiring with **no** label or merge command added by the
fix workflow.
**Tasks:**
- [ ] On a live automerge-eligible patch/minor that's red on a prerequisite:
      confirm Claude's push fires `renovate_auto_approve`'s `synchronize`
      trigger and the approval is (re-)applied after the new commit (covers
      ruleset stale-review dismissal on push).
- [ ] Confirm native auto-merge (already enabled by Renovate via
      `platformAutomerge`) merges the PR once it's green on the full required-
      check set, incl. `test-build` — with no `gh pr merge --auto` call from this
      workflow.
- [ ] Confirm a Claude-fixed **major** goes green but does **not** merge (it has
      no `automerge` label and no native auto-merge) — i.e. the fix workflow
      adds no label and runs no merge command, preserving majors-are-manual.
**Validates:** Eligible patch/minor lands hands-off after a Claude fix; majors
go green but wait for a human; the required-check set is the sole gate; no new
merge machinery was introduced.

### Phase 3: Documentation + hardening

**Goal:** Capture the new policy and the non-obvious mechanics so the next
person doesn't re-derive them, and harden the rate-limit/loop guards.
**Tasks:**
- [ ] Document the end-to-end flow in `docs/dependency_automation.md`: failed
      `renovate/*` PR → hold-gate → (held: skip-comment, cap intact) / (migration:
      Claude fixes + pushes) → the **existing** auto-approve/auto-merge wiring
      lands an already-eligible patch/minor on green, while a major goes green
      and waits for a human. Reference it from `docs/dependency_automation.md`.
- [ ] Record in the toolchain doc's Gotchas section: why the fix workflow
      deliberately adds **no** `automerge` label and runs **no** merge command
      (doing either would auto-land majors, which we don't want); that Claude's
      push must re-trigger `renovate_auto_approve` via `synchronize`; and the
      `rebaseWhen: conflicted` rationale.
- [ ] Confirm the rate-limit guards (PR-number concurrency, prior-comment skip,
      `dependencies`-label gate) hold under a multi-PR Monday burst.
**Validates:** The full policy is documented and holds: no migration bypasses
required checks; eligible patch/minor auto-merge on green while majors stay
manual; held
caps are never torn out; no runaway fan-out on a Renovate burst.

## Audit Notes (ripls-audit, 2026-06-07)

This plan was audited against the standard architecture reference set
(`docs/client/{architecture,caching,i18n,accessibility,testing}.md`,
`docs/server/{architecture,observability}.md`). **None of those surfaces are in
scope:** the deliverable is GitHub Actions workflows, shell, and `renovate.json`
— no Flutter client code, no Go service code, no RPCs/protos, no client cache,
no user-facing strings (so the i18n and "Event"-not-"Experience" rules don't
apply), no accessibility primitives, no ViewModel tests, and no server `slog`
logging. The architectural reference docs therefore raise no findings here.

The plan *was* checked against the doc that does govern it,
`docs/dependency_automation.md`, and is consistent with its as-built facts:
- Auto-approve gates on **PR author = Renovate bot + `automerge` label** (not
  commit author), per §4. The plan **preserves** this unchanged: it adds no
  label and runs no merge command, so an already-eligible patch/minor merges via
  the existing wiring once Claude's fix turns it green, while majors (no label)
  go green and wait for a human — exactly today's per-update-type policy. The
  only mechanic relied on is that Claude's push re-fires `renovate_auto_approve`
  via `synchronize`.
- Required-check / job-gating policy ("no special path to `main`") matches §3 —
  a Claude-resolved PR still waits on every required check, including
  `test-build`, before the existing auto-merge fires.
- The `rebaseWhen` gap is correctly flagged as unverified and pushed to an
  open question + a Phase-2 task rather than asserted — appropriate, since the
  repo's `renovate.json` does not set it today.
- The go.sum `postUpgradeTask` complication (a reason Option B is heavier) is
  real (renovate.json gomod `postUpgradeTask` + §1 Gotchas).

One convention reminder carried into the implementation phases (not a plan
defect, a guardrail for the code): any **new `.github/workflows/*.yaml`** file
must use the `.yaml` extension (not `.yml`) per CLAUDE.md, and any shell that
posts comments must not add Claude branding/co-authorship — the existing
`claude_fix_ci_failure.yaml` prompt already encodes the no-branding rule, so an
in-place extension (Option A) inherits it.

No contradictions with existing architecture decisions were found. The plan is
cleared to proceed.

## Related Issues
- **#2395** — sibling follow-up: automates the `needs runner rebuild`
  baked-tool Renovate PRs (candidate-image validation, auto-merge,
  auto-recreate). The toolchain-rebuild half to this issue's code-migration
  half; shares the Renovate cutover and auto-approve infra.
- **#2359** — the self-hosted Renovate cutover both #2395 and #2396 extend.
- **#2287** — the toolchain/dependency automation epic (as-built in
  `docs/dependency_automation.md`).
- **#2286** — example of a dependency-surfaced migration done by hand (the
  `x/net` h2c → `http.Server.Protocols` move) — the kind of work this automates.
