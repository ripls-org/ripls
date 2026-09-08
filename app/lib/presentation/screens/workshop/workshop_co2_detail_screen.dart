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
import 'package:ripls/presentation/widgets/impact/equivalence/co2_daily_life_ladder.dart';
import 'package:ripls/presentation/widgets/impact/equivalence/equivalence_ladder_card.dart';
import 'package:ripls/presentation/widgets/impact/equivalence/equivalence_metric.dart';
import 'package:ripls/presentation/widgets/impact/impact_copy.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';
import 'package:ripls/presentation/widgets/workshop/recent_items_list.dart';
import 'package:ripls/services/providers/impact_providers.dart';

/// Aggregated CO₂ data computed client-side from per-community metric detail.
/// Bar points carry the bucket-start timestamp; axis labels resolve at build
/// time in the viewer's locale.
class _Co2Aggregate {
  final double totalKg;
  final List<({int sec, double value})> monthlyBars;
  final List<_SourceRow> sourceRows;
  final List<RecentActivity> recentItems;

  const _Co2Aggregate({
    required this.totalKg,
    required this.monthlyBars,
    required this.sourceRows,
    required this.recentItems,
  });
}

class _SourceRow {
  final ImpactSourceType type;
  final String formatted;
  final int count;
  final double valueKg;

  const _SourceRow({
    required this.type,
    required this.formatted,
    required this.count,
    required this.valueKg,
  });
}

/// WorkshopCo2DetailScreen shows a CO₂ avoided breakdown for the Workshop scope.
///
/// Reached by tapping the CO₂ impact row on the workshop home. Fetches
/// [GetCommunityMetricDetailResponse] for each selected community in parallel
/// and aggregates the results client-side:
/// - Total kg: sum of [EmissionsPreventedDetail.currentKg]
/// - Monthly bars: sum by month label
/// - Source breakdown: sum by source label (Loans, Giveaways, Requests, Events)
///
/// Sections (matching docs/cowork/App Design/co2-detail-field-guide.html):
/// 1. "The equivalent" — editorial serif headline + body paragraph
/// 2. Monthly bar chart of kg avoided
/// 3. "Where it came from" source breakdown
/// 4. "Recent contributors" — most recent items that drove this metric
/// 5. Methodology link
class WorkshopCo2DetailScreen extends ConsumerStatefulWidget {
  /// Community IDs forming the Workshop scope — one or more circles.
  final List<String> communityIds;

  /// When true, renders with a transparent scaffold so it overlays the
  /// community photo as a morph-reveal panel (caller forces dark theme + paints
  /// the backdrop). When false (default), a normal opaque screen.
  final bool overlay;

  const WorkshopCo2DetailScreen({
    super.key,
    required this.communityIds,
    this.overlay = false,
  });

  /// push slides the CO₂ detail screen in from the right.
  static Future<void> push(
    BuildContext context, {
    required List<String> communityIds,
  }) {
    return NavigationHelpers.pushScreen(
      context: context,
      screen: WorkshopCo2DetailScreen(
        communityIds: communityIds,
      ),
      useRootNavigator: true,
      routeName: 'workshop_co2_detail',
    );
  }

  @override
  ConsumerState<WorkshopCo2DetailScreen> createState() =>
      _WorkshopCo2DetailScreenState();
}

