package storage

import (
	"math"
	"testing"
)

// TestHaversineDistance tests the Haversine distance calculation.
func TestHaversineDistance(t *testing.T) {
	tests := []struct {
		name         string
		lat1, lon1   float64
		lat2, lon2   float64
		expectedDist float64
		tolerance    float64 // Allow some tolerance for floating point
	}{
		{
			name:         "Same point",
			lat1:         30.2672,
			lon1:         -97.7431,
			lat2:         30.2672,
			lon2:         -97.7431,
			expectedDist: 0,
			tolerance:    1, // 1 meter tolerance
		},
		{
			name:         "Austin to Dallas (approximately 300km)",
			lat1:         30.2672, // Austin
			lon1:         -97.7431,
			lat2:         32.7767, // Dallas
			lon2:         -96.7970,
			expectedDist: 300000, // ~300km
			tolerance:    10000,  // 10km tolerance (Haversine is approximate)
		},
		{
			name:         "Small distance (approximately 1km)",
			lat1:         30.2672,
			lon1:         -97.7431,
			lat2:         30.2762, // ~1km north
			lon2:         -97.7431,
			expectedDist: 1000,
			tolerance:    100, // 100m tolerance
		},
		{
			name:         "Cross equator (Singapore to Sydney)",
			lat1:         1.3521, // Singapore
			lon1:         103.8198,
			lat2:         -33.8688, // Sydney
			lon2:         151.2093,
			expectedDist: 6300000, // ~6300km
			tolerance:    100000,  // 100km tolerance
		},
		{
			name:         "Cross prime meridian (London to Paris)",
			lat1:         51.5074, // London
			lon1:         -0.1278,
			lat2:         48.8566, // Paris
			lon2:         2.3522,
			expectedDist: 344000, // ~344km
			tolerance:    5000,   // 5km tolerance
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			distance := HaversineDistance(tt.lat1, tt.lon1, tt.lat2, tt.lon2)

			// Check if distance is within tolerance
			diff := math.Abs(distance - tt.expectedDist)
			if diff > tt.tolerance {
				t.Errorf("HaversineDistance(%f, %f, %f, %f) = %f meters, want %f ± %f meters (diff: %f)",
					tt.lat1, tt.lon1, tt.lat2, tt.lon2, distance, tt.expectedDist, tt.tolerance, diff)
			}

			// Distance should always be non-negative
			if distance < 0 {
				t.Errorf("HaversineDistance returned negative distance: %f", distance)
			}

			// Distance should be symmetric
			reverseDistance := HaversineDistance(tt.lat2, tt.lon2, tt.lat1, tt.lon1)
			if math.Abs(distance-reverseDistance) > 0.001 {
				t.Errorf("Distance is not symmetric: forward=%f, reverse=%f", distance, reverseDistance)
			}
		})
	}
}

