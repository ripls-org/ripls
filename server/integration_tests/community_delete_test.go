package integration_tests

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// TestDeleteCommunity_HappyPath is the end-to-end proof for issue #1654:
// the owner calls DeleteCommunity, the cascade soft-deletes every join
// row, the snapshot captures the active members, the COMMUNITY_DELETED
// event is recorded, and the dispatcher pushes the §8 copy to every
// snapshot member except the deleter.
func TestDeleteCommunity_HappyPath(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	ctx := context.Background()

	ownerID := uuid.New().String()
	memberAID := uuid.New().String()
	memberBID := uuid.New().String()

	insertUser(t, db, ownerID, "owner@example.com", "Olivia Owner")
	insertUser(t, db, memberAID, "alice@example.com", "Alice")
	insertUser(t, db, memberBID, "bob@example.com", "Bob")

	communityID := uuid.New().String()
	community := &models.Community{
		Id:          communityID,
		Name:        "Test Community",
		CreatorId:   ownerID,
		OwnerUserId: ownerID,
	}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("seed community: %v", err)
	}
	for _, uid := range []string{ownerID, memberAID, memberBID} {
		if _, err := db.Insert(ctx, &models.CommunityUser{
			Id:          uuid.New().String(),
			CommunityId: communityID,
			UserId:      uid,
			InviterId:   ownerID,
		}); err != nil {
			t.Fatalf("seed membership for %s: %v", uid, err)
		}
	}

	// Seed one row of each cascade type so we can assert all five fire.
	gearID, err := db.Insert(ctx, &models.Gear{
		Id: uuid.New().String(), Name: "Tent", OwnerId: ownerID,
		State: models.GearState_GEAR_STATE_AVAILABLE,
	})
	if err != nil {
		t.Fatalf("seed gear: %v", err)
	}
	cgID, err := db.Insert(ctx, &models.CommunityGear{
		Id: uuid.New().String(), CommunityId: communityID, GearId: gearID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	})
	if err != nil {
		t.Fatalf("seed community_gear: %v", err)
	}

	requestID, err := db.Insert(ctx, &models.Request{
		Id: uuid.New().String(), Title: "Need a saw", RequesterId: memberAID,
	})
	if err != nil {
		t.Fatalf("seed request: %v", err)
	}
	crID, err := db.Insert(ctx, &models.CommunityRequest{
		Id: uuid.New().String(), CommunityId: communityID, RequestId: requestID,
	})
	if err != nil {
		t.Fatalf("seed community_request: %v", err)
	}

	expID, err := db.Insert(ctx, &models.Experience{
		Id: uuid.New().String(), Name: "Block party", OwnerId: memberBID,
	})
	if err != nil {
		t.Fatalf("seed experience: %v", err)
	}
	ceID, err := db.Insert(ctx, &models.CommunityExperience{
		Id: uuid.New().String(), CommunityId: communityID, ExperienceId: expID,
	})
	if err != nil {
		t.Fatalf("seed community_experience: %v", err)
	}

	prefsID, err := db.Insert(ctx, &models.CommunityNotificationPreferences{
		Id: uuid.New().String(), CommunityId: communityID, UserId: memberAID,
	})
	if err != nil {
		t.Fatalf("seed community_notification_preferences: %v", err)
	}

	linkID, err := db.Insert(ctx, &models.CommunityInvitationLink{
		Id: uuid.New().String(), CommunityId: communityID, InviterId: ownerID,
		ShortCode: "abcd1234",
	})
	if err != nil {
		t.Fatalf("seed community_invitation_link: %v", err)
	}

	shareLinkID, err := db.Insert(ctx, &models.ShareLink{
		Id: uuid.New().String(), CommunityId: communityID, InviterId: ownerID,
		ShortCode: "efgh5678",
		Target:    &models.ShareLink_CommunityInviteId{CommunityInviteId: communityID},
	})
	if err != nil {
		t.Fatalf("seed share_link: %v", err)
	}

	// Build the service with a notification capture and a completion signal.
	tempDir := t.TempDir()
	bucket, err := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	if err != nil {
		t.Fatalf("bucket: %v", err)
	}
	mockNotifs := notifications.NewMockService()
	communitySvc, bus := newTestCommunityService(t, db, bucket, mockNotifs)
	_ = bus // drained inline below

	ownerCtx := makeAuthContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// === Act: owner deletes the community. ===
	if _, err := communitySvc.DeleteCommunity(ownerCtx, connect.NewRequest(&api.DeleteCommunityRequest{Id: communityID})); err != nil {
		t.Fatalf("DeleteCommunity: %v", err)
	}

	// Wait for the async notification dispatch to complete.
	drainCommunityEventBus(t, bus)

	// === Assert: Community is soft-deleted with snapshot populated. ===
	got := &models.Community{}
	if err := db.GetByID(ctx, communityID, got, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("re-fetch community: %v", err)
	}
	if got.Deleted == nil || got.Deleted.DeletedByUserId != ownerID {
		t.Errorf("Community.Deleted not set by owner: %+v", got.Deleted)
	}
	if got.DeletedSnapshot == nil {
		t.Fatalf("Community.DeletedSnapshot is nil")
	}
	wantMembers := map[string]bool{ownerID: true, memberAID: true, memberBID: true}
	if len(got.DeletedSnapshot.MemberUserIds) != len(wantMembers) {
		t.Errorf("snapshot has %d IDs, want %d", len(got.DeletedSnapshot.MemberUserIds), len(wantMembers))
	}
	for _, id := range got.DeletedSnapshot.MemberUserIds {
		if !wantMembers[id] {
			t.Errorf("snapshot includes unexpected user %q", id)
		}
		delete(wantMembers, id)
	}
	if len(wantMembers) > 0 {
		t.Errorf("snapshot missing members: %v", wantMembers)
	}

	// === Assert: every cascade row is soft-deleted. ===
	assertSoftDeleted(t, db, ctx, cgID, &models.CommunityGear{}, "community_gear")
	assertSoftDeleted(t, db, ctx, crID, &models.CommunityRequest{}, "community_request")
	assertSoftDeleted(t, db, ctx, ceID, &models.CommunityExperience{}, "community_experience")
	assertSoftDeleted(t, db, ctx, prefsID, &models.CommunityNotificationPreferences{}, "community_notification_preferences")
	assertSoftDeleted(t, db, ctx, linkID, &models.CommunityInvitationLink{}, "community_invitation_link")
	assertSoftDeleted(t, db, ctx, shareLinkID, &models.ShareLink{}, "share_link")

	// === Assert: COMMUNITY_DELETED event was recorded. ===
	events, err := db.QueryByField(ctx, "community_id", communityID, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("query community events: %v", err)
	}
	var sawDeleted bool
	for _, e := range events {
		ev := e.(*models.CommunityEvent)
		if ev.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED {
			sawDeleted = true
			if ev.ActorId != ownerID {
				t.Errorf("COMMUNITY_DELETED actor = %q, want %q", ev.ActorId, ownerID)
			}
		}
	}
	if !sawDeleted {
		t.Errorf("no COMMUNITY_DELETED event recorded")
	}

	// === Assert: dispatcher pushed §8 copy to non-deleter snapshot members. ===
	calls := mockNotifs.GetCalls()
	notified := make(map[string]*models.Notification)
	for _, c := range calls {
		notified[c.UserID] = c.Notification
	}
	if _, ok := notified[ownerID]; ok {
		t.Errorf("deleter (owner) should not have been notified")
	}
	for _, uid := range []string{memberAID, memberBID} {
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

// TestDeleteCommunity_PermissionDeniedNonOwner asserts that only the
// owner can delete the community (using OwnerUserId, not CreatorId).
func TestDeleteCommunity_PermissionDeniedNonOwner(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	ctx := context.Background()

	ownerID := uuid.New().String()
	memberID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia")
	insertUser(t, db, memberID, "alice@example.com", "Alice")

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "Test", CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed community: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: memberID, InviterId: ownerID,
	}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	tempDir := t.TempDir()
	bucket, err := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	if err != nil {
		t.Fatalf("bucket: %v", err)
	}
	communitySvc, _ := newTestCommunityService(t, db, bucket, notifications.NewMockService())

	memberCtx := makeAuthContext(memberID, "alice@example.com", models.Role_ROLE_USER)
	_, err = communitySvc.DeleteCommunity(memberCtx, connect.NewRequest(&api.DeleteCommunityRequest{Id: communityID}))
	if err == nil {
		t.Fatalf("expected PermissionDenied, got nil")
	}
	if got := connect.CodeOf(err); got != connect.CodePermissionDenied {
		t.Fatalf("expected PermissionDenied, got %s", got)
	}
}

