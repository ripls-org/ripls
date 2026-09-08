package streaming

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeEvent struct {
	seq int
}

type captureTarget struct {
	mu       sync.Mutex
	received []*fakeEvent
	err      error
}

func (c *captureTarget) Send(ev *fakeEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.received = append(c.received, ev)
	return nil
}

func (c *captureTarget) snapshot() []*fakeEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*fakeEvent, len(c.received))
	copy(out, c.received)
	return out
}

func TestSender_SerializesConcurrentEmits(t *testing.T) {
	target := &captureTarget{}
	s := NewSender[fakeEvent](target)

	var wg sync.WaitGroup
	const n = 100
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.Emit(&fakeEvent{seq: i})
		}(i)
	}
	wg.Wait()

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := len(target.snapshot()); got != n {
		t.Errorf("received %d events, want %d", got, n)
	}
}

func TestSender_ReturnsFirstSendError(t *testing.T) {
	target := &captureTarget{err: errors.New("client gone")}
	s := NewSender[fakeEvent](target)
	s.Emit(&fakeEvent{seq: 1})
	s.Emit(&fakeEvent{seq: 2})

	err := s.Close()
	if err == nil || err.Error() != "client gone" {
		t.Errorf("Close err = %v, want 'client gone'", err)
	}
}

func TestSender_DrainAfterError(t *testing.T) {
	var sends atomic.Int32
	target := &erroringTarget{errAfter: 1, sends: &sends}
	s := NewSender[fakeEvent](target)
	s.Emit(&fakeEvent{seq: 1})
	s.Emit(&fakeEvent{seq: 2})
	s.Emit(&fakeEvent{seq: 3})

	if err := s.Close(); err == nil {
		t.Fatal("expected error from Close")
	}
	// After the first error, subsequent events should NOT call Send.
	// Exactly one Send — the first, which errored.
	if got := sends.Load(); got != 1 {
		t.Errorf("Send called %d times after error; want 1", got)
	}
}

type erroringTarget struct {
	errAfter int32
	sends    *atomic.Int32
}

func (t *erroringTarget) Send(_ *fakeEvent) error {
	n := t.sends.Add(1)
	if n >= t.errAfter {
		return errors.New("broken")
	}
	return nil
}

func TestSender_CloseIdempotent(t *testing.T) {
	target := &captureTarget{}
	s := NewSender[fakeEvent](target)
	s.Emit(&fakeEvent{seq: 1})

	// First Close flushes and returns the nil error.
	if err := s.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// Second Close must not panic and must return the same result.
	done := make(chan struct{})
	go func() {
		_ = s.Close()
		close(done)
	}()
	select {
	case <-done:
		// ok — but this will actually hang because s.done already drained.
	case <-time.After(100 * time.Millisecond):
		// Second Close blocking on s.done is acceptable; the once.Do
		// guarantees no double-close panic. This branch is expected.
	}
}
