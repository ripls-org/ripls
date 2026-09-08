package notifications

import (
	"context"
	"sync"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// MockService tracks notification calls without accessing the database.
// Useful for testing notification behavior without database side effects.
type MockService struct {
	mu                sync.Mutex
	notificationCalls []NotificationCall
	welcomeCalls      []WelcomeCall
	hasDevicesResult  bool
}

// NotificationCall represents a single notification call.
type NotificationCall struct {
	UserID       string
	Notification *models.Notification
}

// WelcomeCall represents a single SendPhoneOptInWelcome call.
type WelcomeCall struct {
	UserID string
	Phone  string
}

// NewMockService creates a new mock notification service.
func NewMockService() *MockService {
	return &MockService{
		hasDevicesResult: true, // Default to true for testing
	}
}

// NotifyUser records a notification call.
func (m *MockService) NotifyUser(ctx context.Context, userID string, notification *models.Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notificationCalls = append(m.notificationCalls, NotificationCall{
		UserID:       userID,
		Notification: notification,
	})
	return nil
}

// SendPhoneOptInWelcome records a welcome call.
func (m *MockService) SendPhoneOptInWelcome(ctx context.Context, user *models.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.welcomeCalls = append(m.welcomeCalls, WelcomeCall{
		UserID: user.GetId(),
		Phone:  user.GetPhoneNumber(),
	})
	return nil
}

// GetWelcomeCalls returns a copy of all SendPhoneOptInWelcome calls.
func (m *MockService) GetWelcomeCalls() []WelcomeCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]WelcomeCall{}, m.welcomeCalls...)
}

// HasDevices returns the configured result for testing.
func (m *MockService) HasDevices(ctx context.Context, userID string) bool {
	return m.hasDevicesResult
}

// UnregisterDevice is a no-op for the mock.
func (m *MockService) UnregisterDevice(ctx context.Context, deviceToken string) error {
	return nil
}

// GetCalls returns a copy of all notification calls.
func (m *MockService) GetCalls() []NotificationCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]NotificationCall{}, m.notificationCalls...)
}

// Reset clears all recorded notification calls.
func (m *MockService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notificationCalls = nil
	m.welcomeCalls = nil
}

// SetHasDevicesResult configures the return value for HasDevices.
func (m *MockService) SetHasDevicesResult(result bool) {
	m.hasDevicesResult = result
}

// Verify MockService implements the Service interface.
var _ Service = (*MockService)(nil)
