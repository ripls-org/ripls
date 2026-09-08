package location

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestService_GetUserLocations tests the GetUserLocations RPC.
func TestService_GetUserLocations(t *testing.T) {
	service, sqlStorage, _ := setupTestService(t)

	t.Run("returns empty list for user with no locations", func(t *testing.T) {
		ctx := createAuthContext("user-no-locations", "nolocations@example.com")

		// Create user first
		user := &models.User{
			Id:    "user-no-locations",
			Email: "nolocations@example.com",
			Name:  "No Locations User",
		}
		_, err := sqlStorage.Insert(context.Background(), user)
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		req := connect.NewRequest(&api.GetUserLocationsRequest{
			Limit: 10,
		})

		resp, err := service.GetUserLocations(ctx, req)
		if err != nil {
			t.Fatalf("GetUserLocations failed: %v", err)
		}

		if len(resp.Msg.Locations) != 0 {
			t.Errorf("Expected 0 locations, got %d", len(resp.Msg.Locations))
		}
	})

	t.Run("returns locations sorted by last_used_at descending", func(t *testing.T) {
		ctx := createAuthContext("user-sort-test", "sort@example.com")

		// Create user first
		user := &models.User{
			Id:    "user-sort-test",
			Email: "sort@example.com",
			Name:  "Sort Test User",
		}
		_, err := sqlStorage.Insert(context.Background(), user)
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Create 3 locations with different timestamps
		now := time.Now().Unix()
		locations := []struct {
			id        string
			timestamp int64
		}{
			{"loc-old", now - 3600*24*7},  // 7 days ago
			{"loc-new", now - 3600},       // 1 hour ago
			{"loc-medium", now - 3600*24}, // 1 day ago
		}

		for _, loc := range locations {
			// Create location
			location := &models.Location{
				Id: loc.id,
				Geolocation: &models.Geolocation{
					LatitudeDeg:  37.4220,
					LongitudeDeg: -122.0856,
				},
				Address: &models.Address{
					RegionCode: "US",
					Locality:   "Mountain View",
				},
			}
			_, err := sqlStorage.Insert(context.Background(), location)
			if err != nil {
				t.Fatalf("Failed to create location: %v", err)
			}

			// Create user location association
			userLoc := &models.UserLocation{
				Id:                "ul-" + loc.id,
				UserId:            "user-sort-test",
				LocationId:        loc.id,
				LastUsedAtUnixSec: loc.timestamp,
			}
			_, err = sqlStorage.Insert(context.Background(), userLoc)
			if err != nil {
				t.Fatalf("Failed to create user location: %v", err)
			}
		}

		req := connect.NewRequest(&api.GetUserLocationsRequest{
			Limit: 10,
		})

		resp, err := service.GetUserLocations(ctx, req)
		if err != nil {
			t.Fatalf("GetUserLocations failed: %v", err)
		}

		if len(resp.Msg.Locations) != 3 {
			t.Fatalf("Expected 3 locations, got %d", len(resp.Msg.Locations))
		}

		// Verify sort order (most recent first)
		if resp.Msg.Locations[0].Location.Id != "loc-new" {
			t.Errorf("Expected first location to be loc-new, got %s", resp.Msg.Locations[0].Location.Id)
		}
		if resp.Msg.Locations[1].Location.Id != "loc-medium" {
			t.Errorf("Expected second location to be loc-medium, got %s", resp.Msg.Locations[1].Location.Id)
		}
		if resp.Msg.Locations[2].Location.Id != "loc-old" {
			t.Errorf("Expected third location to be loc-old, got %s", resp.Msg.Locations[2].Location.Id)
		}
	})

	t.Run("returns primary location first regardless of timestamp", func(t *testing.T) {
		ctx := createAuthContext("user-primary-test", "primary@example.com")

		// Create user first
		user := &models.User{
			Id:    "user-primary-test",
			Email: "primary@example.com",
			Name:  "Primary Test",
		}
		_, err := sqlStorage.Insert(context.Background(), user)
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Create 2 locations
		now := time.Now().Unix()
		locations := []struct {
			id        string
			timestamp int64
			isPrimary bool
		}{
			{"loc-primary", now - 3600*24*30, true}, // 30 days ago but primary
			{"loc-recent", now - 3600, false},       // 1 hour ago
		}

		for _, loc := range locations {
			// Create location
			location := &models.Location{
				Id: loc.id,
				Geolocation: &models.Geolocation{
					LatitudeDeg:  37.4220,
					LongitudeDeg: -122.0856,
				},
				Address: &models.Address{
					RegionCode: "US",
					Locality:   "Mountain View",
				},
			}
			_, err := sqlStorage.Insert(context.Background(), location)
			if err != nil {
				t.Fatalf("Failed to create location: %v", err)
			}

			// Create user location association
			userLoc := &models.UserLocation{
				Id:                "ul-" + loc.id,
				UserId:            "user-primary-test",
				LocationId:        loc.id,
				LastUsedAtUnixSec: loc.timestamp,
			}
			_, err = sqlStorage.Insert(context.Background(), userLoc)
			if err != nil {
				t.Fatalf("Failed to create user location: %v", err)
			}

			// Set as primary if needed
			if loc.isPrimary {
				user.PrimaryResidenceLocationId = loc.id
				err = sqlStorage.Update(context.Background(), user)
				if err != nil {
					t.Fatalf("Failed to update user primary location: %v", err)
				}
			}
		}

		req := connect.NewRequest(&api.GetUserLocationsRequest{
			Limit: 10,
		})

		resp, err := service.GetUserLocations(ctx, req)
		if err != nil {
			t.Fatalf("GetUserLocations failed: %v", err)
		}

		if len(resp.Msg.Locations) != 2 {
			t.Fatalf("Expected 2 locations, got %d", len(resp.Msg.Locations))
		}

		// Verify primary location is first
		if resp.Msg.Locations[0].Location.Id != "loc-primary" {
			t.Errorf("Expected first location to be primary (loc-primary), got %s", resp.Msg.Locations[0].Location.Id)
		}
		if resp.Msg.Locations[1].Location.Id != "loc-recent" {
			t.Errorf("Expected second location to be loc-recent, got %s", resp.Msg.Locations[1].Location.Id)
		}
	})

	t.Run("respects limit parameter", func(t *testing.T) {
		ctx := createAuthContext("user-limit-test", "limit@example.com")

		// Create user first
		user := &models.User{
			Id:    "user-limit-test",
			Email: "limit@example.com",
			Name:  "Limit Test User",
		}
		_, err := sqlStorage.Insert(context.Background(), user)
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Create 5 locations
		now := time.Now().Unix()
		for i := 0; i < 5; i++ {
			locID := "loc-limit-" + string(rune('a'+i))

			// Create location
			location := &models.Location{
				Id: locID,
				Geolocation: &models.Geolocation{
					LatitudeDeg:  37.4220,
					LongitudeDeg: -122.0856,
				},
				Address: &models.Address{
					RegionCode: "US",
					Locality:   "Mountain View",
				},
			}
			_, err := sqlStorage.Insert(context.Background(), location)
			if err != nil {
				t.Fatalf("Failed to create location: %v", err)
			}

			// Create user location association
			userLoc := &models.UserLocation{
				Id:                "ul-" + locID,
				UserId:            "user-limit-test",
				LocationId:        locID,
				LastUsedAtUnixSec: now - int64(i*3600),
			}
			_, err = sqlStorage.Insert(context.Background(), userLoc)
			if err != nil {
				t.Fatalf("Failed to create user location: %v", err)
			}
		}

		req := connect.NewRequest(&api.GetUserLocationsRequest{
			Limit: 3,
		})

		resp, err := service.GetUserLocations(ctx, req)
		if err != nil {
			t.Fatalf("GetUserLocations failed: %v", err)
		}

		if len(resp.Msg.Locations) != 3 {
			t.Errorf("Expected 3 locations (limited), got %d", len(resp.Msg.Locations))
		}
	})

	t.Run("requires authentication", func(t *testing.T) {
		ctx := context.Background() // No auth

		req := connect.NewRequest(&api.GetUserLocationsRequest{
			Limit: 10,
		})

		_, err := service.GetUserLocations(ctx, req)
		if err == nil {
			t.Error("Expected authentication error, got nil")
		}
	})
}

