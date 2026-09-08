import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' show ExperienceState;
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show RequestState;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show Transfer, TransferState, TransferType;
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/services/providers.dart';

part 'community_impact_view_model.freezed.dart';

final _log = Logger('CommunityImpactViewModel');

/// Data for community impact metrics (loaded state only - AsyncValue handles loading/error)
///
/// The community hero image URL is no longer mirrored on this state —
/// `CommunityMetricsScreen` watches `heroMediaUrlProvider` directly so
/// the URL lifecycle is owned by Riverpod. See #2064.
@freezed
sealed class CommunityImpactData with _$CommunityImpactData {
  const factory CommunityImpactData({
    required String communityId,
    required CommunityImpactMetrics metrics,
    @Default([]) List<String> insights,
    @Default(0) double co2PotentialGrams,
    @Default(0) double qualityTimePerPersonMinutes,
    @Default(0) double timeToSolveMinutes,
    @Default(0) int percentileMoney,
    @Default(0) int percentileCo2,
    @Default(0) int percentileQualityTime,
    @Default(false) bool userHasActiveLoan,
    @Default(false) bool userHasActiveExperience,
    @Default(false) bool userHasActiveRequest,
  }) = _CommunityImpactData;

  const CommunityImpactData._();

  /// Returns cost savings formatted as currency string (e.g., "$0.50", "$1,234" or "$1.2K")
  String get costSavingsFormatted {
    final usd = metrics.costSavingsUsd.mean;
    if (usd == 0) {
      return '\$0';
    }
    if (usd < 1) {
      return '\$${usd.toStringAsFixed(2)}';
    }
    final dollars = usd.toInt();
    if (dollars < 1000) {
      return '\$$dollars';
    }
    if (dollars < 10000) {
      final formatted = dollars.toString().replaceAllMapped(
        RegExp(r'(\d{1,3})(?=(\d{3})+(?!\d))'),
        (m) => '${m[1]},',
      );
      return '\$$formatted';
    }
    final k = dollars / 1000;
    return '\$${k.toStringAsFixed(1)}K';
  }

  
  /// Returns carbon savings formatted as string (always in kg, e.g., "0.5 kg", "23 kg", "1,234 kg" or "14K kg")
  String get carbonSavingsFormatted {
    // carbonSavingsGrams is an Estimate (mean + stddev) in grams CO2e.
    final grams = metrics.carbonSavingsGrams.mean;
    if (grams == 0) {
      return '0 kg';
    }
    final kg = grams / 1000;

    // For values < 10, show 1 decimal place
    if (kg < 10) {
      return '${kg.toStringAsFixed(1)} kg';
    }

    // For values < 1000, show as integer
    final kgInt = kg.toInt();
    if (kgInt < 1000) {
      return '$kgInt kg';
    }

    // For values < 10000, add thousands separator
    if (kgInt < 10000) {
      final formatted = kgInt.toString().replaceAllMapped(
        RegExp(r'(\d{1,3})(?=(\d{3})+(?!\d))'),
        (m) => '${m[1]},',
      );
      return '$formatted kg';
    }

    // For values >= 10000, use K abbreviation
    final k = kgInt / 1000;
    return '${k.toStringAsFixed(1)}K kg';
  }

  
  
  
  /// Returns total value formatted as currency string (e.g., "$1,234" or "$1.2K")
  String get totalValueFormatted {
    final dollars = metrics.totalValueUsd.toInt();
    if (dollars == 0) {
      return '\$0';
    }
    if (dollars < 1000) {
      return '\$$dollars';
    }
    if (dollars < 10000) {
      final formatted = dollars.toString().replaceAllMapped(
        RegExp(r'(\d{1,3})(?=(\d{3})+(?!\d))'),
        (m) => '${m[1]},',
      );
      return '\$$formatted';
    }
    final k = dollars / 1000;
    return '\$${k.toStringAsFixed(1)}K';
  }

  /// Returns whether total value data is available
  bool get hasTotalValue => metrics.totalValueUsd > 0;

