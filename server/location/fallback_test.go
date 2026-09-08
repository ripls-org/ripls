package location

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// TestGetUserLocationFallback_PrimaryResidence tests the primary residence fallback path.
func TestGetUserLocationFallback_PrimaryResidence(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create test location
	location := &models.Location{
		Name: proto.String("Home Sweet Home"),
		Geolocation: &models.Geolocation{
			LatitudeDeg:  37.7749,
			LongitudeDeg: -122.4194,
		},
		Address: &models.Address{
			Locality:     "San Francisco",
			RegionCode:   "CA",
			PostalCode:   "94102",
			AddressLines: []string{"123 Market St"},
		},
	}
	locationID, err := store.Insert(ctx, location)
	if err != nil {
		t.Fatalf("Failed to create location: %v", err)
	}

	// Create user with primary residence
	user := &models.User{
		Email:                      "test@example.com",
		PrimaryResidenceLocationId: locationID,
	}
	userID, err := store.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Test the fallback
	resultLocationID, geocoded, err := GetUserLocationFallback(ctx, store, userID)
	if err != nil {
		t.Fatalf("GetUserLocationFallback failed: %v", err)
	}

	// Verify results
	if resultLocationID != locationID {
		t.Errorf("expected location ID %s, got %s", locationID, resultLocationID)
	}
	if geocoded == nil {
		t.Fatal("expected geocoded location, got nil")
	}
	if geocoded.Name != "Home Sweet Home" {
		t.Errorf("expected name 'Home Sweet Home', got %s", geocoded.Name)
	}
	if geocoded.LatitudeDeg != 37.7749 {
		t.Errorf("expected latitude 37.7749, got %f", geocoded.LatitudeDeg)
	}
	if geocoded.LongitudeDeg != -122.4194 {
		t.Errorf("expected longitude -122.4194, got %f", geocoded.LongitudeDeg)
	}
	if geocoded.Locality != "San Francisco" {
		t.Errorf("expected locality 'San Francisco', got %s", geocoded.Locality)
	}
}

// TestGetUserLocationFallback_MostRecentLocation tests fallback to most recent user_location.
func TestGetUserLocationFallback_MostRecentLocation(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create test locations
	location1 := &models.Location{
		Name: proto.String("Old Location"),
		Geolocation: &models.Geolocation{
			LatitudeDeg:  37.3861,
			LongitudeDeg: -122.0839,
		},
		Address: &models.Address{
			Locality:   "Mountain View",
			RegionCode: "CA",
		},
	}
	location2 := &models.Location{
		Name: proto.String("Recent Location"),
		Geolocation: &models.Geolocation{
			LatitudeDeg:  37.7749,
			LongitudeDeg: -122.4194,
		},
		Address: &models.Address{
			Locality:   "San Francisco",
			RegionCode: "CA",
		},
	}

	locationID1, err := store.Insert(ctx, location1)
	if err != nil {
		t.Fatalf("Failed to create location1: %v", err)
	}
	locationID2, err := store.Insert(ctx, location2)
	if err != nil {
		t.Fatalf("Failed to create location2: %v", err)
	}

	// Create user WITHOUT primary residence
	user := &models.User{
		Email: "test@example.com",
	}
	userID, err := store.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create user_locations with different timestamps
	userLoc1 := &models.UserLocation{
		UserId:            userID,
		LocationId:        locationID1,
		LastUsedAtUnixSec: 1000,
	}
	userLoc2 := &models.UserLocation{
		UserId:            userID,
		LocationId:        locationID2,
		LastUsedAtUnixSec: 2000, // More recent
	}

	_, err = store.Insert(ctx, userLoc1)
	if err != nil {
		t.Fatalf("Failed to create userLoc1: %v", err)
	}
	_, err = store.Insert(ctx, userLoc2)
	if err != nil {
		t.Fatalf("Failed to create userLoc2: %v", err)
	}

	// Test the fallback - should get the most recent location
	resultLocationID, geocoded, err := GetUserLocationFallback(ctx, store, userID)
	if err != nil {
		t.Fatalf("GetUserLocationFallback failed: %v", err)
	}

	// Verify we got the most recent location
	if resultLocationID != locationID2 {
		t.Errorf("expected location ID %s, got %s", locationID2, resultLocationID)
	}
	if geocoded == nil {
		t.Fatal("expected geocoded location, got nil")
	}
	if geocoded.Name != "Recent Location" {
		t.Errorf("expected name 'Recent Location', got %s", geocoded.Name)
	}
}

