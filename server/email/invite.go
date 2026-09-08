package email

import (
	"context"
	"fmt"
	"net/url"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
)

// InviteEmailInput carries one off-app invite email to an email invitee (a
// provisional member with no account). The subject and body are rendered here
// in the recipient's locale; the open link doubles as the call-to-action.
type InviteEmailInput struct {
	ToEmail           string
	RecipientName     string
	HostName          string
	ShareURL          string
	PreferredLanguage string // usually "" for an invitee → ctx/Accept-Language
}

// SuppressionChecker reports whether an email address has opted out of platform
// email. Injected so the email package stays storage-free.
type SuppressionChecker func(ctx context.Context, email string) (bool, error)

// InviteSender renders and sends CAN-SPAM-compliant invite emails to email
// invitees, honoring the email-handle suppression list. It composes the shared
// off-app notification renderer (branded shell + footer) with invite copy and a
// one-click unsubscribe signed on the recipient's email handle.
type InviteSender struct {
	sender       Service
	appBaseURL   string
	unsubSecret  []byte
	isSuppressed SuppressionChecker
}

// NewInviteSender builds an InviteSender. isSuppressed may be nil (no
// suppression gate, e.g. in tests).
func NewInviteSender(sender Service, appBaseURL string, unsubSecret []byte, isSuppressed SuppressionChecker) *InviteSender {
	return &InviteSender{
		sender:       sender,
		appBaseURL:   appBaseURL,
		unsubSecret:  unsubSecret,
		isSuppressed: isSuppressed,
	}
}

// SendInviteEmail renders the invite in the recipient's locale and sends it,
// unless the address has opted out (in which case it is skipped without error).
func (s *InviteSender) SendInviteEmail(ctx context.Context, in InviteEmailInput) error {
	logger := logging.LoggerWithContext(ctx)

	if s.isSuppressed != nil {
		suppressed, err := s.isSuppressed(ctx, in.ToEmail)
		if err != nil {
			logger.WarnContext(ctx, "invite email: suppression check failed",
				"error", err, "to", logging.MaskEmail(in.ToEmail))
			return err
		}
		if suppressed {
			logger.InfoContext(ctx, "invite email: recipient opted out, skipping",
				"to", logging.MaskEmail(in.ToEmail))
			return nil
		}
	}

	tag := localeForRecipient(ctx, in.PreferredLanguage)
	loc, err := l10n.NewLocalizer(tag)
	if err != nil {
		return fmt.Errorf("invite email localizer: %w", err)
	}
	data := map[string]any{"HostName": in.HostName}
	subject := loc.T(ctx, "email.invite.subject", data)
	body := loc.T(ctx, "email.invite.body", data)

	return s.sender.SendNotification(ctx, NotificationEmailInput{
		ToEmail:           in.ToEmail,
		PreferredLanguage: in.PreferredLanguage,
		Title:             subject,
		Body:              body,
		ActionURL:         in.ShareURL,
		UnsubscribeURL:    s.unsubscribeURL(in.ToEmail),
		ColdInvite:        true,
	})
}

// unsubscribeURL builds the one-click unsubscribe link for an email handle,
// signed so the web endpoint can authorize the opt-out statelessly. Returns ""
// when no signing secret is configured (the renderer then omits the link).
func (s *InviteSender) unsubscribeURL(email string) string {
	if len(s.unsubSecret) == 0 {
		return ""
	}
	token := auth.SignUnsubscribeToken(s.unsubSecret, email)
	return fmt.Sprintf("%s/email/unsubscribe?u=%s&token=%s",
		s.appBaseURL, url.QueryEscape(email), token)
}