  /// Returns quality time in compact form for hero stats (e.g., "42 min", "~31 h").
  String get qualityTimeFormatted =>
      SavingsFormatter.formatQualityTimeCompact(metrics.qualityTimeMinutes.mean);

  /// Returns whether quality time data is available
  bool get hasQualityTime => metrics.qualityTimeMinutes.mean > 0;
  }

/// AsyncNotifier for managing community impact state.
///
/// Uses AsyncNotifier pattern which automatically handles disposal - no manual
/// `ref.mounted` checks needed. The `build()` method loads data automatically
/// when the provider is first accessed.
///
/// Family pattern: pass communityId when accessing the provider.
class CommunityImpactMetricsNotifier extends AsyncNotifier<CommunityImpactData> {
  /// Constructor receives the communityId from the family provider
  CommunityImpactMetricsNotifier(this.communityId);

  /// The communityId for this notifier instance
  final String communityId;

  @override
  Future<CommunityImpactData> build() async {
    // React to impact cache invalidation events fired after experience completion
    // or request fulfillment. Rebuilds the notifier so community metrics refresh
    // in place without a full loading spinner.
    ref.watch(impactCacheInvalidationProvider);

    _log.info('📥 Loading impact data for community: $communityId');
    return _loadImpactData(communityId);
  }

  /// Loads all impact data including server-generated insights.
  Future<CommunityImpactData> _loadImpactData(String communityId) async {
    final impactRepository = ref.read(impactMetricsRepositoryProvider);

    // Fetch metrics and per-user activity flags in parallel. Per-user fetches
    // fail open: if any of them throws, the corresponding flag stays false and
    // the metrics screen still renders. We'd rather show a stale nudge than
    // hard-fail the screen because a downstream service hiccupped.
    final results = await Future.wait([
      impactRepository.getMetricsResponse(communityId),
      _userHasActiveLoan(),
      _userHasActiveExperience(),
      _userHasActiveRequest(),
    ]);

    final response = results[0] as GetCommunityImpactMetricsResponse;
    final hasActiveLoan = results[1] as bool;
    final hasActiveExperience = results[2] as bool;
    final hasActiveRequest = results[3] as bool;

    _log.info('Impact data loaded for community: $communityId');

    return CommunityImpactData(
      communityId: communityId,
      metrics: response.metrics,
      insights: response.insights,
      co2PotentialGrams: response.hasCo2PotentialGrams()
          ? response.co2PotentialGrams
          : 0,
      qualityTimePerPersonMinutes:
          response.hasQualityTimePerPersonMinutes()
              ? response.qualityTimePerPersonMinutes
              : 0,
      timeToSolveMinutes: response.hasTimeToSolveMinutes()
          ? response.timeToSolveMinutes
          : 0,
      percentileMoney: response.hasPercentileMoney()
          ? response.percentileMoney
          : 0,
      percentileCo2: response.hasPercentileCo2()
          ? response.percentileCo2
          : 0,
      percentileQualityTime: response.hasPercentileQualityTime()
          ? response.percentileQualityTime
          : 0,
      userHasActiveLoan: hasActiveLoan,
      userHasActiveExperience: hasActiveExperience,
      userHasActiveRequest: hasActiveRequest,
    );
  }

  /// Returns true if the current user has any in-flight loan transfer
  /// (as borrower or lender). Fails open — returns false on error.
  Future<bool> _userHasActiveLoan() async {
    try {
      final transferRepository = ref.read(transferRepositoryProvider);
      final results = await Future.wait([
        transferRepository.listMyTransfers(
          transferType: TransferType.TRANSFER_TYPE_LOAN,
        ),
        transferRepository.listReceivedTransfers(
          transferType: TransferType.TRANSFER_TYPE_LOAN,
        ),
      ]);
      final all = [...results[0], ...results[1]];
      return all.any(_isActiveTransferState);
    } catch (e) {
      _log.warning('Failed to check active loans, treating as none: $e');
      return false;
    }
  }

