# Viewmodel tests

Unit tests for `app/lib/presentation/viewmodels/`. Each test file mirrors the
corresponding source file and uses a `ProviderContainer` with mock overrides —
no Flutter widget tree required.

## Debounce-driven tests

ViewModels that debounce user input (search, inbox search, event/fulfill
previews) must **not** use `await Future.delayed(...)` to bridge the debounce
window. Wall-clock waits race the scheduler and flake under CI load.

Use `runDebounced` from `app/test/helpers/fake_async_helpers.dart` instead:

```dart
import '../../helpers/fake_async_helpers.dart';

test('search fires after debounce', () {
  runDebounced((async) {
    notifier.search(query: 'foo');
    async.elapse(const Duration(milliseconds: 300)); // debounce duration
    async.flushMicrotasks();                          // let RPCs complete
    expect(state.results, isNotEmpty);
  });
});
```

Rules:
- Replace each `await Future.delayed(Duration(milliseconds: N))` with
  `async.elapse(Duration(milliseconds: N)); async.flushMicrotasks();`
- Replace `await Future.microtask(() {})` with `async.flushMicrotasks()`
- Replace `await container.read(provider.future)` with
  `container.read(provider.future); async.flushMicrotasks();`
- Fire-and-forget async notifier methods (`notifier.refresh()`) then call
  `async.flushMicrotasks()` to let them complete

## Mock generation

Run `flutter pub run build_runner build` after changing `@GenerateMocks`
annotations to regenerate the `.mocks.dart` files.
