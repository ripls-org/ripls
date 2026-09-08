// Package clock provides simulation-aware time functions.
// When a simulation timestamp is injected into the context (via the SimulationTimestamp
// middleware), Now() and UnixSec() return the simulated time instead of the real time.
// This allows the simulation system to replay activity at arbitrary points in time
// while the server processes requests normally.
package clock

import (
	"context"
	"time"
)

type contextKey struct{}

// WithSimulationTime returns a new context with the given time stored for
// clock.Now() to return instead of the real wall clock.
func WithSimulationTime(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, contextKey{}, t)
}

// Now returns the simulation timestamp from the context if present,
// otherwise time.Now().
func Now(ctx context.Context) time.Time {
	if t, ok := ctx.Value(contextKey{}).(time.Time); ok {
		return t
	}
	return time.Now()
}

// UnixSec returns Now(ctx).Unix() for convenience. This is the most common
// usage in the codebase where business timestamps are recorded as int64 seconds.
func UnixSec(ctx context.Context) int64 {
	return Now(ctx).Unix()
}

// IsSimulated returns true if the context has a simulation timestamp injected.
// This can be used to skip expensive operations (like AI calls) during simulation.
func IsSimulated(ctx context.Context) bool {
	_, ok := ctx.Value(contextKey{}).(time.Time)
	return ok
}
