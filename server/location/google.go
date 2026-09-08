package location

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
)

// providerGoogleMaps is the canonical value used in PlaceResult.ExternalProvider
// and health.Status.Backend for results sourced from Google Maps Platform.
// It also matches the external_service log field per docs/server/observability.md.
const providerGoogleMaps = "google_maps"

// Google Maps Platform endpoints. Geocoding uses the classic Maps API host
// (key in query string); Places API (New) uses the modern Places host with
// the key in the X-Goog-Api-Key header.
const (
	googleGeocodingURL          = "https://maps.googleapis.com/maps/api/geocode/json"
	googlePlacesAutocompleteURL = "https://places.googleapis.com/v1/places:autocomplete"
	googlePlaceDetailsURLBase   = "https://places.googleapis.com/v1/places/"
)

// placeDetailsFieldMask scopes Place Details billing to the cheaper
// "Place Details (Basic)" SKU. Adding atmosphere or contact fields
// would jump us to higher tiers — keep this list minimal.
const placeDetailsFieldMask = "id,displayName,formattedAddress,location,types,addressComponents"

// healthCheckTimeout caps the latency the health endpoint will wait on
// Google before reporting unhealthy. Probes run on the request path of
// /readyz; keep this short so an outage doesn't stall the readiness probe.
const healthCheckTimeout = 10 * time.Second

// healthCheckAddress is a stable address the Geocoding API has resolved
// reliably since launch. Used as the probe target for CheckHealth.
const healthCheckAddress = "1600 Amphitheatre Parkway, Mountain View, CA"

// GoogleMapsClient calls Google Maps Platform APIs (Geocoding API and
// Places API New) over HTTP. One instance handles all Provider methods.
type GoogleMapsClient struct {
	apiKey     string
	httpClient *http.Client
}

