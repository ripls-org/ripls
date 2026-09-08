package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
)

// This module abstracts verification of OIDC authentication tokens from
// various providers like Google.

// ProviderConfig holds configuration for a specific OIDC provider.
type ProviderConfig struct {
	ClientID string
	Verifier *oidc.IDTokenVerifier
}

// OIDCProviderManager manages OIDC providers and token validation.
type OIDCProviderManager struct {
	providers map[api.OIDCProvider]*ProviderConfig
}

// NewOIDCProviderManager creates a new OIDC provider manager.
func NewOIDCProviderManager() *OIDCProviderManager {
	return &OIDCProviderManager{
		providers: make(map[api.OIDCProvider]*ProviderConfig),
	}
}

// RegisterGoogleProvider registers Google as an OIDC provider.
func (m *OIDCProviderManager) RegisterGoogleProvider(ctx context.Context, clientID string) error {
	// Trim whitespace because Secret Manager uploads occasionally include a
	// trailing newline, which causes audience mismatch on every Verify() call.
	clientID = strings.TrimSpace(clientID)

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RegisterGoogleProvider",
		"auth_method", "oidc",
		"provider", "google",
	)

	provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		logger.ErrorContext(ctx, "failed to create OIDC provider",
			"error", err,
		)
		return fmt.Errorf("failed to create Google OIDC provider: %w", err)
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: clientID})
	m.providers[api.OIDCProvider_OIDC_PROVIDER_GOOGLE] = &ProviderConfig{
		ClientID: clientID,
		Verifier: verifier,
	}

	logger.InfoContext(ctx, "OIDC provider registered successfully")
	return nil
}

// ValidateToken validates an ID token from the specified provider.
func (m *OIDCProviderManager) ValidateToken(ctx context.Context, provider api.OIDCProvider, idToken string) (*oidc.IDToken, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ValidateToken",
		"auth_method", "oidc",
		"provider", provider.String(),
		"token", logging.MaskToken(idToken),
	)

	providerConfig, exists := m.providers[provider]
	if !exists {
		logger.WarnContext(ctx, "provider not registered",
			"token_type", "id_token",
		)
		return nil, fmt.Errorf("provider %v not registered", provider)
	}

	token, err := providerConfig.Verifier.Verify(ctx, idToken)
	if err != nil {
		logger.WarnContext(ctx, "token verification failed",
			"error", err,
			"token_type", "id_token",
		)
		return nil, fmt.Errorf("failed to verify %v token: %w", provider, err)
	}

	logger.DebugContext(ctx, "token verified successfully",
		"token_type", "id_token",
		"subject", token.Subject,
	)
	return token, nil
}

// ExtractUserInfo extracts user information from a validated ID token.
func ExtractUserInfo(token *oidc.IDToken) (*UserInfo, error) {
	ctx := context.Background()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ExtractUserInfo",
		"auth_method", "oidc",
	)

	if token == nil {
		logger.ErrorContext(ctx, "token is nil")
		return nil, fmt.Errorf("token cannot be nil")
	}

	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}

	if err := token.Claims(&claims); err != nil {
		logger.ErrorContext(ctx, "failed to extract claims from token",
			"error", err,
			"subject", token.Subject,
		)
		return nil, fmt.Errorf("failed to extract claims: %w", err)
	}

	logger.DebugContext(ctx, "user info extracted successfully",
		"subject", token.Subject,
		"user_email", logging.MaskEmail(claims.Email),
		"email_verified", claims.EmailVerified,
	)

	return &UserInfo{
		Subject:       token.Subject,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
		Name:          claims.Name,
		Picture:       claims.Picture,
	}, nil
}

// UserInfo represents user information extracted from OIDC claims.
type UserInfo struct {
	Subject       string // The user's unique identifier from the provider
	Email         string // The user's email address
	EmailVerified bool   // Whether the email has been verified
	Name          string // The user's display name
	Picture       string // URL to the user's profile picture
}

// SupportedProviders returns a list of registered OIDC providers.
func (m *OIDCProviderManager) SupportedProviders() []api.OIDCProvider {
	providers := make([]api.OIDCProvider, 0, len(m.providers))
	for provider := range m.providers {
		providers = append(providers, provider)
	}
	return providers
}
