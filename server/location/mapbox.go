package location

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
)

// MapboxClient provides geocoding services using the Mapbox Geocoding API.
type MapboxClient struct {
	accessToken string
	httpClient  *http.Client
}

// NewMapboxClient creates a new Mapbox geocoding client.
func NewMapboxClient(accessToken string) *MapboxClient {
	return &MapboxClient{
		accessToken: accessToken,
		httpClient:  &http.Client{},
	}
}

// mapboxFeature represents a single geocoding result from Mapbox API.
type mapboxFeature struct {
	Type     string `json:"type"`
	Geometry struct {
		Type        string    `json:"type"`
		Coordinates []float64 `json:"coordinates"` // [longitude, latitude]
	} `json:"geometry"`
	Properties struct {
		Name        string `json:"name"`
		MapboxID    string `json:"mapbox_id"`
		PlaceName   string `json:"place_name"`
		FullAddress string `json:"full_address"`
		FeatureType string `json:"feature_type"`
		Context     struct {
			Country *struct {
				Name      string `json:"name"`
				ShortCode string `json:"country_code"`
			} `json:"country"`
			Region *struct {
				Name       string `json:"name"`
				RegionCode string `json:"region_code"`
			} `json:"region"`
			Postcode *struct {
				Name string `json:"name"`
			} `json:"postcode"`
			Place *struct {
				Name string `json:"name"`
			} `json:"place"`
			Locality *struct {
				Name string `json:"name"`
			} `json:"locality"`
			Neighborhood *struct {
				Name string `json:"name"`
			} `json:"neighborhood"`
			District *struct {
				Name string `json:"name"`
			} `json:"district"`
			Address *struct {
				Name          string `json:"name"`
				AddressNumber string `json:"address_number"`
				StreetName    string `json:"street_name"`
			} `json:"address"`
		} `json:"context"`
	} `json:"properties"`
}

// mapboxResponse represents the response from Mapbox Geocoding API v6.
type mapboxResponse struct {
	Type     string          `json:"type"`
	Features []mapboxFeature `json:"features"`
}

// ForwardGeocode converts a structured address to coordinates using Mapbox v6 structured input API.
func (c *MapboxClient) ForwardGeocode(ctx context.Context, address *models.Address) (*models.Geolocation, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"external_service", "mapbox",
		"operation", "ForwardGeocode",
	)
	startTime := time.Now()

	if address == nil {
		return nil, fmt.Errorf("address cannot be nil")
	}

	// Build the structured geocoding API URL
	endpoint := "https://api.mapbox.com/search/geocode/v6/forward"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Build query parameters using structured input
	q := req.URL.Query()
	q.Add("access_token", c.accessToken)
	q.Add("limit", "1")            // Only return the best match
	q.Add("autocomplete", "false") // Better for structured input

	// Map our Address fields to Mapbox structured parameters
	if address.RegionCode != "" {
		q.Add("country", address.RegionCode)
	}
	if address.PostalCode != "" {
		q.Add("postcode", address.PostalCode)
	}
	if address.Locality != "" {
		q.Add("place", address.Locality)
	}

	// Handle address_lines - use the first line as the full address
	if len(address.AddressLines) > 0 {
		q.Add("address_line1", address.AddressLines[0])
	}

	req.URL.RawQuery = q.Encode()

	// Make the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.Error("mapbox geocoding request failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to make geocoding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Error("mapbox geocoding API error", "status", resp.StatusCode, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("geocoding API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var result mapboxResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logger.Error("failed to decode geocoding response", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to decode geocoding response: %w", err)
	}

	if len(result.Features) == 0 {
		logger.Warn("no results found for address", "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("no results found for address")
	}

	logger.Info("mapbox geocoding completed", "results", len(result.Features), "duration_ms", time.Since(startTime).Milliseconds())

	// Return only the geolocation
	feature := result.Features[0]
	return &models.Geolocation{
		LongitudeDeg: feature.Geometry.Coordinates[0],
		LatitudeDeg:  feature.Geometry.Coordinates[1],
	}, nil
}

