import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';

/// GlassModalHeader is the kicker + value block at the top of a glass bottom
/// sheet. The reference uses a circular icon badge to the left of an
/// UPPERCASE kicker label (e.g. "WHEN") with a single-line value below
/// (e.g. "Fri, May 8 · 9:00 AM · 1 hr").
///
/// The value text is wrapped in `Semantics(header: true)` so screen readers
/// announce it as the modal heading — matching the existing `ModalHeader`
/// accessibility contract.
///
/// See docs/issues/1797-glass-modal-revamp.md.
class GlassModalHeader extends StatelessWidget {
  /// Optional decorative icon shown in the circular badge. When null the badge
  /// is omitted entirely.
  final IconData? icon;

  /// Optional override for the icon color. Defaults to [AppColors.modalTextPrimary].
  final Color? iconColor;

  /// UPPERCASE kicker label. Pass an already-localized string (e.g.
  /// `context.l10n.experienceModalKickerWhen`).
  final String kicker;

  /// The single-line value rendered below the kicker.
  final String value;

  /// Optional widget rendered to the right (e.g. close button or status pill).
  final Widget? trailing;

  const GlassModalHeader({
    super.key,
    required this.kicker,
    required this.value,
    this.icon,
    this.iconColor,
    this.trailing,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(4, 0, 4, 20),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          if (icon != null) ...[
            _IconBadge(icon: icon!, color: iconColor),
            const SizedBox(width: 12),
          ],
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  kicker.toUpperCase(),
                  style: ModalTheme.kickerStyle.copyWith(
                    color: AppColors.modalTextMuted,
                  ),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
                const SizedBox(height: 4),
                Semantics(
                  header: true,
                  child: Text(
                    value,
                    style: ModalTheme.headerValueStyle.copyWith(
                      color: AppColors.modalTextPrimary,
                    ),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ],
            ),
          ),
          if (trailing != null) ...[const SizedBox(width: 12), trailing!],
        ],
      ),
    );
  }
}

class _IconBadge extends StatelessWidget {
  final IconData icon;
  final Color? color;

  const _IconBadge({required this.icon, this.color});

  @override
  Widget build(BuildContext context) {
    return Container(
      width: ModalTheme.headerIconBadgeSize,
      height: ModalTheme.headerIconBadgeSize,
      decoration: BoxDecoration(
        color: AppColors.modalIconBadgeBackground,
        shape: BoxShape.circle,
        border: Border.all(color: AppColors.modalIconBadgeBorder, width: 1),
      ),
      child: Icon(
        icon,
        size: 18,
        color: color ?? AppColors.modalTextPrimary,
      ),
    );
  }
}
