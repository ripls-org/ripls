//go:build integration

// twilio_integration_test.go exercises TwilioSMSSender against the REAL Twilio
// Messages API using TEST credentials + magic numbers — no real text message is
// sent and no charge is incurred. It validates our request shaping, basic auth,
// and error-code mapping against live Twilio, which the httptest-faked unit
// tests cannot. Skipped unless TWILIO_TEST_ACCOUNT_SID and TWILIO_TEST_AUTH_TOKEN
// are set and the `integration` build tag is enabled (`go test -tags=integration`).
//
// Use the TEST credentials from Twilio Console → Account → Keys & tokens (a
// distinct Test Account SID + Test Auth Token), NOT the live ones. Test mode
// rejects MessagingServiceSid, so the sender is configured with a magic From
// number; production routes through the Messaging Service SID instead.
package sms

import (
	"context"
	"os"
	"strings"
	"testing"
)

// magicFrom passes all of Twilio's test-mode validation as a sender.
const magicFrom = "+15005550006"

func newTestCredSender(t *testing.T) *TwilioSMSSender {
	t.Helper()
	sid := os.Getenv("TWILIO_TEST_ACCOUNT_SID")
	token := os.Getenv("TWILIO_TEST_AUTH_TOKEN")
	if sid == "" || token == "" {
		t.Skip("set TWILIO_TEST_ACCOUNT_SID and TWILIO_TEST_AUTH_TOKEN to run the Twilio integration test")
	}
	// No Messaging Service SID: test credentials reject it. The magic From
	// number drives the success path.
	s := NewTwilioSMSSender(sid, token, "")
	s.from = magicFrom
	return s
}

// TestTwilioIntegration_ValidNumberSucceeds proves a well-formed, authenticated
// request to a valid magic number returns 201/queued with no error.
func TestTwilioIntegration_ValidNumberSucceeds(t *testing.T) {
	s := newTestCredSender(t)
	if err := s.SendSMS(context.Background(), "+15005550006", "ripls integration test"); err != nil {
		t.Fatalf("expected success sending to the valid magic number, got: %v", err)
	}
}

// TestTwilioIntegration_InvalidNumberMapsError proves our error path surfaces
// the Twilio error code (21211 = invalid 'To') for a rejected send.
func TestTwilioIntegration_InvalidNumberMapsError(t *testing.T) {
	s := newTestCredSender(t)
	err := s.SendSMS(context.Background(), "+15005550001", "x")
	if err == nil {
		t.Fatal("expected an error for the invalid magic number")
	}
	if !strings.Contains(err.Error(), "21211") {
		t.Errorf("expected Twilio code 21211 (invalid number) in error, got: %v", err)
	}
}

// TestTwilioIntegration_CannotRouteMapsError covers a second distinct Twilio
// error code (21612 = can't route) so the mapping isn't a one-off.
func TestTwilioIntegration_CannotRouteMapsError(t *testing.T) {
	s := newTestCredSender(t)
	err := s.SendSMS(context.Background(), "+15005550002", "x")
	if err == nil {
		t.Fatal("expected an error for the unroutable magic number")
	}
	if !strings.Contains(err.Error(), "21612") {
		t.Errorf("expected Twilio code 21612 (cannot route) in error, got: %v", err)
	}
}
