package media

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/storage"
)

// mockStockImageryProvider tracks calls for testing the cache wrapper.
type mockStockImageryProvider struct {
	calls   []mockProviderCall
	results map[string]*models.StockImage
	err     error
}

type mockProviderCall struct {
	query string
	opts  *StockImageOptions
}

func newMockStockImageryProvider() *mockStockImageryProvider {
	return &mockStockImageryProvider{
		results: make(map[string]*models.StockImage),
	}
}

func (m *mockStockImageryProvider) SetResult(query string, result *models.StockImage) {
	m.results[query] = result
}

func (m *mockStockImageryProvider) GetStockImage(ctx context.Context, query string, opts *StockImageOptions) (*models.StockImage, error) {
	m.calls = append(m.calls, mockProviderCall{query: query, opts: opts})
	if m.err != nil {
		return nil, m.err
	}
	if result, ok := m.results[query]; ok {
		return result, nil
	}
	// Default result
	return &models.StockImage{
		Id:       "default-stock-image-id",
		MediaId:  "default-media-id",
		Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_FAKE,
		ProviderImage: &models.ProviderImage{
			Id:          "default-provider-image-id",
			Description: query,
			Creator: &models.CreatorInfo{
				Name:     "Test Creator",
				Username: "testcreator",
			},
		},
	}, nil
}

func (m *mockStockImageryProvider) CallCount() int {
	return len(m.calls)
}

func (m *mockStockImageryProvider) LastCall() *mockProviderCall {
	if len(m.calls) == 0 {
		return nil
	}
	return &m.calls[len(m.calls)-1]
}

func (m *mockStockImageryProvider) SearchStockImageCandidates(_ context.Context, _ string, _ int) ([]StockImageCandidate, error) {
	return nil, nil
}

func (m *mockStockImageryProvider) GetStockImageByID(_ context.Context, _ string) (*models.StockImage, error) {
	return nil, fmt.Errorf("not supported")
}

func (m *mockStockImageryProvider) CheckHealth(_ context.Context) ([]*health.Status, error) {
	return []*health.Status{{Name: "stock_imagery", Backend: "mock"}}, nil
}