class _WorkshopCo2DetailScreenState
    extends ConsumerState<WorkshopCo2DetailScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  static const _accentColor = Color(0xFF4C8A4A);

  // Canonical display order for source breakdown rows.
  static const _sourceOrder = [
    ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS,
    ImpactSourceType.IMPACT_SOURCE_TYPE_GIVEAWAYS,
    ImpactSourceType.IMPACT_SOURCE_TYPE_REQUESTS,
    ImpactSourceType.IMPACT_SOURCE_TYPE_EVENTS,
  ];

  late Future<_Co2Aggregate> _aggregateFuture;

  @override
  void initState() {
    super.initState();
    _aggregateFuture = _loadAggregate();
  }

  Future<_Co2Aggregate> _loadAggregate() async {
    final repo = ref.read(impactMetricsRepositoryProvider);
    final responses = await Future.wait(
      widget.communityIds.map(
        (id) => repo.getMetricDetail(
          id,
          ImpactMetricDimension.IMPACT_METRIC_DIMENSION_EMISSIONS,
          ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
        ),
      ),
    );
    return _buildAggregate(responses);
  }

  static _Co2Aggregate _buildAggregate(
    List<GetCommunityMetricDetailResponse> responses,
  ) {
    final barAccum = <int, double>{};
    final barOrder = <int>[];
    final sourceAccum = <ImpactSourceType, double>{};
    final countAccum = <ImpactSourceType, int>{};
    final recentItems = <RecentActivity>[];

    for (final r in responses) {
      if (r.hasEmissionsPreventedDetail()) {
        final detail = r.emissionsPreventedDetail;
        for (final bar in detail.monthlyBars) {
          // monthly_bars values are in grams; convert to kg.
          final sec = bar.bucketStartUnixSec.toInt();
          if (!barAccum.containsKey(sec)) barOrder.add(sec);
          barAccum[sec] = (barAccum[sec] ?? 0) + bar.value / 1000;
        }
      }
      for (final s in r.sourceBreakdown) {
        // SourceBreakdown.value is in grams; convert to kg for display.
        sourceAccum[s.sourceType] =
            (sourceAccum[s.sourceType] ?? 0) + s.value / 1000;
        countAccum[s.sourceType] =
            (countAccum[s.sourceType] ?? 0) + (s.hasCount() ? s.count : 0);
      }
      recentItems.addAll(r.recentItems);
    }

    // Derive the headline total from the source breakdown so the
    // "Where it came from" rows always sum to the headline number.
    final totalKg = sourceAccum.values.fold<double>(0, (a, b) => a + b);

    final monthlyBars =
        barOrder.map((sec) => (sec: sec, value: barAccum[sec]!)).toList();

    // Always include Events even at zero; filter other sources at zero.
    // Sort descending by kg value so the largest contributor appears first.
    final sourceRows = _sourceOrder
        .where((t) =>
            t == ImpactSourceType.IMPACT_SOURCE_TYPE_EVENTS ||
            (sourceAccum[t] ?? 0) > 0)
        .map((t) => _SourceRow(
              type: t,
              formatted: _formatKg(sourceAccum[t] ?? 0),
              count: countAccum[t] ?? 0,
              valueKg: sourceAccum[t] ?? 0,
            ))
        .toList()
      ..sort((a, b) => b.valueKg.compareTo(a.valueKg));

    return _Co2Aggregate(
      totalKg: totalKg,
      monthlyBars: monthlyBars,
      sourceRows: sourceRows,
      recentItems: recentItems,
    );
  }

  static String _formatKg(double kg) {
    if (kg < 1) return '${(kg * 1000).round()} g';
    return '${kg.round()} kg';
  }

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<_Co2Aggregate>(
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

  Widget _buildTitle(BuildContext context, _Co2Aggregate? aggregate) {
    final kgPart = aggregate != null
        ? _formatKg(aggregate.totalKg).toUpperCase()
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
          const TextSpan(text: 'CO\u2082 AVOIDED \u00b7 '),
          TextSpan(
            text: kgPart,
            style: const TextStyle(fontWeight: FontWeight.w700),
          ),
          const TextSpan(text: ' \u00b7 ALL-TIME'),
        ],
      ),
    );
  }

  Widget _buildBody(BuildContext context, _Co2Aggregate aggregate) {
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

        // Monthly bar chart.
        if (aggregate.monthlyBars.isNotEmpty)
          SliverToBoxAdapter(
            child: Padding(
              padding: const EdgeInsets.fromLTRB(20, 32, 20, 0),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  _sectionHeader(
                      context, context.l10n.workshopCo2DetailChartHeader),
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
                    valueFormatter: (v) => '${v.round()} kg',
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

        // "Keep the momentum" intro — section header + serif headline + subtitle.
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
                  ImpactMetricDimension.IMPACT_METRIC_DIMENSION_EMISSIONS,
            ),
          ),
        ],

        // Bottom padding.
        const SliverToBoxAdapter(child: SizedBox(height: 40)),
      ],
    ),
    );
  }

  Widget _buildEquivalent(BuildContext context, _Co2Aggregate aggregate) {
    final l10n = context.l10n;
    return LadderEquivalentSection(
      ladder: kCo2DailyLifeLadder,
      metric: EquivalenceMetric.co2Avoided,
      surface: EquivalenceSurface.workshopDetail,
      value: aggregate.totalKg,
      communitySize: widget.communityIds.length,
      sectionHeader: l10n.workshopCo2DetailEquivalentHeader,
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
      fallbackBody: l10n.equivalenceCo2FallbackBody,
    );
  }

  Widget _buildBreakdown(BuildContext context, List<_SourceRow> rows) {
    final l10n = context.l10n;
    final dividerColor = AppColors.border(context);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _sectionHeader(context, l10n.workshopCo2DetailBreakdownHeader),
        const SizedBox(height: 12),
        // Column headers.
        Row(
          children: [
            Expanded(
              child: Text(
                l10n.workshopCo2DetailBreakdownColSource,
                style: _colHeaderStyle(context),
              ),
            ),
            SizedBox(
              width: 60,
              child: Text(
                l10n.workshopCo2DetailBreakdownColItems,
                textAlign: TextAlign.right,
                style: _colHeaderStyle(context),
              ),
            ),
            SizedBox(
              width: 88,
              child: Text(
                l10n.workshopCo2DetailBreakdownColCo2,
                textAlign: TextAlign.right,
                style: _colHeaderStyle(context),
              ),
            ),
          ],
        ),
        const SizedBox(height: 4),
        for (int i = 0; i < rows.length; i++) ...[
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
