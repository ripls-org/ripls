package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// FakeProviderConfig configures the behavior of FakeProvider for testing.
type FakeProviderConfig struct {
	// AlwaysReturnNew forces creation of new images even without RequireUnique.
	// Useful for testing scenarios where caching should be bypassed.
	AlwaysReturnNew bool

	// SimulateError causes GetStockImage to return this error.
	SimulateError error

	// SimulateNoResults causes GetStockImage to return an error as if no images were found.
	SimulateNoResults bool
}

// FakeProvider implements StockImageryProvider for testing.
// It generates unique test images based on queries without making external API calls.
type FakeProvider struct {
	storage *storage.ProtoSQLStorage
	bucket  storage.BucketStorage
	config  FakeProviderConfig

	// imageCounter tracks how many images have been created (for unique IDs)
	imageCounter int
}

// NewFakeProvider creates a new fake stock imagery provider for testing.
func NewFakeProvider(sqlStorage *storage.ProtoSQLStorage, bucket storage.BucketStorage) *FakeProvider {
	return &FakeProvider{
		storage:      sqlStorage,
		bucket:       bucket,
		config:       FakeProviderConfig{},
		imageCounter: 0,
	}
}

// NewFakeProviderWithConfig creates a new fake provider with custom configuration.
func NewFakeProviderWithConfig(sqlStorage *storage.ProtoSQLStorage, bucket storage.BucketStorage, config FakeProviderConfig) *FakeProvider {
	return &FakeProvider{
		storage:      sqlStorage,
		bucket:       bucket,
		config:       config,
		imageCounter: 0,
	}
}

// GetStockImage returns a fake stock image for testing.
// The image is generated based on the query, so the same query produces the same image ID.
func (p *FakeProvider) GetStockImage(ctx context.Context, query string, opts *StockImageOptions) (*models.StockImage, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "stock_imagery",
		"provider", "fake",
		"operation", "GetStockImage",
		"query", query,
	)
	startTime := time.Now()

	// Check for simulated errors
	if p.config.SimulateError != nil {
		return nil, p.config.SimulateError
	}
	if p.config.SimulateNoResults {
		return nil, fmt.Errorf("no images found for query: %s", query)
	}

	if opts == nil {
		opts = &StockImageOptions{}
	}

	// Generate a deterministic image ID based on query (for cache testing)
	imageID := p.generateImageID(query, opts.RequireUnique)

	// Check for existing image unless RequireUnique or AlwaysReturnNew
	if !opts.RequireUnique && !p.config.AlwaysReturnNew {
		existing, err := p.getExistingStockImage(ctx, imageID)
		if err == nil && existing != nil {
			logger.Info("returning existing fake stock image",
				"stock_image_id", existing.Id,
				"media_id", existing.MediaId,
				"image_id", imageID,
				"duration_ms", time.Since(startTime).Milliseconds(),
			)
			return existing, nil
		}
	}

	// Generate a unique image with the query text encoded
	imageData := p.generateTestImage(query, imageID)

	// Store the fake image as Media (canonical stock image, not a copy)
	mediaID, err := storage.StoreMedia(
		ctx,
		p.storage,
		p.bucket,
		SystemUserID,
		imageData,
		"image/jpeg",
		fmt.Sprintf("fake_%s.jpg", imageID),
		fmt.Sprintf("Fake test image for: %s", query),
		"",    // Canonical stock images don't reference themselves
		false, // stripMetadata: retain EXIF (#1953)
	)
	if err != nil {
		logger.Error("failed to store fake media", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to store fake media: %w", err)
	}

	// Build provider image metadata
	providerImage := &models.ProviderImage{
		Id:             imageID,
		Url:            fmt.Sprintf("https://fake.test/%s.jpg", imageID),
		Description:    fmt.Sprintf("Fake image for query: %s", query),
		AltDescription: "Test image",
		Creator: &models.CreatorInfo{
			Name:     "Test Creator",
			Username: "testcreator",
		},
	}

	// Create StockImage record
	stockImage := &models.StockImage{
		Provider:         models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_FAKE,
		ProviderImage:    providerImage,
		MediaId:          mediaID,
		CreatedAtUnixSec: time.Now().Unix(),
	}

	stockImageID, err := p.storage.Insert(ctx, stockImage)
	if err != nil {
		logger.Error("failed to insert stock image record", "error", err, "media_id", mediaID, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to insert stock image record: %w", err)
	}
	stockImage.Id = stockImageID

	logger.Info("fake stock image created",
		"stock_image_id", stockImageID,
		"media_id", mediaID,
		"image_id", imageID,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return stockImage, nil
}

// generateImageID creates a deterministic ID based on query.
// If requireUnique is true, appends a counter to ensure uniqueness.
func (p *FakeProvider) generateImageID(query string, requireUnique bool) string {
	hash := sha256.Sum256([]byte(query))
	baseID := fmt.Sprintf("fake-%x", hash[:8])

	if requireUnique {
		p.imageCounter++
		return fmt.Sprintf("%s-%d", baseID, p.imageCounter)
	}
	return baseID
}

// generateTestImage creates a simple test image with a colored pattern.
// The color is derived from the query hash, making each query visually distinct.
func (p *FakeProvider) generateTestImage(query, _ string) []byte {
	// Create a 100x100 image with a color derived from the query
	hash := sha256.Sum256([]byte(query))
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))

	// Use hash bytes to generate a unique color
	baseColor := color.RGBA{R: hash[0], G: hash[1], B: hash[2]}

	// Fill with a gradient pattern
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			// Create a simple gradient effect
			r := uint8((int(baseColor.R) + x) % 256)
			g := uint8((int(baseColor.G) + y) % 256)
			b := baseColor.B
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}

	// Encode as JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		// Fallback to minimal JPEG if encoding fails
		return minimalJPEG
	}
	return buf.Bytes()
}

// getExistingStockImage returns an existing StockImage for the given image ID.
func (p *FakeProvider) getExistingStockImage(ctx context.Context, imageID string) (*models.StockImage, error) {
	results, err := p.storage.QueryByField(ctx, "provider_image_id", imageID, &models.StockImage{})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0].(*models.StockImage), nil
}

// minimalJPEG is a fallback minimal valid JPEG.
var minimalJPEG = []byte{
	0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
	0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
	0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
	0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
	0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
	0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
	0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
	0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
	0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x14, 0x00, 0x01,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x03, 0xFF, 0xC4, 0x00, 0x14, 0x10, 0x01, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F, 0x00,
	0x37, 0xFF, 0xD9,
}

// SearchStockImageCandidates returns nil for the fake provider — tests
// that need candidates should use a dedicated stub or the real Pexels
// path. Keeps the fake's behavior simple and predictable.
func (p *FakeProvider) SearchStockImageCandidates(_ context.Context, _ string, _ int) ([]StockImageCandidate, error) {
	return nil, nil
}

// GetStockImageByID is not supported on the fake provider.
func (p *FakeProvider) GetStockImageByID(_ context.Context, _ string) (*models.StockImage, error) {
	return nil, fmt.Errorf("fake provider by-id lookup not supported")
}

// CheckHealth returns a healthy status for the fake provider.
func (p *FakeProvider) CheckHealth(_ context.Context) ([]*health.Status, error) {
	return []*health.Status{{
		Name:    "stock_imagery",
		Backend: "fake",
	}}, nil
}
