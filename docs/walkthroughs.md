---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: User journey walkthrough pipeline (#2684) — scene-based Playwright reels of core user journeys rendered from the hermetic e2e stack, fixture packs (fabricated, or real-content under consent/provenance rules), and how to add the next reel.
  globs: [e2e/tests/walkthroughs/**, e2e/lib/walkthrough.ts, e2e/lib/capture.ts, e2e/scripts/run_walkthrough.sh, e2e/scripts/export_walkthrough.sh, e2e/scripts/build_walkthrough_page.mjs, e2e/scripts/build_walkthrough_page.test.mjs, e2e/scripts/fetch_walkthrough_fixtures.mjs, e2e/fixtures/walkthroughs/**]
  triggers: [walkthrough, user-journey, reel, video, screen-recording, fixture-pack, kitchen, journey-video, demo-video]
  lens: [testing, infra]
freshness:
  verified_commit: "402d4d9ff"
  verified_on: "2026-08-22"
---
# User journey walkthroughs

A fully automatic, reproducible way to render **share-worthy videos of
core user journeys** from the Playwright e2e harness (#2684, completing
harness plan #2162's Phase 3). The reels serve double duty:

1. **Walkthrough assets** — paced, realistic screen captures ready for external
   editing (no voiceover/titles here).
2. **A self-inspection loop** — when the UI changes, re-render the reels and
   *watch* the core journeys end-to-end to confirm they still look great and
   make sense. If a reel breaks or looks wrong, a journey regressed.

## Rendering a reel

Build once, then one command per reel — hermetic (throwaway Postgres
testcontainer, own server, reaped on exit):

```bash
npm run build:e2e                      # once per app/server change
e2e/scripts/run_walkthrough.sh events  # or: onboarding, requests, gear
```

`build:e2e` = `build:web:e2e` + `go build -o tmp/server ./server`, **in that
order** — the Flutter Web bundle is *embedded in the server binary* at Go
build time (`//go:embed app_assets`, `server/services/web/app_embed.go`), so
rebuilding the bundle alone changes nothing served. Both the runner and
`e2e/lib/server.ts` fail fast when `tmp/server` is older than the bundle, so
a stale binary can't silently render an old app.

Deliverables land under `e2e/videos-walkthroughs/<reel>/` (gitignored):
`scene-NN-<name>.webm` per-scene masters (untrimmed), `scene-NN-<name>.mp4`
per-scene clips (head-trimmed — see below), `<reel>.mp4` the assembled reel,
`walkthrough.html` the scene-by-scene walkthrough page (when the reel has
walkthrough copy — see below), and `frames/` QC stills (one every 2s) —
**review the frames after every render**.

Pacing knobs: `WALKTHROUGH_PAUSE_MS` (per-screen dwell, default 2200),
`WALKTHROUGH_TYPE_DELAY_MS` (per-keystroke, default 60).

**Render one reel at a time.** The stack binds a single fixed port, so a
second concurrent run drives its scenes against the *first* run's server and
DB — no error, just plausible clips of the wrong world and random locator
flakes. The runner refuses to start when the port is busy; pass `E2E_PORT` to
override deliberately.

**Desktop-aspect evaluation render (#2912).** The same reel at a 16:9 laptop
viewport (1440×810, `e2e/lib/capture.ts` → `DESKTOP_VIEWPORT`), for eyeballing
desktop/adaptive layout changes:

```bash
WALKTHROUGH_VIEWPORT=desktop e2e/scripts/run_walkthrough.sh events
```

Output lands under `e2e/videos-walkthroughs/<reel>-desktop/` — a separate,
`-desktop`-suffixed directory so it never clobbers the phone render — and this
mode skips the website deploy step (see `export_walkthrough.sh`); it is
local-only, never rendered to `website/content/walkthroughs/`.

## Reel catalog

| Reel | Spec | Scenes |
|---|---|---|
| `onboarding` | `e2e/tests/walkthroughs/onboarding.spec.ts` | 1 — app-less phone-first guest onboarding (epic #2492) |
| `events` | `e2e/tests/walkthroughs/events.spec.ts` | 7 — host creates+shares · guest joins by phone · host plans needs · chat rallies the crew · friends claim needs · day-of photos stream in · wrap-up with impact preview, closing on the confirmed roster |
| `gear` | `e2e/tests/walkthroughs/gear.spec.ts` | 6 — shared-food giveaways (#2687): photo→listing creates (image mode) · multi-community share · single claim · contested claim decided in the item chat · recipient selection · handoffs |
| `requests` | `e2e/tests/walkthroughs/requests.spec.ts` | 5 — lawn-mower request (#2686): typed sentence→request (text mode) + community share + link copy · appless guest offers via SSR landing + phone OTP · thanks in the request's thread (live reply) · Mark Fulfilled with helper confirm + impact · closing impact view |
| `classroom` | `e2e/tests/walkthroughs/classroom.spec.ts` | 4 — teacher's classroom supply drive (#2703): a sentence that names the whole supply list → the request born with all five pieces as claimable needs (create extracts them, #2731) + community share + link copy · parents deliver mostly by GIVING items (giveaway offers) with one LENDING a whiteboard, landing on the teacher's view with Giving/Lending tags · appless parent joins from the WhatsApp group by phone OTP and grabs the last piece · Mark Fulfilled DELIVERS the confirmed gifts (giveaways complete, loan goes active — the #2703 server fix) and closes on the impact |
| `runclub` | `e2e/tests/walkthroughs/runclub.spec.ts` | 7 — a Wednesday morning run that becomes a standing group: host posts it + copies the link · a runner joins by phone off that link · six have joined by Tuesday evening · the run happens and a runner reads its thread · **Wrap up** from the card foot confirms who came · the People tab's nameless group is named "Hump Day Milers" and gains an invite link via the group page's **Invite** · Home offers to schedule the run again, opening next Wednesday pre-filled |

The `gear` reel's content is the **kitchen pack**
(`e2e/fixtures/walkthroughs/kitchen/`) — fully **fabricated** (no real
people, no prod extraction; the consent/purge rules below don't apply).
Its images are Unsplash stock at the fixtures top level; the `food-*.jpg`
fixture filenames are load-bearing — the e2e AI provider keys its
deterministic image-mode listings on them
(`server/ai/provider_e2e.go` → `e2eGearDetectionForFilename`).

The `requests` reel's content is the **mower pack**
(`e2e/fixtures/walkthroughs/mower/`) — also fully fabricated, and
copy-only (`walkthrough.md`; the lawn-mower hero is Unsplash stock at the
fixtures top level). Its text-mode create relies on the e2e provider's
request generation echoing the prompt (`GenerateRequestFunc` in
`server/ai/provider_e2e.go`), and the guest scene reuses the phone-first
OTP loop — the correctness twin is
`e2e/tests/workflows/phone-request-offer-full-loop.spec.ts`.

The `classroom` reel's content is the **classroom pack**
(`e2e/fixtures/walkthroughs/classroom/`) — fully fabricated, copy-only
(`walkthrough.md`; the classroom hero and the item photos are Unsplash stock
at the fixtures top level). It is the first reel to exercise the **request-side
Needs & Contributions layer** (`docs/planning.md`) and the **complete-on-fulfill
delivery of gear offers** (#2703 server fix). A teacher posts a request whose
text names its whole supply list, so create extracts every item and the request
is **born with one claimable need per named thing** — five names in one sentence
become five needs, no pasting or re-typing (#2731). Parents fulfill it mostly by **giving** items
and one by **lending** a whiteboard, each a real escalated offer seeded via
`claimRequestNeed` + `offerRequestTransfer` (`e2e/lib/seed/transfers.ts`).
Three app facts shape the shots: an **escalated give/lend offer is visible to
the requester** (it rides the `gearOffers` array with a Giving/Lending tag),
unlike a plain claim pill which only the contributor sees; the Mark Fulfilled
modal **pre-confirms every offerer/contributor**, so the teacher credits the
crew by simply confirming (tapping a tile would *remove* someone); and
**marking the request fulfilled now DELIVERS** the confirmed gifts — giveaways
complete, the loan goes active — so the seeded gear needs real value/weight
(via `seedSharedGear`'s `impact`) or the closing impact lands at $0.

The `runclub` reel's content is the **runclub pack**
(`e2e/fixtures/walkthroughs/runclub/`) — fully fabricated, copy-only
(`walkthrough.md`; the hero, the two day-of photos, and every portrait are
Unsplash stock at the fixtures top level). It is the first reel where **time
passes**: `setFilmedAt` is re-declared at the top of each scene (both
`lib/filmed-at.ts` readers resolve it lazily, so the seed RPCs and the scene's
page both move), and the story runs Monday → Tuesday → Wednesday. A recurring
group is a story about elapsed time; held to one instant the host would be
filmed creating a run that had already happened. It closes on affordances a rhythm
needs and that did not exist before it: the **group page's Invite action**
(`CommunityPublicScreen`'s header row — the People tab's group page had
Message / Plans / Library and an overflow with only "Edit name"), a promoted
**Wrap up** at the foot of the Who's-in card (previously only inside the ⋯
manage sheet), and the **host prompt on Home** that opens the
`GenerateWorkshopDraft` prefill — same name, place, photo, the confirmed
attendees, and the next weekly slot. That draft path existed end to end but
had no shipping surface: its `schedule_repeat` nudges were filtered out of
both pools and their only reader left the nav with #2568. The wrapped event
also keeps a **"Schedule the next one"** chip, so the draft has a second route
for a host who never leaves the event.

Three facts shape the shots. **Marcus keeps his initials**: he registers in the
browser by phone OTP, so his tokens never pass through the test process (on
web the bundle keeps them in `flutter_secure_storage`, not the plain
localStorage keys `lib/auth.ts` injects) and `SaveUser` is self-only, so
nothing off camera can give him a portrait. The **read surface is
`ExperienceReadShell`, not `ExperienceEventPane`** — the pane renders only in
edit mode, which a terminal event has none of, so any affordance a wrapped-up
event needs belongs in the shell. And the Home prompt only appears once the
momentum engine has materialized it, which happens in a detached goroutine off
`GetWorkshopBrief`: scene 7 asks for the brief and then polls `GetHomeView`
until the prompt is really there, rather than filming whatever the first load
happened to hold.

Two product bugs surfaced only because this reel drove the real thing.
`InboxNudge` ranked any nudge with imagery above one without, which buried the
(imageless) host prompt behind a generic card forever; and `openRepeatDraft`
read from its `WidgetRef` after the draft round trip, which throws once the
inbox's optimistic dismiss has unmounted the nudge that owns the ref. Both are
fixed with regression tests — see [nudges.md](nudges.md).

## How a reel works

- **One scene = one test = one actor on one page = one clip.** Playwright
  records one video per page, so a coherent multi-actor story is shot as
  per-scene clips and concatenated (ffmpeg) by
  `e2e/scripts/export_walkthrough.sh`. Scene tests run serially in one
  worker (`test.describe.configure({ mode: 'serial' })`), sharing the
  hermetic server + DB, so the world accumulates scene over scene.
- **Everything not on camera happens via RPC** — including *mid-scene*
  actions the camera watches land live (RSVPs, need claims, photo messages
  streaming into chat). Seed helpers live in `e2e/lib/seed/` (`chat.ts` and
  `planning.ts` were added for the reels).
- **The scene toolkit is `e2e/lib/walkthrough.ts`**: `openScene` (recording
  context + full phone/dark-mode emulation + auth injection + emulator-banner
  strip + **pre-seeded data consent** so the Accept-All modal never films),
  `settle` (paced dwell + storyboard annotation), `typeInto` (human-speed
  keystrokes with dropped-keystroke retry). Capture recipe constants are
  shared with `playwright.config.ts` via `e2e/lib/capture.ts`.
- **A reel can declare the date it is filmed on** (`e2e/lib/filmed-at.ts`).
  Call `setFilmedAt(new Date(...))` at spec scope, before any seeding, and
  everything that reads a clock is told the same thing: the **server** via the
  `X-Simulation-Timestamp` header it honours under `--dev-mode` (both the seed
  RPCs and the app's own calls from the page), and the **browser** via
  Playwright's clock, which does reach Dart's `DateTime.now()` through the
  wasm bundle. Set both or neither — a page that believes it is May, reading
  rows the server stamped in July, renders "2 months ago" on a message sent
  moments ago; the module exists so the two can't be set apart.

  Without it a reel can only film dates a few days from whenever it happens to
  render. The events reel needs a real Mother's Day, so it declares the
  Thursday before one: relative dates in prompts ("this Sunday") resolve
  against that, and the WHEN card still reads "3d from now". Note the tz: the
  capture emulation pins `America/Los_Angeles`, so pick the instant in that
  zone.

  **A reel can also let time pass.** Both readers resolve the declared moment
  lazily, so calling `setFilmedAt` again at the top of a later scene moves that
  scene's seeding and its page together — the `runclub` reel runs Monday →
  Wednesday this way. Rows written in earlier scenes keep their earlier
  timestamps, which is the point ("2 days ago" on the roster). Two caveats:
  the page clock is *fixed*, not running, so everything sent within one scene
  carries the same instant and same-timestamp ordering is arbitrary; and
  moving backwards would render as fresh rows dated in the future, so only
  ever move forward.
- **No live third parties.** `--mock-ai-provider` and
  `--mock-weather-provider` (both in `scripts/start_e2e_server.sh`) keep a
  render hermetic and repeatable. The weather one is also what lets a reel
  film a date the real forecast horizon doesn't cover — a fortnight either
  side of today. Deterministic weather comes from the day-of-year, so a
  filmed date always has a plausible forecast.
- **Clip heads are trimmed automatically.** A scene's first `settle()`
  writes `trim.json` (recording-start → first settled screen) into the
  test's output dir; the export cuts that blank-boot footage from the scene
  mp4s and the assembled reel. Single-scene reels using the built-in `page`
  fixture call `armCameraTrim(page)` at the top of the test body.
- **Scene titles are load-bearing**: `"@walkthrough NN <short-ascii-slug>"`.
  The `@walkthrough` tag routes the spec to the `walkthrough` project (the
  standard projects grepInvert it); the zero-padded index makes lexicographic
  output-directory order the reel order; short ASCII titles keep Playwright
  from middle-hashing the directory (= scene file) names.
- **Chat photo authoring is blocked on Flutter Web** (deliberate deferral in
  `web_unsupported.dart`), so photo-message beats are always filmed from the
  *receiving* side, with sends seeded via `sendExperienceChatMessage`.

**The walkthrough page is the primary deliverable.** Multi-actor reels read
better as a **scene-by-scene walkthrough page** than one hard-cut film: each
step gets a serif heading, a smaller actor line, explanatory copy, and its
clip in a phone frame. The copy is hand-editable markdown in the reel's
fixture pack (`e2e/fixtures/walkthroughs/<pack>/walkthrough.md`):

- Frontmatter: `reel:` (maps pack→reel), `slug:` (the site URL segment),
  `title:` (page headline, e.g. "Group Event Walkthrough: Mother's Day
  Brunch"), `blurb:` (the index-card line).
- `## NN · Heading` sections map to scene clips by the two-digit `NN` —
  which is **mapping-only and never rendered** (steps aren't numbered on the
  page); an `actor:` first line names who's on camera. Work the punchline
  into the body (bold), not the heading.
- Styling comes from the site's generated design tokens — the fixed
  `--light-*` set so pages always read light — because `lint:web:colors`
  bans raw colors under `website/content/`.

**The index teases each reel with a clip.** Every reel opens on the same empty
Home, so a row of first-scene stills is five identical thumbnails; the index
therefore teases with **scene 2** by default, where each story's own subject is
on screen. A pack whose scene 2 is still setup overrides it with `teaser:` in
its frontmatter (the kitchen pack uses `"03"` — its scene 2 is still the create
flow). The chosen scene is recorded in `meta.json` as `teaserScene`; the clip
FILE is resolved from what is on disk at deploy time, so renaming a scene can't
strand the index on a clip that no longer exists.

**Rendering deploys to the site.** `export_walkthrough.sh` invokes
`scripts/build_walkthrough_page.mjs`, which writes the standalone
artifact AND deploys to **`website/content/walkthroughs/<slug>/`**
(page as `index.html` + the scene clips + `meta.json`), then regenerates the
**`/walkthroughs/` index** from every deployed `meta.json`. Editing copy or
re-rendering and re-running the export is the whole refresh — commit the
`website/content/walkthroughs/` diff for review (the PR staging site serves
it). The generator has unit tests
(`e2e/scripts/build_walkthrough_page.test.mjs`, run by
`npm run test:scripts` in CI's Check Versions job).

**Showcase, not correctness gate.** `@walkthrough` specs are exempt from
`e2e/README.md`'s "state changes happen through the UI" rule — off-camera RPC
mid-scene is the whole point. The correctness versions of these journeys stay
in `e2e/tests/workflows/`. Keep a light server-side assertion per scene (an
RSVP landed, the event completed) so a broken journey fails the render
instead of producing a silently wrong video.

## Fixture packs (content + imagery)

`e2e/fixtures/walkthroughs/` holds committed, deterministic content:

- **Stock set** (top level): Unsplash-fetched hero/avatar/item images +
  `manifest.json` attribution; regenerate with
  `e2e/scripts/fetch_walkthrough_fixtures.mjs` (idempotent).
- **Real-content packs** (subdirectories, e.g. `brunch/`): extracted from one
  real prod event. Each pack contains the images (recompressed: avatars
  320px, photos 1280px long side), `content.json` (the scrubbed
  "screenplay": cast, needs, message script with cadence offsets), and
  `manifest.json` (provenance: source experience id, export date, per-file
  `gs://` origin, and the **consent statement**).

**Consent + privacy rules for real-content packs (non-negotiable):**

- A pack ships only with explicit consent from the people pictured, recorded
  in both `manifest.json` and `content.json` → `source.consent`.
- A mechanical scrubber enforces the floor — first names only, mention tokens
  rewritten, email/phone redaction, venue reduced to name+locality — and human
  review of `content.json` and every image before commit is the backstop. The
  scrubber is a floor, not a guarantee.
- Every real-content pack carries a `purgeIfRepoGoesPublic` flag. Consent is
  granted for a *particular* audience: consent to appear in material held in a
  private repository is not consent to appear in a public one. **If the
  repository's visibility changes, packs marked `true` are purged — from the
  working tree and from history — rather than re-interpreted.**
- Raw (pre-scrub) exports never enter git.

Extracting a pack from a real event needs read access to a production database
and media bucket, so the procedure is specific to a given deployment and is not
documented here. The fabricated packs need none of that: they are the default,
and the reels built from them (`kitchen`, `classroom`, `mower`, `runclub`) cover
every journey the pipeline exercises.

## Adding a new reel

1. Pick the journeys and write the storyboard as scene titles
   (`@walkthrough NN <slug>`); paraphrase real content from a fixture pack for
   authenticity. **Keep titles short and ASCII** (roughly ≤ 25 chars after the
   `NN`): the exporter orders scenes by Playwright's test-results directory
   names, and long titles get hash-truncated mid-string, which both breaks
   the lexicographic scene order and garbles the clip filenames. Em-dashes
   and other non-ASCII in titles end up in directory/file names — don't.
2. Create `e2e/tests/walkthroughs/<reel>.spec.ts` following
   `events.spec.ts`: serial mode, module-level world state, one scene
   per test via `openScene`, off-camera RPC seeding (add seed helpers to
   `e2e/lib/seed/` if a surface lacks one), a light server-side assertion
   per scene.
3. Locators: role/label first (`getByRole('button', { name })`) — labels come
   from `app_localizations_en.dart`. Primitive `semanticsIdentifier`s do NOT
   reach the DOM on Flutter Web
   (`docs/client/testing/semantics_identifiers.md`); if a widget is
   unreachable, add an l10n-sourced `semanticsLabel` or a route-level anchor
   in the app instead.
4. Render, read `frames/`, iterate pacing until it's share-worthy.
5. Add the reel to the catalog table above.

Iteration gotchas (each has bitten a real render):

- **Create-prompt wording steers the e2e classifier.** The deterministic
  classifier checks gear cues before request/event (`classifyE2EText`,
  `server/ai/provider_e2e.go`) — keep "borrow"/"lend"/"loan" out of a
  request-creation prompt or the create flow misroutes to gear.
- **Renaming scenes strands old clips on the site.** The export step
  auto-deploys to `website/content/walkthroughs/<slug>/`; after retitling
  scenes, `rm` the slug's old `scene-*.mp4` and re-run
  `build_walkthrough_page.mjs` so only the current five ship (stale clips are
  committed binaries — they bloat the repo silently).
- **Nested interactive widgets are unreachable on Flutter Web.** A `Toggle`
  or `Tappable` nested inside another `Tappable`'s subtree is swallowed by
  the row's merged semantics — no locator will find it, and taps land on the
  row. Hoist per-row actions out as siblings of the tappable area (see
  `docs/client/testing/semantics_identifiers.md` § nested interactive
  widgets).

## Re-render policy

Re-render affected reels after significant UI changes to the journeys they
cover (the reels are the cheapest full-journey visual review we have), and
before using any clip externally. The specs run only under
`--project=walkthrough` and never gate CI.
