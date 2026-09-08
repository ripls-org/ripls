package integration_tests

import (
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// TestLeaveCommunity_NonOwnerHappyPath is the end-to-end proof for
// the non-owner branch of #1656 Phase 2: a non-owner member leaves
// an active community, the §4 cascade soft-deletes their gear/
// request/notification-prefs/invitation-link rows and cancels their
// active transfer + RSVP, the leaver's CommunityUser row is
// soft-deleted (preserving the 30-day rejoin window), and a
// MEMBER_LEFT event fires.
func TestLeaveCommunity_NonOwnerHappyPath(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	leaverID := uuid.New().String()
	bystanderID := uuid.New().String()

	insertUser(t, db, ownerID, "owner@example.com", "Olivia Owner")
	insertUser(t, db, leaverID, "leaver@example.com", "Larry Leaver")
	insertUser(t, db, bystanderID, "bystander@example.com", "Bea Bystander")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id:          communityID,
		Name:        "Test Community",
		CreatorId:   ownerID,
		OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed community: %v", err)
	}
	for _, uid := range []string{ownerID, leaverID, bystanderID} {
		if _, err := db.Insert(ctx, &models.CommunityUser{
			Id: uuid.New().String(), CommunityId: communityID, UserId: uid, InviterId: ownerID,
		}); err != nil {
			t.Fatalf("seed membership for %s: %v", uid, err)
		}
	}

	// Seed leaver's cascade rows. Each has a peer that should NOT
	// be touched (off-scope by user, off-scope by community, or
	// already-completed/cancelled state).
	leaverGearID, _ := db.Insert(ctx, &models.Gear{Id: uuid.New().String(), Name: "Leaver Tent", OwnerId: leaverID, State: models.GearState_GEAR_STATE_AVAILABLE})
	bystanderGearID, _ := db.Insert(ctx, &models.Gear{Id: uuid.New().String(), Name: "Bystander Tent", OwnerId: bystanderID, State: models.GearState_GEAR_STATE_AVAILABLE})
	leaverCG, _ := db.Insert(ctx, &models.CommunityGear{Id: uuid.New().String(), CommunityId: communityID, GearId: leaverGearID})
	bystanderCG, _ := db.Insert(ctx, &models.CommunityGear{Id: uuid.New().String(), CommunityId: communityID, GearId: bystanderGearID})

	leaverReqID, _ := db.Insert(ctx, &models.Request{Id: uuid.New().String(), Title: "Need a saw", RequesterId: leaverID})
	bystanderReqID, _ := db.Insert(ctx, &models.Request{Id: uuid.New().String(), Title: "Need a hammer", RequesterId: bystanderID})
	leaverCR, _ := db.Insert(ctx, &models.CommunityRequest{Id: uuid.New().String(), CommunityId: communityID, RequestId: leaverReqID})
	bystanderCR, _ := db.Insert(ctx, &models.CommunityRequest{Id: uuid.New().String(), CommunityId: communityID, RequestId: bystanderReqID})

	leaverActiveTx, _ := db.Insert(ctx, &models.Transfer{
		Id: uuid.New().String(), CommunityId: communityID,
		OwnerId: leaverID, RecipientId: bystanderID,
		State: models.TransferState_TRANSFER_STATE_ACTIVE,
	})
	bystanderTx, _ := db.Insert(ctx, &models.Transfer{
		Id: uuid.New().String(), CommunityId: communityID,
		OwnerId: ownerID, RecipientId: bystanderID,
		State: models.TransferState_TRANSFER_STATE_ACTIVE,
	})
	leaverCompletedTx, _ := db.Insert(ctx, &models.Transfer{
		Id: uuid.New().String(), CommunityId: communityID,
		OwnerId: leaverID, RecipientId: bystanderID,
		State: models.TransferState_TRANSFER_STATE_COMPLETED,
	})

	leaverRSVP, _ := db.Insert(ctx, &models.ExperienceRSVP{
		Id: uuid.New().String(), ExperienceId: uuid.New().String(),
		UserId: leaverID, CommunityId: communityID,
	})
	bystanderRSVP, _ := db.Insert(ctx, &models.ExperienceRSVP{
		Id: uuid.New().String(), ExperienceId: uuid.New().String(),
		UserId: bystanderID, CommunityId: communityID,
	})

	leaverPrefs, _ := db.Insert(ctx, &models.CommunityNotificationPreferences{
		Id: uuid.New().String(), CommunityId: communityID, UserId: leaverID,
	})
	bystanderPrefs, _ := db.Insert(ctx, &models.CommunityNotificationPreferences{
		Id: uuid.New().String(), CommunityId: communityID, UserId: bystanderID,
	})

	leaverLink, _ := db.Insert(ctx, &models.CommunityInvitationLink{
		Id: uuid.New().String(), CommunityId: communityID, InviterId: leaverID, ShortCode: "LVR1",
	})
	ownerLink, _ := db.Insert(ctx, &models.CommunityInvitationLink{
		Id: uuid.New().String(), CommunityId: communityID, InviterId: ownerID, ShortCode: "OWN1",
	})
	leaverShare, _ := db.Insert(ctx, &models.ShareLink{
		Id: uuid.New().String(), CommunityId: communityID, InviterId: leaverID, ShortCode: "LVR2",
		Target: &models.ShareLink_CommunityInviteId{CommunityInviteId: communityID},
	})
	ownerShare, _ := db.Insert(ctx, &models.ShareLink{
		Id: uuid.New().String(), CommunityId: communityID, InviterId: ownerID, ShortCode: "OWN2",
		Target: &models.ShareLink_CommunityInviteId{CommunityInviteId: communityID},
	})

	tempDir := t.TempDir()
	bucket, err := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	if err != nil {
		t.Fatalf("bucket: %v", err)
	}
	// MEMBER_LEFT is not in shouldNotify (no push fires), so no
	// notifDone signal to wait on; the call is fully synchronous.
	communitySvc, _ := newTestCommunityService(t, db, bucket, notifications.NewMockService())

	leaverCtx := makeAuthContext(leaverID, "leaver@example.com", models.Role_ROLE_USER)
	if _, err := communitySvc.LeaveCommunity(leaverCtx, connect.NewRequest(&api.LeaveCommunityRequest{
		CommunityId: communityID,
	})); err != nil {
		t.Fatalf("LeaveCommunity: %v", err)
	}

	// === Assert: leaver's CommunityUser row is soft-deleted. ===
	leaverMembership := findMembership(t, db, ctx, communityID, leaverID)
	if leaverMembership.GetDeleted() == nil || leaverMembership.GetDeleted().DeletedAtUnixSec == 0 {
		t.Error("leaver's CommunityUser should be soft-deleted")
	}
	if leaverMembership.GetDeleted().DeletedByUserId != leaverID {
		t.Errorf("DeletedByUserId = %q, want %q", leaverMembership.GetDeleted().DeletedByUserId, leaverID)
	}

	// === Assert: bystander/owner memberships untouched. ===
	if findMembership(t, db, ctx, communityID, bystanderID).GetDeleted() != nil {
		t.Error("bystander membership should remain active")
	}
	if findMembership(t, db, ctx, communityID, ownerID).GetDeleted() != nil {
		t.Error("owner membership should remain active")
	}

	// === Assert: leaver's join rows soft-deleted; bystander's untouched. ===
	assertSoftDeleted(t, db, ctx, leaverCG, &models.CommunityGear{}, "leaver CG")
	assertNotSoftDeleted(t, db, ctx, bystanderCG, &models.CommunityGear{}, "bystander CG")

	assertSoftDeleted(t, db, ctx, leaverCR, &models.CommunityRequest{}, "leaver CR")
	assertNotSoftDeleted(t, db, ctx, bystanderCR, &models.CommunityRequest{}, "bystander CR")

	assertSoftDeleted(t, db, ctx, leaverRSVP, &models.ExperienceRSVP{}, "leaver RSVP")
	assertNotSoftDeleted(t, db, ctx, bystanderRSVP, &models.ExperienceRSVP{}, "bystander RSVP")

	assertSoftDeleted(t, db, ctx, leaverPrefs, &models.CommunityNotificationPreferences{}, "leaver prefs")
	assertNotSoftDeleted(t, db, ctx, bystanderPrefs, &models.CommunityNotificationPreferences{}, "bystander prefs")

	assertSoftDeleted(t, db, ctx, leaverLink, &models.CommunityInvitationLink{}, "leaver link")
	assertNotSoftDeleted(t, db, ctx, ownerLink, &models.CommunityInvitationLink{}, "owner link")
	assertSoftDeleted(t, db, ctx, leaverShare, &models.ShareLink{}, "leaver share_link")
	assertNotSoftDeleted(t, db, ctx, ownerShare, &models.ShareLink{}, "owner share_link")

	// === Assert: leaver's active transfer cancelled; bystander/completed untouched. ===
	assertTransferState(t, db, ctx, leaverActiveTx, models.TransferState_TRANSFER_STATE_CANCELLED, "leaver active transfer")
	assertTransferState(t, db, ctx, bystanderTx, models.TransferState_TRANSFER_STATE_ACTIVE, "bystander transfer")
	assertTransferState(t, db, ctx, leaverCompletedTx, models.TransferState_TRANSFER_STATE_COMPLETED, "leaver completed transfer")

	// === Assert: gear/request rows themselves untouched (leaver keeps them). ===
	assertNotSoftDeleted(t, db, ctx, leaverGearID, &models.Gear{}, "leaver gear (parent)")
	assertNotSoftDeleted(t, db, ctx, leaverReqID, &models.Request{}, "leaver request (parent)")

	// === Assert: MEMBER_LEFT event was recorded with leaver as actor. ===
	events, _ := db.QueryByField(ctx, "community_id", communityID, &models.CommunityEvent{})
	var sawMemberLeft bool
	for _, e := range events {
		ev := e.(*models.CommunityEvent)
		if ev.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT && ev.ActorId == leaverID {
			sawMemberLeft = true
			break
		}
	}
	if !sawMemberLeft {
		t.Error("expected MEMBER_LEFT event with leaver as actor")
	}
}

