package integration_tests

// Integration tests for request workflows.
// Tests are mapped 1:1 to workflow examples in docs/workflows/request.md.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// TestRequest_Example1_RequestFulfilled tests the happy path request fulfillment workflow.
// Covers: docs/workflows/request.md - Workflow Example 1.
func TestRequest_Example1_RequestFulfilled(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// ========== PRECONDITIONS ==========
	// - A test community exists
	// - Four users exist: a requester, helper A, helper B, and helper C, all members of the community

	requesterToken, requesterID := registerFirstUser(t, serverURL, "requester@example.com", "Requester")
	requesterCommunityClient := createAuthCommunityClient(requesterToken, serverURL)
	requesterLocationClient := createAuthLocationClient(requesterToken, serverURL)
	requesterRequestClient := createAuthRequestClient(requesterToken, serverURL)
	requesterChatClient := createAuthChatClient(requesterToken, serverURL)

	communityID := setupTestCommunity(t, ctx, requesterCommunityClient, "Request Test Community", "Testing request workflows")
	locationID := setupTestLocation(t, ctx, requesterLocationClient, "Portland")

	helperAToken, helperAID := registerUserByInvite(t, serverURL, requesterToken, communityID, "helper-a@example.com", "Helper A")
	helperARequestClient := createAuthRequestClient(helperAToken, serverURL)

	helperBToken, helperBID := registerUserByInvite(t, serverURL, requesterToken, communityID, "helper-b@example.com", "Helper B")
	helperBRequestClient := createAuthRequestClient(helperBToken, serverURL)

	helperCToken, helperCID := registerUserByInvite(t, serverURL, requesterToken, communityID, "helper-c@example.com", "Helper C")
	helperCRequestClient := createAuthRequestClient(helperCToken, serverURL)

	var requestID string
	var conversationID string

	// ========== STEPS ==========

	// Step 1: Requester creates request
	t.Run("Step1_RequesterCreatesRequest", func(t *testing.T) {
		resp, err := requesterRequestClient.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
			Title:       "Need a Ladder",
			Description: "Looking to borrow a ladder for painting this weekend",
			LocationId:  locationID,
		}))
		if err != nil {
			t.Fatalf("SubmitRequest failed: %v", err)
		}
		shareRequestIntoCommunity(t, ctx, requesterCommunityClient, resp.Msg.RequestId, communityID)
		requestID = resp.Msg.RequestId

		// Verify initial state
		getResp, err := requesterRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
			RequestId: requestID,
		}))
		if err != nil {
			t.Fatalf("GetRequest failed: %v", err)
		}
		if getResp.Msg.Request.State != api.RequestState_REQUEST_STATE_ACTIVE {
			t.Errorf("Expected ACTIVE state, got %v", getResp.Msg.Request.State)
		}
	})

	// Step 3: Helper A offers to help
	t.Run("Step3_HelperAOffersHelp", func(t *testing.T) {
		resp, err := helperARequestClient.OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
			RequestId:   requestID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("OfferToFulfill failed: %v", err)
		}
		conversationID = resp.Msg.Request.GetConversationId()
		if resp.Msg.Request.State != api.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
			t.Errorf("Expected OFFERS_RECEIVED state, got %v", resp.Msg.Request.State)
		}
		if conversationID == "" {
			t.Error("Expected conversation to be created")
		}
	})

	// Step 5: Helper B also offers to help
	t.Run("Step5_HelperBOffersHelp", func(t *testing.T) {
		resp, err := helperBRequestClient.OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
			RequestId:   requestID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("OfferToFulfill failed: %v", err)
		}
		// Should still be in OFFERS_RECEIVED state
		if resp.Msg.Request.State != api.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
			t.Errorf("Expected OFFERS_RECEIVED state, got %v", resp.Msg.Request.State)
		}
	})

	// Step 7: Helper C also offers to help
	t.Run("Step7_HelperCOffersHelp", func(t *testing.T) {
		resp, err := helperCRequestClient.OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
			RequestId:   requestID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("OfferToFulfill failed: %v", err)
		}
		if resp.Msg.Request.State != api.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
			t.Errorf("Expected OFFERS_RECEIVED state, got %v", resp.Msg.Request.State)
		}
	})

	// Step 9: Requester coordinates with helpers in group chat (just verify conversation is accessible)
	t.Run("Step9_RequesterCanAccessChat", func(t *testing.T) {
		_, err := requesterChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationID,
		}))
		if err != nil {
			t.Fatalf("GetConversation failed: %v", err)
		}
	})

	// Step 10: Requester marks request as fulfilled
	var fulfillImpact *api.ImpactEstimate
	t.Run("Step10_RequesterMarksFulfilled", func(t *testing.T) {
		resp, err := requesterRequestClient.MarkRequestFulfilled(ctx, connect.NewRequest(&api.MarkRequestFulfilledRequest{
			RequestId: requestID,
		}))
		if err != nil {
			t.Fatalf("MarkRequestFulfilled failed: %v", err)
		}
		fulfillImpact = resp.Msg.Impact
	})

	// ========== POSTCONDITIONS ==========

	t.Run("Postcondition_RequestFulfilled", func(t *testing.T) {
		resp, err := requesterRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
			RequestId: requestID,
		}))
		if err != nil {
			t.Fatalf("GetRequest failed: %v", err)
		}
		if resp.Msg.Request.State != api.RequestState_REQUEST_STATE_FULFILLED {
			t.Errorf("Expected FULFILLED state, got %v", resp.Msg.Request.State)
		}
	})

	t.Run("Postcondition_NotInCommunityFeed", func(t *testing.T) {
		// When listing active requests, the fulfilled one should not appear
		listResp, err := requesterRequestClient.ListRequests(ctx, connect.NewRequest(&api.ListRequestsRequest{
			CommunityId: communityID,
			State:       api.RequestState_REQUEST_STATE_ACTIVE,
		}))
		if err != nil {
			t.Fatalf("ListRequests failed: %v", err)
		}
		for _, req := range listResp.Msg.Requests {
			if req.Id == requestID {
				t.Error("Fulfilled request should not appear in active requests list")
			}
		}
	})

	t.Run("Postcondition_VisibleInMyRequests", func(t *testing.T) {
		listResp, err := requesterRequestClient.ListMyRequests(ctx, connect.NewRequest(&api.ListMyRequestsRequest{}))
		if err != nil {
			t.Fatalf("ListMyRequests failed: %v", err)
		}
		var found bool
		for _, req := range listResp.Msg.Requests {
			if req.Id == requestID {
				found = true
				break
			}
		}
		if !found {
			t.Error("Request should still be visible in requester's My Requests")
		}
	})

	t.Run("Postcondition_ConversationAccessible", func(t *testing.T) {
		_, err := requesterChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationID,
		}))
		if err != nil {
			t.Fatalf("GetConversation should still be accessible: %v", err)
		}
	})

	// Per docs/workflows/request.md Example 1:
	// - REQUEST_CREATED broadcasts "New request" to each non-actor community
	//   member (3 helpers) when the request is created.
	// - REQUEST_OFFER_MADE notifies the requester for each of the 3 offers.
	// - REQUEST_FULFILLED does NOT notify (product decision, #2492): the
	//   offerers get no fulfillment push.
	// Total = 3 "New request" + 3 "Help offered" = 6 notifications.
	t.Run("Postcondition_RequesterNotifiedOnOffers", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 6, 5*time.Second)
		requesterNotifs := logCapture.GetNotificationLogsForUser(requesterID)
		// Requester only receives the 3 "Help offered" notifications.
		var helpOfferedCount int
		for _, n := range requesterNotifs {
			if n.Title == "Help offered" {
				helpOfferedCount++
			}
		}
		if helpOfferedCount != 3 {
			t.Errorf("Expected requester to receive 3 'Help Offered' notifications, got %d", helpOfferedCount)
			for _, n := range requesterNotifs {
				t.Logf("  Notification: title=%s", n.Title)
			}
		}
	})

	t.Run("Postcondition_HelpersNotifiedOnNewRequestOnly", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 6, 5*time.Second)
		// Each helper receives exactly one notification — "New request" at
		// creation. REQUEST_FULFILLED does not notify (#2492), so there is no
		// follow-up fulfillment push.
		for _, h := range []struct {
			label string
			id    string
		}{
			{"Helper A", helperAID},
			{"Helper B", helperBID},
			{"Helper C", helperCID},
		} {
			notifs := logCapture.GetNotificationLogsForUser(h.id)
			if len(notifs) != 1 {
				t.Errorf("Expected %s to receive 1 notification (new request only), got %d", h.label, len(notifs))
				for _, n := range notifs {
					t.Logf("  Notification: title=%s", n.Title)
				}
				continue
			}
			if notifs[0].Title != "New request" {
				t.Errorf("%s expected a 'New Request' notification, got %q", h.label, notifs[0].Title)
			}
		}
	})

	// Impact estimation postconditions (per docs/workflows/request.md Example 1)
	t.Run("Postcondition_RequestImpactEstimate", func(t *testing.T) {
		assertImpactPopulated(t, fulfillImpact, "MarkRequestFulfilled response")
		// Requests use flat defaults: emissions_prevented and time_saved populated
		if fulfillImpact.EmissionsPrevented != nil {
			if fulfillImpact.EmissionsPrevented.ManufactureAvoidedCarbon != nil {
				assertEstimatePositive(t, fulfillImpact.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams, "request emissions_prevented")
			}
		}
		if fulfillImpact.TimeSaved != nil {
			assertEstimatePositive(t, fulfillImpact.TimeSaved.Minutes, "request time_saved")
		} else {
			t.Error("time_saved is nil")
		}
	})

	t.Run("Postcondition_CommunityImpactMetrics", func(t *testing.T) {
		impactClient := createAuthImpactClient(requesterToken, serverURL)
		metrics := getCommunityImpactMetrics(t, ctx, impactClient, communityID)
		assertEstimatePositive(t, metrics.TimeBankedMinutes, "community time_banked_minutes after fulfilled request")
	})
}

