package jobs

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.ripls.org/ripls/server/logging"
)

// TestIsShutdownError covers the three signals the backfill loops rely on to
// distinguish a process teardown from a genuine per-row failure.
func TestIsShutdownError(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{"live ctx, nil err", context.Background(), nil, false},
		{"live ctx, ordinary err", context.Background(), errors.New("boom"), false},
		{"cancelled ctx, nil err", cancelledCtx, nil, true},
		{"cancelled ctx, ordinary err", cancelledCtx, errors.New("boom"), true},
		{"live ctx, context.Canceled err", context.Background(), context.Canceled, true},
		{"live ctx, deadline err", context.Background(), context.DeadlineExceeded, true},
		{"live ctx, wrapped canceled", context.Background(), fmt.Errorf("query: %w", context.Canceled), true},
		// The database/sql closed-DB sentinel is unexported, so it is matched
		// by string. This is the error a per-row query surfaces when the pool
		// closes before the context is observed cancelled.
		{"live ctx, db closed string", context.Background(), errors.New("failed to query: sql: database is closed"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isShutdownError(tt.ctx, tt.err); got != tt.want {
				t.Errorf("isShutdownError() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestLogBackfillCancelled verifies the helper returns the cancellation cause,
// falling back to context.Canceled when the context is not (yet) cancelled —
// the rare case where the DB pool closed first. The returned error must always
// satisfy errors.Is(err, context.Canceled) so main.go's guard swallows it.
func TestLogBackfillCancelled(t *testing.T) {
	logger := logging.Default()

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := logBackfillCancelled(cancelledCtx, logger, "unit"); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled ctx: got %v, want context.Canceled", err)
	}

	if err := logBackfillCancelled(context.Background(), logger, "unit"); !errors.Is(err, context.Canceled) {
		t.Errorf("live ctx fallback: got %v, want context.Canceled", err)
	}
}
