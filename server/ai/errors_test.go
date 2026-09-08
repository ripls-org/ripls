package ai

import (
	"errors"
	"testing"
)

func TestIsTransientError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unrelated", errors.New("permission denied"), false},
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsTransientError(tc.err); got != tc.want {
				t.Errorf("IsTransientError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
