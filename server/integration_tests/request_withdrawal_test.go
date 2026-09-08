package integration_tests

// Integration tests for request offer-withdrawal workflows.
// Tests are mapped 1:1 to workflow examples in docs/workflows/request.md.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// TestRequest_Example3_HelperWithdrawsOffer tests the helper withdrawal workflow.
// Covers: docs/workflows/request.md - Workflow Example 3.
func TestRequest_Example3_HelperWithdrawsOffer(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// ========== PRECONDITIONS ==========
	// - A test community exists
	// - Three users exist: a requester, helper A, and helper B
	// - The requester has created a request
	// - Helper A and Helper B have both offered to help

	requesterToken, _ := registerFirstUser(t, serverURL, "requester@example.com", "Requester")
	requesterCommunityClient := createAuthCommunityClient(requesterToken, serverURL)
	requesterLocationClient := createAuthLocationClient(requesterToken, serverURL)
	requesterRequestClient := createAuthRequestClient(requesterToken, serverURL)

	communityID := setupTestCommunity(t, ctx, requesterCommunityClient, "Request Test Community", "Testing request workflows")
	locationID := setupTestLocation(t, ctx, requesterLocationClient, "Denver")

	helperAToken, _ := registerUserByInvite(t, serverURL, requesterToken, communityID, "helper-a@example.com", "Helper A")
	helperARequestClient := createAuthRequestClient(helperAToken, serverURL)

	helperBToken, _ := registerUserByInvite(t, serverURL, requesterToken, communityID, "helper-b@example.com", "Helper B")
	helperBRequestClient := createAuthRequestClient(helperBToken, serverURL)

	// Create request
	submitResp, err := requesterRequestClient.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Need a Tent",
		Description: "Looking for a tent for camping trip",
		LocationId:  locationID,
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestIntoCommunity(t, ctx, requesterCommunityClient, submitResp.Msg.RequestId, communityID)
	requestID := submitResp.Msg.RequestId

	// Helper A offers to help
	_, err = helperARequestClient.OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill failed: %v", err)
	}

	// Helper B offers to help
	_, err = helperBRequestClient.OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill failed: %v", err)
	}

	// ========== STEPS ==========

	// Step 1: Helper A leaves the conversation (withdraws offer)
	t.Run("Step1_HelperAWithdrawsOffer", func(t *testing.T) {
		_, err := helperARequestClient.WithdrawOffer(ctx, connect.NewRequest(&api.WithdrawOfferRequest{
			RequestId:   requestID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("WithdrawOffer failed: %v", err)
		}
	})

	// Step 2: System removes Helper A from participant list (verified implicitly)

	// ========== POSTCONDITIONS ==========

	t.Run("Postcondition_RequestStillActive", func(t *testing.T) {
		resp, err := requesterRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
			RequestId: requestID,
		}))
		if err != nil {
			t.Fatalf("GetRequest failed: %v", err)
		}
		// Should remain in OFFERS_RECEIVED since Helper B is still offering
		if resp.Msg.Request.State != api.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
			t.Errorf("Expected OFFERS_RECEIVED state, got %v", resp.Msg.Request.State)
		}
	})

	t.Run("Postcondition_HelperBStillOffering", func(t *testing.T) {
		// Helper B should still be able to see their offer
		resp, err := helperBRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
			RequestId: requestID,
		}))
		if err != nil {
			t.Fatalf("GetRequest failed: %v", err)
		}
		// Verify Helper B is still in the offerers list or conversation participants
		// The request should still have at least 1 helper
		offererCount := len(resp.Msg.Request.Offerers)
		if offererCount < 1 {
			t.Errorf("Expected at least 1 offerer, got %d", offererCount)
		}
	})

	// Per docs/workflows/request.md Example 3:
	//   2 "New request" (REQUEST_CREATED → both helpers) +
	//   2 "Help offered" (OFFER_MADE → requester) +
	//   1 "Offer withdrawn" (OFFER_WITHDRAWN → requester)
	t.Run("Postcondition_RequesterNotifiedOfWithdrawal", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 5, 5*time.Second)
		notifLogs := logCapture.GetNotificationLogs()
		expectedCount := 5
		if len(notifLogs) != expectedCount {
			t.Errorf("Expected %d notifications (2 new request + 2 offers + 1 withdrawal), got %d", expectedCount, len(notifLogs))
			for _, n := range notifLogs {
				t.Logf("  Notification: user=%s title=%s", n.UserID, n.Title)
			}
		}
		var withdrawnCount int
		for _, n := range notifLogs {
			if n.Title == "Offer withdrawn" {
				withdrawnCount++
			}
		}
		if withdrawnCount != 1 {
			t.Errorf("Expected 1 'Offer Withdrawn' notification, got %d", withdrawnCount)
		}
	})
}

