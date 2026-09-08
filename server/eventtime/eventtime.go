package eventtime

import (
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// Moment is the display-ready interpretation of an ExperienceTime.
type Moment struct {
	// Time is the event's moment, localized to the event's timezone when
	// known and left in UTC otherwise.
	Time time.Time
	// HasTimezone reports whether Time is in the event's real local
	// timezone. When false the wall-clock time (and, near midnight UTC,
	// even the date) is unreliable — renderers must show date-only labels
	// rather than a confidently wrong "12:00 AM".
	HasTimezone bool
	// IsAllDay reports the stored all-day flag: the event has a date but
	// deliberately no time of day.
	IsAllDay bool
}

// ShowsTimeOfDay reports whether a renderer should include a wall-clock
// time: the timezone must be known and the event must not be all-day.
func (m Moment) ShowsTimeOfDay() bool {
	return m.HasTimezone && !m.IsAllDay
}

// Resolve interprets et for display. ok is false when et holds no concrete
// moment (nil, TBD, or an unset timestamp).
//
// A stored timezone of "" or the literal "UTC" is treated as unknown:
// creation paths historically persisted "UTC" as a fallback when the
// client's timezone was missing (#2621), and genuine UTC-timezone venues
// are essentially nonexistent, so the literal is overwhelmingly that
// fallback artifact. An invalid timezone name also resolves as unknown —
// this is display, not validation, and must never fail on bad stored data.
func Resolve(et *models.ExperienceTime) (m Moment, ok bool) {
	if et == nil {
		return Moment{}, false
	}
	var ts int64
	var tz string
	var allDay bool
	switch t := et.TimeType.(type) {
	case *models.ExperienceTime_Specific:
		if t.Specific == nil {
			return Moment{}, false
		}
		ts = t.Specific.UnixTimestampSec
		tz = t.Specific.Timezone
		allDay = t.Specific.IsAllDay
	case *models.ExperienceTime_Range:
		if t.Range == nil {
			return Moment{}, false
		}
		ts = t.Range.StartUnixSec
		tz = t.Range.Timezone
		allDay = t.Range.IsAllDay
	default:
		return Moment{}, false
	}
	if ts == 0 {
		return Moment{}, false
	}
	m = Moment{Time: time.Unix(ts, 0).UTC(), IsAllDay: allDay}
	if tz != "" && tz != "UTC" {
		if loc, err := time.LoadLocation(tz); err == nil {
			m.Time = m.Time.In(loc)
			m.HasTimezone = true
		}
	}
	return m, true
}
