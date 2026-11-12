# Issue #803: The discover_screen still sometimes does not load

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/803
**Filed:** 2026-03-07 | **Last activity:** 2026-03-07
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When a new user logs in and navigates to the discover screen, the map and
bottom sheet are sometimes empty — no markers, no search results. However, if
the user taps the search bar and types even a single character, results appear
immediately in both the map and the list. This proves the community has items
and the search RPC works; the issue is that the **initial zero-state search
does not fire or its results are silently lost**.

The word "still" in the title indicates this is a recurring problem. Phase 8
regression tests (in `search_view_model_test.dart`) previously fixed a related
bug where `SelectedCommunityNotifier` re-emissions during startup caused
`build()` to be abandoned and results lost. The current occurrence suggests
another timing path was missed.

## Context from Comments

No comments on the issue. This is a fresh report with reproduction steps but no
discussion yet. The reproduction is straightforward:

1. Login as a new user
2. Go to the discover screen
3. Nothing shows up
4. Type a single letter → results appear

## Analysis

### How the initial search is triggered (three mechanisms)

The discover screen has **three independent mechanisms** that attempt to trigger
the initial search. Each has guards that can prevent it from firing:

**Mechanism 1 — `SearchNotifier.build()` → `_performInitialSearch`**
([search_view_model.dart:220-238](app/lib/presentation/viewmodels/search_view_model.dart#L220-L238)):

- `build()` watches `selectedCommunityProvider`
- When community is available and `_initialSearchCommunityId != community.id`,
  fires `_performInitialSearch` as fire-and-forget (no await)
- Guard: `_initialSearchCommunityId` prevents duplicate searches for the same
  community
- Pre-resolves `searchRepository` and `locationFetcher` to avoid stale refs
  after async gaps

**Mechanism 2 — `DiscoverScreen.ref.listen(selectedCommunityProvider)`**
([discover_screen.dart:266-278](app/lib/presentation/screens/discover/discover_screen.dart#L266-L278)):

- Fires when `selectedCommunity.id` changes
- Sets `_lastSearchedCommunityId = null` and calls `_triggerSearch()`
- `_triggerSearch` calls `searchWithLocation(query: '', communityId: X)` which
  uses a 300ms debounce timer
- Guard: Only fires on *changes*, not the initial value. If the community is
  already set when DiscoverScreen mounts, this listener **never fires**.

**Mechanism 3 — DiscoverScreen fallback check in `build()`**
([discover_screen.dart:309-317](app/lib/presentation/screens/discover/discover_screen.dart#L309-L317)):

```dart
if (selectedCommunity != null &&
    searchState.results.isEmpty &&
    !searchAsync.isLoading &&
    !searchState.isLoading &&
    _lastSearchedCommunityId != selectedCommunity.id) {
  WidgetsBinding.instance.addPostFrameCallback((_) {
    if (mounted) _triggerSearch();
  });
}
```

- Guards: Requires results empty, not loading (both `AsyncValue.isLoading` and
  `SearchState.isLoading`), and `_lastSearchedCommunityId` not yet set
- **One-shot**: Once `_triggerSearch()` runs and sets `_lastSearchedCommunityId`,
  this fallback never fires again for the same community — even if the search
  fails

### Race condition analysis

The three mechanisms interact in timing-dependent ways. Key observations:

1. **`SelectedCommunityState` does not override `==`**
   ([providers.dart:601-628](app/lib/services/providers.dart#L601-L628)).
   Every `copyWith` creates a new identity, so Riverpod always re-notifies
   watchers. `setCommunities` emits at least two state changes
   (communities list, then selectedCommunity) with an async gap for
   `SharedPreferences`.

2. **`IndexedStack` in HomeScreen**
   ([home_screen.dart:446](app/lib/presentation/screens/home/home_screen.dart#L446))
   means DiscoverScreen is mounted immediately, even if the user starts on a
   different tab. All three mechanisms are active from app startup.

3. **`keepAlive()`** on SearchNotifier
   ([search_view_model.dart:195](app/lib/presentation/viewmodels/search_view_model.dart#L195))
   means the provider persists across navigation. `_initialSearchCommunityId`
   persists with it.

### Root cause hypothesis

The most likely failure path for a **new user** is:

1. User logs in. `login_screen.dart` starts community loading as fire-and-forget
   ([login_screen.dart:76-82](app/lib/presentation/screens/auth/login_screen.dart#L76-L82))
   and navigates to `/` via `context.go(destination)`.
2. HomeScreen builds → IndexedStack creates DiscoverScreen.
3. `ref.watch(searchProvider)` triggers `SearchNotifier.build()`. Community is
   **null** (still loading). Returns empty state.
4. Communities load. `setCommunities` emits state update with
   `selectedCommunity` set.
5. `SearchNotifier.build()` re-invokes. Community is now available.
   `_performInitialSearch` fires (fire-and-forget).
6. **Simultaneously**, DiscoverScreen's `ref.listen(selectedCommunityProvider)`
   fires (community changed from null to set). Calls `_triggerSearch()` →
   `searchWithLocation(query: '', communityId: X)`.
7. `searchWithLocation` sets `state = AsyncData(copyWith(isLoading: true))` and
   starts a 300ms debounce timer.
8. `_performInitialSearch` is also running (GPS fetch + search RPC).
9. The debounced `_executeSearchWithLocation` fires after 300ms.
   `_executeSearch` reads `ref.read(searchRepositoryProvider)` fresh and
   executes the search.
10. **Both** searches are now in flight concurrently. If `_performInitialSearch`
    completes first and sets results, `_executeSearch`'s stale-check
    (`currentState.currentQuery != query`) should pass (both use `''`). But if
    `_executeSearch` reads an intermediate state where `currentQuery` was
    briefly modified by a rapid rebuild, it might bail out via the race guard
    at [search_view_model.dart:475-478](app/lib/presentation/viewmodels/search_view_model.dart#L475-L478).
11. Additionally, each build() re-invocation returns `state.value` which the
    AsyncNotifier framework sets as the new state. If build() re-invokes
    **after** `_performInitialSearch` set results but **before**
    `_executeSearch` completes, the framework might transition the state to
    `AsyncLoading` briefly, causing the fallback check to see empty results
    but `isLoading = true` (blocking it from retrying).

The intermittent nature ("sometimes") is consistent with a timing-dependent
race between GPS resolution, debounce timers, and Riverpod's notification
batching.

## Potential Solutions

### Option A: Single authoritative trigger in SearchNotifier

Remove the search trigger responsibility from DiscoverScreen entirely.
SearchNotifier.build() already watches `selectedCommunityProvider` and fires
`_performInitialSearch`. Make this the **only** trigger for the initial
zero-state search.

This also fixes an **architecture violation**: the current DiscoverScreen
contains business logic (search triggering, community-change handling) that
belongs in the ViewModel layer per `docs/client/architecture.md` rule 1
("ViewModels are the ONLY layer that calls Repositories") and the UI guideline
("Don't put business logic in widgets").

In DiscoverScreen:
- Remove the `ref.listen(selectedCommunityProvider)` callback that calls
  `_triggerSearch()`
- Remove the fallback check at lines 309-317
- Keep the `ref.listen(searchProvider)` for map marker updates (already exists)

In SearchNotifier:
- Add a retry mechanism: if `_performInitialSearch` fails, schedule a retry
  after a short delay (e.g., 2s)
- Ensure `_performInitialSearch` is robust against concurrent build()
  re-invocations

- **Pros:** Eliminates the dual-trigger race condition entirely. Single source
  of truth for initial search. Easier to reason about. Fixes architecture
  violation (business logic in Screen widget).
- **Cons:** DiscoverScreen loses its ability to independently retry if
  SearchNotifier fails (mitigated by retry in SearchNotifier).
- **Effort:** Small.

### Option B: Replace fire-and-forget with awaited initial search

Change `_performInitialSearch` from fire-and-forget to awaited within build().
This was the original approach before Phase 8, which was changed to avoid
community re-emission cancellation. Combine with a `Completer` guard to prevent
cancellation.

- **Pros:** Simpler control flow; build() result always reflects the search
  outcome.
- **Cons:** Requires careful handling of Riverpod's build() re-invocation
  semantics. The Phase 8 fix specifically moved away from this pattern. Could
  reintroduce the original bug if not handled carefully.
- **Effort:** Medium.

### Option C: Debounced initial search with retry in DiscoverScreen

Keep the current architecture but fix the race:
- In DiscoverScreen, use a single search trigger (not three) with a short
  initial delay to let SearchNotifier.build() finish its first attempt
- Add retry logic: if results are still empty after a timeout (e.g., 3s),
  force a search regardless of guards
- Remove `_lastSearchedCommunityId` one-shot guard; use a timer-based retry
  instead

- **Pros:** Defensive; guaranteed to eventually show results.
- **Cons:** Adds complexity rather than removing it. Retry-based approaches
  mask root causes.
- **Effort:** Medium.

## Recommendation

**Option A** — Single authoritative trigger in SearchNotifier. The fundamental
issue is having three independent mechanisms trying to trigger the same search,
with guards that can collectively prevent all three from succeeding in certain
timing scenarios. The fix is to eliminate the redundancy, not to add more
safety nets.

SearchNotifier.build() already has the right architecture: it watches the
community provider, guards against duplicate searches, and fires the search
with pre-resolved dependencies. Making it the sole trigger removes the race
condition entirely.

The DiscoverScreen should only react to **search results changing** (which it
already does via `ref.listen(searchProvider)` for map markers and
`ref.watch(searchProvider)` for the bottom sheet).

## Open Questions

- **Does the `mounted` check in login_screen.dart's community loading prevent
  `setCommunities` from being called?** — Recommendation: Replace `if (mounted)`
  with a direct provider update since `ref.read()` doesn't require a mounted
  widget. The `mounted` check protects against `setState` on a disposed widget,
  but `ref.read().setCommunities()` doesn't call `setState`.

- **Should `SelectedCommunityState` implement `==` / use `@freezed`?** —
  Recommendation: Yes, and this is required by architecture, not optional.
  `docs/client/architecture.md` core principle 2 mandates "Freezed for
  immutable state" on all state classes. `SelectedCommunityState` currently
  uses a hand-written `copyWith` without `==` override, which causes Riverpod
  to re-notify on every `copyWith` even when nothing changed. Migrating to
  `@freezed` would prevent unnecessary `build()` re-invocations and make
  behavior more predictable. This should be a Phase 1 task, not deferred.

- **Should SearchNotifier persist results across logout/login cycles?** —
  Recommendation: No. On logout, invalidate the provider or clear its state to
  ensure fresh results after re-login. The `keepAlive` behavior combined with
  `_initialSearchCommunityId` could cause stale state across sessions.

## Implementation Plan

### Phase 1: Fix the initial search race condition

**Goal:** Eliminate the redundant search triggers and make the initial search
reliable.

**Tasks:**

- [ ] In `SearchNotifier._performInitialSearch`: Add error recovery — if the
  search fails, reset `_initialSearchCommunityId` so the next build()
  re-invocation retries
- [ ] In `DiscoverScreen`: Remove the `ref.listen(selectedCommunityProvider)`
  search trigger (lines 266-278). The existing `ref.listen(searchProvider)` at
  lines 281-291 already handles map marker updates when results change, which
  covers the community-switch case (new search → new results → markers update).
  For the edge case of switching to a community with zero results, add
  `clearAllMarkers` to the searchProvider listener when results become empty.
- [ ] In `DiscoverScreen`: Remove the fallback check at lines 309-317 and the
  `_lastSearchedCommunityId` field entirely
- [ ] Migrate `SelectedCommunityState` to `@freezed` (required by architecture:
  all state classes must use Freezed for value equality). This prevents
  spurious `build()` re-invocations from `setCommunities`'s multiple emissions
  when the `selectedCommunity` hasn't actually changed.
- [ ] Add a loading state indicator in DiscoverBottomSheet when
  `searchState.isLoading` is true and results are empty

**Validates:** Initial search reliably fires and produces results for new users
on first visit to discover screen.

### Phase 2: Fix community loading reliability

**Goal:** Ensure communities are always loaded after login, regardless of
widget lifecycle.

**Tasks:**

- [ ] In `login_screen.dart`: Remove the `if (mounted)` check around
  `setCommunities` (line 78). The `mounted` guard is standard for widget-level
  async (per architecture.md "Widget Async Exceptions"), but here it guards a
  `ref.read()` call which accesses the provider container — not the widget
  tree. `ref.read()` is safe after disposal, so the `mounted` check is
  unnecessary and actively harmful (can prevent community loading).
- [ ] Verify that `_loadCommunities()` in `main.dart` is called after login
  (not just during initial app startup). If not, add an auth state listener
  that triggers community loading when the user transitions from
  unauthenticated to authenticated.

**Validates:** Communities are loaded for new users even if the login screen is
disposed before the API call returns.

### Phase 3: Add regression tests

**Goal:** Prevent the race condition from reoccurring.

**Tasks:**

- [ ] Add a test to `search_view_model_test.dart`: verify that when community
  becomes available after initial null state, `_performInitialSearch` fires
  exactly once and produces results
- [ ] Add a test: if `_performInitialSearch` fails (search RPC error),
  `_initialSearchCommunityId` is reset and the next build() re-invocation
  retries
- [ ] Add a widget test for DiscoverScreen: verify that results appear when
  community is set after screen mount (simulates new user flow)

**Validates:** Automated tests cover the specific race condition reported in
the issue.

## Related Issues

- Phase 8 regression tests in `search_view_model_test.dart` (lines 885-1062)
  — Fixed a related bug where community re-emissions caused build() to be
  abandoned. The current issue is a different timing path in the same system.

## Audit Notes

Audited against `docs/client/architecture.md`, `docs/client/caching.md`, and
`docs/server/architecture.md`.

**Changes made during audit:**

1. **Elevated `@freezed` migration for `SelectedCommunityState` from Open
   Question to Phase 1 task.** Architecture mandates Freezed for all state
   classes. The missing `==` override is a direct contributor to the race
   condition (spurious re-notifications).

2. **Added architecture violation callout to Option A.** The current
   DiscoverScreen contains business logic (search triggering) that belongs in
   the ViewModel layer. This provides additional justification for Option A
   beyond just fixing the race.

3. **Clarified `ref.listen(searchProvider)` marker cleanup.** The original
   plan said to "move clearAllMarkers to `ref.listen(searchProvider)`" but
   didn't address the semantic difference. Clarified: the searchProvider
   listener already handles marker updates on result changes; only the
   zero-result edge case needs explicit marker clearing.

4. **Clarified `mounted` check rationale in Phase 2.** Explained why
   `ref.read()` is safe after disposal (accesses provider container, not
   widget tree), distinguishing it from the widget async exception pattern.

**No server-side changes needed.** The issue is entirely client-side. The
server search RPC correctly returns results for empty queries (confirmed via
`QueryCommunitySearch` → `textUnifiedSearch` fallback path).
