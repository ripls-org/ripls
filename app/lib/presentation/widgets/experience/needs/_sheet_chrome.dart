// Shared visual design tokens and stateless builder helpers used by every
// needs/contributions sheet in the experience feature.
//
// Glass-migrated: every sheet wraps in GlassSheet (which provides the
// drag handle), drops cream tokens for AppColors.modal*, and keeps
// transferCoral / experienceSageGreen as semantic accents on the
// primary button + status dot.

import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

const double kSheetHPadding = 24;
const double kSheetTopPadding = 4;
const double kSheetBottomPadding = 16;

/// Composes the GlassSheet padding for a needs sheet. The keyboard
/// inset stacks on top of the base bottom padding so the breathing
/// room above the home indicator is preserved as breathing room above
/// the keyboard when one is up. Pure function for unit testing.
EdgeInsets resolveBatchSheetPadding({
  required double horizontal,
  required double top,
  required double base,
  required double keyboardInset,
}) {
  return EdgeInsets.fromLTRB(horizontal, top, horizontal, base + keyboardInset);
}

/// Drag-handle slot for legacy callers. GlassSheet renders its own drag
/// handle, so this is now a no-op spacer kept for source compatibility
/// across the six needs sheets that still call it.
Widget buildDragHandle(BuildContext context) => const SizedBox.shrink();

/// Wraps a needs sheet's content in the standard glass bottom-sheet
/// surface. GlassSheet handles the drag handle, top-rounded corners,
/// horizontal inset, and max-height ceiling.
///
/// `isScrollControlled: true` on `showAccessibleModal` is required but
/// not sufficient for keyboard handling — content widgets must apply
/// `MediaQuery.viewInsets.bottom` themselves. Fixes #1992.
Widget buildSheetContainer({
  required BuildContext context,
  required Widget child,
}) {
  final keyboardInset = MediaQuery.of(context).viewInsets.bottom;
  return GlassSheet(
    padding: resolveBatchSheetPadding(
      horizontal: kSheetHPadding,
      top: kSheetTopPadding,
      base: kSheetBottomPadding,
      keyboardInset: keyboardInset,
    ),
    child: child,
  );
}

/// Builds a small colored dot + sentence-case label, e.g. "● Open · asked by Thomas".
Widget buildStatusBadge({
  required BuildContext context,
  required Color dotColor,
  required String label,
}) {
  return Row(
    children: [
      Container(
        width: 7,
        height: 7,
        decoration: BoxDecoration(color: dotColor, shape: BoxShape.circle),
      ),
      const SizedBox(width: 7),
      Expanded(
        child: Text(
          label,
          style: TextStyle(
            fontSize: 11,
            fontWeight: FontWeight.w600,
            color: AppColors.modalTextSecondary,
            letterSpacing: 0.2,
          ),
          overflow: TextOverflow.ellipsis,
        ),
      ),
    ],
  );
}

/// Builds a single-line modal title — solid white, normal weight,
/// fits on one line with ellipsis on overflow.
///
/// The historical two-tone "Bring \n extra layers?" treatment is gone:
/// [accent], [accentColor], and [compact] are kept for call-site
/// compatibility but the parameters no longer influence color or
/// layout. The title always renders as `"$primary $accent"` in white.
Widget buildSerifTitle(
  BuildContext context, {
  required String primary,
  String? accent,
  Color? accentColor,
  bool compact = false,
}) {
  final text = accent == null ? primary : '$primary $accent';
  return Text(
    text,
    maxLines: 1,
    overflow: TextOverflow.ellipsis,
    style: const TextStyle(
      fontSize: 22,
      fontWeight: FontWeight.w600,
      color: AppColors.modalTextPrimary,
      height: 1.2,
      letterSpacing: -0.2,
    ),
  );
}

/// Builds a thin rule + uppercase label (default "NOTE") with body text + thin rule.
Widget buildNoteSection(
  BuildContext context,
  String body, {
  String label = 'NOTE',
}) {
  return Column(
    mainAxisSize: MainAxisSize.min,
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Container(height: 1, color: AppColors.modalFooterDivider),
      Padding(
        padding: const EdgeInsets.symmetric(vertical: 10),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            SizedBox(
              width: 64,
              child: Text(
                label,
                style: TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w700,
                  color: AppColors.modalTextMuted,
                  letterSpacing: 0.8,
                ),
              ),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                body,
                style: TextStyle(
                  fontSize: 13,
                  color: AppColors.modalTextSecondary,
                  height: 1.5,
                ),
              ),
            ),
          ],
        ),
      ),
      Container(height: 1, color: AppColors.modalFooterDivider),
    ],
  );
}

/// Single primary submit button using the canonical [GlassFooterButtons]
/// shape (pill-radius coral, hairline divider above). Pass [onPressed]
/// `null` to render disabled.
Widget buildPrimaryButton({
  required BuildContext context,
  required String label,
  required VoidCallback? onPressed,
}) {
  return GlassFooterButtons(
    showSecondary: false,
    primaryEnabled: onPressed != null,
    primaryLabel: label,
    onPrimary: onPressed,
  );
}

