package media

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/storage"
)

// StockImageOptions configures stock image fetching behavior.
type StockImageOptions struct {
	// RequireUnique ensures the returned image hasn't been used elsewhere in Ripls.
	RequireUnique bool
}

// StockImageCandidate carries the metadata needed to surface a stock
// image alternate to the client without downloading the bytes. The
// streaming Gen* flows ride a slice of these on MediaReady so the
// client can render thumbnails in the Replace Media modal.
type StockImageCandidate struct {
	// URL the server fetches when this candidate is imported. For image
	// candidates this is also a reasonable thumbnail source. For video
	// candidates this is the MP4 URL; the client renders ThumbnailURL
	// as the in-modal thumbnail instead.
	URL string
	// ContentType is the MIME type of URL. "image/jpeg" for photo
	// candidates, "video/mp4" for Pexels-video candidates.
	ContentType string
	// WidthPx and HeightPx are the advertised pixel dimensions when
	// known; zero when the provider did not declare them.
	WidthPx  int32
	HeightPx int32
	// Provider identifies which stock-image source produced this
	// candidate. Drives attribution at import time.
	Provider models.StockImageryProvider
	// ProviderPhotoID is the provider-specific photo or video id; used
	// by the server at import time to look up attribution metadata.
	ProviderPhotoID string
	// ThumbnailURL is a separate URL the client should render as the
	// in-modal thumbnail. Only meaningful for video candidates, where
	// URL points at the MP4 and this points at the poster image.
	// Empty for image candidates.
	ThumbnailURL string
}

// StockImageryProvider defines the interface for fetching stock imagery.
type StockImageryProvider interface {
	// GetStockImage fetches a stock image matching the query.
	// Returns the StockImage record, or an error if no suitable image is found.
	GetStockImage(ctx context.Context, query string, opts *StockImageOptions) (*models.StockImage, error)

	// SearchStockImageCandidates returns lightweight candidate entries
	// (URL + metadata only, no download) so callers can surface
	// alternates without paying download cost up-front. Implementations
	// that do not support listing alternates return nil, nil. The
	// caller passes a limit on how many entries to return.
	SearchStockImageCandidates(ctx context.Context, query string, limit int) ([]StockImageCandidate, error)

	// GetStockImageByID fetches a specific photo by its provider-side id
	// and stores it as a canonical StockImage (deduped against existing).
	// Used by the Replace Media flow so a candidate import preserves
	// attribution without paying a search round-trip. Implementations
	// that do not support direct lookup return an error and the caller
	// falls back to generic-URL import.
	GetStockImageByID(ctx context.Context, providerImageID string) (*models.StockImage, error)

	// CheckHealth verifies the provider is accessible and returns status information.
	CheckHealth(ctx context.Context) ([]*health.Status, error)
}

// StockImageAttribution returns a formatted attribution string for display.
// Format: "Photo by {name} on {provider}".
func StockImageAttribution(stockImage *models.StockImage) string {
	if stockImage == nil || stockImage.ProviderImage == nil || stockImage.ProviderImage.Creator == nil {
		return ""
	}
	providerName := "Unknown"
	switch stockImage.Provider {
	case models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH:
		providerName = "Unsplash"
	case models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS:
		providerName = "Pexels"
	case models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_FAKE:
		providerName = "Test"
	}
	return fmt.Sprintf("Photo by %s on %s", stockImage.ProviderImage.Creator.Name, providerName)
}

// GetStockImageByID retrieves a StockImage record by its ID.
// Returns nil if the stock image is not found.
func GetStockImageByID(ctx context.Context, sqlStorage *storage.ProtoSQLStorage, id string) (*models.StockImage, error) {
	stockImage := &models.StockImage{}
	if err := sqlStorage.GetByID(ctx, id, stockImage); err != nil {
		return nil, fmt.Errorf("failed to get stock image: %w", err)
	}
	return stockImage, nil
}
