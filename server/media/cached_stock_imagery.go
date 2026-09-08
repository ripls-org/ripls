package media

import (
	"context"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// DefaultSimilarityThreshold is the minimum similarity score for a cache hit.
// Cosine similarity ranges from 0 (orthogonal) to 1 (identical).
const DefaultSimilarityThreshold = 0.85

// CachedStockImageryProvider wraps a StockImageryProvider with semantic caching.
// It uses vector similarity search to find existing stock images that match the query
// before delegating to the underlying provider.
type CachedStockImageryProvider struct {
	primary   StockImageryProvider
	storage   *storage.ProtoSQLStorage
	threshold float64
}

// NewCachedStockImageryProvider creates a caching wrapper around a stock imagery provider.
// The threshold controls the minimum similarity score (0.0-1.0) required for a cache hit.
// Use DefaultSimilarityThreshold for the recommended default.
func NewCachedStockImageryProvider(
	primary StockImageryProvider,
	sqlStorage *storage.ProtoSQLStorage,
	threshold float64,
) *CachedStockImageryProvider {
	return &CachedStockImageryProvider{
		primary:   primary,
		storage:   sqlStorage,
		threshold: threshold,
	}
}

// GetStockImage attempts to find a cached stock image matching the query before
// delegating to the underlying provider.
func (c *CachedStockImageryProvider) GetStockImage(ctx context.Context, query string, opts *StockImageOptions) (*models.StockImage, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "stock_imagery",
		"operation", "GetStockImage",
		"provider", "cached",
		"query", query,
	)
	startTime := time.Now()

	if opts == nil {
		opts = &StockImageOptions{}
	}

	// RequireUnique bypasses the cache entirely
	if opts.RequireUnique {
		logger.DebugContext(ctx, "bypassing cache due to RequireUnique",
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return c.primary.GetStockImage(ctx, query, opts)
	}

	// Check if embedder is configured
	embedder := c.storage.GetEmbedder()
	if embedder == nil {
		logger.DebugContext(ctx, "no embedder configured, delegating to primary",
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return c.primary.GetStockImage(ctx, query, opts)
	}

	// Generate embedding for the query
	queryEmbedding, err := embedder.Generate(ctx, query)
	if err != nil {
		logger.WarnContext(ctx, "failed to generate query embedding, delegating to primary",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return c.primary.GetStockImage(ctx, query, opts)
	}

	// Search for similar stock images
	info := embedder.Info()
	results, err := c.storage.QueryBySemanticSearch(ctx, &models.StockImage{}, info, queryEmbedding, 1)
	if err != nil {
		logger.WarnContext(ctx, "semantic search failed, delegating to primary",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return c.primary.GetStockImage(ctx, query, opts)
	}

	// Check if we have a cache hit
	if len(results) > 0 && results[0].Similarity >= c.threshold {
		match := results[0]
		cached := &models.StockImage{}
		if err := c.storage.GetByID(ctx, match.ID, cached); err != nil {
			logger.WarnContext(ctx, "failed to load cached stock image, delegating to primary",
				"stock_image_id", match.ID,
				"error", err,
				"duration_ms", time.Since(startTime).Milliseconds(),
			)
			return c.primary.GetStockImage(ctx, query, opts)
		}
		logger.InfoContext(ctx, "cache hit",
			"stock_image_id", match.ID,
			"media_id", cached.MediaId,
			"similarity", match.Similarity,
			"threshold", c.threshold,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)

		return cached, nil
	}

	// Cache miss - delegate to primary
	if len(results) > 0 {
		logger.DebugContext(ctx, "cache miss - similarity below threshold",
			"best_similarity", results[0].Similarity,
			"threshold", c.threshold,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
	} else {
		logger.DebugContext(ctx, "cache miss - no existing stock images",
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
	}

	return c.primary.GetStockImage(ctx, query, opts)
}

// SearchStockImageCandidates delegates to the primary provider. The
// per-image cache here is keyed on `query` for chosen images and does
// not pre-populate candidate URLs; the primary's lightweight search
// path is the source of truth.
func (c *CachedStockImageryProvider) SearchStockImageCandidates(ctx context.Context, query string, limit int) ([]StockImageCandidate, error) {
	return c.primary.SearchStockImageCandidates(ctx, query, limit)
}

// GetStockImageByID delegates to the primary provider. The semantic
// cache here is keyed by query similarity; direct-by-id lookups bypass
// it because they're already deterministic.
func (c *CachedStockImageryProvider) GetStockImageByID(ctx context.Context, providerImageID string) (*models.StockImage, error) {
	return c.primary.GetStockImageByID(ctx, providerImageID)
}

// CheckHealth delegates to the primary provider's health check.
func (c *CachedStockImageryProvider) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	return c.primary.CheckHealth(ctx)
}
