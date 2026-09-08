package integration_tests

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// TestRestoreCommunity_HappyPath is the end-to-end proof for issue
// #1655: a snapshot member calls RestoreCommunity, the community
// flips back to active, the cascade rows are un-soft-deleted, the
// restorer becomes the new owner, and the dispatcher notifies the
// other snapshot members with the §8 copy.
func TestRestoreCommunity_HappyPath(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	restorerID := uuid.New().String() // different user — restorer becomes new owner
	memberCID := uuid.New().String()

	insertUser(t, db, ownerID, "owner@example.com", "Olivia Owner")
	insertUser(t, db, restorerID, "restorer@example.com", "Riley Restorer")
	insertUser(t, db, memberCID, "carol@example.com", "Carol")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id:          communityID,
		Name:        "Test Community",
		CreatorId:   ownerID,
		OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed community: %v", err)
	}
	for _, uid := range []string{ownerID, restorerID, memberCID} {
		if _, err := db.Insert(ctx, &models.CommunityUser{
			Id: uuid.New().String(), CommunityId: communityID, UserId: uid, InviterId: ownerID,
		}); err != nil {
			t.Fatalf("seed membership for %s: %v", uid, err)
		}
	}

	// Seed one of each cascade row type.
	gearID, _ := db.Insert(ctx, &models.Gear{Id: uuid.New().String(), Name: "Tent", OwnerId: ownerID, State: models.GearState_GEAR_STATE_AVAILABLE})
	cgID, _ := db.Insert(ctx, &models.CommunityGear{Id: uuid.New().String(), CommunityId: communityID, GearId: gearID, Availability: models.Availability_AVAILABILITY_FOR_LOAN})
	requestID, _ := db.Insert(ctx, &models.Request{Id: uuid.New().String(), Title: "Need a saw", RequesterId: restorerID})
	crID, _ := db.Insert(ctx, &models.CommunityRequest{Id: uuid.New().String(), CommunityId: communityID, RequestId: requestID})
	expID, _ := db.Insert(ctx, &models.Experience{Id: uuid.New().String(), Name: "Block party", OwnerId: memberCID})
	ceID, _ := db.Insert(ctx, &models.CommunityExperience{Id: uuid.New().String(), CommunityId: communityID, ExperienceId: expID})
	prefsID, _ := db.Insert(ctx, &models.CommunityNotificationPreferences{Id: uuid.New().String(), CommunityId: communityID, UserId: memberCID})
	linkID, _ := db.Insert(ctx, &models.CommunityInvitationLink{Id: uuid.New().String(), CommunityId: communityID, InviterId: ownerID, ShortCode: "abcd1234"})
	shareID, _ := db.Insert(ctx, &models.ShareLink{
		Id: uuid.New().String(), CommunityId: communityID, InviterId: ownerID, ShortCode: "efgh5678",
		Target: &models.ShareLink_CommunityInviteId{CommunityInviteId: communityID},
	})

	tempDir := t.TempDir()
	bucket, err := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	if err != nil {
		t.Fatalf("bucket: %v", err)
	}
	mockNotifs := notifications.NewMockService()
	communitySvc, bus := newTestCommunityService(t, db, bucket, mockNotifs)
	_ = bus // drained inline below

	// Step 1: owner deletes the community (uses #1654's path).
	ownerCtx := makeAuthContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	if _, err := communitySvc.DeleteCommunity(ownerCtx, connect.NewRequest(&api.DeleteCommunityRequest{Id: communityID})); err != nil {
		t.Fatalf("DeleteCommunity: %v", err)
	}
	drainCommunityEventBus(t, bus)
	mockNotifs.Reset()

	// Step 2: restorer (different user, in snapshot) restores.
	restorerCtx := makeAuthContext(restorerID, "restorer@example.com", models.Role_ROLE_USER)
	if _, err := communitySvc.RestoreCommunity(restorerCtx, connect.NewRequest(&api.RestoreCommunityRequest{CommunityId: communityID})); err != nil {
		t.Fatalf("RestoreCommunity: %v", err)
	}
	drainCommunityEventBus(t, bus)

	// === Assert: Community is active and ownership transferred. ===
	got := &models.Community{}
	if err := db.GetByID(ctx, communityID, got); err != nil {
		t.Fatalf("re-fetch community: %v", err)
	}
	if got.Deleted != nil {
		t.Errorf("Community.Deleted not cleared: %+v", got.Deleted)
	}
	if got.DeletedSnapshot != nil {
		t.Errorf("Community.DeletedSnapshot not cleared: %+v", got.DeletedSnapshot)
	}
	if got.OwnerUserId != restorerID {
		t.Errorf("OwnerUserId = %q, want %q (restorer)", got.OwnerUserId, restorerID)
	}

	// === Assert: every cascade row is un-soft-deleted. ===
	assertNotSoftDeleted(t, db, ctx, cgID, &models.CommunityGear{}, "community_gear")
	assertNotSoftDeleted(t, db, ctx, crID, &models.CommunityRequest{}, "community_request")
	assertNotSoftDeleted(t, db, ctx, ceID, &models.CommunityExperience{}, "community_experience")
	assertNotSoftDeleted(t, db, ctx, prefsID, &models.CommunityNotificationPreferences{}, "community_notification_preferences")
	assertNotSoftDeleted(t, db, ctx, linkID, &models.CommunityInvitationLink{}, "community_invitation_link")
	assertNotSoftDeleted(t, db, ctx, shareID, &models.ShareLink{}, "share_link")

	// === Assert: COMMUNITY_RESTORED event was recorded. ===
	events, _ := db.QueryByField(ctx, "community_id", communityID, &models.CommunityEvent{})
	var sawRestored bool
	for _, e := range events {
		ev := e.(*models.CommunityEvent)
		if ev.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED {
			sawRestored = true
			if ev.ActorId != restorerID {
				t.Errorf("COMMUNITY_RESTORED actor = %q, want %q", ev.ActorId, restorerID)
			}
		}
	}
	if !sawRestored {
		t.Errorf("no COMMUNITY_RESTORED event recorded")
	}

	// === Assert: dispatcher pushed §8 copy to non-restorer members. ===
	calls := mockNotifs.GetCalls()
	notified := make(map[string]*models.Notification)
	for _, c := range calls {
		notified[c.UserID] = c.Notification
	}
	if _, ok := notified[restorerID]; ok {
		t.Errorf("restorer should not have been notified")
	}
	for _, uid := range []string{ownerID, memberCID} {
		n, ok := notified[uid]
		if !ok {
			t.Errorf("snapshot member %s was not notified", uid)
			continue
		}
		if n.Title == "" || n.Body == "" {
			t.Errorf("member %s notification missing title/body: %+v", uid, n)
		}
	}
}

