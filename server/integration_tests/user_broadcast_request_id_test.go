package integration_tests

// Regression test parallel to chat_broadcast_request_id_test.go but for the
// community event fan-out. Before, the broadcaster used logging.Default() and
// its fan-out lines carried no request_id at all; after threading ctx, the
// originating RPC's request_id reaches the broadcast log.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/storage"
)

// createAuthCommunityClientWithRequestID returns a community client that
// stamps the given X-Request-ID on every outbound request.
func createAuthCommunityClientWithRequestID(token, serverURL, requestID string) apiconnect.CommunityServiceClient {
	return apiconnect.NewCommunityServiceClient(
		&http.Client{
			Transport: &requestIDTransport{
				requestID: requestID,
				base:      &authTransport{token: token, base: http.DefaultTransport},
			},
		},
		serverURL,
	)
}

// TestUserBroadcast_PropagatesActorRequestID asserts that when one member
// triggers a community event (here: ShareGear, which produces a GEAR_SHARED
// event) and another member has an open StreamUserEvents connection, the
// broadcast log line carries the *actor's* RPC request_id and the
// community_event_id of the broadcast event.
//
// This is the only test that exercises request_id propagation through the real
// bus -> fan-out -> Connect stream path; the unit tests in
// services/community use fake senders and never build a transport.
func TestUserBroadcast_PropagatesActorRequestID(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// Two users in one community: actor (who shares gear) and observer (who
	// holds the open stream). The fan-out does deliver to the actor too
	// (#2702), but keeping them distinct is what makes the assertion about
	// cross-user delivery rather than self-delivery.
	actorToken, _ := registerFirstUser(t, serverURL, "actor@example.com", "Actor")
	actorGear := createAuthGearClient(actorToken, serverURL)
	actorCommunity := createAuthCommunityClient(actorToken, serverURL)

	communityResp, err := actorCommunity.CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{
		Name:        "Broadcast Test Community",
		Description: "Verifies community event broadcasts carry the actor's request_id.",
	}))
	if err != nil {
		t.Fatalf("CreateCommunity: %v", err)
	}
	communityID := communityResp.Msg.Id

	observerToken, _ := registerUserByInvite(t, serverURL, actorToken, communityID, "observer@example.com", "Observer")
	observerCommunity := createAuthCommunityClient(observerToken, serverURL)

	gearResp, err := actorGear.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
		Name:        proto.String("Broadcast Test Gear"),
		Description: proto.String("for community broadcast correlation test"),
	}))
	if err != nil {
		t.Fatalf("SaveGear: %v", err)
	}
	gearID := gearResp.Msg.Id

	// Observer opens their single per-user stream. The subscription names no
	// community — membership is resolved server-side per event — so the
	// observer being a member of communityID is what makes the event reach
	// them. A non-member observer receives nothing and this test would hang
	// to the context deadline rather than fail readably.
	//
	// since_unix_sec=0 puts the handler in catch-up mode so it immediately
	// Sends the existing JOINED / community-creation events. Without an
	// initial Send the connect client blocks on Receive until the first
	// heartbeat (30s default), making the test slow and flaky; the catch-up
	// Send also flushes response headers so the call returns promptly. The
	// server clamps 0 up to seven days ago, so this asks for "recent", not
	// "everything since the epoch".
	streamCtx, streamCancel := context.WithTimeout(ctx, 30*time.Second)
	defer streamCancel()

	since := int64(0)
	stream, err := observerCommunity.StreamUserEvents(streamCtx, connect.NewRequest(&api.StreamUserEventsRequest{
		SinceUnixSec: &since,
	}))
	if err != nil {
		t.Fatalf("StreamUserEvents: %v", err)
	}
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		for stream.Receive() {
			// Drain — we just need the subscription to exist server-side.
		}
	}()
	defer streamCancel()

	// The per-user register line carries user_id, not community_id — the
	// registry is keyed by user and the subscription names no community.
	if entry := waitForLogEntry(logCapture, func(e map[string]any) bool {
		return e["message"] == "registered user event stream"
	}, 5*time.Second); entry == nil {
		t.Fatal("observer user event stream did not register in time")
	}

	logCapture.Clear()

	// Actor shares gear under a known request_id. ShareGear records a
	// GEAR_SHARED community event, which fans out through
	// broadcastToUserStreams to the observer's per-user stream.
	const shareRequestID = "user-broadcast-rpc-id-001"
	actorCommunityWithID := createAuthCommunityClientWithRequestID(actorToken, serverURL, shareRequestID)

	shareGearIntoCommunity(t, ctx, actorCommunityWithID, gearID, communityID, api.Availability_AVAILABILITY_FOR_LOAN)

	// The broadcast log line must carry the ShareGear RPC's request_id
	// and identify the event by community_event_id and event_type.
	const broadcastMessage = "broadcasting event to user streams"
	entry := waitForLogEntry(logCapture, func(e map[string]any) bool {
		return e["message"] == broadcastMessage &&
			e["request_id"] == shareRequestID &&
			e["event_type"] == "COMMUNITY_EVENT_TYPE_GEAR_SHARED"
	}, 2*time.Second)

	if entry == nil {
		t.Errorf("no %q log line found with request_id=%q event_type=GEAR_SHARED",
			broadcastMessage, shareRequestID)
		t.Logf("entries with request_id=%q: %v", shareRequestID, logCapture.FindByRequestID(shareRequestID))
		t.Logf("%q entries: %v", broadcastMessage, logCapture.FindByMessage(broadcastMessage))
		return
	}

	if eventID, _ := entry["community_event_id"].(string); eventID == "" {
		t.Errorf("broadcast log entry missing community_event_id: %v", entry)
	}
}
