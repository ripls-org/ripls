package chat

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// addParticipant adds a user to a conversation's participant list.
func addParticipant(t *testing.T, sqlStorage *storage.ProtoSQLStorage, conversationID, userID string) {
	t.Helper()
	conversation := &models.ChatConversation{}
	if err := sqlStorage.GetByID(context.Background(), conversationID, conversation); err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}
	conversation.ParticipantIds = append(conversation.ParticipantIds, userID)
	if err := sqlStorage.Update(context.Background(), conversation); err != nil {
		t.Fatalf("Failed to update conversation: %v", err)
	}
}

// createConversationWithMessage sets up a conversation via a transfer topic
// and sends one user message. Returns (conversationID, messageID).
func createConversationWithMessage(t *testing.T, svc *Service, sqlStorage *storage.ProtoSQLStorage, senderID, communityID string) (string, string) {
	t.Helper()

	// Create a transfer to anchor the conversation topic.
	// Use senderID as both lender and borrower so the sender is a participant.
	transferID := createTestLoanWithCommunity(t, sqlStorage, senderID, senderID, communityID)

	ctx := contextWithAuth(senderID, senderID+"@example.com")
	startResp, err := svc.StartConversation(ctx, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: transferID}},
	}))
	if err != nil {
		t.Fatalf("Failed to start conversation: %v", err)
	}

	sendResp, err := svc.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: startResp.Msg.ConversationId,
		Text:           "A message to react to",
	}))
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	return startResp.Msg.ConversationId, sendResp.Msg.MessageId
}

func TestAddReaction(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "alice", "Alice")
	createTestUsers(t, sqlStorage, "bob", "Bob")
	communityID := createTestCommunity(t, sqlStorage, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "bob")

	convID, msgID := createConversationWithMessage(t, svc, sqlStorage, "alice", communityID)
	addParticipant(t, sqlStorage, convID, "bob")

	// Bob adds a reaction.
	bobCtx := contextWithAuth("bob", "bob@example.com")
	resp, err := svc.AddReaction(bobCtx, connect.NewRequest(&api.AddReactionRequest{
		ConversationId: convID,
		MessageId:      msgID,
		Emoji:          "👍",
	}))
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	if len(resp.Msg.Reactions) != 1 {
		t.Fatalf("Expected 1 reaction, got %d", len(resp.Msg.Reactions))
	}
	if resp.Msg.Reactions[0].Emoji != "👍" {
		t.Errorf("Expected 👍, got %s", resp.Msg.Reactions[0].Emoji)
	}
	if resp.Msg.Reactions[0].Sender.Id != "bob" {
		t.Errorf("Expected sender bob, got %s", resp.Msg.Reactions[0].Sender.Id)
	}

	// Verify persisted in storage.
	chatMsg := &models.ChatMessage{}
	if err := sqlStorage.GetByID(context.Background(), msgID, chatMsg); err != nil {
		t.Fatalf("Failed to get message: %v", err)
	}
	if len(chatMsg.Reactions) != 1 {
		t.Fatalf("Expected 1 stored reaction, got %d", len(chatMsg.Reactions))
	}
}

func TestAddReaction_ReplacesExisting(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "alice", "Alice")
	communityID := createTestCommunity(t, sqlStorage, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "alice")

	convID, msgID := createConversationWithMessage(t, svc, sqlStorage, "alice", communityID)

	ctx := contextWithAuth("alice", "alice@example.com")

	// Add first reaction.
	_, err := svc.AddReaction(ctx, connect.NewRequest(&api.AddReactionRequest{
		ConversationId: convID,
		MessageId:      msgID,
		Emoji:          "👍",
	}))
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	// Replace with different emoji.
	resp, err := svc.AddReaction(ctx, connect.NewRequest(&api.AddReactionRequest{
		ConversationId: convID,
		MessageId:      msgID,
		Emoji:          "❤️",
	}))
	if err != nil {
		t.Fatalf("AddReaction (replace) failed: %v", err)
	}

	if len(resp.Msg.Reactions) != 1 {
		t.Fatalf("Expected 1 reaction after replace, got %d", len(resp.Msg.Reactions))
	}
	if resp.Msg.Reactions[0].Emoji != "❤️" {
		t.Errorf("Expected ❤️ after replace, got %s", resp.Msg.Reactions[0].Emoji)
	}
}

