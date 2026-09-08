package request

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

func TestService_OfferToFulfill_FirstOffer(t *testing.T) {
	service, testStorage, _, done, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID := setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offererID)

	// Create a request
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createReq := connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Looking for a power drill",
	})
	createResp, _ := service.SubmitRequest(ctx, createReq)
	services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch

	requestID := createResp.Msg.RequestId
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

	// First user offers to fulfill
	ctx = createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)
	offerReq := connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})

	offerResp, err := service.OfferToFulfill(ctx, offerReq)
	if err != nil {
		t.Fatalf("OfferToFulfill failed: %v", err)
	}
	services.WaitForNotification(t, done) // Wait for notification

	// Verify enriched response contains full request details
	if offerResp.Msg.Request == nil {
		t.Fatal("Expected request in response")
	}

	req := offerResp.Msg.Request
	if req.Id != requestID {
		t.Errorf("Expected request ID %s, got %s", requestID, req.Id)
	}
	if req.GetConversationId() == "" {
		t.Error("Expected non-empty conversation ID")
	}
	// State should transition to OFFERS_RECEIVED when first offer is made
	if req.State != api.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
		t.Errorf("Expected state OFFERS_RECEIVED after first offer, got %v", req.State)
	}
	if req.Requester == nil {
		t.Error("Expected requester to be populated")
	} else if req.Requester.Id != requesterID {
		t.Errorf("Expected requester ID %s, got %s", requesterID, req.Requester.Id)
	}
	if len(req.Offerers) != 1 {
		t.Errorf("Expected 1 offerer, got %d", len(req.Offerers))
	} else if req.Offerers[0].Id != offererID {
		t.Errorf("Expected offerer ID %s, got %s", offererID, req.Offerers[0].Id)
	}
	if req.Description != "Looking for a power drill" {
		t.Errorf("Expected description 'Looking for a power drill', got %s", req.Description)
	}

	// Verify request state remains ACTIVE (until someone posts a message)
	request := &models.Request{}
	err = testStorage.GetByID(context.Background(), requestID, request)
	if err != nil {
		t.Fatalf("Failed to retrieve request: %v", err)
	}

	// State should transition to OFFERS_RECEIVED when first offer is made
	if request.State != models.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
		t.Errorf("Expected state OFFERS_RECEIVED after first offer, got %v", request.State)
	}

	// Get conversation ID from Request
	requestStored := &models.Request{}
	if err := testStorage.GetByID(context.Background(), requestID, requestStored); err != nil {
		t.Fatalf("Failed to get Request: %v", err)
	}
	if requestStored.ConversationId == "" {
		t.Error("Expected conversation_id to be set")
	}

	// Verify conversation has both requester and offerer
	conversation := &models.ChatConversation{}
	err = testStorage.GetByID(context.Background(), requestStored.ConversationId, conversation)
	if err != nil {
		t.Fatalf("Failed to retrieve conversation: %v", err)
	}

	if len(conversation.ParticipantIds) != 2 {
		t.Errorf("Expected 2 participants, got %d", len(conversation.ParticipantIds))
	}

	hasRequester := false
	hasOfferer := false
	for _, pid := range conversation.ParticipantIds {
		if pid == requesterID {
			hasRequester = true
		}
		if pid == offererID {
			hasOfferer = true
		}
	}

	if !hasRequester {
		t.Error("Expected requester in conversation")
	}
	if !hasOfferer {
		t.Error("Expected offerer in conversation")
	}

	// Verify offer made event was created
	events, err := testStorage.QueryByFields(context.Background(), map[string]any{
		"community_id": communityID,
		"event_type":   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE,
	}, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("Failed to query events: %v", err)
	}

	if len(events) != 1 {
		t.Errorf("Expected 1 offer event, got %d", len(events))
	}
}