// providerMapbox is the canonical value used in PlaceResult.ExternalProvider
// for results sourced from the Mapbox APIs. Kept as a constant so future
// providers (Google, OSM, etc.) can be added without typos.
const providerMapbox = "mapbox"

// PlaceResult represents a single place search result from a geocoding
// provider. Fields are provider-agnostic; ExternalID + ExternalProvider
// together form a stable identifier that can be used for deduplication.
type PlaceResult struct {
	Name        string
	FullAddress string
	Type        string // POI, Address, City, Locality, Place
	Coordinates struct {
		Longitude float64
		Latitude  float64
	}
	Confidence float64 // 0.0-1.0 score based on result relevance

	// External identifier for cross-session deduplication.
	// ExternalID is opaque; its meaning depends on ExternalProvider
	// (e.g., "poi.12345" when ExternalProvider is "mapbox").
	ExternalID       string
	ExternalProvider string // e.g. "mapbox"; empty if no stable ID was available

	// Administrative hierarchy extracted from the provider's context data.
	// Any of these may be empty if the provider didn't return them for
	// this particular result.
	Locality           string // e.g., "Boulder"
	RegionCode         string // e.g., "CO" or "US-CO"
	PostalCode         string // e.g., "80302"
	Neighborhood       string // e.g., "Capitol Hill"
	County             string // e.g., "Boulder County"
	AdministrativeArea string // e.g., "Colorado"
}

// searchBoxResponse represents the response from Mapbox Search Box API v1.
type searchBoxResponse struct {
	Features []struct {
		Type     string `json:"type"`
		Geometry struct {
			Type        string    `json:"type"`
			Coordinates []float64 `json:"coordinates"` // [longitude, latitude]
		} `json:"geometry"`
		Properties struct {
			Name           string           `json:"name"`
			MapboxID       string           `json:"mapbox_id"`
			FullAddress    string           `json:"full_address"`
			PlaceFormatted string           `json:"place_formatted"`
			FeatureType    string           `json:"feature_type"`
			Context        searchBoxContext `json:"context"`
		} `json:"properties"`
	} `json:"features"`
}

// searchBoxContext represents the administrative hierarchy Mapbox attaches
// to a Search Box result. All sub-objects are optional; a given feature
// type only populates the levels that apply to it.
type searchBoxContext struct {
	Address *struct {
		Name string `json:"name"`
	} `json:"address,omitempty"`
	Street *struct {
		Name string `json:"name"`
	} `json:"street,omitempty"`
	Neighborhood *struct {
		Name string `json:"name"`
	} `json:"neighborhood,omitempty"`
	Locality *struct {
		Name string `json:"name"`
	} `json:"locality,omitempty"`
	Postcode *struct {
		Name string `json:"name"`
	} `json:"postcode,omitempty"`
	Place *struct {
		Name string `json:"name"`
	} `json:"place,omitempty"`
	District *struct {
		Name string `json:"name"`
	} `json:"district,omitempty"`
	Region *struct {
		Name           string `json:"name"`
		RegionCode     string `json:"region_code"`
		RegionCodeFull string `json:"region_code_full"`
	} `json:"region,omitempty"`
	Country *struct {
		Name        string `json:"name"`
		CountryCode string `json:"country_code"`
	} `json:"country,omitempty"`
}

