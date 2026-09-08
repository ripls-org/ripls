package fcm

import (
	"context"
	"fmt"
	"strings"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications"
)

// messageSender abstracts the FCM send operation for testability.
type messageSender interface {
	Send(ctx context.Context, message *messaging.Message) (string, error)
}

// Provider implements notification delivery via Firebase Cloud Messaging.
type Provider struct {
	client   messageSender
	platform models.DevicePlatform
}

// NewMessagingClient creates a Firebase messaging client from an existing
// Firebase app. Call this once at startup, then pass the client to NewProvider
// for each platform.
func NewMessagingClient(ctx context.Context, app *firebase.App) (*messaging.Client, error) {
	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get messaging client: %w", err)
	}

	logger := logging.LoggerWithContext(ctx)
	logger.InfoContext(ctx, "initialized FCM messaging client")

	return client, nil
}

// NewProvider creates a new FCM provider for the given platform using a shared messaging client.
func NewProvider(client *messaging.Client, platform models.DevicePlatform) *Provider {
	return &Provider{
		client:   client,
		platform: platform,
	}
}

// newTestProvider creates a provider with a custom messageSender for testing.
func newTestProvider(sender messageSender, platform models.DevicePlatform) *Provider {
	return &Provider{
		client:   sender,
		platform: platform,
	}
}

// Send sends a notification via FCM to the specified device token.
func (p *Provider) Send(ctx context.Context, target notifications.Target, notification *models.Notification) error {
	message := buildMessage(target, notification, p.platform)

	logger := logging.LoggerWithContext(ctx)
	if commEventID := notifications.CommunityEventIDFromContext(ctx); commEventID != "" {
		logger = logger.With("community_event_id", commEventID)
	}

	// Send the message, retrying once on auth errors.
	// Auth errors can be either:
	//   1. Transient: expired ADC/OAuth2 token on Cloud Run (retrying refreshes it)
	//   2. Persistent: FCM's THIRD_PARTY_AUTH_ERROR — Apple APNs rejected the push.
	//      Despite the misleading "OAuth 2 access token" error message, this means
	//      FCM failed to authenticate with APNs, NOT that our credentials to FCM
	//      are wrong. Common causes: app bundle ID mismatch between client and
	//      Firebase project, wrong APNs key/team ID in Firebase Console.
	response, err := p.client.Send(ctx, message)
	if err != nil && isAuthError(err) {
		logger.WarnContext(ctx, "FCM auth error on first attempt, retrying",
			"device_token", logging.MaskToken(target.DeviceToken),
			"platform", p.platform,
			"error", err,
		)
		response, err = p.client.Send(ctx, message)
		if err != nil {
			logger.ErrorContext(ctx, "FCM send failed after auth retry — likely APNs auth rejection (THIRD_PARTY_AUTH_ERROR), not an OAuth2 issue. Check: (1) app bundle ID matches Firebase iOS app config, (2) APNs key ID and team ID are correct in Firebase Console",
				"device_token", logging.MaskToken(target.DeviceToken),
				"platform", p.platform,
				"error", err,
			)
			// Persistent auth failure for this token — FCM cannot deliver.
			// Mark as invalid so the notification service cleans it up.
			return fmt.Errorf("failed to send FCM message: %w: %w", notifications.ErrInvalidToken, err)
		}
		logger.InfoContext(ctx, "FCM send succeeded after auth retry (transient ADC token expiry)",
			"device_token", logging.MaskToken(target.DeviceToken),
			"platform", p.platform,
			"message_id", response,
		)
		return nil
	}
	if err != nil {
		// Check if this is an invalid token error that should trigger cleanup.
		if isInvalidTokenError(err) {
			logger.WarnContext(ctx, "invalid or expired token detected",
				"device_token", logging.MaskToken(target.DeviceToken),
				"platform", p.platform,
				"error", err,
			)
			return fmt.Errorf("failed to send FCM message: %w: %w", notifications.ErrInvalidToken, err)
		}
		logger.ErrorContext(ctx, "failed to send FCM message",
			"device_token", logging.MaskToken(target.DeviceToken),
			"platform", p.platform,
			"error", err,
		)
		return fmt.Errorf("failed to send FCM message: %w", err)
	}

	logger.DebugContext(ctx, "successfully sent FCM notification",
		"device_token", logging.MaskToken(target.DeviceToken),
		"platform", p.platform,
		"message_id", response,
	)

	return nil
}

