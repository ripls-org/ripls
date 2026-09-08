package chat

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestGetConversationForTransfer_Success(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender")
	createTestUsers(t, sqlStorage, "borrower1", "Borrower")

	loanID := createTestLoanWithCommunity(t, sqlStorage, "lender1", "borrower1", communityID)

	// Create conversation
	ctxLender := contextWithAuth("lender1", "lender@example.com")
	startReq := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})

	startResp, err := svc.StartConversation(ctxLender, startReq)
	if err != nil {
		t.Fatalf("Failed to start conversation: %v", err)
	}

	// Update Gear with conversation_id (needed for GetConversationForTransfer).
	transfer := &models.Transfer{}
	if err := sqlStorage.GetByID(context.Background(), loanID, transfer); err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}
	gear := &models.Gear{}
	if err := sqlStorage.GetByID(context.Background(), transfer.GearId, gear); err != nil {
		t.Fatalf("Failed to get gear: %v", err)
	}
	gear.ConversationId = startResp.Msg.ConversationId
	if err := sqlStorage.Update(context.Background(), gear); err != nil {
		t.Fatalf("Failed to update gear conversation_id: %v", err)
	}

	// Get conversation by loan ID
	getReq := connect.NewRequest(&api.GetConversationForTransferRequest{
		TransferId: loanID,
	})

	getResp, err := svc.GetConversationForTransfer(ctxLender, getReq)
	if err != nil {
		t.Fatalf("GetConversationForTransfer failed: %v", err)
	}

	// Verify conversation ID matches
	if getResp.Msg.Conversation.ConversationId != startResp.Msg.ConversationId {
		t.Errorf("Expected conversation ID %s, got %s", startResp.Msg.ConversationId, getResp.Msg.Conversation.ConversationId)
	}

	// Verify response includes all necessary fields
	if getResp.Msg.Conversation.CommunityId != communityID {
		t.Errorf("Expected community ID %s, got %s", communityID, getResp.Msg.Conversation.CommunityId)
	}
	if len(getResp.Msg.Conversation.Participants) != 2 {
		t.Errorf("Expected 2 participants, got %d", len(getResp.Msg.Conversation.Participants))
	}
	if getResp.Msg.Conversation.GetTopic().GetTransferId() != loanID {
		t.Errorf("Expected transfer ID %s, got %s", loanID, getResp.Msg.Conversation.GetTopic().GetTransferId())
	}
}

func TestGetConversationForTransfer_NotFound(t *testing.T) {
	svc, _ := setupTestChatService(t)

	ctx := contextWithAuth("user1", "user1@example.com")
	req := connect.NewRequest(&api.GetConversationForTransferRequest{
		TransferId: "nonexistent-loan",
	})

	_, err := svc.GetConversationForTransfer(ctx, req)
	if err == nil {
		t.Fatal("Expected error when conversation doesn't exist")
	}

	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("Expected NotFound error, got: %v", err)
	}
}

func TestGetConversationForTransfer_NonParticipantDenied(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")

	// Create conversation
	ctxLender := contextWithAuth("lender1", "lender@example.com")
	startReq := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})

	_, err := svc.StartConversation(ctxLender, startReq)
	if err != nil {
		t.Fatalf("Failed to start conversation: %v", err)
	}

	// Non-participant tries to get conversation
	ctxStranger := contextWithAuth("stranger", "stranger@example.com")
	getReq := connect.NewRequest(&api.GetConversationForTransferRequest{
		TransferId: loanID,
	})

	_, err = svc.GetConversationForTransfer(ctxStranger, getReq)
	if err == nil {
		t.Fatal("Expected error when non-participant tries to get conversation")
	}

	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("Expected PermissionDenied error, got: %v", err)
	}
}

func TestGetConversationForRequest_Success(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and members
	communityID := createTestCommunity(t, sqlStorage, "requester1")
	createTestCommunityMembership(t, sqlStorage, communityID, "requester1")
	createTestCommunityMembership(t, sqlStorage, communityID, "offerer1")

	requestID := createTestRequest(t, sqlStorage, "requester1", communityID)

	// Create conversation
	ctxRequester := contextWithAuth("requester1", "requester@example.com")
	startReq := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_RequestId{RequestId: requestID}},
	})

	startResp, err := svc.StartConversation(ctxRequester, startReq)
	if err != nil {
		t.Fatalf("Failed to start conversation: %v", err)
	}

	// Add requester as participant (request conversations start with no participants)
	// This simulates what the request service does when someone offers to fulfill
	conversation := &models.ChatConversation{}
	if err := sqlStorage.GetByID(context.Background(), startResp.Msg.ConversationId, conversation); err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}
	conversation.ParticipantIds = append(conversation.ParticipantIds, "requester1", "offerer1")
	if err := sqlStorage.Update(context.Background(), conversation); err != nil {
		t.Fatalf("Failed to update conversation: %v", err)
	}

	// Get conversation by request ID
	getReq := connect.NewRequest(&api.GetConversationForRequestRequest{
		RequestId: requestID,
	})

	getResp, err := svc.GetConversationForRequest(ctxRequester, getReq)
	if err != nil {
		t.Fatalf("GetConversationForRequest failed: %v", err)
	}

	// Verify conversation ID matches
	if getResp.Msg.Conversation.ConversationId != startResp.Msg.ConversationId {
		t.Errorf("Expected conversation ID %s, got %s", startResp.Msg.ConversationId, getResp.Msg.Conversation.ConversationId)
	}

	// Verify response includes all necessary fields
	if getResp.Msg.Conversation.CommunityId != communityID {
		t.Errorf("Expected community ID %s, got %s", communityID, getResp.Msg.Conversation.CommunityId)
	}
	if getResp.Msg.Conversation.GetTopic().GetRequestId() != requestID {
		t.Errorf("Expected request ID %s, got %s", requestID, getResp.Msg.Conversation.GetTopic().GetRequestId())
	}
}

