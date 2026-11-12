# Crashlytics Analysis: _DiscoverScreenState._onSearchChanged

**Issue ID:** 6a23e8224ddd863d1e134ecde32a9c56
**Platform:** Android
**Version:** 0.1.4 (3280)
**Date:** 2026-01-28
**Status:** RESOLVED

---

## Resolution Summary

This crash has been fixed by implementing two complementary solutions:

1. **Phase 1 (Hotfix):** Added `if (!mounted) return;` guards after all async operations in `_DiscoverScreenState` to prevent `ref.read()` calls on disposed widgets.

2. **Phase 2 (Architectural Fix):** Moved the async location fetch from the widget into `SearchNotifier.searchWithLocation()`, eliminating the async gap in the widget entirely. The `_onSearchChanged` method is now synchronous.

**Key changes:**
- [discover_screen.dart](../../app/lib/presentation/screens/discover/discover_screen.dart) - Added mounted guards, made `_onSearchChanged` synchronous
- [search_view_model.dart](../../app/lib/presentation/viewmodels/search_view_model.dart) - Added `searchWithLocation()` method
- [search_view_model_test.dart](../../app/test/presentation/viewmodels/search_view_model_test.dart) - Added 7 unit tests

**Verification:** All 43 unit tests passing, Flutter analyzer reports no issues.

---

## Problem Summary

The app crashes with a fatal exception when `_onSearchChanged` is invoked after the `DiscoverScreen` widget has been unmounted. The crash occurs because `ref.read()` is called on a disposed `ConsumerStatefulElement`.

### Stack Trace (Key Lines)

```
Fatal Exception: FlutterError: Bad state: Using "ref" when a widget is about to or has been unmounted is unsafe.
Ref relies on BuildContext, and BuildContext is unsafe to use when the widget is deactivated.
To safely refer to the state of providers inside State.dispose(), save the provider state in a field of your State class.
       at ConsumerStatefulElement._assertNotDisposed(consumer.dart:432)
       at ConsumerStatefulElement.read(consumer.dart:511)
       at _DiscoverScreenState._onSearchChanged(discover_screen.dart:241)
```

---

## Root Cause Analysis

### 1. The Problematic Code Path

The crash originates from this listener chain:

```dart
// initState (line 75)
widget.searchController?.addListener(_onExternalSearchChanged);

// _onExternalSearchChanged (line 130-133)
void _onExternalSearchChanged() {
  final query = widget.searchController?.text ?? '';
  _onSearchChanged(query);  // <-- This calls ref.read()
}

// _onSearchChanged (lines 206-249) - Multiple ref.read() calls:
// - Line 211: ref.read(selectedCommunityProvider)
// - Line 226-234: ref.read(searchProvider.notifier).search(...)
// - Line 240-248: ref.read(searchProvider.notifier).search(...)
```

### 2. Race Condition Scenarios

The crash can occur in several scenarios:

**Scenario A: Async Operation Completes After Dispose**
1. User triggers search (e.g., types in search field)
2. `_onSearchChanged` calls `MapboxLocationService.getCurrentLocation()` (async, line 222)
3. User navigates away, widget disposes
4. Async operation completes, code continues to line 226/241
5. `ref.read()` fails because widget is disposed

**Scenario B: Listener Notification Race**
1. Search controller text changes
2. Flutter schedules listener notification
3. Widget begins disposal, `dispose()` removes listener
4. Previously-scheduled notification still fires
5. `_onExternalSearchChanged` → `_onSearchChanged` → `ref.read()` fails

**Scenario C: Community Change Listener**
1. Community change triggers `_onSearchChanged` via `ref.listen()` callback (line 713)
2. Widget disposes mid-callback
3. `ref.read()` fails

### 3. Why `dispose()` Doesn't Fully Protect

The current `dispose()` implementation:

```dart
@override
void dispose() {
  widget.searchController?.removeListener(_onExternalSearchChanged);
  _mapHelper.dispose();
  _scrollController.dispose();
  super.dispose();
}
```

While it removes the listener, it doesn't protect against:
- In-flight async operations in `_onSearchChanged`
- Already-dispatched listener notifications
- `ref.listen()` callbacks that are executing

---

## Potential Solutions

### Solution 1: Add `mounted` Guards (Quick Fix)

Add `if (!mounted) return;` checks at key points in `_onSearchChanged`:

