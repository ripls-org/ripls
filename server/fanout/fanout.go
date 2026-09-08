// Package fanout provides a shared concurrency primitive for the parallel
// post-AI work in Gen* RPC handlers (experience, gear, request). Each of
// those handlers runs two independent follow-up steps after the AI call —
// Mapbox geocoding and stock-media fetch — that together dominate the
// post-AI wall-clock when run serially (~300 ms + ~800 ms). Running them
// concurrently cuts the chain to max(branch) instead of sum(branch).
//
// Design note: this package stays a thin concurrency primitive. It does not
// depend on storage, the Mapbox client, or any stock-media provider. Each
// caller constructs its own per-branch closures (wrapping service-specific
// method calls with whatever logging / error handling is appropriate) and
// hands them to Run. This keeps the shared helper testable with trivial
// sleep closures and avoids pulling any service-level dependencies up into
// a shared package.
package fanout

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// Branches holds the optional per-branch closures for the parallel post-AI
// work. A nil closure means "skip this branch" (e.g., the user already
// uploaded their own media so we don't need a stock fallback, or the AI
// didn't extract a location so there's nothing to geocode).
type Branches struct {
	// StockMediaFn fetches a stock video or image and returns its stored
	// media ID, or "" on failure. The caller is responsible for logging
	// errors inside the closure — Run does not surface them.
	StockMediaFn func(ctx context.Context) string

	// MapboxFn geocodes a free-text location query and returns a populated
	// GeocodedLocation, or nil on failure. The caller is responsible for
	// logging errors inside the closure.
	MapboxFn func(ctx context.Context) *api.GeocodedLocation
}

// Results holds the outputs of the parallel fan-out. Either field may be
// empty/nil — callers treat those as "skip this field on the response"
// rather than as hard errors.
type Results struct {
	StockMediaID     string
	GeocodedLocation *api.GeocodedLocation
}

// Run runs the provided branches concurrently and returns once all have
// completed. Each branch is optional (nil = skip). No branch can cause
// another to abort — errors are captured inside the closure (via logs) and
// surfaced through empty returns, matching the "non-fatal" semantics Gen*
// handlers have always used for these particular side-steps.
func Run(ctx context.Context, branches Branches) Results {
	var out Results
	g := new(errgroup.Group)
	if branches.StockMediaFn != nil {
		g.Go(func() error {
			out.StockMediaID = branches.StockMediaFn(ctx)
			return nil
		})
	}
	if branches.MapboxFn != nil {
		g.Go(func() error {
			out.GeocodedLocation = branches.MapboxFn(ctx)
			return nil
		})
	}
	_ = g.Wait()
	return out
}

// LogCompleted emits the standard "post-AI fan-out completed" structured
// event with a consistent field set across Gen* handlers. Uses the caller's
// logger so its context attributes (user_id, prompt_length, etc.) carry
// through. ran_stock_media / ran_mapbox record whether each branch was
// actually scheduled; stock_media_hit / mapbox_hit record whether each
// branch produced a result. The two pairs let prod log queries distinguish
// "skipped" from "attempted but failed" without grepping branch preconditions.
func LogCompleted(logger *logging.Logger, operation string, duration time.Duration, ranStockMedia, ranMapbox bool, result Results) {
	logger.Info("post-AI fan-out completed",
		"operation", operation,
		"fanout_duration_ms", duration.Milliseconds(),
		"ran_stock_media", ranStockMedia,
		"ran_mapbox", ranMapbox,
		"stock_media_hit", result.StockMediaID != "",
		"mapbox_hit", result.GeocodedLocation != nil,
	)
}

// StreamingBranches is the streaming-aware analogue of Branches. Each branch
// fires the moment its trigger key's value closes in the AI streaming output
// — Mapbox can start ~250-350 ms into the AI call instead of after it
// closes, Pexels can start ~500-700 ms in. Closures receive the raw JSON
// value of the trigger key so they can decide their own behavior (e.g., skip
// Mapbox when location_query is "USER_PRIMARY_LOCATION").
//
// A nil closure or empty trigger key disables that branch entirely.
type StreamingBranches struct {
	// StockMediaTriggerKey is the JSON key whose value closes earliest and
	// suffices to fire stock-media fetch (e.g., StreamFieldSearchKeywords
	// for Experience/Request, StreamFieldTitle for Gear).
	StockMediaTriggerKey ai.StreamFieldKey
	StockMediaFn         func(ctx context.Context, triggerValue json.RawMessage) string

	// MapboxTriggerKey is the JSON key whose value triggers Mapbox geocoding
	// (always StreamFieldLocationQuery today).
	MapboxTriggerKey ai.StreamFieldKey
	MapboxFn         func(ctx context.Context, triggerValue json.RawMessage) *api.GeocodedLocation
}

