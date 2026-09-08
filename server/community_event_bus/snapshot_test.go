package community_event_bus

import (
	"context"
	"sort"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/pubsub"
	"go.ripls.org/ripls/server/storage"
)

func TestSnapshotsMembers_TypeList(t *testing.T) {
	snapshotted := []models.CommunityEventType{
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED,
	}
	for _, et := range snapshotted {
		if !SnapshotsMembers(et) {
			t.Errorf("SnapshotsMembers(%v) = false, want true", et)
		}
	}
	// COMMUNITY_DELETED's audience is the deleted_snapshot capture; targeted
	// types resolve single recipients that don't race membership.
	for _, et := range []models.CommunityEventType{
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED,
	} {
		if SnapshotsMembers(et) {
			t.Errorf("SnapshotsMembers(%v) = true, want false", et)
		}
	}
}

func TestInProcessBus_PublishSnapshotsMembersForBroadcastTypes(t *testing.T) {
	st, bus, cleanup := setup(t)
	defer cleanup()

	ctx := context.Background()
	for _, userID := range []string{"user-1", "user-2"} {
		if _, err := st.Insert(ctx, &models.CommunityUser{
			CommunityId: "community-1",
			UserId:      userID,
		}); err != nil {
			t.Fatalf("Insert membership for %s: %v", userID, err)
		}
	}

	sub := pubsub.NewFakeSubscriber[*PublishedEvent]("snapshot-checker")
	unsub, err := bus.Subscribe(sub)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	if _, err := bus.Publish(ctx, &models.CommunityEvent{
		CommunityId: "community-1",
		ActorId:     "user-1",
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	drainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := bus.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	got := sub.Received()
	if len(got) != 1 {
		t.Fatalf("subscriber received %d events, want 1", len(got))
	}
	snapshot := got[0].MemberIDsAtPublish
	if snapshot == nil {
		t.Fatal("MemberIDsAtPublish = nil for a broadcast type, want member capture")
	}
	sort.Strings(snapshot)
	if len(snapshot) != 2 || snapshot[0] != "user-1" || snapshot[1] != "user-2" {
		t.Errorf("MemberIDsAtPublish = %v, want [user-1 user-2]", snapshot)
	}
}

func TestInProcessBus_PublishSkipsSnapshotForTargetedTypes(t *testing.T) {
	st, bus, cleanup := setup(t)
	defer cleanup()

	ctx := context.Background()
	if _, err := st.Insert(ctx, &models.CommunityUser{
		CommunityId: "community-1",
		UserId:      "user-1",
	}); err != nil {
		t.Fatalf("Insert membership: %v", err)
	}

	sub := pubsub.NewFakeSubscriber[*PublishedEvent]("targeted-checker")
	unsub, err := bus.Subscribe(sub)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	if _, err := bus.Publish(ctx, &models.CommunityEvent{
		CommunityId: "community-1",
		ActorId:     "user-2",
		GearId:      "gear-1",
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	drainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := bus.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	got := sub.Received()
	if len(got) != 1 {
		t.Fatalf("subscriber received %d events, want 1", len(got))
	}
	if got[0].MemberIDsAtPublish != nil {
		t.Errorf("MemberIDsAtPublish = %v for a targeted type, want nil", got[0].MemberIDsAtPublish)
	}
}

func TestSnapshotMembers_QueryFailureLeavesNil(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	// Closing the storage's connection induces a query failure.
	st.Close()

	pe := &PublishedEvent{Event: &models.CommunityEvent{
		Id:          "evt-1",
		CommunityId: "community-1",
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
	}}
	snapshotMembers(context.Background(), st, pe)
	if pe.MemberIDsAtPublish != nil {
		t.Errorf("MemberIDsAtPublish = %v after query failure, want nil (fail-open)", pe.MemberIDsAtPublish)
	}
}
