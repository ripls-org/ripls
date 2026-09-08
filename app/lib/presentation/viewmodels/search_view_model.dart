import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:geolocator/geolocator.dart' show Position;
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/utils/media_helpers.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/presentation/models/discover_item.dart';
import 'package:ripls/presentation/viewmodels/search_state.dart';
import 'package:ripls/services/device_location_service.dart';
import 'package:ripls/services/providers.dart';

// State types live in search_state.dart so callers that imported from
// the view-model file historically keep working without churn.
export 'package:ripls/presentation/viewmodels/search_state.dart';

final _log = Logger('SearchViewModel');

/// Notifier for managing search state with debouncing.
///
/// Uses AsyncNotifier to properly model async operations (network searches).
/// Community changes trigger build() re-invocation via ref.watch.
/// The _initialSearchCommunityId guard ensures the initial search fires exactly
/// once per community, even when communitiesProvider re-emits multiple
/// times during startup (Setting communities → Restored persisted community →
/// Community selection finalized). The search is called directly (not via
/// Future.microtask) so it runs while the ref is still valid, then reads all
/// needed dependencies eagerly before any async gap.
class SearchNotifier extends AsyncNotifier<SearchState> {
  // Debounce configuration.
  static const Duration _debounceDuration = Duration(milliseconds: 300);
  Timer? _debounceTimer;

  // Tracks which community set has had its initial search started.
  // Prevents duplicate initial searches when the community provider re-emits
  // the same set multiple times during startup.
  Set<String>? _initialSearchCommunityIds;

  @override
  Future<SearchState> build() async {
    _log.info('SearchViewModel build() called');

    // CRITICAL: Keep this provider alive even when navigating away from discover screen.
    // This prevents disposal and preserves search results + scroll position.
    ref.keepAlive();

    // Watch for search cache invalidations and refresh in-place.
    ref.listen(searchCacheInvalidationProvider, (prev, next) {
      final currentState = state.value;
      if (prev != null &&
          prev != next &&
          currentState != null &&
          currentState.results.isNotEmpty) {
        _log.info(
          'Cache invalidation detected (prev: $prev, next: $next) - refreshing in-place',
        );
        _refreshInPlace();
      }
    });

    // Set up cleanup when provider is disposed.
    ref.onDispose(() {
      _debounceTimer?.cancel();
      _log.info('SearchViewModel disposed');
    });

    // Watch community — triggers rebuild when enabled set changes.
    final communityState = ref.watch(communitiesProvider);
    final enabledIds = communityState.communityIds;
    final enabledSet = enabledIds.toSet();

    if (enabledIds.isNotEmpty && _initialSearchCommunityIds != enabledSet) {
      _log.info(
        'build: ${enabledIds.length} communities enabled'
        ' — starting initial search (prev guard: $_initialSearchCommunityIds)',
      );
      _initialSearchCommunityIds = enabledSet;

      // Clear any person filter from the previous set — the selected
      // user may not exist in the new results.
      final existingState = state.value;
      if (existingState != null &&
          existingState.filters.selectedPersonId != null) {
        state = AsyncData(
          existingState.copyWith(
            filters: existingState.filters.copyWith(selectedPersonId: null),
          ),
        );
      }

      // Read all dependencies eagerly while ref is guaranteed valid.
      // userLocationProvider is session-cached (per-intent); reading .future
      // here both triggers GPS resolution (if not yet resolved) and hands
      // _performInitialSearch a future it can await without another
      // ref.read after an async gap. Initial search is proximity biasing —
      // the user does not see this position in the UI.
      final locationFuture = ref.read(
        userLocationProvider(LocationIntent.proximityBias).future,
      );
      final searchRepository = ref.read(searchRepositoryProvider);

      unawaited(_performInitialSearch(
        communityIds: enabledIds.toList(),
        locationFuture: locationFuture,
        searchRepository: searchRepository,
      ));
    } else if (enabledIds.isNotEmpty) {
      _log.fine(
        'build: community set unchanged — skipping',
      );
    } else {
      _log.info('build: no communities enabled — returning empty state');
    }

    // Preserve existing results/loading state across build() re-invocations.
    return state.value ?? const SearchState();
  }

