// Package timezone provides timezone validation utilities.
package timezone

import (
	"fmt"
	"time"
)

// ValidateIANA validates that the given string is a valid IANA timezone.
// Returns an error if the timezone is invalid or not found.
//
// Example valid timezones:
// - "America/Los_Angeles"
// - "Europe/London"
// - "Asia/Tokyo".
func ValidateIANA(tz string) error {
	if tz == "" {
		return fmt.Errorf("timezone cannot be empty")
	}

	_, err := time.LoadLocation(tz)
	if err != nil {
		return fmt.Errorf("invalid IANA timezone %q: %w", tz, err)
	}

	return nil
}

// IsValidOrEmpty returns true if timezone is empty or valid IANA format.
// Useful for optional timezone fields.
func IsValidOrEmpty(tz string) bool {
	if tz == "" {
		return true
	}
	return ValidateIANA(tz) == nil
}
