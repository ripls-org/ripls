import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/services/impact_service.dart';

import 'impact_repository_test.mocks.dart';

@GenerateMocks([ImpactMetricsService])
void main() {
  group('ImpactMetricsRepository — draft methods', () {
    late ImpactMetricsRepository repository;
    late MockImpactMetricsService mockService;
    late StashCacheManager cacheManager;

    setUp(() async {
      mockService = MockImpactMetricsService();
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
      repository = ImpactMetricsRepository(cacheManager, mockService);
    });

    // ── draftImpact ────────────────────────────────────────────────────────

    test('draftImpact(experienceId) delegates to service and returns impact', () async {
      final expectedImpact = ImpactEstimate(
        moneySaved: MoneySavings(),
      );
      when(mockService.draftImpactEstimate(experienceId: 'exp1'))
          .thenAnswer((_) async => DraftImpactEstimateResponse(impact: expectedImpact));

      final result = await repository.draftImpact(experienceId: 'exp1');

      expect(result, expectedImpact);
      verify(mockService.draftImpactEstimate(experienceId: 'exp1')).called(1);
    });

    test('draftImpact(requestId) delegates to service and returns impact', () async {
      final expectedImpact = ImpactEstimate();
      when(mockService.draftImpactEstimate(requestId: 'req1'))
          .thenAnswer((_) async => DraftImpactEstimateResponse(impact: expectedImpact));

      final result = await repository.draftImpact(requestId: 'req1');

      expect(result, expectedImpact);
      verify(mockService.draftImpactEstimate(requestId: 'req1')).called(1);
    });

    // ── redraftImpact ──────────────────────────────────────────────────────

    test('redraftImpact delegates overrides to service and returns impact', () async {
      final overrideQT = QualityTimeAttributes(
        estimatedDurationMinutes: 30,
        modality: SocialModality.SOCIAL_MODALITY_IN_PERSON_SHARED,
      );
      final expectedImpact = ImpactEstimate();
      when(
        mockService.draftImpactEstimateWithOverrides(
          experienceId: 'exp1',
          qualityTimeInput: overrideQT,
        ),
      ).thenAnswer(
        (_) async => DraftImpactEstimateWithOverridesResponse(impact: expectedImpact),
      );

      final result = await repository.redraftImpact(
        experienceId: 'exp1',
        qualityTimeInput: overrideQT,
      );

      expect(result, expectedImpact);
      verify(
        mockService.draftImpactEstimateWithOverrides(
          experienceId: 'exp1',
          qualityTimeInput: overrideQT,
        ),
      ).called(1);
    });

    // ── INV-DRAFT-NOT-CACHED ───────────────────────────────────────────────

    test('draftImpact never reads from or writes to CacheService', () async {
      when(mockService.draftImpactEstimate(experienceId: 'exp1'))
          .thenAnswer((_) async => DraftImpactEstimateResponse(impact: ImpactEstimate()));

      await repository.draftImpact(experienceId: 'exp1');

      // If cache had been hit, a second call would return the cached value
      // without calling the service again. Both calls must hit the service.
      await repository.draftImpact(experienceId: 'exp1');
      verify(mockService.draftImpactEstimate(experienceId: 'exp1')).called(2);
    });

    test('redraftImpact never reads from or writes to CacheService', () async {
      when(
        mockService.draftImpactEstimateWithOverrides(experienceId: 'exp1'),
      ).thenAnswer(
        (_) async => DraftImpactEstimateWithOverridesResponse(impact: ImpactEstimate()),
      );

      await repository.redraftImpact(experienceId: 'exp1');
      await repository.redraftImpact(experienceId: 'exp1');
      verify(
        mockService.draftImpactEstimateWithOverrides(experienceId: 'exp1'),
      ).called(2);
    });
  });

  group('ImpactMetricsRepository — cached methods', () {
    late ImpactMetricsRepository repository;
    late MockImpactMetricsService mockService;
    late StashCacheManager cacheManager;

    setUp(() async {
      mockService = MockImpactMetricsService();
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
      repository = ImpactMetricsRepository(cacheManager, mockService);
    });

    // ── getMetrics / getMetricsResponse ────────────────────────────────────

    test('getMetrics returns metrics from service response', () async {
      final expectedMetrics = CommunityImpactMetrics();
      when(mockService.getCommunityImpactMetrics(communityId: 'com1'))
          .thenAnswer((_) async => GetCommunityImpactMetricsResponse(
                metrics: expectedMetrics,
              ));

      final result = await repository.getMetrics('com1');

      expect(result, expectedMetrics);
      verify(mockService.getCommunityImpactMetrics(communityId: 'com1')).called(1);
    });

    test('getMetrics caches the response', () async {
      when(mockService.getCommunityImpactMetrics(communityId: 'com1'))
          .thenAnswer((_) async => GetCommunityImpactMetricsResponse(
                metrics: CommunityImpactMetrics(),
              ));

      await repository.getMetrics('com1');
      await repository.getMetrics('com1');

      verify(mockService.getCommunityImpactMetrics(communityId: 'com1')).called(1);
    });

    test('getMetrics error propagates to caller', () {
      when(mockService.getCommunityImpactMetrics(communityId: 'com1'))
          .thenThrow(Exception('network error'));

      expect(() => repository.getMetrics('com1'), throwsException);
    });

    // ── getUserMetrics ─────────────────────────────────────────────────────

    test('getUserMetrics returns service response', () async {
      final expectedResponse = GetUserImpactMetricsResponse(
        metrics: UserImpactMetrics(),
      );
      when(mockService.getUserImpactMetrics(userId: 'user1'))
          .thenAnswer((_) async => expectedResponse);

      final result = await repository.getUserMetrics('user1');

      expect(result, expectedResponse);
      verify(mockService.getUserImpactMetrics(userId: 'user1')).called(1);
    });

    test('getUserMetrics caches the response', () async {
      when(mockService.getUserImpactMetrics(userId: 'user1'))
          .thenAnswer((_) async => GetUserImpactMetricsResponse(
                metrics: UserImpactMetrics(),
              ));

      await repository.getUserMetrics('user1');
      await repository.getUserMetrics('user1');

      verify(mockService.getUserImpactMetrics(userId: 'user1')).called(1);
    });

    test('invalidateUserMetrics forces re-fetch on next call', () async {
      when(mockService.getUserImpactMetrics(userId: 'user1'))
          .thenAnswer((_) async => GetUserImpactMetricsResponse(
                metrics: UserImpactMetrics(),
              ));

      await repository.getUserMetrics('user1');
      await repository.invalidateUserMetrics('user1');
      await repository.getUserMetrics('user1');

      verify(mockService.getUserImpactMetrics(userId: 'user1')).called(2);
    });

    test('refreshUserMetrics invalidates and re-fetches', () async {
      when(mockService.getUserImpactMetrics(userId: 'user1'))
          .thenAnswer((_) async => GetUserImpactMetricsResponse(
                metrics: UserImpactMetrics(),
              ));

      await repository.getUserMetrics('user1');
      await repository.refreshUserMetrics('user1');

      verify(mockService.getUserImpactMetrics(userId: 'user1')).called(2);
    });

    // ── getMetricDetail ────────────────────────────────────────────────────

    test('getMetricDetail returns service response', () async {
      final expectedResponse = GetCommunityMetricDetailResponse();
      when(
        mockService.getCommunityMetricDetail(
          communityId: 'com1',
          dimension: ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
          period: ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
        ),
      ).thenAnswer((_) async => expectedResponse);

      final result = await repository.getMetricDetail(
        'com1',
        ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
        ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
      );

      expect(result, expectedResponse);
    });

    test('getMetricDetail caches by dimension+period', () async {
      when(
        mockService.getCommunityMetricDetail(
          communityId: 'com1',
          dimension: ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
          period: ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
        ),
      ).thenAnswer((_) async => GetCommunityMetricDetailResponse());

      await repository.getMetricDetail(
        'com1',
        ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
        ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
      );
      await repository.getMetricDetail(
        'com1',
        ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
        ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
      );

      verify(
        mockService.getCommunityMetricDetail(
          communityId: 'com1',
          dimension: ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
          period: ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
        ),
      ).called(1);
    });

    test('getMetricDetail uses different cache entries for different dimensions', () async {
      when(
        mockService.getCommunityMetricDetail(
          communityId: 'com1',
          dimension: ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
          period: ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
        ),
      ).thenAnswer((_) async => GetCommunityMetricDetailResponse());
      when(
        mockService.getCommunityMetricDetail(
          communityId: 'com1',
          dimension: ImpactMetricDimension.IMPACT_METRIC_DIMENSION_TIME,
          period: ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
        ),
      ).thenAnswer((_) async => GetCommunityMetricDetailResponse());

      await repository.getMetricDetail(
        'com1',
        ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
        ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
      );
      await repository.getMetricDetail(
        'com1',
        ImpactMetricDimension.IMPACT_METRIC_DIMENSION_TIME,
        ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
      );

      verify(
        mockService.getCommunityMetricDetail(
          communityId: 'com1',
          dimension: ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
          period: ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
        ),
      ).called(1);
      verify(
        mockService.getCommunityMetricDetail(
          communityId: 'com1',
          dimension: ImpactMetricDimension.IMPACT_METRIC_DIMENSION_TIME,
          period: ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
        ),
      ).called(1);
    });

    // ── refreshMetrics ─────────────────────────────────────────────────────

    test('refreshMetrics invalidates cache so second call re-fetches', () async {
      when(mockService.getCommunityImpactMetrics(communityId: 'com1'))
          .thenAnswer((_) async => GetCommunityImpactMetricsResponse(
                metrics: CommunityImpactMetrics(),
              ));

      await repository.getMetrics('com1');
      await repository.refreshMetrics('com1');

      verify(mockService.getCommunityImpactMetrics(communityId: 'com1')).called(2);
    });

    // ── getCommunityUtilization ────────────────────────────────────────────

    test('getCommunityUtilization caches the response', () async {
      when(mockService.getCommunityUtilization(communityId: 'com1'))
          .thenAnswer((_) async => GetCommunityUtilizationResponse());

      await repository.getCommunityUtilization('com1');
      await repository.getCommunityUtilization('com1');

      verify(mockService.getCommunityUtilization(communityId: 'com1')).called(1);
    });

    test('invalidateUtilization forces re-fetch', () async {
      when(mockService.getCommunityUtilization(communityId: 'com1'))
          .thenAnswer((_) async => GetCommunityUtilizationResponse());

      await repository.getCommunityUtilization('com1');
      await repository.invalidateUtilization('com1');
      await repository.getCommunityUtilization('com1');

      verify(mockService.getCommunityUtilization(communityId: 'com1')).called(2);
    });

    // ── getCommunityLeaderboard ────────────────────────────────────────────

    test('getCommunityLeaderboard caches by communityId+dimension', () async {
      when(
        mockService.getCommunityLeaderboard(
          communityId: 'com1',
          dimension: ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
        ),
      ).thenAnswer((_) async => GetCommunityLeaderboardResponse());

      await repository.getCommunityLeaderboard(
        'com1',
        ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
      );
      await repository.getCommunityLeaderboard(
        'com1',
        ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
      );

      verify(
        mockService.getCommunityLeaderboard(
          communityId: 'com1',
          dimension: ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
        ),
      ).called(1);
    });

    // ── invalidateAll ──────────────────────────────────────────────────────

    test('invalidateAll clears both metrics and metric detail caches', () async {
      when(mockService.getCommunityImpactMetrics(communityId: 'com1'))
          .thenAnswer((_) async => GetCommunityImpactMetricsResponse(
                metrics: CommunityImpactMetrics(),
              ));
      when(
        mockService.getCommunityMetricDetail(
          communityId: 'com1',
          dimension: ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
          period: ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
        ),
      ).thenAnswer((_) async => GetCommunityMetricDetailResponse());

      await repository.getMetrics('com1');
      await repository.getMetricDetail(
        'com1',
        ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
        ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
      );

      await repository.invalidateAll('com1');

      await repository.getMetrics('com1');
      await repository.getMetricDetail(
        'com1',
        ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
        ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
      );

      verify(mockService.getCommunityImpactMetrics(communityId: 'com1')).called(2);
      verify(
        mockService.getCommunityMetricDetail(
          communityId: 'com1',
          dimension: ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
          period: ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
        ),
      ).called(2);
    });
  });
}
