package integration_tests

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// TestListDeletedCommunitiesForRestore_FullLifecycle exercises
// the §1722 RPC end-to-end against the real DeleteCommunity →
// list → RestoreCommunity → re-delete → list cycle, proving the
// snapshot-column lifecycle stays in sync across all the writes.
func TestListDeletedCommunitiesForRestore_FullLifecycle(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	memberID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia Owner")
	insertUser(t, db, memberID, "member@example.com", "Mary Member")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "Lifecycle Community",
		CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed community: %v", err)
	}
	for _, uid := range []string{ownerID, memberID} {
		if _, err := db.Insert(ctx, &models.CommunityUser{
			Id: uuid.New().String(), CommunityId: communityID, UserId: uid, InviterId: ownerID,
		}); err != nil {
			t.Fatalf("seed membership for %s: %v", uid, err)
		}
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	mockNotifs := notifications.NewMockService()
	communitySvc, bus := newTestCommunityService(t, db, bucket, mockNotifs)
	_ = bus // drained inline below

	ownerCtx := makeAuthContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	memberCtx := makeAuthContext(memberID, "member@example.com", models.Role_ROLE_USER)

	// === Phase A: pre-delete, both users see empty restore lists. ===
	for _, c := range []struct {
		label string
		ctx   context.Context
	}{{"owner", ownerCtx}, {"member", memberCtx}} {
		resp, err := communitySvc.ListDeletedCommunitiesForRestore(c.ctx,
			connect.NewRequest(&api.ListDeletedCommunitiesForRestoreRequest{}))
		if err != nil {
			t.Fatalf("phase A %s: %v", c.label, err)
		}
		if len(resp.Msg.Communities) != 0 {
			t.Errorf("phase A %s: expected empty list pre-delete, got %d", c.label, len(resp.Msg.Communities))
		}
	}

	// === Phase B: owner deletes; both members see the community. ===
	if _, err := communitySvc.DeleteCommunity(ownerCtx,
		connect.NewRequest(&api.DeleteCommunityRequest{Id: communityID})); err != nil {
		t.Fatalf("DeleteCommunity: %v", err)
	}
	drainCommunityEventBus(t, bus)

	for _, c := range []struct {
		label string
		ctx   context.Context
	}{{"owner", ownerCtx}, {"member", memberCtx}} {
		resp, err := communitySvc.ListDeletedCommunitiesForRestore(c.ctx,
			connect.NewRequest(&api.ListDeletedCommunitiesForRestoreRequest{}))
		if err != nil {
			t.Fatalf("phase B %s: %v", c.label, err)
		}
		if len(resp.Msg.Communities) != 1 {
			t.Fatalf("phase B %s: expected 1 result, got %d", c.label, len(resp.Msg.Communities))
		}
		got := resp.Msg.Communities[0]
		if got.Id != communityID {
			t.Errorf("phase B %s: id = %q, want %q", c.label, got.Id, communityID)
		}
		if got.DeletedAtUnixSec == 0 {
			t.Errorf("phase B %s: DeletedAtUnixSec should be populated", c.label)
		}
		if got.DeletedByUserId != ownerID {
			t.Errorf("phase B %s: DeletedByUserId = %q, want %q", c.label, got.DeletedByUserId, ownerID)
		}
	}

	// === Phase C: member restores; both lists go empty (snapshot column cleared by ClaimCommunityRestore). ===
	if _, err := communitySvc.RestoreCommunity(memberCtx,
		connect.NewRequest(&api.RestoreCommunityRequest{CommunityId: communityID})); err != nil {
		t.Fatalf("RestoreCommunity: %v", err)
	}
	drainCommunityEventBus(t, bus)

	for _, c := range []struct {
		label string
		ctx   context.Context
	}{{"owner", ownerCtx}, {"member", memberCtx}} {
		resp, err := communitySvc.ListDeletedCommunitiesForRestore(c.ctx,
			connect.NewRequest(&api.ListDeletedCommunitiesForRestoreRequest{}))
		if err != nil {
			t.Fatalf("phase C %s: %v", c.label, err)
		}
		if len(resp.Msg.Communities) != 0 {
			t.Errorf("phase C %s: expected empty list post-restore, got %d", c.label, len(resp.Msg.Communities))
		}
	}

	// === Phase D: re-delete; the cycle works (snapshot is re-populated). ===
	// After restore, member is the new owner.
	if _, err := communitySvc.DeleteCommunity(memberCtx,
		connect.NewRequest(&api.DeleteCommunityRequest{Id: communityID})); err != nil {
		t.Fatalf("re-DeleteCommunity: %v", err)
	}
	drainCommunityEventBus(t, bus)

	resp, err := communitySvc.ListDeletedCommunitiesForRestore(memberCtx,
		connect.NewRequest(&api.ListDeletedCommunitiesForRestoreRequest{}))
	if err != nil {
		t.Fatalf("phase D member: %v", err)
	}
	if len(resp.Msg.Communities) != 1 {
		t.Errorf("phase D member: expected 1 result post-redelete, got %d", len(resp.Msg.Communities))
	}
}
