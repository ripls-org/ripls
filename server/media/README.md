# Media

The `media` package is a library for stock imagery and stock video: fetching, caching, fallback, and provider abstraction. It does not handle user-uploaded media (that lives in `server/services/media` and is stored in GCS); it handles external stock content used to illustrate gear, requests, and experiences when no user photo is available.

## Key files

- `stock_imagery_provider.go` — `StockImageryProvider` interface and `StockImageOptions`.
- `pexels_provider.go` — stock image provider backed by the Pexels API.
- `pixabay_provider.go` — stock image provider backed by the Pixabay API.
- `unsplash_provider.go` — stock image provider backed by the Unsplash API.
- `cached_stock_imagery.go` — caching decorator that avoids duplicate provider calls for the same query.
- `stock_imagery_fallback.go` — `FallbackStockImageryProvider`: tries Pexels, Pixabay, and Unsplash in order.
- `stock_video_provider.go`, `stock_video_provider_cached.go`, `stock_video_fallback.go` — equivalent stack for stock video.
- `fake_provider.go` — deterministic fake for use in tests.

## When to add code here vs. elsewhere

A new stock content source (image or video provider) belongs here as a new `StockImageryProvider` or `StockVideoProvider` implementation. User media upload, presigned GCS URL generation, and media metadata storage belong in `server/services/media`. AI-generated thumbnails belong in `server/ai`.
