package fcm

import (
	"context"
	"errors"
	"testing"

	"firebase.google.com/go/v4/messaging"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
)

// mockSender implements messageSender for testing.
type mockSender struct {
	responses []sendResult // sequential results for each call
	callCount int
}

type sendResult struct {
	id  string
	err error
}

func (m *mockSender) Send(_ context.Context, _ *messaging.Message) (string, error) {
	if m.callCount >= len(m.responses) {
		return "", errors.New("unexpected call to Send")
	}
	r := m.responses[m.callCount]
	m.callCount++
	return r.id, r.err
}

func TestBuildMessage_PlatformConfig(t *testing.T) {
	notification := &models.Notification{
		Title: "Test Title",
		Body:  "Test Body",
	}

	tests := []struct {
		name     string
		platform models.DevicePlatform
		check    func(t *testing.T, msg *messaging.Message)
	}{
		{
			name:     "iOS sets APNs badge, content-available, mutable-content, and push-type header",
			platform: models.DevicePlatform_DEVICE_PLATFORM_IOS,
			check: func(t *testing.T, msg *messaging.Message) {
				t.Helper()
				if msg.APNS == nil {
					t.Fatal("expected APNS config to be set")
				}
				if msg.Android != nil {
					t.Error("expected Android config to be nil for iOS")
				}
				if got := msg.APNS.Headers["apns-push-type"]; got != "alert" {
					t.Errorf("apns-push-type = %q, want %q", got, "alert")
				}
				aps := msg.APNS.Payload.Aps
				if aps.Badge == nil || *aps.Badge != 1 {
					t.Errorf("badge = %v, want 1", aps.Badge)
				}
				if !aps.ContentAvailable {
					t.Error("expected ContentAvailable to be true")
				}
				if !aps.MutableContent {
					t.Error("expected MutableContent to be true")
				}
				if aps.Sound != "default" {
					t.Errorf("sound = %q, want %q", aps.Sound, "default")
				}
				if aps.Alert.Title != "Test Title" || aps.Alert.Body != "Test Body" {
					t.Errorf("alert = %+v, want title=%q body=%q", aps.Alert, "Test Title", "Test Body")
				}
			},
		},
		{
			name:     "Android sets high priority and notification fields",
			platform: models.DevicePlatform_DEVICE_PLATFORM_ANDROID,
			check: func(t *testing.T, msg *messaging.Message) {
				t.Helper()
				if msg.Android == nil {
					t.Fatal("expected Android config to be set")
				}
				if msg.APNS != nil {
					t.Error("expected APNS config to be nil for Android")
				}
				if msg.Android.Priority != "high" {
					t.Errorf("priority = %q, want %q", msg.Android.Priority, "high")
				}
				if msg.Android.Notification.Title != "Test Title" {
					t.Errorf("title = %q, want %q", msg.Android.Notification.Title, "Test Title")
				}
			},
		},
		{
			name:     "unknown platform sets neither iOS nor Android config",
			platform: models.DevicePlatform_DEVICE_PLATFORM_UNSPECIFIED,
			check: func(t *testing.T, msg *messaging.Message) {
				t.Helper()
				if msg.APNS != nil {
					t.Error("expected APNS config to be nil for unspecified platform")
				}
				if msg.Android != nil {
					t.Error("expected Android config to be nil for unspecified platform")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := buildMessage(notifications.Target{DeviceToken: "test-token"}, notification, tt.platform)

			// Common checks for all platforms.
			//nolint:staticcheck // SA1019: Token is deprecated in favor of Fid, but the provider sends FCM registration tokens, which use Token; Fid is a distinct installation ID.
			if msg.Token != "test-token" {
				t.Errorf("token = %q, want %q", msg.Token, "test-token")
			}
			if msg.Notification.Title != "Test Title" {
				t.Errorf("notification title = %q, want %q", msg.Notification.Title, "Test Title")
			}

			tt.check(t, msg)
		})
	}
}

// TestBuildMessage_SendTarget locks the #3043 addressing rule: send to the
// Firebase installation ID when the client reported one, otherwise fall back to
// the registration token. The two are different identifiers — FCM's `fid`
// accepts only a FID — so putting the token in the wrong slot silently stops
// delivery, and a device registered before the client sent an installation ID
// must keep working.
func TestBuildMessage_SendTarget(t *testing.T) {
	notification := &models.Notification{Title: "T", Body: "B"}

	t.Run("installation ID present wins", func(t *testing.T) {
		msg := buildMessage(
			notifications.Target{DeviceToken: "reg-token", InstallationID: "fid-abc"},
			notification, models.DevicePlatform_DEVICE_PLATFORM_ANDROID,
		)

		if msg.Fid != "fid-abc" {
			t.Errorf("Fid = %q, want %q", msg.Fid, "fid-abc")
		}
		//nolint:staticcheck // SA1019: asserting the deprecated field is NOT set.
		if msg.Token != "" {
			t.Errorf("Token = %q, want empty — exactly one target may be set", msg.Token)
		}
	})

	t.Run("no installation ID falls back to the token", func(t *testing.T) {
		msg := buildMessage(
			notifications.Target{DeviceToken: "reg-token"},
			notification, models.DevicePlatform_DEVICE_PLATFORM_ANDROID,
		)

		//nolint:staticcheck // SA1019: the fallback path is the whole point here.
		if msg.Token != "reg-token" {
			t.Errorf("Token = %q, want %q", msg.Token, "reg-token")
		}
		if msg.Fid != "" {
			t.Errorf("Fid = %q, want empty — exactly one target may be set", msg.Fid)
		}
	})
}

func TestBuildMessage_ChatMessageGrouping(t *testing.T) {
	chatNotification := &models.Notification{
		Title: "Alice",
		Body:  "Hello!",
		Payload: &models.Notification_ChatMessage{
			ChatMessage: &models.ChatMessagePayload{
				ConversationId: "conv-42",
				MessageId:      "msg-1",
				SenderUserId:   "user-alice",
				SenderName:     "Alice",
				PreviewText:    "Hello!",
			},
		},
	}

	nonChatNotification := &models.Notification{
		Title: "Test",
		Body:  "Test",
		Payload: &models.Notification_CommunityEvent{
			CommunityEvent: &models.CommunityEventPayload{
				CommunityId: "comm-1",
				EventId:     "evt-1",
				EventType:   "GEAR_SHARED",
			},
		},
	}

	t.Run("Android chat message is data-only (no Notification, no android.Notification)", func(t *testing.T) {
		// The client renders chat messages from the data payload using
		// MessagingStyle, both in foreground and via the background isolate.
		// Including a top-level or android.Notification block would cause FCM
		// to display its own simple alert before the client renderer ran.
		msg := buildMessage(notifications.Target{DeviceToken: "token"}, chatNotification, models.DevicePlatform_DEVICE_PLATFORM_ANDROID)
		if msg.Notification != nil {
			t.Errorf("top-level Notification must be nil for Android chat, got %+v", msg.Notification)
		}
		if msg.Android == nil {
			t.Fatal("Android config must be set even when Notification is omitted")
		}
		if msg.Android.Notification != nil {
			t.Errorf("android.Notification must be nil for Android chat, got %+v", msg.Android.Notification)
		}
		if msg.Android.Priority != "high" {
			t.Errorf("android.Priority = %q, want %q", msg.Android.Priority, "high")
		}
		if msg.Data["type"] != "chat_message" {
			t.Errorf("data[type] = %q, want %q", msg.Data["type"], "chat_message")
		}
		if msg.Data["conversation_id"] != "conv-42" {
			t.Errorf("data[conversation_id] = %q, want %q", msg.Data["conversation_id"], "conv-42")
		}
	})

	t.Run("Android non-chat notification keeps Notification block and omits tag", func(t *testing.T) {
		msg := buildMessage(notifications.Target{DeviceToken: "token"}, nonChatNotification, models.DevicePlatform_DEVICE_PLATFORM_ANDROID)
		if msg.Notification == nil {
			t.Error("top-level Notification must be set for non-chat Android push")
		}
		if msg.Android == nil || msg.Android.Notification == nil {
			t.Fatal("expected Android notification config to be set")
		}
		if got := msg.Android.Notification.Tag; got != "" {
			t.Errorf("android tag = %q, want empty for non-chat notification", got)
		}
	})

	t.Run("iOS chat message sets thread-id and apns-priority", func(t *testing.T) {
		msg := buildMessage(notifications.Target{DeviceToken: "token"}, chatNotification, models.DevicePlatform_DEVICE_PLATFORM_IOS)
		if msg.APNS == nil || msg.APNS.Payload == nil || msg.APNS.Payload.Aps == nil {
			t.Fatal("expected APNS payload to be set")
		}
		if got := msg.APNS.Payload.Aps.ThreadID; got != "conv-42" {
			t.Errorf("aps thread-id = %q, want %q", got, "conv-42")
		}
		if got := msg.APNS.Headers["apns-priority"]; got != "10" {
			t.Errorf("apns-priority header = %q, want %q", got, "10")
		}
	})

	t.Run("iOS non-chat notification omits thread-id and apns-priority", func(t *testing.T) {
		msg := buildMessage(notifications.Target{DeviceToken: "token"}, nonChatNotification, models.DevicePlatform_DEVICE_PLATFORM_IOS)
		if msg.APNS == nil || msg.APNS.Payload == nil || msg.APNS.Payload.Aps == nil {
			t.Fatal("expected APNS payload to be set")
		}
		if got := msg.APNS.Payload.Aps.ThreadID; got != "" {
			t.Errorf("aps thread-id = %q, want empty for non-chat notification", got)
		}
		if _, present := msg.APNS.Headers["apns-priority"]; present {
			t.Errorf("apns-priority header present for non-chat notification; want absent")
		}
	})

	t.Run("chat notification with empty conversation_id falls back to normal Notification path", func(t *testing.T) {
		// An empty conversation_id is a degraded / malformed chat payload —
		// the client renderer would have nothing to key on. We fall back to
		// the normal Notification path so the user still sees the push.
		emptyChat := &models.Notification{
			Title: "Test",
			Body:  "Test",
			Payload: &models.Notification_ChatMessage{
				ChatMessage: &models.ChatMessagePayload{
					ConversationId: "",
				},
			},
		}
		androidMsg := buildMessage(notifications.Target{DeviceToken: "token"}, emptyChat, models.DevicePlatform_DEVICE_PLATFORM_ANDROID)
		if androidMsg.Notification == nil {
			t.Error("top-level Notification must be set when conversation_id is empty (fallback path)")
		}
		if androidMsg.Android == nil || androidMsg.Android.Notification == nil {
			t.Fatal("Android.Notification must be set in fallback path")
		}
		if got := androidMsg.Android.Notification.Tag; got != "" {
			t.Errorf("android tag = %q, want empty when conversation_id is empty", got)
		}
		iosMsg := buildMessage(notifications.Target{DeviceToken: "token"}, emptyChat, models.DevicePlatform_DEVICE_PLATFORM_IOS)
		if got := iosMsg.APNS.Payload.Aps.ThreadID; got != "" {
			t.Errorf("aps thread-id = %q, want empty when conversation_id is empty", got)
		}
		if _, present := iosMsg.APNS.Headers["apns-priority"]; present {
			t.Errorf("apns-priority header set when conversation_id is empty; want absent")
		}
	})

	t.Run("iOS chat message keeps top-level Notification (iOS displays system-side)", func(t *testing.T) {
		// iOS data-only delivery requires a Notification Service Extension
		// follow-up. Until then we keep the top-level Notification so iOS
		// shows the alert directly; thread-id still groups them.
		msg := buildMessage(notifications.Target{DeviceToken: "token"}, chatNotification, models.DevicePlatform_DEVICE_PLATFORM_IOS)
		if msg.Notification == nil {
			t.Error("top-level Notification must remain set for iOS chat messages")
		}
		if msg.APNS == nil || msg.APNS.Payload == nil || msg.APNS.Payload.Aps == nil ||
			msg.APNS.Payload.Aps.Alert == nil {
			t.Fatal("APNS alert payload must remain set for iOS chat messages")
		}
	})

	t.Run("conversation_title is forwarded to the data payload and the iOS APS subtitle", func(t *testing.T) {
		title := "Hiking at Zilker Greenbelt"
		chatWithTitle := &models.Notification{
			Title: "Alice",
			Body:  "Hello!",
			Payload: &models.Notification_ChatMessage{
				ChatMessage: &models.ChatMessagePayload{
					ConversationId:    "conv-42",
					MessageId:         "msg-1",
					SenderUserId:      "user-alice",
					SenderName:        "Alice",
					PreviewText:       "Hello!",
					ConversationTitle: &title,
				},
			},
		}

		androidMsg := buildMessage(notifications.Target{DeviceToken: "token"}, chatWithTitle, models.DevicePlatform_DEVICE_PLATFORM_ANDROID)
		if got := androidMsg.Data["conversation_title"]; got != title {
			t.Errorf("android data[conversation_title] = %q, want %q", got, title)
		}

		iosMsg := buildMessage(notifications.Target{DeviceToken: "token"}, chatWithTitle, models.DevicePlatform_DEVICE_PLATFORM_IOS)
		if got := iosMsg.APNS.Payload.Aps.Alert.SubTitle; got != title {
			t.Errorf("iOS aps.alert.subtitle = %q, want %q", got, title)
		}
		if got := iosMsg.Data["conversation_title"]; got != title {
			t.Errorf("iOS data[conversation_title] = %q, want %q", got, title)
		}
	})

	t.Run("conversation_title is omitted from data and subtitle when absent", func(t *testing.T) {
		androidMsg := buildMessage(notifications.Target{DeviceToken: "token"}, chatNotification, models.DevicePlatform_DEVICE_PLATFORM_ANDROID)
		if _, ok := androidMsg.Data["conversation_title"]; ok {
			t.Error("android data[conversation_title] must be absent when not set")
		}
		iosMsg := buildMessage(notifications.Target{DeviceToken: "token"}, chatNotification, models.DevicePlatform_DEVICE_PLATFORM_IOS)
		if got := iosMsg.APNS.Payload.Aps.Alert.SubTitle; got != "" {
			t.Errorf("iOS aps.alert.subtitle = %q, want empty when conversation_title unset", got)
		}
		if _, ok := iosMsg.Data["conversation_title"]; ok {
			t.Error("iOS data[conversation_title] must be absent when not set")
		}
	})
}

func TestChatConversationIDFor(t *testing.T) {
	tests := []struct {
		name         string
		notification *models.Notification
		want         string
	}{
		{
			name: "chat message returns conversation_id",
			notification: &models.Notification{
				Payload: &models.Notification_ChatMessage{
					ChatMessage: &models.ChatMessagePayload{ConversationId: "conv-9"},
				},
			},
			want: "conv-9",
		},
		{
			name: "community event returns empty",
			notification: &models.Notification{
				Payload: &models.Notification_CommunityEvent{
					CommunityEvent: &models.CommunityEventPayload{CommunityId: "c1"},
				},
			},
			want: "",
		},
		{
			name:         "no payload returns empty",
			notification: &models.Notification{},
			want:         "",
		},
		{
			name: "nil ChatMessage returns empty",
			notification: &models.Notification{
				Payload: &models.Notification_ChatMessage{ChatMessage: nil},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chatConversationIDFor(tt.notification); got != tt.want {
				t.Errorf("chatConversationIDFor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildDataPayload_ChatMessage(t *testing.T) {
	tests := []struct {
		name       string
		payload    *models.ChatMessagePayload
		wantKeys   map[string]string
		absentKeys []string
	}{
		{
			name: "includes community_id when present",
			payload: &models.ChatMessagePayload{
				ConversationId: "conv1",
				MessageId:      "msg1",
				SenderUserId:   "user1",
				SenderName:     "Alice",
				PreviewText:    "Hello",
				CommunityId:    "comm1",
			},
			wantKeys: map[string]string{
				"type":            "chat_message",
				"conversation_id": "conv1",
				"message_id":      "msg1",
				"sender_user_id":  "user1",
				"sender_name":     "Alice",
				"preview_text":    "Hello",
				"community_id":    "comm1",
			},
			absentKeys: nil,
		},
		{
			name: "omits community_id when empty",
			payload: &models.ChatMessagePayload{
				ConversationId: "conv1",
				MessageId:      "msg1",
				SenderUserId:   "user1",
				SenderName:     "Bob",
				PreviewText:    "Hi",
				CommunityId:    "",
			},
			wantKeys: map[string]string{
				"type":            "chat_message",
				"conversation_id": "conv1",
			},
			absentKeys: []string{"community_id"},
		},
		{
			name: "includes experience_id when present",
			payload: &models.ChatMessagePayload{
				ConversationId: "conv1",
				MessageId:      "msg1",
				SenderUserId:   "user1",
				SenderName:     "Carol",
				PreviewText:    "Excited!",
				CommunityId:    "comm1",
				ExperienceId:   strPtr("exp1"),
			},
			wantKeys: map[string]string{
				"type":            "chat_message",
				"conversation_id": "conv1",
				"community_id":    "comm1",
				"experience_id":   "exp1",
			},
			absentKeys: []string{"gear_id"},
		},
		{
			name: "includes gear_id when present",
			payload: &models.ChatMessagePayload{
				ConversationId: "conv1",
				MessageId:      "msg1",
				SenderUserId:   "user1",
				SenderName:     "Dave",
				PreviewText:    "Thanks!",
				CommunityId:    "comm1",
				GearId:         strPtr("gear1"),
			},
			wantKeys: map[string]string{
				"type":            "chat_message",
				"conversation_id": "conv1",
				"community_id":    "comm1",
				"gear_id":         "gear1",
			},
			absentKeys: []string{"experience_id", "request_id"},
		},
		{
			name: "includes request_id when present",
			payload: &models.ChatMessagePayload{
				ConversationId: "conv1",
				MessageId:      "msg1",
				SenderUserId:   "user1",
				SenderName:     "Eve",
				PreviewText:    "I can help!",
				CommunityId:    "comm1",
				RequestId:      strPtr("req1"),
			},
			wantKeys: map[string]string{
				"type":            "chat_message",
				"conversation_id": "conv1",
				"community_id":    "comm1",
				"request_id":      "req1",
			},
			absentKeys: []string{"experience_id", "gear_id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notification := &models.Notification{
				Title: "Test",
				Body:  "Test body",
				Payload: &models.Notification_ChatMessage{
					ChatMessage: tt.payload,
				},
			}

			data := buildDataPayload(notification)

			for key, want := range tt.wantKeys {
				got, ok := data[key]
				if !ok {
					t.Errorf("expected key %q in payload", key)
					continue
				}
				if got != want {
					t.Errorf("data[%q] = %q, want %q", key, got, want)
				}
			}

			for _, key := range tt.absentKeys {
				if _, ok := data[key]; ok {
					t.Errorf("expected key %q to be absent, but it was present", key)
				}
			}
		})
	}
}

func strPtr(s string) *string { return &s }

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "missing required authentication credential",
			err:  errors.New("Request is missing required authentication credential. Expected OAuth 2 access token, login cookie or other valid authentication credential."),
			want: true,
		},
		{
			name: "invalid authentication credentials",
			err:  errors.New("Request had invalid authentication credentials. Expected OAuth 2 access token."),
			want: true,
		},
		{
			name: "OAuth 2 access token partial match",
			err:  errors.New("some error about OAuth 2 access token being expired"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("connection refused"),
			want: false,
		},
		{
			name: "invalid token error is not auth error",
			err:  errors.New("Requested entity was not found"),
			want: false,
		},
		{
			name: "sender mismatch is not auth error",
			err:  errors.New("SenderId mismatch"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAuthError(tt.err)
			if got != tt.want {
				t.Errorf("isAuthError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsInvalidTokenError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "requested entity was not found",
			err:  errors.New("Requested entity was not found."),
			want: true,
		},
		{
			name: "sender id mismatch",
			err:  errors.New("SenderId mismatch"),
			want: true,
		},
		{
			name: "registration token not registered",
			err:  errors.New("registration-token-not-registered"),
			want: true,
		},
		{
			name: "invalid registration token",
			err:  errors.New("invalid-registration-token"),
			want: true,
		},
		{
			name: "invalid-argument error code",
			err:  errors.New("invalid-argument: malformed token"),
			want: true,
		},
		{
			name: "human-readable invalid argument",
			err:  errors.New("Request contains an invalid argument."),
			want: true,
		},
		{
			name: "NotRegistered HTTP v1 message",
			err:  errors.New("NotRegistered"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("connection refused"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInvalidTokenError(tt.err)
			if got != tt.want {
				t.Errorf("isInvalidTokenError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsInvalidTokenError_NotAuthError(t *testing.T) {
	// Auth errors must never be classified as invalid token errors,
	// which would trigger device cleanup.
	authErrors := []error{
		errors.New("Request is missing required authentication credential. Expected OAuth 2 access token."),
		errors.New("Request had invalid authentication credentials."),
	}

	for _, err := range authErrors {
		if isInvalidTokenError(err) {
			t.Errorf("isInvalidTokenError(%v) = true, want false — auth errors must not trigger device cleanup", err)
		}
	}
}

func TestBuildDataPayload_CommunityEvent(t *testing.T) {
	tests := []struct {
		name       string
		payload    *models.CommunityEventPayload
		wantKeys   map[string]string
		absentKeys []string
	}{
		{
			name: "includes gear_id when present",
			payload: &models.CommunityEventPayload{
				CommunityId: "comm1",
				EventId:     "evt1",
				EventType:   "TRANSFER_PICKUP_PROPOSED",
				GearId:      "gear1",
			},
			wantKeys: map[string]string{
				"type":         "community_event",
				"community_id": "comm1",
				"event_id":     "evt1",
				"event_type":   "TRANSFER_PICKUP_PROPOSED",
				"gear_id":      "gear1",
			},
			absentKeys: []string{"conversation_id", "gear_name", "request_title"},
		},
		{
			name: "omits gear_id when empty",
			payload: &models.CommunityEventPayload{
				CommunityId: "comm1",
				EventId:     "evt1",
				EventType:   "GEAR_SHARED",
			},
			wantKeys: map[string]string{
				"type": "community_event",
			},
			absentKeys: []string{"gear_id", "conversation_id"},
		},
		{
			name: "includes all optional fields when present",
			payload: &models.CommunityEventPayload{
				CommunityId:  "comm1",
				EventId:      "evt1",
				EventType:    "TRANSFER_INTEREST_EXPRESSED",
				GearId:       "gear1",
				GearName:     "Drill",
				RequestTitle: "Need a ladder",
			},
			wantKeys: map[string]string{
				"type":          "community_event",
				"gear_id":       "gear1",
				"gear_name":     "Drill",
				"request_title": "Need a ladder",
			},
			absentKeys: []string{"conversation_id"},
		},
		{
			name: "includes request_id when present",
			payload: &models.CommunityEventPayload{
				CommunityId: "comm1",
				EventId:     "evt1",
				EventType:   "REQUEST_OFFER_MADE",
				RequestId:   strPtr("req1"),
			},
			wantKeys: map[string]string{
				"type":       "community_event",
				"request_id": "req1",
			},
			absentKeys: []string{"gear_id", "experience_id", "conversation_id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notification := &models.Notification{
				Title: "Test",
				Body:  "Test body",
				Payload: &models.Notification_CommunityEvent{
					CommunityEvent: tt.payload,
				},
			}

			data := buildDataPayload(notification)

			for key, want := range tt.wantKeys {
				got, ok := data[key]
				if !ok {
					t.Errorf("expected key %q in payload", key)
					continue
				}
				if got != want {
					t.Errorf("data[%q] = %q, want %q", key, got, want)
				}
			}

			for _, key := range tt.absentKeys {
				if _, ok := data[key]; ok {
					t.Errorf("expected key %q to be absent, but it was present", key)
				}
			}
		})
	}
}

func TestSend_PersistentAuthError_ReturnsInvalidToken(t *testing.T) {
	authErr := errors.New("Request is missing required authentication credential. Expected OAuth 2 access token, login cookie or other valid authentication credential.")

	sender := &mockSender{
		responses: []sendResult{
			{err: authErr}, // first attempt
			{err: authErr}, // retry also fails
		},
	}

	provider := newTestProvider(sender, models.DevicePlatform_DEVICE_PLATFORM_IOS)
	notification := &models.Notification{Title: "Test", Body: "Test"}

	err := provider.Send(context.Background(), notifications.Target{DeviceToken: "test-token"}, notification)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, notifications.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken in error chain, got: %v", err)
	}
	if sender.callCount != 2 {
		t.Errorf("expected 2 Send calls (initial + retry), got %d", sender.callCount)
	}
}

func TestSend_AuthErrorThenSuccess(t *testing.T) {
	authErr := errors.New("Request is missing required authentication credential. Expected OAuth 2 access token.")

	sender := &mockSender{
		responses: []sendResult{
			{err: authErr},            // first attempt fails
			{id: "msg-123", err: nil}, // retry succeeds
		},
	}

	provider := newTestProvider(sender, models.DevicePlatform_DEVICE_PLATFORM_IOS)
	notification := &models.Notification{Title: "Test", Body: "Test"}

	err := provider.Send(context.Background(), notifications.Target{DeviceToken: "test-token"}, notification)
	if err != nil {
		t.Errorf("expected no error after successful retry, got: %v", err)
	}
	if sender.callCount != 2 {
		t.Errorf("expected 2 Send calls (initial + retry), got %d", sender.callCount)
	}
}

func TestSend_InvalidTokenError_ReturnsInvalidToken(t *testing.T) {
	sender := &mockSender{
		responses: []sendResult{
			{err: errors.New("Requested entity was not found")},
		},
	}

	provider := newTestProvider(sender, models.DevicePlatform_DEVICE_PLATFORM_IOS)
	notification := &models.Notification{Title: "Test", Body: "Test"}

	err := provider.Send(context.Background(), notifications.Target{DeviceToken: "test-token"}, notification)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, notifications.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken in error chain, got: %v", err)
	}
	if sender.callCount != 1 {
		t.Errorf("expected 1 Send call (no retry for non-auth errors), got %d", sender.callCount)
	}
}

func TestSend_NotRegistered_ReturnsInvalidToken(t *testing.T) {
	sender := &mockSender{
		responses: []sendResult{
			{err: errors.New("NotRegistered")},
		},
	}

	provider := newTestProvider(sender, models.DevicePlatform_DEVICE_PLATFORM_IOS)
	notification := &models.Notification{Title: "Test", Body: "Test"}

	err := provider.Send(context.Background(), notifications.Target{DeviceToken: "test-token"}, notification)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, notifications.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken in error chain, got: %v", err)
	}
	if sender.callCount != 1 {
		t.Errorf("expected 1 Send call (no retry for non-auth errors), got %d", sender.callCount)
	}
}

func TestSend_NonAuthNonTokenError_NoInvalidToken(t *testing.T) {
	sender := &mockSender{
		responses: []sendResult{
			{err: errors.New("connection refused")},
		},
	}

	provider := newTestProvider(sender, models.DevicePlatform_DEVICE_PLATFORM_IOS)
	notification := &models.Notification{Title: "Test", Body: "Test"}

	err := provider.Send(context.Background(), notifications.Target{DeviceToken: "test-token"}, notification)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, notifications.ErrInvalidToken) {
		t.Errorf("expected error NOT to be ErrInvalidToken for transient failures, got: %v", err)
	}
}

func TestSend_Success(t *testing.T) {
	sender := &mockSender{
		responses: []sendResult{
			{id: "msg-456", err: nil},
		},
	}

	provider := newTestProvider(sender, models.DevicePlatform_DEVICE_PLATFORM_ANDROID)
	notification := &models.Notification{Title: "Test", Body: "Test"}

	err := provider.Send(context.Background(), notifications.Target{DeviceToken: "test-token"}, notification)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if sender.callCount != 1 {
		t.Errorf("expected 1 Send call, got %d", sender.callCount)
	}
}
