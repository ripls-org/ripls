package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// TestCommunityPurgeReminderJob_HappyPath: a community deleted
// 29.5 days ago with a populated snapshot gets exactly one push
// per snapshot member, and the storage flag is set so a re-run
// is a no-op.
func TestCommunityPurgeReminderJob_HappyPath(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	now := time.Now().Unix()

	const ownerID = "user-owner"
	const otherID = "user-other"

	communityID, err := db.Insert(ctx, &models.Community{
		Name: "About to be purged", CreatorId: ownerID, OwnerUserId: ownerID,
		Deleted: &models.DeletedMetadata{
			DeletedByUserId:  ownerID,
			DeletedAtUnixSec: now - communitylib.PurgeReminderLeadSeconds - (12 * 60 * 60),
		},
		DeletedSnapshot: &models.CommunityDeletedSnapshot{
			MemberUserIds: []string{ownerID, otherID},
		},
	})
	if err != nil {
		t.Fatalf("seed community: %v", err)
	}

	// Out-of-window community to confirm scope.
	if _, err := db.Insert(ctx, &models.Community{
		Name: "Too recent", CreatorId: ownerID, OwnerUserId: ownerID,
		Deleted: &models.DeletedMetadata{
			DeletedByUserId:  ownerID,
			DeletedAtUnixSec: now - (28 * 24 * 60 * 60),
		},
		DeletedSnapshot: &models.CommunityDeletedSnapshot{MemberUserIds: []string{ownerID}},
	}); err != nil {
		t.Fatalf("seed too-recent community: %v", err)
	}

	mockNotif := notifications.NewMockService()
	job := NewCommunityPurgeReminderJob(db, mockNotif)
	if err := job.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	calls := mockNotif.GetCalls()
	if len(calls) != 2 {
		t.Fatalf("expected exactly 2 pushes (one per snapshot member), got %d", len(calls))
	}
	gotRecipients := make(map[string]bool, len(calls))
	for _, c := range calls {
		gotRecipients[c.UserID] = true
		if c.Notification == nil || c.Notification.Title == "" || c.Notification.Body == "" {
			t.Errorf("push missing title/body: %+v", c.Notification)
		}
	}
	if !gotRecipients[ownerID] {
		t.Errorf("expected owner %q to receive the reminder", ownerID)
	}
	if !gotRecipients[otherID] {
		t.Errorf("expected other %q to receive the reminder", otherID)
	}

	// Storage flag is now set — re-run dispatches no additional pushes.
	mockNotif.Reset()
	if err := job.Run(ctx); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if got := len(mockNotif.GetCalls()); got != 0 {
		t.Errorf("expected 0 pushes on re-run (idempotent), got %d", got)
	}

	// Confirm the flat column is set on the community.
	got := &models.Community{}
	if err := db.GetByID(ctx, communityID, got, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("re-fetch: %v", err)
	}
	// We can't directly check the flat column from here, but the
	// claim-fail-on-second-run above proves it landed.
	_ = got
}

