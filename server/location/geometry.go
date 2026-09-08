package location

import "math"

// haversineMeters returns the great-circle distance between two points
// on Earth in meters using the haversine formula. The formula treats
// Earth as a perfect sphere; the resulting distance is accurate to within
// 0.5% for the radii we care about (kilometers, not millimeters).
func haversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusM = 6_371_000.0
	toRad := func(deg float64) float64 { return deg * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLng := toRad(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusM * c
}

// filterByProximityBound drops places outside the post-filter cutoff for
// a ProximityBound request. The cutoff is RadiusM expanded by
// BoundRadiusExpansionFactor to absorb provider imprecision; when RadiusM
// is 0, DefaultBoundRadiusM is used. Returns the original slice unchanged
// for any other semantics or when Coord is nil.
func filterByProximityBound(places []PlaceResult, req ProximityRequest) []PlaceResult {
	if req.Semantics != ProximityBound || req.Coord == nil {
		return places
	}
	radiusM := req.RadiusM
	if radiusM <= 0 {
		radiusM = DefaultBoundRadiusM
	}
	cutoff := float64(radiusM) * BoundRadiusExpansionFactor
	kept := places[:0]
	for _, p := range places {
		d := haversineMeters(req.Coord.LatitudeDeg, req.Coord.LongitudeDeg,
			p.Coordinates.Latitude, p.Coordinates.Longitude)
		if d <= cutoff {
			kept = append(kept, p)
		}
	}
	return kept
}
