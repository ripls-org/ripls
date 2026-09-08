import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/user_error.dart';

part 'splash_view_model.freezed.dart';

final _log = Logger('SplashNotifier');

/// Splash screen loading states
enum SplashLoadingStep {
  initializing,
  checkingAuth,
  loadingCommunities,
  loadingFeed,
  ready,
  error,
}

@freezed
sealed class SplashState with _$SplashState {
  const factory SplashState({
    @Default(SplashLoadingStep.initializing) SplashLoadingStep step,
    @Default('Initializing...') String statusMessage,
    UserError? error,
    @Default(false) bool hasError,
  }) = _SplashState;
}

/// SplashNotifier manages the splash screen loading state and coordination.
/// It provides status updates during app initialization.
class SplashNotifier extends Notifier<SplashState> {
  @override
  SplashState build() {
    return const SplashState();
  }

  /// Updates the current loading step with appropriate status message.
  void updateStep(SplashLoadingStep step) {
    _log.info('🎬 Splash screen step: ${step.name} -> "${_getStatusMessage(step)}"');
    final message = _getStatusMessage(step);
    state = state.copyWith(
      step: step,
      statusMessage: message,
      hasError: false,
      error: null,
    );
  }

  /// Sets error state with custom message.
  void setError(String message) {
    state = state.copyWith(
      step: SplashLoadingStep.error,
      statusMessage: 'Failed to load',
      error: UserError.generic(fallback: message),
      hasError: true,
    );
  }

  /// Clears error state and returns to initializing.
  void clearError() {
    state = const SplashState();
  }

  String _getStatusMessage(SplashLoadingStep step) {
    switch (step) {
      case SplashLoadingStep.initializing:
        return 'Initializing...';
      case SplashLoadingStep.checkingAuth:
        return 'Checking authentication...';
      case SplashLoadingStep.loadingCommunities:
        return 'Loading communities...';
      case SplashLoadingStep.loadingFeed:
        return 'Loading feed...';
      case SplashLoadingStep.ready:
        return 'Ready';
      case SplashLoadingStep.error:
        return 'Failed to load';
    }
  }
}

final splashProvider = NotifierProvider<SplashNotifier, SplashState>(
  SplashNotifier.new,
);
