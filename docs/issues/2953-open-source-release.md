# Issue #2953: [Epic] Open-source the codebase: strip org/deployment specifics and publish from a fresh public repo

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2953
**Filed:** 2026-08-22 | **Last activity:** 2026-10-31
**Labels:** enhancement, open-source | **Assignees:** unassigned
**Context docs:** `docs/llms.txt`, `docs/context_map.md`, `docs/coding_agent_cheat_sheet.md`, `docs/secrets.md`, `docs/secret-rotation.md`, `docs/dependency_automation.md`, `docs/infra_mimir.md`, `docs/walkthroughs.md`, `docs/reviews/security_review_prompt.md`, `docs/reviews/dependency_review_prompt.md`, `docs/reviews/tech_debt_review_prompt.md`, `docs/reviews/issue_filing_spec.md`, `docs/server/architecture.md`, `docs/client/architecture.md`, `docs/proto_conventions.md`

> **What this doc is for.** The epic body carries the *audit* (what is org-specific,
> where, at which line) and the *decisions*. The 17 sub-issues carry the per-area
> inventories. This doc carries the **execution layer**: the sequencing, the
> dependency graph, and — most importantly — four corrections that only surfaced
> when the tree was re-read against the confirmed decisions. An autonomous triage
> pass on 2026-08-22 concluded that a plan doc would duplicate the epic; that was
> written *before* the decisions were confirmed, and §Corrections below is the part
> that isn't in the epic. Following the sub-issues literally as written today would
> break the live website deploy.

