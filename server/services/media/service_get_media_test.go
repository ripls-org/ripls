package media

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

func TestService_GetMedia(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{
		SignedURL: "https://example.com/signed-url/media123",
	}
	service := New(testStorage, mockBucket)

	// Pre-populate storage with test data
	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	testMedia := &models.Media{
		UserId:      "user123",
		ContentType: "image/jpeg",
		StorageUrl:  "mock://bucket/media/user123/test-media",
		SizeBytes:   15,
		Filename:    proto.String("test.jpg"),
		Description: proto.String("Test image"),
	}

	mediaID, err := testStorage.Insert(ctx, testMedia)
	if err != nil {
		t.Fatalf("Failed to insert test media: %v", err)
	}

	t.Run("successful retrieval with authentication", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.GetMediaRequest{
			Id: mediaID,
		})

		resp, err := service.GetMedia(ctx, req)
		if err != nil {
			t.Fatalf("GetMedia failed: %v", err)
		}

		if resp.Msg.Id != mediaID {
			t.Errorf("Expected ID %s, got %s", mediaID, resp.Msg.Id)
		}

		if resp.Msg.Filename != testMedia.GetFilename() {
			t.Errorf("Expected filename %s, got %s", testMedia.GetFilename(), resp.Msg.Filename)
		}

		if resp.Msg.Description != testMedia.GetDescription() {
			t.Errorf("Expected description %s, got %s", testMedia.GetDescription(), resp.Msg.Description)
		}

		if resp.Msg.ContentType != "image/jpeg" {
			t.Errorf("Expected content_type image/jpeg, got %s", resp.Msg.ContentType)
		}

		if resp.Msg.Url == "" {
			t.Error("Expected non-empty URL")
		}

		if resp.Msg.Url != mockBucket.SignedURL {
			t.Errorf("Expected URL %s, got %s", mockBucket.SignedURL, resp.Msg.Url)
		}
	})

	t.Run("no authentication", func(t *testing.T) {
		ctx := context.Background()

		req := connect.NewRequest(&api.GetMediaRequest{
			Id: mediaID,
		})

		_, err := service.GetMedia(ctx, req)
		if err == nil {
			t.Fatal("Expected error for unauthenticated request")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("Expected Unauthenticated error, got %v", connectErr.Code())
		}
	})

	t.Run("media not found", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.GetMediaRequest{
			Id: "nonexistent-id",
		})

		_, err := service.GetMedia(ctx, req)
		if err == nil {
			t.Fatal("Expected error for non-existent media")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("Expected NotFound error, got %v", connectErr.Code())
		}
	})

	t.Run("media without stock image has no attribution", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.GetMediaRequest{
			Id: mediaID,
		})

		resp, err := service.GetMedia(ctx, req)
		if err != nil {
			t.Fatalf("GetMedia failed: %v", err)
		}

		if resp.Msg.Attribution != nil {
			t.Errorf("Expected nil attribution for non-stock media, got %+v", resp.Msg.Attribution)
		}
	})

	t.Run("media with stock image source has attribution", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Create a stock image record
		stockImage := &models.StockImage{
			Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH,
			ProviderImage: &models.ProviderImage{
				Id:          "unsplash-photo-123",
				Url:         "https://unsplash.com/photos/abc123",
				Description: "Beautiful landscape",
				Creator: &models.CreatorInfo{
					Name:     "Jane Photographer",
					Username: "janephoto",
				},
			},
			MediaId: "original-stock-media-id",
		}
		stockImageID, err := testStorage.Insert(ctx, stockImage)
		if err != nil {
			t.Fatalf("Failed to insert stock image: %v", err)
		}

		// Create media that references the stock image
		stockDerivedMedia := &models.Media{
			UserId:             "user123",
			ContentType:        "image/jpeg",
			StorageUrl:         "mock://bucket/media/user123/stock-derived",
			SizeBytes:          1024,
			Filename:           proto.String("stock-derived.jpg"),
			Description:        proto.String("Stock image copy"),
			SourceStockImageId: proto.String(stockImageID),
		}
		derivedMediaID, err := testStorage.Insert(ctx, stockDerivedMedia)
		if err != nil {
			t.Fatalf("Failed to insert stock-derived media: %v", err)
		}

		req := connect.NewRequest(&api.GetMediaRequest{
			Id: derivedMediaID,
		})

		resp, err := service.GetMedia(ctx, req)
		if err != nil {
			t.Fatalf("GetMedia failed: %v", err)
		}

		if resp.Msg.Attribution == nil {
			t.Fatal("Expected attribution for stock-derived media")
		}

		if resp.Msg.Attribution.Provider != api.StockImageProvider_STOCK_IMAGE_PROVIDER_UNSPLASH {
			t.Errorf("Expected UNSPLASH provider, got %v", resp.Msg.Attribution.Provider)
		}

		if resp.Msg.Attribution.OriginalUrl != "https://unsplash.com/photos/abc123" {
			t.Errorf("Expected original URL 'https://unsplash.com/photos/abc123', got '%s'", resp.Msg.Attribution.OriginalUrl)
		}

		if resp.Msg.Attribution.CreatorName != "Jane Photographer" {
			t.Errorf("Expected creator name 'Jane Photographer', got '%s'", resp.Msg.Attribution.CreatorName)
		}

		if resp.Msg.Attribution.CreatorUsername != "janephoto" {
			t.Errorf("Expected creator username 'janephoto', got '%s'", resp.Msg.Attribution.CreatorUsername)
		}
	})

	t.Run("media with Pexels stock image has attribution with photographer URL", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Create a Pexels stock image record
		pexelsStockImage := &models.StockImage{
			Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS,
			ProviderImage: &models.ProviderImage{
				Id:          "12345678",
				Url:         "https://www.pexels.com/photo/sunset-beach-12345678/",
				Description: "Beautiful sunset at beach",
				Creator: &models.CreatorInfo{
					Name:            "John Doe",
					Username:        "",
					PhotographerUrl: "https://www.pexels.com/@johndoe",
				},
			},
			MediaId: "original-pexels-media-id",
		}
		pexelsStockImageID, err := testStorage.Insert(ctx, pexelsStockImage)
		if err != nil {
			t.Fatalf("Failed to insert Pexels stock image: %v", err)
		}

		// Create media that references the Pexels stock image
		pexelsDerivedMedia := &models.Media{
			UserId:             "user123",
			ContentType:        "image/jpeg",
			StorageUrl:         "mock://bucket/media/user123/pexels-derived",
			SizeBytes:          2048,
			Filename:           proto.String("pexels-derived.jpg"),
			Description:        proto.String("Pexels stock image copy"),
			SourceStockImageId: proto.String(pexelsStockImageID),
		}
		derivedMediaID, err := testStorage.Insert(ctx, pexelsDerivedMedia)
		if err != nil {
			t.Fatalf("Failed to insert Pexels-derived media: %v", err)
		}

		req := connect.NewRequest(&api.GetMediaRequest{
			Id: derivedMediaID,
		})

		resp, err := service.GetMedia(ctx, req)
		if err != nil {
			t.Fatalf("GetMedia failed: %v", err)
		}

		if resp.Msg.Attribution == nil {
			t.Fatal("Expected attribution for Pexels-derived media")
		}

		if resp.Msg.Attribution.Provider != api.StockImageProvider_STOCK_IMAGE_PROVIDER_PEXELS {
			t.Errorf("Expected PEXELS provider, got %v", resp.Msg.Attribution.Provider)
		}

		if resp.Msg.Attribution.OriginalUrl != "https://www.pexels.com/photo/sunset-beach-12345678/" {
			t.Errorf("Expected original URL 'https://www.pexels.com/photo/sunset-beach-12345678/', got '%s'", resp.Msg.Attribution.OriginalUrl)
		}

		if resp.Msg.Attribution.CreatorName != "John Doe" {
			t.Errorf("Expected creator name 'John Doe', got '%s'", resp.Msg.Attribution.CreatorName)
		}

		if resp.Msg.Attribution.PhotographerUrl != "https://www.pexels.com/@johndoe" {
			t.Errorf("Expected photographer URL 'https://www.pexels.com/@johndoe', got '%s'", resp.Msg.Attribution.PhotographerUrl)
		}

		// Pexels doesn't have username, should be empty
		if resp.Msg.Attribution.CreatorUsername != "" {
			t.Errorf("Expected empty creator username for Pexels, got '%s'", resp.Msg.Attribution.CreatorUsername)
		}
	})

	// Issue #1529 acceptance criterion: a caller who is neither the owner
	// nor reaches the media through any surface (community, story, gear,
	// experience, request, chat, or transfer) is denied at the RPC layer.
	// testMedia is owned by user123 and is not surfaced anywhere, so a
	// stranger has no legitimate path to it.
	t.Run("non-participant denied", func(t *testing.T) {
		ctxStranger := createAuthenticatedContext("stranger999", "stranger999@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.GetMediaRequest{
			Id: mediaID,
		})

		_, err := service.GetMedia(ctxStranger, req)
		if err == nil {
			t.Fatal("Expected PermissionDenied when a non-participant reads another user's media")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", connectErr.Code())
		}
	})
}
