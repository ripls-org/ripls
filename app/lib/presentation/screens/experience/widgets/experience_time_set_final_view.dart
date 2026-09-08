import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart' show TimeProposal;
import 'package:ripls/presentation/screens/experience/widgets/experience_time_panel_helpers.dart';
import 'package:ripls/presentation/viewmodels/time_modal_view_model.dart';
import 'package:ripls/presentation/widgets/poll/poll_option_widgets.dart';

/// The inline set-final-time view that absorbed `TimePollConfirmModal` into
/// [ExperienceTimePanel] (when-screen v3) — the time twin of
/// [LocationSetFinalView]. The owner picks the winning time from the proposed
/// list, sees a tie / not-leader warning and the "how everyone picked"
/// breakdown, then locks it in.
class ExperienceTimeSetFinalView extends ConsumerWidget {
  final String experienceId;
  final String? finalTargetId;
  final Color accentColor;
  final bool busy;
  final ValueChanged<String> onRetarget;
  final VoidCallback onCancel;
  final VoidCallback? onLock;

  const ExperienceTimeSetFinalView({
    super.key,
    required this.experienceId,
    required this.finalTargetId,
    required this.accentColor,
    required this.busy,
    required this.onRetarget,
    required this.onCancel,
    required this.onLock,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final data = ref.watch(timeModalProvider(experienceId)).asData?.value;
    if (data == null) {
      return const Padding(
        padding: EdgeInsets.only(top: 24),
        child: Center(child: CircularProgressIndicator()),
      );
    }

    final visible = data.currentPollProposals;
    final options = {for (final p in visible) p.id: resolveTimeOption(p)};
    final target =
        visible.where((p) => p.id == finalTargetId).firstOrNull;
    final leadingId = data.leadingProposalId;
    final warning = _warning(context, data, visible, target);
    final ranked = [...visible]..sort((a, b) =>
        data.effectiveVoteCount(b.id).compareTo(data.effectiveVoteCount(a.id)));

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          l10n.timePollConfirmKicker,
          style: const TextStyle(
            color: AppColors.darkTextSecondary,
            fontSize: 11,
            fontWeight: FontWeight.w800,
            letterSpacing: 1.4,
          ),
        ),
        const SizedBox(height: 8),
        Text(
          target == null
              ? l10n.timePollProposeSetTime
              : l10n.timePollConfirmTitle(options[target.id]!.name),
          style: const TextStyle(
            color: AppColors.onContentImage,
            fontSize: 24,
            fontWeight: FontWeight.w800,
            height: 1.1,
          ),
        ),
        const SizedBox(height: 8),
        Text(
          l10n.timePollConfirmSubtitle,
          style: const TextStyle(
            color: AppColors.darkTextSecondary,
            fontSize: 14.5,
            height: 1.45,
          ),
        ),
        if (target != null) ...[
          const SizedBox(height: 16),
          PollFinalCard(
            icon: Icons.event,
            kicker: l10n.timePollConfirmFinalTime,
            name: options[target.id]!.name,
            subtitle: options[target.id]!.subtitle,
            accentColor: accentColor,
          ),
        ],
        if (warning != null) ...[
          const SizedBox(height: 12),
          _WarningBanner(text: warning),
        ],
        const SizedBox(height: 18),
        Text(
          l10n.timePollConfirmBreakdown,
          style: const TextStyle(
            color: AppColors.darkTextSecondary,
            fontSize: 10,
            fontWeight: FontWeight.w700,
            letterSpacing: 0.8,
          ),
        ),
        for (final p in ranked)
          PollPickRow(
            letter: pollOptionLetter(visible.indexWhere((v) => v.id == p.id)),
            name: options[p.id]!.name,
            voters: timeYesVoters(p.votes),
            selected: p.id == finalTargetId,
            leading: p.id == leadingId,
            accentColor: accentColor,
            onTap: () => onRetarget(p.id),
          ),
        const SizedBox(height: 20),
        Row(
          children: [
            Expanded(
              child: PollPanelButton(
                label: l10n.commonCancel,
                accentColor: accentColor,
                onTap: busy ? null : onCancel,
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              flex: 2,
              child: PollPanelButton(
                label: l10n.timePollConfirmLockNotify,
                accentColor: accentColor,
                primary: true,
                onTap: (finalTargetId == null || busy) ? null : onLock,
              ),
            ),
          ],
        ),
      ],
    );
  }

  String? _warning(
    BuildContext context,
    TimeModalData data,
    List<TimeProposal> visible,
    TimeProposal? target,
  ) {
    if (target == null) return null;
    final l10n = context.l10n;
    final tiedTop = pollTiedTopCount(
      visible.map((p) => p.id).toList(),
      data.effectiveVoteCount,
    );
    if (tiedTop > 1 && data.effectiveVoteCount(target.id) == tiedTop) {
      return l10n.locationPanelConfirmTieWarning;
    }
    final leadingId = data.leadingProposalId;
    if (leadingId != null && target.id != leadingId) {
      final lead = visible.where((p) => p.id == leadingId).firstOrNull;
      if (lead != null) {
        return l10n.locationPanelConfirmNotLeaderWarning(
            resolveTimeOption(lead).name);
      }
    }
    return null;
  }
}

class _WarningBanner extends StatelessWidget {
  final String text;
  const _WarningBanner({required this.text});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
      decoration: BoxDecoration(
        color: AppColors.statusWarningOnDark.withValues(alpha: 0.12),
        border: Border.all(
          color: AppColors.statusWarningOnDark.withValues(alpha: 0.4),
        ),
        borderRadius: BorderRadius.circular(14),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Icon(
            Icons.balance,
            size: 18,
            color: AppColors.statusWarningOnDark,
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              text,
              style: const TextStyle(
                color: AppColors.onContentImage,
                fontSize: 13.5,
                height: 1.4,
              ),
            ),
          ),
        ],
      ),
    );
  }
}
