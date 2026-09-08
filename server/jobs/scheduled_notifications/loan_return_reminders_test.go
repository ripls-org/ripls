package scheduled_notifications

import (
	"context"
	"sort"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

type fakeLoanWorld struct {
	loans          []*models.Transfer
	transfers      map[string]*models.Transfer // for GetTransfer fallthrough
	gears          map[string]*models.Gear
	userNames      map[string]string
	prefs          map[string]*models.UserNotificationPreferences
	prefsErr       error
	listErr        error
	getTransferErr error
}

func (f *fakeLoanWorld) ListActiveLoansWithExpectedReturn(_ context.Context) ([]*models.Transfer, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.loans, nil
}

func (f *fakeLoanWorld) GetTransfer(_ context.Context, transferID string) (*models.Transfer, error) {
	if f.getTransferErr != nil {
		return nil, f.getTransferErr
	}
	if t, ok := f.transfers[transferID]; ok {
		return t, nil
	}
	for _, t := range f.loans {
		if t.Id == transferID {
			return t, nil
		}
	}
	return nil, errLoanNotFound
}

func (f *fakeLoanWorld) GetGear(_ context.Context, gearID string) (*models.Gear, error) {
	if g, ok := f.gears[gearID]; ok {
		return g, nil
	}
	return nil, errLoanNotFound
}

func (f *fakeLoanWorld) GetUserDisplayName(_ context.Context, userID string) (string, error) {
	if name, ok := f.userNames[userID]; ok {
		return name, nil
	}
	return "", nil
}

func (f *fakeLoanWorld) GetUserNotificationPreferences(_ context.Context, userID string) (*models.UserNotificationPreferences, error) {
	if f.prefsErr != nil {
		return nil, f.prefsErr
	}
	return f.prefs[userID], nil
}

var errLoanNotFound = loanNotFoundError("loan not found")

type loanNotFoundError string

func (e loanNotFoundError) Error() string { return string(e) }

func activeLoan(id, ownerID, borrowerID, gearID string, expectedReturn int64) *models.Transfer {
	er := expectedReturn
	return &models.Transfer{
		Id:                    id,
		GearId:                gearID,
		OwnerId:               ownerID,
		RecipientId:           borrowerID,
		TransferType:          models.TransferType_TRANSFER_TYPE_LOAN,
		State:                 models.TransferState_TRANSFER_STATE_ACTIVE,
		CommunityId:           "c-1",
		ExpectedReturnUnixSec: &er,
	}
}

func newLoanReconciler(world LoanWorld) *LoanReturnReminderReconciler {
	return &LoanReturnReminderReconciler{
		world: world,
		existing: func(_ context.Context) ([]*models.ScheduledNotification, error) {
			return nil, nil
		},
	}
}

func TestLoanReturnReminders_EmitsThreeOffsetsPerActiveLoan(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	expectedReturn := now.Add(7 * 24 * time.Hour).Unix() // 7 days from now

	world := &fakeLoanWorld{
		loans: []*models.Transfer{
			activeLoan("t-1", "u-owner", "u-borrower", "g-1", expectedReturn),
		},
	}

	got, err := newLoanReconciler(world).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3", len(got))
	}
	offsets := []int64{}
	for _, row := range got {
		loan := row.GetLoan()
		if loan == nil {
			t.Fatalf("row %+v: missing loan variant", row)
		}
		if loan.TransferId != "t-1" {
			t.Errorf("TransferId = %q, want t-1", loan.TransferId)
		}
		if loan.Purpose != models.LoanNotificationPurpose_LOAN_NOTIFICATION_PURPOSE_RETURN_REMINDER {
			t.Errorf("Purpose = %v, want RETURN_REMINDER", loan.Purpose)
		}
		if row.RecipientUserId != "u-borrower" {
			t.Errorf("RecipientUserId = %q, want u-borrower", row.RecipientUserId)
		}
		offsets = append(offsets, loan.OffsetSecondsFromAnchor)
	}
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
	want := []int64{OffsetLoanReturnDayBefore, OffsetLoanReturnDueDay, OffsetLoanReturnOverdue}
	for i, w := range want {
		if offsets[i] != w {
			t.Errorf("offset[%d] = %d, want %d", i, offsets[i], w)
		}
	}
}

