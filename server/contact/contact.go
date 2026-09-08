package contact

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nyaruka/phonenumbers/v2"
)

// e164Pattern matches a normalized E.164 number: a leading '+', a non-zero
// country-code digit, then 7–14 more digits (8–15 digits total).
var e164Pattern = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)

// nonDigit matches any non-digit character.
var nonDigit = regexp.MustCompile(`\D`)

// NormalizePhoneE164 normalizes a loosely-formatted phone number to E.164
// (e.g. "(555) 123-4567" -> "+15551234567"). It mirrors the client's
// normalization (see docs/registration_and_login.md): a leading '+' is kept
// as-is (international); a bare 10-digit number is assumed US/Canada (+1); an
// 11-digit number starting with 1 is treated as US/Canada; formatting
// characters are stripped. It returns an error when the result is not a valid
// E.164 number, so callers never persist a malformed handle.
func NormalizePhoneE164(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("phone number is empty")
	}

	hasPlus := strings.HasPrefix(trimmed, "+")
	digits := nonDigit.ReplaceAllString(trimmed, "")
	if digits == "" {
		return "", fmt.Errorf("phone number has no digits")
	}

	var e164 string
	switch {
	case hasPlus:
		e164 = "+" + digits
	case len(digits) == 10:
		e164 = "+1" + digits
	case len(digits) == 11 && strings.HasPrefix(digits, "1"):
		e164 = "+" + digits
	default:
		return "", fmt.Errorf("ambiguous phone number %q: prefix with '+' and a country code", raw)
	}

	if !e164Pattern.MatchString(e164) {
		return "", fmt.Errorf("invalid phone number %q", raw)
	}
	return e164, nil
}

// IsDialableE164 reports whether e164 is a real, dialable phone number — not
// merely E.164-shaped. NormalizePhoneE164 only validates *shape* (a leading '+'
// and a plausible digit count), so it accepts well-formed-but-fake numbers like
// the +1 555-xxx-xxxx range (the 555 area code is unassigned in the North
// American Numbering Plan) or Firebase Phone Auth's fictional test numbers.
// Those are useful at sign-up (Firebase returns a fixed OTP without touching a
// real carrier) but must never reach an SMS provider, which rejects them
// (Twilio error 21211). This consults libphonenumber's validity metadata, which
// encodes assigned number ranges per region, so it rejects them.
//
// e164 should already be normalized E.164 (leading '+'); a parse failure or a
// number libphonenumber does not consider valid returns false.
func IsDialableE164(e164 string) bool {
	num, err := phonenumbers.Parse(e164, "")
	if err != nil {
		return false
	}
	return phonenumbers.IsValidNumber(num)
}