// TestService_AddUserLocation tests the AddUserLocation RPC.
func TestService_AddUserLocation(t *testing.T) {
	service, sqlStorage, _ := setupTestService(t)

	t.Run("creates new user location association", func(t *testing.T) {
		ctx := createAuthContext("user-add-test", "add@example.com")

		// Create a location first
		location := &models.Location{
			Id: "loc-add-test",
			Geolocation: &models.Geolocation{
				LatitudeDeg:  37.4220,
				LongitudeDeg: -122.0856,
			},
			Address: &models.Address{
				RegionCode: "US",
				Locality:   "Mountain View",
			},
		}
		_, err := sqlStorage.Insert(context.Background(), location)
		if err != nil {
			t.Fatalf("Failed to create location: %v", err)
		}

		req := connect.NewRequest(&api.AddUserLocationRequest{
			LocationId: "loc-add-test",
		})

		resp, err := service.AddUserLocation(ctx, req)
		if err != nil {
			t.Fatalf("AddUserLocation failed: %v", err)
		}

		if resp.Msg == nil {
			t.Error("Expected non-nil response")
		}

		// Verify the association was created
		userLocs, err := sqlStorage.QueryByFields(context.Background(), map[string]any{
			"user_id":     "user-add-test",
			"location_id": "loc-add-test",
		}, &models.UserLocation{})
		if err != nil {
			t.Fatalf("Failed to query user location: %v", err)
		}

		if len(userLocs) != 1 {
			t.Errorf("Expected 1 user location, got %d", len(userLocs))
		}
	})

	t.Run("updates last_used_at for existing association", func(t *testing.T) {
		ctx := createAuthContext("user-update-test", "update@example.com")

		// Create location
		location := &models.Location{
			Id: "loc-update-test",
			Geolocation: &models.Geolocation{
				LatitudeDeg:  37.4220,
				LongitudeDeg: -122.0856,
			},
			Address: &models.Address{
				RegionCode: "US",
				Locality:   "Mountain View",
			},
		}
		_, err := sqlStorage.Insert(context.Background(), location)
		if err != nil {
			t.Fatalf("Failed to create location: %v", err)
		}

		// Create initial user location association with old timestamp
		oldTimestamp := time.Now().Unix() - 3600*24 // 1 day ago
		userLoc := &models.UserLocation{
			Id:                "ul-update-test",
			UserId:            "user-update-test",
			LocationId:        "loc-update-test",
			LastUsedAtUnixSec: oldTimestamp,
		}
		_, err = sqlStorage.Insert(context.Background(), userLoc)
		if err != nil {
			t.Fatalf("Failed to create user location: %v", err)
		}

		// Add the same location again
		req := connect.NewRequest(&api.AddUserLocationRequest{
			LocationId: "loc-update-test",
		})

		_, err = service.AddUserLocation(ctx, req)
		if err != nil {
			t.Fatalf("AddUserLocation failed: %v", err)
		}

		// Verify timestamp was updated
		userLocs, err := sqlStorage.QueryByFields(context.Background(), map[string]any{
			"user_id":     "user-update-test",
			"location_id": "loc-update-test",
		}, &models.UserLocation{})
		if err != nil {
			t.Fatalf("Failed to query user location: %v", err)
		}

		if len(userLocs) != 1 {
			t.Fatalf("Expected 1 user location, got %d", len(userLocs))
		}

		updatedUserLoc := userLocs[0].(*models.UserLocation)
		if updatedUserLoc.LastUsedAtUnixSec <= oldTimestamp {
			t.Errorf("Expected timestamp to be updated, old=%d, new=%d",
				oldTimestamp, updatedUserLoc.LastUsedAtUnixSec)
		}
	})

	t.Run("requires authentication", func(t *testing.T) {
		ctx := context.Background() // No auth

		req := connect.NewRequest(&api.AddUserLocationRequest{
			LocationId: "some-location",
		})

		_, err := service.AddUserLocation(ctx, req)
		if err == nil {
			t.Error("Expected authentication error, got nil")
		}
	})

	t.Run("allows creating association for any location ID", func(t *testing.T) {
		ctx := createAuthContext("user-any-loc-test", "anyloc@example.com")

		// Note: AddUserLocation doesn't validate location exists
		// This allows deferred location fetching
		req := connect.NewRequest(&api.AddUserLocationRequest{
			LocationId: "any-location-id",
		})

		resp, err := service.AddUserLocation(ctx, req)
		if err != nil {
			t.Fatalf("AddUserLocation failed: %v", err)
		}

		if resp.Msg == nil {
			t.Error("Expected non-nil response")
		}

		// Verify association was created
		userLocs, err := sqlStorage.QueryByFields(context.Background(), map[string]any{
			"user_id":     "user-any-loc-test",
			"location_id": "any-location-id",
		}, &models.UserLocation{})
		if err != nil {
			t.Fatalf("Failed to query user location: %v", err)
		}

		if len(userLocs) != 1 {
			t.Errorf("Expected 1 user location, got %d", len(userLocs))
		}
	})
}