func TestService_OfferToFulfill_SubsequentOffer(t *testing.T) {
	service, testStorage, _, done, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offerer1ID := setupTestUser(t, testStorage, "Offerer1", "offerer1@example.com")
	offerer2ID := setupTestUser(t, testStorage, "Offerer2", "offerer2@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offerer1ID, offerer2ID)

	// Create a request
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createReq := connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Looking for a power drill",
	})
	createResp, _ := service.SubmitRequest(ctx, createReq)
	services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch

	requestID := createResp.Msg.RequestId
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

	// First offer
	ctx = createAuthenticatedContext(offerer1ID, "offerer1@example.com", models.Role_ROLE_USER)
	offerReq1 := connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	offerResp1, _ := service.OfferToFulfill(ctx, offerReq1)
	services.WaitForNotification(t, done)

	// Second offer
	ctx = createAuthenticatedContext(offerer2ID, "offerer2@example.com", models.Role_ROLE_USER)
	offerReq2 := connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	offerResp2, err := service.OfferToFulfill(ctx, offerReq2)
	if err != nil {
		t.Fatalf("Second OfferToFulfill failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Both should return same conversation
	if offerResp1.Msg.Request.GetConversationId() != offerResp2.Msg.Request.GetConversationId() {
		t.Error("Expected both offers to use same conversation")
	}

	// Verify second response has both offerers
	if len(offerResp2.Msg.Request.Offerers) != 2 {
		t.Errorf("Expected 2 offerers in second response, got %d", len(offerResp2.Msg.Request.Offerers))
	}

	// Verify state is OFFERS_RECEIVED (changed when first offer was made)
	if offerResp2.Msg.Request.State != api.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
		t.Errorf("Expected state OFFERS_RECEIVED after offers, got %v", offerResp2.Msg.Request.State)
	}

	// Verify conversation has all 3 participants
	conversation := &models.ChatConversation{}
	err = testStorage.GetByID(context.Background(), offerResp1.Msg.Request.GetConversationId(), conversation)
	if err != nil {
		t.Fatalf("Failed to retrieve conversation: %v", err)
	}

	if len(conversation.ParticipantIds) != 3 {
		t.Errorf("Expected 3 participants, got %d", len(conversation.ParticipantIds))
	}

	// Verify 2 offer events were created
	events, err := testStorage.QueryByFields(context.Background(), map[string]any{
		"community_id": communityID,
		"event_type":   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE,
	}, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("Failed to query events: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("Expected 2 offer events, got %d", len(events))
	}
}

func TestService_WithdrawOffer_Success(t *testing.T) {
	service, testStorage, _, done, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID := setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offererID)

	// Create a request
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createReq := connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Looking for a power drill",
	})
	createResp, _ := service.SubmitRequest(ctx, createReq)
	services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch

	requestID := createResp.Msg.RequestId
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

	// Offerer makes an offer
	ctx = createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)
	offerReq := connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	offerResp, _ := service.OfferToFulfill(ctx, offerReq)
	services.WaitForNotification(t, done)

	conversationID := offerResp.Msg.Request.GetConversationId()

	// Verify conversation has 2 participants before withdrawal
	conversation := &models.ChatConversation{}
	err := testStorage.GetByID(context.Background(), conversationID, conversation)
	if err != nil {
		t.Fatalf("Failed to retrieve conversation: %v", err)
	}
	if len(conversation.ParticipantIds) != 2 {
		t.Errorf("Expected 2 participants before withdrawal, got %d", len(conversation.ParticipantIds))
	}

	// Offerer withdraws their offer
	withdrawReq := connect.NewRequest(&api.WithdrawOfferRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	_, err = service.WithdrawOffer(ctx, withdrawReq)
	if err != nil {
		t.Fatalf("WithdrawOffer failed: %v", err)
	}

	// Verify request state returned to ACTIVE (last offerer withdrew)
	request := &models.Request{}
	err = testStorage.GetByID(context.Background(), requestID, request)
	if err != nil {
		t.Fatalf("Failed to retrieve request: %v", err)
	}

	if request.State != models.RequestState_REQUEST_STATE_ACTIVE {
		t.Errorf("Expected state ACTIVE after last offerer withdrew, got %v", request.State)
	}

	// Verify no active offers remain
	offers, err := testStorage.QueryByFields(context.Background(), map[string]any{
		"request_id": requestID,
		"withdrawn":  false,
	}, &models.RequestOffer{})
	if err != nil {
		t.Fatalf("Failed to query offers: %v", err)
	}
	if len(offers) != 0 {
		t.Errorf("Expected 0 active offerers after withdrawal, got %d", len(offers))
	}

	// Verify offerer was removed from conversation
	err = testStorage.GetByID(context.Background(), conversationID, conversation)
	if err != nil {
		t.Fatalf("Failed to retrieve conversation: %v", err)
	}

	if len(conversation.ParticipantIds) != 1 {
		t.Errorf("Expected 1 participant after withdrawal, got %d", len(conversation.ParticipantIds))
	}

	// Verify requester is still in conversation
	if conversation.ParticipantIds[0] != requesterID {
		t.Errorf("Expected requester to remain in conversation")
	}
}

