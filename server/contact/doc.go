// Package contact normalizes and validates phone numbers to E.164 for the
// off-app contact handles shared across the provisional-user, invitation, and
// notification flows. Centralizing normalization means a handle dedups and
// routes consistently no matter where it enters the system. Email handles are
// normalized via server/auth.NormalizeEmail (lowercase + trim); this package is
// the home for the phone-specific logic.
package contact
