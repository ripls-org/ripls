import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart'
    show TimeProposal, TimeVote, TimeVoteStatus;
import 'package:ripls/presentation/viewmodels/time_modal_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// Confirm modal — owner-only "Set the final time" flow that mirrors
/// [LocationPollConfirmModal] one-for-one. After a poll closes, the
/// organizer picks the winning time from the proposed list, sees a
/// per-proposal vote breakdown, and confirms with "Lock in & notify
/// group". Selection is mutually exclusive.
///
/// Returns `true` when the user successfully locked in a winning time so
/// callers can chain to [TimePollFinalizedModal]; `false`/`null` otherwise.
class TimePollConfirmModal extends ConsumerStatefulWidget {
  const TimePollConfirmModal({super.key, required this.experienceId});

  final String experienceId;

  static Future<bool?> show(BuildContext context, String experienceId) async {
    return showAccessibleModal<bool>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => TimePollConfirmModal(experienceId: experienceId),
    );
  }

  @override
  ConsumerState<TimePollConfirmModal> createState() =>
      _TimePollConfirmModalState();
}

class _TimePollConfirmModalState
    extends ConsumerState<TimePollConfirmModal> {
  String? _selectedProposalId;
  bool _isSaving = false;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final dataAsync = ref.watch(timeModalProvider(widget.experienceId));
    return GlassSheet(
      padding: EdgeInsets.zero,
      child: SafeArea(
        top: false,
        child: ConstrainedBox(
          constraints: BoxConstraints(
            maxHeight: MediaQuery.of(context).size.height * 0.85,
          ),
          child: dataAsync.when(
            data: (data) => _buildLoaded(context, data),
            loading: () => const Padding(
              padding: EdgeInsets.all(48),
              child: Center(child: CircularProgressIndicator()),
            ),
            error: (e, _) => Padding(
              padding: const EdgeInsets.all(32),
              child: Center(
                child: Text(
                  l10n.commonError,
                  style: TextStyle(color: AppColors.modalTextPrimary),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildLoaded(BuildContext context, TimeModalData data) {
    final l10n = context.l10n;
    final pollId = data.currentPollId;
    final visible = data.proposals.where((p) {
      if (pollId == null || pollId.isEmpty) return true;
      if (!p.hasPollId()) return true;
      return p.pollId == pollId;
    }).toList();

    // Default selection: proposal with the most YES votes (single winner).
    _selectedProposalId ??= _resolveLeading(visible);
    final selected = visible.firstWhere(
      (p) => p.id == _selectedProposalId,
      orElse: () => TimeProposal(),
    );

    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 4, 20, 0),
          child: _Eyebrow(text: l10n.timePollConfirmKicker),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 4, 20, 0),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _ConfirmTitle(proposal: selected.id.isEmpty ? null : selected),
              const SizedBox(height: 6),
              _Subtitle(text: l10n.timePollConfirmSubtitle),
            ],
          ),
        ),
        Flexible(
          child: SingleChildScrollView(
            padding: const EdgeInsets.fromLTRB(20, 16, 20, 12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                if (selected.id.isNotEmpty)
                  _FinalTimeCard(proposal: selected),
                const SizedBox(height: 14),
                _SectionLabel(text: l10n.timePollConfirmBreakdown),
                const SizedBox(height: 6),
                ...visible.map((p) => _BreakdownRow(
                      proposal: p,
                      isSelected: p.id == _selectedProposalId,
                      enabled: !_isSaving,
                      onTap: () => setState(() => _selectedProposalId = p.id),
                    )),
              ],
            ),
          ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
          child: Row(
            children: [
              Expanded(
                child: Tappable(
                  semanticsLabel: l10n.commonCancel,
                  onTap: _isSaving
                      ? null
                      : () => Navigator.of(context).pop(),
                  child: _PillButton(
                    label: l10n.commonCancel,
                    variant: _PillVariant.ghost,
                  ),
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                flex: 2,
                child: Tappable(
                  semanticsLabel: l10n.timePollConfirmLockNotify,
                  onTap: (_selectedProposalId == null || _isSaving)
                      ? null
                      : _confirm,
                  child: _PillButton(
                    label: l10n.timePollConfirmLockNotify,
                    variant: _PillVariant.primary,
                    disabled: _selectedProposalId == null || _isSaving,
                  ),
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }

  String? _resolveLeading(List<TimeProposal> proposals) {
    if (proposals.isEmpty) return null;
    int top = -1;
    String? topId;
    for (final p in proposals) {
      final n = p.votes
          .where((v) => v.status == TimeVoteStatus.TIME_VOTE_STATUS_YES)
          .length;
      if (n > top) {
        top = n;
        topId = p.id;
      }
    }
    return topId;
  }

  Future<void> _confirm() async {
    if (_selectedProposalId == null) return;
    setState(() => _isSaving = true);
    try {
      await ref
          .read(timeModalProvider(widget.experienceId).notifier)
          .lockTime(_selectedProposalId!);
      if (!mounted) return;
      // Return `true` so the caller can chain to [TimePollFinalizedModal]
      // — matches the location confirm modal's contract.
      Navigator.of(context).pop(true);
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.commonError)),
      );
    } finally {
      if (mounted) setState(() => _isSaving = false);
    }
  }
}

/* ── Atoms ──────────────────────────────────────────────────────────────── */

class _Eyebrow extends StatelessWidget {
  final String text;
  const _Eyebrow({required this.text});

  @override
  Widget build(BuildContext context) {
    return Text(
      text.toUpperCase(),
      style: TextStyle(
        color: AppColors.modalTextPrimary,
        fontSize: 11,
        fontWeight: FontWeight.w700,
        letterSpacing: 1.6,
      ),
    );
  }
}

class _ConfirmTitle extends StatelessWidget {
  final TimeProposal? proposal;
  const _ConfirmTitle({required this.proposal});

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    if (proposal == null) {
      return _TitleText(text: l10n.timePollConfirmTitleEmpty);
    }
    return _TitleText(
      text: l10n.timePollConfirmTitle(_formatWhen(proposal!)),
    );
  }

  String _formatWhen(TimeProposal p) {
    if (!p.time.hasSpecific()) return '';
    final dt = DateTime.fromMillisecondsSinceEpoch(
      p.time.specific.unixTimestampSec.toInt() * 1000,
    );
    return DateFormat('EEE, MMM d · h:mm a').format(dt);
  }
}

class _TitleText extends StatelessWidget {
  final String text;
  const _TitleText({required this.text});

  @override
  Widget build(BuildContext context) {
    return Semantics(
      header: true,
      child: Text(
        text,
        style: TextStyle(
          color: AppColors.modalTextPrimary,
          fontSize: 28,
          fontWeight: FontWeight.w800,
          height: 1.1,
          letterSpacing: -0.4,
        ),
      ),
    );
  }
}

class _Subtitle extends StatelessWidget {
  final String text;
  const _Subtitle({required this.text});

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: TextStyle(
        color: AppColors.modalTextSecondary,
        fontSize: 14,
        height: 1.4,
      ),
    );
  }
}

class _SectionLabel extends StatelessWidget {
  final String text;
  const _SectionLabel({required this.text});

  @override
  Widget build(BuildContext context) {
    return Text(
      text.toUpperCase(),
      style: TextStyle(
        color: AppColors.modalTextSecondary,
        fontSize: 10,
        fontWeight: FontWeight.w700,
        letterSpacing: 0.8,
      ),
    );
  }
}

/// Sage-tinted "FINAL TIME" card showing the currently-selected winner.
class _FinalTimeCard extends StatelessWidget {
  final TimeProposal proposal;
  const _FinalTimeCard({required this.proposal});

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.lightAccent;
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: accent.withValues(alpha: 0.22),
        border: Border.all(color: accent.withValues(alpha: 0.55), width: 1.5),
        borderRadius: BorderRadius.circular(16),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 56,
            height: 56,
            decoration: BoxDecoration(
              color: accent.withValues(alpha: 0.18),
              borderRadius: BorderRadius.circular(12),
              border: Border.all(color: accent.withValues(alpha: 0.30)),
            ),
            alignment: Alignment.center,
            child: Icon(Icons.access_time_rounded, size: 28, color: accent),
          ),
          const SizedBox(width: 12),
          Expanded(child: _FinalTimeText(proposal: proposal)),
        ],
      ),
    );
  }
}