  /// Safety-net timeout for the location fetch during initial search.
  ///
  /// The proximity-bias path on [userLocationProvider] (see
  /// `DeviceLocationService` `LocationIntent.proximityBias`) uses a recent
  /// OS last-known fix when available and falls through to a medium-accuracy
  /// `getCurrentPosition()` otherwise. Both paths are typically sub-second,
  /// so this timeout is a safety net — not the primary mitigation — for the
  /// rare case where both last-known is missing AND `getCurrentPosition()`
  /// hangs (e.g. right after device reboot, or residual lock contention with
  /// the Mapbox map widget on iOS). If it fires, the search still completes
  /// using fallback coordinates rather than blocking forever.
  static const Duration _locationTimeout = Duration(seconds: 3);

  /// Performs the zero-state initial search for the given [communityIds].
  ///
  /// All dependencies (locationFuture, searchRepository) are pre-resolved by
  /// the caller while ref is guaranteed valid, so this method does not call
  /// ref.read() after any async gap. It only accesses ref.mounted and the
  /// state getter/setter, which are safe to call at any time.
  Future<void> _performInitialSearch({
    required List<String> communityIds,
    required Future<Position?> locationFuture,
    required SearchRepository searchRepository,
  }) async {
    try {
      if (!ref.mounted) {
        _log.info('_performInitialSearch: ref unmounted — resetting guard');
        _initialSearchCommunityIds = null;
        return;
      }
      _log.info('Initial search: fetching location for ${communityIds.length} communities');

      // Show loading immediately.
      final currentState = state.value ?? const SearchState();
      state = AsyncData(
        currentState.copyWith(
          currentCommunityIds: communityIds,
          currentQuery: '',
          isLoading: true,
        ),
      );

      // Invalidate stale cache before searching.
      await searchRepository.invalidateAll();
      _log.info('Initial search: cache invalidated');

      // Fetch location with timeout. The Geolocator plugin can hang when the
      // Mapbox map widget is concurrently requesting Core Location on iOS.
      // On timeout or error, fall back to default coordinates so the search
      // still fires rather than blocking indefinitely.
      Position? position;
      try {
        position = await locationFuture.timeout(_locationTimeout);
      } on TimeoutException {
        _log.warning(
          'Initial search: location fetch timed out after ${_locationTimeout.inSeconds}s — using fallback',
        );
      } catch (e) {
        _log.warning('Initial search: location fetch failed — using fallback: $e');
      }
      final lat = position?.latitude ?? 30.2672; // Austin, TX fallback
      final lng = position?.longitude ?? -97.7431;

      _log.info('Initial search: location resolved ($lat, $lng)');

      // Guard against disposal or community change during the async gap.
      if (!ref.mounted) {
        _log.info('_performInitialSearch: ref unmounted after location — resetting guard');
        _initialSearchCommunityIds = null;
        return;
      }
      final afterLocationState = state.value;
      if (afterLocationState == null) return;
      if (!_communityIdsMatch(afterLocationState.currentCommunityIds, communityIds)) {
        _log.info('_performInitialSearch: community set changed during location fetch — resetting guard');
        _initialSearchCommunityIds = null;
        return;
      }

      state = AsyncData(
        afterLocationState.copyWith(
          currentLatitude: lat,
          currentLongitude: lng,
        ),
      );

      _log.info(
        'Initial search: executing RPC (query="", communities=${communityIds.length})',
      );

      // Uses pre-resolved searchRepository, no ref.read() after async gap.
      final searchResults = await searchRepository.search(
        query: '',
        communityIds: communityIds,
        latitudeDeg: lat,
        longitudeDeg: lng,
      );

      final results = searchResults
          .map((result) => SearchDiscoverItem(result))
          .toList();

      _log.info('Initial search complete: ${results.length} results');

      if (!ref.mounted) {
        _log.info('_performInitialSearch: ref unmounted after search RPC — resetting guard');
        _initialSearchCommunityIds = null;
        return;
      }
      final afterSearchState = state.value;
      if (afterSearchState == null) return;

      state = AsyncData(
        afterSearchState.copyWith(
          results: results,
          isLoading: false,
          error: null,
        ),
      );
    } catch (e, stackTrace) {
      _log.severe('Initial search error: $e', e, stackTrace);
      // Reset so the next build() re-invocation retries the initial search.
      _initialSearchCommunityIds = null;
      if (!ref.mounted) return;
      final afterSearchState = state.value;
      if (afterSearchState != null) {
        state = AsyncData(
          afterSearchState.copyWith(
            isLoading: false,
            error: RpcErrorHandler.classify(e),
          ),
        );
      }
    }
  }

