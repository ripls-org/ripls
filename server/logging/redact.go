package logging

import (
	"log/slog"
	"strings"
)

// MaskEmail masks an email address for safe logging.
// Example: "alice@example.com" → "al***@example.com"
// If the email is invalid or too short, returns "***@***".
func MaskEmail(email string) string {
	if email == "" {
		return ""
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "***@***"
	}

	local, domain := parts[0], parts[1]
	if domain == "" {
		return "***@***"
	}

	// Mask the local part, keeping first 2 characters if possible
	var maskedLocal string
	switch {
	case len(local) == 0:
		maskedLocal = "***"
	case len(local) == 1:
		maskedLocal = local + "***"
	case len(local) == 2:
		maskedLocal = local + "***"
	default:
		maskedLocal = local[:2] + "***"
	}

	return maskedLocal + "@" + domain
}

// MaskToken masks a token for safe logging by showing only the first 8 characters.
// Example: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." → "eyJhbGci..."
// Returns empty string for empty input, full token if shorter than 8 chars.
func MaskToken(token string) string {
	if token == "" {
		return ""
	}

	if len(token) <= 8 {
		return token
	}

	return token[:8] + "..."
}

// MaskPhone masks a phone number for safe logging, keeping only the last 4
// digits. Example: "+15551234567" → "+1******4567". Returns "" for empty input
// and "***" when there are fewer than 4 digits to mask meaningfully. A leading
// "+" is preserved so the masked value still reads as a phone number.
func MaskPhone(phone string) string {
	if phone == "" {
		return ""
	}

	var digits []rune
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) < 4 {
		return "***"
	}

	masked := strings.Repeat("*", len(digits)-4) + string(digits[len(digits)-4:])
	if strings.HasPrefix(phone, "+") {
		return "+" + masked
	}
	return masked
}

// RedactedPhone wraps a phone string and implements slog.LogValuer to
// automatically mask the number when logged.
type RedactedPhone string

// LogValue implements slog.LogValuer for automatic phone masking.
func (p RedactedPhone) LogValue() slog.Value {
	return slog.StringValue(MaskPhone(string(p)))
}

// RedactedEmail wraps an email string and implements slog.LogValuer
// to automatically mask the email when logged.
type RedactedEmail string

// LogValue implements slog.LogValuer for automatic email masking.
func (e RedactedEmail) LogValue() slog.Value {
	return slog.StringValue(MaskEmail(string(e)))
}

// RedactedToken wraps a token string and implements slog.LogValuer
// to automatically mask the token when logged.
type RedactedToken string

// LogValue implements slog.LogValuer for automatic token masking.
func (t RedactedToken) LogValue() slog.Value {
	return slog.StringValue(MaskToken(string(t)))
}
