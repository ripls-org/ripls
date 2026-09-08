package storage

import (
	"math"
)

// EarthRadiusMeters is the mean radius of Earth in meters.
const EarthRadiusMeters = 6371000.0

// HaversineDistance calculates the great circle distance in meters between two points
// on Earth specified by latitude and longitude in degrees using the Haversine formula.
func HaversineDistance(lat1Deg, lon1Deg, lat2Deg, lon2Deg float64) float64 {
	// Convert degrees to radians
	lat1 := lat1Deg * math.Pi / 180.0
	lon1 := lon1Deg * math.Pi / 180.0
	lat2 := lat2Deg * math.Pi / 180.0
	lon2 := lon2Deg * math.Pi / 180.0

	// Haversine formula
	dLat := lat2 - lat1
	dLon := lon2 - lon1

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return EarthRadiusMeters * c
}

// BoundingBox represents a rectangular geographic region.
type BoundingBox struct {
	MinLatDeg float64
	MaxLatDeg float64
	MinLonDeg float64
	MaxLonDeg float64
}

// CalculateBoundingBox computes a bounding box around a point with a given radius in meters.
// This provides a fast filter for spatial queries before applying the more expensive Haversine calculation.
func CalculateBoundingBox(latDeg, lonDeg, radiusMeters float64) BoundingBox {
	// Convert latitude to radians
	lat := latDeg * math.Pi / 180.0

	// Angular distance in radians on a great circle
	angularDistance := radiusMeters / EarthRadiusMeters

	// Calculate latitude bounds
	minLatDeg := latDeg - (angularDistance * 180.0 / math.Pi)
	maxLatDeg := latDeg + (angularDistance * 180.0 / math.Pi)

	// Calculate longitude bounds (adjust for latitude)
	// At higher latitudes, longitude degrees span shorter distances
	deltaLon := angularDistance * 180.0 / math.Pi / math.Cos(lat)
	minLonDeg := lonDeg - deltaLon
	maxLonDeg := lonDeg + deltaLon

	// Clamp latitude to valid range [-90, 90]
	if minLatDeg < -90.0 {
		minLatDeg = -90.0
	}
	if maxLatDeg > 90.0 {
		maxLatDeg = 90.0
	}

	// Handle longitude wrapping at ±180 degrees
	// For simplicity in SQL queries, we don't handle the wrap-around case
	// This means queries near the antimeridian may return fewer results
	if minLonDeg < -180.0 {
		minLonDeg = -180.0
	}
	if maxLonDeg > 180.0 {
		maxLonDeg = 180.0
	}

	return BoundingBox{
		MinLatDeg: minLatDeg,
		MaxLatDeg: maxLatDeg,
		MinLonDeg: minLonDeg,
		MaxLonDeg: maxLonDeg,
	}
}
