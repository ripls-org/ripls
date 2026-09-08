package scheduled_notifications

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications/notification_content"
	"go.ripls.org/ripls/server/storage"
)

// Loan-return reminder offsets, relative to the loan's
// expected_return_unix_sec (negative = before due). The borrower
// gets a heads-up the day before, a reminder on the due day, and a
// nudge three days after if they haven't returned the item.
const (
	// OffsetLoanReturnDayBefore fires 24h before the loan's
	// expected return time.
	OffsetLoanReturnDayBefore int64 = -24 * 3600
	// OffsetLoanReturnDueDay fires at the expected return time.
	OffsetLoanReturnDueDay int64 = 0
	// OffsetLoanReturnOverdue fires three days after the expected
	// return time if the loan is still active.
	OffsetLoanReturnOverdue int64 = 3 * 24 * 3600
)

var loanReturnOffsets = []int64{
	OffsetLoanReturnDayBefore,
	OffsetLoanReturnDueDay,
	OffsetLoanReturnOverdue,
}

// loanBacklogGrace bounds the reconciler's "still in scope" window
// past the latest reminder slot. Once now > expected_return +
// max-offset + grace, the reconciler stops emitting tuples for the
// loan, so existing rows can age out cleanly via the dispatcher and
// nothing re-emits indefinitely.
const loanBacklogGrace = 24 * 3600

// LoanWorld is the subset of storage and user preferences the
// loan-return reconciler and dispatcher need. Defined as an interface
// so tests can pass a lightweight fake.
type LoanWorld interface {
	ListActiveLoansWithExpectedReturn(ctx context.Context) ([]*models.Transfer, error)
	GetTransfer(ctx context.Context, transferID string) (*models.Transfer, error)
	GetGear(ctx context.Context, gearID string) (*models.Gear, error)
	GetUserDisplayName(ctx context.Context, userID string) (string, error)
	GetUserNotificationPreferences(ctx context.Context, userID string) (*models.UserNotificationPreferences, error)
}

// LoanReturnReminderReconciler emits desired-state tuples for the
// RETURN_REMINDER purpose at the three policy-defined offsets, one
// per slot per borrower per active loan with an expected return.
type LoanReturnReminderReconciler struct {
	world    LoanWorld
	existing func(ctx context.Context) ([]*models.ScheduledNotification, error)
}

// NewLoanReturnReminderReconciler wires the reconciler against real
// storage.
func NewLoanReturnReminderReconciler(s *storage.ProtoSQLStorage, world LoanWorld) *LoanReturnReminderReconciler {
	return &LoanReturnReminderReconciler{
		world:    world,
		existing: s.FindLoanScheduledNotifications,
	}
}

// Name returns the stable identifier used in structured logs.
func (r *LoanReturnReminderReconciler) Name() string {
	return "loan_return_reminders"
}

// Existing returns the rows currently in storage whose item variant
// is loan.
func (r *LoanReturnReminderReconciler) Existing(ctx context.Context) ([]*models.ScheduledNotification, error) {
	return r.existing(ctx)
}

// Desired walks active loans and produces one desired tuple per
// (borrower, offset) pair whose fire_at is still in the future or
// within the dispatch grace window.
func (r *LoanReturnReminderReconciler) Desired(ctx context.Context) ([]*models.ScheduledNotification, error) {
	loans, err := r.world.ListActiveLoansWithExpectedReturn(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active loans: %w", err)
	}

	now := clock.UnixSec(ctx)
	maxOffset := loanReturnOffsets[0]
	for _, o := range loanReturnOffsets {
		if o > maxOffset {
			maxOffset = o
		}
	}

	var out []*models.ScheduledNotification
	for _, loan := range loans {
		if loan.GetDeleted() != nil {
			continue
		}
		if loan.ExpectedReturnUnixSec == nil {
			continue
		}
		expectedReturn := *loan.ExpectedReturnUnixSec
		// Drop loans whose latest reminder slot is past the
		// backlog grace — keeps the system from cycling stale
		// reminders forever for items the borrower never returned.
		if expectedReturn+maxOffset < now-loanBacklogGrace {
			continue
		}
		if loan.RecipientId == "" {
			// Provisional-recipient loans (no user_id) cannot receive a
			// push; skip silently.
			continue
		}
		for _, offset := range loanReturnOffsets {
			fireAt := expectedReturn + offset
			if fireAt < now-DesiredFireAtGrace {
				continue
			}
			out = append(out, buildLoanReturnReminder(loan, fireAt, offset))
		}
	}
	return out, nil
}

func buildLoanReturnReminder(loan *models.Transfer, fireAt, offset int64) *models.ScheduledNotification {
	row := &models.ScheduledNotification{
		RecipientUserId: loan.RecipientId,
		FireAtUnixSec:   fireAt,
		Item: &models.ScheduledNotification_Loan{
			Loan: &models.LoanNotification{
				TransferId:              loan.Id,
				Purpose:                 models.LoanNotificationPurpose_LOAN_NOTIFICATION_PURPOSE_RETURN_REMINDER,
				OffsetSecondsFromAnchor: offset,
			},
		},
	}
	if loan.CommunityId != "" {
		cid := loan.CommunityId
		row.CommunityId = &cid
	}
	return row
}

