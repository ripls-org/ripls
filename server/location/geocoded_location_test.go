package location

import (
	"testing"
)

func TestPlaceResultToGeocodedLocation(t *testing.T) {
	place := PlaceResult{
		Name:               "North Boulder Park",
		FullAddress:        "3456 Broadway, Boulder, CO 80304",
		Type:               "POI",
		Locality:           "Boulder",
		RegionCode:         "CO",
		PostalCode:         "80304",
		Neighborhood:       "North Boulder",
		County:             "Boulder County",
		AdministrativeArea: "Colorado",
		ExternalID:         "poi.12345",
		ExternalProvider:   "mapbox",
	}
	place.Coordinates.Latitude = 40.05
	place.Coordinates.Longitude = -105.27

	got := place.ToGeocodedLocation("North Boulder Park")

	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Name != "North Boulder Park" {
		t.Errorf("Name = %q, want %q", got.Name, "North Boulder Park")
	}
	if got.LatitudeDeg != 40.05 {
		t.Errorf("LatitudeDeg = %v, want 40.05", got.LatitudeDeg)
	}
	if got.LongitudeDeg != -105.27 {
		t.Errorf("LongitudeDeg = %v, want -105.27", got.LongitudeDeg)
	}
	if got.Locality != "Boulder" {
		t.Errorf("Locality = %q, want Boulder", got.Locality)
	}
	if got.RegionCode != "CO" {
		t.Errorf("RegionCode = %q, want CO", got.RegionCode)
	}
	if got.PostalCode != "80304" {
		t.Errorf("PostalCode = %q, want 80304", got.PostalCode)
	}
	if len(got.AddressLines) != 1 || got.AddressLines[0] != "3456 Broadway, Boulder, CO 80304" {
		t.Errorf("AddressLines = %v, want [\"3456 Broadway, Boulder, CO 80304\"]", got.AddressLines)
	}
	if got.ExternalPlaceId != "poi.12345" {
		t.Errorf("ExternalPlaceId = %q, want poi.12345", got.ExternalPlaceId)
	}
	if got.ExternalPlaceProvider != "mapbox" {
		t.Errorf("ExternalPlaceProvider = %q, want mapbox", got.ExternalPlaceProvider)
	}
	if got.Neighborhood != "North Boulder" {
		t.Errorf("Neighborhood = %q, want North Boulder", got.Neighborhood)
	}
	if got.County != "Boulder County" {
		t.Errorf("County = %q, want Boulder County", got.County)
	}
	if got.AdministrativeArea != "Colorado" {
		t.Errorf("AdministrativeArea = %q, want Colorado", got.AdministrativeArea)
	}
}

func TestPlaceResultToGeocodedLocation_DisplayNameOverride(t *testing.T) {
	// When Mapbox matches as "Address", callers override the Name to avoid a
	// bare street-address display. This test confirms the displayName arg is
	// what ends up on the proto, independent of p.Name.
	place := PlaceResult{
		Name:        "3393 Airport Road",
		FullAddress: "3393 Airport Road, Boulder, CO",
		Type:        "Address",
	}

	got := place.ToGeocodedLocation("Boulder Airport")
	if got.Name != "Boulder Airport" {
		t.Errorf("Name = %q, want %q (display-name override)", got.Name, "Boulder Airport")
	}
}
