// Package health defines types for health checking.
package health

// Status is returned by CheckHealth implementations to describe their health.
// Providers should always return a Status, populating the Error field on failure
// rather than returning a Go error. This ensures backend information is always
// available for logging and alerting.
type Status struct {
	// Name identifies the dependency category (e.g., "ai", "stock_imagery", "database").
	// For composite providers that return multiple statuses, each status should use
	// the same category name with different backends.
	Name string

	// Backend identifies the specific implementation (e.g., "anthropic", "gemini", "pexels").
	Backend string

	// LatencyMs is the time taken to check this specific dependency (milliseconds).
	// Each provider should measure and report its own latency.
	LatencyMs int64

	// Metadata contains additional details (e.g., bucket name, connection pool stats).
	Metadata map[string]string

	// Error contains the error message if the health check failed.
	// Empty string indicates the check passed.
	Error string
}

// IsHealthy returns true if the status has no error.
func (s *Status) IsHealthy() bool {
	return s.Error == ""
}
