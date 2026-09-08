import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart'
    show LocationProposal, LocationVote, LocationVoteStatus;
import 'package:ripls/data/gen/ripls/api/location.pb.dart' as locapi;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/experience/location_poll_confirm_modal.dart';
import 'package:ripls/presentation/screens/experience/location_poll_finalized_modal.dart';
import 'package:ripls/presentation/screens/experience/location_poll_manage_menu.dart';
import 'package:ripls/presentation/screens/experience/location_poll_propose_modal.dart';
import 'package:ripls/presentation/viewmodels/location_modal_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/location/add_spot_card.dart';
import 'package:ripls/presentation/widgets/location/location_picker_modal.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/poll/poll_flexible_vote_row.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/providers.dart';

/// Vote modal — matches LV1 / LV2 / LV3 in the location-redesign HTML, but
/// auto-submits each pick on tap rather than batching behind a "Send picks"
/// CTA. Visual states collapse to two:
///   - `idle`: untouched, white outline.
///   - `voted`: server-confirmed YES, sage fill + dark check. (Optimistic.)
///
/// Header switches to "YOU'RE IN" once any pick is on the server.
class LocationPollVoteModal extends ConsumerStatefulWidget {
  const LocationPollVoteModal({
    super.key,
    required this.experienceId,
    this.pollId,
  });

  final String experienceId;
  final String? pollId;

  static Future<void> show(
    BuildContext context,
    String experienceId, {
    String? pollId,
  }) async {
    await showAccessibleModal(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => LocationPollVoteModal(
        experienceId: experienceId,
        pollId: pollId,
      ),
    );
  }

  @override
  ConsumerState<LocationPollVoteModal> createState() =>
      _LocationPollVoteModalState();
}

