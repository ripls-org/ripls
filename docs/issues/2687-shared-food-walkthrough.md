# Issue #2687: Gear-sharing walkthrough (user journey video pipeline)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2687
**Filed:** 2026-07-09 | **Last activity:** 2026-07-10
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** `docs/walkthroughs.md`, `docs/workflows/giveaway.md`, `docs/client/create.md`, `docs/workflows/item_sharing.md` (via map), `docs/users/priorities.md` (anchor), plus code: `e2e/tests/walkthroughs/events.spec.ts`, `e2e/lib/walkthrough.ts`, `server/ai/provider_e2e.go`, `server/ai/streaming_provider.go`, `server/services/unified_create/stream_gen.go`, `app/lib/presentation/viewmodels/unified_create_view_model.dart`

## Problem Statement

Build the third reel on the #2684 walkthrough pipeline. The issue as filed
sketches a tool-lending storyboard (drill / record player, loan + return);
the direction has since been updated (Bryan, via `/ripls-issue`): the reel is
a **giveaway story — someone shares extra food from their kitchen before
leaving town**. The situation is **fabricated** (people, items, chat), unlike
the brunch pack which was extracted from a real consented prod event. The
items are spaghetti, a bread loaf, and broccoli; imagery comes from **freely
licensed Unsplash photos** fetched via the existing stock-fixture generator
(Bryan's 2026-07-10 follow-up — the reference photos originally supplied in
`~/Desktop/shared_food/` had unclear licensing and are not used).

Requested beats:

1. Add each food item **from a photo**, sharing to several existing
   communities ("Alameda Neighbors", "Fourth Grade Moms", "Vegans of Travis
   Heights").
2. People claim the items: first a single requester; then two people
   requesting one item, a chat about who gets it (someone already has
   spaghetti sauce made and just needs the pasta), and it goes to that
   person.
3. The offerer reviews all three claimed items.

Liberties to exercise more of the flow are explicitly welcome.

The flagship beat — **photo → generated listing** — is currently broken
*in the environment the walkthrough renders against*: "you can't add an
item from an image on the web app." The UI investigation below reproduces
this and pins the layer: it is **not** a web-client bug but a server-side
stream-adapter bug that only manifests with **unary-wrapped AI providers**
— in practice, the e2e/dev mock provider. Bryan confirms image-create works
fine from iOS/Android, which is consistent: mobile runs against dev/prod
where Gemini/Anthropic stream natively (description arrives mid-stream);
web against those same backends would work identically. The plan still
fixes the root cause (it hard-blocks the walkthrough and would bite any
future unary/fallback provider), but there is no production regression.

This reel showcases the atomic offer/claim fabric interaction
(`docs/users/priorities.md`: "small offers and asks in the spaces between"),
complementing the events reel (gathering) and the planned requests reel
(ask).

## Context from Comments

The issue has no comments. All direction beyond the issue body comes from
the `/ripls-issue` invocation summarized above, which supersedes the issue
body's storyboard (food giveaway instead of tool loan; fabricate instead of
extract). The issue body's render command (`run_marketing_video.sh gear`)
predates the #2684 rename — the current entry point is
`e2e/scripts/run_walkthrough.sh <reel>`.

Follow-up direction (Bryan, 2026-07-10, after the investigation): (1)
image-create works fine from the iOS/Android clients — reconciled in the
Problem Statement above (provider-not-platform is the variable); (2) replace
the supplied food photos with freely licensed Unsplash ones.

## UI Investigation

**Investigated:** 2026-07-10 on `sharing_walkthrough`@e469d67f8 | **Environment:** hermetic e2e stack (`ripls-ui-investigate`)
**Verdict:** server-logic (primary) — the unary→streaming AI adapters never emit a `description` field event, and the client's terminal-payload reducer intentionally ignores the final payload's description, so with any unary provider the image-mode preview can never satisfy the Save gate. Secondary: e2e-provider content gap (image mode classifies as Event; placeholder copy). The web client itself is **not** at fault.

### Reproduction

One registered user; Flutter Web bundle at HEAD. Home → Create → Image tab
→ "Open gallery" → pick `e2e/fixtures/walkthroughs/gear-drill.jpg`. The
upload and stream succeed, the preview renders with the photo as hero — but:

- The stream classifies **Event** ("Mock Experience - Unknown region"), not
  Item, because the e2e classifier only reads prompt text, which is empty in
  image mode.
- The **description stays empty** (placeholder "Describe the event…"), so
  Save Event is disabled (gate: description ≥ 10 chars).
