package location

import (
	api "go.ripls.org/ripls/server/gen/ripls/api"
)

// ToGeocodedLocation converts a Mapbox PlaceResult into the api.GeocodedLocation
// proto returned to clients. displayName is used for the Name field — callers
// pass p.Name by default, or override (for example, when Mapbox matches as
// "Address", the p.Name is just the street address and callers prefer a
// JSON-LD place name or FullAddress instead).
//
// The mapping is mechanical — every field on PlaceResult that has a
// corresponding proto field is copied. This method lives here so that all
// Gen* handlers (experience, gear, request) share one canonical conversion;
// prior to extraction, each handler had its own verbatim copy of this struct
// literal.
func (p PlaceResult) ToGeocodedLocation(displayName string) *api.GeocodedLocation {
	return &api.GeocodedLocation{
		Name:                  displayName,
		LatitudeDeg:           p.Coordinates.Latitude,
		LongitudeDeg:          p.Coordinates.Longitude,
		Locality:              p.Locality,
		RegionCode:            p.RegionCode,
		PostalCode:            p.PostalCode,
		AddressLines:          []string{p.FullAddress},
		ExternalPlaceId:       p.ExternalID,
		ExternalPlaceProvider: p.ExternalProvider,
		Neighborhood:          p.Neighborhood,
		County:                p.County,
		AdministrativeArea:    p.AdministrativeArea,
	}
}