// SearchPlaces searches for places using the Mapbox Search Box API v1 with
// optional proximity influence. Returns up to 'limit' results sorted by
// relevance. Mapbox's proximity parameter is a soft bias; ProximityHint
// and ProximityBound both map to that parameter, and ProximityBound
// results are further post-filtered to drop matches outside the requested
// radius (expanded by BoundRadiusExpansionFactor).
func (c *MapboxClient) SearchPlaces(ctx context.Context, query string, proximity ProximityRequest, limit int) ([]PlaceResult, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"external_service", "mapbox",
		"operation", "SearchPlaces",
	)
	startTime := time.Now()

	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	if limit <= 0 || limit > 10 {
		limit = 10 // Default to 10 results (matches client)
	}

	// Use Mapbox Search Box API v1 (same as client)
	endpoint := "https://api.mapbox.com/search/searchbox/v1/forward"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Build query parameters (matches client implementation)
	q := req.URL.Query()
	q.Add("q", query)
	q.Add("access_token", c.accessToken)
	q.Add("types", "poi,address,place,locality")
	q.Add("limit", fmt.Sprintf("%d", limit))
	q.Add("auto_complete", "true") // Enable autocomplete mode

	// Mapbox's proximity is a soft bias; we use it for both Hint and Bound
	// semantics and post-filter Bound results after parsing.
	if proximity.HasCoord() {
		q.Add("proximity", fmt.Sprintf("%f,%f", proximity.Coord.LongitudeDeg, proximity.Coord.LatitudeDeg))
	}

	req.URL.RawQuery = q.Encode()

	// Make the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.Error("mapbox search request failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to make search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Error("mapbox search API error", "status", resp.StatusCode, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("search API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var result searchBoxResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logger.Error("failed to decode search response", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	// Convert to PlaceResult format
	places := make([]PlaceResult, 0, len(result.Features))
	for i, feature := range result.Features {
		// Determine type from feature_type
		featureType := feature.Properties.FeatureType
		placeType := "Place"
		switch featureType {
		case "address":
			placeType = "Address"
		case "poi":
			placeType = "POI"
		case "place", "city":
			placeType = "City"
		case "locality":
			placeType = "Locality"
		}

		// Get full address (try multiple fields)
		fullAddress := feature.Properties.FullAddress
		if fullAddress == "" {
			fullAddress = feature.Properties.PlaceFormatted
		}
		if fullAddress == "" {
			fullAddress = feature.Properties.Name
		}

		place := PlaceResult{
			Name:        feature.Properties.Name,
			FullAddress: fullAddress,
			Type:        placeType,
			// Confidence decreases with result position (first result = highest confidence)
			Confidence: 1.0 - (float64(i) * 0.1), // 1.0, 0.9, 0.8, etc.
		}
		place.Coordinates.Longitude = feature.Geometry.Coordinates[0]
		place.Coordinates.Latitude = feature.Geometry.Coordinates[1]

		// Capture a stable external identifier for deduplication.
		// Older Mapbox Search Box responses omit this; callers should
		// tolerate an empty ExternalID.
		if feature.Properties.MapboxID != "" {
			place.ExternalID = feature.Properties.MapboxID
			place.ExternalProvider = providerMapbox
		}

		// Extract administrative hierarchy from context.
		populatePlaceFromSearchBoxContext(&place, &feature.Properties.Context)

		places = append(places, place)
	}

	// Apply the ProximityBound post-filter when requested. Mapbox's
	// proximity hint biases ranking but doesn't enforce a radius, so we
	// drop cross-region matches here.
	if proximity.Semantics == ProximityBound {
		before := len(places)
		places = filterByProximityBound(places, proximity)
		if dropped := before - len(places); dropped > 0 {
			logger.Debug("mapbox bound filter dropped out-of-radius results",
				"dropped", dropped, "remaining", len(places))
		}
	}

	logger.Info("mapbox search completed", "results", len(places), "duration_ms", time.Since(startTime).Milliseconds())

	return places, nil
}

