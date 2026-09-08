import 'package:flutter/material.dart';

/// ContentErrorView provides standardized error display for content loading failures.
///
/// This widget follows the Ripls architecture:
/// - Displays error icon, title, message, and retry button
/// - Consistent error UX across all content types
/// - Pure presentation component with zero business logic
/// - Black background suitable for overlay on media content
class ContentErrorView extends StatelessWidget {
  /// Error message to display
  final String errorMessage;

  /// Title text (defaults to 'Failed to load')
  final String title;

  /// Retry button label (defaults to 'Retry')
  final String retryLabel;

  /// Callback when retry button is pressed
  final VoidCallback? onRetry;

  /// Icon to display (defaults to error_outline)
  final IconData icon;

  /// Icon size (defaults to 48)
  final double iconSize;

  const ContentErrorView({
    super.key,
    required this.errorMessage,
    this.title = 'Failed to load',
    this.retryLabel = 'Retry',
    this.onRetry,
    this.icon = Icons.error_outline,
    this.iconSize = 48,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      color: Colors.black,
      child: Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(icon, color: Colors.white, size: iconSize),
            const SizedBox(height: 16),
            Text(
              title,
              style: const TextStyle(
                color: Colors.white,
                fontSize: 18,
                fontWeight: FontWeight.bold,
              ),
            ),
            const SizedBox(height: 8),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 24),
              child: Text(
                errorMessage,
                style: const TextStyle(color: Colors.white70, fontSize: 14),
                textAlign: TextAlign.center,
              ),
            ),
            if (onRetry != null) ...[
              const SizedBox(height: 16),
              ElevatedButton(
                onPressed: onRetry,
                child: Text(retryLabel),
              ),
            ],
          ],
        ),
      ),
    );
  }
}
