import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/gear_metadata_formatter.dart';
import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show Provenance;
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart'
    show QualityTimeAttributes, SocialModality, SocialTieStrength, SocialReciprocity, SocialNovelty, SocialVulnerabilityLevel;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart';
import 'package:ripls/data/repositories/story_repository.dart';
import 'package:ripls/presentation/widgets/item/item_metric_data.dart';
import 'package:ripls/services/feed_service.dart';
import 'package:ripls/services/providers.dart';

part 'gear_metric_view_model.freezed.dart';

final _log = Logger('GearMetricViewModel');

/// Display-ready strings for the seven social attributes of a single loan.
///
/// All fields are human-readable; enum mapping is performed in the view model
/// following MVVM convention so the widget layer contains no proto knowledge.
class LoanSocialAttributeDisplay {
  final String duration;
  final String modality;
  final String groupSize;
  final String connection;
  final String reciprocity;
  final String novelty;
  final String vulnerability;

  const LoanSocialAttributeDisplay({
    required this.duration,
    required this.modality,
    required this.groupSize,
    required this.connection,
    required this.reciprocity,
    required this.novelty,
    required this.vulnerability,
  });
}

/// Maps a [QualityTimeAttributes] proto to display strings for each attribute.
///
/// Returns null when attrs is null or not set on the estimate.
LoanSocialAttributeDisplay? mapSocialAttributes(QualityTimeAttributes? attrs) {
  if (attrs == null) return null;

  return LoanSocialAttributeDisplay(
    duration: _formatDurationMinutes(attrs.estimatedDurationMinutes),
    modality: _formatModality(attrs.modality),
    groupSize: '${attrs.groupSize} ${attrs.groupSize == 1 ? "person" : "people"}',
    connection: _formatTieStrength(attrs.tieStrength),
    reciprocity: _formatReciprocity(attrs.reciprocity),
    novelty: _formatNovelty(attrs.novelty),
    vulnerability: _formatVulnerability(attrs.vulnerability),
  );
}

/// Formats duration in minutes to a human-readable string matching belonging_minutes display.
String _formatDurationMinutes(double minutes) {
  if (minutes <= 0) return '—';
  if (minutes < 60) return '${minutes.round()} min';
  final hours = minutes / 60;
  if (hours == hours.truncateToDouble()) {
    return '${hours.round()} hrs';
  }
  return '${hours.toStringAsFixed(1)} hrs';
}

String _formatModality(SocialModality modality) {
  switch (modality) {
    case SocialModality.SOCIAL_MODALITY_IN_PERSON_SHARED:
      return 'In-person';
    case SocialModality.SOCIAL_MODALITY_IN_PERSON_BRIEF:
      return 'In-person (brief)';
    case SocialModality.SOCIAL_MODALITY_VIDEO:
      return 'Video';
    case SocialModality.SOCIAL_MODALITY_PHONE:
      return 'Phone';
    case SocialModality.SOCIAL_MODALITY_TEXT:
      return 'Text';
    default:
      return '—';
  }
}

String _formatTieStrength(SocialTieStrength tier) {
  switch (tier) {
    case SocialTieStrength.SOCIAL_TIE_STRENGTH_NEW:
      return 'New contact';
    case SocialTieStrength.SOCIAL_TIE_STRENGTH_ACQUAINTANCE:
      return 'Acquaintance';
    case SocialTieStrength.SOCIAL_TIE_STRENGTH_ACTIVE:
      return 'Active tie';
    case SocialTieStrength.SOCIAL_TIE_STRENGTH_CLOSE:
      return 'Close tie';
    default:
      return '—';
  }
}

String _formatReciprocity(SocialReciprocity role) {
  switch (role) {
    case SocialReciprocity.SOCIAL_RECIPROCITY_GIVING:
      return 'Giving';
    case SocialReciprocity.SOCIAL_RECIPROCITY_RECEIVING:
      return 'Receiving';
    case SocialReciprocity.SOCIAL_RECIPROCITY_MUTUAL:
      return 'Mutual';
    default:
      return '—';
  }
}

