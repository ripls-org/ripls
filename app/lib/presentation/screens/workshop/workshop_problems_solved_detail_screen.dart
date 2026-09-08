import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/repositories/impact_repository.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/presentation/widgets/impact/community/impact_chart_card.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';
import 'package:ripls/services/providers/impact_providers.dart';

/// Aggregated "problems handled" data across the workshop scope, as an X-of-Y
/// fraction per bucket: handled (completed loans + fulfilled requests + claimed
/// needs) over potential (all non-cancelled loans / requests / posted needs).
class _ProblemsAggregate {
  final int handledLoans;
  final int potentialLoans;
  final int handledRequests;
  final int potentialRequests;
  final int handledNeeds;
  final int potentialNeeds;
  final List<ProblemSolvedItem> items;

  /// Handled problems bucketed by month (continuous from the earliest to the
  /// latest handled entry), for the monthly bar chart.
  final List<ChartDataPoint> monthlyBars;

  const _ProblemsAggregate({
    required this.handledLoans,
    required this.potentialLoans,
    required this.handledRequests,
    required this.potentialRequests,
    required this.handledNeeds,
    required this.potentialNeeds,
    required this.items,
    required this.monthlyBars,
  });

  int get handled => handledLoans + handledRequests + handledNeeds;
  int get potential => potentialLoans + potentialRequests + potentialNeeds;
}

/// Deep-dive for the Workshop "X of Y problems" metric — completed requests,
/// completed loans, and claimed needs over the full universe of requests. Reached
/// by tapping the problems figure in the overview's metrics sentence. Mirrors
/// the time/money/CO₂ detail screens' overlay scaffold.
class WorkshopProblemsSolvedDetailScreen extends ConsumerStatefulWidget {
  final List<String> communityIds;

  /// When true, renders with a transparent scaffold so it overlays the
  /// community photo as a morph-reveal panel (the caller forces dark theme +
  /// paints the backdrop). When false (default), a normal opaque screen.
  final bool overlay;

  const WorkshopProblemsSolvedDetailScreen({
    super.key,
    required this.communityIds,
    this.overlay = false,
  });

  @override
  ConsumerState<WorkshopProblemsSolvedDetailScreen> createState() =>
      _WorkshopProblemsSolvedDetailScreenState();
}

