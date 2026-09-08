# data/cache

Caching infrastructure used by all repositories.

## Key types

- **`CacheManager`** (`cache_manager.dart`) — abstract interface for all caching operations: `get()`, `put()`, `remove()`, `clear()`. Implementations provide the backing store. TTL is configured via the `CACHE_TTL_MINUTES` environment variable.
- **`StashCacheManager`** (`stash_cache_manager.dart`) — production implementation backed by an in-memory LRU store (max 1,000 entries). Initialized lazily; concurrent calls to `initialize()` are safe. Used for all proto-serializable app data.
- **`CacheService`** (`cache_service.dart`) — thin wrapper around `CacheManager` that prefixes every key with a namespace (e.g., `'gear'`). Repositories instantiate one `CacheService` per entity type to avoid key collisions and to support namespace-scoped invalidation.
- **`VideoCacheManager`** (`video_cache_manager.dart`) — disk-based file cache for video assets (200 MB cap, 7-day TTL). Backed by `flutter_cache_manager`. Access via `VideoCacheManager.instance`; keyed by stable media ID rather than the expiring presigned URL.

## Cache key conventions

Keys follow the pattern `namespace:item_id` for single items and `namespace:list_type:filter` for lists. Examples:

- `user:user123` — a single user profile
- `gear:user:list` — the current user's gear list
- `loan:received:gear:gear123` — loans received for a specific gear item

`CacheService` automatically prepends the namespace, so repositories only pass the suffix.

## When to use each type

| Need | Use |
|---|---|
| Cache a proto-based entity or list | `CacheService` wrapping `StashCacheManager` |
| Cache a video file on disk | `VideoCacheManager` |
| Test a repository in isolation | Inject a `StashCacheManager` with a short TTL |
| Add a new cache backend | Implement `CacheManager` and inject via `providers.dart` |

Do not add manual `Map<String, T>` caches to viewmodels. All caching goes through repositories, which use this infrastructure.
