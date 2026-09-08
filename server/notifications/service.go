package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

var (
	// ErrInvalidToken indicates the device token is invalid or expired.
	ErrInvalidToken = errors.New("invalid device token")
	// ErrRateLimited indicates the provider is rate limiting requests.
	ErrRateLimited = errors.New("rate limited by provider")
)

// Service sends notifications to users through various providers.
type Service interface {
	// NotifyUser sends a notification to all of a user's registered devices
	NotifyUser(ctx context.Context, userID string, notification *models.Notification) error

	// HasDevices checks if a user has any registered devices
	HasDevices(ctx context.Context, userID string) bool

	// UnregisterDevice removes a device token (shared with device service)
	UnregisterDevice(ctx context.Context, deviceToken string) error

	// SendPhoneOptInWelcome sends the one-time opt-in confirmation ("welcome")
	// text to a user's freshly verified phone via the platform SMS/RCS channel.
	// Sent on first opt-in only (PhoneRegister) and best-effort: callers must not
	// block registration on the result.
	SendPhoneOptInWelcome(ctx context.Context, user *models.User) error
}

// Target identifies the app instance a notification is addressed to.
//
// FCM deprecated addressing by registration token in favour of the Firebase
// installation ID, but the two are different identifiers and are not
// interchangeable — sending a registration token in the FID slot does not
// work. So both travel together: the provider uses the installation ID when
// the client reported one and falls back to the token otherwise, which keeps
// every already-registered device working unchanged. See #3043.
type Target struct {
	// DeviceToken is the FCM registration token. Always present.
	DeviceToken string

	// InstallationID is the Firebase installation ID, when the client
	// supplied one at registration. Empty for older registrations.
	InstallationID string
}

// Provider handles sending notifications for a specific platform.
type Provider interface {
	// Send sends a notification to a specific app instance
	Send(ctx context.Context, target Target, notification *models.Notification) error

	// Platform returns the platform this provider handles
	Platform() models.DevicePlatform
}

type service struct {
	providers   map[models.DevicePlatform]Provider
	storage     *storage.ProtoSQLStorage
	offAppEmail OffAppEmailConfig
	sms         SMSChannel
	shareLinks  ShareLinkResolver
}

