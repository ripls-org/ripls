package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testSigningKey = "test-mailgun-webhook-signing-key"

// signedPayload builds a webhook body signed the way Mailgun signs one.
func signedPayload(t *testing.T, key, eventJSON string, ts time.Time) string {
	t.Helper()
	timestamp := strconv.FormatInt(ts.Unix(), 10)
	const token = "0123456789abcdef0123456789abcdef01234567"
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(timestamp + token))
	sig := hex.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf(`{"signature":{"timestamp":%q,"token":%q,"signature":%q},"event-data":%s}`,
		timestamp, token, sig, eventJSON)
}

func postWebhook(t *testing.T, svc *Service, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, emailStatusPath, strings.NewReader(body))
	rec := httptest.NewRecorder()
	svc.HandleEmailStatusWebhook(rec, req)
	return rec
}

func serviceWithKey(key string) *Service {
	return &Service{emailWebhookSigningKey: key, hostname: testHostname}
}

// realDeliveredEvent is a `delivered` event captured verbatim from Mailgun's
// Events API for example.com (2026-08-07), trimmed only of storage keys and
// message-id. The webhook posts this same object under `event-data`.
//
// It is here because the other tests in this file construct payloads by hand,
// which makes them self-consistent rather than correct: they would pass just as
// happily if every field name in mailgunWebhook were wrong. This one pins the
// struct against bytes the provider actually produced, so a field rename or a
// misremembered shape fails here instead of silently logging zeroes in
// production.
const realDeliveredEvent = `{
  "user-variables": {"ripls_email_type": "email_code", "ripls_sent_at_ms": "1786103450000", "ripls_env": "test.example.com"},
  "flags": {"is-system-test": false, "is-test-mode": false, "is-authenticated": true, "is-routed": false},
  "id": "4qTwHa4JRyG2IHWValKh_w",
  "recipient-provider": "Google Workspace",
  "timestamp": 1786103455.5221853,
  "tags": [],
  "message": {"size": 10472, "attachments": [], "headers": {"to": "ops@example.com", "from": "Ripls <noreply@example.com>", "subject": "Your Ripls sign-in code"}},
  "campaigns": [],
  "log-level": "info",
  "recipient-domain": "example.com",
  "event": "delivered",
  "recipient": "ops@example.com",
  "envelope": {"sender": "noreply@example.com", "transport": "smtp", "targets": "ops@example.com"},
  "delivery-status": {
    "certificate-verified": true, "first-delivery-attempt-seconds": 0.122,
    "enhanced-code": "2.0.0", "mx-host": "smtp.google.com", "session-seconds": 0.563,
    "code": 250, "description": "", "message": "2.0.0 OK 1786103455 - gsmtp",
    "tls": true, "attempt-no": 1
  }
}`

// Decoding a real event must populate every field the handler reads. Fields
// Mailgun sends that we ignore are fine; fields we read that never arrive are
// the failure this catches.
func TestMailgunWebhook_DecodesRealPayload(t *testing.T) {
	body := signedPayload(t, testSigningKey, realDeliveredEvent, time.Now())

	var payload mailgunWebhook
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("failed to decode a real Mailgun payload: %v", err)
	}

	ev := payload.EventData
	if ev.Event != "delivered" {
		t.Errorf("event = %q, want %q", ev.Event, "delivered")
	}
	if ev.Recipient != "ops@example.com" {
		t.Errorf("recipient = %q, want %q", ev.Recipient, "ops@example.com")
	}
	if ev.Timestamp == 0 {
		t.Error("timestamp did not decode — delivery latency would be uncomputable")
	}
	if ev.DeliveryStatus.Code != 250 {
		t.Errorf("delivery-status.code = %d, want 250", ev.DeliveryStatus.Code)
	}
	// The whole latency mechanism rests on these round-tripping.
	if got := userVariable(ev.UserVariables, "ripls_email_type"); got != "email_code" {
		t.Errorf("stamped email type = %q, want %q", got, "email_code")
	}
	if got := userVariable(ev.UserVariables, "ripls_sent_at_ms"); got != "1786103450000" {
		t.Errorf("stamped send time = %q, want %q", got, "1786103450000")
	}
}

func TestEmailStatusWebhook_AcceptsSignedDelivery(t *testing.T) {
	svc := serviceWithKey(testSigningKey)
	sentAt := time.Now().Add(-3 * time.Second)
	event := fmt.Sprintf(`{
		"event":"delivered",
		"timestamp":%d,
		"recipient":"someone@example.com",
		"user-variables":{"ripls_email_type":"email_code","ripls_env":"test.example.com","ripls_sent_at_ms":"%d"}
	}`, time.Now().Unix(), sentAt.UnixMilli())

	rec := postWebhook(t, svc, signedPayload(t, testSigningKey, event, time.Now()))
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

// An unsigned or wrongly-signed payload must be refused: the endpoint feeds the
// signal that tells us whether sign-in codes arrive, so anyone able to POST to
// it could otherwise manufacture healthy-looking delivery.
func TestEmailStatusWebhook_RejectsBadSignature(t *testing.T) {
	svc := serviceWithKey(testSigningKey)
	event := `{"event":"delivered","recipient":"someone@example.com"}`

	rec := postWebhook(t, svc, signedPayload(t, "the-wrong-key", event, time.Now()))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d for a payload signed with the wrong key", rec.Code, http.StatusForbidden)
	}
}

