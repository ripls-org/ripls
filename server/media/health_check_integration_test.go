package media

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

// TestPexelsProvider_CheckHealth_InvalidAPIKey verifies that the Pexels health check
// reports an error when an invalid API key is provided. This test makes actual HTTP calls
// to the Pexels API to ensure the health check properly validates the API key.
//
// This test exists because Pexels CDN caches/allows some single-word queries without
// authentication, which could cause health checks to pass even with invalid keys.
func TestPexelsProvider_CheckHealth_InvalidAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test in short mode")
	}

	// Create a Pexels provider with an invalid API key
	invalidKey := "invalid-api-key-for-testing"
	client := NewPexelsClient(invalidKey)
	provider := NewPexelsProvider(client, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Health check should return a status with Error field populated
	statuses, err := provider.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("CheckHealth() returned unexpected Go error: %v", err)
	}

	if len(statuses) == 0 {
		t.Fatal("CheckHealth() returned empty status slice")
	}

	status := statuses[0]
	if status.Error == "" {
		t.Error("CheckHealth() should report error with invalid API key, but status.Error is empty")
	} else {
		t.Logf("CheckHealth correctly reported error with invalid key: %v", status.Error)
		// Verify the error mentions the status code or unauthorized
		errStr := strings.ToLower(status.Error)
		if !strings.Contains(errStr, "401") && !strings.Contains(errStr, "unauthorized") && !strings.Contains(errStr, "invalid") {
			t.Errorf("Expected error to mention 401/unauthorized/invalid, got: %v", status.Error)
		}
	}
}

// TestUnsplashProvider_CheckHealth_InvalidAPIKey verifies that the Unsplash health check
// reports an error when an invalid API key is provided.
func TestUnsplashProvider_CheckHealth_InvalidAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test in short mode")
	}

	// Create an Unsplash provider with an invalid API key
	invalidKey := "invalid-access-key-for-testing"
	client := NewUnsplashClient(invalidKey)
	provider := NewUnsplashProvider(client, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Health check should return a status with Error field populated
	statuses, err := provider.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("CheckHealth() returned unexpected Go error: %v", err)
	}

	if len(statuses) == 0 {
		t.Fatal("CheckHealth() returned empty status slice")
	}

	status := statuses[0]
	if status.Error == "" {
		t.Error("CheckHealth() should report error with invalid API key, but status.Error is empty")
	} else {
		t.Logf("CheckHealth correctly reported error with invalid key: %v", status.Error)
		// Verify the error mentions the status code or unauthorized
		errStr := strings.ToLower(status.Error)
		if !strings.Contains(errStr, "401") && !strings.Contains(errStr, "unauthorized") && !strings.Contains(errStr, "invalid") && !strings.Contains(errStr, "oauth") {
			t.Errorf("Expected error to mention 401/unauthorized/invalid/oauth, got: %v", status.Error)
		}
	}
}

// TestPexelsProvider_CheckHealth_ValidAPIKey verifies that the Pexels health check
// succeeds with a valid API key (when available in test environment).
func TestPexelsProvider_CheckHealth_ValidAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test in short mode")
	}

	// This test requires PEXELS_API_KEY environment variable to be set
	// Skip if not available (e.g., in CI without secrets)
	apiKey := getEnvOrSkip(t, "PEXELS_API_KEY")

	client := NewPexelsClient(apiKey)
	provider := NewPexelsProvider(client, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	statuses, err := provider.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("CheckHealth() returned unexpected Go error: %v", err)
	}

	if len(statuses) == 0 {
		t.Fatal("CheckHealth() returned empty status slice")
	}

	status := statuses[0]
	if status.Error != "" {
		t.Errorf("CheckHealth() reported error with valid API key: %v", status.Error)
	} else {
		t.Logf("CheckHealth succeeded: backend=%s, latency=%dms",
			status.Backend, status.LatencyMs)
	}
}

// TestUnsplashProvider_CheckHealth_ValidAPIKey verifies that the Unsplash health check
// succeeds with a valid API key (when available in test environment).
func TestUnsplashProvider_CheckHealth_ValidAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test in short mode")
	}

	// This test requires UNSPLASH_ACCESS_KEY environment variable to be set
	// Skip if not available (e.g., in CI without secrets)
	apiKey := getEnvOrSkip(t, "UNSPLASH_ACCESS_KEY")

	client := NewUnsplashClient(apiKey)
	provider := NewUnsplashProvider(client, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	statuses, err := provider.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("CheckHealth() returned unexpected Go error: %v", err)
	}

	if len(statuses) == 0 {
		t.Fatal("CheckHealth() returned empty status slice")
	}

	status := statuses[0]
	if status.Error != "" {
		t.Errorf("CheckHealth() reported error with valid API key: %v", status.Error)
	} else {
		t.Logf("CheckHealth succeeded: backend=%s, latency=%dms",
			status.Backend, status.LatencyMs)
	}
}