- Flipping the type selector to **Item** re-streams (`force_type=gear`) and
  yields title "Mock Gear Item" + details "Test Brand · Mock Model" — but
  again **no description**, so the generic "Save" button stays
  `aria-disabled` indefinitely. The description field *is* editable
  (`unified_preview_card.dart:141` wires `vm.editDescription`), so a user
  who realizes the blank field is the blocker can hand-type a description
  and save — but that defeats image mode's entire promise (photo →
  ready-to-save listing), and nothing tells the user why Save is disabled.
  In practice this reads as "you can't add an item from an image."

### Hypotheses tested

| # | Hypothesis | Probe | Observation | Verdict |
|---|---|---|---|---|
| H1 | Web gallery button / file chooser wiring is dead | probe-01 | "Open gallery" visible, `filechooser` event fires, file accepted | refuted |
| H2 | Blob-backed XFile upload fails on web | probe-01 | `AddMedia` succeeded; photo became the preview hero; zero console/RPC errors | refuted |
| H3 | Server image path errors | probe-01 + server.log | Both streams completed cleanly (`stream completed … input_mode=image`, `item_detected=true title="Mock Gear Item"`) | refuted |
| H4 | e2e provider classifies image input wrong / content unsaveable | probe-01 | Classified `event` with empty text; after manual flip, gear content lands **without description** → Save permanently disabled | confirmed |
| H5 | Not reproducible on HEAD (report is env-specific) | probe-01 | Reproduced on HEAD; root cause affects any unary provider, not just the mock | refuted |

### Evidence

- Screenshot `after-pick-10s`: Event preview, title "Mock Experience -
  Unknown region", description placeholder, Save Event greyed.
- Screenshot `after-flip-to-item`: Item preview, title "Mock Gear Item",
  Lend/Give toggle, "Test Brand · Mock Model" details row, description
  placeholder; Playwright click log shows the Save button held
  `aria-disabled="true"` through a 30 s retry loop.
- Server log: `classified … detected_type=event classifier_duration_ms=0`
  then, after the flip, `stream completed …
  detected_type=DETECTED_CONTENT_TYPE_GEAR force_type_used=true
  input_mode=image` — both successful; the defect is what's *in* the stream,
  not an error.
- Code (the two halves of the dead end):
  - **Server:** `emitGearDetectionFromUnary`
    (`server/ai/streaming_provider.go:407`) emits only `StreamFieldTitle`;
    the siblings `emitExperienceFromUnary`, `emitRequestFromUnary`,
    `emitGearFromUnary` likewise never emit `StreamFieldDescription`, even
    though every unary result carries a description into the terminal
    payload. Text mode masks this because the per-type services emit the
    *prompt* as an early description (`server/services/gear/gen_ai_streaming.go:260`);
    image mode has no prompt, so nothing ever fills the field. This is the
    same bug class as the Date/Time fix already commented at
    `streaming_provider.go:214-218` ("the client takes no time from the
    terminal payload") — description was missed.
  - **Client:** the `UnifiedCreateEventFinal` reducer
    (`app/lib/presentation/viewmodels/unified_create_view_model.dart:550-566`)
    takes only `itemDetails` + `type` from the terminal payload — never
    title/description — so the description in `DetectedGearItem.description`
    (`proto/ripls/api/gear_service.proto:348`) is dropped on the floor.
- Blast radius: any **unary-wrapped** provider — the e2e/dev mock always,
  and any production fallback path without native streaming. Gemini and
  Anthropic providers stream natively (`provider_gemini_streaming.go:298`
  emits title/description mid-stream), which is why prod image-create
  usually works. The walkthrough renders against the mock, so the reel is
  hard-blocked until this is fixed.

### Fix direction

1. **Server (root cause):** emit `StreamFieldDescription` from all four
   `emit*FromUnary` helpers in `server/ai/streaming_provider.go` (unit-test
   each). This alone makes image mode saveable everywhere.
2. **Client (hardening, same PR):** in the final-event reducer, fall back to
   the terminal payload's title/description when the field is still unset
   and not user-edited — the exact precedent `itemDetails` already follows.
3. **e2e provider (walkthrough content):** classify image-mode input as gear
   and key deterministic food content off the uploaded media's filename (see
   Phase 2).

No distilled e2e spec: the repro is fully catchable by a Go unit test on the
unary emitters plus a ViewModel test on the final-reducer fallback — browser
cost isn't warranted (`e2e/README.md` § Testing philosophy).

## Potential Solutions

Two real forks; the rest of the work (fixture pack, spec, copy) has one
reasonable shape established by the events reel.