class _LocationPollVoteModalState
    extends ConsumerState<LocationPollVoteModal> {
  /// Set of proposal ids the viewer is currently mid-toggle on. Used to
  /// disable that row's checkbox while the RPC is in flight — the
  /// view-model handles optimistic UI for everything else.
  final Set<String> _inFlight = {};
  bool _endingPoll = false;
  bool _addingSpot = false;

  String get _experienceId => widget.experienceId;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final dataAsync = ref.watch(locationModalProvider(_experienceId));
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

  Widget _buildLoaded(BuildContext context, LocationModalData data) {
    final l10n = context.l10n;
    final effectivePollId = widget.pollId ?? data.currentLocationPollId;
    final visibleProposals = data.proposals.where((p) {
      if (effectivePollId == null) return true;
      if (!p.hasPollId()) return true;
      return p.pollId == effectivePollId;
    }).toList();
    final readOnly = !data.locationPollActive ||
        (effectivePollId != null &&
            effectivePollId != data.currentLocationPollId);

    // Voters that replied to the current poll — an explicit YES on any spot
    // or a flexible ("any spot works") vote, which counts as a reply too.
    final voterIds = visibleProposals
        .expand((p) => p.votes
            .where((v) =>
                v.status == LocationVoteStatus.LOCATION_VOTE_STATUS_YES ||
                v.status == LocationVoteStatus.LOCATION_VOTE_STATUS_FLEXIBLE)
            .map((v) => v.user.id))
        .toSet();
    final voterCount = voterIds.length;
    final invitedCount = _resolveInvitedCount(data, voterCount);

    final leadingId = _resolveLeading(visibleProposals);
    final viewerHasSubmitted = data.currentUserYesProposalIds.isNotEmpty;
    final viewerInVoters =
        data.currentUserId != null && voterIds.contains(data.currentUserId);

    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 4, 20, 0),
          child: _Eyebrow(
            text: viewerHasSubmitted
                ? l10n.locationPollVoteSubmittedKicker
                : l10n.locationPollVoteKicker,
            accent: viewerHasSubmitted,
          ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 4, 20, 0),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _Title(
                text: viewerHasSubmitted
                    ? l10n.locationPollVoteSubmittedTitle
                    : l10n.locationPollVoteTitle,
              ),
              const SizedBox(height: 6),
              if (!viewerHasSubmitted)
                _Subtitle(text: l10n.locationPollVoteSubtitle),
              const SizedBox(height: 10),
              LiveRegion(
                child: _VoteCounter(
                  voted: voterCount,
                  invited: invitedCount,
                  includingYou: viewerInVoters,
                ),
              ),
            ],
          ),
        ),
        Flexible(
          child: SingleChildScrollView(
            padding: const EdgeInsets.fromLTRB(20, 14, 20, 12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                ...visibleProposals.map((p) {
                  final voted = data.currentUserYesProposalIds.contains(p.id);
                  return _VoteLocRow(
                    proposal: p,
                    voted: voted,
                    leading: leadingId == p.id,
                    showAddedBy: p.proposedBy.id != data.currentUserId,
                    enabled: !readOnly && !_inFlight.contains(p.id),
                    onToggle: () => _toggleVote(p.id),
                  );
                }),
                if (!readOnly && !data.isOrganizer) ...[
                  const SizedBox(height: 9),
                  PollFlexibleVoteRow(
                    title: l10n.locationPollVoteFlexibleTitle,
                    subtitle: l10n.locationPollVoteFlexibleSubtitle,
                    semanticsLabel: l10n.locationPollVoteFlexibleTitle,
                    selected: data.currentUserIsFlexible,
                    onTap: () => _toggleFlexible(data.currentUserIsFlexible),
                  ),
                ],
                if (!readOnly &&
                    !data.locationProposalsLocked &&
                    !data.isOrganizer) ...[
                  const SizedBox(height: 4),
                  AddSpotCard(
                    label: l10n.locationPollProposeAddAnother,
                    onTap: _addingSpot ? null : _addSpot,
                  ),
                ],
              ],
            ),
          ),
        ),
        if (data.isOrganizer && !readOnly)
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 4, 20, 24),
            child: Tappable(
              semanticsLabel: l10n.locationPollManageButton,
              onTap: _endingPoll
                  ? null
                  : () => _openManageMenu(context, data),
              child: _PillButton(
                label: l10n.locationPollManageButton,
                variant: _PillVariant.ghost,
                leadingIcon: Icons.more_horiz,
              ),
            ),
          )
        else
          const SizedBox(height: 24),
      ],
    );
  }

  Future<void> _openManageMenu(
      BuildContext context, LocationModalData data) async {
    final result = await showAccessibleModal<LocationPollManageAction>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => LocationPollManageMenuSheet(
        hasProposals: data.proposals.isNotEmpty,
        proposalsLocked: data.locationProposalsLocked,
      ),
    );
    if (result == null || !mounted) return;
    switch (result) {
      case LocationPollManageAction.addSpot:
        // The "Add another spot" menu item was relabelled to "Edit Choices"
        // — instead of opening the location picker for a single new spot,
        // route the owner to the Pick a Spot screen where they can add,
        // remove, or change the deadline against the active poll.
        await LocationPollProposeModal.show(this.context, _experienceId);
        break;
      case LocationPollManageAction.setFinal:
        final confirmed =
            await LocationPollConfirmModal.show(this.context, _experienceId);
        if ((confirmed ?? false) && mounted) {
          // Close the vote modal so the user lands on "It's a plan"
          // instead of falling back to the (now stale) vote sheet.
          Navigator.of(this.context).pop();
          if (!mounted) return;
          await LocationPollFinalizedModal.show(this.context, _experienceId);
        }
        break;
      case LocationPollManageAction.toggleLock:
        await _toggleLock(data.locationProposalsLocked);
        break;
      case LocationPollManageAction.nudge:
        await _nudgeUnreplied();
        break;
      case LocationPollManageAction.cancel:
        final confirmed = await _confirmCancel();
        if (confirmed != true || !mounted) return;
        await _endPoll();
        break;
    }
  }

  Future<void> _toggleLock(bool currentlyLocked) async {
    try {
      await ref
          .read(locationModalProvider(_experienceId).notifier)
          .setLocationProposalsLocked(!currentlyLocked);
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
          .read(locationModalProvider(_experienceId).notifier)
          .nudgeUnreplied();
      if (!mounted) return;
      final l10n = context.l10n;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(count == 0
              ? l10n.locationPollNudgeNoneSnack
              : l10n.locationPollNudgeSentSnack(count)),
        ),
      );
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.commonError)),
      );
    }
  }

  Future<bool?> _confirmCancel() {
    final l10n = context.l10n;
    return showDialog<bool>(
      context: context,
      builder: (dialogCtx) => AlertDialog(
        title: Text(l10n.locationPollManageCancelPoll),
        content: Text(l10n.locationPollManageCancelDesc),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogCtx).pop(false),
            child: Text(l10n.commonCancel),
          ),
          TextButton(
            onPressed: () => Navigator.of(dialogCtx).pop(true),
            child: Text(l10n.locationPollManageCancelPoll),
          ),
        ],
      ),
    );
  }

  Future<void> _toggleVote(String proposalId) async {
    if (_inFlight.contains(proposalId)) return;
    setState(() => _inFlight.add(proposalId));
    try {
      await ref
          .read(locationModalProvider(_experienceId).notifier)
          .voteOnLocation(proposalId);
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.commonError)),
      );
    } finally {
      if (mounted) setState(() => _inFlight.remove(proposalId));
    }
  }

  String? _resolveLeading(List<LocationProposal> proposals) {
    if (proposals.isEmpty) return null;
    int top = 0;
    String? topId;
    int dupes = 0;
    for (final p in proposals) {
      final n = p.votes
          .where((v) => v.status == LocationVoteStatus.LOCATION_VOTE_STATUS_YES)
          .length;
      if (n > top) {
        top = n;
        topId = p.id;
        dupes = 0;
      } else if (n == top && n > 0) {
        dupes += 1;
      }
    }
    if (top == 0 || dupes > 0) return null;
    return topId;
  }

  int _resolveInvitedCount(LocationModalData data, int voterCount) {
    // The modal state does not currently expose the full participants
    // list, so we report the voter count for both sides — matching the
    // design's "N OF M REPLIED" copy while N == M when everyone has
    // voted. Future work can plumb participants through the view-model.
    return voterCount > 0 ? voterCount : 0;
  }

  Future<void> _endPoll() async {
    setState(() => _endingPoll = true);
    try {
      await ref
          .read(locationModalProvider(_experienceId).notifier)
          .endLocationPoll();
      if (!mounted) return;
      Navigator.of(context).pop();
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.commonError)),
      );
    } finally {
      if (mounted) setState(() => _endingPoll = false);
    }
  }

  /// Toggles the viewer's "any spot works" flexible vote on the active poll.
  /// The view-model records it optimistically and folds the flexible voter
  /// into every option's tally.
  Future<void> _toggleFlexible(bool currentlyFlexible) async {
    try {
      await ref
          .read(locationModalProvider(_experienceId).notifier)
          .setFlexibleOnLocation(!currentlyFlexible);
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.commonError)),
      );
    }
  }

  /// Opens the location picker and proposes the picked spot as a new
  /// option on the active poll. Available to any viewer (organizer or
  /// not) as long as the proposal list isn't locked — the call site
  /// already gates on `locationProposalsLocked` + `readOnly`.
  Future<void> _addSpot() async {
    final id = await LocationPickerModal.show(context);
    if (id == null || id.isEmpty || !mounted) return;
    setState(() => _addingSpot = true);
    try {
      await ref
          .read(locationModalProvider(_experienceId).notifier)
          .proposeSavedLocation(id);
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.commonError)),
      );
    } finally {
      if (mounted) setState(() => _addingSpot = false);
    }
  }
}

