import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/presentation/screens/create/blank_create_dispatcher.dart';
import 'package:ripls/presentation/screens/portfolio/home_decision_routing.dart';
import 'package:ripls/presentation/screens/portfolio/home_needs_you_see_all_screen.dart';
import 'package:ripls/presentation/viewmodels/feed_view_model.dart'
    show feedProvider;
import 'package:ripls/presentation/viewmodels/home_tab_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/adaptive/content_column.dart';
import 'package:ripls/presentation/widgets/destination_header.dart';
import 'package:ripls/presentation/widgets/home/community_pulse_section.dart';
import 'package:ripls/presentation/widgets/home/home_decision_copy.dart';
import 'package:ripls/presentation/widgets/home/home_editorial_row.dart';
import 'package:ripls/presentation/widgets/home/home_empty_states.dart';
import 'package:ripls/services/providers.dart'
    show authStateProvider, communitiesProvider;

/// Maximum rows shown per section on the editorial inbox root.
const int _kRootCap = 3;

/// HomeTabScreen is the editorial inbox (the first/landing tab), matching
/// the converged-nav mock (#2634): a greeting header with the avatar chip,
/// then Needs-you on top, the "In your communities" pulse cards, and a
/// quiet Yours preview. The calendar lives on the Plans tab.
class HomeTabScreen extends ConsumerStatefulWidget {
  const HomeTabScreen({super.key});

  @override
  ConsumerState<HomeTabScreen> createState() => _HomeTabScreenState();
}

