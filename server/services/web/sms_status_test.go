package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/logging"
)

// The status handler records no DB state — its only observable output is the log
// line — so these tests drive it with a captured JSON logger injected via
// logging.WithLogger and assert the severity + fields of the emitted record.
// That proves the observability contract (right level, masked PII, Twilio code),
// not just the 204/403 signature contract.

// postStatus fires the status callback with the given form. When signToken is
// non-empty, the service is given that auth token and the request is signed with
// it (verification enabled); when empty, verification is disabled (dev mode). It
// returns the recorder plus the captured log buffer.
func postStatus(t *testing.T, form url.Values, signToken string) (*httptest.ResponseRecorder, *bytes.Buffer) {
	t.Helper()
	svc, _ := newWebServiceForTest(t)

	req := httptest.NewRequest(http.MethodPost, "/sms/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if signToken != "" {
		svc.SetSMSAuthToken(signToken)
		// The handler verifies against "https://<hostname>/sms/status"; hostname is
		// "test.example.com" in newWebServiceForTest.
		req.Header.Set("X-Twilio-Signature",
			twilioSignature(signToken, "https://test.example.com/sms/status", form))
	}

	buf := &bytes.Buffer{}
	lg := logging.NewLogger(logging.Options{Level: "debug", Format: "json", Output: buf})
	req = req.WithContext(logging.WithLogger(req.Context(), lg))

	rec := httptest.NewRecorder()
	svc.HandleSMSStatusCallback(rec, req)
	return rec, buf
}

// findLogLine returns the parsed JSON log record whose "message" equals msg, or
// fails the test if none is present. The JSON handler renames level→severity and
// msg→message (cloudLoggingReplaceAttr).
func findLogLine(t *testing.T, buf *bytes.Buffer, msg string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %q (%v)", line, err)
		}
		if rec["message"] == msg {
			return rec
		}
	}
	t.Fatalf("no log line with message %q in:\n%s", msg, buf.String())
	return nil
}

func TestSMSStatus_DeliveredLogsInfo(t *testing.T) {
	rec, buf := postStatus(t, url.Values{
		"MessageSid":    {"SM0123456789"},
		"MessageStatus": {"delivered"},
		"To":            {"+15551234567"},
	}, "a-real-token")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	line := findLogLine(t, buf, "sms delivered")
	if line["severity"] != "INFO" {
		t.Errorf("delivered should log at INFO, got severity=%v", line["severity"])
	}
	if line["message_status"] != "delivered" {
		t.Errorf("message_status = %v, want delivered", line["message_status"])
	}
	if line["message_sid"] != "SM0123456789" {
		t.Errorf("message_sid = %v, want SM0123456789", line["message_sid"])
	}
	if line["recipient_phone"] != "+*******4567" {
		t.Errorf("recipient_phone = %v, want masked +*******4567", line["recipient_phone"])
	}
	// The raw number must never appear anywhere in the logs.
	if strings.Contains(buf.String(), "+15551234567") {
		t.Errorf("raw recipient number leaked into logs:\n%s", buf.String())
	}
}

func TestSMSStatus_FailedLogsErrorWithCode(t *testing.T) {
	rec, buf := postStatus(t, url.Values{
		"MessageSid":    {"SM0123456789"},
		"MessageStatus": {"failed"},
		"ErrorCode":     {"30007"}, // carrier filtering
		"To":            {"+15551234567"},
	}, "a-real-token")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	line := findLogLine(t, buf, "sms delivery failed")
	if line["severity"] != "ERROR" {
		t.Errorf("failed should log at ERROR (trips the alert), got severity=%v", line["severity"])
	}
	if line["twilio_code"] != "30007" {
		t.Errorf("twilio_code = %v, want 30007", line["twilio_code"])
	}
	if line["recipient_phone"] != "+*******4567" {
		t.Errorf("recipient_phone = %v, want masked", line["recipient_phone"])
	}
}

func TestSMSStatus_UndeliveredLogsError(t *testing.T) {
	rec, buf := postStatus(t, url.Values{
		"MessageSid":    {"SM0123456789"},
		"MessageStatus": {"undelivered"},
		"ErrorCode":     {"21610"}, // recipient opted out
		"To":            {"+15551234567"},
	}, "a-real-token")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	line := findLogLine(t, buf, "sms delivery failed")
	if line["severity"] != "ERROR" {
		t.Errorf("undelivered should log at ERROR, got severity=%v", line["severity"])
	}
	if line["twilio_code"] != "21610" {
		t.Errorf("twilio_code = %v, want 21610", line["twilio_code"])
	}
}

func TestSMSStatus_IntermediateIsDebugAndAcceptsUnsigned(t *testing.T) {
	// Empty token ⇒ signature verification disabled (dev mode): the callback is
	// accepted. An intermediate lifecycle status logs at DEBUG.
	rec, buf := postStatus(t, url.Values{
		"MessageSid":    {"SM0123456789"},
		"MessageStatus": {"sent"},
		"To":            {"+15551234567"},
	}, "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	line := findLogLine(t, buf, "sms delivery status update")
	if line["severity"] != "DEBUG" {
		t.Errorf("intermediate status should log at DEBUG, got severity=%v", line["severity"])
	}
	if line["message_status"] != "sent" {
		t.Errorf("message_status = %v, want sent", line["message_status"])
	}
}

func TestSMSStatus_ForgedSignatureRejected(t *testing.T) {
	svc, _ := newWebServiceForTest(t)
	svc.SetSMSAuthToken("a-real-token") // enables verification

	req := httptest.NewRequest(http.MethodPost, "/sms/status",
		strings.NewReader(url.Values{
			"MessageSid":    {"SM0123456789"},
			"MessageStatus": {"failed"},
			"To":            {"+15551234567"},
		}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", "obviously-wrong")

	buf := &bytes.Buffer{}
	lg := logging.NewLogger(logging.Options{Level: "debug", Format: "json", Output: buf})
	req = req.WithContext(logging.WithLogger(req.Context(), lg))

	rec := httptest.NewRecorder()
	svc.HandleSMSStatusCallback(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	// A forged callback must be rejected before any delivery log is emitted.
	if strings.Contains(buf.String(), "sms delivery failed") {
		t.Errorf("forged callback must not produce a delivery log line:\n%s", buf.String())
	}
}

func TestSMSStatus_MalformedFormIsBadRequest(t *testing.T) {
	svc, _ := newWebServiceForTest(t)
	// An invalid percent-escape in the body makes ParseForm fail.
	req := httptest.NewRequest(http.MethodPost, "/sms/status", strings.NewReader("%zz"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	svc.HandleSMSStatusCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a malformed form", rec.Code)
	}
}
