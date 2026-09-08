package location

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// newGoogleTestClient returns a *GoogleMapsClient whose HTTP transport
// is the supplied roundTripFunc. Tests can inspect the outgoing requests
// and craft per-endpoint responses.
func newGoogleTestClient(rt func(req *http.Request) (*http.Response, error)) *GoogleMapsClient {
	return &GoogleMapsClient{
		apiKey: "test-google-key",
		httpClient: &http.Client{
			Transport: &mockRoundTripper{roundTripFunc: rt},
		},
	}
}

// respondJSON returns an *http.Response with status 200 and the supplied
// JSON body. Convenience for test fixtures.
func respondJSON(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

// === ForwardGeocode ===.

func TestGoogleForwardGeocode_Success(t *testing.T) {
	const body = `{
	  "status": "OK",
	  "results": [{
	    "formatted_address": "1600 Amphitheatre Pkwy, Mountain View, CA 94043, USA",
	    "geometry": {"location": {"lat": 37.4220, "lng": -122.0841}},
	    "place_id": "ChIJ_test",
	    "types": ["street_address"]
	  }]
	}`

	var gotURL string
	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		gotURL = req.URL.String()
		return respondJSON(body), nil
	})

	geo, err := client.ForwardGeocode(context.Background(), &models.Address{
		AddressLines: []string{"1600 Amphitheatre Parkway"},
		Locality:     "Mountain View",
		RegionCode:   "US",
		PostalCode:   "94043",
	})
	if err != nil {
		t.Fatalf("ForwardGeocode failed: %v", err)
	}
	if geo.LatitudeDeg != 37.4220 {
		t.Errorf("LatitudeDeg = %v, want 37.4220", geo.LatitudeDeg)
	}
	if geo.LongitudeDeg != -122.0841 {
		t.Errorf("LongitudeDeg = %v, want -122.0841", geo.LongitudeDeg)
	}
	if !strings.Contains(gotURL, "components=country%3AUS%7Cpostal_code%3A94043%7Clocality%3AMountain+View") {
		t.Errorf("expected structured components in URL, got %s", gotURL)
	}
}

func TestGoogleForwardGeocode_ZeroResults(t *testing.T) {
	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		return respondJSON(`{"status": "ZERO_RESULTS", "results": []}`), nil
	})

	_, err := client.ForwardGeocode(context.Background(), &models.Address{
		AddressLines: []string{"nowhere"},
	})
	if err == nil {
		t.Fatal("expected error on zero results")
	}
}

func TestGoogleForwardGeocode_NilAddress(t *testing.T) {
	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		t.Fatal("HTTP should not be called for nil address")
		return nil, nil
	})

	_, err := client.ForwardGeocode(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil address")
	}
}

// === SearchPlaces ===.

// searchFixtureBodies returns canned (autocomplete, details) responses
// for a single POI suggestion. The details payload is parameterizable so
// distance-filter tests can shift the centroid.
func searchFixtureBodies(lat, lng float64, name, placeID string) (string, string) {
	autocomplete := `{
	  "suggestions": [{
	    "placePrediction": {
	      "placeId": "` + placeID + `",
	      "text": {"text": "` + name + `"},
	      "types": ["establishment"]
	    }
	  }]
	}`
	details := `{
	  "id": "` + placeID + `",
	  "displayName": {"text": "` + name + `"},
	  "formattedAddress": "Test St, ` + name + `",
	  "location": {"latitude": ` + floatJSON(lat) + `, "longitude": ` + floatJSON(lng) + `},
	  "types": ["establishment", "point_of_interest"],
	  "addressComponents": [
	    {"longText": "Boulder", "shortText": "Boulder", "types": ["locality", "political"]},
	    {"longText": "Colorado", "shortText": "CO", "types": ["administrative_area_level_1", "political"]}
	  ]
	}`
	return autocomplete, details
}

func floatJSON(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}

func TestGoogleSearchPlaces_ProximityNone_OmitsLocation(t *testing.T) {
	auto, details := searchFixtureBodies(40.0150, -105.2705, "Local Cafe", "ChIJ_local")

	var gotAutocompleteBody string
	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, ":autocomplete"):
			body, _ := io.ReadAll(req.Body)
			gotAutocompleteBody = string(body)
			return respondJSON(auto), nil
		case strings.Contains(req.URL.Path, "/v1/places/"):
			return respondJSON(details), nil
		}
		t.Fatalf("unexpected path: %s", req.URL.Path)
		return nil, nil
	})

	_, err := client.SearchPlaces(context.Background(), "cafe", ProximityRequest{}, 1)
	if err != nil {
		t.Fatalf("SearchPlaces failed: %v", err)
	}
	if strings.Contains(gotAutocompleteBody, "locationBias") {
		t.Errorf("ProximityNone should not send locationBias; body: %s", gotAutocompleteBody)
	}
	if strings.Contains(gotAutocompleteBody, "locationRestriction") {
		t.Errorf("ProximityNone should not send locationRestriction; body: %s", gotAutocompleteBody)
	}
}

