import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/solid_sheet.dart';

/// The profile overflow menu's "Edit name" sheet: one prefilled field
/// on the opaque [SolidSheet] surface, popping the trimmed new name (or
/// null when dismissed). Persistence stays with the caller — the user
/// and community profiles save through different repositories.
class ProfileRenameSheet extends StatefulWidget {
  const ProfileRenameSheet({super.key, required this.initialName});

  final String initialName;

  /// Shows the sheet; resolves with the new name, or null when
  /// dismissed or unchanged.
  static Future<String?> show(
    BuildContext context, {
    required String initialName,
  }) {
    return showAccessibleModal<String>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => ProfileRenameSheet(initialName: initialName),
    );
  }

  @override
  State<ProfileRenameSheet> createState() => _ProfileRenameSheetState();
}

class _ProfileRenameSheetState extends State<ProfileRenameSheet> {
  late final TextEditingController _controller =
      TextEditingController(text: widget.initialName);

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _save() {
    final name = _controller.text.trim();
    if (name.isEmpty) return;
    Navigator.of(context).pop(name == widget.initialName.trim() ? null : name);
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    // The outer padding keeps the sheet above the keyboard.
    return Padding(
      padding:
          EdgeInsets.only(bottom: MediaQuery.of(context).viewInsets.bottom),
      child: SolidSheet(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              l10n.profileMenuEditName,
              style: TextStyle(
                fontFamily: AppTheme.headingFont,
                fontSize: 20,
                fontWeight: FontWeight.w600,
                color: AppColors.textPrimary(context),
              ),
            ),
            const SizedBox(height: 12),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 4),
              decoration: BoxDecoration(
                color: AppColors.surface(context),
                borderRadius: BorderRadius.circular(14),
                border: Border.all(color: AppColors.border(context)),
              ),
              child: TextField(
                controller: _controller,
                autofocus: true,
                textCapitalization: TextCapitalization.words,
                textInputAction: TextInputAction.done,
                onChanged: (_) => setState(() {}),
                onSubmitted: (_) => _save(),
                style: TextStyle(
                  fontSize: 15,
                  color: AppColors.textPrimary(context),
                ),
                decoration: const InputDecoration(border: InputBorder.none),
              ),
            ),
            const SizedBox(height: 14),
            Tappable(
              semanticsLabel: l10n.commonSave,
              onTap: _controller.text.trim().isEmpty ? null : _save,
              inkBorderRadius: BorderRadius.circular(999),
              child: Container(
                height: 48,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  color: _controller.text.trim().isEmpty
                      ? AppColors.surface(context)
                      : AppColors.primary(context),
                  borderRadius: BorderRadius.circular(999),
                ),
                child: Text(
                  l10n.commonSave,
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w600,
                    color: _controller.text.trim().isEmpty
                        ? AppColors.textTertiary(context)
                        : AppColors.background(context),
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
