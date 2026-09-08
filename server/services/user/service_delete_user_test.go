package user

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestService_DeleteUser(t *testing.T) {
	service, userManager, sqlStorage := setupTestService(t)
	ctx := context.Background()

	t.Run("successful deletion", func(t *testing.T) {
		// Create a test user
		testUser, err := userManager.CreateUser(ctx, "delete-me@example.com", "Delete Me", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		// Create authenticated context for the user
		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Delete the user
		req := connect.NewRequest(&api.DeleteUserRequest{})
		_, err = service.DeleteUser(authCtx, req)
		if err != nil {
			t.Fatalf("DeleteUser failed: %v", err)
		}

		// Verify user is not accessible via normal GetByID
		_, err = userManager.GetUserByID(ctx, testUser.Id)
		if err == nil {
			t.Error("Expected error when getting soft-deleted user via normal GetByID")
		}

		// Verify user still exists with IncludeDeleted option
		user := &models.User{}
		err = sqlStorage.GetByID(ctx, testUser.Id, user, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get deleted user with IncludeDeleted: %v", err)
		}

		if user.Deleted == nil {
			t.Error("Expected user to have deleted metadata")
		}
	})

	t.Run("unauthenticated request fails", func(t *testing.T) {
		// Try to delete without authentication
		req := connect.NewRequest(&api.DeleteUserRequest{})
		_, err := service.DeleteUser(ctx, req)
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

	t.Run("soft deletion sets deleted metadata", func(t *testing.T) {
		// Create a test user
		testUser, err := userManager.CreateUser(ctx, "soft-delete@example.com", "Soft Delete User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		// Create authenticated context for the user
		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Delete the user
		req := connect.NewRequest(&api.DeleteUserRequest{})
		_, err = service.DeleteUser(authCtx, req)
		if err != nil {
			t.Fatalf("DeleteUser failed: %v", err)
		}

		// Verify deletion metadata
		user := &models.User{}
		err = sqlStorage.GetByID(ctx, testUser.Id, user, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get deleted user: %v", err)
		}

		if user.Deleted == nil {
			t.Fatal("Expected user to have deleted metadata")
		}
		if user.Deleted.DeletedByUserId != testUser.Id {
			t.Errorf("Expected deleted_by_user_id '%s', got '%s'", testUser.Id, user.Deleted.DeletedByUserId)
		}
		if user.Deleted.DeletedAtUnixSec == 0 {
			t.Error("Expected deleted_at_unix_sec to be set")
		}
	})
}
