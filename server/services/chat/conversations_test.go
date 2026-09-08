package chat

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestStartConversation_NonParticipantCannotStart(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and add lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")
	createTestCommunityMembership(t, sqlStorage, communityID, "stranger")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")

	// User who is not lender or borrower tries to start conversation
	ctx := contextWithAuth("stranger", "stranger@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})

	_, err := svc.StartConversation(ctx, req)
	if err == nil {
		t.Fatal("Expected error when non-participant tries to start conversation")
	}

	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("Expected PermissionDenied error, got: %v", err)
	}
}

func TestStartConversation_LenderCanStart(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and add lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender")
	createTestUsers(t, sqlStorage, "borrower1", "Borrower")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")

	ctx := contextWithAuth("lender1", "lender@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})

	resp, err := svc.StartConversation(ctx, req)
	if err != nil {
		t.Fatalf("Lender should be able to start conversation: %v", err)
	}

	if resp.Msg.ConversationId == "" {
		t.Error("Expected conversation ID to be returned")
	}

	if len(resp.Msg.Participants) != 2 {
		t.Errorf("Expected 2 participants, got %d", len(resp.Msg.Participants))
	}
}

func TestStartConversation_BorrowerCanStart(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and add lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")

	ctx := contextWithAuth("borrower1", "borrower@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})

	resp, err := svc.StartConversation(ctx, req)
	if err != nil {
		t.Fatalf("Borrower should be able to start conversation: %v", err)
	}

	if resp.Msg.ConversationId == "" {
		t.Error("Expected conversation ID to be returned")
	}
}

func TestStartConversation_CreatesNewConversation(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and add lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")

	ctx := contextWithAuth("lender1", "lender@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})

	resp, err := svc.StartConversation(ctx, req)
	if err != nil {
		t.Fatalf("Failed to start conversation: %v", err)
	}

	// Verify conversation was created in storage
	conversation := &models.ChatConversation{}
	err = sqlStorage.GetByID(context.Background(), resp.Msg.ConversationId, conversation)
	if err != nil {
		t.Fatalf("Failed to get conversation from storage: %v", err)
	}

	if conversation.GetTopic().GetTransferId() != loanID {
		t.Errorf("Expected transfer ID %s, got %s", loanID, conversation.GetTopic().GetTransferId())
	}

	if len(conversation.ParticipantIds) != 2 {
		t.Errorf("Expected 2 participants, got %d", len(conversation.ParticipantIds))
	}
}

func TestStartConversation_ReturnsExistingConversation(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and add lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")

	ctx := contextWithAuth("lender1", "lender@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})

	// Start conversation first time
	resp1, err := svc.StartConversation(ctx, req)
	if err != nil {
		t.Fatalf("Failed to start conversation: %v", err)
	}

	// Start conversation again
	resp2, err := svc.StartConversation(ctx, req)
	if err != nil {
		t.Fatalf("Failed to start conversation second time: %v", err)
	}

	// Should return same conversation ID
	if resp1.Msg.ConversationId != resp2.Msg.ConversationId {
		t.Errorf("Expected same conversation ID, got %s and %s",
			resp1.Msg.ConversationId, resp2.Msg.ConversationId)
	}
}

func TestStartConversation_StoresCorrectParticipants(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and add lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender")
	createTestUsers(t, sqlStorage, "borrower1", "Borrower")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")

	ctx := contextWithAuth("lender1", "lender@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})

	resp, err := svc.StartConversation(ctx, req)
	if err != nil {
		t.Fatalf("Failed to start conversation: %v", err)
	}

	// Verify participants
	participants := resp.Msg.Participants
	if len(participants) != 2 {
		t.Fatalf("Expected 2 participants, got %d", len(participants))
	}

	hasLender := false
	hasBorrower := false
	for _, user := range participants {
		if user.Id == "lender1" {
			hasLender = true
		}
		if user.Id == "borrower1" {
			hasBorrower = true
		}
	}

	if !hasLender || !hasBorrower {
		t.Error("Participants should include both lender and borrower")
	}
}

func TestStartConversation_NonCommunityMemberCannotStart(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community with lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")

	// Non-member tries to start conversation
	ctx := contextWithAuth("nonmember", "nonmember@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})

	_, err := svc.StartConversation(ctx, req)
	if err == nil {
		t.Fatal("Expected error when non-community-member tries to start conversation")
	}

	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("Expected PermissionDenied error, got: %v", err)
	}
}

func TestStartConversation_RequiresCommunityID(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")

	// Try to start conversation without community_id
	ctx := contextWithAuth("lender1", "lender@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		Topic: &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})

	_, err := svc.StartConversation(ctx, req)
	if err == nil {
		t.Fatal("Expected error when community_id is not provided")
	}

	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("Expected InvalidArgument error, got: %v", err)
	}
}
