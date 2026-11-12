# Issue #2488: Inbox follow-ups: action cleanup, contextual nudges, transactional bulk edits, and UX test coverage

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2488
**Filed:** 2026-06-17 | **Last activity:** 2026-06-17
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** [docs/client/inbox.md](../client/inbox.md), [docs/client/calendar.md](../client/calendar.md), [docs/client/architecture.md](../client/architecture.md), [docs/client/conventions.md](../client/conventions.md), [docs/client/testing.md](../client/testing.md), [docs/client/testing/semantics_identifiers.md](../client/testing/semantics_identifiers.md), [docs/client/modals.md](../client/modals.md), [docs/proto_conventions.md](../proto_conventions.md), [docs/reviews/api_design_review_prompt.md](../reviews/api_design_review_prompt.md), [docs/reviews/error_handling_review_prompt.md](../reviews/error_handling_review_prompt.md), [docs/reviews/accessibility_review_prompt.md](../reviews/accessibility_review_prompt.md), [docs/reviews/testing_review_prompt.md](../reviews/testing_review_prompt.md)

> **This is an epic.** Issue #2488 bundles five independently-shippable inbox
> refinements. The bundling is intentional ("each is independently shippable"),
> but a single PR for all five would be too large. **Recommendation: split into
> five tracked sub-issues**, one per numbered item, and treat the phases below as
> those sub-issues. Items 2 and 3 **reuse the existing nudge system** (not a new
> engine — see [docs/nudges.md](../nudges.md)); item 2 lands the shared
> `GetHomeViewResponse.nudge` plumbing and item 3 reads the same field, so 3
> depends on 2. Item 5 should land *after* the surfaces it tests stop moving (i.e.
> after 1 and 4), or its selectors churn.

## Problem Statement

The redesigned Home tab inbox (#2435 — root preview plus the Needs-you, Yours,
and Calendar see-all screens) shipped, and building it surfaced five rough edges.
None is a bug; all are polish/tech-debt that makes the inbox feel less coherent or
harder to test:

1. **Inconsistent Needs-you actions.** A "chip" tap dispatches through
   [`runDecisionAction`](../../app/lib/presentation/screens/portfolio/home_decision_routing.dart)
   to wildly different surfaces by kind: lend/give accept *in place* (snackbar
   undo), mark-done opens a *modal*, pickup (`TRANSFER_UPDATE`) opens a full
   *gear screen*, reply/thanks open a *chat thread*. Same affordance, three
   surface types — unpredictable.
2. **Static empty-day calendar nudge.** `_buildNudge` in
   [`up_next_week_view.dart`](../../app/lib/presentation/widgets/home/up_next_week_view.dart)
   shows one static "Suggest a plan ›" for any empty day. It could carry a
   *contextual* suggestion (return the drill you borrowed, host a weekend walk).
3. **Dumb zero-state.** [`HomeZeroStateCard`](../../app/lib/presentation/widgets/home/home_empty_states.dart)
   rotates through 5 *canned* example asks rather than real, personalized
   suggestions. Should share the nudge engine with #2.
4. **Non-transactional bulk edits.** Every bulk path on the Yours screen —
   `_bulkComplete` / `_bulkShare` / `_bulkSetLocation` in
   [`home_gear_see_all_screen.dart`](../../app/lib/presentation/screens/portfolio/home_gear_see_all_screen.dart),
   `bulkWrapUp` in
   [`home_tab_view_model.dart`](../../app/lib/presentation/viewmodels/home_tab_view_model.dart),
   and the repo-level `updateMultipleLocations` — fires **one mutation per item in
   a loop**. Items disappear one-by-one (flicker), undo is hard, and partial
   failure is reported with a generic "something went wrong."
5. **No e2e coverage of inbox permutations.** Four screens × many states (populated
   / empty / zero, each decision kind, each Yours section, calendar density 0 /
   1–3 / 4+, undo flows) and no Playwright coverage. Needs stable
   `semanticsIdentifier`s first.

