//go:build integration

package ai

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

// isVendorUnreachable reports whether a CheckHealth error string looks like a
// runner-side reachability failure (context deadline, network timeout, TCP
// reset) rather than a real defect in the provider wiring. Live-vendor tests
// use this to skip when the vendor is momentarily unreachable — the required
// Go Tests gate must not fail because of third-party latency (#2865).
func isVendorUnreachable(errStr string) bool {
	s := strings.ToLower(errStr)
	return strings.Contains(s, "timeout") ||
		strings.Contains(s, "deadline") ||
		strings.Contains(s, "connection")
}

// TestAnthropicProvider_CheckHealth_InvalidAPIKey verifies that the Anthropic health check
// reports an error when an invalid API key is provided.
func TestAnthropicProvider_CheckHealth_InvalidAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test in short mode")
	}

	// Create an Anthropic provider with an invalid API key
	invalidKey := "sk-ant-invalid-api-key-for-testing"
	provider, err := NewAnthropicProvider(invalidKey, "", DefaultTemperature)
	if err != nil {
		t.Fatalf("NewAnthropicProvider() failed: %v", err)
	}

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
		// Skip the auth-content assertion when the API endpoint is unreachable from
		// this runner (timeout / connection failure). The important contract — that
		// an error is reported at all — is already checked above.
		if isVendorUnreachable(status.Error) {
			t.Skipf("Anthropic API not reachable from this runner, skipping auth-error content check: %v", status.Error)
		}
		errStr := strings.ToLower(status.Error)
		if !strings.Contains(errStr, "401") && !strings.Contains(errStr, "403") && !strings.Contains(errStr, "unauthorized") && !strings.Contains(errStr, "invalid") && !strings.Contains(errStr, "authentication") {
			t.Errorf("Expected error to mention authentication failure, got: %v", status.Error)
		}
	}
}

// TestOpenAIProvider_CheckHealth_InvalidAPIKey verifies that the OpenAI health check
// reports an error when an invalid API key is provided.
func TestOpenAIProvider_CheckHealth_InvalidAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test in short mode")
	}

	// Create an OpenAI provider with an invalid API key
	invalidKey := "sk-invalid-api-key-for-testing"
	provider, err := NewOpenAIProvider(invalidKey, "", DefaultTemperature)
	if err != nil {
		t.Fatalf("NewOpenAIProvider() failed: %v", err)
	}

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
		// Skip the auth-content assertion when the API endpoint is unreachable
		// from this runner. The important contract — that an error is reported
		// at all — is already checked above.
		if isVendorUnreachable(status.Error) {
			t.Skipf("OpenAI API not reachable from this runner, skipping auth-error content check: %v", status.Error)
		}
		errStr := strings.ToLower(status.Error)
		if !strings.Contains(errStr, "401") && !strings.Contains(errStr, "403") && !strings.Contains(errStr, "unauthorized") && !strings.Contains(errStr, "invalid") && !strings.Contains(errStr, "authentication") {
			t.Errorf("Expected error to mention authentication failure, got: %v", status.Error)
		}
	}
}

// TestAnthropicProvider_CheckHealth_ValidAPIKey verifies that the Anthropic health check
// succeeds with a valid API key (when available in test environment).
func TestAnthropicProvider_CheckHealth_ValidAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test in short mode")
	}

	// This test requires ANTHROPIC_API_KEY environment variable to be set
	apiKey := getEnvOrSkip(t, "ANTHROPIC_API_KEY")

	provider, err := NewAnthropicProvider(apiKey, "", DefaultTemperature)
	if err != nil {
		t.Fatalf("NewAnthropicProvider() failed: %v", err)
	}

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
		// A timeout / connection error is a runner-side reachability problem,
		// not a defect in the change under test. Skip so the required Go Tests
		// gate is not blocked by third-party latency (#2865).
		if isVendorUnreachable(status.Error) {
			t.Skipf("Anthropic API not reachable from this runner: %v", status.Error)
		}
		t.Errorf("CheckHealth() reported error with valid API key: %v", status.Error)
	} else {
		t.Logf("CheckHealth succeeded: backend=%s, latency=%dms",
			status.Backend, status.LatencyMs)
	}
}