func TestGoogleSearchPlaces_ProximityHint_UsesBias(t *testing.T) {
	auto, details := searchFixtureBodies(40.0155, -105.2700, "Local Cafe", "ChIJ_local")

	var gotBody string
	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, ":autocomplete"):
			body, _ := io.ReadAll(req.Body)
			gotBody = string(body)
			return respondJSON(auto), nil
		case strings.Contains(req.URL.Path, "/v1/places/"):
			return respondJSON(details), nil
		}
		return nil, nil
	})

	_, err := client.SearchPlaces(context.Background(), "cafe", ProximityRequest{
		Coord:     &models.Geolocation{LatitudeDeg: 40.0150, LongitudeDeg: -105.2705},
		Semantics: ProximityHint,
	}, 1)
	if err != nil {
		t.Fatalf("SearchPlaces failed: %v", err)
	}
	if !strings.Contains(gotBody, "locationBias") {
		t.Errorf("ProximityHint should send locationBias; body: %s", gotBody)
	}
	if strings.Contains(gotBody, "locationRestriction") {
		t.Errorf("ProximityHint should not send locationRestriction; body: %s", gotBody)
	}
}

func TestGoogleSearchPlaces_ProximityBound_UsesRestriction(t *testing.T) {
	auto, details := searchFixtureBodies(40.0155, -105.2700, "Local Cafe", "ChIJ_local")

	var gotBody string
	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, ":autocomplete"):
			body, _ := io.ReadAll(req.Body)
			gotBody = string(body)
			return respondJSON(auto), nil
		case strings.Contains(req.URL.Path, "/v1/places/"):
			return respondJSON(details), nil
		}
		return nil, nil
	})

	places, err := client.SearchPlaces(context.Background(), "cafe", ProximityRequest{
		Coord:     &models.Geolocation{LatitudeDeg: 40.0150, LongitudeDeg: -105.2705},
		RadiusM:   10_000,
		Semantics: ProximityBound,
	}, 1)
	if err != nil {
		t.Fatalf("SearchPlaces failed: %v", err)
	}
	if !strings.Contains(gotBody, "locationRestriction") {
		t.Errorf("ProximityBound should send locationRestriction; body: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"radius":10000`) {
		t.Errorf("ProximityBound should send the requested radius; body: %s", gotBody)
	}
	if len(places) != 1 {
		t.Fatalf("expected 1 result (in radius), got %d", len(places))
	}
	if places[0].Name != "Local Cafe" {
		t.Errorf("Name = %q, want 'Local Cafe'", places[0].Name)
	}
	if places[0].ExternalProvider != providerGoogleMaps {
		t.Errorf("ExternalProvider = %q, want %q", places[0].ExternalProvider, providerGoogleMaps)
	}
}

func TestGoogleSearchPlaces_ProximityBound_PostFiltersFarResults(t *testing.T) {
	// Autocomplete returns one suggestion. Place Details places its
	// centroid in Manhattan (~2400 km from Boulder), so the post-filter
	// should drop it even though Google "restricted" the search.
	auto, details := searchFixtureBodies(40.7484, -73.9857, "Far Cafe", "ChIJ_far")

	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, ":autocomplete"):
			return respondJSON(auto), nil
		case strings.Contains(req.URL.Path, "/v1/places/"):
			return respondJSON(details), nil
		}
		return nil, nil
	})

	places, err := client.SearchPlaces(context.Background(), "cafe", ProximityRequest{
		Coord:     &models.Geolocation{LatitudeDeg: 40.0150, LongitudeDeg: -105.2705},
		RadiusM:   10_000,
		Semantics: ProximityBound,
	}, 1)
	if err != nil {
		t.Fatalf("SearchPlaces failed: %v", err)
	}
	if len(places) != 0 {
		t.Errorf("expected 0 results after post-filter, got %d: %+v", len(places), places)
	}
}

func TestGoogleSearchPlaces_NoSuggestions(t *testing.T) {
	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, ":autocomplete") {
			return respondJSON(`{}`), nil
		}
		t.Fatal("Place Details should not be called when there are no suggestions")
		return nil, nil
	})

	places, err := client.SearchPlaces(context.Background(), "cafe", ProximityRequest{}, 5)
	if err != nil {
		t.Fatalf("SearchPlaces failed: %v", err)
	}
	if len(places) != 0 {
		t.Errorf("expected 0 results, got %d", len(places))
	}
}