**Ambiguities to resolve before coding:** the issue gives *direction*, not
specs. Items 1–3 are genuinely design-led (the "right" interaction model and
nudge sources aren't pinned down). Items 4–5 are well-specified and ready to
build.

## Context from Comments

**The issue has no comments.** Everything is in a single structured description
from the author. That's fine for the mechanical items (4, 5), but the
design-heavy items (1, 2, 3) carry no product decision yet — there's no agreed
interaction model for #1, no nudge-source list for #2/#3, and no ranking/frequency
policy. Those need a product/design pass (or at least a maintainer decision)
before implementation, which is reflected in Open Questions and in the phasing.

## Potential Solutions

Because the five items are loosely coupled, the "options" that matter are
**how to sequence them** and **a couple of genuine design forks within items**.

### Option A: Ship the two ready items first, design the rest (recommended)
- **Approach:** Land #4 (transactional bulk) and #5 (semantics + e2e) — both
  fully specified and high-value — first. In parallel, run a small design spike
  for #1 (interaction model) and #2/#3 (nudge engine), then implement those once
  decided. Item 5's selectors stabilize after #1/#4, so order is 4 → (1, design)
  → 5 → (2, 3).
- **Pros:** Immediate UX win (bulk flicker) and a testing safety net before the
  design-churny screens move. Doesn't block mechanical work on undecided design.
- **Cons:** #5 partly re-touches surfaces changed by #1; some selector rework.
- **Effort:** medium overall; #4 small, #5 medium, #1 medium, #2/#3 large.

### Option B: Design-first — settle the interaction model (#1) before everything
- **Approach:** Resolve #1's per-kind interaction model first since it reshapes
  the Needs-you surfaces that #5 tests, then #4, then nudges, then #5 last.
- **Pros:** Tests are written once against final surfaces.
- **Cons:** Blocks all shippable work behind a design decision with no current
  owner; #4 (pure win, independent of #1) waits for no reason.
- **Effort:** large lead time before anything ships.

### Option C: One big PR for all five
- **Approach:** Do everything together.
- **Cons:** Violates the issue's own "independently shippable" framing; huge
  review surface; mixes server proto changes (#4 batch RPC) with pure-UI design
  work; likely to stall. Not recommended.
- **Effort:** large, high-risk.

### Design fork inside #4 — client-orchestrated vs. server batch RPC
- **4a (client-only, single optimistic update):** Keep N writes under the hood
  but make the UX atomic — one optimistic removal of the whole selection, one
  toast, roll back the set on any failure (or report "N of M done"). Small, no
  proto change, no server work. The flicker is the *UX* problem the issue leads
  with, and this fixes it.
- **4b (server batch RPC):** Add a real batch endpoint so the operation is
  *atomic server-side*. There's precedent — `BatchAddExperienceNeeds`,
  `BatchClaimRequestNeeds` in the proto. But "complete" spans three services
  (request / experience / transfer) and two kinds (fulfill / complete event /
  complete loan), so a single atomic RPC is awkward and cross-service. **If
  pursued, each batch RPC must follow the proto conventions**: dedicated,
  non-reused request/response types with flattened fields, a *bounded* repeated
  id list (not unbounded), and a per-item result list in the response so the
  client can report partial success — not a single bool. The service-independence
  rule means a batch endpoint lives on the owning service (request / experience /
  transfer), never a cross-service mega-RPC.
- **Recommendation:** Do **4a now** (it solves the stated UX problem cheaply),
  and only pursue 4b if true atomicity is required — and if so, scope it per
  service, not one mega-RPC, to respect the service-independence rule.

### Items #2/#3 — reuse the existing nudge system (not a new engine)
**There is already a production nudge system** ([docs/nudges.md](../nudges.md)) —
AI-generated, per-`(user, community)`, with a server generation/storage pipeline,
a `NudgePayload` API type, three rendered card variants, CTA→creation-modal
routing, consume/seen-expiry, and async stock imagery. The CTA actions
(`plan_experience`, `list_item`, `ask_for_help`, `check_listings`) are *exactly*
the "host a walk / share the drill / ask the crew" suggestions items #2/#3 want.
So this is not "build a nudge engine" — it's **surface the existing one on two
new inbox locations**. Two reuse facts make it close to implementation-ready:

- **Client visuals already exist and are already reused off-feed.**
  `NudgeContentView({required NudgePayload nudge})`
  ([nudge_content_view.dart](../../app/lib/presentation/widgets/nudge/nudge_content_view.dart))
  is rendered today in both `feed_screen.dart` and `nudge_detail_screen.dart` —
  rendering it on the empty calendar day and in the zero-state is a third reuse
  site, not new UI. `NudgeCardVariants` and the `nudge*Style` theme tokens carry
  the visuals (reuse them per [feedback_theme_reuse]).
- **`NudgePayload` already anticipates non-feed surfaces.** It carries
  `context_id`, `kicker`, and `community_id` documented as "Workshop-surface
  nudges populate this… Empty for feed-surface nudges" — i.e. the schema is
  already designed for surfaces beyond the feed. The inbox is another such
  surface; no new payload type is needed.

The only genuine fork is **where the inbox nudge comes from on the wire:**
- **2a (surface on `GetHomeViewResponse`) — recommended.** Add an optional
  `NudgePayload nudge` (or a small `repeated`) field to `GetHomeViewResponse` and
  populate it in [`home_view.go`](../../server/services/portfolio/home_view.go)
  by reusing the feed package's `getActiveNudgesForUser` /
  `generateAndPersistNudges` and the existing `NudgePayload` builder
  ([feed_nudges.go](../../server/services/feed/feed_nudges.go) `nudgeToFeedItem`).
  One fetch path, one cache, consistent with how every other inbox section is
  server-assembled.
- **2b (separate fetch from the inbox client).** Client calls a nudge RPC
  directly. Rejected — the inbox is built on the "one RPC, `GetHomeView`"
  principle ([inbox.md](../client/inbox.md)); a side fetch breaks that and the
  single-cache model.

**Recommendation:** 2a. Reuse the existing generation/storage + `NudgePayload` +
`NudgeContentView`; the inbox-specific work is (a) one new response field, (b)
picking which community to scope generation to (the viewer's primary/most-active
community — `community_id` already rides on the payload), and (c) two new render
sites. Consumption reuses the existing `ConsumeNudge` RPC and `consumedNudgeIds`
pattern.

## Recommendation

**Adopt Option A**, split #2488 into five sub-issues, and sequence:

1. **#4 first** with approach **4a** (client-orchestrated atomic UX, no proto
   change) — biggest perceived win for least work and risk.
2. **#1** with the accepted interaction model (Decisions, below) — it reshapes
   the surfaces #5 will test.
3. **#5** after #1/#4 settle, so semantic identifiers and specs are written once.
4. **#2 then #3** by **reusing the existing nudge system** (approach 2a): surface
   a `NudgePayload` on `GetHomeViewResponse` and render it with the existing
   `NudgeContentView`. #3 shares the engine with #2 by reading the same field.

This front-loads shippable value, keeps proto/server churn out of the critical
path, and writes e2e tests against stable surfaces.

## Decisions (accepted)

These started as open questions; the answers below are **accepted** and drive the
Implementation Plan. No design sign-off remains blocking.

- **#1 chip interaction model — ACCEPTED:** "**chip = the lightest surface that
  completes the action; body tap = navigate to the item**." Concretely: keep
  lend/give as in-place accept (already lightest); convert `TRANSFER_UPDATE`
  pickup from a full gear-screen push to an **in-place confirm sheet**; keep
  mark-done as a modal *only* when it collects helpers, else an in-place confirm;
  keep reply/thanks → thread (a chat genuinely needs the thread). Reduces three
  surface types to "in-place confirm + thread," modals reserved for data-
  collecting completions.
- **#1 confirm/undo home — ACCEPTED:** standardize on the existing
  `ToastHelper.showServerUndo` snackbar-undo already used by lend/give accept,
  routed through **repository** methods (not raw services) so the home view
  re-surfaces on undo — the contract [inbox.md](../client/inbox.md) already
  documents. Apply uniformly to every newly in-place action.
- **#4 atomic UX vs server transaction — ACCEPTED: 4a now** (client-orchestrated
  atomic UX, no proto change). 4b (per-service batch RPC) only if product later
  wants true all-or-nothing.