String _formatNovelty(SocialNovelty level) {
  switch (level) {
    case SocialNovelty.SOCIAL_NOVELTY_NOVEL:
      return 'Novel';
    case SocialNovelty.SOCIAL_NOVELTY_INFREQUENT:
      return 'Infrequent';
    case SocialNovelty.SOCIAL_NOVELTY_ROUTINE:
      return 'Routine';
    default:
      return '—';
  }
}

String _formatVulnerability(SocialVulnerabilityLevel level) {
  switch (level) {
    case SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_LOW:
      return 'Low';
    case SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_MEDIUM:
      return 'Medium';
    case SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_HIGH:
      return 'High';
    default:
      return '—';
  }
}

/// Parameters for gear metric view model
@freezed
sealed class GearMetricParams with _$GearMetricParams {
  const factory GearMetricParams({
    required String gearId,
    String? communityId,
  }) = _GearMetricParams;
}

/// Data for gear metrics display (loaded state only - AsyncValue handles loading/error)
@freezed
sealed class GearMetricData with _$GearMetricData {
  const factory GearMetricData({
    required String gearId,
    required String itemName,
    required String description,
    required ItemMetricDisplayData displayData,
    required List<PersonWithRole> people,
    required GetGearStatsResponse stats,
    required List<StoryPayload> stories,
    required List<ItemMetricValue> metadataMetrics,
    required List<Transfer> loans,
    Availability? availability,
    // Social attributes from the per-loan baseline (potentialImpact).
    // All loans share the same baseline; per-loan variation requires LLM enrichment.
    LoanSocialAttributeDisplay? loanSocialAttributes,
    // Weight and material for drill-down sub-formula display.
    double? weightKg,
    String? materialName,
  }) = _GearMetricData;

  const GearMetricData._();

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

  /// Returns count of unique partners (people who borrowed/interacted with item).
  ///
  /// Counts all unique users in the people list excluding the owner.
  /// This includes current borrowers, past borrowers, and interested parties.
  int get partnersCount {
    return people
        .where((p) => p.roleLabel != 'Owner')
        .map((p) => p.user.id)
        .toSet()
        .length;
  }
  
  
  
  }

/// AsyncNotifier for managing gear metric state.
///
/// Uses AsyncNotifier pattern which automatically handles disposal.
/// The `build()` method loads data automatically when the provider is first accessed.
///
/// Family pattern: pass GearMetricParams when accessing the provider.
class GearImpactMetricsNotifier extends AsyncNotifier<GearMetricData> {
  /// Constructor receives the params from the family provider
  GearImpactMetricsNotifier(this.params);

  /// The parameters for this notifier instance
  final GearMetricParams params;

  @override
  Future<GearMetricData> build() async {
    _log.info('📥 Loading gear metrics for: ${params.gearId}');
    return _loadGearMetrics(params);
  }

