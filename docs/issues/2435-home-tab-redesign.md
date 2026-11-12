# Issue #2435: Redesign Home tab (portfolio inbox) to v4.1 build spec

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2435
**Filed:** 2026-06-11 | **Last activity:** 2026-06-11
**Labels:** none | **Assignees:** tescobar
**Context docs:** [client/inbox.md](../client/inbox.md), [client/architecture.md](../client/architecture.md), [client/conventions.md](../client/conventions.md), [server/portfolio.md](../server/portfolio.md), [proto_conventions.md](../proto_conventions.md), [client/chat.md](../client/chat.md), [realtime_updates.md](../realtime_updates.md), [reviews/accessibility_review_prompt.md](../reviews/accessibility_review_prompt.md). Reference mock: [docs/cowork/App Design/inbox-home-desk-v4.1.html](../cowork/App%20Design/inbox-home-desk-v4.1.html).

> **Implementation status (2026-06-11):** Phases 1–6 implemented in one pass on the
> `inbox` branch — nav rename + tooltip, `GetHomeView` proto/server + tests, the
> sectioned Home screen with the three-counter split and empty states, gear
> see-all with filter chips, search scope chips + Yours-pinned grouping + Home
> entry point, and the Messages list with the decision-waiting pill. Verified:
> `go vet`/server tests, full Flutter suite (4,769 tests), `flutter analyze`,
> and all `npm run lint` gates (the `lint:context-map` failure pre-exists on
> untracked `docs/cowork/tmp/` files). **Descoped to follow-ups:** pure DMs
> (#2436), the manual loan-nudge chip with 24h throttle (#2437), and in-place
> ask-claim confirmation (no confirm-helper RPC exists — helpers are confirmed
> at fulfillment; the card's confirm action navigates to the request screen).
> The week-view "view thread"/initial-tab deep links open the content screen's
> default tab (Discuss surfaces inline).

> **Scope note — this is epic-sized.** The mock is 8 frames spanning a re-architected home screen, two search frames, a new Messages list, two empty states, and a gear see-all, plus server-side assembly changes and at least one new concept (decision queue) that doesn't exist today. It should ship as a tracked **epic with sub-issues per phase** (see Implementation Plan), not a single PR. This document is the umbrella plan; each phase below is a candidate sub-issue.

---

## Problem Statement

Replace the current second-tab experience — a flat, chat-style portfolio inbox split across four filter pills (Needs Attention / Upcoming / Your Stuff / Done) — with an **action-first, sectioned "Home" screen** per the v4.1 build spec. The tab is renamed **Home** (house icon) and reorganized into ordered sections:

1. **Needs you** — a strict decision queue (accept / decline / confirm only), resolvable in place.
2. **Up next** — an events agenda with a list ↔ week toggle; week view also shows dated gear obligations.
3. **Your asks** — the user's open requests with fulfillment progress.
4. **Gear** — owned/borrowed gear with loan-state pills, capped at 3 + "See all ›".
5. **Recent activity** footer — an unseen-activity entry point with a "N new" pill.

Search and Messages move to **header icons** on the root. Search reuses the feed's global search component (now with "Yours"-pinned / "From your communities" grouping). Messages becomes a unified conversation list.

The defining behavioral shift is **three independent counters that never sum**: the Home tab badge counts only open *Needs you* decisions; the chat icon counts conversations with unreads; the "N new" pill counts activity since the Recent-activity screen was last opened. Today there is a single `inboxUnreadCountProvider` (unread *watched items*) driving both the nav badge and the app-icon badge — the new model splits these apart.

### Contradictions / ambiguities to resolve up front

- **"Needs you" ≠ today's "Needs Attention."** The current Needs-Attention pill is the catch-all engagement view (unread chats, active loans, RSVPs, offers, watched items). The new "Needs you" is *only* items requiring an explicit accept/decline/confirm. This is a narrower, new server-side classification, not a rename.
- **Messages frame shows pure DMs ("Direct message" — Betty, Maria), but the data model has no DM conversation type.** Every conversation today is object-attached (transfer / request / experience / gear / community); there is no `userId` topic. Frame 5 as drawn is not buildable without a proto/data change — **descoped for v1**, tracked in [#2436](https://github.com/Type2Fun-Foundation/ripls/issues/2436). See Resolved Decisions.
- **"decision waiting in Needs you" pill on a message thread** requires a link from a conversation to its pending decision. No such link exists today.
- The mock references a calendar/week toggle "restored from v3." A `week_navigator.dart` / `day_selector.dart` precedent exists under `app/lib/presentation/widgets/portfolio/`, so this is a revival, not a greenfield build.

---

## Context from Comments

**The issue has no comments.** It was filed from the mock with no follow-up discussion. Several product decisions are therefore unresolved and are captured in Open Questions below — the issue should get a round of clarification (especially on DMs and the server data-shape decision) before Phase 1 implementation begins. The mock's "Engineering notes" block is the authoritative spec for behavior the static frames can't show, and is treated as binding here.

---

## Current-State Map (what exists today)

**Navigation & shell** — [app/lib/presentation/screens/home/home_screen.dart](../../app/lib/presentation/screens/home/home_screen.dart)
- 3 visual nav tabs: 0 = Feed (home icon), 1 = Inbox (chat-bubble icon), 2 = Workshop ("ripls" icon), plus a floating Create FAB. An orphaned Discover sits unused in the IndexedStack.
- `IndexedStack` keyed off `homeProvider.selectedIndex`; `_visualToStackIndex()` maps visual→stack. The Inbox tab's badge reads `inboxUnreadCountProvider`.

**Tab 2 today** — [portfolio_inbox_screen.dart](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart) + [portfolio_inbox_view_model.dart](../../app/lib/presentation/viewmodels/portfolio_inbox_view_model.dart)
- Flat `DailyItem` list with four independently-queried filter pills, floating search pill, swipe-to-unwatch. Driven by `GetPortfolioInboxFeed`. See [client/inbox.md](../client/inbox.md).

**Server portfolio assembly** — [server/services/portfolio/](../../server/services/portfolio/), [proto/ripls/api/portfolio.proto](../../proto/ripls/api/portfolio.proto)
- Two relevant shapes already exist:
  - `GetPortfolioInboxFeed` — flat feed (current UI).
  - **`GetPortfolioInboxView`** (deprecated but live, fully populated) — a *sectioned* response: `WeeklyMetrics`, `people`, **actions-today**, **needs-attention**, **coming-up** (Tomorrow / Later This Week / Next Week), shared-inventory. This is structurally the closest existing thing to v4.1's sections. See [server/portfolio.md](../server/portfolio.md).

**Search** — [search_view_model.dart](../../app/lib/presentation/viewmodels/search_view_model.dart), [search_nearby_pill.dart](../../app/lib/presentation/widgets/search/search_nearby_pill.dart), [discover_screen.dart](../../app/lib/presentation/screens/discover/discover_screen.dart), [proto/ripls/api/search_service.proto](../../proto/ripls/api/search_service.proto)
- Rich component: semantic `Search` RPC across communities, distance per row, category filters, sort, **quick-search chips**, **communities carousel**, **interests row**, recent searches. Results are a **single flat list** (no "Yours"/"communities" grouping, no scope chips). Only reachable from the feed's pill — **no Home entry point**.

**Chat / Messages** — [chat_service.dart](../../app/lib/services/chat_service.dart), [chat_repository.dart](../../app/lib/data/repositories/chat_repository.dart), [unread_count_view_model.dart](../../app/lib/presentation/viewmodels/unread_count_view_model.dart)
- `listConversations()` returns `ConversationItem` carrying `topic`, `unreadCount`, `lastMessageText`, `lastMessageSender`, `isItemDone`; `ConversationContext` carries `topicTitle` / `topicImageMediaId`. **No standalone Messages list screen exists** (conversations open embedded in content screens). **No DM topic type. No "decision waiting" link.** `unreadCountProvider` already tracks per-conversation unread counts (a count of conversations with unreads is derivable).

**One-time tooltip infra** — [tutorial_view_model.dart](../../app/lib/presentation/viewmodels/tutorial_view_model.dart) + [tutorial_state_repository.dart](../../app/lib/data/repositories/tutorial_state_repository.dart)
- `TutorialNotifier` with a persisted `seenTutorials` map and `checkAndShowTutorial(key)` — ready to drive the one-time "Your inbox is now Home" tooltip.

---

## Potential Solutions

The central architectural decision is **how to source the new sectioned data**. The UI rebuild and the counter split follow from that choice. Three options:

### Option A: New dedicated `GetHomeView` RPC, reusing existing assembly internals
- **Approach:** Add a new RPC (`GetHomeView` / `GetHomeViewResponse`) with first-class, flattened sections matching the mock: `needs_you` (decision items), `up_next` (events + dated gear obligations), `your_asks`, `gear`, `recent_activity_unseen_count`, and the three counter fields. Implement server-side by reusing `fetchAll()` and the existing per-item assemblers (`assembleTransfer/Experience/Request`), but with a **new decision classifier** for `needs_you` and the mock's lifecycle/ordering rules. Client gets a new `HomeViewNotifier` + sectioned screen.
- **Pros:** Honors the proto convention "never reuse RPC request/response types across RPCs" and the API-shape-per-RPC rule. Clean contract that can evolve independently. Keeps the legacy feed RPC intact for rollback/A-B. Counter semantics are explicit server fields, not client guesses.
- **Cons:** Largest net-new surface (proto + server + client + tests). Some assembly logic overlaps the existing view RPC.
- **Effort:** Large.

### Option B: Extend/repurpose the existing `GetPortfolioInboxView` (sectioned) RPC
- **Approach:** `GetPortfolioInboxView` already returns needs-attention / coming-up / shared-inventory sections. Tighten "needs-attention" into the strict decision queue, add the dated-gear-obligation entries to coming-up, add counter fields, and rebuild the client on this response.
- **Pros:** Reuses the most existing server code; the sectioned shape already exists and is tested.
- **Cons:** The RPC is **already marked deprecated** in favor of `GetPortfolioInboxWeek`; reviving + reshaping a deprecated contract is muddy. Its response type is laden with fields the new design doesn't use (WeeklyMetrics breakdowns, people row) — reshaping risks the "no field reuse / dedicated types" convention and could break other consumers. Counter semantics get bolted onto a legacy shape.
- **Effort:** Medium-Large.

### Option C: Assemble sections client-side from existing RPCs
- **Approach:** Call `GetPortfolioInboxFeed` + `listConversations` + search/experience data and derive the five sections + counters in a client view model. No server change.
- **Pros:** No proto/server work; fastest to a clickable prototype.
- **Cons:** Pushes domain classification (what's a "decision," lifecycle exits, dated obligations, ordering) into the client, violating the MVVM/data-layer boundary and duplicating logic the server already owns. The "decision" classification and "fulfilled ask stays until its date passes → Recent activity" rules need cross-entity joins that belong server-side. Counters become fragile client derivations. Hard to keep consistent with realtime updates.
- **Effort:** Medium now, high long-term maintenance.

---

## Recommendation

**Option A — a new `GetHomeView` RPC building on existing assembly internals.** The new screen is a genuinely different contract (decision queue + dated obligations + three explicit counters + lifecycle-graduation rules) that the project's proto conventions say should get its own dedicated request/response types rather than overloading the deprecated sectioned RPC or the flat feed RPC. Reusing `fetchAll()` and the per-item assemblers keeps server duplication low, and keeping the legacy feed RPC live lets us ship behind a flag and roll back cleanly.

Concretely:
- **Server:** new `GetHomeView` in `server/services/portfolio/`, reusing `fetchAll` + assemblers, adding (1) a decision classifier for `needs_you`, (2) dated-gear-obligation projection for Up-next week view, (3) the three counter computations, (4) lifecycle/ordering rules from the mock's Engineering notes.
- **Client:** new `HomeViewNotifier` + sectioned `HomeScreen` body; split the nav badge to a new `homeDecisionCountProvider`; reuse `unreadCountProvider` for the chat-icon count; add a Recent-activity "last opened" timestamp for the "N new" pill.
- **Search & Messages:** treat as adjacent workstreams (Phases 4–5) that reuse existing components with additive changes, not rewrites.

**Descoped for v1** (confirmed, see Resolved Decisions): pure person-to-person DMs (object threads only — DM topic type tracked in [#2436](https://github.com/Type2Fun-Foundation/ripls/issues/2436)), provenance-chip tap behavior (already out of scope per the issue), and the A/B test variants.

---

## Resolved Decisions (formerly Open Questions)

All questions resolved 2026-06-11 — Q1 descoped to its own issue; Q2–Q7 adopt the recommendations as written.

- **Q1 — Pure DMs in Messages (frame 5). ✅ DECIDED: out of scope for v1.** The mock shows "Direct message" rows (Betty, Maria) but no DM conversation type exists; all conversations are object-attached. v1 builds the Messages list over existing object-attached `listConversations()` data (which already supports the "re: …" context line + thumbnail overlay). The DM topic type is tracked separately in **[#2436](https://github.com/Type2Fun-Foundation/ripls/issues/2436)** and does not gate this redesign.
- **Q2 — Data-shape decision. ✅ DECIDED: Option A** — new `GetHomeView` RPC, per the Recommendation section. This sets the contract everything else builds on.
- **Q3 — "Needs you" decision classification rules. ✅ DECIDED:** owner-side incoming borrow request (`INTEREST_EXPRESSED` awaiting accept/decline), owner-side request offer awaiting confirm, and incoming event/role confirmations qualify. Explicitly *exclude* replies, comments, FYIs, and items the user already acted on. Encode as a single server-side predicate so the badge count and the queue never disagree.
- **Q4 — "Recent activity" surface. ✅ DECIDED:** v1 ships the footer + "N new" pill backed by an unseen-count, linking to a simple reverse-chronological activity list assembled from existing portfolio "actions" data (`assembleActionsToday` generalized). A richer screen is a fast-follow.
- **Q5 — Up-next week-view toggle persistence. ✅ DECIDED:** persist via local prefs (`SharedPreferences`, as `TutorialStateRepository` does) keyed per user; default list. No server round-trip.
- **Q6 — App-icon badge after the counter split. ✅ DECIDED:** open *Needs you* decisions (the actionable count) drive the OS app-icon badge, matching the in-app Home tab badge.
- **Q7 — Workshop tab fate. ✅ DECIDED:** keep Workshop as-is; only tab 1 (Feed icon→compass) and tab 2 (→house, "Home") change. No structural nav change beyond icons/labels + the tooltip.

---

## Implementation Plan

Each phase is a shippable increment and a candidate sub-issue. Phases 1–3 are the core; 4–6 are adjacent surfaces. Build behind a feature flag so the legacy four-pill inbox remains the fallback until the new Home is complete.

### Phase 1: Nav rename, icons, and one-time tooltip
**Goal:** The tab becomes "Home" (house icon) and Feed gets the compass icon, with the migration tooltip — no content change yet (still renders the existing inbox body behind the flag).
**Tasks:**
- [ ] Update [home_screen.dart](../../app/lib/presentation/screens/home/home_screen.dart): tab 1 icon → compass + label "Feed"; tab 2 icon → house + label "Home". Reuse existing theme tokens (AppColors / AppTheme); no per-tab color constants.
- [ ] Wire a one-time "Your inbox is now Home" tooltip on tab 2 via `TutorialNotifier.checkAndShowTutorial('home_rename')`.
- [ ] Workshop/FAB unchanged (Q7 — decided).
**Validates:** Nav chrome + tooltip land safely with zero data risk; establishes the flag.

### Phase 2: `GetHomeView` contract + server assembly
**Goal:** A new RPC returns the five sections + three counters with correct classification, ordering, and lifecycle rules.
**Tasks:**
- [ ] Add `GetHomeView` RPC with dedicated request/response + section/item messages to [portfolio.proto](../../proto/ripls/api/portfolio.proto) (flattened, no cross-RPC type reuse; follow [proto_conventions.md](../proto_conventions.md)). Add proto comments on every RPC/message/field (api_design rubric §6). `npm run generate`.
- [ ] **Bound every repeated field.** Root sections are spec-capped (Needs-you top 3, Up-next max 4, Gear 3) — cap them server-side. The unbounded lists (Gear see-all ~16+, Recent activity) must paginate or carry an explicit server-enforced limit + `has_more` rather than an unbounded `repeated` (api_design rubric §2.4/§2.5).
- [ ] Implement handler in `server/services/portfolio/` reusing `fetchAll()` + the existing per-item assemblers; add the decision classifier (Q3), dated-gear-obligation projection (Up-next week), the three counter computations, and the lifecycle/ordering rules (oldest-first decisions, top-3 caps, "fulfilled ask stays until date passes," loan leaves on return-confirm, event leaves on end).
- [ ] **No new N+1.** Compute the three counters and the decision classification as a **pure function over the already-fetched maps** (mirror the existing pure `assembleInboxView` — all storage I/O in `fetchAll`/`fetchExtras` before assembly). No per-item queries in the classifier or counter loops (performance rubric; [server/conventions.md](../server/conventions.md) SQL-efficiency).
- [ ] "No empty section header" is data-driven: zero-item sections omitted server-side.
- [ ] Structured `slog` on the handler boundary; map storage errors to `CodeInternal`, not raw errors ([server/conventions.md](../server/conventions.md) fail-fast + observability).
- [ ] Go unit tests for the classifier, ordering, lifecycle exits, and each counter (fail-fast, table-driven per [server/conventions.md](../server/conventions.md)).
**Validates:** The hard domain logic (what's a decision, what graduates, what each counter equals) is correct and tested before any UI depends on it.

### Phase 3: Home screen body + counter split (frames 1, 2, 7 + empty state 6)
**Goal:** The sectioned Home renders from `GetHomeView`, with the list↔week toggle, in-place decision resolution, and the new badges.
**Tasks:**
- [ ] `HomeViewNotifier` + repository method (cache key `portfolio:home_view` in the `portfolio` namespace; invalidate alongside `inbox_feed` on mutation) calling `GetHomeView`; optimistic in-place decision resolve (card animates out, badge decrements; "Not now" dismisses without re-queue) per the [optimistic-update pattern](../client/architecture.md#optimistic-updates). No manual cache maps in the viewmodel.
- [ ] Build the section widgets — each its own widget class to stay under the **1,000-line Dart file cap** (one screen file orchestrating small section widgets): Needs-you cards, Up-next list + week strip/agenda (revive [week_navigator.dart](../../app/lib/presentation/widgets/portfolio/week_navigator.dart) / [day_selector.dart](../../app/lib/presentation/widgets/portfolio/day_selector.dart)), Your-asks rows with progress + conditional Share chip, Gear rows (capped at 3 + "See all ›") with correct pills, Recent-activity footer with "N new" pill.
- [ ] **All media thumbnails (gear, event images, avatars) via `CachedMediaImage`/`mediaObjectProvider`** — never `Image.network` for user media ([client/conventions.md](../client/conventions.md) Media Display).
- [ ] **User-facing copy uses "Event", never "Experience"** (l10n ARB strings + semantic labels), per [client/conventions.md](../client/conventions.md). No phase-marker comments in Dart (`lint:ephemeral-comments` fails the build) — keep rollout context in the commit/PR, not code.
- [ ] Toggle persistence per user (Q5). Empty states: new-user hero (frame 6) and all-caught-up calm card (frame 7).
- [ ] Split badges: new `homeDecisionCountProvider` for the tab badge (hidden at zero); the OS app-icon badge also mirrors the decision count (Q6 — decided).
- [ ] Realtime: in-place refresh on mutation per [realtime_updates.md](../realtime_updates.md); decision resolve marks same-request messages read.
- [ ] Accessibility pass per [reviews/accessibility_review_prompt.md](../reviews/accessibility_review_prompt.md) (semantic labels on cards/chips/toggle, touch targets). Widget/viewmodel tests incl. disposal-safety.
**Validates:** The core experience (frames 1–2, 6–7) and the counter model end-to-end. This is the flag-flip candidate.

### Phase 4: Gear see-all with inline filters (frame 8)
**Goal:** "See all ›" opens the full gear list with state filter chips and live counts; root stays capped at 3.
**Tasks:**
- [ ] Gear see-all screen with filter chips (All / Out / Borrowing / Listed / With you) + live counts, ordering per spec, nudge throttle (one per loan per 24h), "Set a date" → loan return-date picker.
- [ ] Paginate or hard-cap the see-all list (it is the unbounded one — the mock shows "6 more ↓"); don't return an unbounded `repeated`.
- [ ] Tests for filter counts and ordering.
**Validates:** Heavy-user gear management without search.

### Phase 5: Search grouping + Home entry point (frames 3, 4)
**Goal:** The header search icon opens the shared search with default grouped results.
**Tasks:**
- [ ] Add a Home header search-icon entry point into the existing search component (no fork — one component, two entry points).
- [ ] Add scope chips (Yours / Everything) and group default results "Yours" (pinned) then "From your communities" — post-process the flat `Search` response client-side (current user's items = Yours) unless server grouping is preferred. Quick-search chips clear active scope. Reuse existing communities/interests rows.
- [ ] Tests for grouping/scoping logic.
**Validates:** "where's my X" + "who has an X" answered in one query.

### Phase 6: Messages list (frame 5)
**Goal:** A unified conversation list reachable from the header chat icon, with the single-unread-state rule.
**Tasks:**
- [ ] New Messages list screen over `listConversations()`: "re: …" context line, object-thumbnail overlay, unread dot + darker preview, sorted by last message desc.
- [ ] "⚡ decision waiting in Needs you" pill on threads whose pending decision sits in Needs-you (requires the conversation→decision link from Phase 2); tap jumps to the card. One unread state per row, never two.
- [ ] Chat-icon header count = conversations with unreads (reuse `unreadCountProvider`).
- [ ] DMs are out of scope (Q1 — decided): object-attached threads only; the DM topic type lands later via [#2436](https://github.com/Type2Fun-Foundation/ripls/issues/2436).
**Validates:** Messages consolidation and the never-two-unread-states invariant.

---

## Related Issues
- **#2274** (3-day recurring request-followup push prompts) — touches request followups that may surface as "Your asks" / Needs-you items; keep the decision-classifier consistent.
- **#2020** — existing portfolio proto-deprecation TODO markers; the `GetPortfolioInboxView`/`GetPortfolioInboxWeek` deprecation context is relevant to the Option A vs B decision.
- **#2436** — first-class DM conversation type (filed; extends the Phase 6 Messages list post-v1).
- Search grouping (Phase 5) should be filed as its own sub-issue under this epic.

---

## Audit Notes

Audited against the documentation context map. Findings folded into the plan above:

- **API design (new `GetHomeView`):** Confirmed the Option-A direction complies with the dedicated-types / no-cross-RPC-reuse / flattened-message conventions. Added explicit requirements for (1) proto comments on every RPC/message/field, and (2) **bounding all repeated fields** — root sections are spec-capped, but the Gear see-all and Recent-activity lists must paginate or carry a server-enforced limit + `has_more` rather than an unbounded `repeated`.
- **Performance / SQL efficiency:** Added a hard requirement that the decision classifier and the three counters are a **pure function over the maps already loaded by `fetchAll`** (mirroring the existing pure `assembleInboxView`), so the new sections add no N+1 query paths.
- **Server error handling / observability:** Added structured-`slog` + `CodeInternal` error-mapping on the handler boundary.
- **Client conventions:** Added `CachedMediaImage` for all media thumbnails, the "Event" not "Experience" user-facing-copy rule, the no-phase-marker-comments lint caution, the 1,000-line Dart file cap (drives the one-screen-many-section-widgets decomposition), an explicit cache key (`portfolio:home_view`), and the no-manual-cache-maps rule.
- **Accessibility:** Phase 3 already carries an a11y pass per the review prompt (semantic labels on cards/chips/toggle, touch targets); reaffirmed.
- **No contradictions** with existing documented decisions were found; the plan keeps the legacy feed RPC live for rollback and treats Search/Messages as additive to existing components.

**Docs consulted:** [proto_conventions.md](../proto_conventions.md), [reviews/api_design_review_prompt.md](../reviews/api_design_review_prompt.md), [reviews/performance_review_prompt.md](../reviews/performance_review_prompt.md), [client/conventions.md](../client/conventions.md), [client/architecture.md](../client/architecture.md), [server/conventions.md](../server/conventions.md), [server/portfolio.md](../server/portfolio.md), [client/inbox.md](../client/inbox.md), [client/chat.md](../client/chat.md), [realtime_updates.md](../realtime_updates.md), [reviews/accessibility_review_prompt.md](../reviews/accessibility_review_prompt.md).