// NewGoogleMapsClient creates a client backed by the supplied API key.
// The key must have at least the Geocoding API and Places API (New)
// enabled, plus whatever application restrictions are appropriate for
// the calling environment (server-side keys are typically IP-restricted).
func NewGoogleMapsClient(apiKey string) *GoogleMapsClient {
	return &GoogleMapsClient{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// === Geocoding API response shapes ===.

type googleGeocodingResponse struct {
	Status       string                  `json:"status"`
	Results      []googleGeocodingResult `json:"results"`
	ErrorMessage string                  `json:"error_message,omitempty"`
}

type googleGeocodingResult struct {
	FormattedAddress  string                   `json:"formatted_address"`
	Geometry          googleGeometry           `json:"geometry"`
	PlaceID           string                   `json:"place_id"`
	Types             []string                 `json:"types"`
	AddressComponents []googleAddressComponent `json:"address_components"`
}

type googleGeometry struct {
	Location googleLatLng `json:"location"`
}

type googleLatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type googleAddressComponent struct {
	LongName  string   `json:"long_name"`
	ShortName string   `json:"short_name"`
	Types     []string `json:"types"`
}

// === Places API (New) request + response shapes ===.

type googleAutocompleteRequest struct {
	Input                string                `json:"input"`
	LocationBias         *googleLocationCircle `json:"locationBias,omitempty"`
	LocationRestriction  *googleLocationCircle `json:"locationRestriction,omitempty"`
	IncludedPrimaryTypes []string              `json:"includedPrimaryTypes,omitempty"`
}

type googleLocationCircle struct {
	Circle googleCircle `json:"circle"`
}

type googleCircle struct {
	Center googleLocation `json:"center"`
	Radius float64        `json:"radius"`
}

type googleLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type googleAutocompleteResponse struct {
	Suggestions []googlePlaceSuggestion `json:"suggestions"`
}

type googlePlaceSuggestion struct {
	PlacePrediction *googlePlacePrediction `json:"placePrediction,omitempty"`
}

type googlePlacePrediction struct {
	PlaceID          string                  `json:"placeId"`
	Text             googleLocalizedText     `json:"text"`
	StructuredFormat *googleStructuredFormat `json:"structuredFormat,omitempty"`
	Types            []string                `json:"types"`
}

type googleLocalizedText struct {
	Text string `json:"text"`
}

type googleStructuredFormat struct {
	MainText      googleLocalizedText `json:"mainText"`
	SecondaryText googleLocalizedText `json:"secondaryText"`
}

type googlePlaceDetailsResponse struct {
	ID                string                        `json:"id"`
	DisplayName       googleLocalizedText           `json:"displayName"`
	FormattedAddress  string                        `json:"formattedAddress"`
	Location          googleLocation                `json:"location"`
	Types             []string                      `json:"types"`
	AddressComponents []googlePlaceAddressComponent `json:"addressComponents"`
}

type googlePlaceAddressComponent struct {
	LongText  string   `json:"longText"`
	ShortText string   `json:"shortText"`
	Types     []string `json:"types"`
}

// ForwardGeocode converts a structured address to coordinates using the
// Geocoding API with structured component filtering.
func (c *GoogleMapsClient) ForwardGeocode(ctx context.Context, address *models.Address) (*models.Geolocation, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"external_service", providerGoogleMaps,
		"operation", "ForwardGeocode",
	)
	startTime := time.Now()

	if address == nil {
		return nil, fmt.Errorf("address cannot be nil")
	}

	q := url.Values{}
	if len(address.AddressLines) > 0 {
		q.Set("address", address.AddressLines[0])
	}
	q.Set("components", buildGoogleComponents(address))
	q.Set("key", c.apiKey)

	endpoint := googleGeocodingURL + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	result, err := c.doGeocodingRequest(ctx, logger, req, startTime)
	if err != nil {
		return nil, err
	}
	if len(result.Results) == 0 {
		logger.WarnContext(ctx, "no results found for address",
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("no results found for address")
	}

	logger.InfoContext(ctx, "google forward geocoding completed",
		"results", len(result.Results),
		"duration_ms", time.Since(startTime).Milliseconds())

	loc := result.Results[0].Geometry.Location
	return &models.Geolocation{
		LatitudeDeg:  loc.Lat,
		LongitudeDeg: loc.Lng,
	}, nil
}

// SearchPlaces searches for places using Places Autocomplete (New) for
// suggestions and Place Details (New) for coordinates. The proximity
// semantics map onto Google's two location modes:
//   - ProximityNone: no location parameter; global ranking.
//   - ProximityHint: locationBias.circle (soft bias, far results allowed).
//   - ProximityBound: locationRestriction.circle plus a server-side
//     post-filter (Google's restriction is loose at the edge — see the
//     parity audit; the post-filter drops the wrong-state matches).
func (c *GoogleMapsClient) SearchPlaces(ctx context.Context, query string, proximity ProximityRequest, limit int) ([]PlaceResult, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"external_service", providerGoogleMaps,
		"operation", "SearchPlaces",
	)
	startTime := time.Now()

	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	if limit <= 0 || limit > 10 {
		limit = 10
	}

	autoReq := googleAutocompleteRequest{Input: query}
	if proximity.HasCoord() {
		circle := &googleLocationCircle{
			Circle: googleCircle{
				Center: googleLocation{
					Latitude:  proximity.Coord.LatitudeDeg,
					Longitude: proximity.Coord.LongitudeDeg,
				},
				Radius: proximityRadiusMeters(proximity),
			},
		}
		switch proximity.Semantics {
		case ProximityBound:
			autoReq.LocationRestriction = circle
		case ProximityHint:
			autoReq.LocationBias = circle
		case ProximityNone:
			// no location parameter
		}
	}

	suggestions, err := c.fetchAutocomplete(ctx, logger, autoReq, startTime)
	if err != nil {
		return nil, err
	}
	if len(suggestions) == 0 {
		logger.InfoContext(ctx, "google search returned no suggestions",
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, nil
	}

	if len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}

	places := make([]PlaceResult, 0, len(suggestions))
	for i, s := range suggestions {
		if s.PlacePrediction == nil || s.PlacePrediction.PlaceID == "" {
			continue
		}
		details, err := c.fetchPlaceDetails(ctx, logger, s.PlacePrediction.PlaceID)
		if err != nil {
			logger.WarnContext(ctx, "google place details failed",
				"place_id", s.PlacePrediction.PlaceID, "error", err)
			continue
		}
		place := buildPlaceResultFromGoogleDetails(details)
		// Confidence decreases with result position to match Mapbox.
		place.Confidence = 1.0 - (float64(i) * 0.1)
		places = append(places, place)
	}

	if proximity.Semantics == ProximityBound {
		before := len(places)
		places = filterByProximityBound(places, proximity)
		if dropped := before - len(places); dropped > 0 {
			logger.Debug("google bound filter dropped out-of-radius results",
				"dropped", dropped, "remaining", len(places))
		}
	}

	logger.InfoContext(ctx, "google search completed",
		"results", len(places),
		"duration_ms", time.Since(startTime).Milliseconds())
	return places, nil
}

// ReverseGeocode converts coordinates to a single best-match PlaceResult
// using the Geocoding API. The result_type filter prefers real addresses
// over nearby businesses; Plus-Code-only results are dropped because they
// are not user-readable.
func (c *GoogleMapsClient) ReverseGeocode(ctx context.Context, geolocation *models.Geolocation) (*PlaceResult, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"external_service", providerGoogleMaps,
		"operation", "ReverseGeocode",
	)
	startTime := time.Now()

	if geolocation == nil {
		return nil, fmt.Errorf("geolocation cannot be nil")
	}
	if geolocation.LatitudeDeg < -90 || geolocation.LatitudeDeg > 90 {
		return nil, fmt.Errorf("latitude must be between -90 and 90")
	}
	if geolocation.LongitudeDeg < -180 || geolocation.LongitudeDeg > 180 {
		return nil, fmt.Errorf("longitude must be between -180 and 180")
	}

	q := url.Values{}
	q.Set("latlng", fmt.Sprintf("%f,%f", geolocation.LatitudeDeg, geolocation.LongitudeDeg))
	// Restrict the response to address-like types. The pipe-separated list
	// is interpreted as OR by the Geocoding API. This excludes pure Plus
	// Codes and de-prioritizes establishment / point_of_interest matches.
	q.Set("result_type", "street_address|route|premise|subpremise|locality")
	q.Set("key", c.apiKey)

	endpoint := googleGeocodingURL + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	result, err := c.doGeocodingRequest(ctx, logger, req, startTime)
	if err != nil {
		return nil, err
	}

	// Drop Plus-Code-only results defensively even though result_type
	// should already exclude them. See parity audit:
	// docs/issues/2188-parity-audit.md "Required mitigations".
	best := firstNonPlusCodeResult(result.Results)
	if best == nil {
		logger.WarnContext(ctx, "no usable reverse-geocode result",
			"lat", geolocation.LatitudeDeg,
			"lng", geolocation.LongitudeDeg,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("no results found for coordinates: %.6f,%.6f",
			geolocation.LatitudeDeg, geolocation.LongitudeDeg)
	}

	logger.InfoContext(ctx, "google reverse geocoding completed",
		"results", len(result.Results),
		"duration_ms", time.Since(startTime).Milliseconds())

	place := buildPlaceResultFromGeocodingResult(best)
	return &place, nil
}

// CheckHealth probes the Geocoding API with a fixed known-good address.
// OK or ZERO_RESULTS counts as healthy (the key works); anything else
// surfaces the upstream status as the error string.
func (c *GoogleMapsClient) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	start := time.Now()
	status := &health.Status{
		Name:    "location",
		Backend: providerGoogleMaps,
	}

	q := url.Values{}
	q.Set("address", healthCheckAddress)
	q.Set("key", c.apiKey)
	endpoint := googleGeocodingURL + "?" + q.Encode()

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
		status.Error = fmt.Sprintf("status %d", resp.StatusCode)
		return []*health.Status{status}, nil
	}

	var decoded googleGeocodingResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		status.Error = fmt.Sprintf("decode: %v", err)
		return []*health.Status{status}, nil
	}

	switch decoded.Status {
	case "OK", "ZERO_RESULTS":
		return []*health.Status{status}, nil
	default:
		// REQUEST_DENIED, OVER_QUERY_LIMIT, INVALID_REQUEST, UNKNOWN_ERROR.
		// Report the status code; error_message (when present) often
		// includes the key fingerprint, so leave it out of the health
		// payload that may be served publicly.
		status.Error = decoded.Status
		return []*health.Status{status}, nil
	}
}

