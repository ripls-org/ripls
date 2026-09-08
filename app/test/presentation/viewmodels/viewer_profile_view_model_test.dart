import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/data/repositories/profile_repository.dart';
import 'package:ripls/presentation/viewmodels/viewer_profile_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/impact_providers.dart';
import 'package:ripls/services/providers/media_providers.dart';
import 'package:ripls/services/providers/profile_providers.dart';

import 'viewer_profile_view_model_test.mocks.dart';

@GenerateMocks([ProfileRepository, ImpactMetricsRepository, MediaRepository])
void main() {
  late MockProfileRepository mockProfileRepo;
  late MockImpactMetricsRepository mockImpactRepo;
  late MockMediaRepository mockMediaRepo;
  late ProviderContainer container;

  const targetId = 'user-target-1';

  GetUserProfileForViewerResponse profileResponse({
    bool withMedia = false,
    List<String> knownFor = const <String>[],
  }) {
    final r = GetUserProfileForViewerResponse(
      targetUserId: targetId,
      targetName: 'Thomas Escobar',
      knownFor: knownFor,
    );
    if (withMedia) {
      r.targetMediaId = 'media-1';
    }
    return r;
  }

  GetUserImpactMetricsResponse impactResponse({
    double timeMinutes = 18000,
    double costUsd = 955,
    double carbonGrams = 57000,
  }) {
    return GetUserImpactMetricsResponse(
      metrics: UserImpactMetrics(
        timeBankedMinutes: Estimate(mean: timeMinutes),
        costSavingsUsd: Estimate(mean: costUsd),
        carbonSavingsGrams: Estimate(mean: carbonGrams),
      ),
    );
  }

  ProviderContainer buildContainer({User? authUser}) {
    return ProviderContainer(
      overrides: [
        profileRepositoryProvider.overrideWithValue(mockProfileRepo),
        impactMetricsRepositoryProvider.overrideWithValue(mockImpactRepo),
        mediaRepositoryProvider.overrideWithValue(mockMediaRepo),
        // Stub auth so the viewmodel can read authStateProvider without
        // triggering AuthStateNotifier.build() (which touches
        // WidgetsBinding.instance). Default to an unrelated signed-in
        // user so isSelfView resolves to false.
        authStateProvider.overrideWith(() => _StubAuthStateNotifier(authUser)),
      ],
    );
  }

  setUp(() {
    mockProfileRepo = MockProfileRepository();
    mockImpactRepo = MockImpactMetricsRepository();
    mockMediaRepo = MockMediaRepository();

    container = buildContainer(
      authUser: User(id: 'unrelated-viewer', name: 'Unrelated Viewer'),
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockProfileRepo);
    reset(mockImpactRepo);
    reset(mockMediaRepo);
  });

  group('ViewerProfileNotifier.build', () {
    test('loads profile + impact metrics in parallel', () async {
      when(mockProfileRepo.getProfile(targetId))
          .thenAnswer((_) async => profileResponse());
      when(mockImpactRepo.getUserMetrics(targetId))
          .thenAnswer((_) async => impactResponse());

      container.listen(viewerProfileProvider(targetId), (_, _) {});
      final state =
          await container.read(viewerProfileProvider(targetId).future);

      expect(state.targetName, 'Thomas Escobar');
      expect(state.impactMetrics, isNotNull);
      expect(state.impactMetrics!.timeBankedMinutes.mean, 18000);
      verify(mockProfileRepo.getProfile(targetId)).called(1);
      verify(mockImpactRepo.getUserMetrics(targetId)).called(1);
    });

    test('treats impact-fetch failure as non-fatal', () async {
      when(mockProfileRepo.getProfile(targetId))
          .thenAnswer((_) async => profileResponse());
      when(mockImpactRepo.getUserMetrics(targetId))
          .thenAnswer((_) async => throw Exception('impact backend down'));

      container.listen(viewerProfileProvider(targetId), (_, _) {});
      final state =
          await container.read(viewerProfileProvider(targetId).future);

      // Profile data survives.
      expect(state.targetName, 'Thomas Escobar');
      // Impact field is null; the impact-rows widget hides itself.
      expect(state.impactMetrics, isNull);
    });

    test('surfaces known_for tags onto state', () async {
      when(mockProfileRepo.getProfile(targetId)).thenAnswer((_) async =>
          profileResponse(knownFor: const [
            'Power Tools lender',
            'Camping lender',
            'Cooking lender',
          ]));
      when(mockImpactRepo.getUserMetrics(targetId))
          .thenAnswer((_) async => impactResponse());

      container.listen(viewerProfileProvider(targetId), (_, _) {});
      final state =
          await container.read(viewerProfileProvider(targetId).future);

      expect(state.knownFor, hasLength(3));
      expect(state.knownFor.first, 'Power Tools lender');
    });

    test('resolves media URL when target has media id', () async {
      when(mockProfileRepo.getProfile(targetId))
          .thenAnswer((_) async => profileResponse(withMedia: true));
      when(mockImpactRepo.getUserMetrics(targetId))
          .thenAnswer((_) async => impactResponse());
      when(mockMediaRepo.getFullMediaUrl('media-1')).thenAnswer(
        (_) async => const MediaUrl(
          url: 'https://media.example/full/media-1.jpg',
          isThumbnail: false,
          mediaId: 'media-1',
        ),
      );

      container.listen(viewerProfileProvider(targetId), (_, _) {});
      final state =
          await container.read(viewerProfileProvider(targetId).future);

      expect(state.targetMediaId, 'media-1');
      expect(state.targetMediaUrl, 'https://media.example/full/media-1.jpg');
    });

    test('handles disposal during in-flight load without throwing',
        () async {
      when(mockProfileRepo.getProfile(targetId))
          .thenAnswer((_) async => profileResponse());
      when(mockImpactRepo.getUserMetrics(targetId))
          .thenAnswer((_) async => impactResponse());

      final future = container.read(viewerProfileProvider(targetId).future);
      container.dispose();

      // The autodispose provider may complete normally or throw the
      // "ref disposed" error — both are acceptable; the test asserts
      // we do not crash before reaching here.
      await expectLater(
        future.then<void>((_) {}, onError: (Object _) {}),
        completes,
      );
    });
  });

  group('self-view (#1996)', () {
    test('isSelfView=true when authenticated user matches target', () async {
      when(mockProfileRepo.getProfile(targetId))
          .thenAnswer((_) async => profileResponse());
      when(mockImpactRepo.getUserMetrics(targetId))
          .thenAnswer((_) async => impactResponse());

      container.dispose();
      container = buildContainer(authUser: User(id: targetId, name: 'Me'));

      container.listen(viewerProfileProvider(targetId), (_, _) {});
      final state =
          await container.read(viewerProfileProvider(targetId).future);

      expect(state.isSelfView, isTrue);
    });

    test('isSelfView=false when viewer differs from target', () async {
      when(mockProfileRepo.getProfile(targetId))
          .thenAnswer((_) async => profileResponse());
      when(mockImpactRepo.getUserMetrics(targetId))
          .thenAnswer((_) async => impactResponse());

      container.listen(viewerProfileProvider(targetId), (_, _) {});
      final state =
          await container.read(viewerProfileProvider(targetId).future);

      expect(state.isSelfView, isFalse);
    });

    test('isSelfView=false when no authenticated user', () async {
      when(mockProfileRepo.getProfile(targetId))
          .thenAnswer((_) async => profileResponse());
      when(mockImpactRepo.getUserMetrics(targetId))
          .thenAnswer((_) async => impactResponse());

      container.dispose();
      container = buildContainer(authUser: null);

      container.listen(viewerProfileProvider(targetId), (_, _) {});
      final state =
          await container.read(viewerProfileProvider(targetId).future);

      expect(state.isSelfView, isFalse);
    });
  });

  group('refresh', () {
    test('invalidates both repositories and reloads', () async {
      when(mockProfileRepo.getProfile(targetId))
          .thenAnswer((_) async => profileResponse());
      when(mockImpactRepo.getUserMetrics(targetId))
          .thenAnswer((_) async => impactResponse());

      container.listen(viewerProfileProvider(targetId), (_, _) {});
      await container.read(viewerProfileProvider(targetId).future);

      when(mockProfileRepo.invalidate(targetId)).thenAnswer((_) => Future.value());
      when(mockImpactRepo.invalidateUserMetrics(targetId))
          .thenAnswer((_) => Future.value());

      await container.read(viewerProfileProvider(targetId).notifier).refresh();

      verify(mockProfileRepo.invalidate(targetId)).called(1);
      verify(mockImpactRepo.invalidateUserMetrics(targetId)).called(1);
      verify(mockProfileRepo.getProfile(targetId)).called(2);
      verify(mockImpactRepo.getUserMetrics(targetId)).called(2);
    });
  });
}

class _StubAuthStateNotifier extends AuthStateNotifier {
  _StubAuthStateNotifier(this._user);

  final User? _user;

  @override
  AuthStateData build() {
    return AuthStateData(
      user: _user,
      accessToken: _user != null ? 'stub-token' : null,
      isLoading: false,
    );
  }
}
