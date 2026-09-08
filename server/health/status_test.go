package health

import "testing"

func TestStatus_IsHealthy_Empty(t *testing.T) {
	s := &Status{}
	if !s.IsHealthy() {
		t.Error("IsHealthy() = false, want true for empty Error")
	}
}

func TestStatus_IsHealthy_NonEmpty(t *testing.T) {
	s := &Status{Error: "connection refused"}
	if s.IsHealthy() {
		t.Error("IsHealthy() = true, want false for non-empty Error")
	}
}

func TestStatus_AllFieldsRoundTrip(t *testing.T) {
	s := Status{
		Name:      "database",
		Backend:   "postgresql",
		LatencyMs: 42,
		Metadata:  map[string]string{"pool_size": "10"},
		Error:     "",
	}
	if s.Name != "database" {
		t.Errorf("Name = %q, want %q", s.Name, "database")
	}
	if s.Backend != "postgresql" {
		t.Errorf("Backend = %q, want %q", s.Backend, "postgresql")
	}
	if s.LatencyMs != 42 {
		t.Errorf("LatencyMs = %d, want 42", s.LatencyMs)
	}
	if s.Metadata["pool_size"] != "10" {
		t.Errorf("Metadata[pool_size] = %q, want %q", s.Metadata["pool_size"], "10")
	}
	if s.Error != "" {
		t.Errorf("Error = %q, want empty", s.Error)
	}
}
