// Package quiethours defines the local-time window during which the platform
// avoids sending time-sensitive notifications (push, SMS). It is the shared,
// dependency-free home for the window so both the scheduled-notification
// dispatcher and the real-time notification service can consult it without an
// import cycle.
package quiethours

import "time"

// StartHour is the local-time hour at which quiet hours begin (inclusive):
// a notification attempted at 21:59 still goes out; at 22:00 it does not.
const StartHour = 22

// EndHour is the local-time hour at which quiet hours end (exclusive):
// at 07:00 local, deliveries resume.
const EndHour = 7

// InWindow reports whether t falls inside the quiet window for the named IANA
// timezone. An empty or unparseable timezone disables quiet hours — better to
// notify than to silently drop because we couldn't parse the recipient's
// locale.
func InWindow(timezone string, t time.Time) bool {
	if timezone == "" {
		return false
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return false
	}
	hour := t.In(loc).Hour()
	return hour >= StartHour || hour < EndHour
}