// TestLeaveCommunity_NonOwnerEmptyCascade exercises the non-owner
// branch when the leaver has no gear, requests, transfers, etc. The
// cascade is a chain of noops; the leave still succeeds and the
// CommunityUser row is soft-deleted.
func TestLeaveCommunity_NonOwnerEmptyCascade(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	leaverID := uuid.New().String()

	insertUser(t, db, ownerID, "owner@example.com", "Olivia Owner")
	insertUser(t, db, leaverID, "leaver@example.com", "Larry Leaver")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "C", CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for _, uid := range []string{ownerID, leaverID} {
		if _, err := db.Insert(ctx, &models.CommunityUser{
			Id: uuid.New().String(), CommunityId: communityID, UserId: uid, InviterId: ownerID,
		}); err != nil {
			t.Fatalf("seed membership: %v", err)
		}
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	communitySvc, _ := newTestCommunityService(t, db, bucket, notifications.NewMockService())

	leaverCtx := makeAuthContext(leaverID, "leaver@example.com", models.Role_ROLE_USER)
	if _, err := communitySvc.LeaveCommunity(leaverCtx, connect.NewRequest(&api.LeaveCommunityRequest{
		CommunityId: communityID,
	})); err != nil {
		t.Fatalf("LeaveCommunity: %v", err)
	}

	leaverMembership := findMembership(t, db, ctx, communityID, leaverID)
	if leaverMembership.GetDeleted() == nil {
		t.Error("leaver membership should be soft-deleted after empty-cascade leave")
	}
}

// TestLeaveCommunity_NotMember rejects calls from users with no
// active membership. Either the user never joined or has already
// soft-deleted (in the rejoin window) — both surface as NotFound.
func TestLeaveCommunity_NotMember(t *testing.T) {
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
		t.Fatalf("seed membership: %v", err)
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	communitySvc, _ := newTestCommunityService(t, db, bucket, notifications.NewMockService())

	strangerCtx := makeAuthContext(strangerID, "stranger@example.com", models.Role_ROLE_USER)
	_, err := communitySvc.LeaveCommunity(strangerCtx, connect.NewRequest(&api.LeaveCommunityRequest{
		CommunityId: communityID,
	}))
	if err == nil {
		t.Fatal("expected NotFound for non-member")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connectErr.Code())
	}
}

