package webfetch

import (
	"net/http"
	"net/http/cookiejar"
)

// newTestFetcher creates an HTTPFetcher backed by a plain http.Client
// that allows loopback connections, for use in unit tests that spin up
// httptest servers. Both the SSRF dialer guard and the IP-literal pre-check
// are intentionally absent; use NewHTTPFetcher for any test that must verify
// SSRF behaviour.
func newTestFetcher(opts ...Option) *HTTPFetcher {
	f := NewHTTPFetcher(opts...)
	jar, _ := cookiejar.New(nil)
	f.client = &http.Client{
		Timeout: f.timeout,
		Jar:     jar,
	}
	f.skipIPPreCheck = true
	return f
}
