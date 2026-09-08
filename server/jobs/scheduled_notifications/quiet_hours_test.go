package scheduled_notifications

import (
	"testing"
	"time"
)

func TestInQuietHours(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("load tz: %v", err)
	}

	t.Run("3am local is in quiet hours", func(t *testing.T) {
		moment := time.Date(2026, 5, 15, 3, 0, 0, 0, loc).UTC()
		if !InQuietHours("America/Los_Angeles", moment) {
			t.Error("3am local should be in quiet hours")
		}
	})

	t.Run("10pm local is in quiet hours", func(t *testing.T) {
		moment := time.Date(2026, 5, 15, 22, 0, 0, 0, loc).UTC()
		if !InQuietHours("America/Los_Angeles", moment) {
			t.Error("10pm local should be in quiet hours")
		}
	})

	t.Run("7am local is the first allowed moment", func(t *testing.T) {
		moment := time.Date(2026, 5, 15, 7, 0, 0, 0, loc).UTC()
		if InQuietHours("America/Los_Angeles", moment) {
			t.Error("7:00am local should be allowed (exclusive end)")
		}
	})

	t.Run("9pm local is just inside the allowed window", func(t *testing.T) {
		moment := time.Date(2026, 5, 15, 21, 59, 0, 0, loc).UTC()
		if InQuietHours("America/Los_Angeles", moment) {
			t.Error("9:59pm local should be allowed")
		}
	})

	t.Run("empty timezone disables quiet hours", func(t *testing.T) {
		moment := time.Date(2026, 5, 15, 3, 0, 0, 0, loc).UTC()
		if InQuietHours("", moment) {
			t.Error("empty timezone should disable quiet hours")
		}
	})

	t.Run("unrecognized timezone falls back to delivering", func(t *testing.T) {
		moment := time.Date(2026, 5, 15, 3, 0, 0, 0, loc).UTC()
		if InQuietHours("Not/A/Real/Zone", moment) {
			t.Error("invalid timezone should not block delivery")
		}
	})

	t.Run("timezones with different local clocks evaluate differently", func(t *testing.T) {
		// 09:00 UTC = 02:00 PT (quiet hours) but 11:00 in Berlin (open).
		moment := time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC)
		if !InQuietHours("America/Los_Angeles", moment) {
			t.Error("PT recipient: 9 UTC = 2am local, expected quiet")
		}
		if InQuietHours("Europe/Berlin", moment) {
			t.Error("Berlin recipient: 9 UTC = 11am local, expected open")
		}
	})
}
