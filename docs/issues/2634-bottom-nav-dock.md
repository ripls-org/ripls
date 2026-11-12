# Issue #2634: Restructure primary nav: converged 5-target bottom dock (Home · Plans · Create · Library · People) + search orb — liquid glass

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2634
**Filed:** 2026-07-04 | **Last activity:** 2026-07-04
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** `docs/cowork/App Design/bottom-nav-liquid-glass-mock.html` (rev 7, the source design), `docs/llms.txt`, anchors `client/architecture.md` + `client/conventions.md` + `users/priorities.md` + `users/personas.md` (skimmed), leaf docs `client/modals.md`, `client/accessibility.md`; review dimensions engaged: accessibility, performance, testing, i18n.

## Problem Statement

The app's primary navigation no longer matches where the product design has converged. Today the shell is a hand-rolled 3-slot pill (Home/inbox, Feed, Directory-or-Workshop) plus a detached Create FAB, backed by an `IndexedStack` inside `HomeScreen`. The liquid-glass mock (rev 7 of the converged-nav series) locks a different information architecture: a floating glass capsule with **Home · Plans · Create (center) · Library · People**, a detached universal-search orb, one badge (the Needs You queue) and one account entry (an avatar chip on every destination header opening a Me bottom sheet).

Two of the five destinations (Plans, Library) do not exist as top-level surfaces today — Plans lives as a sub-screen of the Home tab ("Calendar" see-all) and Library (browsable gear across groups) has no direct equivalent; the closest is the retired Discover screen and per-community gear lists. The Feed tab, conversely, exists today but has no slot in the target dock. So this is an IA restructure, not a reskin.

The issue was filed by the design side with no discussion yet; the mock itself is unusually specific (locked target set, badge math, verb rules, fallback behavior, build-spec footer), so most requirements are firm, but the *migration path* — what happens to Feed and Workshop, and how the two new destinations get built — is undefined and is the core of this plan.

## Context from Comments

No comments yet — the issue was filed today directly from the design mock. Flagging per skill protocol: an issue with no discussion may need maintainer input on the open questions below before Phase 2+ work starts. Phase 1 (dock component behind a flag) is safe to start regardless.

The mock's own prose carries decisions that function as maintainer direction:

- **Locked:** the bar is capped at six targets forever — four places, an action, a question. Money/rallies live inside Home and group pages; notifications live in the badge; account lives in the avatar chip.
- **Badge math:** one badge, one queue — the Home badge equals the Needs You queue count, and group invitations move *into* that queue.
- **Vocabulary:** the mock says "Groups", not "circles"/"communities" — **overridden by maintainer decision below: use "Communities."**
- **Search contract:** the orb searches everything in your groups and nothing else; zero results always converts to contribution; natural language is explicitly deferred to v2.
- **Escape hatch:** if Phase 2 shows crowding, the orb merges into a fifth capsule slot — nothing else moves.
- **Header Variant A vs B** (messages icon + clean avatar vs. dotted avatar only) is an explicit unresolved test pair — "Phase 2 picks one for all four tabs."

## Current State (codebase research)

