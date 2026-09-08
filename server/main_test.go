package main

import (
	"testing"
	"time"
)

// TestHTTPTimeoutsAllowLongStreams asserts the http.Server timeouts in main.go
// are sized so server-streaming RPCs (capped at ~14 min by the community
// service's defaultStreamLifetime) aren't preempted by the transport. Regression
// guard for #1810: prior values (WriteTimeout=120s, IdleTimeout=120s) closed
// every long-lived stream after 2 minutes.
func TestHTTPTimeoutsAllowLongStreams(t *testing.T) {
	// streamLifetimeBudget mirrors server/services/community.defaultStreamLifetime.
	// If that constant ever rises above this value, bump it here too.
	const streamLifetimeBudget = 14 * time.Minute

	if httpWriteTimeout != 0 {
		t.Errorf("httpWriteTimeout = %v; want 0 (no cap, so streams can run their full lifetime)", httpWriteTimeout)
	}
	if httpIdleTimeout <= streamLifetimeBudget {
		t.Errorf("httpIdleTimeout = %v; want > %v so persistent connections aren't reaped mid-stream", httpIdleTimeout, streamLifetimeBudget)
	}
	if httpReadHeaderTimeout <= 0 {
		t.Errorf("httpReadHeaderTimeout = %v; want > 0 to guard against slow-loris on headers", httpReadHeaderTimeout)
	}
	if httpReadTimeout <= 0 {
		t.Errorf("httpReadTimeout = %v; want > 0 to guard against slow-loris on request bodies", httpReadTimeout)
	}
}
