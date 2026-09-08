package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// EmailProofPurpose is the value of the `purpose` claim on an email-ownership
// proof token. Access tokens never carry a purpose claim, and NewAuthFunc
// rejects any token that does — that asymmetry is what keeps a proof token from
// being replayed as a session credential even though both are signed with the
// same secret.
const EmailProofPurpose = "email_verification"

// EmailProofExpiration is how long an email-ownership proof stays usable. It
// only has to cover the hop from "code accepted" to "register or log in", so it
// is deliberately far shorter than an access token's lifetime.
const EmailProofExpiration = 10 * time.Minute

// GenerateEmailProofToken mints a short-lived token asserting that the bearer
// proved they can receive mail at email. It is the email counterpart of the
// phone flow's verified provider token: the credential that EmailRegister and
// EmailLogin accept in place of a password.
//
// The token deliberately carries no user_id and no role. It says one thing —
// "this address was proven" — and cannot identify or authorize an account.
func (c *TokenConfig) GenerateEmailProofToken(email string) (token string, expiresAt time.Time, err error) {
	if email == "" {
		return "", time.Time{}, fmt.Errorf("email is required to mint an email proof token")
	}

	now := time.Now()
	expiresAt = now.Add(EmailProofExpiration)

	claims := jwt.MapClaims{
		"purpose": EmailProofPurpose,
		"email":   email,
		"exp":     expiresAt.Unix(),
		"iat":     now.Unix(),
		"nbf":     now.Unix(),
		"iss":     "gear-library",
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(c.Secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign email proof token: %w", err)
	}
	return signed, expiresAt, nil
}

// ValidateEmailProofToken verifies an email-ownership proof and returns the
// address it attests to. It rejects tokens signed with a different key, expired
// tokens, and — critically — tokens that are not email proofs, so an access
// token cannot be passed off as proof of owning its own email claim.
func (c *TokenConfig) ValidateEmailProofToken(tokenString string) (string, error) {
	parsed, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return c.Secret, nil
	})
	if err != nil {
		return "", fmt.Errorf("invalid email proof token: %w", err)
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || !parsed.Valid {
		return "", fmt.Errorf("invalid email proof token")
	}

	if purpose, _ := claims["purpose"].(string); purpose != EmailProofPurpose {
		return "", fmt.Errorf("token is not an email ownership proof")
	}

	email, _ := claims["email"].(string)
	if email == "" {
		return "", fmt.Errorf("email proof token carries no address")
	}
	return email, nil
}
