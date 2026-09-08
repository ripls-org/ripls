import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';

/// GlassSearchInput renders a translucent text field that reads correctly on
/// a glass sheet — opaque-white fills pop out as a flat block, so this widget
/// uses the canonical `modalChipBackground`/`modalChipBorder` tokens instead.
class GlassSearchInput extends StatelessWidget {
  final TextEditingController controller;
  final FocusNode focusNode;
  final String hintText;
  final ValueChanged<String> onChanged;
  final ValueChanged<String> onSubmitted;

  /// Optional leading icon, defaults to [Icons.search].
  final IconData? leadingIcon;

  const GlassSearchInput({
    super.key,
    required this.controller,
    required this.focusNode,
    required this.hintText,
    required this.onChanged,
    required this.onSubmitted,
    this.leadingIcon = Icons.search,
  });

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      focusNode: focusNode,
      onChanged: onChanged,
      onSubmitted: (text) {
        onSubmitted(text);
        focusNode.unfocus();
      },
      textInputAction: TextInputAction.search,
      cursorColor: AppColors.modalTextPrimary,
      style: ModalTheme.inlineActionTextStyle.copyWith(
        color: AppColors.modalTextPrimary,
      ),
      decoration: InputDecoration(
        hintText: hintText,
        hintStyle: ModalTheme.inlineActionTextStyle.copyWith(
          color: AppColors.modalTextMuted,
        ),
        prefixIcon: leadingIcon == null
            ? null
            : Icon(
                leadingIcon,
                size: 18,
                color: AppColors.modalTextMuted,
              ),
        prefixIconConstraints: const BoxConstraints(
          minWidth: 40,
          minHeight: ModalTheme.inlineActionHeight,
        ),
        filled: true,
        fillColor: AppColors.modalSearchFieldBackground,
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 16,
          vertical: 12,
        ),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(ModalTheme.inlineActionRadius),
          borderSide: BorderSide(color: AppColors.modalSearchFieldBorder),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(ModalTheme.inlineActionRadius),
          borderSide: BorderSide(color: AppColors.modalSearchFieldBorder),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(ModalTheme.inlineActionRadius),
          borderSide: BorderSide(
            color: AppColors.modalBorder,
            width: 1.5,
          ),
        ),
      ),
    );
  }
}
