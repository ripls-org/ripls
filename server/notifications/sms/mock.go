package sms

import (
	"context"
	"sync"
)

// SentSMS captures one delivery for assertion in tests.
type SentSMS struct {
	To   string
	Body string
}

// MockSMSSender records every SendSMS call instead of contacting Twilio. It is
// the dev/test stand-in (analogous to the noop notification provider) and is
// safe for concurrent use. Set Err to force every send to fail.
type MockSMSSender struct {
	mu   sync.Mutex
	Sent []SentSMS
	Err  error
}

// SendSMS records the message (or returns the configured error).
func (m *MockSMSSender) SendSMS(_ context.Context, toE164, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	m.Sent = append(m.Sent, SentSMS{To: toE164, Body: body})
	return nil
}

// Messages returns a copy of the captured sends.
func (m *MockSMSSender) Messages() []SentSMS {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]SentSMS, len(m.Sent))
	copy(out, m.Sent)
	return out
}
