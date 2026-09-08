package aitest

import (
	"errors"
	"testing"
)

func TestIsQuotaError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unrelated", errors.New("permission denied"), false},
		{"http 429", errors.New("HTTP 429: Too Many Requests"), true},
		{"rate limit", errors.New("Rate limit exceeded"), true},
		{"resource exhausted", errors.New("rpc error: code = ResourceExhausted desc = quota"), true},
		{"quota", errors.New("Quota exceeded for project"), true},
		{"transient eof", errors.New("unavailable: unexpected EOF"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsQuotaError(tc.err); got != tc.want {
				t.Errorf("IsQuotaError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsTransientError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unrelated", errors.New("permission denied"), false},
		{"quota also transient", errors.New("HTTP 429"), true},
		{"unexpected eof", errors.New("unavailable: unexpected EOF"), true},
		{"grpc unavailable", errors.New("rpc error: code = Unavailable"), true},
		{"deadline exceeded", errors.New("context deadline exceeded"), true},
		{"connection reset", errors.New("read tcp: connection reset by peer"), true},
		{"connection refused", errors.New("dial tcp: connection refused"), true},
		{"no such host", errors.New("dial tcp: lookup foo.example: no such host"), true},
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
