package notifications

import (
	"context"
	"errors"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// mockProvider is a test provider that tracks calls.
type mockProvider struct {
	platform    models.DevicePlatform
	sendCalls   []sendCall
	shouldError bool
	errorType   error
	// tokenErrors overrides errorType per device token when set, so tests can
	// simulate a mix of failures (e.g. one invalid token + one transient error).
	tokenErrors map[string]error
}

type sendCall struct {
	token        string
	notification *models.Notification
}

func (m *mockProvider) Send(ctx context.Context, target Target, notification *models.Notification) error {
	m.sendCalls = append(m.sendCalls, sendCall{
		token:        target.DeviceToken,
		notification: notification,
	})
	if err, ok := m.tokenErrors[target.DeviceToken]; ok {
		return err
	}
	if m.shouldError {
		return m.errorType
	}
	return nil
}

func (m *mockProvider) Platform() models.DevicePlatform {
	return m.platform
}

func setupTestService(t *testing.T) (*service, *storage.ProtoSQLStorage, *mockProvider) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	// Create mock provider
	provider := &mockProvider{
		platform: models.DevicePlatform_DEVICE_PLATFORM_ANDROID,
	}

	// Create service
	svc := NewService([]Provider{provider}, sqlStorage).(*service)

	return svc, sqlStorage, provider
}

func TestNotifyUser_NoDevices(t *testing.T) {
	svc, _, provider := setupTestService(t)

	notification := &models.Notification{
		Title: "Test",
		Body:  "Test notification",
	}

	err := svc.NotifyUser(context.Background(), "user123", notification)
	if err != nil {
		t.Errorf("Expected no error for user with no devices, got: %v", err)
	}

	if len(provider.sendCalls) != 0 {
		t.Errorf("Expected no send calls for user with no devices, got %d", len(provider.sendCalls))
	}
}

func TestNotifyUser_SingleDevice(t *testing.T) {
	svc, sqlStorage, provider := setupTestService(t)
	ctx := context.Background()

	// Register a device
	device := &models.UserDevice{
		UserId:           "user123",
		DeviceToken:      "token123",
		Platform:         models.DevicePlatform_DEVICE_PLATFORM_ANDROID,
		CreatedAtUnixSec: 1000,
	}
	_, err := sqlStorage.Insert(ctx, device)
	if err != nil {
		t.Fatalf("Failed to insert device: %v", err)
	}

	// Send notification
	notification := &models.Notification{
		Title: "Test",
		Body:  "Test notification",
		Payload: &models.Notification_CommunityEvent{
			CommunityEvent: &models.CommunityEventPayload{
				CommunityId: "comm123",
				EventId:     "event123",
			},
		},
	}

	err = svc.NotifyUser(ctx, "user123", notification)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(provider.sendCalls) != 1 {
		t.Fatalf("Expected 1 send call, got %d", len(provider.sendCalls))
	}

	call := provider.sendCalls[0]
	if call.token != "token123" {
		t.Errorf("Expected token 'token123', got '%s'", call.token)
	}
	if call.notification.Title != "Test" {
		t.Errorf("Expected title 'Test', got '%s'", call.notification.Title)
	}
}

func TestNotifyUser_MultipleDevices(t *testing.T) {
	svc, sqlStorage, provider := setupTestService(t)
	ctx := context.Background()

	// Register multiple devices for same user
	devices := []*models.UserDevice{
		{
			UserId:           "user123",
			DeviceToken:      "token1",
			Platform:         models.DevicePlatform_DEVICE_PLATFORM_ANDROID,
			CreatedAtUnixSec: 1000,
		},
		{
			UserId:           "user123",
			DeviceToken:      "token2",
			Platform:         models.DevicePlatform_DEVICE_PLATFORM_ANDROID,
			CreatedAtUnixSec: 1001,
		},
	}

	for _, device := range devices {
		_, err := sqlStorage.Insert(ctx, device)
		if err != nil {
			t.Fatalf("Failed to insert device: %v", err)
		}
	}

	// Send notification
	notification := &models.Notification{
		Title: "Test",
		Body:  "Test notification",
	}

	err := svc.NotifyUser(ctx, "user123", notification)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(provider.sendCalls) != 2 {
		t.Fatalf("Expected 2 send calls, got %d", len(provider.sendCalls))
	}

	// Verify both tokens were called
	tokens := map[string]bool{}
	for _, call := range provider.sendCalls {
		tokens[call.token] = true
	}
	if !tokens["token1"] || !tokens["token2"] {
		t.Errorf("Expected both tokens to be called, got: %v", tokens)
	}
}

