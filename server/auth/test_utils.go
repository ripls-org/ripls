package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
)

// mockTransport implements http.RoundTripper for mocking HTTP responses.
type mockTransport struct {
	responses map[string]string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
	response, exists := m.responses[url]
	if !exists {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       http.NoBody,
		}, nil
	}

	header := make(http.Header)
	header.Set("Content-Type", "application/json")

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(response)),
		Header:     header,
	}, nil
}

// TestKeyPair holds a test RSA key pair and associated JWKS.
type TestKeyPair struct {
	PrivateKey *rsa.PrivateKey
	JWKS       string
	KeyID      string
}

// GenerateTestKeyAndJWKS creates a test RSA key pair and returns the private key and JWKS document.
func GenerateTestKeyAndJWKS(t *testing.T, keyID string) *TestKeyPair {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	publicKey := &privateKey.PublicKey

	// Convert public key to JWK format
	jwk := map[string]interface{}{
		"kty": "RSA",
		"kid": keyID,
		"use": "sig",
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(publicKey.E)).Bytes()),
	}

	jwks := map[string]interface{}{
		"keys": []interface{}{jwk},
	}

	jwksBytes, err := json.Marshal(jwks)
	if err != nil {
		t.Fatalf("Failed to marshal JWKS: %v", err)
	}

	return &TestKeyPair{
		PrivateKey: privateKey,
		JWKS:       string(jwksBytes),
		KeyID:      keyID,
	}
}

// GenerateTestIDToken creates a test ID token with the given claims.
func GenerateTestIDToken(t *testing.T, keyPair *TestKeyPair, claims map[string]interface{}) string {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims(claims))
	token.Header["kid"] = keyPair.KeyID

	tokenString, err := token.SignedString(keyPair.PrivateKey)
	if err != nil {
		t.Fatalf("Failed to sign token: %v", err)
	}

	return tokenString
}

// CreateMockOIDCContext creates a context with mocked HTTP responses for OIDC discovery and JWKS.
func CreateMockOIDCContext(t *testing.T, issuer, jwksURI string, keyPair *TestKeyPair) context.Context {
	discoveryDoc := fmt.Sprintf(`{
		"issuer": "%s",
		"authorization_endpoint": "%s/auth",
		"token_endpoint": "%s/token",
		"userinfo_endpoint": "%s/userinfo",
		"jwks_uri": "%s",
		"subject_types_supported": ["public"],
		"response_types_supported": ["code", "id_token"],
		"id_token_signing_alg_values_supported": ["RS256"]
	}`, issuer, issuer, issuer, issuer, jwksURI)

	responses := map[string]string{
		issuer + "/.well-known/openid_configuration": discoveryDoc,
		issuer + "/.well-known/openid-configuration": discoveryDoc, // Alternative spelling
		jwksURI: keyPair.JWKS,
	}

	client := &http.Client{
		Transport: &mockTransport{responses: responses},
	}

	return oidc.ClientContext(context.Background(), client)
}

// CreateStandardGoogleClaims creates standard Google OIDC claims for testing.
func CreateStandardGoogleClaims(clientID, subject, email, name string) map[string]interface{} {
	now := time.Now()
	return map[string]interface{}{
		"iss":            "https://accounts.google.com",
		"aud":            clientID,
		"sub":            subject,
		"email":          email,
		"email_verified": true,
		"name":           name,
		"picture":        "https://example.com/photo.jpg",
		"iat":            now.Unix(),
		"exp":            now.Add(time.Hour).Unix(),
	}
}

// CreateStandardAppleClaims creates standard Apple OIDC claims for testing.
func CreateStandardAppleClaims(clientID, subject, email, name string) map[string]interface{} {
	now := time.Now()
	return map[string]interface{}{
		"iss":            "https://appleid.apple.com",
		"aud":            clientID,
		"sub":            subject,
		"email":          email,
		"email_verified": true,
		"name":           name,
		"iat":            now.Unix(),
		"exp":            now.Add(time.Hour).Unix(),
	}
}

// MockOIDCSetup contains everything needed for a mock OIDC test setup.
type MockOIDCSetup struct {
	Manager  *OIDCProviderManager
	KeyPair  *TestKeyPair
	ClientID string
	//nolint:containedctx // Test fixture: each mock OIDC setup carries the
	// context its handlers were registered with, so callers get one value back.
	Context context.Context
}

// SetupMockGoogleOIDC creates a complete mock Google OIDC setup for testing.
func SetupMockGoogleOIDC(t *testing.T, clientID string) *MockOIDCSetup {
	keyPair := GenerateTestKeyAndJWKS(t, "google-test-key")
	ctx := CreateMockOIDCContext(t, "https://accounts.google.com", "https://www.googleapis.com/oauth2/v3/certs", keyPair)

	manager := NewOIDCProviderManager()
	err := manager.RegisterGoogleProvider(ctx, clientID)
	if err != nil {
		t.Fatalf("Failed to register Google provider: %v", err)
	}

	return &MockOIDCSetup{
		Manager:  manager,
		KeyPair:  keyPair,
		ClientID: clientID,
		Context:  ctx,
	}
}

// CreateValidGoogleToken creates a valid Google ID token for testing.
func (setup *MockOIDCSetup) CreateValidGoogleToken(t *testing.T, subject, email, name string) string {
	claims := CreateStandardGoogleClaims(setup.ClientID, subject, email, name)
	return GenerateTestIDToken(t, setup.KeyPair, claims)
}

// CreateValidGoogleTokenWithPicture creates a valid Google ID token with a custom picture URL.
func (setup *MockOIDCSetup) CreateValidGoogleTokenWithPicture(t *testing.T, subject, email, name, pictureURL string) string {
	claims := CreateStandardGoogleClaims(setup.ClientID, subject, email, name)
	// Override the picture URL (or remove it if empty)
	if pictureURL == "" {
		delete(claims, "picture")
	} else {
		claims["picture"] = pictureURL
	}
	return GenerateTestIDToken(t, setup.KeyPair, claims)
}

// CreateValidAppleToken creates a valid Apple ID token for testing.
func (setup *MockOIDCSetup) CreateValidAppleToken(t *testing.T, subject, email, name string) string {
	claims := CreateStandardAppleClaims(setup.ClientID, subject, email, name)
	return GenerateTestIDToken(t, setup.KeyPair, claims)
}

// CreateExpiredToken creates an expired ID token for testing.
func (setup *MockOIDCSetup) CreateExpiredToken(t *testing.T, issuer, subject string) string {
	pastTime := time.Now().Add(-2 * time.Hour)
	claims := map[string]interface{}{
		"iss": issuer,
		"aud": setup.ClientID,
		"sub": subject,
		"iat": pastTime.Unix(),
		"exp": pastTime.Add(time.Hour).Unix(), // Expired 1 hour ago
	}
	return GenerateTestIDToken(t, setup.KeyPair, claims)
}

// CreateWrongAudienceToken creates a token with wrong audience for testing.
func (setup *MockOIDCSetup) CreateWrongAudienceToken(t *testing.T, issuer, subject string) string {
	now := time.Now()
	claims := map[string]interface{}{
		"iss": issuer,
		"aud": "wrong-client-id.apps.googleusercontent.com",
		"sub": subject,
		"iat": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
	}
	return GenerateTestIDToken(t, setup.KeyPair, claims)
}
