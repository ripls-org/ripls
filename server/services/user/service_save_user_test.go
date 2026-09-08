package user

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestService_SaveUser(t *testing.T) {
	service, userManager, _ := setupTestService(t)
	ctx := context.Background()

	t.Run("successful update with locations", func(t *testing.T) {
		// Create a test user
		testUser, err := userManager.CreateUser(ctx, "update@example.com", "Update User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		// Create authenticated context for the user
		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Update user with locations
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:                     testUser.Id,
			Name:                       proto.String("Updated Name"),
			PrimaryResidenceLocationId: proto.String("location-home"),
			OtherLocationIds:           []string{"location-cabin", "location-office"},
		})

		resp, err := service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		if resp.Msg.UserId != testUser.Id {
			t.Errorf("Expected user ID %s, got %s", testUser.Id, resp.Msg.UserId)
		}

		// Verify updates were applied
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if getResp.Msg.Name != "Updated Name" {
			t.Errorf("Expected name 'Updated Name', got %s", getResp.Msg.Name)
		}
		if getResp.Msg.PrimaryResidenceLocationId != "location-home" {
			t.Errorf("Expected primary_residence_location_id 'location-home', got %s", getResp.Msg.PrimaryResidenceLocationId)
		}
		if len(getResp.Msg.OtherLocationIds) != 2 {
			t.Fatalf("Expected 2 other locations, got %d", len(getResp.Msg.OtherLocationIds))
		}

		// Verify both expected locations exist (order may vary due to map iteration)
		locationMap := make(map[string]bool)
		for _, locID := range getResp.Msg.OtherLocationIds {
			locationMap[locID] = true
		}

		if !locationMap["location-cabin"] {
			t.Errorf("Expected 'location-cabin' in other locations, got %v", getResp.Msg.OtherLocationIds)
		}
		if !locationMap["location-office"] {
			t.Errorf("Expected 'location-office' in other locations, got %v", getResp.Msg.OtherLocationIds)
		}
	})

	t.Run("unauthenticated request fails", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "unauth@example.com", "Unauth User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		req := connect.NewRequest(&api.SaveUserRequest{
			UserId: testUser.Id,
			Name:   proto.String("Should Fail"),
		})

		_, err = service.SaveUser(ctx, req)
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

	t.Run("cannot update other user", func(t *testing.T) {
		testUser1, err := userManager.CreateUser(ctx, "user1@example.com", "User 1", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		testUser2, err := userManager.CreateUser(ctx, "user2@example.com", "User 2", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		// User 1 tries to update User 2
		authCtx := createAuthenticatedContext(testUser1.Id, testUser1.Email, testUser1.Role)

		req := connect.NewRequest(&api.SaveUserRequest{
			UserId: testUser2.Id,
			Name:   proto.String("Should Fail"),
		})

		_, err = service.SaveUser(authCtx, req)
		if err == nil {
			t.Fatal("Expected error when updating another user")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", connectErr.Code())
		}
	})

	t.Run("missing user_id", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "missing@example.com", "Missing User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		req := connect.NewRequest(&api.SaveUserRequest{
			Name: proto.String("Should Fail"),
		})

		_, err = service.SaveUser(authCtx, req)
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

	t.Run("partial update - name only", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "partial@example.com", "Partial User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Update only name
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId: testUser.Id,
			Name:   proto.String("New Name Only"),
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		// Verify name was updated, locations remain empty
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if getResp.Msg.Name != "New Name Only" {
			t.Errorf("Expected name 'New Name Only', got %s", getResp.Msg.Name)
		}
		if getResp.Msg.PrimaryResidenceLocationId != "" {
			t.Errorf("Expected empty primary_residence_location_id, got %s", getResp.Msg.PrimaryResidenceLocationId)
		}
	})

	t.Run("update with media_id", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "media-update@example.com", "Media User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Update user with media_id
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:  testUser.Id,
			Name:    proto.String("User With Profile Pic"),
			MediaId: proto.String("media-avatar-456"),
		})

		resp, err := service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		if resp.Msg.UserId != testUser.Id {
			t.Errorf("Expected user ID %s, got %s", testUser.Id, resp.Msg.UserId)
		}

		// Verify media_id was saved
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if getResp.Msg.Name != "User With Profile Pic" {
			t.Errorf("Expected name 'User With Profile Pic', got %s", getResp.Msg.Name)
		}
		if getResp.Msg.MediaId != "media-avatar-456" {
			t.Errorf("Expected media_id 'media-avatar-456', got %s", getResp.Msg.MediaId)
		}
	})

	t.Run("partial update - media_id only", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "media-only@example.com", "Media Only User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Update only media_id
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:  testUser.Id,
			MediaId: proto.String("media-new-avatar-789"),
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		// Verify media_id was updated, other fields remain unchanged
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if getResp.Msg.MediaId != "media-new-avatar-789" {
			t.Errorf("Expected media_id 'media-new-avatar-789', got %s", getResp.Msg.MediaId)
		}
		if getResp.Msg.Name != "Media Only User" {
			t.Errorf("Expected name unchanged 'Media Only User', got %s", getResp.Msg.Name)
		}
		if getResp.Msg.PrimaryResidenceLocationId != "" {
			t.Errorf("Expected empty primary_residence_location_id, got %s", getResp.Msg.PrimaryResidenceLocationId)
		}
	})

	t.Run("old primary location saved as other location when setting new primary", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "location-change@example.com", "Location Change User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Set initial primary location
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:                     testUser.Id,
			PrimaryResidenceLocationId: proto.String("location-old-home"),
			OtherLocationIds:           []string{"location-cabin"},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed for initial location: %v", err)
		}

		// Verify initial state
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if getResp.Msg.PrimaryResidenceLocationId != "location-old-home" {
			t.Errorf("Expected primary 'location-old-home', got %s", getResp.Msg.PrimaryResidenceLocationId)
		}
		if len(getResp.Msg.OtherLocationIds) != 1 {
			t.Fatalf("Expected 1 other location initially, got %d", len(getResp.Msg.OtherLocationIds))
		}

		// Update to new primary location
		req = connect.NewRequest(&api.SaveUserRequest{
			UserId:                     testUser.Id,
			PrimaryResidenceLocationId: proto.String("location-new-home"),
			OtherLocationIds:           []string{"location-cabin"},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed for new location: %v", err)
		}

		// Verify old primary was saved as other location
		getResp, err = service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed after update: %v", err)
		}

		if getResp.Msg.PrimaryResidenceLocationId != "location-new-home" {
			t.Errorf("Expected primary 'location-new-home', got %s", getResp.Msg.PrimaryResidenceLocationId)
		}

		// Should have 2 other locations now: original cabin + old home
		if len(getResp.Msg.OtherLocationIds) != 2 {
			t.Fatalf("Expected 2 other locations, got %d", len(getResp.Msg.OtherLocationIds))
		}

		// Verify the old primary is in other locations
		foundOldPrimary := false
		foundCabin := false
		for _, locID := range getResp.Msg.OtherLocationIds {
			if locID == "location-old-home" {
				foundOldPrimary = true
			}
			if locID == "location-cabin" {
				foundCabin = true
			}
		}

		if !foundOldPrimary {
			t.Errorf("Expected old primary 'location-old-home' in other locations, got %v", getResp.Msg.OtherLocationIds)
		}
		if !foundCabin {
			t.Errorf("Expected 'location-cabin' in other locations, got %v", getResp.Msg.OtherLocationIds)
		}
	})

	t.Run("old primary not duplicated if already in other locations", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "no-duplicate@example.com", "No Duplicate User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Set initial primary location
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:                     testUser.Id,
			PrimaryResidenceLocationId: proto.String("location-primary"),
			OtherLocationIds:           []string{"location-cabin"},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed for initial location: %v", err)
		}

		// Update to new primary, but manually include old primary in other locations
		req = connect.NewRequest(&api.SaveUserRequest{
			UserId:                     testUser.Id,
			PrimaryResidenceLocationId: proto.String("location-new"),
			OtherLocationIds:           []string{"location-cabin", "location-primary"},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed for new location: %v", err)
		}

		// Verify old primary is not duplicated
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		// Should still have 2 other locations (no duplicate)
		if len(getResp.Msg.OtherLocationIds) != 2 {
			t.Fatalf("Expected 2 other locations (no duplicate), got %d: %v",
				len(getResp.Msg.OtherLocationIds), getResp.Msg.OtherLocationIds)
		}

		// Count occurrences of location-primary
		count := 0
		for _, locID := range getResp.Msg.OtherLocationIds {
			if locID == "location-primary" {
				count++
			}
		}

		if count != 1 {
			t.Errorf("Expected 'location-primary' to appear once, got %d times", count)
		}
	})

	t.Run("no old primary when user has no initial primary", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "no-initial@example.com", "No Initial User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Set primary location without any previous primary
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:                     testUser.Id,
			PrimaryResidenceLocationId: proto.String("location-first-home"),
			OtherLocationIds:           []string{"location-cabin"},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		// Verify only the specified other locations exist
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if getResp.Msg.PrimaryResidenceLocationId != "location-first-home" {
			t.Errorf("Expected primary 'location-first-home', got %s", getResp.Msg.PrimaryResidenceLocationId)
		}

		// Should only have the cabin, no empty string added
		if len(getResp.Msg.OtherLocationIds) != 1 {
			t.Fatalf("Expected 1 other location, got %d: %v",
				len(getResp.Msg.OtherLocationIds), getResp.Msg.OtherLocationIds)
		}

		if getResp.Msg.OtherLocationIds[0] != "location-cabin" {
			t.Errorf("Expected 'location-cabin', got %s", getResp.Msg.OtherLocationIds[0])
		}
	})

	t.Run("no change when setting same primary location", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "same-primary@example.com", "Same Primary User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Set initial primary location
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:                     testUser.Id,
			PrimaryResidenceLocationId: proto.String("location-home"),
			OtherLocationIds:           []string{"location-cabin"},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		// Update with same primary location
		req = connect.NewRequest(&api.SaveUserRequest{
			UserId:                     testUser.Id,
			PrimaryResidenceLocationId: proto.String("location-home"),
			OtherLocationIds:           []string{"location-cabin"},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed on update: %v", err)
		}

		// Verify no duplication of primary in other locations
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if len(getResp.Msg.OtherLocationIds) != 1 {
			t.Fatalf("Expected 1 other location, got %d: %v",
				len(getResp.Msg.OtherLocationIds), getResp.Msg.OtherLocationIds)
		}

		if getResp.Msg.OtherLocationIds[0] != "location-cabin" {
			t.Errorf("Expected 'location-cabin', got %s", getResp.Msg.OtherLocationIds[0])
		}
	})

	t.Run("duplicate locations in other_location_ids are deduplicated", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "dup-other@example.com", "Duplicate Other User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Submit request with duplicate location IDs in other_location_ids
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:                     testUser.Id,
			PrimaryResidenceLocationId: proto.String("location-home"),
			OtherLocationIds:           []string{"location-cabin", "location-office", "location-cabin", "location-office"},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		// Verify duplicates were removed
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if len(getResp.Msg.OtherLocationIds) != 2 {
			t.Fatalf("Expected 2 unique other locations, got %d: %v",
				len(getResp.Msg.OtherLocationIds), getResp.Msg.OtherLocationIds)
		}

		// Verify both unique locations exist
		locationMap := make(map[string]bool)
		for _, locID := range getResp.Msg.OtherLocationIds {
			locationMap[locID] = true
		}

		if !locationMap["location-cabin"] {
			t.Errorf("Expected 'location-cabin' in other locations, got %v", getResp.Msg.OtherLocationIds)
		}
		if !locationMap["location-office"] {
			t.Errorf("Expected 'location-office' in other locations, got %v", getResp.Msg.OtherLocationIds)
		}
	})

	t.Run("primary location removed from other_location_ids if present", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "primary-in-other@example.com", "Primary In Other User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Submit request with primary location also in other_location_ids
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:                     testUser.Id,
			PrimaryResidenceLocationId: proto.String("location-home"),
			OtherLocationIds:           []string{"location-cabin", "location-home", "location-office"},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		// Verify primary location was removed from other locations
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if len(getResp.Msg.OtherLocationIds) != 2 {
			t.Fatalf("Expected 2 other locations (primary removed), got %d: %v",
				len(getResp.Msg.OtherLocationIds), getResp.Msg.OtherLocationIds)
		}

		// Verify primary location is NOT in other locations
		for _, locID := range getResp.Msg.OtherLocationIds {
			if locID == "location-home" {
				t.Errorf("Primary location 'location-home' should not be in other locations, got %v", getResp.Msg.OtherLocationIds)
			}
		}

		// Verify expected locations exist
		locationMap := make(map[string]bool)
		for _, locID := range getResp.Msg.OtherLocationIds {
			locationMap[locID] = true
		}

		if !locationMap["location-cabin"] {
			t.Errorf("Expected 'location-cabin' in other locations, got %v", getResp.Msg.OtherLocationIds)
		}
		if !locationMap["location-office"] {
			t.Errorf("Expected 'location-office' in other locations, got %v", getResp.Msg.OtherLocationIds)
		}
	})

	t.Run("empty string location IDs are filtered out", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "empty-strings@example.com", "Empty Strings User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// Submit request with empty strings in other_location_ids
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:                     testUser.Id,
			PrimaryResidenceLocationId: proto.String("location-home"),
			OtherLocationIds:           []string{"location-cabin", "", "location-office", ""},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}

		// Verify empty strings were filtered out
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if len(getResp.Msg.OtherLocationIds) != 2 {
			t.Fatalf("Expected 2 other locations (empty strings filtered), got %d: %v",
				len(getResp.Msg.OtherLocationIds), getResp.Msg.OtherLocationIds)
		}

		// Verify no empty strings exist
		for _, locID := range getResp.Msg.OtherLocationIds {
			if locID == "" {
				t.Errorf("Empty string should not be in other locations, got %v", getResp.Msg.OtherLocationIds)
			}
		}

		// Verify expected locations exist
		locationMap := make(map[string]bool)
		for _, locID := range getResp.Msg.OtherLocationIds {
			locationMap[locID] = true
		}

		if !locationMap["location-cabin"] {
			t.Errorf("Expected 'location-cabin' in other locations, got %v", getResp.Msg.OtherLocationIds)
		}
		if !locationMap["location-office"] {
			t.Errorf("Expected 'location-office' in other locations, got %v", getResp.Msg.OtherLocationIds)
		}
	})

	t.Run("multiple primary location changes with deduplication", func(t *testing.T) {
		testUser, err := userManager.CreateUser(ctx, "multiple-changes@example.com", "Multiple Changes User", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		authCtx := createAuthenticatedContext(testUser.Id, testUser.Email, testUser.Role)

		// First update: Set primary to location-1
		req := connect.NewRequest(&api.SaveUserRequest{
			UserId:                     testUser.Id,
			PrimaryResidenceLocationId: proto.String("location-1"),
			OtherLocationIds:           []string{},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed (1st update): %v", err)
		}

		// Second update: Set primary to location-2 (location-1 should move to other)
		req = connect.NewRequest(&api.SaveUserRequest{
			UserId:                     testUser.Id,
			PrimaryResidenceLocationId: proto.String("location-2"),
			OtherLocationIds:           []string{},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed (2nd update): %v", err)
		}

		// Third update: Set primary back to location-1 (should be removed from other)
		req = connect.NewRequest(&api.SaveUserRequest{
			UserId:                     testUser.Id,
			PrimaryResidenceLocationId: proto.String("location-1"),
			OtherLocationIds:           []string{},
		})

		_, err = service.SaveUser(authCtx, req)
		if err != nil {
			t.Fatalf("SaveUser failed (3rd update): %v", err)
		}

		// Verify final state
		getResp, err := service.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{
			UserId: testUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}

		if getResp.Msg.PrimaryResidenceLocationId != "location-1" {
			t.Errorf("Expected primary 'location-1', got %s", getResp.Msg.PrimaryResidenceLocationId)
		}

		if len(getResp.Msg.OtherLocationIds) != 1 {
			t.Fatalf("Expected 1 other location, got %d: %v",
				len(getResp.Msg.OtherLocationIds), getResp.Msg.OtherLocationIds)
		}

		if getResp.Msg.OtherLocationIds[0] != "location-2" {
			t.Errorf("Expected 'location-2' in other locations, got %v", getResp.Msg.OtherLocationIds)
		}
	})
}
