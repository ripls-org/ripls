import 'package:flutter/material.dart';
import 'package:ripls/presentation/models/chat_message.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/reaction/reaction_badge.dart';

/// SystemMessageRow renders a centered system message label with optional
/// reaction badge.
///
/// Long-pressing the label invokes [onLongPress] when provided (typically to
/// open the reaction picker).
class SystemMessageRow extends StatelessWidget {
  const SystemMessageRow({
    super.key,
    required this.text,
    required this.chatMessage,
    this.onLongPress,
    this.onReactionBadgeTap,
  });

  final String text;
  final ChatMessage chatMessage;

  /// Called when the user long-presses the system label.
  final VoidCallback? onLongPress;

  /// Called when the user taps the reaction badge below the label.
  final VoidCallback? onReactionBadgeTap;

  @override
  Widget build(BuildContext context) {
    final hasReactions = chatMessage.reactions.isNotEmpty;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Column(
        children: [
          Builder(
            builder: (bubbleContext) => Tappable(
              semanticsLabel: text,
              onTap: null,
              onLongPress: onLongPress,
              excludeChildSemantics: false,
              child: Center(
                child: Text(
                  text,
                  textAlign: TextAlign.center,
                  style: const TextStyle(
                    color: Colors.white,
                    fontSize: 13,
                  ),
                ),
              ),
            ),
          ),
          if (hasReactions)
            ReactionBadge(
              reactions: chatMessage.reactions,
              onTap: onReactionBadgeTap ?? () {},
            ),
        ],
      ),
    );
  }
}
