# Issue #2702: Fulfill a request by lending your gear: helper adds the item, loan starts, request morphs into the loan

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2702
**Filed:** 2026-07-11 | **Last activity:** 2026-07-11
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** `docs/llms.txt` (routing); anchors: `proto_conventions.md`, `server/architecture.md`, `users/priorities.md`; leaf docs: `workflows/request.md`, `workflows/loan.md`, `workflows/giveaway.md`, `workflows/delta.md`, `planning.md`, `client/needs.md`, `ad_hoc_communities.md`, `single_player_mode.md`, `design/unified-create.md`. Engaged review dimensions: api_design, architecture, error_handling, security, testing, performance.

## Problem Statement

Offering help on a request today is only a conversation join: `OfferToFulfill` creates a
`RequestOffer` row and adds the helper to the request chat. Fulfillment
(`MarkRequestFulfilled`) records bare `confirmed_helper_ids` and computes impact from
request defaults. If Theo actually lends June his mower, nothing connects the request to any
gear or transfer — no loan record, no return reminder, no handoff flow, and the impact is
a flat default rather than the mower's item-based estimate (#2275: "backdoor loan" shows
$0 saved).

Meanwhile the loan machinery (five-state transfer, handoff/return, return reminders,
perpetual item conversation, item-based impact) fully exists — but is unreachable from a
request. The demand side (the request) and the supply side (the loan) never meet.