// TestLeaveCommunity_DeletedCommunity rejects calls against an
// already soft-deleted community with FailedPrecondition. Leaving a
// deleted community is meaningless — the user sees nothing of it
// once #1654's read filter ships.
func TestLeaveCommunity_DeletedCommunity(t *testing.T) {
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
		Deleted: &models.DeletedMetadata{DeletedByUserId: ownerID, DeletedAtUnixSec: time.Now().Unix()},
		DeletedSnapshot: &models.CommunityDeletedSnapshot{
			MemberUserIds: []string{ownerID, memberID},
		},
	}); err != nil {
		t.Fatalf("seed deleted community: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: memberID, InviterId: ownerID,
	}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	communitySvc, _ := newTestCommunityService(t, db, bucket, notifications.NewMockService())

	memberCtx := makeAuthContext(memberID, "member@example.com", models.Role_ROLE_USER)
	_, err := communitySvc.LeaveCommunity(memberCtx, connect.NewRequest(&api.LeaveCommunityRequest{
		CommunityId: communityID,
	}))
	if err == nil {
		t.Fatal("expected FailedPrecondition for deleted community")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeFailedPrecondition {
		t.Errorf("expected CodeFailedPrecondition, got %v", connectErr.Code())
	}
}

// TestLeaveCommunity_OwnerHandoffHappyPath is the end-to-end proof
// for Phase 3 of #1656: owner picks a candidate, ownership transfers
// atomically, the owner's CommunityUser row soft-deletes, the
// cascade runs, OWNERSHIP_TRANSFERRED + MEMBER_LEFT events fire in
// that order, and the new owner gets the §8 push notification.
func TestLeaveCommunity_OwnerHandoffHappyPath(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	oldOwnerID := uuid.New().String()
	newOwnerID := uuid.New().String()
	bystanderID := uuid.New().String()
	insertUser(t, db, oldOwnerID, "old@example.com", "Olivia Old")
	insertUser(t, db, newOwnerID, "new@example.com", "Nina New")
	insertUser(t, db, bystanderID, "bystander@example.com", "Bea Bystander")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "Test Community",
		CreatorId: oldOwnerID, OwnerUserId: oldOwnerID,
	}); err != nil {
		t.Fatalf("seed community: %v", err)
	}
	for _, uid := range []string{oldOwnerID, newOwnerID, bystanderID} {
		if _, err := db.Insert(ctx, &models.CommunityUser{
			Id: uuid.New().String(), CommunityId: communityID, UserId: uid, InviterId: oldOwnerID,
		}); err != nil {
			t.Fatalf("seed membership for %s: %v", uid, err)
		}
	}

	// Seed one of each cascade row owned by the leaver to confirm
	// the cascade still runs in the owner-leave branch.
	leaverGearID, _ := db.Insert(ctx, &models.Gear{Id: uuid.New().String(), Name: "Leaver Tent", OwnerId: oldOwnerID, State: models.GearState_GEAR_STATE_AVAILABLE})
	leaverCG, _ := db.Insert(ctx, &models.CommunityGear{Id: uuid.New().String(), CommunityId: communityID, GearId: leaverGearID})

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	mockNotifs := notifications.NewMockService()
	communitySvc, bus := newTestCommunityService(t, db, bucket, mockNotifs)
	_ = bus // drained inline below

	oldOwnerCtx := makeAuthContext(oldOwnerID, "old@example.com", models.Role_ROLE_USER)
	if _, err := communitySvc.LeaveCommunity(oldOwnerCtx, connect.NewRequest(&api.LeaveCommunityRequest{
		CommunityId:    communityID,
		NewOwnerUserId: stringPtr(newOwnerID),
	})); err != nil {
		t.Fatalf("LeaveCommunity (owner-handoff): %v", err)
	}
	// OWNERSHIP_TRANSFERRED is in shouldNotify, so the dispatcher
	// signals back. MEMBER_LEFT is not in shouldNotify and does not
	// signal.
	drainCommunityEventBus(t, bus)

	// === Assert: owner_user_id was transferred (both flat and proto). ===
	got := &models.Community{}
	if err := db.GetByID(ctx, communityID, got); err != nil {
		t.Fatalf("re-fetch community: %v", err)
	}
	if got.OwnerUserId != newOwnerID {
		t.Errorf("OwnerUserId = %q, want %q", got.OwnerUserId, newOwnerID)
	}

	// === Assert: previous owner's membership is soft-deleted. ===
	leaverMembership := findMembership(t, db, ctx, communityID, oldOwnerID)
	if leaverMembership.GetDeleted() == nil {
		t.Error("previous owner's membership should be soft-deleted")
	}

	// === Assert: cascade ran. ===
	assertSoftDeleted(t, db, ctx, leaverCG, &models.CommunityGear{}, "leaver CG")

	// === Assert: events were recorded with the right shape. ===
	events, _ := db.QueryByField(ctx, "community_id", communityID, &models.CommunityEvent{})
	var sawTransferred, sawLeft bool
	for _, e := range events {
		ev := e.(*models.CommunityEvent)
		switch ev.EventType {
		case models.CommunityEventType_COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED:
			if ev.ActorId == oldOwnerID && ev.ObjectUserId == newOwnerID {
				sawTransferred = true
			}
		case models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT:
			if ev.ActorId == oldOwnerID {
				sawLeft = true
			}
		}
	}
	if !sawTransferred {
		t.Error("expected OWNERSHIP_TRANSFERRED event with old owner as actor and new owner as object")
	}
	if !sawLeft {
		t.Error("expected MEMBER_LEFT event with old owner as actor")
	}

	// === Assert: only the new owner got the OWNERSHIP_TRANSFERRED push. ===
	calls := mockNotifs.GetCalls()
	var sawNewOwnerPush bool
	for _, c := range calls {
		if c.UserID == newOwnerID && c.Notification != nil {
			sawNewOwnerPush = true
		}
		if c.UserID == oldOwnerID {
			t.Errorf("old owner should not receive a push for their own ownership transfer")
		}
		if c.UserID == bystanderID {
			t.Errorf("bystander should not receive a push for ownership transfer (new owner only per §8)")
		}
	}
	if !sawNewOwnerPush {
		t.Error("expected the new owner to receive an OWNERSHIP_TRANSFERRED push")
	}
}

