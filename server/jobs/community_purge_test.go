package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// purgeFixtures creates a soft-deleted community deleted
// (30 + grace) days ago and seeds one row in every cascade
// table. Returns the community ID. The caller is responsible
// for choosing whether it is past the purge window.
func purgeFixtures(t *testing.T, db *storage.ProtoSQLStorage, deletedAtUnixSec int64) string {
	t.Helper()
	ctx := context.Background()

	communityID, err := db.Insert(ctx, &models.Community{
		Name: "to-purge", OwnerUserId: "owner",
		Deleted:         &models.DeletedMetadata{DeletedByUserId: "owner", DeletedAtUnixSec: deletedAtUnixSec},
		DeletedSnapshot: &models.CommunityDeletedSnapshot{MemberUserIds: []string{"owner", "alice"}},
	})
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityUser{CommunityId: communityID, UserId: "u-1"}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityGear{CommunityId: communityID, GearId: "g-1"}); err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityRequest{CommunityId: communityID, RequestId: "r-1"}); err != nil {
		t.Fatalf("insert request: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityExperience{CommunityId: communityID, ExperienceId: "e-1"}); err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityNotificationPreferences{CommunityId: communityID, UserId: "u-1"}); err != nil {
		t.Fatalf("insert notif prefs: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityInvitationLink{CommunityId: communityID, ShortCode: "code"}); err != nil {
		t.Fatalf("insert invite link: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityEvent{CommunityId: communityID, EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT}); err != nil {
		t.Fatalf("insert event: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityRegion{CommunityId: communityID, RegionId: "region-1"}); err != nil {
		t.Fatalf("insert region: %v", err)
	}
	if _, err := db.Insert(ctx, &models.Story{CommunityId: communityID}); err != nil {
		t.Fatalf("insert story: %v", err)
	}
	if _, err := db.Insert(ctx, &models.StoredNudge{CommunityId: communityID}); err != nil {
		t.Fatalf("insert nudge: %v", err)
	}
	if _, err := db.Insert(ctx, &models.FeedItemView{CommunityId: communityID, UserId: "u-1", FeedItemId: "fi-1"}); err != nil {
		t.Fatalf("insert feed item view: %v", err)
	}
	convID, err := db.Insert(ctx, &models.ChatConversation{CommunityId: communityID})
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err := db.Insert(ctx, &models.ChatMessage{ConversationId: convID}); err != nil {
		t.Fatalf("insert message: %v", err)
	}
	return communityID
}

// assertCommunityPurged checks every cascade table is empty for
// communityID and the parent row is gone.
func assertCommunityPurged(t *testing.T, db *storage.ProtoSQLStorage, communityID string) {
	t.Helper()
	ctx := context.Background()

	tables := []struct {
		name string
		msg  func() interface{ Reset() }
	}{}
	_ = tables // unused; using direct calls below for clarity

	checks := []struct {
		field string
		query func() (int, error)
	}{
		{"community_user", func() (int, error) {
			rows, err := db.QueryByField(ctx, "community_id", communityID, &models.CommunityUser{}, storage.QueryOptions{IncludeDeleted: true})
			return len(rows), err
		}},
		{"community_gear", func() (int, error) {
			rows, err := db.QueryByField(ctx, "community_id", communityID, &models.CommunityGear{}, storage.QueryOptions{IncludeDeleted: true})
			return len(rows), err
		}},
		{"community_request", func() (int, error) {
			rows, err := db.QueryByField(ctx, "community_id", communityID, &models.CommunityRequest{}, storage.QueryOptions{IncludeDeleted: true})
			return len(rows), err
		}},
		{"community_experience", func() (int, error) {
			rows, err := db.QueryByField(ctx, "community_id", communityID, &models.CommunityExperience{}, storage.QueryOptions{IncludeDeleted: true})
			return len(rows), err
		}},
		{"community_notification_preferences", func() (int, error) {
			rows, err := db.QueryByField(ctx, "community_id", communityID, &models.CommunityNotificationPreferences{}, storage.QueryOptions{IncludeDeleted: true})
			return len(rows), err
		}},
		{"community_invitation_link", func() (int, error) {
			rows, err := db.QueryByField(ctx, "community_id", communityID, &models.CommunityInvitationLink{}, storage.QueryOptions{IncludeDeleted: true})
			return len(rows), err
		}},
		{"community_event", func() (int, error) {
			rows, err := db.QueryByField(ctx, "community_id", communityID, &models.CommunityEvent{}, storage.QueryOptions{IncludeDeleted: true})
			return len(rows), err
		}},
		{"community_region", func() (int, error) {
			rows, err := db.QueryByField(ctx, "community_id", communityID, &models.CommunityRegion{}, storage.QueryOptions{IncludeDeleted: true})
			return len(rows), err
		}},
		{"Story", func() (int, error) {
			rows, err := db.QueryByField(ctx, "community_id", communityID, &models.Story{}, storage.QueryOptions{IncludeDeleted: true})
			return len(rows), err
		}},
		{"stored_nudge", func() (int, error) {
			rows, err := db.QueryByField(ctx, "community_id", communityID, &models.StoredNudge{}, storage.QueryOptions{IncludeDeleted: true})
			return len(rows), err
		}},
		{"FeedItemView", func() (int, error) {
			rows, err := db.QueryByField(ctx, "community_id", communityID, &models.FeedItemView{}, storage.QueryOptions{IncludeDeleted: true})
			return len(rows), err
		}},
		{"chat_conversation", func() (int, error) {
			rows, err := db.QueryByField(ctx, "community_id", communityID, &models.ChatConversation{}, storage.QueryOptions{IncludeDeleted: true})
			return len(rows), err
		}},
	}
	for _, c := range checks {
		got, err := c.query()
		if err != nil {
			t.Fatalf("query %s: %v", c.field, err)
		}
		if got != 0 {
			t.Errorf("table %s: %d rows remain after purge, want 0", c.field, got)
		}
	}

	got := &models.Community{}
	if err := db.GetByID(ctx, communityID, got, storage.QueryOptions{IncludeDeleted: true}); err == nil {
		t.Errorf("community %s still exists after purge", communityID)
	}
}

// TestCommunityPurgeJob_HappyPath: a community whose
// deleted_at is past PurgeWindowSeconds is hard-deleted with its
// cascade; an audit row is written; an in-window community is
// untouched.
func TestCommunityPurgeJob_HappyPath(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	now := time.Now().Unix()

	expiredID := purgeFixtures(t, db, now-communitylib.PurgeWindowSeconds-3600)

	freshID, err := db.Insert(ctx, &models.Community{
		Name: "fresh", OwnerUserId: "owner",
		Deleted: &models.DeletedMetadata{DeletedAtUnixSec: now - 3600},
	})
	if err != nil {
		t.Fatalf("seed fresh: %v", err)
	}

	if err := NewCommunityPurgeJob(db, false, false).Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	assertCommunityPurged(t, db, expiredID)

	// Fresh community remains.
	got := &models.Community{}
	if err := db.GetByID(ctx, freshID, got, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Errorf("fresh community vanished: %v", err)
	}

	// Audit row exists with non-empty per-table counts.
	audits, err := db.QueryByField(ctx, "community_id", expiredID, &models.CommunityPurgeAudit{}, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if len(audits) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(audits))
	}
	audit := audits[0].(*models.CommunityPurgeAudit)
	if audit.PerTableRowCounts == nil {
		t.Errorf("audit per_table_row_counts is nil")
	}
	if audit.PerTableRowCounts["community_user"] != 1 {
		t.Errorf("audit per_table_row_counts[community_user] = %d, want 1", audit.PerTableRowCounts["community_user"])
	}
	if len(audit.SnapshotMemberUserIds) != 2 {
		t.Errorf("audit snapshot_member_user_ids = %v, want 2 entries", audit.SnapshotMemberUserIds)
	}

	// Second run is a no-op (idempotent).
	if err := NewCommunityPurgeJob(db, false, false).Run(ctx); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	audits2, err := db.QueryByField(ctx, "community_id", expiredID, &models.CommunityPurgeAudit{}, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("post-rerun audit query: %v", err)
	}
	if len(audits2) != 1 {
		t.Errorf("audit rows after re-run = %d, want 1 (still just the original)", len(audits2))
	}
}

// TestCommunityPurgeJob_ThresholdGate: when the candidate count
// exceeds the percent-of-total threshold the run aborts and
// nothing is deleted.
func TestCommunityPurgeJob_ThresholdGate(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	now := time.Now().Unix()

	// Five expired communities; total = 5; threshold =
	// max(thresholdAbsoluteFloor=5, 0.05*5=0) = 5. Five
	// candidates is NOT > 5, so threshold passes. Add a
	// sixth: now total=6, threshold=5; six > 5, abort fires.
	for i := 0; i < 6; i++ {
		_, err := db.Insert(ctx, &models.Community{
			Name: "expired", OwnerUserId: "owner",
			Deleted: &models.DeletedMetadata{DeletedAtUnixSec: now - communitylib.PurgeWindowSeconds - 100},
		})
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	err := NewCommunityPurgeJob(db, false, false).Run(ctx)
	if err == nil {
		t.Fatalf("expected threshold-exceeded error, got nil")
	}

	// Nothing was deleted; all six communities still exist.
	for i := 0; i < 6; i++ {
		// Actually, easier: count.
	}
	count, err := db.CountCommunities(ctx)
	if err != nil {
		t.Fatalf("CountCommunities: %v", err)
	}
	if count != 6 {
		t.Errorf("communities after aborted run = %d, want 6 (all should remain)", count)
	}

	// And no audit rows.
	all, err := db.QueryByField(ctx, "community_id", "", &models.CommunityPurgeAudit{}, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("audit rows = %d, want 0 (threshold abort writes none)", len(all))
	}
}

// TestCommunityPurgeJob_DryRun: when dryRun=true, no rows are
// deleted and no audit row is committed, but the cascade runs
// (and rolls back).
func TestCommunityPurgeJob_DryRun(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	now := time.Now().Unix()

	communityID := purgeFixtures(t, db, now-communitylib.PurgeWindowSeconds-3600)

	if err := NewCommunityPurgeJob(db, false, true).Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Community still exists.
	got := &models.Community{}
	if err := db.GetByID(ctx, communityID, got, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Errorf("community vanished in dry-run: %v", err)
	}

	// Cascade rows still present.
	users, err := db.QueryByField(ctx, "community_id", communityID, &models.CommunityUser{}, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("query users: %v", err)
	}
	if len(users) != 1 {
		t.Errorf("CommunityUser rows in dry-run = %d, want 1", len(users))
	}

	// No audit row committed.
	audits, err := db.QueryByField(ctx, "community_id", communityID, &models.CommunityPurgeAudit{}, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if len(audits) != 0 {
		t.Errorf("audit rows after dry-run = %d, want 0 (rolled back)", len(audits))
	}

	// Now disable dry-run and re-run; cascade actually fires.
	if err := NewCommunityPurgeJob(db, false, false).Run(ctx); err != nil {
		t.Fatalf("post-dry-run Run: %v", err)
	}
	assertCommunityPurged(t, db, communityID)
}

// TestCommunityPurgeJob_KillSwitch: when disabled=true, the job
// returns immediately without scanning candidates.
func TestCommunityPurgeJob_KillSwitch(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	now := time.Now().Unix()

	communityID := purgeFixtures(t, db, now-communitylib.PurgeWindowSeconds-3600)

	if err := NewCommunityPurgeJob(db, true, false).Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Nothing happened.
	got := &models.Community{}
	if err := db.GetByID(ctx, communityID, got, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Errorf("community deleted with kill switch on: %v", err)
	}
	users, err := db.QueryByField(ctx, "community_id", communityID, &models.CommunityUser{}, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("query users: %v", err)
	}
	if len(users) != 1 {
		t.Errorf("CommunityUser rows with kill switch = %d, want 1", len(users))
	}
}

// TestCommunityPurgeJob_RestoreRace: a community whose deleted
// flag is cleared between candidate selection and the per-row
// re-check is skipped.
//
// Implementation: we can't easily inject a hook between
// candidate-fetch and re-check, so we simulate by mutating the
// community's deleted state after seeding to "fresh-deleted" but
// passing it as a candidate via direct call. The simplest pin is
// to seed with DeletedAtUnixSec inside the purge window (so
// FindExpiredCommunities does not return it), demonstrating that
// the candidate set excludes restored-during-window rows.
//
// To exercise the in-tx re-check specifically, we call the
// per-community helper directly with a candidate whose live row
// has been restored.
func TestCommunityPurgeJob_RestoreRace(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	now := time.Now().Unix()

	communityID := purgeFixtures(t, db, now-communitylib.PurgeWindowSeconds-3600)

	// Build the candidate snapshot (what FindExpiredCommunities
	// would return).
	candidate := &models.Community{}
	if err := db.GetByID(ctx, communityID, candidate, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// Now simulate a concurrent restore: clear the deleted
	// metadata in storage. The candidate snapshot still has
	// Deleted populated, but the re-check inside the tx will
	// fetch fresh state and notice the mismatch.
	candidate.Deleted = candidate.GetDeleted() // keep snapshot intact
	live := &models.Community{}
	if err := db.GetByID(ctx, communityID, live, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("get live: %v", err)
	}
	live.Deleted = nil
	if err := db.Update(ctx, live); err != nil {
		t.Fatalf("simulate restore: %v", err)
	}

	job := NewCommunityPurgeJob(db, false, false)
	err := job.purgeCommunity(ctx, candidate, false, now)
	if !errors.Is(err, errSkipRestoreRace) {
		t.Fatalf("purgeCommunity should return errSkipRestoreRace on restore race, got: %v", err)
	}

	// Community still exists; cascade rows untouched.
	got := &models.Community{}
	if err := db.GetByID(ctx, communityID, got, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Errorf("community deleted despite restore race: %v", err)
	}
	users, err := db.QueryByField(ctx, "community_id", communityID, &models.CommunityUser{}, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("query users: %v", err)
	}
	if len(users) != 1 {
		t.Errorf("CommunityUser rows after restore-race = %d, want 1", len(users))
	}
}

// TestCommunityPurgeJob_MembershipsExpired: a soft-deleted
// CommunityUser row whose deleted_at is older than
// RejoinWindowSeconds is hard-deleted; an in-window membership is
// preserved.
func TestCommunityPurgeJob_MembershipsExpired(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	now := time.Now().Unix()

	activeCommunityID, err := db.Insert(ctx, &models.Community{Name: "active", OwnerUserId: "owner"})
	if err != nil {
		t.Fatalf("seed community: %v", err)
	}

	expiredMembershipID, err := db.Insert(ctx, &models.CommunityUser{
		CommunityId: activeCommunityID, UserId: "expired-leaver",
		Deleted: &models.DeletedMetadata{DeletedAtUnixSec: now - communitylib.RejoinWindowSeconds - 3600},
	})
	if err != nil {
		t.Fatalf("seed expired membership: %v", err)
	}
	freshMembershipID, err := db.Insert(ctx, &models.CommunityUser{
		CommunityId: activeCommunityID, UserId: "fresh-leaver",
		Deleted: &models.DeletedMetadata{DeletedAtUnixSec: now - 3600},
	})
	if err != nil {
		t.Fatalf("seed fresh membership: %v", err)
	}

	if err := NewCommunityPurgeJob(db, false, false).Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Expired membership is gone.
	gone := &models.CommunityUser{}
	if err := db.GetByID(ctx, expiredMembershipID, gone, storage.QueryOptions{IncludeDeleted: true}); err == nil {
		t.Errorf("expired membership %s still exists", expiredMembershipID)
	}
	// Fresh membership remains.
	fresh := &models.CommunityUser{}
	if err := db.GetByID(ctx, freshMembershipID, fresh, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Errorf("fresh membership vanished: %v", err)
	}
}