- **#4 partial failure — ACCEPTED: keep successes, report precisely** ("Marked 4
  of 6 — 2 couldn't be completed"), mirroring `bulkWrapUp`'s partial-rollback
  shape, not the generic `commonSomethingWentWrong`.
- **#2/#3 nudge engine — ACCEPTED: reuse the existing nudge system (2a).** Do
  **not** build a new client engine. Surface a `NudgePayload` on
  `GetHomeViewResponse` via the feed package's existing generation/storage
  (`getActiveNudgesForUser` / `generateAndPersistNudges` + the `NudgePayload`
  builder), and render it with the existing `NudgeContentView` on the empty
  calendar day (#2) and zero-state (#3). #3 shares the engine with #2 by reading
  the same response field. (See "Items #2/#3 — reuse the existing nudge system".)
- **#2/#3 nudge content/ranking — ACCEPTED: server-owned, reused.** Nudge copy,
  CTA coherence, ranking, and seen/consume expiry already live in the feed nudge
  pipeline and AI prompt; the inbox inherits them. No new client ranking logic.
  Generation is scoped to the viewer's **primary/most-active community**, and the
  payload already carries `community_id`. Open sub-decision (non-blocking): cap to
  **one** nudge per inbox surface vs. a small pool — default to one per surface to
  start (the calendar shows one empty-day nudge; the zero-state shows one hero).
- **#5 e2e priority — ACCEPTED:** prioritize state-changing flows (each Needs-you
  action + undo, Yours bulk edit, zero-state CTA tap → consume) over pure-render
  density; cover calendar 0/1–3/4+ as render smoke tests. Follow
  [testing/update_system_tests.md](../testing/update_system_tests.md).

