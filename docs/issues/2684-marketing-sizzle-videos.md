# Issue #2684: Marketing sizzle-video pipeline: events journey reel from real prod content, generalized for requests + gear

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2684
**Filed:** 2026-07-08 | **Last activity:** 2026-07-08
**Labels:** documentation, enhancement | **Assignees:** unassigned
**Context docs:** docs/issues/2162-web-e2e-harness.md (Phase-3 design intent), docs/workflows/experience.md, docs/planning.md, docs/client/chat.md (media-attach sections), docs/impact_metrics.md + docs/impact_metrics/quality_time.md (scene-6/7 surfaces), docs/testing/architecture.md, docs/users/priorities.md (latent-host thesis), e2e/README.md, plus code research across `e2e/lib/**`, `playwright.config.ts`, `server/storage/**`, `terraform/**`

## Problem Statement

We have exactly one marketing-grade video: the paced phone-onboarding clip
(`e2e/tests/workflows/phone-onboarding-video.spec.ts`), rendered hermetically by
`e2e/scripts/run_marketing_video.sh` + `export_marketing_video.sh`. Everything
about the pipeline is hardcoded to that single spec — the runner, the export
globs, the output names. The fixtures are stock Unsplash imagery.

The goal (and the unbuilt "Phase 3: marketing-video artefacts" of the e2e
harness plan, #2162) is a **fully automatic, reproducible way to render
marketing-worthy videos of core user journeys** — usable both as marketing
assets and as a recurring self-inspection loop: when the UI changes, re-render
the reels and *watch* the core journeys to confirm they still look great and
make sense.

First deliverable: an **events sizzle reel** featuring six journeys —

1. Host creates a new event and shares it with people and communities.
2. Invitee accepts the invitation via the app-less phone flow (`/go` → OTP →
   promote-on-verify → auto-RSVP).
3. Host manages needs: requests items, adds more folks, messages the group
   with enthusiasm about what's still needed.
4. Invitees sign up to bring things ("I'll bring it" claims).
5. People post photos and comments from the event as it happens.
6. Host and invitees explore the event's **impact afterward** — the host
   completes the event and sees the live impact preview (quality time,
   per-metric detail); invitees revisit the recap story and community impact
   metrics.

Content comes from the **real Mother's Day brunch event in prod (May 10,
2026)** — photos, avatars, roster shape, message cadence. It is Bryan's
family; **consent to use the images is confirmed**. Message text may be
paraphrased. The process must generalize so requests and gear-sharing reels
follow with only a new spec + fixture pack.

## Context from Comments

