package integration_tests

import (
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/clock"
	communitylib "go.ripls.org/ripls/server/community"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// TestRejoinCommunity_HappyPath: a leaver within the 30-day window
// rejoins, the soft-deleted CommunityUser is reactivated, and a
// MEMBER_REJOINED_WITHIN_WINDOW audit event is recorded. The
// original created_at_unix_sec is preserved (rejoin does not reset
// the historical "joined at" timestamp).
func TestRejoinCommunity_HappyPath(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	rejoinerID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia Owner")
	insertUser(t, db, rejoinerID, "rejoiner@example.com", "Riley Rejoiner")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "Test Community",
		CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed community: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: ownerID, InviterId: ownerID,
	}); err != nil {
		t.Fatalf("seed owner membership: %v", err)
	}
	originalJoinedAt := int64(1700000000)
	rejoinerMembershipID, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: rejoinerID, InviterId: ownerID,
		CreatedAtUnixSec: originalJoinedAt,
		Deleted:          &models.DeletedMetadata{DeletedByUserId: rejoinerID, DeletedAtUnixSec: time.Now().Unix() - 100},
	})
	if err != nil {
		t.Fatalf("seed soft-deleted membership: %v", err)
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	communitySvc, _ := newTestCommunityService(t, db, bucket, notifications.NewMockService())

	rejoinerCtx := makeAuthContext(rejoinerID, "rejoiner@example.com", models.Role_ROLE_USER)
	if _, err := communitySvc.RejoinCommunity(rejoinerCtx, connect.NewRequest(&api.RejoinCommunityRequest{
		CommunityId: communityID,
	})); err != nil {
		t.Fatalf("RejoinCommunity: %v", err)
	}

	// Membership is active again with original created_at_unix_sec preserved.
	got := &models.CommunityUser{}
	if err := db.GetByID(ctx, rejoinerMembershipID, got); err != nil {
		t.Fatalf("re-fetch membership: %v", err)
	}
	if got.Deleted != nil {
		t.Errorf("expected Deleted=nil after rejoin, got %+v", got.Deleted)
	}
	if got.CreatedAtUnixSec != originalJoinedAt {
		t.Errorf("CreatedAtUnixSec = %d, want preserved original %d", got.CreatedAtUnixSec, originalJoinedAt)
	}

	// MEMBER_REJOINED_WITHIN_WINDOW recorded with rejoiner as actor.
	events, _ := db.QueryByField(ctx, "community_id", communityID, &models.CommunityEvent{})
	var sawRejoined bool
	for _, e := range events {
		ev := e.(*models.CommunityEvent)
		if ev.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_REJOINED_WITHIN_WINDOW && ev.ActorId == rejoinerID {
			sawRejoined = true
		}
	}
	if !sawRejoined {
		t.Error("expected MEMBER_REJOINED_WITHIN_WINDOW event with rejoiner as actor")
	}
}

// TestRejoinCommunity_WindowExpired: leaver's soft-delete is older
// than 30 days → FailedPrecondition (rejoin_window_expired).
func TestRejoinCommunity_WindowExpired(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	rejoinerID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia Owner")
	insertUser(t, db, rejoinerID, "rejoiner@example.com", "Riley Rejoiner")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "C", CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: ownerID, InviterId: ownerID,
	}); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	// 31 days ago.
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: rejoinerID, InviterId: ownerID,
		Deleted: &models.DeletedMetadata{
			DeletedByUserId:  rejoinerID,
			DeletedAtUnixSec: time.Now().Unix() - communitylib.RejoinWindowSeconds - 24*60*60,
		},
	}); err != nil {
		t.Fatalf("seed expired: %v", err)
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	communitySvc, _ := newTestCommunityService(t, db, bucket, notifications.NewMockService())

	rejoinerCtx := makeAuthContext(rejoinerID, "rejoiner@example.com", models.Role_ROLE_USER)
	_, err := communitySvc.RejoinCommunity(rejoinerCtx, connect.NewRequest(&api.RejoinCommunityRequest{
		CommunityId: communityID,
	}))
	assertConnectCode(t, err, connect.CodeFailedPrecondition, "window expired")
	if err != nil && !contains(err.Error(), "rejoin_window_expired") {
		t.Errorf("expected reason 'rejoin_window_expired' in error, got: %v", err)
	}
}

