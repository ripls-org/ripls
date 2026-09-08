package ai

import (
	"context"
	"time"
)

// CallWithTimeout runs fn against a derived context bounded by timeout and
// returns fn's result. The derived context is always cancelled before return,
// so callers do not need to manage cancellation themselves. Intended for
// RPC-level AI calls where the surrounding handler already treats provider
// errors as best-effort (log and fall back to config defaults).
func CallWithTimeout[T any](ctx context.Context, timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return fn(ctx)
}
