import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';

/// GlassFieldLabel renders the small UPPERCASE label that introduces a
/// section in a glass bottom-sheet (e.g. "DATE", "START TIME", "DURATION").
///
/// Pass an already-localized string from `context.l10n`. The widget itself
/// applies the `toUpperCase()` transform so callers do not need to commit to
/// uppercase strings in their ARB files.
class GlassFieldLabel extends StatelessWidget {
  final String text;

  /// Optional bottom padding override. Defaults to 8px.
  final double bottomPadding;

  const GlassFieldLabel({
    super.key,
    required this.text,
    this.bottomPadding = 8,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: EdgeInsets.fromLTRB(4, 0, 4, bottomPadding),
      child: Text(
        text.toUpperCase(),
        style: ModalTheme.fieldLabelStyle.copyWith(
          color: AppColors.modalTextTertiary,
        ),
      ),
    );
  }
}