// populatePlaceFromSearchBoxContext fills in the administrative hierarchy
// fields on a PlaceResult from Mapbox Search Box context data. Fields left
// empty in the context are left empty on the result; callers must not
// assume any particular field is populated.
func populatePlaceFromSearchBoxContext(place *PlaceResult, ctx *searchBoxContext) {
	if ctx == nil {
		return
	}

	// Locality — prefer the "place" level (city) over "locality" since
	// Mapbox uses "place" for US cities and "locality" for sub-city areas.
	if ctx.Place != nil && ctx.Place.Name != "" {
		place.Locality = ctx.Place.Name
	} else if ctx.Locality != nil && ctx.Locality.Name != "" {
		place.Locality = ctx.Locality.Name
	}

	if ctx.Neighborhood != nil {
		place.Neighborhood = ctx.Neighborhood.Name
	}

	if ctx.District != nil {
		place.County = ctx.District.Name
	}

	if ctx.Region != nil {
		place.AdministrativeArea = ctx.Region.Name
		// Prefer the short region_code (e.g., "CO"); fall back to full
		// form (e.g., "US-CO") so the field is always populated when
		// region data exists.
		switch {
		case ctx.Region.RegionCode != "":
			place.RegionCode = strings.ToUpper(ctx.Region.RegionCode)
		case ctx.Region.RegionCodeFull != "":
			place.RegionCode = strings.ToUpper(ctx.Region.RegionCodeFull)
		}
	}

	if ctx.Postcode != nil {
		place.PostalCode = ctx.Postcode.Name
	}
}