// isAuthError checks if an FCM error indicates a server-side authentication
// failure (e.g. expired ADC OAuth2 token on Cloud Run). These are transient and
// should be retried — the Go oauth2 library refreshes the token lazily on the
// next call after expiry.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	authErrors := []string{
		"missing required authentication credential",
		"Request had invalid authentication credentials",
		"OAuth 2 access token",
	}

	for _, authErr := range authErrors {
		if strings.Contains(errMsg, authErr) {
			return true
		}
	}

	return false
}

// isInvalidTokenError checks if an FCM error indicates an invalid/expired token
// that should be removed from the database.
func isInvalidTokenError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()

	// FCM error messages that indicate invalid tokens. FCM surfaces these in
	// two forms depending on the code path: machine-readable error codes
	// (hyphenated) and human-readable messages (spaced, capitalized). Match
	// both so malformed tokens are cleaned up consistently.
	invalidTokenErrors := []string{
		"Requested entity was not found",
		"SenderId mismatch",
		"registration-token-not-registered",
		"invalid-registration-token",
		"invalid-argument",                     // error-code form
		"Request contains an invalid argument", // human-readable form
		"NotRegistered",                        // HTTP v1 message form surfaced via FirebaseError.Error()
	}

	for _, invalidErr := range invalidTokenErrors {
		if strings.Contains(errMsg, invalidErr) {
			return true
		}
	}

	return false
}

// Platform returns the platform this provider handles.
func (p *Provider) Platform() models.DevicePlatform {
	return p.platform
}

// buildMessage constructs a platform-specific FCM message for the given notification.
func buildMessage(target notifications.Target, notification *models.Notification, platform models.DevicePlatform) *messaging.Message {
	// Non-empty when this notification is a chat message; drives per-platform
	// grouping fields (Android tag, iOS thread-id) so multiple messages in the
	// same conversation collapse into one notification on the device.
	chatConversationID := chatConversationIDFor(notification)
	chatConversationTitle := chatConversationTitleFor(notification)

	// Chat messages on Android use data-only delivery: the client renders the
	// notification with MessagingStyle in onMessageReceived (foreground) or in
	// firebaseMessagingBackgroundHandler (background), reconstructing the
	// recent transcript from a per-conversation cache. Including a top-level
	// `notification` block here would cause FCM to display its own simple
	// alert before the client renderer ran, producing duplicate notifications.
	androidChatDataOnly := chatConversationID != "" &&
		platform == models.DevicePlatform_DEVICE_PLATFORM_ANDROID

	// Address by Firebase installation ID when the client reported one; fall
	// back to the registration token otherwise (#3043).
	//
	// These are NOT interchangeable. The FCM v1 Message resource deprecates
	// `token` in favour of `fid`, but its own reference says `token` "also
	// accepts a Firebase Installation ID (FID)" only "during the transition
	// period", while `fid` accepts a FID and nothing else. Putting a
	// registration token in the FID slot does not work — so the fallback is
	// not defensive padding, it is the only correct behaviour for a device
	// that registered before the client started sending an installation ID,
	// and it has to stay until those registrations have aged out.
	//
	// That is also why the deprecated field cannot simply be dropped, and why
	// the //nolint below is permanent rather than transitional.
	message := &messaging.Message{}
	if target.InstallationID != "" {
		message.Fid = target.InstallationID
	} else {
		message.Token = target.DeviceToken //nolint:staticcheck // SA1019: required fallback for devices registered without an installation ID (#3043).
	}
	if !androidChatDataOnly {
		message.Notification = &messaging.Notification{
			Title: notification.Title,
			Body:  notification.Body,
		}
	}

	// Add platform-specific configuration.
	switch platform {
	case models.DevicePlatform_DEVICE_PLATFORM_ANDROID:
		androidCfg := &messaging.AndroidConfig{
			Priority: "high",
		}
		if !androidChatDataOnly {
			androidNotif := &messaging.AndroidNotification{
				Title: notification.Title,
				Body:  notification.Body,
			}
			// Android collapses multiple notifications with the same tag at
			// the system level — second push replaces first in the shade. This
			// branch handles non-chat pushes; chat messages skip the entire
			// android.Notification block above.
			if chatConversationID != "" {
				androidNotif.Tag = chatConversationID
			}
			androidCfg.Notification = androidNotif
		}
		message.Android = androidCfg
	case models.DevicePlatform_DEVICE_PLATFORM_IOS:
		badge := 1
		headers := map[string]string{
			"apns-push-type": "alert",
		}
		aps := &messaging.Aps{
			Alert: &messaging.ApsAlert{
				Title: notification.Title,
				Body:  notification.Body,
			},
			Badge:            &badge,
			ContentAvailable: true,
			MutableContent:   true,
			Sound:            "default",
		}
		// iOS groups notifications sharing a thread-id into a single
		// expandable thread on the lock screen. Combined with apns-priority
		// 10 (immediate delivery) this is the standard chat-app pattern.
		// The conversation title becomes the alert subtitle so the lock
		// screen shows "Sender / Topic / Message" — meaningful even on a
		// busy thread where consecutive messages come from different senders.
		if chatConversationID != "" {
			aps.ThreadID = chatConversationID
			headers["apns-priority"] = "10"
			if chatConversationTitle != "" {
				aps.Alert.SubTitle = chatConversationTitle
			}
		}
		message.APNS = &messaging.APNSConfig{
			Headers: headers,
			Payload: &messaging.APNSPayload{
				Aps: aps,
			},
		}
	}

	// Add custom data payload if present.
	data := buildDataPayload(notification)
	if len(data) > 0 {
		message.Data = data
	}

	return message
}

