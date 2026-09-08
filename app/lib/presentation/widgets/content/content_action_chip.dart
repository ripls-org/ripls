import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// ContentActionChip is the solid "sage" call-to-action chip on the redesigned
/// content sheet (event V6b · CTA at card bottom): a filled accent pill with
/// dark label text and a trailing chevron — the tap target that says *what to
/// do* ("Vote ›", "Set time ›", "RSVP — are you in? ›").
///
/// Always-on-dark and role-agnostic. The caller resolves [label] and
/// [semanticsLabel] (l10n) and supplies the action accent (heritage sage by
/// default on experiences). Use [fullWidth] for the roomy card-foot placement
/// (the Who's-in RSVP chip); leave it false for the compact in-box placement
/// (the Where/When fact chips).
class ContentActionChip extends StatelessWidget {
  /// Visible chip text, e.g. "Vote" or "RSVP — are you in?".
  final String label;

  /// Screen-reader label for the tap target (caller-resolved from
  /// `context.l10n`).
  final String semanticsLabel;

  final VoidCallback onTap;

  /// The action accent — the filled background color.
  final Color accentColor;

  /// When true the chip stretches to its parent's width and centers its
  /// content (card-foot placement); when false it hugs its label (in-box).
  final bool fullWidth;

  const ContentActionChip({
    super.key,
    required this.label,
    required this.semanticsLabel,
    required this.onTap,
    required this.accentColor,
    this.fullWidth = false,
  });

  @override
  Widget build(BuildContext context) {
    final chip = Container(
      padding: const EdgeInsets.symmetric(horizontal: 13, vertical: 7),
      decoration: BoxDecoration(
        color: accentColor,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Row(
        mainAxisSize: fullWidth ? MainAxisSize.max : MainAxisSize.min,
        mainAxisAlignment: fullWidth
            ? MainAxisAlignment.center
            : MainAxisAlignment.start,
        children: [
          Flexible(
            child: Text(
              label,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                color: AppColors.darkBackground,
                fontSize: 12.5,
                fontWeight: FontWeight.w700,
                height: 1.2,
              ),
            ),
          ),
          const SizedBox(width: 5),
          const Text(
            '›',
            style: TextStyle(
              color: AppColors.darkBackground,
              fontSize: 13,
              fontWeight: FontWeight.w800,
              height: 1,
            ),
          ),
        ],
      ),
    );

    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      child: fullWidth ? chip : Align(alignment: Alignment.centerLeft, child: chip),
    );
  }
}
