// Package safehttp provides an SSRF-guarded HTTP client builder for
// user-supplied URLs. Use NewClient for any code path that fetches a URL
// whose host is even partly attacker-controlled; prefer the standard
// library http.Client for trusted egress to known endpoints.
package safehttp
