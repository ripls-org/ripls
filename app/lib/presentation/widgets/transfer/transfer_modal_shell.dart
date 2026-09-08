import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

/// Shared modal shell used by glass transfer flows. Wraps content in a
/// [GlassSheet] with the standard modal shape (12px inset, 28px top
/// corners, 92% max height) so every transfer modal matches the rest of
/// the glass-modal family.
class TransferModalShell extends StatelessWidget {
  const TransferModalShell({
    super.key,
    this.header,
    required this.content,
    this.bottomBar,
    this.isLoading = false,
    this.errorMessage,
    this.onRetry,
  });

  final Widget? header;
  final Widget content;
  final Widget? bottomBar;
  final bool isLoading;
  final String? errorMessage;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    return GlassSheet(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          ?header,
          Expanded(
            child: isLoading
                ? Center(
                    child: Semantics(
                      label: context.l10n.a11yLoading,
                      liveRegion: true,
                      child: const CircularProgressIndicator(
                        valueColor:
                            AlwaysStoppedAnimation(AppColors.modalTextPrimary),
                      ),
                    ),
                  )
                : errorMessage != null
                    ? LiveRegion(
                        child: Center(
                          child: Padding(
                            padding: const EdgeInsets.all(24),
                            child: Column(
                              mainAxisAlignment: MainAxisAlignment.center,
                              children: [
                                const Icon(
                                  Icons.error_outline,
                                  size: 48,
                                  color: AppColors.transferCoral,
                                ),
                                const SizedBox(height: 16),
                                Text(
                                  'Error',
                                  style: Theme.of(context)
                                      .textTheme
                                      .titleLarge
                                      ?.copyWith(
                                          color: AppColors.modalTextPrimary),
                                ),
                                const SizedBox(height: 8),
                                Text(
                                  errorMessage!,
                                  textAlign: TextAlign.center,
                                  style: Theme.of(context)
                                      .textTheme
                                      .bodyMedium
                                      ?.copyWith(
                                          color: AppColors.modalTextSecondary),
                                ),
                                if (onRetry != null) ...[
                                  const SizedBox(height: 24),
                                  ElevatedButton(
                                    onPressed: onRetry,
                                    style: ElevatedButton.styleFrom(
                                      backgroundColor:
                                          AppColors.modalPrimaryButtonBackground,
                                      foregroundColor:
                                          AppColors.modalPrimaryButtonText,
                                    ),
                                    child: Text(context.l10n.commonRetry),
                                  ),
                                ],
                              ],
                            ),
                          ),
                        ),
                      )
                    : content,
          ),
          ?bottomBar,
        ],
      ),
    );
  }
}
