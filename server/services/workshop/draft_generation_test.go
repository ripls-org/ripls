package workshop

import (
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// laTime parses a wall-clock time in America/Los_Angeles. The reels and the
// app both think in a real zone, and the DST cases below are only meaningful
// against one.
func laTime(t *testing.T, layout, value string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("load America/Los_Angeles: %v", err)
	}
	parsed, err := time.ParseInLocation(layout, value, loc)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return parsed
}

func specificTime(t *testing.T, when time.Time, tz string) *models.ExperienceTime {
	t.Helper()
	return &models.ExperienceTime{
		TimeType: &models.ExperienceTime_Specific{
			Specific: &models.SpecificTime{
				UnixTimestampSec: when.Unix(),
				Timezone:         tz,
			},
		},
	}
}

// TestNextWeeklySlot_SameWeekdayNextWeek — the base case a recurring group
// lives on: a Wednesday 6:30am run, wrapped up that morning, suggests the
// following Wednesday at 6:30am.
func TestNextWeeklySlot_SameWeekdayNextWeek(t *testing.T) {
	tz := "America/Los_Angeles"
	prior := laTime(t, "2006-01-02 15:04", "2026-08-12 06:30") // Wednesday
	now := laTime(t, "2006-01-02 15:04", "2026-08-12 09:30")   // same morning

	got, ok := nextWeeklySlot(specificTime(t, prior, tz), now)
	if !ok {
		t.Fatal("nextWeeklySlot: want a suggestion, got none")
	}
	want := laTime(t, "2006-01-02 15:04", "2026-08-19 06:30")
	if got != want.Unix() {
		t.Errorf("suggested %s, want %s",
			time.Unix(got, 0).In(want.Location()), want)
	}
}

// TestNextWeeklySlot_RollsPastMultipleWeeks — a host who lets three weeks
// lapse gets the next upcoming slot, not a date already gone by.
func TestNextWeeklySlot_RollsPastMultipleWeeks(t *testing.T) {
	tz := "America/Los_Angeles"
	prior := laTime(t, "2006-01-02 15:04", "2026-08-12 06:30")
	now := laTime(t, "2006-01-02 15:04", "2026-09-01 12:00") // ~3 weeks later

	got, ok := nextWeeklySlot(specificTime(t, prior, tz), now)
	if !ok {
		t.Fatal("nextWeeklySlot: want a suggestion, got none")
	}
	want := laTime(t, "2006-01-02 15:04", "2026-09-02 06:30") // next Wednesday
	if got != want.Unix() {
		t.Errorf("suggested %s, want %s",
			time.Unix(got, 0).In(want.Location()), want)
	}
}

// TestNextWeeklySlot_PreservesWallClockAcrossDST — stepping by calendar days
// in the event's own zone is the whole reason priorStart carries a timezone:
// a fixed +7*24h would move a 6:30am run to 5:30am the week the clocks change.
func TestNextWeeklySlot_PreservesWallClockAcrossDST(t *testing.T) {
	tz := "America/Los_Angeles"
	// 2026-10-28 is the Wednesday before the Nov 1 fall-back; 2026-11-04 is
	// the Wednesday after it.
	prior := laTime(t, "2006-01-02 15:04", "2026-10-28 06:30")
	now := laTime(t, "2006-01-02 15:04", "2026-10-28 09:00")

	got, ok := nextWeeklySlot(specificTime(t, prior, tz), now)
	if !ok {
		t.Fatal("nextWeeklySlot: want a suggestion, got none")
	}
	want := laTime(t, "2006-01-02 15:04", "2026-11-04 06:30")
	if got != want.Unix() {
		t.Errorf("suggested %s, want %s (wall clock must survive DST)",
			time.Unix(got, 0).In(want.Location()), want)
	}
	if delta := want.Unix() - prior.Unix(); delta == 7*24*3600 {
		t.Fatal("fixture no longer crosses a DST boundary — pick another week")
	}
}

