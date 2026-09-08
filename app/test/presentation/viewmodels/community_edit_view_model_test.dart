import 'package:cross_file/cross_file.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/presentation/viewmodels/community_edit_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'community_edit_view_model_test.mocks.dart';

@GenerateMocks([CommunityRepository, MediaRepository])
void main() {
  late MockCommunityRepository mockCommunityRepo;
  late MockMediaRepository mockMediaRepo;
  late ProviderContainer container;

  const communityId = 'community1';

  setUp(() {
    mockCommunityRepo = MockCommunityRepository();
    mockMediaRepo = MockMediaRepository();
    container = ProviderContainer(
      overrides: [
        communityRepositoryProvider.overrideWithValue(mockCommunityRepo),
        mediaRepositoryProvider.overrideWithValue(mockMediaRepo),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockCommunityRepo);
    reset(mockMediaRepo);
  });

  group('startEditing / cancelEditing', () {
    test('startEditing sets isEditing = true', () {
      final notifier =
          container.read(communityEditProvider(communityId).notifier);
      notifier.startEditing();

      final state = container.read(communityEditProvider(communityId));
      expect(state.isEditing, isTrue);
      expect(state.error, isNull);
    });

    test('cancelEditing clears editing state and newMediaId', () {
      final notifier =
          container.read(communityEditProvider(communityId).notifier);
      notifier.startEditing();
      notifier.cancelEditing();

      final state = container.read(communityEditProvider(communityId));
      expect(state.isEditing, isFalse);
      expect(state.newMediaId, isNull);
      expect(state.error, isNull);
    });
  });

  group('uploadMedia', () {
    test('happy path: sets newMediaId, clears isUploadingMedia', () async {
      when(mockMediaRepo.addMedia(
        file: anyNamed('file'),
        description: anyNamed('description'),
      )).thenAnswer((_) async => 'media-123');

      final notifier =
          container.read(communityEditProvider(communityId).notifier);
      await notifier.uploadMedia(XFile('/path/to/image.jpg'));

      final state = container.read(communityEditProvider(communityId));
      expect(state.newMediaId, 'media-123');
      expect(state.isUploadingMedia, isFalse);
    });

    test('error path: sets errorMessage, rethrows', () async {
      when(mockMediaRepo.addMedia(
        file: anyNamed('file'),
        description: anyNamed('description'),
      )).thenThrow(Exception('Upload failed'));

      final notifier =
          container.read(communityEditProvider(communityId).notifier);

      await expectLater(
        notifier.uploadMedia(XFile('/path/to/image.jpg')),
        throwsA(isA<Exception>()),
      );

      final state = container.read(communityEditProvider(communityId));
      expect(state.isUploadingMedia, isFalse);
      expect(state.error, isNotNull);
    });
  });

  group('saveCommunity', () {
    test('happy path: returns true, clears isEditing', () async {
      when(mockCommunityRepo.updateCommunity(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaId: anyNamed('mediaId'),
      )).thenAnswer((_) async {});

      final notifier =
          container.read(communityEditProvider(communityId).notifier);
      notifier.startEditing();
      final result = await notifier.saveCommunity(
        communityId: communityId,
        name: 'New Name',
        description: 'New Description',
      );

      expect(result, isTrue);
      final state = container.read(communityEditProvider(communityId));
      expect(state.isSaving, isFalse);
      expect(state.isEditing, isFalse);
      expect(state.newMediaId, isNull);
    });

    test('error path: returns false, sets errorMessage', () async {
      when(mockCommunityRepo.updateCommunity(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaId: anyNamed('mediaId'),
      )).thenThrow(Exception('Save failed'));

      final notifier =
          container.read(communityEditProvider(communityId).notifier);
      final result = await notifier.saveCommunity(
        communityId: communityId,
        name: 'New Name',
      );

      expect(result, isFalse);
      final state = container.read(communityEditProvider(communityId));
      expect(state.isSaving, isFalse);
      expect(state.error, isNotNull);
    });
  });

  group('clearError', () {
    test('clears errorMessage', () async {
      when(mockCommunityRepo.updateCommunity(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaId: anyNamed('mediaId'),
      )).thenThrow(Exception('Error'));

      final notifier =
          container.read(communityEditProvider(communityId).notifier);
      await notifier.saveCommunity(communityId: communityId);
      notifier.clearError();

      final state = container.read(communityEditProvider(communityId));
      expect(state.error, isNull);
    });
  });

  group('loadAllMedia', () {
    test('empty list clears allMediaItems', () async {
      final notifier =
          container.read(communityEditProvider(communityId).notifier);
      await notifier.loadAllMedia([]);

      final state = container.read(communityEditProvider(communityId));
      expect(state.allMediaItems, isEmpty);
    });

    test('populates allMediaItems from media IDs', () async {
      final media1 = GetMediaResponse(
        contentType: 'image/jpeg',
        url: 'https://example.com/image1.jpg',
        thumbnailUrl: 'https://example.com/thumb1.jpg',
      );
      final media2 = GetMediaResponse(
        contentType: 'image/png',
        url: 'https://example.com/image2.png',
        thumbnailUrl: '',
      );
      when(mockMediaRepo.get('m1')).thenAnswer((_) async => media1);
      when(mockMediaRepo.get('m2')).thenAnswer((_) async => media2);

      final notifier =
          container.read(communityEditProvider(communityId).notifier);
      await notifier.loadAllMedia(['m1', 'm2']);

      final state = container.read(communityEditProvider(communityId));
      expect(state.allMediaItems, hasLength(2));
      expect(state.allMediaItems[0].id, 'm1');
      expect(state.allMediaItems[1].id, 'm2');
    });

    test('continues loading other items when one fails', () async {
      final media2 = GetMediaResponse(
        contentType: 'image/png',
        url: 'https://example.com/image2.png',
        thumbnailUrl: '',
      );
      when(mockMediaRepo.get('m1')).thenThrow(Exception('Not found'));
      when(mockMediaRepo.get('m2')).thenAnswer((_) async => media2);

      final notifier =
          container.read(communityEditProvider(communityId).notifier);
      await notifier.loadAllMedia(['m1', 'm2']);

      final state = container.read(communityEditProvider(communityId));
      // m1 failed but m2 loaded; partial success
      expect(state.allMediaItems, hasLength(1));
      expect(state.allMediaItems[0].id, 'm2');
    });
  });

  group('deleteMedia', () {
    test('updates community without the deleted ID and reloads', () async {
      when(mockCommunityRepo.updateCommunity(
        id: anyNamed('id'),
        mediaIds: anyNamed('mediaIds'),
      )).thenAnswer((_) async {});
      when(mockMediaRepo.get(any)).thenAnswer((_) async => GetMediaResponse(
            contentType: 'image/jpeg',
            url: 'https://example.com/img.jpg',
            thumbnailUrl: '',
          ));

      final notifier =
          container.read(communityEditProvider(communityId).notifier);
      await notifier.deleteMedia('m1', ['m1', 'm2', 'm3']);

      verify(mockCommunityRepo.updateCommunity(
        id: communityId,
        mediaIds: ['m2', 'm3'],
      )).called(1);
    });

    test('error path: sets errorMessage and rethrows', () async {
      when(mockCommunityRepo.updateCommunity(
        id: anyNamed('id'),
        mediaIds: anyNamed('mediaIds'),
      )).thenThrow(Exception('Delete failed'));

      final notifier =
          container.read(communityEditProvider(communityId).notifier);

      await expectLater(
        notifier.deleteMedia('m1', ['m1', 'm2']),
        throwsA(isA<Exception>()),
      );

      final state = container.read(communityEditProvider(communityId));
      expect(state.error, isNotNull);
    });
  });

  group('reorderMedia', () {
    test('updates community with new order and reloads', () async {
      final newOrder = ['m3', 'm1', 'm2'];
      when(mockCommunityRepo.updateCommunity(
        id: anyNamed('id'),
        mediaIds: anyNamed('mediaIds'),
      )).thenAnswer((_) async {});
      when(mockMediaRepo.get(any)).thenAnswer((_) async => GetMediaResponse(
            contentType: 'image/jpeg',
            url: 'https://example.com/img.jpg',
            thumbnailUrl: '',
          ));

      final notifier =
          container.read(communityEditProvider(communityId).notifier);
      await notifier.reorderMedia(newOrder);

      verify(mockCommunityRepo.updateCommunity(
        id: communityId,
        mediaIds: newOrder,
      )).called(1);
    });

    test('error path: sets errorMessage and rethrows', () async {
      when(mockCommunityRepo.updateCommunity(
        id: anyNamed('id'),
        mediaIds: anyNamed('mediaIds'),
      )).thenThrow(Exception('Reorder failed'));

      final notifier =
          container.read(communityEditProvider(communityId).notifier);

      await expectLater(
        notifier.reorderMedia(['m1', 'm2']),
        throwsA(isA<Exception>()),
      );

      final state = container.read(communityEditProvider(communityId));
      expect(state.error, isNotNull);
    });
  });

  group('disposal safety', () {
    test('container.dispose() mid-flight completes without throwing', () async {
      when(mockMediaRepo.addMedia(
        file: anyNamed('file'),
        description: anyNamed('description'),
      )).thenAnswer((_) async {
        await Future<void>.delayed(const Duration(milliseconds: 10));
        return 'media-abc';
      });

      final notifier =
          container.read(communityEditProvider(communityId).notifier);
      final future = notifier.uploadMedia(XFile('/path/to/image.jpg'));
      container.dispose();

      await expectLater(future, completes);
    });
  });
}