### Fork 1 — the image-mode product fix

#### Option A: Server-only adapter fix
- **Approach:** Add `emitFieldIfSet(fields, StreamFieldDescription, result.Description)` to the four unary emitters.
- **Pros:** Single root-cause fix; benefits every provider and both text mode (real description instead of prompt echo) and image mode; trivially unit-testable in Go.
- **Cons:** Client remains fragile to any future provider that only delivers a terminal payload.
- **Effort:** Small.

#### Option B: Server fix + client terminal-payload fallback (recommended)
- **Approach:** Option A, plus the final reducer applies terminal title/description when unset and not user-edited (mirroring `itemDetails`).
- **Pros:** Root cause fixed *and* the client honors data the wire already carries; two cheap, independently testable changes.
- **Cons:** Marginally more code; the fallback is redundant while all providers behave.
- **Effort:** Small.

#### Option C: Client-only fallback
- **Approach:** Only the reducer change.
- **Pros:** Smallest diff.
- **Cons:** Leaves the server stream semantically wrong (mid-stream preview stays empty until the final lands — worse UX on slow providers); violates fix-root-cause.
- **Effort:** Small.

### Fork 2 — deterministic food content from the e2e provider

#### Option A: Filename-keyed content (recommended)
- **Approach:** Plumb the media record's `filename` (`models.Media.filename`) into `ai.DetectionImage` (new field, populated in `unified_create.resolveMediaForClassifier` and the gear service's own resolve path). `provider_e2e.go` classifies image input as **gear** whenever `in.Image != nil` and returns a small filename→content table (spaghetti/bread/broccoli titles + food-appropriate descriptions), falling back to the current defaults for unknown files.
- **Pros:** Deterministic; content lives beside the other e2e copy; `Filename` on `DetectionImage` is also useful logging context; no fixture-byte coupling.
- **Cons:** Touches the `DetectionImage` struct (real providers ignore the field); content table in server code rather than the fixture pack.
- **Effort:** Small.

#### Option B: Image-hash-keyed content
- **Approach:** e2e provider fetches `DetectionImage.ImageURL` and matches SHA-256 against known fixture hashes.
- **Pros:** No interface change.
- **Cons:** Hashes break on any recompression of pack images; provider does HTTP fetches in tests; hardcoded fixture hashes in server code are worse coupling than a filename.
- **Effort:** Small-medium, brittle.

