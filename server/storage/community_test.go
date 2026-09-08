package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestGetCommunityWithMembership(t *testing.T) {
	ctx := context.Background()
	s, cleanup := SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := uuid.New().String()
	memberID := uuid.New().String()
	nonMemberID := uuid.New().String()

	community := &models.Community{
		Id:          communityID,
		Name:        "Test Community",
		CreatorId:   memberID,
		OwnerUserId: memberID,
	}
	if _, err := s.Insert(ctx, community); err != nil {
		t.Fatalf("insert community: %v", err)
	}

	membership := &models.CommunityUser{
		Id:          uuid.New().String(),
		CommunityId: communityID,
		UserId:      memberID,
	}
	if _, err := s.Insert(ctx, membership); err != nil {
		t.Fatalf("insert membership: %v", err)
	}

	t.Run("active community, member", func(t *testing.T) {
		c, m, err := s.GetCommunityWithMembership(ctx, communityID, memberID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if c == nil || c.Id != communityID {
			t.Fatalf("expected community %s, got %+v", communityID, c)
		}
		if m == nil || m.UserId != memberID {
			t.Fatalf("expected membership for %s, got %+v", memberID, m)
		}
		if c.Deleted != nil {
			t.Fatalf("expected non-deleted community, got Deleted=%+v", c.Deleted)
		}
	})

	t.Run("active community, non-member returns nil membership", func(t *testing.T) {
		c, m, err := s.GetCommunityWithMembership(ctx, communityID, nonMemberID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if c == nil {
			t.Fatalf("expected community, got nil")
		}
		if m != nil {
			t.Fatalf("expected nil membership for non-member, got %+v", m)
		}
	})

	t.Run("missing community returns ErrRecordNotFound", func(t *testing.T) {
		_, _, err := s.GetCommunityWithMembership(ctx, uuid.New().String(), memberID)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !errors.Is(err, ErrRecordNotFound) {
			t.Fatalf("expected ErrRecordNotFound, got %v", err)
		}
	})

	t.Run("soft-deleted community is still returned with Deleted populated", func(t *testing.T) {
		// Force-flip community to soft-deleted state.
		community.Deleted = &models.DeletedMetadata{
			DeletedByUserId:  memberID,
			DeletedAtUnixSec: 1700000000,
		}
		if err := s.Update(ctx, community); err != nil {
			t.Fatalf("update community to deleted: %v", err)
		}

		c, m, err := s.GetCommunityWithMembership(ctx, communityID, memberID)
		if err != nil {
			t.Fatalf("expected no error for soft-deleted community, got %v", err)
		}
		if c == nil || c.Deleted == nil || c.Deleted.DeletedAtUnixSec == 0 {
			t.Fatalf("expected soft-deleted community, got %+v", c)
		}
		if m == nil {
			t.Fatalf("expected membership preserved across soft-delete, got nil")
		}
	})

	t.Run("single round-trip", func(t *testing.T) {
		statsCtx := WithQueryStats(ctx)
		AssertMaxQueries(t, statsCtx, 1, func() {
			if _, _, err := s.GetCommunityWithMembership(statsCtx, communityID, memberID); err != nil {
				t.Fatalf("query failed: %v", err)
			}
		})
	})
}

func TestGetCommunitiesWithMembership_Batch(t *testing.T) {
	ctx := context.Background()
	s, cleanup := SetupTestStorage(t)
	t.Cleanup(cleanup)

	memberID := uuid.New().String()
	otherID := uuid.New().String()
	missingID := uuid.New().String()

	// Three communities: one the user belongs to, one they don't, one
	// soft-deleted.
	c1 := &models.Community{Id: uuid.New().String(), Name: "C1", CreatorId: memberID, OwnerUserId: memberID}
	c2 := &models.Community{Id: uuid.New().String(), Name: "C2", CreatorId: otherID, OwnerUserId: otherID}
	c3 := &models.Community{
		Id: uuid.New().String(), Name: "C3 deleted", CreatorId: memberID,
		Deleted: &models.DeletedMetadata{DeletedByUserId: memberID, DeletedAtUnixSec: 1700000000},
	}
	for _, c := range []*models.Community{c1, c2, c3} {
		if _, err := s.Insert(ctx, c); err != nil {
			t.Fatalf("insert community %s: %v", c.Name, err)
		}
	}
	if _, err := s.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: c1.Id, UserId: memberID,
	}); err != nil {
		t.Fatalf("insert membership: %v", err)
	}

	t.Run("returns one row per existing community", func(t *testing.T) {
		statsCtx := WithQueryStats(ctx)
		var pairs map[string]CommunityWithMembership
		AssertMaxQueries(t, statsCtx, 1, func() {
			var err error
			pairs, err = s.GetCommunitiesWithMembership(statsCtx,
				[]string{c1.Id, c2.Id, c3.Id, missingID}, memberID)
			if err != nil {
				t.Fatalf("batch query failed: %v", err)
			}
		})
		if len(pairs) != 3 {
			t.Fatalf("expected 3 rows (c1, c2, c3 — missingID dropped), got %d: %+v", len(pairs), pairs)
		}
		if pairs[c1.Id].Membership == nil {
			t.Errorf("expected membership for c1")
		}
		if pairs[c2.Id].Membership != nil {
			t.Errorf("expected nil membership for c2")
		}
		if pairs[c3.Id].Community.Deleted == nil {
			t.Errorf("expected c3 to be soft-deleted")
		}
		if _, exists := pairs[missingID]; exists {
			t.Errorf("missing community must not appear in result")
		}
	})

	t.Run("empty input returns empty map without query", func(t *testing.T) {
		statsCtx := WithQueryStats(ctx)
		AssertMaxQueries(t, statsCtx, 0, func() {
			pairs, err := s.GetCommunitiesWithMembership(statsCtx, nil, memberID)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if len(pairs) != 0 {
				t.Fatalf("expected empty map, got %+v", pairs)
			}
		})
	})
}

