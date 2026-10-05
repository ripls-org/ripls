package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.ripls.org/ripls/server/logging"
)

func TestLogging_BasicRequest(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewLogger(logging.Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	handler := Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if logEntry["method"] != "GET" {
		t.Errorf("Expected method 'GET', got %v", logEntry["method"])
	}
	if logEntry["path"] != "/api/test" {
		t.Errorf("Expected path '/api/test', got %v", logEntry["path"])
	}
	if logEntry["status"] != float64(200) {
		t.Errorf("Expected status 200, got %v", logEntry["status"])
	}
	if _, ok := logEntry["duration_ms"]; !ok {
		t.Error("Expected duration_ms to be present")
	}
}

func TestLogging_IncludesRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewLogger(logging.Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	// Chain with RequestID middleware
	handler := RequestID(Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(RequestIDHeader, "test-request-123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if logEntry["request_id"] != "test-request-123" {
		t.Errorf("Expected request_id 'test-request-123', got %v", logEntry["request_id"])
	}
}

func TestLogging_IncludesClientIP(t *testing.T) {
	tests := []struct {
		name   string
		xff    string
		wantIP string
	}{
		{name: "behind a proxy", xff: "203.0.113.7, 10.0.0.2", wantIP: "203.0.113.7"},
		{name: "direct", xff: "", wantIP: "192.0.2.1"}, // httptest's RemoteAddr
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := logging.NewLogger(logging.Options{
				Level:  "info",
				Format: "json",
				Output: &buf,
			})

			// Chained as in routes.go: RemoteAddr resolves, Logging records.
			handler := RemoteAddr(Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			handler.ServeHTTP(httptest.NewRecorder(), req)

			var logEntry map[string]any
			if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
				t.Fatalf("Failed to parse JSON log: %v", err)
			}
			if logEntry["client_ip"] != tt.wantIP {
				t.Errorf("client_ip = %v, want %q", logEntry["client_ip"], tt.wantIP)
			}
			if logEntry["remote_addr"] != req.RemoteAddr {
				t.Errorf("remote_addr = %v, want the TCP peer %q", logEntry["remote_addr"], req.RemoteAddr)
			}
		})
	}
}

func TestLogging_OmitsClientIPWithoutRemoteAddr(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewLogger(logging.Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	handler := Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test", nil))

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}
	if _, ok := logEntry["client_ip"]; ok {
		t.Errorf("client_ip present without RemoteAddr middleware: %v", logEntry["client_ip"])
	}
}

func TestLogging_StatusCodes(t *testing.T) {
	tests := []struct {
		name             string
		expectedSeverity string
		status           int
	}{
		{"success 200", "INFO", http.StatusOK},
		{"success 201", "INFO", http.StatusCreated},
		{"client error 400", "WARNING", http.StatusBadRequest}, // Cloud Logging uses WARNING not WARN
		{"not found 404", "WARNING", http.StatusNotFound},      // Cloud Logging uses WARNING not WARN
		{"server error 500", "ERROR", http.StatusInternalServerError},
		{"service unavailable 503", "ERROR", http.StatusServiceUnavailable},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := logging.NewLogger(logging.Options{
				Level:  "debug",
				Format: "json",
				Output: &buf,
			})

			handler := Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			var logEntry map[string]any
			if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
				t.Fatalf("Failed to parse JSON log: %v", err)
			}

			// Cloud Logging uses "severity" not "level"
			if logEntry["severity"] != tc.expectedSeverity {
				t.Errorf("Expected severity %q for status %d, got %v", tc.expectedSeverity, tc.status, logEntry["severity"])
			}
		})
	}
}

func TestLogging_CapturesWrittenStatus(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewLogger(logging.Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	handler := Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if logEntry["status"] != float64(201) {
		t.Errorf("Expected status 201, got %v", logEntry["status"])
	}
}

func TestLogging_DefaultStatusOnWrite(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewLogger(logging.Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	// Handler that writes body without explicit WriteHeader
	handler := Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("response"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	// Should default to 200
	if logEntry["status"] != float64(200) {
		t.Errorf("Expected default status 200, got %v", logEntry["status"])
	}
}

func TestResponseWriter_Unwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	wrapped := &responseWriter{ResponseWriter: rec, status: http.StatusOK}

	unwrapped := wrapped.Unwrap()
	if unwrapped != rec {
		t.Error("Unwrap should return the underlying ResponseWriter")
	}
}

func TestExtractRPCMethod(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/ripls.api.GearService/SaveGear", "GearService/SaveGear"},
		{"/ripls.api.ChatService/ListConversations", "ChatService/ListConversations"},
		{"/ripls.api.LoginService/Login", "LoginService/Login"},
		{"/ripls.api.HealthService/CheckHealth", "HealthService/CheckHealth"},
		// Non-RPC paths return empty (no ripls.api package prefix)
		{"/health", ""},
		{"/media/file.jpg", ""},
		{"/debug/pprof/", ""},
		{"/", ""},
		{"", ""},
		{"/go/abc123", ""},
		// Scanner probes for dot-files must not become rpc_method label
		// values (#2622) — their first segment also contains a dot, which
		// the old heuristic accepted.
		{"/.git/config", ""},
		{"/.git/HEAD", ""},
		{"/.aws/credentials", ""},
		{"/.well-known/assetlinks.json", ""},
		{"/.well-known/acme-challenge/index.php", ""},
		{"/.vscode/sftp.json", ""},
		{"/.github/workflows/deploy.yml", ""},
		// Wrong or truncated package prefixes are not RPCs either.
		{"/ripls.api./Method", ""},
		{"/ripls.api.GearService/", ""},
		{"/ripls.api.GearService", ""},
		{"/ripls.api.GearService/Save/Extra", ""},
		{"/other.api.GearService/SaveGear", ""},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := extractRPCMethod(tc.path)
			if got != tc.expected {
				t.Errorf("extractRPCMethod(%q) = %q, want %q", tc.path, got, tc.expected)
			}
		})
	}
}

func TestLogging_ResponseBytesAndRPCMethod(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewLogger(logging.Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	body := []byte("hello world")
	handler := Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))

	req := httptest.NewRequest(http.MethodPost, "/ripls.api.GearService/SaveGear", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if logEntry["response_bytes"] != float64(len(body)) {
		t.Errorf("Expected response_bytes %d, got %v", len(body), logEntry["response_bytes"])
	}
	if logEntry["rpc_method"] != "GearService/SaveGear" {
		t.Errorf("Expected rpc_method 'GearService/SaveGear', got %v", logEntry["rpc_method"])
	}
}
