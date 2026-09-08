package noop

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
)

func TestNewProvider(t *testing.T) {
	provider := NewProvider(models.DevicePlatform_DEVICE_PLATFORM_ANDROID)

	if provider == nil {
		t.Fatal("Expected provider to be created, got nil")
	}

	if provider.platform != models.DevicePlatform_DEVICE_PLATFORM_ANDROID {
		t.Errorf("Expected platform ANDROID, got %v", provider.platform)
	}
}

func TestPlatform(t *testing.T) {
	tests := []struct {
		name     string
		platform models.DevicePlatform
	}{
		{"Android", models.DevicePlatform_DEVICE_PLATFORM_ANDROID},
		{"iOS", models.DevicePlatform_DEVICE_PLATFORM_IOS},
		{"Web", models.DevicePlatform_DEVICE_PLATFORM_WEB},
		{"Unspecified", models.DevicePlatform_DEVICE_PLATFORM_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewProvider(tt.platform)
			if provider.Platform() != tt.platform {
				t.Errorf("Expected platform %v, got %v", tt.platform, provider.Platform())
			}
		})
	}
}

func TestSend_LogsNotification(t *testing.T) {
	provider := NewProvider(models.DevicePlatform_DEVICE_PLATFORM_ANDROID)
	ctx := context.Background()

	notification := &models.Notification{
		Title: "Test Notification",
		Body:  "This is a test",
		Payload: &models.Notification_CommunityEvent{
			CommunityEvent: &models.CommunityEventPayload{
				CommunityId: "comm123",
				EventId:     "event456",
				EventType:   "GEAR_SHARED",
			},
		},
	}

	// Should not error - just log and return
	err := provider.Send(ctx, notifications.Target{DeviceToken: "test_token_123"}, notification)
	if err != nil {
		t.Errorf("Expected no error from no-op provider, got: %v", err)
	}
}

func TestSend_EmptyToken(t *testing.T) {
	provider := NewProvider(models.DevicePlatform_DEVICE_PLATFORM_ANDROID)
	ctx := context.Background()

	notification := &models.Notification{
		Title: "Test",
		Body:  "Test",
	}

	err := provider.Send(ctx, notifications.Target{DeviceToken: ""}, notification)
	if err != nil {
		t.Errorf("Expected no error with empty token, got: %v", err)
	}
}

func TestSend_NilPayload(t *testing.T) {
	provider := NewProvider(models.DevicePlatform_DEVICE_PLATFORM_ANDROID)
	ctx := context.Background()

	notification := &models.Notification{
		Title:   "Test",
		Body:    "Test",
		Payload: nil,
	}

	err := provider.Send(ctx, notifications.Target{DeviceToken: "token123"}, notification)
	if err != nil {
		t.Errorf("Expected no error with nil payload, got: %v", err)
	}
}

func TestSend_DifferentTokens(t *testing.T) {
	provider := NewProvider(models.DevicePlatform_DEVICE_PLATFORM_ANDROID)
	ctx := context.Background()

	tests := []struct {
		name  string
		token string
	}{
		{"Long token", "abcdefghij1234567890klmnopqrst"},
		{"Short token", "short"},
		{"Exactly 20 chars", "12345678901234567890"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notification := &models.Notification{
				Title: "Test",
				Body:  "Test",
			}

			err := provider.Send(ctx, notifications.Target{DeviceToken: tt.token}, notification)
			if err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}

func TestSend_DifferentPayloadTypes(t *testing.T) {
	provider := NewProvider(models.DevicePlatform_DEVICE_PLATFORM_ANDROID)
	ctx := context.Background()

	tests := []struct {
		name         string
		notification *models.Notification
	}{
		{
			name: "CommunityEvent payload",
			notification: &models.Notification{
				Title: "Community Event",
				Body:  "Test",
				Payload: &models.Notification_CommunityEvent{
					CommunityEvent: &models.CommunityEventPayload{
						CommunityId: "comm123",
						EventId:     "event456",
					},
				},
			},
		},
		{
			name: "ChatMessage payload",
			notification: &models.Notification{
				Title: "Chat Message",
				Body:  "Test",
				Payload: &models.Notification_ChatMessage{
					ChatMessage: &models.ChatMessagePayload{
						ConversationId: "conv123",
						MessageId:      "msg456",
						SenderUserId:   "user789",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provider.Send(ctx, notifications.Target{DeviceToken: "token123"}, tt.notification)
			if err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}
