package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications/sms"
	"go.ripls.org/ripls/server/smsoptout"
	"go.ripls.org/ripls/server/storage"
)

// setupBothChannels wires both off-app channels (email + SMS enabled) so the
// dispatcher's handle-based channel selection can be exercised end-to-end
// through NotifyUser (a deviceless recipient falls through to dispatchOffApp).
func setupBothChannels(t *testing.T) (*service, *fakeEmailSender, *sms.MockSMSSender, *storage.ProtoSQLStorage) {
	t.Helper()
	store, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	es := &fakeEmailSender{}
	ms := &sms.MockSMSSender{}
	svc := NewService(nil, store,
		WithOffAppEmail(OffAppEmailConfig{
			Enabled: true, Sender: es, AppBaseURL: "https://test.example.com", UnsubSecret: []byte(testUnsubSecret),
		}),
		WithSMS(SMSChannel{Enabled: true, Sender: ms, AppBaseURL: "https://test.example.com"}),
	).(*service)
	return svc, es, ms, store
}

func insertDevicelessUser(t *testing.T, store *storage.ProtoSQLStorage, emailAddr, phone, tz string) string {
	t.Helper()
	u := &models.User{
		Email:             emailAddr,
		Name:              "Deviceless",
		Role:              models.Role_ROLE_USER,
		PreferredTimezone: tz,
	}
	if phone != "" {
		u.PhoneNumber = proto.String(phone)
	}
	id, err := store.Insert(context.Background(), u)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func notify(t *testing.T, svc *service, ctx context.Context, userID string) {
	t.Helper()
	if err := svc.NotifyUser(ctx, userID, &models.Notification{Title: "Heads up", Body: "Something happened."}); err != nil {
		t.Fatalf("NotifyUser: %v", err)
	}
}

func TestDispatch_PhoneRecipientPrefersSMS(t *testing.T) {
	svc, es, ms, store := setupBothChannels(t)
	id := insertDevicelessUser(t, store, "both@example.com", "+14155551234", "")

	notify(t, svc, context.Background(), id)

	if len(ms.Messages()) != 1 {
		t.Errorf("expected 1 SMS, got %d", len(ms.Messages()))
	}
	if len(es.sent) != 0 {
		t.Errorf("a phone recipient must not also get email; got %d emails", len(es.sent))
	}
}

func TestDispatch_OptedOutSMSFallsBackToEmail(t *testing.T) {
	svc, es, ms, store := setupBothChannels(t)
	ctx := context.Background()
	id := insertDevicelessUser(t, store, "both@example.com", "+14155551234", "")
	if err := smsoptout.RecordOptOut(ctx, store, "+14155551234"); err != nil {
		t.Fatalf("RecordOptOut: %v", err)
	}

	notify(t, svc, ctx, id)

	if len(ms.Messages()) != 0 {
		t.Errorf("opted-out number must not be texted, got %d", len(ms.Messages()))
	}
	if len(es.sent) != 1 {
		t.Errorf("expected email fallback, got %d emails", len(es.sent))
	}
}

func TestDispatch_UndialableNumberFallsBackToEmail(t *testing.T) {
	svc, es, ms, store := setupBothChannels(t)
	// A phone-bearing recipient prefers SMS, but an undialable number (here the
	// Firebase OTP test number) is skipped before the provider call and the
	// dispatcher falls back to email.
	id := insertDevicelessUser(t, store, "both@example.com", "+15555550001", "")

	notify(t, svc, context.Background(), id)

	if len(ms.Messages()) != 0 {
		t.Errorf("undialable number must not be texted, got %d", len(ms.Messages()))
	}
	if len(es.sent) != 1 {
		t.Errorf("expected email fallback for an undialable number, got %d emails", len(es.sent))
	}
}

func TestDispatch_QuietHoursFallsBackToEmail(t *testing.T) {
	svc, es, ms, store := setupBothChannels(t)
	denver, _ := time.LoadLocation("America/Denver")
	ctx := clock.WithSimulationTime(context.Background(), time.Date(2026, 6, 26, 3, 0, 0, 0, denver))
	id := insertDevicelessUser(t, store, "both@example.com", "+14155551234", "America/Denver")

	notify(t, svc, ctx, id)

	if len(ms.Messages()) != 0 {
		t.Errorf("must not text during quiet hours, got %d", len(ms.Messages()))
	}
	if len(es.sent) != 1 {
		t.Errorf("expected email fallback during quiet hours, got %d emails", len(es.sent))
	}
}

func TestDispatch_NoPhoneUsesEmail(t *testing.T) {
	svc, es, ms, store := setupBothChannels(t)
	id := insertDevicelessUser(t, store, "emailonly@example.com", "", "")

	notify(t, svc, context.Background(), id)

	if len(ms.Messages()) != 0 {
		t.Errorf("no phone → no SMS, got %d", len(ms.Messages()))
	}
	if len(es.sent) != 1 {
		t.Errorf("expected email, got %d", len(es.sent))
	}
}

func TestDispatch_SMSDisabledUsesEmail(t *testing.T) {
	// Email-only service (no WithSMS), but the recipient has a phone: must email.
	sender := &fakeEmailSender{}
	svc, store := setupOffAppService(t, sender)
	id := insertDevicelessUser(t, store, "both@example.com", "+14155551234", "")

	notify(t, svc, context.Background(), id)

	if len(sender.sent) != 1 {
		t.Errorf("SMS disabled → email, got %d emails", len(sender.sent))
	}
}

func TestDispatch_NoEmailAndOptedOutSMSSuppresses(t *testing.T) {
	svc, es, ms, store := setupBothChannels(t)
	ctx := context.Background()
	id := insertDevicelessUser(t, store, "", "+14155551234", "") // phone only, no email
	if err := smsoptout.RecordOptOut(ctx, store, "+14155551234"); err != nil {
		t.Fatalf("RecordOptOut: %v", err)
	}

	notify(t, svc, ctx, id)

	if len(ms.Messages()) != 0 || len(es.sent) != 0 {
		t.Errorf("both channels unavailable → suppress; got sms=%d email=%d", len(ms.Messages()), len(es.sent))
	}
}

// insertSimulationUser inserts a deviceless user tagged with a SimulationId —
// the marker the E2E suite and load-test harness set at registration. Such users
// must never reach a real (or mock) delivery provider (#2588).
func insertSimulationUser(t *testing.T, store *storage.ProtoSQLStorage, emailAddr, phone string) string {
	t.Helper()
	u := &models.User{
		Email:        emailAddr,
		Name:         "Simulation",
		Role:         models.Role_ROLE_USER,
		SimulationId: proto.String("sim-test-123"),
	}
	if phone != "" {
		u.PhoneNumber = proto.String(phone)
	}
	id, err := store.Insert(context.Background(), u)
	if err != nil {
		t.Fatalf("insert simulation user: %v", err)
	}
	return id
}

// ctxWithLogBuffer returns a context whose logger writes JSON lines into
// buf, so tests can assert on the dispatch outcome log lines.
func ctxWithLogBuffer(buf *bytes.Buffer) context.Context {
	logger := logging.NewLogger(logging.Options{Format: "json", Output: buf})
	return logging.WithLogger(context.Background(), logger)
}

// simSuppressCount counts "suppressed_simulation" outcome log entries for
// the given channel — the observable record of the simulation guard (the
// log line feeds Cloud Logging queries; there is no in-process counter).
func simSuppressCount(buf *bytes.Buffer, channel string) int {
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["outcome"] == "suppressed_simulation" && entry["channel"] == channel {
			count++
		}
	}
	return count
}