// TestRequest_Example2_RequestCancelled tests the request cancellation workflow.
// Covers: docs/workflows/request.md - Workflow Example 2.
func TestRequest_Example2_RequestCancelled(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// ========== PRECONDITIONS ==========
	// - A test community exists
	// - Two users exist: a requester and a helper
	// - The requester has created a request
	// - The helper has offered to help (request is in OFFERS_RECEIVED state)

	requesterToken, _ := registerFirstUser(t, serverURL, "requester@example.com", "Requester")
	requesterCommunityClient := createAuthCommunityClient(requesterToken, serverURL)
	requesterLocationClient := createAuthLocationClient(requesterToken, serverURL)
	requesterRequestClient := createAuthRequestClient(requesterToken, serverURL)
	requesterChatClient := createAuthChatClient(requesterToken, serverURL)

	communityID := setupTestCommunity(t, ctx, requesterCommunityClient, "Request Test Community", "Testing request workflows")
	locationID := setupTestLocation(t, ctx, requesterLocationClient, "Seattle")

	helperToken, _ := registerUserByInvite(t, serverURL, requesterToken, communityID, "helper@example.com", "Helper")
	helperRequestClient := createAuthRequestClient(helperToken, serverURL)

	// Create request
	submitResp, err := requesterRequestClient.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Need a Drill",
		Description: "Looking for a power drill for a weekend project",
		LocationId:  locationID,
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestIntoCommunity(t, ctx, requesterCommunityClient, submitResp.Msg.RequestId, communityID)
	requestID := submitResp.Msg.RequestId

	// Helper offers to help
	offerResp, err := helperRequestClient.OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill failed: %v", err)
	}
	conversationID := offerResp.Msg.Request.GetConversationId()

	// ========== STEPS ==========

	// Step 1: Requester realizes they no longer need the item
	// Step 2: Requester cancels request
	t.Run("Step2_RequesterCancelsRequest", func(t *testing.T) {
		_, err := requesterRequestClient.CancelRequest(ctx, connect.NewRequest(&api.CancelRequestRequest{
			RequestId: requestID,
		}))
		if err != nil {
			t.Fatalf("CancelRequest failed: %v", err)
		}
	})

	// Step 3: System records community event (verified implicitly)

	// ========== POSTCONDITIONS ==========

	t.Run("Postcondition_RequestCancelled", func(t *testing.T) {
		resp, err := requesterRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
			RequestId: requestID,
		}))
		if err != nil {
			t.Fatalf("GetRequest failed: %v", err)
		}
		if resp.Msg.Request.State != api.RequestState_REQUEST_STATE_CANCELLED {
			t.Errorf("Expected CANCELLED state, got %v", resp.Msg.Request.State)
		}
	})

	t.Run("Postcondition_NotInCommunityFeed", func(t *testing.T) {
		// When listing active requests, the cancelled one should not appear
		listResp, err := requesterRequestClient.ListRequests(ctx, connect.NewRequest(&api.ListRequestsRequest{
			CommunityId: communityID,
			State:       api.RequestState_REQUEST_STATE_ACTIVE,
		}))
		if err != nil {
			t.Fatalf("ListRequests failed: %v", err)
		}
		for _, req := range listResp.Msg.Requests {
			if req.Id == requestID {
				t.Error("Cancelled request should not appear in active requests list")
			}
		}
	})

	t.Run("Postcondition_ConversationAccessible", func(t *testing.T) {
		_, err := requesterChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationID,
		}))
		if err != nil {
			t.Fatalf("GetConversation should still be accessible: %v", err)
		}
	})

	// Per docs/workflows/request.md Example 2:
	// - REQUEST_CREATED → 1 "New request" to the helper
	// - REQUEST_OFFER_MADE → 1 "Help offered" to the requester
	// - REQUEST_CANCELLED → 1 "Request cancelled" to the helper
	t.Run("Postcondition_CancellationNotifiesOfferers", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 3, 5*time.Second)
		notifLogs := logCapture.GetNotificationLogs()
		expectedCount := 3
		if len(notifLogs) != expectedCount {
			t.Errorf("Expected %d notifications (new request + offer + cancellation), got %d", expectedCount, len(notifLogs))
			for _, n := range notifLogs {
				t.Logf("  Notification: user=%s title=%s", n.UserID, n.Title)
			}
		}
		var cancelledCount int
		for _, n := range notifLogs {
			if n.Title == "Request cancelled" {
				cancelledCount++
			}
		}
		if cancelledCount != 1 {
			t.Errorf("Expected 1 'Request Cancelled' notification, got %d", cancelledCount)
		}
	})

	t.Run("Postcondition_ZeroImpact", func(t *testing.T) {
		impactClient := createAuthImpactClient(requesterToken, serverURL)
		metrics := getCommunityImpactMetrics(t, ctx, impactClient, communityID)
		assertCommunityImpactZero(t, metrics)
	})
}