// TestGetUserLocationFallback_NoLocations tests when user has no locations at all.
func TestGetUserLocationFallback_NoLocations(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create user without primary residence
	user := &models.User{
		Email: "test@example.com",
	}
	userID, err := store.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Test the fallback - should return nil since no locations exist
	locationID, geocoded, err := GetUserLocationFallback(ctx, store, userID)
	if err != nil {
		t.Fatalf("GetUserLocationFallback failed: %v", err)
	}

	if locationID != "" {
		t.Errorf("expected empty location ID, got %s", locationID)
	}
	if geocoded != nil {
		t.Errorf("expected nil geocoded location, got %v", geocoded)
	}
}

// TestGetUserLocationFallback_UserNotFound tests when user doesn't exist.
func TestGetUserLocationFallback_UserNotFound(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Test with non-existent user - should return nil (no locations)
	locationID, geocoded, err := GetUserLocationFallback(ctx, store, "nonexistent-user")
	if err != nil {
		t.Fatalf("GetUserLocationFallback failed: %v", err)
	}

	if locationID != "" {
		t.Errorf("expected empty location ID, got %s", locationID)
	}
	if geocoded != nil {
		t.Errorf("expected nil geocoded location, got %v", geocoded)
	}
}

// TestGetUserLocationFallback_PrimaryResidenceMissing tests when primary residence location is missing.
func TestGetUserLocationFallback_PrimaryResidenceMissing(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create user with primary residence ID that doesn't exist
	user := &models.User{
		Email:                      "test@example.com",
		PrimaryResidenceLocationId: "nonexistent-location",
	}
	userID, err := store.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Test the fallback - should return error since location doesn't exist
	_, _, err = GetUserLocationFallback(ctx, store, userID)
	if err == nil {
		t.Error("expected error for missing location, got nil")
	}
}

// TestGetUserLocationFallbackLogged_PrimaryResidence verifies the logged variant returns
// the same location/geocoded values as the error-returning variant on the happy path.
func TestGetUserLocationFallbackLogged_PrimaryResidence(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	loc := &models.Location{
		Name: proto.String("Home"),
		Geolocation: &models.Geolocation{
			LatitudeDeg:  37.7749,
			LongitudeDeg: -122.4194,
		},
		Address: &models.Address{
			Locality:   "San Francisco",
			RegionCode: "CA",
		},
	}
	locationID, err := store.Insert(ctx, loc)
	if err != nil {
		t.Fatalf("Failed to create location: %v", err)
	}

	user := &models.User{
		Email:                      "test@example.com",
		PrimaryResidenceLocationId: locationID,
	}
	userID, err := store.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	gotID, geocoded := GetUserLocationFallbackLogged(ctx, store, userID)
	if gotID != locationID {
		t.Errorf("expected location ID %s, got %s", locationID, gotID)
	}
	if geocoded == nil {
		t.Fatal("expected non-nil geocoded location")
	}
	if geocoded.Name != "Home" {
		t.Errorf("expected name 'Home', got %s", geocoded.Name)
	}
}

// TestGetUserLocationFallbackLogged_NoLocations verifies the logged variant returns
// empty values (no panic, no error) when the user has no locations.
func TestGetUserLocationFallbackLogged_NoLocations(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	user := &models.User{Email: "test@example.com"}
	userID, err := store.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	gotID, geocoded := GetUserLocationFallbackLogged(ctx, store, userID)
	if gotID != "" {
		t.Errorf("expected empty location ID, got %s", gotID)
	}
	if geocoded != nil {
		t.Errorf("expected nil geocoded location, got %v", geocoded)
	}
}

// TestGetUserLocationFallbackLogged_PrimaryResidenceMissing verifies that when the
// underlying GetUserLocationFallback returns an error (primary-residence id points to
// a deleted location), the logged variant swallows the error and returns empty values.
// The error is logged at Warn level inside the wrapper rather than surfaced to the caller.
func TestGetUserLocationFallbackLogged_PrimaryResidenceMissing(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	user := &models.User{
		Email:                      "test@example.com",
		PrimaryResidenceLocationId: "nonexistent-location",
	}
	userID, err := store.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	gotID, geocoded := GetUserLocationFallbackLogged(ctx, store, userID)
	if gotID != "" {
		t.Errorf("expected empty location ID on missing-location error, got %s", gotID)
	}
	if geocoded != nil {
		t.Errorf("expected nil geocoded location on missing-location error, got %v", geocoded)
	}
}

