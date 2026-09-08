package community_event_bus

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/pubsub"
	"go.ripls.org/ripls/server/storage"
)

// setup returns a storage instance, a fresh bus wired to a MemTopic, and a
// teardown helper. Subscribers should be registered on the returned bus.
func setup(t *testing.T) (*storage.ProtoSQLStorage, *InProcessBus, func()) {
	t.Helper()
	st, cleanup := storage.SetupTestStorage(t)
	topic := pubsub.NewMemTopic[*PublishedEvent](TopicName)
	bus := NewInProcessBus(st, topic)
	return st, bus, cleanup
}

func TestInProcessBus_PublishInsertsEventAndDispatches(t *testing.T) {
	st, bus, cleanup := setup(t)
	defer cleanup()

	sub := pubsub.NewFakeSubscriber[*PublishedEvent]("test-sub")

	unsub, err := bus.Subscribe(sub)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	ctx := context.Background()
	event := &models.CommunityEvent{
		CommunityId: "community-1",
		ActorId:     "user-1",
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
	}

	id, err := bus.Publish(ctx, event)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if id == "" {
		t.Fatal("Publish returned empty event ID")
	}
	if event.Id != id {
		t.Errorf("event.Id = %q, want %q (publisher should write back)", event.Id, id)
	}
	if event.OccurredAtUnixSec == 0 {
		t.Error("OccurredAtUnixSec should default from clock.UnixSec")
	}

	// Verify the row was actually inserted.
	persisted := &models.CommunityEvent{}
	if err := st.GetByID(ctx, id, persisted); err != nil {
		t.Fatalf("GetByID for persisted event: %v", err)
	}
	if persisted.CommunityId != "community-1" {
		t.Errorf("persisted CommunityId = %q, want community-1", persisted.CommunityId)
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
	if got[0].Event == nil || got[0].Event.Id != id {
		t.Errorf("subscriber got event with id %q, want %q", got[0].Event.Id, id)
	}
}

func TestInProcessBus_PublishPrefetchesGearAndActor(t *testing.T) {
	st, bus, cleanup := setup(t)
	defer cleanup()

	ctx := context.Background()

	// Insert prerequisite rows so the prefetch reads find them.
	user := &models.User{Name: "Alice", Email: "alice@example.com", Role: models.Role_ROLE_USER}
	userID, err := st.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Insert user: %v", err)
	}
	user.Id = userID

	gear := &models.Gear{Name: "Tent", OwnerId: userID}
	gearID, err := st.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("Insert gear: %v", err)
	}

	// Subscriber records what it received.
	sub := pubsub.NewFakeSubscriber[*PublishedEvent]("prefetch-checker")
	unsub, err := bus.Subscribe(sub)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	event := &models.CommunityEvent{
		CommunityId: "community-1",
		ActorId:     userID,
		GearId:      gearID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
	}

	if _, err := bus.Publish(ctx, event); err != nil {
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
	pe := got[0]
	if pe.Gear == nil {
		t.Error("PublishedEvent.Gear should be prefetched, got nil")
	} else if pe.Gear.Name != "Tent" {
		t.Errorf("PublishedEvent.Gear.Name = %q, want Tent", pe.Gear.Name)
	}
	if pe.Actor == nil {
		t.Error("PublishedEvent.Actor should be prefetched, got nil")
	} else if pe.Actor.Name != "Alice" {
		t.Errorf("PublishedEvent.Actor.Name = %q, want Alice", pe.Actor.Name)
	}
	// No transfer/request/experience referenced — must remain nil.
	if pe.Transfer != nil {
		t.Errorf("PublishedEvent.Transfer should be nil, got %v", pe.Transfer)
	}
	if pe.Request != nil {
		t.Errorf("PublishedEvent.Request should be nil, got %v", pe.Request)
	}
	if pe.Experience != nil {
		t.Errorf("PublishedEvent.Experience should be nil, got %v", pe.Experience)
	}
}

func TestInProcessBus_PrefetchFailureDoesNotAbortDispatch(t *testing.T) {
	_, bus, cleanup := setup(t)
	defer cleanup()

	// Subscriber receives the event even when the referenced gear doesn't
	// exist (the prefetch logs at WARN and leaves Gear nil).
	sub := pubsub.NewFakeSubscriber[*PublishedEvent]("missing-gear")
	unsub, err := bus.Subscribe(sub)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	event := &models.CommunityEvent{
		CommunityId: "community-1",
		ActorId:     "missing-user",
		GearId:      "missing-gear",
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
	}
	if _, err := bus.Publish(context.Background(), event); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bus.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	got := sub.Received()
	if len(got) != 1 {
		t.Fatalf("subscriber received %d events despite prefetch errors, got %d", len(got), 1)
	}
	if got[0].Gear != nil {
		t.Errorf("expected Gear nil after prefetch failure, got %+v", got[0].Gear)
	}
	if got[0].Actor != nil {
		t.Errorf("expected Actor nil after prefetch failure, got %+v", got[0].Actor)
	}
}

