// Outbound email delivery webhook — observability-only.
//
// The synchronous send result only tells us Mailgun *accepted* a message, which
// is not the same as it arriving. Mailgun reports the rest of the lifecycle
// (delivered / failed / complained) asynchronously to this endpoint. It is the
// email counterpart of the Twilio status callback in sms_status.go, and it
// exists for the same reason: deliverability problems — greylisting, bounces,
// spam complaints, a misconfigured domain — are invisible to the send call.
//
// It matters more for email than it did for SMS. Since #2864 a mailed one-time
// code is the only way into an email account, so a message that never arrives
// is a lockout rather than a missed notification.
//
// True accepted→delivered latency is computed here without any correlation
// table: the send stamps its own timestamp into a Mailgun custom variable
// (server/email.stampDeliveryVariables), and Mailgun echoes it back under
// `event-data.user-variables`. The message carries its own start time.
//
// Records no state and returns 204. The only PII in the payload is the
// recipient address, masked in every log line.

package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"go.ripls.org/ripls/server/logging"
)

// emailStatusPath is the fixed public path Mailgun's webhooks are configured to
// POST to. Unlike the Twilio callbacks the signature does not cover the URL, so
// this constant is only the route — but keeping the shape identical to
// smsStatusPath keeps the two webhooks recognisably the same thing.
const emailStatusPath = "/email/status"

// mailgunMaxClockSkew bounds how far in the past a signed request may be.
// Mailgun's signature covers a timestamp precisely so a captured payload cannot
// be replayed forever; without this check the signature alone would make any
// intercepted webhook valid indefinitely.
const mailgunMaxClockSkew = 15 * time.Minute

