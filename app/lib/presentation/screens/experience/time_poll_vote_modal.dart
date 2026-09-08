import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart'
    show TimeProposal, TimeVoteStatus;
import 'package:ripls/presentation/screens/experience/time_poll_confirm_modal.dart';
import 'package:ripls/presentation/screens/experience/time_poll_finalized_modal.dart';
import 'package:ripls/presentation/screens/experience/time_poll_manage_menu.dart';
import 'package:ripls/presentation/screens/experience/time_poll_propose_modal.dart';
import 'package:ripls/presentation/viewmodels/time_modal_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/experience/time_proposal_tile.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/poll/poll_flexible_vote_row.dart';
import 'package:ripls/services/providers.dart' show authStateProvider;

/// Standalone modal for voting on time-poll proposals.
///
/// Each tile tap commits an immediate YES vote via [TimeModalNotifier.voteOnTime]
/// — there is no batch-and-save step. The notifier handles optimistic state
/// and rollback; this widget tracks per-row in-flight state so the row's tap
/// target disables while the RPC is mid-flight (mirrors
/// [LocationPollVoteModal]).
class TimePollVoteModal extends ConsumerStatefulWidget {
  const TimePollVoteModal({
    super.key,
    required this.experienceId,
    this.pollId,
    this.embedded = false,
  });

  final String experienceId;

  /// When true, the body renders without its own [GlassSheet] chrome so it
  /// can be hosted inside the morphing [TimePollSheet].
  final bool embedded;

  /// Optional poll filter. When set, only proposals belonging to this poll
  /// are shown — used to view a specific historical poll's results from the
  /// time modal's poll-history list. When null, the modal shows the most
  /// recent poll (the one whose id matches the experience's `currentPollId`).
  final String? pollId;

  /// Opens [TimePollVoteModal] as a bottom sheet.
  static Future<void> show(
    BuildContext context,
    String experienceId, {
    String? pollId,
  }) async {
    await showAccessibleModal(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => TimePollVoteModal(
        experienceId: experienceId,
        pollId: pollId,
      ),
    );
  }

  @override
  ConsumerState<TimePollVoteModal> createState() => _TimePollVoteModalState();
}

class _TimePollVoteModalState extends ConsumerState<TimePollVoteModal> {
  /// Proposal IDs the viewer is currently mid-toggle on. Used to disable
  /// that row's tap target while the RPC is in flight — the notifier owns
  /// optimistic UI for everything else.
  final Set<String> _inFlight = {};

  String get experienceId => widget.experienceId;

  /// Resolves which poll this modal session is viewing. An explicit
  /// [TimePollVoteModal.pollId] wins; otherwise default to the experience's
  /// most recent poll. Returns null only when the experience has no current
  /// poll AND the caller didn't pin one (legacy data).
  String? _effectivePollId(TimeModalData data) =>
      widget.pollId ?? data.currentPollId;

  /// Returns the proposals that belong to the poll this session is viewing.
  /// When no poll context is resolvable, falls back to the full list so we
  /// don't accidentally hide legacy proposals that predate `poll_id`.
  List<TimeProposal> _visibleProposals(TimeModalData data) {
    final pollId = _effectivePollId(data);
    if (pollId == null || pollId.isEmpty) return data.proposals;
    return data.proposals
        .where((p) => p.hasPollId() && p.pollId == pollId)
        .toList();
  }

  /// True when the modal should render its read-only results view: either the
  /// poll is no longer running, or the caller pinned a historical poll that
  /// isn't the experience's currently-active one.
  bool _isReadOnly(TimeModalData data) {
    if (!data.timePollActive) return true;
    final pollId = _effectivePollId(data);
    return pollId != null && pollId != data.currentPollId;
  }

