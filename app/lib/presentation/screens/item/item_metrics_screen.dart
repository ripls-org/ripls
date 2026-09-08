import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../../core/extensions/l10n_extensions.dart';
import '../../../core/theme/app_colors.dart';
import '../../../core/theme/app_theme.dart';
import '../../../core/utils/drill_down_builder.dart';
import '../../../core/utils/savings_formatter.dart';
import '../../../data/gen/ripls/api/impact_estimate.pb.dart';
import '../../../data/gen/ripls/api/transfer.pb.dart';
import '../../models/drill_down_data.dart';
import '../../models/item_metric_data_base.dart';
import '../../viewmodels/experience_metric_view_model.dart';
import '../../viewmodels/gear_metric_view_model.dart';
import '../../viewmodels/request_metric_view_model.dart';
import '../../widgets/accessibility/tappable.dart';
import '../../widgets/app_bar_back_button.dart';
import '../../widgets/impact/impact_hero_section.dart';
import '../../widgets/impact/loan_history_card.dart';
import '../../widgets/impact/sharing_impact_card.dart';
import '../../widgets/item/item_metric_data.dart'
    show ItemSavingsData, PersonWithRole;
import '../../widgets/swipe_to_close_mixin.dart';
import '../../widgets/user_avatar.dart';
import '../impact_metrics/metric_drill_down_screen.dart';

/// ItemMetricsScreen displays impact metrics for gear, requests, or experiences.
///
/// Uses a sealed-class pattern to support all three item types with a single screen.
/// Shows:
/// - Hero section with item name, owner, description, and inline stats row
/// - "What Sharing This Means" explanatory card with per-action metrics
/// - Transaction history section (loans/fulfillments/sessions)
/// - Pre-transaction empty state (if no transactions)
class ItemMetricsScreen extends ConsumerStatefulWidget {
  final ItemType itemType;
  final String itemId;
  final String? communityId;

  const ItemMetricsScreen({
    super.key,
    required this.itemType,
    required this.itemId,
    this.communityId,
  });

  @override
  ConsumerState<ItemMetricsScreen> createState() => _ItemMetricsScreenState();
}

