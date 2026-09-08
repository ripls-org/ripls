package location

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// mockRoundTripper is a custom http.RoundTripper for testing.
type mockRoundTripper struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

// TestForwardGeocode tests forward geocoding with a mocked HTTP client.
func TestForwardGeocode(t *testing.T) {
	mockResponse := mapboxResponse{
		Type: "FeatureCollection",
		Features: []mapboxFeature{
			{
				Type: "Feature",
				Geometry: struct {
					Type        string    `json:"type"`
					Coordinates []float64 `json:"coordinates"`
				}{
					Type:        "Point",
					Coordinates: []float64{-122.0856, 37.4220},
				},
			},
		},
	}

	client := &MapboxClient{
		accessToken: "test-access-token",
		httpClient: &http.Client{
			Transport: &mockRoundTripper{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					// Verify the request path
					if !strings.Contains(req.URL.Path, "/search/geocode/v6/forward") {
						t.Errorf("unexpected path: %s", req.URL.Path)
					}

					// Check query parameters
					query := req.URL.Query()
					if query.Get("country") != "US" {
						t.Errorf("expected country=US, got %s", query.Get("country"))
					}
					if query.Get("place") != "Mountain View" {
						t.Errorf("expected place=Mountain View, got %s", query.Get("place"))
					}
					if query.Get("postcode") != "94043" {
						t.Errorf("expected postcode=94043, got %s", query.Get("postcode"))
					}
					if query.Get("address_line1") != "1600 Amphitheatre Parkway" {
						t.Errorf("expected address_line1=1600 Amphitheatre Parkway, got %s", query.Get("address_line1"))
					}

					// Return mock response
					body, _ := json.Marshal(mockResponse)
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(string(body))),
						Header:     make(http.Header),
					}, nil
				},
			},
		},
	}

	address := &models.Address{
		RegionCode:   "US",
		PostalCode:   "94043",
		Locality:     "Mountain View",
		AddressLines: []string{"1600 Amphitheatre Parkway"},
	}

	ctx := context.Background()
	result, err := client.ForwardGeocode(ctx, address)
	if err != nil {
		t.Fatalf("ForwardGeocode failed: %v", err)
	}

	// Verify the result
	if result.LongitudeDeg != -122.0856 {
		t.Errorf("expected longitude -122.0856, got %f", result.LongitudeDeg)
	}
	if result.LatitudeDeg != 37.4220 {
		t.Errorf("expected latitude 37.4220, got %f", result.LatitudeDeg)
	}
}

// TestReverseGeocode tests reverse geocoding with a mocked HTTP client.
// Uses a raw JSON fixture so the test survives additions to the response
// struct fields — the parser is the thing under test, not the struct shape.
func TestReverseGeocode(t *testing.T) {
	const mockJSON = `{
	  "type": "FeatureCollection",
	  "features": [{
	    "type": "Feature",
	    "geometry": {"type": "Point", "coordinates": [-122.0856, 37.4220]},
	    "properties": {
	      "name": "1600 Amphitheatre Parkway",
	      "mapbox_id": "dXJuOm1ieGFkcjphbXBoaXRoZWF0cmU",
	      "feature_type": "address",
	      "full_address": "1600 Amphitheatre Parkway, Mountain View, CA 94043, United States",
	      "context": {
	        "country": {"name": "United States", "country_code": "US"},
	        "region": {"name": "California", "region_code": "CA"},
	        "postcode": {"name": "94043"},
	        "place": {"name": "Mountain View"},
	        "neighborhood": {"name": "Shoreline West"},
	        "address": {"name": "1600 Amphitheatre Parkway"}
	      }
	    }
	  }]
	}`

	client := &MapboxClient{
		accessToken: "test-access-token",
		httpClient: &http.Client{
			Transport: &mockRoundTripper{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					if !strings.Contains(req.URL.Path, "/search/geocode/v6/reverse") {
						t.Errorf("unexpected path: %s", req.URL.Path)
					}
					query := req.URL.Query()
					if query.Get("longitude") == "" || query.Get("latitude") == "" {
						t.Error("missing longitude or latitude parameter")
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(mockJSON)),
						Header:     make(http.Header),
					}, nil
				},
			},
		},
	}

	geolocation := &models.Geolocation{
		LatitudeDeg:  37.4220,
		LongitudeDeg: -122.0856,
	}

	ctx := context.Background()
	result, err := client.ReverseGeocode(ctx, geolocation)
	if err != nil {
		t.Fatalf("ReverseGeocode failed: %v", err)
	}

	if result.ExternalProvider != "mapbox" {
		t.Errorf("expected ExternalProvider=mapbox, got %q", result.ExternalProvider)
	}
	if result.ExternalID != "dXJuOm1ieGFkcjphbXBoaXRoZWF0cmU" {
		t.Errorf("expected ExternalID from mapbox_id, got %q", result.ExternalID)
	}
	if result.Type != "Address" {
		t.Errorf("expected Type=Address, got %q", result.Type)
	}
	if result.RegionCode != "CA" {
		t.Errorf("expected region code CA, got %s", result.RegionCode)
	}
	if result.PostalCode != "94043" {
		t.Errorf("expected postal code 94043, got %s", result.PostalCode)
	}
	if result.Locality != "Mountain View" {
		t.Errorf("expected locality Mountain View, got %s", result.Locality)
	}
	if result.Neighborhood != "Shoreline West" {
		t.Errorf("expected neighborhood Shoreline West, got %s", result.Neighborhood)
	}
	if result.AdministrativeArea != "California" {
		t.Errorf("expected administrative area California, got %s", result.AdministrativeArea)
	}
	if result.FullAddress != "1600 Amphitheatre Parkway" {
		t.Errorf("expected FullAddress to be the address line, got %q", result.FullAddress)
	}
	if result.Coordinates.Latitude != 37.4220 || result.Coordinates.Longitude != -122.0856 {
		t.Errorf("unexpected coordinates: %+v", result.Coordinates)
	}
}