  /// Returns the set of proposal IDs in [visibleProposals] that the
  /// currently-authenticated user has cast a YES vote on. Derived from the
  /// live vote rows on each proposal rather than the cached map in
  /// [TimeModalData.currentUserVotes] so the result is robust to a
  /// stale-after-account-switch cache.
  Set<String> _yesVotedIds(List<TimeProposal> visibleProposals) {
    final currentUser = ref.read(authStateProvider).user;
    if (currentUser == null) return const {};
    final yes = <String>{};
    for (final p in visibleProposals) {
      for (final v in p.votes) {
        if (v.user.id == currentUser.id &&
            v.status == TimeVoteStatus.TIME_VOTE_STATUS_YES) {
          yes.add(p.id);
        }
      }
    }
    return yes;
  }

  Future<void> _toggleVote(String proposalId) async {
    if (_inFlight.contains(proposalId)) return;
    setState(() => _inFlight.add(proposalId));

    // Snapshot the pre-tap state so the announcement reflects the user's
    // actual transition (YES → cleared, or not-YES → recorded).
    final previousYes = _yesVotedIds(
      _visibleProposals(ref.read(timeModalProvider(experienceId)).requireValue),
    );
    final wasVoted = previousYes.contains(proposalId);

    try {
      await ref
          .read(timeModalProvider(experienceId).notifier)
          .voteOnTime(proposalId);
      if (!mounted) return;
      // ignore: use_build_context_synchronously
      unawaitedAnnounce(
        context,
        wasVoted
            ? context.l10n.timePollVoteCleared
            : context.l10n.timePollVoteRecorded,
      );
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.commonError)),
      );
    } finally {
      if (mounted) setState(() => _inFlight.remove(proposalId));
    }
  }

  Future<void> _openManageMenu(TimeModalData data) async {
    final result = await showAccessibleModal<TimePollManageAction>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => TimePollManageMenuSheet(
        hasProposals: _visibleProposals(data).isNotEmpty,
        proposalsLocked: data.proposalsLocked,
      ),
    );
    if (result == null || !mounted) return;
    switch (result) {
      case TimePollManageAction.addOption:
        await _openProposeModal();
        break;
      case TimePollManageAction.setFinal:
        await _pickWinningTime();
        break;
      case TimePollManageAction.toggleLock:
        await _toggleLock(data.proposalsLocked);
        break;
      case TimePollManageAction.nudge:
        await _nudgeUnreplied();
        break;
      case TimePollManageAction.cancel:
        await _endPoll(data);
        break;
    }
  }

  Future<void> _toggleLock(bool currentlyLocked) async {
    try {
      await ref
          .read(timeModalProvider(experienceId).notifier)
          .lockTimeProposals(!currentlyLocked);
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.commonError)),
      );
    }
  }

  Future<void> _nudgeUnreplied() async {
    try {
      final count = await ref
          .read(timeModalProvider(experienceId).notifier)
          .nudgeTimePollVoters();
      if (!mounted) return;
      final l10n = context.l10n;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(count == 0
              ? l10n.timePollNudgeNoneSnack
              : l10n.timePollNudgeSentSnack(count)),
        ),
      );
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.commonError)),
      );
    }
  }

  Future<void> _endPoll(TimeModalData data) async {
    final hasVotes = data.proposals.any((p) => p.votes.any(
          (v) => v.status == TimeVoteStatus.TIME_VOTE_STATUS_YES,
        ));

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(context.l10n.timeEndPollTitle),
        content: Text(
          hasVotes
              ? context.l10n.timeEndPollWithVotes
              : context.l10n.timeEndPollNoVotes,
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: Text(context.l10n.timeKeepPoll),
          ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child: Text(
              context.l10n.timeEndPoll,
              style: const TextStyle(color: Colors.red),
            ),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;

    try {
      await ref
          .read(timeModalProvider(experienceId).notifier)
          .endTimePoll();
      // Stay on the modal — once timePollActive flips false the build switches
      // to the read-only results view automatically.
    } catch (_) {
      // State is rolled back in the notifier — no local handling needed.
    }
  }

  Future<void> _openProposeModal() async {
    await TimePollProposeModal.show(context, experienceId);
    if (!mounted) return;
    ref.invalidate(timeModalProvider(experienceId));
  }

  /// Opens [TimePollConfirmModal] so the organizer can pick the winning time.
  /// On a successful confirm, close the vote modal and land on the "It's Set"
  /// finalized modal — mirrors the location flow's Manage → Set the final
  /// spot → Lock-in chain.
  Future<void> _pickWinningTime() async {
    if (!mounted) return;
    final confirmed = await TimePollConfirmModal.show(context, experienceId);
    if (confirmed != true || !mounted) return;
    // Embedded inside the morphing sheet: the confirm already flipped the
    // poll to a confirmed time, so the sheet morphs to the "It's Set" body
    // on its own — don't close it or push a sibling finalized sheet.
    if (widget.embedded) return;
    Navigator.of(context).pop();
    if (!mounted) return;
    await TimePollFinalizedModal.show(context, experienceId);
  }

  @override
  Widget build(BuildContext context) {
    final timeDataAsync = ref.watch(timeModalProvider(experienceId));

    final body = Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _buildHeader(context, timeDataAsync),
        Expanded(
          child: timeDataAsync.when(
            data: (data) => _buildBody(context, data),
            loading: () => const Center(child: CircularProgressIndicator()),
            error: (e, _) => Center(child: Text(e.toString())),
          ),
        ),
        _buildBottomBar(context, timeDataAsync),
      ],
    );
    return SizedBox(
      height: MediaQuery.of(context).size.height * 0.82,
      child: widget.embedded
          ? body
          : GlassSheet(padding: EdgeInsets.zero, child: body),
    );
  }

  Widget _buildHeader(
    BuildContext context,
    AsyncValue<TimeModalData> timeDataAsync,
  ) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 20, 12, 0),
      child: Row(
        children: [
          timeDataAsync.when(
            data: (data) {
              final visible = _visibleProposals(data);
              final totalVoters = _countYesVotersFor(visible);
              final totalOptions = visible.length;
              final viewerHasSubmitted = _yesVotedIds(visible).isNotEmpty;
              return Text(
                viewerHasSubmitted
                    ? context.l10n.timePollVoteSubmittedKicker
                    : context.l10n
                        .timePollVoteEyebrow(totalVoters, totalOptions)
                        .toUpperCase(),
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w700,
                  color: viewerHasSubmitted
                      ? AppColors.modalTextPrimary
                      : AppColors.modalTextMuted,
                  letterSpacing: 0.8,
                ),
              );
            },
            loading: () => const SizedBox.shrink(),
            error: (_, _) => const SizedBox.shrink(),
          ),
          const Spacer(),
          Tappable(
            semanticsLabel: context.l10n.a11yClose,
            onTap: () => Navigator.of(context).pop(),
            child: Icon(
              Icons.close,
              size: 22,
              color: AppColors.modalTextSecondary,
            ),
          ),
          const SizedBox(width: 8),
        ],
      ),
    );
  }

  Widget _buildBody(BuildContext context, TimeModalData data) {
    final readOnly = _isReadOnly(data);
    final visibleProposals = _visibleProposals(data);
    final yesIds = _yesVotedIds(visibleProposals);
    final currentUser = ref.watch(authStateProvider).user;
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 0),
      children: [
        // Title — switches to "Poll results." when the poll has ended.
        Text(
          readOnly
              ? '${context.l10n.timePollResultsTitleLine1} '
                  '${context.l10n.timePollResultsTitleLine2}'
              : '${context.l10n.timePollVoteTitleLine1} '
                  '${context.l10n.timePollVoteTitleLine2}',
          style: const TextStyle(
            fontFamily: AppTheme.headingFont,
            fontSize: 25,
            fontWeight: FontWeight.w600,
            color: AppColors.modalTextPrimary,
            height: 1.1,
            letterSpacing: -0.4,
          ),
          maxLines: 2,
          overflow: TextOverflow.ellipsis,
        ),
        const SizedBox(height: 12),

        // Sub-label
        Row(
          children: [
            Icon(
              readOnly ? Icons.poll_outlined : Icons.check,
              size: 14,
              color: AppColors.modalTextMuted,
            ),
            const SizedBox(width: 5),
            Text(
              (readOnly
                      ? context.l10n.timePollResultsSubLabel
                      : context.l10n.timePollVoteSubLabel)
                  .toUpperCase(),
              style: TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w700,
                color: AppColors.modalTextMuted,
                letterSpacing: 0.6,
              ),
            ),
          ],
        ),
        const SizedBox(height: 20),

        // Proposal tiles. Each tap auto-submits via the notifier; in-flight
        // taps disable the row to avoid double-submit.
        for (final proposal in visibleProposals)
          Padding(
            padding: const EdgeInsets.only(bottom: 10),
            child: TimeProposalTile(
              proposal: proposal,
              isSelected: !readOnly && yesIds.contains(proposal.id),
              isVoting: _inFlight.contains(proposal.id),
              currentUser: readOnly ? null : currentUser,
              onTap: readOnly || _inFlight.contains(proposal.id)
                  ? null
                  : () => _toggleVote(proposal.id),
            ),
          ),

        // Flexible "any time works" toggle — shown to voters (not the
        // organizer, who decides the final time). Folds into every option's
        // tally via the view-model.
        if (!readOnly && !data.isOrganizer) ...[
          PollFlexibleVoteRow(
            title: context.l10n.timePollVoteFlexibleTitle,
            subtitle: context.l10n.timePollVoteFlexibleSubtitle,
            semanticsLabel: context.l10n.timePollVoteFlexibleTitle,
            selected: data.currentUserIsFlexible,
            onTap: () => _toggleFlexible(data.currentUserIsFlexible),
          ),
          const SizedBox(height: 10),
        ],

        // Add option row — hidden once the poll has ended or proposals are locked.
        if (!readOnly && !data.proposalsLocked)
          _AddOptionRow(onTap: _openProposeModal),
        const SizedBox(height: 16),
      ],
    );
  }

  Widget _buildBottomBar(
    BuildContext context,
    AsyncValue<TimeModalData> timeDataAsync,
  ) {
    final data = timeDataAsync.value;
    final isOrganizer = data?.isOrganizer ?? false;
    final readOnly = data != null && _isReadOnly(data);

    if (readOnly) {
      // When the owner ended the poll without picking a time, surface a CTA
      // so they can recover from inside the modal without hunting for the
      // PollEndedBanner.
      final ownerCanPickTime = isOrganizer &&
          data.timePollCompleted &&
          !(data.eventTime?.hasSpecific() ?? false) &&
          !(data.eventTime?.hasRange() ?? false);

      return SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(16, 8, 16, 12),
          child: ownerCanPickTime
              ? Tappable(
                  semanticsLabel: context.l10n.timePollPickWinnerCta,
                  onTap: _pickWinningTime,
                  child: Container(
                    height: 54,
                    decoration: BoxDecoration(
                      color: AppColors.lightAccent,
                      borderRadius: BorderRadius.circular(16),
                    ),
                    alignment: Alignment.center,
                    child: Text(
                      context.l10n.timePollPickWinnerCta,
                      style: const TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                        color: Colors.white,
                      ),
                    ),
                  ),
                )
              : Tappable(
                  semanticsLabel: context.l10n.timePollResultsClose,
                  onTap: () => Navigator.of(context).pop(),
                  child: Container(
                    height: 54,
                    decoration: BoxDecoration(
                      color: AppColors.modalInsetCardBorder,
                      borderRadius: BorderRadius.circular(16),
                    ),
                    alignment: Alignment.center,
                    child: Text(
                      context.l10n.timePollResultsClose,
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                        color: AppColors.modalTextSecondary,
                      ),
                    ),
                  ),
                ),
        ),
      );
    }

    // Active-poll bottom bar: organizers see a single Manage button that
    // opens the action menu (mirrors the location-poll Manage affordance).
    // Non-organizers see no bottom bar — the dashed-add row inside the body
    // is their only affordance.
    if (!isOrganizer || data == null) {
      return const SafeArea(child: SizedBox(height: 12));
    }

    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 8, 16, 12),
        child: Tappable(
          semanticsLabel: context.l10n.timePollManageButton,
          onTap: () => _openManageMenu(data),
          child: Container(
            height: 54,
            decoration: BoxDecoration(
              color: AppColors.modalInsetCardBg,
              borderRadius: BorderRadius.circular(16),
              border: Border.all(
                color: AppColors.modalInsetCardBorder,
                width: 1,
              ),
            ),
            alignment: Alignment.center,
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(
                  Icons.more_horiz,
                  size: 18,
                  color: AppColors.modalTextPrimary,
                ),
                const SizedBox(width: 8),
                Text(
                  context.l10n.timePollManageButton,
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w600,
                    color: AppColors.modalTextPrimary,
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  int _countYesVotersFor(List<TimeProposal> proposals) {
    final ids = <String>{};
    for (final p in proposals) {
      for (final v in p.votes) {
        // A flexible ("any time works") vote counts as a reply too.
        if (v.status == TimeVoteStatus.TIME_VOTE_STATUS_YES ||
            v.status == TimeVoteStatus.TIME_VOTE_STATUS_FLEXIBLE) {
          ids.add(v.user.id);
        }
      }
    }
    return ids.length;
  }

  /// Toggles the viewer's "any time works" flexible vote on the active poll.
  Future<void> _toggleFlexible(bool currentlyFlexible) async {
    try {
      await ref
          .read(timeModalProvider(experienceId).notifier)
          .setFlexibleOnTime(!currentlyFlexible);
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.commonError)),
      );
    }
  }
}

