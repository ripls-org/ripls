package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.ripls.org/ripls/server/logging"
)

func TestRemoteAddr_XForwardedFor(t *testing.T) {
	var got string
	handler := RemoteAddr(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = logging.RemoteAddrFromContext(r.Context())
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1, 10.0.0.2")
	handler.ServeHTTP(httptest.NewRecorder(), r)

	if got != "203.0.113.5" {
		t.Errorf("expected first XFF hop 203.0.113.5, got %q", got)
	}
}

func TestRemoteAddr_XForwardedForSingle(t *testing.T) {
	var got string
	handler := RemoteAddr(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = logging.RemoteAddrFromContext(r.Context())
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "198.51.100.42")
	handler.ServeHTTP(httptest.NewRecorder(), r)

	if got != "198.51.100.42" {
		t.Errorf("expected 198.51.100.42, got %q", got)
	}
}

func TestRemoteAddr_FallbackToRemoteAddr(t *testing.T) {
	var got string
	handler := RemoteAddr(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = logging.RemoteAddrFromContext(r.Context())
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	// No X-Forwarded-For; httptest.NewRequest sets RemoteAddr to "192.0.2.1:1234".
	r.RemoteAddr = "192.0.2.1:1234"
	handler.ServeHTTP(httptest.NewRecorder(), r)

	if got != "192.0.2.1" {
		t.Errorf("expected 192.0.2.1 (port stripped), got %q", got)
	}
}

func TestRemoteAddr_FallbackIPv6(t *testing.T) {
	var got string
	handler := RemoteAddr(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = logging.RemoteAddrFromContext(r.Context())
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "[::1]:5432"
	handler.ServeHTTP(httptest.NewRecorder(), r)

	if got != "::1" {
		t.Errorf("expected ::1 (brackets stripped), got %q", got)
	}
}

func TestResolveClientAddr(t *testing.T) {
	cases := []struct {
		name     string
		xff      string
		remote   string
		expected string
	}{
		{"xff multi-hop", "1.2.3.4, 5.6.7.8", "10.0.0.1:9999", "1.2.3.4"},
		{"xff single", "9.9.9.9", "10.0.0.1:9999", "9.9.9.9"},
		{"xff with spaces", "  9.9.9.9  , 10.0.0.1", "10.0.0.1:9999", "9.9.9.9"},
		{"no xff ipv4", "", "1.2.3.4:8080", "1.2.3.4"},
		{"no xff ipv6", "", "[::1]:8080", "::1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			r.RemoteAddr = tc.remote
			got := resolveClientAddr(r)
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
