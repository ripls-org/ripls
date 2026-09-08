package errs

import "strings"

// IsTransient reports whether err looks like a transient
// infrastructure failure: rate-limit / quota responses
// (HTTP 429, gRPC RESOURCE_EXHAUSTED), transport-level blips
// (connection reset, broken pipe, i/o timeout, TLS handshake),
// upstream 5xx (502/503/504), gRPC UNAVAILABLE / DEADLINE_EXCEEDED,
// and DNS lookup failures.
//
// The predicate is intentionally string-based: it has to recognize
// errors that originate in three very different stacks (PostgreSQL
// drivers, HTTP clients for AI providers, gRPC). All three lower
// their wire failures into Go errors whose human-readable strings
// share the substrings matched below.
//
// Callers use this to decide whether to retry, downgrade a log
// level, or convert to connect.CodeUnavailable instead of
// CodeInternal.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "resource exhausted") ||
		strings.Contains(msg, "quota") ||
		strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "unavailable") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "tls handshake") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "502") ||
		strings.Contains(msg, "504")
}
