import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_sheet.dart';

/// Shared glass-sheet wrapper for every Needs & Contributions surface.
///
/// Wraps [GlassSheet] with the keyboard-aware padding pattern the needs
/// flows already used and re-exposes the standard top/horizontal/bottom
/// values so each new sheet doesn't redefine them. Pair with
/// [showAccessibleModal] — see the README and `docs/client/needs.md`.
class NeedsSheetChrome extends StatelessWidget {
  static const double horizontalPadding = 24;
  static const double topPadding = 4;
  static const double bottomPadding = 16;

  final Widget child;

  /// When true, the drag handle floats over content (used by the Picker
  /// modal so its content can bleed to the sheet top edge).
  final bool dragHandleOverlay;

  const NeedsSheetChrome({
    super.key,
    required this.child,
    this.dragHandleOverlay = false,
  });

  @override
  Widget build(BuildContext context) {
    final keyboardInset = MediaQuery.of(context).viewInsets.bottom;
    return GlassSheet(
      dragHandleOverlay: dragHandleOverlay,
      padding: EdgeInsets.fromLTRB(
        horizontalPadding,
        topPadding,
        horizontalPadding,
        bottomPadding + keyboardInset,
      ),
      child: child,
    );
  }
}

/// Small colored dot + sentence-case eyebrow label (e.g. "● Open · asked by
/// Thomas"). Identical contract to the helper in the legacy chrome — extracted
/// so new sheets don't pull from `widgets/experience/needs/_sheet_chrome.dart`.
class NeedsStatusBadge extends StatelessWidget {
  final Color dotColor;
  final String label;

  const NeedsStatusBadge({
    super.key,
    required this.dotColor,
    required this.label,
  });

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: label,
      container: true,
      child: ExcludeSemantics(
        child: Row(
          children: [
            Container(
              width: 7,
              height: 7,
              decoration:
                  BoxDecoration(color: dotColor, shape: BoxShape.circle),
            ),
            const SizedBox(width: 7),
            Expanded(
              child: Text(
                label,
                style: const TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w600,
                  color: AppColors.modalTextSecondary,
                  letterSpacing: 0.2,
                ),
                overflow: TextOverflow.ellipsis,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
