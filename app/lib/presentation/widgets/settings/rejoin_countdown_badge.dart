import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// RejoinCountdownBadge renders the days-left label for a recently-left
/// community in the rejoin window. The visible text is the canonical
/// signal — colour is secondary — so screen-reader users hear the same
/// urgency cue sighted users see.
class RejoinCountdownBadge extends StatelessWidget {
  /// Days remaining before the 30-day rejoin window expires. Clamp to
  /// a non-negative value before passing in.
  final int daysLeft;

  const RejoinCountdownBadge({super.key, required this.daysLeft});

  @override
  Widget build(BuildContext context) {
    final label = context.l10n.communityRejoinCountdownBadge(daysLeft);
    return Semantics(
      label: label,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
        decoration: BoxDecoration(
          color: AppColors.statusSuccess(context).withValues(alpha: 0.15),
          borderRadius: BorderRadius.circular(10),
        ),
        child: Text(
          label,
          style: Theme.of(context).textTheme.bodySmall?.copyWith(
                color: AppColors.statusSuccess(context),
                fontWeight: FontWeight.w600,
              ),
        ),
      ),
    );
  }
}

/// Computes the days remaining before the rejoin window expires.
///
/// `leftAtUnixSec` is the soft-delete timestamp on the caller's
/// CommunityUser row; `nowUnixSec` is the current wall clock
/// (parameterised so tests can pin a time). Returns a value in
/// `[0, windowDays]` — the upper clamp guards against client/server
/// clock skew where the server records `leftAtUnixSec` a fraction of
/// a second after the client took its `now` reading, which would
/// otherwise round up to `windowDays + 1`.
int daysLeftUntilRejoinExpiry({
  required int leftAtUnixSec,
  required int nowUnixSec,
  int windowDays = 30,
}) {
  const secondsPerDay = 86400;
  final expiresAt = leftAtUnixSec + windowDays * secondsPerDay;
  final secondsLeft = expiresAt - nowUnixSec;
  if (secondsLeft <= 0) return 0;
  // Ceiling division so 29.4 days reads as "1 day left", not "0".
  final raw = (secondsLeft + secondsPerDay - 1) ~/ secondsPerDay;
  return raw > windowDays ? windowDays : raw;
}
