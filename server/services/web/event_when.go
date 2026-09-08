package web

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.ripls.org/ripls/server/eventtime"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
)

// formatEventWhen produces the four localized time-label variants used in the
// event template. Returns the localized "Date TBD"/"Soon" labels when the
// experience time is unset or in TBD mode. The wall-clock time (TimeFormatted,
// and the time segment of WhenSummary) is included only when the event's
// timezone is actually known and the event isn't all-day — a UTC-rendered
// "12:00 AM" is confidently wrong (#2621).
//
// Go's time.Format is not locale-aware, so weekday/month names come from the
// server l10n catalog (web.when.*) and the *_format catalog strings assemble
// them per locale (Spanish reorders day/month and inserts "de"). The clock is
// 12-hour in both locales with a localized AM/PM marker.
func formatEventWhen(ctx context.Context, loc *l10n.Localizer, et *models.ExperienceTime) (dayLabel, whenShort, whenSummary, timeFormatted string) {
	m, ok := eventtime.Resolve(et)
	if !ok {
		tbd := loc.T(ctx, "web.when.date_tbd", nil)
		return loc.T(ctx, "web.when.soon", nil), tbd, tbd, ""
	}

	wdKey := strings.ToLower(m.Time.Weekday().String()) // "Saturday" -> "saturday"
	moKey := strings.ToLower(m.Time.Month().String())   // "May" -> "may"
	weekdayFull := loc.T(ctx, "web.when.weekday."+wdKey, nil)
	weekdayAbbr := loc.T(ctx, "web.when.weekday_abbr."+wdKey, nil)
	monthFull := loc.T(ctx, "web.when.month."+moKey, nil)
	monthAbbr := loc.T(ctx, "web.when.month_abbr."+moKey, nil)
	day := m.Time.Day()

	dayLabel = weekdayFull
	whenShort = loc.T(ctx, "web.when.short_format", map[string]any{
		"WeekdayAbbr": weekdayAbbr, "MonthAbbr": monthAbbr, "Day": day,
	})
	whenSummary = loc.T(ctx, "web.when.summary_format", map[string]any{
		"Weekday": weekdayFull, "Month": monthFull, "Day": day,
	})
	if m.ShowsTimeOfDay() {
		timeFormatted = formatClock(ctx, loc, m.Time)
		whenSummary = fmt.Sprintf("%s · %s", whenSummary, timeFormatted)
	}
	return dayLabel, whenShort, whenSummary, timeFormatted
}

// formatClock renders a 12-hour wall-clock time ("6:00 PM") with a localized
// AM/PM marker. 12h (rather than a per-locale 12/24 split) keeps the English
// output byte-identical to the previous time.Format("3:04 PM") and avoids
// coupling this code to a locale allow-list; only the marker is translated.
func formatClock(ctx context.Context, loc *l10n.Localizer, t time.Time) string {
	h := t.Hour() % 12
	if h == 0 {
		h = 12
	}
	markerKey := "web.when.am"
	if t.Hour() >= 12 {
		markerKey = "web.when.pm"
	}
	return fmt.Sprintf("%d:%02d %s", h, t.Minute(), loc.T(ctx, markerKey, nil))
}
