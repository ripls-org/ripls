package chat

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/clock"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// TestSendMessage_NonParticipantDeniedForTransfer verifies that only conversation
// participants can send messages in transfer-topic conversations. Community members
// who are not participants are denied (mirrors requireConversationAccess semantics).
func TestSendMessage_NonParticipantDeniedForTransfer(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")
	createTestCommunityMembership(t, sqlStorage, communityID, "stranger")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender")
	createTestUsers(t, sqlStorage, "stranger", "Stranger")

	ctx := contextWithAuth("lender1", "lender@example.com")
	resp, err := svc.StartConversation(ctx, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	}))
	if err != nil {
		t.Fatalf("StartConversation failed: %v", err)
	}

	// Participant (lender) can send.
	if _, err := svc.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: resp.Msg.ConversationId,
		Text:           "Hello from lender",
	})); err != nil {
		t.Errorf("expected participant to be able to send, got: %v", err)
	}

	// Non-participant community member is denied for transfer conversations.
	strangerCtx := contextWithAuth("stranger", "stranger@example.com")
	_, err = svc.SendMessage(strangerCtx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: resp.Msg.ConversationId,
		Text:           "Hello from stranger",
	}))
	if err == nil {
		t.Fatal("expected PermissionDenied for non-participant in transfer conversation, got nil")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", connect.CodeOf(err))
	}
}

func TestSendMessage_StoresMessageCorrectly(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and add lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender")

	ctx := contextWithAuth("lender1", "lender@example.com")
	startReq := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})
	startResp, _ := svc.StartConversation(ctx, startReq)

	sendReq := connect.NewRequest(&api.SendMessageRequest{
		ConversationId: startResp.Msg.ConversationId,
		Text:           "Hello from lender",
	})

	sendResp, err := svc.SendMessage(ctx, sendReq)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Verify message was stored
	message := &models.ChatMessage{}
	err = sqlStorage.GetByID(context.Background(), sendResp.Msg.MessageId, message)
	if err != nil {
		t.Fatalf("Failed to get message from storage: %v", err)
	}

	// Verify it's a user message with correct content
	userMsg, ok := message.Message.(*models.ChatMessage_UserMessage)
	if !ok {
		t.Fatalf("Expected UserMessage, got %T", message.Message)
	}

	if userMsg.UserMessage.Text != "Hello from lender" {
		t.Errorf("Expected text 'Hello from lender', got '%s'", userMsg.UserMessage.Text)
	}

	if userMsg.UserMessage.SenderId != "lender1" {
		t.Errorf("Expected sender ID 'lender1', got '%s'", userMsg.UserMessage.SenderId)
	}
}

func TestSendMessage_UpdatesConversationTimestamp(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and add lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender")

	ctx := contextWithAuth("lender1", "lender@example.com")
	startReq := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})
	startResp, _ := svc.StartConversation(ctx, startReq)

	// Get initial conversation state
	conversation := &models.ChatConversation{}
	if err := sqlStorage.GetByID(context.Background(), startResp.Msg.ConversationId, conversation); err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}
	initialTimestamp := conversation.LastMessageAtUnixSec

	// Send message with a future simulation time so the timestamp is always strictly greater.
	laterCtx := clock.WithSimulationTime(ctx, time.Now().Add(2*time.Second))
	sendReq := connect.NewRequest(&api.SendMessageRequest{
		ConversationId: startResp.Msg.ConversationId,
		Text:           "Test message",
	})
	if _, err := svc.SendMessage(laterCtx, sendReq); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Verify timestamp was updated
	if err := sqlStorage.GetByID(context.Background(), startResp.Msg.ConversationId, conversation); err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}
	if conversation.LastMessageAtUnixSec <= initialTimestamp {
		t.Error("Expected conversation timestamp to be updated")
	}
}

func TestSendMessage_EmptyMessageRejected(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and add lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")

	ctx := contextWithAuth("lender1", "lender@example.com")
	startReq := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})
	startResp, _ := svc.StartConversation(ctx, startReq)

	// Try to send empty message
	sendReq := connect.NewRequest(&api.SendMessageRequest{
		ConversationId: startResp.Msg.ConversationId,
		Text:           "",
	})

	_, err := svc.SendMessage(ctx, sendReq)
	if err == nil {
		t.Fatal("Expected error when sending empty message")
	}

	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("Expected InvalidArgument error, got: %v", err)
	}
}

