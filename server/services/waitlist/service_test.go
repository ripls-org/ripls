package waitlist

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/email"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/storage"
)

const emailTimeout = 2 * time.Second

// waitForEmails waits for the expected number of emails to be sent, with timeout.
func waitForEmails(t *testing.T, emailChan chan struct{}, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case <-emailChan:
			// Email received
		case <-time.After(emailTimeout):
			t.Fatalf("Timed out waiting for email %d of %d", i+1, count)
		}
	}
}

func setupTestService(t *testing.T) (*Service, *storage.ProtoSQLStorage, *email.MockEmailService) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	// Buffer size of 10 to avoid blocking goroutines
	emailChan := make(chan struct{}, 10)
	mockEmail := &email.MockEmailService{
		OnEmailSent: emailChan,
	}
	notifyEmail := "dev@example.com"

	svc := New(sqlStorage, mockEmail, notifyEmail)
	return svc, sqlStorage, mockEmail
}

func TestJoinWaitlist_Success(t *testing.T) {
	svc, sqlStorage, mockEmail := setupTestService(t)
	ctx := context.Background()

	req := connect.NewRequest(&api.JoinWaitlistRequest{
		Email: "newuser@example.com",
	})

	resp, err := svc.JoinWaitlist(ctx, req)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if resp.Msg.AlreadyRegistered {
		t.Error("Expected AlreadyRegistered to be false for new signup")
	}

	if resp.Msg.Message == "" {
		t.Error("Expected a welcome message")
	}

	// Verify entry was created in storage
	entries, err := sqlStorage.QueryByField(ctx, "email", "newuser@example.com", &models.WaitlistEntry{})
	if err != nil {
		t.Fatalf("Failed to query storage: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry in storage, got %d", len(entries))
	}

	entry := entries[0].(*models.WaitlistEntry)
	if entry.Email != "newuser@example.com" {
		t.Errorf("Expected email 'newuser@example.com', got '%s'", entry.Email)
	}
	if entry.Id == "" {
		t.Error("Expected entry to have an ID")
	}

	// Wait for both emails (welcome + notification) to be sent
	waitForEmails(t, mockEmail.OnEmailSent, 2)

	// Verify welcome email was sent
	if len(mockEmail.WaitlistWelcomes) != 1 {
		t.Errorf("Expected 1 welcome email, got %d", len(mockEmail.WaitlistWelcomes))
	}

	// Verify notification was sent
	if len(mockEmail.WaitlistNotifications) != 1 {
		t.Errorf("Expected 1 notification, got %d", len(mockEmail.WaitlistNotifications))
	}
}