### Still genuinely open (non-blocking, decide during build)
- **Which community scopes inbox-nudge generation** when the viewer has several —
  start with most-active; revisit if it feels arbitrary.
- **Nudge freshness on a cached home view** — the feed relies on `GetFeed`
  re-fetches to apply seen-expiry; the inbox is cached under `portfolio:home_view`,
  so confirm seen/consume bookkeeping still advances (consume already dismisses
  optimistically via `consumedNudgeIds`; expiry advances on the next `refresh()`).

## Implementation Plan

> Each phase is a standalone sub-issue / shippable increment. With the Decisions
> above accepted, **all five are implementation-ready** — none is blocked on a
> design call. Phases 4–5 (the nudges) are lighter than they first appear: the
> generation pipeline, payload type, card visuals, and consume/expiry already
> exist; the work is one response field plus two render sites.

### Phase 1 (Item #4): Transactional bulk edits
**Goal:** Bulk operations on the Yours screen read as one committed action, not N
flickers, with precise partial-failure reporting.
**Tasks:**
- [ ] In `_bulkComplete` / `_bulkShare` / `_bulkSetLocation`, apply a **single
      optimistic removal** of the whole selection up front (not per-item), and a
      **single toast** at the end.
- [ ] Route bulk completion through the view model (extend the `bulkWrapUp`
      pattern) so optimistic removal + rollback live in one place, mirroring its
      partial-failure rollback.
- [ ] Replace the generic `commonSomethingWentWrong` partial-failure path with a
      precise "N of M" message (reuse the `homeBulkLocationPartial` shape, a new
      ARB key — no hardcoded strings; the message is resolved at the widget layer
      via `context.l10n`, not in the view model).
- [ ] **Log failures, don't swallow them.** Today `_bulkComplete` collapses each
      error into `catch (_)`; mirror `bulkWrapUp`'s `_log.warning(...)` so silent
      failures are observable (error_handling rubric → "Silent Failures").
- [ ] Keep the loop under the hood for now (4a); do **not** add a batch RPC.
- [ ] Confirm cache invalidation still fires once after the batch (currently each
      repo call invalidates `'*'`) — debounce or invalidate once to avoid N
      refreshes.
- [ ] Unit-test the view-model bulk path: full success, partial failure (rollback
      of failures only), empty selection.
**Validates:** the flicker/incrementing complaint is gone and partial failures
are honestly reported, with no proto/server change.

### Phase 2 (Item #5): Semantics identifiers + e2e coverage
**Goal:** Stable Playwright coverage of the inbox permutations.
**Tasks:**
- [ ] Add kebab-case `semanticsIdentifier`s to the key interactive elements
      (Needs-you action chips per kind, Yours bulk-edit controls, zero-state
      CTAs, calendar day cells), per
      [semantics_identifiers.md](../client/testing/semantics_identifiers.md).
- [ ] Add e2e specs for: root previews, zero-state CTAs, each Needs-you action +
      undo, Yours bulk edit (post-Phase 1 behavior), calendar densities 0/1–3/4+.
