package media

import (
	"context"
	"os"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

func TestService_AddMedia(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	t.Run("successful addition with JPEG image", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.AddMediaRequest{
			EncodedBytes: testJPEGBytes,
			ContentType:  "image/jpeg",
			Filename:     "test.jpg",
			Description:  "A test JPEG image",
		})

		resp, err := service.AddMedia(ctx, req)
		if err != nil {
			t.Fatalf("AddMedia failed: %v", err)
		}

		if resp.Msg.Id == "" {
			t.Error("Expected non-empty ID")
		}

		// Verify media was stored
		stored := &models.Media{}
		err = testStorage.GetByID(ctx, resp.Msg.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored media: %v", err)
		}

		if stored.GetFilename() != "test.jpg" {
			t.Errorf("Expected filename test.jpg, got %s", stored.GetFilename())
		}

		if stored.GetDescription() != "A test JPEG image" {
			t.Errorf("Expected description 'A test JPEG image', got %s", stored.GetDescription())
		}

		// StoreMedia sanitizes (re-encodes) the image, so the stored size differs
		// from the input length; just assert it recorded a positive size.
		if stored.SizeBytes <= 0 {
			t.Errorf("Expected positive size_bytes, got %d", stored.SizeBytes)
		}

		if stored.UserId != "user123" {
			t.Errorf("Expected user_id user123, got %s", stored.UserId)
		}

		if stored.ContentType != "image/jpeg" {
			t.Errorf("Expected content_type image/jpeg, got %s", stored.ContentType)
		}

		if stored.StorageUrl == "" {
			t.Error("Expected non-empty storage_url")
		}

		// Verify bucket Put was called
		if !mockBucket.PutCalled {
			t.Error("Expected bucket Put to be called")
		}
	})

	t.Run("HEIC upload is transcoded to JPEG for web compatibility", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		mockBucket.PutCalled = false

		// HEIC doesn't render outside Safari, so the upload path always transcodes
		// it to JPEG (#2210) — independent of the default-off metadata stripping.
		// Use the real fixture; the transcode must decode it.
		heic, err := os.ReadFile("../../test_data/example.heic")
		if err != nil {
			t.Fatalf("read heic fixture: %v", err)
		}
		req := connect.NewRequest(&api.AddMediaRequest{
			EncodedBytes: heic,
			ContentType:  "image/heic",
			Filename:     "test.heic",
			Description:  "A test HEIC image",
		})

		resp, err := service.AddMedia(ctx, req)
		if err != nil {
			t.Fatalf("AddMedia failed: %v", err)
		}
		if resp.Msg.Id == "" {
			t.Error("Expected non-empty ID")
		}

		stored := &models.Media{}
		if err := testStorage.GetByID(ctx, resp.Msg.Id, stored); err != nil {
			t.Fatalf("Failed to retrieve stored media: %v", err)
		}
		if stored.ContentType != "image/jpeg" {
			t.Errorf("HEIC should be stored as image/jpeg, got %s", stored.ContentType)
		}
	})

	t.Run("successful addition with MP4 video", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		mockBucket.PutCalled = false // Reset for this test

		req := connect.NewRequest(&api.AddMediaRequest{
			EncodedBytes: testMP4Bytes,
			ContentType:  "video/mp4",
			Filename:     "test.mp4",
			Description:  "A test MP4 video",
		})

		resp, err := service.AddMedia(ctx, req)
		if err != nil {
			t.Fatalf("AddMedia failed: %v", err)
		}

		if resp.Msg.Id == "" {
			t.Error("Expected non-empty ID")
		}

		// Verify video metadata was stored
		stored := &models.Media{}
		err = testStorage.GetByID(ctx, resp.Msg.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored media: %v", err)
		}

		if stored.ContentType != "video/mp4" {
			t.Errorf("Expected content_type video/mp4, got %s", stored.ContentType)
		}

		if stored.UserId != "user123" {
			t.Errorf("Expected user_id user123, got %s", stored.UserId)
		}
	})

	t.Run("user_id is set correctly for different users", func(t *testing.T) {
		// Test with user456
		ctx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.AddMediaRequest{
			EncodedBytes: testJPEGBytes,
			ContentType:  "image/jpeg",
			Filename:     "user456.jpg",
		})

		resp, err := service.AddMedia(ctx, req)
		if err != nil {
			t.Fatalf("AddMedia failed: %v", err)
		}

		// Verify media was stored with correct user_id
		stored := &models.Media{}
		err = testStorage.GetByID(ctx, resp.Msg.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored media: %v", err)
		}

		if stored.UserId != "user456" {
			t.Errorf("Expected user_id user456, got %s", stored.UserId)
		}
	})

	t.Run("no authentication", func(t *testing.T) {
		ctx := context.Background()

		req := connect.NewRequest(&api.AddMediaRequest{
			EncodedBytes: []byte("fake data"),
			ContentType:  "image/jpeg",
			Filename:     "test.jpg",
		})

		_, err := service.AddMedia(ctx, req)
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

	t.Run("rejects disallowed and spoofed payloads with InvalidArgument", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
		cases := []struct {
			name        string
			bytes       []byte
			contentType string
		}{
			{"svg disallowed", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), "image/svg+xml"},
			{"html spoofed as jpeg", []byte("<!DOCTYPE html><html></html>"), "image/jpeg"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := service.AddMedia(ctx, connect.NewRequest(&api.AddMediaRequest{
					EncodedBytes: tc.bytes,
					ContentType:  tc.contentType,
					Filename:     "x",
				}))
				if err == nil {
					t.Fatal("expected rejection")
				}
				if connect.CodeOf(err) != connect.CodeInvalidArgument {
					t.Fatalf("expected InvalidArgument, got %v", connect.CodeOf(err))
				}
			})
		}
	})
}
