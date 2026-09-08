package activitystamp

import (
	"context"
	"net/http"
	"sync"
	"time"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/logging"
)

// stampMinInterval bounds writes to at most one per user per interval.
// 15 minutes keeps window-edge accuracy well inside what a daily ops
// digest needs while making the steady-state write cost negligible.
const stampMinInterval = 15 * time.Minute

// stampWriteTimeout bounds the async upsert so a wedged database can't
// accumulate goroutines.
const stampWriteTimeout = 10 * time.Second

// Recorder persists one observed activity stamp. Implemented by
// storage.ProtoSQLStorage.RecordUserActivity.
type Recorder interface {
	RecordUserActivity(ctx context.Context, userID string, atUnixSec int64) error
}

// Stamper is the middleware. Construct with New; the zero value is not
// usable.
type Stamper struct {
	recorder Recorder
	now      func() time.Time

	mu sync.Mutex
	// lastStamp remembers when each user was last stamped by this
	// process. Bounded by the distinct users a process serves between
	// deploys — a few bytes per daily-active user — so no eviction is
	// needed.
	lastStamp map[string]time.Time
}

// New returns a Stamper writing through recorder. now is used for
// interval checks and stamp timestamps; pass nil for time.Now.
func New(recorder Recorder, now func() time.Time) *Stamper {
	if now == nil {
		now = time.Now
	}
	return &Stamper{
		recorder:  recorder,
		now:       now,
		lastStamp: map[string]time.Time{},
	}
}

// Wrap returns a handler that stamps the authenticated user (if any)
// before delegating. It must sit inside the authn middleware so the
// auth info is already on the request context.
func (s *Stamper) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if info, ok := auth.GetAuthInfo(r.Context()); ok && info != nil && info.UserID != "" {
			s.stamp(r.Context(), info.UserID)
		}
		next.ServeHTTP(w, r)
	})
}

// stamp records activity for userID unless it was stamped within
// stampMinInterval. The write happens asynchronously on a fresh
// context (the request context ends at response time).
func (s *Stamper) stamp(ctx context.Context, userID string) {
	now := s.now()
	s.mu.Lock()
	if last, ok := s.lastStamp[userID]; ok && now.Sub(last) < stampMinInterval {
		s.mu.Unlock()
		return
	}
	s.lastStamp[userID] = now
	s.mu.Unlock()

	logger := logging.LoggerWithContext(ctx)
	logging.GoSafe(ctx, "activity-stamp", func() {
		writeCtx, cancel := context.WithTimeout(context.Background(), stampWriteTimeout)
		defer cancel()
		if err := s.recorder.RecordUserActivity(writeCtx, userID, now.Unix()); err != nil {
			// Best-effort ops telemetry: a failed stamp must never fail or
			// slow a request, and the user's next request after the
			// interval retries naturally — so a Warn is the whole story.
			// Clearing the cache entry lets that retry happen sooner.
			s.mu.Lock()
			delete(s.lastStamp, userID)
			s.mu.Unlock()
			logger.Warn("failed to record user activity stamp",
				"user_id", userID,
				"error", err,
			)
		}
	})
}
