package chat

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestListConversations_ReturnsCommunityID(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender")
	createTestUsers(t, sqlStorage, "borrower1", "Borrower")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")

	// Create conversation
	ctx := contextWithAuth("lender1", "lender@example.com")
	startReq := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})

	startResp, err := svc.StartConversation(ctx, startReq)
	if err != nil {
		t.Fatalf("Failed to start conversation: %v", err)
	}

	// Send a message so the conversation appears in the list
	sendReq := connect.NewRequest(&api.SendMessageRequest{
		ConversationId: startResp.Msg.ConversationId,
		Text:           "Test message",
	})

	_, err = svc.SendMessage(ctx, sendReq)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// List conversations
	listReq := connect.NewRequest(&api.ListConversationsRequest{})
	listResp, err := svc.ListConversations(ctx, listReq)
	if err != nil {
		t.Fatalf("ListConversations failed: %v", err)
	}

	if len(listResp.Msg.Conversations) != 1 {
		t.Fatalf("Expected 1 conversation, got %d", len(listResp.Msg.Conversations))
	}

	// Verify community_id is returned
	if listResp.Msg.Conversations[0].CommunityId != communityID {
		t.Errorf("Expected community ID %s, got %s", communityID, listResp.Msg.Conversations[0].CommunityId)
	}
}

func TestListConversations_ExcludesEmptyConversations(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender")
	createTestUsers(t, sqlStorage, "borrower1", "Borrower")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")

	// Create conversation but don't send any messages
	ctx := contextWithAuth("lender1", "lender@example.com")
	startReq := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})

	startResp, err := svc.StartConversation(ctx, startReq)
	if err != nil {
		t.Fatalf("Failed to start conversation: %v", err)
	}

	// List conversations - should be empty because no messages sent
	listReq := connect.NewRequest(&api.ListConversationsRequest{})
	listResp, err := svc.ListConversations(ctx, listReq)
	if err != nil {
		t.Fatalf("ListConversations failed: %v", err)
	}

	if len(listResp.Msg.Conversations) != 0 {
		t.Fatalf("Expected 0 conversations (no messages sent), got %d", len(listResp.Msg.Conversations))
	}

	// Now send a message
	sendReq := connect.NewRequest(&api.SendMessageRequest{
		ConversationId: startResp.Msg.ConversationId,
		Text:           "First message",
	})

	_, err = svc.SendMessage(ctx, sendReq)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// List conversations again - should now have 1 conversation
	listResp, err = svc.ListConversations(ctx, listReq)
	if err != nil {
		t.Fatalf("ListConversations failed: %v", err)
	}

	if len(listResp.Msg.Conversations) != 1 {
		t.Fatalf("Expected 1 conversation (after message sent), got %d", len(listResp.Msg.Conversations))
	}

	if listResp.Msg.Conversations[0].ConversationId != startResp.Msg.ConversationId {
		t.Errorf("Expected conversation ID %s, got %s", startResp.Msg.ConversationId, listResp.Msg.Conversations[0].ConversationId)
	}
}

func TestListConversations_IncludesLastMessagePreview(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender User")
	createTestUsers(t, sqlStorage, "borrower1", "Borrower User")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")

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

	// Send a message
	sendReq := connect.NewRequest(&api.SendMessageRequest{
		ConversationId: startResp.Msg.ConversationId,
		Text:           "Hey, can I borrow this item?",
	})

	_, err = svc.SendMessage(ctxLender, sendReq)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// List conversations
	listReq := connect.NewRequest(&api.ListConversationsRequest{})
	listResp, err := svc.ListConversations(ctxLender, listReq)
	if err != nil {
		t.Fatalf("ListConversations failed: %v", err)
	}

	if len(listResp.Msg.Conversations) != 1 {
		t.Fatalf("Expected 1 conversation, got %d", len(listResp.Msg.Conversations))
	}

	conv := listResp.Msg.Conversations[0]

	// Verify last message text is populated
	if conv.LastMessageText != "Hey, can I borrow this item?" {
		t.Errorf("Expected last message text 'Hey, can I borrow this item?', got '%s'", conv.LastMessageText)
	}

	// Verify last message sender is populated
	if conv.LastMessageSender == nil {
		t.Fatal("Expected last message sender to be populated")
	}

	if conv.LastMessageSender.Id != "lender1" {
		t.Errorf("Expected last message sender ID 'lender1', got '%s'", conv.LastMessageSender.Id)
	}

	if conv.LastMessageSender.Name != "Lender User" {
		t.Errorf("Expected last message sender name 'Lender User', got '%s'", conv.LastMessageSender.Name)
	}
}