func TestSendMessage_NonMemberRejected(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community with lender1 and borrower1 as members; outsider is NOT a member.
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender")

	// lender1 starts the conversation so it exists in storage.
	ctx := contextWithAuth("lender1", "lender@example.com")
	startResp, err := svc.StartConversation(ctx, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	}))
	if err != nil {
		t.Fatalf("StartConversation: %v", err)
	}
	convID := startResp.Msg.ConversationId

	// Capture the baseline message count (StartConversation inserts a system message).
	baseline, err := sqlStorage.QueryByField(context.Background(), "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("QueryByField baseline: %v", err)
	}
	baselineCount := len(baseline)

	// outsider is authenticated but is neither a participant nor a member of any
	// community the conversation is shared with.
	createTestUsers(t, sqlStorage, "outsider", "Outsider")
	outsiderCtx := contextWithAuth("outsider", "outsider@example.com")

	_, sendErr := svc.SendMessage(outsiderCtx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: convID,
		Text:           "injected message",
	}))
	if sendErr == nil {
		t.Fatal("expected SendMessage to fail for non-member caller, got nil error")
	}
	if connect.CodeOf(sendErr) != connect.CodePermissionDenied {
		t.Errorf("expected CodePermissionDenied, got %v", connect.CodeOf(sendErr))
	}

	// Confirm no message was inserted.
	after, err := sqlStorage.QueryByField(context.Background(), "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("QueryByField after: %v", err)
	}
	if len(after) != baselineCount {
		t.Errorf("expected %d messages after rejected send, got %d", baselineCount, len(after))
	}
}

func TestGetConversationHistory_ReturnsMessagesInOrder(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and add lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender")

	ctx := contextWithAuth("lender1", "lender@example.com")
	startReq := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})
	startResp, _ := svc.StartConversation(ctx, startReq)

	// Send multiple messages with distinct simulated timestamps to ensure ordering.
	messages := []string{"First message", "Second message", "Third message"}
	baseTime := time.Now()
	for i, text := range messages {
		msgCtx := clock.WithSimulationTime(ctx, baseTime.Add(time.Duration(i+1)*time.Second))
		sendReq := connect.NewRequest(&api.SendMessageRequest{
			ConversationId: startResp.Msg.ConversationId,
			Text:           text,
		})
		if _, err := svc.SendMessage(msgCtx, sendReq); err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}
	}

	// Get history
	historyReq := connect.NewRequest(&api.GetConversationHistoryRequest{
		ConversationId: startResp.Msg.ConversationId,
	})
	historyResp, err := svc.GetConversationHistory(ctx, historyReq)
	if err != nil {
		t.Fatalf("Failed to get conversation history: %v", err)
	}

	// Verify messages are in descending order (newest first)
	if len(historyResp.Msg.Messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(historyResp.Msg.Messages))
	}

	// Check the newest message (should be a user message)
	if historyResp.Msg.Messages[0].GetUserMessage() == nil {
		t.Error("Expected first message to be a user message")
	} else if historyResp.Msg.Messages[0].GetUserMessage().Text != "Third message" {
		t.Errorf("Expected newest message first, got: %s", historyResp.Msg.Messages[0].GetUserMessage().Text)
	}
}

func TestGetConversationHistory_PaginationWorks(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and add lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender")

	ctx := contextWithAuth("lender1", "lender@example.com")
	startReq := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})
	startResp, _ := svc.StartConversation(ctx, startReq)

	// Send 5 messages
	for i := 0; i < 5; i++ {
		sendReq := connect.NewRequest(&api.SendMessageRequest{
			ConversationId: startResp.Msg.ConversationId,
			Text:           "Message",
		})
		if _, err := svc.SendMessage(ctx, sendReq); err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}
	}

	// Get first 3 messages
	historyReq := connect.NewRequest(&api.GetConversationHistoryRequest{
		ConversationId: startResp.Msg.ConversationId,
		MaxMessages:    3,
	})
	historyResp, err := svc.GetConversationHistory(ctx, historyReq)
	if err != nil {
		t.Fatalf("Failed to get conversation history: %v", err)
	}

	if len(historyResp.Msg.Messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(historyResp.Msg.Messages))
	}

	if !historyResp.Msg.HasMore {
		t.Error("Expected has_more to be true")
	}
}

