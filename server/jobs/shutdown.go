package jobs

import (
	"context"
	"errors"
	"strings"

	"go.ripls.org/ripls/server/logging"
)

// isShutdownError reports whether a cancelled context or err indicates the
// process is shutting down, rather than a genuine per-row failure.
//
// During a Cloud Run instance teardown main.go cancels the root context and,
// after the request-drain window, the deferred sqlStorage.Close() closes the
// shared *sql.DB pool — both while startup backfill goroutines may still be
// mid-loop. Because database/sql checks its closed flag before the context,
// the first storage call after teardown surfaces "sql: database is closed" (an
// unexported sentinel, hence the string match) even though the context is also
// already cancelled. A loop that keeps iterating then logs one error per
// remaining row, turning a routine shutdown into an error-rate alert. Backfill
// loops call this to stop early instead.
func isShutdownError(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return err != nil && strings.Contains(err.Error(), "sql: database is closed")
}

// logBackfillCancelled emits the single INFO line a backfill loop uses when it
// stops early because the process is shutting down. Uniform wording keeps the
// shutdown path easy to recognize and exclude from error-rate alerts across
// every startup job. It returns the context cancellation cause, suitable for
// returning up to main.go (which swallows context.Canceled). When the context
// is not yet cancelled — the rare case where the DB pool closed first — it
// returns context.Canceled so the caller's shutdown path stays uniform.
func logBackfillCancelled(ctx context.Context, logger *logging.Logger, job string) error {
	cause := ctx.Err()
	if cause == nil {
		cause = context.Canceled
	}
	logger.InfoContext(ctx, "backfill cancelled mid-run, stopping early",
		"job", job, "error", cause)
	return cause
}
