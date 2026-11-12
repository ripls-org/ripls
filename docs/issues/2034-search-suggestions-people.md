# Issue #2034: Improve search suggestions: add top categories, communities, and people search

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2034
**Filed:** 2026-05-19 | **Last activity:** 2026-05-19
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

> **Audit applied 2026-05-19** against `docs/client/{architecture,caching,i18n,accessibility,testing}.md` and `docs/server/{architecture,observability}.md`. Material changes from the initial draft: per-entry TTL removed (Phase 1 caching only supports the global `CACHE_TTL_MINUTES`); ViewModel layer added before the widget; widget tests downgraded to ViewModel + repository tests per the test pyramid; `Tappable` + `CachedMediaImage(semanticsLabel:)` + ARB `a11y*` keys spelled out; `SharedCommunityRef` reused instead of inventing a new `CommunityRef`; `FilterActiveMemberCommunities` clarified to filter-silently, not reject; observability fields expanded per `observability.md`; `oneof` field-8 reuse made explicit; navigation routed through `NavigationHelpers`.
>
> **Scope expanded 2026-05-19** to include pill-activation UX fixes (stationary text, hidden avatar, X-close reuse) as Phase 4. Originally filed as a separate issue (#2036) which has been closed in favor of this consolidated plan.
>
> **Open Questions resolved 2026-05-19.** User directed: (1) community chips filter the search instead of opening `CommunityScreen` — all suggestion chips behave uniformly as filters; (2) `USER` is included in default search itemTypes and rendered as a `SearchResultRow` line item with a circular avatar and the name as title. Remaining open questions adopt their audit-stage recommendations as final.

## Problem Statement

The idle-state search affordance ([search_nearby_pill.dart](app/lib/presentation/widgets/search/search_nearby_pill.dart)) currently renders a static `_SuggestionsPanel` with four hardcoded localized "quick chips" (Help Nearby, Tools to Borrow, Events This Week, People in Circles) plus a recent searches list. The chips are the same for every user and every community, so they don't reflect what a user's network is actually known for or which communities matter to them.

The issue asks for three things (the third was added after initial filing — captured here rather than as a separate issue):

1. **Personalize the suggestions panel** by adding two new sections:
   - Top 5 "known for" categories drawn from the current user's portfolio.
   - Top 5 community names the user belongs to.
2. **Surface people as a search result type** end-to-end, so a person tile can be tapped to open `user_screen`.
3. **Fix the pill activation transition** so the search bar is visually stable on tap. Three sub-issues to address (see [search_nearby_pill.dart:271-329](app/lib/presentation/widgets/search/search_nearby_pill.dart#L271-L329)):
   - Input text jumps vertically a few pixels when the idle `Text` is swapped for the active `TextField` — should stay in the exact same baseline as the placeholder.
   - The `FloatingHeader` avatar is still visible during the active state — should be hidden while active and restored on dismiss.
   - The flat "Cancel" text label sits outside the pill frame, narrowing the input. Replace it with the same circular `Icons.close` X used by [`_CloseSearchButton` in discover_screen.dart:610-639](app/lib/presentation/screens/discover/discover_screen.dart#L610-L639), placed inside the pill frame so the input width doesn't change between idle and active.

Note on the "people search" half: the server already implements user search (`SEARCH_ITEM_TYPE_USER` is wired through [search_service.proto:67-74](proto/ripls/api/search_service.proto#L67-L74) and `searchUsers()` in [server/services/search/service.go:381-418](server/services/search/service.go#L381-L418)). The gap is on the client — [SearchResultRow._dispatch()](app/lib/presentation/widgets/search/search_result_row.dart#L34-L57) has no case for `SEARCH_ITEM_TYPE_USER`, and the discover repository defaults exclude `USER` from `itemTypes`. So "implement people search" is really "wire the existing server capability through the Flutter layer."

## Context from Comments

There are no comments on this issue. It was just filed. The acceptance criteria in the body are unambiguous, but a few product-shaped questions are unresolved (see Open Questions).

## Potential Solutions

### Option A: Two new server RPCs (per-section endpoints)

- **Approach:** Add `GetTopKnownForCategories` and `GetTopCommunitiesForUser` RPCs to a service (search or user). The Flutter `_SuggestionsPanel` calls each independently and renders them as horizontally-scrolling chip rows. People search is handled by setting `itemTypes: [GEAR, REQUEST, EXPERIENCE, USER]` on the existing `Search` RPC and adding a `UserDiscoverItem` case to the client.
- **Pros:** Tight contracts per surface. Easy to cache per-section with different TTLs. No bloat on the `Search` RPC.
- **Cons:** Two extra round-trips on every search-panel open. Two new RPCs to maintain, version, and test. Per-RPC caching is more code than per-response caching.
- **Effort:** medium

### Option B: One new `GetSearchSuggestions` RPC + wire people through existing Search

- **Approach:** Add a single `GetSearchSuggestions(GetSearchSuggestionsRequest) → GetSearchSuggestionsResponse` RPC on `SearchService` that returns `repeated string top_known_for_categories` and `repeated CommunityRef top_communities` in one shot. The Flutter panel calls this once on mount. People search reuses the existing `Search` RPC by including `SEARCH_ITEM_TYPE_USER` in `itemTypes`, plus a client-side `UserDiscoverItem`/`SearchResultRow` case.
- **Pros:** One round-trip for the suggestions panel. Suggestions stays a coherent concept in the API. Cleanly separates "what to suggest before typing" from "what to return after typing." Wire-compatible to extend later (add more suggestion fields).
- **Cons:** A new RPC, but small. Both sections share a TTL.
- **Effort:** medium

### Option C: Bundle suggestions into the existing `Search` response

- **Approach:** Extend `SearchResponse` with optional `repeated string top_known_for_categories` and `repeated CommunityRef top_communities`. Clients call `Search` with an empty query at panel-open time and use the suggestion fields. People search is wired through `itemTypes`.
- **Pros:** No new RPC. Minimal proto churn.
- **Cons:** Conflates two surfaces: the "results" RPC now also returns "suggestions" payload even for typed queries (wasted bytes), and the empty-query path is overloaded. Cache key for `Search` is already complex (query, strategy, community_ids, lat/lon, etc.) — bolting suggestions on means the suggestion payload pays the price of being invalidated for the wrong reasons.
- **Effort:** small

## Recommendation

**Option B.** A dedicated `GetSearchSuggestions` RPC keeps the suggestion surface independent from the query surface, gives the client a single round-trip to populate the panel, and matches the existing project convention of one request/response per RPC. People search uses the already-working server pipeline — the client just needs a `UserDiscoverItem` variant and a `SEARCH_ITEM_TYPE_USER` case in `SearchResultRow._dispatch()`.

The known-for computation should reuse [known_for.Derive()](server/known_for/known_for.go#L102-L167) with `ModePerUser`, which already does the aggregation across loans, hosted experiences, and fulfilled requests, normalizes category strings, and respects suppression. Top 5 = first five of its sorted output.

Top communities should rank by `CommunityUser.created_at_unix_sec` descending (most recently joined first) for v1 — it's the only ranking signal available without new instrumentation, and it works fine until we have data to do better.

## Resolved Decisions

All open questions resolved 2026-05-19. Two were directed by the user (chip behavior + people in default search); the rest carry the audit-stage recommendations forward as final.

- **Section order in the panel.** Render top-down: (1) Top communities, (2) Top known-for categories, (3) Recent searches, (4) static fallback chips. Empty sections are hidden silently — no empty headers.
- **Tapping a "known for" chip.** Seeds the search field with the category string and runs the search (`onPick(category)`).
- **Tapping a community chip — filter, not navigate.** Calls `onPick(communityName)` to populate the search input and run a filtered search. **Every chip on the suggestions panel — communities, categories, fallback chips — behaves as a filter that seeds the search query.** The panel is never a launcher for screens. (Decision overrides the audit-stage recommendation to push `CommunityScreen`.)
- **`Search` itemTypes default includes `USER`.** People are returned as line items alongside gear/requests/events. Render with the same `SearchResultRow` shape as other items, **except the avatar is round** (circular `CachedMediaImage`, not the standard rounded-square thumbnail) and the **title is the user's name** (no subtitle/meta required for v1). Tapping a person row opens `user_screen` via `ContentViewHelpers.openUserScreen`.
- **Top community ranking.** Most-recently-joined ordering for v1. Revisit with per-community activity metrics later — follow-up only.
- **People in the suggestions panel.** No top-5-people section in v1 — issue scope is categories + communities only. People appear in *typed* search results via `SEARCH_ITEM_TYPE_USER`.
- **Caching for suggestions.** Dedicated `'search_suggestions'` `CacheService` namespace keyed on `userId:sortedCommunityIds`. TTL inherits from the global `CACHE_TTL_MINUTES`. Invalidate via `refreshSuggestions()` from community-join/leave + loan/experience/request completion mutations.

## Implementation Plan

### Phase 1: Wire people search through the Flutter client

**Goal:** Users appear in search results when typed, tapping opens `user_screen`. This unlocks the "people search" half of the issue using only client changes — the server is already there.

**Tasks:**
- [ ] Add `UserDiscoverItem` subclass to [app/lib/presentation/models/discover_item.dart](app/lib/presentation/models/discover_item.dart) wrapping the **existing** `api.User` carried in `SearchResultItem.item` oneof field 8. **No proto changes in Phase 1** — `SEARCH_ITEM_TYPE_USER` and `User user = 8` already exist in [search_service.proto:73,100](proto/ripls/api/search_service.proto#L73-L100). Surface `id`, `name`, `primaryMediaId` (from `User.media_id`), and `compositeScore`/`distanceMeters` from the parent `SearchResultItem`.
- [ ] Extend `SearchDiscoverItem.when<T>()` (in the same file) to handle the `SEARCH_ITEM_TYPE_USER` case and surface the embedded `User`.
- [ ] Add a `SEARCH_ITEM_TYPE_USER` branch to [search_result_row.dart](app/lib/presentation/widgets/search/search_result_row.dart#L34-L57) that calls `ContentViewHelpers.openUserScreen(context, userId)` (which itself uses `NavigationHelpers.pushScreen` per [content_view_helpers.dart:115-133](app/lib/presentation/widgets/content/content_view_helpers.dart#L115-L133)). Add an `onUserTap` callback parameter to `SearchResultRow` (matching the existing per-type callback pattern) so the parent decides what to do on tap; default it in `DiscoverBottomSheet` to call `ContentViewHelpers.openUserScreen`. *(If #1009 is open at implementation time, ping that branch before changing the `'profile'` route name. Otherwise no coordination needed.)*
- [ ] Render the user tile **with the same `SearchResultRow` shape as gear/request/experience tiles — same 56×56 leading thumbnail slot, same bold title, same trailing chevron — but two differences**:
    1. **Avatar is circular** (`ClipOval` around the `CachedMediaImage`, not the standard rounded-square `ClipRRect` other types use). The thumbnail still occupies the 56×56 slot so list alignment is uniform.
    2. **Title is the user's name.** No subtitle / meta line for v1 (skip the owner-name · distance pattern other types use — for a user, the name IS the title).
  - A11y per [accessibility.md](../client/accessibility.md): wrap the row's tap surface in `Tappable(semanticsLabel: context.l10n.a11yOpenUserProfile(userName), onTap: …)`. Avatar uses `CachedMediaImage(semanticsLabel: context.l10n.searchResultUserAvatarLabel(userName), cacheKey: mediaUrl.cacheKey)` — never `Image.network` for user media (per CLAUDE.md §Media Display). All visible strings go through ARB (`context.l10n`).
- [ ] Update [search_repository.dart](app/lib/data/repositories/search_repository.dart) so the default `itemTypes` includes `SEARCH_ITEM_TYPE_USER`. (Cached pre-change search responses expire within the global `CACHE_TTL_MINUTES`; no explicit invalidation needed.)
- [ ] Tests per [testing.md](../client/testing.md) test pyramid:
    - **Unit test** of `SearchResultRow._dispatch()` (or its extracted dispatcher) — pure callback verification for `SEARCH_ITEM_TYPE_USER`, no widget tree.
    - **Smoke widget test** rendering `SearchResultRow` with a user-bearing `SearchDiscoverItem`, asserting the `Tappable` has the localized `semanticsLabel` and `CachedMediaImage` mounts with the user's `cacheKey`.

**Validates:** End-to-end people search works against the existing server. The third and fourth acceptance criteria are met. Surfaces any visual or navigation rough edges before suggestions work begins.

### Phase 2: Add `GetSearchSuggestions` RPC

**Goal:** Define the wire contract and ship a working server implementation, plus tests, with no client UI change yet.

**Tasks:**
- [ ] Extend [search_service.proto](proto/ripls/api/search_service.proto):
  - Add `rpc GetSearchSuggestions(GetSearchSuggestionsRequest) returns (GetSearchSuggestionsResponse);`
  - `GetSearchSuggestionsRequest`: `repeated string community_ids` (membership-filtered server-side — see below).
  - `GetSearchSuggestionsResponse`: `repeated string top_known_for_categories` and `repeated SharedCommunityRef top_communities`. **Reuse the existing `SharedCommunityRef` from [profile_service.proto:30](proto/ripls/api/profile_service.proto#L30)** — it already has the `{string id; string name}` shape we want. Do NOT define a new `CommunityRef` type; do NOT import `models.Community` (per [proto_conventions.md](../proto_conventions.md)). If the "Shared" prefix is misleading outside profile contexts, rename to `CommunityRef` in the same PR and migrate the one existing reference.
  - Each list returns up to 5 entries; **write the proto comment as a client-facing description** ("Up to 5 community names, most-recently-joined first" / "Up to 5 known-for categories, highest-weight first"), not as "server caps at 5" (per CLAUDE.md §Proto Comments).
- [ ] `npm run generate` to refresh stubs.
- [ ] Implement `GetSearchSuggestions` in [server/services/search/service.go](server/services/search/service.go):
  - `auth.RequireAuth(ctx)` to get user ID.
  - `auth.FilterActiveMemberCommunities()` to **silently filter `community_ids` to the subset the user is an active member of** (mirrors `Search` handler behavior — non-member entries are dropped, not rejected). If the result is empty after filtering, return an empty suggestions response, not an error.
  - Call `known_for.Derive(ctx, storage, known_for.Options{Mode: ModePerUser, OwnerID: userID, CommunityIDs: communityIDs})`, slice first 5 for `top_known_for_categories`.
  - Query `CommunityUser` rows for the user, JOIN onto `Community` to get names, filter to non-deleted, ORDER BY `community_user.created_at_unix_sec DESC LIMIT 5`. Use a single SQL query — avoid the N+1 trap (one query for memberships then N queries for community names).
  - Structured logging per [observability.md](../server/observability.md): `logger := logging.LoggerWithContext(ctx).With("operation", "GetSearchSuggestions")`. `request_id` and `user_id` propagate from context — don't re-attach them. Log start (`InfoContext "starting"`) and completion (`InfoContext "completed", "duration_ms", …, "category_count", …, "community_count", …`). Wrap any 5xx via `connecterr.Internal(ctx, "GetSearchSuggestions.<step>", err, kv...)` — never bare `connect.NewError(connect.CodeInternal, err)` (gated by `scripts/check_codeinternal.js`).
- [ ] Add a `doc.go` line or update the existing one if `searchUsers` already documents the package; no new package.
- [ ] Server tests in [service_test.go](server/services/search/service_test.go) using `storage.SetupTestStorage(t)`:
  - User with no portfolio → empty categories, empty (or single) communities.
  - User with mixed loans/experiences/requests → top 5 categories ordered correctly.
  - User in >5 communities → exactly 5 returned, most-recently-joined first.
  - **Non-member `community_ids` are silently filtered out** (not rejected) — verifies the "filter, not reject" semantics.
  - `storage.AssertMaxQueries(t, ctx, 4, fn)` to lock in the no-N+1 invariant: ~1 for membership filter + ~1 for known_for derivation + 1 for community-name JOIN + a margin of 1. Adjust the cap based on `known_for.Derive` internals once measured.
- [ ] `npm run build` and `go test ./...`.

**Validates:** The server can produce personalized suggestions cheaply. The contract is stable. The first two acceptance criteria are implementable client-side.

### Phase 3: Render suggestions in `_SuggestionsPanel`

**Goal:** First two acceptance criteria are met end-to-end. The idle search surface reflects the user's communities and known-for.

**Tasks:**
- [ ] Add a **separate** `SearchSuggestionsRepository` (keep deps shallow, max 1 level per [architecture.md](../client/architecture.md)). Construct it with `CacheService(cacheManager, 'search_suggestions')` via `cacheManagerProvider`. Cache key format: `'$userId:${(List.of(communityIds)..sort()).join(",")}'` (sorted so order-insensitive). **TTL inherits from the global `CACHE_TTL_MINUTES`** — per-entry TTL is not supported by the Phase 1 cache (see [caching.md](../client/caching.md)).
- [ ] Add `refreshSuggestions()` to the repository that calls the cache's `invalidatePattern('*')` for the `'search_suggestions'` namespace. Wire invalidation hooks from these existing repositories' mutation paths:
    - `CommunityRepository` join/leave mutations
    - `LoanRepository` completion mutation (shifts known-for)
    - `ExperienceRepository` host-completion mutation
    - `RequestRepository` fulfillment mutation
- [ ] Register `searchSuggestionsRepositoryProvider` alongside `searchRepositoryProvider`, wiring `cacheManagerProvider` and a new `searchSuggestionsServiceProvider` (Connect client).
- [ ] Add a **ViewModel** layer per [architecture.md](../client/architecture.md) rules 1–3 (screens watch ViewModels, not repositories): `SearchSuggestionsNotifier` with a Freezed `SearchSuggestionsState` (`topCommunities: List<SharedCommunityRef>`, `topKnownForCategories: List<String>`, `isLoading: bool`, `error: UserError?`) at [app/lib/presentation/viewmodels/search_suggestions_view_model.dart](app/lib/presentation/viewmodels/search_suggestions_view_model.dart). Use `AsyncNotifierProvider.autoDispose` so panel-scoped state cleans up on close. `build()` reads `searchSuggestionsRepositoryProvider` and calls `getSuggestions(communityIds: …)`.
- [ ] Update [_SuggestionsPanel in search_nearby_pill.dart](app/lib/presentation/widgets/search/search_nearby_pill.dart#L335-L363):
  - Watch `searchSuggestionsProvider` (the `AsyncNotifierProvider`) and render via `.when(data:, loading:, error:)`. Empty-section logic lives in the ViewModel-exposed state (e.g. `state.showCommunitiesSection`), not the widget.
  - Render order: top communities → top known-for → recent searches → static fallback chips.
  - Each section has a `_SectionLabel` header. Skip section entirely if its list is empty.
  - **Every chip is a filter.** Community chip taps call `onPick(communityName)` to seed the input and run a filtered search. Known-for chip taps call `onPick(category)`. Same dispatch path as the existing static fallback chips — the panel is never a launcher for screens (decision recorded in §Resolved Decisions).
  - Reuse `_QuickChipButton` for both new sections (verify it composes from `Tappable` or `Toggle` — if it uses raw `GestureDetector`/`InkWell`, refactor it first; `npm run lint:dart:a11y` will reject otherwise). Each chip gets a `semanticsLabel` from `context.l10n`. Per MEMORY: do not invent new per-section coral/cream/font constants; use AppColors helpers and AppTheme.headingFont.
- [ ] L10n: add ARB keys to both `app_en.arb` and `app_es.arb` (with `@key` metadata blocks in `app_en.arb` per [i18n.md](../client/i18n.md)):
    - Section headers: `searchSuggestionsTopCommunitiesHeader`, `searchSuggestionsTopCategoriesHeader`
    - Chip a11y labels (per [accessibility.md](../client/accessibility.md) — required `a11y` prefix, enforced by `require_l10n_for_semantic_labels`): `a11ySearchSuggestionCommunity(name)`, `a11ySearchSuggestionCategory(category)`
    - Phase 1 a11y labels: `a11yOpenUserProfile(userName)`, `searchResultUserAvatarLabel(userName)`.
- [ ] Tests, per [testing.md](../client/testing.md) test pyramid — **ViewModel-first**:
  - **ViewModel tests** at [app/test/presentation/viewmodels/search_suggestions_view_model_test.dart](app/test/presentation/viewmodels/search_suggestions_view_model_test.dart) using `ProviderContainer` + Mockito-mocked `SearchSuggestionsRepository`:
    - empty server response → state has empty lists, no error.
    - both populated → state exposes both lists in correct order.
    - repository throws → state surfaces a typed `UserError` (per `app/lib/core/errors/`).
    - disposal safety (per [architecture.md](../client/architecture.md) §Disposal Testing).
  - **Repository test** at [app/test/data/repositories/search_suggestions_repository_test.dart](app/test/data/repositories/search_suggestions_repository_test.dart) — cache hit/miss pattern with mocked `CacheService` + service (per [testing.md](../client/testing.md) §Repository Testing).
  - **Smoke widget test** for `_SuggestionsPanel` rendering one populated section with a stubbed `searchSuggestionsProvider` override, asserting section headers come from `context.l10n` and chips have the expected `semanticsLabel`. Use `app/test/helpers/l10n_helpers.dart` for the MaterialApp wrapper.
- [ ] `flutter test --reporter expanded`.

**Validates:** All four acceptance criteria are satisfied.

### Phase 4: Stabilize the pill activation transition

**Goal:** Tapping the idle pill no longer causes visible jumps. The active state reuses the existing `_CloseSearchButton` X-affordance instead of the "Cancel" text. The `FloatingHeader` avatar is hidden while the search is active.

This phase is UI-only and independent of Phases 1–3 — it can ship in any order, though slotting it after Phase 3 keeps related search-pill changes in one PR if convenient.

**Tasks:**
- [ ] **Eliminate the input jump.** In [_ActiveInputRow](app/lib/presentation/widgets/search/search_nearby_pill.dart#L255-L331), the `TextField`'s default `InputDecoration` resolves a different vertical metric than the idle `Text` widget at `fontSize: 13`. Match the two: keep `isCollapsed: true` (already set) and tune `contentPadding` to `EdgeInsets.zero` (or whatever matches the idle `Text` baseline). Verify with a screenshot diff between `_IdlePill` and `_ActiveInputRow` showing the placeholder character is pixel-identical. If text-baseline alignment proves flaky across font sizes, pin both surfaces to a `Baseline(baseline: …, baselineType: TextBaseline.alphabetic)` wrapper.
- [ ] **Hide the FloatingHeader avatar while active.** The avatar lives in `FloatingHeader`'s trailing slot. Expose an `isSearchActive` (or similar) signal — either a new Riverpod provider (`searchPillActiveProvider`) toggled by `_SearchNearbyPillState._activate()`/`_deactivate()`, or a callback passed down. `FloatingHeader` watches the signal and hides the avatar (e.g. `Visibility(visible: !isActive, maintainSize: true, maintainAnimation: true, maintainState: true)` so the header layout doesn't reflow when the avatar disappears).
- [ ] **Replace "Cancel" text with the existing `_CloseSearchButton` X-affordance.** Extract `_CloseSearchButton` from [discover_screen.dart:613-639](app/lib/presentation/screens/discover/discover_screen.dart#L613-L639) into a shared widget at `app/lib/presentation/widgets/search/close_search_button.dart` (or similar) so both surfaces can import it. In `_ActiveInputRow`, render the close button **inside** the `_PillFrame` (after the `TextField` in the same `Row`), so the active input width matches the idle pill width exactly. Drop the outer `Row` that currently wraps `_PillFrame` + Cancel.
- [ ] **A11y label.** The close button uses a localized `a11ySearchDismiss` `semanticsLabel` (an ARB key already exists with that name — verify in `app_en.arb`; reuse if present, otherwise add per [accessibility.md](../client/accessibility.md) and [i18n.md](../client/i18n.md) conventions). Remove the now-unused `commonCancel` reference from this widget (do not delete the ARB key — it's likely used elsewhere).
- [ ] **Tests** per [testing.md](../client/testing.md):
    - Golden test (or widget snapshot) for `_IdlePill` and `_ActiveInputRow` showing the placeholder character renders at the same baseline, same x-offset, same width. This is the single tightest assertion that catches a regression.
    - Widget test asserting the active state renders a `_CloseSearchButton` (not a "Cancel" text), and tapping it calls `_deactivate`.
    - Widget test asserting `FloatingHeader` hides the avatar when `searchPillActiveProvider` is true and restores it when false. Use `maintainSize: true` so the test can assert the header's overall size is unchanged across states.
- [ ] `flutter test --reporter expanded` and a manual check on a physical device — text-baseline regressions are easy to miss in widget tests because they don't render real glyphs.

**Validates:** The pill activation feels instantaneous and stable — no jump, no avatar, consistent X-close affordance between activation and search-done states.

## Related Issues

- #1009 — user_screen redesign. People search lands on `user_screen`; if #1009 is mid-flight, coordinate on what the entry route name should be (currently `'profile'` per [content_view_helpers.dart:115-133](app/lib/presentation/widgets/content/content_view_helpers.dart#L115-L133)).
- #1896 — original Discover/search architecture issue that introduced `SearchNearbyPill` and the `_SuggestionsPanel` this issue extends. Useful background for the suggestions panel's design intent.
