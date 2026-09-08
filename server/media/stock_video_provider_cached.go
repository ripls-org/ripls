// Package media provides media upload, storage, and stock imagery services.
//
// stock_video_provider_cached.go implements a semantic caching layer for stock video
// lookups. CachedStockVideoProvider wraps any StockVideoProvider and uses
// pgvector similarity search against the shared stock_image table to avoid
// redundant Pexels API calls.
//
// Cache lookup strategy:
//  1. Embed the text query using the configured embedder.
//  2. Run a vector similarity search over stock_image rows, fetching the top N
//     results (cachedVideoSearchLimit). The table stores both images and videos,
//     so only rows whose ProviderImage.Id starts with "video_" are eligible.
//  3. If the best matching video exceeds the primary threshold, return it as a
//     cache hit and skip the external API call entirely.
//  4. If no cache hit is found, delegate to the primary StockVideoProvider.
//  5. If the primary provider returns a rate-limit error and a below-threshold
//     video candidate exists, return that candidate when its similarity meets
//     the lower fallbackThreshold — keeping the AI flow in video-first mode
//     rather than silently degrading to a static image.
package media

import (
	"context"
	"strings"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// videoProviderIDPrefix is the prefix used for Pexels video entries in ProviderImage.Id.
// Video entries use "video_<pexelsID>" while image entries use just "<pexelsID>".
const videoProviderIDPrefix = "video_"

// cachedVideoSearchLimit is the number of semantic search results to request.
// We request more than 1 because the shared stock_image table contains both image
// and video entries, and the top results by similarity may be images.
const cachedVideoSearchLimit = 5

// FallbackSimilarityThreshold is the minimum similarity score required for a cached
// video to be used as a rate-limit fallback. It is intentionally lower than
// DefaultSimilarityThreshold (0.85) to allow moderately relevant cached videos to be
// returned when the API is rate limited, while still rejecting irrelevant videos that
// would degrade the user experience more than falling back to a static image.
const FallbackSimilarityThreshold = 0.65

// StockVideoProviderCached wraps a StockVideoProvider with semantic caching.
// It uses vector similarity search to find existing stock video entries that match
// the query before delegating to the underlying provider. Only entries with a
// "video_" prefix in ProviderImage.Id are considered cache hits.
type StockVideoProviderCached struct {
	primary           StockVideoProvider
	storage           *storage.ProtoSQLStorage
	threshold         float64
	fallbackThreshold float64
}

// NewStockVideoProviderCached creates a caching wrapper around a stock video provider.
// The threshold controls the minimum similarity score (0.0-1.0) required for a cache hit.
// The fallbackThreshold controls the minimum similarity required for a cached video to be
// used when the primary provider is rate limited; use FallbackSimilarityThreshold for the
// recommended default.
func NewStockVideoProviderCached(
	primary StockVideoProvider,
	sqlStorage *storage.ProtoSQLStorage,
	threshold float64,
	fallbackThreshold float64,
) *StockVideoProviderCached {
	return &StockVideoProviderCached{
		primary:           primary,
		storage:           sqlStorage,
		threshold:         threshold,
		fallbackThreshold: fallbackThreshold,
	}
}

// GetStockVideo attempts to find a cached stock video matching the query before
// delegating to the underlying provider. Only entries with a "video_" prefix in
// ProviderImage.Id are considered cache hits.
func (c *StockVideoProviderCached) GetStockVideo(ctx context.Context, query string) (*models.StockImage, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "stock_video",
		"operation", "GetStockVideo",
		"provider", "cached",
		"query", query,
	)
	startTime := time.Now()

	// Check if embedder is configured.
	embedder := c.storage.GetEmbedder()
	if embedder == nil {
		logger.DebugContext(ctx, "no embedder configured, delegating to primary",
			"duration_ms", time.Since(startTime).Milliseconds())
		return c.primary.GetStockVideo(ctx, query)
	}

	// Generate embedding for the query.
	queryEmbedding, err := embedder.Generate(ctx, query)
	if err != nil {
		logger.WarnContext(ctx, "failed to generate query embedding, delegating to primary",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return c.primary.GetStockVideo(ctx, query)
	}

	// Search for similar stock entries (shared table contains both images and videos).
	info := embedder.Info()
	results, err := c.storage.QueryBySemanticSearch(ctx, &models.StockImage{}, info, queryEmbedding, cachedVideoSearchLimit)
	if err != nil {
		logger.WarnContext(ctx, "semantic search failed, delegating to primary",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return c.primary.GetStockVideo(ctx, query)
	}

	// Scan all results to find the best video entry. The shared stock_image table
	// contains both image and video entries, so we must check the video_ prefix.
	// We track the best video match even below threshold for rate-limit fallback.
	var bestVideo *models.StockImage
	var bestVideoSimilarity float64
	var bestVideoID string
	for _, match := range results {
		cached := &models.StockImage{}
		if err := c.storage.GetByID(ctx, match.ID, cached); err != nil {
			logger.WarnContext(ctx, "failed to load cached stock image, skipping",
				"stock_image_id", match.ID,
				"error", err)
			continue
		}
		if cached.ProviderImage == nil || !strings.HasPrefix(cached.ProviderImage.Id, videoProviderIDPrefix) {
			continue
		}
		// First video entry found (highest similarity since results are ordered).
		bestVideo = cached
		bestVideoSimilarity = match.Similarity
		bestVideoID = match.ID
		break
	}

	// Return cache hit if the best video is above threshold.
	if bestVideo != nil && bestVideoSimilarity >= c.threshold {
		logger.InfoContext(ctx, "cache hit",
			"stock_image_id", bestVideoID,
			"media_id", bestVideo.MediaId,
			"similarity", bestVideoSimilarity,
			"threshold", c.threshold,
			"duration_ms", time.Since(startTime).Milliseconds())
		return bestVideo, nil
	}

	// Cache miss — delegate to primary.
	if bestVideo != nil {
		logger.DebugContext(ctx, "cache miss - best video below threshold",
			"best_video_similarity", bestVideoSimilarity,
			"threshold", c.threshold,
			"duration_ms", time.Since(startTime).Milliseconds())
	} else if len(results) > 0 {
		logger.DebugContext(ctx, "cache miss - no video entries in results",
			"best_similarity", results[0].Similarity,
			"duration_ms", time.Since(startTime).Milliseconds())
	} else {
		logger.DebugContext(ctx, "cache miss - no existing stock entries",
			"duration_ms", time.Since(startTime).Milliseconds())
	}

	result, err := c.primary.GetStockVideo(ctx, query)
	if err != nil && isRateLimited(err) && bestVideo != nil {
		if bestVideoSimilarity >= c.fallbackThreshold {
			// API is rate limited and the cached video is relevant enough — use it
			// rather than falling back to a static image.
			logger.InfoContext(ctx, "rate limited, using best cached video as fallback",
				"stock_image_id", bestVideoID,
				"media_id", bestVideo.MediaId,
				"similarity", bestVideoSimilarity,
				"fallback_threshold", c.fallbackThreshold,
				"duration_ms", time.Since(startTime).Milliseconds())
			return bestVideo, nil
		}
		// Cached video exists but is too dissimilar — let the error propagate so
		// gen_ai.go can fall back to the image APIs instead.
		logger.DebugContext(ctx, "rate limited, cached video too dissimilar for fallback",
			"stock_image_id", bestVideoID,
			"similarity", bestVideoSimilarity,
			"fallback_threshold", c.fallbackThreshold,
			"duration_ms", time.Since(startTime).Milliseconds())
	}
	return result, err
}

// SearchStockVideoCandidates delegates to the primary provider. The
// per-video cache here is keyed for chosen videos and does not
// pre-populate candidate poster URLs; the primary's lightweight search
// path is the source of truth.
func (c *StockVideoProviderCached) SearchStockVideoCandidates(ctx context.Context, query string, limit int) ([]StockImageCandidate, error) {
	return c.primary.SearchStockVideoCandidates(ctx, query, limit)
}

// GetStockVideoByID delegates to the primary provider. By-id lookups
// bypass the semantic cache because they're already deterministic.
func (c *StockVideoProviderCached) GetStockVideoByID(ctx context.Context, providerVideoID string) (*models.StockImage, error) {
	return c.primary.GetStockVideoByID(ctx, providerVideoID)
}

// CheckHealth delegates to the primary provider's health check.
func (c *StockVideoProviderCached) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	return c.primary.CheckHealth(ctx)
}
