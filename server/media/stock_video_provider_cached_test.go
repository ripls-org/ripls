package media

import (
	"context"
	"fmt"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/storage"
)

// mockStockVideoProvider tracks calls for testing the video cache wrapper.
type mockStockVideoProvider struct {
	calls  int
	result *models.StockImage
	err    error
}

func (m *mockStockVideoProvider) GetStockVideo(_ context.Context, _ string) (*models.StockImage, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	if m.result != nil {
		return m.result, nil
	}
	return &models.StockImage{
		Id:       "primary-video-id",
		MediaId:  "primary-media-id",
		Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS,
		ProviderImage: &models.ProviderImage{
			Id:          "video_99999",
			Description: "primary video result",
			Creator: &models.CreatorInfo{
				Name: "Primary Creator",
			},
		},
	}, nil
}

func (m *mockStockVideoProvider) SearchStockVideoCandidates(_ context.Context, _ string, _ int) ([]StockImageCandidate, error) {
	return nil, nil
}

func (m *mockStockVideoProvider) GetStockVideoByID(_ context.Context, _ string) (*models.StockImage, error) {
	return nil, fmt.Errorf("not supported")
}

func (m *mockStockVideoProvider) CheckHealth(_ context.Context) ([]*health.Status, error) {
	return []*health.Status{{Name: "stock_video", Backend: "mock"}}, nil
}

