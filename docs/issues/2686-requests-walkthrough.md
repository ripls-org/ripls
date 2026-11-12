# Issue #2686: Requests walkthrough (user journey video pipeline)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2686
**Filed:** 2026-07-09 | **Last activity:** 2026-07-10
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** `docs/walkthroughs.md`, `docs/workflows/request.md`,
`docs/workflows/phone_first_request_offer.md`, `docs/testing/architecture.md`,
`docs/users/priorities.md`, `docs/client/manage_menus.md` (fulfillment UI),
`docs/client/testing/semantics_identifiers.md` (locator rules), plus the two
prior reel plans as precedent: `docs/issues/2684-marketing-sizzle-videos.md`
and `docs/issues/2687-shared-food-walkthrough.md` (pending PR #2696).

> **Note on issue number:** this work was kicked off as "/ripls-issue 2690",
> but #2690 is the RecordAttendance cross-community bug. The described
> storyline (request a lawn mower → a neighbor responds) is issue **#2686**;
> Bryan confirmed the retarget. Direction confirmed 2026-07-11:
> follow the **issue's original storyboard** (SSR landing + phone-first offer
> flow); the "member shares their own mower and fulfills by selecting that
> item" variant is deferred to a future reel with multiple respondents and
> shared gear. End with a **short closing scene** of the asker's
> fulfilled-request view. Imagery comes from **Unsplash** via
> `fetch_walkthrough_fixtures.mjs` — no prod extraction this time.

## Problem Statement

Third reel on the #2684 user-journey walkthrough pipeline (after `events` and
the pending `gear`/kitchen reel): a scene-based Playwright reel plus published
walkthrough page showing the **request loop** end to end — a neighbor posts an
ask (a lawn mower), shares it out, a non-app guest opens the shared link, views
the SSR request landing, registers via phone OTP, and their offer lands; the
asker thanks them in the thread and marks the request fulfilled, closing on the
impact view. Per `docs/walkthroughs.md` this should need **only a new spec +
fixture content** — the toolkit (`openScene`/`settle`/`typeInto`), capture
recipe, export/publish pipeline, and most seed helpers already exist.

The reel doubles as the request journey's cheapest full-visual regression
check, complementing (not replacing) the correctness twins
`e2e/tests/workflows/phone-request-offer-full-loop.spec.ts` and
`request-authoring-share.spec.ts`.

## Context from Comments

The issue has **no comments**. Direction came from Bryan at plan time
(2026-07-11):

- Follow the issue's storyboard sketch: SSR link + phone-first offer, not an
  in-app member responder. A second requests reel later will cover multiple
  respondents and fulfilling by selecting shared gear.
