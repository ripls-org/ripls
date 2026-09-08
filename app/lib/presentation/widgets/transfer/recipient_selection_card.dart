import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// RecipientSelectionCard displays a selectable user card for giveaways.
///
/// Re-skinned to glass: a [GlassInsetCard] sits inside the parent
/// [GlassSheet]. Selected state flips the card to a sage-tinted accent
/// background with a sage check badge — sage and coral are retained as
/// semantic accents under the glass migration.
class RecipientSelectionCard extends StatelessWidget {
  /// User object
  final User user;

  /// Optional message from the user requesting the item
  final String? message;

  /// Whether this card is currently selected
  final bool isSelected;

  /// Callback when the card is tapped
  final VoidCallback onTap;

  /// Optional timestamp for the request
  final String? timestamp;

  RecipientSelectionCard({
    Key? key,
    required this.user,
    this.message,
    required this.isSelected,
    required this.onTap,
    this.timestamp,
  }) : super(key: key ?? Key('recipient_selection_card_${user.id}'));

  @override
  Widget build(BuildContext context) {
    return Toggle(
      semanticsLabel: user.name,
      selected: isSelected,
      onTap: onTap,
      child: Container(
        margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
        child: GlassInsetCard(
          padding: const EdgeInsets.all(16),
          fill: isSelected
              ? AppColors.transferSage.withValues(alpha: 0.18)
              : null,
          border: isSelected ? AppColors.transferSage : null,
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              UserAvatar(
                user: user,
                radius: 24,
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Expanded(
                          child: Text(
                            user.name,
                            style: Theme.of(context)
                                .textTheme
                                .bodyLarge
                                ?.copyWith(
                                  fontWeight: FontWeight.w600,
                                  color: AppColors.modalTextPrimary,
                                ),
                          ),
                        ),
                        if (isSelected)
                          Container(
                            padding: const EdgeInsets.all(4),
                            decoration: const BoxDecoration(
                              color: AppColors.transferSage,
                              shape: BoxShape.circle,
                            ),
                            child: const Icon(
                              Icons.check,
                              color: Colors.white,
                              size: 16,
                            ),
                          ),
                      ],
                    ),
                    if (timestamp != null) ...[
                      const SizedBox(height: 4),
                      Text(
                        timestamp!,
                        style: Theme.of(context).textTheme.bodySmall?.copyWith(
                              color: AppColors.modalTextMuted,
                            ),
                      ),
                    ],
                    if (message != null && message!.isNotEmpty) ...[
                      const SizedBox(height: 8),
                      Container(
                        padding: const EdgeInsets.all(12),
                        decoration: BoxDecoration(
                          color: AppColors.modalSearchFieldBackground,
                          borderRadius: BorderRadius.circular(8),
                          border: Border.all(
                            color: AppColors.modalSearchFieldBorder,
                          ),
                        ),
                        child: Text(
                          message!,
                          style:
                              Theme.of(context).textTheme.bodyMedium?.copyWith(
                                    color: AppColors.modalTextSecondary,
                                  ),
                        ),
                      ),
                    ],
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
