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
import 'package:ripls/presentation/widgets/impact/equivalence/money_paycheck_ladder.dart';
import 'package:ripls/presentation/widgets/impact/impact_copy.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';
import 'package:ripls/presentation/widgets/workshop/recent_items_list.dart';
import 'package:ripls/services/providers/impact_providers.dart';

/// Aggregated money-saved data computed client-side from per-community
/// metric detail responses. Trend points carry the bucket-start timestamp;
/// axis labels resolve at build time in the viewer's locale.
class _MoneyAggregate {
  final double totalUsd;
  final List<({int sec, double value})> cumulativeTrend;
  final List<_SourceRow> sourceRows;
  final List<RecentActivity> recentItems;

  const _MoneyAggregate({
    required this.totalUsd,
    required this.cumulativeTrend,
    required this.sourceRows,
    required this.recentItems,
  });
}

class _SourceRow {
  final ImpactSourceType type;
  final String formatted;
  final int count;
  final double valueUsd;

  const _SourceRow({
    required this.type,
    required this.formatted,
    required this.count,
    required this.valueUsd,
  });
}

/// WorkshopMoneyDetailScreen shows a dollar-savings breakdown for the
/// Workshop scope. Reached by tapping the money impact row on the workshop home.
///
/// Structure:
/// 1. "The equivalent" — meal-tier serif headline + body
/// 2. Cumulative-dollars line chart
/// 3. "Where it came from" source breakdown (Loans, Giveaways, Requests, Events)
/// 4. "Recent contributors" — most recent items that drove this metric
class WorkshopMoneyDetailScreen extends ConsumerStatefulWidget {
  final List<String> communityIds;

  /// When true, renders with a transparent scaffold so it overlays the
  /// community photo as a morph-reveal panel (caller forces dark theme + paints
  /// the backdrop). When false (default), a normal opaque screen.
  final bool overlay;

  const WorkshopMoneyDetailScreen({
    super.key,
    required this.communityIds,
    this.overlay = false,
  });

  /// push slides the Money Saved detail screen in from the right.
  static Future<void> push(
    BuildContext context, {
    required List<String> communityIds,
  }) {
    return NavigationHelpers.pushScreen(
      context: context,
      screen: WorkshopMoneyDetailScreen(
        communityIds: communityIds,
      ),
      useRootNavigator: true,
      routeName: 'workshop_money_detail',
    );
  }

  @override
  ConsumerState<WorkshopMoneyDetailScreen> createState() =>
      _WorkshopMoneyDetailScreenState();
}

