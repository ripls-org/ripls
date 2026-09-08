// Disposal Safety Tests
//
// These tests verify that Notifiers using SafeNotifierMixin handle disposal
// correctly during async operations. When a provider is disposed while an
// async operation is in flight, the operation should complete without throwing.
//
// This is critical for autoDispose providers where users can navigate away
// before async operations complete (e.g., API calls, data loading).
//
// Related crashes fixed:
// - GearNotifier.loadGearDetails disposal crash
// - ExperienceContentView disposal crash
// See: docs/crashlytics/01292026/mitigation.md

import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/gear_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/user_repository.dart';
import 'package:ripls/presentation/viewmodels/community_content_view_model.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/viewmodels/governance_view_model.dart';
import 'package:ripls/services/gear_service.dart';
import 'package:ripls/services/location_service.dart';
import 'package:ripls/services/media_service.dart';
import 'package:ripls/services/providers.dart';

import '../../core/observability/analytics_test_helper.dart';
import 'disposal_safety_test.mocks.dart';

@GenerateMocks([
  GearRepository,
  UserRepository,
  MediaRepository,
  CommunityRepository,
  GearService,
  MediaService,
  LocationService,
])
void main() {
  group('GearNotifier disposal safety', () {
    late MockGearRepository mockGearRepository;
    late MockUserRepository mockUserRepository;
    late MockMediaRepository mockMediaRepository;
    late MockGearService mockGearService;
    late MockMediaService mockMediaService;
    late MockLocationService mockLocationService;
    late MockObservabilityService mockObservability;

    setUp(() {
      mockGearRepository = MockGearRepository();
      mockUserRepository = MockUserRepository();
      mockMediaRepository = MockMediaRepository();
      mockGearService = MockGearService();
      mockMediaService = MockMediaService();
      mockLocationService = MockLocationService();
      mockObservability = MockObservabilityService();
    });

    tearDown(() {
      reset(mockGearRepository);
      reset(mockUserRepository);
      reset(mockMediaRepository);
      reset(mockGearService);
      reset(mockMediaService);
      reset(mockLocationService);
    });

    test('loadGearDetails completes without throwing when disposed mid-await',
        () async {
      final gearCompleter = Completer<GetGearResponse>();

      when(mockGearRepository.getGearDetails(any))
          .thenAnswer((_) => gearCompleter.future);

      final container = ProviderContainer(
        overrides: [
          gearRepositoryProvider.overrideWithValue(mockGearRepository),
          userRepositoryProvider.overrideWithValue(mockUserRepository),
          mediaRepositoryProvider.overrideWithValue(mockMediaRepository),
          gearServiceProvider.overrideWithValue(mockGearService),
          mediaServiceProvider.overrideWithValue(mockMediaService),
          locationServiceProvider.overrideWithValue(mockLocationService),
          observabilityServiceProvider.overrideWithValue(mockObservability),
        ],
      );

      final notifier = container.read(gearProvider('test-gear-123').notifier);

      // Start async operation
      final future = notifier.loadGearDetails();

      // Dispose container while async operation is pending
      container.dispose();

      // Complete the data (simulating API response arriving after disposal)
      gearCompleter.complete(GetGearResponse(
        id: 'test-gear-123',
        name: 'Test Gear',
        description: 'Description',
        owner: User(id: 'owner-123'),
        mediaIds: [],
      ));

      // The future should complete without throwing
      await expectLater(future, completes);
    });

    test('saveChanges completes without throwing when disposed mid-await',
        () async {
      final saveCompleter = Completer<void>();

      when(mockGearRepository.saveGear(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      )).thenAnswer((_) => saveCompleter.future);

      final container = ProviderContainer(
        overrides: [
          gearRepositoryProvider.overrideWithValue(mockGearRepository),
          userRepositoryProvider.overrideWithValue(mockUserRepository),
          mediaRepositoryProvider.overrideWithValue(mockMediaRepository),
          gearServiceProvider.overrideWithValue(mockGearService),
          mediaServiceProvider.overrideWithValue(mockMediaService),
          locationServiceProvider.overrideWithValue(mockLocationService),
          observabilityServiceProvider.overrideWithValue(mockObservability),
        ],
      );

      final notifier = container.read(gearProvider('test-gear-123').notifier);

      // Set up state with gear details so saveChanges can proceed
      notifier.state = notifier.state.copyWith(
        gearDetails: GetGearResponse(
          id: 'test-gear-123',
          name: 'Old Name',
          description: 'Old Description',
          owner: User(id: 'owner-123'),
          mediaIds: [],
        ),
      );

      // Start async operation
      final future = notifier.saveChanges(
        name: 'New Name',
        description: 'New Description',
      );

      // Dispose container while async operation is pending
      container.dispose();

      // Complete the save
      saveCompleter.complete();

      // The future should complete without throwing
      await expectLater(future, completes);
    });
  });

  group('CommunityContentViewModel disposal safety', () {
    late MockCommunityRepository mockCommunityRepository;

    setUp(() {
      mockCommunityRepository = MockCommunityRepository();
    });

    tearDown(() {
      reset(mockCommunityRepository);
    });

    test('loadMembers completes without throwing when disposed mid-await',
        () async {
      final membersCompleter = Completer<List<CommunityMember>>();

      when(mockCommunityRepository.getMembers(any))
          .thenAnswer((_) => membersCompleter.future);

      final container = ProviderContainer(
        overrides: [
          communityRepositoryProvider.overrideWithValue(mockCommunityRepository),
        ],
      );

      final notifier = container.read(communityContentProvider.notifier);

      // Start async operation
      final future = notifier.loadMembers('test-community-123');

      // Dispose container while async operation is pending
      container.dispose();

      // Complete the data
      membersCompleter.complete([]);

      // The future should complete without throwing
      await expectLater(future, completes);
    });

    test('loadEvents completes without throwing when disposed mid-await',
        () async {
      final eventsCompleter = Completer<List<CommunityEventItem>>();

      when(mockCommunityRepository.getEvents(any))
          .thenAnswer((_) => eventsCompleter.future);

      final container = ProviderContainer(
        overrides: [
          communityRepositoryProvider.overrideWithValue(mockCommunityRepository),
        ],
      );

      final notifier = container.read(communityContentProvider.notifier);

      // Start async operation
      final future = notifier.loadEvents('test-community-123');

      // Dispose container while async operation is pending
      container.dispose();

      // Complete the data
      eventsCompleter.complete([]);

      // The future should complete without throwing
      await expectLater(future, completes);
    });

    test('loadGearCount completes without throwing when disposed mid-await',
        () async {
      final countCompleter = Completer<int>();

      when(mockCommunityRepository.getGearCount(any))
          .thenAnswer((_) => countCompleter.future);

      final container = ProviderContainer(
        overrides: [
          communityRepositoryProvider.overrideWithValue(mockCommunityRepository),
        ],
      );

      final notifier = container.read(communityContentProvider.notifier);

      // Start async operation
      final future = notifier.loadGearCount('test-community-123');

      // Dispose container while async operation is pending
      container.dispose();

      // Complete the data
      countCompleter.complete(42);

      // The future should complete without throwing
      await expectLater(future, completes);
    });
  });

  group('GovernanceNotifier disposal safety', () {
    late MockCommunityRepository mockCommunityRepository;

    setUp(() {
      mockCommunityRepository = MockCommunityRepository();
    });

    tearDown(() {
      reset(mockCommunityRepository);
    });

    test('initialize completes without throwing when disposed mid-await',
        () async {
      final regionsCompleter = Completer<List<CommunityRegionItem>>();

      when(mockCommunityRepository.getCommunityRegions(any))
          .thenAnswer((_) => regionsCompleter.future);

      final container = ProviderContainer(
        overrides: [
          communityRepositoryProvider.overrideWithValue(mockCommunityRepository),
        ],
      );

      final notifier = container.read(governanceProvider.notifier);

      // Start async operation
      final future = notifier.initialize('test-community-123');

      // Dispose container while async operation is pending
      container.dispose();

      // Complete the data
      regionsCompleter.complete([]);

      // The future should complete without throwing
      await expectLater(future, completes);
    });

    test('refresh completes without throwing when disposed mid-await',
        () async {
      final refreshCompleter = Completer<List<CommunityRegionItem>>();
      final regionsCompleter = Completer<List<CommunityRegionItem>>();

      when(mockCommunityRepository.refreshCommunityRegions(any))
          .thenAnswer((_) => refreshCompleter.future);
      when(mockCommunityRepository.getCommunityRegions(any))
          .thenAnswer((_) => regionsCompleter.future);

      final container = ProviderContainer(
        overrides: [
          communityRepositoryProvider.overrideWithValue(mockCommunityRepository),
        ],
      );

      final notifier = container.read(governanceProvider.notifier);

      // Set communityId first
      notifier.state = notifier.state.copyWith(communityId: 'test-community-123');

      // Start async operation
      final future = notifier.refresh();

      // Dispose container while async operation is pending
      container.dispose();

      // Complete the operations
      refreshCompleter.complete([]);
      regionsCompleter.complete([]);

      // The future should complete without throwing
      await expectLater(future, completes);
    });
  });

  group('Concurrent disposal safety', () {
    late MockCommunityRepository mockCommunityRepository;

    setUp(() {
      mockCommunityRepository = MockCommunityRepository();
    });

    tearDown(() {
      reset(mockCommunityRepository);
    });

    test('multiple concurrent operations complete safely when disposed',
        () async {
      final membersCompleter = Completer<List<CommunityMember>>();
      final eventsCompleter = Completer<List<CommunityEventItem>>();
      final gearCompleter = Completer<int>();

      when(mockCommunityRepository.getMembers(any))
          .thenAnswer((_) => membersCompleter.future);
      when(mockCommunityRepository.getEvents(any))
          .thenAnswer((_) => eventsCompleter.future);
      when(mockCommunityRepository.getGearCount(any))
          .thenAnswer((_) => gearCompleter.future);

      final container = ProviderContainer(
        overrides: [
          communityRepositoryProvider.overrideWithValue(mockCommunityRepository),
        ],
      );

      final notifier = container.read(communityContentProvider.notifier);

      // Start multiple async operations concurrently
      final futures = Future.wait([
        notifier.loadMembers('test-community-123'),
        notifier.loadEvents('test-community-123'),
        notifier.loadGearCount('test-community-123'),
      ]);

      // Dispose container while all operations are pending
      container.dispose();

      // Complete all operations
      membersCompleter.complete([]);
      eventsCompleter.complete([]);
      gearCompleter.complete(10);

      // All futures should complete without throwing
      await expectLater(futures, completes);
    });
  });
}
