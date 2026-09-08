import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/repositories/feedback_repository.dart';
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/services/feedback_service.dart';
import 'package:ripls/services/impact_service.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/cache_providers.dart';
import 'package:ripls/services/providers/workshop_community_provider.dart';

/// Per-member impact stats aggregated across all enabled communities.
class MemberStats {
  const MemberStats({
    this.formattedHours,
    this.formattedMoney,
    this.formattedEmissions,
  });

  final String? formattedHours;
  final String? formattedMoney;
  final String? formattedEmissions;
}

/// Fetches leaderboard data for the Workshop-scoped communities across
/// the three impact dimensions and merges results by user id into a
/// [MemberStats] map.
///
/// The first non-null value per dimension wins (biased toward the first
/// enabled circle). Results are keyed by user id. Scoping follows the
/// Workshop carousel pin via [workshopEnabledCommunityIdsProvider] so a
/// single-community selection narrows the stats to that circle's
/// members.
final rosterStatsProvider =
    FutureProvider<Map<String, MemberStats>>((ref) async {
  final communityIds = ref.watch(workshopEnabledCommunityIdsProvider);
  if (communityIds.isEmpty) return const {};
  final repo = ref.read(impactMetricsRepositoryProvider);

  const dims = [
    ImpactMetricDimension.IMPACT_METRIC_DIMENSION_QUALITY_TIME,
    ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
    ImpactMetricDimension.IMPACT_METRIC_DIMENSION_EMISSIONS,
  ];

  // Fetch all dimension × community combinations concurrently. Use limit=50
  // (server max) so we get stats for as many members as possible, not just
  // the default top 10.
  final rawResults = await Future.wait([
    for (final id in communityIds)
      for (final dim in dims)
        repo
            .getCommunityLeaderboard(id, dim, limit: 50)
            .then((resp) => (dim, resp)),
  ]);

  // Merge into a user_id → MemberStats map; first non-absent value wins.
  final timeMap = <String, String>{};
  final moneyMap = <String, String>{};
  final emissionsMap = <String, String>{};

  for (final (dim, resp) in rawResults) {
    final allMembers = [
      if (resp.hasCallingUser()) resp.callingUser,
      ...resp.members,
    ];
    for (final m in allMembers) {
      final uid = m.userId;
      if (uid.isEmpty || m.formattedValue.isEmpty) continue;
      switch (dim) {
        case ImpactMetricDimension.IMPACT_METRIC_DIMENSION_QUALITY_TIME:
          timeMap.putIfAbsent(uid, () => m.formattedValue);
        case ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY:
          moneyMap.putIfAbsent(uid, () => m.formattedValue);
        case ImpactMetricDimension.IMPACT_METRIC_DIMENSION_EMISSIONS:
          emissionsMap.putIfAbsent(uid, () => m.formattedValue);
        default:
          break;
      }
    }
  }

  final allUids = {...timeMap.keys, ...moneyMap.keys, ...emissionsMap.keys};
  return {
    for (final uid in allUids)
      uid: MemberStats(
        formattedHours: timeMap[uid],
        formattedMoney: moneyMap[uid],
        formattedEmissions: emissionsMap[uid],
      ),
  };
});

/// Provider for ImpactMetricsService
final impactMetricsServiceProvider = Provider<ImpactMetricsService>((ref) {
  return ImpactMetricsService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    onUnauthenticated: () async {
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Provider for FeedbackService
final feedbackServiceProvider = Provider<FeedbackService>((ref) {
  return FeedbackService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    onUnauthenticated: () async {
      // Clear auth state - the router will automatically redirect to login
      // and preserve the current location in the 'from' query parameter
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Provider for ImpactMetricsRepository
final impactMetricsRepositoryProvider = Provider<ImpactMetricsRepository>((ref) {
  final cache = ref.watch(cacheManagerProvider);
  final service = ref.watch(impactMetricsServiceProvider);
  return ImpactMetricsRepository(cache, service);
});

/// Provider for FeedbackRepository
final feedbackRepositoryProvider = Provider<FeedbackRepository>((ref) {
  final service = ref.watch(feedbackServiceProvider);
  return FeedbackRepository(service);
});
