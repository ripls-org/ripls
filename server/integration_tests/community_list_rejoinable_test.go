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

// TestListRejoinableCommunities_FullLifecycle exercises the
// §1723 RPC end-to-end against the real LeaveCommunity → list →
// DeleteCommunity → list-empty (§2.7) → RestoreCommunity → list →
// RejoinCommunity → list-empty cycle. Proves the §2.7 cross-cut
// (community-deleted blocks rejoin even within the per-user
// window) holds end-to-end.
func TestListRejoinableCommunities_FullLifecycle(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	leaverID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia Owner")
	insertUser(t, db, leaverID, "leaver@example.com", "Larry Leaver")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "Lifecycle Community",
		CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed community: %v", err)
	}
	for _, uid := range []string{ownerID, leaverID} {
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
	leaverCtx := makeAuthContext(leaverID, "leaver@example.com", models.Role_ROLE_USER)

	listLeaver := func(t *testing.T, label string) []*api.RejoinableCommunityItem {
		t.Helper()
		resp, err := communitySvc.ListRejoinableCommunities(leaverCtx,
			connect.NewRequest(&api.ListRejoinableCommunitiesRequest{}))
		if err != nil {
			t.Fatalf("%s: ListRejoinableCommunities: %v", label, err)
		}
		return resp.Msg.Communities
	}

	// === Phase A: pre-leave, list is empty (caller is an active member). ===
	if got := listLeaver(t, "phase A"); len(got) != 0 {
		t.Errorf("phase A: expected empty list pre-leave, got %d", len(got))
	}

	// === Phase B: leaver leaves; list shows the community. ===
	if _, err := communitySvc.LeaveCommunity(leaverCtx,
		connect.NewRequest(&api.LeaveCommunityRequest{CommunityId: communityID})); err != nil {
		t.Fatalf("LeaveCommunity: %v", err)
	}
	got := listLeaver(t, "phase B")
	if len(got) != 1 {
		t.Fatalf("phase B: expected 1 result post-leave, got %d", len(got))
	}
	if got[0].Id != communityID {
		t.Errorf("phase B: id = %q, want %q", got[0].Id, communityID)
	}
	if got[0].LeftAtUnixSec == 0 {
		t.Error("phase B: LeftAtUnixSec should be populated")
	}

	// === Phase C: owner deletes the community; §2.7 hides it from the rejoin list. ===
	if _, err := communitySvc.DeleteCommunity(ownerCtx,
		connect.NewRequest(&api.DeleteCommunityRequest{Id: communityID})); err != nil {
		t.Fatalf("DeleteCommunity: %v", err)
	}
	drainCommunityEventBus(t, bus)
	if got := listLeaver(t, "phase C"); len(got) != 0 {
		t.Errorf("phase C: expected empty list (§2.7 — community deleted), got %d", len(got))
	}

	// === Phase D: owner restores; the community reappears in the rejoin list. ===
	if _, err := communitySvc.RestoreCommunity(ownerCtx,
		connect.NewRequest(&api.RestoreCommunityRequest{CommunityId: communityID})); err != nil {
		t.Fatalf("RestoreCommunity: %v", err)
	}
	drainCommunityEventBus(t, bus)
	got = listLeaver(t, "phase D")
	if len(got) != 1 {
		t.Errorf("phase D: expected 1 result after restore, got %d", len(got))
	}

	// === Phase E: leaver rejoins; list goes empty (membership active). ===
	if _, err := communitySvc.RejoinCommunity(leaverCtx,
		connect.NewRequest(&api.RejoinCommunityRequest{CommunityId: communityID})); err != nil {
		t.Fatalf("RejoinCommunity: %v", err)
	}
	if got := listLeaver(t, "phase E"); len(got) != 0 {
		t.Errorf("phase E: expected empty list after rejoin, got %d", len(got))
	}
}
