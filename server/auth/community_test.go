package auth

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestIsMemberOfCommunity(t *testing.T) {
	ctx := context.Background()
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := uuid.New().String()
	userID := uuid.New().String()

	t.Run("user is member", func(t *testing.T) {
		// Create membership
		membership := &models.CommunityUser{
			Id:          uuid.New().String(),
			CommunityId: communityID,
			UserId:      userID,
		}
		_, err := sqlStorage.Insert(ctx, membership)
		if err != nil {
			t.Fatalf("Failed to create membership: %v", err)
		}

		isMember, err := IsMemberOfCommunity(ctx, sqlStorage, communityID, userID)
		if err != nil {
			t.Fatalf("IsMemberOfCommunity failed: %v", err)
		}

		if !isMember {
			t.Error("Expected user to be member, but was not")
		}
	})

	t.Run("user is not member", func(t *testing.T) {
		nonMemberID := uuid.New().String()

		isMember, err := IsMemberOfCommunity(ctx, sqlStorage, communityID, nonMemberID)
		if err != nil {
			t.Fatalf("IsMemberOfCommunity failed: %v", err)
		}

		if isMember {
			t.Error("Expected user to not be member, but was")
		}
	})

	t.Run("different community", func(t *testing.T) {
		differentCommunityID := uuid.New().String()

		isMember, err := IsMemberOfCommunity(ctx, sqlStorage, differentCommunityID, userID)
		if err != nil {
			t.Fatalf("IsMemberOfCommunity failed: %v", err)
		}

		if isMember {
			t.Error("Expected user to not be member of different community, but was")
		}
	})
}

func TestGetCommunityUser(t *testing.T) {
	ctx := context.Background()
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := uuid.New().String()
	userID := uuid.New().String()
	membershipID := uuid.New().String()

	t.Run("membership exists", func(t *testing.T) {
		// Create membership
		membership := &models.CommunityUser{
			Id:          membershipID,
			CommunityId: communityID,
			UserId:      userID,
		}
		_, err := sqlStorage.Insert(ctx, membership)
		if err != nil {
			t.Fatalf("Failed to create membership: %v", err)
		}

		result, err := GetCommunityUser(ctx, sqlStorage, communityID, userID)
		if err != nil {
			t.Fatalf("GetCommunityUser failed: %v", err)
		}

		if result == nil {
			t.Fatal("Expected membership record, got nil")
		}

		if result.Id != membershipID {
			t.Errorf("Expected membership ID %s, got %s", membershipID, result.Id)
		}

		if result.CommunityId != communityID {
			t.Errorf("Expected community ID %s, got %s", communityID, result.CommunityId)
		}

		if result.UserId != userID {
			t.Errorf("Expected user ID %s, got %s", userID, result.UserId)
		}
	})

	t.Run("membership does not exist", func(t *testing.T) {
		nonMemberID := uuid.New().String()

		result, err := GetCommunityUser(ctx, sqlStorage, communityID, nonMemberID)
		if err != nil {
			t.Fatalf("GetCommunityUser failed: %v", err)
		}

		if result != nil {
			t.Errorf("Expected nil for non-existent membership, got %v", result)
		}
	})
}

func TestRequireUserIsCommunityMember(t *testing.T) {
	ctx := context.Background()
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := uuid.New().String()
	userID := uuid.New().String()

	t.Run("user is member - no error", func(t *testing.T) {
		// Create membership
		membership := &models.CommunityUser{
			Id:          uuid.New().String(),
			CommunityId: communityID,
			UserId:      userID,
		}
		_, err := sqlStorage.Insert(ctx, membership)
		if err != nil {
			t.Fatalf("Failed to create membership: %v", err)
		}

		err = RequireUserIsCommunityMember(ctx, sqlStorage, communityID, userID, "custom error message")
		if err != nil {
			t.Fatalf("Expected no error for valid member, got: %v", err)
		}
	})

	t.Run("user is not member - permission denied", func(t *testing.T) {
		nonMemberID := uuid.New().String()
		customMsg := "you must be a member to access this resource"

		err := RequireUserIsCommunityMember(ctx, sqlStorage, communityID, nonMemberID, customMsg)
		if err == nil {
			t.Fatal("Expected permission denied error, got nil")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected CodePermissionDenied, got %v", connectErr.Code())
		}

		if connectErr.Message() != customMsg {
			t.Errorf("Expected error message %q, got %q", customMsg, connectErr.Message())
		}
	})
}