// TestRestoreCommunity_PermissionDeniedNonSnapshot covers the §2.7
// scenario: a leaver who was NOT in the snapshot at delete time gets
// PermissionDenied even within the 30-day window.
func TestRestoreCommunity_PermissionDeniedNonSnapshot(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	leaverID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia")
	insertUser(t, db, leaverID, "leaver@example.com", "Lee Leaver")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "Test", CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed community: %v", err)
	}
	// Owner is a member; leaver is NOT (already left before delete).
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: ownerID, InviterId: ownerID,
	}); err != nil {
		t.Fatalf("seed owner membership: %v", err)
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	mockNotifs := notifications.NewMockService()
	communitySvc, bus := newTestCommunityService(t, db, bucket, mockNotifs)
	_ = bus // drained inline below

	// Owner deletes — snapshot will only contain owner.
	ownerCtx := makeAuthContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	if _, err := communitySvc.DeleteCommunity(ownerCtx, connect.NewRequest(&api.DeleteCommunityRequest{Id: communityID})); err != nil {
		t.Fatalf("DeleteCommunity: %v", err)
	}
	drainCommunityEventBus(t, bus)

	// Leaver attempts to restore.
	leaverCtx := makeAuthContext(leaverID, "leaver@example.com", models.Role_ROLE_USER)
	_, err := communitySvc.RestoreCommunity(leaverCtx, connect.NewRequest(&api.RestoreCommunityRequest{CommunityId: communityID}))
	if err == nil {
		t.Fatalf("expected PermissionDenied, got nil")
	}
	if got := connect.CodeOf(err); got != connect.CodePermissionDenied {
		t.Fatalf("expected PermissionDenied, got %s", got)
	}
}

// TestRestoreCommunity_FailedPreconditionNotDeleted asserts that
// calling Restore on a never-deleted community returns
// FailedPrecondition.
func TestRestoreCommunity_FailedPreconditionNotDeleted(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia")
	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "Test", CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed community: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: ownerID, InviterId: ownerID,
	}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	communitySvc, _ := newTestCommunityService(t, db, bucket, notifications.NewMockService())

	ownerCtx := makeAuthContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	_, err := communitySvc.RestoreCommunity(ownerCtx, connect.NewRequest(&api.RestoreCommunityRequest{CommunityId: communityID}))
	if err == nil {
		t.Fatalf("expected FailedPrecondition, got nil")
	}
	if got := connect.CodeOf(err); got != connect.CodeFailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %s", got)
	}
}