// TestLeaveCommunity_OwnerMissingNewOwner rejects an owner-leave call
// without new_owner_user_id.
func TestLeaveCommunity_OwnerMissingNewOwner(t *testing.T) {
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

	ownerCtx := makeAuthContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	_, err := communitySvc.LeaveCommunity(ownerCtx, connect.NewRequest(&api.LeaveCommunityRequest{
		CommunityId: communityID,
	}))
	assertConnectCode(t, err, connect.CodeInvalidArgument, "missing new_owner_user_id")
}

// TestLeaveCommunity_OwnerCandidateIsCaller rejects new_owner_user_id
// pointing at the caller themselves.
func TestLeaveCommunity_OwnerCandidateIsCaller(t *testing.T) {
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

	ownerCtx := makeAuthContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	_, err := communitySvc.LeaveCommunity(ownerCtx, connect.NewRequest(&api.LeaveCommunityRequest{
		CommunityId:    communityID,
		NewOwnerUserId: stringPtr(ownerID),
	}))
	assertConnectCode(t, err, connect.CodeInvalidArgument, "candidate==caller")
}

// TestLeaveCommunity_OwnerCandidateNotMember returns FailedPrecondition
// when new_owner_user_id is not an active member.
func TestLeaveCommunity_OwnerCandidateNotMember(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	memberID := uuid.New().String()
	strangerID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia Owner")
	insertUser(t, db, memberID, "member@example.com", "Mary Member")
	insertUser(t, db, strangerID, "stranger@example.com", "Sam Stranger")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "C", CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Only owner and member; stranger is NOT a member.
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

	ownerCtx := makeAuthContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	_, err := communitySvc.LeaveCommunity(ownerCtx, connect.NewRequest(&api.LeaveCommunityRequest{
		CommunityId:    communityID,
		NewOwnerUserId: stringPtr(strangerID),
	}))
	assertConnectCode(t, err, connect.CodeFailedPrecondition, "candidate not a member")
}

