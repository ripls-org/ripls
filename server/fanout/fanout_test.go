package fanout

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestRun_ConcurrentExecution proves Run executes branches in parallel
// rather than serially — a serial implementation would exceed the deadline.
func TestRun_ConcurrentExecution(t *testing.T) {
	const (
		stockDelay  = 80 * time.Millisecond
		mapboxDelay = 60 * time.Millisecond
		// Headroom above max(stockDelay, mapboxDelay) = 80 ms; serial would take
		// 140 ms. 120 ms catches that regression with ~40 ms of buffer.
		deadline = 120 * time.Millisecond
	)

	var stockCalls, mapboxCalls atomic.Int32

	branches := Branches{
		StockMediaFn: func(_ context.Context) string {
			stockCalls.Add(1)
			time.Sleep(stockDelay) //nolint:forbidigo // fake branch simulates real-world provider latency
			return "stock-id"
		},
		MapboxFn: func(_ context.Context) *api.GeocodedLocation {
			mapboxCalls.Add(1)
			time.Sleep(mapboxDelay) //nolint:forbidigo // fake branch simulates real-world provider latency
			return &api.GeocodedLocation{Name: "Test Place"}
		},
	}

	start := time.Now()
	result := Run(context.Background(), branches)
	elapsed := time.Since(start)

	if elapsed > deadline {
		t.Errorf("fan-out ran serially: elapsed %v > deadline %v (sum would be %v, max should be ~%v)",
			elapsed, deadline, stockDelay+mapboxDelay, max(stockDelay, mapboxDelay))
	}
	if result.StockMediaID != "stock-id" {
		t.Errorf("StockMediaID = %q, want %q", result.StockMediaID, "stock-id")
	}
	if result.GeocodedLocation == nil || result.GeocodedLocation.Name != "Test Place" {
		t.Errorf("GeocodedLocation = %v, want {Name:\"Test Place\"}", result.GeocodedLocation)
	}
	if got := stockCalls.Load(); got != 1 {
		t.Errorf("StockMediaFn called %d times, want 1", got)
	}
	if got := mapboxCalls.Load(); got != 1 {
		t.Errorf("MapboxFn called %d times, want 1", got)
	}
}

func TestRun_NilBranches(t *testing.T) {
	tests := []struct {
		name         string
		branches     Branches
		wantStockMID string
		wantGeocoded bool
	}{
		{
			name:         "only-stock",
			branches:     Branches{StockMediaFn: func(context.Context) string { return "s" }},
			wantStockMID: "s",
		},
		{
			name:         "only-mapbox",
			branches:     Branches{MapboxFn: func(context.Context) *api.GeocodedLocation { return &api.GeocodedLocation{Name: "m"} }},
			wantGeocoded: true,
		},
		{
			name:     "neither",
			branches: Branches{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Run(context.Background(), tt.branches)
			if got.StockMediaID != tt.wantStockMID {
				t.Errorf("StockMediaID = %q, want %q", got.StockMediaID, tt.wantStockMID)
			}
			if (got.GeocodedLocation != nil) != tt.wantGeocoded {
				t.Errorf("GeocodedLocation present = %v, want %v", got.GeocodedLocation != nil, tt.wantGeocoded)
			}
		})
	}
}

func TestRun_ErrorPermutations(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name         string
		stockReturn  string
		mapboxReturn *api.GeocodedLocation
	}{
		{"both-ok", "stock-id", &api.GeocodedLocation{Name: "place"}},
		{"stock-fails", "", &api.GeocodedLocation{Name: "place"}},
		{"mapbox-fails", "stock-id", nil},
		{"both-fail", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			branches := Branches{
				StockMediaFn: func(context.Context) string { return tt.stockReturn },
				MapboxFn:     func(context.Context) *api.GeocodedLocation { return tt.mapboxReturn },
			}
			got := Run(ctx, branches)
			if got.StockMediaID != tt.stockReturn {
				t.Errorf("StockMediaID = %q, want %q", got.StockMediaID, tt.stockReturn)
			}
			if got.GeocodedLocation != tt.mapboxReturn {
				t.Errorf("GeocodedLocation = %v, want %v", got.GeocodedLocation, tt.mapboxReturn)
			}
		})
	}
}

