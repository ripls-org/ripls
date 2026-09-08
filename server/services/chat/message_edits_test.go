package chat

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/chat_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// findUserMessage returns the history item for the given message ID, or nil.
func findHistoryItem(items []*api.MessageHistoryItem, messageID string) *api.MessageHistoryItem {
	for _, item := range items {
		if item.MessageId == messageID {
			return item
		}
	}
	return nil
}

func TestEditMessage(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "alice", "Alice")
	communityID := createTestCommunity(t, sqlStorage, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "alice")

	convID, msgID := createConversationWithMessage(t, svc, sqlStorage, "alice", communityID)

	ctx := contextWithAuth("alice", "alice@example.com")
	resp, err := svc.EditMessage(ctx, connect.NewRequest(&api.EditMessageRequest{
		ConversationId: convID,
		MessageId:      msgID,
		Text:           "edited text",
	}))
	if err != nil {
		t.Fatalf("EditMessage failed: %v", err)
	}
	if resp.Msg.EditedAtUnixSec == 0 {
		t.Error("Expected non-zero edited_at_unix_sec")
	}

	// Verify persisted.
	chatMsg := &models.ChatMessage{}
	if err := sqlStorage.GetByID(context.Background(), msgID, chatMsg); err != nil {
		t.Fatalf("Failed to get message: %v", err)
	}
	userMsg := chatMsg.GetUserMessage()
	if userMsg.Text != "edited text" {
		t.Errorf("Expected text 'edited text', got %q", userMsg.Text)
	}
	if userMsg.GetEditedAtUnixSec() == 0 {
		t.Error("Expected edited_at_unix_sec set on stored message")
	}

	// Verify edited_at surfaces in history.
	histResp, err := svc.GetConversationHistory(ctx, connect.NewRequest(&api.GetConversationHistoryRequest{
		ConversationId: convID,
	}))
	if err != nil {
		t.Fatalf("GetConversationHistory failed: %v", err)
	}
	item := findHistoryItem(histResp.Msg.Messages, msgID)
	if item == nil {
		t.Fatal("Edited message not found in history")
	}
	if item.GetUserMessage().GetEditedAtUnixSec() == 0 {
		t.Error("Expected edited_at in history user message")
	}
	if item.GetUserMessage().Text != "edited text" {
		t.Errorf("Expected edited text in history, got %q", item.GetUserMessage().Text)
	}
}

func TestEditMessage_NonAuthorRejected(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "alice", "Alice")
	createTestUsers(t, sqlStorage, "bob", "Bob")
	communityID := createTestCommunity(t, sqlStorage, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "bob")

	convID, msgID := createConversationWithMessage(t, svc, sqlStorage, "alice", communityID)
	addParticipant(t, sqlStorage, convID, "bob")

	bobCtx := contextWithAuth("bob", "bob@example.com")
	_, err := svc.EditMessage(bobCtx, connect.NewRequest(&api.EditMessageRequest{
		ConversationId: convID,
		MessageId:      msgID,
		Text:           "bob's edit",
	}))
	if err == nil {
		t.Fatal("Expected error editing another user's message")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok || connectErr.Code() != connect.CodePermissionDenied {
		t.Errorf("Expected PermissionDenied, got %v", err)
	}
}

func TestEditMessage_EmptyTextRejected(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "alice", "Alice")
	communityID := createTestCommunity(t, sqlStorage, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "alice")

	convID, msgID := createConversationWithMessage(t, svc, sqlStorage, "alice", communityID)

	ctx := contextWithAuth("alice", "alice@example.com")
	_, err := svc.EditMessage(ctx, connect.NewRequest(&api.EditMessageRequest{
		ConversationId: convID,
		MessageId:      msgID,
		Text:           "",
	}))
	if err == nil {
		t.Fatal("Expected error for empty text")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok || connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Expected InvalidArgument, got %v", err)
	}
}

func TestEditMessage_NotFound(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "alice", "Alice")
	communityID := createTestCommunity(t, sqlStorage, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "alice")

	convID, _ := createConversationWithMessage(t, svc, sqlStorage, "alice", communityID)

	ctx := contextWithAuth("alice", "alice@example.com")
	_, err := svc.EditMessage(ctx, connect.NewRequest(&api.EditMessageRequest{
		ConversationId: convID,
		MessageId:      "nonexistent",
		Text:           "edit",
	}))
	if err == nil {
		t.Fatal("Expected error for missing message")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok || connectErr.Code() != connect.CodeNotFound {
		t.Errorf("Expected NotFound, got %v", err)
	}
}

func TestDeleteMessage(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "alice", "Alice")
	communityID := createTestCommunity(t, sqlStorage, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "alice")

	convID, msgID := createConversationWithMessage(t, svc, sqlStorage, "alice", communityID)

	ctx := contextWithAuth("alice", "alice@example.com")
	_, err := svc.DeleteMessage(ctx, connect.NewRequest(&api.DeleteMessageRequest{
		ConversationId: convID,
		MessageId:      msgID,
	}))
	if err != nil {
		t.Fatalf("DeleteMessage failed: %v", err)
	}

	// Deleted message must be filtered from history.
	histResp, err := svc.GetConversationHistory(ctx, connect.NewRequest(&api.GetConversationHistoryRequest{
		ConversationId: convID,
	}))
	if err != nil {
		t.Fatalf("GetConversationHistory failed: %v", err)
	}
	if item := findHistoryItem(histResp.Msg.Messages, msgID); item != nil {
		t.Error("Deleted message should not appear in history")
	}
}

