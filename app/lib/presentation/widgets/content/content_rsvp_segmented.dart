import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';

/// One option in a [ContentRsvpSegmented] control.
class ContentSegment {
  /// Visible label (caller-resolved), e.g. "Going".
  final String label;

  /// Screen-reader label (caller-resolved from `context.l10n`).
  final String semanticsLabel;

  /// Whether this option is the current selection.
  final bool selected;

  /// Tap handler — wires to the RSVP update.
  final VoidCallback onTap;

  const ContentSegment({
    required this.label,
    required this.semanticsLabel,
    required this.selected,
    required this.onTap,
  });
}

/// ContentRsvpSegmented is a subtle inline segmented control — e.g. Going /
/// Maybe / Can't go — for the expanded pitching-in panel
/// (docs/issues/2280-pitching-in-expand.md). The selected segment is filled
/// with the accent; the rest read as muted text on the dark track. Each segment
/// is a [Toggle] so its selected state is announced.
class ContentRsvpSegmented extends StatelessWidget {
  final List<ContentSegment> segments;
  final Color accentColor;

  const ContentRsvpSegmented({
    super.key,
    required this.segments,
    required this.accentColor,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(4),
      decoration: BoxDecoration(
        color: AppColors.darkTextPrimary.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        children: [
          for (final segment in segments)
            Expanded(child: _segment(segment)),
        ],
      ),
    );
  }

  Widget _segment(ContentSegment segment) {
    return Toggle(
      semanticsLabel: segment.semanticsLabel,
      selected: segment.selected,
      onTap: segment.onTap,
      child: Container(
        height: 40,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: segment.selected ? accentColor : Colors.transparent,
          borderRadius: BorderRadius.circular(9),
        ),
        child: Text(
          segment.label,
          style: TextStyle(
            color: segment.selected
                ? AppColors.darkBackground
                : AppColors.darkTextSecondary,
            fontSize: 14.5,
            fontWeight: FontWeight.w700,
          ),
        ),
      ),
    );
  }
}
