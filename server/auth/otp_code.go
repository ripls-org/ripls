package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashOTPCode hashes a short one-time code for storage.
//
// It is deliberately separate from HashPassword rather than a caller of it:
// HashPassword enforces a minimum length that a 6-digit code cannot meet, and
// that policy is right for passwords and meaningless for codes. Keeping them
// apart also means the code path survives the removal of password auth (#2571)
// without inheriting a rule written for a credential that no longer exists.
//
// bcrypt is the right cost here despite the small keyspace — precisely because
// of it. A 6-digit code is one of a million, which a fast hash would exhaust
// instantly if the database were ever disclosed; at cost 12 that same sweep
// costs days, by which point every code has long expired.
func HashOTPCode(code string) (string, error) {
	if code == "" {
		return "", fmt.Errorf("code is required")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(code), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash code: %w", err)
	}
	return string(hash), nil
}

// VerifyOTPCode reports whether a submitted code matches a stored hash.
// bcrypt's comparison is constant-time with respect to the code contents.
func VerifyOTPCode(code, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(code))
}
