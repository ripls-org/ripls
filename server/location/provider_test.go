package location

import (
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestProximityRequestFromSource covers the AI-flow source-label → semantics
// mapping. Event coords are high-confidence (Bound); GPS / location-id
// fallbacks are lower-confidence (Hint); anything else returns None.
func TestProximityRequestFromSource(t *testing.T) {
	coord := &models.Geolocation{LatitudeDeg: 30.27, LongitudeDeg: -97.74}

	tests := []struct {
		name      string
		coord     *models.Geolocation
		source    string
		wantSem   ProximitySemantics
		wantCoord bool
	}{
		{
			name:      "event_coords -> Bound",
			coord:     coord,
			source:    "event_coords",
			wantSem:   ProximityBound,
			wantCoord: true,
		},
		{
			name:      "gps -> Hint",
			coord:     coord,
			source:    "gps",
			wantSem:   ProximityHint,
			wantCoord: true,
		},
		{
			name:      "location_id -> Hint",
			coord:     coord,
			source:    "location_id",
			wantSem:   ProximityHint,
			wantCoord: true,
		},
		{
			name:      "none -> None",
			coord:     coord,
			source:    "none",
			wantSem:   ProximityNone,
			wantCoord: false,
		},
		{
			name:      "empty source -> None",
			coord:     coord,
			source:    "",
			wantSem:   ProximityNone,
			wantCoord: false,
		},
		{
			name:      "unknown source -> None",
			coord:     coord,
			source:    "made_up",
			wantSem:   ProximityNone,
			wantCoord: false,
		},
		{
			name:      "nil coord -> None regardless of source",
			coord:     nil,
			source:    "event_coords",
			wantSem:   ProximityNone,
			wantCoord: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ProximityRequestFromSource(tc.coord, tc.source)
			if got.Semantics != tc.wantSem {
				t.Errorf("Semantics = %v, want %v", got.Semantics, tc.wantSem)
			}
			gotHasCoord := got.Coord != nil
			if gotHasCoord != tc.wantCoord {
				t.Errorf("Coord set = %v, want %v", gotHasCoord, tc.wantCoord)
			}
			if got.HasCoord() != (tc.wantCoord && tc.wantSem != ProximityNone) {
				t.Errorf("HasCoord() = %v, expected truth from semantics+coord", got.HasCoord())
			}
		})
	}
}
