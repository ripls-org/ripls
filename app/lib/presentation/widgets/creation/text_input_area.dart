import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// Multi-line text input area for creation prompts.
///
/// Simpler than ContentEditableField - designed specifically for
/// AI generation prompt input in creation modals.
///
/// Example:
/// ```dart
/// TextInputArea(
///   controller: _textController,
///   hintText: 'e.g., Potluck dinner this weekend, hiking trip...',
///   onChanged: (value) => notifier.setTextInput(value),
/// )
/// ```
class TextInputArea extends StatelessWidget {
  final TextEditingController controller;
  final String hintText;
  final int minLines;
  final int maxLines;
  final FormFieldValidator<String>? validator;
  final Function(String)? onChanged;
  final FocusNode? focusNode;

  /// When `true`, the field renders without an opaque fill — relying on
  /// the surrounding frosted-glass sheet to provide the surface. Used by
  /// the glass-migrated community-creation modal; legacy creation modals
  /// leave it `false` so the field keeps its opaque card background.
  final bool onGlass;

  const TextInputArea({
    super.key,
    required this.controller,
    required this.hintText,
    this.minLines = 4,
    this.maxLines = 4,
    this.validator,
    this.onChanged,
    this.focusNode,
    this.onGlass = false,
  });

  @override
  Widget build(BuildContext context) {
    if (onGlass) {
      // Glass-modal styling: hairline border, transparent fill, modal-
      // theme text colors, no outer Padding wrapper — the surrounding
      // glass sheet supplies its own inset. Matches `_BorderlessGlassField`
      // in the unified-create input drawer.
      return Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(20),
          border: Border.all(color: AppColors.modalSearchFieldBorder),
        ),
        child: TextField(
            controller: controller,
            focusNode: focusNode,
            maxLines: maxLines,
            minLines: minLines,
            textAlignVertical: TextAlignVertical.top,
            keyboardType: TextInputType.multiline,
            textInputAction: TextInputAction.done,
            textCapitalization: TextCapitalization.sentences,
            cursorColor: AppColors.modalTextPrimary,
            style: TextStyle(
              color: AppColors.modalTextPrimary,
              fontSize: 16,
            ),
            decoration: InputDecoration(
              isCollapsed: true,
              filled: false,
              fillColor: Colors.transparent,
              border: InputBorder.none,
              enabledBorder: InputBorder.none,
              focusedBorder: InputBorder.none,
              hintText: hintText,
              hintStyle: TextStyle(
                color: AppColors.modalTextMuted.withValues(alpha: 0.55),
                fontSize: 16,
              ),
            ),
            onChanged: onChanged,
          ),
      );
    }
    return Padding(
      padding: const EdgeInsets.all(24),
      child: TextField(
        controller: controller,
        focusNode: focusNode,
        maxLines: maxLines,
        minLines: minLines,
        textAlignVertical: TextAlignVertical.top,
        keyboardType: TextInputType.multiline,
        textInputAction: TextInputAction.done,
        textCapitalization: TextCapitalization.sentences,
        style: TextStyle(
          color: AppColors.textPrimary(context),
          fontSize: 16,
        ),
        decoration: InputDecoration(
          hintText: hintText,
          hintStyle: TextStyle(
            color: AppColors.textTertiary(context),
            fontSize: 16,
          ),
          filled: true,
          fillColor: AppColors.cardBackground(context),
          border: OutlineInputBorder(
            borderRadius: BorderRadius.circular(12),
            borderSide: BorderSide(color: AppColors.border(context)),
          ),
          enabledBorder: OutlineInputBorder(
            borderRadius: BorderRadius.circular(12),
            borderSide: BorderSide(color: AppColors.border(context)),
          ),
          focusedBorder: OutlineInputBorder(
            borderRadius: BorderRadius.circular(12),
            borderSide: BorderSide(
              color: AppColors.primary(context),
              width: 2,
            ),
          ),
          contentPadding: const EdgeInsets.all(16),
        ),
        onChanged: onChanged,
      ),
    );
  }
}
