import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart' show TimeProposal;
import 'package:ripls/data/gen/ripls/api/time.pbenum.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Displays a "Poll is live" banner at the top of the Experience Plan tab.
///
/// Shown when [experience.timePollActive] is true. The banner shifts to a
/// "Voted ✓" sage state once the current user has cast at least one YES vote.
/// Tapping opens the standalone vote modal via [onTap].
class PollBanner extends ConsumerWidget {
  const PollBanner({
    super.key,
    required this.experienceId,
    required this.currentUserId,
    required this.onTap,
  });

  final String experienceId;
  final String currentUserId;
  final VoidCallback onTap;

  /// Returns true when the current user has at least one YES vote across all
  /// proposals. NONE_WORK and UNSPECIFIED do not count as voted.
  static bool hasVotedYes(
    List<TimeProposal> proposals,
    String currentUserId,
  ) {
    for (final p in proposals) {
      for (final v in p.votes) {
        if (v.user.id == currentUserId &&
            v.status == TimeVoteStatus.TIME_VOTE_STATUS_YES) {
          return true;
        }
      }
    }
    return false;
  }

  /// Counts unique voters who have cast a YES vote across all proposals.
  static int countYesVoters(List<TimeProposal> proposals) {
    final voterIds = <String>{};
    for (final p in proposals) {
      for (final v in p.votes) {
        if (v.status == TimeVoteStatus.TIME_VOTE_STATUS_YES) {
          voterIds.add(v.user.id);
        }
      }
    }
    return voterIds.length;
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(experienceProvider(experienceId));
    final exp = state.experienceDetails?.experience;
    if (exp == null || !exp.timePollActive) return const SizedBox.shrink();

    // Scope to the currently-active poll only. Historical polls' proposals
    // and votes are still in `timeProposals` (so users can browse past
    // results), but counting them here would inflate the option/vote totals
    // and falsely flash "Voted ✓" when the user voted in an earlier poll.
    final activePollId = exp.hasCurrentPollId() ? exp.currentPollId : '';
    final proposals = activePollId.isEmpty
        ? exp.timeProposals
        : exp.timeProposals
            .where((p) => p.hasPollId() && p.pollId == activePollId)
            .toList();
    final optionCount = proposals.length;
    final voteCount = countYesVoters(proposals);
    final voted = hasVotedYes(proposals, currentUserId);

    final accentColor =
        voted ? AppColors.experienceSageGreen : AppColors.transferCoral;

    return Tappable(
      semanticsLabel: context.l10n.a11yMiscPollBanner,
      onTap: onTap,
      excludeChildSemantics: false,
      child: Container(
        margin: const EdgeInsets.only(bottom: 16),
        decoration: BoxDecoration(
          color: accentColor.withValues(alpha: 0.28),
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: accentColor.withValues(alpha: 0.40)),
        ),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        child: Row(
          children: [
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    context.l10n.planPollBannerEyebrow,
                    style: TextStyle(
                      fontSize: 11,
                      fontWeight: FontWeight.w700,
                      color: accentColor,
                      letterSpacing: 0.5,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    context.l10n.planPollBannerHeadline,
                    style: const TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                      color: AppColors.onContentImage,
                    ),
                  ),
                  const SizedBox(height: 3),
                  Text(
                    context.l10n.planPollBannerMeta(voteCount, optionCount),
                    style: const TextStyle(
                      fontSize: 12,
                      color: AppColors.onContentImage,
                    ),
                  ),
                ],
              ),
            ),
            Container(
              padding:
                  const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
              decoration: BoxDecoration(
                color: accentColor,
                borderRadius: BorderRadius.circular(999),
              ),
              child: Text(
                voted
                    ? context.l10n.planPollBannerVotedLabel
                    : context.l10n.planPollBannerCta,
                style: const TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w700,
                  color: AppColors.onContentImage,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Displays a "Poll is live · Where to meet?" banner at the top of the
/// Experience Plan tab when a location poll is active. Mirrors [PollBanner]
/// but for the location-poll flow.
///
/// The banner shifts to a "Voted ✓" sage state once the current user has
/// cast at least one YES vote on the currently active poll. Tapping opens
/// the location vote modal via [onTap].
class LocationPollBanner extends ConsumerWidget {
  const LocationPollBanner({
    super.key,
    required this.experienceId,
    required this.currentUserId,
    required this.onTap,
  });

  final String experienceId;
  final String currentUserId;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(experienceProvider(experienceId));
    final exp = state.experienceDetails?.experience;
    if (exp == null || !exp.locationPollActive) {
      return const SizedBox.shrink();
    }

    // Scope to the currently-active poll only — historical polls' votes
    // shouldn't flip the banner to "Voted ✓" on a freshly-opened poll.
    final activePollId =
        exp.hasCurrentLocationPollId() ? exp.currentLocationPollId : '';
    final proposals = activePollId.isEmpty
        ? exp.locationProposals
        : exp.locationProposals
            .where((p) => p.hasPollId() && p.pollId == activePollId)
            .toList();

    final voterIds = <String>{};
    var voted = false;
    for (final p in proposals) {
      for (final v in p.votes) {
        if (v.user.id == currentUserId) voted = true;
        voterIds.add(v.user.id);
      }
    }

    final accentColor =
        voted ? AppColors.experienceSageGreen : AppColors.transferCoral;

    return Tappable(
      semanticsLabel: context.l10n.locationPollBannerLive,
      onTap: onTap,
      excludeChildSemantics: false,
      child: Container(
        margin: const EdgeInsets.only(bottom: 16),
        decoration: BoxDecoration(
          color: accentColor.withValues(alpha: 0.28),
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: accentColor.withValues(alpha: 0.40)),
        ),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        child: Row(
          children: [
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    context.l10n.locationPollBannerLive,
                    style: TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w700,
                      color: AppColors.onContentImage,
                    ),
                  ),
                  const SizedBox(height: 3),
                  Text(
                    '${voterIds.length} · ${proposals.length}',
                    style: const TextStyle(
                      fontSize: 12,
                      color: AppColors.onContentImage,
                    ),
                  ),
                ],
              ),
            ),
            Container(
              padding:
                  const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
              decoration: BoxDecoration(
                color: accentColor,
                borderRadius: BorderRadius.circular(999),
              ),
              child: Text(
                voted
                    ? context.l10n.locationPollBannerVoted
                    : context.l10n.locationPollBannerVoteCta,
                style: const TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w700,
                  color: AppColors.onContentImage,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Owner-only "Poll ended · Pick the winning spot" banner. Mirrors
/// [PollEndedBanner] but for the location-poll flow.
class LocationPollEndedBanner extends ConsumerWidget {
  const LocationPollEndedBanner({
    super.key,
    required this.experienceId,
    required this.currentUserId,
    required this.onTap,
  });

  final String experienceId;
  final String currentUserId;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(experienceProvider(experienceId));
    final exp = state.experienceDetails?.experience;
    if (exp == null) return const SizedBox.shrink();

    final isOwner = exp.owner.id == currentUserId;
    final pollEnded =
        (exp.hasLocationPollCompleted() && exp.locationPollCompleted) &&
            !exp.locationPollActive;
    final hasLocation = exp.locationId.isNotEmpty;
    if (!isOwner || !pollEnded || hasLocation) {
      return const SizedBox.shrink();
    }

    return Tappable(
      semanticsLabel: context.l10n.locationPollBannerEnded,
      onTap: onTap,
      excludeChildSemantics: false,
      child: Container(
        margin: const EdgeInsets.only(bottom: 16),
        decoration: BoxDecoration(
          color: AppColors.experienceSageGreen.withValues(alpha: 0.15),
          borderRadius: BorderRadius.circular(14),
          border: Border.all(
            color: AppColors.experienceSageGreen.withValues(alpha: 0.40),
          ),
        ),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        child: Row(
          children: [
            Expanded(
              child: Text(
                context.l10n.locationPollBannerEnded,
                style: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: AppColors.onContentImage,
                ),
              ),
            ),
            Icon(
              Icons.chevron_right,
              color: AppColors.experienceSageGreen,
              size: 20,
            ),
          ],
        ),
      ),
    );
  }
}

/// Displays a sage "Poll ended · Pick the winning time" banner for the owner
/// when a poll has completed but no time has been confirmed yet.
///
/// Only visible to the experience owner. Tapping opens the confirm-winner modal
/// via [onTap].
class PollEndedBanner extends ConsumerWidget {
  const PollEndedBanner({
    super.key,
    required this.experienceId,
    required this.currentUserId,
    required this.onTap,
  });

  final String experienceId;
  final String currentUserId;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(experienceProvider(experienceId));
    final exp = state.experienceDetails?.experience;
    if (exp == null) return const SizedBox.shrink();

    final isOwner = exp.owner.id == currentUserId;
    final pollEnded = exp.timePollCompleted && !exp.timePollActive;
    final hasTime = exp.hasTime() && (exp.time.hasSpecific() || exp.time.hasRange());
    if (!isOwner || !pollEnded || hasTime) return const SizedBox.shrink();

    return Tappable(
      semanticsLabel: context.l10n.a11yMiscPollResults,
      onTap: onTap,
      excludeChildSemantics: false,
      child: Container(
        margin: const EdgeInsets.only(bottom: 16),
        decoration: BoxDecoration(
          color: AppColors.experienceSageGreen.withValues(alpha: 0.15),
          borderRadius: BorderRadius.circular(14),
          border: Border.all(
            color: AppColors.experienceSageGreen.withValues(alpha: 0.40),
          ),
        ),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        child: Row(
          children: [
            Expanded(
              child: Text(
                context.l10n.planPollConfirmBannerOwner,
                style: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: AppColors.onContentImage,
                ),
              ),
            ),
            Icon(
              Icons.chevron_right,
              color: AppColors.experienceSageGreen,
              size: 20,
            ),
          ],
        ),
      ),
    );
  }
}
