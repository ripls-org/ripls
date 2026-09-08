package notifications

import (
	"context"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/smsoptout"
)

// SendPhoneOptInWelcome sends the one-time CTIA/RCS opt-in confirmation
// ("welcome") text to a freshly verified phone number via the platform SMS/RCS
// channel (the Twilio Messaging Service tries RCS first and falls back to SMS).
//
// It differs from a content notification in three deliberate ways:
//   - It always targets the phone (never push): the recipient just proved
//     ownership of this number and has no registered device yet.
//   - It bypasses quiet hours: it is a real-time response to the user's own
//     opt-in action — the same action that just delivered their OTP — so it is
//     never an unsolicited 3am buzz.
//   - It carries its own complete STOP/HELP/frequency/rate disclosure (the
//     sms.welcome.body catalog string), so no opt-out footer is appended; a
//     successful send records the disclosure so the next platform SMS can omit
//     it until the ~monthly cadence elapses.
//
// STOP is still honored (fail closed). Best-effort: a disabled channel, missing
// phone, prior opt-out, or send error returns without panicking and never
// blocks the caller's registration.
func (s *service) SendPhoneOptInWelcome(ctx context.Context, user *models.User) error {
	if !s.sms.Enabled || s.sms.Sender == nil {
		return nil // platform SMS not enabled — nothing to send
	}
	phone := user.GetPhoneNumber()
	if phone == "" {
		return nil // no phone handle
	}

	logger := logging.LoggerWithContext(ctx).With("recipient_phone", logging.MaskPhone(phone))

	// Simulation/test recipients never reach a provider (#2588).
	if suppressedForSimulation(ctx, logger, user, "sms") {
		return nil
	}

	// Fail closed: never text a number we cannot confirm is opted in.
	optedOut, err := smsoptout.IsOptedOut(ctx, s.storage, phone)
	if err != nil {
		logger.ErrorContext(ctx, "welcome sms: opt-out check failed; skipping", "error", err, "outcome", "failed")
		return err
	}
	if optedOut {
		logger.InfoContext(ctx, "welcome sms: recipient opted out; skipping", "outcome", "suppressed_opted_out")
		return nil
	}

	// The welcome is rendered whole from the server l10n catalog (never inline
	// copy) in the recipient's locale, and already contains the opt-out footer.
	//
	// AppURL is a param rather than baked into the catalog string so the link
	// points at whichever deployment sent the message (#2953). The catalog drops
	// the whole "Connect at …" sentence when it is empty — the CTIA disclosure
	// that makes this message compliant stands on its own.
	body := localizerForUser(user).T(ctx, "sms.welcome.body", map[string]any{
		"AppURL": s.sms.AppBaseURL,
	})
	if err := s.sms.Sender.SendSMS(ctx, phone, body); err != nil {
		logger.ErrorContext(ctx, "welcome sms: send failed", "error", err, "outcome", "failed")
		return err
	}

	// Record that this send carried the opt-out disclosure so the next platform
	// SMS within the interval can omit it. Best-effort: a failed write just means
	// the footer rides one more message (the conservative direction).
	user.SmsOptoutDisclosedAtUnixSec = clock.Now(ctx).Unix()
	if err := s.storage.Update(ctx, user); err != nil {
		logger.WarnContext(ctx, "welcome sms: failed to record opt-out disclosure timestamp", "error", err)
	}

	logger.InfoContext(ctx, "welcome sms sent on opt-in", "outcome", "sent")
	return nil
}
