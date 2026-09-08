package notifications

import (
	"context"
	"fmt"
	"net/url"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications/notification_content"
	"go.ripls.org/ripls/server/storage"
)

// localizerForUser builds a Localizer for the recipient's stored
// preferred_language. The off-app renderers compose the *entire* message, so
// they must use the recipient's locale (not the request's) — and unlike push,
// the off-app dispatch doesn't carry the per-recipient locale on the context.
// loc.T is nil-safe, so a load failure degrades to the default catalog.
func localizerForUser(user *models.User) *l10n.Localizer {
	loc, _ := l10n.NewLocalizer(l10n.Normalize(user.GetPreferredLanguage()))
	return loc
}

// EmailSender delivers an off-app notification email to a deviceless recipient.
// It is the narrow surface the notification service needs; both
// *email.MailgunService and *email.MockEmailService satisfy it.
type EmailSender interface {
	SendNotification(ctx context.Context, in email.NotificationEmailInput) error
}

// OffAppEmailConfig configures the email channel used when a recipient has no
// active app device. The zero value (Enabled false) preserves today's
// behavior: notifications to deviceless users are silently suppressed. The
// channel is gated by an explicit flag so enabling email for every deviceless
// user is a deliberate ops decision (cf. the SMS enablement gate).
type OffAppEmailConfig struct {
	Enabled     bool
	Sender      EmailSender
	AppBaseURL  string                // e.g. "https://app.example.com" — CTA + unsubscribe link host
	UnsubSecret []byte                // HMAC key for signing one-click unsubscribe links
	Bucket      storage.BucketStorage // object store for inline hero images; nil disables the photo (card still renders)
}

// Option configures the notification service at construction.
type Option func(*service)

// WithOffAppEmail wires the off-app email fallback channel. Without it, the
// service behaves exactly as before (deviceless recipients are suppressed).
func WithOffAppEmail(cfg OffAppEmailConfig) Option {
	return func(s *service) { s.offAppEmail = cfg }
}

// dispatchOffApp delivers a notification to a recipient with no registered
// device, choosing the off-app channel by handle: SMS for a phone-bearing
// recipient (the native channel for the phone-first audience), email otherwise.
// SMS falls back to email when it is off, there is no phone, it's the
// recipient's quiet hours, the number opted out, or the text failed — so the
// recipient is still reached. It is strictly best-effort: every outcome is
// logged and counted, and it never propagates an error. PII: the recipient's
// contact handle is never logged.
func (s *service) dispatchOffApp(ctx context.Context, userID string, notification *models.Notification) {
	logger := logging.LoggerWithContext(ctx).With("user_id", userID)

	emailOn := s.offAppEmail.Enabled && s.offAppEmail.Sender != nil
	smsOn := s.sms.Enabled && s.sms.Sender != nil
	if !emailOn && !smsOn {
		// Both off-app channels disabled — preserve the historical
		// suppressed-no-devices log without a recipient lookup.
		logger.InfoContext(ctx, "no devices registered for user", "outcome", "suppressed_no_devices")
		return
	}

	user := &models.User{}
	if err := s.storage.GetByID(ctx, userID, user); err != nil {
		logger.WarnContext(ctx, "off-app: failed to load recipient", "error", err,
			"outcome", "suppressed_no_handle")
		return
	}

	if s.sendOffAppSMS(ctx, user, notification) {
		return
	}
	s.sendOffAppEmail(ctx, user, notification)
}

