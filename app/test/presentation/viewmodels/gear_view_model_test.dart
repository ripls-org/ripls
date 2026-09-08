import 'package:cross_file/cross_file.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/models/media_item_data.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show SharedCommunity, TrackedString;
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart' show CommunityItem;
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show ActiveLoan;
import 'package:ripls/data/gen/ripls/api/gear.pbenum.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show GearTransferContext, Transfer, TransferRequest;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/gen/ripls/api/user_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/value.pb.dart' show ValueEstimate;
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/gear_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/data/repositories/user_repository.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/gear/gear_metadata_sheet.dart'
    show GearMetadataEditResult;
import 'package:ripls/services/gear_service.dart';
import 'package:ripls/services/location_service.dart';
import 'package:ripls/services/media_service.dart';
import 'package:ripls/services/providers.dart';

import '../../core/observability/analytics_test_helper.dart';
import 'gear_view_model_test.mocks.dart';

@GenerateMocks([
  ChatRepository,
  GearRepository,
  TransferRepository,
  UserRepository,
  MediaRepository,
  CommunityRepository,
  GearService,
  MediaService,
  LocationService,
])
void main() {
  late ProviderContainer container;
  late MockChatRepository mockChatRepository;
  late MockGearRepository mockGearRepository;
  late MockTransferRepository mockTransferRepository;
  late MockUserRepository mockUserRepository;
  late MockMediaRepository mockMediaRepository;
  late MockCommunityRepository mockCommunityRepository;
  late MockGearService mockGearService;
  late MockMediaService mockMediaService;
  late MockLocationService mockLocationService;
  late MockObservabilityService mockObservability;

  // Test gear ID used across all tests
  const testGearId = 'test-gear-123';

  setUp(() {
    mockChatRepository = MockChatRepository();
    mockGearRepository = MockGearRepository();
    mockTransferRepository = MockTransferRepository();
    mockUserRepository = MockUserRepository();
    mockMediaRepository = MockMediaRepository();
    mockCommunityRepository = MockCommunityRepository();
    when(mockCommunityRepository.listUserCommunities()).thenAnswer(
      (_) async => const <CommunityItem>[],
    );
    mockGearService = MockGearService();
    mockMediaService = MockMediaService();
    mockLocationService = MockLocationService();
    mockObservability = MockObservabilityService();

    container = ProviderContainer(
      overrides: [
        chatRepositoryProvider.overrideWithValue(mockChatRepository),
        gearRepositoryProvider.overrideWithValue(mockGearRepository),
        transferRepositoryProvider.overrideWithValue(mockTransferRepository),
        userRepositoryProvider.overrideWithValue(mockUserRepository),
        mediaRepositoryProvider.overrideWithValue(mockMediaRepository),
        communityRepositoryProvider.overrideWithValue(mockCommunityRepository),
        gearServiceProvider.overrideWithValue(mockGearService),
        mediaServiceProvider.overrideWithValue(mockMediaService),
        locationServiceProvider.overrideWithValue(mockLocationService),
        observabilityServiceProvider.overrideWithValue(mockObservability),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockChatRepository);
    reset(mockGearRepository);
    reset(mockTransferRepository);
    reset(mockUserRepository);
    reset(mockMediaRepository);
    reset(mockCommunityRepository);
    reset(mockGearService);
    reset(mockMediaService);
    reset(mockLocationService);
  });

  group('GearState', () {
    test('initial state is correct', () {
      final state = container.read(gearProvider(testGearId));

      expect(state.gearId, testGearId);
      expect(state.currentUserId, isNull);
      expect(state.gearDetails, isNull);
      expect(state.ownerName, isNull);
      expect(state.locationName, isNull);
      expect(state.isLoading, isTrue);
      expect(state.isEditing, isFalse);
      expect(state.isSaving, isFalse);
      expect(state.isUploadingMedia, isFalse);
      expect(state.error, isNull);
      expect(state.mediaPath, isNull);
      expect(state.mediaId, isNull);
      expect(state.isVideo, isFalse);
      expect(state.allMediaItems, isEmpty);
    });

    test('isOwner returns true when current user is owner', () {
      final notifier = container.read(gearProvider(testGearId).notifier);

      notifier.state = notifier.state.copyWith(
        currentUserId: 'user-123',
        gearDetails: GetGearResponse(
          id: 'gear-123',
          name: 'Test Gear',
          description: 'Test Description',
          owner: User(id: 'user-123', name: 'Test User'),
          mediaIds: [],
        ),
      );

      expect(container.read(gearProvider(testGearId)).isOwner, isTrue);
    });

    test('isOwner returns false when current user is not owner', () {
      final notifier = container.read(gearProvider(testGearId).notifier);

      notifier.state = notifier.state.copyWith(
        currentUserId: 'user-123',
        gearDetails: GetGearResponse(
          id: 'gear-123',
          name: 'Test Gear',
          description: 'Test Description',
          owner: User(id: 'user-456', name: 'Other User'),
          mediaIds: [],
        ),
      );

      expect(container.read(gearProvider(testGearId)).isOwner, isFalse);
    });

    test('hasError returns correct value', () {
      final notifier = container.read(gearProvider(testGearId).notifier);

      // No error initially
      expect(container.read(gearProvider(testGearId)).hasError, isFalse);

      // Set error
      notifier.state = notifier.state.copyWith(
          error: const UserError.generic(fallback: 'Test error'));
      expect(container.read(gearProvider(testGearId)).hasError, isTrue);
    });

    test('canEditCoverPhoto false when not editing', () {
      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        currentUserId: 'user-123',
        gearDetails: GetGearResponse(
          id: 'gear-123',
          owner: User(id: 'user-123', name: 'Owner'),
        ),
        isEditing: false,
      );

      expect(
        container.read(gearProvider(testGearId)).canEditCoverPhoto,
        isFalse,
      );
    });

    test('canEditCoverPhoto true when owner is editing', () {
      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        currentUserId: 'user-123',
        gearDetails: GetGearResponse(
          id: 'gear-123',
          owner: User(id: 'user-123', name: 'Owner'),
        ),
        isEditing: true,
      );

      expect(
        container.read(gearProvider(testGearId)).canEditCoverPhoto,
        isTrue,
      );
    });

    test('canEditCoverPhoto true even when a cover photo already exists',
        () {
      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        currentUserId: 'user-123',
        gearDetails: GetGearResponse(
          id: 'gear-123',
          owner: User(id: 'user-123', name: 'Owner'),
        ),
        isEditing: true,
        mediaPath: 'https://example.com/photo.jpg',
        mediaId: 'media-1',
      );

      expect(
        container.read(gearProvider(testGearId)).canEditCoverPhoto,
        isTrue,
      );
    });

    test('canEditCoverPhoto false when editing as non-owner', () {
      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        currentUserId: 'user-123',
        gearDetails: GetGearResponse(
          id: 'gear-123',
          owner: User(id: 'user-456', name: 'Other Owner'),
        ),
        isEditing: true,
      );

      expect(
        container.read(gearProvider(testGearId)).canEditCoverPhoto,
        isFalse,
      );
    });
  });

  group('initialize', () {
    test('successfully loads gear details without media', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
        locationId: 'location-123',
      );

      when(mockGearRepository.getGearDetails('gear-123'))
          .thenAnswer((_) async => gear);

      when(mockUserRepository.getUserProfile('owner-123')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(
            userId: 'owner-123',
            name: 'Test Owner',
          ),
        ),
      );

      when(mockLocationService.getLocation('location-123')).thenAnswer(
        (_) async => Location(
          locality: 'San Francisco',
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        ),
      );

      final notifier = container.read(gearProvider(testGearId).notifier);
      await notifier.initialize(
        gearId: 'gear-123',
        currentUserId: 'user-123',
      );

      final state = container.read(gearProvider(testGearId));
      expect(state.gearId, 'gear-123');
      expect(state.currentUserId, 'user-123');
      expect(state.gearDetails, gear);
      expect(state.ownerName, 'Test Owner');
      expect(state.locationName, 'San Francisco');
      expect(state.locationLatitude, 37.7749);
      expect(state.locationLongitude, -122.4194);
      expect(state.isLoading, isFalse);
      expect(state.error, isNull);
    });

    test('successfully loads gear details with media', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: ['media-1'],
      );

      when(mockGearRepository.getGearDetails('gear-123'))
          .thenAnswer((_) async => gear);

      when(mockUserRepository.getUserProfile('owner-123')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(
            userId: 'owner-123',
            name: 'Test Owner',
          ),
        ),
      );

      when(mockMediaRepository.get('media-1')).thenAnswer(
        (_) async => GetMediaResponse(
          id: 'media-1',
          url: 'https://example.com/image.jpg',
          contentType: 'image/jpeg',
        ),
      );

      when(mockMediaRepository.getFullMediaUrl('media-1')).thenAnswer(
        (_) async => MediaUrl(
          url: 'https://example.com/image.jpg',
          mediaId: 'media-1',
          isThumbnail: false,
        ),
      );

      // Keep the autoDispose provider alive while fire-and-forget tasks run.
      final sub = container.listen(gearProvider(testGearId), (_, _) {});
      final notifier = container.read(gearProvider(testGearId).notifier);
      await notifier.initialize(
        gearId: 'gear-123',
        currentUserId: 'user-123',
      );
      // _loadMediaFromServer is now fire-and-forget; allow microtasks to flush.
      await Future<void>.delayed(Duration.zero);
      sub.close();

      final state = container.read(gearProvider(testGearId));
      expect(state.gearDetails, gear);
      expect(state.allMediaItems.length, 1);
      expect(state.allMediaItems.first.id, 'media-1');
      expect(state.mediaPath, 'https://example.com/image.jpg');
      expect(state.mediaId, 'media-1');
      expect(state.isVideo, isFalse);
      expect(state.isLoading, isFalse);
    });

    test('handles error loading gear details', () async {
      when(mockGearRepository.getGearDetails('gear-123'))
          .thenThrow(Exception('Failed to load'));

      final notifier = container.read(gearProvider(testGearId).notifier);
      await notifier.initialize(
        gearId: 'gear-123',
        currentUserId: 'user-123',
      );

      final state = container.read(gearProvider(testGearId));
      expect(state.isLoading, isFalse);
      expect(state.error, isNotNull);
    });
  });

  group('toggleEditMode', () {
    test('toggles edit mode', () {
      final notifier = container.read(gearProvider(testGearId).notifier);

      expect(container.read(gearProvider(testGearId)).isEditing, isFalse);

      notifier.toggleEditMode();
      expect(container.read(gearProvider(testGearId)).isEditing, isTrue);

      notifier.toggleEditMode();
      expect(container.read(gearProvider(testGearId)).isEditing, isFalse);
    });
  });

  group('saveChanges', () {
    test('successfully saves gear changes', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Old Name',
        description: 'Old Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockGearRepository.saveGear(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      )).thenAnswer((_) async => {});

      final updatedGear = GetGearResponse(
        id: 'gear-123',
        name: 'New Name',
        description: 'New Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );
      when(mockGearRepository.getGearDetails('gear-123'))
          .thenAnswer((_) async => updatedGear);
      when(mockUserRepository.getUserProfile('owner-123')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(userId: 'owner-123', name: 'Test Owner'),
        ),
      );

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: 'gear-123',
        gearDetails: gear,
        isEditing: true,
      );

      await notifier.saveChanges(
        name: 'New Name',
        description: 'New Description',
      );

      final state = container.read(gearProvider(testGearId));
      expect(state.gearDetails!.name, 'New Name');
      expect(state.gearDetails!.description, 'New Description');
      expect(state.isEditing, isFalse);
      expect(state.isSaving, isFalse);

      verify(mockGearRepository.saveGear(
        id: 'gear-123',
        name: 'New Name',
        description: 'New Description',
        mediaIds: [],
      )).called(1);

      // Verify analytics event was logged
      expect(mockObservability.hasEventOfType<GearEditedEvent>(), isTrue);
      final editEvent = mockObservability.lastEventOfType<GearEditedEvent>();
      expect(editEvent?.parameters['gear_id'], 'gear-123');
    });

    test('handles error saving gear changes', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockGearRepository.saveGear(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      )).thenThrow(Exception('Save failed'));

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(gearDetails: gear);

      expect(
        () => notifier.saveChanges(
          name: 'New Name',
          description: 'New Description',
        ),
        throwsException,
      );
    });

    test('does nothing if already saving', () async {
      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(isSaving: true);

      await notifier.saveChanges(
        name: 'New Name',
        description: 'New Description',
      );

      verifyNever(mockGearRepository.saveGear(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      ));
    });
  });

  group('cancelEdit', () {
    test('exits edit mode', () {
      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(isEditing: true);

      notifier.cancelEdit();

      expect(container.read(gearProvider(testGearId)).isEditing, isFalse);
    });
  });

  group('availability getters', () {
    test('isGiveaway reflects the loaded mode', () {
      final loan = GearState(
        gearDetails: GetGearResponse(
          availability: Availability.AVAILABILITY_FOR_LOAN,
        ),
      );
      final giveaway = GearState(
        gearDetails: GetGearResponse(
          availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
        ),
      );
      expect(loan.isGiveaway, isFalse);
      expect(giveaway.isGiveaway, isTrue);
    });

    test('hasInFlightTransfer is true with an active loan or pending interest',
        () {
      final withActiveLoan = GearState(
        gearDetails: GetGearResponse(activeLoan: ActiveLoan(status: 'On loan')),
      );
      final withInterest = GearState(
        transferContext: GearTransferContext(
          pendingRequests: [TransferRequest(transferId: 't1')],
        ),
      );
      final withSelectedRecipient = GearState(
        transferContext: GearTransferContext(
          selectedRecipient: TransferRequest(transferId: 't2'),
        ),
      );
      final idle = GearState(
        gearDetails: GetGearResponse(
          availability: Availability.AVAILABILITY_FOR_LOAN,
        ),
        transferContext: GearTransferContext(),
      );
      expect(withActiveLoan.hasInFlightTransfer, isTrue);
      expect(withInterest.hasInFlightTransfer, isTrue);
      expect(withSelectedRecipient.hasInFlightTransfer, isTrue);
      expect(idle.hasInFlightTransfer, isFalse);
    });
  });

  group('setAvailability', () {
    GetGearResponse gearWith(Availability availability) => GetGearResponse(
          id: 'gear-123',
          name: 'Puzzle',
          description: 'A 1000-piece puzzle',
          owner: User(id: 'owner-123', name: 'Test Owner'),
          mediaIds: [],
          availability: availability,
        );

    test('flips the mode via the repository and reloads', () async {
      when(mockGearRepository.setGearAvailability(any, any))
          .thenAnswer((_) async => {});
      when(mockGearRepository.getGearDetails(testGearId))
          .thenAnswer((_) async =>
              gearWith(Availability.AVAILABILITY_FOR_GIVEAWAY));
      when(mockUserRepository.getUserProfile('owner-123')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(userId: 'owner-123', name: 'Test Owner'),
        ),
      );

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        currentUserId: 'owner-123',
        gearDetails: gearWith(Availability.AVAILABILITY_FOR_LOAN),
      );

      await notifier.setAvailability(Availability.AVAILABILITY_FOR_GIVEAWAY);

      verify(mockGearRepository.setGearAvailability(
        testGearId,
        Availability.AVAILABILITY_FOR_GIVEAWAY,
      )).called(1);
      final state = container.read(gearProvider(testGearId));
      expect(state.isGiveaway, isTrue);
      expect(state.isSaving, isFalse);
    });

    test('is a no-op when the mode is unchanged', () async {
      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: 'gear-123',
        currentUserId: 'owner-123',
        gearDetails: gearWith(Availability.AVAILABILITY_FOR_LOAN),
      );

      await notifier.setAvailability(Availability.AVAILABILITY_FOR_LOAN);

      verifyNever(mockGearRepository.setGearAvailability(any, any));
    });

    test('rethrows without tripping the full-screen error when rejected',
        () async {
      when(mockGearRepository.setGearAvailability(any, any))
          .thenThrow(Exception('in-progress transfer'));

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: 'gear-123',
        currentUserId: 'owner-123',
        gearDetails: gearWith(Availability.AVAILABILITY_FOR_LOAN),
      );

      await expectLater(
        notifier.setAvailability(Availability.AVAILABILITY_FOR_GIVEAWAY),
        throwsException,
      );

      final state = container.read(gearProvider(testGearId));
      // No state.error: the content view renders a fatal screen on state.error,
      // so a transient guard rejection is surfaced by the caller as a toast.
      expect(state.hasError, isFalse);
      expect(state.isSaving, isFalse);
      // The rejected flip must not move the local mode.
      expect(state.isGiveaway, isFalse);
    });
  });

  group('deleteGear', () {
    test('successfully deletes gear', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockGearRepository.deleteGear('gear-123'))
          .thenAnswer((_) async => {});

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(gearDetails: gear);

      await notifier.deleteGear();

      verify(mockGearRepository.deleteGear('gear-123')).called(1);

      // Verify analytics event was logged
      expect(mockObservability.hasEventOfType<GearDeletedEvent>(), isTrue);
      final deleteEvent = mockObservability.lastEventOfType<GearDeletedEvent>();
      expect(deleteEvent?.parameters['gear_id'], 'gear-123');
    });

    test('handles error deleting gear', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockGearRepository.deleteGear('gear-123'))
          .thenThrow(Exception('Delete failed'));

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(gearDetails: gear);

      expect(() => notifier.deleteGear(), throwsException);
    });
  });

  group('loadGearDetails', () {
    test('loads gear details with the provided gearId', () async {
      final gear = GetGearResponse(
        id: testGearId,
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockGearRepository.getGearDetails(testGearId))
          .thenAnswer((_) async => gear);
      when(mockUserRepository.getUserProfile('owner-123')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(
            userId: 'owner-123',
            name: 'Test Owner',
          ),
        ),
      );

      final notifier = container.read(gearProvider(testGearId).notifier);
      await notifier.loadGearDetails();

      verify(mockGearRepository.getGearDetails(testGearId)).called(1);
      final state = container.read(gearProvider(testGearId));
      expect(state.gearDetails, gear);
    });

    test('handles error fetching owner name gracefully', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockGearRepository.getGearDetails('gear-123'))
          .thenAnswer((_) async => gear);
      when(mockUserRepository.getUserProfile('owner-123'))
          .thenThrow(Exception('User not found'));

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(gearId: 'gear-123');

      await notifier.loadGearDetails();

      // Should complete without error, owner name just won't be set
      final state = container.read(gearProvider(testGearId));
      expect(state.gearDetails, gear);
      expect(state.ownerName, isNull);
      expect(state.isLoading, isFalse);
      expect(state.error, isNull);
    });

    test('handles error fetching location gracefully', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
        locationId: 'location-123',
      );

      when(mockGearRepository.getGearDetails('gear-123'))
          .thenAnswer((_) async => gear);
      when(mockUserRepository.getUserProfile('owner-123')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(
            userId: 'owner-123',
            name: 'Test Owner',
          ),
        ),
      );
      when(mockLocationService.getLocation('location-123'))
          .thenThrow(Exception('Location not found'));

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(gearId: 'gear-123');

      await notifier.loadGearDetails();

      // Should complete without error, location just won't be set
      final state = container.read(gearProvider(testGearId));
      expect(state.gearDetails, gear);
      expect(state.ownerName, 'Test Owner');
      expect(state.locationName, isNull);
      expect(state.isLoading, isFalse);
      expect(state.error, isNull);
    });

    test('handles multiple media items successfully', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: ['media-1', 'media-2', 'media-3'],
      );

      when(mockGearRepository.getGearDetails('gear-123'))
          .thenAnswer((_) async => gear);
      when(mockUserRepository.getUserProfile('owner-123')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(
            userId: 'owner-123',
            name: 'Test Owner',
          ),
        ),
      );

      when(mockMediaRepository.get('media-1')).thenAnswer(
        (_) async => GetMediaResponse(
          id: 'media-1',
          url: 'https://example.com/image1.jpg',
          contentType: 'image/jpeg',
        ),
      );
      when(mockMediaRepository.get('media-2')).thenAnswer(
        (_) async => GetMediaResponse(
          id: 'media-2',
          url: 'https://example.com/image2.jpg',
          contentType: 'image/jpeg',
        ),
      );
      when(mockMediaRepository.get('media-3')).thenAnswer(
        (_) async => GetMediaResponse(
          id: 'media-3',
          url: 'https://example.com/image3.jpg',
          contentType: 'image/jpeg',
        ),
      );
      when(mockMediaRepository.getFullMediaUrl('media-1')).thenAnswer(
        (_) async => MediaUrl(
          url: 'https://example.com/image1.jpg',
          mediaId: 'media-1',
          isThumbnail: false,
        ),
      );

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(gearId: 'gear-123');

      await notifier.loadGearDetails();

      final state = container.read(gearProvider(testGearId));
      expect(state.allMediaItems.length, 3);
      expect(state.allMediaItems[0].id, 'media-1');
      expect(state.allMediaItems[1].id, 'media-2');
      expect(state.allMediaItems[2].id, 'media-3');
    });

    test('handles partial media loading failure gracefully', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: ['media-1', 'media-2'],
      );

      when(mockGearRepository.getGearDetails('gear-123'))
          .thenAnswer((_) async => gear);
      when(mockUserRepository.getUserProfile('owner-123')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(
            userId: 'owner-123',
            name: 'Test Owner',
          ),
        ),
      );

      when(mockMediaRepository.get('media-1')).thenAnswer(
        (_) async => GetMediaResponse(
          id: 'media-1',
          url: 'https://example.com/image1.jpg',
          contentType: 'image/jpeg',
        ),
      );
      when(mockMediaRepository.get('media-2'))
          .thenThrow(Exception('Media not found'));
      when(mockMediaRepository.getFullMediaUrl('media-1')).thenAnswer(
        (_) async => MediaUrl(
          url: 'https://example.com/image1.jpg',
          mediaId: 'media-1',
          isThumbnail: false,
        ),
      );

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(gearId: 'gear-123');

      await notifier.loadGearDetails();

      // Should only load media-1, media-2 fails gracefully
      final state = container.read(gearProvider(testGearId));
      expect(state.allMediaItems.length, 1);
      expect(state.allMediaItems[0].id, 'media-1');
      expect(state.isLoading, isFalse);
    });

    test('falls back to state.communityId when called without explicit communityId', () async {
      // Regression for PR #1375 / issue #1374: mutations (media reorder, delete,
      // location update) call loadGearDetails() without passing a communityId.
      // Without the fallback, the server omits shared_at_unix_sec and the UI
      // renders "1970-01-01" as "20568 days ago".
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockGearRepository.getGearDetails(
        'gear-123',
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async => gear);
      when(mockUserRepository.getUserProfile('owner-123')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(userId: 'owner-123', name: 'Test Owner'),
        ),
      );

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: 'gear-123',
        communityId: 'community-456',
      );

      await notifier.loadGearDetails();

      verify(mockGearRepository.getGearDetails(
        'gear-123',
        communityId: 'community-456',
      )).called(1);
    });

    test('explicit communityId argument takes precedence over state.communityId', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockGearRepository.getGearDetails(
        'gear-123',
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async => gear);
      when(mockUserRepository.getUserProfile('owner-123')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(userId: 'owner-123', name: 'Test Owner'),
        ),
      );

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: 'gear-123',
        communityId: 'community-from-state',
      );

      await notifier.loadGearDetails(communityId: 'community-explicit');

      verify(mockGearRepository.getGearDetails(
        'gear-123',
        communityId: 'community-explicit',
      )).called(1);
    });

    test('sets loading state during operation', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: 'gear-123',
        isLoading: false,
      );

      when(mockGearRepository.getGearDetails('gear-123')).thenAnswer((_) async {
        // Verify loading state is true during async operation
        final currentState = container.read(gearProvider(testGearId));
        expect(currentState.isLoading, isTrue);
        return gear;
      });
      when(mockUserRepository.getUserProfile('owner-123')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(
            userId: 'owner-123',
            name: 'Test Owner',
          ),
        ),
      );

      await notifier.loadGearDetails();

      final state = container.read(gearProvider(testGearId));
      expect(state.isLoading, isFalse);
    });
  });

  group('saveChanges edge cases', () {
    test('does nothing if gearDetails is null', () async {
      final notifier = container.read(gearProvider(testGearId).notifier);
      // gearDetails is null by default

      await notifier.saveChanges(
        name: 'New Name',
        description: 'New Description',
      );

      verifyNever(mockGearRepository.saveGear(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      ));
    });

    test('trims whitespace from name and description', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Old Name',
        description: 'Old Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockGearRepository.saveGear(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      )).thenAnswer((_) async => {});

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(gearDetails: gear);

      await notifier.saveChanges(
        name: '  New Name  ',
        description: '  New Description  ',
      );

      verify(mockGearRepository.saveGear(
        id: 'gear-123',
        name: 'New Name',
        description: 'New Description',
        mediaIds: [],
      )).called(1);
    });

    test('includes all media items when saving', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Old Name',
        description: 'Old Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: ['media-1', 'media-2'],
      );

      when(mockGearRepository.saveGear(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      )).thenAnswer((_) async => {});

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearDetails: gear,
        allMediaItems: [
          MediaItemData(
            id: 'media-1',
            url: 'url1',
            contentType: 'image/jpeg',
            isVideo: false,
          ),
          MediaItemData(
            id: 'media-2',
            url: 'url2',
            contentType: 'image/jpeg',
            isVideo: false,
          ),
        ],
      );

      await notifier.saveChanges(
        name: 'New Name',
        description: 'New Description',
      );

      verify(mockGearRepository.saveGear(
        id: 'gear-123',
        name: 'New Name',
        description: 'New Description',
        mediaIds: ['media-1', 'media-2'],
      )).called(1);
    });

    test('sets isSaving to false on error', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockGearRepository.saveGear(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      )).thenThrow(Exception('Save failed'));

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(gearDetails: gear);

      try {
        await notifier.saveChanges(
          name: 'New Name',
          description: 'New Description',
        );
      } catch (_) {
        // Expected
      }

      final state = container.read(gearProvider(testGearId));
      expect(state.isSaving, isFalse);
    });
  });

  group('isOwner edge cases', () {
    test('returns false when currentUserId is null', () {
      final notifier = container.read(gearProvider(testGearId).notifier);

      notifier.state = notifier.state.copyWith(
        currentUserId: null,
        gearDetails: GetGearResponse(
          id: 'gear-123',
          name: 'Test Gear',
          description: 'Test Description',
          owner: User(id: 'user-123', name: 'Test User'),
          mediaIds: [],
        ),
      );

      expect(container.read(gearProvider(testGearId)).isOwner, isFalse);
    });

    test('returns false when gearDetails is null', () {
      final notifier = container.read(gearProvider(testGearId).notifier);

      notifier.state = notifier.state.copyWith(
        currentUserId: 'user-123',
        gearDetails: null,
      );

      expect(container.read(gearProvider(testGearId)).isOwner, isFalse);
    });

    test('returns false when both are null', () {
      final notifier = container.read(gearProvider(testGearId).notifier);

      notifier.state = notifier.state.copyWith(
        currentUserId: null,
        gearDetails: null,
      );

      expect(container.read(gearProvider(testGearId)).isOwner, isFalse);
    });
  });

  group('state immutability', () {
    test('state cannot be mutated directly', () {
      final state1 = container.read(gearProvider(testGearId));
      container.read(gearProvider(testGearId).notifier).toggleEditMode();
      final state2 = container.read(gearProvider(testGearId));

      // Original state should be unchanged
      expect(state1.isEditing, isFalse);
      expect(state2.isEditing, isTrue);
    });

    test('copyWith creates new state instance', () {
      final notifier = container.read(gearProvider(testGearId).notifier);
      final state1 = container.read(gearProvider(testGearId));

      notifier.toggleEditMode();
      final state2 = container.read(gearProvider(testGearId));

      expect(identical(state1, state2), isFalse);
    });
  });

  group('updateLocation', () {
    test('preserves all existing fields when updating location', () async {
      // Setup gear with all fields populated
      final gearWithAllFields = GetGearResponse(
        id: testGearId,
        name: 'Test Gear',
        description: 'Test Description',
        mediaIds: ['media-1', 'media-2'],
        locationId: 'old-location',
        owner: User(id: 'owner-123'),
      );

      // Setup mock to return gear details
      when(mockGearRepository.getGearDetails(testGearId))
          .thenAnswer((_) async => gearWithAllFields);

      // Initialize the notifier to load gear details
      final notifier = container.read(gearProvider(testGearId).notifier);
      await notifier.loadGearDetails();

      // Setup mock to capture the saveGear call
      when(mockGearRepository.saveGear(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
        locationId: anyNamed('locationId'),
      )).thenAnswer((_) async => testGearId);

      // Call updateLocation with a new location
      const newLocationId = 'new-location-456';
      await notifier.updateLocation(newLocationId);

      // Verify saveGear was called with ALL existing fields preserved
      verify(mockGearRepository.saveGear(
        id: testGearId,
        name: 'Test Gear',
        description: 'Test Description',
        mediaIds: ['media-1', 'media-2'],
        locationId: newLocationId,
      )).called(1);
    });

    test('does nothing if gearDetails is null', () async {
      final notifier = container.read(gearProvider(testGearId).notifier);

      // Call updateLocation without loading gear first
      await notifier.updateLocation('new-location');

      // Verify saveGear was never called
      verifyNever(mockGearRepository.saveGear(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
        locationId: anyNamed('locationId'),
      ));
    });
  });

  group('Media Upload Ordering', () {
    // Note: Full media upload testing requires mocking MediaPickerHelper
    // and file system operations. These tests document the expected behavior
    // of the insertAtFront parameter. Integration tests verify the complete flow.

    test('picker methods accept insertAtFront parameter', () {
      // This test verifies that the API signature is correct
      // The methods should compile and accept the insertAtFront parameter
      final notifier = container.read(gearProvider(testGearId).notifier);

      // Verify the methods exist with the correct signature
      expect(notifier.pickImageFromGallery, isA<Function>());
      expect(notifier.pickImageFromCamera, isA<Function>());
      expect(notifier.pickVideoFromGallery, isA<Function>());

      // The actual behavior (prepend vs append) is tested in integration tests
      // since it requires mocking file pickers and media upload infrastructure
    });

    test('video upload writes server URL, not local file path, to mediaPath', () async {
      const localFilePath =
          '/Users/test/Library/Developer/CoreSimulator/tmp/image_picker_video.mp4';
      const newMediaId = 'media-new';
      const serverUrl = 'https://example.com/signed-bucket/v.mp4';

      when(
        mockMediaRepository.addMedia(
          file: argThat(isA<XFile>(), named: 'file'),
          description: anyNamed('description'),
        ),
      ).thenAnswer((_) async => newMediaId);
      when(mockMediaRepository.getMedia(newMediaId)).thenAnswer(
        (_) async => GetMediaResponse(
          id: newMediaId,
          url: serverUrl,
          contentType: 'video/mp4',
        ),
      );

      // Drive the upload without initializing gear; with state.gearDetails == null,
      // _setMediaFile skips the saveChanges branch, isolating the state update we
      // care about (the mediaPath assignment). state.allMediaItems is also empty,
      // so becomesPrimary is true and the new video claims the background slot.
      final notifier = container.read(gearProvider(testGearId).notifier);
      await notifier.setMediaFileForTesting(XFile(localFilePath));

      final state = container.read(gearProvider(testGearId));
      expect(state.mediaId, newMediaId);
      expect(state.isVideo, isTrue);
      expect(state.mediaPath, serverUrl);
      expect(state.mediaPath, isNot(startsWith('/Users/')));
      expect(state.mediaPath, isNot(startsWith('/tmp/')));
    });

    test('carousel-append video leaves the existing background untouched', () async {
      const localFilePath =
          '/Users/test/Library/Developer/CoreSimulator/tmp/image_picker_video.mp4';
      const existingId = 'media-existing';
      const existingUrl = 'https://example.com/existing.jpg';
      const newId = 'media-new';
      const newUrl = 'https://example.com/new-video.mp4';

      // Seed state with one existing image item; gearDetails stays null so
      // saveChanges is skipped and the assertion stays focused on the
      // optimistic state update inside _setMediaFile.
      final notifier = container.read(gearProvider(testGearId).notifier);
      const existingItem = MediaItemData(
        id: existingId,
        url: existingUrl,
        contentType: 'image/jpeg',
        isVideo: false,
      );
      notifier.state = notifier.state.copyWith(
        allMediaItems: const [existingItem],
        mediaId: existingId,
        mediaPath: existingUrl,
        isVideo: false,
      );

      when(
        mockMediaRepository.addMedia(
          file: argThat(isA<XFile>(), named: 'file'),
          description: anyNamed('description'),
        ),
      ).thenAnswer((_) async => newId);
      when(mockMediaRepository.getMedia(newId)).thenAnswer(
        (_) async => GetMediaResponse(
          id: newId,
          url: newUrl,
          contentType: 'video/mp4',
        ),
      );

      // Carousel-add uses the default insertAtFront: false.
      await notifier.setMediaFileForTesting(XFile(localFilePath));

      final state = container.read(gearProvider(testGearId));
      // The new video is appended.
      expect(state.allMediaItems.map((m) => m.id).toList(), [existingId, newId]);
      // The existing image remains the background.
      expect(state.mediaId, existingId);
      expect(state.mediaPath, existingUrl);
      expect(state.isVideo, isFalse);
    });
  });

  group('saveMetadata', () {
    test('successfully saves metadata via repository and reloads', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: ['media-1'],
      );

      when(mockGearRepository.saveGear(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
        metadata: anyNamed('metadata'),
      )).thenAnswer((_) async => {});

      // Mock the reload after save
      when(mockGearRepository.getGearDetails('gear-123'))
          .thenAnswer((_) async => gear);
      when(mockUserRepository.getUserProfile('owner-123')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(
            userId: 'owner-123',
            name: 'Test Owner',
          ),
        ),
      );

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: 'gear-123',
        gearDetails: gear,
      );

      final metadata = GearMetadata(
        brand: TrackedString(value: 'Coleman'),
        model: TrackedString(value: 'Sundome'),
      );

      await notifier.saveMetadata(GearMetadataEditResult(metadata: metadata));

      // Verify repository was called with correct metadata
      verify(mockGearRepository.saveGear(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        mediaIds: ['media-1'],
        metadata: metadata,
      )).called(1);

      // Verify gear details were reloaded
      verify(mockGearRepository.getGearDetails('gear-123')).called(1);
    });

    test('merges value estimate into metadata', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockGearRepository.saveGear(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
        metadata: anyNamed('metadata'),
      )).thenAnswer((_) async => {});

      when(mockGearRepository.getGearDetails('gear-123'))
          .thenAnswer((_) async => gear);
      when(mockUserRepository.getUserProfile('owner-123')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(
            userId: 'owner-123',
            name: 'Test Owner',
          ),
        ),
      );

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: 'gear-123',
        gearDetails: gear,
      );

      final metadata = GearMetadata(
        brand: TrackedString(value: 'DeWalt'),
      );
      final ve = ValueEstimate(estimatedValueUsd: 99);

      await notifier.saveMetadata(
        GearMetadataEditResult(metadata: metadata, valueEstimate: ve),
      );

      // Verify the metadata passed to saveGear has the value estimate merged in
      final captured = verify(mockGearRepository.saveGear(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        mediaIds: [],
        metadata: captureAnyNamed('metadata'),
      )).captured.single as GearMetadata;

      expect(captured.hasValueEstimate(), isTrue);
      expect(captured.valueEstimate.estimatedValueUsd, 99.0);
      expect(captured.brand.value, 'DeWalt');
    });

    test('does nothing if gearDetails is null', () async {
      final notifier = container.read(gearProvider(testGearId).notifier);
      // gearDetails is null by default

      final metadata = GearMetadata(
        brand: TrackedString(value: 'Test'),
      );

      await notifier.saveMetadata(GearMetadataEditResult(metadata: metadata));

      verifyNever(mockGearRepository.saveGear(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
        metadata: anyNamed('metadata'),
      ));
    });

    test('sets isSaving to false on error and rethrows', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockGearRepository.saveGear(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
        metadata: anyNamed('metadata'),
      )).thenThrow(Exception('Save failed'));

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(gearDetails: gear);

      final metadata = GearMetadata(
        brand: TrackedString(value: 'Test'),
      );

      expect(
        () => notifier.saveMetadata(GearMetadataEditResult(metadata: metadata)),
        throwsException,
      );

      final state = container.read(gearProvider(testGearId));
      expect(state.isSaving, isFalse);
    });

    test('sets isSaving to true during save', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockGearRepository.saveGear(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
        metadata: anyNamed('metadata'),
      )).thenAnswer((_) async {
        // Check isSaving during the save
        final currentState = container.read(gearProvider(testGearId));
        expect(currentState.isSaving, isTrue);
      });

      when(mockGearRepository.getGearDetails('gear-123'))
          .thenAnswer((_) async => gear);
      when(mockUserRepository.getUserProfile('owner-123')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(
            userId: 'owner-123',
            name: 'Test Owner',
          ),
        ),
      );

      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: 'gear-123',
        gearDetails: gear,
      );

      final metadata = GearMetadata(brand: TrackedString(value: 'Test'));
      await notifier.saveMetadata(GearMetadataEditResult(metadata: metadata));
    });
  });

  group('loadGearDetails community guard', () {
    test('corrects stale communityId to first shared community on load', () async {
      const gearId = 'gear-guard-test';
      final gear = GetGearResponse(
        id: gearId,
        name: 'Guard Test Gear',
        description: 'Tests stale community correction',
        owner: User(id: 'owner-123', name: 'Owner'),
        mediaIds: [],
        sharedCommunities: [SharedCommunity(communityId: 'right')],
      );

      when(mockGearRepository.getGearDetails(
        gearId,
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async => gear);

      when(mockUserRepository.getUserProfile('owner-123')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(userId: 'owner-123', name: 'Owner'),
        ),
      );

      final localContainer = ProviderContainer(
        overrides: [
          gearRepositoryProvider.overrideWithValue(mockGearRepository),
          userRepositoryProvider.overrideWithValue(mockUserRepository),
          mediaRepositoryProvider.overrideWithValue(mockMediaRepository),
          communityRepositoryProvider.overrideWithValue(mockCommunityRepository),
          gearServiceProvider.overrideWithValue(mockGearService),
          mediaServiceProvider.overrideWithValue(mockMediaService),
          locationServiceProvider.overrideWithValue(mockLocationService),
          observabilityServiceProvider.overrideWithValue(mockObservability),
        ],
      );
      addTearDown(localContainer.dispose);

      final notifier = localContainer.read(gearProvider(gearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: gearId,
        communityId: 'wrong',
      );

      await notifier.loadGearDetails();

      final state = localContainer.read(gearProvider(gearId));
      expect(state.communityId, 'right');
    });

    // Phase 4 of #1705: page-load membership scoping. When a multi-community
    // gear page is opened with a state.communityId pointing at a community
    // the user is NOT a member of, loadGearDetails must resolve to one the
    // user IS a member of before issuing getGearTransferContext, otherwise
    // the server rejects with permission_denied.
    test(
        'page-load: resolves to a community the user is a member of for '
        'multi-community gear and issues getGearTransferContext with that id',
        () async {
      const gearId = 'gear-membership-test';
      // Gear shared into both 'a' (the page-state community) and 'b'
      // (the user's actual member community).
      final gear = GetGearResponse(
        id: gearId,
        name: 'Multi-community gear',
        description: '',
        owner: User(id: 'owner-123', name: 'Owner'),
        mediaIds: [],
        sharedCommunities: [
          SharedCommunity(communityId: 'a'),
          SharedCommunity(communityId: 'b'),
        ],
      );

      when(mockGearRepository.getGearDetails(
        gearId,
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async => gear);
      when(mockUserRepository.getUserProfile('owner-123')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(userId: 'owner-123', name: 'Owner'),
        ),
      );
      when(mockCommunityRepository.listUserCommunities()).thenAnswer(
        (_) async => [CommunityItem(id: 'b'), CommunityItem(id: 'c')],
      );
      when(mockTransferRepository.getGearTransferContext(
        gearId: anyNamed('gearId'),
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async => GearTransferContext());

      final notifier = container.read(gearProvider(gearId).notifier);
      // Caller passed 'a' (the user is NOT a member of 'a').
      await notifier.loadGearDetails(communityId: 'a');

      // The followup transfer-context call must use 'b', not 'a'.
      verify(mockTransferRepository.getGearTransferContext(
        gearId: gearId,
        communityId: 'b',
      )).called(1);
      verifyNever(mockTransferRepository.getGearTransferContext(
        gearId: anyNamed('gearId'),
        communityId: 'a',
      ));
      // State should reflect the corrected id.
      expect(container.read(gearProvider(gearId)).communityId, 'b');
    });

    test(
        'page-load: re-fetches with the resolved community so a giveaway is '
        'not rendered as a loan', () async {
      const gearId = 'gear-giveaway';
      // First fetch has no community context, so the server cannot resolve a
      // CommunityGear and availability comes back UNSPECIFIED (which the UI
      // would treat as a loan).
      final unscoped = GetGearResponse(
        id: gearId,
        name: 'Free shovel',
        description: '',
        owner: User(id: 'owner-1', name: 'Owner'),
        mediaIds: [],
        sharedCommunities: [SharedCommunity(communityId: 'comm-1')],
        availability: Availability.AVAILABILITY_UNSPECIFIED,
      );
      // Re-fetch scoped to the resolved community carries the real availability.
      final scoped = GetGearResponse(
        id: gearId,
        name: 'Free shovel',
        description: '',
        owner: User(id: 'owner-1', name: 'Owner'),
        mediaIds: [],
        sharedCommunities: [SharedCommunity(communityId: 'comm-1')],
        availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
      );

      when(mockGearRepository.getGearDetails(gearId, communityId: null))
          .thenAnswer((_) async => unscoped);
      when(mockGearRepository.getGearDetails(gearId, communityId: 'comm-1'))
          .thenAnswer((_) async => scoped);
      when(mockUserRepository.getUserProfile('owner-1')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(userId: 'owner-1', name: 'Owner'),
        ),
      );
      when(mockCommunityRepository.listUserCommunities()).thenAnswer(
        (_) async => [CommunityItem(id: 'comm-1')],
      );
      when(mockTransferRepository.getGearTransferContext(
        gearId: anyNamed('gearId'),
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async => GearTransferContext());

      final notifier = container.read(gearProvider(gearId).notifier);
      // No community context supplied — the worst case (deep link / fresh
      // creation) that previously left availability UNSPECIFIED.
      await notifier.loadGearDetails();

      final state = container.read(gearProvider(gearId));
      expect(state.communityId, 'comm-1');
      expect(state.gearDetails?.availability,
          Availability.AVAILABILITY_FOR_GIVEAWAY);
      verify(mockGearRepository.getGearDetails(gearId, communityId: 'comm-1'))
          .called(1);
    });

    test(
        'page-load: leaves supplied id alone when the user is a member of it',
        () async {
      const gearId = 'gear-membership-ok';
      final gear = GetGearResponse(
        id: gearId,
        name: 'gear',
        description: '',
        owner: User(id: 'owner-1', name: 'Owner'),
        mediaIds: [],
        sharedCommunities: [
          SharedCommunity(communityId: 'a'),
          SharedCommunity(communityId: 'b'),
        ],
      );
      when(mockGearRepository.getGearDetails(gearId,
              communityId: anyNamed('communityId')))
          .thenAnswer((_) async => gear);
      when(mockUserRepository.getUserProfile('owner-1')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(userId: 'owner-1', name: 'Owner'),
        ),
      );
      when(mockCommunityRepository.listUserCommunities()).thenAnswer(
        (_) async => [CommunityItem(id: 'a'), CommunityItem(id: 'b')],
      );
      when(mockTransferRepository.getGearTransferContext(
        gearId: anyNamed('gearId'),
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async => GearTransferContext());

      final notifier = container.read(gearProvider(gearId).notifier);
      await notifier.loadGearDetails(communityId: 'a');

      verify(mockTransferRepository.getGearTransferContext(
        gearId: gearId,
        communityId: 'a',
      )).called(1);
    });

    test(
        'page-load: degrades to first shared community when listUserCommunities '
        'fails (network)', () async {
      const gearId = 'gear-membership-degraded';
      final gear = GetGearResponse(
        id: gearId,
        name: 'gear',
        description: '',
        owner: User(id: 'owner-1', name: 'Owner'),
        mediaIds: [],
        sharedCommunities: [
          SharedCommunity(communityId: 'a'),
          SharedCommunity(communityId: 'b'),
        ],
      );
      when(mockGearRepository.getGearDetails(gearId,
              communityId: anyNamed('communityId')))
          .thenAnswer((_) async => gear);
      when(mockUserRepository.getUserProfile('owner-1')).thenAnswer(
        (_) async => UserProfile(
          user: GetUserResponse(userId: 'owner-1', name: 'Owner'),
        ),
      );
      when(mockCommunityRepository.listUserCommunities())
          .thenThrow(Exception('network down'));
      when(mockTransferRepository.getGearTransferContext(
        gearId: anyNamed('gearId'),
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async => GearTransferContext());

      final notifier = container.read(gearProvider(gearId).notifier);
      // No supplied communityId — falls back to first shared = 'a'.
      await notifier.loadGearDetails();

      verify(mockTransferRepository.getGearTransferContext(
        gearId: gearId,
        communityId: 'a',
      )).called(1);
    });
  });

  group('expressInterest', () {
    // Verifies the fix for #1705: client must use the server-authoritative
    // community_id from the mutation response when issuing the followup
    // refresh, not state.communityId, which can point at a non-member
    // community for multi-community gear.
    test(
        'uses server-chosen community_id from mutation response, not state.communityId',
        () async {
      final notifier = container.read(gearProvider(testGearId).notifier);
      // state.communityId is a community the user is NOT a member of (the bug
      // condition: gear is in {state.communityId, server-chosen}, user is in
      // server-chosen only).
      notifier.state = notifier.state.copyWith(
        gearId: testGearId,
        communityId: 'wrong-community',
      );

      final newTransfer = Transfer(
        id: 'transfer-1',
        gearId: testGearId,
        communityId: 'server-chosen-community',
      );
      when(mockTransferRepository.expressInterest(gearId: testGearId))
          .thenAnswer((_) async => newTransfer);
      when(mockTransferRepository.getGearTransferContext(
        gearId: testGearId,
        communityId: 'server-chosen-community',
      )).thenAnswer(
        (_) async => GearTransferContext(userTransfer: newTransfer),
      );
      when(mockGearRepository.getStats(any, communityId: anyNamed('communityId'))).thenAnswer((_) async => GetGearStatsResponse());
      when(mockGearRepository.getPeople(any, communityId: anyNamed('communityId'))).thenAnswer((_) async => GetGearPeopleResponse());

      await notifier.expressInterest();

      // Verify followup used the server-chosen id, not state.communityId.
      verify(mockTransferRepository.getGearTransferContext(
        gearId: testGearId,
        communityId: 'server-chosen-community',
      )).called(1);
      verifyNever(mockTransferRepository.getGearTransferContext(
        gearId: testGearId,
        communityId: 'wrong-community',
      ));

      final state = container.read(gearProvider(testGearId));
      expect(state.transferContext?.hasUserTransfer(), isTrue);
      expect(state.transferContext!.userTransfer.id, 'transfer-1');
      expect(state.isSaving, isFalse);
      expect(state.error, isNull);
    });

    // Closes #1684/#1685: when the post-mutation refresh fails (e.g., the
    // server still rejects the followup), local state must still reflect the
    // newly-created transfer so the UI flips to "in line" and the user
    // doesn't tap Borrow again.
    test(
        'falls back to optimistic context when post-mutation refresh fails',
        () async {
      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(gearId: testGearId);

      final newTransfer = Transfer(
        id: 'transfer-2',
        gearId: testGearId,
        communityId: 'server-chosen-community',
      );
      when(mockTransferRepository.expressInterest(gearId: testGearId))
          .thenAnswer((_) async => newTransfer);
      when(mockTransferRepository.getGearTransferContext(
        gearId: anyNamed('gearId'),
        communityId: anyNamed('communityId'),
      )).thenThrow(Exception('permission_denied'));
      when(mockGearRepository.getStats(any, communityId: anyNamed('communityId'))).thenAnswer((_) async => GetGearStatsResponse());
      when(mockGearRepository.getPeople(any, communityId: anyNamed('communityId'))).thenAnswer((_) async => GetGearPeopleResponse());

      await notifier.expressInterest();

      final state = container.read(gearProvider(testGearId));
      expect(state.transferContext?.hasUserTransfer(), isTrue,
          reason: 'optimistic update should set userTransfer from mutation '
              'response even when refresh fails');
      expect(state.transferContext!.userTransfer.id, 'transfer-2');
      expect(state.isSaving, isFalse);
      // Refresh failure is a recovery path, not a user-facing error.
      expect(state.error, isNull);
    });

    // Closes Bug A of #1703: a permission_denied (or any error) on the
    // mutation itself becomes a typed UserError on state, not a rethrow that
    // bubbles to PlatformDispatcher.onError as a fatal Crashlytics record.
    test('records typed UserError on mutation failure without rethrowing',
        () async {
      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(gearId: testGearId);

      when(mockTransferRepository.expressInterest(gearId: testGearId))
          .thenThrow(Exception('boom'));

      // Must complete without throwing — the catch path stores the error on
      // state instead of rethrowing.
      await notifier.expressInterest();

      final state = container.read(gearProvider(testGearId));
      expect(state.error, isNotNull);
      expect(state.isSaving, isFalse);
      // transferContext stays null since the mutation never succeeded.
      expect(state.transferContext, isNull);
      verifyNever(mockTransferRepository.getGearTransferContext(
        gearId: anyNamed('gearId'),
        communityId: anyNamed('communityId'),
      ));
    });

    test('returns early when gearId is null', () async {
      final notifier = container.read(gearProvider(testGearId).notifier);
      // Force state.gearId to null to exercise the early-return guard
      // (the family parameter normally seeds state.gearId in the
      // constructor).
      notifier.state = notifier.state.copyWith(gearId: null);
      await notifier.expressInterest();

      verifyNever(mockTransferRepository.expressInterest(
          gearId: anyNamed('gearId')));
    });

    test('is no-op while isSaving is true', () async {
      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: testGearId,
        isSaving: true,
      );

      await notifier.expressInterest();

      verifyNever(mockTransferRepository.expressInterest(
          gearId: anyNamed('gearId')));
    });

    // Disposal safety per docs/client/architecture.md "Disposal Testing".
    test('completes safely when container is disposed mid-flight', () async {
      final localContainer = ProviderContainer(
        overrides: [
          gearRepositoryProvider.overrideWithValue(mockGearRepository),
          transferRepositoryProvider.overrideWithValue(mockTransferRepository),
          userRepositoryProvider.overrideWithValue(mockUserRepository),
          mediaRepositoryProvider.overrideWithValue(mockMediaRepository),
          communityRepositoryProvider.overrideWithValue(mockCommunityRepository),
          gearServiceProvider.overrideWithValue(mockGearService),
          mediaServiceProvider.overrideWithValue(mockMediaService),
          locationServiceProvider.overrideWithValue(mockLocationService),
          observabilityServiceProvider.overrideWithValue(mockObservability),
        ],
      );

      final notifier = localContainer.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(gearId: testGearId);

      when(mockTransferRepository.expressInterest(gearId: testGearId))
          .thenAnswer((_) async {
        // Dispose while the mutation is mid-flight.
        localContainer.dispose();
        return Transfer(
          id: 't',
          gearId: testGearId,
          communityId: 'c',
        );
      });

      await expectLater(notifier.expressInterest(), completes);
    });
  });

  group('withdrawInterest', () {
    // Verifies the fix for #1705 on the withdraw path: refresh uses the
    // *transfer's* community_id (server-authoritative), not state.communityId.
    test(
        'uses transfer.communityId for refresh, not state.communityId',
        () async {
      final notifier = container.read(gearProvider(testGearId).notifier);
      final existingTransfer = Transfer(
        id: 'transfer-w1',
        gearId: testGearId,
        communityId: 'server-chosen-community',
      );
      notifier.state = notifier.state.copyWith(
        gearId: testGearId,
        communityId: 'wrong-community',
        transferContext: GearTransferContext(userTransfer: existingTransfer),
      );

      when(mockTransferRepository.withdrawInterest(transferId: 'transfer-w1'))
          .thenAnswer((_) async {});
      when(mockTransferRepository.getGearTransferContext(
        gearId: testGearId,
        communityId: 'server-chosen-community',
      )).thenAnswer((_) async => GearTransferContext());
      when(mockGearRepository.getStats(any, communityId: anyNamed('communityId'))).thenAnswer((_) async => GetGearStatsResponse());
      when(mockGearRepository.getPeople(any, communityId: anyNamed('communityId'))).thenAnswer((_) async => GetGearPeopleResponse());

      await notifier.withdrawInterest();

      verify(mockTransferRepository.getGearTransferContext(
        gearId: testGearId,
        communityId: 'server-chosen-community',
      )).called(1);
      verifyNever(mockTransferRepository.getGearTransferContext(
        gearId: testGearId,
        communityId: 'wrong-community',
      ));

      final state = container.read(gearProvider(testGearId));
      expect(state.transferContext?.hasUserTransfer(), isFalse);
      expect(state.isSaving, isFalse);
      expect(state.error, isNull);
    });

    // Symmetrical to expressInterest: refresh failure → optimistic clear.
    test(
        'optimistically clears userTransfer when post-mutation refresh fails',
        () async {
      final notifier = container.read(gearProvider(testGearId).notifier);
      final existingTransfer = Transfer(
        id: 'transfer-w2',
        gearId: testGearId,
        communityId: 'c',
      );
      notifier.state = notifier.state.copyWith(
        gearId: testGearId,
        transferContext: GearTransferContext(userTransfer: existingTransfer),
      );

      when(mockTransferRepository.withdrawInterest(transferId: 'transfer-w2'))
          .thenAnswer((_) async {});
      when(mockTransferRepository.getGearTransferContext(
        gearId: anyNamed('gearId'),
        communityId: anyNamed('communityId'),
      )).thenThrow(Exception('permission_denied'));
      when(mockGearRepository.getStats(any, communityId: anyNamed('communityId'))).thenAnswer((_) async => GetGearStatsResponse());
      when(mockGearRepository.getPeople(any, communityId: anyNamed('communityId'))).thenAnswer((_) async => GetGearPeopleResponse());

      await notifier.withdrawInterest();

      final state = container.read(gearProvider(testGearId));
      expect(state.transferContext?.hasUserTransfer(), isFalse,
          reason: 'optimistic clear should remove userTransfer when refresh '
              'fails so UI returns to "Borrow" state');
      expect(state.isSaving, isFalse);
      expect(state.error, isNull);
    });

    test('records typed UserError on mutation failure without rethrowing',
        () async {
      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: testGearId,
        transferContext: GearTransferContext(
          userTransfer: Transfer(id: 't', gearId: testGearId, communityId: 'c'),
        ),
      );

      when(mockTransferRepository.withdrawInterest(transferId: 't'))
          .thenThrow(Exception('boom'));

      await notifier.withdrawInterest();

      final state = container.read(gearProvider(testGearId));
      expect(state.error, isNotNull);
      expect(state.isSaving, isFalse);
      // The original transferContext is unchanged since the mutation never
      // succeeded — the user is still in line.
      expect(state.transferContext?.hasUserTransfer(), isTrue);
    });

    test('returns early when no userTransfer is present', () async {
      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(gearId: testGearId);

      await notifier.withdrawInterest();

      verifyNever(mockTransferRepository.withdrawInterest(
          transferId: anyNamed('transferId')));
    });

    test('is no-op while isSaving is true', () async {
      final notifier = container.read(gearProvider(testGearId).notifier);
      final existingTransfer = Transfer(
        id: 'transfer-saving',
        gearId: testGearId,
        communityId: 'some-community',
      );
      notifier.state = notifier.state.copyWith(
        gearId: testGearId,
        isSaving: true,
        transferContext: GearTransferContext(userTransfer: existingTransfer),
      );

      await notifier.withdrawInterest();

      verifyNever(mockTransferRepository.withdrawInterest(
          transferId: anyNamed('transferId')));
    });
  });

  group('ensureGearConversation', () {
    test('no-op when conversationId is already set', () async {
      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: testGearId,
        gearDetails: GetGearResponse(
          id: testGearId,
          conversationId: 'existing-conv-id',
        ),
        sharedCommunities: [SharedCommunity(communityId: 'comm-1')],
        communityId: 'comm-1',
      );

      await notifier.ensureGearConversation();

      verifyNever(mockChatRepository.startGearConversation(
        gearId: anyNamed('gearId'),
        communityId: anyNamed('communityId'),
      ));
    });

    test('no-op when gear has no shared communities', () async {
      final notifier = container.read(gearProvider(testGearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: testGearId,
        gearDetails: GetGearResponse(id: testGearId),
        sharedCommunities: [],
      );

      await notifier.ensureGearConversation();

      verifyNever(mockChatRepository.startGearConversation(
        gearId: anyNamed('gearId'),
        communityId: anyNamed('communityId'),
      ));
    });

    test('calls startGearConversation with current community, invalidates gear, and reloads', () async {
      const gearId = testGearId;
      const communityId = 'comm-1';

      final notifier = container.read(gearProvider(gearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: gearId,
        gearDetails: GetGearResponse(id: gearId),
        sharedCommunities: [SharedCommunity(communityId: communityId)],
        communityId: communityId,
      );

      when(mockChatRepository.startGearConversation(
        gearId: gearId,
        communityId: communityId,
      )).thenAnswer((_) async => 'new-conv-id');

      when(mockGearRepository.invalidate(gearId, communityId: communityId))
          .thenAnswer((_) async {});

      final updatedGear = GetGearResponse(
        id: gearId,
        conversationId: 'new-conv-id',
      );
      when(mockGearRepository.getGearDetails(gearId,
              communityId: anyNamed('communityId')))
          .thenAnswer((_) async => updatedGear);

      await notifier.ensureGearConversation();

      verify(mockChatRepository.startGearConversation(
        gearId: gearId,
        communityId: communityId,
      )).called(1);
      verify(mockGearRepository.invalidate(gearId, communityId: communityId))
          .called(1);
      verify(mockGearRepository.getGearDetails(gearId,
              communityId: anyNamed('communityId')))
          .called(1);
      final state = container.read(gearProvider(gearId));
      expect(state.error, isNull);
    });

    test('falls back to first shared community when current community not in share set', () async {
      const gearId = testGearId;

      final notifier = container.read(gearProvider(gearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: gearId,
        gearDetails: GetGearResponse(id: gearId),
        sharedCommunities: [SharedCommunity(communityId: 'other-comm')],
        communityId: 'unrelated-comm',
      );

      when(mockChatRepository.startGearConversation(
        gearId: gearId,
        communityId: 'other-comm',
      )).thenAnswer((_) async => 'new-conv-id');

      when(mockGearRepository.invalidate(gearId,
              communityId: anyNamed('communityId')))
          .thenAnswer((_) async {});

      when(mockGearRepository.getGearDetails(gearId,
              communityId: anyNamed('communityId')))
          .thenAnswer((_) async => GetGearResponse(
                id: gearId,
                conversationId: 'new-conv-id',
              ));

      await notifier.ensureGearConversation();

      verify(mockChatRepository.startGearConversation(
        gearId: gearId,
        communityId: 'other-comm',
      )).called(1);
    });

    test('sets error state and clears in-flight guard when service throws', () async {
      const gearId = testGearId;
      const communityId = 'comm-1';

      final notifier = container.read(gearProvider(gearId).notifier);
      notifier.state = notifier.state.copyWith(
        gearId: gearId,
        gearDetails: GetGearResponse(id: gearId),
        sharedCommunities: [SharedCommunity(communityId: communityId)],
        communityId: communityId,
      );

      when(mockChatRepository.startGearConversation(
        gearId: gearId,
        communityId: communityId,
      )).thenThrow(Exception('network error'));

      await notifier.ensureGearConversation();

      final state = container.read(gearProvider(gearId));
      expect(state.error, isNotNull);
      verifyNever(mockGearRepository.invalidate(
        any,
        communityId: anyNamed('communityId'),
      ));

      // Guard must be cleared so a retry call can proceed.
      clearInteractions(mockChatRepository);
      when(mockChatRepository.startGearConversation(
        gearId: gearId,
        communityId: communityId,
      )).thenAnswer((_) async => 'new-conv-id');
      when(mockGearRepository.invalidate(gearId,
              communityId: anyNamed('communityId')))
          .thenAnswer((_) async {});
      when(mockGearRepository.getGearDetails(gearId,
              communityId: anyNamed('communityId')))
          .thenAnswer((_) async => GetGearResponse(
                id: gearId,
                conversationId: 'new-conv-id',
              ));
      await notifier.ensureGearConversation();
      verify(mockChatRepository.startGearConversation(
        gearId: gearId,
        communityId: communityId,
      )).called(1);
    });
  });
}
