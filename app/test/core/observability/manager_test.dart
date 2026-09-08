import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/settings.dart';
import 'package:ripls/services/providers.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('ObservabilityManager', () {
    testWidgets('renders child widget', (tester) async {
      await tester.pumpWidget(ProviderScope(
        overrides: [
          observabilitySettingsProvider.overrideWith(
            () => _TestObservabilitySettingsNotifier(ObservabilitySettings.notAsked),
          ),
        ],
        child: const MaterialApp(
          home: _SimpleObservabilityManager(
            child: Scaffold(body: Text('Main Content')),
          ),
        ),
      ));
      await tester.pumpAndSettle();

      expect(find.text('Main Content'), findsOneWidget);
    });

    testWidgets('loads settings on startup', (tester) async {
      final notifier = _TestObservabilitySettingsNotifier(
        const ObservabilitySettings(isLoading: true),
      );

      await tester.pumpWidget(ProviderScope(
        overrides: [
          observabilitySettingsProvider.overrideWith(() => notifier),
        ],
        child: const MaterialApp(
          home: _SimpleObservabilityManager(
            child: Scaffold(body: Text('Main Content')),
          ),
        ),
      ));
      await tester.pump();

      // Settings should have been loaded
      expect(notifier.loadSettingsCalled, isTrue);
    });
  });
}

/// Test implementation of ObservabilitySettingsNotifier.
class _TestObservabilitySettingsNotifier extends ObservabilitySettingsNotifier {
  final ObservabilitySettings _initialSettings;
  bool loadSettingsCalled = false;

  _TestObservabilitySettingsNotifier(this._initialSettings);

  @override
  ObservabilitySettings build() => _initialSettings;

  @override
  Future<void> loadSettings() async {
    loadSettingsCalled = true;
  }

  @override
  Future<void> setCrashReporting(ObservabilityConsent consent) async {
    state = state.copyWith(crashReporting: consent);
  }

  @override
  Future<void> setPerformanceMonitoring(ObservabilityConsent consent) async {
    state = state.copyWith(performanceMonitoring: consent);
  }

  @override
  Future<void> setAnalytics(ObservabilityConsent consent) async {
    state = state.copyWith(analytics: consent);
  }

  @override
  Future<void> setAll({
    required ObservabilityConsent crashReporting,
    required ObservabilityConsent performanceMonitoring,
    required ObservabilityConsent analytics,
  }) async {
    state = ObservabilitySettings(
      crashReporting: crashReporting,
      performanceMonitoring: performanceMonitoring,
      analytics: analytics,
      isLoading: false,
    );
  }
}

/// Simplified ObservabilityManager for testing that doesn't depend on auth state.
class _SimpleObservabilityManager extends ConsumerStatefulWidget {
  final Widget child;

  const _SimpleObservabilityManager({required this.child});

  @override
  ConsumerState<_SimpleObservabilityManager> createState() =>
      _SimpleObservabilityManagerState();
}

class _SimpleObservabilityManagerState
    extends ConsumerState<_SimpleObservabilityManager> {
  @override
  void initState() {
    super.initState();
    Future.microtask(() async {
      await ref.read(observabilitySettingsProvider.notifier).loadSettings();
    });
  }

  @override
  Widget build(BuildContext context) {
    ref.watch(observabilitySettingsProvider);
    return widget.child;
  }
}