  /// Loads all gear metric data and converts to display format
  Future<GearMetricData> _loadGearMetrics(GearMetricParams params) async {
    final gearRepository = ref.read(gearRepositoryProvider);
    final storyRepository = ref.read(storyRepositoryProvider);
    final transferService = ref.read(transferServiceProvider);

    // Fetch all data in parallel for better performance
    final results = await Future.wait<dynamic>([
      gearRepository.getGearDetails(
        params.gearId,
        communityId: params.communityId,
      ),
      gearRepository.getPeople(
        params.gearId,
        communityId: params.communityId,
      ),
      gearRepository.getStats(
        params.gearId,
        communityId: params.communityId,
      ),
      storyRepository.listByItem(params.gearId, limit: 10),
      transferService.getGearTransfers(gearId: params.gearId),
    ]);

    final gearDetails = results[0] as GetGearResponse;
    final peopleResponse = results[1] as GetGearPeopleResponse;
    final statsResponse = results[2] as GetGearStatsResponse;
    final stories = results[3] as List<StoryPayload>;
    final allTransfers = results[4] as List<Transfer>;

    // Filter to loans only (exclude giveaways) and sort by status/date
    final loans = allTransfers
        .where((t) => t.transferType == TransferType.TRANSFER_TYPE_LOAN)
        .toList()
      ..sort((a, b) {
        // Active loans first
        final aActive = a.state == TransferState.TRANSFER_STATE_ACTIVE;
        final bActive = b.state == TransferState.TRANSFER_STATE_ACTIVE;
        if (aActive && !bActive) return -1;
        if (!aActive && bActive) return 1;

        // Then sort by most recent pickup/return date
        final aDate = a.hasActualReturnUnixSec()
            ? a.actualReturnUnixSec
            : a.actualPickupUnixSec;
        final bDate = b.hasActualReturnUnixSec()
            ? b.actualReturnUnixSec
            : b.actualPickupUnixSec;
        return bDate.compareTo(aDate); // Most recent first
      });

    // Get media ID and URL for background
    final mediaId = gearDetails.mediaIds.isNotEmpty
        ? gearDetails.mediaIds.first
        : '';

    String? mediaUrl;
    if (mediaId.isNotEmpty) {
      try {
        final mediaRepository = ref.read(mediaRepositoryProvider);
        final mediaUrlObj = await mediaRepository.getFullMediaUrl(mediaId);
        mediaUrl = mediaUrlObj.url;
      } catch (e) {
        _log.warning('Failed to load gear image: $e');
      }
    }

    // Get estimated value for display
    String estimatedValue = 'N/A';
    Provenance? valueProv;

    if (gearDetails.hasValueEstimate()) {
      final estimate = gearDetails.valueEstimate;
      final dollars = estimate.estimatedValueUsd.toInt();
      estimatedValue = '\$$dollars';

      if (estimate.hasProvenance()) {
        valueProv = estimate.provenance;
      }
    }

    // Build metrics list (all displayed in 2-column grid)
    final metrics = [
      ItemMetricValue(
        label: 'VALUE',
        displayValue: estimatedValue,
        confidence: valueProv != null && valueProv.hasConfidence() ? valueProv.confidence : null,
        methodName: valueProv != null ? SavingsFormatter.provenanceDisplayName(valueProv.name) : null,
        reasoning: valueProv != null && valueProv.reasoning.isNotEmpty ? valueProv.reasoning : null,
        sources: valueProv != null && valueProv.sources.isNotEmpty ? valueProv.sources.toList() : null,
      ),
      const ItemMetricValue(label: 'CO2', displayValue: 'N/A'),
      ItemMetricValue(
        label: 'TIMES LOANED',
        displayValue: '${statsResponse.timesLoaned}',
      ),
      ItemMetricValue(
        label: 'PEOPLE HELPED',
        displayValue: '${statsResponse.peopleHelped}',
      ),
    ];

    // potentialImpact = per-loan estimate (server always populates this).
    // Used for SharingImpactCard ("what each loan saves").
    // impact = cumulative across all loans. Used for the hero totals row.

    // Per-loan savings for SharingImpactCard — use potentialImpact (per-loan baseline).
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

    // Add owner (sort order 0)
    if (peopleResponse.hasOwner()) {
      people.add(
        PersonWithRole(
          user: peopleResponse.owner,
          roleLabel: 'Owner',
          roleColor: AppColors.lightPrimary,
          sortOrder: 0,
        ),
      );
    }

    // Add current borrower (sort order 1)
    if (peopleResponse.hasCurrentBorrower()) {
      people.add(
        PersonWithRole(
          user: peopleResponse.currentBorrower,
          roleLabel: 'Borrowing',
          roleColor: AppColors.loanColorOnDark,
          sortOrder: 1,
        ),
      );
    }

    // Add past borrowers (sort order 2)
    for (var borrower in peopleResponse.pastBorrowers) {
      people.add(
        PersonWithRole(
          user: borrower,
          roleLabel: 'Past borrower',
          roleColor: AppColors.statusSuccessOnDark,
          sortOrder: 2,
        ),
      );
    }

    // Add interested parties (sort order 3)
    for (var interested in peopleResponse.interestedParties) {
      people.add(
        PersonWithRole(
          user: interested,
          roleLabel: 'Interested',
          roleColor: AppColors.statusInfoOnDark,
          sortOrder: 3,
        ),
      );
    }

    // Format metadata for display
    final metadataMetrics = gearDetails.hasMetadata()
        ? GearMetadataFormatter.formatMetadata(gearDetails.metadata)
        : <ItemMetricValue>[];

    _log.info(
      '✅ Gear metrics loaded - '
      'Times loaned: ${statsResponse.timesLoaned}, '
      'People: ${people.length}, '
      'Stories: ${stories.length}, '
      'Metadata fields: ${metadataMetrics.length}, '
      'Loan records: ${loans.length}',
    );

    // Extract social attributes from the per-loan baseline impact estimate.
    final QualityTimeAttributes? socialAttrsProto = statsResponse.hasPotentialImpact() &&
            statsResponse.potentialImpact.hasQualityTime() &&
            statsResponse.potentialImpact.qualityTime.hasAttributes()
        ? statsResponse.potentialImpact.qualityTime.attributes
        : null;

    return GearMetricData(
      gearId: params.gearId,
      itemName: gearDetails.name,
      description: gearDetails.description,
      displayData: displayData,
      people: people,
      stats: statsResponse,
      stories: stories,
      metadataMetrics: metadataMetrics,
      loans: loans,
      availability: gearDetails.hasAvailability() ? gearDetails.availability : null,
      loanSocialAttributes: mapSocialAttributes(socialAttrsProto),
      weightKg: _extractWeightKg(gearDetails),
      materialName: _extractMaterialName(gearDetails),
    );
  }

