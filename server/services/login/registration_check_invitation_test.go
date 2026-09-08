package login

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestService_CheckInvitation(t *testing.T) {
	service, sqlStorage, userManager, _ := setupTestService(t)
	ctx := context.Background()

	// Create an inviter user and community for invitation tests
	inviter, err := userManager.CreateUser(ctx, "inviter@example.com", "Inviter", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("Failed to create inviter: %v", err)
	}
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)

	t.Run("valid invitation token", func(t *testing.T) {
		invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

		req := connect.NewRequest(&api.CheckInvitationRequest{
			ShortCode: invitation.ShortCode,
		})

		resp, err := service.CheckInvitation(ctx, req)
		if err != nil {
			t.Fatalf("CheckInvitation failed: %v", err)
		}

		if !resp.Msg.IsValid {
			t.Errorf("Expected invitation to be valid, got error: %s", resp.Msg.ErrorMessage)
		}

		if resp.Msg.CommunityId != community.Id {
			t.Errorf("Expected community ID '%s', got '%s'", community.Id, resp.Msg.CommunityId)
		}

		if resp.Msg.CommunityName != community.Name {
			t.Errorf("Expected community name '%s', got '%s'", community.Name, resp.Msg.CommunityName)
		}

		if resp.Msg.InviterName != inviter.Name {
			t.Errorf("Expected inviter name '%s', got '%s'", inviter.Name, resp.Msg.InviterName)
		}

		if resp.Msg.ErrorMessage != "" {
			t.Errorf("Expected no error message, got '%s'", resp.Msg.ErrorMessage)
		}

		// Verify member count fields are populated
		if resp.Msg.MaxMembers != 32 {
			t.Errorf("Expected MaxMembers to be 32, got %d", resp.Msg.MaxMembers)
		}

		// NumMembers should be 0 since no one has joined yet
		if resp.Msg.NumMembers < 0 {
			t.Errorf("Expected NumMembers to be >= 0, got %d", resp.Msg.NumMembers)
		}

		// Community has no media, so image URL should be empty
		if resp.Msg.CommunityImageUrl != "" {
			t.Errorf("Expected empty community image URL for community without media, got '%s'", resp.Msg.CommunityImageUrl)
		}
	})

	t.Run("valid invitation with community image", func(t *testing.T) {
		// Create a fresh service so we can access its bucket for storing test media
		imageService, imageSQLStorage, imageUserManager, _ := setupTestService(t)
		imageCtx := context.Background()

		imageInviter, err := imageUserManager.CreateUser(imageCtx, "imageinviter@example.com", "ImageInviter", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create inviter: %v", err)
		}

		// Create a media record and store fake image data in bucket
		media := &models.Media{
			Id:     uuid.New().String(),
			UserId: imageInviter.Id,
		}
		_, err = imageSQLStorage.Insert(imageCtx, media)
		if err != nil {
			t.Fatalf("Failed to create media: %v", err)
		}

		bucketKey := storage.MediaBucketKey(media.UserId, media.Id)
		_, err = imageService.bucket.Put(imageCtx, bucketKey, []byte("fake image data"), "image/jpeg", nil)
		if err != nil {
			t.Fatalf("Failed to store media in bucket: %v", err)
		}

		// Create a community with an image
		communityWithImage := &models.Community{
			Id:               uuid.New().String(),
			Name:             "Community With Image",
			Description:      "A community with a photo",
			CreatorId:        imageInviter.Id,
			OwnerUserId:      imageInviter.Id,
			MediaIds:         []string{media.Id},
			CreatedAtUnixSec: time.Now().Unix(),
			UpdatedAtUnixSec: time.Now().Unix(),
		}
		_, err = imageSQLStorage.Insert(imageCtx, communityWithImage)
		if err != nil {
			t.Fatalf("Failed to create community: %v", err)
		}

		invitation := createTestInvitation(t, imageCtx, imageSQLStorage, communityWithImage.Id, imageInviter.Id)

		req := connect.NewRequest(&api.CheckInvitationRequest{
			ShortCode: invitation.ShortCode,
		})

		resp, err := imageService.CheckInvitation(imageCtx, req)
		if err != nil {
			t.Fatalf("CheckInvitation failed: %v", err)
		}

		if !resp.Msg.IsValid {
			t.Fatalf("Expected invitation to be valid, got error: %s", resp.Msg.ErrorMessage)
		}

		if resp.Msg.CommunityImageUrl == "" {
			t.Error("Expected non-empty community image URL for community with media")
		}
	})

	t.Run("invalid invitation token", func(t *testing.T) {
		req := connect.NewRequest(&api.CheckInvitationRequest{
			ShortCode: "invalid-token-12345",
		})

		resp, err := service.CheckInvitation(ctx, req)
		if err != nil {
			t.Fatalf("CheckInvitation failed: %v", err)
		}

		if resp.Msg.IsValid {
			t.Error("Expected invitation to be invalid")
		}

		if resp.Msg.ErrorMessage != "invalid invitation code" {
			t.Errorf("Expected 'invalid invitation code' error, got '%s'", resp.Msg.ErrorMessage)
		}
	})

	t.Run("invalid token allowed for first user (bootstrap)", func(t *testing.T) {
		// Create a fresh service with empty database
		bootstrapService, _, _, _ := setupTestService(t)
		bootstrapCtx := context.Background()

		req := connect.NewRequest(&api.CheckInvitationRequest{
			ShortCode: "any-invalid-token-for-bootstrap",
		})

		resp, err := bootstrapService.CheckInvitation(bootstrapCtx, req)
		if err != nil {
			t.Fatalf("CheckInvitation failed: %v", err)
		}

		if !resp.Msg.IsValid {
			t.Errorf("Expected bootstrap invitation to be valid for first user, got error: %s", resp.Msg.ErrorMessage)
		}

		if resp.Msg.CommunityName != "Bootstrap User" {
			t.Errorf("Expected community name 'Bootstrap User', got '%s'", resp.Msg.CommunityName)
		}

		if resp.Msg.InviterName != "System" {
			t.Errorf("Expected inviter name 'System', got '%s'", resp.Msg.InviterName)
		}
	})

	t.Run("revoked invitation token", func(t *testing.T) {
		// Create a revoked community-invite share link
		revokedInvitation := &models.ShareLink{
			Id:               uuid.New().String(),
			CommunityId:      community.Id,
			InviterId:        inviter.Id,
			ShortCode:        generateTestShortCode(t),
			IsRevoked:        true,
			CreatedAtUnixSec: time.Now().Unix(),
			Target: &models.ShareLink_CommunityInviteId{
				CommunityInviteId: community.Id,
			},
		}

		_, err := sqlStorage.Insert(ctx, revokedInvitation)
		if err != nil {
			t.Fatalf("Failed to create revoked invitation: %v", err)
		}

		req := connect.NewRequest(&api.CheckInvitationRequest{
			ShortCode: revokedInvitation.ShortCode,
		})

		resp, err := service.CheckInvitation(ctx, req)
		if err != nil {
			t.Fatalf("CheckInvitation failed: %v", err)
		}

		if resp.Msg.IsValid {
			t.Error("Expected revoked invitation to be invalid")
		}

		if resp.Msg.ErrorMessage != "invitation link has been revoked" {
			t.Errorf("Expected 'invitation link has been revoked' error, got '%s'", resp.Msg.ErrorMessage)
		}
	})

	t.Run("missing invitation token", func(t *testing.T) {
		req := connect.NewRequest(&api.CheckInvitationRequest{
			ShortCode: "",
		})

		resp, err := service.CheckInvitation(ctx, req)
		if err != nil {
			t.Fatalf("CheckInvitation failed: %v", err)
		}

		if resp.Msg.IsValid {
			t.Error("Expected missing short code to be invalid")
		}

		if resp.Msg.ErrorMessage != "short code is required" {
			t.Errorf("Expected 'short code is required' error, got '%s'", resp.Msg.ErrorMessage)
		}
	})
}
