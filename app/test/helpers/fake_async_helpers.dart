import 'package:fake_async/fake_async.dart';

/// Runs [body] inside a FakeAsync zone, advancing simulated time and
/// flushing microtasks so debounce timers + their continuations resolve
/// deterministically.
///
/// Use this wherever a test would otherwise need `await Future.delayed(...)` to
/// wait for a debounce window to pass. Inside [body], replace each
/// `await Future.delayed(Duration(milliseconds: N))` with
/// `async.elapse(Duration(milliseconds: N)); async.flushMicrotasks();`.
/// Replace `await Future.microtask(() {})` with `async.flushMicrotasks()`.
///
/// Example:
/// ```dart
/// test('debounced search fires once', () {
///   runDebounced((async) {
///     notifier.search(query: 'foo');
///     async.elapse(const Duration(milliseconds: 300));
///     async.flushMicrotasks();
///     expect(state.results, isNotEmpty);
///   });
/// });
/// ```
void runDebounced(void Function(FakeAsync async) body) {
  fakeAsync((async) {
    body(async);
    async.flushTimers();
  });
}