// TestRejoinCommunity_CommunityDeleted is the §2.7 cross-cut: a
// leaver within their per-user 30-day window cannot rejoin a
// community in soft-deleted state. The community's deleted state
// trumps the per-user window.
func TestRejoinCommunity_CommunityDeleted(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	rejoinerID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia Owner")
	insertUser(t, db, rejoinerID, "rejoiner@example.com", "Riley Rejoiner")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "Deleted Community",
		CreatorId: ownerID, OwnerUserId: ownerID,
		Deleted: &models.DeletedMetadata{DeletedByUserId: ownerID, DeletedAtUnixSec: time.Now().Unix() - 100},
		DeletedSnapshot: &models.CommunityDeletedSnapshot{
			MemberUserIds: []string{ownerID},
		},
	}); err != nil {
		t.Fatalf("seed deleted community: %v", err)
	}
	// Rejoiner has a fresh-enough soft-delete (within 30 days) but
	// the community is deleted, so §2.7 blocks the rejoin.
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: rejoinerID, InviterId: ownerID,
		Deleted: &models.DeletedMetadata{DeletedByUserId: rejoinerID, DeletedAtUnixSec: time.Now().Unix() - 200},
	}); err != nil {
		t.Fatalf("seed rejoiner: %v", err)
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	communitySvc, _ := newTestCommunityService(t, db, bucket, notifications.NewMockService())

	rejoinerCtx := makeAuthContext(rejoinerID, "rejoiner@example.com", models.Role_ROLE_USER)
	_, err := communitySvc.RejoinCommunity(rejoinerCtx, connect.NewRequest(&api.RejoinCommunityRequest{
		CommunityId: communityID,
	}))
	assertConnectCode(t, err, connect.CodeFailedPrecondition, "community deleted")
	if err != nil && !contains(err.Error(), "community_deleted") {
		t.Errorf("expected reason 'community_deleted' in error, got: %v", err)
	}
}

