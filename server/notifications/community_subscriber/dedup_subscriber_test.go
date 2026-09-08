package community_subscriber

import (
	"context"
	"testing"

	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications"
)

// countCallsFor returns how many NotifyUser calls targeted userID.
func countCallsFor(calls []notifications.NotificationCall, userID string) int {
	n := 0
	for _, c := range calls {
		if c.UserID == userID {
			n++
		}
	}
	return n
}

// gearSharedEvent builds a GEAR_SHARED event for one community + gear.
func gearSharedEvent(id, communityID, actorID, gearID string) *cebus.PublishedEvent {
	return &cebus.PublishedEvent{Event: &models.CommunityEvent{
		Id:          id,
		CommunityId: communityID,
		ActorId:     actorID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		GearId:      gearID,
	}}
}

// TestSubscriber_DedupsBroadcastAcrossCommunities is the #2088 regression: a
// gear shared into two communities a user belongs to fans out into two
// GEAR_SHARED events sharing one originating request_id, and the user must get
// exactly one push — not one per community.
func TestSubscriber_DedupsBroadcastAcrossCommunities(t *testing.T) {
	store := setupTestStorage(t)
	actorID := insertTestUser(t, store, "actor@example.com", "Actor")
	memberID := insertTestUser(t, store, "member@example.com", "Member")
	c1 := insertTestCommunity(t, store, "Community One", actorID)
	c2 := insertTestCommunity(t, store, "Community Two", actorID)
	for _, c := range []string{c1, c2} {
		insertMembership(t, store, c, actorID)
		insertMembership(t, store, c, memberID)
	}

	mock := notifications.NewMockService()
	sub := New(store, mock)

	// Both events carry the same request_id — the bus propagates the
	// originating RPC's correlation id to every fan-out dispatch.
	ctx := logging.WithRequestID(context.Background(), "req-share-1")
	if err := sub.Handle(ctx, gearSharedEvent("evt-c1", c1, actorID, "gear-1")); err != nil {
		t.Fatalf("Handle evt-c1: %v", err)
	}
	if err := sub.Handle(ctx, gearSharedEvent("evt-c2", c2, actorID, "gear-1")); err != nil {
		t.Fatalf("Handle evt-c2: %v", err)
	}

	if got := countCallsFor(mock.GetCalls(), memberID); got != 1 {
		t.Errorf("member notified %d times for the same gear across 2 communities, want 1", got)
	}
}

// TestSubscriber_DistinctActionsNotDeduped guards against over-suppression: the
// same gear shipped under two *different* request_ids (two separate user
// actions) must notify the member both times.
func TestSubscriber_DistinctActionsNotDeduped(t *testing.T) {
	store := setupTestStorage(t)
	actorID := insertTestUser(t, store, "actor@example.com", "Actor")
	memberID := insertTestUser(t, store, "member@example.com", "Member")
	c1 := insertTestCommunity(t, store, "Community One", actorID)
	insertMembership(t, store, c1, actorID)
	insertMembership(t, store, c1, memberID)

	mock := notifications.NewMockService()
	sub := New(store, mock)

	ctxA := logging.WithRequestID(context.Background(), "req-A")
	ctxB := logging.WithRequestID(context.Background(), "req-B")
	if err := sub.Handle(ctxA, gearSharedEvent("evt-a", c1, actorID, "gear-1")); err != nil {
		t.Fatalf("Handle evt-a: %v", err)
	}
	if err := sub.Handle(ctxB, gearSharedEvent("evt-b", c1, actorID, "gear-1")); err != nil {
		t.Fatalf("Handle evt-b: %v", err)
	}

	if got := countCallsFor(mock.GetCalls(), memberID); got != 2 {
		t.Errorf("member notified %d times for two distinct actions, want 2", got)
	}
}

// TestSubscriber_NoRequestIDBypassesDedup confirms that without a propagated
// request_id (e.g. a background-job publisher) the guard is inert and behaves
// exactly as before — both events notify.
func TestSubscriber_NoRequestIDBypassesDedup(t *testing.T) {
	store := setupTestStorage(t)
	actorID := insertTestUser(t, store, "actor@example.com", "Actor")
	memberID := insertTestUser(t, store, "member@example.com", "Member")
	c1 := insertTestCommunity(t, store, "Community One", actorID)
	c2 := insertTestCommunity(t, store, "Community Two", actorID)
	for _, c := range []string{c1, c2} {
		insertMembership(t, store, c, actorID)
		insertMembership(t, store, c, memberID)
	}

	mock := notifications.NewMockService()
	sub := New(store, mock)

	ctx := context.Background() // no request_id
	if err := sub.Handle(ctx, gearSharedEvent("evt-c1", c1, actorID, "gear-1")); err != nil {
		t.Fatalf("Handle evt-c1: %v", err)
	}
	if err := sub.Handle(ctx, gearSharedEvent("evt-c2", c2, actorID, "gear-1")); err != nil {
		t.Fatalf("Handle evt-c2: %v", err)
	}

	if got := countCallsFor(mock.GetCalls(), memberID); got != 2 {
		t.Errorf("member notified %d times without a request_id, want 2 (dedup must be inert)", got)
	}
}

