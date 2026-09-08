package web

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
)

// TestFormatEventWhen covers the label variants the event landing renders,
// in particular the #2621 rules: a known timezone localizes the labels, an
// unknown one (empty or the legacy "UTC" fallback literal) degrades to
// date-only, and all-day events never show a wall-clock time.
func TestFormatEventWhen(t *testing.T) {
	ctx := context.Background()
	loc, err := l10n.NewLocalizerForContext(ctx) // default locale (en)
	if err != nil {
		t.Fatalf("build localizer: %v", err)
	}
	// 2027-07-02 00:00 UTC — the live-report shape: midnight UTC, which is
	// the previous evening in US timezones.
	midnightUTC := time.Date(2027, 7, 2, 0, 0, 0, 0, time.UTC).Unix()

	specific := func(tz string, allDay bool) *models.ExperienceTime {
		return &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Specific{
				Specific: &models.SpecificTime{UnixTimestampSec: midnightUTC, Timezone: tz, IsAllDay: allDay},
			},
		}
	}

	tests := []struct {
		name              string
		et                *models.ExperienceTime
		wantDayLabel      string
		wantWhenShort     string
		wantWhenSummary   string
		wantTimeFormatted string
	}{
		{
			name:              "known timezone localizes day, date, and time",
			et:                specific("America/Denver", false),
			wantDayLabel:      "Thursday",
			wantWhenShort:     "Thu, Jul 1",
			wantWhenSummary:   "Thursday, July 1 · 6:00 PM",
			wantTimeFormatted: "6:00 PM",
		},
		{
			name:              "empty timezone degrades to date-only",
			et:                specific("", false),
			wantDayLabel:      "Friday",
			wantWhenShort:     "Fri, Jul 2",
			wantWhenSummary:   "Friday, July 2",
			wantTimeFormatted: "",
		},
		{
			name:              "legacy UTC literal degrades to date-only",
			et:                specific("UTC", false),
			wantDayLabel:      "Friday",
			wantWhenShort:     "Fri, Jul 2",
			wantWhenSummary:   "Friday, July 2",
			wantTimeFormatted: "",
		},
		{
			name:              "all-day never shows a time",
			et:                specific("America/Denver", true),
			wantDayLabel:      "Thursday",
			wantWhenShort:     "Thu, Jul 1",
			wantWhenSummary:   "Thursday, July 1",
			wantTimeFormatted: "",
		},
		{
			name: "range uses its timezone",
			et: &models.ExperienceTime{
				TimeType: &models.ExperienceTime_Range{
					Range: &models.TimeRange{
						StartUnixSec: midnightUTC,
						EndUnixSec:   midnightUTC + 7200,
						Timezone:     "America/Denver",
					},
				},
			},
			wantDayLabel:      "Thursday",
			wantWhenShort:     "Thu, Jul 1",
			wantWhenSummary:   "Thursday, July 1 · 6:00 PM",
			wantTimeFormatted: "6:00 PM",
		},
		{
			name:              "TBD",
			et:                &models.ExperienceTime{TimeType: &models.ExperienceTime_Tbd{Tbd: &models.TimeTBD{}}},
			wantDayLabel:      "Soon",
			wantWhenShort:     "Date TBD",
			wantWhenSummary:   "Date TBD",
			wantTimeFormatted: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dayLabel, whenShort, whenSummary, timeFormatted := formatEventWhen(ctx, loc, tt.et)
			if dayLabel != tt.wantDayLabel {
				t.Errorf("dayLabel = %q, want %q", dayLabel, tt.wantDayLabel)
			}
			if whenShort != tt.wantWhenShort {
				t.Errorf("whenShort = %q, want %q", whenShort, tt.wantWhenShort)
			}
			if whenSummary != tt.wantWhenSummary {
				t.Errorf("whenSummary = %q, want %q", whenSummary, tt.wantWhenSummary)
			}
			if timeFormatted != tt.wantTimeFormatted {
				t.Errorf("timeFormatted = %q, want %q", timeFormatted, tt.wantTimeFormatted)
			}
		})
	}
}

// TestFormatEventWhen_Spanish verifies the localized assembly: Spanish
// weekday/month names, the reordered "{Weekday}, {Day} de {Month}" summary
// format, the 12-hour clock with a Spanish AM/PM marker, and the TBD labels.
func TestFormatEventWhen_Spanish(t *testing.T) {
	ctx := l10n.WithAcceptLanguage(context.Background(), "es")
	loc, err := l10n.NewLocalizerForContext(ctx)
	if err != nil {
		t.Fatalf("build es localizer: %v", err)
	}
	midnightUTC := time.Date(2027, 7, 2, 0, 0, 0, 0, time.UTC).Unix()

	t.Run("full date with time", func(t *testing.T) {
		et := &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Specific{
				Specific: &models.SpecificTime{UnixTimestampSec: midnightUTC, Timezone: "America/Denver"},
			},
		}
		dayLabel, whenShort, whenSummary, timeFormatted := formatEventWhen(ctx, loc, et)
		// 2027-07-02 00:00 UTC is 2027-07-01 18:00 in Denver.
		assertEq(t, "dayLabel", dayLabel, "jueves")
		assertEq(t, "whenShort", whenShort, "jue, 1 jul")
		assertEq(t, "whenSummary", whenSummary, "jueves, 1 de julio · 6:00 p. m.")
		assertEq(t, "timeFormatted", timeFormatted, "6:00 p. m.")
	})

	t.Run("all-day date-only", func(t *testing.T) {
		et := &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Specific{
				Specific: &models.SpecificTime{UnixTimestampSec: midnightUTC, Timezone: "America/Denver", IsAllDay: true},
			},
		}
		_, _, whenSummary, timeFormatted := formatEventWhen(ctx, loc, et)
		assertEq(t, "whenSummary", whenSummary, "jueves, 1 de julio")
		assertEq(t, "timeFormatted", timeFormatted, "")
	})

	t.Run("TBD", func(t *testing.T) {
		et := &models.ExperienceTime{TimeType: &models.ExperienceTime_Tbd{Tbd: &models.TimeTBD{}}}
		dayLabel, whenShort, _, _ := formatEventWhen(ctx, loc, et)
		assertEq(t, "dayLabel", dayLabel, "Pronto")
		assertEq(t, "whenShort", whenShort, "Fecha por confirmar")
	})
}

func assertEq(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", field, got, want)
	}
}
