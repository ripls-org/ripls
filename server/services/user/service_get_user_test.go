package user

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestService_GetUser(t *testing.T) {
	service, userManager, sqlStorage := setupTestService(t)
	ctx := context.Background()

	// callerUser is the authenticated caller for GetUser requests.
	callerUser, err := userManager.CreateUser(ctx, "caller@example.com", "Caller", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("Failed to create caller user: %v", err)
	}
	authCtx := createAuthenticatedContext(callerUser.Id, callerUser.Email, callerUser.Role)

	t.Run("unauthenticated_request_returns_unauthenticated", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "unauth-getuser@example.com", "Unauth User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		req := connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		})

		_, err = service.GetUser(ctx, req)
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

	t.Run("self fetch returns full profile", func(t *testing.T) {
		// A self-fetch (caller == target) returns every field, including the
		// email the account-settings UI needs.
		testUser, err := userManager.CreateUser(ctx, "test@example.com", "Test User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}
		selfCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Get the user
		req := connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		})

		resp, err := service.GetUser(selfCtx, req)
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		userResp := resp.Msg

		// Verify response fields
		if userResp.UserId != testUser.Id {
			t.Errorf("Expected user_id %s, got %s", testUser.Id, userResp.UserId)
		}

		if userResp.Email != testUser.Email {
			t.Errorf("Expected email %s, got %s", testUser.Email, userResp.Email)
		}

		if userResp.Name != testUser.Name {
			t.Errorf("Expected name %s, got %s", testUser.Name, userResp.Name)
		}

		// Verify location fields are empty by default
		if userResp.PrimaryResidenceLocationId != "" {
			t.Errorf("Expected empty primary residence location, got %s", userResp.PrimaryResidenceLocationId)
		}

		if len(userResp.OtherLocationIds) != 0 {
			t.Errorf("Expected no other locations, got %v", userResp.OtherLocationIds)
		}

		// Verify media_id is empty by default
		if userResp.MediaId != "" {
			t.Errorf("Expected empty media_id, got %s", userResp.MediaId)
		}
	})

	t.Run("self fetch returns locations", func(t *testing.T) {
		// Location IDs are PII; a self-fetch is the only caller that receives them.
		testUser, err := userManager.CreateUser(ctx, "user-with-locations@example.com", "User With Locations", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		// Update user with location data using storage directly
		testUser.PrimaryResidenceLocationId = "location-primary-residence"
		testUser.OtherLocationIds = []string{"location-cabin", "location-office"}

		err = sqlStorage.Update(ctx, testUser)
		if err != nil {
			t.Fatalf("Failed to update user with locations: %v", err)
		}
		selfCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Get the user
		req := connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		})

		resp, err := service.GetUser(selfCtx, req)
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		userResp := resp.Msg

		// Verify location fields are returned
		if userResp.PrimaryResidenceLocationId != "location-primary-residence" {
			t.Errorf("Expected primary_residence_location_id 'location-primary-residence', got %s", userResp.PrimaryResidenceLocationId)
		}

		if len(userResp.OtherLocationIds) != 2 {
			t.Fatalf("Expected 2 other locations, got %d", len(userResp.OtherLocationIds))
		}

		if userResp.OtherLocationIds[0] != "location-cabin" {
			t.Errorf("Expected first other location 'location-cabin', got %s", userResp.OtherLocationIds[0])
		}

		if userResp.OtherLocationIds[1] != "location-office" {
			t.Errorf("Expected second other location 'location-office', got %s", userResp.OtherLocationIds[1])
		}
	})

	t.Run("cross-user fetch omits PII but returns display fields", func(t *testing.T) {
		// A non-self caller must never receive the target's email or location
		// IDs (#2138). Display fields (name, avatar, bio) stay available so
		// owner / participant rendering is unaffected.
		target, err := userManager.CreateUser(ctx, "cross-target@example.com", "Cross Target", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create target user: %v", err)
		}
		target.PrimaryResidenceLocationId = "location-home"
		target.OtherLocationIds = []string{"location-cabin"}
		target.MediaIds = []string{"media-avatar-xyz"}
		target.Description = "Borrows often"
		if updateErr := sqlStorage.Update(ctx, target); updateErr != nil {
			t.Fatalf("Failed to update target user: %v", updateErr)
		}

		// callerUser (authCtx) is a different user than target.
		resp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: target.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}
		userResp := resp.Msg

		// PII must be absent for a cross-user caller.
		if userResp.Email != "" {
			t.Errorf("cross-user fetch leaked email: %q", userResp.Email)
		}
		if userResp.PrimaryResidenceLocationId != "" {
			t.Errorf("cross-user fetch leaked primary_residence_location_id: %q", userResp.PrimaryResidenceLocationId)
		}
		if len(userResp.OtherLocationIds) != 0 {
			t.Errorf("cross-user fetch leaked other_location_ids: %v", userResp.OtherLocationIds)
		}

		// Display fields must still be present.
		if userResp.UserId != target.Id {
			t.Errorf("Expected user_id %s, got %s", target.Id, userResp.UserId)
		}
		if userResp.Name != target.Name {
			t.Errorf("Expected name %s, got %s", target.Name, userResp.Name)
		}
		if userResp.MediaId != "media-avatar-xyz" {
			t.Errorf("Expected media_id 'media-avatar-xyz', got %s", userResp.MediaId)
		}
		if userResp.Description != "Borrows often" {
			t.Errorf("Expected description 'Borrows often', got %s", userResp.Description)
		}
	})

	t.Run("get user with media", func(t *testing.T) {
		// Create a test user
		testUser, err := userManager.CreateUser(ctx, "user-with-media@example.com", "User With Media", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		// Avatar is stored exclusively in media_ids[] after #2083 Phase 4a.
		testUser.MediaIds = []string{"media-profile-pic-123"}

		err = sqlStorage.Update(ctx, testUser)
		if err != nil {
			t.Fatalf("Failed to update user with media_ids: %v", err)
		}

		// Get the user
		req := connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		})

		resp, err := service.GetUser(authCtx, req)
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		userResp := resp.Msg

		// Verify media_id is returned
		if userResp.MediaId != "media-profile-pic-123" {
			t.Errorf("Expected media_id 'media-profile-pic-123', got %s", userResp.MediaId)
		}
	})

	t.Run("missing user_id", func(t *testing.T) {
		req := connect.NewRequest(&api.GetUserRequest{
			// UserId is missing
		})

		_, err := service.GetUser(ctx, req)
		if err == nil {
			t.Fatal("Expected error for missing user_id")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
		}
	})

	t.Run("user not found", func(t *testing.T) {
		req := connect.NewRequest(&api.GetUserRequest{
			UserId: "nonexistent-user-id",
		})

		_, err := service.GetUser(authCtx, req)
		if err == nil {
			t.Fatal("Expected error for non-existent user")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("Expected NotFound error, got %v", connectErr.Code())
		}
	})

	t.Run("self fetch as admin returns full profile", func(t *testing.T) {
		// Create an admin user
		adminUser, err := userManager.CreateUser(ctx, "admin@example.com", "Admin User", models.Role_ROLE_ADMIN)
		if err != nil {
			t.Fatalf("Failed to create admin user: %v", err)
		}
		selfCtx := createAuthenticatedContext(adminUser.Id, adminUser.Email, adminUser.Role)

		req := connect.NewRequest(&api.GetUserRequest{
			UserId: adminUser.Id,
		})

		resp, err := service.GetUser(selfCtx, req)
		if err != nil {
			t.Fatalf("GetUser failed for admin: %v", err)
		}

		userResp := resp.Msg

		// Verify admin user data (role is not exposed in GetUserResponse)
		if userResp.UserId != adminUser.Id {
			t.Errorf("Expected user_id %s, got %s", adminUser.Id, userResp.UserId)
		}

		if userResp.Email != adminUser.Email {
			t.Errorf("Expected email %s, got %s", adminUser.Email, userResp.Email)
		}

		if userResp.Name != adminUser.Name {
			t.Errorf("Expected name %s, got %s", adminUser.Name, userResp.Name)
		}
	})
}

