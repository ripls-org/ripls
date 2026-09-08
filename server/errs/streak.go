package errs

import (
	"context"
	"log/slog"
	"time"
)

// TransientPageAfter is the duration after which an ongoing streak
// of transient failures stops being treated as a single self-healing
// blip and starts escalating to ERROR-level logs (which fires the
// "Server Error Logged" alert policy and pages on-call).
//
// 5 minutes is the operational SLO for "noticed in time to matter":
// long enough that a one-off connection reset on a pooled DB
// connection doesn't page, short enough that a real upstream outage
// pages before user impact compounds. Change here to shift the SLO
// globally.
const TransientPageAfter = 5 * time.Minute

// TransientStreak tracks how long the current run of transient
// failures has been going for a single long-running background job.
// It is the small piece of state that converts "every tick failed
// with a transient error" into "this has been failing for too long
// — escalate."
//
// Concurrency: not safe for concurrent use. Each instance is owned
// by exactly one goroutine (one background-job loop).
type TransientStreak struct {
	firstTransientAt time.Time // zero when no streak is active
	now              func() time.Time
}

// NewTransientStreak returns a fresh streak with no in-progress
// transient run.
func NewTransientStreak() *TransientStreak {
	return &TransientStreak{now: time.Now}
}

// Classify decides the log level for a non-nil error from a
// background job. Callers must only call Classify with err != nil;
// the success path is Reset.
//
// Behavior:
//   - non-transient err → ERROR, and the streak resets (a real
//     error supersedes any in-progress transient streak).
//   - transient err, first of a fresh streak → WARN; records the
//     streak's start time.
//   - transient err, streak still under TransientPageAfter → WARN.
//   - transient err, streak past TransientPageAfter → ERROR; the
//     streak continues, so subsequent transient errors keep
//     returning ERROR until Reset is called.
func (s *TransientStreak) Classify(err error) slog.Level {
	if !IsTransient(err) {
		s.firstTransientAt = time.Time{}
		return slog.LevelError
	}
	if s.firstTransientAt.IsZero() {
		s.firstTransientAt = s.now()
		return slog.LevelWarn
	}
	if s.now().Sub(s.firstTransientAt) < TransientPageAfter {
		return slog.LevelWarn
	}
	return slog.LevelError
}

// Reset clears any in-progress transient streak. Background-job
// callers should call this on every successful tick so the next
// isolated transient failure starts a fresh streak.
func (s *TransientStreak) Reset() {
	s.firstTransientAt = time.Time{}
}

// LeveledLogger is the minimal slog surface LogJobError needs.
// Both *slog.Logger and the project's *logging.Logger (which
// embeds *slog.Logger) satisfy it, so call sites can pass either.
type LeveledLogger interface {
	Log(ctx context.Context, level slog.Level, msg string, args ...any)
}

// LogJobError is the one-line wrapper background-job goroutines use
// to log the result of a tick. It encapsulates the level decision
// (via streak.Classify) and the standard "error" log field
// (see docs/server/observability.md §Standard Field Names).
//
// On success (err == nil) it resets the streak and emits no log
// line; success-path logging stays the caller's choice.
//
// Extra key-value pairs in kv are appended after the error field,
// following slog conventions.
func LogJobError(
	ctx context.Context,
	logger LeveledLogger,
	streak *TransientStreak,
	msg string,
	err error,
	kv ...any,
) {
	if err == nil {
		streak.Reset()
		return
	}
	level := streak.Classify(err)
	args := make([]any, 0, 2+len(kv))
	args = append(args, "error", err)
	args = append(args, kv...)
	logger.Log(ctx, level, msg, args...)
}
