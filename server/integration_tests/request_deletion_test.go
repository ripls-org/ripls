package integration_tests

// Integration tests for request deletion workflows.
// Tests are mapped 1:1 to workflow examples in docs/workflows/request.md.

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// TestRequest_Example5_RequestDeleted tests the request soft-deletion workflow.
// Covers: docs/workflows/request.md - Workflow Example 5.
func TestRequest_Example5_RequestDeleted(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// ========== PRECONDITIONS ==========
	// - A test community exists
	// - Two users exist: a requester and a helper
	// - The requester has created a request
	// - The helper has offered to help (conversation exists)

	requesterToken, _ := registerFirstUser(t, serverURL, "requester@example.com", "Requester")
	requesterCommunityClient := createAuthCommunityClient(requesterToken, serverURL)
	requesterLocationClient := createAuthLocationClient(requesterToken, serverURL)
	requesterRequestClient := createAuthRequestClient(requesterToken, serverURL)
	requesterChatClient := createAuthChatClient(requesterToken, serverURL)
	requesterSearchClient := createAuthSearchClient(requesterToken, serverURL)

	communityID := setupTestCommunity(t, ctx, requesterCommunityClient, "Deletion Test Community", "Testing deletion workflows")
	locationID := setupTestLocation(t, ctx, requesterLocationClient, "Portland")

	helperToken, _ := registerUserByInvite(t, serverURL, requesterToken, communityID, "helper@example.com", "Helper")
	helperRequestClient := createAuthRequestClient(helperToken, serverURL)

	// Create request with unique searchable title
	submitResp, err := requesterRequestClient.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Need a UniqueXYZ123 Chainsaw",
		Description: "Looking for a chainsaw for tree work",
		LocationId:  locationID,
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestIntoCommunity(t, ctx, requesterCommunityClient, submitResp.Msg.RequestId, communityID)
	requestID := submitResp.Msg.RequestId

	// Helper offers to help (creates conversation)
	offerResp, err := helperRequestClient.OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill failed: %v", err)
	}
	conversationID := offerResp.Msg.Request.GetConversationId()

	// ========== VERIFY PRECONDITIONS ==========

	t.Run("Precondition_RequestAccessible", func(t *testing.T) {
		resp, err := requesterRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
			RequestId: requestID,
		}))
		if err != nil {
			t.Fatalf("GetRequest should succeed before deletion: %v", err)
		}
		if resp.Msg.Request.State != api.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
			t.Errorf("Expected OFFERS_RECEIVED state, got %v", resp.Msg.Request.State)
		}
	})

	t.Run("Precondition_RequestInListings", func(t *testing.T) {
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
			t.Error("Request should appear in listings before deletion")
		}
	})

	t.Run("Precondition_RequestInSearch", func(t *testing.T) {
		searchResp, err := requesterSearchClient.Search(ctx, connect.NewRequest(&api.SearchRequest{
			CommunityIds: []string{communityID},
			Query:        "UniqueXYZ123",
		}))
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		var found bool
		for _, result := range searchResp.Msg.Results {
			if result.ItemType == api.SearchItemType_SEARCH_ITEM_TYPE_REQUEST {
				if result.GetRequest().Id == requestID {
					found = true
					break
				}
			}
		}
		if !found {
			t.Error("Request should appear in search results before deletion")
		}
	})

	t.Run("Precondition_ConversationContextAccessible", func(t *testing.T) {
		resp, err := requesterChatClient.GetConversationContext(ctx, connect.NewRequest(&api.GetConversationContextRequest{
			ConversationId: conversationID,
		}))
		if err != nil {
			t.Fatalf("GetConversationContext should succeed before deletion: %v", err)
		}
		if !strings.Contains(resp.Msg.Context.TopicTitle, "UniqueXYZ123") {
			t.Errorf("Expected topic title to contain 'UniqueXYZ123', got '%s'", resp.Msg.Context.TopicTitle)
		}
	})

	// ========== DELETE REQUEST ==========

	t.Run("Step_RequesterDeletesRequest", func(t *testing.T) {
		_, err := requesterRequestClient.DeleteRequest(ctx, connect.NewRequest(&api.DeleteRequestRequest{
			RequestId: requestID,
		}))
		if err != nil {
			t.Fatalf("DeleteRequest failed: %v", err)
		}
	})

	// ========== VERIFY POSTCONDITIONS ==========

	t.Run("Postcondition_RequestReturnsNotFound", func(t *testing.T) {
		_, err := requesterRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
			RequestId: requestID,
		}))
		if err == nil {
			t.Fatal("Expected NOT_FOUND error for deleted request")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("Expected NOT_FOUND error code, got %v", connectErr.Code())
		}
	})

	t.Run("Postcondition_RequestExcludedFromListings", func(t *testing.T) {
		listResp, err := requesterRequestClient.ListRequests(ctx, connect.NewRequest(&api.ListRequestsRequest{
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("ListRequests failed: %v", err)
		}
		for _, req := range listResp.Msg.Requests {
			if req.Id == requestID {
				t.Error("Deleted request should not appear in listings")
			}
		}
	})

	t.Run("Postcondition_RequestExcludedFromMyRequests", func(t *testing.T) {
		listResp, err := requesterRequestClient.ListMyRequests(ctx, connect.NewRequest(&api.ListMyRequestsRequest{}))
		if err != nil {
			t.Fatalf("ListMyRequests failed: %v", err)
		}
		for _, req := range listResp.Msg.Requests {
			if req.Id == requestID {
				t.Error("Deleted request should not appear in My Requests")
			}
		}
	})

	t.Run("Postcondition_RequestExcludedFromSearch", func(t *testing.T) {
		searchResp, err := requesterSearchClient.Search(ctx, connect.NewRequest(&api.SearchRequest{
			CommunityIds: []string{communityID},
			Query:        "UniqueXYZ123",
		}))
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		for _, result := range searchResp.Msg.Results {
			if result.ItemType == api.SearchItemType_SEARCH_ITEM_TYPE_REQUEST {
				if result.GetRequest().Id == requestID {
					t.Error("Deleted request should not appear in search results")
				}
			}
		}
	})

	t.Run("Postcondition_ConversationContextReturnsNotFound", func(t *testing.T) {
		_, err := requesterChatClient.GetConversationContext(ctx, connect.NewRequest(&api.GetConversationContextRequest{
			ConversationId: conversationID,
		}))
		if err == nil {
			t.Fatal("Expected NOT_FOUND error for conversation with deleted request topic")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("Expected NOT_FOUND error code, got %v", connectErr.Code())
		}
	})

	t.Run("Postcondition_NonOwnerCannotDeleteRequest", func(t *testing.T) {
		// Create another request to test non-owner deletion
		submitResp2, err := requesterRequestClient.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
			Title:       "Another Request",
			Description: "Test request",
			LocationId:  locationID,
		}))
		if err != nil {
			t.Fatalf("SubmitRequest failed: %v", err)
		}
		shareRequestIntoCommunity(t, ctx, requesterCommunityClient, submitResp2.Msg.RequestId, communityID)
		anotherRequestID := submitResp2.Msg.RequestId

		// Helper tries to delete requester's request (should fail)
		_, err = helperRequestClient.DeleteRequest(ctx, connect.NewRequest(&api.DeleteRequestRequest{
			RequestId: anotherRequestID,
		}))
		if err == nil {
			t.Fatal("Expected PERMISSION_DENIED error when non-owner tries to delete")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PERMISSION_DENIED error code, got %v", connectErr.Code())
		}
	})

	// Per docs/workflows/request.md Example 5:
	//   1 "New request" (REQUEST_CREATED → helper, original request) +
	//   1 "Help offered" (OFFER_MADE → requester) +
	//   1 "New request" (REQUEST_CREATED → helper, second request created in
	//                    the non-owner-delete postcondition).
	// Deletion itself does not emit a notification.
	t.Run("Postcondition_NoNotificationForDeletionItself", func(t *testing.T) {
		WaitForNotificationCount(logCapture, 3, 5*time.Second)
		notifLogs := logCapture.GetNotificationLogs()
		expectedCount := 3
		if len(notifLogs) != expectedCount {
			t.Errorf("Expected %d notifications (2 new request + 1 offer), got %d", expectedCount, len(notifLogs))
			for _, n := range notifLogs {
				t.Logf("  Notification: user=%s title=%s", n.UserID, n.Title)
			}
		}
	})
}
