package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/authn"
	"github.com/golang-jwt/jwt/v5"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// GenerateRefreshToken generates a cryptographically secure random refresh token.
// Returns a base64url-encoded 32-byte token suitable for sending to clients.
// The raw token must never be stored — use HashRefreshToken for storage.
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashRefreshToken computes the SHA-256 hash of a refresh token for secure storage.
// Always hash before storing — raw tokens are equivalent to passwords.
func HashRefreshToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// TokenConfig holds configuration for JWT token operations.
type TokenConfig struct {
	Secret     []byte
	Expiration time.Duration
}

// MinSigningSecretBytes is the minimum acceptable length for a JWT signing
// secret. HS256 keys should be at least as long as the hash output
// (SHA-256 → 32 bytes); shorter keys weaken the construction.
const MinSigningSecretBytes = 32

// ValidateSigningSecret returns an error if the secret is missing or shorter
// than MinSigningSecretBytes. Callers should treat the error as fatal at
// startup.
func ValidateSigningSecret(secret []byte) error {
	if len(secret) < MinSigningSecretBytes {
		return fmt.Errorf("jwt signing secret must be at least %d bytes (got %d)", MinSigningSecretBytes, len(secret))
	}
	return nil
}

// GenerateToken creates a new JWT token for a user. For phone-based users,
// pass an empty email and a non-empty phoneNumber. For email-based users,
// pass an empty phoneNumber.
func (c *TokenConfig) GenerateToken(userID, email string, role models.Role, phoneNumber ...string) (string, error) {
	ctx := context.Background()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GenerateToken",
		"auth_method", "jwt",
		"user_id", userID,
		"user_email", logging.MaskEmail(email),
		"role", role.String(),
	)

	claims := jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"role":    role.String(), // Store enum as human-readable string
		"exp":     time.Now().Add(c.Expiration).Unix(),
		"iat":     time.Now().Unix(),
		"nbf":     time.Now().Unix(),
		"iss":     "gear-library",
		"sub":     userID,
	}

	if len(phoneNumber) > 0 && phoneNumber[0] != "" {
		claims["phone_number"] = phoneNumber[0]
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(c.Secret)
	if err != nil {
		logger.ErrorContext(ctx, "failed to sign token",
			"error", err,
		)
		return "", err
	}

	logger.DebugContext(ctx, "token generated successfully",
		"token_type", "access",
		"expiration", c.Expiration,
	)
	return tokenString, nil
}

// ValidateToken validates a JWT token and returns the claims.
func (c *TokenConfig) ValidateToken(tokenString string) (jwt.MapClaims, error) {
	ctx := context.Background()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ValidateToken",
		"auth_method", "jwt",
		"token", logging.MaskToken(tokenString),
	)

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			logger.WarnContext(ctx, "unexpected signing method",
				"signing_method", token.Header["alg"],
			)
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return c.Secret, nil
	})
	if err != nil {
		logger.WarnContext(ctx, "token parsing failed",
			"error", err,
			"token_type", "access",
		)
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		logger.DebugContext(ctx, "token validated successfully",
			"token_type", "access",
		)
		return claims, nil
	}

	logger.WarnContext(ctx, "invalid token claims",
		"token_type", "access",
	)
	return nil, fmt.Errorf("invalid token")
}

// Info represents the authenticated user information.
type Info struct {
	UserID      string
	Email       string
	PhoneNumber string // E.164 format, set for phone auth users.
	Role        models.Role
}

// parseRole converts role string back to enum.
func parseRole(roleStr string) models.Role {
	switch roleStr {
	case "ROLE_USER":
		return models.Role_ROLE_USER
	case "ROLE_ADMIN":
		return models.Role_ROLE_ADMIN
	default:
		return models.Role_ROLE_UNSPECIFIED
	}
}

// NewAuthFunc creates an authn.AuthFunc that validates JWT tokens.
func (c *TokenConfig) NewAuthFunc() authn.AuthFunc {
	return func(ctx context.Context, req *http.Request) (any, error) {
		logger := logging.LoggerWithContext(ctx).With(
			"operation", "AuthFunc",
			"auth_method", "jwt",
		)

		// Extract Bearer token from Authorization header
		token, ok := authn.BearerToken(req)
		if !ok {
			logger.WarnContext(ctx, "authentication failed: missing or invalid authorization header",
				"token_type", "access",
			)
			return nil, authn.Errorf("missing or invalid authorization header")
		}

		claims, err := c.ValidateToken(token)
		if err != nil {
			logger.WarnContext(ctx, "authentication failed: token validation failed",
				"error", err,
				"token_type", "access",
			)
			return nil, authn.Errorf("invalid token: %v", err)
		}

		// Reject single-purpose tokens outright. Email-ownership proofs are
		// signed with this same secret but are not session credentials; they
		// carry a `purpose` claim and access tokens never do. They would also
		// fail the user_id check below, but that is incidental — this is the
		// explicit guard, so the property survives future claim changes.
		if purpose, _ := claims["purpose"].(string); purpose != "" {
			logger.WarnContext(ctx, "authentication failed: single-purpose token presented as access token",
				"token_type", "access",
				"purpose", purpose,
			)
			return nil, authn.Errorf("invalid token claims")
		}

		// Extract our custom claims
		userID, _ := claims["user_id"].(string)
		email, _ := claims["email"].(string)
		phoneNumber, _ := claims["phone_number"].(string)
		roleStr, _ := claims["role"].(string)

		if userID == "" || (email == "" && phoneNumber == "") {
			logger.WarnContext(ctx, "authentication failed: invalid token claims",
				"token_type", "access",
				"has_user_id", userID != "",
				"has_email", email != "",
				"has_phone", phoneNumber != "",
			)
			return nil, authn.Errorf("invalid token claims")
		}

		role := parseRole(roleStr)

		logger.DebugContext(ctx, "authentication successful",
			"user_id", userID,
			"user_email", logging.MaskEmail(email),
			"role", role.String(),
			"token_type", "access",
		)

		// Return AuthInfo which will be available via authn.GetInfo(ctx)
		return &Info{
			UserID:      userID,
			Email:       email,
			PhoneNumber: phoneNumber,
			Role:        role,
		}, nil
	}
}

// GetAuthInfo extracts authentication info from context using authn-go.
func GetAuthInfo(ctx context.Context) (*Info, bool) {
	info, ok := authn.GetInfo(ctx).(*Info)
	return info, ok
}
