package user

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestService_SaveUser_MediaAndDescription(t *testing.T) {
	service, userManager, _ := setupTestService(t)
	ctx := context.Background()

	t.Run("update description", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "desc@example.com", "Desc User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Update with description
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:      testUser.Id,
			Description: proto.String("This is my bio"),
		})

		resp, err := service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		if resp.Msg.UserId != testUser.Id {
			t.Errorf("Expected user ID %s, got %s", testUser.Id, resp.Msg.UserId)
		}

		// Verify description was saved
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if getResp.Msg.Description != "This is my bio" {
			t.Errorf("Expected description 'This is my bio', got %s", getResp.Msg.Description)
		}
	})

	t.Run("sync media_id to media_ids when only media_id provided", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "media-sync1@example.com", "Media Sync User 1", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Update with only media_id
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:  testUser.Id,
			MediaId: proto.String("media-123"),
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		// Verify both media_id and media_ids are set
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if getResp.Msg.MediaId != "media-123" {
			t.Errorf("Expected media_id 'media-123', got %s", getResp.Msg.MediaId)
		}

		if len(getResp.Msg.MediaIds) != 1 {
			t.Fatalf("Expected 1 media_id in media_ids, got %d", len(getResp.Msg.MediaIds))
		}

		if getResp.Msg.MediaIds[0] != "media-123" {
			t.Errorf("Expected media_ids[0] 'media-123', got %s", getResp.Msg.MediaIds[0])
		}
	})

	t.Run("sync media_ids to media_id when media_ids provided", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "media-sync2@example.com", "Media Sync User 2", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Update with media_ids array
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:   testUser.Id,
			MediaIds: []string{"media-001", "media-002", "media-003"},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		// Verify media_id is synced with first element of media_ids
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if getResp.Msg.MediaId != "media-001" {
			t.Errorf("Expected media_id 'media-001' (synced from media_ids[0]), got %s", getResp.Msg.MediaId)
		}

		if len(getResp.Msg.MediaIds) != 3 {
			t.Fatalf("Expected 3 media_ids, got %d", len(getResp.Msg.MediaIds))
		}

		if getResp.Msg.MediaIds[0] != "media-001" {
			t.Errorf("Expected media_ids[0] 'media-001', got %s", getResp.Msg.MediaIds[0])
		}
		if getResp.Msg.MediaIds[1] != "media-002" {
			t.Errorf("Expected media_ids[1] 'media-002', got %s", getResp.Msg.MediaIds[1])
		}
		if getResp.Msg.MediaIds[2] != "media-003" {
			t.Errorf("Expected media_ids[2] 'media-003', got %s", getResp.Msg.MediaIds[2])
		}
	})

	t.Run("media_ids overrides media_id when both provided", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "media-override@example.com", "Media Override User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Update with both media_id and media_ids - media_ids should take precedence
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:   testUser.Id,
			MediaId:  proto.String("media-old"), // This should be ignored
			MediaIds: []string{"media-new1", "media-new2"},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		// Verify media_ids took precedence
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if getResp.Msg.MediaId != "media-new1" {
			t.Errorf("Expected media_id 'media-new1' (from media_ids[0]), got %s", getResp.Msg.MediaId)
		}

		if len(getResp.Msg.MediaIds) != 2 {
			t.Fatalf("Expected 2 media_ids, got %d", len(getResp.Msg.MediaIds))
		}
	})

	t.Run("update multiple profile images", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "multi-media@example.com", "Multi Media User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Set initial profile images
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:   testUser.Id,
			MediaIds: []string{"img-1", "img-2"},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		// Update to reorder and add new image
		req = connect.NewRequest(&api.SaveUserRequest{
			UserId:   testUser.Id,
			MediaIds: []string{"img-2", "img-3", "img-1"}, // Reordered with new image
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser update failed: %v", err)
		}

		// Verify new order
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if getResp.Msg.MediaId != "img-2" {
			t.Errorf("Expected media_id 'img-2' (new primary), got %s", getResp.Msg.MediaId)
		}

		if len(getResp.Msg.MediaIds) != 3 {
			t.Fatalf("Expected 3 media_ids, got %d", len(getResp.Msg.MediaIds))
		}

		if getResp.Msg.MediaIds[0] != "img-2" || getResp.Msg.MediaIds[1] != "img-3" || getResp.Msg.MediaIds[2] != "img-1" {
			t.Errorf("Expected media_ids ['img-2', 'img-3', 'img-1'], got %v", getResp.Msg.MediaIds)
		}
	})

	t.Run("update name, description, and media together", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "full-update@example.com", "Original Name", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Update all profile fields at once
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:      testUser.Id,
			Name:        proto.String("New Name"),
			Description: proto.String("Updated bio"),
			MediaIds:    []string{"profile-1", "profile-2", "profile-3"},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		// Verify all fields updated correctly
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if getResp.Msg.Name != "New Name" {
			t.Errorf("Expected name 'New Name', got %s", getResp.Msg.Name)
		}

		if getResp.Msg.Description != "Updated bio" {
			t.Errorf("Expected description 'Updated bio', got %s", getResp.Msg.Description)
		}

		if getResp.Msg.MediaId != "profile-1" {
			t.Errorf("Expected media_id 'profile-1', got %s", getResp.Msg.MediaId)
		}

		if len(getResp.Msg.MediaIds) != 3 {
			t.Fatalf("Expected 3 media_ids, got %d", len(getResp.Msg.MediaIds))
		}
	})
}