// fetchAutocomplete posts to Places Autocomplete (New) and returns the
// suggestions slice. Errors are wrapped with operation context; callers
// log with their own structured fields.
func (c *GoogleMapsClient) fetchAutocomplete(ctx context.Context, logger *logging.Logger, body googleAutocompleteRequest, startTime time.Time) ([]googlePlaceSuggestion, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal autocomplete request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googlePlacesAutocompleteURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create autocomplete request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.ErrorContext(ctx, "google autocomplete request failed",
			"error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to make autocomplete request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		logger.ErrorContext(ctx, "google autocomplete API error",
			"status", resp.StatusCode,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("autocomplete API returned status %d", resp.StatusCode)
	}

	var decoded googleAutocompleteResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		logger.ErrorContext(ctx, "failed to decode autocomplete response",
			"error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to decode autocomplete response: %w", err)
	}
	return decoded.Suggestions, nil
}

// fetchPlaceDetails resolves a placeId to a Place Details payload. The
// field mask is set to the cheapest Place Details Basic SKU; expanding
// it bumps every call into a more expensive tier.
func (c *GoogleMapsClient) fetchPlaceDetails(ctx context.Context, logger *logging.Logger, placeID string) (*googlePlaceDetailsResponse, error) {
	if placeID == "" {
		return nil, fmt.Errorf("placeID cannot be empty")
	}

	endpoint := googlePlaceDetailsURLBase + url.PathEscape(placeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create place details request: %w", err)
	}
	req.Header.Set("X-Goog-Api-Key", c.apiKey)
	req.Header.Set("X-Goog-Fieldmask", placeDetailsFieldMask)

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.ErrorContext(ctx, "google place details request failed",
			"error", err, "duration_ms", time.Since(start).Milliseconds())
		return nil, fmt.Errorf("failed to make place details request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		logger.ErrorContext(ctx, "google place details API error",
			"status", resp.StatusCode,
			"duration_ms", time.Since(start).Milliseconds())
		return nil, fmt.Errorf("place details API returned status %d", resp.StatusCode)
	}

	var decoded googlePlaceDetailsResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		logger.ErrorContext(ctx, "failed to decode place details response",
			"error", err, "duration_ms", time.Since(start).Milliseconds())
		return nil, fmt.Errorf("failed to decode place details response: %w", err)
	}
	return &decoded, nil
}