class _WorkshopProblemsSolvedDetailScreenState
    extends ConsumerState<WorkshopProblemsSolvedDetailScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  static const _accentColor = Color(0xFF3E6B4E);

  late Future<_ProblemsAggregate> _future;

  @override
  void initState() {
    super.initState();
    _future = _load();
  }

  Future<_ProblemsAggregate> _load() async {
    final repo = ref.read(impactMetricsRepositoryProvider);
    final responses = await Future.wait(
      widget.communityIds.map(repo.getProblemsSolvedDetail),
    );
    var handledLoans = 0, potentialLoans = 0;
    var handledRequests = 0, potentialRequests = 0;
    var handledNeeds = 0, potentialNeeds = 0;
    final items = <ProblemSolvedItem>[];
    for (final r in responses) {
      handledLoans += r.handledLoans;
      potentialLoans += r.potentialLoans;
      handledRequests += r.handledRequests;
      potentialRequests += r.potentialRequests;
      handledNeeds += r.handledNeeds;
      potentialNeeds += r.potentialNeeds;
      items.addAll(r.items);
    }
    items.sort(
      (a, b) => b.completedAtUnixSec.compareTo(a.completedAtUnixSec),
    );
    return _ProblemsAggregate(
      handledLoans: handledLoans,
      potentialLoans: potentialLoans,
      handledRequests: handledRequests,
      potentialRequests: potentialRequests,
      handledNeeds: handledNeeds,
      potentialNeeds: potentialNeeds,
      items: items,
      monthlyBars: _buildMonthlyBars(items),
    );
  }

  /// Buckets handled entries by calendar month and fills every month between
  /// the earliest and latest so the bar chart reads continuously — mirroring
  /// the server's monthly-acts chart. Entries without a timestamp are skipped.
  static List<ChartDataPoint> _buildMonthlyBars(List<ProblemSolvedItem> items) {
    final counts = <DateTime, int>{};
    for (final item in items) {
      final secs = item.completedAtUnixSec.toInt();
      if (secs <= 0) continue;
      final d = DateTime.fromMillisecondsSinceEpoch(secs * 1000);
      final key = DateTime(d.year, d.month);
      counts[key] = (counts[key] ?? 0) + 1;
    }
    if (counts.isEmpty) return const [];

    final keys = counts.keys.toList()..sort();
    final latest = keys.last;
    final thisYear = DateTime.now().year;
    final bars = <ChartDataPoint>[];
    for (var cursor = keys.first;
        !cursor.isAfter(latest);
        cursor = DateTime(cursor.year, cursor.month + 1)) {
      // Current year drops the year suffix ("Mar"); prior years keep it
      // ("Mar 24"), matching the other metric charts.
      final label = cursor.year == thisYear
          ? DateFormat('MMM').format(cursor)
          : DateFormat('MMM yy').format(cursor);
      bars.add(
        ChartDataPoint(label: label, value: (counts[cursor] ?? 0).toDouble()),
      );
    }
    return bars;
  }

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<_ProblemsAggregate>(
      future: _future,
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

  Widget _buildTitle(BuildContext context, _ProblemsAggregate? aggregate) {
    final countPart = aggregate != null
        ? '${aggregate.handled} / ${aggregate.potential}'
        : '…';
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
          TextSpan(text: '${context.l10n.workshopProblemsDetailTitle.toUpperCase()} · '),
          TextSpan(
            text: countPart,
            style: const TextStyle(fontWeight: FontWeight.w700),
          ),
          const TextSpan(text: ' · ALL-TIME'),
        ],
      ),
    );
  }

  Widget _buildBody(BuildContext context, _ProblemsAggregate aggregate) {
    final l10n = context.l10n;
    if (aggregate.potential == 0) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Text(
            l10n.workshopProblemsDetailEmpty,
            textAlign: TextAlign.center,
            style: TextStyle(
              fontFamily: AppTheme.headingFont,
              fontSize: 16,
              color: AppColors.textSecondary(context),
            ),
          ),
        ),
      );
    }

    return DefaultTextStyle.merge(
      style: const TextStyle(fontFamily: AppTheme.headingFont),
      child: CustomScrollView(
        slivers: [
          // Monthly bar chart of problems handled over time.
          if (aggregate.monthlyBars.isNotEmpty)
            SliverToBoxAdapter(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(20, 24, 20, 0),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    _sectionHeader(
                      context,
                      l10n.workshopProblemsDetailChartHeader,
                    ),
                    const SizedBox(height: 16),
                    ImpactChartCard(
                      title: '',
                      data: aggregate.monthlyBars,
                      accentColor: _accentColor,
                      chartType: ChartType.bar,
                      showYAxisLabels: true,
                      valueFormatter: (v) => '${v.round()}',
                      backgroundColor: Colors.transparent,
                    ),
                  ],
                ),
              ),
            ),
          // Bucket breakdown — top spacing matches whichever section precedes
          // it (the chart when present, otherwise it's the first section).
          SliverToBoxAdapter(
            child: Padding(
              padding: EdgeInsets.fromLTRB(
                20,
                aggregate.monthlyBars.isNotEmpty ? 32 : 24,
                20,
                0,
              ),
              child: _buildBreakdown(context, aggregate),
            ),
          ),
          SliverToBoxAdapter(
            child: Padding(
              padding: const EdgeInsets.fromLTRB(20, 32, 20, 12),
              child: _sectionHeader(
                context,
                l10n.workshopProblemsDetailListHeader,
              ),
            ),
          ),
          SliverList.builder(
            itemCount: aggregate.items.length,
            itemBuilder: (context, i) => _ItemRow(item: aggregate.items[i]),
          ),
          const SliverToBoxAdapter(child: SizedBox(height: 40)),
        ],
      ),
    );
  }

  Widget _buildBreakdown(BuildContext context, _ProblemsAggregate a) {
    final l10n = context.l10n;
    final dividerColor = AppColors.border(context);
    // Each row is "handled / potential" for one bucket.
    final rows = <(String, int, int)>[
      (l10n.workshopProblemsBucketRequests, a.handledRequests, a.potentialRequests),
      (l10n.workshopProblemsBucketLoans, a.handledLoans, a.potentialLoans),
      (l10n.workshopProblemsBucketNeeds, a.handledNeeds, a.potentialNeeds),
    ];
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _sectionHeader(context, l10n.workshopProblemsDetailBreakdownHeader),
        const SizedBox(height: 12),
        for (final (label, handled, potential) in rows) ...[
          Divider(height: 1, thickness: 1, color: dividerColor),
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 13),
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    label,
                    style: TextStyle(
                      fontSize: 15,
                      color: AppColors.textPrimary(context),
                    ),
                  ),
                ),
                Text(
                  '$handled / $potential',
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w600,
                    color: AppColors.primary(context),
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

  // Matches the quality-time detail screen's section headers (theme primary,
  // not a per-screen accent).
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

/// One completed "problem solved" row: title + "kind · person · date",
/// tappable to open the underlying gear / request / experience.
class _ItemRow extends StatelessWidget {
  final ProblemSolvedItem item;
  const _ItemRow({required this.item});

  String _kindLabel(BuildContext context) {
    final l10n = context.l10n;
    return switch (item.kind) {
      ProblemSolvedKind.PROBLEM_SOLVED_KIND_LOAN => l10n.workshopProblemsKindLoan,
      ProblemSolvedKind.PROBLEM_SOLVED_KIND_REQUEST =>
        l10n.workshopProblemsKindRequest,
      ProblemSolvedKind.PROBLEM_SOLVED_KIND_NEED =>
        l10n.workshopProblemsKindNeed,
      _ => '',
    };
  }

  void _open(BuildContext context) {
    // The server sets content_type ("gear"/"request"/"experience") since a
    // need's parent may be either an experience or a request.
    final route = item.contentType;
    if (route.isEmpty || item.contentId.isEmpty) return;
    NavigationHelpers.pushToItemScreen(
      context: context,
      itemId: item.contentId,
      itemType: route,
    );
  }

  @override
  Widget build(BuildContext context) {
    final meta = [
      _kindLabel(context),
      if (item.personName.isNotEmpty) item.personName,
      if (item.completedAtUnixSec > 0)
        DateFormat('MMM d, yyyy').format(
          DateTime.fromMillisecondsSinceEpoch(
            item.completedAtUnixSec.toInt() * 1000,
          ),
        ),
    ].where((s) => s.isNotEmpty).join(' · ');

    return Tappable(
      semanticsLabel: item.title,
      onTap: () => _open(context),
      child: Container(
        decoration: BoxDecoration(
          border: Border(
            bottom: BorderSide(color: AppColors.border(context)),
          ),
        ),
        padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 13),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              item.title,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontFamily: AppTheme.headingFont,
                fontSize: 16,
                color: AppColors.textPrimary(context),
              ),
            ),
            if (meta.isNotEmpty)
              Padding(
                padding: const EdgeInsets.only(top: 3),
                child: Text(
                  meta,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    fontSize: 12.5,
                    color: AppColors.textSecondary(context),
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}