// NewService creates a new notification service with the given providers. Pass
// WithOffAppEmail to enable the email fallback for deviceless recipients.
func NewService(providers []Provider, storage *storage.ProtoSQLStorage, opts ...Option) Service {
	providerMap := make(map[models.DevicePlatform]Provider)
	for _, p := range providers {
		providerMap[p.Platform()] = p
	}

	svc := &service{
		providers: providerMap,
		storage:   storage,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// getNotificationType extracts the notification type from the payload.
func getNotificationType(notification *models.Notification) string {
	if notification == nil {
		return "unknown"
	}
	switch notification.Payload.(type) {
	case *models.Notification_CommunityEvent:
		return "community_event"
	case *models.Notification_ChatMessage:
		return "chat_message"
	default:
		return "unknown"
	}
}

// NotifyUser sends a notification to all of a user's registered devices.
func (s *service) NotifyUser(ctx context.Context, userID string, notification *models.Notification) error {
	logger := logging.LoggerWithContext(ctx).With(
		"user_id", userID,
	)
	if commEventID := CommunityEventIDFromContext(ctx); commEventID != "" {
		logger = logger.With("community_event_id", commEventID)
	}

	// Get all devices for this user
	devices, err := s.getUserDevices(ctx, userID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get user devices", "error", err)
		return fmt.Errorf("failed to get user devices: %w", err)
	}

	if len(devices) == 0 {
		// No active app device: fall back to an off-app channel, chosen by the
		// recipient's handle (SMS for a phone, email otherwise). dispatchOffApp
		// is best-effort and logs/counts every outcome itself.
		s.dispatchOffApp(ctx, userID, notification)
		return nil
	}

	// Determine notification type from payload
	notificationType := getNotificationType(notification)

	logger.InfoContext(ctx, "sending notification to user devices",
		"notification_type", notificationType,
		"recipient_count", len(devices),
	)

	// Build device summary for logging.
	deviceSummary := make([]string, 0, len(devices))
	for _, d := range devices {
		deviceSummary = append(deviceSummary, fmt.Sprintf("%s(%s)", d.Id, d.Platform))
	}

	// Send to all user's devices
	var errs []error
	for _, device := range devices {
		provider, exists := s.providers[device.Platform]
		if !exists {
			logger.WarnContext(ctx, "no provider for platform, skipping device",
				"platform", device.Platform,
				"device_id", device.Id,
			)
			continue
		}

		target := Target{DeviceToken: device.DeviceToken, InstallationID: device.InstallationId}
		if err := provider.Send(ctx, target, notification); err != nil {
			logger.WarnContext(ctx, "failed to send notification to device",
				"device_id", device.Id,
				"device_token", logging.MaskToken(device.DeviceToken),
				"platform", device.Platform,
				"error", err,
			)
			errs = append(errs, err)

			// If token is invalid, unregister the device automatically
			if errors.Is(err, ErrInvalidToken) {
				logger.InfoContext(ctx, "automatically removing invalid device token",
					"device_id", device.Id,
					"device_token", logging.MaskToken(device.DeviceToken),
					"platform", device.Platform,
				)
				if unregErr := s.UnregisterDevice(ctx, device.DeviceToken); unregErr != nil {
					logger.ErrorContext(ctx, "failed to unregister invalid device",
						"device_id", device.Id,
						"error", unregErr,
					)
				} else {
					logger.InfoContext(ctx, "successfully removed invalid device",
						"device_id", device.Id,
					)
				}
			}
		} else {
			// Update last_used_unix_sec on successful send
			s.updateDeviceLastUsed(ctx, device)
		}
	}

	if len(errs) == len(devices) && len(devices) > 0 {
		// If every failure was an invalid-token cleanup, the system has
		// self-healed: the dead tokens are unregistered, and the next send
		// after the client re-registers will succeed. Nothing is actionable.
		allInvalidToken := true
		for _, e := range errs {
			if !errors.Is(e, ErrInvalidToken) {
				allInvalidToken = false
				break
			}
		}
		if allInvalidToken {
			logger.WarnContext(ctx, "all devices had invalid tokens; cleaned up",
				"total_devices", len(devices),
				"outcome", "all_invalid_tokens",
			)
			return nil
		}
		logger.ErrorContext(ctx, "failed to send to all devices",
			"total_devices", len(devices),
			"failed_devices", len(errs),
			"outcome", "all_failed",
		)
		return fmt.Errorf("failed to send to all %d devices", len(devices))
	}

	if len(errs) > 0 {
		logger.WarnContext(ctx, "notification sent with partial failures",
			"total_devices", len(devices),
			"successful_devices", len(devices)-len(errs),
			"failed_devices", len(errs),
			"devices", deviceSummary,
			"outcome", "partial_failure",
		)
	} else {
		logger.InfoContext(ctx, "notification sent to all devices",
			"total_devices", len(devices),
			"devices", deviceSummary,
			"outcome", "sent",
		)
	}

	return nil
}

// HasDevices checks if a user has any registered devices.
func (s *service) HasDevices(ctx context.Context, userID string) bool {
	devices, err := s.getUserDevices(ctx, userID)
	return err == nil && len(devices) > 0
}

// UnregisterDevice removes a device token.
func (s *service) UnregisterDevice(ctx context.Context, deviceToken string) error {
	device, err := s.getDeviceByToken(ctx, deviceToken)
	if err != nil {
		// Not found is OK (idempotent)
		return nil
	}

	if err := s.storage.Delete(ctx, device); err != nil {
		// A concurrent caller may have deleted the same row between our
		// lookup and our DELETE (e.g. two NotifyUser calls cleaning up the
		// same invalid FCM token). The cleanup goal is already achieved.
		if errors.Is(err, storage.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("failed to delete device: %w", err)
	}

	return nil
}

// getUserDevices retrieves all registered devices for a user.
func (s *service) getUserDevices(ctx context.Context, userID string) ([]*models.UserDevice, error) {
	messages, err := s.storage.QueryByField(ctx, "user_id", userID, &models.UserDevice{})
	if err != nil {
		return nil, err
	}

	devices := make([]*models.UserDevice, 0, len(messages))
	for _, msg := range messages {
		devices = append(devices, msg.(*models.UserDevice))
	}

	return devices, nil
}

// getDeviceByToken finds a device by its token.
func (s *service) getDeviceByToken(ctx context.Context, deviceToken string) (*models.UserDevice, error) {
	messages, err := s.storage.QueryByField(ctx, "device_token", deviceToken, &models.UserDevice{})
	if err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("device not found")
	}

	return messages[0].(*models.UserDevice), nil
}

// updateDeviceLastUsed updates the last_used_unix_sec field for a device.
func (s *service) updateDeviceLastUsed(ctx context.Context, device *models.UserDevice) {
	logger := logging.LoggerWithContext(ctx)

	// Clone the device to avoid modifying the original
	updated := proto.Clone(device).(*models.UserDevice)
	updated.LastUsedUnixSec = time.Now().Unix()

	if err := s.storage.Update(ctx, updated); err != nil {
		logger.WarnContext(ctx, "failed to update device last_used",
			"device_id", device.Id,
			"error", err,
		)
	}
}
