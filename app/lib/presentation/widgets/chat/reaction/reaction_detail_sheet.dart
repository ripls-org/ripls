import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart' show Reaction;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// Bottom sheet that shows who reacted to a message, with filter tabs by emoji.
///
/// The current user's row shows "Tap to remove" and calls [onRemove] on tap.
class ReactionDetailSheet extends StatefulWidget {
  final List<Reaction> reactions;
  final String currentUserId;
  final VoidCallback? onRemove;

  const ReactionDetailSheet({
    super.key,
    required this.reactions,
    required this.currentUserId,
    this.onRemove,
  });

  @override
  State<ReactionDetailSheet> createState() => _ReactionDetailSheetState();
}

class _ReactionDetailSheetState extends State<ReactionDetailSheet> {
  String? _selectedEmoji; // null = "All"

  @override
  Widget build(BuildContext context) {
    final groups = <String, List<Reaction>>{};
    for (final r in widget.reactions) {
      groups.putIfAbsent(r.emoji, () => []).add(r);
    }

    final filtered = _selectedEmoji == null
        ? widget.reactions
        : (groups[_selectedEmoji] ?? []);

    return GlassSheet(
      applyMaxHeight: false,
      padding: EdgeInsets.zero,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Filter tabs.
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              child: Row(
                children: [
                  _FilterChip(
                    label: 'All \u00b7 ${widget.reactions.length}',
                    selected: _selectedEmoji == null,
                    onTap: () => setState(() => _selectedEmoji = null),
                  ),
                  const SizedBox(width: 8),
                  for (final entry in groups.entries) ...[
                    _FilterChip(
                      label: '${entry.key} ${entry.value.length}',
                      selected: _selectedEmoji == entry.key,
                      onTap: () =>
                          setState(() => _selectedEmoji = entry.key),
                    ),
                    const SizedBox(width: 8),
                  ],
                ],
              ),
            ),
          ),
          const SizedBox(height: 12),
          // User list.
          ...filtered.map((r) => _buildReactorRow(context, r)),
          const SizedBox(height: 16),
        ],
      ),
    );
  }

  Widget _buildReactorRow(BuildContext context, Reaction reaction) {
    final isCurrentUser = reaction.sender.id == widget.currentUserId;
    return Tappable(
      semanticsLabel: isCurrentUser
          ? context.l10n.a11yChatRemoveReaction
          : reaction.sender.name,
      onTap: isCurrentUser ? _handleRemove : null,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
        child: Row(
          children: [
            UserAvatar(user: reaction.sender, radius: 18),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    isCurrentUser ? 'You' : reaction.sender.name,
                    style: TextStyle(
                      color: AppColors.modalTextPrimary,
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                    ),
                  ),
                  if (isCurrentUser)
                    Text(
                      'Tap to remove',
                      style: TextStyle(
                        color: AppColors.modalTextMuted,
                        fontSize: 13,
                      ),
                    ),
                ],
              ),
            ),
            Text(reaction.emoji, style: const TextStyle(fontSize: 24)),
          ],
        ),
      ),
    );
  }

  void _handleRemove() {
    widget.onRemove?.call();
    Navigator.pop(context);
  }
}

class _FilterChip extends StatelessWidget {
  final String label;
  final bool selected;
  final VoidCallback onTap;

  const _FilterChip({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Toggle(
      semanticsLabel: label,
      selected: selected,
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
        decoration: BoxDecoration(
          color: selected
              ? AppColors.primary(context).withValues(alpha: 0.2)
              : AppColors.modalInsetCardBg,
          borderRadius: BorderRadius.circular(16),
          border: Border.all(
            color: selected
                ? AppColors.primary(context)
                : AppColors.modalInsetCardBorder,
          ),
        ),
        child: Text(
          label,
          style: TextStyle(
            color: selected
                ? AppColors.primary(context)
                : AppColors.modalTextSecondary,
            fontSize: 13,
            fontWeight: FontWeight.w600,
          ),
        ),
      ),
    );
  }
}