// TestSubmitRequest_BornWithMultipleSeededNeeds is the full-server integration
// coverage for #2731: a request whose text plainly names several things is born
// with one claimable need per named thing, over the real RPC wire. The
// deterministic AI provider is not in play here — the client passes the names
// on SubmitRequest exactly as the create flow does after extraction — so this
// pins the RPC → service → storage → read-path contract independent of the
// model.
func TestSubmitRequest_BornWithMultipleSeededNeeds(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL := startTestServer(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	teacherToken, teacherID := registerFirstUser(t, serverURL, "teacher@example.com", "Teacher")
	requestClient := createAuthRequestClient(teacherToken, serverURL)

	seedNames := []string{"Picture books", "Whiteboard", "Storage bins", "Art supplies", "Construction paper"}
	submitResp, err := requestClient.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:         "Back-to-school supplies for Room 7",
		Description:   "We need picture books, a whiteboard, storage bins, art supplies, and construction paper.",
		SeedNeedNames: seedNames,
	}))
	if err != nil {
		t.Fatalf("SubmitRequest: %v", err)
	}
	requestID := submitResp.Msg.RequestId

	listResp, err := requestClient.ListRequestNeedsAndContributions(ctx,
		connect.NewRequest(&api.ListRequestNeedsAndContributionsRequest{RequestId: requestID}))
	if err != nil {
		t.Fatalf("ListRequestNeedsAndContributions: %v", err)
	}

	if len(listResp.Msg.Needs) != len(seedNames) {
		var got []string
		for _, n := range listResp.Msg.Needs {
			got = append(got, n.Name)
		}
		t.Fatalf("expected %d seeded needs, got %d (%v)", len(seedNames), len(listResp.Msg.Needs), got)
	}
	wantSet := map[string]bool{}
	for _, n := range seedNames {
		wantSet[n] = true
	}
	for _, need := range listResp.Msg.Needs {
		if !wantSet[need.Name] {
			t.Errorf("unexpected seeded need %q", need.Name)
		}
		if need.Slots != 1 || need.SlotsRemaining != 1 {
			t.Errorf("need %q: expected 1/1 slots, got %d/%d", need.Name, need.SlotsRemaining, need.Slots)
		}
		if need.GetProposer().GetId() != teacherID {
			t.Errorf("need %q: expected proposer %s, got %s", need.Name, teacherID, need.GetProposer().GetId())
		}
	}

	// The empty contract, over the same wire: a request that names nothing
	// concrete is born with no needs.
	emptyResp, err := requestClient.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Rough patch for our family",
		Description: "We could really use some community support right now — anything helps.",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest (empty): %v", err)
	}
	emptyList, err := requestClient.ListRequestNeedsAndContributions(ctx,
		connect.NewRequest(&api.ListRequestNeedsAndContributionsRequest{RequestId: emptyResp.Msg.RequestId}))
	if err != nil {
		t.Fatalf("ListRequestNeedsAndContributions (empty): %v", err)
	}
	if len(emptyList.Msg.Needs) != 0 {
		t.Errorf("expected 0 needs when nothing concrete was named, got %d", len(emptyList.Msg.Needs))
	}
}