/// Builds a centered subtle text link.
///
/// Used for "Maybe later", "Release back to the group", "Remove this
/// contribution".
Widget buildSecondaryLink({
  required BuildContext context,
  required String label,
  required VoidCallback onPressed,
  Color? color,
}) {
  return Center(
    child: TextButton(
      onPressed: onPressed,
      style: TextButton.styleFrom(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      ),
      child: Text(
        label,
        style: TextStyle(
          fontSize: 12.5,
          fontWeight: FontWeight.w500,
          color: color ?? AppColors.modalTextMuted,
        ),
      ),
    ),
  );
}

// ── Batch-sheet shared helpers ───────────────────────────────────────────────

/// Row data for the pending-items list shown in batch sheets.
class PendingRowData {
  const PendingRowData({required this.title, this.subtitle});
  final String title;
  final String? subtitle;
}

/// Builds the batch-sheet top row: status badge + serif title + X close button.
Widget buildBatchHeader({
  required BuildContext context,
  required Color accent,
  required String statusLabel,
  required String title,
  required String accentText,
  required VoidCallback onClose,
}) {
  return Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    mainAxisSize: MainAxisSize.min,
    children: [
      Row(
        children: [
          Expanded(
            child: buildStatusBadge(
              context: context,
              dotColor: accent,
              label: statusLabel,
            ),
          ),
          Tappable(
            semanticsLabel: context.l10n.a11yClose,
            onTap: onClose,
            child: Icon(Icons.close,
                color: AppColors.modalTextSecondary, size: 22),
          ),
        ],
      ),
      const SizedBox(height: 12),
      buildSerifTitle(
        context,
        primary: title,
        accent: accentText,
        accentColor: accent,
        compact: true,
      ),
    ],
  );
}

/// Builds the pending-items list shown inside batch sheets.
Widget buildPendingList({
  required BuildContext context,
  required Color accent,
  required List<PendingRowData> items,
  required void Function(int) onTap,
  required void Function(int) onRemove,
}) {
  return Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    mainAxisSize: MainAxisSize.min,
    children: [
      Row(
        children: [
          Icon(Icons.check, size: 13, color: accent),
          const SizedBox(width: 4),
          Text(
            'YOUR LIST',
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w700,
              color: accent,
              letterSpacing: 0.8,
            ),
          ),
        ],
      ),
      const SizedBox(height: 8),
      ...List.generate(items.length, (i) {
        final row = items[i];
        return Tappable(
          semanticsLabel: row.title,
          onTap: () => onTap(i),
          child: Container(
            margin: const EdgeInsets.only(bottom: 6),
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 14),
            decoration: BoxDecoration(
              color: accent.withValues(alpha: 0.18),
              borderRadius: BorderRadius.circular(10),
            ),
            child: Row(
              children: [
                Container(
                  width: 22,
                  height: 22,
                  margin: const EdgeInsets.only(right: 10),
                  decoration: BoxDecoration(
                    color: accent,
                    shape: BoxShape.circle,
                  ),
                  child: const Icon(
                    Icons.check,
                    size: 13,
                    color: GlassTokens.onPrimary,
                  ),
                ),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text(
                        row.title,
                        style: TextStyle(
                          fontSize: 14,
                          fontWeight: FontWeight.w600,
                          color: AppColors.modalTextPrimary,
                        ),
                      ),
                      if (row.subtitle != null) ...[
                        const SizedBox(height: 2),
                        Text(
                          row.subtitle!,
                          style: TextStyle(
                            fontSize: 12,
                            color: AppColors.modalTextSecondary,
                          ),
                        ),
                      ],
                    ],
                  ),
                ),
                Tappable(
                  semanticsLabel: context.l10n.a11yDismiss,
                  onTap: () => onRemove(i),
                  child: Icon(Icons.close,
                      size: 16, color: AppColors.modalTextMuted),
                ),
              ],
            ),
          ),
        );
      }),
    ],
  );
}

