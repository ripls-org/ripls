import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/presentation/viewmodels/gen_community_view_model.dart';
import 'package:ripls/presentation/viewmodels/home_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/providers.dart';

import 'gen_community_view_model_test.mocks.dart';

class _FakeAuthStateNotifier extends AuthStateNotifier {
  @override
  AuthStateData build() => const AuthStateData(isLoading: false);
}

class _FakeHomeNotifier extends HomeNotifier {
  @override
  HomeState build() => const HomeState();
}

class _FakeCommunitiesNotifier extends CommunitiesNotifier {
  @override
  CommunitiesState build() => const CommunitiesState();
}

@GenerateMocks([CommunityRepository, MediaRepository, FeedRepository])
void main() {
  late MockCommunityRepository mockCommunityRepo;
  late MockMediaRepository mockMediaRepo;
  late MockFeedRepository mockFeedRepo;
  late ProviderContainer container;

  setUp(() {
    mockCommunityRepo = MockCommunityRepository();
    mockMediaRepo = MockMediaRepository();
    mockFeedRepo = MockFeedRepository();

    container = ProviderContainer(
      overrides: [
        communityRepositoryProvider.overrideWithValue(mockCommunityRepo),
        mediaRepositoryProvider.overrideWithValue(mockMediaRepo),
        feedRepositoryProvider.overrideWithValue(mockFeedRepo),
        authStateProvider.overrideWith(_FakeAuthStateNotifier.new),
        homeProvider.overrideWith(_FakeHomeNotifier.new),
        communitiesProvider
            .overrideWith(_FakeCommunitiesNotifier.new),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockCommunityRepo);
    reset(mockMediaRepo);
    reset(mockFeedRepo);
  });

  group('setPreviewData / setName / setDescription / setMediaIds', () {
    test('setPreviewData initializes state', () {
      final notifier = container.read(genCommunityProvider.notifier);
      notifier.setPreviewData(
        name: 'My Community',
        description: 'A great place',
        mediaIds: ['m1', 'm2'],
      );

      final state = container.read(genCommunityProvider);
      expect(state.name, 'My Community');
      expect(state.description, 'A great place');
      expect(state.mediaIds, ['m1', 'm2']);
      expect(state.error, isNull);
    });

    test('setName updates name', () {
      final notifier = container.read(genCommunityProvider.notifier);
      notifier.setName('Updated Name');
      expect(container.read(genCommunityProvider).name, 'Updated Name');
    });

    test('setDescription updates description', () {
      final notifier = container.read(genCommunityProvider.notifier);
      notifier.setDescription('Updated Description');
      expect(container.read(genCommunityProvider).description,
          'Updated Description');
    });

    test('setMediaIds updates mediaIds', () {
      final notifier = container.read(genCommunityProvider.notifier);
      notifier.setMediaIds(['m3', 'm4']);
      expect(container.read(genCommunityProvider).mediaIds, ['m3', 'm4']);
    });
  });

  group('createCommunity', () {
    test('returns null and sets errorMessage when name is empty', () async {
      final notifier = container.read(genCommunityProvider.notifier);
      notifier.setPreviewData(name: '', description: 'Desc');
      final result = await notifier.createCommunity();

      expect(result, isNull);
      final state = container.read(genCommunityProvider);
      expect(state.error, isNotNull);
    });

    test('succeeds when description is empty (description is optional)',
        () async {
      when(mockCommunityRepo.createCommunity(
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      )).thenAnswer((_) async => 'new-community-id');
      when(mockFeedRepo.invalidateFeed()).thenAnswer((_) async {});

      final notifier = container.read(genCommunityProvider.notifier);
      notifier.setPreviewData(name: 'Name', description: '');
      final result = await notifier.createCommunity();

      expect(result, 'new-community-id');
      final state = container.read(genCommunityProvider);
      expect(state.createdCommunityId, 'new-community-id');
      expect(state.error, isNull);
    });

    test('happy path: creates community and sets createdCommunityId', () async {
      when(mockCommunityRepo.createCommunity(
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      )).thenAnswer((_) async => 'new-community-id');
      when(mockFeedRepo.invalidateFeed()).thenAnswer((_) async {});

      final notifier = container.read(genCommunityProvider.notifier);
      notifier.setPreviewData(
        name: 'My Community',
        description: 'A great place',
      );
      final result = await notifier.createCommunity();

      expect(result, 'new-community-id');
      final state = container.read(genCommunityProvider);
      expect(state.createdCommunityId, 'new-community-id');
      expect(state.isCreating, isFalse);
      expect(state.isCompleted, isTrue);
    });

    test('error path: sets errorMessage and returns null', () async {
      when(mockCommunityRepo.createCommunity(
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      )).thenThrow(Exception('Server error'));

      final notifier = container.read(genCommunityProvider.notifier);
      notifier.setPreviewData(
        name: 'My Community',
        description: 'A great place',
      );
      final result = await notifier.createCommunity();

      expect(result, isNull);
      final state = container.read(genCommunityProvider);
      expect(state.isCreating, isFalse);
      expect(state.error, isNotNull);
    });
  });

  group('createCommunity promote mode', () {
    test('promotes existing community via updateCommunity, not createCommunity',
        () async {
      when(mockCommunityRepo.updateCommunity(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      )).thenAnswer((_) async {});

      final notifier = container.read(genCommunityProvider.notifier);
      notifier.setExistingCommunityId('existing-id');
      notifier.setPreviewData(
        name: 'Trivia Crew',
        description: 'Wednesday night regulars',
        mediaIds: ['m1'],
      );

      final result = await notifier.createCommunity();

      expect(result, 'existing-id');
      final state = container.read(genCommunityProvider);
      expect(state.createdCommunityId, 'existing-id');
      expect(state.isCreating, isFalse);

      verify(mockCommunityRepo.updateCommunity(
        id: 'existing-id',
        name: 'Trivia Crew',
        description: 'Wednesday night regulars',
        mediaIds: ['m1'],
      )).called(1);
      verifyNever(mockCommunityRepo.createCommunity(
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      ));
    });

    test('reset clears existingCommunityId so the next submit creates',
        () async {
      when(mockCommunityRepo.createCommunity(
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      )).thenAnswer((_) async => 'fresh-id');
      when(mockFeedRepo.invalidateFeed()).thenAnswer((_) async {});

      final notifier = container.read(genCommunityProvider.notifier);
      notifier.setExistingCommunityId('existing-id');
      notifier.reset();
      notifier.setPreviewData(name: 'New', description: 'Brand new');

      final result = await notifier.createCommunity();

      expect(result, 'fresh-id');
      verify(mockCommunityRepo.createCommunity(
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      )).called(1);
      verifyNever(mockCommunityRepo.updateCommunity(
        id: anyNamed('id'),
        name: anyNamed('name'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
      ));
    });
  });

  group('reset', () {
    test('clears state to initial', () {
      final notifier = container.read(genCommunityProvider.notifier);
      notifier.setPreviewData(name: 'Name', description: 'Desc');
      notifier.reset();

      final state = container.read(genCommunityProvider);
      expect(state.name, isNull);
      expect(state.description, isNull);
      expect(state.isCreating, isFalse);
      expect(state.error, isNull);
    });
  });

  group('streaming methods', () {
    test('beginStreaming clears fields and sets isStreaming=true', () {
      final notifier = container.read(genCommunityProvider.notifier);
      notifier.setPreviewData(
          name: 'Old', description: 'Old desc', mediaIds: ['m1']);
      notifier.beginStreaming();

      final state = container.read(genCommunityProvider);
      expect(state.name, isNull);
      expect(state.description, isNull);
      expect(state.mediaIds, isNull);
      expect(state.error, isNull);
      expect(state.isStreaming, isTrue);
    });

    test('applyStreamingMediaReady writes mediaIds', () {
      final notifier = container.read(genCommunityProvider.notifier);
      notifier.applyStreamingMediaReady(['m1', 'm2']);

      expect(container.read(genCommunityProvider).mediaIds, ['m1', 'm2']);
    });

    test('applyStreamingMediaReady with empty list is a no-op', () {
      final notifier = container.read(genCommunityProvider.notifier);
      notifier.applyStreamingMediaReady(['m1']);
      notifier.applyStreamingMediaReady([]);

      expect(container.read(genCommunityProvider).mediaIds, ['m1']);
    });

    test('finalizeStreaming sets mediaIds and clears isStreaming', () {
      final notifier = container.read(genCommunityProvider.notifier);
      notifier.beginStreaming();

      notifier.finalizeStreaming(mediaIds: ['m1']);

      final state = container.read(genCommunityProvider);
      expect(state.mediaIds, ['m1']);
      expect(state.isStreaming, isFalse);
      expect(state.error, isNull);
    });

    test('finalizeStreaming with empty mediaIds preserves existing mediaIds',
        () {
      final notifier = container.read(genCommunityProvider.notifier);
      notifier.applyStreamingMediaReady(['existing']);

      notifier.finalizeStreaming(mediaIds: []);

      expect(container.read(genCommunityProvider).mediaIds, ['existing']);
    });

    test('failStreaming sets non-null error and clears isStreaming', () {
      final notifier = container.read(genCommunityProvider.notifier);
      notifier.beginStreaming();
      notifier.failStreaming('AI service unavailable');

      final state = container.read(genCommunityProvider);
      expect(state.error, isNotNull);
      expect(state.isStreaming, isFalse);
    });

    test('fetchBackground streams a media_ready into mediaIds', () async {
      when(mockCommunityRepo.streamGenCommunity(
        prompt: anyNamed('prompt'),
        region: anyNamed('region'),
      )).thenAnswer(
        (_) => Stream.value(
          StreamGenCommunityResponse(
            mediaReady: MediaReady(mediaIds: ['bg1']),
          ),
        ),
      );

      final notifier = container.read(genCommunityProvider.notifier);
      notifier.setName('Tool Library');
      await notifier.fetchBackground();
      // Flush the single-subscription stream so its media_ready event and
      // the controller's onDone callback run.
      await Future<void>.delayed(Duration.zero);

      final state = container.read(genCommunityProvider);
      expect(state.mediaIds, ['bg1']);
      expect(state.isStreaming, isFalse);
    });

    test('fetchBackground is a no-op when the name is empty', () async {
      final notifier = container.read(genCommunityProvider.notifier);
      await notifier.fetchBackground();

      verifyNever(mockCommunityRepo.streamGenCommunity(
        prompt: anyNamed('prompt'),
        region: anyNamed('region'),
      ));
    });
  });
}