// TestSubscriber_DedupsTargetedAcrossCommunities covers the second, more severe
// mechanism: a REQUEST_CANCELLED whose recipients (offerers) are
// community-independent, fired once per community. The single offerer must get
// one push, not one per community.
func TestSubscriber_DedupsTargetedAcrossCommunities(t *testing.T) {
	store := setupTestStorage(t)
	actorID := insertTestUser(t, store, "actor@example.com", "Actor")
	offererID := insertTestUser(t, store, "offerer@example.com", "Offerer")
	c1 := insertTestCommunity(t, store, "Community One", actorID)
	c2 := insertTestCommunity(t, store, "Community Two", actorID)
	insertMembership(t, store, c1, actorID)
	insertMembership(t, store, c2, actorID)

	const requestID = "request-entity-1"
	if _, err := store.Insert(context.Background(), &models.RequestOffer{
		RequestId: requestID,
		UserId:    offererID,
		Withdrawn: false,
	}); err != nil {
		t.Fatalf("insert request offer: %v", err)
	}

	mock := notifications.NewMockService()
	sub := New(store, mock)

	cancelled := func(id, communityID string) *cebus.PublishedEvent {
		return &cebus.PublishedEvent{Event: &models.CommunityEvent{
			Id:          id,
			CommunityId: communityID,
			ActorId:     actorID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED,
			Topic:       &models.CommunityEvent_RequestId{RequestId: requestID},
		}}
	}

	ctx := logging.WithRequestID(context.Background(), "req-cancel-1")
	if err := sub.Handle(ctx, cancelled("evt-c1", c1)); err != nil {
		t.Fatalf("Handle evt-c1: %v", err)
	}
	if err := sub.Handle(ctx, cancelled("evt-c2", c2)); err != nil {
		t.Fatalf("Handle evt-c2: %v", err)
	}

	if got := countCallsFor(mock.GetCalls(), offererID); got != 1 {
		t.Errorf("offerer notified %d times for one fulfillment across 2 communities, want 1", got)
	}
}

// TestSubscriber_OutcomeSuppressedDuplicate asserts the duplicate suppression
// emits the structured outcome value Cloud Logging groups on.
func TestSubscriber_OutcomeSuppressedDuplicate(t *testing.T) {
	store := setupTestStorage(t)
	actorID := insertTestUser(t, store, "actor@example.com", "Actor")
	memberID := insertTestUser(t, store, "member@example.com", "Member")
	c1 := insertTestCommunity(t, store, "Community One", actorID)
	c2 := insertTestCommunity(t, store, "Community Two", actorID)
	for _, c := range []string{c1, c2} {
		insertMembership(t, store, c, actorID)
		insertMembership(t, store, c, memberID)
	}

	ctx, sink := sinkCtx()
	ctx = logging.WithRequestID(ctx, "req-share-dup")
	sub := New(store, notifications.NewMockService())

	if err := sub.Handle(ctx, gearSharedEvent("evt-c1", c1, actorID, "gear-1")); err != nil {
		t.Fatalf("Handle evt-c1: %v", err)
	}
	if err := sub.Handle(ctx, gearSharedEvent("evt-c2", c2, actorID, "gear-1")); err != nil {
		t.Fatalf("Handle evt-c2: %v", err)
	}

	// The member is sent on the first community and suppressed on the
	// second; assert the suppression outcome appears across the run.
	if !hasOutcomeFor(sink.Records(), memberID, outcomeSuppressedDuplicate) {
		t.Errorf("no %q outcome logged for member across the fan-out", outcomeSuppressedDuplicate)
	}
}

// hasOutcomeFor reports whether any record for userID carries the given outcome.
func hasOutcomeFor(records []logging.LogRecord, userID, outcome string) bool {
	for _, r := range records {
		if uid, _ := r.Attrs["user_id"].(string); uid != userID {
			continue
		}
		if out, _ := r.Attrs["outcome"].(string); out == outcome {
			return true
		}
	}
	return false
}
