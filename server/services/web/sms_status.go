// Outbound SMS delivery status callback — observability-only.
//
// When the server sends a platform SMS (server/notifications/sms), Twilio later
// reports that message's delivery lifecycle (queued → sent → delivered, or
// undelivered/failed with an ErrorCode) asynchronously to a separate
// status-callback URL. This handler is the receiver: it signature-verifies the
// callback (same HMAC-SHA1 X-Twilio-Signature scheme as the inbound opt-out
// webhook) and emits a structured log line per status — ERROR for
// undelivered/failed (so Cloud Logging's "Server Error Logged" alert fires and a
// log-based metric can count delivery outcomes by status), INFO for delivered,
// DEBUG for the intermediate lifecycle states. It records no state and sends no
// reply; it exists purely to surface deliverability (carrier filtering, opted-out
// recipients, unreachable numbers) that the synchronous send result never sees.
// The payload carries no From/Body — the only PII is the To number, which is
// masked in every log line.

package web

import (
	"net/http"

	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications/sms"
)

// smsStatusPath is the fixed public path Twilio's Messaging Service is configured
// to POST delivery status callbacks to. It is reconstructed (with the service
// hostname) into the URL that the inbound signature is verified against — so it
// must match the callback URL configured in Twilio exactly.
const smsStatusPath = "/sms/status"

// HandleSMSStatusCallback records the delivery outcome of an outbound platform
// SMS. Wired to POST /sms/status. It is observability-only: it verifies the
// Twilio signature, then logs MessageStatus (with the Twilio ErrorCode on
// failure) at a level chosen by the outcome, and acknowledges with 204 No
// Content. It changes no state and never replies. Always returns 2xx once
// authenticated so Twilio does not retry the callback.
func (s *Service) HandleSMSStatusCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.LoggerWithContext(ctx).With("operation", "SMSStatusCallback")

	if err := r.ParseForm(); err != nil {
		logger.InfoContext(ctx, "sms status callback: malformed form", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	publicURL := "https://" + s.hostname + smsStatusPath
	if !sms.ValidateSignature(s.smsAuthToken, publicURL, r.PostForm, r.Header.Get("X-Twilio-Signature")) {
		logger.WarnContext(ctx, "sms status callback: invalid signature; rejecting")
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if s.smsAuthToken == "" {
		logger.WarnContext(ctx, "sms status callback: signature verification disabled (no auth token configured)")
	}

	// The status payload carries no From/Body; To is the only PII and is masked.
	messageStatus := r.PostForm.Get("MessageStatus")
	logger = logger.With(
		"message_sid", r.PostForm.Get("MessageSid"),
		"message_status", messageStatus,
		"recipient_phone", logging.MaskPhone(r.PostForm.Get("To")),
	)
	// ErrorCode is a Twilio numeric error code (e.g. 30007 carrier filtering,
	// 21610 opted out, 30003 unreachable) — safe to log (no PII), present only on
	// failure.
	if errorCode := r.PostForm.Get("ErrorCode"); errorCode != "" {
		logger = logger.With("twilio_code", errorCode)
	}

	switch messageStatus {
	case "failed", "undelivered":
		// Terminal failure — the deliverability signal this endpoint exists for.
		// ERROR trips the "Server Error Logged" alert and feeds the log-based
		// delivery metric.
		logger.ErrorContext(ctx, "sms delivery failed")
	case "delivered":
		logger.InfoContext(ctx, "sms delivered")
	default:
		// Intermediate lifecycle (queued/sending/sent/accepted/read/…) — high
		// volume, low signal; kept at DEBUG for traceability without noise.
		logger.DebugContext(ctx, "sms delivery status update")
	}

	// Status callbacks expect a bare 2xx; Twilio ignores the response body and
	// retries on non-2xx.
	w.WriteHeader(http.StatusNoContent)
}
