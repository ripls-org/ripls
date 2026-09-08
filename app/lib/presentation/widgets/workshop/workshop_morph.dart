import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_backdrop.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_overview_theme.dart';

/// Hosts an existing full-screen detail screen (deep dives, roster, known-for)
/// as a morph-reveal destination overlaid on the community photo (#2447).
///
/// Those screens use theme-aware [AppColors], so forcing [AppTheme.darkTheme]
/// flips their text to white-on-dark automatically. The screen is rendered with
/// a transparent scaffold (see each screen's `overlay` flag) over a painted
/// backdrop + readability scrim, so the photo reads behind it. The morph route
/// supplies the grow-from-rect animation; the screen's AppBar close pops it.
class WorkshopOverlayHost extends StatelessWidget {
  /// Backdrop media id (pinned community photo); null shows the ripples image.
  final String? backdropMediaId;
  final Widget child;

  const WorkshopOverlayHost({
    super.key,
    required this.backdropMediaId,
    required this.child,
  });

  @override
  Widget build(BuildContext context) {
    return Theme(
      data: AppTheme.darkTheme,
      child: Stack(
        fit: StackFit.expand,
        children: [
          const ColoredBox(color: Color(0xFF0A0C0A)),
          WorkshopBackdrop(mediaId: backdropMediaId),
          // Readability scrim — stronger than the overview's since full-screen
          // content sits directly on it (not in cards).
          const ColoredBox(color: Color(0xC20A0C0A)),
          child,
        ],
      ),
    );
  }
}

/// Shared chrome for the Workshop overview's morph-reveal destinations (#2447)
/// — roster, metric deep dives, known-for, library map/calendar.
///
/// Every destination grows from its source card's footprint and overlays the
/// community-photo backdrop alone (the overview hides its body while open). This
/// wraps the destination [child] in the shared [ContentMorphPanel] chrome
/// (strong scrim + horizontal swipe-to-close), with an optional serif [title]
/// header and a close (✕) in the upper-right. Content is white-on-transparent
/// so the photo reads behind it.
class WorkshopMorphPanel extends StatelessWidget {
  /// Optional serif header title (e.g. "Library · map").
  final String? title;

  /// Optional eyebrow above the title (uppercase, accent).
  final String? eyebrow;

  /// The destination body — typically a scrollable list/column.
  final Widget child;

  const WorkshopMorphPanel({
    super.key,
    this.title,
    this.eyebrow,
    required this.child,
  });

  @override
  Widget build(BuildContext context) {
    // Transparent header + body + close X. The opaque backdrop + scrim are
    // supplied by [WorkshopOverlayHost] (same as the deep-dive/roster/known-for
    // destinations) so the overview body and nav bars are fully covered. The
    // transparent Material provides the default text style so on-photo Text
    // doesn't render with debug yellow underlines.
    return Material(
      type: MaterialType.transparency,
      child: SafeArea(
        child: Stack(
          children: [
            Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                if (title != null || eyebrow != null) _header(context),
                Expanded(child: child),
              ],
            ),
            Positioned(
              top: 4,
              right: 8,
              child: IconAction(
                icon: Icons.close_rounded,
                semanticsLabel: context.l10n.a11yClose,
                color: WorkshopOverviewPalette.onPhoto,
                onPressed: () => Navigator.of(context).pop(),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _header(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(22, 8, 56, 10),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          if (eyebrow != null)
            Padding(
              padding: const EdgeInsets.only(bottom: 4),
              child: Text(
                eyebrow!,
                style: const TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w700,
                  letterSpacing: 1.3,
                  color: WorkshopOverviewPalette.accentSoft,
                ),
              ),
            ),
          if (title != null)
            Text(
              title!,
              style: const TextStyle(
                fontFamily: AppTheme.headingFont,
                fontWeight: FontWeight.w600,
                fontSize: 22,
                color: WorkshopOverviewPalette.onPhoto,
              ),
            ),
        ],
      ),
    );
  }
}
