package scheduled_notifications

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

type fakeDispatchStorage struct {
	mu               sync.Mutex
	due              []*models.ScheduledNotification
	claimedIDs       map[string]bool // ids that ClaimAndDelete returned true for
	loserIDs         map[string]bool // ids whose claim returns false (race lost)
	findErr          error
	claimErrByID     map[string]error
	claimAndDelCalls int
}

func newFakeDispatchStorage() *fakeDispatchStorage {
	return &fakeDispatchStorage{
		claimedIDs:   map[string]bool{},
		loserIDs:     map[string]bool{},
		claimErrByID: map[string]error{},
	}
}

func (f *fakeDispatchStorage) FindDueScheduledNotifications(_ context.Context, _ int64, _ int) ([]*models.ScheduledNotification, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.due, nil
}

func (f *fakeDispatchStorage) ClaimAndDeleteScheduledNotification(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimAndDelCalls++
	if err, has := f.claimErrByID[id]; has {
		return false, err
	}
	if f.loserIDs[id] {
		return false, nil
	}
	f.claimedIDs[id] = true
	return true, nil
}

type fakeNotifService struct {
	mu              sync.Mutex
	sentByRecipient map[string][]*models.Notification
	sendErrByUser   map[string]error
}

func newFakeNotifService() *fakeNotifService {
	return &fakeNotifService{
		sentByRecipient: map[string][]*models.Notification{},
		sendErrByUser:   map[string]error{},
	}
}

func (f *fakeNotifService) NotifyUser(_ context.Context, userID string, n *models.Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, has := f.sendErrByUser[userID]; has {
		return err
	}
	f.sentByRecipient[userID] = append(f.sentByRecipient[userID], n)
	return nil
}

func (f *fakeNotifService) HasDevices(_ context.Context, _ string) bool { return true }
func (f *fakeNotifService) UnregisterDevice(_ context.Context, _ string) error {
	return nil
}

func (f *fakeNotifService) SendPhoneOptInWelcome(_ context.Context, _ *models.User) error {
	return nil
}

type fixedDispatcher struct {
	name           string
	canHandleAll   bool
	canHandleLoan  bool
	canHandleExp   bool
	quietPolicy    QuietHoursPolicy // defaults to Defer (zero value)
	renderResult   *models.Notification
	renderSkip     bool // return (nil, nil)
	renderErr      error
	renderCallback func(row *models.ScheduledNotification)
}

func (f *fixedDispatcher) Name() string { return f.name }
func (f *fixedDispatcher) CanHandle(row *models.ScheduledNotification) bool {
	if f.canHandleAll {
		return true
	}
	switch row.Item.(type) {
	case *models.ScheduledNotification_Experience:
		return f.canHandleExp
	case *models.ScheduledNotification_Loan:
		return f.canHandleLoan
	}
	return false
}

func (f *fixedDispatcher) QuietHoursPolicy(_ *models.ScheduledNotification) QuietHoursPolicy {
	return f.quietPolicy
}

func (f *fixedDispatcher) Render(_ context.Context, row *models.ScheduledNotification) (*models.Notification, error) {
	if f.renderCallback != nil {
		f.renderCallback(row)
	}
	if f.renderErr != nil {
		return nil, f.renderErr
	}
	if f.renderSkip {
		return nil, nil
	}
	return f.renderResult, nil
}

func newSimpleNotification(body string) *models.Notification {
	return &models.Notification{Body: body}
}

func TestDispatch_RoutesToMatchingDispatcherAndSends(t *testing.T) {
	store := newFakeDispatchStorage()
	row := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100)
	row.Id = "id-1"
	store.due = []*models.ScheduledNotification{row}

	notif := newFakeNotifService()
	disp := &fixedDispatcher{
		name:         "experience",
		canHandleExp: true,
		renderResult: newSimpleNotification("event tomorrow"),
	}

	stats, err := Dispatch(context.Background(), store, notif, []Dispatcher{disp}, 10)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if stats.Sent != 1 {
		t.Errorf("Sent = %d, want 1", stats.Sent)
	}
	if got := notif.sentByRecipient["u-1"]; len(got) != 1 || got[0].Body != "event tomorrow" {
		t.Errorf("expected one push to u-1 with body \"event tomorrow\", got %v", got)
	}
	if !store.claimedIDs["id-1"] {
		t.Errorf("expected row id-1 to be claimed")
	}
}