func TestService_WithdrawOffer_NotLastOfferer(t *testing.T) {
	service, testStorage, _, done, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offerer1ID := setupTestUser(t, testStorage, "Offerer1", "offerer1@example.com")
	offerer2ID := setupTestUser(t, testStorage, "Offerer2", "offerer2@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offerer1ID, offerer2ID)

	// Create a request
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createReq := connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Looking for a power drill",
	})
	createResp, _ := service.SubmitRequest(ctx, createReq)
	services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch

	requestID := createResp.Msg.RequestId
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

	// Both offerers make offers
	ctx1 := createAuthenticatedContext(offerer1ID, "offerer1@example.com", models.Role_ROLE_USER)
	offerReq1 := connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	offerResp, _ := service.OfferToFulfill(ctx1, offerReq1)
	services.WaitForNotification(t, done)

	ctx2 := createAuthenticatedContext(offerer2ID, "offerer2@example.com", models.Role_ROLE_USER)
	offerReq2 := connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	_, _ = service.OfferToFulfill(ctx2, offerReq2)
	services.WaitForNotification(t, done)

	conversationID := offerResp.Msg.Request.GetConversationId()

	// First offerer withdraws
	withdrawReq := connect.NewRequest(&api.WithdrawOfferRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	_, err := service.WithdrawOffer(ctx1, withdrawReq)
	if err != nil {
		t.Fatalf("WithdrawOffer failed: %v", err)
	}

	// Verify request state remains OFFERS_RECEIVED (still has one offerer)
	request := &models.Request{}
	err = testStorage.GetByID(context.Background(), requestID, request)
	if err != nil {
		t.Fatalf("Failed to retrieve request: %v", err)
	}

	if request.State != models.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
		t.Errorf("Expected state OFFERS_RECEIVED (still has one offerer), got %v", request.State)
	}

	// Verify exactly one active offer remains
	offers, err := testStorage.QueryByFields(context.Background(), map[string]any{
		"request_id": requestID,
		"withdrawn":  false,
	}, &models.RequestOffer{})
	if err != nil {
		t.Fatalf("Failed to query offers: %v", err)
	}
	if len(offers) != 1 {
		t.Errorf("Expected 1 active offerer after withdrawal, got %d", len(offers))
	}

	if len(offers) == 1 {
		offer := offers[0].(*models.RequestOffer)
		if offer.UserId != offerer2ID {
			t.Errorf("Expected remaining offerer to be offerer2, got %s", offer.UserId)
		}
	}

	// Verify conversation has 2 participants (requester + remaining offerer)
	conversation := &models.ChatConversation{}
	err = testStorage.GetByID(context.Background(), conversationID, conversation)
	if err != nil {
		t.Fatalf("Failed to retrieve conversation: %v", err)
	}

	if len(conversation.ParticipantIds) != 2 {
		t.Errorf("Expected 2 participants after one withdrawal, got %d", len(conversation.ParticipantIds))
	}
}

func TestService_WithdrawOffer_NotAnOfferer(t *testing.T) {
	service, testStorage, _, done, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID := setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	nonOffererID := setupTestUser(t, testStorage, "NonOfferer", "nonofferer@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offererID, nonOffererID)

	// Create a request and have one person offer
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createReq := connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Looking for a power drill",
	})
	createResp, _ := service.SubmitRequest(ctx, createReq)
	services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch

	requestID := createResp.Msg.RequestId
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

	ctx = createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)
	offerReq := connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	_, _ = service.OfferToFulfill(ctx, offerReq)
	services.WaitForNotification(t, done)

	// Non-offerer tries to withdraw
	ctx = createAuthenticatedContext(nonOffererID, "nonofferer@example.com", models.Role_ROLE_USER)
	withdrawReq := connect.NewRequest(&api.WithdrawOfferRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	_, err := service.WithdrawOffer(ctx, withdrawReq)

	if err == nil {
		t.Fatal("Expected error when non-offerer tries to withdraw")
	}

	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("Expected connect.Error, got %T", err)
	}

	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("Expected NotFound error, got %v", connectErr.Code())
	}
}

