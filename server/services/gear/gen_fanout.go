package gear

import (
	"context"
	"encoding/json"
	"time"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/fanout"
	"go.ripls.org/ripls/server/logging"
)

// fetchStockImage wraps findAndStoreStockImage for the fan-out callback.
// Gear uses the generated title as the query (unlike Experience/Request
// which use search_keywords). Returns an empty string on any failure —
// errors are logged in-place and non-fatal.
func (s *Service) fetchStockImage(ctx context.Context, query, userID string) string {
	if s.stockImageryProvider == nil {
		return ""
	}
	mediaID, err := s.findAndStoreStockImage(ctx, query, userID)
	if err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"failed to get stock image",
			"operation", "GenGear",
			"error", err)
		return ""
	}
	return mediaID
}

// runStreamingFanout drives streaming AI generation through the streaming
// fan-out for GenGear: opens the streaming AI call and fires the stock-image
// branch as its trigger key closes. Gear's stock-image trigger is "title"
// (Gear uses the generated title as the stock-image query, not a separate
// search_keywords field).
//
// Gear does not run a geocoding branch — gear has no location provider
// wired in. If a future change requires per-gear geocoding, wire the
// shared location.Provider through to gen_streaming and add a Mapbox*
// branch here.
func (s *Service) runStreamingFanout(
	ctx context.Context,
	logger *logging.Logger,
	prompt string,
	region string,
	userID string,
) (*ai.GearGeneration, fanout.StreamingResults, error) {
	streamFields, streamFinal, err := s.aiProvider.GenerateGearFromTextStreaming(ctx, prompt, region)
	if err != nil {
		return nil, fanout.StreamingResults{}, err
	}

	branches := fanout.StreamingBranches{}
	if s.stockImageryProvider != nil {
		branches.StockMediaTriggerKey = ai.StreamFieldTitle
		branches.StockMediaFn = func(ctx context.Context, value json.RawMessage) string {
			var title string
			if err := json.Unmarshal(value, &title); err != nil || title == "" {
				return ""
			}
			return s.fetchStockImage(ctx, title, userID)
		}
	}

	streamStart := time.Now()
	result := fanout.StreamingRun(ctx, streamFields, branches)
	streamDuration := time.Since(streamStart)

	final := <-streamFinal
	if final.Err != nil {
		return nil, fanout.StreamingResults{}, final.Err
	}

	fanout.LogStreamingCompleted(logger, "GenGear", streamStart, streamDuration, branches, result)
	return final.Result, result, nil
}
