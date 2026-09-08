import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// SolidSheet is the opaque bottom-sheet surface for the Me-sheet grammar
/// (#2634 v2): full-width, large top radius, grabber, solid theme-aware
/// fill. Deliberately **not** glass — no blur, no translucency; these
/// sheets are moving off the glass language.
///
/// Content uses the regular theme tokens (`AppColors.textPrimary` etc.),
/// not the on-glass `modal*` whites.
class SolidSheet extends StatelessWidget {
  final Widget child;

  /// Extra padding below the content (defaults leave room for the home
  /// indicator).
  final EdgeInsetsGeometry padding;

  const SolidSheet({
    super.key,
    required this.child,
    this.padding = const EdgeInsets.fromLTRB(20, 0, 20, 30),
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      constraints: BoxConstraints(
        maxHeight: MediaQuery.of(context).size.height * 0.85,
      ),
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context),
        borderRadius: const BorderRadius.vertical(top: Radius.circular(28)),
        border: Border(
          top: BorderSide(color: AppColors.border(context)),
        ),
      ),
      child: SafeArea(
        top: false,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Padding(
              padding: const EdgeInsets.only(top: 12, bottom: 12),
              child: Container(
                width: 38,
                height: 4.5,
                decoration: BoxDecoration(
                  color: AppColors.border(context),
                  borderRadius: BorderRadius.circular(3),
                ),
              ),
            ),
            // Content scrolls when it exceeds the sheet's max height
            // (small screens, keyboard up) rather than overflowing.
            Flexible(
              child: SingleChildScrollView(
                child: Padding(padding: padding, child: child),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
