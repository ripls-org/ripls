package gear

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// findAndStoreStockImage searches for a stock image using the gear category/title and creates a copy for gear use.
// Returns the media ID of the copied image, or empty string if no image found.
// Follows copy-on-use pattern: creates a new media record with source_stock_image_id set for provenance tracking.
func (s *Service) findAndStoreStockImage(ctx context.Context, query, userID string) (string, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "gear",
		"operation", "findAndStoreStockImage",
		"query", query,
	)
	startTime := time.Now()

	if s.stockImageryProvider == nil {
		return "", fmt.Errorf("stock imagery provider not configured")
	}

	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("empty query provided")
	}

	// Get stock image from provider
	stockImage, err := s.stockImageryProvider.GetStockImage(ctx, query, nil)
	if err != nil {
		logger.Warn("failed to get stock image", "query", query, "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return "", fmt.Errorf("could not find suitable image for query: %s", query)
	}

	logger.Info("successfully retrieved stock image",
		"stock_image_id", stockImage.Id,
		"media_id", stockImage.MediaId,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	// Create a copy for this gear (enables cascade deletion and attribution tracking)
	mediaID, err := storage.CopyStockImageForUser(
		ctx,
		s.storage,
		s.bucket,
		stockImage,
		userID,
		"gear-stock.jpg",
		fmt.Sprintf("Stock image for gear: %s", query),
	)
	if err != nil {
		logger.Error("failed to copy stock image", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return "", err
	}

	logger.Info("successfully created media copy for gear",
		"stock_image_id", stockImage.Id,
		"original_media_id", stockImage.MediaId,
		"copied_media_id", mediaID,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return mediaID, nil
}
