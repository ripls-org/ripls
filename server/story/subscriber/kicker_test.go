package story_subscriber

import (
	"context"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// ptrInt64 returns a pointer to v. Used to set optional unix-seconds fields
// on Experience.
func ptrInt64(v int64) *int64 { return &v }

// ptrStr returns a pointer to s.
func ptrStr(s string) *string { return &s }

// 1700000000 is Tue, 14 Nov 2023 22:13:20 UTC. Day-of-week + month tokens
// the formatter uses are derived from time.Unix and are stable across
// time zones because the formatter forces UTC.
const fixedTS = int64(1700000000)

func TestFormatKickerDate(t *testing.T) {
	tests := []struct {
		name string
		exp  *models.Experience
		want string
	}{
		{
			name: "uses StartedAtUnixSec when set",
			exp:  &models.Experience{StartedAtUnixSec: ptrInt64(fixedTS)},
			want: "TUE · NOV 14",
		},
		{
			name: "falls back to CompletedAtUnixSec when started is zero",
			exp:  &models.Experience{CompletedAtUnixSec: ptrInt64(fixedTS)},
			want: "TUE · NOV 14",
		},
		{
			name: "prefers StartedAtUnixSec over CompletedAtUnixSec",
			exp: &models.Experience{
				StartedAtUnixSec:   ptrInt64(fixedTS),
				CompletedAtUnixSec: ptrInt64(fixedTS + 86400),
			},
			want: "TUE · NOV 14",
		},
		{
			name: "returns empty when neither timestamp is set",
			exp:  &models.Experience{},
			want: "",
		},
		{
			name: "treats zero StartedAt as unset and falls back",
			exp: &models.Experience{
				StartedAtUnixSec:   ptrInt64(0),
				CompletedAtUnixSec: ptrInt64(fixedTS),
			},
			want: "TUE · NOV 14",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatKickerDate(tt.exp); got != tt.want {
				t.Errorf("formatKickerDate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatKickerLocation_NoLocationID(t *testing.T) {
	exp := &models.Experience{}
	if got := formatKickerLocation(context.Background(), nil, exp); got != "" {
		t.Errorf("formatKickerLocation with no LocationId = %q, want empty", got)
	}
}

func TestFormatKickerLocation_PrefersName(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	loc := &models.Location{
		Name: ptrStr("Clear Creek"),
	}
	locID, err := s.Insert(context.Background(), loc)
	if err != nil {
		t.Fatalf("Insert location: %v", err)
	}
	exp := &models.Experience{LocationId: locID}
	got := formatKickerLocation(context.Background(), s, exp)
	if got != "CLEAR CREEK" {
		t.Errorf("formatKickerLocation = %q, want CLEAR CREEK", got)
	}
}

func TestFormatKickerLocation_FallsBackToLocalityRegion(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	loc := &models.Location{
		Address: &models.Address{
			Locality:           "Boulder",
			AdministrativeArea: ptrStr("CO"),
		},
	}
	locID, err := s.Insert(context.Background(), loc)
	if err != nil {
		t.Fatalf("Insert location: %v", err)
	}
	exp := &models.Experience{LocationId: locID}
	got := formatKickerLocation(context.Background(), s, exp)
	if got != "BOULDER, CO" {
		t.Errorf("formatKickerLocation = %q, want BOULDER, CO", got)
	}
}

func TestFormatKickerLocation_FallsBackToLocalityWhenNoRegion(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	loc := &models.Location{
		Address: &models.Address{Locality: "Berlin"},
	}
	locID, err := s.Insert(context.Background(), loc)
	if err != nil {
		t.Fatalf("Insert location: %v", err)
	}
	exp := &models.Experience{LocationId: locID}
	got := formatKickerLocation(context.Background(), s, exp)
	if got != "BERLIN" {
		t.Errorf("formatKickerLocation = %q, want BERLIN", got)
	}
}

func TestFormatKickerLocation_ReturnsEmptyOnLocationLookupFailure(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	exp := &models.Experience{LocationId: "nonexistent-location-id"}
	if got := formatKickerLocation(context.Background(), s, exp); got != "" {
		t.Errorf("formatKickerLocation with missing location = %q, want empty (best-effort)", got)
	}
}

func TestFormatStoryKicker_CombinesDateAndLocation(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	loc := &models.Location{
		Address: &models.Address{Locality: "Boulder", AdministrativeArea: ptrStr("CO")},
	}
	locID, _ := s.Insert(context.Background(), loc)
	exp := &models.Experience{
		StartedAtUnixSec: ptrInt64(fixedTS),
		LocationId:       locID,
	}
	got := formatStoryKicker(context.Background(), s, exp)
	if !strings.Contains(got, "TUE · NOV 14") || !strings.Contains(got, "BOULDER, CO") {
		t.Errorf("formatStoryKicker = %q, want it to include date and location", got)
	}
	if !strings.Contains(got, " · ") {
		t.Errorf("formatStoryKicker = %q, want segments joined by ' · '", got)
	}
}

func TestFormatStoryKicker_OmitsMissingSegments(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	// No timestamps and no location — kicker should be empty rather than
	// degenerate to a stray separator.
	exp := &models.Experience{}
	if got := formatStoryKicker(context.Background(), s, exp); got != "" {
		t.Errorf("formatStoryKicker with empty experience = %q, want empty", got)
	}
}