- Short closing scene (asker's fulfilled-request view), not a full loan loop.
- Fetch imagery from Unsplash via the fixture-fetch script; do not extract
  prod content (the issue's `walkthrough-export` option is explicitly out).

## Codebase Findings (drives the plan)

- **Almost everything is reusable.** Off-camera seeding: `registerUser`,
  `createCommunity`, `registerUserViaInvite`, `setUserAvatar`,
  `createProvisionalUser`, `seedRequest`, `mintRequestShareLink`,
  `uploadMedia`. On-camera: `createRequestViaWebUI`
  (`e2e/lib/ui/create-item.ts:67`, save label `/^save request$/i`), the share
  sheet locators from `request-authoring-share.spec.ts`, and
  `completePhoneRegister` + `emulatorVerificationCode`
  (`e2e/lib/ui/phone-register.ts`) for the guest's OTP loop.
- **Classifier gotcha (load-bearing):** the e2e mock classifier
  (`server/ai/provider_e2e.go:84`) checks **gear cues before request cues** —
  `borrow`/`lend`/`loan` route to gear. The on-camera prompt must use a
  request cue and avoid gear words: e.g. *"Looking for a lawn mower this
  weekend — ours died mid-mow"* (`looking for` → request). Same rule is
  documented at `create-item.ts:74`.
- **No offer-acceptance mechanism exists.** `RequestService` has only
  `OfferToFulfill`/`WithdrawOffer`; `REQUEST_OFFER_SELECTED` is a dormant enum
  nothing emits. Fulfillment is the asker's **Mark Fulfilled** modal
  (`mark_fulfilled_modal.dart`): tap-to-confirm helper tiles, inline impact
  bar, primary button `Mark Fulfilled` → `MarkRequestFulfilled` (state →
  `FULFILLED`, response carries the impact estimate).
- **The hermetic e2e server has no stock-imagery provider** (Pexels/Unsplash
  keys absent; `server/media/fake_provider.go` is test-only), so a request
  created on camera gets **no hero image**. The SSR landing
  (`server/services/web/request_page.go:140`) renders the request media as its
  hero (logo fallback otherwise) — the lawn-mower fixture must be attached to
  the request **off-camera after scene 01** for scenes 02+ to look right.
- **SSR page requirements:** it needs a minted share link (`/go/{shortCode}`)
  and renders an error for terminal requests — fulfillment must come after the
  offer scene (it does). CTA is a link with accessible name
  `"Offer to help {requesterName}"`; `run_walkthrough.sh` sets
  `E2E_INVITE_LINK_HOSTNAME=ripls.app` for display realism while specs
  navigate by local short code.
- **Missing seed helpers (small):** `sendRequestChatMessage` (a ~15-line twin
  of `sendExperienceChatMessage` swapping in
  `ChatService.GetConversationForRequest`) and a `setRequestMedia` wrapper
  (`uploadMedia` + `UpdateRequest.media_id`) for the hero attach. No
  `offerToFulfill`/`markRequestFulfilled` wrappers are needed — both happen on
  camera.
- **Asker-side surfaces:** offerers appear via the WHO'S IN card →
  `RequestHelpersPanel` (semantics "View helpers"); the manage menu opens via
  the "Edit request" pencil; **View Impact** row appears once fulfilled, and
  the fulfilled request's first tab shows the inline `ContentImpactRow`
  (Money saved · Quality time · CO₂ avoided).
- **Overlap with pending PR #2696 (kitchen reel):** both touch
  `e2e/scripts/fetch_walkthrough_fixtures.mjs`, the Unsplash attribution
  `e2e/fixtures/walkthroughs/manifest.json`, `docs/walkthroughs.md`'s reel
  catalog, and — critically — `website/content/walkthroughs/index.html`,
  which the export **regenerates from every deployed `meta.json`**. Rendering
  this reel from plain `main` would emit an index that drops the kitchen page.

## Potential Solutions

### Option A: Film the phone-first loop live (issue storyboard) — CONFIRMED
- **Approach:** Five serial scenes. The guest's offer happens **on camera**
  through the real SSR landing + Firebase-Auth-Emulator OTP flow (the
  `phone-request-offer-full-loop` journey, paced for film); everything else a
  camera isn't watching is RPC-seeded.
- **Pros:** Shows the product's differentiating loop (no app, no account →
  helping neighbor in under a minute); maximal reuse of proven helpers; the
  reel doubles as a visual regression check on WEB-3.
- **Cons:** OTP screens add footage that needs pacing care; auth-emulator
  determinism is already solved but adds a moving part per render.
- **Effort:** Small-medium (the pipeline absorbs most of it).

### Option B: In-app member responder (original session description)
- **Approach:** A registered member shares their own mower and responds to
  the request by selecting that gear item.
- **Pros:** Would showcase gear-linking on requests.
- **Cons:** Bryan explicitly deferred this to a later reel (multiple
  respondents + shared gear); "fulfill by selecting an item" as a
  first-class mechanism doesn't exist today (no offer-selection RPC), so the
  story would have to fake it through chat + manual fulfillment.
- **Effort:** Medium, and premature.

### Option C: Seed the offer off-camera, film only the asker
- **Approach:** Two-three asker-only scenes; the neighbor never appears.
- **Pros:** Smallest possible spec.
- **Cons:** Loses the reel's entire point — the guest's phone-first moment is
  the story. Also requires the `offerToFulfill` seed wrapper Option A avoids.
- **Effort:** Small, but wrong deliverable.

## Recommendation

**Option A**, as confirmed by Bryan. Storyboard (titles short per the
middle-hash gotcha; scene NN is load-bearing for export order):

| Scene | Actor (camera) | Beats | Off-camera RPC |
|---|---|---|---|
| `@walkthrough 01 the ask` | June (asker) | Unified create → Text tab → type the mower prompt → Generate → preview → **Save request** → share sheet: invite community, Copy Link | Pre-scene: register June + neighbors, community, avatars, location; post-scene: resolve requestId **and the ad-hoc community id** via `ListMyRequests` + `GetRequest` polling (mock-AI titles aren't deterministic, and `SubmitRequestResponse.item_community_id` went to the UI client — the `request-authoring-share.spec.ts` precedent), then `setRequestMedia` (lawn-mower hero), `mintRequestShareLink`, `createProvisionalUser` (guest's phone — June "texts the link"). Light assert: request exists in `ACTIVE` with media attached |
| `@walkthrough 02 a neighbor steps up` | Marcus (guest, logged out) | Open `/go/{code}` SSR landing (mower hero) → **Offer to help** → phone → OTP → name → **Finish** → lands on the request, offer auto-fires | Post-scene assert: `GetRequest` → `OFFERS_RECEIVED` (poll ≤30s, per the correctness twin) |
| `@walkthrough 03 thanks in the thread` | June | Open the request (Home → Yours), WHO'S IN shows Marcus (View helpers), open conversation, `typeInto` a thank-you; Marcus's reply streams in live | Mid-scene: `sendRequestChatMessage` (new helper) as Marcus. Light assert: conversation message count grew to include both messages |
| `@walkthrough 04 mark it fulfilled` | June | Manage pencil ("Edit request") → **Mark Fulfilled** → tap Marcus's tile to confirm → impact bar → **Mark Fulfilled** | Post-scene assert: `GetRequest` → `FULFILLED` |
| `@walkthrough 05 what it added up to` | June | Short closing: fulfilled request view — `ContentImpactRow` pills, optional **View Impact** screen | Light assert: state still `FULFILLED` (scene is read-only; the assert just fails the render if the view 404s) |

Reel slug `requests` (`e2e/tests/walkthroughs/requests.spec.ts`; distinct,
non-prefix of existing slugs). Fixture pack
`e2e/fixtures/walkthroughs/mower/` with `walkthrough.md`
(`reel: requests`, `slug: mower`) — fabricated pack, so **no `content.json`
or provenance manifest needed** (the pipeline only consumes `walkthrough.md`;
kitchen sets the precedent). The lawn-mower image goes in the **top-level
stock set** with Unsplash attribution, next to the existing
`request-ladder.jpg`.

## Open Questions

- **Branch base: stack on `sharing_walkthrough` (PR #2696) or wait for its
  merge?** — Recommendation: **stack** (rebase `request_walkthrough` onto
  `sharing_walkthrough`, open the PR with that base), mirroring how #2696
  stacked on #2697. This keeps `fetch_walkthrough_fixtures.mjs`, the stock
  manifest, the walkthroughs index, and the docs catalog conflict-free, and
  keeps the regenerated `/walkthroughs/` index from dropping the kitchen
  page. GitHub auto-retargets to `main` when #2696 merges.
- **Is the mock provider's request copy good enough on film?** — The e2e
  provider echoes the prompt as title/description (`e2eTitle`/
  `e2eDescription`). For an ask, an echoed sentence reads fairly naturally.
  Recommendation: render scene 01 first and judge the frames; only if it
  reads poorly, add keyed request copy to `provider_e2e.go` (precedent:
  #2696's filename-keyed food content). Don't pre-build it.
- **Pack slug `mower` vs something else?** — Recommendation: `mower`
  (content-flavored, matching `brunch`/`kitchen`; the page URL becomes
  `/walkthroughs/mower/`).
- **Pre-seed the guest as a provisional user?** — Recommendation: yes. It
  matches the correctness twin, exercises promote-on-verify on film-adjacent
  infrastructure, and costs one RPC. The walkthrough copy can say June texted
  the link to Marcus.
- **Set a needed-by date on the request?** — Recommendation: skip for this
  reel ("simple"); the Home-calendar tie-in is a nice-to-have that adds a
  surface to keep visually correct.

## Implementation Plan

### Phase 1: Fixtures + seed helpers
**Goal:** Everything the spec consumes exists and is committed.
**Tasks:**
- [x] Rebase `request_walkthrough` onto `sharing_walkthrough` (stacked on PR #2696).
- [x] Add `ensureSingle('request-lawn-mower.jpg', 'lawn mower grass yard', 'landscape', 1280)` to `fetch_walkthrough_fixtures.mjs`; run it with the Unsplash key fetched per the script's own header (`fetch_secret.sh` from Secret Manager — never echo or commit the key; specs never need it at test time since images are committed); review the fetched photo (delete + re-run for a better draw if needed); commit the JPEG + regenerated attribution manifest.
- [x] Add `sendRequestChatMessage` to `e2e/lib/seed/chat.ts` (twin of the experience variant via `GetConversationForRequest`).
- [x] Add `setRequestMedia` to `e2e/lib/seed/requests.ts` (`uploadMedia` + `UpdateRequest.media_id`).
- [x] Create `e2e/fixtures/walkthroughs/mower/walkthrough.md` with frontmatter (`reel: requests`, `slug: mower`, title, blurb) and draft section stubs `## 01`–`## 05` with actor lines.
**Validates:** Unsplash fetch path works without prod data; the two helper gaps close cleanly.

### Phase 2: The reel spec
**Goal:** `e2e/scripts/run_walkthrough.sh requests` renders five green scenes.
**Tasks:**
- [x] Write `e2e/tests/walkthroughs/requests.spec.ts` per the storyboard: `// @walkthrough` header, `test.describe.configure({mode:'serial'})`, `test.setTimeout(300_000)` per scene, module-level world state, `openScene`/`settle`/`typeInto`, `asActor` for June, actor-less scene 02.
- [x] Scene 01: `createRequestViaWebUI` beats inlined at walkthrough pacing with a **gear-cue-free prompt** ("Looking for a lawn mower this weekend — ours died mid-mow"); community-invite + Copy Link via the share sheet (reuse `request-authoring-share.spec.ts` locators; remember share-sheet sub-flows dismiss the whole sheet).
- [x] Scene 02: `/go/{shortCode}` → `getByRole('link', {name: /offer to help/i})` → `completePhoneRegister`; assert `OFFERS_RECEIVED`.
- [x] Scenes 03–05: helpers panel, thread thanks + live seeded reply, Mark Fulfilled modal (confirm helper tile), closing impact view; assert `FULFILLED` after scene 04.
- [x] Iterate on `frames/` until pacing and copy are share-worthy; fix the e2e provider's request copy only if the frames demand it — and if `provider_e2e.go` is touched, extend `provider_e2e_test.go` alongside (the #2696 precedent).
**Validates:** The full request journey works on the web bundle end to end — any regression in WEB-3, share sheet, chat, or the fulfillment modal fails the render.

### Phase 3: Page, publish, docs
**Goal:** Published walkthrough page + updated catalog; PR open.
**Tasks:**
- [x] Finalize `walkthrough.md` copy (serif headings, actor lines, bolded punchlines in body — never in headings).
- [x] Full render → review every QC frame → deploy lands in `website/content/walkthroughs/mower/` + regenerated index; verify the index still lists brunch **and** kitchen.
- [x] Add the reel to `docs/walkthroughs.md`'s catalog table (it also lists this reel as "planned next" — update that line).
- [x] Reap any orphaned `ripls-walkthrough-*` containers; commit; open the PR (base `sharing_walkthrough` until #2696 merges).
**Validates:** Deliverable is the page, not just clips; catalog and index stay coherent across the stacked PRs.

## Risks / carried-over gotchas

- Flutter Web: `.fill()` is ignored (use `pressSequentially`/`typeInto`);
  clicks right after boot are swallowed pre-hydration (waitFor a rendered
  element first); bottom-edge widget semantics are culled until scrolled
  (waitFor a mid-screen anchor, then `mouse.wheel`).
- Share-sheet confirm lands on the item view — `goto` home between beats that
  assume it.
- Long renders: run in foreground Bash (10-min timeout); kills leave orphan
  containers to reap.
- Scene titles must stay short-ASCII or Playwright middle-hashes the output
  dirs and breaks export ordering.
- The SSR landing 404s/errors on terminal requests — never reorder
  fulfillment before scene 02.

## As-built notes (2026-07-11, PR #2700)

Implemented as planned, stacked on `sharing_walkthrough` per Bryan's
"implement and stack". Deviations and discoveries:

- **Scene 03's "View helpers" beat was cut**: on the owner's read shell that
  affordance opens the *needs composer* ("What would help?"), not a helpers
  list. Theo's raised hand shows on the shell itself (scroll beat instead).
- **The tidy also pins June's home location** (`UpdateRequest.location_id`)
  — without it the ask's WHERE card reads "TBD" on camera.
- **"$0 Saved" is by design for requests** (no purchase prevented; quality
  time + CO₂ carry the impact story) — the pack copy was reworded away from
  money framing rather than fighting the estimator.
- **Found + fixed #2699 inline** (minor, per Bryan's mid-flight directive):
  the Mark Fulfilled quick-add offered Theo's already-claimed provisional
  placeholder beside his real account. `SearchProvisionalUsersByName` now
  excludes claimed rows (service test added); verified gone in the re-render.
- **Provider override was needed up front**: the bare mock generates
  "Mock Request" for text-mode requests, so `GenerateRequestFunc` (echo
  title/description) went in during Phase 1 rather than waiting for frames.
- **One flaky render**: a ~6-minute renderer-tab freeze between Generate and
  Save (all queued RPCs flushed in one burst at test timeout). Environment
  load, not spec logic — a plain re-run passed.
- New seed helper beyond plan: `loginPhoneUser` (PhoneLogin via the auth
  emulator) to recover the guest's session after his on-camera registration,
  for the off-camera avatar + chat reply.

**Post-review follow-ups (Bryan, 2026-07-11):** #2701 (offer state invisible
on the request card — **fixed in PR #2704**, which this reel now stacks on:
OFFERING TO HELP rows on the participation card + a server-truth-confirmed
"You offered to help" toast on the guest handoff; both verified on film in
the re-render), #2702 (fulfill a request by lending gear — request→loan
morph), #2703 (requests reel #2: multi-helper school yard day).

**Hard-won iteration gotchas:**

- **`npm run build:web:e2e` alone does NOT update the served bundle** — the
  script copies the bundle into `server/services/web/app_assets/`, which is
  *embedded in the Go binary*; without a following `go build -o tmp/server
  ./server` the hermetic server keeps serving the previous app. (Cost us
  three phantom debugging rounds against a stale bundle — docs/walkthroughs.md
  lists the two commands in the right order for a reason.)
- **The participation card's text is locator-unreachable on Flutter Web**:
  the card is one Tappable whose merged semantics ("View helpers") swallow
  all child text — `getByText('OFFERING TO HELP')` can never match. Verify
  such beats via frames + server asserts.
- `debugPrint` is compiled out of the profile/wasm e2e bundle; instrument
  with `package:logging` (`Logger(...).info`) when console evidence is
  needed, and read it from the Playwright trace
  (`unzip trace.zip; grep '"text"' 0-trace.trace`).

## Audit notes (2026-07-11)

Audited via `/ripls-audit`. Engaged dimensions: testing, code_quality,
security (secret handling + imagery licensing). Fixes applied in-place:

- Added a **light server-side assertion to every scene** (walkthroughs.md
  requires one per scene, not just at the two state transitions).
- Spelled out how scene 01 resolves the **ad-hoc community id** off-camera
  (`ListMyRequests` + `GetRequest` polling) — `SubmitRequestResponse.
  item_community_id` is only returned to the creating UI client, and mock-AI
  titles are non-deterministic, so title-keyed lookup is out.
- Secret hygiene on the Unsplash fetch: key comes from Secret Manager per the
  script header, is never echoed or committed, and is not needed at test time.
- If `provider_e2e.go` gains keyed request copy, `provider_e2e_test.go` must
  be extended alongside.

Non-findings worth recording: inlining `createRequestViaWebUI` beats at
walkthrough pacing is acceptable duplication (tests prefer DAMP; the events
reel sets the precedent); the fabricated pack carries no consent/provenance
obligations (those rules govern real-content packs only); no proto, app, or
server-runtime surface changes, so api_design/accessibility/observability
dimensions are not engaged — if Phase 2 does touch `provider_e2e.go`, that
stays inside the e2e-only mock provider.

Docs consulted: `docs/llms.txt`, `docs/walkthroughs.md`,
`docs/workflows/request.md`, `docs/workflows/phone_first_request_offer.md`,
`docs/testing/architecture.md`, `docs/users/priorities.md`,
`docs/client/manage_menus.md`, `docs/client/testing/semantics_identifiers.md`,
`docs/reviews/testing_review_prompt.md`,
`docs/reviews/security_review_prompt.md`,
`docs/reviews/code_quality_review_prompt.md`; plus
`e2e/tests/workflows/phone-request-offer-full-loop.spec.ts`,
`request-authoring-share.spec.ts`, `e2e/lib/seed/*`, `e2e/lib/ui/*`,
`e2e/scripts/fetch_walkthrough_fixtures.mjs`, `server/ai/provider_e2e.go`,
`server/services/web/request_page.go`, and PR #2696's file list.

## Related Issues

- #2684 — parent pipeline (merged, PR #2689); this reel follows its "Adding a new reel" protocol.
- #2687 / PR #2696 — kitchen reel (pending); file-level overlap drives the stacked-branch recommendation.
- #2492 — phone-first pivot epic; scene 02 films its WEB-3 loop.
- #2690 — RecordAttendance cross-community bug (the number this session was invoked with; unrelated to this plan).
- Future: a second requests reel with multiple respondents + fulfill-via-shared-gear (Bryan, 2026-07-11 — file when picked up).