func TestProximityForGeocoding(t *testing.T) {
	ptr := func(f float64) *float64 { return &f }
	tests := []struct {
		name     string
		eventLat *float64
		eventLng *float64
		gpsLat   float64
		gpsLng   float64
		defLoc   bool
		wantSrc  string
	}{
		{"event-wins", ptr(40.0), ptr(-105.0), 41.0, -106.0, true, "event_coords"},
		{"gps-when-no-event", nil, nil, 41.0, -106.0, true, "gps"},
		{"location_id-when-no-gps-no-event", nil, nil, 0, 0, true, "location_id"},
		{"none-when-nothing", nil, nil, 0, 0, false, "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var defLoc *models.Geolocation
			if tt.defLoc {
				defLoc = &models.Geolocation{LatitudeDeg: 42.0, LongitudeDeg: -107.0}
			}
			_, src := ProximityForGeocoding(tt.eventLat, tt.eventLng, tt.gpsLat, tt.gpsLng, defLoc)
			if src != tt.wantSrc {
				t.Errorf("proximitySource = %q, want %q", src, tt.wantSrc)
			}
		})
	}
}

// TestStreamingRun_FiresOnTriggerArrival proves the streaming fanout fires
// each branch the moment its trigger key arrives — not after the field
// channel closes. This is the property the server-side streaming wall-clock
// win depends on: Mapbox / Pexels start partway through the AI streaming
// call rather than after it.
func TestStreamingRun_FiresOnTriggerArrival(t *testing.T) {
	const (
		mapboxArrives = 50 * time.Millisecond
		stockArrives  = 100 * time.Millisecond
		streamCloses  = 250 * time.Millisecond
		branchWork    = 200 * time.Millisecond
	)

	field := make(chan ai.FieldEvent)
	go func() {
		defer close(field)
		time.Sleep(mapboxArrives) //nolint:forbidigo // drives a fake AI stream to test concurrent timing behaviour
		field <- ai.FieldEvent{Key: ai.StreamFieldLocationQuery, Value: json.RawMessage(`"Boulder"`)}
		time.Sleep(stockArrives - mapboxArrives) //nolint:forbidigo // drives a fake AI stream to test concurrent timing behaviour
		field <- ai.FieldEvent{Key: ai.StreamFieldSearchKeywords, Value: json.RawMessage(`["a","b"]`)}
		time.Sleep(streamCloses - stockArrives) //nolint:forbidigo // drives a fake AI stream to test concurrent timing behaviour
	}()

	branches := StreamingBranches{
		StockMediaTriggerKey: ai.StreamFieldSearchKeywords,
		StockMediaFn: func(_ context.Context, _ json.RawMessage) string {
			time.Sleep(branchWork) //nolint:forbidigo // fake branch simulates real-world provider latency
			return "stock-id"
		},
		MapboxTriggerKey: ai.StreamFieldLocationQuery,
		MapboxFn: func(_ context.Context, _ json.RawMessage) *api.GeocodedLocation {
			time.Sleep(branchWork) //nolint:forbidigo // fake branch simulates real-world provider latency
			return &api.GeocodedLocation{Name: "Boulder"}
		},
	}

	start := time.Now()
	result := StreamingRun(context.Background(), field, branches)
	total := time.Since(start)

	// Mapbox starts at t=50ms and runs 200ms → done at t=250ms.
	// Stock-media starts at t=100ms and runs 200ms → done at t=300ms.
	// Total ≈ 300ms (max(mapbox_done, stock_done, streamCloses)).
	// A serial-after-stream-close implementation would take 250 + 200 + 200 = 650ms.
	// Headroom of 80ms above 300ms catches the regression while tolerating noise.
	if total > 380*time.Millisecond {
		t.Errorf("StreamingRun took %v; expected ~300ms (proves trigger-on-arrival, not start-after-close)", total)
	}
	if result.StockMediaID != "stock-id" {
		t.Errorf("StockMediaID = %q, want stock-id", result.StockMediaID)
	}
	if result.GeocodedLocation == nil || result.GeocodedLocation.Name != "Boulder" {
		t.Errorf("GeocodedLocation = %+v, want Boulder", result.GeocodedLocation)
	}
	// Branch start offsets prove the early-fire property is observable in the
	// returned struct (used for prod logging in LogStreamingCompleted). Headroom
	// is generous (vs. the ~30ms used elsewhere in this test) because these
	// offsets measure goroutine start latency directly, which is more exposed
	// to scheduler jitter on a loaded CI runner than the aggregate `total`
	// check above.
	mapboxOffset := result.MapboxStartedAt.Sub(start)
	if mapboxOffset > 150*time.Millisecond {
		t.Errorf("Mapbox started at +%v from stream start; expected ~50ms", mapboxOffset)
	}
	stockOffset := result.StockMediaStartedAt.Sub(start)
	if stockOffset > 200*time.Millisecond {
		t.Errorf("StockMedia started at +%v from stream start; expected ~100ms", stockOffset)
	}
}