// LoanReturnReminderDispatcher renders the push payload for a
// loan-anchored row at dispatch time.
type LoanReturnReminderDispatcher struct {
	world LoanWorld
}

// NewLoanReturnReminderDispatcher wires the dispatcher.
func NewLoanReturnReminderDispatcher(world LoanWorld) *LoanReturnReminderDispatcher {
	return &LoanReturnReminderDispatcher{world: world}
}

// Name returns the stable identifier used in structured logs.
func (d *LoanReturnReminderDispatcher) Name() string {
	return "loan_return_reminders"
}

// CanHandle reports whether the row's item variant is a loan-return
// row this dispatcher owns. Future loan-anchored purposes will
// register their own dispatchers and CanHandle branches on Purpose.
func (d *LoanReturnReminderDispatcher) CanHandle(row *models.ScheduledNotification) bool {
	loan := row.GetLoan()
	if loan == nil {
		return false
	}
	return loan.Purpose == models.LoanNotificationPurpose_LOAN_NOTIFICATION_PURPOSE_RETURN_REMINDER
}

// QuietHoursPolicy returns Defer for every loan-return slot. All
// three offsets (day-before, due-day, overdue) speak in day-grained
// terms ("Return tomorrow", "Due back today", "Overdue") whose copy
// remains accurate when delivered the morning after a quiet-hours
// fire_at.
func (d *LoanReturnReminderDispatcher) QuietHoursPolicy(_ *models.ScheduledNotification) QuietHoursPolicy {
	return QuietHoursPolicyDefer
}

// Render reads the current loan + gear and builds the Notification.
// Returns (nil, nil) when the loan is gone, no longer active, or the
// recipient has disabled loan-return reminders.
func (d *LoanReturnReminderDispatcher) Render(ctx context.Context, row *models.ScheduledNotification) (*models.Notification, error) {
	loanRow := row.GetLoan()
	if loanRow == nil {
		return nil, fmt.Errorf("row missing loan variant")
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ScheduledNotificationDispatch",
		"dispatcher", d.Name(),
		"transfer_id", loanRow.TransferId,
		"offset_seconds_from_anchor", loanRow.OffsetSecondsFromAnchor,
		"recipient_user_id", row.RecipientUserId,
	)

	transfer, err := d.world.GetTransfer(ctx, loanRow.TransferId)
	if err != nil {
		logger.InfoContext(ctx, "anchor Transfer not loadable; skipping send", "error", err)
		return nil, nil
	}
	if transfer.GetDeleted() != nil {
		logger.InfoContext(ctx, "anchor Transfer soft-deleted; skipping send")
		return nil, nil
	}
	if transfer.State != models.TransferState_TRANSFER_STATE_ACTIVE {
		logger.InfoContext(ctx, "loan is no longer ACTIVE; skipping send",
			"transfer_state", transfer.State.String(),
		)
		return nil, nil
	}

	// Per-user preference gate. Fail open on lookup error so a
	// transient DB hiccup doesn't suppress a real reminder.
	prefs, err := d.world.GetUserNotificationPreferences(ctx, row.RecipientUserId)
	if err != nil {
		logger.WarnContext(ctx, "preference lookup failed; failing open and delivering", "error", err)
	} else if prefs != nil && prefs.NotifyLoanReturnReminders != nil && !*prefs.NotifyLoanReturnReminders {
		logger.InfoContext(ctx, "recipient has disabled loan-return reminders; skipping")
		return nil, nil
	}

	gearName := ""
	if gear, err := d.world.GetGear(ctx, transfer.GearId); err == nil && gear != nil {
		gearName = gear.Name
	}
	ownerName, _ := d.world.GetUserDisplayName(ctx, transfer.OwnerId)

	return notification_content.SystemNotification(ctx, &models.CommunityEventPayload{
		CommunityId: row.GetCommunityId(),
		EventType:   loanReturnEventType(loanRow.OffsetSecondsFromAnchor),
		GearId:      transfer.GearId,
		GearName:    gearName,
		// The lender. ActorName is the payload's only human-name field, and for
		// a loan reminder the salient person is the one the borrower returns
		// the item to — there is no actor, since a job produced this.
		ActorName: ownerName,
	}), nil
}

// loanReturnEventType maps the offset from the loan's expected return time to
// the event type whose copy matches it: negative is still upcoming, zero is due
// today, positive is overdue. Same reasoning as reminderEventType — the offset
// is not on the payload, so it is encoded in the type.
func loanReturnEventType(offsetSeconds int64) string {
	switch {
	case offsetSeconds < 0:
		return "LOAN_RETURN_REMINDER_UPCOMING"
	case offsetSeconds == 0:
		return "LOAN_RETURN_REMINDER_DUE"
	default:
		return "LOAN_RETURN_REMINDER_OVERDUE"
	}
}