// TestReverseGeocode_NoRegionCode_FallsBackToCountry verifies that when
// Mapbox omits region_code, RegionCode falls back to the country short
// code so the field is never empty when any region info was returned.
func TestReverseGeocode_NoRegionCode_FallsBackToCountry(t *testing.T) {
	const mockJSON = `{
	  "type": "FeatureCollection",
	  "features": [{
	    "type": "Feature",
	    "geometry": {"type": "Point", "coordinates": [2.3522, 48.8566]},
	    "properties": {
	      "name": "Paris",
	      "feature_type": "place",
	      "context": {
	        "country": {"name": "France", "country_code": "fr"}
	      }
	    }
	  }]
	}`

	client := &MapboxClient{
		accessToken: "test-access-token",
		httpClient: &http.Client{
			Transport: &mockRoundTripper{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(mockJSON)),
						Header:     make(http.Header),
					}, nil
				},
			},
		},
	}

	result, err := client.ReverseGeocode(context.Background(), &models.Geolocation{
		LatitudeDeg:  48.8566,
		LongitudeDeg: 2.3522,
	})
	if err != nil {
		t.Fatalf("ReverseGeocode failed: %v", err)
	}

	if result.RegionCode != "FR" {
		t.Errorf("expected RegionCode to fall back to country code FR, got %q", result.RegionCode)
	}
	if result.ExternalID != "" || result.ExternalProvider != "" {
		t.Error("expected empty external ID when response omits mapbox_id")
	}
}

// TestForwardGeocode_ValidationErrors tests input validation.
func TestForwardGeocode_ValidationErrors(t *testing.T) {
	client := NewMapboxClient("test-access-token")
	ctx := context.Background()

	// Test nil address
	_, err := client.ForwardGeocode(ctx, nil)
	if err == nil {
		t.Error("expected error for nil address")
	}
	if !strings.Contains(err.Error(), "cannot be nil") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestReverseGeocode_ValidationErrors tests input validation for reverse geocoding.
func TestReverseGeocode_ValidationErrors(t *testing.T) {
	client := NewMapboxClient("test-access-token")
	ctx := context.Background()

	tests := []struct {
		name        string
		geolocation *models.Geolocation
		wantError   string
	}{
		{
			name:        "nil geolocation",
			geolocation: nil,
			wantError:   "cannot be nil",
		},
		{
			name: "invalid latitude - too low",
			geolocation: &models.Geolocation{
				LatitudeDeg:  -91.0,
				LongitudeDeg: 0.0,
			},
			wantError: "latitude must be between -90 and 90",
		},
		{
			name: "invalid latitude - too high",
			geolocation: &models.Geolocation{
				LatitudeDeg:  91.0,
				LongitudeDeg: 0.0,
			},
			wantError: "latitude must be between -90 and 90",
		},
		{
			name: "invalid longitude - too low",
			geolocation: &models.Geolocation{
				LatitudeDeg:  0.0,
				LongitudeDeg: -181.0,
			},
			wantError: "longitude must be between -180 and 180",
		},
		{
			name: "invalid longitude - too high",
			geolocation: &models.Geolocation{
				LatitudeDeg:  0.0,
				LongitudeDeg: 181.0,
			},
			wantError: "longitude must be between -180 and 180",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.ReverseGeocode(ctx, tt.geolocation)
			if err == nil {
				t.Error("expected error but got none")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("expected error containing %q, got %v", tt.wantError, err)
			}
		})
	}
}

