package jobs

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// newTestJob constructs an ActivityDigestJob with a deterministic clock
// pinned to `now`, an in-memory MockEmailService, and the given config.
func newTestJob(
	t *testing.T, store *storage.ProtoSQLStorage,
	mock *email.MockEmailService, now time.Time, recipient string,
) *ActivityDigestJob {
	t.Helper()
	loc, err := time.LoadLocation("America/Denver")
	if err != nil {
		t.Fatalf("load Denver: %v", err)
	}
	j, err := NewActivityDigestJob(store, mock, ActivityDigestConfig{
		Recipient:    recipient,
		TimezoneName: "America/Denver",
		SendHour:     now.In(loc).Hour(),
	})
	if err != nil {
		t.Fatalf("NewActivityDigestJob: %v", err)
	}
	j.now = func() time.Time { return now }
	return j
}

func TestActivityDigestJob_OutsideSendHour_DoesNothing(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	mock := &email.MockEmailService{}

	loc, _ := time.LoadLocation("America/Denver")
	now := time.Date(2026, 5, 14, 13, 0, 0, 0, loc) // 1pm Mountain

	j, err := NewActivityDigestJob(store, mock, ActivityDigestConfig{
		Recipient:    "ops@example.com",
		TimezoneName: "America/Denver",
		SendHour:     8, // 8am — outside
	})
	if err != nil {
		t.Fatalf("NewActivityDigestJob: %v", err)
	}
	j.now = func() time.Time { return now }

	if err := j.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(mock.ActivityDigests) != 0 {
		t.Errorf("expected no digest sends outside send hour, got %d", len(mock.ActivityDigests))
	}
}

func TestActivityDigestJob_SendsAndClaimsThenSecondRunIsNoOp(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	mock := &email.MockEmailService{}

	loc, _ := time.LoadLocation("America/Denver")
	now := time.Date(2026, 5, 14, 8, 30, 0, 0, loc)

	// Seed an event in yesterday's local window so the digest has content.
	yesterdayStart := time.Date(2026, 5, 13, 0, 0, 0, 0, loc).Unix()
	commID, err := store.Insert(ctx, &models.Community{Name: "Test Co-op", OwnerUserId: "owner"})
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
		OccurredAtUnixSec: yesterdayStart + 600,
	}); err != nil {
		t.Fatalf("insert event: %v", err)
	}

	j := newTestJob(t, store, mock, now, "ops@example.com")

	if err := j.Run(ctx); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if len(mock.ActivityDigests) != 1 {
		t.Fatalf("expected 1 digest after first run, got %d", len(mock.ActivityDigests))
	}
	got := mock.ActivityDigests[0]
	if got.ToEmail != "ops@example.com" {
		t.Errorf("ToEmail = %q, want ops@example.com", got.ToEmail)
	}
	if got.Input.TotalMemberActionCount != 1 {
		t.Errorf("TotalMemberActionCount = %d, want 1", got.Input.TotalMemberActionCount)
	}

	// Second run on the same local day must not send again.
	if err := j.Run(ctx); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if len(mock.ActivityDigests) != 1 {
		t.Errorf("expected still 1 digest after second run, got %d", len(mock.ActivityDigests))
	}

	// Advance to the next local day; the job should send again.
	next := time.Date(2026, 5, 15, 8, 30, 0, 0, loc)
	j.now = func() time.Time { return next }
	if err := j.Run(ctx); err != nil {
		t.Fatalf("next-day Run: %v", err)
	}
	if len(mock.ActivityDigests) != 2 {
		t.Errorf("expected 2 digests after next-day run, got %d", len(mock.ActivityDigests))
	}
}

func TestActivityDigestJob_InvalidConfig(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	mock := &email.MockEmailService{}

	tests := []struct {
		name string
		cfg  ActivityDigestConfig
	}{
		{"missing recipient", ActivityDigestConfig{TimezoneName: "America/Denver", SendHour: 8}},
		{"invalid timezone", ActivityDigestConfig{Recipient: "r@x", TimezoneName: "Not/AZone", SendHour: 8}},
		{"hour too high", ActivityDigestConfig{Recipient: "r@x", TimezoneName: "America/Denver", SendHour: 24}},
		{"hour negative", ActivityDigestConfig{Recipient: "r@x", TimezoneName: "America/Denver", SendHour: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewActivityDigestJob(store, mock, tt.cfg); err == nil {
				t.Error("expected error")
			}
		})
	}
}
