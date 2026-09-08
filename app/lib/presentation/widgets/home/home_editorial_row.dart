import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// HomeEditorialRow is the simplified inbox list row used across Needs-you and
/// Yours on the editorial Home: a serif title, a muted subtitle, and either a
/// tappable sage action pill or a plain "when" date on the right. No
/// thumbnails — the layout stays text-forward and quiet.
class HomeEditorialRow extends StatelessWidget {
  final String title;
  final String subtitle;

  /// Tapping the row body — opens the underlying item.
  final VoidCallback onTap;

  /// A sage action pill (e.g. "Pick up", "Say thanks"). When set, [onPill]
  /// runs the typed action.
  final String? pillLabel;
  final VoidCallback? onPill;

  /// A plain date shown on the right (e.g. "May 29") for Yours rows.
  final String? whenLabel;

  /// Leading bullet color. Needs-you rows use a dark heritage sage; Yours
  /// rows a lighter sage. Null hides the bullet.
  final Color? dotColor;

  /// Hairline divider above the row (false on the first row of a section).
  final bool showTopBorder;

  const HomeEditorialRow({
    super.key,
    required this.title,
    required this.subtitle,
    required this.onTap,
    this.pillLabel,
    this.onPill,
    this.whenLabel,
    this.dotColor,
    this.showTopBorder = true,
  });

  @override
  Widget build(BuildContext context) {
    // The action pill is a SIBLING of the row's Tappable, not a child. The
    // row Tappable excludes descendant semantics (Tappable's default), so a
    // nested pill has no semantics node of its own: on Flutter Web the row's
    // flt-semantics element then covers the pill's area and every click runs
    // the row action instead — the pill is untappable on web and invisible
    // to screen readers. Same failure class the gear who-card CTA documents.
    return Container(
      decoration: BoxDecoration(
        border: showTopBorder
            ? Border(top: BorderSide(color: AppColors.border(context)))
            : null,
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Tappable(
              semanticsLabel: title,
              onTap: onTap,
              inkBorderRadius: BorderRadius.zero,
              child: Padding(
                padding: const EdgeInsets.symmetric(vertical: 12),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    if (dotColor != null) ...[
                      Padding(
                        padding: const EdgeInsets.only(top: 6),
                        child: Container(
                          width: 7,
                          height: 7,
                          decoration: BoxDecoration(
                              color: dotColor, shape: BoxShape.circle),
                        ),
                      ),
                      const SizedBox(width: 12),
                    ],
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            title,
                            maxLines: 2,
                            overflow: TextOverflow.ellipsis,
                            style: TextStyle(
                              fontFamily: AppTheme.headingFont,
                              fontSize: 16,
                              fontWeight: FontWeight.w500,
                              height: 1.2,
                              color: AppColors.textPrimary(context),
                            ),
                          ),
                          if (subtitle.isNotEmpty) ...[
                            const SizedBox(height: 2),
                            Text(
                              subtitle,
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                              style: TextStyle(
                                fontSize: 12,
                                color: AppColors.textSecondary(context),
                              ),
                            ),
                          ],
                        ],
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
          if (pillLabel != null) ...[
            const SizedBox(width: 10),
            Padding(
              padding: const EdgeInsets.only(top: 12),
              child: _buildPill(context, pillLabel!),
            ),
          ] else if (whenLabel != null && whenLabel!.isNotEmpty) ...[
            const SizedBox(width: 10),
            Padding(
              padding: const EdgeInsets.only(top: 13),
              child: Text(
                whenLabel!,
                style: TextStyle(
                  fontSize: 12,
                  fontWeight: FontWeight.w600,
                  color: AppColors.primary(context),
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildPill(BuildContext context, String label) {
    return Tappable(
      semanticsLabel: label,
      onTap: onPill ?? onTap,
      inkBorderRadius: BorderRadius.circular(16),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 11, vertical: 5),
        decoration: BoxDecoration(
          color: AppColors.primary(context).withAlpha(28),
          borderRadius: BorderRadius.circular(16),
        ),
        child: Text(
          label,
          style: TextStyle(
            fontSize: 11.5,
            fontWeight: FontWeight.w600,
            color: AppColors.primary(context),
          ),
        ),
      ),
    );
  }
}
