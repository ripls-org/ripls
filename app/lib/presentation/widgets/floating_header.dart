import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../core/theme/app_colors.dart';
import '../../core/theme/app_theme.dart';
import 'accessibility/tappable.dart';

/// FloatingHeader provides a reusable floating header component for all screens.
/// Renders an optional [leading] widget, title or custom content in the center,
/// and an optional [trailing] widget. Callers that want a leading control pass
/// their own widget (e.g. an avatar or icon) along with [onLeadingTap].
class FloatingHeader extends ConsumerWidget {
  /// Height of the floating header container
  static const double headerHeight = 48;

  /// Spacing below the floating header before content starts
  static const double headerSpacing = 8;

  /// Height of the floating bottom navigation container
  static const double navHeight = 54;

  /// Optional title text to display (if no custom content provided)
  final String? title;

  /// Optional custom content widget to display in the header
  final Widget? content;

  /// Optional trailing widget (e.g., buttons, icons)
  final Widget? trailing;

  /// Spacing between leading widget and content (defaults to 12px).
  final double contentSpacing;

  /// Optional leading widget rendered before the content.
  final Widget? leading;

  /// Optional tap handler for the [leading] widget.
  final VoidCallback? onLeadingTap;

  /// Optional a11y semantics label for the [leading] tap target.
  final String? leadingSemanticsLabel;

  const FloatingHeader({
    super.key,
    this.title,
    this.content,
    this.trailing,
    this.contentSpacing = 12.0,
    this.leading,
    this.onLeadingTap,
    this.leadingSemanticsLabel,
  });

  /// Returns the top padding needed to position content below the floating header.
  /// Includes safe area top padding + header height + spacing.
  static double contentTop(BuildContext context) {
    return MediaQuery.of(context).padding.top + headerHeight + headerSpacing;
  }

  /// Returns the bottom padding needed to position content above the floating navigation.
  /// Includes safe area bottom padding + nav height + spacing.
  static double contentBottom(BuildContext context) {
    final bottomPadding = MediaQuery.of(context).padding.bottom;
    return (bottomPadding > 0 ? bottomPadding : 12) + navHeight + 12;
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Container(
      height: 48,
      clipBehavior: Clip.none,
      margin: const EdgeInsets.symmetric(horizontal: 16),
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context).withAlpha(217),
        borderRadius: BorderRadius.circular(24),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withAlpha(25),
            blurRadius: 8,
            offset: const Offset(0, 2),
          ),
        ],
      ),
      child: Row(
        children: [
          const SizedBox(width: 12),
          if (leading != null) ...[
            if (onLeadingTap != null)
              Tappable(
                semanticsLabel: leadingSemanticsLabel ?? '',
                onTap: onLeadingTap!,
                excludeChildSemantics: false,
                child: leading!,
              )
            else
              leading!,
            SizedBox(width: contentSpacing),
          ],
          if (content != null)
            Expanded(child: content!)
          else if (title != null)
            Expanded(
              child: Text(
                title!,
                style: AppTheme.communityHeaderStyle.copyWith(
                  color: AppColors.textPrimary(context),
                ),
                overflow: TextOverflow.ellipsis,
                maxLines: 1,
                textAlign: TextAlign.center,
              ),
            )
          else
            const Spacer(),
          ?trailing,
          const SizedBox(width: 12),
        ],
      ),
    );
  }
}