// TestListConversations_DecodesMentionsInLastMessageText verifies that encoded
// @-mentions in the most recent message are decoded to "@Display Name" form
// before being surfaced as the conversation preview. Regression test for #1668.
func TestListConversations_DecodesMentionsInLastMessageText(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender User")
	createTestUsers(t, sqlStorage, "borrower1", "Borrower User")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")

	ctxLender := contextWithAuth("lender1", "lender@example.com")
	startResp, err := svc.StartConversation(ctxLender, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	}))
	if err != nil {
		t.Fatalf("Failed to start conversation: %v", err)
	}

	_, err = svc.SendMessage(ctxLender, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: startResp.Msg.ConversationId,
		Text:           "Hey @[user:borrower1:Borrower User], the @[loan:loan-x:Drill] is ready",
	}))
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	listResp, err := svc.ListConversations(ctxLender, connect.NewRequest(&api.ListConversationsRequest{}))
	if err != nil {
		t.Fatalf("ListConversations failed: %v", err)
	}
	if len(listResp.Msg.Conversations) != 1 {
		t.Fatalf("Expected 1 conversation, got %d", len(listResp.Msg.Conversations))
	}

	want := "Hey @Borrower User, the @Drill is ready"
	if got := listResp.Msg.Conversations[0].LastMessageText; got != want {
		t.Errorf("LastMessageText = %q, want %q", got, want)
	}
}

func TestListConversations_TruncatesLongMessages(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender User")

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

	// Send a long message (more than 150 characters)
	longMessage := "This is a very long message that exceeds the maximum length of 150 characters. " +
		"It should be truncated when returned in the conversation list. " +
		"This text should not appear in the preview because it's beyond the 150 character limit. " +
		"Even more text to make it really long."

	sendReq := connect.NewRequest(&api.SendMessageRequest{
		ConversationId: startResp.Msg.ConversationId,
		Text:           longMessage,
	})

	_, err = svc.SendMessage(ctxLender, sendReq)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// List conversations
	listReq := connect.NewRequest(&api.ListConversationsRequest{})
	listResp, err := svc.ListConversations(ctxLender, listReq)
	if err != nil {
		t.Fatalf("ListConversations failed: %v", err)
	}

	conv := listResp.Msg.Conversations[0]

	// Verify message is truncated to 150 characters (counting runes, not bytes)
	if len([]rune(conv.LastMessageText)) > 150 {
		t.Errorf("Expected last message text to be truncated to 150 characters, got %d", len([]rune(conv.LastMessageText)))
	}

	// Verify it's not empty
	if conv.LastMessageText == "" {
		t.Error("Expected last message text to not be empty")
	}

	// Verify it starts with the beginning of the message
	if conv.LastMessageText[:20] != longMessage[:20] {
		t.Errorf("Expected truncated message to start with '%s', got '%s'", longMessage[:20], conv.LastMessageText[:20])
	}
}