func TestLoanReturnReminders_FireAtComputedFromExpectedReturnPlusOffset(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	expectedReturn := now.Add(7 * 24 * time.Hour).Unix()

	world := &fakeLoanWorld{
		loans: []*models.Transfer{
			activeLoan("t-1", "u-owner", "u-borrower", "g-1", expectedReturn),
		},
	}
	got, err := newLoanReconciler(world).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	wantByOffset := map[int64]int64{
		OffsetLoanReturnDayBefore: expectedReturn + OffsetLoanReturnDayBefore,
		OffsetLoanReturnDueDay:    expectedReturn + OffsetLoanReturnDueDay,
		OffsetLoanReturnOverdue:   expectedReturn + OffsetLoanReturnOverdue,
	}
	for _, row := range got {
		off := row.GetLoan().OffsetSecondsFromAnchor
		want, ok := wantByOffset[off]
		if !ok {
			t.Errorf("unexpected offset %d", off)
			continue
		}
		if row.FireAtUnixSec != want {
			t.Errorf("offset %d: FireAtUnixSec = %d, want %d", off, row.FireAtUnixSec, want)
		}
	}
}

func TestLoanReturnReminders_SkipsLoansWithoutExpectedReturn(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)

	world := &fakeLoanWorld{
		loans: []*models.Transfer{
			{
				Id:           "t-no-er",
				RecipientId:  "u-borrower",
				State:        models.TransferState_TRANSFER_STATE_ACTIVE,
				TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			},
		},
	}
	got, err := newLoanReconciler(world).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d rows, want 0 (no expected_return_unix_sec)", len(got))
	}
}

func TestLoanReturnReminders_SkipsProvisionalRecipients(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	expectedReturn := now.Add(48 * time.Hour).Unix()

	prov := "prov-1"
	world := &fakeLoanWorld{
		loans: []*models.Transfer{
			{
				Id:                     "t-prov",
				State:                  models.TransferState_TRANSFER_STATE_ACTIVE,
				TransferType:           models.TransferType_TRANSFER_TYPE_LOAN,
				ProvisionalRecipientId: &prov,
				CommunityId:            "c-1",
				ExpectedReturnUnixSec:  &expectedReturn,
			},
		},
	}
	got, err := newLoanReconciler(world).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d rows, want 0 (provisional recipient cannot receive a push)", len(got))
	}
}

func TestLoanReturnReminders_OnlyEmitsFutureOrRecentSlots(t *testing.T) {
	// expected_return = now (overdue slot fires in 3 days, due-day
	// fires now, day-before fires 24h ago). With the grace, only
	// due-day and overdue should be emitted.
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	expectedReturn := now.Unix()

	world := &fakeLoanWorld{
		loans: []*models.Transfer{activeLoan("t-1", "u-o", "u-b", "g-1", expectedReturn)},
	}
	got, err := newLoanReconciler(world).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	offsets := map[int64]bool{}
	for _, row := range got {
		offsets[row.GetLoan().OffsetSecondsFromAnchor] = true
	}
	if offsets[OffsetLoanReturnDayBefore] {
		t.Error("day-before slot should NOT be emitted (fire_at 24h in past)")
	}
	if !offsets[OffsetLoanReturnDueDay] {
		t.Error("due-day slot should be emitted (fire_at = now)")
	}
	if !offsets[OffsetLoanReturnOverdue] {
		t.Error("overdue slot should be emitted (fire_at = now + 3d)")
	}
}

func TestLoanReturnReminders_DropsLoansBeyondBacklogGrace(t *testing.T) {
	// expected_return + max_offset = expected_return + 3d. If
	// expected_return is 5 days ago, that's 2 days past the latest
	// slot — well beyond loanBacklogGrace of 24h. The reconciler
	// should drop the loan entirely so the system doesn't churn on
	// items the borrower never returned.
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	expectedReturn := now.Add(-5 * 24 * time.Hour).Unix()

	world := &fakeLoanWorld{
		loans: []*models.Transfer{activeLoan("t-1", "u-o", "u-b", "g-1", expectedReturn)},
	}
	got, err := newLoanReconciler(world).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d rows, want 0 (loan past backlog grace)", len(got))
	}
}

