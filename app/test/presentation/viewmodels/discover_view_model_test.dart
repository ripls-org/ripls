import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show Request;
import 'package:ripls/data/gen/ripls/api/user_service.pb.dart';
import 'package:ripls/data/repositories/user_repository.dart';
import 'package:ripls/presentation/models/discover_item.dart';
import 'package:ripls/presentation/viewmodels/discover_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'discover_view_model_test.mocks.dart';

@GenerateMocks([
  UserRepository,
])
void main() {
  late ProviderContainer container;
  late MockUserRepository mockUserRepository;

  setUp(() {
    mockUserRepository = MockUserRepository();
    container = ProviderContainer(
      overrides: [
        userRepositoryProvider.overrideWithValue(mockUserRepository),
      ],
    );
  });

  tearDown(() {
    container.dispose();
  });

  group('DiscoverState', () {
    test('initial state is correct', () {
      final state = container.read(discoverProvider);

      expect(state.items, isEmpty);
      expect(state.isLoading, isTrue);
      expect(state.error, isNull);
      expect(state.selectedItem, isNull);
    });

    test('hasError returns correct value', () {
      const stateWithError =
          DiscoverState(error: UserError.generic(fallback: 'Error'));
      final stateWithoutError = const DiscoverState();

      expect(stateWithError.hasError, isTrue);
      expect(stateWithoutError.hasError, isFalse);
    });

    test('isEmpty returns correct value', () {
      final emptyState = const DiscoverState(items: [], isLoading: false);
      final loadingState = const DiscoverState(items: [], isLoading: true);
      final withDataState = DiscoverState(
        items: [GearDiscoverItem(CommunityGearItem())],
        isLoading: false,
      );

      expect(emptyState.isEmpty, isTrue);
      expect(loadingState.isEmpty, isFalse);
      expect(withDataState.isEmpty, isFalse);
    });

    test('itemsWithLocations filters gear items correctly', () {
      final gear1 = CommunityGearItem(
        id: '1',
        latitudeDeg: 37.7749,
        longitudeDeg: -122.4194,
      );
      final gear2 = CommunityGearItem(id: '2', latitudeDeg: 0, longitudeDeg: 0);
      final gear3 = CommunityGearItem(
        id: '3',
        latitudeDeg: 40.7128,
        longitudeDeg: -74.0060,
      );

      final state = DiscoverState(
        items: [
          GearDiscoverItem(gear1),
          GearDiscoverItem(gear2),
          GearDiscoverItem(gear3),
        ],
        isLoading: false,
      );

      final withLocations = state.itemsWithLocations;
      expect(withLocations.length, 2);
      expect(withLocations[0].id, '1');
      expect(withLocations[1].id, '3');
    });

    test('itemsWithLocations filters gear and requests correctly', () {
      final gear1 = CommunityGearItem(
        id: 'gear1',
        latitudeDeg: 37.7749,
        longitudeDeg: -122.4194,
      );
      final gear2 = CommunityGearItem(
        id: 'gear2',
        latitudeDeg: 0,
        longitudeDeg: 0,
      );
      final request1 = Request(
        id: 'req1',
        locationId: 'loc-123',
        latitudeDeg: 40.7128,
        longitudeDeg: -74.0060,
      );
      final request2 = Request(
        id: 'req2',
        locationId: '',
        latitudeDeg: 0,
        longitudeDeg: 0,
      );

      final state = DiscoverState(
        items: [
          GearDiscoverItem(gear1),
          GearDiscoverItem(gear2),
          RequestDiscoverItem(request1),
          RequestDiscoverItem(request2),
        ],
        isLoading: false,
      );

      final withLocations = state.itemsWithLocations;
      expect(withLocations.length, 2);
      expect(withLocations[0].id, 'gear1');
      expect(withLocations[1].id, 'req1');
    });

    test('selectedItemIndex returns correct index', () {
      final gear1 = CommunityGearItem(id: 'gear1');
      final gear2 = CommunityGearItem(id: 'gear2');
      final request1 = Request(id: 'req1');

      final state = DiscoverState(
        items: [
          GearDiscoverItem(gear1),
          RequestDiscoverItem(request1),
          GearDiscoverItem(gear2),
        ],
        selectedItem: RequestDiscoverItem(request1),
        isLoading: false,
      );

      expect(state.selectedItemIndex, 1);
    });
  });

  group('clear', () {
    test('resets all state to initial values', () {
      final gear = CommunityGearItem(id: '1', name: 'Gear 1');

      final notifier = container.read(discoverProvider.notifier);
      notifier.selectItem(GearDiscoverItem(gear));

      // Verify selected item is set
      expect(container.read(discoverProvider).selectedItem, isNotNull);

      // Clear
      notifier.clear();

      // Verify state is reset
      final state = container.read(discoverProvider);
      expect(state.items, isEmpty);
      expect(state.isLoading, isFalse);
      expect(state.error, isNull);
      expect(state.selectedItem, isNull);
    });
  });

  group('selectItem', () {
    test('sets selected item for gear', () {
      final gear = CommunityGearItem(id: '1', name: 'Test Gear');
      final item = GearDiscoverItem(gear);

      final notifier = container.read(discoverProvider.notifier);
      notifier.selectItem(item);

      final state = container.read(discoverProvider);
      expect(state.selectedItem, item);
      expect(state.selectedItem?.id, '1');
    });

    test('sets selected item for request', () {
      final request = Request(id: 'req1', title: 'Test Request');
      final item = RequestDiscoverItem(request);

      final notifier = container.read(discoverProvider.notifier);
      notifier.selectItem(item);

      final state = container.read(discoverProvider);
      expect(state.selectedItem, item);
      expect(state.selectedItem?.id, 'req1');
    });

    test('replaces previously selected item', () {
      final gear = GearDiscoverItem(CommunityGearItem(id: '1'));
      final request = RequestDiscoverItem(Request(id: 'req1'));

      final notifier = container.read(discoverProvider.notifier);
      notifier.selectItem(gear);
      expect(container.read(discoverProvider).selectedItem?.id, '1');

      notifier.selectItem(request);
      expect(container.read(discoverProvider).selectedItem?.id, 'req1');
    });
  });

  group('clearSelectedItem', () {
    test('clears selected item', () {
      final gear = GearDiscoverItem(CommunityGearItem(id: '1'));

      final notifier = container.read(discoverProvider.notifier);
      notifier.selectItem(gear);
      expect(container.read(discoverProvider).selectedItem, isNotNull);

      notifier.clearSelectedItem();
      expect(container.read(discoverProvider).selectedItem, isNull);
    });

    test('does nothing if no item selected', () {
      final notifier = container.read(discoverProvider.notifier);
      notifier.clearSelectedItem();

      expect(container.read(discoverProvider).selectedItem, isNull);
    });
  });

  group('findItemById', () {
    test('returns gear item when found', () {
      final items = [
        GearDiscoverItem(CommunityGearItem(id: 'gear1', name: 'Gear 1')),
        GearDiscoverItem(CommunityGearItem(id: 'gear2', name: 'Gear 2')),
        RequestDiscoverItem(Request(id: 'req1', title: 'Request 1')),
      ];

      final notifier = container.read(discoverProvider.notifier);
      notifier.state = notifier.state.copyWith(items: items, isLoading: false);

      final foundItem = notifier.findItemById('gear2');
      expect(foundItem, isNotNull);
      expect(foundItem?.id, 'gear2');
      expect(foundItem, isA<GearDiscoverItem>());
    });

    test('returns request item when found', () {
      final items = [
        GearDiscoverItem(CommunityGearItem(id: 'gear1', name: 'Gear 1')),
        RequestDiscoverItem(Request(id: 'req1', title: 'Request 1')),
      ];

      final notifier = container.read(discoverProvider.notifier);
      notifier.state = notifier.state.copyWith(items: items, isLoading: false);

      final foundItem = notifier.findItemById('req1');
      expect(foundItem, isNotNull);
      expect(foundItem?.id, 'req1');
      expect(foundItem, isA<RequestDiscoverItem>());
    });

    test('returns null when item not found', () {
      final items = [
        GearDiscoverItem(CommunityGearItem(id: 'gear1', name: 'Gear 1')),
      ];

      final notifier = container.read(discoverProvider.notifier);
      notifier.state = notifier.state.copyWith(items: items, isLoading: false);

      final foundItem = notifier.findItemById('nonexistent');
      expect(foundItem, isNull);
    });

    test('returns null for empty items list', () {
      final notifier = container.read(discoverProvider.notifier);
      final foundItem = notifier.findItemById('1');
      expect(foundItem, isNull);
    });
  });

  group('loadOwnerName', () {
    test('successfully loads and caches owner name', () async {
      const ownerId = 'owner-123';
      final userResponse = GetUserResponse(userId: ownerId, name: 'John Doe');

      when(
        mockUserRepository.get(ownerId),
      ).thenAnswer((_) async => userResponse);

      final notifier = container.read(discoverProvider.notifier);
      await notifier.loadOwnerName(ownerId);

      final state = container.read(discoverProvider);
      expect(state.ownerNames[ownerId], 'John Doe');
      expect(state.loadingOwnerIds, isEmpty);

      verify(mockUserRepository.get(ownerId)).called(1);
    });

    test('skips loading if owner name already cached', () async {
      const ownerId = 'owner-123';

      final notifier = container.read(discoverProvider.notifier);

      // Manually set cached name
      notifier.state = notifier.state.copyWith(
        ownerNames: {'owner-123': 'Cached Name'},
      );

      await notifier.loadOwnerName(ownerId);

      // Should not call repository
      verifyNever(mockUserRepository.get(any));

      final state = container.read(discoverProvider);
      expect(state.ownerNames[ownerId], 'Cached Name');
    });

    test('skips loading if owner is currently being loaded', () async {
      const ownerId = 'owner-123';

      final notifier = container.read(discoverProvider.notifier);

      // Manually set loading state
      notifier.state = notifier.state.copyWith(loadingOwnerIds: {'owner-123'});

      await notifier.loadOwnerName(ownerId);

      // Should not call repository
      verifyNever(mockUserRepository.get(any));
    });

    test('sets loading state during fetch', () async {
      const ownerId = 'owner-123';
      final userResponse = GetUserResponse(userId: ownerId, name: 'John Doe');

      when(mockUserRepository.get(ownerId)).thenAnswer((_) async {
        await Future.delayed(const Duration(milliseconds: 10));
        return userResponse;
      });

      final notifier = container.read(discoverProvider.notifier);
      final loadFuture = notifier.loadOwnerName(ownerId);

      // Check loading state immediately
      await Future.delayed(const Duration(milliseconds: 1));
      expect(
        container.read(discoverProvider).loadingOwnerIds,
        contains(ownerId),
      );

      // Wait for completion
      await loadFuture;
      expect(container.read(discoverProvider).loadingOwnerIds, isEmpty);
    });

    test('handles error gracefully', () async {
      const ownerId = 'owner-123';

      when(
        mockUserRepository.get(ownerId),
      ).thenThrow(Exception('Network error'));

      final notifier = container.read(discoverProvider.notifier);
      await notifier.loadOwnerName(ownerId);

      final state = container.read(discoverProvider);
      expect(state.ownerNames.containsKey(ownerId), isFalse);
      expect(state.loadingOwnerIds, isEmpty);

      verify(mockUserRepository.get(ownerId)).called(1);
    });

    test('can load multiple owners in parallel', () async {
      const owner1 = 'owner-1';
      const owner2 = 'owner-2';

      when(mockUserRepository.get(owner1)).thenAnswer(
        (_) async => GetUserResponse(userId: owner1, name: 'User 1'),
      );
      when(mockUserRepository.get(owner2)).thenAnswer(
        (_) async => GetUserResponse(userId: owner2, name: 'User 2'),
      );

      final notifier = container.read(discoverProvider.notifier);

      await Future.wait([
        notifier.loadOwnerName(owner1),
        notifier.loadOwnerName(owner2),
      ]);

      final state = container.read(discoverProvider);
      expect(state.ownerNames[owner1], 'User 1');
      expect(state.ownerNames[owner2], 'User 2');
      expect(state.loadingOwnerIds, isEmpty);
    });
  });

  group('DiscoverState new fields', () {
    test('initial state has empty owner names cache', () {
      final state = container.read(discoverProvider);
      expect(state.ownerNames, isEmpty);
    });

    test('initial state has no loading owner IDs', () {
      final state = container.read(discoverProvider);
      expect(state.loadingOwnerIds, isEmpty);
    });

    test('clear resets owner names cache', () async {
      const ownerId = 'owner-123';
      when(
        mockUserRepository.get(ownerId),
      ).thenAnswer((_) async => GetUserResponse(userId: ownerId, name: 'John'));

      final notifier = container.read(discoverProvider.notifier);
      await notifier.loadOwnerName(ownerId);

      expect(container.read(discoverProvider).ownerNames, isNotEmpty);

      notifier.clear();

      expect(container.read(discoverProvider).ownerNames, isEmpty);
    });
  });
}