class _FinalTimeText extends StatelessWidget {
  final TimeProposal proposal;
  const _FinalTimeText({required this.proposal});

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.lightAccent;
    final dt = proposal.time.hasSpecific()
        ? DateTime.fromMillisecondsSinceEpoch(
            proposal.time.specific.unixTimestampSec.toInt() * 1000)
        : null;
    final dateLabel = dt == null ? 'TBD' : DateFormat('EEEE, MMM d').format(dt);
    final timeLabel = dt == null ? '' : DateFormat('h:mm a').format(dt);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          context.l10n.timePollConfirmFinalTime,
          style: TextStyle(
            color: accent,
            fontSize: 9,
            fontWeight: FontWeight.w800,
            letterSpacing: 1,
          ),
        ),
        const SizedBox(height: 3),
        Text(
          dateLabel,
          style: TextStyle(
            color: AppColors.modalTextPrimary,
            fontSize: 18,
            fontWeight: FontWeight.w800,
            height: 1.15,
          ),
          overflow: TextOverflow.ellipsis,
        ),
        if (timeLabel.isNotEmpty) ...[
          const SizedBox(height: 2),
          Text(
            timeLabel,
            style: TextStyle(color: accent, fontSize: 12),
          ),
        ],
      ],
    );
  }
}

/// Tappable row showing a proposal's date/time, selected-state styling,
/// and a voter avatar stack. Tapping selects this proposal as the winner.
class _BreakdownRow extends StatelessWidget {
  final TimeProposal proposal;
  final bool isSelected;
  final bool enabled;
  final VoidCallback onTap;
  const _BreakdownRow({
    required this.proposal,
    required this.isSelected,
    required this.enabled,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.lightAccent;
    final dt = proposal.time.hasSpecific()
        ? DateTime.fromMillisecondsSinceEpoch(
            proposal.time.specific.unixTimestampSec.toInt() * 1000)
        : null;
    final label = dt == null
        ? 'TBD'
        : DateFormat('EEE, MMM d · h:mm a').format(dt);
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Toggle(
        semanticsLabel: label,
        selected: isSelected,
        inMutuallyExclusiveGroup: true,
        onTap: enabled ? onTap : null,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
          decoration: BoxDecoration(
            color: isSelected
                ? accent.withValues(alpha: 0.12)
                : Colors.transparent,
            borderRadius: BorderRadius.circular(10),
            border: Border.all(
              color: isSelected
                  ? accent.withValues(alpha: 0.55)
                  : Colors.transparent,
            ),
          ),
          child: Row(
            children: [
              Expanded(
                child: Text(
                  label,
                  style: TextStyle(
                    color: isSelected ? accent : AppColors.modalTextPrimary,
                    fontSize: 13,
                    fontWeight: isSelected
                        ? FontWeight.w700
                        : FontWeight.w500,
                  ),
                  overflow: TextOverflow.ellipsis,
                ),
              ),
              const SizedBox(width: 8),
              _BreakdownVoterStack(votes: proposal.votes),
            ],
          ),
        ),
      ),
    );
  }
}

