import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/viewmodels/splash_view_model.dart';

void main() {
  late ProviderContainer container;

  setUp(() {
    container = ProviderContainer();
  });

  tearDown(() {
    container.dispose();
  });

  group('initial state', () {
    test('starts in initializing step', () {
      final state = container.read(splashProvider);
      expect(state.step, SplashLoadingStep.initializing);
      expect(state.statusMessage, 'Initializing...');
      expect(state.hasError, isFalse);
      expect(state.error, isNull);
    });
  });

  group('updateStep', () {
    test('checkingAuth sets correct status message', () {
      final notifier = container.read(splashProvider.notifier);
      notifier.updateStep(SplashLoadingStep.checkingAuth);

      final state = container.read(splashProvider);
      expect(state.step, SplashLoadingStep.checkingAuth);
      expect(state.statusMessage, 'Checking authentication...');
      expect(state.hasError, isFalse);
    });

    test('loadingCommunities sets correct status message', () {
      final notifier = container.read(splashProvider.notifier);
      notifier.updateStep(SplashLoadingStep.loadingCommunities);

      final state = container.read(splashProvider);
      expect(state.step, SplashLoadingStep.loadingCommunities);
      expect(state.statusMessage, 'Loading communities...');
    });

    test('loadingFeed sets correct status message', () {
      final notifier = container.read(splashProvider.notifier);
      notifier.updateStep(SplashLoadingStep.loadingFeed);

      final state = container.read(splashProvider);
      expect(state.step, SplashLoadingStep.loadingFeed);
      expect(state.statusMessage, 'Loading feed...');
    });

    test('ready sets correct status message', () {
      final notifier = container.read(splashProvider.notifier);
      notifier.updateStep(SplashLoadingStep.ready);

      final state = container.read(splashProvider);
      expect(state.step, SplashLoadingStep.ready);
      expect(state.statusMessage, 'Ready');
    });

    test('clears error on step update', () {
      final notifier = container.read(splashProvider.notifier);
      notifier.setError('Something went wrong');
      notifier.updateStep(SplashLoadingStep.checkingAuth);

      final state = container.read(splashProvider);
      expect(state.hasError, isFalse);
      expect(state.error, isNull);
    });
  });

  group('setError', () {
    test('sets error state with message', () {
      final notifier = container.read(splashProvider.notifier);
      notifier.setError('Auth failed');

      final state = container.read(splashProvider);
      expect(state.step, SplashLoadingStep.error);
      expect(state.hasError, isTrue);
      expect(state.error, isNotNull);
      expect(state.statusMessage, 'Failed to load');
    });
  });

  group('clearError', () {
    test('resets to initial state', () {
      final notifier = container.read(splashProvider.notifier);
      notifier.setError('Some error');
      notifier.clearError();

      final state = container.read(splashProvider);
      expect(state.step, SplashLoadingStep.initializing);
      expect(state.hasError, isFalse);
      expect(state.error, isNull);
    });
  });

  group('routing decision matrix', () {
    test('step sequence: initializing → checkingAuth → ready', () {
      final notifier = container.read(splashProvider.notifier);

      notifier.updateStep(SplashLoadingStep.initializing);
      expect(container.read(splashProvider).step,
          SplashLoadingStep.initializing);

      notifier.updateStep(SplashLoadingStep.checkingAuth);
      expect(container.read(splashProvider).step,
          SplashLoadingStep.checkingAuth);

      notifier.updateStep(SplashLoadingStep.ready);
      expect(container.read(splashProvider).step, SplashLoadingStep.ready);
    });

    test('error mid-flow transitions to error state', () {
      final notifier = container.read(splashProvider.notifier);
      notifier.updateStep(SplashLoadingStep.loadingCommunities);
      notifier.setError('Failed to load communities');

      final state = container.read(splashProvider);
      expect(state.step, SplashLoadingStep.error);
      expect(state.error, isNotNull);
    });
  });
}
