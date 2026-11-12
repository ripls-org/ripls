# Issue #2703: Requests walkthrough #2 — a teacher's classroom supply drive (pivoted from campus yard day)

> **PIVOT (2026-07-14, Bryan):** the original "campus yard day" framing read
> too much like an *event* (a cleanup "day"). Re-themed to a **teacher
> collecting classroom supplies at the start of the school year** —
> unambiguously a *request*. Parents fulfill it **mostly by giving** supplies
> (giveaway offers) with **one lending a whiteboard**; one parent joins from the
> parents' **WhatsApp group** by phone. This surfaced — and Bryan approved
> fixing — a real **giveaway-logic gap**: marking a multi-need request fulfilled
> *cancelled* the confirmed helpers' still-open gear offers instead of
> delivering them, so donations evaporated. See **Giveaway fix (as-built)** and
> **Implementation status** below; the yard-day sections are kept as the design
> trail. Copy uses "request(s)", never "ask" as a noun (Bryan's standing rule).

## Giveaway fix (as-built)

**Problem:** the #2702 escalation was loan-first. A parent *giving* an item
creates a giveaway offer (`OfferTransfer(GIVEAWAY, recipient=requester)`) that
sits at `RECIPIENT_SELECTED`. On `REQUEST_FULFILLED`, the transfer service's
`cancelOpenOriginOffers` cancelled *every* still-open origin offer — right for
the loan morph (one loan fulfills, siblings stand down), wrong for a supply
drive where the offers *are* the fulfillment. Giveaways to a requester had no
completion path, so they were cancelled and the donated items never changed
hands.

**Fix (server):** on `REQUEST_FULFILLED`, `finalizeOriginOffersOnFulfillment`
(replaces the blanket cancel) reads the request's `confirmed_helper_ids` and, for
each still-open origin offer: a **confirmed** helper's **giveaway** →
**COMPLETED** (gear `GIVEN_AWAY`), a confirmed **loan** → **ACTIVE**; an
**unconfirmed** helper's offer → cancelled (prior behavior). The request then
**re-adopts** the delivered transfers' item-based impact
(`readoptImpactFromHandoffs` sets `ImpactAdoptedFromTransferId` + recomputes via
`transferAdoptedImpact`) so it never double-counts against them. Files:
`server/services/transfer/request_subscriber.go`,
`server/services/request/fulfill.go`. Integration test:
`server/integration_tests/request_to_giveaway_test.go` (giveaway-completes /
loan-activates / unconfirmed-cancels; existing request-to-loan tests still
pass). **Note:** a giveaway marks the gear `GIVEN_AWAY` — it does **not**
reassign the gear record's owner (existing system-wide model, unchanged).

**Follow-ups (both now FIXED, 2026-07-14):**
1. **Undo now reverses every delivered offer.** `UndoMarkRequestFulfilled` used
   to cancel only the single `fulfilled_by_transfer_id`, stranding the delivered
   loans ACTIVE after undoing a manual multi-need fulfillment. The
   `REQUEST_FULFILLED_UNDONE` handler now calls
   `cancelDeliveredOffersOnFulfillmentUndo` — every still-cancellable origin
   offer stands down (loans revert, pre-handoff offers cancel) while a completed
   giveaway stays put (permanent). Covered by the extended
   `request_to_giveaway_test.go` (undo cancels the whiteboard loan, the given
   books stay COMPLETED, the request reopens); the request-to-loan undo test
   still passes.
2. **Impact card framing is scope-aware.** `SharingImpactCard` took a hardcoded
   gear-loan line ("each time a friend borrows this instead of buying one");
   it now accepts a `framingText`, and `ItemMetricsScreen` passes a line worded
   for the item type — a request reads "Because the community pitched in instead
   of everyone buying their own, you all saved:". Widget test covers the custom
   framing; the classroom reel was re-rendered so its closing impact shows the
   corrected copy.

---

# (original plan) Issue #2703: Requests walkthrough #2: school campus yard day — multiple helpers, multiple things requested

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2703
**Filed:** 2026-07-11 | **Last activity:** 2026-07-11
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** `docs/walkthroughs.md` (anchor for this work), `docs/planning.md`, `docs/client/needs.md`, `docs/workflows/request.md` (skimmed), `docs/client/testing.md` + `docs/client/testing/semantics_identifiers.md` (e2e locators), `testing/architecture.md` (anchor), `users/priorities.md` + `users/personas.md` (product anchors); code read: `e2e/tests/walkthroughs/requests.spec.ts`, `e2e/lib/seed/planning.ts`, `e2e/lib/seed/requests.ts`, `server/ai/provider_e2e.go`, `proto/ripls/api/request_service.proto`, `e2e/scripts/run_walkthrough.sh` + `export_walkthrough.sh`, `e2e/scripts/fetch_walkthrough_fixtures.mjs`.

> **Branch decision (from Bryan, 2026-07-14):** this reel is built **on top of PR #2722** (`plan/2708-event-child-transfers`), not a fresh branch off main. That branch already carries the shared walkthrough infra #2708 touched — the `.gitattributes` LFS filter for `website/content/walkthroughs/**/*.mp4` and the `lfs: true` + `git lfs pull` steps in both website-deploy workflows — so the new reel's mp4s ride the same rails with zero extra infra.

## Problem Statement

We have four walkthrough reels (`onboarding`, `events`, `gear`, `requests`) rendered from the hermetic e2e stack (#2684). The `requests` reel (the "mower" pack, #2686 upgraded for #2702) tells a **single-need, single-helper** story: one person asks for one thing, one neighbor lends it, the request closes itself as a loan. It deliberately deferred the harder, more social shape.

This issue is Bryan's deferred variant: **one ask, many pieces, many hands** — a school campus yard/cleanup day. One requester posts a *needs list* (rakes, a wheelbarrow, work gloves, a truck for the green-waste run, plus extra pairs of hands). Multiple helpers each take a different piece; at least one links the actual gear they're bringing; an appless parent grabs the last slot via the phone-first loop; the day wraps with the requester marking it fulfilled and confirming a whole crew, with an impact story that spans the day rather than a single item.

Its **product value is double**:

1. **Showcase.** A cooperative, multi-actor "many hands" story is exactly the fabric-tier moment the product is built around (`users/priorities.md` → latent host; `users/growth-use-cases.md` → project days rank #1 for network bootstrapping). The mower reel can't carry this — its whole point is the frictionless single loan.
2. **The missing visual-regression pass on request-side Needs & Contributions.** The `events` reel exercises the *experience* twin of the planning layer (who's-bringing-what), but **no reel exercises request needs** (`AddRequestNeed` / `ClaimRequestNeed` / `AddRequestContribution`, `docs/planning.md`). This reel becomes the standing full-journey visual check on that surface — when it breaks or looks wrong, request needs regressed.

**No ambiguity in the issue** — it ships with a full six-beat storyboard and an explicit mechanics/sequencing note. The one genuine design fork the author left open is *how much of the multi-need posting and claiming happens on camera vs. seeded off-camera* (see Open Questions).

**Scope is narrow and additive.** The issue's own mechanics note is correct: this needs "only a new spec + fabricated fixture pack," a request-need twin for the currently experience-only `e2e/lib/seed/planning.ts`, and *maybe* an e2e-provider request-suggestion override. Research confirms **no proto, client, or server-service changes are required** — every request-needs RPC the reel drives already exists and is public:

| RPC | Shape (verified) |
|---|---|
| `AddRequestNeed` | requester posts a need (name, note, slots) |
| `ClaimRequestNeed` | `need_id`, `request_id`, optional `note`, `community_id`, **optional `gear_id`** (the gear-linked claim) |
| `AddRequestContribution` | freestanding offer, optional `gear_id` |
| `ListRequestNeedsAndContributions` | read-path for per-scene asserts |
| `UnclaimRequestNeed` / `EditRequestContribution` | not needed by the reel |

## Context from Comments

**No comments on the issue.** It is a single, unusually complete brief from the repo owner (Bryan), with a storyboard, a mechanics list, and a sequencing note. The absence of discussion is *not* a signal that the issue is under-specified — it's the opposite; the body reads as a spec. The design decisions that would normally surface in comments are captured inline (fabricated pack, no prod extraction; sequence after #2701; don't block on #2702).

Cross-referencing the sibling work (memory + `docs/issues/2686-requests-walkthrough.md`, `docs/issues/2702-request-to-loan.md`):

- **Both named sequencing dependencies are now merged.** #2701 (offer state visible on the request detail card) shipped in the combined PR #2707; #2702 (request→loan morph) shipped in PR #2705. The issue said "better after #2701" and "a fulfill-via-loan beat depends on #2702 but the reel doesn't have to wait" — both are moot now; nothing blocks a full-fidelity reel.
- **Bryan explicitly deferred this exact variant** when scoping #2686 ("deferred an in-app member-responder / multiple-respondents / fulfill-via-shared-gear variant to a future reel"). #2703 *is* that future reel.

## Potential Solutions

Two independent axes. **Axis 1 (reel shape)** has an obvious answer; the real design energy is **Axis 2 (how multi-need/multi-helper is driven on camera)**.

### Axis 1 — reel shape

#### Option A: New dedicated reel + new fabricated pack (issue's intended shape)
- **Approach:** New spec `e2e/tests/walkthroughs/yardday.spec.ts` (reel key `yardday`), new fabricated pack `e2e/fixtures/walkthroughs/yard-day/` with `walkthrough.md` copy + `reel: yardday` / `slug: yard-day` frontmatter, yard-work imagery fetched into the shared stock set via `fetch_walkthrough_fixtures.mjs`.
- **Pros:** Matches the pipeline's one-reel-per-spec model exactly (`run_walkthrough.sh <reel>` resolves `tests/walkthroughs/<reel>.spec.ts`; export maps pack→reel by `walkthrough.md` frontmatter). Self-contained; the mower story stays intact and re-renderable. Independent site page at `/walkthroughs/yard-day/`.
- **Cons:** A second requests-family reel to maintain. Reel-slug prefix rule (`export_walkthrough.sh`: slugs must not prefix one another) rules out names like `request`/`requests2`; use a distinct name (`yardday`).
- **Effort:** Medium (the bulk of the work regardless of option — spec + pack + copy + render/QC).

#### Option B: Extend `requests.spec.ts` with more scenes
- **Approach:** Bolt yard-day scenes onto the existing mower reel.
- **Pros:** One fewer file.
- **Cons:** Wrong model — a reel is one spec = one pack = one site page; you can't have two casts/communities/slugs in one reel cleanly. Bloats a 5-scene reel to ~11 and muddles two unrelated stories. The exporter concatenates *all* `@walkthrough` scenes in the file into one mp4.
- **Effort:** Medium, and produces a worse artifact.

#### Option C: Reuse the mower pack's cast/community
- **Approach:** Same neighbors, new scenario.
- **Cons:** The scenario is a *school community* with different people and yard-work items; reusing Cedar Court cast/hero fights the story. No real saving.
- **Effort:** Low but wrong.

### Axis 2 — how the "needs list" + "many hands" is driven

The reel's whole reason to exist is the **request-side Needs layer with multiple needs and multiple concurrent claimers**. How much runs through the UI on camera vs. seeded off-camera is the load-bearing choice (it trades authenticity against locator fragility on Flutter Web).

#### Option A: Everything on camera
- **Approach:** Requester posts all ~5 needs through the "Break it down" Propose sheet on camera; each helper's claim is filmed.
- **Pros:** Maximally authentic.
- **Cons:** A 5-need Propose marathon + N filmed claims is a locator/pacing minefield (nested-Tappable hoist gotchas, per-keystroke typing, suggestion-grid determinism). High flake, long render, and it *undersells* — a wall of typing isn't the punchline.
- **Effort:** Large.

#### Option B: Everything seeded, camera only watches
- **Approach:** Create the base request on camera; seed the whole breakdown + all claims off-camera via new `addRequestNeed`/`claimRequestNeed` twins; the camera only lingers on the assembled result.
- **Pros:** Rock-solid, fast, deterministic.
- **Cons:** Loses the two beats the reel is *for* — a human posting a needs list and a human claiming a piece. Feels like a screenshot tour.
- **Effort:** Small.

#### Option C: Hybrid — film the load-bearing beats, seed for density (recommended)
- **Approach:** Film exactly the moments that carry the story, seed the rest to create a busy, real-feeling world:
  - **On camera:** requester creates the ask (unified create) → posts **two** needs via the Propose sheet to *show* the breakdown mechanic (e.g. "Rakes", "Pickup truck"); **one** helper claims a piece **with real gear linked** (the wheelbarrow, gear pill); the appless parent grabs the **last** slot via phone-first; the requester **marks fulfilled + confirms the crew**; the closing impact.
  - **Off camera / mid-scene live:** seed the remaining ~3 needs, land 2–3 other helpers' claims *live while the requester watches* (the events-reel mid-scene RSVP pattern), and seed the coordination chatter.
- **Pros:** Every filmed beat is a real product moment; the world still looks populated ("many hands"); flake and render time stay bounded. Directly follows the `events` and `requests` reels' proven recipe (on-camera actor + off-camera live RPC).
- **Cons:** Requires the new seed twins (which we need anyway) and careful pacing so the live-landing claims read as arrivals, not glitches.
- **Effort:** Medium.

## Recommendation

**Axis 1 → Option A** (new `yardday` reel + fabricated `yard-day` pack) and **Axis 2 → Option C** (hybrid), built on **PR #2722's branch**.

Rationale: Option A is simply how the pipeline is wired — anything else fights the tooling. Option C is the same director's technique both existing multi-actor reels use, and it puts the camera on the two beats that don't exist in any other reel (posting a request *breakdown*, and a *gear-linked* request claim) while keeping the render deterministic. Building on #2722 means the LFS-tracked mp4 + staging-deploy path is already proven on this branch (#2708 curl-verified staging serves the real 4 MB mp4, not a pointer stub), so Phase 4 is a re-run, not new infra.

**Deliberately keep this reel distinct from the mower reel:** the mower reel *owns* the request→loan morph (#2702). This reel's gear-linked claim should be a **plain gear-pill link** ("here's the wheelbarrow I'm bringing"), not a loan escalation — supply meeting demand, not a transfer lifecycle. A lend-escalation flourish is available (both scopes support it now) but would blur the two reels; leave it as an optional, explicitly-noted variant rather than the default (see Q2).

What I'd want to confirm before locking the storyboard (all cheap, all in Phase 1): that the **Mark Fulfilled modal confirms *multiple* helpers** (Q3), and that the **Propose-sheet suggestion chips are deterministic** under the e2e provider (Q5). Both are verify-then-proceed, not blockers.

## Open Questions

- **Q1 — How many needs on camera vs. seeded?** Recommendation: **post 2 on camera** via the "Break it down" Propose sheet (enough to show the mechanic and the suggestion grid), **seed 3 more** off-camera (`addRequestNeed`) for a full list of ~5. Filming five sequential Propose adds is a flake magnet and reads as tedium on playback.
- **Q2 — The gear-linked claim: plain pill, or #2702 loan escalation?** Recommendation: **plain gear-pill link** (`ClaimRequestNeed` with `gear_id`), no loan morph. Keeps this reel's identity separate from the mower reel, which exists specifically for the loan escalation. Note the escalation as an available variant in the spec header but don't film it. (If Bryan wants the loan beat here too, it's a one-line swap to the #2702 path — but then the two requests reels overlap.)
- **Q3 — Does completion support multi-helper confirmation on camera? → RESOLVED (verified in code during audit): yes.** `MarkRequestFulfilledRequest.confirmed_helper_ids` is a **`repeated string`** (`proto/ripls/api/request_service.proto:225`), and the client `mark_fulfilled_modal.dart` toggles **multiple** helper tiles via `toggleHelper(user.id)` / `toggleHelper(prov.id)` (`app/lib/presentation/screens/request/mark_fulfilled_modal.dart:325,339`) — provisional (appless) helpers included, which the storyboard needs for the phone-first parent. The issue's "tap-to-confirm several tiles" beat is safe to film exactly as written. The #2686 original filmed a single confirm only because that request had one helper.
- **Q4 — Reel key + slug names.** Recommendation: reel key **`yardday`** (spec `yardday.spec.ts`, `run_walkthrough.sh yardday`), pack **`yard-day`**, site slug **`yard-day`**, page title "Request Walkthrough: A Campus Yard Day". `yardday` collides with no existing reel and violates no prefix rule (`onboarding`/`events`/`gear`/`requests`).
- **Q5 — Provider request-suggestion override for on-camera Propose chips? → RESOLVED (verified during implementation): no override needed.** The Propose/Picker suggestion grid reads `breakdownPieces`/`additionalAsks` off `RequestNeedsState`, populated at gen time from `GenerateRequestSuggestionsFunc`, which under the e2e provider returns deterministic, non-placeholder strings (`BreakdownPieces: ["Pick up","Return run"]`, `AdditionalAsks: ["Extra hands","Fuel topped up"]`) — no bare-mock leak. The reel types need names (Rakes, Pickup truck) rather than tapping chips, so chip *content* is cosmetic; the strings read acceptably for a yard day. Revisit only if Phase-3 frames show an off-key chip in frame.
- **Q6 — Fixture imagery + deterministic gear listing.** Recommendation: fetch yard-work item photos (wheelbarrow, rakes, work gloves, a pickup/truck, a hero of a group cleanup) into the **shared stock set** via `fetch_walkthrough_fixtures.mjs` (Unsplash, attributed in `manifest.json`) — fabricated pack rules, **no prod extraction**. For the gear-linked claim, **pre-seed the helper's wheelbarrow in their library off-camera** and have them *pick* it from the claim-sheet gear picker on camera, rather than a photo→listing capture. That avoids adding a second filename to `e2eGearDetectionForFilename` and a second on-camera capture flow (the mower reel already films photo→listing; picking-from-library is the untried, and simpler, path here).
- **Q7 — Appless parent: reuse the phone-first loop wholesale?** Recommendation: **yes** — reuse the exact `/go/<code>` SSR landing + phone-OTP + provisional-promote loop the mower reel's scene 02 uses (`createProvisionalUser` + `loginPhoneUser` recovery), just pointed at "the last open slot" instead of the only need. The correctness twin (`phone-request-offer-full-loop.spec.ts`) still covers it.
- **Q8 — Scene count: 6 or tighten to 5?** Recommendation: aim for **6** (the events reel runs 7), but treat the issue's scene 4 (pure thread coordination) as **foldable** into scene 3 or 5 if playback drags — coordination chatter can land as mid-scene live messages under a claim beat rather than its own scene. Decide during QC-frame review.

## Implementation Plan

Each phase is a shippable increment on the #2722 branch; commit after each (author = current user, `--no-verify`, no Claude authorship — per standing prefs).

### Phase 1: Foundations & verification
**Goal:** Prove the request-needs surface can be driven end-to-end off-camera, and resolve Q3/Q5 before storyboarding.
**Tasks:**
- [ ] Add request-need twins to `e2e/lib/seed/planning.ts`: `addRequestNeed`, `claimRequestNeed` (with optional `gearId`), `addRequestContribution` (mirroring the existing experience functions, over `RequestService`). Keep the file's doc-comment honest about the two scopes now living in it.
- [ ] Verify `ClaimRequestNeed` gear-linking and the read-path (`ListRequestNeedsAndContributions`) return the gear pill data the reel asserts on.
- [ ] **Q5:** Run the existing `requests` reel (or a throwaway probe) and confirm what the Propose suggestion grid renders under the e2e provider; decide if an override is needed.
- [ ] (Q3 already resolved in the audit — `MarkRequestFulfilled` + the modal support multi-helper confirm; no code check needed.)
- [ ] No unit test for the seed twins: the `e2e/lib/seed/` layer is test infrastructure exercised by the reel render itself (a broken twin fails the render loudly via the per-scene server assert) — matches how the existing experience twins and `requests.ts` are validated. Don't add a bespoke unit test.
**Validates:** The request-needs layer is drivable from seed; Q5 is answered; the storyboard can be finalized against real UI behavior.

### Phase 2: Fabricated fixture pack
**Goal:** A committed, deterministic yard-day pack.
**Tasks:**
- [ ] Add yard-work item + hero image fetches to `e2e/scripts/fetch_walkthrough_fixtures.mjs` (wheelbarrow, rakes, work gloves, truck, group-cleanup hero); run it (idempotent); confirm `manifest.json` attribution entries land.
- [ ] Create `e2e/fixtures/walkthroughs/yard-day/walkthrough.md` with frontmatter (`reel: yardday`, `slug: yard-day`, `title`, `blurb`) and per-scene copy (`## NN · Heading`, `actor:` line, bold punchline in the body). Reflect the **$0-money-saved-is-by-design** reality: because this reel's gear-linked claim is a **plain pill, not a #2702 loan** (Q2), fulfillment records no transfer value — impact is carried by **quality time + emissions prevented** (the many-hands day is the story). Write the closing-scene copy to the QT/CO₂ metrics, not a dollar figure. (#2702's real-gear-value fix only applies to the loan-escalation path, which this reel deliberately avoids.)
- [ ] Fabricated cast names/avatars (avatars from the shared stock set); a school-community name (e.g. "Bexley Elementary Families").
**Validates:** Imagery renders with attribution; copy is hand-editable and reads well; pack maps to the reel.

### Phase 3: The reel spec
**Goal:** `yardday.spec.ts` renders a share-worthy, deterministic reel.
**Tasks:**
- [ ] Write `e2e/tests/walkthroughs/yardday.spec.ts` modeled on `requests.spec.ts`: serial mode, module-level world state, `openScene` per scene, off-camera + mid-scene-live RPC seeding, a **light server-side assert per scene** (a need posted, a claim landed with its gear, the request fulfilled with N confirmed helpers).
- [ ] Scenes (hybrid per Axis-2 Option C; final count per Q8): 01 create ask + post 2 needs + seed the rest + share to school + copy link · 02 helpers claim pieces (one filmed **gear-linked** claim; others land live off-camera) · 03 appless parent joins via phone-first, grabs the **last** slot · (04 coordination — fold if it drags) · 05 mark fulfilled + confirm the crew + aggregate impact · 06 closing record.
- [ ] Locators: role/label first (`getByRole('button',{name})` from `app_localizations_en.dart`); apply the **nested-interactive-widget hoist** rule and `semanticsLabel` fallbacks (`docs/client/testing/semantics_identifiers.md`); keep create-prompt wording free of gear cues ("borrow/lend/loan") so the e2e classifier routes to *request*.
- [ ] **Owner-view gotcha (from #2686):** on the requester's own request the "View helpers" affordance opens the **needs composer** ("What would help?"), not a helpers list. Incoming claims/offers surface on the content view's **participation / who's-helping card** (the #2701 offer-visibility surface). Storyboard the requester *watching claims land* against that card, and note that its text may be locator-unreachable (merged Tappable semantics) — assert via the server read-path + verify visually on `frames/`, not `getByText`.
- [ ] `npm run build:e2e` (rebuild the embedded bundle+server after any client-visible change — none expected, but the binary must post-date the bundle) → `e2e/scripts/run_walkthrough.sh yardday` → **review `frames/`** → iterate pacing (`WALKTHROUGH_PAUSE_MS`) until share-worthy.
**Validates:** The full multi-need/multi-helper journey renders green; the request-needs surface gets its first visual-regression pass; the frames prove it looks great.

### Phase 4: Deploy, catalog, and doc
**Goal:** The page ships to the site and the docs stay true.
**Tasks:**
- [ ] `export_walkthrough.sh yardday` auto-deploys to `website/content/walkthroughs/yard-day/` (page `index.html` + scene mp4s + `meta.json`) and regenerates the `/walkthroughs/` index; commit the `website/content/walkthroughs/` diff. mp4s are LFS via the #2722 `.gitattributes` filter — confirm they're tracked (`git lfs ls-files`), not committed as blobs.
- [ ] Add the `yardday` reel to the **catalog table** in `docs/walkthroughs.md`; if `planning.ts` twins or a provider override changed behavior worth noting, update the relevant doc prose (keep `freshness:` honest).
- [ ] Verify the **staging preview** serves the real reel mp4 (the #2722 `git lfs pull` deploy step must materialize it — re-confirm content-length/`ftyp` bytes, not a 132-byte pointer).
- [ ] Push to `plan/2708-event-child-transfers`; the change rides PR #2722 (or, if Bryan prefers it separate at review time, split into its own PR — but the default per his instruction is on #2722).
**Validates:** `/walkthroughs/yard-day/` is live on the staging preview with real footage; the catalog and docs describe reality.

## Audit dimensions engaged

Scope is **testing / infra**, not production code. Engaged dimensions for the Phase-4 `/ripls-audit`: **testing** (e2e reel is a showcase, not a correctness gate — correctness twins stay in `tests/workflows/`; keep the per-scene server assert so a broken journey fails the render), **code_quality** (the `planning.ts` twins must mirror the experience functions' shape/naming and not duplicate needlessly; honest doc-comment now that both scopes live there), **accessibility** (e2e locators — role/label-first, nested-Tappable hoist, no reliance on primitive `semanticsIdentifier` reaching DOM on Flutter Web), and **tech_debt** (retitling scenes strands old clips on the site — `rm` stale `scene-*.mp4` before re-export; don't let committed binaries accumulate). **Not engaged:** security, api_design, observability, error_handling (no proto/RPC/server-service/client-production changes).

## Implementation status (as-built, 2026-07-14)

**DONE — all 4 phases, reel renders green (5/5), on PR #2722's branch.** Commits:
Phase 1 `3613a7605` (seed twins), Phase 2 `944d55a45` (fixture pack), Phase 3
`a18a07348` (spec), Phase 4 `b6415e935` (deploy + catalog). The reel deploys to
`website/content/walkthroughs/yard-day/`; the 5 scene mp4s are LFS objects.

**Final shape:** 5 scenes (folded the standalone coordination beat per Q8) —
01 create + five-piece breakdown + share/copy · 02 four pieces claimed on
Priya's open view (Marcus links real gear) · 03 appless Rob joins by phone,
grabs the last slot · 04 whole-crew Mark Fulfilled · 05 closing impact. Cast:
Priya (requester) + Marcus/Dana/Sam/Nadia (a helper per piece) + Rob
(phone-first). Needs are one-slot each so the day ends fully covered.

**Two app behaviors discovered during the render (both now reflected in the
spec + `docs/walkthroughs.md`):**
1. **The claim gear pill is contributor-only.** `NeedsClaimRow` renders the
   `GearPill` only for *the current user's own* contribution, so the requester
   never sees a helper's pill — sharing the gear into her community (which I do,
   for realism) does not change that. Consequence: "supply meeting demand" reads
   on Priya's screen as the covered Wheelbarrow need carrying **Marcus's name**,
   not a pill; the gear link is real server-side and asserted. This revises the
   plan's Q2/Q6 assumption that the pill would be visible to the requester.
2. **The Mark Fulfilled modal pre-confirms every offerer/contributor.**
   `fulfill_modal_view_model.initialize()` seeds all helpers as confirmed, so the
   authentic gesture is *confirm as-is* — tapping a tile *removes* a helper. The
   first render tapped all four tiles and credited zero; scene 04 now just
   confirms. This revises the plan's Q3 "tap-to-confirm several tiles" beat.

**Other fixes during the render:** scene 05's title was shortened ("what it
added up to") after a long title hash-truncated its Playwright output dir and
scrambled the export order (the `docs/walkthroughs.md` short-title gotcha); the
scripted coordination chat message was dropped (the seeded helper wasn't a
member of the request *conversation's* community, and each claim already posts
its own system message to the thread).

**Known limitation / possible follow-ups:** the request **View Impact** detail
reuses gear-loan copy ("each time a friend borrows this instead of buying one")
which reads slightly off for a help request — pre-existing app string, out of
scope here; worth a separate issue. Surfacing a helper's gear pill to the
requester (so supply-meeting-demand is visible, not just named) would also be a
product follow-up, not a reel fix.

## Related Issues
- #2684 — parent walkthrough pipeline; this is the next reel on it.
- #2686 — the first requests reel (mower, single-need/single-helper); this is its deliberately-deferred "many hands" sibling. Shares the `provider_e2e` request-generation + phone-first-loop machinery.
- #2701 — offer visibility on the request detail card (**merged**, PR #2707); a stated prerequisite so multi-helper beats don't undersell — now satisfied.
- #2702 — request→loan morph (**merged**, PR #2705); the mower reel owns this; the yard-day gear-linked claim deliberately stays a plain pill to avoid overlap.
- #2708 — event child transfers (**PR #2722**, the branch this work builds on); source of the shared LFS + website-deploy plumbing the new reel's mp4s reuse.

## Audit notes (`/ripls-audit`, 2026-07-14)

**Scope:** testing / infra — a new e2e walkthrough reel plus a test-infra seed twin and a fabricated fixture pack. **No proto, client, or server-service production code changes.** Engaged dimensions: **testing**, **code_quality**, **accessibility** (e2e locators), **tech_debt**. Not engaged: security, api_design, observability, error_handling, performance, dependency (nothing in those surfaces changes).

**Changes applied during the audit:**

1. **Q3 grounded, not deferred (testing / correctness).** Verified in code that multi-helper confirmation is fully supported: `MarkRequestFulfilledRequest.confirmed_helper_ids` is `repeated string` (`request_service.proto:225`) and `mark_fulfilled_modal.dart` toggles multiple helper tiles including provisional users (`:325,339`). Rewrote Q3 from "verify in Phase 1" to a resolved yes with citations, and dropped the now-redundant Phase-1 verify task. The storyboard's multi-tile confirm beat is safe as written.
2. **Phase-1 seed-test scope corrected (testing).** The testing rubric warns against trivial tests; the `e2e/lib/seed/` layer is test infrastructure exercised by the render itself (the per-scene server assert fails a broken twin loudly), matching how the existing experience twins and `requests.ts` are validated. Replaced "add a smoke test if warranted" with an explicit "no bespoke unit test" so the plan doesn't manufacture a low-value test.
3. **Owner-view locator gotcha added (accessibility).** Surfaced the #2686 finding that "View helpers" on the requester's *own* request opens the needs composer, not a helpers list; incoming claims surface on the #2701 participation card whose text is locator-unreachable (merged Tappable semantics). Phase 3 now storyboards the requester watching claims land against that card and asserts via the server read-path + `frames/`, not `getByText`.
4. **$0-money-saved copy causally grounded (correctness).** Made explicit that request fulfillment shows no dollar figure *because this reel's gear-linked claim is a plain pill, not a #2702 loan* — impact is QT + emissions. Prevents a reader from assuming #2702's real-gear-value fix applies here (it only touches the loan-escalation path this reel deliberately avoids).

**Confirmed sound (no change needed):**
- **Reel-slug prefix rule (tech_debt / tooling).** `export_walkthrough.sh` requires reel slugs not to prefix one another; `yardday` has no prefix relationship with `onboarding`/`events`/`gear`/`requests`. Reel key (`yardday`) ≠ page slug (`yard-day`) mirrors the established `requests`→`mower` split.
- **Showcase-not-gate discipline (testing).** Plan keeps a light per-scene server assert and points correctness twins to `tests/workflows/` — matches `docs/walkthroughs.md`'s standing rule.
- **`planning.ts` twin shape (code_quality).** Request twins mirror the experience functions (parity, not avoidable duplication); the file already houses the planning seed layer, and the plan calls for an honest doc-comment once both scopes live there.
- **Fabricated pack / no prod extraction (privacy).** No real-content pack, so `docs/walkthroughs.md`'s consent/purge rules for prod-extracted packs don't apply; Unsplash imagery is attributed in `manifest.json`.

**Docs consulted:** `docs/walkthroughs.md` (anchor), `docs/planning.md`, `docs/client/needs.md`, `docs/workflows/request.md`, `docs/client/testing.md`, `docs/client/testing/semantics_identifiers.md`, `testing/architecture.md` (anchor), `docs/reviews/testing_review_prompt.md`; product anchors `users/priorities.md` + `users/personas.md`. Code verified: `proto/ripls/api/request_service.proto`, `app/lib/presentation/screens/request/mark_fulfilled_modal.dart`, `server/ai/provider_e2e.go`, `e2e/lib/seed/planning.ts` + `requests.ts`, `e2e/scripts/{run,export}_walkthrough.sh` + `fetch_walkthrough_fixtures.mjs`.