class _BreakdownVoterStack extends StatelessWidget {
  final List<TimeVote> votes;
  const _BreakdownVoterStack({required this.votes});

  @override
  Widget build(BuildContext context) {
    final yesVotes = votes
        .where((v) => v.status == TimeVoteStatus.TIME_VOTE_STATUS_YES)
        .toList();
    if (yesVotes.isEmpty) {
      return Text(
        '0',
        style: TextStyle(
          color: AppColors.modalTextMuted,
          fontSize: 12,
          fontWeight: FontWeight.w700,
        ),
      );
    }
    final shown = yesVotes.take(3).toList();
    const double avatarDiameter = 18;
    const double overlap = 7;
    final stackWidth =
        avatarDiameter + (shown.length - 1) * (avatarDiameter - overlap);
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        SizedBox(
          width: stackWidth,
          height: avatarDiameter + 4,
          child: Stack(
            clipBehavior: Clip.none,
            children: [
              for (var i = 0; i < shown.length; i++)
                Positioned(
                  left: i * (avatarDiameter - overlap),
                  top: 0,
                  child: Container(
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      border: Border.all(
                        color: AppColors.modalBackdrop,
                        width: 2,
                      ),
                    ),
                    child: UserAvatar(user: shown[i].user, radius: 9),
                  ),
                ),
            ],
          ),
        ),
        const SizedBox(width: 4),
        Text(
          '${yesVotes.length}',
          style: TextStyle(
            color: AppColors.modalTextPrimary,
            fontSize: 12,
            fontWeight: FontWeight.w700,
          ),
        ),
      ],
    );
  }
}

enum _PillVariant { primary, ghost }

class _PillButton extends StatelessWidget {
  final String label;
  final _PillVariant variant;
  final bool disabled;

  const _PillButton({
    required this.label,
    this.variant = _PillVariant.primary,
    this.disabled = false,
  });

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.lightAccent;
    final isPrimary = variant == _PillVariant.primary;
    final bg = disabled
        ? AppColors.surface(context).withValues(alpha: 0.08)
        : isPrimary
            ? accent
            : Colors.transparent;
    final fg = disabled
        ? AppColors.modalTextMuted.withValues(alpha: 0.42)
        : isPrimary
            ? AppColors.cardBackground(context)
            : AppColors.modalTextPrimary;
    final useBorder = disabled || !isPrimary;
    return Container(
      height: 50,
      decoration: BoxDecoration(
        color: bg,
        border:
            useBorder ? Border.all(color: AppColors.border(context)) : null,
        borderRadius: BorderRadius.circular(14),
        boxShadow: isPrimary && !disabled
            ? [
                BoxShadow(
                  color: accent.withValues(alpha: 0.30),
                  offset: const Offset(0, 8),
                  blurRadius: 20,
                ),
              ]
            : null,
      ),
      alignment: Alignment.center,
      child: Text(
        label,
        style: TextStyle(
          color: fg,
          fontWeight: FontWeight.w700,
          fontSize: 14.5,
        ),
      ),
    );
  }
}