func TestLoanReturnReminderDispatcher_CanHandle(t *testing.T) {
	d := &LoanReturnReminderDispatcher{}

	t.Run("matches RETURN_REMINDER loan rows", func(t *testing.T) {
		row := &models.ScheduledNotification{
			Item: &models.ScheduledNotification_Loan{
				Loan: &models.LoanNotification{
					Purpose: models.LoanNotificationPurpose_LOAN_NOTIFICATION_PURPOSE_RETURN_REMINDER,
				},
			},
		}
		if !d.CanHandle(row) {
			t.Error("expected CanHandle=true for RETURN_REMINDER")
		}
	})

	t.Run("does not match experience rows", func(t *testing.T) {
		row := experienceRow("u-1", "e-1",
			models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
			-86400, 100)
		if d.CanHandle(row) {
			t.Error("expected CanHandle=false for experience rows")
		}
	})
}

func TestLoanReturnReminderDispatcher_QuietHoursPolicy(t *testing.T) {
	// All three loan-return offsets are day-grained — Defer for every
	// slot so a borrower whose due-date falls in their overnight
	// window still gets reminded at 07:00 local.
	d := &LoanReturnReminderDispatcher{}

	offsets := []int64{
		OffsetLoanReturnDayBefore,
		OffsetLoanReturnDueDay,
		OffsetLoanReturnOverdue,
	}
	for _, offset := range offsets {
		row := &models.ScheduledNotification{
			Item: &models.ScheduledNotification_Loan{
				Loan: &models.LoanNotification{
					Purpose:                 models.LoanNotificationPurpose_LOAN_NOTIFICATION_PURPOSE_RETURN_REMINDER,
					OffsetSecondsFromAnchor: offset,
				},
			},
		}
		if got := d.QuietHoursPolicy(row); got != QuietHoursPolicyDefer {
			t.Errorf("offset %d: policy = %v, want Defer", offset, got)
		}
	}
}

func TestLoanReturnReminderDispatcher_Render(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	expectedReturn := now.Add(48 * time.Hour).Unix()
	loan := activeLoan("t-1", "u-owner", "u-borrower", "g-1", expectedReturn)
	gear := &models.Gear{Id: "g-1", Name: "Pressure Cooker"}
	world := &fakeLoanWorld{
		loans:     []*models.Transfer{loan},
		gears:     map[string]*models.Gear{"g-1": gear},
		userNames: map[string]string{"u-owner": "Alice"},
	}
	d := &LoanReturnReminderDispatcher{world: world}
	ctx := pinClock(now)

	cases := []struct {
		name          string
		offset        int64
		wantTitle     string
		wantBody      string
		wantEventType string
	}{
		{"day-before", OffsetLoanReturnDayBefore, "Return tomorrow", "Return Pressure Cooker to Alice tomorrow", "LOAN_RETURN_REMINDER_UPCOMING"},
		{"due-day", OffsetLoanReturnDueDay, "Return today", "Pressure Cooker is due back today", "LOAN_RETURN_REMINDER_DUE"},
		{"overdue", OffsetLoanReturnOverdue, "Overdue", "Pressure Cooker is overdue — please return it to Alice", "LOAN_RETURN_REMINDER_OVERDUE"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := &models.ScheduledNotification{
				Id:              "id-1",
				RecipientUserId: "u-borrower",
				Item: &models.ScheduledNotification_Loan{
					Loan: &models.LoanNotification{
						TransferId:              "t-1",
						Purpose:                 models.LoanNotificationPurpose_LOAN_NOTIFICATION_PURPOSE_RETURN_REMINDER,
						OffsetSecondsFromAnchor: c.offset,
					},
				},
			}
			notif, err := d.Render(ctx, row)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if notif == nil {
				t.Fatal("Render returned nil notification")
			}
			if notif.Title != c.wantTitle {
				t.Errorf("Title = %q, want %q", notif.Title, c.wantTitle)
			}
			if notif.Body != c.wantBody {
				t.Errorf("Body = %q, want %q", notif.Body, c.wantBody)
			}
			payload := notif.GetCommunityEvent()
			if payload == nil {
				t.Fatal("missing CommunityEventPayload")
			}
			// One event type per due-ness slot; see the reminder dispatcher.
			if payload.EventType != c.wantEventType {
				t.Errorf("EventType = %q, want %q", payload.EventType, c.wantEventType)
			}
			// Both names the copy interpolates must reach the payload so the
			// off-app senders can re-render the same sentence (#2896).
			// ActorName carries the lender: there is no actor on a job-produced
			// reminder, and the lender is who the borrower returns the item to.
			if payload.GearName != "Pressure Cooker" {
				t.Errorf("payload GearName = %q, want Pressure Cooker", payload.GearName)
			}
			if payload.ActorName != "Alice" {
				t.Errorf("payload ActorName (the lender) = %q, want Alice", payload.ActorName)
			}
			if payload.GearId != "g-1" {
				t.Errorf("GearId = %q, want g-1", payload.GearId)
			}
		})
	}
}

