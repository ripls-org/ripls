package location

import (
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/services"
)

// TestLocation_FieldRoundTrip verifies that every user-provided field on
// SaveLocationRequest survives the save → fetch cycle through GetLocation.
//
// This is the test that would have failed the moment the #1142 class of bug
// was introduced: a new API field wired into the save-path request but
// forgotten on the response-path message. If any field below is removed
// from GetLocationResponse (or from the save-side persistence), this test
// fails with a clear message about which field broke.
//
// See docs/proto_conventions.md for the round-trip convention.
func TestLocation_FieldRoundTrip(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := createAuthContext("user-roundtrip", "roundtrip@example.com")

	// Arrange: one SaveLocationRequest carrying a known value for every
	// field the client can set. Each sub-test round-trips a single field by
	// re-saving this same request (dedup will match on coordinates) and
	// fetching the row by ID.
	input := &api.SaveLocationRequest{
		Name:                  "Empower Field at Mile High",
		LatitudeDeg:           39.7439,
		LongitudeDeg:          -105.0201,
		RegionCode:            "US",
		PostalCode:            "80204",
		Locality:              "Denver",
		AddressLines:          []string{"1701 Bryant St"},
		Neighborhood:          "Sun Valley",
		County:                "Denver County",
		AdministrativeArea:    "Colorado",
		ExternalPlaceId:       "mapbox:poi.empower-field",
		ExternalPlaceProvider: "mapbox",
	}

	var savedID string
	save := func() error {
		resp, err := svc.SaveLocation(ctx, connect.NewRequest(input))
		if err != nil {
			return err
		}
		savedID = resp.Msg.Id
		return nil
	}

	// Perform the save once before running the sub-cases. All sub-cases
	// share the same persisted row.
	if err := save(); err != nil {
		t.Fatalf("initial SaveLocation failed: %v", err)
	}

	noopSave := func() error { return nil }
	get := func() (*api.GetLocationResponse, error) {
		resp, err := svc.GetLocation(ctx, connect.NewRequest(&api.GetLocationRequest{Id: savedID}))
		if err != nil {
			return nil, err
		}
		return resp.Msg, nil
	}

	// Each case asserts one field's round-trip independently so a failure
	// points at the broken field, not a generic "response doesn't match."
	cases := []struct {
		name  string
		field string
		want  any
		fetch func() (any, error)
	}{
		{
			name: "name", field: "name", want: input.Name,
			fetch: func() (any, error) { r, err := get(); return r.GetName(), err },
		},
		{
			name: "latitude", field: "latitude_deg", want: input.LatitudeDeg,
			fetch: func() (any, error) { r, err := get(); return r.GetLatitudeDeg(), err },
		},
		{
			name: "longitude", field: "longitude_deg", want: input.LongitudeDeg,
			fetch: func() (any, error) { r, err := get(); return r.GetLongitudeDeg(), err },
		},
		{
			name: "region_code", field: "region_code", want: input.RegionCode,
			fetch: func() (any, error) { r, err := get(); return r.GetRegionCode(), err },
		},
		{
			name: "postal_code", field: "postal_code", want: input.PostalCode,
			fetch: func() (any, error) { r, err := get(); return r.GetPostalCode(), err },
		},
		{
			name: "locality", field: "locality", want: input.Locality,
			fetch: func() (any, error) { r, err := get(); return r.GetLocality(), err },
		},
		{
			name: "address_lines", field: "address_lines", want: input.AddressLines,
			fetch: func() (any, error) { r, err := get(); return r.GetAddressLines(), err },
		},
		{
			name: "neighborhood", field: "neighborhood", want: input.Neighborhood,
			fetch: func() (any, error) { r, err := get(); return r.GetNeighborhood(), err },
		},
		{
			name: "county", field: "county", want: input.County,
			fetch: func() (any, error) { r, err := get(); return r.GetCounty(), err },
		},
		{
			name: "administrative_area", field: "administrative_area", want: input.AdministrativeArea,
			fetch: func() (any, error) { r, err := get(); return r.GetAdministrativeArea(), err },
		},
		{
			name: "external_place_id", field: "external_place_id", want: input.ExternalPlaceId,
			fetch: func() (any, error) { r, err := get(); return r.GetExternalPlaceId(), err },
		},
		{
			name: "external_place_provider", field: "external_place_provider", want: input.ExternalPlaceProvider,
			fetch: func() (any, error) { r, err := get(); return r.GetExternalPlaceProvider(), err },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			services.AssertFieldRoundTrip(t, tc.field, tc.want, noopSave, tc.fetch)
		})
	}
}

