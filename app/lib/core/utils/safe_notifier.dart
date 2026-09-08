import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';

final _log = Logger('SafeNotifierMixin');

/// A mixin that provides safe state updates for Riverpod Notifiers.
///
/// When using `autoDispose` providers, the notifier can be disposed while async
/// operations are in flight. This mixin provides `safeSetState` and
/// `safeUpdateState` methods that check `ref.mounted` before updating state,
/// logging when updates are skipped.
///
/// **Note:** This is a transitional tool. New ViewModels should use
/// `AsyncNotifier` or `FutureProvider` instead, which handle lifecycle
/// automatically. See docs/client/architecture.md for details.
///
/// **Important:** Use `safeUpdateState` (not `safeSetState`) after async gaps
/// because it defers accessing `state` until after the mounted check:
///
/// ```dart
/// class MyNotifier extends Notifier<MyState> with SafeNotifierMixin<MyState> {
///   Future<void> loadData() async {
///     safeUpdateState((s) => s.copyWith(isLoading: true));
///
///     final results = await repository.getData();
///
///     // Safe: state is only accessed if still mounted
///     safeUpdateState((s) => s.copyWith(data: results, isLoading: false));
///   }
/// }
/// ```
///
/// Works with both regular `NotifierProvider` and `NotifierProvider.autoDispose`.
mixin SafeNotifierMixin<T> on Notifier<T> {
  /// Sets state to a specific value, only if the notifier is still mounted.
  ///
  /// **Warning:** Do NOT use after async gaps like this:
  /// ```dart
  /// safeSetState(state.copyWith(...));  // BAD: state accessed before guard
  /// ```
  ///
  /// Instead use `safeUpdateState` which defers state access:
  /// ```dart
  /// safeUpdateState((s) => s.copyWith(...));  // GOOD: state accessed inside guard
  /// ```
  ///
  /// Returns `true` if state was set, `false` if skipped due to disposal.
  bool safeSetState(T newState) {
    try {
      if (!ref.mounted) {
        _log.warning(
          'State update skipped - notifier disposed: ${runtimeType.toString()}',
        );
        return false;
      }
      state = newState;
      return true;
    } catch (e) {
      // In Riverpod 3, accessing ref after full disposal throws
      _log.warning(
        'State update skipped - notifier disposed: ${runtimeType.toString()}',
      );
      return false;
    }
  }

  /// Updates state using a function, only if mounted.
  ///
  /// **This is the recommended method for use after async operations** because
  /// it defers accessing `state` until after the mounted check.
  ///
  /// Returns `true` if state was updated, `false` if skipped due to disposal.
  ///
  /// Example:
  /// ```dart
  /// Future<void> loadData() async {
  ///   safeUpdateState((s) => s.copyWith(isLoading: true));
  ///   final data = await repository.getData();
  ///   safeUpdateState((s) => s.copyWith(data: data, isLoading: false));
  /// }
  /// ```
  bool safeUpdateState(T Function(T current) update) {
    try {
      if (!ref.mounted) {
        _log.warning(
          'State update skipped - notifier disposed: ${runtimeType.toString()}',
        );
        return false;
      }
      state = update(state);
      return true;
    } catch (e) {
      // In Riverpod 3, accessing ref/state after full disposal throws
      _log.warning(
        'State update skipped - notifier disposed: ${runtimeType.toString()}',
      );
      return false;
    }
  }
}
