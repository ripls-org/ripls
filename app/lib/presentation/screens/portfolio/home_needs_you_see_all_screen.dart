import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/presentation/screens/portfolio/home_decision_routing.dart';
import 'package:ripls/presentation/viewmodels/home_tab_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/presentation/widgets/home/needs_you_group_row.dart';

/// HomeNeedsYouSeeAllScreen is the full Needs-you list, grouped by kind (Ready
/// to pick up · Lend · Say thanks · Reply · Did it happen?). Each row carries
/// the same action chip as the inbox-root preview (e.g. "Mark picked up") and
/// runs the identical typed action via [HomeDecisionRouting.runDecisionAction]
/// — opening the associated modal or screen — so the short and comprehensive
/// views stay in lock-step. The "Did it happen?" group keeps a "Mark all done"
/// bulk affordance.
class HomeNeedsYouSeeAllScreen extends ConsumerStatefulWidget {
  const HomeNeedsYouSeeAllScreen({super.key});

  @override
  ConsumerState<HomeNeedsYouSeeAllScreen> createState() =>
      _HomeNeedsYouSeeAllScreenState();
}

class _HomeNeedsYouSeeAllScreenState
    extends ConsumerState<HomeNeedsYouSeeAllScreen> with HomeDecisionRouting {
  Future<void> _markAllDone(List<HomeDecision> items) async {
    if (items.isEmpty) return;
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(context.l10n.homeWrapUpConfirmTitle),
        content: Text(context.l10n.homeWrapUpConfirmBody),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: Text(context.l10n.commonCancel),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: Text(context.l10n.homeWrapUpConfirm),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    final failed = await ref.read(homeTabProvider.notifier).bulkWrapUp(items);
    if (!mounted || failed == 0) return;
    ToastHelper.showError(context, context.l10n.homeWrapUpFailed(failed));
  }

  @override
  Widget build(BuildContext context) {
    final homeState = ref.watch(homeTabProvider);
    final decisions = homeState.visibleDecisions;
    final total = ref.watch(homeNeedsYouCountProvider);

    // Group by kind, in display order; skip empty groups.
    List<HomeDecision> of(bool Function(HomeDecisionKind) test) =>
        decisions.where((d) => test(d.kind)).toList();
    final groups = <({String label, List<HomeDecision> items, bool done})>[
      (
        label: context.l10n.homeNeedsGroupPickup,
        items: of((k) => k == HomeDecisionKind.HOME_DECISION_KIND_TRANSFER_UPDATE),
        done: false,
      ),
      (
        label: context.l10n.homeNeedsGroupLend,
        items: of((k) =>
            k == HomeDecisionKind.HOME_DECISION_KIND_LENDING_REQUEST ||
            k == HomeDecisionKind.HOME_DECISION_KIND_GIVEAWAY_REQUEST),
        done: false,
      ),
      (
        label: context.l10n.homeNeedsGroupThanks,
        items: of((k) => k == HomeDecisionKind.HOME_DECISION_KIND_ASK_CLAIM),
        done: false,
      ),
      (
        label: context.l10n.homeNeedsGroupReply,
        items: of((k) => k == HomeDecisionKind.HOME_DECISION_KIND_REPLY),
        done: false,
      ),
      (
        label: context.l10n.homeNeedsGroupDone,
        items: of((k) => k == HomeDecisionKind.HOME_DECISION_KIND_MARK_DONE),
        done: true,
      ),
    ].where((g) => g.items.isNotEmpty).toList();

    return Scaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        leading: AppBarBackButton(onPressed: () => Navigator.pop(context)),
        centerTitle: true,
        title: Text(
          '${context.l10n.homeNeedsYouSection} · $total',
          style: TextStyle(
            fontFamily: AppTheme.headingFont,
            fontSize: 19,
            fontWeight: FontWeight.w600,
            color: AppColors.textPrimary(context),
          ),
        ),
      ),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(22, 8, 22, 24),
        children: [
          for (final g in groups)
            _buildGroup(context, g.label, g.items, g.done),
        ],
      ),
    );
  }

  Widget _buildGroup(
    BuildContext context,
    String label,
    List<HomeDecision> items,
    bool isDoneGroup,
  ) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 26),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Text(
                label.toUpperCase(),
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w600,
                  letterSpacing: 1.3,
                  color: AppColors.primary(context),
                ),
              ),
              const SizedBox(width: 9),
              Text(
                '· ${items.length}',
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w500,
                  color: AppColors.textSecondary(context),
                ),
              ),
              if (isDoneGroup && items.length > 1) ...[
                const Spacer(),
                Tappable(
                  semanticsLabel: context.l10n.homeNeedsMarkAllDone,
                  onTap: () => _markAllDone(items),
                  inkBorderRadius: BorderRadius.circular(8),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(Icons.done_all,
                          size: 14, color: AppColors.primary(context)),
                      const SizedBox(width: 4),
                      Text(
                        context.l10n.homeNeedsMarkAllDone,
                        style: TextStyle(
                          fontSize: 11.5,
                          fontWeight: FontWeight.w600,
                          color: AppColors.primary(context),
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ],
          ),
          const SizedBox(height: 4),
          for (var i = 0; i < items.length; i++)
            NeedsYouGroupRow(
              decision: items[i],
              onTap: () => openDecision(items[i]),
              onAction: () => runDecisionAction(items[i]),
              showTopBorder: i > 0,
            ),
        ],
      ),
    );
  }
}
