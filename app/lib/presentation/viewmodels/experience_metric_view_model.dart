import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart' show QualityTimeAttributes;
import 'package:ripls/data/repositories/story_repository.dart';
import 'package:ripls/presentation/viewmodels/gear_metric_view_model.dart'
    show LoanSocialAttributeDisplay, mapSocialAttributes;
import 'package:ripls/presentation/widgets/item/item_metric_data.dart';
import 'package:ripls/services/feed_service.dart';
import 'package:ripls/services/providers.dart';

part 'experience_metric_view_model.freezed.dart';

final _log = Logger('ExperienceMetricViewModel');

/// Parameters for experience metric view model
@freezed
sealed class ExperienceMetricParams with _$ExperienceMetricParams {
  const factory ExperienceMetricParams({
    required String experienceId,
    String? communityId,
  }) = _ExperienceMetricParams;
}

/// Data for experience metrics display (loaded state only - AsyncValue handles loading/error)
@freezed
sealed class ExperienceMetricData with _$ExperienceMetricData {
  const factory ExperienceMetricData({
    required String experienceId,
    required String itemName,
    required String description,
    required ItemMetricDisplayData displayData,
    required List<PersonWithRole> people,
    required GetExperienceStatsResponse stats,
    required List<StoryPayload> stories,
    required List<ItemMetricValue> metadataMetrics,
    // Social attributes from the per-session baseline (potentialImpact).
    LoanSocialAttributeDisplay? socialAttributes,
  }) = _ExperienceMetricData;

  const ExperienceMetricData._();

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

  /// Returns count of unique partners (people who participated in experience).
  ///
  /// Counts all unique users in the people list excluding the owner.
  /// This includes participants and people who RSVPed.
  int get partnersCount {
    return people
        .where((p) => p.roleLabel != 'Owner')
        .map((p) => p.user.id)
        .toSet()
        .length;
  }
  
  
  
  }

/// AsyncNotifier for managing experience metric state.
///
/// Uses AsyncNotifier pattern which automatically handles disposal.
/// The `build()` method loads data automatically when the provider is first accessed.
///
/// Family pattern: pass ExperienceMetricParams when accessing the provider.
class ExperienceImpactMetricsNotifier extends AsyncNotifier<ExperienceMetricData> {
  /// Constructor receives the params from the family provider
  ExperienceImpactMetricsNotifier(this.params);

  /// The parameters for this notifier instance
  final ExperienceMetricParams params;

  @override
  Future<ExperienceMetricData> build() async {
    _log.info('📥 Loading experience metrics for: ${params.experienceId}');
    return _loadExperienceMetrics(params);
  }

