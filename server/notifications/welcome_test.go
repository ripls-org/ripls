package notifications

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications/sms"
	"go.ripls.org/ripls/server/smsoptout"
	"go.ripls.org/ripls/server/storage"
)

// expectedWelcomeEN is the exact en.toml sms.welcome.body string, rendered.
// Pinned so a catalog edit that drops a required CTIA element (program name,
// frequency, rates, HELP, STOP) fails this test.
//
// The URL is the sending deployment's own (SMSChannel.AppBaseURL, #2953), so
// this spells out the fixture host setupSMSService wires — which is also what
// proves the param reaches the rendered body rather than a literal surviving
// in the catalog.
const expectedWelcomeEN = "Welcome to Ripls! Let's lean on each other. You'll get event invites and community updates. Message & data rates may apply. Message frequency varies. Reply HELP for help, STOP to opt out.\n\nConnect at https://test.example.com"

// Everything before the "Connect at" sentence: the CTIA elements that make the
// message compliant. A deployment with no app URL still has to send all of it.
const welcomeDisclosureEN = "Welcome to Ripls! Let's lean on each other. You'll get event invites and community updates. Message & data rates may apply. Message frequency varies. Reply HELP for help, STOP to opt out."

func TestSendPhoneOptInWelcome_SendsToOptedInNumber(t *testing.T) {
	svc, sender, _ := setupSMSService(t)
	ctx := context.Background()

	u := phoneUser("+15551234567", "")
	if err := svc.SendPhoneOptInWelcome(ctx, u); err != nil {
		t.Fatalf("SendPhoneOptInWelcome: %v", err)
	}

	msgs := sender.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 SMS sent, got %d", len(msgs))
	}
	if msgs[0].To != "+15551234567" {
		t.Errorf("To = %q", msgs[0].To)
	}
	// The welcome is self-contained and must NOT carry the appended dispatcher
	// footer ("Reply STOP to opt out, HELP for help.") on top of its own copy.
	if msgs[0].Body != expectedWelcomeEN {
		t.Errorf("body = %q, want %q", msgs[0].Body, expectedWelcomeEN)
	}
	// The welcome carries the opt-out disclosure, so the timestamp is recorded
	// (lets the next platform SMS omit the footer until the ~monthly cadence).
	if u.GetSmsOptoutDisclosedAtUnixSec() == 0 {
		t.Error("expected opt-out disclosure timestamp to be recorded")
	}
}

func TestSendPhoneOptInWelcome_SkipsOptedOut(t *testing.T) {
	svc, sender, store := setupSMSService(t)
	ctx := context.Background()

	if err := smsoptout.RecordOptOut(ctx, store, "+15551234567"); err != nil {
		t.Fatalf("RecordOptOut: %v", err)
	}
	if err := svc.SendPhoneOptInWelcome(ctx, phoneUser("+15551234567", "")); err != nil {
		t.Fatalf("SendPhoneOptInWelcome: %v", err)
	}
	if got := len(sender.Messages()); got != 0 {
		t.Errorf("opted-out number must not be texted; got %d sends", got)
	}
}

func TestSendPhoneOptInWelcome_NoopWhenChannelDisabled(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	// No WithSMS option — the platform SMS channel is disabled.
	svc := NewService(nil, store).(*service)
	if err := svc.SendPhoneOptInWelcome(context.Background(), phoneUser("+15551234567", "")); err != nil {
		t.Errorf("disabled channel should be a no-op, got err: %v", err)
	}
}

func TestSendPhoneOptInWelcome_NoopWhenNoPhone(t *testing.T) {
	svc, sender, _ := setupSMSService(t)
	if err := svc.SendPhoneOptInWelcome(context.Background(), phoneUser("", "")); err != nil {
		t.Fatalf("SendPhoneOptInWelcome: %v", err)
	}
	if got := len(sender.Messages()); got != 0 {
		t.Errorf("no phone handle should send nothing; got %d", got)
	}
}

func TestSendPhoneOptInWelcome_SendsDuringQuietHours(t *testing.T) {
	svc, sender, _ := setupSMSService(t)
	denver, _ := time.LoadLocation("America/Denver")
	// 3am local is inside quiet hours — but a welcome is a real-time response to
	// the user's own opt-in action, so it must still send (unlike content SMS).
	ctx := clock.WithSimulationTime(context.Background(), time.Date(2026, 6, 26, 3, 0, 0, 0, denver))

	if err := svc.SendPhoneOptInWelcome(ctx, phoneUser("+15551234567", "America/Denver")); err != nil {
		t.Fatalf("SendPhoneOptInWelcome: %v", err)
	}
	if got := len(sender.Messages()); got != 1 {
		t.Errorf("welcome must send even during quiet hours; got %d sends", got)
	}
}

func TestSendPhoneOptInWelcome_LocalizesSpanish(t *testing.T) {
	svc, sender, _ := setupSMSService(t)
	u := &models.User{Id: "u-es", PhoneNumber: proto.String("+15551239999"), PreferredLanguage: proto.String("es")}

	if err := svc.SendPhoneOptInWelcome(context.Background(), u); err != nil {
		t.Fatalf("SendPhoneOptInWelcome: %v", err)
	}
	msgs := sender.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 SMS sent, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Body, "Bienvenido") {
		t.Errorf("expected Spanish welcome, got %q", msgs[0].Body)
	}
}

// A deployment that supplies no app URL still has to send a compliant message:
// the catalog drops the "Connect at …" sentence rather than rendering a
// dangling "Connect at " or the literal template (#2953).
func TestSendPhoneOptInWelcome_OmitsCTAWhenNoAppURL(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	sender := &sms.MockSMSSender{}
	svc := NewService(nil, store, WithSMS(SMSChannel{
		Enabled: true,
		Sender:  sender,
		// AppBaseURL deliberately unset.
	})).(*service)

	if err := svc.SendPhoneOptInWelcome(context.Background(), phoneUser("+15551234567", "")); err != nil {
		t.Fatalf("SendPhoneOptInWelcome: %v", err)
	}
	msgs := sender.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 SMS sent, got %d", len(msgs))
	}
	if msgs[0].Body != welcomeDisclosureEN {
		t.Errorf("body = %q, want %q", msgs[0].Body, welcomeDisclosureEN)
	}
	// The compliance elements are not optional, whatever the URL is.
	for _, required := range []string{"Reply HELP for help", "STOP to opt out", "Message frequency varies"} {
		if !strings.Contains(msgs[0].Body, required) {
			t.Errorf("welcome missing required CTIA element %q: %q", required, msgs[0].Body)
		}
	}
}