class _ItemMetricsScreenState extends ConsumerState<ItemMetricsScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  @override
  Widget build(BuildContext context) {
    // Load data based on item type
    final AsyncValue<ItemMetricDataBase> metricsAsync = switch (widget.itemType) {
      ItemType.gear => ref.watch(
          gearImpactMetricsProvider(
            GearMetricParams(
              gearId: widget.itemId,
              communityId: widget.communityId,
            ),
          ),
        ).whenData((data) => GearItemMetricData(data)),
      ItemType.request => ref.watch(
          requestImpactMetricsProvider(
            RequestMetricParams(
              requestId: widget.itemId,
              communityId: widget.communityId,
            ),
          ),
        ).whenData((data) => RequestItemMetricData(data)),
      ItemType.experience => ref.watch(
          experienceImpactMetricsProvider(
            ExperienceMetricParams(
              experienceId: widget.itemId,
              communityId: widget.communityId,
            ),
          ),
        ).whenData((data) => ExperienceItemMetricData(data)),
    };

    return buildSwipeableScaffold(
      backgroundColor: AppColors.transferCardBackground(context),
      extendBodyBehindAppBar: true,
      appBar: metricsAsync.maybeWhen(
        data: (data) => AppBar(
          leading: AppBarBackButton(color: Colors.white, onPressed: handleClose),
          title: Text(
            data.itemName,
            style: const TextStyle(
              color: Colors.white,
              fontSize: 17,
              fontWeight: FontWeight.w600,
            ),
          ),
          backgroundColor: Colors.transparent,
          elevation: 0,
        ),
        orElse: () => AppBar(
          leading: AppBarBackButton(color: Colors.white, onPressed: handleClose),
          backgroundColor: Colors.transparent,
          elevation: 0,
        ),
      ),
      body: metricsAsync.when(
        data: (data) => _buildContent(context, data),
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (error, stack) => Center(
          child: Text('Error loading metrics: $error'),
        ),
      ),
    );
  }

  Widget _buildContent(BuildContext context, ItemMetricDataBase data) {
    final hasTransactions = data.hasTransactions;

    // Cumulative impact from server (hero stats row shows totals, zeros when absent).
    final cumulativeSavings = SavingsFormatter.formatSavings(
      switch (data) {
        GearItemMetricData(:final data) =>
          data.stats.hasImpact() ? data.stats.impact : null,
        RequestItemMetricData(:final data) =>
          data.stats.hasImpact() ? data.stats.impact : null,
        ExperienceItemMetricData(:final data) =>
          data.stats.hasImpact() ? data.stats.impact : null,
      },
    );

    // Per-action savings (potentialImpact when no transactions,
    // actualImpact/timesLoaned otherwise) shown by the explanatory card.
    final savings = data.displayData.savings;
    final sharingCard = SharingImpactCard(
      framingText: _sharingFramingText(context, widget.itemType),
      perActionLabel: _perActionLabel(context, widget.itemType),
      valueSaved: savings?.costDetail?.displayValue,
      timeRecovered: savings?.timeDetail?.displayValue,
      co2Avoided: savings?.co2Detail?.displayValue,
      qualityTime: savings?.qtDetail?.displayValue,
      onValueTap: data.perActionImpact != null
          ? () => _showDrillDown(context, 'value', data.perActionImpact, data)
          : null,
      onTimeTap: data.perActionImpact != null
          ? () => _showDrillDown(context, 'time', data.perActionImpact, data)
          : null,
      onCo2Tap: data.perActionImpact != null
          ? () => _showDrillDown(context, 'co2', data.perActionImpact, data)
          : null,
      onSocialTap: data.perActionImpact != null
          ? () => _showDrillDown(context, 'social', data.perActionImpact, data)
          : null,
    );

    return CustomScrollView(
      slivers: [
        // Hero section with inline stats row (cumulative totals; null when
        // every metric is zero — a "$0 Saved" headline reads as broken).
        SliverToBoxAdapter(
          child: ImpactMetricsHeroSection(
            subtitle: data.ownerName.toUpperCase(),
            description: data.description,
            backgroundImageUrl: data.displayData.mediaUrl,
            extendToTop: true,
            bottomContent: _buildHeroStatsRow(context, cumulativeSavings, data),
          ),
        ),

        // Main content
        SliverToBoxAdapter(
          child: Padding(
            padding: const EdgeInsets.fromLTRB(20, 24, 20, 40),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                // Per-action explanatory card. Hidden when every per-action
                // metric is zero/missing (#2724).
                if (sharingCard.hasAnyMetric) sharingCard,

                ..._buildHistorySection(context, data, hasTransactions),
              ],
            ),
          ),
        ),
      ],
    );
  }

  /// The italic framing line above the metric tiles, worded for the item
  /// type *and* the likely viewer. Gear is borrowed instead of bought
  /// (owner-framed); a request receipt is read by the requester, so it is
  /// requester-framed — neighbors pitched in so *you* didn't buy new
  /// (#2724); an experience gathers people instead of everyone going it
  /// alone.
  String _sharingFramingText(BuildContext context, ItemType itemType) =>
      switch (itemType) {
        ItemType.gear => context.l10n.impactFramingGear,
        ItemType.request => context.l10n.impactFramingRequest,
        ItemType.experience => context.l10n.impactFramingExperience,
      };

  /// Header qualifier for the explanatory card: its numbers are per action
  /// (one loan / fulfillment / session), unlike the hero strip's cumulative
  /// totals (#2724).
  String _perActionLabel(BuildContext context, ItemType itemType) =>
      switch (itemType) {
        ItemType.gear => context.l10n.impactPerActionLoan,
        ItemType.request => context.l10n.impactPerActionFulfillment,
        ItemType.experience => context.l10n.impactPerActionSession,
      };

  /// _buildHeroStatsRow builds the centered metric columns shown in the
  /// hero header (cumulative impact from server: value, time, CO₂, and
  /// quality time). Zero metrics are suppressed and the remaining columns
  /// fill the row; returns null when nothing is non-zero so the hero
  /// renders without an all-zeros strip (#2724). A one-line caption
  /// distinguishes the two time metrics when either is shown (#2724).
  Widget? _buildHeroStatsRow(
    BuildContext context,
    ItemSavingsData? cumulativeSavings,
    ItemMetricDataBase data,
  ) {
    if (cumulativeSavings == null) return null;
    final cumImpact = data.cumulativeImpact;
    final metrics = [
      if (cumulativeSavings.costDetail != null)
        (
          value: cumulativeSavings.costSaved,
          label: 'SAVED',
          isTime: false,
          onTap: cumImpact != null
              ? () => _showDrillDown(context, 'value', cumImpact, data)
              : null,
        ),
      if (cumulativeSavings.co2Detail != null)
        (
          value: cumulativeSavings.co2Saved,
          label: 'CO₂ AVOIDED',
          isTime: false,
          onTap: cumImpact != null
              ? () => _showDrillDown(context, 'co2', cumImpact, data)
              : null,
        ),
      if (cumulativeSavings.timeDetail != null)
        (
          value: cumulativeSavings.timeSaved,
          label: 'RECOVERED',
          isTime: true,
          onTap: cumImpact != null
              ? () => _showDrillDown(context, 'time', cumImpact, data)
              : null,
        ),
      if (cumulativeSavings.qtSaved != null)
        (
          value: cumulativeSavings.qtSaved!,
          label: 'QUALITY TIME',
          isTime: true,
          onTap: cumImpact != null
              ? () => _showDrillDown(context, 'social', cumImpact, data)
              : null,
        ),
    ];
    if (metrics.isEmpty) return null;
    final hasTimeMetric = metrics.any((m) => m.isTime);

    return Column(
      children: [
        Row(
          children: metrics.map((m) {
            final content = Column(
              children: [
                Text(m.value, style: AppTheme.heroMetricValueStyle),
                const SizedBox(height: 2),
                Text(m.label, style: AppTheme.heroMetricLabelStyle),
              ],
            );

            // Expanded must be outermost so Row flex parent data is preserved.
            return Expanded(
              child: m.onTap != null
                  ? Tappable(
                      semanticsLabel: m.label,
                      onTap: m.onTap,
                      excludeChildSemantics: false,
                      child: content,
                    )
                  : content,
            );
          }).toList(),
        ),
        if (hasTimeMetric) ...[
          const SizedBox(height: 10),
          Text(
            context.l10n.impactCaptionTimeMetrics,
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 10,
              height: 1.4,
              fontStyle: FontStyle.italic,
              color: Colors.white.withValues(alpha: 0.55),
            ),
          ),
        ],
      ],
    );
  }

  String _getHistorySectionLabel(ItemMetricDataBase data) {
    return switch (data.itemType) {
      ItemType.gear => 'LOAN HISTORY',
      ItemType.request => 'FULFILLMENT HISTORY',
      ItemType.experience => 'SESSION HISTORY',
    };
  }

  /// Builds the history heading plus its body. When transactions exist but no
  /// rows are renderable client-side, the whole section is omitted — a heading
  /// over empty space reads as broken.
  List<Widget> _buildHistorySection(
    BuildContext context,
    ItemMetricDataBase data,
    bool hasTransactions,
  ) {
    final history = hasTransactions ? _buildTransactionHistory(data) : null;
    if (hasTransactions && history == null) return const [];

    return [
      const SizedBox(height: 32),
      Text(
        _getHistorySectionLabel(data),
        style: TextStyle(
          color: AppColors.transferTextMuted(context),
          fontSize: 11,
          fontWeight: FontWeight.w700,
          letterSpacing: 0.6,
        ),
      ),
      const SizedBox(height: 16),
      history ?? _buildEmptyState(data),
    ];
  }

  Widget? _buildTransactionHistory(ItemMetricDataBase data) {
    return switch (data) {
      GearItemMetricData(:final data) => _buildGearLoanHistory(data),
      RequestItemMetricData(:final data) => _buildRequestFulfillmentHistory(data),
      ExperienceItemMetricData(:final data) => _buildExperienceSessionHistory(data),
    };
  }

  Widget? _buildGearLoanHistory(GearMetricData data) {
    if (data.loans.isEmpty) {
      return null;
    }

    final qtValue = data.displayData.savings?.qtSaved;
    final attrs = data.loanSocialAttributes;

    return Column(
      children: data.loans.map((loan) {
        final isActive = loan.state == TransferState.TRANSFER_STATE_ACTIVE;
        final startDate = loan.hasActualPickupUnixSec()
            ? DateTime.fromMillisecondsSinceEpoch(
                loan.actualPickupUnixSec.toInt() * 1000)
            : DateTime.now();
        final endDate = loan.hasActualReturnUnixSec()
            ? DateTime.fromMillisecondsSinceEpoch(
                loan.actualReturnUnixSec.toInt() * 1000)
            : null;

        return LoanHistoryCard(
          borrower: loan.recipient,
          startDate: startDate,
          endDate: endDate,
          isActive: isActive,
          rmValue: qtValue,
          attributes: attrs,
          onDrillDownTap: data.stats.hasPotentialImpact()
              ? () => _showDrillDown(
                    context,
                    'social',
                    data.stats.potentialImpact,
                    GearItemMetricData(data),
                  )
              : null,
        );
      }).toList(),
    );
  }

  Widget? _buildRequestFulfillmentHistory(RequestMetricData data) {
    // No per-fulfillment RPC exists yet; the people list (current + past
    // helpers) is the fulfillment record we can show.
    return _buildPeopleHistoryCard(
      data.people,
      const {'Helping', 'Helped'},
      'Helped fulfill this request',
    );
  }

  Widget? _buildExperienceSessionHistory(ExperienceMetricData data) {
    // No per-session RPC exists yet; confirmed attendees are the session
    // record we can show.
    return _buildPeopleHistoryCard(
      data.people,
      const {'Attended'},
      'Attended',
    );
  }

  /// Renders a history card of the people matching [roles], or null when
  /// there is no one to show (the caller then hides the section).
  Widget? _buildPeopleHistoryCard(
    List<PersonWithRole> people,
    Set<String> roles,
    String subtitle,
  ) {
    final seen = <String>{};
    final rows = people
        .where((p) => roles.contains(p.roleLabel) && seen.add(p.user.id))
        .toList();
    if (rows.isEmpty) return null;

    return Container(
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: AppColors.transferBorder(context),
          width: 1,
        ),
      ),
      child: Column(
        children: [
          for (final (i, person) in rows.indexed) ...[
            if (i > 0)
              Divider(
                height: 1,
                color: AppColors.transferBorderLight(context),
              ),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
              child: Row(
                children: [
                  UserAvatar(user: person.user, radius: 16),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          person.user.name,
                          style: TextStyle(
                            fontSize: 14,
                            fontWeight: FontWeight.w600,
                            color: AppColors.transferTextPrimary(context),
                          ),
                        ),
                        const SizedBox(height: 2),
                        Text(
                          subtitle,
                          style: TextStyle(
                            fontSize: 12,
                            color: AppColors.transferTextMuted(context),
                          ),
                        ),
                      ],
                    ),
                  ),
                  Icon(
                    Icons.check_circle,
                    size: 18,
                    color: AppColors.statusSuccess(context),
                  ),
                ],
              ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildEmptyState(ItemMetricDataBase data) {
    final (icon, heading, description) = switch (data.itemType) {
      ItemType.gear => (
        Icons.favorite_border,
        'No loans yet',
        'When someone borrows this item, you\'ll see the impact it makes here.',
      ),
      ItemType.request => (
        Icons.help_outline,
        'No help yet',
        'When someone helps with this request, you\'ll see the impact here.',
      ),
      ItemType.experience => (
        Icons.event_available,
        'No sessions yet',
        'When this event has sessions, you\'ll see the impact here.',
      ),
    };

    return Container(
      padding: const EdgeInsets.all(24),
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: AppColors.transferBorder(context),
          width: 1,
        ),
      ),
      child: Column(
        children: [
          Container(
            width: 64,
            height: 64,
            decoration: BoxDecoration(
              color: AppColors.transferBorderLight(context),
              shape: BoxShape.circle,
            ),
            child: Icon(
              icon,
              size: 32,
              color: AppColors.transferTextSecondary(context),
            ),
          ),
          const SizedBox(height: 16),
          Text(
            heading,
            style: TextStyle(
              color: AppColors.transferTextPrimary(context),
              fontSize: 18,
              fontWeight: FontWeight.w600,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            description,
            textAlign: TextAlign.center,
            style: TextStyle(
              color: AppColors.transferTextSecondary(context),
              fontSize: 14,
              height: 1.5,
            ),
          ),
        ],
      ),
    );
  }

  void _showDrillDown(
    BuildContext context,
    String metricKey,
    ImpactEstimate? impact,
    ItemMetricDataBase data,
  ) {
    if (impact == null) return;

    final DrillDownData? drillDown = switch (metricKey) {
      'value' => DrillDownBuilder.buildMoneySavedDrillDown(
          impact,
          itemType: data.itemType,
          itemName: data.itemName,
        ),
      'co2' => DrillDownBuilder.buildEmissionsPreventedDrillDown(
          impact,
          itemType: data.itemType,
          weightKg: data is GearItemMetricData ? data.weightKg : null,
          material: data is GearItemMetricData ? data.materialName : null,
        ),
      'time' => DrillDownBuilder.buildTimeSavedDrillDown(
          impact,
          itemType: data.itemType,
        ),
      'social' || 'quality time' => DrillDownBuilder.buildQualityTimeDrillDown(
          impact,
          itemType: data.itemType,
        ),
      _ => null,
    };

    if (drillDown != null) {
      MetricDrillDownScreen.push(context, data: drillDown);
    }
  }
}
