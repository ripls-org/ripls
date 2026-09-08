package activitystamp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/authn"

	"go.ripls.org/ripls/server/auth"
)

// recordingRecorder captures stamps and can simulate failures.
type recordingRecorder struct {
	mu     sync.Mutex
	stamps []string
	err    error
	done   chan struct{} // signaled once per RecordUserActivity call
}

func newRecordingRecorder() *recordingRecorder {
	return &recordingRecorder{done: make(chan struct{}, 100)}
}

func (r *recordingRecorder) RecordUserActivity(_ context.Context, userID string, _ int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer func() { r.done <- struct{}{} }()
	if r.err != nil {
		return r.err
	}
	r.stamps = append(r.stamps, userID)
	return nil
}

func (r *recordingRecorder) waitForCalls(t *testing.T, n int) {
	t.Helper()
	for range n {
		select {
		case <-r.done:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %d recorder calls", n)
		}
	}
}

func (r *recordingRecorder) stampedUsers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.stamps...)
}

// serveAs runs one request through the stamper with the given user
// authenticated (empty userID = unauthenticated).
func serveAs(t *testing.T, s *Stamper, userID string) {
	t.Helper()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	if userID != "" {
		req = req.WithContext(authn.SetInfo(req.Context(), &auth.Info{UserID: userID}))
	}
	s.Wrap(inner).ServeHTTP(httptest.NewRecorder(), req)
}

func TestStamper_StampsOncePerInterval(t *testing.T) {
	rec := newRecordingRecorder()
	current := time.Unix(1_000_000, 0)
	s := New(rec, func() time.Time { return current })

	serveAs(t, s, "alice")
	serveAs(t, s, "alice") // within interval — deduped
	serveAs(t, s, "bob")
	rec.waitForCalls(t, 2)

	users := rec.stampedUsers()
	if len(users) != 2 {
		t.Fatalf("stamps = %v, want exactly [alice bob] in some order", users)
	}

	// Past the interval, alice stamps again.
	current = current.Add(stampMinInterval + time.Second)
	serveAs(t, s, "alice")
	rec.waitForCalls(t, 1)
	if got := len(rec.stampedUsers()); got != 3 {
		t.Errorf("stamps after interval = %d, want 3", got)
	}
}

func TestStamper_IgnoresUnauthenticated(t *testing.T) {
	rec := newRecordingRecorder()
	s := New(rec, nil)

	serveAs(t, s, "")

	select {
	case <-rec.done:
		t.Fatal("unauthenticated request must not stamp")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestStamper_FailureClearsCacheForRetry(t *testing.T) {
	rec := newRecordingRecorder()
	rec.err = errors.New("db down")
	current := time.Unix(1_000_000, 0)
	s := New(rec, func() time.Time { return current })

	serveAs(t, s, "alice")
	rec.waitForCalls(t, 1)

	// Recorder recovers; the very next request (still inside the
	// interval) retries because the failed stamp cleared the cache.
	rec.mu.Lock()
	rec.err = nil
	rec.mu.Unlock()
	current = current.Add(time.Second)
	serveAs(t, s, "alice")
	rec.waitForCalls(t, 1)

	if users := rec.stampedUsers(); len(users) != 1 || users[0] != "alice" {
		t.Errorf("stamps = %v, want [alice] after retry", users)
	}
}

func TestStamper_RequestNotBlockedBySlowWrite(t *testing.T) {
	block := make(chan struct{})
	rec := &blockingRecorder{block: block}
	s := New(rec, nil)

	done := make(chan struct{})
	go func() {
		serveAs(t, s, "alice")
		close(done)
	}()
	select {
	case <-done:
		// Request returned while the write is still blocked — async
		// confirmed.
	case <-time.After(5 * time.Second):
		t.Fatal("request blocked on the activity stamp write")
	}
	close(block)
}

type blockingRecorder struct{ block chan struct{} }

func (r *blockingRecorder) RecordUserActivity(context.Context, string, int64) error {
	<-r.block
	return nil
}
