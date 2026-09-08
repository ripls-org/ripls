package portfolio

import (
	"testing"
)

// TestIsoWeekKey verifies the YYYY-WW formatting contract.
func TestIsoWeekKey(t *testing.T) {
	tests := []struct {
		year, week int
		want       string
	}{
		{2024, 1, "2024-01"},
		{2024, 9, "2024-09"},
		{2024, 10, "2024-10"},
		{2024, 52, "2024-52"},
		{2025, 53, "2025-53"},
	}
	for _, tc := range tests {
		got := isoWeekKey(tc.year, tc.week)
		if got != tc.want {
			t.Errorf("isoWeekKey(%d, %d) = %q, want %q", tc.year, tc.week, got, tc.want)
		}
	}
}