No comments on the issue yet (filed today from an interactive design session;
the issue body itself captures the requirements — journey 6 was added in the
same session). The e2e harness plan (#2162) is the de-facto prior discussion:
it explicitly names "marketing-video generation — screen recordings of common
journeys that re-render whenever the app changes" as a Phase-3 use case and
chose per-context `recordVideo` as the mechanism.

## Research Findings (load-bearing constraints)

These shape every option below; verified against the code 2026-07-08.

**Recording model.** Playwright records one video **per page/context** — there
is no cross-context single capture. Multi-client specs produce N separate
webm files (`global-teardown.ts` flattens them as `<test>-1.webm`,
`<test>-2.webm`, …). A coherent multi-actor film therefore means either
switching one page between actors, or shooting **per-scene clips and
concatenating** in post (ffmpeg already in the export path).

**Chat photo-attach is blocked on Flutter Web.**
`inline_conversation_view.dart` short-circuits `_pickAndStageMedia` through
`WebUnsupported.showPhotoUploadNotice` (`web_unsupported.dart:41-46`) — the
picker never opens on web (deliberate deferral; staged previews use
`dart:io`). Rendering received photo messages works; only authoring is
blocked. `ChatService.SendMessage` accepts `media_ids`, so photo messages can
be seeded via RPC off-camera. Journey 5's on-camera beat must be the
*receiving* side (photos + comments streaming in live) plus an on-camera
*text* comment.

**Needs UI has zero e2e coverage.** The event-scope claim CTA is **"I'll
bring it"** (`experience/needs/view_need_sheet.dart:120`); no spec drives the
Needs widgets today, so locatability on CanvasKit is unproven and probing is
budgeted work. Note the documented constraint
(`docs/client/testing/semantics_identifiers.md`): primitive-level
`semanticsIdentifier`s are **dropped from DOM serialization** — Playwright
must use role/label locators (which find the l10n-sourced `semanticsLabel` on
`Tappable`/`Toggle`/`IconAction`) or a Scaffold-body-level route anchor, not
new primitive identifiers. Similarly, the share sheet's "Invite people" UI
path has never been driven by a spec (only `ShareItem` via RPC); "Invite
community" and "Copy Link" are proven locators.

**Impact surfaces exist for journey 6.** The completion flow
(`event_modal_view_model.dart`) live-previews impact via
`PreviewExperienceImpact` (debounced as the host edits attendance), with
per-metric completion modals (`widgets/impact/completion/` — quality time,
value, CO2 — plus edit/select chips for per-attribute overrides). Post-
completion, attendees get the recap story (with ESM pulse-check) in the feed,
and aggregate views live in `screens/impact_metrics/` (community metrics,
user-community impact, per-metric drill-downs). None of these are e2e-driven
today — same probing caveat as Needs.

**Seed layer.** `e2e/lib/seed/` covers users+avatars, locations, experiences
(+`uploadMedia` for arbitrary images), share links, attendees with RSVPs,
provisional users, gear, requests — but has **no helpers for chat messages or
needs/contributions**. The generated Connect clients exist; the helpers are
small additions.

**Prod extraction path.** Prod Cloud SQL (`ripls-prod:us-central1:ripls-prod`,
Postgres 15, database `ripls`, user `ripls_user`, password in Secret Manager
`ripls-db-password`) is **private-IP-only** (`ipv4_enabled=false`,
`ssl_mode=ENCRYPTED_ONLY`) and the shared Terraform module
(`terraform/modules/storage/main.tf:79-83`) has **no authorized-networks
variable** — the temporary public IP must be patched **out-of-band via
`gcloud sql instances patch`** and reverted the same way (never through the
shared module: a `-target` apply would drift/restart dev *and* prod).
Media binaries live at `gs://ripls-prod/{userId}/{mediaId}` (+`-thumb`); the
exact `gs://` URI is stored on each `media` row. Repeated/map proto fields
(`media_ids` ordering, reactions, read maps) are only faithful in the
`binary_proto` column, so extraction should reuse
`storage.InitializePostgreSQLDatabase(dbURL, storage.DefaultStorageTypes())`
from Go (pattern: `server/cmd/seed-purge-fixture/main.go`) rather than raw
SQL. Tables: `experience`, `experience_rsvp`, `planning_need`,
`planning_contribution`, `chat_conversation`, `chat_message`, `media`,
`user`, `provisional_user`. The repo is **private**, so committing consented
family imagery is acceptable; the manifest must record provenance + consent
so the pack is purged if visibility ever changes.

## Potential Solutions

### Option A: One continuous film per reel (single page, actor switching)
- **Approach:** One `@marketing` test on one page; between journey beats,
  `clearAuth` + `injectAuth` as the next actor and reload. One webm out.
- **Pros:** Single capture, no post-stitching; simplest export.
- **Cons:** Actor switches film as jarring reload/flash frames — the seams are
  *in* the footage and can't be edited out without re-encoding anyway; a
  failure in beat 5 loses the whole take; per-beat re-rendering impossible
  (the whole reason we want cheap re-renders as the UI evolves); auth-swap
  reloads on one context are the least-proven pattern in the harness.
- **Effort:** Medium.

### Option B: Scene-based reel — one test per scene, ffmpeg concat (recommended)
- **Approach:** One spec file per reel (`e2e/tests/marketing/events-sizzle.spec.ts`)
  containing a `test.describe.serial` of **scene tests**, each filming one
  actor on one fresh recording context (`newRecordingContext` + `injectAuth` +
  banner-strip + `settle()` pacing). Scenes share the worker's server + DB, so
  the world accumulates across scenes (the host's event from scene 1 is the
  same event guests join, plan around, and recap in scenes 2–7). Everything
  not on camera is seeded/driven via RPC (new chat + needs seed helpers),
  including *live* off-camera actions mid-scene (RSVPs landing, photo
  messages streaming in). A generalized `export_marketing_video.sh <reel>`
  concatenates the scene clips in declared order into `<reel>.webm` +
  `<reel>.mp4` + per-scene deliverables + QC frames.
- **Pros:** Per-scene re-render and QC (the self-inspection goal); a scene
  failure doesn't lose the reel; scene list doubles as the storyboard in the
  process doc; matches how the harness already names/collects multi-clip
  output; marketing can also use individual scenes as short clips (often more
  useful than one long reel).
- **Cons:** ffmpeg concat step (already in the pipeline); scene boundaries are
  hard cuts (acceptable — title cards/transitions are explicitly out of scope).
- **Effort:** Medium.

### Option C: Post-hoc assembly from the standard suite's videos
- **Approach:** No new specs; cherry-pick clips from the existing workflow
  specs' always-on videos and stitch them.
- **Pros:** Near-zero new code.
- **Cons:** Workflow specs are correctness gates, not paced showcases — no
  dwell time, test-fixture names ("Riley Chen", `+1555…`), assertion detours
  on camera, stock imagery; violates the "marketing-worthy" bar and the
  storyboard has no home. Rejected.
- **Effort:** Small (and inadequate).

## Recommendation

**Option B.** It is the only option that satisfies both halves of the goal —
marketing-worthy output *and* cheap per-journey re-rendering for
self-inspection — and it composes from proven harness pieces
(`newRecordingContext`, `injectAuth`, RPC seeding, the `marketing-video`
project, ffmpeg post-processing). The onboarding baseline already proved the
single-actor-on-camera / world-seeded-off-camera formula; Option B is that
formula, N times, with a concat step.

For prod content: a read-only Go extractor command
(`server/cmd/marketing-export`) reusing `DefaultStorageTypes()`, run against a
temporarily public-IP'd prod instance (documented `gcloud patch` / revert
runbook, low-traffic window), pulling one experience's full object graph to a
committed fixture pack under `e2e/fixtures/marketing/brunch/` with a
provenance + consent manifest and PII scrubbing (first names only; no emails,
phone numbers, auth fields; fictional phone fixtures at film time).