func TestGoogleSearchPlaces_EmptyQuery(t *testing.T) {
	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		t.Fatal("HTTP should not be called for empty query")
		return nil, nil
	})

	_, err := client.SearchPlaces(context.Background(), "", ProximityRequest{}, 1)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestGoogleSearchPlaces_FieldMaskHeader(t *testing.T) {
	auto, details := searchFixtureBodies(40.0155, -105.2700, "Local", "ChIJ_x")

	var detailsFieldMask string
	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, ":autocomplete"):
			return respondJSON(auto), nil
		case strings.Contains(req.URL.Path, "/v1/places/"):
			detailsFieldMask = req.Header.Get("X-Goog-Fieldmask")
			return respondJSON(details), nil
		}
		return nil, nil
	})

	_, err := client.SearchPlaces(context.Background(), "local", ProximityRequest{}, 1)
	if err != nil {
		t.Fatalf("SearchPlaces failed: %v", err)
	}
	// Cheap Place Details Basic tier requires exactly this field mask.
	if detailsFieldMask != placeDetailsFieldMask {
		t.Errorf("Place Details X-Goog-FieldMask = %q, want %q", detailsFieldMask, placeDetailsFieldMask)
	}
}

// === ReverseGeocode ===.

func TestGoogleReverseGeocode_Success(t *testing.T) {
	const body = `{
	  "status": "OK",
	  "results": [{
	    "formatted_address": "3393 Airport Rd, Boulder, CO 80302, USA",
	    "geometry": {"location": {"lat": 40.0167, "lng": -105.2817}},
	    "place_id": "ChIJ_test",
	    "types": ["street_address"],
	    "address_components": [
	      {"long_name": "3393", "short_name": "3393", "types": ["street_number"]},
	      {"long_name": "Airport Road", "short_name": "Airport Rd", "types": ["route"]},
	      {"long_name": "Boulder", "short_name": "Boulder", "types": ["locality", "political"]},
	      {"long_name": "Colorado", "short_name": "CO", "types": ["administrative_area_level_1", "political"]},
	      {"long_name": "80302", "short_name": "80302", "types": ["postal_code"]}
	    ]
	  }]
	}`

	var gotURL string
	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		gotURL = req.URL.String()
		return respondJSON(body), nil
	})

	place, err := client.ReverseGeocode(context.Background(), &models.Geolocation{
		LatitudeDeg:  40.0167,
		LongitudeDeg: -105.2817,
	})
	if err != nil {
		t.Fatalf("ReverseGeocode failed: %v", err)
	}
	if !strings.Contains(gotURL, "result_type=street_address") {
		t.Errorf("expected result_type filter in URL, got %s", gotURL)
	}
	if place.Name != "3393 Airport Road" {
		t.Errorf("Name = %q, want '3393 Airport Road'", place.Name)
	}
	if place.Locality != "Boulder" {
		t.Errorf("Locality = %q, want 'Boulder'", place.Locality)
	}
	if place.RegionCode != "CO" {
		t.Errorf("RegionCode = %q, want 'CO'", place.RegionCode)
	}
	if place.PostalCode != "80302" {
		t.Errorf("PostalCode = %q, want '80302'", place.PostalCode)
	}
	if place.Type != "Address" {
		t.Errorf("Type = %q, want 'Address'", place.Type)
	}
	if place.ExternalProvider != providerGoogleMaps {
		t.Errorf("ExternalProvider = %q, want %q", place.ExternalProvider, providerGoogleMaps)
	}
}

func TestGoogleReverseGeocode_PlusCodeOnly_Skipped(t *testing.T) {
	// Google sometimes returns Plus Codes for rural reverse-geocodes.
	// Defensive: even if result_type filtering somehow returns a
	// plus_code-only result, drop it.
	const body = `{
	  "status": "OK",
	  "results": [{
	    "formatted_address": "H629+25 Keystone, CO, USA",
	    "geometry": {"location": {"lat": 39.6, "lng": -105.9}},
	    "place_id": "GhIJ_pluscode",
	    "types": ["plus_code"],
	    "address_components": []
	  }]
	}`

	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		return respondJSON(body), nil
	})

	_, err := client.ReverseGeocode(context.Background(), &models.Geolocation{
		LatitudeDeg:  39.6,
		LongitudeDeg: -105.9,
	})
	if err == nil {
		t.Fatal("expected error when only result is a Plus Code")
	}
}

