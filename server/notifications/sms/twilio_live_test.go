//go:build integration

// twilio_live_test.go sends a REAL text message through the live Twilio account
// and Messaging Service, exercising the production MessagingServiceSid path that
// the test-credential integration test cannot (test mode rejects
// MessagingServiceSid and forces a magic From). It SENDS A REAL SMS and costs a
// fraction of a cent, so it self-skips unless TWILIO_LIVE_TO is set — and it is
// deliberately NOT wired into CI's default env (unlike the no-send
// test-credential test). It's a manual, on-demand smoke test.
//
// Run it with `npm run test:server:integration:sms:live` (which fetches the live
// creds from Secret Manager), supplying your own number:
//
//	TWILIO_LIVE_TO=+15551234567 npm run test:server:integration:sms:live
//
// Send only to a number you own / have consent to text — a real A2P message
// goes out under the registered campaign.
package sms

import (
	"context"
	"os"
	"testing"
)

func TestTwilioLiveSend(t *testing.T) {
	to := os.Getenv("TWILIO_LIVE_TO")
	sid := os.Getenv("TWILIO_LIVE_ACCOUNT_SID")
	tok := os.Getenv("TWILIO_LIVE_AUTH_TOKEN")
	mg := os.Getenv("TWILIO_LIVE_MESSAGING_SERVICE_SID")
	if to == "" || sid == "" || tok == "" || mg == "" {
		t.Skip("set TWILIO_LIVE_TO (+ live creds) to send a real test SMS; npm run test:server:integration:sms:live supplies the creds")
	}

	// Production path: route through the Messaging Service (sender pool + A2P
	// campaign), no From — exactly how the server sends.
	s := NewTwilioSMSSender(sid, tok, mg)
	if err := s.SendSMS(context.Background(), to, "Ripls: live SMS test via the Messaging Service path. Reply STOP to opt out, HELP for help."); err != nil {
		t.Fatalf("live send failed: %v", err)
	}
	t.Logf("live SMS accepted for delivery via Messaging Service %s", mg)
}
