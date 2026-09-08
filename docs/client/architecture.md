---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Flutter client architecture — MVVM + Riverpod layers, repository pattern, async-first immutable state, do's and don'ts.
  globs: [app/lib/**]
  lens: [architecture, client]
  alwaysApply: true
  domain: client
freshness:
  verified_commit: "6ca26eeaf"
  verified_on: "2026-06-12"
---
# Client Architecture

## Overview

The Ripls Flutter app follows an **[MVVM (Model-View-ViewModel)](https://en.wikipedia.org/wiki/Model%E2%80%93view%E2%80%93viewmodel)** architecture with **[Riverpod 3.0](https://riverpod.dev/)** for state management. The architecture prioritizes async-first patterns, immutable state, and clear separation of concerns. This follows **[Flutter best practices.](https://docs.flutter.dev/app-architecture/guide)** 

## Architecture Layers

```
┌─────────────────────────────────────────────────────────┐
│                   Presentation Layer                    │
│                                                         │
│                   ┌────────────────┐                    │
│                   │    Screens     │                    │
│                   │   (Widgets)    │                    │
│                   └───────┬────────┘                    │
│                           │ watch / read                │
│                           ↓                             │
│                   ┌────────────────---┐                 │
│                   │  ViewModels       │                 │
│                   │ (Business Logic)  │                 │
│                   └───────┬──────---──┘                 │
│                           │                             │
└───────────────────────────┼─────────────────────────────┘
                            │
┌───────────────────────────┼─────────────────────────────┐
│                           ↓         Data Layer          │
│                   ┌────────────────┐                    │
│                   │  Repositories  │                    │
│                   └───────┬────────┘                    │
│                           │ uses                        │
│                           ↓                             │
│                   ┌────────────────┐                    │
│                   │    Services    │                    │
│                   │  (API Clients) │                    │
│                   └───────┬────────┘                    │
└───────────────────────────┼─────────────────────────────┘
                            │
                            ↓
┌─────────────────────────────────────────────────────────┐
│                  Protocol Buffers                       │
│              (Type-safe data models)                    │
└─────────────────────────────────────────────────────────┘
```
**Key Architectural Rules:**

1. **ViewModels are the ONLY layer that calls Repositories**
   - Screens/Widgets never access repositories directly
   - Services never access repositories directly
   - All data flows through ViewModels

2. **Repositories are the ONLY layer that calls Services**
   - ViewModels never access services directly
   - Repositories handle caching and data access logic

3. **Screens only interact with ViewModels**
   - via `ref.watch()` for reactive state updates
   - via `ref.read().notifier` for triggering actions

## Directory Structure

```
lib/
├── core/              # Cross-cutting concerns
│   ├── config/        # App configuration
│   ├── theme/         # Theme definitions
│   ├── router/        # Navigation & routing
│   └── utils/         # Utility classes (NOT controllers)
│
├── data/              # Data layer
│   ├── gen/           # Generated code from protos
│   ├── cache/         # Caching infrastructure
│   └── repositories/  # Data access with caching
│
├── services/          # API clients & external services
│
├── presentation/      # UI layer
│   ├── screens/       # Full page widgets
│   ├── widgets/       # Reusable components
│   ├── viewmodels/    # Riverpod Notifiers (business logic)
│   ├── models/        # UI-specific models
│   ├── mixins/        # Reusable behaviors
│   └── utils/         # UI-specific utilities
│
└── main.dart          # Entry point
```

## Core Principles

### 1. Async-First Architecture

**All data access is async.** There are no synchronous repository methods.

```dart
// ✅ CORRECT: Always use async
final user = await userRepository.getById(userId);

// ❌ WRONG: No sync methods exist
final user = userRepository.getByIdSync(userId);
```

**Why:** Async-first eliminates sync/async confusion and prevents UI blocking. Flutter's [FutureBuilder](https://api.flutter.dev/flutter/widgets/FutureBuilder-class.html) and [AsyncValue](https://riverpod.dev/docs/concepts/async_value) handle loading states naturally. Even cache hits use async APIs for consistency, making the codebase predictable and preventing accidental main-thread blocking.

### 2. Riverpod 3.0 Notifier Pattern

**[Riverpod](https://riverpod.dev/)** is a type-safe state management library that eliminates runtime errors from provider access.

**[Freezed](https://pub.dev/packages/freezed)** is a code generator that creates immutable state classes—once created, state objects can't be modified directly. Instead, you create new versions with `copyWith()`, which copies the object with only the fields you specify changed.

All business logic lives in **Notifiers** with **Freezed state**.

**What you write:**

```dart
// 1. State class with @freezed annotation (you write this)
@freezed
class MyState with _$MyState {  // _$MyState connects to generated code
  const factory MyState({
    @Default([]) List<Item> items,
    @Default(false) bool isLoading,
    String? errorMessage,
  }) = _MyState;
}

// 2. Notifier with business logic (you write this)
class MyNotifier extends Notifier<MyState> {
  @override
  MyState build() => const MyState();

  Future<void> loadItems() async {
    state = state.copyWith(isLoading: true);  // copyWith is generated
    try {
      final items = await repository.list();
      state = state.copyWith(items: items, isLoading: false);
    } catch (e) {
      state = state.copyWith(errorMessage: e.toString(), isLoading: false);
    }
  }
}

// 3. Provider (you write this)
final myProvider = NotifierProvider<MyNotifier, MyState>(
  MyNotifier.new,
);
```

**What Freezed generates** (in `my_state.freezed.dart`):
- `copyWith()` method for creating modified copies
- `==` operator and `hashCode` for value equality
- `toString()` for debugging
- Immutable implementation class

**Key characteristics:**
- Immutable state via Freezed
- State updates through `copyWith()`
- `NotifierProvider` for simple notifiers
- `NotifierProvider.autoDispose` for screen-scoped state (see [Async Safety](#async-safety-in-notifiers) - requires `ref.mounted` checks after async operations)
- `StreamNotifier` for reactive streams

**Why:** Riverpod provides compile-time safety (no context needed, no provider not found errors), testability (no widget tree required), and flexibility (easy dependency injection). Freezed eliminates boilerplate for immutable state classes and ensures state changes are explicit via `copyWith()`, making state mutations predictable and debuggable.

### 3. Providers

**[Providers](https://riverpod.dev/docs/concepts/providers)** are the foundation of Riverpod's state management system. A provider is a container that holds a value (state, service, repository, etc.) and makes it accessible throughout your app without needing to pass it through constructors.

```dart
// providers/gear_provider.dart
// Providers are top-level variables, accessible via import
final gearProvider = NotifierProvider<GearNotifier, GearState>(
  GearNotifier.new,
);
```

```dart
// screens/my_screen.dart
import 'package:app/providers/gear_provider.dart';  // Import to access provider

class MyWidget extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // Access gearProvider without passing it through constructors
    final gearState = ref.watch(gearProvider);
    final notifier = ref.read(gearProvider.notifier);

    return Text('Items: ${gearState.items.length}');
  }
}
```

**How it works:** Providers are top-level variables that you import—no registration or lookup by name needed. The `ProviderScope` widget at your app's root enables `ref` to access any imported provider.

**Types of providers in this codebase:**

- **[Provider](https://riverpod.dev/docs/providers/provider)**: Holds immutable values (repositories, services, config)
- **[NotifierProvider](https://riverpod.dev/docs/providers/notifier_provider)**: Holds mutable state managed by a Notifier
- **[NotifierProvider.autoDispose](https://riverpod.dev/docs/concepts/modifiers/auto_dispose)**: Same as NotifierProvider but cleans up when no longer used
- **[StreamNotifierProvider](https://riverpod.dev/docs/providers/stream_notifier_provider)**: For reactive stream-based state

**Why:** Providers enable dependency injection, compile-time safety, automatic reactivity, and easy testability. They support lifecycle management (`autoDispose`), lazy initialization, and global access without context.

### 4. Repository Pattern with Stash-Based Caching

Repositories provide async data access with automatic caching using **[Stash](https://pub.dev/packages/stash)**.

```dart
class UserRepository {
  final CacheService _cache;
  final UserService _service;

  UserRepository(CacheManager cacheManager, this._service)
      : _cache = CacheService(cacheManager, 'user');

  Future<GetUserResponse> get(String userId) {
    return _cache.get(
      key: userId,
      fetch: () => _service.getUser(userId),
    );
  }
}
```

**Current implementation:**
- Memory-only storage using Stash
- LRU eviction (1000 max entries)
- Global TTL configured via environment (`CACHE_TTL_MINUTES`, default: 30 minutes)
- TTL refreshes on access (AccessedExpiryPolicy)
- Async-first APIs (no sync cache access)
- Namespace isolation per repository

**Why:** Repositories centralize data access and eliminate duplicate caching code in ViewModels. Stash provides proven caching internals with a clean API. Transparent caching makes the app feel instant while ensuring data freshness. The global TTL simplifies configuration and is sufficient for most use cases.

**See also:** [caching.md](caching.md) for detailed caching architecture, patterns, and guidelines.

### 5. No Controllers

**There are no "controllers" in the presentation layer.** Use the correct pattern:

- **ViewModels ([Notifiers](https://riverpod.dev/docs/providers/notifier_provider)):** Business logic, state management
- **Helpers/Utils:** Pure utility functions (e.g., `DiscoverMapHelper` for [Mapbox](https://www.mapbox.com/) lifecycle)

```dart
// ✅ CORRECT: ViewModel for business logic
class GearNotifier extends Notifier<GearState> { ... }

// ✅ CORRECT: Helper for SDK lifecycle
class DiscoverMapHelper {
  void dispose() { ... }
}

// ❌ WRONG: Don't create "controllers"
class GearController extends ChangeNotifier { ... }
```

**Why:** The term "controller" is ambiguous and often leads to mixing concerns. Riverpod Notifiers provide clear semantics for state management, while utility classes/helpers handle non-state concerns like SDK lifecycle management. This distinction prevents bloated classes and makes responsibilities explicit.

## Do's and Don'ts

### State Management

✅ **DO:**
- Use Notifiers for all business logic
- Use Freezed for immutable state
- Use `copyWith()` for state updates
- Use `autoDispose` for screen-scoped providers
- Use [`ref.watch()`](https://riverpod.dev/docs/concepts/reading#using-refwatch-to-observe-a-provider) in build methods
- Use [`ref.read()`](https://riverpod.dev/docs/concepts/reading#using-refread-to-obtain-the-state-of-a-provider-once) in event handlers

❌ **DON'T:**
- Use [ChangeNotifier](https://api.flutter.dev/flutter/foundation/ChangeNotifier-class.html) or [StatefulWidget](https://api.flutter.dev/flutter/widgets/StatefulWidget-class.html) for business logic
- Manually manage in-memory caches in ViewModels
- Use `ref.watch()` in event handlers
- Use `ref.read()` in build methods

### Data Access

✅ **DO:**
- Always use async methods for data access
- Use repositories for all data access
- Let repositories handle caching automatically
- Call repository mutation methods (which handle invalidation)

❌ **DON'T:**
- Access services directly from ViewModels
- Create manual cache maps in ViewModels
- Skip repository layer

**See:** [caching.md](caching.md) for caching patterns and best practices

### Colors and Theming

**All colors must be theme-aware.** The app supports light and dark modes; hardcoded
colors break the dark theme.

✅ **DO:**
- Use `AppColors.xxx(context)` helpers for all UI colors (backgrounds, text, borders,
  dividers, surfaces, card backgrounds, etc.)
- Use `Theme.of(context).textTheme` or `Theme.of(context).colorScheme` for standard
  Material colors
- Use `AppColors` theme-aware status getters (`statusSuccess(context)`, `statusWarning(context)`, `statusError(context)`, `statusInfo(context)`) for status indicators. The legacy shared consts (`success`/`warning`/`error`/`info`) hold fixed dark-theme values that under-deliver on light surfaces — call-site migration is tracked in #2445.

❌ **DON'T:**
- Use `Colors.white` for backgrounds or containers — it is invisible in light mode and
  blindingly bright in dark mode. Use `AppColors.cardBackground(context)` or
  `AppColors.surface(context)` instead.
- Use hardcoded `Colors.grey[50]`, `Colors.grey[100]`, `Colors.grey[200]`,
  `Colors.grey[300]` for backgrounds or borders — these are light-only colors.
  Use `AppColors.surface(context)` or `AppColors.border(context)` instead.
- Use hardcoded hex `Color(0xFF...)` values for UI elements — add a named helper to
  `AppColors` if a new semantic color is needed.

**Quick reference:**

| Need | Use |
|------|-----|
| Page / scaffold background | `AppColors.background(context)` |
| Card / modal background | `AppColors.cardBackground(context)` |
| Surface (input, chip) | `AppColors.surface(context)` |
| Border / divider | `AppColors.border(context)` / `AppColors.divider(context)` |
| Primary text | `AppColors.textPrimary(context)` |
| Secondary text | `AppColors.textSecondary(context)` |
| App bar background | `AppColors.appBarBackground(context)` |

### UI Structure

✅ **DO:**
- Break down large `build()` methods into private methods
- Extract reusable widgets into separate classes
- Use builder methods for complex widget trees
- Keep screen widgets focused on UI composition

❌ **DON'T:**
- Put business logic in widgets
- Create deeply nested widget trees
- Fetch data directly in widgets
- Use `setState()` for app state (only for local UI state like text field focus)

### Navigation & Gestures

#### Pushing screens

All in-app forward navigation must use `NavigationHelpers` — never raw `Navigator.push(MaterialPageRoute(...))` or GoRouter's `context.push()` for navigating to detail screens. `MaterialPageRoute` animates both the incoming and outgoing screens simultaneously, causing the HomeScreen to slide left and briefly flashing a blank background. `NavigationHelpers` keeps the previous screen still and only moves the new one.

| Helper | When to use |
|--------|-------------|
| `NavigationHelpers.pushScreen()` | Primary content screens (gear, request, experience, conversation, user, community). The destination screen must use `SwipeToCloseMixin` to provide the slide animation. |
| `NavigationHelpers.pushWithSlide()` | Utility/settings screens that don't need swipe-to-close (timezone pickers, profile edit forms, docs). The route itself provides the slide animation. |
| `NavigationHelpers.pushToItemScreen()` | Shorthand for navigating to gear / experience / request by item type string. |

Pass `useRootNavigator: true` when navigating from inside a modal or nested navigator so the new screen overlays everything (including the modal).

`context.push()` (GoRouter) is reserved for auth flows in `login_screen.dart` only. Run `npm run lint:nav` to catch violations in CI.

#### Adding SwipeToCloseMixin to a screen

Detail/content screens pushed via `pushScreen()` are responsible for their own slide animation via `SwipeToCloseMixin`. Add it to any new full-screen detail screen:

```dart
class _MyScreenState extends ConsumerState<MyScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {

  @override
  Widget build(BuildContext context) {
    // Pattern A — screen has an AppBar:
    return buildSwipeableScaffold(
      appBar: AppBar(
        leading: AppBarBackButton(onPressed: handleClose),
      ),
      body: ...,
    );

    // Pattern B — overlay screen without AppBar (e.g. full-bleed media):
    // return buildSwipeableContent(child: Scaffold(...));
  }
}
```

✅ **DO:**
- Use `AppBarBackButton(onPressed: handleClose)` so the back button plays the reverse slide animation before popping
- Use `BackButtonWidget` for overlay screens with a floating circular back button
- Add a `if (!context.mounted) return;` guard between `Navigator.pop()` and any subsequent navigation call
- Use `pushScreen` + `SwipeToCloseMixin` for all primary content screens

❌ **DON'T:**
- Use `Navigator.push(MaterialPageRoute(...))` — animates both screens, flashes background
- Use `context.push()` for in-app navigation (GoRouter replaces the route stack)
- Add `SwipeToCloseMixin` to list/menu screens or screens with text input fields
- Use `Icons.arrow_back` directly — use `AppBarBackButton` or `BackButtonWidget`

### Testing

✅ **DO:**
- Test ViewModels with fake implementations
- Use [`ProviderContainer`](https://riverpod.dev/docs/cookbooks/testing) for unit tests
- Test async flows completely
- Test error cases

❌ **DON'T:**
- Test UI and logic together
- Use real services in unit tests
- Skip error case testing

## Architecture Strengths

### ✅ Strengths

1. **Type Safety:** Protocol Buffers provide end-to-end type safety from server to UI
2. **Predictable State:** Freezed + Riverpod make state changes explicit and traceable
3. **Performance:** Stash-based memory caching with LRU eviction delivers instant responses
4. **Testability:** ViewModels are pure Dart classes, easy to test without Flutter
5. **Scalability:** Clear separation of concerns makes it easy to add features
6. **Consistency:** Single state management pattern across the entire app
7. **Async-First:** No sync/async confusion, everything is consistently async
8. **Developer Experience:** Hot reload, code generation, strong typing

## Architecture Weaknesses

### ⚠️ Weaknesses

1. **Boilerplate:** Freezed, Riverpod, and repositories require code generation and setup
2. **Learning Curve:** Developers need to understand Riverpod, Freezed, and protobuf
3. **State Explosion:** Complex screens can have large state classes with many fields
4. **Testing Setup:** Test setup requires provider containers and mock configuration
5. **No Disk Persistence:** Cache doesn't survive app restarts (Phase 1 limitation)
6. **Global TTL:** Can't set per-entry TTL yet (Phase 1 uses global 30-minute policy)
7. **No Offline Queue:** Failed mutations are not automatically retried

## Next Steps

### Recommended Improvements

1. **Caching (see [caching.md](caching.md))**
   - Phase 2: Add disk persistence for cache
   - Phase 3: Implement offline mutation queue
   - Phase 4: Add cache warming and metrics

2. **Code Generation**
   - Generate ViewModels from OpenAPI specs
   - Auto-generate repository methods
   - Generate test fixtures

3. **Performance**
   - Implement pagination for large lists
   - Add infinite scroll support
   - Optimize memory usage for image caching

4. **Developer Tools**
   - Add Riverpod inspector integration
   - Create VS Code snippets for common patterns
   - Build CLI for scaffolding ViewModels

5. **State Management**
   - Consider family providers for parameterized state
   - Evaluate AsyncNotifier for simpler async patterns
   - Assess need for global vs scoped providers

6. **Testing**
   - Add integration tests for critical flows
   - Improve test utilities and helpers
   - Add golden tests for UI consistency

## Resources

### Key Technologies
- **Riverpod 3.0:** https://riverpod.dev/
- **Freezed:** https://pub.dev/packages/freezed
- **Stash:** https://pub.dev/packages/stash
- **Protocol Buffers:** https://protobuf.dev/

### Internal Documentation
- [caching.md](caching.md) - Stash-based caching architecture, patterns, and guidelines
- [testing.md](testing.md) - Testing architecture, patterns, and guidelines

### Code Examples
- [conversation_view_model.dart](../app/lib/presentation/viewmodels/conversation_view_model.dart) - Complex ViewModel with streams
- [user_repository.dart](../app/lib/data/repositories/user_repository.dart) - Repository with Stash caching
- [transfer_repository.dart](../app/lib/data/repositories/transfer_repository.dart) - Mutations with auto-invalidation
- [stash_cache_manager.dart](../app/lib/data/cache/stash_cache_manager.dart) - Stash implementation
- [search_view_model.dart](../../app/lib/presentation/viewmodels/search_view_model.dart) - SafeNotifierMixin usage
- [safe_notifier.dart](../../app/lib/core/utils/safe_notifier.dart) - SafeNotifierMixin implementation

---

## Appendix

### Design Patterns

#### ViewModel Initialization

ViewModels that need parameters use the **initialize pattern**:

```dart
class MyNotifier extends Notifier<MyState> {
  @override
  MyState build() {
    throw UnimplementedError('Call initialize() first');
  }

  Future<void> initialize({required String id}) async {
    state = MyState(id: id);
    await _loadData();
  }
}

// In widget:
@override
void initState() {
  super.initState();
  WidgetsBinding.instance.addPostFrameCallback((_) {
    ref.read(myProvider.notifier).initialize(id: widget.id);
  });
}
```

#### Resource Cleanup

Use [`ref.onDispose()`](https://riverpod.dev/docs/concepts/provider_lifespan) for cleanup:

```dart
class MyNotifier extends Notifier<MyState> {
  StreamSubscription? _subscription;

  @override
  MyState build() {
    ref.onDispose(() {
      _subscription?.cancel();
    });
    return const MyState();
  }
}
```

#### Manual Stream Management

For complex scenarios with multiple streams or timers, use regular `Notifier` (not `StreamNotifier`):

```dart
class ConversationNotifier extends Notifier<ConversationState> {
  StreamSubscription<Message>? _messageStream;
  final Map<String, Timer> _pendingTimers = {};

  @override
  ConversationState build() {
    ref.onDispose(() {
      _messageStream?.cancel();
      _pendingTimers.values.forEach((t) => t.cancel());
    });
    return const ConversationState(...);
  }
}
```

#### Repository Composition

Repositories can depend on other repositories **shallowly** (max 1 level):

```dart
class UserRepository {
  final MediaRepository _mediaRepository;

  // ✅ CORRECT: One-level dependency
  Future<UserProfile> getUserProfile(String userId) async {
    final user = await get(userId);
    final mediaUrl = await _mediaRepository.getMediaUrl(user.mediaId);
    return UserProfile(user: user, mediaUrl: mediaUrl);
  }
}
```

**Don't:** Create deep repository chains or circular dependencies.

**See:** [caching.md](caching.md) for cache key naming conventions and patterns.

### Common Patterns

#### Loading States

```dart
@freezed
class MyState with _$MyState {
  const factory MyState({
    @Default([]) List<Item> items,
    @Default(true) bool isLoading,  // Start loading
    String? errorMessage,
  }) = _MyState;

  const MyState._();

  bool get hasError => errorMessage != null;
  bool get isEmpty => items.isEmpty && !isLoading;
}
```

#### Error Handling

```dart
Future<void> loadData() async {
  state = state.copyWith(isLoading: true, errorMessage: null);
  try {
    final items = await repository.list();
    if (!ref.mounted) return;  // Required for autoDispose providers
    state = state.copyWith(items: items, isLoading: false);
  } catch (e, stackTrace) {
    _log.severe('Failed to load data', e, stackTrace);
    if (!ref.mounted) return;  // Required for autoDispose providers
    state = state.copyWith(
      errorMessage: e.toString(),
      isLoading: false,
    );
  }
}
```

#### Optimistic Updates

```dart
Future<void> sendMessage(String text) async {
  // Add optimistically
  final tempMessage = Message(
    id: 'temp_${DateTime.now().millisecondsSinceEpoch}',
    text: text,
    state: MessageState.pending,
  );
  state = state.copyWith(
    messages: [...state.messages, tempMessage],
  );

  try {
    final sentMessage = await chatService.sendMessage(text);
    // Replace temp with real
    state = state.copyWith(
      messages: state.messages
          .where((m) => m.id != tempMessage.id)
          .toList()..add(sentMessage),
    );
  } catch (e) {
    // Mark as failed
    final failedMessage = tempMessage.copyWith(state: MessageState.failed);
    state = state.copyWith(
      messages: state.messages.map((m) =>
        m.id == tempMessage.id ? failedMessage : m
      ).toList(),
    );
  }
}
```

#### Parallel Data Loading

```dart
Future<void> initialize() async {
  state = state.copyWith(isLoading: true);

  // Load multiple things in parallel
  await Future.wait([
    _loadUser(),
    _loadGear(),
    _loadLoans(),
  ]);

  if (!ref.mounted) return;  // Required for autoDispose providers

  state = state.copyWith(isLoading: false);
}
```

#### Async Safety in Notifiers

When using `autoDispose` providers, the notifier can be disposed while async operations are in flight.

**Default Pattern: Use AsyncNotifier/FutureProvider**

The recommended approach is to use Riverpod's `AsyncNotifier` or `FutureProvider`, which handle lifecycle automatically:

```dart
// RECOMMENDED: AsyncNotifier handles disposal in build()
final myDataProvider = AsyncNotifierProvider.autoDispose<
    MyDataNotifier,
    MyData
>(MyDataNotifier.new);

class MyDataNotifier extends AsyncNotifier<MyData> {
  @override
  Future<MyData> build() async {
    return _loadData();  // Auto-handled on dispose - no ref.mounted needed
  }

  // IMPORTANT: Methods that manually set state still need ref.mounted checks!
  Future<void> refresh() async {
    if (!ref.mounted) return;  // Required before state assignment
    state = const AsyncLoading();

    final result = await AsyncValue.guard(() => _loadData());

    if (!ref.mounted) return;  // Required after async gap
    state = result;
  }

  Future<MyData> _loadData() async {
    final repo = ref.read(myRepositoryProvider);
    return repo.getData();
  }
}

// Widget consumption - clean pattern with built-in loading/error
ref.watch(myDataProvider).when(
  data: (data) => MyDataDisplay(data),
  loading: () => const LoadingSpinner(),
  error: (e, st) => ErrorWidget(e.toString()),
);
```

**Critical Finding from Migration:**

| Scenario | `build()` Method | Other Methods (`refresh()`, etc.) |
|----------|------------------|-----------------------------------|
| Disposal during async | **Automatic** - Riverpod handles it | **Manual** - Still need `ref.mounted` checks |
| Error handling | Captured as `AsyncError` | Need `AsyncValue.guard()` + mounted check |
| State access | Safe - managed by Riverpod | Need `ref.mounted` before `state = ...` |

**Why AsyncNotifier is preferred:**
- `build()` method handles disposal automatically - no checks needed
- `AsyncValue` provides type-safe loading/error/data states
- `.when()` pattern makes widget code cleaner
- Reduces (but doesn't eliminate) disposal-related crashes

**Transitional Tool: SafeNotifierMixin**

For existing Notifiers not ready for AsyncNotifier migration, use `SafeNotifierMixin`:

```dart
import 'package:ripls/core/utils/safe_notifier.dart';

class MyNotifier extends Notifier<MyState> with SafeNotifierMixin<MyState> {
  Future<void> loadData() async {
    safeUpdateState((s) => s.copyWith(isLoading: true));

    final results = await repository.getData();

    // Safe: logs warning if disposed, doesn't crash
    safeUpdateState((s) => s.copyWith(data: results, isLoading: false));
  }
}
```

The mixin logs when updates are skipped due to disposal, helping identify patterns that need migration.

**Legacy Pattern: Manual ref.mounted checks**

For existing Notifiers not yet migrated, check `ref.mounted` after async gaps:

```dart
// LEGACY: Manual checks required for regular Notifier with async
Future<void> loadData() async {
  state = state.copyWith(isLoading: true);

  final results = await Future.wait([...]);

  // REQUIRED: Check mounted after every await before accessing state
  if (!ref.mounted) return;

  state = state.copyWith(data: results, isLoading: false);
}
```

**Why crashes happen:** When a user navigates away, `autoDispose` providers dispose immediately. If an async operation is still in progress, the completion handler runs after disposal:

```
Cannot use the Ref of NotifierProvider<MyNotifier, MyState> after it has been disposed.
```

#### Widget Async Exceptions

Some async operations are appropriate in widgets. These are **documented exceptions** that require manual `mounted` checks:

| Pattern | Example | Why it's acceptable |
|---------|---------|---------------------|
| **Dialogs** | `showDialog()`, `showModalBottomSheet()` | Inherently UI-level, awkward in ViewModel |
| **Navigation with result** | `Navigator.push()` returning data | UI navigation concern |
| **Permission requests** | `Permission.camera.request()` | Single await, UI feedback needed |
| **Image/file pickers** | `ImagePicker.pickImage()` | Platform UI integration |

```dart
// ACCEPTABLE: Dialog result in widget (documented exception)
Future<void> _showConfirmDelete() async {
  final confirmed = await showDialog<bool>(
    context: context,
    builder: (context) => ConfirmDialog(message: 'Delete this item?'),
  );
  if (!mounted) return;  // Single guard for dialog result
  if (confirmed == true) {
    ref.read(gearProvider.notifier).delete(gearId);
  }
}
```

**Rule:** If async is in a widget, add a comment explaining why:

```dart
// Widget async exception: Platform image picker requires UI context
Future<void> _pickImage() async {
  final image = await ImagePicker().pickImage(source: ImageSource.gallery);
  if (!mounted) return;
  // ...
}
```

#### Disposal Testing

All async Notifiers **must** have disposal tests:

```dart
group('disposal safety', () {
  test('handles disposal during async operation gracefully', () async {
    final container = ProviderContainer();
    final notifier = container.read(myProvider.notifier);

    // Start async operation
    final future = notifier.loadData();

    // Dispose before completion
    container.dispose();

    // Should complete without throwing
    await expectLater(future, completes);
  });
});
```

**See also:**
- [Riverpod: ref.mounted documentation](https://riverpod.dev/docs/concepts/ref#refmounted)
- [Riverpod: AsyncNotifier](https://riverpod.dev/docs/providers/async_notifier_provider)
- [Analysis: ref_check.md](../ai/ref_check.md) for full migration plan

---

**Last Updated:** 2026-02-21 - Expanded Navigation & Gestures section with NavigationHelpers and SwipeToCloseMixin patterns
**Architecture Version:** 2.3 (Riverpod 3.0 + Stash + Async-First + Async Safety + Navigation Standards)