func TestListConversations_ArchiveFiltering(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender")
	createTestUsers(t, sqlStorage, "borrower1", "Borrower")

	// Create an active transfer (RECIPIENT_SELECTED) and a completed transfer
	activeTransferID := createTestTransferWithState(t, sqlStorage, "lender1", "borrower1", models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED)
	completedTransferID := createTestTransferWithState(t, sqlStorage, "lender1", "borrower1", models.TransferState_TRANSFER_STATE_COMPLETED)

	ctxLender := contextWithAuth("lender1", "lender@example.com")

	// Create conversations for both transfers
	activeConvResp, err := svc.StartConversation(ctxLender, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: activeTransferID}},
	}))
	if err != nil {
		t.Fatalf("Failed to start active conversation: %v", err)
	}

	completedConvResp, err := svc.StartConversation(ctxLender, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: completedTransferID}},
	}))
	if err != nil {
		t.Fatalf("Failed to start completed conversation: %v", err)
	}

	// Send messages to both conversations so they appear in the list
	_, err = svc.SendMessage(ctxLender, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: activeConvResp.Msg.ConversationId,
		Text:           "Message in active conversation",
	}))
	if err != nil {
		t.Fatalf("Failed to send message to active conversation: %v", err)
	}

	_, err = svc.SendMessage(ctxLender, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: completedConvResp.Msg.ConversationId,
		Text:           "Message in completed conversation",
	}))
	if err != nil {
		t.Fatalf("Failed to send message to completed conversation: %v", err)
	}

	// Mark messages as read for lender so the completed conversation has 0 unread
	_, err = svc.MarkMessagesRead(ctxLender, connect.NewRequest(&api.MarkMessagesReadRequest{
		ConversationId: completedConvResp.Msg.ConversationId,
	}))
	if err != nil {
		t.Fatalf("Failed to mark completed conversation as read: %v", err)
	}

	t.Run("active conversations (archived=false)", func(t *testing.T) {
		listReq := connect.NewRequest(&api.ListConversationsRequest{Archived: false})
		listResp, err := svc.ListConversations(ctxLender, listReq)
		if err != nil {
			t.Fatalf("ListConversations failed: %v", err)
		}

		// Should only include the active transfer conversation
		if len(listResp.Msg.Conversations) != 1 {
			t.Fatalf("Expected 1 active conversation, got %d", len(listResp.Msg.Conversations))
		}

		conv := listResp.Msg.Conversations[0]
		if conv.GetTopic().GetTransferId() != activeTransferID {
			t.Errorf("Expected active transfer ID %s, got %s", activeTransferID, conv.GetTopic().GetTransferId())
		}
		if conv.IsItemDone {
			t.Error("Expected IsItemDone to be false for active transfer")
		}
	})

	t.Run("archived conversations (archived=true)", func(t *testing.T) {
		listReq := connect.NewRequest(&api.ListConversationsRequest{Archived: true})
		listResp, err := svc.ListConversations(ctxLender, listReq)
		if err != nil {
			t.Fatalf("ListConversations failed: %v", err)
		}

		// Should only include the completed transfer conversation (no unread messages)
		if len(listResp.Msg.Conversations) != 1 {
			t.Fatalf("Expected 1 archived conversation, got %d", len(listResp.Msg.Conversations))
		}

		conv := listResp.Msg.Conversations[0]
		if conv.GetTopic().GetTransferId() != completedTransferID {
			t.Errorf("Expected completed transfer ID %s, got %s", completedTransferID, conv.GetTopic().GetTransferId())
		}
		if !conv.IsItemDone {
			t.Error("Expected IsItemDone to be true for completed transfer")
		}
	})

	t.Run("completed item with unread messages stays in active", func(t *testing.T) {
		// Send a message in the completed conversation (from borrower to create unread for lender)
		// First get the completed conversation ID
		listReq := connect.NewRequest(&api.ListConversationsRequest{Archived: true})
		listResp, err := svc.ListConversations(ctxLender, listReq)
		if err != nil {
			t.Fatalf("ListConversations failed: %v", err)
		}
		completedConvID := listResp.Msg.Conversations[0].ConversationId

		// Send message as borrower
		ctxBorrower := contextWithAuth("borrower1", "borrower@example.com")
		_, err = svc.SendMessage(ctxBorrower, connect.NewRequest(&api.SendMessageRequest{
			ConversationId: completedConvID,
			Text:           "New message after completion",
		}))
		if err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}

		// Now check active conversations for lender - should include the completed one (has unread)
		activeReq := connect.NewRequest(&api.ListConversationsRequest{Archived: false})
		activeResp, err := svc.ListConversations(ctxLender, activeReq)
		if err != nil {
			t.Fatalf("ListConversations failed: %v", err)
		}

		// Should have 2 active: one for active transfer, one for completed with unread
		if len(activeResp.Msg.Conversations) != 2 {
			t.Fatalf("Expected 2 active conversations (one with unread), got %d", len(activeResp.Msg.Conversations))
		}

		// The completed one should have IsItemDone=true but still be in active list
		var foundCompletedInActive bool
		for _, conv := range activeResp.Msg.Conversations {
			if conv.GetTopic().GetTransferId() == completedTransferID {
				foundCompletedInActive = true
				if !conv.IsItemDone {
					t.Error("Expected IsItemDone to be true for completed transfer")
				}
				if conv.UnreadCount == 0 {
					t.Error("Expected UnreadCount > 0 for conversation with unread message")
				}
			}
		}
		if !foundCompletedInActive {
			t.Error("Expected completed conversation with unread to appear in active list")
		}

		// Archived should now be empty for lender
		archivedReq := connect.NewRequest(&api.ListConversationsRequest{Archived: true})
		archivedResp, err := svc.ListConversations(ctxLender, archivedReq)
		if err != nil {
			t.Fatalf("ListConversations failed: %v", err)
		}
		if len(archivedResp.Msg.Conversations) != 0 {
			t.Fatalf("Expected 0 archived conversations for lender (has unread), got %d", len(archivedResp.Msg.Conversations))
		}
	})
}