/* ── Atoms ──────────────────────────────────────────────────────────────── */

class _Eyebrow extends StatelessWidget {
  final String text;
  final bool accent;
  const _Eyebrow({required this.text, this.accent = false});

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

class _Title extends StatelessWidget {
  final String text;
  const _Title({required this.text});

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

/// "N OF M REPLIED" counter with an optional sage-tinted suffix once the
/// viewer is among the responders.
class _VoteCounter extends StatelessWidget {
  final int voted;
  final int invited;
  final bool includingYou;
  const _VoteCounter({
    required this.voted,
    required this.invited,
    required this.includingYou,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final dotColor =
        voted > 0 ? AppColors.lightAccent : AppColors.modalTextMuted;
    return Wrap(
      crossAxisAlignment: WrapCrossAlignment.center,
      children: [
        Container(
          width: 6,
          height: 6,
          margin: const EdgeInsets.only(right: 6),
          decoration: BoxDecoration(color: dotColor, shape: BoxShape.circle),
        ),
        Text(
          l10n.locationPollVoteCounter(voted, invited),
          style: TextStyle(
            color: AppColors.modalTextPrimary,
            fontSize: 11,
            fontWeight: FontWeight.w700,
            letterSpacing: 0.8,
          ),
        ),
        if (includingYou) ...[
          Text(
            '  ·  ',
            style: TextStyle(
              color: AppColors.modalTextMuted,
              fontSize: 11,
              fontWeight: FontWeight.w700,
            ),
          ),
          Text(
            l10n.locationPollVoteCounterYou,
            style: TextStyle(
              color: AppColors.modalTextPrimary,
              fontSize: 11,
              fontWeight: FontWeight.w700,
              letterSpacing: 0.8,
            ),
          ),
        ],
      ],
    );
  }
}

/// Vote row — auto-submits on tap. Two visual states only: idle / voted.
class _VoteLocRow extends ConsumerWidget {
  final LocationProposal proposal;
  final bool voted;
  final bool leading;
  final bool showAddedBy;
  final bool enabled;
  final VoidCallback onToggle;