func TestAddReaction_MultipleUsers(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "alice", "Alice")
	createTestUsers(t, sqlStorage, "bob", "Bob")
	communityID := createTestCommunity(t, sqlStorage, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "bob")

	convID, msgID := createConversationWithMessage(t, svc, sqlStorage, "alice", communityID)
	addParticipant(t, sqlStorage, convID, "bob")

	// Alice reacts.
	aliceCtx := contextWithAuth("alice", "alice@example.com")
	_, err := svc.AddReaction(aliceCtx, connect.NewRequest(&api.AddReactionRequest{
		ConversationId: convID,
		MessageId:      msgID,
		Emoji:          "👍",
	}))
	if err != nil {
		t.Fatalf("Alice AddReaction failed: %v", err)
	}

	// Bob reacts with a different emoji.
	bobCtx := contextWithAuth("bob", "bob@example.com")
	resp, err := svc.AddReaction(bobCtx, connect.NewRequest(&api.AddReactionRequest{
		ConversationId: convID,
		MessageId:      msgID,
		Emoji:          "❤️",
	}))
	if err != nil {
		t.Fatalf("Bob AddReaction failed: %v", err)
	}

	if len(resp.Msg.Reactions) != 2 {
		t.Fatalf("Expected 2 reactions, got %d", len(resp.Msg.Reactions))
	}
}

func TestRemoveReaction(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "alice", "Alice")
	communityID := createTestCommunity(t, sqlStorage, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "alice")

	convID, msgID := createConversationWithMessage(t, svc, sqlStorage, "alice", communityID)

	ctx := contextWithAuth("alice", "alice@example.com")

	// Add then remove.
	_, err := svc.AddReaction(ctx, connect.NewRequest(&api.AddReactionRequest{
		ConversationId: convID,
		MessageId:      msgID,
		Emoji:          "👍",
	}))
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	resp, err := svc.RemoveReaction(ctx, connect.NewRequest(&api.RemoveReactionRequest{
		ConversationId: convID,
		MessageId:      msgID,
	}))
	if err != nil {
		t.Fatalf("RemoveReaction failed: %v", err)
	}

	if len(resp.Msg.Reactions) != 0 {
		t.Fatalf("Expected 0 reactions after remove, got %d", len(resp.Msg.Reactions))
	}

	// Verify persisted.
	chatMsg := &models.ChatMessage{}
	if err := sqlStorage.GetByID(context.Background(), msgID, chatMsg); err != nil {
		t.Fatalf("Failed to get message: %v", err)
	}
	if len(chatMsg.Reactions) != 0 {
		t.Fatalf("Expected 0 stored reactions, got %d", len(chatMsg.Reactions))
	}
}

func TestRemoveReaction_Idempotent(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "alice", "Alice")
	communityID := createTestCommunity(t, sqlStorage, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "alice")

	convID, msgID := createConversationWithMessage(t, svc, sqlStorage, "alice", communityID)

	ctx := contextWithAuth("alice", "alice@example.com")

	// Remove when no reaction exists — should succeed silently.
	resp, err := svc.RemoveReaction(ctx, connect.NewRequest(&api.RemoveReactionRequest{
		ConversationId: convID,
		MessageId:      msgID,
	}))
	if err != nil {
		t.Fatalf("RemoveReaction (no existing) should not fail: %v", err)
	}
	if len(resp.Msg.Reactions) != 0 {
		t.Fatalf("Expected 0 reactions, got %d", len(resp.Msg.Reactions))
	}
}

func TestAddReaction_NonParticipant(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "alice", "Alice")
	createTestUsers(t, sqlStorage, "stranger", "Stranger")
	communityID := createTestCommunity(t, sqlStorage, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "alice")

	convID, msgID := createConversationWithMessage(t, svc, sqlStorage, "alice", communityID)

	strangerCtx := contextWithAuth("stranger", "stranger@example.com")
	_, err := svc.AddReaction(strangerCtx, connect.NewRequest(&api.AddReactionRequest{
		ConversationId: convID,
		MessageId:      msgID,
		Emoji:          "👍",
	}))
	if err == nil {
		t.Fatal("Expected error for non-participant, got nil")
	}

	connectErr, ok := err.(*connect.Error)
	if !ok || connectErr.Code() != connect.CodePermissionDenied {
		t.Errorf("Expected PermissionDenied, got %v", err)
	}
}

