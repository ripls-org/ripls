package noop

import (
	"context"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications"
)

// Provider is a no-op notification provider that only logs notifications
// Useful for testing without requiring FCM/APNs setup.
type Provider struct {
	platform models.DevicePlatform
}

// NewProvider creates a new no-op provider for the given platform.
func NewProvider(platform models.DevicePlatform) *Provider {
	return &Provider{
		platform: platform,
	}
}

// Send logs the notification without actually sending it.
func (p *Provider) Send(ctx context.Context, target notifications.Target, notification *models.Notification) error {
	logger := logging.LoggerWithContext(ctx)
	logger.InfoContext(ctx, "noop notification (not actually sent)",
		"device_token", logging.MaskToken(target.DeviceToken),
		"has_installation_id", target.InstallationID != "",
		"platform", p.platform,
		"title", notification.Title,
		"body", notification.Body,
		"has_payload", notification.Payload != nil,
	)
	return nil
}

// Platform returns the platform this provider handles.
func (p *Provider) Platform() models.DevicePlatform {
	return p.platform
}
