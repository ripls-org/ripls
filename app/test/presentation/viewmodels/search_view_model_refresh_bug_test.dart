import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Gear;
import 'package:ripls/data/gen/ripls/api/search_service.pb.dart';
import 'package:ripls/presentation/models/discover_item.dart';
import 'package:ripls/presentation/viewmodels/search_view_model.dart';
import 'package:ripls/services/providers.dart';

import '../../core/observability/analytics_test_helper.dart';
import '../../helpers/fake_async_helpers.dart';
import 'search_view_model_test.mocks.dart';

/// This test reproduces Phase 1.2 Bug: UI doesn't update when item data changes
///
/// **Problem Summary:**
/// When users edit items (change media, name, description) and navigate back:
/// - ✅ Scroll position is preserved (SearchViewModel stays alive)
/// - ✅ Search cache is invalidated correctly
/// - ✅ Fresh data is fetched from server with updated mediaIds
/// - ✅ SearchViewModel state is updated with new results
/// - ❌ **BUG:** UI doesn't re-render with new data - thumbnails show old media
///
/// **Root Cause:**
/// Riverpod reference equality check - when we do:
///   `state = state.copyWith(results: newResults)`
///
/// Even though we create a new SearchState object, Riverpod may not notify
/// listeners because the state "looks the same" (same list length, same item IDs).
///
/// **Expected Fix (Option B - refreshCount):**
///   `state = state.copyWith(results: newResults, refreshCount: state.refreshCount + 1)`
///
/// This guarantees Riverpod will detect a state change and notify all listeners.
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

  group('Phase 1.2: In-Place Refresh Bug', () {
    test('REPRODUCES BUG: state updates but same object references may prevent UI update',
        () => runDebounced((async) {
      // The AsyncNotifier build is driven here rather than in setUp so it
      // resolves under FakeAsync's control.
      container.read(searchProvider.future);
      async.flushMicrotasks();

      const gearId = 'gear-123';
      const communityId = 'community-123';

      // STEP 1: Initial search with old media
      final initialResults = [
        SearchResultItem(
          itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
          gear: Gear(
            id: gearId,
            name: 'Test Gear',
            mediaIds: ['old-media-id'],
          ),
        ),
      ];

      when(mockSearchRepository.search(
        query: '',
        communityIds: [communityId],
        latitudeDeg: 37.7749,
        longitudeDeg: -122.4194,
        maxResults: null,
      )).thenAnswer((_) async => initialResults);

      final notifier = container.read(searchProvider.notifier);
      notifier.search(
        query: '',
        communityIds: [communityId],
        latitudeDeg: 37.7749,
        longitudeDeg: -122.4194,
      );

      // Fire the debounce timer and let the mocked RPC settle.
      async.elapse(const Duration(milliseconds: 300));
      async.flushMicrotasks();

      // Verify initial state
      var state = container.read(searchProvider).value ?? const SearchState();
      expect(state.results.length, 1);

      final initialItem = state.results[0] as SearchDiscoverItem;
      expect(initialItem.searchResult.gear.id, gearId);
      expect(initialItem.searchResult.gear.mediaIds, ['old-media-id']);

      // Capture initial state object reference
      final initialStateRef = state;
      final initialResultsRef = state.results;

      // STEP 2: Simulate user editing gear (changed media)
      final updatedResults = [
        SearchResultItem(
          itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
          gear: Gear(
            id: gearId,
            name: 'Test Gear',
            mediaIds: ['new-media-id'], // ← Changed media
          ),
        ),
      ];

      when(mockSearchRepository.search(
        query: '',
        communityIds: [communityId],
        latitudeDeg: 37.7749,
        longitudeDeg: -122.4194,
        maxResults: null,
      )).thenAnswer((_) async => updatedResults);

      // STEP 3: Trigger refresh (cache invalidation)
      notifier.refresh();
      async.flushMicrotasks();

      // Verify state was updated
      state = container.read(searchProvider).value ?? const SearchState();
      expect(state.results.length, 1);

      final updatedItem = state.results[0] as SearchDiscoverItem;
      expect(updatedItem.searchResult.gear.mediaIds, ['new-media-id']);

      // CRITICAL BUG ANALYSIS:
      // The state object IS different (new reference) ✅
      expect(identical(state, initialStateRef), isFalse,
          reason: 'SearchState object should be a new instance');

      // The results list IS different (new reference) ✅
      expect(identical(state.results, initialResultsRef), isFalse,
          reason: 'results list should be a new instance');

      // ✅ Data is correct - state has new mediaIds
      // ✅ State object is new
      // ✅ Results list is new
      //
      // So why doesn't the UI update in the real app?
      //
      // **Hypothesis:**
      // Riverpod's default equality check (==) might compare SearchState objects
      // and determine they're "equal" even though they're different references.
      //
      // Freezed generates == and hashCode based on field values.
      // If results list "looks the same" (same length, same item IDs),
      // Riverpod might skip notifying listeners.
      //
      // **Solution (Option B):**
      // Add refreshCount field that increments on each refresh.
      // This guarantees state will never be "equal" to previous state.
    }));

    test('VERIFIES FIX: refreshCount increments and triggers Riverpod notification',
        () => runDebounced((async) {
      // This test verifies that Option B (refreshCount) fixes the bug
      // by forcing Riverpod to always consider the state as "changed"

      container.read(searchProvider.future);
      async.flushMicrotasks();

      const communityId = 'community-123';

      final results = [
        SearchResultItem(
          itemType: SearchItemType.SEARCH_ITEM_TYPE_GEAR,
          gear: Gear(id: '1', name: 'Item 1'),
        ),
      ];

      when(mockSearchRepository.search(
        query: '',
        communityIds: [communityId],
        latitudeDeg: 37.7749,
        longitudeDeg: -122.4194,
        maxResults: null,
      )).thenAnswer((_) async => results);

      final notifier = container.read(searchProvider.notifier);
      notifier.search(
        query: '',
        communityIds: [communityId],
        latitudeDeg: 37.7749,
        longitudeDeg: -122.4194,
      );

      async.elapse(const Duration(milliseconds: 300));
      async.flushMicrotasks();

      // Verify initial refreshCount is 0
      var state = container.read(searchProvider).value ?? const SearchState();
      expect(state.refreshCount, 0, reason: 'Initial refreshCount should be 0');

      // Listen for state changes AFTER initial search
      var notificationCount = 0;
      AsyncValue<SearchState>? lastNotifiedState;
      container.listen(
        searchProvider,
        (previous, next) {
          notificationCount++;
          lastNotifiedState = next;
        },
      );

      // Trigger refresh with SAME data
      notifier.refresh();
      async.flushMicrotasks();

      // ✅ FIX VERIFICATION: refreshCount should have incremented
      state = container.read(searchProvider).value ?? const SearchState();
      expect(state.refreshCount, 1, reason: 'refreshCount should increment to 1 after first refresh');

      // ✅ FIX VERIFICATION: Riverpod SHOULD have notified listeners
      expect(notificationCount, greaterThan(0),
          reason: 'Riverpod should notify listeners when refreshCount changes');

      expect(lastNotifiedState?.value?.refreshCount, 1,
          reason: 'Notified state should have refreshCount = 1');

      // Trigger second refresh
      notifier.refresh();
      async.flushMicrotasks();

      state = container.read(searchProvider).value ?? const SearchState();
      expect(state.refreshCount, 2, reason: 'refreshCount should increment to 2 after second refresh');

      // Should have gotten another notification
      expect(notificationCount, greaterThan(1),
          reason: 'Riverpod should notify on each refresh');
    }));
  });
}