// TestGenerateLocationDisplayName tests all display name generation paths.
func TestGenerateLocationDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		location *models.Location
		want     string
	}{
		{
			name: "uses existing name",
			location: &models.Location{
				Name: proto.String("My House"),
				Address: &models.Address{
					AddressLines: []string{"123 Main St"},
					Locality:     "Springfield",
					RegionCode:   "IL",
				},
			},
			want: "My House",
		},
		{
			name: "uses first address line when no name",
			location: &models.Location{
				Address: &models.Address{
					AddressLines: []string{"456 Oak Ave", "Apt 2B"},
					Locality:     "Portland",
					RegionCode:   "OR",
				},
			},
			want: "456 Oak Ave",
		},
		{
			name: "uses locality and region when no address lines",
			location: &models.Location{
				Address: &models.Address{
					AddressLines: []string{},
					Locality:     "Seattle",
					RegionCode:   "WA",
				},
			},
			want: "Seattle, WA",
		},
		{
			name: "uses locality only when no region code",
			location: &models.Location{
				Address: &models.Address{
					AddressLines: []string{},
					Locality:     "Boston",
					RegionCode:   "",
				},
			},
			want: "Boston",
		},
		{
			name: "uses default fallback when no address info",
			location: &models.Location{
				Address: &models.Address{
					AddressLines: []string{},
					Locality:     "",
					RegionCode:   "",
				},
			},
			want: "Your Location",
		},
		{
			name: "ignores empty address lines",
			location: &models.Location{
				Address: &models.Address{
					AddressLines: []string{""},
					Locality:     "Denver",
					RegionCode:   "CO",
				},
			},
			want: "Denver, CO",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateLocationDisplayName(tt.location)
			if got != tt.want {
				t.Errorf("GenerateLocationDisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestGetUserLocationFallback_PrimaryResidencePriority tests that primary residence takes priority.
func TestGetUserLocationFallback_PrimaryResidencePriority(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create two locations
	primaryLoc := &models.Location{
		Name: proto.String("Primary Home"),
		Geolocation: &models.Geolocation{
			LatitudeDeg:  37.7749,
			LongitudeDeg: -122.4194,
		},
		Address: &models.Address{
			Locality:   "San Francisco",
			RegionCode: "CA",
		},
	}
	recentLoc := &models.Location{
		Name: proto.String("Recent Place"),
		Geolocation: &models.Geolocation{
			LatitudeDeg:  37.3861,
			LongitudeDeg: -122.0839,
		},
		Address: &models.Address{
			Locality:   "Mountain View",
			RegionCode: "CA",
		},
	}

	primaryLocID, err := store.Insert(ctx, primaryLoc)
	if err != nil {
		t.Fatalf("Failed to create primary location: %v", err)
	}
	recentLocID, err := store.Insert(ctx, recentLoc)
	if err != nil {
		t.Fatalf("Failed to create recent location: %v", err)
	}

	// Create user with primary residence
	user := &models.User{
		Email:                      "test@example.com",
		PrimaryResidenceLocationId: primaryLocID,
	}
	userID, err := store.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create a more recent user_location
	userLoc := &models.UserLocation{
		UserId:            userID,
		LocationId:        recentLocID,
		LastUsedAtUnixSec: 9999999, // Very recent
	}
	_, err = store.Insert(ctx, userLoc)
	if err != nil {
		t.Fatalf("Failed to create userLoc: %v", err)
	}

	// Test the fallback - should prioritize primary residence over more recent location
	resultLocationID, geocoded, err := GetUserLocationFallback(ctx, store, userID)
	if err != nil {
		t.Fatalf("GetUserLocationFallback failed: %v", err)
	}

	// Verify we got the primary residence, not the more recent location
	if resultLocationID != primaryLocID {
		t.Errorf("expected primary location ID %s, got %s", primaryLocID, resultLocationID)
	}
	if geocoded.Name != "Primary Home" {
		t.Errorf("expected name 'Primary Home', got %s", geocoded.Name)
	}
}

// TestGetUserLocationFallback_GeneratesDisplayName tests display name generation for locations without names.
func TestGetUserLocationFallback_GeneratesDisplayName(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create location without a name
	location := &models.Location{
		Geolocation: &models.Geolocation{
			LatitudeDeg:  37.7749,
			LongitudeDeg: -122.4194,
		},
		Address: &models.Address{
			Locality:     "San Francisco",
			RegionCode:   "CA",
			PostalCode:   "94102",
			AddressLines: []string{"789 Mission St"},
		},
	}
	locationID, err := store.Insert(ctx, location)
	if err != nil {
		t.Fatalf("Failed to create location: %v", err)
	}

	// Create user with this location as primary residence
	user := &models.User{
		Email:                      "test@example.com",
		PrimaryResidenceLocationId: locationID,
	}
	userID, err := store.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Test the fallback
	resultLocationID, geocoded, err := GetUserLocationFallback(ctx, store, userID)
	if err != nil {
		t.Fatalf("GetUserLocationFallback failed: %v", err)
	}

	// Verify display name was generated from address
	if resultLocationID != locationID {
		t.Errorf("expected location ID %s, got %s", locationID, resultLocationID)
	}
	if geocoded.Name != "789 Mission St" {
		t.Errorf("expected generated name '789 Mission St', got %s", geocoded.Name)
	}
}
