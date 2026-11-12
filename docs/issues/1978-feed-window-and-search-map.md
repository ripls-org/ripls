# Issue #1978: Extend feed to two weeks and switch to map view on search tap

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1978
**Filed:** 2026-05-16 | **Last activity:** 2026-05-16
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

> **Audit pass — 2026-05-16.** Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/client/accessibility.md`, `docs/client/testing.md`, `docs/server/architecture.md`, `docs/server/observability.md`, and `docs/client/the_feed.md`. Changes from the original plan:
> 1. **Feed expiry is per-community already.** `Community.feed_expiry_hours` exists; plan now bumps the default and applies a one-shot migration rather than introducing a new constant.
> 2. **Ephemeral items already bypass expiry** (experiences, requests, giveaways) — only gear + community-creation cards are affected by the freshness filter, so the change is narrower than first stated.
> 3. **7-day `unreadWindowSeconds` is a hard cap** the original plan missed. Plan now extends it to 14 days so "unread of any age (within window)" is genuinely surfaced and the sidebar "New" dot stays consistent with the feed.
> 4. **ViewModels orchestrate Repositories, not widgets or helpers.** Phase 2 now extracts a `SearchPillNotifier`; Phase 4 keeps repository composition in `DiscoverNotifier`, leaving `DiscoverMapHelper` as a render-only utility. This matches the layering rules in `docs/client/architecture.md`.
> 5. **Accessibility, i18n, and theming gates wired in.** All new strings via `context.l10n` (EN + ES ARB); all new tappable surfaces via `Tappable` / `IconAction`; all new colors via `AppColors.*(context)`; CI lints listed explicitly in Phase 5.
> 6. **Testing structure follows project conventions.** ViewModel-first tests with `ProviderContainer`; widget-level integration tests deferred to manual QA per `docs/client/testing.md`'s note that client-side integration is not currently set up.
> 7. **Server observability.** Phase 1 now requires `logging.LoggerWithContext(ctx)` patterns and forbids silent failures per CLAUDE.md.

## Problem Statement

After the search tab was removed (#1895 / #1896), the app's feed has become the only "what's happening" surface — and on a quiet morning, with no unread items left, it can look empty even when the user belongs to active communities. Two compounding causes:

1. **Feed drops read items quickly.** The server's `generateFeedItems` filters items through `isItemFresh()` ([server/services/feed/service.go:737-747](server/services/feed/service.go#L737-L747)), which evicts seen items after `Community.feed_expiry_hours` (default 24h, configurable per community per [docs/client/the_feed.md](docs/client/the_feed.md#freshness)). Independently, the 7-day **unread window** at [server/services/feed/service.go:662](server/services/feed/service.go#L662) hard-caps `is_unread = true` to activity within the last 7 days — older items, even if never viewed, can never be marked unread. A user who scrolls through their feed in the morning sees an empty list by lunch.
2. **Discovery is no longer a destination.** The Discover/map screen is orphaned in the IndexedStack ([app/lib/presentation/screens/home/home_screen.dart:186-198](app/lib/presentation/screens/home/home_screen.dart#L186-L198)) and only reachable via the feed's search-nearby pill, which fully navigates away to a separate route. There's no ambient sense that "more is happening elsewhere."

The issue proposes two changes that, together, address both: a longer rolling window of feed content, and an immediate map-view reveal the moment the user reaches for search.

**Audit findings already reflected in this plan:**
- The canonical feed reference is [docs/client/the_feed.md](docs/client/the_feed.md). It clarifies that the per-community `Community.feed_expiry_hours` knob already exists, that the default is 24h (not 24–48h), and that **ephemeral items (experiences, requests, giveaways) already bypass expiry** ([docs/client/the_feed.md#ephemeral-items-experiences-requests-and-giveaways](docs/client/the_feed.md)). Only gear and community-creation cards are subject to the freshness filter today. Plan now reflects this.
- "Unread of any age" requires extending or removing the 7-day `unreadWindowSeconds` cap, which the original plan missed. Plan now treats this as a first-class change.

## Context from Comments

No comments on the issue yet. The issue was filed by the project owner with a clear two-part proposal, so direction is not in dispute — but a few non-obvious design decisions need to be settled (captured in Open Questions below).

## Potential Solutions

### Option A: Server-side feed window + unread cap extension + IndexedStack switch on search tap

- **Approach:**
  - **Part 1:** Two coordinated server changes:
    1. Raise the default `Community.feed_expiry_hours` to `14 * 24 = 336` (or add a separate `feedDisplayWindowSeconds = 14d` constant used at the same filter site). Because `feed_expiry_hours` is already a per-community knob, the simplest path is to bump the default in the community-create code path and apply a one-shot migration that raises `0` / `24` values to `336`. The schema and storage path already exist.
    2. Extend `unreadWindowSeconds` from 7 days to 14 days at [server/services/feed/service.go:662](server/services/feed/service.go#L662), OR remove the cap entirely so any item the user has not seen continues to surface as `is_unread = true` regardless of age. Issue text says "always include unread items regardless of age" — leans toward removing the cap, but with a guard so a never-fetched 2-year-old gear item doesn't suddenly resurface (see Open Questions).
  - **Part 2:** Replace the `NavigationHelpers.pushScreen(DiscoverScreen)` call in `search_nearby_pill.dart` with `homeProvider.navigateToTab(2)` (the orphaned Discover stack index), and modify the `OverlayPortal` so its overlay survives the IndexedStack switch. Seed the search provider with an "all content" sentinel so DiscoverScreen renders all gear/events in the user's communities (not filtered by a query).
- **Pros:** Minimal proto churn; reuses existing DiscoverScreen + map helper; the orphaned Discover stack index gets a real purpose again; the persistent overlay pattern matches what the issue asks for ("continue to show the search suggestions"). `feed_expiry_hours` already being per-community means we can roll out gradually if needed.
- **Cons:** Requires teaching the search/discover pipeline a new "all content" mode (the search RPC currently needs a `query` to return results). The overlay must outlive a tab switch — `OverlayPortal` is anchored to a widget in the Feed subtree, so we need to either hoist the overlay above the IndexedStack or render the search pill at a level that survives both tabs. Extending the unread cap changes the meaning of the sidebar "New" dot (`GetFeedStatus`) — items more than 7 days old can now mark a community as having new activity.
- **Effort:** Medium.

### Option B: Client-side feed augmentation + push DiscoverScreen with overlay-pass-through

- **Approach:**
  - **Part 1:** Keep the server feed behavior unchanged; add a second client-side fetch (e.g., a "recent activity" RPC or reuse `Search` with no query and a date filter) and merge results in the Flutter `FeedNotifier` so two weeks of activity is stitched in below unread items.
  - **Part 2:** Keep `pushScreen(DiscoverScreen)` but pass the search-pill state into the pushed route so the overlay is rendered on the new screen instead of the old one.
- **Pros:** No server changes for Part 1; preserves the current "Discover is a route, not a tab" mental model.
- **Cons:** Part 1 stitches two RPCs together in the client, doubling network cost and complicating dedup / ordering / pagination. Part 2 means the overlay is re-created on the destination screen, not the same overlay surviving — a visual seam (re-focus, animation glitch) is likely. Long-term, the right place for "show me two weeks" is the same RPC that already knows about read state and freshness.
- **Effort:** Medium (Part 1) + Small (Part 2), but the Part-1 stitching is technical debt.

### Option C: Add `since_unix_sec` parameter to `GetFeedRequest` and let the client decide

- **Approach:** Same as Option A but expose the window through the proto so the client passes `since_unix_sec = now - 14d` on every call. Server returns all items in the window plus older unreads.
- **Pros:** Window becomes tunable per-call (useful for "show me everything from this month" affordances later); honest API contract — the client tells the server what range it wants.
- **Cons:** Slightly larger surface change; window is now a knob the client must remember to set, and forgetting it would silently revert to today's narrow window. Proto change requires regenerate + careful server-side default.
- **Effort:** Medium-Large.

## Recommendation

**Option A**, with Part 1 implemented as a pure server-side change (no proto field). The 14-day window is a UX product decision, not a per-call tunable, and hard-coding it server-side keeps the API simple. If we later want the client to control the window (e.g. a "show me the last month" filter), we can graduate to Option C without breaking anything — adding an optional proto field is wire-compatible.

For Part 1 specifically, take the **least-invasive shape** consistent with `the_feed.md`:
- Use the existing `Community.feed_expiry_hours` knob — change the default to `336` and migrate stored communities (one-shot SQL UPDATE for rows with `feed_expiry_hours <= 24`). This keeps the rule "freshness = within window OR has new activity" intact and respects per-community overrides.
- Extend `unreadWindowSeconds` from 7 days to 14 days so the unread window matches the display window. If a stronger "unread of any age" semantic is needed later, we can revisit — but matching the two windows keeps the sidebar "New" badge ([docs/client/the_feed.md#the-new-badge-sidebar-indicator](docs/client/the_feed.md)) consistent with what the feed shows.
- Note that ephemeral items (experiences, requests, giveaways) are already exempt from expiry per `the_feed.md` — they require no change. Only `GEAR_SHARED` and `COMMUNITY_CREATED` cards are affected by the new window.

For Part 2, the trickiest piece is making the search-suggestions overlay outlive the IndexedStack switch. The cleanest approach is to **hoist the `OverlayPortal` to a level above the IndexedStack** (e.g., wrap the home `Scaffold` body) so the same overlay instance is rendered regardless of which tab is selected beneath it. The search-nearby pill stays in the Feed app bar, but its overlay anchors to the global overlay layer.

## Resolved Decisions

The original open questions have been answered by the issue author (2026-05-16). Captured here so reviewers and implementers don't re-litigate:

- **Freshness rule:** match the two windows. `Community.feed_expiry_hours = 336` for display and `unreadWindowSeconds = 14d` for unread. Effective rule: `keep = (age < 14d AND fresh) OR isUnread`, with `isUnread` bounded by the same 14d window.
- **`unreadWindowSeconds`:** extend to 14 days. Do not remove the cap (avoids surfacing long-tail items the user never reached and keeps the sidebar "New" dot consistent with feed contents).
- **Rollout shape:** change the default at community-creation time AND apply a one-shot migration that bumps stored rows with `feed_expiry_hours <= 24` to `336`. Per-community overrides remain available.
- **Dismiss-without-typing:** reverts the underlying tab to Feed. Only a submitted search (or in-map navigation) commits the user to Discover.
- **"All content" on the map:** all **gear**, **active experiences**, AND **active requests** across the user's joined communities, unfiltered by time or read state. Compose existing repository listings in `DiscoverNotifier` (viewmodel-level) — `DiscoverMapHelper` stays render-only per [docs/client/architecture.md](docs/client/architecture.md).
- **Discover bottom-nav status:** stays nav-invisible. Bottom nav remains three items wide (Feed, Inbox, Workshop). The search pill is the only entry point.
- **Query refinement:** map starts with "everything," then narrows to query matches as the user types.
- **Cache invalidation for the longer feed window:** no extra work needed. The `feed:list` cache has a 30-min default TTL per [docs/client/caching.md](docs/client/caching.md); items aging into or out of the 14-day window are picked up on the next fetch. Existing invalidation triggers (mark viewed, mutate gear / community, pull-to-refresh) remain correct.

## Implementation Plan

### Phase 1: Server feed-window change
**Goal:** Feed RPC returns 14 days of activity plus all unread items, with read items still surfaced inside that window.
**Tasks:**
- [ ] Bump the default value of `Community.feed_expiry_hours` from `24` to `336` at the community-creation call site in `server/services/community/`. Search for `feed_expiry_hours` and `feedExpiryHours` to find the canonical default.
- [ ] Add a SQL migration that updates stored communities with `feed_expiry_hours <= 24` (or `0`) to `336`. Per CLAUDE.md SQL efficiency rules, this is a single UPDATE — no loop. Confirm storage-layer test coverage of the migration.
- [ ] Extend `unreadWindowSeconds` at [server/services/feed/service.go:662](server/services/feed/service.go#L662) from `7 * 24 * 3600` to `14 * 24 * 3600`. Update the comment that documents the 7-day cap, plus the corresponding line in `the_feed.md` ("Activity older than 7 days is expired and never triggers unread" → 14 days).
- [ ] Leave `isItemFresh()`, the deduplication path, and the ephemeral-items exemption unchanged — they already do the right thing once the windows widen.
- [ ] Confirm the structured-logging pattern is intact: any new log lines in this path must use `logging.LoggerWithContext(ctx).With("operation", "GetFeed", ...)` per [docs/server/observability.md](docs/server/observability.md). No new `log.Printf` / `fmt.Printf`; no silent "warn and continue" — return errors per CLAUDE.md.
- [ ] Add a unit test in `server/services/feed/` that exercises the matrix: (unread × within-14d), (unread × older-than-14d), (read × within-14d), (read × older-than-14d). Only the last should be dropped. Use `storage.SetupTestStorage(t)` per project testing conventions. **Don't forget the cross-community deduplication scenario** — the new window must not introduce duplicates across communities.
- [ ] Add a `GetFeedStatus` test verifying the sidebar "New" badge respects the extended unread window (a 10-day-old unseen gear item should mark the community as having new activity).
- [ ] Validate `npm run build` passes (golangci-lint + go build). If the change touches anything consumed by integration tests, also run `go build -tags=integration ./...` per CLAUDE.md.
**Validates:** The feed surfaces two weeks of activity end-to-end with no client changes. A scroll through the feed before/after on a test account should show meaningfully more content. The sidebar "New" dot continues to reflect feed state consistently.

### Phase 2: Promote the search-suggestions overlay above the IndexedStack
**Goal:** The same `OverlayPortal` instance can sit on top of any home tab, not just Feed.
**Tasks:**
- [ ] Refactor [app/lib/presentation/widgets/search/search_nearby_pill.dart](app/lib/presentation/widgets/search/search_nearby_pill.dart) so the overlay rendering is decoupled from the pill's mounting position. Per [docs/client/architecture.md](docs/client/architecture.md), business logic and state belong in a Notifier — extract the active/inactive state and the seeded query into a new `SearchPillNotifier` (Freezed state, `NotifierProvider.autoDispose`, `SafeNotifierMixin`). The widget keeps render-only concerns. Either:
  - hoist the overlay rendering into a single widget mounted at the home `Scaffold` body level that watches `searchPillProvider`, or
  - render the overlay into the root `Overlay.of(context, rootOverlay: true)` so it survives tab changes.
- [ ] Use existing accessibility primitives per [docs/client/accessibility.md](docs/client/accessibility.md): the pill's tap target must remain a `Tappable` with a `semanticsLabel` pulled from `context.l10n` (e.g. `a11ySearchNearby`); the dismiss scrim and any new icon affordances must follow `IconAction` / `LiveRegion` patterns. Run `npm run lint:dart:a11y` and confirm the allowlist count does not grow.
- [ ] All user-visible strings must come from `context.l10n.*` per [docs/client/i18n.md](docs/client/i18n.md). Add any new keys to both `app_en.arb` and `app_es.arb` before `npm run generate`. CI lint `npm run lint:dart:i18n` must pass.
- [ ] Use `AppColors.*(context)` for the scrim/dim color rather than `Colors.black54` or other hardcoded values, so the overlay respects dark mode.
- [ ] Verify the overlay backdrop still dims everything below it (including the map, when shown).
- [ ] Verify keyboard / focus behavior survives the tab transition — the TextField focus must persist when the underlying stack swaps. Add an `accessibleDuration(context, ...)` wrapper on any new animation so reduce-motion users aren't surprised.
- [ ] Add a ViewModel test at `app/test/presentation/viewmodels/search_pill_view_model_test.dart` covering activate/deactivate/submit transitions and the interaction with `homeProvider` (per [docs/client/testing.md](docs/client/testing.md), test the Notifier with `ProviderContainer`, mocking `homeProvider` and `searchProvider`). Since client-side integration testing is not currently set up per the same doc, manual QA covers the visual overlay-survival aspect.
**Validates:** The overlay state machine is testable in isolation; the visual overlay-survival behavior is covered by manual QA on iOS + Android per CLAUDE.md UI guidance.

### Phase 3: Switch underlying view to map on search tap
**Goal:** Tapping the search pill changes the visible tab to Discover (index 2) while the overlay persists.
**Tasks:**
- [ ] In the new `SearchPillNotifier.activate()` action (Phase 2), additionally call `ref.read(homeProvider.notifier).navigateToTab(2)` to surface DiscoverScreen. Per architecture rules, ViewModels are the only layer that triggers cross-provider actions; widgets are render-only.
- [ ] In `deactivate()` (overlay dismiss without submit), call `navigateToTab(0)` to return to Feed. Use `_visualToStackIndex` semantics — confirm that index 0 still maps to Feed; if `home_screen.dart`'s `_stackToVisualIndex` mapping needs to be aware of the "tab 2 selected via overlay" state, update it accordingly (today, [home_screen.dart:264](app/lib/presentation/screens/home/home_screen.dart#L264) treats stack-index-2 as visual Feed; we may want to suppress the visual bottom-nav selection during overlay-driven Discover).
- [ ] In `submit()`, *remove* the `NavigationHelpers.pushScreen(...)` call — the user is already on the Discover tab; only the underlying search query needs to update. The overlay closes via `deactivate()`.
- [ ] Confirm the existing `searchProvider.searchWithLocation(...)` call still re-renders the map markers + bottom sheet correctly when the query changes in-place. Per [docs/client/caching.md](docs/client/caching.md) Pattern 6, the discover screen already uses `searchCacheInvalidationProvider` for in-place refreshes — reuse this rather than introducing manual refresh paths.
- [ ] Verify the `DiscoverViewedEvent` analytics call ([home_screen.dart:180](app/lib/presentation/screens/home/home_screen.dart#L180)) still fires when the Discover tab becomes visible via this path. If `_logScreenViewEvent` only fires on bottom-nav taps, hook the analytics into the navigation action itself.
- [ ] Manual test: cold-start the app, tap the feed search bar, confirm map appears underneath the suggestions; type a query and submit; confirm map narrows; back out and confirm Feed returns. Test on both iOS and Android per CLAUDE.md UI guidance.
**Validates:** Search-tap-to-map works without a route push. The user can dismiss-without-typing and end up back on Feed.

### Phase 4: "Show everything" map state for no-query browsing
**Goal:** When the search pill is tapped but no query is entered, the map shows all **gear + active experiences + active requests** in the user's communities, not an empty state.
**Tasks:**
- [ ] Add a `browseMode` flag to `DiscoverState` (Freezed) and a corresponding `DiscoverNotifier.loadBrowseMode()` action that fetches via the existing repository layer. Per [docs/client/architecture.md](docs/client/architecture.md): ViewModels orchestrate Repositories, not the other way around — `DiscoverMapHelper` stays a utility that renders markers given a list, not the orchestrator.
- [ ] Compose existing repositories — `GearRepository` for community gear, `ExperienceRepository` (or equivalent) for active experiences, and `RequestRepository` for active requests. Per [docs/client/caching.md](docs/client/caching.md), repository composition stays shallow (max one level); each repository reads from its own cache namespace (`gear:community:<id>:list`, `experience:community:<id>:list`, `request:community:<id>:list`). Use `Future.wait` to load gear / experiences / requests in parallel per the Parallel Data Loading pattern in `architecture.md`.
- [ ] Filter to "active" state on the server-listing side so the map doesn't surface archived / completed / cancelled items. Existing list RPCs likely already expose an "active" filter — confirm before composing in the viewmodel. If not, prefer pushing the filter to SQL per CLAUDE.md SQL efficiency rules rather than dropping items client-side.
- [ ] Geocoding/location filtering: only items with a `location` (gear pickup point, experience venue, request location) should produce a map marker. Items without coordinates fall through to the bottom-sheet list only.
- [ ] Bottom-sheet results must show a consistent count and ordering with what the map markers represent. Sort: most recently active first across all three types (mirror the feed's `last_activity_at` semantics).
- [ ] Use the existing `DiscoverItem` presentation model (so card rendering stays unchanged) and reuse the bottom-sheet implementation from #1896. Confirm `DiscoverItem` already has a request variant; if it covers only gear + experience today, extend it with a `request` variant (preserving the discriminated-union shape).
- [ ] When the user begins typing, smoothly transition from browse-mode results to query results — `SearchNotifier.searchWithLocation` already handles the query side; the transition is just "stop rendering browse-mode results once a query is present." Verify `Search` returns request matches too (the `SearchItemType` enum should already include requests; if not, that's a separate proto enum addition).
- [ ] Per [docs/client/the_feed.md](docs/client/the_feed.md), images must use `CachedMediaImage` with `MediaUrl.cacheKey` for stable URLs (CLAUDE.md Media Display rule). Confirm any new card variants do not introduce raw `Image.network`.
- [ ] Any new user-facing strings (e.g. an empty-state "No items in your communities yet" if a user has truly empty communities) go through `context.l10n` with both `app_en.arb` and `app_es.arb` entries. Note the CLAUDE.md "Event" vs "Experience" rule — bottom-sheet labels for experience cards must say "Event," but request and gear labels are unchanged.
- [ ] Add a ViewModel test at `app/test/presentation/viewmodels/discover_view_model_test.dart` covering: `loadBrowseMode()` populates state from all three repositories; an empty community produces a clean empty state; items missing locations appear in the bottom sheet but not on the map; transitioning to query mode replaces results; pulling-to-refresh invalidates the right cache namespaces (gear + experience + request + media + location, mirroring `DiscoverNotifier.refresh()` per Pattern 5 in `caching.md`).
- [ ] Remove or repurpose the "no query" empty-state path in [app/lib/presentation/screens/discover/discover_screen.dart:37-40](app/lib/presentation/screens/discover/discover_screen.dart#L37-L40) (currently flagged as preserved-for-routing-safety).
**Validates:** The map is never empty when the user taps the feed's search bar — even on first launch with no query history. Gear, experiences, and requests all surface together. ViewModel layer remains the single point of repository orchestration; map helper stays render-only.

### Phase 5: Polish + verification
**Goal:** UX seams smoothed; analytics intact; tests green.
**Tasks:**
- [ ] Verify the bottom-nav selection circle handles the "tab 2 is selected but visually we're on Feed" case sensibly — currently `_stackToVisualIndex(2) = 0` returns Feed, which is now wrong since tab 2 is genuinely showing. Update so the circle reflects "no tab selected" when Discover is showing via the search overlay (or simply hide the bottom nav while the overlay is active — see `homeState.isNavVisible` at [home_screen.dart:549](app/lib/presentation/screens/home/home_screen.dart#L549)).
- [ ] Run `npm run test:app --reporter expanded` and confirm no Flutter regressions; run `go test ./...` for the server changes. Both per CLAUDE.md testing guidance.
- [ ] Run `npm run build` for server lint+build, plus `go build -tags=integration ./...` if anything exported changed. Run `npm run lint:dart:a11y`, `npm run lint:dart:i18n`, `npm run lint:dart:size`, `npm run lint:nav`, and `npm run lint:ephemeral-comments` per project conventions.
- [ ] Update [docs/client/the_feed.md](docs/client/the_feed.md) Freshness, Unread State, and "New" Badge sections to reflect the 14-day windows.
- [ ] Manual QA the full flow on iOS + Android using the `ripls-app-logs` skill to watch for errors: feed → tap search → map appears → dismiss → feed restored. Verify VoiceOver/TalkBack pass-through on the persistent overlay.
**Validates:** No regressions; new flow feels native; docs match behavior.

## Related Issues
- #1895 — Removed the search tab and made Discover orphaned; this issue restores discoverability via a new mechanism.
- #1896 — Unified one-row-per-item discover results list; the bottom-sheet behavior built there is what Phase 4's browse-mode populates.
- #1897 / #1898 — Post-removal cleanup PRs referenced in DiscoverScreen's doc comment; Phase 4 retires the "preserved for routing safety" empty-state path noted there.
