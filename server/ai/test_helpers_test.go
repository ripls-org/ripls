//go:build integration || benchmark

package ai

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

const (
	rateLimitMaxRetries    = 5
	rateLimitInitialDelay  = 2 * time.Second
	rateLimitBackoffFactor = 2.0
)

// retryOnRateLimit retries a function on rate limit errors with exponential backoff.
func retryOnRateLimit[T any](t *testing.T, name string, fn func() (T, error)) (T, error) {
	t.Helper()
	var lastErr error
	for attempt := range rateLimitMaxRetries {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		errMsg := strings.ToLower(err.Error())
		isRateLimit := strings.Contains(errMsg, "429") ||
			strings.Contains(errMsg, "rate limit") ||
			strings.Contains(errMsg, "resource exhausted") ||
			strings.Contains(errMsg, "quota")
		if !isRateLimit {
			return result, err
		}
		lastErr = err
		delay := time.Duration(float64(rateLimitInitialDelay) * math.Pow(rateLimitBackoffFactor, float64(attempt)))
		t.Logf("%s: rate limited (attempt %d/%d), retrying in %v: %v", name, attempt+1, rateLimitMaxRetries, delay, err)
		time.Sleep(delay) //nolint:forbidigo // exponential backoff between live API retries
	}
	var zero T
	return zero, fmt.Errorf("%s: exhausted %d retries on rate limit: %w", name, rateLimitMaxRetries, lastErr)
}