func TestAddReaction_InvalidInput(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "alice", "Alice")

	ctx := contextWithAuth("alice", "alice@example.com")

	_, err := svc.AddReaction(ctx, connect.NewRequest(&api.AddReactionRequest{
		ConversationId: "",
		MessageId:      "msg1",
		Emoji:          "👍",
	}))
	if err == nil {
		t.Fatal("Expected error for empty conversation_id")
	}

	_, err = svc.AddReaction(ctx, connect.NewRequest(&api.AddReactionRequest{
		ConversationId: "conv1",
		MessageId:      "msg1",
		Emoji:          "",
	}))
	if err == nil {
		t.Fatal("Expected error for empty emoji")
	}
}

func TestReactionsInHistory(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "alice", "Alice")
	createTestUsers(t, sqlStorage, "bob", "Bob")
	communityID := createTestCommunity(t, sqlStorage, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "bob")

	convID, msgID := createConversationWithMessage(t, svc, sqlStorage, "alice", communityID)
	addParticipant(t, sqlStorage, convID, "bob")

	// Bob reacts.
	bobCtx := contextWithAuth("bob", "bob@example.com")
	_, err := svc.AddReaction(bobCtx, connect.NewRequest(&api.AddReactionRequest{
		ConversationId: convID,
		MessageId:      msgID,
		Emoji:          "👍",
	}))
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	// Fetch history and verify reactions are included.
	aliceCtx := contextWithAuth("alice", "alice@example.com")
	histResp, err := svc.GetConversationHistory(aliceCtx, connect.NewRequest(&api.GetConversationHistoryRequest{
		ConversationId: convID,
	}))
	if err != nil {
		t.Fatalf("GetConversationHistory failed: %v", err)
	}

	// Find the user message (skip system messages).
	var found bool
	for _, item := range histResp.Msg.Messages {
		if item.MessageId == msgID {
			found = true
			if len(item.Reactions) != 1 {
				t.Fatalf("Expected 1 reaction in history, got %d", len(item.Reactions))
			}
			if item.Reactions[0].Emoji != "👍" {
				t.Errorf("Expected 👍 in history, got %s", item.Reactions[0].Emoji)
			}
			break
		}
	}
	if !found {
		t.Fatal("Message not found in history")
	}
}

// TestConvertReactionsToAPI_DeletedSender verifies that reactions from a
// soft-deleted user surface as a former-member placeholder (id preserved,
// FormerMember=true, no PII). Regression for #1670 — the old behavior was
// Sender=nil which would crash clients reading sender.name.
func TestConvertReactionsToAPI_DeletedSender(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)
	ctx := context.Background()

	createTestUsers(t, sqlStorage, "active-reactor", "Active Reactor")
	createTestUsers(t, sqlStorage, "ghost-reactor", "Ghost Reactor")

	// Soft-delete the ghost reactor.
	ghost := &models.User{}
	if err := sqlStorage.GetByID(ctx, "ghost-reactor", ghost); err != nil {
		t.Fatalf("Load ghost: %v", err)
	}
	ghost.Deleted = &models.DeletedMetadata{DeletedByUserId: ghost.Id, DeletedAtUnixSec: 1}
	if err := sqlStorage.Update(ctx, ghost); err != nil {
		t.Fatalf("Soft-delete ghost: %v", err)
	}

	reactions := []*models.MessageReaction{
		{UserId: "active-reactor", Emoji: "👍"},
		{UserId: "ghost-reactor", Emoji: "❤️"},
	}
	apiReactions := convertReactionsToAPI(ctx, svc, reactions)
	if len(apiReactions) != 2 {
		t.Fatalf("len = %d, want 2", len(apiReactions))
	}

	var active, ghostAPI *api.Reaction
	for _, r := range apiReactions {
		if r.Sender == nil {
			t.Fatalf("Reaction.Sender must never be nil: %+v", r)
		}
		switch r.Sender.Id {
		case "active-reactor":
			active = r
		case "ghost-reactor":
			ghostAPI = r
		}
	}
	if active == nil || active.Sender.FormerMember || active.Sender.Name != "Active Reactor" {
		t.Errorf("Active reactor = %+v; want Name=Active Reactor, FormerMember=false", active)
	}
	if ghostAPI == nil || !ghostAPI.Sender.FormerMember || ghostAPI.Sender.Name != "" {
		t.Errorf("Ghost reactor = %+v; want FormerMember=true, Name=\"\"", ghostAPI)
	}
}
