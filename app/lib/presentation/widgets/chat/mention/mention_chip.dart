import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'mention_types.dart';

/// A styled chip/pill for displaying a mention in message text.
///
/// Tappable to navigate to the mentioned entity's detail screen.
class MentionChip extends StatelessWidget {
  /// The type of mention.
  final MentionType type;

  /// The display name to show.
  final String displayName;

  /// Callback when the chip is tapped.
  final VoidCallback? onTap;

  /// Whether this chip is displayed on a dark background (e.g., outgoing chat bubble).
  /// When true, uses inverted colors for better readability.
  final bool onDarkBackground;

  const MentionChip({
    super.key,
    required this.type,
    required this.displayName,
    this.onTap,
    this.onDarkBackground = false,
  });

  /// Creates a MentionChip from a ParsedMention.
  factory MentionChip.fromParsed({
    Key? key,
    required ParsedMention mention,
    VoidCallback? onTap,
    bool onDarkBackground = false,
  }) {
    return MentionChip(
      key: key,
      type: mention.type,
      displayName: mention.displayName,
      onTap: onTap,
      onDarkBackground: onDarkBackground,
    );
  }

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: context.l10n.a11yMention(displayName),
      isLink: true,
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
        decoration: BoxDecoration(
          color: _getBackgroundColor(context),
          borderRadius: BorderRadius.circular(4),
        ),
        child: Text(
          '@$displayName',
          style: TextStyle(
            color: _getTextColor(context),
            fontSize: 14,
            fontWeight: FontWeight.w500,
          ),
        ),
      ),
    );
  }

  // Blue color for mention chips (consistent across all types)
  static const Color _chipBlue = Color(0xFF5A7A82); // Stone blue

  Color _getBackgroundColor(BuildContext context) {
    if (onDarkBackground) {
      // On dark backgrounds, use semi-transparent white
      return Colors.white.withValues(alpha: 0.2);
    }
    // On light backgrounds, use blue tint
    return _chipBlue.withValues(alpha: 0.15);
  }

  Color _getTextColor(BuildContext context) {
    if (onDarkBackground) {
      // On dark backgrounds, use white text
      return Colors.white;
    }
    // On light backgrounds, use blue text
    return _chipBlue;
  }
}
