import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/impact_service.pb.dart'
    show ImpactSourceType, RecentActivity;
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/presentation/widgets/impact/community/impact_chart_card.dart';
import 'package:ripls/presentation/widgets/impact/equivalence/equivalence_ladder_card.dart';
import 'package:ripls/presentation/widgets/impact/equivalence/equivalence_metric.dart';
import 'package:ripls/presentation/widgets/impact/equivalence/time_health_ladder.dart';
import 'package:ripls/presentation/widgets/impact/impact_copy.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';
import 'package:ripls/presentation/widgets/workshop/recent_items_list.dart';
import 'package:ripls/services/providers/impact_providers.dart';

/// Aggregated quality-time data computed client-side from per-community
/// metric detail responses. All time values are stored in minutes. Bar
/// points carry the bucket-start timestamp; axis labels resolve at build
/// time in the viewer's locale.
class _TimeAggregate {
  final double totalMinutes;
  final List<({int sec, double value})> monthlyBars;
  final List<_SourceRow> sourceRows;
  final List<RecentActivity> recentItems;

  const _TimeAggregate({
    required this.totalMinutes,
    required this.monthlyBars,
    required this.sourceRows,
    required this.recentItems,
  });
}

class _SourceRow {
  final ImpactSourceType type;
  final String formatted;
  final int count;
  final double valueMinutes;

  const _SourceRow({
    required this.type,
    required this.formatted,
    required this.count,
    required this.valueMinutes,
  });
}

/// WorkshopTimeDetailScreen shows a "time together" breakdown for the
/// Workshop scope. Reached by tapping the time impact row on the
/// workshop home.
///
/// Mirrors [WorkshopCo2DetailScreen]'s structure:
/// 1. "The equivalent" — walk-tier serif headline + body
/// 2. Monthly bar chart of hours together
/// 3. "Where it came from" source breakdown
///    (server folds Giveaways into Requests for the TIME dimension, so only
///    Loans, Requests, and Events appear)
/// 4. "Keep the momentum" — reused lean-card rows
class WorkshopTimeDetailScreen extends ConsumerStatefulWidget {
  final List<String> communityIds;

  /// When true, renders with a transparent scaffold so it overlays the
  /// community photo as a morph-reveal panel (the caller forces dark theme +
  /// paints the backdrop). When false (default), a normal opaque screen.
  final bool overlay;

  const WorkshopTimeDetailScreen({
    super.key,
    required this.communityIds,
    this.overlay = false,
  });

  /// push slides the Time Together detail screen in from the right.
  static Future<void> push(
    BuildContext context, {
    required List<String> communityIds,
  }) {
    return NavigationHelpers.pushScreen(
      context: context,
      screen: WorkshopTimeDetailScreen(
        communityIds: communityIds,
      ),
      useRootNavigator: true,
      routeName: 'workshop_time_detail',
    );
  }

  @override
  ConsumerState<WorkshopTimeDetailScreen> createState() =>
      _WorkshopTimeDetailScreenState();
}

