import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Default quick-reaction emoji shown in the picker bar.
const List<String> defaultQuickReactions = ['❤️', '👍', '👎', '😂', '😮', '😢'];

/// A single action (Reply / Edit / Delete) offered in the message long-press
/// menu beneath the emoji bar.
class MessageAction {
  /// Icon shown at the leading edge of the row.
  final IconData icon;

  /// Visible label.
  final String label;

  /// Accessibility label (from `context.l10n`).
  final String semanticsLabel;

  /// Invoked when the row is tapped.
  final VoidCallback onTap;

  /// When true, the row is rendered in the error color (e.g. Delete).
  final bool isDestructive;

  const MessageAction({
    required this.icon,
    required this.label,
    required this.semanticsLabel,
    required this.onTap,
    this.isDestructive = false,
  });
}

/// A pill-shaped row of quick-reaction emoji plus an ellipsis button for the
/// full system emoji picker, optionally followed by an action card (Reply /
/// Edit / Delete). Displayed above a message on long-press.
class ReactionPicker extends StatelessWidget {
  /// Called when the user taps a quick emoji.
  final ValueChanged<String> onEmojiSelected;

  /// Called when the user taps the "..." button for the full picker.
  final VoidCallback onExpandPicker;

  /// Context actions shown beneath the emoji bar. Empty hides the action card.
  final List<MessageAction> actions;

  const ReactionPicker({
    super.key,
    required this.onEmojiSelected,
    required this.onExpandPicker,
    this.actions = const [],
  });

  @override
  Widget build(BuildContext context) {
    // Wrap in a Material so descendant Text inherits the app's themed
    // DefaultTextStyle. Without a Material ancestor, Text in an Overlay falls
    // back to the engine default (monospace), which doesn't match the rest of
    // the chat UI.
    return Material(
      type: MaterialType.transparency,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          _buildEmojiBar(context),
          if (actions.isNotEmpty) ...[
            const SizedBox(height: 8),
            _buildActionCard(context),
          ],
        ],
      ),
    );
  }

  Widget _buildEmojiBar(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 4),
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context),
        borderRadius: BorderRadius.circular(24),
        border: Border.all(color: AppColors.border(context), width: 0.5),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.25),
            blurRadius: 12,
            offset: const Offset(0, 4),
          ),
        ],
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          for (final emoji in defaultQuickReactions)
            _EmojiButton(
              emoji: emoji,
              onTap: () => onEmojiSelected(emoji),
            ),
          _EmojiButton(
            emoji: '···',
            onTap: onExpandPicker,
            isEllipsis: true,
          ),
        ],
      ),
    );
  }

  Widget _buildActionCard(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context),
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: AppColors.border(context), width: 0.5),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.25),
            blurRadius: 12,
            offset: const Offset(0, 4),
          ),
        ],
      ),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(14),
        // IntrinsicWidth sizes the card to its widest row so the menu is only
        // as wide as necessary; stretch makes every row share that width so the
        // trailing icons line up.
        child: IntrinsicWidth(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              for (var i = 0; i < actions.length; i++) ...[
                if (i > 0)
                  Divider(
                    height: 0.5,
                    thickness: 0.5,
                    color: AppColors.divider(context),
                  ),
                _ActionRow(action: actions[i]),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

class _ActionRow extends StatelessWidget {
  final MessageAction action;

  const _ActionRow({required this.action});

  @override
  Widget build(BuildContext context) {
    final color = action.isDestructive
        ? AppColors.statusError(context)
        : AppColors.textPrimary(context);
    return Tappable(
      semanticsLabel: action.semanticsLabel,
      onTap: action.onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        child: Row(
          children: [
            Text(
              action.label,
              style: TextStyle(
                fontSize: 16,
                color: color,
                fontWeight: FontWeight.w500,
              ),
            ),
            const SizedBox(width: 24),
            const Spacer(),
            Icon(action.icon, size: 20, color: color),
          ],
        ),
      ),
    );
  }
}

class _EmojiButton extends StatelessWidget {
  final String emoji;
  final VoidCallback onTap;
  final bool isEllipsis;

  const _EmojiButton({
    required this.emoji,
    required this.onTap,
    this.isEllipsis = false,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: isEllipsis
          ? context.l10n.a11yChatExpandEmojiPicker
          : context.l10n.a11yChatPickEmoji(emoji),
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 3),
        child: Text(
          emoji,
          style: TextStyle(
            fontSize: isEllipsis ? 17 : 24,
            color: isEllipsis ? AppColors.textTertiary(context) : null,
            decoration: TextDecoration.none,
          ),
        ),
      ),
    );
  }
}