// TestRequest_Example4_LastHelperWithdraws tests the last helper withdrawal workflow.
// Covers: docs/workflows/request.md - Workflow Example 4.
func TestRequest_Example4_LastHelperWithdraws(t *testing.T) {
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

	communityID := setupTestCommunity(t, ctx, requesterCommunityClient, "Request Test Community", "Testing request workflows")
	locationID := setupTestLocation(t, ctx, requesterLocationClient, "Austin")

	helperToken, _ := registerUserByInvite(t, serverURL, requesterToken, communityID, "helper@example.com", "Helper")
	helperRequestClient := createAuthRequestClient(helperToken, serverURL)

	// Create request
	submitResp, err := requesterRequestClient.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Need a Bike Pump",
		Description: "Looking for a bike pump",
		LocationId:  locationID,
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestIntoCommunity(t, ctx, requesterCommunityClient, submitResp.Msg.RequestId, communityID)
	requestID := submitResp.Msg.RequestId

	// Helper offers to help
	_, err = helperRequestClient.OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill failed: %v", err)
	}

	// Verify in OFFERS_RECEIVED state
	getResp, err := requesterRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
		RequestId: requestID,
	}))
	if err != nil {
		t.Fatalf("GetRequest failed: %v", err)
	}
	if getResp.Msg.Request.State != api.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
		t.Fatalf("Expected OFFERS_RECEIVED state before withdrawal, got %v", getResp.Msg.Request.State)
	}

	// ========== STEPS ==========

	// Step 1: Helper leaves the conversation (withdraws offer)
	t.Run("Step1_HelperWithdrawsOffer", func(t *testing.T) {
		_, err := helperRequestClient.WithdrawOffer(ctx, connect.NewRequest(&api.WithdrawOfferRequest{
			RequestId:   requestID,
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("WithdrawOffer failed: %v", err)
		}
	})

	// Step 2: System removes helper from participant list (verified implicitly)

	// ========== POSTCONDITIONS ==========

	t.Run("Postcondition_RequestRevertsToActive", func(t *testing.T) {
		resp, err := requesterRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
			RequestId: requestID,
		}))
		if err != nil {
			t.Fatalf("GetRequest failed: %v", err)
		}
		// Should revert to ACTIVE since no helpers remain
		if resp.Msg.Request.State != api.RequestState_REQUEST_STATE_ACTIVE {
			t.Errorf("Expected ACTIVE state after last helper withdraws, got %v", resp.Msg.Request.State)
		}
	})

	t.Run("Postcondition_VisibleInCommunityFeed", func(t *testing.T) {
		// The request should be visible in community request feed again
		listResp, err := requesterRequestClient.ListRequests(ctx, connect.NewRequest(&api.ListRequestsRequest{
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("ListRequests failed: %v", err)
		}
		var found bool
		for _, req := range listResp.Msg.Requests {
			if req.Id == requestID {
				found = true
				break
			}
		}
		if !found {
			t.Error("Request should be visible in community feed after reverting to ACTIVE")
		}
	})

	// Per docs/workflows/request.md Example 4:
	//   1 "New request" (REQUEST_CREATED → helper) +
	//   1 "Help offered" (OFFER_MADE → requester) +
	//   1 "Offer withdrawn" (OFFER_WITHDRAWN → requester)
	t.Run("Postcondition_RequesterNotifiedOfWithdrawal", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 3, 5*time.Second)
		notifLogs := logCapture.GetNotificationLogs()
		expectedCount := 3
		if len(notifLogs) != expectedCount {
			t.Errorf("Expected %d notifications (new request + offer + withdrawal), got %d", expectedCount, len(notifLogs))
			for _, n := range notifLogs {
				t.Logf("  Notification: user=%s title=%s", n.UserID, n.Title)
			}
		}
	})
}
