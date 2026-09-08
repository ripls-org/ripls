import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// GlassFooterButtons is the bottom action bar of a glass bottom-sheet:
/// a translucent secondary (cancel/discard) button on the left and a coral
/// primary (save/confirm) button on the right, separated from the sheet body
/// by a 1px [AppColors.modalFooterDivider] hairline.
///
/// When [secondaryLabel] is null the secondary button is omitted and the
/// primary button fills the row. Both buttons animate the press-scale via
/// `accessibleDuration`.
class GlassFooterButtons extends StatelessWidget {
  /// Primary action label. Defaults to `commonSave` when null.
  final String? primaryLabel;

  /// Whether the primary action is enabled.
  final bool primaryEnabled;

  /// Primary action callback.
  final VoidCallback? onPrimary;

  /// Optional secondary (cancel) label. Defaults to `commonCancel` when null
  /// AND [showSecondary] is true.
  final String? secondaryLabel;

  /// Whether to render the secondary button.
  final bool showSecondary;

  /// Whether the secondary action is enabled.
  final bool secondaryEnabled;

  /// Secondary action callback.
  final VoidCallback? onSecondary;

  /// Optional override for the primary semantics label. Defaults to
  /// [primaryLabel] (or `commonSave`).
  final String? primarySemanticsLabel;

  /// Optional override for the secondary semantics label.
  final String? secondarySemanticsLabel;

  /// Optional fill color for the primary button. Defaults to
  /// [AppColors.modalPrimaryButtonBackground]. Use a higher-contrast value
  /// on darker glass surfaces where the default deep sage reads as low
  /// contrast.
  final Color? primaryColor;

  const GlassFooterButtons({
    super.key,
    this.primaryLabel,
    required this.primaryEnabled,
    required this.onPrimary,
    this.secondaryLabel,
    this.showSecondary = true,
    this.secondaryEnabled = true,
    this.onSecondary,
    this.primarySemanticsLabel,
    this.secondarySemanticsLabel,
    this.primaryColor,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final primaryText = primaryLabel ?? l10n.commonSave;
    final secondaryText = secondaryLabel ?? l10n.commonCancel;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(
          height: 1,
          margin: const EdgeInsets.only(top: 12, bottom: 14),
          color: AppColors.modalFooterDivider,
        ),
        Row(
          children: [
            if (showSecondary) ...[
              Expanded(
                child: _SecondaryButton(
                  label: secondaryText,
                  semanticsLabel: secondarySemanticsLabel ?? secondaryText,
                  enabled: secondaryEnabled,
                  onTap: onSecondary,
                ),
              ),
              const SizedBox(width: 10),
            ],
            Expanded(
              child: _PrimaryButton(
                label: primaryText,
                semanticsLabel: primarySemanticsLabel ?? primaryText,
                enabled: primaryEnabled,
                onTap: onPrimary,
                color: primaryColor,
              ),
            ),
          ],
        ),
      ],
    );
  }
}

class _SecondaryButton extends StatefulWidget {
  final String label;
  final String semanticsLabel;
  final bool enabled;
  final VoidCallback? onTap;

  const _SecondaryButton({
    required this.label,
    required this.semanticsLabel,
    required this.enabled,
    required this.onTap,
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
    final disabled = !widget.enabled || widget.onTap == null;
    final body = AnimatedScale(
      duration: accessibleDuration(context, ModalTheme.pressDuration),
      scale: _pressed ? ModalTheme.pressScale : 1.0,
      child: Container(
        height: ModalTheme.buttonHeight,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: AppColors.modalSecondaryButtonBackground,
          borderRadius: BorderRadius.circular(ModalTheme.buttonHeight / 2),
          border: Border.all(color: AppColors.modalSecondaryButtonBorder),
        ),
        child: Opacity(
          opacity: disabled ? 0.5 : 1.0,
          child: Text(
            widget.label,
            style: ModalTheme.buttonTextStyle.copyWith(
              color: AppColors.modalSecondaryButtonText,
            ),
          ),
        ),
      ),
    );

    return Tappable(
      semanticsLabel: widget.semanticsLabel,
      onTap: disabled ? null : widget.onTap,
      child: Listener(
        onPointerDown: (_) => _setPressed(true),
        onPointerUp: (_) => _setPressed(false),
        onPointerCancel: (_) => _setPressed(false),
        child: body,
      ),
    );
  }
}

class _PrimaryButton extends StatefulWidget {
  final String label;
  final String semanticsLabel;
  final bool enabled;
  final VoidCallback? onTap;
  final Color? color;

  const _PrimaryButton({
    required this.label,
    required this.semanticsLabel,
    required this.enabled,
    required this.onTap,
    this.color,
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
    final disabled = !widget.enabled || widget.onTap == null;
    final body = AnimatedScale(
      duration: accessibleDuration(context, ModalTheme.pressDuration),
      scale: _pressed ? ModalTheme.pressScale : 1.0,
      child: Container(
        height: ModalTheme.buttonHeight,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: disabled
              ? (widget.color ?? AppColors.modalPrimaryButtonBackground)
                  .withValues(alpha: 0.6)
              : widget.color ?? AppColors.modalPrimaryButtonBackground,
          borderRadius: BorderRadius.circular(ModalTheme.buttonHeight / 2),
          boxShadow: disabled
              ? null
              : [
                  BoxShadow(
                    color: Colors.black.withValues(alpha: 0.18),
                    blurRadius: 8,
                    offset: const Offset(0, 2),
                  ),
                ],
        ),
        child: Text(
          widget.label,
          style: ModalTheme.buttonTextStyle.copyWith(
            color: AppColors.modalPrimaryButtonText,
          ),
        ),
      ),
    );

    return Tappable(
      semanticsLabel: widget.semanticsLabel,
      onTap: disabled ? null : widget.onTap,
      child: Listener(
        onPointerDown: (_) => _setPressed(true),
        onPointerUp: (_) => _setPressed(false),
        onPointerCancel: (_) => _setPressed(false),
        child: body,
      ),
    );
  }
}