func TestDispatch_DefersInQuietHours(t *testing.T) {
	store := newFakeDispatchStorage()
	tz := "America/Los_Angeles"
	row := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100)
	row.Id = "id-1"
	row.QuietHoursTimezone = &tz
	store.due = []*models.ScheduledNotification{row}

	notif := newFakeNotifService()
	disp := &fixedDispatcher{name: "experience", canHandleExp: true, renderResult: newSimpleNotification("body")}

	// Pin the clock to 3am PT (quiet hours).
	loc, _ := time.LoadLocation(tz)
	ctx := clock.WithSimulationTime(context.Background(), time.Date(2026, 5, 15, 3, 0, 0, 0, loc))

	stats, err := Dispatch(ctx, store, notif, []Dispatcher{disp}, 10)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if stats.QuietHoursDeferred != 1 {
		t.Errorf("QuietHoursDeferred = %d, want 1", stats.QuietHoursDeferred)
	}
	if stats.Sent != 0 {
		t.Errorf("Sent = %d, want 0", stats.Sent)
	}
	if store.claimAndDelCalls != 0 {
		t.Errorf("expected no claim attempts during quiet hours, got %d", store.claimAndDelCalls)
	}
}

func TestDispatch_QuietHoursSkipDropsTheRow(t *testing.T) {
	// Dispatcher whose QuietHoursPolicy is Skip — typically a
	// short-lead reminder where firing late would mislead. The
	// driver should claim-and-delete the row without firing, and
	// count the row as QuietHoursSkipped (not Deferred).
	store := newFakeDispatchStorage()
	tz := "America/Los_Angeles"
	row := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, 0, 100)
	row.Id = "id-1"
	row.QuietHoursTimezone = &tz
	store.due = []*models.ScheduledNotification{row}

	notif := newFakeNotifService()
	disp := &fixedDispatcher{
		name:         "experience",
		canHandleExp: true,
		quietPolicy:  QuietHoursPolicySkip,
		renderResult: newSimpleNotification("body"),
	}

	loc, _ := time.LoadLocation(tz)
	ctx := clock.WithSimulationTime(context.Background(), time.Date(2026, 5, 15, 3, 0, 0, 0, loc))

	stats, err := Dispatch(ctx, store, notif, []Dispatcher{disp}, 10)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if stats.QuietHoursSkipped != 1 {
		t.Errorf("QuietHoursSkipped = %d, want 1", stats.QuietHoursSkipped)
	}
	if stats.QuietHoursDeferred != 0 {
		t.Errorf("QuietHoursDeferred = %d, want 0", stats.QuietHoursDeferred)
	}
	if stats.Sent != 0 {
		t.Errorf("Sent = %d, want 0", stats.Sent)
	}
	if !store.claimedIDs["id-1"] {
		t.Error("expected the SKIP row to be claimed-and-deleted")
	}
	if len(notif.sentByRecipient) != 0 {
		t.Errorf("expected no push sends on a SKIP row, got %v", notif.sentByRecipient)
	}
}

func TestDispatch_QuietHoursSkipRaceLost(t *testing.T) {
	// Another runner already claimed the same row. The current
	// runner's DELETE returns rowsAffected=0; the driver counts it
	// as a claim race rather than a skipped row.
	store := newFakeDispatchStorage()
	tz := "America/Los_Angeles"
	row := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, 0, 100)
	row.Id = "id-1"
	row.QuietHoursTimezone = &tz
	store.due = []*models.ScheduledNotification{row}
	store.loserIDs["id-1"] = true

	notif := newFakeNotifService()
	disp := &fixedDispatcher{
		name:         "experience",
		canHandleExp: true,
		quietPolicy:  QuietHoursPolicySkip,
	}

	loc, _ := time.LoadLocation(tz)
	ctx := clock.WithSimulationTime(context.Background(), time.Date(2026, 5, 15, 3, 0, 0, 0, loc))

	stats, err := Dispatch(ctx, store, notif, []Dispatcher{disp}, 10)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if stats.ClaimRaceLost != 1 {
		t.Errorf("ClaimRaceLost = %d, want 1", stats.ClaimRaceLost)
	}
	if stats.QuietHoursSkipped != 0 {
		t.Errorf("QuietHoursSkipped = %d, want 0 (the other runner won the claim)", stats.QuietHoursSkipped)
	}
}

func TestDispatch_NoMatchingDispatcherLogsAndSkips(t *testing.T) {
	store := newFakeDispatchStorage()
	row := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100)
	row.Id = "id-1"
	store.due = []*models.ScheduledNotification{row}

	notif := newFakeNotifService()
	disp := &fixedDispatcher{name: "only-loan", canHandleLoan: true}

	stats, err := Dispatch(context.Background(), store, notif, []Dispatcher{disp}, 10)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if stats.UnknownItemType != 1 {
		t.Errorf("UnknownItemType = %d, want 1", stats.UnknownItemType)
	}
	if store.claimedIDs["id-1"] {
		t.Errorf("row should not be claimed when no dispatcher matches")
	}
}

