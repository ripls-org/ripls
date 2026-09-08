package notifications

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// fakeEmailSender records SendNotification calls and can simulate failures.
type fakeEmailSender struct {
	sent []email.NotificationEmailInput
	err  error
}

func (f *fakeEmailSender) SendNotification(_ context.Context, in email.NotificationEmailInput) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, in)
	return nil
}

const testUnsubSecret = "test-unsubscribe-secret"

// setupOffAppService builds a notification service with the off-app email
// channel enabled and a fake sender, plus a fresh test store.
func setupOffAppService(t *testing.T, sender EmailSender) (*service, *storage.ProtoSQLStorage) {
	t.Helper()
	store, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	svc := NewService(nil, store, WithOffAppEmail(OffAppEmailConfig{
		Enabled:     true,
		Sender:      sender,
		AppBaseURL:  "https://test.example.com",
		UnsubSecret: []byte(testUnsubSecret),
	})).(*service)
	return svc, store
}

// insertUserNoDevice inserts a User with no registered device and returns its ID.
func insertUserNoDevice(t *testing.T, store *storage.ProtoSQLStorage, emailAddr string, optedOut bool) string {
	t.Helper()
	u := &models.User{
		Email:               emailAddr,
		Name:                "Deviceless",
		Role:                models.Role_ROLE_USER,
		PreferredLanguage:   ptr("es"),
		OffAppEmailOptedOut: optedOut,
	}
	id, err := store.Insert(context.Background(), u)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func ptr(s string) *string { return &s }

func TestNotifyUser_OffAppEmail_SendsToDevicelessUser(t *testing.T) {
	sender := &fakeEmailSender{}
	svc, store := setupOffAppService(t, sender)
	userID := insertUserNoDevice(t, store, "deviceless@example.com", false)

	notification := &models.Notification{Title: "Alex RSVP'd yes", Body: "Tap to see who's coming."}
	if err := svc.NotifyUser(context.Background(), userID, notification); err != nil {
		t.Fatalf("NotifyUser: %v", err)
	}

	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 email sent, got %d", len(sender.sent))
	}
	got := sender.sent[0]
	if got.ToEmail != "deviceless@example.com" {
		t.Errorf("ToEmail = %q, want deviceless@example.com", got.ToEmail)
	}
	if got.Title != notification.Title || got.Body != notification.Body {
		t.Errorf("title/body not forwarded: got (%q,%q)", got.Title, got.Body)
	}
	if got.PreferredLanguage != "es" {
		t.Errorf("PreferredLanguage = %q, want es (from User)", got.PreferredLanguage)
	}
	// The unsubscribe link must carry a token that verifies for this user.
	if !strings.Contains(got.UnsubscribeURL, "u="+userID) {
		t.Errorf("unsubscribe URL missing user id: %q", got.UnsubscribeURL)
	}
	token := extractQueryParam(got.UnsubscribeURL, "token")
	if !auth.VerifyUnsubscribeToken([]byte(testUnsubSecret), userID, token) {
		t.Errorf("unsubscribe token does not verify: %q", got.UnsubscribeURL)
	}
}

func TestNotifyUser_OffAppEmail_SuppressedWhenOptedOut(t *testing.T) {
	sender := &fakeEmailSender{}
	svc, store := setupOffAppService(t, sender)
	userID := insertUserNoDevice(t, store, "optedout@example.com", true)

	if err := svc.NotifyUser(context.Background(), userID, &models.Notification{Title: "x", Body: "y"}); err != nil {
		t.Fatalf("NotifyUser: %v", err)
	}
	if len(sender.sent) != 0 {
		t.Errorf("expected no email for opted-out user, got %d", len(sender.sent))
	}
}

func TestNotifyUser_OffAppEmail_SuppressedWhenNoEmail(t *testing.T) {
	sender := &fakeEmailSender{}
	svc, store := setupOffAppService(t, sender)
	userID := insertUserNoDevice(t, store, "", false)

	if err := svc.NotifyUser(context.Background(), userID, &models.Notification{Title: "x", Body: "y"}); err != nil {
		t.Fatalf("NotifyUser: %v", err)
	}
	if len(sender.sent) != 0 {
		t.Errorf("expected no email for user with no address, got %d", len(sender.sent))
	}
}

func TestNotifyUser_OffAppEmail_DisabledByDefault(t *testing.T) {
	// No WithOffAppEmail option: the channel is inert even for a deviceless
	// user who has an email on file.
	store, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	svc := NewService(nil, store).(*service)
	userID := insertUserNoDevice(t, store, "deviceless@example.com", false)

	if err := svc.NotifyUser(context.Background(), userID, &models.Notification{Title: "x", Body: "y"}); err != nil {
		t.Fatalf("NotifyUser: %v", err)
	}
	// Nothing to assert beyond the no-panic/no-error path: with no sender wired
	// the dispatch is a logged no-op (historical suppressed_no_devices behavior).
}

func TestNotifyUser_OffAppEmail_SendFailureDoesNotFail(t *testing.T) {
	sender := &fakeEmailSender{err: errors.New("mailgun down")}
	svc, store := setupOffAppService(t, sender)
	userID := insertUserNoDevice(t, store, "deviceless@example.com", false)

	// A send failure is best-effort: NotifyUser must still return nil.
	if err := svc.NotifyUser(context.Background(), userID, &models.Notification{Title: "x", Body: "y"}); err != nil {
		t.Errorf("NotifyUser should not fail when off-app send fails, got: %v", err)
	}
}

// extractQueryParam pulls a query param value out of a URL without importing
// net/url in the test (the URL is built by the code under test).
func extractQueryParam(rawURL, key string) string {
	idx := strings.Index(rawURL, key+"=")
	if idx < 0 {
		return ""
	}
	rest := rawURL[idx+len(key)+1:]
	if amp := strings.IndexByte(rest, '&'); amp >= 0 {
		rest = rest[:amp]
	}
	return rest
}
