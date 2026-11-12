# Issue #1057: [Feature Request] Start your first loan should be an action + suppress stale nudges

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1057
**Filed:** 2026-04-07 | **Last activity:** 2026-04-07
**Labels:** enhancement, user-feedback | **Assignees:** unassigned
**Context docs:** none

> **Audit notes (2026-04-07):** Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, and `docs/server/architecture.md`. Plan is compliant with MVVM/Riverpod architecture and `homeProvider.navigateToTab()` is the established tab-switching primitive (see `app/lib/services/post_creation_service.dart:33`). `NavigationHelpers` does **not** apply here because this is in-tab navigation, not a screen push. No server changes are involved.
>
> **Update (2026-04-07, second pass):** Scope expanded to also suppress all three next-step nudges (loans, experiences, requests) when the user already has an in-flight activity of that type. Per `docs/client/architecture.md` §1, the gating logic must live in a viewmodel — Phase 2 extends `CommunityImpactData` rather than reading repositories from the screen. Per `docs/client/caching.md`, the repository layer already caches `listMyTransfers`/`listMyExperiences`/`listMyRequests` with namespace isolation, so the extra fetches are cheap and existing mutation paths already invalidate the relevant caches (Phase 4 verifies this rather than re-implementing it). Per-user fetches are wrapped in try/catch to fail open — a transfer-service hiccup must not break the impact screen.

## Problem Statement

There are **two related problems** with the "next step" nudges on the community impact metrics screen (Metrics tab → community view):

**Problem 1 — "Start your first loan" is inert.**
The "listed" stage shows a `NextStepCard` titled **"Start your first loan"** with description *"Turn this potential into real savings. Share something with a neighbor today."* The screenshot shows it as a static info panel — no chevron, no tap affordance, nothing happens when tapped. The two sibling cards in the same `_buildListedNextStep` switch — **"Share your first event"** (quality time tab) and **"Post your first request"** (emissions tab) — are wired up to open creation modals. The loans/money-saved case was left out: it has no `onTap` handler, so `NextStepCard` hides the chevron icon (line 59 of `next_step_card.dart`) and the card looks visually inert. This matches the screenshot exactly.

**Problem 2 — Nudges keep showing after the user has already engaged.**
The "listed" stage is determined by *community-wide* state (≥1 item, 0 *completed* transactions). That means a user who has already expressed interest in a loan, RSVP'd to an experience, or filed a request — but whose action hasn't yet *completed* — still gets nagged with "Start your first loan / event / request." The nudge feels stale and condescending: "I literally just did this." All three nudges (loans, experiences, requests) should be suppressed for any user who already has an in-flight activity of that type.

The issue author expects: (a) the loans card to behave like its siblings — tapping it kicks off the action — and (b) each card to disappear once the user is no longer the target audience for it.

## Context from Comments

There are no comments on this issue. Direction has to come from the code
itself and product intent. Worth flagging: the issue author is the project
owner, so the feature request is authoritative.

## Codebase Findings

### Problem 1 (inert card)

- **The bug site:** `app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart:393-397` — the `default` branch of `_buildListedNextStep` constructs a `NextStepCard` without an `onTap`. The two other branches (lines 382-386 and 388-392) pass `onTap: _openAddEventModal` and `onTap: _openAddRequestModal` respectively.
- **Why the chevron is missing:** `app/lib/presentation/widgets/impact/community/next_step_card.dart:59` — `if (onTap != null) ... arrow icon`. No callback → no chevron → looks like a static panel.
- **Sibling handlers** already in the same file:
  - `_openAddEventModal` (line 368): `await ExperienceCreationModal.show(context);`
  - `_openAddRequestModal` (line 372): `await RequestCreationModal.show(context, communityId: widget.communityId);`
  - `_openAddItemModal` (line 351): `await CreateGearModal.show(context);`
