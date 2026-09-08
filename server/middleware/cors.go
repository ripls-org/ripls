package middleware

import "strings"

// CORSOriginValidator returns an origin-validation func that allows origins
// matching the provided allowlist. Entries may be:
//   - an exact origin string (e.g. "https://app.example.com"),
//   - a port wildcard ending in ":*" (e.g. "http://localhost:*"), matching any
//     port on that host,
//   - a single-label subdomain wildcard of the form "https://*.example.com",
//     matching "https://<label>.example.com" for exactly one leading label
//     (used for per-PR staging previews like "<slug>.staging.example.com").
//
// The returned func satisfies gorilla/handlers.OriginValidator.
func CORSOriginValidator(allowed []string) func(origin string) bool {
	return func(origin string) bool {
		for _, a := range allowed {
			if a == "*" {
				return true
			}
			if strings.HasSuffix(a, ":*") {
				prefix := strings.TrimSuffix(a, "*")
				if strings.HasPrefix(origin, prefix) {
					return true
				}
			}
			if i := strings.Index(a, "://*."); i != -1 {
				scheme := a[:i+len("://")]  // e.g. "https://"
				suffix := a[i+len("://*"):] // e.g. ".staging.example.com"
				if strings.HasPrefix(origin, scheme) {
					host := origin[len(scheme):]
					if strings.HasSuffix(host, suffix) {
						label := host[:len(host)-len(suffix)]
						// Exactly one non-empty label, no nested subdomains or paths.
						if label != "" && !strings.ContainsAny(label, "./") {
							return true
						}
					}
				}
			}
			if origin == a {
				return true
			}
		}
		return false
	}
}