// TestForwardGeocode_NoResults tests the case when Mapbox returns no results.
func TestForwardGeocode_NoResults(t *testing.T) {
	mockResponse := mapboxResponse{
		Type:     "FeatureCollection",
		Features: []mapboxFeature{}, // Empty results
	}

	client := &MapboxClient{
		accessToken: "test-access-token",
		httpClient: &http.Client{
			Transport: &mockRoundTripper{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					body, _ := json.Marshal(mockResponse)
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(string(body))),
						Header:     make(http.Header),
					}, nil
				},
			},
		},
	}

	address := &models.Address{
		RegionCode:   "ZZ",
		Locality:     "Nonexistent",
		AddressLines: []string{"Fake Street"},
	}

	ctx := context.Background()
	_, err := client.ForwardGeocode(ctx, address)
	if err == nil {
		t.Error("expected error for no results")
	}
	if !strings.Contains(err.Error(), "no results found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestReverseGeocode_APIError tests handling of API errors.
func TestReverseGeocode_APIError(t *testing.T) {
	client := &MapboxClient{
		accessToken: "test-access-token",
		httpClient: &http.Client{
			Transport: &mockRoundTripper{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusUnauthorized,
						Body:       io.NopCloser(strings.NewReader(`{"message": "Invalid API key"}`)),
						Header:     make(http.Header),
					}, nil
				},
			},
		},
	}

	geolocation := &models.Geolocation{
		LatitudeDeg:  37.4220,
		LongitudeDeg: -122.0856,
	}

	ctx := context.Background()
	_, err := client.ReverseGeocode(ctx, geolocation)
	if err == nil {
		t.Error("expected error for API error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error containing status 401, got: %v", err)
	}
}

// TestSearchPlaces_POI verifies POI search result parsing including the
// external ID, context hierarchy, and type mapping.
func TestSearchPlaces_POI(t *testing.T) {
	const mockJSON = `{
	  "features": [{
	    "geometry": {"type": "Point", "coordinates": [-105.2705, 40.0150]},
	    "properties": {
	      "name": "Boulder Airport",
	      "mapbox_id": "dXJuOm1ieHBsYzpib3VsZGVyYWlycG9ydA",
	      "feature_type": "poi",
	      "full_address": "3393 Airport Rd, Boulder, CO 80302",
	      "place_formatted": "Boulder, CO 80302",
	      "context": {
	        "neighborhood": {"name": "North Boulder"},
	        "district": {"name": "Boulder County"},
	        "place": {"name": "Boulder"},
	        "region": {"name": "Colorado", "region_code": "CO"},
	        "postcode": {"name": "80302"},
	        "country": {"name": "United States", "country_code": "US"}
	      }
	    }
	  }]
	}`

	client := &MapboxClient{
		accessToken: "test-access-token",
		httpClient: &http.Client{
			Transport: &mockRoundTripper{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					if !strings.Contains(req.URL.Path, "/search/searchbox/v1/forward") {
						t.Errorf("unexpected path: %s", req.URL.Path)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(mockJSON)),
						Header:     make(http.Header),
					}, nil
				},
			},
		},
	}

	places, err := client.SearchPlaces(context.Background(), "Boulder Airport", ProximityRequest{}, 1)
	if err != nil {
		t.Fatalf("SearchPlaces failed: %v", err)
	}
	if len(places) != 1 {
		t.Fatalf("expected 1 result, got %d", len(places))
	}

	p := places[0]
	if p.Name != "Boulder Airport" {
		t.Errorf("Name = %q, want 'Boulder Airport'", p.Name)
	}
	if p.Type != "POI" {
		t.Errorf("Type = %q, want 'POI'", p.Type)
	}
	if p.ExternalID != "dXJuOm1ieHBsYzpib3VsZGVyYWlycG9ydA" {
		t.Errorf("ExternalID = %q, want the mapbox_id value", p.ExternalID)
	}
	if p.ExternalProvider != "mapbox" {
		t.Errorf("ExternalProvider = %q, want 'mapbox'", p.ExternalProvider)
	}
	if p.Locality != "Boulder" {
		t.Errorf("Locality = %q, want 'Boulder'", p.Locality)
	}
	if p.Neighborhood != "North Boulder" {
		t.Errorf("Neighborhood = %q, want 'North Boulder'", p.Neighborhood)
	}
	if p.County != "Boulder County" {
		t.Errorf("County = %q, want 'Boulder County'", p.County)
	}
	if p.RegionCode != "CO" {
		t.Errorf("RegionCode = %q, want 'CO'", p.RegionCode)
	}
	if p.AdministrativeArea != "Colorado" {
		t.Errorf("AdministrativeArea = %q, want 'Colorado'", p.AdministrativeArea)
	}
	if p.PostalCode != "80302" {
		t.Errorf("PostalCode = %q, want '80302'", p.PostalCode)
	}
	if p.Coordinates.Latitude != 40.0150 || p.Coordinates.Longitude != -105.2705 {
		t.Errorf("unexpected coordinates: %+v", p.Coordinates)
	}
}

