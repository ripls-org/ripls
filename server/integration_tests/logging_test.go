package integration_tests

// Integration tests for structured logging functionality.
// These tests verify that logging infrastructure works correctly end-to-end.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/middleware"
	"go.ripls.org/ripls/server/storage"
)

// TestLogging_RequestIDPropagation verifies that client-provided request IDs
// are preserved in logs and returned in response headers.
func TestLogging_RequestIDPropagation(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	// Make request with client-provided X-Request-ID
	clientRequestID := "client-test-request-123"
	req, err := http.NewRequest(http.MethodGet, serverURL+"/", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set(middleware.RequestIDHeader, clientRequestID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify response header has same request ID
	responseRequestID := resp.Header.Get(middleware.RequestIDHeader)
	if responseRequestID != clientRequestID {
		t.Errorf("Response X-Request-ID = %q, want %q", responseRequestID, clientRequestID)
	}

	// Wait for log entry with the request ID
	entry := waitForLogEntry(logCapture, func(e map[string]any) bool {
		return e["request_id"] == clientRequestID
	}, 2*time.Second)

	if entry == nil {
		t.Errorf("Expected log entry with request_id=%q, found none", clientRequestID)
		t.Logf("All entries: %v", logCapture.Entries())
	}
}

// TestLogging_RequestIDGeneration verifies that request IDs are generated
// when clients don't provide one.
func TestLogging_RequestIDGeneration(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	// Make request WITHOUT X-Request-ID
	req, err := http.NewRequest(http.MethodGet, serverURL+"/", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify response header has a generated request ID
	responseRequestID := resp.Header.Get(middleware.RequestIDHeader)
	if responseRequestID == "" {
		t.Fatal("Response should have X-Request-ID header with generated ID")
	}

	// UUID v4 format check (basic length check)
	if len(responseRequestID) != 36 {
		t.Errorf("Generated request ID should be UUID format (36 chars), got %d: %q",
			len(responseRequestID), responseRequestID)
	}

	// Wait for log entry with the generated request ID
	entry := waitForLogEntry(logCapture, func(e map[string]any) bool {
		return e["request_id"] == responseRequestID
	}, 2*time.Second)

	if entry == nil {
		t.Errorf("Expected log entry with generated request_id=%q, found none", responseRequestID)
	}
}

// TestLogging_HTTPRequestFields verifies that HTTP request logs contain
// expected structured fields.
func TestLogging_HTTPRequestFields(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	// Make a request
	requestID := "fields-test-456"
	req, err := http.NewRequest(http.MethodGet, serverURL+"/", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set(middleware.RequestIDHeader, requestID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Wait for the HTTP request log entry
	entry := waitForLogEntry(logCapture, func(e map[string]any) bool {
		return e["request_id"] == requestID && e["message"] == "http request"
	}, 2*time.Second)

	if entry == nil {
		t.Fatalf("No HTTP request log entry found for request_id=%q", requestID)
	}

	// Verify expected fields
	requiredFields := []string{"method", "path", "status", "duration_ms", "request_id"}
	for _, field := range requiredFields {
		if _, ok := entry[field]; !ok {
			t.Errorf("HTTP request log missing field %q: %v", field, entry)
		}
	}

	// Verify specific field values
	if entry["method"] != "GET" {
		t.Errorf("method = %v, want GET", entry["method"])
	}
	if entry["path"] != "/" {
		t.Errorf("path = %v, want /", entry["path"])
	}
}

// TestLogging_AuthenticatedRequestHasRequestID verifies that authenticated
// requests have request_id logged in both HTTP access logs and service-level logs.
// Also verifies that user_id appears in service-level logs (after auth middleware runs).
func TestLogging_AuthenticatedRequestHasRequestID(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	// Register a user
	userToken, userID := registerFirstUser(t, serverURL, "authlog-test@example.com", "Auth Log User")

	// Make an authenticated request with custom request ID
	// Use SaveGear which logs at Info level ("creating gear")
	authRequestID := "auth-request-202"
	gearClient := createGearClientWithRequestID(userToken, serverURL, authRequestID)
	ctx := context.Background()

	_, _ = gearClient.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
		Name:        proto.String("Test Gear for Logging"),
		Description: proto.String("Testing request ID propagation to service logs"),
	}))

	// Wait for HTTP access log entry
	httpEntry := waitForLogEntry(logCapture, func(e map[string]any) bool {
		return e["request_id"] == authRequestID && e["message"] == "http request"
	}, 2*time.Second)

	if httpEntry == nil {
		entries := logCapture.FindByRequestID(authRequestID)
		t.Errorf("Expected http request log entry for request_id=%q, entries: %v", authRequestID, entries)
	} else if httpEntry["status"] != float64(200) {
		// Verify it's a successful authenticated request
		t.Errorf("Expected status 200, got %v", httpEntry["status"])
	}

	// Verify service-level log has request_id AND user_id
	// The gear service logs "creating gear" at Info level
	serviceEntry := waitForLogEntry(logCapture, func(e map[string]any) bool {
		return e["request_id"] == authRequestID &&
			e["user_id"] == userID &&
			e["message"] == "creating gear"
	}, 2*time.Second)

	if serviceEntry == nil {
		entries := logCapture.FindByRequestID(authRequestID)
		t.Errorf("Expected service log 'creating gear' with request_id=%q and user_id=%q, found entries: %v",
			authRequestID, userID, entries)
	} else {
		// Verify user_email is present (masked form)
		if _, ok := serviceEntry["user_email"]; !ok {
			t.Errorf("Service log missing user_email field: %v", serviceEntry)
		}
		// Verify gear_name is present
		if serviceEntry["gear_name"] != "Test Gear for Logging" {
			t.Errorf("Service log gear_name = %v, want 'Test Gear for Logging'", serviceEntry["gear_name"])
		}
	}
}

// TestLogging_StartupMessages verifies that server startup logs expected messages.
func TestLogging_StartupMessages(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	// Server is ready, so startup messages should already be logged
	// Verify key startup messages are logged
	startupChecks := []struct {
		name    string
		checker func() bool
	}{
		{
			name:    "database initialization",
			checker: func() bool { return logCapture.HasEntry("message", "using PostgreSQL database") },
		},
		{
			name:    "server start",
			checker: func() bool { return logCapture.HasEntry("message", "starting server") },
		},
	}

	for _, check := range startupChecks {
		if !check.checker() {
			t.Errorf("Missing startup log: %s", check.name)
		}
	}

	// Verify we're using JSON format (all entries should be valid JSON)
	entries := logCapture.Entries()
	if len(entries) == 0 {
		t.Error("No log entries captured - logging may not be working")
	}

	// All entries should have standard Cloud Logging fields
	for i, entry := range entries {
		if _, ok := entry["time"]; !ok {
			t.Errorf("Entry %d missing 'time' field", i)
		}
		if _, ok := entry["severity"]; !ok {
			t.Errorf("Entry %d missing 'severity' field", i)
		}
		if _, ok := entry["message"]; !ok {
			t.Errorf("Entry %d missing 'message' field", i)
		}
	}

	_ = serverURL // Used for setup
}

// TestLogging_MultipleRequestsHaveUniqueIDs verifies that each request gets
// a unique ID when not provided by the client.
func TestLogging_MultipleRequestsHaveUniqueIDs(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, _ := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	// Make multiple requests without X-Request-ID
	requestIDs := make(map[string]bool)
	for i := 0; i < 5; i++ {
		req, err := http.NewRequest(http.MethodGet, serverURL+"/", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}

		responseRequestID := resp.Header.Get(middleware.RequestIDHeader)
		resp.Body.Close()

		if responseRequestID == "" {
			t.Errorf("Request %d: missing X-Request-ID in response", i)
			continue
		}

		if requestIDs[responseRequestID] {
			t.Errorf("Request %d: duplicate request ID %q", i, responseRequestID)
		}
		requestIDs[responseRequestID] = true
	}

	if len(requestIDs) != 5 {
		t.Errorf("Expected 5 unique request IDs, got %d", len(requestIDs))
	}
}