// TestStreamingRun_SkipsWhenTriggerNeverArrives verifies that branches whose
// trigger key never appears in the field stream simply do not fire — same
// "skip the branch" semantics as Run when a closure is nil.
func TestStreamingRun_SkipsWhenTriggerNeverArrives(t *testing.T) {
	var stockCalls, mapboxCalls atomic.Int32

	field := make(chan ai.FieldEvent, 1)
	field <- ai.FieldEvent{Key: ai.StreamFieldTitle, Value: json.RawMessage(`"hello"`)}
	close(field)

	branches := StreamingBranches{
		StockMediaTriggerKey: ai.StreamFieldSearchKeywords,
		StockMediaFn: func(_ context.Context, _ json.RawMessage) string {
			stockCalls.Add(1)
			return "x"
		},
		MapboxTriggerKey: ai.StreamFieldLocationQuery,
		MapboxFn: func(_ context.Context, _ json.RawMessage) *api.GeocodedLocation {
			mapboxCalls.Add(1)
			return &api.GeocodedLocation{}
		},
	}

	result := StreamingRun(context.Background(), field, branches)

	if stockCalls.Load() != 0 {
		t.Errorf("StockMediaFn called %d times; expected 0 (search_keywords never arrived)", stockCalls.Load())
	}
	if mapboxCalls.Load() != 0 {
		t.Errorf("MapboxFn called %d times; expected 0 (location_query never arrived)", mapboxCalls.Load())
	}
	if result.StockMediaID != "" || result.GeocodedLocation != nil {
		t.Errorf("expected empty results when no triggers arrived, got %+v", result)
	}
	if !result.StockMediaStartedAt.IsZero() || !result.MapboxStartedAt.IsZero() {
		t.Errorf("expected zero start timestamps when no triggers arrived")
	}
}

// TestStreamingRun_NilBranchesAndKeysSkip verifies the nil-closure and empty-
// key cases both disable the branch entirely.
func TestStreamingRun_NilBranchesAndKeysSkip(t *testing.T) {
	field := make(chan ai.FieldEvent, 2)
	field <- ai.FieldEvent{Key: ai.StreamFieldSearchKeywords, Value: json.RawMessage(`["a"]`)}
	field <- ai.FieldEvent{Key: ai.StreamFieldLocationQuery, Value: json.RawMessage(`"Boulder"`)}
	close(field)

	// All branches disabled (one via empty key, one via nil closure).
	branches := StreamingBranches{
		StockMediaTriggerKey: "", // disabled via empty key
		StockMediaFn: func(_ context.Context, _ json.RawMessage) string {
			t.Fatal("StockMediaFn should not have been called when key is empty")
			return ""
		},
		MapboxTriggerKey: ai.StreamFieldLocationQuery,
		MapboxFn:         nil,
	}

	result := StreamingRun(context.Background(), field, branches)

	if result.StockMediaID != "" || result.GeocodedLocation != nil {
		t.Errorf("expected empty results when all branches disabled, got %+v", result)
	}
}

// TestStreamingRun_TriggerValuePassedToClosure verifies the closure receives
// the JSON bytes of the trigger value — needed for the closure to do its
// own filtering (e.g., skip Mapbox on "USER_PRIMARY_LOCATION").
func TestStreamingRun_TriggerValuePassedToClosure(t *testing.T) {
	field := make(chan ai.FieldEvent, 1)
	field <- ai.FieldEvent{Key: ai.StreamFieldLocationQuery, Value: json.RawMessage(`"USER_PRIMARY_LOCATION"`)}
	close(field)

	var captured json.RawMessage
	branches := StreamingBranches{
		MapboxTriggerKey: ai.StreamFieldLocationQuery,
		MapboxFn: func(_ context.Context, value json.RawMessage) *api.GeocodedLocation {
			captured = value
			return nil
		},
	}

	result := StreamingRun(context.Background(), field, branches)

	if string(captured) != `"USER_PRIMARY_LOCATION"` {
		t.Errorf("captured trigger value = %q, want %q", string(captured), `"USER_PRIMARY_LOCATION"`)
	}
	if result.GeocodedLocation != nil {
		t.Errorf("expected nil GeocodedLocation when closure returns nil")
	}
	// MapboxStartedAt is set even though the closure returned nil — proves
	// the closure was invoked and the timestamp is meaningful for logging.
	if result.MapboxStartedAt.IsZero() {
		t.Errorf("expected MapboxStartedAt to be set when closure ran")
	}
}
