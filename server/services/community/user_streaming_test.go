package community

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications"
)

// fakeUserSender drives runUserStreamLoop without a Connect HTTP pipeline.
// Mirrors fakeSender in streaming_test.go.
type fakeUserSender struct {
	sends     atomic.Int64
	heartbeat atomic.Int64
	event     atomic.Int64
	failOn    int64 // if >0, Send returns an error on the Nth (1-indexed) call
}

func (f *fakeUserSender) Send(msg *api.StreamUserEventsResponse) error {
	n := f.sends.Add(1)
	if msg.GetHeartbeat() != nil {
		f.heartbeat.Add(1)
	}
	if msg.GetEvent() != nil {
		f.event.Add(1)
	}
	if f.failOn > 0 && n == f.failOn {
		return errors.New("simulated dead peer")
	}
	return nil
}

func TestRegisterUserStream(t *testing.T) {
	testStorage := setupTestStorage(t)
	svc := setupTestService(t, testStorage)

	ch := make(chan *api.StreamUserEventsResponse, 10)
	svc.registerUserStream(context.Background(), "user-1", ch)

	if !svc.HasActiveUserStream("user-1") {
		t.Error("expected user-1 to have an active user stream")
	}
	if !svc.hasAnyUserStream() {
		t.Error("expected hasAnyUserStream to report true")
	}

	svc.unregisterUserStream(context.Background(), "user-1", ch)

	if svc.HasActiveUserStream("user-1") {
		t.Error("expected user-1 to have no active user stream after unregister")
	}
	if svc.hasAnyUserStream() {
		t.Error("expected hasAnyUserStream to report false after unregister")
	}
}

// TestUnregisterUserStream_DoesNotCloseChannel pins the ownership rule stated in
// user_streaming.go: unregister removes the channel from the broadcast registry
// but must NOT close it. broadcastToUserStreams snapshots subscribers under
// RLock and sends outside the lock, so an in-flight broadcaster can still hold
// this channel after unregister returns — closing would make that send panic.
func TestUnregisterUserStream_DoesNotCloseChannel(t *testing.T) {
	testStorage := setupTestStorage(t)
	svc := setupTestService(t, testStorage)

	ch := make(chan *api.StreamUserEventsResponse, 10)
	svc.registerUserStream(context.Background(), "user-1", ch)
	svc.unregisterUserStream(context.Background(), "user-1", ch)

	select {
	case ch <- &api.StreamUserEventsResponse{}:
		// Good — still open.
	default:
		t.Error("expected channel to still be open and have buffer space")
	}
}

// TestBroadcastToUserStreams_DoesNotRaceWithUnregister exercises the close race
// directly: a handler returning (unregistering its channel) concurrently with a
// broadcast sending to that same channel. Run under -race this is what catches a
// future "tidy up by closing the channel on unregister" change.
func TestBroadcastToUserStreams_DoesNotRaceWithUnregister(t *testing.T) {
	testStorage := setupTestStorage(t)
	svc := setupTestService(t, testStorage)

	const iterations = 200
	for range iterations {
		ch := make(chan *api.StreamUserEventsResponse, 10)
		svc.registerUserStream(context.Background(), "user-1", ch)

		done := make(chan struct{})
		go func() {
			svc.unregisterUserStream(context.Background(), "user-1", ch)
			close(done)
		}()

		item := &api.CommunityEventItem{
			Id:          "event-1",
			CommunityId: "community-1",
			EventType:   api.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		}
		for range 10 {
			svc.broadcastToUserStreams(context.Background(), []string{"user-1"}, item)
		}

		<-done
	}
}

