import 'package:fixnum/fixnum.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' show Experience;
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Gear, Availability;
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show Request;
import 'package:ripls/data/gen/ripls/api/search_service.pb.dart'
    show SearchItemType, SearchResultItem;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/models/discover_item.dart';
import 'package:ripls/presentation/viewmodels/search_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'search_view_model_test.mocks.dart';

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

SearchDiscoverItem _makeGear({
  required String id,
  Availability avail = Availability.AVAILABILITY_FOR_LOAN,
  double distanceMeters = 0,
}) {
  return SearchDiscoverItem(
    SearchResultItem(
      itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
      distanceMeters: distanceMeters,
      gear: Gear(id: id, availability: avail),
    ),
  );
}

SearchDiscoverItem _makeRequest({
  required String id,
  double distanceMeters = 0,
  int createdAtUnixSec = 0,
}) {
  return SearchDiscoverItem(
    SearchResultItem(
      itemType: SearchItemType.SEARCH_ITEM_TYPE_REQUEST,
      distanceMeters: distanceMeters,
      request: Request(id: id, createdAtUnixSec: Int64(createdAtUnixSec)),
    ),
  );
}

SearchDiscoverItem _makeEvent({
  required String id,
  double distanceMeters = 0,
  int createdAtUnixSec = 0,
}) {
  return SearchDiscoverItem(
    SearchResultItem(
      itemType: SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE,
      distanceMeters: distanceMeters,
      experience: Experience(id: id, createdAtUnixSec: Int64(createdAtUnixSec)),
    ),
  );
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late ProviderContainer container;
  late MockSearchRepository mockSearchRepository;

  setUp(() {
    mockSearchRepository = MockSearchRepository();
    when(
      mockSearchRepository.search(
        query: anyNamed('query'),
        communityIds: anyNamed('communityIds'),
        latitudeDeg: anyNamed('latitudeDeg'),
        longitudeDeg: anyNamed('longitudeDeg'),
        maxResults: anyNamed('maxResults'),
        includeCompleted: anyNamed('includeCompleted'),
      ),
    ).thenAnswer((_) async => []);
    when(
      mockSearchRepository.invalidateSearchesForCommunity(any),
    ).thenAnswer((_) async {});
    container = ProviderContainer(
      overrides: [
        searchRepositoryProvider.overrideWithValue(mockSearchRepository),
      ],
    );
  });

  tearDown(() {
    container.dispose();
  });

  // -------------------------------------------------------------------------
  // DiscoverFilters.activeFilterCount
  // -------------------------------------------------------------------------

  group('DiscoverFilters.activeFilterCount', () {
    test('returns 0 for default filters', () {
      const f = DiscoverFilters();
      expect(f.activeFilterCount, 0);
    });

    test('counts each deselected category separately', () {
      const f = DiscoverFilters(
        showRequests: false,
        showEvents: false,
        showSharing: false,
        showGiving: false,
      );
      expect(f.activeFilterCount, 4);
    });

    test('counts includeCompleted when true', () {
      const f = DiscoverFilters(includeCompleted: true);
      expect(f.activeFilterCount, 1);
    });

    test('counts maxDistanceMiles when set', () {
      const f = DiscoverFilters(maxDistanceMiles: 5);
      expect(f.activeFilterCount, 1);
    });

    test('counts sortBy when not relevancy', () {
      const f = DiscoverFilters(sortBy: DiscoverSortBy.nearest);
      expect(f.activeFilterCount, 1);
    });

    test('sums all active filters', () {
      const f = DiscoverFilters(
        includeCompleted: true,
        showRequests: false,
        showEvents: false,
        maxDistanceMiles: 1,
        sortBy: DiscoverSortBy.newest,
      );
      // 1 (completed) + 1 (requests) + 1 (events) + 1 (distance) + 1 (sort)
      expect(f.activeFilterCount, 5);
    });

    test('does not count map style change', () {
      const f = DiscoverFilters(mapStyle: 'mapbox://styles/mapbox/dark-v11');
      // Map style is a preference, not a "filter" for the count.
      expect(f.activeFilterCount, 0);
    });
  });

  // -------------------------------------------------------------------------
  // SearchState.categoryFilteredResults
  // -------------------------------------------------------------------------

  group('SearchState.categoryFilteredResults', () {
    final loanGear = _makeGear(id: 'gear-loan', avail: Availability.AVAILABILITY_FOR_LOAN);
    final giveGear = _makeGear(id: 'gear-give', avail: Availability.AVAILABILITY_FOR_GIVEAWAY);
    final request = _makeRequest(id: 'req-1');
    final event = _makeEvent(id: 'evt-1');

    test('returns all results when all categories are enabled', () {
      final state = SearchState(
        results: [loanGear, giveGear, request, event],
      );
      expect(state.categoryFilteredResults.length, 4);
    });

    test('excludes requests when showRequests is false', () {
      final state = SearchState(
        results: [loanGear, giveGear, request, event],
        filters: const DiscoverFilters(showRequests: false),
      );
      final ids = state.categoryFilteredResults.map((i) => i.id).toList();
      expect(ids, isNot(contains('req-1')));
      expect(ids, containsAll(['gear-loan', 'gear-give', 'evt-1']));
    });

    test('excludes events when showEvents is false', () {
      final state = SearchState(
        results: [loanGear, giveGear, request, event],
        filters: const DiscoverFilters(showEvents: false),
      );
      final ids = state.categoryFilteredResults.map((i) => i.id).toList();
      expect(ids, isNot(contains('evt-1')));
      expect(ids, containsAll(['gear-loan', 'gear-give', 'req-1']));
    });

    test('excludes loan gear when showSharing is false', () {
      final state = SearchState(
        results: [loanGear, giveGear, request],
        filters: const DiscoverFilters(showSharing: false),
      );
      final ids = state.categoryFilteredResults.map((i) => i.id).toList();
      expect(ids, isNot(contains('gear-loan')));
      expect(ids, contains('gear-give'));
    });

    test('excludes giveaway gear when showGiving is false', () {
      final state = SearchState(
        results: [loanGear, giveGear, request],
        filters: const DiscoverFilters(showGiving: false),
      );
      final ids = state.categoryFilteredResults.map((i) => i.id).toList();
      expect(ids, isNot(contains('gear-give')));
      expect(ids, contains('gear-loan'));
    });

    test('returns empty list when all categories disabled', () {
      final state = SearchState(
        results: [loanGear, giveGear, request, event],
        filters: const DiscoverFilters(
          showRequests: false,
          showEvents: false,
          showSharing: false,
          showGiving: false,
        ),
      );
      expect(state.categoryFilteredResults, isEmpty);
    });
  });

  // -------------------------------------------------------------------------
  // Distance filtering
  // -------------------------------------------------------------------------

  group('Distance filtering', () {
    test('excludes items beyond maxDistanceMiles', () {
      final near = _makeRequest(id: 'near', distanceMeters: 500);
      final far = _makeRequest(id: 'far', distanceMeters: 5000);
      // 1 mi = 1609.34 m, so 5000 m > 1 mi but < 5 mi
      final state = SearchState(
        results: [near, far],
        filters: const DiscoverFilters(maxDistanceMiles: 1),
      );
      final ids = state.categoryFilteredResults.map((i) => i.id).toList();
      expect(ids, contains('near'));
      expect(ids, isNot(contains('far')));
    });

    test('includes all items when maxDistanceMiles is null', () {
      final near = _makeRequest(id: 'near', distanceMeters: 500);
      final far = _makeRequest(id: 'far', distanceMeters: 100000);
      final state = SearchState(results: [near, far]);
      expect(state.categoryFilteredResults.length, 2);
    });

    test('includes items with no distance data when filter is set', () {
      // distanceMeters = 0 means no data; should not be excluded.
      final noDistance = _makeRequest(id: 'no-dist');
      final state = SearchState(
        results: [noDistance],
        filters: const DiscoverFilters(maxDistanceMiles: 1),
      );
      expect(state.categoryFilteredResults, isNotEmpty);
    });
  });

  // -------------------------------------------------------------------------
  // SearchState.sortedResults
  // -------------------------------------------------------------------------

  group('SearchState.sortedResults', () {
    test('relevancy preserves server order', () {
      final items = [
        _makeRequest(id: 'r1', distanceMeters: 5000),
        _makeRequest(id: 'r2', distanceMeters: 100),
        _makeRequest(id: 'r3', distanceMeters: 2000),
      ];
      final state = SearchState(results: items);
      final ids = state.sortedResults.map((i) => i.id).toList();
      expect(ids, ['r1', 'r2', 'r3']); // Preserved server order.
    });

    test('nearest sorts by distance ascending', () {
      final items = [
        _makeRequest(id: 'r1', distanceMeters: 5000),
        _makeRequest(id: 'r2', distanceMeters: 100),
        _makeRequest(id: 'r3', distanceMeters: 2000),
      ];
      final state = SearchState(
        results: items,
        filters: const DiscoverFilters(sortBy: DiscoverSortBy.nearest),
      );
      final ids = state.sortedResults.map((i) => i.id).toList();
      expect(ids, ['r2', 'r3', 'r1']);
    });

    test('newest sorts by creation timestamp descending', () {
      final items = [
        _makeRequest(id: 'old', createdAtUnixSec: 100),
        _makeRequest(id: 'new', createdAtUnixSec: 300),
        _makeRequest(id: 'mid', createdAtUnixSec: 200),
      ];
      final state = SearchState(
        results: items,
        filters: const DiscoverFilters(sortBy: DiscoverSortBy.newest),
      );
      final ids = state.sortedResults.map((i) => i.id).toList();
      expect(ids, ['new', 'mid', 'old']);
    });

    test('newest sorts events by creation timestamp descending', () {
      final items = [
        _makeEvent(id: 'e-old', createdAtUnixSec: 100),
        _makeEvent(id: 'e-new', createdAtUnixSec: 500),
      ];
      final state = SearchState(
        results: items,
        filters: const DiscoverFilters(sortBy: DiscoverSortBy.newest),
      );
      final ids = state.sortedResults.map((i) => i.id).toList();
      expect(ids, ['e-new', 'e-old']);
    });
  });

  // -------------------------------------------------------------------------
  // filteredResults pipeline order
  // -------------------------------------------------------------------------

  group('filteredResults applies filters in correct order', () {
    test('person → category → distance → sort', () {
      // Two users; person A has requests, person B has events.
      final personAReq = SearchDiscoverItem(
        SearchResultItem(
          itemType: SearchItemType.SEARCH_ITEM_TYPE_REQUEST,
          distanceMeters: 1000,
          request: Request(
            id: 'req-a',
            requester: User(id: 'user-a'),
          ),
        ),
      );
      final personBEvt = SearchDiscoverItem(
        SearchResultItem(
          itemType: SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE,
          distanceMeters: 500,
          experience: Experience(
            id: 'evt-b',
            owner: User(id: 'user-b'),
          ),
        ),
      );
      final state = SearchState(
        results: [personAReq, personBEvt],
        filters: const DiscoverFilters(
          selectedPersonId: 'user-a',
          showEvents: false,
          sortBy: DiscoverSortBy.nearest,
        ),
      );
      // Person filter → only personA_req remains; showEvents=false irrelevant;
      // sort by nearest → still just personA_req.
      expect(state.filteredResults.length, 1);
      expect(state.filteredResults.first.id, personAReq.id);
    });
  });

  // -------------------------------------------------------------------------
  // itemsWithLocations respects active filters
  // -------------------------------------------------------------------------

  group('SearchState.itemsWithLocations', () {
    test('excludes filtered-out items', () {
      final request = SearchDiscoverItem(
        SearchResultItem(
          itemType: SearchItemType.SEARCH_ITEM_TYPE_REQUEST,
          request: Request(
            id: 'req-loc',
            latitudeDeg: 37.7,
            longitudeDeg: -122.4,
          ),
        ),
      );
      final state = SearchState(
        results: [request],
        filters: const DiscoverFilters(showRequests: false),
      );
      expect(state.itemsWithLocations, isEmpty);
    });

    test('includes items with valid coordinates', () {
      final request = SearchDiscoverItem(
        SearchResultItem(
          itemType: SearchItemType.SEARCH_ITEM_TYPE_REQUEST,
          request: Request(
            id: 'req-loc',
            latitudeDeg: 37.7,
            longitudeDeg: -122.4,
          ),
        ),
      );
      final state = SearchState(results: [request]);
      expect(state.itemsWithLocations, hasLength(1));
    });
  });

  // -------------------------------------------------------------------------
  // SearchNotifier mutation methods
  // -------------------------------------------------------------------------

  group('SearchNotifier.toggleCategory', () {
    test('toggles showRequests', () async {
      await container.read(searchProvider.future);
      final notifier = container.read(searchProvider.notifier);

      notifier.toggleCategory('requests');
      final s = container.read(searchProvider).value!;
      expect(s.filters.showRequests, isFalse);

      notifier.toggleCategory('requests');
      final s2 = container.read(searchProvider).value!;
      expect(s2.filters.showRequests, isTrue);
    });

    test('toggles showEvents', () async {
      await container.read(searchProvider.future);
      container.read(searchProvider.notifier).toggleCategory('events');
      expect(container.read(searchProvider).value!.filters.showEvents, isFalse);
    });

    test('toggles showSharing', () async {
      await container.read(searchProvider.future);
      container.read(searchProvider.notifier).toggleCategory('sharing');
      expect(
        container.read(searchProvider).value!.filters.showSharing,
        isFalse,
      );
    });

    test('toggles showGiving', () async {
      await container.read(searchProvider.future);
      container.read(searchProvider.notifier).toggleCategory('giving');
      expect(
        container.read(searchProvider).value!.filters.showGiving,
        isFalse,
      );
    });
  });

  group('SearchNotifier.setMaxDistance', () {
    test('sets maxDistanceMiles', () async {
      await container.read(searchProvider.future);
      container.read(searchProvider.notifier).setMaxDistance(5);
      expect(
        container.read(searchProvider).value!.filters.maxDistanceMiles,
        5.0,
      );
    });

    test('clears maxDistanceMiles when null', () async {
      await container.read(searchProvider.future);
      final notifier = container.read(searchProvider.notifier);
      notifier.setMaxDistance(5);
      notifier.setMaxDistance(null);
      expect(
        container.read(searchProvider).value!.filters.maxDistanceMiles,
        isNull,
      );
    });
  });

  group('SearchNotifier.setSortBy', () {
    test('sets sort order', () async {
      await container.read(searchProvider.future);
      container.read(searchProvider.notifier).setSortBy(DiscoverSortBy.nearest);
      expect(
        container.read(searchProvider).value!.filters.sortBy,
        DiscoverSortBy.nearest,
      );
    });
  });

  group('SearchNotifier.setMapStyle', () {
    test('sets map style URI', () async {
      await container.read(searchProvider.future);
      container
          .read(searchProvider.notifier)
          .setMapStyle('mapbox://styles/mapbox/dark-v11');
      expect(
        container.read(searchProvider).value!.filters.mapStyle,
        'mapbox://styles/mapbox/dark-v11',
      );
    });
  });

  group('SearchNotifier.resetFilters', () {
    test('resets all filters to defaults', () async {
      await container.read(searchProvider.future);
      final notifier = container.read(searchProvider.notifier);
      notifier.toggleCategory('requests');
      notifier.setMaxDistance(5);
      notifier.setSortBy(DiscoverSortBy.newest);
      notifier.setMapStyle('mapbox://styles/mapbox/dark-v11');

      notifier.resetFilters();
      final f = container.read(searchProvider).value!.filters;
      expect(f.showRequests, isTrue);
      expect(f.showEvents, isTrue);
      expect(f.showSharing, isTrue);
      expect(f.showGiving, isTrue);
      expect(f.maxDistanceMiles, isNull);
      expect(f.sortBy, DiscoverSortBy.relevancy);
      expect(f.includeCompleted, isFalse);
    });

    test('does not trigger a search RPC', () async {
      await container.read(searchProvider.future);
      container.read(searchProvider.notifier).resetFilters();
      // No additional search calls beyond the initial build.
      verifyNever(
        mockSearchRepository.search(
          query: anyNamed('query'),
          communityIds: anyNamed('communityIds'),
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
        ),
      );
    });
  });
}
