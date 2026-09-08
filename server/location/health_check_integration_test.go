package location

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// getEnvOrSkip returns the value of an environment variable or skips the test if not set.
func getEnvOrSkip(t *testing.T, key string) string {
	value := os.Getenv(key)
	if value == "" {
		t.Skipf("skipping test: %s environment variable not set", key)
	}
	return value
}

// TestMapboxClient_CheckHealth_InvalidAPIKey verifies that the Mapbox health check
// reports an error when an invalid access token is provided.
func TestMapboxClient_CheckHealth_InvalidAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test in short mode")
	}

	// Create a Mapbox client with an invalid access token
	invalidToken := "pk.invalid-token-for-testing"
	client := NewMapboxClient(invalidToken)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Health check should return a status with Error field populated
	statuses, err := client.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("CheckHealth() returned unexpected Go error: %v", err)
	}

	if len(statuses) == 0 {
		t.Fatal("CheckHealth() returned empty status slice")
	}

	status := statuses[0]
	if status.Error == "" {
		t.Error("CheckHealth() should report error with invalid access token, but status.Error is empty")
	} else {
		t.Logf("CheckHealth correctly reported error with invalid token: %v", status.Error)
		// Verify the error mentions the status code, unauthorized, or malformed token
		errStr := strings.ToLower(status.Error)
		if !strings.Contains(errStr, "401") && !strings.Contains(errStr, "403") && !strings.Contains(errStr, "unauthorized") && !strings.Contains(errStr, "invalid") && !strings.Contains(errStr, "malformed") {
			t.Errorf("Expected error to mention 401/403/unauthorized/invalid/malformed, got: %v", status.Error)
		}
	}
}

// TestMapboxClient_CheckHealth_ValidAPIKey verifies that the Mapbox health check
// succeeds with a valid access token (when available in test environment).
func TestMapboxClient_CheckHealth_ValidAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test in short mode")
	}

	// This test requires MAPBOX_ACCESS_TOKEN environment variable to be set
	// Skip if not available (e.g., in CI without secrets)
	accessToken := getEnvOrSkip(t, "MAPBOX_ACCESS_TOKEN")

	client := NewMapboxClient(accessToken)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	statuses, err := client.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("CheckHealth() returned unexpected Go error: %v", err)
	}

	if len(statuses) == 0 {
		t.Fatal("CheckHealth() returned empty status slice")
	}

	status := statuses[0]
	if status.Error != "" {
		t.Errorf("CheckHealth() reported error with valid access token: %v", status.Error)
	} else {
		t.Logf("CheckHealth succeeded: backend=%s, latency=%dms",
			status.Backend, status.LatencyMs)
	}
}
