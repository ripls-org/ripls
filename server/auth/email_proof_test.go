package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func testTokenConfig() *TokenConfig {
	return &TokenConfig{
		Secret:     []byte("test-secret-key-that-is-long-enough-32b"),
		Expiration: time.Hour,
	}
}

func TestGenerateEmailProofToken_RoundTrip(t *testing.T) {
	cfg := testTokenConfig()

	token, expiresAt, err := cfg.GenerateEmailProofToken("someone@example.com")
	if err != nil {
		t.Fatalf("GenerateEmailProofToken failed: %v", err)
	}
	if !expiresAt.After(time.Now()) {
		t.Error("expiry is not in the future")
	}

	email, err := cfg.ValidateEmailProofToken(token)
	if err != nil {
		t.Fatalf("ValidateEmailProofToken failed: %v", err)
	}
	if email != "someone@example.com" {
		t.Errorf("email = %q, want %q", email, "someone@example.com")
	}
}

func TestGenerateEmailProofToken_RequiresEmail(t *testing.T) {
	if _, _, err := testTokenConfig().GenerateEmailProofToken(""); err == nil {
		t.Fatal("minted a proof token with no address")
	}
}

func TestValidateEmailProofToken_RejectsForeignSignature(t *testing.T) {
	token, _, err := testTokenConfig().GenerateEmailProofToken("someone@example.com")
	if err != nil {
		t.Fatalf("GenerateEmailProofToken failed: %v", err)
	}

	other := &TokenConfig{Secret: []byte("a-completely-different-signing-secret!!"), Expiration: time.Hour}
	if _, err := other.ValidateEmailProofToken(token); err == nil {
		t.Fatal("a proof token validated under the wrong signing key")
	}
}

func TestValidateEmailProofToken_RejectsExpired(t *testing.T) {
	cfg := testTokenConfig()

	// Mint by hand so the expiry can be in the past.
	expired := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"purpose": EmailProofPurpose,
		"email":   "someone@example.com",
		"exp":     time.Now().Add(-time.Minute).Unix(),
		"iat":     time.Now().Add(-time.Hour).Unix(),
	})
	signed, err := expired.SignedString(cfg.Secret)
	if err != nil {
		t.Fatalf("failed to sign the test token: %v", err)
	}

	if _, err := cfg.ValidateEmailProofToken(signed); err == nil {
		t.Fatal("an expired proof token was accepted")
	}
}

// An access token carries an email claim of its own. Without the purpose check
// it would satisfy ValidateEmailProofToken, letting anyone holding a session
// assert ownership of the address on it — which is the whole thing the proof
// exists to establish independently.
func TestValidateEmailProofToken_RejectsAccessToken(t *testing.T) {
	cfg := testTokenConfig()

	accessToken, err := cfg.GenerateToken("user-1", "someone@example.com", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	if _, err := cfg.ValidateEmailProofToken(accessToken); err == nil {
		t.Fatal("an access token was accepted as an email ownership proof")
	}
}

// The mirror of the above: a proof token is signed with the same secret as an
// access token, so the authentication boundary must refuse it explicitly.
func TestNewAuthFunc_RejectsEmailProofToken(t *testing.T) {
	cfg := testTokenConfig()

	proof, _, err := cfg.GenerateEmailProofToken("someone@example.com")
	if err != nil {
		t.Fatalf("GenerateEmailProofToken failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/anything", nil)
	req.Header.Set("Authorization", "Bearer "+proof)

	if _, err := cfg.NewAuthFunc()(req.Context(), req); err == nil {
		t.Fatal("an email proof token authenticated a request as an access token")
	}
}

// A well-formed access token must keep working — the purpose guard has to
// reject single-purpose tokens without breaking ordinary authentication.
func TestNewAuthFunc_AcceptsAccessToken(t *testing.T) {
	cfg := testTokenConfig()

	accessToken, err := cfg.GenerateToken("user-1", "someone@example.com", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/anything", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	info, err := cfg.NewAuthFunc()(req.Context(), req)
	if err != nil {
		t.Fatalf("a valid access token was rejected: %v", err)
	}
	if info.(*Info).UserID != "user-1" {
		t.Errorf("UserID = %q, want %q", info.(*Info).UserID, "user-1")
	}
}