// chatConversationIDFor returns the conversation_id when the notification
// payload is a chat message, or "" otherwise. Used by buildMessage to scope
// chat-specific grouping fields without leaking the payload type-switch.
func chatConversationIDFor(notification *models.Notification) string {
	cm, ok := notification.Payload.(*models.Notification_ChatMessage)
	if !ok || cm.ChatMessage == nil {
		return ""
	}
	return cm.ChatMessage.ConversationId
}

// chatConversationTitleFor returns the human-readable topic title for a chat
// message payload, or "" when absent. Optional on the wire — clients fall
// back to rendering the sender's name as the heading when this is missing.
func chatConversationTitleFor(notification *models.Notification) string {
	cm, ok := notification.Payload.(*models.Notification_ChatMessage)
	if !ok || cm.ChatMessage == nil {
		return ""
	}
	return cm.ChatMessage.GetConversationTitle()
}

// buildDataPayload converts the notification payload oneof to FCM data fields.
func buildDataPayload(notification *models.Notification) map[string]string {
	data := make(map[string]string)

	switch payload := notification.Payload.(type) {
	case *models.Notification_CommunityEvent:
		data["type"] = "community_event"
		data["community_id"] = payload.CommunityEvent.CommunityId
		data["event_id"] = payload.CommunityEvent.EventId
		data["event_type"] = payload.CommunityEvent.EventType
		if payload.CommunityEvent.GearId != "" {
			data["gear_id"] = payload.CommunityEvent.GearId
		}
		if payload.CommunityEvent.ExperienceId != nil {
			data["experience_id"] = payload.CommunityEvent.GetExperienceId()
		}
		if payload.CommunityEvent.RequestId != nil {
			data["request_id"] = payload.CommunityEvent.GetRequestId()
		}
		if payload.CommunityEvent.GearName != "" {
			data["gear_name"] = payload.CommunityEvent.GearName
		}
		if payload.CommunityEvent.RequestTitle != "" {
			data["request_title"] = payload.CommunityEvent.RequestTitle
		}
	case *models.Notification_ChatMessage:
		data["type"] = "chat_message"
		data["conversation_id"] = payload.ChatMessage.ConversationId
		data["message_id"] = payload.ChatMessage.MessageId
		data["sender_user_id"] = payload.ChatMessage.SenderUserId
		data["sender_name"] = payload.ChatMessage.SenderName
		data["preview_text"] = payload.ChatMessage.PreviewText
		if payload.ChatMessage.CommunityId != "" {
			data["community_id"] = payload.ChatMessage.CommunityId
		}
		if title := payload.ChatMessage.GetConversationTitle(); title != "" {
			data["conversation_title"] = title
		}
		if payload.ChatMessage.ExperienceId != nil {
			data["experience_id"] = payload.ChatMessage.GetExperienceId()
		}
		if payload.ChatMessage.GearId != nil {
			data["gear_id"] = payload.ChatMessage.GetGearId()
		}
		if payload.ChatMessage.RequestId != nil {
			data["request_id"] = payload.ChatMessage.GetRequestId()
		}
	}

	return data
}
