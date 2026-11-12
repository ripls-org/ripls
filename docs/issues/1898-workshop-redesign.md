# Issue #1898: Workshop: community carousel (+ / Everything / per-community), replace 'actions' metric with people count, metric detail slide-in

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1898
**Filed:** 2026-05-13 | **Last activity:** 2026-05-13
**Labels:** none | **Assignees:** unassigned
**Context docs:** [docs/cowork/mocks/simple/new_workshop.png](../cowork/mocks/simple/new_workshop.png), [docs/workshop.md](../workshop.md)

## Problem Statement

After #1895 the hamburger and its community-filter UI go away, leaving
the Workshop screen without a way to scope by community. This issue
adds an on-screen **community carousel** at the top of the Workshop
that owns per-community selection (the only surface that does, now
that context elsewhere is portfolio-wide).

Three discrete UI changes:

1. **Community carousel** at the very top of the Workshop screen.
   Entries: a leading "+" button, an "Everything" pill, and one pill
   per community the user belongs to. Selecting an entry filters the
   hero, ticker, and postcards to that scope.
2. **Ticker swap**: the "actions" cell becomes a **unique people
   count** cell. Other two cells (saved $ / CO₂) are unchanged.
3. **Metric detail slide-in**: each ticker cell and the hero open a
   detail screen as a slide-in from the right.

The hero / ticker / postcard payload pipeline (`GetWorkshopBrief`,
`BriefRanker`, the detector chain — see [docs/workshop.md](../workshop.md))
is **unchanged**. This is presentation + scope-selection wiring on
top of the existing payload, plus one new server-side field for the
people count.

