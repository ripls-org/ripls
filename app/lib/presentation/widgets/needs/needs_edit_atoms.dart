import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Atoms for the NK4 (need edit) confirm view in
/// [NeedsPickerModal]. Kept in their own file so the picker modal
/// stays under the dart-file-size gate. Library-private (`needs/`
/// only) — callers outside this folder should use [NeedsPickerModal].

/// Uppercase section label used above each field in the NK4 edit
/// confirm view (TITLE / QUANTITY NEEDED / COMMENT).
class NeedsEditFieldLabel extends StatelessWidget {
  const NeedsEditFieldLabel({super.key, required this.label});
  final String label;

  @override
  Widget build(BuildContext context) {
    return Text(
      label.toUpperCase(),
      style: const TextStyle(
        fontSize: 10.5,
        fontWeight: FontWeight.w700,
        letterSpacing: 1.4,
        color: AppColors.modalTextSecondary,
      ),
    );
  }
}

/// Serif title input on the NK4 edit confirm view. Allows the need's
/// display name to be renamed inline. Styled with [AppTheme.headingFont]
/// to match the editorial treatment in `claim-sheet-prototype.html`.
class NeedsEditTitleInput extends StatelessWidget {
  const NeedsEditTitleInput({
    super.key,
    required this.controller,
    required this.onChanged,
  });
  final TextEditingController controller;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
      decoration: BoxDecoration(
        color: GlassTokens.fillFaint,
        border: Border.all(color: AppColors.modalChipBorder),
        borderRadius: BorderRadius.circular(12),
      ),
      child: TextField(
        controller: controller,
        onChanged: onChanged,
        autofocus: true,
        textCapitalization: TextCapitalization.sentences,
        cursorColor: AppColors.experienceSageGreen,
        style: const TextStyle(
          fontFamily: AppTheme.headingFont,
          fontSize: 24,
          fontWeight: FontWeight.w500,
          letterSpacing: -0.2,
          height: 1.15,
          color: AppColors.modalTextPrimary,
        ),
        decoration: const InputDecoration(
          // Explicit transparent fill — the wrapping Container owns
          // the glass-tinted background, and Material's default
          // InputDecorationTheme can otherwise leak a white fill on
          // top of it once the field gains focus.
          filled: true,
          fillColor: Colors.transparent,
          hoverColor: Colors.transparent,
          border: InputBorder.none,
          enabledBorder: InputBorder.none,
          focusedBorder: InputBorder.none,
          disabledBorder: InputBorder.none,
          isCollapsed: true,
          contentPadding: EdgeInsets.zero,
        ),
      ),
    );
  }
}

/// Pill-shaped quantity stepper used by the NK4 edit confirm view.
/// Houses circular − / + buttons + the live value + a "person / item"
/// (singular) or "people / items" (plural) suffix.
///
/// When [readOnly] is true the stepper still renders the count and
/// suffix, but the − / + buttons are hidden and the surrounding pill
/// is tinted muted — used by the Claim sheet to surface the
/// proposer's slot count without giving the helper an editing
/// affordance they don't actually have.
class NeedsEditQuantityStepper extends StatelessWidget {
  const NeedsEditQuantityStepper({
    super.key,
    required this.value,
    required this.suffix,
    required this.decrementLabel,
    required this.incrementLabel,
    required this.onChanged,
    this.readOnly = false,
    this.max = 20,
  });
  final int value;
  final String suffix;
  final String decrementLabel;
  final String incrementLabel;
  final ValueChanged<int> onChanged;
  final bool readOnly;

  /// Upper bound for the + button. Default `20` matches the NK4 edit
  /// modal's safety cap on slot count. Pass `null` (or a very large
  /// value) on surfaces where the count is intentionally uncapped —
  /// e.g. the claim sheet, where a helper can pledge more than the
  /// proposer asked for.
  final int? max;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: EdgeInsets.fromLTRB(
        readOnly ? 14 : 6,
        readOnly ? 10 : 6,
        14,
        readOnly ? 10 : 6,
      ),
      decoration: BoxDecoration(
        color: Colors.white.withValues(
          alpha: readOnly ? 0.025 : 0.04,
        ),
        border: Border.all(color: AppColors.modalChipBorder),
        borderRadius: BorderRadius.circular(14),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (!readOnly)
            _StepperRoundButton(
              icon: Icons.remove,
              semanticsLabel: decrementLabel,
              enabled: value > 1,
              onTap: value > 1 ? () => onChanged(value - 1) : null,
            ),
          SizedBox(
            width: readOnly ? null : 36,
            child: Text(
              '$value',
              textAlign: TextAlign.center,
              style: const TextStyle(
                fontSize: 17,
                fontWeight: FontWeight.w700,
                color: AppColors.modalTextPrimary,
              ),
            ),
          ),
          if (!readOnly)
            _StepperRoundButton(
              icon: Icons.add,
              semanticsLabel: incrementLabel,
              enabled: max == null || value < max!,
              onTap: (max == null || value < max!)
                  ? () => onChanged(value + 1)
                  : null,
            ),
          const SizedBox(width: 12),
          Text(
            suffix,
            style: const TextStyle(
              fontSize: 12,
              color: AppColors.modalTextMuted,
            ),
          ),
        ],
      ),
    );
  }
}

