import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/presentation/viewmodels/community_view_model.dart';
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/media_service.dart';
import 'package:ripls/services/providers.dart' as service_providers;

import 'community_view_model_test.mocks.dart';

@GenerateMocks([
  CommunityRepository,
  MediaRepository,
  CommunityService,
  MediaService,
])
void main() {
  late ProviderContainer container;
  late MockCommunityRepository mockCommunityRepository;
  late MockMediaRepository mockMediaRepository;
  late MockCommunityService mockCommunityService;
  late MockMediaService mockMediaService;

  setUp(() {
    mockCommunityRepository = MockCommunityRepository();
    mockMediaRepository = MockMediaRepository();
    mockCommunityService = MockCommunityService();
    mockMediaService = MockMediaService();

    container = ProviderContainer(
      overrides: [
        service_providers.communityRepositoryProvider.overrideWithValue(mockCommunityRepository),
        service_providers.mediaRepositoryProvider.overrideWithValue(mockMediaRepository),
        service_providers.communityServiceProvider.overrideWithValue(mockCommunityService),
        service_providers.mediaServiceProvider.overrideWithValue(mockMediaService),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockCommunityRepository);
    reset(mockMediaRepository);
    reset(mockCommunityService);
    reset(mockMediaService);
  });

  group('CommunityState', () {
    test('initial state is correct', () {
      final state = container.read(communityProvider);

      expect(state.communityId, isNull);
      expect(state.communityName, isNull);
      expect(state.communityDescription, isNull);
      expect(state.mediaPath, isNull);
      expect(state.mediaId, isNull);
      expect(state.isVideo, isFalse);
      expect(state.isLoading, isTrue);
      expect(state.isEditing, isFalse);
      expect(state.isSaving, isFalse);
      expect(state.isUploadingMedia, isFalse);
      expect(state.error, isNull);
    });

    test('hasError returns correct value', () {
      final notifier = container.read(communityProvider.notifier);

      // No error initially
      expect(container.read(communityProvider).hasError, isFalse);

      // Set error
      notifier.state = notifier.state.copyWith(
          error: const UserError.generic(fallback: 'Test error'));
      expect(container.read(communityProvider).hasError, isTrue);
    });
  });

  group('toggleEditMode', () {
    test('toggles edit mode on and off', () {
      final notifier = container.read(communityProvider.notifier);

      expect(container.read(communityProvider).isEditing, isFalse);

      notifier.toggleEditMode();
      expect(container.read(communityProvider).isEditing, isTrue);

      notifier.toggleEditMode();
      expect(container.read(communityProvider).isEditing, isFalse);
    });
  });

  group('cancelEdit', () {
    test('sets editing to false', () {
      final notifier = container.read(communityProvider.notifier);

      // First toggle to true
      notifier.toggleEditMode();
      expect(container.read(communityProvider).isEditing, isTrue);

      // Cancel edit
      notifier.cancelEdit();
      expect(container.read(communityProvider).isEditing, isFalse);
    });
  });

  group('saveChanges', () {
    test('does nothing if communityId is null', () async {
      final notifier = container.read(communityProvider.notifier);
      // State has no communityId

      await notifier.saveChanges(
        name: 'Test',
        description: 'Test',
      );

      verifyNever(mockCommunityService.updateCommunity(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      ));
    });

    test('sets saving state correctly', () async {
      const communityId = 'community-123';
      const updatedName = 'Updated Name';
      const updatedDescription = 'Updated Description';

      final notifier = container.read(communityProvider.notifier);
      notifier.state = notifier.state.copyWith(
        communityId: communityId,
        communityName: 'Original Name',
        communityDescription: 'Original Description',
        isEditing: true,
        isLoading: false,
      );

      when(mockCommunityRepository.updateCommunity(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaId: anyNamed('mediaId'),
      )).thenThrow(Exception('Save failed'));

      expect(
        () => notifier.saveChanges(
          name: updatedName,
          description: updatedDescription,
        ),
        throwsA(isA<Exception>()),
      );

      final state = container.read(communityProvider);
      expect(state.isSaving, isFalse);
    });
  });

  group('refresh', () {
    test('does nothing if communityId is null', () async {
      final notifier = container.read(communityProvider.notifier);
      // State has no communityId

      await notifier.refresh();

      verifyNever(mockCommunityRepository.get(any));
    });
  });

  group('state management', () {
    test('can set communityId', () {
      final notifier = container.read(communityProvider.notifier);

      notifier.state = notifier.state.copyWith(
        communityId: 'test-id',
        communityName: 'Test Community',
      );

      final state = container.read(communityProvider);
      expect(state.communityId, 'test-id');
      expect(state.communityName, 'Test Community');
    });

    test('can set loading state', () {
      final notifier = container.read(communityProvider.notifier);

      notifier.state = notifier.state.copyWith(
        isLoading: false,
      );

      expect(container.read(communityProvider).isLoading, isFalse);
    });

    test('can set media state', () {
      final notifier = container.read(communityProvider.notifier);

      notifier.state = notifier.state.copyWith(
        mediaPath: 'https://example.com/image.jpg',
        mediaId: 'media-123',
        isVideo: false,
      );

      final state = container.read(communityProvider);
      expect(state.mediaPath, 'https://example.com/image.jpg');
      expect(state.mediaId, 'media-123');
      expect(state.isVideo, isFalse);
    });
  });

  group('initialize', () {
    test('sets communityId and loads details successfully', () async {
      const communityId = 'community-123';
      final community = GetCommunityResponse(
        id: communityId,
        name: 'Test Community',
        description: 'A test community',
      );

      when(mockCommunityRepository.get(communityId))
          .thenAnswer((_) async => community);

      final notifier = container.read(communityProvider.notifier);
      await notifier.initialize(communityId);

      final state = container.read(communityProvider);
      expect(state.communityId, communityId);
      expect(state.communityName, 'Test Community');
      expect(state.communityDescription, 'A test community');
      expect(state.isLoading, isFalse);
      expect(state.error, isNull);
      verify(mockCommunityRepository.get(communityId)).called(1);
    });

    test('handles error during initialization', () async {
      const communityId = 'community-123';

      when(mockCommunityRepository.get(communityId))
          .thenThrow(Exception('Failed to load community'));

      final notifier = container.read(communityProvider.notifier);
      await notifier.initialize(communityId);

      final state = container.read(communityProvider);
      expect(state.communityId, communityId);
      expect(state.isLoading, isFalse);
      expect(state.hasError, isTrue);
      expect(state.error, isNotNull);
    });
  });

  group('loadCommunityDetails', () {
    test('loads community without media successfully', () async {
      const communityId = 'community-123';
      final community = GetCommunityResponse(
        id: communityId,
        name: 'Test Community',
        description: 'Test description',
      );

      final notifier = container.read(communityProvider.notifier);
      notifier.state = notifier.state.copyWith(communityId: communityId);

      when(mockCommunityRepository.get(communityId))
          .thenAnswer((_) async => community);

      await notifier.loadCommunityDetails();

      final state = container.read(communityProvider);
      expect(state.communityName, 'Test Community');
      expect(state.communityDescription, 'Test description');
      expect(state.isLoading, isFalse);
      expect(state.error, isNull);
      verifyNever(mockMediaRepository.getMediaUrl(any));
    });

    test('loads community with image media successfully', () async {
      const communityId = 'community-123';
      const mediaId = 'media-456';
      final community = GetCommunityResponse(
        id: communityId,
        name: 'Test Community',
        description: 'Test description',
        mediaIds: [mediaId],
      );

      const mediaUrl = MediaUrl(
        url: 'https://example.com/image.jpg',
        isThumbnail: false,
        mediaId: mediaId,
      );

      final notifier = container.read(communityProvider.notifier);
      notifier.state = notifier.state.copyWith(communityId: communityId);

      when(mockCommunityRepository.get(communityId))
          .thenAnswer((_) async => community);
      when(mockMediaRepository.getMediaUrl(mediaId))
          .thenAnswer((_) async => mediaUrl);

      await notifier.loadCommunityDetails();
      // _loadMediaFromServer is now fire-and-forget; allow microtasks to flush.
      await Future<void>.delayed(Duration.zero);

      final state = container.read(communityProvider);
      expect(state.communityName, 'Test Community');
      expect(state.mediaPath, 'https://example.com/image.jpg');
      expect(state.mediaId, mediaId);
      expect(state.isVideo, isFalse);
      expect(state.isLoading, isFalse);
      verify(mockMediaRepository.getMediaUrl(mediaId)).called(1);
    });

    test('handles media loading failure gracefully', () async {
      const communityId = 'community-123';
      const mediaId = 'media-456';
      final community = GetCommunityResponse(
        id: communityId,
        name: 'Test Community',
        description: 'Test description',
        mediaIds: [mediaId],
      );

      final notifier = container.read(communityProvider.notifier);
      notifier.state = notifier.state.copyWith(communityId: communityId);

      when(mockCommunityRepository.get(communityId))
          .thenAnswer((_) async => community);
      when(mockMediaRepository.getMediaUrl(mediaId))
          .thenThrow(Exception('Media not found'));

      await notifier.loadCommunityDetails();

      // Should not fail the whole operation
      final state = container.read(communityProvider);
      expect(state.communityName, 'Test Community');
      expect(state.isLoading, isFalse);
      expect(state.error, isNull); // No error shown to user
    });

    test('does nothing when communityId is null', () async {
      final notifier = container.read(communityProvider.notifier);
      // communityId is null by default

      await notifier.loadCommunityDetails();

      verifyNever(mockCommunityRepository.get(any));
    });

    test('handles repository error', () async {
      const communityId = 'community-123';

      final notifier = container.read(communityProvider.notifier);
      notifier.state = notifier.state.copyWith(communityId: communityId);

      when(mockCommunityRepository.get(communityId))
          .thenThrow(Exception('Network error'));

      await notifier.loadCommunityDetails();

      final state = container.read(communityProvider);
      expect(state.isLoading, isFalse);
      expect(state.hasError, isTrue);
      expect(state.error, isNotNull);
    });

    test('sets loading state during operation', () async {
      const communityId = 'community-123';
      final community = GetCommunityResponse(
        id: communityId,
        name: 'Test Community',
        description: 'Test description',
      );

      final notifier = container.read(communityProvider.notifier);
      notifier.state = notifier.state.copyWith(
        communityId: communityId,
        isLoading: false,
      );

      when(mockCommunityRepository.get(communityId))
          .thenAnswer((_) async {
        // Verify loading state is true during async operation
        final currentState = container.read(communityProvider);
        expect(currentState.isLoading, isTrue);
        return community;
      });

      await notifier.loadCommunityDetails();

      final state = container.read(communityProvider);
      expect(state.isLoading, isFalse);
    });

    test('clears previous error on successful load', () async {
      const communityId = 'community-123';
      final community = GetCommunityResponse(
        id: communityId,
        name: 'Test Community',
        description: 'Test description',
      );

      final notifier = container.read(communityProvider.notifier);
      notifier.state = notifier.state.copyWith(
        communityId: communityId,
        error: const UserError.generic(fallback: 'Previous error'),
      );

      when(mockCommunityRepository.get(communityId))
          .thenAnswer((_) async => community);

      await notifier.loadCommunityDetails();

      final state = container.read(communityProvider);
      expect(state.error, isNull);
      expect(state.hasError, isFalse);
    });
  });

  group('saveChanges', () {
    test('saves community changes successfully', () async {
      const communityId = 'community-123';
      const updatedName = 'Updated Name';
      const updatedDescription = 'Updated Description';

      final updatedCommunity = GetCommunityResponse(
        id: communityId,
        name: updatedName,
        description: updatedDescription,
      );

      final notifier = container.read(communityProvider.notifier);
      notifier.state = notifier.state.copyWith(
        communityId: communityId,
        communityName: 'Original Name',
        communityDescription: 'Original Description',
        isEditing: true,
        isLoading: false,
      );

      when(mockCommunityRepository.updateCommunity(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaId: anyNamed('mediaId'),
      )).thenAnswer((_) async {});

      when(mockCommunityRepository.get(communityId))
          .thenAnswer((_) async => updatedCommunity);

      await notifier.saveChanges(
        name: updatedName,
        description: updatedDescription,
      );

      final state = container.read(communityProvider);
      expect(state.communityName, updatedName);
      expect(state.communityDescription, updatedDescription);
      expect(state.isEditing, isFalse);
      expect(state.isSaving, isFalse);

      verify(mockCommunityRepository.updateCommunity(
        id: communityId,
        name: updatedName,
        description: updatedDescription,
        mediaId: null,
      )).called(1);
    });

    test('trims whitespace from name and description', () async {
      const communityId = 'community-123';
      const nameWithSpaces = '  Updated Name  ';
      const descriptionWithSpaces = '  Updated Description  ';

      final notifier = container.read(communityProvider.notifier);
      notifier.state = notifier.state.copyWith(
        communityId: communityId,
        isEditing: true,
        isLoading: false,
      );

      when(mockCommunityRepository.updateCommunity(
        id: communityId,
        name: 'Updated Name',
        description: 'Updated Description',
        mediaId: anyNamed('mediaId'),
      )).thenAnswer((_) async {});

      when(mockCommunityRepository.get(communityId))
          .thenAnswer((_) async => GetCommunityResponse(
                id: communityId,
                name: 'Updated Name',
                description: 'Updated Description',
              ));

      await notifier.saveChanges(
        name: nameWithSpaces,
        description: descriptionWithSpaces,
      );

      verify(mockCommunityRepository.updateCommunity(
        id: communityId,
        name: 'Updated Name',
        description: 'Updated Description',
        mediaId: null,
      )).called(1);
    });

    test('includes mediaId in update when present', () async {
      const communityId = 'community-123';
      const mediaId = 'media-456';
      const updatedName = 'Updated Name';
      const updatedDescription = 'Updated Description';

      final notifier = container.read(communityProvider.notifier);
      notifier.state = notifier.state.copyWith(
        communityId: communityId,
        mediaId: mediaId,
        isEditing: true,
        isLoading: false,
      );

      when(mockCommunityRepository.updateCommunity(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaId: anyNamed('mediaId'),
      )).thenAnswer((_) async {});

      when(mockCommunityRepository.get(communityId))
          .thenAnswer((_) async => GetCommunityResponse(
                id: communityId,
                name: updatedName,
                description: updatedDescription,
                mediaIds: [mediaId],
              ));

      await notifier.saveChanges(
        name: updatedName,
        description: updatedDescription,
      );

      verify(mockCommunityRepository.updateCommunity(
        id: communityId,
        name: updatedName,
        description: updatedDescription,
        mediaId: mediaId,
      )).called(1);
    });

    test('sets isSaving state during save', () async {
      const communityId = 'community-123';

      final notifier = container.read(communityProvider.notifier);
      notifier.state = notifier.state.copyWith(
        communityId: communityId,
        isEditing: true,
        isLoading: false,
      );

      when(mockCommunityService.updateCommunity(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      )).thenAnswer((_) async {
        // Verify saving state is true during async operation
        final currentState = container.read(communityProvider);
        expect(currentState.isSaving, isTrue);
      });

      when(mockCommunityRepository.invalidate(communityId))
          .thenAnswer((_) async {});

      when(mockCommunityRepository.get(communityId))
          .thenAnswer((_) async => GetCommunityResponse(
                id: communityId,
                name: 'Name',
                description: 'Description',
              ));

      await notifier.saveChanges(name: 'Name', description: 'Description');

      final state = container.read(communityProvider);
      expect(state.isSaving, isFalse);
    });

    test('rethrows exception on save failure', () async {
      const communityId = 'community-123';

      final notifier = container.read(communityProvider.notifier);
      notifier.state = notifier.state.copyWith(
        communityId: communityId,
        isEditing: true,
        isLoading: false,
      );

      when(mockCommunityRepository.updateCommunity(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      )).thenThrow(Exception('Save failed'));

      await expectLater(
        notifier.saveChanges(name: 'Name', description: 'Description'),
        throwsA(isA<Exception>()),
      );
    });

    test('sets isSaving to false on error', () async {
      const communityId = 'community-123';

      final notifier = container.read(communityProvider.notifier);
      notifier.state = notifier.state.copyWith(
        communityId: communityId,
        isEditing: true,
        isLoading: false,
      );

      when(mockCommunityRepository.updateCommunity(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      )).thenThrow(Exception('Save failed'));

      try {
        await notifier.saveChanges(name: 'Name', description: 'Description');
      } catch (_) {
        // Expected
      }

      final state = container.read(communityProvider);
      expect(state.isSaving, isFalse);
    });
  });

  group('refresh', () {
    test('reloads community details', () async {
      const communityId = 'community-123';
      final community = GetCommunityResponse(
        id: communityId,
        name: 'Refreshed Community',
        description: 'Refreshed description',
      );

      final notifier = container.read(communityProvider.notifier);
      notifier.state = notifier.state.copyWith(
        communityId: communityId,
        isLoading: false,
      );

      when(mockCommunityRepository.get(communityId))
          .thenAnswer((_) async => community);

      await notifier.refresh();

      final state = container.read(communityProvider);
      expect(state.communityName, 'Refreshed Community');
      expect(state.communityDescription, 'Refreshed description');
      verify(mockCommunityRepository.get(communityId)).called(1);
    });
  });
}
