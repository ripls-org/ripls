// Package middleware provides HTTP middleware for the server.
package middleware

import (
	"net/http"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/logging"
)

// RequestIDHeader is the HTTP header used for request ID propagation.
const RequestIDHeader = "X-Request-ID"

// RequestID returns middleware that ensures each request has a unique request ID.
// It first checks for an existing X-Request-ID header from the client.
// If not present, it generates a new UUID v4.
// The request ID is stored in the request context and added to the response header.
//
// Usage:
//
//	handler := middleware.RequestID(myHandler)
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Store in context for downstream handlers
		ctx := logging.WithRequestID(r.Context(), requestID)

		// Add to response header for client correlation
		w.Header().Set(RequestIDHeader, requestID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFunc returns middleware using the standard http.HandlerFunc signature.
// This is useful when chaining with other middleware that expects HandlerFunc.
func RequestIDFunc(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		RequestID(next).ServeHTTP(w, r)
	}
}