/// Builds the primary text input field used in batch and add sheets.
///
/// Solid cream pill with dark text and a muted placeholder — matches the
/// reference design for the request/offer modal "What's needed?" input.
/// When [accent] is non-null and the field has text, a thin tinted ring
/// is drawn around the pill to signal the active draft.
Widget buildInputField({
  required BuildContext context,
  required TextEditingController controller,
  required Color accent,
  required String hint,
  required VoidCallback onChanged,
  VoidCallback? onSubmitted,
}) {
  final hasText = controller.text.trim().isNotEmpty;
  return Container(
    height: 56,
    decoration: BoxDecoration(
      color: AppColors.modalSheetInputBackground,
      borderRadius: BorderRadius.circular(16),
      border: Border.all(
        color: hasText ? accent : Colors.transparent,
        width: 2,
      ),
    ),
    padding: const EdgeInsets.symmetric(horizontal: 20),
    child: Center(
      child: TextField(
        controller: controller,
        autofocus: true,
        cursorColor: AppColors.modalSheetInputText,
        textAlignVertical: TextAlignVertical.center,
        style: const TextStyle(
          color: AppColors.modalSheetInputText,
          fontWeight: FontWeight.w500,
          fontSize: 15,
          height: 1.2,
        ),
        decoration: InputDecoration(
          hintText: hint,
          hintStyle: const TextStyle(
            color: AppColors.modalSheetInputPlaceholder,
            fontWeight: FontWeight.w500,
            fontSize: 15,
            height: 1.2,
          ),
          // The cream pill is painted by the outer Container; opt out of the
          // global inputDecorationTheme's filled surface so the dark-theme
          // fillColor doesn't paint a dark box over the cream pill (#2004).
          filled: false,
          border: InputBorder.none,
          enabledBorder: InputBorder.none,
          focusedBorder: InputBorder.none,
          disabledBorder: InputBorder.none,
          errorBorder: InputBorder.none,
          focusedErrorBorder: InputBorder.none,
          isCollapsed: true,
          contentPadding: EdgeInsets.zero,
        ),
        onChanged: (_) => onChanged(),
        onSubmitted: onSubmitted == null ? null : (_) => onSubmitted(),
      ),
    ),
  );
}

/// Builds the secondary note/description input field used in batch sheets.
///
/// Matches the primary [buildInputField] cream-pill treatment so the two
/// inputs visually belong to the same family. Slightly shorter and
/// without the active-state accent ring, since the note is optional.
Widget buildNoteInputField({
  required BuildContext context,
  required TextEditingController controller,
  required String hint,
}) {
  return Container(
    height: 48,
    decoration: BoxDecoration(
      color: AppColors.modalSheetInputBackground,
      borderRadius: BorderRadius.circular(14),
    ),
    padding: const EdgeInsets.symmetric(horizontal: 18),
    child: Center(
      child: TextField(
        controller: controller,
        maxLines: 1,
        cursorColor: AppColors.modalSheetInputText,
        textAlignVertical: TextAlignVertical.center,
        style: const TextStyle(
          color: AppColors.modalSheetInputText,
          fontSize: 14,
          height: 1.2,
        ),
        decoration: InputDecoration(
          hintText: hint,
          hintStyle: const TextStyle(
            color: AppColors.modalSheetInputPlaceholder,
            fontSize: 14,
            height: 1.2,
          ),
          // See buildInputField above — opt out of the global filled surface
          // so dark mode doesn't paint over the cream pill (#2004).
          filled: false,
          border: InputBorder.none,
          enabledBorder: InputBorder.none,
          focusedBorder: InputBorder.none,
          disabledBorder: InputBorder.none,
          errorBorder: InputBorder.none,
          focusedErrorBorder: InputBorder.none,
          isCollapsed: true,
          contentPadding: EdgeInsets.zero,
        ),
      ),
    ),
  );
}

/// Two-button action row at the bottom of batch sheets, rendered with
/// the canonical [GlassFooterButtons] shape (pill secondary + coral
/// primary, hairline divider above). The accent param is no longer
/// needed — every modal's primary action uses the unified coral.
Widget buildBatchActionRow({
  required BuildContext context,
  required String addLabel,
  required String saveLabel,
  required bool canAdd,
  required bool canSave,
  required VoidCallback onAdd,
  required VoidCallback onSave,
}) {
  return GlassFooterButtons(
    secondaryLabel: addLabel,
    secondaryEnabled: canAdd,
    onSecondary: canAdd ? onAdd : null,
    primaryLabel: saveLabel,
    primaryEnabled: canSave,
    onPrimary: canSave ? onSave : null,
  );
}

/// Builds the discard-confirmation overlay shown when the user tries to close
/// a batch sheet that has unsaved entries.
Widget buildDiscardOverlay({
  required BuildContext context,
  required int count,
  required VoidCallback onKeep,
  required VoidCallback onDiscard,
}) {
  return Positioned.fill(
    child: GlassSurface(
      borderRadius: BorderRadius.zero,
      padding: EdgeInsets.fromLTRB(
        kSheetHPadding,
        40,
        kSheetHPadding,
        MediaQuery.of(context).padding.bottom + 24,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          buildSerifTitle(
            context,
            primary: context.l10n.experienceBatchDiscardTitle(count),
            compact: true,
          ),
          const SizedBox(height: 12),
          Text(
            context.l10n.experienceBatchDiscardBody,
            style: TextStyle(
              fontSize: 14,
              color: AppColors.modalTextSecondary,
              height: 1.5,
            ),
          ),
          const SizedBox(height: 24),
          buildPrimaryButton(
            context: context,
            label: context.l10n.experienceBatchKeepEditing,
            onPressed: onKeep,
          ),
          buildSecondaryLink(
            context: context,
            label: context.l10n.experienceBatchDiscard,
            onPressed: onDiscard,
            color: AppColors.transferCoral,
          ),
        ],
      ),
    ),
  );
}
