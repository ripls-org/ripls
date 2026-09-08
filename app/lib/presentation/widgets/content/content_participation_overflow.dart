import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// The most item rows a collapsed participation card (the "who's in" / "needs"
/// card on a request or experience content view) renders before it folds the
/// rest behind a "+N more" indicator. Kept under five so the card stays short
/// and the content view's hero is not pushed off-screen.
///
/// Shared by the request and experience read shells so both content views cap
/// their participation lists identically — the Needs/Contributions parity
/// contract (docs/client/needs.md) treats per-scope divergence as a bug.
const int kParticipationCardMaxItems = 4;

/// The muted "{label} ›" overflow indicator shown at the foot of a collapsed
/// participation card once its list is capped at [kParticipationCardMaxItems].
/// The row is display-only — the card itself is the tap target that opens the
/// expanded panel. [label] is the already-localized count phrase (e.g.
/// "3 more"), so the caller resolves the right per-surface string.
Widget contentParticipationMoreRow(String label) {
  return Padding(
    padding: const EdgeInsets.only(top: 10),
    child: Text(
      '$label ›',
      style: TextStyle(
        color: AppColors.onContentImage.withValues(alpha: 0.6),
        fontSize: 13.5,
        fontWeight: FontWeight.w500,
      ),
    ),
  );
}