func TestInProcessBus_StorageInsertFailureSurfacesError(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	// Use a MockTopic; we want to assert that on storage failure no
	// publish happens.
	mockTopic := pubsub.NewMockTopic[*PublishedEvent]()
	bus := NewInProcessBus(st, mockTopic)

	// Closing the storage's connection induces an Insert failure.
	st.Close()

	event := &models.CommunityEvent{
		CommunityId: "community-1",
		ActorId:     "user-1",
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
	}
	id, err := bus.Publish(context.Background(), event)
	if err == nil {
		t.Fatal("Publish should return error when storage insert fails")
	}
	if id != "" {
		t.Errorf("Publish on storage failure returned id=%q, want empty", id)
	}
	if got := mockTopic.Captured(); len(got) != 0 {
		t.Errorf("MockTopic captured %d events on storage failure, want 0", len(got))
	}
}

func TestInProcessBus_MultipleSubscribersIsolated(t *testing.T) {
	_, bus, cleanup := setup(t)
	defer cleanup()

	good := pubsub.NewFakeSubscriber[*PublishedEvent]("good")
	bad := pubsub.NewFakeSubscriber[*PublishedEvent]("bad")
	bad.HandleErr = errors.New("subscriber boom")

	unsubGood, _ := bus.Subscribe(good)
	defer unsubGood()
	unsubBad, _ := bus.Subscribe(bad)
	defer unsubBad()

	event := &models.CommunityEvent{
		CommunityId: "community-1",
		ActorId:     "user-1",
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
	}
	if _, err := bus.Publish(context.Background(), event); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bus.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	if len(good.Received()) != 1 {
		t.Errorf("good subscriber received %d events, want 1", len(good.Received()))
	}
	if len(bad.Received()) != 1 {
		t.Errorf("bad subscriber should still have seen the event (Handle was called), got %d", len(bad.Received()))
	}
}

func TestInProcessBus_OccurredAtUnixSecPreservedWhenSet(t *testing.T) {
	_, bus, cleanup := setup(t)
	defer cleanup()

	const explicitTS = int64(1_700_000_000)
	event := &models.CommunityEvent{
		CommunityId:       "community-1",
		ActorId:           "user-1",
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		OccurredAtUnixSec: explicitTS,
	}
	if _, err := bus.Publish(context.Background(), event); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if event.OccurredAtUnixSec != explicitTS {
		t.Errorf("OccurredAtUnixSec = %d, want preserved %d", event.OccurredAtUnixSec, explicitTS)
	}
}

func TestInProcessBus_NilEventReturnsError(t *testing.T) {
	_, bus, cleanup := setup(t)
	defer cleanup()

	if _, err := bus.Publish(context.Background(), nil); err == nil {
		t.Error("Publish(nil) should return an error")
	}
}

func TestPublishedEvent_LogContextAlwaysIncludesCanonicalFields(t *testing.T) {
	pe := &PublishedEvent{Event: &models.CommunityEvent{
		Id:          "evt-123",
		CommunityId: "community-x",
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE,
	}}
	got := pe.LogContext()
	want := map[string]string{
		"community_event_id": "evt-123",
		"community_id":       "community-x",
		"event_type":         "COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE",
	}
	for i := 0; i+1 < len(got); i += 2 {
		key, _ := got[i].(string)
		if expected, ok := want[key]; ok {
			actual, _ := got[i+1].(string)
			if actual != expected {
				t.Errorf("LogContext[%q] = %q, want %q", key, actual, expected)
			}
			delete(want, key)
		}
	}
	if len(want) != 0 {
		t.Errorf("LogContext missing expected keys: %v", want)
	}
}

func TestPublishedEvent_LogContextSafeOnNil(t *testing.T) {
	var pe *PublishedEvent
	if got := pe.LogContext(); got != nil {
		t.Errorf("(*PublishedEvent)(nil).LogContext() = %v, want nil", got)
	}
	pe = &PublishedEvent{}
	if got := pe.LogContext(); got != nil {
		t.Errorf("PublishedEvent{}.LogContext() = %v, want nil (event field missing)", got)
	}
}