The feature: a helper satisfies a borrow-style request with their actual thing — picking it
from their library or creating it inline — and the request morphs into a real loan (or
giveaway). The lend-vs-give distinction must not add friction for either the requester or the
giver. The same mechanism should extend to experiences ("bring my wheelbarrow to the yard
day") as a follow-up phase, via the shared Needs & Contributions layer.

### Codebase reality (verified 2026-07-11)

- **No request↔transfer link exists anywhere.** `models.Transfer` has no
  `request_id`/`experience_id`/origin field; the request service never touches transfers.
- **Gear-linking on claims exists but is display-only.** `PlanningContribution.gear_id`
  (shared by request and experience scopes) is validated only for ownership
  (`server/planning/gear_link.go`); no transfer, no gear-state change, no impact effect.
- **The claim gear picker already supports inline creation.**
  `NeedsClaimGearPicker._openCapture()` launches `UnifiedCreateModal`, diffs the library,
  and auto-links the new gear (deliberately without opening the share sheet).
- **Claiming already implies offering.** `ClaimRequestNeed` auto-creates a `RequestOffer`
  when the claimer isn't the requester (`ensureRequestOffer`, `server/services/request/needs.go`).
- **Targeted transfer creation exists, but past-only.** `CreateTransfer`
  (`server/services/transfer/create.go`) takes a pre-chosen recipient (registered or
  provisional) and bypasses the interest handshake — but hard-requires
  `completed_at_unix_sec` in the past. The gear-booking path (`ClaimGearDays` /
  `GearOwnerReserveDock`) is the one live owner-targets-a-person precedent.
- **A reserved seam is waiting.** `COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED` is defined
  in both enums with undo-registry, notification copy ("[user] chose your offer"), digest,
  and client-invalidation scaffolding — and is never emitted.
- **Item-based impact exists on the transfer side.** `BuildTransferImpactMetrics(gear, …)`
  vs. the request-default `BuildRequestImpactMetrics(valueUSD, …)`.
- **Unified create carries Lend/Give.** `TransferIntent` (default lend) →
  `SaveGearRequest.availability` (#2687); gear is born into its per-item ad-hoc community.
- **The #2700 requests reel has no gear at all.** The mower is a hero image; the offer is
  the chat path. PR #2700 (branch `request_walkthrough`) is open, not yet merged.

## Context from Comments

The issue has no GitHub comments. In its place, a live design interview with Bryan
(2026-07-11) settled the major forks — recorded in **Design Decisions** below, which
carries maintainer-decision weight for this plan.

## Design Decisions (from the design interview)

1. **Link model — Transfer origin + claim back-ref.** `models.Transfer` gains an origin
   reference (oneof: `origin_request_id` / `origin_experience_id`) making any transfer a
   *child transfer* of the entity it sprang from. Gear-linked contributions that escalated
   into a transfer carry a `transfer_id` back-ref. Multi-respondent = many child transfers
   per parent. Generalizes to experiences without new concepts.
2. **Lifecycle — fulfill at handoff.** A gear-backed offer creates the transfer in
   `RECIPIENT_SELECTED` with the requester as recipient (both sides have already signaled:
   they asked, you offered). The request auto-fulfills when the item changes hands — loan
   → `ACTIVE` or giveaway → `COMPLETED` — with the fulfillment pointing at the transfer.
   Cancelling pre-handoff just withdraws the offer (existing last-offer reversion applies).
   Manual `MarkRequestFulfilled` remains as an override, pre-linked.
3. **Gear visibility — auto-share into the request's community.** The (new or picked)
   gear is shared with the chosen lend/give availability into the community through which
   the helper saw the request, announced on the confirmation with easy undo/unshare.
   Matches the inventory-as-trust-anchor thesis; zero extra taps.
4. **Scope — requests first, plus the reel.** Ship request→loan/giveaway, the item-based
   impact fix (#2275), and update the #2700 reel so the mower is added as gear and lent.
   The data model supports experiences from day one; event child transfers are a
   follow-up issue feeding walkthrough #2703.
5. **Offer surface — claim-first, single CTA, no zero-state.** Bryan's steer: don't add a
   second competing CTA. Instead **every request is born with at least one need** —
   AI-extracted from the creation prompt ("lawn mower"), falling back to a title-derived
   need — so the needs list *is* the offer surface. The request view's one primary
   "Offer to help" CTA opens the needs/volunteer sheet: claim the seeded need (gear link
   prominent for physical things, inline create included) or "Offer something else"
   (freestanding contribution) for non-thing help. Both paths auto-create the
   `RequestOffer` + conversation join as today. The chat-first `RequestHelpModal` offer
   path retires; the thread stays reachable from the shell. The web SSR CTA keeps
   auto-firing `OfferToFulfill` and lands the guest on the request view with the claim
   sheet as the visible next step.
6. **Requester accept — optional.** The requester *can* tap "this one works" on an offered item —
   finally emitting the dormant `REQUEST_OFFER_SELECTED` event (notification copy already
   exists) and signaling other helpers — but it isn't required; handoff still fulfills
   without it.
7. **Siblings — auto-cancel politely.** When the request reaches `FULFILLED`, other
   non-terminal child transfers on the same request auto-cancel with a friendly
   "this ask was covered" notification (mirrors the `completeGiveaway` sibling-cancel
   precedent). Their gear never left `AVAILABLE`. *(Refined by decision 9: cancellation
   scopes per-need on multi-need requests.)*

### Round 2 (open-questions interview, 2026-07-11)

8. **Accept model — marker on the contribution.** Multiple transfers can support one
   request ("I need a table and chairs"), so acceptance is per-need, not per-request.
   `PlanningContribution` — already the need↔helper↔gear↔transfer junction — gains a
   requester-set `accepted_at_unix_sec`. A need renders its chosen offer(s) from its accepted
   contributions (multi-slot needs can accept several, up to `slots`); freestanding
   contributions are acceptable too. The earlier `Request.accepted_transfer_id` idea is
   dropped.
9. **Auto-fulfill — single-need auto, multi-need manual** (refines decision 2). First
   handoff fulfills a request with at most one need (the seeded mower case; legacy
   zero-need requests included). On multi-need requests, a handoff marks its need covered
   and cancels sibling offers **on that need only**; the requester closes the request via Mark
   Fulfilled (pre-populated checklist).
10. **Undo of an auto-fulfillment unwinds the loan too** *(overrides the audit
    recommendation)*. Undoing a transfer-linked fulfillment restores the request AND
    cancels the linked transfer (bus-driven; the existing cancel machinery restores gear
    state). Rationale: the undo case is a mis-tapped handoff — the loan start was part of
    the same mistake. Already-cancelled siblings stay cancelled (terminal transfer states
    don't revive; re-offering is cheap).
11. **Cancelling an origin-linked transfer unwinds the claim and offer** *(overrides the
    audit recommendation)*. Pre-handoff cancel releases the claimed slot, clears the
    contribution back-ref and any accepted marker, and withdraws the helper's
    `RequestOffer` unless they still have another live contribution on the request — the need
    visibly reopens. Bus-driven so services stay independent; the existing last-offer
    reversion then governs request state.
12. **Legacy zero-need requests: no backfill.** The ≥1-need invariant applies to new
    requests only; old requests keep working via freestanding contributions.
13. **Impact: adopt the transfer's item-based estimate; QT stays request-side.** The
    request contributes no separate money/emissions to community metrics (the transfer
    counted once). Fixes #2275 without double-counting.
14. **Need seeding via GenRequest structured extraction** — the primary asked-for thing
    becomes a seed need (name + slots=1) in the same LLM call, with a title-derived
    fallback; the invariant is enforced server-side in `SubmitRequest`.
15. **All offer entry points route to the claim sheet** (content-view CTA, management
    card, conversation actions); the `OfferToFulfill` RPC survives for the web handoff.
16. **Availability conflicts: the toggle wins.** When picked gear is already shared in
    the request's community with a different availability, the offer's Lend/Give choice
    is used for the transfer AND updates that community's `CommunityGear` availability —
    the owner just expressed current intent; per-community availability is presentation.

## Potential Solutions

The interview already converged, so these options are recorded for the audit trail with
the losing options' tradeoffs.

### Option A: Transfer origin + claim back-ref, claim-first offer surface (CHOSEN)
- **Approach:** Origin oneof on `Transfer`; gear-linked need claims escalate into child
  transfers; every request seeded with ≥1 need so claims are the single offer path;
  lifecycle coupling via the community-event bus (services stay independent).
- **Pros:** One concept covers requests now and experiences later; reuses the entire
  existing loan machinery, gear picker, inline create, and `ensureRequestOffer`; kills
  the confusing request zero-state (#2701's dead-end); item-based impact for free.
- **Cons:** Touches request creation (need seeding), the needs layer, transfer creation,
  and the request content view in one feature; requires careful bus-driven coupling to
  respect service independence.
- **Effort:** Large (phased below).

### Option B: Request-level offer flow only (transfer origin, no needs-layer changes)
- **Approach:** A dedicated "I have one to lend" CTA beside "Offer to help"; transfer
  carries `origin_request_id`; needs/contributions stay display-only.
- **Pros:** Smallest server delta; no request-creation changes.
- **Cons:** Two competing CTAs (explicitly rejected in the interview); experiences would
  need a second mechanism later; the needs zero-state confusion (#2701) survives.
- **Effort:** Medium.

### Option C: Contribution-as-the-bridge only (no origin field on Transfer)
- **Approach:** The transfer link lives solely on `PlanningContribution`; requests without
  needs synthesize an implicit one at claim time.
- **Pros:** Maximally unified with the planning layer.
- **Cons:** The transfer itself can't answer "what request did I come from" without a join
  through planning rows; freestanding future origins (e.g. a transfer spawned from chat)
  have nowhere to point; fulfillment/undo bookkeeping crosses an extra table.
- **Effort:** Large, with weaker query ergonomics.

## Recommendation

Option A, exactly as settled in the interview. The one architecture-sensitive point is
**how the coupling runs without service-to-service calls** (`server/architecture.md` §
Service Independence):

- **Request → transfer effects** (sibling auto-cancel on `FULFILLED`) and
  **transfer → request effects** (auto-fulfill on handoff) both ride the community-event
  bus, which already carries `TRANSFER_ACTIVE` / `TRANSFER_COMPLETED` / `REQUEST_FULFILLED`
  events with a `topic` oneof. Precedent: story generation and notifications are bus
  subscribers today. Each service subscribes and mutates only its own aggregate.
- **Claim + transfer creation is client-orchestrated in two RPCs** (claim → create
  transfer), not one cross-service mega-RPC. The degraded state if call 2 fails is
  exactly today's behavior (a display-only gear-linked claim), and the client can retry.
  The transfer RPC owns writing the `PlanningContribution.transfer_id` back-ref via the
  shared `server/planning` package (which already owns `ValidateGearLink`).

## Resolved Questions & API shapes

All ten open questions from the first draft were resolved in the round-2 interview
(2026-07-11); the product-level outcomes are in **Design Decisions → Round 2**. The
settled API/implementation shapes, kept here for implementers:

- **`OfferTransfer` RPC** (new; `CreateTransfer` keeps its past-tense contract — per
  proto conventions each RPC's contract evolves independently): fields `gear_id`,
  `transfer_type`, `recipient_user_id`, `community_id`, `origin_request_id`,
  `contribution_id` (optional, for the back-ref), optional `loan_duration_days`; creates
  the transfer in `RECIPIENT_SELECTED`. Per the no-speculative-API-fields rule,
  `origin_experience_id` is **not** added in v1 anywhere (model or API) — the model
  `origin` oneof ships with only `origin_request_id`; the experience variant is added by
  the follow-up issue (extending a oneof is wire-compatible).
- **`RequestGearOffer`** read-path summary on API `Request` (dedicated nested type — no
  message reuse): `transfer_id`, `transfer_type`, `state`, gear id/name/media, helper
  `User`, plus the linked need/contribution id and accepted state so the requester's view
  renders per-need chosen/covered chips. Populated in `buildRequest` with **batch**
  fetches (one query for transfers by `origin_request_id`, one for gear, one for users —
  no per-offer queries; `storage.AssertMaxQueries` guard). Round-trip coverage:
  OfferTransfer → GetRequest must show the offer (the #1142 read-path class).
- **Acceptance** lives on the contribution: `PlanningContribution.accepted_at_unix_sec`
  (optional, near `transfer_id`), requester-set via `AcceptRequestOffer`. Multi-slot needs
  can accept several contributions, up to `slots`.
- **Giveaway handoff auto-fulfills**: `COMPLETED` is the giveaway's handoff (no ACTIVE
  state); the trigger set is loan `ACTIVE` + giveaway `COMPLETED`.
- **Recipient** is always the registered requester. `provisional_recipient_id` exists on
  the model if a future off-app path needs it; it is not exposed on `OfferTransfer`.
- **Need seeding mechanics**: `GenRequest`/`StreamGenRequest` emit the primary asked-for
  thing as a structured seed need (name + slots=1) in the same LLM call; `SubmitRequest`
  enforces the ≥1-need invariant with a title-derived fallback. Requester-only need
  posting is unaffected — seeding acts on the requester's behalf at creation time.

## Open Questions

Minor, implementation-time:

- **Un-accepting an offer** — the requester changes their mind after "this one works." —
  Recommendation: calling `AcceptRequestOffer` on an already-accepted contribution clears
  the marker (toggle semantics), without re-emitting `REQUEST_OFFER_SELECTED`.
- **"Ask was covered" notification copy** — final wording in `server/l10n/source`, gated
  by `notify_transfer_updates` like the other transfer pushes.

## Implementation status (2026-07-11)

**All five phases implemented** → PR #2705, stacked on
`request_walkthrough_combined` (PR #2707). Phases 1–3 = the server core;
Phase 4 = the claim-first client flow (Lend/Give in the claim sheet,
escalation orchestration with undoable toast, accept chip, seed_need plumb,
RequestHelpModal retired); Phase 5 = the restructured mower reel (all five
scenes green, page redeployed), durable-doc updates, and follow-up issue
**#2708** (event child transfers).

**Review round (2026-07-12, reel feedback):**

- **$0-saved RESOLVED.** Root cause: unified-create's SaveGear mapped only
  brand+model out of the AI detection, dropping the value estimate, material,
  and weight (`_gearMetadataFromItemDetails`). State now keeps the raw
  `DetectedGearItem`; save persists the full detection (plus
  `generation_mode`/`source_url` like the legacy gen-gear flow), with
  item-details-sheet edits overlaid at USER provenance. The reel receipt now
  reads $210 saved / 40kg CO₂ from the $420 mower.
- **Impact roll-up:** `impact_metrics.SumTransferImpactDimensions` sums
  money/emissions/time across ALL handed-off child transfers into the
  request's displayed estimate (means add, uncertainties in quadrature;
  single child passes through cloned). Both fulfillment paths feed it every
  handed-off origin transfer. Aggregation masking unchanged (once per child,
  on the transfer).
- **Actor-stream delivery fix:** `BroadcastCommunityEvent` no longer skips
  the attributed actor — the auto-fulfill event is attributed to the
  requester while the helper acted, so the skip left the requester's open
  screen stale for the 30s poll backstop. Scene 04 now shows the request
  closing itself live.
- **Seed need surfaced editable** on both creation previews (unified create
  "Needs" row + legacy modal field), guarded against re-stream clobber.
- **Copy:** "Add or link yours (optional)" claim-sheet hint; picker "Add a
  new item" (photo/text/link); offer pill Offered→Accepted ✓; requests say
  "Who's helping" / "Shared · no reply" / "Share"; repo-wide never-"ask"-as-
  a-noun sweep (protos, Go/Dart comments, docs, home-view server labels,
  push copy, marketing pages). Deliberately left: `docs/users/` product
  vocabulary ("Give/Ask Balance", the "asker" persona) — product decision.

Judgment calls made during implementation:

- **`contribution_id` is required on `OfferTransfer`** (was optional in the
  settled shape): the claim-first flow always produces one, and requiring it
  closes the state hole where a transfer could exist on a request with no
  offer row (the claim's `ensureRequestOffer` drives OFFERS_RECEIVED).
- **Per-need sibling cancel fires only for single-slot needs** — one of three
  wanted rakes arriving must not cancel the other rake offers.
- **The "request was covered" push copy is neutral** ("Offer closed —
  {requester}'s request no longer needs {gear}") because the same event
  covers both automatic stand-downs and the requester declining an offer.
- **Transfer-side undo (owner undoes StartLoan) does not un-fulfill the
  request in v1** — the requester's fulfillment undo covers the mis-tap case and
  also cancels the loan; document as a known asymmetry.
- **Double-count guard** is a server-only `Request.impact_adopted_from_transfer_id`
  marker masked inside both impact calculators (single interception point)
  rather than threading a transfer set through every aggregation site.
- **Drive-by fix:** the models→api `CommunityEventType` numeric cast mislabeled
  later event types on the wire (storage `REQUEST_FULFILLED`=13 surfaced as API
  `REQUEST_OFFER_SELECTED`=13); now converted by name.

## Implementation Plan

### Phase 1: Proto + targeted live transfer (server)
**Goal:** A helper can open a live, requester-targeted loan/giveaway referencing the request.
**Tasks:**
- [ ] `proto/ripls/models/transfer.proto`: add `oneof origin { string origin_request_id = …; }` (placed near `community_id`, per field-order convention; the oneof exists so the follow-up can add the experience variant compatibly — no speculative variant now).
- [ ] `proto/ripls/models/planning.proto`: add `optional string transfer_id` to `PlanningContribution` (near `gear_id`).
- [ ] `proto/ripls/api/transfer.proto` / `transfer_service.proto`: API `Transfer.origin_request_id` (expose only what the client renders), new `OfferTransferRequest/Response` (dedicated messages; no reuse), creating the transfer in `RECIPIENT_SELECTED` with `recipient = requester`, `owner = caller`.
- [ ] `proto/ripls/api/request.proto`: repeated `RequestGearOffer` read-path summary on `Request` (see Resolved Questions & API shapes) so the requester's view can render offered items.
- [ ] `server/services/transfer/offer.go`: validation (caller owns gear; gear `AVAILABLE`; caller ≠ requester; caller + recipient active members of `community_id`; request exists, is shared into that community, and is `ACTIVE`/`OFFERS_RECEIVED` — request state read via shared storage, never by calling the request service); write transfer; auto-share gear into the community with the chosen availability (existing `shareGearToCommunity` core); write the contribution back-ref via `server/planning`; post a system message with gear reference into the request conversation; emit `TRANSFER_RECIPIENT_SELECTED` community event. Log with `transfer_id` + `target_request_id` (never `request_id`, which is the HTTP correlation key).
- [ ] `buildRequest` enrichment: batch-fetch origin-linked transfers + gear + helpers (no per-offer queries; `storage.AssertMaxQueries` regression test).
- [ ] Round-trip tests for every new API field (incl. OfferTransfer → GetRequest read-path); unit tests for the validation matrix; integration test: offer → transfer visible in `ListMyTransfers`/`ListReceivedTransfers` with origin populated.
**Validates:** the child-transfer model works end-to-end at the API level before any UI.

### Phase 2: Lifecycle coupling via the event bus (server)
**Goal:** The request morphs into the loan — and back out cleanly on cancellation.
**Tasks:**
- [ ] Request-service bus subscriber (fulfillment): on `TRANSFER_ACTIVE` / `TRANSFER_COMPLETED` events whose transfer carries `origin_request_id` — if the request has **at most one need** (decision 9; includes legacy zero-need requests), run the fulfillment core (state → `FULFILLED`, `confirmed_helper_ids = [owner]`, fulfillment linked to the transfer, transfer-adopted impact, `RequestFulfillmentUndo` snapshot populated, community events attributed to the transfer owner as actor) unless already terminal; if multi-need, mark the linked need covered and do not fulfill. Idempotent — a replayed or racing event is a no-op. Failure handling is warn-and-continue **by documented design**: the loan is already physically underway, and the requester's manual `MarkRequestFulfilled` is the explicit fallback; log at Error with `transfer_id` + `target_request_id` so the miss is visible.
- [ ] Transfer-service bus subscriber (sibling cancel): on need coverage and on `REQUEST_FULFILLED`, cancel sibling non-terminal origin-linked transfers **scoped to the covered need** (all remaining ones on full fulfillment), with a "this ask was covered" notification (copy in `server/l10n/source`; reuses the existing `TRANSFER_CANCELLED` event type, so no new registry entries).
- [ ] Undo unwind (decision 10): `UndoMarkRequestFulfilled` on a transfer-linked fulfillment restores the request AND cancels the linked transfer — bus event consumed by the transfer service, whose existing cancel machinery restores gear state. Already-cancelled siblings stay cancelled (terminal states don't revive).
- [ ] Cancel unwind (decision 11): request-service subscriber on `TRANSFER_CANCELLED` for origin-linked transfers — release the claimed slot, clear `PlanningContribution.transfer_id` and any accepted marker, withdraw the helper's `RequestOffer` unless another live contribution of theirs remains on the request; the existing last-offer reversion then governs request state. Must not re-trigger on the cancellations the sibling-cancel subscriber itself emits post-fulfillment (guard on request terminal state).
- [ ] `AcceptRequestOffer` RPC (request service): requester-only; validates the contribution belongs to this request and carries a transfer; sets `PlanningContribution.accepted_at_unix_sec` (new optional model field, near `transfer_id`); emits `REQUEST_OFFER_SELECTED` — the first real emitter. No registry work needed (already classified in both the undo and digest registries), but verify the digest counter and the client event-router invalidation behave as expected on first emission.
- [ ] Note: fulfillment via the bus is asynchronous — the handoff RPC response won't carry the fulfilled request. The existing realtime layer (event router invalidation on `TRANSFER_*`/`REQUEST_*` events) refreshes the requester's view; verify that path in the integration test.
- [ ] `MarkRequestFulfilled` (manual path): no new request field — impact adopts the item-based estimates of every handed-off origin-linked transfer (counted once, QT stays request-side); the Mark Fulfilled modal pre-populates confirmed helpers from them.
- [ ] Integration tests: single-need handoff auto-fulfills; multi-need handoff covers only its need and cancels that need's siblings; giveaway completion auto-fulfills; pre-handoff cancel reopens the slot, withdraws the offer, and reverts request state when it was the last offer; undo of an auto-fulfillment cancels the loan and restores gear state; impact equals the transfer's item-based estimate (regression for #2275); accept marker round-trips and clears on cancel.
**Validates:** the "morph" promise and the impact fix, with services still independent.

### Phase 3: Every request born with a need (server + create flow)
**Goal:** Kill the request zero-state so the needs list is always a valid offer surface.
**Tasks:**
- [ ] `GenRequest`/`StreamGenRequest`: emit a structured primary seed need (name, slots=1) alongside existing breakdown pieces.
- [ ] `SubmitRequest`: server-side invariant — seed the provided/generated need(s); fallback to a title-derived single need. No backfill of existing requests.
- [ ] Client create flow passes the seed need through; requester can edit/remove post-creation via existing needs management.
- [ ] Tests: creation always yields ≥1 need across gen, plain, and web paths; title fallback.
**Validates:** the claim-first CTA always has something to open onto.

### Phase 4: Claim-first offer flow (client)
**Goal:** One "Offer to help" CTA; claiming with gear starts the loan in one motion.
**Tasks:**
- [ ] Request content view: single primary CTA opens the needs volunteer/claim sheet; retire the `RequestHelpModal` chat-first offer path (management card + conversation actions reroute); thread stays reachable.
- [ ] Claim confirm sheet (request scope): gear link becomes the prominent affordance for physical needs; add the Lend/Give toggle (default lend; reuse `UnifiedLendGiveToggle`); confirming a gear-linked claim calls claim → `OfferTransfer` (client-orchestrated; failure of call 2 degrades to today's display-only link with a retry affordance).
- [ ] Inline create: existing `NeedsClaimGearPicker._openCapture` flow unchanged; after `OfferTransfer`, confirmation notes the gear is now visible to the community, with undo (unshare).
- [ ] Requester view: offered items render on the request (gear pill + helper) with the optional "this one works" accept chip → `AcceptRequestOffer`; chosen state and covered-need state render on the needs rows. Complements #2701's offer-visibility fixes.
- [ ] Experience-scope parity check per `client/needs.md`: the toggle/escalation is request-scope-only for now — record it under **Intentional differences** with a pointer to the follow-up issue so it isn't flagged as drift.
- [ ] l10n: every new string in both `app_en.arb` and `app_es.arb`; ViewModel/repository unit tests; a11y semantics on new toggles/chips.
**Validates:** the frictionless helper journey on the real UI.

### Phase 5: Walkthrough + docs
**Goal:** The #2700 reel shows the mower becoming a real lent item; docs stay true.
**Tasks:**
- [ ] After PR #2700 merges: extend `e2e/tests/walkthroughs/requests.spec.ts` — Theo claims the seeded "lawn mower" need, snaps his mower inline (Lend default), June optionally accepts, handoff fulfills, impact counted from the real item. Mind the classifier-cue warning (create prompt stays gear-word-free; the *gear* creation happens in Theo's capture, where gear typing is natural).
- [ ] e2e correctness twin (`e2e/tests/workflows/`) for the claim→lend→fulfill loop; seed helpers for `OfferTransfer`.
- [ ] Update `docs/workflows/request.md`, `docs/workflows/loan.md`, `docs/workflows/delta.md`, `docs/planning.md`, `docs/client/needs.md` (+ their `context:`/freshness stamps); regenerate `docs/llms.txt` if frontmatter changes.
- [ ] File the follow-up issue: event child transfers (experience gear-contributions escalating to event-duration loans), feeding walkthrough #2703.
**Validates:** the flagship journey on film, regression-gated, documented.

## Audit notes (2026-07-11)

Applied during the `/ripls-audit` pass; each item names the standard it enforces.

1. **Dropped speculative `origin_experience_id`** from v1 protos (model + API) — the
   no-speculative-API-fields rule; the oneof is extended compatibly by the follow-up.
2. **Added the read-path surface for offered items** (`RequestGearOffer` on API `Request`
   + batch-fetched `buildRequest` enrichment + `AssertMaxQueries`) — the plan had the
   requester-side UI with no API carrying its data (the #1142 read-path class; SQL-efficiency
   rules).
3. **Added `caller ≠ requester`** to the OfferTransfer validation matrix and transfer
   ownership validation for `AcceptRequestOffer` (authorization checklist).
4. **Documented the warn-and-continue rationale** for the auto-fulfill subscriber and its
   manual-fulfillment fallback, with `target_request_id` logging (fail-fast convention
   requires an explicit documented reason; `request_id` is reserved for the HTTP
   correlation key).
5. **Clarified registry obligations**: no new `CommunityEventType` values are introduced,
   so no undo/digest registry entries are needed; `REQUEST_OFFER_SELECTED` is already
   classified in both — first emission needs verification, not classification.
6. **Recorded the async-fulfillment consistency note** (bus-driven fulfillment is not
   visible in the handoff RPC response; realtime invalidation covers the refresh).
7. **Added undo semantics for auto-fulfillment** and **transfer-cancel vs. claim/offer
   cleanup** as Open Questions with recommendations (undo registry design; parity with
   existing `RequestFulfillmentUndo` machinery).
8. **Actor attribution** for system-initiated community events (fulfillment attributed to
   the transfer owner) so notifications/digest render a sensible actor.
9. **Cross-service reads clarified**: transfer-service validation reads `models.Request`
   via shared storage, never via the request service (service-independence rule).

**Round-2 addendum (2026-07-11):** the open-questions interview subsequently **overrode**
the audit's recommendations in item 7 — undo of an auto-fulfillment now also cancels the
linked transfer, and transfer cancellation now unwinds the claim/offer (Design Decisions
10–11) — and replaced the `Request.accepted_transfer_id` shape with the per-need
contribution marker (Design Decision 8). The remaining audit items stand as applied.

Docs consulted: `docs/llms.txt`, `proto_conventions.md`, `server/architecture.md`,
`server/conventions.md`, `users/priorities.md`, `workflows/request.md`,
`workflows/loan.md`, `workflows/giveaway.md`, `workflows/delta.md`, `planning.md`,
`client/needs.md`, `ad_hoc_communities.md`, `single_player_mode.md`,
`design/unified-create.md`, and the api_design / security / error_handling / testing
review prompts ("Areas to Review").

## Related Issues
- #2701 — sibling: offer state invisible on the request view; the claim-first CTA resolves its needs-composer dead-end, its avatar/offer-count display work remains complementary.
- #2275 — the impact symptom this fixes ("backdoor loan" shows $0); Phase 2 closes it.
- #2703 — requests walkthrough #2 (multi-helper yard day); the fulfill-via-loan beat depends on this issue.
- #2686 / PR #2700 — the first requests reel this feature upgrades.
- #2687 — Lend/Give plumbing (`SaveGearRequest.availability`) this feature reuses.
- #2266 — transfers as a first-class content type; child transfers raise its priority.
- #2492 — phone-first epic; the web offer handoff constraint comes from there.
