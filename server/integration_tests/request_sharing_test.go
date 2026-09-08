package integration_tests

// Integration tests for request cross-community sharing workflows.
// Tests are mapped 1:1 to workflow examples in docs/workflows/request.md.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// TestRequest_Example6_CrossCommunitySharing tests the cross-community sharing workflow.
// Covers: docs/workflows/request.md - Workflow Example 6.
func TestRequest_Example6_CrossCommunitySharing(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// ========== PRECONDITIONS ==========
	// - Two test communities exist (Community A and Community B)
	// - A requester is a member of both communities
	// - The requester has created a request in Community A
	// - Helper X is a member of Community A only
	// - Helper Y is a member of Community B only

	requesterToken, requesterID := registerFirstUser(t, serverURL, "requester@example.com", "Requester")
	requesterCommunityClient := createAuthCommunityClient(requesterToken, serverURL)
	requesterLocationClient := createAuthLocationClient(requesterToken, serverURL)
	requesterRequestClient := createAuthRequestClient(requesterToken, serverURL)

	communityAID := setupTestCommunity(t, ctx, requesterCommunityClient, "Community A", "First community")
	communityBID := setupTestCommunity(t, ctx, requesterCommunityClient, "Community B", "Second community")
	locationID := setupTestLocation(t, ctx, requesterLocationClient, "Portland")

	helperXToken, helperXID := registerUserByInvite(t, serverURL, requesterToken, communityAID, "helper-x@example.com", "Helper X")
	helperXRequestClient := createAuthRequestClient(helperXToken, serverURL)

	helperYToken, helperYID := registerUserByInvite(t, serverURL, requesterToken, communityBID, "helper-y@example.com", "Helper Y")
	helperYRequestClient := createAuthRequestClient(helperYToken, serverURL)

	var requestID string
	var conversationAID string
	var conversationBID string

	// ========== STEPS ==========

	// Step 1: Requester creates request in Community A
	t.Run("Step1_RequesterCreatesRequestInCommunityA", func(t *testing.T) {
		resp, err := requesterRequestClient.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
			Title:       "Need a Power Drill",
			Description: "Looking for a drill for weekend project",
			LocationId:  locationID,
		}))
		if err != nil {
			t.Fatalf("SubmitRequest failed: %v", err)
		}
		shareRequestIntoCommunity(t, ctx, requesterCommunityClient, resp.Msg.RequestId, communityAID)
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
		conversationAID = getResp.Msg.Request.GetConversationId()
	})

	// Step 2: System creates conversation for Community A (verified in step 1)

	// Step 3: Helper X offers to help in Community A
	t.Run("Step3_HelperXOffersHelp", func(t *testing.T) {
		resp, err := helperXRequestClient.OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
			RequestId:   requestID,
			CommunityId: communityAID,
		}))
		if err != nil {
			t.Fatalf("OfferToFulfill failed: %v", err)
		}
		if resp.Msg.Request.State != api.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
			t.Errorf("Expected OFFERS_RECEIVED state, got %v", resp.Msg.Request.State)
		}
	})

	// Step 4: System adds Helper X to Community A conversation (verified implicitly)

	// Step 5: Requester shares the same request with Community B
	t.Run("Step5_RequesterSharesWithCommunityB", func(t *testing.T) {
		shareRequestIntoCommunity(t, ctx, requesterCommunityClient, requestID, communityBID)

		// Request should still be in the same state. ShareItem returns the
		// audience it provisioned, not the item, so read the state back.
		getResp, err := requesterRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
			RequestId: requestID,
		}))
		if err != nil {
			t.Fatalf("GetRequest failed: %v", err)
		}
		if getResp.Msg.Request.State != api.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
			t.Errorf("Expected OFFERS_RECEIVED state after sharing, got %v", getResp.Msg.Request.State)
		}
	})

	// Step 6: System creates CommunityRequest junction for Community B, reusing the existing
	// conversation from Community A (single conversation per request architecture).
	// Verified by successful sharing

	// Step 7: Helper Y offers to help in Community B.
	// The new architecture stores one conversation per request (on Request.ConversationId),
	// so Community B reuses the same conversation as Community A.
	t.Run("Step7_HelperYOffersHelp", func(t *testing.T) {
		resp, err := helperYRequestClient.OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
			RequestId:   requestID,
			CommunityId: communityBID,
		}))
		if err != nil {
			t.Fatalf("OfferToFulfill failed: %v", err)
		}
		if resp.Msg.Request.State != api.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
			t.Errorf("Expected OFFERS_RECEIVED state, got %v", resp.Msg.Request.State)
		}
		// Single conversation per request — both communities share the same conversation.
		conversationBID = resp.Msg.Request.GetConversationId()
		if conversationBID == "" {
			t.Error("Expected conversation ID to be set")
		}
		if conversationBID != conversationAID {
			t.Errorf("Expected single shared conversation, got different IDs: A=%s B=%s", conversationAID, conversationBID)
		}
	})

	// Step 8: System adds Helper Y to Community B conversation (verified implicitly)

	// Step 9: Requester coordinates with both groups in separate conversations (verified by conversation access)

	// Step 10: Requester marks request as fulfilled
	t.Run("Step10_RequesterMarksFulfilled", func(t *testing.T) {
		_, err := requesterRequestClient.MarkRequestFulfilled(ctx, connect.NewRequest(&api.MarkRequestFulfilledRequest{
			RequestId: requestID,
		}))
		if err != nil {
			t.Fatalf("MarkRequestFulfilled failed: %v", err)
		}
	})

	// ========== POSTCONDITIONS ==========

	t.Run("Postcondition_RequestFulfilledInBothCommunities", func(t *testing.T) {
		// Verify state (same across both communities)
		resp, err := requesterRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
			RequestId: requestID,
		}))
		if err != nil {
			t.Fatalf("GetRequest failed: %v", err)
		}
		if resp.Msg.Request.State != api.RequestState_REQUEST_STATE_FULFILLED {
			t.Errorf("Expected FULFILLED state, got %v", resp.Msg.Request.State)
		}

		// Verify request is visible in both communities via listings
		listRespA, err := requesterRequestClient.ListRequests(ctx, connect.NewRequest(&api.ListRequestsRequest{
			CommunityId: communityAID,
			State:       api.RequestState_REQUEST_STATE_FULFILLED,
		}))
		if err != nil {
			t.Fatalf("ListRequests in Community A failed: %v", err)
		}
		foundA := false
		for _, req := range listRespA.Msg.Requests {
			if req.Id == requestID {
				foundA = true
				break
			}
		}
		if !foundA {
			t.Error("Request should be visible in Community A fulfilled listings")
		}

		listRespB, err := requesterRequestClient.ListRequests(ctx, connect.NewRequest(&api.ListRequestsRequest{
			CommunityId: communityBID,
			State:       api.RequestState_REQUEST_STATE_FULFILLED,
		}))
		if err != nil {
			t.Fatalf("ListRequests in Community B failed: %v", err)
		}
		foundB := false
		for _, req := range listRespB.Msg.Requests {
			if req.Id == requestID {
				foundB = true
				break
			}
		}
		if !foundB {
			t.Error("Request should be visible in Community B fulfilled listings")
		}
	})

	t.Run("Postcondition_TwoSeparateConversationsExist", func(t *testing.T) {
		// Single conversation per request — both communities share the same conversation.
		if conversationAID == "" {
			t.Error("Expected shared conversation ID to be set")
		}
		if conversationAID != conversationBID {
			t.Errorf("Expected single shared conversation, got different IDs: A=%s B=%s", conversationAID, conversationBID)
		}
	})

	t.Run("Postcondition_HelperXOnlyInCommunityA", func(t *testing.T) {
		// Helper X should be able to see the request via Community A listings
		listRespA, err := helperXRequestClient.ListRequests(ctx, connect.NewRequest(&api.ListRequestsRequest{
			CommunityId: communityAID,
			State:       api.RequestState_REQUEST_STATE_FULFILLED,
		}))
		if err != nil {
			t.Fatalf("Helper X should access Community A listings: %v", err)
		}
		foundInA := false
		for _, req := range listRespA.Msg.Requests {
			if req.Id == requestID {
				foundInA = true
				break
			}
		}
		if !foundInA {
			t.Error("Helper X should see request in Community A")
		}

		// Helper X should not see the request in Community B listings (not a member)
		listRespB, err := helperXRequestClient.ListRequests(ctx, connect.NewRequest(&api.ListRequestsRequest{
			CommunityId: communityBID,
			State:       api.RequestState_REQUEST_STATE_FULFILLED,
		}))
		if err == nil {
			// If Helper X somehow can list Community B, verify request is not there
			for _, req := range listRespB.Msg.Requests {
				if req.Id == requestID {
					t.Error("Helper X should not see request in Community B (not a member)")
				}
			}
		}
	})

	t.Run("Postcondition_HelperYOnlyInCommunityB", func(t *testing.T) {
		// Helper Y should be able to see the request via Community B listings
		listRespB, err := helperYRequestClient.ListRequests(ctx, connect.NewRequest(&api.ListRequestsRequest{
			CommunityId: communityBID,
			State:       api.RequestState_REQUEST_STATE_FULFILLED,
		}))
		if err != nil {
			t.Fatalf("Helper Y should access Community B listings: %v", err)
		}
		foundInB := false
		for _, req := range listRespB.Msg.Requests {
			if req.Id == requestID {
				foundInB = true
				break
			}
		}
		if !foundInB {
			t.Error("Helper Y should see request in Community B")
		}

		// Helper Y should not see the request in Community A listings (not a member)
		listRespA, err := helperYRequestClient.ListRequests(ctx, connect.NewRequest(&api.ListRequestsRequest{
			CommunityId: communityAID,
			State:       api.RequestState_REQUEST_STATE_FULFILLED,
		}))
		if err == nil {
			// If Helper Y somehow can list Community A, verify request is not there
			for _, req := range listRespA.Msg.Requests {
				if req.Id == requestID {
					t.Error("Helper Y should not see request in Community A (not a member)")
				}
			}
		}
	})

	t.Run("Postcondition_RequesterInBothConversations", func(t *testing.T) {
		// Requester should be able to access both conversations
		requesterChatClient := createAuthChatClient(requesterToken, serverURL)

		_, err := requesterChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationAID,
		}))
		if err != nil {
			t.Errorf("Requester should access Community A conversation: %v", err)
		}

		_, err = requesterChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationBID,
		}))
		if err != nil {
			t.Errorf("Requester should access Community B conversation: %v", err)
		}
	})

	t.Run("Postcondition_NotVisibleInEitherCommunityFeed", func(t *testing.T) {
		// Check Community A
		listRespA, err := requesterRequestClient.ListRequests(ctx, connect.NewRequest(&api.ListRequestsRequest{
			CommunityId: communityAID,
			State:       api.RequestState_REQUEST_STATE_ACTIVE,
		}))
		if err != nil {
			t.Fatalf("ListRequests in Community A failed: %v", err)
		}
		for _, req := range listRespA.Msg.Requests {
			if req.Id == requestID {
				t.Error("Fulfilled request should not appear in Community A active feed")
			}
		}

		// Check Community B
		listRespB, err := requesterRequestClient.ListRequests(ctx, connect.NewRequest(&api.ListRequestsRequest{
			CommunityId: communityBID,
			State:       api.RequestState_REQUEST_STATE_ACTIVE,
		}))
		if err != nil {
			t.Fatalf("ListRequests in Community B failed: %v", err)
		}
		for _, req := range listRespB.Msg.Requests {
			if req.Id == requestID {
				t.Error("Fulfilled request should not appear in Community B active feed")
			}
		}
	})

	// Per docs/workflows/request.md Example 6 (cross-community fan-out):
	// - REQUEST_CREATED in Community A → "New request" to Helper X. The
	//   subsequent ShareRequest with Community B does NOT emit
	//   REQUEST_CREATED (sharing an existing request does not re-broadcast).
	// - 2 OFFER_MADE events → 2 "Help offered" to requester.
	// - REQUEST_FULFILLED does NOT notify (product decision, #2492): neither
	//   offerer receives a fulfillment push. The cross-community dedup of a
	//   targeted recipient set — the #2088 regression this workflow once
	//   guarded — is now exercised against a still-notifying event by
	//   community_subscriber.TestSubscriber_DedupsTargetedAcrossCommunities.
	// Total = 1 "New request" + 2 "Help offered" = 3 notifications.
	t.Run("Postcondition_RequesterNotifiedOnOffers", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 3, 5*time.Second)
		requesterNotifs := logCapture.GetNotificationLogsForUser(requesterID)
		var helpOfferedCount int
		for _, n := range requesterNotifs {
			if n.Title == "Help offered" {
				helpOfferedCount++
			}
		}
		if helpOfferedCount != 2 {
			t.Errorf("Expected requester to receive 2 'Help Offered' notifications, got %d", helpOfferedCount)
			for _, n := range requesterNotifs {
				t.Logf("  Notification: title=%s", n.Title)
			}
		}
	})

	t.Run("Postcondition_HelpersNewRequestOnly", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 3, 5*time.Second)
		// Helper X (Community A): 1 "New request" and no fulfillment push.
		// Helper Y (Community B): no "New request" (sharing does not re-broadcast)
		// and no fulfillment push. REQUEST_FULFILLED does not notify (#2492).
		helperXNotifs := logCapture.GetNotificationLogsForUser(helperXID)
		var xNewReq, xFulfilled int
		for _, n := range helperXNotifs {
			switch n.Title {
			case "New request":
				xNewReq++
			case "Request fulfilled":
				xFulfilled++
			}
		}
		if xNewReq != 1 || xFulfilled != 0 {
			if xNewReq != 1 {
				t.Errorf("Expected Helper X to receive 1 'New Request' notification, got %d", xNewReq)
			}
			if xFulfilled != 0 {
				t.Errorf("Expected Helper X to receive 0 'Request Fulfilled' notifications (#2492), got %d", xFulfilled)
			}
			for _, n := range helperXNotifs {
				t.Logf("  Helper X notification: title=%s", n.Title)
			}
		}

		helperYNotifs := logCapture.GetNotificationLogsForUser(helperYID)
		var yNewReq, yFulfilled int
		for _, n := range helperYNotifs {
			switch n.Title {
			case "New request":
				yNewReq++
			case "Request fulfilled":
				yFulfilled++
			}
		}
		if yNewReq != 0 || yFulfilled != 0 {
			if yNewReq != 0 {
				t.Errorf("Expected Helper Y to receive 0 'New Request' notifications (sharing does not re-broadcast), got %d", yNewReq)
			}
			if yFulfilled != 0 {
				t.Errorf("Expected Helper Y to receive 0 'Request Fulfilled' notifications (#2492), got %d", yFulfilled)
			}
			for _, n := range helperYNotifs {
				t.Logf("  Helper Y notification: title=%s", n.Title)
			}
		}
	})
}

