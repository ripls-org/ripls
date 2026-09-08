import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';

/// LanguagePicker displays the available app languages for selection.
///
/// Mirrors the [TimezonePicker] pattern: a full screen pushed from the right
/// with a list of options and a check mark on the current selection.
class LanguagePicker extends StatefulWidget {
  /// Currently selected locale language code, or null for system default.
  final String? currentLanguageCode;

  /// Callback when a language is selected. Passes null for system default.
  final ValueChanged<String?> onLanguageSelected;

  const LanguagePicker({
    super.key,
    this.currentLanguageCode,
    required this.onLanguageSelected,
  });

  @override
  State<LanguagePicker> createState() => _LanguagePickerState();
}

class _LanguagePickerState extends State<LanguagePicker>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  @override
  Widget build(BuildContext context) {
    final options = <(String?, String)>[
      (null, context.l10n.settingsLanguageSystemDefault),
      ('en', context.l10n.settingsLanguageEnglish),
      ('es', context.l10n.settingsLanguageSpanish),
    ];

    return buildSwipeableScaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        leading: AppBarBackButton(onPressed: handleClose),
        title: Text(
          context.l10n.settingsLanguage,
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
      ),
      body: ListView.builder(
        itemCount: options.length,
        itemBuilder: (context, index) {
          final (code, label) = options[index];
          final isSelected = code == widget.currentLanguageCode;

          return ListTile(
            title: Text(
              label,
              style: TextStyle(color: AppColors.textPrimary(context)),
            ),
            trailing: isSelected
                ? Icon(Icons.check, color: AppColors.primary(context))
                : null,
            selected: isSelected,
            onTap: () {
              widget.onLanguageSelected(code);
              Navigator.pop(context);
            },
          );
        },
      ),
    );
  }
}