// mailgunWebhook is the subset of Mailgun's webhook payload this handler reads.
// Fields it does not use are ignored rather than rejected, so a Mailgun payload
// addition cannot break delivery reporting.
//
// What is verified against live data, and what is not — worth stating, because
// the unit tests sign payloads the same way this file verifies them and would
// pass just as happily if the shape below were wrong:
//
//   - `event`, `timestamp`, `recipient`, `delivery-status.{code,message}`, and
//     `user-variables` are confirmed against real `delivered` events pulled
//     from Mailgun's Events API for this domain, which returns the same object
//     the webhook posts. TestMailgunWebhook_DecodesRealPayload pins them.
//   - `severity` and `reason` come from Mailgun's documentation for `failed`
//     events. The account had no failures inside the events retention window to
//     check against. If those names are wrong the handler still functions — the
//     fields log empty and the outcome is still recorded — so this degrades
//     rather than breaks.
//   - The `signature` / `event-data` envelope and the HMAC scheme are
//     documented behavior, unverifiable without a real signed delivery. The
//     first real webhook confirms or refutes both at once: a mismatch shows up
//     as "invalid signature; rejecting" in the logs, not as silence.
type mailgunWebhook struct {
	Signature struct {
		TimestampSec string `json:"timestamp"`
		Token        string `json:"token"`
		Signature    string `json:"signature"`
	} `json:"signature"`
	EventData struct {
		Event     string  `json:"event"`
		Timestamp float64 `json:"timestamp"`
		Recipient string  `json:"recipient"`
		Severity  string  `json:"severity"`
		Reason    string  `json:"reason"`
		// UserVariables carries what the send stamped. Values arrive as JSON
		// strings; typed as any so a non-string never fails the whole decode.
		UserVariables  map[string]any `json:"user-variables"`
		DeliveryStatus struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"delivery-status"`
	} `json:"event-data"`
}

// verifyMailgunSignature reports whether the payload was signed with the
// configured webhook signing key. Mailgun signs HMAC-SHA256 over
// (timestamp + token) — note this is the *webhook signing key*, a different
// secret from the API key used to send.
//
// Returns false when no key is configured: an unauthenticated endpoint that
// writes to the observability stream is a way to fabricate delivery data, so
// the safe default is to reject rather than to accept everything.
func verifyMailgunSignature(signingKey, timestamp, token, signature string) bool {
	if signingKey == "" || timestamp == "" || token == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write([]byte(timestamp + token))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// userVariable reads a stamped variable as a string, tolerating the numeric
// decoding a JSON value may land on.
func userVariable(vars map[string]any, key string) string {
	switch v := vars[key].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	default:
		return ""
	}
}

// HandleEmailStatusWebhook records the delivery outcome of an outbound email.
// Wired to POST /email/status. Observability-only: it verifies the Mailgun
// signature, logs the event at a level chosen by the outcome, and acknowledges
// with 204. Always returns 2xx once authenticated so Mailgun does not retry.
func (s *Service) HandleEmailStatusWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.LoggerWithContext(ctx).With("operation", "EmailStatusWebhook")

	var payload mailgunWebhook
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		logger.InfoContext(ctx, "email status webhook: malformed payload", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	sig := payload.Signature
	if !verifyMailgunSignature(s.emailWebhookSigningKey, sig.TimestampSec, sig.Token, sig.Signature) {
		// Covers both a bad signature and no key configured. Warn either way:
		// silently dropping delivery events would look identical to healthy
		// delivery, which is the failure mode this endpoint exists to prevent.
		logger.WarnContext(ctx, "email status webhook: invalid signature; rejecting",
			"signing_key_configured", s.emailWebhookSigningKey != "",
		)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Reject stale payloads. The signature never expires on its own, so without
	// this a captured webhook could be replayed indefinitely to forge delivery
	// events.
	if secs, err := strconv.ParseInt(sig.TimestampSec, 10, 64); err == nil {
		if age := time.Since(time.Unix(secs, 0)); age > mailgunMaxClockSkew || age < -mailgunMaxClockSkew {
			logger.WarnContext(ctx, "email status webhook: timestamp outside accepted window; rejecting",
				"age_sec", int64(age.Seconds()),
			)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}

	event := payload.EventData

	// Drop what another environment sent. Mailgun webhooks are configured per
	// *domain* and dev and prod share one, so every registered URL receives
	// every environment's events. Without this filter a dev simulation run —
	// thousands of addresses — would land in production's delivery metrics and
	// swamp the signal this endpoint exists to produce.
	//
	// An unstamped message (sent before the stamp existed, or by a build
	// without it) is skipped rather than counted: attributing it to whichever
	// environment happened to receive the webhook would be a guess.
	if env := userVariable(event.UserVariables, "ripls_env"); env != s.hostname {
		logging.LoggerWithContext(ctx).DebugContext(ctx, "ignoring email delivery event from another environment",
			"operation", "EmailStatusWebhook",
			"event_env", env,
			"this_env", s.hostname,
		)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	emailType := userVariable(event.UserVariables, "ripls_email_type")
	if emailType == "" {
		emailType = "unknown"
	}

	logger = logger.With(
		"email_event", event.Event,
		"email_type", emailType,
		"recipient_email", logging.MaskEmail(event.Recipient),
	)

	// Accepted→delivered latency, from the timestamp the send stamped onto the
	// message. Only meaningful for a terminal event, and only when both ends
	// are present — a message sent before this stamping existed has no start.
	if sentAtMs, err := strconv.ParseInt(userVariable(event.UserVariables, "ripls_sent_at_ms"), 10, 64); err == nil && event.Timestamp > 0 {
		latencyMs := int64(event.Timestamp*1000) - sentAtMs
		// Guard against clock disagreement between Mailgun and this server
		// producing a negative or absurd figure that would skew the metric.
		if latencyMs >= 0 && latencyMs < mailgunMaxClockSkew.Milliseconds() {
			logger = logger.With("delivery_latency_ms", latencyMs)
		}
	}

	switch event.Event {
	case "delivered":
		// The signal the endpoint exists for: the message actually arrived, and
		// how long it took.
		logger.InfoContext(ctx, "email delivered")
	case "failed":
		// severity=permanent is a hard bounce (bad address, blocked); temporary
		// is a retryable deferral Mailgun will attempt again.
		// NOT "severity": the logging package renames the log level to that key
		// for Cloud Logging (logging/logger.go), so an attribute of the same
		// name collides with it — the two values concatenate into nonsense like
		// "ERRORpermanent", corrupting both the provider's classification and
		// the entry's own level.
		logger.ErrorContext(ctx, "email delivery failed",
			"bounce_severity", event.Severity,
			"reason", event.Reason,
			"smtp_code", event.DeliveryStatus.Code,
			"smtp_message", event.DeliveryStatus.Message,
		)
	case "complained":
		// A spam complaint. Rare and worth knowing: enough of them damage the
		// sending domain's reputation, which degrades delivery for everyone.
		logger.ErrorContext(ctx, "email marked as spam by recipient")
	default:
		// accepted/opened/clicked/unsubscribed — high volume, low signal here.
		logger.DebugContext(ctx, "email delivery event")
	}

	w.WriteHeader(http.StatusNoContent)
}