> **Status (2026-10-31): the end state changed to one public repo.** This repo
> will not become a private deploy repo that pins the public one. Instead, deploy
> and infra config are published alongside the product code, and the genuinely
> private material moves out: strategy and working docs go to a private notes
> repo, user feedback to a private tracker, the marketing site to its own repo,
> and secrets stay in Secret Manager. The old history is archived privately, not
> rewritten for publication. Tracked in #3228 (OSS-17, replacing #2971) and #3229
> (OSS-15, replacing #2969); see Phase 4. Three decisions behind it:
> 1. **Account identifiers are no longer private.** That covers GCP project ids,
>    the Apple Team ID, bundle ids and Firebase configs. The denylist covers only
>    secrets and personal or legal identity.
> 2. **The website gets its own repo.**
> 3. **The old history stays archived.**
>
> The planned move to self-hosted Forgejo (`code.overt.org`, registration closed)
> removes the fork-safety reason for two repos. OSS-7 (#2959) and the OSS-16
> dispatch bridge (#2970) are done. The fork negative test was dropped because of
> that move. #2954 (rotation) was closed as not planned.

> **Status (2026-10-19): `ripls-org/ripls` is public since 2026-10-05** — ahead of
> OSS-17, deliberately; see the Phase 4 note. Publish record on #2966, repo
> settings on #2967 (both closed). Until the #3228 cut-over, **this repo is still the
> source of truth**: product changes land here and reach the public repo as a
> sync commit on top of public `main` (author `OSS Export <oss@export>` — never a
> personal identity, never a force-push; `main` is protected against both). From
> a clean `main` at `origin/main`, `npm run oss:sync` builds the export, runs the
> content gate and LFS checks, and stages that commit with its diff;
> `npm run oss:sync -- --push` pushes it. Sync after merging anything that
> changes the exported tree, such as workflow changes the public repo needs. A PR
> accepted on the public repo must be ported back here or the next sync reverts
> it. The public repo holds only `BUF_TOKEN` and `AGENT_DISPATCH_PRIVATE_KEY`.
> Agent secrets live in the private agent repo, behind the OSS-16 dispatch bridge.

## Problem Statement

Publish the product code (Go server, Flutter app, protos, reusable Terraform
modules, engineering docs, generic CI) as Apache-2.0 open source at
`github.com/ripls-org/ripls`, while leaking nothing that belongs to the
organization rather than the product: legal identity and address, cloud and store
account identifiers, physical-infra runbooks, business strategy, real users'
data, and the marketing site.

Three findings make this a migration rather than a scrub:

1. **The history can't ship.** 327 gitleaks findings across 40 of 8,016 commits.
2. **The tracker can't ship.** ~200 `user-feedback` issues carry real users' device
   context and screenshots.
3. **Real people's private content is tracked.** The `brunch` walkthrough pack is a
   prod extraction of a family event, flagged `purgeIfRepoGoesPublic` by its own
   manifest.

So the public repo is created by a **scripted, filtered export** into a fresh
history, and this repo stays private — becoming the deploy repo that pins the
public one. *(Superseded 2026-10-31: the public repo becomes the only repo, and
this one is archived. See #3228 and #3229.)*

## Context from Comments

One comment, an autonomous triage pass (2026-08-22). Two claims, handled:

- *"The epic is already a complete plan; a plan doc would duplicate the body."*
  True for the audit and the decisions — this doc deliberately doesn't restate
  them. Not true for execution: see §Corrections.
- *"Phase 0 needs human decisions (org name, license, icons, release notes,
  brunch re-consent)."* All resolved on 2026-08-22 and recorded in the epic's
  *Design decisions (confirmed)*: `ripls-org/ripls`, Apache-2.0 + DCO, brand
  assets kept under `TRADEMARK.md`, no release notes in the first cut, brunch
  dropped. Only #2954 (rotation) still needs a human with credential authority.
  *(#2954 was closed as not planned on 2026-10-30.)*

The decision that reshapes everything, and that the sub-issue bodies predate:
**this repo becomes the private deploy repo** rather than a new one being created
for the private material. *(Superseded 2026-10-31 by the single-repo plan, #3228.
Corrections 1–5 still describe how the export works until that cut-over.)*

## Corrections

These are the reason this doc exists.

### 1. "Move to a private repo" now means "keep in place and exclude from the export"

OSS-3, OSS-5, OSS-6, OSS-8, OSS-11 and OSS-12 were written when the plan was a
*separate* private repo. Since this repo **is** the private repo, the private
material — `website/`, `infra/`, `runners/`, `terraform/environments/`,
`app/fastlane/`, Firebase configs, strategy docs, `docs/issues/` — **stays exactly
where it is**. The operations become:

| Written as | Actually |
|---|---|
| "delete `website/` from this repo" | keep it; **decouple** `server/` from it; exclude from export |
| "move `docs/users/` to the private repo" | relocate to an excluded path; exclude from export |
| "move `terraform/environments/` private" | keep; blank the four `modules/` defaults |
| "move `runners/`, `infra/` private" | keep; parameterize the org/repo strings inside them |
| "untrack Firebase configs" | keep tracked **here**; add `.example` siblings; exclude the real ones from export |

Only three things are genuinely deleted from this repo's tip: the brunch pack
(OSS-4, PII), the copyrighted band flyer (OSS-12), and — via history rewrite —
the leaked secrets (OSS-15).

**Why this matters concretely:** `.github/workflows/publish_website.yaml:3-7`
deploys the live site on push to `main` touching `website/**`, and
`server/services/web/service.go:20` imports `type2fun.org/ripls/website`. A first
PR that "splits out `website/`" per OSS-3 as written takes down the site deploy
and breaks the server build.

### 2. The export should be an allowlist, not a denylist

OSS-14 currently specifies a denylist of paths that must never ship. A denylist
**fails open**: any new private file added between now and the export — or in any
future sync — leaks unless someone remembers to add it. An allowlist **fails
closed**: a new file is absent from the public repo until someone deliberately
adds it, and the failure mode is a missing file (loud, harmless) instead of a leak
(silent, unrecoverable).

Recommendation: an explicit `oss_manifest.txt` of public-bound path globs drives
the export; keep the denylist as a **second, independent gate** that fails the run
if anything matching it survived. Belt and braces, in that order.

### 3. The export must be a pure path filter, never a content transform

Any file whose content differs between public and private is untested divergence —
what we build and test here isn't what ships. So: **every file is either
public-bound or private-bound; none is both-with-different-content.** Where content
must differ, split into two files at different paths.

Three places need that split:

- **`README.md`** — public one at the root; the deploy/setup runbook moves to a
  private path.
- **CI** — already file-separable: shared test workflows are public-bound, deploy
  and release workflows are private-bound, and the gated dispatchers (OSS-7) are
  new public files.
- **`docs/llms.txt`** — this is the subtle one. `scripts/gen_context_map.js`
  indexes all of `docs/`, and `npm run lint:context-map` fails on a stale index. If
  the export drops ~29 indexed docs, the public repo's `llms.txt` describes docs it
  doesn't have and the gate fails on the first PR. Regenerating during export would
  be a content transform (violating the invariant above).

  **Fix:** relocate private-bound docs under a prefix the generator already
  ignores. Add `docs/private/` to `EXCLUDED_PREFIXES`
  (`scripts/gen_context_map.js:64-71`, alongside the six already there) and move
  `docs/users/`, `docs/infra_mimir.md`, `docs/secret*.md`, `docs/release/*checklist*`,
  `docs/server/private_llm_*`, `docs/dependency_automation.md`,
  `docs/coding_agent_cheat_sheet.md`, `docs/concepts/`, `docs/evals/` there. Then
  `docs/llms.txt` is **byte-identical** in both repos, the gate passes on both, and
  the export stays a pure filter. `docs/issues/`, `docs/ai/`, `docs/release/notes/`,
  `docs/crashlytics/`, `docs/reviews/reports/` are already excluded — leave them.

### 4. Public docs currently link into private docs

Four confirmed dangling links once the private set is excluded:

- `docs/client/testing.md:486` → `../infra_mimir.md`
- `docs/workshop.md:917,926` → `users/host-journey.md`
- `docs/sms_notifications.md:353` → `secrets.md`
- `docs/user_feedback.md:195,196` → `secrets.md`, `secret-rotation.md`

For `secrets.md` specifically, don't just delete the link: split the doc into a
public *mechanism* doc (how Secret Manager fetch, `entrypoint.sh`, and the gitleaks
stack work — no project IDs, no inventory) and a private *inventory/runbook*. That
keeps the two cross-links meaningful. Add a link check to the export gate: no
public-bound doc may reference a `docs/private/` path.

Two docs need that same split rather than a straight move, because each is half
public mechanism and half private inventory:

| Doc | Public half | Private half |
|---|---|---|
| `docs/secrets.md` | the SM-fetch architecture (`:23-68`), the DIY gitleaks stack (`:403-459`) | the per-credential inventory, the GitHub App ID (`:230`), the token-format table with real prefixes (`:373-380`), every `--project=ripls-*` example |
| `docs/walkthroughs.md` | the reel pipeline, scene toolkit, fixture-pack rules | the **prod-extraction runbook** (temporary Cloud SQL public IP, `metabase_reader`, `gs://ripls-prod` — `:307-340`) |

`walkthroughs.md` also carries `context: globs` pointing at
`website/content/walkthroughs/**` and `server/cmd/walkthrough-export/**`
(`:6`), both of which OSS-3/OSS-4 retarget or delete. Stale globs don't fail
`lint:context-map` (it validates block shape and index freshness, not glob
resolution), so nothing will catch this automatically — fix the block in the same
PR.

### 5. `docs/issues/` must exist publicly even though its 480 files don't ship

The plan-PR lifecycle is built on it: `claude_plan_issue.yaml:276,294,305` writes
`docs/issues/<n>-<slug>.md`, and refine/implement read the path back out of the
`<!-- claude-plan-path: … -->` PR marker. If `docs/issues/` is simply "private",
the public repo cannot host a plan PR at all — which would gut the gated
automation OSS-7 exists to preserve.

Resolution: the **path** is public-bound, the **existing contents** are not. The
export allowlist omits `docs/issues/*`, so the public repo starts with none and
accumulates its own from its first plan PR. This repo keeps its 480 for private
planning. The two trees share a directory name and never sync — so OSS-14's
"second export produces a clean diff" check must exempt `docs/issues/`, and after
OSS-17 the rule is: *plans for public product work live in the public repo; plans
for deploy/infra work live here.* This doc is an example of the latter.
*(Under #3228, every new plan lives in the one public repo, deploy and infra plans
included. The existing, unvetted plans move to the private notes repo.)*

### 6. Going public trips a documented decision trigger the plan didn't honor

`docs/secrets.md:21` records the 2026-05-08 decision (#1773) to run a **DIY**
gitleaks stack instead of paid GitHub Secret Protection, and names the exact
conditions for revisiting it — one of which is verbatim: *"the repo opens to
outside contributors."* This plan is that trigger firing.

It matters because the DIY stack's documented residual risk (`docs/secrets.md:435`)
is that there is **no server-side push block**: a leaked secret reaches GitHub's
commit database the moment a push is accepted, and detection happens seconds
*after*. In a private repo the blast radius is the team. In a public repo,
credential-scraping bots index new commits continuously, so the same leak is
harvested before anyone reads the CI failure. The mitigation that closes it —
push protection — is precisely what #1773 defers.

**Add to Phase 0 (#2967):** re-evaluate #1773 and decide whether push protection
is switched on for `ripls-org/ripls` before it accepts outside contributions.
Note that GitHub Secret Protection is **free on public repositories** (the
per-committer price applies to private ones), so the cost argument that drove the
2026-05-08 deferral does not apply to the public repo at all. Also update
`docs/secrets.md:21` itself, whose rationale sentence ("The repo is private")
stops being true.

Related, and easy to miss: `.gitleaks.toml` is **public-bound** (it is the
scanner's config), and its allowlist contains our own identifiers —
`:57` a truncated real Google key prefix, `:65` a real Mapbox token prefix
encoding the product name, `:63-65` the Secret Manager secret names. OSS-13
(#2965) currently scopes the scrub to "lines 60-65" and so misses `:48-57`.
Scrub the whole `regexes` block, and do it in the **same PR** that untracks the
Firebase configs (OSS-11) so the `paths` allowlist at `:28-30` never sits stale
— an allowlist entry for a file that no longer exists is a loaded gun if the file
is ever re-added.

## Conventions the implementer must honor

Migration-shaped work trips repo conventions that ordinary feature work doesn't.
From `CLAUDE.md` and the CI gates:

- **`git mv` for every relocation.** Correction 3's doc move and OSS-3's asset
  move are large renames; `git mv` preserves history *in this repo* (the export's
  history is fresh regardless, but this repo is where blame still has to work).
- **New Go package ⇒ `doc.go` + `README.md`.** If OSS-3 lands the embedded
  assets as `server/services/web/assets/`, `revive.package-comments` fails the
  build without a `// Package assets ...` comment in a dedicated `doc.go`, and
  the multi-file-directory rule requires a `README.md`.
- **`TODO(#NNNN):` or nothing.** `scripts/check_todo_refs.js` fails CI on a bare
  `TODO`, and `CLAUDE.md` requires the tracking issue to be filed *before* the
  TODO is written. Placeholder defaults left by OSS-9/OSS-11 are exactly where
  bare TODOs appear.
- **No phase labels in code comments.** "Phase 1/2/3" is this doc's vocabulary,
  not the codebase's; `scripts/check_ephemeral_comments.js` gates it. Reference
  durable issue numbers instead.
- **`.yaml`, never `.yml`,** for any new workflow or compose file. (`runners/docker-compose.yml`
  is a pre-existing violation — don't copy it into new files.)
- **New script ⇒ unit test.** `scripts/` convention pairs non-trivial scripts with
  a `node --test` companion (`check_todo_refs.test.js`, `doc_freshness.test.js`,
  `gen_design_tokens.test.js`, …). The export harness's allowlist matcher is the
  single highest-consequence pure function in this project — it decides what
  becomes public — so it ships with `scripts/oss_export.test.js`.

### 7. Rotation is not a gate on publishing — this plan said it was, and that was wrong

The epic's design decision 7 and OSS-14's checklist both treat #2954 (credential
rotation) as a hard gate on the first public push. That was carried over from
#1771, which was written for a different scenario — *flipping this repo public
with its history intact*. Once the decision changed to a fresh-history export,
the reasoning stopped applying and nobody re-derived it.

**Why it does not gate publishing.** The export is a copy of current tracked
files into a new repository with a single commit. No historical object travels
with it. The only way a leaked value reaches the public repo is if it is still
present in a *current* file — which is exactly what the gitleaks run and the
denylist scan over the export tree check, and the working tree is clean today.
So the publication gate is **"the exported tree scans clean"**, nothing more.

**What rotation actually buys, independently.** It closes the residual exposure
to whoever could already have seen the values. Measured honestly, that surface
is smaller here than the runbook's default posture assumes:

- The repository has been **private since it was created** (2025-09-10) and is
  not a fork. The values were never publicly reachable.
- The two npm scripts that carried the inline keys — `start:server` and
  `benchmark:skills` — were **never invoked by any workflow** (`git log -S` over
  `.github/workflows/` returns nothing), so they never reached Actions logs.
- Access was three humans plus bots, all still associated with the project.

What remains is: local clones on those three machines, historical checkouts on
the **non-ephemeral** self-hosted runners (`_work` directories on a host the org
controls), and GitHub's own object store, where unreachable objects survive a
force-push until garbage collection and stay reachable by SHA to anyone with
repo access.

**So triage by consequence × cost rather than rotating all twelve as a ritual:**

| Credential | Verdict |
|---|---|
| OpenAI, Anthropic, Mailgun, Unsplash, Pexels, Pixabay | **Rotate.** Cheap (new key → new SM version → redeploy), no user impact, and they are billable — this org has already lost ~$800/mo once to a leaked-key path. |
| JWT signing secret | **Rotate, scheduled.** Highest consequence by far: possession forges a token for any user, indefinitely, undetectably, without touching our infrastructure. Also the most disruptive — it logs everyone out — so it needs a comms window, not a quiet swap. |
| Firebase service-account key | Already revoked 2026-05-11 (#1767 row 1). **Verify only.** |
| `metrics-auth-token` | Endpoint removed in #1613/#1614 (`docs/secrets.md:48-50`). **Confirm dead, then delete the SM entry.** |
| Mapbox public token | **Different calculus** — it is a *client* token shipped inside app binaries by design, so it is already public. Restrict by URL/bundle in the Mapbox console; rotating requires a client release and buys little. |
| Prod Postgres password, GitHub App private key | **Lower urgency.** The database is reachable only via private IP + IAM; the App key's blast radius is issue creation in this repo. Rotate when convenient. |

**Revised sequencing:** #2954 is independent remediation on its own priority, not
a blocker for #2966. It should still happen — the JWT secret in particular — but
the first public push waits on a clean scan, not on a fully green rotation table.
The one scenario that would make rotation mandatory-first is publishing with real
history, and we are not doing that.

### 8. The bundle ID stays; the Apple Team ID does not

OSS-11's task line asks for "Gradle/xcconfig/manifest placeholders for bundle
IDs" alongside blanking `DEVELOPMENT_TEAM`, as if the two were the same kind of
thing. They are not, and lumping them together was a mistake.

`org.ripls.app` is **public by construction**. It is the app's identity in both
stores — it is literally in the Play listing URL — so anyone who has installed
the app already has it. Publishing it discloses nothing, and it sits in exactly
the class of identifiers this plan keeps on purpose: the product name, the
`ripls://` deep-link scheme, and the `github.com/ripls-org/ripls` module path.

Removing it, on the other hand, is not cheap. On Android `applicationId` and
`namespace` are entangled with the Kotlin package tree (`org.ripls.gear_app`)
and with the upload identity Play matches on; on iOS it is entangled with the
`match AppStore org.ripls.app` provisioning profile. That is release-pipeline
risk in exchange for no confidentiality gain.

The **Apple Team ID** (`2HL54765FF`) is the opposite case: an account
identifier that maps to the legal entity in Apple's directory, disclosed
nowhere the app itself reveals. It moved out of `project.pbxproj` and into
`ios/Flutter/{Dev,Prod}.xcconfig`, which the manifest excludes; the six
`DEVELOPMENT_TEAM` lines in the project file are gone. Nothing regressed:
`xcodebuild -showBuildSettings` resolves the same team for Debug, Release and
Profile, and the release lane never depended on the committed value anyway —
`fastlane ios release` calls `update_code_signing_settings` with
`ENV["APPLE_TEAM_ID"]` and rewrites the project at build time.

Consequence for the harness: `org\.ripls\.app` is removed from the
`store-and-app-identity` denylist rules. It was inflating three categories at
once (it also matches the `ripls\.app` product-domain pattern as a substring),
which made the scan's remaining count unreadable as a punch list.

### 9. For the apex domain, the test is "would a fork phone home?", not "is it secret"

`ripls.app` is public by construction, like the bundle id in Correction 8 — it
is in every share link the product has ever sent. Scrubbing it from *prose*
buys no confidentiality and makes internal docs worse, which is the opposite
of the house style (`CLAUDE.md` → *READMEs and Internal Documentation*: use
real names, they orient the reader).

The reason `product-domains` is a category at all is a different failure, and
a real one: **a fork that inherits a hardcoded domain sends its users to our
servers.** A wrong `resetURL` mails our host to their users; a wrong
`android:host` hands them our App Links. That is misrouting, not disclosure,
and it is why the code-side work — `Environment.getServer`,
`branding.AppBaseURL`, `TERMS_URL`, the deep-link scheme — was worth doing.

So the line is:

| Kind of occurrence | Treatment |
|---|---|
| Code, config, or a command a reader would copy and run | Parameterize. Copying it verbatim must not reach our infrastructure. |
| Prose describing how *this* deployment is wired | Name the real domain. It is clearer and discloses nothing. |

Applied: the AndroidManifest snippet in `docs/client/deep_linking.md` becomes
`example.app` (it is copy-paste bait); the sentence right below it that says
Associated Domains are configured for `ripls.app` stays as it is.

**The real code-side entries, and what each would have done to a fork:**

| Entry | Failure it would have caused |
|---|---|
| `sms.welcome.body` in `l10n/source/{en,es}.toml` | the opt-in confirmation text every new phone user receives would send *their* users to *our* site. Now an `AppURL` param from `SMSChannel.AppBaseURL`; the catalog drops the sentence entirely when it is empty, so the CTIA disclosure that makes the message compliant still stands alone. |
| `Environment.getServer` | every RPC from a fork's app would hit our API. Now the `SERVER_URL` dart-define; unset for dev/prod returns `""` so the failure is loud rather than a quiet cross-deployment call. `local` stays derived — the right answer varies by platform and by how the bundle is served. |

**The App Links config is deliberately NOT parameterized**, in
`AndroidManifest.xml` and `Runner.entitlements` alike. It looks like the same
class of problem and is not: `autoVerify` makes Android fetch
`/.well-known/assetlinks.json` from the host and match the package plus signing
certificate, and iOS does the equivalent with
`apple-app-site-association` and the Team ID. A host a fork does not control
simply fails verification and the filter never fires — no misrouting, no
disclosure. Weighed against that, a `manifestPlaceholders` / xcconfig
substitution is a change whose only real failure mode (silently breaking
Universal Links) **CI cannot catch**, because the iOS lane builds unsigned and
entitlements are applied at signing. Both files instead carry a comment saying
what a fork must change and why. Same reasoning, same conclusion as
Correction 8's bundle id.

Consequence: the residual `product-domains` count will not reach zero, and
should not be read as a punch list once the code-side entries are gone. The
categories that *must* reach zero are `credentials-and-keys`,
`cloud-projects`, `store-and-app-identity`, `github-app`, `people-and-pii`
and `self-hosted-infra`.

### 10. The server owns its static assets, and the association files ship with them

OSS-3 was the last structural blocker, and the measured version of it was
blunt: `server/services/web/service.go` imported `type2fun.org/ripls/website`,
`website/**` is export-excluded, and so **the public repo did not compile**.
Checking every internal import in the exported tree found exactly one
unresolved package.

The fix is the half of OSS-3 that was always right — *move the embedded assets
into `server/`* — and none of the half that was written before Correction 1.
`website/` stays exactly where it is; `publish_website.yaml` still deploys the
live site from it.

`css/`, `fonts/`, `assets/logo.png` and `.well-known/` moved to
`server/services/web/static/`. They are the **server's** assets: it is the
origin that serves them at `/css/`, `/fonts/` and `/.well-known/` on the apex
domain. The marketing site, which needs the same stylesheets on a different
host, reaches them through **directory symlinks** at
`website/content/{css,fonts,.well-known}` that its deploy dereferences with
`rsync --copy-dirlinks`. That flag was chosen over `--copy-links` because it
leaves *file* symlinks alone, and `website/content/legal/privacy-policy.html`
is one — verified against a scratch tree before committing to it. The
resulting deploy tree is **byte-identical** to what `main` produces today,
verified by rsyncing both and diffing.

**The two association files ship publicly, deliberately.**
`apple-app-site-association` carries the Apple Team ID that Correction 8 took
out of `project.pbxproj`, and `assetlinks.json` carries the Android signing
certificate fingerprints. Keeping them is right anyway, because they are
**served to the public internet at a fixed, well-known URL** — every device
that opens one of our links fetches them. There is no confidentiality to
protect, which puts them in the same class as the bundle id (Correction 8) and
the App Links host (Correction 9): public by construction, and inert for a
fork, since the OS checks them against the app that is actually installed.

Every alternative costs something real for nothing:

| Alternative | Cost |
|---|---|
| Generate them from branding flags | `/.well-known/` 404s until configured; two JSON shapes to keep in step with the manifest and entitlements |
| Ship placeholders, override at runtime | public and private repos carry different content at the same path — Correction 3 forbids it |
| Leave them in `website/` | the `//go:embed` pattern matches nothing and the build fails again |

`scripts/oss_scan.js` exempts the two files, and
`server/services/web/static/README.md` records the reasoning next to them.

One incidental find, worth remembering because it is the same shape as the
widget-test trap in Correction 9: `scripts/check_web_colors.js` walked
directories with `Dirent.isDirectory()`, which is **false for a symlink**. The
moment `website/content/css` became one, the linter silently stopped scanning
every stylesheet and kept reporting OK. It now uses `statSync`, and the fix
was verified by injecting a violating color and watching it fail.

### 11. The module path is `go.ripls.org/ripls`, not a GitHub URL

This supersedes the epic's confirmed decision 1, which said the module path
would be `github.com/ripls-org/ripls`. Bryan's objection was the right one:
that path bakes *the website the code happens to be hosted on* into the
identity of every import statement in the project — the same coupling this
whole epic exists to remove, in the one place we were about to add it
deliberately.

Go supports the fix natively. A module path is resolved by fetching
`https://<path>?go-get=1` and reading a `go-import` meta tag naming the real
repository. So the path names the **project**, the tag names the **host**, and
moving forge is a one-line edit that changes no import anywhere and requires
nothing of downstream users. `k8s.io/kubernetes`, `golang.org/x/…`,
`go.uber.org/zap` and `gorm.io/gorm` all work this way; several are on GitHub
and do not say so.

**Where it is served.** A standalone static site on mimir —
`infra/go-vanity/`, its own nginx container, its own reverse-proxy entry,
private-bound. Three options were considered and rejected:

| Option | Why not |
|---|---|
| `ripls.org/ripls` on the marketing site | that site is rsynced to a **home server**; module resolution should not depend on a homelab, and it would share a deploy with marketing content |
| Cloudflare Pages / Worker | a new deployment platform for one HTML file |
| A handler in the Go server | tidy, and it already serves `.well-known/` for the same class of protocol handshake — but it makes prod uptime a dependency of `go get`, and prod is Cloud Run |

Standalone means a marketing deploy cannot break module resolution, and vice
versa.

**Two things this commits us to.** `github.com/…` would have had GitHub keep
the path resolvable forever; now it is our DNS record and our container.
`proxy.golang.org` is an immutable mirror, so already-published versions keep
resolving through an outage — but a new version, a `GOPRIVATE` consumer, and
pkg.go.dev indexing all need the host live.

**Two implementation details that are easy to get wrong.** The toolchain
probes the **full import path**, not just the module root, so the tag has to
be served for every path beneath `/ripls` — the nginx config is a catch-all
for exactly that reason, and a 404 reads to the go tool as "no such module".
And `https://github.com/ripls-org/ripls` still appears in `package.json` and
one User-Agent string: those name where the code is *hosted*, which is a
different fact from the module path and must survive a forge move unchanged.
The rename skipped any occurrence preceded by `/`.

`npm run lint:go-vanity` compares the tag against `go.mod`. They live in
different files, in different languages, deployed by different mechanisms, and
nothing else compares them — and drift is invisible from inside the project,
because every local build keeps working while `go get` silently fails for
everyone without a checkout. The check skips cleanly when `infra/` is absent,
which is its normal state in the public repo.

## Potential Solutions

### Option A: Prepare in place, filter-export at the end
- **Approach:** All decoupling, parameterization and scrubbing lands as normal PRs
  against `main` here. Private material stays put. An export script produces the
  public tree from an allowlist. Public repo created at the end from that tree.
- **Pros:** Never breaks the running system (site deploy, releases, CI keep
  working throughout). Every change is reviewed in the normal flow. One source of
  truth until the moment of cutover. Rollback is a revert.
- **Cons:** The public tree doesn't exist until late, so "does the public subset
  stand alone?" is unverified for most of the project — unless the export harness
  is built early (see Recommendation).
- **Effort:** Large, but incremental and parallelizable.

### Option B: Export early, iterate in the public repo
- **Approach:** Rough export now into a private-visibility `ripls-org/ripls`, then
  fix org-specific values there while this repo continues.
- **Pros:** The public tree is real and testable immediately.
- **Cons:** Two diverging trees for the whole project, with no merge path — every
  fix must be applied twice or cherry-picked. The divergence problem OSS-17 exists
  to end, started at the beginning instead of the end.
- **Effort:** Large, plus continuous sync tax.

### Option C: Long-lived `oss-prep` branch
- **Approach:** Do all the work on one branch, export from it, merge to `main` at
  the end.
- **Pros:** `main` is untouched until cutover.
- **Cons:** This repo runs ~9,800 commits of history and merges continuously; a
  branch touching every Go import (OSS-10), every workflow, and the docs tree would
  conflict constantly. The module rename alone would be unmergeable after a week.
- **Effort:** Large and rising with time.

## Recommendation

**Option A**, with four design choices baked in:

1. **Allowlist manifest** drives the export; denylist is the second gate
   (Correction 2).
2. **Pure filter, no transforms** — split files rather than diverge content
   (Correction 3).
3. **Relocate private docs** under `docs/private/` so `docs/llms.txt` is identical
   in both repos (Correction 3).
4. **Build the export harness first, not last.** OSS-14's script lands in Phase 0
   as a *dry-run* — `npm run oss:export` filters into `tmp/oss-export/` and
   `npm run oss:check` builds and lints that tree. Wire it as a **non-blocking** CI
   job immediately, flip it to blocking once green. This turns "is the public
   subset self-contained?" from a question answered once at the end into a
   measurable signal on every PR. OSS-14's *gate* (scans, human checklist, first
   push) still runs last.

The tradeoff accepted: the harness is throwaway scaffolding if the project stalls.
It is worth it — without it, Phase 1's decoupling work has no acceptance test.

One sequencing constraint that follows from the decisions: **keep
`ripls-org/ripls` at private visibility until OSS-17 lands.** The moment the public
repo accepts a contribution — or a Claude dispatcher writes a plan doc into its
`docs/issues/` — the two trees diverge with no merge path. Flip to public only
once this repo has been slimmed and pins the public one.

## Open Questions

- **Where exactly do private-bound docs live?** — Recommendation: `docs/private/`,
  added to `EXCLUDED_PREFIXES`. One prefix, greppable, keeps `llms.txt` identical.
  Alternative (`docs/ops/`, `docs/internal/`) is cosmetic; pick one and move on.
- **Does `server/test_data/manual/` stay in this repo?** — Recommendation: yes,
  private-bound. The eval goldens reference it; make `server/ai/eval` skip cleanly
  when the images are absent so public CI is green, and keep
  `scripts/eval_seed_urls.txt` public so contributors can refetch. Delete the band
  flyer outright (no fair-use story for redistribution *or* retention value).
- **Do the fabricated walkthrough reels ship publicly?** — Recommendation: the
  *fixture packs* (`e2e/fixtures/walkthroughs/{kitchen,classroom,mower,runclub}`)
  ship; the rendered mp4s under `website/content/walkthroughs/` stay private with
  the site. Keeps public LFS small and the e2e suite runnable.
- **Per-file SPDX headers?** — Recommendation: no. Root `LICENSE` satisfies
  Apache-2.0 §4; a 3,400-file header sweep would collide with the module rename.
  Revisit if a downstream consumer asks.
- **Who are the initial public maintainers, and does `auto_assign_author` survive?**
  — Recommendation: repo admins = current maintainers; drop
  `DEFAULT_RENOVATE_REVIEWER` in favor of `CODEOWNERS` on the public side.
- **Does `test_web_e2e` run on hosted runners?** — Recommendation: try it (Docker
  is available on `ubuntu-latest`); if it's too slow, route it through the same
  trusted-dispatch bridge as the Claude jobs rather than registering runners.
- **Does the private side get write access to the public repo?** — It must:
  issues, PRs, labels, contents (push) and `actions: read` on `ripls-org/ripls`.
  Recommendation: a GitHub App installation scoped to the public repo rather than
  a long-lived PAT, since it is the single most powerful credential in the system.
- **Do the three stage labels get created on the public repo?** — Yes;
  `claude_plan_issue.yaml:158-166` and `claude_implement_plan.yaml:192-200` both
  create them idempotently, so they self-heal once the private side has label
  write access.

## Implementation Plan

Dependency graph (→ = must precede):

```
OSS-1 ─┬─► OSS-16
       └─► OSS-10
OSS-14a (harness) ──► [all Phase 1/2 work validated against it] ─► OSS-14b (gate) ─► OSS-17
OSS-3a ─► OSS-9    (server must not import website/ before Branding lands)
OSS-5a ─► OSS-14b  (doc relocation before the link check can pass)
OSS-4  ─► OSS-15   (brunch paths feed the filter-repo path list)
OSS-{3a,5a,6,7,8,9,11,12,13} ─► OSS-10 (rename last)

OSS-2 (rotation) ─► OSS-15 (rewrite)     ← a separate track. Neither gates
                                            the public push; see Correction 7.
```

*Revised 2026-10-31:* OSS-17 is now #3228 (one public repo) and OSS-15 is #3229
(archive the old history). OSS-2 (#2954) was closed as not planned. #3228 should
land with or after the Forgejo migration, so deploy and release are rewired only
once. #3229 follows the #3228 cut-over, and its purge is optional hygiene on a
private archive.

### Phase 0: Foundations
**Goal:** Decisions are executable, the target repo exists, and there is a
measurable definition of "the public subset."
**Tasks:**
- [ ] #2967 — create `ripls-org` + `ripls-org/ripls` (private visibility, LFS on,
      branch protection, DCO check, zero self-hosted runners). Add `LICENSE`,
      `NOTICE`, `TRADEMARK.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`,
      `SECURITY.md` **to this repo's root** (they are public-bound files, and the
      no-transform rule means they must exist here to ship there). Note the
      lifecycle: once OSS-17 removes the product code from this repo, an
      Apache-2.0 `LICENSE` at this root would be describing a tree that no longer
      contains the licensed work — OSS-17 removes or rescopes it here.
      *(Moot under #3228: the licensed work stays in the one repo, and this
      repo is archived.)*
- [ ] #2967 — **re-decide #1773** (Correction 6): GitHub Secret Protection with
      push protection is free on public repos, and "the repo opens to outside
      contributors" is the documented trigger. Turn it on for `ripls-org/ripls`
      before it accepts contributions, and correct `docs/secrets.md:21`.
- [x] ~~#2954~~ — closed as not planned 2026-10-30. The Mapbox client token
      still needs restricting by bundle id and URL before #3228 publishes
      `env.*.json`. Original task: walk #1767's 12 rows to a recorded verdict per Correction 7's
      triage; restrict the Firebase / Maps / Mapbox client keys in console.
      **Human-gated, and on its own track — it does NOT block the public push**
      (the gate is a clean scan of the export). Sequenced in Phase 0 because the
      JWT secret needs a comms window, not because anything waits on it.
- [ ] #2966 (part a) — `scripts/oss_export.sh` + `oss_manifest.txt` allowlist +
      `scripts/oss_export.test.js` (the matcher decides what becomes public —
      it gets unit tests) + `npm run oss:export` / `oss:check`; non-blocking CI
      job. Add `.env.local` to `.gitignore` in the same PR (OSS-13 consumes it).
**Validates:** The manifest forces an explicit, reviewable answer to "what is
public?" before any code moves — and the first `oss:check` run enumerates exactly
how much of `server/` and `app/` currently fails to stand alone.

### Phase 1: Separate — make the public subset independent of the private one
**Goal:** `npm run oss:check` builds and lints green.
**Tasks:**
- [ ] #2955 — `git mv` the six embedded assets out of `website/` into
      `server/services/web/assets/` (new Go package ⇒ `doc.go` + `README.md`);
      delete `website/embed.go`; fix `service.go:20`, `routes.go:102`,
      `Dockerfile:44`. **`website/` stays; the site deploy keeps working.**
      Retarget release-notes generation, design-token output and walkthrough
      export so they don't write into public-bound paths. The two
      `.well-known/` files are org account identity (Apple Team ID, Android
      signing fingerprints) — they must become deploy-time-supplied, not
      embedded, or the public binary carries them.
- [ ] #2957 — create `docs/private/`, add it to `EXCLUDED_PREFIXES`
      (`scripts/gen_context_map.js:64-71`), `git mv` the strategy/runbook docs
      there, split `secrets.md` and `walkthroughs.md` per Correction 4's table,
      fix the four dangling links, scrub the retained docs' project IDs and
      emails, fix `walkthroughs.md`'s stale `context: globs`, regenerate
      `docs/llms.txt`.
- [ ] #2956 — delete the brunch fixture pack and its rendered reel; retire or
      privatize `server/cmd/walkthrough-export/`; swap the real name in
      `content_test.go:51-52`.
- [ ] #2964 — make eval tests skip without `server/test_data/manual/`; delete the
      band flyer; inspect the lost-pet posters for legible phone numbers; strip
      EXIF GPS; write `NOTICE` entries; add the OFL file to `app/fonts/`.
- [ ] #2965 — parameterize `package.json` and `scripts/` off `GCP_PROJECT_DEV/PROD`
      in a gitignored `.env.local`; relocate the private `.claude` skills; scrub
      the **whole** `.gitleaks.toml` `regexes` block (`:48-66`, not just `:60-65`
      — `:57` and `:65` carry real key/token prefixes) in the same PR that
      OSS-11 untracks the Firebase configs, so the `paths` allowlist at `:28-30`
      is never stale.
**Validates:** The public subset compiles and tests without any private path —
this is the load-bearing proof of the whole project.

### Phase 2: Neutralize — remove org identity from what ships
**Goal:** The export tree contains no org identifier outside `TRADEMARK.md`.
**Tasks:**
- [ ] #2961 — `config.Branding` struct + flags; blank the org defaults; route the
      l10n/email/SSR brand strings and the hardcoded password-reset URL through it.
- [ ] #2963 — `.example` Firebase/env configs; Gradle/xcconfig/manifest
      placeholders for bundle IDs, schemes and App Links; `{appName}` ARB
      placeholder (the SMS-consent string at `app_en.arb:252` must not ship
      verbatim); blank `DEVELOPMENT_TEAM`.
- [ ] #2958 — blank the four `terraform/modules/containers/variables.tf` defaults
      (incl. the legal name + street address at `:149`); add
      `var.service_id_prefix`; add `environments/example/`.
- [x] #2959 + #2970 — `vars.*` parameterization; every public workflow to
      `ubuntu-latest`; collapse the **eight** `claude_*` workflows into gated
      dispatchers + the private-side `agent_dispatch.yaml`; verify with a fork
      negative test. **Done 2026-10-30**, except the fork negative test, which
      was dropped: a migration to Forgejo is planned, and the gates were
      audited instead (#2959). Anatomy notes that shaped this work:
      - Only ~21% of the 3,308 lines is boilerplate (a byte-identical ~40-line
        prologue and ~57-line epilogue repeated 7×); the eight prompts are the
        product. The collapse is *moving prompts*, not rewriting logic — factor
        the prologue/epilogue into a composite action first.
      - `GH_REPO: ${{ github.repository }}` appears in all 8 discover/guard steps
        and is interpolated into 7 prompts. On the private side it resolves to the
        *wrong* repo — it must become an explicit input, and every
        `GH_TOKEN: secrets.GITHUB_TOKEN` binding (~20) must become the
        public-scoped PAT/App token.
      - `claude_resolve_conflict` **splits**: its `enumerate` job polls
        `gh pr view --json mergeable` up to 10× (GitHub computes mergeability
        lazily) and must run where the PRs live → stays public; the resolve half
        goes private. It already self-dispatches via `repos/…/dispatches` with the
        PAT because `GITHUB_TOKEN` can't trigger downstream workflows — the same
        constraint recurs verbatim at the public→private hop.
      - `claude_fix_ci_failure` / `claude_fix_renovate_ci`: `workflow_run` fires
        only in the repo owning the run, so the trigger stays public; logs are
        fetchable cross-repo with `actions: read`, so pass `failed_run_id` through.
        `renovate.json` must stay public (the renovate variant reads it via the
        contents API).
      - `claude_refresh_docs` is pure `schedule` with no bridge need — schedule it
        **privately**, targeting the public repo, since it must not put Claude
        secrets in the public repo. Its selection is glob-based, not path-based,
        so relocating `docs/users` barely touches its working set (1 of 13 of
        those docs carries `globs:`), but its `git add docs/` whole-directory
        commit assumes one tree and needs scoping.
      - **Flipping `runs-on` is necessary but nowhere near sufficient, and the
        gap is bigger than the issue implies.** `runs-on` in every public
        workflow now resolves from `vars.CI_RUNNER_LINUX`, falling back to
        `ubuntu-latest` — so the public repo already lands on GitHub-hosted
        runners with no change to the files. But **`test_go.yaml`,
        `test_flutter.yaml` and `test_flutter_web.yaml` install no toolchain at
        all**: no `setup-go`, no `setup-node`, no `flutter-action`, no
        `setup-java`. Go, Node, Flutter, the JDK, the NDK, golangci-lint,
        gofumpt and ONNX Runtime all come from the baked runner image
        (`runners/Dockerfile` ARGs, fed from `versions.env`). On a
        GitHub-hosted runner those three fail at the first step. `test_web_e2e`
        additionally relies on baked Temurin for the Firebase Auth Emulator,
        and `test_go` on the host Docker daemon for testcontainers. Only
        `test_terraform` and `test_cloud_functions` install what they use.
        So OSS-7 owns adding `setup-*` steps driven by `versions.env` — and
        those steps must be **no-ops or harmless** when the baked image already
        provides the tool, since the same file runs on both fleets
        (Correction 3).
      - Minimal dispatch input set: `mode` discriminator + `issue_number`,
        `pr_number`, `trigger_body`, `trigger_kind`, `review_id`, `review_state`,
        `failed_run_id`, `base`, `actor`. Everything else (`head_ref`, `head_sha`,
        `plan_path`) is re-derived server-side from `gh pr view` and the PR markers.
- [ ] #2960 — parameterize the org/repo strings inside `runners/` and `renovate/`
      (the directories stay here).
**Validates:** A grep for the org denylist over the export tree comes back empty
while the running system is unchanged.

### Phase 3: Rename, gate, publish
**Goal:** First push, verified clean.
**Tasks:**
- [x] #2962 — `go mod edit -module github.com/ripls-org/ripls` + import sweep +
      tooling. One PR, nothing else in it. (Landed as `go.ripls.org/ripls` —
      Correction 11.)
- [x] #2966 (part b) — flip `oss:check` to blocking; run the secret / denylist /
      PII / media / license scans; complete the human checklist; push the squashed
      export to `ripls-org/ripls` (still private visibility). Done 2026-09-08;
      second sync and full hosted-runner CI 2026-10-05, which caught one real bug
      (#3153: Docker build check shipped the `.onnx` LFS pointer).
- [x] #2970 — verify the dispatch bridge end-to-end against the real public repo.
      Done 2026-10-23 with the member test (ripls-org/ripls#1 → #2). The fork
      negative test was dropped because of the Forgejo move; the gates were
      audited statically instead (#2959, #3227).
**Validates:** A fresh clone of the public repo builds, tests, and lints green on
GitHub-hosted runners with only `BUF_TOKEN`.

### Phase 4: One repo, archive the rest
**Goal:** One public repo containing product code, CI, deploy, release and infra
config. Everything genuinely private lives outside it. *(Revised 2026-10-31; the
superseded two-repo tasks are kept struck through below for the record.)*
**Tasks:**
- [ ] #3228 (OSS-17) — sort the 1,335 privately classified files into publish,
      move or delete.
      - Publish: runners, terraform environments, deploy and release workflows,
        Firebase and env configs, fastlane minus personal contacts.
      - Move: the website to its own repo; `docs/private/` and the unvetted
        working docs to a private notes repo.
      - Delete: the brunch pack and the export harness.

      The denylist is re-scoped to secrets plus identity and supplied to CI as a
      secret file, and `oss_scan` becomes a required check on the tree. Product
      issues are refiled publicly, user feedback goes to a private tracker, and
      `TODO(#NNNN)` references are renumbered. At cut-over, make a final sync,
      switch development, deploy and release to the public repo, turn on required
      checks, and archive this repo. Land it with or after the Forgejo migration.
- [ ] #3229 (OSS-15) — archive this repo's history privately: a bare repo in
      `/srv/git`, with the GitHub repo archived for its closed issues. A local
      `git replace --graft` lets `git blame` cross the cut-over. Purging the brunch
      pack and old secret files from the archive is optional hygiene, with no
      force-push window to coordinate.
- [x] ~~#2971~~ (closed, superseded by #3228) — subtree-pin `ripls-org/ripls` at the export tag; rewire the deploy,
      release and Terraform paths to build from the pin + overlay; delete the
      product code from this repo; remove or rescope the root `LICENSE` here once
      the licensed work has left; rewrite `CLAUDE.md` / `README.md` for the
      two-repo workflow. **Then flip the public repo to public** — and only then,
      with push protection already on (Correction 6).
      **Superseded 2026-10-05:** the repo was flipped before OSS-17, accepting
      manual port-back of public PRs as the cost. Push protection, secret
      scanning, private vulnerability reporting, external-contributor run
      approval, `main` force-push/deletion protection, and the DCO app went on
      immediately after. Required PRs/checks on public `main` stay off until
      this task reverses the sync direction.
- [x] ~~#2969~~ (closed, superseded by #3229) — `git filter-repo` this repo's
      history (secrets + brunch), coordinate the force-push window, prune LFS.
**Validates:** One full release cut from the public repo, with deploy, store upload
and tag. `oss_scan` and gitleaks pass on the public tree. Nothing in this repo is
still needed, and it is archived.

## Risks

| Risk | Mitigation |
|---|---|
| A "split out `website/`" PR breaks the live site deploy | Correction 1; `website/` never moves |
| Public repo diverges before OSS-17 | ~~Keep it private-visibility until OSS-17 lands~~ (flipped early, 2026-10-05): this repo stays the source of truth, public PRs are ported back by hand, syncs are whole-tree commits; exempt `docs/issues/` from the diff check |
| Gated automation can't open plan PRs publicly | Correction 5 — `docs/issues/` path is public-bound, contents are not |
| `lint:context-map` fails on the first public PR | Correction 3 — `docs/private/` prefix, identical `llms.txt` |
| Module rename collides with everything | Phase 3, alone in its PR, after all content work |
| Force-push (OSS-15) disrupts open PRs and CI caches | ~~Quiet window~~ Moot: #3229 archives the history; any purge runs on an archive nobody works in |
| Publishing deploy and infra config leaks something private | Denylist re-scoped to secrets plus identity, run as a required CI check (#3228); restrict client tokens before publishing `env.*.json` |
| Deploy secrets sit in a repo anyone can fork | Forgejo with registration closed; if the repo stays on GitHub, use environment protection and branch-scoped secrets (#3228) |
| Denylist misses a newly added private file | Allowlist manifest is the primary control |
| Release pipeline breaks mid-migration | Release workflows stay private and untouched until the #3228 cut-over, which rewires them once, with the Forgejo move |
| A secret leaks into the public repo and is scraped before CI reports it | Correction 6 — enable push protection (free on public repos) before outside contributions |
| `.gitleaks.toml` ships our own key prefixes, or allowlists a file we just untracked | Scrub `:48-66` in the same PR as the Firebase untracking |
| A relocation loses `git blame` in this repo | `git mv` for every move (§Conventions) |

## Audit notes

Audited 2026-08-22 via `/ripls-audit`. Engaged dimensions: `security` (secrets,
PII, CI trust boundaries), `dependency` (asset licensing), `tech_debt`, `testing`,
`code_quality`. Eight findings, all applied in place:

1. **`security`** — the plan missed that going public *is* the documented #1773
   trigger, and that the DIY stack has no push block (→ Correction 6, Phase 0
   task, risk row). Materially changes Phase 0.
2. **`security`** — `.gitleaks.toml`'s allowlist ships our own key/token prefixes
   and would sit stale over untracked Firebase paths (→ Correction 6 tail,
   OSS-13 task).
3. **`security`/`code_quality`** — `docs/walkthroughs.md` needs the same
   public-mechanism / private-runbook split as `secrets.md`, and its
   `context: globs` go stale silently (→ Correction 4 table).
4. **`testing`** — the export harness had no test; its allowlist matcher is the
   highest-consequence pure function in the project (→ `scripts/oss_export.test.js`).
5. **`code_quality`** — `git mv` for relocations; new Go package needs
   `doc.go` + `README.md` or `revive.package-comments` fails the build
   (→ §Conventions, Phase 1 tasks).
6. **`tech_debt`** — placeholder work invites bare `TODO`s and phase-label
   comments, both CI-gated (→ §Conventions).
7. **`code_quality`** — `.env.local` needed a `.gitignore` entry alongside the
   OSS-13 parameterization (→ Phase 0 task).
8. **`dependency`/legal** — an Apache-2.0 `LICENSE` at this repo's root becomes
   wrong once OSS-17 removes the licensed work (→ Phase 0 note, Phase 4 task).

Not applied, and why: the `dependency` rubric's package-CVE workflow
(`go list -m -u`, `govulncheck`, pub.dev health) is out of scope — this plan
changes no dependency versions; its licensing concerns are asset licensing, which
OSS-12 (#2964) already owns.

**Docs consulted:** `docs/llms.txt`, `docs/context_map.md` (anchor — index
mechanics and `EXCLUDED_PREFIXES` contract), `docs/secrets.md` (leaf; globs match
`.gitleaks.toml`/`scripts/fetch_secret.sh` — source of findings 1–2),
`docs/walkthroughs.md` (leaf; globs match `e2e/fixtures/walkthroughs/**` —
finding 3), `docs/coding_agent_cheat_sheet.md` (leaf; globs match
`.github/workflows/claude_*` — the plan-PR lifecycle behind Correction 5),
`docs/reviews/security_review_prompt.md` §3 Secrets Management (anchor for the
`security` lens), `docs/reviews/dependency_review_prompt.md` (anchor,
`dependency` lens), `CLAUDE.md` (TODO/`git mv`/README/`doc.go`/`.yaml`
conventions), `scripts/gen_context_map.js`, `scripts/check_todo_refs.js`,
`package.json` lint composition.

## Related Issues

- #1766 / #1767 / #1771 — secrets umbrella, rotation tracker, the history rewrite
  that was closed without executing (revived as #2969)
- #2895 — homepage rewrite; stays a private-repo concern
- #2952 — admin dashboard; ops tooling that stays private
- #1941 — dev Cloud SQL naming; lives with `terraform/environments/`