// TestBroadcastToUserStreams_ClonesMessage proves recipients do not share one
// mutable message. Without the proto.Clone, one client's handler mutating the
// item it received would corrupt what every other recipient sees.
func TestBroadcastToUserStreams_ClonesMessage(t *testing.T) {
	testStorage := setupTestStorage(t)
	svc := setupTestService(t, testStorage)

	ch1 := make(chan *api.StreamUserEventsResponse, 10)
	ch2 := make(chan *api.StreamUserEventsResponse, 10)
	svc.registerUserStream(context.Background(), "user-1", ch1)
	svc.registerUserStream(context.Background(), "user-2", ch2)

	svc.broadcastToUserStreams(context.Background(), []string{"user-1", "user-2"}, &api.CommunityEventItem{
		Id:          "event-1",
		CommunityId: "community-1",
		EventType:   api.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		GearName:    "Original Name",
	})

	msg1 := <-ch1
	msg2 := <-ch2

	msg1.GetEvent().GearName = "Mutated"
	if msg2.GetEvent().GearName != "Original Name" {
		t.Error("broadcast should clone messages — mutation to one recipient affected the other")
	}
}

// TestUserStream_OneConnectionCoversEveryCommunity is the point of #2867: the
// number of connections a client needs is 1, not one per community. Events from
// unrelated communities all land on the same channel.
func TestUserStream_OneConnectionCoversEveryCommunity(t *testing.T) {
	testStorage := setupTestStorage(t)
	svc := setupTestService(t, testStorage)

	ch := make(chan *api.StreamUserEventsResponse, 10)
	svc.registerUserStream(context.Background(), "user-1", ch)

	for _, communityID := range []string{"community-1", "community-2", "community-3"} {
		svc.broadcastToUserStreams(context.Background(), []string{"user-1"}, &api.CommunityEventItem{
			Id:          "event-" + communityID,
			CommunityId: communityID,
			EventType:   api.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		})
	}

	if len(ch) != 3 {
		t.Fatalf("expected 3 events on the single stream, got %d", len(ch))
	}
	seen := map[string]bool{}
	for range 3 {
		seen[(<-ch).GetEvent().CommunityId] = true
	}
	for _, want := range []string{"community-1", "community-2", "community-3"} {
		if !seen[want] {
			t.Errorf("expected an event for %s on the single stream", want)
		}
	}
}

func TestBroadcastToUserStreams_OnlyReachesRecipients(t *testing.T) {
	testStorage := setupTestStorage(t)
	svc := setupTestService(t, testStorage)

	memberCh := make(chan *api.StreamUserEventsResponse, 10)
	strangerCh := make(chan *api.StreamUserEventsResponse, 10)
	svc.registerUserStream(context.Background(), "member", memberCh)
	svc.registerUserStream(context.Background(), "stranger", strangerCh)

	svc.broadcastToUserStreams(context.Background(), []string{"member"}, &api.CommunityEventItem{
		Id:          "event-1",
		CommunityId: "community-1",
		EventType:   api.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
	})

	if len(memberCh) != 1 {
		t.Errorf("expected the member to receive 1 event, got %d", len(memberCh))
	}
	if len(strangerCh) != 0 {
		t.Errorf("expected a non-recipient to receive nothing, got %d", len(strangerCh))
	}
}

// TestBusPublish_ReachesUserStreamsOfLiveMembers covers the property that makes
// the per-community subscribe call unnecessary: membership is resolved from
// storage at event time, so a member who never announced any community still
// receives that community's events.
func TestBusPublish_ReachesUserStreamsOfLiveMembers(t *testing.T) {
	testStorage := setupTestStorage(t)
	svc, _ := newTestServiceWithSignal(t, testStorage, notifications.NewMockService())

	aliceID := setupTestUser(t, testStorage, "alice@test.com", "Alice")
	bobID := setupTestUser(t, testStorage, "bob@test.com", "Bob")
	ctxAlice := createAuthenticatedContext(aliceID, "alice@test.com", models.Role_ROLE_USER)

	createResp, err := svc.CreateCommunity(ctxAlice, connect.NewRequest(&api.CreateCommunityRequest{
		Name:        "Test Community",
		Description: "A test community",
	}))
	if err != nil {
		t.Fatalf("failed to create community: %v", err)
	}
	communityID := createResp.Msg.Id
	addUserToCommunity(t, svc, communityID, aliceID, bobID, "alice@test.com", "bob@test.com")
	// Settle the join's own INVITATION_LINK_USED dispatch before registering,
	// so the count below is about the event this test publishes.
	drainBus(t, svc)

	// Bob's stream names no community — it is opened before this event and
	// carries no subscription list.
	bobCh := make(chan *api.StreamUserEventsResponse, 10)
	svc.registerUserStream(context.Background(), bobID, bobCh)

	if _, err := busFor(t, svc).Publish(ctxAlice, &models.CommunityEvent{
		CommunityId: communityID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:     aliceID,
	}); err != nil {
		t.Fatalf("bus.Publish failed: %v", err)
	}
	drainBus(t, svc)

	if len(bobCh) != 1 {
		t.Fatalf("expected Bob's user stream to receive 1 event, got %d", len(bobCh))
	}
	if got := (<-bobCh).GetEvent().CommunityId; got != communityID {
		t.Errorf("expected event for community %s, got %s", communityID, got)
	}
}

