package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.ripls.org/ripls/server/logging"
)

func newTestLogger(buf *bytes.Buffer) *logging.Logger {
	return logging.NewLogger(logging.Options{
		Level:  "debug",
		Format: "json",
		Output: buf,
	})
}

func TestPanicRecovery_NoPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	handler := PanicRecovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %q", rec.Body.String())
	}
	if buf.Len() != 0 {
		t.Errorf("expected no log output for non-panicking handler, got: %s", buf.String())
	}
}

func TestPanicRecovery_CatchesPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	handler := PanicRecovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went very wrong")
	}))

	req := httptest.NewRequest(http.MethodPost, "/ripls.api.GearService/SaveGear", nil)
	rec := httptest.NewRecorder()

	// Must not panic to the test runner.
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var errBody connectError
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("response body is not valid JSON: %v (body: %s)", err, rec.Body.String())
	}
	if errBody.Code != "internal" {
		t.Errorf("expected error code 'internal', got %q", errBody.Code)
	}
	if errBody.Message != "internal server error" {
		t.Errorf("expected message 'internal server error', got %q", errBody.Message)
	}
}

func TestPanicRecovery_LogsPanicDetails(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	panicValue := "nil pointer dereference"
	handler := PanicRecovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(panicValue)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("log output is not valid JSON: %v (output: %s)", err, buf.String())
	}

	if logEntry["severity"] != "ERROR" {
		t.Errorf("expected severity ERROR, got %v", logEntry["severity"])
	}
	if logEntry["panic"] != panicValue {
		t.Errorf("expected panic=%q in log, got %v", panicValue, logEntry["panic"])
	}
	if _, ok := logEntry["stack"]; !ok {
		t.Error("expected 'stack' field in log entry")
	}
}

func TestPanicRecovery_LogsRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	// Chain with RequestID so the ID is in context when the panic fires.
	handler := RequestID(PanicRecovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(RequestIDHeader, "test-req-xyz")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("log output is not valid JSON: %v (output: %s)", err, buf.String())
	}

	if logEntry["request_id"] != "test-req-xyz" {
		t.Errorf("expected request_id 'test-req-xyz', got %v", logEntry["request_id"])
	}
}

func TestPanicRecovery_RuntimePanic(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	// Simulate the most common production panic: nil pointer dereference.
	handler := PanicRecovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p *string
		_ = *p //nolint:govet // intentional nil dereference to trigger runtime panic in test
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for runtime panic, got %d", rec.Code)
	}
}