/// Wrapper around [SemanticAnnouncer.announce] that swallows the Future so
/// callers don't have to mark themselves async just to fire an announcement.
void unawaitedAnnounce(BuildContext context, String message) {
  // ignore: discarded_futures
  SemanticAnnouncer.announce(context, message);
}

class _AddOptionRow extends StatelessWidget {
  const _AddOptionRow({required this.onTap});
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final sage = AppColors.lightAccent;
    return Tappable(
      semanticsLabel: context.l10n.timePollVoteAddOption,
      onTap: onTap,
      child: SizedBox(
        height: 56,
        child: CustomPaint(
          painter: _DashedBorderPainter(
            color: sage.withValues(alpha: 0.45),
            radius: 14,
          ),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Container(
                width: 28,
                height: 28,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  border: Border.all(
                    color: sage.withValues(alpha: 0.55),
                    width: 1.5,
                  ),
                ),
                child: Icon(Icons.add, size: 16, color: sage),
              ),
              const SizedBox(width: 10),
              Text(
                context.l10n.timePollVoteAddOption,
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: AppColors.modalTextPrimary,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _DashedBorderPainter extends CustomPainter {
  const _DashedBorderPainter({required this.color, required this.radius});
  final Color color;
  final double radius;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1.5
      ..style = PaintingStyle.stroke;
    const dashWidth = 6.0;
    const dashSpace = 4.0;
    final rrect = RRect.fromRectAndRadius(
      Rect.fromLTWH(0, 0, size.width, size.height),
      Radius.circular(radius),
    );
    final path = Path()..addRRect(rrect);
    for (final metric in path.computeMetrics()) {
      var distance = 0.0;
      while (distance < metric.length) {
        canvas.drawPath(
          metric.extractPath(distance, distance + dashWidth),
          paint,
        );
        distance += dashWidth + dashSpace;
      }
    }
  }

  @override
  bool shouldRepaint(_DashedBorderPainter old) =>
      old.color != color || old.radius != radius;
}
