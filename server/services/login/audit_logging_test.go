package login

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// loggedContext returns a context with a JSON logger writing to buf,
// and a helper that parses all log lines emitted after the call.
func loggedContext(buf *bytes.Buffer) context.Context {
	logger := logging.NewLogger(logging.Options{Format: "json", Output: buf})
	return logging.WithLogger(context.Background(), logger)
}

// parseWarnLines returns all log entries at severity WARNING from buf.
func parseWarnLines(buf *bytes.Buffer) []map[string]any {
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["severity"] == "WARNING" {
			out = append(out, entry)
		}
	}
	return out
}

// assertWarnField fails the test if the field is absent or empty in every warn log entry.
func assertWarnField(t *testing.T, entries []map[string]any, field string) {
	t.Helper()
	for _, e := range entries {
		if v, ok := e[field]; ok && v != "" {
			return
		}
	}
	t.Errorf("expected warn log to contain non-empty field %q; entries: %v", field, entries)
}

// TestAuditLog_EmailLogin_UnknownEmail verifies a warn log is emitted when a login
// attempt uses an email address that has no corresponding account.
func TestAuditLog_EmailLogin_UnknownEmail(t *testing.T) {
	service, _, _, _ := setupTestService(t)

	var buf bytes.Buffer
	ctx := loggedContext(&buf)

	_, err := service.EmailLogin(ctx, connect.NewRequest(&api.EmailLoginRequest{
		Email:    "nobody@example.com",
		Password: "password",
	}))
	if err == nil {
		t.Fatal("expected error for unknown email")
	}

	entries := parseWarnLines(&buf)
	if len(entries) == 0 {
		t.Fatal("expected at least one warn log entry for unknown email login")
	}
	assertWarnField(t, entries, "user_email")
	assertWarnField(t, entries, "reason")
}

// TestAuditLog_EmailLogin_WrongPassword verifies a warn log is emitted when a login
// attempt uses a valid email but incorrect password.
func TestAuditLog_EmailLogin_WrongPassword(t *testing.T) {
	service, sqlStorage, _, _ := setupTestService(t)
	ctx := context.Background()

	// A legacy password account, written directly: a wrong-password sign-in is
	// only reachable for accounts that hold one, and registration no longer
	// creates them (#2864).
	createLegacyPasswordUser(t, ctx, sqlStorage, "audit-user@example.com", "Audit User", "correct-password")

	var buf bytes.Buffer
	logCtx := loggedContext(&buf)

	_, err := service.EmailLogin(logCtx, connect.NewRequest(&api.EmailLoginRequest{
		Email:    "audit-user@example.com",
		Password: "wrong-password",
	}))
	if err == nil {
		t.Fatal("expected error for wrong password")
	}

	entries := parseWarnLines(&buf)
	if len(entries) == 0 {
		t.Fatal("expected at least one warn log entry for wrong-password login")
	}
	assertWarnField(t, entries, "user_email")
	assertWarnField(t, entries, "reason")
}

// TestAuditLog_OIDCLogin_TokenValidationFailed verifies a warn log is emitted when
// OIDC token validation fails.
func TestAuditLog_OIDCLogin_TokenValidationFailed(t *testing.T) {
	service, _, oidcSetup := createServiceWithMockOIDC(t)

	var buf bytes.Buffer
	ctx := loggedContext(&buf)

	_, err := service.OIDCLogin(ctx, connect.NewRequest(&api.OIDCLoginRequest{
		Provider: api.OIDCProvider_OIDC_PROVIDER_GOOGLE,
		IdToken:  "invalid-token",
	}))
	if err == nil {
		t.Fatal("expected error for invalid OIDC token")
	}
	_ = oidcSetup // used to set up the mock

	entries := parseWarnLines(&buf)
	if len(entries) == 0 {
		t.Fatal("expected at least one warn log entry for OIDC token validation failure")
	}
	assertWarnField(t, entries, "reason")
}

// TestAuditLog_OIDCLogin_UnknownEmail verifies a warn log is emitted when OIDC token
// is valid but no account exists for the email.
func TestAuditLog_OIDCLogin_UnknownEmail(t *testing.T) {
	service, _, oidcSetup := createServiceWithMockOIDC(t)

	var buf bytes.Buffer
	ctx := loggedContext(&buf)

	testToken := oidcSetup.CreateValidGoogleToken(t, "new-sub-123", "no-account@example.com", "New User")

	_, err := service.OIDCLogin(ctx, connect.NewRequest(&api.OIDCLoginRequest{
		Provider: api.OIDCProvider_OIDC_PROVIDER_GOOGLE,
		IdToken:  testToken,
	}))
	if err == nil {
		t.Fatal("expected error for unknown OIDC user")
	}

	entries := parseWarnLines(&buf)
	if len(entries) == 0 {
		t.Fatal("expected at least one warn log entry for unknown OIDC email")
	}
	assertWarnField(t, entries, "user_email")
	assertWarnField(t, entries, "reason")
}