func TestMarkMessagesRead_UpdatesReadFlag(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and add lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender")

	// Lender sends message
	lenderCtx := contextWithAuth("lender1", "lender@example.com")
	startReq := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})
	startResp, _ := svc.StartConversation(lenderCtx, startReq)

	sendReq := connect.NewRequest(&api.SendMessageRequest{
		ConversationId: startResp.Msg.ConversationId,
		Text:           "Hello",
	})
	if _, err := svc.SendMessage(lenderCtx, sendReq); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Borrower marks message as read
	borrowerCtx := contextWithAuth("borrower1", "borrower@example.com")
	markReq := connect.NewRequest(&api.MarkMessagesReadRequest{
		ConversationId: startResp.Msg.ConversationId,
	})

	markResp, err := svc.MarkMessagesRead(borrowerCtx, markReq)
	if err != nil {
		t.Fatalf("Failed to mark messages as read: %v", err)
	}

	if markResp.Msg.MessagesMarked != 1 {
		t.Errorf("Expected 1 message marked, got %d", markResp.Msg.MessagesMarked)
	}
}

func TestMarkMessagesRead_DoesNotMarkOwnMessages(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and add lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender")

	// Lender sends message
	lenderCtx := contextWithAuth("lender1", "lender@example.com")
	startReq := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})
	startResp, _ := svc.StartConversation(lenderCtx, startReq)

	sendReq := connect.NewRequest(&api.SendMessageRequest{
		ConversationId: startResp.Msg.ConversationId,
		Text:           "Hello",
	})
	if _, err := svc.SendMessage(lenderCtx, sendReq); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Lender tries to mark their own message as read
	markReq := connect.NewRequest(&api.MarkMessagesReadRequest{
		ConversationId: startResp.Msg.ConversationId,
	})

	markResp, err := svc.MarkMessagesRead(lenderCtx, markReq)
	if err != nil {
		t.Fatalf("Failed to mark messages as read: %v", err)
	}

	// Should not mark any messages (own messages don't count)
	if markResp.Msg.MessagesMarked != 0 {
		t.Errorf("Expected 0 messages marked, got %d", markResp.Msg.MessagesMarked)
	}
}