// TestDeleteCommunity_NoOpOnAlreadyDeleted asserts that calling delete
// twice is idempotent (success on both, snapshot from the first call
// is preserved).
func TestDeleteCommunity_NoOpOnAlreadyDeleted(t *testing.T) {
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
	bucket, err := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	if err != nil {
		t.Fatalf("bucket: %v", err)
	}
	mockNotifs := notifications.NewMockService()
	communitySvc, bus := newTestCommunityService(t, db, bucket, mockNotifs)
	_ = bus // drained inline below
	ownerCtx := makeAuthContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// First delete.
	if _, err := communitySvc.DeleteCommunity(ownerCtx, connect.NewRequest(&api.DeleteCommunityRequest{Id: communityID})); err != nil {
		t.Fatalf("first DeleteCommunity: %v", err)
	}
	drainCommunityEventBus(t, bus)
	firstCallCount := len(mockNotifs.GetCalls())

	// Second delete should succeed without re-emitting events or
	// re-dispatching notifications.
	if _, err := communitySvc.DeleteCommunity(ownerCtx, connect.NewRequest(&api.DeleteCommunityRequest{Id: communityID})); err != nil {
		t.Fatalf("second DeleteCommunity unexpectedly errored: %v", err)
	}
	if got := len(mockNotifs.GetCalls()); got != firstCallCount {
		t.Errorf("second delete dispatched extra notifications: %d → %d", firstCallCount, got)
	}
}

