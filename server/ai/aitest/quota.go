// Package aitest provides shared helpers for tests that hit live AI
// providers. These helpers are only intended for use from _test.go files,
// but live in a non-_test.go so they can be imported across packages.
package aitest

import (
	"strings"

	"go.ripls.org/ripls/server/errs"
)

// IsQuotaError reports whether err looks like a provider rate-limit / quota
// response (HTTP 429, gRPC RESOURCE_EXHAUSTED, "quota", "rate limit"). When
// a live-AI test hits a quota outage the right behavior is t.Skip rather
// than t.Fatal — the test cannot prove or disprove anything about our code.
func IsQuotaError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "resource exhausted") ||
		strings.Contains(msg, "quota")
}

// IsTransientError reports whether err looks like a transient infrastructure
// failure from a live AI provider — quota throttling (see IsQuotaError) plus
// transport-level blips like dropped connections, HTTP 5xx, and gRPC
// UNAVAILABLE / DEADLINE_EXCEEDED. Same rationale as IsQuotaError: when the
// upstream is broken, the test cannot prove or disprove anything about our
// code, so t.Skip is more useful than t.Fatal.
//
// Forwards to errs.IsTransient; the substrings are identical to the
// historical aitest implementation. Kept under aitest so callers in
// _test.go files don't need to rename their imports.
func IsTransientError(err error) bool {
	return errs.IsTransient(err)
}
