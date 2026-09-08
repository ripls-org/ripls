import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart' show QualityTimeAttributes;
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show Request;
import 'package:ripls/data/gen/ripls/api/request_service.pb.dart';
import 'package:ripls/data/repositories/story_repository.dart';
import 'package:ripls/presentation/viewmodels/gear_metric_view_model.dart'
    show LoanSocialAttributeDisplay, mapSocialAttributes;
import 'package:ripls/presentation/widgets/item/item_metric_data.dart';
import 'package:ripls/services/feed_service.dart';
import 'package:ripls/services/providers.dart';

part 'request_metric_view_model.freezed.dart';

final _log = Logger('RequestMetricViewModel');

/// Parameters for request metric view model
@freezed
sealed class RequestMetricParams with _$RequestMetricParams {
  const factory RequestMetricParams({
    required String requestId,
    String? communityId,
  }) = _RequestMetricParams;
}

/// Data for request metrics display (loaded state only - AsyncValue handles loading/error)
@freezed
sealed class RequestMetricData with _$RequestMetricData {
  const factory RequestMetricData({
    required String requestId,
    required String itemName,
    required String description,
    required ItemMetricDisplayData displayData,
    required List<PersonWithRole> people,
    required GetRequestStatsResponse stats,
    required List<StoryPayload> stories,
    required List<ItemMetricValue> metadataMetrics,
    // Social attributes from the per-fulfillment baseline (potentialImpact).
    LoanSocialAttributeDisplay? socialAttributes,
  }) = _RequestMetricData;

  const RequestMetricData._();

  /// Returns the owner's display name, extracted from people list.
  ///
  /// Looks for the person with role "Owner", falling back to the first person
  /// in the list if no owner is found, or "Unknown" if the list is empty.
  String get ownerName {
    final owner = people.cast<PersonWithRole?>().firstWhere(
      (p) => p?.roleLabel == 'Owner',
      orElse: () => people.isNotEmpty ? people.first : null,
    );
    if (owner == null) return 'Unknown';
    return owner.user.name.isNotEmpty ? owner.user.name : owner.user.id;
  }

  /// Returns count of unique partners (people who helped/offered help).
  ///
  /// Counts all unique users in the people list excluding the owner.
  /// This includes helpers and people who offered help.
  int get partnersCount {
    return people
        .where((p) => p.roleLabel != 'Owner')
        .map((p) => p.user.id)
        .toSet()
        .length;
  }
  
  
  
  }

/// AsyncNotifier for managing request metric state.
///
/// Uses AsyncNotifier pattern which automatically handles disposal.
/// The `build()` method loads data automatically when the provider is first accessed.
///
/// Family pattern: pass RequestMetricParams when accessing the provider.
class RequestImpactMetricsNotifier extends AsyncNotifier<RequestMetricData> {
  /// Constructor receives the params from the family provider
  RequestImpactMetricsNotifier(this.params);

  /// The parameters for this notifier instance
  final RequestMetricParams params;

  @override
  Future<RequestMetricData> build() async {
    _log.info('📥 Loading request metrics for: ${params.requestId}');
    return _loadRequestMetrics(params);
  }

