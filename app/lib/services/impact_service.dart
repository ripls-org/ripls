import 'package:connectrpc/connect.dart' as connect;
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/impact_service.pb.dart';

export 'package:ripls/core/errors/rpc_error_handler.dart'
    show ServiceException;
export 'package:ripls/data/gen/ripls/api/impact.pb.dart'
    show CommunityImpactMetrics, UserImpactMetrics;
export 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart'
    show
        ImpactEstimate,
        QualityTimeAttributes,
        QualityTimeEstimate,
        MoneySavings,
        MoneySavingsInput,
        ContributionValueInput,
        PreventedEmissions,
        PreventedEmissionsInput,
        ContributionEmissionsInput,
        SocialModality,
        SocialTieStrength,
        SocialReciprocity,
        SocialNovelty,
        SocialVulnerabilityLevel;
export 'package:ripls/data/gen/ripls/api/impact_service.pb.dart'
    show
        DraftImpactEstimateRequest,
        DraftImpactEstimateResponse,
        DraftImpactEstimateWithOverridesRequest,
        DraftImpactEstimateWithOverridesResponse;
export 'package:ripls/data/gen/ripls/api/impact_service.pb.dart'
    show
        GetCommunityImpactMetricsRequest,
        GetCommunityImpactMetricsResponse,
        GetUserImpactMetricsResponse,
        GetCommunityMetricDetailResponse,
        GetCommunityActsDetailResponse,
        ActsCategoryBreakdown,
        GetCommunityProblemsSolvedDetailRequest,
        GetCommunityProblemsSolvedDetailResponse,
        ProblemSolvedItem,
        ProblemSolvedKind,
        GetCommunityActionsRequest,
        GetCommunityActionsResponse,
        ActionItem,
        TopItem,
        ImpactMetricDimension,
        ImpactMetricPeriod,
        SourceBreakdown,
        TimeSeriesPoint,
        GetCommunityUtilizationResponse,
        UtilizationItem,
        RedundancyGroup,
        SocialCategory,
        GetTimeToSolveDetailRequest,
        GetTimeToSolveDetailResponse,
        SolvedRequest,
        GetCommunityPercentileDetailRequest,
        GetCommunityPercentileDetailResponse,
        RankedCommunity,
        GetCommunityLeaderboardRequest,
        GetCommunityLeaderboardResponse,
        LeaderboardMember,
        GetUserCommunityImpactDetailRequest,
        GetUserCommunityImpactDetailResponse,
        UserImpactTransaction,
        UserRankedCommunity;

/// ImpactMetricsService handles impact metrics operations using the ImpactService API.
class ImpactMetricsService {
  final ImpactServiceClient _client;
  final String? Function() _getAccessToken;
  final RpcErrorHandler _errorHandler;
  final Future<void> Function()? _onUnauthenticated;

