package login

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestRefreshToken_HappyPath(t *testing.T) {
	service, sqlStorage, userManager, _ := setupTestService(t)
	ctx := context.Background()

	inviter, err := userManager.CreateUser(ctx, "inviter-refresh@example.com", "Inviter", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("Failed to create inviter: %v", err)
	}
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

	registerReq := connect.NewRequest(&api.EmailRegisterRequest{
		Email:           "refresh-user@example.com",
		Name:            "Refresh User",
		EmailProofToken: proveEmail(t, ctx, service, "refresh-user@example.com"),
		ShortCode:       invitation.ShortCode,
	})
	registerResp, err := service.EmailRegister(ctx, registerReq)
	if err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	if registerResp.Msg.Tokens == nil {
		t.Fatal("Expected tokens in register response")
	}
	refreshToken := registerResp.Msg.Tokens.RefreshToken
	if refreshToken == "" {
		t.Fatal("Expected refresh token in register response")
	}

	// Refresh the token.
	refreshReq := connect.NewRequest(&api.RefreshTokenRequest{
		RefreshToken: refreshToken,
	})
	refreshResp, err := service.RefreshToken(ctx, refreshReq)
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}

	if refreshResp.Msg.Tokens == nil || refreshResp.Msg.Tokens.AccessToken == "" {
		t.Error("Expected non-empty access token in refresh response")
	}
	if refreshResp.Msg.Tokens.RefreshToken == "" {
		t.Error("Expected non-empty refresh token in refresh response")
	}
	if refreshResp.Msg.Tokens.RefreshToken == refreshToken {
		t.Error("Expected rotated refresh token, got same token back")
	}
}

