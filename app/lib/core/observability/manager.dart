import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/observability/settings.dart';
import 'package:ripls/presentation/widgets/observability/consent_dialog.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('ObservabilityManager');

/// ObservabilityManager handles loading observability settings and showing
/// the consent dialog when needed.
///
/// Wrap the app's main content with this widget to enable automatic consent
/// prompting after authentication.
///
/// Usage:
/// ```dart
/// ObservabilityManager(
///   child: MaterialApp.router(...),
/// )
/// ```
class ObservabilityManager extends ConsumerStatefulWidget {
  final Widget child;

  const ObservabilityManager({
    super.key,
    required this.child,
  });

  @override
  ConsumerState<ObservabilityManager> createState() =>
      _ObservabilityManagerState();
}

class _ObservabilityManagerState extends ConsumerState<ObservabilityManager> {
  bool _isInitialized = false;

  @override
  void initState() {
    super.initState();

    // Load observability settings and initialize service on startup
    Future.microtask(() async {
      _log.info('Loading observability settings...');
      await ref.read(observabilitySettingsProvider.notifier).loadSettings();
      await _initializeService();
    });
  }

  Future<void> _initializeService() async {
    if (_isInitialized) return;

    final settings = ref.read(observabilitySettingsProvider);
    final service = ref.read(observabilityServiceProvider);

    _log.info('Initializing ObservabilityService with settings: $settings');
    await service.initialize(settings);
    _isInitialized = true;
  }

  @override
  Widget build(BuildContext context) {
    // Watch settings to react to consent changes
    final settings = ref.watch(observabilitySettingsProvider);

    // Update service when settings change (after initial load)
    if (_isInitialized && !settings.isLoading) {
      final service = ref.read(observabilityServiceProvider);
      if (service.settings != settings) {
        _log.info('Settings changed, updating ObservabilityService');
        service.updateConsent(settings);
      }
    }

    return widget.child;
  }
}

/// Widget to place inside the app's widget tree (below MaterialApp) to trigger
/// the consent dialog when needed.
///
/// Add this as a child of your main scaffold or home screen.
class ObservabilityConsentTrigger extends ConsumerStatefulWidget {
  final Widget child;

  const ObservabilityConsentTrigger({
    super.key,
    required this.child,
  });

  @override
  ConsumerState<ObservabilityConsentTrigger> createState() =>
      _ObservabilityConsentTriggerState();
}

class _ObservabilityConsentTriggerState
    extends ConsumerState<ObservabilityConsentTrigger> {
  bool _hasShownConsentDialog = false;
  bool _isShowingDialog = false;

  @override
  Widget build(BuildContext context) {
    // Watch auth and observability settings
    final authState = ref.watch(authStateProvider);
    final settings = ref.watch(observabilitySettingsProvider);

    // Check if we should show consent dialog after frame is built
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _maybeShowConsentDialog(context, authState.isAuthenticated, settings);
    });

    return widget.child;
  }

  void _maybeShowConsentDialog(
    BuildContext context,
    bool isAuthenticated,
    ObservabilitySettings settings,
  ) {
    // Don't show if already showing or already shown
    if (_isShowingDialog || _hasShownConsentDialog) {
      return;
    }

    // Don't show if settings are still loading
    if (settings.isLoading) {
      return;
    }

    // Don't show if consent has already been asked
    if (!settings.needsConsentPrompt) {
      _log.fine('Consent already collected, skipping dialog');
      return;
    }

    // Don't show if user is not authenticated (they should see it after login)
    if (!isAuthenticated) {
      _log.fine('User not authenticated, deferring consent dialog');
      return;
    }

    // Show the consent dialog
    _showConsentDialog(context);
  }

  Future<void> _showConsentDialog(BuildContext context) async {
    if (!mounted) return;

    _isShowingDialog = true;
    _log.info('Showing observability consent dialog');

    try {
      final result = await ObservabilityConsentDialog.show(
        context,
        isInitialPrompt: true,
      );

      // Mark as shown regardless of result so we don't show again this session
      _hasShownConsentDialog = true;

      if (result ?? false) {
        _log.info('User made consent decision');
      } else {
        _log.info('User dismissed consent dialog (will ask again next launch)');
      }
    } catch (e) {
      _log.warning('Error showing consent dialog: $e');
    } finally {
      _isShowingDialog = false;
    }
  }
}