- [ ] Follow the e2e companion-spec conventions in
      [update_system_tests.md](../testing/update_system_tests.md).
**Validates:** the inbox is regression-protected before the design-churny items
(1–3) reshape it. (Sequence after Phase 3 if #1 lands first, to avoid selector
churn — see the epic note.)

### Phase 3 (Item #1): Consistent Needs-you interaction model
**Goal:** A predictable per-kind interaction model; fewer surface types.
**Tasks:**
- [ ] Document the accepted chip-vs-body / in-place-vs-modal-vs-navigate rule
      (Decisions) and update [inbox.md](../client/inbox.md)'s action table.
- [ ] Convert `TRANSFER_UPDATE` pickup from a full gear-screen push to an
      in-place confirm sheet in `runDecisionAction`. Any new confirm sheet must
      use the unified glass bottom-sheet surface via `showAccessibleModal`
      (see [modals.md](../client/modals.md)) — not an ad-hoc `showModalBottomSheet`.
- [ ] Reduce other full-screen/modal surfaces to lightweight confirm sheets where
      no extra data is collected; keep modals only for helper-collecting
      completions.
- [ ] Standardize undo on `ToastHelper.showServerUndo` routed through
      **repository** methods so the home view re-surfaces on undo.
- [ ] Keep the root preview and Needs-you see-all in lock-step (they already
      share `HomeDecisionRouting`).
**Validates:** every chip behaves predictably; the same affordance no longer
drops the user into three different surface types.

### Phase 4 (Item #2 + the #2/#3 shared plumbing): Inbox nudges on empty calendar days
**Goal:** Empty calendar days carry a real, contextual nudge — reusing the
existing nudge system, not a new engine. This phase also lands the shared
plumbing #3 depends on.
**Server tasks (reuse the feed nudge pipeline):**
- [ ] Add an optional `NudgePayload nudge` field to `GetHomeViewResponse` in
      `portfolio.proto` (next free field number; flattened per
      [proto_conventions.md](../proto_conventions.md)). Reuse the existing
      `NudgePayload` type — it's a shared payload sub-message, not an RPC
      request/response, so the no-cross-RPC-reuse rule doesn't apply.
- [ ] Populate it in [`home_view.go`](../../server/services/portfolio/home_view.go)
      by calling the feed package's existing `getActiveNudgesForUser` /
      `generateAndPersistNudges` and the `NudgePayload` builder in
      [feed_nudges.go](../../server/services/feed/feed_nudges.go) (`nudgeToFeedItem`).
      If those helpers are unexported, lift the shared bits into a small exported
      surface rather than copying — respect feed service ownership
      ([server/architecture.md](../server/architecture.md): service independence;
      don't reach into another service's internals — expose a function).
- [ ] Scope generation to the viewer's primary/most-active community; the chosen
      `community_id` already rides on `NudgePayload`. Degrade gracefully to no
      nudge when AI/stock-imagery providers are nil (same as the feed).
- [ ] Reuse seen/consume expiry — no new freshness logic.
**Client tasks (reuse the visuals):**
- [ ] In `_buildNudge` ([up_next_week_view.dart](../../app/lib/presentation/widgets/home/up_next_week_view.dart)),
      when the home view carries a nudge for an empty day, render the existing
      `NudgeContentView(nudge: …)` instead of the static "Suggest a plan ›"; keep
      the static prompt as the no-nudge fallback.
- [ ] Wire CTA consumption through the existing `ConsumeNudge` RPC and the
      `consumedNudgeIds` dismiss pattern (mirror `FeedNotifier`), so a tapped
      inbox nudge disappears optimistically and re-fetches via `refresh()`.
- [ ] No new copy strings or client ranking — nudge text/CTA come from the
      payload (server-generated), exactly like feed nudges; the
      "viewmodels-never-resolve-strings" rule is satisfied by construction.
- [ ] **a11y:** `NudgeContentView` should already carry its semantics; verify the
      empty-day render keeps a meaningful label and ≥48×48 CTA, and wrap the
      swap-in region in a `LiveRegion` if it isn't already
      ([accessibility.md](../client/accessibility.md)).
**Validates:** the existing nudge system surfaces in the inbox end-to-end (server
field → existing card → consume), proving the shared plumbing for #3.