func TestRefreshToken_TokenRotation_OldTokenInvalid(t *testing.T) {
	service, sqlStorage, userManager, _ := setupTestService(t)
	ctx := context.Background()

	inviter, err := userManager.CreateUser(ctx, "inviter-rotate@example.com", "Inviter", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("Failed to create inviter: %v", err)
	}
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

	registerReq := connect.NewRequest(&api.EmailRegisterRequest{
		Email:           "rotate-user@example.com",
		Name:            "Rotate User",
		EmailProofToken: proveEmail(t, ctx, service, "rotate-user@example.com"),
		ShortCode:       invitation.ShortCode,
	})
	registerResp, err := service.EmailRegister(ctx, registerReq)
	if err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	originalRefreshToken := registerResp.Msg.Tokens.RefreshToken

	// Use the refresh token once.
	refreshReq := connect.NewRequest(&api.RefreshTokenRequest{RefreshToken: originalRefreshToken})
	_, err = service.RefreshToken(ctx, refreshReq)
	if err != nil {
		t.Fatalf("First refresh failed: %v", err)
	}

	// Reuse the old (now revoked) token — must be rejected.
	_, err = service.RefreshToken(ctx, refreshReq)
	if err == nil {
		t.Fatal("Expected error when reusing revoked token, got nil")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok || connectErr.Code() != connect.CodeUnauthenticated {
		t.Errorf("Expected unauthenticated error, got %v", err)
	}
}

func TestRefreshToken_RevokedTokenTheftDetection(t *testing.T) {
	service, sqlStorage, userManager, _ := setupTestService(t)
	ctx := context.Background()

	inviter, err := userManager.CreateUser(ctx, "inviter-theft@example.com", "Inviter", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("Failed to create inviter: %v", err)
	}
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

	registerReq := connect.NewRequest(&api.EmailRegisterRequest{
		Email:           "theft-user@example.com",
		Name:            "Theft User",
		EmailProofToken: proveEmail(t, ctx, service, "theft-user@example.com"),
		ShortCode:       invitation.ShortCode,
	})
	registerResp, err := service.EmailRegister(ctx, registerReq)
	if err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	originalToken := registerResp.Msg.Tokens.RefreshToken

	// Legitimate user refreshes once.
	firstRefresh := connect.NewRequest(&api.RefreshTokenRequest{RefreshToken: originalToken})
	firstResp, err := service.RefreshToken(ctx, firstRefresh)
	if err != nil {
		t.Fatalf("First refresh failed: %v", err)
	}
	newToken := firstResp.Msg.Tokens.RefreshToken

	// Attacker reuses original (now revoked) token — triggers theft detection.
	_, err = service.RefreshToken(ctx, firstRefresh)
	if err == nil {
		t.Fatal("Expected theft detection error, got nil")
	}

	// After theft detection, all tokens for this user must be revoked.
	// The new token from the first refresh must also be rejected.
	secondRefresh := connect.NewRequest(&api.RefreshTokenRequest{RefreshToken: newToken})
	_, err = service.RefreshToken(ctx, secondRefresh)
	if err == nil {
		t.Fatal("Expected all tokens revoked after theft detection, but new token still works")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok || connectErr.Code() != connect.CodeUnauthenticated {
		t.Errorf("Expected unauthenticated error after revocation, got %v", err)
	}
}

func TestRefreshToken_ExpiredToken(t *testing.T) {
	service, _, _, _ := setupTestService(t)
	ctx := context.Background()

	// Manually create an expired refresh token in storage.
	rawToken, err := auth.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("Failed to generate raw token: %v", err)
	}

	expired := &models.RefreshToken{
		Id:               "test-expired-id",
		UserId:           "some-user-id",
		TokenHash:        auth.HashRefreshToken(rawToken),
		ExpiresAtUnixSec: time.Now().Add(-1 * time.Hour).Unix(), // already expired
		CreatedAtUnixSec: time.Now().Add(-2 * time.Hour).Unix(),
		IsRevoked:        false,
	}
	if _, err := service.storage.Insert(ctx, expired); err != nil {
		t.Fatalf("Failed to insert expired token: %v", err)
	}

	req := connect.NewRequest(&api.RefreshTokenRequest{RefreshToken: rawToken})
	_, err = service.RefreshToken(ctx, req)
	if err == nil {
		t.Fatal("Expected error for expired token, got nil")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok || connectErr.Code() != connect.CodeUnauthenticated {
		t.Errorf("Expected unauthenticated error for expired token, got %v", err)
	}
}

func TestRefreshToken_InvalidToken(t *testing.T) {
	service, _, _, _ := setupTestService(t)
	ctx := context.Background()

	req := connect.NewRequest(&api.RefreshTokenRequest{RefreshToken: "not-a-real-token"})
	_, err := service.RefreshToken(ctx, req)
	if err == nil {
		t.Fatal("Expected error for invalid token, got nil")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok || connectErr.Code() != connect.CodeUnauthenticated {
		t.Errorf("Expected unauthenticated error, got %v", err)
	}
}

func TestRefreshToken_MissingToken(t *testing.T) {
	service, _, _, _ := setupTestService(t)
	ctx := context.Background()

	req := connect.NewRequest(&api.RefreshTokenRequest{})
	_, err := service.RefreshToken(ctx, req)
	if err == nil {
		t.Fatal("Expected error for missing token, got nil")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok || connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Expected invalid argument error, got %v", err)
	}
}

func TestRefreshToken_GeneratedOnLogin(t *testing.T) {
	service, sqlStorage, userManager, _ := setupTestService(t)
	ctx := context.Background()

	inviter, err := userManager.CreateUser(ctx, "inviter-login-rt@example.com", "Inviter", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("Failed to create inviter: %v", err)
	}
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

	registerReq := connect.NewRequest(&api.EmailRegisterRequest{
		Email:           "login-rt-user@example.com",
		Name:            "Login RT User",
		EmailProofToken: proveEmail(t, ctx, service, "login-rt-user@example.com"),
		ShortCode:       invitation.ShortCode,
	})
	if _, err := service.EmailRegister(ctx, registerReq); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	loginReq := connect.NewRequest(&api.EmailLoginRequest{
		Email:           "login-rt-user@example.com",
		EmailProofToken: proveEmail(t, ctx, service, "login-rt-user@example.com"),
	})
	loginResp, err := service.EmailLogin(ctx, loginReq)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if loginResp.Msg.Tokens == nil || loginResp.Msg.Tokens.RefreshToken == "" {
		t.Error("Expected refresh token in login response")
	}
	if loginResp.Msg.Tokens.AccessToken == "" {
		t.Error("Expected access token in login response")
	}
}

// TestRefreshToken_PhoneOnlyUserKeepsPhoneClaim is a regression test for
// #1450: refresh-issued access tokens for phone-only users (Email == "")
// must include phone_number in the claims, otherwise auth middleware
// rejects them on the next RPC and the client logs the user out.
func TestRefreshToken_PhoneOnlyUserKeepsPhoneClaim(t *testing.T) {
	service, sqlStorage, _, authTokenConfig := setupTestService(t)
	ctx := context.Background()

	// Create a phone-only user (no email) — mirrors the production
	// PhoneLogin flow's persisted shape.
	phone := "+15551234567"
	now := time.Now().Unix()
	user := &models.User{
		Id:          uuid.New().String(),
		Name:        "Phone-only User",
		PhoneNumber: &phone,
		CreatedAt:   now,
		UpdatedAt:   now,
		Role:        models.Role_ROLE_USER,
		AuthMethod:  models.AuthMethod_AUTH_METHOD_PHONE,
	}
	if _, err := sqlStorage.Insert(ctx, user); err != nil {
		t.Fatalf("Failed to create phone user: %v", err)
	}

	// Create a refresh token for the user directly via the service
	// helper (mirrors what PhoneLogin would have done).
	rawRefreshToken, err := service.createRefreshToken(ctx, user.Id, models.RefreshTokenOrigin_REFRESH_TOKEN_ORIGIN_INTERACTIVE)
	if err != nil {
		t.Fatalf("Failed to create refresh token: %v", err)
	}

	// Refresh the token.
	refreshResp, err := service.RefreshToken(ctx, connect.NewRequest(&api.RefreshTokenRequest{
		RefreshToken: rawRefreshToken,
	}))
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	newAccessToken := refreshResp.Msg.Tokens.AccessToken
	if newAccessToken == "" {
		t.Fatal("Expected non-empty access token in refresh response")
	}

	// Validate the new access token end-to-end. Token validation
	// requires the user_id claim plus at least one of email or
	// phone_number; for phone-only users the phone_number claim must
	// survive token rotation.
	claims, err := authTokenConfig.ValidateToken(newAccessToken)
	if err != nil {
		t.Fatalf("Refreshed access token failed validation: %v", err)
	}

	if claims["user_id"] != user.Id {
		t.Errorf("user_id claim mismatch: expected %q, got %v", user.Id, claims["user_id"])
	}
	if got, _ := claims["phone_number"].(string); got != phone {
		t.Errorf("phone_number claim mismatch: expected %q, got %q", phone, got)
	}
	// Email claim should be empty for phone-only user.
	if got, _ := claims["email"].(string); got != "" {
		t.Errorf("email claim should be empty for phone-only user, got %q", got)
	}
}
