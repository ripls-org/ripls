package experience

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/fanout"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/genai/mediacand"
	"go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/logging"
	mediapkg "go.ripls.org/ripls/server/media"
)

// fetchStockCandidates returns a mixed slice of video + image candidate
// alternates for the keyword query, suitable for emission on MediaReady
// as one-tap replacement targets. Tries the video provider first
// (matches the chosen-media preference order in fetchStockMediaFallback),
// then fills remaining slots from the image provider. Best-effort: any
// failure is logged in-place and silently degrades.
func (s *Service) fetchStockCandidates(ctx context.Context, keywords []string, limit int, logger *logging.Logger) []*api.MediaCandidate {
	if limit <= 0 || len(keywords) == 0 {
		return nil
	}
	query := strings.Join(keywords, " ")
	var combined []mediapkg.StockImageCandidate

	if s.stockVideoProvider != nil {
		videoCands, err := s.stockVideoProvider.SearchStockVideoCandidates(ctx, query, limit)
		if err != nil {
			logger.WarnContext(ctx, "stock video candidate search failed", "error", err)
		} else if len(videoCands) > 0 {
			combined = append(combined, videoCands...)
		}
	}

	if len(combined) < limit && s.stockImageryProvider != nil {
		imageCands, err := s.stockImageryProvider.SearchStockImageCandidates(ctx, query, limit-len(combined))
		if err != nil {
			logger.WarnContext(ctx, "stock image candidate search failed", "error", err)
		} else if len(imageCands) > 0 {
			combined = append(combined, imageCands...)
		}
	}

	if len(combined) > limit {
		combined = combined[:limit]
	}
	return mediacand.StockImageCandidatesToAPI(combined)
}

// fetchStockMediaFallback tries stock video first (more immersive), falling
// back to stock image. Returns an empty string on any failure or when neither
// provider is configured — errors are logged in-place and non-fatal.
func (s *Service) fetchStockMediaFallback(ctx context.Context, keywords []string, userID string) string {
	logger := logging.LoggerWithContext(ctx)

	if s.stockVideoProvider != nil {
		videoMediaID, err := s.findAndStoreStockVideo(ctx, keywords, userID)
		if err != nil {
			logger.WarnContext(ctx, "failed to get stock video, falling back to image",
				"operation", "GenExperience",
				"error", err)
		} else if videoMediaID != "" {
			return videoMediaID
		}
	}

	if s.stockImageryProvider != nil {
		stockMediaID, err := s.findAndStoreStockImage(ctx, keywords, userID)
		if err != nil {
			logger.WarnContext(ctx, "failed to get stock image",
				"operation", "GenExperience",
				"error", err)
		} else if stockMediaID != "" {
			return stockMediaID
		}
	}

	return ""
}

// geocodePlaceQuery runs a place search via the location provider for the
// given query with the supplied proximity request, and builds a
// GeocodedLocation from the top match. Returns nil on error or no-result
// — errors are logged in-place and non-fatal (the caller preserves the
// raw LocationQuery so the client can still show text even when geocoding
// fails).
//
// Callers select proximity semantics via [location.ProximityRequestFromSource]
// off the proximity-source label produced by [fanout.ProximityForGeocoding]:
// event-coords get Bound (high-confidence reference point), GPS or
// location-id fallbacks get Hint (don't drop named distant POIs the user
// explicitly typed). Bare empty / "none" sources flow through as ProximityNone.
//
// Experience-specific twist: when the provider returns an "Address"-type
// match, the place Name is just the street address (e.g. "3393 Airport
// Road") which is redundant with the address lines in the response.
// Prefer the JSON-LD place name from the source webpage when available;
// otherwise use FullAddress as the display name.
func (s *Service) geocodePlaceQuery(ctx context.Context, query string, proximity location.ProximityRequest, eventLocationName string) *api.GeocodedLocation {
	logger := logging.LoggerWithContext(ctx)

	places, err := s.locationProvider.SearchPlaces(ctx, query, proximity, 1)
	if err != nil {
		logger.WarnContext(ctx, "failed to geocode location",
			"location_query", query, "error", err)
		return nil
	}
	if len(places) == 0 {
		return nil
	}

	// Auto-select the best match (first result has highest confidence).
	best := places[0]
	logger.DebugContext(ctx, "auto-selected location",
		"name", best.Name, "type", best.Type, "confidence", best.Confidence)

	displayName := best.Name
	if best.Type == "Address" {
		if eventLocationName != "" {
			displayName = eventLocationName
		} else {
			displayName = best.FullAddress
		}
	}
	return best.ToGeocodedLocation(displayName)
}

// runStreamingFanout drives streaming text-mode AI generation through the
// streaming fan-out: opens the streaming AI call, fires Mapbox + Pexels
// concurrently as their respective JSON keys close in the streaming tool
// output (Mapbox on location_query, Pexels on search_keywords), then awaits
// the final ExperienceGeneration. The branch closures interpret the
// trigger-key values themselves so the special USER_PRIMARY_LOCATION marker
// and the empty-keywords case are handled inside the closure rather than via
// pre-flight conditionals (those would require knowing the AI output before
// streaming completes, defeating the early-fire win).
//
// Returns the final ExperienceGeneration alongside the joined branch results.
// On AI streaming error, returns the error — caller maps to a Connect status.
func (s *Service) runStreamingFanout(
	ctx context.Context,
	logger *logging.Logger,
	prompt string,
	region string,
	currentTime string,
	proximity location.ProximityRequest,
	skipStockMedia bool,
	userID string,
) (*ai.ExperienceGeneration, fanout.StreamingResults, error) {
	streamFields, streamFinal, err := s.aiProvider.GenerateExperienceFromTextStreaming(ctx, prompt, region, currentTime)
	if err != nil {
		return nil, fanout.StreamingResults{}, err
	}

	branches := fanout.StreamingBranches{
		MapboxTriggerKey: ai.StreamFieldLocationQuery,
		MapboxFn: func(ctx context.Context, value json.RawMessage) *api.GeocodedLocation {
			if s.locationProvider == nil {
				return nil
			}
			var locQuery string
			if err := json.Unmarshal(value, &locQuery); err != nil {
				return nil
			}
			if locQuery == "" || locQuery == ai.LocationQueryUserPrimary {
				return nil
			}
			// Text mode has no JSON-LD event location, so eventLocationName="".
			return s.geocodePlaceQuery(ctx, locQuery, proximity, "")
		},
	}
	if !skipStockMedia && (s.stockVideoProvider != nil || s.stockImageryProvider != nil) {
		branches.StockMediaTriggerKey = ai.StreamFieldSearchKeywords
		branches.StockMediaFn = func(ctx context.Context, value json.RawMessage) string {
			var keywords []string
			if err := json.Unmarshal(value, &keywords); err != nil || len(keywords) == 0 {
				return ""
			}
			return s.fetchStockMediaFallback(ctx, keywords, userID)
		}
	}

	streamStart := time.Now()
	result := fanout.StreamingRun(ctx, streamFields, branches)
	streamDuration := time.Since(streamStart)

	final := <-streamFinal
	if final.Err != nil {
		return nil, fanout.StreamingResults{}, final.Err
	}

	fanout.LogStreamingCompleted(logger, "GenExperience", streamStart, streamDuration, branches, result)
	return final.Result, result, nil
}
