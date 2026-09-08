package auth

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"

	api "go.ripls.org/ripls/server/gen/ripls/api"
)

func TestNewOIDCProviderManager(t *testing.T) {
	manager := NewOIDCProviderManager()

	if manager == nil {
		t.Fatal("Expected manager to be created")
	}

	if manager.providers == nil {
		t.Error("Expected providers map to be initialized")
	}

	if len(manager.providers) != 0 {
		t.Errorf("Expected empty providers map, got %d providers", len(manager.providers))
	}
}

func TestOIDCProviderManager_RegisterGoogleProvider(t *testing.T) {
	manager := NewOIDCProviderManager()

	t.Run("successful registration", func(t *testing.T) {
		clientID := "test-client-id.apps.googleusercontent.com"

		// Create mock OIDC setup
		setup := SetupMockGoogleOIDC(t, clientID)

		// Verify provider was registered
		config, exists := setup.Manager.providers[api.OIDCProvider_OIDC_PROVIDER_GOOGLE]
		if !exists {
			t.Fatal("Google provider not found in providers map")
		}

		if config.ClientID != clientID {
			t.Errorf("Expected client ID %s, got %s", clientID, config.ClientID)
		}

		if config.Verifier == nil {
			t.Error("Expected verifier to be initialized")
		}
	})

	t.Run("trims whitespace from client ID", func(t *testing.T) {
		// Secret Manager values occasionally arrive with a trailing newline,
		// which historically broke audience verification at runtime.
		trimmedID := "test-client-id.apps.googleusercontent.com"
		setup := SetupMockGoogleOIDC(t, trimmedID+"\n")

		config, exists := setup.Manager.providers[api.OIDCProvider_OIDC_PROVIDER_GOOGLE]
		if !exists {
			t.Fatal("Google provider not found in providers map")
		}

		if config.ClientID != trimmedID {
			t.Errorf("Expected trimmed client ID %q, got %q", trimmedID, config.ClientID)
		}
	})

	t.Run("invalid discovery response", func(t *testing.T) {
		// Create context with invalid JSON response
		responses := map[string]string{
			"https://accounts.google.com/.well-known/openid_configuration": "invalid-json",
		}
		client := &http.Client{
			Transport: &mockTransport{responses: responses},
		}
		ctx := oidc.ClientContext(context.Background(), client)

		err := manager.RegisterGoogleProvider(ctx, "test-client-id")
		if err == nil {
			t.Fatal("Expected error due to invalid discovery response")
		}

		// Should contain "failed to create" in the error message
		if !strings.Contains(err.Error(), "failed to create") {
			t.Errorf("Expected error to contain 'failed to create', got: %v", err)
		}
	})
}

