package ai

import "go.ripls.org/ripls/server/errs"

// IsTransientError reports whether err looks like a transient failure from an
// AI provider — quota throttling (HTTP 429, RESOURCE_EXHAUSTED, rate-limit)
// or transport-level blips (dropped connections, HTTP 5xx, gRPC UNAVAILABLE).
// When a call to an AI provider fails with a transient error, callers should
// return connect.CodeUnavailable rather than CodeInternal so that clients and
// tests can distinguish infrastructure flakes from real bugs.
//
// Forwards to errs.IsTransient; kept under the ai package so existing
// callers in server/services/experience don't need to rename their imports.
func IsTransientError(err error) bool {
	return errs.IsTransient(err)
}
