package auth

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"go.ripls.org/ripls/server/logging"
)

const (
	bcryptCost        = 12 // Good balance of security and performance
	minPasswordLength = 8
)

// HashPassword hashes a password using bcrypt.
func HashPassword(password string) (string, error) {
	ctx := context.Background()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "HashPassword",
		"auth_method", "password",
	)

	if err := ValidatePasswordStrength(password); err != nil {
		logger.WarnContext(ctx, "password validation failed",
			"error", err,
		)
		return "", err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		logger.ErrorContext(ctx, "failed to hash password",
			"error", err,
		)
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	logger.DebugContext(ctx, "password hashed successfully")
	return string(hash), nil
}

// VerifyPassword checks if a password matches a hash.
func VerifyPassword(password, hash string) error {
	ctx := context.Background()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "VerifyPassword",
		"auth_method", "password",
	)

	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		logger.WarnContext(ctx, "password verification failed",
			"error", err,
		)
		return err
	}

	logger.DebugContext(ctx, "password verified successfully")
	return nil
}

// ValidatePasswordStrength checks minimum password requirements.
func ValidatePasswordStrength(password string) error {
	if len(password) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}
	return nil
}
