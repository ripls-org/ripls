import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Primary action button for the unified-create flow ("Draft it", "Save
/// Request", "Save Event", community creation).
///
///   * Disabled  → translucent (`modalChipBackground`) with muted text.
///   * Enabled   → the glass primary (`modalPrimaryButtonBackground`) with its
///                 paired foreground.
///
/// Enabled used to take `modalChipBackgroundActive` — solid white — because it
/// was matching the active mode-tab *pill*. A chip that says which tab you are
/// on and a button that performs the screen's action are not the same control,
/// and copying the chip's treatment left the primary CTA of the create flow
/// white while every other primary CTA in the app is sage.
class UnifiedPrimaryButton extends StatelessWidget {
  const UnifiedPrimaryButton({
    super.key,
    required this.label,
    required this.enabled,
    required this.onTap,
    this.loading = false,
    this.semanticsIdentifier,
  });

  final String label;
  final bool enabled;
  final VoidCallback onTap;

  /// When true, swap the label for a centred spinner and ignore taps —
  /// even if [enabled] is true. Used by the unified-create flow while
  /// the per-type Save+Share RPCs and feed refresh are in flight.
  final bool loading;

  /// Optional stable identifier exposed on the underlying Semantics node
  /// (`flt-semantics-identifier` on Flutter Web) for Playwright targeting —
  /// useful where the visible label collides with another control. See
  /// docs/client/testing/semantics_identifiers.md.
  final String? semanticsIdentifier;

  @override
  Widget build(BuildContext context) {
    final interactive = enabled && !loading;
    return Tappable(
      semanticsLabel: label,
      semanticsIdentifier: semanticsIdentifier,
      onTap: interactive ? onTap : null,
      inkBorderRadius: BorderRadius.circular(999),
      child: AnimatedContainer(
        duration: accessibleDuration(context, const Duration(milliseconds: 150)),
        height: 48,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          // Disabled stays the translucent chip fill rather than a dimmed
          // sage: it reads as inert, which is the point, and keeps the
          // enabled sage as the only green in the drawer.
          color: interactive
              ? AppColors.modalPrimaryButtonBackground
              : AppColors.modalChipBackground,
          borderRadius: BorderRadius.circular(999),
        ),
        child: loading
            ? SizedBox(
                width: 22,
                height: 22,
                child: CircularProgressIndicator(
                  strokeWidth: 2.2,
                  valueColor: AlwaysStoppedAnimation<Color>(
                    AppColors.modalPrimaryButtonText,
                  ),
                ),
              )
            : Text(
                label,
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w600,
                  color: interactive
                      ? AppColors.modalPrimaryButtonText
                      : GlassTokens.textFaint,
                ),
              ),
      ),
    );
  }
}
