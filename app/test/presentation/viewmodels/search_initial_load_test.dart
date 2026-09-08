// Integration tests for the full login → community load → search flow.
//
// These tests verify that search results appear reliably regardless of how many
// times communitiesProvider emits during startup, including the race
// conditions described in docs/ai/load.md.

import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Gear;
import 'package:ripls/data/gen/ripls/api/search_service.pb.dart';
import 'package:ripls/presentation/viewmodels/search_view_model.dart';
import 'package:ripls/services/providers.dart';

import '../../core/observability/analytics_test_helper.dart';
import '../../helpers/fake_async_helpers.dart';
import 'search_view_model_test.mocks.dart';

/// Controllable community notifier for integration tests.
class _ManualCommunityNotifier extends CommunitiesNotifier {
  @override
  CommunitiesState build() => const CommunitiesState();

  void emit(CommunitiesState newState) => state = newState;
}

/// Builds a [SearchResultItem] with a single gear entry.
SearchResultItem _gearResult(String id, {String? name}) {
  return SearchResultItem(
    itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
    gear: Gear(id: id, name: name ?? id),
  );
}

void main() {
  late MockSearchRepository mockSearchRepository;
  late MockObservabilityService mockObservability;

  setUp(() {
    mockSearchRepository = MockSearchRepository();
    mockObservability = MockObservabilityService();
  });

  /// Creates a [ProviderContainer] wired for initial-load integration tests.
  ProviderContainer makeContainer(
    _ManualCommunityNotifier communityNotifier,
  ) {
    return ProviderContainer(
      overrides: [
        searchRepositoryProvider.overrideWithValue(mockSearchRepository),
        observabilityServiceProvider.overrideWithValue(mockObservability),
        communitiesProvider.overrideWith(() => communityNotifier),
        userLocationProvider.overrideWith((ref, intent) async => null),
      ],
    );
  }

  // ---------------------------------------------------------------------------
  // Happy path
  // ---------------------------------------------------------------------------

  group('happy path', () {
    test('search results appear after single community emission',
        () => runDebounced((async) {
      // The ideal path: communitiesProvider emits exactly once with
      // communities set.
      final community = CommunityItem(id: 'c-happy', name: 'Happy Community');

      when(
        mockSearchRepository.search(
          query: '',
          communityIds: ['c-happy'],
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          maxResults: null,
        ),
      ).thenAnswer((_) async => [_gearResult('gear-happy')]);

      final communityNotifier = _ManualCommunityNotifier();
      final container = makeContainer(communityNotifier);
      addTearDown(container.dispose);

      container.listen(searchProvider, (_, _) {});

      // Single atomic emission (Phase 1 outcome).
      communityNotifier.emit(CommunitiesState(
        communities: [community],
        isLoading: false,
      ));

      async.elapse(const Duration(milliseconds: 300));
      async.flushMicrotasks();

      final state = container.read(searchProvider).value ?? const SearchState();
      expect(state.results.length, 1,
          reason: 'Search results must appear after single emission');
      expect(state.results[0].id, 'gear-happy');
      expect(state.isLoading, isFalse);
      expect(state.error, isNull);
    }));
  });

  // ---------------------------------------------------------------------------
  // Multi-emission startup sequence
  // ---------------------------------------------------------------------------

  group('multi-emission startup', () {
    test(
        'search results appear when community provider emits multiple times with same community',
        () => runDebounced((async) {
      // Simulates the pre-Phase-1 startup sequence where setCommunities emitted
      // 2 state changes (once with empty communities, once populated) plus a
      // duplicate from main.dart listener.
      final community =
          CommunityItem(id: 'c-multi', name: 'Multi Emit Community');

      when(
        mockSearchRepository.search(
          query: '',
          communityIds: ['c-multi'],
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          maxResults: null,
        ),
      ).thenAnswer((_) async => [_gearResult('gear-multi')]);

      final communityNotifier = _ManualCommunityNotifier();
      final container = makeContainer(communityNotifier);
      addTearDown(container.dispose);

      container.listen(searchProvider, (_, _) {});

      // Emission 1: communities set, empty portfolio.
      communityNotifier.emit(const CommunitiesState(
        communities: [],
        isLoading: false,
      ));
      async.flushMicrotasks();

      // Emission 2: community selection now set.
      communityNotifier.emit(CommunitiesState(
        communities: [community],
        isLoading: false,
      ));
      async.flushMicrotasks();

      // Emission 3: duplicate from main.dart listener.
      communityNotifier.emit(CommunitiesState(
        communities: [community],
        isLoading: false,
      ));

      async.elapse(const Duration(milliseconds: 300));
      async.flushMicrotasks();

      final state = container.read(searchProvider).value ?? const SearchState();
      expect(state.results.length, 1,
          reason: 'Results must appear even with multiple community emissions');
      expect(state.results[0].id, 'gear-multi');
      expect(state.isLoading, isFalse);
    }));

    test('search fires exactly once across multiple same-community emissions',
        () => runDebounced((async) {
      final community = CommunityItem(id: 'c-once', name: 'Once Community');
      var searchCallCount = 0;

      when(
        mockSearchRepository.search(
          query: '',
          communityIds: ['c-once'],
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          maxResults: null,
        ),
      ).thenAnswer((_) async {
        searchCallCount++;
        return [_gearResult('gear-once')];
      });

      final communityNotifier = _ManualCommunityNotifier();
      final container = makeContainer(communityNotifier);
      addTearDown(container.dispose);

      container.listen(searchProvider, (_, _) {});

      // Six rapid emissions — only one search should fire.
      for (var i = 0; i < 6; i++) {
        communityNotifier.emit(CommunitiesState(
          communities: [community],
          ));
        async.flushMicrotasks();
      }

      async.elapse(const Duration(milliseconds: 300));
      async.flushMicrotasks();

      expect(searchCallCount, 1,
          reason: 'Search RPC must fire exactly once despite 6 emissions');
    }));
  });

  // ---------------------------------------------------------------------------
  // Cold start (already authenticated)
  // ---------------------------------------------------------------------------

  group('cold start', () {
    test('search results appear on cold start (already authenticated)',
        () => runDebounced((async) {
      // On cold start, main.dart._loadCommunities() fires during init —
      // no prior null-community phase.
      final community =
          CommunityItem(id: 'c-cold', name: 'Cold Start Community');

      when(
        mockSearchRepository.search(
          query: '',
          communityIds: ['c-cold'],
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          maxResults: null,
        ),
      ).thenAnswer((_) async => [_gearResult('gear-cold')]);

      final communityNotifier = _ManualCommunityNotifier();
      final container = makeContainer(communityNotifier);
      addTearDown(container.dispose);

      container.listen(searchProvider, (_, _) {});

      // Immediate single emission — no prior null state.
      communityNotifier.emit(CommunitiesState(
        communities: [community],
        isLoading: false,
      ));

      async.elapse(const Duration(milliseconds: 300));
      async.flushMicrotasks();

      final state = container.read(searchProvider).value ?? const SearchState();
      expect(state.results.length, 1,
          reason: 'Cold start must produce search results');
      expect(state.results[0].id, 'gear-cold');
    }));
  });

  // ---------------------------------------------------------------------------
  // Fresh login
  // ---------------------------------------------------------------------------

  group('fresh login', () {
    test('search results appear on fresh login', () => runDebounced((async) {
      // Simulates the fresh login path: auth transitions, main.dart listener
      // fires, communities load, single atomic emission occurs (Phase 2 fix —
      // login_screen no longer triggers a second load).
      final community =
          CommunityItem(id: 'c-login', name: 'Login Community');

      when(
        mockSearchRepository.search(
          query: '',
          communityIds: ['c-login'],
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          maxResults: null,
        ),
      ).thenAnswer((_) async => [_gearResult('gear-login')]);

      final communityNotifier = _ManualCommunityNotifier();
      final container = makeContainer(communityNotifier);
      addTearDown(container.dispose);

      container.listen(searchProvider, (_, _) {});

      communityNotifier.emit(CommunitiesState(
        communities: [community],
        isLoading: false,
      ));

      async.elapse(const Duration(milliseconds: 300));
      async.flushMicrotasks();

      final state = container.read(searchProvider).value ?? const SearchState();
      expect(state.results.length, 1,
          reason: 'Fresh login must produce search results');
      expect(state.results[0].id, 'gear-login');
      expect(state.isLoading, isFalse);
    }));
  });

  // ---------------------------------------------------------------------------
  // Concurrent setCommunities (pre-fix race simulation)
  // ---------------------------------------------------------------------------

  group('concurrent setCommunities', () {
    test(
        'search eventually completes when two rapid emissions arrive with slow RPC',
        () => runDebounced((async) {
      // Validates Phase 3 safety net: two near-simultaneous emissions land.
      // The first triggers _performInitialSearch; the second re-emits the same
      // community but build() guard prevents a duplicate search. The in-flight
      // search must still complete and populate results.
      final community = CommunityItem(id: 'c-race', name: 'Race Community');
      var searchCallCount = 0;

      when(
        mockSearchRepository.search(
          query: '',
          communityIds: ['c-race'],
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          maxResults: null,
        ),
      ).thenAnswer((_) async {
        searchCallCount++;
        // Slow network — wide enough for second emission to land mid-flight.
        await Future.delayed(const Duration(milliseconds: 50));
        return [_gearResult('gear-race')];
      });

      final communityNotifier = _ManualCommunityNotifier();
      final container = makeContainer(communityNotifier);
      addTearDown(container.dispose);

      container.listen(searchProvider, (_, _) {});

      // Emission 1 — triggers _performInitialSearch.
      communityNotifier.emit(CommunitiesState(
        communities: [community],
        isLoading: false,
      ));

      async.flushMicrotasks();

      // Emission 2 arrives immediately (simulates pre-fix race).
      communityNotifier.emit(CommunitiesState(
        communities: [community],
        isLoading: false,
      ));

      // Covers the 300ms debounce plus the 50ms simulated RPC latency.
      async.elapse(const Duration(milliseconds: 350));
      async.flushMicrotasks();

      final state = container.read(searchProvider).value ?? const SearchState();
      expect(state.results, isNotEmpty,
          reason: 'Search results must survive concurrent setCommunities calls');
      expect(state.results[0].id, 'gear-race');
      expect(searchCallCount, 1,
          reason: 'Search RPC must fire exactly once despite concurrent emissions');
    }));
  });

  // ---------------------------------------------------------------------------
  // Disposal during initial search
  // ---------------------------------------------------------------------------

  group('disposal', () {
    test('disposal during initial search completes without throwing',
        () => runDebounced((async) {
      // Per architecture.md disposal testing requirement: start search,
      // dispose container, verify no exceptions are thrown.
      final community =
          CommunityItem(id: 'c-dispose', name: 'Dispose Community');
      final searchCompleter = Completer<List<SearchResultItem>>();

      when(
        mockSearchRepository.search(
          query: '',
          communityIds: ['c-dispose'],
          latitudeDeg: anyNamed('latitudeDeg'),
          longitudeDeg: anyNamed('longitudeDeg'),
          maxResults: null,
        ),
      ).thenAnswer((_) => searchCompleter.future);

      final communityNotifier = _ManualCommunityNotifier();
      final container = makeContainer(communityNotifier);

      container.listen(searchProvider, (_, _) {});

      // Trigger initial search.
      communityNotifier.emit(CommunitiesState(
        communities: [community],
        isLoading: false,
      ));

      // Let the search start (microtask + location fetch microtask).
      async.flushMicrotasks();

      // Dispose the container while the search is still in-flight.
      container.dispose();

      // Complete the search after disposal — must not throw.
      expect(
        () => searchCompleter.complete([_gearResult('gear-dispose')]),
        returnsNormally,
      );

      // Allow any pending microtasks to drain.
      async.flushMicrotasks();
    }));
  });
}
