package simulation

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.ripls.org/ripls/server/clock"
)

// TestClientPool_For_ReturnsIndependentClients verifies that each user's
// pool client carries its own auth token and that switching between them
// never leaks one user's token to another's requests — the guarantee that
// makes the #1061 giveaway bug impossible to reintroduce.
func TestClientPool_For_ReturnsIndependentClients(t *testing.T) {
	var mu sync.Mutex
	capturedAuth := map[string]string{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedAuth[r.URL.Path] = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	pool := NewClientPool(ts.URL, map[string]string{
		"alice@example.com": "token-alice",
		"bob@example.com":   "token-bob",
	})

	a := pool.For("alice@example.com")
	b := pool.For("bob@example.com")
	if a == nil || b == nil {
		t.Fatal("pool.For returned nil for seeded users")
	}
	if a == b {
		t.Error("pool.For returned the same *Client for different users")
	}

	// Interleave requests to exercise that each client's token stays put.
	doReq := func(c *Client, path string) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		resp, err := c.httpClient().Do(req)
		if err != nil {
			t.Fatalf("request %s: %v", path, err)
		}
		resp.Body.Close()
	}
	doReq(a, "/as-alice")
	doReq(b, "/as-bob")
	doReq(a, "/as-alice-again")

	mu.Lock()
	defer mu.Unlock()
	if capturedAuth["/as-alice"] != "Bearer token-alice" {
		t.Errorf("/as-alice saw %q, want Bearer token-alice", capturedAuth["/as-alice"])
	}
	if capturedAuth["/as-bob"] != "Bearer token-bob" {
		t.Errorf("/as-bob saw %q, want Bearer token-bob", capturedAuth["/as-bob"])
	}
	if capturedAuth["/as-alice-again"] != "Bearer token-alice" {
		t.Errorf("/as-alice-again saw %q, want Bearer token-alice", capturedAuth["/as-alice-again"])
	}
}

// TestClientPool_SetTimestamp_FansOut verifies that SetTimestamp applies the
// given timestamp to every pool client. This is how the sequential executor
// keeps cross-user side operations within a single step on the same
// simulated wall clock.
func TestClientPool_SetTimestamp_FansOut(t *testing.T) {
	var mu sync.Mutex
	capturedTS := map[string]string{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedTS[r.URL.Path] = r.Header.Get(clock.SimulationTimestampHeader)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	pool := NewClientPool(ts.URL, map[string]string{
		"alice@example.com": "t-a",
		"bob@example.com":   "t-b",
	})
	simTime := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	pool.SetTimestamp(simTime)

	for email, path := range map[string]string{
		"alice@example.com": "/a",
		"bob@example.com":   "/b",
	} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		resp, err := pool.For(email).httpClient().Do(req)
		if err != nil {
			t.Fatalf("request %s: %v", path, err)
		}
		resp.Body.Close()
	}

	want := fmt.Sprintf("%d", simTime.Unix())
	mu.Lock()
	defer mu.Unlock()
	if capturedTS["/a"] != want || capturedTS["/b"] != want {
		t.Errorf("timestamps = %v, want both %q", capturedTS, want)
	}
}

// TestClientPool_For_UnknownUser verifies that looking up a user who isn't
// in the pool returns nil (rather than panicking). Callers are expected to
// nil-check and skip the step rather than send a tokenless request.
func TestClientPool_For_UnknownUser(t *testing.T) {
	pool := NewClientPool("http://unused.test", map[string]string{
		"alice@example.com": "t",
	})
	if got := pool.For("ghost@example.com"); got != nil {
		t.Errorf("For unknown user = %v, want nil", got)
	}
	// Empty tokens are intentionally excluded at pool construction.
	pool2 := NewClientPool("http://unused.test", map[string]string{
		"empty@example.com": "",
	})
	if got := pool2.For("empty@example.com"); got != nil {
		t.Errorf("For user with empty token = %v, want nil", got)
	}
}