func TestService_WithdrawOffer_RequesterCannotWithdraw(t *testing.T) {
	service, testStorage, _, done, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID := setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offererID)

	// Create a request and have someone offer
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createReq := connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Looking for a power drill",
	})
	createResp, _ := service.SubmitRequest(ctx, createReq)
	services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch

	requestID := createResp.Msg.RequestId
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

	ctx = createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)
	offerReq := connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	_, _ = service.OfferToFulfill(ctx, offerReq)
	services.WaitForNotification(t, done)

	// Requester tries to withdraw (should fail)
	ctx = createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	withdrawReq := connect.NewRequest(&api.WithdrawOfferRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	_, err := service.WithdrawOffer(ctx, withdrawReq)

	if err == nil {
		t.Fatal("Expected error when requester tries to withdraw")
	}

	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("Expected connect.Error, got %T", err)
	}

	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
	}
}

func TestService_WithdrawOffer_WrongState(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID := setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offererID)

	// Create a request (state is ACTIVE, no offers yet)
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createReq := connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Looking for a power drill",
	})
	createResp, _ := service.SubmitRequest(ctx, createReq)
	services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch

	requestID := createResp.Msg.RequestId
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

	// Try to withdraw from an ACTIVE request (no offers)
	ctx = createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)
	withdrawReq := connect.NewRequest(&api.WithdrawOfferRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	_, err := service.WithdrawOffer(ctx, withdrawReq)

	if err == nil {
		t.Fatal("Expected error when withdrawing from ACTIVE request")
	}

	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("Expected connect.Error, got %T", err)
	}

	// Since the user never offered, the error should be NotFound (not in offerers list)
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("Expected NotFound error (user not in offerers list), got %v", connectErr.Code())
	}
}

// TestService_OfferToFulfill_RequiresCommunityId verifies that empty community_id
// returns InvalidArgument (not PermissionDenied). This prevents misleading error
// messages when the community_id is missing.
func TestService_OfferToFulfill_RequiresCommunityId(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID := setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offererID)

	// Create a request
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createReq := connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Looking for a power drill",
	})
	createResp, _ := service.SubmitRequest(ctx, createReq)
	services.WaitForStockImagery(t, stockImageryDone)

	requestID := createResp.Msg.RequestId
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

	// Try to offer with empty community_id
	ctx = createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)
	offerReq := connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: "", // Empty community_id
	})

	_, err := service.OfferToFulfill(ctx, offerReq)
	if err == nil {
		t.Fatal("Expected error when offering with empty community_id")
	}

	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("Expected connect.Error, got %T", err)
	}

	// Should return InvalidArgument, not PermissionDenied
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
	}

	// Verify error message mentions community_id
	if connectErr.Message() == "" || !contains(connectErr.Message(), "community_id") {
		t.Errorf("Expected error message to mention 'community_id', got: %s", connectErr.Message())
	}
}

// TestService_OfferToFulfill_RequiresRequestId verifies that empty request_id
// returns InvalidArgument error.
func TestService_OfferToFulfill_RequiresRequestId(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID := setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offererID)

	// Try to offer with empty request_id
	ctx := createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)
	offerReq := connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   "", // Empty request_id
		CommunityId: communityID,
	})

	_, err := service.OfferToFulfill(ctx, offerReq)
	if err == nil {
		t.Fatal("Expected error when offering with empty request_id")
	}

	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("Expected connect.Error, got %T", err)
	}

	// Should return InvalidArgument
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
	}

	// Verify error message mentions request_id
	if connectErr.Message() == "" || !contains(connectErr.Message(), "request_id") {
		t.Errorf("Expected error message to mention 'request_id', got: %s", connectErr.Message())
	}
}

// contains checks if a string contains a substring (case-insensitive helper for tests).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