#### Option C: Film text-mode creates instead
- **Approach:** Sidestep image mode; type "Giving away spaghetti…" prompts.
- **Pros:** Works today.
- **Cons:** Abandons the requested flagship beat (photo → listing is the product's core magic per `docs/users/priorities.md`); leaves the image dead end unfixed.
- **Effort:** Small, but fails the ask.

## Recommendation

**Fork 1 Option B + Fork 2 Option A.** The adapter fix is the true root
cause (same class as the already-fixed Date/Time terminal-payload bug) and
unblocks image-create in every mock-backed environment; the client fallback
is cheap hardening with an existing in-file precedent. Filename keying is
the only deterministic content mechanism that survives image recompression
and keeps the e2e provider self-contained. Then build the reel exactly on
the `events.spec.ts` pattern: serial scenes, RPC world-building off camera,
one light server assertion per scene.

## Storyboard (proposed)

Fabricated cast, Austin world: **Maya** (offerer, leaving town for two
weeks), communities **Alameda Neighbors** (her street), **Fourth Grade
Moms** (school), **Vegans of Travis Heights** (interest circle) — all
seeded off camera with members: **Priya** (neighbor), **Dana** (fourth-grade
mom, has a pot of sauce made), **Marcus** (neighbor), **Jules** (vegan
circle). Scene titles follow `@walkthrough NN <short-slug>` (short — long
titles middle-hash the output dir and break export ordering).

| NN | Scene | On camera | Beats |
|---|---|---|---|
| 01 | `photo to listing` | Maya | Create → Image tab → gallery pick (spaghetti photo) → listing streams in (title/description/details) → **Give** toggle → Save → share sheet: invite **Alameda Neighbors + Fourth Grade Moms** (multi-select). The flagship beat. |
| 02 | `clear the fridge` | Maya | Bread and broccoli via the same flow at a brisker pace; broccoli additionally shared to **Vegans of Travis Heights**. Shows repetition is cheap. |
| 03 | `first claim` | Priya | Opens the bread from her feed, "I want this" (single requester), sends a chat line ("Perfect timing — soup night"). |
| 04 | `two want the pasta` | Maya | The spaghetti conversation: Marcus's interest + message land live (RPC mid-scene), then Dana's — "I've got a pot of sauce already made, just need the pasta 🍝". Group-interest model on display. |
| 05 | `it goes to dana` | Maya | Maya replies, taps Select recipient → Dana. System message lands; Marcus's request stays visible (transparent, no hard feelings). |
| 06 | `everything claimed` | Maya | Home → Yours / the three items: all claimed. Maya completes each giveaway (spaghetti → Dana, bread → Priya, broccoli → Jules — Jules's claim seeded off camera in scene 4/5 gap). Show whatever the giveaway completion flow surfaces (the experiences-style live impact preview may not exist for transfers — verify at render; `CompleteTransfer` does return an `ImpactEstimate`). |
| 07 | `a fed street` | Dana | Closing: Dana's view of the completed item — "Gone to a new home", hers — then the item's thread holding the whole story. *(Filming this scene originally exposed #2695 — post-completion the gear view 403'd for non-owners, regressed to the loan look for the owner, and its conversation stopped opening. This PR now stacks on the #2695 fix branch, and the scene doubles as the fix's living regression guard.)* |

Server-side assertion per scene (transfer states, recipient ids,
`GEAR_STATE_GIVEN_AWAY` after 06) so a broken journey fails the render.

Notes from the brunch render (memory + `docs/walkthroughs.md`): unique phone
fixtures per spec; consent pre-seed is built into `openScene`; typeInto
retry for dropped keystrokes; claims by RPC need the actor to be a
community member first; photos film from the receiving side only.

## Open Questions

- **Licensing/provenance of the three food photos.** — **RESOLVED
  (2026-07-10):** Bryan confirmed the supplied photos should be replaced
  with freely licensed Unsplash equivalents. Extend
  `fetch_walkthrough_fixtures.mjs` with three food entries
  (`food-spaghetti.jpg` / `food-bread.jpg` / `food-broccoli.jpg`); the
  fetcher records photographer attribution in the existing top-level stock
  `manifest.json`, same as the gear/request item heroes. No purge marker
  needed — this is standard stock-set content.
- **Reel/pack naming.** — Recommendation: reel `gear` (matches the issue and
  the planned catalog row), fixture pack + site slug `kitchen`; page title
  along the lines of "Sharing Walkthrough: Emptying the Fridge Before a
  Trip".
- **Include a phone-first (no-app) claim beat?** The brunch reel's scene 02
  is its most differentiating moment, and `phone-gear-interest-full-loop`
  proves the item flow works. — Recommendation: yes if scene count allows —
  make Priya's scene 03 a phone-first web claim from a text link instead of
  an in-app claim; decide during render QC (it adds ~40 s of reel time).
- **Fabricated-content marker.** Real packs carry consent statements;
  this pack has no real people. — Recommendation: the pack's `manifest.json`
  states `source: fabricated` explicitly so the consent tooling/reviewers
  don't chase a nonexistent prod event; keep the scrubber untouched. With
  Unsplash imagery (above), no `purgeIfRepoGoesPublic` marker is needed.

## Implementation Plan

### Phase 1: Image-mode product fix
**Goal:** Photo → saveable listing works with unary providers (mock included) on all platforms.
**Tasks:**
- [x] Emit `StreamFieldDescription` in `emitGearDetectionFromUnary`, `emitExperienceFromUnary`, `emitRequestFromUnary`, `emitGearFromUnary` (`server/ai/streaming_provider.go`); add or extend unit tests covering each emitter's field set.
- [x] Lock the contract at the service boundary: assert in `server/services/unified_create/integration_test.go` (mock provider) that an image-mode stream delivers a non-empty description before the final event.
- [x] Client final-reducer fallback: apply terminal-payload title/description when unset and not user-edited (`unified_create_view_model.dart`); ViewModel test.
- [x] Re-run the probe flow (or a quick manual check against the e2e stack): pick image → Item preview → Save enabled → gear created. *(Verified by the rendered reel itself: scene 01 creates and saves from a photo on camera.)*
**Validates:** The investigation's root cause and that no other gap hides behind it.

### Phase 2: e2e provider food content + fixture pack
**Goal:** Deterministic, on-camera-worthy food listings from the three photos.
**Tasks:**
- [x] Add `Filename` to `ai.DetectionImage`; populate in `unified_create.resolveMediaForClassifier` and `gear.StreamGenGearFromImage`'s own resolve path. Keep the struct out of logs as today (`stream_gen.go`'s existing log-hygiene note: it carries presigned URLs; the filename is user-supplied and shouldn't be logged raw either).
- [x] `provider_e2e.go`: image-present classification → gear; filename→content table for the three foods (titles, warm descriptions, no brand/model noise — food, not tools); unit tests.
- [x] Fetch the three food images into the top-level stock set via `fetch_walkthrough_fixtures.mjs` (new `food-spaghetti.jpg` / `food-bread.jpg` / `food-broccoli.jpg` entries; Unsplash attribution lands in the stock `manifest.json`; key from `scripts/fetch_secret.sh ripls-dev unsplash-access-key`). The deterministic filenames double as the e2e provider's content-table keys.
- [x] Build `e2e/fixtures/walkthroughs/kitchen/`: fabricated `content.json` screenplay (cast, per-item community routing, chat script), `manifest.json` (`source: fabricated`), `walkthrough.md` page copy (frontmatter `reel: gear`, `slug: kitchen`). Images stay in the stock set — the pack holds only content.
**Validates:** Scene 01's on-camera content reads real before any spec work.

### Phase 3: Seed helpers + the reel spec
**Goal:** `gear.spec.ts` renders all scenes green.
**Tasks:**
- [x] New seed helpers: `sendGearChatMessage` (gear conversation via `Gear.conversation_id`) and `expressGearInterest` / `selectGearRecipient` / `completeGearTransfer` wrappers (`e2e/lib/seed/transfers.ts`) — claims land mid-scene like the brunch RSVPs.
- [x] Write `e2e/tests/walkthroughs/gear.spec.ts` per the storyboard: serial mode, module world state, `openScene` per scene, one server-side assertion per scene (interest counts, recipient = Dana, all three `COMPLETED` + `GIVEN_AWAY`).
- [x] Decide the phone-first claim beat (open question) while iterating. *(Decided: in-app claim — the iteration budget went to the #2695 discoveries; the phone-first flow is already the onboarding reel's showcase.)*
**Validates:** The full giveaway journey (multi-community share → group interest → chat → selection → completion) works end-to-end on the web bundle.

### Phase 4: Render, page, docs
**Goal:** Share-worthy deliverables deployed.
**Tasks:**
- [x] `e2e/scripts/run_walkthrough.sh gear`; read `frames/` after every render; iterate pacing.
- [x] Review the generated walkthrough page + `/walkthroughs/` index; commit the `website/content/walkthroughs/kitchen/` diff.
- [x] Update `docs/walkthroughs.md` catalog table (+ context/freshness stamp); close the loop on the issue.
**Validates:** The reel meets the "watch the journey end-to-end and it looks great" bar that is the pipeline's second purpose.

## Audit Notes

Audited 2026-07-10 (`/ripls-audit`) against: `docs/server/llm.md`,
`docs/server/conventions.md` + `docs/client/conventions.md` (anchors, via
prior loads), `docs/walkthroughs.md`, `docs/workflows/giveaway.md`,
`docs/client/create.md`, `docs/testing/architecture.md` (map summary), and
the "Areas to Review" sections of the testing / code-quality / security
review prompts. Changes applied:

- **Corrected an overclaim** in the UI Investigation: the preview
  description field is editable (`unified_preview_card.dart:141`), so a user
  *can* hand-type a description to unblock Save — reframed the failure as a
  silent degradation of image mode's promise, not a hard dead end.
- **Testing rubric:** added a service-boundary assertion
  (`unified_create/integration_test.go`) to Phase 1 so the description
  contract is locked above the adapter layer; softened "extend existing
  tests" to "add or extend."
- **Log hygiene:** noted that the new `DetectionImage.Filename` must follow
  the struct's existing keep-out-of-logs rule.
- **Storyboard accuracy:** scene 06's "impact preview" beat flagged as
  verify-at-render (the experiences-style completion preview may not exist
  for transfers).
- **Provenance:** tied the `purgeIfRepoGoesPublic` decision to the
  image-licensing open question.

No proto/API changes are proposed (nothing for `proto_conventions.md` to
gate); no accessibility-impacting UI changes (the reel consumes existing
screens); no new dependencies.

## Related Issues
- #2684 — parent pipeline (walkthrough toolkit, brunch + onboarding reels; PR #2689)
- #2686 — requests reel (sibling, same pattern; Phase 1-2 fixes benefit it too)
- #2492 — phone-first epic (the optional no-app claim beat; `phone-gear-interest-full-loop` is the correctness twin)
- #2690 — RecordAttendance cross-community bug found during the brunch render (events-only; no bearing here, listed for pipeline history)
- #1939 — classifier sees actual image content (the server plumbing this reel exercises)
- #2157 — item-creation media upload web-enablement (the part that already works)