func TestGetConversationForRequest_NotFound(t *testing.T) {
	svc, _ := setupTestChatService(t)

	ctx := contextWithAuth("user1", "user1@example.com")
	req := connect.NewRequest(&api.GetConversationForRequestRequest{
		RequestId: "nonexistent-request",
	})

	_, err := svc.GetConversationForRequest(ctx, req)
	if err == nil {
		t.Fatal("Expected error when conversation doesn't exist")
	}

	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("Expected NotFound error, got: %v", err)
	}
}

func TestGetConversationForRequest_NonParticipantDenied(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and members
	communityID := createTestCommunity(t, sqlStorage, "requester1")
	createTestCommunityMembership(t, sqlStorage, communityID, "requester1")

	requestID := createTestRequest(t, sqlStorage, "requester1", communityID)

	// Create conversation
	ctxRequester := contextWithAuth("requester1", "requester@example.com")
	startReq := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_RequestId{RequestId: requestID}},
	})

	startResp, err := svc.StartConversation(ctxRequester, startReq)
	if err != nil {
		t.Fatalf("Failed to start conversation: %v", err)
	}

	// Add requester as participant (so we can test that stranger is NOT a participant)
	conversation := &models.ChatConversation{}
	if err := sqlStorage.GetByID(context.Background(), startResp.Msg.ConversationId, conversation); err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}
	conversation.ParticipantIds = []string{"requester1"}
	if err := sqlStorage.Update(context.Background(), conversation); err != nil {
		t.Fatalf("Failed to update conversation: %v", err)
	}

	// Non-participant tries to get conversation
	ctxStranger := contextWithAuth("stranger", "stranger@example.com")
	getReq := connect.NewRequest(&api.GetConversationForRequestRequest{
		RequestId: requestID,
	})

	_, err = svc.GetConversationForRequest(ctxStranger, getReq)
	if err == nil {
		t.Fatal("Expected error when non-participant tries to get conversation")
	}

	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("Expected PermissionDenied error, got: %v", err)
	}
}

func TestGetConversationForTransfer_IncludesLastMessagePreview(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender User")
	createTestUsers(t, sqlStorage, "borrower1", "Borrower User")

	loanID := createTestLoanWithCommunity(t, sqlStorage, "lender1", "borrower1", communityID)

	// Create conversation
	ctxLender := contextWithAuth("lender1", "lender@example.com")
	startReq := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})

	startResp, err := svc.StartConversation(ctxLender, startReq)
	if err != nil {
		t.Fatalf("Failed to start conversation: %v", err)
	}

	// Update Gear with conversation_id (needed for GetConversationForTransfer)
	// Get the transfer to find gear_id
	transfer := &models.Transfer{}
	if err := sqlStorage.GetByID(context.Background(), loanID, transfer); err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}
	gear := &models.Gear{}
	if err := sqlStorage.GetByID(context.Background(), transfer.GearId, gear); err != nil {
		t.Fatalf("Failed to get gear: %v", err)
	}
	gear.ConversationId = startResp.Msg.ConversationId
	if err := sqlStorage.Update(context.Background(), gear); err != nil {
		t.Fatalf("Failed to update gear conversation_id: %v", err)
	}

	// Send a message
	sendReq := connect.NewRequest(&api.SendMessageRequest{
		ConversationId: startResp.Msg.ConversationId,
		Text:           "Sure, when do you need it?",
	})

	_, err = svc.SendMessage(ctxLender, sendReq)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Get conversation
	getReq := connect.NewRequest(&api.GetConversationForTransferRequest{
		TransferId: loanID,
	})

	getResp, err := svc.GetConversationForTransfer(ctxLender, getReq)
	if err != nil {
		t.Fatalf("GetConversationForTransfer failed: %v", err)
	}

	conv := getResp.Msg.Conversation

	// Verify last message text is populated
	if conv.LastMessageText != "Sure, when do you need it?" {
		t.Errorf("Expected last message text 'Sure, when do you need it?', got '%s'", conv.LastMessageText)
	}

	// Verify last message sender is populated
	if conv.LastMessageSender == nil {
		t.Fatal("Expected last message sender to be populated")
	}

	if conv.LastMessageSender.Id != "lender1" {
		t.Errorf("Expected last message sender ID 'lender1', got '%s'", conv.LastMessageSender.Id)
	}
}
