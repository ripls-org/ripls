package scheduled_notifications

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications"
)

// DispatchStorage is the storage surface the dispatch driver needs.
// Defined as an interface so tests can pass an in-memory fake.
type DispatchStorage interface {
	FindDueScheduledNotifications(ctx context.Context, nowUnixSec int64, limit int) ([]*models.ScheduledNotification, error)
	ClaimAndDeleteScheduledNotification(ctx context.Context, id string) (bool, error)
}

// Dispatch runs one dispatcher tick: pull due rows, defer rows in
// recipient quiet hours, route each remaining row to the matching
// per-item-type Dispatcher, claim atomically, render, and deliver.
//
// The claim happens BEFORE the send, so a provider failure leaves
// the row gone and the push lost — we accept this on the grounds
// that FCM is reliable and crash-mid-send is rare, per the design.
func Dispatch(
	ctx context.Context,
	store DispatchStorage,
	notif notifications.Service,
	dispatchers []Dispatcher,
	batchLimit int,
) (DispatchStats, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ScheduledNotificationDispatch",
	)
	logger.InfoContext(ctx, "dispatch tick starting")

	now := clock.UnixSec(ctx)
	due, err := store.FindDueScheduledNotifications(ctx, now, batchLimit)
	if err != nil {
		return DispatchStats{}, fmt.Errorf("find due: %w", err)
	}

	var stats DispatchStats
	for _, row := range due {
		perRow := logger.With(
			"scheduled_notification_id", row.Id,
			"recipient_user_id", row.RecipientUserId,
			"fire_at_unix_sec", row.FireAtUnixSec,
		)

		matched := matchingDispatcher(dispatchers, row)
		if matched == nil {
			stats.UnknownItemType++
			perRow.ErrorContext(ctx, "no dispatcher matched row's item variant")
			continue
		}
		perRow = perRow.With("dispatcher", matched.Name())

		// Quiet-hours handling. The dispatcher's QuietHoursPolicy
		// chooses between deferring (day-grained reminders that
		// tolerate a delay until 07:00 local) and skipping (short-
		// lead reminders whose copy would be misleading after a
		// multi-hour delay — better to fire nothing than fire wrong).
		if row.QuietHoursTimezone != nil && InQuietHours(*row.QuietHoursTimezone, clock.Now(ctx)) {
			switch matched.QuietHoursPolicy(row) {
			case QuietHoursPolicyDefer:
				stats.QuietHoursDeferred++
				perRow.DebugContext(ctx, "deferring row for quiet hours",
					"quiet_hours_timezone", *row.QuietHoursTimezone,
				)
				continue
			case QuietHoursPolicySkip:
				// Atomic claim-and-delete with no send. If another
				// runner already deleted the row, that's fine — the
				// outcome (no push) is identical.
				claimed, err := store.ClaimAndDeleteScheduledNotification(ctx, row.Id)
				if err != nil {
					perRow.ErrorContext(ctx, "quiet-hours skip claim failed", "error", err)
					continue
				}
				if claimed {
					stats.QuietHoursSkipped++
					perRow.InfoContext(ctx, "skipping row due to quiet hours; copy would be misleading after delay",
						"quiet_hours_timezone", *row.QuietHoursTimezone,
					)
				} else {
					stats.ClaimRaceLost++
				}
				continue
			}
		}

		// Atomic claim-and-delete. Losing this race means another
		// runner is sending the push — nothing for us to do.
		claimed, err := store.ClaimAndDeleteScheduledNotification(ctx, row.Id)
		if err != nil {
			stats.RenderFailed++
			perRow.ErrorContext(ctx, "claim failed", "error", err)
			continue
		}
		if !claimed {
			stats.ClaimRaceLost++
			perRow.DebugContext(ctx, "claim lost to a concurrent runner")
			continue
		}

		notification, err := matched.Render(ctx, row)
		if err != nil {
			stats.RenderFailed++
			perRow.ErrorContext(ctx, "render failed; push lost", "error", err)
			continue
		}
		if notification == nil {
			stats.RenderSkipped++
			perRow.InfoContext(ctx, "render skipped (entity gone or prefs gated)")
			continue
		}

		if err := notif.NotifyUser(ctx, row.RecipientUserId, notification); err != nil {
			stats.SendFailed++
			perRow.ErrorContext(ctx, "notify failed; push lost", "error", err)
			continue
		}
		stats.Sent++
		perRow.InfoContext(ctx, "scheduled notification delivered")
	}

	logger.InfoContext(ctx, "dispatch tick complete",
		"sent", stats.Sent,
		"quiet_hours_deferred", stats.QuietHoursDeferred,
		"quiet_hours_skipped", stats.QuietHoursSkipped,
		"unknown_item_type", stats.UnknownItemType,
		"claim_race_lost", stats.ClaimRaceLost,
		"render_skipped", stats.RenderSkipped,
		"render_failed", stats.RenderFailed,
		"send_failed", stats.SendFailed,
	)
	return stats, nil
}

// matchingDispatcher returns the first Dispatcher that claims the
// row, or nil if none does. Linear scan — the dispatcher list is
// small (one entry per item type).
func matchingDispatcher(dispatchers []Dispatcher, row *models.ScheduledNotification) Dispatcher {
	for _, d := range dispatchers {
		if d.CanHandle(row) {
			return d
		}
	}
	return nil
}
