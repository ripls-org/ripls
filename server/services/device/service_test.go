package device

import (
	"context"
	"testing"

	"connectrpc.com/authn"
	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/notifications/noop"
	"go.ripls.org/ripls/server/storage"
)

func setupTestDeviceService(t *testing.T) (*Service, *storage.ProtoSQLStorage) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	// Create notification service with test provider
	provider := noop.NewProvider(models.DevicePlatform_DEVICE_PLATFORM_ANDROID)
	notifService := notifications.NewService([]notifications.Provider{provider}, sqlStorage)

	svc := New(sqlStorage, notifService)
	return svc, sqlStorage
}

func contextWithAuth(userID, email string) context.Context {
	authInfo := &auth.Info{
		UserID: userID,
		Email:  email,
		Role:   models.Role_ROLE_USER,
	}
	return authn.SetInfo(context.Background(), authInfo)
}

func TestRegisterDeviceToken_NewDevice(t *testing.T) {
	svc, _ := setupTestDeviceService(t)
	ctx := contextWithAuth("user123", "test@example.com")

	req := connect.NewRequest(&api.RegisterDeviceTokenRequest{
		DeviceToken: "token123",
		Platform:    api.DevicePlatform_DEVICE_PLATFORM_ANDROID,
	})

	resp, err := svc.RegisterDeviceToken(ctx, req)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if resp.Msg.DeviceId == "" {
		t.Error("Expected device ID to be returned")
	}
}

func TestRegisterDeviceToken_Idempotent(t *testing.T) {
	svc, _ := setupTestDeviceService(t)
	ctx := contextWithAuth("user123", "test@example.com")

	req := connect.NewRequest(&api.RegisterDeviceTokenRequest{
		DeviceToken: "token123",
		Platform:    api.DevicePlatform_DEVICE_PLATFORM_ANDROID,
	})

	// Register first time
	resp1, err := svc.RegisterDeviceToken(ctx, req)
	if err != nil {
		t.Fatalf("First registration failed: %v", err)
	}

	// Register again with same token
	resp2, err := svc.RegisterDeviceToken(ctx, req)
	if err != nil {
		t.Fatalf("Second registration failed: %v", err)
	}

	// Should return same device ID
	if resp1.Msg.DeviceId != resp2.Msg.DeviceId {
		t.Errorf("Expected same device ID, got %s and %s", resp1.Msg.DeviceId, resp2.Msg.DeviceId)
	}
}

func TestRegisterDeviceToken_TokenMigration(t *testing.T) {
	svc, sqlStorage := setupTestDeviceService(t)

	// User 1 registers a token
	ctx1 := contextWithAuth("user1", "user1@example.com")
	req := connect.NewRequest(&api.RegisterDeviceTokenRequest{
		DeviceToken: "shared_token",
		Platform:    api.DevicePlatform_DEVICE_PLATFORM_ANDROID,
	})

	resp1, err := svc.RegisterDeviceToken(ctx1, req)
	if err != nil {
		t.Fatalf("User 1 registration failed: %v", err)
	}

	// User 2 registers the same token (user switched accounts)
	ctx2 := contextWithAuth("user2", "user2@example.com")
	resp2, err := svc.RegisterDeviceToken(ctx2, req)
	if err != nil {
		t.Fatalf("User 2 registration failed: %v", err)
	}

	// Should get different device IDs
	if resp1.Msg.DeviceId == resp2.Msg.DeviceId {
		t.Error("Expected different device IDs for different users")
	}

	// Verify user 1's device was removed
	messages, err := sqlStorage.QueryByField(context.Background(), "user_id", "user1", &models.UserDevice{})
	if err != nil {
		t.Fatalf("Failed to query devices: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("Expected user 1's device to be removed, found %d devices", len(messages))
	}

	// Verify user 2's device exists
	messages, err = sqlStorage.QueryByField(context.Background(), "user_id", "user2", &models.UserDevice{})
	if err != nil {
		t.Fatalf("Failed to query devices: %v", err)
	}
	if len(messages) != 1 {
		t.Errorf("Expected 1 device for user 2, found %d", len(messages))
	}
}

func TestRegisterDeviceToken_MultiplePlatforms(t *testing.T) {
	svc, sqlStorage := setupTestDeviceService(t)
	ctx := contextWithAuth("user123", "test@example.com")

	platforms := []api.DevicePlatform{
		api.DevicePlatform_DEVICE_PLATFORM_ANDROID,
		api.DevicePlatform_DEVICE_PLATFORM_IOS,
		api.DevicePlatform_DEVICE_PLATFORM_WEB,
	}

	// Register devices on different platforms
	for i, platform := range platforms {
		req := connect.NewRequest(&api.RegisterDeviceTokenRequest{
			DeviceToken: string(rune('a'+i)) + "token123", // Different tokens
			Platform:    platform,
		})

		_, err := svc.RegisterDeviceToken(ctx, req)
		if err != nil {
			t.Fatalf("Failed to register device on platform %v: %v", platform, err)
		}
	}

	// Verify all devices are registered
	messages, err := sqlStorage.QueryByField(context.Background(), "user_id", "user123", &models.UserDevice{})
	if err != nil {
		t.Fatalf("Failed to query devices: %v", err)
	}
	if len(messages) != len(platforms) {
		t.Errorf("Expected %d devices, found %d", len(platforms), len(messages))
	}
}

func TestRegisterDeviceToken_RequiresAuth(t *testing.T) {
	svc, _ := setupTestDeviceService(t)
	ctx := context.Background() // No auth

	req := connect.NewRequest(&api.RegisterDeviceTokenRequest{
		DeviceToken: "token123",
		Platform:    api.DevicePlatform_DEVICE_PLATFORM_ANDROID,
	})

	_, err := svc.RegisterDeviceToken(ctx, req)
	if err == nil {
		t.Fatal("Expected error for unauthenticated request, got nil")
	}

	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("Expected Unauthenticated error, got: %v", err)
	}
}