  /// Searches for items with explicit coordinates.
  ///
  /// Prefer [searchWithLocation] for production use (auto-fetches location).
  /// This method is useful for testing or when coordinates are already known.
  ///
  /// [query] - Search text (empty string returns all items)
  /// [communityId] - Community to search within
  /// [latitudeDeg] - User latitude for distance scoring
  /// [longitudeDeg] - User longitude for distance scoring
  /// [showModal] - Unused; kept for call-site compatibility
  void search({
    required String query,
    required List<String> communityIds,
    required double latitudeDeg,
    required double longitudeDeg,
    int? maxResults,
    bool showModal = true,
  }) {
    _debounceTimer?.cancel();
    final currentState = state.value ?? const SearchState();
    state = AsyncData(
      currentState.copyWith(
        currentQuery: query,
        currentCommunityIds: communityIds,
        currentLatitude: latitudeDeg,
        currentLongitude: longitudeDeg,
        isLoading: true,
      ),
    );
    _debounceTimer = Timer(_debounceDuration, () {
      _executeSearch(
        query: query,
        communityIds: communityIds,
        latitudeDeg: latitudeDeg,
        longitudeDeg: longitudeDeg,
        maxResults: maxResults,
      );
    });
  }

  /// Searches with automatic location fetching.
  ///
  /// Handles the async location fetch internally, preventing widgets from
  /// needing to await location before calling search. Uses Austin, TX
  /// (30.2672, -97.7431) as fallback if location is unavailable.
  ///
  /// [query] - Search text (empty string returns all items)
  /// [communityIds] - Communities to search within
  /// [showModal] - Unused; kept for call-site compatibility
  void searchWithLocation({
    required String query,
    required List<String> communityIds,
    bool showModal = true,
    List<SearchItemType> itemTypes = const <SearchItemType>[],
    String? browseCommunityId,
  }) {
    // Cancel previous debounce timer.
    _debounceTimer?.cancel();

    // Show loading state immediately. A typed keyword search resets the
    // item-type filter, the browse-community marker, and any active results
    // scope (#2435: a new search — including quick-search chips — clears
    // the scope) unless the caller passes them explicitly.
    final currentState = state.value ?? const SearchState();
    state = AsyncData(
      currentState.copyWith(
        currentQuery: query,
        currentCommunityIds: communityIds,
        currentItemTypes: itemTypes,
        currentBrowseCommunityId: browseCommunityId,
        scope: SearchScope.none,
        isLoading: true,
      ),
    );

    // Start debounce timer, then fetch location and execute search.
    _debounceTimer = Timer(_debounceDuration, () {
      _executeSearchWithLocation(
        query: query,
        communityIds: communityIds,
        itemTypes: itemTypes,
      );
    });
  }

  
  /// Issues a "browse people in my circles" search — empty query
  /// restricted to USER results, ordered by the server's default
  /// composite score. Used by the People in Circles suggestion chip;
  /// the typed keyword path is intentionally not reused so the chip
  /// does not turn into a literal keyword search for "people".
  void browsePeople({
    required List<String> communityIds,
  }) {
    searchWithLocation(
      query: '',
      communityIds: communityIds,
      itemTypes: const [SearchItemType.SEARCH_ITEM_TYPE_USER],
    );
  }

