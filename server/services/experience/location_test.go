package experience

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/webfetch"
)

// TestService_GenExperience_LocationExtraction tests location extraction and geocoding.
func TestService_GenExperience_LocationExtraction(t *testing.T) {
	service, testStorage, bucket := setupTestService(t)
	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	t.Run("extract and geocode location", func(t *testing.T) {
		// Setup mock AI provider with location query
		mockProvider := newMockAIProvider(
			&ai.ExperienceGeneration{
				Title:         "Coffee Meetup",
				Description:   "Let's grab coffee and chat",
				Confidence:    0.90,
				LocationQuery: "The Cup Boulder",
			},
			nil,
			nil,
		)
		service.aiProvider = mockProvider

		// Setup mock Mapbox client with full PlaceResult (external ID + context).
		mockMapbox := &mockLocationProvider{
			searchResults: []location.PlaceResult{
				{
					Name:               "The Cup",
					FullAddress:        "1801 13th St, Boulder, CO 80302",
					Type:               "POI",
					Confidence:         0.95,
					ExternalID:         "poi.thecup-boulder",
					ExternalProvider:   "mapbox",
					Locality:           "Boulder",
					RegionCode:         "CO",
					PostalCode:         "80302",
					Neighborhood:       "Downtown Boulder",
					AdministrativeArea: "Colorado",
				},
			},
		}
		service.locationProvider = mockMapbox

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt:       &api.GenExperienceRequest_Text{Text: "coffee at The Cup in Boulder"},
			LatitudeDeg:  40.0150,
			LongitudeDeg: -105.2705,
		})

		resp, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		// Verify location query was extracted
		if resp.Msg.LocationQuery != "The Cup Boulder" {
			t.Errorf("Expected location query 'The Cup Boulder', got '%s'", resp.Msg.LocationQuery)
		}

		// Verify geocoding was attempted with proximity
		if mockMapbox.lastQuery != "The Cup Boulder" {
			t.Errorf("Expected geocoding query 'The Cup Boulder', got '%s'", mockMapbox.lastQuery)
		}
		if mockMapbox.lastProximity() == nil {
			t.Error("Expected proximity to be set")
		} else {
			if mockMapbox.lastProximity().LatitudeDeg != 40.0150 {
				t.Errorf("Expected proximity latitude 40.0150, got %f", mockMapbox.lastProximity().LatitudeDeg)
			}
			if mockMapbox.lastProximity().LongitudeDeg != -105.2705 {
				t.Errorf("Expected proximity longitude -105.2705, got %f", mockMapbox.lastProximity().LongitudeDeg)
			}
		}
		// GPS-supplied proximity uses Hint (soft bias) so named distant POIs
		// the user explicitly typed still resolve; only event-coords promote
		// to Bound.
		if mockMapbox.lastProximityReq.Semantics != location.ProximityHint {
			t.Errorf("Expected ProximityHint semantics, got %v", mockMapbox.lastProximityReq.Semantics)
		}

		// Verify the full GeocodedLocation payload flows through to the client.
		gl := resp.Msg.GeocodedLocation
		if gl == nil {
			t.Fatal("Expected GeocodedLocation in response")
		}
		if gl.ExternalPlaceId != "poi.thecup-boulder" {
			t.Errorf("ExternalPlaceId = %q, want 'poi.thecup-boulder'", gl.ExternalPlaceId)
		}
		if gl.ExternalPlaceProvider != "mapbox" {
			t.Errorf("ExternalPlaceProvider = %q, want 'mapbox'", gl.ExternalPlaceProvider)
		}
		if gl.Locality != "Boulder" {
			t.Errorf("Locality = %q, want 'Boulder'", gl.Locality)
		}
		if gl.RegionCode != "CO" {
			t.Errorf("RegionCode = %q, want 'CO'", gl.RegionCode)
		}
		if gl.PostalCode != "80302" {
			t.Errorf("PostalCode = %q, want '80302'", gl.PostalCode)
		}
		if gl.Neighborhood != "Downtown Boulder" {
			t.Errorf("Neighborhood = %q, want 'Downtown Boulder'", gl.Neighborhood)
		}
		if gl.AdministrativeArea != "Colorado" {
			t.Errorf("AdministrativeArea = %q, want 'Colorado'", gl.AdministrativeArea)
		}
	})

	t.Run("handle failed geocoding gracefully", func(t *testing.T) {
		// Setup mock AI provider with location query
		mockProvider := newMockAIProvider(
			&ai.ExperienceGeneration{
				Title:         "Park Picnic",
				Description:   "Outdoor lunch gathering",
				Confidence:    0.88,
				LocationQuery: "Nonexistent Place XYZ",
			},
			nil,
			nil,
		)
		service.aiProvider = mockProvider

		// Setup mock Mapbox client that returns error
		mockMapbox := &mockLocationProvider{
			searchError: fmt.Errorf("location not found"),
		}
		service.locationProvider = mockMapbox

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_Text{Text: "picnic at Nonexistent Place XYZ"},
		})

		// Should succeed despite geocoding failure
		resp, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience should not fail when geocoding fails: %v", err)
		}

		// Verify location query is still returned (text-only fallback)
		if resp.Msg.LocationQuery != "Nonexistent Place XYZ" {
			t.Errorf("Expected location query 'Nonexistent Place XYZ', got '%s'", resp.Msg.LocationQuery)
		}

		// Verify location ID is empty (geocoding failed)
		if resp.Msg.LocationId != "" {
			t.Errorf("Expected empty location ID when geocoding fails, got '%s'", resp.Msg.LocationId)
		}
	})

	t.Run("skip geocoding when no mapbox client", func(t *testing.T) {
		serviceNoMapbox, _, _ := setupTestService(t)

		// Setup mock AI provider with location query
		mockProvider := newMockAIProvider(
			&ai.ExperienceGeneration{
				Title:         "Library Study",
				Description:   "Study session at library",
				Confidence:    0.91,
				LocationQuery: "Boulder Public Library",
			},
			nil,
			nil,
		)
		serviceNoMapbox.aiProvider = mockProvider
		// Don't set mapboxClient

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_Text{Text: "study at Boulder Public Library"},
		})

		resp, err := serviceNoMapbox.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		// Verify location query is still returned
		if resp.Msg.LocationQuery != "Boulder Public Library" {
			t.Errorf("Expected location query 'Boulder Public Library', got '%s'", resp.Msg.LocationQuery)
		}

		// Verify location ID is empty (no geocoding attempted)
		if resp.Msg.LocationId != "" {
			t.Errorf("Expected empty location ID when no mapbox client, got '%s'", resp.Msg.LocationId)
		}
	})

	t.Run("geocode without proximity", func(t *testing.T) {
		// Setup mock AI provider with location query
		mockProvider := newMockAIProvider(
			&ai.ExperienceGeneration{
				Title:         "Park Meetup",
				Description:   "Let's meet at the park",
				Confidence:    0.87,
				LocationQuery: "Central Park",
			},
			nil,
			nil,
		)
		service.aiProvider = mockProvider

		// Setup mock Mapbox client
		mockMapbox := &mockLocationProvider{
			searchResults: []location.PlaceResult{
				{
					Name:        "Central Park",
					FullAddress: "Central Park, New York, NY",
					Type:        "Place",
					Confidence:  0.92,
				},
			},
		}
		service.locationProvider = mockMapbox

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_Text{Text: "meetup at Central Park"},
			// No lat/lng provided
		})

		resp, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		// Verify location query was extracted
		if resp.Msg.LocationQuery != "Central Park" {
			t.Errorf("Expected location query 'Central Park', got '%s'", resp.Msg.LocationQuery)
		}

		// Verify geocoding was attempted without proximity
		if mockMapbox.lastQuery != "Central Park" {
			t.Errorf("Expected geocoding query 'Central Park', got '%s'", mockMapbox.lastQuery)
		}
		if mockMapbox.lastProximity() != nil {
			t.Errorf("Expected nil proximity, got %+v", mockMapbox.lastProximity())
		}
	})

	_ = testStorage
	_ = bucket
}

