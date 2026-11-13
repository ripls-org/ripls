# Issue #3228: [OSS-17] Move to one public repo: publish deploy and infra config, move private docs and tracker out

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/3228
**Filed:** 2026-10-31 | **Last activity:** 2026-10-31
**Labels:** open-source | **Assignees:** unassigned
**Context docs:** `docs/issues/2953-open-source-release.md` (epic plan), `docs/secrets.md`,
`docs/release/automation.md`, `docs/release/cheat_sheet.md`, `docs/context_map.md`,
`docs/reviews/security_review_prompt.md` (Areas to Review: secrets management). Side
anchors (`client/*`, `server/*`) not loaded: this issue changes repo layout, CI and
tooling, not product code.

## Problem Statement

Today the private repo (`Type2Fun-Foundation/ripls`) is the source of truth.
`npm run oss:sync` filters it through `oss_manifest.txt` (allowlist) and
`scripts/oss_denylist.txt` (content gate), then pushes one "OSS Export" commit per
sync to `ripls-org/ripls`. #3228 ends that arrangement: everything publishable moves
into the public repo, development moves there, and this repo is archived (#3229).

To get there, every one of the 1,335 currently excluded files needs an outcome
(publish, move to the notes or website repo, or delete). The export gate becomes a
CI gate on the working tree. The tracker splits into public and private. Release and
deploy then switch to the public repo.

The issue was filed on 2026-10-31. Research against the tree on 2026-11-02 changes
the scope in five places.

1. **Release build numbers would go backward. This blocks the cut-over.**
   - Both `release_app.yaml:87` and `app/fastlane/Fastfile:23` set the build number
     to `git rev-list --count HEAD`.
   - That count is 10,246 here and **7** in the public repo.
   - Play rejects any `versionCode` that isn't higher than the last one uploaded.
   - The issue's main validation, "one release cut from the public repo", would
     fail at upload.
2. **The tracker split is mostly done already.**
   - 41 issues were closed as not planned on 2026-10-31 and 2026-11-02.
   - Nine issues are open. Three are open-source issues (#2953, #3228, #3229).
   - Five are product issues: #3213, #3079, #2952, #2864, #2792.
   - One is `user-feedback`: #2760.
   - "Refile 30, move 18" is now "refile 5, move 1".
3. **The TODO renumbering is mostly cleanup, not renumbering.**
   - Outside the planning docs, 19 distinct `TODO(#NNNN)` refs remain.
   - Only #2864 and #2792 point at open issues.
   - #1234 and #42 are illustrative examples in `CLAUDE.md`.
   - The other 15 point at **closed** issues, which the TODO rule in `CLAUDE.md`
     calls "worse than no TODO".
   - They need a decision each: resolve, refile, or delete. A mechanical
     renumbering doesn't fit.
4. **Moving the docs breaks links.**
   - 184 files that stay public link to 114 distinct docs that move: 62 in
     `docs/issues/`, 24 in `docs/ai/`, 15 in `docs/concepts/`, 11 review
     reports, and 2 others.
   - The files that become public also link into `docs/private/`. These are
     runners, renovate, terraform environments, `infra/server` and
     walkthrough-export: 10 files outside `docs/issues/`.
   - Several of those private docs are runbooks for exactly the infra being
     published: `infra_mimir.md`, `terraform_deployment.md`,
     `dependency_automation.md`, `secret-rotation.md` and the release
     checklists.
   - Under decision 1 they are publishable. The issue's blanket "`docs/private/` →
     notes repo" is too coarse.
5. **There is more GitHub coupling than the issue lists.**
   - The agent's plan mode writes `docs/issues/<n>-*.md`, and
     `validate_request.sh:253` requires that path. So `docs/issues/` must still
     exist in the public repo, even with the old files moved out.
   - `ripls-triage` reads `docs/private/users/{priorities,personas}.md`, but the
     skill itself is being published.
   - `cloud_functions/` is the alert-to-issue automation. It files GitHub issues
     through Octokit and a GitHub App. That answers the issue's question "why
     excluded?": it was grouped with deploy because it is deployed. Nothing in
     it is secret.
   - `.gitleaks.toml` allowlists `^docs/private/`.
   - `gen_context_map.js` excludes subtrees that will no longer exist.

**The open dependency is still open.** The issue says to do the Forgejo migration
first or in the same cut-over. The Forgejo port of the Claude workflows has not been
decided yet: a one-day spike was proposed. Most of this plan doesn't depend on that
decision. Phase 4 does.

## Context from Comments

There are no comments. The issue was written straight after the 2026-10-31
decision session. The direction comes from the epic plan
(`docs/issues/2953-open-source-release.md`, status block of 2026-10-31) and these
three decisions:

1. Account identifiers are not private.
2. The website gets its own repo.
3. The old history is archived, not published.

Two closures sit around this issue:

- #2971, the deploy-repo split, was closed as superseded.
- #2954, credential rotation, was closed as not planned, so this plan assumes no
  rotation.

## Potential Solutions

### Option A: Prepare here, then flip in one cut-over (the issue's plan)

- **Approach:**
  1. In this repo, make every change that turns an excluded path into a
     publishable one, a moved one or a deleted one.
  2. Shrink the manifest's excludes as each group is done. Each `oss:sync` then
     publishes more.
  3. When the excludes cover only moved or deleted items, make a final sync.
  4. Point release, deploy and agents at the public repo and archive this one.
- **Pros:**
  - Every step is gated: the export plus `oss_scan` checks each widening before
    it reaches the public repo.
  - The work can be stopped at any point.
  - The public history stays append-only (no force-push).
- **Cons:**
  - Every change is made twice, once here and once in the sync.
  - Until the flip, the public repo receives one large commit per sync, not
    reviewable history.
- **Effort:** large, but in small, mostly mechanical pieces.

### Option B: Flip first, then sort the private set in the public repo

- **Approach:**
  1. Move development to `ripls-org/ripls` now.
  2. Keep the excluded files here as an overlay until each group is sorted.
- **Pros:**
  - The double-handling of the sync ends immediately.
- **Cons:**
  - Rebuilds the two-repo coupling that #2971 was closed to avoid.
  - Release and deploy keep running from here and would need source from there,
    which is the subtree/pin design again.
  - The flip happens on GitHub, so deploy secrets and self-hosted runners would
    sit on a forkable repo. The issue's own open dependency warns against
    exactly that.
- **Effort:** medium to start, then open-ended.

### Option C: Option A, cutting over straight into Forgejo

- **Approach:**
  - Phases 1–3 as in A, all done here on GitHub.
  - At the cut-over, the public history is imported into
    `code.overt.org/ripls-org/ripls`, which becomes canonical.
  - `ripls-org/ripls` on GitHub becomes a push mirror, so the project keeps its
    discoverable presence there.
- **Pros:**
  - Release and deploy wiring changes once.
  - Self-hosted runners and deploy secrets never sit on a forkable repo, because
    Forgejo registration is closed.
  - Matches the stated end state of code and CI/CD self-hosted on mimir.
- **Cons:**
  - The cut-over now waits on the Forgejo port, which isn't decided:
    - the agent's ~98 `gh` calls;
    - no `workflow_run`;
    - no `author_association`;
    - Actions currently disabled on the instance.
  - Outside contributors can't open PRs on a registration-closed Forgejo. The
    GitHub mirror only collects issues, and patches would arrive by email or
    through a maintainer-created account.
- **Effort:** large. The Forgejo port is its own project, outside this issue.

## Recommendation

**Option A for phases 1–3. The phase 4 target is decided separately: Option C if
the Forgejo spike succeeds.**

Most of the work has to be done whichever forge wins:

- sorting the private set;
- turning the gate into a CI check;
- fixing build numbers;
- cleaning up TODOs;
- fixing links.

It's also safe to do now on GitHub, because the export stays the gate until the
flip. Only the cut-over (phase 4) depends on the destination. Keeping that
decision in phase 4 means a Forgejo delay doesn't block the rest.

If the Forgejo spike fails, phase 4 runs on GitHub. That adds three items:

- environment protection with required reviewers on the deploy and release
  environments;
- secrets scoped to those environments;
- self-hosted runners restricted to `push`, `workflow_dispatch` and `schedule`
  events, never `pull_request`.

Option B is rejected: it reintroduces the coupling the single-repo decision exists
to remove.

## Open Questions

- **Do the old plan docs move out, or get published after a scan?**
  - **Decided 2026-11-12: published, all of them, after a scrub** (#3286). See
    phase 3. The analysis below is what led there.
  - The issue says to move all of `docs/issues/` because it was "never vetted".
    But 62 of those docs are the design rationale that code comments cite, and
    a contributor following such a link from public code finds nothing.
  - Recommendation:
    1. Publish the 62 docs that public files link to, after two checks: they
       pass the narrowed denylist scan, and someone reads them for
       strategy/persona content.
    2. Move the other ~420 docs.
    3. Treat `docs/ai/` and `docs/concepts/` the same way.
  - Links that still dangle are rewritten to plain text such as "(design notes
    archived)" rather than left as dead links.
- **Which `docs/private/` docs ship with the infra they describe?**
  - Recommendation: publish the runbooks once a scan finds no secret values:
    - `infra_mimir.md`, `terraform_deployment.md`, `dependency_automation.md`,
      `secret-rotation.md`;
    - `release/*`, `walkthrough_extraction.md`, `coding_agent_cheat_sheet.md`.
  - Move to the notes repo:
    - `secrets_inventory.md`. Decision 1 would allow publishing it, but
      `docs/secrets.md` already settles this: the per-credential inventory "is
      instance data and does not live in this repository". Publishing it would
      mean rewriting that design, for little gain to contributors. *(audit fix)*
    - `users/**`, `user_journeys.md`, `sms_a2p_registration.md`;
    - `server/private_llm_*`, which is an evaluation whose data provenance
      isn't checked.
- **Where does `ripls-triage` get personas and priorities?**
  - **Decided 2026-11-03: retire the skill**, along with
    `docs/private/users/{personas,priorities}.md`. With nine open issues, the
    skill has no backlog left to triage.
- **How are build numbers kept increasing after the cut-over?**
  - Recommendation: `BUILD_NUMBER = git rev-list --count HEAD + BUILD_NUMBER_OFFSET`.
  - The offset goes in `versions.env` and is set once: the last uploaded build
    number, rounded up (for example 10,300).
  - It's simple and auditable, and it stays monotonic on any forge.
  - An alternative is a counter stored in the stores themselves, using
    fastlane's `latest_testflight_build_number` and
    `google_play_track_version_codes`. That is more robust but adds API calls
    to every build.
- **Where do alert-to-issue filings go, and does the code survive the forge move?**
  - Recommendation: crash and monitoring alerts carry user data, so they should
    file into the **private** tracker, alongside user feedback.
  - Publish `cloud_functions/` now. Porting its Octokit client to Forgejo is a
    Forgejo-port task, not part of this issue.
- **Does the denylist need to be a secret?**
  - Under decision 1 it keeps only:
    - personal emails and staff addresses (role addresses such as `support@`
      are public);
    - real users' names;
    - credential shapes.
  - Publishing personal emails and user names would confirm the very values the
    list exists to catch, so it does need to stay out of the tree.
  - Recommendation: keep it as a forge secret file, as the issue says. Have
    `oss_scan` **fail** in CI when the secret is missing on the canonical repo,
    and skip with a notice on forks.
- **Do the legal name and postal address stay on the denylist?**
  - **Decided 2026-11-03 (implementation finding): no, both are dropped.**
  - The address sits in files that phase 1 publishes: `infra/server/common.yaml`
    and `terraform/environments/{dev,prod}/main.tf`, as the Mailgun footer
    value. The law requires it in every marketing email, and the website's
    privacy policy and SMS terms pages already show it.
  - The legal name is already public too: `NOTICE` in the public repo names the
    entity, and the same config sets it as `LEGAL_ENTITY_NAME`. Decision 1
    listed it as private, but nothing private was left to protect.
  - Against the post-move tree, the `type2fun` pattern hit ~1,030 times in
    72 files. Nearly all were the GitHub org name in release-note links and
    container names, which are account identifiers under decision 1.
- **Is the public repo's PR flow ready for outside contributors at cut-over?**
  - Recommendation: no.
  - The cut-over makes it the working repo for maintainers. Contribution intake
    (CONTRIBUTING, DCO enforcement, fork CI) is already covered by #2953's
    earlier phases, and is re-checked once the forge is settled.

## Implementation Plan

### Phase 1: Make the excluded set publishable (here, on GitHub)
**Goal:** every excluded group is published, moved or deleted. Each group is its
own PR that removes its manifest exclude, and the next `oss:sync` publishes it.

**Tasks:**
- [x] **Narrow the denylist** to decision 1's scope:
  - dropped `[cloud-projects]`, `[cloud-account-ids]`, `[store-and-app-identity]`,
    `[github-app]`, `[self-hosted-infra]`, `[org-github]`, `[product-domains]`,
    `[go-module-path]` and `[org-legal-identity]` (see the open question
    above);
  - dropped the publishable key shapes from `[credentials-and-keys]`: Google
    API keys, OAuth client ids, the Analytics id and the Mapbox `pk.` token;
  - `[people-and-pii]` now lets role addresses through and catches the personal
    mail domain only in address form;
  - kept `[consent-flags]` and `[dangling-private-refs]`;
  - updated `oss_scan.test.js` to match.

  **Punch list against the post-move tree** (every excluded file except the
  moving subtrees, scanned with the narrowed list):
  - 4 `people-and-pii` hits, all fixed by later groups:
    - the Apple ID default in `Appfile` and `Matchfile`;
    - a name in `deploy_cloud_functions.yaml`;
    - a name in `walkthrough-export/content_test.go`.
  - 19 `dangling-private-refs` hits in 12 files: the infra, runner and
    Terraform links to `docs/private/` runbooks, plus `ripls-triage`.
- [x] **Delete** (#3240):
  - `e2e/fixtures/walkthroughs/brunch/` (15 files);
  - `server/test_data/manual/` (46);
  - `benchmark_results/` (6).

  Fix whatever references them, such as the walkthrough README and seed
  scripts.
- [x] **One-off scripts:** delete `drop_skill_tables.sql` and the two `_2188`
  scripts. Move `analyze_cloud_logs.sh`, `render_retrospective_charts.py`,
  `fetch_manual_case_images.sh`, `readonly-db-user.sql` and
  `README_BENCHMARKS.md` to the notes repo.
  - Done in #3240. `fetch_manual_case_images.sh` stayed and is published: it
    now fetches the eval images instead of committing them.
  - `analyze_cloud_logs.sh` and `readonly-db-user.sql` are published instead
    (#3268): `create_reader.sh`, `db:init-reader` and `observability.md` use
    them, and both scan clean.
- [x] **Fastlane** (#3246):
  - in `app/fastlane/Appfile:5`, replace the default Apple ID email with
    `ENV.fetch("APPLE_ID")`;
  - leave the Team ID default (decision 1);
  - publish `app/fastlane/**`.
- [x] **Instance config** (#3263):
  - ~~restrict the Mapbox public token~~: no longer needed. Mapbox was retired
    instead (the app and server had both run on Google Maps since #2188), so
    no Mapbox token ships. The token was revoked on 2026-11-11;
  - publish the Firebase configs, `env.*.json`, `Dev/Prod.xcconfig` and
    `debug.keystore`;
  - delete the `.example` siblings and the docs that tell people to copy them,
    or keep them for forks. Recommendation: keep them, since they're cheap and
    useful.
- [x] **CI fleet** (#3262): publish `runners/`, `renovate/`, the runner and
  renovate image workflows, `scripts/build_runner_image.sh` and
  `.github/actionlint.yaml`.
  - Actions run on the public repo, which has no runners registered, so jobs
    pinned to self-hosted labels would queue forever. They now run only where
    the repo variable `INSTANCE_PIPELINES` is `true`: set here, and set on the
    public repo at cut-over (phase 4).
- [x] **Deploy and release:**
  - publish `terraform/environments/`, the `release_*`, `deploy_*`,
    `build_and_deploy_container`, `test_ios_build` and `test_dev_e2e` workflows,
    `infra/server`, `infra/go-vanity`, `infra/sync.sh`,
    `cloud_functions/` and `test_cloud_functions.yaml`, gated the same way;
  - published after #3079's Terraform shrink;
  - `infra/web-nginx-reload` and `infra/staging-web` serve only the website
    deploy, so they stay with the website and move to its repo (phase 3);
  - once these and the CI fleet reach the public repo, drop the
    missing-file fallbacks in `check_versions.js` and `check_go_vanity.js`.
- [x] **`server/cmd/walkthrough-export/`** (#3265): remove its `docs/private`
  link and check it for prod identifiers that still matter (personal data, not
  account ids), then publish it. Real names, user ids and a street address in
  its tests became synthetic values.
- [x] **Skills** (#3246, #3240):
  - publish `ripls-release`;
  - delete `ripls-triage` and `docs/private/users/{personas,priorities}.md`
    (decided 2026-11-03).
- [x] **`docs/private/` runbooks** (decided 2026-11-05, after a full read):
  - **Published:**
    - `docs/infra/terraform_deployment.md` and
      `docs/infra/dependency_automation.md`;
    - `docs/coding_agent_cheat_sheet.md`;
    - `docs/release/{apple_checklist,google_checklist,google_sign_in}.md`.
  - **Kept private for the notes repo:**
    - `infra_mimir.md`: the host's personal services, LAN and Tailscale IPs,
      and an unauthenticated service port.
    - `secret-rotation.md`: a table of leaked keys that were never rotated,
      with where each one leaked.
    - `release/ios_build_server_setup.md`: FileVault off with auto-login,
      LAN IPs and open ports.
    - `walkthrough_extraction.md`: prod database access, for packs that can
      no longer be committed.
  - Links from published files to the private four are now plain text.
- [x] **`docs/private/` (rest)** (#3268): sort it by the open question above.
  Publish the runbooks to `docs/infra/` (or similar) with `context:`
  frontmatter, and move the rest to the notes repo.
  - Every remaining file was notes-repo material, so all 21 moved to
    `bryan/ripls-notes`, a private repo on the Forgejo instance. The
    `.gitleaks.toml` and context-map exclusions for the subtree went with it.
  - Each published runbook becomes a durable doc. It needs a `context:` block
    with globs over the infra it governs, and a `freshness:` stamp set after
    reading it against the code. Without the stamp, the nightly ratchet starts
    refreshing it. *(audit fix)*
- [x] **`docs/release/notes/`** (#3266, #3267): skim for user names (55
  files), then publish. Every "reported by" credit is gone from the notes and
  the website's copies, and `notes_prompt.md` no longer asks for them.

**Validates:** the manifest's excludes cover only `website/**`, the harness itself
and doc subtrees that are moving. `oss:sync` publishes cleanly under the narrowed
denylist.

### Phase 2: Fix what the move and the new history would break
**Goal:** the public tree builds, releases and links correctly on its own.

**Tasks:**
- [ ] **Build-number offset:**
  - add `BUILD_NUMBER_OFFSET` to `versions.env`;
  - apply it in `release_app.yaml` and `Fastfile:get_build_number`;
  - add a release-time guard: the `prepare` job compares the computed number
    with fastlane's `latest_testflight_build_number` /
    `google_play_track_version_codes`, and fails before building if it is not
    higher;
  - unit-test the offset arithmetic wherever it lives (shell function or
    Fastfile helper). *(audit fix: "test or check" was not a verifiable task)*
- [ ] **Release tags:**
  - release notes and the cut flow may diff against the last `v*` tag, and the
    public repo has none;
  - push `v0.33.0` onto the public commit that matches it, or record the base
    tag in `versions.env`;
  - check `release/notes_prompt.md`'s diff base.
- [ ] **The 15 TODOs that point at closed issues:** decide each one:
  - **resolve:** the deferred work is done or no longer wanted;
  - **refile:** open a fresh issue and point the TODO at it;
  - **delete:** the comment no longer earns its place.

  Done as one PR. Optionally, `check_todo_refs.js` could reject references to
  closed issues, but that needs the API and so can't run offline. Recommend
  doing it as a nightly check instead, not per PR.
- [ ] **Links into moving docs:**
  - publish the ~62 or more docs that code links to (open question 1), after a
    scan and a read;
  - rewrite the remaining links in the 184 files.

  `dangling-private-refs` widens to cover every moved subtree, so this stays
  fixed.
- [ ] **Gitleaks and the context map:**
  - drop `^docs/private/` from `.gitleaks.toml` once the subtree is empty;
  - prune `EXCLUDED_PREFIXES` in `gen_context_map.js` to subtrees that still
    exist (keep `docs/issues/`, since new plans land there);
  - regenerate `llms.txt`.

**Validates:** a build of the export alone produces a store-acceptable build
number. No public file links into a moved path.

### Phase 3: Gate on the working tree; settle what stays private
**Goal:** what was checked once at export time is checked on every change, and the
non-code pieces have their own homes.

**Tasks:**
- [x] **`oss_scan.js --tree`:**
  - scan `git ls-files` instead of an export;
  - read the denylist from a path set by `OSS_DENYLIST_FILE`;
  - extend `oss_scan.test.js` to cover tree mode, a missing denylist and
    redaction of matches in the output. CI logs on a public repo are public,
    so a hit must never print the matched value. *(audit fix)*
- [x] **CI job:** done in `check_oss_export.yaml` rather than the lint
  workflow, next to the export gate it shares the denylist with
  (`npm run oss:gate:tree`). The denylist is in the `OSS_DENYLIST` secret and
  the notes repo at `oss/oss_denylist.txt`; the canonical repo is named in the
  workflow, so update it at the cut-over. That workflow and `oss_scan.js` are
  private-bound today, so the cut-over must also move the tree step, the
  scanner and its fixture tests into something that ships; the real-denylist
  tests stay out of the public tree.
  - add an `oss_scan` job to the existing lint workflow;
  - write the denylist from a secret to a temp file;
  - skip it, with a notice, when the secret is absent (forks); fail it when the
    secret is absent on the canonical repo. Key this on the repository name,
    not on whether the secret is present, so deleting the secret can't quietly
    turn the gate off;
  - move `scripts/oss_denylist.txt` out of git: it lives in the secret and in
    the notes repo.
- [x] **Website** (#3287; **decided 2026-11-12: it stays in this repo and ships
  publicly**, reversing the split):
  - a separate repo was too entangled: `website/content/` symlinks static
    assets from `server/`, release notes are generated into it, and e2e pages
    live there. The site itself is already public at ripls.org;
  - the denylist's `!allow-under website/content/` section lets the founder's
    name and the staff addresses appear there, and nowhere else;
  - the deploy endpoint moved from the workflows into the repo variables
    `WEB_DEPLOY_SSH`, `WEB_DEPLOY_PORT` and `WEB_DEPLOY_ROOT`. The deploy
    jobs run only where `INSTANCE_PIPELINES` is set, and the staging preview
    never runs for PRs from forks;
  - `oss_export.js` recreates tracked symlinks instead of copying them.
- [x] **Plans** (#3286; **decided 2026-11-12: they stay public**, so new plans
  keep going to `docs/issues/`):
  - `docs/issues/`, `docs/ai/`, `docs/evals/`, `docs/crashlytics/` and
    `docs/reviews/reports/` ship, scrubbed of personal data: names, handles,
    emails, phone numbers, production ids and LAN IPs became placeholders;
  - the denylist gained a pattern for phone numbers written with separators;
  - two things stay excluded: `docs/concepts/**/*.mov` (36 MB of video that
    can't be reviewed like text) and `docs/reviews/reports/security_review.md`
    (it lists findings without saying whether each is fixed). Both move to
    the notes repo or are deleted before the cut-over.
- [x] **Notes repo:** `bryan/ripls-notes`, a private repo on the Forgejo
  instance; mimir's policy allows private repos there as long as nothing
  deploys from them. It holds `docs/private/**` (phases 1–2) and receives
  the denylist with the CI job above.
- [ ] **Private tracker:** **decided 2026-11-12: a private repo on Forgejo.**
  It moved to #3293 (phase 3 there), which ports both clients to the Forgejo
  API. The original GitHub-shaped tasks, for the record:
  - create it and move #2760;
  - point `server/github/client.go`'s config (`Owner`/`Repo` from app config)
    and `cloud_functions`' target at it;
  - both stay GitHub-API clients until the Forgejo port.
- [x] **Refile the five product issues** in `ripls-org/ripls`: #3213, #3079,
  #2952, #2864, #2792. Then renumber the TODOs for #2864 and #2792.
  - Refiled 2026-11-12 with personal details and account counts removed:
    #3213 → ripls-org/ripls#3, #2952 → #4, #2864 → #5, #2792 → #6.
  - `TODO(#2864)` became `TODO(#5)` and `TODO(#2792)` became `TODO(#6)`.
    Until the cut-over those numbers name the public tracker's issues, not
    this repo's. Prose that points at the open work says
    `ripls-org/ripls#5` in full; prose recording what #2864 already did
    keeps the old number.
  - #3079 is not refiled: it is closed, and its two leftovers are instance
    operations.

**Validates:** `oss_scan` and gitleaks pass on the working tree in CI. The website
deploys from this repo through the variable-based endpoint. (A feedback report
landing in the private tracker moves to phase 4 with the tracker.)

### Phase 4: Cut-over (gated on the forge decision)
**Goal:** development, deploy and release run from the public repo, and this repo
is archived.

**Tasks:**
- [x] **Decide the destination: Forgejo or GitHub.** **Decided 2026-11-12:
  Forgejo**, with `ripls-org/ripls` on GitHub as a push mirror. The port is
  #3293 (`docs/issues/3293-forgejo-port.md`); the rest of this phase waits on
  it. Its phase 0 is the spike, and the GitHub branch below stays as the
  fallback if the spike fails.
  - If Forgejo, the port must be done first:
    - the agent's `gh` calls replaced by a `forge` wrapper;
    - dispatchers ported (no `author_association`, so use
      `/collaborators/{user}/permission`);
    - the CI-fixer trigger replaced, since there's no `workflow_run`;
    - Actions enabled on the instance.
  - If GitHub:
    - add environment protection to deploy and release;
    - restrict self-hosted runners to non-PR events;
    - turn on push protection, which `docs/secrets.md` requires for a public
      repo;
    - require approval for fork-PR workflow runs.

    The public test workflows resolve `runs-on` from `vars.CI_RUNNER_LINUX`.
    Once that variable is set on the canonical repo, fork PRs would land on the
    self-hosted fleet unless PR jobs force `ubuntu-latest`. *(audit fix)*
  - If Forgejo: it has no push protection, so the push-time gap that
    `docs/secrets.md` describes remains. Close it with a server-side
    `pre-receive` gitleaks hook on the instance (admin-configured, in mimir's
    forgejo stack), or accept the gap and say so in `docs/secrets.md`.
    *(audit fix)*
- [ ] **Final export:**
  1. Run a final `npm run oss:sync -- --push`, with the excludes reduced to the
     harness itself.
  2. Delete the harness: `oss_manifest.txt`, `oss_export*.js`, `oss_publish.js`
     and `check_oss_export.yaml`.
  3. Delete the `oss:*` npm scripts.
  4. Fix `server/cmd/check-image-gps/main.go`'s reference.
- [ ] **Move the secrets and variables** to the public repo or its environments:
  - `BUF_TOKEN`, the GCP service-account keys, Android signing, ASC and Play;
  - `MATCH_*`, `MIMIR_DEPLOY_SSH_KEY`, `WEB_PUBLISH_SSH_PRIVATE_KEY`;
  - the denylist.

  Register the runners against the new repo.
- [ ] **Agent:**
  - set `AGENT_TARGETS` to the new repo;
  - update the ripls-agent README;
  - `AGENT_REPO` and dispatch variables move with the secrets.
- [ ] **Branch protection:** turn on required PR checks on `main`.
- [ ] **Docs:** rewrite `CLAUDE.md` (export references) and `README.md` for
  one-repo development.
- [ ] **Prove it:** cut one release from the public repo, with deploy, store
  upload and tag.
- [ ] **Archive:** archive this repo, which is #3229.

**Validates:**
- The issue's three criteria: one release cut from the public repo; `oss_scan`
  and gitleaks pass on the public tree; and this repo is archived because
  nothing in it is still needed.
- Also: the first public-repo build number is greater than the last private
  build number.

## Related Issues
- #2953: the epic. Phase 4 of its plan is "One repo, archive the rest".
- #3229 (OSS-15): archives this repo's history. It is the last step of phase 4
  here.
- #3293 (OSS-18): the Forgejo port that phase 4 waits on.
- #2971, #2969: replaced by this issue and #3229; both closed.
- #3079 (HOST-5): GCP decommission and Terraform shrink. Landing it before phase
  1's deploy group means less Terraform gets published.
- #2954: credential rotation, closed as not planned. Its one carried-over item,
  restricting the Mapbox token, was dropped when Mapbox was retired.
- #2970 (OSS-16): the dispatch bridge. Its validation and dispatchers would be
  ported in phase 4 if the destination is Forgejo.
- #2864, #2792: the only open issues that `TODO`s point at. Both are refiled in
  phase 3.

## Audit Notes (2026-11-02)

The scope is infra, CI and tooling, with no app or server product code. The
dimensions that apply are security (secrets, PII, the public CI surface),
testing (the new scan mode and the build-number logic) and code quality for the
scripts. The plan was edited in place; each change is marked *(audit fix)*.

1. **`secrets_inventory.md` now moves to the notes repo instead of being
   published.** `docs/secrets.md` already decides that the per-instance
   inventory stays out of the repo, and the plan had contradicted it.
2. **Push protection.**
   - `docs/secrets.md` requires GitHub push protection on a public repo.
   - Forgejo has none, so phase 4 now either closes that gap with a
     server-side `pre-receive` gitleaks hook, or records it as accepted.
3. **Fork PRs on self-hosted runners.**
   - 19 workflows resolve `runs-on` from `vars.CI_RUNNER_LINUX`. That variable
     is unset on `ripls-org/ripls` today (checked).
   - If the cut-over sets it on a public GitHub repo, fork PRs would run on the
     fleet. Phase 4 (GitHub branch) now covers this and requires approval for
     fork-PR runs.
4. **Tests are concrete now.**
   - `oss_scan` tree mode gets tests: missing denylist, and redaction, since CI
     logs on a public repo are public.
   - The build-number offset gets a unit test and a release-time guard against
     the store's latest number. Previously the plan said only "test or check".
5. **Fail-closed gate.** The CI scan's skip/fail rule keys on the repository
   name, not on whether the secret is present.
6. **Context map.** Runbooks published from `docs/private/` become durable docs,
   so they need `context:` blocks and `freshness:` stamps
   (`docs/context_map.md`).

No conflict found with the architecture anchors, since no product code changes.
The proto, client and server conventions don't apply.

**Docs consulted:**
- `docs/llms.txt`
- `docs/context_map.md`
- `docs/secrets.md`
- `docs/release/automation.md`
- `docs/reviews/security_review_prompt.md` (Areas to Review §3 Secrets)
- `docs/reviews/testing_review_prompt.md` (Areas to Review §1, §4)
- `docs/issues/2953-open-source-release.md`