  /// Issues a "browse help requests" search — empty query restricted
  /// to REQUEST results, ordered by composite score. Used by the
  /// Help Needed suggestion chip so it surfaces actual help requests
  /// instead of running a literal keyword search for "help nearby".
  void browseRequests({
    required List<String> communityIds,
  }) {
    searchWithLocation(
      query: '',
      communityIds: communityIds,
      itemTypes: const [SearchItemType.SEARCH_ITEM_TYPE_REQUEST],
    );
  }

  /// Issues a "browse this community" search — empty query, all
  /// default item types, scoped to a single [communityId]. Used by
  /// the community carousel on the suggestions panel so tapping a
  /// community shows that community's active items instead of
  /// running a keyword search for the community name.
  void browseCommunity({
    required String communityId,
  }) {
    searchWithLocation(
      query: '',
      communityIds: [communityId],
      browseCommunityId: communityId,
    );
  }

  /// Executes search after fetching user's location (called after debounce).
  Future<void> _executeSearchWithLocation({
    required String query,
    required List<String> communityIds,
    required List<SearchItemType> itemTypes,
  }) async {
    // Fetch user's current location via the session-cached provider.
    // Search is proximity biasing — coarse/stale-tolerant is fine.
    final position = await ref.read(
      userLocationProvider(LocationIntent.proximityBias).future,
    );
    final latitudeDeg = position?.latitude ?? 30.2672;
    final longitudeDeg = position?.longitude ?? -97.7431;

    // Check disposal after the async gap before writing state.
    if (!ref.mounted) return;
    final currentState = state.value;
    if (currentState == null) return; // Disposed, don't continue.
    state = AsyncData(
      currentState.copyWith(
        currentLatitude: latitudeDeg,
        currentLongitude: longitudeDeg,
      ),
    );

    // Execute the search.
    await _executeSearch(
      query: query,
      communityIds: communityIds,
      latitudeDeg: latitudeDeg,
      longitudeDeg: longitudeDeg,
      itemTypes: itemTypes,
    );
  }

  /// Executes the actual search API call (called after debounce).
  Future<void> _executeSearch({
    required String query,
    required List<String> communityIds,
    required double latitudeDeg,
    required double longitudeDeg,
    int? maxResults,
    List<SearchItemType> itemTypes = const <SearchItemType>[],
  }) async {
    _log.info('Searching for "$query" in ${communityIds.length} communities');

    // Read current filters before async gap.
    final filters = state.value?.filters ?? const DiscoverFilters();

    try {
      final searchRepository = ref.read(searchRepositoryProvider);
      final searchResults = await searchRepository.search(
        query: query,
        communityIds: communityIds,
        latitudeDeg: latitudeDeg,
        longitudeDeg: longitudeDeg,
        maxResults: maxResults,
        itemTypes: itemTypes.isEmpty ? null : itemTypes,
        includeCompleted: filters.includeCompleted,
      );

      // Convert search results to unified DiscoverItem model.
      final results = searchResults
          .map((result) => SearchDiscoverItem(result))
          .toList();

      _log.info('Search results: ${results.length} total');

      // Only update if the search parameters haven't changed (race condition check).
      final currentState = state.value;
      if (currentState == null) return; // Provider disposed.
      if (currentState.currentQuery != query ||
          !_communityIdsMatch(currentState.currentCommunityIds, communityIds)) {
        return; // Parameters changed, don't update.
      }
      state = AsyncData(
        currentState.copyWith(
          results: results,
          isLoading: false,
          error: null,
        ),
      );

      // Log analytics event for actual searches (not zero-state).
      if (query.isNotEmpty) {
        try {
          unawaited(ref.read(observabilityServiceProvider).logAnalyticsEvent(
                SearchPerformedEvent(
                  queryLength: query.length,
                  resultCount: results.length,
                  searchType: 'all',
                ),
              ));
        } catch (_) {
          // Ignore if disposed during analytics logging.
        }
      }
    } catch (e, stackTrace) {
      _log.severe('❌ Search error: $e', e, stackTrace);

      // Only update error if the search parameters haven't changed.
      final currentState = state.value;
      if (currentState == null) return; // Provider disposed.
      if (currentState.currentQuery != query ||
          !_communityIdsMatch(currentState.currentCommunityIds, communityIds)) {
        return; // Parameters changed, don't update.
      }
      state = AsyncData(
        currentState.copyWith(isLoading: false, error: RpcErrorHandler.classify(e)),
      );
    }
  }

