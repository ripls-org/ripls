package main

// Notification-delivery channel assembly for the server binary: the email
// service (Mailgun or mock), the push providers (FCM or noop), and the
// off-app email/SMS channels that reach recipients without an active app
// device. Consumed by wireServices (wiring.go).

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"

	"go.ripls.org/ripls/server/config"
	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/notifications/fcm"
	"go.ripls.org/ripls/server/notifications/noop"
	"go.ripls.org/ripls/server/notifications/sms"
	"go.ripls.org/ripls/server/storage"
)

// initializeNotificationProviders initializes notification providers based on
// the selected provider name. In fcm mode the providers share the given
// Firebase app (the same one that backs phone auth); test mode ignores it.
func initializeNotificationProviders(ctx context.Context, providerName string, firebaseApp *firebase.App, logger *logging.Logger) ([]notifications.Provider, error) {
	switch providerName {
	case "fcm":
		// Use FCM for both iOS and Android, sharing a single Firebase app and messaging client
		logger.Info("initializing FCM notification providers")

		client, err := fcm.NewMessagingClient(ctx, firebaseApp)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize FCM messaging client: %w", err)
		}

		iosProvider := fcm.NewProvider(client, models.DevicePlatform_DEVICE_PLATFORM_IOS)
		androidProvider := fcm.NewProvider(client, models.DevicePlatform_DEVICE_PLATFORM_ANDROID)

		logger.Info("FCM notification providers initialized")
		return []notifications.Provider{iosProvider, androidProvider}, nil

	case "test", "noop": // "noop" kept for backwards compatibility
		// Use noop providers for testing/development
		logger.Info("using noop notification providers", "mode", "testing")
		return []notifications.Provider{
			noop.NewProvider(models.DevicePlatform_DEVICE_PLATFORM_IOS),
			noop.NewProvider(models.DevicePlatform_DEVICE_PLATFORM_ANDROID),
		}, nil

	default:
		return nil, fmt.Errorf("unknown notification provider: %s (must be 'fcm' or 'test')", providerName)
	}
}

// buildNotificationChannels constructs the email service and the notification
// service with its optional off-app channels (email fallback, platform SMS).
// resolveShareLink is late-bound: the caller assigns it once the community
// service exists, and the share-link closure reads it only at send time.
func buildNotificationChannels(ctx context.Context, cfg *config.Config, logger *logging.Logger, firebaseApp *firebase.App, sqlStorage *storage.ProtoSQLStorage, bucketStorage storage.BucketStorage, resolveShareLink *notifications.ShareLinkResolver) (notifications.Service, email.Service, error) {
	// Initialize email service (also backs the off-app notification channel below).
	var emailService email.Service
	if cfg.MailgunAPIKey != "" {
		logger.Info("initializing Mailgun email service", "domain", cfg.MailgunDomain)
		mailgunSvc, err := email.NewMailgunService(cfg.MailgunDomain, cfg.MailgunAPIKey, cfg.MailgunFromAddress)
		if err != nil {
			logger.Error("failed to initialize Mailgun email service", "error", err)
			return nil, nil, err
		}
		// CAN-SPAM postal address for the off-app notification footer (single
		// source of truth; required before --off-app-email-enabled flips on).
		mailgunSvc.SetPostalAddress(cfg.MailgunPostalAddress)
		// Stamp the environment on outbound mail so the delivery webhook can
		// ignore the other environment's events — dev and prod share one
		// Mailgun domain, so both receive everything.
		mailgunSvc.SetEnvironmentTag(cfg.InviteLinkHostname)
		// Store listings are per-deployment; unset omits the download CTA.
		mailgunSvc.SetStoreLinks(cfg.Branding.AppStoreURL, cfg.Branding.PlayStoreURL)
		// Operator name signed at the foot of the waitlist welcome; unset
		// omits the line rather than signing as someone else.
		mailgunSvc.SetLegalEntityName(cfg.Branding.LegalEntityName)
		emailService = mailgunSvc
		logger.Info("Mailgun email service initialized")
	} else {
		logger.Info("Mailgun API key not set, using mock email service")
		emailService = &email.MockEmailService{}
	}

	// Initialize notification providers
	notificationProviders, err := initializeNotificationProviders(ctx, cfg.NotificationProvider, firebaseApp, logger)
	if err != nil {
		logger.Error("failed to initialize notification providers", "error", err)
		return nil, nil, err
	}
	// When enabled (EMAIL-1), the notification service falls back to email for
	// recipients with no active app device. Off by default; the unsubscribe link
	// is signed with the JWT secret and verified by the web service.
	var notificationOpts []notifications.Option
	if cfg.OffAppEmailEnabled {
		notificationOpts = append(notificationOpts, notifications.WithOffAppEmail(notifications.OffAppEmailConfig{
			Enabled:     true,
			Sender:      emailService,
			AppBaseURL:  "https://" + cfg.InviteLinkHostname,
			UnsubSecret: []byte(cfg.JWTSigningSecret),
			Bucket:      bucketStorage, // inline hero images in the rich email card
		}))
		logger.Info("off-app email notification channel enabled")
	}
	// Platform SMS channel (#2492): when Twilio credentials are present, a
	// deviceless recipient with a phone handle can be texted. Empty creds leave
	// the channel unwired; the flag gates whether a wired channel actually sends
	// (off until A2P registration + counsel sign-off). The same auth token also
	// verifies inbound STOP/HELP/START webhook signatures (wired in routes.go).
	if cfg.TwilioAccountSID != "" && cfg.TwilioAuthToken != "" && cfg.TwilioMessagingSID != "" {
		notificationOpts = append(notificationOpts, notifications.WithSMS(notifications.SMSChannel{
			Enabled:    cfg.PlatformSMSEnabled,
			Sender:     sms.NewTwilioSMSSender(cfg.TwilioAccountSID, cfg.TwilioAuthToken, cfg.TwilioMessagingSID),
			AppBaseURL: "https://" + cfg.InviteLinkHostname,
		}))
		if cfg.PlatformSMSEnabled {
			logger.Info("platform SMS notification channel enabled")
		} else {
			logger.Info("platform SMS credentials present but channel flag is off")
		}
	}
	// Off-app notifications (SMS/email) carry a short `/go` share link to the
	// entity they're about, resolved by the community service. That service is
	// built after this one but depends on it, so the resolver is late-bound
	// through this closure: the caller assigns *resolveShareLink once the
	// community service exists, and reads happen only at send time.
	notificationOpts = append(notificationOpts, notifications.WithShareLinkResolver(
		func(ctx context.Context, communityID, experienceID, gearID, requestID string) (string, error) {
			if *resolveShareLink == nil {
				return "", nil // not yet wired — sender falls back to the bare app URL
			}
			return (*resolveShareLink)(ctx, communityID, experienceID, gearID, requestID)
		},
	))
	return notifications.NewService(notificationProviders, sqlStorage, notificationOpts...), emailService, nil
}
