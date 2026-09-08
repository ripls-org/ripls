package auth

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return sqlStorage
}

func TestUserManager_CreateUser(t *testing.T) {
	storage := setupTestStorage(t)
	userManager := NewUserManager(storage)

	ctx := context.Background()
	email := "test@example.com"
	name := "Test User"
	role := models.Role_ROLE_USER

	user, err := userManager.CreateUser(ctx, email, name, role)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Check user fields
	if user.Email != email {
		t.Errorf("Expected email %s, got %s", email, user.Email)
	}

	if user.Name != name {
		t.Errorf("Expected name %s, got %s", name, user.Name)
	}

	if user.Role != role {
		t.Errorf("Expected role %v, got %v", role, user.Role)
	}

	if user.Id == "" {
		t.Error("Expected user ID to be generated")
	}

	if user.CreatedAt == 0 {
		t.Error("Expected CreatedAt to be set")
	}

	if user.UpdatedAt == 0 {
		t.Error("Expected UpdatedAt to be set")
	}
}

func TestUserManager_CreateUser_Duplicate(t *testing.T) {
	storage := setupTestStorage(t)
	userManager := NewUserManager(storage)

	ctx := context.Background()
	email := "test@example.com"
	name := "Test User"
	role := models.Role_ROLE_USER

	// Create first user
	_, err := userManager.CreateUser(ctx, email, name, role)
	if err != nil {
		t.Fatalf("Failed to create first user: %v", err)
	}

	// Try to create second user with same email
	_, err = userManager.CreateUser(ctx, email, "Another User", role)
	if err == nil {
		t.Fatal("Expected error when creating user with duplicate email")
	}
}

func TestUserManager_GetUserByID(t *testing.T) {
	storage := setupTestStorage(t)
	userManager := NewUserManager(storage)

	ctx := context.Background()
	email := "test@example.com"
	name := "Test User"
	role := models.Role_ROLE_ADMIN

	// Create a user
	createdUser, err := userManager.CreateUser(ctx, email, name, role)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Retrieve the user by ID
	retrievedUser, err := userManager.GetUserByID(ctx, createdUser.Id)
	if err != nil {
		t.Fatalf("Failed to get user by ID: %v", err)
	}

	// Check that retrieved user matches created user
	if retrievedUser.Id != createdUser.Id {
		t.Errorf("Expected ID %s, got %s", createdUser.Id, retrievedUser.Id)
	}

	if retrievedUser.Email != createdUser.Email {
		t.Errorf("Expected email %s, got %s", createdUser.Email, retrievedUser.Email)
	}

	if retrievedUser.Name != createdUser.Name {
		t.Errorf("Expected name %s, got %s", createdUser.Name, retrievedUser.Name)
	}

	if retrievedUser.Role != createdUser.Role {
		t.Errorf("Expected role %v, got %v", createdUser.Role, retrievedUser.Role)
	}
}

func TestUserManager_GetUserByID_NotFound(t *testing.T) {
	storage := setupTestStorage(t)
	userManager := NewUserManager(storage)

	ctx := context.Background()
	_, err := userManager.GetUserByID(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("Expected error when getting nonexistent user")
	}
}

func TestUserManager_GetUserByEmail(t *testing.T) {
	storage := setupTestStorage(t)
	userManager := NewUserManager(storage)

	ctx := context.Background()
	email := "test@example.com"
	name := "Test User"
	role := models.Role_ROLE_USER

	// Create a user
	createdUser, err := userManager.CreateUser(ctx, email, name, role)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Retrieve the user by email
	retrievedUser, err := userManager.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("Failed to get user by email: %v", err)
	}

	if retrievedUser == nil {
		t.Fatal("Expected to find user by email")
	}

	// Check that retrieved user matches created user
	if retrievedUser.Id != createdUser.Id {
		t.Errorf("Expected ID %s, got %s", createdUser.Id, retrievedUser.Id)
	}

	if retrievedUser.Email != createdUser.Email {
		t.Errorf("Expected email %s, got %s", createdUser.Email, retrievedUser.Email)
	}
}

func TestUserManager_GetUserByEmail_NotFound(t *testing.T) {
	storage := setupTestStorage(t)
	userManager := NewUserManager(storage)

	ctx := context.Background()
	user, err := userManager.GetUserByEmail(ctx, "nonexistent@example.com")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if user != nil {
		t.Fatal("Expected nil user for nonexistent email")
	}
}

