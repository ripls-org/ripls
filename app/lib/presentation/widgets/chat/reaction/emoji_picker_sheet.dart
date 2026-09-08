import 'package:emoji_picker_flutter/emoji_picker_flutter.dart';
import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

/// A bottom-sheet wrapper around [EmojiPicker] that calls [onEmojiSelected]
/// with the chosen emoji string.
class EmojiPickerSheet extends StatelessWidget {
  final ValueChanged<String> onEmojiSelected;

  const EmojiPickerSheet({
    super.key,
    required this.onEmojiSelected,
  });

  @override
  Widget build(BuildContext context) {
    const bg = AppColors.modalInsetCardBg;
    const accent = AppColors.modalPrimaryButtonBackground;

    return GlassSheet(
      applyMaxHeight: false,
      padding: EdgeInsets.zero,
      child: SizedBox(
        height: 360,
        child: EmojiPicker(
          onEmojiSelected: (category, emoji) {
            onEmojiSelected(emoji.emoji);
          },
          config: Config(
            height: 360,
            checkPlatformCompatibility: true,
            categoryViewConfig: const CategoryViewConfig(
              backgroundColor: bg,
              indicatorColor: accent,
              iconColor: AppColors.modalTextMuted,
              iconColorSelected: accent,
              backspaceColor: accent,
              dividerColor: AppColors.modalFooterDivider,
            ),
            emojiViewConfig: const EmojiViewConfig(
              backgroundColor: bg,
              noRecents: Text(
                'No recent emoji',
                style: TextStyle(
                  fontSize: 14,
                  color: AppColors.modalTextMuted,
                ),
              ),
            ),
            bottomActionBarConfig: const BottomActionBarConfig(
              backgroundColor: bg,
              buttonColor: accent,
              buttonIconColor: AppColors.modalPrimaryButtonText,
            ),
            searchViewConfig: const SearchViewConfig(
              backgroundColor: bg,
              buttonIconColor: accent,
              hintText: 'Search emoji',
            ),
          ),
        ),
      ),
    );
  }
}
