package email

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestInviteSender_SendsLocalizedEmailWithUnsubscribe(t *testing.T) {
	mock := &MockEmailService{}
	sender := NewInviteSender(mock, "https://example.com", []byte("secret"), nil)

	err := sender.SendInviteEmail(context.Background(), InviteEmailInput{
		ToEmail:  "friend@example.com",
		HostName: "Alex",
		ShareURL: "https://example.com/go/ABC12345",
	})
	if err != nil {
		t.Fatalf("SendInviteEmail: %v", err)
	}
	if len(mock.Notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(mock.Notifications))
	}
	n := mock.Notifications[0]
	if n.ToEmail != "friend@example.com" {
		t.Errorf("ToEmail = %q", n.ToEmail)
	}
	if !strings.Contains(n.Title, "Alex") {
		t.Errorf("subject should name the host, got %q", n.Title)
	}
	if n.Body == "" {
		t.Error("expected a non-empty localized body")
	}
	if n.ActionURL != "https://example.com/go/ABC12345" {
		t.Errorf("ActionURL should be the share url, got %q", n.ActionURL)
	}
	// Unsubscribe link is keyed on the (URL-encoded) email handle + signed token.
	if !strings.Contains(n.UnsubscribeURL, "u=friend%40example.com") {
		t.Errorf("unsubscribe URL should carry the email subject, got %q", n.UnsubscribeURL)
	}
	if !strings.Contains(n.UnsubscribeURL, "token=") {
		t.Errorf("unsubscribe URL should carry a token, got %q", n.UnsubscribeURL)
	}
}

func TestInviteSender_SkipsSuppressed(t *testing.T) {
	mock := &MockEmailService{}
	sender := NewInviteSender(mock, "https://example.com", []byte("secret"),
		func(_ context.Context, _ string) (bool, error) { return true, nil })

	if err := sender.SendInviteEmail(context.Background(), InviteEmailInput{
		ToEmail: "optout@example.com", HostName: "Alex",
	}); err != nil {
		t.Fatalf("SendInviteEmail: %v", err)
	}
	if len(mock.Notifications) != 0 {
		t.Errorf("expected no send for a suppressed address, got %d", len(mock.Notifications))
	}
}

func TestInviteSender_SuppressionErrorPropagates(t *testing.T) {
	mock := &MockEmailService{}
	sentinel := errors.New("db down")
	sender := NewInviteSender(mock, "https://example.com", []byte("secret"),
		func(_ context.Context, _ string) (bool, error) { return false, sentinel })

	err := sender.SendInviteEmail(context.Background(), InviteEmailInput{ToEmail: "a@b.com", HostName: "A"})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected the suppression-check error to propagate, got %v", err)
	}
	if len(mock.Notifications) != 0 {
		t.Errorf("expected no send when the suppression check fails, got %d", len(mock.Notifications))
	}
}

func TestInviteSender_OmitsUnsubscribeWithoutSecret(t *testing.T) {
	mock := &MockEmailService{}
	sender := NewInviteSender(mock, "https://example.com", nil, nil)

	if err := sender.SendInviteEmail(context.Background(), InviteEmailInput{
		ToEmail: "x@y.com", HostName: "A",
	}); err != nil {
		t.Fatalf("SendInviteEmail: %v", err)
	}
	if mock.Notifications[0].UnsubscribeURL != "" {
		t.Errorf("expected no unsubscribe URL without a signing secret, got %q", mock.Notifications[0].UnsubscribeURL)
	}
}
