package eventtime

import (
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// ts is 2027-06-05 00:30 UTC — chosen so localizing to a US timezone moves
// the date back a day, which is exactly the failure mode #2621 fixed.
const ts = int64(1812155400)

func specific(tz string, allDay bool) *models.ExperienceTime {
	return &models.ExperienceTime{
		TimeType: &models.ExperienceTime_Specific{
			Specific: &models.SpecificTime{UnixTimestampSec: ts, Timezone: tz, IsAllDay: allDay},
		},
	}
}

func timeRange(tz string, allDay bool) *models.ExperienceTime {
	return &models.ExperienceTime{
		TimeType: &models.ExperienceTime_Range{
			Range: &models.TimeRange{StartUnixSec: ts, EndUnixSec: ts + 3600, Timezone: tz, IsAllDay: allDay},
		},
	}
}

func TestResolve_NoMoment(t *testing.T) {
	cases := map[string]*models.ExperienceTime{
		"nil":            nil,
		"tbd":            {TimeType: &models.ExperienceTime_Tbd{Tbd: &models.TimeTBD{}}},
		"empty specific": {TimeType: &models.ExperienceTime_Specific{Specific: &models.SpecificTime{}}},
		"empty range":    {TimeType: &models.ExperienceTime_Range{Range: &models.TimeRange{}}},
		"nil specific":   {TimeType: &models.ExperienceTime_Specific{}},
	}
	for name, et := range cases {
		t.Run(name, func(t *testing.T) {
			if _, ok := Resolve(et); ok {
				t.Errorf("Resolve(%s) ok = true, want false", name)
			}
		})
	}
}

func TestResolve_Timezones(t *testing.T) {
	tests := []struct {
		name            string
		et              *models.ExperienceTime
		wantHasTimezone bool
		wantDay         int // day-of-month after localization
		wantAllDay      bool
	}{
		{"specific with tz", specific("America/Denver", false), true, 4, false},
		{"specific empty tz", specific("", false), false, 5, false},
		{"specific literal UTC is unknown", specific("UTC", false), false, 5, false},
		{"specific invalid tz", specific("Not/AZone", false), false, 5, false},
		{"specific all-day", specific("America/Denver", true), true, 4, true},
		{"range with tz", timeRange("America/Denver", false), true, 4, false},
		{"range empty tz", timeRange("", false), false, 5, false},
		{"range all-day", timeRange("", true), false, 5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, ok := Resolve(tt.et)
			if !ok {
				t.Fatalf("Resolve ok = false, want true")
			}
			if m.HasTimezone != tt.wantHasTimezone {
				t.Errorf("HasTimezone = %v, want %v", m.HasTimezone, tt.wantHasTimezone)
			}
			if m.Time.Day() != tt.wantDay {
				t.Errorf("Day = %d, want %d (moment %v)", m.Time.Day(), tt.wantDay, m.Time)
			}
			if m.IsAllDay != tt.wantAllDay {
				t.Errorf("IsAllDay = %v, want %v", m.IsAllDay, tt.wantAllDay)
			}
		})
	}
}

func TestMoment_ShowsTimeOfDay(t *testing.T) {
	tests := []struct {
		name string
		m    Moment
		want bool
	}{
		{"tz known, timed", Moment{HasTimezone: true}, true},
		{"tz known, all-day", Moment{HasTimezone: true, IsAllDay: true}, false},
		{"tz unknown, timed", Moment{}, false},
	}
	for _, tt := range tests {
		if got := tt.m.ShowsTimeOfDay(); got != tt.want {
			t.Errorf("%s: ShowsTimeOfDay = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestResolve_LocalizedWallClock(t *testing.T) {
	m, ok := Resolve(specific("America/Denver", false))
	if !ok {
		t.Fatal("Resolve ok = false")
	}
	// 2027-06-05 00:30 UTC = 2027-06-04 18:30 MDT.
	if got := m.Time.Format("Mon Jan 2 3:04 PM"); got != "Fri Jun 4 6:30 PM" {
		t.Errorf("localized wall clock = %q, want %q", got, "Fri Jun 4 6:30 PM")
	}
	if _, offset := m.Time.Zone(); offset != -6*3600 {
		t.Errorf("zone offset = %d, want %d (MDT)", offset, -6*3600)
	}
	// The date rolled back across midnight — the day-of-week bug from the
	// live report (#2621).
	if m.Time.In(time.UTC).Day() == m.Time.Day() {
		t.Error("expected localization to change the calendar day for this instant")
	}
}
