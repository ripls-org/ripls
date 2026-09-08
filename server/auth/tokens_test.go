package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/authn"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestValidateSigningSecret(t *testing.T) {
	tests := []struct {
		name    string
		secret  []byte
		wantErr bool
	}{
		{"nil rejected", nil, true},
		{"empty rejected", []byte(""), true},
		{"one byte rejected", []byte("x"), true},
		{"dev-secret-key (14 bytes) rejected", []byte("dev-secret-key"), true},
		{"31 bytes rejected", make([]byte, 31), true},
		{"exactly 32 bytes accepted", make([]byte, 32), false},
		{"64 bytes accepted", make([]byte, 64), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSigningSecret(tt.secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSigningSecret(%d bytes) err = %v, wantErr %v", len(tt.secret), err, tt.wantErr)
			}
		})
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	t.Run("returns non-empty token", func(t *testing.T) {
		token, err := GenerateRefreshToken()
		if err != nil {
			t.Fatalf("GenerateRefreshToken returned error: %v", err)
		}
		if token == "" {
			t.Fatal("Expected non-empty token")
		}
	})

	t.Run("tokens are unique", func(t *testing.T) {
		token1, err := GenerateRefreshToken()
		if err != nil {
			t.Fatalf("GenerateRefreshToken returned error: %v", err)
		}
		token2, err := GenerateRefreshToken()
		if err != nil {
			t.Fatalf("GenerateRefreshToken returned error: %v", err)
		}
		if token1 == token2 {
			t.Error("Expected distinct tokens on successive calls")
		}
	})

	t.Run("token is base64url-encoded 32 bytes", func(t *testing.T) {
		token, err := GenerateRefreshToken()
		if err != nil {
			t.Fatalf("GenerateRefreshToken returned error: %v", err)
		}
		// base64url without padding: ceil(32*8/6) = 43 chars
		if len(token) != 43 {
			t.Errorf("Expected 43-char base64url token, got length %d", len(token))
		}
	})
}

func TestHashRefreshToken(t *testing.T) {
	t.Run("same input produces same hash", func(t *testing.T) {
		token := "test-refresh-token"
		hash1 := HashRefreshToken(token)
		hash2 := HashRefreshToken(token)
		if hash1 != hash2 {
			t.Error("Expected identical hashes for the same input")
		}
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		hash1 := HashRefreshToken("token-a")
		hash2 := HashRefreshToken("token-b")
		if hash1 == hash2 {
			t.Error("Expected different hashes for different inputs")
		}
	})

	t.Run("hash is lowercase hex SHA-256", func(t *testing.T) {
		hash := HashRefreshToken("any-token")
		// SHA-256 hex is always 64 lowercase hex chars
		if len(hash) != 64 {
			t.Errorf("Expected 64-char hex hash, got length %d", len(hash))
		}
		for _, c := range hash {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("Hash contains non-lowercase-hex character: %c", c)
				break
			}
		}
	})

	t.Run("raw token does not appear in hash", func(t *testing.T) {
		token := "super-secret-refresh-token"
		hash := HashRefreshToken(token)
		if hash == token {
			t.Error("Hash must not equal the raw token")
		}
	})
}

func TestTokenConfig_GenerateToken(t *testing.T) {
	config := &TokenConfig{
		Secret:     []byte("test-secret"),
		Expiration: time.Hour,
	}

	token, err := config.GenerateToken("user123", "test@example.com", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	if token == "" {
		t.Fatal("Generated token is empty")
	}
}

func TestTokenConfig_ValidateToken(t *testing.T) {
	config := &TokenConfig{
		Secret:     []byte("test-secret"),
		Expiration: time.Hour,
	}

	// Generate a token
	userID := "user123"
	email := "test@example.com"
	role := models.Role_ROLE_ADMIN

	token, err := config.GenerateToken(userID, email, role)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Validate the token
	claims, err := config.ValidateToken(token)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	// Check claims
	if claims["user_id"] != userID {
		t.Errorf("Expected user_id %s, got %v", userID, claims["user_id"])
	}

	if claims["email"] != email {
		t.Errorf("Expected email %s, got %v", email, claims["email"])
	}

	if claims["role"] != role.String() {
		t.Errorf("Expected role %s, got %v", role.String(), claims["role"])
	}
}

func TestTokenConfig_ValidateToken_Invalid(t *testing.T) {
	config := &TokenConfig{
		Secret:     []byte("test-secret"),
		Expiration: time.Hour,
	}

	tests := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"invalid token", "invalid-token"},
		{"wrong signature", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoidGVzdCJ9.wrong-signature"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.ValidateToken(tt.token)
			if err == nil {
				t.Errorf("Expected validation to fail for %s", tt.name)
			}
		})
	}
}

func TestTokenConfig_NewAuthFunc(t *testing.T) {
	config := &TokenConfig{
		Secret:     []byte("test-secret"),
		Expiration: time.Hour,
	}

	// Generate a valid token
	token, err := config.GenerateToken("user123", "test@example.com", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	authFunc := config.NewAuthFunc()

	t.Run("valid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		ctx := context.Background()
		authInfo, err := authFunc(ctx, req)
		if err != nil {
			t.Fatalf("Expected auth to succeed, got error: %v", err)
		}

		info, ok := authInfo.(*Info)
		if !ok {
			t.Fatalf("Expected *AuthInfo, got %T", authInfo)
		}

		if info.UserID != "user123" {
			t.Errorf("Expected UserID user123, got %s", info.UserID)
		}

		if info.Email != "test@example.com" {
			t.Errorf("Expected Email test@example.com, got %s", info.Email)
		}

		if info.Role != models.Role_ROLE_USER {
			t.Errorf("Expected Role ROLE_USER, got %v", info.Role)
		}
	})

	t.Run("missing authorization header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)

		ctx := context.Background()
		_, err := authFunc(ctx, req)
		if err == nil {
			t.Fatal("Expected auth to fail with missing header")
		}
	})

	t.Run("invalid token format", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Invalid token-format")

		ctx := context.Background()
		_, err := authFunc(ctx, req)
		if err == nil {
			t.Fatal("Expected auth to fail with invalid format")
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")

		ctx := context.Background()
		_, err := authFunc(ctx, req)
		if err == nil {
			t.Fatal("Expected auth to fail with invalid token")
		}
	})
}

func TestParseRole(t *testing.T) {
	tests := []struct {
		input    string
		expected models.Role
	}{
		{"ROLE_USER", models.Role_ROLE_USER},
		{"ROLE_ADMIN", models.Role_ROLE_ADMIN},
		{"", models.Role_ROLE_UNSPECIFIED},
		{"INVALID", models.Role_ROLE_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseRole(tt.input)
			if result != tt.expected {
				t.Errorf("parseRole(%s) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetAuthInfo(t *testing.T) {
	authInfo := &Info{
		UserID: "user123",
		Email:  "test@example.com",
		Role:   models.Role_ROLE_USER,
	}

	ctx := authn.SetInfo(context.Background(), authInfo)

	retrievedInfo, ok := GetAuthInfo(ctx)
	if !ok {
		t.Fatal("Expected to retrieve auth info from context")
	}

	if retrievedInfo.UserID != authInfo.UserID {
		t.Errorf("Expected UserID %s, got %s", authInfo.UserID, retrievedInfo.UserID)
	}

	if retrievedInfo.Email != authInfo.Email {
		t.Errorf("Expected Email %s, got %s", authInfo.Email, retrievedInfo.Email)
	}

	if retrievedInfo.Role != authInfo.Role {
		t.Errorf("Expected Role %v, got %v", authInfo.Role, retrievedInfo.Role)
	}
}
