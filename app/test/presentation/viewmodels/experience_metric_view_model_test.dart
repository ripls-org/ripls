import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/data/repositories/story_repository.dart';
import 'package:ripls/presentation/viewmodels/experience_metric_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'experience_metric_view_model_test.mocks.dart';

@GenerateMocks([ExperienceRepository, StoryRepository])
void main() {
  late MockExperienceRepository mockExperienceRepo;
  late MockStoryRepository mockStoryRepo;
  late ProviderContainer container;

  const params = ExperienceMetricParams(experienceId: 'exp1');

  setUp(() {
    mockExperienceRepo = MockExperienceRepository();
    mockStoryRepo = MockStoryRepository();

    when(mockExperienceRepo.getExperienceDetails(
      any,
      communityId: anyNamed('communityId'),
    )).thenAnswer((_) async => GetExperienceResponse());

    when(mockExperienceRepo.getPeople(
      any,
      communityId: anyNamed('communityId'),
    )).thenAnswer((_) async => GetExperiencePeopleResponse());

    when(mockExperienceRepo.getStats(
      any,
      communityId: anyNamed('communityId'),
    )).thenAnswer((_) async => GetExperienceStatsResponse());

    when(mockStoryRepo.listByItem(any, limit: anyNamed('limit')))
        .thenAnswer((_) async => []);

    container = ProviderContainer(
      overrides: [
        experienceRepositoryProvider.overrideWithValue(mockExperienceRepo),
        storyRepositoryProvider.overrideWithValue(mockStoryRepo),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockExperienceRepo);
    reset(mockStoryRepo);
  });

  group('build', () {
    test('loads experience metric data', () async {
      final state =
          await container.read(experienceImpactMetricsProvider(params).future);
      expect(state.experienceId, 'exp1');
      expect(state.people, isEmpty);
      expect(state.stories, isEmpty);
    });

    test('error state on failure', () async {
      when(mockExperienceRepo.getExperienceDetails(
        any,
        communityId: anyNamed('communityId'),
      )).thenThrow(Exception('Load failed'));

      final sub = container.listen(
        experienceImpactMetricsProvider(params),
        (_, _) {},
      );
      addTearDown(sub.close);
      await Future<void>.delayed(Duration.zero);
      expect(
        container.read(experienceImpactMetricsProvider(params)).hasError,
        isTrue,
      );
    });
  });

  group('refresh', () {
    test('invalidates caches and reloads', () async {
      when(mockExperienceRepo.invalidatePeople(
        any,
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async {});
      when(mockExperienceRepo.invalidateStats(
        any,
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async {});
      when(mockStoryRepo.invalidateItem(any)).thenAnswer((_) async {});

      await container.read(experienceImpactMetricsProvider(params).future);
      final notifier =
          container.read(experienceImpactMetricsProvider(params).notifier);
      await notifier.refresh();

      verify(mockExperienceRepo.invalidatePeople(
        'exp1',
        communityId: anyNamed('communityId'),
      )).called(1);
      verify(mockExperienceRepo.invalidateStats(
        'exp1',
        communityId: anyNamed('communityId'),
      )).called(1);
      verify(mockStoryRepo.invalidateItem('exp1')).called(1);
    });
  });
}