func TestClaimCommunityUserRejoin(t *testing.T) {
	ctx := context.Background()
	s, cleanup := SetupTestStorage(t)
	t.Cleanup(cleanup)

	const window = int64(30 * 24 * 60 * 60)
	now := time.Now().Unix()

	t.Run("succeeds when row is soft-deleted within window", func(t *testing.T) {
		commID := uuid.New().String()
		userID := uuid.New().String()
		if _, err := s.Insert(ctx, &models.CommunityUser{
			CommunityId: commID, UserId: userID,
			Deleted: &models.DeletedMetadata{DeletedByUserId: userID, DeletedAtUnixSec: now - 100},
		}); err != nil {
			t.Fatalf("insert: %v", err)
		}

		claimed, err := s.ClaimCommunityUserRejoin(ctx, commID, userID, now, window)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if !claimed {
			t.Fatal("expected claim to succeed")
		}

		// Idempotent: second call should fail because row is now active.
		again, err := s.ClaimCommunityUserRejoin(ctx, commID, userID, now, window)
		if err != nil {
			t.Fatalf("second claim: %v", err)
		}
		if again {
			t.Error("expected second claim to fail (row already active)")
		}
	})

	t.Run("fails on already-active row", func(t *testing.T) {
		commID := uuid.New().String()
		userID := uuid.New().String()
		if _, err := s.Insert(ctx, &models.CommunityUser{
			CommunityId: commID, UserId: userID,
		}); err != nil {
			t.Fatalf("insert: %v", err)
		}

		claimed, err := s.ClaimCommunityUserRejoin(ctx, commID, userID, now, window)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if claimed {
			t.Error("expected claim to fail; row is already active")
		}
	})

	t.Run("fails on past-window row", func(t *testing.T) {
		commID := uuid.New().String()
		userID := uuid.New().String()
		// 31 days ago
		if _, err := s.Insert(ctx, &models.CommunityUser{
			CommunityId: commID, UserId: userID,
			Deleted: &models.DeletedMetadata{DeletedByUserId: userID, DeletedAtUnixSec: now - window - 24*60*60},
		}); err != nil {
			t.Fatalf("insert: %v", err)
		}

		claimed, err := s.ClaimCommunityUserRejoin(ctx, commID, userID, now, window)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if claimed {
			t.Error("expected claim to fail; row is past window")
		}
	})

	t.Run("fails on missing row", func(t *testing.T) {
		commID := uuid.New().String()
		userID := uuid.New().String()
		claimed, err := s.ClaimCommunityUserRejoin(ctx, commID, userID, now, window)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if claimed {
			t.Error("expected claim to fail; no such row")
		}
	})

	t.Run("inclusive boundary at exactly window seconds", func(t *testing.T) {
		commID := uuid.New().String()
		userID := uuid.New().String()
		// deleted_at = now - window exactly → inclusive, should succeed.
		if _, err := s.Insert(ctx, &models.CommunityUser{
			CommunityId: commID, UserId: userID,
			Deleted: &models.DeletedMetadata{DeletedByUserId: userID, DeletedAtUnixSec: now - window},
		}); err != nil {
			t.Fatalf("insert: %v", err)
		}

		claimed, err := s.ClaimCommunityUserRejoin(ctx, commID, userID, now, window)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if !claimed {
			t.Error("expected claim to succeed at exact window boundary (inclusive)")
		}
	})
}