func TestGoogleReverseGeocode_SkipsPlusCodeThenReturnsAddress(t *testing.T) {
	// First result is a Plus Code, second is a real street address.
	// The Plus Code should be skipped; the address should win.
	const body = `{
	  "status": "OK",
	  "results": [
	    {
	      "formatted_address": "H629+25 Keystone, CO, USA",
	      "geometry": {"location": {"lat": 39.6, "lng": -105.9}},
	      "place_id": "GhIJ_pluscode",
	      "types": ["plus_code"],
	      "address_components": []
	    },
	    {
	      "formatted_address": "124 E Colorado Ave, Grant, CO, USA",
	      "geometry": {"location": {"lat": 39.6, "lng": -105.9}},
	      "place_id": "ChIJ_real",
	      "types": ["street_address"],
	      "address_components": [
	        {"long_name": "Grant", "short_name": "Grant", "types": ["locality", "political"]}
	      ]
	    }
	  ]
	}`

	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		return respondJSON(body), nil
	})

	place, err := client.ReverseGeocode(context.Background(), &models.Geolocation{
		LatitudeDeg:  39.6,
		LongitudeDeg: -105.9,
	})
	if err != nil {
		t.Fatalf("ReverseGeocode failed: %v", err)
	}
	if place.ExternalID != "ChIJ_real" {
		t.Errorf("ExternalID = %q, expected to skip Plus Code and pick the real address", place.ExternalID)
	}
}

func TestGoogleReverseGeocode_BadCoordinates(t *testing.T) {
	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		t.Fatal("HTTP should not be called for invalid coordinates")
		return nil, nil
	})

	_, err := client.ReverseGeocode(context.Background(), &models.Geolocation{
		LatitudeDeg:  100, // out of range
		LongitudeDeg: 0,
	})
	if err == nil {
		t.Fatal("expected error for out-of-range latitude")
	}
}

// === CheckHealth ===.

func TestGoogleCheckHealth_OK(t *testing.T) {
	const body = `{"status": "OK", "results": [{"formatted_address": "x", "geometry": {"location": {"lat": 0, "lng": 0}}, "place_id": "p", "types": ["x"]}]}`

	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		return respondJSON(body), nil
	})

	statuses, err := client.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth returned go error: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Error != "" {
		t.Errorf("Error = %q, expected empty for OK", statuses[0].Error)
	}
	if statuses[0].Backend != providerGoogleMaps {
		t.Errorf("Backend = %q, want %q", statuses[0].Backend, providerGoogleMaps)
	}
}

func TestGoogleCheckHealth_REQUEST_DENIED(t *testing.T) {
	const body = `{"status": "REQUEST_DENIED", "error_message": "The provided API key is invalid."}`

	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		return respondJSON(body), nil
	})

	statuses, err := client.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth returned go error: %v", err)
	}
	if statuses[0].Error != "REQUEST_DENIED" {
		t.Errorf("Error = %q, want 'REQUEST_DENIED'", statuses[0].Error)
	}
	// The detailed error_message must not leak — it can include the key
	// fingerprint and the health payload is served publicly.
	if strings.Contains(statuses[0].Error, "API key is invalid") {
		t.Errorf("Error should not include error_message text from Google: %q", statuses[0].Error)
	}
}

func TestGoogleCheckHealth_HTTP500(t *testing.T) {
	client := newGoogleTestClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("upstream error")),
			Header:     make(http.Header),
		}, nil
	})

	statuses, err := client.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth returned go error: %v", err)
	}
	if !strings.Contains(statuses[0].Error, "500") {
		t.Errorf("Error = %q, want to include HTTP 500", statuses[0].Error)
	}
}

// === Helpers ===.

func TestIsPlusCodeOnly(t *testing.T) {
	cases := []struct {
		name  string
		types []string
		want  bool
	}{
		{"empty", nil, false},
		{"plus_code only", []string{"plus_code"}, true},
		{"compound plus", []string{"compound_plus_code"}, true},
		{"plus with address", []string{"plus_code", "street_address"}, false},
		{"address only", []string{"street_address"}, false},
		{"poi only", []string{"establishment", "point_of_interest"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isPlusCodeOnly(c.types); got != c.want {
				t.Errorf("isPlusCodeOnly(%v) = %v, want %v", c.types, got, c.want)
			}
		})
	}
}

func TestBuildGoogleComponents(t *testing.T) {
	cases := []struct {
		name string
		in   *models.Address
		want string
	}{
		{"empty address", &models.Address{}, ""},
		{"country only", &models.Address{RegionCode: "US"}, "country:US"},
		{
			"full structure",
			&models.Address{RegionCode: "US", PostalCode: "94043", Locality: "Mountain View"},
			"country:US|postal_code:94043|locality:Mountain View",
		},
		{"nil safe", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := buildGoogleComponents(c.in); got != c.want {
				t.Errorf("buildGoogleComponents = %q, want %q", got, c.want)
			}
		})
	}
}