// TestCachedStockVideoProvider_NoEmbedderDelegatesToPrimary tests that
// when no embedder is configured, all requests go to the primary provider.
func TestCachedStockVideoProvider_NoEmbedderDelegatesToPrimary(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	mockPrimary := &mockStockVideoProvider{}
	cached := NewStockVideoProviderCached(mockPrimary, sqlStorage, DefaultSimilarityThreshold, FallbackSimilarityThreshold)

	ctx := context.Background()
	result, err := cached.GetStockVideo(ctx, "skiing mountain resort")
	if err != nil {
		t.Fatalf("GetStockVideo failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if mockPrimary.calls != 1 {
		t.Errorf("Expected 1 call to primary, got %d", mockPrimary.calls)
	}
}

// TestCachedStockVideoProvider_CacheHit_VideoEntry tests that a cached video
// entry (with "video_" prefix in ProviderImage.Id) is returned as a cache hit.
func TestCachedStockVideoProvider_CacheHit_VideoEntry(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	embeddingDone := setupTestEmbedder(t, sqlStorage)

	ctx := context.Background()

	// Insert a stock entry with video_ prefix (a video record).
	stockVideo := &models.StockImage{
		Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS,
		ProviderImage: &models.ProviderImage{
			Id:          "video_12345",
			Description: "skiing at a mountain resort with fresh powder",
			Creator: &models.CreatorInfo{
				Name: "Video Creator",
			},
		},
		MediaId: "cached-video-media-id",
	}
	_, err := sqlStorage.Insert(ctx, stockVideo)
	if err != nil {
		t.Fatalf("Failed to insert stock video: %v", err)
	}

	waitForEmbedding(t, embeddingDone)

	mockPrimary := &mockStockVideoProvider{}
	cached := NewStockVideoProviderCached(mockPrimary, sqlStorage, 0.1, FallbackSimilarityThreshold) // Low threshold

	result, err := cached.GetStockVideo(ctx, "skiing mountain resort powder")
	if err != nil {
		t.Fatalf("GetStockVideo failed: %v", err)
	}

	if result.MediaId != "cached-video-media-id" {
		t.Errorf("Expected cached media ID, got %s", result.MediaId)
	}

	if result.ProviderImage.Id != "video_12345" {
		t.Errorf("Expected cached provider image ID, got %s", result.ProviderImage.Id)
	}

	// Primary should NOT have been called.
	if mockPrimary.calls != 0 {
		t.Errorf("Expected 0 calls to primary (cache hit), got %d", mockPrimary.calls)
	}
}

// TestCachedStockVideoProvider_CacheMiss_ImageEntry tests that a cached image
// entry (without "video_" prefix) is NOT returned for video searches.
func TestCachedStockVideoProvider_CacheMiss_ImageEntry(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	embeddingDone := setupTestEmbedder(t, sqlStorage)

	ctx := context.Background()

	// Insert a stock entry WITHOUT video_ prefix (an image record).
	stockImage := &models.StockImage{
		Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS,
		ProviderImage: &models.ProviderImage{
			Id:          "12345",
			Description: "skiing at a mountain resort with fresh powder",
			Creator: &models.CreatorInfo{
				Name: "Image Creator",
			},
		},
		MediaId: "cached-image-media-id",
	}
	_, err := sqlStorage.Insert(ctx, stockImage)
	if err != nil {
		t.Fatalf("Failed to insert stock image: %v", err)
	}

	waitForEmbedding(t, embeddingDone)

	mockPrimary := &mockStockVideoProvider{}
	cached := NewStockVideoProviderCached(mockPrimary, sqlStorage, 0.1, FallbackSimilarityThreshold) // Low threshold

	result, err := cached.GetStockVideo(ctx, "skiing mountain resort powder")
	if err != nil {
		t.Fatalf("GetStockVideo failed: %v", err)
	}

	// Should return result from primary, NOT the cached image entry.
	if result.MediaId == "cached-image-media-id" {
		t.Error("Expected cache miss for image entry, but got cached image")
	}

	// Primary should have been called.
	if mockPrimary.calls != 1 {
		t.Errorf("Expected 1 call to primary (cache miss), got %d", mockPrimary.calls)
	}
}

// TestCachedStockVideoProvider_CacheMiss_BelowThreshold tests that
// when similarity is below threshold, primary provider is called.
func TestCachedStockVideoProvider_CacheMiss_BelowThreshold(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	embeddingDone := setupTestEmbedder(t, sqlStorage)

	ctx := context.Background()

	stockVideo := &models.StockImage{
		Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS,
		ProviderImage: &models.ProviderImage{
			Id:          "video_55555",
			Description: "underwater coral reef with tropical fish",
			Creator: &models.CreatorInfo{
				Name: "Video Creator",
			},
		},
		MediaId: "cached-video-media-id",
	}
	_, err := sqlStorage.Insert(ctx, stockVideo)
	if err != nil {
		t.Fatalf("Failed to insert stock video: %v", err)
	}

	waitForEmbedding(t, embeddingDone)

	mockPrimary := &mockStockVideoProvider{}
	cached := NewStockVideoProviderCached(mockPrimary, sqlStorage, 0.95, FallbackSimilarityThreshold) // High threshold

	// Query with very different text.
	result, err := cached.GetStockVideo(ctx, "desert sandstorm cactus")
	if err != nil {
		t.Fatalf("GetStockVideo failed: %v", err)
	}

	if result.MediaId == "cached-video-media-id" {
		t.Error("Expected cache miss but got cached media ID")
	}

	if mockPrimary.calls != 1 {
		t.Errorf("Expected 1 call to primary (cache miss), got %d", mockPrimary.calls)
	}
}

// TestCachedStockVideoProvider_CacheMiss_NoExisting tests that
// when there are no existing stock entries, primary provider is called.
func TestCachedStockVideoProvider_CacheMiss_NoExisting(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	_ = setupTestEmbedder(t, sqlStorage)

	mockPrimary := &mockStockVideoProvider{}
	cached := NewStockVideoProviderCached(mockPrimary, sqlStorage, DefaultSimilarityThreshold, FallbackSimilarityThreshold)

	ctx := context.Background()
	result, err := cached.GetStockVideo(ctx, "skiing mountain resort")
	if err != nil {
		t.Fatalf("GetStockVideo failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if mockPrimary.calls != 1 {
		t.Errorf("Expected 1 call to primary, got %d", mockPrimary.calls)
	}
}

// TestCachedStockVideoProvider_RateLimitFallback tests that when the primary
// provider is rate limited and a cached video exists below threshold, the
// cached video is returned instead of an error.
func TestCachedStockVideoProvider_RateLimitFallback(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	embeddingDone := setupTestEmbedder(t, sqlStorage)

	ctx := context.Background()

	// Insert a video entry with different-enough description to be below threshold.
	stockVideo := &models.StockImage{
		Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS,
		ProviderImage: &models.ProviderImage{
			Id:          "video_77777",
			Description: "hiking in the mountains with beautiful scenery",
			Creator: &models.CreatorInfo{
				Name: "Video Creator",
			},
		},
		MediaId: "cached-video-media-id",
	}
	_, err := sqlStorage.Insert(ctx, stockVideo)
	if err != nil {
		t.Fatalf("Failed to insert stock video: %v", err)
	}

	waitForEmbedding(t, embeddingDone)

	// Primary returns rate limit error.
	mockPrimary := &mockStockVideoProvider{
		err: fmt.Errorf("pexels video API: %w", ErrRateLimited),
	}
	cached := NewStockVideoProviderCached(mockPrimary, sqlStorage, 0.99, FallbackSimilarityThreshold) // Very high threshold — ensures cache miss

	result, err := cached.GetStockVideo(ctx, "hiking mountains scenery trail")
	if err != nil {
		t.Fatalf("GetStockVideo should not fail when rate limited with cached fallback: %v", err)
	}

	if result.MediaId != "cached-video-media-id" {
		t.Errorf("Expected cached video fallback, got media ID %s", result.MediaId)
	}

	if result.ProviderImage.Id != "video_77777" {
		t.Errorf("Expected cached provider image ID video_77777, got %s", result.ProviderImage.Id)
	}

	if mockPrimary.calls != 1 {
		t.Errorf("Expected 1 call to primary, got %d", mockPrimary.calls)
	}
}

// TestCachedStockVideoProvider_RateLimitNoFallback tests that when the primary
// provider is rate limited but no cached video exists, the error propagates.
func TestCachedStockVideoProvider_RateLimitNoFallback(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	_ = setupTestEmbedder(t, sqlStorage)

	// Primary returns rate limit error, no cached entries exist.
	mockPrimary := &mockStockVideoProvider{
		err: fmt.Errorf("pexels video API: %w", ErrRateLimited),
	}
	cached := NewStockVideoProviderCached(mockPrimary, sqlStorage, DefaultSimilarityThreshold, FallbackSimilarityThreshold)

	ctx := context.Background()
	_, err := cached.GetStockVideo(ctx, "skiing mountain resort")
	if err == nil {
		t.Fatal("Expected error when rate limited with no cached fallback")
	}

	if !isRateLimited(err) {
		t.Errorf("Expected rate limited error, got: %v", err)
	}
}

// TestCachedStockVideoProvider_RelevanceGate_AboveThreshold tests that when the
// primary provider is rate limited and a cached video is above the fallback
// similarity threshold, the cached video is returned.
func TestCachedStockVideoProvider_RelevanceGate_AboveThreshold(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	embeddingDone := setupTestEmbedder(t, sqlStorage)

	ctx := context.Background()

	// Insert a video with semantically similar description to the query.
	stockVideo := &models.StockImage{
		Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS,
		ProviderImage: &models.ProviderImage{
			Id:          "video_88888",
			Description: "hiking in the mountains along a scenic trail",
			Creator: &models.CreatorInfo{
				Name: "Video Creator",
			},
		},
		MediaId: "relevant-video-media-id",
	}
	_, err := sqlStorage.Insert(ctx, stockVideo)
	if err != nil {
		t.Fatalf("Failed to insert stock video: %v", err)
	}

	waitForEmbedding(t, embeddingDone)

	// Primary is rate limited.
	mockPrimary := &mockStockVideoProvider{
		err: fmt.Errorf("pexels video API: %w", ErrRateLimited),
	}
	// Very high regular threshold ensures a cache miss; fallback threshold at 0.5
	// means a reasonably relevant cached video should be returned.
	cached := NewStockVideoProviderCached(mockPrimary, sqlStorage, 0.99, FallbackSimilarityThreshold)

	result, err := cached.GetStockVideo(ctx, "mountain hiking trail outdoor")
	if err != nil {
		t.Fatalf("GetStockVideo should not fail: cached video is relevant enough for fallback: %v", err)
	}

	if result.MediaId != "relevant-video-media-id" {
		t.Errorf("Expected relevant cached video fallback, got media ID %s", result.MediaId)
	}
}

// TestCachedStockVideoProvider_RelevanceGate_BelowThreshold tests that when the
// primary provider is rate limited but the best cached video is below the fallback
// similarity threshold, the rate limit error propagates so gen_ai.go can fall back
// to the image APIs instead of showing an irrelevant video.
func TestCachedStockVideoProvider_RelevanceGate_BelowThreshold(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	embeddingDone := setupTestEmbedder(t, sqlStorage)

	ctx := context.Background()

	// Insert a video about a completely unrelated topic.
	stockVideo := &models.StockImage{
		Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS,
		ProviderImage: &models.ProviderImage{
			Id:          "video_99999",
			Description: "underwater coral reef with tropical fish swimming",
			Creator: &models.CreatorInfo{
				Name: "Video Creator",
			},
		},
		MediaId: "irrelevant-video-media-id",
	}
	_, err := sqlStorage.Insert(ctx, stockVideo)
	if err != nil {
		t.Fatalf("Failed to insert stock video: %v", err)
	}

	waitForEmbedding(t, embeddingDone)

	// Primary is rate limited.
	mockPrimary := &mockStockVideoProvider{
		err: fmt.Errorf("pexels video API: %w", ErrRateLimited),
	}
	// Use FallbackSimilarityThreshold (0.5) — coral reef should be far below this
	// for a desert sandstorm query.
	cached := NewStockVideoProviderCached(mockPrimary, sqlStorage, 0.99, FallbackSimilarityThreshold)

	_, err = cached.GetStockVideo(ctx, "desert sandstorm arid landscape")
	if err == nil {
		t.Fatal("Expected error: irrelevant cached video should not be used as fallback")
	}

	if !isRateLimited(err) {
		t.Errorf("Expected rate limited error to propagate, got: %v", err)
	}
}
