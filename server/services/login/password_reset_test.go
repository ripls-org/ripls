package login

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/email"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestPasswordReset_RequestPasswordReset(t *testing.T) {
	service, sqlStorage, _, _ := setupTestService(t)
	ctx := context.Background()

	// Create a test user with email/password auth
	user := &models.User{
		Id:           uuid.New().String(),
		Email:        "testuser@example.com",
		Name:         "Test User",
		Role:         models.Role_ROLE_USER,
		AuthMethod:   models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD,
		PasswordHash: "hashed_password",
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
	}
	_, err := sqlStorage.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Request password reset
	req := connect.NewRequest(&api.RequestPasswordResetRequest{
		Email: "testuser@example.com",
	})

	resp, err := service.RequestPasswordReset(ctx, req)
	if err != nil {
		t.Fatalf("RequestPasswordReset failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Response is nil")
	}

	// Verify that a reset request was created
	resetRequests, err := sqlStorage.QueryByField(ctx, "user_id", user.Id, &models.PendingPasswordReset{})
	if err != nil {
		t.Fatalf("Failed to query reset requests: %v", err)
	}

	if len(resetRequests) != 1 {
		t.Fatalf("Expected 1 reset request, got %d", len(resetRequests))
	}

	resetReq := resetRequests[0].(*models.PendingPasswordReset)
	if resetReq.Token == "" {
		t.Error("Reset token should not be empty")
	}

	// Verify expiration is set to ~1 hour from now
	expectedExpiry := time.Now().Unix() + 3600
	if resetReq.ExpiresAtUnixSec < expectedExpiry-10 || resetReq.ExpiresAtUnixSec > expectedExpiry+10 {
		t.Errorf("Unexpected expiry time: %d", resetReq.ExpiresAtUnixSec)
	}

	// Verify email was sent
	mockEmail := service.emailService.(*email.MockEmailService)
	if len(mockEmail.ResetEmails) != 1 {
		t.Fatalf("Expected 1 email sent, got %d", len(mockEmail.ResetEmails))
	}

	sentEmail := mockEmail.ResetEmails[0]
	if sentEmail.ToEmail != "testuser@example.com" {
		t.Errorf("Email sent to wrong address: %s", sentEmail.ToEmail)
	}
}

// TestPasswordReset_ThreadsPreferredLanguageToEmail confirms that the
// recipient User's stored preferred_language flows into the email send
// — without it, every reset email would render in the default locale
// regardless of what the user picked in their profile.
func TestPasswordReset_ThreadsPreferredLanguageToEmail(t *testing.T) {
	service, sqlStorage, _, _ := setupTestService(t)
	ctx := context.Background()

	es := "es"
	user := &models.User{
		Id:                uuid.New().String(),
		Email:             "es-user@example.com",
		Name:              "Spanish User",
		Role:              models.Role_ROLE_USER,
		AuthMethod:        models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD,
		PasswordHash:      "hashed_password",
		PreferredLanguage: &es,
		CreatedAt:         time.Now().Unix(),
		UpdatedAt:         time.Now().Unix(),
	}
	if _, err := sqlStorage.Insert(ctx, user); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	req := connect.NewRequest(&api.RequestPasswordResetRequest{
		Email: "es-user@example.com",
	})
	if _, err := service.RequestPasswordReset(ctx, req); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}

	mockEmail := service.emailService.(*email.MockEmailService)
	if len(mockEmail.ResetEmails) != 1 {
		t.Fatalf("expected 1 reset email, got %d", len(mockEmail.ResetEmails))
	}
	got := mockEmail.ResetEmails[0]
	if got.PreferredLanguage != "es" {
		t.Errorf("reset email PreferredLanguage = %q; want %q", got.PreferredLanguage, "es")
	}
}

func TestPasswordReset_RequestForNonExistentUser(t *testing.T) {
	service, _, _, _ := setupTestService(t)
	ctx := context.Background()

	// Request password reset for non-existent user
	req := connect.NewRequest(&api.RequestPasswordResetRequest{
		Email: "nonexistent@example.com",
	})

	resp, err := service.RequestPasswordReset(ctx, req)
	if err != nil {
		t.Fatalf("RequestPasswordReset should succeed even for non-existent users: %v", err)
	}

	if resp == nil {
		t.Fatal("Response is nil")
	}

	// Verify no email was sent (for security, we return success but don't send email)
	mockEmail := service.emailService.(*email.MockEmailService)
	if len(mockEmail.ResetEmails) != 0 {
		t.Errorf("Expected 0 emails sent for non-existent user, got %d", len(mockEmail.ResetEmails))
	}
}

func TestPasswordReset_CheckResetPasswordToken(t *testing.T) {
	service, sqlStorage, _, _ := setupTestService(t)
	ctx := context.Background()

	// Create a test user
	user := &models.User{
		Id:           uuid.New().String(),
		Email:        "testuser@example.com",
		Name:         "Test User",
		Role:         models.Role_ROLE_USER,
		AuthMethod:   models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD,
		PasswordHash: "hashed_password",
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
	}
	_, err := sqlStorage.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create a valid reset request
	token := uuid.New().String()
	resetReq := &models.PendingPasswordReset{
		Id:               uuid.New().String(),
		UserId:           user.Id,
		Token:            token,
		CreatedAtUnixSec: time.Now().Unix(),
		ExpiresAtUnixSec: time.Now().Unix() + 3600, // 1 hour from now
	}
	_, err = sqlStorage.Insert(ctx, resetReq)
	if err != nil {
		t.Fatalf("Failed to create reset request: %v", err)
	}

	// Check valid token
	req := connect.NewRequest(&api.CheckResetPasswordTokenRequest{
		Token: token,
	})

	resp, err := service.CheckResetPasswordToken(ctx, req)
	if err != nil {
		t.Fatalf("CheckResetPasswordToken failed: %v", err)
	}

	if !resp.Msg.IsValid {
		t.Error("Token should be valid")
	}

	if resp.Msg.Email != "testuser@example.com" {
		t.Errorf("Expected email testuser@example.com, got %s", resp.Msg.Email)
	}
}