// TestSearchPlaces_Address verifies address-type results map to Type=Address.
func TestSearchPlaces_Address(t *testing.T) {
	const mockJSON = `{
	  "features": [{
	    "geometry": {"type": "Point", "coordinates": [-105.2705, 40.0150]},
	    "properties": {
	      "name": "1600 Broadway",
	      "mapbox_id": "dXJuOmFkZHI6MTYwMA",
	      "feature_type": "address",
	      "full_address": "1600 Broadway, Boulder, CO 80302",
	      "context": {
	        "place": {"name": "Boulder"},
	        "region": {"name": "Colorado", "region_code": "CO"}
	      }
	    }
	  }]
	}`

	client := &MapboxClient{
		accessToken: "test-access-token",
		httpClient: &http.Client{
			Transport: &mockRoundTripper{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(mockJSON)),
						Header:     make(http.Header),
					}, nil
				},
			},
		},
	}

	places, err := client.SearchPlaces(context.Background(), "1600 Broadway", ProximityRequest{}, 1)
	if err != nil {
		t.Fatalf("SearchPlaces failed: %v", err)
	}
	if len(places) != 1 {
		t.Fatalf("expected 1 result, got %d", len(places))
	}
	if places[0].Type != "Address" {
		t.Errorf("Type = %q, want 'Address'", places[0].Type)
	}
	if places[0].ExternalID == "" || places[0].ExternalProvider != "mapbox" {
		t.Errorf("expected external ID+provider, got id=%q provider=%q",
			places[0].ExternalID, places[0].ExternalProvider)
	}
}

// TestSearchPlaces_Locality verifies locality/place mapping and the case
// where mapbox_id is missing (older responses).
func TestSearchPlaces_Locality_NoMapboxID(t *testing.T) {
	const mockJSON = `{
	  "features": [{
	    "geometry": {"type": "Point", "coordinates": [-105.2705, 40.0150]},
	    "properties": {
	      "name": "Boulder",
	      "feature_type": "place",
	      "full_address": "Boulder, Colorado, United States",
	      "context": {
	        "region": {"name": "Colorado", "region_code": "CO"},
	        "country": {"name": "United States", "country_code": "US"}
	      }
	    }
	  }]
	}`

	client := &MapboxClient{
		accessToken: "test-access-token",
		httpClient: &http.Client{
			Transport: &mockRoundTripper{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(mockJSON)),
						Header:     make(http.Header),
					}, nil
				},
			},
		},
	}

	places, err := client.SearchPlaces(context.Background(), "Boulder", ProximityRequest{}, 1)
	if err != nil {
		t.Fatalf("SearchPlaces failed: %v", err)
	}
	if places[0].Type != "City" {
		t.Errorf("Type = %q, want 'City' for feature_type=place", places[0].Type)
	}
	if places[0].ExternalID != "" || places[0].ExternalProvider != "" {
		t.Error("expected empty external ID when mapbox_id is absent")
	}
}

// proximityFixture returns a SearchBox response with three POI features at
// known distances from Boulder (40.0150, -105.2705): one local (<1 km),
// one regional (~50 km), and one cross-country (~2400 km).
func proximityFixture() string {
	return `{
	  "features": [
	    {
	      "geometry": {"type": "Point", "coordinates": [-105.2700, 40.0155]},
	      "properties": {"name": "Local Cafe", "feature_type": "poi"}
	    },
	    {
	      "geometry": {"type": "Point", "coordinates": [-104.9903, 39.7392]},
	      "properties": {"name": "Denver Cafe", "feature_type": "poi"}
	    },
	    {
	      "geometry": {"type": "Point", "coordinates": [-73.9857, 40.7484]},
	      "properties": {"name": "Manhattan Cafe", "feature_type": "poi"}
	    }
	  ]
	}`
}