func TestLoanReturnReminderDispatcher_RenderSkipsWhenAnchorMissing(t *testing.T) {
	world := &fakeLoanWorld{getTransferErr: errLoanNotFound}
	d := &LoanReturnReminderDispatcher{world: world}

	row := &models.ScheduledNotification{
		Id:              "id-1",
		RecipientUserId: "u-borrower",
		Item: &models.ScheduledNotification_Loan{
			Loan: &models.LoanNotification{
				TransferId: "t-missing",
				Purpose:    models.LoanNotificationPurpose_LOAN_NOTIFICATION_PURPOSE_RETURN_REMINDER,
			},
		},
	}
	notif, err := d.Render(context.Background(), row)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if notif != nil {
		t.Error("expected nil notification when anchor is missing")
	}
}

func TestLoanReturnReminderDispatcher_RenderSkipsWhenLoanNoLongerActive(t *testing.T) {
	expectedReturn := int64(1700000000)
	loan := activeLoan("t-1", "u-o", "u-b", "g-1", expectedReturn)
	loan.State = models.TransferState_TRANSFER_STATE_COMPLETED // borrower returned the item

	world := &fakeLoanWorld{
		transfers: map[string]*models.Transfer{"t-1": loan},
	}
	d := &LoanReturnReminderDispatcher{world: world}

	row := &models.ScheduledNotification{
		RecipientUserId: "u-b",
		Item: &models.ScheduledNotification_Loan{
			Loan: &models.LoanNotification{
				TransferId: "t-1",
				Purpose:    models.LoanNotificationPurpose_LOAN_NOTIFICATION_PURPOSE_RETURN_REMINDER,
			},
		},
	}
	notif, err := d.Render(context.Background(), row)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if notif != nil {
		t.Error("expected nil notification when loan is no longer ACTIVE")
	}
}

func TestLoanReturnReminderDispatcher_RenderSkipsWhenPreferenceDisabled(t *testing.T) {
	expectedReturn := int64(1700000000)
	loan := activeLoan("t-1", "u-o", "u-b", "g-1", expectedReturn)

	disabled := false
	world := &fakeLoanWorld{
		transfers: map[string]*models.Transfer{"t-1": loan},
		gears:     map[string]*models.Gear{"g-1": {Id: "g-1", Name: "Drill"}},
		prefs: map[string]*models.UserNotificationPreferences{
			"u-b": {NotifyLoanReturnReminders: &disabled},
		},
	}
	d := &LoanReturnReminderDispatcher{world: world}

	row := &models.ScheduledNotification{
		RecipientUserId: "u-b",
		Item: &models.ScheduledNotification_Loan{
			Loan: &models.LoanNotification{
				TransferId: "t-1",
				Purpose:    models.LoanNotificationPurpose_LOAN_NOTIFICATION_PURPOSE_RETURN_REMINDER,
			},
		},
	}
	notif, err := d.Render(context.Background(), row)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if notif != nil {
		t.Error("expected nil notification when pref disables loan reminders")
	}
}

func TestLoanReturnReminderDispatcher_PreferenceFetchErrorsFailOpen(t *testing.T) {
	expectedReturn := int64(1700000000)
	loan := activeLoan("t-1", "u-o", "u-b", "g-1", expectedReturn)

	world := &fakeLoanWorld{
		transfers: map[string]*models.Transfer{"t-1": loan},
		gears:     map[string]*models.Gear{"g-1": {Id: "g-1", Name: "Drill"}},
		prefsErr:  errLoanNotFound,
	}
	d := &LoanReturnReminderDispatcher{world: world}

	row := &models.ScheduledNotification{
		RecipientUserId: "u-b",
		Item: &models.ScheduledNotification_Loan{
			Loan: &models.LoanNotification{
				TransferId: "t-1",
				Purpose:    models.LoanNotificationPurpose_LOAN_NOTIFICATION_PURPOSE_RETURN_REMINDER,
			},
		},
	}
	notif, err := d.Render(context.Background(), row)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if notif == nil {
		t.Error("expected fail-open delivery on pref lookup error")
	}
}