class _StepperRoundButton extends StatelessWidget {
  const _StepperRoundButton({
    required this.icon,
    required this.semanticsLabel,
    required this.enabled,
    required this.onTap,
  });
  final IconData icon;
  final String semanticsLabel;
  final bool enabled;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final color = enabled
        ? AppColors.modalTextPrimary
        : AppColors.modalTextPrimary.withValues(alpha: 0.35);
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(999),
      child: Container(
        width: 32,
        height: 32,
        decoration: BoxDecoration(
          color: enabled ? GlassTokens.fillSubtle : GlassTokens.fillFaint,
          shape: BoxShape.circle,
        ),
        alignment: Alignment.center,
        child: Icon(icon, size: 16, color: color),
      ),
    );
  }
}

/// Multi-line comment textarea used by the NK4 edit confirm view.
/// Surfaces "Context for whoever picks this up." as placeholder copy.
class NeedsEditCommentField extends StatelessWidget {
  const NeedsEditCommentField({
    super.key,
    required this.controller,
    required this.placeholder,
  });
  final TextEditingController controller;
  final String placeholder;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 13, vertical: 11),
      decoration: BoxDecoration(
        color: GlassTokens.fillFaint,
        border: Border.all(color: AppColors.modalChipBorder),
        borderRadius: BorderRadius.circular(12),
      ),
      child: TextField(
        controller: controller,
        minLines: 3,
        maxLines: 5,
        textCapitalization: TextCapitalization.sentences,
        cursorColor: AppColors.experienceSageGreen,
        style: const TextStyle(
          color: AppColors.modalTextPrimary,
          fontSize: 13.5,
          height: 1.45,
        ),
        decoration: InputDecoration(
          hintText: placeholder,
          hintStyle: const TextStyle(
            color: AppColors.modalTextMuted,
            fontSize: 13.5,
          ),
          // Explicit transparent fill — see [NeedsEditTitleInput] for
          // why we override Material's default fill on focus.
          filled: true,
          fillColor: Colors.transparent,
          hoverColor: Colors.transparent,
          border: InputBorder.none,
          enabledBorder: InputBorder.none,
          focusedBorder: InputBorder.none,
          disabledBorder: InputBorder.none,
          isCollapsed: true,
          contentPadding: EdgeInsets.zero,
        ),
      ),
    );
  }
}

/// Sage "Save changes ✓" CTA used by the NK4 edit confirm view.
/// Mirrors the prototype's primary green button — leading check icon,
/// dimmed disabled state, deeper shadow when active.
class NeedsEditSaveChangesButton extends StatelessWidget {
  const NeedsEditSaveChangesButton({
    super.key,
    required this.label,
    required this.enabled,
    required this.onTap,
  });
  final String label;
  final bool enabled;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.experienceSageGreen;
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(14),
      child: Container(
        height: 50,
        decoration: BoxDecoration(
          color: enabled ? accent : accent.withValues(alpha: 0.30),
          borderRadius: BorderRadius.circular(14),
          boxShadow: enabled
              ? [
                  BoxShadow(
                    color: accent.withValues(alpha: 0.30),
                    offset: const Offset(0, 8),
                    blurRadius: 20,
                  ),
                ]
              : null,
        ),
        alignment: Alignment.center,
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.check,
              size: 16,
              color: enabled
                  ? const Color(0xFF11201A)
                  : AppColors.modalTextMuted,
            ),
            const SizedBox(width: 8),
            Text(
              label,
              style: TextStyle(
                fontSize: 15,
                fontWeight: FontWeight.w700,
                color: enabled
                    ? const Color(0xFF11201A)
                    : AppColors.modalTextMuted,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
