import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/presentation/viewmodels/community_content_view_model.dart';
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/providers.dart';

import 'community_content_view_model_test.mocks.dart';

@GenerateMocks([CommunityRepository])
void main() {
  group('CommunityContentViewModel', () {
    late MockCommunityRepository mockRepository;
    late ProviderContainer container;

    setUp(() {
      mockRepository = MockCommunityRepository();

      // Create a ProviderContainer with overridden repositories
      container = ProviderContainer(
        overrides: [
          communityRepositoryProvider.overrideWithValue(mockRepository),
        ],
      );
    });

    tearDown(() {
      container.dispose();
    });

    group('initialize', () {
      test('loads all data sources in parallel', () async {
        const communityId = 'comm1';
        final mockMembers = [
          CommunityMember(user: User(id: 'user1', name: 'Alice')),
          CommunityMember(user: User(id: 'user2', name: 'Bob')),
        ];
        final mockEvents = [
          CommunityEventItem(
            id: 'event1',
            eventType: CommunityEventType.COMMUNITY_EVENT_TYPE_GEAR_SHARED,
          ),
        ];

        when(mockRepository.getMembers(communityId))
            .thenAnswer((_) async => mockMembers);
        when(mockRepository.getEvents(communityId))
            .thenAnswer((_) async => mockEvents);
        when(mockRepository.getGearCount(communityId))
            .thenAnswer((_) async => 5);

        final notifier = container.read(communityContentProvider.notifier);

        // Explicitly build the state before calling initialize
        container.read(communityContentProvider);

        await notifier.initialize(communityId);

        final state = container.read(communityContentProvider);

        // Verify all data was loaded
        expect(state.members.length, 2);
        expect(state.events.length, 1);
        expect(state.gearCount, 5);

        // Verify all loading flags are false
        expect(state.isLoadingMembers, false);
        expect(state.isLoadingEvents, false);
        expect(state.isLoadingGear, false);

        // Verify all methods were called once
        verify(mockRepository.getMembers(communityId)).called(1);
        verify(mockRepository.getEvents(communityId)).called(1);
        verify(mockRepository.getGearCount(communityId)).called(1);
      });

      test('handles empty community ID gracefully', () async {
        final notifier = container.read(communityContentProvider.notifier);
        await notifier.initialize('');

        // Should not call repository with empty ID
        verifyNever(mockRepository.getMembers(any));
        verifyNever(mockRepository.getEvents(any));
        verifyNever(mockRepository.getGearCount(any));
      });

      test('continues loading other sources when one fails', () async {
        const communityId = 'comm1';
        final mockEvents = [
          CommunityEventItem(
            id: 'event1',
            eventType: CommunityEventType.COMMUNITY_EVENT_TYPE_GEAR_SHARED,
          ),
        ];

        when(mockRepository.getMembers(communityId))
            .thenThrow(Exception('Network error'));
        when(mockRepository.getEvents(communityId))
            .thenAnswer((_) async => mockEvents);
        when(mockRepository.getGearCount(communityId))
            .thenAnswer((_) async => 3);

        final notifier = container.read(communityContentProvider.notifier);
        await notifier.initialize(communityId);

        final state = container.read(communityContentProvider);

        // Members should have error, but events and gear should load
        expect(state.membersError, isNotNull);
        expect(state.members.length, 0);
        expect(state.events.length, 1);
        expect(state.gearCount, 3);
        expect(state.eventsError, isNull);
        expect(state.gearError, isNull);
      });
    });

    group('loadMembers', () {
      test('sets loading state and loads members successfully', () async {
        const communityId = 'comm1';
        final mockMembers = [
          CommunityMember(user: User(id: 'user1', name: 'Alice')),
          CommunityMember(user: User(id: 'user2', name: 'Bob')),
        ];

        when(mockRepository.getMembers(communityId))
            .thenAnswer((_) async => mockMembers);

        // Read the notifier to build it first
        final notifier = container.read(communityContentProvider.notifier);

        // Explicitly build the state
        container.read(communityContentProvider);

        await notifier.loadMembers(communityId);

        final state = container.read(communityContentProvider);

        expect(state.members.length, 2);
        expect(state.members[0].user.id, 'user1');
        expect(state.members[1].user.id, 'user2');
        expect(state.isLoadingMembers, false);
        expect(state.membersError, isNull);

        verify(mockRepository.getMembers(communityId)).called(1);
      });

      test('sets error state when loading fails', () async {
        const communityId = 'comm1';
        final error = Exception('Network error');

        when(mockRepository.getMembers(communityId)).thenThrow(error);

        final notifier = container.read(communityContentProvider.notifier);
        await notifier.loadMembers(communityId);

        final state = container.read(communityContentProvider);

        expect(state.members.length, 0);
        expect(state.isLoadingMembers, false);
        expect(state.membersError, isNotNull);

        verify(mockRepository.getMembers(communityId)).called(1);
      });

      test('clears previous error on successful reload', () async {
        const communityId = 'comm1';
        final mockMembers = [CommunityMember(user: User(id: 'user1', name: 'Alice'))];

        // First call fails
        when(mockRepository.getMembers(communityId))
            .thenThrow(Exception('Error'));

        final notifier = container.read(communityContentProvider.notifier);
        await notifier.loadMembers(communityId);

        var state = container.read(communityContentProvider);
        expect(state.membersError, isNotNull);

        // Second call succeeds
        when(mockRepository.getMembers(communityId))
            .thenAnswer((_) async => mockMembers);

        await notifier.loadMembers(communityId);

        state = container.read(communityContentProvider);
        expect(state.membersError, isNull);
        expect(state.members.length, 1);
      });
    });

    group('loadEvents', () {
      test('sets loading state and loads events successfully', () async {
        const communityId = 'comm1';
        final mockEvents = [
          CommunityEventItem(
            id: 'event1',
            eventType: CommunityEventType.COMMUNITY_EVENT_TYPE_GEAR_SHARED,
          ),
          CommunityEventItem(
            id: 'event2',
            eventType:
                CommunityEventType.COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED,
          ),
        ];

        when(mockRepository.getEvents(communityId))
            .thenAnswer((_) async => mockEvents);

        final notifier = container.read(communityContentProvider.notifier);
        await notifier.loadEvents(communityId);

        final state = container.read(communityContentProvider);

        expect(state.events.length, 2);
        expect(state.events[0].id, 'event1');
        expect(state.events[1].id, 'event2');
        expect(state.isLoadingEvents, false);
        expect(state.eventsError, isNull);

        verify(mockRepository.getEvents(communityId)).called(1);
      });

      test('sets error state when loading fails', () async {
        const communityId = 'comm1';
        final error = Exception('Network error');

        when(mockRepository.getEvents(communityId)).thenThrow(error);

        final notifier = container.read(communityContentProvider.notifier);
        await notifier.loadEvents(communityId);

        final state = container.read(communityContentProvider);

        expect(state.events.length, 0);
        expect(state.isLoadingEvents, false);
        expect(state.eventsError, isNotNull);

        verify(mockRepository.getEvents(communityId)).called(1);
      });
    });

    group('loadGearCount', () {
      test('sets loading state and loads gear count successfully', () async {
        const communityId = 'comm1';

        when(mockRepository.getGearCount(communityId))
            .thenAnswer((_) async => 7);

        final notifier = container.read(communityContentProvider.notifier);
        await notifier.loadGearCount(communityId);

        final state = container.read(communityContentProvider);

        expect(state.gearCount, 7);
        expect(state.isLoadingGear, false);
        expect(state.gearError, isNull);

        verify(mockRepository.getGearCount(communityId)).called(1);
      });

      test('handles zero gear count', () async {
        const communityId = 'comm1';

        when(mockRepository.getGearCount(communityId))
            .thenAnswer((_) async => 0);

        final notifier = container.read(communityContentProvider.notifier);
        await notifier.loadGearCount(communityId);

        final state = container.read(communityContentProvider);

        expect(state.gearCount, 0);
        expect(state.isLoadingGear, false);
        expect(state.gearError, isNull);
      });

      test('sets error state when loading fails', () async {
        const communityId = 'comm1';
        final error = Exception('Network error');

        when(mockRepository.getGearCount(communityId)).thenThrow(error);

        final notifier = container.read(communityContentProvider.notifier);
        await notifier.loadGearCount(communityId);

        final state = container.read(communityContentProvider);

        expect(state.gearCount, 0);
        expect(state.isLoadingGear, false);
        expect(state.gearError, isNotNull);

        verify(mockRepository.getGearCount(communityId)).called(1);
      });
    });

    group('refresh', () {
      test('invalidates caches and reloads all data', () async {
        const communityId = 'comm1';
        final mockMembers = [CommunityMember(user: User(id: 'user1', name: 'Alice'))];
        final mockEvents = [
          CommunityEventItem(
            id: 'event1',
            eventType: CommunityEventType.COMMUNITY_EVENT_TYPE_GEAR_SHARED,
          ),
        ];

        when(mockRepository.refreshAll(communityId))
            .thenAnswer((_) async => {});
        when(mockRepository.getMembers(communityId))
            .thenAnswer((_) async => mockMembers);
        when(mockRepository.getEvents(communityId))
            .thenAnswer((_) async => mockEvents);
        when(mockRepository.getGearCount(communityId))
            .thenAnswer((_) async => 3);

        final notifier = container.read(communityContentProvider.notifier);

        // Explicitly build the state before calling refresh
        container.read(communityContentProvider);

        await notifier.refresh(communityId);

        final state = container.read(communityContentProvider);

        // Verify refreshAll was called
        verify(mockRepository.refreshAll(communityId)).called(1);

        // Verify all data was reloaded
        verify(mockRepository.getMembers(communityId)).called(1);
        verify(mockRepository.getEvents(communityId)).called(1);
        verify(mockRepository.getGearCount(communityId)).called(1);

        // Verify state is updated
        expect(state.members.length, 1);
        expect(state.events.length, 1);
        expect(state.gearCount, 3);
      });
    });

    group('refreshMembers', () {
      test('invalidates members cache and reloads', () async {
        const communityId = 'comm1';
        final mockMembers = [CommunityMember(user: User(id: 'user1', name: 'Alice'))];

        when(mockRepository.refreshMembers(communityId))
            .thenAnswer((_) async => {});
        when(mockRepository.getMembers(communityId))
            .thenAnswer((_) async => mockMembers);

        final notifier = container.read(communityContentProvider.notifier);
        await notifier.refreshMembers(communityId);

        verify(mockRepository.refreshMembers(communityId)).called(1);
        verify(mockRepository.getMembers(communityId)).called(1);

        final state = container.read(communityContentProvider);
        expect(state.members.length, 1);
      });
    });

    group('refreshEvents', () {
      test('invalidates events cache and reloads', () async {
        const communityId = 'comm1';
        final mockEvents = [
          CommunityEventItem(
            id: 'event1',
            eventType: CommunityEventType.COMMUNITY_EVENT_TYPE_GEAR_SHARED,
          ),
        ];

        when(mockRepository.refreshEvents(communityId))
            .thenAnswer((_) async => {});
        when(mockRepository.getEvents(communityId))
            .thenAnswer((_) async => mockEvents);

        final notifier = container.read(communityContentProvider.notifier);
        await notifier.refreshEvents(communityId);

        verify(mockRepository.refreshEvents(communityId)).called(1);
        verify(mockRepository.getEvents(communityId)).called(1);

        final state = container.read(communityContentProvider);
        expect(state.events.length, 1);
      });
    });

    group('refreshGearCount', () {
      test('invalidates gear cache and reloads', () async {
        const communityId = 'comm1';

        when(mockRepository.refreshGearCount(communityId))
            .thenAnswer((_) async => {});
        when(mockRepository.getGearCount(communityId))
            .thenAnswer((_) async => 5);

        final notifier = container.read(communityContentProvider.notifier);
        await notifier.refreshGearCount(communityId);

        verify(mockRepository.refreshGearCount(communityId)).called(1);
        verify(mockRepository.getGearCount(communityId)).called(1);

        final state = container.read(communityContentProvider);
        expect(state.gearCount, 5);
      });
    });

    group('state computed properties', () {
      test('isFullyLoaded returns true when all loading is complete', () async {
        const communityId = 'comm1';

        when(mockRepository.getMembers(communityId))
            .thenAnswer((_) async => []);
        when(mockRepository.getEvents(communityId))
            .thenAnswer((_) async => []);
        when(mockRepository.getGearCount(communityId))
            .thenAnswer((_) async => 0);

        final notifier = container.read(communityContentProvider.notifier);
        await notifier.initialize(communityId);

        final state = container.read(communityContentProvider);
        expect(state.isFullyLoaded, true);
        expect(state.isLoading, false);
      });

      test('hasError returns true when any source has an error', () async {
        const communityId = 'comm1';

        when(mockRepository.getMembers(communityId))
            .thenThrow(Exception('Error'));
        when(mockRepository.getEvents(communityId))
            .thenAnswer((_) async => []);
        when(mockRepository.getGearCount(communityId))
            .thenAnswer((_) async => 0);

        final notifier = container.read(communityContentProvider.notifier);
        await notifier.initialize(communityId);

        final state = container.read(communityContentProvider);
        expect(state.hasError, true);
      });

      test('hasMembers returns true when members list is not empty', () async {
        const communityId = 'comm1';
        final mockMembers = [CommunityMember(user: User(id: 'user1', name: 'Alice'))];

        when(mockRepository.getMembers(communityId))
            .thenAnswer((_) async => mockMembers);

        final notifier = container.read(communityContentProvider.notifier);
        await notifier.loadMembers(communityId);

        final state = container.read(communityContentProvider);
        expect(state.hasMembers, true);
      });

      test('hasEvents returns true when events list is not empty', () async {
        const communityId = 'comm1';
        final mockEvents = [
          CommunityEventItem(
            id: 'event1',
            eventType: CommunityEventType.COMMUNITY_EVENT_TYPE_GEAR_SHARED,
          ),
        ];

        when(mockRepository.getEvents(communityId))
            .thenAnswer((_) async => mockEvents);

        final notifier = container.read(communityContentProvider.notifier);
        await notifier.loadEvents(communityId);

        final state = container.read(communityContentProvider);
        expect(state.hasEvents, true);
      });

      test('hasGear returns true when gear count is greater than zero',
          () async {
        const communityId = 'comm1';

        when(mockRepository.getGearCount(communityId))
            .thenAnswer((_) async => 5);

        final notifier = container.read(communityContentProvider.notifier);
        await notifier.loadGearCount(communityId);

        final state = container.read(communityContentProvider);
        expect(state.hasGear, true);
      });
    });
  });
}
