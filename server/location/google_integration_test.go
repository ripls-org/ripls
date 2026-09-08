//go:build integration

// google_integration_test.go exercises *GoogleMapsClient against the real
// Google Maps Platform APIs. Skipped unless GOOGLE_MAPS_API_KEY is set
// and the `integration` build tag is enabled (`go test -tags=integration`).
package location

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// requireGoogleKey returns the GOOGLE_MAPS_API_KEY env var or skips the
// test if not set. Keep the helper name distinct from getEnvOrSkip used
// by the Mapbox file (they share the package).
func requireGoogleKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("GOOGLE_MAPS_API_KEY")
	if key == "" {
		t.Skip("skipping integration test - GOOGLE_MAPS_API_KEY not set")
	}
	return key
}

// TestGoogleIntegration_ForwardGeocode resolves a known address via the
// Geocoding API.
func TestGoogleIntegration_ForwardGeocode(t *testing.T) {
	client := NewGoogleMapsClient(requireGoogleKey(t))
	ctx := context.Background()

	address := &models.Address{
		RegionCode:   "US",
		PostalCode:   "94043",
		Locality:     "Mountain View",
		AddressLines: []string{"1600 Amphitheatre Parkway"},
	}

	geo, err := client.ForwardGeocode(ctx, address)
	if err != nil {
		t.Fatalf("ForwardGeocode failed: %v", err)
	}
	if geo == nil {
		t.Fatal("expected non-nil result")
	}

	// Googleplex sits near 37.422°N, 122.084°W.
	if geo.LatitudeDeg < 37.4 || geo.LatitudeDeg > 37.45 {
		t.Errorf("LatitudeDeg = %v, expected ~37.42", geo.LatitudeDeg)
	}
	if geo.LongitudeDeg < -122.1 || geo.LongitudeDeg > -122.05 {
		t.Errorf("LongitudeDeg = %v, expected ~-122.08", geo.LongitudeDeg)
	}
}

