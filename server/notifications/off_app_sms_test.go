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

// setupSMSService builds a notification service with the SMS channel enabled
// and a mock sender, plus a fresh test store.
func setupSMSService(t *testing.T) (*service, *sms.MockSMSSender, *storage.ProtoSQLStorage) {
	t.Helper()
	store, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	sender := &sms.MockSMSSender{}
	svc := NewService(nil, store, WithSMS(SMSChannel{
		Enabled:    true,
		Sender:     sender,
		AppBaseURL: "https://test.example.com",
	})).(*service)
	return svc, sender, store
}

func phoneUser(phone, tz string) *models.User {
	u := &models.User{Id: "u-sms", PreferredTimezone: tz}
	if phone != "" {
		u.PhoneNumber = proto.String(phone)
	}
	return u
}

func TestSendOffAppSMS_SendsToOptedInNumber(t *testing.T) {
	svc, sender, _ := setupSMSService(t)
	ctx := context.Background()

	n := &models.Notification{Title: "Interest in your drill", Body: "Alex wants to borrow it."}
	if sent := svc.sendOffAppSMS(ctx, phoneUser("+14155551234", ""), n); !sent {
		t.Fatal("expected sendOffAppSMS to report sent=true")
	}

	msgs := sender.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 SMS sent, got %d", len(msgs))
	}
	if msgs[0].To != "+14155551234" {
		t.Errorf("To = %q", msgs[0].To)
	}
	body := msgs[0].Body
	if !strings.Contains(body, "Alex wants to borrow it.") {
		t.Errorf("body missing notification content: %q", body)
	}
	if !strings.Contains(body, "https://test.example.com") {
		t.Errorf("body missing branded link: %q", body)
	}
	if !strings.Contains(body, "Reply STOP to opt out") {
		t.Errorf("body missing opt-out footer: %q", body)
	}
}

func TestSendOffAppSMS_SuppressesOptedOut(t *testing.T) {
	svc, sender, store := setupSMSService(t)
	ctx := context.Background()

	if err := smsoptout.RecordOptOut(ctx, store, "+14155551234"); err != nil {
		t.Fatalf("RecordOptOut: %v", err)
	}
	if sent := svc.sendOffAppSMS(ctx, phoneUser("+14155551234", ""), &models.Notification{Title: "Hi", Body: "There"}); sent {
		t.Error("opted-out number must not be texted")
	}
	if got := len(sender.Messages()); got != 0 {
		t.Errorf("expected 0 sends, got %d", got)
	}
}

func TestSendOffAppSMS_SkipsUndialableNumber(t *testing.T) {
	svc, sender, _ := setupSMSService(t)
	// A shape-valid but undialable number (the Firebase OTP test number) must
	// never reach the provider — it is skipped before the send and the caller
	// falls back to email.
	if sent := svc.sendOffAppSMS(context.Background(), phoneUser("+15555550001", ""), &models.Notification{Body: "x"}); sent {
		t.Error("an undialable number must not be texted")
	}
	if got := len(sender.Messages()); got != 0 {
		t.Errorf("expected 0 sends to an undialable number, got %d", got)
	}
}

func TestSendOffAppSMS_QuietHours(t *testing.T) {
	svc, sender, _ := setupSMSService(t)
	denver, _ := time.LoadLocation("America/Denver")
	// 3am local is inside quiet hours.
	ctx := clock.WithSimulationTime(context.Background(), time.Date(2026, 6, 26, 3, 0, 0, 0, denver))

	if sent := svc.sendOffAppSMS(ctx, phoneUser("+14155551234", "America/Denver"), &models.Notification{Body: "x"}); sent {
		t.Error("must not text during the recipient's quiet hours")
	}
	if got := len(sender.Messages()); got != 0 {
		t.Errorf("expected 0 sends during quiet hours, got %d", got)
	}
}

func TestSendOffAppSMS_DisabledChannel(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	// No WithSMS option → channel disabled.
	svc := NewService(nil, store).(*service)

	if sent := svc.sendOffAppSMS(context.Background(), phoneUser("+14155551234", ""), &models.Notification{Body: "x"}); sent {
		t.Error("disabled channel must not send")
	}
}

func TestSendOffAppSMS_NoPhone(t *testing.T) {
	svc, sender, _ := setupSMSService(t)
	if sent := svc.sendOffAppSMS(context.Background(), phoneUser("", ""), &models.Notification{Body: "x"}); sent {
		t.Error("a recipient with no phone must not be texted")
	}
	if got := len(sender.Messages()); got != 0 {
		t.Errorf("expected 0 sends, got %d", got)
	}
}
