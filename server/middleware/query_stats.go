// Query stats middleware: attaches per-request query counters and logs totals.

package middleware

import (
	"net/http"

	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// QueryStats returns middleware that attaches per-request database query
// counters to the context and logs the total query count and duration after
// the request completes.
func QueryStats(logger *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := storage.WithQueryStats(r.Context())
			next.ServeHTTP(w, r.WithContext(ctx))

			stats := storage.GetQueryStats(ctx)
			if stats == nil {
				return
			}
			count := stats.Count.Load()
			if count == 0 {
				return
			}

			attrs := []any{
				"db_queries", count,
				"db_duration_ms", stats.TotalDuration().Milliseconds(),
				"path", r.URL.Path,
			}
			// rpc_method labels the db_request_duration log-based metric
			// consistently with the RPC request metrics (#1613).
			if rpcMethod := extractRPCMethod(r.URL.Path); rpcMethod != "" {
				attrs = append(attrs, "rpc_method", rpcMethod)
			}
			if requestID := logging.RequestIDFromContext(ctx); requestID != "" {
				attrs = append(attrs, "request_id", requestID)
			}
			logger.Info("request db stats", attrs...)
		})
	}
}