// No configured key means reject, not accept-everything. This is the opposite
// of the SMS callbacks' fail-open default, and deliberately so.
func TestEmailStatusWebhook_RejectsWhenNoKeyConfigured(t *testing.T) {
	svc := serviceWithKey("")
	event := `{"event":"delivered","recipient":"someone@example.com"}`

	rec := postWebhook(t, svc, signedPayload(t, testSigningKey, event, time.Now()))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d when no signing key is configured", rec.Code, http.StatusForbidden)
	}
}

// A valid signature never expires by itself, so a captured payload would
// otherwise be replayable forever.
func TestEmailStatusWebhook_RejectsStaleTimestamp(t *testing.T) {
	svc := serviceWithKey(testSigningKey)
	stale := time.Now().Add(-2 * mailgunMaxClockSkew)
	event := `{"event":"delivered","recipient":"someone@example.com"}`

	rec := postWebhook(t, svc, signedPayload(t, testSigningKey, event, stale))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d for a replayed payload", rec.Code, http.StatusForbidden)
	}
}

func TestEmailStatusWebhook_RejectsMalformedBody(t *testing.T) {
	svc := serviceWithKey(testSigningKey)

	rec := postWebhook(t, svc, "not json")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// A failure event must still be acknowledged: returning non-2xx makes Mailgun
// retry a webhook we have already recorded.
func TestEmailStatusWebhook_AcknowledgesFailureEvents(t *testing.T) {
	svc := serviceWithKey(testSigningKey)
	event := `{
		"event":"failed",
		"severity":"permanent",
		"reason":"suppress-bounce",
		"recipient":"someone@example.com",
		"delivery-status":{"code":550,"message":"mailbox unavailable"},
		"user-variables":{"ripls_email_type":"email_code","ripls_env":"test.example.com"}
	}`

	rec := postWebhook(t, svc, signedPayload(t, testSigningKey, event, time.Now()))
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestUserVariable(t *testing.T) {
	vars := map[string]any{
		"str": "email_code",
		// A JSON number decodes to float64; the send stamps strings, but a
		// hand-edited Mailgun variable could arrive numeric.
		"num": float64(1786045337062),
	}

	if got := userVariable(vars, "str"); got != "email_code" {
		t.Errorf("string variable = %q, want %q", got, "email_code")
	}
	if got := userVariable(vars, "num"); got != "1786045337062" {
		t.Errorf("numeric variable = %q, want %q", got, "1786045337062")
	}
	if got := userVariable(vars, "absent"); got != "" {
		t.Errorf("absent variable = %q, want empty", got)
	}
}

// Dev and prod share one Mailgun domain, so both receive every environment's
// events. Counting the other environment's mail would put a dev simulation run
// — thousands of addresses — into production's delivery metrics.
func TestEmailStatusWebhook_IgnoresOtherEnvironments(t *testing.T) {
	svc := serviceWithKey(testSigningKey)
	event := `{
		"event":"delivered",
		"recipient":"someone@example.com",
		"user-variables":{"ripls_email_type":"email_code","ripls_env":"dev.example.com"}
	}`

	// Still acknowledged — Mailgun must not retry an event we deliberately skip.
	rec := postWebhook(t, svc, signedPayload(t, testSigningKey, event, time.Now()))
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

// The logging package renames the log level to "severity" for Cloud Logging
// (logging/logger.go). An attribute of the same name collides with it and the
// two concatenate — a real bounce produced "ERRORpermanent", which corrupted
// both the provider's classification AND the entry's own level, so severity
// filtering and the delivery-failure metric would both have been wrong.
//
// This pins the attribute name rather than the behaviour, because the collision
// is invisible in the handler: it only appears once the entry is serialized.
func TestEmailStatusWebhook_DoesNotShadowLogSeverity(t *testing.T) {
	src, err := os.ReadFile("email_status.go")
	if err != nil {
		t.Fatalf("read email_status.go: %v", err)
	}
	if strings.Contains(string(src), `"severity",`) {
		t.Error(`logs a "severity" attribute; it collides with the reserved Cloud Logging severity key — use bounce_severity`)
	}
	if !strings.Contains(string(src), `"bounce_severity"`) {
		t.Error("the bounce severity is no longer logged; the delivery-failure metric labels on it")
	}
}
