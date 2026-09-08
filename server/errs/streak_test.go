package errs

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

// fakeClock lets streak tests advance time deterministically.
type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time { return f.now }

func (f *fakeClock) Advance(d time.Duration) {
	f.now = f.now.Add(d)
}

func newTestStreak(clock *fakeClock) *TransientStreak {
	return &TransientStreak{now: clock.Now}
}

// recordedLog is one captured Log call from a recordingLogger.
type recordedLog struct {
	level slog.Level
	msg   string
	args  []any
}

// recordingLogger satisfies LeveledLogger and captures every call
// without touching slog.JSONHandler (which transitively trips the
// project's forbidigo `analyze-types` linter).
type recordingLogger struct {
	calls []recordedLog
}

func (r *recordingLogger) Log(_ context.Context, level slog.Level, msg string, args ...any) {
	r.calls = append(r.calls, recordedLog{level: level, msg: msg, args: args})
}

func TestTransientStreak_NonTransientErrorIsAlwaysError(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	s := newTestStreak(clock)

	if got := s.Classify(errors.New("permission denied")); got != slog.LevelError {
		t.Errorf("non-transient err: got %v, want ERROR", got)
	}
}

func TestTransientStreak_FreshTransientIsWarn(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	s := newTestStreak(clock)

	got := s.Classify(errors.New("connection reset by peer"))
	if got != slog.LevelWarn {
		t.Errorf("fresh transient: got %v, want WARN", got)
	}
}

func TestTransientStreak_StreakStaysWarnUnderThreshold(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	s := newTestStreak(clock)
	transient := errors.New("connection reset by peer")

	if got := s.Classify(transient); got != slog.LevelWarn {
		t.Fatalf("tick 1: got %v, want WARN", got)
	}
	clock.Advance(1 * time.Minute)
	if got := s.Classify(transient); got != slog.LevelWarn {
		t.Errorf("tick 2 (1m): got %v, want WARN", got)
	}
	clock.Advance(TransientPageAfter - 1*time.Minute - 1*time.Second)
	if got := s.Classify(transient); got != slog.LevelWarn {
		t.Errorf("tick 3 (~5m - 1s): got %v, want WARN", got)
	}
}

func TestTransientStreak_StreakEscalatesAtThreshold(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	s := newTestStreak(clock)
	transient := errors.New("connection reset by peer")

	if got := s.Classify(transient); got != slog.LevelWarn {
		t.Fatalf("tick 1: got %v, want WARN", got)
	}
	clock.Advance(TransientPageAfter + 1*time.Second)
	if got := s.Classify(transient); got != slog.LevelError {
		t.Errorf("tick 2 (>5m): got %v, want ERROR", got)
	}
	clock.Advance(1 * time.Minute)
	if got := s.Classify(transient); got != slog.LevelError {
		t.Errorf("tick 3 (>6m): got %v, want ERROR", got)
	}
}

func TestTransientStreak_ResetClearsStreak(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	s := newTestStreak(clock)
	transient := errors.New("connection reset by peer")

	s.Classify(transient)
	clock.Advance(TransientPageAfter + 1*time.Second)
	if got := s.Classify(transient); got != slog.LevelError {
		t.Fatalf("pre-reset: got %v, want ERROR", got)
	}

	s.Reset()

	if got := s.Classify(transient); got != slog.LevelWarn {
		t.Errorf("post-reset: got %v, want WARN", got)
	}
}

func TestTransientStreak_NonTransientResetsStreak(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	s := newTestStreak(clock)
	transient := errors.New("connection reset by peer")

	s.Classify(transient)
	clock.Advance(2 * time.Minute)

	if got := s.Classify(errors.New("permission denied")); got != slog.LevelError {
		t.Fatalf("non-transient mid-streak: got %v, want ERROR", got)
	}

	if got := s.Classify(transient); got != slog.LevelWarn {
		t.Errorf("post-non-transient transient: got %v, want WARN", got)
	}
}

func TestLogJobError_NilErrorResetsAndEmitsNothing(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	s := newTestStreak(clock)
	s.Classify(errors.New("connection reset by peer"))
	if s.firstTransientAt.IsZero() {
		t.Fatal("setup: expected non-zero firstTransientAt after Classify")
	}

	r := &recordingLogger{}
	LogJobError(context.Background(), r, s, "job", nil)

	if !s.firstTransientAt.IsZero() {
		t.Error("LogJobError(nil) should Reset the streak")
	}
	if len(r.calls) != 0 {
		t.Errorf("LogJobError(nil) should emit no log line, got %d calls", len(r.calls))
	}
}

func TestLogJobError_TransientLogsWarnWithErrorField(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	s := newTestStreak(clock)
	r := &recordingLogger{}

	transient := errors.New("connection reset by peer")
	LogJobError(context.Background(), r, s, "dispatch failed", transient, "extra_key", "extra_val")

	if len(r.calls) != 1 {
		t.Fatalf("expected 1 log call, got %d", len(r.calls))
	}
	got := r.calls[0]
	if got.level != slog.LevelWarn {
		t.Errorf("level: got %v, want WARN", got.level)
	}
	if got.msg != "dispatch failed" {
		t.Errorf("msg: got %q, want %q", got.msg, "dispatch failed")
	}
	wantArgs := []any{"error", transient, "extra_key", "extra_val"}
	if !argsEqual(got.args, wantArgs) {
		t.Errorf("args: got %v, want %v", got.args, wantArgs)
	}
}

func TestLogJobError_NonTransientLogsError(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	s := newTestStreak(clock)
	r := &recordingLogger{}

	LogJobError(context.Background(), r, s, "dispatch failed", errors.New("permission denied"))

	if len(r.calls) != 1 {
		t.Fatalf("expected 1 log call, got %d", len(r.calls))
	}
	if r.calls[0].level != slog.LevelError {
		t.Errorf("level: got %v, want ERROR", r.calls[0].level)
	}
}

func TestLogJobError_TransientPastThresholdLogsError(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	s := newTestStreak(clock)
	transient := errors.New("connection reset by peer")

	// Burn one WARN to start the streak.
	LogJobError(context.Background(), &recordingLogger{}, s, "dispatch failed", transient)

	// Advance past threshold.
	clock.Advance(TransientPageAfter + 1*time.Second)

	r := &recordingLogger{}
	LogJobError(context.Background(), r, s, "dispatch failed", transient)

	if len(r.calls) != 1 {
		t.Fatalf("expected 1 log call, got %d", len(r.calls))
	}
	if r.calls[0].level != slog.LevelError {
		t.Errorf("level: got %v, want ERROR (streak past TransientPageAfter)", r.calls[0].level)
	}
}

func argsEqual(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