### Phase 5 (Item #3): Smarter zero-state, sharing the same nudge field
**Goal:** The zero-state shows the same real nudge instead of canned rotating
examples — sharing the engine with #2 by reading the same `GetHomeViewResponse`
field landed in Phase 4.
**Tasks:**
- [ ] Have [`HomeZeroStateCard`](../../app/lib/presentation/widgets/home/home_empty_states.dart)
      render the home view's `NudgePayload` via `NudgeContentView` in place of the
      5 canned rotating examples — **keep the canned rotation as the fallback**
      when no nudge is present (brand-new users with no community activity).
- [ ] Keep the "ask the crew" / "offer something" CTAs alongside the nudge.
- [ ] Reuse the same `ConsumeNudge` + `consumedNudgeIds` consumption from Phase 4.
- [ ] Reuse existing nudge theme tokens/visuals — no new per-section colors or
      fonts ([feedback_theme_reuse]).
- [ ] Widget-test: nudge present → renders `NudgeContentView`; nudge absent →
      falls back to the rotating examples.
**Validates:** the nudge engine is proven shared across two inbox surfaces with no
duplicated generation, ranking, or copy.

## Related Issues
- #2435 — the Home tab inbox redesign (v4.1) these five items polish; the parent
  feature.
- #2420 — doc-drift guard; touching the inbox surfaces should keep
  [inbox.md](../client/inbox.md) fresh via the ratchet.

---

## Audit Notes

Audited against the documentation context map (`docs/llms.txt`). Findings folded
into the plan above:

- **nudge reuse (#2/#3)** — Rewrote items #2/#3 against the existing nudge system
  ([docs/nudges.md](../nudges.md)) per the user's direction: reuse the feed
  generation/storage pipeline, the `NudgePayload` type, `NudgeContentView`/
  `NudgeCardVariants` visuals, and the `ConsumeNudge`/`consumedNudgeIds` flow.
  Net new work is one `GetHomeViewResponse.nudge` field + two render sites, not a
  new engine.
- **api_design** — New `NudgePayload nudge` on `GetHomeViewResponse` is a reused
  *payload* sub-message (allowed) on a dedicated response (the no-cross-RPC-reuse
  rule targets request/response types, not shared payloads). For the optional 4b
  batch-RPC fork: dedicated/non-reused request-response types, bounded repeated id
  lists, a per-item result list (not a single bool), per-service ownership, per
  [proto_conventions.md](../proto_conventions.md).
- **service independence** — Phase 4 reuses the feed nudge helpers via an exported
  surface rather than reaching into feed-service internals
  ([server/architecture.md](../server/architecture.md)); copy-paste was rejected.
- **error_handling** — Phase 1 now requires logging bulk failures (`_log.warning`)
  instead of the current `catch (_)` swallow, and precise per-item partial-failure
  reporting (keep successes, report "N of M"). Flagged "Silent Failures."
- **i18n** — Phase 1's new toast strings are ARB keys resolved at the widget layer
  via `context.l10n`. The earlier "structured nudge descriptor" concern dissolves
  with reuse: nudge copy is server-generated and arrives as payload data (like
  feed nudges), so no client-side string resolution is introduced.
- **accessibility** — Phase 3 routes new confirm sheets through
  `showAccessibleModal` (glass surface); Phase 4 verifies `NudgeContentView`'s
  existing semantics carry over to the empty-day render (meaningful label,
  `LiveRegion`, ≥48×48 CTA). Phase 5's `semanticsIdentifier`s remain kebab-case
  per [semantics_identifiers.md](../client/testing/semantics_identifiers.md).
- **architecture/caching** — Phase 1 keeps bulk completion in the view model
  (MVVM) and collapses the per-call `invalidatePattern('*')` into one refresh.
- **testing** — Phase 1 unit-tests the view-model bulk path (success / partial /
  empty); Phase 5 e2e covers state-changing flows first per
  [update_system_tests.md](../testing/update_system_tests.md).

**Docs consulted:** client/inbox.md, client/calendar.md, client/architecture.md,
client/conventions.md, client/i18n.md, client/accessibility.md, client/modals.md,
client/testing.md, client/testing/semantics_identifiers.md, nudges.md,
server/architecture.md, proto_conventions.md, and the api_design / error_handling
/ accessibility / testing review prompts.