// TestService_RemoveUserLocation tests the RemoveUserLocation RPC.
func TestService_RemoveUserLocation(t *testing.T) {
	service, sqlStorage, _ := setupTestService(t)

	t.Run("deletes user location association", func(t *testing.T) {
		ctx := createAuthContext("user-remove-test", "remove@example.com")

		// Create location
		location := &models.Location{
			Id: "loc-remove-test",
			Geolocation: &models.Geolocation{
				LatitudeDeg:  37.4220,
				LongitudeDeg: -122.0856,
			},
			Address: &models.Address{
				RegionCode: "US",
				Locality:   "Mountain View",
			},
		}
		_, err := sqlStorage.Insert(context.Background(), location)
		if err != nil {
			t.Fatalf("Failed to create location: %v", err)
		}

		// Create user location association
		userLoc := &models.UserLocation{
			Id:                "ul-remove-test",
			UserId:            "user-remove-test",
			LocationId:        "loc-remove-test",
			LastUsedAtUnixSec: time.Now().Unix(),
		}
		_, err = sqlStorage.Insert(context.Background(), userLoc)
		if err != nil {
			t.Fatalf("Failed to create user location: %v", err)
		}

		// Remove the association
		req := connect.NewRequest(&api.RemoveUserLocationRequest{
			LocationId: "loc-remove-test",
		})

		resp, err := service.RemoveUserLocation(ctx, req)
		if err != nil {
			t.Fatalf("RemoveUserLocation failed: %v", err)
		}

		if resp.Msg == nil {
			t.Error("Expected non-nil response")
		}

		// Verify the association was deleted
		userLocs, err := sqlStorage.QueryByFields(context.Background(), map[string]any{
			"user_id":     "user-remove-test",
			"location_id": "loc-remove-test",
		}, &models.UserLocation{})
		if err != nil {
			t.Fatalf("Failed to query user location: %v", err)
		}

		if len(userLocs) != 0 {
			t.Errorf("Expected 0 user locations after deletion, got %d", len(userLocs))
		}
	})

	t.Run("fails for non-existent association", func(t *testing.T) {
		ctx := createAuthContext("user-no-assoc-test", "noassoc@example.com")

		req := connect.NewRequest(&api.RemoveUserLocationRequest{
			LocationId: "non-existent-location",
		})

		_, err := service.RemoveUserLocation(ctx, req)
		if err == nil {
			t.Error("Expected error for non-existent association, got nil")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Error("Expected connect.Error")
		} else if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("Expected CodeNotFound, got %v", connectErr.Code())
		}
	})

	t.Run("requires authentication", func(t *testing.T) {
		ctx := context.Background() // No auth

		req := connect.NewRequest(&api.RemoveUserLocationRequest{
			LocationId: "some-location",
		})

		_, err := service.RemoveUserLocation(ctx, req)
		if err == nil {
			t.Error("Expected authentication error, got nil")
		}
	})

	t.Run("cannot delete another user's location", func(t *testing.T) {
		ctx := createAuthContext("user-other-test", "other@example.com")

		// Create location
		location := &models.Location{
			Id: "loc-other-user-test",
			Geolocation: &models.Geolocation{
				LatitudeDeg:  37.4220,
				LongitudeDeg: -122.0856,
			},
			Address: &models.Address{
				RegionCode: "US",
				Locality:   "Mountain View",
			},
		}
		_, err := sqlStorage.Insert(context.Background(), location)
		if err != nil {
			t.Fatalf("Failed to create location: %v", err)
		}

		// Create user location association for different user
		userLoc := &models.UserLocation{
			Id:                "ul-other-user-test",
			UserId:            "different-user",
			LocationId:        "loc-other-user-test",
			LastUsedAtUnixSec: time.Now().Unix(),
		}
		_, err = sqlStorage.Insert(context.Background(), userLoc)
		if err != nil {
			t.Fatalf("Failed to create user location: %v", err)
		}

		// Try to remove it as different user
		req := connect.NewRequest(&api.RemoveUserLocationRequest{
			LocationId: "loc-other-user-test",
		})

		_, err = service.RemoveUserLocation(ctx, req)
		if err == nil {
			t.Error("Expected error when trying to delete another user's location, got nil")
		}

		// Verify the association still exists
		userLocs, err := sqlStorage.QueryByFields(context.Background(), map[string]any{
			"user_id":     "different-user",
			"location_id": "loc-other-user-test",
		}, &models.UserLocation{})
		if err != nil {
			t.Fatalf("Failed to query user location: %v", err)
		}

		if len(userLocs) != 1 {
			t.Errorf("Expected association to still exist, got %d", len(userLocs))
		}
	})
}

