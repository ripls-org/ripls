import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// A faint-accent-bordered "add another option" affordance.
///
/// Two layouts share the same shell:
/// - `big: true` — empty-state card with a large icon, a bold heading, and
///   an optional centered hint.
/// - `big: false` — compact row with a small icon-circle and a muted label.
///
/// Used by the location-poll, time-poll, and needs-volunteer flows.
class AddOptionCard extends StatelessWidget {
  final bool big;
  final String label;
  final String? hint;
  final IconData icon;
  final Color accent;
  final String? semanticsLabel;
  final VoidCallback? onTap;

  const AddOptionCard({
    super.key,
    this.big = false,
    required this.label,
    this.hint,
    required this.icon,
    required this.onTap,
    Color? accent,
    this.semanticsLabel,
  }) : accent = accent ?? AppColors.lightAccent;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: semanticsLabel ?? label,
      onTap: onTap,
      child: Container(
        padding: EdgeInsets.symmetric(
          horizontal: big ? 18 : 16,
          vertical: big ? 22 : 12,
        ),
        decoration: BoxDecoration(
          color: big
              ? AppColors.surface(context).withValues(alpha: 0.04)
              : Colors.transparent,
          border: Border.all(
            color: accent.withValues(alpha: 0.40),
            width: 1.5,
          ),
          borderRadius: BorderRadius.circular(big ? 18 : 14),
        ),
        child: big ? _buildBig() : _buildCompact(),
      ),
    );
  }

  Widget _buildBig() {
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(
          width: 52,
          height: 52,
          decoration: BoxDecoration(
            color: accent.withValues(alpha: 0.22),
            border: Border.all(
              color: accent.withValues(alpha: 0.40),
              width: 1.5,
            ),
            shape: BoxShape.circle,
          ),
          alignment: Alignment.center,
          // Foreground icons render in white against the frosted-glass
          // backdrop — the sage accent doesn't have enough contrast.
          child: Icon(icon, size: 22, color: AppColors.modalTextPrimary),
        ),
        const SizedBox(height: 10),
        Text(
          label,
          style: const TextStyle(
            color: AppColors.modalTextPrimary,
            fontWeight: FontWeight.w700,
            fontSize: 17,
          ),
        ),
        if (hint != null) ...[
          const SizedBox(height: 6),
          SizedBox(
            width: 250,
            child: Text(
              hint!,
              textAlign: TextAlign.center,
              style: const TextStyle(
                color: AppColors.modalTextSecondary,
                fontSize: 12.5,
                height: 1.4,
              ),
            ),
          ),
        ],
      ],
    );
  }

  Widget _buildCompact() {
    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(
          width: 20,
          height: 20,
          decoration: BoxDecoration(
            color: accent.withValues(alpha: 0.22),
            border: Border.all(
              color: accent.withValues(alpha: 0.40),
              width: 1,
            ),
            shape: BoxShape.circle,
          ),
          alignment: Alignment.center,
          child: Icon(icon, size: 11, color: AppColors.modalTextPrimary),
        ),
        const SizedBox(width: 8),
        // Flexible + ellipsis so the row never overflows when the
        // label is too long for the parent's width (e.g. the Volunteer
        // sheet's two-column bottom action row, where this card sits
        // next to a Manage pill at ~172px).
        Flexible(
          child: Text(
            label,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              color: AppColors.modalTextPrimary,
              fontWeight: FontWeight.w600,
              fontSize: 13,
            ),
          ),
        ),
      ],
    );
  }
}