func TestDispatch_SimulationPhoneRecipientSuppressedOnSMSChannel(t *testing.T) {
	svc, es, ms, store := setupBothChannels(t)
	// A phone-bearing recipient routes to SMS (the expected channel); a simulation
	// recipient must reach neither provider, and the suppression is recorded as SMS.
	id := insertSimulationUser(t, store, "sim@e2etest.example.com", "+14155551234")

	var buf bytes.Buffer
	notify(t, svc, ctxWithLogBuffer(&buf), id)

	if len(ms.Messages()) != 0 {
		t.Errorf("simulation recipient must not be texted, got %d SMS", len(ms.Messages()))
	}
	if len(es.sent) != 0 {
		t.Errorf("simulation recipient must not be emailed either, got %d emails", len(es.sent))
	}
	if got := simSuppressCount(&buf, "sms"); got != 1 {
		t.Errorf("expected one sms suppressed_simulation log, got %d", got)
	}
}

func TestDispatch_SimulationEmailOnlyRecipientSuppressedOnEmailChannel(t *testing.T) {
	svc, es, ms, store := setupBothChannels(t)
	// No phone → the expected channel is email; a simulation recipient is
	// suppressed there before any send, recorded as the email channel.
	id := insertSimulationUser(t, store, "sim@e2etest.example.com", "")

	var buf bytes.Buffer
	notify(t, svc, ctxWithLogBuffer(&buf), id)

	if len(es.sent) != 0 {
		t.Errorf("simulation recipient must not be emailed, got %d emails", len(es.sent))
	}
	if len(ms.Messages()) != 0 {
		t.Errorf("no phone → no SMS, got %d", len(ms.Messages()))
	}
	if got := simSuppressCount(&buf, "email"); got != 1 {
		t.Errorf("expected one email suppressed_simulation log, got %d", got)
	}
}

func TestDispatch_NonSimulationRecipientNotSuppressed(t *testing.T) {
	// Regression: a real (untagged) deviceless recipient still gets email and does
	// not trip the simulation guard — the guard is specific to simulation users.
	svc, es, _, store := setupBothChannels(t)
	id := insertDevicelessUser(t, store, "real@example.com", "", "")

	var buf bytes.Buffer
	notify(t, svc, ctxWithLogBuffer(&buf), id)

	if len(es.sent) != 1 {
		t.Errorf("non-simulation recipient should be emailed, got %d", len(es.sent))
	}
	if got := simSuppressCount(&buf, "email"); got != 0 {
		t.Errorf("non-simulation recipient must not log suppressed_simulation, got %d", got)
	}
}
