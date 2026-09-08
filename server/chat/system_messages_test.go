package chat

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/chat_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestSystemMessageWriter_InsertSystemMessage(t *testing.T) {
	storage, _ := setupTestStorage(t)
	ctx := context.Background()

	// Create a conversation to insert messages into
	conversation := &models.ChatConversation{
		CommunityId:    "community123",
		ParticipantIds: []string{"user1", "user2"},
	}
	convID, err := storage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("Failed to create test conversation: %v", err)
	}

	writer := NewSystemMessageWriter(storage, nil)

	tests := []struct {
		name           string
		conversationID string
		actorID        string
		action         models.ChatSystemAction
		text           string
		wantErr        bool
	}{
		{
			name:           "valid joined message",
			conversationID: convID,
			actorID:        "user1",
			action:         models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED,
			text:           "Alex joined the conversation",
			wantErr:        false,
		},
		{
			name:           "valid left message",
			conversationID: convID,
			actorID:        "user1",
			action:         models.ChatSystemAction_CHAT_SYSTEM_ACTION_LEFT,
			text:           "Alex left the conversation",
			wantErr:        false,
		},
		{
			name:           "valid approved message",
			conversationID: convID,
			actorID:        "owner1",
			action:         models.ChatSystemAction_CHAT_SYSTEM_ACTION_APPROVED,
			text:           "Bob was selected as the recipient",
			wantErr:        false,
		},
		{
			name:           "valid cancelled message",
			conversationID: convID,
			actorID:        "owner1",
			action:         models.ChatSystemAction_CHAT_SYSTEM_ACTION_CANCELLED,
			text:           "This request was cancelled",
			wantErr:        false,
		},
		{
			name:           "valid started message",
			conversationID: convID,
			actorID:        "owner1",
			action:         models.ChatSystemAction_CHAT_SYSTEM_ACTION_STARTED,
			text:           "The loan has started",
			wantErr:        false,
		},
		{
			name:           "valid completed message",
			conversationID: convID,
			actorID:        "owner1",
			action:         models.ChatSystemAction_CHAT_SYSTEM_ACTION_COMPLETED,
			text:           "This transfer has been completed",
			wantErr:        false,
		},
		{
			name:           "valid offered message",
			conversationID: convID,
			actorID:        "helper1",
			action:         models.ChatSystemAction_CHAT_SYSTEM_ACTION_OFFERED,
			text:           "Carol offered to help",
			wantErr:        false,
		},
		{
			name:           "valid fulfilled message",
			conversationID: convID,
			actorID:        "requester1",
			action:         models.ChatSystemAction_CHAT_SYSTEM_ACTION_FULFILLED,
			text:           "This request has been fulfilled",
			wantErr:        false,
		},
		{
			name:           "empty conversation ID fails",
			conversationID: "",
			actorID:        "user1",
			action:         models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED,
			text:           "Alex joined",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := writer.InsertSystemMessage(
				ctx,
				tt.conversationID,
				tt.actorID,
				tt.action,
				tt.text,
			)

			if (err != nil) != tt.wantErr {
				t.Errorf("InsertSystemMessage() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSystemMessageWriter_VerifyMessageFields(t *testing.T) {
	storage, _ := setupTestStorage(t)
	ctx := context.Background()

	// Create a conversation
	conversation := &models.ChatConversation{
		CommunityId:    "community123",
		ParticipantIds: []string{"user1", "user2"},
	}
	convID, err := storage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("Failed to create test conversation: %v", err)
	}

	writer := NewSystemMessageWriter(storage, nil)

	// Insert a system message
	err = writer.InsertSystemMessage(
		ctx,
		convID,
		"actor123",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED,
		"Actor joined the conversation",
	)
	if err != nil {
		t.Fatalf("InsertSystemMessage failed: %v", err)
	}

	// Query the message back
	messages, err := storage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("Failed to query messages: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	msg := messages[0].(*models.ChatMessage)

	// Verify all fields
	if msg.ConversationId != convID {
		t.Errorf("ConversationId = %s, want %s", msg.ConversationId, convID)
	}
	if msg.SentAtUnixSec == 0 {
		t.Error("SentAtUnixSec should be set")
	}
	if msg.Id == "" {
		t.Error("Id should be set")
	}

	// Verify it's a system message
	systemMsg, ok := msg.Message.(*models.ChatMessage_SystemMessage)
	if !ok {
		t.Fatalf("Expected system message, got %T", msg.Message)
	}

	if systemMsg.SystemMessage.GetActorId() != "actor123" {
		t.Errorf("ActorId = %s, want actor123", systemMsg.SystemMessage.GetActorId())
	}
	if systemMsg.SystemMessage.Description != "Actor joined the conversation" {
		t.Errorf("Description = %s, want 'Actor joined the conversation'", systemMsg.SystemMessage.Description)
	}
	if systemMsg.SystemMessage.Action != models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED {
		t.Errorf("Action = %v, want CHAT_SYSTEM_ACTION_JOINED", systemMsg.SystemMessage.Action)
	}
}

// TestSystemMessageWriter_PublishesMessages verifies system messages are published to the chat bus.
func TestSystemMessageWriter_PublishesMessages(t *testing.T) {
	storage, _ := setupTestStorage(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		CommunityId:    "community123",
		ParticipantIds: []string{"user1", "user2"},
	}
	convID, err := storage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	mockBus := chat_event_bus.NewMockBus()
	writer := NewSystemMessageWriter(storage, mockBus)

	err = writer.InsertSystemMessage(
		ctx,
		convID,
		"user1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED,
		"User1 joined the conversation",
	)
	if err != nil {
		t.Fatalf("InsertSystemMessage failed: %v", err)
	}

	if mockBus.CallCount() != 1 {
		t.Fatalf("Expected 1 bus publish call, got %d", mockBus.CallCount())
	}

	evt := mockBus.Captured()[0]
	if evt.Kind != chat_event_bus.KindSystemMessage {
		t.Errorf("Published kind = %v, want KindSystemMessage", evt.Kind)
	}
	if evt.Conversation.Id != convID {
		t.Errorf("Published conversation ID = %s, want %s", evt.Conversation.Id, convID)
	}
	sys := evt.Message.GetSystemMessage()
	if sys == nil {
		t.Fatal("Published message has no system message")
	}
	if sys.Description != "User1 joined the conversation" {
		t.Errorf("Published description = %s, want 'User1 joined the conversation'", sys.Description)
	}
}

// TestSystemMessageWriter_UpdatesConversationTimestamp verifies last message timestamp is updated.
func TestSystemMessageWriter_UpdatesConversationTimestamp(t *testing.T) {
	storage, _ := setupTestStorage(t)
	ctx := context.Background()

	// Create conversation with participants
	originalTime := int64(1000000)
	conversation := &models.ChatConversation{
		CommunityId:          "community123",
		ParticipantIds:       []string{"user1", "user2"},
		LastMessageAtUnixSec: originalTime,
	}
	convID, err := storage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	// Create writer
	writer := NewSystemMessageWriter(storage, nil)

	// Insert system message
	err = writer.InsertSystemMessage(
		ctx,
		convID,
		"user1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_COMPLETED,
		"Transfer completed",
	)
	if err != nil {
		t.Fatalf("InsertSystemMessage failed: %v", err)
	}

	// Fetch conversation and verify timestamp was updated
	updatedConv := &models.ChatConversation{}
	if err := storage.GetByID(ctx, convID, updatedConv); err != nil {
		t.Fatalf("Failed to fetch conversation: %v", err)
	}

	if updatedConv.LastMessageAtUnixSec <= originalTime {
		t.Errorf("Conversation timestamp not updated: got %d, want > %d",
			updatedConv.LastMessageAtUnixSec, originalTime)
	}
}

// TestSystemMessageWriter_InitializesReadStatus verifies per-participant read status is initialized.
func TestSystemMessageWriter_InitializesReadStatus(t *testing.T) {
	storage, _ := setupTestStorage(t)
	ctx := context.Background()

	// Create conversation with participants
	conversation := &models.ChatConversation{
		CommunityId:    "community123",
		ParticipantIds: []string{"user1", "user2", "user3"},
	}
	convID, err := storage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	// Create writer
	writer := NewSystemMessageWriter(storage, nil)

	// Insert system message
	err = writer.InsertSystemMessage(
		ctx,
		convID,
		"user1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_STARTED,
		"Loan started",
	)
	if err != nil {
		t.Fatalf("InsertSystemMessage failed: %v", err)
	}

	// Fetch message and verify read status
	messages, err := storage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("Failed to query messages: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	msg := messages[0].(*models.ChatMessage)

	// All participants should be marked as unread
	for _, participantID := range conversation.ParticipantIds {
		isRead, exists := msg.ParticipantIdToIsRead[participantID]
		if !exists {
			t.Errorf("Read status not initialized for participant %s", participantID)
		}
		if isRead {
			t.Errorf("Participant %s marked as read, want unread", participantID)
		}
	}

	// Verify we have exactly the right number of entries
	if len(msg.ParticipantIdToIsRead) != len(conversation.ParticipantIds) {
		t.Errorf("Read status map has %d entries, want %d",
			len(msg.ParticipantIdToIsRead), len(conversation.ParticipantIds))
	}
}

// TestSystemMessageWriter_NilBroadcaster verifies system message writer works without a broadcaster.
func TestSystemMessageWriter_NilBroadcaster(t *testing.T) {
	storage, _ := setupTestStorage(t)
	ctx := context.Background()

	// Create conversation with participants
	conversation := &models.ChatConversation{
		CommunityId:    "community123",
		ParticipantIds: []string{"user1", "user2"},
	}
	convID, err := storage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	// Create writer with nil broadcaster (like in unit tests)
	writer := NewSystemMessageWriter(storage, nil)

	// Insert system message - should not panic
	err = writer.InsertSystemMessage(
		ctx,
		convID,
		"user1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED,
		"User1 joined the conversation",
	)
	if err != nil {
		t.Fatalf("InsertSystemMessage failed with nil broadcaster: %v", err)
	}

	// Verify message was stored
	messages, err := storage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("Failed to query messages: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
}