// TestRequest_Example7_UnsharingFromCommunity tests the unsharing workflow.
// Covers: docs/workflows/request.md - Workflow Example 7.
func TestRequest_Example7_UnsharingFromCommunity(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// ========== PRECONDITIONS ==========
	// - Two test communities exist (Community A and Community B)
	// - A requester is a member of both communities
	// - The requester has created a request shared with both communities
	// - Helper A has offered in Community A
	// - Helper B has offered in Community B

	requesterToken, _ := registerFirstUser(t, serverURL, "requester@example.com", "Requester")
	requesterCommunityClient := createAuthCommunityClient(requesterToken, serverURL)
	requesterLocationClient := createAuthLocationClient(requesterToken, serverURL)
	requesterRequestClient := createAuthRequestClient(requesterToken, serverURL)

	communityAID := setupTestCommunity(t, ctx, requesterCommunityClient, "Community A", "First community")
	communityBID := setupTestCommunity(t, ctx, requesterCommunityClient, "Community B", "Second community")
	locationID := setupTestLocation(t, ctx, requesterLocationClient, "Seattle")

	helperAToken, _ := registerUserByInvite(t, serverURL, requesterToken, communityAID, "helper-a@example.com", "Helper A")
	helperARequestClient := createAuthRequestClient(helperAToken, serverURL)

	helperBToken, _ := registerUserByInvite(t, serverURL, requesterToken, communityBID, "helper-b@example.com", "Helper B")
	helperBRequestClient := createAuthRequestClient(helperBToken, serverURL)
	helperBChatClient := createAuthChatClient(helperBToken, serverURL)

	// Create request in Community A
	submitResp, err := requesterRequestClient.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Need a Ladder",
		Description: "Looking for a ladder",
		LocationId:  locationID,
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestIntoCommunity(t, ctx, requesterCommunityClient, submitResp.Msg.RequestId, communityAID)
	requestID := submitResp.Msg.RequestId
	// Every request is born in its own per-item community (#2492); it is always
	// present as a CommunityRequest alongside any communities it is explicitly
	// shared to. This per-item community is the genuine "last remaining"
	// community for the unshare guard.
	itemCommunityID := submitResp.Msg.GetItemCommunityId()
	if itemCommunityID == "" {
		t.Fatal("expected SubmitRequest to return ItemCommunityId")
	}

	// Helper A offers in Community A
	_, err = helperARequestClient.OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityAID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill in Community A failed: %v", err)
	}

	// Share with Community B
	shareRequestIntoCommunity(t, ctx, requesterCommunityClient, requestID, communityBID)

	// Helper B offers in Community B
	offerResp, err := helperBRequestClient.OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityBID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill in Community B failed: %v", err)
	}
	conversationBID := offerResp.Msg.Request.GetConversationId()

	// ========== STEPS ==========

	// Step 1: Requester unshares request from Community B
	t.Run("Step1_RequesterUnsharesFromCommunityB", func(t *testing.T) {
		if err := unshareRequestFromCommunity(ctx, requesterCommunityClient, requestID, communityBID); err != nil {
			t.Fatalf("UnshareItem failed: %v", err)
		}
	})

	// Step 2: System archives the CommunityRequest record for Community B (verified implicitly)

	// ========== POSTCONDITIONS ==========

	t.Run("Postcondition_NotVisibleInCommunityBFeed", func(t *testing.T) {
		listResp, err := requesterRequestClient.ListRequests(ctx, connect.NewRequest(&api.ListRequestsRequest{
			CommunityId: communityBID,
		}))
		if err != nil {
			t.Fatalf("ListRequests in Community B failed: %v", err)
		}
		for _, req := range listResp.Msg.Requests {
			if req.Id == requestID {
				t.Error("Request should not appear in Community B feed after unsharing")
			}
		}
	})

	t.Run("Postcondition_VisibleInCommunityAFeed", func(t *testing.T) {
		listResp, err := requesterRequestClient.ListRequests(ctx, connect.NewRequest(&api.ListRequestsRequest{
			CommunityId: communityAID,
		}))
		if err != nil {
			t.Fatalf("ListRequests in Community A failed: %v", err)
		}
		var found bool
		for _, req := range listResp.Msg.Requests {
			if req.Id == requestID {
				found = true
				break
			}
		}
		if !found {
			t.Error("Request should still be visible in Community A feed")
		}
	})

	t.Run("Postcondition_CommunityBConversationAccessible", func(t *testing.T) {
		// Helper B should still be able to access the conversation history
		_, err := helperBChatClient.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
			ConversationId: conversationBID,
		}))
		if err != nil {
			t.Errorf("Community B conversation should remain accessible: %v", err)
		}
	})

	t.Run("Postcondition_RequestStateUnchanged", func(t *testing.T) {
		resp, err := requesterRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
			RequestId: requestID,
		}))
		if err != nil {
			t.Fatalf("GetRequest in Community A failed: %v", err)
		}
		if resp.Msg.Request.State != api.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
			t.Errorf("Expected OFFERS_RECEIVED state, got %v", resp.Msg.Request.State)
		}
	})

	t.Run("Postcondition_CannotUnshareFromLastCommunity", func(t *testing.T) {
		// After unsharing B, the request is in [per-item community, A] (#2492).
		// Unsharing from Community A now succeeds: the per-item community remains.
		if err := unshareRequestFromCommunity(ctx, requesterCommunityClient, requestID, communityAID); err != nil {
			t.Fatalf("UnshareItem from Community A should succeed (per-item community remains): %v", err)
		}

		// Now only the per-item community remains. Unsharing from it must fail:
		// you can't unshare from the last remaining community.
		err := unshareRequestFromCommunity(ctx, requesterCommunityClient, requestID, itemCommunityID)
		if err == nil {
			t.Fatal("Expected error when trying to unshare from last (per-item) community")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeFailedPrecondition {
			t.Errorf("Expected FAILED_PRECONDITION error code, got %v", connectErr.Code())
		}
	})

	// Per docs/workflows/request.md Example 7:
	//   1 "New request" (REQUEST_CREATED in Community A on submit; ShareRequest
	//                    with Community B does NOT re-emit REQUEST_CREATED) +
	//   2 "Help offered" (REQUEST_OFFER_MADE → requester for each offer).
	// Unsharing itself does not emit a notification.
	t.Run("Postcondition_NoNotificationForUnsharing", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 3, 5*time.Second)
		notifLogs := logCapture.GetNotificationLogs()
		expectedCount := 3
		if len(notifLogs) != expectedCount {
			t.Errorf("Expected %d notifications (1 new request + 2 offers), got %d", expectedCount, len(notifLogs))
			for _, n := range notifLogs {
				t.Logf("  Notification: user=%s title=%s", n.UserID, n.Title)
			}
		}
	})
}
