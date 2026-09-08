package integration_tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/storage"
)

// TestHealthEndpoint_Integration tests the /health endpoint with a running server.
// This verifies that all health checks work against live dependencies.
func TestHealthEndpoint_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Setup database
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	// Start server
	_, serverURL := startTestServer(t, dbURL)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("health_endpoint_returns_ok", func(t *testing.T) {
		resp, err := http.Get(serverURL + "/health")
		if err != nil {
			t.Fatalf("GET /health failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
		}

		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", ct)
		}

		// Parse response
		var healthResp map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Check overall health
		if healthy, ok := healthResp["healthy"].(bool); !ok || !healthy {
			t.Errorf("Expected healthy=true, got %v", healthResp["healthy"])
		}

		// Check version is present
		if version, ok := healthResp["version"].(string); !ok || version == "" {
			t.Errorf("Expected non-empty version, got %v", healthResp["version"])
		}

		// Check uptime is present (protojson encodes int64 as string)
		if healthResp["uptimeMs"] == nil {
			t.Error("Expected uptimeMs to be present")
		}

		// Check dependencies are present
		deps, ok := healthResp["dependencies"].([]any)
		if !ok {
			t.Fatalf("Expected dependencies array, got %T", healthResp["dependencies"])
		}

		t.Logf("Health check returned %d dependencies", len(deps))

		// Log each dependency status
		for _, d := range deps {
			dep := d.(map[string]any)
			name := dep["name"].(string)
			healthy := dep["healthy"].(bool)
			backend := ""
			if b, ok := dep["backend"].(string); ok {
				backend = b
			}
			latency := int64(0)
			if l, ok := dep["latencyMs"].(float64); ok {
				latency = int64(l)
			}
			errMsg := ""
			if e, ok := dep["error"].(string); ok {
				errMsg = e
			}

			if healthy {
				t.Logf("  ✓ %s (%s): healthy, latency=%dms", name, backend, latency)
			} else {
				t.Logf("  ✗ %s: UNHEALTHY - %s", name, errMsg)
			}
		}
	})

	t.Run("health_rpc_returns_ok", func(t *testing.T) {
		healthClient := apiconnect.NewHealthServiceClient(http.DefaultClient, serverURL)

		resp, err := healthClient.CheckHealth(ctx, connect.NewRequest(&api.CheckHealthRequest{}))
		if err != nil {
			t.Fatalf("CheckHealth RPC failed: %v", err)
		}

		if !resp.Msg.Healthy {
			t.Error("Expected healthy=true from RPC")
		}

		// Verify database is among the dependencies
		var foundDB bool
		for _, dep := range resp.Msg.Dependencies {
			if dep.Name == "database" {
				foundDB = true
				if !dep.Healthy {
					t.Errorf("Database dependency unhealthy: %s", dep.Error)
				}
				if dep.Backend != "postgresql" {
					t.Errorf("Expected database backend=postgresql, got %s", dep.Backend)
				}
			}
		}

		if !foundDB {
			t.Error("Expected to find 'database' dependency")
		}

		// Verify storage is among the dependencies
		var foundStorage bool
		for _, dep := range resp.Msg.Dependencies {
			if dep.Name == "storage" {
				foundStorage = true
				if !dep.Healthy {
					t.Errorf("Storage dependency unhealthy: %s", dep.Error)
				}
				// Local storage is used in tests
				if dep.Backend != "local" {
					t.Errorf("Expected storage backend=local, got %s", dep.Backend)
				}
			}
		}

		if !foundStorage {
			t.Error("Expected to find 'storage' dependency")
		}
	})

	t.Run("health_check_logs_completion", func(t *testing.T) {
		// Start server with log capture
		_, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)

		// Clear any startup logs
		time.Sleep(100 * time.Millisecond) //nolint:forbidigo // waits for live server startup before clearing log capture
		logCapture.Clear()

		// Make health check request
		resp, err := http.Get(serverURL + "/health")
		if err != nil {
			t.Fatalf("GET /health failed: %v", err)
		}
		resp.Body.Close()

		// Wait for log entry
		entry := waitForLogEntry(logCapture, func(e map[string]any) bool {
			msg, ok := e["message"].(string)
			return ok && msg == "health_check_complete"
		}, 2*time.Second)

		if entry == nil {
			t.Error("Expected 'health_check_complete' log entry")
			t.Logf("Available log entries: %d", logCapture.Count())
			for _, e := range logCapture.Entries() {
				t.Logf("  - %v", e["message"])
			}
		} else {
			t.Logf("Found health_check_complete log: healthy=%v, dependency_count=%v",
				entry["healthy"], entry["dependency_count"])
		}
	})
}