- **Shell:** [`home_screen.dart`](../../app/lib/presentation/screens/home/home_screen.dart) — `_buildBottomNavigation()` (solid pill, `cardBackground.withAlpha(217)`, 54 px, 3 slots), `_buildCreateFab()` (separate floating +), `_visualToStackIndex`/`_stackToVisualIndex` (visual↔stack mapping with an orphaned Discover entry), `IndexedStack` in `_buildBody()`. Tab state in [`home_view_model.dart`](../../app/lib/presentation/viewmodels/home_view_model.dart) (`selectedIndex`, `navigateToTab`). No `ShellRoute` — tab switching is Riverpod state inside the `/` route.
- **Destinations that already exist:** Home = `HomeTabScreen` (#2435 inbox: Up next · Needs you · Yours; badge already wired to `homeNeedsYouCountProvider`); People ≈ `DirectoryScreen` (#2568, `directoryEnabledProvider` defaults on); Create = `UnifiedCreateModal`; Plans ≈ `home_calendar_screen.dart` (server-assembled `GetHomeView.calendar`, currently a see-all sub-screen); Library ≈ nothing top-level (gear lists exist per-community and in "Yours"; the Discover map screen is orphaned but its Mapbox plumbing — `DiscoverMapHelper`, `map_avatar_renderer` — is alive).
- **Glass:** the modal system already has the primitive — [`glass_surface.dart`](../../app/lib/presentation/widgets/modal/glass/glass_surface.dart) (`BackdropFilter` + blur with a `useBlur` low-end fallback) and `ModalTheme` constants. The nav pill itself is solid today. `client/modals.md` mandates composing from glass primitives and `AppColors.modal*` tokens rather than ad-hoc styling.
- **Search:** no nav-level search. `HomeSearchScaffold` decorates Home sub-screens; the server has a search service with semantic search (`docs/server/semantic_search.md`) over gear/requests/experiences — but no single grouped "everything in your groups" RPC.
- **Account entry:** `ProfileMenuAvatar` → `ProfileMenuModal`, already placed in tab-screen headers — structurally very close to the mock's avatar chip → Me sheet.
- **Constraints that bind this work:** theme tokens only (`AppColors.*`, no mock hexes — the mock's paper/forest palette must map to Ink & Sage tokens); a11y primitives (`Tappable`/`IconAction` + `context.l10n` labels, CI-enforced); `accessibleDuration()` for the dock animations; 1,000-line Dart file cap (`home_screen.dart` is already ~670 lines — the dock must be extracted, not grown in place); l10n for all new labels; "Event" not "Experience" in any Plans copy.

## Potential Solutions

### Option A: Incremental restructure of the existing shell, behind a feature flag
- **Approach:** Keep the `HomeScreen` + `IndexedStack` + `homeProvider` architecture. Extract the dock into a new `presentation/widgets/navigation/` package (glass capsule + center Create + orb), extend the `IndexedStack` with the new destinations, and gate the whole IA switch on a compile-time flag (`Environment.navDockEnabled` + Riverpod provider, same pattern as `homeRedesignEnabledProvider`). Ship in phases: dock with existing destinations first, then Plans/Library promotion, then search orb, then Me sheet.
- **Pros:** Follows the proven rollout pattern (#1895, #2435, #2568 all did exactly this); every phase is revertible by flag; reuses `GlassSurface`, `DirectoryScreen`, calendar, `UnifiedCreateModal`; no router surgery.
- **Cons:** The visual↔stack index mapping gets hairier before it gets simpler (mitigate: replace int indexes with an enum while extracting); legacy tabs (Feed, orphaned Discover) linger in the stack until cutover.
- **Effort:** Large overall, but medium per phase.

### Option B: New shell on go_router `StatefulShellRoute`
- **Approach:** Rebuild primary navigation as URL-driven tab routing with a fresh dock widget, retiring the `IndexedStack`-in-a-screen pattern.
- **Pros:** Deep-linkable tabs; removes the visual/stack index mapping debt; standard Flutter navigation shape.
- **Cons:** Touches the auth-redirect logic and `NavigationHelpers` conventions for zero user-visible benefit the mock asks for; big-bang risk on the app's most-trafficked surface; contradicts the incremental flag-based rollout culture the repo has used for every prior nav change.
- **Effort:** Large, high risk.

### Option C: Reskin first, IA later
- **Approach:** Restyle the existing 3-tab pill + FAB into the glass capsule (visual language only), defer the tab restructure.
- **Pros:** Small, low risk, immediate visual progress.
- **Cons:** Doesn't deliver the actual issue ("more tabs"); the capsule geometry is designed around 5 targets, so the reskin work gets redone; postpones the hard decisions without de-risking them.
- **Effort:** Small, but mostly throwaway.

## Recommendation

**Option A.** It matches how this codebase has landed every previous nav change (flagged, incremental, inside the existing shell), reuses the most existing code (glass primitives, Directory, calendar, unified create, profile menu), and keeps each phase independently shippable and revertible. Option B's router migration is real debt-paydown but belongs in its own issue after the IA has settled — doing both at once couples a product-visible restructure to an infrastructure rewrite. While extracting the dock, replace the raw int visual/stack indexes with a `NavDestination` enum to stop the mapping debt from compounding.

## Decisions (resolved 2026-07-04)

All open questions were answered by the maintainer:

- **Feed tab: eliminated.** No dock slot opens it. `feed_screen.dart` stays in the codebase for now (orphaned, like Discover), but the Home tab's **"In Your Groups" section uses the feed lists as its data source**, rendering *smaller* cards that, when tapped, open the existing full-screen views. This is in scope for this issue (Phase 2), not a separate one.
- **Library v1: map-first.** Reuse the existing `discover_screen` / search-results map view — **do not build a new custom map view**. The mock's walk-time pin labels and clustering are not v1 requirements; the existing map behavior is.
- **Universal search: yes, a new dedicated server RPC** (grouped results, keyword-only v1), as planned in Phase 3.
- **Vocabulary: "Communities", not "Groups."** The mock's Groups rename is overridden — new user-facing strings introduced by this work say "Communities," consistent with existing app copy. No rename sweep.
- **Header variant:** build Variant A (messages icon + clean avatar) as the default via one shared destination-header widget; design picks final during Phase 2.
- **Reduce Transparency:** reuse the `GlassSurface.useBlur` mechanism keyed off the existing device-capability check (plus `MediaQuery.highContrastOf`); solid fallback at identical geometry.

## Implementation Plan

### Phase 1: Glass dock component + new tab set behind `navDockEnabled` flag
**Goal:** The five-target capsule (Home · Plans · Create · Library · People) renders and navigates, flag-gated, with existing or placeholder destinations.
**Tasks:**
- [ ] Add `Environment.navDockEnabled` + `navDockEnabledProvider` (pattern: `feature_flags.dart`)
- [ ] New `presentation/widgets/navigation/` package (+ README per repo rule): `NavDock` (glass capsule via `GlassSurface`, center Create button, badge, scroll minimize/restore hooks), `NavDestination` enum replacing raw visual indexes
- [ ] Solid-fallback rendering path (no blur) at identical geometry; wire to the existing `useBlur` capability check
- [ ] Extend `HomeScreen` `IndexedStack` + `home_view_model.dart` for the new destination set: Home=`HomeTabScreen`, Plans=promoted `home_calendar_screen`, Library=map-first screen reusing the existing `discover_screen`/search-results map view (no new custom map), People=`DirectoryScreen`; Create stays `UnifiedCreateModal`. With the flag on, Feed has no tab (`FeedScreen` stays in the codebase, orphaned like Discover)
- [ ] Bottom content inset (dock height + 12) on destination scroll views; a11y labels (`a11y*` ARB keys), `Tappable`/`accessibleDuration` throughout
- [ ] Touch targets ≥ 48 dp on every dock element even where visuals are smaller (the mock's 44 pt Create button and 10.5 px labels are visual specs, not hit-area specs)
- [ ] Theme-aware glass: capsule/orb colors from `AppColors` (Ink & Sage) in both light and dark mode — the mock is light-only; verify icon/label contrast over worst-case passing content in both themes (the solid fallback is the contrast floor)
- [ ] Audit all `navigateToTab`/stack-index callers (deep links, notification routing, `ToastHelper.showUndo` margins above the floating nav) — the destination set and dock geometry both change
- [ ] Profile blur cost on low-end Android (BackdropFilter over scrolling content is per-frame work); confirm the `useBlur` capability check covers the dock, and decide lazy-build vs. eager `IndexedStack` children now that the stack grows
- [ ] ViewModel tests for destination mapping + badge; widget tests with flag overridden both ways; disposal tests per architecture doc
**Validates:** The capsule geometry works with real content at 5 targets; the flag rollback story; whether `home_screen.dart` stays under the line cap after extraction.

### Phase 2: Plans and Library become real destinations; Home absorbs the pulse
**Goal:** The two new tabs are useful, not stubs, and Home replaces the eliminated Feed tab's role.
**Tasks:**
- [ ] Plans: month grid + list toggle over `GetHomeView.calendar` data (reuse calendar widgets from `widgets/home/calendar/`), "Event" terminology in all copy
- [ ] Library: Map/List toggle (map-first, reusing the existing discover map view); filters (All communities / Available / On loan / Needed), Yours segment, owner-before-condition row layout on the list side
- [ ] Home pulse section (mock's "In Your Groups"; copy says "In Your Communities"): consume the existing feed lists (feed repository/`GetFeed` data, preserving its three-tier unread ordering) rendered as compact cards; tap opens the existing full-screen views via `NavigationHelpers.pushScreen`
- [ ] Pick header arrangement (Variant A default) as one shared destination-header widget with the avatar chip
- [ ] e2e semantics identifiers for the new destinations (`client/testing/semantics_identifiers.md` conventions)
**Validates:** Whether promoted-calendar data is sufficient for a top-level Plans tab, whether compact feed cards on Home preserve the Feed's engagement role, and Library's real data density on the reused map.

### Phase 3: Universal search orb
**Goal:** The detached orb searches everything in the user's groups, grouped Library/Plans/People, keyword-only.
**Tasks:**
- [ ] Proto: dedicated search RPC with grouped results (own request/response messages; API package only; `optional` for conditionally-present fields; per-group result caps in the contract — no unbounded repeated fields)
- [ ] Server: implement over existing search/semantic-search infrastructure, scoped strictly to the caller's communities; standard `auth.RequireAuth()` + fail-fast error conventions; follow `server/observability.md` masking rules for query logging (search text is user content)
- [ ] Authz test: a caller sees zero results from communities they don't belong to (the mock names search as "where the trust promise is easiest to break silently")
- [ ] Client: orb → bottom search field, veiled results overlay, keyboard handling per `client/modals.md` inset rules; context biasing (placeholder + group ordering) without scoping
- [ ] Zero-results → contribution conversion (prefilled Create for things; "start one" for plans)
- [ ] Round-trip tests per proto conventions; viewmodel tests for grouping/biasing
**Validates:** The search contract (nothing outside your groups) end-to-end, and whether the orb needs to merge into the capsule (the mock's escape hatch).

### Phase 4: Me sheet + polish + cutover
**Goal:** Finish the mock; retire the flag.
**Tasks:**
- [ ] Me bottom sheet replacing `ProfileMenuModal` content (Profile link, Yours-as-filtered-views rows, Account section; logout at bottom of Settings) on the standard `GlassSheet` shape
- [ ] Library no-results anti-shopping state ("ask before you buy" → Create with the search term prefilled). Walk-time pin labels and 15+ clustering from the mock are *optional polish* on the reused map — file separately if design still wants them after seeing real density
- [ ] Community invitations routed into the Needs You queue (badge math per mock)
- [ ] Default `navDockEnabled` on; delete the legacy pill/FAB code and the dead visual↔stack mapping. `FeedScreen` and Workshop stay in the codebase but unreachable from the nav; their removal is a separate cleanup issue
**Validates:** Full-mock parity and that the one-badge/one-queue invariant holds with invitations included.

## Audit notes (`/ripls-audit`, 2026-07-04)

Fixes applied to this plan during audit:

1. **Touch targets** (accessibility rubric §3): the mock's 44 pt Create button and small labels are below the 48 dp minimum — added an explicit hit-area-vs-visual task to Phase 1.
2. **Dark mode + contrast on glass** (accessibility rubric §4, `client/design.md` AppColors discipline): the mock is light-only; added a Phase 1 task requiring theme-aware tokens and contrast verification in both themes.
3. **Blur performance + stack growth** (performance rubric §4/§5): added a Phase 1 task to profile `BackdropFilter` on low-end Android and decide lazy vs. eager `IndexedStack` children.
4. **Nav-index blast radius**: deep links, notification routing, and `ToastHelper.showUndo` margins (see `client/undo.md`) all depend on current tab indexes/geometry — added an audit task to Phase 1.
5. **Search API hardening** (api_design rubric §2, security rubric §1/§4, `server/observability.md`): added per-group result caps (no unbounded repeated fields), `optional` field usage, `auth.RequireAuth()` + community-scoping authz test, and query-logging masking to Phase 3.
6. No contradictions found with documented product decisions: within-groups-only search honors the "no strangers / no discoverability" constraint (`users/priorities.md`); "no per-person ledger" is respected by the mock's aggregate-only group stats; the Library framing matches the "inventory anchors trust, not a lending marketplace" de-prioritization.

Docs consulted: `docs/llms.txt`, `client/architecture.md`, `client/conventions.md`, `client/modals.md`, `client/accessibility.md`, `client/design.md` (via architecture theming rules), `proto_conventions.md`, `users/priorities.md`, review rubrics for accessibility, performance, security, and api_design (`docs/reviews/*_review_prompt.md` → "Areas to Review").

## Related Issues
- #1895 — built the current pill + floating Create FAB; this issue supersedes that layout
- #2435 — Home tab redesign; the dock's Home destination and badge reuse its work
- #2568 — Directory tab; becomes the People destination
- #1802 — glass-surface migration for modals; the dock extends the same primitives to primary chrome
- #1967 — user feedback about the bottom menu being hard to discover; the always-floating capsule should resolve it
