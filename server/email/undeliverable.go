package email

import "strings"

// reservedTLDs are the top-level domains RFC 2606 and RFC 6761 set aside for
// documentation and testing. They will never resolve, so mail addressed to them
// cannot be delivered — it is a guaranteed bounce.
var reservedTLDs = []string{".test", ".example", ".invalid", ".localhost"}

// reservedSecondLevel are the second-level domains RFC 2606 reserves for the
// same purpose. Subdomains count too, which is what covers the synthetic
// addresses the test harnesses generate (e.g. "…@e2etest.example.com").
var reservedSecondLevel = []string{"example.com", "example.net", "example.org"}

// IsUndeliverableTestAddress reports whether an address is in a domain reserved
// for testing, and therefore cannot receive mail.
//
// Attempting delivery to one is not merely wasteful — it is actively harmful.
// Every attempt is a hard bounce, and bounce rate is the primary input to a
// sending domain's reputation. Since a mailed one-time code is now how people
// sign in (#2571), degrading that reputation degrades the ability to log in.
//
// The exposure is real, not theoretical: registration sends a code, and the
// test harnesses register in bulk. The dev database holds ~1,900 accounts at
// `e2etest.example.com` alone. Pointed at a server with live mail credentials,
// that is ~1,900 hard bounces off the domain production depends on.
//
// This is checked at the send layer rather than at registration on purpose:
// the address must still be accepted, the code still stored, and verification
// still work. Only the doomed network call is skipped. That keeps every test
// path functioning — they read the code from the dev-mode echo, never a mailbox
// — while removing the bounce.
//
// It applies in every environment, including production: someone typing
// "someone@example.com" into a real sign-in form should not cost us a bounce
// either.
func IsUndeliverableTestAddress(email string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := strings.ToLower(strings.TrimSpace(email[at+1:]))
	if domain == "" {
		return false
	}

	for _, tld := range reservedTLDs {
		// Match both a subdomain ("thing.localhost") and the bare label used as
		// the whole domain ("localhost"), which the suffix check alone misses.
		if domain == strings.TrimPrefix(tld, ".") || strings.HasSuffix(domain, tld) {
			return true
		}
	}
	for _, sld := range reservedSecondLevel {
		if domain == sld || strings.HasSuffix(domain, "."+sld) {
			return true
		}
	}
	return false
}