  /// Clears search results and state.
  void clear() {
    _debounceTimer?.cancel();
    state = const AsyncData(SearchState());
  }

  
  /// Filters all sections to items belonging to [userId].
  ///
  /// Passing null clears the person filter. The filter is applied client-side
  /// against the existing search results without a new network call.
  void filterByPerson(String? userId) {
    final currentState = state.value ?? const SearchState();
    state = AsyncData(
      currentState.copyWith(
        filters: currentState.filters.copyWith(selectedPersonId: userId),
      ),
    );
  }

  /// Toggles a category filter on/off.
  ///
  /// [category] must be one of: 'requests', 'events', 'sharing', 'giving'.
  /// Applied client-side — does not trigger a new search RPC.
  void toggleCategory(String category) {
    final currentState = state.value;
    if (currentState == null) return;
    final f = currentState.filters;
    final newFilters = switch (category) {
      'requests' => f.copyWith(showRequests: !f.showRequests),
      'events' => f.copyWith(showEvents: !f.showEvents),
      'sharing' => f.copyWith(showSharing: !f.showSharing),
      'giving' => f.copyWith(showGiving: !f.showGiving),
      _ => f,
    };
    state = AsyncData(currentState.copyWith(filters: newFilters));
  }

  /// Sets the maximum distance filter in miles.
  ///
  /// Pass null to clear the filter ("Any"). Applied client-side — no RPC call.
  void setMaxDistance(double? miles) {
    final currentState = state.value;
    if (currentState == null) return;
    state = AsyncData(
      currentState.copyWith(
        filters: currentState.filters.copyWith(maxDistanceMiles: miles),
      ),
    );
  }

  /// Sets the sort order for results.
  ///
  /// Applied client-side — does not trigger a new search RPC.
  void setSortBy(DiscoverSortBy sortBy) {
    final currentState = state.value;
    if (currentState == null) return;
    state = AsyncData(
      currentState.copyWith(
        filters: currentState.filters.copyWith(sortBy: sortBy),
      ),
    );
  }

  /// Sets the Mapbox map style URI.
  ///
  /// UI-only — does not trigger a new search RPC.
  void setMapStyle(String styleUri) {
    final currentState = state.value;
    if (currentState == null) return;
    state = AsyncData(
      currentState.copyWith(
        filters: currentState.filters.copyWith(mapStyle: styleUri),
      ),
    );
  }

  /// Resets all filters to their default values.
  ///
  /// Preserves selectedPersonId since that is managed separately by the
  /// avatar row, not the filter sheet.
  void resetFilters() {
    final currentState = state.value;
    if (currentState == null) return;
    state = AsyncData(
      currentState.copyWith(filters: const DiscoverFilters()),
    );
  }

  /// Toggles the include_completed flag and re-runs the current search.
  void toggleIncludeCompleted() {
    final currentState = state.value;
    if (currentState == null) return;

    final newFilters = currentState.filters.copyWith(
      includeCompleted: !currentState.filters.includeCompleted,
    );

    state = AsyncData(currentState.copyWith(filters: newFilters, isLoading: true));

    // Re-run search with updated flag.
    _debounceTimer?.cancel();
    _debounceTimer = Timer(_debounceDuration, () {
      final s = state.value;
      if (s == null) return;
      final cIds = s.currentCommunityIds;
      final lat = s.currentLatitude;
      final lng = s.currentLongitude;
      if (cIds.isEmpty || lat == null || lng == null) return;
      _executeSearch(
        query: s.currentQuery,
        communityIds: cIds,
        latitudeDeg: lat,
        longitudeDeg: lng,
      );
    });
  }