// TestCalculateBoundingBox tests the bounding box calculation.
func TestCalculateBoundingBox(t *testing.T) {
	tests := []struct {
		name         string
		lat, lon     float64
		radiusMeters float64
		checkFunc    func(t *testing.T, bbox BoundingBox)
	}{
		{
			name:         "Small radius at equator",
			lat:          0,
			lon:          0,
			radiusMeters: 1000, // 1km
			checkFunc: func(t *testing.T, bbox BoundingBox) {
				// At equator, 1km is roughly 0.009 degrees
				// Verify the bounds are approximately correct
				latRange := bbox.MaxLatDeg - bbox.MinLatDeg
				lonRange := bbox.MaxLonDeg - bbox.MinLonDeg

				if latRange < 0.015 || latRange > 0.020 {
					t.Errorf("Latitude range unexpected: %f degrees (expected ~0.018)", latRange)
				}
				if lonRange < 0.015 || lonRange > 0.020 {
					t.Errorf("Longitude range unexpected: %f degrees (expected ~0.018)", lonRange)
				}
			},
		},
		{
			name:         "Large radius",
			lat:          30.2672,
			lon:          -97.7431,
			radiusMeters: 100000, // 100km
			checkFunc: func(t *testing.T, bbox BoundingBox) {
				// 100km should be roughly 0.9 degrees latitude
				latRange := bbox.MaxLatDeg - bbox.MinLatDeg
				if latRange < 1.5 || latRange > 2.5 {
					t.Errorf("Latitude range unexpected: %f degrees", latRange)
				}

				// Should contain the center point
				if bbox.MinLatDeg > 30.2672 || bbox.MaxLatDeg < 30.2672 {
					t.Errorf("Bounding box does not contain center latitude")
				}
				if bbox.MinLonDeg > -97.7431 || bbox.MaxLonDeg < -97.7431 {
					t.Errorf("Bounding box does not contain center longitude")
				}
			},
		},
		{
			name:         "Near north pole",
			lat:          85,
			lon:          0,
			radiusMeters: 10000, // 10km
			checkFunc: func(t *testing.T, bbox BoundingBox) {
				// At high latitudes, longitude degrees span shorter distances
				// so longitude range should be much larger than latitude range
				latRange := bbox.MaxLatDeg - bbox.MinLatDeg
				lonRange := bbox.MaxLonDeg - bbox.MinLonDeg

				if lonRange < latRange {
					t.Errorf("Expected longitude range (%f) > latitude range (%f) near pole",
						lonRange, latRange)
				}

				// Max latitude should be clamped at 90
				if bbox.MaxLatDeg > 90 {
					t.Errorf("Max latitude exceeds 90: %f", bbox.MaxLatDeg)
				}
			},
		},
		{
			name:         "Near south pole",
			lat:          -85,
			lon:          0,
			radiusMeters: 10000, // 10km
			checkFunc: func(t *testing.T, bbox BoundingBox) {
				// Min latitude should be clamped at -90
				if bbox.MinLatDeg < -90 {
					t.Errorf("Min latitude below -90: %f", bbox.MinLatDeg)
				}
			},
		},
		{
			name:         "Cross antimeridian (near 180/-180)",
			lat:          0,
			lon:          179,
			radiusMeters: 200000, // 200km (should cross 180/-180 line)
			checkFunc: func(t *testing.T, bbox BoundingBox) {
				// For simplicity, implementation clamps at ±180
				// Verify clamping works
				if bbox.MaxLonDeg > 180 {
					t.Errorf("Max longitude exceeds 180: %f", bbox.MaxLonDeg)
				}
				if bbox.MinLonDeg < -180 {
					t.Errorf("Min longitude below -180: %f", bbox.MinLonDeg)
				}
			},
		},
		{
			name:         "Zero radius",
			lat:          30.2672,
			lon:          -97.7431,
			radiusMeters: 0,
			checkFunc: func(t *testing.T, bbox BoundingBox) {
				// Zero radius should give a degenerate box at the point
				if bbox.MinLatDeg != bbox.MaxLatDeg {
					t.Errorf("Expected degenerate box, got lat range: [%f, %f]",
						bbox.MinLatDeg, bbox.MaxLatDeg)
				}
				if bbox.MinLonDeg != bbox.MaxLonDeg {
					t.Errorf("Expected degenerate box, got lon range: [%f, %f]",
						bbox.MinLonDeg, bbox.MaxLonDeg)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bbox := CalculateBoundingBox(tt.lat, tt.lon, tt.radiusMeters)

			// Basic sanity checks
			if bbox.MinLatDeg > bbox.MaxLatDeg {
				t.Errorf("MinLatDeg (%f) > MaxLatDeg (%f)", bbox.MinLatDeg, bbox.MaxLatDeg)
			}
			if bbox.MinLonDeg > bbox.MaxLonDeg {
				t.Errorf("MinLonDeg (%f) > MaxLonDeg (%f)", bbox.MinLonDeg, bbox.MaxLonDeg)
			}

			// Latitude should be within valid range
			if bbox.MinLatDeg < -90 || bbox.MinLatDeg > 90 {
				t.Errorf("MinLatDeg out of range: %f", bbox.MinLatDeg)
			}
			if bbox.MaxLatDeg < -90 || bbox.MaxLatDeg > 90 {
				t.Errorf("MaxLatDeg out of range: %f", bbox.MaxLatDeg)
			}

			// Longitude should be within valid range
			if bbox.MinLonDeg < -180 || bbox.MinLonDeg > 180 {
				t.Errorf("MinLonDeg out of range: %f", bbox.MinLonDeg)
			}
			if bbox.MaxLonDeg < -180 || bbox.MaxLonDeg > 180 {
				t.Errorf("MaxLonDeg out of range: %f", bbox.MaxLonDeg)
			}

			// Run test-specific checks
			tt.checkFunc(t, bbox)
		})
	}
}

// TestBoundingBoxContainment verifies that points within the radius are inside the bounding box.
func TestBoundingBoxContainment(t *testing.T) {
	centerLat := 30.2672
	centerLon := -97.7431
	radiusMeters := 10000.0 // 10km

	bbox := CalculateBoundingBox(centerLat, centerLon, radiusMeters)

	// Test points at cardinal directions approximately 5km away (well within radius)
	testPoints := []struct {
		name string
		lat  float64
		lon  float64
	}{
		{"North", centerLat + 0.045, centerLon},           // ~5km north
		{"South", centerLat - 0.045, centerLon},           // ~5km south
		{"East", centerLat, centerLon + 0.045},            // ~5km east
		{"West", centerLat, centerLon - 0.045},            // ~5km west
		{"Center", centerLat, centerLon},                  // center point
		{"Northeast", centerLat + 0.03, centerLon + 0.03}, // ~4km northeast
	}

	for _, tp := range testPoints {
		t.Run(tp.name, func(t *testing.T) {
			// Calculate actual distance
			distance := HaversineDistance(centerLat, centerLon, tp.lat, tp.lon)

			// If point is within radius, it must be within bounding box
			if distance <= radiusMeters {
				if tp.lat < bbox.MinLatDeg || tp.lat > bbox.MaxLatDeg {
					t.Errorf("Point %s at distance %f meters is within radius but outside bounding box latitude [%f, %f]",
						tp.name, distance, bbox.MinLatDeg, bbox.MaxLatDeg)
				}
				if tp.lon < bbox.MinLonDeg || tp.lon > bbox.MaxLonDeg {
					t.Errorf("Point %s at distance %f meters is within radius but outside bounding box longitude [%f, %f]",
						tp.name, distance, bbox.MinLonDeg, bbox.MaxLonDeg)
				}
			}
		})
	}
}

// TestEarthRadiusConstant verifies the Earth radius constant is reasonable.
func TestEarthRadiusConstant(t *testing.T) {
	// Mean Earth radius is approximately 6,371 km
	expectedRadius := 6371000.0 // meters
	tolerance := 10000.0        // 10km tolerance

	if math.Abs(EarthRadiusMeters-expectedRadius) > tolerance {
		t.Errorf("EarthRadiusMeters = %f, want %f ± %f",
			EarthRadiusMeters, expectedRadius, tolerance)
	}
}