func TestUserManager_UpdateUser(t *testing.T) {
	storage := setupTestStorage(t)
	userManager := NewUserManager(storage)
	ctx := context.Background()

	t.Run("successful update", func(t *testing.T) {
		// Create a user
		user, err := userManager.CreateUser(ctx, "test@example.com", "Test User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		originalUpdatedAt := user.UpdatedAt

		// Update the user
		user.Name = "Updated Name"
		user.PrimaryResidenceLocationId = "location-123"
		user.OtherLocationIds = []string{"location-456", "location-789"}

		err = userManager.UpdateUser(ctx, user)
		if err != nil {
			t.Fatalf("UpdateUser failed: %v", err)
		}

		// Verify the update
		updatedUser, err := userManager.GetUserByID(ctx, user.Id)
		if err != nil {
			t.Fatalf("GetUserByID failed: %v", err)
		}

		if updatedUser.Name != "Updated Name" {
			t.Errorf("Expected name 'Updated Name', got %s", updatedUser.Name)
		}
		if updatedUser.PrimaryResidenceLocationId != "location-123" {
			t.Errorf("Expected primary_residence_location_id 'location-123', got %s", updatedUser.PrimaryResidenceLocationId)
		}
		if len(updatedUser.OtherLocationIds) != 2 {
			t.Fatalf("Expected 2 other locations, got %d", len(updatedUser.OtherLocationIds))
		}
		if updatedUser.OtherLocationIds[0] != "location-456" {
			t.Errorf("Expected first other location 'location-456', got %s", updatedUser.OtherLocationIds[0])
		}
		if updatedUser.OtherLocationIds[1] != "location-789" {
			t.Errorf("Expected second other location 'location-789', got %s", updatedUser.OtherLocationIds[1])
		}

		// Verify UpdatedAt is set (may be same as original if update happens in same second)
		if updatedUser.UpdatedAt == 0 {
			t.Error("Expected UpdatedAt to be set")
		}
		if updatedUser.UpdatedAt < originalUpdatedAt {
			t.Error("Expected UpdatedAt to not be less than original")
		}
	})

	t.Run("update non-existent user fails", func(t *testing.T) {
		user := &models.User{
			Id:    "nonexistent-id",
			Email: "nonexistent@example.com",
			Name:  "Nonexistent User",
		}

		err := userManager.UpdateUser(ctx, user)
		if err == nil {
			t.Fatal("Expected error when updating non-existent user")
		}
	})

	t.Run("multiple updates", func(t *testing.T) {
		// Create a user
		user, err := userManager.CreateUser(ctx, "multi@example.com", "Multi User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Update 1: Change name
		user.Name = "First Update"
		err = userManager.UpdateUser(ctx, user)
		if err != nil {
			t.Fatalf("First UpdateUser failed: %v", err)
		}

		// Update 2: Add primary residence
		user.PrimaryResidenceLocationId = "location-primary"
		err = userManager.UpdateUser(ctx, user)
		if err != nil {
			t.Fatalf("Second UpdateUser failed: %v", err)
		}

		// Update 3: Add other locations
		user.OtherLocationIds = []string{"location-1", "location-2"}
		err = userManager.UpdateUser(ctx, user)
		if err != nil {
			t.Fatalf("Third UpdateUser failed: %v", err)
		}

		// Verify all updates persisted
		finalUser, err := userManager.GetUserByID(ctx, user.Id)
		if err != nil {
			t.Fatalf("GetUserByID failed: %v", err)
		}

		if finalUser.Name != "First Update" {
			t.Errorf("Expected name 'First Update', got %s", finalUser.Name)
		}
		if finalUser.PrimaryResidenceLocationId != "location-primary" {
			t.Errorf("Expected primary_residence_location_id 'location-primary', got %s", finalUser.PrimaryResidenceLocationId)
		}
		if len(finalUser.OtherLocationIds) != 2 {
			t.Errorf("Expected 2 other locations, got %d", len(finalUser.OtherLocationIds))
		}
	})
}

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"already normalized", "alice@example.com", "alice@example.com"},
		{"uppercase", "ALICE@EXAMPLE.COM", "alice@example.com"},
		{"mixed case", "Alice@Example.Com", "alice@example.com"},
		{"leading whitespace", "  alice@example.com", "alice@example.com"},
		{"trailing whitespace", "alice@example.com  ", "alice@example.com"},
		{"both whitespace and case", "  Alice@Example.Com\t", "alice@example.com"},
		{"empty", "", ""},
		{"only whitespace", "   ", ""},
		{"unicode preserved", "Élise@example.com", "élise@example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeEmail(tt.in)
			if got != tt.want {
				t.Errorf("NormalizeEmail(%q) = %q, want %q", tt.in, got, tt.want)
			}
			// Idempotence: applying twice yields the same result.
			if again := NormalizeEmail(got); again != got {
				t.Errorf("NormalizeEmail not idempotent: %q -> %q -> %q", tt.in, got, again)
			}
		})
	}
}

func TestUserManager_GetUserByEmail_NormalizesInput(t *testing.T) {
	storage := setupTestStorage(t)
	userManager := NewUserManager(storage)
	ctx := context.Background()

	original, err := userManager.CreateUser(ctx, "Foo@Example.Com", "Foo", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if original.Email != "foo@example.com" {
		t.Errorf("expected stored email to be normalized to foo@example.com, got %q", original.Email)
	}

	for _, lookup := range []string{
		"foo@example.com",
		"FOO@EXAMPLE.COM",
		"  Foo@Example.Com  ",
	} {
		got, err := userManager.GetUserByEmail(ctx, lookup)
		if err != nil {
			t.Fatalf("GetUserByEmail(%q) failed: %v", lookup, err)
		}
		if got == nil {
			t.Fatalf("GetUserByEmail(%q) returned nil; expected to find normalized user", lookup)
		}
		if got.Id != original.Id {
			t.Errorf("GetUserByEmail(%q) returned id %s, want %s", lookup, got.Id, original.Id)
		}
	}
}

func TestUserManager_GetUserByEmail_ExcludesSoftDeleted(t *testing.T) {
	storage := setupTestStorage(t)
	userManager := NewUserManager(storage)
	ctx := context.Background()

	user, err := userManager.CreateUser(ctx, "deleted@example.com", "Deleted", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if err := userManager.DeleteUser(ctx, user.Id); err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	got, err := userManager.GetUserByEmail(ctx, "deleted@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for soft-deleted user, got %+v", got)
	}
}