func TestDeleteMessage_NonAuthorRejected(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "alice", "Alice")
	createTestUsers(t, sqlStorage, "bob", "Bob")
	communityID := createTestCommunity(t, sqlStorage, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "bob")

	convID, msgID := createConversationWithMessage(t, svc, sqlStorage, "alice", communityID)
	addParticipant(t, sqlStorage, convID, "bob")

	bobCtx := contextWithAuth("bob", "bob@example.com")
	_, err := svc.DeleteMessage(bobCtx, connect.NewRequest(&api.DeleteMessageRequest{
		ConversationId: convID,
		MessageId:      msgID,
	}))
	if err == nil {
		t.Fatal("Expected error deleting another user's message")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok || connectErr.Code() != connect.CodePermissionDenied {
		t.Errorf("Expected PermissionDenied, got %v", err)
	}
}

func TestSendMessage_Reply(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "alice", "Alice")
	communityID := createTestCommunity(t, sqlStorage, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "alice")

	convID, parentMsgID := createConversationWithMessage(t, svc, sqlStorage, "alice", communityID)

	ctx := contextWithAuth("alice", "alice@example.com")
	replyResp, err := svc.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId:   convID,
		Text:             "this is a reply",
		ReplyToMessageId: &parentMsgID,
	}))
	if err != nil {
		t.Fatalf("SendMessage (reply) failed: %v", err)
	}

	// Verify the reply carries a denormalized quote of the parent.
	histResp, err := svc.GetConversationHistory(ctx, connect.NewRequest(&api.GetConversationHistoryRequest{
		ConversationId: convID,
	}))
	if err != nil {
		t.Fatalf("GetConversationHistory failed: %v", err)
	}
	item := findHistoryItem(histResp.Msg.Messages, replyResp.Msg.MessageId)
	if item == nil {
		t.Fatal("Reply not found in history")
	}
	replyTo := item.GetUserMessage().GetReplyTo()
	if replyTo == nil {
		t.Fatal("Expected reply_to on reply message")
	}
	if replyTo.ReplyToMessageId != parentMsgID {
		t.Errorf("Expected reply_to_message_id %q, got %q", parentMsgID, replyTo.ReplyToMessageId)
	}
	if replyTo.QuotedText != "A message to react to" {
		t.Errorf("Expected quoted text from parent, got %q", replyTo.QuotedText)
	}
	if replyTo.GetQuotedSender().GetId() != "alice" {
		t.Errorf("Expected quoted sender alice, got %q", replyTo.GetQuotedSender().GetId())
	}
}

func TestSendMessage_ReplyToMissingParent(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	createTestUsers(t, sqlStorage, "alice", "Alice")
	communityID := createTestCommunity(t, sqlStorage, "alice")
	createTestCommunityMembership(t, sqlStorage, communityID, "alice")

	convID, _ := createConversationWithMessage(t, svc, sqlStorage, "alice", communityID)

	ctx := contextWithAuth("alice", "alice@example.com")
	missing := "does-not-exist"
	resp, err := svc.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId:   convID,
		Text:             "reply to nothing",
		ReplyToMessageId: &missing,
	}))
	if err != nil {
		t.Fatalf("SendMessage should not fail on stale reply target: %v", err)
	}

	// The message sends, but without a quote block.
	chatMsg := &models.ChatMessage{}
	if err := sqlStorage.GetByID(context.Background(), resp.Msg.MessageId, chatMsg); err != nil {
		t.Fatalf("Failed to get message: %v", err)
	}
	if chatMsg.GetUserMessage().GetReplyTo() != nil {
		t.Error("Expected no reply_to for a missing parent")
	}
}

// TestStreamSubscriber_EditAndDelete verifies the new event kinds convert to
// the in-place update / delete stream responses clients expect.
func TestStreamSubscriber_EditAndDelete(t *testing.T) {
	svc, _ := setupTestChatService(t)
	ss := &chatStreamSubscriber{svc: svc}
	ctx := context.Background()

	conversation := &models.ChatConversation{Id: "conv1"}
	now := int64(1700000000)
	editedMsg := &models.ChatMessage{
		Id:             "msg1",
		ConversationId: "conv1",
		SentAtUnixSec:  now,
		Message: &models.ChatMessage_UserMessage{
			UserMessage: &models.UserChatMessage{
				SenderId:        "alice",
				Text:            "new text",
				EditedAtUnixSec: &now,
			},
		},
	}

	editResp := ss.toStreamResponse(ctx, &chat_event_bus.PublishedEvent{
		Kind:         chat_event_bus.KindUserMessageUpdate,
		Message:      editedMsg,
		Conversation: conversation,
	})
	if editResp == nil {
		t.Fatal("Expected stream response for KindUserMessageUpdate")
	}
	update := editResp.GetUserMessageUpdate()
	if update == nil {
		t.Fatal("Expected UserMessageUpdate variant")
	}
	if update.MessageId != "msg1" || update.Text != "new text" || update.EditedAtUnixSec != now {
		t.Errorf("Unexpected UserMessageUpdate: %+v", update)
	}

	deleteResp := ss.toStreamResponse(ctx, &chat_event_bus.PublishedEvent{
		Kind:         chat_event_bus.KindMessageDelete,
		Message:      editedMsg,
		Conversation: conversation,
	})
	if deleteResp == nil {
		t.Fatal("Expected stream response for KindMessageDelete")
	}
	del := deleteResp.GetMessageDelete()
	if del == nil || del.MessageId != "msg1" {
		t.Errorf("Unexpected MessageDelete: %+v", del)
	}
}
