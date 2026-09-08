package notifications

import (
	"context"
	"time"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/contact"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications/notification_content"
	"go.ripls.org/ripls/server/quiethours"
	"go.ripls.org/ripls/server/smsoptout"
)

// optOutDisclosureInterval is how often the STOP/HELP footer re-appears on a
// recurring platform SMS thread after the first message. CTIA asks for the
// reminder in the opt-in/first message and "conspicuously and frequently"
// thereafter; ~monthly is the common best-practice cadence. The keywords are
// honored centrally regardless of the rendered text, so this only governs copy.
const optOutDisclosureInterval = 30 * 24 * time.Hour

// shouldDiscloseOptOut reports whether this platform SMS should carry the
// STOP/HELP footer: always on the user's first one (timestamp 0/unset), then
// only once the interval has elapsed since the last disclosure.
func shouldDiscloseOptOut(user *models.User, now time.Time) bool {
	last := user.GetSmsOptoutDisclosedAtUnixSec()
	if last == 0 {
		return true
	}
	return now.Sub(time.Unix(last, 0)) >= optOutDisclosureInterval
}

// SMSSender delivers a platform text message to an E.164 number. It is the
// narrow surface the notification service needs; both *sms.TwilioSMSSender and
// *sms.MockSMSSender satisfy it. SMS has no UserDevice row, so it does not fit
// the providers map[DevicePlatform]Provider model and is wired as a separate
// channel on the service.
type SMSSender interface {
	SendSMS(ctx context.Context, toE164, body string) error
}

// SMSChannel configures the text-message channel used when a deviceless
// recipient has a phone handle. The zero value (Enabled false) preserves
// today's behavior. The channel is gated by an explicit flag so enabling
// platform SMS is a deliberate ops decision made only after A2P registration
// and counsel sign-off.
type SMSChannel struct {
	Enabled    bool
	Sender     SMSSender
	AppBaseURL string // branded link included in the message body, e.g. "https://app.example.com"
}

// WithSMS wires the platform SMS channel. Without it, the service never sends
// text messages.
func WithSMS(cfg SMSChannel) Option {
	return func(s *service) { s.sms = cfg }
}

// sendOffAppSMS attempts to text a deviceless recipient and reports whether a
// message was actually sent. It returns false — letting the dispatcher fall
// back to email — when the channel is disabled, the recipient has no phone, it
// is the recipient's quiet hours, the number opted out, or the send failed. It
// is best-effort and never propagates an error. PII: the number is masked in
// logs; the body is never logged.
func (s *service) sendOffAppSMS(ctx context.Context, user *models.User, notification *models.Notification) bool {
	if !s.sms.Enabled || s.sms.Sender == nil {
		return false // channel off — let email handle it
	}
	phone := user.GetPhoneNumber()
	if phone == "" {
		return false // no phone handle — email is the right channel
	}

	logger := logging.LoggerWithContext(ctx).With("recipient_phone", logging.MaskPhone(phone))

	// Simulation/test recipients are suppressed before any provider call (#2588).
	// Placed here — once SMS is the chosen channel (enabled + phone present) — so
	// the suppression is recorded as the SMS channel. Return true so the caller
	// does not fall back to email: a simulation recipient must reach no provider.
	if suppressedForSimulation(ctx, logger, user, "sms") {
		return true
	}

	// Skip numbers that are shape-valid (so they pass NormalizePhoneE164 at
	// sign-up) but not actually dialable — e.g. Firebase Phone Auth's fictional
	// OTP test numbers or any +1 555-area-code number. The provider rejects
	// these (Twilio 21211), so detecting them here saves a metered API call,
	// avoids an ERROR-level provider rejection in the logs, and falls back to
	// email immediately. This is data-quality, not a fault: log at WARN. Sign-up
	// deliberately still accepts them (Firebase test numbers must keep working).
	if !contact.IsDialableE164(phone) {
		logger.WarnContext(ctx, "off-app sms: recipient number not dialable; falling back to email", "outcome", "suppressed_invalid_number")
		return false
	}

	// Real-time sends can't defer like the scheduled dispatcher, so a text that
	// would land in the recipient's quiet hours falls back to email (untimed)
	// rather than buzzing a phone at 3am.
	if quiethours.InWindow(user.GetPreferredTimezone(), clock.Now(ctx)) {
		logger.InfoContext(ctx, "off-app sms: recipient quiet hours; falling back to email", "outcome", "suppressed_quiet_hours")
		return false
	}

	// Fail closed: if we cannot confirm the number is opted in, do not text it.
	optedOut, err := smsoptout.IsOptedOut(ctx, s.storage, phone)
	if err != nil {
		logger.ErrorContext(ctx, "off-app sms: opt-out check failed; falling back to email", "error", err, "outcome", "failed")
		return false
	}
	if optedOut {
		logger.InfoContext(ctx, "off-app sms: recipient opted out; falling back to email", "outcome", "suppressed_opted_out")
		return false
	}

	now := clock.Now(ctx)
	includeOptOut := shouldDiscloseOptOut(user, now)
	body := s.renderSMSBody(ctx, notification, localizerForUser(user), includeOptOut)
	if err := s.sms.Sender.SendSMS(ctx, phone, body); err != nil {
		logger.ErrorContext(ctx, "off-app sms: send failed; falling back to email", "error", err, "outcome", "failed")
		return false
	}

	// Record that this send carried the opt-out disclosure so the next one within
	// the interval can omit it. Best-effort: a failed write just means the footer
	// rides one more message (the conservative direction).
	if includeOptOut {
		user.SmsOptoutDisclosedAtUnixSec = now.Unix()
		if err := s.storage.Update(ctx, user); err != nil {
			logger.WarnContext(ctx, "off-app sms: failed to record opt-out disclosure timestamp", "error", err)
		}
	}

	logger.InfoContext(ctx, "off-app sms sent to deviceless recipient", "outcome", "sent")
	return true
}

// renderSMSBody composes the outbound text via the server l10n catalog (never
// inline copy). For a community event it uses the surface-specific renderer
// (notification_content): a self-contained, entity-inline message branded
// "Ripls:", the STOP/HELP footer, and a CTA on the resolved /go link. Other
// notification types (e.g. chat) fall back to the legacy wrapper around the
// already-rendered body. loc is the recipient's locale (the whole SMS is
// rendered here, so it must match the recipient — not the request — language).
func (s *service) renderSMSBody(ctx context.Context, notification *models.Notification, loc *l10n.Localizer, includeOptOut bool) string {
	link := s.notificationLink(ctx, s.sms.AppBaseURL, notification)

	if ev := notification.GetCommunityEvent(); ev != nil {
		return notification_content.FromCommunityEvent(ctx, ev).RenderSMS(ctx, loc, link, includeOptOut)
	}

	// Fallback for non-community-event notifications: the legacy wrapper around
	// the already-localized body (title when body is empty). The opt-out footer
	// is appended here on the same first/periodic cadence as the renderer.
	message := notification.GetBody()
	if message == "" {
		message = notification.GetTitle()
	}
	body := loc.T(ctx, "sms.notification.body", map[string]any{"Message": message, "Url": link})
	if includeOptOut {
		body += "\n\n" + loc.T(ctx, "sms.optout", nil)
	}
	return body
}
