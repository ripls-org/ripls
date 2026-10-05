package feed

import (
	"context"
	"fmt"
	"time"

	"go.ripls.org/ripls/server/logging"
	mediapkg "go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/storage"
)

// Nudge imagery and shared helpers.
//
// Generic per-user nudge generation lived here until #2936: it queried
// personalization context, asked an LLM for a batch of headline/description/CTA
// triples, persisted them, and fetched a stock image for each. That surface is
// gone — the Home affordances are now unconditional and hand-written, and the
// only cards still using this file's imagery path are the terminator and the
// momentum engine's host prompts.

const (
	// nudgeImageryCap is the process-wide limit on concurrent fetchNudgeImagery
	// calls. It bounds memory and external-API burst across all overlapping feed
	// loads, not just within a single batch.
	nudgeImageryCap = 4
)

// nudgeImagerySem is a process-wide semaphore that caps concurrent
// fetchNudgeImagery goroutines at nudgeImageryCap. Acquired at the start of
// fetchNudgeImagery and released on exit via defer.
var nudgeImagerySem = make(chan struct{}, nudgeImageryCap)

// timeNowUnix returns the current time as a Unix timestamp in seconds.
// Defined as a variable so tests can override it.
var timeNowUnix = func() int64 {
	return time.Now().Unix()
}

// unixSecToDayOfYear converts a Unix timestamp to the day-of-year (1–366).
func unixSecToDayOfYear(unixSec int64) int {
	return time.Unix(unixSec, 0).UTC().YearDay()
}

// fetchNudgeImagery fetches a stock image for a nudge and updates its media_id.
// Called asynchronously; failures leave the nudge invisible (no media_id).
// Concurrency is bounded by nudgeImagerySem to prevent overlapping feed loads
// from exhausting memory or external-API connections.
//
// The copy is owned by media.SystemUserID, not by whoever's feed load triggered
// it. Terminator nudges are global — the same card is served to everyone — so a
// copy owned by one user is unreadable to every other viewer, who gets
// PermissionDenied from GetMedia and a card with a broken image (#3105).
// System-owned stock is public by design, which is what a shared card needs.
func (s *Service) fetchNudgeImagery(ctx context.Context, nudgeID, query string) {
	if s.stockImageryProvider == nil {
		return
	}
	nudgeImagerySem <- struct{}{}
	defer func() { <-nudgeImagerySem }()

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "FetchNudgeImagery",
		"nudge_id", nudgeID,
		"query", query,
	)
	startTime := time.Now()

	stockImage, err := s.stockImageryProvider.GetStockImage(ctx, query, nil)
	if err != nil {
		logger.WarnContext(ctx, "stock image fetch failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return
	}

	mediaID, err := storage.CopyStockImageForUser(
		ctx,
		s.sqlStorage,
		s.bucketStorage,
		stockImage,
		mediapkg.SystemUserID,
		fmt.Sprintf("nudge-%s.jpg", nudgeID),
		fmt.Sprintf("Stock image for nudge: %s", query),
	)
	if err != nil {
		logger.WarnContext(ctx, "failed to copy stock image for nudge", "error", err)
		return
	}

	if err := updateNudgeMediaID(ctx, s.sqlStorage, nudgeID, mediaID); err != nil {
		logger.WarnContext(ctx, "failed to update nudge media_id", "error", err)
		return
	}

	logger.InfoContext(ctx, "nudge imagery attached",
		"media_id", mediaID,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)
}

// SetStockImageryProvider sets the stock imagery provider for nudge background images.
func (s *Service) SetStockImageryProvider(provider mediapkg.StockImageryProvider) {
	s.stockImageryProvider = provider
}

// SetBucketStorage sets the bucket storage for copying stock images.
func (s *Service) SetBucketStorage(bucket storage.BucketStorage) {
	s.bucketStorage = bucket
}
