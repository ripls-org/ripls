package media

import (
	"fmt"

	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// StockProviderKeys holds the per-vendor API keys for the stock media
// providers. An empty key disables that vendor.
type StockProviderKeys struct {
	UnsplashAccessKey string
	PexelsAPIKey      string
	PixabayAPIKey     string
}

// NewStockProviders assembles the stock imagery and video provider chains
// from whichever vendor keys are present:
//
//   - Images: Unsplash (primary) → Pexels (fallback), each wrapped in the
//     semantic cache so repeated queries reuse stored images.
//   - Video: Pexels (primary) → Pixabay (fallback), wrapped in the semantic
//     video cache. Video requires the Pexels key.
//
// With no keys set both providers are nil — callers already nil-guard, so
// stock media is simply disabled.
func NewStockProviders(keys StockProviderKeys, sqlStorage *storage.ProtoSQLStorage, bucketStorage storage.BucketStorage, logger *logging.Logger) (StockImageryProvider, StockVideoProvider, error) {
	if keys.UnsplashAccessKey == "" && keys.PexelsAPIKey == "" {
		logger.Info("No stock imagery API keys set, stock images disabled")
		return nil, nil, nil
	}
	logger.Info("initializing stock imagery providers")

	var providers []StockImageryProvider
	var providerNames []string
	var stockVideoProvider StockVideoProvider

	// Add Unsplash provider if configured (PRIMARY)
	if keys.UnsplashAccessKey != "" {
		unsplashClient := NewUnsplashClient(keys.UnsplashAccessKey)
		unsplashProvider := NewUnsplashProvider(unsplashClient, sqlStorage, bucketStorage)

		// Wrap with semantic cache layer for efficient image reuse
		cachedUnsplash := NewCachedStockImageryProvider(
			unsplashProvider,
			sqlStorage,
			DefaultSimilarityThreshold,
		)

		providers = append(providers, cachedUnsplash)
		providerNames = append(providerNames, "unsplash")
		logger.Info("Unsplash provider initialized as primary")
	}

	// Add Pexels provider if configured (FALLBACK for images, PRIMARY for video)
	if keys.PexelsAPIKey != "" {
		pexelsClient := NewPexelsClient(keys.PexelsAPIKey)
		pexelsProvider := NewPexelsProvider(pexelsClient, sqlStorage, bucketStorage)

		// Build a video provider chain: Pexels first, Pixabay as fallback.
		var videoProviders []StockVideoProvider
		var videoProviderNames []string
		videoProviders = append(videoProviders, pexelsProvider)
		videoProviderNames = append(videoProviderNames, "pexels")

		if keys.PixabayAPIKey != "" {
			pixabayClient := NewPixabayClient(keys.PixabayAPIKey)
			pixabayProvider := NewPixabayProvider(pixabayClient, sqlStorage, bucketStorage)
			videoProviders = append(videoProviders, pixabayProvider)
			videoProviderNames = append(videoProviderNames, "pixabay")
			logger.Info("Pixabay video provider initialized as fallback")
		}

		var rawVideoProvider StockVideoProvider
		if len(videoProviders) > 1 {
			fallbackVideo, err := NewFallbackStockVideoProvider(videoProviders, videoProviderNames)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to initialize fallback video provider: %w", err)
			}
			rawVideoProvider = fallbackVideo
			logger.Info("video provider fallback chain initialized", "providers", videoProviderNames)
		} else {
			rawVideoProvider = pexelsProvider
		}

		// Wrap the video provider chain with semantic cache.
		stockVideoProvider = NewStockVideoProviderCached(
			rawVideoProvider,
			sqlStorage,
			DefaultSimilarityThreshold,
			FallbackSimilarityThreshold,
		)
		logger.Info("stock video provider initialized with semantic cache")

		// Wrap with semantic cache layer for efficient image reuse
		cachedPexels := NewCachedStockImageryProvider(
			pexelsProvider,
			sqlStorage,
			DefaultSimilarityThreshold,
		)

		providers = append(providers, cachedPexels)
		providerNames = append(providerNames, "pexels")
		logger.Info("Pexels provider initialized as fallback")
	}

	// Create fallback chain with all configured providers
	fallbackProvider, err := NewFallbackStockImageryProvider(
		providers,
		providerNames,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize fallback provider: %w", err)
	}

	logger.Info("stock imagery provider initialized with fallback chain", "providers", providerNames)
	return fallbackProvider, stockVideoProvider, nil
}