- **Loans cannot be created from a modal.** Unlike events and requests, a loan is initiated by an *interested borrower* against an *existing piece of gear*. There is no `LoanCreationModal`, and there shouldn't be — the loan flow lives on the gear detail screen via "express interest". This is the key asymmetry that makes this feature non-trivial.
- **How users find gear to borrow:** The Discover tab (home tab index 1) is where members browse community gear. Tab navigation goes through `ref.read(homeProvider.notifier).navigateToTab(1)` — see `app/lib/services/post_creation_service.dart:33` for the existing pattern.
- **Wording ambiguity:** the description says *"Share something with a neighbor today"*, which on its face sounds like *list your gear*. But the title says *"Start your first loan"* and the issue says *"initiate a loan"* — and the metric is **money saved**, which only accrues when something is **borrowed**. The intent is borrowing, not listing. The description copy is just imprecise.

### Problem 2 (stale nudges)

- **Stage semantics** (file header lines 50-55): the "listed" stage means the community has **≥1 item** but **0 completed transactions** — *community-wide*, not per-user. So a user who already expressed interest yesterday but whose loan hasn't completed will keep seeing "Start your first loan." Same logic for experiences and requests.
- **Per-user "active" data is already available via repositories:**
  - **Loans:** `transferRepository.listMyTransfers()` and `transferRepository.listReceivedTransfers()` (`app/lib/data/repositories/transfer_repository.dart:68, 87`). Filter to `TransferType.LOAN` and to non-terminal states. Per `proto/ripls/api/transfer.proto:32-37`, the non-terminal `TransferState` values are `INTEREST_EXPRESSED` (1), `RECIPIENT_SELECTED` (2), `ACTIVE` (3). `COMPLETED` (4) and `CANCELLED` (5) are terminal.
  - **Experiences:** `experienceRepository.listMyExperiences()` (`app/lib/data/repositories/experience_repository.dart:102`). Returns experiences the current user is hosting or attending. Filter client-side to upcoming (not yet completed).
  - **Requests:** `requestRepository.listMyRequests()` (`app/lib/data/repositories/request_repository.dart:60`). Filter client-side to non-fulfilled, non-cancelled.
- **Repositories already cache these lists** with namespace isolation, so the screen can read them on every build without hammering the network.
- **Per-MVVM rules** (`docs/client/architecture.md` §1): the screen must not call repositories directly. The gating logic belongs in a viewmodel.
- **Existing viewmodel for this screen:** `communityImpactProvider` (`app/lib/presentation/viewmodels/community_impact_view_model.dart:316`) — an `AsyncNotifierProvider.autoDispose.family<...>` keyed by `communityId`. It only exposes community-wide metrics, no per-user activity flags.
- **`listMyTransfers` only takes a single `state` filter** (line 68-81). To check "any in-flight loan" we'd either issue 3 calls (one per non-terminal state), pass `state: null` and filter client-side, or add a server-side "non-terminal" filter. The cheap option is `state: null` + client-side filter — one cached request, three boolean checks.

## Potential Solutions

### Problem 1: where the loans card should send the user

#### Option 1A: Navigate to the Discover tab *(recommended)*
- **Approach:** Wire `onTap` to `ref.read(homeProvider.notifier).navigateToTab(1)`. This drops the user into the gear browse view for the selected community, where they can find an item and tap it to start the loan flow via the existing "express interest" path.
- **Pros:** Matches how loans actually get started in the app (no shortcut exists). Reuses an existing, tested navigation primitive (`navigateToTab`). Zero new screens, zero new flows. Consistent with the spirit of the sibling cards (each takes the user to the place where the next action happens).
- **Cons:** It's two taps to a started loan (browse → pick gear → express interest), not one. The user lands on a generic browse tab rather than directly on a loan-creation flow, which is a slightly weaker call-to-action than the events/requests modals offer.
- **Effort:** Small.

#### Option 1B: Open a "find something to borrow" gear-picker modal
- **Approach:** Build a new modal that lists the community's borrowable gear, lets the user pick one, then routes them to that gear's detail screen with the loan-interest sheet pre-opened.
- **Pros:** Most direct mapping of "start a loan" → loan started.
- **Cons:** New widget, new viewmodel, duplicates the gear-list rendering that already exists on Discover. Higher maintenance burden. Diverges from every other "start a loan" entry point in the app.
- **Effort:** Medium.

