package media

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

func TestService_DeleteMedia(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	t.Run("successful deletion by owner", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Create test media
		testMedia := &models.Media{
			UserId:      "user123",
			ContentType: "image/jpeg",
			StorageUrl:  "mock://bucket/media/user123/test-media",
			SizeBytes:   15,
			Filename:    proto.String("test.jpg"),
			Description: proto.String("Test image to delete"),
		}

		mediaID, err := testStorage.Insert(ctx, testMedia)
		if err != nil {
			t.Fatalf("Failed to insert test media: %v", err)
		}

		// Delete media
		req := connect.NewRequest(&api.DeleteMediaRequest{
			Id: mediaID,
		})

		_, err = service.DeleteMedia(ctx, req)
		if err != nil {
			t.Fatalf("DeleteMedia failed: %v", err)
		}

		// Verify media is not accessible via normal GetByID (soft deleted)
		deletedMedia := &models.Media{}
		err = testStorage.GetByID(ctx, mediaID, deletedMedia)
		if err == nil {
			t.Error("Expected error when getting soft-deleted media via normal GetByID")
		}

		// Verify media still exists with IncludeDeleted option
		err = testStorage.GetByID(ctx, mediaID, deletedMedia, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get soft-deleted media with IncludeDeleted: %v", err)
		}
		if deletedMedia.Deleted == nil {
			t.Error("Expected media to have deleted metadata")
		}

		// Bucket storage should NOT be deleted on soft delete (kept for recovery)
		if mockBucket.DeleteCalled {
			t.Error("Expected bucket Delete NOT to be called for soft delete")
		}
	})

	t.Run("successful deletion with thumbnail", func(t *testing.T) {
		ctx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

		// Create test media with thumbnail
		testMedia := &models.Media{
			UserId:              "user456",
			ContentType:         "image/jpeg",
			StorageUrl:          "mock://bucket/media/user456/test-media",
			ThumbnailStorageUrl: proto.String("mock://bucket/thumbnails/user456/test-media"),
			SizeBytes:           1024,
			Filename:            proto.String("large.jpg"),
			Description:         proto.String("Large image with thumbnail"),
		}

		mediaID, err := testStorage.Insert(ctx, testMedia)
		if err != nil {
			t.Fatalf("Failed to insert test media: %v", err)
		}

		// Reset mock state
		mockBucket.DeleteCalled = false

		// Delete media
		req := connect.NewRequest(&api.DeleteMediaRequest{
			Id: mediaID,
		})

		_, err = service.DeleteMedia(ctx, req)
		if err != nil {
			t.Fatalf("DeleteMedia failed: %v", err)
		}

		// Verify media is not accessible via normal GetByID (soft deleted)
		deletedMedia := &models.Media{}
		err = testStorage.GetByID(ctx, mediaID, deletedMedia)
		if err == nil {
			t.Error("Expected error when getting soft-deleted media via normal GetByID")
		}

		// Verify media still exists with IncludeDeleted option
		err = testStorage.GetByID(ctx, mediaID, deletedMedia, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get soft-deleted media with IncludeDeleted: %v", err)
		}
		if deletedMedia.Deleted == nil {
			t.Error("Expected media to have deleted metadata")
		}

		// Bucket storage should NOT be deleted on soft delete (kept for recovery)
		if mockBucket.DeleteCalled {
			t.Error("Expected bucket Delete NOT to be called for soft delete")
		}
	})

	t.Run("unauthorized deletion by different user", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "user123@example.com", models.Role_ROLE_USER)

		// Create test media owned by user123
		testMedia := &models.Media{
			UserId:      "user123",
			ContentType: "image/jpeg",
			StorageUrl:  "mock://bucket/media/user123/private-media",
			SizeBytes:   15,
			Filename:    proto.String("private.jpg"),
			Description: proto.String("Private image"),
		}

		mediaID, err := testStorage.Insert(ctx, testMedia)
		if err != nil {
			t.Fatalf("Failed to insert test media: %v", err)
		}

		// Try to delete as different user (user789)
		ctxOtherUser := createAuthenticatedContext("user789", "user789@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.DeleteMediaRequest{
			Id: mediaID,
		})

		_, err = service.DeleteMedia(ctxOtherUser, req)
		if err == nil {
			t.Fatal("Expected error when deleting another user's media")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", connectErr.Code())
		}

		// Verify media was NOT deleted
		stillExists := &models.Media{}
		err = testStorage.GetByID(ctx, mediaID, stillExists)
		if err != nil {
			t.Error("Expected media to still exist after unauthorized delete attempt")
		}
	})

	t.Run("no authentication", func(t *testing.T) {
		ctx := context.Background()

		req := connect.NewRequest(&api.DeleteMediaRequest{
			Id: "some-media-id",
		})

		_, err := service.DeleteMedia(ctx, req)
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

		req := connect.NewRequest(&api.DeleteMediaRequest{
			Id: "nonexistent-media-id",
		})

		_, err := service.DeleteMedia(ctx, req)
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

	t.Run("deletion does not affect other media", func(t *testing.T) {
		ctx := createAuthenticatedContext("user999", "user999@example.com", models.Role_ROLE_USER)

		// Create multiple media items
		media1 := &models.Media{
			UserId:      "user999",
			ContentType: "image/jpeg",
			StorageUrl:  "mock://bucket/media/user999/media1",
			Filename:    proto.String("image1.jpg"),
		}

		media2 := &models.Media{
			UserId:      "user999",
			ContentType: "image/png",
			StorageUrl:  "mock://bucket/media/user999/media2",
			Filename:    proto.String("image2.png"),
		}

		mediaID1, err := testStorage.Insert(ctx, media1)
		if err != nil {
			t.Fatalf("Failed to insert media1: %v", err)
		}

		mediaID2, err := testStorage.Insert(ctx, media2)
		if err != nil {
			t.Fatalf("Failed to insert media2: %v", err)
		}

		// Delete first media
		req := connect.NewRequest(&api.DeleteMediaRequest{
			Id: mediaID1,
		})

		_, err = service.DeleteMedia(ctx, req)
		if err != nil {
			t.Fatalf("DeleteMedia failed: %v", err)
		}

		// Verify media1 is soft-deleted (not accessible via normal GetByID)
		err = testStorage.GetByID(ctx, mediaID1, &models.Media{})
		if err == nil {
			t.Error("Expected media1 to be soft-deleted")
		}

		// Verify media2 still exists
		err = testStorage.GetByID(ctx, mediaID2, &models.Media{})
		if err != nil {
			t.Error("Expected media2 to still exist")
		}
	})

	t.Run("soft deletion sets deleted metadata", func(t *testing.T) {
		ctx := createAuthenticatedContext("user-soft-del", "softdel@example.com", models.Role_ROLE_USER)

		// Create test media
		testMedia := &models.Media{
			UserId:      "user-soft-del",
			ContentType: "image/jpeg",
			StorageUrl:  "mock://bucket/media/user-soft-del/test-media",
			SizeBytes:   1024,
			Filename:    proto.String("soft-delete-test.jpg"),
		}

		mediaID, err := testStorage.Insert(ctx, testMedia)
		if err != nil {
			t.Fatalf("Failed to insert test media: %v", err)
		}

		// Delete media
		req := connect.NewRequest(&api.DeleteMediaRequest{Id: mediaID})
		_, err = service.DeleteMedia(ctx, req)
		if err != nil {
			t.Fatalf("DeleteMedia failed: %v", err)
		}

		// Verify media still exists with IncludeDeleted option and has deletion metadata
		media := &models.Media{}
		err = testStorage.GetByID(ctx, mediaID, media, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get deleted media with IncludeDeleted: %v", err)
		}

		if media.Deleted == nil {
			t.Fatal("Expected media to have deleted metadata")
		}
		if media.Deleted.DeletedByUserId != "user-soft-del" {
			t.Errorf("Expected deleted_by_user_id 'user-soft-del', got '%s'", media.Deleted.DeletedByUserId)
		}
		if media.Deleted.DeletedAtUnixSec == 0 {
			t.Error("Expected deleted_at_unix_sec to be set")
		}
	})
}