// TestGoogleIntegration_SearchPlaces_WithoutProximity exercises the
// Autocomplete + Place Details flow without a location parameter.
func TestGoogleIntegration_SearchPlaces_WithoutProximity(t *testing.T) {
	client := NewGoogleMapsClient(requireGoogleKey(t))
	ctx := context.Background()

	results, err := client.SearchPlaces(ctx, "Golden Gate Bridge", ProximityRequest{}, 3)
	if err != nil {
		t.Fatalf("SearchPlaces failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	first := results[0]
	t.Logf("First result: name=%s, type=%s, address=%s, lat=%f, lon=%f",
		first.Name, first.Type, first.FullAddress,
		first.Coordinates.Latitude, first.Coordinates.Longitude)

	if first.ExternalProvider != providerGoogleMaps {
		t.Errorf("ExternalProvider = %q, want %q", first.ExternalProvider, providerGoogleMaps)
	}
	if first.ExternalID == "" {
		t.Error("expected non-empty ExternalID (Google placeId)")
	}
	// Golden Gate Bridge sits near 37.8°N, 122.5°W.
	if first.Coordinates.Latitude < 37.5 || first.Coordinates.Latitude > 38.0 {
		t.Errorf("latitude out of expected range: %f", first.Coordinates.Latitude)
	}
}

// TestGoogleIntegration_SearchPlaces_ProximityHint biases toward Austin
// for a chain-store query.
func TestGoogleIntegration_SearchPlaces_ProximityHint(t *testing.T) {
	client := NewGoogleMapsClient(requireGoogleKey(t))
	ctx := context.Background()

	austin := &models.Geolocation{LatitudeDeg: 30.2672, LongitudeDeg: -97.7431}
	results, err := client.SearchPlaces(ctx, "coffee", ProximityRequest{
		Coord:     austin,
		Semantics: ProximityHint,
	}, 5)
	if err != nil {
		t.Fatalf("SearchPlaces failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	for i, r := range results {
		t.Logf("Result %d: name=%s, type=%s, lat=%f, lon=%f",
			i, r.Name, r.Type, r.Coordinates.Latitude, r.Coordinates.Longitude)
	}
}

// TestGoogleIntegration_SearchPlaces_ProximityBound verifies that the
// hard-restriction + post-filter combination keeps cross-region results
// out. The audit found "park" → Parker, CO at 70 km when 50 km was
// requested; with our 1.5× post-filter cutoff (75 km), Parker should
// just barely survive at the default radius. Loosen to 30 km to force
// Parker out.
func TestGoogleIntegration_SearchPlaces_ProximityBound(t *testing.T) {
	client := NewGoogleMapsClient(requireGoogleKey(t))
	ctx := context.Background()

	boulder := &models.Geolocation{LatitudeDeg: 40.0150, LongitudeDeg: -105.2705}
	results, err := client.SearchPlaces(ctx, "park", ProximityRequest{
		Coord:     boulder,
		RadiusM:   30_000, // 30km × 1.5 = 45km cutoff; Parker (70km) drops
		Semantics: ProximityBound,
	}, 5)
	if err != nil {
		t.Fatalf("SearchPlaces failed: %v", err)
	}
	for i, r := range results {
		t.Logf("Result %d: name=%s, lat=%f, lon=%f",
			i, r.Name, r.Coordinates.Latitude, r.Coordinates.Longitude)
		// Anything > 45 km from Boulder should have been dropped.
		d := haversineMeters(
			boulder.LatitudeDeg, boulder.LongitudeDeg,
			r.Coordinates.Latitude, r.Coordinates.Longitude,
		)
		if d > 45_000 {
			t.Errorf("result %d (%s) is %.1f km from Boulder; post-filter should have dropped it",
				i, r.Name, d/1000)
		}
	}
}

// TestGoogleIntegration_ReverseGeocode verifies the result_type filter and
// Plus-Code suppression on an urban coordinate.
func TestGoogleIntegration_ReverseGeocode(t *testing.T) {
	client := NewGoogleMapsClient(requireGoogleKey(t))
	ctx := context.Background()

	// Pearl Street Mall, Boulder.
	place, err := client.ReverseGeocode(ctx, &models.Geolocation{
		LatitudeDeg:  40.0177,
		LongitudeDeg: -105.2799,
	})
	if err != nil {
		t.Fatalf("ReverseGeocode failed: %v", err)
	}
	if place == nil {
		t.Fatal("expected non-nil place")
	}
	t.Logf("Place: name=%s, type=%s, full=%s, locality=%s, region=%s",
		place.Name, place.Type, place.FullAddress, place.Locality, place.RegionCode)

	if place.ExternalProvider != providerGoogleMaps {
		t.Errorf("ExternalProvider = %q, want %q", place.ExternalProvider, providerGoogleMaps)
	}
	if place.Locality != "Boulder" {
		t.Errorf("Locality = %q, want 'Boulder'", place.Locality)
	}
	// The audit recommended preferring real addresses over Plus Codes
	// and businesses; the formatted address must not look like a Plus
	// Code (e.g. "H629+25 ...").
	if strings.Contains(place.FullAddress, "+") && len(place.FullAddress) < 30 {
		t.Errorf("FullAddress looks like a Plus Code, expected a street address: %q", place.FullAddress)
	}
}

// TestGoogleIntegration_CheckHealth_Valid verifies that a real API key
// reports healthy.
func TestGoogleIntegration_CheckHealth_Valid(t *testing.T) {
	client := NewGoogleMapsClient(requireGoogleKey(t))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	statuses, err := client.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("CheckHealth returned go error: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if !statuses[0].IsHealthy() {
		t.Errorf("expected healthy, got error: %q", statuses[0].Error)
	}
}

// TestGoogleIntegration_CheckHealth_InvalidKey verifies that an invalid
// API key surfaces an error in Status.Error (not a Go error).
func TestGoogleIntegration_CheckHealth_InvalidKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test in short mode")
	}

	client := NewGoogleMapsClient("AIza-invalid-key-for-testing")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	statuses, err := client.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("CheckHealth returned go error: %v", err)
	}
	if len(statuses) == 0 {
		t.Fatal("expected at least one status")
	}
	if statuses[0].Error == "" {
		t.Error("expected non-empty Error for invalid API key")
	}
	t.Logf("CheckHealth correctly reported error for invalid key: %q", statuses[0].Error)
}
