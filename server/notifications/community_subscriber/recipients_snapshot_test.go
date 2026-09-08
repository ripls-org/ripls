package community_subscriber

import (
	"context"
	"testing"

	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
)

// The #2657 regression: broadcast recipients used to be resolved from live
// membership at dispatch time, so a user who joined the community between an
// event's publish and its async dispatch received a push for an event that
// predates their membership (in the wild: SaveExperience publishes
// EXPERIENCE_CREATED for the ad-hoc origin community, then an RSVP promotes
// the RSVPer into that community before the lagging subscriber dispatches).
// With the publish-time snapshot on the envelope, the audience is pinned.
func TestSubscriber_SnapshotPinsExperienceCreatedAudience(t *testing.T) {
	s := setupTestStorage(t)
	mockNotif := notifications.NewMockService()

	ownerID := insertTestUser(t, s, "owner@example.com", "Owner")
	memberID := insertTestUser(t, s, "member@example.com", "Member")
	lateJoinerID := insertTestUser(t, s, "late@example.com", "Late Joiner")
	communityID := insertTestCommunity(t, s, "Test Community", ownerID)
	insertMembership(t, s, communityID, ownerID)
	insertMembership(t, s, communityID, memberID)

	// Envelope built "at publish time": only owner + member are members.
	evt := &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			Id:          "evt-snapshot",
			CommunityId: communityID,
			ActorId:     ownerID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		},
		MemberIDsAtPublish: []string{ownerID, memberID},
	}

	// The late joiner lands between publish and dispatch.
	insertMembership(t, s, communityID, lateJoinerID)

	sub := New(s, mockNotif)
	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	calls := mockNotif.GetCalls()
	if len(calls) != 1 {
		t.Fatalf("NotifyUser called %d times, want 1 (member only)", len(calls))
	}
	if calls[0].UserID != memberID {
		t.Errorf("notified %s, want member %s", calls[0].UserID, memberID)
	}
}

// GEAR_SHARED goes through the same member-broadcast arm; pin it too so the
// shared code path can't regress for one type while the other stays covered.
func TestSubscriber_SnapshotPinsGearSharedAudience(t *testing.T) {
	s := setupTestStorage(t)
	mockNotif := notifications.NewMockService()

	ownerID := insertTestUser(t, s, "owner@example.com", "Owner")
	memberID := insertTestUser(t, s, "member@example.com", "Member")
	lateJoinerID := insertTestUser(t, s, "late@example.com", "Late Joiner")
	communityID := insertTestCommunity(t, s, "Test Community", ownerID)
	insertMembership(t, s, communityID, ownerID)
	insertMembership(t, s, communityID, memberID)

	evt := &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			Id:          "evt-gear-snapshot",
			CommunityId: communityID,
			ActorId:     ownerID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		},
		MemberIDsAtPublish: []string{ownerID, memberID},
	}

	insertMembership(t, s, communityID, lateJoinerID)

	sub := New(s, mockNotif)
	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	calls := mockNotif.GetCalls()
	if len(calls) != 1 {
		t.Fatalf("NotifyUser called %d times, want 1 (member only)", len(calls))
	}
	if calls[0].UserID != memberID {
		t.Errorf("notified %s, want member %s", calls[0].UserID, memberID)
	}
}

// An envelope without a snapshot (failed snapshot read, or a publisher that
// bypasses InProcessBus) must fall back to live membership so the fan-out is
// degraded — back to the pre-snapshot dispatch-lag exposure — rather than
// dropped entirely.
func TestSubscriber_NilSnapshotFallsBackToLiveMembership(t *testing.T) {
	s := setupTestStorage(t)
	mockNotif := notifications.NewMockService()

	ownerID := insertTestUser(t, s, "owner@example.com", "Owner")
	memberID := insertTestUser(t, s, "member@example.com", "Member")
	communityID := insertTestCommunity(t, s, "Test Community", ownerID)
	insertMembership(t, s, communityID, ownerID)
	insertMembership(t, s, communityID, memberID)

	evt := &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			Id:          "evt-no-snapshot",
			CommunityId: communityID,
			ActorId:     ownerID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		},
	}

	sub := New(s, mockNotif)
	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	calls := mockNotif.GetCalls()
	if len(calls) != 1 {
		t.Fatalf("NotifyUser called %d times, want 1 (live-membership fallback)", len(calls))
	}
	if calls[0].UserID != memberID {
		t.Errorf("notified %s, want member %s", calls[0].UserID, memberID)
	}
}

func TestResolveRecipients_SnapshotSemantics(t *testing.T) {
	// No storage: resolveRecipients must never touch it when a snapshot is
	// present, so a nil ProtoSQLStorage doubles as the assertion.
	event := &models.CommunityEvent{
		Id:          "evt-unit",
		CommunityId: "community-1",
		ActorId:     "actor",
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
	}

	t.Run("ActorExcluded", func(t *testing.T) {
		evt := &cebus.PublishedEvent{
			Event:              event,
			MemberIDsAtPublish: []string{"actor", "a", "b"},
		}
		got := resolveRecipients(context.Background(), nil, evt)
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Errorf("resolveRecipients = %v, want [a b]", got)
		}
	})

	t.Run("EmptySnapshotMeansNobodyNotFallback", func(t *testing.T) {
		// A successful snapshot of a community with no members is non-nil
		// empty; it must resolve to no recipients, not trigger the live
		// fallback (nil storage would panic if it did).
		evt := &cebus.PublishedEvent{
			Event:              event,
			MemberIDsAtPublish: []string{},
		}
		if got := resolveRecipients(context.Background(), nil, evt); len(got) != 0 {
			t.Errorf("resolveRecipients = %v, want empty", got)
		}
	})
}