// doGeocodingRequest executes a Geocoding API GET request and decodes
// the wrapper envelope. Returns the decoded response on OK or
// ZERO_RESULTS; any other Geocoding status becomes an error.
func (c *GoogleMapsClient) doGeocodingRequest(ctx context.Context, logger *logging.Logger, req *http.Request, startTime time.Time) (*googleGeocodingResponse, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.ErrorContext(ctx, "google geocoding request failed",
			"error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to make geocoding request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read geocoding response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		logger.ErrorContext(ctx, "google geocoding API error",
			"status", resp.StatusCode,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("geocoding API returned status %d", resp.StatusCode)
	}

	var decoded googleGeocodingResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		logger.ErrorContext(ctx, "failed to decode geocoding response",
			"error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to decode geocoding response: %w", err)
	}

	switch decoded.Status {
	case "OK", "ZERO_RESULTS":
		return &decoded, nil
	default:
		// REQUEST_DENIED, OVER_QUERY_LIMIT, INVALID_REQUEST, UNKNOWN_ERROR.
		// The error_message body field can include the key; do not log it
		// at higher severity than warn.
		return nil, fmt.Errorf("geocoding API status %s", decoded.Status)
	}
}

// buildGoogleComponents formats the `components` query parameter for the
// Geocoding API from a structured Address. Empty components are skipped.
// Format: pipe-separated key:value pairs (e.g. "country:US|locality:Boulder").
func buildGoogleComponents(address *models.Address) string {
	if address == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if address.RegionCode != "" {
		parts = append(parts, "country:"+address.RegionCode)
	}
	if address.PostalCode != "" {
		parts = append(parts, "postal_code:"+address.PostalCode)
	}
	if address.Locality != "" {
		parts = append(parts, "locality:"+address.Locality)
	}
	return strings.Join(parts, "|")
}

// proximityRadiusMeters returns the radius in meters to send in the
// Google location parameter. Defaults to DefaultBoundRadiusM when the
// caller didn't specify; Google's max for circle radius is 50 km.
func proximityRadiusMeters(req ProximityRequest) float64 {
	if req.RadiusM > 0 {
		return float64(req.RadiusM)
	}
	return float64(DefaultBoundRadiusM)
}

// firstNonPlusCodeResult returns the first geocoding result whose types
// include at least one address-like component. Results whose only type is
// plus_code are skipped — Plus Codes are not user-readable and shouldn't
// be saved as a location name (see parity audit).
func firstNonPlusCodeResult(results []googleGeocodingResult) *googleGeocodingResult {
	for i := range results {
		if !isPlusCodeOnly(results[i].Types) {
			return &results[i]
		}
	}
	return nil
}

// isPlusCodeOnly reports whether the type set consists exclusively of
// plus_code-related entries (with no street_address, route, locality,
// or other address-like type to fall back on).
func isPlusCodeOnly(types []string) bool {
	if len(types) == 0 {
		return false
	}
	for _, t := range types {
		if t != "plus_code" && t != "compound_plus_code" && t != "global_plus_code" {
			return false
		}
	}
	return true
}

// buildPlaceResultFromGoogleDetails converts a Place Details (New) response
// to the provider-neutral PlaceResult shape.
func buildPlaceResultFromGoogleDetails(d *googlePlaceDetailsResponse) PlaceResult {
	place := PlaceResult{
		Name:             d.DisplayName.Text,
		FullAddress:      d.FormattedAddress,
		Type:             googleTypeForResult(d.Types),
		ExternalID:       d.ID,
		ExternalProvider: providerGoogleMaps,
	}
	place.Coordinates.Latitude = d.Location.Latitude
	place.Coordinates.Longitude = d.Location.Longitude
	populatePlaceFromGoogleAddressComponents(&place, d.AddressComponents)
	return place
}

