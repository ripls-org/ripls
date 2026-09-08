package services

import (
	"testing"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestConvertTimeModelsToAPI verifies that all oneof variants and all fields
// survive the models→API conversion without data loss.
func TestConvertTimeModelsToAPI(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		got := ConvertTimeModelsToAPI(nil)
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("TBD variant", func(t *testing.T) {
		input := &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Tbd{Tbd: &models.TimeTBD{}},
		}
		got := ConvertTimeModelsToAPI(input)
		if got == nil {
			t.Fatal("expected non-nil result for TBD input")
		}
		if _, ok := got.TimeType.(*api.ExperienceTime_Tbd); !ok {
			t.Errorf("expected TBD variant, got %T", got.TimeType)
		}
	})

	t.Run("Specific variant copies all fields", func(t *testing.T) {
		input := &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Specific{
				Specific: &models.SpecificTime{
					UnixTimestampSec: 1_700_000_000,
					Timezone:         "America/Los_Angeles",
					DurationMinutes:  90,
					IsAllDay:         true,
				},
			},
		}
		got := ConvertTimeModelsToAPI(input)
		if got == nil {
			t.Fatal("expected non-nil result")
		}
		s, ok := got.TimeType.(*api.ExperienceTime_Specific)
		if !ok {
			t.Fatalf("expected Specific variant, got %T", got.TimeType)
		}
		if s.Specific.UnixTimestampSec != 1_700_000_000 {
			t.Errorf("UnixTimestampSec: got %d, want 1700000000", s.Specific.UnixTimestampSec)
		}
		if s.Specific.Timezone != "America/Los_Angeles" {
			t.Errorf("Timezone: got %q, want %q", s.Specific.Timezone, "America/Los_Angeles")
		}
		if s.Specific.DurationMinutes != 90 {
			t.Errorf("DurationMinutes: got %d, want 90", s.Specific.DurationMinutes)
		}
		if !s.Specific.IsAllDay {
			t.Errorf("IsAllDay: got false, want true")
		}
	})

	t.Run("Range variant copies all fields", func(t *testing.T) {
		input := &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Range{
				Range: &models.TimeRange{
					Description:     "next weekend",
					StartUnixSec:    1_700_000_000,
					EndUnixSec:      1_700_086_400,
					DurationMinutes: 120,
					IsAllDay:        true,
				},
			},
		}
		got := ConvertTimeModelsToAPI(input)
		if got == nil {
			t.Fatal("expected non-nil result")
		}
		r, ok := got.TimeType.(*api.ExperienceTime_Range)
		if !ok {
			t.Fatalf("expected Range variant, got %T", got.TimeType)
		}
		if r.Range.Description != "next weekend" {
			t.Errorf("Description: got %q, want %q", r.Range.Description, "next weekend")
		}
		if r.Range.StartUnixSec == nil || *r.Range.StartUnixSec != 1_700_000_000 {
			t.Errorf("StartUnixSec: got %v, want 1700000000", r.Range.StartUnixSec)
		}
		if r.Range.EndUnixSec == nil || *r.Range.EndUnixSec != 1_700_086_400 {
			t.Errorf("EndUnixSec: got %v, want 1700086400", r.Range.EndUnixSec)
		}
		if r.Range.DurationMinutes != 120 {
			t.Errorf("DurationMinutes: got %d, want 120", r.Range.DurationMinutes)
		}
		if !r.Range.IsAllDay {
			t.Errorf("IsAllDay: got false, want true")
		}
	})

	t.Run("Range StartUnixSec pointer is independent of input", func(t *testing.T) {
		// Verify that mutating the returned pointer does not affect a subsequent call.
		input := &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Range{
				Range: &models.TimeRange{
					StartUnixSec: 1_000,
					EndUnixSec:   2_000,
				},
			},
		}
		got1 := ConvertTimeModelsToAPI(input)
		got2 := ConvertTimeModelsToAPI(input)
		r1 := got1.TimeType.(*api.ExperienceTime_Range)
		r2 := got2.TimeType.(*api.ExperienceTime_Range)
		if r1.Range.StartUnixSec == r2.Range.StartUnixSec {
			t.Errorf("StartUnixSec pointers should be independent across calls")
		}
	})

	t.Run("unknown oneof variant returns TBD", func(t *testing.T) {
		// An ExperienceTime with no oneof set has a nil TimeType, which falls
		// through to the default case.
		input := &models.ExperienceTime{}
		got := ConvertTimeModelsToAPI(input)
		if got == nil {
			t.Fatal("expected non-nil result for unknown variant")
		}
		if _, ok := got.TimeType.(*api.ExperienceTime_Tbd); !ok {
			t.Errorf("expected TBD variant for unknown input, got %T", got.TimeType)
		}
	})
}
