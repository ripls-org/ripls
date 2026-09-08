---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Stock imagery subsystem — provider-independent StockImageryProvider interface with semantic cache, Pexels/Unsplash fallback chain, and StockImage persistence for attribution and provenance.
  globs: [server/media/**, proto/ripls/models/stock_image.proto]
  triggers: [stock-imagery, pexels, unsplash, stock-photo, attribution, image-fallback]
  lens: [server, domain]
  domain: content
freshness:
  verified_commit: "5c2d3e59c"
  verified_on: "2026-07-07"
---
# Stock Imagery

Ripls has an imagery-forward aesthetic. Users are encouraged to take photos to illustrate their content, but often they just contribute text, like a description of an event or a request. We use stock imagery to provide visual context for this kind of content throughout the app.

## Design

Different components of the app (e.g. the request or experience service) can request stock imagery based on a search query via a stock imagery API. This is an opaque interface that returns the best image for the query in the form of a media ID and attribution metadata that may be required to be shown along with the imagery.

### Provider-independent API abstraction

The main interface for accessing stock imagery is at `server/media/stock_imagery_provider.go`, exposing the `StockImageryProvider` interface. The `GetStockImage` function returns a full `*models.StockImage` record containing:

- `Id`: The stock image record ID (for provenance tracking)
- `MediaId`: The ID of the stored canonical Media record
- `Provider`: Which provider this came from (Pexels, Unsplash, Fake, etc.)
- `ProviderImage`: Metadata from the provider (for attribution)

Callers receive the full StockImage and are responsible for:
1. Associating the media with their domain objects (using `MediaId` or creating a copy)
2. Setting `source_stock_image_id` on any copied media to track provenance

The stock imagery API handles image fetching, storage as Media records, and caching—but not the caller-specific persistence.

**Uniqueness option**: Callers can request a unique image by passing a uniqueness flag. When enabled:

1. The cache layer is bypassed entirely
2. Results from the provider are filtered against existing `StockImage` records by `provider_image_id`
3. Only images not already in our system are considered

This ensures that different content items (e.g., two different requests) can have visually distinct stock imagery even when their descriptions are semantically similar.

### StockImage data model

Stock imagery metadata is persisted in a `StockImage` proto model (defined in `proto/ripls/models/stock_image.proto`). Each record represents an image fetched from a provider and includes:

- **Provider identification**: Which provider (e.g., "unsplash"), the provider's original image ID, and the original URL from which the image was fetched
- **Media reference**: The media ID of the stored image in our Media system
- **Search text**: The semantic cache embeds the provider-supplied metadata stored on `provider_image` — its `description` and `alt_description` (configured at `server/storage/protosql_schema.go`). This enriches the cache with terms that accurately describe the image content.
- **Attribution**: Photographer name, username, and any required attribution text

This model enables semantic search over previously fetched images and ensures attribution requirements can be met.

### Multi-layer implementation with caching and fallback

The implementation uses a three-layer architecture: cache → fallback chain → individual providers.

**Cache layer**: Before querying external providers, the cache layer performs a semantic search over existing `StockImage` records. Using the same embedding infrastructure as other search features, it finds images whose query terms are semantically similar to the current query. If similarity exceeds a configured threshold (0.85), the cached image's media ID is returned without hitting external providers.

**Fallback layer**: When the cache misses, the fallback provider tries configured providers in sequence until one succeeds. Each provider is tried with full query fallback strategies before moving to the next provider.

**Provider layer**: Individual providers (Pexels, Unsplash) query external stock imagery services, download images, store them as Media records, and create corresponding `StockImage` records for future cache hits.

### Providers

#### Primary: Unsplash

Unsplash is the primary stock imagery provider (`server/media/stock_providers.go`). The implementation searches the Unsplash API, downloads images, and handles attribution.

**API Limits:**
- **Rate Limit**: 50 requests/hour (free tier)
- **Library Size**: 4+ million photos
- **Attribution**: Required ("Photo by [Photographer] on Unsplash")
- **License**: Free for personal and commercial use

**Note on download tracking**: When serving cached images, we skip Unsplash's download trigger since we already have the image. This may need verification for API terms compliance.

#### Fallback: Pexels

Pexels serves as the fallback image provider when Unsplash fails or returns no results. Pexels is also the primary provider for stock video (with Pixabay as its fallback).

**API Limits:**
- **Rate Limit**: 200 requests/hour per IP address (free tier)
- **Library Size**: 3+ million free stock photos
- **Attribution**: Optional but recommended ("Photo by [Photographer] on Pexels")
- **License**: Free for personal and commercial use

**Query Strategy**: Uses multi-strategy fallback with keyword extraction to maximize results.

### Dummy provider for testing

To speed up testing and avoid using API quota, the dummy provider returns a real static test image that is stored with test fixtures. This allows tests to exercise the full flow including media storage without external API calls.

## Implementation Notes

### File Organization

| File | Purpose |
|------|---------|
| `server/media/stock_imagery_provider.go` | Core interface (`StockImageryProvider`, `StockImageOptions`, `StockImageAttribution`) |
| `server/media/stock_providers.go` | `NewStockProviders` — assembles the image (Unsplash → Pexels) and video (Pexels → Pixabay) fallback chains from whichever vendor keys are configured |
| `server/media/stock_imagery_fallback.go` | Fallback provider orchestrating sequential provider attempts |
| `server/media/pexels_provider.go` | Pexels client and provider implementation with search, download, and storage (images + videos) |
| `server/media/unsplash_provider.go` | Unsplash client and provider implementation with search, download, and storage |
| `server/media/cached_stock_imagery.go` | Semantic cache wrapper using embedding similarity |
| `server/media/fake_provider.go` | Test provider generating colored images from query hash |
| `server/media/stock_video_provider.go` | `StockVideoProvider` interface for video-specific providers |
| `server/media/stock_video_fallback.go` | Fallback video provider chain (Pexels → Pixabay) |
| `server/media/pixabay_provider.go` | Pixabay client and provider for fallback stock video |
| `server/media/stock_video_provider_cached.go` | Semantic cache wrapper for video providers |
| `proto/ripls/models/stock_image.proto` | StockImage proto model with provider metadata |
| `proto/ripls/models/media.proto` | Media proto with `source_stock_image_id` for provenance tracking |

### Configuration

**Command-line flags:**

- `--pexels-api-key`: Pexels API key for fallback stock imagery and primary stock video provider
- `--unsplash-access-key`: Unsplash API key for primary image provider
- `--pixabay-api-key`: Pixabay API key for fallback video provider (optional, independent quota)
- At least one provider key is required for stock imagery to be enabled

**Environment variables (Docker/Cloud Run):**

- `PEXELS_API_KEY` → `--pexels-api-key`
- `PIXABAY_API_KEY` → `--pixabay-api-key`
- `UNSPLASH_ACCESS_KEY` → `--unsplash-access-key`

**Image provider order:**

1. **Semantic Cache** (checks all previously fetched images)
2. **Unsplash** (primary, 50 req/hour)
3. **Pexels** (fallback, 200 req/hour)

**Video provider order:**

1. **Semantic Cache** (checks stock_image table for entries with `video_` prefix in ProviderImage.Id)
2. **Pexels Videos** (primary, shares Pexels quota)
3. **Pixabay Videos** (fallback, independent 20,000 req/month quota)

**Constants:**

- `DefaultSimilarityThreshold` (0.85): Minimum cosine similarity for cache hit. Higher values require closer semantic match.
- `defaultSearchResults` (10): Number of results to request from providers (supports RequireUnique filtering)

### Testing with FakeProvider

The `FakeProvider` generates deterministic colored images based on query hash, enabling tests without external API calls:

```go
// Basic usage
provider := media.NewFakeProvider(storage, bucket)

// With error simulation
provider := media.NewFakeProviderWithConfig(storage, bucket, media.FakeProviderConfig{
    SimulateError: fmt.Errorf("simulated failure"),
})
```

### Copy-on-Use Pattern

Services create copies of stock images rather than referencing the canonical version:

1. `StockImageryProvider.GetStockImage()` returns a `*models.StockImage` with the canonical media ID
2. When attaching to a request/community/gear/experience, the service reads the image bytes and stores a new copy
3. The copy's `source_stock_image_id` is set to `stockImage.Id` for provenance tracking
4. This enables cascade deletion—when a request is deleted, its media copy is deleted too

The canonical stock image media does NOT have `source_stock_image_id` set—only copies do. This allows distinguishing between original cached images and copies made for specific domain objects.

**⚠️ Common mistake**: Returning `stockImage.MediaId` directly without creating a copy. This breaks attribution tracking since the canonical media record does not have `source_stock_image_id` set. Always create a copy and pass `stockImage.Id` as the `sourceStockImageID` parameter to `storage.StoreMedia()`.

### Semantic Cache and Fallback Flow

1. Generate embedding for query
2. Search `stock_image` table by vector similarity
3. If similarity ≥ threshold (0.85), return cached `media_id`
4. Otherwise, delegate to fallback provider
5. Fallback tries Unsplash (primary):
   - Searches with query + keyword fallbacks
   - If successful, stores image and creates `StockImage` record
   - Returns `media_id`
6. If Unsplash fails, fallback tries Pexels:
   - Searches with query + keyword fallbacks
   - If successful, stores image and creates `StockImage` record
   - Returns `media_id`
7. If all providers fail, returns error

**Query Fallback Strategy** (applied within each provider):
1. Try full query
2. Try extracted keywords joined
3. Try first 2-3 keywords
4. Try single most important keyword

This multi-level fallback (cache → Unsplash → Pexels) combined with query strategies maximizes image fetch success rate.