// TestCachedStockImageryProvider_NoEmbedderDelegatesToPrimary tests that
// when no embedder is configured, all requests go to the primary provider.
func TestCachedStockImageryProvider_NoEmbedderDelegatesToPrimary(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	mockPrimary := newMockStockImageryProvider()
	cached := NewCachedStockImageryProvider(mockPrimary, sqlStorage, DefaultSimilarityThreshold)

	ctx := context.Background()
	result, err := cached.GetStockImage(ctx, "mountain landscape", nil)
	if err != nil {
		t.Fatalf("GetStockImage failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if mockPrimary.CallCount() != 1 {
		t.Errorf("Expected 1 call to primary, got %d", mockPrimary.CallCount())
	}
}

// TestCachedStockImageryProvider_RequireUniqueBypassesCache tests that
// RequireUnique=true always delegates to the primary provider.
func TestCachedStockImageryProvider_RequireUniqueBypassesCache(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	mockPrimary := newMockStockImageryProvider()
	cached := NewCachedStockImageryProvider(mockPrimary, sqlStorage, DefaultSimilarityThreshold)

	ctx := context.Background()
	opts := &StockImageOptions{RequireUnique: true}
	result, err := cached.GetStockImage(ctx, "unique query", opts)
	if err != nil {
		t.Fatalf("GetStockImage failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if mockPrimary.CallCount() != 1 {
		t.Errorf("Expected 1 call to primary, got %d", mockPrimary.CallCount())
	}

	lastCall := mockPrimary.LastCall()
	if lastCall.opts == nil || !lastCall.opts.RequireUnique {
		t.Error("Expected RequireUnique to be passed through to primary")
	}
}

// TestCachedStockImageryProvider_CacheHit tests that a cached image is returned
// when similarity is above threshold.
func TestCachedStockImageryProvider_CacheHit(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	// Set up embedder for semantic search
	embeddingDone := setupTestEmbedder(t, sqlStorage)

	ctx := context.Background()

	// Create a stock image with description that matches our query
	stockImage := &models.StockImage{
		Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_FAKE,
		ProviderImage: &models.ProviderImage{
			Id:          "cached-photo-id",
			Description: "beautiful mountain landscape",
			Creator: &models.CreatorInfo{
				Name:     "Cached Creator",
				Username: "cachedcreator",
			},
		},
		MediaId: "cached-media-id",
	}
	_, err := sqlStorage.Insert(ctx, stockImage)
	if err != nil {
		t.Fatalf("Failed to insert stock image: %v", err)
	}

	// Wait for embedding to be generated
	waitForEmbedding(t, embeddingDone)

	// Create cached provider with low threshold to ensure hit
	mockPrimary := newMockStockImageryProvider()
	cached := NewCachedStockImageryProvider(mockPrimary, sqlStorage, 0.1) // Low threshold

	// Query with similar text
	result, err := cached.GetStockImage(ctx, "mountain landscape scenery", nil)
	if err != nil {
		t.Fatalf("GetStockImage failed: %v", err)
	}

	// Should return cached result
	if result.MediaId != "cached-media-id" {
		t.Errorf("Expected cached media ID, got %s", result.MediaId)
	}

	if result.ProviderImage.Id != "cached-photo-id" {
		t.Errorf("Expected cached provider image ID, got %s", result.ProviderImage.Id)
	}

	// Primary should NOT have been called
	if mockPrimary.CallCount() != 0 {
		t.Errorf("Expected 0 calls to primary (cache hit), got %d", mockPrimary.CallCount())
	}
}

// TestCachedStockImageryProvider_CacheMiss_BelowThreshold tests that
// when similarity is below threshold, primary provider is called.
func TestCachedStockImageryProvider_CacheMiss_BelowThreshold(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	// Set up embedder for semantic search
	embeddingDone := setupTestEmbedder(t, sqlStorage)

	ctx := context.Background()

	// Create a stock image with very different description
	stockImage := &models.StockImage{
		Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_FAKE,
		ProviderImage: &models.ProviderImage{
			Id:          "cached-photo-id",
			Description: "underwater coral reef with colorful fish",
			Creator: &models.CreatorInfo{
				Name:     "Cached Creator",
				Username: "cachedcreator",
			},
		},
		MediaId: "cached-media-id",
	}
	_, err := sqlStorage.Insert(ctx, stockImage)
	if err != nil {
		t.Fatalf("Failed to insert stock image: %v", err)
	}

	// Wait for embedding to be generated
	waitForEmbedding(t, embeddingDone)

	// Create cached provider with high threshold
	mockPrimary := newMockStockImageryProvider()
	cached := NewCachedStockImageryProvider(mockPrimary, sqlStorage, 0.95) // High threshold

	// Query with very different text - should not match coral reef description
	result, err := cached.GetStockImage(ctx, "snowy mountain peak", nil)
	if err != nil {
		t.Fatalf("GetStockImage failed: %v", err)
	}

	// Should return result from primary, not cache
	if result.MediaId == "cached-media-id" {
		t.Error("Expected cache miss but got cached media ID")
	}

	// Primary should have been called
	if mockPrimary.CallCount() != 1 {
		t.Errorf("Expected 1 call to primary (cache miss), got %d", mockPrimary.CallCount())
	}
}

// TestCachedStockImageryProvider_CacheMiss_NoExistingImages tests that
// when there are no existing stock images, primary provider is called.
func TestCachedStockImageryProvider_CacheMiss_NoExistingImages(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	// Set up embedder for semantic search
	_ = setupTestEmbedder(t, sqlStorage)

	mockPrimary := newMockStockImageryProvider()
	cached := NewCachedStockImageryProvider(mockPrimary, sqlStorage, DefaultSimilarityThreshold)

	ctx := context.Background()
	result, err := cached.GetStockImage(ctx, "mountain landscape", nil)
	if err != nil {
		t.Fatalf("GetStockImage failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Primary should have been called since no cached images exist
	if mockPrimary.CallCount() != 1 {
		t.Errorf("Expected 1 call to primary, got %d", mockPrimary.CallCount())
	}
}

// setupTestEmbedder configures the test storage with an embedder.
// Returns a channel that receives the embedding outcome (nil=success, non-nil=error).
func setupTestEmbedder(t *testing.T, sqlStorage *storage.ProtoSQLStorage) chan error {
	t.Helper()

	embedder := storage.SetupTestEmbedder(t)
	if embedder == nil {
		t.Skip("Embedder not available - skipping test that requires embeddings")
	}

	// Set up the embedding done channel for synchronization
	done := make(chan error, 10)
	sqlStorage.SetEmbeddingDoneChannel(done)

	ctx := context.Background()
	if err := sqlStorage.SetEmbedder(ctx, embedder); err != nil {
		t.Fatalf("Failed to set embedder: %v", err)
	}

	return done
}

// waitForEmbedding waits for an async embedding to complete and fails the test if it errored.
func waitForEmbedding(t *testing.T, done chan error) {
	t.Helper()

	select {
	case embErr := <-done:
		if embErr != nil {
			t.Fatalf("embedding generation failed: %v", embErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for embedding generation")
	}
}
