package main

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
)

// TestEndToEnd_CompleteRequestLifecycle tests complete request lifecycle.
func TestEndToEnd_CompleteRequestLifecycle(t *testing.T) {
	serverURL := getTestServerURL(t)
	t.Logf("Testing against server: %s", serverURL)

	simID := generateSimulationID("request")
	registerSimulationCleanup(t, serverURL, simID)

	ctx := context.Background()

	// Create two test users: requester and offerer
	requesterToken, requesterUserID := createUniqueTestUser(t, serverURL, "requester", simID)

	offererToken, _ := createUniqueTestUser(t, serverURL, "offerer", simID)

	// Create authenticated clients
	requesterCommunityClient := apiconnect.NewCommunityServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: requesterToken,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	requesterRequestClient := apiconnect.NewRequestServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: requesterToken,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	offererCommunityClient := apiconnect.NewCommunityServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: offererToken,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	offererRequestClient := apiconnect.NewRequestServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: offererToken,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	// Step 1: Requester creates a community
	t.Log("Step 1: Requester creating community...")
	communityResp, err := requesterCommunityClient.CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{
		Name:         "E2E Test Request Community",
		Description:  "Community for testing request lifecycle",
		SimulationId: proto.String(simID),
	}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	communityID := communityResp.Msg.Id
	t.Logf("Created community: %s", communityID)

	// Step 2: Requester creates invitation link
	t.Log("Step 2: Requester creating invitation link...")
	inviteLinkResp, err := requesterCommunityClient.GetOrCreateShareLink(ctx, connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
		Target:      &api.GetOrCreateShareLinkRequest_CommunityInvite{CommunityInvite: communityID},
	}))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink failed: %v", err)
	}

	// Step 3: Offerer accepts invitation link
	t.Log("Step 3: Offerer accepting invitation link...")
	_, err = offererCommunityClient.AcceptInvitationLink(ctx, connect.NewRequest(&api.AcceptInvitationLinkRequest{
		ShortCode: inviteLinkResp.Msg.ShortCode,
	}))
	if err != nil {
		t.Fatalf("AcceptInvitationLink failed: %v", err)
	}

	// Step 4: Requester submits a request
	t.Log("Step 4: Requester submitting request...")
	submitResp, err := requesterRequestClient.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Need a Ladder",
		Description: "Looking for a ladder to borrow for home repairs",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	if _, err := requesterCommunityClient.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:                &api.ShareItemRequest_RequestId{RequestId: submitResp.Msg.RequestId},
		ShareToCommunityIds: []string{communityID},
	})); err != nil {
		t.Fatalf("ShareItem failed: %v", err)
	}
	requestID := submitResp.Msg.RequestId
	t.Logf("Created request: %s", requestID)

	// Step 5: Verify request appears in community feed
	t.Log("Step 5: Verifying request appears in community feed...")
	listResp, err := offererRequestClient.ListRequests(ctx, connect.NewRequest(&api.ListRequestsRequest{
		CommunityId: communityID,
		State:       api.RequestState_REQUEST_STATE_ACTIVE,
	}))
	if err != nil {
		t.Fatalf("ListRequests failed: %v", err)
	}
	if len(listResp.Msg.Requests) == 0 {
		t.Fatal("Expected at least one active request in community")
	}
	foundRequest := false
	for _, req := range listResp.Msg.Requests {
		if req.Id == requestID {
			foundRequest = true
			if req.Description != "Looking for a ladder to borrow for home repairs" {
				t.Errorf("Expected description 'Looking for a ladder to borrow for home repairs', got %s", req.Description)
			}
			if req.State != api.RequestState_REQUEST_STATE_ACTIVE {
				t.Errorf("Expected state ACTIVE, got %v", req.State)
			}
		}
	}
	if !foundRequest {
		t.Fatal("Request not found in community feed")
	}

	// Step 6: Offerer offers to fulfill request
	t.Log("Step 6: Offerer offering to fulfill request...")
	offerResp, err := offererRequestClient.OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill failed: %v", err)
	}
	if offerResp.Msg.Request.GetConversationId() == "" {
		t.Fatal("Expected conversation ID after offering to fulfill")
	}
	t.Logf("Created conversation: %s", offerResp.Msg.Request.GetConversationId())

	// Step 7: Verify request state changed to OFFERS_RECEIVED
	t.Log("Step 7: Verifying request state changed to OFFERS_RECEIVED...")
	getResp, err := requesterRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
		RequestId: requestID,
	}))
	if err != nil {
		t.Fatalf("GetRequest failed: %v", err)
	}
	if getResp.Msg.Request.State != api.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
		t.Errorf("Expected state OFFERS_RECEIVED, got %v", getResp.Msg.Request.State)
	}
	if len(getResp.Msg.Request.Offerers) == 0 {
		t.Error("Expected at least one offerer")
	}

	// Step 8: Verify offerer cannot mark request as fulfilled (only requester can)
	t.Log("Step 8: Verifying offerer cannot mark request as fulfilled...")
	_, err = offererRequestClient.MarkRequestFulfilled(ctx, connect.NewRequest(&api.MarkRequestFulfilledRequest{
		RequestId: requestID,
	}))
	if err == nil {
		t.Fatal("Expected error when offerer tries to mark request fulfilled, but got none")
	}
	t.Logf("Correctly rejected offerer's fulfillment attempt: %v", err)

	// Step 9: Requester marks request as fulfilled
	t.Log("Step 9: Requester marking request as fulfilled...")
	_, err = requesterRequestClient.MarkRequestFulfilled(ctx, connect.NewRequest(&api.MarkRequestFulfilledRequest{
		RequestId: requestID,
	}))
	if err != nil {
		t.Fatalf("MarkRequestFulfilled failed: %v", err)
	}

	// Step 10: Verify request no longer appears in active feed
	t.Log("Step 10: Verifying request no longer appears in active feed...")
	listResp2, err := offererRequestClient.ListRequests(ctx, connect.NewRequest(&api.ListRequestsRequest{
		CommunityId: communityID,
		State:       api.RequestState_REQUEST_STATE_ACTIVE,
	}))
	if err != nil {
		t.Fatalf("ListRequests failed: %v", err)
	}
	for _, req := range listResp2.Msg.Requests {
		if req.Id == requestID {
			t.Error("Fulfilled request should not appear in active feed")
		}
	}

	// Step 11: Verify request appears in fulfilled state when explicitly queried
	t.Log("Step 11: Verifying request appears in fulfilled state...")
	getFinalResp, err := requesterRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
		RequestId: requestID,
	}))
	if err != nil {
		t.Fatalf("GetRequest failed: %v", err)
	}
	if getFinalResp.Msg.Request.State != api.RequestState_REQUEST_STATE_FULFILLED {
		t.Errorf("Expected state FULFILLED, got %v", getFinalResp.Msg.Request.State)
	}

	t.Log("✓ Complete request lifecycle test passed!")

	// Off-app simulation suppression (#2588). The requester is a deviceless
	// simulation user, so the offer-to-fulfill notification routes to the off-app
	// email channel and must be suppressed there — before any (mock) provider
	// send — which is what stops E2E and load-test traffic from burning the
	// shared Mailgun daily quota and the metered A2P SMS path. Asserted only in
	// local mode, where the server's structured logs are captured (a deployed
	// server's process logs aren't reachable from the test).
	if os.Getenv("E2E_SERVER_URL") == "" {
		t.Log("Verifying the requester's off-app email notification reached the email channel and was suppressed as a simulation recipient (not sent)...")
		// Positive: the notification reached the email channel and was suppressed.
		// Dispatch is async (background goroutine), so poll.
		if !waitForServerLog(20*time.Second, map[string]string{
			"outcome": "suppressed_simulation",
			"channel": "email",
			"user_id": requesterUserID,
		}) {
			t.Errorf("expected an off-app email suppressed_simulation log for the requester (user_id=%s, simulation_id=%s); none appeared", requesterUserID, simID)
		}
		// Negative: it must never have actually been sent. The off-app email send
		// has its own distinct message; outcome="sent" alone would also match the
		// "notification dispatched to user" wrapper, so match on the message.
		if n := countServerLog(map[string]string{
			"message": "off-app email sent to deviceless recipient",
			"user_id": requesterUserID,
		}); n != 0 {
			t.Errorf("off-app email must not be sent to simulation recipient %s, but %d send(s) were logged", requesterUserID, n)
		}
	}
}
