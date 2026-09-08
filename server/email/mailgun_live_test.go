//go:build integration

// mailgun_live_test.go sends REAL email through Mailgun so the templates can be
// checked in a real client. Email-client rendering is the one thing this repo
// cannot assert on: Apple Mail, Gmail and Outlook each drop a different subset
// of the CSS, and the defect that motivated this file (#2927 — a logo Apple
// Mail left-aligned while Gmail centered it) was invisible to every automated
// check. Seeing the message in the client is the test.
//
// It SENDS REAL MAIL, so it self-skips unless MAILGUN_LIVE_TO is set and is
// deliberately not wired into CI. Run it on demand with your own address:
//
//	MAILGUN_LIVE_TO=you@example.com npm run test:server:integration:email:live
//
// Send only to an address you own. Dev and prod share the one real Mailgun
// domain (example.com) and therefore share the sending quota — keep runs small.
package email

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMailgunLiveSend(t *testing.T) {
	to := os.Getenv("MAILGUN_LIVE_TO")
	apiKey := os.Getenv("MAILGUN_LIVE_API_KEY")
	if to == "" || apiKey == "" {
		t.Skip("set MAILGUN_LIVE_TO to send real test email; npm run test:server:integration:email:live supplies the API key")
	}

	domain := os.Getenv("MAILGUN_LIVE_DOMAIN")
	if domain == "" {
		domain = "example.com"
	}
	from := os.Getenv("MAILGUN_LIVE_FROM")
	if from == "" {
		from = "Ripls <noreply@example.com>"
	}

	svc, err := NewMailgunService(domain, apiKey, from)
	if err != nil {
		t.Fatalf("NewMailgunService: %v", err)
	}
	// Tag the delivery variables so the status webhook drops these events
	// instead of folding a manual render check into dev or prod delivery
	// metrics.
	svc.SetEnvironmentTag("local")

	ctx := context.Background()

	// The sign-in code is the email #2927 was reported against, and the one
	// with the most at stake: it is all that stands between a new user and
	// their first session.
	if err := svc.SendEmailCode(ctx, to, "277122", 10*time.Minute, "en"); err != nil {
		t.Fatalf("send email code: %v", err)
	}
	t.Logf("sign-in code email accepted for %s", to)

	// A second template proves the header change is the shared idiom rather
	// than a one-off edit to the reported email.
	if err := svc.SendWaitlistWelcome(ctx, to); err != nil {
		t.Fatalf("send waitlist welcome: %v", err)
	}
	t.Logf("waitlist welcome email accepted for %s", to)
}