func TestMarkMessagesRead_PerUserTracking(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create a custom conversation with three participants
	// (In a real scenario, this might be a group loan or a different topic type)
	conversation := &models.ChatConversation{
		Topic:                &models.ConversationTopic{TopicId: &models.ConversationTopic_TransferId{TransferId: "loan123"}},
		ParticipantIds:       []string{"user1", "user2", "user3"},
		CreatedAtUnixSec:     time.Now().Unix(),
		LastMessageAtUnixSec: 0,
	}
	conversationID, err := sqlStorage.Insert(context.Background(), conversation)
	if err != nil {
		t.Fatalf("Failed to create test conversation: %v", err)
	}

	// Also create a fake transfer so authorization checks pass
	transfer := &models.Transfer{
		Id:           "loan123",
		OwnerId:      "user1",
		RecipientId:  "user2",
		GearId:       "gear123",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		CommunityId:  "community123",
	}
	if _, err := sqlStorage.Insert(context.Background(), transfer); err != nil {
		t.Fatalf("Failed to create test transfer: %v", err)
	}

	createTestUsers(t, sqlStorage, "user1", "User One")
	createTestUsers(t, sqlStorage, "user2", "User Two")
	createTestUsers(t, sqlStorage, "user3", "User Three")

	// User1 sends a message
	user1Ctx := contextWithAuth("user1", "user1@example.com")
	sendReq := connect.NewRequest(&api.SendMessageRequest{
		ConversationId: conversationID,
		Text:           "Hello everyone!",
	})
	sendResp, err := svc.SendMessage(user1Ctx, sendReq)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Verify message was created with correct per-user read status
	message := &models.ChatMessage{}
	if err := sqlStorage.GetByID(context.Background(), sendResp.Msg.MessageId, message); err != nil {
		t.Fatalf("Failed to get message: %v", err)
	}

	// User1 (sender) should be marked as read
	if !message.ParticipantIdToIsRead["user1"] {
		t.Error("Expected user1 (sender) to be marked as read")
	}

	// User2 and User3 should NOT be marked as read
	if message.ParticipantIdToIsRead["user2"] {
		t.Error("Expected user2 to NOT be marked as read initially")
	}
	if message.ParticipantIdToIsRead["user3"] {
		t.Error("Expected user3 to NOT be marked as read initially")
	}

	// User2 marks the message as read
	user2Ctx := contextWithAuth("user2", "user2@example.com")
	markReq := connect.NewRequest(&api.MarkMessagesReadRequest{
		ConversationId: conversationID,
	})
	markResp, err := svc.MarkMessagesRead(user2Ctx, markReq)
	if err != nil {
		t.Fatalf("Failed to mark messages as read: %v", err)
	}

	if markResp.Msg.MessagesMarked != 1 {
		t.Errorf("Expected 1 message marked, got %d", markResp.Msg.MessagesMarked)
	}

	// Reload the message and verify read status
	message = &models.ChatMessage{}
	if err := sqlStorage.GetByID(context.Background(), sendResp.Msg.MessageId, message); err != nil {
		t.Fatalf("Failed to get message: %v", err)
	}

	// User1 should still be marked as read
	if !message.ParticipantIdToIsRead["user1"] {
		t.Error("Expected user1 to still be marked as read")
	}

	// User2 should now be marked as read
	if !message.ParticipantIdToIsRead["user2"] {
		t.Error("Expected user2 to be marked as read after marking")
	}

	// User3 should still NOT be marked as read
	if message.ParticipantIdToIsRead["user3"] {
		t.Error("Expected user3 to still NOT be marked as read")
	}

	// Verify User3's view of the conversation shows the message as unread
	user3Ctx := contextWithAuth("user3", "user3@example.com")
	historyReq := connect.NewRequest(&api.GetConversationHistoryRequest{
		ConversationId: conversationID,
	})
	historyResp, err := svc.GetConversationHistory(user3Ctx, historyReq)
	if err != nil {
		t.Fatalf("Failed to get conversation history: %v", err)
	}

	if len(historyResp.Msg.Messages) != 1 {
		t.Fatalf("Expected 1 message in history, got %d", len(historyResp.Msg.Messages))
	}

	if historyResp.Msg.Messages[0].IsRead {
		t.Error("Expected message to be unread for user3")
	}

	// Verify User2's view shows the message as read
	historyResp2, err := svc.GetConversationHistory(user2Ctx, historyReq)
	if err != nil {
		t.Fatalf("Failed to get conversation history: %v", err)
	}

	if len(historyResp2.Msg.Messages) != 1 {
		t.Fatalf("Expected 1 message in history, got %d", len(historyResp2.Msg.Messages))
	}

	if !historyResp2.Msg.Messages[0].IsRead {
		t.Error("Expected message to be read for user2")
	}
}

func TestMarkMessagesRead_ClearsWatchUnread(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	const gearID = "gear-watch-clear-test"

	// Create a gear conversation with userA and userB as participants.
	conversationID, err := sqlStorage.Insert(context.Background(), &models.ChatConversation{
		Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
		ParticipantIds: []string{"userA", "userB"},
	})
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	// userA sends a message (sender must exist in storage).
	createTestUsers(t, sqlStorage, "userA", "User A")
	userACtx := contextWithAuth("userA", "a@example.com")
	_, err = svc.SendMessage(userACtx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: conversationID,
		Text:           "Hello!",
	}))
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Seed userB's gear watch entry in the unread state.
	ws := storage.NewWatchStorage(sqlStorage)
	if err := ws.UpsertWatch(context.Background(), "userB", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, gearID); err != nil {
		t.Fatalf("Failed to upsert watch: %v", err)
	}

	// userB marks all messages read.
	userBCtx := contextWithAuth("userB", "b@example.com")
	_, err = svc.MarkMessagesRead(userBCtx, connect.NewRequest(&api.MarkMessagesReadRequest{
		ConversationId: conversationID,
	}))
	if err != nil {
		t.Fatalf("Failed to mark messages read: %v", err)
	}

	// Watch entry for userB should now be read.
	unread, err := ws.IsWatchedUnread(context.Background(), "userB", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, gearID)
	if err != nil {
		t.Fatalf("Failed to check watch: %v", err)
	}
	if unread {
		t.Error("Expected gear watch to be read after MarkMessagesRead, but it is still unread")
	}
}

