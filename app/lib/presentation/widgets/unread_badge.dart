import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// UnreadBadge displays an unread count indicator with consistent styling.
///
/// Used across the app for:
/// - Community avatar badge (top-left corner)
/// - Sidebar community list items
/// - Inbox conversation items
/// - Bottom navigation messages tab
///
/// Default design:
/// - Amber color scheme (amber-50 background, amber-900 text, amber-200 border)
/// - Displays "99+" for counts > 99
/// - Hidden when count is 0
/// - Rounded pill shape with subtle shadow
///
/// Pass [backgroundColor] and [textColor] to override the default amber scheme.
/// When [backgroundColor] is provided, the border is omitted for a solid look.
class UnreadBadge extends StatelessWidget {
  final int count;
  final EdgeInsets padding;
  final double fontSize;
  final FontWeight fontWeight;

  /// When set, overrides the default amber-50 background. The border is
  /// omitted when a custom background is provided.
  final Color? backgroundColor;

  /// When set, overrides the default primary-color text.
  final Color? textColor;

  const UnreadBadge({
    super.key,
    required this.count,
    this.padding = const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
    this.fontSize = 11,
    this.fontWeight = FontWeight.w700,
    this.backgroundColor,
    this.textColor,
  });

  @override
  Widget build(BuildContext context) {
    if (count == 0) {
      return const SizedBox.shrink();
    }

    final bg = backgroundColor;
    final fg = textColor ?? AppColors.primary(context);

    return Semantics(
      label: context.l10n.a11yMiscUnreadCount(count),
      container: true,
      child: ExcludeSemantics(
        child: Container(
          padding: padding,
          decoration: BoxDecoration(
            color: bg ?? const Color(0xFFFFFBEB), // amber-50
            borderRadius: BorderRadius.circular(10),
            border: bg == null
                ? Border.all(color: AppColors.primary(context), width: 1)
                : null,
            boxShadow: [
              BoxShadow(
                color: Colors.black.withAlpha(20),
                blurRadius: 8,
                offset: const Offset(0, 2),
              ),
            ],
          ),
          constraints: const BoxConstraints(minWidth: 20, minHeight: 20),
          child: Text(
            count > 99 ? '99+' : count.toString(),
            style: TextStyle(
              color: fg,
              fontSize: fontSize,
              fontWeight: fontWeight,
            ),
            textAlign: TextAlign.center,
          ),
        ),
      ),
    );
  }
}