// TestOpenAIProvider_CheckHealth_ValidAPIKey verifies that the OpenAI health check
// succeeds with a valid API key (when available in test environment).
func TestOpenAIProvider_CheckHealth_ValidAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test in short mode")
	}

	// This test requires OPENAI_API_KEY environment variable to be set
	apiKey := getEnvOrSkip(t, "OPENAI_API_KEY")

	provider, err := NewOpenAIProvider(apiKey, "", DefaultTemperature)
	if err != nil {
		t.Fatalf("NewOpenAIProvider() failed: %v", err)
	}

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
		// A timeout / connection error is a runner-side reachability problem,
		// not a defect in the change under test. Skip so the required Go Tests
		// gate is not blocked by third-party latency (#2865).
		if isVendorUnreachable(status.Error) {
			t.Skipf("OpenAI API not reachable from this runner: %v", status.Error)
		}
		t.Errorf("CheckHealth() reported error with valid API key: %v", status.Error)
	} else {
		t.Logf("CheckHealth succeeded: backend=%s, latency=%dms",
			status.Backend, status.LatencyMs)
	}
}

// Note: GeminiProvider uses GCP Application Default Credentials (ADC) rather than
// an API key, so testing with invalid credentials requires mocking the GCP client
// or using invalid project IDs, which is not straightforward. The Gemini health
// check validation is covered by integration tests that run in the CI environment
// with valid credentials.

// TestFallbackProvider_CheckHealth_Composite_Live verifies the composite health
// behaviour under real API conditions: a fallback chain with one working provider
// and one deliberately-broken provider must report composite healthy and must not
// emit a "dependency_health_check_failed" log (which would trip the Cloud Monitoring
// alert). Requires ANTHROPIC_API_KEY to be set; skips otherwise.
func TestFallbackProvider_CheckHealth_Composite_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test in short mode")
	}

	apiKey := getEnvOrSkip(t, "ANTHROPIC_API_KEY")

	// Primary: Anthropic with a valid API key.
	primary, err := NewAnthropicProvider(apiKey, "", DefaultTemperature)
	if err != nil {
		t.Fatalf("NewAnthropicProvider() failed: %v", err)
	}

	// Tertiary: OpenAI with a deliberately-broken API key — simulates the
	// production scenario where the tertiary fallback is momentarily unhealthy.
	broken, err := NewOpenAIProvider("sk-invalid-key-for-composite-health-test", "", DefaultTemperature)
	if err != nil {
		t.Fatalf("NewOpenAIProvider() failed: %v", err)
	}

	fb, err := NewFallbackProvider(primary, []Provider{broken})
	if err != nil {
		t.Fatalf("NewFallbackProvider() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	statuses, err := fb.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("CheckHealth() returned unexpected Go error: %v", err)
	}

	if len(statuses) != 1 {
		t.Fatalf("expected 1 composite status, got %d", len(statuses))
	}

	s := statuses[0]

	// The primary (Anthropic) is healthy, so the composite must be healthy
	// despite the broken tertiary.
	if !s.IsHealthy() {
		// If the primary call timed out, skip rather than fail — the runner
		// may not have outbound access to the Anthropic API (#2865).
		if isVendorUnreachable(s.Error) {
			t.Skipf("Anthropic API not reachable from this runner: %v", s.Error)
		}
		t.Errorf("expected composite healthy with working primary, got error: %v", s.Error)
	}

	// The composite backend label should contain "+fallback".
	if !strings.Contains(s.Backend, "+fallback") {
		t.Errorf("expected Backend to contain +fallback, got %q", s.Backend)
	}

	// Metadata must reflect the primary as healthy.
	if s.Metadata["primary_healthy"] != "true" {
		t.Errorf("expected primary_healthy=true in metadata, got %q", s.Metadata["primary_healthy"])
	}

	t.Logf("composite status: backend=%s, primary_healthy=%s, healthy_fallback_count=%s, latency=%dms",
		s.Backend, s.Metadata["primary_healthy"], s.Metadata["healthy_fallback_count"], s.LatencyMs)
}
