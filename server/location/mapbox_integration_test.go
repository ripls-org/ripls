package location

import (
	"context"
	"os"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestMapboxIntegration_ForwardGeocode tests forward geocoding with the real Mapbox API.
// Requires MAPBOX_ACCESS_TOKEN environment variable to be set.
func TestMapboxIntegration_ForwardGeocode(t *testing.T) {
	accessToken := os.Getenv("MAPBOX_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("Skipping integration test - MAPBOX_ACCESS_TOKEN not set")
	}

	client := NewMapboxClient(accessToken)
	ctx := context.Background()

	// Test with a well-known address: Googleplex
	address := &models.Address{
		RegionCode:   "US",
		PostalCode:   "94043",
		Locality:     "Mountain View",
		AddressLines: []string{"1600 Amphitheatre Parkway"},
	}

	result, err := client.ForwardGeocode(ctx, address)
	if err != nil {
		t.Fatalf("ForwardGeocode failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Verify coordinates are in reasonable range for Mountain View, CA
	// Approximate location: 37.4°N, 122.1°W
	if result.LatitudeDeg < 37.0 || result.LatitudeDeg > 38.0 {
		t.Errorf("latitude out of expected range for Mountain View: %f", result.LatitudeDeg)
	}
	if result.LongitudeDeg < -123.0 || result.LongitudeDeg > -121.0 {
		t.Errorf("longitude out of expected range for Mountain View: %f", result.LongitudeDeg)
	}

	t.Logf("Forward geocoding result: lat=%f, lon=%f", result.LatitudeDeg, result.LongitudeDeg)
}

// TestMapboxIntegration_ReverseGeocode tests reverse geocoding with the real Mapbox API.
// Requires MAPBOX_ACCESS_TOKEN environment variable to be set.
func TestMapboxIntegration_ReverseGeocode(t *testing.T) {
	accessToken := os.Getenv("MAPBOX_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("Skipping integration test - MAPBOX_ACCESS_TOKEN not set")
	}

	client := NewMapboxClient(accessToken)
	ctx := context.Background()

	// A public landmark (Royal Observatory, Greenwich) rather than anywhere
	// anyone lives. These used to be the coordinates lifted out of
	// books_exif.jpg, which was a real photo from a real phone (#2953); that
	// fixture's GPS is now synthetic and points here too.
	geolocation := &models.Geolocation{
		LatitudeDeg:  51.4779,
		LongitudeDeg: -0.0015,
	}

	result, err := client.ReverseGeocode(ctx, geolocation)
	if err != nil {
		t.Fatalf("ReverseGeocode failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Verify we got something back from the API
	if result.RegionCode == "" && result.Locality == "" && result.FullAddress == "" {
		t.Error("expected at least some address information")
	}

	t.Logf("Reverse geocoding result: region=%s, locality=%s, postal=%s, address=%s, mapbox_id=%s",
		result.RegionCode, result.Locality, result.PostalCode, result.FullAddress, result.ExternalID)
}

// TestMapboxIntegration_RoundTrip tests forward then reverse geocoding.
// Requires MAPBOX_ACCESS_TOKEN environment variable to be set.
func TestMapboxIntegration_RoundTrip(t *testing.T) {
	accessToken := os.Getenv("MAPBOX_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("Skipping integration test - MAPBOX_ACCESS_TOKEN not set")
	}

	client := NewMapboxClient(accessToken)
	ctx := context.Background()

	// Start with an address
	originalAddress := &models.Address{
		RegionCode:   "US",
		Locality:     "Austin",
		PostalCode:   "78701",
		AddressLines: []string{"100 Congress Ave"},
	}

	// Forward geocode to get coordinates
	coords, err := client.ForwardGeocode(ctx, originalAddress)
	if err != nil {
		t.Fatalf("ForwardGeocode failed: %v", err)
	}

	t.Logf("Forward geocode result: lat=%f, lon=%f", coords.LatitudeDeg, coords.LongitudeDeg)

	// Reverse geocode back to address
	reversedAddress, err := client.ReverseGeocode(ctx, coords)
	if err != nil {
		t.Fatalf("ReverseGeocode failed: %v", err)
	}

	// We don't expect exact match, but should get Austin, TX back
	if reversedAddress.Locality != "Austin" {
		t.Logf("Note: locality changed from %s to %s", originalAddress.Locality, reversedAddress.Locality)
	}

	t.Logf("Reverse geocode result: region=%s, locality=%s, postal=%s, address=%s",
		reversedAddress.RegionCode, reversedAddress.Locality, reversedAddress.PostalCode, reversedAddress.FullAddress)
}

// TestMapboxIntegration_SearchPlaces tests place search with the real Mapbox API.
// Requires MAPBOX_ACCESS_TOKEN environment variable to be set.
func TestMapboxIntegration_SearchPlaces(t *testing.T) {
	accessToken := os.Getenv("MAPBOX_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("Skipping integration test - MAPBOX_ACCESS_TOKEN not set")
	}

	client := NewMapboxClient(accessToken)
	ctx := context.Background()

	t.Run("SearchWithoutProximity", func(t *testing.T) {
		// Search for a well-known landmark
		results, err := client.SearchPlaces(ctx, "Golden Gate Bridge", ProximityRequest{}, 5)
		if err != nil {
			t.Fatalf("SearchPlaces failed: %v", err)
		}

		if len(results) == 0 {
			t.Fatal("expected at least one result")
		}

		// First result should be the Golden Gate Bridge
		firstResult := results[0]
		t.Logf("First result: name=%s, type=%s, address=%s, lat=%f, lon=%f, confidence=%f",
			firstResult.Name, firstResult.Type, firstResult.FullAddress,
			firstResult.Coordinates.Latitude, firstResult.Coordinates.Longitude,
			firstResult.Confidence)

		// Verify coordinates are in San Francisco area (approximately 37.8°N, 122.5°W)
		if firstResult.Coordinates.Latitude < 37.0 || firstResult.Coordinates.Latitude > 38.0 {
			t.Errorf("latitude out of expected range for San Francisco: %f", firstResult.Coordinates.Latitude)
		}
		if firstResult.Coordinates.Longitude < -123.0 || firstResult.Coordinates.Longitude > -122.0 {
			t.Errorf("longitude out of expected range for San Francisco: %f", firstResult.Coordinates.Longitude)
		}

		// Verify confidence decreases with result position
		for i := 1; i < len(results); i++ {
			if results[i].Confidence >= results[i-1].Confidence {
				t.Errorf("expected decreasing confidence: result[%d]=%f >= result[%d]=%f",
					i, results[i].Confidence, i-1, results[i-1].Confidence)
			}
		}
	})

	t.Run("SearchWithProximity", func(t *testing.T) {
		// Search for coffee shops near Austin, TX
		austinProximity := &models.Geolocation{
			LatitudeDeg:  30.2672,
			LongitudeDeg: -97.7431,
		}

		results, err := client.SearchPlaces(ctx, "coffee", ProximityRequest{
			Coord:     austinProximity,
			Semantics: ProximityHint,
		}, 3)
		if err != nil {
			t.Fatalf("SearchPlaces failed: %v", err)
		}

		if len(results) == 0 {
			t.Fatal("expected at least one result")
		}

		// Log all results
		for i, result := range results {
			t.Logf("Result %d: name=%s, type=%s, address=%s, lat=%f, lon=%f, confidence=%f",
				i+1, result.Name, result.Type, result.FullAddress,
				result.Coordinates.Latitude, result.Coordinates.Longitude,
				result.Confidence)
		}

		// Verify we got at most 3 results
		if len(results) > 3 {
			t.Errorf("expected at most 3 results, got %d", len(results))
		}
	})

	t.Run("SearchWithDefaultLimit", func(t *testing.T) {
		// Test with limit=0 should default to 10
		results, err := client.SearchPlaces(ctx, "pizza", ProximityRequest{}, 0)
		if err != nil {
			t.Fatalf("SearchPlaces failed: %v", err)
		}

		if len(results) == 0 {
			t.Fatal("expected at least one result")
		}

		// Should return up to 10 results (default)
		if len(results) > 10 {
			t.Errorf("expected at most 10 results with default limit, got %d", len(results))
		}

		t.Logf("Got %d results with default limit", len(results))
	})

	t.Run("EmptyQuery", func(t *testing.T) {
		// Test with empty query should return error
		_, err := client.SearchPlaces(ctx, "", ProximityRequest{}, 5)
		if err == nil {
			t.Fatal("expected error for empty query")
		}
		t.Logf("Got expected error for empty query: %v", err)
	})
}