// StreamingResults is the streaming-aware analogue of Results. Adds branch-
// start timestamps so handlers can record streaming-first-fire wins in their
// structured logs.
type StreamingResults struct {
	StockMediaID     string
	GeocodedLocation *api.GeocodedLocation

	// StockMediaStartedAt is the time the StockMediaFn goroutine began. Zero
	// if the branch never fired (no event with the trigger key arrived).
	StockMediaStartedAt time.Time
	// MapboxStartedAt is the time the MapboxFn goroutine began. Zero if the
	// branch never fired.
	MapboxStartedAt time.Time
}

// StreamingRun consumes field events from the AI provider's streaming
// channel, firing each configured branch the moment its trigger key arrives.
// Returns once both the field channel has closed AND all fired branches have
// completed. Branches whose trigger key never arrived simply do not run —
// matching the "skip the branch when there is no work" semantics of Run.
//
// The fanout stays a thin concurrency primitive — it does not interpret the
// trigger values, does not know about Mapbox or Pexels, does not log per-
// branch results. Callers wrap their own logging / decision-making inside the
// closures.
func StreamingRun(ctx context.Context, fieldEvents <-chan ai.FieldEvent, branches StreamingBranches) StreamingResults {
	var (
		mu  sync.Mutex
		out StreamingResults
	)
	wg := &sync.WaitGroup{}

	for ev := range fieldEvents {
		switch ev.Key {
		case branches.StockMediaTriggerKey:
			if branches.StockMediaTriggerKey == "" || branches.StockMediaFn == nil {
				continue
			}
			startedAt := time.Now()
			value := ev.Value
			wg.Add(1)
			logging.GoSafe(ctx, "fanout-stock-media", func() {
				defer wg.Done()
				result := branches.StockMediaFn(ctx, value)
				mu.Lock()
				out.StockMediaID = result
				out.StockMediaStartedAt = startedAt
				mu.Unlock()
			})

		case branches.MapboxTriggerKey:
			if branches.MapboxTriggerKey == "" || branches.MapboxFn == nil {
				continue
			}
			startedAt := time.Now()
			value := ev.Value
			wg.Add(1)
			logging.GoSafe(ctx, "fanout-mapbox", func() {
				defer wg.Done()
				result := branches.MapboxFn(ctx, value)
				mu.Lock()
				out.GeocodedLocation = result
				out.MapboxStartedAt = startedAt
				mu.Unlock()
			})
		}
	}

	wg.Wait()
	return out
}

// LogStreamingCompleted emits the streaming-aware analogue of LogCompleted.
// Records both the overall fan-out wall-clock and the per-branch start
// offsets relative to streamStart, which lets prod logs show how much of the
// branch run-time overlapped the AI streaming call (the actual perceived-
// latency win).
func LogStreamingCompleted(logger *logging.Logger, operation string, streamStart time.Time, fanoutDuration time.Duration, branches StreamingBranches, result StreamingResults) {
	fields := []any{
		"operation", operation,
		"fanout_duration_ms", fanoutDuration.Milliseconds(),
		"ran_stock_media", branches.StockMediaFn != nil,
		"ran_mapbox", branches.MapboxFn != nil,
		"stock_media_hit", result.StockMediaID != "",
		"mapbox_hit", result.GeocodedLocation != nil,
	}
	if !result.StockMediaStartedAt.IsZero() {
		fields = append(fields, "stock_media_started_offset_ms", result.StockMediaStartedAt.Sub(streamStart).Milliseconds())
	}
	if !result.MapboxStartedAt.IsZero() {
		fields = append(fields, "mapbox_started_offset_ms", result.MapboxStartedAt.Sub(streamStart).Milliseconds())
	}
	logger.Info("post-AI fan-out completed", fields...)
}

// ProximityForGeocoding picks the highest-confidence proximity bias available
// for a Mapbox geocode call. Ladder (highest-confidence first):
//  1. JSON-LD event coords (exact event location from structured webpage data)
//  2. Client-supplied device GPS
//  3. User's default location coords (from locationId)
//  4. nil (unbiased)
//
// Returns the chosen proximity and a short source label for structured
// logging (lets prod logs show which branch fired without re-deriving it).
func ProximityForGeocoding(eventLat, eventLng *float64, clientLat, clientLng float64, defaultLocation *models.Geolocation) (*models.Geolocation, string) {
	if eventLat != nil && eventLng != nil {
		return &models.Geolocation{LatitudeDeg: *eventLat, LongitudeDeg: *eventLng}, "event_coords"
	}
	if clientLat != 0 && clientLng != 0 {
		return &models.Geolocation{LatitudeDeg: clientLat, LongitudeDeg: clientLng}, "gps"
	}
	if defaultLocation != nil {
		return defaultLocation, "location_id"
	}
	return nil, "none"
}
