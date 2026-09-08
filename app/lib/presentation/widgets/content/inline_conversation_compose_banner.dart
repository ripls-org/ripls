import 'package:flutter/material.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// ComposeBanner is the strip shown above the chat input bar while editing a
/// message or composing a reply. It shows an accent bar, a title (and optional
/// quoted snippet for replies), and an "✕" cancel affordance.
///
/// Colors derive from [accentColor] and white-on-dark so it sits correctly on
/// the immersive conversation background.
class ComposeBanner extends StatelessWidget {
  const ComposeBanner({
    super.key,
    required this.title,
    required this.cancelSemanticsLabel,
    required this.onCancel,
    required this.accentColor,
    this.subtitle,
  });

  /// Banner title (e.g. "Editing message" or "Replying to Alex").
  final String title;

  /// Optional quoted snippet shown beneath the title for replies.
  final String? subtitle;

  /// Accessibility label for the cancel button.
  final String cancelSemanticsLabel;

  /// Invoked when the cancel button is tapped.
  final VoidCallback onCancel;

  /// Accent color for the leading bar and title.
  final Color accentColor;

  @override
  Widget build(BuildContext context) {
    final hasSubtitle = subtitle != null && subtitle!.isNotEmpty;
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 4, 12, 0),
      child: Row(
        children: [
          Container(
            width: 3,
            height: hasSubtitle ? 32 : 18,
            margin: const EdgeInsets.only(right: 10),
            decoration: BoxDecoration(
              color: accentColor,
              borderRadius: BorderRadius.circular(2),
            ),
          ),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  title,
                  style: TextStyle(
                    color: accentColor,
                    fontSize: 12,
                    fontWeight: FontWeight.w600,
                  ),
                ),
                if (hasSubtitle)
                  Text(
                    subtitle!,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      color: Colors.white.withValues(alpha: 0.55),
                      fontSize: 12,
                    ),
                  ),
              ],
            ),
          ),
          Tappable(
            semanticsLabel: cancelSemanticsLabel,
            onTap: onCancel,
            child: Padding(
              padding: const EdgeInsets.all(6),
              child: Icon(
                Icons.close,
                size: 18,
                color: Colors.white.withValues(alpha: 0.6),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
