package jobs

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// newWeeklyTestJob builds a job with a deterministic clock pinned to now,
// mailing to recipient, scheduled to fire on the same weekday + hour as
// `now` so a single Run trigger fires the send (when claimed).
func newWeeklyTestJob(
	t *testing.T, store *storage.ProtoSQLStorage,
	mock *email.MockEmailService, now time.Time, recipient string,
) *ActivityWeeklyDigestJob {
	t.Helper()
	loc, _ := time.LoadLocation("America/Denver")
	local := now.In(loc)
	j, err := NewActivityWeeklyDigestJob(store, mock, ActivityWeeklyDigestConfig{
		Recipient:    recipient,
		TimezoneName: "America/Denver",
		SendWeekday:  local.Weekday(),
		SendHour:     local.Hour(),
	})
	if err != nil {
		t.Fatalf("NewActivityWeeklyDigestJob: %v", err)
	}
	j.now = func() time.Time { return now }
	return j
}

func TestActivityWeeklyDigestJob_OutsideSendWindow_DoesNothing(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	mock := &email.MockEmailService{}

	loc, _ := time.LoadLocation("America/Denver")
	// Tuesday 9am — wrong weekday for a Monday-morning job.
	now := time.Date(2026, 5, 12, 9, 0, 0, 0, loc)

	j, err := NewActivityWeeklyDigestJob(store, mock, ActivityWeeklyDigestConfig{
		Recipient:    "ops@example.com",
		TimezoneName: "America/Denver",
		SendWeekday:  time.Monday,
		SendHour:     8,
	})
	if err != nil {
		t.Fatalf("NewActivityWeeklyDigestJob: %v", err)
	}
	j.now = func() time.Time { return now }

	if err := j.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(mock.ActivityDigests) != 0 {
		t.Errorf("expected no sends outside send window, got %d", len(mock.ActivityDigests))
	}
}

func TestActivityWeeklyDigestJob_SendsThenIdempotent(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	mock := &email.MockEmailService{}

	loc, _ := time.LoadLocation("America/Denver")
	// Monday 2026-05-18 08:30 Mountain. The window will cover the
	// preceding 7 local days: 2026-05-11 → 2026-05-18.
	now := time.Date(2026, 5, 18, 8, 30, 0, 0, loc)

	// Seed an event last Wednesday so the digest has content.
	prev := time.Date(2026, 5, 13, 12, 0, 0, 0, loc).Unix()
	commID, err := store.Insert(ctx, &models.Community{Name: "Test Coop", OwnerUserId: "owner"})
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	alice, err := store.Insert(ctx, &models.User{Name: "Alice", Email: "alice@example.com", AuthMethod: models.AuthMethod_AUTH_METHOD_GOOGLE})
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityEvent{
		CommunityId:       commID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           alice,
		OccurredAtUnixSec: prev,
	}); err != nil {
		t.Fatalf("insert event: %v", err)
	}

	j := newWeeklyTestJob(t, store, mock, now, "ops@example.com")

	if err := j.Run(ctx); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if len(mock.ActivityDigests) != 1 {
		t.Fatalf("expected 1 send after first run, got %d", len(mock.ActivityDigests))
	}
	got := mock.ActivityDigests[0]
	if got.Input.Period != email.DigestPeriodWeekly {
		t.Errorf("Period = %v, want DigestPeriodWeekly", got.Input.Period)
	}
	if got.Input.TotalMemberActionCount != 1 {
		t.Errorf("action count = %d, want 1", got.Input.TotalMemberActionCount)
	}

	// Second run same week — must not re-send.
	if err := j.Run(ctx); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if len(mock.ActivityDigests) != 1 {
		t.Errorf("expected still 1 send after second run, got %d", len(mock.ActivityDigests))
	}

	// Advance one week — should send again.
	nextWeek := now.AddDate(0, 0, 7)
	j.now = func() time.Time { return nextWeek }
	if err := j.Run(ctx); err != nil {
		t.Fatalf("next-week Run: %v", err)
	}
	if len(mock.ActivityDigests) != 2 {
		t.Errorf("expected 2 sends after next-week run, got %d", len(mock.ActivityDigests))
	}
}

func TestActivityWeeklyDigestJob_InvalidConfig(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	mock := &email.MockEmailService{}

	tests := []struct {
		name string
		cfg  ActivityWeeklyDigestConfig
	}{
		{"missing recipient", ActivityWeeklyDigestConfig{TimezoneName: "America/Denver", SendWeekday: time.Monday, SendHour: 8}},
		{"invalid timezone", ActivityWeeklyDigestConfig{Recipient: "r@x", TimezoneName: "Not/AZone", SendWeekday: time.Monday, SendHour: 8}},
		{"hour out of range", ActivityWeeklyDigestConfig{Recipient: "r@x", TimezoneName: "America/Denver", SendWeekday: time.Monday, SendHour: 24}},
		{"weekday out of range", ActivityWeeklyDigestConfig{Recipient: "r@x", TimezoneName: "America/Denver", SendWeekday: 9, SendHour: 8}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewActivityWeeklyDigestJob(store, mock, tt.cfg); err == nil {
				t.Error("expected error")
			}
		})
	}
}
