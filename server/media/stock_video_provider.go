package media

import (
	"context"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
)

// StockVideoProvider searches for and downloads stock video clips.
// Implementations should be nil-safe at call sites: use constructor injection and check
// for nil before calling, following the same pattern as StockImageryProvider.
type StockVideoProvider interface {
	// GetStockVideo fetches a stock video clip matching the query.
	// Returns a StockImage record whose media has content_type video/mp4, or an error
	// if no suitable video is found.
	GetStockVideo(ctx context.Context, query string) (*models.StockImage, error)

	// SearchStockVideoCandidates returns lightweight candidate entries
	// (poster URL + metadata only, no download) so callers can surface
	// alternates without paying download cost up-front. Mirrors
	// StockImageryProvider.SearchStockImageCandidates. Candidate
	// `content_type` is "video/*" so clients can render a play overlay
	// on the thumbnail. Implementations that do not support listing
	// alternates return nil, nil.
	SearchStockVideoCandidates(ctx context.Context, query string, limit int) ([]StockImageCandidate, error)

	// GetStockVideoByID fetches a specific video by its provider-side id
	// and stores it as a canonical StockImage (deduped against existing).
	// Mirrors StockImageryProvider.GetStockImageByID for videos.
	GetStockVideoByID(ctx context.Context, providerVideoID string) (*models.StockImage, error)

	// CheckHealth verifies the provider is accessible and returns status information.
	CheckHealth(ctx context.Context) ([]*health.Status, error)
}