// TestBusPublish_SkipsUserStreamsOfNonMembers pins the membership filter. A
// connected user who does not belong to the community must not see its events —
// the stream carries no community, so this filter is the only thing standing
// between one user's connection and every other community's traffic.
func TestBusPublish_SkipsUserStreamsOfNonMembers(t *testing.T) {
	testStorage := setupTestStorage(t)
	svc, _ := newTestServiceWithSignal(t, testStorage, notifications.NewMockService())

	aliceID := setupTestUser(t, testStorage, "alice@test.com", "Alice")
	strangerID := setupTestUser(t, testStorage, "stranger@test.com", "Stranger")
	ctxAlice := createAuthenticatedContext(aliceID, "alice@test.com", models.Role_ROLE_USER)

	createResp, err := svc.CreateCommunity(ctxAlice, connect.NewRequest(&api.CreateCommunityRequest{
		Name:        "Private Community",
		Description: "Stranger is not a member",
	}))
	if err != nil {
		t.Fatalf("failed to create community: %v", err)
	}

	strangerCh := make(chan *api.StreamUserEventsResponse, 10)
	svc.registerUserStream(context.Background(), strangerID, strangerCh)

	if _, err := busFor(t, svc).Publish(ctxAlice, &models.CommunityEvent{
		CommunityId: createResp.Msg.Id,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:     aliceID,
	}); err != nil {
		t.Fatalf("bus.Publish failed: %v", err)
	}
	drainBus(t, svc)

	if len(strangerCh) != 0 {
		t.Errorf("expected a non-member's stream to receive nothing, got %d events", len(strangerCh))
	}
}

// TestPushSuppression_SkipsUsersWithUserStream: a user reachable over the
// per-user stream must not also be pushed. The suppression hook is keyed by
// community, but a per-user stream covers every community, so it has to answer
// for a community it was never told about.
func TestPushSuppression_SkipsUsersWithUserStream(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockNotif := notifications.NewMockService()
	svc, _ := newTestServiceWithSignal(t, testStorage, mockNotif)

	aliceID := setupTestUser(t, testStorage, "alice@test.com", "Alice")
	bobID := setupTestUser(t, testStorage, "bob@test.com", "Bob")

	gearID, err := testStorage.Insert(context.Background(), &models.Gear{
		Name:    "Test Gear",
		OwnerId: bobID,
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	})
	if err != nil {
		t.Fatalf("failed to create gear: %v", err)
	}

	// Bob has a per-user stream and no per-community stream at all.
	bobCh := make(chan *api.StreamUserEventsResponse, 10)
	svc.registerUserStream(context.Background(), bobID, bobCh)

	if _, err := busFor(t, svc).Publish(
		createAuthenticatedContext(aliceID, "alice@test.com", models.Role_ROLE_USER),
		&models.CommunityEvent{
			CommunityId: "community-1",
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
			ActorId:     aliceID,
			GearId:      gearID,
		},
	); err != nil {
		t.Fatalf("bus.Publish failed: %v", err)
	}
	drainBus(t, svc)

	if len(mockNotif.GetCalls()) > 0 {
		t.Errorf("expected push suppressed for a user with a per-user stream, got %d notifications",
			len(mockNotif.GetCalls()))
	}
}

