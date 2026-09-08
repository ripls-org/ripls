package media

import (
	"context"
	"fmt"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// providerTestCase defines a test case for stock imagery providers.
type providerTestCase struct {
	name     string
	provider func(t *testing.T, sqlStorage *storage.ProtoSQLStorage, bucket storage.BucketStorage) StockImageryProvider
}

// getTestProviders returns all providers to test.
// Each provider is instantiated fresh for each test.
func getTestProviders() []providerTestCase {
	return []providerTestCase{
		{
			name: "UnsplashProvider",
			provider: func(t *testing.T, sqlStorage *storage.ProtoSQLStorage, bucket storage.BucketStorage) StockImageryProvider {
				// Use a mock client for testing
				mockClient := newMockUnsplashClient()
				return &UnsplashProvider{
					client:  mockClient,
					storage: sqlStorage,
					bucket:  bucket,
				}
			},
		},
	}
}

// mockUnsplashClient is a mock implementation of UnsplashClient for testing.
type mockUnsplashClient struct {
	searchResponse *SearchResponse
	downloadData   []byte
	searchError    error
	downloadError  error

	// getPhotoByID maps photoID → response for GetPhoto. Tests that
	// exercise the by-id path (#2063) populate this; tests that don't
	// can leave it nil and any GetPhoto call returns an error.
	getPhotoByID map[string]*Photo
	getPhotoErr  error
}

func newMockUnsplashClient() *mockUnsplashClient {
	return &mockUnsplashClient{
		searchResponse: &SearchResponse{
			Results: []Photo{
				{
					ID:             "photo123",
					Description:    "A beautiful mountain landscape",
					AltDescription: "Mountain with snow",
					URLs: PhotoURLs{
						Full:    "https://images.unsplash.com/photo123?w=2400",
						Regular: "https://images.unsplash.com/photo123?w=1080",
					},
					User: PhotoUser{
						Name:     "John Photographer",
						Username: "johnphoto",
					},
				},
				{
					ID:             "photo456",
					Description:    "Ocean sunset",
					AltDescription: "Sun setting over the ocean",
					URLs: PhotoURLs{
						Full:    "https://images.unsplash.com/photo456?w=2400",
						Regular: "https://images.unsplash.com/photo456?w=1080",
					},
					User: PhotoUser{
						Name:     "Jane Artist",
						Username: "janeart",
					},
				},
			},
		},
		downloadData: fakeJPEGData,
	}
}

// SearchPhotos implements the search interface for the mock.
func (m *mockUnsplashClient) SearchPhotos(ctx context.Context, query string, perPage int) (*SearchResponse, error) {
	if m.searchError != nil {
		return nil, m.searchError
	}
	if m.searchResponse == nil {
		return &SearchResponse{Results: []Photo{}}, nil
	}
	return m.searchResponse, nil
}

// DownloadPhoto implements the download interface for the mock.
func (m *mockUnsplashClient) DownloadPhoto(ctx context.Context, photo *Photo) ([]byte, error) {
	if m.downloadError != nil {
		return nil, m.downloadError
	}
	return m.downloadData, nil
}

// GetPhoto implements the by-id lookup for the mock. Tests that need
// the by-id path populate getPhotoByID; otherwise unrecognised ids
// return a not-found-style error.
func (m *mockUnsplashClient) GetPhoto(ctx context.Context, photoID string) (*Photo, error) {
	if m.getPhotoErr != nil {
		return nil, m.getPhotoErr
	}
	if p, ok := m.getPhotoByID[photoID]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("mock unsplash: photo %q not found", photoID)
}

// fakeJPEGData is a minimal valid JPEG for testing.
var fakeJPEGData = []byte{
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

func setupTestStorage(t *testing.T) (*storage.ProtoSQLStorage, storage.BucketStorage) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	tmpDir := t.TempDir()
	bucket, err := storage.NewLocalBucketStorage(tmpDir, "http://localhost:8080")
	if err != nil {
		t.Fatalf("Failed to create bucket storage: %v", err)
	}

	return sqlStorage, bucket
}

// TestStockImageryProvider_GetStockImage tests the basic GetStockImage functionality.
func TestStockImageryProvider_GetStockImage(t *testing.T) {
	for _, tc := range getTestProviders() {
		t.Run(tc.name, func(t *testing.T) {
			sqlStorage, bucket := setupTestStorage(t)
			provider := tc.provider(t, sqlStorage, bucket)
			ctx := context.Background()

			result, err := provider.GetStockImage(ctx, "mountain landscape", nil)
			if err != nil {
				t.Fatalf("GetStockImage failed: %v", err)
			}

			if result.MediaId == "" {
				t.Error("Expected non-empty media ID")
			}

			if result.Id == "" {
				t.Error("Expected non-empty stock image ID")
			}

			if result.ProviderImage == nil {
				t.Fatal("Expected non-nil ProviderImage")
			}

			if result.ProviderImage.Description == "" && result.ProviderImage.AltDescription == "" {
				t.Error("Expected at least one description field to be set")
			}

			// Verify media was stored
			media := &models.Media{}
			if err := sqlStorage.GetByID(ctx, result.MediaId, media); err != nil {
				t.Fatalf("Failed to get stored media: %v", err)
			}

			if media.ContentType != "image/jpeg" {
				t.Errorf("Expected content type image/jpeg, got %s", media.ContentType)
			}
		})
	}
}

// TestStockImageryProvider_CreatesStockImageRecord tests that a StockImage record is created.
func TestStockImageryProvider_CreatesStockImageRecord(t *testing.T) {
	for _, tc := range getTestProviders() {
		t.Run(tc.name, func(t *testing.T) {
			sqlStorage, bucket := setupTestStorage(t)
			provider := tc.provider(t, sqlStorage, bucket)
			ctx := context.Background()

			result, err := provider.GetStockImage(ctx, "test query", nil)
			if err != nil {
				t.Fatalf("GetStockImage failed: %v", err)
			}

			// Query for the stock image record by media_id
			stockImages, err := sqlStorage.QueryByField(ctx, "media_id", result.MediaId, &models.StockImage{})
			if err != nil {
				t.Fatalf("Failed to query stock images: %v", err)
			}

			if len(stockImages) != 1 {
				t.Fatalf("Expected 1 stock image record, got %d", len(stockImages))
			}

			stockImage := stockImages[0].(*models.StockImage)
			if stockImage.ProviderImage == nil {
				t.Fatal("Expected non-nil ProviderImage in stock image record")
			}

			if stockImage.ProviderImage.Id == "" {
				t.Error("Expected non-empty provider image ID")
			}
		})
	}
}

// TestStockImageryProvider_RepeatedQueryReturnsSameImage tests that repeated queries
// for the same text return the same provider image when uniqueness is not required.
func TestStockImageryProvider_RepeatedQueryReturnsSameImage(t *testing.T) {
	for _, tc := range getTestProviders() {
		t.Run(tc.name, func(t *testing.T) {
			sqlStorage, bucket := setupTestStorage(t)
			provider := tc.provider(t, sqlStorage, bucket)
			ctx := context.Background()

			// First call
			result1, err := provider.GetStockImage(ctx, "mountain landscape", nil)
			if err != nil {
				t.Fatalf("First GetStockImage failed: %v", err)
			}

			// Second call with same query (no RequireUnique)
			result2, err := provider.GetStockImage(ctx, "mountain landscape", nil)
			if err != nil {
				t.Fatalf("Second GetStockImage failed: %v", err)
			}

			// Without RequireUnique, both calls should return the same provider image ID
			// (the first result from the search)
			if result1.ProviderImage.Id != result2.ProviderImage.Id {
				t.Errorf("Expected same provider image ID without RequireUnique, got %s and %s",
					result1.ProviderImage.Id, result2.ProviderImage.Id)
			}

			// The media IDs should also be the same (provider should reuse existing StockImage)
			if result1.MediaId != result2.MediaId {
				t.Errorf("Expected same media ID without RequireUnique, got %s and %s",
					result1.MediaId, result2.MediaId)
			}

			// The stock image IDs should also be the same
			if result1.Id != result2.Id {
				t.Errorf("Expected same stock image ID without RequireUnique, got %s and %s",
					result1.Id, result2.Id)
			}
		})
	}
}

// TestStockImageryProvider_RequireUnique tests the uniqueness option.
func TestStockImageryProvider_RequireUnique(t *testing.T) {
	for _, tc := range getTestProviders() {
		t.Run(tc.name, func(t *testing.T) {
			sqlStorage, bucket := setupTestStorage(t)
			provider := tc.provider(t, sqlStorage, bucket)
			ctx := context.Background()

			// First call without RequireUnique
			result1, err := provider.GetStockImage(ctx, "test query", nil)
			if err != nil {
				t.Fatalf("First GetStockImage failed: %v", err)
			}

			// Second call with RequireUnique should return different image
			result2, err := provider.GetStockImage(ctx, "test query", &StockImageOptions{RequireUnique: true})
			if err != nil {
				t.Fatalf("Second GetStockImage failed: %v", err)
			}

			// The provider image IDs should be different
			if result1.ProviderImage.Id == result2.ProviderImage.Id {
				t.Errorf("Expected different provider image IDs with RequireUnique, got same: %s", result1.ProviderImage.Id)
			}
		})
	}
}

// TestStockImageryProvider_ProviderImageMetadata tests that provider metadata is captured.
func TestStockImageryProvider_ProviderImageMetadata(t *testing.T) {
	for _, tc := range getTestProviders() {
		t.Run(tc.name, func(t *testing.T) {
			sqlStorage, bucket := setupTestStorage(t)
			provider := tc.provider(t, sqlStorage, bucket)
			ctx := context.Background()

			result, err := provider.GetStockImage(ctx, "test", nil)
			if err != nil {
				t.Fatalf("GetStockImage failed: %v", err)
			}

			pi := result.ProviderImage
			if pi.Id == "" {
				t.Error("Expected non-empty provider image ID")
			}

			if pi.Url == "" {
				t.Error("Expected non-empty provider URL")
			}

			if pi.Creator == nil {
				t.Fatal("Expected non-nil Creator")
			}

			if pi.Creator.Name == "" {
				t.Error("Expected non-empty creator name")
			}
		})
	}
}

// TestStoreMedia_SourceStockImageID tests that source_stock_image_id is correctly set on media.
func TestStoreMedia_SourceStockImageID(t *testing.T) {
	sqlStorage, bucket := setupTestStorage(t)
	ctx := context.Background()

	t.Run("media with source_stock_image_id", func(t *testing.T) {
		stockImageID := "test-stock-image-id"
		mediaID, err := storage.StoreMedia(
			ctx,
			sqlStorage,
			bucket,
			"test-user",
			fakeJPEGData,
			"image/jpeg",
			"test.jpg",
			"Test image",
			stockImageID,
			false, // stripMetadata
		)
		if err != nil {
			t.Fatalf("StoreMedia failed: %v", err)
		}

		// Verify media was stored with source_stock_image_id
		media := &models.Media{}
		if err := sqlStorage.GetByID(ctx, mediaID, media); err != nil {
			t.Fatalf("Failed to get media: %v", err)
		}

		if media.GetSourceStockImageId() != stockImageID {
			t.Errorf("Expected source_stock_image_id %q, got %q", stockImageID, media.GetSourceStockImageId())
		}
	})

	t.Run("media without source_stock_image_id", func(t *testing.T) {
		mediaID, err := storage.StoreMedia(
			ctx,
			sqlStorage,
			bucket,
			"test-user",
			fakeJPEGData,
			"image/jpeg",
			"test2.jpg",
			"Test image 2",
			"",    // No stock image source
			false, // stripMetadata
		)
		if err != nil {
			t.Fatalf("StoreMedia failed: %v", err)
		}

		// Verify media was stored without source_stock_image_id
		media := &models.Media{}
		if err := sqlStorage.GetByID(ctx, mediaID, media); err != nil {
			t.Fatalf("Failed to get media: %v", err)
		}

		if media.GetSourceStockImageId() != "" {
			t.Errorf("Expected empty source_stock_image_id, got %q", media.GetSourceStockImageId())
		}
	})
}

// TestStockImageAttribution tests the attribution helper.
func TestStockImageAttribution(t *testing.T) {
	stockImage := &models.StockImage{
		Id:       "stock-image-123",
		MediaId:  "media123",
		Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH,
		ProviderImage: &models.ProviderImage{
			Id:          "photo123",
			Description: "Test photo",
			Creator: &models.CreatorInfo{
				Name:     "Jane Doe",
				Username: "janedoe",
			},
		},
	}

	t.Run("Unsplash attribution", func(t *testing.T) {
		attribution := StockImageAttribution(stockImage)
		expected := "Photo by Jane Doe on Unsplash"
		if attribution != expected {
			t.Errorf("Expected %q, got %q", expected, attribution)
		}
	})

	t.Run("Pexels attribution", func(t *testing.T) {
		pexelsStockImage := &models.StockImage{
			Id:       "stock-image-456",
			MediaId:  "media456",
			Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS,
			ProviderImage: &models.ProviderImage{
				Id:          "photo456",
				Description: "Test photo",
				Creator: &models.CreatorInfo{
					Name:     "Bob Smith",
					Username: "bobsmith",
				},
			},
		}
		attribution := StockImageAttribution(pexelsStockImage)
		expected := "Photo by Bob Smith on Pexels"
		if attribution != expected {
			t.Errorf("Expected %q, got %q", expected, attribution)
		}
	})

	t.Run("Fake provider attribution", func(t *testing.T) {
		fakeStockImage := &models.StockImage{
			Id:       "stock-image-789",
			MediaId:  "media789",
			Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_FAKE,
			ProviderImage: &models.ProviderImage{
				Id:          "photo789",
				Description: "Test photo",
				Creator: &models.CreatorInfo{
					Name:     "Jane Doe",
					Username: "janedoe",
				},
			},
		}
		attribution := StockImageAttribution(fakeStockImage)
		expected := "Photo by Jane Doe on Test"
		if attribution != expected {
			t.Errorf("Expected %q, got %q", expected, attribution)
		}
	})

	t.Run("nil creator returns empty", func(t *testing.T) {
		nilCreatorImage := &models.StockImage{
			Id:            "stock-image-789",
			MediaId:       "media789",
			Provider:      models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH,
			ProviderImage: &models.ProviderImage{},
		}
		attribution := StockImageAttribution(nilCreatorImage)
		if attribution != "" {
			t.Errorf("Expected empty attribution, got %q", attribution)
		}
	})

	t.Run("nil stock image returns empty", func(t *testing.T) {
		attribution := StockImageAttribution(nil)
		if attribution != "" {
			t.Errorf("Expected empty attribution, got %q", attribution)
		}
	})
}

// TestGetStockImageById tests the stock image lookup function.
func TestGetStockImageById(t *testing.T) {
	sqlStorage, _ := setupTestStorage(t)
	ctx := context.Background()

	t.Run("returns stock image when found", func(t *testing.T) {
		// Create a stock image
		stockImage := &models.StockImage{
			Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH,
			ProviderImage: &models.ProviderImage{
				Id:          "photo-abc",
				Url:         "https://unsplash.com/photos/abc",
				Description: "Test photo",
				Creator: &models.CreatorInfo{
					Name:     "Test Creator",
					Username: "testcreator",
				},
			},
			MediaId: "media-123",
		}
		stockImageID, err := sqlStorage.Insert(ctx, stockImage)
		if err != nil {
			t.Fatalf("Failed to insert stock image: %v", err)
		}

		// Look it up
		result, err := GetStockImageByID(ctx, sqlStorage, stockImageID)
		if err != nil {
			t.Fatalf("GetStockImageByID failed: %v", err)
		}

		if result.Id != stockImageID {
			t.Errorf("Expected ID %s, got %s", stockImageID, result.Id)
		}

		if result.Provider != models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH {
			t.Errorf("Expected UNSPLASH provider, got %v", result.Provider)
		}

		if result.ProviderImage == nil {
			t.Fatal("Expected non-nil ProviderImage")
		}

		if result.ProviderImage.Url != "https://unsplash.com/photos/abc" {
			t.Errorf("Expected URL 'https://unsplash.com/photos/abc', got %s", result.ProviderImage.Url)
		}

		if result.ProviderImage.Creator == nil {
			t.Fatal("Expected non-nil Creator")
		}

		if result.ProviderImage.Creator.Name != "Test Creator" {
			t.Errorf("Expected creator name 'Test Creator', got %s", result.ProviderImage.Creator.Name)
		}
	})

	t.Run("returns error when not found", func(t *testing.T) {
		_, err := GetStockImageByID(ctx, sqlStorage, "nonexistent-id")
		if err == nil {
			t.Fatal("Expected error for nonexistent stock image")
		}
	})
}

// TestUnsplashProvider_GetStockImageByID exercises the candidate-import
// path that PR #2062 stubbed and #2063 implements. Three cases mirror
// the Pexels equivalent.
func TestUnsplashProvider_GetStockImageByID(t *testing.T) {
	t.Run("dedupe: existing StockImage returned without DownloadPhoto", func(t *testing.T) {
		sqlStorage, bucket := setupTestStorage(t)
		ctx := context.Background()

		// Seed a canonical StockImage row keyed on photoID "abc123".
		seedMedia := &models.Media{
			UserId:           SystemUserID,
			ContentType:      "image/jpeg",
			SizeBytes:        int64(len(fakeJPEGData)),
			CreatedAtUnixSec: 1,
		}
		mediaID, err := sqlStorage.Insert(ctx, seedMedia)
		if err != nil {
			t.Fatalf("seed media: %v", err)
		}
		seedStock := &models.StockImage{
			Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH,
			ProviderImage: &models.ProviderImage{
				Id:  "abc123",
				Url: "https://unsplash.com/photos/abc123",
			},
			MediaId:          mediaID,
			CreatedAtUnixSec: 1,
		}
		stockID, err := sqlStorage.Insert(ctx, seedStock)
		if err != nil {
			t.Fatalf("seed stock image: %v", err)
		}

		mockClient := newMockUnsplashClient()
		// If GetPhoto fires, the dedupe didn't kick in — fail loudly.
		mockClient.getPhotoErr = fmt.Errorf("GetPhoto should not have been called on dedupe hit")
		mockClient.downloadError = fmt.Errorf("DownloadPhoto should not have been called on dedupe hit")
		provider := &UnsplashProvider{client: mockClient, storage: sqlStorage, bucket: bucket}

		result, err := provider.GetStockImageByID(ctx, "abc123")
		if err != nil {
			t.Fatalf("GetStockImageByID: %v", err)
		}
		if result.Id != stockID {
			t.Errorf("expected dedupe to return id=%s, got %s", stockID, result.Id)
		}
		if result.MediaId != mediaID {
			t.Errorf("expected dedupe to return media_id=%s, got %s", mediaID, result.MediaId)
		}
	})

	t.Run("fresh: GetPhoto + store creates a new canonical StockImage", func(t *testing.T) {
		sqlStorage, bucket := setupTestStorage(t)
		ctx := context.Background()

		mockClient := newMockUnsplashClient()
		mockClient.getPhotoByID = map[string]*Photo{
			"new-photo-1": {
				ID:          "new-photo-1",
				Description: "Mountain at dawn",
				URLs: PhotoURLs{
					Full:    "https://images.unsplash.com/new-photo-1/full",
					Regular: "https://images.unsplash.com/new-photo-1/regular",
				},
				User: PhotoUser{Name: "Alice Photographer", Username: "alice"},
			},
		}
		provider := &UnsplashProvider{client: mockClient, storage: sqlStorage, bucket: bucket}

		result, err := provider.GetStockImageByID(ctx, "new-photo-1")
		if err != nil {
			t.Fatalf("GetStockImageByID: %v", err)
		}
		if result.Id == "" || result.MediaId == "" {
			t.Errorf("expected non-empty ids, got Id=%q MediaId=%q", result.Id, result.MediaId)
		}
		if result.Provider != models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH {
			t.Errorf("provider = %v, want UNSPLASH", result.Provider)
		}
		if result.ProviderImage == nil {
			t.Fatal("expected non-nil ProviderImage")
		}
		if result.ProviderImage.Id != "new-photo-1" {
			t.Errorf("ProviderImage.Id = %q", result.ProviderImage.Id)
		}
		if result.ProviderImage.Url != "https://images.unsplash.com/new-photo-1/full" {
			t.Errorf("ProviderImage.Url = %q", result.ProviderImage.Url)
		}
		if result.ProviderImage.Creator == nil || result.ProviderImage.Creator.Name != "Alice Photographer" {
			t.Errorf("ProviderImage.Creator = %+v", result.ProviderImage.Creator)
		}

		// Second call should dedupe (no new row inserted).
		result2, err := provider.GetStockImageByID(ctx, "new-photo-1")
		if err != nil {
			t.Fatalf("second GetStockImageByID: %v", err)
		}
		if result2.Id != result.Id {
			t.Errorf("dedupe failed: first id=%s second id=%s", result.Id, result2.Id)
		}
	})

	t.Run("GetPhoto error surfaces as non-nil error", func(t *testing.T) {
		sqlStorage, bucket := setupTestStorage(t)
		ctx := context.Background()

		mockClient := newMockUnsplashClient()
		mockClient.getPhotoErr = fmt.Errorf("unsplash photo lookup: status 404")
		provider := &UnsplashProvider{client: mockClient, storage: sqlStorage, bucket: bucket}

		_, err := provider.GetStockImageByID(ctx, "missing-photo")
		if err == nil {
			t.Fatal("expected error when GetPhoto fails, got nil")
		}
		// Non-nil error is what FallbackStockImageryProvider.GetStockImageByID
		// needs to see so it moves on to the next provider in the chain
		// (or, when there are no more providers, surfaces to the caller
		// which falls back to the generic-URL import path).
	})
}
