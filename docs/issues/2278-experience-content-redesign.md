# Issue #2278: Redesign content views — new Experience (Event) content view with reusable components

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2278
**Filed:** 2026-06-02 | **Last activity:** 2026-06-02
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** [docs/client/content.md](../client/content.md), [docs/cowork/App Design/event-rsvp-v7.html](../cowork/App%20Design/event-rsvp-v7.html), [docs/cowork/App Design/request-help-v8.html](../cowork/App%20Design/request-help-v8.html), [docs/issues/2154-event-redesign.md](2154-event-redesign.md), [docs/issues/2175-request-content-view-redesign.md](2175-request-content-view-redesign.md)

> **Audit pass (2026-06-02):** Mapped the issue against the current source, the two prior redesign plans (#2154 / #2175 — both shipped), and the API protos the client consumes. Three findings drive this plan: (1) This is **not** an incremental chrome change like #2154/#2175 — it replaces the whole `Stack`-of-tab-panes layout with a scrollable-sheet-over-hero "edges" surface. (2) Roughly half the mockup's surfaces depend on **data the API does not expose today** (reach/silent-pass counts, a friendship ledger, social-tier sharing, personal loop-in invites) — those must be carved into backend follow-ups, not bundled here. (3) The "edges" / "you-gap" / contribution-graph model maps cleanly onto data that **already exists** (RSVPs, the shared Needs/Contributions system, `ExperienceState` lifecycle, `locationDistanceMeters`), so a compelling v1 is shippable client-only.

> **Compliance audit pass (2026-06-02) — reviewed against [`docs/client/{architecture,caching,i18n,accessibility,testing}.md`](../client/) and [`docs/server/{architecture,observability}.md`](../server/).** Corrections folded into Open Questions and the phases below. The eight that matter most: **(a) MVVM boundary** — the edges/you-gap **data assembly** (joining RSVPs with contributions, deriving status pills, building the updates feed) is **business logic** and must live in `ExperienceNotifier` / `ExperienceState`, exposed as typed view-data; the new `widgets/content/` widgets receive that view-data + callbacks and must not call repositories or join protos themselves (architecture.md §Key Architectural Rules). **(b) Async safety** — `experienceProvider` is `NotifierProvider.autoDispose.family`; every new async method needs an `if (!ref.mounted) return;` guard after each await before `state = ...`, and every async path needs a disposal-safety test (architecture.md §Async Safety, §Disposal Testing). **(c) Navigation** — the chat route (and any pushed detail screen) must use `NavigationHelpers.pushScreen()` + `SwipeToCloseMixin`, never raw `Navigator.push(MaterialPageRoute)` / `context.push()`; `npm run lint:nav` enforces it. **(d) i18n** — the `ContentLifecyclePhase` enum is resolved to copy in the **widget** via `context.l10n`, never in the viewmodel; the viewmodel exposes the enum, error state as `UserError? error` (i18n.md, errors/README.md). **(e) Accessibility** — glass top-bar icons → `IconAction` (mandatory `tooltip` + `semanticsLabel`); you-gap quick-picks / CTA toggles → `Toggle` with `selected:`; one-shot taps → `Tappable`; avatars via `CachedMediaImage(semanticsLabel:)`; all sheets via `showAccessibleModal`; hero-tint / sheet animations via `accessibleDuration(context, ...)`; all a11y strings from `context.l10n` with the `a11y` prefix — the allowlist is empty, so any new violation fails CI. **(f) Caching** — no new manual cache maps; you-gap RSVP/claim mutations flow through the existing providers → `ExperienceRepository` which owns invalidation; do not touch `StashCacheManager` from widgets/viewmodels (caching.md §Key Caching Rules). **(g) Testing** — ViewModel-first (`ProviderContainer` + Mockito, `addTearDown(container.dispose)`, `reset(mock)` in `tearDown`); widget tests are thin and need the `l10n_helpers.dart` `MaterialApp` wrapper; use `runDebounced` for the existing 300ms `scheduleRefresh` debounce, never `Future.delayed`. **(h) Server follow-ups** — the four backend issues are out of scope here but must follow server conventions when filed: each new RPC gets its own request/response types (CLAUDE.md proto conventions), `connecterr.Internal` for internal errors, structured logging with `experience_id` / masked fields at INFO/DEBUG, no service-to-service deps, goroutine-panic safety, and any new `CommunityEventType` (e.g. loop-in invite, widen-share) classified in `server/undo/registry.go`.

## Implementation status — `exp-advanced` (updated 2026-06-02)

Phases 1–3 are built and live; the Experience read shell ([experience_read_shell.dart](../../app/lib/presentation/screens/experience/widgets/experience_read_shell.dart)) renders a deliberately **slimmer, refined composition** than the original plan after a round of in-session UX iteration. **The phase checkboxes below are the original plan of record; this section is the source of truth for what actually shipped and how it deviates.**

### Shipped and wired into the live read shell
- **Read shell** ([experience_read_shell.dart](../../app/lib/presentation/screens/experience/widgets/experience_read_shell.dart)) — a **bottom-anchored, non-scrolling** content panel over the feed's hero with the legacy semi-transparent black gradient (no grab handle, no internal scroll, so the TikTok-style vertical feed swipe passes through). Read-only; edit mode still routes through the existing event pane.
- **`ContentHeadline`** (title only), **`ContentFactsRow`** (WHERE on the left, WHEN on the right; tappable → time/location pickers), **`ContentEdgesCard` + `ContentEdgeRow`** ("Who's pitching in?" — **names only, no avatars**; neutral header color matching the fact labels; the **current viewer is pinned to the top** with the **RSVP CTA inline** in their row), **`ContentDiscussionCard`** (the description rendered as the discussion's **opening comment** + reply count + most-recent-comment preview, placed **below** the pitching-in section).
- **Lifecycle derivation** — `ContentLifecyclePhase` enum + `lifecyclePhase` / `edges()` / `isFarAway` as a pure extension on `ExperienceState` ([experience_content_view_data.dart](../../app/lib/presentation/viewmodels/experience_content_view_data.dart)), re-exported from the view-model. Wrapped/cancelled terminal treatment (recap text, status pill, dimmed hero) ships.
- **Unified RSVP composer** ([experience_rsvp_composer_sheet.dart](../../app/lib/presentation/screens/experience/widgets/experience_rsvp_composer_sheet.dart)) — intention (Going / Maybe / No) + the "Bringing" contributions row + Send. Frosted-glass (`GlassSheet` + glass primitives) with a 40 % content scrim.
- **"Who's pitching in" roster** — superseded by #2280: the bottom-sheet modal (`experience_pitching_in_sheet.dart`) was replaced by an **expand-in-place full-screen panel** ([experience_pitching_in_screen.dart](../../app/lib/presentation/screens/experience/widgets/experience_pitching_in_screen.dart)) that grows from the card's footprint via a clip-reveal morph route ([morph_reveal_route.dart](../../app/lib/presentation/widgets/content/morph_reveal_route.dart)). Roster is grouped (Going / Maybe / Not going) with a stocked-needs bar and tap-to-claim pills; assembly moved into the view-model (`rosterGroups()` / `stockedSummaryFrom()`).
- **Shared claim flow** — `NeedClaimChip` ([need_claim_chip.dart](../../app/lib/presentation/widgets/needs/need_claim_chip.dart)) renders every need/contribution/suggestion pill identically (a `GlassChip` with an optional **×N quantity badge** and a **comment glyph**); tapping routes through `NeedsActions.openClaimSheet` → the Request flow's **`NeedsClaimConfirmSheet`** ("Claim a need" when the viewer isn't bringing it, "Edit your contribution" when they are).

### Built but NOT currently wired
These exist under [widgets/content/](../../app/lib/presentation/widgets/content/) from earlier phases but have **no live consumer** after the refinements below. They are kept for the Request re-skin / possible re-wiring — decide their fate in Phase 4 (wire, keep for Request, or delete to avoid dead code): **`ContentHero`**, **`ContentSheetScaffold`**, **`ContentChipRow`**, **`ContentMessagesCard`**, **`ContentYouGapRow`**, **`ContentCtaBar`**, **`ContentWhatsNext`**, **`ContentUpdatesFeed`**, **`ContentFriendshipLedger`** (the last two are also data-gated — no backend data yet).

### Key deviations from the plan
1. **Layout:** non-scrolling bottom panel over the feed hero (the feed owns vertical swipe) **instead of** the scrollable `ContentSheetScaffold`-over-`ContentHero`. No grab handle, no internal scroll.
2. **Chip row removed** as redundant (host / location / type were already conveyed elsewhere).
3. **Chat → discussion:** the `ContentMessagesCard` preview was replaced by `ContentDiscussionCard` (description-as-first-comment + replies + recent comment) and **moved below** "Who's pitching in" rather than sitting between facts and edges.
4. **You-gap + bottom CTA bar → inline CTA:** the standalone you-gap row and sticky `ContentCtaBar` were dropped; the RSVP CTA now lives **inline in the viewer's pinned edge row**. `isFarAway` distance-muting is derived but **not currently surfaced** (it lived on the removed you-gap).
5. **Composer scope:** the unified composer initially included "Suggest another time / location" entry points; these were **removed** at the user's request (the composer is now RSVP + contributions only). `rsvpComposerSuggest` is now an unused ARB key.
6. **Pill semantics unified:** pills across the composer and roster share `NeedClaimChip` and the same claim/edit modal; chips surface quantity (×N) and a comment glyph.
7. **Misc UX:** facts swapped (location left), pitching-in header recolored neutral, avatars removed from the pitching-in list, opaque-white content fonts.

### Tests & gates (current)
Content-widget tests (incl. `content_discussion_card_test`, `content_edges_card_test`), experience read-shell / composer / pitching-in tests, and glass-chip tests pass (~370 in the content + experience suites). `lint:dart:{a11y,i18n,l10n-parity,size}` green; iOS simulator debug build green. **Not yet run:** the full `flutter test` suite, `lint:nav`, `lint:ephemeral-comments`, and Android.

## Work remaining

- **Phase 4 (reuse validation + docs) — not started.** Audit the wired widgets for Experience-specific leakage; map each to the Request consumption; **rewrite [docs/client/content.md](../client/content.md)** (still documents the old tab-pane layout).
- **Resolve the unwired widgets** listed above — wire them, reserve them explicitly for Request, or delete them. Leaving them as dead code risks confusion and bloats the package.
- **Chat integration re-verification (Phase 3 carry-over):** unread-badge correctness across phases, the deep-link "open in chat" entry, and terminal-state conversation handling against the final `onOpenConversation` route. The discussion card's recent-comment preview also depends on the server populating `lastMessageText` / `lastMessageSender` / `lastMessageTimeAgo` on `GetExperience` — confirm on-device.
- **Data-gated surfaces:** `ContentUpdatesFeed` and `ContentFriendshipLedger` never render today (no backend) — wire them when their follow-ups land. Decide whether to resurface `isFarAway` distance-muting now that the you-gap is gone.
- **Test debt:** add disposal-safety tests for the new async paths (`openClaimSheet` and the RSVP/claim/edit transitions) per architecture.md §Disposal Testing; run the full suite + remaining gates.
- **Follow-up backend issues NOT yet filed** (a search on 2026-06-02 found none): reach/view metrics, shared-history (ledger) API, social-tier sharing, personal loop-in invite, remote-participation + reciprocity. File these so the gaps are tracked.
- **Cleanup:** prune the now-unused `rsvpComposerSuggest` ARB key if the time/location cards stay out.
- **On-device verification** of the full flow (RSVP commit, claim/edit, contribution quantity, discussion population) against the dev server — not automatable here.

## Problem Statement

We're starting a ground-up redesign of the client content views, beginning with the **Experience (Event) content view** ([experience_content_view.dart](../../app/lib/presentation/screens/experience/experience_content_view.dart), 965 lines). The **Request content view** follows next against [request-help-v8.html](../cowork/App%20Design/request-help-v8.html), so the explicit constraint is to **build Experience out of role-agnostic, reusable widgets** that Request can re-skin with different data, labels, and verbs.

The new design (event-rsvp-v7.html) is a different surface from what ships today:

- **Today** (post-#2154): a `Stack` of full-bleed background → gradient → a `Positioned` bottom panel of *swipeable tab panes* (Event · Chat), with inline `ContentInfoRow`s, a trailing access pill, and an owner "manage" pencil sheet. See [docs/client/content.md](../client/content.md) for the full current architecture.
- **New**: a full-bleed **hero** that the user **scrolls a single editorial sheet over**. No primary tab bar. The sheet stacks: host/phase **chip row** → serif **title** + description → **facts** cards → **messages card** (the chat entry point — see below) → **reach line** → **edges card** (the contribution/RSVP graph, including the one-tap "you-gap" action) → **updates feed** → **friendship ledger** → sticky phase-aware **CTA bar** → **whats-next** nudge. The hero, copy, CTA, and edge styling all shift across a lifecycle (**Sharing → Confirmed → Leaving → On the road → Wrapped**), culminating in a photo-grid **wrapped** state.

**Decisions confirmed with the user (2026-06-02):**
- **Chat lives in an inline messages card** positioned **below the facts row and above the edges card** — a preview row (chat glyph · "N messages" · unread dot · "{name} just posted · {time}" · chevron). Tapping it opens the full conversation either as a **bottom modal** (`showAccessibleModal`) or a **right-swing route that replaces the content** (the behavior the current tabs use). Both are acceptable; recommend the right-swing route via `NavigationHelpers.pushScreen()` + `SwipeToCloseMixin` for consistency with content-screen navigation, re-hosting the existing `InlineConversationView` and re-pointing the unread/deep-link wiring at that route.
- **v1 ships without** the reach line, friendship ledger, widen-share, and loop-in invite — those are a **future PR** gated on the backend follow-ups below. All other open-question recommendations are confirmed as written.

### Relationship to #2154 / #2175 (important)

#2154 (Experience) and #2175 (Request) were a *lighter* redesign against an **older mockup** (`event-item-4.html`): they unified chrome *within* the existing tab layout (dropped the Plan tab, folded actions into rows, added the manage pencil + access pill, migrated to shared Needs widgets). **Both have shipped.** This issue **supersedes that layout** — the tab-pane structure those plans refined is largely replaced. But the work they landed is the reuse foundation here: shared `widgets/needs/*`, [content_access_pill.dart](../../app/lib/presentation/widgets/content/content_access_pill.dart), [row_call_to_action_pill.dart](../../app/lib/presentation/widgets/content/row_call_to_action_pill.dart), [experience_manage_menu_sheet.dart](../../app/lib/presentation/screens/experience/widgets/experience_manage_menu_sheet.dart), the `experienceProvider` state/viewmodel, and the `ExperienceState` lifecycle accessors all carry forward.

## Context from Comments

The issue has no comments — it was filed in this session. The authoritative spec is the mockup ([event-rsvp-v7.html](../cowork/App%20Design/event-rsvp-v7.html)); its own changelog (phones 1–6 + the "what ported clean" principle block) resolves most ambiguities. The two prior plan docs (#2154, #2175) are de-facto context for the current code baseline and the reuse primitives.

The mockup itself flags several things as **prototype-only or research scaffolding** that must NOT ship as drawn:
- The **phase toggle** (5 buttons) is "Prototype · move through phases" — production derives the phase, it is not a control.
- The **reach-line A/B copy variants** ("quietly passed / not this time / saw later / sat it out") are "research copy," hidden in production.

## Codebase & API findings (what's reusable vs. what needs backend)

**Reusable today (client-only path exists):**
- **Hero/background:** [video_background_host.dart](../../app/lib/presentation/widgets/media/video_background_host.dart) owns the player lifecycle; [content_gradient_overlay.dart](../../app/lib/presentation/widgets/content/content_gradient_overlay.dart) the fade. The new hero is a restyle/extension, not a rewrite.
- **Edges data:** RSVPs (`RSVP` message: `intention` YES/MAYBE/NO + `attended`) and the full Needs/Contributions system (`ExperienceNeedResponse`, `ExperienceContributionResponse`, `ListExperienceNeedsAndContributions`, claim/contribute RPCs) already model "who's bringing what." The **you-gap one-tap** maps to `NeedsActions.claim` / RSVP — no new RPC.
- **Lifecycle:** `ExperienceState` = `ACTIVE / JOINED / IN_PROCESS / COMPLETED / CANCELLED`, plus `started_at_unix_sec` / `completed_at_unix_sec`. The design's **Leaving** and **On the road** are time-derived sub-phases of `IN_PROCESS`, not new states.
- **Distance degradation:** `locationDistanceMeters` already lives on `ExperienceState`. Muting the in-person RSVP past a distance/time threshold is client-only.
- **Manage / edit:** inline edit mode (`ContentEditingMixin`) + `ExperienceManageMenuSheet` carry over unchanged.
- **Wrapped state:** completion summary + impact already exist (`completion_summary`, `GetExperienceStatsResponse`, the completion mixin).

**Needs new backend — carve into follow-up issues, do NOT block v1:**
- **Reach line** ("3 going · 5 quietly passed · 12 saw"): **no** view/impression/silent-pass data in any API proto.
- **Friendship ledger** ("You + Anna · 4 trips together"): **no** shared-history-between-two-users API.
- **Widen-share tiers** (concentric "Close friends / + Friends / + Climbing group"): only community-level `ShareExperience` exists; **no** social-tier concept.
- **Loop-in personal invite** (private invite + consent + AI-drafted note): **no** per-person invite API distinct from community share / RSVP.
- **Event-shaped distance fallbacks** ("Follow the album / Tip jar / Friday video chat") and **wrapped reciprocity** ("Plan the next trip", cost-split): new product features, not just UI.

**Theme reality (per [feedback_theme_reuse](../../../.claude/projects/-Users-thomasescobar-Documents-GitHub-ripls/memory/feedback_theme_reuse.md)):** the mockup's serif is **Newsreader** and its accent is green — but the app **already** has a serif heading font (`AppTheme.headingFont = 'Libre Baskerville'`) and a sage-green primary palette (`AppColors`). **Map the mockup to existing tokens: Newsreader → `AppTheme.headingFont`, mockup green → `AppColors` sage primary.** Do **not** add Newsreader or invent per-section coral/cream/font constants; do **not** create `v2/`-suffixed directories.

## Potential Solutions

### Option A: One PR, full in-place rewrite of the Experience view
- **Approach:** Rewrite `experience_content_view.dart` + its `widgets/` to the new sheet-over-hero layout, extracting shared widgets along the way, all in one PR. Drop the tab pane structure.
- **Pros:** Single coherent change; the new surface ships intact.
- **Cons:** Enormous PR (the live screen is 965 + ~900-line panes); high revert risk; the **chat IA question** (no more tab — see Open Questions) and the **backend-gap surfaces** force partial/hidden states anyway, so "full" isn't actually achievable in one shot. Hard to validate reuse before Request.
- **Effort:** Large.

### Option B: Phased — shared scaffold first, then interactivity, then lifecycle, gated follow-ups for backend-dependent surfaces
- **Approach:** Phase 1 builds the role-agnostic scaffold widgets and renders Experience **read-only** in the new layout on existing data. Phase 2 adds the interactive edges/you-gap/CTA + lifecycle derivation + distance muting. Phase 3 adds the wrapped/terminal state, updates feed, chat integration, and the (data-gated) friendship ledger shell. Phase 4 validates reuse + docs. Reach line / widen / loop-in / fallbacks / reciprocity become **separate backend+UI follow-ups**.
- **Pros:** Each phase is shippable and reviewable; reuse is proven incrementally (Phase 1 outputs are the Request foundation); backend gaps don't block the visible redesign; matches the project's "delete-and-replace, don't fork" convention without a flag.
- **Cons:** The new surface lands feature-incomplete vs. the full mockup until follow-ups ship (acceptable — the gated surfaces need backend work regardless).
- **Effort:** Large overall, but each phase is Medium.

### Option C: Build the shared component library standalone first, compose later
- **Approach:** Ship `widgets/content/` scaffold widgets with their own gallery/tests first, then wire Experience.
- **Pros:** Forces clean role-agnostic APIs up front.
- **Cons:** Components designed without a live consumer drift from real needs; no user-visible value until the wiring PR; against the grain of how #2154/#2175 extracted shared widgets (in-situ, with a consumer).
- **Effort:** Large, back-loaded.

## Recommendation

**Option B — phased, shared-scaffold-first, with backend-dependent surfaces split into follow-ups.** It's the only option that respects the two hard realities: the chat information-architecture decision and the backend data gaps both force incremental delivery, and reuse is best proven by extracting each widget against the live Experience consumer (exactly how #2154/#2175 succeeded) and *then* re-skinning Request.

Concretely: every widget Phase 1–3 produces lands under [app/lib/presentation/widgets/content/](../../app/lib/presentation/widgets/content/) with **no Experience-specific assumptions** (data in via typed view-data structs/callbacks, labels/verbs via parameters), so the Request follow-up is a re-skin. Reuse the existing `experienceProvider`/`ExperienceState` rather than introducing new state.

The IA/scope decisions are now **confirmed** (2026-06-02): chat = inline messages card opening a right-swing route; v1 ships without the four backend-gated surfaces (future PR); theme maps to existing `AppColors` + `AppTheme.headingFont`. The remaining recommendations below stand as written.

## Open Questions

- **Where does Chat/Discuss live now that there's no tab?** — **RESOLVED (user, 2026-06-02):** an **inline messages card** between the facts row and the edges card (chat glyph · "N messages" · unread dot · "{name} just posted · {time}" · chevron). Tap opens the full conversation as either a bottom modal or a right-swing route replacing the content; recommend the **right-swing route** via `NavigationHelpers.pushScreen()` + `SwipeToCloseMixin` (not raw `Navigator.push` / `context.push()`; `npm run lint:nav` blocks otherwise), re-hosting the existing `InlineConversationView`. Preserve the `unreadCountProvider` badge wiring and any deep-link `initialTab`-style "open in chat" entry by re-pointing it at the route. The updates feed ("since you…") remains a separate inline surface for non-chat activity.
- **Does v1 ship without the reach line, friendship ledger, widen-share, and loop-in invite?** — **RESOLVED (user, 2026-06-02): Yes — ship the redesign without them; they are a future PR.** Build each component data-gated ("render only when data present") so it lights up when its follow-up backend lands. File the four follow-ups (below) immediately so the gaps are tracked, not lost.
- **How are the lifecycle "phases" derived?** The phase toggle is prototype-only. — **Recommendation:** compute a **client-side UI phase enum** from `ExperienceState` + time proximity (`time` window, `started_at_unix_sec`): `ACTIVE`→Sharing, `JOINED`→Confirmed, `IN_PROCESS` split into Leaving (pre-start, within N hours) / On-the-road (after `started_at`), `COMPLETED`→Wrapped, `CANCELLED`→a cancelled treatment. No new proto for the core phases. Put the derivation on `ExperienceState` as a pure getter (unit-testable, no widget tree). The **enum is presentation-neutral** — the widget maps it to localized chip/CTA copy via `context.l10n` in `build()` (i18n.md: viewmodels never resolve strings).
- **Theme mapping — Newsreader/green → existing tokens?** — **Recommendation:** **Yes.** Use `AppTheme.headingFont` (Libre Baskerville) for the serif title and `AppColors` sage helpers for the accent; the mockup's amber/clay/done accents map to existing `AppColors` semantic colors (warn/amber, sage, info-blue). Add at most one named `AppColors` helper if a genuinely new tone is unavoidable — never inline hex, never per-section constants (memory: [feedback_theme_reuse](../../../.claude/projects/-Users-thomasescobar-Documents-GitHub-ripls/memory/feedback_theme_reuse.md)).
- **Edges data source — RSVPs + Needs/Contributions?** — **Recommendation:** **Yes.** Build the edges list from RSVPs (status pills: IN / HOSTING / WRAPPED from `intention`/`attended`/state) joined with each user's contributions (`ExperienceContributionResponse`). The you-gap one-tap dispatches to RSVP (`updateRSVP`) and/or `NeedsActions.claim` via `NeedsScope.experience(...)`. No new data.
- **Distance degradation in v1?** — **Recommendation:** **Partially — yes for the muting, no for the fallbacks.** Mute/annotate the in-person you-gap when `locationDistanceMeters` exceeds a threshold (data exists). The "Follow the album / Tip jar / Friday video chat" alternatives are new features → follow-up.
- **Wrapped state in v1?** — **Recommendation:** **Yes, the client-only parts:** photo-grid hero from existing `allMediaItems`, `WRAPPED` chip, past-tense edges with WRAPPED pills, and the existing completion summary/impact. **Defer** the reciprocity prompt ("Plan the next trip") and cost-split — both new features → follow-up.
- **In-place rewrite vs. new files?** — **Recommendation:** in-place; new shared widgets under `widgets/content/`. **No** `_v2` files/dirs and **no** feature flag dir-suffix (memory ban). The branch (`exp-advanced`) is the isolation boundary.
- **Edit mode & manage menu — keep?** — **Recommendation:** **Keep** `ContentEditingMixin` + `ExperienceManageMenuSheet` (owner pencil by the title). They're orthogonal to the layout change and already role-aligned.
- **File-size ceiling.** The new view + panes must each stay under the 1,000-line CI cap (`npm run lint:dart:size`). — **Recommendation:** the scaffold-extraction approach naturally keeps the screen thin (it becomes composition); watch the edges/CTA widgets. No `// dart-line-count-allow` hatch.

## Implementation Plan

### Phase 1: Shared scaffold + read-only Experience shell
**Status (2026-06-02): Largely done.** Scaffold widgets + the `ContentLifecyclePhase` derivation shipped, but the scrollable `ContentSheetScaffold`/`ContentHero`/`ContentChipRow`/`ContentMessagesCard` were superseded by a non-scrolling bottom panel + `ContentDiscussionCard`. See **Implementation status** above.
**Goal:** Stand up the new sheet-over-hero layout as role-agnostic widgets and render the Experience **read-only** on existing data — no behavior loss, chat still reachable.
**Tasks:**
- [ ] Build under [widgets/content/](../../app/lib/presentation/widgets/content/), each taking typed view-data + callbacks (no Experience proto types, no repository access): **`ContentHero`** (state-driven tint/height + gradient fade + glass top bar; wraps `VideoBackgroundHost`; tint transitions via `accessibleDuration(context, ...)`; top-bar icons via `IconAction` with `tooltip`+`semanticsLabel`), **`ContentSheetScaffold`** (scrollable sheet overlapping a hero: grab handle, rounded top, sticky footer slot), **`ContentChipRow`** (attribution/host chip + status/phase chip variants), **`ContentFactsRow` / `ContentFactCard`** (1–2 stat cards with state styling), **`ContentMessagesCard`** (chat-preview row: glyph · "N messages" · unread dot · "{name} just posted · {time}" · chevron; built with `Tappable` + `semanticsLabel`, watches `unreadCountProvider`), and a serif **title/description** block using `AppTheme.headingFont`.
- [ ] Add a **`ContentLifecyclePhase`** enum + a derivation getter on `ExperienceState` (state + time → phase). Pure getter; unit-tested. The widget maps the enum → localized copy via `context.l10n` (not the viewmodel).
- [ ] Recompose `experience_content_view.dart` to render the read shell from these widgets, replacing the Event-pane content, with the messages card between the facts row and where the edges card will land. Wire the messages card to open the conversation via `NavigationHelpers.pushScreen()` (right-swing route, destination using `SwipeToCloseMixin`) re-hosting the existing `InlineConversationView`; re-point the unread-badge and deep-link "open in chat" wiring at that route. The screen keeps the `initialize()` + `addPostFrameCallback` pattern it already uses.
- [ ] Theme via `AppColors(context)` helpers + `AppTheme.headingFont` only — no hardcoded hex, no `Colors.white`/`Colors.grey[...]`, no per-section constants, no `v2/`-suffixed dirs (architecture.md §Colors and Theming; [feedback_theme_reuse](../../../.claude/projects/-Users-thomasescobar-Documents-GitHub-ripls/memory/feedback_theme_reuse.md)).
- [ ] **l10n:** route any new user-visible strings through `context.l10n`; add keys to **both** `app_en.arb` and `app_es.arb` (l10n-parity gate); a11y labels use the `a11y` prefix; run `npm run generate`.
- [ ] **Tests:** unit-test the phase-derivation getter (pure, no widget tree); a `ProviderContainer` + Mockito test (with `overrides`, `addTearDown(() => container.dispose())`, `reset(mock)` in `tearDown`) that the Experience shell view-data renders title/facts/chips across phases; thin widget tests for each scaffold widget using the `app/test/helpers/l10n_helpers.dart` `MaterialApp` wrapper. Follow [docs/client/testing.md](../client/testing.md).
**Validates:** the new layout renders real Experience data, the shared widgets have clean role-agnostic APIs, chat is still reachable, and lint/size/a11y/l10n gates pass.

### Phase 2: Edges card + you-gap action + phase-aware CTA + distance
**Status (2026-06-02): Done with deviations.** The view-model edges join + `ContentEdgesCard`/`ContentEdgeRow` shipped; the standalone you-gap row and sticky `ContentCtaBar` were replaced by an **inline CTA in the viewer's pinned edge row**. Distance-muting (`isFarAway`) is derived but not surfaced. See **Implementation status** above.
**Goal:** Make the surface interactive on existing data.
**Tasks:**
- [ ] **Assemble edges view-data in the viewmodel.** Add an `ExperienceNotifier` method/getter that joins RSVPs with contributions into a typed `EdgeViewData` list (status pill derived from `intention`/`attended`/lifecycle) — this is business logic and must not live in the widget (architecture.md §Key Architectural Rules). Async paths guard with `if (!ref.mounted) return;` before `state = ...`.
- [ ] **`ContentEdgesCard`** + **`ContentEdgeRow`** (avatar · name · contribution · status pill) render the `EdgeViewData`. Avatars via `CachedMediaImage(semanticsLabel: 'Photo of ${name}')` (a11y gate `require_semantic_label_on_cached_media_image`).
- [ ] **`ContentYouGapRow`** — the one-tap prompt + quick-pick chips. Wire to `updateRSVP` and `NeedsActions.claim` via `NeedsScope.experience(...)`. Quick-pick / confirmed states use `Toggle` with `selected:` (selectable state, per accessibility.md); reuse [row_call_to_action_pill.dart](../../app/lib/presentation/widgets/content/row_call_to_action_pill.dart). Mutations run through the existing provider → `ExperienceRepository` (which owns cache invalidation — no manual cache writes).
- [ ] **`ContentCtaBar`** (sticky status block + primary action pill) whose label/tone/action follow `ContentLifecyclePhase` (resolved to copy in the widget). Reuse the existing RSVP action machinery ([experience_rsvp_button.dart](../../app/lib/presentation/screens/experience/widgets/experience_rsvp_button.dart) / `WorkflowActionButtons.buildExperience`) and the owner manage pencil → `ExperienceManageMenuSheet` opened via `showAccessibleModal`. **`ContentWhatsNext`** nudge line below it.
- [ ] **Distance muting:** when `locationDistanceMeters` exceeds the threshold, render the in-person you-gap as muted/annotated ("not suggested — you're far"). Alternatives (album/tip/video) are **out of scope** (follow-up).
- [ ] **Tests:** viewmodel-level coverage of the edges-join getter and you-gap → RSVP/claim transitions (happy path + error → typed `UserError? error`), plus a **disposal-safety test** (start the async op, `container.dispose()` mid-flight, `expectLater(future, completes)`) per architecture.md §Disposal Testing. Thin widget tests for edge-row status pills, CTA per phase, and distance-muting using the l10n test wrapper.
**Validates:** RSVP and "bring something" work end-to-end through the new surface; the CTA and edges reflect the live phase; distance is a first-class fact.

### Phase 3: Wrapped/terminal state + updates feed + chat integration + ledger shell
**Status (2026-06-02): Partial.** Wrapped/terminal treatment + the discussion card shipped; `ContentUpdatesFeed` and `ContentFriendshipLedger` are built but **data-gated and never render yet**; chat deep-link / unread re-verification is **outstanding**. See **Implementation status** + **Work remaining** above.
**Goal:** Complete the lifecycle and the data-gated reuse components.
**Tasks:**
- [ ] **Wrapped state:** photo-grid hero from `allMediaItems`, `WRAPPED`/`CANCELLED` chip treatment, past-tense edges with WRAPPED pills, and the existing completion summary/impact rendered inline. (Reciprocity + cost-split deferred.)
- [ ] **`ContentUpdatesFeed`** ("since you…") — the viewmodel assembles the typed feed-item list from existing data (new RSVPs/contributions/messages); the widget renders it and routes a tap to the conversation (`NavigationHelpers.pushScreen`) or the relevant row.
- [ ] **Chat integration:** verify the messages card (built Phase 1) against the final lifecycle — unread badge correctness across phases, terminal-state conversation handling, and deep-link `initialTab`-style "open in chat" entry routing to the pushed conversation (re-point any `chatTabIndex` callers).
- [ ] **`ContentFriendshipLedger`** — build the component **role-agnostic and data-gated**: it renders only when shared-history data is present. Until the backend follow-up ships, it's hidden (no data). This proves the component for both Experience (trips) and Request (loans).
- [ ] **Tests:** wrapped-state rendering, updates-feed item routing, ledger renders-when-data / hidden-when-absent.
**Validates:** the full client-only lifecycle (sharing → wrapped) renders for owner and guest; the ledger/feed components are reuse-ready.

### Phase 4: Reuse validation + documentation
**Status (2026-06-02): Not started.** See **Work remaining** above.
**Goal:** Prove the Request reuse goal and document the new architecture.
**Tasks:**
- [ ] Audit every new `widgets/content/` widget for Experience-specific leakage; fix any that would force a Request fork.
- [ ] PR **reuse checklist**: map each shared component to how Request ([request-help-v8.html](../cowork/App%20Design/request-help-v8.html)) will consume it (different verbs/labels/data), validating the issue's core acceptance criterion **before** building Request.
- [ ] Rewrite [docs/client/content.md](../client/content.md) to describe the new sheet-over-hero architecture (it currently documents the tab-pane layout).
- [ ] Full gate run: `npm run lint:dart:size`, `lint:dart:a11y`, `lint:dart:i18n`, `lint:dart:l10n-parity`, `lint:nav`, `lint:ephemeral-comments`; `npm run generate`; `flutter test --reporter expanded`. Manual verify on iOS sim + Android emulator via [/ripls-app-logs](../../.claude/skills/ripls-app-logs/SKILL.md).
**Validates:** the components are genuinely role-agnostic, docs match reality, and the Request follow-up is unblocked.

### Follow-up issues (file now; backend-dependent — out of scope here)
**Status (2026-06-02): Not yet filed** — a GitHub search found none of the five. File them so the data-gated surfaces are tracked.
- [ ] **Experience view/reach metrics** — track impressions + "saw but didn't respond" to power the reach line. New proto + backend.
- [ ] **Shared-history (friendship ledger) API** — co-participation count + recent shared events between two users.
- [ ] **Social-tier sharing (widen-share)** — concentric invite circles beyond community-level sharing, with privacy rails.
- [ ] **Personal loop-in invite** — per-person invite distinct from community share/RSVP, with consent + (optional) AI-drafted note.
- [ ] **Event-shaped remote participation + reciprocity** — distance fallbacks (album/tip/video chat), wrapped "plan the next," cost-split. (May fold into existing event/invite work — see related.)

## Related Issues
- #2154 — Experience redesign against the older `event-item-4.html` (shipped); this issue supersedes that layout but reuses its primitives.
- #2175 — Request redesign mirroring #2154 (shipped); the Request follow-up to *this* issue replaces that layout in turn.
- #2148 — Needs & Contributions modal redesign (the `widgets/needs/` primitives reused by the edges/you-gap surface).
- #2237 — Event invite/join (likely overlaps the "loop-in invite" follow-up).
- #1858 — Content-views refresh (background on content-view caching/refresh behavior).