The carousel needs its own selection state, **separate** from the
existing [`selectedCommunityProvider`](../../app/lib/services/providers/community_providers.dart).
That provider is read by `SearchNotifier` (#1896) and
`inboxFlatItemsProvider` (#1897), among others; #1895 Phase 4
neuters its setter to always-`null` so those surfaces see the full
portfolio. If #1898 *also* drove that provider, a workshop carousel
selection would bleed into Feed search and Inbox filtering — Feed
results would silently scope to the workshop-selected community,
Inbox would hide items from other communities. #1898 introduces a
dedicated `workshopCommunityProvider` that the carousel and the
`WorkshopNotifier` use exclusively; `selectedCommunityProvider`
stays neutered per #1895.

## Context from Comments

No comments. The issue body, [docs/workshop.md](../workshop.md), and
[docs/cowork/mocks/simple/new_workshop.png](../cowork/mocks/simple/new_workshop.png)
are the specification.

## Audit Notes (2026-05-12)

Audited against [docs/client/architecture.md](../client/architecture.md),
[docs/client/caching.md](../client/caching.md),
[docs/client/modals.md](../client/modals.md),
[docs/client/accessibility.md](../client/accessibility.md),
[docs/client/i18n.md](../client/i18n.md),
[docs/client/testing.md](../client/testing.md),
[docs/client/the_feed.md](../client/the_feed.md),
[docs/workshop.md](../workshop.md),
[docs/server/architecture.md](../server/architecture.md), and
[docs/server/observability.md](../server/observability.md).

The plan aligns well with established patterns. Folded-in
corrections:

- **Carousel uses a dedicated `workshopCommunityProvider`, not the
  existing `selectedCommunityProvider`.** Initial draft proposed
  reusing `selectedCommunityProvider` and re-enabling its
  `setSelectedCommunity` non-null path. That would have leaked
  workshop carousel selections into `SearchNotifier` (#1896) and
  `inboxFlatItemsProvider` (#1897), silently scoping search and
  inbox to the workshop's selected community. Phase 1 now creates
  a fresh `workshopCommunityProvider` (selection-id only — derives
  the communities list from the existing provider's loaded data).
  `selectedCommunityProvider` stays neutered per #1895 Phase 4 —
  no assert revert needed.

- **`ScrollController` disposal added explicitly.** The carousel
  uses a `ScrollController` keyed to the selected id. Per
  [architecture.md "Resource Cleanup"](../client/architecture.md#resource-cleanup),
  the controller must be disposed when the widget is removed. The
  carousel widget becomes a `ConsumerStatefulWidget` so it has a
  `dispose()` lifecycle method; the controller is created in
  `initState()` and disposed in `dispose()`. Phase 1's carousel task
  now spells this out.
- **`WorkshopNotifier` disposal safety is out-of-scope.** #1898
  doesn't modify the Notifier body — only switches its `watch`
  source from `selectedCommunityProvider` to
  `workshopEnabledCommunityIdsProvider` (one-line change). Whatever
  async-safety patterns the Notifier uses today (`ref.mounted`,
  `SafeNotifierMixin`, AsyncNotifier auto-dispose) carry through.
  No new disposal tests needed for the Notifier itself.
- **Server error wrapping for the new helper.** Per [server
  architecture.md "Error wrapping"](../server/architecture.md#error-wrapping),
  the new `countUniquePeople` helper added in Phase 2 returns errors
  to its caller (`GetWorkshopBrief` handler), which already wraps
  storage errors via `connecterr.Internal`. The helper itself does
  NOT call `connect.NewError` — it returns plain Go errors. Phase 2's
  task list makes this explicit so the implementer doesn't
  accidentally introduce a bare `connect.CodeInternal` and trip
  `scripts/check_codeinternal.js`.
- **Cache behavior verified.** [caching.md](../client/caching.md)'s
  Stash + `CacheService` pattern caches the brief under
  `workshop_brief` namespace, keyed by sorted community ids per
  [workshop.md](../workshop.md). Switching between "Everything" and
  a specific community produces distinct cache entries (the keys
  differ); each is memory-only and LRU-bounded. No new cache
  namespaces or invalidation paths needed for #1898.
- **Server observability stance confirmed.** [observability.md](../server/observability.md)
  doesn't require new log lines for an internal computation helper.
  The existing `GetWorkshopBrief` request log covers Phase 2's
  changes. The audit confirms the plan's "don't preemptively log"
  posture.

## Codebase Survey

- **Workshop screen entry** — [workshop_screen.dart:27](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L27).
  Composes `FloatingHeader` (line 61, with `showMenuIcon: true` —
  #1895 drops this) + `WorkshopTopStrip` (218) + `WorkshopHero` (220)
  + `WorkshopTicker` (224) + postcards (236). The carousel slots
  between `FloatingHeader` and `WorkshopTopStrip`.
- **Community selection state** — [community_providers.dart:111](../../app/lib/services/providers/community_providers.dart#L111).
  `SelectedCommunityState` carries `selectedCommunityId: String?`,
  `communities: List<CommunityItem>`, and derived getters
  `enabledCommunityIdList` (line 136) + `enabledCommunities`
  (line 146). The Workshop screen reads `enabledCommunityIdList` at
  [workshop_screen.dart:160](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L160);
  the `WorkshopNotifier` watches the provider at
  [workshop_view_model.dart:46-47](../../app/lib/presentation/viewmodels/workshop_view_model.dart#L46-L47)
  and refetches `getBrief(communityIds)` on selection change.

  **Cross-surface coupling — important.** This same provider is
  read by `SearchNotifier` ([#1896](../issues/1896-feed-search.md))
  and `inboxFlatItemsProvider` ([#1897](../issues/1897-inbox-search-filters.md)).
  Per [#1895](1895-nav-restructure.md) Phase 4, its setter is
  neutered to `null` so those surfaces see the full portfolio.
  #1898 must **not** reuse this provider for the carousel — doing
  so would leak workshop selections into Feed/Inbox. The plan
  introduces a dedicated `workshopCommunityProvider`; the
  `WorkshopNotifier`'s `watch` switches to the new provider in
  Phase 1.
- **WorkshopTopStrip + roster** — [workshop_home_layout.dart:28](../../app/lib/presentation/widgets/workshop/workshop_home_layout.dart#L28)
  dedupes members by user id across enabled communities. The roster
  sheet — [workshop_roster_sheet.dart:29](../../app/lib/presentation/screens/workshop/workshop_roster_sheet.dart#L29)
  — is opened via `NavigationHelpers.pushScreen` (line 34) and uses
  `SwipeToCloseMixin` (line 47). It already renders the unique-member
  list with impact stats — **this is the natural destination for the
  new people-count metric tap.** Reuse, don't add a new
  `WorkshopPeopleDetailScreen`.
- **Hero, ticker, postcards** — [workshop_home_layout.dart:73 (Hero), :166 (Ticker)](../../app/lib/presentation/widgets/workshop/workshop_home_layout.dart#L73).
  Ticker exposes `onTapActs`, `onTapMoney`, `onTapCo2` callbacks
  (lines 177–184). Hero takes an optional `onTap`.
- **Detail screen push pattern (already slide-in from the right)** —
  [workshop_screen.dart:163-205](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L163-L205)
  pushes each detail screen via `NavigationHelpers.pushScreen()` with
  `useRootNavigator: true`. Per [navigation_helpers.dart:88-110](../../app/lib/core/utils/navigation_helpers.dart#L88-L110),
  this uses an identity-transition route while the destination's
  `SwipeToCloseMixin` provides the slide animation — i.e. the
  destination slides in from the right, the source stays still. This
  is the documented "slide-in from the right" pattern per
  [architecture.md "Navigation & Gestures"](../client/architecture.md#navigation--gestures).
  **The metric-detail-slide-in requirement is already met for the
  existing four cells.** The only new work here is wiring the new
  people-cell tap → roster sheet through the same pattern.
- **Detail screen constructor args** — each accepts `communityIds:
  List<String>`, `momentumRows: List<BriefCTARow>`, and a brief-total
  seed (`briefTotalActs`, `briefTotalHours`, `briefTotalUsd`,
  `briefCo2Pounds`). All four use `SwipeToCloseMixin`. See
  [WorkshopActsDetailScreen:77-92](../../app/lib/presentation/screens/workshop/workshop_acts_detail_screen.dart#L77-L92),
  [WorkshopTimeDetailScreen:71-87](../../app/lib/presentation/screens/workshop/workshop_time_detail_screen.dart#L71-L87),
  [WorkshopMoneyDetailScreen:68-84](../../app/lib/presentation/screens/workshop/workshop_money_detail_screen.dart#L68-L84),
  [WorkshopCo2DetailScreen:78-94](../../app/lib/presentation/screens/workshop/workshop_co2_detail_screen.dart#L78-L94).
- **`BriefPayload` proto** — [workshop_service.proto:142-193](../../proto/ripls/api/workshop_service.proto#L142-L193).
  Fields: `since_unix_sec`, `hours_together`, `events_count`,
  `replaced_cost_usd`, `co2_avoided_pounds`, `acts_count`. **No
  `unique_people_count` field** — Phase 2 adds it.
- **People-count source today** — *none on the brief*. The per-
  community `MemberCount` exists in [impact.pb.go:46](../../server/gen/ripls/api/impact.pb.go#L46)
  but isn't surfaced on `BriefPayload`. Client-side, the
  `WorkshopTopStrip` computes a unique people count by iterating
  `communityMembersProvider(id)` per circle and deduping by user id
  — but that's a roster-level computation, not a single scalar
  exposed for the ticker. Phase 2 lifts that dedupe logic to the
  server so the ticker has a stable numeral to render.
- **`acts_count` source** — server populates it from
  `totals.actsCount` at [brief.go:94](../../server/services/workshop/brief.go#L94),
  computed by `collectSeasonTotals` at [brief.go:215-256](../../server/services/workshop/brief.go#L215-L256)
  via `calculator.CountActs()`. Sum per community at
  [synthesis.go:174-177](../../server/services/workshop/synthesis.go#L174-L177).
- **Add-community flow** — [CommunityCreationModal.show()](../../app/lib/presentation/screens/communities/community_creation_modal.dart#L48),
  currently opened from [sidebar.dart:550](../../app/lib/presentation/screens/home/sidebar.dart#L550)
  (sidebar being deleted in #1895 Phase 4). The new carousel "+"
  button invokes the same entry point.
- **#1895 interaction** — #1895 Phase 4 sets
  `setSelectedCommunity` to `assert(communityId == null, ...)`. #1898
  re-enables non-null selection. See Resolved Decisions for sequencing.

## Potential Solutions

### Option A: Dedicated `workshopCommunityProvider` (recommended)

- **Approach:** Introduce a new provider, `workshopCommunityProvider`,
  scoped to the Workshop screen. Holds only the selected community
  id (`String?`); reads the communities list from the existing
  `selectedCommunityProvider`'s loaded `communities` (the user's
  membership data — that part of the existing provider is still the
  source of truth, only its `selectedCommunityId` setter is
  neutered). The carousel reads/writes the new provider; the
  `WorkshopNotifier`'s `watch` switches from
  `selectedCommunityProvider` to `workshopCommunityProvider`.
  `selectedCommunityProvider.setSelectedCommunity` stays neutered
  (always-`null`) per #1895 Phase 4 — no revert needed.
- **Pros:** No cross-surface bleed. Feed search (#1896) and inbox
  filtering (#1897) keep reading `selectedCommunityProvider` and
  keep seeing the full portfolio. Workshop selection is local to
  the Workshop screen. Naming reflects scope.
- **Cons:** One more provider in the codebase. The
  `WorkshopNotifier` migrates its `watch` (a one-line change).
  Slightly more code than the rejected Option B's "reuse existing
  provider" path.
- **Effort:** Small (Phase 1).

### Option B: Carousel drives existing `selectedCommunityProvider` (rejected)

- **Approach:** Reuse `selectedCommunityProvider` as the carousel's
  backing state — re-enable the non-null setter that #1895 Phase 4
  neutered. No new provider.
- **Pros:** Slightly less code. Existing `WorkshopNotifier.watch` is
  already pointed at the right provider.
- **Cons:** **Leaks workshop carousel selection into Feed search
  and Inbox filtering.** `SearchNotifier` (#1896) and
  `inboxFlatItemsProvider` (#1897) read
  `selectedCommunityProvider.effectiveCommunityIds`; with #1898
  setting a non-null id, those surfaces silently scope to the
  workshop-selected community. Selecting "Boulder BC" on the
  Workshop would make a Feed search return only Boulder BC results.
  Contradicts the design direction "context is the entire portfolio
  everywhere except the Workshop."
- **Effort:** Small, but ships a regression.

### Option C: Server-side scope filter via `GetWorkshopBrief` parameter (rejected)

- **Approach:** Add a server-side scope argument that the carousel
  drives directly via the repository, bypassing
  `selectedCommunityProvider`.
- **Pros:** Decouples the Workshop carousel from any global provider.
- **Cons:** `GetWorkshopBrief` already accepts `circle_ids[]`. The
  client-side provider just turns a single-select pill into that
  list. There's no server-side simplification to be had; this is
  pure indirection.
- **Effort:** Medium-Large. Rejected.

## Recommendation

**Option A — dedicated `workshopCommunityProvider`.** The small
cost of one extra provider buys clean scope separation: workshop
selection stays in the Workshop, Feed/Inbox keep seeing the full
portfolio per the design direction. `selectedCommunityProvider`
stays neutered by #1895 Phase 4 with no special-casing.

This also simplifies the phase-ordering question — #1898 no longer
needs to coordinate with #1895 Phase 4 (since #1898 doesn't touch
the neutered setter at all). Either issue can land first.

## Resolved Decisions (2026-05-12 issue review)

All open questions resolved by accepting the recommendations.
Decisions are folded into the Implementation Plan below; recorded
here for the audit trail.

- **#1898 introduces its own `workshopCommunityProvider`** — does
  not reuse `selectedCommunityProvider`. Avoids leaking workshop
  carousel selections into Feed search (#1896) and Inbox filtering
  (#1897). `selectedCommunityProvider.setSelectedCommunity` stays
  neutered per #1895 Phase 4; #1898 doesn't touch it.
- **No coordinated phase ordering with #1895 Phase 4.** Because
  #1898 doesn't touch `selectedCommunityProvider`'s setter, the two
  issues are independent — either can land first.
- **People-count detail target: reuse `WorkshopRosterSheet`.**
  Already renders a unique-member list with impact stats; already
  uses the `NavigationHelpers.pushScreen` + `SwipeToCloseMixin`
  pattern. No new `WorkshopPeopleDetailScreen`.
- **Carousel is pinned** below the `FloatingHeader` (stable control
  band, doesn't scroll with content).
- **"Everything" and per-community pills share the same `Toggle`
  shape**, with per-entry leading icons: "Everything" uses a
  globe/earth glyph; per-community pills use
  `CommunityItem.iconEmoji` (or thumbnail).
- **"+" button is pinned at the front** of the carousel as a smaller
  circular button (not a full pill). Stays visible regardless of
  scroll position. Tap →
  `CommunityCreationModal.show(context, useRootNavigator: true)`.
- **Carousel auto-scrolls the selected pill into view** on selection
  change (e.g. after creating a new community). Uses a
  `ScrollController` keyed to the selected id; centers the selected
  pill with `accessibleDuration(context, const Duration(milliseconds: 200))`.
- **People count is server-computed.** Phase 2 adds
  `unique_people_count` to `BriefPayload`, populated by
  deduplicated membership aggregation across the requested
  communities. Server owns the truth (matches [the_feed.md](../client/the_feed.md)
  pattern); client just renders the numeral.
- **`acts_count` stays on the wire** after the ticker swap. Field
  remains populated server-side (cheap); future surfaces may use it.
  `WorkshopActsDetailScreen` also stays in the codebase, no longer
  reachable from the home ticker. Both flagged for the cleanup PR
  to delete if no surface adopts.
- **People count is unique across the selected scope** — deduped by
  user id. Matches the `WorkshopTopStrip` avatar semantic; avoids
  double-counting cross-community members.
- **People cell label is "PEOPLE"** — matches the existing
  ALL-CAPS ticker label family ("ACTS", "SAVED", "CO₂ AVOIDED"). New
  l10n key `workshopTickerPeopleLabel`.

## Implementation Plan

### Phase 1: Community carousel + selection wiring

**Goal:** Ship the carousel at the top of the Workshop screen, with
its own `workshopCommunityProvider` driving the selection state.
`WorkshopNotifier` watches the new provider and refetches the brief
on selection change. `selectedCommunityProvider` is **not** touched —
remains neutered per #1895 Phase 4.

**Tasks:**
- [ ] Add new l10n keys to **both** [app/lib/l10n/app_en.arb](../../app/lib/l10n/app_en.arb)
      **and** [app/lib/l10n/app_es.arb](../../app/lib/l10n/app_es.arb):
  - `workshopCarouselEverything` — "Everything".
  - `workshopCarouselAddCommunity` — short label for the "+" button
    (or none if icon-only — see a11y key below).
  - `a11yWorkshopCarouselAddCommunity` — `IconAction` semantic label
    for the "+" button ("Add a community").
  - `a11yWorkshopCarouselEntry` — parameterized `Toggle` semantic
    label for per-community entries ("{name}, select community").
  - `a11yWorkshopCarouselEverythingEntry` — `Toggle` semantic label
    for the "Everything" entry.

      Run `npm run generate`.
- [ ] **Create `workshopCommunityProvider`** at
      `app/lib/services/providers/workshop_community_provider.dart`
      (or in a sibling file under `app/lib/services/providers/`).
  - `NotifierProvider<WorkshopCommunityNotifier, String?>` holding
    just the selected community id (`null` = "Everything",
    portfolio-wide scope). No Freezed state class needed — the value
    is a single nullable string.
  - Methods: `select(String? communityId)` — writes
    `state = communityId`. That's it.
  - **Derived getter** (extension or sibling provider):
    `workshopEnabledCommunityIdsProvider` returns
    `List<String>` — when `state == null`, reads the full membership
    list from `selectedCommunityProvider`'s `communities` and
    returns all ids; when `state == <id>`, returns `[id]`. This is
    the value `WorkshopNotifier` consumes.
  - Stable durable header comment on the file:

      ```dart
      // Workshop-scoped community selection (#1898). Drives the
      // Workshop screen's community carousel and the WorkshopNotifier's
      // brief refetch. Does NOT propagate to Feed search, Inbox
      // filtering, or any other surface — those read
      // selectedCommunityProvider, which stays portfolio-wide.
      ```

      Per [CLAUDE.md "Go Code Comments"](../../CLAUDE.md#go-code-comments),
      issue numbers OK, no rollout-phase language.
- [ ] **Switch `WorkshopNotifier.build()`** to watch
      `workshopEnabledCommunityIdsProvider` instead of
      `selectedCommunityProvider`. Single-line change in
      [workshop_view_model.dart](../../app/lib/presentation/viewmodels/workshop_view_model.dart).
      The existing refetch-on-change wiring carries over — the
      Notifier doesn't know or care which provider it watches.
- [ ] **Switch `WorkshopTopStrip`** ([workshop_home_layout.dart:28](../../app/lib/presentation/widgets/workshop/workshop_home_layout.dart#L28))
      to read `workshopEnabledCommunityIdsProvider` instead of
      `selectedCommunityProvider`. The dedupe-by-user-id logic is
      unchanged; only the source-of-truth provider changes.
- [ ] Build the carousel widget at
      `app/lib/presentation/widgets/workshop/workshop_community_carousel.dart`.
      Implement as a [`ConsumerStatefulWidget`](https://riverpod.dev/docs/concepts/reading)
      so it has an `initState()` / `dispose()` lifecycle — required
      for the `ScrollController` (see below).
  - Horizontal scrollable row.
  - Entries in order:
    1. **"+" button** — circular `IconAction` (or `Tappable` wrapping
       a circular icon button) with `tooltip:
       context.l10n.a11yWorkshopCarouselAddCommunity`. Pinned at the
       front (does not scroll). On tap:
       `CommunityCreationModal.show(context, useRootNavigator: true)`.
    2. **"Everything" pill** — [`Toggle`](../../app/lib/presentation/widgets/accessibility/)
       with `selected: ref.watch(workshopCommunityProvider) == null`
       and `semanticsLabel: context.l10n.a11yWorkshopCarouselEverythingEntry`.
       Label: `context.l10n.workshopCarouselEverything`. Leading icon:
       a globe/earth glyph (use the existing icon set).
       On tap: `ref.read(workshopCommunityProvider.notifier).select(null);`.
    3. **Per-community pills** — one per entry in the user's
       membership list (read from `selectedCommunityProvider`'s
       `enabledCommunities` — that provider still loads and exposes
       the user's communities; only its `selectedCommunityId` setter
       is neutered). Each pill is a `Toggle` with
       `selected: ref.watch(workshopCommunityProvider) == community.id`
       and a parameterized `semanticsLabel` from
       `context.l10n.a11yWorkshopCarouselEntry(community.name)`.
       Leading icon: the community's thumbnail / emoji from
       `CommunityItem`. Label: `community.name`. On tap:
       `ref.read(workshopCommunityProvider.notifier).select(community.id);`.
  - Use a `ScrollController` keyed to the selected id. When selection
    changes, `controller.animateTo(...)` so the selected pill is
    centered. Wrap the animation duration in
    `accessibleDuration(context, const Duration(milliseconds: 200))`
    per [accessibility.md "When to reach for which primitive"](../client/accessibility.md#when-to-reach-for-which-primitive).
  - **`ScrollController` lifecycle** per [architecture.md "Resource
    Cleanup"](../client/architecture.md#resource-cleanup): create in
    `initState()`, dispose in `dispose()`. A leaked
    `ScrollController` retains its `ScrollPosition` and any attached
    listeners — small leak per screen visit, but adds up. The
    `ConsumerStatefulWidget` form makes this straightforward.
  - **React to selection changes** with `ref.listen<String?>(workshopCommunityProvider, …)`
    (not `ref.watch`) in the build method, so external selection
    changes (e.g. created-community → newly-selected) trigger
    `controller.animateTo(...)` exactly once per change rather than
    re-running on every rebuild. Match the `ref.listen` pattern used
    elsewhere in the codebase.
  - Theme-aware colors throughout (`AppColors.*(context)`); no
    `Colors.white` / hardcoded hex per [architecture.md "Colors and
    Theming"](../client/architecture.md#colors-and-theming). Active
    pill: dark fill + cream text; inactive: muted stone fill +
    secondary text (same family as the inbox filter pills in #1897).
- [ ] Update [workshop_screen.dart](../../app/lib/presentation/screens/workshop/workshop_screen.dart)
      to slot `WorkshopCommunityCarousel` between the `FloatingHeader`
      and `WorkshopTopStrip`. The carousel watches
      `workshopCommunityProvider` directly; no extra prop drilling
      needed.
- [ ] **Verify the brief refetch works.** After the Phase 1
      `watch`-switch, `WorkshopNotifier` watches
      `workshopEnabledCommunityIdsProvider` and re-runs
      `getBrief(communityIds)` on change. Manual test: tap a
      per-community pill, confirm the hero/ticker numerals update.
      If integration tests show stale state, the
      `WorkshopRepository.invalidateAll()` call inside
      `WorkshopNotifier.refresh()` may need a sibling call on
      selection change.
- [ ] **Verify no cross-surface bleed.** Manual test: select a
      specific community in the Workshop carousel, navigate to the
      Feed, kick off a search. The search must return results from
      the full portfolio, not scoped to the workshop-selected
      community. Same check for the Inbox list. (Automated coverage
      lives in the ViewModel tests below.)
- [ ] **Tests** per [docs/client/testing.md](../client/testing.md)
      ViewModel-first focus:
  - ViewModel test for `WorkshopCommunityNotifier`:
    `select(null)` → state is `null`; `select("abc")` → state is
    `"abc"`; round-trip back to `null` works.
  - ViewModel test for `workshopEnabledCommunityIdsProvider`: when
    workshop state is `null`, returns the full membership list from
    `selectedCommunityProvider`; when state is `"abc"`, returns
    `["abc"]`.
  - **Isolation test**: with `workshopCommunityProvider` set to a
    specific id, read `selectedCommunityProvider.effectiveCommunityIds`
    in a `ProviderContainer` and confirm it still returns the full
    portfolio — i.e. workshop selection does NOT bleed through.
    This is the regression guard that justifies the new provider.
  - Widget-level dispatch test for `WorkshopCommunityCarousel`:
    tapping "+" calls `CommunityCreationModal.show`; tapping
    "Everything" calls `select(null)`; tapping a per-community pill
    calls `select(community.id)`; the active pill has
    `selected: true` Toggle semantics.
- [ ] Run `npm run lint:dart:a11y` and `npm run lint:dart:i18n` to
      confirm clean.

**Validates:** The carousel renders, the "+" button opens
`CommunityCreationModal`, "Everything" reverts to portfolio scope,
per-community taps filter the brief. Hero / ticker / postcards
re-render with scope-correct data.

### Phase 2: People-count proto field + server population

**Goal:** Ship the new `unique_people_count` field on `BriefPayload`,
populated server-side via deduplicated membership across the
requested communities. No client UI change yet.

**Tasks:**
- [ ] **Proto changes** in [proto/ripls/api/workshop_service.proto](../../proto/ripls/api/workshop_service.proto):
  - Add `optional int32 unique_people_count = N;` to `BriefPayload`
    (wire-compatible per [CLAUDE.md "Proto Field Conventions"](../../CLAUDE.md#proto-field-conventions)).
  - Field comment per [CLAUDE.md "Proto Comments"](../../CLAUDE.md#proto-comments)
    — describe semantics only. Example acceptable phrasing:

      ```proto
      // Total unique people across the communities included in this
      // brief. A person who belongs to multiple included communities
      // is counted once.
      optional int32 unique_people_count = N;
      ```

      Do NOT write: "Computed by `collectSeasonTotals`",
      "Powers the Workshop ticker's PEOPLE cell", "Set in
      brief.go". Describe what the number means to a downstream
      client.
- [ ] Regenerate: `npm run generate`.
- [ ] **Server: compute the count** in [brief.go](../../server/services/workshop/brief.go).
  - Add a helper, e.g. `countUniquePeople(ctx, store, communityIDs)
    (int32, error)`, that batch-fetches members for the requested
    community ids in a single SQL `IN` query (no per-community loop
    — per [CLAUDE.md "SQL Efficiency"](../../CLAUDE.md#sql-efficiency)),
    dedupes by user id, and returns the count. Probably belongs
    next to `collectSeasonTotals`.
  - **Error wrapping:** the helper returns **plain Go errors**, not
    `connect.Error`. Per [server architecture.md "Error wrapping"](../server/architecture.md#error-wrapping),
    `connect.CodeInternal` errors must use `connecterr.Internal(ctx,
    op, err, kv...)` — but that wrapping lives in the
    `GetWorkshopBrief` RPC handler, which already wraps storage
    errors. `countUniquePeople` itself just bubbles the error up.
    Do **not** call `connect.NewError` inside the helper —
    `scripts/check_codeinternal.js` will trip on a bare
    `connect.CodeInternal` outside the RPC handler.
  - Populate `BriefPayload.unique_people_count` from the helper at
    the same point `acts_count` is set (around [brief.go:94](../../server/services/workshop/brief.go#L94)).
- [ ] **Server tests** at [server/services/workshop/brief_test.go](../../server/services/workshop/brief_test.go)
      (or sibling file):
  - Single community with 5 members → `unique_people_count = 5`.
  - Two communities with 3 members each, 1 shared →
    `unique_people_count = 5` (not 6).
  - Empty community set → `unique_people_count = 0`.
  - **N+1 guard**: wrap the multi-community test in
    `storage.AssertMaxQueries(t, ctx, max, fn)` per
    [CLAUDE.md "SQL Efficiency"](../../CLAUDE.md#sql-efficiency).
  - Run `npm run build` and `go test ./server/services/workshop/...`.
- [ ] **Server observability** per [docs/server/observability.md](../server/observability.md):
      no new RPC, no new log lines required. The existing
      `GetWorkshopBrief` request log covers this. Don't add fields
      just because we're touching the file.

**Validates:** Wire carries `unique_people_count`. Server tests
prove the dedupe logic. The client ignores the new field for now —
no user-visible change.

### Phase 3: Swap the acts ticker cell for people, wire to roster sheet

**Goal:** Replace the leftmost ticker cell's content (acts → people)
and route taps to the existing `WorkshopRosterSheet`. After this
phase the user-visible Workshop matches the mock end-to-end.

**Tasks:**
- [ ] Add l10n keys (both ARB files):
  - `workshopTickerPeopleLabel` — "PEOPLE" (matches the existing
    ALL-CAPS ticker label family).
  - `a11yWorkshopTickerPeople` — `Tappable` / cell semantic label
    parameterized with count ("{count} people, tap to view roster").

      Run `npm run generate`.
- [ ] Update [workshop_home_layout.dart](../../app/lib/presentation/widgets/workshop/workshop_home_layout.dart):
  - In `WorkshopTicker`, rename the `onTapActs` callback to
    `onTapPeople` (or add a new `onTapPeople` and remove `onTapActs`
    once no callers remain). Bind the leftmost cell to
    `brief.uniquePeopleCount` instead of
    `brief.actsCount ?? brief.eventsCount` (around
    [workshop_home_layout.dart:191-200](../../app/lib/presentation/widgets/workshop/workshop_home_layout.dart#L191-L200)).
  - Cell label: `context.l10n.workshopTickerPeopleLabel`. Cell
    semantic label: `context.l10n.a11yWorkshopTickerPeople(count)`.
  - Empty / unset handling: if
    `brief.uniquePeopleCount == null || 0`, hide the cell or render
    "—". Confirm against the existing fallback behavior of the
    acts cell.
- [ ] Wire the new tap target in [workshop_screen.dart](../../app/lib/presentation/screens/workshop/workshop_screen.dart):
  - Replace `openActsDetail()` with `openPeopleDetail()` (or
    similar) that pushes `WorkshopRosterSheet` via the same
    `NavigationHelpers.pushScreen(..., useRootNavigator: true,
    routeName: ...)` pattern the existing detail-screen pushes use
    (around [workshop_screen.dart:163-194](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L163-L194)).
  - The roster sheet already accepts the community-id scope it needs
    via its constructor; pass the same `enabledCommunityIdList` the
    other detail screens get.
- [ ] **Verify slide-in-from-right behavior.** Per the survey, the
      existing `NavigationHelpers.pushScreen()` + `SwipeToCloseMixin`
      pattern *is* slide-in from the right. `WorkshopRosterSheet`
      already uses `SwipeToCloseMixin`. No new animation code
      required. Visually confirm the hero / acts / money / co₂ / new
      people path all animate identically.
- [ ] **Acts cell cleanup:**
  - The `acts_count` field stays on the wire (back-compat;
    cheaply computed server-side; future surfaces may use).
  - `WorkshopActsDetailScreen` stays in the codebase, no longer
    reachable from the home ticker. Add a stable durable comment
    at the top:

      ```dart
      // No longer reachable from the Workshop ticker after #1898
      // (the acts cell was replaced by people-count). Retained
      // because other future surfaces may push this screen.
      ```

      Per [CLAUDE.md "Go Code Comments"](../../CLAUDE.md#go-code-comments)
      — issue numbers allowed, no rollout-phase language. Flag for
      the post-#1895/#1896/#1897/#1898 cleanup PR to delete if no
      surface adopts it.
- [ ] **Tests:**
  - ViewModel-style test on the `WorkshopTicker` widget: render
    with a `BriefPayload` carrying `uniquePeopleCount: 7` → leftmost
    cell shows "7" and label "PEOPLE"; tapping it triggers
    `onTapPeople`.
  - Widget-level dispatch test for the new
    `openPeopleDetail()` handler: tap dispatches
    `NavigationHelpers.pushScreen` with `WorkshopRosterSheet` and
    the correct `enabledCommunityIdList`.
  - Snapshot or manual visual check against `new_workshop.png`
    in both light and dark modes (theme-aware colors).
- [ ] Run `npm run lint:dart:a11y` / `lint:dart:i18n` /
      `flutter test --reporter expanded`.

**Validates:** The Workshop screen renders end-to-end like
`new_workshop.png`: carousel at the top, hero, ticker with PEOPLE /
SAVED / CO₂, postcards. All metric taps slide in their detail screen
from the right.

## Related Issues

- #1895 — Bottom + floating nav restructure. **Hard dependency.** The
  carousel only makes sense after the hamburger is gone (the carousel
  *replaces* the sidebar's community filter on the Workshop surface).
  See Resolved Decisions for sequencing.
- #1896 — Feed search redesign. No direct dependency; same
  nav-restructure cohort.
- #1897 — Inbox search + filter pills. No direct dependency; same
  nav-restructure cohort. The pill styling (`Toggle` primitive,
  active-fill family) can be aligned with #1897's `InboxFilterRow`
  if both ship in proximity.
- (TBD post-#1895/#1896/#1897/#1898) — Dead-code cleanup PR. Adds to
  its sweep list:
  - Collapse RPC call sites passing
    `selectedCommunityProvider.effectiveCommunityIds` now that the
    value is permanently the full portfolio; possibly delete the
    provider's neutered `selectedCommunityId` field entirely. The
    new `workshopCommunityProvider` stays — it's the post-#1898
    canonical surface.
  - Delete `WorkshopActsDetailScreen` if no surface adopts it.
  - Remove `BriefPayload.acts_count` if no surface adopts it.
