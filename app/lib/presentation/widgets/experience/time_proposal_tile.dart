import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart'
    show TimeProposal, TimeVote, TimeVoteStatus;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// TimeProposalTile renders a single time proposal row used in the vote
/// modal, confirm modal, and any other time-poll surface.
///
/// Layout:
/// - Left: radio indicator (empty circle or filled check when selected)
/// - Middle: date on row 1 (bold), time on row 2 (secondary)
/// - Right: overlapping voter avatar stack + yes-voter count
class TimeProposalTile extends StatelessWidget {
  const TimeProposalTile({
    super.key,
    required this.proposal,
    required this.isSelected,
    this.isVoting = false,
    this.onTap,
    this.currentUser,
  });

  /// The time proposal to display.
  final TimeProposal proposal;

  /// Whether the current user has voted yes on (or selected) this proposal.
  final bool isSelected;

  /// Whether a vote network request is in flight for this proposal.
  final bool isVoting;

  /// Called when the user taps the tile. Null makes the tile non-interactive.
  final VoidCallback? onTap;

  /// The authenticated user. When provided, the voter stack reflects
  /// [isSelected] optimistically — the current user's avatar appears or
  /// disappears immediately on tap without waiting for the server round-trip.
  final User? currentUser;

  @override
  Widget build(BuildContext context) {
    final yesVoters = <TimeVote>[];
    for (final v in proposal.votes) {
      if (v.status != TimeVoteStatus.TIME_VOTE_STATUS_YES) continue;
      // Skip the current user here — we'll add them back below based on the
      // local selection state so taps feel instant.
      if (currentUser != null && v.user.id == currentUser!.id) continue;
      yesVoters.add(v);
    }
    if (isSelected && currentUser != null) {
      yesVoters.insert(
        0,
        TimeVote(
          user: currentUser,
          status: TimeVoteStatus.TIME_VOTE_STATUS_YES,
        ),
      );
    }

    final dateLabel = proposal.time.hasSpecific()
        ? DateFormat('EEE, MMM d, h:mm a').format(
            DateTime.fromMillisecondsSinceEpoch(
              proposal.time.specific.unixTimestampSec.toInt() * 1000,
            ),
          )
        : 'TBD';
    return Toggle(
      semanticsLabel: dateLabel,
      selected: isSelected,
      onTap: onTap,
      child: AnimatedContainer(
        duration: accessibleDuration(context, const Duration(milliseconds: 150)),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
        decoration: BoxDecoration(
          color: isSelected
              ? AppColors.transferSage.withValues(alpha: 0.18)
              : AppColors.modalInsetCardBg,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(
            color: isSelected
                ? AppColors.transferSage
                : AppColors.modalInsetCardBorder,
            width: isSelected ? 1.5 : 1.0,
          ),
        ),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            // Radio indicator on the left
            _buildIndicator(context),
            const SizedBox(width: 12),

            // Date + time column
            Expanded(child: _buildDateTimeColumn(context)),

            // Voter avatar stack + count
            if (yesVoters.isNotEmpty) ...[
              const SizedBox(width: 8),
              _buildVoterStack(yesVoters),
              const SizedBox(width: 5),
              Text(
                '${yesVoters.length}',
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: AppColors.modalTextPrimary,
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }

  Widget _buildDateTimeColumn(BuildContext context) {
    if (!proposal.time.hasSpecific()) {
      return Text(
        'TBD',
        style: TextStyle(
          fontSize: 15,
          fontWeight: FontWeight.w700,
          color: AppColors.modalTextPrimary,
        ),
      );
    }

    final dt = DateTime.fromMillisecondsSinceEpoch(
      proposal.time.specific.unixTimestampSec.toInt() * 1000,
    );
    final dateStr = DateFormat('EEE, MMM d').format(dt);
    final timeStr = DateFormat('h:mm a').format(dt);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          dateStr,
          style: TextStyle(
            fontSize: 15,
            fontWeight: FontWeight.w700,
            color: AppColors.modalTextPrimary,
          ),
        ),
        const SizedBox(height: 2),
        Text(
          timeStr,
          style: TextStyle(
            fontSize: 13,
            color: AppColors.modalTextSecondary,
          ),
        ),
      ],
    );
  }

  Widget _buildVoterStack(List<TimeVote> voters) {
    const avatarRadius = 10.0;
    const overlap = 5.0;
    const maxShown = 4;

    final shown = voters.take(maxShown).toList();
    final stackWidth = shown.length * (avatarRadius * 2 - overlap) + overlap;

    return SizedBox(
      width: stackWidth,
      height: avatarRadius * 2,
      child: Stack(
        clipBehavior: Clip.none,
        children: [
          for (int i = 0; i < shown.length; i++)
            Positioned(
              left: i * (avatarRadius * 2 - overlap),
              child: UserAvatar(user: shown[i].user, radius: avatarRadius),
            ),
        ],
      ),
    );
  }

  Widget _buildIndicator(BuildContext context) {
    return SizedBox(
      width: 24,
      height: 24,
      child: isVoting
          ? CircularProgressIndicator(
              strokeWidth: 2,
              valueColor: AlwaysStoppedAnimation<Color>(AppColors.transferSage),
            )
          : isSelected
              ? Container(
                  decoration: BoxDecoration(
                    color: AppColors.transferSage,
                    shape: BoxShape.circle,
                  ),
                  child: const Icon(Icons.check, color: Colors.white, size: 14),
                )
              : Container(
                  decoration: BoxDecoration(
                    color: Colors.white,
                    shape: BoxShape.circle,
                    border: Border.all(
                      color: AppColors.modalInsetCardBorder,
                      width: 1.5,
                    ),
                  ),
                ),
    );
  }
}
