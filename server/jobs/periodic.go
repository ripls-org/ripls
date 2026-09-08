package jobs

import (
	"context"
	"errors"
	"time"

	"go.ripls.org/ripls/server/errs"
	"go.ripls.org/ripls/server/logging"
)

// Periodic describes a background job loop: run once at startup, then on
// every Interval tick until the context is cancelled.
type Periodic struct {
	// Name identifies the goroutine in panic logs (logging.GoSafe).
	Name string
	// Interval between runs after the immediate startup run.
	Interval time.Duration
	// StartupMsg is the failure log message for the immediate startup run,
	// TickMsg for the periodic runs. Several jobs deliberately distinguish
	// the two in alert queries; pass the same string when the distinction
	// doesn't matter. These strings are alert-query surface — treat existing
	// values as frozen.
	StartupMsg string
	TickMsg    string
	// Run performs one iteration. A context.Canceled return is treated as
	// shutdown and not logged; any other error is classified through a
	// shared errs.TransientStreak (transient errors log WARN, escalating to
	// ERROR after errs.TransientPageAfter; the rest log ERROR immediately).
	// A nil return resets the streak silently.
	Run func(context.Context) error
}

// RunPeriodic spawns a panic-safe goroutine that drives p until ctx is
// cancelled. It owns the boilerplate every periodic job used to copy:
// logging.GoSafe wrapping, the immediate startup run, the ticker loop, the
// shutdown check, and TransientStreak-classified error logging.
func RunPeriodic(ctx context.Context, logger errs.LeveledLogger, p Periodic) {
	logging.GoSafe(ctx, p.Name, func() {
		streak := errs.NewTransientStreak()
		runOnce := func(msg string) {
			err := p.Run(ctx)
			if errors.Is(err, context.Canceled) {
				return
			}
			errs.LogJobError(ctx, logger, streak, msg, err)
		}
		runOnce(p.StartupMsg)
		t := time.NewTicker(p.Interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				runOnce(p.TickMsg)
			}
		}
	})
}
