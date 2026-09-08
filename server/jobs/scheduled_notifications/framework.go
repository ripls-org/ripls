package scheduled_notifications

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// Reconciler computes the desired set of scheduled-notification rows
// for one item type (Experience-anchored, loan-anchored, etc.) and
// loads the current set from storage. The driver in reconcile.go
// diffs the two and applies inserts, fire-at updates, and
// hard-deletes.
//
// Implementations are responsible for which storage finder to call
// in Existing() — each item type has its own
// FindXScheduledNotifications helper.
type Reconciler interface {
	// Name returns a stable identifier used in structured logs.
	Name() string

	// Desired returns the full set of rows the reconciler wants to
	// exist for its item type, given the current world. Rows are
	// constructed in memory; the driver assigns ids and persists
	// the ones not yet present.
	Desired(ctx context.Context) ([]*models.ScheduledNotification, error)

	// Existing returns the rows currently in storage for this
	// reconciler's item type.
	Existing(ctx context.Context) ([]*models.ScheduledNotification, error)
}

// Dispatcher renders a scheduled-notification row's payload at fire
// time. The dispatcher driver in dispatch.go routes each due row to
// the dispatcher that returns true from CanHandle.
//
// Render runs after the row has been atomically claimed and deleted;
// returning (nil, nil) means "the row should not produce a push"
// (e.g. anchor entity was just deleted, preferences flipped off) —
// the dispatcher driver simply records the skip and moves on. The
// row is already gone either way.
type Dispatcher interface {
	// Name returns a stable identifier used in structured logs.
	Name() string

	// CanHandle reports whether this dispatcher owns the row's item
	// variant.
	CanHandle(row *models.ScheduledNotification) bool

	// QuietHoursPolicy reports how the dispatcher wants the driver
	// to handle a row whose recipient is currently in quiet hours.
	// Day-grained slots return Defer (skip-and-wait until 07:00
	// local); short-lead slots whose copy would be misleading after
	// a multi-hour delay return Skip (drop the row without sending).
	QuietHoursPolicy(row *models.ScheduledNotification) QuietHoursPolicy

	// Render builds the notification payload from current state of
	// the anchored entity at dispatch time. May return (nil, nil) to
	// skip without sending.
	Render(ctx context.Context, row *models.ScheduledNotification) (*models.Notification, error)
}

// QuietHoursPolicy controls what the dispatcher driver does when a
// row's recipient is in their local quiet-hours window at dispatch
// time.
type QuietHoursPolicy int

const (
	// QuietHoursPolicyDefer leaves the row PENDING and tries again
	// on the next dispatcher tick. The reminder eventually fires
	// when local time crosses 07:00 — suitable for day-grained
	// reminders ("event tomorrow", "due tomorrow") whose copy
	// tolerates a multi-hour shift.
	QuietHoursPolicyDefer QuietHoursPolicy = iota

	// QuietHoursPolicySkip claims and deletes the row without
	// sending. Suitable for short-lead reminders ("starts in 2
	// hours", "starting now") whose copy would be actively
	// misleading if delayed until 07:00. The recipient simply
	// doesn't get this reminder; they still get any longer-lead
	// reminders earlier in the day.
	QuietHoursPolicySkip
)

// UniquenessKey extracts the reconciler's uniqueness key for a row:
// (recipient, item-type, item-id, purpose, offset). Two rows with
// the same key are the same scheduled notification, even if their
// fire_at differs (an update, not an insert+delete pair).
//
// Returns "" for rows with an unset item oneof; the caller treats
// these as malformed and logs ERROR.
func UniquenessKey(n *models.ScheduledNotification) string {
	if n == nil {
		return ""
	}
	switch item := n.Item.(type) {
	case *models.ScheduledNotification_Experience:
		e := item.Experience
		if e == nil {
			return ""
		}
		return fmt.Sprintf("u=%s|t=exp|id=%s|p=%d|o=%d",
			n.RecipientUserId, e.ExperienceId, int(e.Purpose), e.OffsetSecondsFromAnchor)
	case *models.ScheduledNotification_Loan:
		l := item.Loan
		if l == nil {
			return ""
		}
		return fmt.Sprintf("u=%s|t=loan|id=%s|p=%d|o=%d",
			n.RecipientUserId, l.TransferId, int(l.Purpose), l.OffsetSecondsFromAnchor)
	case *models.ScheduledNotification_Request:
		r := item.Request
		if r == nil {
			return ""
		}
		return fmt.Sprintf("u=%s|t=req|id=%s|p=%d|o=%d",
			n.RecipientUserId, r.RequestId, int(r.Purpose), r.OffsetSecondsFromAnchor)
	}
	return ""
}

// ReconcileStats summarises one reconciler tick.
type ReconcileStats struct {
	Inserted  int
	Updated   int
	Unchanged int
	Deleted   int
	// ReapedDuplicates counts rows that the reaper deleted because they
	// shared a uniqueness key with another future-fire-at row — a side
	// effect of the multi-replica reconciler race fixed in #2458.
	ReapedDuplicates int
	// Errored counts rows where an INSERT/UPDATE/DELETE failed; the
	// reconciler continues past these (best-effort) and the next
	// tick gets another shot.
	Errored int
}

// DesiredFireAtGrace is how stale a desired-state fire_at may be and
// still be emitted by a reconciler. Tuples whose fire_at is older
// than now − grace are presumed already fired (so re-emission would
// produce a duplicate push) and dropped. The constant must be
// smaller than the reconciler tick interval, otherwise a row fired
// shortly after a reconciler tick would be re-inserted on the next
// tick and re-fire.
const DesiredFireAtGrace = 5 * 60 // seconds

// DispatchStats summarises one dispatcher tick.
type DispatchStats struct {
	Sent               int
	QuietHoursDeferred int
	// QuietHoursSkipped counts rows that the dispatcher dropped
	// (claim+delete, no send) because the recipient was in quiet
	// hours and the dispatcher's QuietHoursPolicy was Skip — meaning
	// the reminder's copy is too short-lead to tolerate a delay.
	QuietHoursSkipped int
	UnknownItemType   int
	// ClaimRaceLost counts rows where another runner claimed the
	// row before we could. Expected with concurrent dispatchers.
	ClaimRaceLost int
	// RenderSkipped counts rows whose dispatcher returned (nil, nil)
	// — e.g. anchor entity gone, preferences gated off.
	RenderSkipped int
	// RenderFailed counts rows where the per-kind dispatcher
	// returned a non-nil error. The row is already deleted at this
	// point (claim happened first); the push is lost.
	RenderFailed int
	// SendFailed counts rows where notifications.Service.NotifyUser
	// returned an error post-render. Row already deleted; push lost.
	SendFailed int
}