// TestService_UpdateLocationLastUsed tests the UpdateLocationLastUsed RPC.
func TestService_UpdateLocationLastUsed(t *testing.T) {
	service, sqlStorage, _ := setupTestService(t)

	t.Run("updates timestamp for existing association", func(t *testing.T) {
		ctx := createAuthContext("user-timestamp-test", "timestamp@example.com")

		// Create location
		location := &models.Location{
			Id: "loc-timestamp-test",
			Geolocation: &models.Geolocation{
				LatitudeDeg:  37.4220,
				LongitudeDeg: -122.0856,
			},
			Address: &models.Address{
				RegionCode: "US",
				Locality:   "Mountain View",
			},
		}
		_, err := sqlStorage.Insert(context.Background(), location)
		if err != nil {
			t.Fatalf("Failed to create location: %v", err)
		}

		// Create user location association with old timestamp
		oldTimestamp := time.Now().Unix() - 3600*24
		userLoc := &models.UserLocation{
			Id:                "ul-timestamp-test",
			UserId:            "user-timestamp-test",
			LocationId:        "loc-timestamp-test",
			LastUsedAtUnixSec: oldTimestamp,
		}
		_, err = sqlStorage.Insert(context.Background(), userLoc)
		if err != nil {
			t.Fatalf("Failed to create user location: %v", err)
		}

		// Update timestamp
		req := connect.NewRequest(&api.UpdateLocationLastUsedRequest{
			LocationId: "loc-timestamp-test",
		})

		resp, err := service.UpdateLocationLastUsed(ctx, req)
		if err != nil {
			t.Fatalf("UpdateLocationLastUsed failed: %v", err)
		}

		if resp.Msg == nil {
			t.Error("Expected non-nil response")
		}

		// Verify timestamp was updated
		userLocs, err := sqlStorage.QueryByFields(context.Background(), map[string]any{
			"user_id":     "user-timestamp-test",
			"location_id": "loc-timestamp-test",
		}, &models.UserLocation{})
		if err != nil {
			t.Fatalf("Failed to query user location: %v", err)
		}

		if len(userLocs) != 1 {
			t.Fatalf("Expected 1 user location, got %d", len(userLocs))
		}

		updatedUserLoc := userLocs[0].(*models.UserLocation)
		if updatedUserLoc.LastUsedAtUnixSec <= oldTimestamp {
			t.Errorf("Expected timestamp to be updated, old=%d, new=%d",
				oldTimestamp, updatedUserLoc.LastUsedAtUnixSec)
		}
	})

	t.Run("creates association if it doesn't exist", func(t *testing.T) {
		ctx := createAuthContext("user-create-ts-test", "createts@example.com")

		// Create location
		location := &models.Location{
			Id: "loc-create-ts-test",
			Geolocation: &models.Geolocation{
				LatitudeDeg:  37.4220,
				LongitudeDeg: -122.0856,
			},
			Address: &models.Address{
				RegionCode: "US",
				Locality:   "Mountain View",
			},
		}
		_, err := sqlStorage.Insert(context.Background(), location)
		if err != nil {
			t.Fatalf("Failed to create location: %v", err)
		}

		// Update timestamp for non-existent association
		req := connect.NewRequest(&api.UpdateLocationLastUsedRequest{
			LocationId: "loc-create-ts-test",
		})

		resp, err := service.UpdateLocationLastUsed(ctx, req)
		if err != nil {
			t.Fatalf("UpdateLocationLastUsed failed: %v", err)
		}

		if resp.Msg == nil {
			t.Error("Expected non-nil response")
		}

		// Verify association was created
		userLocs, err := sqlStorage.QueryByFields(context.Background(), map[string]any{
			"user_id":     "user-create-ts-test",
			"location_id": "loc-create-ts-test",
		}, &models.UserLocation{})
		if err != nil {
			t.Fatalf("Failed to query user location: %v", err)
		}

		if len(userLocs) != 1 {
			t.Errorf("Expected 1 user location to be created, got %d", len(userLocs))
		}
	})

	t.Run("requires authentication", func(t *testing.T) {
		ctx := context.Background() // No auth

		req := connect.NewRequest(&api.UpdateLocationLastUsedRequest{
			LocationId: "some-location",
		})

		_, err := service.UpdateLocationLastUsed(ctx, req)
		if err == nil {
			t.Error("Expected authentication error, got nil")
		}
	})
}