// buildPlaceResultFromGeocodingResult converts a Geocoding API result to
// the provider-neutral PlaceResult shape.
func buildPlaceResultFromGeocodingResult(r *googleGeocodingResult) PlaceResult {
	// Geocoding has no display-name field; derive a "name" by walking the
	// components for the most specific level we have.
	name := nameFromGeocodingComponents(r.AddressComponents)
	if name == "" {
		name = r.FormattedAddress
	}
	place := PlaceResult{
		Name:             name,
		FullAddress:      r.FormattedAddress,
		Type:             googleTypeForResult(r.Types),
		ExternalID:       r.PlaceID,
		ExternalProvider: providerGoogleMaps,
	}
	place.Coordinates.Latitude = r.Geometry.Location.Lat
	place.Coordinates.Longitude = r.Geometry.Location.Lng
	// Convert the geocoding-shape components to the places-shape and
	// reuse the same populator.
	mapped := make([]googlePlaceAddressComponent, len(r.AddressComponents))
	for i, c := range r.AddressComponents {
		mapped[i] = googlePlaceAddressComponent{
			LongText:  c.LongName,
			ShortText: c.ShortName,
			Types:     c.Types,
		}
	}
	populatePlaceFromGoogleAddressComponents(&place, mapped)
	return place
}

// nameFromGeocodingComponents returns the most specific human-readable
// name available in the address-components list. For a street_address
// it returns the street_number + route; for a locality it returns the
// locality name.
func nameFromGeocodingComponents(components []googleAddressComponent) string {
	var streetNumber, route, locality string
	for _, c := range components {
		switch {
		case containsType(c.Types, "street_number"):
			streetNumber = c.LongName
		case containsType(c.Types, "route"):
			route = c.LongName
		case containsType(c.Types, googleTypeLocality):
			locality = c.LongName
		}
	}
	if route != "" {
		if streetNumber != "" {
			return streetNumber + " " + route
		}
		return route
	}
	return locality
}

// PlaceResult.Type taxonomy values. This is our own vocabulary, not the
// provider's — [googleTypeForResult] maps the provider's much larger types[]
// array down onto these five.
const (
	placeTypeAddress  = "Address"
	placeTypePOI      = "POI"
	placeTypeCity     = "City"
	placeTypeLocality = "Locality"
	placeTypePlace    = "Place"
)

// googleTypeLocality is the provider's address-component type for a city. It
// is read in three places to pick out the city name, and is distinct from
// [placeTypeCity], which is what we report back.
const googleTypeLocality = "locality"

// googleTypeForResult maps a Google types[] array onto the PlaceResult.Type
// taxonomy callers expect (POI, Address, City, Locality, Place).
func googleTypeForResult(types []string) string {
	for _, t := range types {
		switch t {
		case "street_address", "premise", "subpremise", "route":
			return placeTypeAddress
		case "establishment", "point_of_interest":
			return placeTypePOI
		case googleTypeLocality:
			return placeTypeCity
		case "sublocality", "neighborhood":
			return placeTypeLocality
		}
	}
	return placeTypePlace
}

// populatePlaceFromGoogleAddressComponents fills in the administrative
// hierarchy fields on a PlaceResult from a Place Details / Geocoding
// address_components array.
func populatePlaceFromGoogleAddressComponents(place *PlaceResult, components []googlePlaceAddressComponent) {
	for _, c := range components {
		switch {
		case containsType(c.Types, googleTypeLocality):
			place.Locality = c.LongText
		case containsType(c.Types, "postal_code"):
			place.PostalCode = c.LongText
		case containsType(c.Types, "neighborhood"):
			place.Neighborhood = c.LongText
		case containsType(c.Types, "administrative_area_level_2"):
			place.County = c.LongText
		case containsType(c.Types, "administrative_area_level_1"):
			place.AdministrativeArea = c.LongText
			if c.ShortText != "" {
				place.RegionCode = strings.ToUpper(c.ShortText)
			}
		case containsType(c.Types, "country"):
			if place.RegionCode == "" && c.ShortText != "" {
				place.RegionCode = strings.ToUpper(c.ShortText)
			}
		}
	}
}

// containsType reports whether t is in the types slice (case-sensitive,
// as Google's types are stable lowercase strings).
func containsType(types []string, t string) bool {
	return slices.Contains(types, t)
}
