# data/repositories

Data-access layer with transparent caching. Repositories are the single source of truth for all app data — viewmodels and screens read from repositories, never directly from services.

## Repository pattern

Each repository:
1. Accepts a `CacheManager` and one or more service instances via its constructor.
2. Creates a `CacheService` scoped to its namespace (e.g., `'gear'`).
3. Exposes `get*()` / `list*()` methods that check the cache before calling the service.
4. Exposes `refresh*()` or `invalidate*()` methods that callers must use after mutations.

Never call `invalidate()` by constructing cache keys by hand — use the repository's own invalidation methods so key formatting stays consistent.

## Cache key conventions

| Pattern | Example | Meaning |
|---|---|---|
| `namespace:item_id` | `gear:abc123` | Single entity |
| `namespace:list_type` | `gear:user:list` | Flat list |
| `namespace:list_type:filter` | `loan:received:gear:abc123` | Filtered list |

The namespace is set per-repository in the `CacheService` constructor call; the remainder is the key passed to `cache.get()` / `cache.invalidate()`.

## Key types

- **`GearRepository`** — gear items; cross-invalidates `SearchRepository`, `FeedRepository`, and `ChatRepository` on mutation.
- **`MediaRepository`** — media assets; returns `MediaUrl` objects with stable cache keys for `CachedNetworkImage`. Always use `CachedMediaImage` for display, never `Image.network`.
- **`UserRepository`** — user profiles.
- **`LoanRepository`** / **`TransferRepository`** — loan and transfer state.
- **`CommunityRepository`** — community membership and metadata.
- **`FeedRepository`** / **`StoryRepository`** — feed and story items.
- **`SearchRepository`** — search results (gear and communities).
- **`MediaUrl`** (`media_url.dart`) — wraps a presigned URL with a stable `cacheKey` derived from the media ID.

## When to add code here vs. elsewhere

- **New entity type** with server-side storage → new repository file.
- **New field on an existing response** → extend the existing repository's fetch/cache path.
- **Business logic** (computing a derived value, combining two entities) → belongs in the viewmodel or a `core/utils/` helper, not the repository.
- **Raw API access** with no caching need → call the service directly from the viewmodel (rare; prefer caching).