class _WorkshopTimeDetailScreenState
    extends ConsumerState<WorkshopTimeDetailScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  static const _accentColor = Color(0xFF1565C0);

  // Server folds Giveaways into Requests for the TIME dimension; Giveaways
  // simply won't appear. The list keeps the canonical order regardless.
  static const _sourceOrder = [
    ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS,
    ImpactSourceType.IMPACT_SOURCE_TYPE_GIVEAWAYS,
    ImpactSourceType.IMPACT_SOURCE_TYPE_REQUESTS,
    ImpactSourceType.IMPACT_SOURCE_TYPE_EVENTS,
  ];

  late Future<_TimeAggregate> _aggregateFuture;

  @override
  void initState() {
    super.initState();
    _aggregateFuture = _loadAggregate();
  }

  Future<_TimeAggregate> _loadAggregate() async {
    final repo = ref.read(impactMetricsRepositoryProvider);
    final responses = await Future.wait(
      widget.communityIds.map(
        (id) => repo.getMetricDetail(
          id,
          ImpactMetricDimension.IMPACT_METRIC_DIMENSION_QUALITY_TIME,
          ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
        ),
      ),
    );
    return _buildAggregate(responses);
  }

  static _TimeAggregate _buildAggregate(
    List<GetCommunityMetricDetailResponse> responses,
  ) {
    final barAccum = <int, double>{};
    final barOrder = <int>[];
    final sourceAccum = <ImpactSourceType, double>{};
    final countAccum = <ImpactSourceType, int>{};
    final recentItems = <RecentActivity>[];

    for (final r in responses) {
      if (r.hasQualityTimeDetail()) {
        for (final bar in r.qualityTimeDetail.monthlyBars) {
          final sec = bar.bucketStartUnixSec.toInt();
          if (!barAccum.containsKey(sec)) barOrder.add(sec);
          barAccum[sec] = (barAccum[sec] ?? 0) + bar.value;
        }
      }
      for (final s in r.sourceBreakdown) {
        // SourceBreakdown.value for QUALITY_TIME is in minutes.
        sourceAccum[s.sourceType] = (sourceAccum[s.sourceType] ?? 0) + s.value;
        countAccum[s.sourceType] =
            (countAccum[s.sourceType] ?? 0) + (s.hasCount() ? s.count : 0);
      }
      recentItems.addAll(r.recentItems);
    }

    // Derive the headline total from the source breakdown so the
    // "Where it came from" rows always sum to the headline number.
    final totalMinutes = sourceAccum.values.fold<double>(0, (a, b) => a + b);

    final monthlyBars =
        barOrder.map((sec) => (sec: sec, value: barAccum[sec]!)).toList();

    // Always include Events even at zero; filter other sources at zero.
    // Sort descending by minutes so the largest contributor appears first.
    final sourceRows = _sourceOrder
        .where((t) =>
            t == ImpactSourceType.IMPACT_SOURCE_TYPE_EVENTS ||
            (sourceAccum[t] ?? 0) > 0)
        .map((t) => _SourceRow(
              type: t,
              formatted: _formatDuration(sourceAccum[t] ?? 0),
              count: countAccum[t] ?? 0,
              valueMinutes: sourceAccum[t] ?? 0,
            ))
        .toList()
      ..sort((a, b) => b.valueMinutes.compareTo(a.valueMinutes));

    return _TimeAggregate(
      totalMinutes: totalMinutes,
      monthlyBars: monthlyBars,
      sourceRows: sourceRows,
      recentItems: recentItems,
    );
  }

  /// Formats minutes for display: "45 min", "3 hr", "31 hr".
  static String _formatDuration(double mins) {
    if (mins < 60) return '${mins.round()} min';
    return '${(mins / 60).round()} hr';
  }

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<_TimeAggregate>(
      future: _aggregateFuture,
      builder: (context, snap) {
        final aggregate = snap.data;
        final scaffoldBg =
            widget.overlay ? Colors.transparent : AppColors.background(context);
        final appBar = AppBar(
          automaticallyImplyLeading: false,
          leading:
              widget.overlay ? null : AppBarBackButton(onPressed: handleClose),
          title: _buildTitle(context, aggregate),
          actions: widget.overlay
              ? [
                  IconAction(
                    icon: Icons.close_rounded,
                    semanticsLabel: context.l10n.a11yClose,
                    onPressed: () => Navigator.of(context).pop(),
                  ),
                  const SizedBox(width: 4),
                ]
              : null,
          backgroundColor: scaffoldBg,
          foregroundColor: AppColors.textPrimary(context),
          elevation: 0,
          scrolledUnderElevation: 0,
        );
        final body = switch (snap.connectionState) {
          ConnectionState.done when snap.hasError => Center(
              child: Padding(
                padding: const EdgeInsets.all(24),
                child: Text(
                  snap.error.toString(),
                  style: TextStyle(color: AppColors.textSecondary(context)),
                ),
              ),
            ),
          ConnectionState.done => _buildBody(context, aggregate!),
          _ => const Center(child: CircularProgressIndicator()),
        };
        // overlay: plain transparent Scaffold (no swipe-slide, which would
        // fight the morph grow); the AppBar close still pops → morph reverse.
        if (widget.overlay) {
          return Scaffold(
            backgroundColor: Colors.transparent,
            appBar: appBar,
            body: body,
          );
        }
        return buildSwipeableScaffold(
          backgroundColor: scaffoldBg,
          appBar: appBar,
          body: body,
        );
      },
    );
  }

  Widget _buildTitle(BuildContext context, _TimeAggregate? aggregate) {
    final durationPart = aggregate != null
        ? _formatDuration(aggregate.totalMinutes).toUpperCase()
        : '\u2026';
    final baseStyle = TextStyle(
      fontFamily: AppTheme.headingFont,
      fontSize: 12,
      letterSpacing: 0.8,
      color: AppColors.textPrimary(context),
    );
    return Text.rich(
      TextSpan(
        style: baseStyle,
        children: [
          const TextSpan(text: 'TIME TOGETHER \u00b7 '),
          TextSpan(
            text: durationPart,
            style: const TextStyle(fontWeight: FontWeight.w700),
          ),
          const TextSpan(text: ' \u00b7 ALL-TIME'),
        ],
      ),
    );
  }

  Widget _buildBody(BuildContext context, _TimeAggregate aggregate) {
    final hasMomentum = aggregate.recentItems.isNotEmpty;

    return DefaultTextStyle.merge(
      style: const TextStyle(fontFamily: AppTheme.headingFont),
      child: CustomScrollView(
        slivers: [
          // Editorial equivalence section.
          SliverToBoxAdapter(
            child: Padding(
              padding: const EdgeInsets.fromLTRB(20, 24, 20, 0),
              child: _buildEquivalent(context, aggregate),
            ),
          ),

          // Monthly bar chart (hours).
          if (aggregate.monthlyBars.isNotEmpty)
            SliverToBoxAdapter(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(20, 32, 20, 0),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    _sectionHeader(
                        context, context.l10n.workshopTimeDetailChartHeader),
                    const SizedBox(height: 16),
                    ImpactChartCard(
                      title: '',
                      data: [
                        for (final p in aggregate.monthlyBars)
                          ChartDataPoint(
                            label: impactMonthLabel(context.l10n, p.sec),
                            value: p.value,
                          ),
                      ],
                      accentColor: _accentColor,
                      chartType: ChartType.bar,
                      showYAxisLabels: true,
                      valueFormatter: _formatDuration,
                      backgroundColor: Colors.transparent,
                    ),
                  ],
                ),
              ),
            ),

          // Source breakdown.
          if (aggregate.sourceRows.isNotEmpty)
            SliverToBoxAdapter(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(20, 32, 20, 0),
                child: _buildBreakdown(context, aggregate.sourceRows),
              ),
            ),

          if (hasMomentum) ...[
            SliverToBoxAdapter(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(20, 32, 20, 0),
                child: _buildMomentumIntro(context),
              ),
            ),
            SliverToBoxAdapter(
              child: RecentItemsList(
                items: aggregate.recentItems,
                dimension:
                    ImpactMetricDimension.IMPACT_METRIC_DIMENSION_QUALITY_TIME,
              ),
            ),
          ],

          const SliverToBoxAdapter(child: SizedBox(height: 40)),
        ],
      ),
    );
  }

  Widget _buildEquivalent(BuildContext context, _TimeAggregate aggregate) {
    final l10n = context.l10n;
    return LadderEquivalentSection(
      ladder: kTimeHealthLadder,
      metric: EquivalenceMetric.timeTogether,
      surface: EquivalenceSurface.workshopDetail,
      value: aggregate.totalMinutes / 60,
      communitySize: widget.communityIds.length,
      sectionHeader: l10n.workshopTimeDetailEquivalentHeader,
      sectionHeaderStyle: TextStyle(
        fontSize: 10,
        fontWeight: FontWeight.w700,
        letterSpacing: 1.8,
        color: AppColors.primary(context),
      ),
      headlineStyle: TextStyle(
        fontFamily: AppTheme.headingFont,
        fontSize: 26,
        fontWeight: FontWeight.w700,
        color: AppColors.textPrimary(context),
        height: 1.2,
      ),
      bodyStyle: TextStyle(
        fontSize: 16,
        color: AppColors.textPrimary(context),
        height: 1.55,
      ),
      accentColor: _accentColor,
      fallbackBody: l10n.equivalenceTimeFallbackBody,
    );
  }

  Widget _buildBreakdown(BuildContext context, List<_SourceRow> rows) {
    final l10n = context.l10n;
    final dividerColor = AppColors.border(context);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _sectionHeader(context, l10n.workshopTimeDetailBreakdownHeader),
        const SizedBox(height: 12),
        Row(
          children: [
            Expanded(
              child: Text(
                l10n.workshopTimeDetailBreakdownColSource,
                style: _colHeaderStyle(context),
              ),
            ),
            SizedBox(
              width: 60,
              child: Text(
                l10n.workshopTimeDetailBreakdownColItems,
                textAlign: TextAlign.right,
                style: _colHeaderStyle(context),
              ),
            ),
            SizedBox(
              width: 88,
              child: Text(
                l10n.workshopTimeDetailBreakdownColTime,
                textAlign: TextAlign.right,
                style: _colHeaderStyle(context),
              ),
            ),
          ],
        ),
        const SizedBox(height: 4),
        for (var i = 0; i < rows.length; i++) ...[
          Divider(height: 1, thickness: 1, color: dividerColor),
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 13),
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    impactSourceLabel(l10n, rows[i].type),
                    style: TextStyle(
                      fontSize: 15,
                      color: AppColors.textPrimary(context),
                    ),
                  ),
                ),
                SizedBox(
                  width: 60,
                  child: Text(
                    rows[i].count > 0 ? '${rows[i].count}' : '',
                    textAlign: TextAlign.right,
                    style: TextStyle(
                      fontSize: 15,
                      color: AppColors.textSecondary(context),
                    ),
                  ),
                ),
                SizedBox(
                  width: 88,
                  child: Text(
                    rows[i].formatted,
                    textAlign: TextAlign.right,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w600,
                      color: AppColors.primary(context),
                    ),
                  ),
                ),
              ],
            ),
          ),
        ],
        Divider(height: 1, thickness: 1, color: dividerColor),
      ],
    );
  }

  Widget _buildMomentumIntro(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _sectionHeader(context, context.l10n.workshopRecentContributionsHeader),
        const SizedBox(height: 16),
      ],
    );
  }

  TextStyle _colHeaderStyle(BuildContext context) => TextStyle(
        fontSize: 9,
        fontWeight: FontWeight.w600,
        letterSpacing: 1.2,
        color: AppColors.textTertiary(context),
      );

  Widget _sectionHeader(BuildContext context, String text) {
    return Text(
      text,
      style: TextStyle(
        fontSize: 10,
        fontWeight: FontWeight.w700,
        letterSpacing: 1.8,
        color: AppColors.primary(context),
      ),
    );
  }
}
