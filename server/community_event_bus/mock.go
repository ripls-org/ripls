package community_event_bus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// MockBus is a Publisher for tests that need to assert "the right event was
// published" without running the real fan-out. It records every Publish
// call, generates synthetic event IDs (so callers' downstream paths that
// thread the ID through still exercise that wiring), and returns those IDs
// from Publish.
//
// For tests that DO want to observe subscriber behavior, use
// NewInProcessBus with a real pubsub.MemTopic and a pubsub.FakeSubscriber
// instead.
type MockBus struct {
	mu sync.Mutex

	captured  []*models.CommunityEvent
	idCounter atomic.Int64

	publishErr error
}

// NewMockBus constructs an empty MockBus. The first Publish returns event ID
// "evt-1", the second "evt-2", and so on, unless the test calls
// SetNextEventID before each Publish.
func NewMockBus() *MockBus {
	return &MockBus{}
}

// Publish records event into the captured slice and returns a synthetic ID.
// If SetPublishError has been configured, returns that error and does NOT
// record the event (matches the production behavior where insert failure
// suppresses dispatch and surfaces an error).
func (m *MockBus) Publish(_ context.Context, event *models.CommunityEvent) (string, error) {
	m.mu.Lock()
	if m.publishErr != nil {
		err := m.publishErr
		m.mu.Unlock()
		return "", err
	}
	id := fmt.Sprintf("evt-%d", m.idCounter.Add(1))
	if event != nil {
		event.Id = id
	}
	m.captured = append(m.captured, event)
	m.mu.Unlock()
	return id, nil
}

// Captured returns a copy of the recorded events.
func (m *MockBus) Captured() []*models.CommunityEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*models.CommunityEvent, len(m.captured))
	copy(out, m.captured)
	return out
}

// Reset clears the captured slice and ID counter.
func (m *MockBus) Reset() {
	m.mu.Lock()
	m.captured = nil
	m.idCounter.Store(0)
	m.publishErr = nil
	m.mu.Unlock()
}

// SetPublishError configures Publish to return err on every subsequent call.
// Pass nil to clear.
func (m *MockBus) SetPublishError(err error) {
	m.mu.Lock()
	m.publishErr = err
	m.mu.Unlock()
}
