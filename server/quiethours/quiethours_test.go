package quiethours

import (
	"testing"
	"time"
)

func TestInWindow(t *testing.T) {
	denver, err := time.LoadLocation("America/Denver")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	cases := []struct {
		name string
		tz   string
		when time.Time
		want bool
	}{
		{"midnight is quiet", "America/Denver", time.Date(2026, 6, 26, 0, 30, 0, 0, denver), true},
		{"3am is quiet", "America/Denver", time.Date(2026, 6, 26, 3, 0, 0, 0, denver), true},
		{"22:00 is quiet (inclusive start)", "America/Denver", time.Date(2026, 6, 26, 22, 0, 0, 0, denver), true},
		{"21:59 is awake", "America/Denver", time.Date(2026, 6, 26, 21, 59, 0, 0, denver), false},
		{"07:00 is awake (exclusive end)", "America/Denver", time.Date(2026, 6, 26, 7, 0, 0, 0, denver), false},
		{"noon is awake", "America/Denver", time.Date(2026, 6, 26, 12, 0, 0, 0, denver), false},
		{"empty tz disables quiet hours", "", time.Date(2026, 6, 26, 3, 0, 0, 0, denver), false},
		{"unparseable tz disables quiet hours", "Not/AZone", time.Date(2026, 6, 26, 3, 0, 0, 0, denver), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := InWindow(tc.tz, tc.when); got != tc.want {
				t.Errorf("InWindow(%q, %v) = %v, want %v", tc.tz, tc.when, got, tc.want)
			}
		})
	}
}