  /// Loads all request metric data and converts to display format
  Future<RequestMetricData> _loadRequestMetrics(
    RequestMetricParams params,
  ) async {
    final requestRepository = ref.read(requestRepositoryProvider);
    final storyRepository = ref.read(storyRepositoryProvider);

    // Fetch all data in parallel for better performance
    final results = await Future.wait<dynamic>([
      requestRepository.getRequest(requestId: params.requestId),
      requestRepository.getPeople(
        params.requestId,
        communityId: params.communityId,
      ),
      requestRepository.getStats(
        params.requestId,
        communityId: params.communityId,
      ),
      storyRepository.listByItem(params.requestId, limit: 10),
    ]);

    final requestDetails = results[0] as Request;
    final peopleResponse = results[1] as GetRequestPeopleResponse;
    final statsResponse = results[2] as GetRequestStatsResponse;
    final stories = results[3] as List<StoryPayload>;

    // Get media ID and URL for background
    final mediaId = requestDetails.mediaIds.isNotEmpty
        ? requestDetails.mediaIds.first
        : '';

    String? mediaUrl;
    if (mediaId.isNotEmpty) {
      try {
        final mediaRepository = ref.read(mediaRepositoryProvider);
        final mediaUrlObj = await mediaRepository.getFullMediaUrl(mediaId);
        mediaUrl = mediaUrlObj.url;
      } catch (e) {
        _log.warning('Failed to load request image: $e');
      }
    }

    // Get estimated value from impact stats if available
    String estimatedValue = 'N/A';
    if (statsResponse.helpValueUsd > 0) {
      estimatedValue = '\$${statsResponse.helpValueUsd.toInt()}';
    }

    // Build metrics list (all displayed in 2-column grid)
    final metrics = [
      ItemMetricValue(
        label: 'VALUE',
        displayValue: estimatedValue,
      ),
      const ItemMetricValue(label: 'CO2', displayValue: 'N/A'),
      ItemMetricValue(
        label: 'OFFERS RECEIVED',
        displayValue: '${statsResponse.offersReceived}',
      ),
      ItemMetricValue(
        label: 'DAYS OPEN',
        displayValue: '${statsResponse.daysOpen}',
      ),
    ];

    // potentialImpact = per-fulfillment estimate (server always populates this).
    // Used for SharingImpactCard ("what each fulfillment saves").
    // impact = cumulative. Used for the hero totals row.
    final savings = statsResponse.hasPotentialImpact()
        ? SavingsFormatter.formatSavings(statsResponse.potentialImpact)
        : null;

    // Build display data
    final displayData = ItemMetricDisplayData(
      mediaId: mediaId,
      mediaUrl: mediaUrl,
      metrics: metrics,
      savings: savings,
    );

    // Convert people to PersonWithRole list
    final people = <PersonWithRole>[];

    // Add requester (sort order 0)
    if (peopleResponse.hasRequester()) {
      people.add(
        PersonWithRole(
          user: peopleResponse.requester,
          roleLabel: 'Requester',
          roleColor: AppColors.requestColorOnDark,
          sortOrder: 0,
        ),
      );
    }

    // Add current helper (sort order 1)
    if (peopleResponse.hasCurrentHelper()) {
      people.add(
        PersonWithRole(
          user: peopleResponse.currentHelper,
          roleLabel: 'Helping',
          roleColor: AppColors.statusSuccessOnDark,
          sortOrder: 1,
        ),
      );
    }

    // Add past helpers (sort order 2)
    for (var helper in peopleResponse.pastHelpers) {
      people.add(
        PersonWithRole(
          user: helper,
          roleLabel: 'Helped',
          roleColor: AppColors.statusSuccessOnDark,
          sortOrder: 2,
        ),
      );
    }

    // Add offerers (sort order 3)
    for (var offerer in peopleResponse.offerers) {
      people.add(
        PersonWithRole(
          user: offerer,
          roleLabel: 'Offered',
          roleColor: AppColors.statusInfoOnDark,
          sortOrder: 3,
        ),
      );
    }

    // For requests, metadata metrics are typically empty (no physical attributes to detect)
    final metadataMetrics = <ItemMetricValue>[];

    _log.info(
      '✅ Request metrics loaded - '
      'Offers received: ${statsResponse.offersReceived}, '
      'People: ${people.length}, '
      'Stories: ${stories.length}',
    );

    final QualityTimeAttributes? socialAttrsProto =
        statsResponse.hasPotentialImpact() &&
                statsResponse.potentialImpact.hasQualityTime() &&
                statsResponse.potentialImpact.qualityTime.hasAttributes()
            ? statsResponse.potentialImpact.qualityTime.attributes
            : null;

    return RequestMetricData(
      requestId: params.requestId,
      itemName: requestDetails.title,
      description: requestDetails.description,
      displayData: displayData,
      people: people,
      stats: statsResponse,
      stories: stories,
      metadataMetrics: metadataMetrics,
      socialAttributes: mapSocialAttributes(socialAttrsProto),
    );
  }

  /// Refreshes all request metric data by invalidating caches.
  Future<void> refresh() async {
    _log.info('🔄 Refreshing request metrics for: ${params.requestId}');

    if (!ref.mounted) return;
    state = const AsyncLoading();

    final result = await AsyncValue.guard(() async {
      final requestRepository = ref.read(requestRepositoryProvider);
      final storyRepository = ref.read(storyRepositoryProvider);

      // Invalidate all caches
      await requestRepository.invalidatePeople(
        params.requestId,
        communityId: params.communityId,
      );
      await requestRepository.invalidateStats(
        params.requestId,
        communityId: params.communityId,
      );
      await storyRepository.invalidateItem(params.requestId);

      final data = await _loadRequestMetrics(params);
      _log.info('✅ Request metrics refreshed');
      return data;
    });

    // Check mounted after async gap before updating state
    if (!ref.mounted) return;
    state = result;
  }
}

/// Provider for request metric view model.
///
/// Uses family pattern - pass RequestMetricParams to get metrics for that request.
/// Data loads automatically when provider is first watched.
///
/// Example:
/// ```dart
/// final metricsAsync = ref.watch(requestImpactMetricsProvider(RequestMetricParams(requestId: id)));
/// return metricsAsync.when(
///   data: (metrics) => ItemScoreCard(data: metrics.displayData),
///   loading: () => CircularProgressIndicator(),
///   error: (e, st) => Text('Error: $e'),
/// );
/// ```
final requestImpactMetricsProvider = AsyncNotifierProvider.autoDispose
    .family<RequestImpactMetricsNotifier, RequestMetricData, RequestMetricParams>(
      RequestImpactMetricsNotifier.new,
    );
