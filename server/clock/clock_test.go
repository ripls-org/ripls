package clock

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNow_WithoutContext_ReturnsCurrentTime(t *testing.T) {
	ctx := context.Background()
	before := time.Now()
	got := Now(ctx)
	after := time.Now()

	if got.Before(before) || got.After(after) {
		t.Errorf("Now(ctx) = %v, want between %v and %v", got, before, after)
	}
}

func TestNow_WithSimulationTime_ReturnsOverride(t *testing.T) {
	simTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	ctx := WithSimulationTime(context.Background(), simTime)

	got := Now(ctx)
	if !got.Equal(simTime) {
		t.Errorf("Now(ctx) = %v, want %v", got, simTime)
	}
}

func TestUnixSec_WithSimulationTime(t *testing.T) {
	simTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	ctx := WithSimulationTime(context.Background(), simTime)

	got := UnixSec(ctx)
	want := simTime.Unix()
	if got != want {
		t.Errorf("UnixSec(ctx) = %d, want %d", got, want)
	}
}

func TestUnixSec_WithoutContext_ReturnsCurrentUnix(t *testing.T) {
	ctx := context.Background()
	before := time.Now().Unix()
	got := UnixSec(ctx)
	after := time.Now().Unix()

	if got < before || got > after {
		t.Errorf("UnixSec(ctx) = %d, want between %d and %d", got, before, after)
	}
}

func TestMiddleware_SetsContextFromHeader(t *testing.T) {
	var capturedTime time.Time
	handler := SimulationTimestamp(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTime = Now(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(SimulationTimestampHeader, "1700000000") // 2023-11-14T22:13:20Z
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	want := time.Unix(1700000000, 0)
	if !capturedTime.Equal(want) {
		t.Errorf("captured time = %v, want %v", capturedTime, want)
	}
}

func TestMiddleware_WithoutHeader_FallsThrough(t *testing.T) {
	var capturedTime time.Time
	handler := SimulationTimestamp(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTime = Now(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	before := time.Now()
	handler.ServeHTTP(rec, req)
	after := time.Now()

	if capturedTime.Before(before) || capturedTime.After(after) {
		t.Errorf("captured time = %v, want between %v and %v", capturedTime, before, after)
	}
}

func TestMiddleware_InvalidHeader_FallsThrough(t *testing.T) {
	var capturedTime time.Time
	handler := SimulationTimestamp(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTime = Now(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(SimulationTimestampHeader, "not-a-number")
	rec := httptest.NewRecorder()

	before := time.Now()
	handler.ServeHTTP(rec, req)
	after := time.Now()

	if capturedTime.Before(before) || capturedTime.After(after) {
		t.Errorf("captured time = %v, want between %v and %v (header was invalid)", capturedTime, before, after)
	}
}