// TestRunUserStreamLoop_CommunityDeletedIsNotTerminal is the behavior that had
// to change: on a per-community stream COMMUNITY_DELETED ended the stream,
// because nothing else would ever arrive on it. Here it is one community among
// many, so ending would silently drop realtime for every other community the
// user belongs to.
func TestRunUserStreamLoop_CommunityDeletedIsNotTerminal(t *testing.T) {
	testStorage := setupTestStorage(t)
	svc := setupTestService(t, testStorage)

	eventChan := make(chan *api.StreamUserEventsResponse, 10)
	eventChan <- &api.StreamUserEventsResponse{
		Payload: &api.StreamUserEventsResponse_Event{Event: &api.CommunityEventItem{
			Id:          "deleted-event",
			CommunityId: "community-1",
			EventType:   api.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED,
		}},
	}
	eventChan <- &api.StreamUserEventsResponse{
		Payload: &api.StreamUserEventsResponse_Event{Event: &api.CommunityEventItem{
			Id:          "later-event",
			CommunityId: "community-2",
			EventType:   api.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		}},
	}

	// A short lifetime cap is what ends the loop; if COMMUNITY_DELETED were
	// still terminal the loop would return after ONE event instead.
	svc.streamLifetimeOverride = 150 * time.Millisecond
	svc.streamHeartbeatOverride = 10 * time.Second // never fires during the test

	sender := &fakeUserSender{}
	if err := svc.runUserStreamLoop(
		context.Background(), sender, eventChan, map[string]bool{},
		logging.Default(), time.Now(),
	); err != nil {
		t.Fatalf("runUserStreamLoop returned an error: %v", err)
	}

	if got := sender.event.Load(); got != 2 {
		t.Errorf("expected 2 events delivered (the delete must not terminate the stream), got %d", got)
	}
}

// TestRunUserStreamLoop_DeadPeerReapedOnHeartbeat is the core guarantee against
// zombie streams: a silently disconnected client (Send errors) is detected on
// the very next heartbeat tick, not after the full lifetime cap. With one
// connection per user rather than per community this matters more, not less —
// a leaked stream now holds a whole client's realtime path.
func TestRunUserStreamLoop_DeadPeerReapedOnHeartbeat(t *testing.T) {
	testStorage := setupTestStorage(t)
	svc := setupTestService(t, testStorage)
	// Heartbeat far faster than the lifetime, so exiting proves the heartbeat
	// reaped the stream rather than the cap.
	svc.streamHeartbeatOverride = 20 * time.Millisecond
	svc.streamLifetimeOverride = 10 * time.Second

	sender := &fakeUserSender{failOn: 1} // fail on the first heartbeat send

	start := time.Now()
	err := svc.runUserStreamLoop(
		context.Background(),
		sender,
		make(chan *api.StreamUserEventsResponse, 10),
		map[string]bool{},
		logging.Default(),
		start,
	)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from dead-peer Send, got nil")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("expected dead peer to be reaped within ~heartbeat interval, took %v", elapsed)
	}
	if got := sender.heartbeat.Load(); got != 1 {
		t.Errorf("expected exactly 1 heartbeat send attempt, got %d", got)
	}
}

// TestRunUserStreamLoop_ExitsAtLifetimeCap verifies a healthy stream still ends
// cleanly at the cap, so Cloud Run's 900s frontend never sees the request and
// the client gets a retryable EOF instead of a 504.
func TestRunUserStreamLoop_ExitsAtLifetimeCap(t *testing.T) {
	testStorage := setupTestStorage(t)
	svc := setupTestService(t, testStorage)
	svc.streamLifetimeOverride = 100 * time.Millisecond
	svc.streamHeartbeatOverride = 10 * time.Second // never fires during the test

	start := time.Now()
	err := svc.runUserStreamLoop(
		context.Background(),
		&fakeUserSender{}, // all sends succeed
		make(chan *api.StreamUserEventsResponse, 10),
		map[string]bool{},
		logging.Default(),
		start,
	)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected clean exit at lifetime cap, got error: %v", err)
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("exited before the lifetime cap, took %v", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("lifetime cap took too long to fire, took %v", elapsed)
	}
}

