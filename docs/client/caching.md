---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Client-side caching — repository pattern with Stash-based transparent caching, cache-key conventions, repository-level invalidation after mutations.
  globs: [app/lib/data/**]
  triggers: [cache, caching, repository, invalidation, stash, cache-key]
  lens: [client, architecture]
  domain: client
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Client-Side Caching

## Overview

The Ripls Flutter app uses a **Repository Pattern** with **[Stash](https://pub.dev/packages/stash)**-based transparent caching to deliver instant, consistent data access while minimizing network requests. All caching is handled at the repository layer using **async-first** APIs.

## Caching Architecture

```
                      Riverpod Provider
                            │
                            │ provides singleton
                            ▼
                  ┌──────────────────┐
                  │ StashCacheManager│
                  │   (singleton)    │
                  └────────┬─────────┘
                           │
            ┌──────────────┼──────────────┐
            │              │              │
            ▼              ▼              ▼
    ┌─────────────┐ ┌─────────────┐ ┌────────────────┐
    │UserRepository│ │GearRepository│ │TransferRepository│
    │             │ │             │ │                │
    │ uses        │ │ uses        │ │ uses           │
    │ ↓           │ │ ↓           │ │ ↓              │
    │CacheService │ │CacheService │ │CacheService    │
    │ namespace:  │ │ namespace:  │ │ namespace:     │
    │ 'user'      │ │ 'gear'      │ │ 'transfer'     │
    └─────────────┘ └─────────────┘ └────────────────┘
           │                │                │
           └────────────────┴────────────────┘
                           │
                      Cache Keys
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
        ▼                  ▼                  ▼
  'user:user123'     'gear:gear456'    'transfer:received:list'
```

**Key Caching Rules:**

1. **Repositories own caching - ViewModels don't**
   - Never implement manual cache maps in ViewModels
   - Never access StashCacheManager or CacheService directly from ViewModels
   - Always call repository methods for cached data access

2. **All cache operations are async**
   - No synchronous cache access (`getSync()` is deprecated)
   - Use FutureBuilder or AsyncValue for loading states
   - Consistent async pattern throughout

3. **Single CacheManager instance shared via Riverpod**
   - `cacheManagerProvider` provides singleton StashCacheManager
   - All repositories share the same cache instance
   - Namespace isolation prevents key collisions

## Core Principles

### 1. Async-First Architecture

**All data access is async.** The caching layer uses async APIs exclusively.

```dart
// ✅ CORRECT: Always use async
final user = await userRepository.get(userId);

// ❌ WRONG: getSync() is deprecated (returns null with StashCacheManager)
final user = userRepository.getSync(userId); // Returns null!
```

**Why:** Stash is async-only, so StashCacheManager's `getSync()` always returns null. Flutter's FutureBuilder and Riverpod's AsyncValue handle loading states naturally. Async-first eliminates sync/async confusion and prevents UI blocking.

### 2. Repository Pattern with Transparent Caching

Repositories provide async data access with automatic caching.

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

**Caching behavior:**
- Memory cache with LRU eviction (1000 max entries)
- Single global TTL configured via `CACHE_TTL_MINUTES` environment variable (default: 30 minutes)
- TTL refreshes on access using Stash's AccessedExpiryPolicy
- Automatic cache-or-fetch pattern
- Namespace isolation per repository

**Why:** Repositories centralize data access logic and eliminate duplicate caching code in ViewModels. Transparent caching makes the app feel instant (cache hits) while ensuring data freshness (TTL expiration).

### 3. Cache Invalidation

Repositories handle mutations and automatically invalidate caches.

```dart
class LoanRepository {
  Future<void> approveLoan(String loanId) async {
    await _service.approveLoan(loanId);
    // Automatically invalidate relevant caches
    await _cache.invalidate('received:list');
  }
}

// In ViewModel
await loanRepository.approveLoan(loanId);
await refreshInbox(); // Gets fresh data (cache invalidated)
```

**Why:** Manual cache invalidation is error-prone. Repositories that own mutations also own invalidation, ensuring consistency.

## Usage Patterns

### Pattern 1: Fetching Data in ViewModels

ViewModels call repositories directly for cached data access. Repositories handle all caching automatically.

```dart
class GearNotifier extends Notifier<GearState> {
  Future<void> loadGear(String gearId) async {
    final gear = await ref.read(gearRepositoryProvider).get(gearId);
    state = state.copyWith(gear: gear);
  }
}
```

### Pattern 2: Mutations with Auto-Invalidation

Repositories handle both service calls and cache invalidation together. ViewModels just call repository methods.

```dart
class LoanNotifier extends Notifier<LoanState> {
  Future<void> approveLoan(String loanId) async {
    await ref.read(loanRepositoryProvider).approveLoan(loanId);
    await loadLoans(); // Gets fresh data
  }
}
```

### Pattern 3: Dependency Injection with Riverpod

Riverpod provides singleton CacheManager to all repositories. ViewModels access repositories through providers.

```dart
final userRepositoryProvider = Provider<UserRepository>((ref) {
  return UserRepository(
    ref.watch(cacheManagerProvider),
    ref.watch(userServiceProvider),
  );
});
```

### Pattern 4: Parallel Data Loading

Use Future.wait() to load multiple items concurrently. Each repository handles its own caching independently.

```dart
class InboxNotifier extends Notifier<InboxState> {
  Future<void> initialize() async {
    final results = await Future.wait([
      ref.read(loanRepositoryProvider).listLoanRequestsReceived(),
      ref.read(userRepositoryProvider).get(currentUserId),
    ]);
  }
}
```

### Pattern 5: Pull-to-Refresh Cache Invalidation

Both [feed_screen](../app/lib/presentation/screens/feed/feed_screen.dart) and [discover_screen](../app/lib/presentation/screens/discover/discover_screen.dart) implement comprehensive pull-to-refresh that uses pattern-based cache clearing to ensure fresh data.

**Implementation:**
- [FeedNotifier.refresh()](../app/lib/presentation/viewmodels/feed_view_model.dart#L140-L177) - Clears `gear:*`, `request:*`, `user:*`, `media:*`
- [DiscoverNotifier.refresh()](../app/lib/presentation/viewmodels/discover_view_model.dart#L102-L144) - Clears `gear:*`, `user:*`, `media:*`, `location:*`

**Why:** Screens display nested data (gear → user profiles, media, locations). Without clearing all related caches, users would see mixed fresh/stale data (e.g., updated gear name but old user avatar).

**Trade-off:** This "nuclear" approach over-invalidates but guarantees correctness. Acceptable because pull-to-refresh is user-initiated, infrequent, and cache repopulates quickly.

### Pattern 6: Automatic Cache Invalidation with UI Preservation

The discover screen uses SearchRepository's global cache invalidation mechanism to automatically update search results when items are edited, while preserving scroll position and carousel state.

**How it works:**
1. User edits an item (e.g., changes media on an experience)
2. Repository's save method calls `SearchRepository.invalidateSearches()`
3. SearchRepository notifies via `searchCacheInvalidationProvider`
4. SearchViewModel listens for invalidation and triggers in-place refresh
5. Fresh data is fetched and state is updated with `refreshCount` increment
6. DiscoverScreen updates `selectedItem` reference before updating map markers
7. UI updates with new data while preserving scroll position and carousel position

**Key components:**
- `searchCacheInvalidationProvider` - Notification counter that increments on cache invalidation
- `SearchNotifier._refreshInPlace()` - Fetches fresh data without showing loading state
- `refreshCount` field in SearchState - Forces Riverpod to notify listeners even when data "looks the same"
- DiscoverScreen listener - Updates selectedItem reference to point to refreshed data

**Why refreshCount is necessary:** When search results refresh with the same number of items and same IDs, Riverpod's reference equality check may not notify listeners. The incrementing refreshCount field guarantees state is considered "changed" on every refresh.

**Why selectedItem update is necessary:** When items refresh with new data, selectedItem holds a reference to the old item object. Updating it to reference the new item with the same ID preserves carousel position by ensuring index calculation works correctly.

**Result:** Users see updated thumbnails and data immediately after editing, without losing their place in the list or carousel.

### Pattern 7: Event-Driven Cache Invalidation with Optimistic Patches (Portfolio Inbox)

The portfolio inbox stays fresh automatically after mutations, without requiring a
pull-to-refresh. Two complementary mechanisms are used: event-driven background
refreshes and optimistic unread count patches.

**Event-driven background refresh:**

1. Any mutation repository (TransferRepository, GearRepository, ExperienceRepository,
   RequestRepository, ChatRepository) calls `portfolioCacheInvalidationProvider.notify()`
   after its state-changing mutations.
2. `PortfolioInboxNotifier` listens to `portfolioCacheInvalidationProvider` in its `build()` method.
3. On signal, `PortfolioInboxNotifier._onInvalidated()` schedules a refresh (only once data
   has loaded — guarded on `state.feedResponse != null`):
   - **Not in cooldown:** debounces 300ms, then calls `_refreshInPlace()`.
   - **In cooldown:** sets a `_hasPendingInvalidation` flag, deferred until the cooldown ends.
4. When the inbox tab becomes active, `PortfolioInboxNotifier.onTabVisible()` checks the flag
   and triggers a refresh if set. Also triggered when the user returns from a detail
   screen (gear, experience, request) via `.then()` on the navigation push.
5. `_refreshInPlace()` invalidates the cache, fetches fresh data, and swaps in the new
   response while preserving toggle state (expanded metric, person, value panel).
   No loading spinner is shown — the user continues to see their existing data during
   the fetch.

**Debounce and cooldown parameters:**
- **Debounce:** 300ms — coalesces rapid cascade invalidations from a single user action
  (which often touches 2–3 repositories in quick succession).
- **Cooldown:** 5 seconds — limits server fetches to at most once per 5-second window.
  If an invalidation arrives during cooldown, one refresh is scheduled at the end
  of the cooldown period.

**Optimistic unread count patches:**

1. `PortfolioInboxNotifier` also listens to `unreadCountProvider` in its `build()` method.
2. When live unread counts change (new messages arrive, user marks conversation as read),
   `_applyUnreadDeltas()` computes the delta between live counts and the counts baked
   into the inbox view response at fetch time.
3. `PortfolioInboxState.unreadDeltas` stores a `Map<String, int>` (conversationId → delta).
4. `PortfolioInboxState.effectiveUnreadCount(item)` applies the delta on top of `item.unreadCount`,
   clamped to `[0, 999]`. The screen passes this to each `DailyItemRow` via
   `effectiveUnreadCount`.
5. On the next `_refreshInPlace()` completion, the delta map is rebuilt from fresh data,
   reconciling any client/server drift.

**Key components:**
- `portfolioCacheInvalidationProvider` — `Notifier<int>` counter incremented by repositories
- `PortfolioInboxNotifier._onInvalidated()` — handles incoming signals
- `PortfolioInboxNotifier._refreshInPlace()` — background fetch preserving toggle state
- `PortfolioInboxState.unreadDeltas` — per-conversation delta map for badge updates
- `PortfolioInboxState.effectiveUnreadCount(item)` — live-adjusted count for an inbox item
- `DailyItemRow.effectiveUnreadCount` — optional override parameter for live counts

**Result:** Users see updated item state, metrics, and conversation previews within
seconds of performing an action. Unread badges update instantly without any server
round-trip.

## Cache Key Naming Convention

All cache keys follow: `'namespace:identifier'`

```dart
// Repository namespaces
'user'      → CacheService(cacheManager, 'user')
'gear'      → CacheService(cacheManager, 'gear')
'transfer'  → CacheService(cacheManager, 'transfer')
'media'     → CacheService(cacheManager, 'media')
'community' → CacheService(cacheManager, 'community')

// Item keys (namespace added by CacheService)
'user:user123'           → _cache.get(key: 'user123')
'gear:gear456'           → _cache.get(key: 'gear456')

// List keys
'gear:user:list'         → User's gear list
'transfer:received:list' → Received transfer requests
```

**Why:**
- Namespace isolation prevents collisions
- Pattern-based invalidation: `invalidatePattern('gear:*')`
- Human-readable for debugging

## Image Cache Keys

### Separate Caching System

Image caching uses `CachedNetworkImage` with `flutter_cache_manager`, which is
**completely separate** from the data cache (`StashCacheManager`). Image bytes
are cached on disk, while API responses are cached in memory.

| Aspect | Data Cache | Image Cache | Video Cache (mobile / desktop) | Video Cache (web) |
|--------|-----------|-------------|-------------------------------|-------------------|
| **Library** | `StashCacheManager` | `flutter_cache_manager` (via `cached_network_image`) | `flutter_cache_manager` (via `VideoCacheManager`) | None (browser HTTP cache) |
| **What's cached** | API response objects (protobufs) | Image bytes | Video files | Nothing on disk; the browser caches `<video>` byte ranges per its own policy |
| **Storage** | Memory | Disk | Disk | Browser HTTP cache |
| **Key management** | `CacheService` with namespaces | `ImageCacheKeys` utility | `video_$mediaId` via `MediaRepository` | URL-keyed by the browser |

### Key Format

```
media:thumb:{mediaId}  → Thumbnail images
media:full:{mediaId}   → Full-resolution images
```

### Why Separate Keys?

Thumbnails and full images MUST use different cache keys. If they share a key,
`CachedNetworkImage` will serve the cached 400px thumbnail when a 4000px
full-resolution image is requested, causing blurry backgrounds.

### Usage

Always use `ImageCacheKeys` for consistency:

```dart
import 'package:ripls/core/utils/image_cache_keys.dart';

// Thumbnail (cards, avatars, lists)
CachedNetworkImage(
  imageUrl: url,
  cacheKey: ImageCacheKeys.thumbnail(mediaId),
)

// Full resolution (backgrounds, detail views)
CachedNetworkImage(
  imageUrl: url,
  cacheKey: ImageCacheKeys.full(mediaId),
)

// Via MediaUrl (auto-selects based on isThumbnail flag)
final mediaUrl = await mediaRepository.getMediaUrl(mediaId);
CachedNetworkImage(
  imageUrl: mediaUrl.url,
  cacheKey: mediaUrl.cacheKey,
)
```

### Choosing Thumbnail vs Full

| Context | Method | Examples |
|---------|--------|----------|
| Small display (< 200px) | `thumbnail()` | Cards, avatars, list items |
| Large display (>= 200px) | `full()` | Backgrounds, detail views, viewers |

## Video Cache

Video playback uses a platform-conditional helper. On mobile / desktop the player reads from a disk-backed file cache; on web the player streams directly into an HTML5 `<video>` element and lets the browser handle range-request caching.

### Mobile / desktop

Video files are cached on disk separately from both the data cache and the image cache. Without caching, `VideoPlayerController.networkUrl()` makes repeated HTTP 206 range requests on every loop iteration and ViewModel recreation.

**How it works:**
- `MediaRepository.getVideoFile(mediaId, url)` downloads the video once and caches it on disk, keyed by `video_$mediaId`. Subsequent calls return the local file without a network request
- `createCachedVideoController()` ([video_cache_helper.dart](../app/lib/core/utils/video_cache_helper.dart)) dispatches via a conditional import to `video_cache_helper_io.dart`, which wraps the repository call and returns an initialized, looping, muted `VideoPlayerController.file()` ready for playback
- All content ViewModels use this helper for server-loaded video

**Cache configuration:**
- Backed by `VideoCacheManager` ([video_cache_manager.dart](../app/lib/data/cache/video_cache_manager.dart)), a singleton extending `flutter_cache_manager`'s `CacheManager`
- Max 100 cached objects, 7-day stale period, LRU eviction
- `MediaRepository.evictVideo(mediaId)` removes a specific entry

**Key rule:** ViewModels access video caching through `MediaRepository`, never through `VideoCacheManager` directly. The cache manager is an internal data-layer detail.

### Web

`dart:io`, `flutter_cache_manager`, and `VideoPlayerController.file` have no web implementation. On web, `createCachedVideoController` dispatches to `video_cache_helper_web.dart`, which builds the controller directly via `VideoPlayerController.networkUrl(Uri.parse(url))` — rendering into an HTML5 `<video>` element via `video_player_web`. The browser's HTTP cache (not `flutter_cache_manager`) caches range-request responses across reloads.

`MediaRepository.getVideoFile` is guarded by `assert(!kIsWeb, ...)` so a stray web caller fails loudly in debug rather than dying inside the `dart:io`-based stack at runtime.

## Global TTL Configuration

All cached entries use a single global TTL configured via the `CACHE_TTL_MINUTES` environment variable in `env.*.json` files:
- **local**: 5 minutes (for rapid testing)
- **dev**: 30 minutes (default)
- **prod**: 60 minutes (longer cache for production)

Set `CACHE_TTL_MINUTES=0` to completely disable caching (always fetches fresh data).

**How it works:**
- All entries share the same TTL value across all repositories
- The TTL refreshes on access using Stash's `AccessedExpiryPolicy` (frequently accessed data stays cached longer)
- This simplified approach ensures consistent caching behavior throughout the app

**Note:** StashCacheManager Phase 1 uses a global environment-configured TTL. Per-entry TTLs may be supported in Phase 2 with disk persistence.

## Do's and Don'ts

### Data Access

✅ **DO:**
- Always use async methods for data access
- Use repositories for all data access
- Let repositories handle caching
- Call `refresh*()` or `invalidate*()` after mutations
- Use FutureBuilder or AsyncValue for loading states

❌ **DON'T:**
- Access services directly from ViewModels
- Create manual cache maps in ViewModels
- Use `getSync()` (it returns null with StashCacheManager)
- Skip repository layer
- Access StashCacheManager directly

### Repository Design

✅ **DO:**
- Use CacheService for namespace isolation
- Provide `refresh*()` methods for cache invalidation
- Handle mutations and invalidation together
- Keep dependencies shallow (max 1 level)
- Document cache keys and TTLs

❌ **DON'T:**
- Create circular repository dependencies
- Expose CacheManager to ViewModels
- Forget to invalidate after mutations
- Use deep repository chains

### Testing

✅ **DO:**
- Use Mockito to mock repositories in tests
- Test cache invalidation behavior
- Test async flows completely
- Test error cases

❌ **DON'T:**
- Test UI and logic together
- Use real services in unit tests
- Skip error case testing

## Testing with Caching

### Unit Tests: Mock Repositories

```dart
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';

@GenerateMocks([UserRepository, GearRepository])
void main() {
  test('loads user successfully', () async {
    final mockRepo = MockUserRepository();
    when(mockRepo.get('user-123')).thenAnswer((_) async => mockUser);

    final notifier = UserNotifier(mockRepo);
    await notifier.loadUser('user-123');

    expect(notifier.state.user, equals(mockUser));
    verify(mockRepo.get('user-123')).called(1);
  });
}
```

### Integration Tests: Test Repository Caching

```dart
void main() {
  test('cache invalidation works correctly', () async {
    final cache = StashCacheManager();
    await cache.initialize();

    final mockService = MockGearService();
    final repo = GearRepository(cache, mockService);

    when(mockService.getGear('gear-1')).thenAnswer((_) async => gear1);

    // First fetch - cache miss
    await repo.get('gear-1');
    verify(mockService.getGear('gear-1')).called(1);

    // Second fetch - cache hit (no service call)
    await repo.get('gear-1');
    verifyNever(mockService.getGear('gear-1'));

    // Invalidate and refetch
    await repo.invalidate('gear-1');
    await repo.get('gear-1');
    verify(mockService.getGear('gear-1')).called(1);
  });
}
```

## Troubleshooting

### Problem: Stale Data Displayed

**Symptom:** UI shows old data after mutation

**Solution:** Use repository mutations or manually refresh

```dart
// ✅ Best: Repository handles mutation + invalidation
await loanRepository.approveLoan(loanId);
await loadLoans(); // Gets fresh data

// ✅ Alternative: Manual refresh
await loanService.approveLoan(loanId);
await loanRepository.refreshLoanRequestsReceived();
await loadLoans();
```

### Problem: getSync() Returns Null

**Symptom:** `getSync()` always returns null

**Solution:** Use async methods instead

```dart
// ❌ Wrong: getSync() is deprecated
final user = userRepository.getSync(userId); // Returns null!

// ✅ Correct: Use async
final user = await userRepository.get(userId);
```

**Why:** StashCacheManager is async-only. Use FutureBuilder or AsyncValue for loading states.

### Problem: Redundant API Calls

**Symptom:** Network requests even with cached data

**Solution:** Use repositories, not services

```dart
// ❌ Wrong: Calling service directly
final user = await userService.getUser(userId); // Always fetches

// ✅ Correct: Use repository
final user = await userRepository.get(userId); // Uses cache
```
## Future Enhancements

Current implementation is **Phase 1** (memory-only). Future phases:

### Phase 2: Disk Persistence
- Persist cache to disk across app restarts
- Per-entry TTL support
- Selective persistence (some data memory-only)

### Phase 3: Offline Support
- Queue mutations when offline
- Sync when connection restored
- Conflict resolution

### Phase 4: Advanced Features
- Cache warming on app startup
- Metrics and monitoring (hit rates, size)
- Debug UI for cache inspection

See [client_stash.md](client_stash.md) for roadmap details.

## Architecture Strengths

✅ **Strengths:**

1. **Consistency:** Single source of truth with automatic invalidation
2. **Performance:** Memory caching with LRU eviction delivers instant responses
3. **Scalability:** 80% reduction in API calls per session
4. **Developer Experience:** Transparent caching - just call repositories
5. **Type Safety:** Protocol Buffers provide end-to-end type safety
6. **Testability:** Easy to mock repositories in tests
7. **Battle-Tested:** Stash is a proven caching library

## Architecture Weaknesses

⚠️ **Weaknesses:**

1. **No Disk Persistence:** Cache doesn't survive app restarts (Phase 1)
2. **Global TTL Only:** All entries share the same TTL; can't optimize TTL per data type (Phase 1 limitation)
3. **No Offline Queue:** Failed mutations are not automatically retried
4. **Manual Invalidation:** Developers must remember to invalidate after mutations

## Resources

### Key Technologies
- **Stash:** https://pub.dev/packages/stash
- **Riverpod 3.0:** https://riverpod.dev/
- **Protocol Buffers:** https://protobuf.dev/

### Internal Documentation
- `architecture.md` - Overall app architecture
- `client_stash.md` - Stash migration plan and roadmap
- `media.md` - Media architecture including caching

### Code Examples
- [StashCacheManager](../app/lib/data/cache/stash_cache_manager.dart) - Stash implementation
- [CacheService](../app/lib/data/cache/cache_service.dart) - Namespace wrapper
- [UserRepository](../app/lib/data/repositories/user_repository.dart) - Example repository
- [TransferRepository](../app/lib/data/repositories/transfer_repository.dart) - Mutations with auto-invalidation

---

## Appendix

### Core Components

#### CacheManager Interface

**Location:** [app/lib/data/cache/cache_manager.dart](../app/lib/data/cache/cache_manager.dart)

Defines the caching contract implemented by StashCacheManager.

```dart
abstract class CacheManager {
  Future<T> get<T>({
    required String key,
    required Future<T> Function() fetch,
  });

  T? getSync<T>(String key); // DEPRECATED: Returns null with Stash

  Future<void> put<T>(String key, T value);
  Future<void> remove(String key);
  Future<void> clear({String? pattern});
  CacheStats getStats();
}
```

#### StashCacheManager

**Location:** [app/lib/data/cache/stash_cache_manager.dart](../app/lib/data/cache/stash_cache_manager.dart)

Stash-based implementation using memory storage.

**Features:**
- Memory-only storage (Phase 1)
- LRU eviction with 1000 max entries
- Single global TTL configured via `CACHE_TTL_MINUTES` environment variable
- TTL refreshes on access using Stash's AccessedExpiryPolicy
- Auto-initialization on first use
- Pattern-based cache clearing
- Set `CACHE_TTL_MINUTES=0` to disable caching

**Limitations:**
- `getSync()` always returns null (Stash is async-only)
- All entries share the same global TTL (per-entry TTL not supported in Phase 1)
- Entry counts in stats are 0 (Stash size is async)

#### CacheService

**Location:** [app/lib/data/cache/cache_service.dart](../app/lib/data/cache/cache_service.dart)

Wraps CacheManager with namespace support.

```dart
class CacheService {
  final CacheManager _cache;
  final String _namespace;

  Future<T> get<T>({required String key, required Future<T> Function() fetch});
  Future<void> invalidate(String key);
  Future<void> invalidatePattern(String pattern);
  Future<void> invalidateAll();
}
```

### Available Repositories

#### MediaRepository
- **Purpose:** Media URL caching with thumbnail support
- **Location:** [media_repository.dart](../app/lib/data/repositories/media_repository.dart)

#### UserRepository
- **Purpose:** User profile caching
- **Location:** [user_repository.dart](../app/lib/data/repositories/user_repository.dart)

#### GearRepository
- **Purpose:** Gear item and list caching
- **Location:** [gear_repository.dart](../app/lib/data/repositories/gear_repository.dart)

#### TransferRepository
- **Purpose:** Transfer (loan and giveaway) caching with mutations
- **Location:** [transfer_repository.dart](../app/lib/data/repositories/transfer_repository.dart)

#### CommunityRepository
- **Purpose:** Community data and member caching
- **Location:** [community_repository.dart](../app/lib/data/repositories/community_repository.dart)

**Note:** All repositories use the same global TTL configured via `CACHE_TTL_MINUTES` environment variable.

---

## Appendix: Advanced Caching Patterns

### Optimistic Delta Pattern (UnreadCountRepository)

The UnreadCountRepository uses an **optimistic delta pattern** to provide instant UI updates for unread message counts while maintaining eventual consistency with the server.

#### How It Works

The repository maintains two data sources:
1. **Server counts** - Cached response from `getUnreadCounts()` RPC
2. **Optimistic deltas** - In-memory `Map<String, int>` tracking local adjustments

Total displayed count = cached server count + sum(optimistic deltas)

See [unread_count_repository.dart](../app/lib/data/repositories/unread_count_repository.dart) for the full implementation.

#### When to Use This Pattern

✅ **Use optimistic deltas when:**
- Immediate UI feedback is critical for UX (e.g., unread counts, notifications)
- The operation is eventually consistent (server will catch up)
- You can reconcile deltas with server state periodically
- The value is numeric and can be incremented/decremented

❌ **Avoid optimistic deltas when:**
- The data structure is complex (not a simple number)
- Conflicts are common or difficult to resolve
- Server state can change in unpredictable ways
- The operation must be immediately consistent

#### Key Operations

**Delta Creation:**
- `incrementUnreadCount()` - Adds +1 when receiving new messages
- `resetUnreadCount()` - Sets negative delta to negate server count when marking as read
- See [unread_count_repository.dart:128-152](../app/lib/data/repositories/unread_count_repository.dart#L128-L152)

**Delta Application:**
- `getTotalUnreadCount()` - Returns server count + sum of all deltas
- `getUnreadCount()` - Returns server count + delta for specific conversation
- See [unread_count_repository.dart:28-66](../app/lib/data/repositories/unread_count_repository.dart#L28-L66)

**Delta Cleanup:**
- `invalidate()` - Clears both cache AND deltas together (critical!)
- `syncWithServer()` - Self-healing logic that clears deltas when server count is 0
- See [unread_count_repository.dart:154-194](../app/lib/data/repositories/unread_count_repository.dart#L154-L194)

#### Trade-offs

**Pros:**
- ✅ Instant UI updates (no loading spinners)
- ✅ Better perceived performance
- ✅ Maintains consistency across app (all widgets see same count)

**Cons:**
- ❌ Additional complexity (must manage delta lifecycle)
- ❌ Risk of bugs if deltas aren't cleared properly (double-counting)
- ❌ Requires careful synchronization with server state

#### Common Pitfalls

1. **Forgetting to clear deltas on cache invalidation** - Must clear both cache AND deltas together. See `invalidate()` in [unread_count_repository.dart:154-161](../app/lib/data/repositories/unread_count_repository.dart#L154-L161)

2. **Bypassing repository invalidation** - ViewModels must call repository methods, not access cache directly. This was the root cause of the double-decrement bug.

3. **Not reconciling with server** - Deltas must be cleared when server catches up. The `syncWithServer()` method implements self-healing logic. See [unread_count_repository.dart:171-185](../app/lib/data/repositories/unread_count_repository.dart#L171-L185)

---

**Last Updated:** 2026-03-10
**Architecture Version:** 1.1 (Stash + Async-First + Optimistic Deltas + Video Disk Cache)