  const _VoteLocRow({
    required this.proposal,
    required this.voted,
    required this.leading,
    required this.showAddedBy,
    required this.enabled,
    required this.onToggle,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final accent = AppColors.lightAccent;

    final Color bgColor;
    final Color borderColor;
    final Color nameColor;
    final Color addrColor;
    if (voted) {
      bgColor = accent.withValues(alpha: 0.22);
      borderColor = accent.withValues(alpha: 0.55);
      nameColor = AppColors.modalTextPrimary;
      addrColor = AppColors.modalTextSecondary;
    } else {
      bgColor = AppColors.surface(context).withValues(alpha: 0.06);
      borderColor = leading
          ? AppColors.statusWarningOnDark.withValues(alpha: 0.45)
          : AppColors.border(context);
      nameColor = AppColors.modalTextPrimary;
      addrColor = AppColors.modalTextSecondary;
    }

    return Padding(
      padding: const EdgeInsets.only(bottom: 9),
      child: Toggle(
        semanticsLabel: _semanticLabel(context),
        selected: voted,
        onTap: enabled ? onToggle : null,
        child: Stack(
          clipBehavior: Clip.none,
          children: [
            Container(
              decoration: BoxDecoration(
                color: bgColor,
                border: Border.all(
                  color: borderColor,
                  width: voted ? 1.5 : 1,
                ),
                borderRadius: BorderRadius.circular(16),
              ),
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.center,
                children: [
                  _Checkbox(voted: voted),
                  const SizedBox(width: 11),
                  Expanded(
                    child: _RowContent(
                      proposal: proposal,
                      nameColor: nameColor,
                      addrColor: addrColor,
                      showAddedBy: showAddedBy,
                    ),
                  ),
                  const SizedBox(width: 6),
                  _VoterStack(votes: proposal.votes),
                ],
              ),
            ),
            if (leading)
              Positioned(
                top: -8,
                left: 12,
                child: Container(
                  padding:
                      const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
                  decoration: BoxDecoration(
                    color: AppColors.statusWarningOnDark,
                    borderRadius: BorderRadius.circular(4),
                  ),
                  child: Text(
                    context.l10n.locationPollVoteLeadingBadge.toUpperCase(),
                    style: const TextStyle(
                      color: Color(0xFF3B2A12),
                      fontSize: 9,
                      fontWeight: FontWeight.w800,
                      letterSpacing: 0.8,
                    ),
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }

  String _semanticLabel(BuildContext context) {
    if (proposal.location.locationId.isNotEmpty) {
      return proposal.location.locationId;
    }
    if (proposal.location.hasGeocoded() &&
        proposal.location.geocoded.name.isNotEmpty) {
      return proposal.location.geocoded.name;
    }
    return context.l10n.locationPollProposeAddSpot;
  }
}

/// Rounded-square checkbox with two states: idle / voted.
class _Checkbox extends StatelessWidget {
  final bool voted;
  const _Checkbox({required this.voted});

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.lightAccent;
    return Container(
      width: 24,
      height: 24,
      decoration: BoxDecoration(
        color: voted ? accent : Colors.transparent,
        border: voted
            ? null
            : Border.all(
                color: AppColors.modalTextPrimary.withValues(alpha: 0.55),
                width: 2,
              ),
        borderRadius: BorderRadius.circular(6),
      ),
      alignment: Alignment.center,
      child: voted
          ? const Icon(Icons.check, size: 14, color: Color(0xFF0F1A14))
          : null,
    );
  }
}

class _RowContent extends ConsumerWidget {
  final LocationProposal proposal;
  final Color nameColor;
  final Color addrColor;
  final bool showAddedBy;

  const _RowContent({
    required this.proposal,
    required this.nameColor,
    required this.addrColor,
    required this.showAddedBy,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final loc = proposal.location;
    if (loc.locationId.isNotEmpty) {
      return FutureBuilder<locapi.Location>(
        future: ref.read(locationRepositoryProvider).get(loc.locationId),
        builder: (context, snap) {
          final saved = snap.data;
          final name = _resolveSavedName(saved) ?? loc.locationId;
          final addr = _resolveSavedAddress(saved);
          return _renderText(context, name, addr);
        },
      );
    }
    if (loc.hasGeocoded()) {
      final g = loc.geocoded;
      final name = g.name.isNotEmpty
          ? g.name
          : (g.addressLines.isNotEmpty ? g.addressLines.first : g.locality);
      final addrParts = <String>[];
      if (g.addressLines.isNotEmpty && g.name.isNotEmpty) {
        addrParts.add(g.addressLines.first);
      }
      if (g.locality.isNotEmpty) addrParts.add(g.locality);
      return _renderText(context, name, addrParts.join(', '));
    }
    return _renderText(context, '—', null);
  }

  Widget _renderText(BuildContext context, String name, String? addr) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          name,
          style: TextStyle(
            color: nameColor,
            fontSize: 15,
            fontWeight: FontWeight.w700,
            height: 1.15,
          ),
          overflow: TextOverflow.ellipsis,
        ),
        if (addr != null && addr.isNotEmpty) ...[
          const SizedBox(height: 2),
          Text(
            addr,
            style: TextStyle(
              color: addrColor,
              fontSize: 12,
            ),
            overflow: TextOverflow.ellipsis,
          ),
        ],
        if (showAddedBy && proposal.proposedBy.name.isNotEmpty) ...[
          const SizedBox(height: 4),
          _AddedByPill(user: proposal.proposedBy),
        ],
      ],
    );
  }

  String? _resolveSavedName(locapi.Location? saved) {
    if (saved == null) return null;
    if (saved.hasName() && saved.name.isNotEmpty) return saved.name;
    if (saved.addressLines.isNotEmpty) return saved.addressLines.first;
    if (saved.locality.isNotEmpty) return saved.locality;
    return null;
  }

  String? _resolveSavedAddress(locapi.Location? saved) {
    if (saved == null) return null;
    final parts = <String>[];
    if (saved.addressLines.isNotEmpty && saved.hasName()) {
      parts.add(saved.addressLines.first);
    }
    if (saved.locality.isNotEmpty) parts.add(saved.locality);
    return parts.isEmpty ? null : parts.join(', ');
  }
}

/// Subtle text-only "Added by X" attribution line. No pill, no avatar —
/// the row is already information-dense, so this stays in the visual
/// hierarchy below the address.
class _AddedByPill extends StatelessWidget {
  final User user;
  const _AddedByPill({required this.user});

  @override
  Widget build(BuildContext context) {
    return Text(
      context.l10n.locationPollVoteAddedBy(user.name),
      style: TextStyle(
        color: AppColors.modalTextMuted,
        fontSize: 11,
        fontWeight: FontWeight.w500,
      ),
    );
  }
}

/// Small overlapping stack of YES-voter avatars with a count chip.
class _VoterStack extends StatelessWidget {
  final List<LocationVote> votes;
  const _VoterStack({required this.votes});

  @override
  Widget build(BuildContext context) {
    final yesVotes = votes
        .where((v) => v.status == LocationVoteStatus.LOCATION_VOTE_STATUS_YES)
        .toList();
    if (yesVotes.isEmpty) return const SizedBox.shrink();

    final shown = yesVotes.take(3).toList();
    // Avatar radius 9 → diameter 18. Overlap by 7px so neighbors visibly stack.
    const double avatarDiameter = 18;
    const double overlap = 7;
    final stackWidth =
        avatarDiameter + (shown.length - 1) * (avatarDiameter - overlap);
    return Container(
      padding: const EdgeInsets.fromLTRB(4, 3, 6, 3),
      decoration: BoxDecoration(
        color: AppColors.surface(context).withValues(alpha: 0.06),
        borderRadius: BorderRadius.circular(100),
      ),
      child: Row(
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
      ),
    );
  }
}

enum _PillVariant { primary, ghost }

class _PillButton extends StatelessWidget {
  final String label;
  final _PillVariant variant;
  final IconData? leadingIcon;

  const _PillButton({
    required this.label,
    this.variant = _PillVariant.primary,
    this.leadingIcon,
  });

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.lightAccent;
    final isPrimary = variant == _PillVariant.primary;
    final fg = isPrimary
        ? AppColors.cardBackground(context)
        : AppColors.modalTextPrimary;
    return Container(
      height: 52,
      decoration: BoxDecoration(
        color: isPrimary ? accent : Colors.transparent,
        border: isPrimary ? null : Border.all(color: AppColors.border(context)),
        borderRadius: BorderRadius.circular(16),
        boxShadow: isPrimary
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
      child: Row(
        mainAxisAlignment: MainAxisAlignment.center,
        mainAxisSize: MainAxisSize.min,
        children: [
          if (leadingIcon != null) ...[
            Icon(leadingIcon, size: 16, color: fg),
            const SizedBox(width: 8),
          ],
          Text(
            label,
            style: TextStyle(
              color: fg,
              fontWeight: FontWeight.w700,
              fontSize: 15,
              letterSpacing: 0.1,
            ),
          ),
        ],
      ),
    );
  }
}