  /// Refreshes the current search.
  ///
  /// Uses in-place refresh to preserve scroll position and avoid loading state.
  Future<void> refresh() async {
    final currentState = state.value;
    if (currentState == null ||
        currentState.currentCommunityIds.isEmpty ||
        currentState.currentLatitude == null ||
        currentState.currentLongitude == null) {
      return;
    }
    await _refreshInPlace();
  }

  /// Silently refreshes search results in-place without showing loading state.
  ///
  /// Called automatically when repository cache is invalidated. Preserves
  /// scroll position by updating only the results list.
  Future<void> _refreshInPlace() async {
    final currentState = state.value;
    if (currentState == null ||
        currentState.currentCommunityIds.isEmpty ||
        currentState.currentLatitude == null ||
        currentState.currentLongitude == null) {
      return;
    }

    try {
      final searchRepository = ref.read(searchRepositoryProvider);
      final searchResults = await searchRepository.search(
        query: currentState.currentQuery,
        communityIds: currentState.currentCommunityIds,
        latitudeDeg: currentState.currentLatitude!,
        longitudeDeg: currentState.currentLongitude!,
        itemTypes: currentState.currentItemTypes.isEmpty
            ? null
            : currentState.currentItemTypes,
        includeCompleted: currentState.filters.includeCompleted,
      );

      final results = searchResults
          .map((result) => SearchDiscoverItem(result))
          .toList();

      // Check state.value again after async operation.
      final afterAsyncState = state.value;
      if (afterAsyncState == null) return; // Disposed.
      state = AsyncData(
        afterAsyncState.copyWith(
          results: results,
          refreshCount: afterAsyncState.refreshCount + 1,
        ),
      );
    } catch (e, stackTrace) {
      _log.warning('Error refreshing in-place: $e', e, stackTrace);
      // Don't update error state - this is a silent background refresh.
    }
  }

  /// refreshAllCaches invalidates all community and search caches, then
  /// re-runs the current search query.
  ///
  /// Moves cache invalidation out of the screen layer into the ViewModel,
  /// so screens never call repositories directly.
  Future<void> refreshAllCaches() async {
    final currentState = state.value;
    final communityIds = currentState?.currentCommunityIds ?? [];

    // Refresh caches for all enabled communities.
    for (final cid in communityIds) {
      await Future.wait([
        ref.read(communityRepositoryProvider).refreshCommunityGear(cid),
        ref
            .read(requestRepositoryProvider)
            .refreshRequests(communityId: cid),
        ref
            .read(experienceRepositoryProvider)
            .refreshCommunityExperiences(cid),
      ]);
    }

    if (!ref.mounted) return;

    await Future.wait([
      ref.read(searchRepositoryProvider).invalidateAll(),
      ref.read(gearRepositoryProvider).invalidateAll(),
      ref.read(requestRepositoryProvider).invalidateAll(),
      ref.read(experienceRepositoryProvider).invalidateAll(),
      ref.read(userRepositoryProvider).invalidateAll(),
      ref.read(mediaRepositoryProvider).invalidateAll(),
      ref.read(locationRepositoryProvider).invalidateAll(),
    ]);

    if (!ref.mounted) return;
    await refresh();
  }

  /// Gets media URL for a given media ID via MediaRepository.
  Future<MediaUrl> getMediaUrl(String mediaId) =>
      MediaHelpers.getMediaUrl(ref, mediaId);

  /// Compares two community ID lists for set equality.
  static bool _communityIdsMatch(List<String> a, List<String> b) {
    if (a.length != b.length) return false;
    return a.toSet().containsAll(b);
  }
}

/// Provider for the search state.
///
/// Returns `AsyncValue<SearchState>` to properly model async operations.
/// Use `.value`, `.when()`, or selectors to access the state.
final searchProvider = AsyncNotifierProvider<SearchNotifier, SearchState>(
  SearchNotifier.new,
);