// TestRestoreCommunity_RespectsPriorSoftDeletes covers §5: rows that
// were soft-deleted BEFORE the community-delete cascade stay
// soft-deleted after restore.
func TestRestoreCommunity_RespectsPriorSoftDeletes(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia")
	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "Test", CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed community: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: ownerID, InviterId: ownerID,
	}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	gearID, _ := db.Insert(ctx, &models.Gear{Id: uuid.New().String(), Name: "Tent", OwnerId: ownerID, State: models.GearState_GEAR_STATE_AVAILABLE})

	// Pre-soft-deleted CommunityGear (different actor + earlier time).
	priorDeleter := uuid.New().String()
	insertUser(t, db, priorDeleter, "prior@example.com", "Prior Deleter")
	priorCG := &models.CommunityGear{
		Id: uuid.New().String(), CommunityId: communityID, GearId: gearID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
		Deleted: &models.DeletedMetadata{
			DeletedByUserId:  priorDeleter,
			DeletedAtUnixSec: time.Now().Unix() - 3600, // an hour earlier
		},
	}
	if _, err := db.Insert(ctx, priorCG); err != nil {
		t.Fatalf("seed pre-deleted community_gear: %v", err)
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	mockNotifs := notifications.NewMockService()
	communitySvc, bus := newTestCommunityService(t, db, bucket, mockNotifs)
	_ = bus // drained inline below

	ownerCtx := makeAuthContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	if _, err := communitySvc.DeleteCommunity(ownerCtx, connect.NewRequest(&api.DeleteCommunityRequest{Id: communityID})); err != nil {
		t.Fatalf("DeleteCommunity: %v", err)
	}
	drainCommunityEventBus(t, bus)
	if _, err := communitySvc.RestoreCommunity(ownerCtx, connect.NewRequest(&api.RestoreCommunityRequest{CommunityId: communityID})); err != nil {
		t.Fatalf("RestoreCommunity: %v", err)
	}
	drainCommunityEventBus(t, bus)

	// The pre-deleted CommunityGear must STAY soft-deleted.
	got := &models.CommunityGear{}
	if err := db.GetByID(ctx, priorCG.Id, got, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("re-fetch pre-deleted community_gear: %v", err)
	}
	if got.Deleted == nil {
		t.Errorf("pre-deleted community_gear was incorrectly restored")
	}
	if got.Deleted != nil && got.Deleted.DeletedByUserId != priorDeleter {
		t.Errorf("pre-deleted community_gear deleted_by changed: got %q, want %q", got.Deleted.DeletedByUserId, priorDeleter)
	}
}

// TestRestoreCommunity_CrashWindowRecovery simulates the half-state
// produced by a crash between ClaimCommunityRestore and the resync
// Update: flat columns are clear but binary_proto still has Deleted
// set. The recovery branch in RestoreCommunity must detect this and
// complete the restore.
func TestRestoreCommunity_CrashWindowRecovery(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	ownerID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia")
	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "Test", CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed community: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: ownerID, InviterId: ownerID,
	}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	tempDir := t.TempDir()
	bucket, _ := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	mockNotifs := notifications.NewMockService()
	communitySvc, bus := newTestCommunityService(t, db, bucket, mockNotifs)
	_ = bus // drained inline below

	ownerCtx := makeAuthContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	if _, err := communitySvc.DeleteCommunity(ownerCtx, connect.NewRequest(&api.DeleteCommunityRequest{Id: communityID})); err != nil {
		t.Fatalf("DeleteCommunity: %v", err)
	}
	drainCommunityEventBus(t, bus)

	// Simulate the crash-window: flat columns clear but binary_proto
	// still says deleted. Done by directly invoking the claim helper
	// without the follow-up resync Update.
	claimed, err := db.ClaimCommunityRestore(ctx, communityID)
	if err != nil {
		t.Fatalf("ClaimCommunityRestore: %v", err)
	}
	if !claimed {
		t.Fatalf("ClaimCommunityRestore returned false; expected true")
	}
	// Sanity: GetByID returns Community.Deleted != nil despite flat clear.
	probe := &models.Community{}
	if err := db.GetByID(ctx, communityID, probe, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	if probe.Deleted == nil {
		t.Fatalf("probe expected Community.Deleted != nil before recovery")
	}

	// Now call RestoreCommunity — recovery branch should kick in.
	if _, err := communitySvc.RestoreCommunity(ownerCtx, connect.NewRequest(&api.RestoreCommunityRequest{CommunityId: communityID})); err != nil {
		t.Fatalf("RestoreCommunity recovery: %v", err)
	}
	drainCommunityEventBus(t, bus)

	// Final state: fully restored, both layers consistent.
	final := &models.Community{}
	if err := db.GetByID(ctx, communityID, final); err != nil {
		t.Fatalf("re-fetch final: %v", err)
	}
	if final.Deleted != nil {
		t.Errorf("Community.Deleted not cleared after recovery: %+v", final.Deleted)
	}
	if final.OwnerUserId != ownerID {
		t.Errorf("OwnerUserId = %q, want %q", final.OwnerUserId, ownerID)
	}
}

// assertNotSoftDeleted is the inverse of assertSoftDeleted — re-fetches
// with IncludeDeleted: true and asserts Deleted is nil/zero.
func assertNotSoftDeleted[T softDeletable](t *testing.T, db *storage.ProtoSQLStorage, ctx context.Context, id string, msg T, label string) {
	t.Helper()
	if err := db.GetByID(ctx, id, msg, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("re-fetch %s %s: %v", label, id, err)
	}
	deleted := msg.GetDeleted()
	if deleted != nil && deleted.DeletedAtUnixSec > 0 {
		t.Errorf("%s %s still soft-deleted: %+v", label, id, deleted)
	}
}
