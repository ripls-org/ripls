package middleware

import (
	"net/http"
	"strings"

	"go.ripls.org/ripls/server/logging"
)

// RemoteAddr returns middleware that resolves the client IP address and stores
// it in the request context via logging.WithRemoteAddr. It reads the first
// (leftmost) address from X-Forwarded-For, which is the original client IP
// as set by the first proxy in the chain. When that header is absent it falls
// back to r.RemoteAddr (the direct TCP connection address).
//
// Place this middleware early in the chain (alongside RequestID) so that all
// downstream handlers — including auth failure log sites — can call
// logging.RemoteAddrFromContext(ctx) without needing access to the HTTP request.
func RemoteAddr(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addr := resolveClientAddr(r)
		ctx := logging.WithRemoteAddr(r.Context(), addr)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resolveClientAddr returns the best-effort client IP for the request.
// It prefers the first value of X-Forwarded-For (set by the outermost proxy)
// and falls back to the TCP-level RemoteAddr stripped of its port.
func resolveClientAddr(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For may be a comma-separated list: "client, proxy1, proxy2".
		// The leftmost entry is the original client IP.
		first := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
		if first != "" {
			return first
		}
	}
	// Strip port from "host:port" or "[::1]:port" — keep only the host/IP part.
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i > 0 {
		host := addr[:i]
		// Unwrap IPv6 bracket notation: "[::1]" → "::1"
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
		if host != "" {
			return host
		}
	}
	return addr
}
