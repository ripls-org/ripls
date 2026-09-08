package location

import (
	"context"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
)

// Provider is the geocoding and place-search abstraction used by server
// services. Implementations wrap an external geocoding API; callers must
// not depend on the concrete type.
//
// All methods accept a context.Context for cancellation and request-scoped
// logging. Result types (PlaceResult, models.Geolocation, health.Status)
// are provider-neutral; ExternalID + ExternalProvider on PlaceResult
// identify the underlying vendor for deduplication.
type Provider interface {
	// ForwardGeocode converts a structured address to coordinates.
	ForwardGeocode(ctx context.Context, address *models.Address) (*models.Geolocation, error)

	// SearchPlaces returns up to limit place results matching query, sorted
	// by relevance. The proximity argument declares whether and how a
	// reference coordinate should influence ranking and filtering; see
	// ProximityRequest for the per-semantic contract.
	SearchPlaces(ctx context.Context, query string, proximity ProximityRequest, limit int) ([]PlaceResult, error)

	// ReverseGeocode converts coordinates to a single best-match PlaceResult.
	ReverseGeocode(ctx context.Context, geolocation *models.Geolocation) (*PlaceResult, error)

	// CheckHealth probes the upstream API and returns one or more health
	// statuses describing the provider's reachability and authentication
	// state. Returning a non-nil status with Status.Error set is the
	// canonical way to signal an unhealthy provider; returning an error
	// indicates the check itself could not be performed.
	CheckHealth(ctx context.Context) ([]*health.Status, error)
}

// ProximitySemantics declares how a SearchPlaces caller wants the
// proximity hint to affect ranking and filtering. Different call sites
// have materially different needs, and the Google Places API distinguishes
// soft bias (locationBias) from hard restriction (locationRestriction);
// modeling the intent here lets each provider apply its closest analogue.
type ProximitySemantics int

const (
	// ProximityNone disables proximity-based ranking and filtering.
	// Use when the user is searching globally with no local context
	// (e.g. configuring a community that may be anywhere on Earth).
	ProximityNone ProximitySemantics = iota

	// ProximityHint biases ranking toward Coord but does not exclude
	// out-of-radius results. Use when the user may legitimately type a
	// globally-known place ("1 Infinite Loop", "Bondi Beach") and we
	// want local results boosted but not exclusive.
	ProximityHint

	// ProximityBound restricts results to within RadiusM of Coord (with
	// a small expansion factor to absorb provider looseness). Use when
	// an out-of-radius result would be actively wrong UX — e.g. an AI
	// extracted a place name from a user's local description, or the
	// user is typing into a map viewport.
	ProximityBound
)

// ProximityRequest expresses how a reference coordinate should influence
// a SearchPlaces call. The zero value disables proximity entirely.
type ProximityRequest struct {
	// Coord is the reference point. Must be non-nil for Hint and Bound;
	// ignored for None.
	Coord *models.Geolocation

	// RadiusM is the requested radius in meters. Ignored for None and
	// Hint. For Bound, 0 means "use the provider's default" (currently
	// DefaultBoundRadiusM).
	RadiusM int

	// Semantics declares how Coord and RadiusM should be applied.
	// Defaults to ProximityNone (the zero value).
	Semantics ProximitySemantics
}

// HasCoord reports whether r carries a reference coordinate that should
// influence the search. Returns false for ProximityNone regardless of
// whether Coord is set.
func (r ProximityRequest) HasCoord() bool {
	return r.Coord != nil && r.Semantics != ProximityNone
}

// ProximityRequestFromSource builds a ProximityRequest with semantics
// chosen by the proximity-source label produced by the AI-flow proximity
// ladder. The mapping reflects how much we trust each source:
//
//   - "event_coords" → Bound. Coordinates came from structured event
//     metadata (e.g. JSON-LD on the source webpage); the event's
//     reference point is high-confidence, and a match outside its
//     radius is almost certainly wrong.
//   - "gps" → Hint. Client device GPS. The user may legitimately name
//     a distant POI ("Zilker Park"); soft bias only.
//   - "location_id" → Hint. User's community / saved location used as
//     a last-resort fallback. Same rationale as gps.
//   - anything else (including "none" or empty) → None.
//
// When coord is nil the result is always ProximityNone regardless of
// source — there's nothing to bias against.
func ProximityRequestFromSource(coord *models.Geolocation, source string) ProximityRequest {
	if coord == nil {
		return ProximityRequest{Semantics: ProximityNone}
	}
	switch source {
	case "event_coords":
		return ProximityRequest{Coord: coord, Semantics: ProximityBound}
	case "gps", "location_id":
		return ProximityRequest{Coord: coord, Semantics: ProximityHint}
	default:
		return ProximityRequest{Semantics: ProximityNone}
	}
}

// DefaultBoundRadiusM is the radius applied to a ProximityBound request
// when the caller doesn't specify one. 50 km is generous enough to cover
// a typical community-scale area without admitting cross-state results;
// implementations expand by BoundRadiusExpansionFactor for the actual
// post-filter cutoff.
const DefaultBoundRadiusM = 50_000

// BoundRadiusExpansionFactor is the multiplier applied to the requested
// radius when post-filtering ProximityBound results. Provider proximity
// hints are imprecise (Google's locationRestriction is loose enough to
// return cross-state results at the boundary); expanding by 1.5× catches
// the wrong-state cases while keeping legitimate edge-of-radius results.
const BoundRadiusExpansionFactor = 1.5

// Compile-time assertions that the bundled provider implementations
// satisfy Provider. Add new implementations to this list as they land.
var (
	_ Provider = (*MapboxClient)(nil)
	_ Provider = (*GoogleMapsClient)(nil)
)