func TestProbeCommunityUserDeletedAt(t *testing.T) {
	ctx := context.Background()
	s, cleanup := SetupTestStorage(t)
	t.Cleanup(cleanup)

	t.Run("missing row returns found=false", func(t *testing.T) {
		commID := uuid.New().String()
		userID := uuid.New().String()
		deletedAt, found, err := s.ProbeCommunityUserDeletedAt(ctx, commID, userID)
		if err != nil {
			t.Fatalf("probe: %v", err)
		}
		if found {
			t.Errorf("expected found=false, got found=true deletedAt=%d", deletedAt)
		}
	})

	t.Run("active row returns found=true, deletedAt=0", func(t *testing.T) {
		commID := uuid.New().String()
		userID := uuid.New().String()
		if _, err := s.Insert(ctx, &models.CommunityUser{
			CommunityId: commID, UserId: userID,
		}); err != nil {
			t.Fatalf("insert: %v", err)
		}
		deletedAt, found, err := s.ProbeCommunityUserDeletedAt(ctx, commID, userID)
		if err != nil {
			t.Fatalf("probe: %v", err)
		}
		if !found {
			t.Fatal("expected found=true")
		}
		if deletedAt != 0 {
			t.Errorf("expected deletedAt=0 for active row, got %d", deletedAt)
		}
	})

	t.Run("soft-deleted row returns the deletedAt timestamp", func(t *testing.T) {
		commID := uuid.New().String()
		userID := uuid.New().String()
		want := int64(1700000000)
		if _, err := s.Insert(ctx, &models.CommunityUser{
			CommunityId: commID, UserId: userID,
			Deleted: &models.DeletedMetadata{DeletedByUserId: userID, DeletedAtUnixSec: want},
		}); err != nil {
			t.Fatalf("insert: %v", err)
		}
		deletedAt, found, err := s.ProbeCommunityUserDeletedAt(ctx, commID, userID)
		if err != nil {
			t.Fatalf("probe: %v", err)
		}
		if !found {
			t.Fatal("expected found=true")
		}
		if deletedAt != want {
			t.Errorf("deletedAt = %d, want %d", deletedAt, want)
		}
	})
}