  ImpactMetricsService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
  })  : _client = ImpactServiceClient(transport),
        _getAccessToken = getAccessToken,
        _errorHandler = errorHandler ?? RpcErrorHandler(),
        _onUnauthenticated = onUnauthenticated;

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// GetCommunityImpactMetrics retrieves impact metrics for a community.
  ///
  /// [communityId] is required.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetCommunityImpactMetricsResponse> getCommunityImpactMetrics({
    required String communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetCommunityImpactMetricsRequest(
          communityId: communityId,
        );
        final response = await _client.getCommunityImpactMetrics(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetCommunityImpactMetrics',
    );
  }

  /// GetUserImpactMetrics retrieves aggregated impact metrics for a user.
  ///
  /// [userId] is required.
  ///
  /// Returns response with user impact metrics including activity counts
  /// and savings generated through the user's contributions.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetUserImpactMetricsResponse> getUserImpactMetrics({
    required String userId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetUserImpactMetricsRequest(
          userId: userId,
        );
        final response = await _client.getUserImpactMetrics(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetUserImpactMetrics',
    );
  }

  /// GetCommunityMetricDetail retrieves detailed breakdown data for a specific
  /// impact metric dimension (money, time, or CO2) over a specified time period.
  ///
  /// [communityId] is required.
  /// [dimension] specifies which metric (MONEY, TIME, or CO2).
  /// [period] specifies the time range (ALL, ONE_YEAR, THREE_MONTHS, FOUR_WEEKS).
  ///
  /// Returns response with time series, breakdowns, factors, top items/contributors,
  /// and community comparisons for the specified metric.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetCommunityMetricDetailResponse> getCommunityMetricDetail({
    required String communityId,
    required ImpactMetricDimension dimension,
    required ImpactMetricPeriod period,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetCommunityMetricDetailRequest(
          communityId: communityId,
          dimension: dimension,
          period: period,
        );
        final response = await _client.getCommunityMetricDetail(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetCommunityMetricDetail',
    );
  }

  /// GetCommunityActsDetail retrieves the per-category and per-month
  /// breakdown of community acts.
  ///
  /// [communityId] is required.
  ///
  /// Returns response with the acts total, per-category counts, and a
  /// monthly bar series for the chart.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetCommunityActsDetailResponse> getCommunityActsDetail({
    required String communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetCommunityActsDetailRequest(
          communityId: communityId,
        );
        return _client.getCommunityActsDetail(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetCommunityActsDetail',
    );
  }

  /// GetCommunityProblemsSolvedDetail retrieves the community's "problems
  /// solved" breakdown (completed loans + fulfilled requests + need-fulfilling
  /// experiences) and the individual entries.
  Future<GetCommunityProblemsSolvedDetailResponse>
      getCommunityProblemsSolvedDetail({
    required String communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetCommunityProblemsSolvedDetailRequest(
          communityId: communityId,
        );
        return _client.getCommunityProblemsSolvedDetail(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetCommunityProblemsSolvedDetail',
    );
  }

  /// GetCommunityUtilization retrieves utilization analytics for a community.
  Future<GetCommunityUtilizationResponse> getCommunityUtilization({
    required String communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetCommunityUtilizationRequest(
          communityId: communityId,
        );
        return _client.getCommunityUtilization(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetCommunityUtilization',
    );
  }

  /// GetCommunityLeaderboard retrieves top members ranked by impact.
  Future<GetCommunityLeaderboardResponse> getCommunityLeaderboard({
    required String communityId,
    required ImpactMetricDimension dimension,
    int limit = 10,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetCommunityLeaderboardRequest(
          communityId: communityId,
          dimension: dimension,
          limit: limit,
        );
        return _client.getCommunityLeaderboard(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetCommunityLeaderboard',
    );
  }

  /// GetUserCommunityImpactDetail retrieves a user's impact detail.
  Future<GetUserCommunityImpactDetailResponse> getUserCommunityImpactDetail({
    required String communityId,
    required String userId,
    required ImpactMetricDimension dimension,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetUserCommunityImpactDetailRequest(
          communityId: communityId,
          userId: userId,
          dimension: dimension,
        );
        return _client.getUserCommunityImpactDetail(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetUserCommunityImpactDetail',
    );
  }

  /// GetCommunityPercentileDetail retrieves percentile ranking detail.
  Future<GetCommunityPercentileDetailResponse> getCommunityPercentileDetail({
    required String communityId,
    required ImpactMetricDimension dimension,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetCommunityPercentileDetailRequest(
          communityId: communityId,
          dimension: dimension,
        );
        return _client.getCommunityPercentileDetail(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetCommunityPercentileDetail',
    );
  }

  /// GetTimeToSolveDetail retrieves time-to-solve detail for a community.
  Future<GetTimeToSolveDetailResponse> getTimeToSolveDetail({
    required String communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetTimeToSolveDetailRequest(
          communityId: communityId,
        );
        return _client.getTimeToSolveDetail(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetTimeToSolveDetail',
    );
  }

  /// GetCommunityActions retrieves a paginated list of individual sharing
  /// actions with per-transaction impact values.
  Future<GetCommunityActionsResponse> getCommunityActions({
    required String communityId,
    String? pageToken,
    int pageSize = 25,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetCommunityActionsRequest(
          communityId: communityId,
          pageSize: pageSize,
        );
        if (pageToken != null) {
          request.pageToken = pageToken;
        }
        final response = await _client.getCommunityActions(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetCommunityActions',
    );
  }

  /// DraftImpactEstimate produces an LLM-enriched impact estimate for an
  /// experience or request without persisting it.
  Future<DraftImpactEstimateResponse> draftImpactEstimate({
    String? experienceId,
    String? requestId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = DraftImpactEstimateRequest();
        if (experienceId != null) request.experienceId = experienceId;
        if (requestId != null) request.requestId = requestId;
        return _client.draftImpactEstimate(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'DraftImpactEstimate',
    );
  }

  /// DraftImpactEstimateWithOverrides recomputes an impact estimate using
  /// caller-supplied input overrides without re-calling the LLM.
  Future<DraftImpactEstimateWithOverridesResponse> draftImpactEstimateWithOverrides({
    String? experienceId,
    String? requestId,
    QualityTimeAttributes? qualityTimeInput,
    MoneySavings? moneySavingsInput,
    PreventedEmissions? emissionsInput,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = DraftImpactEstimateWithOverridesRequest();
        if (experienceId != null) request.experienceId = experienceId;
        if (requestId != null) request.requestId = requestId;
        if (qualityTimeInput != null) request.qualityTimeInput = qualityTimeInput;
        if (moneySavingsInput != null) request.moneySavingsInput = moneySavingsInput;
        if (emissionsInput != null) request.emissionsInput = emissionsInput;
        return _client.draftImpactEstimateWithOverrides(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'DraftImpactEstimateWithOverrides',
    );
  }
}
