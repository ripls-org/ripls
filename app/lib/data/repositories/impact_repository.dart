import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/services/impact_service.dart';

export 'package:ripls/services/impact_service.dart'
    show
        CommunityImpactMetrics,
        UserImpactMetrics,
        GetCommunityImpactMetricsResponse,
        GetUserImpactMetricsResponse,
        GetCommunityMetricDetailResponse,
        GetCommunityActsDetailResponse,
        ActsCategoryBreakdown,
        GetCommunityProblemsSolvedDetailResponse,
        ProblemSolvedItem,
        ProblemSolvedKind,
        GetCommunityActionsResponse,
        ActionItem,
        TopItem,
        ImpactMetricDimension,
        ImpactMetricPeriod,
        ServiceException,
        SourceBreakdown,
        TimeSeriesPoint,
        GetCommunityUtilizationResponse,
        UtilizationItem,
        RedundancyGroup,
        SocialCategory,
        GetTimeToSolveDetailResponse,
        SolvedRequest,
        GetCommunityPercentileDetailResponse,
        RankedCommunity,
        GetCommunityLeaderboardResponse,
        LeaderboardMember,
        GetUserCommunityImpactDetailResponse,
        UserImpactTransaction,
        UserRankedCommunity,
        ImpactEstimate,
        QualityTimeAttributes,
        QualityTimeEstimate,
        MoneySavings,
        PreventedEmissions,
        SocialModality,
        SocialTieStrength,
        SocialReciprocity,
        SocialNovelty,
        SocialVulnerabilityLevel;

/// Repository for impact metrics with transparent caching.
///
/// This repository wraps ImpactMetricsService and provides caching for community
/// and user impact metrics using the global TTL from the environment (CACHE_TTL_MINUTES).
///
/// Cache key naming convention:
/// - 'community:{communityId}:metrics' - Impact metrics for a community
/// - 'community:{communityId}:detail:{dimension}:{period}' - Metric detail breakdown
/// - 'user:{userId}:metrics' - Impact metrics for a user
class ImpactMetricsRepository {
  final CacheService _cache;
  final ImpactMetricsService _service;

  ImpactMetricsRepository(CacheManager cacheManager, this._service)
    : _cache = CacheService(cacheManager, 'impact');