// ReverseGeocode converts coordinates to a PlaceResult using Mapbox v6 API.
// The returned PlaceResult includes the external place identifier, human-
// readable name, and full administrative hierarchy when available.
func (c *MapboxClient) ReverseGeocode(ctx context.Context, geolocation *models.Geolocation) (*PlaceResult, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"external_service", "mapbox",
		"operation", "ReverseGeocode",
	)
	startTime := time.Now()

	if geolocation == nil {
		return nil, fmt.Errorf("geolocation cannot be nil")
	}

	// Validate coordinates
	if geolocation.LatitudeDeg < -90 || geolocation.LatitudeDeg > 90 {
		return nil, fmt.Errorf("latitude must be between -90 and 90")
	}
	if geolocation.LongitudeDeg < -180 || geolocation.LongitudeDeg > 180 {
		return nil, fmt.Errorf("longitude must be between -180 and 180")
	}

	// Build the API URL (note: Mapbox uses longitude,latitude order)
	endpoint := fmt.Sprintf("https://api.mapbox.com/search/geocode/v6/reverse?longitude=%f&latitude=%f",
		geolocation.LongitudeDeg, geolocation.LatitudeDeg)

	// G704 sees caller data reaching an outbound request. The host is a literal
	// and the only caller-supplied values are two float64s that the range checks
	// above have already bounded to [-90,90] and [-180,180], rendered with %f —
	// a float64 cannot introduce a host, a path, or a scheme.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil) //nolint:gosec // G704: fixed host; the only interpolated values are range-checked float64s.
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Add("access_token", c.accessToken)
	q.Add("limit", "1") // Only return the best match
	req.URL.RawQuery = q.Encode()

	// Make the request
	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: fixed host; the only interpolated values are range-checked float64s.
	if err != nil {
		logger.Error("mapbox reverse geocoding request failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to make reverse geocoding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Error("mapbox reverse geocoding API error", "status", resp.StatusCode, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("reverse geocoding API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var result mapboxResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logger.Error("failed to decode reverse geocoding response", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to decode reverse geocoding response: %w", err)
	}

	if len(result.Features) == 0 {
		logger.Warn("no results found for coordinates", "lat", geolocation.LatitudeDeg, "lng", geolocation.LongitudeDeg, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("no results found for coordinates: %.6f,%.6f",
			geolocation.LatitudeDeg, geolocation.LongitudeDeg)
	}

	logger.Info("mapbox reverse geocoding completed", "results", len(result.Features), "duration_ms", time.Since(startTime).Milliseconds())

	feature := result.Features[0]
	return buildPlaceResultFromFeature(&feature), nil
}

// buildPlaceResultFromFeature constructs a PlaceResult from a Mapbox
// Geocoding v6 feature (used by both forward and reverse geocoding).
func buildPlaceResultFromFeature(feature *mapboxFeature) *PlaceResult {
	props := feature.Properties
	ctx := props.Context

	// Map feature_type to the PlaceResult.Type taxonomy used by callers.
	placeType := "Place"
	switch props.FeatureType {
	case "address":
		placeType = "Address"
	case "poi":
		placeType = "POI"
	case "place", "city":
		placeType = "City"
	case "locality":
		placeType = "Locality"
	}

	// Prefer the explicit address line; fall back to full_address so the
	// caller never gets an empty address when Mapbox returned *something*.
	var addressLine string
	switch {
	case ctx.Address != nil && ctx.Address.Name != "":
		addressLine = ctx.Address.Name
	case props.FullAddress != "":
		addressLine = props.FullAddress
	}

	place := &PlaceResult{
		Name:        props.Name,
		FullAddress: addressLine,
		Type:        placeType,
	}
	if len(feature.Geometry.Coordinates) >= 2 {
		place.Coordinates.Longitude = feature.Geometry.Coordinates[0]
		place.Coordinates.Latitude = feature.Geometry.Coordinates[1]
	}

	if props.MapboxID != "" {
		place.ExternalID = props.MapboxID
		place.ExternalProvider = providerMapbox
	}

	// Administrative hierarchy.
	if ctx.Place != nil && ctx.Place.Name != "" {
		place.Locality = ctx.Place.Name
	} else if ctx.Locality != nil && ctx.Locality.Name != "" {
		place.Locality = ctx.Locality.Name
	}
	if ctx.Neighborhood != nil {
		place.Neighborhood = ctx.Neighborhood.Name
	}
	if ctx.District != nil {
		place.County = ctx.District.Name
	}
	if ctx.Region != nil {
		place.AdministrativeArea = ctx.Region.Name
		if ctx.Region.RegionCode != "" {
			place.RegionCode = strings.ToUpper(ctx.Region.RegionCode)
		}
	}
	if ctx.Postcode != nil {
		place.PostalCode = ctx.Postcode.Name
	}
	// Country short code takes precedence if no region code was set, so the
	// RegionCode field always has some country/state indicator when Mapbox
	// returned one (callers expecting "US" style country codes still get it).
	if place.RegionCode == "" && ctx.Country != nil && ctx.Country.ShortCode != "" {
		place.RegionCode = strings.ToUpper(ctx.Country.ShortCode)
	}

	return place
}

// mapboxTokenResponse represents the response from Mapbox token validation endpoint.
type mapboxTokenResponse struct {
	Code string `json:"code,omitempty"` // "TokenValid" for success, or error codes like "TokenMalformed", "TokenInvalid"
}

// isMapboxTokenError returns true if the code indicates an invalid token.
func isMapboxTokenError(code string) bool {
	switch code {
	case "TokenMalformed", "TokenInvalid", "TokenExpired", "TokenRevoked":
		return true
	default:
		return false
	}
}

// CheckHealth validates Mapbox API connectivity using the token validation endpoint (free API call).
func (c *MapboxClient) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	start := time.Now()

	status := &health.Status{
		Name:    "location",
		Backend: "mapbox",
	}

	// Use the Mapbox tokens API to validate the access token (free endpoint)
	endpoint := "https://api.mapbox.com/tokens/v2?access_token=" + c.accessToken

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		status.LatencyMs = time.Since(start).Milliseconds()
		status.Error = err.Error()
		return []*health.Status{status}, nil
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		status.LatencyMs = time.Since(start).Milliseconds()
		status.Error = err.Error()
		return []*health.Status{status}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		status.LatencyMs = time.Since(start).Milliseconds()
		status.Error = fmt.Sprintf("failed to read response: %v", err)
		return []*health.Status{status}, nil
	}

	status.LatencyMs = time.Since(start).Milliseconds()

	if resp.StatusCode != http.StatusOK {
		status.Error = fmt.Sprintf("status %d: %s", resp.StatusCode, string(body))
		return []*health.Status{status}, nil
	}

	// Mapbox returns HTTP 200 even for invalid tokens, but includes an error code in the response body.
	// Check for error codes like "TokenMalformed" or "TokenInvalid" (but not "TokenValid" which means success).
	var tokenResp mapboxTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err == nil && isMapboxTokenError(tokenResp.Code) {
		status.Error = tokenResp.Code
		return []*health.Status{status}, nil
	}

	return []*health.Status{status}, nil
}