// TestService_GenExperience_StructuredLocationOverride verifies that JSON-LD
// location data overrides LLM-extracted location queries.
func TestService_GenExperience_StructuredLocationOverride(t *testing.T) {
	service, _, _ := setupTestService(t)
	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	t.Run("structured location overrides LLM location with POI match", func(t *testing.T) {
		lat := 40.016722
		lng := -105.281713
		startDate := time.Date(2026, 6, 20, 18, 30, 0, 0, time.FixedZone("MDT", -6*60*60))

		// Mock web fetcher returns page with JSON-LD event data.
		mockFetcher := &webfetch.MockFetcher{
			FetchPageContentFunc: func(ctx context.Context, url string) (*webfetch.PageContent, error) {
				return &webfetch.PageContent{
					URL:      url,
					Title:    "1940s Ball at Boulder Airport",
					BodyText: "Join us for a 1940s ball at the airport",
					Event: &webfetch.StructuredEventData{
						Name:         "1940s Ball at Boulder Airport",
						StartDate:    &startDate,
						Location:     "Boulder Airport, 3393 Airport Rd, Boulder, CO 80302",
						LocationName: "Boulder Airport",
						Latitude:     &lat,
						Longitude:    &lng,
					},
				}, nil
			},
		}
		service.SetWebFetcher(mockFetcher)

		// Mock AI provider returns a DIFFERENT location query than JSON-LD.
		mockProvider := newMockAIProvider(nil, nil, &ai.ExperienceGeneration{
			Title:          "1940s Ball",
			Description:    "A nostalgic evening",
			Confidence:     0.90,
			LocationQuery:  "Boulder Airport area", // LLM's less precise extraction
			Date:           "2026-06-20",
			Time:           "18:30",
			TimeConfidence: "EXPLICIT",
		})
		service.aiProvider = mockProvider

		// Mock Mapbox returns POI match — name should be used as-is.
		mockMapbox := &mockLocationProvider{
			searchResults: []location.PlaceResult{
				{
					Name:        "Boulder Airport",
					FullAddress: "3393 Airport Rd, Boulder, CO 80302",
					Type:        "POI",
					Confidence:  0.95,
				},
			},
		}
		service.locationProvider = mockMapbox

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_WebsiteUrl{
				WebsiteUrl: "https://www.eventbrite.com/e/1940s-ball",
			},
			CurrentTimeUnixSec: 1718000000,
		})

		resp, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		// Verify structured location overrode LLM extraction.
		if mockMapbox.lastQuery != "Boulder Airport, 3393 Airport Rd, Boulder, CO 80302" {
			t.Errorf("Expected Mapbox query from JSON-LD, got %q", mockMapbox.lastQuery)
		}

		// Verify structured geo coordinates were used as proximity bias.
		if mockMapbox.lastProximity() == nil {
			t.Fatal("Expected proximity bias from JSON-LD geo coordinates")
		}
		if mockMapbox.lastProximity().LatitudeDeg != 40.016722 {
			t.Errorf("Proximity latitude = %f, want 40.016722", mockMapbox.lastProximity().LatitudeDeg)
		}
		if mockMapbox.lastProximity().LongitudeDeg != -105.281713 {
			t.Errorf("Proximity longitude = %f, want -105.281713", mockMapbox.lastProximity().LongitudeDeg)
		}

		// POI match: display name should be the Mapbox POI name.
		if resp.Msg.GeocodedLocation.Name != "Boulder Airport" {
			t.Errorf("GeocodedLocation.Name = %q, want 'Boulder Airport'", resp.Msg.GeocodedLocation.Name)
		}
	})

	t.Run("address match uses JSON-LD place name as display name", func(t *testing.T) {
		lat := 40.016722
		lng := -105.281713
		startDate := time.Date(2026, 6, 20, 18, 30, 0, 0, time.FixedZone("MDT", -6*60*60))

		mockFetcher := &webfetch.MockFetcher{
			FetchPageContentFunc: func(ctx context.Context, url string) (*webfetch.PageContent, error) {
				return &webfetch.PageContent{
					URL:      url,
					Title:    "1940s Ball at Boulder Airport",
					BodyText: "Join us for a 1940s ball at the airport",
					Event: &webfetch.StructuredEventData{
						Name:         "1940s Ball at Boulder Airport",
						StartDate:    &startDate,
						Location:     "Boulder Airport, 3393 Airport Rd, Boulder, CO 80302",
						LocationName: "Boulder Airport",
						Latitude:     &lat,
						Longitude:    &lng,
					},
				}, nil
			},
		}
		service.SetWebFetcher(mockFetcher)

		mockProvider := newMockAIProvider(nil, nil, &ai.ExperienceGeneration{
			Title:          "1940s Ball",
			Description:    "A nostalgic evening",
			Confidence:     0.90,
			LocationQuery:  "Boulder Airport area",
			Date:           "2026-06-20",
			Time:           "18:30",
			TimeConfidence: "EXPLICIT",
		})
		service.aiProvider = mockProvider

		// Mapbox matches as Address, not POI — name is just the street address.
		mockMapbox := &mockLocationProvider{
			searchResults: []location.PlaceResult{
				{
					Name:        "3393 Airport Road",
					FullAddress: "3393 Airport Road, Boulder, Colorado 80301, United States",
					Type:        "Address",
					Confidence:  0.95,
				},
			},
		}
		service.locationProvider = mockMapbox

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_WebsiteUrl{
				WebsiteUrl: "https://www.eventbrite.com/e/1940s-ball",
			},
			CurrentTimeUnixSec: 1718000000,
		})

		resp, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		// Address match with JSON-LD: display name should be the JSON-LD place name.
		if resp.Msg.GeocodedLocation.Name != "Boulder Airport" {
			t.Errorf("GeocodedLocation.Name = %q, want 'Boulder Airport' from JSON-LD", resp.Msg.GeocodedLocation.Name)
		}
	})

	t.Run("address match without JSON-LD uses full address", func(t *testing.T) {
		// Text-based experience (no webpage, no JSON-LD).
		mockProvider := newMockAIProvider(
			&ai.ExperienceGeneration{
				Title:         "Dinner Party",
				Description:   "Dinner at a friend's house",
				Confidence:    0.85,
				LocationQuery: "3393 Airport Rd Boulder CO",
			},
			nil,
			nil,
		)
		service.aiProvider = mockProvider

		// Mapbox matches as Address.
		mockMapbox := &mockLocationProvider{
			searchResults: []location.PlaceResult{
				{
					Name:        "3393 Airport Road",
					FullAddress: "3393 Airport Road, Boulder, Colorado 80301, United States",
					Type:        "Address",
					Confidence:  0.90,
				},
			},
		}
		service.locationProvider = mockMapbox

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_Text{Text: "dinner at 3393 Airport Rd Boulder CO"},
		})

		resp, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		// Address match without JSON-LD: display name should be the full address.
		if resp.Msg.GeocodedLocation.Name != "3393 Airport Road, Boulder, Colorado 80301, United States" {
			t.Errorf("GeocodedLocation.Name = %q, want full address", resp.Msg.GeocodedLocation.Name)
		}
	})

	t.Run("structured location without geo uses user proximity", func(t *testing.T) {
		startDate := time.Date(2026, 4, 5, 12, 0, 0, 0, time.FixedZone("MDT", -6*60*60))

		mockFetcher := &webfetch.MockFetcher{
			FetchPageContentFunc: func(ctx context.Context, url string) (*webfetch.PageContent, error) {
				return &webfetch.PageContent{
					URL:      url,
					Title:    "Green Mountain Hike",
					BodyText: "Hiking event in Boulder",
					Event: &webfetch.StructuredEventData{
						Name:      "Green Mountain West Ridge",
						StartDate: &startDate,
						Location:  "Boulder, CO",
						// No geo coordinates
					},
				}, nil
			},
		}
		service.SetWebFetcher(mockFetcher)

		mockProvider := newMockAIProvider(nil, nil, &ai.ExperienceGeneration{
			Title:          "Green Mountain Hike",
			Description:    "A hiking event",
			Confidence:     0.85,
			LocationQuery:  "Green Mountain",
			Date:           "2026-04-05",
			Time:           "12:00",
			TimeConfidence: "EXPLICIT",
		})
		service.aiProvider = mockProvider

		mockMapbox := &mockLocationProvider{
			searchResults: []location.PlaceResult{
				{Name: "Boulder, CO", FullAddress: "Boulder, CO", Type: "City", Confidence: 0.9},
			},
		}
		service.locationProvider = mockMapbox

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_WebsiteUrl{
				WebsiteUrl: "https://www.meetup.com/boulder-hikers/events/123",
			},
			CurrentTimeUnixSec: 1718000000,
			LatitudeDeg:        40.0150,
			LongitudeDeg:       -105.2705,
		})

		_, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		// Verify structured location overrode LLM extraction.
		if mockMapbox.lastQuery != "Boulder, CO" {
			t.Errorf("Expected Mapbox query from JSON-LD, got %q", mockMapbox.lastQuery)
		}

		// Without JSON-LD geo, should fall back to user's proximity.
		if mockMapbox.lastProximity() == nil {
			t.Fatal("Expected user proximity as fallback")
		}
		if mockMapbox.lastProximity().LatitudeDeg != 40.0150 {
			t.Errorf("Proximity latitude = %f, want user's 40.0150", mockMapbox.lastProximity().LatitudeDeg)
		}
	})
}

