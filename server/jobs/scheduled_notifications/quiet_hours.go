package scheduled_notifications

import (
	"time"

	"go.ripls.org/ripls/server/quiethours"
)

// QuietHoursStartHour is the local-time hour at which quiet hours begin
// (inclusive). Canonical definition lives in server/quiethours.
const QuietHoursStartHour = quiethours.StartHour

// QuietHoursEndHour is the local-time hour at which quiet hours end (exclusive).
const QuietHoursEndHour = quiethours.EndHour

// InQuietHours reports whether the given moment is inside the quiet window for
// the named IANA timezone. Thin wrapper over [quiethours.InWindow] so the
// scheduled dispatcher and the real-time notification service share one
// definition.
func InQuietHours(timezone string, t time.Time) bool {
	return quiethours.InWindow(timezone, t)
}
