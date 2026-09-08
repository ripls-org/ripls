import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';

/// ContentErrorBanner displays inline error messages with consistent styling.
///
/// This banner is used to show validation errors or API errors within content
/// forms and modals. It provides a prominent red-bordered container with error text.
///
/// Usage:
/// ```dart
/// if (state.hasError)
///   ContentErrorBanner(errorMessage: state.errorMessage!)
/// ```
class ContentErrorBanner extends StatelessWidget {
  /// The error message to display
  final String errorMessage;

  /// Optional icon to show before the message (defaults to warning icon)
  final IconData? icon;

  /// Whether to show an icon (defaults to true)
  final bool showIcon;

  const ContentErrorBanner({
    super.key,
    required this.errorMessage,
    this.icon,
    this.showIcon = true,
  });

  @override
  Widget build(BuildContext context) {
    return LiveRegion(
      child: Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: AppColors.statusError(context).withAlpha(50),
          borderRadius: BorderRadius.circular(8),
          border: Border.all(color: AppColors.statusError(context), width: 1),
        ),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (showIcon) ...[
              Icon(
                icon ?? Icons.warning_amber_rounded,
                color: AppColors.statusError(context),
                size: 20,
              ),
              const SizedBox(width: 8),
            ],
            Expanded(
              child: Text(
                errorMessage,
                style: TextStyle(
                  color: AppColors.statusError(context),
                  fontSize: 14,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