// mockLocationProvider is a mock implementation of location.Provider for testing.
type mockLocationProvider struct {
	searchResults    []location.PlaceResult
	searchError      error
	lastQuery        string
	lastProximityReq location.ProximityRequest
	lastLimit        int
}

// SearchPlaces records its arguments and returns the configured fixture.
func (m *mockLocationProvider) SearchPlaces(ctx context.Context, query string, proximity location.ProximityRequest, limit int) ([]location.PlaceResult, error) {
	m.lastQuery = query
	m.lastProximityReq = proximity
	m.lastLimit = limit

	if m.searchError != nil {
		return nil, m.searchError
	}
	return m.searchResults, nil
}

// lastProximity returns the coordinate from the most recent SearchPlaces
// call, or nil if no proximity was supplied. Convenience accessor for
// tests that pre-date typed proximity.
func (m *mockLocationProvider) lastProximity() *models.Geolocation {
	return m.lastProximityReq.Coord
}

// ForwardGeocode is not exercised by these tests.
func (m *mockLocationProvider) ForwardGeocode(ctx context.Context, address *models.Address) (*models.Geolocation, error) {
	return nil, fmt.Errorf("not implemented")
}

// ReverseGeocode is not exercised by these tests.
func (m *mockLocationProvider) ReverseGeocode(ctx context.Context, geolocation *models.Geolocation) (*location.PlaceResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// CheckHealth is not exercised by these tests.
func (m *mockLocationProvider) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	return nil, fmt.Errorf("not implemented")
}
