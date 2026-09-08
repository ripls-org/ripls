// One-click unsubscribe for the off-app email notification channel.
//
// Off-app emails embed a signed link of the form
// /email/unsubscribe?u={subject}&token={hmac}. The subject is either a registered
// user's id (sets off_app_email_opted_out on the User) or — for an invite email
// to someone with no account — their email address (recorded in the email
// suppression list). Both are durable CAN-SPAM suppression records. The token is
// a stateless HMAC (auth.SignUnsubscribeToken) so the link is tamper-proof
// without any server-side token store.

package web

import (
	"errors"
	"net/http"
	"strings"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/emailsuppression"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// HandleEmailUnsubscribe opts a recipient out of off-app email.
// Wired to GET /email/unsubscribe?u={subject}&token={hmac}. Returns 200 with a
// short confirmation page on success (and on a repeat click — opting out is
// idempotent), 400 on a missing/invalid token, 404 when a user subject is gone,
// and 500 on an internal failure. The subject is never echoed back to the page,
// and an email subject is masked in logs.
func (s *Service) HandleEmailUnsubscribe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	subject := r.URL.Query().Get("u")
	token := r.URL.Query().Get("token")

	logger := logging.LoggerWithContext(ctx).With("operation", "EmailUnsubscribe")

	if subject == "" || token == "" {
		logger.InfoContext(ctx, "unsubscribe rejected: missing subject or token")
		http.Error(w, "invalid unsubscribe link", http.StatusBadRequest)
		return
	}

	if !auth.VerifyUnsubscribeToken(s.unsubscribeSecret, subject, token) {
		logger.InfoContext(ctx, "unsubscribe rejected: invalid token")
		http.Error(w, "invalid unsubscribe link", http.StatusBadRequest)
		return
	}

	// An email-address subject is an off-app invitee with no account; record the
	// opt-out in the email suppression list. A bare id is a registered user.
	if strings.Contains(subject, "@") {
		if err := emailsuppression.Suppress(ctx, s.storage, subject); err != nil {
			logger.ErrorContext(ctx, "unsubscribe: failed to suppress email",
				"error", err, "email", logging.MaskEmail(subject))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		logger.InfoContext(ctx, "invitee email opted out of off-app email",
			"email", logging.MaskEmail(subject))
		s.writeUnsubscribeConfirmation(w)
		return
	}

	user := &models.User{}
	if err := s.storage.GetByID(ctx, subject, user); err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			logger.InfoContext(ctx, "unsubscribe rejected: user not found", "user_id", subject)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		logger.ErrorContext(ctx, "unsubscribe: failed to load user", "error", err, "user_id", subject)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Idempotent: a repeat click on an already-opted-out user still confirms.
	if !user.OffAppEmailOptedOut {
		user.OffAppEmailOptedOut = true
		if err := s.storage.Update(ctx, user); err != nil {
			logger.ErrorContext(ctx, "unsubscribe: failed to persist opt-out", "error", err, "user_id", subject)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		logger.InfoContext(ctx, "user opted out of off-app email notifications", "user_id", subject)
	}

	s.writeUnsubscribeConfirmation(w)
}

// writeUnsubscribeConfirmation renders the shared opt-out confirmation page.
func (s *Service) writeUnsubscribeConfirmation(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(unsubscribeConfirmationHTML))
}

const unsubscribeConfirmationHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>Unsubscribed</title></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 40px auto; padding: 20px; text-align: center; color: #3d3531;">
<h1 style="font-size: 22px;">You're unsubscribed</h1>
<p>You will no longer receive Ripls email notifications. You can still get notifications in the Ripls app.</p>
</body></html>`