// TestLeaveCommunity_OwnerCandidateSoftDeletedMembership treats a
// soft-deleted candidate (in the rejoin window) as not-a-member —
// they have to rejoin before they can receive ownership.
func TestLeaveCommunity_OwnerCandidateSoftDeletedMembership(t *testing.T) {
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
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: ownerID, InviterId: ownerID,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Member's row is soft-deleted (mid-rejoin-window).
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: memberID, InviterId: ownerID,
		Deleted: &models.DeletedMetadata{DeletedByUserId: memberID, DeletedAtUnixSec: time.Now().Unix() - 100},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	communitySvc, _ := newTestCommunityService(t, db, bucket, notifications.NewMockService())

	ownerCtx := makeAuthContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	// Note: the active-member count is 1 (just the owner), so this
	// would route to leaveAsSoleMember (Phase 4) before owner-leave.
	// Skip if that turns out to be the case — the soft-deleted-
	// candidate path is genuinely owner-leave only when there's
	// ALSO at least one other active member. Re-seed an extra active
	// member to make this test exercise the candidate-soft-deleted
	// branch specifically.
	otherActive := uuid.New().String()
	insertUser(t, db, otherActive, "extra@example.com", "Eddie Extra")
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: otherActive, InviterId: ownerID,
	}); err != nil {
		t.Fatalf("seed extra: %v", err)
	}

	_, err := communitySvc.LeaveCommunity(ownerCtx, connect.NewRequest(&api.LeaveCommunityRequest{
		CommunityId:    communityID,
		NewOwnerUserId: stringPtr(memberID),
	}))
	assertConnectCode(t, err, connect.CodeFailedPrecondition, "candidate has soft-deleted membership")
}

