package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// recordingLogger captures LogJobError output for assertions.
type recordingLogger struct {
	mu      sync.Mutex
	entries []recordedEntry
}

type recordedEntry struct {
	level slog.Level
	msg   string
}

func (r *recordingLogger) Log(_ context.Context, level slog.Level, msg string, _ ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, recordedEntry{level: level, msg: msg})
}

func (r *recordingLogger) snapshot() []recordedEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedEntry(nil), r.entries...)
}

// waitFor polls cond until it returns true or the deadline passes.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		//nolint:forbidigo // Backoff inside a bounded condition poll, which is the
		// pattern #1364 prescribes as the alternative to a fixed-duration sleep.
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}

func TestRunPeriodicRunsAtStartupAndOnTicks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	runs := 0
	RunPeriodic(ctx, &recordingLogger{}, Periodic{
		Name:       "test-periodic",
		Interval:   10 * time.Millisecond,
		StartupMsg: "test job failed at startup",
		TickMsg:    "test job failed",
		Run: func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			runs++
			return nil
		},
	})

	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return runs >= 3 // startup run + at least two ticks
	})
}

func TestRunPeriodicLogsStartupAndTickMessages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rec := &recordingLogger{}
	RunPeriodic(ctx, rec, Periodic{
		Name:       "test-periodic-msgs",
		Interval:   10 * time.Millisecond,
		StartupMsg: "job failed at startup",
		TickMsg:    "job failed",
		Run: func(context.Context) error {
			return fmt.Errorf("boom")
		},
	})

	waitFor(t, func() bool { return len(rec.snapshot()) >= 2 })
	entries := rec.snapshot()
	if entries[0].msg != "job failed at startup" {
		t.Errorf("first failure msg = %q; want startup message", entries[0].msg)
	}
	if entries[1].msg != "job failed" {
		t.Errorf("second failure msg = %q; want tick message", entries[1].msg)
	}
	// Non-transient errors log at ERROR immediately.
	if entries[0].level != slog.LevelError {
		t.Errorf("first failure level = %v; want ERROR", entries[0].level)
	}
}

func TestRunPeriodicIgnoresContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	rec := &recordingLogger{}
	started := make(chan struct{})
	var once sync.Once
	RunPeriodic(ctx, rec, Periodic{
		Name:       "test-periodic-cancel",
		Interval:   time.Hour,
		StartupMsg: "should not log",
		TickMsg:    "should not log",
		Run: func(ctx context.Context) error {
			once.Do(func() { close(started) })
			return context.Canceled
		},
	})

	<-started
	cancel()
	// Give the goroutine a beat to exit; no log entries may appear.
	//nolint:forbidigo // Asserts an absence (no log entry is ever written). There
	// is nothing to synchronize on when the expected outcome is that nothing
	// happens, so a bounded wait is the only way to observe it.
	time.Sleep(20 * time.Millisecond)
	if entries := rec.snapshot(); len(entries) != 0 {
		t.Errorf("context.Canceled produced log entries: %v", entries)
	}
}

func TestRunPeriodicStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var mu sync.Mutex
	runs := 0
	RunPeriodic(ctx, &recordingLogger{}, Periodic{
		Name:       "test-periodic-stop",
		Interval:   10 * time.Millisecond,
		StartupMsg: "m",
		TickMsg:    "m",
		Run: func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			runs++
			return nil
		},
	})

	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return runs >= 2
	})
	cancel()
	//nolint:forbidigo // Asserts an absence (the loop stops ticking after cancel).
	// The two sleeps sample the run counter across a window in which, if the loop
	// were still alive, it would have ticked several times.
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	after := runs
	mu.Unlock()
	//nolint:forbidigo // Second sample of the same absence assertion; see above.
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	final := runs
	mu.Unlock()
	// At most one in-flight run may complete after cancel; the loop must stop.
	if final > after+1 {
		t.Errorf("runs continued after cancel: %d -> %d", after, final)
	}
}

func TestRunPeriodicRecoversPanicsViaGoSafe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	runs := 0
	RunPeriodic(ctx, &recordingLogger{}, Periodic{
		Name:       "test-periodic-panic",
		Interval:   10 * time.Millisecond,
		StartupMsg: "m",
		TickMsg:    "m",
		Run: func(context.Context) error {
			mu.Lock()
			runs++
			n := runs
			mu.Unlock()
			if n == 1 {
				panic("startup panic")
			}
			return nil
		},
	})

	// The panic in the startup run kills the goroutine (GoSafe logs it and
	// prevents a process crash); the test passes if the process survives.
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return runs >= 1
	})
	if errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("unexpected context cancellation")
	}
}
