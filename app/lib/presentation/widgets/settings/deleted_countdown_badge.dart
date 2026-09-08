import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// DeletedCountdownBadge renders the days-left label for a soft-deleted
/// community. The visible text is the canonical signal — colour is
/// secondary — so screen-reader users hear the same urgency cue
/// sighted users see.
class DeletedCountdownBadge extends StatelessWidget {
  /// Days remaining before the community is hard-deleted. Clamp to a
  /// non-negative value before passing in.
  final int daysLeft;

  const DeletedCountdownBadge({super.key, required this.daysLeft});

  @override
  Widget build(BuildContext context) {
    final label = context.l10n.communityDeletedCountdownBadge(daysLeft);
    return Semantics(
      label: label,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
        decoration: BoxDecoration(
          color: AppColors.statusWarning(context).withValues(alpha: 0.15),
          borderRadius: BorderRadius.circular(10),
        ),
        child: Text(
          label,
          style: Theme.of(context).textTheme.bodySmall?.copyWith(
                color: AppColors.statusWarning(context),
                fontWeight: FontWeight.w600,
              ),
        ),
      ),
    );
  }
}

/// Computes the days remaining before a community is hard-deleted.
///
/// `deletedAtUnixSec` is the soft-delete timestamp; `nowUnixSec` is
/// the current wall clock (parameterised so tests can pin a time).
/// Returns a value in `[0, ∞)` — the caller should not pass negative
/// counts to the badge.
int daysLeftUntilPurge({
  required int deletedAtUnixSec,
  required int nowUnixSec,
  int windowDays = 30,
}) {
  const secondsPerDay = 86400;
  final purgeAt = deletedAtUnixSec + windowDays * secondsPerDay;
  final secondsLeft = purgeAt - nowUnixSec;
  if (secondsLeft <= 0) return 0;
  // Ceiling division so 29.4 days reads as "1 day left", not "0".
  return (secondsLeft + secondsPerDay - 1) ~/ secondsPerDay;
}
