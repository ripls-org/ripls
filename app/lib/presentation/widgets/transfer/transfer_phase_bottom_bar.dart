import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Fixed action bar at the bottom of a transfer modal. Glass-tokened: a
/// hairline divider above the buttons (matching `GlassFooterButtons`),
/// coral primary, glass-secondary outlined button, optional leading
/// widget (e.g. chat button) — the canonical `GlassFooterButtons` does
/// not support a leading slot, hence this dedicated variant.
///
/// Uses `transferCoral` and `transferSage` as semantic accent fills for
/// the primary action only — destructive actions render in coral, the
/// default action renders in sage. Both colors survive the glass
/// migration as semantic accents (per `docs/issues/1802-modals-glass-sweep.md`
/// §Resolved Decisions).
class TransferPhaseBottomBar extends StatelessWidget {
  /// Optional leading widget (e.g., chat button)
  final Widget? leadingWidget;

  /// Label for the primary action button
  final String primaryLabel;

  /// Callback for the primary action
  final VoidCallback? onPrimaryPressed;

  /// Whether the primary action is destructive (uses coral color)
  final bool isPrimaryDestructive;

  /// Optional label for the secondary action button
  final String? secondaryLabel;

  /// Optional callback for the secondary action
  final VoidCallback? onSecondaryPressed;

  /// Whether the secondary action is destructive (uses coral color)
  final bool isSecondaryDestructive;

  /// Whether to show a loading indicator
  final bool isLoading;

  const TransferPhaseBottomBar({
    super.key,
    this.leadingWidget,
    required this.primaryLabel,
    this.onPrimaryPressed,
    this.isPrimaryDestructive = false,
    this.secondaryLabel,
    this.onSecondaryPressed,
    this.isSecondaryDestructive = false,
    this.isLoading = false,
  });

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Container(
          height: 1,
          margin: const EdgeInsets.only(top: 12, bottom: 14),
          color: AppColors.modalFooterDivider,
        ),
        SafeArea(
          top: false,
          child: Row(
            children: [
              if (leadingWidget != null) ...[
                leadingWidget!,
                const SizedBox(width: 12),
              ],
              if (secondaryLabel != null && onSecondaryPressed != null) ...[
                _SecondaryButton(
                  label: secondaryLabel!,
                  onPressed: isLoading ? null : onSecondaryPressed,
                  isDestructive: isSecondaryDestructive,
                ),
                const SizedBox(width: 12),
              ],
              Expanded(
                child: _PrimaryButton(
                  label: primaryLabel,
                  onPressed: isLoading ? null : onPrimaryPressed,
                  isDestructive: isPrimaryDestructive,
                  isLoading: isLoading,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

class _PrimaryButton extends StatefulWidget {
  final String label;
  final VoidCallback? onPressed;
  final bool isDestructive;
  final bool isLoading;

  const _PrimaryButton({
    required this.label,
    required this.onPressed,
    required this.isDestructive,
    required this.isLoading,
  });

  @override
  State<_PrimaryButton> createState() => _PrimaryButtonState();
}

class _PrimaryButtonState extends State<_PrimaryButton> {
  bool _pressed = false;

  void _setPressed(bool value) {
    if (!mounted || _pressed == value) return;
    setState(() => _pressed = value);
  }

  @override
  Widget build(BuildContext context) {
    final disabled = widget.onPressed == null;
    final fill = widget.isDestructive
        ? AppColors.transferCoral
        : AppColors.transferSage;
    final body = AnimatedScale(
      duration: accessibleDuration(context, ModalTheme.pressDuration),
      scale: _pressed ? ModalTheme.pressScale : 1.0,
      child: Container(
        height: ModalTheme.buttonHeight,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: disabled ? fill.withValues(alpha: 0.4) : fill,
          borderRadius: BorderRadius.circular(ModalTheme.buttonHeight / 2),
        ),
        child: widget.isLoading
            ? const SizedBox(
                width: 20,
                height: 20,
                child: CircularProgressIndicator(
                  strokeWidth: 2,
                  valueColor:
                      AlwaysStoppedAnimation<Color>(GlassTokens.onPrimary),
                ),
              )
            : Text(
                widget.label,
                style: ModalTheme.buttonTextStyle.copyWith(
                  color: widget.isDestructive
                      ? AppColors.darkBackground
                      : AppColors.modalPrimaryButtonText,
                  fontWeight: FontWeight.w700,
                ),
              ),
      ),
    );
    return Tappable(
      semanticsLabel: widget.label,
      onTap: disabled ? null : widget.onPressed,
      child: Listener(
        onPointerDown: (_) => _setPressed(true),
        onPointerUp: (_) => _setPressed(false),
        onPointerCancel: (_) => _setPressed(false),
        child: body,
      ),
    );
  }
}

class _SecondaryButton extends StatefulWidget {
  final String label;
  final VoidCallback? onPressed;
  final bool isDestructive;

  const _SecondaryButton({
    required this.label,
    required this.onPressed,
    required this.isDestructive,
  });

  @override
  State<_SecondaryButton> createState() => _SecondaryButtonState();
}

class _SecondaryButtonState extends State<_SecondaryButton> {
  bool _pressed = false;

  void _setPressed(bool value) {
    if (!mounted || _pressed == value) return;
    setState(() => _pressed = value);
  }

  @override
  Widget build(BuildContext context) {
    final disabled = widget.onPressed == null;
    final accent = widget.isDestructive
        ? AppColors.transferCoral
        : AppColors.modalSecondaryButtonText;
    final body = AnimatedScale(
      duration: accessibleDuration(context, ModalTheme.pressDuration),
      scale: _pressed ? ModalTheme.pressScale : 1.0,
      child: Container(
        height: ModalTheme.buttonHeight,
        padding: const EdgeInsets.symmetric(horizontal: 20),
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: AppColors.modalSecondaryButtonBackground,
          borderRadius: BorderRadius.circular(ModalTheme.buttonHeight / 2),
          border: Border.all(
            color: widget.isDestructive
                ? AppColors.transferCoral.withValues(alpha: 0.7)
                : AppColors.modalSecondaryButtonBorder,
          ),
        ),
        child: Opacity(
          opacity: disabled ? 0.5 : 1.0,
          child: Text(
            widget.label,
            style: ModalTheme.buttonTextStyle.copyWith(
              color: accent,
              fontWeight: FontWeight.w600,
              fontSize: 13,
            ),
          ),
        ),
      ),
    );
    return Tappable(
      semanticsLabel: widget.label,
      onTap: disabled ? null : widget.onPressed,
      child: Listener(
        onPointerDown: (_) => _setPressed(true),
        onPointerUp: (_) => _setPressed(false),
        onPointerCancel: (_) => _setPressed(false),
        child: body,
      ),
    );
  }
}
