package request

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
)

// maxImageCandidates caps how many alternate URLs ride on MediaReady.
// Matches the client's 4-up Replace Media row capacity.
const maxImageCandidates = 4

// fetchStockCandidates returns a Pexels image-alternate slice for the
// keyword query, suitable for emission on MediaReady. Best-effort:
// any provider failure logs and returns nil. Request has no stock
// video provider — only image alternates.
func (s *Service) fetchStockCandidates(ctx context.Context, keywords []string, limit int, logger *logging.Logger) []*api.MediaCandidate {
	if limit <= 0 || len(keywords) == 0 || s.stockImageryProvider == nil {
		return nil
	}
	query := strings.Join(keywords, " ")
	stockCands, err := s.stockImageryProvider.SearchStockImageCandidates(ctx, query, limit)
	if err != nil {
		logger.WarnContext(ctx, "stock image candidate search failed", "error", err)
		return nil
	}
	return mediacand.StockImageCandidatesToAPI(stockCands)
}

// fetchStockImage wraps findAndStoreStockImage for the fan-out callback.
// Returns an empty string on any failure — errors are logged in-place and
// non-fatal (image is optional for a request preview).
func (s *Service) fetchStockImage(ctx context.Context, keywords []string) string {
	if s.stockImageryProvider == nil {
		return ""
	}
	mediaID, err := s.findAndStoreStockImage(ctx, keywords)
	if err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"failed to get stock image",
			"operation", "GenRequest",
			"error", err)
		return ""
	}
	return mediaID
}

// geocodePlaceQuery runs a place search via the location provider and
// builds a GeocodedLocation from the top result. Returns nil on error or
// no-result — errors are logged in-place and non-fatal.
//
// Callers select proximity semantics via [location.ProximityRequestFromSource]
// off the proximity-source label produced by [fanout.ProximityForGeocoding]:
// GPS / location-id sources use Hint (don't drop named distant POIs the
// user explicitly typed); the request flow has no event-coords source.
func (s *Service) geocodePlaceQuery(ctx context.Context, query string, proximity location.ProximityRequest) *api.GeocodedLocation {
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

	best := places[0]
	logger.DebugContext(ctx, "auto-selected location",
		"name", best.Name, "confidence", best.Confidence)
	return best.ToGeocodedLocation(best.Name)
}

// runStreamingFanout drives streaming AI generation through the streaming
// fan-out for GenRequest: opens the streaming AI call and fires Mapbox +
// stock-image concurrently as their respective trigger keys close in the AI
// tool output (Mapbox on location_query, Pexels on search_keywords). The
// branch closures interpret the values themselves so the empty-keywords /
// USER_PRIMARY_LOCATION cases are handled inside.
func (s *Service) runStreamingFanout(
	ctx context.Context,
	logger *logging.Logger,
	prompt string,
	region string,
	proximity location.ProximityRequest,
) (*ai.RequestGeneration, fanout.StreamingResults, error) {
	streamFields, streamFinal, err := s.aiProvider.GenerateRequestContentStreaming(ctx, prompt, region)
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
			return s.geocodePlaceQuery(ctx, locQuery, proximity)
		},
	}
	if s.stockImageryProvider != nil {
		branches.StockMediaTriggerKey = ai.StreamFieldSearchKeywords
		branches.StockMediaFn = func(ctx context.Context, value json.RawMessage) string {
			var keywords []string
			if err := json.Unmarshal(value, &keywords); err != nil || len(keywords) == 0 {
				return ""
			}
			return s.fetchStockImage(ctx, keywords)
		}
	}

	streamStart := time.Now()
	result := fanout.StreamingRun(ctx, streamFields, branches)
	streamDuration := time.Since(streamStart)

	final := <-streamFinal
	if final.Err != nil {
		return nil, fanout.StreamingResults{}, final.Err
	}

	fanout.LogStreamingCompleted(logger, "GenRequest", streamStart, streamDuration, branches, result)
	return final.Result, result, nil
}