class _HomeTabScreenState extends ConsumerState<HomeTabScreen>
    with HomeDecisionRouting {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      ref.read(homeTabProvider.notifier).load();
    });
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(homeTabProvider);
    return Material(
      color: AppColors.background(context),
      // Route-level anchor for the Playwright e2e harness — sits at body depth
      // so it reaches the flt-semantics DOM tree (see
      // docs/client/testing/semantics_identifiers.md).
      child: Semantics(
        explicitChildNodes: true,
        container: true,
        identifier: 'web-home-screen',
        child: SafeArea(
          bottom: false,
          child: _buildBody(context, state),
        ),
      ),
    );
  }

  // ─── Body ───

  Widget _buildBody(BuildContext context, HomeTabState state) {
    if (state.isLoading && state.view == null) {
      return const Center(child: CircularProgressIndicator());
    }
    if (state.hasError && state.view == null) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Text(
            RpcErrorHandler.localize(state.error!, context.l10n),
            textAlign: TextAlign.center,
            style: TextStyle(color: AppColors.textSecondary(context)),
          ),
        ),
      );
    }
    final view = state.view;
    if (view == null) return const SizedBox.shrink();

    final needsYou = _buildNeedsYou(context, state);
    // Zero state: nothing needs the user — the empty section collapses
    // into one focused invitation, which owns the nudge in this state.
    // (The pulse may still render below it once communities have activity.)
    final sectionsEmpty = needsYou.isEmpty;

    return RefreshIndicator(
      onRefresh: () async {
        // The pulse is fed by the feed lists, so pull-to-refresh renews
        // both server views together.
        final communityIds = ref.read(communitiesProvider).communityIds;
        await Future.wait<void>([
          ref.read(homeTabProvider.notifier).refresh(),
          if (communityIds.isNotEmpty)
            ref.read(feedProvider.notifier).initialize(communityIds),
        ]);
      },
      // The pulse is the page's long tail (the user's own stuff moved to
      // the Me sheet), so the home scroll paginates the feed like the old
      // Feed tab did.
      child: NotificationListener<ScrollNotification>(
        onNotification: (notification) {
          if (notification.metrics.extentAfter < 600) {
            final feedState = ref.read(feedProvider);
            if (feedState.hasMore &&
                !feedState.isLoading &&
                !feedState.isLoadingMore) {
              unawaited(ref.read(feedProvider.notifier).loadMore());
            }
          }
          return false;
        },
        // Every section holds the reading measure (#2912): the ListView stays
        // window-wide (scrollbar at the window edge), each child centers at
        // ContentColumn's cap — a no-op at phone widths.
        child: ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: EdgeInsets.zero,
          children: [
            ContentColumn(child: _buildGreeting(context)),
            if (sectionsEmpty)
              ContentColumn(
                child: HomeZeroStateCard(
                  onPlan: () =>
                      unawaited(openBlankCreateExperience(context, ref)),
                  onAsk: () => unawaited(openBlankCreateRequest(context, ref)),
                  onOffer: () => unawaited(openBlankCreateGear(context, ref)),
                  nudge: state.inboxNudge,
                  onNudgeConsumed: state.inboxNudge == null
                      ? null
                      : () => ref
                          .read(homeTabProvider.notifier)
                          .consumeInboxNudge(state.inboxNudge!.nudgeId,
                              state.inboxNudge!.ctaAction),
                ),
              )
            else
              // Needs-you leads (#2634 mock)…
              ...needsYou.map((w) => ContentColumn(child: w)),
            // …then the pulse retells the feed as smaller cards.
            const ContentColumn(
              child: Padding(
                padding: EdgeInsets.symmetric(horizontal: 20),
                child: CommunityPulseSection(),
              ),
            ),
            const SizedBox(height: 90), // clearance for the floating nav
          ],
        ),
      ),
    );
  }

  // ─── Greeting header: two-line serif greeting + the avatar chip ───

  Widget _buildGreeting(BuildContext context) {
    final hour = DateTime.now().hour;
    final greeting = hour < 12
        ? context.l10n.homeGreetingMorning
        : hour < 17
            ? context.l10n.homeGreetingAfternoon
            : context.l10n.homeGreetingEvening;
    final name = (ref.watch(authStateProvider).user?.name ?? '').trim();
    final firstName = name.isEmpty ? '' : name.split(' ').first;
    return DestinationHeader(
      title: firstName.isEmpty ? greeting : '$greeting,\n$firstName',
    );
  }

  // ─── Needs you ───

  List<Widget> _buildNeedsYou(BuildContext context, HomeTabState state) {
    final interpersonal = state.visibleDecisions
        .where((d) => d.kind != HomeDecisionKind.HOME_DECISION_KIND_MARK_DONE)
        .toList();
    final total = ref.watch(homeNeedsYouCountProvider);
    if (interpersonal.isEmpty && total == 0) return const [];

    final shown = interpersonal.take(_kRootCap).toList();
    return [
      _sectionLabel(
        context,
        label: context.l10n.homeNeedsYouSection,
        count: total,
        onSeeAll: _openNeedsYouSeeAll,
      ),
      Padding(
        padding: const EdgeInsets.symmetric(horizontal: 20),
        child: Column(
          children: [
            for (var i = 0; i < shown.length; i++)
              HomeEditorialRow(
                title: shown[i].subjectTitle,
                subtitle: _decisionSubtitle(context, shown[i]),
                pillLabel: homeDecisionAcceptLabel(context.l10n, shown[i]),
                onPill: () => runDecisionAction(shown[i]),
                onTap: () => openDecision(shown[i]),
                // Needs-you bullets are dark heritage sage.
                dotColor: AppColors.primary(context),
                showTopBorder: i > 0,
              ),
          ],
        ),
      ),
    ];
  }

  String _decisionSubtitle(BuildContext context, HomeDecision d) {
    if (d.kind == HomeDecisionKind.HOME_DECISION_KIND_ASK_CLAIM) {
      final names = d.helpers
          .map((h) => h.displayName.split(' ').first)
          .where((n) => n.isNotEmpty)
          .toList();
      if (names.isNotEmpty) return context.l10n.homePitchedIn(_joinNames(names));
    }
    final who = d.hasCounterparty()
        ? d.counterparty.displayName.split(' ').first
        : '';
    final why = homeDecisionWhy(context.l10n, d);
    if (who.isNotEmpty && why.isNotEmpty) return '$who · $why';
    if (who.isNotEmpty) return who;
    return why;
  }

  String _joinNames(List<String> names) {
    if (names.length == 1) return names.first;
    if (names.length == 2) return '${names[0]} & ${names[1]}';
    return '${names.sublist(0, names.length - 1).join(', ')} & ${names.last}';
  }

  // ─── Shared section chrome ───

  Widget _sectionTitle(BuildContext context, String label) => Text(
        label.toUpperCase(),
        style: TextStyle(
          fontSize: 11.5,
          fontWeight: FontWeight.w600,
          letterSpacing: 1.3,
          color: AppColors.primary(context),
        ),
      );

  Widget _sectionLabel(
    BuildContext context, {
    required String label,
    int? count,
    VoidCallback? onSeeAll,
  }) {
    // The count now lives on the "See all" affordance, e.g. "See all 29 ›".
    final seeAllText = count != null
        ? '${context.l10n.homeSeeAllCount(count)} ›'
        : '${context.l10n.homeSeeAll} ›';
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 4, 20, 10),
      child: Row(
        children: [
          _sectionTitle(context, label),
          const Spacer(),
          if (onSeeAll != null)
            Tappable(
              semanticsLabel: seeAllText,
              onTap: onSeeAll,
              inkBorderRadius: BorderRadius.circular(8),
              child: Text(
                seeAllText,
                style: TextStyle(
                  fontSize: 12,
                  fontWeight: FontWeight.w500,
                  color: AppColors.textSecondary(context),
                ),
              ),
            ),
        ],
      ),
    );
  }

  // ─── Navigation ───

  void _openNeedsYouSeeAll() {
    unawaited(NavigationHelpers.pushWithSlide(
      context: context,
      screen: const HomeNeedsYouSeeAllScreen(),
      routeName: 'home_needs_you_see_all',
    ));
  }

}