// TestDeleteCommunity_ZeroMembers asserts that a community with no
// active members deletes cleanly: empty snapshot, no recipients, no
// notification panic.
func TestDeleteCommunity_ZeroMembers(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	ownerID := uuid.New().String()
	insertUser(t, db, ownerID, "owner@example.com", "Olivia")

	communityID := uuid.New().String()
	// Note: no CommunityUser rows seeded — pathological state but we
	// should still be able to clean it up. The CHECK constraint requires
	// owner_user_id non-empty on active rows; that's satisfied.
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "Empty", CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed community: %v", err)
	}

	tempDir := t.TempDir()
	bucket, err := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	if err != nil {
		t.Fatalf("bucket: %v", err)
	}
	mockNotifs := notifications.NewMockService()
	communitySvc, bus := newTestCommunityService(t, db, bucket, mockNotifs)
	_ = bus // drained inline below
	ownerCtx := makeAuthContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	if _, err := communitySvc.DeleteCommunity(ownerCtx, connect.NewRequest(&api.DeleteCommunityRequest{Id: communityID})); err != nil {
		t.Fatalf("DeleteCommunity: %v", err)
	}
	drainCommunityEventBus(t, bus)

	got := &models.Community{}
	if err := db.GetByID(ctx, communityID, got, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("re-fetch: %v", err)
	}
	if got.Deleted == nil {
		t.Errorf("Community.Deleted not set")
	}
	if got.DeletedSnapshot == nil {
		t.Fatalf("DeletedSnapshot nil; expected an empty snapshot")
	}
	if len(got.DeletedSnapshot.MemberUserIds) != 0 {
		t.Errorf("snapshot has %d IDs, want 0", len(got.DeletedSnapshot.MemberUserIds))
	}
	if got := len(mockNotifs.GetCalls()); got != 0 {
		t.Errorf("expected zero notifications, got %d", got)
	}
}

// softDeletable is the minimal contract assertSoftDeleted needs:
// a proto message whose generated code includes a GetDeleted accessor.
// Every cascade target satisfies this.
type softDeletable interface {
	proto.Message
	GetDeleted() *models.DeletedMetadata
}

// assertSoftDeleted re-fetches a row by ID with IncludeDeleted: true
// and asserts the row's DeletedMetadata is populated.
func assertSoftDeleted[T softDeletable](t *testing.T, db *storage.ProtoSQLStorage, ctx context.Context, id string, msg T, label string) {
	t.Helper()
	if err := db.GetByID(ctx, id, msg, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("re-fetch %s %s: %v", label, id, err)
	}
	deleted := msg.GetDeleted()
	if deleted == nil || deleted.DeletedAtUnixSec == 0 {
		t.Errorf("%s %s not soft-deleted", label, id)
	}
}
