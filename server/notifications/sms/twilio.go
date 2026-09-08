package sms

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // G505: Twilio's webhook signature is HMAC-SHA1; see validateSignature.
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"go.ripls.org/ripls/server/logging"
)

// defaultBaseURL is the Twilio REST API root. Overridable on the sender for
// tests (httptest) and for Twilio's test-credential host if ever needed.
const defaultBaseURL = "https://api.twilio.com"

// sendTimeout bounds a single send so a hung provider can never block the
// caller's NotifyUser path indefinitely.
const sendTimeout = 30 * time.Second

// TwilioSMSSender sends text messages via Twilio's Messages REST API using a
// Messaging Service (the SID routes to the registered sender pool + campaign,
// rather than a bare number). It is safe for concurrent use.
type TwilioSMSSender struct {
	accountSID          string
	authToken           string
	messagingServiceSID string
	httpClient          *http.Client
	baseURL             string
	// from, when non-empty, sends with a bare From number instead of the
	// Messaging Service SID. Production always routes through the Messaging
	// Service (for A2P campaign attribution) and leaves this empty; it exists
	// because Twilio TEST credentials reject MessagingServiceSid and require a
	// magic From number (see twilio_integration_test.go).
	from string
}

// NewTwilioSMSSender builds a sender from the three credentials resolved at
// startup. All three must be non-empty; callers gate construction on presence
// (empty creds ⇒ SMS disabled), so this does not itself validate them.
func NewTwilioSMSSender(accountSID, authToken, messagingServiceSID string) *TwilioSMSSender {
	return &TwilioSMSSender{
		accountSID:          accountSID,
		authToken:           authToken,
		messagingServiceSID: messagingServiceSID,
		httpClient:          &http.Client{},
		baseURL:             defaultBaseURL,
	}
}

// twilioMessageResponse models the subset of the Messages resource we read.
// On success Twilio returns 201 with sid/status; on failure a 4xx with
// code/message (the Twilio error code, e.g. 21610 = recipient opted out).
type twilioMessageResponse struct {
	SID       string `json:"sid"`
	Status    string `json:"status"`
	ErrorCode *int   `json:"error_code"`
	Code      int    `json:"code"`
	Message   string `json:"message"`
}

// SendSMS delivers body to the E.164 number toE164 via the Messaging Service.
// It never logs the recipient number unmasked or the message body (both are
// PII / user content). The returned error wraps the HTTP status and Twilio
// error code but not the recipient or body.
func (s *TwilioSMSSender) SendSMS(ctx context.Context, toE164, body string) error {
	logger := logging.LoggerWithContext(ctx).With(
		"external_service", "twilio",
		"operation", "SendSMS",
		"recipient_phone", logging.MaskPhone(toE164),
	)
	startTime := time.Now()

	form := url.Values{}
	if s.from != "" {
		form.Set("From", s.from)
	} else {
		form.Set("MessagingServiceSid", s.messagingServiceSID)
	}
	form.Set("To", toE164)
	form.Set("Body", body)

	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", s.baseURL, s.accountSID)

	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build sms request: %w", err)
	}
	req.SetBasicAuth(s.accountSID, s.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		logger.ErrorContext(ctx, "sms send transport error", "error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return fmt.Errorf("send sms: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var parsed twilioMessageResponse
	_ = json.Unmarshal(respBody, &parsed)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// parsed.Code is the Twilio error code; safe to log (no PII).
		logger.ErrorContext(ctx, "sms send rejected by provider",
			"http_status", resp.StatusCode,
			"twilio_code", parsed.Code,
			"twilio_message", parsed.Message,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return fmt.Errorf("sms send failed: http %d, twilio code %d", resp.StatusCode, parsed.Code)
	}

	logger.InfoContext(ctx, "sms sent",
		"message_sid", parsed.SID,
		"message_status", parsed.Status,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)
	return nil
}

// ValidateSignature reports whether header is a valid X-Twilio-Signature for an
// inbound webhook POST to publicURL carrying the given form params, signed with
// authToken. The algorithm (per Twilio's spec): append each POST param, sorted
// by key, as key+value to the full request URL, HMAC-SHA1 with the auth token,
// base64-encode, and constant-time compare to the header.
//
// authToken empty ⇒ validation is disabled and this returns true; the caller
// decides whether an unsigned webhook is acceptable (it is not in production).
func ValidateSignature(authToken, publicURL string, params url.Values, header string) bool {
	if authToken == "" {
		return true
	}

	var sb strings.Builder
	sb.WriteString(publicURL)
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString(k)
		// Twilio concatenates only the first value of each param.
		sb.WriteString(params.Get(k))
	}

	mac := hmac.New(sha1.New, []byte(authToken))
	mac.Write([]byte(sb.String()))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(header))
}
