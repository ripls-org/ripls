# scheduled_notifications

Drives the reconciler/dispatcher pair that turns durable properties of
in-flight entities into push notifications fired at the right moment.

**Canonical design reference:**
[`docs/server/scheduled_notifications.md`](../../../docs/server/scheduled_notifications.md).
Read that first for the proto shape, lifecycle, uniqueness key,
quiet-hours behavior, preference gating, and the per-item-type pattern
— including a step-by-step "how to add a new kind" walkthrough.

## Key files

- `framework.go` — `Reconciler` and `Dispatcher` interfaces, the
  per-row `UniquenessKey` extraction, and the shared stats types.
- `reconcile.go` — `Reconcile(ctx, storage, reconciler)`: diffs the
  reconciler's desired-state set against current storage and applies
  inserts / fire-at updates / hard-deletes.
- `dispatch.go` — `Dispatch(ctx, storage, notif, dispatchers, limit)`:
  pulls due rows, applies quiet hours, routes each row to the
  matching `Dispatcher`, claims via atomic
  `ClaimAndDeleteScheduledNotification`, and delivers.
- `quiet_hours.go` — timezone-aware `InQuietHours` helper used by the
  dispatcher to skip-and-wait between 22:00 and 07:00 local.
- `experience_reminders.go` — concrete `Reconciler` + `Dispatcher` for
  Experience-anchored reminders (REMINDER_BEFORE_START at three
  policy-defined offsets).

Duplicate-safe inserts are handled by
`storage.InsertScheduledNotificationIfAbsent` (`ON CONFLICT DO NOTHING`
against the per-item-variant partial unique indexes added in #2458); the
reconciler's reaper pass cleans up any pre-existing duplicates on each tick.

## When to add code here vs. elsewhere

A new reminder family (a new oneof variant on `ScheduledNotification.item`)
ships as one file in this package implementing both `Reconciler` and
`Dispatcher`. The driver code (`reconcile.go` / `dispatch.go`) stays
generic. Storage-side changes (a new oneof variant requires a new
per-item-type finder) belong in `server/storage/scheduled_notification.go`.
