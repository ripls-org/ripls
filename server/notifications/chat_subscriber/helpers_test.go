package chat_subscriber

import (
	"context"
	"sync"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// mockNotificationService implements notifications.Service for tests.
type mockNotificationService struct {
	mu            sync.Mutex
	notifications []notifRecord
}

type notifRecord struct {
	userID       string
	notification *models.Notification
}

func (m *mockNotificationService) NotifyUser(_ context.Context, userID string, n *models.Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, notifRecord{userID: userID, notification: n})
	return nil
}

func (m *mockNotificationService) HasDevices(_ context.Context, _ string) bool {
	return true
}

func (m *mockNotificationService) UnregisterDevice(_ context.Context, _ string) error {
	return nil
}

func (m *mockNotificationService) SendPhoneOptInWelcome(_ context.Context, _ *models.User) error {
	return nil
}

func (m *mockNotificationService) dispatched() []notifRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]notifRecord, len(m.notifications))
	copy(out, m.notifications)
	return out
}