```dart
Future<void> _onSearchChanged(String query, {bool showModal = true}) async {
  if (!mounted) return;  // Guard at entry

  setState(() {
    _searchQuery = query;
  });

  final selectedCommunity = ref.read(selectedCommunityProvider).selectedCommunity;
  if (selectedCommunity == null) return;

  _lastSearchedCommunityId = selectedCommunity.id;

  final position = await MapboxLocationService.getCurrentLocation();

  if (!mounted) return;  // Guard after async operation

  if (position == null) {
    ref.read(searchProvider.notifier).search(...);
    return;
  }

  if (!mounted) return;  // Guard before final ref.read()

  ref.read(searchProvider.notifier).search(...);
}
```

**Pros:** Simple, minimal code change
**Cons:** Easy to miss guards on new async operations; defensive rather than structural

### Solution 2: Store Providers in State Fields (Riverpod Recommended)

Save provider references in `late final` fields during `initState`:

```dart
class _DiscoverScreenState extends ConsumerState<DiscoverScreen> {
  late final SearchNotifier _searchNotifier;
  late final SelectedCommunityNotifier _communityNotifier;

  @override
  void initState() {
    super.initState();
    _searchNotifier = ref.read(searchProvider.notifier);
    _communityNotifier = ref.read(selectedCommunityProvider.notifier);
    // ... rest of initState
  }

  Future<void> _onSearchChanged(String query, {bool showModal = true}) async {
    if (!mounted) return;

    setState(() { _searchQuery = query; });

    // Use stored reference instead of ref.read()
    final selectedCommunity = _communityNotifier.state.selectedCommunity;
    // ...

    _searchNotifier.search(...);  // Safe even after dispose
  }
}
```

**Pros:** Follows Riverpod best practices; safe access pattern
**Cons:** Requires refactoring all `ref.read()` calls in callbacks; stored references may have stale state

### Solution 3: Cancellation Token / Disposed Flag Pattern

Use a flag to track disposal and cancel in-flight operations:

```dart
class _DiscoverScreenState extends ConsumerState<DiscoverScreen> {
  bool _isDisposed = false;

  @override
  void dispose() {
    _isDisposed = true;
    widget.searchController?.removeListener(_onExternalSearchChanged);
    // ... rest of dispose
    super.dispose();
  }

  Future<void> _onSearchChanged(String query, {bool showModal = true}) async {
    if (_isDisposed) return;

    setState(() { _searchQuery = query; });

    final selectedCommunity = ref.read(selectedCommunityProvider).selectedCommunity;
    if (selectedCommunity == null || _isDisposed) return;

    final position = await MapboxLocationService.getCurrentLocation();
    if (_isDisposed) return;  // Check after every async gap

    ref.read(searchProvider.notifier).search(...);
  }
}
```

**Pros:** Explicit control; works with `mounted` too
**Cons:** Redundant with `mounted`; still requires manual checks

### Solution 4: Move Search Logic to ViewModel (Architectural)

Move the search orchestration out of the widget entirely:

```dart
// In SearchNotifier (search_view_model.dart)
class SearchNotifier extends Notifier<SearchState> {
  Future<void> searchWithLocation({
    required String query,
    required String communityId,
    bool showModal = true,
  }) async {
    final position = await MapboxLocationService.getCurrentLocation();

    final lat = position?.latitude ?? 30.2672;  // Fallback
    final lng = position?.longitude ?? -97.7431;

    search(
      query: query,
      communityId: communityId,
      latitudeDeg: lat,
      longitudeDeg: lng,
      showModal: showModal,
    );
  }
}

// In DiscoverScreen
void _onSearchChanged(String query, {bool showModal = true}) {
  if (!mounted) return;

  setState(() { _searchQuery = query; });

  final selectedCommunity = ref.read(selectedCommunityProvider).selectedCommunity;
  if (selectedCommunity == null) return;

  _lastSearchedCommunityId = selectedCommunity.id;

  // Single call, no async gaps in widget
  ref.read(searchProvider.notifier).searchWithLocation(
    query: query,
    communityId: selectedCommunity.id,
    showModal: showModal,
  );
}
```

**Pros:** Clean separation; widget has no async gaps; testable
**Cons:** More refactoring; changes ViewModel API

---

## Recommendation

**Implemented Solution 1 (mounted guards) as a hotfix**, then **Solution 4 (move to ViewModel)** for a proper architectural fix. Both phases are now complete.

### Rationale

1. **Immediate stability:** `mounted` guards stop the crash with minimal risk
2. **Long-term maintainability:** Moving async logic to ViewModels follows the architecture in [docs/client/architecture.md](../client/architecture.md) which states "ViewModels are the ONLY layer that calls Repositories" and keeps widgets focused on UI
3. **Pattern consistency:** Other async-heavy operations in the codebase (e.g., `_refreshResults`, `_loadUserLocation`) have also been hardened with mounted guards

---

## Open Questions