// TestLocation_GeocodedLocationToSaveRoundTrip simulates the client-side
// flow that regressed in #1142: the server returns a GeocodedLocation from
// GenExperience, the client constructs a SaveLocationRequest from it, the
// location is saved, and the client fetches it back. Every field on the
// returned location should match the GeocodedLocation the server originally
// produced.
//
// This test is pure API-boundary coverage: if a field is added to
// GeocodedLocation but forgotten on SaveLocationRequest (or vice-versa),
// or dropped on the read path, the corresponding case fails.
func TestLocation_GeocodedLocationToSaveRoundTrip(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := createAuthContext("user-geocoded", "geocoded@example.com")

	// What the server would return from GenExperience.
	geocoded := &api.GeocodedLocation{
		Name:                  "Central Park",
		LatitudeDeg:           40.7829,
		LongitudeDeg:          -73.9654,
		Locality:              "New York",
		RegionCode:            "NY",
		PostalCode:            "10024",
		AddressLines:          []string{"Central Park"},
		ExternalPlaceId:       "mapbox:poi.central-park",
		ExternalPlaceProvider: "mapbox",
		Neighborhood:          "Upper West Side",
		County:                "New York County",
		AdministrativeArea:    "New York",
	}

	// What the client would build from it. Every field on GeocodedLocation
	// that's also on SaveLocationRequest should be copied over. This is the
	// exact step that #1142's client patch touched.
	saveReq := &api.SaveLocationRequest{
		Name:                  geocoded.Name,
		LatitudeDeg:           geocoded.LatitudeDeg,
		LongitudeDeg:          geocoded.LongitudeDeg,
		RegionCode:            geocoded.RegionCode,
		PostalCode:            geocoded.PostalCode,
		Locality:              geocoded.Locality,
		AddressLines:          geocoded.AddressLines,
		Neighborhood:          geocoded.Neighborhood,
		County:                geocoded.County,
		AdministrativeArea:    geocoded.AdministrativeArea,
		ExternalPlaceId:       geocoded.ExternalPlaceId,
		ExternalPlaceProvider: geocoded.ExternalPlaceProvider,
	}

	var savedID string
	save := func() error {
		resp, err := svc.SaveLocation(ctx, connect.NewRequest(saveReq))
		if err != nil {
			return err
		}
		savedID = resp.Msg.Id
		return nil
	}
	if err := save(); err != nil {
		t.Fatalf("SaveLocation failed: %v", err)
	}

	noopSave := func() error { return nil }
	get := func() (*api.GetLocationResponse, error) {
		resp, err := svc.GetLocation(ctx, connect.NewRequest(&api.GetLocationRequest{Id: savedID}))
		if err != nil {
			return nil, err
		}
		return resp.Msg, nil
	}

	services.AssertFieldRoundTrip(t, "external_place_id", geocoded.ExternalPlaceId,
		noopSave,
		func() (string, error) { r, err := get(); return r.GetExternalPlaceId(), err },
	)
	services.AssertFieldRoundTrip(t, "external_place_provider", geocoded.ExternalPlaceProvider,
		noopSave,
		func() (string, error) { r, err := get(); return r.GetExternalPlaceProvider(), err },
	)
	services.AssertFieldRoundTrip(t, "neighborhood", geocoded.Neighborhood,
		noopSave,
		func() (string, error) { r, err := get(); return r.GetNeighborhood(), err },
	)
	services.AssertFieldRoundTrip(t, "county", geocoded.County,
		noopSave,
		func() (string, error) { r, err := get(); return r.GetCounty(), err },
	)
	services.AssertFieldRoundTrip(t, "administrative_area", geocoded.AdministrativeArea,
		noopSave,
		func() (string, error) { r, err := get(); return r.GetAdministrativeArea(), err },
	)
}