  /// Gets impact metrics for a community with caching.
  ///
  /// Returns the full response including metrics and server-generated insights.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetCommunityImpactMetricsResponse> getMetricsResponse(
      String communityId) async {
    return _cache.get(
      key: 'community:$communityId:metrics_response',
      fetch: () async {
        return _service.getCommunityImpactMetrics(communityId: communityId);
      },
    );
  }

  /// Gets impact metrics for a community with caching.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<CommunityImpactMetrics> getMetrics(String communityId) async {
    final response = await getMetricsResponse(communityId);
    return response.metrics;
  }

  /// Gets aggregated impact metrics for a user with caching.
  ///
  /// Returns [GetUserImpactMetricsResponse] containing:
  /// - metrics: Activity counts, savings metrics, and total value
  /// - userName: User's display name
  /// - userDescription: User's profile description
  /// - userLocationName: User's primary location name
  /// - userMediaId: User's primary profile media ID
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetUserImpactMetricsResponse> getUserMetrics(String userId) async {
    return _cache.get(
      key: 'user:$userId:metrics',
      fetch: () async {
        final response = await _service.getUserImpactMetrics(
          userId: userId,
        );
        return response;
      },
    );
  }

  /// Invalidates cached metrics for a user.
  ///
  /// Call this after mutations that affect user impact
  /// (e.g., completing a transfer, sharing new gear).
  Future<void> invalidateUserMetrics(String userId) async {
    await _cache.invalidate('user:$userId:metrics');
  }

  /// Refreshes metrics for a user by invalidating and re-fetching.
  ///
  /// Returns the freshly fetched metrics with user profile information.
  Future<GetUserImpactMetricsResponse> refreshUserMetrics(String userId) async {
    await invalidateUserMetrics(userId);
    return getUserMetrics(userId);
  }

  /// Invalidates cached metrics for a community.
  ///
  /// Call this after mutations that affect community impact
  /// (e.g., completing a transfer, creating gear).
  Future<void> invalidateMetrics(String communityId) async {
    await _cache.invalidate('community:$communityId:metrics_response');
  }

  /// Invalidates all cached data for a community.
  ///
  /// Clears metrics and all metric detail caches. Call this
  /// after pull-to-refresh or major mutations affecting multiple aspects
  /// of community data.
  Future<void> invalidateAll(String communityId) async {
    await Future.wait([
      invalidateMetrics(communityId),
      invalidateMetricDetails(communityId),
    ]);
  }

  /// Refreshes metrics for a community by invalidating and re-fetching.
  ///
  /// Returns the freshly fetched metrics.
  Future<CommunityImpactMetrics> refreshMetrics(String communityId) async {
    await invalidateMetrics(communityId);
    return getMetrics(communityId);
  }

  /// Gets detailed metric breakdown for a specific dimension and period with caching.
  ///
  /// Returns [GetCommunityMetricDetailResponse] containing:
  /// - Time series for cumulative trend
  /// - Source breakdown (loans, giveaways, events)
  /// - Contributing factors with expandable details
  /// - Top items and contributors
  /// - Community comparisons
  /// - Dimension-specific details (money/time/CO2)
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetCommunityMetricDetailResponse> getMetricDetail(
    String communityId,
    ImpactMetricDimension dimension,
    ImpactMetricPeriod period,
  ) async {
    return _cache.get(
      key: 'community:$communityId:detail:${dimension.name}:${period.name}',
      fetch: () async {
        final response = await _service.getCommunityMetricDetail(
          communityId: communityId,
          dimension: dimension,
          period: period,
        );
        return response;
      },
    );
  }

  /// Invalidates all metric detail caches for a community.
  ///
  /// Call this after mutations that affect community metrics.
  Future<void> invalidateMetricDetails(String communityId) async {
    await _cache.invalidatePattern('community:$communityId:detail:*');
  }

  /// Gets the per-category and per-month breakdown of community acts
  /// with caching.
  ///
  /// Returns [GetCommunityActsDetailResponse] containing the total acts
  /// count, per-category breakdown rows, and a monthly bar series for
  /// the chart.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetCommunityActsDetailResponse> getActsDetail(
    String communityId,
  ) async {
    return _cache.get(
      key: 'community:$communityId:acts',
      fetch: () async {
        return _service.getCommunityActsDetail(communityId: communityId);
      },
    );
  }

  /// Community "problems solved" detail — completed loans + fulfilled requests
  /// + need-fulfilling experiences, with the individual entries. Cached.
  Future<GetCommunityProblemsSolvedDetailResponse> getProblemsSolvedDetail(
    String communityId,
  ) async {
    return _cache.get(
      key: 'community:$communityId:problems_solved',
      fetch: () async {
        return _service.getCommunityProblemsSolvedDetail(
          communityId: communityId,
        );
      },
    );
  }

  /// Invalidates the cached acts detail for a community.
  Future<void> invalidateActsDetail(String communityId) async {
    await _cache.invalidate('community:$communityId:acts');
  }

  /// Refreshes metric detail for a specific dimension and period.
  ///
  /// Returns the freshly fetched metric detail.
  Future<GetCommunityMetricDetailResponse> refreshMetricDetail(
    String communityId,
    ImpactMetricDimension dimension,
    ImpactMetricPeriod period,
  ) async {
    await _cache.invalidate(
      'community:$communityId:detail:${dimension.name}:${period.name}',
    );
    return getMetricDetail(communityId, dimension, period);
  }

  /// Gets utilization analytics for a community with caching.
  Future<GetCommunityUtilizationResponse> getCommunityUtilization(
      String communityId) async {
    return _cache.get(
      key: 'community:$communityId:utilization',
      fetch: () async {
        return _service.getCommunityUtilization(communityId: communityId);
      },
    );
  }

  /// Invalidates cached utilization data for a community.
  Future<void> invalidateUtilization(String communityId) async {
    await _cache.invalidate('community:$communityId:utilization');
  }

  /// Gets paginated list of individual sharing actions for a community.
  Future<GetCommunityActionsResponse> getCommunityActions(
    String communityId, {
    String? pageToken,
    int pageSize = 25,
  }) async {
    return _cache.get(
      key: 'impact:actions:$communityId:${pageToken ?? 'first'}',
      fetch: () async {
        return _service.getCommunityActions(
          communityId: communityId,
          pageToken: pageToken,
          pageSize: pageSize,
        );
      },
    );
  }

  /// Invalidates cached actions for a community.
  Future<void> invalidateActions(String communityId) async {
    await _cache.invalidatePattern('impact:actions:$communityId:*');
  }

  /// Gets community leaderboard with caching.
  Future<GetCommunityLeaderboardResponse> getCommunityLeaderboard(
    String communityId,
    ImpactMetricDimension dimension, {
    int limit = 10,
  }) async {
    return _cache.get(
      key: 'impact:leaderboard:$communityId:${dimension.name}:$limit',
      fetch: () async {
        return _service.getCommunityLeaderboard(
          communityId: communityId,
          dimension: dimension,
          limit: limit,
        );
      },
    );
  }

  /// Gets user community impact detail with caching.
  Future<GetUserCommunityImpactDetailResponse> getUserCommunityImpactDetail(
    String communityId,
    String userId,
    ImpactMetricDimension dimension,
  ) async {
    return _cache.get(
      key: 'impact:user_detail:$communityId:$userId:${dimension.name}',
      fetch: () async {
        return _service.getUserCommunityImpactDetail(
          communityId: communityId,
          userId: userId,
          dimension: dimension,
        );
      },
    );
  }

  /// Gets percentile detail for a community and dimension with caching.
  Future<GetCommunityPercentileDetailResponse> getCommunityPercentileDetail(
    String communityId,
    ImpactMetricDimension dimension,
  ) async {
    return _cache.get(
      key: 'impact:percentile_detail:$communityId:${dimension.name}',
      fetch: () async {
        return _service.getCommunityPercentileDetail(
          communityId: communityId,
          dimension: dimension,
        );
      },
    );
  }

  /// Gets time-to-solve detail for a community with caching.
  Future<GetTimeToSolveDetailResponse> getTimeToSolveDetail(
      String communityId) async {
    return _cache.get(
      key: 'impact:time_to_solve:$communityId',
      fetch: () async {
        return _service.getTimeToSolveDetail(communityId: communityId);
      },
    );
  }

  /// draftImpact produces an LLM-enriched impact estimate for an experience or
  /// request. Results are not cached — each call reflects current chat and
  /// contribution state.
  Future<ImpactEstimate> draftImpact({
    String? experienceId,
    String? requestId,
  }) async {
    final response = await _service.draftImpactEstimate(
      experienceId: experienceId,
      requestId: requestId,
    );
    return response.impact;
  }

  /// redraftImpact recomputes an impact estimate using the provided input
  /// overrides without re-calling the LLM. Results are not cached.
  Future<ImpactEstimate> redraftImpact({
    String? experienceId,
    String? requestId,
    QualityTimeAttributes? qualityTimeInput,
    MoneySavings? moneySavingsInput,
    PreventedEmissions? emissionsInput,
  }) async {
    final response = await _service.draftImpactEstimateWithOverrides(
      experienceId: experienceId,
      requestId: requestId,
      qualityTimeInput: qualityTimeInput,
      moneySavingsInput: moneySavingsInput,
      emissionsInput: emissionsInput,
    );
    return response.impact;
  }
}