// TestListConversations_NoPlusOneQueries verifies that ListConversations issues a bounded
// number of queries regardless of the number of conversations (no N+1 regression).
func TestListConversations_NoPlusOneQueries(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender One")
	createTestUsers(t, sqlStorage, "borrower1", "Borrower One")

	ctx := contextWithAuth("lender1", "lender@example.com")

	// Create 3 conversations and send a message in each so they appear in the inbox.
	const numConversations = 3
	for i := 0; i < numConversations; i++ {
		loanID := createTestLoanWithCommunity(t, sqlStorage, "lender1", "borrower1", communityID)

		startResp, err := svc.StartConversation(ctx, connect.NewRequest(&api.StartConversationRequest{
			CommunityId: communityID,
			Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
		}))
		if err != nil {
			t.Fatalf("StartConversation: %v", err)
		}

		_, err = svc.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
			ConversationId: startResp.Msg.ConversationId,
			Text:           "message",
		}))
		if err != nil {
			t.Fatalf("SendMessage: %v", err)
		}
	}

	// Query budget after the indexed ListByParticipant fix:
	// 1 (ListByParticipant) + 1 (batch messages) + 1 (batch transfers) +
	// 1 (batch requests, empty) + 1 (batch users) = 5. Allow max=6 for
	// incidental overhead.
	ctxWithStats := storage.WithQueryStats(ctx)
	storage.AssertMaxQueries(t, ctxWithStats, 6, func() {
		resp, err := svc.ListConversations(ctxWithStats, connect.NewRequest(&api.ListConversationsRequest{}))
		if err != nil {
			t.Fatalf("ListConversations: %v", err)
		}
		if len(resp.Msg.Conversations) != numConversations {
			t.Errorf("want %d conversations, got %d", numConversations, len(resp.Msg.Conversations))
		}
	})
}

// TestListConversations_RowCountAssertion verifies that ListConversations returns only the
// caller's conversations and does not leak other users' conversations, and that the query
// count does not scale with the total number of conversations (N+1 safety).
func TestListConversations_RowCountAssertion(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// user1 is the caller; user2 owns N extra conversations user1 is NOT in.
	const N = 20 // conversations user1 is NOT in
	const M = 3  // conversations user1 IS in

	communityID := createTestCommunity(t, sqlStorage, "user1")
	createTestCommunityMembership(t, sqlStorage, communityID, "user1")
	createTestCommunityMembership(t, sqlStorage, communityID, "user2")
	createTestUsers(t, sqlStorage, "user1", "User One")
	createTestUsers(t, sqlStorage, "user2", "User Two")

	ctx := contextWithAuth("user1", "user1@example.com")
	ctxUser2 := contextWithAuth("user2", "user2@example.com")

	// Create N conversations that user2 owns (user1 is NOT a participant).
	for i := 0; i < N; i++ {
		loanID := createTestLoanWithCommunity(t, sqlStorage, "user2", "user2", communityID)
		startResp, err := svc.StartConversation(ctxUser2, connect.NewRequest(&api.StartConversationRequest{
			CommunityId: communityID,
			Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
		}))
		if err != nil {
			t.Fatalf("StartConversation for user2: %v", err)
		}
		_, err = svc.SendMessage(ctxUser2, connect.NewRequest(&api.SendMessageRequest{
			ConversationId: startResp.Msg.ConversationId,
			Text:           "message",
		}))
		if err != nil {
			t.Fatalf("SendMessage for user2: %v", err)
		}
	}

	// Create M conversations that user1 IS in.
	for i := 0; i < M; i++ {
		loanID := createTestLoanWithCommunity(t, sqlStorage, "user1", "user2", communityID)
		startResp, err := svc.StartConversation(ctx, connect.NewRequest(&api.StartConversationRequest{
			CommunityId: communityID,
			Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
		}))
		if err != nil {
			t.Fatalf("StartConversation for user1: %v", err)
		}
		_, err = svc.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
			ConversationId: startResp.Msg.ConversationId,
			Text:           "message",
		}))
		if err != nil {
			t.Fatalf("SendMessage for user1: %v", err)
		}
	}

	ctxWithStats := storage.WithQueryStats(ctx)
	storage.AssertMaxQueries(t, ctxWithStats, 6, func() {
		resp, err := svc.ListConversations(ctxWithStats, connect.NewRequest(&api.ListConversationsRequest{}))
		if err != nil {
			t.Fatalf("ListConversations: %v", err)
		}
		// Only user1's M conversations should be returned — none of the N user2 conversations.
		if len(resp.Msg.Conversations) != M {
			t.Errorf("want exactly %d conversations (user1's), got %d", M, len(resp.Msg.Conversations))
		}
	})
}