  /// Refreshes all gear metric data by invalidating caches.
  Future<void> refresh() async {
    _log.info('🔄 Refreshing gear metrics for: ${params.gearId}');

    if (!ref.mounted) return;
    state = const AsyncLoading();

    final result = await AsyncValue.guard(() async {
      final gearRepository = ref.read(gearRepositoryProvider);
      final storyRepository = ref.read(storyRepositoryProvider);

      // Invalidate all caches
      await gearRepository.invalidatePeople(
        params.gearId,
        communityId: params.communityId,
      );
      await gearRepository.invalidateStats(
        params.gearId,
        communityId: params.communityId,
      );
      await storyRepository.invalidateItem(params.gearId);

      final data = await _loadGearMetrics(params);
      _log.info('✅ Gear metrics refreshed');
      return data;
    });

    // Check mounted after async gap before updating state
    if (!ref.mounted) return;
    state = result;
  }

  static double? _extractWeightKg(GetGearResponse gear) {
    if (!gear.hasMetadata()) return null;
    final md = gear.metadata;
    if (!md.hasWeightGrams() || !md.weightGrams.hasValue()) return null;
    final grams = md.weightGrams.value.mean;
    return grams > 0 ? grams / 1000.0 : null;
  }

  static String? _extractMaterialName(GetGearResponse gear) {
    if (!gear.hasMetadata()) return null;
    final md = gear.metadata;
    if (!md.hasMaterialCategory() || !md.materialCategory.hasValue()) {
      return null;
    }
    return GearMetadataFormatter.formatMaterialCategory(
        md.materialCategory.value);
  }
}

/// Provider for gear metric view model.
///
/// Uses family pattern - pass GearMetricParams to get metrics for that gear item.
/// Data loads automatically when provider is first watched.
///
/// Example:
/// ```dart
/// final metricsAsync = ref.watch(gearImpactMetricsProvider(GearMetricParams(gearId: id)));
/// return metricsAsync.when(
///   data: (metrics) => ItemScoreCard(data: metrics.displayData),
///   loading: () => CircularProgressIndicator(),
///   error: (e, st) => Text('Error: $e'),
/// );
/// ```
final gearImpactMetricsProvider = AsyncNotifierProvider.autoDispose
    .family<GearImpactMetricsNotifier, GearMetricData, GearMetricParams>(
      GearImpactMetricsNotifier.new,
    );