// TestCommunityPurgeReminderJob_EmptySnapshot: a community deleted
// 29.5 days ago with no snapshot members still gets its reminder
// flag set so the job doesn't re-process it tomorrow. No pushes
// are dispatched.
func TestCommunityPurgeReminderJob_EmptySnapshot(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	now := time.Now().Unix()

	commID, err := db.Insert(ctx, &models.Community{
		Name: "Lonely", CreatorId: "u", OwnerUserId: "u",
		Deleted: &models.DeletedMetadata{
			DeletedByUserId:  "u",
			DeletedAtUnixSec: now - communitylib.PurgeReminderLeadSeconds - 100,
		},
		DeletedSnapshot: &models.CommunityDeletedSnapshot{MemberUserIds: nil},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	mockNotif := notifications.NewMockService()
	job := NewCommunityPurgeReminderJob(db, mockNotif)
	if err := job.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := len(mockNotif.GetCalls()); got != 0 {
		t.Errorf("expected 0 pushes for empty snapshot, got %d", got)
	}

	// The reminder flag must still be set so this community
	// doesn't show up in tomorrow's run.
	claimed, err := db.ClaimCommunityPurgeReminder(ctx, commID, now+10)
	if err != nil {
		t.Fatalf("claim probe: %v", err)
	}
	if claimed {
		t.Error("expected reminder flag to already be set; second claim should fail")
	}
}

// TestCommunityPurgeReminderJob_ActiveCommunityIgnored: an active
// community is never selected, even if it has an old created_at.
func TestCommunityPurgeReminderJob_ActiveCommunityIgnored(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	if _, err := db.Insert(ctx, &models.Community{
		Name: "Active", CreatorId: "u", OwnerUserId: "u",
		// Created long ago, but never deleted.
		CreatedAtUnixSec: 1700000000,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	mockNotif := notifications.NewMockService()
	if err := NewCommunityPurgeReminderJob(db, mockNotif).Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := len(mockNotif.GetCalls()); got != 0 {
		t.Errorf("expected 0 pushes for active community, got %d", got)
	}
}

// TestCommunityPurgeReminderJob_RestoreClearsFlag: the reminder
// flag is cleared when the community is restored, so a
// delete → restore → delete cycle gets a fresh reminder window.
// The storage layer's behaviour here is what matters; this test
// exercises it directly via Insert/Update without going through
// the full RestoreCommunity RPC (covered separately in #1655 and
// #1656 round-trip tests).
func TestCommunityPurgeReminderJob_RestoreClearsFlag(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	now := time.Now().Unix()

	const owner = "u"
	const other = "o"
	communityID, err := db.Insert(ctx, &models.Community{
		Id:   uuid.New().String(),
		Name: "Cycle", CreatorId: owner, OwnerUserId: owner,
		Deleted: &models.DeletedMetadata{
			DeletedByUserId:  owner,
			DeletedAtUnixSec: now - communitylib.PurgeReminderLeadSeconds - 100,
		},
		DeletedSnapshot: &models.CommunityDeletedSnapshot{
			MemberUserIds: []string{owner, other},
		},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// First run: dispatches.
	mockNotif := notifications.NewMockService()
	job := NewCommunityPurgeReminderJob(db, mockNotif)
	if err := job.Run(ctx); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if len(mockNotif.GetCalls()) != 2 {
		t.Fatalf("expected 2 pushes from first run, got %d", len(mockNotif.GetCalls()))
	}

	// Simulate restore: ClaimCommunityRestore clears all three
	// flat columns (deleted_*, purge_reminder_sent_at_unix_sec)
	// in one conditional UPDATE, then storage.Update rewrites the
	// proto layer to match. This mirrors what RestoreCommunity
	// (#1655) does in the service handler.
	claimed, err := db.ClaimCommunityRestore(ctx, communityID)
	if err != nil {
		t.Fatalf("claim restore: %v", err)
	}
	if !claimed {
		t.Fatal("expected restore claim to succeed")
	}
	got := &models.Community{}
	if err := db.GetByID(ctx, communityID, got, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("re-fetch: %v", err)
	}
	got.Deleted = nil
	got.DeletedSnapshot = nil
	got.PurgeReminderSentAtUnixSec = nil
	got.OwnerUserId = owner
	if err := db.Update(ctx, got); err != nil {
		t.Fatalf("update for restore: %v", err)
	}

	// Soft-delete again, 29.5 days ago.
	got = &models.Community{}
	if err := db.GetByID(ctx, communityID, got); err != nil {
		t.Fatalf("post-restore re-fetch: %v", err)
	}
	got.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  owner,
		DeletedAtUnixSec: now - communitylib.PurgeReminderLeadSeconds - 50,
	}
	got.DeletedSnapshot = &models.CommunityDeletedSnapshot{
		MemberUserIds: []string{owner, other},
	}
	if err := db.Update(ctx, got); err != nil {
		t.Fatalf("re-delete update: %v", err)
	}

	// Second run: should dispatch again because the reminder flag was cleared on restore.
	mockNotif.Reset()
	if err := job.Run(ctx); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if len(mockNotif.GetCalls()) != 2 {
		t.Errorf("expected 2 pushes from second run after restore cycle, got %d", len(mockNotif.GetCalls()))
	}
}