  /// Returns true if the current user hosts any in-flight experience.
  /// Fails open — returns false on error.
  Future<bool> _userHasActiveExperience() async {
    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      final mine = await experienceRepository.listMyExperiences();
      return mine.any((e) => _isActiveExperienceState(e.state));
    } catch (e) {
      _log.warning('Failed to check active experiences, treating as none: $e');
      return false;
    }
  }

  /// Returns true if the current user has any open request they filed.
  /// Fails open — returns false on error.
  Future<bool> _userHasActiveRequest() async {
    try {
      final requestRepository = ref.read(requestRepositoryProvider);
      final mine = await requestRepository.listMyRequests();
      return mine.any((r) => _isActiveRequestState(r.state));
    } catch (e) {
      _log.warning('Failed to check active requests, treating as none: $e');
      return false;
    }
  }

  bool _isActiveTransferState(Transfer t) =>
      t.state == TransferState.TRANSFER_STATE_INTEREST_EXPRESSED ||
      t.state == TransferState.TRANSFER_STATE_RECIPIENT_SELECTED ||
      t.state == TransferState.TRANSFER_STATE_ACTIVE;

  bool _isActiveExperienceState(ExperienceState s) =>
      s == ExperienceState.EXPERIENCE_STATE_ACTIVE ||
      s == ExperienceState.EXPERIENCE_STATE_JOINED ||
      s == ExperienceState.EXPERIENCE_STATE_IN_PROCESS;

  bool _isActiveRequestState(RequestState s) =>
      s == RequestState.REQUEST_STATE_ACTIVE ||
      s == RequestState.REQUEST_STATE_OFFERS_RECEIVED;

  /// Refreshes all impact data by invalidating caches.
  ///
  /// Note: Even with AsyncNotifier, we still need ref.mounted checks for
  /// methods that manually update state after async operations.
  Future<void> refresh() async {
    _log.info('🔄 Refreshing impact data for community: $communityId');

    if (!ref.mounted) return;
    state = const AsyncLoading();

    final result = await AsyncValue.guard(() async {
      final impactRepository = ref.read(impactMetricsRepositoryProvider);

      // Invalidate all caches
      await impactRepository.invalidateAll(communityId);

      final data = await _loadImpactData(communityId);
      _log.info('✅ Impact data refreshed');
      return data;
    });

    // Check mounted after async gap before updating state
    if (!ref.mounted) return;
    state = result;
  }

  /// Refreshes just the metrics without reloading the chain.
  ///
  /// Useful for quick updates after mutations that affect metrics
  /// but not the chain.
  Future<void> refreshMetrics() async {
    final currentData = state.value;
    if (currentData == null) return;

    _log.info('🔄 Refreshing metrics for community: $communityId');

    if (!ref.mounted) return;
    state = const AsyncLoading();

    final result = await AsyncValue.guard(() async {
      final impactRepository = ref.read(impactMetricsRepositoryProvider);
      final metrics = await impactRepository.refreshMetrics(communityId);

      _log.info('✅ Metrics refreshed');

      return currentData.copyWith(metrics: metrics);
    });

    // Check mounted after async gap before updating state
    if (!ref.mounted) return;
    state = result;
  }
}

/// Provider for community impact view model.
///
/// Uses family pattern - pass communityId to get impact data for that community.
/// Data loads automatically when provider is first watched.
///
/// Example:
/// ```dart
/// final impactAsync = ref.watch(communityImpactProvider(communityId));
/// return impactAsync.when(
///   data: (impact) => Text('Savings: ${impact.costSavingsFormatted}'),
///   loading: () => CircularProgressIndicator(),
///   error: (e, st) => Text('Error: $e'),
/// );
/// ```
final communityImpactProvider = AsyncNotifierProvider.autoDispose
    .family<CommunityImpactMetricsNotifier, CommunityImpactData, String>(
  CommunityImpactMetricsNotifier.new,
);