## Open Questions

- **Journey 5 on-camera beat: receiving side only?** Web cannot author photo
  messages (deliberate deferral in `web_unsupported.dart`). — Recommendation:
  film the *attendee receiving* photos + comments streaming in live (seeded
  via RPC mid-scene) and typing an on-camera text comment. Do **not** build
  web chat photo upload for this issue; if we want that beat filmed from the
  author's side later, file it as its own feature issue.
- **How do we make needs/impact widgets locatable if probing fails?** Locators
  for the Plan tab sheets, the completion impact preview, and the impact
  metrics screens are unproven on CanvasKit. — Recommendation: probe with
  role/label locators first (the pattern that carried every other surface).
  If a widget is genuinely unreachable, the documented fixes
  (`docs/client/testing/semantics_identifiers.md` § "What to do instead") are:
  add an l10n-sourced `semanticsLabel` to the primitive (a11y-positive, and
  role/label locators find it) or a Scaffold-body-level route anchor for a new
  sheet — **not** primitive-level `semanticsIdentifier`s, which are dropped
  from DOM serialization and must not be relied on by Playwright. Same policy
  for the share sheet's "Invite people" path; if it resists quick probing,
  scene 1 shows "Add community" + link share on camera and seeds individual
  invitees via RPC (they're off-camera cast anyway).
- **Does the impact scene need time travel?** The reel films journeys 1–5 on
  a *future* event, but journey 6 needs a completable, past-feeling event and
  a populated recap. — Recommendation: no clock manipulation; `CompleteExperience`
  is legal from ACTIVE/JOINED/IN_PROCESS regardless of event time (the
  past/informal path skips IN_PROCESS), so scene 6 completes the same event
  the reel built. Verify the recap story + ESM render for a just-completed
  event; if a detour is needed, seed a sibling completed event off-camera for
  scene 7 instead — decide during Phase 3 probing.
- **Where does the process doc live?** The user asked for a README in the
  marketing directory; the repo also has the `docs/llms.txt` context-map +
  nightly freshness ratchet for durable docs. — Recommendation: put the full
  process doc at **`docs/marketing_videos.md`** (with `context:` frontmatter so
  it's indexed and freshness-ratcheted — this doc is exactly the kind that rots
  when the UI evolves), plus a short `e2e/fixtures/marketing/README.md` for
  fixture provenance/consent rules pointing at it. `e2e/README.md` gets a
  section pointer.
- **Consented family imagery in a private repo.** Committing is fine today;
  the manifest records consent + provenance. — Recommendation: manifest gains
  a `consent` block per fixture pack; add a standing note that the `brunch/`
  pack must be purged from history if the repo ever goes public. Names in
  fixtures: first names only.
- **Exact scene count for the events reel.** Six journeys → seven scenes
  (journey 6 has two actors, and scenes are one-actor-one-context by
  construction). — Recommendation: keep 1 scene : 1 on-camera actor-beat; the
  export script takes an ordered scene list so trimming/merging later is a
  config change.

## Implementation Plan

### Phase 1: Generalize the render pipeline
**Goal:** Any `@marketing` reel renders with one command; onboarding baseline
migrates onto the general path (no behavior change to its output).
**Tasks:**
- [ ] `e2e/lib/marketing.ts`: shared scene toolkit — `settle()` pacing,
      human `typeInto`, emulator-banner strip, `openScene(browser, {actor,
      slug, sceneIndex})` wrapping `newRecordingContext` + `injectAuth` +
      `installRequestIdOverride`, and a scene-manifest writer (ordered scene →
      clip mapping for the export step).
- [ ] `run_marketing_video.sh <reel-slug>`: parameterize spec selection
      (`tests/marketing/<reel>.spec.ts` or grep), keep the hermetic
      throwaway-Postgres lifecycle unchanged.
- [ ] `export_marketing_video.sh <reel-slug>`: per-scene clips →
      `marketing-videos/<reel>/scene-NN-<name>.{webm,mp4}` + ffmpeg-concat
      `<reel>.webm/.mp4` + QC frames; keep the single-clip path for
      onboarding.
- [ ] Move/alias the onboarding spec under the reel convention
      (`tests/marketing/onboarding.spec.ts`) with `git mv`; keep the
      `@marketing` tag + project untouched.
**Validates:** The pipeline abstraction is right — the existing clip
reproduces equivalent output (same scenes, resolution, pacing; video encodes
are not byte-stable) through the generalized path.

### Phase 2: Extract the Mother's Day brunch content from prod
**Goal:** A committed, consented, PII-scrubbed fixture pack with everything
the events reel needs.
**Tasks:**
- [ ] `server/cmd/marketing-export`: read-only Go command (pattern:
      `seed-purge-fixture`) — locate the experience (name/date match,
      interactive confirm), walk RSVPs → users/provisional users → avatars,
      conversation → messages (+`media_ids`), needs/contributions, media rows;
      emit `content.json` (roster, message script, needs list, timings) with
      PII scrub (first names; drop emails/phones/auth fields). Repo
      conventions apply: `doc.go` package comment, README if multi-file, and
      **unit tests for the scrub/shaping logic** (it's a pure function and
      the part that must not regress).
- [ ] Least-privilege DB access: connect as the existing **read-only
      `metabase_reader` user** (`metabase-reader-password` in Secret Manager,
      SELECT-only per `scripts/metabase-readonly-user.sql`) — if the protosql
      init path insists on write privileges, add a read-only open mode rather
      than escalating to `ripls_user`.
- [ ] Runbook (in the Phase-4 doc, drafted now): `gcloud sql instances patch
      ripls-prod --assign-ip --authorized-networks=$MYIP/32` → run extractor
      (`sslmode=require`, password via `scripts/fetch_secret.sh ripls-prod …`
      passed through env, never echoed or logged) → `gsutil -m cp` each
      `storage_url` (+thumbs) → **revert** (`--no-assign-ip
      --clear-authorized-networks`); low-traffic window (patch briefly
      reconfigures the instance); verify revert with `gcloud sql instances
      describe`. The raw export (pre-scrub dump) stays out of git — only the
      scrubbed pack is committed.
- [ ] Fixture pack `e2e/fixtures/marketing/brunch/`: hero + event photos +
      avatars (recompressed to fixture-appropriate sizes), `content.json`,
      and `manifest.json` with provenance (`prod experience <id>, exported
      <date>`) + consent block; purge-if-public note.
- [ ] Confirm with Bryan the final image selection before commit (consent is
      for the set; curation is editorial).
**Validates:** Real content is in hand and shaped for seeding; prod access
pattern is proven and documented (and reverted).

### Phase 3: The events sizzle reel
**Goal:** `run_marketing_video.sh events-sizzle` renders the seven-scene reel.
**Tasks:**
- [ ] New seed helpers: `sendChatMessage({experienceId, text, mediaIds?,
      asUser})` (resolves the event conversation via
      `GetConversationForExperience`), `addNeed` / `claimNeed` /
      `addContribution` wrappers, and `completeExperience` if the completion
      beat needs off-camera assists.
- [ ] Probe locators for the Needs sheets, share-sheet "Invite people", the
      completion impact preview, and the recap/impact screens; where a widget
      is unreachable, add an l10n-sourced `semanticsLabel` or a route-level
      anchor per `semantics_identifiers.md` (separate small app commit — no
      primitive-level `semanticsIdentifier`s for Playwright use).
- [ ] `e2e/tests/marketing/events-sizzle.spec.ts` — `test.describe.serial`,
      seven scenes, brunch fixture content throughout:
      1. *Host creates & shares* (on camera: unified-create → Save Event →
         share sheet → add community / copy link; off camera: individual
         invitees via `ShareItem`).
      2. *Invitee joins by phone* (on camera: `/go` landing → OTP → RSVP'd;
         reuses the onboarding formula with brunch skin).
      3. *Host rallies the crew* (on camera: Plan tab → add needs → an
         enthusiastic chat message; off camera mid-scene: RSVPs land live).
      4. *Friends pitch in* (on camera: invitee → need sheet → "I'll bring
         it"; off camera mid-scene: other claims + chat pills land).
      5. *Day-of* (on camera: attendee in the event chat as photo messages +
         comments stream in via RPC; types a comment; hold on the filled
         chat).
      6. *Wrap-up — the host sees the impact* (on camera: host completes the
         event; the completion sheet's live impact preview fills in — quality
         time with the whole crew; open a per-metric detail modal; confirm).
      7. *The ripple — invitees revisit it* (on camera: invitee opens the
         recap story in the feed — brunch photos, ESM pulse — then the
         community impact metrics view; hold on the quality-time aggregate).
- [ ] Render, QC frames, iterate pacing until marketing-worthy.
**Validates:** The full pipeline — seeded world, multi-scene capture, concat —
produces the deliverable; journey gaps (needs/impact locators, invite-people
path, just-completed recap rendering) are resolved or explicitly routed
around.

### Phase 4: Process documentation
**Goal:** Anyone (or the nightly doc ratchet) can re-render, extend, and trust
the reels.
**Tasks:**
- [ ] `docs/marketing_videos.md` with `context:` frontmatter — the process
      doc: reel catalog + storyboards, fixture packs + provenance/consent
      rules, extraction runbook, render/export commands, pacing knobs,
      how-to-add-a-reel (requests, gear next), re-render-on-UI-change policy;
      `npm run generate:context-map`. Explicitly codify the `@marketing`
      exception to `e2e/README.md`'s "state changes happen through the UI"
      rule: sizzle specs are showcases, not correctness gates, and off-camera
      RPC mid-scene is by design there (the correctness versions of these
      journeys stay in the workflow specs).
- [ ] `e2e/fixtures/marketing/README.md` (fixture provenance + consent rules,
      pointer to the doc); section pointer in `e2e/README.md`; mark #2162
      Phase 3 delivered.
**Validates:** The "reproducible even as the UI evolves" goal has an owner:
the indexed, freshness-ratcheted doc.

### Phase 5 (follow-up): Requests + gear sizzle reels
**Goal:** Prove the generalization claim.
**Tasks:**
- [ ] File follow-up issues for a requests reel and a gear-sharing reel
      (storyboards sketched in `docs/marketing_videos.md` now); each should
      need only a spec + fixture pack.
**Validates:** New reel ≈ new spec + content, nothing else — the acceptance
test for this whole issue.

## Related Issues
- #2162 — e2e web harness plan; this issue delivers its unbuilt Phase 3
  (marketing-video artefacts).
- #2492 — phone-first pivot epic; journey 2 films its flagship flow, and the
  onboarding baseline spec came from its design iteration.
- #2157 — item-creation photo upload was web-enabled there; chat attach on
  web remains deliberately deferred (`web_unsupported.dart`) and stays out of
  scope here.

## Audit notes (2026-07-08, /ripls-audit)

Changes applied by the audit:

- **Locator strategy corrected against a documented decision.** The draft
  proposed adding `semanticsIdentifier`s to needs/impact widgets if probing
  is flaky; `docs/client/testing/semantics_identifiers.md` (from #2162
  Phase-0/2 mapping) documents that primitive-level identifiers are dropped
  from DOM serialization and Playwright must not rely on them. Replaced with
  the documented remedies: l10n-sourced `semanticsLabel`s (role/label
  locators find them) or Scaffold-body-level route anchors.
- **Least privilege for the prod extractor.** Added the read-only
  `metabase_reader` user (already provisioned, SELECT-only) as the required
  connection identity, with a fallback path that adds a read-only open mode
  rather than escalating to `ripls_user`.
- **Secret handling in the runbook.** Password via `scripts/fetch_secret.sh`
  through env, never echoed/logged; raw pre-scrub export stays out of git.
- **Repo conventions for the new Go command.** `doc.go` package comment,
  README, and unit tests for the PII-scrub/shaping logic.
- **Testing-philosophy exception codified.** The process doc must state that
  `@marketing` specs are showcases exempt from the e2e "state changes happen
  through the UI" rule, with the workflow specs remaining the correctness
  gates.
- Accuracy: Phase-1 validation reworded (video encodes aren't byte-stable).

Docs consulted: `docs/llms.txt`, `docs/testing/architecture.md`,
`docs/client/testing/semantics_identifiers.md`,
`docs/reviews/security_review_prompt.md` (Areas to Review),
`docs/reviews/testing_review_prompt.md` (Areas to Review), `docs/secrets.md`,
`docs/workflows/experience.md`, `docs/planning.md`, `docs/client/chat.md`,
`docs/impact_metrics.md` (map entry), `docs/users/priorities.md`,
`docs/issues/2162-web-e2e-harness.md`, `e2e/README.md`.