func TestService_GetUser_MultipleUsers(t *testing.T) {
	service, userManager, _ := setupTestService(t)
	ctx := context.Background()

	// Create caller user for authenticated requests.
	callerUser, err := userManager.CreateUser(ctx, "multi-caller@example.com", "Caller", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("Failed to create caller user: %v", err)
	}
	authCtx := createAuthenticatedContext(callerUser.Id, callerUser.Email, callerUser.Role)

	// Create multiple users
	users := []struct {
		email string
		name  string
	}{
		{"user1@example.com", "User One"},
		{"user2@example.com", "User Two"},
		{"user3@example.com", "User Three"},
	}

	var createdUserIds []string

	// Create users
	for _, user := range users {
		createdUser, err := userManager.CreateUser(ctx, user.email, user.name, models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create user %s: %v", user.email, err)
		}
		createdUserIds = append(createdUserIds, createdUser.Id)
	}

	// Retrieve each user cross-user and verify the display fields resolve
	// while the email stays withheld (#2138).
	for i, user := range users {
		req := connect.NewRequest(&api.GetUserRequest{
			UserId: createdUserIds[i],
		})

		resp, err := service.GetUser(authCtx, req)
		if err != nil {
			t.Fatalf("Failed to get user %s: %v", user.email, err)
		}

		userResp := resp.Msg

		if userResp.UserId != createdUserIds[i] {
			t.Errorf("User %s ID mismatch: expected %s, got %s",
				user.email, createdUserIds[i], userResp.UserId)
		}

		if userResp.Email != "" {
			t.Errorf("cross-user fetch of %s leaked email: %q", user.email, userResp.Email)
		}

		if userResp.Name != user.name {
			t.Errorf("User %s name mismatch: expected %s, got %s",
				user.email, user.name, userResp.Name)
		}
	}
}

