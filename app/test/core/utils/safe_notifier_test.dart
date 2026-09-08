import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/safe_notifier.dart';

/// Simple immutable test state (no freezed needed for tests)
class TestState {
  final int count;
  final bool isLoading;
  final String? data;

  const TestState({
    this.count = 0,
    this.isLoading = false,
    this.data,
  });

  TestState copyWith({
    int? count,
    bool? isLoading,
    String? data,
  }) {
    return TestState(
      count: count ?? this.count,
      isLoading: isLoading ?? this.isLoading,
      data: data ?? this.data,
    );
  }
}

/// Test notifier using SafeNotifierMixin
class TestNotifier extends Notifier<TestState>
    with SafeNotifierMixin<TestState> {
  @override
  TestState build() => const TestState();

  void increment() {
    safeSetState(state.copyWith(count: state.count + 1));
  }

  void setLoading(bool loading) {
    safeSetState(state.copyWith(isLoading: loading));
  }

  bool incrementWithResult() {
    return safeUpdateState(
      (current) => current.copyWith(count: current.count + 1),
    );
  }

  /// Simulates an async operation that completes after disposal.
  /// This is the real use case for SafeNotifierMixin.
  /// Uses safeUpdateState (not safeSetState) because it defers state access.
  Future<void> loadDataAsync(Completer<String> dataCompleter) async {
    safeUpdateState((s) => s.copyWith(isLoading: true));

    // Wait for external data (simulating API call)
    final data = await dataCompleter.future;

    // This is where the mixin protects us - after an async gap
    // Using safeUpdateState with lambda defers state access until after guard
    safeUpdateState((s) => s.copyWith(data: data, isLoading: false));
  }

  /// Same as above but returns whether the final state was set
  Future<bool> loadDataAsyncWithResult(Completer<String> dataCompleter) async {
    safeUpdateState((s) => s.copyWith(isLoading: true));

    final data = await dataCompleter.future;

    // Returns false if disposed during the await
    return safeUpdateState((s) => s.copyWith(data: data, isLoading: false));
  }
}

final testProvider =
    NotifierProvider<TestNotifier, TestState>(TestNotifier.new);

/// Test with autoDispose provider (uses same Notifier class)
final autoDisposeTestProvider =
    NotifierProvider.autoDispose<TestNotifier, TestState>(TestNotifier.new);

void main() {
  group('SafeNotifierMixin', () {
    late ProviderContainer container;

    setUp(() {
      container = ProviderContainer();
    });

    tearDown(() {
      container.dispose();
    });

    test('safeSetState updates state when mounted', () {
      final notifier = container.read(testProvider.notifier);

      notifier.increment();

      expect(container.read(testProvider).count, 1);
    });

    test('safeSetState can be called multiple times when mounted', () {
      final notifier = container.read(testProvider.notifier);

      notifier.increment();
      notifier.increment();
      notifier.increment();

      expect(container.read(testProvider).count, 3);
    });

    test('safeUpdateState returns true when mounted', () {
      final notifier = container.read(testProvider.notifier);

      final result = notifier.incrementWithResult();

      expect(result, isTrue);
      expect(container.read(testProvider).count, 1);
    });

    test('safeSetState updates different state fields', () {
      final notifier = container.read(testProvider.notifier);

      notifier.setLoading(true);

      expect(container.read(testProvider).isLoading, isTrue);
      expect(container.read(testProvider).count, 0);
    });
  });

  group('SafeNotifierMixin with autoDispose provider', () {
    test('safeSetState updates state when mounted', () {
      final container = ProviderContainer();
      addTearDown(container.dispose);

      final notifier = container.read(autoDisposeTestProvider.notifier);

      notifier.increment();

      expect(container.read(autoDisposeTestProvider).count, 1);
    });

    test('safeUpdateState returns true when mounted', () {
      final container = ProviderContainer();
      addTearDown(container.dispose);

      final notifier = container.read(autoDisposeTestProvider.notifier);

      final result = notifier.incrementWithResult();

      expect(result, isTrue);
      expect(container.read(autoDisposeTestProvider).count, 1);
    });
  });

  group('async disposal safety (real use case)', () {
    test('async operation completes without throwing when disposed mid-await',
        () async {
      final container = ProviderContainer();
      final notifier = container.read(testProvider.notifier);
      final dataCompleter = Completer<String>();

      // Start async operation
      final future = notifier.loadDataAsync(dataCompleter);

      // Verify loading state was set
      expect(container.read(testProvider).isLoading, isTrue);

      // Dispose container while async operation is pending
      container.dispose();

      // Complete the data (simulating API response arriving after disposal)
      dataCompleter.complete('test data');

      // The future should complete without throwing
      await expectLater(future, completes);
    });

    test('safeSetState returns false when async callback runs after disposal',
        () async {
      final container = ProviderContainer();
      final notifier = container.read(testProvider.notifier);
      final dataCompleter = Completer<String>();

      // Start async operation
      final resultFuture = notifier.loadDataAsyncWithResult(dataCompleter);

      // Dispose container while async operation is pending
      container.dispose();

      // Complete the data
      dataCompleter.complete('test data');

      // The final safeSetState should return false (state not updated)
      final result = await resultFuture;
      expect(result, isFalse);
    });

    test('autoDispose provider handles async disposal gracefully', () async {
      final container = ProviderContainer();
      final notifier = container.read(autoDisposeTestProvider.notifier);
      final dataCompleter = Completer<String>();

      // Start async operation
      final future = notifier.loadDataAsync(dataCompleter);

      // Verify loading state was set
      expect(container.read(autoDisposeTestProvider).isLoading, isTrue);

      // Dispose container
      container.dispose();

      // Complete the data
      dataCompleter.complete('test data');

      // Should complete without throwing
      await expectLater(future, completes);
    });

    test('state is updated correctly when async completes while mounted',
        () async {
      final container = ProviderContainer();
      addTearDown(container.dispose);

      final notifier = container.read(testProvider.notifier);
      final dataCompleter = Completer<String>();

      // Start async operation
      final future = notifier.loadDataAsync(dataCompleter);

      // Verify loading state
      expect(container.read(testProvider).isLoading, isTrue);

      // Complete the data while still mounted
      dataCompleter.complete('test data');
      await future;

      // State should be updated
      expect(container.read(testProvider).isLoading, isFalse);
      expect(container.read(testProvider).data, 'test data');
    });
  });
}
