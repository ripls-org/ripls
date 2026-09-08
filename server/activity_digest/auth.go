package activity_digest

import "go.ripls.org/ripls/server/gen/ripls/models"

// AuthMethodLabel returns a short human-readable label for an
// AuthMethod enum value. Used both in the digest's per-user listing
// and in the auth-method count breakdown line.
func AuthMethodLabel(m models.AuthMethod) string {
	switch m {
	case models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD:
		return "email/password"
	case models.AuthMethod_AUTH_METHOD_GOOGLE:
		return "Google"
	case models.AuthMethod_AUTH_METHOD_APPLE:
		return "Apple"
	case models.AuthMethod_AUTH_METHOD_PHONE:
		return "phone"
	default:
		return "unknown"
	}
}
