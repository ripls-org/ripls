import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/viewmodels/splash_view_model.dart';
import 'package:ripls/presentation/widgets/font_fallback_warmup.dart';

/// SplashScreen displays during app initialization with branding and loading status.
/// Shows app logo, loading progress, and status messages during startup.
class SplashScreen extends ConsumerWidget {
  const SplashScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final splashState = ref.watch(splashProvider);
    final theme = Theme.of(context);

    return Scaffold(
      backgroundColor: theme.colorScheme.surface,
      body: Stack(
        children: [
          // Kicks off the web engine's lazy emoji/symbol font downloads while
          // the splash is on screen, so chat history, CO₂ labels, and weather
          // glyphs render on first paint instead of as tofu (#2724).
          const FontFallbackWarmup(),
          SafeArea(
            child: Center(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  const Spacer(flex: 2),
                  _buildLogo(theme),
                  const SizedBox(height: 48),
                  _buildLoadingIndicator(splashState, theme),
                  const SizedBox(height: 16),
                  _buildStatusMessage(context, splashState, theme),
                  if (splashState.hasError) ...[
                    const SizedBox(height: 24),
                    _buildErrorMessage(context, splashState, theme),
                    const SizedBox(height: 16),
                    _buildRetryButton(context, ref),
                  ],
                  const Spacer(flex: 3),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildLogo(ThemeData theme) {
    return ClipRRect(
      borderRadius: BorderRadius.circular(24),
      child: Image.asset(
        'assets/icon/icon_ios_1024.png',
        width: 120,
        height: 120,
        fit: BoxFit.cover,
      ),
    );
  }

  Widget _buildLoadingIndicator(SplashState state, ThemeData theme) {
    if (state.hasError) {
      return Icon(
        Icons.error_outline,
        size: 48,
        color: theme.colorScheme.error,
      );
    }

    if (state.step == SplashLoadingStep.ready) {
      return Icon(
        Icons.check_circle_outline,
        size: 48,
        color: theme.colorScheme.primary,
      );
    }

    // Show progress based on current step
    final progress = _getStepProgress(state.step);
    return SizedBox(
      width: 48,
      height: 48,
      child: CircularProgressIndicator(
        strokeWidth: 3,
        value: progress,
        valueColor: AlwaysStoppedAnimation<Color>(theme.colorScheme.primary),
        backgroundColor: theme.colorScheme.primary.withValues(alpha: 0.2),
      ),
    );
  }

  double _getStepProgress(SplashLoadingStep step) {
    switch (step) {
      case SplashLoadingStep.initializing:
        return 0.2;
      case SplashLoadingStep.checkingAuth:
        return 0.4;
      case SplashLoadingStep.loadingCommunities:
        return 0.6;
      case SplashLoadingStep.loadingFeed:
        return 0.8;
      case SplashLoadingStep.ready:
        return 1;
      case SplashLoadingStep.error:
        return 0;
    }
  }

  /// User-facing caption for the current step. Deliberately NOT the
  /// developer-facing [SplashState.statusMessage] (which names internal
  /// stages like "Checking authentication..." and is kept for logs):
  /// users just see a neutral, localized "One moment…" / "Loading…"
  /// (#2724).
  Widget _buildStatusMessage(
      BuildContext context, SplashState state, ThemeData theme) {
    final l10n = context.l10n;
    final caption = switch (state.step) {
      SplashLoadingStep.initializing ||
      SplashLoadingStep.checkingAuth =>
        l10n.splashStatusOneMoment,
      SplashLoadingStep.loadingCommunities ||
      SplashLoadingStep.loadingFeed =>
        l10n.splashStatusLoading,
      // Ready/error need no caption: the check icon speaks for ready, and
      // the error step renders its own localized message + retry below.
      SplashLoadingStep.ready || SplashLoadingStep.error => '',
    };
    return Text(
      caption,
      style: theme.textTheme.bodyMedium?.copyWith(
        color: theme.colorScheme.onSurface.withValues(alpha: 0.7),
      ),
      textAlign: TextAlign.center,
    );
  }

  Widget _buildErrorMessage(
      BuildContext context, SplashState state, ThemeData theme) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 32),
      child: Text(
        state.error == null
            ? 'An error occurred'
            : RpcErrorHandler.localize(state.error!, context.l10n),
        style: theme.textTheme.bodySmall?.copyWith(
          color: theme.colorScheme.error,
        ),
        textAlign: TextAlign.center,
        maxLines: 3,
        overflow: TextOverflow.ellipsis,
      ),
    );
  }

  Widget _buildRetryButton(BuildContext context, WidgetRef ref) {
    return ElevatedButton.icon(
      onPressed: () {
        ref.read(splashProvider.notifier).clearError();
        // Trigger app reload - this will be handled by parent widget
      },
      icon: const Icon(Icons.refresh),
      label: Text(context.l10n.commonRetry),
    );
  }
}