func TestOIDCProviderManager_ValidateToken(t *testing.T) {
	t.Run("provider not registered", func(t *testing.T) {
		manager := NewOIDCProviderManager()
		ctx := context.Background()
		_, err := manager.ValidateToken(ctx, api.OIDCProvider_OIDC_PROVIDER_GOOGLE, "fake-token")
		if err == nil {
			t.Fatal("Expected error for unregistered provider")
		}

		expectedError := "provider OIDC_PROVIDER_GOOGLE not registered"
		if err.Error() != expectedError {
			t.Errorf("Expected error %q, got %q", expectedError, err.Error())
		}
	})

	t.Run("successful token validation", func(t *testing.T) {
		clientID := "test-client-id.apps.googleusercontent.com"

		// Create mock OIDC setup
		setup := SetupMockGoogleOIDC(t, clientID)

		// Create a valid test token
		testToken := setup.CreateValidGoogleToken(t, "test-user-123", "test@example.com", "Test User")

		// Validate the token
		idToken, err := setup.Manager.ValidateToken(setup.Context, api.OIDCProvider_OIDC_PROVIDER_GOOGLE, testToken)
		if err != nil {
			t.Fatalf("Failed to validate token: %v", err)
		}

		if idToken == nil {
			t.Fatal("Expected non-nil ID token")
		}

		// Verify token claims
		if idToken.Subject != "test-user-123" {
			t.Errorf("Expected subject test-user-123, got %s", idToken.Subject)
		}

		if idToken.Issuer != "https://accounts.google.com" {
			t.Errorf("Expected issuer https://accounts.google.com, got %s", idToken.Issuer)
		}

		// Verify audience
		expectedAudience := []string{clientID}
		if len(idToken.Audience) != 1 || idToken.Audience[0] != clientID {
			t.Errorf("Expected audience %v, got %v", expectedAudience, idToken.Audience)
		}
	})

	t.Run("invalid token format", func(t *testing.T) {
		clientID := "test-client-id.apps.googleusercontent.com"

		// Create mock OIDC setup
		setup := SetupMockGoogleOIDC(t, clientID)

		_, err := setup.Manager.ValidateToken(setup.Context, api.OIDCProvider_OIDC_PROVIDER_GOOGLE, "invalid-token")
		if err == nil {
			t.Fatal("Expected error for invalid token format")
		}

		if !strings.Contains(err.Error(), "failed to verify") {
			t.Errorf("Expected error to contain 'failed to verify', got: %v", err)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		clientID := "test-client-id.apps.googleusercontent.com"

		// Create mock OIDC setup
		setup := SetupMockGoogleOIDC(t, clientID)

		// Create an expired token
		expiredToken := setup.CreateExpiredToken(t, "https://accounts.google.com", "test-user-123")

		_, err := setup.Manager.ValidateToken(setup.Context, api.OIDCProvider_OIDC_PROVIDER_GOOGLE, expiredToken)
		if err == nil {
			t.Fatal("Expected error for expired token")
		}

		if !strings.Contains(err.Error(), "failed to verify") {
			t.Errorf("Expected error to contain 'failed to verify', got: %v", err)
		}
	})

	t.Run("wrong audience", func(t *testing.T) {
		clientID := "test-client-id.apps.googleusercontent.com"

		// Create mock OIDC setup
		setup := SetupMockGoogleOIDC(t, clientID)

		// Create token with wrong audience
		wrongAudienceToken := setup.CreateWrongAudienceToken(t, "https://accounts.google.com", "test-user-123")

		_, err := setup.Manager.ValidateToken(setup.Context, api.OIDCProvider_OIDC_PROVIDER_GOOGLE, wrongAudienceToken)
		if err == nil {
			t.Fatal("Expected error for wrong audience")
		}

		if !strings.Contains(err.Error(), "failed to verify") {
			t.Errorf("Expected error to contain 'failed to verify', got: %v", err)
		}
	})
}

func TestOIDCProviderManager_SupportedProviders(t *testing.T) {
	t.Run("no providers registered", func(t *testing.T) {
		manager := NewOIDCProviderManager()
		providers := manager.SupportedProviders()
		if len(providers) != 0 {
			t.Errorf("Expected 0 providers, got %d", len(providers))
		}
	})

	t.Run("single provider registered", func(t *testing.T) {
		// Create mock OIDC setup
		setup := SetupMockGoogleOIDC(t, "test-client-id.apps.googleusercontent.com")

		providers := setup.Manager.SupportedProviders()
		if len(providers) != 1 {
			t.Errorf("Expected 1 provider, got %d", len(providers))
		}

		if providers[0] != api.OIDCProvider_OIDC_PROVIDER_GOOGLE {
			t.Errorf("Expected Google provider, got %v", providers[0])
		}
	})
}

func TestExtractUserInfo(t *testing.T) {
	t.Run("nil token", func(t *testing.T) {
		_, err := ExtractUserInfo(nil)
		if err == nil {
			t.Fatal("Expected error for nil token")
		}
	})

	t.Run("successful extraction", func(t *testing.T) {
		clientID := "test-client-id.apps.googleusercontent.com"

		// Create mock OIDC setup
		setup := SetupMockGoogleOIDC(t, clientID)

		// Create a valid test token with user info
		testToken := setup.CreateValidGoogleToken(t, "test-user-123", "test@example.com", "Test User")

		// Validate the token to get an IDToken
		idToken, err := setup.Manager.ValidateToken(setup.Context, api.OIDCProvider_OIDC_PROVIDER_GOOGLE, testToken)
		if err != nil {
			t.Fatalf("Failed to validate token: %v", err)
		}

		// Extract user info
		userInfo, err := ExtractUserInfo(idToken)
		if err != nil {
			t.Fatalf("Failed to extract user info: %v", err)
		}

		// Verify extracted user info
		if userInfo.Subject != "test-user-123" {
			t.Errorf("Expected subject test-user-123, got %s", userInfo.Subject)
		}

		if userInfo.Email != "test@example.com" {
			t.Errorf("Expected email test@example.com, got %s", userInfo.Email)
		}

		if !userInfo.EmailVerified {
			t.Error("Expected email to be verified")
		}

		if userInfo.Name != "Test User" {
			t.Errorf("Expected name 'Test User', got %s", userInfo.Name)
		}

		if userInfo.Picture != "https://example.com/photo.jpg" {
			t.Errorf("Expected picture URL 'https://example.com/photo.jpg', got %s", userInfo.Picture)
		}
	})

	t.Run("missing claims", func(t *testing.T) {
		clientID := "test-client-id.apps.googleusercontent.com"

		// Create mock OIDC setup
		setup := SetupMockGoogleOIDC(t, clientID)

		// Create a token with minimal claims (missing email, name, etc.)
		minimalClaims := CreateStandardGoogleClaims(clientID, "test-user-123", "", "")
		// Remove optional fields
		delete(minimalClaims, "email")
		delete(minimalClaims, "email_verified")
		delete(minimalClaims, "name")
		delete(minimalClaims, "picture")

		testToken := GenerateTestIDToken(t, setup.KeyPair, minimalClaims)

		// Validate the token
		idToken, err := setup.Manager.ValidateToken(setup.Context, api.OIDCProvider_OIDC_PROVIDER_GOOGLE, testToken)
		if err != nil {
			t.Fatalf("Failed to validate token: %v", err)
		}

		// Extract user info (should work with missing optional claims)
		userInfo, err := ExtractUserInfo(idToken)
		if err != nil {
			t.Fatalf("Failed to extract user info: %v", err)
		}

		// Verify that required subject is present and optional fields are empty
		if userInfo.Subject != "test-user-123" {
			t.Errorf("Expected subject test-user-123, got %s", userInfo.Subject)
		}

		if userInfo.Email != "" {
			t.Errorf("Expected empty email, got %s", userInfo.Email)
		}

		if userInfo.EmailVerified {
			t.Error("Expected email_verified to be false by default")
		}

		if userInfo.Name != "" {
			t.Errorf("Expected empty name, got %s", userInfo.Name)
		}

		if userInfo.Picture != "" {
			t.Errorf("Expected empty picture, got %s", userInfo.Picture)
		}
	})
}
