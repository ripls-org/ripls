package location

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestService_SaveLocation(t *testing.T) {
	service, _, _ := setupTestService(t)

	t.Run("successful save location", func(t *testing.T) {
		ctx := createAuthContext("user-123", "test@example.com")

		req := connect.NewRequest(&api.SaveLocationRequest{
			LatitudeDeg:  37.4220,
			LongitudeDeg: -122.0856,
			RegionCode:   "US",
			PostalCode:   "94043",
			Locality:     "Mountain View",
			AddressLines: []string{"1600 Amphitheatre Parkway"},
		})

		resp, err := service.SaveLocation(ctx, req)
		if err != nil {
			t.Fatalf("SaveLocation failed: %v", err)
		}

		if resp.Msg.Id == "" {
			t.Error("Expected non-empty location ID")
		}
	})

	t.Run("unauthenticated request", func(t *testing.T) {
		ctx := context.Background() // No auth info

		req := connect.NewRequest(&api.SaveLocationRequest{
			LatitudeDeg:  37.4220,
			LongitudeDeg: -122.0856,
		})

		_, err := service.SaveLocation(ctx, req)
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

	t.Run("automatically adds location to user's location list", func(t *testing.T) {
		service, sqlStorage, _ := setupTestService(t)
		ctx := createAuthContext("user-no-primary", "noprimary@example.com")

		// Create a test user
		user := &models.User{
			Id:    "user-no-primary",
			Email: "noprimary@example.com",
			Name:  "Test User",
		}
		_, err := sqlStorage.Insert(context.Background(), user)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		// Save a location
		req := connect.NewRequest(&api.SaveLocationRequest{
			LatitudeDeg:  40.7128,
			LongitudeDeg: -74.0060,
			RegionCode:   "US",
			PostalCode:   "10001",
			Locality:     "New York",
			AddressLines: []string{"123 Main St"},
		})

		resp, err := service.SaveLocation(ctx, req)
		if err != nil {
			t.Fatalf("SaveLocation failed: %v", err)
		}

		// Verify location was created
		if resp.Msg.Id == "" {
			t.Error("Expected non-empty location ID")
		}

		// Verify UserLocation entry was created
		userLocations, err := sqlStorage.QueryByFields(
			context.Background(),
			map[string]any{
				"user_id":     "user-no-primary",
				"location_id": resp.Msg.Id,
			},
			&models.UserLocation{},
		)
		if err != nil {
			t.Fatalf("Failed to query user locations: %v", err)
		}

		if len(userLocations) != 1 {
			t.Errorf("Expected 1 user location entry, got %d", len(userLocations))
		}
	})

	t.Run("creates new location and adds to user's location list", func(t *testing.T) {
		service, sqlStorage, _ := setupTestService(t)
		ctx := createAuthContext("user-has-primary", "hasprimary@example.com")

		// Create a test user
		user := &models.User{
			Id:    "user-has-primary",
			Email: "hasprimary@example.com",
			Name:  "Test User",
		}
		_, err := sqlStorage.Insert(context.Background(), user)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		// Save a new location
		req := connect.NewRequest(&api.SaveLocationRequest{
			LatitudeDeg:  37.7749,
			LongitudeDeg: -122.4194,
			RegionCode:   "US",
			PostalCode:   "94102",
			Locality:     "San Francisco",
			AddressLines: []string{"456 Market St"},
		})

		resp, err := service.SaveLocation(ctx, req)
		if err != nil {
			t.Fatalf("SaveLocation failed: %v", err)
		}

		// Verify UserLocation entry was created
		userLocations, err := sqlStorage.QueryByFields(
			context.Background(),
			map[string]any{
				"user_id":     "user-has-primary",
				"location_id": resp.Msg.Id,
			},
			&models.UserLocation{},
		)
		if err != nil {
			t.Fatalf("Failed to query user locations: %v", err)
		}

		if len(userLocations) != 1 {
			t.Errorf("Expected 1 user location entry, got %d", len(userLocations))
		}
	})

	t.Run("updates existing user location when saving same location again", func(t *testing.T) {
		service, sqlStorage, _ := setupTestService(t)
		ctx := createAuthContext("user-duplicate", "duplicate@example.com")

		// Create a test user
		user := &models.User{
			Id:    "user-duplicate",
			Email: "duplicate@example.com",
			Name:  "Test User",
		}
		_, err := sqlStorage.Insert(context.Background(), user)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		// Save a location
		req := connect.NewRequest(&api.SaveLocationRequest{
			LatitudeDeg:  40.7128,
			LongitudeDeg: -74.0060,
			RegionCode:   "US",
			Locality:     "New York",
		})

		resp, err := service.SaveLocation(ctx, req)
		if err != nil {
			t.Fatalf("SaveLocation failed: %v", err)
		}

		// Verify UserLocation entry was created
		userLocations, err := sqlStorage.QueryByFields(
			context.Background(),
			map[string]any{
				"user_id":     "user-duplicate",
				"location_id": resp.Msg.Id,
			},
			&models.UserLocation{},
		)
		if err != nil {
			t.Fatalf("Failed to query user locations: %v", err)
		}

		if len(userLocations) != 1 {
			t.Errorf("Expected 1 user location entry, got %d", len(userLocations))
		}
	})
}

func TestService_GetLocation(t *testing.T) {
	service, sqlStorage, _ := setupTestService(t)

	t.Run("successful get location", func(t *testing.T) {
		ctx := createAuthContext("user-123", "test@example.com")

		// Create a location first
		loc := &models.Location{
			Geolocation: &models.Geolocation{
				LatitudeDeg:  30.271850,
				LongitudeDeg: -97.752842,
			},
			Address: &models.Address{
				RegionCode:   "US",
				PostalCode:   "78703",
				Locality:     "Austin",
				AddressLines: []string{"603 North Lamar Boulevard"},
			},
		}

		locationID, err := sqlStorage.Insert(context.Background(), loc)
		if err != nil {
			t.Fatalf("Failed to create test location: %v", err)
		}

		// Get the location
		req := connect.NewRequest(&api.GetLocationRequest{
			Id: locationID,
		})

		resp, err := service.GetLocation(ctx, req)
		if err != nil {
			t.Fatalf("GetLocation failed: %v", err)
		}

		// Verify response
		if resp.Msg.Id != locationID {
			t.Errorf("Expected ID %s, got %s", locationID, resp.Msg.Id)
		}

		if resp.Msg.LatitudeDeg != 30.271850 {
			t.Errorf("Expected latitude 30.271850, got %f", resp.Msg.LatitudeDeg)
		}

		if resp.Msg.LongitudeDeg != -97.752842 {
			t.Errorf("Expected longitude -97.752842, got %f", resp.Msg.LongitudeDeg)
		}

		if resp.Msg.Locality != "Austin" {
			t.Errorf("Expected locality Austin, got %s", resp.Msg.Locality)
		}
	})

	t.Run("location not found", func(t *testing.T) {
		ctx := createAuthContext("user-123", "test@example.com")

		req := connect.NewRequest(&api.GetLocationRequest{
			Id: "nonexistent-location-id",
		})

		_, err := service.GetLocation(ctx, req)
		if err == nil {
			t.Fatal("Expected error for non-existent location")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("Expected NotFound error, got %v", connectErr.Code())
		}
	})

	t.Run("unauthenticated request", func(t *testing.T) {
		ctx := context.Background()

		req := connect.NewRequest(&api.GetLocationRequest{
			Id: "some-id",
		})

		_, err := service.GetLocation(ctx, req)
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
}

func TestService_DeleteLocation(t *testing.T) {
	t.Run("successful delete location", func(t *testing.T) {
		service, sqlStorage, _ := setupTestService(t)
		ctx := createAuthContext("user-123", "test@example.com")

		// Create a location
		loc := &models.Location{
			Geolocation: &models.Geolocation{
				LatitudeDeg:  30.2672,
				LongitudeDeg: -97.7431,
			},
			Address: &models.Address{
				Locality:   "Austin",
				RegionCode: "US",
			},
		}

		locationID, err := sqlStorage.Insert(context.Background(), loc)
		if err != nil {
			t.Fatalf("Failed to create test location: %v", err)
		}

		// Create a user with this location
		user := &models.User{
			Id:                         "user-123",
			Email:                      "test@example.com",
			Name:                       "Test User",
			PrimaryResidenceLocationId: locationID,
		}

		_, err = sqlStorage.Insert(context.Background(), user)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		// Delete the location
		req := connect.NewRequest(&api.DeleteLocationRequest{
			Id: locationID,
		})

		resp, err := service.DeleteLocation(ctx, req)
		if err != nil {
			t.Fatalf("DeleteLocation failed: %v", err)
		}

		if resp.Msg == nil {
			t.Error("Expected non-nil response")
		}

		// Verify location was deleted
		deletedLoc := &models.Location{}
		err = sqlStorage.GetByID(context.Background(), locationID, deletedLoc)
		if err == nil {
			t.Error("Expected location to be deleted from storage")
		}

		// Verify user's primary residence was cleared
		updatedUser := &models.User{}
		err = sqlStorage.GetByID(context.Background(), "user-123", updatedUser)
		if err != nil {
			t.Fatalf("Failed to get updated user: %v", err)
		}

		if updatedUser.PrimaryResidenceLocationId != "" {
			t.Errorf("Expected primary_residence_location_id to be cleared, got %s",
				updatedUser.PrimaryResidenceLocationId)
		}
	})

	t.Run("delete location from other_location_ids", func(t *testing.T) {
		service, sqlStorage, _ := setupTestService(t)
		ctx := createAuthContext("user-123", "test@example.com")

		// Create two locations
		loc1 := &models.Location{
			Geolocation: &models.Geolocation{
				LatitudeDeg:  30.2672,
				LongitudeDeg: -97.7431,
			},
			Address: &models.Address{Locality: "Austin"},
		}

		loc2 := &models.Location{
			Geolocation: &models.Geolocation{
				LatitudeDeg:  29.4241,
				LongitudeDeg: -98.4936,
			},
			Address: &models.Address{Locality: "San Antonio"},
		}

		loc1ID, _ := sqlStorage.Insert(context.Background(), loc1)
		loc2ID, _ := sqlStorage.Insert(context.Background(), loc2)

		// Create user with both locations in other_location_ids
		user := &models.User{
			Id:               "user-123",
			Email:            "test@example.com",
			Name:             "Test User",
			OtherLocationIds: []string{loc1ID, loc2ID},
		}

		_, err := sqlStorage.Insert(context.Background(), user)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		// Delete loc1
		req := connect.NewRequest(&api.DeleteLocationRequest{
			Id: loc1ID,
		})

		_, err = service.DeleteLocation(ctx, req)
		if err != nil {
			t.Fatalf("DeleteLocation failed: %v", err)
		}

		// Verify loc1 was removed from other_location_ids
		updatedUser := &models.User{}
		err = sqlStorage.GetByID(context.Background(), "user-123", updatedUser)
		if err != nil {
			t.Fatalf("Failed to get updated user: %v", err)
		}

		if len(updatedUser.OtherLocationIds) != 1 {
			t.Fatalf("Expected 1 other location, got %d", len(updatedUser.OtherLocationIds))
		}

		if updatedUser.OtherLocationIds[0] != loc2ID {
			t.Errorf("Expected remaining location to be loc2 (%s), got %s",
				loc2ID, updatedUser.OtherLocationIds[0])
		}
	})

	t.Run("delete location clears gear reference", func(t *testing.T) {
		service, sqlStorage, _ := setupTestService(t)
		ctx := createAuthContext("user-123", "test@example.com")

		// Create a location
		loc := &models.Location{
			Geolocation: &models.Geolocation{
				LatitudeDeg:  30.2672,
				LongitudeDeg: -97.7431,
			},
			Address: &models.Address{Locality: "Austin"},
		}

		locID, _ := sqlStorage.Insert(context.Background(), loc)

		// Create user
		user := &models.User{
			Id:               "user-123",
			Email:            "test@example.com",
			Name:             "Test User",
			OtherLocationIds: []string{locID},
		}

		_, _ = sqlStorage.Insert(context.Background(), user)

		// Create gear with this location
		gear := &models.Gear{
			Name:       "Test Drill",
			OwnerId:    "user-123",
			LocationId: locID,
		}

		gearID, err := sqlStorage.Insert(context.Background(), gear)
		if err != nil {
			t.Fatalf("Failed to create test gear: %v", err)
		}

		// Delete the location
		req := connect.NewRequest(&api.DeleteLocationRequest{
			Id: locID,
		})

		_, err = service.DeleteLocation(ctx, req)
		if err != nil {
			t.Fatalf("DeleteLocation failed: %v", err)
		}

		// Verify gear's location_id was cleared
		updatedGear := &models.Gear{}
		err = sqlStorage.GetByID(context.Background(), gearID, updatedGear)
		if err != nil {
			t.Fatalf("Failed to get updated gear: %v", err)
		}

		if updatedGear.LocationId != "" {
			t.Errorf("Expected gear location_id to be cleared, got %s", updatedGear.LocationId)
		}
	})

	t.Run("cannot delete location owned by another user", func(t *testing.T) {
		service, sqlStorage, _ := setupTestService(t)
		ctx := createAuthContext("user-123", "test@example.com")

		// Create a location
		loc := &models.Location{
			Geolocation: &models.Geolocation{
				LatitudeDeg:  30.2672,
				LongitudeDeg: -97.7431,
			},
			Address: &models.Address{Locality: "Austin"},
		}

		locID, _ := sqlStorage.Insert(context.Background(), loc)

		// Create a different user with this location
		otherUser := &models.User{
			Id:                         "other-user",
			Email:                      "other@example.com",
			Name:                       "Other User",
			PrimaryResidenceLocationId: locID,
		}

		_, err := sqlStorage.Insert(context.Background(), otherUser)
		if err != nil {
			t.Fatalf("Failed to create other user: %v", err)
		}

		// Try to delete as user-123 (should fail)
		req := connect.NewRequest(&api.DeleteLocationRequest{
			Id: locID,
		})

		_, err = service.DeleteLocation(ctx, req)
		if err == nil {
			t.Fatal("Expected error when deleting location owned by another user")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", connectErr.Code())
		}
	})

	t.Run("cannot delete location referenced by another user's gear", func(t *testing.T) {
		service, sqlStorage, _ := setupTestService(t)
		ctx := createAuthContext("user-123", "test@example.com")

		// Create a location
		loc := &models.Location{
			Geolocation: &models.Geolocation{
				LatitudeDeg:  30.2672,
				LongitudeDeg: -97.7431,
			},
			Address: &models.Address{Locality: "Austin"},
		}

		locID, _ := sqlStorage.Insert(context.Background(), loc)

		// Create user who owns the location
		user := &models.User{
			Id:                         "user-123",
			Email:                      "test@example.com",
			Name:                       "Test User",
			PrimaryResidenceLocationId: locID,
		}

		_, _ = sqlStorage.Insert(context.Background(), user)

		// Create gear owned by a different user with this location
		otherGear := &models.Gear{
			Name:       "Other User's Drill",
			OwnerId:    "other-user",
			LocationId: locID,
		}

		_, err := sqlStorage.Insert(context.Background(), otherGear)
		if err != nil {
			t.Fatalf("Failed to create other user's gear: %v", err)
		}

		// Try to delete the location (should fail)
		req := connect.NewRequest(&api.DeleteLocationRequest{
			Id: locID,
		})

		_, err = service.DeleteLocation(ctx, req)
		if err == nil {
			t.Fatal("Expected error when deleting location referenced by another user's gear")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", connectErr.Code())
		}
	})

	t.Run("location not found", func(t *testing.T) {
		service, _, _ := setupTestService(t)
		ctx := createAuthContext("user-123", "test@example.com")

		req := connect.NewRequest(&api.DeleteLocationRequest{
			Id: "nonexistent-location-id",
		})

		_, err := service.DeleteLocation(ctx, req)
		if err == nil {
			t.Fatal("Expected error for non-existent location")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("Expected NotFound error, got %v", connectErr.Code())
		}
	})

	t.Run("unauthenticated request", func(t *testing.T) {
		service, _, _ := setupTestService(t)
		ctx := context.Background()

		req := connect.NewRequest(&api.DeleteLocationRequest{
			Id: "some-location-id",
		})

		_, err := service.DeleteLocation(ctx, req)
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

	t.Run("delete location removes from both primary and other locations", func(t *testing.T) {
		service, sqlStorage, _ := setupTestService(t)
		ctx := createAuthContext("user-123", "test@example.com")

		// Create locations
		primaryLoc := &models.Location{
			Geolocation: &models.Geolocation{
				LatitudeDeg:  30.2672,
				LongitudeDeg: -97.7431,
			},
			Address: &models.Address{Locality: "Austin"},
		}

		otherLoc := &models.Location{
			Geolocation: &models.Geolocation{
				LatitudeDeg:  29.4241,
				LongitudeDeg: -98.4936,
			},
			Address: &models.Address{Locality: "San Antonio"},
		}

		primaryLocID, _ := sqlStorage.Insert(context.Background(), primaryLoc)
		otherLocID, _ := sqlStorage.Insert(context.Background(), otherLoc)

		// Create user with location in both primary and other_location_ids
		// This is an edge case that shouldn't normally happen, but we should handle it
		user := &models.User{
			Id:                         "user-123",
			Email:                      "test@example.com",
			Name:                       "Test User",
			PrimaryResidenceLocationId: primaryLocID,
			OtherLocationIds:           []string{primaryLocID, otherLocID},
		}

		_, err := sqlStorage.Insert(context.Background(), user)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		// Delete primaryLoc
		req := connect.NewRequest(&api.DeleteLocationRequest{
			Id: primaryLocID,
		})

		_, err = service.DeleteLocation(ctx, req)
		if err != nil {
			t.Fatalf("DeleteLocation failed: %v", err)
		}

		// Verify location was removed from both fields
		updatedUser := &models.User{}
		err = sqlStorage.GetByID(context.Background(), "user-123", updatedUser)
		if err != nil {
			t.Fatalf("Failed to get updated user: %v", err)
		}

		if updatedUser.PrimaryResidenceLocationId != "" {
			t.Errorf("Expected primary_residence_location_id to be cleared, got %s",
				updatedUser.PrimaryResidenceLocationId)
		}

		if len(updatedUser.OtherLocationIds) != 1 {
			t.Fatalf("Expected 1 other location remaining, got %d", len(updatedUser.OtherLocationIds))
		}

		if updatedUser.OtherLocationIds[0] != otherLocID {
			t.Errorf("Expected remaining location to be otherLoc, got %s",
				updatedUser.OtherLocationIds[0])
		}
	})

	t.Run("delete orphaned location with no references", func(t *testing.T) {
		service, sqlStorage, _ := setupTestService(t)
		ctx := createAuthContext("user-123", "test@example.com")

		// Create a location that's not referenced by anyone
		loc := &models.Location{
			Geolocation: &models.Geolocation{
				LatitudeDeg:  30.2672,
				LongitudeDeg: -97.7431,
			},
			Address: &models.Address{Locality: "Austin"},
		}

		locID, err := sqlStorage.Insert(context.Background(), loc)
		if err != nil {
			t.Fatalf("Failed to create test location: %v", err)
		}

		// Create user without this location
		user := &models.User{
			Id:    "user-123",
			Email: "test@example.com",
			Name:  "Test User",
		}

		_, err = sqlStorage.Insert(context.Background(), user)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		// Delete the orphaned location
		// This should succeed because no one owns it (no permission restrictions)
		req := connect.NewRequest(&api.DeleteLocationRequest{
			Id: locID,
		})

		resp, err := service.DeleteLocation(ctx, req)
		if err != nil {
			t.Fatalf("DeleteLocation failed: %v", err)
		}

		if resp.Msg == nil {
			t.Error("Expected non-nil response")
		}

		// Verify location is not accessible via normal GetByID (soft deleted)
		checkLoc := &models.Location{}
		err = sqlStorage.GetByID(context.Background(), locID, checkLoc)
		if err == nil {
			t.Error("Expected error when getting soft-deleted location via normal GetByID")
		}

		// Verify location still exists with IncludeDeleted option
		err = sqlStorage.GetByID(context.Background(), locID, checkLoc, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get soft-deleted location with IncludeDeleted: %v", err)
		}
		if checkLoc.Deleted == nil {
			t.Error("Expected location to have deleted metadata")
		}
	})

	t.Run("soft deletion sets deleted metadata", func(t *testing.T) {
		service, sqlStorage, _ := setupTestService(t)

		// Create a user first
		user := &models.User{
			Id:    "user-soft-del",
			Email: "softdel@example.com",
			Name:  "Soft Del User",
		}
		_, err := sqlStorage.Insert(context.Background(), user)
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}

		ctx := createAuthContext("user-soft-del", "softdel@example.com")

		// Create location as this user
		saveReq := connect.NewRequest(&api.SaveLocationRequest{
			LatitudeDeg:  40.7128,
			LongitudeDeg: -74.0060,
			RegionCode:   "US",
			PostalCode:   "10001",
			Locality:     "New York",
			Name:         "Soft Delete Test Location",
		})

		saveResp, err := service.SaveLocation(ctx, saveReq)
		if err != nil {
			t.Fatalf("SaveLocation failed: %v", err)
		}
		locID := saveResp.Msg.Id

		// Delete location
		deleteReq := connect.NewRequest(&api.DeleteLocationRequest{Id: locID})
		_, err = service.DeleteLocation(ctx, deleteReq)
		if err != nil {
			t.Fatalf("DeleteLocation failed: %v", err)
		}

		// Verify location is not accessible via normal GetByID
		loc := &models.Location{}
		err = sqlStorage.GetByID(ctx, locID, loc)
		if err == nil {
			t.Error("Expected error when getting deleted location via normal GetByID")
		}

		// Verify location still exists with IncludeDeleted option and has deletion metadata
		err = sqlStorage.GetByID(ctx, locID, loc, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get deleted location with IncludeDeleted: %v", err)
		}

		if loc.Deleted == nil {
			t.Fatal("Expected location to have deleted metadata")
		}
		if loc.Deleted.DeletedByUserId != "user-soft-del" {
			t.Errorf("Expected deleted_by_user_id 'user-soft-del', got '%s'", loc.Deleted.DeletedByUserId)
		}
		if loc.Deleted.DeletedAtUnixSec == 0 {
			t.Error("Expected deleted_at_unix_sec to be set")
		}
	})
}