func TestPasswordReset_CheckExpiredToken(t *testing.T) {
	service, sqlStorage, _, _ := setupTestService(t)
	ctx := context.Background()

	// Create a test user
	user := &models.User{
		Id:           uuid.New().String(),
		Email:        "testuser@example.com",
		Name:         "Test User",
		Role:         models.Role_ROLE_USER,
		AuthMethod:   models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD,
		PasswordHash: "hashed_password",
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
	}
	_, err := sqlStorage.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create an expired reset request
	token := uuid.New().String()
	resetReq := &models.PendingPasswordReset{
		Id:               uuid.New().String(),
		UserId:           user.Id,
		Token:            token,
		CreatedAtUnixSec: time.Now().Unix() - 7200, // 2 hours ago
		ExpiresAtUnixSec: time.Now().Unix() - 3600, // Expired 1 hour ago
	}
	_, err = sqlStorage.Insert(ctx, resetReq)
	if err != nil {
		t.Fatalf("Failed to create reset request: %v", err)
	}

	// Check expired token
	req := connect.NewRequest(&api.CheckResetPasswordTokenRequest{
		Token: token,
	})

	resp, err := service.CheckResetPasswordToken(ctx, req)
	if err != nil {
		t.Fatalf("CheckResetPasswordToken failed: %v", err)
	}

	if resp.Msg.IsValid {
		t.Error("Expired token should not be valid")
	}

	if resp.Msg.ErrorMessage == "" {
		t.Error("Error message should be provided for expired token")
	}
}

func TestPasswordReset_ResetPassword(t *testing.T) {
	service, sqlStorage, _, _ := setupTestService(t)
	ctx := context.Background()

	// Create a test user
	oldPasswordHash := "old_hashed_password"
	user := &models.User{
		Id:           uuid.New().String(),
		Email:        "testuser@example.com",
		Name:         "Test User",
		Role:         models.Role_ROLE_USER,
		AuthMethod:   models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD,
		PasswordHash: oldPasswordHash,
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
	}
	_, err := sqlStorage.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create a valid reset request
	token := uuid.New().String()
	resetReq := &models.PendingPasswordReset{
		Id:               uuid.New().String(),
		UserId:           user.Id,
		Token:            token,
		CreatedAtUnixSec: time.Now().Unix(),
		ExpiresAtUnixSec: time.Now().Unix() + 3600, // 1 hour from now
	}
	_, err = sqlStorage.Insert(ctx, resetReq)
	if err != nil {
		t.Fatalf("Failed to create reset request: %v", err)
	}

	// Reset password
	newPassword := "newSecurePassword123!"
	req := connect.NewRequest(&api.ResetPasswordRequest{
		Token:       token,
		NewPassword: newPassword,
	})

	resp, err := service.ResetPassword(ctx, req)
	if err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Response is nil")
	}

	// Verify password was updated
	updatedUser := &models.User{}
	err = sqlStorage.GetByID(ctx, user.Id, updatedUser)
	if err != nil {
		t.Fatalf("Failed to get updated user: %v", err)
	}

	if updatedUser.PasswordHash == oldPasswordHash {
		t.Error("Password hash should have been updated")
	}

	// Verify the new password hash is valid
	err = auth.VerifyPassword(newPassword, updatedUser.PasswordHash)
	if err != nil {
		t.Errorf("New password should be verifiable: %v", err)
	}

	// Verify reset request was deleted
	resetRequests, err := sqlStorage.QueryByField(ctx, "token", token, &models.PendingPasswordReset{})
	if err != nil {
		t.Fatalf("Failed to query reset requests: %v", err)
	}

	if len(resetRequests) != 0 {
		t.Error("Reset request should have been deleted after successful password reset")
	}
}

func TestPasswordReset_ResetWithExpiredToken(t *testing.T) {
	service, sqlStorage, _, _ := setupTestService(t)
	ctx := context.Background()

	// Create a test user
	user := &models.User{
		Id:           uuid.New().String(),
		Email:        "testuser@example.com",
		Name:         "Test User",
		Role:         models.Role_ROLE_USER,
		AuthMethod:   models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD,
		PasswordHash: "old_hashed_password",
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
	}
	_, err := sqlStorage.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create an expired reset request
	token := uuid.New().String()
	resetReq := &models.PendingPasswordReset{
		Id:               uuid.New().String(),
		UserId:           user.Id,
		Token:            token,
		CreatedAtUnixSec: time.Now().Unix() - 7200, // 2 hours ago
		ExpiresAtUnixSec: time.Now().Unix() - 3600, // Expired 1 hour ago
	}
	_, err = sqlStorage.Insert(ctx, resetReq)
	if err != nil {
		t.Fatalf("Failed to create reset request: %v", err)
	}

	// Try to reset password with expired token
	req := connect.NewRequest(&api.ResetPasswordRequest{
		Token:       token,
		NewPassword: "newPassword123!",
	})

	_, err = service.ResetPassword(ctx, req)
	if err == nil {
		t.Fatal("ResetPassword should fail with expired token")
	}

	// Verify error is about expired token
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("Error should mention expired token, got: %v", err)
	}
}