func TestUnregisterDeviceToken(t *testing.T) {
	svc, sqlStorage := setupTestDeviceService(t)
	ctx := contextWithAuth("user123", "test@example.com")

	// Register a device
	registerReq := connect.NewRequest(&api.RegisterDeviceTokenRequest{
		DeviceToken: "token123",
		Platform:    api.DevicePlatform_DEVICE_PLATFORM_ANDROID,
	})

	_, err := svc.RegisterDeviceToken(ctx, registerReq)
	if err != nil {
		t.Fatalf("Failed to register device: %v", err)
	}

	// Verify device exists
	messages, err := sqlStorage.QueryByField(context.Background(), "user_id", "user123", &models.UserDevice{})
	if err != nil {
		t.Fatalf("Failed to query devices: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("Expected 1 device, found %d", len(messages))
	}

	// Unregister the device
	unregisterReq := connect.NewRequest(&api.UnregisterDeviceTokenRequest{
		DeviceToken: "token123",
	})

	_, err = svc.UnregisterDeviceToken(ctx, unregisterReq)
	if err != nil {
		t.Fatalf("Failed to unregister device: %v", err)
	}

	// Verify device is removed
	messages, err = sqlStorage.QueryByField(context.Background(), "user_id", "user123", &models.UserDevice{})
	if err != nil {
		t.Fatalf("Failed to query devices: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("Expected device to be removed, found %d devices", len(messages))
	}
}

func TestUnregisterDeviceToken_Idempotent(t *testing.T) {
	svc, _ := setupTestDeviceService(t)
	ctx := contextWithAuth("user123", "test@example.com")

	req := connect.NewRequest(&api.UnregisterDeviceTokenRequest{
		DeviceToken: "nonexistent_token",
	})

	// Unregister non-existent device (should not error)
	_, err := svc.UnregisterDeviceToken(ctx, req)
	if err != nil {
		t.Errorf("Expected no error for unregistering non-existent device, got: %v", err)
	}

	// Try again (should still not error)
	_, err = svc.UnregisterDeviceToken(ctx, req)
	if err != nil {
		t.Errorf("Expected no error for second unregister, got: %v", err)
	}
}

func TestUnregisterDeviceToken_RequiresAuth(t *testing.T) {
	svc, _ := setupTestDeviceService(t)
	ctx := context.Background() // No auth

	req := connect.NewRequest(&api.UnregisterDeviceTokenRequest{
		DeviceToken: "token123",
	})

	_, err := svc.UnregisterDeviceToken(ctx, req)
	if err == nil {
		t.Fatal("Expected error for unauthenticated request, got nil")
	}

	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("Expected Unauthenticated error, got: %v", err)
	}
}

func TestConvertPlatform(t *testing.T) {
	tests := []struct {
		apiPlatform    api.DevicePlatform
		modelsPlatform models.DevicePlatform
	}{
		{api.DevicePlatform_DEVICE_PLATFORM_ANDROID, models.DevicePlatform_DEVICE_PLATFORM_ANDROID},
		{api.DevicePlatform_DEVICE_PLATFORM_IOS, models.DevicePlatform_DEVICE_PLATFORM_IOS},
		{api.DevicePlatform_DEVICE_PLATFORM_WEB, models.DevicePlatform_DEVICE_PLATFORM_WEB},
		{api.DevicePlatform_DEVICE_PLATFORM_UNSPECIFIED, models.DevicePlatform_DEVICE_PLATFORM_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.apiPlatform.String(), func(t *testing.T) {
			result := convertPlatform(tt.apiPlatform)
			if result != tt.modelsPlatform {
				t.Errorf("Expected %v, got %v", tt.modelsPlatform, result)
			}
		})
	}
}