func TestJoinWaitlist_WithOptionalFields(t *testing.T) {
	svc, sqlStorage, mockEmail := setupTestService(t)
	ctx := context.Background()

	name := "Alice"
	referral := "A friend told me"
	use := "Share camping gear with my hiking group"

	req := connect.NewRequest(&api.JoinWaitlistRequest{
		Email:          "alice@example.com",
		Name:           &name,
		ReferralSource: &referral,
		IntendedUse:    &use,
	})

	resp, err := svc.JoinWaitlist(ctx, req)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if resp.Msg.AlreadyRegistered {
		t.Error("Expected AlreadyRegistered to be false")
	}

	// Verify optional fields persisted
	entries, err := sqlStorage.QueryByField(ctx, "email", "alice@example.com", &models.WaitlistEntry{})
	if err != nil {
		t.Fatalf("Failed to query storage: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	entry := entries[0].(*models.WaitlistEntry)
	if entry.GetName() != "Alice" {
		t.Errorf("Expected name 'Alice', got '%s'", entry.GetName())
	}
	if entry.GetReferralSource() != "A friend told me" {
		t.Errorf("Expected referral source 'A friend told me', got '%s'", entry.GetReferralSource())
	}
	if entry.GetIntendedUse() != "Share camping gear with my hiking group" {
		t.Errorf("Expected intended use, got '%s'", entry.GetIntendedUse())
	}

	// Wait for emails and verify notification includes details
	waitForEmails(t, mockEmail.OnEmailSent, 2)

	if len(mockEmail.WaitlistNotifications) != 1 {
		t.Fatalf("Expected 1 notification, got %d", len(mockEmail.WaitlistNotifications))
	}
	notif := mockEmail.WaitlistNotifications[0]
	if notif.Details.Name != "Alice" {
		t.Errorf("Expected notification name 'Alice', got '%s'", notif.Details.Name)
	}
	if notif.Details.ReferralSource != "A friend told me" {
		t.Errorf("Expected notification referral, got '%s'", notif.Details.ReferralSource)
	}
	if notif.Details.IntendedUse != "Share camping gear with my hiking group" {
		t.Errorf("Expected notification intended use, got '%s'", notif.Details.IntendedUse)
	}
}

func TestJoinWaitlist_WithPartialOptionalFields(t *testing.T) {
	svc, sqlStorage, mockEmail := setupTestService(t)
	ctx := context.Background()

	name := "Bob"

	req := connect.NewRequest(&api.JoinWaitlistRequest{
		Email: "bob@example.com",
		Name:  &name,
		// ReferralSource and IntendedUse intentionally omitted
	})

	resp, err := svc.JoinWaitlist(ctx, req)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if resp.Msg.AlreadyRegistered {
		t.Error("Expected AlreadyRegistered to be false")
	}

	// Verify name persisted, other optional fields are nil
	entries, err := sqlStorage.QueryByField(ctx, "email", "bob@example.com", &models.WaitlistEntry{})
	if err != nil {
		t.Fatalf("Failed to query storage: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	entry := entries[0].(*models.WaitlistEntry)
	if entry.GetName() != "Bob" {
		t.Errorf("Expected name 'Bob', got '%s'", entry.GetName())
	}
	if entry.ReferralSource != nil {
		t.Errorf("Expected nil referral source, got '%s'", entry.GetReferralSource())
	}
	if entry.IntendedUse != nil {
		t.Errorf("Expected nil intended use, got '%s'", entry.GetIntendedUse())
	}

	// Verify notification details reflect partial fields
	waitForEmails(t, mockEmail.OnEmailSent, 2)

	if len(mockEmail.WaitlistNotifications) != 1 {
		t.Fatalf("Expected 1 notification, got %d", len(mockEmail.WaitlistNotifications))
	}
	notif := mockEmail.WaitlistNotifications[0]
	if notif.Details.Name != "Bob" {
		t.Errorf("Expected notification name 'Bob', got '%s'", notif.Details.Name)
	}
	if notif.Details.ReferralSource != "" {
		t.Errorf("Expected empty referral source in notification, got '%s'", notif.Details.ReferralSource)
	}
	if notif.Details.IntendedUse != "" {
		t.Errorf("Expected empty intended use in notification, got '%s'", notif.Details.IntendedUse)
	}
}

func TestJoinWaitlist_EmailOnlyHasNilOptionalFields(t *testing.T) {
	svc, sqlStorage, mockEmail := setupTestService(t)
	ctx := context.Background()

	req := connect.NewRequest(&api.JoinWaitlistRequest{
		Email: "minimal@example.com",
	})

	_, err := svc.JoinWaitlist(ctx, req)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	entries, err := sqlStorage.QueryByField(ctx, "email", "minimal@example.com", &models.WaitlistEntry{})
	if err != nil {
		t.Fatalf("Failed to query storage: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	entry := entries[0].(*models.WaitlistEntry)
	if entry.Name != nil {
		t.Errorf("Expected nil name, got '%s'", entry.GetName())
	}
	if entry.ReferralSource != nil {
		t.Errorf("Expected nil referral source, got '%s'", entry.GetReferralSource())
	}
	if entry.IntendedUse != nil {
		t.Errorf("Expected nil intended use, got '%s'", entry.GetIntendedUse())
	}

	// Notification details should have empty strings for optional fields
	waitForEmails(t, mockEmail.OnEmailSent, 2)

	notif := mockEmail.WaitlistNotifications[0]
	if notif.Details.Email != "minimal@example.com" {
		t.Errorf("Expected email in notification, got '%s'", notif.Details.Email)
	}
	if notif.Details.Name != "" {
		t.Errorf("Expected empty name in notification, got '%s'", notif.Details.Name)
	}
}

func TestJoinWaitlist_DuplicateIgnoresOptionalFields(t *testing.T) {
	svc, sqlStorage, mockEmail := setupTestService(t)
	ctx := context.Background()

	name := "Carol"
	use := "Share tools"

	// First signup with optional fields
	req1 := connect.NewRequest(&api.JoinWaitlistRequest{
		Email:       "carol@example.com",
		Name:        &name,
		IntendedUse: &use,
	})

	_, err := svc.JoinWaitlist(ctx, req1)
	if err != nil {
		t.Fatalf("First signup failed: %v", err)
	}
	waitForEmails(t, mockEmail.OnEmailSent, 2)
	mockEmail.Reset()

	// Second signup, same email, different optional fields — should still be duplicate
	name2 := "Carol D."
	req2 := connect.NewRequest(&api.JoinWaitlistRequest{
		Email: "carol@example.com",
		Name:  &name2,
	})

	resp, err := svc.JoinWaitlist(ctx, req2)
	if err != nil {
		t.Fatalf("Second signup failed: %v", err)
	}
	if !resp.Msg.AlreadyRegistered {
		t.Error("Expected duplicate to be detected regardless of optional fields")
	}

	// Original entry should be unchanged
	entries, err := sqlStorage.QueryByField(ctx, "email", "carol@example.com", &models.WaitlistEntry{})
	if err != nil {
		t.Fatalf("Failed to query storage: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry (no duplicate created), got %d", len(entries))
	}
	entry := entries[0].(*models.WaitlistEntry)
	if entry.GetName() != "Carol" {
		t.Errorf("Expected original name 'Carol' preserved, got '%s'", entry.GetName())
	}
}

func TestJoinWaitlist_DuplicateEmail(t *testing.T) {
	svc, _, mockEmail := setupTestService(t)
	ctx := context.Background()

	// First signup
	req := connect.NewRequest(&api.JoinWaitlistRequest{
		Email: "duplicate@example.com",
	})

	resp1, err := svc.JoinWaitlist(ctx, req)
	if err != nil {
		t.Fatalf("First signup failed: %v", err)
	}
	if resp1.Msg.AlreadyRegistered {
		t.Error("First signup should not be marked as already registered")
	}

	// Wait for emails from first signup
	waitForEmails(t, mockEmail.OnEmailSent, 2)
	mockEmail.Reset()

	// Second signup with same email
	resp2, err := svc.JoinWaitlist(ctx, req)
	if err != nil {
		t.Fatalf("Second signup failed: %v", err)
	}

	if !resp2.Msg.AlreadyRegistered {
		t.Error("Second signup should be marked as already registered")
	}

	// No emails should be sent for duplicate - verify channel is empty after brief wait
	select {
	case <-mockEmail.OnEmailSent:
		t.Error("Expected no emails for duplicate signup, but received one")
	case <-time.After(50 * time.Millisecond):
		// Expected - no emails sent
	}

	if len(mockEmail.WaitlistWelcomes) != 0 {
		t.Errorf("Expected 0 welcome emails for duplicate, got %d", len(mockEmail.WaitlistWelcomes))
	}
}

func TestJoinWaitlist_InvalidEmail(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	testCases := []string{
		"not-an-email",
		"missing@",
		"@nodomain.com",
		"",
	}

	for _, invalidEmail := range testCases {
		t.Run(invalidEmail, func(t *testing.T) {
			req := connect.NewRequest(&api.JoinWaitlistRequest{
				Email: invalidEmail,
			})

			_, err := svc.JoinWaitlist(ctx, req)
			if err == nil {
				t.Errorf("Expected error for invalid email '%s', got nil", invalidEmail)
			}

			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Errorf("Expected InvalidArgument error for '%s', got: %v", invalidEmail, err)
			}
		})
	}
}

// TestJoinWaitlist_PropagatesLocaleIntoBackgroundedSend verifies that
// the JoinWaitlist handler captures the request's Accept-Language and
// injects it into the detached background ctx used by the welcome-email
// goroutine. Without this propagation, every welcome email would render
// in the default locale regardless of the signup request's headers.
func TestJoinWaitlist_PropagatesLocaleIntoBackgroundedSend(t *testing.T) {
	svc, _, mockEmail := setupTestService(t)
	ctx := l10n.WithAcceptLanguage(context.Background(), "es")

	req := connect.NewRequest(&api.JoinWaitlistRequest{
		Email: "spanish-signup@example.com",
	})
	if _, err := svc.JoinWaitlist(ctx, req); err != nil {
		t.Fatalf("JoinWaitlist: %v", err)
	}

	waitForEmails(t, mockEmail.OnEmailSent, 2)

	if len(mockEmail.WaitlistWelcomes) != 1 {
		t.Fatalf("expected 1 welcome email, got %d", len(mockEmail.WaitlistWelcomes))
	}
	got := mockEmail.WaitlistWelcomes[0]
	if got.Locale != "es" {
		t.Errorf("welcome email rendered with locale %q; want %q (Accept-Language not propagated through goroutine)", got.Locale, "es")
	}
}

func TestJoinWaitlist_NoNotifyEmailConfigured(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	emailChan := make(chan struct{}, 10)
	mockEmail := &email.MockEmailService{
		OnEmailSent: emailChan,
	}
	// Create service without notify email
	svc := New(sqlStorage, mockEmail, "")

	ctx := context.Background()
	req := connect.NewRequest(&api.JoinWaitlistRequest{
		Email: "user@example.com",
	})

	resp, err := svc.JoinWaitlist(ctx, req)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if resp.Msg.AlreadyRegistered {
		t.Error("Expected successful signup")
	}

	// Wait for welcome email only (no notification since no notify email configured)
	waitForEmails(t, emailChan, 1)

	// Welcome email should still be sent
	if len(mockEmail.WaitlistWelcomes) != 1 {
		t.Errorf("Expected 1 welcome email, got %d", len(mockEmail.WaitlistWelcomes))
	}

	// But no notification should be sent
	if len(mockEmail.WaitlistNotifications) != 0 {
		t.Errorf("Expected 0 notifications when no notify email configured, got %d", len(mockEmail.WaitlistNotifications))
	}
}