// TestRunUserStreamLoop_HealthyStreamKeepsSendingHeartbeats proves the liveness
// signal actually fires on an idle stream rather than merely being configured —
// without it, dead-peer detection above would never get a chance to run.
func TestRunUserStreamLoop_HealthyStreamKeepsSendingHeartbeats(t *testing.T) {
	testStorage := setupTestStorage(t)
	svc := setupTestService(t, testStorage)
	svc.streamHeartbeatOverride = 20 * time.Millisecond
	svc.streamLifetimeOverride = 150 * time.Millisecond

	sender := &fakeUserSender{}
	err := svc.runUserStreamLoop(
		context.Background(),
		sender,
		make(chan *api.StreamUserEventsResponse, 10),
		map[string]bool{},
		logging.Default(),
		time.Now(),
	)
	if err != nil {
		t.Fatalf("expected clean exit, got %v", err)
	}
	// 150ms at a 20ms tick is ~7; assert a conservative floor to avoid flakes.
	if got := sender.heartbeat.Load(); got < 4 {
		t.Errorf("expected at least 4 heartbeats, got %d", got)
	}
}

// TestStreamUserEvents_UnregistersSubscriberAfterDeadPeer completes the
// lifecycle the two tests above only cover halfway: the loop exiting is not
// enough, the registry entry has to go too. A leaked entry would make
// HasActiveUserStream keep suppressing push for a user who is no longer
// connected — they would stop receiving anything at all.
func TestStreamUserEvents_UnregistersSubscriberAfterDeadPeer(t *testing.T) {
	testStorage := setupTestStorage(t)
	svc := setupTestService(t, testStorage)
	svc.streamHeartbeatOverride = 20 * time.Millisecond
	svc.streamLifetimeOverride = 5 * time.Second

	const userID = "user-dead-peer"
	ch := make(chan *api.StreamUserEventsResponse, 10)
	svc.registerUserStream(context.Background(), userID, ch)

	if !svc.HasActiveUserStream(userID) {
		t.Fatal("expected subscriber registered before the loop runs")
	}

	err := svc.runUserStreamLoop(
		context.Background(),
		&fakeUserSender{failOn: 1},
		ch,
		map[string]bool{},
		logging.Default(),
		time.Now(),
	)
	if err == nil {
		t.Fatal("expected dead-peer error from runUserStreamLoop")
	}

	// Mirror the handler's defer.
	svc.unregisterUserStream(context.Background(), userID, ch)

	if svc.HasActiveUserStream(userID) {
		t.Error("dead subscriber was not unregistered after loop exit")
	}
	if svc.hasAnyUserStream() {
		t.Error("registry should be empty after the only subscriber was unregistered")
	}

	// A later broadcast must not find the subscriber and must not panic.
	svc.broadcastToUserStreams(context.Background(), []string{userID}, &api.CommunityEventItem{
		Id:          "event-after-dead-peer",
		CommunityId: "community-1",
		EventType:   api.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
	})
}

// TestPushSuppression_SendsToUsersWithoutUserStream is the other half of
// TestPushSuppression_SkipsUsersWithUserStream. Without it, a suppression bug
// that skipped *everyone* would still pass the suppression test — the negative
// case is what proves the check discriminates rather than always returning true.
func TestPushSuppression_SendsToUsersWithoutUserStream(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockNotif := notifications.NewMockService()
	svc, _ := newTestServiceWithSignal(t, testStorage, mockNotif)

	aliceID := setupTestUser(t, testStorage, "alice@test.com", "Alice")
	bobID := setupTestUser(t, testStorage, "bob@test.com", "Bob")

	gearID, err := testStorage.Insert(context.Background(), &models.Gear{
		Name:    "Test Gear",
		OwnerId: bobID,
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	})
	if err != nil {
		t.Fatalf("failed to create gear: %v", err)
	}

	// Bob has NO stream of any kind — push must go through.
	if _, err := busFor(t, svc).Publish(
		createAuthenticatedContext(aliceID, "alice@test.com", models.Role_ROLE_USER),
		&models.CommunityEvent{
			CommunityId: "community-1",
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
			ActorId:     aliceID,
			GearId:      gearID,
		},
	); err != nil {
		t.Fatalf("bus.Publish failed: %v", err)
	}
	drainBus(t, svc)

	if got := len(mockNotif.GetCalls()); got != 1 {
		t.Errorf("expected 1 push notification for a user with no stream, got %d", got)
	}
}