func TestMarkMessagesRead_TransferTopic_ClearsGearWatch(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// createTestLoan creates a transfer with GearId: "gear123".
	communityID := createTestCommunity(t, sqlStorage, "userA")
	createTestCommunityMembership(t, sqlStorage, communityID, "userA")
	createTestCommunityMembership(t, sqlStorage, communityID, "userB")
	loanID := createTestLoan(t, sqlStorage, "userA", "userB")

	// Start a transfer-topic conversation.
	createTestUsers(t, sqlStorage, "userA", "User A")
	userACtx := contextWithAuth("userA", "a@example.com")
	startResp, err := svc.StartConversation(userACtx, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	}))
	if err != nil {
		t.Fatalf("Failed to start conversation: %v", err)
	}
	conversationID := startResp.Msg.ConversationId

	// userA sends a message.
	_, err = svc.SendMessage(userACtx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: conversationID,
		Text:           "Returning your gear soon",
	}))
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Seed userB's watch on the underlying gear (GearId from createTestLoan is "gear123").
	ws := storage.NewWatchStorage(sqlStorage)
	if err := ws.UpsertWatch(context.Background(), "userB", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear123"); err != nil {
		t.Fatalf("Failed to upsert watch: %v", err)
	}

	// userB marks all messages read.
	userBCtx := contextWithAuth("userB", "b@example.com")
	_, err = svc.MarkMessagesRead(userBCtx, connect.NewRequest(&api.MarkMessagesReadRequest{
		ConversationId: conversationID,
	}))
	if err != nil {
		t.Fatalf("Failed to mark messages read: %v", err)
	}

	// Watch entry should be cleared via the transfer → gear lookup.
	unread, err := ws.IsWatchedUnread(context.Background(), "userB", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear123")
	if err != nil {
		t.Fatalf("Failed to check watch: %v", err)
	}
	if unread {
		t.Error("Expected gear watch to be read after transfer-topic MarkMessagesRead, but it is still unread")
	}
}

func TestMarkMessagesRead_PartialRead_LeavesWatchUnread(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	const gearID = "gear-partial-read-test"

	conversationID, err := sqlStorage.Insert(context.Background(), &models.ChatConversation{
		Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
		ParticipantIds: []string{"userA", "userB"},
	})
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	createTestUsers(t, sqlStorage, "userA", "User A")
	userACtx := contextWithAuth("userA", "a@example.com")

	baseTime := time.Now()
	t1 := baseTime.Add(time.Second)
	t2 := baseTime.Add(2 * time.Second)

	// Send first message at t1.
	_, err = svc.SendMessage(
		clock.WithSimulationTime(userACtx, t1),
		connect.NewRequest(&api.SendMessageRequest{
			ConversationId: conversationID,
			Text:           "Message one",
		}),
	)
	if err != nil {
		t.Fatalf("Failed to send first message: %v", err)
	}

	// Send second message at t2.
	_, err = svc.SendMessage(
		clock.WithSimulationTime(userACtx, t2),
		connect.NewRequest(&api.SendMessageRequest{
			ConversationId: conversationID,
			Text:           "Message two",
		}),
	)
	if err != nil {
		t.Fatalf("Failed to send second message: %v", err)
	}

	// Seed userB's gear watch entry (unread).
	ws := storage.NewWatchStorage(sqlStorage)
	if err := ws.UpsertWatch(context.Background(), "userB", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, gearID); err != nil {
		t.Fatalf("Failed to upsert watch: %v", err)
	}

	// userB marks only messages up to t1 as read (message two at t2 is still unread).
	userBCtx := contextWithAuth("userB", "b@example.com")
	_, err = svc.MarkMessagesRead(userBCtx, connect.NewRequest(&api.MarkMessagesReadRequest{
		ConversationId: conversationID,
		UpToUnixSec:    t1.Unix(),
	}))
	if err != nil {
		t.Fatalf("Failed to mark messages read: %v", err)
	}

	// Watch entry must remain unread because message two is still unread.
	unread, err := ws.IsWatchedUnread(context.Background(), "userB", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, gearID)
	if err != nil {
		t.Fatalf("Failed to check watch: %v", err)
	}
	if !unread {
		t.Error("Expected gear watch to remain unread when not all messages are read")
	}
}

