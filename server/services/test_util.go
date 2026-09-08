package services

import (
	"context"
	"os"
	"testing"
	"time"

	"go.ripls.org/ripls/server/health"
)

// WaitForNotification waits for a notification signal with a timeout.
// Fails the test if the notification doesn't arrive within 10 seconds.
func WaitForNotification(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
		// Notification received
	case <-time.After(10 * time.Second):
		t.Fatal("Timed out waiting for notification (10s)")
	}
}

// WaitForStockImagery waits for a stock imagery fetch signal with a timeout.
// Fails the test if the signal doesn't arrive within 10 seconds.
func WaitForStockImagery(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
		// Stock imagery fetch completed
	case <-time.After(10 * time.Second):
		t.Fatal("Timed out waiting for stock imagery fetch (10s)")
	}
}

// WaitForSuggestionChips waits for a suggestion-chips goroutine completion signal.
// Fails the test if the signal doesn't arrive within 10 seconds.
func WaitForSuggestionChips(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
		// Suggestion chips goroutine completed
	case <-time.After(10 * time.Second):
		t.Fatal("Timed out waiting for suggestion chips goroutine (10s)")
	}
}

// MockBucketStorage is a mock implementation of storage.BucketStorage for testing.
type MockBucketStorage struct {
	PutCalled    bool
	DeleteCalled bool
	LastKey      string
	LastData     []byte
	ReturnURL    string
	SignedURL    string
	// DurableSignedURL is returned by GetDurableSignedURL. When empty it falls
	// back to SignedURL, so tests that don't care about the distinction need
	// only set SignedURL.
	DurableSignedURL string
}

// Put stores media and returns a mock URL.
func (m *MockBucketStorage) Put(ctx context.Context, key string, data []byte, contentType string, metadata map[string]string) (string, error) {
	m.PutCalled = true
	m.LastKey = key
	m.LastData = data
	if m.ReturnURL == "" {
		return "mock://bucket/" + key, nil
	}
	return m.ReturnURL, nil
}

// Get retrieves mock data.
func (m *MockBucketStorage) Get(ctx context.Context, key string) ([]byte, string, error) {
	return []byte("mock data"), "image/jpeg", nil
}

// GetToFile writes mock data to destPath.
func (m *MockBucketStorage) GetToFile(_ context.Context, key, destPath string) (string, error) {
	if err := os.WriteFile(destPath, []byte("mock data"), 0o600); err != nil {
		return "", err
	}
	return "image/jpeg", nil
}

// PutFromFile records the key and returns a mock URL without reading the file.
func (m *MockBucketStorage) PutFromFile(_ context.Context, key, _, _ string, _ map[string]string) (string, error) {
	m.PutCalled = true
	m.LastKey = key
	if m.ReturnURL == "" {
		return "mock://bucket/" + key, nil
	}
	return m.ReturnURL, nil
}

// GetSignedURL generates a mock presigned URL.
func (m *MockBucketStorage) GetSignedURL(ctx context.Context, key string, duration time.Duration) (string, error) {
	if m.SignedURL == "" {
		return "http://mock-url/" + key, nil
	}
	return m.SignedURL, nil
}

// GetDurableSignedURL generates a mock durable presigned URL. Falls back to the
// GetSignedURL behavior when DurableSignedURL is unset.
func (m *MockBucketStorage) GetDurableSignedURL(ctx context.Context, key string, duration time.Duration) (string, error) {
	if m.DurableSignedURL != "" {
		return m.DurableSignedURL, nil
	}
	return m.GetSignedURL(ctx, key, duration)
}

// Copy returns a mock storage URL and zero size without performing any I/O.
func (m *MockBucketStorage) Copy(_ context.Context, _, dstKey string) (string, int64, error) {
	return "mock://bucket/" + dstKey, 0, nil
}

// Delete marks deletion as called for the mock.
func (m *MockBucketStorage) Delete(ctx context.Context, key string) error {
	m.DeleteCalled = true
	return nil
}

// CheckHealth returns mock health status.
func (m *MockBucketStorage) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	return []*health.Status{{Name: "bucket", Backend: "mock"}}, nil
}