// TestLeaveCommunity_OwnerHandoffRace exercises two concurrent
// owner-leave calls from the same owner, each picking a different
// candidate. The conditional-UPDATE atomicity guarantees exactly one
// succeeds and the other returns FailedPrecondition; the community
// ends up with exactly one valid owner (one of the two candidates).
// See design doc §9.1 ("Owner-leave race test") and §6.1 ("never
// zero owners, never two owners").
func TestLeaveCommunity_OwnerHandoffRace(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	candAID := uuid.New().String()
	candBID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia Owner")
	insertUser(t, db, candAID, "a@example.com", "Alice")
	insertUser(t, db, candBID, "b@example.com", "Bob")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "C", CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for _, uid := range []string{ownerID, candAID, candBID} {
		if _, err := db.Insert(ctx, &models.CommunityUser{
			Id: uuid.New().String(), CommunityId: communityID, UserId: uid, InviterId: ownerID,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	communitySvc, _ := newTestCommunityService(t, db, bucket, notifications.NewMockService())

	ownerCtx := makeAuthContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	type result struct {
		candidate string
		err       error
	}
	var wg sync.WaitGroup
	results := make(chan result, 2)
	wg.Add(2)
	for _, cand := range []string{candAID, candBID} {
		go func(c string) {
			defer wg.Done()
			_, err := communitySvc.LeaveCommunity(ownerCtx, connect.NewRequest(&api.LeaveCommunityRequest{
				CommunityId:    communityID,
				NewOwnerUserId: stringPtr(c),
			}))
			results <- result{candidate: c, err: err}
		}(cand)
	}
	wg.Wait()
	close(results)

	// Acceptable outcomes per the §6.1 invariant ("never zero owners,
	// never two owners") and the race-resolution shape of this code:
	//
	// - One goroutine wins the conditional UPDATE and runs the full
	//   handoff path. The other reads the community AFTER the
	//   resync, sees a different owner, and either falls through to
	//   the non-owner-leave path (if it read its own membership
	//   before the soft-delete landed) or returns NotFound (if it
	//   read after).
	// - Both calls may therefore return success, but the database
	//   ends with exactly one valid owner — one of the two
	//   candidates — and exactly one OWNERSHIP_TRANSFERRED event
	//   was recorded.
	//
	// The losing call may also return FailedPrecondition
	// (ownership_changed) or NotFound depending on the precise
	// interleaving. All of these are coherent terminal states; the
	// invariants are owner-count and event-count.
	for r := range results {
		t.Logf("race result: candidate=%s err=%v", r.candidate, r.err)
		if r.err == nil {
			continue
		}
		connectErr, ok := r.err.(*connect.Error)
		if !ok {
			t.Errorf("losing call: expected connect.Error, got %T (%v)", r.err, r.err)
			continue
		}
		switch connectErr.Code() {
		case connect.CodeFailedPrecondition, connect.CodeNotFound:
			// Acceptable terminal states for the loser.
		default:
			t.Errorf("losing call: unexpected code %v (err=%v)", connectErr.Code(), r.err)
		}
	}

	// Owner invariant: exactly one valid owner; must be one of the two candidates.
	got := &models.Community{}
	if err := db.GetByID(ctx, communityID, got); err != nil {
		t.Fatalf("re-fetch: %v", err)
	}
	if got.OwnerUserId != candAID && got.OwnerUserId != candBID {
		t.Errorf("OwnerUserId = %q, want one of {%q, %q}", got.OwnerUserId, candAID, candBID)
	}
	if got.OwnerUserId == "" {
		t.Error("community has no owner (zero-owner state)")
	}

	// Event invariant: exactly one OWNERSHIP_TRANSFERRED event was recorded.
	events, _ := db.QueryByField(ctx, "community_id", communityID, &models.CommunityEvent{})
	transferCount := 0
	for _, e := range events {
		ev := e.(*models.CommunityEvent)
		if ev.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED {
			transferCount++
			if ev.ObjectUserId != got.OwnerUserId {
				t.Errorf("OWNERSHIP_TRANSFERRED.object_user_id = %q, want current owner %q",
					ev.ObjectUserId, got.OwnerUserId)
			}
		}
	}
	if transferCount != 1 {
		t.Errorf("expected exactly one OWNERSHIP_TRANSFERRED event, got %d", transferCount)
	}
}

// TestLeaveCommunity_SoleMember exercises the §2.4 conversion: when
// the sole active member leaves, the call short-circuits to a
// soft-delete with the leaver as the actor. The leaver appears in
// deleted_snapshot.member_user_ids so they can later restore from
// Settings → Communities. The cascade soft-deletes the community-
// level join rows (gear/request/etc), and the audit event is
// COMMUNITY_DELETED, not MEMBER_LEFT.
func TestLeaveCommunity_SoleMember(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	loneID := uuid.New().String()
	insertUser(t, db, loneID, "lone@example.com", "Lone Loner")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "Solo Community",
		CreatorId: loneID, OwnerUserId: loneID,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: loneID, InviterId: loneID,
	}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	// Seed one community-level join row so we can confirm the
	// community-delete cascade ran (per #1654 path).
	gearID, _ := db.Insert(ctx, &models.Gear{Id: uuid.New().String(), Name: "Tent", OwnerId: loneID, State: models.GearState_GEAR_STATE_AVAILABLE})
	cgID, _ := db.Insert(ctx, &models.CommunityGear{Id: uuid.New().String(), CommunityId: communityID, GearId: gearID})

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	mockNotifs := notifications.NewMockService()
	communitySvc, bus := newTestCommunityService(t, db, bucket, mockNotifs)
	_ = bus // drained inline below

	loneCtx := makeAuthContext(loneID, "lone@example.com", models.Role_ROLE_USER)
	if _, err := communitySvc.LeaveCommunity(loneCtx, connect.NewRequest(&api.LeaveCommunityRequest{
		CommunityId: communityID,
	})); err != nil {
		t.Fatalf("sole-member LeaveCommunity: %v", err)
	}
	// COMMUNITY_DELETED is in shouldNotify and fires through
	// firesForDeletedCommunity even after soft-delete — drain.
	drainCommunityEventBus(t, bus)

	// Community is soft-deleted with leaver as actor and in snapshot.
	got := &models.Community{}
	if err := db.GetByID(ctx, communityID, got, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("re-fetch: %v", err)
	}
	if got.Deleted == nil || got.Deleted.DeletedAtUnixSec == 0 {
		t.Error("community should be soft-deleted")
	}
	if got.Deleted != nil && got.Deleted.DeletedByUserId != loneID {
		t.Errorf("DeletedByUserId = %q, want %q", got.Deleted.DeletedByUserId, loneID)
	}
	if got.DeletedSnapshot == nil || len(got.DeletedSnapshot.MemberUserIds) != 1 || got.DeletedSnapshot.MemberUserIds[0] != loneID {
		t.Errorf("DeletedSnapshot.MemberUserIds = %v, want [%q]",
			got.DeletedSnapshot.GetMemberUserIds(), loneID)
	}

	// Cascade ran: CommunityGear is soft-deleted.
	assertSoftDeleted(t, db, ctx, cgID, &models.CommunityGear{}, "cg")

	// Audit event is COMMUNITY_DELETED, not MEMBER_LEFT.
	events, _ := db.QueryByField(ctx, "community_id", communityID, &models.CommunityEvent{})
	var sawDeleted, sawMemberLeft bool
	for _, e := range events {
		ev := e.(*models.CommunityEvent)
		switch ev.EventType {
		case models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED:
			if ev.ActorId == loneID {
				sawDeleted = true
			}
		case models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT:
			sawMemberLeft = true
		}
	}
	if !sawDeleted {
		t.Error("expected COMMUNITY_DELETED event with leaver as actor")
	}
	if sawMemberLeft {
		t.Error("did not expect MEMBER_LEFT event for sole-member-leave (§6.4 says COMMUNITY_DELETED is the audit event)")
	}
}

// TestLeaveCommunity_SoleMemberLeaveThenRestore exercises the
// round-trip with #1655: sole-member leaves (community soft-deletes)
// and then restores from Settings → Communities, recovering full
// access as the new owner.
func TestLeaveCommunity_SoleMemberLeaveThenRestore(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	loneID := uuid.New().String()
	insertUser(t, db, loneID, "lone@example.com", "Lone Loner")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "Round-trip Community",
		CreatorId: loneID, OwnerUserId: loneID,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: loneID, InviterId: loneID,
	}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	gearID, _ := db.Insert(ctx, &models.Gear{Id: uuid.New().String(), Name: "Tent", OwnerId: loneID, State: models.GearState_GEAR_STATE_AVAILABLE})
	cgID, _ := db.Insert(ctx, &models.CommunityGear{Id: uuid.New().String(), CommunityId: communityID, GearId: gearID})

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	mockNotifs := notifications.NewMockService()
	communitySvc, bus := newTestCommunityService(t, db, bucket, mockNotifs)
	_ = bus // drained inline below
	loneCtx := makeAuthContext(loneID, "lone@example.com", models.Role_ROLE_USER)

	// Step 1: sole-member leaves → community soft-deletes.
	if _, err := communitySvc.LeaveCommunity(loneCtx, connect.NewRequest(&api.LeaveCommunityRequest{
		CommunityId: communityID,
	})); err != nil {
		t.Fatalf("sole-member leave: %v", err)
	}
	drainCommunityEventBus(t, bus)
	mockNotifs.Reset()

	// Step 2: same user restores via #1655 path.
	if _, err := communitySvc.RestoreCommunity(loneCtx, connect.NewRequest(&api.RestoreCommunityRequest{
		CommunityId: communityID,
	})); err != nil {
		t.Fatalf("RestoreCommunity: %v", err)
	}
	drainCommunityEventBus(t, bus)

	// Community is active again; cascade rows are reactivated.
	got := &models.Community{}
	if err := db.GetByID(ctx, communityID, got); err != nil {
		t.Fatalf("re-fetch after restore: %v", err)
	}
	if got.Deleted != nil {
		t.Errorf("Community.Deleted not cleared: %+v", got.Deleted)
	}
	if got.DeletedSnapshot != nil {
		t.Errorf("Community.DeletedSnapshot not cleared: %+v", got.DeletedSnapshot)
	}
	if got.OwnerUserId != loneID {
		t.Errorf("OwnerUserId = %q, want %q (restorer)", got.OwnerUserId, loneID)
	}
	assertNotSoftDeleted(t, db, ctx, cgID, &models.CommunityGear{}, "cg after restore")

	// Membership is active.
	leaverMembership := findMembership(t, db, ctx, communityID, loneID)
	if leaverMembership.GetDeleted() != nil {
		t.Error("loner's membership should be active after restore")
	}
}

// --- helpers (continued) ---.

func assertConnectCode(t *testing.T, err error, want connect.Code, label string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error %v, got nil", label, want)
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("%s: expected connect.Error, got %T (%v)", label, err, err)
	}
	if connectErr.Code() != want {
		t.Errorf("%s: code=%v, want %v (err=%v)", label, connectErr.Code(), want, err)
	}
}

// --- helpers ---.

func findMembership(t *testing.T, db *storage.ProtoSQLStorage, ctx context.Context, communityID, userID string) *models.CommunityUser {
	t.Helper()
	rows, err := db.QueryByField(ctx, "community_id", communityID, &models.CommunityUser{}, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("query memberships: %v", err)
	}
	for _, m := range rows {
		cu := m.(*models.CommunityUser)
		if cu.UserId == userID {
			return cu
		}
	}
	t.Fatalf("membership not found for user %s in community %s", userID, communityID)
	return nil
}

func assertTransferState(t *testing.T, db *storage.ProtoSQLStorage, ctx context.Context, id string, want models.TransferState, label string) {
	t.Helper()
	got := &models.Transfer{}
	if err := db.GetByID(ctx, id, got, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("re-fetch %s: %v", label, err)
	}
	if got.State != want {
		t.Errorf("%s: state=%v, want %v", label, got.State, want)
	}
}

func stringPtr(s string) *string { return &s }
