package experience

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// findAndStoreStockVideo searches for a stock video using keywords and creates a copy for experience use.
// Returns the media ID of the copied video, or empty string if no video found.
// Follows copy-on-use pattern: creates a new media record with source_stock_image_id set for provenance tracking.
// The video media record has content_type video/mp4.
func (s *Service) findAndStoreStockVideo(ctx context.Context, keywords []string, userID string) (string, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "experience",
		"operation", "findAndStoreStockVideo",
		"keywords", keywords,
	)
	startTime := time.Now()

	if s.stockVideoProvider == nil {
		return "", fmt.Errorf("stock video provider not configured")
	}

	if len(keywords) == 0 {
		return "", fmt.Errorf("no keywords provided")
	}

	query := strings.Join(keywords, " ")

	stockItem, err := s.stockVideoProvider.GetStockVideo(ctx, query)
	if err != nil {
		logger.WarnContext(ctx, "failed to get stock video",
			"query", query,
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return "", fmt.Errorf("could not find suitable video for keywords: %v", keywords)
	}

	logger.InfoContext(ctx, "successfully retrieved stock video",
		"stock_image_id", stockItem.Id,
		"media_id", stockItem.MediaId,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	// Create a copy for this experience (enables cascade deletion and attribution tracking)
	mediaID, err := storage.CopyStockImageForUser(
		ctx,
		s.storage,
		s.bucket,
		stockItem,
		userID,
		"experience-stock.mp4",
		fmt.Sprintf("Stock video for experience: %s", query),
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to copy stock video",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return "", err
	}

	logger.InfoContext(ctx, "successfully created video copy for experience",
		"stock_image_id", stockItem.Id,
		"original_media_id", stockItem.MediaId,
		"copied_media_id", mediaID,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return mediaID, nil
}

// findAndStoreStockImage searches for a stock image using keywords and creates a copy for experience use.
// Returns the media ID of the copied image, or empty string if no image found.
// Follows copy-on-use pattern: creates a new media record with source_stock_image_id set for provenance tracking.
// This method joins keywords into a query string for the stock imagery provider.
func (s *Service) findAndStoreStockImage(ctx context.Context, keywords []string, userID string) (string, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "experience",
		"operation", "findAndStoreStockImage",
		"keywords", keywords,
	)
	startTime := time.Now()

	if s.stockImageryProvider == nil {
		return "", fmt.Errorf("stock imagery provider not configured")
	}

	if len(keywords) == 0 {
		return "", fmt.Errorf("no keywords provided")
	}

	// Join keywords into a single query
	query := strings.Join(keywords, " ")

	// Get stock image from provider
	stockImage, err := s.stockImageryProvider.GetStockImage(ctx, query, nil)
	if err != nil {
		logger.Warn("failed to get stock image", "query", query, "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return "", fmt.Errorf("could not find suitable image for keywords: %v", keywords)
	}

	logger.Info("successfully retrieved stock image",
		"stock_image_id", stockImage.Id,
		"media_id", stockImage.MediaId,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	// Create a copy for this experience (enables cascade deletion and attribution tracking)
	mediaID, err := storage.CopyStockImageForUser(
		ctx,
		s.storage,
		s.bucket,
		stockImage,
		userID,
		"experience-stock.jpg",
		fmt.Sprintf("Stock image for experience: %s", query),
	)
	if err != nil {
		logger.Error("failed to copy stock image", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return "", err
	}

	logger.Info("successfully created media copy for experience",
		"stock_image_id", stockImage.Id,
		"original_media_id", stockImage.MediaId,
		"copied_media_id", mediaID,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return mediaID, nil
}