// TestRejoinCommunity_AlreadyMember rejects a double-tap or stale-UI
// rejoin attempt against an active membership.
func TestRejoinCommunity_AlreadyMember(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	memberID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia Owner")
	insertUser(t, db, memberID, "member@example.com", "Mary Member")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "C", CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for _, uid := range []string{ownerID, memberID} {
		if _, err := db.Insert(ctx, &models.CommunityUser{
			Id: uuid.New().String(), CommunityId: communityID, UserId: uid, InviterId: ownerID,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	communitySvc, _ := newTestCommunityService(t, db, bucket, notifications.NewMockService())

	memberCtx := makeAuthContext(memberID, "member@example.com", models.Role_ROLE_USER)
	_, err := communitySvc.RejoinCommunity(memberCtx, connect.NewRequest(&api.RejoinCommunityRequest{
		CommunityId: communityID,
	}))
	assertConnectCode(t, err, connect.CodeFailedPrecondition, "already member")
	if err != nil && !contains(err.Error(), "already_member") {
		t.Errorf("expected reason 'already_member' in error, got: %v", err)
	}
}

// TestRejoinCommunity_NoMembership: stranger calls RejoinCommunity
// against a community they never joined → FailedPrecondition
// (no_membership_to_rejoin).
func TestRejoinCommunity_NoMembership(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	strangerID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia Owner")
	insertUser(t, db, strangerID, "stranger@example.com", "Sam Stranger")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "C", CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: ownerID, InviterId: ownerID,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	communitySvc, _ := newTestCommunityService(t, db, bucket, notifications.NewMockService())

	strangerCtx := makeAuthContext(strangerID, "stranger@example.com", models.Role_ROLE_USER)
	_, err := communitySvc.RejoinCommunity(strangerCtx, connect.NewRequest(&api.RejoinCommunityRequest{
		CommunityId: communityID,
	}))
	assertConnectCode(t, err, connect.CodeFailedPrecondition, "no membership")
	if err != nil && !contains(err.Error(), "no_membership_to_rejoin") {
		t.Errorf("expected reason 'no_membership_to_rejoin' in error, got: %v", err)
	}
}

// TestRejoinCommunity_BoundaryExactly30Days verifies the inclusive
// boundary at the window edge: a soft-delete that's exactly
// RejoinWindowSeconds in the past still qualifies for rejoin.
func TestRejoinCommunity_BoundaryExactly30Days(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	rejoinerID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia Owner")
	insertUser(t, db, rejoinerID, "rejoiner@example.com", "Riley Rejoiner")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "C", CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: ownerID, InviterId: ownerID,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Pin the clock so the boundary check is deterministic: the service sees
	// now == fixedNow, and deletedAt == fixedNow - RejoinWindowSeconds, so
	// elapsed == RejoinWindowSeconds which is NOT > RejoinWindowSeconds.
	fixedNow := time.Now()
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: rejoinerID, InviterId: ownerID,
		Deleted: &models.DeletedMetadata{
			DeletedByUserId:  rejoinerID,
			DeletedAtUnixSec: fixedNow.Unix() - communitylib.RejoinWindowSeconds,
		},
	}); err != nil {
		t.Fatalf("seed boundary: %v", err)
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	communitySvc, _ := newTestCommunityService(t, db, bucket, notifications.NewMockService())

	rejoinerCtx := clock.WithSimulationTime(
		makeAuthContext(rejoinerID, "rejoiner@example.com", models.Role_ROLE_USER),
		fixedNow,
	)
	if _, err := communitySvc.RejoinCommunity(rejoinerCtx, connect.NewRequest(&api.RejoinCommunityRequest{
		CommunityId: communityID,
	})); err != nil {
		t.Fatalf("rejoin at exact boundary should succeed: %v", err)
	}
}

// TestRejoinCommunity_ConcurrencyRace: N concurrent RejoinCommunity
// calls from the same caller. The race-safe conditional UPDATE
// guarantees exactly one returns success; the rest report
// already_member (or rejoin_window_expired in the worst-case
// interleaving where the probe reads after the row is already
// re-soft-deleted, which can't happen here because nothing
// re-soft-deletes during the test). The DB ends with exactly one
// active CommunityUser row and exactly one
// MEMBER_REJOINED_WITHIN_WINDOW event.
func TestRejoinCommunity_ConcurrencyRace(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	rejoinerID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia Owner")
	insertUser(t, db, rejoinerID, "rejoiner@example.com", "Riley Rejoiner")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "Race Community",
		CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: ownerID, InviterId: ownerID,
	}); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: rejoinerID, InviterId: ownerID,
		Deleted: &models.DeletedMetadata{DeletedByUserId: rejoinerID, DeletedAtUnixSec: time.Now().Unix() - 100},
	}); err != nil {
		t.Fatalf("seed rejoiner: %v", err)
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	communitySvc, _ := newTestCommunityService(t, db, bucket, notifications.NewMockService())

	rejoinerCtx := makeAuthContext(rejoinerID, "rejoiner@example.com", models.Role_ROLE_USER)

	const goroutines = 4
	type result struct{ err error }
	results := make(chan result, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := communitySvc.RejoinCommunity(rejoinerCtx, connect.NewRequest(&api.RejoinCommunityRequest{
				CommunityId: communityID,
			}))
			results <- result{err: err}
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	for r := range results {
		if r.err == nil {
			successes++
			continue
		}
		connectErr, ok := r.err.(*connect.Error)
		if !ok {
			t.Errorf("losing call: expected connect.Error, got %T (%v)", r.err, r.err)
			continue
		}
		if connectErr.Code() != connect.CodeFailedPrecondition {
			t.Errorf("losing call: code=%v, want FailedPrecondition (err=%v)", connectErr.Code(), r.err)
		}
		if !contains(r.err.Error(), "already_member") {
			t.Errorf("losing call: expected 'already_member' reason, got: %v", r.err)
		}
	}
	if successes != 1 {
		t.Errorf("expected exactly 1 success across %d concurrent rejoins, got %d", goroutines, successes)
	}

	// DB invariant: exactly one active CommunityUser row for the rejoiner.
	rows, err := db.QueryByField(ctx, "community_id", communityID, &models.CommunityUser{}, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("query memberships: %v", err)
	}
	rejoinerActive := 0
	rejoinerSoftDeleted := 0
	for _, m := range rows {
		cu := m.(*models.CommunityUser)
		if cu.UserId != rejoinerID {
			continue
		}
		if cu.Deleted == nil || cu.Deleted.DeletedAtUnixSec == 0 {
			rejoinerActive++
		} else {
			rejoinerSoftDeleted++
		}
	}
	if rejoinerActive != 1 {
		t.Errorf("expected exactly 1 active CommunityUser row for rejoiner, got %d (soft-deleted: %d)",
			rejoinerActive, rejoinerSoftDeleted)
	}
	if rejoinerSoftDeleted != 0 {
		t.Errorf("expected 0 soft-deleted rows for rejoiner, got %d", rejoinerSoftDeleted)
	}

	// Event invariant: exactly one MEMBER_REJOINED_WITHIN_WINDOW event.
	events, _ := db.QueryByField(ctx, "community_id", communityID, &models.CommunityEvent{})
	rejoinEventCount := 0
	for _, e := range events {
		ev := e.(*models.CommunityEvent)
		if ev.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_REJOINED_WITHIN_WINDOW {
			rejoinEventCount++
		}
	}
	if rejoinEventCount != 1 {
		t.Errorf("expected exactly 1 MEMBER_REJOINED_WITHIN_WINDOW event, got %d", rejoinEventCount)
	}
}

// --- helpers ---.

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
