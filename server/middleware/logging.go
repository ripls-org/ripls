package middleware

import (
	"net/http"
	"strings"
	"time"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/logging"
)

// Logging returns middleware that logs each HTTP request.
// It logs method, path, status code, and duration for every request.
// If a request ID is present in context, it's included.
// If the request is authenticated, user_id is included.
//
// The middleware should be placed after RequestID middleware and
// before auth middleware in the chain for best results.
//
// Usage:
//
//	handler := middleware.RequestID(middleware.Logging(logger)(myHandler))
func Logging(logger *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			// Process request
			next.ServeHTTP(wrapped, r)

			// Calculate duration
			duration := time.Since(start)

			// Build log attributes
			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.status,
				"duration_ms", duration.Milliseconds(),
				"response_bytes", wrapped.bytesWritten,
				"remote_addr", r.RemoteAddr,
			}

			// Extract Connect RPC method name from path (e.g. "/ripls.api.GearService/SaveGear" → "GearService/SaveGear").
			if rpcMethod := extractRPCMethod(r.URL.Path); rpcMethod != "" {
				attrs = append(attrs, "rpc_method", rpcMethod)
			}

			// Add request ID if present
			if requestID := logging.RequestIDFromContext(r.Context()); requestID != "" {
				attrs = append(attrs, "request_id", requestID)
			}

			// Add user ID if authenticated
			if authInfo, ok := auth.GetAuthInfo(r.Context()); ok {
				attrs = append(attrs, "user_id", authInfo.UserID)
			}

			// Choose log level based on status code
			switch {
			case wrapped.status >= 500:
				logger.Error("http request", attrs...)
			case wrapped.status >= 400:
				logger.Warn("http request", attrs...)
			default:
				logger.Info("http request", attrs...)
			}
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture the status code
// and count response body bytes.
type responseWriter struct {
	http.ResponseWriter
	status       int
	wroteHeader  bool
	bytesWritten int
}

// WriteHeader captures the status code before writing.
func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

// Write ensures status is set before writing body and tracks bytes written.
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// Unwrap returns the underlying ResponseWriter for middleware that needs it.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// Flush implements http.Flusher for streaming responses (SSE, etc).
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// extractRPCMethod extracts the Connect RPC service and method from a URL path.
// Connect-Go paths follow the pattern "/ripls.api.ServiceName/MethodName", and
// only that exact package prefix is accepted. A looser "first segment contains
// a dot" heuristic let internet scanner probes for dot-files ("/.git/config",
// "/.well-known/…", "/.aws/credentials") mint junk rpc_method label values in
// the log-based metrics built on this field (#2622).
// Returns "ServiceName/MethodName" or empty string for non-RPC paths.
func extractRPCMethod(path string) string {
	// Connect-Go paths: /ripls.api.GearService/SaveGear → GearService/SaveGear
	rest, ok := strings.CutPrefix(path, "/ripls.api.")
	if !ok {
		return ""
	}
	service, method, ok := strings.Cut(rest, "/")
	if !ok || service == "" || method == "" || strings.Contains(method, "/") {
		return ""
	}
	return service + "/" + method
}
