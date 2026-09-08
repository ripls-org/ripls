package errs

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestIsTransient(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		// Negative cases.
		{"nil", nil, false},
		{"unrelated", errors.New("permission denied"), false},
		{"not found", errors.New("record not found"), false},

		// AI / HTTP / gRPC transient cases (parity with the
		// original ai.IsTransientError test inputs).
		{"http 429", errors.New("HTTP 429: Too Many Requests"), true},
		{"resource exhausted", errors.New("Error 429, Message: Resource exhausted. Please try again."), true},
		{"rate limit", errors.New("rate limit exceeded"), true},
		{"quota", errors.New("Quota exceeded for project"), true},
		{"grpc unavailable", errors.New("rpc error: code = Unavailable"), true},
		{"unexpected eof", errors.New("unexpected EOF"), true},
		{"deadline exceeded", errors.New("context deadline exceeded"), true},
		{"connection reset", errors.New("read tcp: connection reset by peer"), true},
		{"connection refused", errors.New("dial tcp: connection refused"), true},
		{"no such host", errors.New("dial tcp: lookup foo: no such host"), true},
		{"i/o timeout", errors.New("read tcp: i/o timeout"), true},
		{"tls handshake", errors.New("tls handshake timeout"), true},
		{"http 502", errors.New("upstream returned 502 Bad Gateway"), true},
		{"http 503", errors.New("503 Service Unavailable"), true},
		{"http 504", errors.New("504 Gateway Timeout"), true},

		// PostgreSQL-flavored transient cases (the reason this
		// package exists — issue #2128 was a connection-reset on
		// a Cloud SQL pooled connection).
		{
			"pg connection reset on read",
			errors.New("read tcp 169.254.8.1:40932->10.113.0.3:5432: read: connection reset by peer"),
			true,
		},
		{
			"pg broken pipe on write",
			errors.New("write tcp 169.254.8.1:40932->10.113.0.3:5432: write: broken pipe"),
			true,
		},
		{
			"pg dial timeout",
			errors.New("dial tcp 10.113.0.3:5432: i/o timeout"),
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsTransient(tc.err); got != tc.want {
				t.Errorf("IsTransient(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestDeadlineExceededIsTransient pins the classification that live-AI
// integration tests depend on. Those tests bound each provider call with
// context.WithTimeout and skip when IsTransient reports true, so a stalled
// backend costs one skipped test instead of a package-wide timeout panic (the
// `go test -timeout` in CI is per test binary, so an unbounded hang fails every
// test in the package). If "deadline exceeded" ever leaves the substring list,
// those tests go back to failing the build on a slow provider — silently.
func TestDeadlineExceededIsTransient(t *testing.T) {
	if !IsTransient(context.DeadlineExceeded) {
		t.Errorf("IsTransient(%q) = false, want true", context.DeadlineExceeded)
	}
	// The wrapped form a provider call actually returns.
	if !IsTransient(fmt.Errorf("calling anthropic: %w", context.DeadlineExceeded)) {
		t.Error("a wrapped DeadlineExceeded must still classify as transient")
	}
}