#### Option 1C: Reword to "list your gear" and reuse `_openAddItemModal`
- **Approach:** Change copy to *"List something to share"*, wire to `_openAddItemModal`.
- **Pros:** Trivial wiring.
- **Cons:** **Misreads the issue.** Listing your own gear is not initiating a loan. Also semantically wrong: by the time this card shows, the community already has ≥1 listed item. Doesn't move the money-saved metric.
- **Effort:** Small.

### Problem 2: how to gate visibility on per-user activity

#### Option 2A: Extend `communityImpactProvider` with per-user activity flags *(recommended)*
- **Approach:** Add three booleans (`userHasActiveLoan`, `userHasActiveExperience`, `userHasActiveRequest`) to `CommunityImpactData`. In `_loadImpactData`, after fetching metrics, fetch the user's transfers/experiences/requests in parallel (`Future.wait`) via the existing repositories and derive the booleans. The screen reads them off the same `AsyncValue` it already watches.
- **Pros:** Single source of truth for the impact screen. No second `AsyncValue` for the screen to juggle. Loads in parallel with the metrics call so no extra latency. Repository caching means subsequent rebuilds are free. Stays inside the established MVVM pattern (gating is computed in the viewmodel; the widget only reads booleans). Refresh paths already exist (`refresh()`, `refreshMetrics()`).
- **Cons:** Adds three repository calls to a viewmodel that previously only called one. If any of those calls fails the whole impact screen would hard-fail unless we explicitly tolerate per-user fetch errors (recommended fallback: treat fetch failure as "no active activity" so the nudge still shows — better to over-nudge than to break the screen).
- **Effort:** Small-to-medium.

#### Option 2B: Create a new `userActivityStatusProvider`
- **Approach:** Build a separate `AsyncNotifierProvider.autoDispose` that exposes only the three booleans. The screen watches both `communityImpactProvider` and `userActivityStatusProvider` and gates accordingly.
- **Pros:** Clean separation — impact data and per-user activity stay decoupled.
- **Cons:** Two `AsyncValue.when()` blocks in the screen for one decision. More plumbing. The "decoupling" benefit is theoretical — no other screen needs these booleans today.
- **Effort:** Medium.

#### Option 2C: Watch the lists directly in the screen build method
- **Approach:** Have the screen call `ref.watch(transferRepository...)` etc. from build.
- **Pros:** Trivial.
- **Cons:** **Violates `docs/client/architecture.md` §1** — screens must not access repositories directly. Rejected.
- **Effort:** Trivial but non-compliant.

## Recommendation

**Option 1A + Option 2A.**

For Problem 1, **Option 1A** is the smallest change that does what the issue asks, reuses existing infrastructure, and matches the actual loan-start flow in the app.

For Problem 2, **Option 2A** keeps gating logic in the viewmodel where it belongs, lets the screen stay simple (one `AsyncValue` to read), and parallelizes the extra fetches with the existing metrics call. It also pairs naturally with the existing `refresh()` paths so the cards self-update after a user takes action.

**Scope decision: gating is global, not community-scoped.** The repository methods used (`listMyTransfers`, `listMyExperiences`, `listMyRequests`) return the user's activity across all communities. Filtering down to the current community would require extra plumbing (joining transfers to gear → community, etc.) and the user-experience win is minimal: if a user already has *any* in-flight loan anywhere, they understand the loan flow and don't need the nudge. Document this tradeoff so a future reviewer can revisit if product wants per-community gating later.

## Open Questions