// TestAuditLog_PhoneLogin_TokenFailed verifies a warn log is emitted when the Firebase
// phone token cannot be verified.
func TestAuditLog_PhoneLogin_TokenFailed(t *testing.T) {
	mockPhone := &mockPhoneTokenVerifier{err: fmt.Errorf("firebase token invalid")}
	service, _, _ := setupPhoneTestService(t, mockPhone)

	var buf bytes.Buffer
	ctx := loggedContext(&buf)

	_, err := service.PhoneLogin(ctx, connect.NewRequest(&api.PhoneLoginRequest{
		FirebaseIdToken: "bad-token",
	}))
	if err == nil {
		t.Fatal("expected error for invalid phone token")
	}

	entries := parseWarnLines(&buf)
	if len(entries) == 0 {
		t.Fatal("expected at least one warn log entry for phone token failure")
	}
	assertWarnField(t, entries, "reason")
}

// TestAuditLog_PhoneLogin_UnknownPhone verifies a warn log is emitted when the Firebase
// token is valid but no account exists for the resolved phone number.
func TestAuditLog_PhoneLogin_UnknownPhone(t *testing.T) {
	mockPhone := &mockPhoneTokenVerifier{phoneNumber: "+15550001111"}
	service, _, _ := setupPhoneTestService(t, mockPhone)

	var buf bytes.Buffer
	ctx := loggedContext(&buf)

	_, err := service.PhoneLogin(ctx, connect.NewRequest(&api.PhoneLoginRequest{
		FirebaseIdToken: "valid-token",
	}))
	if err == nil {
		t.Fatal("expected error for unknown phone number")
	}

	entries := parseWarnLines(&buf)
	if len(entries) == 0 {
		t.Fatal("expected at least one warn log entry for unknown phone number")
	}
	assertWarnField(t, entries, "reason")

	// Phone numbers must NOT appear in logs (PII).
	for _, e := range entries {
		for _, v := range e {
			if s, ok := v.(string); ok && strings.Contains(s, "+15550001111") {
				t.Errorf("phone number found in log entry — PII must not be logged: %v", e)
			}
		}
	}
}

// TestAuditLog_RefreshToken_Unknown verifies a warn log is emitted when the submitted
// refresh token does not match any stored token.
func TestAuditLog_RefreshToken_Unknown(t *testing.T) {
	service, _, _, _ := setupTestService(t)

	var buf bytes.Buffer
	ctx := loggedContext(&buf)

	_, err := service.RefreshToken(ctx, connect.NewRequest(&api.RefreshTokenRequest{
		RefreshToken: "unknown-token-value",
	}))
	if err == nil {
		t.Fatal("expected error for unknown refresh token")
	}

	entries := parseWarnLines(&buf)
	if len(entries) == 0 {
		t.Fatal("expected at least one warn log entry for unknown refresh token")
	}
	assertWarnField(t, entries, "reason")
}

// TestAuditLog_RefreshToken_Expired verifies a warn log is emitted when a known but
// expired refresh token is submitted.
func TestAuditLog_RefreshToken_Expired(t *testing.T) {
	service, sqlStorage, _, _ := setupTestService(t)
	ctx := context.Background()

	// Insert a user and an already-expired refresh token directly.
	now := time.Now().Unix()
	user := &models.User{
		Id:         uuid.New().String(),
		Email:      "expired-rt@example.com",
		Name:       "Expired RT User",
		CreatedAt:  now,
		UpdatedAt:  now,
		Role:       models.Role_ROLE_USER,
		AuthMethod: models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD,
	}
	if _, err := sqlStorage.Insert(ctx, user); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	rawToken, err := auth.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	stored := &models.RefreshToken{
		Id:               uuid.New().String(),
		UserId:           user.Id,
		TokenHash:        auth.HashRefreshToken(rawToken),
		ExpiresAtUnixSec: now - 3600, // already expired
		CreatedAtUnixSec: now - 7200,
		IsRevoked:        false,
	}
	if _, err := sqlStorage.Insert(ctx, stored); err != nil {
		t.Fatalf("insert expired token: %v", err)
	}

	var buf bytes.Buffer
	logCtx := loggedContext(&buf)

	_, err = service.RefreshToken(logCtx, connect.NewRequest(&api.RefreshTokenRequest{
		RefreshToken: rawToken,
	}))
	if err == nil {
		t.Fatal("expected error for expired refresh token")
	}

	entries := parseWarnLines(&buf)
	if len(entries) == 0 {
		t.Fatal("expected at least one warn log entry for expired refresh token")
	}
	assertWarnField(t, entries, "reason")
}
