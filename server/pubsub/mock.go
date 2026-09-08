package pubsub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// MockTopic is a Topic[T] for tests that need to capture publishes without
// running the real fan-out. It records every Publish call in a slice
// readable via Captured(). Subscribe is a no-op except for name-collision
// validation; Drain returns immediately.
//
// Use this in emitter unit tests to assert "the right event was published"
// without exercising subscriber wiring. For integration tests that need to
// observe subscriber behavior, use a real MemTopic with a FakeSubscriber.
type MockTopic[T any] struct {
	mu sync.Mutex

	captured    []T
	subscribers map[string]struct{}
	publishErr  error
}

// NewMockTopic constructs an empty MockTopic.
func NewMockTopic[T any]() *MockTopic[T] {
	return &MockTopic[T]{
		subscribers: make(map[string]struct{}),
	}
}

// Subscribe registers the subscriber name for collision detection only;
// the subscriber's Handle is never invoked.
func (m *MockTopic[T]) Subscribe(s Subscriber[T], _ ...SubscribeOption) (Unsubscribe, error) {
	if s == nil {
		return nil, errors.New("pubsub: nil subscriber")
	}
	name := s.Name()
	if name == "" {
		return nil, errors.New("pubsub: subscriber Name() must be non-empty")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.subscribers[name]; exists {
		return nil, fmt.Errorf("pubsub: subscriber %q already registered", name)
	}
	m.subscribers[name] = struct{}{}
	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			delete(m.subscribers, name)
			m.mu.Unlock()
		})
	}, nil
}

// Publish records event into the captured slice. Returns the configured
// publishErr (default nil). Does NOT invoke subscribers.
func (m *MockTopic[T]) Publish(_ context.Context, event T) error {
	m.mu.Lock()
	m.captured = append(m.captured, event)
	err := m.publishErr
	m.mu.Unlock()
	return err
}

// Drain is a no-op; MockTopic never fans out, so nothing is in flight.
func (m *MockTopic[T]) Drain(_ context.Context) error {
	return nil
}

// Captured returns a copy of the events that have been published.
func (m *MockTopic[T]) Captured() []T {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]T, len(m.captured))
	copy(out, m.captured)
	return out
}

// Reset clears the captured slice.
func (m *MockTopic[T]) Reset() {
	m.mu.Lock()
	m.captured = nil
	m.mu.Unlock()
}

// SetPublishError configures Publish to return err on every subsequent call.
// Pass nil to clear.
func (m *MockTopic[T]) SetPublishError(err error) {
	m.mu.Lock()
	m.publishErr = err
	m.mu.Unlock()
}

// FakeSubscriber is a Subscriber[T] that records received events and supports
// configurable behavior for testing failure modes. Used by both
// pubsub_test.go and downstream consumer tests (e.g. notification subscriber
// tests that want to assert the bus dispatched correctly).
type FakeSubscriber[T any] struct {
	NameVal string

	mu sync.Mutex

	received []T

	// HandleErr, if non-nil, is returned from Handle.
	HandleErr error
	// HandlePanic, if non-nil, is panicked from Handle.
	HandlePanic any
	// HandleDelay, if non-zero, is slept by Handle before returning. Used to
	// exercise backpressure (drives ingress channel saturation).
	HandleDelay time.Duration
	// HandleHook, if non-nil, runs at the start of each Handle call. Useful
	// for asserting on dispatch-ctx fields (request_id propagation, etc.).
	HandleHook func(ctx context.Context, event T)
}

// NewFakeSubscriber constructs a FakeSubscriber with the given name.
func NewFakeSubscriber[T any](name string) *FakeSubscriber[T] {
	return &FakeSubscriber[T]{NameVal: name}
}

// Name implements Subscriber.
func (f *FakeSubscriber[T]) Name() string {
	return f.NameVal
}

// Handle implements Subscriber.
func (f *FakeSubscriber[T]) Handle(ctx context.Context, event T) error {
	if f.HandleHook != nil {
		f.HandleHook(ctx, event)
	}
	if f.HandleDelay > 0 {
		select {
		case <-time.After(f.HandleDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if f.HandlePanic != nil {
		panic(f.HandlePanic)
	}
	f.mu.Lock()
	f.received = append(f.received, event)
	f.mu.Unlock()
	return f.HandleErr
}

// Received returns a copy of the events seen by this subscriber.
func (f *FakeSubscriber[T]) Received() []T {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]T, len(f.received))
	copy(out, f.received)
	return out
}