  /// Loads all experience metric data and converts to display format
  Future<ExperienceMetricData> _loadExperienceMetrics(
    ExperienceMetricParams params,
  ) async {
    final experienceRepository = ref.read(experienceRepositoryProvider);
    final storyRepository = ref.read(storyRepositoryProvider);

    // Fetch all data in parallel for better performance
    final results = await Future.wait<dynamic>([
      experienceRepository.getExperienceDetails(
        params.experienceId,
        communityId: params.communityId,
      ),
      experienceRepository.getPeople(
        params.experienceId,
        communityId: params.communityId,
      ),
      experienceRepository.getStats(
        params.experienceId,
        communityId: params.communityId,
      ),
      storyRepository.listByItem(params.experienceId, limit: 10),
    ]);

    final experienceDetails = results[0] as GetExperienceResponse;
    final peopleResponse = results[1] as GetExperiencePeopleResponse;
    final statsResponse = results[2] as GetExperienceStatsResponse;
    final stories = results[3] as List<StoryPayload>;

    // Get media ID and URL for background
    final mediaId = experienceDetails.experience.mediaIds.isNotEmpty
        ? experienceDetails.experience.mediaIds.first
        : '';

    String? mediaUrl;
    if (mediaId.isNotEmpty) {
      try {
        final mediaRepository = ref.read(mediaRepositoryProvider);
        final mediaUrlObj = await mediaRepository.getFullMediaUrl(mediaId);
        mediaUrl = mediaUrlObj.url;
      } catch (e) {
        _log.warning('Failed to load experience image: $e');
      }
    }

    // Get estimated value from impact stats if available
    String estimatedValue = 'N/A';
    if (statsResponse.valueCreatedUsd > 0) {
      estimatedValue = '\$${statsResponse.valueCreatedUsd.toInt()}';
    }

    // Build metrics list (all displayed in 2-column grid)
    final metrics = [
      ItemMetricValue(
        label: 'VALUE',
        displayValue: estimatedValue,
      ),
      const ItemMetricValue(label: 'CO2', displayValue: 'N/A'),
      ItemMetricValue(
        label: 'SESSIONS HELD',
        displayValue: '${statsResponse.sessionsHeld}',
      ),
      ItemMetricValue(
        label: 'ATTENDEES',
        displayValue: '${statsResponse.totalAttendees}',
      ),
    ];

    // potentialImpact = per-session estimate (server always populates this).
    // Used for SharingImpactCard ("what each session saves").
    // impact = cumulative (attendee-scaled). Used for the hero totals row.
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

    // Add host (sort order 0)
    if (peopleResponse.hasHost()) {
      people.add(
        PersonWithRole(
          user: peopleResponse.host,
          roleLabel: 'Host',
          roleColor: AppColors.experienceColorOnDark,
          sortOrder: 0,
        ),
      );
    }

    // Add upcoming RSVPs (sort order 1)
    for (var rsvp in peopleResponse.upcomingRsvps) {
      people.add(
        PersonWithRole(
          user: rsvp,
          roleLabel: 'RSVP\'d',
          roleColor: AppColors.experienceSageGreen,
          sortOrder: 1,
        ),
      );
    }

    // Add past attendees (sort order 2)
    for (var attendee in peopleResponse.pastAttendees) {
      people.add(
        PersonWithRole(
          user: attendee,
          roleLabel: 'Attended',
          roleColor: AppColors.statusSuccessOnDark,
          sortOrder: 2,
        ),
      );
    }

    // For experiences, metadata metrics are typically empty (no physical attributes to detect)
    final metadataMetrics = <ItemMetricValue>[];

    _log.info(
      '✅ Experience metrics loaded - '
      'Sessions held: ${statsResponse.sessionsHeld}, '
      'People: ${people.length}, '
      'Stories: ${stories.length}',
    );

    final QualityTimeAttributes? socialAttrsProto =
        statsResponse.hasPotentialImpact() &&
                statsResponse.potentialImpact.hasQualityTime() &&
                statsResponse.potentialImpact.qualityTime.hasAttributes()
            ? statsResponse.potentialImpact.qualityTime.attributes
            : null;

    return ExperienceMetricData(
      experienceId: params.experienceId,
      itemName: experienceDetails.experience.name,
      description: experienceDetails.experience.description,
      displayData: displayData,
      people: people,
      stats: statsResponse,
      stories: stories,
      metadataMetrics: metadataMetrics,
      socialAttributes: mapSocialAttributes(socialAttrsProto),
    );
  }

  /// Refreshes all experience metric data by invalidating caches.
  Future<void> refresh() async {
    _log.info('🔄 Refreshing experience metrics for: ${params.experienceId}');

    if (!ref.mounted) return;
    state = const AsyncLoading();

    final result = await AsyncValue.guard(() async {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      final storyRepository = ref.read(storyRepositoryProvider);

      // Invalidate all caches
      await experienceRepository.invalidatePeople(
        params.experienceId,
        communityId: params.communityId,
      );
      await experienceRepository.invalidateStats(
        params.experienceId,
        communityId: params.communityId,
      );
      await storyRepository.invalidateItem(params.experienceId);

      final data = await _loadExperienceMetrics(params);
      _log.info('✅ Experience metrics refreshed');
      return data;
    });

    // Check mounted after async gap before updating state
    if (!ref.mounted) return;
    state = result;
  }
}

/// Provider for experience metric view model.
///
/// Uses family pattern - pass ExperienceMetricParams to get metrics for that experience.
/// Data loads automatically when provider is first watched.
///
/// Example:
/// ```dart
/// final metricsAsync = ref.watch(experienceImpactMetricsProvider(ExperienceMetricParams(experienceId: id)));
/// return metricsAsync.when(
///   data: (metrics) => ItemScoreCard(data: metrics.displayData),
///   loading: () => CircularProgressIndicator(),
///   error: (e, st) => Text('Error: $e'),
/// );
/// ```
final experienceImpactMetricsProvider = AsyncNotifierProvider.autoDispose
    .family<
      ExperienceImpactMetricsNotifier,
      ExperienceMetricData,
      ExperienceMetricParams
    >(ExperienceImpactMetricsNotifier.new);
