# Discover Screen Initial Load — Root Cause Analysis & Fix Plan (v2)

## Reference

- Prior design doc: `docs/ai/discover3.md` (Phase 8)
- PR #810: "Fix 803: The discover_screen still sometimes does not load"
- Issue #803: Discover screen sometimes shows empty results on first load
- Key files:
  - `app/lib/presentation/viewmodels/search_view_model.dart`
  - `app/lib/services/providers.dart` (SelectedCommunityNotifier)
  - `app/lib/main.dart` (startup + auth listener)

## v1 Post-Mortem

The v1 plan diagnosed hypothetical race conditions (duplicate emissions,
ref.mounted failures) but never analyzed actual failure logs. All three phases
were implemented — atomic setCommunities, removing login_screen duplicate,
ref.mounted retry — and the bug persisted.

**What the logs actually show:**

1. `_loadCommunities()` is called TWICE during cold start — once from the
   `ref.listen(authStateProvider)` callback in `build()` (fire-and-forget) and
   once from the `initState()` microtask (awaited). The v1 plan only removed
   the `login_screen.dart` duplicate but missed this one within `main.dart`.

2. The search starts ("fetching location") but "location resolved" NEVER
   appears. No error log, no ref.mounted reset log. The search just vanishes.

3. Three `MapboxLocationService: Got current location` logs appear — but they
   are from the Mapbox map widget, not from the search. The search's
   `Geolocator.getCurrentPosition()` call never returns.

**Root cause:** `_performInitialSearch` calls `await locationFetcher()` which
calls `Geolocator.getCurrentPosition()`. Simultaneously, the Mapbox map widget
requests location via its own Core Location integration. On iOS, concurrent
CLLocationManager requests from two different Flutter plugins
(geolocator + mapbox) can cause one to hang indefinitely. The search blocks
forever at `await locationFetcher()` with no timeout, no fallback, and no
error handling around the location fetch.

## Problems

### 1. Location Fetch Hangs Indefinitely

`_performInitialSearch` calls `await locationFetcher()` with no timeout. The
Geolocator plugin's `getCurrentPosition()` conflicts with the Mapbox map
widget's concurrent Core Location usage on iOS. The search hangs forever.

The location fetch is also outside the `try/catch` block — only the search
RPC is wrapped. If the location fetch throws instead of hanging, the exception
is unhandled and the search fails silently with `_initialSearchCommunityId`
left set (blocking retries).

### 2. Duplicate `_loadCommunities()` in main.dart

Two code paths in `main.dart` both call `_loadCommunities()` during cold start:

**Path A** — `ref.listen(authStateProvider)` in `build()`:
`loadAuthState()` runs in a microtask (after `build()` registers the listener),
transitions auth from null → authenticated, listener fires
`_loadCommunities()` fire-and-forget.

**Path B** — `initState()` microtask continues:
After `loadAuthState()` returns, checks `authState.isAuthenticated` and calls
`await _loadCommunities()`.

Both produce network calls, both call `setCommunities()`. Freezed equality
suppresses duplicate Riverpod notifications (since both set identical state),
so this doesn't directly cause the search bug — but it wastes a network
request and adds confusion to the logs.

## Implementation Plan

### Phase 1: Timeout the Location Fetch

Add a timeout to `locationFetcher()` in `_performInitialSearch` and wrap the
entire method in try/catch. If the location fetch hangs or fails, use fallback
coordinates immediately instead of blocking forever.

**Modified file:** `app/lib/presentation/viewmodels/search_view_model.dart`

**Changes:**
- Add `.timeout(Duration(seconds: 3))` to the `locationFetcher()` call
- Wrap the entire method body in try/catch (not just the search RPC)
- On any failure, reset `_initialSearchCommunityId` and set error state

### Phase 2: Eliminate Duplicate `_loadCommunities()` in main.dart

The `ref.listen(authStateProvider)` callback should NOT call
`_loadCommunities()` on cold start. The `initState()` microtask already
handles that path. The auth listener should only handle fresh login (when the
user transitions from unauthenticated → authenticated after the app is fully
initialized).

**Modified file:** `app/lib/main.dart`

**Changes:**
- Add a `_initialLoadComplete` flag set after the `initState()` microtask
  finishes
- Guard the auth listener's `_loadCommunities()` call with
  `if (_initialLoadComplete)` so it only fires for post-startup auth changes
  (fresh login, token refresh)

### Phase 3: Tests

Add tests that verify:
- Location timeout triggers fallback coordinates
- Search completes even when location fetch throws
- Search completes even when location fetch hangs
- `_initialSearchCommunityId` is reset on any failure
