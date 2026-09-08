import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart' show Position;
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart' show CommunityItem;
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Gear;
import 'package:ripls/data/gen/ripls/api/search_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/presentation/models/discover_item.dart';
import 'package:ripls/presentation/viewmodels/search_view_model.dart';
import 'package:ripls/services/providers.dart';

import '../../core/observability/analytics_test_helper.dart';
import '../../helpers/fake_async_helpers.dart';
import 'search_view_model_test.mocks.dart';

/// Controllable community notifier used in Phase 8 regression tests.
///
/// Extends [CommunitiesNotifier] so it can be used as an override for
/// [communitiesProvider], allowing tests to emit specific community
/// states that simulate the multi-emission startup sequence.
class _ManualCommunityNotifier extends CommunitiesNotifier {
  @override
  CommunitiesState build() => const CommunitiesState();

  void emit(CommunitiesState newState) => state = newState;
}

@GenerateMocks([SearchRepository])
void main() {
  late ProviderContainer container;
  late MockSearchRepository mockSearchRepository;
  late MockObservabilityService mockObservability;

  setUp(() {
    mockSearchRepository = MockSearchRepository();
    mockObservability = MockObservabilityService();
    container = ProviderContainer(
      overrides: [
        searchRepositoryProvider.overrideWithValue(mockSearchRepository),
        observabilityServiceProvider.overrideWithValue(mockObservability),
      ],
    );
  });

  tearDown(() {
    container.dispose();
  });

  group('SearchState', () {
    test('initial state is correct', () async {
      // Wait for async build to complete
      final state = await container.read(searchProvider.future);

      expect(state.results, isEmpty);
      expect(state.isLoading, isFalse);
      expect(state.error, isNull);
      expect(state.currentQuery, '');
      expect(state.currentCommunityIds, isEmpty);
      expect(state.currentLatitude, isNull);
      expect(state.currentLongitude, isNull);
    });

    test('hasError returns correct value', () {
      const stateWithError =
          SearchState(error: UserError.generic(fallback: 'Error'));
      const stateWithoutError = SearchState();

      expect(stateWithError.hasError, isTrue);
      expect(stateWithoutError.hasError, isFalse);
    });

    test('isEmpty returns correct value', () {
      const emptyState = SearchState(results: [], isLoading: false);
      const loadingState = SearchState(results: [], isLoading: true);
      final withDataState = SearchState(
        results: [
          SearchDiscoverItem(
            SearchResultItem(
              itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
              gear: Gear(id: 'test'),
            ),
          ),
        ],
        isLoading: false,
      );

      expect(emptyState.isEmpty, isTrue);
      expect(loadingState.isEmpty, isFalse);
      expect(withDataState.isEmpty, isFalse);
    });
  });

  group('search', () {
    test('sets loading state immediately', () async {
      // Wait for initial build to complete first
      await container.read(searchProvider.future);
      final notifier = container.read(searchProvider.notifier);

      when(
        mockSearchRepository.search(
          query: anyNamed('query'),
          communityIds: anyNamed('communityIds'),
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          maxResults: anyNamed('maxResults'),
        ),
      ).thenAnswer((_) async => []);

      notifier.search(
        query: 'test',
        communityIds: ['community-123'],
        latitudeDeg: 37.7749,
        longitudeDeg: -122.4194,
      );

      final state = container.read(searchProvider).value ?? const SearchState();
      expect(state.isLoading, isTrue);
      expect(state.currentQuery, 'test');
      expect(state.currentCommunityIds, ['community-123']);
    });

    test('debounces multiple rapid calls', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();
        final notifier = container.read(searchProvider.notifier);

        when(
          mockSearchRepository.search(
            query: anyNamed('query'),
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
          ),
        ).thenAnswer((_) async => []);

        // Make multiple rapid search calls
        notifier.search(
          query: 'test1',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        notifier.search(
          query: 'test2',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        notifier.search(
          query: 'test3',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        // Advance past the 300ms debounce window
        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        // Should only call search service once with the last query
        verify(
          mockSearchRepository.search(
            query: 'test3',
            communityIds: ['community-123'],
            latitudeDeg: 37.7749,
            longitudeDeg: -122.4194,
            maxResults: null,
          ),
        ).called(1);
      });
    });

    test('successfully searches and updates results', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();

        final results = [
          SearchResultItem(
            itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
            gear: Gear(id: '1', name: 'Item 1'),
          ),
          SearchResultItem(
            itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
            gear: Gear(id: '2', name: 'Item 2'),
          ),
        ];

        when(
          mockSearchRepository.search(
            query: anyNamed('query'),
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
          ),
        ).thenAnswer((_) async => results);

        final notifier = container.read(searchProvider.notifier);
        notifier.search(
          query: 'test',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        final state = container.read(searchProvider).value ?? const SearchState();
        expect(state.results.length, 2);
        expect(state.results[0].id, '1');
        expect(state.results[1].id, '2');
        expect(state.isLoading, isFalse);
        expect(state.error, isNull);
      });
    });

    test('handles search error', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();

        when(
          mockSearchRepository.search(
            query: anyNamed('query'),
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
          ),
        ).thenThrow(Exception('Network error'));

        final notifier = container.read(searchProvider.notifier);
        notifier.search(
          query: 'test',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        final state = container.read(searchProvider).value ?? const SearchState();
        expect(state.isLoading, isFalse);
        expect(state.error, isNotNull);
        expect(state.results, isEmpty);
      });
    });

    test('ignores stale search results (race condition)', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();

        // Set up first search to take longer (500ms inside the fake zone)
        when(
          mockSearchRepository.search(
            query: 'slow',
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
          ),
        ).thenAnswer((_) async {
          await Future.delayed(const Duration(milliseconds: 500));
          return [
            SearchResultItem(
              itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
              gear: Gear(id: 'slow-result'),
            ),
          ];
        });

        // Set up second search to complete quickly (100ms inside the fake zone)
        when(
          mockSearchRepository.search(
            query: 'fast',
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
          ),
        ).thenAnswer((_) async {
          await Future.delayed(const Duration(milliseconds: 100));
          return [
            SearchResultItem(
              itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
              gear: Gear(id: 'fast-result'),
            ),
          ];
        });

        final notifier = container.read(searchProvider.notifier);

        // Start slow search, advance past debounce to fire the RPC
        notifier.search(
          query: 'slow',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks(); // slow RPC starts, hits its 500ms timer

        // Start fast search (overrides slow)
        notifier.search(
          query: 'fast',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        // Advance past slow RPC (500ms) + fast debounce (300ms) + fast RPC (100ms)
        async.elapse(const Duration(milliseconds: 800));
        async.flushMicrotasks();

        final state = container.read(searchProvider).value ?? const SearchState();
        // Should have fast results, not slow results
        expect(state.results.length, 1);
        expect(state.results[0].id, 'fast-result');
        expect(state.currentQuery, 'fast');
      });
    });

    test('respects maxResults parameter', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();

        when(
          mockSearchRepository.search(
            query: anyNamed('query'),
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: 10,
          ),
        ).thenAnswer((_) async => []);

        final notifier = container.read(searchProvider.notifier);
        notifier.search(
          query: 'test',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
          maxResults: 10,
        );

        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        verify(
          mockSearchRepository.search(
            query: 'test',
            communityIds: ['community-123'],
            latitudeDeg: 37.7749,
            longitudeDeg: -122.4194,
            maxResults: 10,
          ),
        ).called(1);
      });
    });
  });

  group('searchWithLocation', () {
    test('sets loading state and current parameters immediately', () async {
      // Wait for initial build to complete first
      await container.read(searchProvider.future);
      final notifier = container.read(searchProvider.notifier);

      when(
        mockSearchRepository.search(
          query: anyNamed('query'),
          communityIds: anyNamed('communityIds'),
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          maxResults: anyNamed('maxResults'),
        ),
      ).thenAnswer((_) async => []);

      notifier.searchWithLocation(query: 'test', communityIds: ['community-123']);

      final state = container.read(searchProvider).value ?? const SearchState();
      expect(state.isLoading, isTrue);
      expect(state.currentQuery, 'test');
      expect(state.currentCommunityIds, ['community-123']);
    });

    test('sets loading state for non-empty query', () async {
      // Wait for initial build to complete first
      await container.read(searchProvider.future);
      final notifier = container.read(searchProvider.notifier);

      when(
        mockSearchRepository.search(
          query: anyNamed('query'),
          communityIds: anyNamed('communityIds'),
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          maxResults: anyNamed('maxResults'),
        ),
      ).thenAnswer((_) async => []);

      notifier.searchWithLocation(
        query: 'test query',
        communityIds: ['community-123'],
      );

      final state = container.read(searchProvider).value ?? const SearchState();
      expect(state.isLoading, isTrue);
      expect(state.currentQuery, 'test query');
    });

    test('sets loading state for empty query', () async {
      // Wait for initial build to complete first
      await container.read(searchProvider.future);
      final notifier = container.read(searchProvider.notifier);

      when(
        mockSearchRepository.search(
          query: anyNamed('query'),
          communityIds: anyNamed('communityIds'),
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          maxResults: anyNamed('maxResults'),
        ),
      ).thenAnswer((_) async => []);

      notifier.searchWithLocation(query: '', communityIds: ['community-123']);

      final state = container.read(searchProvider).value ?? const SearchState();
      expect(state.isLoading, isTrue);
      expect(state.currentQuery, '');
    });

    test('showModal parameter is accepted without error', () async {
      // showModal is a no-op param kept for call-site compatibility.
      await container.read(searchProvider.future);
      final notifier = container.read(searchProvider.notifier);

      when(
        mockSearchRepository.search(
          query: anyNamed('query'),
          communityIds: anyNamed('communityIds'),
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          maxResults: anyNamed('maxResults'),
        ),
      ).thenAnswer((_) async => []);

      // Should not throw.
      notifier.searchWithLocation(
        query: 'test query',
        communityIds: ['community-123'],
        showModal: false,
      );

      final state = container.read(searchProvider).value ?? const SearchState();
      expect(state.currentQuery, 'test query');
    });

    test('debounces multiple rapid calls', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();
        final notifier = container.read(searchProvider.notifier);

        when(
          mockSearchRepository.search(
            query: anyNamed('query'),
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
          ),
        ).thenAnswer((_) async => []);

        // Make multiple rapid calls
        notifier.searchWithLocation(query: 'test1', communityIds: ['community-123']);
        notifier.searchWithLocation(query: 'test2', communityIds: ['community-123']);
        notifier.searchWithLocation(query: 'test3', communityIds: ['community-123']);

        // Advance past the 300ms debounce; location returns null via microtask
        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        // Should only call search service once with the last query
        verify(
          mockSearchRepository.search(
            query: 'test3',
            communityIds: ['community-123'],
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: null,
          ),
        ).called(1);
      });
    });

    test(
      'executes search with fallback coordinates when location unavailable',
      () {
        // Note: userLocationProvider returns null in tests (no location
        // permission in the test environment), so the fallback coords fire.
        runDebounced((async) {
          container.read(searchProvider.future);
          async.flushMicrotasks();
          final notifier = container.read(searchProvider.notifier);

          when(
            mockSearchRepository.search(
              query: anyNamed('query'),
              communityIds: anyNamed('communityIds'),
              latitudeDeg: anyNamed('latitudeDeg'),
              longitudeDeg: anyNamed('longitudeDeg'),
              maxResults: anyNamed('maxResults'),
            ),
          ).thenAnswer((_) async => []);

          notifier.searchWithLocation(
            query: 'test',
            communityIds: ['community-123'],
          );

          // Advance past debounce; location resolves via microtask
          async.elapse(const Duration(milliseconds: 300));
          async.flushMicrotasks();

          // Verify search was called with some coordinates (fallback: Austin, TX)
          verify(
            mockSearchRepository.search(
              query: 'test',
              communityIds: ['community-123'],
              latitudeDeg: argThat(isA<double>(), named: 'latitudeDeg'),
              longitudeDeg: argThat(isA<double>(), named: 'longitudeDeg'),
              maxResults: null,
            ),
          ).called(1);
        });
      },
    );

    test('updates state with coordinates after location fetch', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();
        final notifier = container.read(searchProvider.notifier);

        when(
          mockSearchRepository.search(
            query: anyNamed('query'),
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
          ),
        ).thenAnswer((_) async => []);

        notifier.searchWithLocation(query: 'test', communityIds: ['community-123']);

        // Advance past debounce; location resolves via microtask
        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        final state = container.read(searchProvider).value ?? const SearchState();
        // Coordinates should be set (either from location service or fallback)
        expect(state.currentLatitude, isNotNull);
        expect(state.currentLongitude, isNotNull);
      });
    });
  });

  group('clear', () {
    test('resets all state to initial values', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();

        // First set some state
        final results = [
          SearchResultItem(
            itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
            gear: Gear(id: '1', name: 'Item 1'),
          ),
        ];

        when(
          mockSearchRepository.search(
            query: anyNamed('query'),
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
          ),
        ).thenAnswer((_) async => results);

        final notifier = container.read(searchProvider.notifier);
        notifier.search(
          query: 'test',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        // Verify state is set
        expect(container.read(searchProvider).value?.results.length ?? 0, 1);

        // Clear
        notifier.clear();

        // Verify state is reset
        final state = container.read(searchProvider).value ?? const SearchState();
        expect(state.results, isEmpty);
        expect(state.isLoading, isFalse);
        expect(state.error, isNull);
        expect(state.currentQuery, '');
        expect(state.currentCommunityIds, isEmpty);
      });
    });

    test('cancels pending debounce timer', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();

        when(
          mockSearchRepository.search(
            query: anyNamed('query'),
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
          ),
        ).thenAnswer((_) async => []);

        final notifier = container.read(searchProvider.notifier);

        // Start search
        notifier.search(
          query: 'test',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        // Clear before debounce completes
        notifier.clear();

        // Advance past debounce window
        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        // Search service should not have been called
        verifyNever(
          mockSearchRepository.search(
            query: anyNamed('query'),
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
          ),
        );
      });
    });
  });

  group('refresh', () {
    test('does nothing if no current search parameters', () async {
      // Wait for initial build to complete first
      await container.read(searchProvider.future);
      final notifier = container.read(searchProvider.notifier);
      await notifier.refresh();

      verifyNever(
        mockSearchRepository.search(
          query: anyNamed('query'),
          communityIds: anyNamed('communityIds'),
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          maxResults: anyNamed('maxResults'),
        ),
      );
    });

    test('refreshes with current search parameters', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();

        final results1 = [
          SearchResultItem(
            itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
            gear: Gear(id: '1', name: 'Item 1'),
          ),
        ];
        final results2 = [
          SearchResultItem(
            itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
            gear: Gear(id: '1', name: 'Item 1'),
          ),
          SearchResultItem(
            itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
            gear: Gear(id: '2', name: 'Item 2'),
          ),
        ];

        when(
          mockSearchRepository.search(
            query: 'test',
            communityIds: ['community-123'],
            latitudeDeg: 37.7749,
            longitudeDeg: -122.4194,
            maxResults: null,
          ),
        ).thenAnswer((_) async => results1);

        final notifier = container.read(searchProvider.notifier);
        notifier.search(
          query: 'test',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();
        expect(container.read(searchProvider).value?.results.length ?? 0, 1);

        // Update mock to return more results
        when(
          mockSearchRepository.search(
            query: 'test',
            communityIds: ['community-123'],
            latitudeDeg: 37.7749,
            longitudeDeg: -122.4194,
            maxResults: null,
          ),
        ).thenAnswer((_) async => results2);

        // Refresh (no debounce — calls _executeSearch directly)
        notifier.refresh();
        async.flushMicrotasks();

        final state = container.read(searchProvider).value ?? const SearchState();
        expect(state.results.length, 2);
      });
    });
  });

  group('filterByPerson', () {
    test('filters results to the selected person', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();

        final results = [
          SearchResultItem(
            itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
            gear: Gear(id: '1', name: 'Tent', owner: User(id: 'user-a')),
          ),
          SearchResultItem(
            itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
            gear: Gear(id: '2', name: 'Kayak', owner: User(id: 'user-b')),
          ),
        ];

        when(
          mockSearchRepository.search(
            query: anyNamed('query'),
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
          ),
        ).thenAnswer((_) async => results);

        final notifier = container.read(searchProvider.notifier);
        notifier.search(
          query: '',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );
        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        // Filter to user-a only.
        notifier.filterByPerson('user-a');

        final state = container.read(searchProvider).value ?? const SearchState();
        expect(state.filters.selectedPersonId, 'user-a');
        expect(state.sharing.length, 1);
        expect(
          state.sharing.first.searchResult.gear.owner.id,
          'user-a',
        );
      });
    });

    test('clears person filter when null is passed', () async {
      await container.read(searchProvider.future);
      final notifier = container.read(searchProvider.notifier);

      notifier.filterByPerson('user-a');
      expect(
        container.read(searchProvider).value?.filters.selectedPersonId,
        'user-a',
      );

      notifier.filterByPerson(null);
      expect(
        container.read(searchProvider).value?.filters.selectedPersonId,
        isNull,
      );
    });
  });

  group('toggleIncludeCompleted', () {
    test('toggles includeCompleted flag and re-triggers search', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();

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

        final notifier = container.read(searchProvider.notifier);
        notifier.search(
          query: '',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );
        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        // Default is false.
        expect(
          container.read(searchProvider).value?.filters.includeCompleted,
          isFalse,
        );

        notifier.toggleIncludeCompleted();

        // Flag should flip immediately.
        expect(
          container.read(searchProvider).value?.filters.includeCompleted,
          isTrue,
        );

        // Advance past debounce + search.
        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        // Search should have been called with includeCompleted: true.
        verify(
          mockSearchRepository.search(
            query: anyNamed('query'),
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
            includeCompleted: true,
          ),
        ).called(1);
      });
    });

    test('toggling twice returns to original value', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();

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

        final notifier = container.read(searchProvider.notifier);
        notifier.search(
          query: '',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );
        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        notifier.toggleIncludeCompleted();
        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        notifier.toggleIncludeCompleted();
        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        expect(
          container.read(searchProvider).value?.filters.includeCompleted,
          isFalse,
        );
      });
    });
  });

  // ---------------------------------------------------------------------------
  // Phase 8 regression tests: initial search must survive community re-emissions
  // ---------------------------------------------------------------------------
  //
  // Bug: CommunitiesNotifier emits 3 times during startup:
  //   1. "Setting communities" — communities list updated, selectedCommunity null
  //   2. "Restored persisted community" — selectedCommunity set
  //   3. "Community selection finalized" — selectedCommunity set (same ID)
  //
  // Old behaviour: build() awaited the location fetch + search RPC. Each
  // re-emission from communitiesProvider caused Riverpod to abandon the
  // in-flight build() and start a new one, so the search RPC fired once per
  // build() invocation (3 times) and only the last result was kept — or, in
  // pathological timing, no result was kept at all.
  //
  // New behaviour: build() schedules _performInitialSearch via
  // Future.microtask, guarded by _initialSearchCommunityId. Re-emissions for
  // the same community ID are ignored; the search fires exactly once.

  group('Phase 8: initial search resilient to community re-emissions', () {
    ProviderContainer? localContainer;

    tearDown(() {
      localContainer?.dispose();
      localContainer = null;
    });

    /// Returns a fresh [ProviderContainer] that uses [communityNotifier] as the
    /// [communitiesProvider] override, with the standard mocks wired in.
    ProviderContainer makeContainer(_ManualCommunityNotifier communityNotifier) {
      localContainer = ProviderContainer(
        overrides: [
          searchRepositoryProvider.overrideWithValue(mockSearchRepository),
          observabilityServiceProvider.overrideWithValue(mockObservability),
          communitiesProvider.overrideWith(() => communityNotifier),
          // Return null immediately instead of calling the real GPS API.
          userLocationProvider.overrideWith((ref, intent) async => null),
        ],
      );
      return localContainer!;
    }

    test(
      'search fires exactly once when community re-emits 3 times with the same ID',
      () {
        runDebounced((async) {
          // Matches the real startup sequence where CommunitiesNotifier
          // emits "Setting communities", "Restored persisted community", and
          // "Community selection finalized" before any search completes.
          final community =
              CommunityItem(id: 'community-123', name: 'Boulder Backcountry Skiers');
          final communityState = CommunitiesState(
            communities: [community],
          );

          var searchCallCount = 0;
          when(
            mockSearchRepository.search(
              query: '',
              communityIds: ['community-123'],
              latitudeDeg: anyNamed('latitudeDeg'),
              longitudeDeg: anyNamed('longitudeDeg'),
              maxResults: null,
            ),
          ).thenAnswer((_) async {
            searchCallCount++;
            return [
              SearchResultItem(
                itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
                gear: Gear(id: 'gear-1', name: 'Test Gear'),
              ),
            ];
          });

          final communityNotifier = _ManualCommunityNotifier();
          final c = makeContainer(communityNotifier);

          // Listen (not just read) so Riverpod eagerly rebuilds when community changes.
          c.listen(searchProvider, (prev, next) {});

          // Simulate the 3-emission startup sequence.
          communityNotifier.emit(communityState); // "Setting communities"
          async.flushMicrotasks(); // allow Riverpod to propagate
          communityNotifier.emit(communityState); // "Restored persisted community"
          async.flushMicrotasks();
          communityNotifier.emit(communityState); // "Community selection finalized"

          // Allow async work: location fetch (null) + RPC to complete via microtasks.
          async.elapse(const Duration(milliseconds: 300));
          async.flushMicrotasks();

          // Search must fire exactly once — not once per build() invocation.
          expect(
            searchCallCount,
            1,
            reason: 'Search RPC should fire exactly once regardless of how many '
                'times the community provider re-emits with the same ID',
          );

          // Results must be populated.
          final state = c.read(searchProvider).value ?? const SearchState();
          expect(state.results.length, 1);
          expect(state.results[0].id, 'gear-1');
          expect(state.isLoading, isFalse);
          expect(state.error, isNull);
        });
      },
    );

    test(
      'search result is not lost when community re-emits during the search RPC',
      () {
        runDebounced((async) {
          // This test pins the exact race: community re-emits WHILE the search
          // RPC is in-flight (100 ms simulated delay). In the old code, build()
          // would be abandoned and the result discarded; the screen stayed empty.
          final community =
              CommunityItem(id: 'community-456', name: 'Test Community');
          final communityState = CommunitiesState(
            communities: [community],
          );

          var searchCallCount = 0;
          when(
            mockSearchRepository.search(
              query: '',
              communityIds: ['community-456'],
              latitudeDeg: anyNamed('latitudeDeg'),
              longitudeDeg: anyNamed('longitudeDeg'),
              maxResults: null,
            ),
          ).thenAnswer((_) async {
            searchCallCount++;
            // Simulate network latency inside the fake zone.
            await Future.delayed(const Duration(milliseconds: 100));
            return [
              SearchResultItem(
                itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
                gear: Gear(id: 'gear-2', name: 'Kayak'),
              ),
            ];
          });

          final communityNotifier = _ManualCommunityNotifier();
          final c = makeContainer(communityNotifier);

          // Listen (not just read) so Riverpod eagerly rebuilds when community changes.
          c.listen(searchProvider, (prev, next) {});

          // Emission 1 — triggers _performInitialSearch (scheduled via microtask).
          communityNotifier.emit(communityState);
          // Let the microtask fire so _performInitialSearch starts.
          async.flushMicrotasks();

          // Re-emit while _performInitialSearch is in-flight (location fetch +
          // search RPC). These must NOT cancel the in-progress search.
          communityNotifier.emit(communityState);
          communityNotifier.emit(communityState);

          // Advance past the 100ms RPC delay and flush remaining work.
          async.elapse(const Duration(milliseconds: 400));
          async.flushMicrotasks();

          // Results must be present — the search was NOT dropped.
          final state = c.read(searchProvider).value ?? const SearchState();
          expect(
            state.results.length,
            1,
            reason: 'Search result must not be lost when community re-emits '
                'while the search RPC is in-flight',
          );
          expect(state.results[0].id, 'gear-2');
          expect(state.isLoading, isFalse);

          // Search fired exactly once.
          expect(
            searchCallCount,
            1,
            reason: 'Search RPC should fire exactly once even with mid-flight re-emissions',
          );
        });
      },
    );

    // Removed in #2023 Phase 2: per-community switching disappeared with the
    // selection field. The "search uses all communities" behavior is covered
    // by the multi-community initial-search tests elsewhere in this file.
  });

  // ---------------------------------------------------------------------------
  // Phase 9 regression tests: error recovery and reliable initial search
  // ---------------------------------------------------------------------------
  //
  // Bug: When the initial search fails, _initialSearchCommunityId is left set
  // so subsequent build() re-invocations never retry. The screen stays empty
  // with an error rather than recovering when the network comes back.
  //
  // Fix: On failure, reset _initialSearchCommunityId = null so the next build()
  // re-invocation (triggered by any provider change) retries the search.

  group('Phase 9: initial search retry on failure', () {
    ProviderContainer? localContainer;

    tearDown(() {
      localContainer?.dispose();
      localContainer = null;
    });

    ProviderContainer makeContainer(_ManualCommunityNotifier communityNotifier) {
      localContainer = ProviderContainer(
        overrides: [
          searchRepositoryProvider.overrideWithValue(mockSearchRepository),
          observabilityServiceProvider.overrideWithValue(mockObservability),
          communitiesProvider.overrideWith(() => communityNotifier),
          userLocationProvider.overrideWith((ref, intent) async => null),
        ],
      );
      return localContainer!;
    }

    test(
      'initial search fires when community becomes available after null state',
      () {
        runDebounced((async) {
          // Simulates the new-user path: community is null on first build(),
          // then set after communities load from the API.
          final community =
              CommunityItem(id: 'community-789', name: 'Test Community');

          when(
            mockSearchRepository.search(
              query: '',
              communityIds: ['community-789'],
              latitudeDeg: anyNamed('latitudeDeg'),
              longitudeDeg: anyNamed('longitudeDeg'),
              maxResults: null,
            ),
          ).thenAnswer(
            (_) async => [
              SearchResultItem(
                itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
                gear: Gear(id: 'gear-new-user', name: 'New User Gear'),
              ),
            ],
          );

          final communityNotifier = _ManualCommunityNotifier();
          final c = makeContainer(communityNotifier);
          // Listen so Riverpod eagerly rebuilds on community changes.
          c.listen(searchProvider, (prev, next) {});

          // Initial build: community is null → empty state, no search.
          async.flushMicrotasks();
          expect(c.read(searchProvider).value?.results, isEmpty);

          // Community becomes available (simulates communities loading after login).
          communityNotifier.emit(
            CommunitiesState(
              communities: [community],
            ),
          );

          // Allow location fetch + search RPC to complete.
          async.elapse(const Duration(milliseconds: 300));
          async.flushMicrotasks();

          final state = c.read(searchProvider).value ?? const SearchState();
          expect(
            state.results.length,
            1,
            reason: 'Initial search must fire when community becomes available',
          );
          expect(state.results[0].id, 'gear-new-user');
          expect(state.isLoading, isFalse);
          expect(state.error, isNull);
        });
      },
    );

    test(
      '_initialSearchCommunityId resets on failure so next build() retries',
      () {
        runDebounced((async) {
          // Simulates a transient network failure: first search fails, then
          // the community provider emits a new state (e.g. setLoading(false) after
          // a community refresh), which triggers a build() re-invocation. Because
          // _initialSearchCommunityId was reset to null on failure, the retry fires.
          final community =
              CommunityItem(id: 'community-retry', name: 'Retry Community');

          var searchCallCount = 0;
          when(
            mockSearchRepository.search(
              query: '',
              communityIds: ['community-retry'],
              latitudeDeg: anyNamed('latitudeDeg'),
              longitudeDeg: anyNamed('longitudeDeg'),
              maxResults: null,
            ),
          ).thenAnswer((_) async {
            searchCallCount++;
            if (searchCallCount == 1) {
              throw Exception('Network error on first attempt');
            }
            return [
              SearchResultItem(
                itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
                gear: Gear(id: 'gear-retry', name: 'Retry Gear'),
              ),
            ];
          });

          final communityNotifier = _ManualCommunityNotifier();
          final c = makeContainer(communityNotifier);
          c.listen(searchProvider, (prev, next) {});

          // First emission — triggers _performInitialSearch which will fail.
          communityNotifier.emit(
            CommunitiesState(
              communities: [community],
            ),
          );
          async.elapse(const Duration(milliseconds: 300));
          async.flushMicrotasks();

          // After failure, _initialSearchCommunityId must be reset.
          // State shows the error.
          final errorState = c.read(searchProvider).value ?? const SearchState();
          expect(
            errorState.error,
            isNotNull,
            reason: 'Error state should be set after first search fails',
          );
          expect(searchCallCount, 1);

          // Emit a structurally different state to trigger a build() re-invocation.
          // Using isLoading: true simulates setLoading(true) being called (which
          // happens in the real app during a community refresh). The state is
          // different from the previous emission (isLoading: false → true), so
          // Riverpod will re-notify and SearchNotifier.build() will re-invoke.
          // Since _initialSearchCommunityId was reset to null on failure, the
          // community != null && _initialSearchCommunityId != community.id check
          // passes and _performInitialSearch is called again.
          communityNotifier.emit(
            CommunitiesState(
              communities: [community],
              isLoading: true,
            ),
          );
          async.elapse(const Duration(milliseconds: 300));
          async.flushMicrotasks();

          final retryState = c.read(searchProvider).value ?? const SearchState();
          expect(
            retryState.results.length,
            1,
            reason: 'Retry must succeed after _initialSearchCommunityId is reset',
          );
          expect(retryState.results[0].id, 'gear-retry');
          expect(retryState.isLoading, isFalse);
          expect(retryState.error, isNull);
          expect(searchCallCount, 2);
        });
      },
    );
  });

  // ---------------------------------------------------------------------------
  // Phase 3: location fetch resilience (timeout, throw, hang)
  // ---------------------------------------------------------------------------
  //
  // Bug: _performInitialSearch called await locationFetcher() with no timeout
  // and no error handling. On iOS, the Geolocator plugin can hang indefinitely
  // when the Mapbox map widget concurrently uses Core Location. The search
  // would block forever with no fallback.
  //
  // Fix: locationFetcher().timeout(3s) with try/catch. On timeout or error,
  // fallback coordinates (Austin, TX: 30.2672, -97.7431) are used so the
  // search always fires.

  group('Phase 3: location fetch resilience', () {
    ProviderContainer? localContainer;

    tearDown(() {
      localContainer?.dispose();
      localContainer = null;
    });

    ProviderContainer makeContainerWithLocation(
      _ManualCommunityNotifier communityNotifier,
      Future<Position?> Function() locationFetcher,
    ) {
      localContainer = ProviderContainer(
        overrides: [
          searchRepositoryProvider.overrideWithValue(mockSearchRepository),
          observabilityServiceProvider.overrideWithValue(mockObservability),
          communitiesProvider.overrideWith(() => communityNotifier),
          userLocationProvider.overrideWith((ref, intent) => locationFetcher()),
        ],
      );
      return localContainer!;
    }

    test(
      'search completes with fallback coordinates when location fetch times out',
      () {
        // Simulates the iOS bug: Geolocator.getCurrentPosition() hangs
        // because Mapbox map widget is concurrently using Core Location.
        // The 3-second timeout should kick in and use fallback coordinates.
        runDebounced((async) {
          final community =
              CommunityItem(id: 'c-timeout', name: 'Timeout Community');

          when(
            mockSearchRepository.search(
              query: '',
              communityIds: ['c-timeout'],
              latitudeDeg: 30.2672, // fallback lat
              longitudeDeg: -97.7431, // fallback lng
              maxResults: null,
            ),
          ).thenAnswer((_) async => [
                SearchResultItem(
                  itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
                  gear: Gear(id: 'gear-timeout', name: 'Timeout Gear'),
                ),
              ]);

          final communityNotifier = _ManualCommunityNotifier();
          // Location fetcher that never completes (simulates iOS hang).
          final c = makeContainerWithLocation(
            communityNotifier,
            () => Completer<Position?>().future,
          );
          c.listen(searchProvider, (_, _) {});

          communityNotifier.emit(CommunitiesState(
            communities: [community],
          ));

          async.flushMicrotasks(); // _performInitialSearch starts, hits 3s timeout timer

          // Advance past the 3s timeout; search fires with fallback coords.
          async.elapse(const Duration(milliseconds: 3500));
          async.flushMicrotasks();

          final state = c.read(searchProvider).value ?? const SearchState();
          expect(
            state.results.length,
            1,
            reason: 'Search must complete with fallback coordinates when '
                'location fetch hangs',
          );
          expect(state.results[0].id, 'gear-timeout');
          expect(state.isLoading, isFalse);
          expect(state.error, isNull);
        });
      },
    );

    test(
      'search completes with fallback coordinates when location is unavailable',
      () {
        runDebounced((async) {
          // Simulates GPS denied / hardware unavailable / no primary residence:
          // userLocationProvider returns null (UserPositionResolver and
          // DeviceLocationService never throw — they swallow errors and return
          // null). The search must still fire using Austin fallback coords.
          final community =
              CommunityItem(id: 'c-null', name: 'Null Location Community');

          when(
            mockSearchRepository.search(
              query: '',
              communityIds: ['c-null'],
              latitudeDeg: 30.2672,
              longitudeDeg: -97.7431,
              maxResults: null,
            ),
          ).thenAnswer((_) async => [
                SearchResultItem(
                  itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
                  gear: Gear(id: 'gear-null-loc', name: 'Null Location Gear'),
                ),
              ]);

          final communityNotifier = _ManualCommunityNotifier();
          final c = makeContainerWithLocation(
            communityNotifier,
            () async => null,
          );
          c.listen(searchProvider, (_, _) {});

          communityNotifier.emit(CommunitiesState(
            communities: [community],
          ));

          async.elapse(const Duration(milliseconds: 300));
          async.flushMicrotasks();

          final state = c.read(searchProvider).value ?? const SearchState();
          expect(
            state.results.length,
            1,
            reason: 'Search must complete with fallback coordinates when '
                'location is unavailable',
          );
          expect(state.results[0].id, 'gear-null-loc');
          expect(state.isLoading, isFalse);
          expect(state.error, isNull);
        });
      },
    );

    test(
      'search uses real coordinates when location fetch succeeds',
      () {
        runDebounced((async) {
          // Control test: when location works, real coordinates are used.
          final community =
              CommunityItem(id: 'c-real-loc', name: 'Real Location Community');

          when(
            mockSearchRepository.search(
              query: '',
              communityIds: ['c-real-loc'],
              latitudeDeg: 37.7749,
              longitudeDeg: -122.4194,
              maxResults: null,
            ),
          ).thenAnswer((_) async => [
                SearchResultItem(
                  itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
                  gear: Gear(id: 'gear-real-loc', name: 'Real Location Gear'),
                ),
              ]);

          final communityNotifier = _ManualCommunityNotifier();
          final c = makeContainerWithLocation(
            communityNotifier,
            () async => Position(
              latitude: 37.7749,
              longitude: -122.4194,
              timestamp: DateTime.now(),
              accuracy: 10,
              altitude: 0,
              altitudeAccuracy: 0,
              heading: 0,
              headingAccuracy: 0,
              speed: 0,
              speedAccuracy: 0,
            ),
          );
          c.listen(searchProvider, (_, _) {});

          communityNotifier.emit(CommunitiesState(
            communities: [community],
          ));

          async.elapse(const Duration(milliseconds: 300));
          async.flushMicrotasks();

          final state = c.read(searchProvider).value ?? const SearchState();
          expect(state.results.length, 1);
          expect(state.results[0].id, 'gear-real-loc');
        });
      },
    );

    test(
      '_initialSearchCommunityId resets when search RPC fails after location fallback',
      () {
        runDebounced((async) {
          // Verifies that if the location falls back AND the search RPC also
          // fails, _initialSearchCommunityId is reset so a retry can fire.
          final community =
              CommunityItem(id: 'c-double-fail', name: 'Double Fail Community');
          var searchCallCount = 0;

          when(
            mockSearchRepository.search(
              query: '',
              communityIds: ['c-double-fail'],
              latitudeDeg: 30.2672,
              longitudeDeg: -97.7431,
              maxResults: null,
            ),
          ).thenAnswer((_) async {
            searchCallCount++;
            if (searchCallCount == 1) {
              throw Exception('Server unreachable');
            }
            return [
              SearchResultItem(
                itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
                gear: Gear(id: 'gear-recovered', name: 'Recovered Gear'),
              ),
            ];
          });

          final communityNotifier = _ManualCommunityNotifier();
          final c = makeContainerWithLocation(
            communityNotifier,
            () async => null,
          );
          c.listen(searchProvider, (_, _) {});

          // First attempt: location null → fallback coords, search RPC throws.
          communityNotifier.emit(CommunitiesState(
            communities: [community],
          ));
          async.elapse(const Duration(milliseconds: 300));
          async.flushMicrotasks();

          final errorState = c.read(searchProvider).value ?? const SearchState();
          expect(errorState.error, isNotNull,
              reason: 'Error state should be set after both location and search fail');
          expect(searchCallCount, 1);

          // Emit different state to trigger build() re-invocation.
          communityNotifier.emit(CommunitiesState(
            communities: [community],
            isLoading: true,
          ));
          async.elapse(const Duration(milliseconds: 300));
          async.flushMicrotasks();

          // Retry must succeed because _initialSearchCommunityId was reset.
          final retryState = c.read(searchProvider).value ?? const SearchState();
          expect(retryState.results.length, 1,
              reason: 'Retry must succeed after _initialSearchCommunityId is reset');
          expect(retryState.results[0].id, 'gear-recovered');
          expect(retryState.isLoading, isFalse);
          expect(retryState.error, isNull);
          expect(searchCallCount, 2);
        });
      },
    );
  });

  group('timer disposal', () {
    test('disposes timer when provider is disposed', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();

        when(
          mockSearchRepository.search(
            query: anyNamed('query'),
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
          ),
        ).thenAnswer((_) async => []);

        final notifier = container.read(searchProvider.notifier);

        // Start search
        notifier.search(
          query: 'test',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        // Dispose container before debounce completes
        container.dispose();

        // Advance past debounce window
        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        // Search service should not have been called because timer was cancelled
        verifyNever(
          mockSearchRepository.search(
            query: anyNamed('query'),
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
          ),
        );
      });
    });
  });

  group('cache invalidation and refresh', () {
    test(
      'REPRODUCES BUG: UI should update when cache invalidation triggers refresh with new data',
      () {
        runDebounced((async) {
          container.read(searchProvider.future);
          async.flushMicrotasks();

          // Initial search results with old mediaId
          final initialResults = [
            SearchResultItem(
              itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
              gear: Gear(
                id: 'gear-123',
                name: 'Test Gear',
                mediaIds: ['old-media-id'],
              ),
            ),
          ];

          // Updated results with new mediaId (simulating user editing the item)
          final updatedResults = [
            SearchResultItem(
              itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
              gear: Gear(
                id: 'gear-123',
                name: 'Test Gear',
                mediaIds: ['new-media-id'], // Changed media
              ),
            ),
          ];

          // First search returns initial results
          when(
            mockSearchRepository.search(
              query: '',
              communityIds: ['community-123'],
              latitudeDeg: 37.7749,
              longitudeDeg: -122.4194,
              maxResults: null,
            ),
          ).thenAnswer((_) async => initialResults);

          final notifier = container.read(searchProvider.notifier);
          notifier.search(
            query: '',
            communityIds: ['community-123'],
            latitudeDeg: 37.7749,
            longitudeDeg: -122.4194,
          );

          async.elapse(const Duration(milliseconds: 300));
          async.flushMicrotasks();

          // Verify initial state
          var state = container.read(searchProvider).value ?? const SearchState();
          expect(state.results.length, 1);
          final initialItem = state.results[0] as SearchDiscoverItem;
          expect(initialItem.searchResult.gear.mediaIds, ['old-media-id']);

          // Simulate cache invalidation (user edited the gear and updated media)
          // Mock now returns updated results
          when(
            mockSearchRepository.search(
              query: '',
              communityIds: ['community-123'],
              latitudeDeg: 37.7749,
              longitudeDeg: -122.4194,
              maxResults: null,
            ),
          ).thenAnswer((_) async => updatedResults);

          // Trigger cache invalidation (simulating the notification from searchCacheInvalidationProvider)
          // In the real app, this happens when GearRepository.saveGear() calls searchRepository.invalidateSearches()
          notifier.refresh();
          async.flushMicrotasks();

          // BUG: The state is updated but UI doesn't reflect changes
          state = container.read(searchProvider).value ?? const SearchState();
          expect(state.results.length, 1);

          final updatedItem = state.results[0] as SearchDiscoverItem;

          // This assertion SHOULD pass - the state has the new data
          expect(updatedItem.searchResult.gear.mediaIds, ['new-media-id']);

          // BUG: In the real app, even though this assertion passes (state has new data),
          // the UI still shows the old thumbnail because:
          // 1. Riverpod might not notify listeners (reference equality)
          // 2. Flutter reuses widgets with same keys (same gear ID)
          //
          // This test PASSES because we're only checking state, not UI rendering.
          // To reproduce the actual bug, we would need a widget test that:
          // 1. Renders the discover screen
          // 2. Verifies old thumbnail is shown
          // 3. Triggers cache invalidation
          // 4. Verifies old thumbnail is STILL shown (BUG)
        });
      },
    );
  });

  group('analytics', () {
    test(
      'logs SearchPerformedEvent on successful search with non-empty query',
      () {
        runDebounced((async) {
          container.read(searchProvider.future);
          async.flushMicrotasks();

          final results = [
            SearchResultItem(
              itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
              gear: Gear(id: '1', name: 'Item 1'),
            ),
            SearchResultItem(
              itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
              gear: Gear(id: '2', name: 'Item 2'),
            ),
          ];

          when(
            mockSearchRepository.search(
              query: anyNamed('query'),
              communityIds: anyNamed('communityIds'),
              latitudeDeg: anyNamed('latitudeDeg'),
              longitudeDeg: anyNamed('longitudeDeg'),
              maxResults: anyNamed('maxResults'),
            ),
          ).thenAnswer((_) async => results);

          final notifier = container.read(searchProvider.notifier);
          notifier.search(
            query: 'tent',
            communityIds: ['community-123'],
            latitudeDeg: 37.7749,
            longitudeDeg: -122.4194,
          );

          async.elapse(const Duration(milliseconds: 300));
          async.flushMicrotasks();

          expect(
            mockObservability.hasEventOfType<SearchPerformedEvent>(),
            isTrue,
          );
          final event = mockObservability.lastEventOfType<SearchPerformedEvent>();
          expect(event?.parameters['query_length'], 4); // "tent" = 4 chars
          expect(event?.parameters['result_count'], 2);
          expect(event?.parameters['search_type'], 'all');
        });
      },
    );

    test('does not log SearchPerformedEvent for empty query', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();

        when(
          mockSearchRepository.search(
            query: anyNamed('query'),
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
          ),
        ).thenAnswer((_) async => []);

        final notifier = container.read(searchProvider.notifier);
        notifier.search(
          query: '',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        // Empty query should not trigger analytics
        expect(mockObservability.hasEventOfType<SearchPerformedEvent>(), isFalse);
      });
    });

    test('does not log SearchPerformedEvent on search error', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();

        when(
          mockSearchRepository.search(
            query: anyNamed('query'),
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
          ),
        ).thenThrow(Exception('Network error'));

        final notifier = container.read(searchProvider.notifier);
        notifier.search(
          query: 'test',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        // Error should not trigger analytics
        expect(mockObservability.hasEventOfType<SearchPerformedEvent>(), isFalse);
      });
    });

    test('logs correct result count including all item types', () {
      runDebounced((async) {
        container.read(searchProvider.future);
        async.flushMicrotasks();

        final results = [
          SearchResultItem(
            itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
            gear: Gear(id: '1', name: 'Tent'),
          ),
          SearchResultItem(
            itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
            gear: Gear(id: '2', name: 'Sleeping Bag'),
          ),
          SearchResultItem(
            itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
            gear: Gear(id: '3', name: 'Camping Stove'),
          ),
        ];

        when(
          mockSearchRepository.search(
            query: anyNamed('query'),
            communityIds: anyNamed('communityIds'),
            latitudeDeg: anyNamed('latitudeDeg'),
            longitudeDeg: anyNamed('longitudeDeg'),
            maxResults: anyNamed('maxResults'),
          ),
        ).thenAnswer((_) async => results);

        final notifier = container.read(searchProvider.notifier);
        notifier.search(
          query: 'camping',
          communityIds: ['community-123'],
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );

        async.elapse(const Duration(milliseconds: 300));
        async.flushMicrotasks();

        final event = mockObservability.lastEventOfType<SearchPerformedEvent>();
        expect(event?.parameters['query_length'], 7); // "camping" = 7 chars
        expect(event?.parameters['result_count'], 3);
      });
    });
  });
}