func newProximityTestClient(t *testing.T, capturedQuery *url.Values) *MapboxClient {
	t.Helper()
	return &MapboxClient{
		accessToken: "test-access-token",
		httpClient: &http.Client{
			Transport: &mockRoundTripper{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					if capturedQuery != nil {
						q := req.URL.Query()
						*capturedQuery = q
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(proximityFixture())),
						Header:     make(http.Header),
					}, nil
				},
			},
		},
	}
}

// TestSearchPlaces_ProximityNone_OmitsParam verifies that ProximityNone
// (the zero value) does not add a proximity query parameter to the Mapbox
// request and does not filter results.
func TestSearchPlaces_ProximityNone_OmitsParam(t *testing.T) {
	var got url.Values
	client := newProximityTestClient(t, &got)

	places, err := client.SearchPlaces(context.Background(), "cafe", ProximityRequest{}, 5)
	if err != nil {
		t.Fatalf("SearchPlaces failed: %v", err)
	}
	if got.Has("proximity") {
		t.Errorf("expected no proximity query param, got %q", got.Get("proximity"))
	}
	if len(places) != 3 {
		t.Errorf("expected 3 unfiltered results, got %d", len(places))
	}
}

// TestSearchPlaces_ProximityHint_NoFilter verifies that ProximityHint adds
// the proximity bias parameter but does not post-filter results: even the
// cross-country result survives.
func TestSearchPlaces_ProximityHint_NoFilter(t *testing.T) {
	var got url.Values
	client := newProximityTestClient(t, &got)

	boulder := &models.Geolocation{LatitudeDeg: 40.0150, LongitudeDeg: -105.2705}
	places, err := client.SearchPlaces(context.Background(), "cafe", ProximityRequest{
		Coord:     boulder,
		Semantics: ProximityHint,
	}, 5)
	if err != nil {
		t.Fatalf("SearchPlaces failed: %v", err)
	}
	if !got.Has("proximity") {
		t.Error("expected proximity query param for ProximityHint")
	}
	if len(places) != 3 {
		t.Errorf("expected all 3 results to survive ProximityHint, got %d", len(places))
	}
}

// TestSearchPlaces_ProximityBound_FiltersOutOfRadius verifies that
// ProximityBound drops results outside RadiusM × BoundRadiusExpansionFactor
// from Coord. With a 10 km radius and 1.5× expansion, only the local cafe
// (<1 km away) survives; Denver (~50 km) and Manhattan (~2400 km) drop.
func TestSearchPlaces_ProximityBound_FiltersOutOfRadius(t *testing.T) {
	client := newProximityTestClient(t, nil)

	boulder := &models.Geolocation{LatitudeDeg: 40.0150, LongitudeDeg: -105.2705}
	places, err := client.SearchPlaces(context.Background(), "cafe", ProximityRequest{
		Coord:     boulder,
		RadiusM:   10_000, // 10 km × 1.5 = 15 km cutoff
		Semantics: ProximityBound,
	}, 5)
	if err != nil {
		t.Fatalf("SearchPlaces failed: %v", err)
	}
	if len(places) != 1 {
		t.Fatalf("expected 1 in-radius result, got %d: %+v", len(places), places)
	}
	if places[0].Name != "Local Cafe" {
		t.Errorf("expected Local Cafe to survive filter, got %q", places[0].Name)
	}
}

// TestSearchPlaces_ProximityBound_DefaultRadius verifies that RadiusM=0
// falls back to DefaultBoundRadiusM (50 km). At 50 km × 1.5 = 75 km, the
// local and Denver cafes survive; Manhattan drops.
func TestSearchPlaces_ProximityBound_DefaultRadius(t *testing.T) {
	client := newProximityTestClient(t, nil)

	boulder := &models.Geolocation{LatitudeDeg: 40.0150, LongitudeDeg: -105.2705}
	places, err := client.SearchPlaces(context.Background(), "cafe", ProximityRequest{
		Coord:     boulder,
		Semantics: ProximityBound,
	}, 5)
	if err != nil {
		t.Fatalf("SearchPlaces failed: %v", err)
	}
	if len(places) != 2 {
		t.Fatalf("expected 2 in-radius results at default radius, got %d: %+v", len(places), places)
	}
	if places[0].Name != "Local Cafe" || places[1].Name != "Denver Cafe" {
		t.Errorf("unexpected surviving results: %q, %q", places[0].Name, places[1].Name)
	}
}
