import 'package:flutter/material.dart';

import '../../core/theme/app_colors.dart';
import 'feedback/feedback_sheet.dart';

/// ServerErrorScreen displays a full-screen error state when the server is
/// unreachable, replacing the normal home screen (including bottom nav).
///
/// Receives [errorMessage], [onRetry] callback, and [isOffline] flag.
/// Pure presentation — no business logic or repository access.
class ServerErrorScreen extends StatelessWidget {
  final String errorMessage;
  final VoidCallback onRetry;
  final bool isOffline;
  final bool isRetrying;

  const ServerErrorScreen({
    required this.errorMessage,
    required this.onRetry,
    this.isOffline = false,
    this.isRetrying = false,
    super.key,
  });

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.background(context),
      body: SafeArea(
        child: Center(
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 32),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                _buildIcon(context),
                const SizedBox(height: 24),
                _buildTitle(context),
                const SizedBox(height: 12),
                _buildMessage(context),
                const SizedBox(height: 40),
                _buildRetryButton(context),
                const SizedBox(height: 16),
                _buildReportLink(context),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildIcon(BuildContext context) {
    final icon = isOffline ? Icons.wifi_off_rounded : Icons.cloud_off_rounded;
    return Icon(
      icon,
      size: 72,
      color: AppColors.textSecondary(context),
    );
  }

  Widget _buildTitle(BuildContext context) {
    final title = isOffline ? "You're offline" : 'Server unavailable';
    return Text(
      title,
      style: Theme.of(context).textTheme.headlineSmall?.copyWith(
        fontWeight: FontWeight.bold,
        color: AppColors.textPrimary(context),
      ),
      textAlign: TextAlign.center,
    );
  }

  Widget _buildMessage(BuildContext context) {
    final message = isOffline
        ? 'Check your internet connection and try again.'
        : 'The server could not be reached. This may be a temporary issue.';
    return Text(
      message,
      style: Theme.of(context).textTheme.bodyMedium?.copyWith(
        color: AppColors.textSecondary(context),
      ),
      textAlign: TextAlign.center,
    );
  }

  Widget _buildRetryButton(BuildContext context) {
    return SizedBox(
      width: double.infinity,
      child: FilledButton.icon(
        onPressed: isRetrying ? null : onRetry,
        icon: isRetrying
            ? const SizedBox(
                width: 16,
                height: 16,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            : const Icon(Icons.refresh),
        label: const Text('Try Again'),
      ),
    );
  }

  Widget _buildReportLink(BuildContext context) {
    return TextButton(
      onPressed: () => FeedbackSheet.show(context),
      child: Text(
        'Report a Problem',
        style: TextStyle(color: AppColors.textSecondary(context)),
      ),
    );
  }
}
