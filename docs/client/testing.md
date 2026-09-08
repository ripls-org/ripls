---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Client testing strategy — layered ViewModel/Repository/Service unit tests with Riverpod ProviderContainer and Mockito as the primary signal, plus the Playwright e2e suite covering the Flutter Web integration boundary.
  globs: [app/test/**, e2e/**]
  triggers: [testing, viewmodel-test, mockito, provider-container, playwright, e2e, mocks]
  lens: [client, testing]
  domain: client
freshness:
  verified_commit: "e5b434c33"
  verified_on: "2026-07-12"
---
# Client Testing

## Overview

The Ripls Flutter app uses a layered testing approach that mirrors the MVVM architecture. Tests are organized by layer (ViewModels, Repositories, Services) with each layer having specific testing strategies and tools.

## Testing Philosophy

**Test business logic, not Flutter widgets.** ViewModels contain all business logic and are pure Dart classes—easy to test without the Flutter framework. UI widgets should be thin and delegate all logic to ViewModels.

**Why:** Testing ViewModels is fast (no widget tree needed), reliable (no flaky widget tests), and provides better coverage of actual business logic. Widget tests are slower, more brittle, and often test framework behavior rather than app logic.

ViewModel tests remain the primary signal for business-logic correctness; the [e2e suite](#client-side-end-to-end-tests-e2e) covers the integration boundary between Flutter Web, the bundle bootstrap, and the wire format. The two layers are complementary, not substitutes.

## Test Structure

```
app/test/                         # Dart unit + widget tests (this doc)
├── data/
│   ├── cache/                    # Cache layer tests
│   └── repositories/             # Repository tests
│
├── presentation/
│   └── viewmodels/               # ViewModel tests (primary focus)
│
└── helpers/                      # Shared test utilities

e2e/                              # Top-level Playwright Test harness
├── tests/                        # *.spec.ts — one per scenario
├── lib/                          # Reusable seed + browser helpers
├── fixtures/                     # LFS-tracked binary fixtures (mp4, png)
└── gen/                          # Generated TS Connect clients
```

`e2e/` runs Playwright Test against the built Flutter Web bundle on
mobile-Chromium and WebKit. It exercises the integration boundary
(bundle bootstrap, Semantics-tree DOM serialization, Connect-Web
wire format) — not a replacement for ViewModel coverage of
business logic. See [§Client-side end-to-end tests (e2e)](#client-side-end-to-end-tests-e2e).

## Testing Layers

### 1. ViewModel Testing (Primary Focus)

ViewModels contain business logic and state management. These tests verify business rules, state transitions, error handling, and async flows.

**Tools:**
- `ProviderContainer` for Riverpod provider setup
- `Mockito` for mocking dependencies (repositories, services)
- `flutter_test` for test framework

**Example:**
```dart
@GenerateMocks([
  ChatService,
  UserRepository,
  LoanRepository,
])
void main() {
  late ProviderContainer container;
  late MockChatService mockChatService;
  late MockUserRepository mockUserRepository;

  setUp(() {
    mockChatService = MockChatService();
    mockUserRepository = MockUserRepository();

    container = ProviderContainer(
      overrides: [
        chatServiceProvider.overrideWithValue(mockChatService),
        userRepositoryProvider.overrideWithValue(mockUserRepository),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockChatService);
    reset(mockUserRepository);
  });

  test('loads conversations successfully', () async {
    final conversations = [
      ConversationItem(conversationId: 'conv-1', ...),
    ];

    when(mockChatService.listConversations())
        .thenAnswer((_) async => conversations);

    final notifier = container.read(inboxProvider.notifier);
    await notifier.initialize(currentUserId: 'user-123');

    final state = container.read(inboxProvider);
    expect(state.conversations, conversations);
    expect(state.isLoading, isFalse);
    expect(state.errorMessage, isNull);
  });
}
```

**Why:** ViewModels are pure Dart, so tests run fast without widget infrastructure. Mocking dependencies gives complete control over test scenarios including edge cases and errors.

### 2. Repository Testing

Repository tests verify caching behavior, data transformation, and interaction with services. Focus on cache hit/miss scenarios and invalidation logic.

**Tools:**
- Mock cache managers
- Mock services
- Verify cache interactions

**Example:**
```dart
test('getById returns cached user on cache hit', () async {
  final user = User(id: 'user123', name: 'Test User');

  when(mockCache.getAsync('user:user123'))
      .thenAnswer((_) async => user);

  final result = await repository.getById('user123');

  expect(result, user);
  verifyNever(mockUserService.getUser(any)); // Service not called
  verify(mockCache.getAsync('user:user123')).called(1);
});

test('getById fetches from service on cache miss', () async {
  final user = User(id: 'user123', name: 'Test User');

  when(mockCache.getAsync('user:user123'))
      .thenAnswer((_) async => null); // Cache miss
  when(mockUserService.getUser('user123'))
      .thenAnswer((_) async => user);

  final result = await repository.getById('user123');

  expect(result, user);
  verify(mockUserService.getUser('user123')).called(1);
  verify(mockCache.setAsync('user:user123', user)).called(1);
});
```

**Why:** Repository tests ensure caching works correctly, preventing unnecessary network calls and verifying data freshness logic.

### 3. Cache Testing

Cache tests verify memory/disk tier behavior, expiration policies, and eviction strategies. These are low-level tests for the caching infrastructure.

**Example:**
```dart
test('HybridCacheManager returns from memory on cache hit', () async {
  await cache.setAsync('key1', 'value1');

  final result = await cache.getAsync('key1');

  expect(result, 'value1');
  // Verify no disk access occurred
});

test('expired items return null', () async {
  await cache.setAsync('key1', 'value1', ttl: Duration(milliseconds: 10));

  await Future.delayed(Duration(milliseconds: 20));

  final result = await cache.getAsync('key1');
  expect(result, isNull);
});
```

**Why:** Cache tests ensure the foundation of the app's performance (multi-tier caching) works correctly under various scenarios.

## Testing Patterns

### Testing Async Flows

Always test the complete async flow including loading, success, and error states.

```dart
test('handles error loading data', () async {
  when(mockRepository.list())
      .thenThrow(Exception('Network error'));

  final notifier = container.read(myProvider.notifier);
  await notifier.loadData();

  final state = container.read(myProvider);
  expect(state.isLoading, isFalse);
  expect(state.hasError, isTrue);
  expect(state.errorMessage, contains('Network error'));
});
```

### Testing State Transitions

Verify state changes through the entire flow.

```dart
test('state transitions through loading to success', () async {
  when(mockRepository.list())
      .thenAnswer((_) async => [item1, item2]);

  final notifier = container.read(myProvider.notifier);

  // Initial state
  expect(container.read(myProvider).isLoading, isTrue);

  await notifier.loadData();

  // Final state
  final state = container.read(myProvider);
  expect(state.isLoading, isFalse);
  expect(state.items, [item1, item2]);
});
```

### Testing Stream Management

For ViewModels with streams, test subscription lifecycle and cleanup.

```dart
test('cancels stream subscriptions on dispose', () async {
  final streamController = StreamController<Message>();
  when(mockChatService.streamMessages(conversationId: any))
      .thenAnswer((_) => streamController.stream);

  final notifier = container.read(conversationProvider.notifier);
  await notifier.initialize(...);

  // Dispose triggers cleanup
  container.dispose();

  expect(streamController.hasListener, isFalse);
  await streamController.close();
});
```

### Testing Optimistic Updates

Verify optimistic UI updates with success and rollback scenarios.

```dart
test('adds message optimistically then confirms', () async {
  final sentMessage = Message(id: 'real-id', text: 'Hello', ...);

  when(mockChatService.sendMessage(any))
      .thenAnswer((_) async => sentMessage);

  final notifier = container.read(conversationProvider.notifier);
  await notifier.initialize(...);

  // Send message
  await notifier.sendMessage('Hello');

  final state = container.read(conversationProvider);

  // Message was added and confirmed (temp ID replaced with real ID)
  expect(state.messages.length, 1);
  expect(state.messages[0].id, 'real-id');
  expect(state.messages[0].state, MessageState.delivered);
});

test('marks message as failed on error', () async {
  when(mockChatService.sendMessage(any))
      .thenThrow(Exception('Send failed'));

  final notifier = container.read(conversationProvider.notifier);
  await notifier.initialize(...);

  await notifier.sendMessage('Hello');

  final state = container.read(conversationProvider);

  // Message still exists but marked as failed
  expect(state.messages.length, 1);
  expect(state.messages[0].state, MessageState.failed);
});
```

### Testing Timer Management

For ViewModels with debounce timers or timeouts, use `runDebounced` from
`app/test/helpers/fake_async_helpers.dart` to advance simulated time
deterministically. Avoid `await Future.delayed(...)` for debounce waits —
it races the wall clock and flakes under CI load.

```dart
import '../../helpers/fake_async_helpers.dart';

test('debounced search fires once after 300ms', () {
  runDebounced((async) {
    notifier.search(query: 'foo');
    async.elapse(const Duration(milliseconds: 300));
    async.flushMicrotasks();
    expect(state.results, isNotEmpty);
  });
});

test('message times out after 10 seconds', () {
  runDebounced((async) {
    final notifier = container.read(conversationProvider.notifier);

    // Mock send that never completes
    when(mockChatService.sendMessage(any))
        .thenAnswer((_) => Future.delayed(const Duration(seconds: 20)));

    notifier.sendMessage('Hello');

    // Advance simulated time — no wall-clock wait
    async.elapse(const Duration(seconds: 11));
    async.flushMicrotasks();

    final state = container.read(conversationProvider);
    expect(state.messages[0].state, MessageState.failed);
  });
});
```

## Mock Generation

Generate mocks using Mockito's code generation:

```dart
@GenerateMocks([
  ChatService,
  UserRepository,
  LoanRepository,
  GearRepository,
  MediaRepository,
])
void main() {
  // Tests...
}
```

Run code generation:
```bash
dart run build_runner build --delete-conflicting-outputs
```

**Why:** Generated mocks are type-safe and automatically updated when interfaces change, preventing runtime errors in tests.

**If build_runner loops forever on "updated builders"** (seen on
build_runner 2.15): the cached build graph is corrupt — `flutter clean`
in `app/` and re-run. Don't wait it out; it never converges.

## Testing Best Practices

### Do's

✅ **DO:**
- Test ViewModels, not widgets
- Use `ProviderContainer` for provider tests
- Mock all external dependencies (services, repositories)
- Test async flows completely (loading → success/error)
- Test error cases explicitly
- Use `setUp()` and `tearDown()` for test isolation
- Test edge cases (empty lists, null values, concurrent operations)
- Verify cache behavior in repository tests
- Test cleanup in `dispose()` methods

### Don'ts

❌ **DON'T:**
- Test UI and logic together (keep them separate)
- Use real services in unit tests (always mock)
- Skip error case testing
- Forget to dispose containers in `tearDown()`
- Test Flutter framework behavior (focus on your logic)
- Use synchronous test helpers for async code
- Share mutable state between tests
- Test implementation details (test behavior, not internals)

## Common Testing Pitfalls

### Pitfall 1: Not Disposing Containers

```dart
// ❌ WRONG: Memory leak
test('my test', () {
  final container = ProviderContainer(...);
  // No dispose() - container leaks
});

// ✅ CORRECT: Proper cleanup
test('my test', () {
  final container = ProviderContainer(...);
  addTearDown(() => container.dispose());
  // Or use tearDown() in setUp/tearDown pattern
});
```

### Pitfall 2: Not Resetting Mocks

```dart
// ❌ WRONG: Mocks carry state between tests
setUp(() {
  mockService = MockService();
});

// ✅ CORRECT: Reset mocks between tests
tearDown(() {
  reset(mockService);
  reset(mockRepository);
});
```

### Pitfall 3: Testing Implementation Instead of Behavior

```dart
// ❌ WRONG: Testing internal state
test('increments counter', () {
  notifier.counter++; // Directly modifying internal state
  expect(notifier.counter, 1);
});

// ✅ CORRECT: Testing through public API
test('increments counter', () {
  notifier.increment(); // Use public method
  expect(container.read(counterProvider), 1);
});
```

### Pitfall 4: Not Testing Async Completion

```dart
// ❌ WRONG: Test completes before async work finishes
test('loads data', () {
  notifier.loadData(); // Not awaited!
  expect(state.isLoading, isFalse); // Fails - still loading
});

// ✅ CORRECT: Await async operations
test('loads data', () async {
  await notifier.loadData();
  expect(container.read(myProvider).isLoading, isFalse);
});
```

## Running Tests

```bash
# Run all tests. Locally always use --reporter expanded: the default compact
# reporter redraws lines with \r, which duplicates output in captured logs
# (CI uses --reporter github).
flutter test --reporter expanded

# Run specific test file
flutter test test/presentation/viewmodels/gear_view_model_test.dart --reporter expanded

# Run tests with coverage
flutter test --coverage

# Run tests in watch mode
flutter test --watch
```

### If a test file "fails to load" with a segfault

A failure shaped like this is **not** a problem with the named test file:

```
❌ loading test/presentation/widgets/.../some_test.dart (failed)
Failed to load "...": Shell subprocess crashed with segmentation fault.
```

It can also surface as `Bad state: Cannot add event while adding stream.` — the
same crash, reported as a failed test instead of a dead device. Flutter 3.47.0's
Dart VM profiler crashes `flutter_tester` at roughly 0.2% of process starts, and
the file named is simply whichever tester was sampled at the wrong moment
(issue #2933). Don't bisect it. CI works around this by running
`scripts/disable_flutter_tester_profiler.sh` before the suite; run that script
locally if you hit it on 3.47.x.

## Test Coverage Goals

- **ViewModels:** 90%+ coverage (critical business logic)
- **Repositories:** 80%+ coverage (caching and data access)
- **Cache:** 80%+ coverage (infrastructure layer)
- **Services:** Mock in tests, don't test directly (external dependencies)

**Why:** High ViewModel coverage ensures business logic is correct. Repository coverage ensures caching works. We don't test services directly since they're external dependencies.

## Integration Testing

Cross-layer integration coverage runs at two levels:

- **Server-driven** (`server/e2e_test.go`,
  `server/simulation/`). Exercises the full RPC
  surface against a real database. Authoritative for product
  invariants — every workflow example in `docs/workflows/*.md` maps
  1:1 to an integration test under `server/integration_tests/` per
  [`docs/testing/update_system_tests.md`](../testing/update_system_tests.md).
- **Client-side via Playwright Test on the Flutter Web bundle**
  (`e2e/` — see [§Client-side end-to-end tests (e2e)](#client-side-end-to-end-tests-e2e)).
  Exercises the integration boundary the server-side tests can't
  see: bundle bootstrap, the Semantics-tree DOM serialization,
  Connect-Web wire format on the browser.

iOS + Android UI automation is **out of scope** post-Patrol-removal
(#1887). The cross-platform mobile matrix never produced a stable
story. The Phase-2 plan in #2162 brings multi-client interactive
scenarios to the web harness instead.

## Client-side end-to-end tests (e2e)

The `e2e/` top-level package is a Playwright Test harness against the
real Flutter Web bundle. It's the canonical client-side E2E path; it
replaced the prior chromedp smoke + `test_web_smoke.yaml` workflow in
the same PR that introduced it.

**What e2e tests cover:**

- Regression-catch for bugs whose failure path is in browser-only
  code (the motivating example is #2155 — a Web video playback
  regression no ViewModel unit test caught because the bug was in
  `createCachedVideoController`'s conditional-import branch).
- The integration boundary between the bundle, the Semantics tree
  serialization, and the Connect-Web wire format.
- Multi-engine fidelity: every scenario runs on Chromium-mobile and
  WebKit (Playwright's bundled WebKit is much closer to Safari than
  Chromium-with-Safari-UA).

**What e2e tests deliberately do NOT cover:**

- Business logic correctness. ViewModel tests + server integration
  tests own that. A regression in a Notifier's state machine should
  be caught by its ViewModel test, not by waiting for an e2e flake.
- iOS / Android native UI. See above — out of scope post-#1887.

**Layout, conventions, and the dev loop** live in
[`e2e/README.md`](../../e2e/README.md). Semantic identifiers used by
specs follow the convention in
[`docs/client/testing/semantics_identifiers.md`](testing/semantics_identifiers.md).

### Working against the bundle — the five traps

Every one of these has cost a real debugging session; internalize them
before driving the web bundle.

1. **The web bundle is embedded in the server binary.** `build:web:e2e`
   copies the built bundle into `server/services/web/app_assets/`, and the
   Go server embeds that directory **at build time**
   (`//go:embed app_assets`, `server/services/web/app_embed.go`). Rebuilding
   the bundle alone changes *nothing served* — the symptom is the app
   silently behaving like an older build. One command does both steps in the
   right order: **`npm run build:e2e`**. The e2e runners fail fast on a
   stale binary (`e2e/lib/server.ts` and `run_walkthrough.sh` compare
   `tmp/server`'s mtime against `app_assets/`), so if you see "OLDER than
   the web bundle", that guard just saved you an hour.
2. **`debugPrint` is dead in the wasm bundle.** Client-side prints don't
   reach anything you can read. For diagnostics, use the `Logger` package
   (its output lands in the browser console, readable via the Playwright
   trace), or assert state via a server RPC — the server's slog stream
   (`logPath` on the e2e server handle) sees every request.

   Walkthrough specs don't capture the console, so to read a `Logger` line
   during a render, tag it and add a temporary filter to the scene:

   ```ts
   page.on('console', (m) => {
     const t = m.text();
     if (t.includes('ZZPROBE')) console.log(`[ZZ] ${t}`);
   });
   ```

   Worth reaching for early on state-timing bugs. Twice now (#2724's impact
   drift, #2727's stale RSVP row) several confident readings of the code
   were wrong and one probe run was decisive — both turned out to be a
   *later* write from a *superseded* caller, which no amount of reading the
   happy path reveals.
3. **Locate via the accessibility tree, and don't nest interactive
   widgets.** Use `getByRole('button', { name: <l10n semanticsLabel> })` —
   raw child text and `semanticsIdentifier`s on primitives usually don't
   reach the DOM, and an interactive `Toggle`/`Tappable` nested inside
   another `Tappable`'s subtree is swallowed entirely by merged semantics
   (hoist per-row actions out as siblings of the tappable area). Full
   treatment: [`semantics_identifiers.md`](testing/semantics_identifiers.md).
4. **Renaming user copy silently rots the specs that locate by it — and
   negative assertions rot without going red.** A spec that waits for a
   renamed button times out and tells you. But
   `expect(getByRole('button', { name: 'Invite community' })).toHaveCount(0)`
   passes *forever* once that button is renamed, because a button that
   cannot be found is absent by definition. Renaming "Invite community" to
   "Share to communities" hit five spec sites this way (#2724): three failed
   honestly, two kept passing while proving nothing.

   After any user-copy change: grep `e2e/tests/` for the old string, **and**
   audit every `toHaveCount(0)` / `not.toBeVisible()` in the specs —
   confirm each name still exists in `app_en.arb` (or the Go templates, for
   `/go/` landing pages). The same trap voided the RSVP-CTA assertion in the
   events reel, where the accessible name is the a11y label, not the visible
   text. **A green negative assertion is only worth what it could fail on.**
5. **An orphan server on the port impersonates your build.** Startup waits
   for `/health`, and a server left behind by a killed run answers on the
   first poll — so it wins the race against the child that just failed to
   bind, startup reports "ready", and the whole run is served by whatever
   binary that orphan embeds. Trap 1's freshness guard cannot see it: the
   stale binary is never executed, and the one on disk is genuinely fresh.

   The symptom is trap 1's symptom exactly — edits and rebuilds have no
   effect — which is what makes it expensive. In #2764 it produced three
   independent "the fix isn't applying" readings (a magenta background, magenta
   button styles, a print marker) against a fix that was already correct and
   already committed. `assertPortFree` in `e2e/lib/server.ts` now refuses to
   start and names the occupying pid.

   When a change appears not to take, **verify what is actually being served
   before doubting the code**: compare the served asset against the built one,
   rather than adding another probe.

   ```bash
   curl -s localhost:8100/main.dart.wasm -o /tmp/served.wasm
   cmp /tmp/served.wasm server/services/web/app_assets/main.dart.wasm \
     && echo "serving the build you think it is"
   ```

   (Note `e2e/tmp/app_assets/` does not exist — the embed directory is
   `server/services/web/app_assets/`.)

## Testing Strengths

### ✅ Strengths

1. **Fast Execution:** ViewModel tests run in milliseconds without Flutter widget infrastructure
2. **Reliable:** No flaky widget tests or timing issues from UI animations
3. **Easy Setup:** `ProviderContainer` provides simple test setup without complex widget trees
4. **Complete Control:** Mocking gives full control over dependencies, edge cases, and error scenarios
5. **Type Safety:** Mockito generates type-safe mocks that catch errors at compile time
6. **Isolated Testing:** Each layer can be tested independently without external dependencies
7. **Clear Patterns:** Established patterns for async flows, streams, and optimistic updates
8. **Good Coverage:** High coverage of business logic without testing framework internals

## Testing Weaknesses

### ⚠️ Weaknesses

1. **Mock Setup Overhead:** Tests require significant setup with provider containers and mock configuration
2. **Code Generation Required:** Mockito requires build_runner, adding build time and complexity
3. **Limited UI Testing:** Current approach doesn't verify actual UI rendering or user interactions
4. **Mobile-native E2E uncovered:** Client-side E2E lives in the `e2e/` Playwright harness against the Flutter Web bundle (#2162). iOS / Android native UI automation remains out of scope post-Patrol-removal (#1887).
5. **Manual Mock Updates:** When interfaces change, mocks need manual regeneration with build_runner
6. **Integration Gaps:** Limited testing of interactions between multiple ViewModels
7. **No Golden Tests:** Missing visual regression testing for UI consistency
8. **Async Complexity:** Testing timers and complex stream scenarios can be challenging
9. **Cache Testing Gaps:** Limited testing of cache invalidation and staleness scenarios
10. **No Performance Tests:** Missing benchmarks for critical operations

## Next Steps

### Recommended Improvements

1. **Coverage Improvements**
   - [test_coverage_improvement_plan.md](../docs/ai/test_coverage_improvement_plan.md)
   - Add tests for error recovery and retry logic
   - Test concurrent operations and race conditions
   - Add cache invalidation test scenarios
   - Test offline behavior and reconnection logic

2. **Integration Testing**
   - Add integration tests for critical user flows (login, loan creation, messaging)
   - Test navigation flows between screens
   - Verify ViewModel interactions in multi-screen workflows
   - Test error recovery scenarios across layers

3. **Visual Testing**
   - Add golden tests for key UI components
   - Implement visual regression testing
   - Test dark mode and theme variations
   - Verify responsive layouts on different screen sizes

4. **E2E Testing**
   - Server-side: `server/e2e_test.go` and the `server/simulation/`
     scenario harness — the source of truth for product workflow
     invariants.
   - Client-side: the Playwright harness at `e2e/` (see
     [§Client-side end-to-end tests (e2e)](#client-side-end-to-end-tests-e2e)).
     Drives the real Flutter Web bundle on mobile-Chromium and WebKit.
   - iOS / Android UI automation remains out of scope post-#1887.

5. **Test Utilities**
   - Create test helpers for common ViewModel setups
   - Build fixture factories for test data generation
   - Add utility functions for common assertions
   - Create reusable mock configurations

6. **Performance Testing**
   - Add benchmarks for cache performance
   - Test large list rendering and scrolling
   - Measure repository response times
   - Profile memory usage in long-running scenarios

7. **Test Documentation**
   - Add inline documentation for complex test scenarios
   - Create testing recipes for common patterns
   - Document testing anti-patterns to avoid
   - Build onboarding guide for new developers

8. **CI/CD Integration**
   - Add automated test runs on pull requests
   - Set up coverage reporting and tracking
   - Configure test result visualization
   - Add performance regression detection

## Resources

### Testing Documentation
- **Flutter Testing:** https://docs.flutter.dev/testing
- **Riverpod Testing:** https://riverpod.dev/docs/essentials/testing
- **Mockito:** https://pub.dev/packages/mockito

### Internal Examples
- [conversation_view_model_test.dart](../app/test/presentation/viewmodels/conversation_view_model_test.dart) - Complex ViewModel with streams
- [conversation_view_model_test.dart](../app/test/presentation/viewmodels/conversation_view_model_test.dart) - Optimistic updates and timers
- [user_repository_test.dart](../app/test/data/repositories/user_repository_test.dart) - Repository caching patterns

---

**Last Updated:** Phase 5 MVVM Migration Complete
**Architecture Version:** 2.0 (Riverpod 3.0 + Async-First)