1. **Frequency:** How often does this crash occur? Is it tied to specific navigation patterns (e.g., quickly switching tabs, using back gesture)?

2. **Other affected screens:** Are there similar patterns in other screens that use `ref.listen()` or TextEditingController listeners with async operations?

3. **Search controller lifecycle:** Who owns `widget.searchController`? If it's owned by the parent (`HomeScreen`), the controller might outlive `DiscoverScreen`, exacerbating the race condition.

4. **Community change timing:** Does the crash correlate with community switches? The `ref.listen(selectedCommunityProvider, ...)` callback also calls `_onSearchChanged`.

---

## Phased Implementation Plan

### Phase 1: Hotfix (Immediate) ✅ COMPLETED

**Goal:** Stop the crash in production

**Changes:**
1. ✅ Add `if (!mounted) return;` guards to `_onSearchChanged`:
   - At method entry
   - After `MapboxLocationService.getCurrentLocation()`
   - Before each `ref.read()` call

2. ✅ Add similar guards to other async methods:
   - `_refreshResults()` - guards at entry and after each `await`
   - `_onStyleLoaded()` - guards after async operations and inside callbacks
   - `_centerOnUserLocation()` - guards at entry and after location fetch

**Files changed:** [discover_screen.dart](../../app/lib/presentation/screens/discover/discover_screen.dart)

**Testing:**
- Manual: Rapidly navigate away from Discover screen while search is in progress
- Manual: Switch communities while on Discover screen, then immediately navigate away

### Phase 2: ViewModel Refactor ✅ COMPLETED

**Goal:** Move async orchestration out of widget

**Changes:**
1. ✅ Added `searchWithLocation()` method to `SearchNotifier` that handles location fetch internally
2. ✅ Added `_executeSearchWithLocation()` private method for the async implementation
3. ✅ Changed `_onSearchChanged` from `Future<void>` to `void` (synchronous)
4. ✅ Added 7 unit tests for `SearchNotifier.searchWithLocation()`

**Files changed:**
- [search_view_model.dart](../../app/lib/presentation/viewmodels/search_view_model.dart)
- [discover_screen.dart](../../app/lib/presentation/screens/discover/discover_screen.dart)
- [search_view_model_test.dart](../../app/test/presentation/viewmodels/search_view_model_test.dart)

**Testing:**
- ✅ All 43 unit tests passing
- ✅ Flutter analyzer reports no issues

### Phase 2.5: SafeNotifierMixin ✅ COMPLETED (2026-01-28)

**Goal:** Add defensive safety to SearchNotifier async methods

**Changes:**
1. ✅ Added `SafeNotifierMixin<SearchState>` to `SearchNotifier`
2. ✅ Updated `_executeSearch()` to use `safeUpdateState()` after async operations
3. ✅ Updated `_executeSearchWithLocation()` to use `safeUpdateState()` with early return on disposal
4. ✅ Updated `_refreshInPlace()` to use `safeUpdateState()`
5. ✅ Updated `fetchCommunitySkills()` to use `safeUpdateState()`

**Files changed:**
- [search_view_model.dart](../../app/lib/presentation/viewmodels/search_view_model.dart) - Added SafeNotifierMixin
- [safe_notifier.dart](../../app/lib/core/utils/safe_notifier.dart) - Mixin implementation

**Note:** SearchNotifier uses `ref.keepAlive()` (not autoDispose), so AsyncNotifier migration is inappropriate. SafeNotifierMixin provides defensive safety while preserving the provider's long-lived nature.

**Testing:**
- ✅ All 43 SearchNotifier unit tests passing
- ✅ Flutter analyzer reports no issues

### Phase 3: Audit & Harden (Future)

**Goal:** Prevent similar issues across the codebase

**Changes:**
1. Audit all `ConsumerStatefulWidget` classes for:
   - Async operations using `ref.read()` after `await`
   - `ref.listen()` callbacks with async operations
   - TextEditingController listeners calling async methods
2. Add lint rules or code review checklist item
3. Document pattern in [CLAUDE.md](../../CLAUDE.md) or architecture docs

**Estimated scope:** Codebase-wide audit

---

## References

- [Riverpod: Using ref safely](https://riverpod.dev/docs/essentials/lifecycle#using-ref-after-a-provider-was-disposed)
- [Flutter StatefulWidget lifecycle](https://api.flutter.dev/flutter/widgets/State-class.html)
- [Project architecture](../client/architecture.md) - Updated with Async Safety section
- [Caching patterns](../client/caching.md)
- [ref_check.md](../ai/ref_check.md) - Full analysis of async safety patterns and migration plan
- [SafeNotifierMixin](../../app/lib/core/utils/safe_notifier.dart) - Transitional tool for existing Notifiers