func TestMarkMessagesRead_CommunityTopic_NoOp(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	const communityID = "community-chat-test"

	// Community row is required so GetCommunitiesWithMembership can find it.
	if _, err := sqlStorage.Insert(context.Background(), &models.Community{
		Id: communityID, OwnerUserId: "userA",
	}); err != nil {
		t.Fatalf("insert community: %v", err)
	}
	for _, uid := range []string{"userA", "userB"} {
		if _, err := sqlStorage.Insert(context.Background(), &models.CommunityUser{
			CommunityId: communityID, UserId: uid,
		}); err != nil {
			t.Fatalf("insert community_user %s: %v", uid, err)
		}
	}

	conversationID, err := sqlStorage.Insert(context.Background(), &models.ChatConversation{
		CommunityId:    communityID,
		Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_CommunityId{CommunityId: communityID}},
		ParticipantIds: []string{"userA", "userB"},
	})
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	createTestUsers(t, sqlStorage, "userA", "User A")
	userACtx := contextWithAuth("userA", "a@example.com")
	_, err = svc.SendMessage(userACtx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: conversationID,
		Text:           "Community announcement",
	}))
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// userB marks messages read — no watch entry should be created or modified.
	userBCtx := contextWithAuth("userB", "b@example.com")
	_, err = svc.MarkMessagesRead(userBCtx, connect.NewRequest(&api.MarkMessagesReadRequest{
		ConversationId: conversationID,
	}))
	if err != nil {
		t.Fatalf("MarkMessagesRead on a community topic should not error: %v", err)
	}

	// Confirm no watch row was created for userB.
	ws := storage.NewWatchStorage(sqlStorage)
	watches, err := ws.GetActiveWatches(context.Background(), "userB")
	if err != nil {
		t.Fatalf("Failed to get watches: %v", err)
	}
	if len(watches) != 0 {
		t.Errorf("Expected no watch rows for community topic, got %d", len(watches))
	}
}

func TestMarkMessagesRead_NoWatchEntry_NoOp(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	const gearID = "gear-no-watch-test"

	conversationID, err := sqlStorage.Insert(context.Background(), &models.ChatConversation{
		Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
		ParticipantIds: []string{"userA", "userB"},
	})
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	createTestUsers(t, sqlStorage, "userA", "User A")
	userACtx := contextWithAuth("userA", "a@example.com")
	_, err = svc.SendMessage(userACtx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: conversationID,
		Text:           "Hello!",
	}))
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// userB has no watch entry for this gear.
	userBCtx := contextWithAuth("userB", "b@example.com")
	_, err = svc.MarkMessagesRead(userBCtx, connect.NewRequest(&api.MarkMessagesReadRequest{
		ConversationId: conversationID,
	}))
	if err != nil {
		t.Fatalf("MarkMessagesRead should succeed even with no watch entry: %v", err)
	}

	// Confirm no watch row was inserted for userB.
	ws := storage.NewWatchStorage(sqlStorage)
	watches, err := ws.GetActiveWatches(context.Background(), "userB")
	if err != nil {
		t.Fatalf("Failed to get watches: %v", err)
	}
	if len(watches) != 0 {
		t.Errorf("Expected no watch row to be created, got %d", len(watches))
	}
}