func TestDispatch_ClaimRaceLost(t *testing.T) {
	store := newFakeDispatchStorage()
	row := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100)
	row.Id = "id-1"
	store.due = []*models.ScheduledNotification{row}
	store.loserIDs["id-1"] = true

	notif := newFakeNotifService()
	disp := &fixedDispatcher{name: "experience", canHandleExp: true, renderResult: newSimpleNotification("body")}

	stats, err := Dispatch(context.Background(), store, notif, []Dispatcher{disp}, 10)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if stats.ClaimRaceLost != 1 {
		t.Errorf("ClaimRaceLost = %d, want 1", stats.ClaimRaceLost)
	}
	if len(notif.sentByRecipient) != 0 {
		t.Errorf("expected no sends after losing claim, got %v", notif.sentByRecipient)
	}
}

func TestDispatch_RenderSkipDoesNotSend(t *testing.T) {
	store := newFakeDispatchStorage()
	row := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100)
	row.Id = "id-1"
	store.due = []*models.ScheduledNotification{row}

	notif := newFakeNotifService()
	disp := &fixedDispatcher{name: "experience", canHandleExp: true, renderSkip: true}

	stats, err := Dispatch(context.Background(), store, notif, []Dispatcher{disp}, 10)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if stats.RenderSkipped != 1 {
		t.Errorf("RenderSkipped = %d, want 1", stats.RenderSkipped)
	}
	if !store.claimedIDs["id-1"] {
		t.Errorf("row should be claimed and deleted even when render skips (entity gone)")
	}
	if stats.Sent != 0 {
		t.Errorf("Sent = %d, want 0", stats.Sent)
	}
}

func TestDispatch_RenderErrorClaimsAndLoses(t *testing.T) {
	// Provider rendering failure after a successful claim — the row
	// is gone and the push is lost, per the design tradeoff.
	store := newFakeDispatchStorage()
	row := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100)
	row.Id = "id-1"
	store.due = []*models.ScheduledNotification{row}

	notif := newFakeNotifService()
	disp := &fixedDispatcher{name: "experience", canHandleExp: true, renderErr: errors.New("boom")}

	stats, err := Dispatch(context.Background(), store, notif, []Dispatcher{disp}, 10)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if stats.RenderFailed != 1 {
		t.Errorf("RenderFailed = %d, want 1", stats.RenderFailed)
	}
	if !store.claimedIDs["id-1"] {
		t.Errorf("row should still be claimed; the push is lost but the row is gone")
	}
}

func TestDispatch_SendFailureLogsAndContinues(t *testing.T) {
	store := newFakeDispatchStorage()
	row := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100)
	row.Id = "id-1"
	store.due = []*models.ScheduledNotification{row}

	notif := newFakeNotifService()
	notif.sendErrByUser["u-1"] = errors.New("fcm down")
	disp := &fixedDispatcher{name: "experience", canHandleExp: true, renderResult: newSimpleNotification("body")}

	stats, err := Dispatch(context.Background(), store, notif, []Dispatcher{disp}, 10)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if stats.SendFailed != 1 {
		t.Errorf("SendFailed = %d, want 1", stats.SendFailed)
	}
	if stats.Sent != 0 {
		t.Errorf("Sent = %d, want 0", stats.Sent)
	}
}

func TestDispatch_PropagatesFindError(t *testing.T) {
	store := newFakeDispatchStorage()
	store.findErr = errors.New("db gone")

	if _, err := Dispatch(context.Background(), store, newFakeNotifService(), nil, 10); err == nil {
		t.Error("expected error to propagate from FindDueScheduledNotifications")
	}
}

func TestDispatch_MultipleRowsRoutedIndependently(t *testing.T) {
	store := newFakeDispatchStorage()
	expRow := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100)
	expRow.Id = "id-exp"

	loanRow := &models.ScheduledNotification{
		Id:              "id-loan",
		RecipientUserId: "u-2",
		FireAtUnixSec:   100,
		Item: &models.ScheduledNotification_Loan{
			Loan: &models.LoanNotification{
				TransferId:              "t-1",
				Purpose:                 models.LoanNotificationPurpose_LOAN_NOTIFICATION_PURPOSE_RETURN_REMINDER,
				OffsetSecondsFromAnchor: -86400,
			},
		},
	}

	store.due = []*models.ScheduledNotification{expRow, loanRow}

	notif := newFakeNotifService()
	expDisp := &fixedDispatcher{name: "experience", canHandleExp: true, renderResult: newSimpleNotification("exp body")}
	loanDisp := &fixedDispatcher{name: "loan", canHandleLoan: true, renderResult: newSimpleNotification("loan body")}

	stats, err := Dispatch(context.Background(), store, notif, []Dispatcher{expDisp, loanDisp}, 10)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if stats.Sent != 2 {
		t.Errorf("Sent = %d, want 2", stats.Sent)
	}
	if got := notif.sentByRecipient["u-1"]; len(got) != 1 || got[0].Body != "exp body" {
		t.Errorf("u-1 push: got %v, want one with body \"exp body\"", got)
	}
	if got := notif.sentByRecipient["u-2"]; len(got) != 1 || got[0].Body != "loan body" {
		t.Errorf("u-2 push: got %v, want one with body \"loan body\"", got)
	}
}
