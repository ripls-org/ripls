import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';

/// ContentRemovedView displays when content has been deleted.
///
/// This widget provides a consistent UX when users navigate to
/// content that has been soft-deleted. It shows a friendly message
/// and provides navigation back to the previous screen.
///
/// Usage:
/// - Deep links to deleted items
/// - Cached references that no longer exist
/// - Conversation topics that have been removed
class ContentRemovedView extends StatelessWidget {
  /// Optional item type for customized messaging (e.g., "request", "gear")
  final String? itemType;

  /// Optional callback when back button is pressed
  final VoidCallback? onBack;

  /// Optional custom title
  final String? title;

  /// Optional custom message
  final String? message;

  const ContentRemovedView({
    super.key,
    this.itemType,
    this.onBack,
    this.title,
    this.message,
  });

  @override
  Widget build(BuildContext context) {
    final displayTitle = title ?? 'Item Removed';
    final displayMessage = message ??
        (itemType != null
            ? 'This $itemType has been removed and is no longer available.'
            : 'This item has been removed and is no longer available.');

    return Container(
      color: AppColors.surface(context),
      child: SafeArea(
        child: Column(
          children: [
            // Back button row
            if (onBack != null)
              Padding(
                padding:
                    const EdgeInsets.symmetric(horizontal: 8, vertical: 8),
                child: Row(
                  children: [
                    AppBarBackButton(
                      onPressed: onBack,
                    ),
                  ],
                ),
              ),

            // Centered content
            Expanded(
              child: Center(
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 32),
                  child: Column(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      // Icon
                      Icon(
                        Icons.delete_outline,
                        size: 64,
                        color: AppColors.textSecondary(context),
                      ),
                      const SizedBox(height: 24),

                      // Title
                      Text(
                        displayTitle,
                        style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                          color: AppColors.textPrimary(context),
                          fontWeight: FontWeight.bold,
                        ),
                        textAlign: TextAlign.center,
                      ),
                      const SizedBox(height: 12),

                      // Message
                      Text(
                        displayMessage,
                        style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                          color: AppColors.textSecondary(context),
                        ),
                        textAlign: TextAlign.center,
                      ),

                      // Back button (alternative to app bar back)
                      if (onBack != null) ...[
                        const SizedBox(height: 32),
                        OutlinedButton.icon(
                          onPressed: onBack,
                          icon: const Icon(Icons.arrow_back_ios_new),
                          label: const Text('Go Back'),
                          style: OutlinedButton.styleFrom(
                            foregroundColor: AppColors.textPrimary(context),
                            side: BorderSide(
                              color: AppColors.textSecondary(context),
                            ),
                          ),
                        ),
                      ],
                    ],
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
