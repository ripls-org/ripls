package scheduled_notifications

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// ReconcileStorage is the storage surface the reconcile driver needs.
// Defined as an interface so tests can pass an in-memory fake without
// spinning up Postgres.
type ReconcileStorage interface {
	// InsertScheduledNotificationIfAbsent inserts a row using
	// ON CONFLICT DO NOTHING and reports whether the row was actually
	// written. inserted=false (not an error) means another replica
	// already holds a row for this uniqueness key (#2458).
	InsertScheduledNotificationIfAbsent(ctx context.Context, row *models.ScheduledNotification) (inserted bool, id string, err error)
	Delete(ctx context.Context, msg proto.Message) error
	UpdateScheduledNotificationFireAt(ctx context.Context, row *models.ScheduledNotification, fireAtUnixSec, updatedAtUnixSec int64) (bool, error)
}

// Reconcile runs one reconciler tick: load Desired and Existing,
// diff by UniquenessKey, and apply inserts / fire-at updates /
// deletes through storage. Each per-row write is best-effort —
// errors are counted into ReconcileStats.Errored and the next tick
// gets another shot.
func Reconcile(ctx context.Context, store ReconcileStorage, r Reconciler) (ReconcileStats, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ScheduledNotificationReconcile",
		"reconciler", r.Name(),
	)
	logger.InfoContext(ctx, "reconcile tick starting")

	desired, err := r.Desired(ctx)
	if err != nil {
		return ReconcileStats{}, fmt.Errorf("desired: %w", err)
	}
	existing, err := r.Existing(ctx)
	if err != nil {
		return ReconcileStats{}, fmt.Errorf("existing: %w", err)
	}

	desiredByKey := make(map[string]*models.ScheduledNotification, len(desired))
	for _, d := range desired {
		key := UniquenessKey(d)
		if key == "" {
			logger.ErrorContext(ctx, "reconciler produced a tuple with no item variant; skipping")
			continue
		}
		// Last-write-wins within a single Desired() result. A
		// reconciler that emits duplicates is buggy; surface it as
		// an INFO log so it shows up but doesn't fail the tick.
		if _, dup := desiredByKey[key]; dup {
			logger.InfoContext(ctx, "reconciler emitted duplicate desired tuple",
				"key", key,
			)
		}
		desiredByKey[key] = d
	}

	now := clock.UnixSec(ctx)
	var stats ReconcileStats

	// Reap duplicates that slipped in via a multi-replica reconciler race
	// (#2458). Group all existing rows by uniqueness key; for each group
	// with >1 row where every row has a future fire_at (past-due rows
	// belong to the dispatcher), delete all but the smallest-id row.
	allByKey := make(map[string][]*models.ScheduledNotification, len(existing))
	for _, e := range existing {
		key := UniquenessKey(e)
		if key != "" {
			allByKey[key] = append(allByKey[key], e)
		}
	}
	reaped := make(map[string]bool) // ids deleted by the reaper this tick
	for key, rows := range allByKey {
		if len(rows) <= 1 {
			continue
		}
		// Only reap if every row in the group has a future fire_at.
		// A past-due row means the dispatcher already owns the group.
		allFuture := true
		for _, row := range rows {
			if row.FireAtUnixSec <= now {
				allFuture = false
				break
			}
		}
		if !allFuture {
			continue
		}
		// Keep the smallest-id row for determinism; the rows are
		// functionally equivalent (same recipient, purpose, offset,
		// fire_at, anchor).
		keeper := rows[0]
		for _, r := range rows[1:] {
			if r.Id < keeper.Id {
				keeper = r
			}
		}
		for _, discard := range rows {
			if discard.Id == keeper.Id {
				continue
			}
			if err := store.Delete(ctx, discard); err != nil {
				stats.Errored++
				logger.ErrorContext(ctx, "reaper delete failed",
					"key", key,
					"discarded_scheduled_notification_id", discard.Id,
					"error", err,
				)
				continue
			}
			reaped[discard.Id] = true
			logFields := []any{
				"recipient_user_id", discard.RecipientUserId,
				"outcome", "reaped_duplicate",
				"kept_scheduled_notification_id", keeper.Id,
				"discarded_scheduled_notification_id", discard.Id,
			}
			switch item := discard.Item.(type) {
			case *models.ScheduledNotification_Experience:
				if e := item.Experience; e != nil {
					logFields = append(logFields,
						"experience_id", e.ExperienceId,
						"purpose", e.Purpose,
						"offset_seconds_from_anchor", e.OffsetSecondsFromAnchor,
					)
				}
			case *models.ScheduledNotification_Loan:
				if l := item.Loan; l != nil {
					logFields = append(logFields,
						"transfer_id", l.TransferId,
						"purpose", l.Purpose,
						"offset_seconds_from_anchor", l.OffsetSecondsFromAnchor,
					)
				}
			case *models.ScheduledNotification_Request:
				if req := item.Request; req != nil {
					// target_request_id avoids collision with the HTTP
					// correlation request_id per server/conventions.md.
					logFields = append(logFields,
						"target_request_id", req.RequestId,
						"purpose", req.Purpose,
						"offset_seconds_from_anchor", req.OffsetSecondsFromAnchor,
					)
				}
			}
			logger.InfoContext(ctx, "reaped duplicate scheduled_notification", logFields...)
			stats.ReapedDuplicates++
		}
	}

	// Build existingByKey for the diff, skipping rows reaped above.
	existingByKey := make(map[string]*models.ScheduledNotification, len(existing))
	for _, e := range existing {
		key := UniquenessKey(e)
		if key == "" {
			// Malformed row with no item variant set; leave it in
			// place so an operator can investigate rather than
			// silently swallowing the data.
			logger.ErrorContext(ctx, "existing row has unset item variant; leaving in place",
				"scheduled_notification_id", e.Id,
			)
			continue
		}
		if reaped[e.Id] {
			continue
		}
		existingByKey[key] = e
	}

	// Inserts and fire-at updates.
	for key, d := range desiredByKey {
		ex, ok := existingByKey[key]
		if !ok {
			d.CreatedAtUnixSec = now
			d.UpdatedAtUnixSec = now
			inserted, _, err := store.InsertScheduledNotificationIfAbsent(ctx, d)
			if err != nil {
				stats.Errored++
				logger.ErrorContext(ctx, "insert failed", "key", key, "error", err)
				continue
			}
			if inserted {
				stats.Inserted++
			} else {
				// Another reconciler replica already holds this row
				// (ON CONFLICT DO NOTHING). Count as Unchanged — the
				// row exists in the desired shape, we just didn't write it.
				stats.Unchanged++
				logger.DebugContext(ctx, "insert skipped: row already written by another replica",
					"key", key,
				)
			}
			continue
		}
		if ex.FireAtUnixSec == d.FireAtUnixSec {
			stats.Unchanged++
			continue
		}
		updated, err := store.UpdateScheduledNotificationFireAt(ctx, ex, d.FireAtUnixSec, now)
		if err != nil {
			stats.Errored++
			logger.ErrorContext(ctx, "fire_at update failed", "key", key, "error", err)
			continue
		}
		if updated {
			stats.Updated++
		} else {
			// Row vanished between Existing() and the UPDATE — a
			// concurrent dispatcher claimed it. The next tick will
			// re-insert if the desired tuple still applies.
			stats.Unchanged++
		}
	}

	// Deletes for rows in storage but not in the desired set. Rows
	// whose fire_at is already in the past belong to the dispatcher —
	// it either fired them and they're gone, or it hasn't yet (e.g.
	// short dispatcher outage) and the reconciler must not race
	// ahead and delete them.
	for key, ex := range existingByKey {
		if _, ok := desiredByKey[key]; ok {
			continue
		}
		if ex.FireAtUnixSec <= now {
			continue
		}
		if err := store.Delete(ctx, ex); err != nil {
			stats.Errored++
			logger.ErrorContext(ctx, "delete failed",
				"key", key,
				"scheduled_notification_id", ex.Id,
				"error", err,
			)
			continue
		}
		stats.Deleted++
	}

	logger.InfoContext(ctx, "reconcile tick complete",
		"inserted", stats.Inserted,
		"updated", stats.Updated,
		"unchanged", stats.Unchanged,
		"deleted", stats.Deleted,
		"reaped_duplicates", stats.ReapedDuplicates,
		"errored", stats.Errored,
	)
	return stats, nil
}