func TestService_GetUser_EmptyString(t *testing.T) {
	service, _, _ := setupTestService(t)
	ctx := context.Background()

	req := connect.NewRequest(&api.GetUserRequest{
		UserId: "",
	})

	_, err := service.GetUser(ctx, req)
	if err == nil {
		t.Fatal("Expected error for empty user_id")
	}

	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("Expected connect.Error, got %T", err)
	}

	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
	}
}

func TestService_GetUser_Integration(t *testing.T) {
	service, userManager, _ := setupTestService(t)
	ctx := context.Background()

	// Create a user through UserManager
	createdUser, err := userManager.CreateUser(ctx, "integration@example.com", "Integration Test User", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	authCtx := createAuthenticatedContext(createdUser.Id, createdUser.Email, createdUser.Role)

	// Retrieve the same user through UserService
	req := connect.NewRequest(&api.GetUserRequest{
		UserId: createdUser.Id,
	})

	resp, err := service.GetUser(authCtx, req)
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}

	userResp := resp.Msg

	// Verify all fields match
	if userResp.UserId != createdUser.Id {
		t.Errorf("User ID mismatch: created=%s, retrieved=%s", createdUser.Id, userResp.UserId)
	}

	if userResp.Email != createdUser.Email {
		t.Errorf("Email mismatch: created=%s, retrieved=%s", createdUser.Email, userResp.Email)
	}

	if userResp.Name != createdUser.Name {
		t.Errorf("Name mismatch: created=%s, retrieved=%s", createdUser.Name, userResp.Name)
	}

	// Verify we can retrieve the user again
	resp2, err := service.GetUser(authCtx, req)
	if err != nil {
		t.Fatalf("Second GetUser failed: %v", err)
	}

	if resp2.Msg.UserId != userResp.UserId {
		t.Error("Multiple GetUser calls returned different data")
	}
}
