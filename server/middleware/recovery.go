package middleware

import (
	"encoding/json"
	"net/http"
	"runtime/debug"

	"go.ripls.org/ripls/server/logging"
)

// connectError is the JSON body shape for a Connect protocol error response.
type connectError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// PanicRecovery returns middleware that catches any panic from downstream
// handlers, logs it with a full stack trace, and writes an HTTP 500 response
// with a Connect-compatible JSON error body so clients receive a parseable
// error rather than a connection drop.
//
// It should be the outermost wrapper in the handler chain so it protects all
// layers below it (auth, logging, CORS, Connect handlers, etc.).
//
// Usage:
//
//	handler := middleware.PanicRecovery(logger)(innerHandler)
func PanicRecovery(logger *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					attrs := []any{
						"panic", rec,
						"stack", string(debug.Stack()),
						"method", r.Method,
						"path", r.URL.Path,
					}
					if requestID := logging.RequestIDFromContext(r.Context()); requestID != "" {
						attrs = append(attrs, "request_id", requestID)
					}
					logger.ErrorContext(r.Context(), "panic recovered", attrs...)

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					body, _ := json.Marshal(connectError{
						Code:    "internal",
						Message: "internal server error",
					})
					_, _ = w.Write(body)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