// suppressedForSimulation reports whether the recipient is a simulation/test
// identity (the E2E suite or the load-test harness, tagged via
// User.simulation_id at registration) and, if so, logs and counts the
// suppression on the given channel. Such identities must never reach a real
// delivery provider: their synthetic addresses would burn Mailgun's shared
// sending-domain daily quota and the metered A2P SMS path on nobody real (#2588).
// The caller passes its already-scoped logger so the line keeps that channel's
// recipient field (recipient_phone for SMS, user_id for email) — re-deriving a
// logger here would instead bind the authenticated caller's user_id, which in
// off-app dispatch is not the recipient. The check is channel-accurate: it runs
// only once the channel that would actually have been used is selected, so the
// metric/log name the real channel.
func suppressedForSimulation(ctx context.Context, logger *logging.Logger, user *models.User, channel string) bool {
	if user.GetSimulationId() == "" {
		return false
	}
	logger.InfoContext(ctx, "off-app: simulation recipient; suppressing send",
		"simulation_id", user.GetSimulationId(),
		"channel", channel, "outcome", "suppressed_simulation")
	return true
}

// sendOffAppEmail delivers the off-app notification email to an already-loaded
// recipient. Best-effort: every outcome (channel disabled, no email on file,
// opted out, send failure) is logged and counted, and no error propagates. PII:
// the recipient's email address is never logged.
func (s *service) sendOffAppEmail(ctx context.Context, user *models.User, notification *models.Notification) {
	logger := logging.LoggerWithContext(ctx).With("user_id", user.GetId())

	// Simulation/test recipients routed to the email channel are suppressed
	// before any provider call (#2588). Placed here — after the channel is the
	// chosen one — so the suppression is recorded as the email channel.
	if suppressedForSimulation(ctx, logger, user, "email") {
		return
	}

	if !s.offAppEmail.Enabled || s.offAppEmail.Sender == nil {
		// Reached only when SMS was the sole configured channel and didn't send.
		logger.InfoContext(ctx, "off-app email: channel disabled; nothing to send",
			"outcome", "suppressed_disabled")
		return
	}

	if user.Email == "" {
		logger.InfoContext(ctx, "off-app email: recipient has no email on file",
			"outcome", "suppressed_no_handle")
		return
	}

	if user.OffAppEmailOptedOut {
		logger.InfoContext(ctx, "off-app email: recipient opted out",
			"outcome", "suppressed_opted_out")
		return
	}

	in := email.NotificationEmailInput{
		ToEmail:           user.Email,
		PreferredLanguage: user.GetPreferredLanguage(),
		Title:             notification.GetTitle(),
		Body:              notification.GetBody(),
		ActionURL:         s.notificationLink(ctx, s.offAppEmail.AppBaseURL, notification),
		UnsubscribeURL:    s.unsubscribeURL(user.GetId()),
	}
	// Community events render self-contained, surface-specific copy (entity
	// inline + a CTA-labeled button) instead of the push title/body. The heading
	// carries the full message, so Body is cleared. Rendered in the recipient's
	// locale (chat keeps the push title/body fallback above).
	if ev := notification.GetCommunityEvent(); ev != nil {
		loc := localizerForUser(user)
		parts := notification_content.FromCommunityEvent(ctx, ev).RenderEmail(ctx, loc)
		in.Title = parts.Heading
		in.Body = ""
		in.CTALabel = parts.CTALabel
		// Enrich with the entity's themed card (photo + summary), the email
		// analogue of the SSR landing page. The card heading becomes the entity
		// name, so the subject (the action sentence) and the heading differ.
		in.Summary, in.InlineImages, in.HeroImageURL = s.buildEmailExtras(ctx, ev, loc)
	}

	if err := s.offAppEmail.Sender.SendNotification(ctx, in); err != nil {
		logger.ErrorContext(ctx, "off-app email: send failed", "error", err, "outcome", "failed")
		return
	}

	logger.InfoContext(ctx, "off-app email sent to deviceless recipient", "outcome", "sent")
}

// unsubscribeURL builds the signed one-click unsubscribe link embedded in
// off-app emails. The endpoint (server/services/web) recomputes the token to
// authorize the opt-out, so the link is tamper-proof and stateless.
func (s *service) unsubscribeURL(userID string) string {
	token := auth.SignUnsubscribeToken(s.offAppEmail.UnsubSecret, userID)
	return fmt.Sprintf("%s/email/unsubscribe?u=%s&token=%s",
		s.offAppEmail.AppBaseURL, url.QueryEscape(userID), token)
}