- **Should the description copy be updated as part of this fix?** — Recommendation: **yes, lightly**. Change `communityMetricsStartFirstLoanDescription` to *"Browse what your neighbors are sharing and borrow your first item."* It correctly tells the user what tapping the card will do. Update both `app_en.arb` and `app_es.arb`.
- **Should the Discover tab pre-filter to the current community?** — Recommendation: **no, leave as-is for this fix**. Discover already respects the selected community via `selectedCommunityProvider`. If product later wants a deeper link, file a follow-up.
- **Is "active" community-scoped or global for the suppression check?** — Recommendation: **global**. See the Recommendation section. Cheaper, simpler, and matches the user mental model that "I already understand this flow." Revisit if product disagrees.
- **What states count as "active" for each entity?**
  - **Loan:** any `Transfer` with `transferType == LOAN` and `state ∈ {INTEREST_EXPRESSED, RECIPIENT_SELECTED, ACTIVE}`. Both `listMyTransfers` and `listReceivedTransfers` (so a user who is the borrower OR the lender of an in-flight loan is suppressed).
  - **Experience:** any `Experience` returned by `listMyExperiences` whose start time is in the future (or whose state isn't COMPLETED/CANCELLED — confirm with the experience proto when implementing).
  - **Request:** any `Request` returned by `listMyRequests` whose state is not FULFILLED and not CANCELLED.
- **What if the per-user fetch fails?** — Recommendation: **fail open (still show the nudge)**. Wrap each per-user fetch in a `try`/`catch` inside `_loadImpactData`, log a warning, and treat failure as "no active activity." Better to over-nudge once than to crash the impact screen because the transfer service hiccupped.
- **Analytics?** — Recommendation: **add a simple tap event** if there's an existing analytics pattern for `NextStepCard` taps. If not, skip — don't introduce a new analytics surface for this fix.

## Implementation Plan

### Phase 1: Wire the loans onTap and update copy
**Goal:** Tapping "Start your first loan" navigates the user to the Discover tab so they can browse gear and borrow. Solves Problem 1 in isolation — independently shippable.
**Tasks:**
- [ ] Add a private handler `_openBrowseGear()` in `community_metrics_screen.dart` that calls `ref.read(homeProvider.notifier).navigateToTab(1)`. Place it next to the other `_openAdd*Modal` handlers (around line 368-374) for consistency.
- [ ] In `_buildListedNextStep` default branch (line 393-397), pass `onTap: _openBrowseGear`. This restores the chevron and makes the card tappable.
- [ ] Update `communityMetricsStartFirstLoanDescription` in `app/lib/l10n/app_en.arb` to *"Browse what your neighbors are sharing and borrow your first item."*
- [ ] Update the same key in `app/lib/l10n/app_es.arb` with a real Spanish translation (e.g. *"Explora lo que comparten tus vecinos y pide prestado tu primer artículo."*). Per `docs/client/i18n.md`, **every new or changed string must exist in both locales** — leaving it stale fails the i18n lint and produces a compile error because `nullable-getter: false`.
- [ ] Run `npm run generate` to regenerate localization sources. The npm script is the canonical entry point.
- [ ] Run `npm run test:app`.
**Validates:** Visual check on the impact screen (chevron now appears, card responds to tap, lands on Discover) and that the localization codegen still passes.

### Phase 2: Add per-user activity flags to the impact viewmodel
**Goal:** Make `CommunityImpactData` carry the three user-activity booleans needed to gate the nudges. No UI change yet — viewmodel work only.
**Tasks:**
- [ ] In `app/lib/presentation/viewmodels/community_impact_view_model.dart`, extend the `CommunityImpactData` Freezed class with three fields: `@Default(false) bool userHasActiveLoan`, `@Default(false) bool userHasActiveExperience`, `@Default(false) bool userHasActiveRequest`. Defaults preserve current behavior on partial-load failures.
- [ ] Add provider lookups at the top of `_loadImpactData` for `transferRepositoryProvider`, `experienceRepositoryProvider`, `requestRepositoryProvider`. Use the existing `services/providers.dart` patterns.
- [ ] Inside `_loadImpactData`, after the existing `getMetricsResponse` call, fetch the three lists in parallel via `Future.wait`:
  - `transferRepository.listMyTransfers(transferType: TransferType.TRANSFER_TYPE_LOAN)` AND `listReceivedTransfers(transferType: TRANSFER_TYPE_LOAN)` — combine results.
  - `experienceRepository.listMyExperiences()`.
  - `requestRepository.listMyRequests()`.
- [ ] Wrap each fetch in its own try/catch that logs a warning and falls back to an empty list. Failure of one of these must not break the metrics screen — fail-open per the Open Question above.
- [ ] Derive the booleans:
  - `userHasActiveLoan = combinedLoanTransfers.any((t) => t.state == INTEREST_EXPRESSED || t.state == RECIPIENT_SELECTED || t.state == ACTIVE)`. Define a small helper `bool _isActiveTransferState(TransferState s)` for readability.
  - `userHasActiveExperience = myExperiences.any((e) => /* upcoming or in-process */)`. Inspect `experience.proto` when implementing — likely `state != COMPLETED && state != CANCELLED`, possibly also `startTime > now()`.
  - `userHasActiveRequest = myRequests.any((r) => /* state not in {FULFILLED, CANCELLED} */)`. Inspect `request.proto` for the actual state enum.
- [ ] Pass the booleans into the `CommunityImpactData` constructor at the bottom of `_loadImpactData`.
- [ ] Add unit tests in `app/test/presentation/viewmodels/community_impact_view_model_test.dart` (create if missing) using `ProviderContainer` with fake repositories. Cover:
  - All three booleans true.
  - All three booleans false.
  - Each individual boolean true in isolation.
  - One repository throws → corresponding boolean stays `false` and the rest of the data still loads.
- [ ] Run `npm run generate` (Freezed regeneration) and `npm run test:app`.
**Validates:** Viewmodel correctly reports per-user activity status; failures in one repository don't cascade.

### Phase 3: Gate the nudge cards in the screen
**Goal:** Hide each `_buildListedNextStep` branch when the corresponding user activity flag is true. Hide the matching `_buildBornCard` branch too if the new-stage card is showing for a user who has already started.
**Tasks:**
- [ ] In `community_metrics_screen.dart`, change `_buildListedNextStep` to return `Widget?` (or `SizedBox.shrink()`) when the relevant flag is true. Read the flags off the same `CommunityImpactData` the screen already has access to. The switch becomes:
  - `QUALITY_TIME` branch: return shrink if `data.userHasActiveExperience`.
  - `EMISSIONS` branch: return shrink if `data.userHasActiveRequest`.
  - default (loans): return shrink if `data.userHasActiveLoan`.
- [ ] At each call site of `_buildListedNextStep`, ensure the section header (the `communityMetricsGetStarted` label above the cards) also disappears if there is nothing to show. If both the listed-next-step card and the other "get started" cards are hidden, suppress the whole section.
- [ ] Audit `_buildBornCard` (`community_metrics_screen.dart:402-428`) for the same problem. The "born" stage shows a `WelcomeCard` with a CTA per metric. Apply the same gating: if the user already has an in-flight loan/experience/request, suppress the matching welcome card. Use `SizedBox.shrink()` so layout stays stable.
- [ ] Add widget tests in `app/test/presentation/screens/impact_metrics/community_metrics_screen_test.dart` (create or extend). Use `ProviderScope` overrides to inject `communityImpactProvider` with hand-crafted `CommunityImpactData` in each combination:
  - All flags false → all three cards visible (and tappable for the loans one — same assertion as Phase 1's test, fold them together).
  - `userHasActiveLoan = true`, money-saved tab selected → loans card is absent.
  - `userHasActiveExperience = true`, quality-time tab selected → experience card is absent.
  - `userHasActiveRequest = true`, emissions tab selected → request card is absent.
- [ ] Run `npm run test:app`.
**Validates:** End-to-end suppression behavior, regression coverage for all three nudges in both stages.

### Phase 4: Cache invalidation paths
**Goal:** When the user takes one of the actions that should suppress a nudge, the impact screen updates immediately on next view.
**Tasks:**
- [ ] Verify (don't add new) cache invalidation: `transferRepository.expressInterest` already invalidates `received:*` (line 175). `experienceRepository.rsvp` and `requestRepository.submitRequest` should similarly invalidate the "my" caches. Trace each mutation path and confirm.
- [ ] Confirm `communityImpactProvider` re-fetches when the user navigates back to the metrics screen. Because it's `autoDispose.family`, it rebuilds on each fresh watch — but only re-runs `_loadImpactData` if the cached repository data changed. The repository invalidations from the bullet above are what make this work end-to-end.
- [ ] If any mutation path *doesn't* invalidate the relevant repository cache, file a sub-issue rather than fixing it in this PR — that's a pre-existing bug, not a regression introduced here.
- [ ] Manually verify the flow on a dev build: express interest in a loan → return to metrics screen → loans nudge should be gone.
**Validates:** Real user flow: take an action, the nudge disappears the next time the screen is viewed.

## Related Issues
- None identified. The two simulator bugs filed earlier today (#1061, #1062) are unrelated.