func TestNotifyUser_InvalidToken_RemovesDevice(t *testing.T) {
	svc, sqlStorage, provider := setupTestService(t)
	ctx := context.Background()

	// Register a device
	device := &models.UserDevice{
		UserId:           "user123",
		DeviceToken:      "invalid_token",
		Platform:         models.DevicePlatform_DEVICE_PLATFORM_ANDROID,
		CreatedAtUnixSec: 1000,
	}
	deviceID, err := sqlStorage.Insert(ctx, device)
	if err != nil {
		t.Fatalf("Failed to insert device: %v", err)
	}
	device.Id = deviceID

	// Configure provider to return invalid token error
	provider.shouldError = true
	provider.errorType = ErrInvalidToken

	// Send notification
	notification := &models.Notification{
		Title: "Test",
		Body:  "Test notification",
	}

	err = svc.NotifyUser(ctx, "user123", notification)
	// When every failure is an invalid-token cleanup, the system has
	// self-healed — no error should propagate to the caller.
	if err != nil {
		t.Errorf("Expected nil error when all failures are invalid-token cleanups, got: %v", err)
	}

	// Verify device was removed from storage
	devices, err := svc.getUserDevices(ctx, "user123")
	if err != nil {
		t.Fatalf("Failed to get devices: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("Expected device to be removed, found %d devices", len(devices))
	}
}

func TestNotifyUser_AllDevicesFail_MixedErrors_ReturnsError(t *testing.T) {
	svc, sqlStorage, provider := setupTestService(t)
	ctx := context.Background()

	invalidDevice := &models.UserDevice{
		UserId:           "user123",
		DeviceToken:      "invalid_token",
		Platform:         models.DevicePlatform_DEVICE_PLATFORM_ANDROID,
		CreatedAtUnixSec: 1000,
	}
	transientDevice := &models.UserDevice{
		UserId:           "user123",
		DeviceToken:      "transient_token",
		Platform:         models.DevicePlatform_DEVICE_PLATFORM_ANDROID,
		CreatedAtUnixSec: 1001,
	}
	if _, err := sqlStorage.Insert(ctx, invalidDevice); err != nil {
		t.Fatalf("Failed to insert invalid-token device: %v", err)
	}
	if _, err := sqlStorage.Insert(ctx, transientDevice); err != nil {
		t.Fatalf("Failed to insert transient-failure device: %v", err)
	}

	// One device has a dead token (self-healing), the other hit a transient
	// network error (real delivery problem). The presence of even one
	// non-invalid-token failure must surface an ERROR to the caller — the
	// user did not receive the notification and the transient device is
	// still registered, so future sends to it will keep failing until the
	// underlying issue is resolved.
	provider.tokenErrors = map[string]error{
		"invalid_token":   ErrInvalidToken,
		"transient_token": errors.New("FCM transient network error"),
	}

	notification := &models.Notification{Title: "Test", Body: "Test notification"}
	err := svc.NotifyUser(ctx, "user123", notification)
	if err == nil {
		t.Error("Expected error when failures include a non-invalid-token error, got nil")
	}

	// The invalid-token device should still be cleaned up; the transient
	// device must remain registered.
	devices, err := svc.getUserDevices(ctx, "user123")
	if err != nil {
		t.Fatalf("Failed to get devices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("Expected 1 device remaining (transient), got %d", len(devices))
	}
	if devices[0].DeviceToken != "transient_token" {
		t.Errorf("Expected transient device to remain, got token %q", devices[0].DeviceToken)
	}
}

func TestNotifyUser_AllDevicesFail_NonInvalidToken_ReturnsError(t *testing.T) {
	svc, sqlStorage, provider := setupTestService(t)
	ctx := context.Background()

	device := &models.UserDevice{
		UserId:           "user123",
		DeviceToken:      "token123",
		Platform:         models.DevicePlatform_DEVICE_PLATFORM_ANDROID,
		CreatedAtUnixSec: 1000,
	}
	if _, err := sqlStorage.Insert(ctx, device); err != nil {
		t.Fatalf("Failed to insert device: %v", err)
	}

	// A non-invalid-token failure (e.g. FCM outage, network error) is a real
	// delivery problem and must surface to the caller.
	provider.shouldError = true
	provider.errorType = errors.New("FCM transient network error")

	notification := &models.Notification{Title: "Test", Body: "Test notification"}
	err := svc.NotifyUser(ctx, "user123", notification)
	if err == nil {
		t.Error("Expected error when all devices fail with non-invalid-token errors, got nil")
	}

	// Device must NOT have been unregistered — it's not known to be bad.
	devices, err := svc.getUserDevices(ctx, "user123")
	if err != nil {
		t.Fatalf("Failed to get devices: %v", err)
	}
	if len(devices) != 1 {
		t.Errorf("Expected device to remain registered after transient failure, found %d devices", len(devices))
	}
}

func TestNotifyUser_NoProviderForPlatform(t *testing.T) {
	svc, sqlStorage, _ := setupTestService(t)
	ctx := context.Background()

	// Register device with platform that has no provider
	device := &models.UserDevice{
		UserId:           "user123",
		DeviceToken:      "token123",
		Platform:         models.DevicePlatform_DEVICE_PLATFORM_IOS, // No iOS provider
		CreatedAtUnixSec: 1000,
	}
	_, err := sqlStorage.Insert(ctx, device)
	if err != nil {
		t.Fatalf("Failed to insert device: %v", err)
	}

	notification := &models.Notification{
		Title: "Test",
		Body:  "Test notification",
	}

	// Should not error, just skip the device
	err = svc.NotifyUser(ctx, "user123", notification)
	if err != nil {
		t.Errorf("Expected no error when no provider exists, got: %v", err)
	}
}

func TestHasDevices(t *testing.T) {
	svc, sqlStorage, _ := setupTestService(t)
	ctx := context.Background()

	// User with no devices
	if svc.HasDevices(ctx, "user123") {
		t.Error("Expected HasDevices to return false for user with no devices")
	}

	// Register a device
	device := &models.UserDevice{
		UserId:           "user123",
		DeviceToken:      "token123",
		Platform:         models.DevicePlatform_DEVICE_PLATFORM_ANDROID,
		CreatedAtUnixSec: 1000,
	}
	_, err := sqlStorage.Insert(ctx, device)
	if err != nil {
		t.Fatalf("Failed to insert device: %v", err)
	}

	// User with devices
	if !svc.HasDevices(ctx, "user123") {
		t.Error("Expected HasDevices to return true for user with devices")
	}
}

func TestUnregisterDevice(t *testing.T) {
	svc, sqlStorage, _ := setupTestService(t)
	ctx := context.Background()

	// Register a device
	device := &models.UserDevice{
		UserId:           "user123",
		DeviceToken:      "token123",
		Platform:         models.DevicePlatform_DEVICE_PLATFORM_ANDROID,
		CreatedAtUnixSec: 1000,
	}
	deviceID, err := sqlStorage.Insert(ctx, device)
	if err != nil {
		t.Fatalf("Failed to insert device: %v", err)
	}
	device.Id = deviceID

	// Unregister device
	err = svc.UnregisterDevice(ctx, "token123")
	if err != nil {
		t.Fatalf("Failed to unregister device: %v", err)
	}

	// Verify device was removed
	devices, err := svc.getUserDevices(ctx, "user123")
	if err != nil {
		t.Fatalf("Failed to get devices: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("Expected device to be removed, found %d devices", len(devices))
	}
}

func TestUnregisterDevice_NotFound(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	// Unregister non-existent device (should be idempotent)
	err := svc.UnregisterDevice(ctx, "nonexistent_token")
	if err != nil {
		t.Errorf("Expected no error for unregistering non-existent device, got: %v", err)
	}
}

// TestUnregisterDevice_ConcurrentRace covers the race where two callers
// simultaneously try to unregister the same device token (e.g. two NotifyUser
// calls cleaning up the same invalid FCM token). Both should pass the lookup,
// only one should win the DELETE, and the loser must return nil rather than
// surfacing storage.ErrRecordNotFound — the cleanup goal is already achieved.
// Regression test for issue #1690.
func TestUnregisterDevice_ConcurrentRace(t *testing.T) {
	svc, sqlStorage, _ := setupTestService(t)
	ctx := context.Background()

	device := &models.UserDevice{
		UserId:           "user_race",
		DeviceToken:      "race_token",
		Platform:         models.DevicePlatform_DEVICE_PLATFORM_ANDROID,
		CreatedAtUnixSec: 1000,
	}
	if _, err := sqlStorage.Insert(ctx, device); err != nil {
		t.Fatalf("Failed to insert device: %v", err)
	}

	const concurrency = 4
	errs := make(chan error, concurrency)
	for range concurrency {
		go func() {
			errs <- svc.UnregisterDevice(ctx, "race_token")
		}()
	}

	for i := range concurrency {
		if err := <-errs; err != nil {
			t.Errorf("UnregisterDevice race caller %d returned error: %v", i, err)
		}
	}

	devices, err := svc.getUserDevices(ctx, "user_race")
	if err != nil {
		t.Fatalf("Failed to get devices after race: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("Expected device to be removed, found %d devices", len(devices))
	}
}

func TestProviderMapLookup(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	// Create providers for different platforms
	androidProvider := &mockProvider{platform: models.DevicePlatform_DEVICE_PLATFORM_ANDROID}
	iosProvider := &mockProvider{platform: models.DevicePlatform_DEVICE_PLATFORM_IOS}

	svc := NewService([]Provider{androidProvider, iosProvider}, sqlStorage).(*service)

	// Verify provider map was built correctly
	if len(svc.providers) != 2 {
		t.Errorf("Expected 2 providers in map, got %d", len(svc.providers))
	}

	androidResult, exists := svc.providers[models.DevicePlatform_DEVICE_PLATFORM_ANDROID]
	if !exists || androidResult != androidProvider {
		t.Error("Android provider not found in map")
	}

	iosResult, exists := svc.providers[models.DevicePlatform_DEVICE_PLATFORM_IOS]
	if !exists || iosResult != iosProvider {
		t.Error("iOS provider not found in map")
	}
}