// TestListConversations_DeletedLastMessageSender verifies that
// ConversationItem.LastMessageSender surfaces a former-member placeholder
// (instead of nil) when the most recent sender's user record has been
// soft-deleted, and that participants the caller is not paired with are
// filtered as before. Regression for #1670.
func TestListConversations_DeletedLastMessageSender(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	communityID := createTestCommunity(t, sqlStorage, "owner-lc")
	createTestCommunityMembership(t, sqlStorage, communityID, "owner-lc")
	createTestCommunityMembership(t, sqlStorage, communityID, "ghost-lc")
	createTestUsers(t, sqlStorage, "owner-lc", "Owner LC")
	createTestUsers(t, sqlStorage, "ghost-lc", "Ghost LC")

	// Use the borrower as the last-message sender; soft-delete them after
	// they send so the conversation's most recent message is from the
	// now-deleted user.
	loanID := createTestLoan(t, sqlStorage, "owner-lc", "ghost-lc")
	ghostCtx := contextWithAuth("ghost-lc", "ghost-lc@example.com")
	startResp, err := svc.StartConversation(ghostCtx, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	}))
	if err != nil {
		t.Fatalf("StartConversation: %v", err)
	}
	if _, err := svc.SendMessage(ghostCtx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: startResp.Msg.ConversationId,
		Text:           "ghost speaking",
	})); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Soft-delete the sender.
	ghost := &models.User{}
	if err := sqlStorage.GetByID(context.Background(), "ghost-lc", ghost); err != nil {
		t.Fatalf("Load ghost: %v", err)
	}
	ghost.Deleted = &models.DeletedMetadata{DeletedByUserId: ghost.Id, DeletedAtUnixSec: 1}
	if err := sqlStorage.Update(context.Background(), ghost); err != nil {
		t.Fatalf("Soft-delete ghost: %v", err)
	}

	// Owner lists conversations — must succeed and surface the placeholder.
	ownerCtx := contextWithAuth("owner-lc", "owner-lc@example.com")
	resp, err := svc.ListConversations(ownerCtx, connect.NewRequest(&api.ListConversationsRequest{}))
	if err != nil {
		t.Fatalf("ListConversations must tolerate deleted last-message sender; got: %v", err)
	}
	if len(resp.Msg.Conversations) != 1 {
		t.Fatalf("got %d conversations, want 1", len(resp.Msg.Conversations))
	}
	item := resp.Msg.Conversations[0]
	if item.LastMessageSender == nil {
		t.Fatal("LastMessageSender must be a placeholder, not nil")
	}
	if !item.LastMessageSender.FormerMember || item.LastMessageSender.Id != "ghost-lc" {
		t.Errorf("LastMessageSender = %+v, want FormerMember=true and Id=ghost-lc", item.LastMessageSender)
	}
	if item.LastMessageSender.Name != "" {
		t.Errorf("LastMessageSender.Name = %q, want empty (no PII)", item.LastMessageSender.Name)
	}
}