// TestNextWeeklySlot_FutureInstanceUnchanged — a prior instance still ahead of
// now is already the next slot; suggesting it verbatim beats pushing the host
// a week past an event that hasn't happened.
func TestNextWeeklySlot_FutureInstanceUnchanged(t *testing.T) {
	tz := "America/Los_Angeles"
	prior := laTime(t, "2006-01-02 15:04", "2026-08-19 06:30")
	now := laTime(t, "2006-01-02 15:04", "2026-08-12 09:30")

	got, ok := nextWeeklySlot(specificTime(t, prior, tz), now)
	if !ok {
		t.Fatal("nextWeeklySlot: want a suggestion, got none")
	}
	if got != prior.Unix() {
		t.Errorf("suggested %s, want the prior instance's own start %s",
			time.Unix(got, 0).In(prior.Location()), prior)
	}
}

// TestNextWeeklySlot_TimeRange — range-timed events carry their start on a
// different field; the inference has to read it too.
func TestNextWeeklySlot_TimeRange(t *testing.T) {
	start := laTime(t, "2006-01-02 15:04", "2026-08-12 06:30")
	now := laTime(t, "2006-01-02 15:04", "2026-08-12 09:30")
	prior := &models.ExperienceTime{
		TimeType: &models.ExperienceTime_Range{
			Range: &models.TimeRange{
				StartUnixSec: start.Unix(),
				EndUnixSec:   start.Add(time.Hour).Unix(),
				Timezone:     "America/Los_Angeles",
			},
		},
	}

	got, ok := nextWeeklySlot(prior, now)
	if !ok {
		t.Fatal("nextWeeklySlot: want a suggestion for a range-timed event")
	}
	want := laTime(t, "2006-01-02 15:04", "2026-08-19 06:30")
	if got != want.Unix() {
		t.Errorf("suggested %s, want %s",
			time.Unix(got, 0).In(want.Location()), want)
	}
}

// TestNextWeeklySlot_NoInference — TBD, unset, and zero-start times leave the
// draft's time empty so the host picks one, rather than proposing the epoch.
func TestNextWeeklySlot_NoInference(t *testing.T) {
	now := laTime(t, "2006-01-02 15:04", "2026-08-12 09:30")
	cases := map[string]*models.ExperienceTime{
		"nil": nil,
		"tbd": {
			TimeType: &models.ExperienceTime_Tbd{Tbd: &models.TimeTBD{}},
		},
		"unset oneof": {},
		"zero start": {
			TimeType: &models.ExperienceTime_Specific{
				Specific: &models.SpecificTime{Timezone: "America/Los_Angeles"},
			},
		},
	}
	for name, prior := range cases {
		t.Run(name, func(t *testing.T) {
			if got, ok := nextWeeklySlot(prior, now); ok {
				t.Errorf("want no suggestion, got %d", got)
			}
		})
	}
}

// TestNextWeeklySlot_UnknownTimezoneFallsBackToUTC — a garbage or empty tz
// must not drop the suggestion; the hour is then held in UTC.
func TestNextWeeklySlot_UnknownTimezoneFallsBackToUTC(t *testing.T) {
	prior := time.Date(2026, 8, 12, 13, 30, 0, 0, time.UTC)
	now := time.Date(2026, 8, 12, 16, 30, 0, 0, time.UTC)

	for _, tz := range []string{"", "Mars/Olympus_Mons"} {
		got, ok := nextWeeklySlot(specificTime(t, prior, tz), now)
		if !ok {
			t.Fatalf("tz %q: want a suggestion, got none", tz)
		}
		want := prior.AddDate(0, 0, 7)
		if got != want.Unix() {
			t.Errorf("tz %q: suggested %s, want %s",
				tz, time.Unix(got, 0).UTC(), want)
		}
	}
}

// TestNextWeeklySlot_AncientInstanceGivesUp — a decade-old instance is a
// revival, not a cadence; bounding the loop keeps a corrupt row from spinning.
func TestNextWeeklySlot_AncientInstanceGivesUp(t *testing.T) {
	now := laTime(t, "2006-01-02 15:04", "2026-08-12 09:30")
	prior := now.AddDate(-11, 0, 0)

	if got, ok := nextWeeklySlot(specificTime(t, prior, "America/Los_Angeles"), now); ok {
		t.Errorf("want no suggestion for an 11-year-old instance, got %d", got)
	}
}