class _WorkshopMoneyDetailScreenState
    extends ConsumerState<WorkshopMoneyDetailScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  static const _accentColor = Color(0xFF2E7D32);
  static const _sourceOrder = [
    ImpactSourceType.IMPACT_SOURCE_TYPE_LOANS,
    ImpactSourceType.IMPACT_SOURCE_TYPE_GIVEAWAYS,
    ImpactSourceType.IMPACT_SOURCE_TYPE_REQUESTS,
    ImpactSourceType.IMPACT_SOURCE_TYPE_EVENTS,
  ];

  late Future<_MoneyAggregate> _aggregateFuture;

  @override
  void initState() {
    super.initState();
    _aggregateFuture = _loadAggregate();
  }

  Future<_MoneyAggregate> _loadAggregate() async {
    final repo = ref.read(impactMetricsRepositoryProvider);
    final responses = await Future.wait(
      widget.communityIds.map(
        (id) => repo.getMetricDetail(
          id,
          ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
          ImpactMetricPeriod.IMPACT_METRIC_PERIOD_ALL,
        ),
      ),
    );
    return _buildAggregate(responses);
  }

  static _MoneyAggregate _buildAggregate(
    List<GetCommunityMetricDetailResponse> responses,
  ) {
    final trendAccum = <int, double>{};
    final trendOrder = <int>[];
    final sourceAccum = <ImpactSourceType, double>{};
    final countAccum = <ImpactSourceType, int>{};
    final recentItems = <RecentActivity>[];

    for (final r in responses) {
      // Top-level cumulativeTrend; for MONEY this is cumulative dollars over
      // time. Buckets align across communities, so accumulate by the typed
      // bucket-start timestamp.
      for (final p in r.cumulativeTrend) {
        final sec = p.bucketStartUnixSec.toInt();
        if (!trendAccum.containsKey(sec)) trendOrder.add(sec);
        trendAccum[sec] = (trendAccum[sec] ?? 0) + p.value;
      }
      for (final s in r.sourceBreakdown) {
        // SourceBreakdown.value for MONEY is in USD.
        sourceAccum[s.sourceType] = (sourceAccum[s.sourceType] ?? 0) + s.value;
        countAccum[s.sourceType] =
            (countAccum[s.sourceType] ?? 0) + (s.hasCount() ? s.count : 0);
      }
      recentItems.addAll(r.recentItems);
    }

    // Derive the headline total from the source breakdown so the
    // "Where it came from" rows always sum to the headline number.
    final totalUsd = sourceAccum.values.fold<double>(0, (a, b) => a + b);

    final cumulativeTrend =
        trendOrder.map((sec) => (sec: sec, value: trendAccum[sec]!)).toList();

    // Always include Events even at zero; filter other sources at zero.
    // Sort descending by USD so the largest contributor appears first.
    final sourceRows = _sourceOrder
        .where((t) =>
            t == ImpactSourceType.IMPACT_SOURCE_TYPE_EVENTS ||
            (sourceAccum[t] ?? 0) > 0)
        .map((t) => _SourceRow(
              type: t,
              formatted: _formatUsd(sourceAccum[t] ?? 0),
              count: countAccum[t] ?? 0,
              valueUsd: sourceAccum[t] ?? 0,
            ))
        .toList()
      ..sort((a, b) => b.valueUsd.compareTo(a.valueUsd));

    return _MoneyAggregate(
      totalUsd: totalUsd,
      cumulativeTrend: cumulativeTrend,
      sourceRows: sourceRows,
      recentItems: recentItems,
    );
  }

  /// Formats a dollar amount for display: "$1,241", "$48", "$0".
  static String _formatUsd(double dollars) {
    final n = dollars.round();
    if (n.abs() < 1000) return '\$$n';
    final s = n.abs().toString();
    final buf = StringBuffer();
    for (var i = 0; i < s.length; i++) {
      if (i > 0 && (s.length - i) % 3 == 0) buf.write(',');
      buf.write(s[i]);
    }
    return '${n < 0 ? '-' : ''}\$${buf.toString()}';
  }

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<_MoneyAggregate>(
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

  Widget _buildTitle(BuildContext context, _MoneyAggregate? aggregate) {
    final amountPart = aggregate != null
        ? _formatUsd(aggregate.totalUsd)
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
          const TextSpan(text: '\$ SAVED \u00b7 '),
          TextSpan(
            text: amountPart,
            style: const TextStyle(fontWeight: FontWeight.w700),
          ),
          const TextSpan(text: ' \u00b7 ALL-TIME'),
        ],
      ),
    );
  }

  Widget _buildBody(BuildContext context, _MoneyAggregate aggregate) {
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

          // Cumulative dollars line chart.
          if (aggregate.cumulativeTrend.isNotEmpty)
            SliverToBoxAdapter(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(20, 32, 20, 0),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    _sectionHeader(
                        context, context.l10n.workshopMoneyDetailChartHeader),
                    const SizedBox(height: 16),
                    ImpactChartCard(
                      title: '',
                      data: [
                        for (final p in aggregate.cumulativeTrend)
                          ChartDataPoint(
                            label: impactMonthLabel(context.l10n, p.sec),
                            value: p.value,
                          ),
                      ],
                      accentColor: _accentColor,
                      chartType: ChartType.line,
                      showYAxisLabels: true,
                      valueFormatter: _formatUsd,
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
                dimension: ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
              ),
            ),
          ],

          const SliverToBoxAdapter(child: SizedBox(height: 40)),
        ],
      ),
    );
  }

  Widget _buildEquivalent(BuildContext context, _MoneyAggregate aggregate) {
    final l10n = context.l10n;
    return LadderEquivalentSection(
      ladder: kMoneyPaycheckLadder,
      metric: EquivalenceMetric.moneySaved,
      surface: EquivalenceSurface.workshopDetail,
      value: aggregate.totalUsd,
      communitySize: widget.communityIds.length,
      sectionHeader: l10n.workshopMoneyDetailEquivalentHeader,
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
      fallbackBody: l10n.equivalenceMoneyFallbackBody,
    );
  }

  Widget _buildBreakdown(BuildContext context, List<_SourceRow> rows) {
    final l10n = context.l10n;
    final dividerColor = AppColors.border(context);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _sectionHeader(context, l10n.workshopMoneyDetailBreakdownHeader),
        const SizedBox(height: 12),
        Row(
          children: [
            Expanded(
              child: Text(
                l10n.workshopMoneyDetailBreakdownColSource,
                style: _colHeaderStyle(context),
              ),
            ),
            SizedBox(
              width: 60,
              child: Text(
                l10n.workshopMoneyDetailBreakdownColItems,
                textAlign: TextAlign.right,
                style: _colHeaderStyle(context),
              ),
            ),
            SizedBox(
              width: 88,
              child: Text(
                l10n.workshopMoneyDetailBreakdownColMoney,
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
