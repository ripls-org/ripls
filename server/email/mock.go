package email

import (
	"context"
	"time"

	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
)

// MockEmailService is a mock implementation of EmailService for testing.
type MockEmailService struct {
	ResetEmails           []ResetEmail
	EmailCodes            []Code
	WaitlistWelcomes      []WaitlistWelcome
	WaitlistNotifications []WaitlistNotification
	ActivityDigests       []RecordedActivityDigest
	Notifications         []NotificationEmailInput
	OnEmailSent           chan struct{} // Optional channel signaled when any email is sent

	// SendEmailCodeErr, when set, makes SendEmailCode fail with it. The
	// sign-in-code send is the one email path whose failure must reach the
	// caller rather than being swallowed, so tests need a way to force it.
	SendEmailCodeErr error
}

// Code records a captured SendEmailCode call. Tests read Code from here to
// complete the verification flow without a mailbox.
type Code struct {
	ToEmail           string
	Code              string
	ExpiresIn         time.Duration
	PreferredLanguage string
}

// LastCodeFor returns the most recently issued code for an address, and
// whether one was found. Codes are re-issued on every request, so the last
// one is the only live one.
func (m *MockEmailService) LastCodeFor(toEmail string) (string, bool) {
	for i := len(m.EmailCodes) - 1; i >= 0; i-- {
		if m.EmailCodes[i].ToEmail == toEmail {
			return m.EmailCodes[i].Code, true
		}
	}
	return "", false
}

// WaitlistWelcome records a captured SendWaitlistWelcome call,
// including the resolved locale (so tests can verify that callers
// propagate Accept-Language correctly into detached goroutines).
type WaitlistWelcome struct {
	ToEmail string
	Locale  string
}

// RecordedActivityDigest records a captured SendActivityDigest call.
type RecordedActivityDigest struct {
	ToEmail string
	Input   ActivityDigestInput
}

// ResetEmail records the details of a password reset email.
type ResetEmail struct {
	ToEmail           string
	ResetURL          string
	PreferredLanguage string
}

// WaitlistNotification records a notification sent to the developer.
type WaitlistNotification struct {
	SignupEmail string
	NotifyEmail string
	Details     WaitlistSignupDetails
}

// signalSent signals that an email was sent, if a listener is configured.
func (m *MockEmailService) signalSent() {
	if m.OnEmailSent != nil {
		select {
		case m.OnEmailSent <- struct{}{}:
		default:
			// Channel full or not being read, skip
		}
	}
}

// SendPasswordReset records the email details instead of actually sending.
func (m *MockEmailService) SendPasswordReset(ctx context.Context, toEmail, resetURL, preferredLanguage string) error {
	logger := logging.LoggerWithContext(ctx)

	logger.Info("mock email service: recording email",
		"recipient_email", logging.MaskEmail(toEmail),
		"email_type", "password_reset",
		"recipient_count", 1,
	)

	m.ResetEmails = append(m.ResetEmails, ResetEmail{
		ToEmail:           toEmail,
		ResetURL:          resetURL,
		PreferredLanguage: preferredLanguage,
	})
	m.signalSent()
	return nil
}

// SendEmailCode records the code instead of sending it, or fails with
// SendEmailCodeErr when the test has set one.
func (m *MockEmailService) SendEmailCode(ctx context.Context, toEmail, code string, expiresIn time.Duration, preferredLanguage string) error {
	logger := logging.LoggerWithContext(ctx)

	if m.SendEmailCodeErr != nil {
		logger.Info("mock email service: failing email code send",
			"recipient_email", logging.MaskEmail(toEmail),
			"email_type", "email_code",
		)
		return m.SendEmailCodeErr
	}

	// The code is deliberately absent from this log line: production
	// implementations must never log it, and the mock keeps that habit so
	// a copied log statement can't leak one.
	logger.Info("mock email service: recording email code",
		"recipient_email", logging.MaskEmail(toEmail),
		"email_type", "email_code",
		"recipient_count", 1,
	)

	m.EmailCodes = append(m.EmailCodes, Code{
		ToEmail:           toEmail,
		Code:              code,
		ExpiresIn:         expiresIn,
		PreferredLanguage: preferredLanguage,
	})
	m.signalSent()
	return nil
}

// SendWaitlistWelcome records a waitlist welcome email.
func (m *MockEmailService) SendWaitlistWelcome(ctx context.Context, toEmail string) error {
	logger := logging.LoggerWithContext(ctx)

	logger.Info("mock email service: recording waitlist welcome",
		"recipient_email", logging.MaskEmail(toEmail),
		"email_type", "waitlist_welcome",
	)

	m.WaitlistWelcomes = append(m.WaitlistWelcomes, WaitlistWelcome{
		ToEmail: toEmail,
		Locale:  l10n.LocaleFromContext(ctx).String(),
	})
	m.signalSent()
	return nil
}

// SendWaitlistNotification records a waitlist notification.
func (m *MockEmailService) SendWaitlistNotification(ctx context.Context, notifyEmail string, details WaitlistSignupDetails) error {
	logger := logging.LoggerWithContext(ctx)

	logger.Info("mock email service: recording waitlist notification",
		"signup_email", logging.MaskEmail(details.Email),
		"notify_email", logging.MaskEmail(notifyEmail),
		"email_type", "waitlist_notification",
	)

	m.WaitlistNotifications = append(m.WaitlistNotifications, WaitlistNotification{
		SignupEmail: details.Email,
		NotifyEmail: notifyEmail,
		Details:     details,
	})
	m.signalSent()
	return nil
}

// SendActivityDigest records the activity digest instead of sending it.
func (m *MockEmailService) SendActivityDigest(ctx context.Context, toEmail string, in ActivityDigestInput) error {
	logger := logging.LoggerWithContext(ctx)
	logger.Info("mock email service: recording activity digest",
		"recipient_email", logging.MaskEmail(toEmail),
		"email_type", "activity_digest",
		"new_user_count", len(in.NewUsers),
		"interactive_signin_count", in.InteractiveSignInCount,
		"community_count", len(in.Communities),
		"total_community_event_count", in.TotalMemberActionCount,
		"total_user_message_count", in.TotalUserMessageCount,
	)
	m.ActivityDigests = append(m.ActivityDigests, RecordedActivityDigest{
		ToEmail: toEmail,
		Input:   in,
	})
	m.signalSent()
	return nil
}

// SendNotification records an off-app notification email instead of sending it.
func (m *MockEmailService) SendNotification(ctx context.Context, in NotificationEmailInput) error {
	logger := logging.LoggerWithContext(ctx)
	logger.Info("mock email service: recording off-app notification",
		"recipient_email", logging.MaskEmail(in.ToEmail),
		"email_type", "off_app_notification",
	)
	m.Notifications = append(m.Notifications, in)
	m.signalSent()
	return nil
}

// Reset clears all recorded emails.
func (m *MockEmailService) Reset() {
	m.ResetEmails = nil
	m.EmailCodes = nil
	m.WaitlistWelcomes = nil
	m.WaitlistNotifications = nil
	m.ActivityDigests = nil
	m.Notifications = nil
}

// CheckHealth returns mock health status.
func (m *MockEmailService) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	return []*health.Status{{Name: "email", Backend: "mock"}}, nil
}
