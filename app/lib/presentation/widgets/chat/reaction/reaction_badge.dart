import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart' show Reaction;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Displays emoji reactions below a message bubble, grouped by emoji with
/// counts. Tapping opens the reaction detail sheet.
class ReactionBadge extends StatelessWidget {
  /// The reactions to display.
  final List<Reaction> reactions;

  /// Called when the badge row is tapped (opens the detail sheet).
  final VoidCallback? onTap;

  const ReactionBadge({
    super.key,
    required this.reactions,
    this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    if (reactions.isEmpty) return const SizedBox.shrink();

    // Group reactions by emoji.
    final groups = <String, int>{};
    for (final r in reactions) {
      groups[r.emoji] = (groups[r.emoji] ?? 0) + 1;
    }

    final summary = groups.entries
        .map((e) => context.l10n.a11yChatReactionSummary(e.key, e.value))
        .join(', ');

    return Semantics(
      container: true,
      label: summary,
      child: Tappable(
        semanticsLabel: context.l10n.a11yChatViewReactions,
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.only(top: 2),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              for (final entry in groups.entries)
                Padding(
                  padding: const EdgeInsets.only(right: 4),
                  child: Text(
                    entry.value > 1 ? '${entry.key} ${entry.value}' : entry.key,
                    style: const TextStyle(fontSize: 14),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}
