import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart'
    show LocationVote, LocationVoteStatus;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/experience/widgets/experience_location_panel_widgets.dart';
import 'package:ripls/presentation/viewmodels/location_modal_view_model.dart';
import 'package:ripls/presentation/viewmodels/location_panel_providers.dart';
import 'package:ripls/presentation/widgets/poll/poll_option_widgets.dart';
import 'package:ripls/services/device_location_service.dart';
import 'package:ripls/services/providers.dart';

/// The inline set-final-spot view that absorbed `LocationPollConfirmModal` into
/// [ExperienceLocationPanel] (where-screen v3). The owner picks the winning
/// spot from the proposed list — seeing a tie / not-leader warning and the
/// full "how everyone picked" breakdown — then locks it in.
///
/// Self-contained: watches the same view-model + resolution providers the
/// panel does, and reports the selected target / cancel / lock-in up via
/// callbacks so the panel owns the mutation and busy state.
class LocationSetFinalView extends ConsumerWidget {
  final String experienceId;
  final String? finalTargetId;
  final Color accentColor;
  final bool busy;
  final ValueChanged<String> onRetarget;
  final VoidCallback onCancel;
  final VoidCallback? onLock;

  const LocationSetFinalView({
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
    final geo = ref.watch(locationPanelGeoProvider(experienceId)).asData?.value;
    final data = ref.watch(locationModalProvider(experienceId)).asData?.value;
    final pos = ref
        .watch(userLocationProvider(LocationIntent.precisePin))
        .asData
        ?.value;
    if (geo == null || data == null) {
      return const Padding(
        padding: EdgeInsets.only(top: 24),
        child: Center(child: CircularProgressIndicator()),
      );
    }

    final pollId = data.currentLocationPollId;
    final visible = geo.proposals
        .where((r) =>
            pollId == null ||
            pollId.isEmpty ||
            !r.proposal.hasPollId() ||
            r.proposal.pollId == pollId)
        .toList();
    final target =
        visible.where((r) => r.proposal.id == finalTargetId).firstOrNull;
    final leadingId = data.leadingProposalId;
    final warning = _warning(context, data, visible, target);
    final ranked = [...visible]..sort((a, b) => data
        .effectiveVoteCount(b.proposal.id)
        .compareTo(data.effectiveVoteCount(a.proposal.id)));

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          l10n.locationPollConfirmKicker,
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
              ? l10n.locationPollConfirmTitleEmpty
              : l10n.locationPollConfirmTitle(target.name),
          style: const TextStyle(
            color: AppColors.onContentImage,
            fontSize: 24,
            fontWeight: FontWeight.w800,
            height: 1.1,
          ),
        ),
        const SizedBox(height: 8),
        Text(
          l10n.locationPollConfirmSubtitle,
          style: const TextStyle(
            color: AppColors.darkTextSecondary,
            fontSize: 14.5,
            height: 1.45,
          ),
        ),
        if (target != null) ...[
          const SizedBox(height: 16),
          PollFinalCard(
            icon: Icons.place,
            kicker: l10n.locationPollConfirmFinalSpot,
            name: target.name,
            subtitle: target.address,
            evidence: locationDistanceLabel(target, pos),
            accentColor: accentColor,
          ),
        ],
        if (warning != null) ...[
          const SizedBox(height: 12),
          _WarningBanner(text: warning),
        ],
        const SizedBox(height: 18),
        Text(
          l10n.locationPollConfirmBreakdown,
          style: const TextStyle(
            color: AppColors.darkTextSecondary,
            fontSize: 10,
            fontWeight: FontWeight.w700,
            letterSpacing: 0.8,
          ),
        ),
        for (final r in ranked)
          PollPickRow(
            letter: pollOptionLetter(
                visible.indexWhere((v) => v.proposal.id == r.proposal.id)),
            name: r.name,
            voters: _yesVoters(r.proposal.votes),
            selected: r.proposal.id == finalTargetId,
            leading: r.proposal.id == leadingId,
            accentColor: accentColor,
            onTap: () => onRetarget(r.proposal.id),
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
                label: l10n.locationPollConfirmLockNotify,
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
    LocationModalData data,
    List<ResolvedLocationProposal> visible,
    ResolvedLocationProposal? target,
  ) {
    if (target == null) return null;
    final l10n = context.l10n;
    final tiedTop = pollTiedTopCount(
      visible.map((r) => r.proposal.id).toList(),
      data.effectiveVoteCount,
    );
    if (tiedTop > 1 && data.effectiveVoteCount(target.proposal.id) == tiedTop) {
      return l10n.locationPanelConfirmTieWarning;
    }
    final leadingId = data.leadingProposalId;
    if (leadingId != null && target.proposal.id != leadingId) {
      final lead =
          visible.where((r) => r.proposal.id == leadingId).firstOrNull;
      if (lead != null) {
        return l10n.locationPanelConfirmNotLeaderWarning(lead.name);
      }
    }
    return null;
  }

  List<User> _yesVoters(List<LocationVote> votes) => [
        for (final v in votes)
          if (v.status == LocationVoteStatus.LOCATION_VOTE_STATUS_YES) v.user,
      ];
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
          Icon(Icons.balance, size: 18, color: AppColors.statusWarningOnDark),
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
