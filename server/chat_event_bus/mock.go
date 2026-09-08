package chat_event_bus

import (
	"context"
	"sync"
	"sync/atomic"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// captured records a single Publish call for test assertions.
type captured struct {
	Kind         Kind
	Message      *models.ChatMessage
	Conversation *models.ChatConversation
	Opts         []PublishOption
}

// MockBus is a Publisher for tests that need to assert "the right event was
// published" without running real fan-out. It records every Publish call in a
// slice readable via Captured().
//
// For tests that DO want to observe subscriber behavior, use NewInProcessBus
// with a real pubsub.MemTopic and a pubsub.FakeSubscriber instead.
type MockBus struct {
	mu sync.Mutex

	calls      []captured
	idCounter  atomic.Int64
	publishErr error
}

// NewMockBus constructs an empty MockBus.
func NewMockBus() *MockBus {
	return &MockBus{}
}

// Publish records the call. If SetPublishError has been configured, returns
// that error and does NOT record the call.
func (m *MockBus) Publish(_ context.Context, kind Kind, msg *models.ChatMessage, conversation *models.ChatConversation, opts ...PublishOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.publishErr != nil {
		return m.publishErr
	}
	m.idCounter.Add(1)
	m.calls = append(m.calls, captured{
		Kind:         kind,
		Message:      msg,
		Conversation: conversation,
		Opts:         opts,
	})
	return nil
}

// Captured returns a copy of the recorded calls.
func (m *MockBus) Captured() []captured {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]captured, len(m.calls))
	copy(out, m.calls)
	return out
}

// Reset clears the captured slice and resets the counter.
func (m *MockBus) Reset() {
	m.mu.Lock()
	m.calls = nil
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

// LastKind returns the kind from the most recent Publish call, or 0 if no
// calls have been made.
func (m *MockBus) LastKind() Kind {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return 0
	}
	return m.calls[len(m.calls)-1].Kind
}

// CallCount returns the number of Publish calls recorded.
func (m *MockBus) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

var _ Publisher = (*MockBus)(nil)
