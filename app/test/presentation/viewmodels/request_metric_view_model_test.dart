import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show Request;
import 'package:ripls/data/gen/ripls/api/request_service.pb.dart';
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/data/repositories/story_repository.dart';
import 'package:ripls/presentation/viewmodels/request_metric_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'request_metric_view_model_test.mocks.dart';

@GenerateMocks([RequestRepository, StoryRepository])
void main() {
  late MockRequestRepository mockRequestRepo;
  late MockStoryRepository mockStoryRepo;
  late ProviderContainer container;

  const params = RequestMetricParams(requestId: 'req1');

  setUp(() {
    mockRequestRepo = MockRequestRepository();
    mockStoryRepo = MockStoryRepository();

    when(mockRequestRepo.getRequest(requestId: anyNamed('requestId')))
        .thenAnswer((_) async => Request());

    when(mockRequestRepo.getPeople(
      any,
      communityId: anyNamed('communityId'),
    )).thenAnswer((_) async => GetRequestPeopleResponse());

    when(mockRequestRepo.getStats(
      any,
      communityId: anyNamed('communityId'),
    )).thenAnswer((_) async => GetRequestStatsResponse());

    when(mockStoryRepo.listByItem(any, limit: anyNamed('limit')))
        .thenAnswer((_) async => []);

    container = ProviderContainer(
      overrides: [
        requestRepositoryProvider.overrideWithValue(mockRequestRepo),
        storyRepositoryProvider.overrideWithValue(mockStoryRepo),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockRequestRepo);
    reset(mockStoryRepo);
  });

  group('build', () {
    test('loads request metric data', () async {
      final state =
          await container.read(requestImpactMetricsProvider(params).future);
      expect(state.requestId, 'req1');
      expect(state.people, isEmpty);
      expect(state.stories, isEmpty);
    });

    test('error state on failure', () async {
      when(mockRequestRepo.getRequest(requestId: anyNamed('requestId')))
          .thenThrow(Exception('Load failed'));

      final sub = container.listen(
        requestImpactMetricsProvider(params),
        (_, _) {},
      );
      addTearDown(sub.close);
      await Future<void>.delayed(Duration.zero);
      expect(
        container.read(requestImpactMetricsProvider(params)).hasError,
        isTrue,
      );
    });
  });

  group('refresh', () {
    test('invalidates caches and reloads', () async {
      when(mockRequestRepo.invalidatePeople(
        any,
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async {});
      when(mockRequestRepo.invalidateStats(
        any,
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async {});
      when(mockStoryRepo.invalidateItem(any)).thenAnswer((_) async {});

      await container.read(requestImpactMetricsProvider(params).future);
      final notifier =
          container.read(requestImpactMetricsProvider(params).notifier);
      await notifier.refresh();

      verify(mockRequestRepo.invalidatePeople(
        'req1',
        communityId: anyNamed('communityId'),
      )).called(1);
      verify(mockRequestRepo.invalidateStats(
        'req1',
        communityId: anyNamed('communityId'),
      )).called(1);
      verify(mockStoryRepo.invalidateItem('req1')).called(1);
    });
  });
}
