package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.ripls.org/ripls/server/logging"
)

func TestRequestID_GeneratesIDWhenMissing(t *testing.T) {
	var capturedRequestID string

	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRequestID = logging.RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should generate a request ID
	if capturedRequestID == "" {
		t.Error("Expected request ID to be generated, got empty string")
	}

	// Should be in response header
	responseID := rec.Header().Get(RequestIDHeader)
	if responseID == "" {
		t.Error("Expected X-Request-ID in response header")
	}

	// Context and response should match
	if capturedRequestID != responseID {
		t.Errorf("Context request ID %q doesn't match response header %q", capturedRequestID, responseID)
	}
}

func TestRequestID_UsesClientProvidedID(t *testing.T) {
	clientID := "client-provided-id-123"
	var capturedRequestID string

	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRequestID = logging.RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(RequestIDHeader, clientID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should use client-provided ID
	if capturedRequestID != clientID {
		t.Errorf("Expected request ID %q, got %q", clientID, capturedRequestID)
	}

	// Response header should have same ID
	responseID := rec.Header().Get(RequestIDHeader)
	if responseID != clientID {
		t.Errorf("Expected response header %q, got %q", clientID, responseID)
	}
}

func TestRequestID_GeneratesValidUUID(t *testing.T) {
	var capturedRequestID string

	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRequestID = logging.RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// UUID v4 format: 8-4-4-4-12 = 36 chars with hyphens
	if len(capturedRequestID) != 36 {
		t.Errorf("Expected UUID length 36, got %d: %q", len(capturedRequestID), capturedRequestID)
	}
}

func TestRequestID_UniquePerRequest(t *testing.T) {
	ids := make(map[string]bool)

	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := logging.RequestIDFromContext(r.Context())
		if ids[id] {
			t.Errorf("Duplicate request ID generated: %s", id)
		}
		ids[id] = true
		w.WriteHeader(http.StatusOK)
	}))

	// Make multiple requests
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	if len(ids) != 100 {
		t.Errorf("Expected 100 unique IDs, got %d", len(ids))
	}
}

func TestRequestIDFunc(t *testing.T) {
	var capturedRequestID string

	handler := RequestIDFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRequestID = logging.RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if capturedRequestID == "" {
		t.Error("Expected request ID to be generated")
	}
}
