// Package ai provides AI provider integrations for Gemini, Vertex AI, Anthropic,
// and OpenAI. This file contains a shared logging helper for service-layer AI call
// sites, implementing the "transient → WARN, terminal → ERROR" discipline described
// in docs/server/observability.md §L98-126 ("Leaf provider + fallback wrapper") and
// mirroring the errs.LogJobError pattern used by background-job goroutines.
package ai

import (
	"context"
	"log/slog"

	"go.ripls.org/ripls/server/errs"
)

// LogAIError logs err at WARN when it is a transient AI-provider failure (429,
// RESOURCE_EXHAUSTED, 5xx, connection reset) and at ERROR otherwise. It always
// adds "error" and "transient" structured fields; extra key-value pairs may be
// appended via kv. If err is nil the function is a no-op.
//
// Use this at every service-handler site that receives an error from
// s.aiProvider.<method> so the "Server Error Logged" alert fires only on true
// system failures, not on quota-throttle blips. This is the service-caller
// analogue of the leaf/fallback doctrine in docs/server/observability.md §L98-126
// and the parallel of errs.LogJobError for request-path code.
func LogAIError(ctx context.Context, logger errs.LeveledLogger, msg string, err error, kv ...any) {
	if err == nil {
		return
	}
	transient := IsTransientError(err)
	level := slog.LevelError
	if transient {
		level = slog.LevelWarn
	}
	args := make([]any, 0, 4+len(kv))
	args = append(args, "error", err, "transient", transient)
	args = append(args, kv...)
	logger.Log(ctx, level, msg, args...)
}
