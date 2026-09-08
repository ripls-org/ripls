package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"go.ripls.org/ripls/server/storage"
)

// RequestHost injects the client's request host into the context so
// storage.LocalBucketStorage can generate presigned URLs that work from any
// client (e.g. Android emulator at 10.0.2.2, localhost, etc.). Hosts without
// an explicit port are normalized with defaultPort. Only useful with local
// bucket storage — with GCS, presigned URLs carry their own host, so callers
// skip this middleware entirely.
func RequestHost(defaultPort string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			if !strings.Contains(host, ":") {
				host = fmt.Sprintf("%s:%s", host, defaultPort)
			}
			ctx := storage.WithRequestHost(r.Context(), host)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
