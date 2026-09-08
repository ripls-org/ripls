# Clock

The `clock` package provides simulation-aware time functions. All server code that records or compares timestamps should use `clock.Now(ctx)` instead of `time.Now()` so that the load-testing simulation can inject a synthetic timestamp and replay activity at arbitrary points in time.

## Key files

- `clock.go` — `Now(ctx)`, `UnixSec(ctx)`, `IsSimulated(ctx)`, and `WithSimulationTime(ctx, t)`.
- `middleware.go` — HTTP middleware that reads a simulation timestamp from a request header and injects it into the context via `WithSimulationTime`.

## When to add code here vs. elsewhere

Keep this package minimal. If you need to drive time-based behaviour from context (e.g. expiry checks, event timestamps), use `clock.Now(ctx)`. Time-zone conversion helpers belong in `server/timezone`; simulation-specific orchestration belongs in `server/simulation`.