func TestClaimCommunityPurgeReminder(t *testing.T) {
	ctx := context.Background()
	s, cleanup := SetupTestStorage(t)
	t.Cleanup(cleanup)

	now := time.Now().Unix()

	t.Run("succeeds on soft-deleted community with unset reminder", func(t *testing.T) {
		commID, err := s.Insert(ctx, &models.Community{
			Name: "C", CreatorId: "owner", OwnerUserId: "owner",
			Deleted: &models.DeletedMetadata{DeletedByUserId: "owner", DeletedAtUnixSec: now - 100},
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		claimed, err := s.ClaimCommunityPurgeReminder(ctx, commID, now)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if !claimed {
			t.Fatal("expected claim to succeed")
		}
		again, err := s.ClaimCommunityPurgeReminder(ctx, commID, now+10)
		if err != nil {
			t.Fatalf("second claim: %v", err)
		}
		if again {
			t.Error("expected second claim to fail (set-once)")
		}
	})

	t.Run("fails on active community", func(t *testing.T) {
		commID, err := s.Insert(ctx, &models.Community{
			Name: "Active", CreatorId: "owner", OwnerUserId: "owner",
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		claimed, err := s.ClaimCommunityPurgeReminder(ctx, commID, now)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if claimed {
			t.Error("expected claim to fail; community is active")
		}
	})

	t.Run("fails when reminder is already set", func(t *testing.T) {
		alreadySet := int64(1700000000)
		commID, err := s.Insert(ctx, &models.Community{
			Name: "C", CreatorId: "owner", OwnerUserId: "owner",
			Deleted:                    &models.DeletedMetadata{DeletedByUserId: "owner", DeletedAtUnixSec: now - 100},
			PurgeReminderSentAtUnixSec: &alreadySet,
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		claimed, err := s.ClaimCommunityPurgeReminder(ctx, commID, now)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if claimed {
			t.Error("expected claim to fail; reminder already set")
		}
	})
}

func TestFindCommunitiesNeedingPurgeReminder(t *testing.T) {
	ctx := context.Background()
	s, cleanup := SetupTestStorage(t)
	t.Cleanup(cleanup)

	now := time.Now().Unix()
	const lead = int64(29 * 24 * 60 * 60)

	eligibleID, err := s.Insert(ctx, &models.Community{
		Name: "Eligible", CreatorId: "u", OwnerUserId: "u",
		Deleted: &models.DeletedMetadata{DeletedByUserId: "u", DeletedAtUnixSec: now - lead - (12 * 60 * 60)},
	})
	if err != nil {
		t.Fatalf("insert eligible: %v", err)
	}
	tooRecentID, err := s.Insert(ctx, &models.Community{
		Name: "TooRecent", CreatorId: "u", OwnerUserId: "u",
		Deleted: &models.DeletedMetadata{DeletedByUserId: "u", DeletedAtUnixSec: now - (28 * 24 * 60 * 60)},
	})
	if err != nil {
		t.Fatalf("insert too-recent: %v", err)
	}
	alreadyTime := int64(1700000000)
	alreadyID, err := s.Insert(ctx, &models.Community{
		Name: "Already", CreatorId: "u", OwnerUserId: "u",
		Deleted:                    &models.DeletedMetadata{DeletedByUserId: "u", DeletedAtUnixSec: now - lead - 100},
		PurgeReminderSentAtUnixSec: &alreadyTime,
	})
	if err != nil {
		t.Fatalf("insert already-reminded: %v", err)
	}
	activeID, err := s.Insert(ctx, &models.Community{
		Name: "Active", CreatorId: "u", OwnerUserId: "u",
	})
	if err != nil {
		t.Fatalf("insert active: %v", err)
	}
	stragglerID, err := s.Insert(ctx, &models.Community{
		Name: "Straggler", CreatorId: "u", OwnerUserId: "u",
		Deleted: &models.DeletedMetadata{DeletedByUserId: "u", DeletedAtUnixSec: now - (35 * 24 * 60 * 60)},
	})
	if err != nil {
		t.Fatalf("insert straggler: %v", err)
	}

	got, err := s.FindCommunitiesNeedingPurgeReminder(ctx, now, lead, 1000)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	gotIDs := make(map[string]bool, len(got))
	for _, c := range got {
		gotIDs[c.Id] = true
	}
	if !gotIDs[eligibleID] {
		t.Errorf("expected eligible community %q in results", eligibleID)
	}
	if !gotIDs[stragglerID] {
		t.Errorf("expected straggler community %q in results", stragglerID)
	}
	if gotIDs[tooRecentID] {
		t.Errorf("did not expect too-recent community %q in results", tooRecentID)
	}
	if gotIDs[alreadyID] {
		t.Errorf("did not expect already-reminded community %q in results", alreadyID)
	}
	if gotIDs[activeID] {
		t.Errorf("did not expect active community %q in results", activeID)
	}

	limited, err := s.FindCommunitiesNeedingPurgeReminder(ctx, now, lead, 1)
	if err != nil {
		t.Fatalf("limited find: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected exactly 1 result with limit=1, got %d", len(limited))
	}
	if limited[0].Id != stragglerID {
		t.Errorf("expected straggler %q to come first by ASC deleted_at, got %q", stragglerID, limited[0].Id)
	}

	if _, err := s.FindCommunitiesNeedingPurgeReminder(ctx, now, lead, 0); err == nil {
		t.Error("expected error for limit=0")
	}
}

func TestSnapshotMembersAutoSync(t *testing.T) {
	ctx := context.Background()
	s, cleanup := SetupTestStorage(t)
	t.Cleanup(cleanup)

	// Insert with a snapshot but no Deleted metadata. The auto-sync
	// (registered for Community.snapshot_member_user_ids) populates
	// the indexed column, but FindCommunitiesEligibleForRestore must
	// still skip the row because the community isn't soft-deleted.
	commID, err := s.Insert(ctx, &models.Community{
		Name: "C", CreatorId: "owner", OwnerUserId: "owner",
		DeletedSnapshot: &models.CommunityDeletedSnapshot{
			MemberUserIds: []string{"u1", "u2", "u3"},
		},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := s.FindCommunitiesEligibleForRestore(ctx, "u2", 100)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("active community returned by restore query, got %d", len(got))
	}

	// Soft-delete via Update — the same auto-sync keeps the column
	// in step with the proto's DeletedSnapshot (no separate write).
	community := &models.Community{}
	if err := s.GetByID(ctx, commID, community); err != nil {
		t.Fatalf("re-fetch: %v", err)
	}
	community.Deleted = &models.DeletedMetadata{
		DeletedByUserId: "owner", DeletedAtUnixSec: 1700000000,
	}
	if err := s.Update(ctx, community); err != nil {
		t.Fatalf("soft-delete via update: %v", err)
	}

	got, err = s.FindCommunitiesEligibleForRestore(ctx, "u2", 100)
	if err != nil {
		t.Fatalf("find post-delete: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result for snapshot member u2, got %d", len(got))
	}
	if got[0].Id != commID {
		t.Errorf("got community %q, want %q", got[0].Id, commID)
	}
}

func TestFindCommunitiesEligibleForRestore(t *testing.T) {
	ctx := context.Background()
	s, cleanup := SetupTestStorage(t)
	t.Cleanup(cleanup)

	const member = "user-member"
	const other = "user-other"
	const stranger = "user-stranger"

	// Eligible: deleted community with member in snapshot.
	eligibleID, err := s.Insert(ctx, &models.Community{
		Name: "Eligible", CreatorId: member, OwnerUserId: member,
		Deleted:         &models.DeletedMetadata{DeletedByUserId: member, DeletedAtUnixSec: 1700000100},
		DeletedSnapshot: &models.CommunityDeletedSnapshot{MemberUserIds: []string{member, other}},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Older eligible: same shape, different deleted_at to verify ordering.
	olderID, err := s.Insert(ctx, &models.Community{
		Name: "Older Eligible", CreatorId: member, OwnerUserId: member,
		Deleted:         &models.DeletedMetadata{DeletedByUserId: member, DeletedAtUnixSec: 1700000050},
		DeletedSnapshot: &models.CommunityDeletedSnapshot{MemberUserIds: []string{member}},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Active community with member in snapshot — must NOT appear
	// because the row is not soft-deleted.
	activeID, err := s.Insert(ctx, &models.Community{
		Name: "Active", CreatorId: member, OwnerUserId: member,
		DeletedSnapshot: &models.CommunityDeletedSnapshot{MemberUserIds: []string{member}},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Deleted community where member is NOT in snapshot.
	notEligibleID, err := s.Insert(ctx, &models.Community{
		Name: "NotEligible", CreatorId: stranger, OwnerUserId: stranger,
		Deleted:         &models.DeletedMetadata{DeletedByUserId: stranger, DeletedAtUnixSec: 1700000080},
		DeletedSnapshot: &models.CommunityDeletedSnapshot{MemberUserIds: []string{stranger, other}},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := s.FindCommunitiesEligibleForRestore(ctx, member, 100)
	if err != nil {
		t.Fatalf("find: %v", err)
	}

	gotIDs := make(map[string]int, len(got))
	for i, c := range got {
		gotIDs[c.Id] = i
	}
	if _, ok := gotIDs[eligibleID]; !ok {
		t.Error("expected eligible community in results")
	}
	if _, ok := gotIDs[olderID]; !ok {
		t.Error("expected older eligible community in results")
	}
	if _, ok := gotIDs[activeID]; ok {
		t.Error("did not expect active community in results")
	}
	if _, ok := gotIDs[notEligibleID]; ok {
		t.Error("did not expect not-in-snapshot community in results")
	}

	// Order: ASC by deleted_at — older first.
	if got[0].Id != olderID {
		t.Errorf("expected older community %q first, got %q", olderID, got[0].Id)
	}

	// Limit must be positive.
	if _, err := s.FindCommunitiesEligibleForRestore(ctx, member, 0); err == nil {
		t.Error("expected error for limit=0")
	}

	// Stranger sees no eligible communities (not in any snapshot member is = stranger but
	// stranger is in their own; but stranger's community is deleted; so stranger sees one).
	strangerResults, err := s.FindCommunitiesEligibleForRestore(ctx, stranger, 100)
	if err != nil {
		t.Fatalf("find for stranger: %v", err)
	}
	if len(strangerResults) != 1 || strangerResults[0].Id != notEligibleID {
		t.Errorf("expected stranger to see only their own deleted community, got %v", strangerResults)
	}
}

func TestClaimCommunityRestore_ClearsSnapshotMembers(t *testing.T) {
	ctx := context.Background()
	s, cleanup := SetupTestStorage(t)
	t.Cleanup(cleanup)

	const member = "user-snap-restorer"

	commID, err := s.Insert(ctx, &models.Community{
		Name: "C", CreatorId: member, OwnerUserId: member,
		Deleted:         &models.DeletedMetadata{DeletedByUserId: member, DeletedAtUnixSec: 1700000100},
		DeletedSnapshot: &models.CommunityDeletedSnapshot{MemberUserIds: []string{member}},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Pre-condition: member sees the community as eligible to restore.
	got, err := s.FindCommunitiesEligibleForRestore(ctx, member, 100)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 eligible community before restore, got %d", len(got))
	}

	// Claim the restore — flat columns (deleted_*, snapshot, purge_reminder) all clear.
	claimed, err := s.ClaimCommunityRestore(ctx, commID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !claimed {
		t.Fatal("expected claim to succeed")
	}

	// Post-condition: member no longer sees the community in the eligible list.
	got, err = s.FindCommunitiesEligibleForRestore(ctx, member, 100)
	if err != nil {
		t.Fatalf("find post-restore: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 eligible communities post-restore, got %d", len(got))
	}
}

func TestFindRejoinableCommunitiesForUser(t *testing.T) {
	ctx := context.Background()
	s, cleanup := SetupTestStorage(t)
	t.Cleanup(cleanup)

	now := time.Now().Unix()
	const window = int64(30 * 24 * 60 * 60)
	const userID = "user-rejoin"
	const ownerID = "user-rejoin-owner"

	// Helper: create community + (optionally soft-deleted) membership.
	mkCommunity := func(t *testing.T, label string, communityDeleted bool) string {
		t.Helper()
		c := &models.Community{
			Name: label, CreatorId: ownerID, OwnerUserId: ownerID,
		}
		if communityDeleted {
			c.Deleted = &models.DeletedMetadata{DeletedByUserId: ownerID, DeletedAtUnixSec: now - 100}
		}
		commID, err := s.Insert(ctx, c)
		if err != nil {
			t.Fatalf("%s: insert community: %v", label, err)
		}
		return commID
	}
	mkMembership := func(t *testing.T, commID string, deletedAt int64) {
		t.Helper()
		cu := &models.CommunityUser{
			CommunityId: commID, UserId: userID, InviterId: ownerID,
		}
		if deletedAt > 0 {
			cu.Deleted = &models.DeletedMetadata{DeletedByUserId: userID, DeletedAtUnixSec: deletedAt}
		}
		if _, err := s.Insert(ctx, cu); err != nil {
			t.Fatalf("insert membership for %s: %v", commID, err)
		}
	}

	// Eligible: active community, soft-deleted membership 100s ago.
	eligibleID := mkCommunity(t, "Eligible", false)
	mkMembership(t, eligibleID, now-100)

	// Eligible (older): active community, soft-deleted membership 1000s ago.
	olderID := mkCommunity(t, "OlderEligible", false)
	mkMembership(t, olderID, now-1000)

	// Past window: active community, membership left > 30 days ago.
	pastWindowID := mkCommunity(t, "PastWindow", false)
	mkMembership(t, pastWindowID, now-window-(24*60*60))

	// Community deleted: §2.7 cross-cut — must NOT appear.
	communityDeletedID := mkCommunity(t, "CommunityDeleted", true)
	mkMembership(t, communityDeletedID, now-200)

	// Active membership in active community: not soft-deleted, must NOT appear.
	activeMembershipID := mkCommunity(t, "ActiveMembership", false)
	mkMembership(t, activeMembershipID, 0)

	got, err := s.FindRejoinableCommunitiesForUser(ctx, userID, now, window, 100)
	if err != nil {
		t.Fatalf("find: %v", err)
	}

	gotIDs := make(map[string]int, len(got))
	for i, r := range got {
		gotIDs[r.Community.Id] = i
	}
	if _, ok := gotIDs[eligibleID]; !ok {
		t.Error("expected eligible community in results")
	}
	if _, ok := gotIDs[olderID]; !ok {
		t.Error("expected older eligible community in results")
	}
	if _, ok := gotIDs[pastWindowID]; ok {
		t.Error("did not expect past-window community in results")
	}
	if _, ok := gotIDs[communityDeletedID]; ok {
		t.Error("did not expect deleted-community in results (§2.7 cross-cut)")
	}
	if _, ok := gotIDs[activeMembershipID]; ok {
		t.Error("did not expect active-membership community in results")
	}

	// Order: most-recently-left first (DESC by deleted_at).
	if got[0].Community.Id != eligibleID {
		t.Errorf("expected most-recently-left %q first, got %q", eligibleID, got[0].Community.Id)
	}

	// Membership.Deleted.DeletedAtUnixSec is populated for left_at_unix_sec mapping.
	if got[0].Membership.GetDeleted().GetDeletedAtUnixSec() == 0 {
		t.Error("expected Membership.Deleted.DeletedAtUnixSec to be populated for left_at mapping")
	}

	// Inclusive boundary: a membership exactly windowSeconds old qualifies.
	boundaryCommID := mkCommunity(t, "Boundary", false)
	mkMembership(t, boundaryCommID, now-window)
	got, err = s.FindRejoinableCommunitiesForUser(ctx, userID, now, window, 100)
	if err != nil {
		t.Fatalf("find boundary: %v", err)
	}
	gotIDs = make(map[string]int, len(got))
	for i, r := range got {
		gotIDs[r.Community.Id] = i
	}
	if _, ok := gotIDs[boundaryCommID]; !ok {
		t.Error("expected boundary community at exactly window seconds to qualify (inclusive boundary)")
	}

	// Limit must be positive.
	if _, err := s.FindRejoinableCommunitiesForUser(ctx, userID, now, window, 0); err == nil {
		t.Error("expected error for limit=0")
	}

	// Limit caps results.
	limited, err := s.FindRejoinableCommunitiesForUser(ctx, userID, now, window, 1)
	if err != nil {
		t.Fatalf("find limited: %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("expected exactly 1 result with limit=1, got %d", len(limited))
	}
}
