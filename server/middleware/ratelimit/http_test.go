package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"

	"go.ripls.org/ripls/server/logging"
)

// newHTTPRequestWithIP builds a request whose context carries the
// resolved client IP, mimicking what middleware.RemoteAddr installs.
func newHTTPRequestWithIP(t *testing.T, ip string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/go/SHORTCODE/decline", nil)
	r = r.WithContext(logging.WithRemoteAddr(context.Background(), ip))
	return r
}

// captureHandler returns a handler that records each call so tests
// can count how many requests passed through.
type captureHandler struct {
	calls int
}

func (c *captureHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	c.calls++
	w.WriteHeader(http.StatusNoContent)
}

func TestPerIPHTTP_AllowsWithinBurst(t *testing.T) {
	capacity := &captureHandler{}
	mw := PerIPHTTP(HTTPBudget{Rate: rate.Limit(1), Burst: 3}, "Op")
	handler := mw(capacity)

	for i := range 3 {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, newHTTPRequestWithIP(t, "10.0.0.1"))
		if w.Code != http.StatusNoContent {
			t.Fatalf("burst request %d expected 204, got %d", i+1, w.Code)
		}
	}
	if capacity.calls != 3 {
		t.Errorf("expected 3 passthrough calls within burst, got %d", capacity.calls)
	}
}

func TestPerIPHTTP_RejectsOverBurst(t *testing.T) {
	capacity := &captureHandler{}
	mw := PerIPHTTP(HTTPBudget{Rate: rate.Limit(0.001), Burst: 1}, "Op")
	handler := mw(capacity)

	// First request consumes the only burst token.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, newHTTPRequestWithIP(t, "10.0.0.2"))
	if w.Code != http.StatusNoContent {
		t.Fatalf("first request expected 204, got %d", w.Code)
	}

	// Second request must be 429.
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, newHTTPRequestWithIP(t, "10.0.0.2"))
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("over-burst request expected 429, got %d", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got == "" {
		t.Errorf("over-burst request missing Retry-After header")
	}
	if capacity.calls != 1 {
		t.Errorf("expected exactly 1 passthrough call (the burst one), got %d", capacity.calls)
	}
}

func TestPerIPHTTP_BucketsAreIndependentPerIP(t *testing.T) {
	capacity := &captureHandler{}
	mw := PerIPHTTP(HTTPBudget{Rate: rate.Limit(0.001), Burst: 1}, "Op")
	handler := mw(capacity)

	// IP A consumes its token.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, newHTTPRequestWithIP(t, "10.0.0.10"))
	if w.Code != http.StatusNoContent {
		t.Fatalf("ipA first call expected 204, got %d", w.Code)
	}

	// IP B should have its own token — independent bucket.
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, newHTTPRequestWithIP(t, "10.0.0.11"))
	if w.Code != http.StatusNoContent {
		t.Fatalf("ipB first call expected 204 (independent bucket), got %d", w.Code)
	}

	// IP A again — should be rate-limited.
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, newHTTPRequestWithIP(t, "10.0.0.10"))
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("ipA second call expected 429, got %d", w.Code)
	}
}

func TestPerIPHTTP_MissingIPFallsBackToSharedBucket(t *testing.T) {
	capacity := &captureHandler{}
	mw := PerIPHTTP(HTTPBudget{Rate: rate.Limit(0.001), Burst: 1}, "Op")
	handler := mw(capacity)

	// Request without RemoteAddr in context — both should share the
	// "unknown" bucket so an addrless flood is still capped.
	r := httptest.NewRequest(http.MethodPost, "/go/SHORTCODE/decline", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("first addrless request expected 204, got %d", w.Code)
	}

	r = httptest.NewRequest(http.MethodPost, "/go/SHORTCODE/decline", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("second addrless request expected 429 (shared bucket), got %d", w.Code)
	}
}
